package permit

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/internal/transport"
	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The routes of the resource instances on the API, under the scope p and e of instanceClient, as
// mockpermit.Server.Handle takes them.
const (
	listInstancesRoute    = "GET /v2/facts/{proj_id}/{env_id}/resource_instances"
	createInstanceRoute   = "POST /v2/facts/{proj_id}/{env_id}/resource_instances"
	detailedInstanceRoute = "GET /v2/facts/{proj_id}/{env_id}/resource_instances/detailed"
	getInstanceRoute      = "GET /v2/facts/{proj_id}/{env_id}/resource_instances/{instance_id}"
	updateInstanceRoute   = "PATCH /v2/facts/{proj_id}/{env_id}/resource_instances/{instance_id}"
	deleteInstanceRoute   = "DELETE /v2/facts/{proj_id}/{env_id}/resource_instances/{instance_id}"
)

// instanceBody is the body of a reply of the API that holds the instance document:doc-1.
const instanceBody = `{"key":"doc-1","tenant":"tenant-a","resource":"document",` +
	`"id":"00000000-0000-4000-8000-000000000401",` +
	`"organization_id":"00000000-0000-4000-8000-000000000001",` +
	`"project_id":"00000000-0000-4000-8000-000000000002",` +
	`"environment_id":"00000000-0000-4000-8000-000000000003",` +
	`"created_at":"2026-10-14T09:30:00Z","updated_at":"2026-10-14T09:30:00Z",` +
	`"resource_id":"00000000-0000-4000-8000-000000000021",` +
	`"tenant_id":"00000000-0000-4000-8000-000000000037","attributes":{}}`

// instanceClient returns a client of api with the scope p and e and opts, whose PDP is a server
// that fails the test when anything reaches it.
func instanceClient(t *testing.T, api *mockpermit.Server, opts ...Option) *Client {
	t.Helper()
	pdp := mockpermit.NewPDP(t)
	t.Cleanup(pdp.AssertUntouched)
	return decisionClient(t, api, pdp, append([]Option{WithScope("p", "e")}, opts...)...)
}

// instancesOf returns the instances document:i<from> to document:i<to-1>, with only a key and a
// resource: an empty list when to is from. A reply of the detailed list holds them as well.
func instancesOf(from, to int) []models.ResourceInstanceRead {
	items := make([]models.ResourceInstanceRead, 0, to-from)
	for i := from; i < to; i++ {
		items = append(items, models.ResourceInstanceRead{
			Key: "i" + strconv.Itoa(i), Resource: "document",
		})
	}
	return items
}

// instanceLists are the two lists of the instances, as their routes and iterators: List and
// All, and ListDetailed and AllDetailed.
var instanceLists = []struct {
	name  string
	route string
	all   func(ctx context.Context, s *ResourceInstancesService,
		opts *ResourceInstancesListOptions) iter.Seq2[string, error]
}{
	{
		name: "All", route: listInstancesRoute,
		all: func(ctx context.Context, s *ResourceInstancesService,
			opts *ResourceInstancesListOptions,
		) iter.Seq2[string, error] {
			return instanceKeys(s.All(ctx, opts), func(i models.ResourceInstanceRead) string {
				return i.Key
			})
		},
	},
	{
		name: "AllDetailed", route: detailedInstanceRoute,
		all: func(ctx context.Context, s *ResourceInstancesService,
			opts *ResourceInstancesListOptions,
		) iter.Seq2[string, error] {
			return instanceKeys(s.AllDetailed(ctx, opts),
				func(i models.ResourceInstanceDetailedRead) string { return i.Key })
		},
	},
}

// instanceKeys returns an iterator over the key of each instance that seq yields, which key
// reads, and over its errors, with an empty key.
func instanceKeys[T any](seq iter.Seq2[T, error], key func(T) string) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for item, err := range seq {
			k := ""
			if err == nil {
				k = key(item)
			}
			if !yield(k, err) {
				return
			}
		}
	}
}

// instanceKeysOf returns the keys of the instances i<from> to i<to-1>.
func instanceKeysOf(from, to int) []string {
	var keys []string
	for _, instance := range instancesOf(from, to) {
		keys = append(keys, instance.Key)
	}
	return keys
}

