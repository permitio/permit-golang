package permit_test

import (
	"context"
	"errors"
	"fmt"
	"go/token"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/surface"
	"github.com/permitio/permit-golang/v2/internal/testkit/faketb"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// wireCase is the wire test of a public method: the request the method must send, the reply it
// gets and the result it must return, and an error reply of the API with the error the method
// must return for it. A method can have more than one case, such as one for each of its options.
// TestWireCases runs each case in each of its modes, as runWireCase describes.
type wireCase struct {
	// Method is the method, as its type and name, such as UsersService.Get.
	Method string

	// Op is the operationId of the route's operation in the API spec, such as get_user.
	Op string

	// Route is the route that the request goes to, as mockpermit.Server.Handle takes it, such as
	// "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}".
	Route string

	// Modes lists the ways the client sends the request; the case runs once in each. A case of
	// an operation of the facts API lists apiMode and factsViaPDPMode, and no other case lists
	// factsViaPDPMode.
	Modes []wireMode

	// Call calls the method with the client c, as the case's request needs, such as
	// func(ctx context.Context, c *permit.Client) (any, error) { return c.Scope(ctx) }, and
	// returns its result and its error. A method that returns only an error returns
	// wireNoResult{} with it.
	Call func(ctx context.Context, c *permit.Client) (any, error)

	// Request is the request the method must send.
	Request wireRequest

	// Reply is the reply to Request.
	Reply mockpermit.Reply

	// WantResult is the result the method must return for Reply, or wireNoResult{} when the
	// method returns only an error.
	WantResult any

	// ErrorReply is an error reply of the API to Request, with the API's error body.
	ErrorReply mockpermit.Reply

	// WantErr is the error the method must return for ErrorReply. In factsViaPDPMode, the
	// runner expects the route of a *permit.Error under /facts.
	WantErr error
}

// wireMode is a way the client sends a case's request.
type wireMode string

const (
	// apiMode sends the request to the Permit API.
	apiMode wireMode = "api"

	// pdpMode sends the request to the PDP.
	pdpMode wireMode = "pdp"

	// factsViaPDPMode sends a facts request to the PDP container instead of the Permit API, as
	// permit.WithFactsViaPDP does: under /facts, without the project and the environment.
	factsViaPDPMode wireMode = "facts-via-pdp"
)

// wireModes are the modes a case can list.
var wireModes = []wireMode{apiMode, pdpMode, factsViaPDPMode}

// wireRequest is the request a case expects its method to send.
type wireRequest struct {
	// RawPath is the path as the client sends it to the API, with its escapes, such as
	// /v2/facts/p/e/users/a%2Fb, where wireProject and wireEnvironment fill {proj_id} and
	// {env_id} for a method that takes neither. In factsViaPDPMode, the runner expects the path
	// under /facts, such as /facts/users/a%2Fb.
	RawPath string

	// RawQuery is the query without the "?", or empty when the method sends none.
	RawQuery string

	// Body is the JSON body, which mockpermit.Request.ExpectJSON compares, or empty when the
	// method sends none.
	Body string
}

// wireNoResult is the WantResult of a case whose method returns only an error.
type wireNoResult struct{}

// wireCaseLists holds the <area>Cases function of each *_wire_test.go file, which returns the
// wire cases of the methods of its area. Add the function of a new file here.
var wireCaseLists = []func() []wireCase{scopeCases, decisionCases, permissionCases}

// routeMethods are the request methods a route can have.
var routeMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

// collectWireCases returns the cases of every list in lists, and an error that names each case
// the registry rejects and what is wrong with it.
func collectWireCases(lists []func() []wireCase) ([]wireCase, error) {
	var cases []wireCase
	var errs []error
	for _, list := range lists {
		for _, c := range list() {
			if problems := checkWireCase(c); problems != nil {
				errs = append(errs, fmt.Errorf("the wire case of %s (%s): %s", c.Method, c.Op,
					strings.Join(problems, "; ")))
			}
			cases = append(cases, c)
		}
	}
	return cases, errors.Join(errs...)
}

// wireCases returns the registered wire cases, and fails the test when the registry rejects one.
func wireCases(t *testing.T) []wireCase {
	t.Helper()
	cases, err := collectWireCases(wireCaseLists)
	if err != nil {
		t.Fatal(err)
	}
	return cases
}

// replyFixture returns the content of the reply fixture testdata/replies/<name>, which the drift
// tests check against the API's spec. It panics when the file cannot be read, which only a broken
// checkout causes, as the <area>Cases functions have no testing.T.
func replyFixture(name string) string {
	body, err := os.ReadFile(filepath.Join("testdata", "replies", name))
	if err != nil {
		panic("the reply fixture " + name + ": " + err.Error())
	}
	return string(body)
}

// wireClientFunc returns the client that the runner calls the method of a case with: one whose
// URLs point at api and pdp, with mockpermit's API key and opts.
type wireClientFunc func(tb testing.TB, api, pdp *mockpermit.Server,
	opts ...permit.Option) *permit.Client

// wireTestClient is the wireClientFunc of the wire cases, which builds the client with
// permit.New and closes it when the test ends.
func wireTestClient(tb testing.TB, api, pdp *mockpermit.Server,
	opts ...permit.Option,
) *permit.Client {
	tb.Helper()
	base := []permit.Option{
		permit.WithAPIKey(mockpermit.APIKey), permit.WithAPIURL(api.URL),
		permit.WithPDPURL(pdp.URL),
	}
	c, err := permit.New(append(base, opts...)...)
	if err != nil {
		tb.Fatalf("permit.New: %v", err)
	}
	tb.Cleanup(func() { _ = c.Close() })
	return c
}

// The scope of the clients of the runner when the route of a case fills {proj_id} or {env_id}
// and the method takes neither, so that the client sends no lookup of the scope before its call.
const (
	wireProject     = "p"
	wireEnvironment = "e"
)

// wireWaitHeaderNames are the headers of a call that waits for sync, which the request of no case
// carries.
var wireWaitHeaderNames = []string{"X-Wait-Timeout", "X-Timeout-Policy"}

// wireScope returns the options that give the client of a case the runner's scope when route,
// the route of the case, fills {proj_id} or {env_id}, and none for another route, such as that
// of the lookup of the scope, whose case looks it up.
func wireScope(route string) []permit.Option {
	placeholders, err := routePlaceholders(route)
	if err != nil || !slices.ContainsFunc(scopePlaceholders, func(name string) bool {
		return slices.Contains(placeholders, name)
	}) {
		return nil
	}
	return []permit.Option{permit.WithScope(wireProject, wireEnvironment)}
}

// factsViaPDP returns path, a route or a path of the facts API such as
// "POST /v2/facts/{proj_id}/{env_id}/users" or /v2/facts/p/e/users/a, as the PDP container serves
// it under /facts, without the project and the environment: "POST /facts/users" or
// /facts/users/a. What comes before /v2/facts, such as the method or the prefix of a test-only
// route, stays. It returns false when path has no /v2/facts/ with three segments after it.
func factsViaPDP(path string) (string, bool) {
	head, rest, ok := strings.Cut(path, "/v2/facts/")
	parts := strings.SplitN(rest, "/", 3)
	if !ok || len(parts) < 3 || parts[2] == "" {
		return "", false
	}
	return head + "/facts/" + parts[2], true
}

// onRoute returns err as the error of a request along route, a method and a path: a copy of the
// *permit.Error that err is, with the path in Route, or else err as it is.
func onRoute(err error, route string) error {
	e, ok := errors.AsType[*permit.Error](err)
	if !ok {
		return err
	}
	moved := *e
	_, moved.Route, _ = strings.Cut(route, " ")
	return &moved
}

// runWireCase runs c in mode against mockpermit servers that fail tb on each request they do not
// expect, with a client of newClient and the options of the mode: WithFactsViaPDP in
// factsViaPDPMode, and the runner's scope when c.Route fills it. The method must send c.Request
// to the server of the mode, the API in apiMode and the PDP in the others, along c.Route, or in
// factsViaPDPMode along the route and the path under /facts. It must return c.WantResult for
// c.Reply, and c.WantErr for c.ErrorReply, with the route under /facts in factsViaPDPMode. Its
// request must carry no wait header, and the other server must get no request and no connection.
func runWireCase(tb testing.TB, c wireCase, mode wireMode, newClient wireClientFunc) {
	tb.Helper()
	target, wantErr := wireTarget{route: c.Route, rawPath: c.Request.RawPath}, c.WantErr
	opts := wireScope(c.Route)
	if mode == factsViaPDPMode {
		var routeOK, pathOK bool
		target.route, routeOK = factsViaPDP(c.Route)
		target.rawPath, pathOK = factsViaPDP(c.Request.RawPath)
		if !routeOK || !pathOK {
			tb.Errorf("%s: the route %q and the path %q are not both of the facts API, which the "+
				"mode %s needs", c.Method, c.Route, c.Request.RawPath, mode)
			return
		}
		wantErr = onRoute(c.WantErr, target.route)
		opts = append(opts, permit.WithFactsViaPDP())
	}
	call := func(reply mockpermit.Reply) (any, []mockpermit.Request, error) {
		api, pdp := mockpermit.NewAPI(tb), mockpermit.NewPDP(tb)
		to, other := api, pdp
		if mode != apiMode {
			to, other = pdp, api
		}
		to.Handle(target.route, reply)
		got, err := c.Call(tb.Context(), newClient(tb, api, pdp, opts...))
		other.AssertUntouched()
		return got, to.Requests(), err
	}

	got, requests, err := call(c.Reply)
	if err != nil {
		tb.Errorf("%s in the mode %s returned %v", c.Method, mode, err)
	} else if diff := cmp.Diff(c.WantResult, got); diff != "" {
		tb.Errorf("%s in the mode %s returned a result that differs (-want +got):\n%s", c.Method,
			mode, diff)
	}
	checkWireRequest(tb, c, mode, target, requests)
	_, _, err = call(c.ErrorReply)
	if diff := cmp.Diff(wantErr, err); diff != "" {
		tb.Errorf("%s in the mode %s returned an error that differs (-want +got):\n%s", c.Method,
			mode, diff)
	}
}

// wireTarget is where the request of a wire case goes in a mode: its route, such as
// "GET /v2/api-key/scope", and its path as sent.
type wireTarget struct {
	route, rawPath string
}

// checkWireRequest checks requests, those that the server of mode got from the call of c: one
// request, to target, with the query and the body of c.Request, and without a wait header.
func checkWireRequest(tb testing.TB, c wireCase, mode wireMode, target wireTarget,
	requests []mockpermit.Request,
) {
	tb.Helper()
	if len(requests) != 1 {
		tb.Errorf("%s in the mode %s sent %d requests to its server, want 1", c.Method, mode,
			len(requests))
		return
	}
	sent := requests[0]
	method, _, _ := strings.Cut(target.route, " ")
	if sent.Method != method || sent.RawPath != target.rawPath ||
		sent.RawQuery != c.Request.RawQuery {
		tb.Errorf("%s in the mode %s sent %s %s?%s, want %s %s?%s", c.Method, mode, sent.Method,
			sent.RawPath, sent.RawQuery, method, target.rawPath, c.Request.RawQuery)
	}
	if c.Request.Body != "" {
		sent.ExpectJSON(tb, c.Request.Body)
	} else if len(sent.Body) != 0 {
		tb.Errorf("%s in the mode %s sent the body %q, want none", c.Method, mode, sent.Body)
	}
	for _, name := range wireWaitHeaderNames {
		if values := sent.Header.Values(name); values != nil {
			tb.Errorf("%s in the mode %s sent %s: %q, which only a call that waits for sync "+
				"sends", c.Method, mode, name, values)
		}
	}
}

// factsModeProblems returns a problem for each case whose modes do not fit its operation in
// registry: the case of a Facts operation runs against the API and against the PDP container,
// so it lists apiMode and factsViaPDPMode, and the case of another operation does not list
// factsViaPDPMode. It leaves a case whose operation registry does not hold to unregisteredOps.
func factsModeProblems(cases []wireCase, registry *rest.Registry) []string {
	var problems []string
	for _, c := range cases {
		op, ok := registry.Op(c.Op)
		bothModes := slices.Contains(c.Modes, apiMode) && slices.Contains(c.Modes, factsViaPDPMode)
		switch {
		case !ok:
		case op.Facts && !bothModes:
			problems = append(problems, fmt.Sprintf("the wire case of %s (%s) has the modes %q; "+
				"the case of an operation of the facts API lists %s and %s", c.Method, c.Op,
				c.Modes, apiMode, factsViaPDPMode))
		case !op.Facts && slices.Contains(c.Modes, factsViaPDPMode):
			problems = append(problems, fmt.Sprintf("the wire case of %s (%s) lists %s, which "+
				"only the case of an operation of the facts API lists", c.Method, c.Op,
				factsViaPDPMode))
		}
	}
	return problems
}

// TestWireCases runs each wire case in each of its modes, as runWireCase describes, and checks
// that the case of each operation of the facts API runs against the API and the PDP container.
func TestWireCases(t *testing.T) {
	cases := wireCases(t)
	for _, problem := range factsModeProblems(cases, rest.Default()) {
		t.Error(problem)
	}
	for _, c := range cases {
		for _, mode := range c.Modes {
			t.Run(c.Method+"/"+string(mode), func(t *testing.T) {
				runWireCase(t, c, mode, wireTestClient)
			})
		}
	}
}

// The tests below check the runner on a test-only operation of the facts API.

// The test-only operations of the runner's tests: one of the facts API, a removal whose body
// names nothing, which permit.Client.CallFacts calls, and one that is not of the facts API.
var (
	opWireFacts = rest.Op{
		ID: "test_wire_facts", Method: http.MethodDelete,
		Route:      rest.TestPrefix + "v2/facts/{proj_id}/{env_id}/objects/{object_id}/members",
		Idempotent: true, Facts: true, KeyLevel: rest.EnvironmentLevel,
	}
	opWirePlain = rest.Op{
		ID: "test_wire_plain", Method: http.MethodGet, Route: rest.TestPrefix + "plain",
		Idempotent: true, KeyLevel: rest.EnvironmentLevel,
	}
)

// conflictBody is the API's error body of a conflict.
const conflictBody = `{"error_code":"CONFLICT","title":"Conflict",` +
	`"message":"the object changed","id":"req-2"}`

// wireFactsCase returns the wire case of a call of opWireFacts for the object a/b, in both
// modes.
func wireFactsCase() wireCase {
	return wireCase{
		Method: "ObjectsService.RemoveMembers",
		Op:     opWireFacts.ID,
		Route:  "DELETE /__test/v2/facts/{proj_id}/{env_id}/objects/{object_id}/members",
		Modes:  []wireMode{apiMode, factsViaPDPMode},
		Call: func(ctx context.Context, c *permit.Client) (any, error) {
			return wireNoResult{}, c.CallFacts(ctx, opWireFacts.ID, "a/b")
		},
		Request:    wireRequest{RawPath: "/__test/v2/facts/p/e/objects/a%2Fb/members"},
		Reply:      mockpermit.Reply{Status: http.StatusNoContent},
		WantResult: wireNoResult{},
		ErrorReply: mockpermit.Reply{Status: http.StatusConflict, Body: conflictBody},
		WantErr: &permit.Error{
			Op: opWireFacts.ID, Method: http.MethodDelete,
			Route:      "/__test/v2/facts/{proj_id}/{env_id}/objects/{object_id}/members",
			StatusCode: http.StatusConflict, Code: "CONFLICT", Title: "Conflict",
			Detail: "the object changed", RequestID: "req-2", Body: []byte(conflictBody),
		},
	}
}

// wireFactsClient is the wireClientFunc of the runner's tests, whose client also holds
// opWireFacts.
func wireFactsClient(tb testing.TB, api, pdp *mockpermit.Server,
	opts ...permit.Option,
) *permit.Client {
	tb.Helper()
	base := []permit.Option{
		permit.WithAPIKey(mockpermit.APIKey), permit.WithAPIURL(api.URL),
		permit.WithPDPURL(pdp.URL),
	}
	c := permit.NewTestClient(tb, append(base, opts...), opWireFacts)
	tb.Cleanup(func() { _ = c.Close() })
	return c
}

// TestWireRunnerRunsFactsInBothModes runs the case of a call of the facts API in each of its
// modes: to the API with the runner's scope, and to the PDP container under /facts, each time
// with nothing reaching the other server.
func TestWireRunnerRunsFactsInBothModes(t *testing.T) {
	c := wireFactsCase()
	if problems := checkWireCase(c); problems != nil {
		t.Fatalf("checkWireCase rejected the case: %q", problems)
	}
	for _, mode := range c.Modes {
		t.Run(string(mode), func(t *testing.T) { runWireCase(t, c, mode, wireFactsClient) })
	}
}

// runFake runs c in mode with the clients of newClient on a fake testing.TB, and returns what
// the runner and the servers reported, once the servers have stopped.
func runFake(t *testing.T, c wireCase, mode wireMode, newClient wireClientFunc) string {
	t.Helper()
	fake := faketb.New(t)
	fake.Run(func() { runWireCase(fake, c, mode, newClient) })
	fake.End()
	return strings.Join(fake.Take(), "\n")
}

// TestWireRunnerFailsAWrongServer checks that the runner fails a call that goes to the other
// server: a facts call that the client sends to the API in factsViaPDPMode, and one that it sends
// to the PDP container in apiMode.
func TestWireRunnerFailsAWrongServer(t *testing.T) {
	for _, tc := range []struct {
		mode wireMode
		// opts are the options of the client, which leaves out those of the mode.
		opts []permit.Option
		want string
	}{
		{factsViaPDPMode, nil, "mockpermit API: want no connection and no request"},
		{
			apiMode,
			[]permit.Option{permit.WithFactsViaPDP()},
			"mockpermit PDP: want no connection and no request",
		},
	} {
		t.Run(string(tc.mode), func(t *testing.T) {
			misrouted := func(tb testing.TB, api, pdp *mockpermit.Server,
				_ ...permit.Option,
			) *permit.Client {
				tb.Helper()
				opts := append([]permit.Option{permit.WithScope(wireProject, wireEnvironment)},
					tc.opts...)
				return wireFactsClient(tb, api, pdp, opts...)
			}
			got := runFake(t, wireFactsCase(), tc.mode, misrouted)
			for _, want := range []string{tc.want, "sent 0 requests to its server, want 1"} {
				if !strings.Contains(got, want) {
					t.Errorf("the runner reported\n%s\nwant a line that contains %q", got, want)
				}
			}
		})
	}
}

// leakingTransport sends each request through http.DefaultTransport with X-Wait-Timeout, as a
// client whose wait reached a call that did not ask for it would.
type leakingTransport struct{}

func (leakingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("X-Wait-Timeout", "1")
	return http.DefaultTransport.RoundTrip(r)
}

// TestWireRunnerFailsAWaitHeader checks that the runner fails a request that carries a wait for
// sync that its call did not ask for.
func TestWireRunnerFailsAWaitHeader(t *testing.T) {
	leaking := func(tb testing.TB, api, pdp *mockpermit.Server,
		opts ...permit.Option,
	) *permit.Client {
		tb.Helper()
		hc := &http.Client{Transport: leakingTransport{}}
		return wireFactsClient(tb, api, pdp, append(opts, permit.WithHTTPClient(hc))...)
	}
	got := runFake(t, wireFactsCase(), factsViaPDPMode, leaking)
	want := `sent X-Wait-Timeout: ["1"], which only a call that waits for sync sends`
	if !strings.Contains(got, want) {
		t.Errorf("the runner reported\n%s\nwant a line that contains %q", got, want)
	}
}

// TestWireRunnerNeedsAFactsRoute checks that the runner fails the case of a route outside the
// facts API in factsViaPDPMode, and sends nothing.
func TestWireRunnerNeedsAFactsRoute(t *testing.T) {
	sent := false
	c := scopeCases()[0]
	c.Call = func(context.Context, *permit.Client) (any, error) {
		sent = true
		return wireNoResult{}, nil
	}
	got := runFake(t, c, factsViaPDPMode, wireTestClient)
	want := `Client.Scope: the route "GET /v2/api-key/scope" and the path "/v2/api-key/scope" ` +
		"are not both of the facts API, which the mode facts-via-pdp needs"
	if got != want || sent {
		t.Errorf("the runner reported %q and made the call: %t; want %q and no call", got, sent,
			want)
	}
}

func TestFactsViaPDP(t *testing.T) {
	for path, want := range map[string]string{
		"POST /v2/facts/{proj_id}/{env_id}/users":           "POST /facts/users",
		"PUT /v2/facts/{proj_id}/{env_id}/users/{user_id}":  "PUT /facts/users/{user_id}",
		"/v2/facts/p/e/users/a%2Fb":                         "/facts/users/a%2Fb",
		"/v2/facts/my-project/production/tenants/t/users/u": "/facts/tenants/t/users/u",
		"DELETE /__test/v2/facts/{proj_id}/{env_id}/x/{id}": "DELETE /__test/facts/x/{id}",
	} {
		if got, ok := factsViaPDP(path); !ok || got != want {
			t.Errorf("factsViaPDP(%q) = %q, %t; want %q, true", path, got, ok, want)
		}
	}
	for _, path := range []string{
		"", "GET /v2/api-key/scope", "/facts/users", "/v2/facts/p/e", "/v2/facts/p/e/",
		"/v2/facts/p", "/v2/factsheet/p/e/users",
	} {
		if got, ok := factsViaPDP(path); ok {
			t.Errorf("factsViaPDP(%q) = %q, true; want false", path, got)
		}
	}
}

func TestOnRoute(t *testing.T) {
	original := &permit.Error{Op: "create_user", Route: "/v2/facts/{proj_id}/{env_id}/users"}
	got := onRoute(original, "POST /facts/users")
	want := &permit.Error{Op: "create_user", Route: "/facts/users"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("onRoute differs (-want +got):\n%s", diff)
	}
	if original.Route != "/v2/facts/{proj_id}/{env_id}/users" {
		t.Errorf("onRoute changed the error it was given: %+v", original)
	}
	if got := onRoute(errSynthetic, "POST /facts/users"); !errors.Is(got, errSynthetic) {
		t.Errorf("onRoute of an error that is not a *permit.Error returned %v, want it as it is",
			got)
	}
}

func TestWireScope(t *testing.T) {
	for route, want := range map[string]string{
		"GET /v2/facts/{proj_id}/{env_id}/users": `permit.WithScope("p", "e")`,
		"GET /v2/projects/{proj_id}/envs":        `permit.WithScope("p", "e")`,
		"GET /v2/envs/{env_id}":                  `permit.WithScope("p", "e")`,
		"GET /v2/api-key/scope":                  "",
		"GET /v2/users/{user_id}":                "",
		"GET v2/facts/{proj_id}":                 "",
	} {
		var got []string
		for _, opt := range wireScope(route) {
			got = append(got, opt.String())
		}
		if (want == "" && got != nil) || (want != "" && !slices.Equal(got, []string{want})) {
			t.Errorf("wireScope(%q) = %q, want %q", route, got, want)
		}
	}
}

func TestFactsModeProblems(t *testing.T) {
	registry, err := rest.Default().WithTestOps(opWireFacts, opWirePlain)
	if err != nil {
		t.Fatal(err)
	}
	facts, plain := wireFactsCase(), syntheticCase("PlainService.Get", "GET /__test/plain",
		"/__test/plain")
	plain.Op = opWirePlain.ID
	unknown := syntheticCase("UnknownService.Get", "GET /x", "/x")
	unknown.Modes = []wireMode{factsViaPDPMode}
	withModes := func(c wireCase, modes ...wireMode) wireCase {
		c.Modes = modes
		return c
	}
	valid := []wireCase{
		facts, withModes(facts, factsViaPDPMode, pdpMode, apiMode), plain,
		withModes(plain, pdpMode), unknown,
	}
	if problems := factsModeProblems(valid, registry); problems != nil {
		t.Errorf("factsModeProblems of cases that fit = %q, want none", problems)
	}
	got := factsModeProblems([]wireCase{
		withModes(facts, apiMode), withModes(facts, factsViaPDPMode),
		withModes(plain, apiMode, factsViaPDPMode),
	}, registry)
	want := []string{
		`the wire case of ObjectsService.RemoveMembers (test_wire_facts) has the modes ["api"]; ` +
			"the case of an operation of the facts API lists api and facts-via-pdp",
		`the wire case of ObjectsService.RemoveMembers (test_wire_facts) has the modes ` +
			`["facts-via-pdp"]; the case of an operation of the facts API lists api and ` +
			"facts-via-pdp",
		"the wire case of PlainService.Get (test_wire_plain) lists facts-via-pdp, which only the " +
			"case of an operation of the facts API lists",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("factsModeProblems differs (-want +got):\n%s", diff)
	}
}

// checkWireCase returns what the case lacks or gets wrong, or nil when it is complete.
func checkWireCase(c wireCase) []string {
	var problems []string
	if !isMethodName(c.Method) {
		problems = append(problems,
			"Method must name a method as <Type>.<Method>, such as UsersService.Get")
	}
	if c.Op == "" {
		problems = append(problems,
			"Op must be the operationId of the route's operation in the API spec")
	}
	if _, err := routePlaceholders(c.Route); err != nil {
		problems = append(problems, "Route: "+err.Error())
	}
	if len(c.Modes) == 0 {
		problems = append(problems, "Modes must list at least one mode")
	}
	for i, mode := range c.Modes {
		switch {
		case !slices.Contains(wireModes, mode):
			problems = append(problems, fmt.Sprintf("Modes has the unknown mode %q", mode))
		case slices.Contains(c.Modes[:i], mode):
			problems = append(problems, fmt.Sprintf("Modes lists %q twice", mode))
		}
	}
	if !strings.HasPrefix(c.Request.RawPath, "/") {
		problems = append(problems,
			"Request.RawPath must be the path the method sends, such as /v2/facts/p/e/users/a%2Fb")
	}
	if c.WantResult == nil {
		problems = append(problems, "WantResult must be the result the method returns for "+
			"Reply, or wireNoResult{} when it returns only an error")
	}
	if c.ErrorReply.Status < 400 || c.ErrorReply.Status > 599 || c.ErrorReply.Body == "" {
		problems = append(problems, "ErrorReply must be an error reply of the API: a status "+
			"from 400 to 599 and the API's error body")
	}
	if c.WantErr == nil {
		problems = append(problems, "WantErr must be the error the method returns for ErrorReply")
	}
	if c.Call == nil {
		problems = append(problems, "Call must call the method with the client it is given")
	}
	return problems
}

// isMethodName reports whether s names an exported method of an exported type, as
// <Type>.<Method>.
func isMethodName(s string) bool {
	typ, name, ok := strings.Cut(s, ".")
	return ok && token.IsIdentifier(typ) && token.IsExported(typ) &&
		token.IsIdentifier(name) && token.IsExported(name)
}

// routePlaceholders returns the names of the placeholders of route in order, or an error when
// route is not a method, a space and a path of segments that are not empty, whose placeholders
// are whole segments.
func routePlaceholders(route string) ([]string, error) {
	method, path, ok := strings.Cut(route, " ")
	if !ok || !slices.Contains(routeMethods, method) || !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("%q is not a method (%s), a space and a path, such as "+
			"\"GET /v2/facts/{proj_id}/{env_id}/users/{user_id}\"", route,
			strings.Join(routeMethods, ", "))
	}
	var names []string
	for segment := range strings.SplitSeq(path[1:], "/") {
		if segment == "" {
			return nil, fmt.Errorf("%q has an empty segment, which mockpermit.Server.Handle "+
				"rejects; write the path without // or a / at the end", route)
		}
		name, opened := strings.CutPrefix(segment, "{")
		name, closed := strings.CutSuffix(name, "}")
		if !opened && !closed {
			continue
		}
		if !opened || !closed || name == "" ||
			strings.Trim(name, "abcdefghijklmnopqrstuvwxyz0123456789_") != "" {
			return nil, fmt.Errorf("%q has the segment %q; a placeholder is a whole segment, a "+
				"lower-case name in braces, such as {user_id}", route, segment)
		}
		names = append(names, name)
	}
	return names, nil
}

