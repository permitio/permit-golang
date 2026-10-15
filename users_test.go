package permit

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/internal/transport"
	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The routes of the users on the API, under the scope p and e of userClient, and on the PDP
// container, as mockpermit.Server.Handle takes them.
const (
	listUsersRoute   = "GET /v2/facts/{proj_id}/{env_id}/users"
	createUserRoute  = "POST /v2/facts/{proj_id}/{env_id}/users"
	getUserRoute     = "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}"
	replaceUserRoute = "PUT /v2/facts/{proj_id}/{env_id}/users/{user_id}"
	updateUserRoute  = "PATCH /v2/facts/{proj_id}/{env_id}/users/{user_id}"
	deleteUserRoute  = "DELETE /v2/facts/{proj_id}/{env_id}/users/{user_id}"
)

// userBody is the body of a reply of the API that holds the user alice.
const userBody = `{"key":"alice","id":"00000000-0000-4000-8000-000000000031",` +
	`"organization_id":"00000000-0000-4000-8000-000000000001",` +
	`"project_id":"00000000-0000-4000-8000-000000000002",` +
	`"environment_id":"00000000-0000-4000-8000-000000000003",` +
	`"created_at":"2026-10-14T09:30:00Z","updated_at":"2026-10-14T09:30:00Z",` +
	`"attributes":{},"associated_tenants":[]}`

// usersOf returns the users u<from> to u<to-1>, with only a key: an empty list when to is from.
func usersOf(from, to int) []models.UserRead {
	items := make([]models.UserRead, 0, to-from)
	for i := from; i < to; i++ {
		items = append(items, models.UserRead{Key: "u" + strconv.Itoa(i)})
	}
	return items
}

// userClient returns a client of api with the scope p and e and opts, whose PDP is a server that
// fails the test when anything reaches it.
func userClient(t *testing.T, api *mockpermit.Server, opts ...Option) *Client {
	t.Helper()
	pdp := mockpermit.NewPDP(t)
	t.Cleanup(pdp.AssertUntouched)
	return decisionClient(t, api, pdp, append([]Option{WithScope("p", "e")}, opts...)...)
}

// TestSyncSendsOnePUT checks that Sync sends one request, a PUT of the whole user, and no read
// before it, through the API and through the PDP container; and that the result's Created is
// true for a reply of 201, as the API created the user, and false for a 200, as it replaced it.
func TestSyncSendsOnePUT(t *testing.T) {
	for _, viaPDP := range []bool{false, true} {
		for status, created := range map[int]bool{http.StatusCreated: true, http.StatusOK: false} {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			to, route, path := api, replaceUserRoute, "/v2/facts/p/e/users/alice"
			opts := []Option{WithScope("p", "e")}
			if viaPDP {
				to, route, path = pdp, "PUT /facts/users/{user_id}", "/facts/users/alice"
				opts = append(opts, WithFactsViaPDP())
			}
			to.Handle(route, mockpermit.Reply{Status: status, Body: userBody})
			c := decisionClient(t, api, pdp, opts...)
			got, err := c.Users.Sync(t.Context(), models.UserCreate{Key: "alice"})
			if err != nil || got.User.Key != "alice" || got.Created != created {
				t.Errorf("via the PDP %t: Sync of a reply of %d returned %+v, %v; want alice "+
					"with Created %t", viaPDP, status, got, err, created)
			}
			requests := to.Requests()
			if len(requests) != 1 || requests[0].Method != http.MethodPut ||
				requests[0].RawPath != path {
				t.Fatalf("via the PDP %t: Sync sent %+v, want one PUT to %s", viaPDP, requests,
					path)
			}
			requests[0].ExpectJSON(t, `{"key":"alice"}`)
		}
	}
}