// TestResourceInstancesAllReadsEveryPage checks that All and AllDetailed read the 31 instances of
// a list, in order, asking for each page in turn: at a per_page of 30, two paginated objects
// without a page_count and the empty page after them; at the default per_page of 100, one page
// of 31 and the empty page after it; and pages up to the page_count that they report, without
// the page after it. All also reads the pages of JSON arrays, up to the first empty one.
func TestResourceInstancesAllReadsEveryPage(t *testing.T) {
	perPage30 := &ResourceInstancesListOptions{ListOptions: ListOptions{PerPage: 30}}
	untilEmpty := []string{"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30"}
	for _, list := range instanceLists {
		for _, tc := range []struct {
			name    string
			opts    *ResourceInstancesListOptions
			counts  string
			bounds  []int
			queries []string
		}{
			{
				name: "per_page 30", opts: perPage30, queries: untilEmpty,
				counts: `,"total_count":31`, bounds: []int{0, 30, 31, 31},
			},
			{
				name:    "the default per_page",
				queries: []string{"page=1&per_page=100", "page=2&per_page=100"},
				counts:  `,"total_count":31`, bounds: []int{0, 31, 31},
			},
			{
				name: "a page_count", opts: perPage30,
				queries: []string{"page=1&per_page=30", "page=2&per_page=30"},
				counts:  `,"total_count":31,"page_count":2`, bounds: []int{0, 30, 31},
			},
		} {
			t.Run(list.name+"/"+tc.name, func(t *testing.T) {
				api := mockpermit.NewAPI(t)
				api.Handle(list.route, instancePagesOf(t, tc.counts, tc.bounds...)...)
				keys, errs := collect(t, list.all(t.Context(),
					instanceClient(t, api).ResourceInstances, tc.opts))
				if diff := cmp.Diff(instanceKeysOf(0, 31), keys); diff != "" || errs != nil {
					t.Errorf("%s yielded the errors %v and keys that differ (-want +got):\n%s",
						list.name, errs, diff)
				}
				if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
					t.Errorf("the queries differ (-want +got):\n%s", diff)
				}
			})
		}
	}
	t.Run("All/arrays", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(listInstancesRoute, arrayOf(t, instancesOf(0, 30)),
			arrayOf(t, instancesOf(30, 31)),
			arrayOf(t, []models.ResourceInstanceRead{}))
		keys, errs := collect(t, instanceLists[0].all(t.Context(),
			instanceClient(t, api).ResourceInstances, perPage30))
		if diff := cmp.Diff(instanceKeysOf(0, 31), keys); diff != "" || errs != nil {
			t.Errorf("All yielded the errors %v and keys that differ (-want +got):\n%s", errs,
				diff)
		}
		if diff := cmp.Diff(untilEmpty, queries(api)); diff != "" {
			t.Errorf("the queries differ (-want +got):\n%s", diff)
		}
	})
}

// instancePagesOf returns a paginated reply for each pair of bounds, the instances i<from> to
// i<to-1>, each with counts, as pageOf builds it.
func instancePagesOf(t *testing.T, counts string, bounds ...int) []mockpermit.Reply {
	t.Helper()
	var replies []mockpermit.Reply
	for i := 0; i+1 < len(bounds); i++ {
		replies = append(replies,
			pageOf(t, instancesOf(bounds[i], bounds[i+1]), counts))
	}
	return replies
}

// TestResourceInstancesAllReadsTheOptionsOnce checks that All and AllDetailed keep the options of
// their call, the filters too, so that a later change to them changes no request; that a loop
// that breaks asks for no other page; and that each loop reads the instances again from the
// first page, along the same path, through the API and through the PDP container.
func TestResourceInstancesAllReadsTheOptionsOnce(t *testing.T) {
	for _, list := range instanceLists {
		for _, viaPDP := range []bool{false, true} {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			to, route := api, list.route
			path, _ := strings.CutPrefix(route, "GET ")
			path = strings.Replace(path, "{proj_id}/{env_id}", "p/e", 1)
			opts := []Option{WithScope("p", "e")}
			if viaPDP {
				to, route = pdp, strings.Replace(route, "/v2/facts/{proj_id}/{env_id}", "/facts", 1)
				path = strings.Replace(path, "/v2/facts/p/e", "/facts", 1)
				opts = append(opts, WithFactsViaPDP())
			}
			two := pageOf(t, instancesOf(0, 2), `,"total_count":9`)
			to.Handle(route, two, two)
			options := &ResourceInstancesListOptions{
				ListOptions: ListOptions{PerPage: 2}, Tenant: "t", Resource: "document",
				Search: []string{"i"},
			}
			seq := list.all(t.Context(), decisionClient(t, api, pdp, opts...).ResourceInstances,
				options)
			options.Page, options.PerPage, options.Tenant, options.Search[0] = 5, 50, "x", "y"
			for loop := 1; loop <= 2; loop++ {
				for key, err := range seq {
					if err != nil || key != "i0" {
						t.Errorf("%s via the PDP %t: loop %d yielded %q and %v first, want i0",
							list.name, viaPDP, loop, key, err)
					}
					break
				}
			}
			var got []string
			for _, r := range to.Requests() {
				got = append(got, r.RawPath+"?"+r.RawQuery)
			}
			first := path + "?page=1&per_page=2&resource=document&search=i&tenant=t"
			if diff := cmp.Diff([]string{first, first}, got); diff != "" {
				t.Errorf("%s via the PDP %t: the requests differ (-want +got):\n%s", list.name,
					viaPDP, diff)
			}
		}
	}
}

