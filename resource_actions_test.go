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
	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/models"
)

// The routes of the actions of a resource, as mockpermit.Server.Handle takes them.
const (
	listResourceActionsRoute  = "GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/actions"
	createResourceActionRoute = "POST /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/" +
		"actions"
	getResourceActionRoute = "GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/" +
		"actions/{action_id}"
	updateResourceActionRoute = "PATCH /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/" +
		"actions/{action_id}"
	deleteResourceActionRoute = "DELETE /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/" +
		"actions/{action_id}"
)

// resourceActionBody is the body of a reply of the API that holds the action read of the
// resource document.
const resourceActionBody = `{"name":"Read","key":"read",` +
	`"id":"00000000-0000-4000-8000-000000000081","permission_name":"document:read",` +
	`"organization_id":"00000000-0000-4000-8000-000000000001",` +
	`"project_id":"00000000-0000-4000-8000-000000000002",` +
	`"environment_id":"00000000-0000-4000-8000-000000000003",` +
	`"resource_id":"00000000-0000-4000-8000-000000000021",` +
	`"created_at":"2026-10-14T09:30:00Z","updated_at":"2026-10-14T09:30:00Z"}`

// resourceActionsOf returns the actions a<from> to a<to-1>, with only a key and a name: an empty
// list when to is from.
func resourceActionsOf(from, to int) []models.ResourceActionRead {
	items := make([]models.ResourceActionRead, 0, to-from)
	for i := from; i < to; i++ {
		key := "a" + strconv.Itoa(i)
		items = append(items, models.ResourceActionRead{Key: key, Name: key})
	}
	return items
}

// lowRisk is the attribute filter of the actions whose attribute risk is low.
func lowRisk() []AttributeFilter {
	return []AttributeFilter{AttributeEquals("risk", "low")}
}

// resourceActionCalls are calls of each method of the actions of a resource that acts on one
// action, with the ID of its operation, its route and a reply of the API to it.
var resourceActionCalls = []struct {
	name  string
	op    string
	route string
	reply mockpermit.Reply
	call  func(ctx context.Context, c *Client, resourceKey, actionKey string) error
}{
	{
		name: "Get", op: opGetResourceAction, route: getResourceActionRoute,
		reply: mockpermit.Reply{Body: resourceActionBody},
		call: func(ctx context.Context, c *Client, resourceKey, actionKey string) error {
			_, err := c.ResourceActions.Get(ctx, resourceKey, actionKey)
			return err
		},
	},
	{
		name: "Update", op: opUpdateResourceAction, route: updateResourceActionRoute,
		reply: mockpermit.Reply{Body: resourceActionBody},
		call: func(ctx context.Context, c *Client, resourceKey, actionKey string) error {
			_, err := c.ResourceActions.Update(ctx, resourceKey, actionKey,
				models.ResourceActionUpdate{Name: "n"})
			return err
		},
	},
	{
		name: "Delete", op: opDeleteResourceAction, route: deleteResourceActionRoute,
		reply: mockpermit.Reply{Status: http.StatusNoContent},
		call: func(ctx context.Context, c *Client, resourceKey, actionKey string) error {
			return c.ResourceActions.Delete(ctx, resourceKey, actionKey)
		},
	},
}