// publicMethods returns the methods that need a wire case, by their names such as
// UsersService.Get: the methods of Client and of the types whose names end in Service, also those
// they promote from the types of the package that they embed. New and the With options are
// functions, not methods, so they are not among them.
func publicMethods(items []surface.Item) map[string]surface.Item {
	methods := make(map[string]surface.Item)
	for _, item := range items {
		if item.Kind == surface.Method && isServiceType(item.Recv) {
			methods[item.Recv+"."+item.Name] = item
		}
	}
	return methods
}

// isServiceType reports whether the type name is Client or ends in Service, whose methods need
// wire cases.
func isServiceType(name string) bool {
	return name == "Client" || strings.HasSuffix(name, "Service")
}

// noWireFile lists the public methods that need no wire case.
const noWireFile = "testdata/no-wire.txt"

// exemption is a line of testdata/no-wire.txt: a public method that needs no wire case, and why.
type exemption struct {
	method string
	reason string
	line   int
}

// parseExemptions returns the exemptions in text, the content of testdata/no-wire.txt, and an
// error that names each line that is not one.
func parseExemptions(text string) ([]exemption, error) {
	var exemptions []exemption
	var errs []error
	lineOf := make(map[string]int)
	for i, line := range strings.Split(text, "\n") {
		n := i + 1
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		method, reason, _ := strings.Cut(line, " ")
		reason = strings.TrimSpace(reason)
		switch {
		case !isMethodName(method):
			errs = append(errs, fmt.Errorf("%s:%d: %q does not start with a method as "+
				"<Type>.<Method>, such as UsersService.WaitForSync", noWireFile, n, line))
		case reason == "":
			errs = append(errs, fmt.Errorf("%s:%d: %s has no reason; write after it why it "+
				"needs no wire case", noWireFile, n, method))
		case lineOf[method] != 0:
			errs = append(errs, fmt.Errorf("%s:%d: %s is on line %d already", noWireFile, n,
				method, lineOf[method]))
		default:
			lineOf[method] = n
			exemptions = append(exemptions, exemption{method: method, reason: reason, line: n})
		}
	}
	return exemptions, errors.Join(errs...)
}

