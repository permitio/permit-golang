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

// The routes of the relationship tuples on the API, under the scope p and e of userClient, and on
// the PDP container, as mockpermit.Server.Handle takes them.
const (
	listTuplesRoute            = "GET /v2/facts/{proj_id}/{env_id}/relationship_tuples"
	listDetailedTuplesRoute    = "GET /v2/facts/{proj_id}/{env_id}/relationship_tuples/detailed"
	createTupleRoute           = "POST /v2/facts/{proj_id}/{env_id}/relationship_tuples"
	deleteTupleRoute           = "DELETE /v2/facts/{proj_id}/{env_id}/relationship_tuples"
	bulkCreateTuplesRoute      = "POST /v2/facts/{proj_id}/{env_id}/relationship_tuples/bulk"
	bulkDeleteTuplesRoute      = "DELETE /v2/facts/{proj_id}/{env_id}/relationship_tuples/bulk"
	pdpListTuplesRoute         = "GET /facts/relationship_tuples"
	pdpListDetailedTuplesRoute = "GET /facts/relationship_tuples/detailed"
	pdpCreateTupleRoute        = "POST /facts/relationship_tuples"
	pdpDeleteTupleRoute        = "DELETE /facts/relationship_tuples"
	pdpBulkCreateTuplesRoute   = "POST /facts/relationship_tuples/bulk"
	pdpBulkDeleteTuplesRoute   = "DELETE /facts/relationship_tuples/bulk"
)

// tupleBody is a reply of the API to a create of a relationship tuple, and missingTupleBody one
// to a delete of a relationship tuple that the environment does not have.
const (
	tupleBody = `{"id":"t1","subject":"folder:f1","relation":"parent",` +
		`"object":"document:doc-1","tenant":"tenant-a"}`
	missingTupleBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the environment has no such relationship tuple"}`
)

// parentTuple is the relationship tuple of the tests of the relationship tuples, folder:f1 parent
// document:doc-1 in the tenant tenant-a, and parentTupleKey names it for a delete.
var (
	parentTuple = models.RelationshipTupleCreate{
		Subject: "folder:f1", Relation: "parent", Object: "document:doc-1", Tenant: "tenant-a",
	}
	parentTupleKey = models.RelationshipTupleDelete{
		Subject: "folder:f1", Relation: "parent", Object: "document:doc-1",
	}
)

// tuplesOf returns the relationship tuples of the subjects folder:s<from> to folder:s<to-1>,
// with only a subject: an empty list when to is from.
func tuplesOf(from, to int) []models.RelationshipTupleRead {
	items := make([]models.RelationshipTupleRead, 0, to-from)
	for i := from; i < to; i++ {
		items = append(items, models.RelationshipTupleRead{Subject: "folder:s" + strconv.Itoa(i)})
	}
	return items
}

// detailedTuplesOf returns the tuples of tuplesOf as those of the detailed list.
func detailedTuplesOf(from, to int) []models.RelationshipTupleDetailedRead {
	items := make([]models.RelationshipTupleDetailedRead, 0, to-from)
	for _, t := range tuplesOf(from, to) {
		items = append(items, models.RelationshipTupleDetailedRead(t))
	}
	return items
}

// tupleTarget returns the server that a call of the relationship tuples goes to, the route of
// apiRoute or pdpRoute on it, and the options of a client that sends it there: the API with the
// scope p and e, or, viaPDP, the PDP container with WithFactsViaPDP.
func tupleTarget(api, pdp *mockpermit.Server, viaPDP bool, apiRoute, pdpRoute string,
) (*mockpermit.Server, string, []Option) {
	if viaPDP {
		return pdp, pdpRoute, []Option{WithScope("p", "e"), WithFactsViaPDP()}
	}
	return api, apiRoute, []Option{WithScope("p", "e")}
}