// TestUsersAllReadsEveryPage checks that UsersService.All reads the 31 users of a list, in order,
// asking for each page in turn: at a per_page of 30, two paginated objects without a page_count
// and the empty page after them; at the default per_page of 100, one page of 31 and the empty
// page after it; and pages up to the page_count that they report, without the page after it.
func TestUsersAllReadsEveryPage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    *UsersListOptions
		pages   [][]models.UserRead
		counts  string
		queries []string
	}{
		{
			name:    "per_page 30",
			opts:    &UsersListOptions{ListOptions: ListOptions{PerPage: 30}},
			pages:   [][]models.UserRead{usersOf(0, 30), usersOf(30, 31), {}},
			counts:  `,"total_count":31`,
			queries: []string{"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30"},
		},
		{
			name:    "the default per_page",
			pages:   [][]models.UserRead{usersOf(0, 31), {}},
			counts:  `,"total_count":31`,
			queries: []string{"page=1&per_page=100", "page=2&per_page=100"},
		},
		{
			name:    "a page_count",
			opts:    &UsersListOptions{ListOptions: ListOptions{PerPage: 30}},
			pages:   [][]models.UserRead{usersOf(0, 30), usersOf(30, 31)},
			counts:  `,"total_count":31,"page_count":2`,
			queries: []string{"page=1&per_page=30", "page=2&per_page=30"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			var replies []mockpermit.Reply
			for _, page := range tc.pages {
				replies = append(replies, pageOf(t, page, tc.counts))
			}
			api.Handle(listUsersRoute, replies...)
			items, errs := collect(t, userClient(t, api).Users.All(t.Context(), tc.opts))
			if diff := cmp.Diff(usersOf(0, 31), items); diff != "" || errs != nil {
				t.Errorf("All yielded the errors %v and users that differ (-want +got):\n%s",
					errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestUsersAllReadsTheOptionsOnce checks that the iterator keeps the options of its call, the
// filters too, so that a later change to them changes no request; that a loop that breaks asks
// for no other page; and that each loop reads the users again from the first page, along the
// same path, through the API and through the PDP container.
func TestUsersAllReadsTheOptionsOnce(t *testing.T) {
	for _, viaPDP := range []bool{false, true} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		to, route, path := api, listUsersRoute, "/v2/facts/p/e/users"
		opts := []Option{WithScope("p", "e")}
		if viaPDP {
			to, route, path = pdp, "GET /facts/users", "/facts/users"
			opts = append(opts, WithFactsViaPDP())
		}
		two := pageOf(t, usersOf(0, 2), `,"total_count":9`)
		to.Handle(route, two, two)
		options := &UsersListOptions{
			ListOptions: ListOptions{PerPage: 2}, Search: "u", Role: "viewer",
		}
		seq := decisionClient(t, api, pdp, opts...).Users.All(t.Context(), options)
		options.Page, options.PerPage, options.Search, options.Role = 5, 50, "x", ""
		for loop := 1; loop <= 2; loop++ {
			for user, err := range seq {
				if err != nil || user.Key != "u0" {
					t.Errorf("via the PDP %t: loop %d yielded %+v and %v first, want u0", viaPDP,
						loop, user, err)
				}
				break
			}
		}
		var got []string
		for _, r := range to.Requests() {
			got = append(got, r.RawPath+"?"+r.RawQuery)
		}
		first := path + "?page=1&per_page=2&role=viewer&search=u"
		if diff := cmp.Diff([]string{first, first}, got); diff != "" {
			t.Errorf("via the PDP %t: the requests differ (-want +got):\n%s", viaPDP, diff)
		}
	}
}

// TestUsersScope checks where the calls of the users go without WithScope: to the project and
// the environment of the API key, which the first call looks up once, and All only when a loop
// asks for a user; and, with the scope of a project-level key, which has no environment, to no
// user route, with an error that matches ErrInvalidConfig.
func TestUsersScope(t *testing.T) {
	t.Run("the scope of the API key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		api.Handle(replaceUserRoute, mockpermit.Reply{Body: userBody})
		api.Handle(listUsersRoute, mockpermit.Reply{Body: `{"data":[],"total_count":0}`})
		c := decisionClient(t, api, mockpermit.NewPDP(t))
		seq := c.Users.All(t.Context(), nil)
		if _, err := c.Users.Sync(t.Context(), models.UserCreate{Key: "alice"}); err != nil {
			t.Fatal(err)
		}
		if _, errs := collect(t, seq); errs != nil {
			t.Fatal(errs)
		}
		users := "/v2/facts/" + scopeProjectID + "/" + scopeEnvironmentID + "/users"
		var got []string
		for _, r := range api.Requests() {
			got = append(got, r.Method+" "+r.RawPath)
		}
		want := []string{
			"GET /v2/api-key/scope", "PUT " + users + "/alice", "GET " + users,
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("the requests differ (-want +got):\n%s", diff)
		}
	})
	t.Run("a scope without an environment", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ".project"))
		c := decisionClient(t, api, mockpermit.NewPDP(t))
		_, err := c.Users.Sync(t.Context(), models.UserCreate{Key: "alice"})
		want := "permit: invalid configuration: replace_user: " +
			validate.ScopeEnvironmentMissing.Message
		if !errors.Is(err, ErrInvalidConfig) || err.Error() != want {
			t.Errorf("Sync returned %v, want %q", err, want)
		}
		if n := len(api.Requests()); n != 1 {
			t.Errorf("the API got %d requests, want the lookup alone", n)
		}
	})
}

