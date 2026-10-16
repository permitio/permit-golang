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

// The routes of the relations of a resource, as mockpermit.Server.Handle takes them.
const (
	relationsPath        = "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/relations"
	relationPath         = relationsPath + "/{relation_id}"
	listRelationsRoute   = "GET " + relationsPath
	createRelationRoute  = "POST " + relationsPath
	getRelationRoute     = "GET " + relationPath
	deleteRelationRoute  = "DELETE " + relationPath
	documentRelationsRaw = "/v2/schema/p/e/resources/document/relations"
)

// relationBody is the body of a reply of the API that holds the relation parent of the resource
// document to the resource folder.
const relationBody = `{"subject_resource":"folder","key":"parent","name":"Parent",` +
	`"id":"00000000-0000-4000-8000-000000000071",` +
	`"organization_id":"00000000-0000-4000-8000-000000000001",` +
	`"project_id":"00000000-0000-4000-8000-000000000002",` +
	`"environment_id":"00000000-0000-4000-8000-000000000003",` +
	`"created_at":"2026-10-14T09:30:00Z","updated_at":"2026-10-14T09:30:00Z",` +
	`"object_resource_id":"00000000-0000-4000-8000-000000000021","object_resource":"document",` +
	`"subject_resource_id":"00000000-0000-4000-8000-000000000022"}`

// relationsOf returns the relations r<from> to r<to-1>, with only a key and a name: an empty
// list when to is from.
func relationsOf(from, to int) []models.RelationRead {
	items := make([]models.RelationRead, 0, to-from)
	for i := from; i < to; i++ {
		key := "r" + strconv.Itoa(i)
		items = append(items, models.RelationRead{Key: key, Name: key})
	}
	return items
}

// relationCalls are calls of each method of the relations that acts on one relation, with the ID
// of its operation, its route and a reply of the API to it. Each call takes the key of the
// resource and the key of the relation.
var relationCalls = []struct {
	name  string
	op    string
	route string
	reply mockpermit.Reply
	call  func(ctx context.Context, c *Client, resourceKey, relationKey string) error
}{
	{
		name: "Get", op: opGetResourceRelation, route: getRelationRoute,
		reply: mockpermit.Reply{Body: relationBody},
		call: func(ctx context.Context, c *Client, resourceKey, relationKey string) error {
			_, err := c.ResourceRelations.Get(ctx, resourceKey, relationKey)
			return err
		},
	},
	{
		name: "Delete", op: opDeleteResourceRelation, route: deleteRelationRoute,
		reply: mockpermit.Reply{Status: http.StatusNoContent},
		call: func(ctx context.Context, c *Client, resourceKey, relationKey string) error {
			return c.ResourceRelations.Delete(ctx, resourceKey, relationKey)
		},
	},
}

// resourceCallsOfRelations are calls of each method of the relations that names a resource but
// no relation, with the ID of its operation, its route and a reply of the API to it.
var resourceCallsOfRelations = []struct {
	name  string
	op    string
	route string
	reply mockpermit.Reply
	call  func(ctx context.Context, c *Client, resourceKey string) error
}{
	{
		name: "List", op: opListResourceRelations, route: listRelationsRoute,
		reply: mockpermit.Reply{Body: `{"data":[],"total_count":0,"page_count":0}`},
		call: func(ctx context.Context, c *Client, resourceKey string) error {
			_, err := c.ResourceRelations.List(ctx, resourceKey, nil)
			return err
		},
	},
	{
		name: "Create", op: opCreateResourceRelation, route: createRelationRoute,
		reply: mockpermit.Reply{Body: relationBody},
		call: func(ctx context.Context, c *Client, resourceKey string) error {
			_, err := c.ResourceRelations.Create(ctx, resourceKey,
				models.RelationCreate{Key: "k", Name: "n", SubjectResource: "s"})
			return err
		},
	},
}

