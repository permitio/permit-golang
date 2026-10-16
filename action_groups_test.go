package permit

import (
	"context"
	"errors"
	"math"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The routes of the action groups of a resource, as mockpermit.Server.Handle takes them.
const (
	actionGroupsPath        = "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/action_groups"
	actionGroupPath         = actionGroupsPath + "/{action_group_id}"
	listActionGroupsRoute   = "GET " + actionGroupsPath
	createActionGroupRoute  = "POST " + actionGroupsPath
	getActionGroupRoute     = "GET " + actionGroupPath
	updateActionGroupRoute  = "PATCH " + actionGroupPath
	deleteActionGroupRoute  = "DELETE " + actionGroupPath
	documentActionGroupsRaw = "/v2/schema/p/e/resources/document/action_groups"
)

// actionGroupBody is the body of a reply of the API that holds the action group editing of the
// resource document, with the actions read and write.
const actionGroupBody = `{"name":"Editing","actions":["read","write"],"key":"editing",` +
	`"id":"00000000-0000-4000-8000-000000000081",` +
	`"organization_id":"00000000-0000-4000-8000-000000000001",` +
	`"project_id":"00000000-0000-4000-8000-000000000002",` +
	`"environment_id":"00000000-0000-4000-8000-000000000003",` +
	`"resource_id":"00000000-0000-4000-8000-000000000021",` +
	`"created_at":"2026-10-14T09:30:00Z","updated_at":"2026-10-14T09:30:00Z"}`

// actionGroupsOf returns the action groups g<from> to g<to-1>, with only a key and a name: an
// empty list when to is from.
func actionGroupsOf(from, to int) []models.ResourceActionGroupRead {
	items := make([]models.ResourceActionGroupRead, 0, to-from)
	for i := from; i < to; i++ {
		key := "g" + strconv.Itoa(i)
		items = append(items, models.ResourceActionGroupRead{Key: key, Name: key})
	}
	return items
}

// actionGroupCalls are calls of each method of the action groups that acts on one action group,
// with the ID of its operation, its route and a reply of the API to it. Each call takes the key
// of the resource and the key of the action group.
var actionGroupCalls = []struct {
	name  string
	op    string
	route string
	reply mockpermit.Reply
	call  func(ctx context.Context, c *Client, resourceKey, groupKey string) error
}{
	{
		name: "Get", op: opGetResourceActionGroup, route: getActionGroupRoute,
		reply: mockpermit.Reply{Body: actionGroupBody},
		call: func(ctx context.Context, c *Client, resourceKey, groupKey string) error {
			_, err := c.ResourceActionGroups.Get(ctx, resourceKey, groupKey)
			return err
		},
	},
	{
		name: "Update", op: opUpdateResourceActionGroup, route: updateActionGroupRoute,
		reply: mockpermit.Reply{Body: actionGroupBody},
		call: func(ctx context.Context, c *Client, resourceKey, groupKey string) error {
			_, err := c.ResourceActionGroups.Update(ctx, resourceKey, groupKey,
				models.ResourceActionGroupUpdate{Name: "n"})
			return err
		},
	},
	{
		name: "Delete", op: opDeleteResourceActionGroup, route: deleteActionGroupRoute,
		reply: mockpermit.Reply{Status: http.StatusNoContent},
		call: func(ctx context.Context, c *Client, resourceKey, groupKey string) error {
			return c.ResourceActionGroups.Delete(ctx, resourceKey, groupKey)
		},
	},
}

// resourceCallsOfActionGroups are calls of each method of the action groups that names a
// resource but no action group, with the ID of its operation, its route and a reply of the API
// to it.
var resourceCallsOfActionGroups = []struct {
	name  string
	op    string
	route string
	reply mockpermit.Reply
	call  func(ctx context.Context, c *Client, resourceKey string) error
}{
	{
		name: "List", op: opListResourceActionGroups, route: listActionGroupsRoute,
		reply: mockpermit.Reply{Body: `[]`},
		call: func(ctx context.Context, c *Client, resourceKey string) error {
			_, err := c.ResourceActionGroups.List(ctx, resourceKey, nil)
			return err
		},
	},
	{
		name: "Create", op: opCreateResourceActionGroup, route: createActionGroupRoute,
		reply: mockpermit.Reply{Body: actionGroupBody},
		call: func(ctx context.Context, c *Client, resourceKey string) error {
			_, err := c.ResourceActionGroups.Create(ctx, resourceKey,
				models.ResourceActionGroupCreate{Key: "k", Name: "n"})
			return err
		},
	},
}

// TestResourceActionGroupsAllReadsEveryPage checks that All reads the 31 action groups of a
// resource, in order, asking for each page in turn: at a per_page of 30, two pages of action
// groups and the empty page after them; at the default per_page of 100, the most that the API
// gives on a page, one page of 31 and the empty page after it.
func TestResourceActionGroupsAllReadsEveryPage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    *ListOptions
		pages   [][]models.ResourceActionGroupRead
		queries []string
	}{
		{
			name: "per_page 30", opts: &ListOptions{PerPage: 30},
			pages: [][]models.ResourceActionGroupRead{
				actionGroupsOf(0, 30), actionGroupsOf(30, 31), {},
			},
			queries: []string{"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30"},
		},
		{
			name:    "the default per_page",
			pages:   [][]models.ResourceActionGroupRead{actionGroupsOf(0, 31), {}},
			queries: []string{"page=1&per_page=100", "page=2&per_page=100"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			var replies []mockpermit.Reply
			for _, items := range tc.pages {
				replies = append(replies, arrayOf(t, items))
			}
			api.Handle(listActionGroupsRoute, replies...)
			items, errs := collect(t, resourceClient(t, api).ResourceActionGroups.All(
				t.Context(), "document", tc.opts))
			if diff := cmp.Diff(actionGroupsOf(0, 31), items); diff != "" || errs != nil {
				t.Errorf("All yielded the errors %v and action groups that differ "+
					"(-want +got):\n%s", errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
			for _, r := range api.Requests() {
				if r.RawPath != documentActionGroupsRaw {
					t.Errorf("All sent a request to %s, want %s", r.RawPath,
						documentActionGroupsRaw)
				}
			}
		})
	}
}

// TestResourceActionGroupsAllReadsTheOptionsOnce checks that the iterator keeps the options of
// its call, so that a later change to them changes no request, that a loop that breaks asks for
// no other page, and that each loop over the iterator reads the action groups again from the
// first page.
func TestResourceActionGroupsAllReadsTheOptionsOnce(t *testing.T) {
	api := mockpermit.NewAPI(t)
	two := arrayOf(t, actionGroupsOf(0, 2))
	api.Handle(listActionGroupsRoute, two, two)
	opts := &ListOptions{PerPage: 2}
	seq := resourceClient(t, api).ResourceActionGroups.All(t.Context(), "document", opts)
	opts.Page, opts.PerPage = 5, 50
	for loop := 1; loop <= 2; loop++ {
		for group, err := range seq {
			if err != nil || group.Key != "g0" {
				t.Errorf("loop %d yielded %+v and %v first, want g0", loop, group, err)
			}
			break
		}
	}
	first := "page=1&per_page=2"
	if diff := cmp.Diff([]string{first, first}, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestResourceActionGroupsAllYieldsAnErrorOnce checks that a page that fails ends the iterator
// with its error, once, after the action groups of the pages before it.
func TestResourceActionGroupsAllYieldsAnErrorOnce(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(listActionGroupsRoute, arrayOf(t, actionGroupsOf(0, 30)),
		mockpermit.Reply{Status: http.StatusForbidden, Body: body})
	items, errs := collect(t, resourceClient(t, api).ResourceActionGroups.All(t.Context(),
		"document", &ListOptions{PerPage: 30}))
	if len(items) != 30 || len(errs) != 1 || !errors.Is(errs[0], ErrForbidden) {
		t.Errorf("All yielded %d action groups and the errors %v, want 30 and one error that "+
			"matches ErrForbidden", len(items), errs)
	}
	if n := len(api.Requests()); n != 2 {
		t.Errorf("All sent %d requests, want 2", n)
	}
}

// TestResourceActionGroupsAllGetsTheScopeInEachLoop checks that All, without WithScope, looks up
// the scope in each loop over the iterator, before its first page, and not when it is called: a
// lookup that fails ends that loop with its error, and the next loop looks the scope up again.
func TestResourceActionGroupsAllGetsTheScopeInEachLoop(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(scopeRoute, mockpermit.Reply{Status: http.StatusForbidden, Body: body},
		scopeReply(t, ""))
	api.Handle(listActionGroupsRoute, arrayOf(t, actionGroupsOf(0, 1)),
		arrayOf(t, actionGroupsOf(1, 1)))
	seq := projectClient(t, api).ResourceActionGroups.All(t.Context(), "document", nil)
	if requests := api.Requests(); len(requests) != 0 {
		t.Fatalf("All sent %+v before a loop, want no request", requests)
	}
	if items, errs := collect(t, seq); items != nil || len(errs) != 1 ||
		!errors.Is(errs[0], ErrForbidden) {
		t.Errorf("the first loop yielded %v and the errors %v, want one error that matches "+
			"ErrForbidden", items, errs)
	}
	if items, errs := collect(t, seq); !cmp.Equal(actionGroupsOf(0, 1), items) || errs != nil {
		t.Errorf("the second loop yielded %v and the errors %v, want g0", items, errs)
	}
	var paths []string
	for _, r := range api.Requests() {
		paths = append(paths, r.RawPath)
	}
	groups := "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID +
		"/resources/document/action_groups"
	want := []string{"/v2/api-key/scope", "/v2/api-key/scope", groups, groups}
	if diff := cmp.Diff(want, paths); diff != "" {
		t.Errorf("the requests differ (-want +got):\n%s", diff)
	}
}

// TestResourceActionGroupsListReturnsAPage checks the page that List returns: the action groups
// of the reply in its order, the page and the per_page that it asked for, page 1 and no query
// for nil options, and no counts, as the reply is a JSON array. A page without action groups has
// an empty list, not nil.
func TestResourceActionGroupsListReturnsAPage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  *ListOptions
		items []models.ResourceActionGroupRead
		query string
		want  *Page[models.ResourceActionGroupRead]
	}{
		{
			name: "nil options", items: actionGroupsOf(0, 2), query: "",
			want: &Page[models.ResourceActionGroupRead]{Data: actionGroupsOf(0, 2), Page: 1},
		},
		{
			name: "a page after the last", opts: &ListOptions{Page: 3, PerPage: 30},
			items: []models.ResourceActionGroupRead{}, query: "page=3&per_page=30",
			want: &Page[models.ResourceActionGroupRead]{
				Data: []models.ResourceActionGroupRead{}, Page: 3, PerPage: 30,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listActionGroupsRoute, arrayOf(t, tc.items))
			got, err := resourceClient(t, api).ResourceActionGroups.List(t.Context(), "document",
				tc.opts)
			if err != nil {
				t.Fatalf("List returned %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" || got.Data == nil {
				t.Errorf("List returned a page that differs (-want +got), or nil Data:\n%s", diff)
			}
			if requests := api.Requests(); len(requests) != 1 ||
				requests[0].RawPath != documentActionGroupsRaw ||
				requests[0].RawQuery != tc.query {
				t.Errorf("List sent %+v, want one request to %s?%s", requests,
					documentActionGroupsRaw, tc.query)
			}
		})
	}
}

// TestInvalidResourceActionGroupListOptions checks that a Page or a PerPage less than zero is an
// error that matches ErrInvalidConfig from List and All, and that neither sends a request.
func TestInvalidResourceActionGroupListOptions(t *testing.T) {
	for _, opts := range []ListOptions{{Page: -1}, {PerPage: -1}} {
		api := mockpermit.NewAPI(t)
		c := resourceClient(t, api)
		_, err := c.ResourceActionGroups.List(t.Context(), "document", &opts)
		_, errs := collect(t, c.ResourceActionGroups.All(t.Context(), "document", &opts))
		for _, err := range append([]error{err}, errs...) {
			if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("%+v: the call returned %v, want an error that matches ErrInvalidConfig",
					opts, err)
			}
		}
		if len(errs) != 1 {
			t.Errorf("%+v: All yielded the errors %v, want one", opts, errs)
		}
		api.AssertUntouched()
	}
}

// TestResourceActionGroupsUseTheScope checks that the calls of the action groups act on the
// environment of the client's scope. Without WithScope, the client looks up the scope of its API
// key once, and each call fills the route with the project and the environment of an
// environment-level key. A scope without an environment, that of a project-level or an
// organization-level key, is an error that matches ErrInvalidConfig and names the operation, and
// no call sends a request but the lookup, which the API fails for any route that the test did
// not register.
func TestResourceActionGroupsUseTheScope(t *testing.T) {
	const prefix = "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID +
		"/resources/document/action_groups"
	calls := map[string]func(ctx context.Context, c *Client) error{}
	for _, m := range resourceCallsOfActionGroups {
		calls[m.op] = func(ctx context.Context, c *Client) error {
			return m.call(ctx, c, "document")
		}
	}
	for _, m := range actionGroupCalls {
		calls[m.op] = func(ctx context.Context, c *Client) error {
			return m.call(ctx, c, "document", "editing")
		}
	}
	t.Run("an environment-level key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		for _, m := range resourceCallsOfActionGroups {
			api.Handle(m.route, m.reply)
		}
		for _, m := range actionGroupCalls {
			api.Handle(m.route, m.reply)
		}
		c := projectClient(t, api)
		for op, call := range calls {
			if err := call(t.Context(), c); err != nil {
				t.Errorf("the call of %s returned %v", op, err)
			}
		}
		requests := api.Requests()
		if len(requests) != len(calls)+1 || requests[0].RawPath != "/v2/api-key/scope" {
			t.Fatalf("the calls sent %+v, want the lookup of the scope and one request for each "+
				"of the %d calls", requests, len(calls))
		}
		for _, r := range requests[1:] {
			if r.RawPath != prefix && r.RawPath != prefix+"/editing" {
				t.Errorf("a call sent its request to %s, want a path under %s", r.RawPath, prefix)
			}
		}
	})
	for _, suffix := range []string{".project", ".organization"} {
		t.Run("the scope of the fixture"+suffix, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(scopeRoute, scopeReply(t, suffix))
			c := projectClient(t, api)
			for op, call := range calls {
				err := call(t.Context(), c)
				if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), op) {
					t.Errorf("the call of %s returned %v, want an error that matches "+
						"ErrInvalidConfig and names %s", op, err, op)
				}
			}
			_, errs := collect(t, c.ResourceActionGroups.All(t.Context(), "document", nil))
			if len(errs) != 1 || !errors.Is(errs[0], ErrInvalidConfig) {
				t.Errorf("All yielded %v, want one error that matches ErrInvalidConfig", errs)
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("the calls sent %d requests, want only the lookup of the scope", n)
			}
		})
	}
}

