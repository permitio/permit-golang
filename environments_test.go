package permit

import (
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The routes of the environments, as mockpermit.Server.Handle takes them.
const (
	listEnvironmentsRoute  = "GET /v2/projects/{proj_id}/envs"
	createEnvironmentRoute = "POST /v2/projects/{proj_id}/envs"
	getEnvironmentRoute    = "GET /v2/projects/{proj_id}/envs/{env_id}"
	updateEnvironmentRoute = "PATCH /v2/projects/{proj_id}/envs/{env_id}"
	deleteEnvironmentRoute = "DELETE /v2/projects/{proj_id}/envs/{env_id}"
	statsEnvironmentRoute  = "GET /v2/projects/{proj_id}/envs/{env_id}/stats"
	copyEnvironmentRoute   = "POST /v2/projects/{proj_id}/envs/{env_id}/copy"
	copyAsyncRoute         = "POST /v2/projects/{proj_id}/envs/{env_id}/copy/async"
	copyResultRoute        = "GET /v2/projects/{proj_id}/envs/{env_id}/copy/async/{task_id}/result"
)

// environmentBody is the body of a reply of the API that holds the environment production.
const environmentBody = `{"key":"production","id":"00000000-0000-4000-8000-000000000003",` +
	`"organization_id":"00000000-0000-4000-8000-000000000001",` +
	`"project_id":"00000000-0000-4000-8000-000000000002","created_at":"2026-10-14T09:30:00Z",` +
	`"updated_at":"2026-10-14T09:30:00Z","name":"Production",` +
	`"email_configuration":"00000000-0000-4000-8000-000000000007"}`

// environmentsOf returns the environments e<from> to e<to-1>, with only a key and a name: an
// empty list when to is from.
func environmentsOf(from, to int) []models.EnvironmentReadWithEmailConfig {
	items := make([]models.EnvironmentReadWithEmailConfig, 0, to-from)
	for i := from; i < to; i++ {
		key := "e" + strconv.Itoa(i)
		items = append(items, models.EnvironmentReadWithEmailConfig{
			EnvironmentRead: models.EnvironmentRead{Key: key, Name: key},
		})
	}
	return items
}

// TestEnvironmentsAllReadsEveryPage checks that EnvironmentsService.All reads the 31
// environments of a project, in order, asking for each page of the project in turn: at a
// per_page of 30, two pages of environments and the empty page after them; at the default
// per_page of 100, the most that the API gives on a page, one page of 31 and the empty page after
// it; and from page 2 on, when the options start there.
func TestEnvironmentsAllReadsEveryPage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    *ListOptions
		replies [][]models.EnvironmentReadWithEmailConfig
		want    []models.EnvironmentReadWithEmailConfig
		queries []string
	}{
		{
			name: "per_page 30",
			opts: &ListOptions{PerPage: 30},
			replies: [][]models.EnvironmentReadWithEmailConfig{
				environmentsOf(0, 30), environmentsOf(30, 31), {},
			},
			want: environmentsOf(0, 31),
			queries: []string{
				"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30",
			},
		},
		{
			name:    "the default per_page",
			replies: [][]models.EnvironmentReadWithEmailConfig{environmentsOf(0, 31), {}},
			want:    environmentsOf(0, 31),
			queries: []string{"page=1&per_page=100", "page=2&per_page=100"},
		},
		{
			name:    "from page 2",
			opts:    &ListOptions{Page: 2, PerPage: 30},
			replies: [][]models.EnvironmentReadWithEmailConfig{environmentsOf(30, 31), {}},
			want:    environmentsOf(30, 31),
			queries: []string{"page=2&per_page=30", "page=3&per_page=30"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			var replies []mockpermit.Reply
			for _, items := range tc.replies {
				replies = append(replies, arrayOf(t, items))
			}
			api.Handle(listEnvironmentsRoute, replies...)
			items, errs := collect(t,
				projectClient(t, api).Environments.All(t.Context(), "my-project", tc.opts))
			if diff := cmp.Diff(tc.want, items); diff != "" || errs != nil {
				t.Errorf("All yielded the errors %v and environments that differ "+
					"(-want +got):\n%s", errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
			for _, r := range api.Requests() {
				if r.RawPath != "/v2/projects/my-project/envs" {
					t.Errorf("All asked for a page of %s, want /v2/projects/my-project/envs",
						r.RawPath)
				}
			}
		})
	}
}

// TestEnvironmentsAllReadsTheOptionsOnce checks that the iterator keeps the project and the
// options of its call, so that a later change to the options changes no request, that a loop
// that breaks asks for no other page, and that each loop over the iterator reads the environments
// again from the first page.
func TestEnvironmentsAllReadsTheOptionsOnce(t *testing.T) {
	api := mockpermit.NewAPI(t)
	two := arrayOf(t, environmentsOf(0, 2))
	api.Handle(listEnvironmentsRoute, two, two)
	opts := &ListOptions{PerPage: 2}
	seq := projectClient(t, api).Environments.All(t.Context(), "p", opts)
	opts.Page, opts.PerPage = 5, 50
	for loop := 1; loop <= 2; loop++ {
		for environment, err := range seq {
			if err != nil || environment.Key != "e0" {
				t.Errorf("loop %d yielded %+v and %v first, want e0", loop, environment, err)
			}
			break
		}
	}
	first := "page=1&per_page=2"
	if diff := cmp.Diff([]string{first, first}, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestEnvironmentsAllYieldsAnErrorOnce checks that a page that fails ends the iterator with its
// error, once, after the environments of the pages before it.
func TestEnvironmentsAllYieldsAnErrorOnce(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(listEnvironmentsRoute, arrayOf(t, environmentsOf(0, 30)),
		mockpermit.Reply{Status: http.StatusForbidden, Body: body})
	items, errs := collect(t, projectClient(t, api).Environments.All(t.Context(), "p",
		&ListOptions{PerPage: 30}))
	if len(items) != 30 || len(errs) != 1 || !errors.Is(errs[0], ErrForbidden) {
		t.Errorf("All yielded %d environments and the errors %v, want 30 and one error that "+
			"matches ErrForbidden", len(items), errs)
	}
}

// TestEnvironmentsListReturnsAPage checks the page that List returns: the environments of the
// reply in its order, the page and the per_page that it asked for, page 1 and no query for nil
// options, and no counts, as the reply is an array. A page without environments has an empty
// list, not nil.
func TestEnvironmentsListReturnsAPage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  *ListOptions
		reply []models.EnvironmentReadWithEmailConfig
		query string
		want  *Page[models.EnvironmentReadWithEmailConfig]
	}{
		{
			name: "nil options", reply: environmentsOf(0, 2), query: "",
			want: &Page[models.EnvironmentReadWithEmailConfig]{
				Data: environmentsOf(0, 2), Page: 1,
			},
		},
		{
			name: "a page after the last", opts: &ListOptions{Page: 3, PerPage: 30},
			reply: environmentsOf(0, 0), query: "page=3&per_page=30",
			want: &Page[models.EnvironmentReadWithEmailConfig]{
				Data: environmentsOf(0, 0), Page: 3, PerPage: 30,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listEnvironmentsRoute, arrayOf(t, tc.reply))
			got, err := projectClient(t, api).Environments.List(t.Context(), "p", tc.opts)
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

// TestInvalidEnvironmentListOptions checks that a Page or a PerPage less than zero is an error
// that matches ErrInvalidConfig from List and All, and that neither sends a request.
func TestInvalidEnvironmentListOptions(t *testing.T) {
	for _, opts := range []*ListOptions{{Page: -1}, {PerPage: -1}} {
		api := mockpermit.NewAPI(t)
		c := projectClient(t, api)
		_, err := c.Environments.List(t.Context(), "p", opts)
		_, errs := collect(t, c.Environments.All(t.Context(), "p", opts))
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

// TestEnvironmentsNeedNoScope checks that the calls of the environments look up no scope and
// fill in none: a client without WithScope lists, gets, creates, changes, deletes and copies the
// environments of the project that the call names, reads their statistics and the task of a
// copy, and sends no request but the call's, as the API fails each request to a route that the
// test did not register, such as the lookup of the scope. With the scope of another project from
// WithScope, each call still sends the project that it names.
func TestEnvironmentsNeedNoScope(t *testing.T) {
	calls := func(t *testing.T, c *Client) {
		t.Helper()
		ctx := t.Context()
		if _, err := c.Environments.List(ctx, "my-project", nil); err != nil {
			t.Errorf("List returned %v", err)
		}
		if _, errs := collect(t, c.Environments.All(ctx, "my-project", nil)); errs != nil {
			t.Errorf("All yielded %v", errs)
		}
		if _, err := c.Environments.Get(ctx, "my-project", "production"); err != nil {
			t.Errorf("Get returned %v", err)
		}
		create := models.EnvironmentCreate{Key: "k", Name: "n"}
		if _, err := c.Environments.Create(ctx, "my-project", create); err != nil {
			t.Errorf("Create returned %v", err)
		}
		_, err := c.Environments.Update(ctx, "my-project", "production",
			models.EnvironmentUpdate{})
		if err != nil {
			t.Errorf("Update returned %v", err)
		}
		if err := c.Environments.Delete(ctx, "my-project", "production"); err != nil {
			t.Errorf("Delete returned %v", err)
		}
		if _, err := c.Environments.Stats(ctx, "my-project", "production"); err != nil {
			t.Errorf("Stats returned %v", err)
		}
		copyTo := models.EnvironmentCopy{TargetEnv: models.EnvironmentCopyTarget{Existing: "qa"}}
		if _, err := c.Environments.Copy(ctx, "my-project", "production", copyTo); err != nil {
			t.Errorf("Copy returned %v", err)
		}
		if _, err := c.Environments.CopyAsync(ctx, "my-project", "production", copyTo); err != nil {
			t.Errorf("CopyAsync returned %v", err)
		}
		_, resultErr := c.Environments.CopyResult(ctx, "my-project", "production", "t", nil)
		if resultErr != nil {
			t.Errorf("CopyResult returned %v", resultErr)
		}
	}
	for name, opts := range map[string][]Option{
		"no scope":                            nil,
		"an environment of another project":   {WithScope("other-project", "other-environment")},
		"another project without environment": {WithScope("other-project", "")},
	} {
		t.Run(name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			empty := mockpermit.Reply{Body: `[]`}
			environment := mockpermit.Reply{Body: environmentBody}
			api.Handle(listEnvironmentsRoute, empty, empty)
			api.Handle(getEnvironmentRoute, environment)
			api.Handle(createEnvironmentRoute, environment)
			api.Handle(updateEnvironmentRoute, environment)
			api.Handle(deleteEnvironmentRoute, mockpermit.Reply{Status: http.StatusNoContent})
			task := mockpermit.Reply{Body: `{"task_id":"t","status":"processing"}`}
			api.Handle(statsEnvironmentRoute, environment)
			api.Handle(copyEnvironmentRoute, environment)
			api.Handle(copyAsyncRoute, task)
			api.Handle(copyResultRoute, task)
			calls(t, projectClient(t, api, opts...))
			for _, r := range api.Requests() {
				if !strings.HasPrefix(r.RawPath, "/v2/projects/my-project/envs") {
					t.Errorf("a call sent %s %s, want a path of the project my-project", r.Method,
						r.RawPath)
				}
			}
		})
	}
}

// TestEnvironmentKeysInThePath checks that each method sends the key of the project, and of the
// environment, as one segment of the path each, escaped, whatever the key holds: a slash, a
// percent sign, a space, a question mark and a hash, a letter outside ASCII, the shape of a UUID
// and a leading dot. A key that is empty, . or .. would change the route, so it is an error that
// matches ErrInvalidConfig and names the placeholder, and no request.
func TestEnvironmentKeysInThePath(t *testing.T) {
	ok := mockpermit.Reply{Body: environmentBody}
	task := mockpermit.Reply{Body: `{"task_id":"t","status":"processing"}`}
	copyTo := models.EnvironmentCopy{TargetEnv: models.EnvironmentCopyTarget{Existing: "qa"}}
	methods := []struct {
		name  string
		route string
		reply mockpermit.Reply
		// envs says that the path of the method names an environment after its project, and
		// suffix is the rest of its path after them, such as /stats.
		envs   bool
		suffix string
		call   func(c *Client, projectKey, environmentKey string) error
	}{
		{
			name: "Stats", route: statsEnvironmentRoute, reply: ok, envs: true, suffix: "/stats",
			call: func(c *Client, projectKey, environmentKey string) error {
				_, err := c.Environments.Stats(t.Context(), projectKey, environmentKey)
				return err
			},
		},
		{
			name: "Copy", route: copyEnvironmentRoute, reply: ok, envs: true, suffix: "/copy",
			call: func(c *Client, projectKey, environmentKey string) error {
				_, err := c.Environments.Copy(t.Context(), projectKey, environmentKey, copyTo)
				return err
			},
		},
		{
			name: "CopyAsync", route: copyAsyncRoute, reply: task, envs: true,
			suffix: "/copy/async",
			call: func(c *Client, projectKey, environmentKey string) error {
				_, err := c.Environments.CopyAsync(t.Context(), projectKey, environmentKey, copyTo)
				return err
			},
		},
		{
			name: "CopyResult", route: copyResultRoute, reply: task, envs: true,
			suffix: "/copy/async/t/result",
			call: func(c *Client, projectKey, environmentKey string) error {
				_, err := c.Environments.CopyResult(t.Context(), projectKey, environmentKey, "t",
					nil)
				return err
			},
		},
		{
			name: "List", route: listEnvironmentsRoute, reply: mockpermit.Reply{Body: `[]`},
			call: func(c *Client, projectKey, _ string) error {
				_, err := c.Environments.List(t.Context(), projectKey, nil)
				return err
			},
		},
		{
			name: "All", route: listEnvironmentsRoute, reply: mockpermit.Reply{Body: `[]`},
			call: func(c *Client, projectKey, _ string) error {
				_, errs := collect(t, c.Environments.All(t.Context(), projectKey, nil))
				return errors.Join(errs...)
			},
		},
		{
			name: "Create", route: createEnvironmentRoute, reply: ok,
			call: func(c *Client, projectKey, _ string) error {
				_, err := c.Environments.Create(t.Context(), projectKey,
					models.EnvironmentCreate{Key: "k", Name: "n"})
				return err
			},
		},
		{
			name: "Get", route: getEnvironmentRoute, reply: ok, envs: true,
			call: func(c *Client, projectKey, environmentKey string) error {
				_, err := c.Environments.Get(t.Context(), projectKey, environmentKey)
				return err
			},
		},
		{
			name: "Update", route: updateEnvironmentRoute, reply: ok, envs: true,
			call: func(c *Client, projectKey, environmentKey string) error {
				_, err := c.Environments.Update(t.Context(), projectKey, environmentKey,
					models.EnvironmentUpdate{Name: "n"})
				return err
			},
		},
		{
			name: "Delete", route: deleteEnvironmentRoute,
			reply: mockpermit.Reply{Status: http.StatusNoContent}, envs: true,
			call: func(c *Client, projectKey, environmentKey string) error {
				return c.Environments.Delete(t.Context(), projectKey, environmentKey)
			},
		},
	}
	for key, escaped := range map[string]string{
		"a/b":              "a%2Fb",
		"50%":              "50%25",
		"a b?c#d":          "a%20b%3Fc%23d",
		"é":                "%C3%A9",
		fixtures.UUIDKey:   fixtures.UUIDKey,
		".hidden":          ".hidden",
		"my-project.v2..x": "my-project.v2..x",
	} {
		for _, m := range methods {
			want := "/v2/projects/" + escaped + "/envs"
			if m.envs {
				want += "/" + escaped + m.suffix
			}
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, m.reply)
			if err := m.call(projectClient(t, api), key, key); err != nil {
				t.Errorf("%s(%q, %q) returned %v", m.name, key, key, err)
			}
			if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want {
				t.Errorf("%s(%q, %q) sent %+v, want one request to %s", m.name, key, key,
					requests, want)
			}
		}
	}
	for _, key := range []string{"", ".", ".."} {
		for _, m := range methods {
			checks := []struct{ projectKey, environmentKey, placeholder string }{
				{key, "production", "{proj_id}"},
			}
			if m.envs {
				checks = append(checks, struct{ projectKey, environmentKey, placeholder string }{
					"my-project", key, "{env_id}",
				})
			}
			for _, c := range checks {
				api := mockpermit.NewAPI(t)
				err := m.call(projectClient(t, api), c.projectKey, c.environmentKey)
				if !errors.Is(err, ErrInvalidConfig) ||
					!strings.Contains(err.Error(), c.placeholder) {
					t.Errorf("%s(%q, %q) returned %v, want an error that matches "+
						"ErrInvalidConfig and names %s", m.name, c.projectKey, c.environmentKey,
						err, c.placeholder)
				}
				api.AssertUntouched()
			}
		}
	}
}

// TestDeleteEnvironmentThatIsGone checks that Delete returns nil for a 404, with or without a
// body, as the environment that the path names is gone already, after one request; and an *Error
// with the status for any other error status, such as 403, 409 or 422.
func TestDeleteEnvironmentThatIsGone(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"NOT_FOUND","title":"Not Found","message":"the environment does not exist"}`,
		"",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteEnvironmentRoute,
			mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		if err := projectClient(t, api).Environments.Delete(t.Context(), "p", "gone"); err != nil {
			t.Errorf("Delete of an environment that the API answers with 404 and the body %q "+
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
		api.Handle(deleteEnvironmentRoute,
			mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := projectClient(t, api).Environments.Delete(t.Context(), "p", "production")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteEnvironment ||
			e.Detail != "no" || errors.Is(err, ErrNotFound) {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status and the detail", status, err, opDeleteEnvironment)
		}
	}
}

// TestEnvironmentBodies checks the bodies of Create and Update byte for byte against the golden
// files in testdata/golden: an environment with the required fields only; one with every field,
// an audience that is one string, an issuer that is a list and text that an encoder may escape;
// the zero update, which changes nothing; an update that sends null for each field that takes
// null; and one that sets each field, the empty description, empty settings and an empty list of
// audiences among them. Each update body decodes back into an EnvironmentUpdate with the same
// state of each field, which encodes as the same body.
func TestEnvironmentBodies(t *testing.T) {
	every := models.EnvironmentCreate{
		Key: "e", Name: "<E> & F", Description: "a" + fixtures.LineSeparator + "b",
		CustomBranchName: "b",
		JWKS: &models.JwksConfig{
			Audience: models.OneString("aud"), Issuer: models.StringList("i1", "i2"),
			JWKS: &models.JwksObj{Keys: []models.Attributes{{"kid": "k"}}}, TTL: 60,
			URL: "https://example.com/jwks",
		},
		Settings: models.Attributes{"n": fixtures.TwoPow53Plus1},
	}
	create := func(environment models.EnvironmentCreate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.Environments.Create(t.Context(), "p", environment)
			return err
		}
	}
	update := func(change models.EnvironmentUpdate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.Environments.Update(t.Context(), "p", "e", change)
			return err
		}
	}
	for _, tc := range []struct {
		golden string
		route  string
		call   func(c *Client) error
	}{
		{
			"create_environment.required.golden", createEnvironmentRoute,
			create(models.EnvironmentCreate{Key: "e", Name: "E"}),
		},
		{"create_environment.every-field.golden", createEnvironmentRoute, create(every)},
		{
			"update_environment.unset.golden", updateEnvironmentRoute,
			update(models.EnvironmentUpdate{}),
		},
		{
			"update_environment.null.golden", updateEnvironmentRoute,
			update(models.EnvironmentUpdate{
				Description: models.Null[string](), CustomBranchName: models.Null[string](),
				JWKS: models.Null[models.JwksConfig](), Settings: models.Null[models.Attributes](),
			}),
		},
		{
			"update_environment.values.golden", updateEnvironmentRoute,
			update(models.EnvironmentUpdate{
				Name: "E", Description: models.Some(""), CustomBranchName: models.Some("b"),
				JWKS: models.Some(models.JwksConfig{
					Audience: models.StringList(), URL: "https://example.com/jwks",
				}),
				Settings: models.Some(models.Attributes{}),
			}),
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: environmentBody})
			if err := tc.call(projectClient(t, api)); err != nil {
				t.Fatalf("the call returned %v", err)
			}
			requests := api.Requests()
			if len(requests) != 1 {
				t.Fatalf("the call sent %d requests, want 1", len(requests))
			}
			requests[0].ExpectBytes(t, filepath.Join("testdata", "golden", tc.golden))
			if tc.route == updateEnvironmentRoute {
				checkUpdateRoundTrip(t, requests[0].Body)
			}
		})
	}
}

// checkUpdateRoundTrip checks that body, the body of an update, decodes into an
// EnvironmentUpdate whose fields that take null are in the state that body gives each, and that
// it encodes back as body.
func checkUpdateRoundTrip(t *testing.T, body []byte) {
	t.Helper()
	var update models.EnvironmentUpdate
	if err := wire.Decode(body, &update); err != nil {
		t.Fatalf("the body %s does not decode into an EnvironmentUpdate: %v", body, err)
	}
	var fields map[string]any
	if err := wire.Decode(body, &fields); err != nil {
		t.Fatal(err)
	}
	for name, state := range map[string]struct{ unset, null bool }{
		"description":        {update.Description.IsZero(), update.Description.IsNull()},
		"custom_branch_name": {update.CustomBranchName.IsZero(), update.CustomBranchName.IsNull()},
		"jwks":               {update.JWKS.IsZero(), update.JWKS.IsNull()},
		"settings":           {update.Settings.IsZero(), update.Settings.IsNull()},
	} {
		value, inBody := fields[name]
		if state.unset != !inBody || state.null != (inBody && value == nil) {
			t.Errorf("%s decodes with %s unset %t and null %t, want unset %t and null %t", body,
				name, state.unset, state.null, !inBody, inBody && value == nil)
		}
	}
	if again, err := wire.Encode(update); err != nil || string(again) != string(body) {
		t.Errorf("%s decodes and encodes back as %s, %v", body, again, err)
	}
}

// TestEnvironmentUpdateThatCannotBeJSON checks that an update whose settings are a Some of nil,
// which would send null in place of the settings, is an error that matches ErrInvalidConfig, and
// sends no request.
func TestEnvironmentUpdateThatCannotBeJSON(t *testing.T) {
	api := mockpermit.NewAPI(t)
	_, err := projectClient(t, api).Environments.Update(t.Context(), "p", "e",
		models.EnvironmentUpdate{Settings: models.Some[models.Attributes](nil)})
	if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), opUpdateEnvironment) {
		t.Errorf("Update of Some(nil) settings returned %v, want an error that matches "+
			"ErrInvalidConfig and names %s", err, opUpdateEnvironment)
	}
	api.AssertUntouched()
}

// TestEnvironmentRetries checks when a call of the environments sends its request again: Create,
// Copy and CopyAsync, which the API may have acted on, after a 429 only, so a 503 ends each after
// one request; Update, Get, Delete, List, Stats and CopyResult, which do the same when sent
// twice, after a 503 too.
func TestEnvironmentRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	ok := mockpermit.Reply{Body: environmentBody}
	createCall := func(c *Client) error {
		_, err := c.Environments.Create(t.Context(), "p",
			models.EnvironmentCreate{Key: "k", Name: "n"})
		return err
	}
	task := mockpermit.Reply{Body: `{"task_id":"t","status":"processing"}`}
	copyTo := models.EnvironmentCopy{TargetEnv: models.EnvironmentCopyTarget{Existing: "qa"}}
	copyCall := func(c *Client) error {
		_, err := c.Environments.Copy(t.Context(), "p", "e", copyTo)
		return err
	}
	copyAsyncCall := func(c *Client) error {
		_, err := c.Environments.CopyAsync(t.Context(), "p", "e", copyTo)
		return err
	}
	for _, tc := range []struct {
		name    string
		route   string
		replies []mockpermit.Reply
		call    func(c *Client) error
		status  int
	}{
		{
			name: "Create after a 429", route: createEnvironmentRoute,
			replies: []mockpermit.Reply{limited, ok}, call: createCall,
		},
		{
			name: "Create after a 503", route: createEnvironmentRoute,
			replies: []mockpermit.Reply{unavailable}, call: createCall,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "Update after a 503", route: updateEnvironmentRoute,
			replies: []mockpermit.Reply{unavailable, ok},
			call: func(c *Client) error {
				_, err := c.Environments.Update(t.Context(), "p", "e",
					models.EnvironmentUpdate{Settings: models.Some(models.Attributes{})})
				return err
			},
		},
		{
			name: "Get after a 503", route: getEnvironmentRoute,
			replies: []mockpermit.Reply{unavailable, ok},
			call: func(c *Client) error {
				_, err := c.Environments.Get(t.Context(), "p", "e")
				return err
			},
		},
		{
			name: "Delete after a 503", route: deleteEnvironmentRoute,
			replies: []mockpermit.Reply{unavailable, {Status: http.StatusNoContent}},
			call:    func(c *Client) error { return c.Environments.Delete(t.Context(), "p", "e") },
		},
		{
			name: "List after a 503", route: listEnvironmentsRoute,
			replies: []mockpermit.Reply{unavailable, {Body: `[]`}},
			call: func(c *Client) error {
				_, err := c.Environments.List(t.Context(), "p", nil)
				return err
			},
		},
		{
			name: "Copy after a 429", route: copyEnvironmentRoute,
			replies: []mockpermit.Reply{limited, ok}, call: copyCall,
		},
		{
			name: "Copy after a 503", route: copyEnvironmentRoute,
			replies: []mockpermit.Reply{unavailable}, call: copyCall,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "CopyAsync after a 429", route: copyAsyncRoute,
			replies: []mockpermit.Reply{limited, task}, call: copyAsyncCall,
		},
		{
			name: "CopyAsync after a 503", route: copyAsyncRoute,
			replies: []mockpermit.Reply{unavailable}, call: copyAsyncCall,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "Stats after a 503", route: statsEnvironmentRoute,
			replies: []mockpermit.Reply{unavailable, ok},
			call: func(c *Client) error {
				_, err := c.Environments.Stats(t.Context(), "p", "e")
				return err
			},
		},
		{
			name: "CopyResult after a 503", route: copyResultRoute,
			replies: []mockpermit.Reply{unavailable, task},
			call: func(c *Client) error {
				_, err := c.Environments.CopyResult(t.Context(), "p", "e", "t", nil)
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

// TestMalformedEnvironmentReplies checks that a reply in 200-299 that is not an environment, or
// for List not a JSON array of environments, is an *Error that names the model, and no
// environment; an audience that is neither a string nor a list of strings is one too.
func TestMalformedEnvironmentReplies(t *testing.T) {
	const prefix = "permit: "
	for _, tc := range []struct {
		route, op, model string
		call             func(c *Client) (any, error)
	}{
		{
			getEnvironmentRoute, "get_environment: GET /v2/projects/{proj_id}/envs/{env_id}",
			"models.EnvironmentReadWithEmailConfig",
			func(c *Client) (any, error) { return c.Environments.Get(t.Context(), "p", "e") },
		},
		{
			createEnvironmentRoute, "create_environment: POST /v2/projects/{proj_id}/envs",
			"models.EnvironmentRead",
			func(c *Client) (any, error) {
				return c.Environments.Create(t.Context(), "p",
					models.EnvironmentCreate{Key: "k", Name: "n"})
			},
		},
		{
			updateEnvironmentRoute,
			"update_environment: PATCH /v2/projects/{proj_id}/envs/{env_id}",
			"models.EnvironmentRead",
			func(c *Client) (any, error) {
				return c.Environments.Update(t.Context(), "p", "e", models.EnvironmentUpdate{})
			},
		},
	} {
		for _, body := range []string{
			``, `null`, `[]`, `"e"`, `{"key":1}`, `{} {}`, `{"jwks":{"audience":7}}`,
			`{"jwks":{"issuer":[null]}}`,
		} {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: body})
			got, err := tc.call(projectClient(t, api))
			want := prefix + tc.op + ": status 200: the reply is not a valid " + tc.model
			if e := mustError(t, err); !isNilResult(got) || e.Error() != want {
				t.Errorf("the reply %q returned %+v and\n%v\nwant no environment and\n%s", body,
					got, err, want)
			}
		}
	}
	for _, body := range []string{``, `null`, `{}`, `{"data":[]}`, `[1]`, `[{"key":1}]`} {
		api := mockpermit.NewAPI(t)
		api.Handle(listEnvironmentsRoute, mockpermit.Reply{Body: body})
		page, err := projectClient(t, api).Environments.List(t.Context(), "p", nil)
		want := prefix + "list_environments: GET /v2/projects/{proj_id}/envs: status 200: the " +
			"reply is not a valid list of models.EnvironmentReadWithEmailConfig"
		if e := mustError(t, err); page != nil || e.Error() != want {
			t.Errorf("List of the reply %q returned %+v and\n%v\nwant no page and\n%s", body,
				page, err, want)
		}
	}
}

// TestEnvironmentsAfterClose checks that each method returns ErrClosed after Close, and sends
// nothing, CopyResult also with options that it rejects.
func TestEnvironmentsAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	_, listErr := c.Environments.List(ctx, "p", nil)
	_, allErrs := collect(t, c.Environments.All(ctx, "p", nil))
	_, getErr := c.Environments.Get(ctx, "p", "e")
	_, createErr := c.Environments.Create(ctx, "p", models.EnvironmentCreate{Key: "k", Name: "n"})
	_, updateErr := c.Environments.Update(ctx, "p", "e", models.EnvironmentUpdate{})
	deleteErr := c.Environments.Delete(ctx, "p", "e")
	_, statsErr := c.Environments.Stats(ctx, "p", "e")
	_, copyErr := c.Environments.Copy(ctx, "p", "e", models.EnvironmentCopy{})
	_, asyncErr := c.Environments.CopyAsync(ctx, "p", "e", models.EnvironmentCopy{})
	_, resultErr := c.Environments.CopyResult(ctx, "p", "e", "t", &CopyResultOptions{Wait: -1})
	for i, err := range append([]error{
		listErr, getErr, createErr, updateErr, deleteErr, statsErr, copyErr, asyncErr, resultErr,
	}, allErrs...) {
		if !errors.Is(err, ErrClosed) {
			t.Errorf("call %d after Close returned %v, want ErrClosed", i, err)
		}
	}
	if len(allErrs) != 1 {
		t.Errorf("All after Close yielded %v, want ErrClosed once", allErrs)
	}
	api.AssertUntouched()
}