// TestRelationshipTupleListsReturnAnEmptyPage checks that a page without relationship tuples, a
// JSON array or a paginated object whose data is empty, is a page whose Data is empty and not
// nil, for List and ListDetailed, through the API and through the PDP container.
func TestRelationshipTupleListsReturnAnEmptyPage(t *testing.T) {
	for _, viaPDP := range []bool{false, true} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		to, list, opts := tupleTarget(api, pdp, viaPDP, listTuplesRoute, pdpListTuplesRoute)
		_, detailed, _ := tupleTarget(api, pdp, viaPDP, listDetailedTuplesRoute,
			pdpListDetailedTuplesRoute)
		empty := mockpermit.Reply{Body: `{"data":[],"total_count":0,"page_count":0}`}
		to.Handle(list, mockpermit.Reply{Body: `[]`}, empty)
		to.Handle(detailed, empty)
		s := decisionClient(t, api, pdp, opts...).RelationshipTuples
		for i := range 2 {
			page, err := s.List(t.Context(), &RelationshipTuplesListOptions{Subject: "folder:x"})
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

// TestRelationshipTuplesAllReadsEveryPage checks that All and AllDetailed read the 101
// relationship tuples of a list, in order, asking for each page in turn at the default per_page
// of 100, the largest that the routes accept: All, whose replies are JSON arrays, asks for the
// pages of 100 and of 1 and for the empty page after them, and AllDetailed, whose paginated
// replies report a total_count of 101, for the two pages alone.
func TestRelationshipTuplesAllReadsEveryPage(t *testing.T) {
	hundred := []string{"page=1&per_page=100", "page=2&per_page=100", "page=3&per_page=100"}
	t.Run("All", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(listTuplesRoute, arrayOf(t, tuplesOf(0, 100)), arrayOf(t, tuplesOf(100, 101)),
			arrayOf(t, tuplesOf(0, 0)))
		items, errs := collect(t, userClient(t, api).RelationshipTuples.All(t.Context(), nil))
		if diff := cmp.Diff(tuplesOf(0, 101), items); diff != "" || errs != nil {
			t.Errorf("All yielded the errors %v and relationship tuples that differ "+
				"(-want +got):\n%s", errs, diff)
		}
		if diff := cmp.Diff(hundred, queries(api)); diff != "" {
			t.Errorf("the queries differ (-want +got):\n%s", diff)
		}
	})
	t.Run("AllDetailed", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(listDetailedTuplesRoute,
			pageOf(t, detailedTuplesOf(0, 100), `,"total_count":101,"page_count":100`),
			pageOf(t, detailedTuplesOf(100, 101), `,"total_count":101,"page_count":1`))
		items, errs := collect(t,
			userClient(t, api).RelationshipTuples.AllDetailed(t.Context(), nil))
		if diff := cmp.Diff(detailedTuplesOf(0, 101), items); diff != "" || errs != nil {
			t.Errorf("AllDetailed yielded the errors %v and relationship tuples that differ "+
				"(-want +got):\n%s", errs, diff)
		}
		if diff := cmp.Diff(hundred[:2], queries(api)); diff != "" {
			t.Errorf("the queries differ (-want +got):\n%s", diff)
		}
	})
}

// TestRelationshipTuplesAllReadsTheOptionsOnce checks that the iterators of All and AllDetailed
// keep the options of their call, the filters too, so that a later change to them changes no
// request; and that each loop reads the relationship tuples again from the first page, along the
// same path, through the API and through the PDP container.
func TestRelationshipTuplesAllReadsTheOptionsOnce(t *testing.T) {
	for _, viaPDP := range []bool{false, true} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		to, list, opts := tupleTarget(api, pdp, viaPDP, listTuplesRoute, pdpListTuplesRoute)
		_, detailed, _ := tupleTarget(api, pdp, viaPDP, listDetailedTuplesRoute,
			pdpListDetailedTuplesRoute)
		to.Handle(list, arrayOf(t, tuplesOf(0, 2)), arrayOf(t, tuplesOf(0, 2)))
		two := pageOf(t, detailedTuplesOf(0, 2), `,"total_count":9`)
		to.Handle(detailed, two, two)
		options := &RelationshipTuplesListOptions{
			ListOptions: ListOptions{PerPage: 2}, Tenant: "t", Subject: "folder:s0",
			Relation: "r", Object: "document:d", SubjectType: "folder", ObjectType: "document",
		}
		s := decisionClient(t, api, pdp, opts...).RelationshipTuples
		all, allDetailed := s.All(t.Context(), options), s.AllDetailed(t.Context(), options)
		*options = RelationshipTuplesListOptions{ListOptions: ListOptions{Page: 5, PerPage: 50}}
		for loop := 1; loop <= 2; loop++ {
			for tuple, err := range all {
				if err != nil || tuple.Subject != "folder:s0" {
					t.Errorf("via the PDP %t: loop %d of All yielded %+v and %v first", viaPDP,
						loop, tuple, err)
				}
				break
			}
			for tuple, err := range allDetailed {
				if err != nil || tuple.Subject != "folder:s0" {
					t.Errorf("via the PDP %t: loop %d of AllDetailed yielded %+v and %v first",
						viaPDP, loop, tuple, err)
				}
				break
			}
		}
		path := strings.TrimPrefix(strings.Replace(list, "{proj_id}/{env_id}", "p/e", 1), "GET ")
		query := "?object=document%3Ad&object_type=document&page=1&per_page=2&relation=r&" +
			"subject=folder%3As0&subject_type=folder&tenant=t"
		first, firstDetailed := path+query, path+"/detailed"+query
		var got []string
		for _, r := range to.Requests() {
			got = append(got, r.RawPath+"?"+r.RawQuery)
		}
		want := []string{first, firstDetailed, first, firstDetailed}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("via the PDP %t: the requests differ (-want +got):\n%s", viaPDP, diff)
		}
	}
}

