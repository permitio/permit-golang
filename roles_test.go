package permit

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/models"
)

// The routes of the roles of an environment, as mockpermit.Server.Handle takes them.
const (
	listRolesRoute             = "GET /v2/schema/{proj_id}/{env_id}/roles"
	createRoleRoute            = "POST /v2/schema/{proj_id}/{env_id}/roles"
	getRoleRoute               = "GET /v2/schema/{proj_id}/{env_id}/roles/{role_id}"
	updateRoleRoute            = "PATCH /v2/schema/{proj_id}/{env_id}/roles/{role_id}"
	deleteRoleRoute            = "DELETE /v2/schema/{proj_id}/{env_id}/roles/{role_id}"
	assignRolePermissionsRoute = "POST /v2/schema/{proj_id}/{env_id}/roles/{role_id}/permissions"
	removeRolePermissionsRoute = "DELETE /v2/schema/{proj_id}/{env_id}/roles/{role_id}/" +
		"permissions"
	roleAncestorsRoute    = "GET /v2/schema/{proj_id}/{env_id}/roles/{role_id}/ancestors"
	roleDescendantsRoute  = "GET /v2/schema/{proj_id}/{env_id}/roles/{role_id}/descendants"
	bulkReplaceRolesRoute = "PUT /v2/schema/{proj_id}/{env_id}/bulk/roles"
)

// roleBody is the body of a reply of the API that holds the role editor.
const roleBody = `{"name":"Editor","key":"editor","id":"00000000-0000-4000-8000-000000000102",` +
	`"organization_id":"00000000-0000-4000-8000-000000000001",` +
	`"project_id":"00000000-0000-4000-8000-000000000002",` +
	`"environment_id":"00000000-0000-4000-8000-000000000003",` +
	`"created_at":"2026-10-14T09:30:00Z","updated_at":"2026-10-14T09:30:00Z"}`

// relativesBody is the body of a reply of the API to the ancestors or the descendants of a role,
// which holds the role editor in a JSON array.
const relativesBody = `{"roles":[` + roleBody + `]}`

// bulkRolesBody is the body of a reply of the API to a bulk replace of the roles.
const bulkRolesBody = `{"created":["auditor"],"updated":["viewer"]}`

// rolesOf returns the roles r<from> to r<to-1>, with only a key and a name: an empty list when to
// is from.
func rolesOf(from, to int) []models.RoleRead {
	items := make([]models.RoleRead, 0, to-from)
	for i := from; i < to; i++ {
		key := "r" + strconv.Itoa(i)
		items = append(items, models.RoleRead{Key: key, Name: key})
	}
	return items
}

// goldTier is the attribute filter of the roles whose attribute tier is gold.
func goldTier() []AttributeFilter {
	return []AttributeFilter{AttributeEquals("tier", "gold")}
}

// rolesReply returns a reply that holds items: a JSON array without counts, and the paginated
// object with the members of counts, such as `,"total_count":2`, and the page_count that the API
// sends, the number of items, with them.
func rolesReply(t *testing.T, items []models.RoleRead, counts string) mockpermit.Reply {
	t.Helper()
	if counts == "" {
		return arrayOf(t, items)
	}
	return pageOf(t, items, counts+`,"page_count":`+strconv.Itoa(len(items)))
}

// roleCall is a call of a method of the roles, with the ID of its operation, its route and a
// reply of the API to it.
type roleCall struct {
	name  string
	op    string
	route string
	reply mockpermit.Reply
	call  func(ctx context.Context, c *Client, roleKey string) error
}

// keyedRoleCalls are calls of each method of the roles that acts on the role whose key it takes.
var keyedRoleCalls = []roleCall{
	{
		name: "Get", op: opGetRole, route: getRoleRoute, reply: mockpermit.Reply{Body: roleBody},
		call: func(ctx context.Context, c *Client, roleKey string) error {
			_, err := c.Roles.Get(ctx, roleKey)
			return err
		},
	},
	{
		name: "Update", op: opUpdateRole, route: updateRoleRoute,
		reply: mockpermit.Reply{Body: roleBody},
		call: func(ctx context.Context, c *Client, roleKey string) error {
			_, err := c.Roles.Update(ctx, roleKey, models.RoleUpdate{Name: "n"})
			return err
		},
	},
	{
		name: "AssignPermissions", op: opAssignRolePermissions, route: assignRolePermissionsRoute,
		reply: mockpermit.Reply{Body: roleBody},
		call: func(ctx context.Context, c *Client, roleKey string) error {
			_, err := c.Roles.AssignPermissions(ctx, roleKey, []string{"document:read"})
			return err
		},
	},
	{
		name: "RemovePermissions", op: opRemoveRolePermissions, route: removeRolePermissionsRoute,
		reply: mockpermit.Reply{Body: roleBody},
		call: func(ctx context.Context, c *Client, roleKey string) error {
			_, err := c.Roles.RemovePermissions(ctx, roleKey, []string{"document:read"})
			return err
		},
	},
	{
		name: "Ancestors", op: opRoleAncestors, route: roleAncestorsRoute,
		reply: mockpermit.Reply{Body: relativesBody},
		call: func(ctx context.Context, c *Client, roleKey string) error {
			_, err := c.Roles.Ancestors(ctx, roleKey)
			return err
		},
	},
	{
		name: "Descendants", op: opRoleDescendants, route: roleDescendantsRoute,
		reply: mockpermit.Reply{Body: relativesBody},
		call: func(ctx context.Context, c *Client, roleKey string) error {
			_, err := c.Roles.Descendants(ctx, roleKey)
			return err
		},
	},
	{
		name: "Delete", op: opDeleteRole, route: deleteRoleRoute,
		reply: mockpermit.Reply{Status: http.StatusNoContent},
		call: func(ctx context.Context, c *Client, roleKey string) error {
			return c.Roles.Delete(ctx, roleKey)
		},
	},
}

