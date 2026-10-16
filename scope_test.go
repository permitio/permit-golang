package permit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/testkit/concurrent"
	"github.com/permitio/permit-golang/v2/internal/testkit/leak"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/internal/validate"
)

// The IDs that the scope fixtures in testdata/replies hold, mostly zeros so that the secret scan
// does not take them for credentials.
const (
	scopeProjectID     = "00000000-0000-4000-8000-000000000002"
	scopeEnvironmentID = "00000000-0000-4000-8000-000000000003"
)

// scopeRoute is the route of the scope lookup, as mockpermit.Server.Handle takes it.
const scopeRoute = "GET /v2/api-key/scope"

// The test-only operations of the scope tests: one of each key level, as a method that gets an
// object of an environment, one that gets an object of a project, and one that lists the projects
// of the organization, such as ProjectsService.List, would call; and one that an
// environment-level key may call and whose route fills only {proj_id}, as a method that lists the
// environments of the project would call.
var (
	opInEnvironment = rest.Op{
		ID: "test_in_environment", Method: http.MethodGet,
		Route:      rest.TestPrefix + "facts/{proj_id}/{env_id}/objects/{object_id}",
		Idempotent: true, KeyLevel: rest.EnvironmentLevel, ResponseModel: "TestObject",
	}
	opInProject = rest.Op{
		ID: "test_in_project", Method: http.MethodGet,
		Route:      rest.TestPrefix + "projects/{proj_id}/objects/{object_id}",
		Idempotent: true, KeyLevel: rest.ProjectLevel, ResponseModel: "TestObject",
	}
	opProjects = rest.Op{
		ID: "test_projects", Method: http.MethodGet, Route: rest.TestPrefix + "projects",
		Idempotent: true, KeyLevel: rest.OrganizationLevel, MaxPerPage: 100,
		ResponseModel: "TestObject",
	}
	opEnvironments = rest.Op{
		ID: "test_environments", Method: http.MethodGet,
		Route:      rest.TestPrefix + "projects/{proj_id}/environments",
		Idempotent: true, KeyLevel: rest.EnvironmentLevel, MaxPerPage: 100,
		ResponseModel: "TestObject",
	}
)

