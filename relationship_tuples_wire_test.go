package permit_test

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the relationship tuples: a key that may
// not act on the relationship tuples of the environment, a relationship tuple that does not
// exist, a field that the API rejects, a page that is too large, and a tuple whose subject and
// object are in different tenants.
const (
	tupleForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the relationship tuples of the environment",` +
		`"id":"req-21"}`
	tupleNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the environment has no such relationship tuple","id":"req-22"}`
	tupleInvalidBody = `{"detail":[{"loc":["body","subject"],` +
		`"msg":"field required","type":"value_error.missing"}]}`
	tuplePerPageBody = `{"detail":[{"loc":["query","per_page"],` +
		`"msg":"ensure this value is less than or equal to 100",` +
		`"type":"value_error.number.not_le","ctx":{"limit_value":100}}]}`
	tupleTenantsBody = `{"error_code":"BAD_REQUEST","title":"Bad Request",` +
		`"message":"the subject and the object are in different tenants","id":"req-23"}`
)

// The values of the reply fixtures of the relationship tuples: tupleFolderID is the ID of the
// subject folder:f1 and tupleParentID that of the relation parent; tupleToDocOneID and
// tupleToDocTwoID are the tuples' own IDs, of folder:f1 parent document:doc-1 and of folder:f1
// parent document:doc-2; and tupleTenantAKey is the key of their tenant, tenant-a. The ID of
// tenant-a and that of document:doc-1 are those of the fixtures of the role assignments,
// assignmentTenantID and assignmentInstanceID.
const (
	tupleFolderID   = "00000000-0000-4000-8000-000000000502"
	tupleParentID   = "00000000-0000-4000-8000-000000000503"
	tupleToDocOneID = "00000000-0000-4000-8000-000000000501"
	tupleToDocTwoID = "00000000-0000-4000-8000-000000000506"
	tupleTenantAKey = "tenant-a"
)

// listedTuples are the relationship tuples of the reply fixture
// list_relationship_tuples.200.json: folder:f1 is the parent of document:doc-1, and of
// document:doc-2, whose reply has no object_id.
func listedTuples() []models.RelationshipTupleRead {
	tuple := func(id, object, created, updated string) models.RelationshipTupleRead {
		return models.RelationshipTupleRead{
			Subject: "folder:f1", Relation: "parent", Object: object, ID: id,
			Tenant: tupleTenantAKey, SubjectID: tupleFolderID, RelationID: tupleParentID,
			TenantID: assignmentTenantID, OrganizationID: exampleOrganizationID,
			ProjectID: exampleProjectID, EnvironmentID: exampleEnvironmentID,
			CreatedAt: at(created), UpdatedAt: at(updated),
		}
	}
	first := tuple(tupleToDocOneID, "document:doc-1", "2026-10-16T09:00:00Z",
		"2026-10-16T09:00:00Z")
	first.ObjectID = assignmentInstanceID
	second := tuple(tupleToDocTwoID, "document:doc-2", "2026-10-16T09:30:00.250000",
		"2026-10-16T10:00:00.500000")
	return []models.RelationshipTupleRead{first, second}
}

// detailedTuples are the relationship tuples of the reply fixture
// list_relationship_tuples_detailed.200.json, which are those of listedTuples with the details of
// their subject, their relation, their object and their tenant.
func detailedTuples() []models.RelationshipTupleDetailedRead {
	listed := listedTuples()
	first, second := listed[0], listed[1]
	first.SubjectDetails = &models.ResourceInstanceBlockRead{
		Key: "f1", Tenant: tupleTenantAKey, Resource: "folder",
		Attributes: models.Attributes{"shared": true},
	}
	first.RelationDetails = &models.StrippedRelationBlockRead{
		Key: "parent", Name: "Parent", Description: "the folder that holds the document",
	}
	first.ObjectDetails = &models.ResourceInstanceBlockRead{
		Key: "doc-1", Tenant: tupleTenantAKey, Resource: "document",
		Attributes: models.Attributes{"pages": json.Number("12")},
	}
	first.TenantDetails = &models.TenantBlockRead{
		Key: tupleTenantAKey, Name: "Tenant A", Description: "the first tenant",
		Attributes: models.Attributes{"tier": "gold"},
	}
	second.SubjectDetails = &models.ResourceInstanceBlockRead{
		Key: "f1", Tenant: tupleTenantAKey, Resource: "folder", Attributes: models.Attributes{},
	}
	second.RelationDetails = &models.StrippedRelationBlockRead{Key: "parent", Name: "Parent"}
	second.ObjectDetails = &models.ResourceInstanceBlockRead{
		Key: "doc-2", Tenant: tupleTenantAKey, Resource: "document",
	}
	second.TenantDetails = &models.TenantBlockRead{Key: tupleTenantAKey, Name: "Tenant A"}
	return []models.RelationshipTupleDetailedRead{
		models.RelationshipTupleDetailedRead(first), models.RelationshipTupleDetailedRead(second),
	}
}