// TestResourceInstanceKeysInThePath checks that Get, Update and Delete send the instance as one
// segment of the path, escaped, whatever it holds, with its : as it is. An instance that is
// empty, . or .. would change the route, so it is an error that matches ErrInvalidConfig and
// names the placeholder, and no request.
func TestResourceInstanceKeysInThePath(t *testing.T) {
	ok := mockpermit.Reply{Body: instanceBody}
	methods := []struct {
		name  string
		route string
		reply mockpermit.Reply
		call  func(c *Client, key string) error
	}{
		{
			name: "Get", route: getInstanceRoute, reply: ok,
			call: func(c *Client, key string) error {
				_, err := c.ResourceInstances.Get(t.Context(), key)
				return err
			},
		},
		{
			name: "Update", route: updateInstanceRoute, reply: ok,
			call: func(c *Client, key string) error {
				_, err := c.ResourceInstances.Update(t.Context(), key,
					models.ResourceInstanceUpdate{})
				return err
			},
		},
		{
			name: "Delete", route: deleteInstanceRoute,
			reply: mockpermit.Reply{Status: http.StatusNoContent},
			call: func(c *Client, key string) error {
				return c.ResourceInstances.Delete(t.Context(), key)
			},
		},
	}
	const instances = "/v2/facts/p/e/resource_instances/"
	for key, want := range map[string]string{
		"document:doc-1":                      instances + "document:doc-1",
		"document:a/b":                        instances + "document:a%2Fb",
		"document:50%":                        instances + "document:50%25",
		"a b?c#d;e,f":                         instances + "a%20b%3Fc%23d%3Be%2Cf",
		fixtures.UUIDKey:                      instances + fixtures.UUIDKey,
		".hidden":                             instances + ".hidden",
		"document:é" + fixtures.LineSeparator: instances + "document:%C3%A9%E2%80%A8",
	} {
		for _, m := range methods {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, m.reply)
			if err := m.call(instanceClient(t, api), key); err != nil {
				t.Errorf("%s(%q) returned %v", m.name, key, err)
			}
			if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want {
				t.Errorf("%s(%q) sent %+v, want one request to %s", m.name, key, requests, want)
			}
		}
	}
	for _, key := range []string{"", ".", ".."} {
		for _, m := range methods {
			api := mockpermit.NewAPI(t)
			err := m.call(instanceClient(t, api), key)
			if !errors.Is(err, ErrInvalidConfig) ||
				!strings.Contains(err.Error(), "{instance_id}") {
				t.Errorf("%s(%q) returned %v, want an error that matches ErrInvalidConfig and "+
					"names {instance_id}", m.name, key, err)
			}
			api.AssertUntouched()
		}
	}
}

// TestGetDetailedInstance checks that Get of the instance detailed, which names the route of
// the detailed list, is an error that matches ErrInvalidConfig with the rule
// path.instance-id.detailed, and sends no request, not even the lookup of the scope, through the
// API and through the PDP container; and that Update and Delete, whose routes have no such
// sibling, send it as any other instance.
func TestGetDetailedInstance(t *testing.T) {
	for _, opts := range [][]Option{nil, {WithFactsViaPDP()}} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		_, err := decisionClient(t, api, pdp, opts...).ResourceInstances.Get(t.Context(),
			detailedInstanceID)
		want := "permit: invalid configuration: get_resource_instance: {instance_id} of " +
			"/v2/facts/{proj_id}/{env_id}/resource_instances/{instance_id}: " +
			validate.PathInstanceIDDetailed.Message
		if !errors.Is(err, ErrInvalidConfig) || err.Error() != want {
			t.Errorf("Get(detailed) with %d options returned %v, want %q", len(opts), err, want)
		}
		api.AssertUntouched()
		pdp.AssertUntouched()
	}
	api := mockpermit.NewAPI(t)
	api.Handle(updateInstanceRoute, mockpermit.Reply{Body: instanceBody})
	api.Handle(deleteInstanceRoute, mockpermit.Reply{Status: http.StatusNoContent})
	c := instanceClient(t, api)
	if _, err := c.ResourceInstances.Update(t.Context(), detailedInstanceID,
		models.ResourceInstanceUpdate{}); err != nil {
		t.Errorf("Update(detailed) returned %v", err)
	}
	if err := c.ResourceInstances.Delete(t.Context(), detailedInstanceID); err != nil {
		t.Errorf("Delete(detailed) returned %v", err)
	}
	var got []string
	for _, r := range api.Requests() {
		got = append(got, r.Method+" "+r.RawPath)
	}
	want := []string{
		"PATCH /v2/facts/p/e/resource_instances/detailed",
		"DELETE /v2/facts/p/e/resource_instances/detailed",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("the requests differ (-want +got):\n%s", diff)
	}
}

