package permit

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/models"
)

// The routes of the role assignments on the API, under the scope p and e of userClient, and on
// the PDP container, as mockpermit.Server.Handle takes them.
const (
	listRoleAssignmentsRoute    = "GET /v2/facts/{proj_id}/{env_id}/role_assignments"
	listDetailedRoute           = "GET /v2/facts/{proj_id}/{env_id}/role_assignments/detailed"
	assignRoleRoute             = "POST /v2/facts/{proj_id}/{env_id}/role_assignments"
	unassignRoleRoute           = "DELETE /v2/facts/{proj_id}/{env_id}/role_assignments"
	pdpListRoleAssignmentsRoute = "GET /facts/role_assignments"
	pdpListDetailedRoute        = "GET /facts/role_assignments/detailed"
	pdpAssignRoleRoute          = "POST /facts/role_assignments"
	pdpUnassignRoleRoute        = "DELETE /facts/role_assignments"
	bulkAssignRoleRoute         = "POST /v2/facts/{proj_id}/{env_id}/role_assignments/bulk"
	bulkUnassignRoleRoute       = "DELETE /v2/facts/{proj_id}/{env_id}/role_assignments/bulk"
	pdpBulkAssignRoleRoute      = "POST /facts/role_assignments/bulk"
	pdpBulkUnassignRoleRoute    = "DELETE /facts/role_assignments/bulk"
)

// The replies of the tests of the bulk routes of the role assignments, each a report that counts
// one role assignment.
const (
	bulkAssignedBody   = `{"assignments_created":1}`
	bulkUnassignedBody = `{"assignments_removed":1}`
)

// assignmentsOf returns the role assignments of the users u<from> to u<to-1>, with only a user:
// an empty list when to is from.
func assignmentsOf(from, to int) []models.RoleAssignmentRead {
	items := make([]models.RoleAssignmentRead, 0, to-from)
	for i := from; i < to; i++ {
		items = append(items, models.RoleAssignmentRead{User: "u" + strconv.Itoa(i)})
	}
	return items
}

// detailedOf returns the detailed role assignments of the users u<from> to u<to-1>, with only
// the key of a user: an empty list when to is from.
func detailedOf(from, to int) []models.RoleAssignmentDetailedRead {
	items := make([]models.RoleAssignmentDetailedRead, 0, to-from)
	for i := from; i < to; i++ {
		items = append(items, models.RoleAssignmentDetailedRead{
			User: models.RoleAssignmentUser{Key: "u" + strconv.Itoa(i)},
		})
	}
	return items
}

// editorAssignment is the assignment of the tests of the role assignments, of the role editor to
// alice in the tenant tenant-a, and editorRemoval is its removal.
var (
	editorAssignment = models.RoleAssignmentCreate{
		User: "alice", Role: "editor", Tenant: "tenant-a",
	}
	editorRemoval = models.RoleAssignmentRemove{User: "alice", Role: "editor", Tenant: "tenant-a"}
)

// TestRoleAssignmentListsReturnAnEmptyPage checks that a page without role assignments, a JSON
// array or a paginated object whose data is empty, is a page whose Data is empty and not nil, for
// List and ListDetailed, through the API and through the PDP container, so that a caller can
// range over it and tell it from a failure.
func TestRoleAssignmentListsReturnAnEmptyPage(t *testing.T) {
	for _, viaPDP := range []bool{false, true} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		to, list, detailed := api, listRoleAssignmentsRoute, listDetailedRoute
		opts := []Option{WithScope("p", "e")}
		if viaPDP {
			to, list, detailed = pdp, pdpListRoleAssignmentsRoute, pdpListDetailedRoute
			opts = append(opts, WithFactsViaPDP())
		}
		empty := mockpermit.Reply{Body: `{"data":[],"total_count":0,"page_count":0}`}
		to.Handle(list, mockpermit.Reply{Body: `[]`}, empty)
		to.Handle(detailed, empty)
		s := decisionClient(t, api, pdp, opts...).RoleAssignments
		for i := range 2 {
			page, err := s.List(t.Context(), &RoleAssignmentsListOptions{Users: []string{"nobody"}})
			if err != nil || page.Data == nil || len(page.Data) != 0 {
				t.Errorf("via the PDP %t: List of the empty reply %d returned %#v, %v; want a "+
					"page whose Data is empty and not nil", viaPDP, i, page, err)
			}
		}
		page, err := s.ListDetailed(t.Context(), nil)
		if err != nil || page.Data == nil || len(page.Data) != 0 || *page.TotalCount != 0 {
			t.Errorf("via the PDP %t: ListDetailed of an empty page returned %#v, %v; want a "+
				"page whose Data is empty and not nil", viaPDP, page, err)
		}
	}
}