// scopeClient returns a test client with the scope tests' operations, mockpermit's API key and
// opts, whose API URL is apiURL, and closes it when the test ends.
func scopeClient(t *testing.T, apiURL string, opts ...Option) *Client {
	t.Helper()
	clearEnv(t)
	base := []Option{WithAPIKey(mockpermit.APIKey), WithAPIURL(apiURL), WithPDPURL(codeURL)}
	c := newTestClient(t, append(base, opts...), opInEnvironment, opInProject, opProjects,
		opEnvironments)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// scopeReply returns the reply fixture testdata/replies/get_api_key_scope.200<suffix>.json, such
// as the scope of a project-level key for the suffix .project.
func scopeReply(t *testing.T, suffix string) mockpermit.Reply {
	t.Helper()
	body, err := os.ReadFile("testdata/replies/get_api_key_scope.200" + suffix + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return mockpermit.Reply{Body: string(body)}
}

// getInEnvironment gets the object key of the client's environment, as a method of the SDK does:
// it fills {proj_id} and {env_id} from the client's scope.
func getInEnvironment(ctx context.Context, c *Client, key string) (*testObject, error) {
	project, environment, err := c.scopeFor(ctx, opInEnvironment.ID)
	if err != nil {
		return nil, err
	}
	return do[testObject](ctx, c, request{
		op: opInEnvironment.ID, target: apiTarget, params: []string{project, environment, key},
	})
}

// getInProject gets the object key of the client's project, as a method of the SDK does.
func getInProject(ctx context.Context, c *Client, key string) (*testObject, error) {
	project, _, err := c.scopeFor(ctx, opInProject.ID)
	if err != nil {
		return nil, err
	}
	return do[testObject](ctx, c, request{
		op: opInProject.ID, target: apiTarget, params: []string{project, key},
	})
}

// listProjects lists the projects of the organization, as a method of the SDK does.
func listProjects(ctx context.Context, c *Client) error {
	if _, _, err := c.scopeFor(ctx, opProjects.ID); err != nil {
		return err
	}
	_, err := doList[testObject](ctx, c, request{op: opProjects.ID, target: apiTarget},
		arrayOrPageShape)
	return err
}

// listEnvironments lists the environments of the client's project, as a method of the SDK does:
// it fills {proj_id} from the client's scope.
func listEnvironments(ctx context.Context, c *Client) error {
	project, _, err := c.scopeFor(ctx, opEnvironments.ID)
	if err != nil {
		return err
	}
	_, err = doList[testObject](ctx, c, request{
		op: opEnvironments.ID, target: apiTarget, params: []string{project},
	}, arrayOrPageShape)
	return err
}

// countRequests returns how many requests of the server went to the path.
func countRequests(s *mockpermit.Server, path string) int {
	n := 0
	for _, r := range s.Requests() {
		if r.RawPath == path {
			n++
		}
	}
	return n
}

// TestScopeConcurrentFirstCalls makes the first calls of a client from 50 goroutines that a start
// barrier releases together, under -race, and checks that they share one scope request, which
// the server answers once every call has started, and that each call gets its object.
func TestScopeConcurrentFirstCalls(t *testing.T) {
	leak.Check(t)
	const callers = 50
	var started, scopeRequests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/api-key/scope" {
			key := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			_, _ = io.WriteString(w, `{"key":"`+key+`"}`)
			return
		}
		scopeRequests.Add(1)
		// Hold the reply until every call has started, so that they all wait for this request.
		for deadline := time.Now().Add(10 * time.Second); started.Load() < callers &&
			time.Now().Before(deadline); {
			time.Sleep(time.Millisecond)
		}
		time.Sleep(20 * time.Millisecond)
		_, _ = io.WriteString(w, `{"organization_id":"o","project_id":"p","environment_id":"e"}`)
	}))
	t.Cleanup(srv.Close)
	c := scopeClient(t, srv.URL)

	err := concurrent.Run(t, callers, 30*time.Second, func(ctx context.Context, i int) error {
		started.Add(1)
		key := strconv.Itoa(i)
		got, err := getInEnvironment(ctx, c, key)
		if err != nil {
			return err
		}
		if got.Key != key {
			return fmt.Errorf("got the object %q, want %q", got.Key, key)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := scopeRequests.Load(); n != 1 {
		t.Errorf("%d first calls sent %d scope requests, want 1", callers, n)
	}
}

// TestScopeFailureIsNotKept checks that a lookup that a 429 ends, after the retries, returns the
// 429 with the API's body, and that the next call looks the scope up again and succeeds.
func TestScopeFailureIsNotKept(t *testing.T) {
	const limited = `{"error_code":"TOO_MANY_REQUESTS","title":"Too Many Requests",` +
		`"message":"slow down"}`
	tooMany := mockpermit.Reply{
		Status: http.StatusTooManyRequests, Body: limited,
		Header: http.Header{"Retry-After": {"0"}},
	}
	api := mockpermit.NewAPI(t)
	api.Handle(scopeRoute, tooMany, tooMany, tooMany, tooMany, scopeReply(t, ""))
	api.Handle(route(opInEnvironment), mockpermit.Reply{Body: `{"key":"a"}`})
	c := scopeClient(t, api.URL)

	_, err := c.Scope(t.Context())
	e := mustError(t, err)
	want := &Error{
		Op: opGetAPIKeyScope, Method: http.MethodGet, Route: "/v2/api-key/scope",
		StatusCode: http.StatusTooManyRequests, Code: "TOO_MANY_REQUESTS",
		Title: "Too Many Requests", Detail: "slow down", Body: []byte(limited),
	}
	if diff := cmp.Diff(want, e); diff != "" || !errors.Is(err, ErrRateLimited) {
		t.Errorf("the rate-limited lookup returned an Error that differs (-want +got):\n%s", diff)
	}
	if n := countRequests(api, "/v2/api-key/scope"); n != 4 {
		t.Errorf("the rate-limited lookup sent %d requests, want 4: the first and 3 retries", n)
	}

	got, err := getInEnvironment(t.Context(), c, "a")
	if err != nil || got.Key != "a" {
		t.Fatalf("the call after the failed lookup returned %+v, %v; want the object", got, err)
	}
	if n := countRequests(api, "/v2/api-key/scope"); n != 5 {
		t.Errorf("the call after the failed lookup made the scope requests %d in all, want 5", n)
	}
	if s, err := c.Scope(t.Context()); err != nil || s.Environment != scopeEnvironmentID {
		t.Errorf("Scope after the lookup returned %+v, %v; want the scope it keeps", s, err)
	}
}

// errorBody returns an error body of the API with the code and the message.
func errorBody(code, message string) string {
	return `{"error_code":"` + code + `","title":"` + code + `","message":"` + message +
		`","id":"req-1"}`
}

// TestScopeErrors checks that each reply and failure of the lookup reaches the caller as the API
// answered it, both from Scope and from a call that needs the scope, which then sends nothing
// else.
func TestScopeErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		sentinel error
	}{
		{"401", http.StatusUnauthorized, ErrUnauthorized},
		{"403", http.StatusForbidden, ErrForbidden},
		{"429", http.StatusTooManyRequests, ErrRateLimited},
		{"500", http.StatusInternalServerError, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := errorBody("CODE_"+tc.name, "the lookup failed")
			reply := mockpermit.Reply{Status: tc.status, Body: body}
			api := mockpermit.NewAPI(t)
			api.Handle(scopeRoute, reply, reply)
			c := scopeClient(t, api.URL, WithRetry(RetryPolicy{}))
			want := &Error{
				Op: opGetAPIKeyScope, Method: http.MethodGet, Route: "/v2/api-key/scope",
				StatusCode: tc.status, Code: "CODE_" + tc.name, Title: "CODE_" + tc.name,
				Detail: "the lookup failed", RequestID: "req-1", Body: []byte(body),
			}
			_, scopeErr := c.Scope(t.Context())
			_, callErr := getInEnvironment(t.Context(), c, "a")
			for name, err := range map[string]error{"Scope": scopeErr, "the call": callErr} {
				if diff := cmp.Diff(want, mustError(t, err)); diff != "" {
					t.Errorf("%s returned an Error that differs (-want +got):\n%s", name, diff)
				}
				if tc.sentinel != nil && !errors.Is(err, tc.sentinel) {
					t.Errorf("%s returned %v, which does not match %v", name, err, tc.sentinel)
				}
			}
		})
	}
}

