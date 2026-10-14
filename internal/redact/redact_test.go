package redact

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/internal/testkit/concurrent"
)

// secretTexts are key and its encodings in hexadecimal, which the %x and %X verbs write.
func secretTexts(key string) []string {
	return []string{
		key, hex.EncodeToString([]byte(key)), strings.ToUpper(hex.EncodeToString([]byte(key))),
	}
}

func TestSecretNeverPrintsItsValue(t *testing.T) {
	key := canary.Key("secret")
	secret := NewSecret(key)
	type exported struct {
		Name   string
		APIKey Secret
	}
	type unexported struct {
		name   string
		apiKey Secret
	}
	scanner := canary.NewScanner(secretTexts(key)...)
	for name, tc := range map[string]struct {
		value any
		// marker says that fmt calls the Secret's methods, so the text holds Marker.
		marker bool
	}{
		"a Secret":                         {secret, true},
		"a *Secret":                        {&secret, true},
		"a []Secret":                       {[]Secret{secret}, true},
		"an exported field":                {exported{"c", secret}, true},
		"a pointer to an exported field":   {&exported{"c", secret}, true},
		"an unexported field":              {unexported{name: "c", apiKey: secret}, false},
		"a pointer to an unexported field": {&unexported{name: "c", apiKey: secret}, false},
	} {
		for _, format := range []string{
			"%v", "%+v", "%#v", "%p", "%s", "%q", "%x", "%X", "%d", "%10.3s", "%-20v",
		} {
			got := fmt.Sprintf(format, tc.value)
			_, _ = scanner.Write([]byte(got + "\n"))
			if tc.marker && format != "%p" && !strings.Contains(got, Marker) {
				t.Errorf("Sprintf(%q, %s) = %q, want it to hold %s", format, name, got, Marker)
			}
		}
	}
	encoded, err := secret.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string]string{
		"String":      secret.String(),
		"GoString":    secret.GoString(),
		"LogValue":    secret.LogValue().String(),
		"Sprint":      fmt.Sprint(secret),
		"MarshalJSON": string(encoded),
	} {
		_, _ = scanner.Write([]byte(got + "\n"))
		want := Marker
		if name == "MarshalJSON" {
			want = `"` + Marker + `"`
		}
		if got != want {
			t.Errorf("%s() = %q, want %q", name, got, want)
		}
	}
	if kind := secret.LogValue().Kind().String(); kind != "String" {
		t.Errorf("LogValue().Kind() = %s, want String", kind)
	}
	scanner.Check(t)
}

func TestSecretInLogs(t *testing.T) {
	key := canary.Key("slog")
	secret := NewSecret(key)
	type exported struct{ APIKey Secret }
	type unexported struct{ apiKey Secret }
	for name, newHandler := range map[string]func(io.Writer) slog.Handler{
		"text": func(w io.Writer) slog.Handler { return slog.NewTextHandler(w, nil) },
		"JSON": func(w io.Writer) slog.Handler { return slog.NewJSONHandler(w, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			slog.New(newHandler(&out)).Info("configured",
				"credential", secret,
				"exported", exported{secret},
				"pointer", &exported{secret},
				"unexported", unexported{apiKey: secret},
			)

			scanner := canary.NewScanner(secretTexts(key)...)
			_, _ = scanner.Write(out.Bytes())
			scanner.Check(t)
			const want = 3 // the Secret, and the exported field twice
			if got := strings.Count(out.String(), Marker); got != want {
				t.Errorf("the log holds %s %d times, want %d: %s", Marker, got, want, &out)
			}
		})
	}
}