// TestRoleAssignmentsAllReadsEveryPage checks that All and AllDetailed read the 1001 role
// assignments of a list, in order, asking for each page in turn at the default per_page of
// 1000, the largest that the route accepts: All, whose replies are JSON arrays, asks for the
// pages of 1000 and of 1 and for the empty page after them, and AllDetailed, whose paginated
// replies report a page_count of 2, for the two pages alone. It checks a per_page of 30 too.
func TestRoleAssignmentsAllReadsEveryPage(t *testing.T) {
	array := func(t *testing.T, from, to int) mockpermit.Reply {
		t.Helper()
		return arrayOf(t, assignmentsOf(from, to))
	}
	thousand := []string{"page=1&per_page=1000", "page=2&per_page=1000", "page=3&per_page=1000"}
	t.Run("All at the default per_page", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(listRoleAssignmentsRoute, array(t, 0, 1000), array(t, 1000, 1001),
			array(t, 0, 0))
		items, errs := collect(t, userClient(t, api).RoleAssignments.All(t.Context(), nil))
		if diff := cmp.Diff(assignmentsOf(0, 1001), items); diff != "" || errs != nil {
			t.Errorf("All yielded the errors %v and role assignments that differ "+
				"(-want +got):\n%s", errs, diff)
		}
		if diff := cmp.Diff(thousand, queries(api)); diff != "" {
			t.Errorf("the queries differ (-want +got):\n%s", diff)
		}
	})
	t.Run("AllDetailed at the default per_page", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		counts := `,"total_count":1001,"page_count":2`
		api.Handle(listDetailedRoute, pageOf(t, detailedOf(0, 1000), counts),
			pageOf(t, detailedOf(1000, 1001), counts))
		items, errs := collect(t, userClient(t, api).RoleAssignments.AllDetailed(t.Context(), nil))
		if diff := cmp.Diff(detailedOf(0, 1001), items); diff != "" || errs != nil {
			t.Errorf("AllDetailed yielded the errors %v and role assignments that differ "+
				"(-want +got):\n%s", errs, diff)
		}
		if diff := cmp.Diff(thousand[:2], queries(api)); diff != "" {
			t.Errorf("the queries differ (-want +got):\n%s", diff)
		}
	})
	t.Run("All at a per_page of 30", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(listRoleAssignmentsRoute, pageOf(t, assignmentsOf(0, 30), ""),
			pageOf(t, assignmentsOf(30, 31), ""), pageOf(t, assignmentsOf(0, 0), ""))
		opts := &RoleAssignmentsListOptions{ListOptions: ListOptions{PerPage: 30}}
		items, errs := collect(t, userClient(t, api).RoleAssignments.All(t.Context(), opts))
		if diff := cmp.Diff(assignmentsOf(0, 31), items); diff != "" || errs != nil {
			t.Errorf("All yielded the errors %v and role assignments that differ "+
				"(-want +got):\n%s", errs, diff)
		}
		want := []string{"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30"}
		if diff := cmp.Diff(want, queries(api)); diff != "" {
			t.Errorf("the queries differ (-want +got):\n%s", diff)
		}
	})
}

// TestRoleAssignmentsAllReadsTheOptionsOnce checks that the iterators of All and AllDetailed
// keep the options of their call, the filters too, so that a later change to them changes no
// request; that a loop that breaks asks for no other page; and that each loop reads the role
// assignments again from the first page, along the same path, through the API and through the
// PDP container.
func TestRoleAssignmentsAllReadsTheOptionsOnce(t *testing.T) {
	for _, viaPDP := range []bool{false, true} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		to, list, detailed := api, listRoleAssignmentsRoute, listDetailedRoute
		path, opts := "/v2/facts/p/e/role_assignments", []Option{WithScope("p", "e")}
		if viaPDP {
			to, list, detailed = pdp, pdpListRoleAssignmentsRoute, pdpListDetailedRoute
			path, opts = "/facts/role_assignments", append(opts, WithFactsViaPDP())
		}
		to.Handle(list, arrayOf(t, assignmentsOf(0, 2)), arrayOf(t, assignmentsOf(0, 2)))
		two := pageOf(t, detailedOf(0, 2), `,"total_count":9`)
		to.Handle(detailed, two, two)
		options := &RoleAssignmentsListOptions{
			ListOptions: ListOptions{PerPage: 2}, Users: []string{"u0"}, Roles: []string{"r"},
			Tenants: []string{"t"}, ResourceInstance: "document:doc-1",
		}
		s := decisionClient(t, api, pdp, opts...).RoleAssignments
		all, allDetailed := s.All(t.Context(), options), s.AllDetailed(t.Context(), options)
		options.Page, options.PerPage, options.ResourceInstance = 5, 50, ""
		options.Users[0], options.Roles, options.Tenants = "x", nil, nil
		for loop := 1; loop <= 2; loop++ {
			for a, err := range all {
				if err != nil || a.User != "u0" {
					t.Errorf("via the PDP %t: loop %d of All yielded %+v and %v first, want u0",
						viaPDP, loop, a, err)
				}
				break
			}
			for a, err := range allDetailed {
				if err != nil || a.User.Key != "u0" {
					t.Errorf("via the PDP %t: loop %d of AllDetailed yielded %+v and %v first, "+
						"want u0", viaPDP, loop, a, err)
				}
				break
			}
		}
		var got []string
		for _, r := range to.Requests() {
			got = append(got, r.RawPath+"?"+r.RawQuery)
		}
		query := "?page=1&per_page=2&resource_instance=document%3Adoc-1&role=r&tenant=t&user=u0"
		first, firstDetailed := path+query, path+"/detailed"+query
		want := []string{first, firstDetailed, first, firstDetailed}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("via the PDP %t: the requests differ (-want +got):\n%s", viaPDP, diff)
		}
	}
}

