package permit

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The routes of the attributes of the users, as mockpermit.Server.Handle takes them.
const (
	listUserAttributesRoute  = "GET /v2/schema/{proj_id}/{env_id}/users/attributes"
	createUserAttributeRoute = "POST /v2/schema/{proj_id}/{env_id}/users/attributes"
	getUserAttributeRoute    = "GET /v2/schema/{proj_id}/{env_id}/users/attributes/" +
		"{attribute_id}"
	updateUserAttributeRoute = "PATCH /v2/schema/{proj_id}/{env_id}/users/attributes/" +
		"{attribute_id}"
	deleteUserAttributeRoute = "DELETE /v2/schema/{proj_id}/{env_id}/users/attributes/" +
		"{attribute_id}"
)

// userAttributesPath is the path of the attributes of the users in the scope of resourceClient.
const userAttributesPath = "/v2/schema/p/e/users/attributes"

// userAttributeBody is the body of a reply of the API that holds the attribute department of the
// users.
const userAttributeBody = `{"type":"string","key":"department",` +
	`"id":"00000000-0000-4000-8000-000000000103",` +
	`"resource_id":"00000000-0000-4000-8000-000000000100","resource_key":"__user",` +
	`"organization_id":"00000000-0000-4000-8000-000000000001",` +
	`"project_id":"00000000-0000-4000-8000-000000000002",` +
	`"environment_id":"00000000-0000-4000-8000-000000000003",` +
	`"created_at":"2026-10-14T09:30:00Z","updated_at":"2026-10-14T09:30:00Z","built_in":false}`

// userAttributesOf returns the attributes u<from> to u<to-1>, with only a key and the type
// string: an empty list when to is from.
func userAttributesOf(from, to int) []models.ResourceAttributeRead {
	items := make([]models.ResourceAttributeRead, 0, to-from)
	for i := from; i < to; i++ {
		items = append(items, models.ResourceAttributeRead{
			Key: "u" + strconv.Itoa(i), Type: models.AttributeTypeString,
		})
	}
	return items
}

// userAttributeCalls are calls of each method of the attributes of the users that acts on one
// attribute, with the ID of its operation, its route and a reply of the API to it.
var userAttributeCalls = []struct {
	name  string
	op    string
	route string
	reply mockpermit.Reply
	call  func(ctx context.Context, c *Client, attributeKey string) error
}{
	{
		name: "Get", op: opGetUserAttribute, route: getUserAttributeRoute,
		reply: mockpermit.Reply{Body: userAttributeBody},
		call: func(ctx context.Context, c *Client, attributeKey string) error {
			_, err := c.UserAttributes.Get(ctx, attributeKey)
			return err
		},
	},
	{
		name: "Update", op: opUpdateUserAttribute, route: updateUserAttributeRoute,
		reply: mockpermit.Reply{Body: userAttributeBody},
		call: func(ctx context.Context, c *Client, attributeKey string) error {
			_, err := c.UserAttributes.Update(ctx, attributeKey,
				models.ResourceAttributeUpdate{Description: models.Some("d")})
			return err
		},
	},
	{
		name: "Delete", op: opDeleteUserAttribute, route: deleteUserAttributeRoute,
		reply: mockpermit.Reply{Status: http.StatusNoContent},
		call: func(ctx context.Context, c *Client, attributeKey string) error {
			return c.UserAttributes.Delete(ctx, attributeKey)
		},
	},
}

// createUserAttribute creates the attribute k of the type string with c, and returns the error.
func createUserAttribute(ctx context.Context, c *Client) error {
	_, err := c.UserAttributes.Create(ctx,
		models.ResourceAttributeCreate{Key: "k", Type: models.AttributeTypeString})
	return err
}

