package permit

import (
	"context"
	"errors"
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

// The routes of the attributes of a resource, as mockpermit.Server.Handle takes them.
const (
	listResourceAttributesRoute = "GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/" +
		"attributes"
	createResourceAttributeRoute = "POST /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/" +
		"attributes"
	getResourceAttributeRoute = "GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/" +
		"attributes/{attribute_id}"
	updateResourceAttributeRoute = "PATCH /v2/schema/{proj_id}/{env_id}/resources/" +
		"{resource_id}/attributes/{attribute_id}"
	deleteResourceAttributeRoute = "DELETE /v2/schema/{proj_id}/{env_id}/resources/" +
		"{resource_id}/attributes/{attribute_id}"
)

// resourceAttributeBody is the body of a reply of the API that holds the attribute owner of the
// resource document.
const resourceAttributeBody = `{"type":"string","key":"owner",` +
	`"id":"00000000-0000-4000-8000-000000000093",` +
	`"resource_id":"00000000-0000-4000-8000-000000000021","resource_key":"document",` +
	`"organization_id":"00000000-0000-4000-8000-000000000001",` +
	`"project_id":"00000000-0000-4000-8000-000000000002",` +
	`"environment_id":"00000000-0000-4000-8000-000000000003",` +
	`"created_at":"2026-10-14T09:30:00Z","updated_at":"2026-10-14T09:30:00Z","built_in":false}`

// resourceAttributesOf returns the attributes a<from> to a<to-1>, with only a key and the type
// string: an empty list when to is from.
func resourceAttributesOf(from, to int) []models.ResourceAttributeRead {
	items := make([]models.ResourceAttributeRead, 0, to-from)
	for i := from; i < to; i++ {
		items = append(items, models.ResourceAttributeRead{
			Key: "a" + strconv.Itoa(i), Type: models.AttributeTypeString,
		})
	}
	return items
}

// resourceAttributeCalls are calls of each method of the attributes of a resource that acts on
// one attribute, with the ID of its operation, its route and a reply of the API to it.
var resourceAttributeCalls = []struct {
	name  string
	op    string
	route string
	reply mockpermit.Reply
	call  func(ctx context.Context, c *Client, resourceKey, attributeKey string) error
}{
	{
		name: "Get", op: opGetResourceAttribute, route: getResourceAttributeRoute,
		reply: mockpermit.Reply{Body: resourceAttributeBody},
		call: func(ctx context.Context, c *Client, resourceKey, attributeKey string) error {
			_, err := c.ResourceAttributes.Get(ctx, resourceKey, attributeKey)
			return err
		},
	},
	{
		name: "Update", op: opUpdateResourceAttribute, route: updateResourceAttributeRoute,
		reply: mockpermit.Reply{Body: resourceAttributeBody},
		call: func(ctx context.Context, c *Client, resourceKey, attributeKey string) error {
			_, err := c.ResourceAttributes.Update(ctx, resourceKey, attributeKey,
				models.ResourceAttributeUpdate{Description: models.Some("d")})
			return err
		},
	},
	{
		name: "Delete", op: opDeleteResourceAttribute, route: deleteResourceAttributeRoute,
		reply: mockpermit.Reply{Status: http.StatusNoContent},
		call: func(ctx context.Context, c *Client, resourceKey, attributeKey string) error {
			return c.ResourceAttributes.Delete(ctx, resourceKey, attributeKey)
		},
	},
}

// TestResourceAttributeTypes checks that List, All and Get read an attribute of each type that
// the SDK knows, and one of a type that it does not know, geo_point, which they keep as it is,
// with Known reporting false for it alone: the reply fixtures of the list and of the get, and a
// list whose second page holds the attribute of the type that the SDK does not know.
func TestResourceAttributeTypes(t *testing.T) {
	known := map[models.AttributeType]bool{
		models.AttributeTypeBool: true, models.AttributeTypeNumber: true,
		models.AttributeTypeString: true, models.AttributeTypeTime: true,
		models.AttributeTypeArray: true, models.AttributeTypeJSON: true,
		models.AttributeTypeObject: true, models.AttributeTypeObjectArray: true,
	}
	listed := replyFixtureOf(t, "list_resource_attributes.200.json")
	unknown := replyFixtureOf(t, "list_resource_attributes.200.unknown-type.json")
	checkTypes := func(t *testing.T, call string, items []models.ResourceAttributeRead,
		want []models.AttributeType,
	) {
		t.Helper()
		var got []models.AttributeType
		for _, a := range items {
			got = append(got, a.Type)
			if a.Type.Known() != known[a.Type] {
				t.Errorf("%s: the type %q of %s reports Known() = %t, want %t", call, a.Type,
					a.Key, a.Type.Known(), known[a.Type])
			}
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("%s read types that differ (-want +got):\n%s", call, diff)
		}
	}
	every := []models.AttributeType{
		"bool", "number", "string", "time", "array", "json", "object", "object_array",
	}
	t.Run("List", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(listResourceAttributesRoute, mockpermit.Reply{Body: listed},
			mockpermit.Reply{Body: unknown})
		c := resourceClient(t, api)
		for _, tc := range []struct {
			page int
			want []models.AttributeType
		}{
			{1, every},
			{2, []models.AttributeType{"geo_point"}},
		} {
			page, err := c.ResourceAttributes.List(t.Context(), "document",
				&ListOptions{Page: tc.page})
			if err != nil {
				t.Fatalf("List of page %d returned %v", tc.page, err)
			}
			checkTypes(t, "List of page "+strconv.Itoa(tc.page), page.Data, tc.want)
		}
	})
	t.Run("All", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(listResourceAttributesRoute, mockpermit.Reply{Body: listed},
			mockpermit.Reply{Body: unknown}, mockpermit.Reply{Body: `[]`})
		items, errs := collect(t, resourceClient(t, api).ResourceAttributes.All(t.Context(),
			"document", nil))
		if errs != nil {
			t.Fatalf("All yielded %v", errs)
		}
		checkTypes(t, "All", items, append(every, "geo_point"))
	})
	t.Run("Get", func(t *testing.T) {
		for fixture, want := range map[string]models.AttributeType{
			"get_resource_attribute.200.json":              models.AttributeTypeString,
			"get_resource_attribute.200.unknown-type.json": "geo_point",
		} {
			api := mockpermit.NewAPI(t)
			api.Handle(getResourceAttributeRoute,
				mockpermit.Reply{Body: replyFixtureOf(t, fixture)})
			got, err := resourceClient(t, api).ResourceAttributes.Get(t.Context(), "document",
				"owner")
			if err != nil {
				t.Fatalf("Get of %s returned %v", fixture, err)
			}
			checkTypes(t, "Get of "+fixture, []models.ResourceAttributeRead{*got},
				[]models.AttributeType{want})
		}
	})
}