// TestRoleAssignmentFilters checks the query of List and ListDetailed for each filter alone: a
// filter that is not set is not sent, a filter of a list repeats its parameter for each value, in
// order, the empty string too, and neither list sends the deprecated query parameters detailed
// and include_total_count.
func TestRoleAssignmentFilters(t *testing.T) {
	for _, tc := range []struct {
		opts *RoleAssignmentsListOptions
		want string
	}{
		{nil, ""},
		{&RoleAssignmentsListOptions{}, ""},
		{&RoleAssignmentsListOptions{Users: []string{}}, ""},
		{&RoleAssignmentsListOptions{Users: []string{"b", "a"}}, "user=b&user=a"},
		{&RoleAssignmentsListOptions{Users: []string{""}}, "user="},
		{&RoleAssignmentsListOptions{Roles: []string{"owner"}}, "role=owner"},
		{&RoleAssignmentsListOptions{Tenants: []string{"t1", "t2"}}, "tenant=t1&tenant=t2"},
		{
			&RoleAssignmentsListOptions{ResourceInstance: "document:a b"},
			"resource_instance=document%3Aa+b",
		},
		{
			&RoleAssignmentsListOptions{ListOptions: ListOptions{Page: 3}, Roles: []string{"r"}},
			"page=3&role=r",
		},
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(listRoleAssignmentsRoute, mockpermit.Reply{Body: `[]`})
		api.Handle(listDetailedRoute, mockpermit.Reply{Body: `{"data":[],"total_count":0}`})
		s := userClient(t, api).RoleAssignments
		if _, err := s.List(t.Context(), tc.opts); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ListDetailed(t.Context(), tc.opts); err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff([]string{tc.want, tc.want}, queries(api)); diff != "" {
			t.Errorf("the queries of the options %+v differ (-want +got):\n%s", tc.opts, diff)
		}
		for _, q := range queries(api) {
			for _, deprecated := range []string{"detailed", "include_total_count"} {
				if strings.Contains(q, deprecated) {
					t.Errorf("the query %q holds the deprecated parameter %s", q, deprecated)
				}
			}
		}
	}
}

// TestRoleAssignmentsScope checks where the calls of the role assignments go without WithScope:
// to the project and the environment of the API key, which the first call looks up once; and,
// with the scope of a project-level key, which has no environment, to no route of the role
// assignments, with an error that matches ErrInvalidConfig.
func TestRoleAssignmentsScope(t *testing.T) {
	t.Run("the scope of the API key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		api.Handle(assignRoleRoute, mockpermit.Reply{Body: assignmentBody})
		api.Handle(listDetailedRoute, mockpermit.Reply{Body: `{"data":[],"total_count":0}`})
		c := decisionClient(t, api, mockpermit.NewPDP(t))
		if _, err := c.RoleAssignments.Assign(t.Context(), editorAssignment); err != nil {
			t.Fatal(err)
		}
		if _, err := c.RoleAssignments.ListDetailed(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
		assignments := "/v2/facts/" + scopeProjectID + "/" + scopeEnvironmentID +
			"/role_assignments"
		var got []string
		for _, r := range api.Requests() {
			got = append(got, r.Method+" "+r.RawPath)
		}
		want := []string{
			"GET /v2/api-key/scope", "POST " + assignments, "GET " + assignments + "/detailed",
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("the requests differ (-want +got):\n%s", diff)
		}
	})
	t.Run("a scope without an environment", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ".project"))
		c := decisionClient(t, api, mockpermit.NewPDP(t))
		err := c.RoleAssignments.Unassign(t.Context(), editorRemoval)
		want := "permit: invalid configuration: unassign_role: " +
			validate.ScopeEnvironmentMissing.Message
		if !errors.Is(err, ErrInvalidConfig) || err.Error() != want {
			t.Errorf("Unassign returned %v, want %q", err, want)
		}
		if n := len(api.Requests()); n != 1 {
			t.Errorf("the API got %d requests, want the lookup alone", n)
		}
	})
}

// TestRoleAssignmentUnassignReplies checks the replies to Unassign, through the API and through
// the PDP container: nil for a 204 without a body, and for a 200 with the role assignment, with an
// empty body or with a body that is not a role assignment, which it does not read; and, as its
// body names the role assignment, an *Error that matches ErrNotFound for a 404 of the API, with or
// without a body, and of the container with a body, never nil, each after one request, also with
// a retry policy that allows one more; and, through the container, an error that matches
// ErrContainerPDPOnly, not ErrNotFound, for a 404 with an empty body.
func TestRoleAssignmentUnassignReplies(t *testing.T) {
	for _, viaPDP := range []bool{false, true} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		to, route := api, unassignRoleRoute
		opts := []Option{WithScope("p", "e")}
		if viaPDP {
			to, route = pdp, pdpUnassignRoleRoute
			opts = append(opts, WithFactsViaPDP())
		}
		replies := []mockpermit.Reply{
			{Status: http.StatusNoContent},
			{Status: http.StatusOK, Body: replyFixtureOf(t, "unassign_role.200.json")},
			{Status: http.StatusOK, Body: ""},
			{Status: http.StatusOK, Body: `[1]`},
		}
		to.Handle(route, replies...)
		s := decisionClient(t, api, pdp, opts...).RoleAssignments
		for i, reply := range replies {
			if err := s.Unassign(t.Context(), editorRemoval); err != nil {
				t.Errorf("via the PDP %t: Unassign of the reply %d, %d %q, returned %v, want nil",
					viaPDP, i, reply.Status, reply.Body, err)
			}
		}
		if n := len(to.Requests()); n != len(replies) {
			t.Errorf("via the PDP %t: %d calls sent %d requests", viaPDP, len(replies), n)
		}
	}
	for _, tc := range []struct {
		viaPDP        bool
		body          string
		containerOnly bool
	}{
		{viaPDP: false, body: unassignNotFoundBody},
		{viaPDP: false, body: ""},
		{viaPDP: true, body: unassignNotFoundBody},
		{viaPDP: true, body: "", containerOnly: true},
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		to, route := api, unassignRoleRoute
		opts := []Option{WithScope("p", "e"), WithRetry(RetryPolicy{MaxRetries: 1})}
		if tc.viaPDP {
			to, route = pdp, pdpUnassignRoleRoute
			opts = append(opts, WithFactsViaPDP())
		}
		to.Handle(route, mockpermit.Reply{Status: http.StatusNotFound, Body: tc.body})
		err := decisionClient(t, api, pdp, opts...).RoleAssignments.Unassign(t.Context(),
			editorRemoval)
		e := mustError(t, err)
		if e.StatusCode != http.StatusNotFound || e.Op != opUnassignRole ||
			errors.Is(err, ErrNotFound) == tc.containerOnly ||
			errors.Is(err, ErrContainerPDPOnly) != tc.containerOnly {
			t.Errorf("via the PDP %t: Unassign of a 404 with the body %q returned %v, want an "+
				"*Error that matches ErrContainerPDPOnly: %t, and else ErrNotFound", tc.viaPDP,
				tc.body, err, tc.containerOnly)
		}
		if n := len(to.Requests()); n != 1 {
			t.Errorf("via the PDP %t: Unassign of a 404 sent %d requests, want 1", tc.viaPDP, n)
		}
	}
}