// readExemptions returns the exemptions in testdata/no-wire.txt, and fails the test when it
// cannot read them.
func readExemptions(t *testing.T) []exemption {
	t.Helper()
	text, err := os.ReadFile(noWireFile)
	if err != nil {
		t.Fatal(err)
	}
	exemptions, err := parseExemptions(string(text))
	if err != nil {
		t.Fatal(err)
	}
	return exemptions
}

// coverage is what checkCoverage found: the number of public methods, of those with a wire case
// and of those exempt, and the problems.
type coverage struct {
	methods, covered, exempt int
	problems                 []string
}

// checkCoverage checks that each public method among items has a wire case in cases or an
// exemption, that each case and exemption names a public method, and that no method has both.
// Its count sentinel checks that methods = covered + exempt. A service type that promotes
// methods internal/surface cannot list, such as those of a type of another package that it
// embeds, is a problem too: those methods would go without a case unseen.
func checkCoverage(items []surface.Item, cases []wireCase, exemptions []exemption) coverage {
	methods := publicMethods(items)
	cov := coverage{methods: len(methods)}
	for _, item := range items {
		if item.Kind != surface.Type || !isServiceType(item.Name) {
			continue
		}
		for _, unlisted := range item.Unlisted {
			cov.problems = append(cov.problems, fmt.Sprintf("%s embeds %s, whose methods "+
				"internal/surface cannot list; declare the methods on %s", item.Name, unlisted,
				item.Name))
		}
	}
	hasCase := make(map[string]bool)
	for _, c := range cases {
		if _, ok := methods[c.Method]; !ok {
			cov.problems = append(cov.problems, fmt.Sprintf("the wire case of %s (%s) names no "+
				"public method; fix its Method or remove it", c.Method, c.Op))
			continue
		}
		hasCase[c.Method] = true
	}
	cov.covered = len(hasCase)
	exempt := make(map[string]bool)
	for _, e := range exemptions {
		_, isMethod := methods[e.method]
		switch {
		case !isMethod:
			cov.problems = append(cov.problems, fmt.Sprintf("%s:%d: %s is not a public method; "+
				"remove the line", noWireFile, e.line, e.method))
		case hasCase[e.method]:
			cov.problems = append(cov.problems, fmt.Sprintf("%s:%d: %s has a wire case; remove "+
				"the line", noWireFile, e.line, e.method))
		default:
			exempt[e.method] = true
		}
	}
	cov.exempt = len(exempt)
	for _, name := range slices.Sorted(maps.Keys(methods)) {
		if !hasCase[name] && !exempt[name] {
			cov.problems = append(cov.problems, fmt.Sprintf("%s has no wire case; add one to "+
				"the <area>Cases function of its *_wire_test.go file, or list it in %s with the "+
				"reason it needs none", name, noWireFile))
		}
	}
	if cov.methods != cov.covered+cov.exempt {
		cov.problems = append(cov.problems, fmt.Sprintf("count sentinel: %d public methods, "+
			"but %d have a wire case and %d are exempt", cov.methods, cov.covered, cov.exempt))
	}
	return cov
}

