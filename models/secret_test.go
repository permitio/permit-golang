package models_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/redact"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// credentialTexts returns the forms of credential that a printer could write: the credential
// itself, and its hex encoding in lower and upper case, as %x and %X write a string.
func credentialTexts(credential string) []string {
	return []string{
		credential, hex.EncodeToString([]byte(credential)),
		strings.ToUpper(hex.EncodeToString([]byte(credential))),
	}
}

// apiKeyWith returns an API key whose secret holds credential, as a reply decodes into one.
func apiKeyWith(credential string) models.APIKeyRead {
	return models.APIKeyRead{
		OrganizationID: "o", OwnerType: models.APIKeyOwnerTypeMember, ID: "k",
		Secret: models.NewSecret(credential), Name: "CI key",
	}
}

// TestSecretPrintsNoCredential checks that fmt with every verb, flag and width prints the marker
// and never the credential of a Secret, of an APIKeyRead that holds one, by value and by
// pointer, of a list of them and of an exported field that holds one; that fmt prints a Secret
// in an unexported field, which it cannot call the methods of, as an address, and never as the
// credential; and that String, GoString, LogValue and MarshalJSON give the marker.
func TestSecretPrintsNoCredential(t *testing.T) {
	credential := canary.Key("models")
	secret := models.NewSecret(credential)
	key := apiKeyWith(credential)
	type exported struct{ Key models.APIKeyRead }
	type unexported struct{ key models.APIKeyRead }
	scanner := canary.NewScanner(credentialTexts(credential)...)
	for name, tc := range map[string]struct {
		value any
		// marker says that fmt calls the Secret's methods, so the text holds the marker.
		marker bool
	}{
		"a Secret":                         {secret, true},
		"a *Secret":                        {&secret, true},
		"an APIKeyRead":                    {key, true},
		"a *APIKeyRead":                    {&key, true},
		"a []APIKeyRead":                   {[]models.APIKeyRead{key}, true},
		"an exported field":                {exported{key}, true},
		"a pointer to an exported field":   {&exported{key}, true},
		"an unexported field":              {unexported{key}, false},
		"a pointer to an unexported field": {&unexported{key}, false},
	} {
		for _, format := range []string{
			"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%10.3s", "%-20v",
		} {
			got := fmt.Sprintf(format, tc.value)
			_, _ = scanner.Write([]byte(got + "\n"))
			if tc.marker && !strings.Contains(got, redact.Marker) {
				t.Errorf("Sprintf(%q, %s) = %q, want it to hold %s", format, name, got,
					redact.Marker)
			}
		}
	}
	encoded, err := secret.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct{ got, want string }{
		"String":      {secret.String(), redact.Marker},
		"GoString":    {secret.GoString(), redact.Marker},
		"LogValue":    {secret.LogValue().String(), redact.Marker},
		"MarshalJSON": {string(encoded), `"` + redact.Marker + `"`},
	} {
		_, _ = scanner.Write([]byte(tc.got + "\n"))
		if tc.got != tc.want {
			t.Errorf("%s() = %q, want %q", name, tc.got, tc.want)
		}
	}
	scanner.Check(t)
}