// TestCreateInstanceWithoutTenant checks that Create of an instance without a tenant is an error
// that matches ErrInvalidConfig with the rule resource-instance.tenant.empty, and sends no
// request, not even the lookup of the scope of a client without WithScope, also through the PDP
// container and through a copy of WaitForSync.
func TestCreateInstanceWithoutTenant(t *testing.T) {
	want := "permit: invalid configuration: create_resource_instance: ResourceInstanceCreate: " +
		validate.ResourceInstanceTenantEmpty.Message
	for _, opts := range [][]Option{nil, {WithScope("p", "e")}, {WithFactsViaPDP()}} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		s := decisionClient(t, api, pdp, opts...).ResourceInstances
		for _, s := range []*ResourceInstancesService{s, s.WaitForSync(time.Second)} {
			got, err := s.Create(t.Context(), models.ResourceInstanceCreate{
				Key: "doc-1", Resource: "document", Attributes: models.Attributes{"a": 1},
			})
			if got != nil || !errors.Is(err, ErrInvalidConfig) || err.Error() != want {
				t.Errorf("Create without a tenant with %d options returned %+v, %v; want %q",
					len(opts), got, err, want)
			}
		}
		api.AssertUntouched()
		pdp.AssertUntouched()
	}
}

// TestDeleteResourceInstanceThatIsGone checks that Delete returns nil for a 404 of the API, with
// or without a body, as the instance that the path names is gone already, after one request; an
// *Error with the status for any other error status; and, through the PDP container, an error
// that matches ErrContainerPDPOnly for a 404 with an empty body, as the cloud PDP answers so,
// and nil for a 404 of the container with a body.
func TestDeleteResourceInstanceThatIsGone(t *testing.T) {
	for _, body := range []string{`{"detail":"the instance does not exist"}`, ""} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteInstanceRoute, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		err := instanceClient(t, api).ResourceInstances.Delete(t.Context(), "document:gone")
		if err != nil {
			t.Errorf("Delete of an instance that the API answers with 404 and the body %q "+
				"returned %v, want nil", body, err)
		}
		if n := len(api.Requests()); n != 1 {
			t.Errorf("Delete sent %d requests, want 1", n)
		}
	}
	for _, status := range []int{http.StatusForbidden, http.StatusUnprocessableEntity} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteInstanceRoute, mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := instanceClient(t, api).ResourceInstances.Delete(t.Context(), "document:doc-1")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteResourceInstance {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status", status, err, opDeleteResourceInstance)
		}
	}
	for body, wantContainer := range map[string]bool{"": true, `{"detail":"gone"}`: false} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle("DELETE /facts/resource_instances/{instance_id}",
			mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		err := decisionClient(t, api, pdp, WithFactsViaPDP()).ResourceInstances.Delete(
			t.Context(), "document:doc-1")
		if got := errors.Is(err, ErrContainerPDPOnly); got != wantContainer ||
			(!wantContainer && err != nil) {
			t.Errorf("Delete through the PDP of a 404 with the body %q returned %v, want an "+
				"error that matches ErrContainerPDPOnly: %t", body, err, wantContainer)
		}
		api.AssertUntouched()
	}
}