// TestResourceAttributesAllReadsEveryPage checks that ResourceAttributesService.All reads the 31
// attributes of a list, in order, asking for each page in turn: at a per_page of 30, two pages of
// attributes and the empty page after them; at the default per_page of 100, the most that the
// API gives on a page, one page of 31 and the empty page after it; and from page 2 on, when the
// options start there.
func TestResourceAttributesAllReadsEveryPage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    *ListOptions
		pages   [][]models.ResourceAttributeRead
		want    []models.ResourceAttributeRead
		queries []string
	}{
		{
			name: "per_page 30",
			opts: &ListOptions{PerPage: 30},
			pages: [][]models.ResourceAttributeRead{
				resourceAttributesOf(0, 30), resourceAttributesOf(30, 31), {},
			},
			want: resourceAttributesOf(0, 31),
			queries: []string{
				"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30",
			},
		},
		{
			name:    "the default per_page",
			pages:   [][]models.ResourceAttributeRead{resourceAttributesOf(0, 31), {}},
			want:    resourceAttributesOf(0, 31),
			queries: []string{"page=1&per_page=100", "page=2&per_page=100"},
		},
		{
			name:    "from page 2",
			opts:    &ListOptions{Page: 2, PerPage: 30},
			pages:   [][]models.ResourceAttributeRead{resourceAttributesOf(30, 31), {}},
			want:    resourceAttributesOf(30, 31),
			queries: []string{"page=2&per_page=30", "page=3&per_page=30"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			var replies []mockpermit.Reply
			for _, items := range tc.pages {
				replies = append(replies, arrayOf(t, items))
			}
			api.Handle(listResourceAttributesRoute, replies...)
			c := resourceClient(t, api)
			items, errs := collect(t, c.ResourceAttributes.All(t.Context(), "document", tc.opts))
			if diff := cmp.Diff(tc.want, items); diff != "" || errs != nil {
				t.Errorf("All yielded the errors %v and attributes that differ (-want +got):\n%s",
					errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestResourceAttributesAllYieldsAnErrorOnce checks that a page that fails ends the iterator with
// its error, once, after the attributes of the pages before it, and that the options of All are
// read when it is called: a later change to them changes no request.
func TestResourceAttributesAllYieldsAnErrorOnce(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(listResourceAttributesRoute, arrayOf(t, resourceAttributesOf(0, 30)),
		mockpermit.Reply{Status: http.StatusForbidden, Body: body})
	opts := &ListOptions{PerPage: 30}
	seq := resourceClient(t, api).ResourceAttributes.All(t.Context(), "document", opts)
	opts.PerPage, opts.Page = 50, 4
	items, errs := collect(t, seq)
	if len(items) != 30 || len(errs) != 1 || !errors.Is(errs[0], ErrForbidden) {
		t.Errorf("All yielded %d attributes and the errors %v, want 30 and one error that "+
			"matches ErrForbidden", len(items), errs)
	}
	want := []string{"page=1&per_page=30", "page=2&per_page=30"}
	if diff := cmp.Diff(want, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestResourceAttributesAllGetsTheScopeInEachLoop checks that All, without WithScope, looks up
// the scope in each loop over the iterator, before its first page, and not when it is called: a
// lookup that fails ends that loop with its error, and the next loop looks the scope up again.
func TestResourceAttributesAllGetsTheScopeInEachLoop(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(scopeRoute, mockpermit.Reply{Status: http.StatusForbidden, Body: body},
		scopeReply(t, ""))
	api.Handle(listResourceAttributesRoute, arrayOf(t, resourceAttributesOf(0, 1)),
		arrayOf(t, resourceAttributesOf(1, 1)))
	seq := projectClient(t, api).ResourceAttributes.All(t.Context(), "document", nil)
	if requests := api.Requests(); len(requests) != 0 {
		t.Fatalf("All sent %+v before a loop, want no request", requests)
	}
	if items, errs := collect(t, seq); items != nil || len(errs) != 1 ||
		!errors.Is(errs[0], ErrForbidden) {
		t.Errorf("the first loop yielded %v and the errors %v, want one error that matches "+
			"ErrForbidden", items, errs)
	}
	if items, errs := collect(t, seq); !cmp.Equal(resourceAttributesOf(0, 1), items) ||
		errs != nil {
		t.Errorf("the second loop yielded %v and the errors %v, want a0", items, errs)
	}
	var paths []string
	for _, r := range api.Requests() {
		paths = append(paths, r.RawPath)
	}
	attributes := "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID +
		"/resources/document/attributes"
	want := []string{"/v2/api-key/scope", "/v2/api-key/scope", attributes, attributes}
	if diff := cmp.Diff(want, paths); diff != "" {
		t.Errorf("the requests differ (-want +got):\n%s", diff)
	}
}

// TestResourceAttributesListReturnsAPage checks the page that List returns: the attributes of
// the reply in its order, the page and the per_page that it asked for, page 1 and no query for
// nil options and for the zero options, and no counts, as the reply is a JSON array. A page
// without attributes has an empty list, not nil.
func TestResourceAttributesListReturnsAPage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  *ListOptions
		items []models.ResourceAttributeRead
		query string
		want  *Page[models.ResourceAttributeRead]
	}{
		{
			name: "nil options", items: resourceAttributesOf(0, 2), query: "",
			want: &Page[models.ResourceAttributeRead]{Data: resourceAttributesOf(0, 2), Page: 1},
		},
		{
			name: "the zero options", opts: &ListOptions{}, items: resourceAttributesOf(0, 1),
			query: "",
			want:  &Page[models.ResourceAttributeRead]{Data: resourceAttributesOf(0, 1), Page: 1},
		},
		{
			name:  "a page after the last",
			opts:  &ListOptions{Page: 3, PerPage: 30},
			items: []models.ResourceAttributeRead{}, query: "page=3&per_page=30",
			want: &Page[models.ResourceAttributeRead]{
				Data: []models.ResourceAttributeRead{}, Page: 3, PerPage: 30,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listResourceAttributesRoute, arrayOf(t, tc.items))
			c := resourceClient(t, api)
			got, err := c.ResourceAttributes.List(t.Context(), "document", tc.opts)
			if err != nil {
				t.Fatalf("List returned %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" || got.Data == nil {
				t.Errorf("List returned a page that differs (-want +got), or nil Data:\n%s", diff)
			}
			const path = "/v2/schema/p/e/resources/document/attributes"
			if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != path ||
				requests[0].RawQuery != tc.query {
				t.Errorf("List sent %+v, want one request to %s?%s", requests, path, tc.query)
			}
		})
	}
}

// TestInvalidResourceAttributeListOptions checks that a Page or a PerPage less than zero is an
// error that matches ErrInvalidConfig from List and All, and that neither sends a request.
func TestInvalidResourceAttributeListOptions(t *testing.T) {
	for _, opts := range []ListOptions{{Page: -1}, {PerPage: -1}} {
		api := mockpermit.NewAPI(t)
		c := resourceClient(t, api)
		_, err := c.ResourceAttributes.List(t.Context(), "document", &opts)
		_, errs := collect(t, c.ResourceAttributes.All(t.Context(), "document", &opts))
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

// TestResourceAttributesUseTheScope checks that the calls of the attributes of a resource act on
// the environment of the client's scope. Without WithScope, the client looks up the scope of its
// API key once, and each call fills the route with the project and the environment of an
// environment-level key. A scope without an environment, that of a project-level or an
// organization-level key, is an error that matches ErrInvalidConfig and names the operation, and
// no call sends a request but the lookup, which the API fails for any route that the test did
// not register.
func TestResourceAttributesUseTheScope(t *testing.T) {
	const prefix = "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID +
		"/resources/document/attributes"
	calls := map[string]func(ctx context.Context, c *Client) error{
		opListResourceAttributes: func(ctx context.Context, c *Client) error {
			_, err := c.ResourceAttributes.List(ctx, "document", nil)
			return err
		},
		opCreateResourceAttribute: func(ctx context.Context, c *Client) error {
			_, err := c.ResourceAttributes.Create(ctx, "document",
				models.ResourceAttributeCreate{Key: "k", Type: models.AttributeTypeString})
			return err
		},
	}
	for _, m := range resourceAttributeCalls {
		calls[m.op] = func(ctx context.Context, c *Client) error {
			return m.call(ctx, c, "document", "owner")
		}
	}
	t.Run("an environment-level key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		api.Handle(listResourceAttributesRoute, mockpermit.Reply{Body: `[]`})
		api.Handle(createResourceAttributeRoute, mockpermit.Reply{Body: resourceAttributeBody})
		for _, m := range resourceAttributeCalls {
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
			if r.RawPath != prefix && r.RawPath != prefix+"/owner" {
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
			seq := c.ResourceAttributes.All(t.Context(), "document", nil)
			if _, errs := collect(t, seq); len(errs) != 1 || !errors.Is(errs[0], ErrInvalidConfig) {
				t.Errorf("All yielded %v, want one error that matches ErrInvalidConfig", errs)
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("the calls sent %d requests, want only the lookup of the scope", n)
			}
		})
	}
}

// TestResourceAttributeKeysInThePath checks that each method of the attributes of a resource
// sends the key of the resource, and the key of the attribute when it names one, each as one
// segment of the path, escaped, after the project and the environment of the scope, whatever the
// key holds: a slash, a percent sign, a space, a question mark and a hash, a letter outside
// ASCII, the shape of a UUID and a leading dot. A key that is empty, . or .. would change the
// route, so it is an error that matches ErrInvalidConfig and names the placeholder, and no
// request.
func TestResourceAttributeKeysInThePath(t *testing.T) {
	const prefix = "/v2/schema/p/e/resources/"
	listCalls := []struct {
		name  string
		route string
		reply mockpermit.Reply
		call  func(ctx context.Context, c *Client, resourceKey string) error
	}{
		{
			name: "List", route: listResourceAttributesRoute, reply: mockpermit.Reply{Body: `[]`},
			call: func(ctx context.Context, c *Client, resourceKey string) error {
				_, err := c.ResourceAttributes.List(ctx, resourceKey, nil)
				return err
			},
		},
		{
			name: "All", route: listResourceAttributesRoute, reply: mockpermit.Reply{Body: `[]`},
			call: func(ctx context.Context, c *Client, resourceKey string) error {
				_, errs := collect(t, c.ResourceAttributes.All(ctx, resourceKey, nil))
				return errors.Join(errs...)
			},
		},
		{
			name: "Create", route: createResourceAttributeRoute,
			reply: mockpermit.Reply{Body: resourceAttributeBody},
			call: func(ctx context.Context, c *Client, resourceKey string) error {
				_, err := c.ResourceAttributes.Create(ctx, resourceKey,
					models.ResourceAttributeCreate{Key: "k", Type: models.AttributeTypeString})
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
			want := prefix + escaped + "/attributes"
			if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want {
				t.Errorf("%s(%q) sent %+v, want one request to %s", m.name, key, requests, want)
			}
		}
		for _, m := range resourceAttributeCalls {
			for _, keys := range [][2]string{{key, "owner"}, {"document", key}} {
				api := mockpermit.NewAPI(t)
				api.Handle(m.route, m.reply)
				c := resourceClient(t, api)
				if err := m.call(t.Context(), c, keys[0], keys[1]); err != nil {
					t.Errorf("%s(%q, %q) returned %v", m.name, keys[0], keys[1], err)
				}
				want := prefix + "document/attributes/" + escaped
				if keys[1] == "owner" {
					want = prefix + escaped + "/attributes/owner"
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
		for _, m := range resourceAttributeCalls {
			for placeholder, keys := range map[string][2]string{
				"{resource_id}": {key, "owner"}, "{attribute_id}": {"document", key},
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

// TestDeleteResourceAttributeThatIsGone checks that Delete returns nil for a 404, with or
// without a body, as the attribute that the path names is gone already, after one request with
// no query, though the API's documentation lists page and per_page for the route; and an *Error
// with the status for any other error status, such as 403, 409 or 422.
func TestDeleteResourceAttributeThatIsGone(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"NOT_FOUND","title":"Not Found","message":"the attribute does not exist"}`,
		"",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteResourceAttributeRoute,
			mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		err := resourceClient(t, api).ResourceAttributes.Delete(t.Context(), "document", "gone")
		if err != nil {
			t.Errorf("Delete of an attribute that the API answers with 404 and the body %q "+
				"returned %v, want nil", body, err)
		}
		if requests := api.Requests(); len(requests) != 1 || requests[0].RawQuery != "" {
			t.Errorf("Delete sent %+v, want one request without a query", requests)
		}
	}
	for _, status := range []int{
		http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteResourceAttributeRoute,
			mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := resourceClient(t, api).ResourceAttributes.Delete(t.Context(), "document", "owner")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteResourceAttribute ||
			e.Detail != "no" || errors.Is(err, ErrNotFound) {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status and the detail", status, err, opDeleteResourceAttribute)
		}
	}
}

// TestNotFoundOfOtherResourceAttributeCalls checks that a 404 to List, Create, Get and Update,
// which delete nothing, is an *Error that matches ErrNotFound.
func TestNotFoundOfOtherResourceAttributeCalls(t *testing.T) {
	const body = `{"error_code":"NOT_FOUND","title":"Not Found","message":"no such object"}`
	notFound := mockpermit.Reply{Status: http.StatusNotFound, Body: body}
	type notFoundCall struct {
		op    string
		route string
		call  func(ctx context.Context, c *Client) error
	}
	calls := []notFoundCall{
		{
			op: opListResourceAttributes, route: listResourceAttributesRoute,
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ResourceAttributes.List(ctx, "gone", nil)
				return err
			},
		},
		{
			op: opCreateResourceAttribute, route: createResourceAttributeRoute,
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ResourceAttributes.Create(ctx, "gone",
					models.ResourceAttributeCreate{Key: "k", Type: models.AttributeTypeString})
				return err
			},
		},
	}
	for _, m := range resourceAttributeCalls[:2] {
		calls = append(calls, notFoundCall{
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

// TestResourceAttributeBodies checks the bodies of Create and Update byte for byte against the
// golden files in testdata/golden: a create with the required fields only, and one with every
// field, with a type that the SDK does not know, which it sends as it is, and a description that
// an encoder may change, with a line separator and <, > and &; the zero update, which sends {};
// an update that sends null for the description; one that sends an empty description; and one
// that sets every field.
func TestResourceAttributeBodies(t *testing.T) {
	description := "<a>&" + fixtures.LineSeparator + "b"
	create := func(attribute models.ResourceAttributeCreate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.ResourceAttributes.Create(t.Context(), "r", attribute)
			return err
		}
	}
	update := func(change models.ResourceAttributeUpdate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.ResourceAttributes.Update(t.Context(), "r", "a", change)
			return err
		}
	}
	for _, tc := range []struct {
		golden string
		route  string
		call   func(c *Client) error
	}{
		{
			"create_resource_attribute.required.golden", createResourceAttributeRoute,
			create(models.ResourceAttributeCreate{Key: "a", Type: models.AttributeTypeBool}),
		},
		{
			"create_resource_attribute.every-field.golden", createResourceAttributeRoute,
			create(models.ResourceAttributeCreate{
				Key: "a", Type: "geo_point", Description: description,
			}),
		},
		{
			"update_resource_attribute.unset.golden", updateResourceAttributeRoute,
			update(models.ResourceAttributeUpdate{}),
		},
		{
			"update_resource_attribute.null.golden", updateResourceAttributeRoute,
			update(models.ResourceAttributeUpdate{Description: models.Null[string]()}),
		},
		{
			"update_resource_attribute.empty-values.golden", updateResourceAttributeRoute,
			update(models.ResourceAttributeUpdate{Description: models.Some("")}),
		},
		{
			"update_resource_attribute.values.golden", updateResourceAttributeRoute,
			update(models.ResourceAttributeUpdate{
				Type: models.AttributeTypeObjectArray, Description: models.Some(description),
			}),
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: resourceAttributeBody})
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

// TestResourceAttributeRetries checks when a call of the attributes of a resource sends its
// request again: Create, which the API may have acted on, after a 429 only, so a 503 ends it
// after one request; Update, Get, Delete and List, which do the same when sent twice, after a 503
// too.
func TestResourceAttributeRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	ok := mockpermit.Reply{Body: resourceAttributeBody}
	createAttribute := func(c *Client) error {
		_, err := c.ResourceAttributes.Create(t.Context(), "document",
			models.ResourceAttributeCreate{Key: "k", Type: models.AttributeTypeString})
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
			name: "Create after a 429", route: createResourceAttributeRoute,
			replies: []mockpermit.Reply{limited, ok}, call: createAttribute,
		},
		{
			name: "Create after a 503", route: createResourceAttributeRoute,
			replies: []mockpermit.Reply{unavailable}, call: createAttribute,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "List after a 503", route: listResourceAttributesRoute,
			replies: []mockpermit.Reply{unavailable, {Body: `[]`}},
			call: func(c *Client) error {
				_, err := c.ResourceAttributes.List(t.Context(), "document", nil)
				return err
			},
		},
	}
	for _, m := range resourceAttributeCalls {
		tests = append(tests, retryCase{
			name: m.name + " after a 503", route: m.route,
			replies: []mockpermit.Reply{unavailable, m.reply},
			call:    func(c *Client) error { return m.call(t.Context(), c, "document", "owner") },
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

// TestMalformedResourceAttributeReplies checks that a reply in 200-299 that is not an attribute,
// or for List and All not a JSON array of attributes, such as the paginated object of them, which
// the API's documentation of the list does not describe, is an *Error that names the model, and
// no attribute, no page and no attribute of the iterator. A type that is not a string, such as a
// number, is not an attribute either.
func TestMalformedResourceAttributeReplies(t *testing.T) {
	const prefix = "permit: "
	for _, body := range []string{``, `null`, `[]`, `"a"`, `{"type":1}`, `{} {}`} {
		api := mockpermit.NewAPI(t)
		api.Handle(getResourceAttributeRoute, mockpermit.Reply{Body: body})
		c := resourceClient(t, api)
		attribute, err := c.ResourceAttributes.Get(t.Context(), "document", "owner")
		want := prefix + "get_resource_attribute: GET /v2/schema/{proj_id}/{env_id}/resources/" +
			"{resource_id}/attributes/{attribute_id}: status 200: the reply is not a valid " +
			"models.ResourceAttributeRead"
		if e := mustError(t, err); attribute != nil || e.Error() != want {
			t.Errorf("Get of the reply %q returned %+v and\n%v\nwant no attribute and\n%s", body,
				attribute, err, want)
		}
	}
	for _, body := range []string{
		``, `null`, `"a"`, `[1]`, `[{"type":true}]`, `{"data":[]}`, `{"data":[{"key":"owner"}]}`,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(listResourceAttributesRoute, mockpermit.Reply{Body: body},
			mockpermit.Reply{Body: body})
		c := resourceClient(t, api)
		page, err := c.ResourceAttributes.List(t.Context(), "document", nil)
		want := prefix + "list_resource_attributes: GET /v2/schema/{proj_id}/{env_id}/" +
			"resources/{resource_id}/attributes: status 200: the reply is not a valid list of " +
			"models.ResourceAttributeRead"
		if e := mustError(t, err); page != nil || e.Error() != want {
			t.Errorf("List of the reply %q returned %+v and\n%v\nwant no page and\n%s", body,
				page, err, want)
		}
		items, errs := collect(t, c.ResourceAttributes.All(t.Context(), "document", nil))
		if items != nil || len(errs) != 1 || mustError(t, errs[0]).Error() != want {
			t.Errorf("All of the reply %q yielded %+v and %v, want only the error\n%s", body,
				items, errs, want)
		}
	}
}

// TestResourceAttributesAfterClose checks that each method returns ErrClosed after Close, and
// sends nothing, also without WithScope, whose lookup it does not send either.
func TestResourceAttributesAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	_, listErr := c.ResourceAttributes.List(ctx, "document", nil)
	_, allErrs := collect(t, c.ResourceAttributes.All(ctx, "document", nil))
	_, createErr := c.ResourceAttributes.Create(ctx, "document",
		models.ResourceAttributeCreate{Key: "k", Type: models.AttributeTypeString})
	errs := []error{listErr, createErr}
	for _, m := range resourceAttributeCalls {
		errs = append(errs, m.call(t.Context(), c, "document", "owner"))
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