// TestUserAttributeTypes checks that List and All read an attribute of each type that the SDK
// knows, Get one of the type object_array, and each one of a type that it does not know,
// geo_point, which they keep as it is, with Known reporting false for it alone: the reply
// fixtures of the list and of the get, and a list whose second page holds the attribute of the
// type that the SDK does not know.
func TestUserAttributeTypes(t *testing.T) {
	known := map[models.AttributeType]bool{
		models.AttributeTypeBool: true, models.AttributeTypeNumber: true,
		models.AttributeTypeString: true, models.AttributeTypeTime: true,
		models.AttributeTypeArray: true, models.AttributeTypeJSON: true,
		models.AttributeTypeObject: true, models.AttributeTypeObjectArray: true,
	}
	listed := replyFixtureOf(t, "list_user_attributes.200.json")
	unknown := replyFixtureOf(t, "list_user_attributes.200.unknown-type.json")
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
		"string", "number", "string", "bool", "time", "array", "json", "object", "object_array",
	}
	t.Run("List", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(listUserAttributesRoute, mockpermit.Reply{Body: listed},
			mockpermit.Reply{Body: unknown})
		c := resourceClient(t, api)
		for _, tc := range []struct {
			page int
			want []models.AttributeType
		}{
			{1, every},
			{2, []models.AttributeType{"geo_point"}},
		} {
			page, err := c.UserAttributes.List(t.Context(), &ListOptions{Page: tc.page})
			if err != nil {
				t.Fatalf("List of page %d returned %v", tc.page, err)
			}
			checkTypes(t, "List of page "+strconv.Itoa(tc.page), page.Data, tc.want)
		}
	})
	t.Run("All", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(listUserAttributesRoute, mockpermit.Reply{Body: listed},
			mockpermit.Reply{Body: unknown}, mockpermit.Reply{Body: `[]`})
		items, errs := collect(t, resourceClient(t, api).UserAttributes.All(t.Context(), nil))
		if errs != nil {
			t.Fatalf("All yielded %v", errs)
		}
		checkTypes(t, "All", items, append(every, "geo_point"))
	})
	t.Run("Get", func(t *testing.T) {
		for fixture, want := range map[string]models.AttributeType{
			"get_user_attribute.200.json":              models.AttributeTypeObjectArray,
			"get_user_attribute.200.unknown-type.json": "geo_point",
		} {
			api := mockpermit.NewAPI(t)
			api.Handle(getUserAttributeRoute, mockpermit.Reply{Body: replyFixtureOf(t, fixture)})
			got, err := resourceClient(t, api).UserAttributes.Get(t.Context(), "department")
			if err != nil {
				t.Fatalf("Get of %s returned %v", fixture, err)
			}
			checkTypes(t, "Get of "+fixture, []models.ResourceAttributeRead{*got},
				[]models.AttributeType{want})
		}
	})
}

