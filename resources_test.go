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

// The routes of the resources, as mockpermit.Server.Handle takes them.
const (
	listResourcesRoute   = "GET /v2/schema/{proj_id}/{env_id}/resources"
	createResourceRoute  = "POST /v2/schema/{proj_id}/{env_id}/resources"
	getResourceRoute     = "GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}"
	replaceResourceRoute = "PUT /v2/schema/{proj_id}/{env_id}/resources/{resource_id}"
	updateResourceRoute  = "PATCH /v2/schema/{proj_id}/{env_id}/resources/{resource_id}"
	deleteResourceRoute  = "DELETE /v2/schema/{proj_id}/{env_id}/resources/{resource_id}"
)

// resourceBody is the body of a reply of the API that holds the resource document.
const resourceBody = `{"key":"document","id":"00000000-0000-4000-8000-000000000021",` +
	`"organization_id":"00000000-0000-4000-8000-000000000001",` +
	`"project_id":"00000000-0000-4000-8000-000000000002",` +
	`"environment_id":"00000000-0000-4000-8000-000000000003",` +
	`"created_at":"2026-10-14T09:30:00Z","updated_at":"2026-10-14T09:30:00Z","name":"Document"}`

// resourcesOf returns the resources r<from> to r<to-1>, with only a key and a name: an empty list
// when to is from.
func resourcesOf(from, to int) []models.ResourceRead {
	items := make([]models.ResourceRead, 0, to-from)
	for i := from; i < to; i++ {
		key := "r" + strconv.Itoa(i)
		items = append(items, models.ResourceRead{Key: key, Name: key})
	}
	return items
}

// resourceClient returns a client of api with the scope of the project p and the environment e,
// and opts, whose PDP is a server that fails the test when anything reaches it.
func resourceClient(t *testing.T, api *mockpermit.Server, opts ...Option) *Client {
	t.Helper()
	return projectClient(t, api, append([]Option{WithScope("p", "e")}, opts...)...)
}

// resourceCalls are calls of each method of the resources that acts on one resource, with the
// ID of its operation, its route and a reply of the API to it.
var resourceCalls = []struct {
	name  string
	op    string
	route string
	reply mockpermit.Reply
	call  func(ctx context.Context, c *Client, key string) error
}{
	{
		name: "Get", op: opGetResource, route: getResourceRoute,
		reply: mockpermit.Reply{Body: resourceBody},
		call: func(ctx context.Context, c *Client, key string) error {
			_, err := c.Resources.Get(ctx, key)
			return err
		},
	},
	{
		name: "Replace", op: opReplaceResource, route: replaceResourceRoute,
		reply: mockpermit.Reply{Body: resourceBody},
		call: func(ctx context.Context, c *Client, key string) error {
			_, err := c.Resources.Replace(ctx, key, models.ResourceReplace{Name: "n"})
			return err
		},
	},
	{
		name: "Update", op: opUpdateResource, route: updateResourceRoute,
		reply: mockpermit.Reply{Body: resourceBody},
		call: func(ctx context.Context, c *Client, key string) error {
			_, err := c.Resources.Update(ctx, key, models.ResourceUpdate{Name: "n"})
			return err
		},
	},
	{
		name: "Delete", op: opDeleteResource, route: deleteResourceRoute,
		reply: mockpermit.Reply{Status: http.StatusNoContent},
		call: func(ctx context.Context, c *Client, key string) error {
			return c.Resources.Delete(ctx, key)
		},
	},
}

