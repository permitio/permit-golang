package permit

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The routes of the user invites of an environment, as mockpermit.Server.Handle takes them, and
// the path of the invites with the scope p and e.
const (
	invitesPath        = "/v2/facts/{proj_id}/{env_id}/user_invites"
	invitePath         = invitesPath + "/{user_invite_id}"
	listInvitesRoute   = "GET " + invitesPath
	createInviteRoute  = "POST " + invitesPath
	getInviteRoute     = "GET " + invitePath
	updateInviteRoute  = "PATCH " + invitePath
	deleteInviteRoute  = "DELETE " + invitePath
	approveInviteRoute = "POST " + invitePath + "/approve"
	invitesRaw         = "/v2/facts/p/e/user_invites"
	inviteEmptyPage    = `{"data":[],"total_count":0,"page_count":0}`
)

// The bodies of replies of the API: an invite, as each call of one invite but an approval
// returns it, and the user of an approval.
const (
	inviteReadBody = `{"id":"00000000-0000-4000-8000-000000000401","key":"alice@example.com",` +
		`"status":"pending","email":"alice@example.com"}`
	inviteUserBody = `{"key":"alice@example.com","id":"00000000-0000-4000-8000-000000000431"}`
)

// inviteFields are the fields of the invite of the tests, as a create and an update send them.
var inviteFields = models.ElementsUserInviteCreate{
	Key: "alice@example.com", Status: models.UserInviteStatusPending,
	Email: "alice@example.com", FirstName: "Alice", LastName: "Liddell",
	RoleID: fixtures.UUIDKey, TenantID: fixtures.UUIDKey, ResourceInstanceID: fixtures.UUIDKey,
}

// inviteApproval is the approval of the tests, with an attribute.
var inviteApproval = models.ElementsUserInviteApprove{
	Email: "alice@example.com", Key: "alice@example.com",
	Attributes: models.Attributes{"department": "finance"},
}

// invitesOf returns the invites i<from> to i<to-1>, with only a key: an empty list when to is
// from.
func invitesOf(from, to int) []models.ElementsUserInviteRead {
	items := make([]models.ElementsUserInviteRead, 0, to-from)
	for i := from; i < to; i++ {
		items = append(items, models.ElementsUserInviteRead{Key: "i" + strconv.Itoa(i)})
	}
	return items
}

// inviteCall is a call of a method of the user invites, with the ID of its operation, its route
// and a reply of the API to it. Each call takes the ID of an invite, which a method that names no
// invite leaves out of its request.
type inviteCall struct {
	name  string
	op    string
	route string
	reply mockpermit.Reply
	call  func(ctx context.Context, c *Client, id string) error
}

// The calls of the methods of the user invites, by what their paths name: nothing but the scope,
// and one invite.
var (
	inviteListCalls = []inviteCall{
		{
			name: "List", op: opListUserInvites, route: listInvitesRoute,
			reply: mockpermit.Reply{Body: inviteEmptyPage},
			call: func(ctx context.Context, c *Client, _ string) error {
				_, err := c.UserInvites.List(ctx, nil)
				return err
			},
		},
		{
			name: "Create", op: opCreateUserInvite, route: createInviteRoute,
			reply: mockpermit.Reply{Body: inviteReadBody},
			call: func(ctx context.Context, c *Client, _ string) error {
				_, err := c.UserInvites.Create(ctx, inviteFields)
				return err
			},
		},
	}
	inviteIDCalls = []inviteCall{
		{
			name: "Get", op: opGetUserInvite, route: getInviteRoute,
			reply: mockpermit.Reply{Body: inviteReadBody},
			call: func(ctx context.Context, c *Client, id string) error {
				_, err := c.UserInvites.Get(ctx, id)
				return err
			},
		},
		{
			name: "Update", op: opUpdateUserInvite, route: updateInviteRoute,
			reply: mockpermit.Reply{Body: inviteReadBody},
			call: func(ctx context.Context, c *Client, id string) error {
				update := models.ElementsUserInviteUpdate(inviteFields)
				_, err := c.UserInvites.Update(ctx, id, update)
				return err
			},
		},
		{
			name: "Approve", op: opApproveUserInvite, route: approveInviteRoute,
			reply: mockpermit.Reply{Body: inviteUserBody},
			call: func(ctx context.Context, c *Client, id string) error {
				_, err := c.UserInvites.Approve(ctx, id, inviteApproval)
				return err
			},
		},
		{
			name: "Delete", op: opDeleteUserInvite, route: deleteInviteRoute,
			reply: mockpermit.Reply{Status: http.StatusNoContent},
			call: func(ctx context.Context, c *Client, id string) error {
				return c.UserInvites.Delete(ctx, id)
			},
		},
	}
)