// unkeyedRoleCalls are calls of each method of the roles that takes no key of a role.
var unkeyedRoleCalls = []roleCall{
	{
		name: "List", op: opListRoles, route: listRolesRoute, reply: mockpermit.Reply{Body: `[]`},
		call: func(ctx context.Context, c *Client, _ string) error {
			_, err := c.Roles.List(ctx, nil)
			return err
		},
	},
	{
		name: "Create", op: opCreateRole, route: createRoleRoute,
		reply: mockpermit.Reply{Body: roleBody},
		call: func(ctx context.Context, c *Client, _ string) error {
			_, err := c.Roles.Create(ctx, models.RoleCreate{Key: "k", Name: "K"})
			return err
		},
	},
	{
		name: "BulkReplace", op: opBulkReplaceRoles, route: bulkReplaceRolesRoute,
		reply: mockpermit.Reply{Body: bulkRolesBody},
		call: func(ctx context.Context, c *Client, _ string) error {
			_, err := c.Roles.BulkReplace(ctx, []models.RoleCreateBulk{{Key: "k", Name: "K"}})
			return err
		},
	},
}

// TestRoleAttributesOfAnyType checks that each method that returns a role reads attributes whose
// values are of each JSON type: a string, a fraction, an integer beyond 2^53, which it keeps as
// a json.Number with every digit, a boolean, null, an array and an object. v1 typed the
// attributes of a role as strings, and failed on a reply with any other value.
func TestRoleAttributesOfAnyType(t *testing.T) {
	attributes := `{"tier":"gold","ratio":0.5,"limit":9007199254740993,"on":true,"gone":null,` +
		`"tags":["a",1],"quota":{"max":3,"regions":["eu"]}}`
	body := `{"name":"Editor","key":"editor","attributes":` + attributes + `}`
	want := models.Attributes{
		"tier": "gold", "ratio": json.Number("0.5"), "limit": json.Number("9007199254740993"),
		"on": true, "gone": nil, "tags": []any{"a", json.Number("1")},
		"quota": map[string]any{"max": json.Number("3"), "regions": []any{"eu"}},
	}
	one := func(r *models.RoleRead, err error) ([]models.RoleRead, error) {
		if err != nil {
			return nil, err
		}
		return []models.RoleRead{*r}, nil
	}
	permissions := []string{"document:read"}
	for _, tc := range []struct {
		name  string
		route string
		reply string
		read  func(ctx context.Context, c *Client) ([]models.RoleRead, error)
	}{
		{
			"List", listRolesRoute, `[` + body + `]`,
			func(ctx context.Context, c *Client) ([]models.RoleRead, error) {
				page, err := c.Roles.List(ctx, nil)
				if err != nil {
					return nil, err
				}
				return page.Data, nil
			},
		},
		{
			"All", listRolesRoute, `{"data":[` + body + `],"total_count":1,"page_count":1}`,
			func(ctx context.Context, c *Client) ([]models.RoleRead, error) {
				items, errs := collect(t, c.Roles.All(ctx, nil))
				return items, errors.Join(errs...)
			},
		},
		{
			"Get", getRoleRoute, body,
			func(ctx context.Context, c *Client) ([]models.RoleRead, error) {
				return one(c.Roles.Get(ctx, "editor"))
			},
		},
		{
			"Create", createRoleRoute, body,
			func(ctx context.Context, c *Client) ([]models.RoleRead, error) {
				return one(c.Roles.Create(ctx, models.RoleCreate{Key: "editor", Name: "Editor"}))
			},
		},
		{
			"Update", updateRoleRoute, body,
			func(ctx context.Context, c *Client) ([]models.RoleRead, error) {
				return one(c.Roles.Update(ctx, "editor", models.RoleUpdate{}))
			},
		},
		{
			"AssignPermissions", assignRolePermissionsRoute, body,
			func(ctx context.Context, c *Client) ([]models.RoleRead, error) {
				return one(c.Roles.AssignPermissions(ctx, "editor", permissions))
			},
		},
		{
			"RemovePermissions", removeRolePermissionsRoute, body,
			func(ctx context.Context, c *Client) ([]models.RoleRead, error) {
				return one(c.Roles.RemovePermissions(ctx, "editor", permissions))
			},
		},
		{
			"Ancestors", roleAncestorsRoute, `{"roles":[` + body + `]}`,
			func(ctx context.Context, c *Client) ([]models.RoleRead, error) {
				return c.Roles.Ancestors(ctx, "editor")
			},
		},
		{
			"Descendants", roleDescendantsRoute,
			`{"roles":{"data":[` + body + `],"total_count":1,"page_count":1}}`,
			func(ctx context.Context, c *Client) ([]models.RoleRead, error) {
				return c.Roles.Descendants(ctx, "editor")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: tc.reply})
			roles, err := tc.read(t.Context(), resourceClient(t, api))
			if err != nil || len(roles) != 1 {
				t.Fatalf("%s returned the roles %+v and %v, want one role", tc.name, roles, err)
			}
			if diff := cmp.Diff(want, roles[0].Attributes); diff != "" {
				t.Errorf("%s read attributes that differ (-want +got):\n%s", tc.name, diff)
			}
		})
	}
}

