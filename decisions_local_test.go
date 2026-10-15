package permit

import (
	"encoding/json"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// The route of the role assignments that the PDP container holds, as mockpermit.Server.Handle
// takes it, and the text of the errors of its calls before the status.
const (
	localRoute = "GET /local/role_assignments"
	localCall  = "permit: " + opLocalRoleAssignments + ": " + localRoute
)

// localFixture is the reply fixture of the route, in testdata/replies, as the wire cases serve
// it.
const localFixture = "list_role_assignments_local_role_assignments_get.200.json"

// assignments returns the role assignments of the users u<from> to u<to-1>, each the viewer of
// tenant-a: an empty list when to is from.
func assignments(from, to int) []LocalRoleAssignment {
	items := make([]LocalRoleAssignment, 0, to-from)
	for i := from; i < to; i++ {
		items = append(items, LocalRoleAssignment{
			User: "u" + strconv.Itoa(i), Role: "viewer", Tenant: "tenant-a",
		})
	}
	return items
}

// assignmentsReply returns a reply whose body is the JSON array of items.
func assignmentsReply(t *testing.T, items []LocalRoleAssignment) mockpermit.Reply {
	t.Helper()
	body, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	return mockpermit.Reply{Body: string(body)}
}

// collectAssignments ranges over seq to its end, and returns the role assignments and the errors
// it yields. It fails the test when seq yields a role assignment with an error, or anything after
// an error.
func collectAssignments(t *testing.T, seq iter.Seq2[LocalRoleAssignment, error],
) ([]LocalRoleAssignment, []error) {
	t.Helper()
	var (
		items []LocalRoleAssignment
		errs  []error
	)
	for item, err := range seq {
		if len(errs) > 0 {
			t.Errorf("the iterator yielded %+v and %v after the error %v", item, err, errs[0])
		}
		if err != nil {
			if item != (LocalRoleAssignment{}) {
				t.Errorf("the iterator yielded the role assignment %+v with the error %v", item,
					err)
			}
			errs = append(errs, err)
			continue
		}
		items = append(items, item)
	}
	return items, errs
}

// TestLocalRoleAssignmentsQueries checks the exact query of each kind of options: each filter
// alone and all of them together, as given, escaped, and none for an empty filter or nil options;
// the page and the per_page of the options when set; and, for the first page of the iterator, the
// same filters with page 1 and per_page 100, the most role assignments that the container gives
// on a page, unless the options give others. Nothing reaches the API.
func TestLocalRoleAssignmentsQueries(t *testing.T) {
	const (
		every = "resource=document&resource_instance=document%3A2026-report&role=editor&" +
			"tenant=tenant-a&user=alice"
		escaped = "tenant=%3F%23%25+&user=a+b%26c%3Dd%2F%C3%A9"
	)
	for _, tc := range []struct {
		name string
		opts *LocalRoleAssignmentsOptions
		// list is the query of LocalRoleAssignments, and all that of the first page of
		// LocalRoleAssignmentsAll.
		list, all string
	}{
		{name: "nil options", list: "", all: "page=1&per_page=100"},
		{
			name: "the zero options", opts: &LocalRoleAssignmentsOptions{}, list: "",
			all: "page=1&per_page=100",
		},
		{
			name: "a user", opts: &LocalRoleAssignmentsOptions{User: "alice"},
			list: "user=alice", all: "page=1&per_page=100&user=alice",
		},
		{
			name: "a role", opts: &LocalRoleAssignmentsOptions{Role: "editor"},
			list: "role=editor", all: "page=1&per_page=100&role=editor",
		},
		{
			name: "a tenant", opts: &LocalRoleAssignmentsOptions{Tenant: "tenant-a"},
			list: "tenant=tenant-a", all: "page=1&per_page=100&tenant=tenant-a",
		},
		{
			name: "a resource type", opts: &LocalRoleAssignmentsOptions{Resource: "document"},
			list: "resource=document", all: "page=1&per_page=100&resource=document",
		},
		{
			name: "a resource instance",
			opts: &LocalRoleAssignmentsOptions{ResourceInstance: "document:2026-report"},
			list: "resource_instance=document%3A2026-report",
			all:  "page=1&per_page=100&resource_instance=document%3A2026-report",
		},
		{
			name: "every field",
			opts: &LocalRoleAssignmentsOptions{
				ListOptions: ListOptions{Page: 3, PerPage: 5}, User: "alice", Role: "editor",
				Tenant: "tenant-a", Resource: "document", ResourceInstance: "document:2026-report",
			},
			list: "page=3&per_page=5&" + every, all: "page=3&per_page=5&" + every,
		},
		{
			name: "values to escape",
			opts: &LocalRoleAssignmentsOptions{User: "a b&c=d/é", Tenant: "?#% "},
			list: escaped, all: "page=1&per_page=100&" + escaped,
		},
		{
			name: "a page alone",
			opts: &LocalRoleAssignmentsOptions{ListOptions: ListOptions{Page: 2}},
			list: "page=2", all: "page=2&per_page=100",
		},
		{
			name: "a per_page alone",
			opts: &LocalRoleAssignmentsOptions{ListOptions: ListOptions{PerPage: 7}},
			list: "per_page=7", all: "page=1&per_page=7",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			pdp.Handle(localRoute, mockpermit.Reply{Body: `[]`}, mockpermit.Reply{Body: `[]`})
			c := decisionClient(t, api, pdp)
			if _, err := c.LocalRoleAssignments(t.Context(), tc.opts); err != nil {
				t.Fatalf("LocalRoleAssignments returned %v", err)
			}
			if _, errs := collectAssignments(t, c.LocalRoleAssignmentsAll(t.Context(),
				tc.opts)); errs != nil {
				t.Fatalf("LocalRoleAssignmentsAll yielded %v", errs)
			}
			if diff := cmp.Diff([]string{tc.list, tc.all}, queries(pdp)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
			api.AssertUntouched()
		})
	}
}

// TestLocalRoleAssignmentsAllReadsEveryPage checks that LocalRoleAssignmentsAll reads the 31 role
// assignments of a list, in order, asking for each page in turn with the filter of the options:
// at a per_page of 30, two pages of role assignments and the empty page after them; at the
// default per_page of 100, one page of 31 and the empty page after it; and from page 2 on, when
// the options start there. Nothing reaches the API.
func TestLocalRoleAssignmentsAllReadsEveryPage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    ListOptions
		replies [][]LocalRoleAssignment
		want    []LocalRoleAssignment
		queries []string
	}{
		{
			name: "per_page 30",
			opts: ListOptions{PerPage: 30},
			replies: [][]LocalRoleAssignment{
				assignments(0, 30), assignments(30, 31), assignments(0, 0),
			},
			want: assignments(0, 31),
			queries: []string{
				"page=1&per_page=30&user=alice", "page=2&per_page=30&user=alice",
				"page=3&per_page=30&user=alice",
			},
		},
		{
			name:    "the default per_page",
			replies: [][]LocalRoleAssignment{assignments(0, 31), assignments(0, 0)},
			want:    assignments(0, 31),
			queries: []string{"page=1&per_page=100&user=alice", "page=2&per_page=100&user=alice"},
		},
		{
			name:    "from page 2",
			opts:    ListOptions{Page: 2, PerPage: 30},
			replies: [][]LocalRoleAssignment{assignments(30, 31), assignments(0, 0)},
			want:    assignments(30, 31),
			queries: []string{"page=2&per_page=30&user=alice", "page=3&per_page=30&user=alice"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			var replies []mockpermit.Reply
			for _, items := range tc.replies {
				replies = append(replies, assignmentsReply(t, items))
			}
			pdp.Handle(localRoute, replies...)
			opts := &LocalRoleAssignmentsOptions{ListOptions: tc.opts, User: "alice"}
			items, errs := collectAssignments(t,
				decisionClient(t, api, pdp).LocalRoleAssignmentsAll(t.Context(), opts))
			if diff := cmp.Diff(tc.want, items); diff != "" || errs != nil {
				t.Errorf("LocalRoleAssignmentsAll yielded the errors %v and role assignments "+
					"that differ (-want +got):\n%s", errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(pdp)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
			api.AssertUntouched()
		})
	}
}

// TestLocalRoleAssignmentsAllReadsTheOptionsOnce checks that the iterator keeps the options of
// its call, so that a later change to them changes no request, and that each loop over it reads
// the role assignments again from the first page of the options.
func TestLocalRoleAssignmentsAllReadsTheOptionsOnce(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	one := assignmentsReply(t, assignments(0, 1))
	empty := mockpermit.Reply{Body: `[]`}
	pdp.Handle(localRoute, one, empty, one, empty)
	opts := &LocalRoleAssignmentsOptions{ListOptions: ListOptions{PerPage: 1}, User: "alice"}
	seq := decisionClient(t, api, pdp).LocalRoleAssignmentsAll(t.Context(), opts)
	opts.User, opts.Page, opts.PerPage = "bob", 5, 50
	for loop := 1; loop <= 2; loop++ {
		if items, errs := collectAssignments(t, seq); len(items) != 1 || errs != nil {
			t.Errorf("loop %d yielded %+v and the errors %v, want one role assignment", loop,
				items, errs)
		}
	}
	first, second := "page=1&per_page=1&user=alice", "page=2&per_page=1&user=alice"
	if diff := cmp.Diff([]string{first, second, first, second}, queries(pdp)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
	api.AssertUntouched()
}

// TestLocalRoleAssignmentsReturnsAPage checks the page that LocalRoleAssignments returns: its role
// assignments in the order of the reply, the page and the per_page that it asked for, page 1 when
// the options give none, and no counts, as the reply reports none. A page without role
// assignments has an empty list, not nil.
func TestLocalRoleAssignmentsReturnsAPage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  *LocalRoleAssignmentsOptions
		reply string
		want  *Page[LocalRoleAssignment]
	}{
		{
			name: "the reply fixture", reply: replyFixtureOf(t, localFixture),
			want: &Page[LocalRoleAssignment]{
				Data: []LocalRoleAssignment{
					{User: "alice", Role: "admin", Tenant: "tenant-a"},
					{
						User: "alice", Role: "editor", Tenant: "tenant-a",
						ResourceInstance: new("document:2026-report"),
					},
				},
				Page: 1,
			},
		},
		{
			name:  "the last page",
			opts:  &LocalRoleAssignmentsOptions{ListOptions: ListOptions{Page: 2, PerPage: 30}},
			reply: assignmentsReply(t, assignments(30, 31)).Body,
			want:  &Page[LocalRoleAssignment]{Data: assignments(30, 31), Page: 2, PerPage: 30},
		},
		{
			name:  "a page after the last",
			opts:  &LocalRoleAssignmentsOptions{ListOptions: ListOptions{Page: 3, PerPage: 30}},
			reply: `[]`,
			want:  &Page[LocalRoleAssignment]{Data: []LocalRoleAssignment{}, Page: 3, PerPage: 30},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			pdp.Handle(localRoute, mockpermit.Reply{Body: tc.reply})
			got, err := decisionClient(t, api, pdp).LocalRoleAssignments(t.Context(), tc.opts)
			if err != nil {
				t.Fatalf("LocalRoleAssignments returned %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("LocalRoleAssignments returned a page that differs (-want +got):\n%s",
					diff)
			}
			if got.Data == nil {
				t.Error("LocalRoleAssignments returned nil Data, want an empty list")
			}
			api.AssertUntouched()
		})
	}
}

