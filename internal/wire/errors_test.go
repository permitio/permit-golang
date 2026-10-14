package wire

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDecodeErrorBody(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want ErrorBody
	}{
		"ErrorDetails": {
			`{"id":"8f2c","title":"Not Found","error_code":"NOT_FOUND","message":"no user u",` +
				`"support_link":"https://example.com/support","additional_info":{"a":1}}`,
			ErrorBody{ID: "8f2c", Title: "Not Found", ErrorCode: "NOT_FOUND", Message: "no user u"},
		},
		"a detail string": {
			`{"detail":"Not Found"}`, ErrorBody{Detail: "Not Found"},
		},
		"a validation error": {
			`{"detail":[{"loc":["body","email"],"msg":"value is not a valid email address",` +
				`"type":"value_error.email","input":"x"},{"loc":["query","page",0],"msg":"m2",` +
				`"type":"t2"}]}`,
			ErrorBody{Validation: []ValidationError{
				{
					Loc: raw(`"body"`, `"email"`), Msg: "value is not a valid email address",
					Type: "value_error.email",
				},
				{Loc: raw(`"query"`, `"page"`, `0`), Msg: "m2", Type: "t2"},
			}},
		},
		"entries that are not validation errors with a message": {
			`{"detail":[1,"s",{"loc":"body","msg":"m"},{"msg":7},{"loc":[],"msg":"kept"},null,` +
				`{"loc":["body"],"msg":""},{"loc":["body"]},{}]}`,
			ErrorBody{Validation: []ValidationError{{Loc: []json.RawMessage{}, Msg: "kept"}}},
		},
		"fields of other types": {
			`{"id":1,"title":["t"],"error_code":{"c":1},"message":true,"detail":{"d":1}}`,
			ErrorBody{},
		},
		"null fields":    {`{"id":null,"message":null,"detail":null}`, ErrorBody{}},
		"unknown fields": {`{"code":403,"msg":"x","other":[1]}`, ErrorBody{}},
		"an empty object": {
			`{}`, ErrorBody{},
		},
		"a PDP error body": {
			`{"code":500,"message":"internal error"}`, ErrorBody{Message: "internal error"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := DecodeErrorBody([]byte(tc.body))
			if !ok {
				t.Fatalf("DecodeErrorBody(%s) = false, want true", tc.body)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("DecodeErrorBody(%s) differs (-want +got):\n%s", tc.body, diff)
			}
		})
	}
}

func TestDecodeErrorBodyRejectsOtherBodies(t *testing.T) {
	for _, body := range []string{
		"", " ", "null", "[]", `"text"`, "42", "true", "Bad Gateway", `{"detail":"x"`,
		"<html><body><h1>502 Bad Gateway</h1></body></html>", `{"a":1} {"b":2}`,
	} {
		if got, ok := DecodeErrorBody([]byte(body)); ok {
			t.Errorf("DecodeErrorBody(%q) = %+v, true; want false", body, got)
		}
	}
}

func TestSummary(t *testing.T) {
	validation := []ValidationError{
		{Loc: raw(`"body"`, `"email"`), Msg: "value is not a valid email address"},
		{Loc: raw(`"body"`, `0`, `"key"`), Msg: "field required"},
	}
	for name, tc := range map[string]struct {
		body ErrorBody
		want string
	}{
		"the message first": {
			ErrorBody{Message: "m", Detail: "d", Validation: validation, Title: "t"}, "m",
		},
		"then the detail": {ErrorBody{Detail: "d", Validation: validation}, "d"},
		"then the validation errors": {
			ErrorBody{Validation: validation},
			"body.email: value is not a valid email address; body.0.key: field required",
		},
		"nothing": {ErrorBody{Title: "t", ID: "i", ErrorCode: "c"}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := tc.body.Summary(); got != tc.want {
				t.Errorf("Summary() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSummaryListsTenValidationErrors(t *testing.T) {
	var validation []ValidationError
	var listed []string
	for i := range 13 {
		field := "f" + strconv.Itoa(i)
		loc := raw(`"body"`, strconv.Quote(field))
		validation = append(validation, ValidationError{Loc: loc, Msg: "m"})
		if i < 10 {
			listed = append(listed, "body."+field+": m")
		}
	}
	for n, want := range map[int]string{
		10: strings.Join(listed, "; "),
		11: strings.Join(listed, "; ") + "; and 1 more",
		13: strings.Join(listed, "; ") + "; and 3 more",
	} {
		if got := (ErrorBody{Validation: validation[:n]}).Summary(); got != want {
			t.Errorf("Summary() of %d validation errors = %q, want %q", n, got, want)
		}
	}
}

func TestLocation(t *testing.T) {
	for want, loc := range map[string][]json.RawMessage{
		"body.email":    raw(`"body"`, `"email"`),
		"query.page.0":  raw(`"query"`, `"page"`, `0`),
		"body..x":       raw(`"body"`, `""`, `"x"`),
		"a.b":           raw(`"a.b"`),
		"null.true.1.5": raw(`null`, `true`, `1.5`),
		"":              nil,
	} {
		if got := (ValidationError{Loc: loc}).Location(); got != want {
			t.Errorf("Location of %s = %q, want %q", loc, got, want)
		}
	}
}

// raw returns values as JSON texts.
func raw(values ...string) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(values))
	for _, v := range values {
		out = append(out, json.RawMessage(v))
	}
	return out
}
