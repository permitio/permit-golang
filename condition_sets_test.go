package permit

import (
	"context"
	"errors"
	"iter"
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

// The routes of the condition sets of an environment, as mockpermit.Server.Handle takes them.
const (
	listConditionSetsRoute  = "GET /v2/schema/{proj_id}/{env_id}/condition_sets"
	createConditionSetRoute = "POST /v2/schema/{proj_id}/{env_id}/condition_sets"
	getConditionSetRoute    = "GET /v2/schema/{proj_id}/{env_id}/condition_sets/" +
		"{condition_set_id}"
	updateConditionSetRoute = "PATCH /v2/schema/{proj_id}/{env_id}/condition_sets/" +
		"{condition_set_id}"
	deleteConditionSetRoute = "DELETE /v2/schema/{proj_id}/{env_id}/condition_sets/" +
		"{condition_set_id}"
	conditionSetAncestorsRoute = "GET /v2/schema/{proj_id}/{env_id}/condition_sets/" +
		"{condition_set_id}/ancestors"
	conditionSetDescendantsRoute = "GET /v2/schema/{proj_id}/{env_id}/condition_sets/" +
		"{condition_set_id}/descendants"
)

// conditionSetsPath is the path of the condition sets in the scope of resourceClient.
const conditionSetsPath = "/v2/schema/p/e/condition_sets"

// conditionSetBody is the body of a reply of the API that holds the user set adults.
const conditionSetBody = `{"key":"adults","type":"userset",` +
	`"id":"00000000-0000-4000-8000-000000000201",` +
	`"organization_id":"00000000-0000-4000-8000-000000000001",` +
	`"project_id":"00000000-0000-4000-8000-000000000002",` +
	`"environment_id":"00000000-0000-4000-8000-000000000003",` +
	`"created_at":"2026-10-14T09:30:00Z","updated_at":"2026-10-14T09:30:00Z","name":"Adults",` +
	`"conditions":{"allOf":[{"allOf":[{"user.age":{"greater-than":18}}]}]}}`

// setsOf returns the condition sets s<from> to s<to-1>, with only a key and a name: an empty list
// when to is from.
func setsOf(from, to int) []models.ConditionSetRead {
	items := make([]models.ConditionSetRead, 0, to-from)
	for i := from; i < to; i++ {
		key := "s" + strconv.Itoa(i)
		items = append(items, models.ConditionSetRead{Key: key, Name: key})
	}
	return items
}

// setsReply returns a reply that holds items: a JSON array without counts, and the paginated
// object with the members of counts, such as `,"total_count":2`, and the page_count that the API
// sends, the number of items, with them.
func setsReply(t *testing.T, items []models.ConditionSetRead, counts string) mockpermit.Reply {
	t.Helper()
	if counts == "" {
		return arrayOf(t, items)
	}
	return pageOf(t, items, counts+`,"page_count":`+strconv.Itoa(len(items)))
}

// conditionSetCall is a call of a method of the condition sets, with the ID of its operation,
// its route and a reply of the API to it.
type conditionSetCall struct {
	name  string
	op    string
	route string
	reply mockpermit.Reply
	call  func(ctx context.Context, c *Client, conditionSetKey string) error
}

// keyedConditionSetCalls are calls of each method of the condition sets that acts on the set
// whose key it takes.
var keyedConditionSetCalls = []conditionSetCall{
	{
		name: "Get", op: opGetConditionSet, route: getConditionSetRoute,
		reply: mockpermit.Reply{Body: conditionSetBody},
		call: func(ctx context.Context, c *Client, conditionSetKey string) error {
			_, err := c.ConditionSets.Get(ctx, conditionSetKey)
			return err
		},
	},
	{
		name: "Update", op: opUpdateConditionSet, route: updateConditionSetRoute,
		reply: mockpermit.Reply{Body: conditionSetBody},
		call: func(ctx context.Context, c *Client, conditionSetKey string) error {
			_, err := c.ConditionSets.Update(ctx, conditionSetKey,
				models.ConditionSetUpdate{Name: "n"})
			return err
		},
	},
	{
		name: "Delete", op: opDeleteConditionSet, route: deleteConditionSetRoute,
		reply: mockpermit.Reply{Status: http.StatusNoContent},
		call: func(ctx context.Context, c *Client, conditionSetKey string) error {
			return c.ConditionSets.Delete(ctx, conditionSetKey)
		},
	},
	{
		name: "Ancestors", op: opConditionSetAncestors, route: conditionSetAncestorsRoute,
		reply: mockpermit.Reply{Body: `[]`},
		call: func(ctx context.Context, c *Client, conditionSetKey string) error {
			_, err := c.ConditionSets.Ancestors(ctx, conditionSetKey, nil)
			return err
		},
	},
	{
		name: "AncestorsAll", op: opConditionSetAncestors, route: conditionSetAncestorsRoute,
		reply: mockpermit.Reply{Body: `[]`},
		call: func(ctx context.Context, c *Client, conditionSetKey string) error {
			return firstError(c.ConditionSets.AncestorsAll(ctx, conditionSetKey, nil))
		},
	},
	{
		name: "Descendants", op: opConditionSetDescendants, route: conditionSetDescendantsRoute,
		reply: mockpermit.Reply{Body: `[]`},
		call: func(ctx context.Context, c *Client, conditionSetKey string) error {
			_, err := c.ConditionSets.Descendants(ctx, conditionSetKey, nil)
			return err
		},
	},
	{
		name: "DescendantsAll", op: opConditionSetDescendants,
		route: conditionSetDescendantsRoute, reply: mockpermit.Reply{Body: `[]`},
		call: func(ctx context.Context, c *Client, conditionSetKey string) error {
			return firstError(c.ConditionSets.DescendantsAll(ctx, conditionSetKey, nil))
		},
	},
}

// unkeyedConditionSetCalls are calls of each method of the condition sets that takes no key of a
// set.
var unkeyedConditionSetCalls = []conditionSetCall{
	{
		name: "List", op: opListConditionSets, route: listConditionSetsRoute,
		reply: mockpermit.Reply{Body: `[]`},
		call: func(ctx context.Context, c *Client, _ string) error {
			_, err := c.ConditionSets.List(ctx, nil)
			return err
		},
	},
	{
		name: "All", op: opListConditionSets, route: listConditionSetsRoute,
		reply: mockpermit.Reply{Body: `[]`},
		call: func(ctx context.Context, c *Client, _ string) error {
			return firstError(c.ConditionSets.All(ctx, nil))
		},
	},
	{
		name: "Create", op: opCreateConditionSet, route: createConditionSetRoute,
		reply: mockpermit.Reply{Body: conditionSetBody},
		call: func(ctx context.Context, c *Client, _ string) error {
			_, err := c.ConditionSets.Create(ctx, models.ConditionSetCreate{Key: "k", Name: "K"})
			return err
		},
	},
}

// everyConditionSetCall returns each call of the condition sets, keyed or not.
func everyConditionSetCall() []conditionSetCall {
	return append(append([]conditionSetCall{}, keyedConditionSetCalls...),
		unkeyedConditionSetCalls...)
}

// firstError ranges over seq to its end and returns the first error that it yields, or nil.
func firstError[T any](seq iter.Seq2[T, error]) error {
	for _, err := range seq {
		if err != nil {
			return err
		}
	}
	return nil
}

// TestConditionSetsListReadsBothForms checks that List and All read the condition sets of a reply
// that is a JSON array, the form that v1.2.8 and earlier read as an empty list, and of one that is
// the paginated object, each with three sets, in the order of the reply, with the counts of the
// paginated object alone; and that an empty array and an empty paginated object give a page
// without sets, whose list is empty and not nil, and an iterator that yields no set. All asks for
// the page after a JSON array of three sets, and not after a paginated object whose three sets
// reach its total_count.
func TestConditionSetsListReadsBothForms(t *testing.T) {
	three := setsOf(0, 3)
	empty := mockpermit.Reply{Body: `[]`}
	for _, tc := range []struct {
		name  string
		reply mockpermit.Reply
		next  []mockpermit.Reply
		want  *Page[models.ConditionSetRead]
	}{
		{
			name: "a JSON array", reply: arrayOf(t, three), next: []mockpermit.Reply{empty},
			want: &Page[models.ConditionSetRead]{Data: three, Page: 1},
		},
		{
			name:  "the paginated object",
			reply: pageOf(t, three, `,"total_count":3,"page_count":3`),
			want: &Page[models.ConditionSetRead]{
				Data: three, Page: 1, TotalCount: new(3), PageCount: new(3),
			},
		},
		{
			name: "an empty array", reply: empty,
			want: &Page[models.ConditionSetRead]{Data: []models.ConditionSetRead{}, Page: 1},
		},
		{
			name: "an empty paginated object", reply: mockpermit.Reply{
				Body: `{"data":[],"total_count":0}`,
			},
			want: &Page[models.ConditionSetRead]{
				Data: []models.ConditionSetRead{}, Page: 1, TotalCount: new(0),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listConditionSetsRoute, append([]mockpermit.Reply{tc.reply, tc.reply},
				tc.next...)...)
			c := resourceClient(t, api)
			got, err := c.ConditionSets.List(t.Context(), nil)
			if err != nil {
				t.Fatalf("List returned %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" || got.Data == nil {
				t.Errorf("List returned a page that differs (-want +got), or nil Data:\n%s", diff)
			}
			items, errs := collect(t, c.ConditionSets.All(t.Context(), nil))
			if len(items) != len(tc.want.Data) || len(items) > 0 && !cmp.Equal(three, items) ||
				errs != nil {
				t.Errorf("All yielded %+v and the errors %v, want the %d sets of the page",
					items, errs, len(tc.want.Data))
			}
		})
	}
}

// TestConditionSetsAllReadsEveryPage checks that All reads the 31 condition sets of a list, in
// order, asking for each page in turn with the filters of its options: at a per_page of 30, two
// pages of sets and the empty page after them; at the default per_page of 100, the most that the
// API gives on a page, one page of 31 and the empty page after it; and, when the replies are
// paginated objects whose total_count the two pages reach, the two pages alone. AncestorsAll and
// DescendantsAll read the 31 ancestors or descendants of a set in the same way, from JSON arrays.
func TestConditionSetsAllReadsEveryPage(t *testing.T) {
	type allCall func(c *Client) iter.Seq2[models.ConditionSetRead, error]
	for _, tc := range []struct {
		name    string
		route   string
		call    allCall
		pages   [][]models.ConditionSetRead
		counts  string
		queries []string
	}{
		{
			name: "All at per_page 30", route: listConditionSetsRoute,
			call: func(c *Client) iter.Seq2[models.ConditionSetRead, error] {
				return c.ConditionSets.All(t.Context(), &ConditionSetsListOptions{
					ListOptions: ListOptions{PerPage: 30}, Type: models.ConditionSetTypeUserSet,
				})
			},
			pages: [][]models.ConditionSetRead{setsOf(0, 30), setsOf(30, 31), {}},
			queries: []string{
				"page=1&per_page=30&type=userset", "page=2&per_page=30&type=userset",
				"page=3&per_page=30&type=userset",
			},
		},
		{
			name: "All at the default per_page", route: listConditionSetsRoute,
			call: func(c *Client) iter.Seq2[models.ConditionSetRead, error] {
				return c.ConditionSets.All(t.Context(), &ConditionSetsListOptions{Search: "s"})
			},
			pages:   [][]models.ConditionSetRead{setsOf(0, 31), {}},
			queries: []string{"page=1&per_page=100&search=s", "page=2&per_page=100&search=s"},
		},
		{
			name: "All of paginated replies", route: listConditionSetsRoute,
			call: func(c *Client) iter.Seq2[models.ConditionSetRead, error] {
				return c.ConditionSets.All(t.Context(),
					&ConditionSetsListOptions{ListOptions: ListOptions{PerPage: 30}})
			},
			pages:   [][]models.ConditionSetRead{setsOf(0, 30), setsOf(30, 31)},
			counts:  `,"total_count":31`,
			queries: []string{"page=1&per_page=30", "page=2&per_page=30"},
		},
		{
			name: "AncestorsAll", route: conditionSetAncestorsRoute,
			call: func(c *Client) iter.Seq2[models.ConditionSetRead, error] {
				return c.ConditionSets.AncestorsAll(t.Context(), "leaf", &ListOptions{PerPage: 30})
			},
			pages: [][]models.ConditionSetRead{setsOf(0, 30), setsOf(30, 31), {}},
			queries: []string{
				"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30",
			},
		},
		{
			name: "DescendantsAll", route: conditionSetDescendantsRoute,
			call: func(c *Client) iter.Seq2[models.ConditionSetRead, error] {
				return c.ConditionSets.DescendantsAll(t.Context(), "root", nil)
			},
			pages:   [][]models.ConditionSetRead{setsOf(0, 31), {}},
			queries: []string{"page=1&per_page=100", "page=2&per_page=100"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			var replies []mockpermit.Reply
			for _, items := range tc.pages {
				replies = append(replies, setsReply(t, items, tc.counts))
			}
			api.Handle(tc.route, replies...)
			items, errs := collect(t, tc.call(resourceClient(t, api)))
			if diff := cmp.Diff(setsOf(0, 31), items); diff != "" || errs != nil {
				t.Errorf("the iterator yielded the errors %v and sets that differ "+
					"(-want +got):\n%s", errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestConditionSetsAllYieldsAnErrorOnce checks that a page that fails ends the iterator with its
// error, once, after the sets of the pages before it; that a loop that stops early asks for no
// other page; and that the options of All are read when it is called: a later change to them
// changes no request.
func TestConditionSetsAllYieldsAnErrorOnce(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(listConditionSetsRoute, arrayOf(t, setsOf(0, 30)),
		mockpermit.Reply{Status: http.StatusForbidden, Body: body}, arrayOf(t, setsOf(0, 30)))
	opts := &ConditionSetsListOptions{ListOptions: ListOptions{PerPage: 30}, Search: "a"}
	seq := resourceClient(t, api).ConditionSets.All(t.Context(), opts)
	opts.PerPage, opts.Search, opts.Type = 50, "b", models.ConditionSetTypeResourceSet
	items, errs := collect(t, seq)
	if len(items) != 30 || len(errs) != 1 || !errors.Is(errs[0], ErrForbidden) {
		t.Errorf("All yielded %d sets and the errors %v, want 30 and one error that matches "+
			"ErrForbidden", len(items), errs)
	}
	for range seq {
		break
	}
	want := []string{
		"page=1&per_page=30&search=a", "page=2&per_page=30&search=a",
		"page=1&per_page=30&search=a",
	}
	if diff := cmp.Diff(want, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestConditionSetsAllGetsTheScopeInEachLoop checks that All, AncestorsAll and DescendantsAll,
// without WithScope, look up the scope in each loop over the iterator, before its first page,
// and not when they are called: a lookup that fails ends that loop with its error, and the next
// loop looks the scope up again.
func TestConditionSetsAllGetsTheScopeInEachLoop(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	sets := "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID + "/condition_sets"
	for _, tc := range []struct {
		route, path string
		call        func(c *Client) iter.Seq2[models.ConditionSetRead, error]
	}{
		{
			listConditionSetsRoute, sets,
			func(c *Client) iter.Seq2[models.ConditionSetRead, error] {
				return c.ConditionSets.All(t.Context(), nil)
			},
		},
		{
			conditionSetAncestorsRoute, sets + "/leaf/ancestors",
			func(c *Client) iter.Seq2[models.ConditionSetRead, error] {
				return c.ConditionSets.AncestorsAll(t.Context(), "leaf", nil)
			},
		},
		{
			conditionSetDescendantsRoute, sets + "/root/descendants",
			func(c *Client) iter.Seq2[models.ConditionSetRead, error] {
				return c.ConditionSets.DescendantsAll(t.Context(), "root", nil)
			},
		},
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, mockpermit.Reply{Status: http.StatusForbidden, Body: body},
			scopeReply(t, ""))
		api.Handle(tc.route, arrayOf(t, setsOf(0, 1)), arrayOf(t, setsOf(1, 1)))
		seq := tc.call(projectClient(t, api))
		if requests := api.Requests(); len(requests) != 0 {
			t.Fatalf("%s: the call sent %+v before a loop, want no request", tc.path, requests)
		}
		if items, errs := collect(t, seq); items != nil || len(errs) != 1 ||
			!errors.Is(errs[0], ErrForbidden) {
			t.Errorf("%s: the first loop yielded %v and the errors %v, want one error that "+
				"matches ErrForbidden", tc.path, items, errs)
		}
		if items, errs := collect(t, seq); !cmp.Equal(setsOf(0, 1), items) || errs != nil {
			t.Errorf("%s: the second loop yielded %v and the errors %v, want s0", tc.path, items,
				errs)
		}
		var paths []string
		for _, r := range api.Requests() {
			paths = append(paths, r.RawPath)
		}
		want := []string{"/v2/api-key/scope", "/v2/api-key/scope", tc.path, tc.path}
		if diff := cmp.Diff(want, paths); diff != "" {
			t.Errorf("the requests differ (-want +got):\n%s", diff)
		}
	}
}

// TestConditionSetListQueries checks the query of each list of the condition sets: List sends no
// query for nil options and for the zero options, so no empty type or search; the page and the
// per_page that it asks for; the type of its options, a known one or one that the SDK does not
// know, as it is; and the search, escaped. Ancestors and Descendants send no query for nil
// options, and the page and the per_page of theirs.
func TestConditionSetListQueries(t *testing.T) {
	type listCall func(ctx context.Context, c *Client) (*Page[models.ConditionSetRead], error)
	list := func(opts *ConditionSetsListOptions) listCall {
		return func(ctx context.Context, c *Client) (*Page[models.ConditionSetRead], error) {
			return c.ConditionSets.List(ctx, opts)
		}
	}
	for _, tc := range []struct {
		name, route, path, query string
		call                     listCall
		want                     *Page[models.ConditionSetRead]
	}{
		{
			name: "List with nil options", route: listConditionSetsRoute,
			path: conditionSetsPath, call: list(nil),
		},
		{
			name: "List with the zero options", route: listConditionSetsRoute,
			path: conditionSetsPath, call: list(&ConditionSetsListOptions{}),
		},
		{
			name: "List with every option", route: listConditionSetsRoute,
			path: conditionSetsPath, query: "page=3&per_page=7&search=a+b%26c&type=tenantset",
			call: list(&ConditionSetsListOptions{
				ListOptions: ListOptions{Page: 3, PerPage: 7}, Type: "tenantset",
				Search: "a b&c",
			}),
			want: &Page[models.ConditionSetRead]{
				Data: []models.ConditionSetRead{}, Page: 3, PerPage: 7,
			},
		},
		{
			name: "Ancestors with nil options", route: conditionSetAncestorsRoute,
			path: conditionSetsPath + "/leaf/ancestors",
			call: func(ctx context.Context, c *Client) (*Page[models.ConditionSetRead], error) {
				return c.ConditionSets.Ancestors(ctx, "leaf", nil)
			},
		},
		{
			name: "Descendants with a page", route: conditionSetDescendantsRoute,
			path: conditionSetsPath + "/root/descendants", query: "page=2&per_page=30",
			call: func(ctx context.Context, c *Client) (*Page[models.ConditionSetRead], error) {
				return c.ConditionSets.Descendants(ctx, "root", &ListOptions{Page: 2, PerPage: 30})
			},
			want: &Page[models.ConditionSetRead]{
				Data: []models.ConditionSetRead{}, Page: 2, PerPage: 30,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: `[]`})
			got, err := tc.call(t.Context(), resourceClient(t, api))
			if err != nil {
				t.Fatalf("the call returned %v", err)
			}
			want := tc.want
			if want == nil {
				want = &Page[models.ConditionSetRead]{Data: []models.ConditionSetRead{}, Page: 1}
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("the call returned a page that differs (-want +got):\n%s", diff)
			}
			if requests := api.Requests(); len(requests) != 1 ||
				requests[0].RawPath != tc.path || requests[0].RawQuery != tc.query {
				t.Errorf("the call sent %+v, want one request to %s?%s", requests, tc.path,
					tc.query)
			}
		})
	}
}

// TestInvalidConditionSetListOptions checks that a Page or a PerPage less than zero is an error
// that matches ErrInvalidConfig from each list of the condition sets and each iterator, and that
// none sends a request.
func TestInvalidConditionSetListOptions(t *testing.T) {
	for _, opts := range []ListOptions{{Page: -1}, {PerPage: -1}} {
		api := mockpermit.NewAPI(t)
		c := resourceClient(t, api)
		ctx := t.Context()
		_, listErr := c.ConditionSets.List(ctx, &ConditionSetsListOptions{ListOptions: opts})
		_, ancestorsErr := c.ConditionSets.Ancestors(ctx, "s", &opts)
		_, descendantsErr := c.ConditionSets.Descendants(ctx, "s", &opts)
		errs := []error{listErr, ancestorsErr, descendantsErr}
		for _, seq := range []iter.Seq2[models.ConditionSetRead, error]{
			c.ConditionSets.All(ctx, &ConditionSetsListOptions{ListOptions: opts}),
			c.ConditionSets.AncestorsAll(ctx, "s", &opts),
			c.ConditionSets.DescendantsAll(ctx, "s", &opts),
		} {
			_, yielded := collect(t, seq)
			if len(yielded) != 1 {
				t.Errorf("%+v: an iterator yielded the errors %v, want one", opts, yielded)
			}
			errs = append(errs, yielded...)
		}
		for i, err := range errs {
			if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("%+v: call %d returned %v, want an error that matches "+
					"ErrInvalidConfig", opts, i, err)
			}
		}
		api.AssertUntouched()
	}
}

// TestConditionSetsUseTheScope checks that the calls of the condition sets act on the
// environment of the client's scope. Without WithScope, the client looks up the scope of its API
// key once, and each call fills the route with the project and the environment of an
// environment-level key. A scope without an environment, that of a project-level or an
// organization-level key, is an error that matches ErrInvalidConfig and names the operation, and
// no call sends a request but the lookup, which the API fails for any route that the test did
// not register.
func TestConditionSetsUseTheScope(t *testing.T) {
	const prefix = "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID + "/condition_sets"
	t.Run("an environment-level key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		replies := map[string][]mockpermit.Reply{}
		for _, m := range everyConditionSetCall() {
			replies[m.route] = append(replies[m.route], m.reply)
		}
		for route, r := range replies {
			api.Handle(route, r...)
		}
		c := projectClient(t, api)
		for _, m := range everyConditionSetCall() {
			if err := m.call(t.Context(), c, "adults"); err != nil {
				t.Errorf("%s returned %v", m.name, err)
			}
		}
		requests := api.Requests()
		if len(requests) != len(everyConditionSetCall())+1 ||
			requests[0].RawPath != "/v2/api-key/scope" {
			t.Fatalf("the calls sent %+v, want the lookup of the scope and one request for "+
				"each call", requests)
		}
		for _, r := range requests[1:] {
			if r.RawPath != prefix && !strings.HasPrefix(r.RawPath, prefix+"/adults") {
				t.Errorf("a call sent its request to %s, want a path under %s", r.RawPath, prefix)
			}
		}
	})
	for _, suffix := range []string{".project", ".organization"} {
		t.Run("the scope of the fixture"+suffix, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(scopeRoute, scopeReply(t, suffix))
			c := projectClient(t, api)
			for _, m := range everyConditionSetCall() {
				err := m.call(t.Context(), c, "adults")
				if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), m.op) {
					t.Errorf("%s returned %v, want an error that matches ErrInvalidConfig and "+
						"names %s", m.name, err, m.op)
				}
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("the calls sent %d requests, want only the lookup of the scope", n)
			}
		})
	}
}

// TestConditionSetKeysInThePath checks that each method of the condition sets that acts on one
// set sends its key as one segment of the path, escaped, after the project and the environment of
// the scope, whatever the key holds: a slash, a percent sign, a space, a question mark and a hash,
// a letter outside ASCII, the shape of a UUID and a leading dot. A key that is empty, . or ..
// would change the route, so it is an error that matches ErrInvalidConfig and names the
// placeholder, and no request.
func TestConditionSetKeysInThePath(t *testing.T) {
	for key, escaped := range map[string]string{
		"a/b":            "a%2Fb",
		"50%":            "50%25",
		"a b?c#d":        "a%20b%3Fc%23d",
		"é":              "%C3%A9",
		fixtures.UUIDKey: fixtures.UUIDKey,
		".hidden":        ".hidden",
	} {
		for _, m := range keyedConditionSetCalls {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, m.reply)
			if err := m.call(t.Context(), resourceClient(t, api), key); err != nil {
				t.Errorf("%s(%q) returned %v", m.name, key, err)
			}
			_, suffix, _ := strings.Cut(m.route, "{condition_set_id}")
			want := conditionSetsPath + "/" + escaped + suffix
			if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want {
				t.Errorf("%s(%q) sent %+v, want one request to %s", m.name, key, requests, want)
			}
		}
	}
	for _, key := range []string{"", ".", ".."} {
		for _, m := range keyedConditionSetCalls {
			api := mockpermit.NewAPI(t)
			err := m.call(t.Context(), resourceClient(t, api), key)
			named := err != nil && strings.Contains(err.Error(), "{condition_set_id}")
			if !errors.Is(err, ErrInvalidConfig) || !named {
				t.Errorf("%s(%q) returned %v, want an error that matches ErrInvalidConfig and "+
					"names {condition_set_id}", m.name, key, err)
			}
			api.AssertUntouched()
		}
	}
}

// TestDeleteConditionSetThatIsGone checks that Delete returns nil for a 404, with or without a
// body, as the set that the path names is gone already, after one request; and an *Error with
// the status for any other error status, such as 403, 409 or 422.
func TestDeleteConditionSetThatIsGone(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"NOT_FOUND","title":"Not Found","message":"no such condition set"}`, "",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteConditionSetRoute,
			mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		if err := resourceClient(t, api).ConditionSets.Delete(t.Context(), "gone"); err != nil {
			t.Errorf("Delete of a set that the API answers with 404 and the body %q returned "+
				"%v, want nil", body, err)
		}
		if n := len(api.Requests()); n != 1 {
			t.Errorf("Delete sent %d requests, want 1", n)
		}
	}
	for _, status := range []int{
		http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteConditionSetRoute,
			mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := resourceClient(t, api).ConditionSets.Delete(t.Context(), "adults")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteConditionSet ||
			e.Detail != "no" || errors.Is(err, ErrNotFound) {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status and the detail", status, err, opDeleteConditionSet)
		}
	}
}

// TestNotFoundOfOtherConditionSetCalls checks that a 404 to each call of the condition sets but
// Delete, which deletes nothing, is an *Error that matches ErrNotFound, also from an iterator.
func TestNotFoundOfOtherConditionSetCalls(t *testing.T) {
	const body = `{"error_code":"NOT_FOUND","title":"Not Found","message":"no such object"}`
	for _, m := range everyConditionSetCall() {
		if m.op == opDeleteConditionSet {
			continue
		}
		api := mockpermit.NewAPI(t)
		api.Handle(m.route, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		err := m.call(t.Context(), resourceClient(t, api), "gone")
		e := mustError(t, err)
		if !errors.Is(err, ErrNotFound) || e.StatusCode != http.StatusNotFound || e.Op != m.op {
			t.Errorf("%s of an object that the API answers with 404 returned %v, want an "+
				"*Error of %s that matches ErrNotFound", m.name, err, m.op)
		}
	}
}

// TestConditionSetBodies checks the bodies of Create and Update byte for byte against the golden
// files in testdata/golden: a create with the required fields only; one of a resource set with
// every field, conditions whose values an encoder may change, a number beyond 2^53, a line
// separator and <, > and &, and a type that the SDK does not know with a resource, which it sends
// as it is; the zero update, which sends {}; an update with empty conditions, which sends {}; and
// one that sets every field.
func TestConditionSetBodies(t *testing.T) {
	description := "<a>&" + fixtures.LineSeparator + "b"
	conditions := models.Attributes{"allOf": []any{map[string]any{"allOf": []any{
		map[string]any{"resource.size": map[string]any{"less-than": fixtures.TwoPow53Plus1}},
		map[string]any{"resource.title": map[string]any{"equals": description}},
	}}}}
	create := func(set models.ConditionSetCreate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.ConditionSets.Create(t.Context(), set)
			return err
		}
	}
	update := func(change models.ConditionSetUpdate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.ConditionSets.Update(t.Context(), "s", change)
			return err
		}
	}
	for _, tc := range []struct {
		golden string
		route  string
		call   func(c *Client) error
	}{
		{
			"create_condition_set.required.golden", createConditionSetRoute,
			create(models.ConditionSetCreate{Key: "s", Name: "S"}),
		},
		{
			"create_condition_set.every-field.golden", createConditionSetRoute,
			create(models.ConditionSetCreate{
				Key: "s", Type: models.ConditionSetTypeResourceSet, Autogenerated: true,
				ResourceID: "document", Name: "S", Description: description,
				Conditions: conditions, ParentID: fixtures.UUIDKey,
			}),
		},
		{
			"create_condition_set.unknown-type.golden", createConditionSetRoute,
			create(models.ConditionSetCreate{
				Key: "s", Type: "tenantset", ResourceID: "document", Name: "S",
			}),
		},
		{
			"update_condition_set.unset.golden", updateConditionSetRoute,
			update(models.ConditionSetUpdate{}),
		},
		{
			"update_condition_set.empty-conditions.golden", updateConditionSetRoute,
			update(models.ConditionSetUpdate{Conditions: models.Attributes{}}),
		},
		{
			"update_condition_set.values.golden", updateConditionSetRoute,
			update(models.ConditionSetUpdate{
				Name: "S2", Description: description, Conditions: conditions,
				ParentID: fixtures.UUIDKey,
			}),
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: conditionSetBody})
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

// TestUserSetWithAResource checks that a create of a user set with a resource, whose Type is
// empty or ConditionSetTypeUserSet and whose ResourceID is not empty, is an error that matches
// ErrInvalidConfig with the message of its rule, which names the operation, and sends no request,
// not even the lookup of the scope, as a user set takes no resource; and that a create of a user
// set without a resource and of a resource set with one each send their request.
func TestUserSetWithAResource(t *testing.T) {
	for _, typ := range []models.ConditionSetType{"", models.ConditionSetTypeUserSet} {
		api := mockpermit.NewAPI(t)
		_, err := projectClient(t, api).ConditionSets.Create(t.Context(),
			models.ConditionSetCreate{Key: "s", Type: typ, ResourceID: "document", Name: "S"})
		want := (&configError{
			setting: opCreateConditionSet + ": ConditionSetCreate",
			rule:    validate.ConditionSetUserSetResource,
		}).Error()
		if !errors.Is(err, ErrInvalidConfig) || err.Error() != want {
			t.Errorf("Create of a user set of the type %q with a resource returned %v, want %s",
				typ, err, want)
		}
		api.AssertUntouched()
	}
	for _, set := range []models.ConditionSetCreate{
		{Key: "s", Type: models.ConditionSetTypeUserSet, Name: "S"},
		{Key: "s", Type: models.ConditionSetTypeResourceSet, ResourceID: "document", Name: "S"},
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(createConditionSetRoute, mockpermit.Reply{Body: conditionSetBody})
		if _, err := resourceClient(t, api).ConditionSets.Create(t.Context(), set); err != nil {
			t.Errorf("Create of %+v returned %v", set, err)
		}
	}
}

// TestConditionSetBodiesThatCannotBeJSON checks that a create and an update whose conditions hold
// a value that JSON cannot hold are errors that match ErrInvalidConfig and name the operation,
// and send no request.
func TestConditionSetBodiesThatCannotBeJSON(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := resourceClient(t, api)
	_, createErr := c.ConditionSets.Create(t.Context(), models.ConditionSetCreate{
		Key: "s", Name: "S", Conditions: models.Attributes{"c": make(chan int)},
	})
	_, updateErr := c.ConditionSets.Update(t.Context(), "s", models.ConditionSetUpdate{
		Conditions: models.Attributes{"f": func() {}},
	})
	for op, err := range map[string]error{
		opCreateConditionSet: createErr, opUpdateConditionSet: updateErr,
	} {
		if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), op) {
			t.Errorf("%s returned %v, want an error that matches ErrInvalidConfig and names it",
				op, err)
		}
	}
	api.AssertUntouched()
}

// TestConditionSetRetries checks when a call of the condition sets sends its request again:
// Create, which the API may have acted on, after a 429 only, so a 503 ends it after one request;
// each other call, which does the same when sent twice, after a 503 too.
func TestConditionSetRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	create := unkeyedConditionSetCalls[2]
	type retryCase struct {
		name    string
		m       conditionSetCall
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
	for _, m := range everyConditionSetCall() {
		if m.op != opCreateConditionSet {
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
			err := tc.m.call(t.Context(), c, "adults")
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

// TestMalformedConditionSetReplies checks that a reply in 200-299 that is not a condition set,
// to each method that returns one, is an *Error that names the operation, its route and the
// model, and no set; and that a reply to List and All that is neither a JSON array nor the
// paginated object of sets, such as an empty body, null, an object without data, a data of null
// or an error body in a 200, and a reply to the ancestors and the descendants that is not a JSON
// array of sets, is such an *Error too, and no page and no set of the iterator, never an empty
// list.
func TestMalformedConditionSetReplies(t *testing.T) {
	const prefix = "permit: "
	type setCall func(ctx context.Context, c *Client) (*models.ConditionSetRead, error)
	for _, m := range []struct {
		name, op, route string
		call            setCall
	}{
		{
			"Get", opGetConditionSet, getConditionSetRoute,
			func(ctx context.Context, c *Client) (*models.ConditionSetRead, error) {
				return c.ConditionSets.Get(ctx, "adults")
			},
		},
		{
			"Create", opCreateConditionSet, createConditionSetRoute,
			func(ctx context.Context, c *Client) (*models.ConditionSetRead, error) {
				return c.ConditionSets.Create(ctx, models.ConditionSetCreate{Key: "a", Name: "A"})
			},
		},
		{
			"Update", opUpdateConditionSet, updateConditionSetRoute,
			func(ctx context.Context, c *Client) (*models.ConditionSetRead, error) {
				return c.ConditionSets.Update(ctx, "adults", models.ConditionSetUpdate{Name: "A"})
			},
		},
	} {
		want := prefix + m.op + ": " + m.route + ": status 200: the reply is not a valid " +
			"models.ConditionSetRead"
		for _, body := range []string{``, `null`, `[]`, `"a"`, `{"conditions":[]}`, `{} {}`} {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, mockpermit.Reply{Body: body})
			set, err := m.call(t.Context(), resourceClient(t, api))
			if e := mustError(t, err); set != nil || e.Error() != want {
				t.Errorf("%s of the reply %q returned %+v and\n%v\nwant no set and\n%s", m.name,
					body, set, err, want)
			}
		}
	}
	type listCall func(ctx context.Context, c *Client) (*Page[models.ConditionSetRead], error)
	type allCall func(ctx context.Context, c *Client) iter.Seq2[models.ConditionSetRead, error]
	for _, m := range []struct {
		op, route string
		bodies    []string
		list      listCall
		all       allCall
	}{
		{
			opListConditionSets, listConditionSetsRoute,
			[]string{
				``, `null`, `{}`, `{"data":null}`, `{"detail":"Not authenticated"}`, `"a"`,
				`[1]`, `[{"conditions":"a"}]`, `{"data":{}}`, `[] []`,
			},
			func(ctx context.Context, c *Client) (*Page[models.ConditionSetRead], error) {
				return c.ConditionSets.List(ctx, nil)
			},
			func(ctx context.Context, c *Client) iter.Seq2[models.ConditionSetRead, error] {
				return c.ConditionSets.All(ctx, nil)
			},
		},
		{
			opConditionSetAncestors, conditionSetAncestorsRoute,
			[]string{``, `null`, `{}`, `{"data":[]}`, `[{"key":1}]`},
			func(ctx context.Context, c *Client) (*Page[models.ConditionSetRead], error) {
				return c.ConditionSets.Ancestors(ctx, "leaf", nil)
			},
			func(ctx context.Context, c *Client) iter.Seq2[models.ConditionSetRead, error] {
				return c.ConditionSets.AncestorsAll(ctx, "leaf", nil)
			},
		},
		{
			opConditionSetDescendants, conditionSetDescendantsRoute,
			[]string{``, `null`, `{}`, `{"data":[]}`, `[{"key":1}]`},
			func(ctx context.Context, c *Client) (*Page[models.ConditionSetRead], error) {
				return c.ConditionSets.Descendants(ctx, "root", nil)
			},
			func(ctx context.Context, c *Client) iter.Seq2[models.ConditionSetRead, error] {
				return c.ConditionSets.DescendantsAll(ctx, "root", nil)
			},
		},
	} {
		want := prefix + m.op + ": " + m.route + ": status 200: the reply is not a valid " +
			"list of models.ConditionSetRead"
		for _, body := range m.bodies {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, mockpermit.Reply{Body: body}, mockpermit.Reply{Body: body})
			c := resourceClient(t, api)
			page, err := m.list(t.Context(), c)
			if e := mustError(t, err); page != nil || e.Error() != want {
				t.Errorf("the list of the reply %q returned %+v and\n%v\nwant no page and\n%s",
					body, page, err, want)
			}
			items, errs := collect(t, m.all(t.Context(), c))
			if items != nil || len(errs) != 1 || mustError(t, errs[0]).Error() != want {
				t.Errorf("the iterator of the reply %q yielded %+v and %v, want only the "+
					"error\n%s", body, items, errs, want)
			}
		}
	}
}

// TestConditionSetsAfterClose checks that each method returns ErrClosed after Close, and sends
// nothing, also without WithScope, whose lookup it does not send either, and also for a create
// of a user set with a resource, which it does not check.
func TestConditionSetsAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	_, allErrs := collect(t, c.ConditionSets.All(t.Context(), nil))
	_, createErr := c.ConditionSets.Create(t.Context(),
		models.ConditionSetCreate{Key: "s", Name: "S", ResourceID: "document"})
	errs := []error{createErr}
	for _, m := range everyConditionSetCall() {
		errs = append(errs, m.call(t.Context(), c, "adults"))
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
