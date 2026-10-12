package redact_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/internal/redact"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeKey is a configured API key long enough to be replaced wherever it appears.
const fakeKey = "fake-api-key-0123456789"

// jsonEscaped returns s with each of the characters in chars written as a six-character JSON
// unicode escape, which a JSON decoder reads as the character itself.
func jsonEscaped(s string, chars string) string {
	var escaped strings.Builder
	for _, r := range s {
		if strings.ContainsRune(chars, r) {
			fmt.Fprintf(&escaped, "%cu%04x", '\\', r)
			continue
		}
		escaped.WriteRune(r)
	}
	return escaped.String()
}

func TestCredentialNamesMatchBySuffixIgnoringCaseAndSeparators(t *testing.T) {
	credentials := []string{
		"secret", "SECRET", "client_secret", "token", "Access-Token", "session_token", "api_key",
		"apikey", "X-API-Key", "password", "user_password", "passwd", "pwd", "db_pwd",
		"Authorization", "Proxy-Authorization", "secret_key", "secretkey", "private_key",
		"privatekey", "access_key", "accesskey", "aws_secret_access_key", "credential",
		"credentials", "client_credentials", "cookie", "Cookie", "Set-Cookie",
	}
	others := []string{
		"", "key", "detail", "token_type", "secret_name", "tokens", "auth", "password_set",
		"api_key_id", "X-Wait-Timeout", "resource_key", "tenant_key", "public_key",
		"private_key_id", "credential_type", "cookies_enabled",
	}
	for _, name := range credentials {
		assert.True(t, redact.IsCredentialName(name), name)
	}
	for _, name := range others {
		assert.False(t, redact.IsCredentialName(name), name)
	}
}

func TestSecretShowsOnlyWhetherACredentialIsSet(t *testing.T) {
	assert.Equal(t, "", redact.Secret(""))
	assert.Equal(t, redact.Marker, redact.Secret("k"))
}

func TestHeadersReplacesCredentialHeaderValuesInACopy(t *testing.T) {
	headers := map[string]string{"Authorization": "Bearer k", "x-api-key": "k", "X-Token": "",
		"Cookie": "session=s", "X-Wait-Timeout": "1.5"}

	redacted := redact.Headers(headers)

	assert.Equal(t, map[string]string{"Authorization": redact.Marker, "x-api-key": redact.Marker,
		"X-Token": "", "Cookie": redact.Marker, "X-Wait-Timeout": "1.5"}, redacted)
	assert.Equal(t, "Bearer k", headers["Authorization"], "the map passed in is not changed")
	assert.Nil(t, redact.Headers(nil))
	assert.Equal(t, map[string]string{}, redact.Headers(map[string]string{}))
}

func TestTextReplacesTheKeyAndPermitKeys(t *testing.T) {
	tests := []struct {
		name string
		text string
		key  string
		want string
	}{
		{name: "the key wherever it appears", text: "a " + fakeKey + " b " + fakeKey,
			key: fakeKey, want: "a [REDACTED] b [REDACTED]"},
		{name: "permit_key_ values", text: "x permit_key_FAKEab9-_z, y",
			want: "x [REDACTED], y"},
		{name: "permit_key_ with nothing after it is kept", text: "permit_key_ prefix",
			want: "permit_key_ prefix"},
		{name: "an empty key is ignored", text: "abc", key: "", want: "abc"},
		{name: "a key shorter than 16 characters is not replaced as text",
			text: "kk akka k permit_key_FAKEshort", key: "k",
			want: "kk akka k [REDACTED]"},
		{name: "a key of 16 characters is replaced", text: "x 0123456789abcdef y",
			key: "0123456789abcdef", want: "x [REDACTED] y"},
		{name: "empty text", text: "", key: fakeKey, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, redact.Text(tt.text, tt.key))
		})
	}
}