// TestResourceInstanceBodies checks the bodies of Create and Update byte for byte against the
// golden files in testdata/golden: an instance without attributes, which leaves them out; one
// with attributes of each kind of JSON value and text that an encoder may escape; one with the
// empty attributes {}; the zero update, which sends {} and changes nothing; an update that sends
// null for the attributes; and one that sets them to {}, which removes every attribute.
func TestResourceInstanceBodies(t *testing.T) {
	create := func(instance models.ResourceInstanceCreate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.ResourceInstances.Create(t.Context(), instance)
			return err
		}
	}
	update := func(change models.ResourceInstanceUpdate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.ResourceInstances.Update(t.Context(), "document:doc-1", change)
			return err
		}
	}
	doc1 := models.ResourceInstanceCreate{Key: "doc-1", Tenant: "tenant-a", Resource: "document"}
	every := doc1
	every.Tenant = "tenant <a> & b"
	every.Attributes = models.Attributes{
		"n": fixtures.TwoPow53Plus1, "list": []any{"a", 1}, "nested": map[string]any{},
		"text": "a" + fixtures.LineSeparator + "b",
	}
	empty := doc1
	empty.Attributes = models.Attributes{}
	for _, tc := range []struct {
		golden string
		route  string
		call   func(c *Client) error
	}{
		{"create_resource_instance.required.golden", createInstanceRoute, create(doc1)},
		{"create_resource_instance.every-field.golden", createInstanceRoute, create(every)},
		{"create_resource_instance.empty.golden", createInstanceRoute, create(empty)},
		{
			"update_resource_instance.unset.golden", updateInstanceRoute,
			update(models.ResourceInstanceUpdate{}),
		},
		{
			"update_resource_instance.null.golden", updateInstanceRoute,
			update(models.ResourceInstanceUpdate{Attributes: models.Null[models.Attributes]()}),
		},
		{
			"update_resource_instance.empty.golden", updateInstanceRoute,
			update(models.ResourceInstanceUpdate{
				Attributes: models.Some(models.Attributes{}),
			}),
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: instanceBody})
			if err := tc.call(instanceClient(t, api)); err != nil {
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

// TestResourceInstanceRetries checks when a call of the resource instances sends its request
// again: Create, which the API may have acted on, after a 429 only, so a 503 ends it after one
// request; Update, Get, Delete, List and ListDetailed, which do the same when sent twice, after a
// 503 too.
func TestResourceInstanceRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	ok := mockpermit.Reply{Body: instanceBody}
	emptyPage := mockpermit.Reply{Body: `{"data":[],"total_count":0}`}
	createInstance := func(c *Client) error {
		_, err := c.ResourceInstances.Create(t.Context(),
			models.ResourceInstanceCreate{Key: "doc-1", Tenant: "t", Resource: "document"})
		return err
	}
	for _, tc := range []struct {
		name    string
		route   string
		replies []mockpermit.Reply
		call    func(c *Client) error
		status  int
	}{
		{
			name: "Create after a 429", route: createInstanceRoute,
			replies: []mockpermit.Reply{limited, ok}, call: createInstance,
		},
		{
			name: "Create after a 503", route: createInstanceRoute,
			replies: []mockpermit.Reply{unavailable}, call: createInstance,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "Update after a 503", route: updateInstanceRoute,
			replies: []mockpermit.Reply{unavailable, ok},
			call: func(c *Client) error {
				_, err := c.ResourceInstances.Update(t.Context(), "document:doc-1",
					models.ResourceInstanceUpdate{})
				return err
			},
		},
		{
			name: "Get after a 503", route: getInstanceRoute,
			replies: []mockpermit.Reply{unavailable, ok},
			call: func(c *Client) error {
				_, err := c.ResourceInstances.Get(t.Context(), "document:doc-1")
				return err
			},
		},
		{
			name: "Delete after a 503", route: deleteInstanceRoute,
			replies: []mockpermit.Reply{unavailable, {Status: http.StatusNoContent}},
			call: func(c *Client) error {
				return c.ResourceInstances.Delete(t.Context(), "document:doc-1")
			},
		},
		{
			name: "List after a 503", route: listInstancesRoute,
			replies: []mockpermit.Reply{unavailable, emptyPage},
			call: func(c *Client) error {
				_, err := c.ResourceInstances.List(t.Context(), nil)
				return err
			},
		},
		{
			name: "ListDetailed after a 503", route: detailedInstanceRoute,
			replies: []mockpermit.Reply{unavailable, emptyPage},
			call: func(c *Client) error {
				_, err := c.ResourceInstances.ListDetailed(t.Context(), nil)
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, tc.replies...)
			err := tc.call(instanceClient(t, api, WithRetry(RetryPolicy{MaxRetries: 1})))
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

// The bodies of a reply in 200-299 that are not a list of instances: for List and All, which
// read a JSON array or the paginated object, and for ListDetailed and AllDetailed, which read
// the paginated object alone, a JSON array too.
var (
	malformedInstanceLists         = []string{``, `null`, `"i"`, `[1]`, `{"data":[1]}`}
	malformedDetailedInstanceLists = []string{
		``, `null`, `"i"`, `[]`, `[{"key":"i"}]`, `{"data":[1]}`,
	}
)

// TestMalformedResourceInstanceReplies checks that a reply in 200-299 that is not an instance, an
// empty body too, to Get, Create and Update, or for List not a list of instances, a JSON array or
// the paginated object, and for ListDetailed not the paginated object, a JSON array too, is an
// *Error that names the model, with the status of the reply, and no result.
func TestMalformedResourceInstanceReplies(t *testing.T) {
	calls := map[string]func(c *Client) (bool, error){
		getInstanceRoute: func(c *Client) (bool, error) {
			instance, err := c.ResourceInstances.Get(t.Context(), "document:doc-1")
			return instance != nil, err
		},
		createInstanceRoute: func(c *Client) (bool, error) {
			instance, err := c.ResourceInstances.Create(t.Context(),
				models.ResourceInstanceCreate{Key: "doc-1", Tenant: "t", Resource: "document"})
			return instance != nil, err
		},
		updateInstanceRoute: func(c *Client) (bool, error) {
			instance, err := c.ResourceInstances.Update(t.Context(), "document:doc-1",
				models.ResourceInstanceUpdate{})
			return instance != nil, err
		},
	}
	for route, call := range calls {
		for _, body := range []string{``, ` `, `null`, `[]`, `"i"`, `{"key":1}`, `{} {}`} {
			api := mockpermit.NewAPI(t)
			api.Handle(route, mockpermit.Reply{Body: body})
			got, err := call(instanceClient(t, api))
			want := "status 200: the reply is not a valid models.ResourceInstanceRead"
			if e := mustError(t, err); got || e.StatusCode != http.StatusOK ||
				!strings.HasSuffix(e.Error(), want) {
				t.Errorf("%s of the reply %q returned a result: %t, and\n%v\nwant no result and "+
					"an error that ends with %q", route, body, got, err, want)
			}
		}
	}
	lists := []struct {
		route  string
		bodies []string
		model  string
		call   func(c *Client) (bool, error)
	}{
		{
			route: listInstancesRoute, model: "models.ResourceInstanceRead",
			bodies: malformedInstanceLists,
			call: func(c *Client) (bool, error) {
				page, err := c.ResourceInstances.List(t.Context(), nil)
				return page != nil, err
			},
		},
		{
			route: detailedInstanceRoute, model: "models.ResourceInstanceDetailedRead",
			bodies: malformedDetailedInstanceLists,
			call: func(c *Client) (bool, error) {
				page, err := c.ResourceInstances.ListDetailed(t.Context(), nil)
				return page != nil, err
			},
		},
	}
	for _, list := range lists {
		for _, body := range list.bodies {
			api := mockpermit.NewAPI(t)
			api.Handle(list.route, mockpermit.Reply{Body: body})
			got, err := list.call(instanceClient(t, api))
			want := "status 200: the reply is not a valid list of " + list.model
			if e := mustError(t, err); got || !strings.HasSuffix(e.Error(), want) {
				t.Errorf("%s of the reply %q returned a page: %t, and\n%v\nwant no page and an "+
					"error that ends with %q", list.route, body, got, err, want)
			}
		}
	}
}

// TestResourceInstanceIteratorsStopAtAMalformedReply checks that All and AllDetailed fail as
// List and ListDetailed do on a reply in 200-299 that is not a list of instances: All on a body
// that List rejects, and AllDetailed on one that ListDetailed rejects, a JSON array too. Each
// loop yields one *Error that names the model, with the status of the reply, and no instance.
func TestResourceInstanceIteratorsStopAtAMalformedReply(t *testing.T) {
	for _, tc := range []struct {
		name, route, model string
		bodies             []string
		all                func(c *Client) (int, []error)
	}{
		{
			name: "All", route: listInstancesRoute, model: "models.ResourceInstanceRead",
			bodies: malformedInstanceLists,
			all: func(c *Client) (int, []error) {
				instances, errs := collect(t, c.ResourceInstances.All(t.Context(), nil))
				return len(instances), errs
			},
		},
		{
			name: "AllDetailed", route: detailedInstanceRoute,
			model: "models.ResourceInstanceDetailedRead", bodies: malformedDetailedInstanceLists,
			all: func(c *Client) (int, []error) {
				instances, errs := collect(t, c.ResourceInstances.AllDetailed(t.Context(), nil))
				return len(instances), errs
			},
		},
	} {
		for _, body := range tc.bodies {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: body})
			n, errs := tc.all(instanceClient(t, api))
			want := "status 200: the reply is not a valid list of " + tc.model
			if n != 0 || len(errs) != 1 || mustError(t, errs[0]).StatusCode != http.StatusOK ||
				!strings.HasSuffix(errs[0].Error(), want) {
				t.Errorf("%s of the reply %q yielded %d instances and the errors %v, want no "+
					"instance and one error that ends with %q", tc.name, body, n, errs, want)
			}
		}
	}
}

// TestDetailedRelationshipsRoundTrip checks that the instances of the reply fixture of the
// detailed list keep their relationship tuples: each decodes with its Relationships, and encodes
// back to the JSON object of the fixture, every field and every tuple in place, also the empty
// list of an instance without a tuple, with its timestamps in the form that models.Time writes,
// which decodes to the same instance again.
func TestDetailedRelationshipsRoundTrip(t *testing.T) {
	timestamps := cmpopts.IgnoreMapEntries(func(key string, _ any) bool {
		return key == "created_at" || key == "updated_at"
	})
	body := []byte(replyFixtureOf(t, "list_resource_instances_detailed.200.json"))
	list, err := wire.DecodeList[models.ResourceInstanceDetailedRead](body)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 2 || len(fixture.Data) != 2 || len(list.Data[0].Relationships) != 2 ||
		list.Data[1].Relationships == nil {
		t.Fatalf("the fixture decoded to %+v, want two instances, the first with two tuples and "+
			"the second with an empty list", list.Data)
	}
	for i, instance := range list.Data {
		encoded, err := wire.Encode(instance)
		if err != nil {
			t.Fatal(err)
		}
		var got, want any
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(fixture.Data[i], &want); err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(want, got, timestamps); diff != "" {
			t.Errorf("instance %d encodes to an object that differs from the fixture's "+
				"(-want +got):\n%s", i, diff)
		}
		var again models.ResourceInstanceDetailedRead
		if err := wire.Decode(encoded, &again); err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(instance, again); diff != "" {
			t.Errorf("instance %d encodes as %s, which decodes to another instance "+
				"(-first +second):\n%s", i, encoded, diff)
		}
	}
}

// TestResourceInstancesAfterClose checks that each method returns ErrClosed after Close, and
// sends nothing, not even the lookup of the scope: also Create without a tenant, Get of the
// instance detailed and each method of a copy of WaitForSync whose wait is not valid, which
// return ErrInvalidConfig before Close.
func TestResourceInstancesAfterClose(t *testing.T) {
	invalid := &ResourceInstancesListOptions{ListOptions: ListOptions{Page: -1}}
	for _, opts := range [][]Option{nil, {WithScope("p", "e")}, {WithFactsViaPDP()}} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		c := decisionClient(t, api, pdp, opts...)
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		ctx := t.Context()
		for _, s := range []*ResourceInstancesService{
			c.ResourceInstances, c.ResourceInstances.WaitForSync(-time.Second),
		} {
			var errs []error
			for _, listOpts := range []*ResourceInstancesListOptions{nil, invalid} {
				_, listErr := s.List(ctx, listOpts)
				_, detailedErr := s.ListDetailed(ctx, listOpts)
				_, allErrs := collect(t, s.All(ctx, listOpts))
				_, allDetailedErrs := collect(t, s.AllDetailed(ctx, listOpts))
				if len(allErrs) != 1 || len(allDetailedErrs) != 1 {
					t.Errorf("All and AllDetailed after Close yielded %v and %v, want ErrClosed "+
						"once each", allErrs, allDetailedErrs)
				}
				errs = append(append(append(errs, listErr, detailedErr), allErrs...),
					allDetailedErrs...)
			}
			_, getErr := s.Get(ctx, "document:doc-1")
			_, detailedGetErr := s.Get(ctx, detailedInstanceID)
			_, createErr := s.Create(ctx, models.ResourceInstanceCreate{Key: "doc-1"})
			_, updateErr := s.Update(ctx, "document:doc-1", models.ResourceInstanceUpdate{})
			deleteErr := s.Delete(ctx, "document:doc-1")
			for i, err := range append(errs, getErr, detailedGetErr, createErr, updateErr,
				deleteErr) {
				if !errors.Is(err, ErrClosed) {
					t.Errorf("call %d after Close returned %v, want ErrClosed", i, err)
				}
			}
		}
		api.AssertUntouched()
		pdp.AssertUntouched()
	}
}

