package mockpermit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/testkit/faketb"
	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
)

func TestExpectJSONPassesTheSameValue(t *testing.T) {
	for _, tt := range []struct{ body, want string }{
		{`{"a":1,"b":[true,null,"x"]}`, "{ \"b\": [true, null, \"x\"],\n \"a\": 1 }"},
		{`{"key":"\u003c\u003e\u0026"}`, `{"key":"<>&"}`},
		{`{"s":"\u2028"}`, `{"s":"` + fixtures.LineSeparator + `"}`},
		// Lone surrogates all decode to U+FFFD, as ExpectJSON's doc says.
		{`{"s":` + fixtures.LoneSurrogateJSON + `}`, `{"s":"\udfff"}`},
		{`{"n":9007199254740993}`, `{"n":9007199254740993}`},
		{`{"n":-0}`, `{"n":-0}`},
		{`{"n":1e+300}`, `{"n":1e+300}`},
		{`[]`, `[ ]`},
		{`{}`, ` {} `},
		{`[{"a":{"b":[]}}]`, `[{"a":{"b":[]}}]`},
		{`"text"`, `"text"`},
		{`null`, `null`},
	} {
		ft := faketb.New(t)
		Request{Method: "POST", RawPath: "/x", Body: []byte(tt.body)}.ExpectJSON(ft, tt.want)
		if got := ft.Take(); len(got) != 0 {
			t.Errorf("ExpectJSON(%s) of the body %s failed with %q, want a pass", tt.want, tt.body,
				got)
		}
	}
}

func TestExpectJSONReportsADiff(t *testing.T) {
	ft := faketb.New(t)
	r := Request{
		Method: "POST", RawPath: "/v2/facts/p/e/users",
		Body: []byte(`{"key":"alice","email":"alice@example.com"}`),
	}

	r.ExpectJSON(ft, `{"key":"bob","email":"alice@example.com"}`)

	got := ft.Take()
	if len(got) != 1 {
		t.Fatalf("ExpectJSON failed with %q, want one failure", got)
	}
	for _, want := range []string{
		"mockpermit: POST /v2/facts/p/e/users: the body differs (-want +got):\n",
		`"bob"`, `"alice"`, "-", "+",
	} {
		if !strings.Contains(got[0], want) {
			t.Errorf("ExpectJSON failed with %q, want it to contain %q", got[0], want)
		}
	}
}

// TestExpectJSONIsExact checks values that a lenient comparison mistakes for the same.
func TestExpectJSONIsExact(t *testing.T) {
	for _, tt := range []struct{ name, body, want string }{
		{"2^53+1 rounded", `{"n":9007199254740993}`, `{"n":9007199254740992}`},
		{"int64 max rounded", `{"n":9223372036854775807}`, `{"n":9223372036854775808}`},
		{"1.0 and 1", `{"n":1.0}`, `{"n":1}`},
		{"-0 and 0", `{"n":-0}`, `{"n":0}`},
		{"exponent notation", `{"n":1e+300}`, `{"n":1e300}`},
		{"null and missing", `{"a":null}`, `{}`},
		{"missing and null", `{}`, `{"a":null}`},
		{"extra field", `{"a":1,"b":2}`, `{"a":1}`},
		{"empty string and null", `{"a":""}`, `{"a":null}`},
		{"string and number", `{"a":"1"}`, `{"a":1}`},
		{"array order", `[1,2]`, `[2,1]`},
		{"nested", `{"a":{"b":[{"c":1}]}}`, `{"a":{"b":[{"c":2}]}}`},
		{"false and null", `false`, `null`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ft := faketb.New(t)
			Request{Method: "PUT", RawPath: "/x", Body: []byte(tt.body)}.ExpectJSON(ft, tt.want)
			wantFailures(t, ft, "mockpermit: PUT /x: the body differs (-want +got):")
		})
	}
}