// TestLocalRoleAssignmentsDecode checks how each method reads the role assignments of a reply: a
// resource_instance that is null or left out is nil, one that is a string, also the empty string,
// is that string, and a field that the SDK does not know is ignored.
func TestLocalRoleAssignmentsDecode(t *testing.T) {
	const body = `[` +
		`{"user":"alice","role":"admin","tenant":"tenant-a","resource_instance":null},` +
		`{"user":"bob","role":"viewer","tenant":"tenant-b"},` +
		`{"user":"carol","role":"editor","tenant":"tenant-a",` +
		`"resource_instance":"document:2026-report","granted_by":{"key":"x"},"new":[1]},` +
		`{"resource_instance":"","tenant":"tenant-c","role":"owner","user":"dave"}]`
	want := []LocalRoleAssignment{
		{User: "alice", Role: "admin", Tenant: "tenant-a"},
		{User: "bob", Role: "viewer", Tenant: "tenant-b"},
		{
			User: "carol", Role: "editor", Tenant: "tenant-a",
			ResourceInstance: new("document:2026-report"),
		},
		{User: "dave", Role: "owner", Tenant: "tenant-c", ResourceInstance: new("")},
	}
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	pdp.Handle(localRoute, mockpermit.Reply{Body: body}, mockpermit.Reply{Body: body},
		mockpermit.Reply{Body: `[]`})
	c := decisionClient(t, api, pdp)
	page, err := c.LocalRoleAssignments(t.Context(), nil)
	if err != nil {
		t.Fatalf("LocalRoleAssignments returned %v", err)
	}
	if diff := cmp.Diff(want, page.Data); diff != "" {
		t.Errorf("LocalRoleAssignments returned role assignments that differ (-want +got):\n%s",
			diff)
	}
	items, errs := collectAssignments(t, c.LocalRoleAssignmentsAll(t.Context(), nil))
	if diff := cmp.Diff(want, items); diff != "" || errs != nil {
		t.Errorf("LocalRoleAssignmentsAll yielded the errors %v and role assignments that "+
			"differ (-want +got):\n%s", errs, diff)
	}
	api.AssertUntouched()
}