func TestEveryPublicMethodHasAWireCase(t *testing.T) {
	root := readSurface(t)[0]
	cov := checkCoverage(root.Items, wireCases(t), readExemptions(t))
	for _, problem := range cov.problems {
		t.Error(problem)
	}
	t.Logf("%d public methods: %d with a wire case, %d exempt in %s", cov.methods, cov.covered,
		cov.exempt, noWireFile)
}

// scopePlaceholders are the placeholders of the project and the environment, which the client
// fills in from its scope when the method takes no parameter for them.
var scopePlaceholders = []string{"proj_id", "env_id"}

// stemAliases maps the short stems of placeholders to the words that parameter names use.
var stemAliases = map[string]string{"proj": "project", "env": "environment"}

// placeholderStem returns the stem of a placeholder: its name without an _id or else a _key
// suffix and without underscores, so user_id has the stem user and api_key_id apikey.
func placeholderStem(name string) string {
	base, ok := strings.CutSuffix(name, "_id")
	if !ok {
		base = strings.TrimSuffix(name, "_key")
	}
	return aliasStem(strings.ReplaceAll(base, "_", ""))
}

// paramStem returns the stem of a parameter name: its lower-case name without an ID or Key
// suffix, so userKey and userID have the stem user.
func paramStem(name string) string {
	for _, suffix := range []string{"ID", "Key"} {
		if base, ok := strings.CutSuffix(name, suffix); ok && base != "" {
			name = base
			break
		}
	}
	return aliasStem(strings.ToLower(name))
}