// instanceWrites are the calls of the resource instances that the PDP container waits for sync
// on, each with its route on the container and its operation.
var instanceWrites = []struct {
	route string
	op    string
	call  func(ctx context.Context, s *ResourceInstancesService) error
}{
	{
		route: "POST /facts/resource_instances", op: opCreateResourceInstance,
		call: func(ctx context.Context, s *ResourceInstancesService) error {
			_, err := s.Create(ctx,
				models.ResourceInstanceCreate{Key: "doc-1", Tenant: "t", Resource: "document"})
			return err
		},
	},
	{
		route: "PATCH /facts/resource_instances/{instance_id}", op: opUpdateResourceInstance,
		call: func(ctx context.Context, s *ResourceInstancesService) error {
			_, err := s.Update(ctx, "document:doc-1", models.ResourceInstanceUpdate{})
			return err
		},
	},
}

// TestResourceInstancesWaitForSync checks the creates and the updates of a copy of WaitForSync
// through the PDP container: each carries the timeout in decimal seconds and the policy of
// WithSyncPolicy, and the calls of the service that WaitForSync was called on carry neither,
// before and after.
func TestResourceInstancesWaitForSync(t *testing.T) {
	for _, w := range instanceWrites {
		t.Run(w.op, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			reply := mockpermit.Reply{Body: instanceBody}
			pdp.Handle(w.route, reply, reply, reply)
			plain := decisionClient(t, api, pdp, WithFactsViaPDP()).ResourceInstances
			waiting := plain.WaitForSync(250*time.Millisecond, WithSyncPolicy(SyncPolicyIgnore))
			for _, s := range []*ResourceInstancesService{plain, waiting, plain} {
				if err := w.call(t.Context(), s); err != nil {
					t.Fatal(err)
				}
			}
			var got []string
			for _, r := range pdp.Requests() {
				got = append(got, waitHeaders(r.Header))
			}
			if diff := cmp.Diff([]string{"- -", "0.25 ignore", "- -"}, got); diff != "" {
				t.Errorf("the wait headers of the three calls differ (-want +got):\n%s", diff)
			}
			api.AssertUntouched()
		})
	}
}

