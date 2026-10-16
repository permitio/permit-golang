package permit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/redact"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
)

// getUser is the call of the tests: the request that gets a user.
var getUser = call{
	op:     "get_user",
	method: http.MethodGet,
	route:  "/v2/facts/{proj_id}/{env_id}/users/{user_id}",
}

// getUserText is how Error writes getUser.
const getUserText = "permit: get_user: GET /v2/facts/{proj_id}/{env_id}/users/{user_id}"

// sentinels are the sentinel errors, each with the status whose Error matches it, or 0.
var sentinels = []struct {
	err    error
	status int
}{
	{ErrNotFound, http.StatusNotFound},
	{ErrConflict, http.StatusConflict},
	{ErrUnauthorized, http.StatusUnauthorized},
	{ErrForbidden, http.StatusForbidden},
	{ErrRateLimited, http.StatusTooManyRequests},
	{ErrContainerPDPOnly, 0},
	{ErrInvalidConfig, 0},
	{ErrMalformedDecision, 0},
	{ErrClosed, 0},
}

// bodyOnly is in each body of errorBodies where the SDK reads nothing, so Error must never hold
// it.
const bodyOnly = "body-only-text"

// errorBodies are the shapes of error bodies, each with the Error fields that the SDK reads from
// it.
var errorBodies = []struct {
	name  string
	body  string
	error Error
}{
	{
		name: "ErrorDetails",
		body: `{"id":"8f2c","title":"Not Found","error_code":"NOT_FOUND","message":"no user u",` +
			`"support_link":"https://example.com/` + bodyOnly + `"}`,
		error: Error{Code: "NOT_FOUND", Title: "Not Found", Detail: "no user u", RequestID: "8f2c"},
	},
	{
		name:  "a detail string",
		body:  `{"detail":"Not Found","extra":"` + bodyOnly + `"}`,
		error: Error{Detail: "Not Found"},
	},
	{
		name: "a validation error",
		body: `{"detail":[{"loc":["body","email"],"msg":"value is not a valid email address",` +
			`"type":"value_error.email","input":"` + bodyOnly + `"}]}`,
		error: Error{Detail: "body.email: value is not a valid email address"},
	},
	{name: "text", body: "upstream connect error " + bodyOnly},
	{name: "empty", body: ""},
	{name: "HTML", body: "<html><body><h1>Bad Gateway</h1>" + bodyOnly + "</body></html>"},
}

func TestReplyErrorStatusesAndBodies(t *testing.T) {
	statuses := []int{0}
	for status := 200; status <= 599; status++ {
		statuses = append(statuses, status)
	}
	for _, shape := range errorBodies {
		for _, status := range statuses {
			e := getUser.replyError(status, []byte(shape.body), nil, nil)
			err := fmt.Errorf("load the profile: %w", e)
			where := fmt.Sprintf("status %d, %s body", status, shape.name)

			var target *Error
			if !errors.As(err, &target) || target != e {
				t.Fatalf("%s: errors.As did not find the *Error", where)
			}
			for _, s := range sentinels {
				want := s.status != 0 && s.status == status
				if got := errors.Is(err, s.err); got != want {
					t.Errorf("%s: errors.Is(err, %v) = %v, want %v", where, s.err, got, want)
				}
			}
			if text := e.Error(); strings.Contains(text, bodyOnly) {
				t.Errorf("%s: Error() = %q holds the body", where, text)
			}

			want := shape.error
			want.Op, want.Method, want.Route, want.StatusCode = getUser.op, getUser.method,
				getUser.route, status
			want.Body = []byte(shape.body)
			if status == 0 || status >= 200 && status <= 299 {
				want = Error{
					Op:         want.Op,
					Method:     want.Method,
					Route:      want.Route,
					StatusCode: status,
				}
			}
			if diff := cmp.Diff(&want, e, cmp.Comparer(bytesEqual)); diff != "" {
				t.Errorf("%s: the Error differs (-want +got):\n%s", where, diff)
			}
		}
	}
}

// bytesEqual compares two bodies, an empty one equal to nil.
func bytesEqual(a, b []byte) bool { return bytes.Equal(a, b) }