// TestRoleAssignmentBodies checks the bodies of Assign and Unassign, and of BulkAssign and
// BulkUnassign, byte for byte against the golden files in testdata/golden: an assignment in a
// tenant to a user whose key a JSON encoder may escape; one on a resource instance without a
// tenant, which leaves the tenant out, not empty, where v1 sent "tenant":""; the zero
// assignment, which holds the user and the role, empty; an unassignment of a role in a tenant and
// of one on a resource instance; and the zero unassignment, which holds the user, the role and
// the tenant, empty. A bulk call sends a JSON array of the same items, in order, the remove
// model for BulkUnassign, so that its zero item holds the tenant, and [] for nil and for an empty
// slice.
func TestRoleAssignmentBodies(t *testing.T) {
	assigned := func(a models.RoleAssignmentCreate) func(s *RoleAssignmentsService) error {
		return func(s *RoleAssignmentsService) error {
			_, err := s.Assign(t.Context(), a)
			return err
		}
	}
	unassigned := func(a models.RoleAssignmentRemove) func(s *RoleAssignmentsService) error {
		return func(s *RoleAssignmentsService) error { return s.Unassign(t.Context(), a) }
	}
	bulkAssigned := func(a []models.RoleAssignmentCreate) func(s *RoleAssignmentsService) error {
		return func(s *RoleAssignmentsService) error {
			_, err := s.BulkAssign(t.Context(), a, nil)
			return err
		}
	}
	bulkUnassigned := func(a []models.RoleAssignmentRemove) func(s *RoleAssignmentsService) error {
		return func(s *RoleAssignmentsService) error {
			_, err := s.BulkUnassign(t.Context(), a)
			return err
		}
	}
	tenantAssignment := models.RoleAssignmentCreate{
		User: "idp|dave@example.com", Role: "editor", Tenant: "tenant-a",
	}
	instanceAssignment := models.RoleAssignmentCreate{
		User: "alice", Role: "owner", ResourceInstance: "document:doc-1",
	}
	instanceRemoval := models.RoleAssignmentRemove{
		User: "alice", Role: "owner", Tenant: "tenant-a", ResourceInstance: "document:doc-1",
	}
	ok := mockpermit.Reply{Body: assignmentBody}
	gone := mockpermit.Reply{Status: http.StatusNoContent}
	created := mockpermit.Reply{Body: bulkAssignedBody}
	removed := mockpermit.Reply{Body: bulkUnassignedBody}
	for _, tc := range []struct {
		golden string
		route  string
		reply  mockpermit.Reply
		call   func(s *RoleAssignmentsService) error
	}{
		{"assign_role.tenant.golden", assignRoleRoute, ok, assigned(tenantAssignment)},
		{"assign_role.instance.golden", assignRoleRoute, ok, assigned(instanceAssignment)},
		{"assign_role.empty.golden", assignRoleRoute, ok, assigned(models.RoleAssignmentCreate{})},
		{"unassign_role.tenant.golden", unassignRoleRoute, gone, unassigned(editorRemoval)},
		{"unassign_role.instance.golden", unassignRoleRoute, gone, unassigned(instanceRemoval)},
		{
			"unassign_role.empty.golden", unassignRoleRoute, gone,
			unassigned(models.RoleAssignmentRemove{}),
		},
		{
			"bulk_assign_role.items.golden", bulkAssignRoleRoute, created,
			bulkAssigned([]models.RoleAssignmentCreate{
				tenantAssignment, instanceAssignment, {},
			}),
		},
		{"bulk_assign_role.empty.golden", bulkAssignRoleRoute, created, bulkAssigned(nil)},
		{
			"bulk_assign_role.empty.golden", bulkAssignRoleRoute, created,
			bulkAssigned([]models.RoleAssignmentCreate{}),
		},
		{
			"bulk_unassign_role.items.golden", bulkUnassignRoleRoute, removed,
			bulkUnassigned([]models.RoleAssignmentRemove{
				{User: "idp|dave@example.com", Role: "editor", Tenant: "tenant-a"},
				instanceRemoval,
				{},
			}),
		},
		{"bulk_unassign_role.empty.golden", bulkUnassignRoleRoute, removed, bulkUnassigned(nil)},
		{
			"bulk_unassign_role.empty.golden", bulkUnassignRoleRoute, removed,
			bulkUnassigned([]models.RoleAssignmentRemove{}),
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, tc.reply)
			if err := tc.call(userClient(t, api).RoleAssignments); err != nil {
				t.Fatalf("the call returned %v", err)
			}
			requests := api.Requests()
			if len(requests) != 1 || requests[0].RawQuery != "" {
				t.Fatalf("the call sent %+v, want one request without a query", requests)
			}
			requests[0].ExpectBytes(t, filepath.Join("testdata", "golden", tc.golden))
		})
	}
}

