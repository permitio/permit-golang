package permit_test

import (
	"context"
	"encoding/json"
	"iter"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the resource instances: a key that may
// not act on the instances of the environment, an instance that does not exist, a key that
// another instance of the resource has already, and a field that the API rejects.
const (
	instanceForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the resource instances","id":"req-41"}`
	instanceNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the resource instance does not exist","id":"req-42"}`
	instanceConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"the resource has an instance with the key doc-3","id":"req-43"}`
	instanceInvalidBody = `{"detail":[{"loc":["body","attributes"],` +
		`"msg":"value is not a valid dict","type":"type_error.dict"}]}`
)

// The IDs of the reply fixtures of the resource instances: the instances doc-1, doc-2 and doc-3,
// the resource document and the tenants tenant-a and tenant-b.
const (
	instanceDoc1ID   = "00000000-0000-4000-8000-000000000401"
	instanceDoc2ID   = "00000000-0000-4000-8000-000000000402"
	instanceDoc3ID   = "00000000-0000-4000-8000-000000000403"
	instanceDocResID = "00000000-0000-4000-8000-000000000021"
	instanceTenantA  = "00000000-0000-4000-8000-000000000037"
	instanceTenantB  = "00000000-0000-4000-8000-000000000022"
)

// instanceAt returns an instance of the resource document of the reply fixtures with the key
// and the ID in the tenant whose key and ID are tenant and tenantID, and the scope of the
// fixtures, created and changed at created.
func instanceAt(key, id, tenant, tenantID, created string) models.ResourceInstanceRead {
	return models.ResourceInstanceRead{
		Key: key, Tenant: tenant, Resource: "document", ID: id,
		OrganizationID: exampleOrganizationID, ProjectID: exampleProjectID,
		EnvironmentID: exampleEnvironmentID, CreatedAt: at(created), UpdatedAt: at(created),
		ResourceID: instanceDocResID, TenantID: tenantID,
	}
}

// listedInstances are the instances of the reply fixtures list_resource_instances.200.json and
// list_resource_instances.200.page.json: one with an attribute, and one with the empty
// attributes {}, which stay an empty map, not nil.
func listedInstances() []models.ResourceInstanceRead {
	doc1 := instanceAt("doc-1", instanceDoc1ID, "tenant-a", instanceTenantA,
		"2026-10-14T09:30:00Z")
	doc1.Attributes = models.Attributes{"owner": "alice"}
	doc2 := instanceAt("doc-2", instanceDoc2ID, "tenant-b", instanceTenantB,
		"2026-10-15T08:00:00Z")
	doc2.Attributes = models.Attributes{}
	return []models.ResourceInstanceRead{doc1, doc2}
}

// detailedInstances are the instances of the reply fixture
// list_resource_instances_detailed.200.json, those of listedInstances with their relationship
// tuples: two of doc-1, and none of doc-2, an empty slice that is not nil.
func detailedInstances() []models.ResourceInstanceDetailedRead {
	var detailed []models.ResourceInstanceDetailedRead
	for _, instance := range listedInstances() {
		detailed = append(detailed, models.ResourceInstanceDetailedRead{
			Key: instance.Key, Tenant: instance.Tenant, Resource: instance.Resource,
			ID: instance.ID, OrganizationID: instance.OrganizationID,
			ProjectID: instance.ProjectID, EnvironmentID: instance.EnvironmentID,
			CreatedAt: instance.CreatedAt, UpdatedAt: instance.UpdatedAt,
			ResourceID: instance.ResourceID, TenantID: instance.TenantID,
			Attributes: instance.Attributes, Relationships: []models.RelationshipTupleBlockRead{},
		})
	}
	detailed[0].Relationships = []models.RelationshipTupleBlockRead{
		{Subject: "folder:f1", Relation: "parent", Object: "document:doc-1"},
		{Subject: "document:doc-1", Relation: "attachment", Object: "document:doc-2"},
	}
	return detailed
}

// fixtureInstance is the instance of the reply fixture get_resource_instance.200.json, with
// attributes of each kind of JSON value.
func fixtureInstance() *models.ResourceInstanceRead {
	instance := instanceAt("doc-1", instanceDoc1ID, "tenant-a", instanceTenantA,
		"2026-10-14T09:30:00Z")
	instance.UpdatedAt = at("2026-10-15T10:00:00.123456")
	instance.Attributes = models.Attributes{
		"owner": "alice", "pages": json.Number("9007199254740993"),
		"tags": []any{"q3", "draft"}, "review": map[string]any{"public": false},
	}
	return &instance
}

// createdInstance is the instance of the reply fixture create_resource_instance.200.json.
func createdInstance() *models.ResourceInstanceRead {
	instance := instanceAt("doc-3", instanceDoc3ID, "tenant-a", instanceTenantA,
		"2026-10-15T11:00:00Z")
	instance.Attributes = models.Attributes{"owner": "carol"}
	return &instance
}

// updatedInstance is the instance of the reply fixture update_resource_instance.200.json, whose
// attributes an update replaced.
func updatedInstance() *models.ResourceInstanceRead {
	instance := instanceAt("doc-1", instanceDoc1ID, "tenant-a", instanceTenantA,
		"2026-10-14T09:30:00Z")
	instance.UpdatedAt = at("2026-10-15T13:00:00Z")
	instance.Attributes = models.Attributes{"owner": "bob"}
	return &instance
}

// firstInstances returns the first n instances that seq yields, and ends the loop there, so that
// it asks for no page past them; or the first error that seq yields.
func firstInstances[T any](seq iter.Seq2[T, error], n int) ([]T, error) {
	var got []T
	for instance, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, instance)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// resourceInstanceCases returns the wire cases of the resource instances of an environment, each
// of which the runner runs against the API, with its scope, and against the PDP container under
// /facts/resource_instances, with WithFactsViaPDP. The instances are named as
// <resource>:<key>, whose : the path sends as it is, with characters that the path escapes, a
// slash, a percent sign and a letter outside ASCII, as document:a/b and document:50%é hold, or
// by an ID, of the shape of a UUID, and one has a leading dot, .hidden, which the path sends as
// it is. No list sends detailed: the detailed list has a route of its own.
func resourceInstanceCases() []wireCase {
	const (
		instances = "/v2/facts/{proj_id}/{env_id}/resource_instances"
		detailed  = instances + "/detailed"
		instance  = instances + "/{instance_id}"
	)
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, instanceForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-41"
		e.Detail = "the API key may not act on the resource instances"
		return e
	}
	invalid := projectError("update_resource_instance", http.MethodPatch, instance,
		http.StatusUnprocessableEntity, instanceInvalidBody)
	invalid.Detail = "body.attributes: value is not a valid dict"
	perPage := func(op, route string) *permit.Error {
		e := projectError(op, http.MethodGet, route, http.StatusUnprocessableEntity, perPageBody)
		e.Detail = "query.per_page: ensure this value is less than or equal to 100"
		return e
	}
	getErr := projectError("get_resource_instance", http.MethodGet, instance,
		http.StatusNotFound, instanceNotFoundBody)
	getErr.Code, getErr.Title, getErr.RequestID = "NOT_FOUND", "Not Found", "req-42"
	getErr.Detail = "the resource instance does not exist"
	createErr := projectError("create_resource_instance", http.MethodPost, instances,
		http.StatusConflict, instanceConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "DUPLICATE_ENTITY", "Conflict", "req-43"
	createErr.Detail = "the resource has an instance with the key doc-3"
	filters := &permit.ResourceInstancesListOptions{
		ListOptions: permit.ListOptions{Page: 2, PerPage: 1},
		Tenant:      "tenant-a", Resource: "document", Search: []string{"a&b c", "", "doc"},
	}
	filtersQuery := "page=2&per_page=1&resource=document&search=a%26b+c&search=doc&tenant=tenant-a"
	both := []wireMode{apiMode, factsViaPDPMode}
	return []wireCase{
		{
			Method: "ResourceInstancesService.List",
			Op:     "list_resource_instances",
			Route:  "GET " + instances,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceInstances.List(ctx, filters)
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/resource_instances", RawQuery: filtersQuery,
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_resource_instances.200.page.json")},
			WantResult: &permit.Page[models.ResourceInstanceRead]{
				Data: listedInstances(), Page: 2, PerPage: 1, TotalCount: new(2),
				PageCount: new(1),
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    perPage("list_resource_instances", instances),
		},
		{
			Method: "ResourceInstancesService.List",
			Op:     "list_resource_instances",
			Route:  "GET " + instances,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceInstances.List(ctx, nil)
			},
			Request:    wireRequest{RawPath: "/v2/facts/p/e/resource_instances"},
			Reply:      mockpermit.Reply{Body: replyFixture("list_resource_instances.200.json")},
			WantResult: &permit.Page[models.ResourceInstanceRead]{Data: listedInstances(), Page: 1},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: instanceForbiddenBody},
			WantErr:    forbidden("list_resource_instances", http.MethodGet, instances),
		},
		{
			Method: "ResourceInstancesService.All",
			Op:     "list_resource_instances",
			Route:  "GET " + instances,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstInstances(c.ResourceInstances.All(ctx, nil), 2)
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/resource_instances", RawQuery: "page=1&per_page=100",
			},
			Reply:      mockpermit.Reply{Body: replyFixture("list_resource_instances.200.json")},
			WantResult: listedInstances(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: instanceForbiddenBody},
			WantErr:    forbidden("list_resource_instances", http.MethodGet, instances),
		},
		{
			Method: "ResourceInstancesService.ListDetailed",
			Op:     "list_resource_instances_detailed",
			Route:  "GET " + detailed,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceInstances.ListDetailed(ctx, filters)
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/resource_instances/detailed", RawQuery: filtersQuery,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("list_resource_instances_detailed.200.json"),
			},
			WantResult: &permit.Page[models.ResourceInstanceDetailedRead]{
				Data: detailedInstances(), Page: 2, PerPage: 1, TotalCount: new(2),
				PageCount: new(1),
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    perPage("list_resource_instances_detailed", detailed),
		},
		{
			Method: "ResourceInstancesService.AllDetailed",
			Op:     "list_resource_instances_detailed",
			Route:  "GET " + detailed,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstInstances(c.ResourceInstances.AllDetailed(ctx, nil), 2)
			},
			Request: wireRequest{
				RawPath:  "/v2/facts/p/e/resource_instances/detailed",
				RawQuery: "page=1&per_page=100",
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("list_resource_instances_detailed.200.json"),
			},
			WantResult: detailedInstances(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: instanceForbiddenBody},
			WantErr:    forbidden("list_resource_instances_detailed", http.MethodGet, detailed),
		},
		{
			Method: "ResourceInstancesService.Get",
			Op:     "get_resource_instance",
			Route:  "GET " + instance,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceInstances.Get(ctx, "document:a/b")
			},
			Request:    wireRequest{RawPath: "/v2/facts/p/e/resource_instances/document:a%2Fb"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_resource_instance.200.json")},
			WantResult: fixtureInstance(),
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: instanceNotFoundBody},
			WantErr:    getErr,
		},
		{
			Method: "ResourceInstancesService.Create",
			Op:     "create_resource_instance",
			Route:  "POST " + instances,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceInstances.Create(ctx, models.ResourceInstanceCreate{
					Key: "doc-3", Tenant: "tenant-a", Resource: "document",
					Attributes: models.Attributes{"owner": "carol"},
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/resource_instances",
				Body: `{"key":"doc-3","tenant":"tenant-a","resource":"document",` +
					`"attributes":{"owner":"carol"}}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("create_resource_instance.200.json")},
			WantResult: createdInstance(),
			ErrorReply: mockpermit.Reply{Status: http.StatusConflict, Body: instanceConflictBody},
			WantErr:    createErr,
		},
		{
			Method: "ResourceInstancesService.Update",
			Op:     "update_resource_instance",
			Route:  "PATCH " + instance,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceInstances.Update(ctx, "document:50%é",
					models.ResourceInstanceUpdate{
						Attributes: models.Some(models.Attributes{"owner": "bob"}),
					})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/resource_instances/document:50%25%C3%A9",
				Body:    `{"attributes":{"owner":"bob"}}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("update_resource_instance.200.json")},
			WantResult: updatedInstance(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: instanceInvalidBody,
			},
			WantErr: invalid,
		},
		{
			Method: "ResourceInstancesService.Update",
			Op:     "update_resource_instance",
			Route:  "PATCH " + instance,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceInstances.Update(ctx, fixtures.UUIDKey,
					models.ResourceInstanceUpdate{Attributes: models.Null[models.Attributes]()})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/resource_instances/" + fixtures.UUIDKey,
				Body:    `{"attributes":null}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("update_resource_instance.200.json")},
			WantResult: updatedInstance(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: instanceForbiddenBody},
			WantErr:    forbidden("update_resource_instance", http.MethodPatch, instance),
		},
		{
			Method: "ResourceInstancesService.Delete",
			Op:     "delete_resource_instance",
			Route:  "DELETE " + instance,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.ResourceInstances.Delete(ctx, ".hidden")
			},
			Request:    wireRequest{RawPath: "/v2/facts/p/e/resource_instances/.hidden"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: instanceForbiddenBody},
			WantErr:    forbidden("delete_resource_instance", http.MethodDelete, instance),
		},
	}
}