// TestResourceInstancesWaitForSyncMisuse checks the calls through a copy of WaitForSync that
// cannot wait: List, All, ListDetailed, AllDetailed, Get and Delete, which the PDP container
// answers without waiting; and Create and Update from a client that sends facts to the API, or
// with a negative timeout. Each returns an error that matches ErrInvalidConfig and names the
// operation and the rule, and sends no request, not even the lookup of the scope.
func TestResourceInstancesWaitForSyncMisuse(t *testing.T) {
	type misuse struct {
		name    string
		viaPDP  bool
		timeout time.Duration
		op      string
		call    func(ctx context.Context, s *ResourceInstancesService) error
		rule    validate.Rule
	}
	first := func(seq iter.Seq2[string, error]) error {
		for _, err := range seq {
			return err
		}
		return nil
	}
	cases := []misuse{
		{
			name: "List", op: opListResourceInstances,
			call: func(ctx context.Context, s *ResourceInstancesService) error {
				_, err := s.List(ctx, nil)
				return err
			},
		},
		{
			name: "All", op: opListResourceInstances,
			call: func(ctx context.Context, s *ResourceInstancesService) error {
				return first(instanceLists[0].all(ctx, s, nil))
			},
		},
		{
			name: "ListDetailed", op: opListResourceInstancesDetailed,
			call: func(ctx context.Context, s *ResourceInstancesService) error {
				_, err := s.ListDetailed(ctx, nil)
				return err
			},
		},
		{
			name: "AllDetailed", op: opListResourceInstancesDetailed,
			call: func(ctx context.Context, s *ResourceInstancesService) error {
				return first(instanceLists[1].all(ctx, s, nil))
			},
		},
		{
			name: "Get", op: opGetResourceInstance,
			call: func(ctx context.Context, s *ResourceInstancesService) error {
				_, err := s.Get(ctx, "document:doc-1")
				return err
			},
		},
		{
			name: "Delete", op: opDeleteResourceInstance,
			call: func(ctx context.Context, s *ResourceInstancesService) error {
				return s.Delete(ctx, "document:doc-1")
			},
		},
	}
	for i := range cases {
		cases[i].viaPDP, cases[i].rule = true, validate.WaitForSyncNotWaiting
	}
	for _, w := range instanceWrites {
		cases = append(cases,
			misuse{
				name: w.op + " to the API", op: w.op, call: w.call,
				rule: validate.WaitForSyncFactsViaAPI,
			},
			misuse{
				name: w.op + " with a negative timeout", viaPDP: true, timeout: -time.Nanosecond,
				op: w.op, call: w.call, rule: validate.WaitForSyncTimeoutNegative,
			})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			var opts []Option
			if tc.viaPDP {
				opts = append(opts, WithFactsViaPDP())
			}
			s := decisionClient(t, api, pdp, opts...).ResourceInstances.WaitForSync(tc.timeout)
			err := tc.call(t.Context(), s)
			want := "permit: invalid configuration: " + tc.op + ": " + tc.rule.Message
			if err == nil || err.Error() != want || !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("the call returned %v, want %q, which matches ErrInvalidConfig", err,
					want)
			}
			api.AssertUntouched()
			pdp.AssertUntouched()
		})
	}
}

