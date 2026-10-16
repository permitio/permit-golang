package permit

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The routes of the projects, as mockpermit.Server.Handle takes them.
const (
	listProjectsRoute  = "GET /v2/projects"
	createProjectRoute = "POST /v2/projects"
	getProjectRoute    = "GET /v2/projects/{proj_id}"
	updateProjectRoute = "PATCH /v2/projects/{proj_id}"
	deleteProjectRoute = "DELETE /v2/projects/{proj_id}"
)

// projectBody is the body of a reply of the API that holds the project my-project.
const projectBody = `{"key":"my-project","id":"00000000-0000-4000-8000-000000000002",` +
	`"organization_id":"00000000-0000-4000-8000-000000000001",` +
	`"created_at":"2026-10-14T09:30:00Z","updated_at":"2026-10-14T09:30:00Z","name":"My project"}`

// projectsOf returns the projects p<from> to p<to-1>, with only a key and a name: an empty list
// when to is from.
func projectsOf(from, to int) []models.ProjectRead {
	items := make([]models.ProjectRead, 0, to-from)
	for i := from; i < to; i++ {
		key := "p" + strconv.Itoa(i)
		items = append(items, models.ProjectRead{Key: key, Name: key})
	}
	return items
}

// projectClient returns a client of api without WithScope and with opts, whose PDP is a server
// that fails the test when anything reaches it.
func projectClient(t *testing.T, api *mockpermit.Server, opts ...Option) *Client {
	t.Helper()
	pdp := mockpermit.NewPDP(t)
	t.Cleanup(pdp.AssertUntouched)
	return decisionClient(t, api, pdp, opts...)
}