// inviteCalls returns every call of inviteListCalls and inviteIDCalls.
func inviteCalls() []inviteCall {
	return append(append([]inviteCall{}, inviteListCalls...), inviteIDCalls...)
}

// TestUserInvitesAllReadsEveryPage checks that All reads the 31 invites of a list, in order,
// asking for each page in turn: up to the page whose invites reach the total_count of its reply,
// at a per_page of 30 and at the default per_page of 100, the most that the API gives on a page;
// up to the empty page when the replies do not report total_count; and from page 2 on, when the
// options start there, up to the empty page after it, as a loop that starts after the first page
// does not stop at the total_count. Each reply reports the page_count of the API, the number of
// invites on its page. Each request goes to the invites of the scope, with no filter in its query.
func TestUserInvitesAllReadsEveryPage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    *UserInvitesListOptions
		replies []mockpermit.Reply
		want    []models.ElementsUserInviteRead
		queries []string
	}{
		{
			name: "per_page 30",
			opts: &UserInvitesListOptions{ListOptions: ListOptions{PerPage: 30}},
			replies: []mockpermit.Reply{
				pageOf(t, invitesOf(0, 30), `,"total_count":31,"page_count":30`),
				pageOf(t, invitesOf(30, 31), `,"total_count":31,"page_count":1`),
			},
			want:    invitesOf(0, 31),
			queries: []string{"page=1&per_page=30", "page=2&per_page=30"},
		},
		{
			name: "the default per_page",
			replies: []mockpermit.Reply{
				pageOf(t, invitesOf(0, 31), `,"total_count":31,"page_count":31`),
			},
			want:    invitesOf(0, 31),
			queries: []string{"page=1&per_page=100"},
		},
		{
			name: "pages without total_count",
			opts: &UserInvitesListOptions{ListOptions: ListOptions{PerPage: 30}},
			replies: []mockpermit.Reply{
				pageOf(t, invitesOf(0, 30), `,"page_count":30`),
				pageOf(t, invitesOf(30, 31), `,"page_count":1`),
				pageOf(t, invitesOf(0, 0), `,"page_count":0`),
			},
			want: invitesOf(0, 31),
			queries: []string{
				"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30",
			},
		},
		{
			name: "from page 2",
			opts: &UserInvitesListOptions{ListOptions: ListOptions{Page: 2, PerPage: 30}},
			replies: []mockpermit.Reply{
				pageOf(t, invitesOf(30, 31), `,"total_count":31,"page_count":1`),
				pageOf(t, invitesOf(0, 0), `,"total_count":31,"page_count":0`),
			},
			want:    invitesOf(30, 31),
			queries: []string{"page=2&per_page=30", "page=3&per_page=30"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listInvitesRoute, tc.replies...)
			items, errs := collect(t, resourceClient(t, api).UserInvites.All(t.Context(), tc.opts))
			if diff := cmp.Diff(tc.want, items); diff != "" || errs != nil {
				t.Errorf("All yielded the errors %v and invites that differ (-want +got):\n%s",
					errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
			for _, r := range api.Requests() {
				if r.RawPath != invitesRaw {
					t.Errorf("All sent a request to %s, want %s", r.RawPath, invitesRaw)
				}
			}
		})
	}
}