// TestResourceActionGroupKeysInThePath checks that each method sends the key of the resource,
// and the key of the action group when it names one, each as one segment of the path, escaped,
// after the project and the environment of the scope, whatever the key holds: escaped where the
// key holds a slash, a percent sign, a space, a question mark and a hash, or a letter outside
// ASCII, and as it is for the shape of a UUID, a leading dot and dots inside a key. A key that is
// empty, . or .. would change the route, so it is an error that matches ErrInvalidConfig and
// names its placeholder, and no request.
func TestResourceActionGroupKeysInThePath(t *testing.T) {
	const prefix = "/v2/schema/p/e/resources/"
	for key, segment := range map[string]string{
		"a/b":            "a%2Fb",
		"50%":            "50%25",
		"a b?c#d":        "a%20b%3Fc%23d",
		"é":              "%C3%A9",
		fixtures.UUIDKey: fixtures.UUIDKey,
		".hidden":        ".hidden",
		"doc.v2..x":      "doc.v2..x",
	} {
		for _, m := range resourceCallsOfActionGroups {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, m.reply)
			if err := m.call(t.Context(), resourceClient(t, api), key); err != nil {
				t.Errorf("%s(%q) returned %v", m.name, key, err)
			}
			want := prefix + segment + "/action_groups"
			if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want {
				t.Errorf("%s(%q) sent %+v, want one request to %s", m.name, key, requests, want)
			}
		}
		for _, m := range actionGroupCalls {
			for _, keys := range [][2]string{{key, "editing"}, {"document", key}} {
				api := mockpermit.NewAPI(t)
				api.Handle(m.route, m.reply)
				err := m.call(t.Context(), resourceClient(t, api), keys[0], keys[1])
				if err != nil {
					t.Errorf("%s(%q, %q) returned %v", m.name, keys[0], keys[1], err)
				}
				want := prefix + "document/action_groups/" + segment
				if keys[0] == key {
					want = prefix + segment + "/action_groups/editing"
				}
				if requests := api.Requests(); len(requests) != 1 ||
					requests[0].RawPath != want {
					t.Errorf("%s(%q, %q) sent %+v, want one request to %s", m.name, keys[0],
						keys[1], requests, want)
				}
			}
		}
	}
	for _, key := range []string{"", ".", ".."} {
		for _, m := range resourceCallsOfActionGroups {
			api := mockpermit.NewAPI(t)
			err := m.call(t.Context(), resourceClient(t, api), key)
			checkActionGroupKeyError(t, m.name, key, "{resource_id}", err)
			api.AssertUntouched()
		}
		for _, m := range actionGroupCalls {
			api := mockpermit.NewAPI(t)
			c := resourceClient(t, api)
			checkActionGroupKeyError(t, m.name, key, "{resource_id}",
				m.call(t.Context(), c, key, "editing"))
			checkActionGroupKeyError(t, m.name, key, "{action_group_id}",
				m.call(t.Context(), c, "document", key))
			api.AssertUntouched()
		}
	}
}