// FuzzResourceInstanceDetailedRead checks the reader of a reply that holds an instance with its
// relationship tuples on any body: it returns an instance or an *Error that names
// models.ResourceInstanceDetailedRead, never both and never a panic, and an instance that it
// returns encodes to a body that it reads back as the same instance. go test runs the seeds.
func FuzzResourceInstanceDetailedRead(f *testing.F) {
	for _, body := range []string{
		instanceBody, ``, `null`, `[]`, `{}`, `{"key":1}`, `{} {}`,
		`{"relationships":null,"attributes":null}`, `{"relationships":[]}`,
		`{"relationships":[{"subject":"a:b","relation":"r","object":"c:d"}]}`,
		`{"relationships":[{"subject":1}]}`, `{"created_at":"not a time"}`,
	} {
		f.Add([]byte(body))
	}
	op := defaultOp(f, opListResourceInstancesDetailed)
	f.Fuzz(func(t *testing.T, body []byte) {
		reply := transport.Reply{Status: 200, Body: body}
		instance, err := decodeObject[models.ResourceInstanceDetailedRead](op, reply)
		if (instance == nil) == (err == nil) {
			t.Fatalf("the reply %q returned the instance %+v and the error %v, want one of them",
				body, instance, err)
		}
		if err != nil {
			if !strings.HasSuffix(err.Error(),
				"the reply is not a valid models.ResourceInstanceDetailedRead") {
				t.Fatalf("the reply %q returned %v, want the error that names the model", body,
					err)
			}
			return
		}
		encoded, err := wire.Encode(instance)
		if err != nil {
			t.Fatalf("the instance of the reply %q does not encode: %v", body, err)
		}
		again, err := decodeObject[models.ResourceInstanceDetailedRead](op,
			transport.Reply{Status: 200, Body: encoded})
		if err != nil {
			t.Fatalf("the instance of the reply %q encodes as %s, which does not decode: %v",
				body, encoded, err)
		}
		if diff := cmp.Diff(instance, again); diff != "" {
			t.Fatalf("the instance of the reply %q encodes as %s, which decodes to another "+
				"instance (-first +second):\n%s", body, encoded, diff)
		}
	})
}