// TestUserKeysInThePath checks that each method that names a user sends its key as one segment
// of the path, escaped, whatever the key holds: Sync the Key of the user that it sends. A key
// that is empty, . or .. would change the route, so it is an error that matches
// ErrInvalidConfig and names the placeholder, and no request.
func TestUserKeysInThePath(t *testing.T) {
	ok := mockpermit.Reply{Body: userBody}
	methods := []struct {
		name  string
		route string
		reply mockpermit.Reply
		call  func(c *Client, key string) error
	}{
		{
			name: "Get", route: getUserRoute, reply: ok,
			call: func(c *Client, key string) error {
				_, err := c.Users.Get(t.Context(), key)
				return err
			},
		},
		{
			name: "Sync", route: replaceUserRoute, reply: ok,
			call: func(c *Client, key string) error {
				_, err := c.Users.Sync(t.Context(), models.UserCreate{Key: key})
				return err
			},
		},
		{
			name: "Update", route: updateUserRoute, reply: ok,
			call: func(c *Client, key string) error {
				_, err := c.Users.Update(t.Context(), key, models.UserUpdate{})
				return err
			},
		},
		{
			name: "Delete", route: deleteUserRoute,
			reply: mockpermit.Reply{Status: http.StatusNoContent},
			call:  func(c *Client, key string) error { return c.Users.Delete(t.Context(), key) },
		},
	}
	for key, want := range map[string]string{
		"a/b":                 "/v2/facts/p/e/users/a%2Fb",
		"50%":                 "/v2/facts/p/e/users/50%25",
		"a b?c#d":             "/v2/facts/p/e/users/a%20b%3Fc%23d",
		"idp|a+b@example.com": "/v2/facts/p/e/users/idp%7Ca+b@example.com",
		fixtures.UUIDKey:      "/v2/facts/p/e/users/" + fixtures.UUIDKey,
		".hidden":             "/v2/facts/p/e/users/.hidden",
		fixtures.TypedKey:     "/v2/facts/p/e/users/" + fixtures.TypedKey,
		"é" + " " + "x":       "/v2/facts/p/e/users/%C3%A9%E2%80%A8x",
	} {
		for _, m := range methods {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, m.reply)
			if err := m.call(userClient(t, api), key); err != nil {
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
			err := m.call(userClient(t, api), key)
			if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), "{user_id}") {
				t.Errorf("%s(%q) returned %v, want an error that matches ErrInvalidConfig and "+
					"names {user_id}", m.name, key, err)
			}
			api.AssertUntouched()
		}
	}
}