// TestProjectsAllReadsEveryPage checks that ProjectsService.All reads the 31 projects of a list,
// in order, asking for each page in turn: at a per_page of 30, two pages of projects and the
// empty page after them; at the default per_page of 100, the most that the API gives on a page,
// one page of 31 and the empty page after it; and from page 2 on, when the options start there.
func TestProjectsAllReadsEveryPage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    *ListOptions
		replies [][]models.ProjectRead
		want    []models.ProjectRead
		queries []string
	}{
		{
			name:    "per_page 30",
			opts:    &ListOptions{PerPage: 30},
			replies: [][]models.ProjectRead{projectsOf(0, 30), projectsOf(30, 31), {}},
			want:    projectsOf(0, 31),
			queries: []string{
				"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30",
			},
		},
		{
			name:    "the default per_page",
			replies: [][]models.ProjectRead{projectsOf(0, 31), {}},
			want:    projectsOf(0, 31),
			queries: []string{"page=1&per_page=100", "page=2&per_page=100"},
		},
		{
			name:    "from page 2",
			opts:    &ListOptions{Page: 2, PerPage: 30},
			replies: [][]models.ProjectRead{projectsOf(30, 31), {}},
			want:    projectsOf(30, 31),
			queries: []string{"page=2&per_page=30", "page=3&per_page=30"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			var replies []mockpermit.Reply
			for _, items := range tc.replies {
				replies = append(replies, arrayOf(t, items))
			}
			api.Handle(listProjectsRoute, replies...)
			items, errs := collect(t, projectClient(t, api).Projects.All(t.Context(), tc.opts))
			if diff := cmp.Diff(tc.want, items); diff != "" || errs != nil {
				t.Errorf("All yielded the errors %v and projects that differ (-want +got):\n%s",
					errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestProjectsAllReadsTheOptionsOnce checks that the iterator keeps the options of its call, so
// that a later change to them changes no request, that a loop that breaks asks for no other page,
// and that each loop over the iterator reads the projects again from the first page.
func TestProjectsAllReadsTheOptionsOnce(t *testing.T) {
	api := mockpermit.NewAPI(t)
	two := arrayOf(t, projectsOf(0, 2))
	api.Handle(listProjectsRoute, two, two)
	opts := &ListOptions{PerPage: 2}
	seq := projectClient(t, api).Projects.All(t.Context(), opts)
	opts.Page, opts.PerPage = 5, 50
	for loop := 1; loop <= 2; loop++ {
		for project, err := range seq {
			if err != nil || project.Key != "p0" {
				t.Errorf("loop %d yielded %+v and %v first, want p0", loop, project, err)
			}
			break
		}
	}
	first := "page=1&per_page=2"
	if diff := cmp.Diff([]string{first, first}, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestProjectsAllYieldsAnErrorOnce checks that a page that fails ends the iterator with its
// error, once, after the projects of the pages before it.
func TestProjectsAllYieldsAnErrorOnce(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(listProjectsRoute, arrayOf(t, projectsOf(0, 30)),
		mockpermit.Reply{Status: http.StatusForbidden, Body: body})
	items, errs := collect(t, projectClient(t, api).Projects.All(t.Context(),
		&ListOptions{PerPage: 30}))
	if len(items) != 30 || len(errs) != 1 || !errors.Is(errs[0], ErrForbidden) {
		t.Errorf("All yielded %d projects and the errors %v, want 30 and one error that matches "+
			"ErrForbidden", len(items), errs)
	}
}

// TestProjectsListReturnsAPage checks the page that List returns: the projects of the reply in
// its order, the page and the per_page that it asked for, page 1 and no query for nil options,
// and no counts, as the reply is an array. A page without projects has an empty list, not nil.
func TestProjectsListReturnsAPage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  *ListOptions
		reply []models.ProjectRead
		query string
		want  *Page[models.ProjectRead]
	}{
		{
			name: "nil options", reply: projectsOf(0, 2), query: "",
			want: &Page[models.ProjectRead]{Data: projectsOf(0, 2), Page: 1},
		},
		{
			name: "a page after the last", opts: &ListOptions{Page: 3, PerPage: 30},
			reply: []models.ProjectRead{}, query: "page=3&per_page=30",
			want: &Page[models.ProjectRead]{Data: []models.ProjectRead{}, Page: 3, PerPage: 30},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listProjectsRoute, arrayOf(t, tc.reply))
			got, err := projectClient(t, api).Projects.List(t.Context(), tc.opts)
			if err != nil {
				t.Fatalf("List returned %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" || got.Data == nil {
				t.Errorf("List returned a page that differs (-want +got), or nil Data:\n%s", diff)
			}
			if diff := cmp.Diff([]string{tc.query}, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestInvalidProjectListOptions checks that a Page or a PerPage less than zero is an error that
// matches ErrInvalidConfig from List and All, and that neither sends a request.
func TestInvalidProjectListOptions(t *testing.T) {
	for _, opts := range []*ListOptions{{Page: -1}, {PerPage: -1}} {
		api := mockpermit.NewAPI(t)
		c := projectClient(t, api)
		_, err := c.Projects.List(t.Context(), opts)
		_, errs := collect(t, c.Projects.All(t.Context(), opts))
		for _, err := range append([]error{err}, errs...) {
			if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("%+v: the call returned %v, want an error that matches ErrInvalidConfig",
					*opts, err)
			}
		}
		if len(errs) != 1 {
			t.Errorf("%+v: All yielded the errors %v, want one", *opts, errs)
		}
		api.AssertUntouched()
	}
}

// TestProjectsNeedNoScope checks that the calls of the projects look up no scope and check none:
// with an organization-level API key, whose scope has no project and no environment, a client
// without WithScope lists, gets, creates, changes and deletes projects, and sends no request but
// the call's, as the API fails each request to a route that the test did not register, such as
// the lookup of the scope. With a scope that the client looked up first, that of an
// organization-level key and that of an environment-level key, and with an environment's scope
// from WithScope, a call sends its request as it is too, and the API decides what the key may
// do.
func TestProjectsNeedNoScope(t *testing.T) {
	calls := func(t *testing.T, c *Client) {
		t.Helper()
		ctx := t.Context()
		if _, err := c.Projects.List(ctx, nil); err != nil {
			t.Errorf("List returned %v", err)
		}
		if _, errs := collect(t, c.Projects.All(ctx, nil)); errs != nil {
			t.Errorf("All yielded %v", errs)
		}
		if _, err := c.Projects.Get(ctx, "my-project"); err != nil {
			t.Errorf("Get returned %v", err)
		}
		if _, err := c.Projects.Create(ctx, models.ProjectCreate{Key: "k", Name: "n"}); err != nil {
			t.Errorf("Create returned %v", err)
		}
		if _, err := c.Projects.Update(ctx, "my-project", models.ProjectUpdate{}); err != nil {
			t.Errorf("Update returned %v", err)
		}
		if err := c.Projects.Delete(ctx, "my-project"); err != nil {
			t.Errorf("Delete returned %v", err)
		}
	}
	handle := func(api *mockpermit.Server) {
		empty := mockpermit.Reply{Body: `[]`}
		project := mockpermit.Reply{Body: projectBody}
		api.Handle(listProjectsRoute, empty, empty)
		api.Handle(getProjectRoute, project)
		api.Handle(createProjectRoute, project)
		api.Handle(updateProjectRoute, project)
		api.Handle(deleteProjectRoute, mockpermit.Reply{Status: http.StatusNoContent})
	}
	t.Run("no lookup", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		handle(api)
		calls(t, projectClient(t, api))
	})
	for _, tc := range []struct {
		name, fixture string
		want          Scope
	}{
		{
			name: "the scope of an organization-level key", want: Scope{},
			fixture: "get_api_key_scope.200.organization.json",
		},
		{
			name: "the scope of an environment-level key", fixture: "get_api_key_scope.200.json",
			want: Scope{
				Project:     "00000000-0000-4000-8000-000000000002",
				Environment: "00000000-0000-4000-8000-000000000003",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			handle(api)
			api.Handle("GET /v2/api-key/scope",
				mockpermit.Reply{Body: replyFixtureOf(t, tc.fixture)})
			c := projectClient(t, api)
			if s, err := c.Scope(t.Context()); err != nil || s != tc.want {
				t.Fatalf("Scope returned %+v, %v; want %+v", s, err, tc.want)
			}
			calls(t, c)
		})
	}
	t.Run("an environment's scope from WithScope", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		handle(api)
		calls(t, projectClient(t, api, WithScope("p", "e")))
	})
}

// TestProjectKeysInThePath checks that each method that names a project sends its key as one
// segment of the path, escaped, whatever the key holds: a slash, a percent sign, a space, a
// question mark and a hash, a letter outside ASCII, the shape of a UUID and a leading dot. A key
// that is empty, . or .. would change the route, so it is an error that matches
// ErrInvalidConfig and names the placeholder, and no request.
func TestProjectKeysInThePath(t *testing.T) {
	methods := []struct {
		name  string
		route string
		reply mockpermit.Reply
		call  func(c *Client, key string) error
	}{
		{
			name: "Get", route: getProjectRoute, reply: mockpermit.Reply{Body: projectBody},
			call: func(c *Client, key string) error {
				_, err := c.Projects.Get(t.Context(), key)
				return err
			},
		},
		{
			name: "Update", route: updateProjectRoute, reply: mockpermit.Reply{Body: projectBody},
			call: func(c *Client, key string) error {
				_, err := c.Projects.Update(t.Context(), key, models.ProjectUpdate{Name: "n"})
				return err
			},
		},
		{
			name: "Delete", route: deleteProjectRoute,
			reply: mockpermit.Reply{Status: http.StatusNoContent},
			call:  func(c *Client, key string) error { return c.Projects.Delete(t.Context(), key) },
		},
	}
	for key, want := range map[string]string{
		"a/b":              "/v2/projects/a%2Fb",
		"50%":              "/v2/projects/50%25",
		"a b?c#d":          "/v2/projects/a%20b%3Fc%23d",
		"é":                "/v2/projects/%C3%A9",
		fixtures.UUIDKey:   "/v2/projects/" + fixtures.UUIDKey,
		".hidden":          "/v2/projects/.hidden",
		"my-project.v2..x": "/v2/projects/my-project.v2..x",
	} {
		for _, m := range methods {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, m.reply)
			if err := m.call(projectClient(t, api), key); err != nil {
				t.Errorf("%s(%q) returned %v", m.name, key, err)
			}
			if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want {
				t.Errorf("%s(%q) sent %+v, want one request to %s", m.name, key, requests, want)
			}
		}
	}
	for _, key := range []string{"", ".", ".."} {
		for _, m := range methods {
			api := mockpermit.NewAPI(t)
			err := m.call(projectClient(t, api), key)
			if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), "{proj_id}") {
				t.Errorf("%s(%q) returned %v, want an error that matches ErrInvalidConfig and "+
					"names {proj_id}", m.name, key, err)
			}
			api.AssertUntouched()
		}
	}
}

// TestDeleteProjectThatIsGone checks that Delete returns nil for a 404, with or without a body,
// as the project that the path names is gone already, after one request; and an *Error with the
// status for any other error status, such as 403, 409 or 422.
func TestDeleteProjectThatIsGone(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"NOT_FOUND","title":"Not Found","message":"the project does not exist"}`,
		"",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteProjectRoute, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		if err := projectClient(t, api).Projects.Delete(t.Context(), "gone"); err != nil {
			t.Errorf("Delete of a project that the API answers with 404 and the body %q "+
				"returned %v, want nil", body, err)
		}
		if n := len(api.Requests()); n != 1 {
			t.Errorf("Delete sent %d requests, want 1", n)
		}
	}
	for _, status := range []int{
		http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteProjectRoute, mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := projectClient(t, api).Projects.Delete(t.Context(), "my-project")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteProject ||
			e.Detail != "no" || errors.Is(err, ErrNotFound) {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status and the detail", status, err, opDeleteProject)
		}
	}
}

// TestProjectBodies checks the bodies of Create and Update byte for byte against the golden
// files in testdata/golden: a project with the required fields only; one with every field, an
// initial environment with every field and text that an encoder may escape; one with an empty
// list of initial environments, which the API reads as no environments; the zero update, which
// changes nothing; an update that sends null for the description; and one that sets each field,
// the empty description and empty settings among them.
func TestProjectBodies(t *testing.T) {
	every := models.ProjectCreate{
		Key: "p", URNNamespace: "ns", Name: "<P> & Q",
		Description:        "a" + fixtures.LineSeparator + "b",
		Settings:           models.Attributes{"n": fixtures.TwoPow53Plus1},
		ActivePolicyRepoID: fixtures.UUIDKey,
		InitialEnvironments: []models.EnvironmentCreate{{
			Key: "e", Name: "E", Description: "d", CustomBranchName: "b",
			JWKS: &models.JwksConfig{
				Audience: models.OneString("aud"), Issuer: models.StringList("i1", "i2"),
				JWKS: &models.JwksObj{Keys: []models.Attributes{{"kid": "k"}}}, TTL: 60,
				URL: "https://example.com/jwks",
			},
			Settings: models.Attributes{},
		}},
	}
	create := func(project models.ProjectCreate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.Projects.Create(t.Context(), project)
			return err
		}
	}
	update := func(change models.ProjectUpdate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.Projects.Update(t.Context(), "p", change)
			return err
		}
	}
	for _, tc := range []struct {
		golden string
		route  string
		call   func(c *Client) error
	}{
		{
			"create_project.required.golden", createProjectRoute,
			create(models.ProjectCreate{Key: "p", Name: "P"}),
		},
		{"create_project.every-field.golden", createProjectRoute, create(every)},
		{
			"create_project.no-environments.golden", createProjectRoute,
			create(models.ProjectCreate{
				Key: "p", Name: "P", InitialEnvironments: []models.EnvironmentCreate{},
			}),
		},
		{"update_project.unset.golden", updateProjectRoute, update(models.ProjectUpdate{})},
		{
			"update_project.null.golden", updateProjectRoute,
			update(models.ProjectUpdate{Description: models.Null[string]()}),
		},
		{
			"update_project.values.golden", updateProjectRoute,
			update(models.ProjectUpdate{
				Name: "P", Description: models.Some(""), Settings: models.Attributes{},
				ActivePolicyRepoID: fixtures.UUIDKey,
			}),
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: projectBody})
			if err := tc.call(projectClient(t, api)); err != nil {
				t.Fatalf("the call returned %v", err)
			}
			requests := api.Requests()
			if len(requests) != 1 {
				t.Fatalf("the call sent %d requests, want 1", len(requests))
			}
			requests[0].ExpectBytes(t, filepath.Join("testdata", "golden", tc.golden))
		})
	}
}

// TestProjectRetries checks when a call of the projects sends its request again: Create, which
// the API may have acted on, after a 429 only, so a 503 ends it after one request; Update, Get
// and Delete, which do the same when sent twice, after a 503 too.
func TestProjectRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	ok := mockpermit.Reply{Body: projectBody}
	for _, tc := range []struct {
		name    string
		route   string
		replies []mockpermit.Reply
		call    func(c *Client) error
		status  int
	}{
		{
			name: "Create after a 429", route: createProjectRoute,
			replies: []mockpermit.Reply{limited, ok},
			call: func(c *Client) error {
				_, err := c.Projects.Create(t.Context(), models.ProjectCreate{Key: "k", Name: "n"})
				return err
			},
		},
		{
			name: "Create after a 503", route: createProjectRoute,
			replies: []mockpermit.Reply{unavailable},
			call: func(c *Client) error {
				_, err := c.Projects.Create(t.Context(), models.ProjectCreate{Key: "k", Name: "n"})
				return err
			},
			status: http.StatusServiceUnavailable,
		},
		{
			name: "Update after a 503", route: updateProjectRoute,
			replies: []mockpermit.Reply{unavailable, ok},
			call: func(c *Client) error {
				_, err := c.Projects.Update(t.Context(), "p", models.ProjectUpdate{Name: "n"})
				return err
			},
		},
		{
			name: "Get after a 503", route: getProjectRoute,
			replies: []mockpermit.Reply{unavailable, ok},
			call: func(c *Client) error {
				_, err := c.Projects.Get(t.Context(), "p")
				return err
			},
		},
		{
			name: "Delete after a 503", route: deleteProjectRoute,
			replies: []mockpermit.Reply{unavailable, {Status: http.StatusNoContent}},
			call:    func(c *Client) error { return c.Projects.Delete(t.Context(), "p") },
		},
		{
			name: "List after a 503", route: listProjectsRoute,
			replies: []mockpermit.Reply{unavailable, {Body: `[]`}},
			call: func(c *Client) error {
				_, err := c.Projects.List(t.Context(), nil)
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, tc.replies...)
			err := tc.call(projectClient(t, api, WithRetry(RetryPolicy{MaxRetries: 1})))
			if tc.status == 0 && err != nil {
				t.Errorf("the call returned %v, want the result of the second request", err)
			}
			if tc.status != 0 && mustError(t, err).StatusCode != tc.status {
				t.Errorf("the call returned %v, want the error of status %d", err, tc.status)
			}
			if n := len(api.Requests()); n != len(tc.replies) {
				t.Errorf("the call sent %d requests, want %d", n, len(tc.replies))
			}
		})
	}
}

// TestMalformedProjectReplies checks that a reply in 200-299 that is not a project, or for List
// not a JSON array of projects, is an *Error that names the model, and no project.
func TestMalformedProjectReplies(t *testing.T) {
	const prefix = "permit: "
	for _, body := range []string{``, `null`, `[]`, `"p"`, `{"key":1}`, `{} {}`} {
		api := mockpermit.NewAPI(t)
		api.Handle(getProjectRoute, mockpermit.Reply{Body: body})
		project, err := projectClient(t, api).Projects.Get(t.Context(), "p")
		want := prefix + "get_project: GET /v2/projects/{proj_id}: status 200: the reply is not " +
			"a valid models.ProjectRead"
		if e := mustError(t, err); project != nil || e.Error() != want {
			t.Errorf("Get of the reply %q returned %+v and\n%v\nwant no project and\n%s", body,
				project, err, want)
		}
	}
	for _, body := range []string{``, `null`, `{}`, `{"data":[]}`, `[1]`, `[{"key":1}]`} {
		api := mockpermit.NewAPI(t)
		api.Handle(listProjectsRoute, mockpermit.Reply{Body: body})
		page, err := projectClient(t, api).Projects.List(t.Context(), nil)
		want := prefix + "list_projects: GET /v2/projects: status 200: the reply is not a valid " +
			"list of models.ProjectRead"
		if e := mustError(t, err); page != nil || e.Error() != want {
			t.Errorf("List of the reply %q returned %+v and\n%v\nwant no page and\n%s", body, page,
				err, want)
		}
	}
}

// TestClientEncodesNoService checks that a Client, whose fields hold its services, encodes as
// the empty JSON object, as its doc says.
func TestClientEncodesNoService(t *testing.T) {
	clearEnv(t)
	c := mustNew(t, WithAPIKey(canary.Key("json")))
	if data, err := json.Marshal(c); err != nil || string(data) != "{}" {
		t.Errorf("json.Marshal of a Client returned %s and %v, want {}", data, err)
	}
}

// TestProjectsAfterClose checks that each method returns ErrClosed after Close, and sends
// nothing.
func TestProjectsAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	_, listErr := c.Projects.List(ctx, nil)
	_, allErrs := collect(t, c.Projects.All(ctx, nil))
	_, getErr := c.Projects.Get(ctx, "p")
	_, createErr := c.Projects.Create(ctx, models.ProjectCreate{Key: "k", Name: "n"})
	_, updateErr := c.Projects.Update(ctx, "p", models.ProjectUpdate{})
	deleteErr := c.Projects.Delete(ctx, "p")
	for i, err := range append([]error{listErr, getErr, createErr, updateErr, deleteErr},
		allErrs...) {
		if !errors.Is(err, ErrClosed) {
			t.Errorf("call %d after Close returned %v, want ErrClosed", i, err)
		}
	}
	if len(allErrs) != 1 {
		t.Errorf("All after Close yielded %v, want ErrClosed once", allErrs)
	}
	api.AssertUntouched()
}