// TestLocalRoleAssignmentsOfEmptyItems checks that an item that is null, or an object without
// the fields, decodes with the fields empty, as the method's doc says, rather than as an error.
func TestLocalRoleAssignmentsOfEmptyItems(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	pdp.Handle(localRoute, mockpermit.Reply{Body: `[null,{}]`})
	page, err := decisionClient(t, api, pdp).LocalRoleAssignments(t.Context(), nil)
	if err != nil {
		t.Fatalf("LocalRoleAssignments returned %v", err)
	}
	if diff := cmp.Diff([]LocalRoleAssignment{{}, {}}, page.Data); diff != "" {
		t.Errorf("LocalRoleAssignments returned role assignments that differ (-want +got):\n%s",
			diff)
	}
	api.AssertUntouched()
}

// TestMalformedLocalRoleAssignments checks that each method returns an *Error of the call, and no
// role assignment, for each reply in 200-299 that is not a JSON array of role assignments: an
// empty body, null, an object, also the paginated object of the API's lists, an array of another
// kind of item, a role assignment whose field has another type, a second value and text that is
// not JSON. Each sends one request.
func TestMalformedLocalRoleAssignments(t *testing.T) {
	const want = localCall + ": status 200: the reply is not a valid list of items"
	for _, body := range []string{
		``, `null`, `{}`, `{"data":[]}`, `[1]`, `[{"user":1}]`, `[] []`, `not json`,
	} {
		t.Run(body, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			pdp.Handle(localRoute, mockpermit.Reply{Body: body}, mockpermit.Reply{Body: body})
			c := decisionClient(t, api, pdp)
			page, err := c.LocalRoleAssignments(t.Context(), nil)
			if e := mustError(t, err); page != nil || e.Error() != want {
				t.Errorf("LocalRoleAssignments returned %+v and\n%v\nwant no page and\n%s", page,
					err, want)
			}
			items, errs := collectAssignments(t, c.LocalRoleAssignmentsAll(t.Context(), nil))
			if len(items) != 0 || len(errs) != 1 || errs[0].Error() != want {
				t.Errorf("LocalRoleAssignmentsAll yielded %+v and the errors %v, want the "+
					"error\n%s", items, errs, want)
			}
			if n := len(pdp.Requests()); n != 2 {
				t.Errorf("the calls sent %d requests, want 1 each", n)
			}
			api.AssertUntouched()
		})
	}
}