// TestScopeConnectionRefused checks that a lookup whose connection fails returns a *Error with
// status 0 that wraps the *url.Error, and that the call that needed it sends nothing else.
func TestScopeConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	c := scopeClient(t, srv.URL, WithRetry(RetryPolicy{}))
	for name, call := range map[string]func() error{
		"Scope":    func() error { _, err := c.Scope(t.Context()); return err },
		"the call": func() error { _, err := getInEnvironment(t.Context(), c, "a"); return err },
	} {
		err := call()
		e := mustError(t, err)
		_, isURLError := errors.AsType[*url.Error](err)
		if e.Op != opGetAPIKeyScope || e.StatusCode != 0 || !isURLError {
			t.Errorf("%s returned %v, want the lookup's *Error with status 0 that wraps a "+
				"*url.Error", name, err)
		}
	}
}

// TestScopeWithACanceledContext checks that a call whose context has ended returns a *Error of
// the lookup that matches context.Canceled, and sends no request: it runs in a synctest bubble,
// so that a lookup that the calls started would have sent its request by the time every
// goroutine of the bubble waits.
func TestScopeWithACanceledContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		counter := &countingTransport{}
		c := scopeClient(t, codeURL, WithHTTPClient(&http.Client{Transport: counter}))
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, scopeErr := c.Scope(ctx)
		_, callErr := getInEnvironment(ctx, c, "a")
		for name, err := range map[string]error{"Scope": scopeErr, "the call": callErr} {
			e := mustError(t, err)
			if !errors.Is(err, context.Canceled) || e.Op != opGetAPIKeyScope ||
				e.Route != "/v2/api-key/scope" {
				t.Errorf("%s returned %v, want the lookup's *Error that matches "+
					"context.Canceled", name, err)
			}
		}
		synctest.Wait()
		if n := counter.requests.Load(); n != 0 {
			t.Errorf("the calls with an ended context sent %d requests, want none", n)
		}
	})
}

// blockingScopeServer starts a server whose scope route answers only once release is closed, or
// fails the request when its context ends, and that sends each scope request it gets to arrived.
// Every other route answers an object.
func blockingScopeServer(t *testing.T) (srv *httptest.Server, arrived <-chan struct{},
	release chan<- struct{},
) {
	t.Helper()
	in := make(chan struct{}, 8)
	out := make(chan struct{})
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/api-key/scope" {
			_, _ = io.WriteString(w, `{"key":"a"}`)
			return
		}
		in <- struct{}{}
		select {
		case <-out:
			_, _ = io.WriteString(w, `{"organization_id":"o","project_id":"p",`+
				`"environment_id":"e"}`)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	return srv, in, out
}