// TestDeleteUserThatIsGone checks that Delete returns nil for a 404 of the API, with or without
// a body, as the user that the path names is gone already, after one request; an *Error with the
// status for any other error status; and, through the PDP container, an error that matches
// ErrContainerPDPOnly for a 404 with an empty body, as the cloud PDP answers so, and nil for a 404
// of the container with a body.
func TestDeleteUserThatIsGone(t *testing.T) {
	for _, body := range []string{`{"detail":"the user does not exist"}`, ""} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteUserRoute, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		if err := userClient(t, api).Users.Delete(t.Context(), "gone"); err != nil {
			t.Errorf("Delete of a user that the API answers with 404 and the body %q returned "+
				"%v, want nil", body, err)
		}
		if n := len(api.Requests()); n != 1 {
			t.Errorf("Delete sent %d requests, want 1", n)
		}
	}
	for _, status := range []int{http.StatusForbidden, http.StatusUnprocessableEntity} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteUserRoute, mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := userClient(t, api).Users.Delete(t.Context(), "alice")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteUser {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status", status, err, opDeleteUser)
		}
	}
	for body, wantContainer := range map[string]bool{"": true, `{"detail":"gone"}`: false} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle("DELETE /facts/users/{user_id}",
			mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		err := decisionClient(t, api, pdp, WithFactsViaPDP()).Users.Delete(t.Context(), "alice")
		if got := errors.Is(err, ErrContainerPDPOnly); got != wantContainer ||
			(!wantContainer && err != nil) {
			t.Errorf("Delete through the PDP of a 404 with the body %q returned %v, want an "+
				"error that matches ErrContainerPDPOnly: %t", body, err, wantContainer)
		}
		api.AssertUntouched()
	}
}