// TestRoleAssignmentRetries checks when a call of the role assignments sends its request again:
// Assign and BulkAssign, which the API may have acted on, after a 429 only, so a 503 ends each
// after one request; Unassign, BulkUnassign, List and ListDetailed, which do the same when sent
// twice, after a 503 too.
func TestRoleAssignmentRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	assigned := func(s *RoleAssignmentsService) error {
		_, err := s.Assign(t.Context(), editorAssignment)
		return err
	}
	bulkAssigned := func(s *RoleAssignmentsService) error {
		_, err := s.BulkAssign(t.Context(), []models.RoleAssignmentCreate{editorAssignment}, nil)
		return err
	}
	for _, tc := range []struct {
		name    string
		route   string
		replies []mockpermit.Reply
		call    func(s *RoleAssignmentsService) error
		status  int
	}{
		{
			name: "Assign after a 429", route: assignRoleRoute,
			replies: []mockpermit.Reply{limited, {Body: assignmentBody}}, call: assigned,
		},
		{
			name: "Assign after a 503", route: assignRoleRoute,
			replies: []mockpermit.Reply{unavailable}, call: assigned,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "Unassign after a 503", route: unassignRoleRoute,
			replies: []mockpermit.Reply{unavailable, {Status: http.StatusNoContent}},
			call: func(s *RoleAssignmentsService) error {
				return s.Unassign(t.Context(), editorRemoval)
			},
		},
		{
			name: "BulkAssign after a 429", route: bulkAssignRoleRoute,
			replies: []mockpermit.Reply{limited, {Body: bulkAssignedBody}}, call: bulkAssigned,
		},
		{
			name: "BulkAssign after a 503", route: bulkAssignRoleRoute,
			replies: []mockpermit.Reply{unavailable}, call: bulkAssigned,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "BulkUnassign after a 503", route: bulkUnassignRoleRoute,
			replies: []mockpermit.Reply{unavailable, {Body: bulkUnassignedBody}},
			call: func(s *RoleAssignmentsService) error {
				_, err := s.BulkUnassign(t.Context(),
					[]models.RoleAssignmentRemove{editorRemoval})
				return err
			},
		},
		{
			name: "List after a 503", route: listRoleAssignmentsRoute,
			replies: []mockpermit.Reply{unavailable, {Body: `[]`}},
			call: func(s *RoleAssignmentsService) error {
				_, err := s.List(t.Context(), nil)
				return err
			},
		},
		{
			name: "ListDetailed after a 503", route: listDetailedRoute,
			replies: []mockpermit.Reply{unavailable, {Body: `{"data":[],"total_count":0}`}},
			call: func(s *RoleAssignmentsService) error {
				_, err := s.ListDetailed(t.Context(), nil)
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, tc.replies...)
			c := userClient(t, api, WithRetry(RetryPolicy{MaxRetries: 1}))
			err := tc.call(c.RoleAssignments)
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

// TestMalformedRoleAssignmentReplies checks that a reply in 200-299 that is not a role
// assignment to Assign, an empty body too, and a reply that is not a list of role assignments of
// the shape that each list's route answers, a JSON array or the paginated object to List and the
// paginated object alone to ListDetailed, is an *Error that names the model, with the status of
// the reply, and no result.
func TestMalformedRoleAssignmentReplies(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusCreated} {
		for _, body := range []string{``, ` `, `null`, `[]`, `"a"`, `{"user":1}`, `{} {}`} {
			api := mockpermit.NewAPI(t)
			api.Handle(assignRoleRoute, mockpermit.Reply{Status: status, Body: body})
			got, err := userClient(t, api).RoleAssignments.Assign(t.Context(), editorAssignment)
			want := "status " + strconv.Itoa(status) +
				": the reply is not a valid models.RoleAssignmentRead"
			if e := mustError(t, err); got != nil || e.StatusCode != status ||
				!strings.HasSuffix(e.Error(), want) {
				t.Errorf("Assign of the reply %d %q returned %+v and\n%v\nwant no result and an "+
					"error that ends with %q", status, body, got, err, want)
			}
		}
	}
	for _, tc := range []struct {
		route, model string
		bodies       []string
		call         func(s *RoleAssignmentsService) (bool, error)
	}{
		{
			route: listRoleAssignmentsRoute, model: "models.RoleAssignmentRead",
			bodies: []string{``, `null`, `"a"`, `{"data":null}`, `[{"user":1}]`, `{"data":[1]}`},
			call: func(s *RoleAssignmentsService) (bool, error) {
				page, err := s.List(t.Context(), nil)
				return page != nil, err
			},
		},
		{
			route: listDetailedRoute, model: "models.RoleAssignmentDetailedRead",
			bodies: []string{``, `null`, `[]`, `[{"user":{"key":"a"}}]`, `{"data":[{"user":1}]}`},
			call: func(s *RoleAssignmentsService) (bool, error) {
				page, err := s.ListDetailed(t.Context(), nil)
				return page != nil, err
			},
		},
	} {
		for _, body := range tc.bodies {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: body})
			got, err := tc.call(userClient(t, api).RoleAssignments)
			want := "status 200: the reply is not a valid list of " + tc.model
			if e := mustError(t, err); got || !strings.HasSuffix(e.Error(), want) {
				t.Errorf("%s of the reply %q returned a page: %t, and\n%v\nwant no page and an "+
					"error that ends with %q", tc.route, body, got, err, want)
			}
		}
	}
}