// aliasStem returns the word for the stem s, such as project for proj.
func aliasStem(s string) string {
	if word, ok := stemAliases[s]; ok {
		return word
	}
	return s
}

// routeParamOrder returns an error unless the placeholders of route name parameters of params in
// the route's order. A placeholder names the parameter with its stem, so {resource_id} names
// resourceKey or resourceID and {proj_id} names projectKey; the placeholders in scopePlaceholders
// need a parameter only when the method takes one.
func routeParamOrder(route string, params []surface.Param) error {
	placeholders, err := routePlaceholders(route)
	if err != nil {
		return err
	}
	taken := make([]bool, len(params))
	last, previous := -1, ""
	for _, placeholder := range placeholders {
		i := freeParam(params, taken, placeholderStem(placeholder))
		switch {
		case i < 0 && slices.Contains(scopePlaceholders, placeholder):
			continue
		case i < 0:
			return fmt.Errorf("no parameter is named after the placeholder {%s} of %q; name it "+
				"after the placeholder, as userKey or userID for {user_id}", placeholder, route)
		case i < last:
			return fmt.Errorf("the parameter %s comes before %s, but %q has {%s} after {%s}; "+
				"take the parameters in the route's order", params[i].Name, params[last].Name,
				route, placeholder, previous)
		}
		taken[i] = true
		last, previous = i, placeholder
	}
	return nil
}

// freeParam returns the index of the first parameter of params with the stem that an earlier
// placeholder has not taken, or -1, so that two placeholders of one stem, such as {user_id} and
// {user_key}, name two parameters.
func freeParam(params []surface.Param, taken []bool, stem string) int {
	for i, p := range params {
		if !taken[i] && paramStem(p.Name) == stem {
			return i
		}
	}
	return -1
}

// routeOrderProblems checks the route of each case against the parameters of its method among
// items.
func routeOrderProblems(items []surface.Item, cases []wireCase) []string {
	methods := publicMethods(items)
	var problems []string
	for _, c := range cases {
		m, ok := methods[c.Method]
		if !ok {
			problems = append(problems, fmt.Sprintf("the wire case of %s (%s) names no public "+
				"method", c.Method, c.Op))
			continue
		}
		if err := routeParamOrder(c.Route, m.Params); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", c.Method, err))
		}
	}
	return problems
}

func TestRouteParamOrder(t *testing.T) {
	root := readSurface(t)[0]
	for _, problem := range routeOrderProblems(root.Items, wireCases(t)) {
		t.Error(problem)
	}
}

// The tests below check the checks on a small SDK written for them.