// TestExpectJSONRejectsInexactJSON checks bodies that are not one JSON value, or that a decoder
// into a map would accept but loses part of. encoding/json words its syntax errors differently
// from one Go release to the next, so only the mock's own reasons are checked word for word.
func TestExpectJSONRejectsInexactJSON(t *testing.T) {
	for _, tt := range []struct{ name, body, want string }{
		{"empty", ``, ""},
		{"not JSON", `{"a":}`, ""},
		{"unclosed object", `{"a":1`, ""},
		{"unclosed array", `[1,2`, ""},
		{"bad array value", `[1,}]`, ""},
		{"bad key", `{1:2}`, ""},
		{"bad value after a key", `{"a":1,"b":}`, ""},
		{"two values", `{} {}`, "it has more after the first JSON value"},
		{"trailing text", `{"a":1} x`, "it has more after the first JSON value"},
		{"repeated key", `{"a":1,"a":1}`, `an object repeats the key "a"`},
		{"repeated escaped key", `{"a":1,"\u0061":2}`, `an object repeats the key "a"`},
		{"repeated nested key", `[{"b":{"c":1,"c":2}}]`, `an object repeats the key "c"`},
		{"invalid UTF-8", "{\"a\":\"\xff\"}", "it is not valid UTF-8"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ft := faketb.New(t)
			Request{Method: "PATCH", RawPath: "/x", Body: []byte(tt.body)}.ExpectJSON(ft, `{}`)
			wantFailures(t, ft, "mockpermit: PATCH /x: the body is not exact JSON: "+tt.want)
			Request{Method: "PATCH", RawPath: "/x", Body: []byte(`{}`)}.ExpectJSON(ft, tt.body)
			wantFailures(t, ft, "mockpermit: ExpectJSON: want is not exact JSON: "+tt.want)
		})
	}
}

func TestExpectBytes(t *testing.T) {
	r := Request{Method: "POST", RawPath: "/v2/x", Body: []byte(`{"key":"a\u003cb"}`)}

	ft := faketb.New(t)
	r.ExpectBytes(ft, filepath.Join("testdata", "escaped.golden"))
	wantFailures(t, ft)

	unescaped := Request{Method: "POST", RawPath: "/v2/x", Body: []byte(`{"key":"a<b"}`)}
	unescaped.ExpectBytes(ft, filepath.Join("testdata", "escaped.golden"))
	wantFailures(t, ft, "mockpermit: POST /v2/x: the body differs from "+
		filepath.Join("testdata", "escaped.golden")+" (-want +got):\n")
}

func TestExpectBytesFinalNewline(t *testing.T) {
	dir := t.TempDir()
	for _, tt := range []struct {
		name, content, body, want string
	}{
		{"no final newline", "{}", "{}", "does not end with a newline; a golden file holds " +
			"the body and then one newline"},
		{"empty file", "", "", "does not end with a newline"},
		{"two final newlines", "{}\n\n", "{}", "the body differs from"},
		{"a body that ends in a newline", "{}\n\n", "{}\n", ""},
		{"an empty body", "\n", "", ""},
		{"a body with lines", "{\n  \"a\": 1\n}\n", "{\n  \"a\": 1\n}", ""},
		{"CRLF", "{}\r\n", "{}", "the body differs from"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			golden := filepath.Join(dir, strings.ReplaceAll(tt.name, " ", "_")+".golden")
			if err := os.WriteFile(golden, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			ft := faketb.New(t)
			Request{Method: "POST", RawPath: "/x", Body: []byte(tt.body)}.ExpectBytes(ft, golden)
			if tt.want == "" {
				wantFailures(t, ft)
			} else {
				wantFailures(t, ft, tt.want)
			}
		})
	}
}

func TestExpectBytesReportsAMissingFile(t *testing.T) {
	ft := faketb.New(t)
	missing := filepath.Join(t.TempDir(), "missing.golden")
	Request{Method: "POST", RawPath: "/x"}.ExpectBytes(ft, missing)
	wantFailures(t, ft, "mockpermit: ExpectBytes: open "+missing+": no such file or directory")
}