func TestErrorText(t *testing.T) {
	for name, tc := range map[string]struct {
		err  *Error
		want string
	}{
		"empty":  {&Error{}, "permit"},
		"op":     {&Error{Op: "get_user"}, "permit: get_user"},
		"method": {&Error{Method: "GET"}, "permit: GET"},
		"route":  {&Error{Route: "/v2/x"}, "permit: /v2/x"},
		"a reply": {
			&Error{
				Op: getUser.op, Method: getUser.method, Route: getUser.route, StatusCode: 404,
				Code: "NOT_FOUND", Title: "Not Found", Detail: "no user u", RequestID: "8f2c",
				Body: []byte(`{"message":"no user u"}`),
			},
			getUserText + ": status 404 NOT_FOUND: no user u",
		},
		"the title without a detail": {
			&Error{StatusCode: 404, Title: "Not Found"}, "permit: status 404: Not Found",
		},
		"a code without a status": {&Error{Code: "NOT_FOUND"}, "permit: NOT_FOUND"},
		"an op and a code":        {&Error{Op: "get_user", Code: "C"}, "permit: get_user: C"},
		"a nil *Error":            {nil, "permit: <nil>"},
		"a cause": {
			&Error{Op: "get_user", Err: context.Canceled}, "permit: get_user: context canceled",
		},
		"everything": {
			&Error{
				Op: "o", Method: "M", Route: "/r", StatusCode: 500, Code: "C", Title: "T",
				Detail: "D", Err: errors.New("cause"),
			},
			"permit: o: M /r: status 500 C: D: cause",
		},
		"a body alone": {&Error{Body: []byte("the body")}, "permit"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestErrorIsChecksTheCauseToo(t *testing.T) {
	e := &Error{StatusCode: http.StatusConflict, Err: ErrContainerPDPOnly}
	if !errors.Is(e, ErrContainerPDPOnly) || !errors.Is(e, ErrConflict) {
		t.Errorf("errors.Is finds ErrContainerPDPOnly %v and ErrConflict %v, want both",
			errors.Is(e, ErrContainerPDPOnly), errors.Is(e, ErrConflict))
	}
	if !errors.Is(e.Unwrap(), ErrContainerPDPOnly) {
		t.Errorf("Unwrap() = %v, want Err", e.Unwrap())
	}
	if e.Is(ErrContainerPDPOnly) || e.Is(errors.New("permit: not found")) || e.Is(nil) {
		t.Error("Is matches a target that is not the sentinel of the status")
	}
	if (&Error{}).Unwrap() != nil {
		t.Error("Unwrap() of an Error without a cause is not nil")
	}
}

func TestContainerPDPOnlyIsNotNotFound(t *testing.T) {
	for name, cause := range map[string]error{
		"the sentinel":       ErrContainerPDPOnly,
		"a wrapped sentinel": fmt.Errorf("route /v2/x: %w", ErrContainerPDPOnly),
	} {
		e := &Error{StatusCode: http.StatusNotFound, Err: cause}
		if !errors.Is(e, ErrContainerPDPOnly) || errors.Is(e, ErrNotFound) {
			t.Errorf("%s: errors.Is finds ErrContainerPDPOnly %v and ErrNotFound %v, want only "+
				"ErrContainerPDPOnly", name, errors.Is(e, ErrContainerPDPOnly),
				errors.Is(e, ErrNotFound))
		}
	}
	otherCause := &Error{StatusCode: http.StatusNotFound, Err: context.Canceled}
	if !errors.Is(otherCause, ErrNotFound) {
		t.Error("errors.Is(404 with another cause, ErrNotFound) is false")
	}
}

func TestNilError(t *testing.T) {
	var e *Error
	var err error = e
	if got := e.Error(); got != "permit: <nil>" {
		t.Errorf("Error() of a nil *Error = %q, want %q", got, "permit: <nil>")
	}
	if e.Unwrap() != nil {
		t.Errorf("Unwrap() of a nil *Error = %v, want nil", e.Unwrap())
	}
	wrapped := fmt.Errorf("load the profile: %w", err)
	for _, s := range sentinels {
		if errors.Is(wrapped, s.err) || e.Is(s.err) {
			t.Errorf("errors.Is(a nil *Error, %v) is true", s.err)
		}
	}
	if target, ok := errors.AsType[*Error](wrapped); !ok || target != nil {
		t.Errorf("errors.AsType = %v, %v; want the nil *Error and true", target, ok)
	}
}

func TestSentinelsAreDistinct(t *testing.T) {
	texts := map[string]bool{}
	for i, s := range sentinels {
		text := s.err.Error()
		if texts[text] || !strings.HasPrefix(text, "permit: ") {
			t.Errorf("the sentinel %q repeats a text or does not start with \"permit: \"", text)
		}
		texts[text] = true
		for j, other := range sentinels {
			if i != j && errors.Is(s.err, other.err) {
				t.Errorf("errors.Is(%v, %v) is true", s.err, other.err)
			}
		}
	}
}

func TestReplyErrorRedacts(t *testing.T) {
	key := canary.Key("client")
	other := canary.Key("otherclient")
	// escapedKey is key with its first letter written as a JSON escape, built so that no editor
	// decodes it.
	escapedKey := string(rune(92)) + "u0070" + key[1:]
	const proxyCredential = "proxy-header-value"
	secrets := redact.NewSet(key)
	secrets.Add(proxyCredential)
	for name, tc := range map[string]struct {
		status int
		body   string
		// wantBody is the Body the Error must keep.
		wantBody string
		// wantDetail is the Detail the Error must have.
		wantDetail string
	}{
		"the key in a 401 message": {
			status: http.StatusUnauthorized,
			body: `{"id":"` + key + `","title":"Unauthorized ` + key + `","error_code":"` + key +
				`","message":"the API key ` + key + ` is not valid"}`,
			wantBody: `{"error_code":"[REDACTED]","id":"[REDACTED]","message":"the API key ` +
				`[REDACTED] is not valid","title":"Unauthorized [REDACTED]"}`,
			wantDetail: "the API key [REDACTED] is not valid",
		},
		"another Permit API key in a 401 detail": {
			status:     http.StatusUnauthorized,
			body:       `{"detail":"invalid key ` + other + `"}`,
			wantBody:   `{"detail":"invalid key [REDACTED]"}`,
			wantDetail: "invalid key [REDACTED]",
		},
		"a secret field in a 422 body": {
			status: http.StatusUnprocessableEntity,
			body: `{"detail":[{"loc":["body","secret"],"msg":"bad value ` + proxyCredential +
				`","input":{"secret":"s3cr3t-value","headers":{"Authorization":"Bearer abc"}}}]}`,
			wantBody: `{"detail":[{"input":{"headers":{"Authorization":"[REDACTED]"},` +
				`"secret":"[REDACTED]"},"loc":["body","secret"],"msg":"bad value [REDACTED]"}]}`,
			wantDetail: "body.secret: bad value [REDACTED]",
		},
		"the key escaped in a 422 message": {
			status:     http.StatusUnprocessableEntity,
			body:       `{"message":"` + escapedKey + `"}`,
			wantBody:   `{"message":"[REDACTED]"}`,
			wantDetail: "[REDACTED]",
		},
		"the key escaped as an object key": {
			status:     http.StatusUnprocessableEntity,
			body:       `{"detail":"d","` + escapedKey + `":1}`,
			wantBody:   `{"detail":"d","[REDACTED]":1}`,
			wantDetail: "d",
		},
		"a pair in a 422 detail string": {
			status:     http.StatusUnprocessableEntity,
			body:       `{"detail":"bad body {\"secret\":\"s3cr3t-value\"}"}`,
			wantBody:   `{"detail":"bad body {\"secret\":\"[REDACTED]\"}"}`,
			wantDetail: `bad body {"secret":"[REDACTED]"}`,
		},
		"the input of a 422 credential field": {
			status: http.StatusUnprocessableEntity,
			body: `{"detail":[{"loc":["body","api_key"],"msg":"bad",` +
				`"input":"s3cr3t-value"}]}`,
			wantBody: `{"detail":[{"input":"[REDACTED]","loc":["body","api_key"],` +
				`"msg":"bad"}]}`,
			wantDetail: "body.api_key: bad",
		},
		"the key in a text body": {
			status:   http.StatusBadGateway,
			body:     "proxy said Authorization: Bearer " + key,
			wantBody: "proxy said Authorization: [REDACTED]",
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := getUser.replyError(tc.status, []byte(tc.body), secrets, nil)

			scanner := canary.NewScanner(key, other, escapedKey, proxyCredential, "s3cr3t-value",
				"Bearer abc")
			for _, text := range []string{
				e.Error(), fmt.Sprintf("%+v", e), fmt.Sprintf("%#v", e), string(e.Body), e.Code,
				e.Title, e.Detail, e.RequestID,
			} {
				_, _ = scanner.Write([]byte(text + "\n"))
			}
			scanner.Check(t)
			if string(e.Body) != tc.wantBody {
				t.Errorf("Body = %s, want %s", e.Body, tc.wantBody)
			}
			if e.Detail != tc.wantDetail {
				t.Errorf("Detail = %q, want %q", e.Detail, tc.wantDetail)
			}
		})
	}
}

func TestReplyErrorScrubsSensitiveFields(t *testing.T) {
	body := []byte(`{"detail":[{"loc":["body"],"msg":"m","input":{"mapping_value":"v1"}}]}`)

	e := getUser.replyError(http.StatusUnprocessableEntity, body, nil, []string{"mapping_value"})

	want := `{"detail":[{"input":{"mapping_value":"[REDACTED]"},"loc":["body"],"msg":"m"}]}`
	if string(e.Body) != want {
		t.Errorf("Body = %s, want %s", e.Body, want)
	}
	e = getUser.replyError(http.StatusUnprocessableEntity, body, nil, nil)
	if !bytes.Equal(e.Body, body) {
		t.Errorf("Body without sensitive fields = %s, want the body unchanged", e.Body)
	}
}

func TestReplyErrorCutsTheBodyAfterRedaction(t *testing.T) {
	key := canary.Key("cut")
	secrets := redact.NewSet(key)
	a := func(n int) string { return strings.Repeat("a", n) }
	for name, tc := range map[string]struct {
		body string
		want string
	}{
		"under the cap":    {a(maxErrorBody - 1), a(maxErrorBody - 1)},
		"at the cap":       {a(maxErrorBody), a(maxErrorBody)},
		"over the cap":     {a(maxErrorBody + 1), a(maxErrorBody)},
		"far over the cap": {a(10 * maxErrorBody), a(maxErrorBody)},
		// Cut first, the body would end with the start of the key, which no pattern finds.
		"a key across the cut": {
			strings.Repeat("a", maxErrorBody-7) + key,
			strings.Repeat("a", maxErrorBody-7) + "[REDACT",
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := getUser.replyError(http.StatusInternalServerError, []byte(tc.body), secrets, nil)
			if string(e.Body) != tc.want {
				t.Errorf("Body has %d bytes ending %q, want %d ending %q", len(e.Body),
					tail(string(e.Body)), len(tc.want), tail(tc.want))
			}
			if cap(e.Body) > maxErrorBody {
				t.Errorf("cap(Body) = %d, want at most %d: the Error keeps only what it shows",
					cap(e.Body), maxErrorBody)
			}
		})
	}
	if maxErrorBody != 8192 {
		t.Errorf("maxErrorBody = %d, want 8 KiB", maxErrorBody)
	}
}

func TestReplyErrorCutsTheFields(t *testing.T) {
	const proxyCredential = "proxy-header-value"
	secrets := new(redact.Set)
	secrets.Add(proxyCredential)
	a := func(n int) string { return strings.Repeat("a", n) }
	for name, tc := range map[string]struct{ text, want string }{
		"at the cap":   {a(maxErrorField), a(maxErrorField)},
		"over the cap": {a(maxErrorField + 1), a(maxErrorField-3) + "..."},
		// Each é has 2 bytes, so the cut at 1021 bytes falls inside one.
		"a character across the cut": {
			strings.Repeat("é", maxErrorField), strings.Repeat("é", 510) + "...",
		},
		// Cut first, the text would end with the start of the credential, which no pattern finds.
		"a credential across the cut": {
			a(maxErrorField-9) + proxyCredential, a(maxErrorField-9) + "[REDAC...",
		},
	} {
		t.Run(name, func(t *testing.T) {
			body, err := json.Marshal(map[string]string{
				"id": tc.text, "title": tc.text, "error_code": tc.text, "message": tc.text,
			})
			if err != nil {
				t.Fatal(err)
			}

			e := getUser.replyError(http.StatusBadRequest, body, secrets, nil)

			for field, got := range map[string]string{
				"Code": e.Code, "Title": e.Title, "Detail": e.Detail, "RequestID": e.RequestID,
			} {
				if got != tc.want {
					t.Errorf("%s has %d bytes ending %q, want %d ending %q", field, len(got),
						tail(got), len(tc.want), tail(tc.want))
				}
			}
		})
	}
	if maxErrorField != 1024 || cutMarker != "..." {
		t.Errorf("maxErrorField = %d and cutMarker = %q, want 1 KiB and ...", maxErrorField,
			cutMarker)
	}
}

func TestReplyErrorListsTenValidationErrors(t *testing.T) {
	var entries []string
	for i := range 30 {
		entries = append(entries, fmt.Sprintf(`{"loc":["body","field%d"],"msg":"is not valid"}`, i))
	}
	body := `{"detail":[` + strings.Join(entries, ",") + `]}`

	e := getUser.replyError(http.StatusUnprocessableEntity, []byte(body), nil, nil)

	if !strings.HasPrefix(e.Detail, "body.field0: is not valid; body.field1: is not valid; ") ||
		!strings.HasSuffix(e.Detail, "; body.field9: is not valid; and 20 more") {
		t.Errorf("Detail = %q, want the first 10 fields and how many more failed", e.Detail)
	}
}

// tail returns the last 16 bytes of s.
func tail(s string) string { return s[max(0, len(s)-16):] }

func TestDecodeError(t *testing.T) {
	e := getUser.decodeError(http.StatusOK, "models.UserRead")

	want := getUserText + ": status 200: the reply is not a valid models.UserRead"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if e.Body != nil || e.Detail != "" || e.StatusCode != http.StatusOK {
		t.Errorf("the Error keeps Body %q, Detail %q and status %d; want no body, no detail and "+
			"200", e.Body, e.Detail, e.StatusCode)
	}
	for _, s := range sentinels {
		if errors.Is(e, s.err) {
			t.Errorf("errors.Is(decode error, %v) is true", s.err)
		}
	}
}

func TestTransportError(t *testing.T) {
	key := canary.Key("transport")
	const proxyCredential = "proxy-header-value"
	secrets := redact.NewSet(key)
	secrets.Add(proxyCredential)
	const objectKey = "alice@example.com"
	urlErr := &url.Error{
		Op:  "Get",
		URL: "https://api.example.com/v2/facts/p/e/users/" + objectKey,
		Err: context.DeadlineExceeded,
	}
	opErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	for name, tc := range map[string]struct {
		cause    error
		wantText string
		wantIs   error
	}{
		"a deadline": {
			urlErr, getUserText + ": context deadline exceeded", context.DeadlineExceeded,
		},
		"a wrapped *url.Error": {
			fmt.Errorf("attempt 2: %w", urlErr), getUserText + ": context deadline exceeded",
			context.DeadlineExceeded,
		},
		"a refused connection": {
			&url.Error{Op: "Get", URL: "http://127.0.0.1:1/v2/users/" + objectKey, Err: opErr},
			getUserText + ": dial tcp: connection refused", opErr,
		},
		"a canceled context": {
			context.Canceled, getUserText + ": context canceled", context.Canceled,
		},
		"a cause with credentials": {
			errors.New("proxy rejected Authorization: Bearer " + key + " and " + proxyCredential),
			getUserText + ": proxy rejected Authorization: [REDACTED] and [REDACTED]", nil,
		},
		"a *url.Error without a cause": {
			&url.Error{Op: "Get", URL: "/v2/users/" + objectKey}, getUserText + ": Get", nil,
		},
		"a *url.Error without a cause or an Op": {
			&url.Error{URL: "/v2/users/" + objectKey}, getUserText + ": the request failed", nil,
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := getUser.transportError(tc.cause, secrets)
			err := fmt.Errorf("sync: %w", e)

			if got := e.Error(); got != tc.wantText {
				t.Errorf("Error() = %q, want %q", got, tc.wantText)
			}
			if e.StatusCode != 0 || e.Body != nil {
				t.Errorf("StatusCode = %d and Body = %q, want 0 and none", e.StatusCode, e.Body)
			}
			if !errors.Is(err, tc.cause) {
				t.Errorf("errors.Is(err, cause) is false")
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Errorf("errors.Is(err, %v) is false", tc.wantIs)
			}
			for _, s := range sentinels {
				if errors.Is(err, s.err) {
					t.Errorf("errors.Is(err, %v) is true for a request without a reply", s.err)
				}
			}
			scanner := canary.NewScanner(key, proxyCredential, objectKey)
			_, _ = scanner.Write([]byte(e.Error() + fmt.Sprint(e.Unwrap())))
			scanner.Check(t)
		})
	}
	if _, ok := errors.AsType[*url.Error](getUser.transportError(urlErr, nil)); !ok {
		t.Error("errors.As does not find the *url.Error")
	}
}