// syntheticSDK is the source of an SDK with a client, two services, a page type, a constructor
// and an option.
const syntheticSDK = `package permit

import "context"

type Client struct {
	Users          *UsersService
	ImplicitGrants *ImplicitGrantsService
}

func New(apiKey string, opts ...Option) (*Client, error) { return nil, nil }

type Option func(*Client)

func WithScope(projectKey, environmentKey string) Option { return nil }

func (c *Client) Scope(ctx context.Context) (string, error) { return "", nil }

type UsersService struct{}

func (s *UsersService) Get(ctx context.Context, userKey string) (string, error) {
	return "", nil
}

func (s *UsersService) Delete(ctx context.Context, userKey string) error { return nil }

func (s *UsersService) WaitForSync() *UsersService { return s }

type ImplicitGrantsService struct{}

func (s *ImplicitGrantsService) Delete(ctx context.Context, resourceKey, roleKey string) error {
	return nil
}

type Page struct{}

func (p *Page) Next(ctx context.Context) bool { return false }
`

// errSynthetic is the WantErr of the synthetic cases.
var errSynthetic = errors.New("the API answered with an error")

// readSynthetic lists the exported API of the source src, a variant of syntheticSDK.
func readSynthetic(t *testing.T, src string) []surface.Item {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "permit.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := surface.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p.Items
}

// syntheticCase returns a complete wire case of method that sends rawPath to route.
func syntheticCase(method, route, rawPath string) wireCase {
	return wireCase{
		Method:     method,
		Op:         "op_" + strings.ReplaceAll(method, ".", "_"),
		Route:      route,
		Modes:      []wireMode{apiMode},
		Request:    wireRequest{RawPath: rawPath, RawQuery: "", Body: ""},
		Reply:      mockpermit.Reply{Status: http.StatusNoContent},
		WantResult: wireNoResult{},
		ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: `{"detail": "not found"}`},
		WantErr:    errSynthetic,
		Call: func(context.Context, *permit.Client) (any, error) {
			return wireNoResult{}, nil
		},
	}
}

// syntheticCases returns a wire case for each method of syntheticSDK that sends a request.
func syntheticCases() []wireCase {
	return []wireCase{
		syntheticCase("Client.Scope", "GET /v2/api-key/scope", "/v2/api-key/scope"),
		syntheticCase("UsersService.Get", "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}",
			"/v2/facts/p/e/users/u"),
		syntheticCase("UsersService.Delete",
			"DELETE /v2/facts/{proj_id}/{env_id}/users/{user_id}", "/v2/facts/p/e/users/u"),
		syntheticCase("ImplicitGrantsService.Delete", "DELETE /v2/schema/{proj_id}/{env_id}/"+
			"resources/{resource_id}/roles/{role_id}/implicit_grants",
			"/v2/schema/p/e/resources/doc/roles/viewer/implicit_grants"),
	}
}

// syntheticExemptions exempts the one method of syntheticSDK that sends no request.
var syntheticExemptions = []exemption{
	{method: "UsersService.WaitForSync", reason: "returns a copy of the service", line: 5},
}

func TestPublicMethods(t *testing.T) {
	got := slices.Sorted(maps.Keys(publicMethods(readSynthetic(t, syntheticSDK))))
	want := []string{
		"Client.Scope",
		"ImplicitGrantsService.Delete",
		"UsersService.Delete",
		"UsersService.Get",
		"UsersService.WaitForSync",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("publicMethods differs (-want +got):\n%s", diff)
	}
}

func TestCheckCoveragePasses(t *testing.T) {
	cov := checkCoverage(readSynthetic(t, syntheticSDK), syntheticCases(), syntheticExemptions)
	want := coverage{methods: 5, covered: 4, exempt: 1}
	if diff := cmp.Diff(want, cov, cmp.AllowUnexported(coverage{})); diff != "" {
		t.Errorf("checkCoverage differs (-want +got):\n%s", diff)
	}
}

func TestCheckCoverageFails(t *testing.T) {
	items := readSynthetic(t, syntheticSDK)
	cases := syntheticCases()
	for _, tc := range []struct {
		name       string
		cases      []wireCase
		exemptions []exemption
		want       coverage
	}{
		{
			name:       "a method without a case",
			cases:      slices.Delete(slices.Clone(cases), 2, 3),
			exemptions: syntheticExemptions,
			want: coverage{methods: 5, covered: 3, exempt: 1, problems: []string{
				"UsersService.Delete has no wire case; add one to the <area>Cases function of " +
					"its *_wire_test.go file, or list it in testdata/no-wire.txt with the reason " +
					"it needs none",
				"count sentinel: 5 public methods, but 3 have a wire case and 1 are exempt",
			}},
		},
		{
			name: "a case that names no method",
			cases: append(slices.Clone(cases), syntheticCase("UsersService.Remove",
				"DELETE /v2/facts/{proj_id}/{env_id}/users/{user_id}", "/v2/facts/p/e/users/u")),
			exemptions: syntheticExemptions,
			want: coverage{methods: 5, covered: 4, exempt: 1, problems: []string{
				"the wire case of UsersService.Remove (op_UsersService_Remove) names no public " +
					"method; fix its Method or remove it",
			}},
		},
		{
			name:  "an exemption of no method",
			cases: cases,
			exemptions: append(slices.Clone(syntheticExemptions),
				exemption{method: "UsersService.List", reason: "gone", line: 7}),
			want: coverage{methods: 5, covered: 4, exempt: 1, problems: []string{
				"testdata/no-wire.txt:7: UsersService.List is not a public method; remove the line",
			}},
		},
		{
			name:  "an exemption of a method with a case",
			cases: cases,
			exemptions: append(slices.Clone(syntheticExemptions),
				exemption{method: "UsersService.Get", reason: "no request", line: 9}),
			want: coverage{methods: 5, covered: 4, exempt: 1, problems: []string{
				"testdata/no-wire.txt:9: UsersService.Get has a wire case; remove the line",
			}},
		},
		{
			name:  "a method that is neither",
			cases: cases,
			want: coverage{methods: 5, covered: 4, exempt: 0, problems: []string{
				"UsersService.WaitForSync has no wire case; add one to the <area>Cases function " +
					"of its *_wire_test.go file, or list it in testdata/no-wire.txt with the " +
					"reason it needs none",
				"count sentinel: 5 public methods, but 4 have a wire case and 0 are exempt",
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := checkCoverage(items, tc.cases, tc.exemptions)
			if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(coverage{})); diff != "" {
				t.Errorf("checkCoverage differs (-want +got):\n%s", diff)
			}
		})
	}
}

// TestCheckCoverageEmbedded checks what a service type gets from the types it embeds: a method of
// an interface of the package needs a wire case, and a type of another package or error, whose
// methods internal/surface cannot list, is a problem. Other types may embed such types.
func TestCheckCoverageEmbedded(t *testing.T) {
	const imports = "import (\n\t\"context\"\n\n\t\"example.com/sdk/models\"\n)"
	src := strings.NewReplacer(
		`import "context"`, imports,
		"ImplicitGrants *ImplicitGrantsService\n}",
		"ImplicitGrants *ImplicitGrantsService\n\terror\n}",
		"type UsersService struct{}", "type UsersService struct {\n\tlister\n\tmodels.Base\n}\n\n"+
			"type lister interface {\n\tList(ctx context.Context) ([]string, error)\n}",
		"type Page struct{}", "type Page struct{ models.Base }",
	).Replace(syntheticSDK)
	got := checkCoverage(readSynthetic(t, src), syntheticCases(), syntheticExemptions)
	want := coverage{methods: 6, covered: 4, exempt: 1, problems: []string{
		"Client embeds error, whose methods internal/surface cannot list; declare the methods " +
			"on Client",
		"UsersService embeds models.Base, whose methods internal/surface cannot list; declare " +
			"the methods on UsersService",
		"UsersService.List has no wire case; add one to the <area>Cases function of its " +
			"*_wire_test.go file, or list it in testdata/no-wire.txt with the reason it needs none",
		"count sentinel: 6 public methods, but 4 have a wire case and 1 are exempt",
	}}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(coverage{})); diff != "" {
		t.Errorf("checkCoverage differs (-want +got):\n%s", diff)
	}
}