// TestResourcesAllReadsEveryPage checks that ResourcesService.All reads the 31 resources of a
// list, in order, asking for each page in turn with the search of the options: at a per_page of
// 30, two pages of resources and the empty page after them; at the default per_page of 100, the
// most that the API gives on a page, one page of 31 and the empty page after it; and up to the
// last page that paginated replies count, without the empty page after it.
func TestResourcesAllReadsEveryPage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    *ResourcesListOptions
		pages   [][]models.ResourceRead
		counts  string
		queries []string
	}{
		{
			name: "per_page 30", opts: &ResourcesListOptions{ListOptions: ListOptions{PerPage: 30}},
			pages: [][]models.ResourceRead{resourcesOf(0, 30), resourcesOf(30, 31), {}},
			queries: []string{
				"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30",
			},
		},
		{
			name: "the default per_page and a search", opts: &ResourcesListOptions{Search: "r"},
			pages:   [][]models.ResourceRead{resourcesOf(0, 31), {}},
			queries: []string{"page=1&per_page=100&search=r", "page=2&per_page=100&search=r"},
		},
		{
			name:    "paginated replies",
			opts:    &ResourcesListOptions{ListOptions: ListOptions{PerPage: 30}, Search: "a b"},
			pages:   [][]models.ResourceRead{resourcesOf(0, 30), resourcesOf(30, 31)},
			counts:  `,"total_count":31,"page_count":2`,
			queries: []string{"page=1&per_page=30&search=a+b", "page=2&per_page=30&search=a+b"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			var replies []mockpermit.Reply
			for _, items := range tc.pages {
				replies = append(replies, resourcesReply(t, items, tc.counts))
			}
			api.Handle(listResourcesRoute, replies...)
			items, errs := collect(t, resourceClient(t, api).Resources.All(t.Context(), tc.opts))
			if diff := cmp.Diff(resourcesOf(0, 31), items); diff != "" || errs != nil {
				t.Errorf("All yielded the errors %v and resources that differ (-want +got):\n%s",
					errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// resourcesReply returns a reply that holds items: a JSON array without counts, and the
// paginated object with the members of counts, such as `,"total_count":2`, with them.
func resourcesReply(t *testing.T, items []models.ResourceRead, counts string) mockpermit.Reply {
	t.Helper()
	if counts == "" {
		return arrayOf(t, items)
	}
	return pageOf(t, items, counts)
}

// TestResourcesAllYieldsAnErrorOnce checks that a page that fails ends the iterator with its
// error, once, after the resources of the pages before it, and that the options of All are
// read when it is called: a later change to them changes no request.
func TestResourcesAllYieldsAnErrorOnce(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(listResourcesRoute, arrayOf(t, resourcesOf(0, 30)),
		mockpermit.Reply{Status: http.StatusForbidden, Body: body})
	opts := &ResourcesListOptions{ListOptions: ListOptions{PerPage: 30}, Search: "doc"}
	seq := resourceClient(t, api).Resources.All(t.Context(), opts)
	opts.PerPage, opts.Search = 50, "changed"
	items, errs := collect(t, seq)
	if len(items) != 30 || len(errs) != 1 || !errors.Is(errs[0], ErrForbidden) {
		t.Errorf("All yielded %d resources and the errors %v, want 30 and one error that "+
			"matches ErrForbidden", len(items), errs)
	}
	want := []string{"page=1&per_page=30&search=doc", "page=2&per_page=30&search=doc"}
	if diff := cmp.Diff(want, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestResourcesAllGetsTheScopeInEachLoop checks that All, without WithScope, looks up the scope
// in each loop over the iterator, before its first page, and not when it is called: a lookup that
// fails ends that loop with its error, and the next loop looks the scope up again.
func TestResourcesAllGetsTheScopeInEachLoop(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(scopeRoute, mockpermit.Reply{Status: http.StatusForbidden, Body: body},
		scopeReply(t, ""))
	api.Handle(listResourcesRoute, arrayOf(t, resourcesOf(0, 1)), arrayOf(t, resourcesOf(1, 1)))
	seq := projectClient(t, api).Resources.All(t.Context(), nil)
	if requests := api.Requests(); len(requests) != 0 {
		t.Fatalf("All sent %+v before a loop, want no request", requests)
	}
	if items, errs := collect(t, seq); items != nil || len(errs) != 1 ||
		!errors.Is(errs[0], ErrForbidden) {
		t.Errorf("the first loop yielded %v and the errors %v, want one error that matches "+
			"ErrForbidden", items, errs)
	}
	if items, errs := collect(t, seq); !cmp.Equal(resourcesOf(0, 1), items) || errs != nil {
		t.Errorf("the second loop yielded %v and the errors %v, want r0", items, errs)
	}
	var paths []string
	for _, r := range api.Requests() {
		paths = append(paths, r.RawPath)
	}
	resources := "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID + "/resources"
	want := []string{"/v2/api-key/scope", "/v2/api-key/scope", resources, resources}
	if diff := cmp.Diff(want, paths); diff != "" {
		t.Errorf("the requests differ (-want +got):\n%s", diff)
	}
}

// TestResourcesListReturnsAPage checks the page that List returns: the resources of the reply in
// its order, the page and the per_page that it asked for, page 1 and no query for nil options,
// the search of the options in the query, escaped, and the counts of a paginated reply. A page
// without resources has an empty list, not nil.
func TestResourcesListReturnsAPage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		opts   *ResourcesListOptions
		items  []models.ResourceRead
		counts string
		query  string
		want   *Page[models.ResourceRead]
	}{
		{
			name: "nil options", items: resourcesOf(0, 2), query: "",
			want: &Page[models.ResourceRead]{Data: resourcesOf(0, 2), Page: 1},
		},
		{
			name: "a search that the query escapes", opts: &ResourcesListOptions{Search: "a b&c=é"},
			items: resourcesOf(0, 1), query: "search=a+b%26c%3D%C3%A9",
			want: &Page[models.ResourceRead]{Data: resourcesOf(0, 1), Page: 1},
		},
		{
			name:  "a paginated reply after the last page",
			opts:  &ResourcesListOptions{ListOptions: ListOptions{Page: 3, PerPage: 30}},
			items: []models.ResourceRead{}, counts: `,"total_count":31,"page_count":2`,
			query: "page=3&per_page=30",
			want: &Page[models.ResourceRead]{
				Data: []models.ResourceRead{}, Page: 3, PerPage: 30, TotalCount: new(31),
				PageCount: new(2),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listResourcesRoute, resourcesReply(t, tc.items, tc.counts))
			got, err := resourceClient(t, api).Resources.List(t.Context(), tc.opts)
			if err != nil {
				t.Fatalf("List returned %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" || got.Data == nil {
				t.Errorf("List returned a page that differs (-want +got), or nil Data:\n%s", diff)
			}
			if requests := api.Requests(); len(requests) != 1 ||
				requests[0].RawPath != "/v2/schema/p/e/resources" ||
				requests[0].RawQuery != tc.query {
				t.Errorf("List sent %+v, want one request to /v2/schema/p/e/resources?%s",
					requests, tc.query)
			}
		})
	}
}

// TestInvalidResourceListOptions checks that a Page or a PerPage less than zero is an error that
// matches ErrInvalidConfig from List and All, and that neither sends a request.
func TestInvalidResourceListOptions(t *testing.T) {
	for _, opts := range []ListOptions{{Page: -1}, {PerPage: -1}} {
		api := mockpermit.NewAPI(t)
		c := resourceClient(t, api)
		filter := &ResourcesListOptions{ListOptions: opts, Search: "s"}
		_, err := c.Resources.List(t.Context(), filter)
		_, errs := collect(t, c.Resources.All(t.Context(), filter))
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

// TestResourcesUseTheScope checks that the calls of the resources act on the environment of
// the client's scope. Without WithScope, the client looks up the scope of its API key once, and
// each call fills the route with the project and the environment of an environment-level key.
// A scope without an environment, that of a project-level or an organization-level key, is an
// error that matches ErrInvalidConfig and names the operation, and no call sends a request but
// the lookup, which the API fails for any route that the test did not register.
func TestResourcesUseTheScope(t *testing.T) {
	const prefix = "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID + "/resources"
	calls := map[string]func(ctx context.Context, c *Client) error{
		opListResources: func(ctx context.Context, c *Client) error {
			_, err := c.Resources.List(ctx, nil)
			return err
		},
		opCreateResource: func(ctx context.Context, c *Client) error {
			_, err := c.Resources.Create(ctx, models.ResourceCreate{Key: "k", Name: "n"})
			return err
		},
	}
	for _, m := range resourceCalls {
		calls[m.op] = func(ctx context.Context, c *Client) error {
			return m.call(ctx, c, "document")
		}
	}
	t.Run("an environment-level key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		api.Handle(listResourcesRoute, mockpermit.Reply{Body: `[]`})
		api.Handle(createResourceRoute, mockpermit.Reply{Body: resourceBody})
		for _, m := range resourceCalls {
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
			if r.RawPath != prefix && r.RawPath != prefix+"/document" {
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
			if _, errs := collect(t, c.Resources.All(t.Context(), nil)); len(errs) != 1 ||
				!errors.Is(errs[0], ErrInvalidConfig) {
				t.Errorf("All yielded %v, want one error that matches ErrInvalidConfig", errs)
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("the calls sent %d requests, want only the lookup of the scope", n)
			}
		})
	}
}

// TestResourceKeysInThePath checks that each method that names a resource sends its key as one
// segment of the path, escaped, after the project and the environment of the scope, whatever the
// key holds: a slash, a percent sign, a space, a question mark and a hash, a letter outside
// ASCII, the shape of a UUID and a leading dot. A key that is empty, . or .. would change the
// route, so it is an error that matches ErrInvalidConfig and names the placeholder, and no
// request.
func TestResourceKeysInThePath(t *testing.T) {
	const prefix = "/v2/schema/p/e/resources/"
	for key, want := range map[string]string{
		"a/b":            prefix + "a%2Fb",
		"50%":            prefix + "50%25",
		"a b?c#d":        prefix + "a%20b%3Fc%23d",
		"é":              prefix + "%C3%A9",
		fixtures.UUIDKey: prefix + fixtures.UUIDKey,
		".hidden":        prefix + ".hidden",
		"doc.v2..x":      prefix + "doc.v2..x",
	} {
		for _, m := range resourceCalls {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, m.reply)
			if err := m.call(t.Context(), resourceClient(t, api), key); err != nil {
				t.Errorf("%s(%q) returned %v", m.name, key, err)
			}
			if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want {
				t.Errorf("%s(%q) sent %+v, want one request to %s", m.name, key, requests, want)
			}
		}
	}
	for _, key := range []string{"", ".", ".."} {
		for _, m := range resourceCalls {
			api := mockpermit.NewAPI(t)
			err := m.call(t.Context(), resourceClient(t, api), key)
			named := err != nil && strings.Contains(err.Error(), "{resource_id}")
			if !errors.Is(err, ErrInvalidConfig) || !named {
				t.Errorf("%s(%q) returned %v, want an error that matches ErrInvalidConfig and "+
					"names {resource_id}", m.name, key, err)
			}
			api.AssertUntouched()
		}
	}
}

// TestDeleteResourceThatIsGone checks that Delete returns nil for a 404, with or without a body,
// as the resource that the path names is gone already, after one request; and an *Error with the
// status for any other error status, such as 403, 409 or 422.
func TestDeleteResourceThatIsGone(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"NOT_FOUND","title":"Not Found","message":"the resource does not exist"}`,
		"",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteResourceRoute, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		if err := resourceClient(t, api).Resources.Delete(t.Context(), "gone"); err != nil {
			t.Errorf("Delete of a resource that the API answers with 404 and the body %q "+
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
		api.Handle(deleteResourceRoute, mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := resourceClient(t, api).Resources.Delete(t.Context(), "document")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteResource ||
			e.Detail != "no" || errors.Is(err, ErrNotFound) {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status and the detail", status, err, opDeleteResource)
		}
	}
}

// TestNotFoundOfOtherResourceCalls checks that a 404 to Get, Replace and Update, which name a
// resource but delete nothing, is an *Error that matches ErrNotFound.
func TestNotFoundOfOtherResourceCalls(t *testing.T) {
	const body = `{"error_code":"NOT_FOUND","title":"Not Found","message":"no resource"}`
	for _, m := range resourceCalls[:3] {
		api := mockpermit.NewAPI(t)
		api.Handle(m.route, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		err := m.call(t.Context(), resourceClient(t, api), "gone")
		e := mustError(t, err)
		if !errors.Is(err, ErrNotFound) || e.StatusCode != http.StatusNotFound || e.Op != m.op {
			t.Errorf("%s of a resource that the API answers with 404 returned %v, want an "+
				"*Error that matches ErrNotFound", m.name, err)
		}
	}
}

// everyResourceField returns the maps of a resource with an entry each, every field of each entry
// set, and values that an encoder may change: 2^53+1, a line separator and <, > and &.
func everyResourceField() (map[string]models.ActionBlockEditable,
	map[string]models.AttributeBlockEditable, map[string]models.RoleBlockEditable,
) {
	actions := map[string]models.ActionBlockEditable{"read": {
		Name: "<R> & S", Description: "a" + fixtures.LineSeparator + "b",
		Attributes: models.Attributes{"n": fixtures.TwoPow53Plus1},
	}}
	attributes := map[string]models.AttributeBlockEditable{
		"owner": {Type: models.AttributeTypeString, Description: "d"},
		"later": {Type: "added_later"},
	}
	roles := map[string]models.RoleBlockEditable{"editor": {
		Name: "E", Description: "d", Permissions: []string{"read"}, Attributes: models.Attributes{},
		Extends: []string{},
		GrantedTo: &models.DerivedRoleBlockEdit{
			UsersWithRole: []models.DerivedRoleRuleCreate{{
				Role: "admin", OnResource: "folder", LinkedByRelation: "parent",
				When: &models.DerivationSettings{NoDirectRolesOnObject: true},
			}},
			When: &models.DerivationSettings{},
		},
	}}
	return actions, attributes, roles
}

// TestResourceBodies checks the bodies of Create, Replace and Update byte for byte against the
// golden files in testdata/golden: a create and a replace with the required fields only, which
// always send the actions, {} for an empty map and null for a nil one, and with every field; the
// zero update, which sends {}; an update that sends null for the description; one with each map
// nil, which sends none; one with each map empty, which sends {} for each; one with a partial map
// of actions, which sends that map as it is; and one that sets every field.
func TestResourceBodies(t *testing.T) {
	actions, attributes, roles := everyResourceField()
	create := func(resource models.ResourceCreate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.Resources.Create(t.Context(), resource)
			return err
		}
	}
	replace := func(resource models.ResourceReplace) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.Resources.Replace(t.Context(), "r", resource)
			return err
		}
	}
	update := func(change models.ResourceUpdate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.Resources.Update(t.Context(), "r", change)
			return err
		}
	}
	empty := map[string]models.ActionBlockEditable{}
	for _, tc := range []struct {
		golden string
		route  string
		call   func(c *Client) error
	}{
		{
			"create_resource.required.golden", createResourceRoute,
			create(models.ResourceCreate{Key: "r", Name: "R", Actions: empty}),
		},
		{
			"create_resource.nil-actions.golden", createResourceRoute,
			create(models.ResourceCreate{Key: "r", Name: "R"}),
		},
		{
			"create_resource.every-field.golden", createResourceRoute,
			create(models.ResourceCreate{
				Key: "r", Name: "R", URN: "prn:p:e:r", Description: "d", Actions: actions,
				TypeAttributes: models.Attributes{"n": fixtures.TwoPow53Plus1},
				Attributes:     attributes, Roles: roles,
				Relations: map[string]string{"parent": "folder"},
			}),
		},
		{
			"replace_resource.required.golden", replaceResourceRoute,
			replace(models.ResourceReplace{Name: "R", Actions: empty}),
		},
		{
			"replace_resource.every-field.golden", replaceResourceRoute,
			replace(models.ResourceReplace{
				Name: "R", URN: "prn:p:e:r", Description: "d", Actions: actions,
				TypeAttributes: models.Attributes{}, Attributes: attributes, Roles: roles,
				Relations: map[string]string{"parent": "folder"},
			}),
		},
		{"update_resource.unset.golden", updateResourceRoute, update(models.ResourceUpdate{})},
		{
			"update_resource.null.golden", updateResourceRoute,
			update(models.ResourceUpdate{Description: models.Null[string]()}),
		},
		{
			"update_resource.empty-maps.golden", updateResourceRoute,
			update(models.ResourceUpdate{
				Actions: empty, TypeAttributes: models.Attributes{},
				Attributes: map[string]models.AttributeBlockEditable{},
				Roles:      map[string]models.RoleBlockEditable{}, Relations: map[string]string{},
			}),
		},
		{
			"update_resource.partial-map.golden", updateResourceRoute,
			update(models.ResourceUpdate{
				Actions: map[string]models.ActionBlockEditable{"read": {}},
			}),
		},
		{
			"update_resource.values.golden", updateResourceRoute,
			update(models.ResourceUpdate{
				Name: "R", URN: "prn:p:e:r", Description: models.Some(""), Actions: actions,
				TypeAttributes: models.Attributes{"n": 1}, Attributes: attributes, Roles: roles,
				Relations: map[string]string{"parent": "folder"},
			}),
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: resourceBody})
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

// TestResourceBodiesThatCannotBeJSON checks that a create, a replace and an update that hold a
// value that JSON cannot hold, in the type attributes of the resource or in the attributes of a
// role, are errors that match ErrInvalidConfig and name the operation, and send no request.
func TestResourceBodiesThatCannotBeJSON(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := resourceClient(t, api)
	_, createErr := c.Resources.Create(t.Context(), models.ResourceCreate{
		Key: "r", Name: "R", TypeAttributes: models.Attributes{"c": make(chan int)},
	})
	_, replaceErr := c.Resources.Replace(t.Context(), "r", models.ResourceReplace{
		Name: "R", TypeAttributes: models.Attributes{"n": math.NaN()},
	})
	_, updateErr := c.Resources.Update(t.Context(), "r", models.ResourceUpdate{
		Roles: map[string]models.RoleBlockEditable{"r": {
			Name: "R", Attributes: models.Attributes{"f": func() {}},
		}},
	})
	for op, err := range map[string]error{
		opCreateResource: createErr, opReplaceResource: replaceErr, opUpdateResource: updateErr,
	} {
		if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), op) {
			t.Errorf("%s returned %v, want an error that matches ErrInvalidConfig and names it",
				op, err)
		}
	}
	api.AssertUntouched()
}

// TestResourceRetries checks when a call of the resources sends its request again: Create, which
// the API may have acted on, after a 429 only, so a 503 ends it after one request; Replace,
// Update, Get, Delete and List, which do the same when sent twice, after a 503 too.
func TestResourceRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	ok := mockpermit.Reply{Body: resourceBody}
	createResource := func(c *Client) error {
		_, err := c.Resources.Create(t.Context(), models.ResourceCreate{Key: "k", Name: "n"})
		return err
	}
	tests := []struct {
		name    string
		route   string
		replies []mockpermit.Reply
		call    func(c *Client) error
		status  int
	}{
		{
			name: "Create after a 429", route: createResourceRoute,
			replies: []mockpermit.Reply{limited, ok}, call: createResource,
		},
		{
			name: "Create after a 503", route: createResourceRoute,
			replies: []mockpermit.Reply{unavailable}, call: createResource,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "List after a 503", route: listResourcesRoute,
			replies: []mockpermit.Reply{unavailable, {Body: `[]`}},
			call: func(c *Client) error {
				_, err := c.Resources.List(t.Context(), nil)
				return err
			},
		},
	}
	for _, m := range resourceCalls {
		tests = append(tests, struct {
			name    string
			route   string
			replies []mockpermit.Reply
			call    func(c *Client) error
			status  int
		}{
			name: m.name + " after a 503", route: m.route,
			replies: []mockpermit.Reply{unavailable, m.reply},
			call:    func(c *Client) error { return m.call(t.Context(), c, "r") },
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

// TestMalformedResourceReplies checks that a reply in 200-299 that is not a resource, or for List
// neither a JSON array of resources nor the paginated object of them, is an *Error that names
// the model, and no resource.
func TestMalformedResourceReplies(t *testing.T) {
	const prefix = "permit: "
	for _, body := range []string{``, `null`, `[]`, `"r"`, `{"key":1}`, `{} {}`} {
		api := mockpermit.NewAPI(t)
		api.Handle(getResourceRoute, mockpermit.Reply{Body: body})
		resource, err := resourceClient(t, api).Resources.Get(t.Context(), "r")
		want := prefix + "get_resource: GET /v2/schema/{proj_id}/{env_id}/resources/" +
			"{resource_id}: status 200: the reply is not a valid models.ResourceRead"
		if e := mustError(t, err); resource != nil || e.Error() != want {
			t.Errorf("Get of the reply %q returned %+v and\n%v\nwant no resource and\n%s", body,
				resource, err, want)
		}
	}
	for _, body := range []string{``, `null`, `"r"`, `[1]`, `[{"key":1}]`, `{"data":[{"key":1}]}`} {
		api := mockpermit.NewAPI(t)
		api.Handle(listResourcesRoute, mockpermit.Reply{Body: body})
		page, err := resourceClient(t, api).Resources.List(t.Context(), nil)
		want := prefix + "list_resources: GET /v2/schema/{proj_id}/{env_id}/resources: status " +
			"200: the reply is not a valid list of models.ResourceRead"
		if e := mustError(t, err); page != nil || e.Error() != want {
			t.Errorf("List of the reply %q returned %+v and\n%v\nwant no page and\n%s", body,
				page, err, want)
		}
	}
}

// TestResourcesAfterClose checks that each method returns ErrClosed after Close, and sends
// nothing, also without WithScope, whose lookup it does not send either.
func TestResourcesAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	_, listErr := c.Resources.List(ctx, nil)
	_, allErrs := collect(t, c.Resources.All(ctx, nil))
	_, createErr := c.Resources.Create(ctx, models.ResourceCreate{Key: "k", Name: "n"})
	errs := []error{listErr, createErr}
	for _, m := range resourceCalls {
		errs = append(errs, m.call(t.Context(), c, "r"))
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