func TestTransportErrorWithoutACause(t *testing.T) {
	e := getUser.transportError(nil, nil)
	if e.Err != nil || e.Error() != getUserText {
		t.Errorf("Err = %v and Error() = %q, want no cause and %q", e.Err, e.Error(), getUserText)
	}
}

// TestConstructorsTakeZeroValues checks that the constructors accept the zero value of every
// argument, as a transport that read nothing passes them.
func TestConstructorsTakeZeroValues(t *testing.T) {
	var c call
	for _, e := range []*Error{
		c.replyError(0, nil, nil, nil),
		c.replyError(http.StatusInternalServerError, nil, nil, nil),
		c.replyError(-1, nil, nil, []string{""}),
		c.replyError(1000, []byte("x"), nil, nil),
		c.decodeError(0, ""),
		c.transportError(nil, nil),
	} {
		if text := e.Error(); !strings.HasPrefix(text, "permit") {
			t.Errorf("Error() = %q, want it to start with permit", text)
		}
	}
}

// permitKey matches a Permit API key, as the stateless pattern of redact does.
var permitKey = regexp.MustCompile(`permit_key_[A-Za-z0-9_-]+`)

// FuzzError checks that an Error never holds the client's key or a Permit API key, in its text,
// its fields or its body, wherever the key is in the reply's body and whatever the body holds.
func FuzzError(f *testing.F) {
	f.Add(uint16(401), []byte(`{"message":"bad key ","title":"t"}`), uint(19))
	f.Add(uint16(422), []byte(`{"detail":[{"loc":["body",""],"msg":"m"}]}`), uint(23))
	f.Add(uint16(404), []byte(`{"detail":""}`), uint(11))
	f.Add(uint16(502), []byte("<html></html>"), uint(6))
	key := canary.Key("fuzzerror")
	secrets := redact.NewSet(key)
	f.Fuzz(func(t *testing.T, status uint16, body []byte, at uint) {
		cut := int(at % uint(len(body)+1))
		in := slices.Concat(body[:cut], []byte(key), body[cut:])

		e := getUser.replyError(int(status%600), in, secrets, nil)

		for name, text := range map[string]string{
			"Error()": e.Error(), "Body": string(e.Body), "Code": e.Code, "Title": e.Title,
			"Detail": e.Detail, "RequestID": e.RequestID,
		} {
			if strings.Contains(text, key) || permitKey.MatchString(text) {
				t.Errorf("status %d, body %q: %s = %q holds a key", status%600, in, name, text)
			}
		}
		if len(e.Body) > maxErrorBody {
			t.Errorf("Body has %d bytes, more than %d", len(e.Body), maxErrorBody)
		}
		if e.StatusCode != int(status%600) {
			t.Errorf("StatusCode = %d, want %d", e.StatusCode, status%600)
		}
	})
}
