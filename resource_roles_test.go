package permit

import (
	"context"
	"encoding/json"
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

// resourceRolesPath is the path template of the roles of a resource.
const resourceRolesPath = "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles"

// The routes of the roles of a resource, as mockpermit.Server.Handle takes them.
const (
	resourceRolePath                   = resourceRolesPath + "/{role_id}"
	listResourceRolesRoute             = "GET " + resourceRolesPath
	createResourceRoleRoute            = "POST " + resourceRolesPath
	getResourceRoleRoute               = "GET " + resourceRolePath
	updateResourceRoleRoute            = "PATCH " + resourceRolePath
	deleteResourceRoleRoute            = "DELETE " + resourceRolePath
	assignResourceRolePermissionsRoute = "POST " + resourceRolePath + "/permissions"
	removeResourceRolePermissionsRoute = "DELETE " + resourceRolePath + "/permissions"
)

// resourceRoleBody is the body of a reply of the API that holds the role editor of the resource
// document.
const resourceRoleBody = `{"name":"Editor","key":"editor",` +
	`"id":"00000000-0000-4000-8000-000000000112",` +
	`"organization_id":"00000000-0000-4000-8000-000000000001",` +
	`"project_id":"00000000-0000-4000-8000-000000000002",` +
	`"environment_id":"00000000-0000-4000-8000-000000000003",` +
	`"resource_id":"00000000-0000-4000-8000-000000000021","resource":"document",` +
	`"created_at":"2026-10-14T09:30:00Z","updated_at":"2026-10-14T09:30:00Z"}`

// resourceRolesOf returns the roles r<from> to r<to-1>, with only a key and a name: an empty list
// when to is from.
func resourceRolesOf(from, to int) []models.ResourceRoleRead {
	items := make([]models.ResourceRoleRead, 0, to-from)
	for i := from; i < to; i++ {
		key := "r" + strconv.Itoa(i)
		items = append(items, models.ResourceRoleRead{Key: key, Name: key})
	}
	return items
}

// oneResourceRole returns role as the only role of a list, or nil when role is nil, with err.
func oneResourceRole(role *models.ResourceRoleRead, err error,
) ([]models.ResourceRoleRead, error) {
	if role == nil {
		return nil, err
	}
	return []models.ResourceRoleRead{*role}, err
}

// resourceRoleCall is a call of a method of the roles of a resource, with the ID of its
// operation, its route and a reply of the API to it. Its call takes the key of the resource and,
// when keyed, that of the role, and returns the roles that the method returns: a page's, one, or
// none.
type resourceRoleCall struct {
	name  string
	op    string
	route string
	reply mockpermit.Reply
	keyed bool
	call  func(ctx context.Context, c *Client, resourceKey, roleKey string,
	) ([]models.ResourceRoleRead, error)
}

// resourceRoleCalls are calls of each method of the roles of a resource but All.
var resourceRoleCalls = []resourceRoleCall{
	{
		name: "List", op: opListResourceRoles, route: listResourceRolesRoute,
		reply: mockpermit.Reply{Body: `[` + resourceRoleBody + `]`},
		call: func(ctx context.Context, c *Client, resourceKey, _ string,
		) ([]models.ResourceRoleRead, error) {
			page, err := c.ResourceRoles.List(ctx, resourceKey, nil)
			if page == nil {
				return nil, err
			}
			return page.Data, err
		},
	},
	{
		name: "Create", op: opCreateResourceRole, route: createResourceRoleRoute,
		reply: mockpermit.Reply{Body: resourceRoleBody},
		call: func(ctx context.Context, c *Client, resourceKey, _ string,
		) ([]models.ResourceRoleRead, error) {
			return oneResourceRole(c.ResourceRoles.Create(ctx, resourceKey,
				models.ResourceRoleCreate{Key: "editor", Name: "Editor"}))
		},
	},
	{
		name: "Get", op: opGetResourceRole, route: getResourceRoleRoute, keyed: true,
		reply: mockpermit.Reply{Body: resourceRoleBody},
		call: func(ctx context.Context, c *Client, resourceKey, roleKey string,
		) ([]models.ResourceRoleRead, error) {
			return oneResourceRole(c.ResourceRoles.Get(ctx, resourceKey, roleKey))
		},
	},
	{
		name: "Update", op: opUpdateResourceRole, route: updateResourceRoleRoute, keyed: true,
		reply: mockpermit.Reply{Body: resourceRoleBody},
		call: func(ctx context.Context, c *Client, resourceKey, roleKey string,
		) ([]models.ResourceRoleRead, error) {
			return oneResourceRole(c.ResourceRoles.Update(ctx, resourceKey, roleKey,
				models.ResourceRoleUpdate{Name: "Editor"}))
		},
	},
	{
		name: "AssignPermissions", op: opAssignResourceRolePermissions, keyed: true,
		route: assignResourceRolePermissionsRoute, reply: mockpermit.Reply{Body: resourceRoleBody},
		call: func(ctx context.Context, c *Client, resourceKey, roleKey string,
		) ([]models.ResourceRoleRead, error) {
			return oneResourceRole(c.ResourceRoles.AssignPermissions(ctx, resourceKey, roleKey,
				[]string{"document:read"}))
		},
	},
	{
		name: "RemovePermissions", op: opRemoveResourceRolePermissions, keyed: true,
		route: removeResourceRolePermissionsRoute, reply: mockpermit.Reply{Body: resourceRoleBody},
		call: func(ctx context.Context, c *Client, resourceKey, roleKey string,
		) ([]models.ResourceRoleRead, error) {
			return oneResourceRole(c.ResourceRoles.RemovePermissions(ctx, resourceKey, roleKey,
				[]string{"document:read"}))
		},
	},
	{
		name: "Delete", op: opDeleteResourceRole, route: deleteResourceRoleRoute, keyed: true,
		reply: mockpermit.Reply{Status: http.StatusNoContent},
		call: func(ctx context.Context, c *Client, resourceKey, roleKey string,
		) ([]models.ResourceRoleRead, error) {
			return nil, c.ResourceRoles.Delete(ctx, resourceKey, roleKey)
		},
	},
}

// handleResourceRoleCalls registers the reply of each of calls with api, once for each call
// that names its route.
func handleResourceRoleCalls(api *mockpermit.Server, calls []resourceRoleCall) {
	replies := map[string][]mockpermit.Reply{}
	for _, m := range calls {
		replies[m.route] = append(replies[m.route], m.reply)
	}
	for route, r := range replies {
		api.Handle(route, r...)
	}
}

// TestResourceRoleAttributesOfAnyType checks that each method that returns a role, and All, reads
// attributes whose values are of each JSON type: a string, a fraction, an integer beyond 2^53,
// which it keeps as a json.Number with every digit, a boolean, null, an array and an object. v1's
// model of a role of a resource had no attributes.
func TestResourceRoleAttributesOfAnyType(t *testing.T) {
	attributes := `{"tier":"gold","ratio":0.5,"limit":9007199254740993,"on":true,"gone":null,` +
		`"tags":["a",1],"quota":{"max":3,"regions":["eu"]}}`
	body := `{"name":"Editor","key":"editor","attributes":` + attributes + `}`
	want := models.Attributes{
		"tier": "gold", "ratio": json.Number("0.5"), "limit": json.Number("9007199254740993"),
		"on": true, "gone": nil, "tags": []any{"a", json.Number("1")},
		"quota": map[string]any{"max": json.Number("3"), "regions": []any{"eu"}},
	}
	for _, m := range resourceRoleCalls {
		if m.op == opDeleteResourceRole {
			continue
		}
		api := mockpermit.NewAPI(t)
		reply := mockpermit.Reply{Body: body}
		if m.op == opListResourceRoles {
			reply.Body = `[` + body + `]`
		}
		api.Handle(m.route, reply)
		roles, err := m.call(t.Context(), resourceClient(t, api), "document", "editor")
		if err != nil || len(roles) != 1 {
			t.Fatalf("%s returned the roles %+v and %v, want one role", m.name, roles, err)
		}
		if diff := cmp.Diff(want, roles[0].Attributes); diff != "" {
			t.Errorf("%s read attributes that differ (-want +got):\n%s", m.name, diff)
		}
	}
	api := mockpermit.NewAPI(t)
	api.Handle(listResourceRolesRoute, mockpermit.Reply{Body: `[` + body + `]`},
		mockpermit.Reply{Body: `[]`})
	roles, errs := collect(t, resourceClient(t, api).ResourceRoles.All(t.Context(), "document",
		nil))
	if len(roles) != 1 || errs != nil || !cmp.Equal(want, roles[0].Attributes) {
		t.Errorf("All yielded the roles %+v and the errors %v, want one role with the attributes "+
			"%v", roles, errs, want)
	}
}

// TestResourceRolesAllReadsEveryPage checks that All reads the 31 roles of a resource, in order,
// asking for each page in turn: at a per_page of 30, two pages of roles and the empty page after
// them; and at the default per_page of 100, the most that the API gives on a page, one page of 31
// and the empty page after it.
func TestResourceRolesAllReadsEveryPage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    *ListOptions
		pages   [][]models.ResourceRoleRead
		queries []string
	}{
		{
			name: "per_page 30", opts: &ListOptions{PerPage: 30},
			pages: [][]models.ResourceRoleRead{
				resourceRolesOf(0, 30), resourceRolesOf(30, 31), {},
			},
			queries: []string{
				"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30",
			},
		},
		{
			name:    "the default per_page",
			pages:   [][]models.ResourceRoleRead{resourceRolesOf(0, 31), {}},
			queries: []string{"page=1&per_page=100", "page=2&per_page=100"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			var replies []mockpermit.Reply
			for _, items := range tc.pages {
				replies = append(replies, arrayOf(t, items))
			}
			api.Handle(listResourceRolesRoute, replies...)
			seq := resourceClient(t, api).ResourceRoles.All(t.Context(), "document", tc.opts)
			items, errs := collect(t, seq)
			if diff := cmp.Diff(resourceRolesOf(0, 31), items); diff != "" || errs != nil {
				t.Errorf("All yielded the errors %v and roles that differ (-want +got):\n%s",
					errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestResourceRolesAllYieldsAnErrorOnce checks that a page that fails ends the iterator with its
// error, once, after the roles of the pages before it, and that the options of All are read when
// it is called: a later change to them changes no request.
func TestResourceRolesAllYieldsAnErrorOnce(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(listResourceRolesRoute, arrayOf(t, resourceRolesOf(0, 30)),
		mockpermit.Reply{Status: http.StatusForbidden, Body: body})
	opts := &ListOptions{PerPage: 30}
	seq := resourceClient(t, api).ResourceRoles.All(t.Context(), "document", opts)
	opts.PerPage = 50
	items, errs := collect(t, seq)
	if len(items) != 30 || len(errs) != 1 || !errors.Is(errs[0], ErrForbidden) {
		t.Errorf("All yielded %d roles and the errors %v, want 30 and one error that matches "+
			"ErrForbidden", len(items), errs)
	}
	if diff := cmp.Diff([]string{"page=1&per_page=30", "page=2&per_page=30"},
		queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestResourceRolesListReturnsAPage checks the page that List returns: the roles of the reply in
// its order, the page and the per_page that it asked for, page 1 and no query for nil options and
// for the zero options, and no counts, as the reply is a JSON array. A page without roles has an
// empty list, not nil.
func TestResourceRolesListReturnsAPage(t *testing.T) {
	for _, tc := range []struct {
		opts  *ListOptions
		items []models.ResourceRoleRead
		query string
		want  *Page[models.ResourceRoleRead]
	}{
		{
			items: resourceRolesOf(0, 2),
			want:  &Page[models.ResourceRoleRead]{Data: resourceRolesOf(0, 2), Page: 1},
		},
		{
			opts: &ListOptions{}, items: resourceRolesOf(0, 1),
			want: &Page[models.ResourceRoleRead]{Data: resourceRolesOf(0, 1), Page: 1},
		},
		{
			opts: &ListOptions{Page: 3, PerPage: 30}, items: []models.ResourceRoleRead{},
			query: "page=3&per_page=30",
			want: &Page[models.ResourceRoleRead]{
				Data: []models.ResourceRoleRead{}, Page: 3, PerPage: 30,
			},
		},
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(listResourceRolesRoute, arrayOf(t, tc.items))
		got, err := resourceClient(t, api).ResourceRoles.List(t.Context(), "document", tc.opts)
		if err != nil {
			t.Fatalf("List with %+v returned %v", tc.opts, err)
		}
		if diff := cmp.Diff(tc.want, got); diff != "" || got.Data == nil {
			t.Errorf("List with %+v returned a page that differs (-want +got), or nil Data:\n%s",
				tc.opts, diff)
		}
		want := "/v2/schema/p/e/resources/document/roles"
		if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want ||
			requests[0].RawQuery != tc.query {
			t.Errorf("List sent %+v, want one request to %s?%s", requests, want, tc.query)
		}
	}
}

// TestInvalidResourceRoleListOptions checks that a Page or a PerPage less than zero is an error
// that matches ErrInvalidConfig from List and All, and that neither sends a request.
func TestInvalidResourceRoleListOptions(t *testing.T) {
	for _, opts := range []ListOptions{{Page: -1}, {PerPage: -1}} {
		api := mockpermit.NewAPI(t)
		c := resourceClient(t, api)
		_, err := c.ResourceRoles.List(t.Context(), "document", &opts)
		_, errs := collect(t, c.ResourceRoles.All(t.Context(), "document", &opts))
		if len(errs) != 1 {
			t.Errorf("%+v: All yielded the errors %v, want one", opts, errs)
		}
		for _, err := range append([]error{err}, errs...) {
			if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("%+v: the call returned %v, want an error that matches ErrInvalidConfig",
					opts, err)
			}
		}
		api.AssertUntouched()
	}
}

// TestResourceRolesUseTheScope checks that the calls of the roles of a resource act on the
// environment of the client's scope. Without WithScope, the client looks up the scope of its API
// key once, and each call fills the route with the project and the environment of an
// environment-level key. A scope without an environment, that of a project-level or an
// organization-level key, is an error that matches ErrInvalidConfig and names the operation, and
// no call sends a request but the lookup, which the API fails for any route that the test did not
// register. All looks the scope up in each loop, not when it is called.
func TestResourceRolesUseTheScope(t *testing.T) {
	const prefix = "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID +
		"/resources/document/roles"
	t.Run("an environment-level key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		handleResourceRoleCalls(api, resourceRoleCalls)
		c := projectClient(t, api)
		for _, m := range resourceRoleCalls {
			if _, err := m.call(t.Context(), c, "document", "editor"); err != nil {
				t.Errorf("the call of %s returned %v", m.op, err)
			}
		}
		requests := api.Requests()
		if len(requests) != len(resourceRoleCalls)+1 ||
			requests[0].RawPath != "/v2/api-key/scope" {
			t.Fatalf("the calls sent %+v, want the lookup of the scope and one request for each "+
				"of the %d calls", requests, len(resourceRoleCalls))
		}
		for _, r := range requests[1:] {
			if r.RawPath != prefix && !strings.HasPrefix(r.RawPath, prefix+"/editor") {
				t.Errorf("a call sent its request to %s, want a path under %s", r.RawPath, prefix)
			}
		}
	})
	for _, suffix := range []string{".project", ".organization"} {
		t.Run("the scope of the fixture"+suffix, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(scopeRoute, scopeReply(t, suffix))
			c := projectClient(t, api)
			for _, m := range resourceRoleCalls {
				_, err := m.call(t.Context(), c, "document", "editor")
				if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), m.op) {
					t.Errorf("the call of %s returned %v, want an error that matches "+
						"ErrInvalidConfig and names %s", m.op, err, m.op)
				}
			}
			_, errs := collect(t, c.ResourceRoles.All(t.Context(), "document", nil))
			if len(errs) != 1 || !errors.Is(errs[0], ErrInvalidConfig) {
				t.Errorf("All yielded %v, want one error that matches ErrInvalidConfig", errs)
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("the calls sent %d requests, want only the lookup of the scope", n)
			}
		})
	}
	t.Run("All looks the scope up in each loop", func(t *testing.T) {
		const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, mockpermit.Reply{Status: http.StatusForbidden, Body: body},
			scopeReply(t, ""))
		api.Handle(listResourceRolesRoute, arrayOf(t, resourceRolesOf(0, 1)),
			arrayOf(t, resourceRolesOf(1, 1)))
		seq := projectClient(t, api).ResourceRoles.All(t.Context(), "document", nil)
		if requests := api.Requests(); len(requests) != 0 {
			t.Fatalf("All sent %+v before a loop, want no request", requests)
		}
		if items, errs := collect(t, seq); items != nil || len(errs) != 1 ||
			!errors.Is(errs[0], ErrForbidden) {
			t.Errorf("the first loop yielded %v and the errors %v, want one error that matches "+
				"ErrForbidden", items, errs)
		}
		if items, errs := collect(t, seq); !cmp.Equal(resourceRolesOf(0, 1), items) ||
			errs != nil {
			t.Errorf("the second loop yielded %v and the errors %v, want r0", items, errs)
		}
		if n := len(api.Requests()); n != 4 {
			t.Errorf("the loops sent %d requests, want two lookups and two pages", n)
		}
	})
}

// TestResourceRoleKeysInThePath checks that each method sends the key of the resource, and the
// key of the role when it names one, each as one segment of the path, escaped, after the project
// and the environment of the scope, whatever the key holds: escaped where the key holds a slash,
// a percent sign, a space, a question mark and a hash, or a letter outside ASCII, and as it is
// for the shape of a UUID, a leading dot and dots inside a key. A key that is empty, . or .. would
// change the route, so it is an error that matches ErrInvalidConfig and names its placeholder, and
// no request.
func TestResourceRoleKeysInThePath(t *testing.T) {
	const prefix = "/v2/schema/p/e/resources/"
	for key, segment := range map[string]string{
		"a/b":            "a%2Fb",
		"50%":            "50%25",
		"a b?c#d":        "a%20b%3Fc%23d",
		"é":              "%C3%A9",
		fixtures.UUIDKey: fixtures.UUIDKey,
		".hidden":        ".hidden",
		"role.v2..x":     "role.v2..x",
	} {
		for _, m := range resourceRoleCalls {
			_, suffix, _ := strings.Cut(m.route, "{role_id}")
			wants := map[[2]string]string{{key, "editor"}: prefix + segment + "/roles"}
			if m.keyed {
				wants = map[[2]string]string{
					{key, "editor"}:   prefix + segment + "/roles/editor" + suffix,
					{"document", key}: prefix + "document/roles/" + segment + suffix,
				}
			}
			for keys, want := range wants {
				api := mockpermit.NewAPI(t)
				api.Handle(m.route, m.reply)
				if _, err := m.call(t.Context(), resourceClient(t, api), keys[0],
					keys[1]); err != nil {
					t.Errorf("%s(%q, %q) returned %v", m.name, keys[0], keys[1], err)
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
		for _, m := range resourceRoleCalls {
			api := mockpermit.NewAPI(t)
			c := resourceClient(t, api)
			placeholders := map[string][2]string{"{resource_id}": {key, "editor"}}
			if m.keyed {
				placeholders["{role_id}"] = [2]string{"document", key}
			}
			for placeholder, keys := range placeholders {
				_, err := m.call(t.Context(), c, keys[0], keys[1])
				named := err != nil && strings.Contains(err.Error(), placeholder)
				if !errors.Is(err, ErrInvalidConfig) || !named {
					t.Errorf("%s(%q, %q) returned %v, want an error that matches "+
						"ErrInvalidConfig and names %s", m.name, keys[0], keys[1], err,
						placeholder)
				}
			}
			api.AssertUntouched()
		}
	}
}

// TestDeleteResourceRoleThatIsGone checks that Delete returns nil for a 404, with or without a
// body, as the role that the path names is gone already, after one request with no query; and an
// *Error with the status for any other error status, such as 403, 409 or 422.
func TestDeleteResourceRoleThatIsGone(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"NOT_FOUND","title":"Not Found","message":"the role does not exist"}`, "",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteResourceRoleRoute,
			mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		err := resourceClient(t, api).ResourceRoles.Delete(t.Context(), "document", "gone")
		if err != nil {
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
		api.Handle(deleteResourceRoleRoute,
			mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := resourceClient(t, api).ResourceRoles.Delete(t.Context(), "document", "editor")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteResourceRole ||
			e.Detail != "no" || errors.Is(err, ErrNotFound) {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status and the detail", status, err, opDeleteResourceRole)
		}
	}
}

// TestNotFoundOfOtherResourceRoleCalls checks that a 404 to each call of the roles of a resource
// but Delete is an *Error that matches ErrNotFound: List, Create, Get, Update and
// AssignPermissions delete nothing, and RemovePermissions names in its body what it removes, so a
// 404 to it can mean a mistyped key, not a removal that happened already.
func TestNotFoundOfOtherResourceRoleCalls(t *testing.T) {
	const body = `{"error_code":"NOT_FOUND","title":"Not Found","message":"no such object"}`
	for _, m := range resourceRoleCalls {
		if m.op == opDeleteResourceRole {
			continue
		}
		api := mockpermit.NewAPI(t)
		api.Handle(m.route, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		_, err := m.call(t.Context(), resourceClient(t, api), "document", "gone")
		e := mustError(t, err)
		if !errors.Is(err, ErrNotFound) || e.StatusCode != http.StatusNotFound || e.Op != m.op {
			t.Errorf("%s of an object that the API answers with 404 returned %v, want an "+
				"*Error that matches ErrNotFound", m.name, err)
		}
	}
}

// TestResourceRoleBodies checks the bodies of the calls of the roles of a resource byte for byte
// against the golden files in testdata/golden: a create with the required fields only, one whose
// lists and attributes are empty, which sends [] and {} for them, and one with every field, with
// a description that an encoder may change, with a line separator and <, > and &, and attributes
// of a number and an object; the zero update, which sends {}, as its nil lists are left out, and
// not null; an update whose lists are empty, which sends [] for each, to clear it; one that sends
// null for each Optional field, and one that sets every field; and the permissions that a grant
// and a removal send, and [] for nil.
func TestResourceRoleBodies(t *testing.T) {
	description := "<a>&" + fixtures.LineSeparator + "b"
	derived := models.DerivedRoleBlockEdit{
		UsersWithRole: []models.DerivedRoleRuleCreate{{
			Role: "owner", OnResource: "folder", LinkedByRelation: "parent",
		}},
		When: &models.DerivationSettings{NoDirectRolesOnObject: true},
	}
	create := func(role models.ResourceRoleCreate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.ResourceRoles.Create(t.Context(), "document", role)
			return err
		}
	}
	update := func(change models.ResourceRoleUpdate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.ResourceRoles.Update(t.Context(), "document", "editor", change)
			return err
		}
	}
	permissions := func(remove bool, given []string) func(c *Client) error {
		return func(c *Client) error {
			call := c.ResourceRoles.AssignPermissions
			if remove {
				call = c.ResourceRoles.RemovePermissions
			}
			_, err := call(t.Context(), "document", "editor", given)
			return err
		}
	}
	for _, tc := range []struct {
		golden string
		route  string
		call   func(c *Client) error
	}{
		{
			"create_resource_role.required.golden", createResourceRoleRoute,
			create(models.ResourceRoleCreate{Key: "r", Name: "R"}),
		},
		{
			"create_resource_role.empty-lists.golden", createResourceRoleRoute,
			create(models.ResourceRoleCreate{
				Key: "r", Name: "R", Permissions: []string{}, Attributes: models.Attributes{},
				Extends: []string{},
			}),
		},
		{
			"create_resource_role.every-field.golden", createResourceRoleRoute,
			create(models.ResourceRoleCreate{
				Key: "r", Name: "R", Description: description, Permissions: []string{"read"},
				Attributes: models.Attributes{"seats": 10, "quota": map[string]any{"max": 2.5}},
				Extends:    []string{"viewer"}, GrantedTo: &derived,
			}),
		},
		{
			"update_resource_role.unset.golden", updateResourceRoleRoute,
			update(models.ResourceRoleUpdate{}),
		},
		{
			"update_resource_role.empty-lists.golden", updateResourceRoleRoute,
			update(models.ResourceRoleUpdate{Permissions: []string{}, Extends: []string{}}),
		},
		{
			"update_resource_role.null.golden", updateResourceRoleRoute,
			update(models.ResourceRoleUpdate{
				Description: models.Null[string](), Attributes: models.Null[models.Attributes](),
				GrantedTo: models.Null[models.DerivedRoleBlockEdit](),
			}),
		},
		{
			"update_resource_role.values.golden", updateResourceRoleRoute,
			update(models.ResourceRoleUpdate{
				Name: "R", Description: models.Some(description), Permissions: []string{"read"},
				Attributes: models.Some(models.Attributes{}), Extends: []string{"viewer"},
				GrantedTo: models.Some(derived),
			}),
		},
		{
			"assign_permissions_to_resource_role.values.golden",
			assignResourceRolePermissionsRoute,
			permissions(false, []string{"document:read", "folder:list"}),
		},
		{
			"assign_permissions_to_resource_role.nil.golden", assignResourceRolePermissionsRoute,
			permissions(false, nil),
		},
		{
			"remove_permissions_from_resource_role.values.golden",
			removeResourceRolePermissionsRoute, permissions(true, []string{"document:write"}),
		},
		{
			"remove_permissions_from_resource_role.nil.golden",
			removeResourceRolePermissionsRoute, permissions(true, nil),
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: resourceRoleBody})
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

// TestResourceRoleBodiesThatCannotBeJSON checks that a create and an update whose attributes hold
// a value that JSON cannot hold, and an update whose attributes are a nil map of Some, which
// would send null for a value, are errors that match ErrInvalidConfig and name the operation, and
// send no request.
func TestResourceRoleBodiesThatCannotBeJSON(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := resourceClient(t, api)
	_, createErr := c.ResourceRoles.Create(t.Context(), "document", models.ResourceRoleCreate{
		Key: "r", Name: "R", Attributes: models.Attributes{"c": make(chan int)},
	})
	_, updateErr := c.ResourceRoles.Update(t.Context(), "document", "r",
		models.ResourceRoleUpdate{Attributes: models.Some(models.Attributes{"n": math.NaN()})})
	_, nilErr := c.ResourceRoles.Update(t.Context(), "document", "r",
		models.ResourceRoleUpdate{Attributes: models.Some[models.Attributes](nil)})
	for _, tc := range []struct {
		op  string
		err error
	}{
		{opCreateResourceRole, createErr},
		{opUpdateResourceRole, updateErr},
		{opUpdateResourceRole, nilErr},
	} {
		if !errors.Is(tc.err, ErrInvalidConfig) || !strings.Contains(tc.err.Error(), tc.op) {
			t.Errorf("%s returned %v, want an error that matches ErrInvalidConfig and names it",
				tc.op, tc.err)
		}
	}
	api.AssertUntouched()
}

// TestResourceRoleRetries checks when a call of the roles of a resource sends its request again:
// Create, which the API may have acted on, after a 429 only, so a 503 ends it after one request;
// each other call, which does the same when sent twice, after a 503 too.
func TestResourceRoleRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	for _, m := range resourceRoleCalls {
		replies := [][]mockpermit.Reply{{unavailable, m.reply}}
		statuses := []int{0}
		if m.op == opCreateResourceRole {
			replies = [][]mockpermit.Reply{{limited, m.reply}, {unavailable}}
			statuses = []int{0, http.StatusServiceUnavailable}
		}
		for i, r := range replies {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, r...)
			c := resourceClient(t, api, WithRetry(RetryPolicy{MaxRetries: 1}))
			_, err := m.call(t.Context(), c, "document", "editor")
			if statuses[i] == 0 && err != nil {
				t.Errorf("%s after %d returned %v, want the result of the second request",
					m.name, r[0].Status, err)
			}
			if statuses[i] != 0 && mustError(t, err).StatusCode != statuses[i] {
				t.Errorf("%s returned %v, want the error of status %d", m.name, err, statuses[i])
			}
			if n := len(api.Requests()); n != len(r) {
				t.Errorf("%s after %d sent %d requests, want %d", m.name, r[0].Status, n, len(r))
			}
		}
	}
}

// TestMalformedResourceRoleReplies checks that a reply in 200-299 that is not a role, to each
// method that returns one, or for List and All not a JSON array of roles, is an *Error that names
// the operation, its route and the model, and no role, no page and no role of the iterator.
// Attributes that are not a JSON object, such as a string, are not those of a role either.
func TestMalformedResourceRoleReplies(t *testing.T) {
	bodies := map[string][]string{
		"models.ResourceRoleRead": {``, `null`, `[]`, `"a"`, `{"attributes":"a"}`, `{} {}`},
		"list of models.ResourceRoleRead": {
			``, `null`, `"a"`, `[1]`, `[{"attributes":[]}]`, `{"data":[]}`,
		},
	}
	for _, m := range resourceRoleCalls {
		model := "models.ResourceRoleRead"
		switch m.op {
		case opDeleteResourceRole:
			continue
		case opListResourceRoles:
			model = "list of models.ResourceRoleRead"
		}
		want := "permit: " + m.op + ": " + m.route + ": status 200: the reply is not a valid " +
			model
		for _, body := range bodies[model] {
			replies := []mockpermit.Reply{{Body: body}}
			if m.op == opListResourceRoles {
				replies = append(replies, mockpermit.Reply{Body: body})
			}
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, replies...)
			c := resourceClient(t, api)
			roles, err := m.call(t.Context(), c, "document", "editor")
			if e := mustError(t, err); roles != nil || e.Error() != want {
				t.Errorf("%s of the reply %q returned %+v and\n%v\nwant no roles and\n%s",
					m.name, body, roles, err, want)
			}
			if m.op != opListResourceRoles {
				continue
			}
			items, errs := collect(t, c.ResourceRoles.All(t.Context(), "document", nil))
			if items != nil || len(errs) != 1 || mustError(t, errs[0]).Error() != want {
				t.Errorf("All of the reply %q yielded %+v and %v, want only the error\n%s", body,
					items, errs, want)
			}
		}
	}
}

// TestResourceRolesAfterClose checks that each method returns ErrClosed after Close, and sends
// nothing, also without WithScope, whose lookup it does not send either.
func TestResourceRolesAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	_, errs := collect(t, c.ResourceRoles.All(t.Context(), "document", nil))
	if len(errs) != 1 {
		t.Errorf("All after Close yielded %v, want ErrClosed once", errs)
	}
	for _, m := range resourceRoleCalls {
		_, err := m.call(t.Context(), c, "document", "editor")
		errs = append(errs, err)
	}
	for i, err := range errs {
		if !errors.Is(err, ErrClosed) {
			t.Errorf("call %d after Close returned %v, want ErrClosed", i, err)
		}
	}
	api.AssertUntouched()
}