// TestResourceRelationsAllReadsEveryPage checks that All reads the 31 relations of a resource, in
// order, asking for each page in turn: up to the page whose relations reach the total_count of
// its reply, at a per_page of 30 and at the default per_page of 100, the most that the API gives
// on a page; up to the empty page when the replies do not report total_count; and from page 2 on,
// when the options start there, up to the empty page after it, as a loop that starts after the
// first page does not stop at the total_count.
func TestResourceRelationsAllReadsEveryPage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    *ListOptions
		replies []mockpermit.Reply
		want    []models.RelationRead
		queries []string
	}{
		{
			name: "per_page 30", opts: &ListOptions{PerPage: 30},
			replies: []mockpermit.Reply{
				pageOf(t, relationsOf(0, 30), `,"total_count":31,"page_count":30`),
				pageOf(t, relationsOf(30, 31), `,"total_count":31,"page_count":1`),
			},
			want:    relationsOf(0, 31),
			queries: []string{"page=1&per_page=30", "page=2&per_page=30"},
		},
		{
			name: "the default per_page",
			replies: []mockpermit.Reply{
				pageOf(t, relationsOf(0, 31), `,"total_count":31,"page_count":31`),
			},
			want:    relationsOf(0, 31),
			queries: []string{"page=1&per_page=100"},
		},
		{
			name: "pages without total_count", opts: &ListOptions{PerPage: 30},
			replies: []mockpermit.Reply{
				pageOf(t, relationsOf(0, 30), `,"page_count":30`),
				pageOf(t, relationsOf(30, 31), `,"page_count":1`),
				pageOf(t, relationsOf(0, 0), `,"page_count":0`),
			},
			want: relationsOf(0, 31),
			queries: []string{
				"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30",
			},
		},
		{
			name: "from page 2", opts: &ListOptions{Page: 2, PerPage: 30},
			replies: []mockpermit.Reply{
				pageOf(t, relationsOf(30, 31), `,"total_count":31,"page_count":1`),
				pageOf(t, relationsOf(0, 0), `,"total_count":31,"page_count":0`),
			},
			want:    relationsOf(30, 31),
			queries: []string{"page=2&per_page=30", "page=3&per_page=30"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listRelationsRoute, tc.replies...)
			items, errs := collect(t, resourceClient(t, api).ResourceRelations.All(t.Context(),
				"document", tc.opts))
			if diff := cmp.Diff(tc.want, items); diff != "" || errs != nil {
				t.Errorf("All yielded the errors %v and relations that differ (-want +got):\n%s",
					errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
			for _, r := range api.Requests() {
				if r.RawPath != documentRelationsRaw {
					t.Errorf("All sent a request to %s, want %s", r.RawPath, documentRelationsRaw)
				}
			}
		})
	}
}