// TestSecretInLogsAndJSON checks that slog's text and JSON handlers, and encoding/json, write the
// marker in place of the credential of a Secret and of an APIKeyRead that holds one, and that a
// log line still names the other fields of the key.
func TestSecretInLogsAndJSON(t *testing.T) {
	credential := canary.Key("modelslog")
	key := apiKeyWith(credential)
	for name, newHandler := range map[string]func(io.Writer) slog.Handler{
		"text": func(w io.Writer) slog.Handler { return slog.NewTextHandler(w, nil) },
		"JSON": func(w io.Writer) slog.Handler { return slog.NewJSONHandler(w, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			slog.New(newHandler(&out)).Info("a key", "credential", key.Secret, "key", key,
				"pointer", &key)
			scanner := canary.NewScanner(credentialTexts(credential)...)
			_, _ = scanner.Write(out.Bytes())
			scanner.Check(t)
			if got := strings.Count(out.String(), redact.Marker); got != 3 ||
				!strings.Contains(out.String(), "CI key") {
				t.Errorf("the log holds %s %d times, want 3, and the key's name: %s",
					redact.Marker, got, &out)
			}
		})
	}
	encoded, err := json.Marshal(key)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"secret":"`+redact.Marker+`"`)) ||
		bytes.Contains(encoded, []byte(credential)) {
		t.Errorf("json.Marshal of the key = %s, want the marker in place of its secret", encoded)
	}
}

// TestSecretKeepsItsCredential checks that Value returns the credential that a Secret holds, and
// "" for the zero Secret, which prints nothing and which an APIKeyRead leaves out when it is
// encoded; and that Equal compares credentials, so that go-cmp, which uses it, finds two keys
// with the same credential equal, and two with different credentials not, also of one length.
func TestSecretKeepsItsCredential(t *testing.T) {
	const credential = "opaque-kept-value"
	secret := models.NewSecret(credential)
	if secret.Value() != credential {
		t.Errorf("Value() = %q, want %q", secret.Value(), credential)
	}
	var empty models.Secret
	if empty.Value() != "" || empty.String() != "" || fmt.Sprintf("%v|%+v", empty, empty) != "|" {
		t.Errorf("the zero Secret holds %q and prints %q", empty.Value(), empty.String())
	}
	encoded, err := wire.Encode(models.APIKeyRead{ID: "k"})
	if err != nil || bytes.Contains(encoded, []byte("secret")) {
		t.Errorf("Encode of a key without a secret = %s, %v; want no secret field", encoded, err)
	}
	for _, tc := range []struct {
		a, b models.Secret
		want bool
	}{
		{secret, models.NewSecret(credential), true},
		{secret, models.NewSecret("opaque-other-value"), false},
		{secret, models.NewSecret("opaque-kept-VALUE"), false},
		{secret, empty, false},
		{empty, models.Secret{}, true},
		{empty, models.NewSecret(""), true},
	} {
		if got := tc.a.Equal(tc.b); got != tc.want {
			t.Errorf("Equal(%q, %q) = %t, want %t", tc.a.Value(), tc.b.Value(), got, tc.want)
		}
		if got := cmp.Equal(apiKeyWith(tc.a.Value()), apiKeyWith(tc.b.Value())); got != tc.want {
			t.Errorf("cmp.Equal of keys with %q and %q = %t, want %t", tc.a.Value(),
				tc.b.Value(), got, tc.want)
		}
	}
}

// TestSecretDecodes checks that a Secret decodes from a JSON string, as the secret of a reply,
// also the empty string; that null decodes to the empty Secret; and that any other JSON value is
// an error that does not hold the value, also inside an APIKeyRead that the SDK's decoder reads.
func TestSecretDecodes(t *testing.T) {
	for data, want := range map[string]string{
		`"opaque-decoded-value"`: "opaque-decoded-value",
		`""`:                     "",
		`null`:                   "",
	} {
		secret := models.NewSecret("opaque-before-value")
		if err := json.Unmarshal([]byte(data), &secret); err != nil || secret.Value() != want {
			t.Errorf("Unmarshal(%s) = %q, %v; want %q", data, secret.Value(), err, want)
		}
	}
	for _, data := range []string{
		`5`, `true`, `{"value":"opaque-nested-value"}`, `["opaque-listed-value"]`, `"unclosed`,
	} {
		var secret models.Secret
		err := secret.UnmarshalJSON([]byte(data))
		if err == nil || secret.Value() != "" || strings.Contains(err.Error(), "opaque") ||
			strings.Contains(err.Error(), data) {
			t.Errorf("UnmarshalJSON(%s) = %q, %v; want an error without the value", data,
				secret.Value(), err)
		}
		var key models.APIKeyRead
		err = wire.Decode([]byte(`{"id":"k","secret":`+data+`}`), &key)
		if err == nil || strings.Contains(err.Error(), "opaque") {
			t.Errorf("Decode of a key whose secret is %s returned %v, want an error without "+
				"the value", data, err)
		}
	}
	var key models.APIKeyRead
	if err := wire.Decode([]byte(`{"id":"k","secret":"opaque-key-value"}`), &key); err != nil ||
		key.Secret.Value() != "opaque-key-value" {
		t.Errorf("Decode of a key returned the secret %q and %v", key.Secret.Value(), err)
	}
}