// TestLocalRoleAssignmentsOfTheCloudPDP checks each method against the cloud PDP, which answers
// the route with 404 and an empty body: the call returns, or the iterator yields once, an *Error
// that matches ErrContainerPDPOnly, not ErrNotFound, and names the operation and the route, and
// no role assignment, after one request, as a 404 is never sent again. Nothing reaches the API.
func TestLocalRoleAssignmentsOfTheCloudPDP(t *testing.T) {
	const want = localCall + ": status 404: permit: only the PDP container serves this route"
	check := func(t *testing.T, method string, err error) {
		t.Helper()
		e := mustError(t, err)
		if !errors.Is(err, ErrContainerPDPOnly) || errors.Is(err, ErrNotFound) ||
			e.Error() != want || e.StatusCode != http.StatusNotFound {
			t.Errorf("%s returned\n%v\nwant an error that matches ErrContainerPDPOnly and not "+
				"ErrNotFound:\n%s", method, err, want)
		}
	}
	t.Run("LocalRoleAssignments", func(t *testing.T) {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t, mockpermit.Cloud)
		page, err := decisionClient(t, api, pdp).LocalRoleAssignments(t.Context(), nil)
		check(t, "LocalRoleAssignments", err)
		if page != nil {
			t.Errorf("LocalRoleAssignments returned the page %+v with its error", page)
		}
		if n := len(pdp.Requests()); n != 1 {
			t.Errorf("LocalRoleAssignments sent %d requests to the cloud PDP, want 1", n)
		}
		api.AssertUntouched()
	})
	t.Run("LocalRoleAssignmentsAll", func(t *testing.T) {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t, mockpermit.Cloud)
		items, errs := collectAssignments(t,
			decisionClient(t, api, pdp).LocalRoleAssignmentsAll(t.Context(), nil))
		if len(items) != 0 || len(errs) != 1 {
			t.Fatalf("LocalRoleAssignmentsAll yielded %+v and the errors %v, want one error",
				items, errs)
		}
		check(t, "LocalRoleAssignmentsAll", errs[0])
		if n := len(pdp.Requests()); n != 1 {
			t.Errorf("LocalRoleAssignmentsAll sent %d requests to the cloud PDP, want 1", n)
		}
		api.AssertUntouched()
	})
}