// TestUserInvitesAllReadsTheOptionsOnce checks that All keeps the options of its call, its page
// and its filters, so that a later change to them changes no request, that a loop that breaks
// asks for no other page, and that each loop over the iterator reads the invites again from the
// first page.
func TestUserInvitesAllReadsTheOptionsOnce(t *testing.T) {
	api := mockpermit.NewAPI(t)
	two := pageOf(t, invitesOf(0, 2), `,"total_count":4,"page_count":2`)
	api.Handle(listInvitesRoute, two, two)
	opts := &UserInvitesListOptions{ListOptions: ListOptions{PerPage: 2}, Role: "editor"}
	seq := resourceClient(t, api).UserInvites.All(t.Context(), opts)
	opts.Page, opts.PerPage, opts.Role, opts.Tenant = 5, 50, "viewer", "tenant-b"
	for loop := 1; loop <= 2; loop++ {
		for invite, err := range seq {
			if err != nil || invite.Key != "i0" {
				t.Errorf("loop %d yielded %+v and %v first, want the invite i0", loop, invite, err)
			}
			break
		}
	}
	first := "page=1&per_page=2&role=editor"
	if diff := cmp.Diff([]string{first, first}, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestUserInvitesAllYieldsAnErrorOnce checks that a page that fails ends All with its error,
// once, after the invites of the pages before it.
func TestUserInvitesAllYieldsAnErrorOnce(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(listInvitesRoute, pageOf(t, invitesOf(0, 30), `,"total_count":60`),
		mockpermit.Reply{Status: http.StatusForbidden, Body: body})
	opts := &UserInvitesListOptions{ListOptions: ListOptions{PerPage: 30}}
	items, errs := collect(t, resourceClient(t, api).UserInvites.All(t.Context(), opts))
	if len(items) != 30 || len(errs) != 1 || !errors.Is(errs[0], ErrForbidden) {
		t.Errorf("All yielded %d invites and the errors %v, want 30 and one error that matches "+
			"ErrForbidden", len(items), errs)
	}
	if n := len(api.Requests()); n != 2 {
		t.Errorf("All sent %d requests, want 2", n)
	}
}

// TestUserInvitesAllGetsTheScopeInEachLoop checks that All, without WithScope, looks up the scope
// in each loop over the iterator, before its first page, and not when it is called: a lookup that
// fails ends that loop with its error, and the next loop looks the scope up again.
func TestUserInvitesAllGetsTheScopeInEachLoop(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(scopeRoute, mockpermit.Reply{Status: http.StatusForbidden, Body: body},
		scopeReply(t, ""))
	api.Handle(listInvitesRoute, pageOf(t, invitesOf(0, 1), `,"total_count":1,"page_count":1`))
	seq := projectClient(t, api).UserInvites.All(t.Context(), nil)
	if items, errs := collect(t, seq); len(items) != 0 || len(errs) != 1 ||
		!errors.Is(errs[0], ErrForbidden) {
		t.Errorf("the first loop yielded %d invites and the errors %v, want one error that "+
			"matches ErrForbidden", len(items), errs)
	}
	if items, errs := collect(t, seq); len(items) != 1 || errs != nil {
		t.Errorf("the second loop yielded %d invites and the errors %v, want 1", len(items), errs)
	}
	var paths []string
	for _, r := range api.Requests() {
		paths = append(paths, r.RawPath)
	}
	want := []string{
		"/v2/api-key/scope", "/v2/api-key/scope",
		"/v2/facts/" + scopeProjectID + "/" + scopeEnvironmentID + "/user_invites",
	}
	if diff := cmp.Diff(want, paths); diff != "" {
		t.Errorf("the requests differ (-want +got):\n%s", diff)
	}
}

// TestUserInvitesListReturnsAPage checks the page that List returns: the invites of the reply in
// its order, the page and the per_page that it asked for, page 1 and no query for nil options,
// and the counts that the reply reports, nil for a count that it leaves out. A page with invites
// and a page without them both have a list that is not nil, empty for the second. Each filter
// that is set is sent in its query parameter, as it is given, and one that is empty is left out.
func TestUserInvitesListReturnsAPage(t *testing.T) {
	empty := []models.ElementsUserInviteRead{}
	for _, tc := range []struct {
		name  string
		opts  *UserInvitesListOptions
		reply mockpermit.Reply
		query string
		want  *Page[models.ElementsUserInviteRead]
	}{
		{
			name:  "nil options",
			reply: pageOf(t, invitesOf(0, 2), `,"total_count":2,"page_count":2`),
			want: &Page[models.ElementsUserInviteRead]{
				Data: invitesOf(0, 2), Page: 1, TotalCount: new(2), PageCount: new(2),
			},
		},
		{
			name:  "a page without page_count",
			opts:  &UserInvitesListOptions{ListOptions: ListOptions{PerPage: 1}},
			reply: pageOf(t, invitesOf(0, 1), `,"total_count":3`),
			query: "per_page=1",
			want: &Page[models.ElementsUserInviteRead]{
				Data: invitesOf(0, 1), Page: 1, PerPage: 1, TotalCount: new(3),
			},
		},
		{
			name:  "a page after the last",
			opts:  &UserInvitesListOptions{ListOptions: ListOptions{Page: 3, PerPage: 30}},
			reply: pageOf(t, empty, `,"total_count":31,"page_count":0`),
			query: "page=3&per_page=30",
			want: &Page[models.ElementsUserInviteRead]{
				Data: empty, Page: 3, PerPage: 30, TotalCount: new(31), PageCount: new(0),
			},
		},
		{
			name:  "no invites",
			opts:  &UserInvitesListOptions{},
			reply: mockpermit.Reply{Body: inviteEmptyPage},
			want: &Page[models.ElementsUserInviteRead]{
				Data: empty, Page: 1, TotalCount: new(0), PageCount: new(0),
			},
		},
		{
			name:  "the role",
			opts:  &UserInvitesListOptions{Role: "a b"},
			reply: pageOf(t, empty, `,"total_count":0`),
			query: "role=a+b",
			want:  &Page[models.ElementsUserInviteRead]{Data: empty, Page: 1, TotalCount: new(0)},
		},
		{
			name:  "the tenant",
			opts:  &UserInvitesListOptions{Tenant: fixtures.UUIDKey},
			reply: pageOf(t, empty, `,"total_count":0`),
			query: "tenant=" + fixtures.UUIDKey,
			want:  &Page[models.ElementsUserInviteRead]{Data: empty, Page: 1, TotalCount: new(0)},
		},
		{
			name:  "the search",
			opts:  &UserInvitesListOptions{Search: "al&ice"},
			reply: pageOf(t, empty, `,"total_count":0`),
			query: "search=al%26ice",
			want:  &Page[models.ElementsUserInviteRead]{Data: empty, Page: 1, TotalCount: new(0)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listInvitesRoute, tc.reply)
			got, err := resourceClient(t, api).UserInvites.List(t.Context(), tc.opts)
			if err != nil {
				t.Fatalf("List returned %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" || got.Data == nil {
				t.Errorf("List returned a page that differs (-want +got), or nil Data:\n%s", diff)
			}
			if requests := api.Requests(); len(requests) != 1 ||
				requests[0].RawPath != invitesRaw || requests[0].RawQuery != tc.query {
				t.Errorf("List sent %+v, want one request to %s?%s", requests, invitesRaw,
					tc.query)
			}
		})
	}
}

// TestInvalidUserInviteListOptions checks that a Page or a PerPage less than zero is an error
// that matches ErrInvalidConfig from List and from All, which yields it once, and that neither
// sends a request.
func TestInvalidUserInviteListOptions(t *testing.T) {
	for _, page := range []ListOptions{{Page: -1}, {PerPage: -1}} {
		api := mockpermit.NewAPI(t)
		c := resourceClient(t, api)
		opts := &UserInvitesListOptions{ListOptions: page}
		_, listErr := c.UserInvites.List(t.Context(), opts)
		_, allErrs := collect(t, c.UserInvites.All(t.Context(), opts))
		if !errors.Is(listErr, ErrInvalidConfig) || len(allErrs) != 1 ||
			!errors.Is(allErrs[0], ErrInvalidConfig) {
			t.Errorf("%+v: List returned %v and All yielded %v, want one error each that matches "+
				"ErrInvalidConfig", page, listErr, allErrs)
		}
		api.AssertUntouched()
	}
}

// TestUserInvitesUseTheScope checks that the calls of the user invites act on the environment of
// the client's scope. Without WithScope, the client looks up the scope of its API key once, and
// each call fills the route with the project and the environment of an environment-level key. A
// scope without an environment, that of a project-level or an organization-level key, is an
// error that matches ErrInvalidConfig and names the operation, and no call sends a request but
// the lookup, which the API fails for any route that the test did not register.
func TestUserInvitesUseTheScope(t *testing.T) {
	const prefix = "/v2/facts/" + scopeProjectID + "/" + scopeEnvironmentID + "/user_invites"
	t.Run("an environment-level key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		for _, m := range inviteCalls() {
			api.Handle(m.route, m.reply)
		}
		c := projectClient(t, api)
		for _, m := range inviteCalls() {
			if err := m.call(t.Context(), c, "invite-1"); err != nil {
				t.Errorf("the call of %s returned %v", m.op, err)
			}
		}
		requests := api.Requests()
		if len(requests) != len(inviteCalls())+1 || requests[0].RawPath != "/v2/api-key/scope" {
			t.Fatalf("the calls sent %+v, want the lookup of the scope and one request for each "+
				"of the %d calls", requests, len(inviteCalls()))
		}
		for _, r := range requests[1:] {
			if r.RawPath != prefix && !strings.HasPrefix(r.RawPath, prefix+"/") {
				t.Errorf("a call sent its request to %s, want a path under %s", r.RawPath, prefix)
			}
		}
	})
	for _, suffix := range []string{".project", ".organization"} {
		t.Run("the scope of the fixture"+suffix, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(scopeRoute, scopeReply(t, suffix))
			c := projectClient(t, api)
			for _, m := range inviteCalls() {
				err := m.call(t.Context(), c, "invite-1")
				if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), m.op) {
					t.Errorf("the call of %s returned %v, want an error that matches "+
						"ErrInvalidConfig and names %s", m.op, err, m.op)
				}
			}
			if _, errs := collect(t, c.UserInvites.All(t.Context(), nil)); len(errs) != 1 ||
				!errors.Is(errs[0], ErrInvalidConfig) {
				t.Errorf("All yielded %v, want one error that matches ErrInvalidConfig", errs)
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("the calls sent %d requests, want only the lookup of the scope", n)
			}
		})
	}
}