// TestUserBodies checks the bodies of Create, Sync and Update byte for byte against the golden
// files in testdata/golden: a user with the key only, which leaves every other field out, as a
// sync that clears them sends it; one with every field, text that an encoder may escape and a
// role on a resource instance; one with the empty attributes {} and an empty list of roles,
// which it sends as [], not nil; the zero update, which changes nothing; an update that sends
// null for each field; and one that sets each field, the attributes to {}.
func TestUserBodies(t *testing.T) {
	create := func(user models.UserCreate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.Users.Create(t.Context(), user)
			return err
		}
	}
	update := func(change models.UserUpdate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.Users.Update(t.Context(), "alice", change)
			return err
		}
	}
	every := models.UserCreate{
		Key: "idp|alice", Email: "alice@example.com", FirstName: "<Alice> & B",
		LastName: "a" + fixtures.LineSeparator + "b",
		Attributes: models.Attributes{
			"n": fixtures.TwoPow53Plus1, "list": []any{"a", 1}, "nested": map[string]any{},
		},
		RoleAssignments: []models.UserRoleCreate{
			{Role: "viewer"},
			{Role: "owner", Tenant: "tenant-a", ResourceInstance: "document:doc-1"},
		},
	}
	for _, tc := range []struct {
		golden string
		route  string
		call   func(c *Client) error
	}{
		{"create_user.required.golden", createUserRoute, create(models.UserCreate{Key: "alice"})},
		{"create_user.every-field.golden", createUserRoute, create(every)},
		{
			"create_user.empty.golden", createUserRoute,
			create(models.UserCreate{
				Key: "alice", Attributes: models.Attributes{},
				RoleAssignments: []models.UserRoleCreate{},
			}),
		},
		{
			"replace_user.required.golden", replaceUserRoute,
			func(c *Client) error {
				_, err := c.Users.Sync(t.Context(), models.UserCreate{Key: "alice"})
				return err
			},
		},
		{"update_user.unset.golden", updateUserRoute, update(models.UserUpdate{})},
		{
			"update_user.null.golden", updateUserRoute,
			update(models.UserUpdate{
				Email: models.Null[string](), FirstName: models.Null[string](),
				LastName: models.Null[string](), Attributes: models.Null[models.Attributes](),
			}),
		},
		{
			"update_user.values.golden", updateUserRoute,
			update(models.UserUpdate{
				Email: models.Some("alice@example.com"), FirstName: models.Some("Alice"),
				LastName: models.Some(""), Attributes: models.Some(models.Attributes{}),
			}),
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: userBody})
			if err := tc.call(userClient(t, api)); err != nil {
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

// TestUserRetries checks when a call of the users sends its request again: Create, which the API
// may have acted on, after a 429 only, so a 503 ends it after one request; Sync, Update, Get,
// Delete and List, which do the same when sent twice, after a 503 too.
func TestUserRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	ok := mockpermit.Reply{Body: userBody}
	createUser := func(c *Client) error {
		_, err := c.Users.Create(t.Context(), models.UserCreate{Key: "alice"})
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
			name: "Create after a 429", route: createUserRoute,
			replies: []mockpermit.Reply{limited, ok}, call: createUser,
		},
		{
			name: "Create after a 503", route: createUserRoute,
			replies: []mockpermit.Reply{unavailable}, call: createUser,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "Sync after a 503", route: replaceUserRoute,
			replies: []mockpermit.Reply{unavailable, ok},
			call: func(c *Client) error {
				_, err := c.Users.Sync(t.Context(), models.UserCreate{Key: "alice"})
				return err
			},
		},
		{
			name: "Update after a 503", route: updateUserRoute,
			replies: []mockpermit.Reply{unavailable, ok},
			call: func(c *Client) error {
				_, err := c.Users.Update(t.Context(), "alice", models.UserUpdate{})
				return err
			},
		},
		{
			name: "Get after a 503", route: getUserRoute,
			replies: []mockpermit.Reply{unavailable, ok},
			call: func(c *Client) error {
				_, err := c.Users.Get(t.Context(), "alice")
				return err
			},
		},
		{
			name: "Delete after a 503", route: deleteUserRoute,
			replies: []mockpermit.Reply{unavailable, {Status: http.StatusNoContent}},
			call:    func(c *Client) error { return c.Users.Delete(t.Context(), "alice") },
		},
		{
			name: "List after a 503", route: listUsersRoute,
			replies: []mockpermit.Reply{unavailable, {Body: `{"data":[],"total_count":0}`}},
			call: func(c *Client) error {
				_, err := c.Users.List(t.Context(), nil)
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, tc.replies...)
			err := tc.call(userClient(t, api, WithRetry(RetryPolicy{MaxRetries: 1})))
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

// TestMalformedUserReplies checks that a reply in 200-299 that is not a user, an empty body too,
// to Get, Create, Sync and Update, or for List not the paginated object of a list of users, is an
// *Error that names the model, with the status of the reply, and no result.
func TestMalformedUserReplies(t *testing.T) {
	calls := map[string]func(c *Client) (bool, error){
		getUserRoute: func(c *Client) (bool, error) {
			user, err := c.Users.Get(t.Context(), "alice")
			return user != nil, err
		},
		createUserRoute: func(c *Client) (bool, error) {
			user, err := c.Users.Create(t.Context(), models.UserCreate{Key: "alice"})
			return user != nil, err
		},
		replaceUserRoute: func(c *Client) (bool, error) {
			result, err := c.Users.Sync(t.Context(), models.UserCreate{Key: "alice"})
			return result != nil, err
		},
		updateUserRoute: func(c *Client) (bool, error) {
			user, err := c.Users.Update(t.Context(), "alice", models.UserUpdate{})
			return user != nil, err
		},
	}
	for route, call := range calls {
		for _, status := range []int{http.StatusOK, http.StatusCreated} {
			for _, body := range []string{``, ` `, `null`, `[]`, `"u"`, `{"key":1}`, `{} {}`} {
				api := mockpermit.NewAPI(t)
				api.Handle(route, mockpermit.Reply{Status: status, Body: body})
				got, err := call(userClient(t, api))
				want := "status " + strconv.Itoa(status) + ": the reply is not a valid " +
					"models.UserRead"
				if e := mustError(t, err); got || e.StatusCode != status ||
					!strings.HasSuffix(e.Error(), want) {
					t.Errorf("%s of the reply %d %q returned a result: %t, and\n%v\nwant no "+
						"result and an error that ends with %q", route, status, body, got, err,
						want)
				}
			}
		}
	}
	for _, body := range []string{``, `null`, `"u"`, `[]`, `[{"key":"u"}]`, `{"data":[1]}`} {
		api := mockpermit.NewAPI(t)
		api.Handle(listUsersRoute, mockpermit.Reply{Body: body})
		page, err := userClient(t, api).Users.List(t.Context(), nil)
		want := "permit: list_users: GET /v2/facts/{proj_id}/{env_id}/users: status 200: " +
			"the reply is not a valid list of models.UserRead"
		if e := mustError(t, err); page != nil || e.Error() != want {
			t.Errorf("List of the reply %q returned %+v and\n%v\nwant no page and\n%s", body, page,
				err, want)
		}
	}
}

// FuzzUserRead checks the reader of a reply that holds a user on any body: it returns a user or
// an *Error that names models.UserRead, never both and never a panic, and a user that it returns
// encodes to a body that it reads back as the same user. go test runs the seeds.
func FuzzUserRead(f *testing.F) {
	for _, body := range []string{
		userBody, replyFixtureOf(f, "get_user.200.json"), ``, `null`, `[]`, `{}`, `{"key":1}`,
		`{} {}`, `{"associated_tenants":null,"attributes":null}`,
		`{"associated_tenants":[{"tenant":"t","roles":null,"status":"added_later"}]}`,
		`{"created_at":"not a time"}`, `{"attributes":{"n":1e400}}`,
	} {
		f.Add([]byte(body))
	}
	op := defaultOp(f, opGetUser)
	f.Fuzz(func(t *testing.T, body []byte) {
		user, err := decodeObject[models.UserRead](op, transport.Reply{Status: 200, Body: body})
		if (user == nil) == (err == nil) {
			t.Fatalf("the reply %q returned the user %+v and the error %v, want one of them",
				body, user, err)
		}
		if err != nil {
			if !strings.HasSuffix(err.Error(), "the reply is not a valid models.UserRead") {
				t.Fatalf("the reply %q returned %v, want the error that names the model", body,
					err)
			}
			return
		}
		encoded, err := wire.Encode(user)
		if err != nil {
			t.Fatalf("the user of the reply %q does not encode: %v", body, err)
		}
		again, err := decodeObject[models.UserRead](op, transport.Reply{Status: 200, Body: encoded})
		if err != nil {
			t.Fatalf("the user of the reply %q encodes as %s, which does not decode: %v", body,
				encoded, err)
		}
		if diff := cmp.Diff(user, again); diff != "" {
			t.Fatalf("the user of the reply %q encodes as %s, which decodes to another user "+
				"(-first +second):\n%s", body, encoded, diff)
		}
	})
}

// TestUsersAfterClose checks that each method returns ErrClosed after Close, and sends nothing,
// not even the lookup of the scope: also List and All with options that are not valid, and each
// method of a copy of WaitForSync whose wait is not valid, which return ErrInvalidConfig before
// Close.
func TestUsersAfterClose(t *testing.T) {
	invalid := &UsersListOptions{ListOptions: ListOptions{Page: -1}}
	for _, opts := range [][]Option{nil, {WithScope("p", "e")}, {WithFactsViaPDP()}} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		c := decisionClient(t, api, pdp, opts...)
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		ctx := t.Context()
		for _, s := range []*UsersService{c.Users, c.Users.WaitForSync(-time.Second)} {
			var errs []error
			for _, listOpts := range []*UsersListOptions{nil, invalid} {
				_, listErr := s.List(ctx, listOpts)
				_, allErrs := collect(t, s.All(ctx, listOpts))
				if len(allErrs) != 1 {
					t.Errorf("All after Close yielded %v, want ErrClosed once", allErrs)
				}
				errs = append(append(errs, listErr), allErrs...)
			}
			_, getErr := s.Get(ctx, "alice")
			_, createErr := s.Create(ctx, models.UserCreate{Key: "alice"})
			_, syncErr := s.Sync(ctx, models.UserCreate{Key: "alice"})
			_, updateErr := s.Update(ctx, "alice", models.UserUpdate{})
			deleteErr := s.Delete(ctx, "alice")
			for i, err := range append(errs, getErr, createErr, syncErr, updateErr, deleteErr) {
				if !errors.Is(err, ErrClosed) {
					t.Errorf("call %d after Close returned %v, want ErrClosed", i, err)
				}
			}
		}
		api.AssertUntouched()
		pdp.AssertUntouched()
	}
}

// userWrites are the calls of the users that the PDP container waits for sync on, each with its
// route on the container and its operation.
var userWrites = []struct {
	route string
	op    string
	call  func(ctx context.Context, s *UsersService) error
}{
	{
		route: "POST /facts/users", op: opCreateUser,
		call: func(ctx context.Context, s *UsersService) error {
			_, err := s.Create(ctx, models.UserCreate{Key: "alice"})
			return err
		},
	},
	{
		route: "PUT /facts/users/{user_id}", op: opReplaceUser,
		call: func(ctx context.Context, s *UsersService) error {
			_, err := s.Sync(ctx, models.UserCreate{Key: "alice"})
			return err
		},
	},
	{
		route: "PATCH /facts/users/{user_id}", op: opUpdateUser,
		call: func(ctx context.Context, s *UsersService) error {
			_, err := s.Update(ctx, "alice", models.UserUpdate{})
			return err
		},
	},
}

// TestUsersWaitForSync checks the creates, syncs and updates of a copy of WaitForSync through
// the PDP container: each carries the timeout in decimal seconds and the policy of
// WithSyncPolicy, and the calls of the service that WaitForSync was called on carry neither,
// before and after.
func TestUsersWaitForSync(t *testing.T) {
	for _, w := range userWrites {
		t.Run(w.op, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			reply := mockpermit.Reply{Body: userBody}
			pdp.Handle(w.route, reply, reply, reply)
			plain := decisionClient(t, api, pdp, WithFactsViaPDP()).Users
			waiting := plain.WaitForSync(1500*time.Millisecond, WithSyncPolicy(SyncPolicyFail))
			for _, s := range []*UsersService{plain, waiting, plain} {
				if err := w.call(t.Context(), s); err != nil {
					t.Fatal(err)
				}
			}
			var got []string
			for _, r := range pdp.Requests() {
				got = append(got, waitHeaders(r.Header))
			}
			if diff := cmp.Diff([]string{"- -", "1.5 fail", "- -"}, got); diff != "" {
				t.Errorf("the wait headers of the three calls differ (-want +got):\n%s", diff)
			}
			api.AssertUntouched()
		})
	}
}