// TestLocalRoleAssignmentsNotFound checks that a 404 with a JSON body, which the PDP container or
// a proxy before it sends, is an *Error that matches ErrNotFound, not ErrContainerPDPOnly, with
// the body, after one request.
func TestLocalRoleAssignmentsNotFound(t *testing.T) {
	const body = `{"detail":"Not Found"}`
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	pdp.Handle(localRoute, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
	_, err := decisionClient(t, api, pdp).LocalRoleAssignments(t.Context(), nil)
	want := &Error{
		Op: opLocalRoleAssignments, Method: http.MethodGet, Route: "/local/role_assignments",
		StatusCode: http.StatusNotFound, Detail: "Not Found", Body: []byte(body),
	}
	if diff := cmp.Diff(want, err); diff != "" {
		t.Errorf("LocalRoleAssignments returned an error that differs (-want +got):\n%s", diff)
	}
	if !errors.Is(err, ErrNotFound) || errors.Is(err, ErrContainerPDPOnly) {
		t.Errorf("LocalRoleAssignments returned %v, want an error that matches ErrNotFound only",
			err)
	}
	api.AssertUntouched()
}

// TestLocalRoleAssignmentsErrorReplies checks that a reply with an error status is an *Error of
// the call, with the status and the body of the reply, and no page: one that ends the call, such
// as 403 or 422, and a 429 or a 503 after the retries, as the list changes nothing. Nothing
// reaches the API.
func TestLocalRoleAssignmentsErrorReplies(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		// detail is the Detail that the Error reads from the body.
		detail string
	}{
		{
			http.StatusForbidden, `{"detail":"the API key may not read this environment"}`,
			"the API key may not read this environment",
		},
		{
			http.StatusUnprocessableEntity,
			`{"detail":[{"loc":["query","page"],"msg":"ensure this value is greater than or ` +
				`equal to 1"}]}`,
			"query.page: ensure this value is greater than or equal to 1",
		},
		{http.StatusTooManyRequests, `{"detail":"Too Many Requests"}`, "Too Many Requests"},
		{http.StatusServiceUnavailable, `<html>busy</html>`, ""},
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			retried := tc.status == http.StatusTooManyRequests ||
				tc.status == http.StatusServiceUnavailable
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			reply := mockpermit.Reply{
				Status: tc.status, Body: tc.body, Header: http.Header{"Retry-After": {"0"}},
			}
			replies := []mockpermit.Reply{reply}
			if retried {
				replies = append(replies, reply)
			}
			pdp.Handle(localRoute, replies...)
			c := decisionClient(t, api, pdp, WithRetry(RetryPolicy{MaxRetries: 1}))
			page, err := c.LocalRoleAssignments(t.Context(), nil)
			want := &Error{
				Op: opLocalRoleAssignments, Method: http.MethodGet,
				Route: "/local/role_assignments", StatusCode: tc.status, Detail: tc.detail,
				Body: []byte(tc.body),
			}
			if diff := cmp.Diff(want, err); diff != "" || page != nil {
				t.Errorf("LocalRoleAssignments returned the page %+v and an error that differs "+
					"(-want +got):\n%s", page, diff)
			}
			if n := len(pdp.Requests()); n != len(replies) {
				t.Errorf("LocalRoleAssignments sent %d requests for status %d, want %d", n,
					tc.status, len(replies))
			}
			api.AssertUntouched()
		})
	}
}

