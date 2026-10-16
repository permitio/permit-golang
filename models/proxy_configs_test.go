package models_test

import (
	"bytes"
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

// The texts of the errors of the JSON of a ProxySecret.
const (
	proxySecretMany = "models: the ProxySecret sets more than one of Basic, Bearer and Headers; " +
		"set only the one of the proxy config's auth mechanism"
	notAProxySecret = "models: a ProxySecret is read from a JSON string, a JSON object of " +
		"strings or null"
)

// secretOf returns a pointer to a Secret that holds credential, for the Basic or the Bearer of a
// ProxySecret.
func secretOf(credential string) *models.Secret {
	return new(models.NewSecret(credential))
}

// headersOf returns the Headers of a ProxySecret that hold the credentials of pairs, a name and
// its credential each.
func headersOf(pairs ...string) map[string]models.Secret {
	headers := map[string]models.Secret{}
	for i := 0; i+1 < len(pairs); i += 2 {
		headers[pairs[i]] = models.NewSecret(pairs[i+1])
	}
	return headers
}

// TestProxySecretEncodes checks that a ProxySecret that sets one branch encodes it with the marker
// in place of each credential: a JSON string for Basic and Bearer, "" for an empty credential, and
// a JSON object of the headers, sorted, {} for an empty map that is not nil; that one that sets
// none, such as the zero ProxySecret, encodes as null, as it decodes from null, also inside a
// model, such as the zero ProxyConfigRead that All yields with an error; and that one that sets
// more than one fails to encode, also inside a model, with an error that holds no credential.
func TestProxySecretEncodes(t *testing.T) {
	marker := `"` + redact.Marker + `"`
	for name, tc := range map[string]struct {
		secret models.ProxySecret
		want   string
	}{
		"the zero ProxySecret":    {models.ProxySecret{}, `null`},
		"nil Headers":             {models.ProxySecret{Headers: nil}, `null`},
		"Basic":                   {models.ProxySecret{Basic: secretOf("opaque-user:pw")}, marker},
		"Bearer":                  {models.ProxySecret{Bearer: secretOf("opaque-token")}, marker},
		"an empty Bearer":         {models.ProxySecret{Bearer: secretOf("")}, `""`},
		"an empty map of headers": {models.ProxySecret{Headers: headersOf()}, `{}`},
		"Headers, sorted by name": {
			models.ProxySecret{
				Headers: headersOf("X-B", "opaque-b", "X-A", "opaque-a", "X-C", ""),
			},
			`{"X-A":` + marker + `,"X-B":` + marker + `,"X-C":""}`,
		},
	} {
		got, err := json.Marshal(tc.secret)
		if err != nil || string(got) != tc.want {
			t.Errorf("%s: Marshal = %s, %v; want %s", name, got, err, tc.want)
		}
	}
	for _, value := range []any{
		models.ProxyConfigCreate{Key: "k", Name: "n"},
		models.ProxyConfigUpdate{},
		models.ProxyConfigRead{},
	} {
		got, err := json.Marshal(value)
		if err != nil || !strings.Contains(string(got), `"secret":null`) {
			t.Errorf("Marshal(%T) = %s, %v; want the secret as null", value, got, err)
		}
	}
	for name, secret := range map[string]models.ProxySecret{
		"Basic and Bearer":   {Basic: secretOf("a:b"), Bearer: secretOf("t")},
		"Basic and Headers":  {Basic: secretOf("a:b"), Headers: headersOf()},
		"Bearer and Headers": {Bearer: secretOf("t"), Headers: headersOf()},
		"all three branches": {
			Basic: secretOf("a:b"), Bearer: secretOf("t"), Headers: headersOf("X", "h"),
		},
	} {
		got, err := secret.MarshalJSON()
		if got != nil || err == nil || err.Error() != proxySecretMany {
			t.Errorf("%s: MarshalJSON = %s, %v; want the error %q", name, got, err,
				proxySecretMany)
		}
		_, err = json.Marshal(models.ProxyConfigCreate{Key: "k", Name: "n", Secret: secret})
		if err == nil || !strings.Contains(err.Error(), proxySecretMany) ||
			strings.Contains(err.Error(), "a:b") {
			t.Errorf("%s: Marshal of a create = %v, want an error that holds %q and no "+
				"credential", name, err, proxySecretMany)
		}
	}
}

// TestProxySecretDecodes checks that a JSON object of strings decodes as Headers, also an empty
// one and a header whose value is null, which is the empty Secret; that a JSON string decodes as
// Bearer, the API's default mechanism, also the empty string; that null sets no branch, in place
// of one that was set; and that any other JSON value is an error that holds no part of the value
// and leaves the ProxySecret as it is.
func TestProxySecretDecodes(t *testing.T) {
	for data, want := range map[string]models.ProxySecret{
		`{"X-Api-Key":"opaque-header","X-Region":"eu"}`: {
			Headers: headersOf("X-Api-Key", "opaque-header", "X-Region", "eu"),
		},
		`{}`:               {Headers: headersOf()},
		`{"X-Empty":null}`: {Headers: headersOf("X-Empty", "")},
		`"opaque-token"`:   {Bearer: secretOf("opaque-token")},
		`"user:pass"`:      {Bearer: secretOf("user:pass")},
		`""`:               {Bearer: secretOf("")},
		`null`:             {},
	} {
		secret := models.ProxySecret{Basic: secretOf("opaque-before")}
		if err := json.Unmarshal([]byte(data), &secret); err != nil {
			t.Errorf("Unmarshal(%s) returned %v", data, err)
			continue
		}
		if diff := cmp.Diff(want, secret); diff != "" {
			t.Errorf("Unmarshal(%s) returned a secret that differs (-want +got):\n%s", data, diff)
		}
	}
	for _, data := range []string{
		`5`, `true`, `["opaque-listed"]`, `{"X-Api-Key":5,"X-Other":"opaque-other"}`,
		`{"X-Api-Key":{"value":"opaque-nested"}}`, `"opaque-unclosed`, `{"opaque-unclosed"`,
	} {
		before := models.ProxySecret{Bearer: secretOf("kept")}
		secret := before
		err := secret.UnmarshalJSON([]byte(data))
		if err == nil || err.Error() != notAProxySecret || !cmp.Equal(before, secret) {
			t.Errorf("UnmarshalJSON(%s) = %v, %v; want the error %q and the secret as it was",
				data, secret, err, notAProxySecret)
		}
	}
}

// TestProxySecretPrintsNoCredential checks that fmt with every verb, flag and width, slog's text
// and JSON handlers and encoding/json print the marker and never a credential of a ProxySecret of
// each branch, of a ProxyConfigRead that holds one, by value and by pointer, and of a list of
// them; and that String and GoString name the branch and the headers.
func TestProxySecretPrintsNoCredential(t *testing.T) {
	basic, bearer := canary.Key("proxybasic"), canary.Key("proxybearer")
	header := canary.Key("proxyheader")
	secrets := map[string]models.ProxySecret{
		"Basic":   {Basic: secretOf(basic)},
		"Bearer":  {Bearer: secretOf(bearer)},
		"Headers": {Headers: headersOf("X-Api-Key", header)},
		"two":     {Basic: secretOf(basic), Bearer: secretOf(bearer)},
	}
	scanner := canary.NewScanner(append(append(credentialTexts(basic),
		credentialTexts(bearer)...), credentialTexts(header)...)...)
	for name, secret := range secrets {
		config := models.ProxyConfigRead{Key: "payments", Name: "Payments API", Secret: secret}
		for _, value := range []any{
			secret, &secret, config, &config, []models.ProxyConfigRead{config},
		} {
			for _, format := range []string{
				"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%10.3s", "%-20v",
			} {
				got := fmt.Sprintf(format, value)
				_, _ = scanner.Write([]byte(got + "\n"))
				if !strings.Contains(got, redact.Marker) {
					t.Errorf("%s: Sprintf(%q, %T) = %q, want it to hold %s", name, format, value,
						got, redact.Marker)
				}
			}
		}
		for handlerName, newHandler := range map[string]func(io.Writer) slog.Handler{
			"text": func(w io.Writer) slog.Handler { return slog.NewTextHandler(w, nil) },
			"JSON": func(w io.Writer) slog.Handler { return slog.NewJSONHandler(w, nil) },
		} {
			var out bytes.Buffer
			slog.New(newHandler(&out)).Info("a config", "credential", secret, "config", config,
				"pointer", &config)
			_, _ = scanner.Write(out.Bytes())
			if name != "two" && strings.Count(out.String(), redact.Marker) != 3 {
				t.Errorf("%s: the %s log holds %s %d times, want 3: %s", name, handlerName,
					redact.Marker, strings.Count(out.String(), redact.Marker), &out)
			}
		}
		if encoded, err := json.Marshal(config); err == nil {
			_, _ = scanner.Write(encoded)
		}
	}
	scanner.Check(t)
	for name, want := range map[string]string{
		"Basic":   "{Basic:" + redact.Marker + "}",
		"Bearer":  "{Bearer:" + redact.Marker + "}",
		"Headers": "{Headers:map[X-Api-Key:" + redact.Marker + "]}",
		"two":     "{Basic:" + redact.Marker + " Bearer:" + redact.Marker + "}",
	} {
		if got := secrets[name].String(); got != want {
			t.Errorf("String() of %s = %q, want %q", name, got, want)
		}
		if got := secrets[name].GoString(); got != want {
			t.Errorf("GoString() of %s = %q, want %q", name, got, want)
		}
	}
	empty := models.ProxySecret{Bearer: secretOf(""), Headers: headersOf("X-Empty", "")}
	if got, want := empty.String(), "{Bearer: Headers:map[X-Empty:]}"; got != want {
		t.Errorf("String() of empty credentials = %q, want %q", got, want)
	}
	if got := (models.ProxySecret{}).String(); got != "{}" {
		t.Errorf("String() of the zero ProxySecret = %q, want {}", got)
	}
}

// TestProxyConfigReadSecret checks how a ProxyConfigRead reads its secret: a JSON string as Basic
// when the auth mechanism is Basic, and as Bearer for Bearer, for a mechanism that the reply leaves
// out, which the API's documentation gives as Bearer, and for one that the SDK does not know; a
// JSON object as Headers, whatever the mechanism; and the other fields as they are. null leaves
// the config as it is, and a reply whose secret does not decode is an error that holds no
// credential and leaves the config as it is.
func TestProxyConfigReadSecret(t *testing.T) {
	headersSecret := models.ProxySecret{Headers: headersOf("X-A", "opaque-a")}
	const fields = `"key":"payments","id":"i","organization_id":"o","project_id":"p",` +
		`"environment_id":"e","created_at":"2026-10-15T08:00:00Z",` +
		`"updated_at":"2026-10-15T08:00:00Z","name":"Payments API"`
	for _, tc := range []struct {
		mechanism, secret string
		want              models.ProxySecret
	}{
		{`"Basic"`, `"opaque-user:pass"`, models.ProxySecret{Basic: secretOf("opaque-user:pass")}},
		{`"Bearer"`, `"opaque-token"`, models.ProxySecret{Bearer: secretOf("opaque-token")}},
		{"", `"opaque-token"`, models.ProxySecret{Bearer: secretOf("opaque-token")}},
		{`"ApiKey"`, `"opaque-token"`, models.ProxySecret{Bearer: secretOf("opaque-token")}},
		{`"Headers"`, `{"X-A":"opaque-a"}`, headersSecret},
		{`"Basic"`, `{"X-A":"opaque-a"}`, headersSecret},
	} {
		body := `{` + fields + `,"secret":` + tc.secret
		var mechanism models.AuthMechanism
		if tc.mechanism != "" {
			body += `,"auth_mechanism":` + tc.mechanism
			mechanism = models.AuthMechanism(strings.Trim(tc.mechanism, `"`))
		}
		body += `,"mapping_rules":[{"url":"u","http_method":"get","resource":"r"}]}`
		var got models.ProxyConfigRead
		if err := wire.Decode([]byte(body), &got); err != nil {
			t.Errorf("Decode(%s) returned %v", body, err)
			continue
		}
		want := models.ProxyConfigRead{
			Key: "payments", ID: "i", OrganizationID: "o", ProjectID: "p", EnvironmentID: "e",
			CreatedAt: got.CreatedAt, UpdatedAt: got.UpdatedAt, Name: "Payments API",
			AuthMechanism: mechanism, Secret: tc.want,
			MappingRules: []models.MappingRule{{
				URL: "u", HTTPMethod: models.MethodsGet, Resource: "r",
			}},
		}
		if diff := cmp.Diff(want, got); diff != "" || got.CreatedAt.IsZero() {
			t.Errorf("Decode(%s) returned a config that differs (-want +got):\n%s", body, diff)
		}
	}
	before := models.ProxyConfigRead{Key: "kept", Secret: models.ProxySecret{
		Bearer: secretOf("kept"),
	}}
	for _, data := range []string{
		`null`, `{"key":"other","secret":5}`, `{"key":"other","secret":["opaque-listed"]}`,
		`{"key":"other","secret":{"X-A":{"v":"opaque-nested"}}}`, `[]`, `"opaque-string"`,
	} {
		got := before
		err := got.UnmarshalJSON([]byte(data))
		if (data == `null`) != (err == nil) || !cmp.Equal(before, got) ||
			err != nil && strings.Contains(err.Error(), "opaque") {
			t.Errorf("UnmarshalJSON(%s) = %+v, %v; want the config as it was, and an error "+
				"without a credential for a value that is not null", data, got, err)
		}
	}
}