// TestUserInvitesStayOnTheAPI checks that a client with WithFactsViaPDP sends each call of the
// user invites to the API, under /v2/facts/ with the project and the environment of its scope, as
// a client without it does, with no header of a wait for sync, and sends nothing to the PDP,
// whose server fails the test when anything reaches it.
func TestUserInvitesStayOnTheAPI(t *testing.T) {
	api := mockpermit.NewAPI(t)
	api.Handle(listInvitesRoute, mockpermit.Reply{Body: inviteEmptyPage},
		mockpermit.Reply{Body: inviteEmptyPage})
	for _, m := range inviteCalls()[1:] {
		api.Handle(m.route, m.reply)
	}
	c := resourceClient(t, api, WithFactsViaPDP())
	_, errs := collect(t, c.UserInvites.All(t.Context(), nil))
	for _, m := range inviteCalls() {
		errs = append(errs, m.call(t.Context(), c, "invite-1"))
	}
	for i, err := range errs {
		if err != nil {
			t.Errorf("call %d with WithFactsViaPDP returned %v", i, err)
		}
	}
	requests := api.Requests()
	if want := 1 + len(inviteCalls()); len(requests) != want {
		t.Fatalf("the API got %d requests, want %d", len(requests), want)
	}
	for _, r := range requests {
		if r.RawPath != invitesRaw && !strings.HasPrefix(r.RawPath, invitesRaw+"/") ||
			r.Header.Get("X-Wait-Timeout") != "" || r.Header.Get("X-Timeout-Policy") != "" {
			t.Errorf("a call sent %s with the headers %v, want a path under %s without a wait",
				r.RawPath, r.Header, invitesRaw)
		}
	}
}