// TestLocalRoleAssignmentsAreRetried checks that a page whose first reply is a 503 is asked for
// again, as the list changes nothing, and that the call returns the role assignments of the
// second reply.
func TestLocalRoleAssignmentsAreRetried(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	pdp.Handle(localRoute, mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Body: `{"detail":"busy"}`,
		Header: http.Header{"Retry-After": {"0"}},
	}, assignmentsReply(t, assignments(0, 2)))
	page, err := decisionClient(t, api, pdp).LocalRoleAssignments(t.Context(), nil)
	if err != nil || page == nil || len(page.Data) != 2 {
		t.Errorf("LocalRoleAssignments returned %+v, %v; want the 2 role assignments of the "+
			"second reply", page, err)
	}
	if n := len(pdp.Requests()); n != 2 {
		t.Errorf("LocalRoleAssignments sent %d requests, want 2", n)
	}
	api.AssertUntouched()
}

// TestLocalRoleAssignmentsWithoutAReply checks that a call whose connection is refused returns an
// *Error of the call with status 0 whose cause is the *url.Error, and that the iterator yields it.
func TestLocalRoleAssignmentsWithoutAReply(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	clearEnv(t)
	c := mustNew(t, WithAPIKey(canary.Key("refused")), WithAPIURL(srv.URL), WithPDPURL(srv.URL),
		WithRetry(RetryPolicy{}))
	t.Cleanup(func() { _ = c.Close() })
	_, err := c.LocalRoleAssignments(t.Context(), nil)
	_, errs := collectAssignments(t, c.LocalRoleAssignmentsAll(t.Context(), nil))
	for _, err := range append([]error{err}, errs...) {
		e := mustError(t, err)
		if _, isURLError := errors.AsType[*url.Error](err); !isURLError || e.StatusCode != 0 ||
			e.Op != opLocalRoleAssignments {
			t.Errorf("the call returned %v, want the *Error of %s with status 0 that wraps a "+
				"*url.Error", err, opLocalRoleAssignments)
		}
	}
	if len(errs) != 1 {
		t.Errorf("LocalRoleAssignmentsAll yielded the errors %v, want one", errs)
	}
}