// TestScopeCanceledCallerLeavesTheLookupToOthers cancels the call that started the lookup while
// another call waits for it: the canceled call returns context.Canceled at once, before the
// server answers, and the other call gets the scope of the one lookup.
func TestScopeCanceledCallerLeavesTheLookupToOthers(t *testing.T) {
	leak.Check(t)
	srv, arrived, release := blockingScopeServer(t)
	c := scopeClient(t, srv.URL)

	ctx, cancel := context.WithCancel(t.Context())
	first := make(chan error, 1)
	go func() {
		_, err := c.Scope(ctx)
		first <- err
	}()
	<-arrived
	second := make(chan error, 1)
	go func() {
		_, err := getInEnvironment(t.Context(), c, "a")
		second <- err
	}()
	cancel()
	select {
	case err := <-first:
		if e := mustError(t, err); !errors.Is(err, context.Canceled) ||
			e.Op != opGetAPIKeyScope {
			t.Errorf("the canceled call returned %v, want the lookup's *Error that matches "+
				"context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the canceled call still waits for the lookup")
	}
	close(release)
	if err := <-second; err != nil {
		t.Errorf("the call that still waited returned %v, want the object", err)
	}
	if s, err := c.Scope(t.Context()); err != nil || s != (Scope{Project: "p", Environment: "e"}) {
		t.Errorf("Scope after the lookup returned %+v, %v; want the scope of the lookup", s, err)
	}
	select {
	case <-arrived:
		t.Error("the server got a second scope request")
	default:
	}
}

// TestCloseEndsTheScopeLookup closes the client while a call waits for the lookup: the call
// returns ErrClosed, the lookup's request ends, and no goroutine of it keeps running.
func TestCloseEndsTheScopeLookup(t *testing.T) {
	leak.Check(t)
	srv, arrived, _ := blockingScopeServer(t)
	c := scopeClient(t, srv.URL)
	waiting := make(chan error, 1)
	go func() {
		_, err := c.Scope(t.Context())
		waiting <- err
	}()
	<-arrived
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-waiting:
		// The error is ErrClosed, as for a call that starts after Close, and not the *Error of
		// the request that Close ended.
		if _, isError := errors.AsType[*Error](err); isError || !errors.Is(err, ErrClosed) {
			t.Errorf("the call that waited at Close returned %v, want ErrClosed", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not end the lookup")
	}
	if _, err := c.Scope(t.Context()); !errors.Is(err, ErrClosed) {
		t.Errorf("Scope after Close returned %v, want ErrClosed", err)
	}
	if _, _, err := c.scopeFor(t.Context(), opInEnvironment.ID); !errors.Is(err, ErrClosed) {
		t.Errorf("scopeFor after Close returned %v, want ErrClosed", err)
	}
}

// checkScopeError checks that err is the error of a call of op whose scope lacks what rule
// names: an error that matches ErrInvalidConfig and names the operation, the missing part and
// WithScope.
func checkScopeError(t *testing.T, err error, op rest.Op, rule validate.Rule, part string) {
	t.Helper()
	ce, ok := errors.AsType[*configError](err)
	if !ok || !errors.Is(err, ErrInvalidConfig) || ce.rule != rule || ce.setting != op.ID {
		t.Fatalf("the call of %s returned %v, want the error of the rule %s", op.ID, err,
			rule.ID)
	}
	for _, want := range []string{op.ID, part, "WithScope(project, environment)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q does not name %q", err, want)
		}
	}
}