func TestRouteOrderProblemsPasses(t *testing.T) {
	if problems := routeOrderProblems(readSynthetic(t, syntheticSDK),
		syntheticCases()); problems != nil {
		t.Errorf("routeOrderProblems found %q, want none", problems)
	}
}

// TestRouteOrderProblemsFindsSwappedParameters checks a method whose parameters name the route's
// placeholders the wrong way round.
func TestRouteOrderProblemsFindsSwappedParameters(t *testing.T) {
	const inOrder, swappedNames = "resourceKey, roleKey string", "roleKey, resourceKey string"
	swapped := strings.Replace(syntheticSDK, inOrder, swappedNames, 1)
	if swapped == syntheticSDK {
		t.Fatal("the synthetic SDK has no ImplicitGrantsService.Delete to swap")
	}
	got := routeOrderProblems(readSynthetic(t, swapped), syntheticCases())
	want := []string{
		"ImplicitGrantsService.Delete: the parameter roleKey comes before resourceKey, but " +
			`"DELETE /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/` +
			`implicit_grants" has {role_id} after {resource_id}; take the parameters in the ` +
			"route's order",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("routeOrderProblems differs (-want +got):\n%s", diff)
	}
}

func TestRouteOrderProblemsFindsACaseWithoutAMethod(t *testing.T) {
	cases := []wireCase{syntheticCase("UsersService.Remove",
		"DELETE /v2/facts/{proj_id}/{env_id}/users/{user_id}", "/v2/facts/p/e/users/u")}
	got := routeOrderProblems(readSynthetic(t, syntheticSDK), cases)
	want := []string{
		"the wire case of UsersService.Remove (op_UsersService_Remove) names no public method",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("routeOrderProblems differs (-want +got):\n%s", diff)
	}
}

func TestRouteParamOrderRules(t *testing.T) {
	ctx := surface.Param{Name: "ctx", Type: "context.Context"}
	str := func(name string) surface.Param { return surface.Param{Name: name, Type: "string"} }
	for _, tc := range []struct {
		name   string
		route  string
		params []surface.Param
		want   string
	}{
		{
			name:   "no placeholders",
			route:  "GET /v2/api-key/scope",
			params: []surface.Param{ctx},
		},
		{
			name:   "scope filled by the client",
			route:  "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}",
			params: []surface.Param{ctx, str("userID")},
		},
		{
			name:   "scope taken as parameters",
			route:  "GET /v2/projects/{proj_id}/envs/{env_id}",
			params: []surface.Param{ctx, str("projectKey"), str("environmentKey")},
		},
		{
			name:   "short scope names",
			route:  "GET /v2/projects/{proj_id}/envs/{env_id}",
			params: []surface.Param{ctx, str("projID"), str("envID")},
		},
		{
			name:   "scope parameters swapped",
			route:  "GET /v2/projects/{proj_id}/envs/{env_id}",
			params: []surface.Param{ctx, str("environmentKey"), str("projectKey")},
			want: `the parameter environmentKey comes before projectKey, but "GET ` +
				`/v2/projects/{proj_id}/envs/{env_id}" has {env_id} after {proj_id}`,
		},
		{
			name:   "names with underscores and a key suffix",
			route:  "GET /v2/facts/{proj_id}/{env_id}/resource_instances/{resource_instance_key}",
			params: []surface.Param{ctx, str("resourceInstanceKey")},
		},
		{
			name:   "a placeholder named key",
			route:  "DELETE /v2/api-key/{api_key_id}",
			params: []surface.Param{ctx, str("apiKeyID")},
		},
		{
			name:   "a parameter without a suffix",
			route:  "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}",
			params: []surface.Param{ctx, str("user")},
		},
		{
			name:   "parameters between the path parameters",
			route:  "PUT /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}/users/{user_id}",
			params: []surface.Param{ctx, str("tenantKey"), str("note"), str("userKey")},
		},
		{
			name:   "two placeholders of one stem",
			route:  "PUT /v2/users/{user_id}/delegates/{user_key}",
			params: []surface.Param{ctx, str("userID"), str("userKey")},
		},
		{
			name:   "a second placeholder of one stem without a parameter",
			route:  "PUT /v2/users/{user_id}/delegates/{user_key}",
			params: []surface.Param{ctx, str("userID")},
			want: `no parameter is named after the placeholder {user_key} of "PUT ` +
				`/v2/users/{user_id}/delegates/{user_key}"`,
		},
		{
			name:   "swapped parameters and a later one of the same stem",
			route:  "PUT /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}/users/{user_id}",
			params: []surface.Param{ctx, str("userKey"), str("tenantKey"), str("user")},
			want:   "the parameter userKey comes before tenantKey",
		},
		{
			name:   "a placeholder without a parameter",
			route:  "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}",
			params: []surface.Param{ctx, str("key")},
			want: `no parameter is named after the placeholder {user_id} of "GET ` +
				`/v2/facts/{proj_id}/{env_id}/users/{user_id}"; name it after the placeholder`,
		},
		{
			name:   "unnamed parameters",
			route:  "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}",
			params: []surface.Param{{Type: "context.Context"}, {Type: "string"}},
			want:   "no parameter is named after the placeholder {user_id}",
		},
		{
			name:   "a malformed route",
			route:  "GET v2/users",
			params: []surface.Param{ctx},
			want: `"GET v2/users" is not a method (GET, POST, PUT, PATCH, DELETE), a space and ` +
				"a path",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := routeParamOrder(tc.route, tc.params)
			switch {
			case tc.want == "" && err != nil:
				t.Errorf("routeParamOrder returned %v, want nil", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Errorf("routeParamOrder returned %v, want an error that contains %q", err,
					tc.want)
			}
		})
	}
}