// TestMalformedBulkRoleAssignmentReplies checks that a reply in 200-299 to BulkAssign or
// BulkUnassign that is not a JSON object of its report, an empty body too, such as that of a
// 204, is an *Error that names the model, with the status of the reply, and no report.
func TestMalformedBulkRoleAssignmentReplies(t *testing.T) {
	for _, tc := range []struct {
		route, model string
		call         func(s *RoleAssignmentsService) (bool, error)
	}{
		{
			route: bulkAssignRoleRoute, model: "models.BulkRoleAssignmentReport",
			call: func(s *RoleAssignmentsService) (bool, error) {
				report, err := s.BulkAssign(t.Context(), nil, nil)
				return report != nil, err
			},
		},
		{
			route: bulkUnassignRoleRoute, model: "models.BulkRoleUnAssignmentReport",
			call: func(s *RoleAssignmentsService) (bool, error) {
				report, err := s.BulkUnassign(t.Context(), nil)
				return report != nil, err
			},
		},
	} {
		for _, reply := range []mockpermit.Reply{
			{Status: http.StatusNoContent},
			{Body: ``},
			{Body: ` `},
			{Body: `null`},
			{Body: `[]`},
			{Body: `"a"`},
			{Body: `{"assignments_created":"1","assignments_removed":"1"}`},
			{Body: `{} {}`},
		} {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, reply)
			got, err := tc.call(userClient(t, api).RoleAssignments)
			status := reply.Status
			if status == 0 {
				status = http.StatusOK
			}
			want := "status " + strconv.Itoa(status) + ": the reply is not a valid " + tc.model
			if e := mustError(t, err); got || e.StatusCode != status ||
				!strings.HasSuffix(e.Error(), want) {
				t.Errorf("%s of the reply %d %q returned a report: %t, and\n%v\nwant no report "+
					"and an error that ends with %q", tc.route, status, reply.Body, got, err, want)
			}
		}
	}
}

// TestBulkRoleAssignmentNotFound checks the 404 of the bulk routes, through the API and through
// the PDP container: as the body names the role assignments, an *Error that matches ErrNotFound,
// never a report, for a 404 with a body, after one request, also with a retry policy that allows
// one more; and, through the container, an error that matches ErrContainerPDPOnly, not
// ErrNotFound, for a 404 with an empty body.
func TestBulkRoleAssignmentNotFound(t *testing.T) {
	for _, tc := range []struct {
		viaPDP        bool
		body          string
		containerOnly bool
	}{
		{viaPDP: false, body: unassignNotFoundBody},
		{viaPDP: true, body: unassignNotFoundBody},
		{viaPDP: true, body: "", containerOnly: true},
	} {
		for _, call := range []struct {
			route, pdpRoute, op string
			call                func(s *RoleAssignmentsService) (bool, error)
		}{
			{
				route: bulkAssignRoleRoute, pdpRoute: pdpBulkAssignRoleRoute, op: opBulkAssignRole,
				call: func(s *RoleAssignmentsService) (bool, error) {
					report, err := s.BulkAssign(t.Context(),
						[]models.RoleAssignmentCreate{editorAssignment}, nil)
					return report != nil, err
				},
			},
			{
				route: bulkUnassignRoleRoute, pdpRoute: pdpBulkUnassignRoleRoute,
				op: opBulkUnassignRole,
				call: func(s *RoleAssignmentsService) (bool, error) {
					report, err := s.BulkUnassign(t.Context(),
						[]models.RoleAssignmentRemove{editorRemoval})
					return report != nil, err
				},
			},
		} {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			to, route := api, call.route
			opts := []Option{WithScope("p", "e"), WithRetry(RetryPolicy{MaxRetries: 1})}
			if tc.viaPDP {
				to, route = pdp, call.pdpRoute
				opts = append(opts, WithFactsViaPDP())
			}
			to.Handle(route, mockpermit.Reply{Status: http.StatusNotFound, Body: tc.body})
			got, err := call.call(decisionClient(t, api, pdp, opts...).RoleAssignments)
			e := mustError(t, err)
			if got || e.StatusCode != http.StatusNotFound || e.Op != call.op ||
				errors.Is(err, ErrNotFound) == tc.containerOnly ||
				errors.Is(err, ErrContainerPDPOnly) != tc.containerOnly {
				t.Errorf("via the PDP %t: %s of a 404 with the body %q returned a report: %t, "+
					"and %v; want an *Error that matches ErrContainerPDPOnly: %t, and else "+
					"ErrNotFound", tc.viaPDP, call.op, tc.body, got, err, tc.containerOnly)
			}
			if n := len(to.Requests()); n != 1 {
				t.Errorf("via the PDP %t: %s of a 404 sent %d requests, want 1", tc.viaPDP,
					call.op, n)
			}
		}
	}
}