// TestScopeOfAProjectKey checks the calls of a client whose key is a project-level key: a call
// that acts on an environment fails before its request with an error that names WithScope, and
// succeeds once the client has what the error names; a call that acts on the project gets the
// project and no environment, and one of the organization needs no scope.
func TestScopeOfAProjectKey(t *testing.T) {
	api := mockpermit.NewAPI(t)
	api.Handle(scopeRoute, scopeReply(t, ".project"))
	api.Handle(route(opInProject), mockpermit.Reply{Body: `{"key":"a"}`})
	api.Handle(route(opProjects), mockpermit.Reply{Body: `[{"key":"p"}]`})
	c := scopeClient(t, api.URL)

	_, err := getInEnvironment(t.Context(), c, "a")
	checkScopeError(t, err, opInEnvironment, validate.ScopeEnvironmentMissing, "environment")
	project, environment, err := c.scopeFor(t.Context(), opInProject.ID)
	if err != nil || project != scopeProjectID || environment != "" {
		t.Errorf("scopeFor of a project-level operation returned %q, %q, %v; want the project "+
			"alone", project, environment, err)
	}
	if got, err := getInProject(t.Context(), c, "a"); err != nil || got.Key != "a" {
		t.Errorf("the call of the project returned %+v, %v; want the object", got, err)
	}
	if err := listProjects(t.Context(), c); err != nil {
		t.Errorf("the call of the organization returned %v", err)
	}
	if n := countRequests(api, "/v2/api-key/scope"); n != 1 || len(api.Requests()) != 3 {
		t.Errorf("the calls sent %d requests, %d of the scope; want 3: one scope request, the "+
			"project's and the organization's, and none of the environment",
			len(api.Requests()), n)
	}

	// The remedy that the error names: the same call with WithScope(project, environment).
	fixed := mockpermit.NewAPI(t)
	fixed.Handle(route(opInEnvironment), mockpermit.Reply{Body: `{"key":"a"}`})
	c = scopeClient(t, fixed.URL, WithScope(scopeProjectID, "production"))
	if got, err := getInEnvironment(t.Context(), c, "a"); err != nil || got.Key != "a" {
		t.Fatalf("the call with WithScope returned %+v, %v; want the object", got, err)
	}
	want := "/__test/facts/" + scopeProjectID + "/production/objects/a"
	if requests := fixed.Requests(); len(requests) != 1 || requests[0].RawPath != want {
		t.Errorf("the call with WithScope sent %d requests, want one to %s and no lookup",
			len(requests), want)
	}
}

// TestScopeOfAnOrganizationKey checks the calls of a client whose key is an organization-level
// key, whose scope the API answers with project_id and environment_id null, or without them:
// both decode as no project and no environment; the calls of a project or an environment fail
// before their requests, and succeed with WithScope as the error names it; and a call of the
// organization needs no scope.
func TestScopeOfAnOrganizationKey(t *testing.T) {
	for _, suffix := range []string{".organization", ".organization-null"} {
		t.Run(suffix[1:], func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(scopeRoute, scopeReply(t, suffix))
			api.Handle(route(opProjects), mockpermit.Reply{Body: `[{"key":"p"}]`})
			c := scopeClient(t, api.URL)

			if s, err := c.Scope(t.Context()); err != nil || s != (Scope{}) {
				t.Errorf("Scope returned %+v, %v; want no project and no environment", s, err)
			}
			_, err := getInEnvironment(t.Context(), c, "a")
			checkScopeError(t, err, opInEnvironment, validate.ScopeProjectMissing, "project")
			_, err = getInProject(t.Context(), c, "a")
			checkScopeError(t, err, opInProject, validate.ScopeProjectMissing, "project")
			if err := listProjects(t.Context(), c); err != nil {
				t.Errorf("the call of the organization returned %v", err)
			}
			if n := len(api.Requests()); n != 2 {
				t.Errorf("the calls sent %d requests, want 2: the lookup and the organization's",
					n)
			}

			fixed := mockpermit.NewAPI(t)
			fixed.Handle(route(opInEnvironment), mockpermit.Reply{Body: `{"key":"a"}`})
			c = scopeClient(t, fixed.URL, WithScope("my-project", "production"))
			if got, err := getInEnvironment(t.Context(), c, "a"); err != nil || got.Key != "a" {
				t.Errorf("the call with WithScope returned %+v, %v; want the object", got, err)
			}
		})
	}
}

// TestScopeOfAnEnvironmentKey checks that a call of an environment gets the project and the
// environment of an environment-level key, and a call of its project the project alone.
func TestScopeOfAnEnvironmentKey(t *testing.T) {
	api := mockpermit.NewAPI(t)
	api.Handle(scopeRoute, scopeReply(t, ""))
	api.Handle(route(opInEnvironment), mockpermit.Reply{Body: `{"key":"a"}`})
	c := scopeClient(t, api.URL)
	if _, err := getInEnvironment(t.Context(), c, "a"); err != nil {
		t.Fatal(err)
	}
	want := "/__test/facts/" + scopeProjectID + "/" + scopeEnvironmentID + "/objects/a"
	if got := api.Requests()[1].RawPath; got != want {
		t.Errorf("the call went to %s, want %s", got, want)
	}
	project, environment, err := c.scopeFor(t.Context(), opInProject.ID)
	if err != nil || project != scopeProjectID || environment != "" {
		t.Errorf("scopeFor of a project-level operation returned %q, %q, %v; want the project "+
			"alone", project, environment, err)
	}
}