func TestStems(t *testing.T) {
	for name, want := range map[string]string{
		"user_id":              "user",
		"group_instance_key":   "groupinstance",
		"resource_instance_id": "resourceinstance",
		"proj_id":              "project",
		"env_id":               "environment",
		"api_key_id":           "apikey",
		"template_type":        "templatetype",
	} {
		if got := placeholderStem(name); got != want {
			t.Errorf("placeholderStem(%q) = %q, want %q", name, got, want)
		}
	}
	for name, want := range map[string]string{
		"userKey":     "user",
		"userID":      "user",
		"user":        "user",
		"ID":          "id",
		"Key":         "key",
		"projectKey":  "project",
		"projID":      "project",
		"envKey":      "environment",
		"keyID":       "key",
		"groupIDKey":  "groupid",
		"apiKeyID":    "apikey",
		"resourceKey": "resource",
	} {
		if got := paramStem(name); got != want {
			t.Errorf("paramStem(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestRoutePlaceholders(t *testing.T) {
	for route, want := range map[string][]string{
		"GET /v2/api-key/scope": nil,
		"POST /allowed":         nil,
		"PUT /v2/x/{a}/{b}":     {"a", "b"},
		"DELETE /v2/facts/{proj_id}/{env_id}/users/{user_id}": {
			"proj_id", "env_id", "user_id",
		},
		"PATCH /v2/schema/{proj_id}/{env_id}/resources/{res_9_id}": {
			"proj_id", "env_id", "res_9_id",
		},
	} {
		got, err := routePlaceholders(route)
		if err != nil {
			t.Errorf("routePlaceholders(%q) returned %v", route, err)
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("routePlaceholders(%q) differs (-want +got):\n%s", route, diff)
		}
	}
	for route, want := range map[string]string{
		"":                        `"" is not a method`,
		"GET":                     `"GET" is not a method`,
		"FETCH /v2/users":         `"FETCH /v2/users" is not a method`,
		"get /v2/users":           `"get /v2/users" is not a method`,
		"GET v2/users":            `"GET v2/users" is not a method`,
		"GET /v2/users/{user_id":  `has the segment "{user_id"; a placeholder is a whole segment`,
		"GET /v2/users/user_id}":  `has the segment "user_id}"`,
		"GET /v2/users/{}":        `has the segment "{}"`,
		"GET /v2/users/{UserID}":  `has the segment "{UserID}"`,
		"GET /v2/users/{user-id}": `has the segment "{user-id}"`,
		"GET /v2/users/x{id}":     `has the segment "x{id}"`,
		"GET /v2//users":          `"GET /v2//users" has an empty segment`,
		"GET /v2/users/":          `"GET /v2/users/" has an empty segment`,
		"GET /":                   `"GET /" has an empty segment`,
	} {
		got, err := routePlaceholders(route)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("routePlaceholders(%q) returned %q, %v; want an error that contains %q",
				route, got, err, want)
		}
	}
}

func TestCheckWireCase(t *testing.T) {
	valid := syntheticCase("UsersService.Get", "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}",
		"/v2/facts/p/e/users/u")
	if problems := checkWireCase(valid); problems != nil {
		t.Fatalf("checkWireCase rejected a complete case: %q", problems)
	}
	for _, tc := range []struct {
		name   string
		change func(*wireCase)
		want   string
	}{
		{"no method", func(c *wireCase) { c.Method = "" }, "Method must name a method"},
		{"a method without its type", func(c *wireCase) { c.Method = "Get" }, "Method must name"},
		{"an unexported type", func(c *wireCase) { c.Method = "usersService.Get" }, "Method must"},
		{"an unexported method", func(c *wireCase) { c.Method = "Users.get" }, "Method must"},
		{"a method of three parts", func(c *wireCase) { c.Method = "A.B.C" }, "Method must name"},
		{"no operation", func(c *wireCase) { c.Op = "" }, "Op must be the operationId"},
		{"no route", func(c *wireCase) { c.Route = "" }, `Route: "" is not a method`},
		{"a malformed route", func(c *wireCase) { c.Route = "GET /users/{id" }, "Route: "},
		{"no modes", func(c *wireCase) { c.Modes = nil }, "Modes must list at least one mode"},
		{
			"empty modes", func(c *wireCase) { c.Modes = []wireMode{} },
			"Modes must list at least one mode",
		},
		{
			"an unknown mode", func(c *wireCase) { c.Modes = []wireMode{"cloud"} },
			`Modes has the unknown mode "cloud"`,
		},
		{
			"a mode twice", func(c *wireCase) { c.Modes = []wireMode{apiMode, pdpMode, apiMode} },
			`Modes lists "api" twice`,
		},
		{"no request", func(c *wireCase) { c.Request = wireRequest{} }, "Request.RawPath must be"},
		{
			"a relative path", func(c *wireCase) { c.Request.RawPath = "v2/users" },
			"Request.RawPath must be",
		},
		{"no result", func(c *wireCase) { c.WantResult = nil }, "WantResult must be the result"},
		{
			"no error reply", func(c *wireCase) { c.ErrorReply = mockpermit.Reply{} },
			"ErrorReply must be an error reply of the API",
		},
		{
			"a success as the error reply", func(c *wireCase) { c.ErrorReply.Status = 200 },
			"ErrorReply must be",
		},
		{"status 399", func(c *wireCase) { c.ErrorReply.Status = 399 }, "ErrorReply must be"},
		{"status 600", func(c *wireCase) { c.ErrorReply.Status = 600 }, "ErrorReply must be"},
		{
			"an error reply without a body", func(c *wireCase) { c.ErrorReply.Body = "" },
			"ErrorReply must be",
		},
		{"no error", func(c *wireCase) { c.WantErr = nil }, "WantErr must be the error"},
		{"no call", func(c *wireCase) { c.Call = nil }, "Call must call the method"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := valid
			c.Modes = slices.Clone(valid.Modes)
			tc.change(&c)
			problems := checkWireCase(c)
			if len(problems) != 1 || !strings.Contains(problems[0], tc.want) {
				t.Errorf("checkWireCase returned %q, want one problem that contains %q", problems,
					tc.want)
			}
		})
	}
	for _, status := range []int{400, 599} {
		c := valid
		c.ErrorReply.Status = status
		if problems := checkWireCase(c); problems != nil {
			t.Errorf("checkWireCase rejected an error reply with status %d: %q", status, problems)
		}
	}
	allModes := valid
	allModes.Modes = []wireMode{apiMode, pdpMode, factsViaPDPMode}
	if problems := checkWireCase(allModes); problems != nil {
		t.Errorf("checkWireCase rejected a case in every mode: %q", problems)
	}
}

func TestCollectWireCases(t *testing.T) {
	good := syntheticCases()
	bad := syntheticCase("UsersService.Get", "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}",
		"/v2/facts/p/e/users/u")
	bad.ErrorReply = mockpermit.Reply{}
	bad.WantErr = nil
	got, err := collectWireCases([]func() []wireCase{
		func() []wireCase { return good[:2] },
		func() []wireCase { return []wireCase{bad} },
		func() []wireCase { return good[2:] },
	})
	want := "the wire case of UsersService.Get (op_UsersService_Get): ErrorReply must be an " +
		"error reply of the API: a status from 400 to 599 and the API's error body; WantErr must " +
		"be the error the method returns for ErrorReply"
	if err == nil || err.Error() != want {
		t.Errorf("collectWireCases returned the error %v, want %q", err, want)
	}
	if len(got) != len(good)+1 || got[2].ErrorReply.Status != 0 {
		t.Errorf("collectWireCases returned %d cases, want all %d in order", len(got), len(good)+1)
	}
	cases, err := collectWireCases([]func() []wireCase{syntheticCases})
	if err != nil || len(cases) != len(good) {
		t.Errorf("collectWireCases of complete cases returned %d cases and %v, want %d and nil",
			len(cases), err, len(good))
	}
	if cases, err := collectWireCases(nil); cases != nil || err != nil {
		t.Errorf("collectWireCases of no lists returned %v, %v; want nil, nil", cases, err)
	}
}

func TestParseExemptions(t *testing.T) {
	got, err := parseExemptions("# header\n\nUsersService.WaitForSync returns a copy of the " +
		"service\nClient.Close  closes idle connections; sends nothing \n")
	want := []exemption{
		{method: "UsersService.WaitForSync", reason: "returns a copy of the service", line: 3},
		{method: "Client.Close", reason: "closes idle connections; sends nothing", line: 4},
	}
	if err != nil {
		t.Errorf("parseExemptions returned %v", err)
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(exemption{})); diff != "" {
		t.Errorf("parseExemptions differs (-want +got):\n%s", diff)
	}
	const noReason = "testdata/no-wire.txt:1: Users.WaitForSync has no reason"
	for text, want := range map[string]string{
		"Users.WaitForSync\n":   noReason,
		"Users.WaitForSync  \n": noReason,
		"WaitForSync returns a copy\n": `testdata/no-wire.txt:1: "WaitForSync returns a copy" ` +
			"does not start with a method",
		" Users.Get indented\n": `testdata/no-wire.txt:1: " Users.Get indented" does not ` +
			"start with a method",
		"A.B one\n\nA.B two\n": "testdata/no-wire.txt:3: A.B is on line 1 already",
	} {
		if _, err := parseExemptions(text); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("parseExemptions(%q) returned %v, want an error that contains %q", text, err,
				want)
		}
	}
}