// TestBulkAssignMissingUserPolicy checks the query of BulkAssign, through the API and through the
// PDP container: missing_user_policy is left out for a nil opts and for the zero options, and
// else holds the policy as it is given, each that the SDK knows and one that it does not, which
// the query escapes.
func TestBulkAssignMissingUserPolicy(t *testing.T) {
	policies := []*RoleAssignmentsBulkAssignOptions{
		nil,
		{},
		{MissingUserPolicy: models.MissingUserPolicyFail},
		{MissingUserPolicy: models.MissingUserPolicyIgnore},
		{MissingUserPolicy: models.MissingUserPolicyCreate},
		{MissingUserPolicy: "added later&x"},
	}
	want := []string{
		"", "", "missing_user_policy=fail", "missing_user_policy=ignore",
		"missing_user_policy=create", "missing_user_policy=added+later%26x",
	}
	for _, viaPDP := range []bool{false, true} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		to, route := api, bulkAssignRoleRoute
		opts := []Option{WithScope("p", "e")}
		if viaPDP {
			to, route = pdp, pdpBulkAssignRoleRoute
			opts = append(opts, WithFactsViaPDP())
		}
		replies := make([]mockpermit.Reply, len(policies))
		for i := range replies {
			replies[i] = mockpermit.Reply{Body: bulkAssignedBody}
		}
		to.Handle(route, replies...)
		s := decisionClient(t, api, pdp, opts...).RoleAssignments
		for _, policy := range policies {
			report, err := s.BulkAssign(t.Context(),
				[]models.RoleAssignmentCreate{editorAssignment}, policy)
			if err != nil || report.AssignmentsCreated != 1 {
				t.Fatalf("via the PDP %t: BulkAssign with %+v returned %+v, %v", viaPDP, policy,
					report, err)
			}
		}
		if diff := cmp.Diff(want, queries(to)); diff != "" {
			t.Errorf("via the PDP %t: the queries differ (-want +got):\n%s", viaPDP, diff)
		}
	}
}