// TestRolesAllReadsEveryPage checks that RolesService.All reads the 31 roles of a list, in
// order, asking for each page in turn with the attribute filters of the options: at a per_page of
// 30, two pages of roles and the empty page after them; at the default per_page of 100, the most
// that the API gives on a page, one page of 31 and the empty page after it; and up to the page
// whose roles reach the total_count of its paginated reply, without the empty page after it.
func TestRolesAllReadsEveryPage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    *RolesListOptions
		pages   [][]models.RoleRead
		counts  string
		queries []string
	}{
		{
			name: "per_page 30", opts: &RolesListOptions{ListOptions: ListOptions{PerPage: 30}},
			pages: [][]models.RoleRead{rolesOf(0, 30), rolesOf(30, 31), {}},
			queries: []string{
				"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30",
			},
		},
		{
			name:  "the default per_page and a filter",
			opts:  &RolesListOptions{Attributes: goldTier()},
			pages: [][]models.RoleRead{rolesOf(0, 31), {}},
			queries: []string{
				"attr_tier=gold&page=1&per_page=100", "attr_tier=gold&page=2&per_page=100",
			},
		},
		{
			name:    "paginated replies",
			opts:    &RolesListOptions{ListOptions: ListOptions{PerPage: 30}},
			pages:   [][]models.RoleRead{rolesOf(0, 30), rolesOf(30, 31)},
			counts:  `,"total_count":31`,
			queries: []string{"page=1&per_page=30", "page=2&per_page=30"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			var replies []mockpermit.Reply
			for _, items := range tc.pages {
				replies = append(replies, rolesReply(t, items, tc.counts))
			}
			api.Handle(listRolesRoute, replies...)
			items, errs := collect(t, resourceClient(t, api).Roles.All(t.Context(), tc.opts))
			if diff := cmp.Diff(rolesOf(0, 31), items); diff != "" || errs != nil {
				t.Errorf("All yielded the errors %v and roles that differ (-want +got):\n%s",
					errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestRolesAllYieldsAnErrorOnce checks that a page that fails ends the iterator with its error,
// once, after the roles of the pages before it, and that the options of All are read when it is
// called: a later change to them, or to their filters, changes no request.
func TestRolesAllYieldsAnErrorOnce(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(listRolesRoute, arrayOf(t, rolesOf(0, 30)),
		mockpermit.Reply{Status: http.StatusForbidden, Body: body})
	opts := &RolesListOptions{ListOptions: ListOptions{PerPage: 30}, Attributes: goldTier()}
	seq := resourceClient(t, api).Roles.All(t.Context(), opts)
	opts.PerPage, opts.Attributes[0] = 50, AttributeEquals("tier", "silver")
	items, errs := collect(t, seq)
	if len(items) != 30 || len(errs) != 1 || !errors.Is(errs[0], ErrForbidden) {
		t.Errorf("All yielded %d roles and the errors %v, want 30 and one error that matches "+
			"ErrForbidden", len(items), errs)
	}
	want := []string{"attr_tier=gold&page=1&per_page=30", "attr_tier=gold&page=2&per_page=30"}
	if diff := cmp.Diff(want, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestRolesAllGetsTheScopeInEachLoop checks that All, without WithScope, looks up the scope in
// each loop over the iterator, before its first page, and not when it is called: a lookup that
// fails ends that loop with its error, and the next loop looks the scope up again.
func TestRolesAllGetsTheScopeInEachLoop(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(scopeRoute, mockpermit.Reply{Status: http.StatusForbidden, Body: body},
		scopeReply(t, ""))
	api.Handle(listRolesRoute, arrayOf(t, rolesOf(0, 1)), arrayOf(t, rolesOf(1, 1)))
	seq := projectClient(t, api).Roles.All(t.Context(), nil)
	if requests := api.Requests(); len(requests) != 0 {
		t.Fatalf("All sent %+v before a loop, want no request", requests)
	}
	if items, errs := collect(t, seq); items != nil || len(errs) != 1 ||
		!errors.Is(errs[0], ErrForbidden) {
		t.Errorf("the first loop yielded %v and the errors %v, want one error that matches "+
			"ErrForbidden", items, errs)
	}
	if items, errs := collect(t, seq); !cmp.Equal(rolesOf(0, 1), items) || errs != nil {
		t.Errorf("the second loop yielded %v and the errors %v, want r0", items, errs)
	}
	var paths []string
	for _, r := range api.Requests() {
		paths = append(paths, r.RawPath)
	}
	roles := "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID + "/roles"
	want := []string{"/v2/api-key/scope", "/v2/api-key/scope", roles, roles}
	if diff := cmp.Diff(want, paths); diff != "" {
		t.Errorf("the requests differ (-want +got):\n%s", diff)
	}
}

// TestRolesListReturnsAPage checks the page that List returns: the roles of the reply in its
// order, the page and the per_page that it asked for, page 1 and no query for nil options and for
// the zero options, and the counts of a paginated reply. A page without roles has an empty list,
// not nil.
func TestRolesListReturnsAPage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		opts   *RolesListOptions
		items  []models.RoleRead
		counts string
		query  string
		want   *Page[models.RoleRead]
	}{
		{
			name: "nil options", items: rolesOf(0, 2),
			want: &Page[models.RoleRead]{Data: rolesOf(0, 2), Page: 1},
		},
		{
			name: "the zero options", opts: &RolesListOptions{}, items: rolesOf(0, 1),
			want: &Page[models.RoleRead]{Data: rolesOf(0, 1), Page: 1},
		},
		{
			name:  "a paginated reply after the last page",
			opts:  &RolesListOptions{ListOptions: ListOptions{Page: 3, PerPage: 30}},
			items: []models.RoleRead{}, counts: `,"total_count":31`,
			query: "page=3&per_page=30",
			want: &Page[models.RoleRead]{
				Data: []models.RoleRead{}, Page: 3, PerPage: 30, TotalCount: new(31),
				PageCount: new(0),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listRolesRoute, rolesReply(t, tc.items, tc.counts))
			got, err := resourceClient(t, api).Roles.List(t.Context(), tc.opts)
			if err != nil {
				t.Fatalf("List returned %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" || got.Data == nil {
				t.Errorf("List returned a page that differs (-want +got), or nil Data:\n%s", diff)
			}
			if requests := api.Requests(); len(requests) != 1 ||
				requests[0].RawPath != "/v2/schema/p/e/roles" || requests[0].RawQuery != tc.query {
				t.Errorf("List sent %+v, want one request to /v2/schema/p/e/roles?%s", requests,
					tc.query)
			}
		})
	}
}

// TestRoleAttributeFilters checks that List and All send each attribute filter in the query as
// attr_<name>=<value>, for a value of each type, with the name and the value escaped; and that a
// filter that is not valid, the zero AttributeFilter, one without a name, a float64 that is NaN,
// +Inf or -Inf, and a second filter of one name, is an error that matches ErrInvalidConfig and
// names the rule and the filter by its place, from 1, from List and from All, and that neither
// sends a request, not even the lookup of the scope.
func TestRoleAttributeFilters(t *testing.T) {
	filters := []AttributeFilter{
		AttributeEquals("tier", "gold & silver=é"), AttributeEquals("billable", false),
		AttributeEquals("limit", fixtures.TwoPow53Plus1), AttributeEquals("score", 1e21),
		AttributeEquals("odd key/&", int64(-7)), AttributeEquals("ratio", -0.25),
	}
	const query = "attr_billable=false&attr_limit=9007199254740993&attr_odd+key%2F%26=-7&" +
		"attr_ratio=-0.25&attr_score=1e%2B21&attr_tier=gold+%26+silver%3D%C3%A9"
	api := mockpermit.NewAPI(t)
	api.Handle(listRolesRoute, mockpermit.Reply{Body: `[]`}, mockpermit.Reply{Body: `[]`})
	c := resourceClient(t, api)
	opts := &RolesListOptions{Attributes: filters}
	if _, err := c.Roles.List(t.Context(), opts); err != nil {
		t.Fatalf("List returned %v", err)
	}
	if _, errs := collect(t, c.Roles.All(t.Context(), opts)); errs != nil {
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
		{"no name", append(goldTier(), AttributeEquals("", "x")), 2, nameEmpty},
		{"NaN", []AttributeFilter{AttributeEquals("score", math.NaN())}, 1, notFinite},
		{"+Inf", []AttributeFilter{AttributeEquals("score", math.Inf(1))}, 1, notFinite},
		{"-Inf", []AttributeFilter{AttributeEquals("score", math.Inf(-1))}, 1, notFinite},
		{
			"a repeated name",
			[]AttributeFilter{
				AttributeEquals("level", 1), goldTier()[0], AttributeEquals("level", 2),
			},
			3, validate.ListAttributeFilterRepeated,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			c := projectClient(t, api)
			opts := &RolesListOptions{Attributes: tc.filters}
			_, err := c.Roles.List(t.Context(), opts)
			_, errs := collect(t, c.Roles.All(t.Context(), opts))
			want := (&configError{
				setting: opListRoles + ": attribute filter " + strconv.Itoa(tc.place),
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

// TestInvalidRoleListOptions checks that a Page or a PerPage less than zero is an error that
// matches ErrInvalidConfig from List and All, and that neither sends a request.
func TestInvalidRoleListOptions(t *testing.T) {
	for _, opts := range []ListOptions{{Page: -1}, {PerPage: -1}} {
		api := mockpermit.NewAPI(t)
		c := resourceClient(t, api)
		filter := &RolesListOptions{ListOptions: opts, Attributes: goldTier()}
		_, err := c.Roles.List(t.Context(), filter)
		_, errs := collect(t, c.Roles.All(t.Context(), filter))
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

// TestRolesUseTheScope checks that the calls of the roles act on the environment of the client's
// scope. Without WithScope, the client looks up the scope of its API key once, and each call fills
// the route with the project and the environment of an environment-level key. A scope without an
// environment, that of a project-level or an organization-level key, is an error that matches
// ErrInvalidConfig and names the operation, and no call sends a request but the lookup, which the
// API fails for any route that the test did not register.
func TestRolesUseTheScope(t *testing.T) {
	const schema = "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID
	calls := append(append([]roleCall{}, keyedRoleCalls...), unkeyedRoleCalls...)
	t.Run("an environment-level key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		replies := map[string][]mockpermit.Reply{}
		for _, m := range calls {
			replies[m.route] = append(replies[m.route], m.reply)
		}
		for route, r := range replies {
			api.Handle(route, r...)
		}
		c := projectClient(t, api)
		for _, m := range calls {
			if err := m.call(t.Context(), c, "editor"); err != nil {
				t.Errorf("the call of %s returned %v", m.op, err)
			}
		}
		requests := api.Requests()
		if len(requests) != len(calls)+1 || requests[0].RawPath != "/v2/api-key/scope" {
			t.Fatalf("the calls sent %+v, want the lookup of the scope and one request for each "+
				"of the %d calls", requests, len(calls))
		}
		for _, r := range requests[1:] {
			if !strings.HasPrefix(r.RawPath, schema+"/") {
				t.Errorf("a call sent its request to %s, want a path under %s", r.RawPath, schema)
			}
		}
	})
	for _, suffix := range []string{".project", ".organization"} {
		t.Run("the scope of the fixture"+suffix, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(scopeRoute, scopeReply(t, suffix))
			c := projectClient(t, api)
			for _, m := range calls {
				err := m.call(t.Context(), c, "editor")
				if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), m.op) {
					t.Errorf("the call of %s returned %v, want an error that matches "+
						"ErrInvalidConfig and names %s", m.op, err, m.op)
				}
			}
			if _, errs := collect(t, c.Roles.All(t.Context(), nil)); len(errs) != 1 ||
				!errors.Is(errs[0], ErrInvalidConfig) {
				t.Errorf("All yielded %v, want one error that matches ErrInvalidConfig", errs)
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("the calls sent %d requests, want only the lookup of the scope", n)
			}
		})
	}
}

// TestRoleKeysInThePath checks that each method of the roles that names a role sends its key as
// one segment of the path, escaped, after the project and the environment of the scope, whatever
// the key holds: a slash, a percent sign, a space, a question mark and a hash, a letter outside
// ASCII, the shape of a UUID and a leading dot. A key that is empty, . or .. would change the
// route, so it is an error that matches ErrInvalidConfig and names the placeholder, and no
// request.
func TestRoleKeysInThePath(t *testing.T) {
	for key, escaped := range map[string]string{
		"a/b":            "a%2Fb",
		"50%":            "50%25",
		"a b?c#d":        "a%20b%3Fc%23d",
		"é":              "%C3%A9",
		fixtures.UUIDKey: fixtures.UUIDKey,
		".hidden":        ".hidden",
		"role.v2..x":     "role.v2..x",
	} {
		for _, m := range keyedRoleCalls {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, m.reply)
			if err := m.call(t.Context(), resourceClient(t, api), key); err != nil {
				t.Errorf("%s(%q) returned %v", m.name, key, err)
			}
			_, suffix, _ := strings.Cut(m.route, "{role_id}")
			want := "/v2/schema/p/e/roles/" + escaped + suffix
			if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want {
				t.Errorf("%s(%q) sent %+v, want one request to %s", m.name, key, requests, want)
			}
		}
	}
	for _, key := range []string{"", ".", ".."} {
		for _, m := range keyedRoleCalls {
			api := mockpermit.NewAPI(t)
			err := m.call(t.Context(), resourceClient(t, api), key)
			named := err != nil && strings.Contains(err.Error(), "{role_id}")
			if !errors.Is(err, ErrInvalidConfig) || !named {
				t.Errorf("%s(%q) returned %v, want an error that matches ErrInvalidConfig and "+
					"names {role_id}", m.name, key, err)
			}
			api.AssertUntouched()
		}
	}
}

// TestRolesSendNoParentRoute checks that no method of the roles or of the roles of a resource
// sends a request to a route of the parents of a role, which v1's AddParentRole and
// RemoveParentRole, and AddParent and RemoveParent of the roles of a resource, sent and the API's
// documentation does not list: the registry holds no route under /roles/ with /parents, and the
// strict mock, which serves only the routes of the roles that the API lists, fails any request
// to one. A role's parents are the roles that it extends, which its update sets. A route of the
// parents of another object, such as a group, is not one of a role.
func TestRolesSendNoParentRoute(t *testing.T) {
	for _, op := range rest.Default().Ops() {
		_, rolePath, isRole := strings.Cut(op.Route, "/roles/")
		if isRole && strings.Contains(rolePath, "/parents") {
			t.Errorf("the registry holds %s %s, a route of the parents of a role", op.Method,
				op.Route)
		}
	}
	calls := append(append([]roleCall{}, keyedRoleCalls...), unkeyedRoleCalls...)
	api := mockpermit.NewAPI(t)
	replies := map[string][]mockpermit.Reply{}
	for _, m := range calls {
		replies[m.route] = append(replies[m.route], m.reply)
	}
	for route, r := range replies {
		api.Handle(route, r...)
	}
	handleResourceRoleCalls(api, resourceRoleCalls)
	c := resourceClient(t, api)
	for _, m := range calls {
		if err := m.call(t.Context(), c, "editor"); err != nil {
			t.Errorf("%s returned %v", m.name, err)
		}
	}
	for _, m := range resourceRoleCalls {
		if _, err := m.call(t.Context(), c, "document", "editor"); err != nil {
			t.Errorf("%s of the roles of a resource returned %v", m.name, err)
		}
	}
	for _, r := range api.Requests() {
		if strings.Contains(r.RawPath, "/parents") {
			t.Errorf("a call sent %s %s, a route of the parents of a role", r.Method, r.RawPath)
		}
	}
}

// TestDeleteRoleThatIsGone checks that Delete returns nil for a 404, with or without a body, as
// the role that the path names is gone already, after one request with no query; and an *Error
// with the status for any other error status, such as 403, 409 or 422.
func TestDeleteRoleThatIsGone(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"NOT_FOUND","title":"Not Found","message":"the role does not exist"}`, "",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteRoleRoute, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		if err := resourceClient(t, api).Roles.Delete(t.Context(), "gone"); err != nil {
			t.Errorf("Delete of a role that the API answers with 404 and the body %q returned "+
				"%v, want nil", body, err)
		}
		if requests := api.Requests(); len(requests) != 1 || requests[0].RawQuery != "" {
			t.Errorf("Delete sent %+v, want one request without a query", requests)
		}
	}
	for _, status := range []int{
		http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteRoleRoute, mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := resourceClient(t, api).Roles.Delete(t.Context(), "editor")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteRole ||
			e.Detail != "no" || errors.Is(err, ErrNotFound) {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status and the detail", status, err, opDeleteRole)
		}
	}
}

// TestNotFoundOfOtherRoleCalls checks that a 404 to each call of the roles but Delete is an
// *Error that matches ErrNotFound: List, Create and BulkReplace delete nothing, Get, Update,
// AssignPermissions, Ancestors and Descendants neither, and RemovePermissions names in its body
// what it removes, so a 404 to it can mean a mistyped key, not a removal that happened already.
func TestNotFoundOfOtherRoleCalls(t *testing.T) {
	const body = `{"error_code":"NOT_FOUND","title":"Not Found","message":"no such object"}`
	calls := append(append([]roleCall{}, keyedRoleCalls...), unkeyedRoleCalls...)
	calls = slices.DeleteFunc(calls, func(m roleCall) bool { return m.op == opDeleteRole })
	for _, m := range calls {
		api := mockpermit.NewAPI(t)
		api.Handle(m.route, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		err := m.call(t.Context(), resourceClient(t, api), "gone")
		e := mustError(t, err)
		if !errors.Is(err, ErrNotFound) || e.StatusCode != http.StatusNotFound || e.Op != m.op {
			t.Errorf("%s of an object that the API answers with 404 returned %v, want an "+
				"*Error that matches ErrNotFound", m.name, err)
		}
	}
}

// TestRoleBodies checks the bodies of the calls of the roles byte for byte against the golden files
// in testdata/golden: a create with the required fields only, one whose lists and attributes are
// empty, which sends [] and {} for them, and one with every field, with a description that an
// encoder may change, with a line separator and <, > and &, and attributes of a number and an
// object; the zero update, which sends {}, as its nil lists are left out; an update whose lists are
// empty, which sends [] for each, to clear it; one whose lists replace the role's; one that sends
// null for each field that the API clears for null, and one that sets every field; the
// permissions that a grant and a removal send, and [] for nil; and a bulk replace of a role with
// every field and one with the required fields only, of a role whose lists and attributes are
// empty, which sends [] and {} for them, and of nil roles, which sends [].
func TestRoleBodies(t *testing.T) {
	description := "<a>&" + fixtures.LineSeparator + "b"
	derived := models.DerivedRoleBlockEdit{
		UsersWithRole: []models.DerivedRoleRuleCreate{{
			Role: "owner", OnResource: "folder", LinkedByRelation: "parent",
		}},
		When: &models.DerivationSettings{NoDirectRolesOnObject: true},
	}
	attributes := models.Attributes{"seats": 10, "quota": map[string]any{"max": 2.5}}
	update := func(change models.RoleUpdate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.Roles.Update(t.Context(), "editor", change)
			return err
		}
	}
	for _, tc := range []struct {
		golden string
		route  string
		call   func(c *Client) error
	}{
		{
			"create_role.required.golden", createRoleRoute,
			func(c *Client) error {
				_, err := c.Roles.Create(t.Context(), models.RoleCreate{Key: "r", Name: "R"})
				return err
			},
		},
		{
			"create_role.empty-lists.golden", createRoleRoute,
			func(c *Client) error {
				_, err := c.Roles.Create(t.Context(), models.RoleCreate{
					Key: "r", Name: "R", Permissions: []string{}, Attributes: models.Attributes{},
					Extends: []string{},
				})
				return err
			},
		},
		{
			"create_role.every-field.golden", createRoleRoute,
			func(c *Client) error {
				_, err := c.Roles.Create(t.Context(), models.RoleCreate{
					Key: "r", Name: "R", Description: description,
					Permissions: []string{"document:read"}, Attributes: attributes,
					Extends: []string{"viewer"}, GrantedTo: &derived,
				})
				return err
			},
		},
		{"update_role.unset.golden", updateRoleRoute, update(models.RoleUpdate{})},
		{
			"update_role.empty-lists.golden", updateRoleRoute,
			update(models.RoleUpdate{Permissions: []string{}, Extends: []string{}}),
		},
		{
			"update_role.lists.golden", updateRoleRoute,
			update(models.RoleUpdate{
				Permissions: []string{"document:read", "document:write"},
				Extends:     []string{"viewer", "commenter"},
			}),
		},
		{
			"update_role.null.golden", updateRoleRoute,
			update(models.RoleUpdate{
				Description: models.Null[string](), Attributes: models.Null[models.Attributes](),
				GrantedTo: models.Null[models.DerivedRoleBlockEdit](),
			}),
		},
		{
			"update_role.values.golden", updateRoleRoute,
			update(models.RoleUpdate{
				Name: "R", Description: models.Some(description),
				Permissions: []string{"document:read"},
				Attributes:  models.Some(models.Attributes{}), Extends: []string{"viewer"},
				GrantedTo: models.Some(derived),
			}),
		},
		{
			"assign_permissions_to_role.values.golden", assignRolePermissionsRoute,
			func(c *Client) error {
				_, err := c.Roles.AssignPermissions(t.Context(), "editor",
					[]string{"document:read", "folder:list"})
				return err
			},
		},
		{
			"assign_permissions_to_role.nil.golden", assignRolePermissionsRoute,
			func(c *Client) error {
				_, err := c.Roles.AssignPermissions(t.Context(), "editor", nil)
				return err
			},
		},
		{
			"remove_permissions_from_role.values.golden", removeRolePermissionsRoute,
			func(c *Client) error {
				_, err := c.Roles.RemovePermissions(t.Context(), "editor",
					[]string{"document:write"})
				return err
			},
		},
		{
			"remove_permissions_from_role.nil.golden", removeRolePermissionsRoute,
			func(c *Client) error {
				_, err := c.Roles.RemovePermissions(t.Context(), "editor", nil)
				return err
			},
		},
		{
			"bulk_create_or_replace_roles.every-field.golden", bulkReplaceRolesRoute,
			func(c *Client) error {
				_, err := c.Roles.BulkReplace(t.Context(), []models.RoleCreateBulk{
					{
						Key: "auditor", Name: "Auditor", Description: description,
						Permissions: []string{"document:read"}, Attributes: attributes,
						Extends: []string{"viewer"}, GrantedTo: &derived, Resource: "document",
					},
					{Key: "viewer", Name: "Viewer"},
				})
				return err
			},
		},
		{
			"bulk_create_or_replace_roles.empty-lists.golden", bulkReplaceRolesRoute,
			func(c *Client) error {
				_, err := c.Roles.BulkReplace(t.Context(), []models.RoleCreateBulk{{
					Key: "r", Name: "R", Permissions: []string{}, Attributes: models.Attributes{},
					Extends: []string{},
				}})
				return err
			},
		},
		{
			"bulk_create_or_replace_roles.nil.golden", bulkReplaceRolesRoute,
			func(c *Client) error {
				_, err := c.Roles.BulkReplace(t.Context(), nil)
				return err
			},
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			reply := roleBody
			if tc.route == bulkReplaceRolesRoute {
				reply = bulkRolesBody
			}
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: reply})
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

// TestBulkReplaceReturnsTheReply checks that BulkReplace returns the lists of the reply, which
// name other roles than the request, and in another order, and empty lists as empty, not nil,
// never the roles that it sent.
func TestBulkReplaceReturnsTheReply(t *testing.T) {
	for reply, want := range map[string]models.RoleCreateBulkOperationResult{
		`{"created":["b"],"updated":["c","a"]}`: {
			Created: []string{"b"}, Updated: []string{"c", "a"},
		},
		`{"created":[],"updated":[]}`: {Created: []string{}, Updated: []string{}},
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(bulkReplaceRolesRoute, mockpermit.Reply{Body: reply})
		got, err := resourceClient(t, api).Roles.BulkReplace(t.Context(), []models.RoleCreateBulk{
			{Key: "a", Name: "A"}, {Key: "x", Name: "X"},
		})
		if err != nil {
			t.Fatalf("BulkReplace of the reply %s returned %v", reply, err)
		}
		if diff := cmp.Diff(&want, got); diff != "" || got.Created == nil || got.Updated == nil {
			t.Errorf("BulkReplace of the reply %s returned a result that differs (-want +got), "+
				"or nil lists:\n%s", reply, diff)
		}
	}
}

// relativesCall is a call of Ancestors or Descendants of the role editor, with the ID of its
// operation and its route.
type relativesCall struct {
	name, op, route string
	call            func(ctx context.Context, c *Client) ([]models.RoleRead, error)
}

// relativesCalls returns the calls of Ancestors and Descendants of the role editor.
func relativesCalls() []relativesCall {
	return []relativesCall{
		{
			"Ancestors", opRoleAncestors, roleAncestorsRoute,
			func(ctx context.Context, c *Client) ([]models.RoleRead, error) {
				return c.Roles.Ancestors(ctx, "editor")
			},
		},
		{
			"Descendants", opRoleDescendants, roleDescendantsRoute,
			func(ctx context.Context, c *Client) ([]models.RoleRead, error) {
				return c.Roles.Descendants(ctx, "editor")
			},
		},
	}
}

// TestRoleRelativesReadBothForms checks that Ancestors and Descendants return the roles of the
// reply in its order, whether its roles are a JSON array or the paginated object of the API,
// whose data array holds them, with or without its counts; and an empty slice, not nil, when its
// roles are an empty array or an empty page, null or left out.
func TestRoleRelativesReadBothForms(t *testing.T) {
	roles := []models.RoleRead{rolesOf(2, 3)[0], rolesOf(0, 1)[0], rolesOf(1, 2)[0]}
	array := arrayOf(t, roles).Body
	for _, tc := range []struct {
		name  string
		reply string
		want  []models.RoleRead
	}{
		{"an array", `{"roles":` + array + `}`, roles},
		{"a page", `{"roles":{"data":` + array + `,"total_count":3,"page_count":3}}`, roles},
		{"a page without counts", `{"roles":{"data":` + array + `}}`, roles},
		{"an empty array", `{"roles":[]}`, []models.RoleRead{}},
		{"an empty page", `{"roles":{"data":[],"total_count":0}}`, []models.RoleRead{}},
		{"null", `{"roles":null}`, []models.RoleRead{}},
		{"no roles", `{}`, []models.RoleRead{}},
	} {
		for _, m := range relativesCalls() {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, mockpermit.Reply{Body: tc.reply})
			got, err := m.call(t.Context(), resourceClient(t, api))
			if err != nil {
				t.Fatalf("%s of %s returned %v", m.name, tc.name, err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" || got == nil {
				t.Errorf("%s of %s returned roles that differ (-want +got), or nil:\n%s", m.name,
					tc.name, diff)
			}
		}
	}
}

// TestRoleBodiesThatCannotBeJSON checks that a create, an update and a bulk replace whose
// attributes hold a value that JSON cannot hold, and an update whose attributes are a nil map of
// Some, which would send null for a value, are errors that match ErrInvalidConfig and name the
// operation, and send no request.
func TestRoleBodiesThatCannotBeJSON(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := resourceClient(t, api)
	_, createErr := c.Roles.Create(t.Context(), models.RoleCreate{
		Key: "r", Name: "R", Attributes: models.Attributes{"c": make(chan int)},
	})
	_, updateErr := c.Roles.Update(t.Context(), "r", models.RoleUpdate{
		Attributes: models.Some(models.Attributes{"n": math.NaN()}),
	})
	_, nilErr := c.Roles.Update(t.Context(), "r", models.RoleUpdate{
		Attributes: models.Some[models.Attributes](nil),
	})
	_, bulkErr := c.Roles.BulkReplace(t.Context(), []models.RoleCreateBulk{
		{Key: "r", Name: "R", Attributes: models.Attributes{"f": func() {}}},
	})
	for _, tc := range []struct {
		op  string
		err error
	}{
		{opCreateRole, createErr},
		{opUpdateRole, updateErr},
		{opUpdateRole, nilErr},
		{opBulkReplaceRoles, bulkErr},
	} {
		if !errors.Is(tc.err, ErrInvalidConfig) || !strings.Contains(tc.err.Error(), tc.op) {
			t.Errorf("%s returned %v, want an error that matches ErrInvalidConfig and names it",
				tc.op, tc.err)
		}
	}
	api.AssertUntouched()
}

// TestRoleRetries checks when a call of the roles sends its request again: Create, which the API
// may have acted on, after a 429 only, so a 503 ends it after one request; each other call, which
// does the same when sent twice, after a 503 too.
func TestRoleRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	create := unkeyedRoleCalls[1]
	type retryCase struct {
		name    string
		m       roleCall
		replies []mockpermit.Reply
		status  int
	}
	tests := []retryCase{
		{name: "Create after a 429", m: create, replies: []mockpermit.Reply{limited, create.reply}},
		{
			name: "Create after a 503", m: create, replies: []mockpermit.Reply{unavailable},
			status: http.StatusServiceUnavailable,
		},
	}
	for _, m := range append(append([]roleCall{}, keyedRoleCalls...), unkeyedRoleCalls...) {
		if m.op != opCreateRole {
			tests = append(tests, retryCase{
				name: m.name + " after a 503", m: m,
				replies: []mockpermit.Reply{unavailable, m.reply},
			})
		}
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.m.route, tc.replies...)
			c := resourceClient(t, api, WithRetry(RetryPolicy{MaxRetries: 1}))
			err := tc.m.call(t.Context(), c, "editor")
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

// TestMalformedRoleReplies checks that a reply in 200-299 that is not a role, to each method that
// returns one, or for List and All neither a JSON array nor the paginated object of roles, for
// Ancestors and Descendants not an object whose roles are either, and for BulkReplace not its
// result, is an *Error that names the operation, its route and the model, and no role, no page,
// no role of the iterator, no roles and no result. Attributes that are not a JSON object, such as
// a string, are not those of a role either.
func TestMalformedRoleReplies(t *testing.T) {
	const prefix = "permit: "
	for _, m := range []struct {
		name, op, route string
		call            func(ctx context.Context, c *Client) (*models.RoleRead, error)
	}{
		{
			"Get", opGetRole, getRoleRoute,
			func(ctx context.Context, c *Client) (*models.RoleRead, error) {
				return c.Roles.Get(ctx, "editor")
			},
		},
		{
			"Create", opCreateRole, createRoleRoute,
			func(ctx context.Context, c *Client) (*models.RoleRead, error) {
				return c.Roles.Create(ctx, models.RoleCreate{Key: "editor", Name: "Editor"})
			},
		},
		{
			"Update", opUpdateRole, updateRoleRoute,
			func(ctx context.Context, c *Client) (*models.RoleRead, error) {
				return c.Roles.Update(ctx, "editor", models.RoleUpdate{Name: "Editor"})
			},
		},
		{
			"AssignPermissions", opAssignRolePermissions, assignRolePermissionsRoute,
			func(ctx context.Context, c *Client) (*models.RoleRead, error) {
				return c.Roles.AssignPermissions(ctx, "editor", []string{"document:read"})
			},
		},
		{
			"RemovePermissions", opRemoveRolePermissions, removeRolePermissionsRoute,
			func(ctx context.Context, c *Client) (*models.RoleRead, error) {
				return c.Roles.RemovePermissions(ctx, "editor", []string{"document:read"})
			},
		},
	} {
		want := prefix + m.op + ": " + m.route + ": status 200: the reply is not a valid " +
			"models.RoleRead"
		for _, body := range []string{``, `null`, `[]`, `"a"`, `{"attributes":"a"}`, `{} {}`} {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, mockpermit.Reply{Body: body})
			role, err := m.call(t.Context(), resourceClient(t, api))
			if e := mustError(t, err); role != nil || e.Error() != want {
				t.Errorf("%s of the reply %q returned %+v and\n%v\nwant no role and\n%s", m.name,
					body, role, err, want)
			}
		}
	}
	for _, body := range []string{``, `null`, `"a"`, `[1]`, `[{"attributes":[]}]`, `{"data":{}}`} {
		api := mockpermit.NewAPI(t)
		api.Handle(listRolesRoute, mockpermit.Reply{Body: body}, mockpermit.Reply{Body: body})
		c := resourceClient(t, api)
		page, err := c.Roles.List(t.Context(), nil)
		want := prefix + "list_roles: GET /v2/schema/{proj_id}/{env_id}/roles: status 200: " +
			"the reply is not a valid list of models.RoleRead"
		if e := mustError(t, err); page != nil || e.Error() != want {
			t.Errorf("List of the reply %q returned %+v and\n%v\nwant no page and\n%s", body,
				page, err, want)
		}
		items, errs := collect(t, c.Roles.All(t.Context(), nil))
		if items != nil || len(errs) != 1 || mustError(t, errs[0]).Error() != want {
			t.Errorf("All of the reply %q yielded %+v and %v, want only the error\n%s", body,
				items, errs, want)
		}
	}
	for _, m := range relativesCalls() {
		_, route, _ := strings.Cut(m.route, " ")
		want := prefix + m.op + ": GET " + route + ": status 200: the reply is not a valid " +
			"models.RoleList"
		for _, body := range []string{
			``, `null`, `[]`, `"a"`, `{"roles":"a"}`, `{"roles":{}}`, `{"roles":{"data":{}}}`,
			`{"roles":[1]}`, `{"roles":[{"attributes":"a"}]}`, `{"roles":[]} {}`,
		} {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, mockpermit.Reply{Body: body})
			roles, err := m.call(t.Context(), resourceClient(t, api))
			if e := mustError(t, err); roles != nil || e.Error() != want {
				t.Errorf("%s of the reply %q returned %+v and\n%v\nwant no roles and\n%s", m.name,
					body, roles, err, want)
			}
		}
	}
	for _, body := range []string{``, `null`, `[]`, `{"created":"a"}`, `{"updated":[1]}`} {
		api := mockpermit.NewAPI(t)
		api.Handle(bulkReplaceRolesRoute, mockpermit.Reply{Body: body})
		result, err := resourceClient(t, api).Roles.BulkReplace(t.Context(), nil)
		want := prefix + "bulk_create_or_replace_roles: PUT /v2/schema/{proj_id}/{env_id}/" +
			"bulk/roles: status 200: the reply is not a valid models.RoleCreateBulkOperationResult"
		if e := mustError(t, err); result != nil || e.Error() != want {
			t.Errorf("BulkReplace of the reply %q returned %+v and\n%v\nwant no result and\n%s",
				body, result, err, want)
		}
	}
}

// TestRolesAfterClose checks that each method returns ErrClosed after Close, and sends nothing,
// also without WithScope, whose lookup it does not send either.
func TestRolesAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	_, allErrs := collect(t, c.Roles.All(t.Context(), nil))
	var errs []error
	for _, m := range append(append([]roleCall{}, keyedRoleCalls...), unkeyedRoleCalls...) {
		errs = append(errs, m.call(t.Context(), c, "editor"))
	}
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