func TestEmptySecretPrintsNothing(t *testing.T) {
	var zero Secret
	for name, secret := range map[string]Secret{
		"the zero Secret": zero, `NewSecret("")`: NewSecret(""),
	} {
		encoded, err := secret.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		for what, got := range map[string]string{
			"String()":   secret.String(),
			"GoString()": secret.GoString(),
			"%v":         fmt.Sprintf("%v", secret),
			"%q":         fmt.Sprintf("%q", secret),
			"%+v":        fmt.Sprintf("%+v", struct{ Key Secret }{secret}),
			"LogValue()": secret.LogValue().String(),
			"Value()":    secret.Value(),
		} {
			want := ""
			if what == "%+v" {
				want = "{Key:}"
			}
			if got != want {
				t.Errorf("%s: %s = %q, want %q: an empty credential prints as empty", name, what,
					got, want)
			}
		}
		if string(encoded) != `""` {
			t.Errorf("%s: MarshalJSON() = %s, want \"\"", name, encoded)
		}
	}
}

func TestSecretKeepsItsValue(t *testing.T) {
	key := canary.Key("value")
	secret := NewSecret(key)
	copied := secret
	if secret.Value() != key || copied.Value() != key {
		t.Errorf("Value() = %q and of a copy %q, want %q", secret.Value(), copied.Value(), key)
	}
	got, err := json.Marshal(struct {
		Key Secret `json:"key"`
	}{secret})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"key":"` + Marker + `"}`; string(got) != want {
		t.Errorf("json.Marshal = %s, want %s: a request body takes Value", got, want)
	}
}

func TestSetAddAndContains(t *testing.T) {
	s := NewSet("first", "")
	s.Add("second", "first", "")

	for secret, want := range map[string]bool{
		"first": true, "second": true, "": false, "third": false, "firs": false,
	} {
		if got := s.Contains(secret); got != want {
			t.Errorf("Contains(%q) = %v, want %v", secret, got, want)
		}
	}
	if diff := cmp.Diff([]string{"first", "second"}, s.secrets); diff != "" {
		t.Errorf("the Set holds each secret once and no empty one (-want +got):\n%s", diff)
	}
}

// TestSetAddFields checks which strings of a JSON document AddFields adds: those under a key
// that is one of the fields or a built-in credential name, at any depth and in any case or
// spelling with '-' and '_', and every string of an object or a list under such a key; never
// another value, such as that of a key that only contains a name, and nothing from a body that is
// not JSON.
// TestMayHoldName checks the pass that lets AddFields skip decoding a reply in which no key can
// match: it must say yes whenever a key could, written in any case and with '-' or '_'.
func TestMayHoldName(t *testing.T) {
	for _, tc := range []struct {
		data   string
		listed []string
		want   bool
	}{
		{`{"allow":true,"query":{"user":{"key":"u"}}}`, nil, false},
		{`{"Access-Token":"x"}`, nil, true},
		{`{"api_key":"x"}`, nil, true},
		{`{"PRIVATE_KEY":"x"}`, nil, true},
		{`{"signing_value":"x"}`, []string{"signingvalue"}, true},
		{`{"signing_value":"x"}`, nil, false},
		{`not json`, nil, false},
	} {
		if got := mayHoldName([]byte(tc.data), tc.listed); got != tc.want {
			t.Errorf("mayHoldName(%s, %v) = %v, want %v", tc.data, tc.listed, got, tc.want)
		}
	}
}

func TestSetAddFields(t *testing.T) {
	for _, tc := range []struct {
		name   string
		data   string
		fields []string
		want   []string
	}{
		{
			name: "fields at any depth",
			data: `{"signing_value":"a","nested":{"Signing-Value":"b","other":"x"},` +
				`"list":[{"signingValue":"c"},"y"],"other":"z"}`,
			fields: []string{"signing_value"},
			want:   []string{"a", "b", "c"},
		},
		{
			name:   "an object and a list under a field",
			data:   `{"headers":{"Authorization":"d","X-Extra":["e",1,true,null]},"id":"w"}`,
			fields: []string{"headers"},
			want:   []string{"d", "e"},
		},
		{
			name:   "a key that only contains a field",
			data:   `{"secret_type":"headers","secret":"f"}`,
			fields: []string{"secret"},
			want:   []string{"f"},
		},
		{
			name:   "two fields",
			data:   `{"one":"g","two":"h","three":"i"}`,
			fields: []string{"one", "TWO"},
			want:   []string{"g", "h"},
		},
		{"a number under a field", `{"pin":1234}`, []string{"pin"}, nil},
		{"an empty string under a field", `{"pin":""}`, []string{"pin"}, nil},
		{"a body that is not JSON", `pin=j`, []string{"pin"}, nil},
		{"an empty body", ``, []string{"pin"}, nil},
		{"no fields", `{"pin":"k"}`, nil, nil},
		{"empty fields", `{"":"l","_":"m"}`, []string{"", "_-"}, nil},
		{
			name: "the secret names without fields",
			data: `{"secret":"a","Secret-Key":"b","clientSecret":"c","list":[{"SECRET":"d"}]}`,
			want: []string{"a", "b", "c", "d"},
		},
		{
			name: "the token names without fields",
			data: `{"token":"a","access_token":"b","RefreshToken":"c","nested":{"Token":"d"}}`,
			want: []string{"a", "b", "c", "d"},
		},
		{
			name: "the key names without fields",
			data: `{"api_key":"a","apiKey":"b","Access-Key":"c","private_key":"d"}`,
			want: []string{"a", "b", "c", "d"},
		},
		{
			name: "the password names without fields",
			data: `{"password":"a","passwd":"b","PWD":"c"}`,
			want: []string{"a", "b", "c"},
		},
		{
			name: "the credential names without fields",
			data: `{"credential":"a","credentials":{"user":"b","password":["c"]}}`,
			want: []string{"a", "c"},
		},
		{
			name: "an object under a credential name is searched by its own keys",
			data: `{"credentials":{"type":"basic","private_key":"k-1"}}`,
			want: []string{"k-1"},
		},
		{
			name: "a list under a credential name",
			data: `{"token":["x","y",{"z":"w"}]}`,
			want: []string{"x", "y"},
		},
		{
			name: "cookies",
			data: `{"cookie":"a","Set-Cookie":["b"]}`,
			want: []string{"a", "b"},
		},
		{
			name: "the authorization name without fields",
			data: `{"headers":{"Authorization":"Bearer a"}}`,
			want: []string{"Bearer a"},
		},
		{
			name: "keys that only contain a built-in name",
			data: `{"token_type":"bearer","secret_type":"headers","tokens":"a","api_key_id":"b",` +
				`"passwords":"c","has_credentials":"d","access_token_expiry":"e"}`,
			want: nil,
		},
		{
			name:   "a built-in name and a field",
			data:   `{"token":"a","signing_value":"b","other":"c"}`,
			fields: []string{"signing_value"},
			want:   []string{"a", "b"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewSet()
			s.AddFields([]byte(tc.data), tc.fields...)
			got := s.longestFirst()
			slices.Sort(got)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("the Set holds other secrets (-want +got):\n%s", diff)
			}
		})
	}
}

func TestZeroSetIsReady(t *testing.T) {
	var s Set
	if s.Contains("x") {
		t.Error("an empty Set contains x")
	}
	s.Add("credential-value")
	if !s.Contains("credential-value") {
		t.Error("Add on a zero Set did not add the secret")
	}
	if got := ScrubString(&s, "a credential-value b"); got != "a "+Marker+" b" {
		t.Errorf("ScrubString = %q, want the secret of the zero Set replaced", got)
	}
}

func TestSetsAreIndependent(t *testing.T) {
	first := NewSet(canary.Key("first"), "first-client-credential")
	second := NewSet(canary.Key("second"))
	second.Add("second-client-credential")

	for _, secret := range []string{canary.Key("second"), "second-client-credential"} {
		if first.Contains(secret) {
			t.Errorf("the first client's Set holds %q, a secret of the second client", secret)
		}
	}
	for _, secret := range []string{canary.Key("first"), "first-client-credential"} {
		if second.Contains(secret) {
			t.Errorf("the second client's Set holds %q, a secret of the first client", secret)
		}
	}
	text := "first-client-credential and second-client-credential"
	if got, want := ScrubString(first, text), Marker+" and second-client-credential"; got != want {
		t.Errorf("Scrub with the first Set = %q, want %q", got, want)
	}
	if got, want := ScrubString(second, text), "first-client-credential and "+Marker; got != want {
		t.Errorf("Scrub with the second Set = %q, want %q", got, want)
	}
}

func TestSetIsSafeForConcurrentUse(t *testing.T) {
	s := NewSet()
	const calls = 64
	err := concurrent.Run(t, calls, 30*time.Second, func(_ context.Context, i int) error {
		secret := "concurrent-credential-" + strconv.Itoa(i)
		s.Add(secret)
		if !s.Contains(secret) {
			return fmt.Errorf("the Set does not hold %q right after Add", secret)
		}
		if got := ScrubString(s, "x "+secret+" y"); got != "x "+Marker+" y" {
			return fmt.Errorf("ScrubString = %q, want the secret replaced", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(s.longestFirst()); got != calls {
		t.Errorf("the Set holds %d secrets after %d calls of Add, want %d", got, calls, calls)
	}
}

func TestLongestFirst(t *testing.T) {
	s := NewSet("ab", "abcd", "abc", "cd")
	if diff := cmp.Diff([]string{"abcd", "abc", "ab", "cd"}, s.longestFirst()); diff != "" {
		t.Errorf("longestFirst differs (-want +got):\n%s", diff)
	}
	var nilSet *Set
	if got := nilSet.longestFirst(); got != nil {
		t.Errorf("a nil Set holds %q, want none", got)
	}
}

func TestScrubReplacesALongerSecretFirst(t *testing.T) {
	s := NewSet("credential", "credential-and-more")
	if got := ScrubString(s, "credential-and-more"); got != Marker {
		t.Errorf("ScrubString = %q, want %q: the longer secret is replaced whole", got, Marker)
	}
}

// escapedP is the letter p written as a JSON escape, p, built so that no editor decodes it.
var escapedP = string(rune(92)) + "u0070"

func TestScrub(t *testing.T) {
	key := canary.Key("scrub")
	other := canary.Key("other")
	set := NewSet(key, "proxy-header-value")
	escapedKey := escapedP + key[1:]
	for name, tc := range map[string]struct{ in, want string }{
		"empty":                  {"", ""},
		"text without any":       {"no credential here", "no credential here"},
		"the key in text":        {"key " + key + " end", "key " + Marker + " end"},
		"another Permit API key": {"use " + other + ".", "use " + Marker + "."},
		"a member of the set":    {"x-proxy-header-value-y", "x-" + Marker + "-y"},
		"a short permit_key_":    {"permit_key_a.", Marker + "."},
		"permit_key_ alone":      {"permit_key_.", "permit_key_."},
		"a permit_key_ with _ and -": {
			"permit_key_" + strings.Repeat("a", 20) + "_b-c d", Marker + " d",
		},
		"an Authorization header": {
			"GET /x\r\nAuthorization: Bearer abc.def\r\nAccept: */*",
			"GET /x\r\nAuthorization: " + Marker + "\r\nAccept: */*",
		},
		"lower case, no space": {"authorization:Basic dXNlcg==", "authorization:" + Marker},
		"Proxy-Authorization": {
			"Proxy-Authorization: Basic dXNlcg== ok", "Proxy-Authorization: " + Marker + " ok",
		},
		"a scheme alone":  {"Authorization: Bearer", "Authorization: " + Marker},
		"an empty header": {"Authorization:", "Authorization:"},
		"a JSON credential": {
			`{"name":"n","secret":"s3","count":2}`,
			`{"count":2,"name":"n","secret":"` + Marker + `"}`,
		},
		"JSON without credentials keeps its bytes": {
			`{ "b": 1,  "a": [true, null] }`, `{ "b": 1,  "a": [true, null] }`,
		},
		"JSON keys in any case and spelling": {
			`{"Secret":"a","API-KEY":"b","apiKey":"c","X-Api-Key":"d","client_secret":"e",` +
				`"access_token":"f","Password":"g","Authorization":"h","key":"i"}`,
			`{"API-KEY":"[REDACTED]","Authorization":"[REDACTED]","Password":"[REDACTED]",` +
				`"Secret":"[REDACTED]","X-Api-Key":"[REDACTED]","access_token":"[REDACTED]",` +
				`"apiKey":"[REDACTED]","client_secret":"[REDACTED]","key":"i"}`,
		},
		"the credential names of v1.3.0": {
			`{"passwd":"a","db_pwd":"b","private_key":"c","access_key":"d","credential":"e",` +
				`"credentials":{"user":"f"},"secret_key":"g","cookie":"h","Set-Cookie":"i"}`,
			`{"Set-Cookie":"[REDACTED]","access_key":"[REDACTED]","cookie":"[REDACTED]",` +
				`"credential":"[REDACTED]","credentials":{"user":"[REDACTED]"},` +
				`"db_pwd":"[REDACTED]","passwd":"[REDACTED]","private_key":"[REDACTED]",` +
				`"secret_key":"[REDACTED]"}`,
		},
		"a JSON credential at depth": {
			`[{"a":{"b":[{"token":"t"}]}}]`, `[{"a":{"b":[{"token":"` + Marker + `"}]}}]`,
		},
		"an object under a credential key": {
			`{"secret":{"X-Header":"v","n":12,"b":true,"z":null,"l":["w",3]}}`,
			`{"secret":{"X-Header":"[REDACTED]","b":true,"l":["[REDACTED]","[REDACTED]"],` +
				`"n":"[REDACTED]","z":null}}`,
		},
		"a number under a credential key": {
			`{"pin":1,"token":123}`, `{"pin":1,"token":"` + Marker + `"}`,
		},
		"an empty string under a credential key": {
			`{"password":""}`, `{"password":"` + Marker + `"}`,
		},
		"an earlier duplicate of a credential key": {
			`{"secret":"s3","secret":null}`, `{"secret":null}`,
		},
		"an earlier duplicate of a credential key with a bool": {
			`{"token":"t","token":true}`, `{"token":true}`,
		},
		"the input of a validation error for a credential field": {
			`{"detail":[{"loc":["body","api_key"],"msg":"m","input":"s3"},` +
				`{"loc":["body","x","password"],"msg":"m","input":7},` +
				`{"loc":["body","name"],"msg":"m","input":"kept"}]}`,
			`{"detail":[{"input":"[REDACTED]","loc":["body","api_key"],"msg":"m"},` +
				`{"input":"[REDACTED]","loc":["body","x","password"],"msg":"m"},` +
				`{"input":"kept","loc":["body","name"],"msg":"m"}]}`,
		},
		"numbers keep their digits": {
			`{"n":9007199254740993,"f":1e300,"secret":"x"}`,
			`{"f":1e300,"n":9007199254740993,"secret":"` + Marker + `"}`,
		},
		"HTML characters stay": {
			`{"h":"<>&","secret":"x"}`, `{"h":"<>&","secret":"` + Marker + `"}`,
		},
		"an escaped key in a JSON string": {
			`{"message":"bad key ` + escapedKey + `"}`, `{"message":"bad key ` + Marker + `"}`,
		},
		"an escaped key as an object key": {
			`{"` + escapedKey + `":1,"a":2}`, `{"a":2,"` + Marker + `":1}`,
		},
		"a key as an object key": {`{"` + key + `":1}`, `{"` + Marker + `":1}`},
		"two keys as object keys stay two": {
			`{"` + key + `":1,"` + other + `":2}`,
			`{"` + Marker + `":2,"` + Marker + `":1}`,
		},
		"an Authorization header in a JSON string": {
			`{"note":"Authorization: Bearer abc"}`, `{"note":"Authorization: ` + Marker + `"}`,
		},
		"a pair in a JSON string": {
			`{"detail":"bad {\"secret\":\"s3\"} sent"}`,
			`{"detail":"bad {\"secret\":\"` + Marker + `\"} sent"}`,
		},
		"a JSON document in a JSON string": {
			`{"m":"{\"secret\":{\"h\":\"v\"}}"}`, `{"m":"{\"secret\":{\"h\":\"` + Marker + `\"}}"}`,
		},
		"a JSON string": {`"` + key + `"`, `"` + Marker + `"`},
		"JSON lines": {
			"{\"a\":{\"secret\":{\"h\":\"v\"}}}\n{\"n\":1,\"token\":2}\n {\"ok\":1}\n",
			"{\"a\":{\"secret\":{\"h\":\"" + Marker + "\"}}}\n{\"n\":1,\"token\":\"" + Marker +
				"\"}\n {\"ok\":1}\n",
		},
		"a JSON document and text": {
			`{"secret":{"h":"v"}} and "token": "t"`,
			`{"secret":{"h":"` + Marker + `"}} and "token": "` + Marker + `"`,
		},
		"a pair in text that is not JSON": {
			`{"secret": "a\"b", "name": "n"} trailing`,
			`{"name":"n","secret":"` + Marker + `"} trailing`,
		},
		"a pair alone": {`"api_key": "abc"`, `"api_key": "` + Marker + `"`},
		"a pair in HTML": {
			`<script>var c = {"api_key":"abc","user":"u"};</script>`,
			`<script>var c = {"api_key":"` + Marker + `","user":"u"};</script>`,
		},
		"truncated JSON": {
			`{"token": "abc", "items": [1, 2`, `{"token": "` + Marker + `", "items": [1, 2`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := string(Scrub(set, []byte(tc.in))); got != tc.want {
				t.Errorf("Scrub(%s)\n got %s\nwant %s", tc.in, got, tc.want)
			}
		})
	}
}

func TestScrubAMemberThatJSONEscapes(t *testing.T) {
	const member = "proxy\"value\\with\tescapes"
	escaped := strings.NewReplacer(`"`, `\"`, `\`, `\\`, "\t", `\t`).Replace(member)
	set := NewSet(member)
	for name, tc := range map[string]struct{ in, want string }{
		"in a string":      {`{"a":"x ` + escaped + ` y"}`, `{"a":"x ` + Marker + ` y"}`},
		"as an object key": {`{"` + escaped + `":1}`, Marker},
		"in a JSON string in a JSON string": {
			`{"m":"{\"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(escaped) + `\":1}"}`,
			`{"m":"` + Marker + `"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := string(Scrub(set, []byte(tc.in)))
			if got != tc.want {
				t.Errorf("Scrub(%s)\n got %s\nwant %s", tc.in, got, tc.want)
			}
			if strings.Contains(got, member) || strings.Contains(got, escaped) {
				t.Errorf("Scrub(%s) = %s holds the member", tc.in, got)
			}
		})
	}
}

func TestScrubSensitiveFields(t *testing.T) {
	body := []byte(`{"name":"n","proxy_value":"v","other":"o"}`)
	for name, tc := range map[string]struct {
		fields []string
		want   string
	}{
		"none": {nil, string(body)},
		"a field": {
			[]string{"Proxy-Value"}, `{"name":"n","other":"o","proxy_value":"[REDACTED]"}`,
		},
		"an empty field":  {[]string{""}, string(body)},
		"only separators": {[]string{"-_"}, string(body)},
		"a field and another": {
			[]string{"other", "absent"}, `{"name":"n","other":"[REDACTED]","proxy_value":"v"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := string(Scrub(nil, body, tc.fields...)); got != tc.want {
				t.Errorf("Scrub(%s, %q) = %s, want %s", body, tc.fields, got, tc.want)
			}
		})
	}
	text := []byte(`not json: {"proxy_value": "v"}`)
	want := `not json: {"proxy_value": "` + Marker + `"}`
	if got := string(Scrub(nil, text, "proxy_value")); got != want {
		t.Errorf("Scrub of text = %s, want %s", got, want)
	}
}

func TestScrubAMemberThatOverlapsTheMarker(t *testing.T) {
	s := NewSet("]x")
	if got := ScrubString(s, "]x]xx"); got != Marker {
		t.Errorf("ScrubString = %q, want %q: a single pass leaves ]x after the marker", got, Marker)
	}
	if got := ScrubString(s, "a ]x b"); got != "a "+Marker+" b" {
		t.Errorf("ScrubString = %q, want only the member replaced", got)
	}
}

func TestScrubReturnsACopy(t *testing.T) {
	data := []byte(`{"a":1}`)
	got := Scrub(nil, data)
	got[0] = 'X'
	if string(data) != `{"a":1}` {
		t.Errorf("changing the result of Scrub changed its input to %s", data)
	}
}

func TestIsCredential(t *testing.T) {
	s := newScrubber(nil, []string{"Mapping_Rules"})
	for key, want := range map[string]bool{
		"secret": true, "SECRET": true, "client_secret": true, "secrets": true,
		"token": true, "access-token": true, "api_key": true, "apiKey": true, "API-KEY": true,
		"password": true, "pass_word": true, "authorization": true, "Proxy-Authorization": true,
		"passwd": true, "user_pwd": true, "private_key": true, "PrivateKey": true,
		"access_key": true, "AWS_ACCESS_KEY_ID": true, "credential": true, "credentials": true,
		"secret_key": true, "cookie": true, "Set-Cookie": true,
		"mappingrules": true, "mapping-rules": true,
		"key": false, "name": false, "api": false, "pass": false, "author": false, "": false,
		"tokn": false, "secre": false, "private": false, "access": false, "cred": false,
		"cook": false, "pw": false,
	} {
		if got := s.isCredential(key); got != want {
			t.Errorf("isCredential(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"api_key": "apikey", "API-Key": "apikey", "apiKey": "apikey", "a-_-b": "ab", "": "",
		"ÄB": "äb",
	} {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

// FuzzScrub checks that Scrub never returns a member of the Set or a Permit API key, wherever
// they are in data and whatever data holds, also in the strings of a result that is JSON once
// they are decoded. secret is a second member, of any text.
func FuzzScrub(f *testing.F) {
	f.Add([]byte(`{"message":"bad key"}`), uint(15), "proxy-credential")
	f.Add([]byte(`{"secret":{"h":"v"},"detail":[{"loc":["body"],"msg":"m"}]}`), uint(3), "]x")
	f.Add([]byte("<html>502</html>"), uint(6), "x")
	f.Add([]byte(""), uint(0), "permit_key_")
	key := canary.Key("fuzz")
	f.Fuzz(func(t *testing.T, data []byte, at uint, secret string) {
		if secret == "" || strings.Contains(Marker, secret) {
			t.Skip("a secret that is empty or part of the marker cannot be told apart from it")
		}
		cut := int(at % uint(len(data)+1))
		in := string(data[:cut]) + key + string(data[cut:])
		in = strings.Replace(in, "x", secret, 1)

		out := string(Scrub(NewSet(key, secret), []byte(in)))

		for _, s := range []string{key, secret} {
			if strings.Contains(out, s) {
				t.Errorf("Scrub(%q) = %q, which holds the member %q", in, out, s)
			}
		}
		if permitKey.MatchString(out) {
			t.Errorf("Scrub(%q) = %q, which holds a Permit API key", in, out)
		}
	})
}