// TestRelationshipTupleFilters checks the query of List and ListDetailed for each filter alone:
// a filter that is empty is not sent, a filter that is set is sent as it is given, and neither
// list sends the deprecated query parameters detailed and include_total_count.
func TestRelationshipTupleFilters(t *testing.T) {
	for _, tc := range []struct {
		opts *RelationshipTuplesListOptions
		want string
	}{
		{nil, ""},
		{&RelationshipTuplesListOptions{}, ""},
		{&RelationshipTuplesListOptions{Tenant: "t1"}, "tenant=t1"},
		{&RelationshipTuplesListOptions{Subject: "folder:a b"}, "subject=folder%3Aa+b"},
		{&RelationshipTuplesListOptions{Relation: "parent"}, "relation=parent"},
		{&RelationshipTuplesListOptions{Object: "document:d&1"}, "object=document%3Ad%261"},
		{&RelationshipTuplesListOptions{SubjectType: "folder"}, "subject_type=folder"},
		{&RelationshipTuplesListOptions{ObjectType: "document"}, "object_type=document"},
		{
			&RelationshipTuplesListOptions{ListOptions: ListOptions{Page: 3}, Relation: "r"},
			"page=3&relation=r",
		},
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(listTuplesRoute, mockpermit.Reply{Body: `[]`})
		api.Handle(listDetailedTuplesRoute, mockpermit.Reply{Body: `{"data":[],"total_count":0}`})
		s := userClient(t, api).RelationshipTuples
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

// TestRelationshipTuplesScope checks where the calls of the relationship tuples go without
// WithScope: to the project and the environment of the API key, which the first call looks up
// once; and, with the scope of a project-level key, which has no environment, to no route of the
// relationship tuples, with an error that matches ErrInvalidConfig.
func TestRelationshipTuplesScope(t *testing.T) {
	t.Run("the scope of the API key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		api.Handle(createTupleRoute, mockpermit.Reply{Body: tupleBody})
		api.Handle(listDetailedTuplesRoute, mockpermit.Reply{Body: `{"data":[],"total_count":0}`})
		c := decisionClient(t, api, mockpermit.NewPDP(t))
		if _, err := c.RelationshipTuples.Create(t.Context(), parentTuple); err != nil {
			t.Fatal(err)
		}
		if _, err := c.RelationshipTuples.ListDetailed(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
		tuples := "/v2/facts/" + scopeProjectID + "/" + scopeEnvironmentID +
			"/relationship_tuples"
		var got []string
		for _, r := range api.Requests() {
			got = append(got, r.Method+" "+r.RawPath)
		}
		want := []string{"GET /v2/api-key/scope", "POST " + tuples, "GET " + tuples + "/detailed"}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("the requests differ (-want +got):\n%s", diff)
		}
	})
	t.Run("a scope without an environment", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ".project"))
		c := decisionClient(t, api, mockpermit.NewPDP(t))
		err := c.RelationshipTuples.Delete(t.Context(), parentTupleKey)
		want := "permit: invalid configuration: delete_relationship_tuple: " +
			validate.ScopeEnvironmentMissing.Message
		if !errors.Is(err, ErrInvalidConfig) || err.Error() != want {
			t.Errorf("Delete returned %v, want %q", err, want)
		}
		if n := len(api.Requests()); n != 1 {
			t.Errorf("the API got %d requests, want the lookup alone", n)
		}
	})
}