// TestUsersWaitForSyncMisuse checks the calls through a copy of WaitForSync that cannot wait:
// List, All, Get and Delete, which the PDP container answers without waiting; and Create, Sync
// and Update from a client that sends facts to the API, or with a negative timeout. Each returns
// an error that matches ErrInvalidConfig and names the operation and the rule, and sends no
// request, not even the lookup of the scope.
func TestUsersWaitForSyncMisuse(t *testing.T) {
	type misuse struct {
		name    string
		viaPDP  bool
		timeout time.Duration
		op      string
		call    func(ctx context.Context, s *UsersService) error
		rule    validate.Rule
	}
	cases := []misuse{
		{
			name: "List", viaPDP: true, op: opListUsers, rule: validate.WaitForSyncNotWaiting,
			call: func(ctx context.Context, s *UsersService) error {
				_, err := s.List(ctx, nil)
				return err
			},
		},
		{
			name: "All", viaPDP: true, op: opListUsers, rule: validate.WaitForSyncNotWaiting,
			call: func(ctx context.Context, s *UsersService) error {
				for _, err := range s.All(ctx, nil) {
					return err
				}
				return nil
			},
		},
		{
			name: "Get", viaPDP: true, op: opGetUser, rule: validate.WaitForSyncNotWaiting,
			call: func(ctx context.Context, s *UsersService) error {
				_, err := s.Get(ctx, "alice")
				return err
			},
		},
		{
			name: "Delete", viaPDP: true, op: opDeleteUser, rule: validate.WaitForSyncNotWaiting,
			call: func(ctx context.Context, s *UsersService) error {
				return s.Delete(ctx, "alice")
			},
		},
	}
	for _, w := range userWrites {
		cases = append(cases,
			misuse{
				name: w.op + " to the API", op: w.op, call: w.call,
				rule: validate.WaitForSyncFactsViaAPI,
			},
			misuse{
				name: w.op + " with a negative timeout", viaPDP: true, timeout: -time.Nanosecond,
				op: w.op, call: w.call, rule: validate.WaitForSyncTimeoutNegative,
			})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			var opts []Option
			if tc.viaPDP {
				opts = append(opts, WithFactsViaPDP())
			}
			s := decisionClient(t, api, pdp, opts...).Users.WaitForSync(tc.timeout)
			err := tc.call(t.Context(), s)
			want := "permit: invalid configuration: " + tc.op + ": " + tc.rule.Message
			if err == nil || err.Error() != want || !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("the call returned %v, want %q, which matches ErrInvalidConfig", err,
					want)
			}
			api.AssertUntouched()
			pdp.AssertUntouched()
		})
	}
}