// relationshipTupleCases returns the wire cases of the relationship tuples of an environment,
// each of which the runner runs against the API, with its scope, and against the PDP container
// under /facts/relationship_tuples, with WithFactsViaPDP. The routes take no key in their path:
// the keys go in the query of a list, which escapes the characters of folder:a/b and
// document:50%é and sends a UUID-shaped tenant and .hidden as they are, and in the body of a
// create or a delete, one or in the JSON object of a bulk call, as they are. The lists never send
// the deprecated query parameters detailed and include_total_count.
func relationshipTupleCases() []wireCase {
	const (
		tuples   = "/v2/facts/{proj_id}/{env_id}/relationship_tuples"
		detailed = tuples + "/detailed"
		bulk     = tuples + "/bulk"
	)
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, tupleForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-21"
		e.Detail = "the API key may not act on the relationship tuples of the environment"
		return e
	}
	tooLarge := func(op, route string) *permit.Error {
		e := projectError(op, http.MethodGet, route, http.StatusUnprocessableEntity,
			tuplePerPageBody)
		e.Detail = "query.per_page: ensure this value is less than or equal to 100"
		return e
	}
	invalid := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusUnprocessableEntity, tupleInvalidBody)
		e.Detail = "body.subject: field required"
		return e
	}
	tenants := func(op, route string) *permit.Error {
		e := projectError(op, http.MethodPost, route, http.StatusBadRequest, tupleTenantsBody)
		e.Code, e.Title, e.RequestID = "BAD_REQUEST", "Bad Request", "req-23"
		e.Detail = "the subject and the object are in different tenants"
		return e
	}
	notFound := func(op, route string) *permit.Error {
		e := projectError(op, http.MethodDelete, route, http.StatusNotFound, tupleNotFoundBody)
		e.Code, e.Title, e.RequestID = "NOT_FOUND", "Not Found", "req-22"
		e.Detail = "the environment has no such relationship tuple"
		return e
	}
	filters := &permit.RelationshipTuplesListOptions{
		ListOptions: permit.ListOptions{Page: 2, PerPage: 100},
		Tenant:      assignmentTenantID, Subject: "folder:a/b", Relation: "parent",
		Object: "document:50%é", SubjectType: "folder", ObjectType: ".hidden",
	}
	filtersQuery := "object=document%3A50%25%C3%A9&object_type=.hidden&page=2&per_page=100&" +
		"relation=parent&subject=folder%3Aa%2Fb&subject_type=folder&tenant=" + assignmentTenantID
	created := listedTuples()[0]
	both := []wireMode{apiMode, factsViaPDPMode}
	return []wireCase{
		{
			Method: "RelationshipTuplesService.List",
			Op:     "list_relationship_tuples",
			Route:  "GET " + tuples,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.RelationshipTuples.List(ctx, filters)
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/relationship_tuples", RawQuery: filtersQuery,
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_relationship_tuples.200.json")},
			WantResult: &permit.Page[models.RelationshipTupleRead]{
				Data: listedTuples(), Page: 2, PerPage: 100,
			},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: tuplePerPageBody,
			},
			WantErr: tooLarge("list_relationship_tuples", tuples),
		},
		{
			Method: "RelationshipTuplesService.List",
			Op:     "list_relationship_tuples",
			Route:  "GET " + tuples,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.RelationshipTuples.List(ctx, nil)
			},
			Request: wireRequest{RawPath: "/v2/facts/p/e/relationship_tuples"},
			Reply: mockpermit.Reply{
				Body: replyFixture("list_relationship_tuples.200.page.json"),
			},
			WantResult: &permit.Page[models.RelationshipTupleRead]{
				Data: listedTuples()[:1], Page: 1, TotalCount: new(3), PageCount: new(1),
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: tupleForbiddenBody},
			WantErr:    forbidden("list_relationship_tuples", http.MethodGet, tuples),
		},
		{
			Method: "RelationshipTuplesService.All",
			Op:     "list_relationship_tuples",
			Route:  "GET " + tuples,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstOf(c.RelationshipTuples.All(ctx, nil), 2)
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/relationship_tuples", RawQuery: "page=1&per_page=100",
			},
			Reply:      mockpermit.Reply{Body: replyFixture("list_relationship_tuples.200.json")},
			WantResult: listedTuples(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: tupleForbiddenBody},
			WantErr:    forbidden("list_relationship_tuples", http.MethodGet, tuples),
		},
		{
			Method: "RelationshipTuplesService.ListDetailed",
			Op:     "list_relationship_tuples_detailed",
			Route:  "GET " + detailed,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.RelationshipTuples.ListDetailed(ctx, filters)
			},
			Request: wireRequest{
				RawPath:  "/v2/facts/p/e/relationship_tuples/detailed",
				RawQuery: filtersQuery,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("list_relationship_tuples_detailed.200.json"),
			},
			WantResult: &permit.Page[models.RelationshipTupleDetailedRead]{
				Data: detailedTuples(), Page: 2, PerPage: 100, TotalCount: new(2),
				PageCount: new(2),
			},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: tuplePerPageBody,
			},
			WantErr: tooLarge("list_relationship_tuples_detailed", detailed),
		},
		{
			Method: "RelationshipTuplesService.AllDetailed",
			Op:     "list_relationship_tuples_detailed",
			Route:  "GET " + detailed,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstOf(c.RelationshipTuples.AllDetailed(ctx, nil), 2)
			},
			Request: wireRequest{
				RawPath:  "/v2/facts/p/e/relationship_tuples/detailed",
				RawQuery: "page=1&per_page=100",
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("list_relationship_tuples_detailed.200.json"),
			},
			WantResult: detailedTuples(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: tupleForbiddenBody},
			WantErr:    forbidden("list_relationship_tuples_detailed", http.MethodGet, detailed),
		},
		{
			Method: "RelationshipTuplesService.Create",
			Op:     "create_relationship_tuple",
			Route:  "POST " + tuples,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.RelationshipTuples.Create(ctx, models.RelationshipTupleCreate{
					Subject: "folder:a/b", Relation: "parent", Object: "document:50%é",
					Tenant: syncedKey,
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/relationship_tuples",
				Body: `{"subject":"folder:a/b","relation":"parent","object":"document:50%é",` +
					`"tenant":"idp|dave@example.com"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("create_relationship_tuple.200.json")},
			WantResult: &created,
			ErrorReply: mockpermit.Reply{Status: http.StatusBadRequest, Body: tupleTenantsBody},
			WantErr:    tenants("create_relationship_tuple", tuples),
		},
		{
			Method: "RelationshipTuplesService.Create",
			Op:     "create_relationship_tuple",
			Route:  "POST " + tuples,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.RelationshipTuples.Create(ctx, models.RelationshipTupleCreate{
					Subject: tupleFolderID, Relation: ".hidden", Object: "document:doc-1",
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/relationship_tuples",
				Body: `{"subject":"` + tupleFolderID + `","relation":".hidden",` +
					`"object":"document:doc-1"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("create_relationship_tuple.200.json")},
			WantResult: &created,
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: tupleInvalidBody,
			},
			WantErr: invalid("create_relationship_tuple", http.MethodPost, tuples),
		},
		{
			Method: "RelationshipTuplesService.Delete",
			Op:     "delete_relationship_tuple",
			Route:  "DELETE " + tuples,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.RelationshipTuples.Delete(ctx,
					models.RelationshipTupleDelete{
						Subject: "folder:50%é", Relation: "parent", Object: "document:a/b",
					})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/relationship_tuples",
				Body:    `{"subject":"folder:50%é","relation":"parent","object":"document:a/b"}`,
			},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: tupleNotFoundBody},
			WantErr:    notFound("delete_relationship_tuple", tuples),
		},
		{
			Method: "RelationshipTuplesService.BulkCreate",
			Op:     "bulk_create_relationship_tuples",
			Route:  "POST " + bulk,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.RelationshipTuples.BulkCreate(ctx,
					[]models.RelationshipTupleCreate{
						{Subject: "folder:a/b", Relation: "parent", Object: "document:.hidden"},
						{
							Subject: "folder:f1", Relation: "parent", Object: "document:50%é",
							Tenant: syncedKey,
						},
					})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/relationship_tuples/bulk",
				Body: `{"operations":[{"subject":"folder:a/b","relation":"parent",` +
					`"object":"document:.hidden"},{"subject":"folder:f1","relation":"parent",` +
					`"object":"document:50%é","tenant":"idp|dave@example.com"}]}`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("bulk_create_relationship_tuples.200.json"),
			},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusBadRequest, Body: tupleTenantsBody},
			WantErr:    tenants("bulk_create_relationship_tuples", bulk),
		},
		{
			Method: "RelationshipTuplesService.BulkDelete",
			Op:     "bulk_delete_relationship_tuples",
			Route:  "DELETE " + bulk,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.RelationshipTuples.BulkDelete(ctx,
					[]models.RelationshipTupleDelete{
						{Subject: "folder:50%é", Relation: ".hidden", Object: "document:a/b"},
						{Subject: tupleFolderID, Relation: "parent", Object: "document:doc-1"},
					})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/relationship_tuples/bulk",
				Body: `{"idents":[{"subject":"folder:50%é","relation":".hidden",` +
					`"object":"document:a/b"},{"subject":"` + tupleFolderID + `",` +
					`"relation":"parent","object":"document:doc-1"}]}`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("bulk_delete_relationship_tuples.200.json"),
			},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: tupleNotFoundBody},
			WantErr:    notFound("bulk_delete_relationship_tuples", bulk),
		},
	}
}
