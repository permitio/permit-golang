package permit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/redact"
	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/internal/wire"
)

// testObject is the model of the replies of the test-only operations.
type testObject struct {
	Key string `json:"key"`
	N   int    `json:"n,omitempty"`
}

// The test-only operations of the transport tests.
var (
	opGet = rest.Op{
		ID: "test_get", Method: http.MethodGet, Route: rest.TestPrefix + "objects/{object_id}",
		Idempotent: true, KeyLevel: rest.EnvironmentLevel, ResponseModel: "TestObject",
	}
	opList = rest.Op{
		ID: "test_list", Method: http.MethodGet, Route: rest.TestPrefix + "objects",
		Idempotent: true, KeyLevel: rest.EnvironmentLevel, MaxPerPage: 100,
		ResponseModel: "TestObject",
	}
	opCreate = rest.Op{
		ID: "test_create", Method: http.MethodPost, Route: rest.TestPrefix + "objects",
		KeyLevel: rest.EnvironmentLevel, RequestModel: "TestObject", ResponseModel: "TestObject",
	}
	opFire = rest.Op{
		ID: "test_fire", Method: http.MethodPost, Route: rest.TestPrefix + "fire",
		KeyLevel: rest.EnvironmentLevel,
	}
	opDeleteSelf = rest.Op{
		ID: "test_delete_self", Method: http.MethodDelete,
		Route:      rest.TestPrefix + "objects/{object_id}",
		Idempotent: true, DeleteSelf: true, KeyLevel: rest.EnvironmentLevel,
	}
	opRemove = rest.Op{
		ID: "test_remove", Method: http.MethodDelete,
		Route:      rest.TestPrefix + "objects/{object_id}/members",
		Idempotent: true, KeyLevel: rest.EnvironmentLevel,
	}
	opContainerOnly = rest.Op{
		ID: "test_container_only", Method: http.MethodPost, Route: rest.TestPrefix + "container",
		ContainerOnly: true, KeyLevel: rest.EnvironmentLevel, ResponseModel: "TestObject",
	}
	opSigning = rest.Op{
		ID: "test_signing", Method: http.MethodPost, Route: rest.TestPrefix + "signing",
		KeyLevel: rest.EnvironmentLevel, SensitiveBodyFields: []string{"signing_value"},
		ResponseModel: "TestObject",
	}
	opUnnamed = rest.Op{
		ID: "test_unnamed", Method: http.MethodGet, Route: rest.TestPrefix + "unnamed",
		Idempotent: true, KeyLevel: rest.EnvironmentLevel,
	}
)

// testOps are the test-only operations that the transport tests register.
var testOps = []rest.Op{
	opGet, opList, opCreate, opFire, opDeleteSelf, opRemove, opContainerOnly, opSigning,
	opUnnamed,
}

// route returns the route of op as mockpermit.Server.Handle takes it.
func route(op rest.Op) string { return op.Method + " " + op.Route }

// wireClient returns a test client with testOps whose URLs point at api and pdp, with
// mockpermit's API key and opts.
func wireClient(t *testing.T, api, pdp *mockpermit.Server, opts ...Option) *Client {
	t.Helper()
	clearEnv(t)
	base := []Option{
		WithAPIKey(mockpermit.APIKey), WithAPIURL(api.URL), WithPDPURL(pdp.URL),
	}
	return newTestClient(t, append(base, opts...), testOps...)
}

// urlClient returns a test client with testOps and the API key given whose URLs both point at
// url.
func urlClient(t *testing.T, key, url string, opts ...Option) *Client {
	t.Helper()
	clearEnv(t)
	base := []Option{WithAPIKey(key), WithAPIURL(url), WithPDPURL(url)}
	return newTestClient(t, append(base, opts...), testOps...)
}

// get returns the request of opGet for the object key.
func get(key string) request {
	return request{op: opGet.ID, target: apiTarget, params: []string{key}}
}