// TestResourceRelationsAllReadsTheOptionsOnce checks that the iterator keeps the options of its
// call, so that a later change to them changes no request, that a loop that breaks asks for no
// other page, and that each loop over the iterator reads the relations again from the first page.
func TestResourceRelationsAllReadsTheOptionsOnce(t *testing.T) {
	api := mockpermit.NewAPI(t)
	two := pageOf(t, relationsOf(0, 2), `,"total_count":4,"page_count":2`)
	api.Handle(listRelationsRoute, two, two)
	opts := &ListOptions{PerPage: 2}
	seq := resourceClient(t, api).ResourceRelations.All(t.Context(), "document", opts)
	opts.Page, opts.PerPage = 5, 50
	for loop := 1; loop <= 2; loop++ {
		for relation, err := range seq {
			if err != nil || relation.Key != "r0" {
				t.Errorf("loop %d yielded %+v and %v first, want r0", loop, relation, err)
			}
			break
		}
	}
	first := "page=1&per_page=2"
	if diff := cmp.Diff([]string{first, first}, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestResourceRelationsAllYieldsAnErrorOnce checks that a page that fails ends the iterator with
// its error, once, after the relations of the pages before it.
func TestResourceRelationsAllYieldsAnErrorOnce(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(listRelationsRoute, pageOf(t, relationsOf(0, 30), `,"total_count":60`),
		mockpermit.Reply{Status: http.StatusForbidden, Body: body})
	items, errs := collect(t, resourceClient(t, api).ResourceRelations.All(t.Context(),
		"document", &ListOptions{PerPage: 30}))
	if len(items) != 30 || len(errs) != 1 || !errors.Is(errs[0], ErrForbidden) {
		t.Errorf("All yielded %d relations and the errors %v, want 30 and one error that "+
			"matches ErrForbidden", len(items), errs)
	}
	if n := len(api.Requests()); n != 2 {
		t.Errorf("All sent %d requests, want 2", n)
	}
}

// TestResourceRelationsAllGetsTheScopeInEachLoop checks that All, without WithScope, looks up the
// scope in each loop over the iterator, before its first page, and not when it is called: a
// lookup that fails ends that loop with its error, and the next loop looks the scope up again.
func TestResourceRelationsAllGetsTheScopeInEachLoop(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(scopeRoute, mockpermit.Reply{Status: http.StatusForbidden, Body: body},
		scopeReply(t, ""))
	api.Handle(listRelationsRoute,
		pageOf(t, relationsOf(0, 1), `,"total_count":1,"page_count":1`))
	seq := projectClient(t, api).ResourceRelations.All(t.Context(), "document", nil)
	if requests := api.Requests(); len(requests) != 0 {
		t.Fatalf("All sent %+v before a loop, want no request", requests)
	}
	if items, errs := collect(t, seq); items != nil || len(errs) != 1 ||
		!errors.Is(errs[0], ErrForbidden) {
		t.Errorf("the first loop yielded %v and the errors %v, want one error that matches "+
			"ErrForbidden", items, errs)
	}
	if items, errs := collect(t, seq); !cmp.Equal(relationsOf(0, 1), items) || errs != nil {
		t.Errorf("the second loop yielded %v and the errors %v, want r0", items, errs)
	}
	var paths []string
	for _, r := range api.Requests() {
		paths = append(paths, r.RawPath)
	}
	relations := "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID +
		"/resources/document/relations"
	want := []string{"/v2/api-key/scope", "/v2/api-key/scope", relations}
	if diff := cmp.Diff(want, paths); diff != "" {
		t.Errorf("the requests differ (-want +got):\n%s", diff)
	}
}

// TestResourceRelationsListReturnsAPage checks the page that List returns: the relations of the
// reply in its order, the page and the per_page that it asked for, page 1 and no query for nil
// options, and the counts that the reply reports, nil for a count that it leaves out. A page with
// relations and a page without them both have a list that is not nil, empty for the second.
func TestResourceRelationsListReturnsAPage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  *ListOptions
		reply mockpermit.Reply
		query string
		want  *Page[models.RelationRead]
	}{
		{
			name:  "nil options",
			reply: pageOf(t, relationsOf(0, 2), `,"total_count":2,"page_count":2`),
			want: &Page[models.RelationRead]{
				Data: relationsOf(0, 2), Page: 1, TotalCount: new(2), PageCount: new(2),
			},
		},
		{
			name: "a page without page_count", opts: &ListOptions{PerPage: 1},
			reply: pageOf(t, relationsOf(0, 1), `,"total_count":3`),
			query: "per_page=1",
			want: &Page[models.RelationRead]{
				Data: relationsOf(0, 1), Page: 1, PerPage: 1, TotalCount: new(3),
			},
		},
		{
			name: "a page after the last", opts: &ListOptions{Page: 3, PerPage: 30},
			reply: pageOf(t, []models.RelationRead{}, `,"total_count":31,"page_count":0`),
			query: "page=3&per_page=30",
			want: &Page[models.RelationRead]{
				Data: []models.RelationRead{}, Page: 3, PerPage: 30, TotalCount: new(31),
				PageCount: new(0),
			},
		},
		{
			name:  "no relations",
			reply: mockpermit.Reply{Body: `{"data":[],"total_count":0,"page_count":0}`},
			want: &Page[models.RelationRead]{
				Data: []models.RelationRead{}, Page: 1, TotalCount: new(0), PageCount: new(0),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listRelationsRoute, tc.reply)
			got, err := resourceClient(t, api).ResourceRelations.List(t.Context(), "document",
				tc.opts)
			if err != nil {
				t.Fatalf("List returned %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" || got.Data == nil {
				t.Errorf("List returned a page that differs (-want +got), or nil Data:\n%s", diff)
			}
			if requests := api.Requests(); len(requests) != 1 ||
				requests[0].RawPath != documentRelationsRaw || requests[0].RawQuery != tc.query {
				t.Errorf("List sent %+v, want one request to %s?%s", requests,
					documentRelationsRaw, tc.query)
			}
		})
	}
}

// TestInvalidResourceRelationListOptions checks that a Page or a PerPage less than zero is an
// error that matches ErrInvalidConfig from List and All, and that neither sends a request.
func TestInvalidResourceRelationListOptions(t *testing.T) {
	for _, opts := range []ListOptions{{Page: -1}, {PerPage: -1}} {
		api := mockpermit.NewAPI(t)
		c := resourceClient(t, api)
		_, err := c.ResourceRelations.List(t.Context(), "document", &opts)
		_, errs := collect(t, c.ResourceRelations.All(t.Context(), "document", &opts))
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

// TestResourceRelationsUseTheScope checks that the calls of the relations act on the environment
// of the client's scope. Without WithScope, the client looks up the scope of its API key once,
// and each call fills the route with the project and the environment of an environment-level
// key. A scope without an environment, that of a project-level or an organization-level key, is
// an error that matches ErrInvalidConfig and names the operation, and no call sends a request but
// the lookup, which the API fails for any route that the test did not register.
func TestResourceRelationsUseTheScope(t *testing.T) {
	const prefix = "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID +
		"/resources/document/relations"
	calls := map[string]func(ctx context.Context, c *Client) error{}
	for _, m := range resourceCallsOfRelations {
		calls[m.op] = func(ctx context.Context, c *Client) error {
			return m.call(ctx, c, "document")
		}
	}
	for _, m := range relationCalls {
		calls[m.op] = func(ctx context.Context, c *Client) error {
			return m.call(ctx, c, "document", "parent")
		}
	}
	t.Run("an environment-level key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		for _, m := range resourceCallsOfRelations {
			api.Handle(m.route, m.reply)
		}
		for _, m := range relationCalls {
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
			if r.RawPath != prefix && r.RawPath != prefix+"/parent" {
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
			_, errs := collect(t, c.ResourceRelations.All(t.Context(), "document", nil))
			if len(errs) != 1 || !errors.Is(errs[0], ErrInvalidConfig) {
				t.Errorf("All yielded %v, want one error that matches ErrInvalidConfig", errs)
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("the calls sent %d requests, want only the lookup of the scope", n)
			}
		})
	}
}

// TestResourceRelationKeysInThePath checks that each method sends the key of the resource, and
// the key of the relation when it names one, each as one segment of the path, escaped, after the
// project and the environment of the scope, whatever the key holds: escaped where the key holds a
// slash, a percent sign, a space, a question mark and a hash, or a letter outside ASCII, and as it
// is for the shape of a UUID, a leading dot and dots inside a key. A key that is empty, . or ..
// would change the route, so it is an error that matches ErrInvalidConfig and names its
// placeholder, and no request.
func TestResourceRelationKeysInThePath(t *testing.T) {
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
		for _, m := range resourceCallsOfRelations {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, m.reply)
			if err := m.call(t.Context(), resourceClient(t, api), key); err != nil {
				t.Errorf("%s(%q) returned %v", m.name, key, err)
			}
			want := prefix + segment + "/relations"
			if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want {
				t.Errorf("%s(%q) sent %+v, want one request to %s", m.name, key, requests, want)
			}
		}
		for _, m := range relationCalls {
			for _, keys := range [][2]string{{key, "parent"}, {"document", key}} {
				api := mockpermit.NewAPI(t)
				api.Handle(m.route, m.reply)
				err := m.call(t.Context(), resourceClient(t, api), keys[0], keys[1])
				if err != nil {
					t.Errorf("%s(%q, %q) returned %v", m.name, keys[0], keys[1], err)
				}
				want := prefix + "document/relations/" + segment
				if keys[0] == key {
					want = prefix + segment + "/relations/parent"
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
		for _, m := range resourceCallsOfRelations {
			api := mockpermit.NewAPI(t)
			err := m.call(t.Context(), resourceClient(t, api), key)
			checkRelationKeyError(t, m.name, key, "{resource_id}", err)
			api.AssertUntouched()
		}
		for _, m := range relationCalls {
			api := mockpermit.NewAPI(t)
			c := resourceClient(t, api)
			checkRelationKeyError(t, m.name, key, "{resource_id}",
				m.call(t.Context(), c, key, "parent"))
			checkRelationKeyError(t, m.name, key, "{relation_id}",
				m.call(t.Context(), c, "document", key))
			api.AssertUntouched()
		}
	}
}

// checkRelationKeyError checks that err, the error of the call of method with key in the
// placeholder, matches ErrInvalidConfig and names the placeholder.
func checkRelationKeyError(t *testing.T, method, key, placeholder string, err error) {
	t.Helper()
	if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), placeholder) {
		t.Errorf("%s with %q in %s returned %v, want an error that matches ErrInvalidConfig "+
			"and names %s", method, key, placeholder, err, placeholder)
	}
}

// TestDeleteResourceRelationThatIsGone checks that Delete returns nil for a 404, with or without a
// body, as the relation that the path names is gone already, after one request to the route of
// the relation; and an *Error with the status for any other error status, such as 403, 409 or
// 422.
func TestDeleteResourceRelationThatIsGone(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"NOT_FOUND","title":"Not Found","message":"the relation does not exist"}`,
		"",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteRelationRoute, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		err := resourceClient(t, api).ResourceRelations.Delete(t.Context(), "document", "gone")
		if err != nil {
			t.Errorf("Delete of a relation that the API answers with 404 and the body %q "+
				"returned %v, want nil", body, err)
		}
		requests := api.Requests()
		if want := documentRelationsRaw + "/gone"; len(requests) != 1 ||
			requests[0].RawPath != want {
			t.Errorf("Delete sent %+v, want one request to %s", requests, want)
		}
	}
	for _, status := range []int{
		http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteRelationRoute, mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := resourceClient(t, api).ResourceRelations.Delete(t.Context(), "document", "parent")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteResourceRelation ||
			e.Detail != "no" || errors.Is(err, ErrNotFound) {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status and the detail", status, err, opDeleteResourceRelation)
		}
	}
}

// TestNotFoundOfOtherResourceRelationCalls checks that a 404 to List, Create and Get, which delete
// nothing, is an *Error that matches ErrNotFound.
func TestNotFoundOfOtherResourceRelationCalls(t *testing.T) {
	const body = `{"error_code":"NOT_FOUND","title":"Not Found","message":"no resource"}`
	notFound := mockpermit.Reply{Status: http.StatusNotFound, Body: body}
	calls := map[string]func(c *Client) error{}
	for _, m := range resourceCallsOfRelations {
		calls[m.route] = func(c *Client) error { return m.call(t.Context(), c, "gone") }
	}
	calls[getRelationRoute] = func(c *Client) error {
		_, err := c.ResourceRelations.Get(t.Context(), "document", "gone")
		return err
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

// TestResourceRelationBodies checks the bodies of Create byte for byte against the golden files
// in testdata/golden: a create with the required fields only, the zero create, which still sends
// the key, the name and the subject, empty, and a create with every field, whose description
// keeps <, > and & as they are.
func TestResourceRelationBodies(t *testing.T) {
	for _, tc := range []struct {
		golden   string
		relation models.RelationCreate
	}{
		{
			"create_resource_relation.required.golden",
			models.RelationCreate{Key: "parent", Name: "Parent", SubjectResource: "folder"},
		},
		{"create_resource_relation.zero.golden", models.RelationCreate{}},
		{
			"create_resource_relation.every-field.golden",
			models.RelationCreate{
				Key: "parent", Name: "Parent",
				Description:     "a" + fixtures.LineSeparator + "b <c> & d",
				SubjectResource: fixtures.UUIDKey,
			},
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(createRelationRoute, mockpermit.Reply{Body: relationBody})
			_, err := resourceClient(t, api).ResourceRelations.Create(t.Context(), "document",
				tc.relation)
			if err != nil {
				t.Fatalf("Create returned %v", err)
			}
			requests := api.Requests()
			if len(requests) != 1 {
				t.Fatalf("Create sent %d requests, want 1", len(requests))
			}
			requests[0].ExpectBytes(t, filepath.Join("testdata", "golden", tc.golden))
		})
	}
}

// TestResourceRelationRetries checks when a call of the relations sends its request again:
// Create, which the API may have acted on, after a 429 only, so a 503 ends it after one request;
// Get, Delete and List, which do the same when sent twice, after a 503 too.
func TestResourceRelationRetries(t *testing.T) {
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
	for _, m := range resourceCallsOfRelations {
		call := func(c *Client) error { return m.call(t.Context(), c, "document") }
		if m.op == opCreateResourceRelation {
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
	for _, m := range relationCalls {
		tests = append(tests, retryCase{
			name: m.name + " after a 503", route: m.route,
			replies: []mockpermit.Reply{unavailable, m.reply},
			call:    func(c *Client) error { return m.call(t.Context(), c, "document", "parent") },
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

// TestMalformedResourceRelationReplies checks that a reply in 200-299 that is not a relation, or
// for List and All not the paginated object of relations, such as a JSON array of them or an
// object without data, is an *Error that names the model, and no relation.
func TestMalformedResourceRelationReplies(t *testing.T) {
	const prefix = "permit: "
	for _, body := range []string{``, `null`, `[]`, `"r"`, `{"key":1}`, `{} {}`} {
		api := mockpermit.NewAPI(t)
		api.Handle(getRelationRoute, mockpermit.Reply{Body: body})
		relation, err := resourceClient(t, api).ResourceRelations.Get(t.Context(), "document",
			"parent")
		want := prefix + "get_resource_relation: GET " + relationPath +
			": status 200: the reply is not a valid models.RelationRead"
		if e := mustError(t, err); relation != nil || e.Error() != want {
			t.Errorf("Get of the reply %q returned %+v and\n%v\nwant no relation and\n%s",
				body, relation, err, want)
		}
	}
	for _, body := range []string{
		``, `null`, `"r"`, `[]`, `[` + relationBody + `]`, `{"total_count":0}`,
		`{"data":null,"total_count":0}`, `{"data":[{"key":1}],"total_count":1}`,
		`{"data":[],"total_count":"0"}`,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(listRelationsRoute, mockpermit.Reply{Body: body})
		page, err := resourceClient(t, api).ResourceRelations.List(t.Context(), "document", nil)
		want := prefix + "list_resource_relations: GET " + relationsPath +
			": status 200: the reply is not a valid list of models.RelationRead"
		if e := mustError(t, err); page != nil || e.Error() != want {
			t.Errorf("List of the reply %q returned %+v and\n%v\nwant no page and\n%s", body,
				page, err, want)
		}
	}
	for _, body := range []string{`[]`, `[` + relationBody + `]`} {
		api := mockpermit.NewAPI(t)
		api.Handle(listRelationsRoute, mockpermit.Reply{Body: body})
		items, errs := collect(t, resourceClient(t, api).ResourceRelations.All(t.Context(),
			"document", nil))
		want := prefix + "list_resource_relations: GET " + relationsPath +
			": status 200: the reply is not a valid list of models.RelationRead"
		if len(items) != 0 || len(errs) != 1 || mustError(t, errs[0]).Error() != want {
			t.Errorf("All of the reply %q yielded %+v and the errors %v\nwant no relation and "+
				"one error:\n%s", body, items, errs, want)
		}
	}
}

// TestResourceRelationsAfterClose checks that each method returns ErrClosed after Close, and
// sends nothing, also without WithScope, whose lookup it does not send either.
func TestResourceRelationsAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	var errs []error
	for _, m := range resourceCallsOfRelations {
		errs = append(errs, m.call(t.Context(), c, "document"))
	}
	for _, m := range relationCalls {
		errs = append(errs, m.call(t.Context(), c, "document", "parent"))
	}
	_, allErrs := collect(t, c.ResourceRelations.All(t.Context(), "document", nil))
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