// TestRelationshipTupleDeleteReplies checks the replies to Delete, through the API and through
// the PDP container: nil for a 204 without a body, and for a 200 with a relationship tuple, with
// an empty body or with a body that is not a relationship tuple, which it does not read; and, as
// its body names the relationship tuple, an *Error that matches ErrNotFound, never nil, for a 404
// of the API, with or without a body, and of the container with a body, each after one request,
// also with a retry policy that allows one more; and, through the container, an error that
// matches ErrContainerPDPOnly, not ErrNotFound, for a 404 with an empty body.
func TestRelationshipTupleDeleteReplies(t *testing.T) {
	replies := []mockpermit.Reply{
		{Status: http.StatusNoContent},
		{Status: http.StatusOK, Body: tupleBody},
		{Status: http.StatusOK, Body: ""},
		{Status: http.StatusOK, Body: `[1]`},
	}
	for _, viaPDP := range []bool{false, true} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		to, route, opts := tupleTarget(api, pdp, viaPDP, deleteTupleRoute, pdpDeleteTupleRoute)
		to.Handle(route, replies...)
		s := decisionClient(t, api, pdp, opts...).RelationshipTuples
		for i, reply := range replies {
			if err := deleteTuple(t.Context(), s); err != nil {
				t.Errorf("via the PDP %t: Delete of the reply %d, %d %q, returned %v, want nil",
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
		{viaPDP: false, body: missingTupleBody},
		{viaPDP: false, body: ""},
		{viaPDP: true, body: missingTupleBody},
		{viaPDP: true, body: "", containerOnly: true},
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		to, route, opts := tupleTarget(api, pdp, tc.viaPDP, deleteTupleRoute, pdpDeleteTupleRoute)
		to.Handle(route, mockpermit.Reply{Status: http.StatusNotFound, Body: tc.body})
		c := decisionClient(t, api, pdp, append(opts, WithRetry(RetryPolicy{MaxRetries: 1}))...)
		err := deleteTuple(t.Context(), c.RelationshipTuples)
		e := mustError(t, err)
		if e.StatusCode != http.StatusNotFound || e.Op != opDeleteRelationshipTuple ||
			errors.Is(err, ErrNotFound) == tc.containerOnly ||
			errors.Is(err, ErrContainerPDPOnly) != tc.containerOnly {
			t.Errorf("via the PDP %t: Delete of a 404 with the body %q returned %v, want an "+
				"*Error that matches ErrContainerPDPOnly: %t, and else ErrNotFound", tc.viaPDP,
				tc.body, err, tc.containerOnly)
		}
		if n := len(to.Requests()); n != 1 {
			t.Errorf("via the PDP %t: Delete of a 404 sent %d requests, want 1", tc.viaPDP, n)
		}
	}
}

// bulkTupleCalls are the bulk calls of the relationship tuples, each with its routes on the API
// and on the PDP container and its operation; the PDP container waits on neither.
var bulkTupleCalls = []struct {
	name, route, pdpRoute, op string
	call                      func(ctx context.Context, s *RelationshipTuplesService) error
}{
	{
		name: "BulkCreate", route: bulkCreateTuplesRoute, pdpRoute: pdpBulkCreateTuplesRoute,
		op: opBulkCreateRelationshipTuples,
		call: func(ctx context.Context, s *RelationshipTuplesService) error {
			return s.BulkCreate(ctx, []models.RelationshipTupleCreate{parentTuple})
		},
	},
	{
		name: "BulkDelete", route: bulkDeleteTuplesRoute, pdpRoute: pdpBulkDeleteTuplesRoute,
		op: opBulkDeleteRelationshipTuples,
		call: func(ctx context.Context, s *RelationshipTuplesService) error {
			return s.BulkDelete(ctx, []models.RelationshipTupleDelete{parentTupleKey})
		},
	},
}

// TestBulkRelationshipTupleReplies checks the replies in 200-299 to BulkCreate and BulkDelete,
// through the API and through the PDP container: nil for a 204 without a body, and for a 200
// with the reply fixture, with an empty body, with a body that is not JSON or with a JSON value
// of another kind, and for a 201 with an object of fields that the spec does not list, none of
// which the calls read, one request each.
func TestBulkRelationshipTupleReplies(t *testing.T) {
	replies := []mockpermit.Reply{
		{Status: http.StatusNoContent},
		{Body: replyFixtureOf(t, "bulk_create_relationship_tuples.200.json")},
		{Body: ""},
		{Body: `not JSON`},
		{Body: `[1]`},
		{Status: http.StatusCreated, Body: `{"created":2}`},
	}
	for _, viaPDP := range []bool{false, true} {
		for _, call := range bulkTupleCalls {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			to, route, opts := tupleTarget(api, pdp, viaPDP, call.route, call.pdpRoute)
			to.Handle(route, replies...)
			s := decisionClient(t, api, pdp, opts...).RelationshipTuples
			for i, reply := range replies {
				if err := call.call(t.Context(), s); err != nil {
					t.Errorf("via the PDP %t: %s of the reply %d, %d %q, returned %v, want nil",
						viaPDP, call.name, i, reply.Status, reply.Body, err)
				}
			}
			if n := len(to.Requests()); n != len(replies) {
				t.Errorf("via the PDP %t: %d calls of %s sent %d requests", viaPDP, len(replies),
					call.name, n)
			}
		}
	}
}

// TestBulkRelationshipTupleNotFound checks the 404 of BulkCreate and BulkDelete, through the API
// and through the PDP container: as the body names the relationship tuples, an *Error that
// matches ErrNotFound, never nil, for a 404 of the API, with or without a body, and of the
// container with a body, after one request, also with a retry policy that allows one more; and,
// through the container, an error that matches ErrContainerPDPOnly, not ErrNotFound, for a 404
// with an empty body.
func TestBulkRelationshipTupleNotFound(t *testing.T) {
	for _, tc := range []struct {
		viaPDP        bool
		body          string
		containerOnly bool
	}{
		{viaPDP: false, body: missingTupleBody},
		{viaPDP: false, body: ""},
		{viaPDP: true, body: missingTupleBody},
		{viaPDP: true, body: "", containerOnly: true},
	} {
		for _, call := range bulkTupleCalls {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			to, route, opts := tupleTarget(api, pdp, tc.viaPDP, call.route, call.pdpRoute)
			to.Handle(route, mockpermit.Reply{Status: http.StatusNotFound, Body: tc.body})
			c := decisionClient(t, api, pdp, append(opts, WithRetry(RetryPolicy{MaxRetries: 1}))...)
			err := call.call(t.Context(), c.RelationshipTuples)
			e := mustError(t, err)
			if e.StatusCode != http.StatusNotFound || e.Op != call.op ||
				errors.Is(err, ErrNotFound) == tc.containerOnly ||
				errors.Is(err, ErrContainerPDPOnly) != tc.containerOnly {
				t.Errorf("via the PDP %t: %s of a 404 with the body %q returned %v, want an "+
					"*Error that matches ErrContainerPDPOnly: %t, and else ErrNotFound",
					tc.viaPDP, call.name, tc.body, err, tc.containerOnly)
			}
			if n := len(to.Requests()); n != 1 {
				t.Errorf("via the PDP %t: %s of a 404 sent %d requests, want 1", tc.viaPDP,
					call.name, n)
			}
		}
	}
}

// TestRelationshipTupleBodies checks the bodies of Create, Delete, BulkCreate and BulkDelete,
// byte for byte against the golden files in testdata/golden: a create with a tenant and one
// without, which leaves the tenant out, not empty; the zero create and the zero delete, which
// hold the subject, the relation and the object, empty; and the JSON object of each bulk call,
// whose operations or idents hold the items in order, with <, > and & as they are and the zero
// item too, and [] for nil and for an empty slice.
func TestRelationshipTupleBodies(t *testing.T) {
	created := func(tuple models.RelationshipTupleCreate) func(s *RelationshipTuplesService) error {
		return func(s *RelationshipTuplesService) error {
			_, err := s.Create(t.Context(), tuple)
			return err
		}
	}
	deleted := func(tuple models.RelationshipTupleDelete) func(s *RelationshipTuplesService) error {
		return func(s *RelationshipTuplesService) error { return s.Delete(t.Context(), tuple) }
	}
	bulkCreated := func(tuples []models.RelationshipTupleCreate,
	) func(s *RelationshipTuplesService) error {
		return func(s *RelationshipTuplesService) error { return s.BulkCreate(t.Context(), tuples) }
	}
	bulkDeleted := func(tuples []models.RelationshipTupleDelete,
	) func(s *RelationshipTuplesService) error {
		return func(s *RelationshipTuplesService) error { return s.BulkDelete(t.Context(), tuples) }
	}
	noTenant := parentTuple
	noTenant.Tenant = ""
	other := models.RelationshipTupleCreate{
		Subject: "folder:<f&2>", Relation: "parent", Object: "document:doc-2",
	}
	otherKey := models.RelationshipTupleDelete{
		Subject: other.Subject, Relation: other.Relation, Object: other.Object,
	}
	ok := mockpermit.Reply{Body: tupleBody}
	gone := mockpermit.Reply{Status: http.StatusNoContent}
	for _, tc := range []struct {
		golden string
		route  string
		reply  mockpermit.Reply
		call   func(s *RelationshipTuplesService) error
	}{
		{"create_relationship_tuple.tenant.golden", createTupleRoute, ok, created(parentTuple)},
		{"create_relationship_tuple.no-tenant.golden", createTupleRoute, ok, created(noTenant)},
		{
			"create_relationship_tuple.empty.golden", createTupleRoute, ok,
			created(models.RelationshipTupleCreate{}),
		},
		{"delete_relationship_tuple.tuple.golden", deleteTupleRoute, gone, deleted(parentTupleKey)},
		{
			"delete_relationship_tuple.empty.golden", deleteTupleRoute, gone,
			deleted(models.RelationshipTupleDelete{}),
		},
		{
			"bulk_create_relationship_tuples.items.golden", bulkCreateTuplesRoute, gone,
			bulkCreated([]models.RelationshipTupleCreate{parentTuple, other, {}}),
		},
		{
			"bulk_create_relationship_tuples.empty.golden", bulkCreateTuplesRoute, gone,
			bulkCreated(nil),
		},
		{
			"bulk_create_relationship_tuples.empty.golden", bulkCreateTuplesRoute, gone,
			bulkCreated([]models.RelationshipTupleCreate{}),
		},
		{
			"bulk_delete_relationship_tuples.items.golden", bulkDeleteTuplesRoute, gone,
			bulkDeleted([]models.RelationshipTupleDelete{parentTupleKey, otherKey, {}}),
		},
		{
			"bulk_delete_relationship_tuples.empty.golden", bulkDeleteTuplesRoute, gone,
			bulkDeleted(nil),
		},
		{
			"bulk_delete_relationship_tuples.empty.golden", bulkDeleteTuplesRoute, gone,
			bulkDeleted([]models.RelationshipTupleDelete{}),
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, tc.reply)
			if err := tc.call(userClient(t, api).RelationshipTuples); err != nil {
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

// TestRelationshipTupleRetries checks when a call of the relationship tuples sends its request
// again: Create and BulkCreate, which the API may have acted on, after a 429 only, so a 503 ends
// each after one request; Delete, BulkDelete, List and ListDetailed, which do the same when sent
// twice, after a 503 too.
func TestRelationshipTupleRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	gone := mockpermit.Reply{Status: http.StatusNoContent}
	create := func(s *RelationshipTuplesService) error {
		_, err := s.Create(t.Context(), parentTuple)
		return err
	}
	bulkCreate := func(s *RelationshipTuplesService) error {
		return s.BulkCreate(t.Context(), []models.RelationshipTupleCreate{parentTuple})
	}
	for _, tc := range []struct {
		name    string
		route   string
		replies []mockpermit.Reply
		call    func(s *RelationshipTuplesService) error
		status  int
	}{
		{
			name: "Create after a 429", route: createTupleRoute,
			replies: []mockpermit.Reply{limited, {Body: tupleBody}}, call: create,
		},
		{
			name: "Create after a 503", route: createTupleRoute,
			replies: []mockpermit.Reply{unavailable}, call: create,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "BulkCreate after a 429", route: bulkCreateTuplesRoute,
			replies: []mockpermit.Reply{limited, gone}, call: bulkCreate,
		},
		{
			name: "BulkCreate after a 503", route: bulkCreateTuplesRoute,
			replies: []mockpermit.Reply{unavailable}, call: bulkCreate,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "Delete after a 503", route: deleteTupleRoute,
			replies: []mockpermit.Reply{unavailable, gone},
			call: func(s *RelationshipTuplesService) error {
				return s.Delete(t.Context(), parentTupleKey)
			},
		},
		{
			name: "BulkDelete after a 503", route: bulkDeleteTuplesRoute,
			replies: []mockpermit.Reply{unavailable, gone},
			call: func(s *RelationshipTuplesService) error {
				return s.BulkDelete(t.Context(), []models.RelationshipTupleDelete{parentTupleKey})
			},
		},
		{
			name: "List after a 503", route: listTuplesRoute,
			replies: []mockpermit.Reply{unavailable, {Body: `[]`}},
			call: func(s *RelationshipTuplesService) error {
				_, err := s.List(t.Context(), nil)
				return err
			},
		},
		{
			name: "ListDetailed after a 503", route: listDetailedTuplesRoute,
			replies: []mockpermit.Reply{unavailable, {Body: `{"data":[],"total_count":0}`}},
			call: func(s *RelationshipTuplesService) error {
				_, err := s.ListDetailed(t.Context(), nil)
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, tc.replies...)
			c := userClient(t, api, WithRetry(RetryPolicy{MaxRetries: 1}))
			err := tc.call(c.RelationshipTuples)
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

// The bodies of a reply in 200-299 that are not a list of relationship tuples: for List and All,
// which read a JSON array or the paginated object, and for ListDetailed and AllDetailed, which
// read the paginated object alone, a JSON array too.
var (
	malformedTupleLists         = []string{``, `null`, `"a"`, `{"data":null}`, `[{"subject":1}]`}
	malformedDetailedTupleLists = []string{
		``, `null`, `[]`, `[{"subject":"a"}]`, `{"data":[{"subject":1}]}`,
	}
)

// TestMalformedRelationshipTupleReplies checks that a reply in 200-299 that is not a relationship
// tuple to Create, an empty body too, and a reply that is not a list of relationship tuples of
// the shape that each list's route answers, a JSON array or the paginated object to List and the
// paginated object alone to ListDetailed, is an *Error that names the model, with the status of
// the reply, and no result.
func TestMalformedRelationshipTupleReplies(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusCreated} {
		for _, body := range []string{``, `null`, `[]`, `"a"`, `{"subject":1}`, `{} {}`} {
			api := mockpermit.NewAPI(t)
			api.Handle(createTupleRoute, mockpermit.Reply{Status: status, Body: body})
			got, err := userClient(t, api).RelationshipTuples.Create(t.Context(), parentTuple)
			want := "status " + strconv.Itoa(status) +
				": the reply is not a valid models.RelationshipTupleRead"
			if e := mustError(t, err); got != nil || e.StatusCode != status ||
				!strings.HasSuffix(e.Error(), want) {
				t.Errorf("Create of the reply %d %q returned %+v and\n%v\nwant no result and an "+
					"error that ends with %q", status, body, got, err, want)
			}
		}
	}
	for _, tc := range []struct {
		route, model string
		bodies       []string
		call         func(s *RelationshipTuplesService) (bool, error)
	}{
		{
			route: listTuplesRoute, model: "models.RelationshipTupleRead",
			bodies: malformedTupleLists,
			call: func(s *RelationshipTuplesService) (bool, error) {
				page, err := s.List(t.Context(), nil)
				return page != nil, err
			},
		},
		{
			route: listDetailedTuplesRoute, model: "models.RelationshipTupleDetailedRead",
			bodies: malformedDetailedTupleLists,
			call: func(s *RelationshipTuplesService) (bool, error) {
				page, err := s.ListDetailed(t.Context(), nil)
				return page != nil, err
			},
		},
	} {
		for _, body := range tc.bodies {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: body})
			got, err := tc.call(userClient(t, api).RelationshipTuples)
			want := "status 200: the reply is not a valid list of " + tc.model
			if e := mustError(t, err); got || !strings.HasSuffix(e.Error(), want) {
				t.Errorf("%s of the reply %q returned a page: %t, and\n%v\nwant no page and an "+
					"error that ends with %q", tc.route, body, got, err, want)
			}
		}
	}
}

// TestRelationshipTupleIteratorsStopAtAMalformedReply checks that All and AllDetailed fail as
// List and ListDetailed do on a reply in 200-299 that is not a list of relationship tuples: All
// on a body that List rejects, and AllDetailed on one that ListDetailed rejects, a JSON array
// too. Each loop yields one *Error that names the model, with the status of the reply, and no
// relationship tuple.
func TestRelationshipTupleIteratorsStopAtAMalformedReply(t *testing.T) {
	for _, tc := range []struct {
		name, route, model string
		bodies             []string
		all                func(s *RelationshipTuplesService) (int, []error)
	}{
		{
			name: "All", route: listTuplesRoute, model: "models.RelationshipTupleRead",
			bodies: malformedTupleLists,
			all: func(s *RelationshipTuplesService) (int, []error) {
				tuples, errs := collect(t, s.All(t.Context(), nil))
				return len(tuples), errs
			},
		},
		{
			name: "AllDetailed", route: listDetailedTuplesRoute,
			model: "models.RelationshipTupleDetailedRead", bodies: malformedDetailedTupleLists,
			all: func(s *RelationshipTuplesService) (int, []error) {
				tuples, errs := collect(t, s.AllDetailed(t.Context(), nil))
				return len(tuples), errs
			},
		},
	} {
		for _, body := range tc.bodies {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: body})
			n, errs := tc.all(userClient(t, api).RelationshipTuples)
			want := "status 200: the reply is not a valid list of " + tc.model
			if n != 0 || len(errs) != 1 || mustError(t, errs[0]).StatusCode != http.StatusOK ||
				!strings.HasSuffix(errs[0].Error(), want) {
				t.Errorf("%s of the reply %q yielded %d relationship tuples and the errors %v, "+
					"want none and one error that ends with %q", tc.name, body, n, errs, want)
			}
		}
	}
}

// TestRelationshipTuplesAfterClose checks that each method returns ErrClosed after Close, and
// sends nothing, not even the lookup of the scope: also each method of a copy of WaitForSync
// whose wait is not valid, which returns ErrInvalidConfig before Close, and the lists with options
// that are not valid.
func TestRelationshipTuplesAfterClose(t *testing.T) {
	invalid := &RelationshipTuplesListOptions{ListOptions: ListOptions{PerPage: -1}}
	for _, opts := range [][]Option{nil, {WithScope("p", "e")}, {WithFactsViaPDP()}} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		c := decisionClient(t, api, pdp, opts...)
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		ctx := t.Context()
		for _, s := range []*RelationshipTuplesService{
			c.RelationshipTuples, c.RelationshipTuples.WaitForSync(-time.Second),
		} {
			var errs []error
			for _, listOpts := range []*RelationshipTuplesListOptions{nil, invalid} {
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
			_, createErr := s.Create(ctx, parentTuple)
			errs = append(errs, createErr, deleteTuple(ctx, s))
			for _, call := range bulkTupleCalls {
				errs = append(errs, call.call(ctx, s))
			}
			for i, err := range errs {
				if !errors.Is(err, ErrClosed) {
					t.Errorf("call %d after Close returned %v, want ErrClosed", i, err)
				}
			}
		}
		api.AssertUntouched()
		pdp.AssertUntouched()
	}
}

// createTuple is the call of the relationship tuples that the PDP container waits for sync on.
func createTuple(ctx context.Context, s *RelationshipTuplesService) error {
	_, err := s.Create(ctx, parentTuple)
	return err
}

// deleteTuple is the delete of the relationship tuple of the tests, which the PDP container
// answers without waiting.
func deleteTuple(ctx context.Context, s *RelationshipTuplesService) error {
	return s.Delete(ctx, parentTupleKey)
}

// TestRelationshipTuplesWaitForSync checks the creates of a copy of WaitForSync through the PDP
// container: each carries the timeout in decimal seconds and the policy of WithSyncPolicy, and
// the calls of the service that WaitForSync was called on carry neither, before and after.
func TestRelationshipTuplesWaitForSync(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	ok := mockpermit.Reply{Body: tupleBody}
	pdp.Handle(pdpCreateTupleRoute, ok, ok, ok)
	plain := decisionClient(t, api, pdp, WithFactsViaPDP()).RelationshipTuples
	waiting := plain.WaitForSync(1500*time.Millisecond, WithSyncPolicy(SyncPolicyFail))
	for _, s := range []*RelationshipTuplesService{plain, waiting, plain} {
		if err := createTuple(t.Context(), s); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for _, r := range pdp.Requests() {
		got = append(got, waitHeaders(r.Header))
	}
	if diff := cmp.Diff([]string{"- -", "1.5 fail", "- -"}, got); diff != "" {
		t.Errorf("the wait headers of the three calls differ (-want +got):\n%s", diff)
	}
	api.AssertUntouched()
}

// TestRelationshipTuplesWaitForSyncMisuse checks the calls through a copy of WaitForSync that
// cannot wait: List, All, ListDetailed, AllDetailed, Delete, BulkCreate and BulkDelete, which the
// PDP container answers without waiting, from a client that sends facts to the PDP container and
// from one that sends them to the API; and Create from a client that sends facts to the API, or
// with a negative timeout. Each returns an error that matches ErrInvalidConfig and names the
// operation and the rule, and sends no request, not even the lookup of the scope.
func TestRelationshipTuplesWaitForSyncMisuse(t *testing.T) {
	type misuse struct {
		name    string
		viaPDP  bool
		timeout time.Duration
		op      string
		call    func(ctx context.Context, s *RelationshipTuplesService) error
		rule    validate.Rule
	}
	calls := []misuse{
		{
			name: "List", op: opListRelationshipTuples,
			call: func(ctx context.Context, s *RelationshipTuplesService) error {
				_, err := s.List(ctx, nil)
				return err
			},
		},
		{
			name: "All", op: opListRelationshipTuples,
			call: func(ctx context.Context, s *RelationshipTuplesService) error {
				for _, err := range s.All(ctx, nil) {
					return err
				}
				return nil
			},
		},
		{
			name: "ListDetailed", op: opListRelationshipTuplesDetailed,
			call: func(ctx context.Context, s *RelationshipTuplesService) error {
				_, err := s.ListDetailed(ctx, nil)
				return err
			},
		},
		{
			name: "AllDetailed", op: opListRelationshipTuplesDetailed,
			call: func(ctx context.Context, s *RelationshipTuplesService) error {
				for _, err := range s.AllDetailed(ctx, nil) {
					return err
				}
				return nil
			},
		},
		{name: "Delete", op: opDeleteRelationshipTuple, call: deleteTuple},
	}
	for _, call := range bulkTupleCalls {
		calls = append(calls, misuse{name: call.name, op: call.op, call: call.call})
	}
	cases := []misuse{
		{
			name: "Create to the API", op: opCreateRelationshipTuple, call: createTuple,
			rule: validate.WaitForSyncFactsViaAPI,
		},
		{
			name: "Create with a negative timeout", viaPDP: true, timeout: -time.Nanosecond,
			op: opCreateRelationshipTuple, call: createTuple,
			rule: validate.WaitForSyncTimeoutNegative,
		},
	}
	for _, c := range calls {
		viaPDP, viaAPI := c, c
		viaPDP.name, viaPDP.viaPDP, viaPDP.rule = c.name+" via the PDP", true,
			validate.WaitForSyncNotWaiting
		viaAPI.name, viaAPI.rule = c.name+" to the API", validate.WaitForSyncFactsViaAPI
		cases = append(cases, viaPDP, viaAPI)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			var opts []Option
			if tc.viaPDP {
				opts = append(opts, WithFactsViaPDP())
			}
			s := decisionClient(t, api, pdp, opts...).RelationshipTuples.WaitForSync(tc.timeout)
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