func TestBodyReplacesCredentials(t *testing.T) {
	require.Len(t, jsonEscaped("a-b", "-"), 8, "jsonEscaped writes six characters for '-'")
	tests := []struct {
		name string
		body string
		key  string
		want string
	}{
		{
			name: "values under credential keys at any depth",
			body: `{"z":1,"secret":"s1","auth":{"Password":"p1","nested":[{"x_token":"t1"}]},` +
				`"api_key":12345,"headers":{"Authorization":["Bearer t2",7,null,true]},` +
				`"Set-Cookie":"c1","aws_secret_access_key":"a1","ok":"v"}`,
			want: `{"Set-Cookie":"[REDACTED]","api_key":"[REDACTED]",` +
				`"auth":{"Password":"[REDACTED]","nested":[{"x_token":"[REDACTED]"}]},` +
				`"aws_secret_access_key":"[REDACTED]",` +
				`"headers":{"Authorization":["[REDACTED]","[REDACTED]",null,true]},` +
				`"ok":"v","secret":"[REDACTED]","z":1}`,
		},
		{
			name: "an object under a credential key",
			body: `{"secret":{"X-Api":"v","n":{"deep":"d"}},"key":"pc"}`,
			want: `{"key":"pc","secret":{"X-Api":"[REDACTED]","n":{"deep":"[REDACTED]"}}}`,
		},
		{
			name: "a top-level array",
			body: `[{"token":"t"},{"name":"n"}]`,
			want: `[{"token":"[REDACTED]"},{"name":"n"}]`,
		},
		{
			name: "numbers and HTML characters are kept",
			body: `{"token":"t","big":9007199254740993,"html":"<a&b>"}`,
			want: `{"big":9007199254740993,"html":"<a&b>","token":"[REDACTED]"}`,
		},
		{
			name: "a body without credential keys keeps its bytes",
			body: "{ \"detail\" : \"x\",\n \"token_type\": \"bearer\", \"resource_key\": \"r\" }",
			want: "{ \"detail\" : \"x\",\n \"token_type\": \"bearer\", \"resource_key\": \"r\" }",
		},
		{
			name: "a body with an empty value under a credential key is encoded again",
			body: "{ \"detail\" : \"x\", \"empty_secret\": \"\" }",
			want: `{"detail":"x","empty_secret":""}`,
		},
		{
			name: "a duplicate credential key does not keep the earlier value",
			body: `{"token":"dup-secret-1","token":""}`,
			want: `{"token":""}`,
		},
		{
			name: "the key and permit keys in JSON",
			body: `{"detail":"bad ` + fakeKey + ` permit_key_FAKEzz9","token":"` + fakeKey + `"}`,
			key:  fakeKey,
			want: `{"detail":"bad [REDACTED] [REDACTED]","token":"[REDACTED]"}`,
		},
		{
			name: "the key in a body without credential keys keeps the other bytes",
			body: "{ \"detail\" : \"bad " + fakeKey + " permit_key_FAKEkeep\" }",
			key:  fakeKey,
			want: "{ \"detail\" : \"bad [REDACTED] [REDACTED]\" }",
		},
		{
			name: "a permit key written with JSON escapes",
			body: `{"detail":"bad ` + jsonEscaped("permit_key_FAKEescaped01", "p_") + `"}`,
			want: `{"detail":"bad [REDACTED]"}`,
		},
		{
			name: "the key written with JSON escapes",
			body: `{"detail":"bad ` + jsonEscaped(fakeKey, "-") + `"}`,
			key:  fakeKey,
			want: `{"detail":"bad [REDACTED]"}`,
		},
		{
			name: "a permit key in an object key",
			body: `{"` + jsonEscaped("permit_key_FAKEname01", "_") + `":1}`,
			want: `{"[REDACTED]":1}`,
		},
		{
			name: "not JSON",
			body: `<html>secret: s1 ` + fakeKey + ` permit_key_FAKEhtml</html>`,
			key:  fakeKey,
			want: `<html>secret: s1 [REDACTED] [REDACTED]</html>`,
		},
		{
			name: "JSON followed by text",
			body: `{"token":"t"} trailing ` + fakeKey,
			key:  fakeKey,
			want: `{"token":"[REDACTED]"} trailing [REDACTED]`,
		},
		{
			name: "two JSON documents",
			body: `{"token":"t"}{"token":"u"}`,
			want: `{"token":"[REDACTED]"}{"token":"[REDACTED]"}`,
		},
		{
			name: "JSON lines keep their layout and the documents without credentials",
			body: "{\"a\": 1}\n  {\"token\": \"t\"}\n{\"b\": 2}\n",
			want: "{\"a\": 1}\n  {\"token\":\"[REDACTED]\"}\n{\"b\": 2}\n",
		},
		{
			// JSON inside other text is not decoded, so only the configured key and permit
			// keys are found there: the limit for a body that does not start with JSON.
			name: "after text that is not JSON, only the key and permit keys are replaced",
			body: `error: {"token":"t"} permit_key_FAKEafter`,
			want: `error: {"token":"t"} [REDACTED]`,
		},
		{
			name: "a short key is not replaced as text",
			body: `{"detail":"k is a letter"}`,
			key:  "k",
			want: `{"detail":"k is a letter"}`,
		},
		{name: "empty", body: "", key: fakeKey, want: ""},
		{name: "a JSON string", body: `"permit_key_FAKEabc"`, want: `"[REDACTED]"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, redact.Body(tt.body, tt.key))
		})
	}
}

func TestBodyNestedTooDeepIsReplacedAsAWhole(t *testing.T) {
	nested := func(depth int) string {
		return strings.Repeat(`{"a":`, depth-1) + `{"token":"deep-secret-1"}` +
			strings.Repeat("}", depth-1)
	}
	brackets := func(depth int) string {
		return strings.Repeat("[", depth) + strings.Repeat("]", depth)
	}

	assert.Equal(t, redact.Marker, redact.Body(nested(1001), ""))
	assert.Equal(t, redact.Marker, redact.Body(nested(20000), ""),
		"deeper than encoding/json decodes")
	assert.Equal(t, redact.Marker, redact.Body(brackets(1001), ""))

	atLimit := redact.Body(nested(1000), "")
	assert.NotContains(t, atLimit, "deep-secret-1")
	assert.Contains(t, atLimit, `{"token":"[REDACTED]"}`)
	assert.Equal(t, brackets(1000), redact.Body(brackets(1000), ""))

	inString := `{"detail":"` + strings.Repeat("[", 2000) + `","token":"t"}`
	assert.Equal(t, `{"detail":"`+strings.Repeat("[", 2000)+`","token":"[REDACTED]"}`,
		redact.Body(inString, ""), "brackets inside strings are not nesting")
}

// directive is a value whose Format method prints the directive it was called with.
type directive struct{}

func (directive) Format(f fmt.State, verb rune) {
	_, _ = fmt.Fprint(f, redact.FormatDirective(f, verb))
}

func TestFormatDirectiveRebuildsTheCallersDirective(t *testing.T) {
	formats := []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%d", "%-12.4v", "% 05d", "%#x",
		"%.0s", "%8q"}
	for _, format := range formats {
		assert.Equal(t, format, fmt.Sprintf(format, directive{}))
	}
}