// TestLocalRoleAssignmentRepliesAddNoCredential checks that a reply of the route, whose values are
// the application's keys, adds nothing to the client's Set of credentials, not even a value
// under a credential name such as token, while the reply of an operation of the API still does.
func TestLocalRoleAssignmentRepliesAddNoCredential(t *testing.T) {
	const echoed, returned = "opaque-echo-local", "opaque-returned-value"
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	pdp.Handle(localRoute, mockpermit.Reply{
		Body: `[{"user":"alice","role":"admin","tenant":"t","token":"` + echoed + `"}]`,
	})
	api.Handle(route(opGet), mockpermit.Reply{Body: `{"key":"a","token":"` + returned + `"}`})
	c := wireClient(t, api, pdp)
	if _, err := c.LocalRoleAssignments(t.Context(), nil); err != nil {
		t.Fatalf("LocalRoleAssignments returned %v", err)
	}
	if c.secrets.Contains(echoed) {
		t.Errorf("a reply of the PDP added %s to the client's Set", echoed)
	}
	if _, err := do[testObject](t.Context(), c, get("a")); err != nil {
		t.Fatalf("the call of the API returned %v", err)
	}
	if !c.secrets.Contains(returned) {
		t.Error("the reply of an operation of the API did not add its token to the client's Set")
	}
}

// TestInvalidLocalRoleAssignmentsOptions checks that a Page or a PerPage less than zero is an
// error that matches ErrInvalidConfig and names the operation, from each method, and that
// neither sends a request.
func TestInvalidLocalRoleAssignmentsOptions(t *testing.T) {
	for _, tc := range []struct {
		opts ListOptions
		want string
	}{
		{ListOptions{Page: -1}, "Page is less than zero"},
		{ListOptions{PerPage: -1}, "PerPage is less than zero"},
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		c := decisionClient(t, api, pdp)
		opts := &LocalRoleAssignmentsOptions{ListOptions: tc.opts, User: "alice"}
		_, err := c.LocalRoleAssignments(t.Context(), opts)
		_, errs := collectAssignments(t, c.LocalRoleAssignmentsAll(t.Context(), opts))
		prefix := ErrInvalidConfig.Error() + ": " + opLocalRoleAssignments + ": ListOptions: " +
			tc.want
		for _, err := range append([]error{err}, errs...) {
			if !errors.Is(err, ErrInvalidConfig) || !strings.HasPrefix(err.Error(), prefix) {
				t.Errorf("%+v: the call returned %v, want an error that matches "+
					"ErrInvalidConfig and starts with %q", tc.opts, err, prefix)
			}
		}
		if len(errs) != 1 {
			t.Errorf("%+v: LocalRoleAssignmentsAll yielded the errors %v, want one", tc.opts, errs)
		}
		pdp.AssertUntouched()
		api.AssertUntouched()
	}
}

// TestLocalRoleAssignmentsAfterClose checks that each method returns ErrClosed after Close, and
// sends nothing.
func TestLocalRoleAssignmentsAfterClose(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	c := decisionClient(t, api, pdp)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.LocalRoleAssignments(t.Context(), nil); !errors.Is(err, ErrClosed) {
		t.Errorf("LocalRoleAssignments after Close returned %v, want ErrClosed", err)
	}
	_, errs := collectAssignments(t, c.LocalRoleAssignmentsAll(t.Context(), nil))
	if len(errs) != 1 || !errors.Is(errs[0], ErrClosed) {
		t.Errorf("LocalRoleAssignmentsAll after Close yielded %v, want ErrClosed once", errs)
	}
	pdp.AssertUntouched()
	api.AssertUntouched()
}