// TestOrganizationCallNeedsNoScope checks that a call of an operation of the organization looks
// nothing up and checks no scope, with any key: the server serves no scope route, so a lookup
// would fail the test.
func TestOrganizationCallNeedsNoScope(t *testing.T) {
	api := mockpermit.NewAPI(t)
	api.Handle(route(opProjects), mockpermit.Reply{Body: `{"data":[{"key":"p"}]}`})
	c := scopeClient(t, api.URL)
	project, environment, err := c.scopeFor(t.Context(), opProjects.ID)
	if err != nil || project != "" || environment != "" {
		t.Errorf("scopeFor of an organization-level operation returned %q, %q, %v; want "+
			"nothing", project, environment, err)
	}
	if err := listProjects(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != "/__test/projects" {
		t.Errorf("the call sent %d requests, want only its own", len(requests))
	}
}

// TestScopeComesFromTheRoute checks that a call needs the parts of the scope that its route
// fills, whatever the KeyLevel of its operation: a call whose route fills {proj_id} alone needs
// no environment, also when an environment-level key may call it, and the scope of a project key
// serves it; a call whose route fills neither looks nothing up, also when its operation is
// environment-level, as get_api_key_scope is.
func TestScopeComesFromTheRoute(t *testing.T) {
	api := mockpermit.NewAPI(t)
	api.Handle(scopeRoute, scopeReply(t, ".project"))
	api.Handle(route(opEnvironments), mockpermit.Reply{Body: `[{"key":"production"}]`})
	c := scopeClient(t, api.URL)
	if err := listEnvironments(t.Context(), c); err != nil {
		t.Fatalf("the call whose route fills {proj_id} alone returned %v with a project key", err)
	}
	want := "/__test/projects/" + scopeProjectID + "/environments"
	if requests := api.Requests(); len(requests) != 2 || requests[1].RawPath != want {
		t.Errorf("the call sent %d requests, want the lookup and one to %s", len(requests), want)
	}

	unscoped := mockpermit.NewAPI(t)
	c = scopeClient(t, unscoped.URL)
	project, environment, err := c.scopeFor(t.Context(), opGetAPIKeyScope)
	if err != nil || project != "" || environment != "" {
		t.Errorf("scopeFor of an environment-level operation whose route has no {proj_id} and "+
			"no {env_id} returned %q, %q, %v; want nothing", project, environment, err)
	}
	unscoped.AssertUntouched()
}

// TestWithScopeSkipsTheLookup checks that a client with WithScope sends no lookup: Scope returns
// the scope the option gives, also with a context that has ended, and a call of a project gets
// the project alone. After Close, Scope returns ErrClosed, as it does without WithScope.
func TestWithScopeSkipsTheLookup(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := scopeClient(t, api.URL, WithScope("my-project", "production"))
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	for _, ctx := range []context.Context{t.Context(), ended} {
		if s, err := c.Scope(ctx); err != nil ||
			s != (Scope{Project: "my-project", Environment: "production"}) {
			t.Errorf("Scope returned %+v, %v; want what WithScope gives", s, err)
		}
	}
	project, environment, err := c.scopeFor(t.Context(), opInProject.ID)
	if err != nil || project != "my-project" || environment != "" {
		t.Errorf("scopeFor of a project-level operation returned %q, %q, %v; want the project "+
			"alone", project, environment, err)
	}

	onlyProject := scopeClient(t, api.URL, WithScope("my-project", ""))
	_, err = getInEnvironment(t.Context(), onlyProject, "a")
	checkScopeError(t, err, opInEnvironment, validate.ScopeEnvironmentMissing, "environment")
	api.AssertUntouched()

	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if s, err := c.Scope(t.Context()); !errors.Is(err, ErrClosed) {
		t.Errorf("Scope after Close returned %+v, %v; want ErrClosed", s, err)
	}
}

// TestScopeForAnUnknownOperation checks that scopeFor names an operation that the registry does
// not hold, and looks nothing up.
func TestScopeForAnUnknownOperation(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := scopeClient(t, api.URL)
	_, _, err := c.scopeFor(t.Context(), "no_such_op")
	want := "permit: the operation no_such_op is not in the client's operation registry"
	if err == nil || err.Error() != want {
		t.Errorf("scopeFor of an unknown operation returned %v, want %q", err, want)
	}
	api.AssertUntouched()
}