// TestUserInviteIDsInThePath checks that each method that acts on one invite sends the ID it is
// given as one segment of the path, escaped, after the project and the environment of the scope,
// and that Approve sends it before /approve, whatever the ID holds: escaped where it holds a
// slash, a percent sign, a space, a question mark and a hash, or a letter outside ASCII, and as it
// is for the shape of a UUID, a key with an @, a leading dot and dots inside it. An ID that is
// empty, . or .. would change the route, so it is an error that matches ErrInvalidConfig and names
// the placeholder, and no request.
func TestUserInviteIDsInThePath(t *testing.T) {
	for id, segment := range map[string]string{
		"a/b":               "a%2Fb",
		"50%":               "50%25",
		"a b?c#d":           "a%20b%3Fc%23d",
		"é":                 "%C3%A9",
		fixtures.UUIDKey:    fixtures.UUIDKey,
		"alice@example.com": "alice@example.com",
		".hidden":           ".hidden",
		"v2..x":             "v2..x",
	} {
		for _, m := range inviteIDCalls {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, m.reply)
			if err := m.call(t.Context(), resourceClient(t, api), id); err != nil {
				t.Errorf("%s(%q) returned %v", m.name, id, err)
			}
			want := invitesRaw + "/" + segment
			if m.op == opApproveUserInvite {
				want += "/approve"
			}
			if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want {
				t.Errorf("%s(%q) sent %+v, want one request to %s", m.name, id, requests, want)
			}
		}
	}
	for _, id := range []string{"", ".", ".."} {
		for _, m := range inviteIDCalls {
			api := mockpermit.NewAPI(t)
			err := m.call(t.Context(), resourceClient(t, api), id)
			if !errors.Is(err, ErrInvalidConfig) ||
				!strings.Contains(err.Error(), "{user_invite_id}") {
				t.Errorf("%s(%q) returned %v, want an error that matches ErrInvalidConfig and "+
					"names {user_invite_id}", m.name, id, err)
			}
			api.AssertUntouched()
		}
	}
}