// mustError returns err as a *Error, and fails the test when it is not one.
func mustError(t *testing.T, err error) *Error {
	t.Helper()
	e, ok := errors.AsType[*Error](err)
	if !ok {
		t.Fatalf("the call returned %v (%T), want a *Error", err, err)
	}
	return e
}

// TestDoDecodesAnObject checks do on the replies in 200-299 that hold an object, and those that
// do not: an empty body, null, another shape or trailing data is a *Error that names the model,
// never a nil value without an error.
func TestDoDecodesAnObject(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want *testObject
	}{
		{"an object", `{"key":"a","n":2,"extra":true}`, &testObject{Key: "a", N: 2}},
		{"an object after white space", " \r\n\t{\"key\":\"b\"}", &testObject{Key: "b"}},
		{"an empty body", "", nil},
		{"white space", " \n", nil},
		{"null", "null", nil},
		{"an array", `[{"key":"a"}]`, nil},
		{"a string", `"a"`, nil},
		{"trailing data", `{"key":"a"} {}`, nil},
		{"a field of another type", `{"key":1}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			api.Handle(route(opGet), mockpermit.Reply{Body: tc.body})
			got, err := do[testObject](t.Context(), wireClient(t, api, pdp), get("a"))
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("do returned a value that differs (-want +got):\n%s", diff)
			}
			if tc.want != nil {
				if err != nil {
					t.Errorf("do returned %v", err)
				}
				return
			}
			e := mustError(t, err)
			want := "permit: test_get: GET /__test/objects/{object_id}: status 200: the reply is " +
				"not a valid models.TestObject"
			if e.Error() != want || e.Body != nil {
				t.Errorf("do returned %q with the body %q, want %q and no body", e, e.Body, want)
			}
		})
	}
}

// TestDoListDecodesEachShape checks doList on each list shape: the shape that the call expects
// gives the items and counts, and any other body, an empty one too, is a *Error.
func TestDoListDecodesEachShape(t *testing.T) {
	const (
		array = `[{"key":"a"},{"key":"b"}]`
		page  = `{"data":[{"key":"a"},{"key":"b"}],"total_count":7,"page_count":4}`
	)
	items := []testObject{{Key: "a"}, {Key: "b"}}
	seven, four := 7, 4
	fromArray := wire.List[testObject]{Data: items}
	fromPage := wire.List[testObject]{Data: items, TotalCount: &seven, PageCount: &four}
	for _, tc := range []struct {
		name  string
		shape listShape
		body  string
		want  *wire.List[testObject]
	}{
		{"an array as an array", arrayShape, array, &fromArray},
		{"an empty array", arrayShape, "[]", &wire.List[testObject]{Data: []testObject{}}},
		{"a page as an array", arrayShape, page, nil},
		{"a page as a page", pageShape, page, &fromPage},
		{"an array as a page", pageShape, array, nil},
		{"an object without data as a page", pageShape, `{"total_count":0}`, nil},
		{"an array as either", arrayOrPageShape, " " + array, &fromArray},
		{"a page as either", arrayOrPageShape, page, &fromPage},
		{"an empty body as an array", arrayShape, "", nil},
		{"an empty body as a page", pageShape, "", nil},
		{"an empty body as either", arrayOrPageShape, "", nil},
		{"null as either", arrayOrPageShape, "null", nil},
		{"a string as either", arrayOrPageShape, `"a"`, nil},
		{"items of another type", arrayShape, `[1]`, nil},
		{"an unknown shape", 0, array, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			api.Handle(route(opList), mockpermit.Reply{Body: tc.body})
			c := wireClient(t, api, pdp)
			got, err := doList[testObject](t.Context(), c, request{
				op:     opList.ID,
				target: apiTarget,
			}, tc.shape)
			if tc.want != nil {
				if err != nil {
					t.Fatalf("doList returned %v", err)
				}
				if diff := cmp.Diff(*tc.want, got); diff != "" {
					t.Errorf("doList returned a list that differs (-want +got):\n%s", diff)
				}
				return
			}
			e := mustError(t, err)
			want := "permit: test_list: GET /__test/objects: status 200: the reply is not a " +
				"valid list of models.TestObject"
			if e.Error() != want || got.Data != nil || got.TotalCount != nil {
				t.Errorf("doList returned %+v and %q, want no list and %q", got, e, want)
			}
		})
	}
}

// TestDecodeErrorsWithoutAModel checks the text of a decode error for an operation that names
// no model.
func TestDecodeErrorsWithoutAModel(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	api.Handle(route(opUnnamed), mockpermit.Reply{}, mockpermit.Reply{})
	c := wireClient(t, api, pdp)
	r := request{op: opUnnamed.ID, target: apiTarget}
	_, err := do[testObject](t.Context(), c, r)
	if !strings.HasSuffix(err.Error(), "the reply is not a valid JSON object") {
		t.Errorf("do returned %v, want the error to name a JSON object", err)
	}
	_, err = doList[testObject](t.Context(), c, r, arrayShape)
	if !strings.HasSuffix(err.Error(), "the reply is not a valid list of items") {
		t.Errorf("doList returned %v, want the error to name a list of items", err)
	}
}

// TestErrorReplies checks the error of a reply whose status is not in 200-299: a *Error with the
// status and what the body says, which matches the status's sentinel.
func TestErrorReplies(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	api.Handle(route(opGet), mockpermit.Reply{
		Status: http.StatusConflict,
		Body:   `{"error_code":"DUPLICATE_ENTITY","title":"Conflict","message":"exists","id":"r1"}`,
	})
	_, err := do[testObject](t.Context(), wireClient(t, api, pdp), get("a"))
	e := mustError(t, err)
	if !errors.Is(err, ErrConflict) || e.StatusCode != http.StatusConflict ||
		e.Code != "DUPLICATE_ENTITY" || e.Detail != "exists" || e.RequestID != "r1" ||
		e.Op != opGet.ID || e.Method != http.MethodGet || e.Route != opGet.Route {
		t.Errorf("do returned %#v, want the 409 with what its body says", e)
	}
}

// TestDoNoValue checks a call that reads no value: a reply in 200-299 is nil, whatever its body;
// a 404 to an operation whose path names the object it deletes is nil, as the object is gone;
// a 404 to a removal whose body names what it removes is ErrNotFound, and any other error status
// is an error whatever the operation. The client sends each request once, so that the 500 is the
// call's result.
func TestDoNoValue(t *testing.T) {
	notFound := `{"error_code":"NOT_FOUND","title":"Not Found","message":"no such object"}`
	for _, tc := range []struct {
		name  string
		op    rest.Op
		reply mockpermit.Reply
		// wantStatus is the StatusCode of the *Error that the call returns, or 0 for nil.
		wantStatus int
	}{
		{"a 204", opDeleteSelf, mockpermit.Reply{Status: 204}, 0},
		{"a 200 with a body", opFire, mockpermit.Reply{Body: `{"key":"a"}`}, 0},
		{
			"a 404 to a DeleteSelf operation", opDeleteSelf,
			mockpermit.Reply{Status: 404, Body: notFound},
			0,
		},
		{
			"a 404 without a body to a DeleteSelf operation", opDeleteSelf,
			mockpermit.Reply{Status: 404},
			0,
		},
		{
			"a 404 to a body-addressed removal", opRemove,
			mockpermit.Reply{Status: 404, Body: notFound},
			404,
		},
		{
			"a 410 to a DeleteSelf operation", opDeleteSelf,
			mockpermit.Reply{Status: 410, Body: notFound},
			410,
		},
		{
			"a 500 to a DeleteSelf operation", opDeleteSelf,
			mockpermit.Reply{Status: 500, Body: notFound},
			500,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			api.Handle(route(tc.op), tc.reply)
			r := request{op: tc.op.ID, target: apiTarget}
			if strings.Contains(tc.op.Route, "{object_id}") {
				r.params = []string{"a"}
			}
			err := wireClient(t, api, pdp, WithRetry(RetryPolicy{})).doNoValue(t.Context(), r)
			if tc.wantStatus == 0 {
				if err != nil {
					t.Errorf("doNoValue returned %v, want nil", err)
				}
				return
			}
			e := mustError(t, err)
			if e.StatusCode != tc.wantStatus ||
				errors.Is(err, ErrNotFound) != (tc.wantStatus == 404) {
				t.Errorf("doNoValue returned %v, want the %d", err, tc.wantStatus)
			}
		})
	}
}

// TestDoOfADeleteSelfOperationKeepsNotFound checks that a call that reads a value gets
// ErrNotFound for a 404 also from a DeleteSelf operation: it has no value to return for it. The
// error is the reply's, not one of decoding the reply's body as a value.
func TestDoOfADeleteSelfOperationKeepsNotFound(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	api.Handle(route(opDeleteSelf), mockpermit.Reply{
		Status: http.StatusNotFound,
		Body:   `{"error_code":"NOT_FOUND","title":"Not Found","message":"no such object"}`,
	})
	got, err := do[testObject](t.Context(), wireClient(t, api, pdp),
		request{op: opDeleteSelf.ID, target: apiTarget, params: []string{"a"}})
	if got != nil || !errors.Is(err, ErrNotFound) {
		t.Fatalf("do returned %v, %v; want ErrNotFound", got, err)
	}
	if e := mustError(t, err); e.Code != "NOT_FOUND" || e.Err != nil {
		t.Errorf("do returned %#v, want the 404 with what its body says", e)
	}
}

// TestContainerOnlyRoutes checks a 404 to an operation that only the PDP container serves: with
// an empty body, as the cloud PDP answers, it matches ErrContainerPDPOnly, not ErrNotFound, and
// names the route; with a body, it is a 404 of the container.
func TestContainerOnlyRoutes(t *testing.T) {
	for _, tc := range []struct {
		name          string
		op            rest.Op
		body          string
		wantContainer bool
	}{
		{"an empty body", opContainerOnly, "", true},
		{"white space", opContainerOnly, " \n", true},
		{"a body", opContainerOnly, `{"detail":"Not Found"}`, false},
		{"an operation that the cloud PDP serves", opCreate, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			pdp.Handle(route(tc.op), mockpermit.Reply{Status: http.StatusNotFound, Body: tc.body})
			_, err := do[testObject](t.Context(), wireClient(t, api, pdp),
				request{op: tc.op.ID, target: pdpTarget, body: testObject{Key: "a"}})
			api.AssertUntouched()
			e := mustError(t, err)
			if got := errors.Is(err, ErrContainerPDPOnly); got != tc.wantContainer {
				t.Errorf("errors.Is(err, ErrContainerPDPOnly) = %t, want %t: %v", got,
					tc.wantContainer, err)
			}
			if got := errors.Is(err, ErrNotFound); got == tc.wantContainer {
				t.Errorf("errors.Is(err, ErrNotFound) = %t, want %t: %v", got,
					!tc.wantContainer, err)
			}
			want := "permit: test_container_only: POST /__test/container: status 404: permit: " +
				"only the PDP container serves this route"
			if tc.wantContainer && e.Error() != want {
				t.Errorf("Error() = %q, want %q", e.Error(), want)
			}
		})
	}
}

// TestCallsGoToTheirTarget checks that a call reaches the server its request names, and not the
// other one.
func TestCallsGoToTheirTarget(t *testing.T) {
	for _, tc := range []struct {
		target target
		to     func(api, pdp *mockpermit.Server) (*mockpermit.Server, *mockpermit.Server)
	}{
		{apiTarget, func(api, pdp *mockpermit.Server) (*mockpermit.Server, *mockpermit.Server) {
			return api, pdp
		}},
		{pdpTarget, func(api, pdp *mockpermit.Server) (*mockpermit.Server, *mockpermit.Server) {
			return pdp, api
		}},
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		to, other := tc.to(api, pdp)
		to.Handle(route(opGet), mockpermit.Reply{Body: `{"key":"a"}`})
		r := get("a")
		r.target = tc.target
		if _, err := do[testObject](t.Context(), wireClient(t, api, pdp), r); err != nil {
			t.Errorf("target %d: %v", tc.target, err)
		}
		other.AssertUntouched()
	}
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	r := get("a")
	r.target = 0
	_, err := do[testObject](t.Context(), wireClient(t, api, pdp), r)
	if err == nil || !strings.Contains(err.Error(), "names no server") {
		t.Errorf("a call without a target returned %v, want an error", err)
	}
	api.AssertUntouched()
	pdp.AssertUntouched()
}

// TestRequestBodyAndQuery checks that a call sends its body as JSON, with Content-Type, and its
// query, and that the server gets the path parameters escaped.
func TestRequestBodyAndQuery(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	api.Handle(route(opCreate), mockpermit.Reply{Status: 201, Body: `{"key":"a<b>"}`})
	api.Handle(route(opGet), mockpermit.Reply{Body: `{"key":"a/b"}`})
	c := wireClient(t, api, pdp)
	var query wire.Query
	query.AddStrings("user", []string{"a", "b c"})
	created, err := do[testObject](t.Context(), c, request{
		op: opCreate.ID, target: apiTarget, query: &query, body: testObject{Key: "a<b>"},
	})
	if err != nil || created.Key != "a<b>" {
		t.Fatalf("do returned %v, %v", created, err)
	}
	if _, err := do[testObject](t.Context(), c, get("a/b")); err != nil {
		t.Fatal(err)
	}
	requests := api.Requests()
	if len(requests) != 2 {
		t.Fatalf("the API got %d requests, want 2", len(requests))
	}
	sent := requests[0]
	sent.ExpectJSON(t, `{"key":"a<b>"}`)
	if !bytes.Contains(sent.Body, []byte("a<b>")) || sent.RawQuery != "user=a&user=b+c" ||
		sent.Header.Get("Content-Type") != "application/json" {
		t.Errorf("the request has the body %s, the query %q and the Content-Type %q", sent.Body,
			sent.RawQuery, sent.Header.Get("Content-Type"))
	}
	if got := requests[1]; got.RawPath != "/__test/objects/a%2Fb" || got.RawQuery != "" ||
		len(got.Body) != 0 || got.Header.Get("Content-Type") != "" {
		t.Errorf("the GET went to %s?%s with the body %q and the Content-Type %q", got.RawPath,
			got.RawQuery, got.Body, got.Header.Get("Content-Type"))
	}
}

// countingTransport counts the requests that a client sends through it, and fails each.
type countingTransport struct {
	requests atomic.Int32
}

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.requests.Add(1)
	return nil, errors.New("countingTransport sends nothing")
}

// TestCallsThatSendNothing checks the calls that fail before a request: a path parameter that is
// empty, . or .., which matches ErrInvalidConfig and names the placeholder, a body that cannot
// be JSON, and an operation that the registry does not hold. None sends a request.
func TestCallsThatSendNothing(t *testing.T) {
	counter := &countingTransport{}
	c := urlClient(t, canary.Key("nothing"), codeURL,
		WithHTTPClient(&http.Client{Transport: counter}))
	pathError := func(rule validate.Rule) string {
		return "permit: invalid configuration: test_get: {object_id} of " +
			"/__test/objects/{object_id}: " + rule.Message
	}
	for _, tc := range []struct {
		name       string
		r          request
		wantText   string
		wantConfig bool
	}{
		{"an empty parameter", get(""), pathError(validate.PathSegmentEmpty), true},
		{"a dot", get("."), pathError(validate.PathSegmentDot), true},
		{"two dots", get(".."), pathError(validate.PathSegmentDot), true},
		{
			"a missing parameter",
			request{op: opGet.ID, target: apiTarget},
			"permit: test_get: the route \"/__test/objects/{object_id}\" has 1 placeholders, " +
				"and 0 values fill them", false,
		},
		{"a body that cannot be JSON", request{
			op: opCreate.ID, target: apiTarget,
			body: math.NaN(),
		}, "permit: invalid configuration: test_create: the request body " +
			"cannot be written as JSON: json: unsupported value: NaN", true},
		{
			"an unknown operation",
			request{op: "test_nothing", target: apiTarget},
			"permit: the operation test_nothing is not in the client's operation registry", false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := do[testObject](t.Context(), c, tc.r)
			if err == nil || err.Error() != tc.wantText {
				t.Errorf("do returned %v, want %q", err, tc.wantText)
			}
			if got := errors.Is(err, ErrInvalidConfig); got != tc.wantConfig {
				t.Errorf("errors.Is(err, ErrInvalidConfig) = %t, want %t", got, tc.wantConfig)
			}
			if _, ok := errors.AsType[*Error](err); ok {
				t.Errorf("do returned a *Error, %v, for a call that sent no request", err)
			}
		})
	}
	if n := counter.requests.Load(); n != 0 {
		t.Errorf("the calls sent %d requests, want none", n)
	}
}

// blockingServer starts a server that answers each request with {"key":"slow"} after delay, or
// when the request's context ends, whichever comes first, and returns it.
func blockingServer(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(delay):
			_, _ = io.WriteString(w, `{"key":"slow"}`)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestWithTimeoutBoundsACall checks that the timeout of WithTimeout bounds a call whose context
// has no deadline: a reply that comes later ends the call with an error that matches
// context.DeadlineExceeded, soon after the timeout.
func TestWithTimeoutBoundsACall(t *testing.T) {
	const timeout = 200 * time.Millisecond
	srv := blockingServer(t, 10*time.Second)
	c := urlClient(t, canary.Key("timeout"), srv.URL, WithTimeout(timeout))
	start := time.Now()
	_, err := do[testObject](t.Context(), c, get("a"))
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("do returned %v, want an error that matches context.DeadlineExceeded", err)
	}
	if e := mustError(t, err); e.StatusCode != 0 {
		t.Errorf("StatusCode = %d, want 0 for a request without a reply", e.StatusCode)
	}
	if elapsed < timeout || elapsed > timeout+5*time.Second {
		t.Errorf("the call took %v, want about the timeout, %v", elapsed, timeout)
	}
}

// TestContextDeadlineReplacesTheTimeout checks that a call whose context has a deadline runs
// until that deadline, not the client's timeout: it waits past the timeout for a reply that
// comes before the deadline, and ends at a deadline that comes before the timeout.
func TestContextDeadlineReplacesTheTimeout(t *testing.T) {
	srv := blockingServer(t, 300*time.Millisecond)
	short := urlClient(t, canary.Key("deadline"), srv.URL, WithTimeout(50*time.Millisecond))
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	if got, err := do[testObject](ctx, short, get("a")); err != nil || got.Key != "slow" {
		t.Errorf("a call with a deadline past the reply returned %v, %v; want the reply", got,
			err)
	}

	long := urlClient(t, canary.Key("deadline"), srv.URL, WithTimeout(time.Minute))
	ctx, cancel = context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := do[testObject](ctx, long, get("a")); !errors.Is(err,
		context.DeadlineExceeded) {
		t.Errorf("a call with a deadline before the reply returned %v, want a deadline error",
			err)
	}
}

// TestCutReply checks a reply whose body ends before its Content-Length: a *Error with the
// reply's status and the cause.
func TestCutReply(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, buf, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = buf.WriteString("HTTP/1.1 404 Not Found\r\nContent-Length: 100\r\n\r\n{")
		_ = buf.Flush()
	}))
	t.Cleanup(srv.Close)
	_, err := do[testObject](t.Context(), urlClient(t, canary.Key("cut"), srv.URL,
		WithRetry(RetryPolicy{})), get("a"))
	e := mustError(t, err)
	if e.StatusCode != http.StatusNotFound || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("do returned %v with the status %d, want the 404 and an unexpected EOF", err,
			e.StatusCode)
	}
}

// TestNoReply checks a request that gets no reply: a *Error with status 0 whose cause is the
// *url.Error, without the URL in its text.
func TestNoReply(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	_, err := do[testObject](t.Context(), urlClient(t, canary.Key("noreply"), srv.URL,
		WithRetry(RetryPolicy{})),
		get("alice@example.com"))
	e := mustError(t, err)
	if _, ok := errors.AsType[*url.Error](err); !ok || e.StatusCode != 0 ||
		strings.Contains(e.Error(), "alice") {
		t.Errorf("do returned %v, want a *Error with status 0 that wraps a *url.Error and "+
			"does not name the object", err)
	}
}

// debugLog returns a logger that writes slog's JSON lines at Debug to a buffer that is safe for
// concurrent use, and a function that returns what it holds.
func debugLog() (*slog.Logger, func() []byte) {
	var (
		mu  sync.Mutex
		out bytes.Buffer
	)
	w := writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return out.Write(p)
	})
	logger := slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logger, func() []byte {
		mu.Lock()
		defer mu.Unlock()
		return bytes.Clone(out.Bytes())
	}
}

// writerFunc is an io.Writer that calls itself.
type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

// TestDebugLineForEachAttempt checks the Debug line of each attempt: the route template, never
// the path with its parameters; the status, 0 without a reply; the attempt; the duration; and
// the error of an attempt that failed, with the credentials replaced. The clients send each
// request once; TestRetryDebugLines checks the lines of a call that retries.
func TestDebugLineForEachAttempt(t *testing.T) {
	logger, logged := debugLog()
	once := WithRetry(RetryPolicy{})
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	api.Handle(route(opGet),
		mockpermit.Reply{Body: `{"key":"a"}`},
		mockpermit.Reply{Status: 500, Body: `{"message":"refused ` + mockpermit.APIKey + `"}`},
	)
	c := wireClient(t, api, pdp, WithLogger(logger), once)
	if _, err := do[testObject](t.Context(), c, get("alice")); err != nil {
		t.Fatal(err)
	}
	if _, err := do[testObject](t.Context(), c, get("alice")); err == nil {
		t.Fatal("the 500 gave no error")
	}
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	offline := urlClient(t, canary.Key("debugoffline"), gone.URL, WithLogger(logger), once)
	if _, err := do[testObject](t.Context(), offline, get("alice")); err == nil {
		t.Fatal("a closed server gave no error")
	}

	lines := jsonLines(t, logged())
	if len(lines) != 3 {
		t.Fatalf("the client wrote %d lines, want one for each attempt, 3:\n%s", len(lines),
			logged())
	}
	for i, want := range []struct {
		status    float64
		errorText string
	}{
		{200, ""},
		{500, "permit: test_get: GET /__test/objects/{object_id}: status 500: refused " +
			redact.Marker},
		{0, "permit: test_get: GET /__test/objects/{object_id}: "},
	} {
		line := lines[i]
		duration, _ := line["duration"].(float64)
		errorText, _ := line["error"].(string)
		if line["level"] != "DEBUG" || line["msg"] != "permit: request" ||
			line["route"] != opGet.Route || line["status"] != want.status ||
			line["attempt"] != 1.0 || duration <= 0 || !strings.HasPrefix(errorText,
			want.errorText) || (want.errorText == "") != (line["error"] == nil) {
			t.Errorf("line %d is %v, want the status %v and the error %q", i, line,
				want.status, want.errorText)
		}
	}
	scanner := canary.NewScanner(mockpermit.APIKey, canary.Key("debugoffline"), "alice")
	_, _ = scanner.Write(logged())
	scanner.Check(t)
}

// TestCallsKeepTheCredentialsTheyReturn checks that the values of an operation's
// SensitiveBodyFields, and of a key that is a credential name such as access_token, in a reply in
// 200-299 join the client's Set, so that the client's later errors and log lines replace them;
// that the value of a key that only contains a credential name, such as token_type, does not;
// and that a call of the operation that reads no body adds nothing.
func TestCallsKeepTheCredentialsTheyReturn(t *testing.T) {
	const (
		signing = "opaque-signing-value-one"
		unread  = "opaque-signing-value-two"
		access  = "opaque-access-value"
	)
	logger, logged := debugLog()
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	api.Handle(route(opSigning),
		mockpermit.Reply{Body: `{"key":"a","signing_value":"` + signing + `"}`},
		mockpermit.Reply{Body: `{"key":"b","signing_value":"` + unread + `"}`},
	)
	api.Handle(route(opCreate), mockpermit.Reply{
		Status: http.StatusCreated,
		Body:   `{"key":"c","access_token":"` + access + `","token_type":"bearer-type"}`,
	})
	api.Handle(route(opGet), mockpermit.Reply{
		Status: http.StatusBadRequest,
		Body:   `{"message":"the values ` + signing + ` and ` + access + ` expired"}`,
	})
	c := wireClient(t, api, pdp, WithLogger(logger))
	if c.secrets.Contains(signing) {
		t.Fatal("the Set holds the value before a call returned it")
	}
	got, err := do[testObject](t.Context(), c, request{op: opSigning.ID, target: apiTarget})
	if err != nil || got.Key != "a" {
		t.Fatalf("do returned %v, %v", got, err)
	}
	if !c.secrets.Contains(signing) {
		t.Error("the value of the sensitive field did not join the client's Set")
	}
	got, err = do[testObject](t.Context(), c, request{
		op: opCreate.ID, target: apiTarget, body: testObject{Key: "c"},
	})
	if err != nil || got.Key != "c" {
		t.Fatalf("do returned %v, %v", got, err)
	}
	if !c.secrets.Contains(access) || c.secrets.Contains("bearer-type") {
		t.Error("the Set does not hold the value of access_token, or holds that of token_type")
	}
	if err = c.doNoValue(t.Context(), request{op: opSigning.ID, target: apiTarget}); err != nil {
		t.Fatal(err)
	}
	if c.secrets.Contains(unread) {
		t.Error("a call that reads no body added a value to the Set")
	}
	_, err = do[testObject](t.Context(), c, get("a"))
	if e := mustError(t, err); e.StatusCode != http.StatusBadRequest {
		t.Fatalf("the 400 gave %v", err)
	}
	scanner := canary.NewScanner(signing, access)
	_, _ = scanner.Write([]byte(err.Error()))
	_, _ = scanner.Write(logged())
	scanner.Check(t)
}

// TestErrorRepliesHideTheSensitiveFieldsOfTheirRoute checks that the body of an error reply keeps
// the values of its operation's SensitiveBodyFields out, also a value that no call returned
// before, which the client's Set does not hold.
func TestErrorRepliesHideTheSensitiveFieldsOfTheirRoute(t *testing.T) {
	const value = "opaque-only-in-error"
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	api.Handle(route(opSigning), mockpermit.Reply{
		Status: http.StatusBadRequest, Body: `{"signing_value":"` + value + `","message":"x"}`,
	})
	c := wireClient(t, api, pdp)
	if c.secrets.Contains(value) {
		t.Fatal("the Set of a new client holds the value")
	}
	_, err := do[testObject](t.Context(), c, request{op: opSigning.ID, target: apiTarget})
	e := mustError(t, err)
	if e.StatusCode != http.StatusBadRequest || e.Detail != "x" {
		t.Fatalf("do returned %#v, want the 400 with what its body says", e)
	}
	if !bytes.Contains(e.Body, []byte(redact.Marker)) || bytes.Contains(e.Body, []byte(value)) {
		t.Errorf("the error's body is %q, want the field's value replaced with %s", e.Body,
			redact.Marker)
	}
}