// checkActionGroupKeyError checks that err, the error of the call of method with key in the
// placeholder, matches ErrInvalidConfig and names the placeholder.
func checkActionGroupKeyError(t *testing.T, method, key, placeholder string, err error) {
	t.Helper()
	if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), placeholder) {
		t.Errorf("%s with %q in %s returned %v, want an error that matches ErrInvalidConfig "+
			"and names %s", method, key, placeholder, err, placeholder)
	}
}

// TestDeleteResourceActionGroupThatIsGone checks that Delete returns nil for a 404, with or
// without a body, as the action group that the path names is gone already, after one request to
// the route of the action group; and an *Error with the status for any other error status, such
// as 403, 409 or 422.
func TestDeleteResourceActionGroupThatIsGone(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"NOT_FOUND","title":"Not Found","message":"the group does not exist"}`,
		"",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteActionGroupRoute,
			mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		err := resourceClient(t, api).ResourceActionGroups.Delete(t.Context(), "document", "gone")
		if err != nil {
			t.Errorf("Delete of an action group that the API answers with 404 and the body %q "+
				"returned %v, want nil", body, err)
		}
		requests := api.Requests()
		if want := documentActionGroupsRaw + "/gone"; len(requests) != 1 ||
			requests[0].RawPath != want {
			t.Errorf("Delete sent %+v, want one request to %s", requests, want)
		}
	}
	for _, status := range []int{
		http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteActionGroupRoute,
			mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := resourceClient(t, api).ResourceActionGroups.Delete(t.Context(), "document",
			"editing")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteResourceActionGroup ||
			e.Detail != "no" || errors.Is(err, ErrNotFound) {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status and the detail", status, err, opDeleteResourceActionGroup)
		}
	}
}

// TestNotFoundOfOtherResourceActionGroupCalls checks that a 404 to List, Create, Get and Update,
// which delete nothing, is an *Error that matches ErrNotFound.
func TestNotFoundOfOtherResourceActionGroupCalls(t *testing.T) {
	const body = `{"error_code":"NOT_FOUND","title":"Not Found","message":"no resource"}`
	notFound := mockpermit.Reply{Status: http.StatusNotFound, Body: body}
	calls := map[string]func(c *Client) error{}
	for _, m := range resourceCallsOfActionGroups {
		calls[m.route] = func(c *Client) error { return m.call(t.Context(), c, "gone") }
	}
	for _, m := range actionGroupCalls[:2] {
		calls[m.route] = func(c *Client) error {
			return m.call(t.Context(), c, "document", "gone")
		}
	}
	for route, call := range calls {
		api := mockpermit.NewAPI(t)
		api.Handle(route, notFound)
		err := call(resourceClient(t, api))
		if e := mustError(t, err); !errors.Is(err, ErrNotFound) ||
			e.StatusCode != http.StatusNotFound {
			t.Errorf("the call of %s that the API answers with 404 returned %v, want an *Error "+
				"that matches ErrNotFound", route, err)
		}
	}
}

// TestResourceActionGroupBodies checks the bodies of Create and Update byte for byte against the
// golden files in testdata/golden, with each state of the Optional fields: a create with the
// required fields only, one with empty actions and attributes, which send [] and {}, and one with
// every field; the zero update, which sends {}; an update of the name alone, which sends no
// actions; one that sends null for the description and the actions; one that sends [] for the
// actions and {} for the attributes; and one that sets every field.
func TestResourceActionGroupBodies(t *testing.T) {
	create := func(group models.ResourceActionGroupCreate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.ResourceActionGroups.Create(t.Context(), "r", group)
			return err
		}
	}
	update := func(change models.ResourceActionGroupUpdate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.ResourceActionGroups.Update(t.Context(), "r", "g", change)
			return err
		}
	}
	attributes := models.Attributes{"n": fixtures.TwoPow53Plus1, "s": "<a> & b"}
	for _, tc := range []struct {
		golden string
		route  string
		call   func(c *Client) error
	}{
		{
			"create_resource_action_group.required.golden", createActionGroupRoute,
			create(models.ResourceActionGroupCreate{Key: "g", Name: "G"}),
		},
		{
			"create_resource_action_group.empty.golden", createActionGroupRoute,
			create(models.ResourceActionGroupCreate{
				Key: "g", Name: "G", Attributes: models.Attributes{}, Actions: []string{},
			}),
		},
		{
			"create_resource_action_group.every-field.golden", createActionGroupRoute,
			create(models.ResourceActionGroupCreate{
				Key: "g", Name: "G", Description: "a" + fixtures.LineSeparator + "b",
				Attributes: attributes, Actions: []string{"read", "write"},
			}),
		},
		{
			"update_resource_action_group.unset.golden", updateActionGroupRoute,
			update(models.ResourceActionGroupUpdate{}),
		},
		{
			"update_resource_action_group.name.golden", updateActionGroupRoute,
			update(models.ResourceActionGroupUpdate{Name: "G"}),
		},
		{
			"update_resource_action_group.null.golden", updateActionGroupRoute,
			update(models.ResourceActionGroupUpdate{
				Description: models.Null[string](), Actions: models.Null[[]string](),
			}),
		},
		{
			"update_resource_action_group.empty.golden", updateActionGroupRoute,
			update(models.ResourceActionGroupUpdate{
				Attributes: models.Attributes{}, Actions: models.Some([]string{}),
			}),
		},
		{
			"update_resource_action_group.values.golden", updateActionGroupRoute,
			update(models.ResourceActionGroupUpdate{
				Name: "G", Description: models.Some(""), Attributes: attributes,
				Actions: models.Some([]string{"read", "write"}),
			}),
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: actionGroupBody})
			if err := tc.call(resourceClient(t, api)); err != nil {
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

// TestResourceActionGroupBodiesThatCannotBeJSON checks that a create and an update that hold a
// value that JSON cannot hold in the attributes of the action group, and an update whose actions
// are Some of a nil slice, which would send null in place of a value, are errors that match
// ErrInvalidConfig and name the operation, and send no request.
func TestResourceActionGroupBodiesThatCannotBeJSON(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := resourceClient(t, api)
	_, createErr := c.ResourceActionGroups.Create(t.Context(), "r",
		models.ResourceActionGroupCreate{
			Key: "g", Name: "G", Attributes: models.Attributes{"c": make(chan int)},
		})
	_, updateErr := c.ResourceActionGroups.Update(t.Context(), "r", "g",
		models.ResourceActionGroupUpdate{Attributes: models.Attributes{"n": math.NaN()}})
	_, nilErr := c.ResourceActionGroups.Update(t.Context(), "r", "g",
		models.ResourceActionGroupUpdate{Actions: models.Some[[]string](nil)})
	for i, tc := range []struct {
		op  string
		err error
	}{
		{opCreateResourceActionGroup, createErr},
		{opUpdateResourceActionGroup, updateErr},
		{opUpdateResourceActionGroup, nilErr},
	} {
		if !errors.Is(tc.err, ErrInvalidConfig) || !strings.Contains(tc.err.Error(), tc.op) {
			t.Errorf("call %d returned %v, want an error that matches ErrInvalidConfig and names "+
				"%s", i, tc.err, tc.op)
		}
	}
	api.AssertUntouched()
}

// TestResourceActionGroupActionsReadBack checks the actions of an action group that a reply
// holds: the keys in their order, an empty slice that is not nil for [], and nil for null or
// for a reply without them.
func TestResourceActionGroupActionsReadBack(t *testing.T) {
	for _, tc := range []struct {
		actions string
		want    []string
	}{
		{`"actions":["write","read"],`, []string{"write", "read"}},
		{`"actions":[],`, []string{}},
		{`"actions":null,`, nil},
		{``, nil},
	} {
		body := strings.Replace(actionGroupBody, `"actions":["read","write"],`, tc.actions, 1)
		api := mockpermit.NewAPI(t)
		api.Handle(getActionGroupRoute, mockpermit.Reply{Body: body})
		group, err := resourceClient(t, api).ResourceActionGroups.Get(t.Context(), "document",
			"editing")
		if err != nil {
			t.Fatal(err)
		}
		if !cmp.Equal(tc.want, group.Actions) || (tc.want == nil) != (group.Actions == nil) {
			t.Errorf("Get of the reply %s returned the actions %#v, want %#v", body,
				group.Actions, tc.want)
		}
	}
}

// TestResourceActionGroupRetries checks when a call of the action groups sends its request
// again: Create, which the API may have acted on, after a 429 only, so a 503 ends it after one
// request; Update, Get, Delete and List, which do the same when sent twice, after a 503 too.
func TestResourceActionGroupRetries(t *testing.T) {
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
	tests := []retryCase{}
	for _, m := range resourceCallsOfActionGroups {
		call := func(c *Client) error { return m.call(t.Context(), c, "document") }
		if m.op == opCreateResourceActionGroup {
			tests = append(tests,
				retryCase{
					name: "Create after a 429", route: m.route,
					replies: []mockpermit.Reply{limited, m.reply}, call: call,
				},
				retryCase{
					name: "Create after a 503", route: m.route,
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
	for _, m := range actionGroupCalls {
		tests = append(tests, retryCase{
			name: m.name + " after a 503", route: m.route,
			replies: []mockpermit.Reply{unavailable, m.reply},
			call:    func(c *Client) error { return m.call(t.Context(), c, "document", "editing") },
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

// TestMalformedResourceActionGroupReplies checks that a reply in 200-299 that is not an action
// group, or for List not a JSON array of action groups, such as the paginated object of them,
// is an *Error that names the model, and no action group.
func TestMalformedResourceActionGroupReplies(t *testing.T) {
	const prefix = "permit: "
	for _, body := range []string{``, `null`, `[]`, `"g"`, `{"key":1}`, `{} {}`} {
		api := mockpermit.NewAPI(t)
		api.Handle(getActionGroupRoute, mockpermit.Reply{Body: body})
		group, err := resourceClient(t, api).ResourceActionGroups.Get(t.Context(), "document",
			"editing")
		want := prefix + "get_resource_action_group: GET " + actionGroupPath +
			": status 200: the reply is not a valid models.ResourceActionGroupRead"
		if e := mustError(t, err); group != nil || e.Error() != want {
			t.Errorf("Get of the reply %q returned %+v and\n%v\nwant no action group and\n%s",
				body, group, err, want)
		}
	}
	for _, body := range []string{
		``, `null`, `"g"`, `[1]`, `[{"key":1}]`, `{"data":[]}`, `{"actions":"read"}`,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(listActionGroupsRoute, mockpermit.Reply{Body: body})
		page, err := resourceClient(t, api).ResourceActionGroups.List(t.Context(), "document",
			nil)
		want := prefix + "list_resource_action_groups: GET " + actionGroupsPath +
			": status 200: the reply is not a valid list of models.ResourceActionGroupRead"
		if e := mustError(t, err); page != nil || e.Error() != want {
			t.Errorf("List of the reply %q returned %+v and\n%v\nwant no page and\n%s", body,
				page, err, want)
		}
	}
}

// TestResourceActionGroupsAfterClose checks that each method returns ErrClosed after Close, and
// sends nothing, also without WithScope, whose lookup it does not send either.
func TestResourceActionGroupsAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	var errs []error
	for _, m := range resourceCallsOfActionGroups {
		errs = append(errs, m.call(t.Context(), c, "document"))
	}
	for _, m := range actionGroupCalls {
		errs = append(errs, m.call(t.Context(), c, "document", "editing"))
	}
	_, allErrs := collect(t, c.ResourceActionGroups.All(t.Context(), "document", nil))
	for i, err := range append(errs, allErrs...) {
		if !errors.Is(err, ErrClosed) {
			t.Errorf("call %d after Close returned %v, want ErrClosed", i, err)
		}
	}
	if len(allErrs) != 1 {
		t.Errorf("All after Close yielded %v, want ErrClosed once", allErrs)
	}
	api.AssertUntouched()
}