// TestDeleteUserInviteThatIsGone checks that Delete returns nil for a 404, with or without a
// body, as the invite that the path names is gone already, after one request to the route of the
// invite; and an *Error with the status for any other error status, such as 403, 409 or 422.
func TestDeleteUserInviteThatIsGone(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"NOT_FOUND","title":"Not Found","message":"the invite does not exist"}`,
		"",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteInviteRoute, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		if err := resourceClient(t, api).UserInvites.Delete(t.Context(), "gone"); err != nil {
			t.Errorf("Delete of an invite that the API answers with 404 and the body %q returned "+
				"%v, want nil", body, err)
		}
		requests := api.Requests()
		if want := invitesRaw + "/gone"; len(requests) != 1 || requests[0].RawPath != want {
			t.Errorf("Delete sent %+v, want one request to %s", requests, want)
		}
	}
	for _, status := range []int{
		http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteInviteRoute, mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := resourceClient(t, api).UserInvites.Delete(t.Context(), "invite-1")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteUserInvite ||
			e.Detail != "no" || errors.Is(err, ErrNotFound) {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status and the detail", status, err, opDeleteUserInvite)
		}
	}
}

// TestNotFoundOfOtherUserInviteCalls checks that a 404 to every call but Delete is an *Error that
// matches ErrNotFound, with or without a body: List, Create, Get, Update and Approve delete
// nothing, so none takes a 404 for an invite that is gone.
func TestNotFoundOfOtherUserInviteCalls(t *testing.T) {
	for _, m := range inviteCalls() {
		if m.op == opDeleteUserInvite {
			continue
		}
		for _, body := range []string{`{"detail":"Not Found"}`, ""} {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
			err := m.call(t.Context(), resourceClient(t, api), "gone")
			if e := mustError(t, err); !errors.Is(err, ErrNotFound) ||
				e.StatusCode != http.StatusNotFound || e.Op != m.op {
				t.Errorf("%s that the API answers with 404 and the body %q returned %v, want an "+
					"*Error of %s that matches ErrNotFound", m.name, body, err, m.op)
			}
		}
	}
}

// TestUserInviteBodies checks the bodies of Create, Update and Approve byte for byte against the
// golden files in testdata/golden: a create and an update with every field, whose values keep
// <, > and & as they are, and the zero create and update, which still send each field, empty, as
// the spec requires each; an approval with attributes, and approvals with nil and with empty
// attributes, which both send {}, and the zero approval, which still sends the email address and
// the key, empty.
func TestUserInviteBodies(t *testing.T) {
	every := models.ElementsUserInviteCreate{
		Key: "alice@example.com", Status: models.UserInviteStatusApproved,
		Email: "alice+invites@example.com", FirstName: "A" + fixtures.LineSeparator + "lice",
		LastName: "<Liddell> & co", RoleID: fixtures.UUIDKey, TenantID: "tenant-a",
		ResourceInstanceID: "00000000-0000-4000-8000-000000000413",
	}
	create := func(invite models.ElementsUserInviteCreate) func(context.Context, *Client) error {
		return func(ctx context.Context, c *Client) error {
			_, err := c.UserInvites.Create(ctx, invite)
			return err
		}
	}
	update := func(invite models.ElementsUserInviteUpdate) func(context.Context, *Client) error {
		return func(ctx context.Context, c *Client) error {
			_, err := c.UserInvites.Update(ctx, "invite-1", invite)
			return err
		}
	}
	approve := func(a models.ElementsUserInviteApprove) func(context.Context, *Client) error {
		return func(ctx context.Context, c *Client) error {
			_, err := c.UserInvites.Approve(ctx, "invite-1", a)
			return err
		}
	}
	noAttributes := models.ElementsUserInviteApprove{Email: "alice@example.com", Key: "alice"}
	emptyAttributes := noAttributes
	emptyAttributes.Attributes = models.Attributes{}
	for _, tc := range []struct {
		golden string
		route  string
		reply  string
		call   func(context.Context, *Client) error
	}{
		{"create_user_invite.every-field.golden", createInviteRoute, inviteReadBody, create(every)},
		{
			"create_user_invite.zero.golden", createInviteRoute, inviteReadBody,
			create(models.ElementsUserInviteCreate{}),
		},
		{
			"update_user_invite.every-field.golden", updateInviteRoute, inviteReadBody,
			update(models.ElementsUserInviteUpdate(every)),
		},
		{
			"update_user_invite.zero.golden", updateInviteRoute, inviteReadBody,
			update(models.ElementsUserInviteUpdate{}),
		},
		{
			"approve_user_invite.attributes.golden", approveInviteRoute, inviteUserBody,
			approve(inviteApproval),
		},
		{
			"approve_user_invite.nil-attributes.golden", approveInviteRoute, inviteUserBody,
			approve(noAttributes),
		},
		{
			"approve_user_invite.nil-attributes.golden", approveInviteRoute, inviteUserBody,
			approve(emptyAttributes),
		},
		{
			"approve_user_invite.zero.golden", approveInviteRoute, inviteUserBody,
			approve(models.ElementsUserInviteApprove{}),
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: tc.reply})
			if err := tc.call(t.Context(), resourceClient(t, api)); err != nil {
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

// TestUserInviteRetries checks when a call of the user invites sends its request again: Create
// and Approve, which the API may have acted on, after a 429 only, so a 503 ends each after one
// request; the other calls, which do the same when sent twice, after a 503 too.
func TestUserInviteRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	type retryCase struct {
		name    string
		route   string
		replies []mockpermit.Reply
		call    func(c *Client) error
		status  int
	}
	var tests []retryCase
	for _, m := range inviteCalls() {
		call := func(c *Client) error { return m.call(t.Context(), c, "invite-1") }
		if m.op == opCreateUserInvite || m.op == opApproveUserInvite {
			tests = append(tests,
				retryCase{
					name: m.name + " after a 429", route: m.route,
					replies: []mockpermit.Reply{limited, m.reply}, call: call,
				},
				retryCase{
					name: m.name + " after a 503", route: m.route,
					replies: []mockpermit.Reply{unavailable}, call: call,
					status: http.StatusServiceUnavailable,
				})
			continue
		}
		tests = append(tests, retryCase{
			name: m.name + " after a 503", route: m.route,
			replies: []mockpermit.Reply{unavailable, m.reply}, call: call,
		})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, tc.replies...)
			err := tc.call(resourceClient(t, api, WithRetry(RetryPolicy{MaxRetries: 1})))
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

// TestMalformedUserInviteReplies checks that a reply in 200-299 that is not an invite, or not a
// user for an approval, or for the list not the paginated object of invites, such as a JSON array
// of them or an object without data, is an *Error that names the model, and nothing else.
func TestMalformedUserInviteReplies(t *testing.T) {
	const prefix = "permit: "
	objects := []struct {
		route, model string
		call         func(c *Client) (any, error)
	}{
		{
			getInviteRoute, "get_user_invite: GET " + invitePath +
				": status 200: the reply is not a valid models.ElementsUserInviteRead",
			func(c *Client) (any, error) { return c.UserInvites.Get(t.Context(), "invite-1") },
		},
		{
			createInviteRoute, "create_user_invite: POST " + invitesPath +
				": status 200: the reply is not a valid models.ElementsUserInviteRead",
			func(c *Client) (any, error) { return c.UserInvites.Create(t.Context(), inviteFields) },
		},
		{
			updateInviteRoute, "update_user_invite: PATCH " + invitePath +
				": status 200: the reply is not a valid models.ElementsUserInviteRead",
			func(c *Client) (any, error) {
				return c.UserInvites.Update(t.Context(), "invite-1",
					models.ElementsUserInviteUpdate(inviteFields))
			},
		},
		{
			approveInviteRoute, "approve_user_invite: POST " + invitePath + "/approve" +
				": status 200: the reply is not a valid models.UserRead",
			func(c *Client) (any, error) {
				return c.UserInvites.Approve(t.Context(), "invite-1", inviteApproval)
			},
		},
	}
	for _, o := range objects {
		for _, body := range []string{``, `null`, `[]`, `"i"`, `{"key":1}`, `{} {}`} {
			api := mockpermit.NewAPI(t)
			api.Handle(o.route, mockpermit.Reply{Body: body})
			got, err := o.call(resourceClient(t, api))
			if e := mustError(t, err); !inviteNilResult(got) || e.Error() != prefix+o.model {
				t.Errorf("%s of the reply %q returned %+v and\n%v\nwant nothing and\n%s",
					o.route, body, got, err, prefix+o.model)
			}
		}
	}
	model := "list_user_invites: GET " + invitesPath +
		": status 200: the reply is not a valid list of models.ElementsUserInviteRead"
	for _, body := range []string{
		``, `null`, `"i"`, `[]`, `[{"key":"a"}]`, `{"total_count":0}`,
		`{"data":null,"total_count":0}`, `{"data":[{"key":1}],"total_count":1}`,
		`{"data":[],"total_count":"0"}`,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(listInvitesRoute, mockpermit.Reply{Body: body})
		got, err := resourceClient(t, api).UserInvites.List(t.Context(), nil)
		if e := mustError(t, err); got != nil || e.Error() != prefix+model {
			t.Errorf("List of the reply %q returned %+v and\n%v\nwant no page and\n%s", body, got,
				err, prefix+model)
		}
	}
	for _, body := range []string{`[]`, `[{"key":"a"}]`} {
		api := mockpermit.NewAPI(t)
		api.Handle(listInvitesRoute, mockpermit.Reply{Body: body})
		items, errs := collect(t, resourceClient(t, api).UserInvites.All(t.Context(), nil))
		if len(items) != 0 || len(errs) != 1 || mustError(t, errs[0]).Error() != prefix+model {
			t.Errorf("All of the reply %q yielded %d invites and the errors %v\nwant none and "+
				"one error:\n%s", body, len(items), errs, prefix+model)
		}
	}
}

// inviteNilResult reports whether v, the result of a call of the user invites, is a nil pointer,
// as a call that fails returns.
func inviteNilResult(v any) bool {
	p := reflect.ValueOf(v)
	return p.Kind() == reflect.Pointer && p.IsNil()
}

// TestUserInvitesAfterClose checks that each method returns ErrClosed after Close, and sends
// nothing, also without WithScope, whose lookup it does not send either.
func TestUserInvitesAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	var errs []error
	for _, m := range inviteCalls() {
		errs = append(errs, m.call(t.Context(), c, "invite-1"))
	}
	_, allErrs := collect(t, c.UserInvites.All(t.Context(), nil))
	if len(allErrs) != 1 {
		t.Errorf("All after Close yielded %v, want ErrClosed once", allErrs)
	}
	for i, err := range append(errs, allErrs...) {
		if !errors.Is(err, ErrClosed) {
			t.Errorf("call %d after Close returned %v, want ErrClosed", i, err)
		}
	}
	api.AssertUntouched()
}