// TestUserAttributesAllReadsEveryPage checks that UserAttributesService.All reads the 31
// attributes of a list, in order, asking for each page in turn: at a per_page of 30, two pages of
// attributes and the empty page after them; at the default per_page of 100, the most that the
// API gives on a page, one page of 31 and the empty page after it; and from page 2 on, when the
// options start there.
func TestUserAttributesAllReadsEveryPage(t *testing.T) {
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
				userAttributesOf(0, 30), userAttributesOf(30, 31), {},
			},
			want: userAttributesOf(0, 31),
			queries: []string{
				"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30",
			},
		},
		{
			name:    "the default per_page",
			pages:   [][]models.ResourceAttributeRead{userAttributesOf(0, 31), {}},
			want:    userAttributesOf(0, 31),
			queries: []string{"page=1&per_page=100", "page=2&per_page=100"},
		},
		{
			name:    "from page 2",
			opts:    &ListOptions{Page: 2, PerPage: 30},
			pages:   [][]models.ResourceAttributeRead{userAttributesOf(30, 31), {}},
			want:    userAttributesOf(30, 31),
			queries: []string{"page=2&per_page=30", "page=3&per_page=30"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			var replies []mockpermit.Reply
			for _, items := range tc.pages {
				replies = append(replies, arrayOf(t, items))
			}
			api.Handle(listUserAttributesRoute, replies...)
			c := resourceClient(t, api)
			items, errs := collect(t, c.UserAttributes.All(t.Context(), tc.opts))
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

// TestUserAttributesAllYieldsAnErrorOnce checks that a page that fails ends the iterator with its
// error, once, after the attributes of the pages before it, and that the options of All are read
// when it is called: a later change to them changes no request.
func TestUserAttributesAllYieldsAnErrorOnce(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(listUserAttributesRoute, arrayOf(t, userAttributesOf(0, 30)),
		mockpermit.Reply{Status: http.StatusForbidden, Body: body})
	opts := &ListOptions{PerPage: 30}
	seq := resourceClient(t, api).UserAttributes.All(t.Context(), opts)
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

// TestUserAttributesAllGetsTheScopeInEachLoop checks that All, without WithScope, looks up the
// scope in each loop over the iterator, before its first page, and not when it is called: a
// lookup that fails ends that loop with its error, and the next loop looks the scope up again.
func TestUserAttributesAllGetsTheScopeInEachLoop(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(scopeRoute, mockpermit.Reply{Status: http.StatusForbidden, Body: body},
		scopeReply(t, ""))
	api.Handle(listUserAttributesRoute, arrayOf(t, userAttributesOf(0, 1)),
		arrayOf(t, userAttributesOf(1, 1)))
	seq := projectClient(t, api).UserAttributes.All(t.Context(), nil)
	if requests := api.Requests(); len(requests) != 0 {
		t.Fatalf("All sent %+v before a loop, want no request", requests)
	}
	if items, errs := collect(t, seq); items != nil || len(errs) != 1 ||
		!errors.Is(errs[0], ErrForbidden) {
		t.Errorf("the first loop yielded %v and the errors %v, want one error that matches "+
			"ErrForbidden", items, errs)
	}
	if items, errs := collect(t, seq); !cmp.Equal(userAttributesOf(0, 1), items) ||
		errs != nil {
		t.Errorf("the second loop yielded %v and the errors %v, want u0", items, errs)
	}
	var paths []string
	for _, r := range api.Requests() {
		paths = append(paths, r.RawPath)
	}
	attributes := "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID + "/users/attributes"
	want := []string{"/v2/api-key/scope", "/v2/api-key/scope", attributes, attributes}
	if diff := cmp.Diff(want, paths); diff != "" {
		t.Errorf("the requests differ (-want +got):\n%s", diff)
	}
}

// TestUserAttributesListReturnsAPage checks the page that List returns: the attributes of the
// reply in its order, the page and the per_page that it asked for, page 1 and no query for nil
// options and for the zero options, and no counts, as the reply is a JSON array. A page without
// attributes has an empty list, not nil.
func TestUserAttributesListReturnsAPage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  *ListOptions
		items []models.ResourceAttributeRead
		query string
		want  *Page[models.ResourceAttributeRead]
	}{
		{
			name: "nil options", items: userAttributesOf(0, 2), query: "",
			want: &Page[models.ResourceAttributeRead]{Data: userAttributesOf(0, 2), Page: 1},
		},
		{
			name: "the zero options", opts: &ListOptions{}, items: userAttributesOf(0, 1),
			query: "",
			want:  &Page[models.ResourceAttributeRead]{Data: userAttributesOf(0, 1), Page: 1},
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
			api.Handle(listUserAttributesRoute, arrayOf(t, tc.items))
			c := resourceClient(t, api)
			got, err := c.UserAttributes.List(t.Context(), tc.opts)
			if err != nil {
				t.Fatalf("List returned %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" || got.Data == nil {
				t.Errorf("List returned a page that differs (-want +got), or nil Data:\n%s", diff)
			}
			if requests := api.Requests(); len(requests) != 1 ||
				requests[0].RawPath != userAttributesPath || requests[0].RawQuery != tc.query {
				t.Errorf("List sent %+v, want one request to %s?%s", requests,
					userAttributesPath, tc.query)
			}
		})
	}
}

// TestInvalidUserAttributeListOptions checks that a Page or a PerPage less than zero is an error
// that matches ErrInvalidConfig from List and All, and that neither sends a request.
func TestInvalidUserAttributeListOptions(t *testing.T) {
	for _, opts := range []ListOptions{{Page: -1}, {PerPage: -1}} {
		api := mockpermit.NewAPI(t)
		c := resourceClient(t, api)
		_, err := c.UserAttributes.List(t.Context(), &opts)
		_, errs := collect(t, c.UserAttributes.All(t.Context(), &opts))
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

// TestUserAttributesUseTheScope checks that the calls of the attributes of the users act on the
// environment of the client's scope. Without WithScope, the client looks up the scope of its API
// key once, and each call fills the route with the project and the environment of an
// environment-level key. A scope without an environment, that of a project-level or an
// organization-level key, is an error that matches ErrInvalidConfig and names the operation, and
// no call sends a request but the lookup, which the API fails for any route that the test did
// not register.
func TestUserAttributesUseTheScope(t *testing.T) {
	const prefix = "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID + "/users/attributes"
	calls := map[string]func(ctx context.Context, c *Client) error{
		opListUserAttributes: func(ctx context.Context, c *Client) error {
			_, err := c.UserAttributes.List(ctx, nil)
			return err
		},
		opCreateUserAttribute: createUserAttribute,
	}
	for _, m := range userAttributeCalls {
		calls[m.op] = func(ctx context.Context, c *Client) error {
			return m.call(ctx, c, "department")
		}
	}
	t.Run("an environment-level key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		api.Handle(listUserAttributesRoute, mockpermit.Reply{Body: `[]`})
		api.Handle(createUserAttributeRoute, mockpermit.Reply{Body: userAttributeBody})
		for _, m := range userAttributeCalls {
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
			if r.RawPath != prefix && r.RawPath != prefix+"/department" {
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
			seq := c.UserAttributes.All(t.Context(), nil)
			if _, errs := collect(t, seq); len(errs) != 1 || !errors.Is(errs[0], ErrInvalidConfig) {
				t.Errorf("All yielded %v, want one error that matches ErrInvalidConfig", errs)
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("the calls sent %d requests, want only the lookup of the scope", n)
			}
		})
	}
}

// TestUserAttributeKeysInThePath checks that each method of the attributes of the users that
// acts on one attribute sends its key as one segment of the path, escaped, after the project and
// the environment of the scope, whatever the key holds: a slash, a percent sign, a space, a
// question mark and a hash, a letter outside ASCII, the shape of a UUID and a leading dot. A key
// that is empty, . or .. would change the route, so it is an error that matches
// ErrInvalidConfig and names the placeholder, and no request.
func TestUserAttributeKeysInThePath(t *testing.T) {
	for key, escaped := range map[string]string{
		"a/b":            "a%2Fb",
		"50%":            "50%25",
		"a b?c#d":        "a%20b%3Fc%23d",
		"é":              "%C3%A9",
		fixtures.UUIDKey: fixtures.UUIDKey,
		".hidden":        ".hidden",
		"doc.v2..x":      "doc.v2..x",
	} {
		for _, m := range userAttributeCalls {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, m.reply)
			if err := m.call(t.Context(), resourceClient(t, api), key); err != nil {
				t.Errorf("%s(%q) returned %v", m.name, key, err)
			}
			want := userAttributesPath + "/" + escaped
			if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want {
				t.Errorf("%s(%q) sent %+v, want one request to %s", m.name, key, requests, want)
			}
		}
	}
	for _, key := range []string{"", ".", ".."} {
		for _, m := range userAttributeCalls {
			api := mockpermit.NewAPI(t)
			err := m.call(t.Context(), resourceClient(t, api), key)
			named := err != nil && strings.Contains(err.Error(), "{attribute_id}")
			if !errors.Is(err, ErrInvalidConfig) || !named {
				t.Errorf("%s(%q) returned %v, want an error that matches ErrInvalidConfig and "+
					"names {attribute_id}", m.name, key, err)
			}
			api.AssertUntouched()
		}
	}
}

// TestUserAttributesSendNoResourceID checks that no method of the attributes of the users sends
// resource_id, which the API's documentation lists on each of their routes, so that each acts on
// the attributes of the users and on no other resource's: List and All send page and per_page
// alone, and the other methods send no query.
func TestUserAttributesSendNoResourceID(t *testing.T) {
	type sentCall struct {
		name  string
		route string
		reply mockpermit.Reply
		call  func(ctx context.Context, c *Client) error
		query string
	}
	calls := []sentCall{
		{
			name: "List", route: listUserAttributesRoute, reply: mockpermit.Reply{Body: `[]`},
			call: func(ctx context.Context, c *Client) error {
				_, err := c.UserAttributes.List(ctx, &ListOptions{Page: 2, PerPage: 5})
				return err
			},
			query: "page=2&per_page=5",
		},
		{
			name: "All", route: listUserAttributesRoute, reply: mockpermit.Reply{Body: `[]`},
			call: func(ctx context.Context, c *Client) error {
				_, errs := collect(t, c.UserAttributes.All(ctx, nil))
				return errors.Join(errs...)
			},
			query: "page=1&per_page=100",
		},
		{
			name: "Create", route: createUserAttributeRoute,
			reply: mockpermit.Reply{Body: userAttributeBody}, call: createUserAttribute,
		},
	}
	for _, m := range userAttributeCalls {
		calls = append(calls, sentCall{
			name: m.name, route: m.route, reply: m.reply,
			call: func(ctx context.Context, c *Client) error {
				return m.call(ctx, c, "department")
			},
		})
	}
	for _, m := range calls {
		api := mockpermit.NewAPI(t)
		api.Handle(m.route, m.reply)
		if err := m.call(t.Context(), resourceClient(t, api)); err != nil {
			t.Errorf("%s returned %v", m.name, err)
		}
		requests := api.Requests()
		if len(requests) != 1 {
			t.Errorf("%s sent %d requests, want 1", m.name, len(requests))
			continue
		}
		query, err := url.ParseQuery(requests[0].RawQuery)
		if err != nil || query.Has("resource_id") || requests[0].RawQuery != m.query {
			t.Errorf("%s sent the query %q, want %q, without resource_id", m.name,
				requests[0].RawQuery, m.query)
		}
	}
}

// TestDeleteUserAttributeThatIsGone checks that Delete returns nil for a 404, with or without a
// body, as the attribute that the path names is gone already, after one request with no query,
// though the API's documentation lists resource_id, page and per_page for the route; and an
// *Error with the status for any other error status, such as 403, 409 or 422.
func TestDeleteUserAttributeThatIsGone(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"NOT_FOUND","title":"Not Found","message":"the attribute does not exist"}`,
		"",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteUserAttributeRoute,
			mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		err := resourceClient(t, api).UserAttributes.Delete(t.Context(), "gone")
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
		api.Handle(deleteUserAttributeRoute,
			mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := resourceClient(t, api).UserAttributes.Delete(t.Context(), "department")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteUserAttribute ||
			e.Detail != "no" || errors.Is(err, ErrNotFound) {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status and the detail", status, err, opDeleteUserAttribute)
		}
	}
}

// TestNotFoundOfOtherUserAttributeCalls checks that a 404 to List, Create, Get and Update, which
// delete nothing, is an *Error that matches ErrNotFound.
func TestNotFoundOfOtherUserAttributeCalls(t *testing.T) {
	const body = `{"error_code":"NOT_FOUND","title":"Not Found","message":"no such object"}`
	notFound := mockpermit.Reply{Status: http.StatusNotFound, Body: body}
	type notFoundCall struct {
		op    string
		route string
		call  func(ctx context.Context, c *Client) error
	}
	calls := []notFoundCall{
		{
			op: opListUserAttributes, route: listUserAttributesRoute,
			call: func(ctx context.Context, c *Client) error {
				_, err := c.UserAttributes.List(ctx, nil)
				return err
			},
		},
		{op: opCreateUserAttribute, route: createUserAttributeRoute, call: createUserAttribute},
	}
	for _, m := range userAttributeCalls[:2] {
		calls = append(calls, notFoundCall{
			op: m.op, route: m.route,
			call: func(ctx context.Context, c *Client) error {
				return m.call(ctx, c, "gone")
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

// TestUserAttributeBodies checks the bodies of Create and Update byte for byte against the golden
// files in testdata/golden: a create with the required fields only, and one with every field,
// with a type that the SDK does not know, which it sends as it is, and a description that an
// encoder may change, with a line separator and <, > and &; the zero update, which sends {}; an
// update that sends null for the description; one that sends an empty description; and one that
// sets every field.
func TestUserAttributeBodies(t *testing.T) {
	description := "<a>&" + fixtures.LineSeparator + "b"
	create := func(attribute models.ResourceAttributeCreate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.UserAttributes.Create(t.Context(), attribute)
			return err
		}
	}
	update := func(change models.ResourceAttributeUpdate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.UserAttributes.Update(t.Context(), "a", change)
			return err
		}
	}
	for _, tc := range []struct {
		golden string
		route  string
		call   func(c *Client) error
	}{
		{
			"create_user_attribute.required.golden", createUserAttributeRoute,
			create(models.ResourceAttributeCreate{Key: "a", Type: models.AttributeTypeBool}),
		},
		{
			"create_user_attribute.every-field.golden", createUserAttributeRoute,
			create(models.ResourceAttributeCreate{
				Key: "a", Type: "geo_point", Description: description,
			}),
		},
		{
			"update_user_attribute.unset.golden", updateUserAttributeRoute,
			update(models.ResourceAttributeUpdate{}),
		},
		{
			"update_user_attribute.null.golden", updateUserAttributeRoute,
			update(models.ResourceAttributeUpdate{Description: models.Null[string]()}),
		},
		{
			"update_user_attribute.empty-values.golden", updateUserAttributeRoute,
			update(models.ResourceAttributeUpdate{Description: models.Some("")}),
		},
		{
			"update_user_attribute.values.golden", updateUserAttributeRoute,
			update(models.ResourceAttributeUpdate{
				Type: models.AttributeTypeObjectArray, Description: models.Some(description),
			}),
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: userAttributeBody})
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

// TestUserAttributeRetries checks when a call of the attributes of the users sends its request
// again: Create, which the API may have acted on, after a 429 only, so a 503 ends it after one
// request; Update, Get, Delete and List, which do the same when sent twice, after a 503 too.
func TestUserAttributeRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	ok := mockpermit.Reply{Body: userAttributeBody}
	create := func(c *Client) error { return createUserAttribute(t.Context(), c) }
	type retryCase struct {
		name    string
		route   string
		replies []mockpermit.Reply
		call    func(c *Client) error
		status  int
	}
	tests := []retryCase{
		{
			name: "Create after a 429", route: createUserAttributeRoute,
			replies: []mockpermit.Reply{limited, ok}, call: create,
		},
		{
			name: "Create after a 503", route: createUserAttributeRoute,
			replies: []mockpermit.Reply{unavailable}, call: create,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "List after a 503", route: listUserAttributesRoute,
			replies: []mockpermit.Reply{unavailable, {Body: `[]`}},
			call: func(c *Client) error {
				_, err := c.UserAttributes.List(t.Context(), nil)
				return err
			},
		},
	}
	for _, m := range userAttributeCalls {
		tests = append(tests, retryCase{
			name: m.name + " after a 503", route: m.route,
			replies: []mockpermit.Reply{unavailable, m.reply},
			call:    func(c *Client) error { return m.call(t.Context(), c, "department") },
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

// TestMalformedUserAttributeReplies checks that a reply in 200-299 that is not an attribute, or
// for List and All not a JSON array of attributes, such as the paginated object of them, which
// the API's documentation of the list does not describe, is an *Error that names the model, and
// no attribute, no page and no attribute of the iterator. A type that is not a string, such as a
// number, is not an attribute either.
func TestMalformedUserAttributeReplies(t *testing.T) {
	const prefix = "permit: "
	for _, body := range []string{``, `null`, `[]`, `"a"`, `{"type":1}`, `{} {}`} {
		api := mockpermit.NewAPI(t)
		api.Handle(getUserAttributeRoute, mockpermit.Reply{Body: body})
		c := resourceClient(t, api)
		attribute, err := c.UserAttributes.Get(t.Context(), "department")
		want := prefix + "get_user_attribute: GET /v2/schema/{proj_id}/{env_id}/users/" +
			"attributes/{attribute_id}: status 200: the reply is not a valid " +
			"models.ResourceAttributeRead"
		if e := mustError(t, err); attribute != nil || e.Error() != want {
			t.Errorf("Get of the reply %q returned %+v and\n%v\nwant no attribute and\n%s", body,
				attribute, err, want)
		}
	}
	for _, body := range []string{
		``, `null`, `"a"`, `[1]`, `[{"type":true}]`, `{"data":[]}`, `{"data":[{"key":"age"}]}`,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(listUserAttributesRoute, mockpermit.Reply{Body: body},
			mockpermit.Reply{Body: body})
		c := resourceClient(t, api)
		page, err := c.UserAttributes.List(t.Context(), nil)
		want := prefix + "list_user_attributes: GET /v2/schema/{proj_id}/{env_id}/users/" +
			"attributes: status 200: the reply is not a valid list of " +
			"models.ResourceAttributeRead"
		if e := mustError(t, err); page != nil || e.Error() != want {
			t.Errorf("List of the reply %q returned %+v and\n%v\nwant no page and\n%s", body,
				page, err, want)
		}
		items, errs := collect(t, c.UserAttributes.All(t.Context(), nil))
		if items != nil || len(errs) != 1 || mustError(t, errs[0]).Error() != want {
			t.Errorf("All of the reply %q yielded %+v and %v, want only the error\n%s", body,
				items, errs, want)
		}
	}
}

// TestUserAttributesAfterClose checks that each method returns ErrClosed after Close, and sends
// nothing, also without WithScope, whose lookup it does not send either.
func TestUserAttributesAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	_, listErr := c.UserAttributes.List(ctx, nil)
	_, allErrs := collect(t, c.UserAttributes.All(ctx, nil))
	errs := []error{listErr, createUserAttribute(ctx, c)}
	for _, m := range userAttributeCalls {
		errs = append(errs, m.call(t.Context(), c, "department"))
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