// TestResourceActionsAllReadsEveryPage checks that ResourceActionsService.All reads the 31
// actions of a list, in order, asking for each page in turn with the attribute filters of the
// options: at a per_page of 30, two pages of actions and the empty page after them; and at the
// default per_page of 100, the most that the API gives on a page, one page of 31 and the empty
// page after it.
func TestResourceActionsAllReadsEveryPage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    *ResourceActionsListOptions
		pages   [][]models.ResourceActionRead
		queries []string
	}{
		{
			name: "per_page 30",
			opts: &ResourceActionsListOptions{ListOptions: ListOptions{PerPage: 30}},
			pages: [][]models.ResourceActionRead{
				resourceActionsOf(0, 30), resourceActionsOf(30, 31), {},
			},
			queries: []string{
				"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30",
			},
		},
		{
			name: "the default per_page and a filter",
			opts: &ResourceActionsListOptions{Attributes: lowRisk()},
			pages: [][]models.ResourceActionRead{
				resourceActionsOf(0, 31), {},
			},
			queries: []string{
				"attr_risk=low&page=1&per_page=100", "attr_risk=low&page=2&per_page=100",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			var replies []mockpermit.Reply
			for _, items := range tc.pages {
				replies = append(replies, arrayOf(t, items))
			}
			api.Handle(listResourceActionsRoute, replies...)
			c := resourceClient(t, api)
			items, errs := collect(t, c.ResourceActions.All(t.Context(), "document", tc.opts))
			if diff := cmp.Diff(resourceActionsOf(0, 31), items); diff != "" || errs != nil {
				t.Errorf("All yielded the errors %v and actions that differ (-want +got):\n%s",
					errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestResourceActionsAllYieldsAnErrorOnce checks that a page that fails ends the iterator with
// its error, once, after the actions of the pages before it, and that the options of All are
// read when it is called: a later change to them, its filters too, changes no request.
func TestResourceActionsAllYieldsAnErrorOnce(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(listResourceActionsRoute, arrayOf(t, resourceActionsOf(0, 30)),
		mockpermit.Reply{Status: http.StatusForbidden, Body: body})
	opts := &ResourceActionsListOptions{
		ListOptions: ListOptions{PerPage: 30}, Attributes: lowRisk(),
	}
	seq := resourceClient(t, api).ResourceActions.All(t.Context(), "document", opts)
	opts.PerPage = 50
	opts.Attributes[0] = AttributeEquals("risk", "changed")
	opts.Attributes = append(opts.Attributes, AttributeEquals("added", true))
	items, errs := collect(t, seq)
	if len(items) != 30 || len(errs) != 1 || !errors.Is(errs[0], ErrForbidden) {
		t.Errorf("All yielded %d actions and the errors %v, want 30 and one error that "+
			"matches ErrForbidden", len(items), errs)
	}
	want := []string{"attr_risk=low&page=1&per_page=30", "attr_risk=low&page=2&per_page=30"}
	if diff := cmp.Diff(want, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestResourceActionsAllGetsTheScopeInEachLoop checks that All, without WithScope, looks up the
// scope in each loop over the iterator, before its first page, and not when it is called: a
// lookup that fails ends that loop with its error, and the next loop looks the scope up again.
func TestResourceActionsAllGetsTheScopeInEachLoop(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(scopeRoute, mockpermit.Reply{Status: http.StatusForbidden, Body: body},
		scopeReply(t, ""))
	api.Handle(listResourceActionsRoute, arrayOf(t, resourceActionsOf(0, 1)),
		arrayOf(t, resourceActionsOf(1, 1)))
	seq := projectClient(t, api).ResourceActions.All(t.Context(), "document", nil)
	if requests := api.Requests(); len(requests) != 0 {
		t.Fatalf("All sent %+v before a loop, want no request", requests)
	}
	if items, errs := collect(t, seq); items != nil || len(errs) != 1 ||
		!errors.Is(errs[0], ErrForbidden) {
		t.Errorf("the first loop yielded %v and the errors %v, want one error that matches "+
			"ErrForbidden", items, errs)
	}
	if items, errs := collect(t, seq); !cmp.Equal(resourceActionsOf(0, 1), items) ||
		errs != nil {
		t.Errorf("the second loop yielded %v and the errors %v, want a0", items, errs)
	}
	var paths []string
	for _, r := range api.Requests() {
		paths = append(paths, r.RawPath)
	}
	actions := "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID +
		"/resources/document/actions"
	want := []string{"/v2/api-key/scope", "/v2/api-key/scope", actions, actions}
	if diff := cmp.Diff(want, paths); diff != "" {
		t.Errorf("the requests differ (-want +got):\n%s", diff)
	}
}

// TestResourceActionsListReturnsAPage checks the page that List returns: the actions of the reply
// in its order, the page and the per_page that it asked for, page 1 and no query for nil options
// and for options without filters, and no counts, as the reply is a JSON array. A page without
// actions has an empty list, not nil.
func TestResourceActionsListReturnsAPage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  *ResourceActionsListOptions
		items []models.ResourceActionRead
		query string
		want  *Page[models.ResourceActionRead]
	}{
		{
			name: "nil options", items: resourceActionsOf(0, 2), query: "",
			want: &Page[models.ResourceActionRead]{Data: resourceActionsOf(0, 2), Page: 1},
		},
		{
			name:  "empty filters",
			opts:  &ResourceActionsListOptions{Attributes: []AttributeFilter{}},
			items: resourceActionsOf(0, 1), query: "",
			want: &Page[models.ResourceActionRead]{Data: resourceActionsOf(0, 1), Page: 1},
		},
		{
			name: "a page after the last",
			opts: &ResourceActionsListOptions{
				ListOptions: ListOptions{Page: 3, PerPage: 30}, Attributes: lowRisk(),
			},
			items: []models.ResourceActionRead{}, query: "attr_risk=low&page=3&per_page=30",
			want: &Page[models.ResourceActionRead]{
				Data: []models.ResourceActionRead{}, Page: 3, PerPage: 30,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listResourceActionsRoute, arrayOf(t, tc.items))
			c := resourceClient(t, api)
			got, err := c.ResourceActions.List(t.Context(), "document", tc.opts)
			if err != nil {
				t.Fatalf("List returned %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" || got.Data == nil {
				t.Errorf("List returned a page that differs (-want +got), or nil Data:\n%s", diff)
			}
			const path = "/v2/schema/p/e/resources/document/actions"
			if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != path ||
				requests[0].RawQuery != tc.query {
				t.Errorf("List sent %+v, want one request to %s?%s", requests, path, tc.query)
			}
		})
	}
}

// TestResourceActionAttributeFilters checks that List and All send each attribute filter in the
// query as attr_<name>=<value>, for a value of each type, with the name and the value escaped;
// and that a filter that is not valid, the zero AttributeFilter, one without a name, a float64
// that is NaN, +Inf or -Inf, and a second filter of one name, is an error that matches
// ErrInvalidConfig and names the rule and the filter by its place, from 1, from List and from
// All, and that neither sends a request, not even the lookup of the scope.
func TestResourceActionAttributeFilters(t *testing.T) {
	filters := []AttributeFilter{
		AttributeEquals("risk", "low & high=é"), AttributeEquals("audited", false),
		AttributeEquals("limit", fixtures.TwoPow53Plus1), AttributeEquals("score", 1e21),
		AttributeEquals("odd key/&", -0.25),
	}
	const query = "attr_audited=false&attr_limit=9007199254740993&" +
		"attr_odd+key%2F%26=-0.25&attr_risk=low+%26+high%3D%C3%A9&attr_score=1e%2B21"
	api := mockpermit.NewAPI(t)
	api.Handle(listResourceActionsRoute, mockpermit.Reply{Body: `[]`}, mockpermit.Reply{Body: `[]`})
	c := resourceClient(t, api)
	opts := &ResourceActionsListOptions{Attributes: filters}
	if _, err := c.ResourceActions.List(t.Context(), "document", opts); err != nil {
		t.Fatalf("List returned %v", err)
	}
	if _, errs := collect(t, c.ResourceActions.All(t.Context(), "document", opts)); errs != nil {
		t.Fatalf("All yielded %v", errs)
	}
	want := []string{query, query + "&page=1&per_page=100"}
	if diff := cmp.Diff(want, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
	nameEmpty := validate.ListAttributeFilterNameEmpty
	notFinite := validate.ListAttributeFilterNotFinite
	for _, tc := range []struct {
		name    string
		filters []AttributeFilter
		place   int
		rule    validate.Rule
	}{
		{"the zero filter", []AttributeFilter{{}}, 1, nameEmpty},
		{"no name", append(lowRisk(), AttributeEquals("", "x")), 2, nameEmpty},
		{"NaN", []AttributeFilter{AttributeEquals("score", math.NaN())}, 1, notFinite},
		{"+Inf", []AttributeFilter{AttributeEquals("score", math.Inf(1))}, 1, notFinite},
		{"-Inf", []AttributeFilter{AttributeEquals("score", math.Inf(-1))}, 1, notFinite},
		{
			"a repeated name",
			[]AttributeFilter{
				AttributeEquals("level", 1), lowRisk()[0], AttributeEquals("level", 2),
			},
			3, validate.ListAttributeFilterRepeated,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			c := projectClient(t, api)
			opts := &ResourceActionsListOptions{Attributes: tc.filters}
			_, err := c.ResourceActions.List(t.Context(), "document", opts)
			_, errs := collect(t, c.ResourceActions.All(t.Context(), "document", opts))
			want := (&configError{
				setting: opListResourceActions + ": attribute filter " + strconv.Itoa(tc.place),
				rule:    tc.rule,
			}).Error()
			for _, err := range append([]error{err}, errs...) {
				if !errors.Is(err, ErrInvalidConfig) || err.Error() != want {
					t.Errorf("the call returned %v, want %s", err, want)
				}
			}
			if len(errs) != 1 {
				t.Errorf("All yielded the errors %v, want one", errs)
			}
			api.AssertUntouched()
		})
	}
}

// TestInvalidResourceActionListOptions checks that a Page or a PerPage less than zero is an error
// that matches ErrInvalidConfig from List and All, and that neither sends a request.
func TestInvalidResourceActionListOptions(t *testing.T) {
	for _, opts := range []ListOptions{{Page: -1}, {PerPage: -1}} {
		api := mockpermit.NewAPI(t)
		c := resourceClient(t, api)
		filter := &ResourceActionsListOptions{ListOptions: opts, Attributes: lowRisk()}
		_, err := c.ResourceActions.List(t.Context(), "document", filter)
		_, errs := collect(t, c.ResourceActions.All(t.Context(), "document", filter))
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

// TestResourceActionsUseTheScope checks that the calls of the actions of a resource act on the
// environment of the client's scope. Without WithScope, the client looks up the scope of its API
// key once, and each call fills the route with the project and the environment of an
// environment-level key. A scope without an environment, that of a project-level or an
// organization-level key, is an error that matches ErrInvalidConfig and names the operation, and
// no call sends a request but the lookup, which the API fails for any route that the test did
// not register.
func TestResourceActionsUseTheScope(t *testing.T) {
	const prefix = "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID +
		"/resources/document/actions"
	calls := map[string]func(ctx context.Context, c *Client) error{
		opListResourceActions: func(ctx context.Context, c *Client) error {
			_, err := c.ResourceActions.List(ctx, "document", nil)
			return err
		},
		opCreateResourceAction: func(ctx context.Context, c *Client) error {
			_, err := c.ResourceActions.Create(ctx, "document",
				models.ResourceActionCreate{Key: "k", Name: "n"})
			return err
		},
	}
	for _, m := range resourceActionCalls {
		calls[m.op] = func(ctx context.Context, c *Client) error {
			return m.call(ctx, c, "document", "read")
		}
	}
	t.Run("an environment-level key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		api.Handle(listResourceActionsRoute, mockpermit.Reply{Body: `[]`})
		api.Handle(createResourceActionRoute, mockpermit.Reply{Body: resourceActionBody})
		for _, m := range resourceActionCalls {
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
			if r.RawPath != prefix && r.RawPath != prefix+"/read" {
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
			seq := c.ResourceActions.All(t.Context(), "document", nil)
			if _, errs := collect(t, seq); len(errs) != 1 || !errors.Is(errs[0], ErrInvalidConfig) {
				t.Errorf("All yielded %v, want one error that matches ErrInvalidConfig", errs)
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("the calls sent %d requests, want only the lookup of the scope", n)
			}
		})
	}
}

// TestResourceActionKeysInThePath checks that each method of the actions of a resource sends the
// key of the resource, and the key of the action when it names one, each as one segment of the
// path, escaped, after the project and the environment of the scope, whatever the key holds: a
// slash, a percent sign, a space, a question mark and a hash, a letter outside ASCII, the shape
// of a UUID and a leading dot. A key that is empty, . or .. would change the route, so it is an
// error that matches ErrInvalidConfig and names the placeholder, and no request.
func TestResourceActionKeysInThePath(t *testing.T) {
	const prefix = "/v2/schema/p/e/resources/"
	listCalls := []struct {
		name  string
		route string
		reply mockpermit.Reply
		call  func(ctx context.Context, c *Client, resourceKey string) error
	}{
		{
			name: "List", route: listResourceActionsRoute, reply: mockpermit.Reply{Body: `[]`},
			call: func(ctx context.Context, c *Client, resourceKey string) error {
				_, err := c.ResourceActions.List(ctx, resourceKey, nil)
				return err
			},
		},
		{
			name: "All", route: listResourceActionsRoute, reply: mockpermit.Reply{Body: `[]`},
			call: func(ctx context.Context, c *Client, resourceKey string) error {
				_, errs := collect(t, c.ResourceActions.All(ctx, resourceKey, nil))
				return errors.Join(errs...)
			},
		},
		{
			name: "Create", route: createResourceActionRoute,
			reply: mockpermit.Reply{Body: resourceActionBody},
			call: func(ctx context.Context, c *Client, resourceKey string) error {
				_, err := c.ResourceActions.Create(ctx, resourceKey,
					models.ResourceActionCreate{Key: "k", Name: "n"})
				return err
			},
		},
	}
	for key, escaped := range map[string]string{
		"a/b":            "a%2Fb",
		"50%":            "50%25",
		"a b?c#d":        "a%20b%3Fc%23d",
		"é":              "%C3%A9",
		fixtures.UUIDKey: fixtures.UUIDKey,
		".hidden":        ".hidden",
		"doc.v2..x":      "doc.v2..x",
	} {
		for _, m := range listCalls {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, m.reply)
			if err := m.call(t.Context(), resourceClient(t, api), key); err != nil {
				t.Errorf("%s(%q) returned %v", m.name, key, err)
			}
			want := prefix + escaped + "/actions"
			if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want {
				t.Errorf("%s(%q) sent %+v, want one request to %s", m.name, key, requests, want)
			}
		}
		for _, m := range resourceActionCalls {
			for _, keys := range [][2]string{{key, "read"}, {"document", key}} {
				api := mockpermit.NewAPI(t)
				api.Handle(m.route, m.reply)
				c := resourceClient(t, api)
				if err := m.call(t.Context(), c, keys[0], keys[1]); err != nil {
					t.Errorf("%s(%q, %q) returned %v", m.name, keys[0], keys[1], err)
				}
				want := prefix + "document/actions/" + escaped
				if keys[1] == "read" {
					want = prefix + escaped + "/actions/read"
				}
				if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want {
					t.Errorf("%s(%q, %q) sent %+v, want one request to %s", m.name, keys[0],
						keys[1], requests, want)
				}
			}
		}
	}
	for _, key := range []string{"", ".", ".."} {
		for _, m := range listCalls {
			api := mockpermit.NewAPI(t)
			err := m.call(t.Context(), resourceClient(t, api), key)
			named := err != nil && strings.Contains(err.Error(), "{resource_id}")
			if !errors.Is(err, ErrInvalidConfig) || !named {
				t.Errorf("%s(%q) returned %v, want an error that matches ErrInvalidConfig and "+
					"names {resource_id}", m.name, key, err)
			}
			api.AssertUntouched()
		}
		for _, m := range resourceActionCalls {
			for placeholder, keys := range map[string][2]string{
				"{resource_id}": {key, "read"}, "{action_id}": {"document", key},
			} {
				api := mockpermit.NewAPI(t)
				err := m.call(t.Context(), resourceClient(t, api), keys[0], keys[1])
				named := err != nil && strings.Contains(err.Error(), placeholder)
				if !errors.Is(err, ErrInvalidConfig) || !named {
					t.Errorf("%s(%q, %q) returned %v, want an error that matches "+
						"ErrInvalidConfig and names %s", m.name, keys[0], keys[1], err,
						placeholder)
				}
				api.AssertUntouched()
			}
		}
	}
}

// TestDeleteResourceActionThatIsGone checks that Delete returns nil for a 404, with or without a
// body, as the action that the path names is gone already, after one request; and an *Error with
// the status for any other error status, such as 403, 409 or 422.
func TestDeleteResourceActionThatIsGone(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"NOT_FOUND","title":"Not Found","message":"the action does not exist"}`,
		"",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteResourceActionRoute,
			mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		err := resourceClient(t, api).ResourceActions.Delete(t.Context(), "document", "gone")
		if err != nil {
			t.Errorf("Delete of an action that the API answers with 404 and the body %q "+
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
		api.Handle(deleteResourceActionRoute,
			mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := resourceClient(t, api).ResourceActions.Delete(t.Context(), "document", "read")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteResourceAction ||
			e.Detail != "no" || errors.Is(err, ErrNotFound) {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status and the detail", status, err, opDeleteResourceAction)
		}
	}
}

// TestNotFoundOfOtherResourceActionCalls checks that a 404 to List, Create, Get and Update, which
// delete nothing, is an *Error that matches ErrNotFound.
func TestNotFoundOfOtherResourceActionCalls(t *testing.T) {
	const body = `{"error_code":"NOT_FOUND","title":"Not Found","message":"no such object"}`
	notFound := mockpermit.Reply{Status: http.StatusNotFound, Body: body}
	calls := []struct {
		op    string
		route string
		call  func(ctx context.Context, c *Client) error
	}{
		{
			op: opListResourceActions, route: listResourceActionsRoute,
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ResourceActions.List(ctx, "gone", nil)
				return err
			},
		},
		{
			op: opCreateResourceAction, route: createResourceActionRoute,
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ResourceActions.Create(ctx, "gone",
					models.ResourceActionCreate{Key: "k", Name: "n"})
				return err
			},
		},
	}
	for _, m := range resourceActionCalls[:2] {
		calls = append(calls, struct {
			op    string
			route string
			call  func(ctx context.Context, c *Client) error
		}{
			op: m.op, route: m.route,
			call: func(ctx context.Context, c *Client) error {
				return m.call(ctx, c, "document", "gone")
			},
		})
	}
	for _, m := range calls {
		api := mockpermit.NewAPI(t)
		api.Handle(m.route, notFound)
		err := m.call(t.Context(), resourceClient(t, api))
		e := mustError(t, err)
		if !errors.Is(err, ErrNotFound) || e.StatusCode != http.StatusNotFound || e.Op != m.op {
			t.Errorf("%s of an object that the API answers with 404 returned %v, want an "+
				"*Error that matches ErrNotFound", m.op, err)
		}
	}
}

// TestResourceActionBodies checks the bodies of Create and Update byte for byte against the
// golden files in testdata/golden: a create with the required fields only, one with empty
// attributes, which sends {}, and one with every field, with values that an encoder may change:
// 2^53+1, a line separator and <, > and &; the zero update, which sends {}; an update that sends
// null for the description and the attributes; one that sends an empty description and empty
// attributes; and one that sets every field.
func TestResourceActionBodies(t *testing.T) {
	attributes := models.Attributes{"n": fixtures.TwoPow53Plus1, "s": "<a>&b"}
	description := "a" + fixtures.LineSeparator + "b"
	create := func(action models.ResourceActionCreate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.ResourceActions.Create(t.Context(), "r", action)
			return err
		}
	}
	update := func(change models.ResourceActionUpdate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.ResourceActions.Update(t.Context(), "r", "a", change)
			return err
		}
	}
	for _, tc := range []struct {
		golden string
		route  string
		call   func(c *Client) error
	}{
		{
			"create_resource_action.required.golden", createResourceActionRoute,
			create(models.ResourceActionCreate{Key: "r", Name: "R"}),
		},
		{
			"create_resource_action.empty-attributes.golden", createResourceActionRoute,
			create(models.ResourceActionCreate{
				Key: "r", Name: "R", Attributes: models.Attributes{},
			}),
		},
		{
			"create_resource_action.every-field.golden", createResourceActionRoute,
			create(models.ResourceActionCreate{
				Key: "r", Name: "<R> & S", Description: description, Attributes: attributes,
			}),
		},
		{
			"update_resource_action.unset.golden", updateResourceActionRoute,
			update(models.ResourceActionUpdate{}),
		},
		{
			"update_resource_action.null.golden", updateResourceActionRoute,
			update(models.ResourceActionUpdate{
				Description: models.Null[string](), Attributes: models.Null[models.Attributes](),
			}),
		},
		{
			"update_resource_action.empty-values.golden", updateResourceActionRoute,
			update(models.ResourceActionUpdate{
				Name: "R", Description: models.Some(""),
				Attributes: models.Some(models.Attributes{}),
			}),
		},
		{
			"update_resource_action.values.golden", updateResourceActionRoute,
			update(models.ResourceActionUpdate{
				Name: "<R> & S", Description: models.Some(description),
				Attributes: models.Some(attributes),
			}),
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: resourceActionBody})
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

// TestResourceActionBodiesThatCannotBeJSON checks that a create and an update whose attributes
// hold a value that JSON cannot hold, and an update whose attributes are a nil map of Some,
// which would send null for a value, are errors that match ErrInvalidConfig and name the
// operation, and send no request.
func TestResourceActionBodiesThatCannotBeJSON(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := resourceClient(t, api)
	_, createErr := c.ResourceActions.Create(t.Context(), "r", models.ResourceActionCreate{
		Key: "a", Name: "A", Attributes: models.Attributes{"c": make(chan int)},
	})
	_, updateErr := c.ResourceActions.Update(t.Context(), "r", "a", models.ResourceActionUpdate{
		Attributes: models.Some(models.Attributes{"n": math.NaN()}),
	})
	_, nilErr := c.ResourceActions.Update(t.Context(), "r", "a", models.ResourceActionUpdate{
		Attributes: models.Some[models.Attributes](nil),
	})
	for _, tc := range []struct {
		op  string
		err error
	}{
		{opCreateResourceAction, createErr},
		{opUpdateResourceAction, updateErr},
		{opUpdateResourceAction, nilErr},
	} {
		if !errors.Is(tc.err, ErrInvalidConfig) || !strings.Contains(tc.err.Error(), tc.op) {
			t.Errorf("%s returned %v, want an error that matches ErrInvalidConfig and names it",
				tc.op, tc.err)
		}
	}
	api.AssertUntouched()
}

// TestResourceActionRetries checks when a call of the actions of a resource sends its request
// again: Create, which the API may have acted on, after a 429 only, so a 503 ends it after one
// request; Update, Get, Delete and List, which do the same when sent twice, after a 503 too.
func TestResourceActionRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	ok := mockpermit.Reply{Body: resourceActionBody}
	createAction := func(c *Client) error {
		_, err := c.ResourceActions.Create(t.Context(), "document",
			models.ResourceActionCreate{Key: "k", Name: "n"})
		return err
	}
	type retryCase struct {
		name    string
		route   string
		replies []mockpermit.Reply
		call    func(c *Client) error
		status  int
	}
	tests := []retryCase{
		{
			name: "Create after a 429", route: createResourceActionRoute,
			replies: []mockpermit.Reply{limited, ok}, call: createAction,
		},
		{
			name: "Create after a 503", route: createResourceActionRoute,
			replies: []mockpermit.Reply{unavailable}, call: createAction,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "List after a 503", route: listResourceActionsRoute,
			replies: []mockpermit.Reply{unavailable, {Body: `[]`}},
			call: func(c *Client) error {
				_, err := c.ResourceActions.List(t.Context(), "document", nil)
				return err
			},
		},
	}
	for _, m := range resourceActionCalls {
		tests = append(tests, retryCase{
			name: m.name + " after a 503", route: m.route,
			replies: []mockpermit.Reply{unavailable, m.reply},
			call:    func(c *Client) error { return m.call(t.Context(), c, "document", "read") },
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

// TestMalformedResourceActionReplies checks that a reply in 200-299 that is not an action, or
// for List and All not a JSON array of actions, such as the paginated object of them, which the
// API's documentation of the list does not describe, is an *Error that names the model, and no
// action, no page and no action of the iterator.
func TestMalformedResourceActionReplies(t *testing.T) {
	const prefix = "permit: "
	for _, body := range []string{``, `null`, `[]`, `"a"`, `{"key":1}`, `{} {}`} {
		api := mockpermit.NewAPI(t)
		api.Handle(getResourceActionRoute, mockpermit.Reply{Body: body})
		c := resourceClient(t, api)
		action, err := c.ResourceActions.Get(t.Context(), "document", "read")
		want := prefix + "get_resource_action: GET /v2/schema/{proj_id}/{env_id}/resources/" +
			"{resource_id}/actions/{action_id}: status 200: the reply is not a valid " +
			"models.ResourceActionRead"
		if e := mustError(t, err); action != nil || e.Error() != want {
			t.Errorf("Get of the reply %q returned %+v and\n%v\nwant no action and\n%s", body,
				action, err, want)
		}
	}
	for _, body := range []string{
		``, `null`, `"a"`, `[1]`, `[{"key":1}]`, `{"data":[]}`, `{"data":[{"key":"read"}]}`,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(listResourceActionsRoute, mockpermit.Reply{Body: body},
			mockpermit.Reply{Body: body})
		c := resourceClient(t, api)
		page, err := c.ResourceActions.List(t.Context(), "document", nil)
		want := prefix + "list_resource_actions: GET /v2/schema/{proj_id}/{env_id}/resources/" +
			"{resource_id}/actions: status 200: the reply is not a valid list of " +
			"models.ResourceActionRead"
		if e := mustError(t, err); page != nil || e.Error() != want {
			t.Errorf("List of the reply %q returned %+v and\n%v\nwant no page and\n%s", body,
				page, err, want)
		}
		items, errs := collect(t, c.ResourceActions.All(t.Context(), "document", nil))
		if items != nil || len(errs) != 1 || mustError(t, errs[0]).Error() != want {
			t.Errorf("All of the reply %q yielded %+v and %v, want only the error\n%s", body,
				items, errs, want)
		}
	}
}

// TestResourceActionsAfterClose checks that each method returns ErrClosed after Close, and sends
// nothing, also without WithScope, whose lookup it does not send either, and also for List and
// All with an attribute filter that is not valid, which they reject only on an open client.
func TestResourceActionsAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	invalid := &ResourceActionsListOptions{Attributes: []AttributeFilter{{}}}
	_, listErr := c.ResourceActions.List(ctx, "document", nil)
	_, invalidErr := c.ResourceActions.List(ctx, "document", invalid)
	_, allErrs := collect(t, c.ResourceActions.All(ctx, "document", nil))
	_, invalidAllErrs := collect(t, c.ResourceActions.All(ctx, "document", invalid))
	_, createErr := c.ResourceActions.Create(ctx, "document",
		models.ResourceActionCreate{Key: "k", Name: "n"})
	errs := []error{listErr, invalidErr, createErr}
	for _, m := range resourceActionCalls {
		errs = append(errs, m.call(t.Context(), c, "document", "read"))
	}
	for i, err := range append(append(errs, allErrs...), invalidAllErrs...) {
		if !errors.Is(err, ErrClosed) {
			t.Errorf("call %d after Close returned %v, want ErrClosed", i, err)
		}
	}
	if len(allErrs) != 1 || len(invalidAllErrs) != 1 {
		t.Errorf("All after Close yielded %v and %v, want ErrClosed once each", allErrs,
			invalidAllErrs)
	}
	api.AssertUntouched()
}