// TestRoleAssignmentListsIgnoreFieldsTheyDoNotKnow checks that each list reads its own shape and
// skips a field that its model does not have, at the top of an item and inside one, as the API
// may add fields: List reads a role assignment with the keys of its user, its role and its
// tenant, and ListDetailed one with them as objects, each in either list shape that its route
// answers.
func TestRoleAssignmentListsIgnoreFieldsTheyDoNotKnow(t *testing.T) {
	const plain = `{"id":"a1","user":"alice","role":"editor","tenant":"tenant-a",` +
		`"resource_instance":"document:doc-1","added_later":{"x":[1]}}`
	const detailed = `{"id":"a1","user":{"key":"alice","added_later":1},` +
		`"role":{"key":"editor","name":"Editor","added_later":true},` +
		`"tenant":{"key":"tenant-a","name":"A"},` +
		`"resource_instance":{"key":"doc-1","resource":"document","added_later":null},` +
		`"added_later":"x"}`
	api := mockpermit.NewAPI(t)
	api.Handle(listRoleAssignmentsRoute, mockpermit.Reply{Body: `[` + plain + `]`},
		mockpermit.Reply{Body: `{"data":[` + plain + `],"total_count":1,"added_later":0}`})
	api.Handle(listDetailedRoute, mockpermit.Reply{Body: `{"data":[` + detailed + `]}`})
	s := userClient(t, api).RoleAssignments
	want := models.RoleAssignmentRead{
		ID: "a1", User: "alice", Role: "editor", Tenant: "tenant-a",
		ResourceInstance: "document:doc-1",
	}
	for range 2 {
		page, err := s.List(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff([]models.RoleAssignmentRead{want}, page.Data); diff != "" {
			t.Errorf("List read role assignments that differ (-want +got):\n%s", diff)
		}
	}
	page, err := s.ListDetailed(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	wantDetailed := models.RoleAssignmentDetailedRead{
		ID: "a1", User: models.RoleAssignmentUser{Key: "alice"},
		Role:   models.RoleAssignmentRole{Key: "editor", Name: "Editor"},
		Tenant: models.RoleAssignmentTenant{Key: "tenant-a", Name: "A"},
		ResourceInstance: &models.RoleAssignmentResourceInstance{
			Key: "doc-1", Resource: "document",
		},
	}
	if diff := cmp.Diff([]models.RoleAssignmentDetailedRead{wantDetailed}, page.Data); diff != "" {
		t.Errorf("ListDetailed read role assignments that differ (-want +got):\n%s", diff)
	}
}

// TestRoleAssignmentsAfterClose checks that each method returns ErrClosed after Close, and sends
// nothing, not even the lookup of the scope: also each method of a copy of WaitForSync whose
// wait is not valid, which returns ErrInvalidConfig before Close, the lists with options that
// are not valid, BulkAssign with a policy that the SDK does not know, which it sends as given,
// and BulkUnassign.
func TestRoleAssignmentsAfterClose(t *testing.T) {
	invalid := &RoleAssignmentsListOptions{ListOptions: ListOptions{PerPage: -1}}
	for _, opts := range [][]Option{nil, {WithScope("p", "e")}, {WithFactsViaPDP()}} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		c := decisionClient(t, api, pdp, opts...)
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		ctx := t.Context()
		for _, s := range []*RoleAssignmentsService{
			c.RoleAssignments, c.RoleAssignments.WaitForSync(-time.Second),
		} {
			var errs []error
			for _, listOpts := range []*RoleAssignmentsListOptions{nil, invalid} {
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
			_, assignErr := s.Assign(ctx, editorAssignment)
			_, bulkAssignErr := s.BulkAssign(ctx, []models.RoleAssignmentCreate{editorAssignment},
				&RoleAssignmentsBulkAssignOptions{MissingUserPolicy: "added_later"})
			_, bulkUnassignErr := s.BulkUnassign(ctx, []models.RoleAssignmentRemove{editorRemoval})
			errs = append(errs, assignErr, bulkAssignErr, bulkUnassignErr)
			for i, err := range append(errs, s.Unassign(ctx, editorRemoval)) {
				if !errors.Is(err, ErrClosed) {
					t.Errorf("call %d after Close returned %v, want ErrClosed", i, err)
				}
			}
		}
		api.AssertUntouched()
		pdp.AssertUntouched()
	}
}

// roleAssignmentWrites are the calls of the role assignments that the PDP container waits for
// sync on, each with its route on the container, its operation and its reply.
var roleAssignmentWrites = []struct {
	route string
	op    string
	reply mockpermit.Reply
	call  func(ctx context.Context, s *RoleAssignmentsService) error
}{
	{
		route: pdpAssignRoleRoute, op: opAssignRole, reply: mockpermit.Reply{Body: assignmentBody},
		call: func(ctx context.Context, s *RoleAssignmentsService) error {
			_, err := s.Assign(ctx, editorAssignment)
			return err
		},
	},
	{
		route: pdpUnassignRoleRoute, op: opUnassignRole,
		reply: mockpermit.Reply{Status: http.StatusNoContent},
		call: func(ctx context.Context, s *RoleAssignmentsService) error {
			return s.Unassign(ctx, editorRemoval)
		},
	},
}

// TestRoleAssignmentsWaitForSync checks the assignments and the unassignments of a copy of
// WaitForSync through the PDP container: each carries the timeout in decimal seconds and the
// policy of WithSyncPolicy, and the calls of the service that WaitForSync was called on carry
// neither, before and after.
func TestRoleAssignmentsWaitForSync(t *testing.T) {
	for _, w := range roleAssignmentWrites {
		t.Run(w.op, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			pdp.Handle(w.route, w.reply, w.reply, w.reply)
			plain := decisionClient(t, api, pdp, WithFactsViaPDP()).RoleAssignments
			waiting := plain.WaitForSync(10*time.Second, WithSyncPolicy(SyncPolicyIgnore))
			for _, s := range []*RoleAssignmentsService{plain, waiting, plain} {
				if err := w.call(t.Context(), s); err != nil {
					t.Fatal(err)
				}
			}
			var got []string
			for _, r := range pdp.Requests() {
				got = append(got, waitHeaders(r.Header))
			}
			if diff := cmp.Diff([]string{"- -", "10 ignore", "- -"}, got); diff != "" {
				t.Errorf("the wait headers of the three calls differ (-want +got):\n%s", diff)
			}
			api.AssertUntouched()
		})
	}
}

// TestRoleAssignmentsWaitForSyncMisuse checks the calls through a copy of WaitForSync that
// cannot wait: List, All, ListDetailed and AllDetailed, and BulkAssign and BulkUnassign, which
// the PDP container answers without waiting; each bulk call from a client that sends facts to
// the API too; and Assign and Unassign from a client that sends facts to the API, or with a
// negative timeout. Each returns an error that matches ErrInvalidConfig and names the operation
// and the rule, and sends no request, not even the lookup of the scope.
func TestRoleAssignmentsWaitForSyncMisuse(t *testing.T) {
	type misuse struct {
		name    string
		viaPDP  bool
		timeout time.Duration
		op      string
		call    func(ctx context.Context, s *RoleAssignmentsService) error
		rule    validate.Rule
	}
	cases := []misuse{
		{
			name: "List", viaPDP: true, op: opListRoleAssignments,
			rule: validate.WaitForSyncNotWaiting,
			call: func(ctx context.Context, s *RoleAssignmentsService) error {
				_, err := s.List(ctx, nil)
				return err
			},
		},
		{
			name: "All", viaPDP: true, op: opListRoleAssignments,
			rule: validate.WaitForSyncNotWaiting,
			call: func(ctx context.Context, s *RoleAssignmentsService) error {
				for _, err := range s.All(ctx, nil) {
					return err
				}
				return nil
			},
		},
		{
			name: "ListDetailed", viaPDP: true, op: opListRoleAssignmentsDetailed,
			rule: validate.WaitForSyncNotWaiting,
			call: func(ctx context.Context, s *RoleAssignmentsService) error {
				_, err := s.ListDetailed(ctx, nil)
				return err
			},
		},
		{
			name: "AllDetailed", viaPDP: true, op: opListRoleAssignmentsDetailed,
			rule: validate.WaitForSyncNotWaiting,
			call: func(ctx context.Context, s *RoleAssignmentsService) error {
				for _, err := range s.AllDetailed(ctx, nil) {
					return err
				}
				return nil
			},
		},
	}
	bulkAssign := func(ctx context.Context, s *RoleAssignmentsService) error {
		_, err := s.BulkAssign(ctx, []models.RoleAssignmentCreate{editorAssignment},
			&RoleAssignmentsBulkAssignOptions{MissingUserPolicy: models.MissingUserPolicyCreate})
		return err
	}
	bulkUnassign := func(ctx context.Context, s *RoleAssignmentsService) error {
		_, err := s.BulkUnassign(ctx, []models.RoleAssignmentRemove{editorRemoval})
		return err
	}
	for _, viaPDP := range []bool{true, false} {
		rule := validate.WaitForSyncNotWaiting
		if !viaPDP {
			rule = validate.WaitForSyncFactsViaAPI
		}
		cases = append(cases,
			misuse{
				name: "BulkAssign via the PDP " + strconv.FormatBool(viaPDP), viaPDP: viaPDP,
				op: opBulkAssignRole, call: bulkAssign, rule: rule,
			},
			misuse{
				name: "BulkUnassign via the PDP " + strconv.FormatBool(viaPDP), viaPDP: viaPDP,
				op: opBulkUnassignRole, call: bulkUnassign, rule: rule,
			})
	}
	for _, w := range roleAssignmentWrites {
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
			s := decisionClient(t, api, pdp, opts...).RoleAssignments.WaitForSync(tc.timeout)
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
