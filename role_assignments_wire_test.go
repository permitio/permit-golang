package permit_test

import (
	"context"
	"encoding/json"
	"iter"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the role assignments: a key that may
// not act on the role assignments of the environment, a role assignment that does not exist, a
// role that the user has already, a field that the API rejects, a page that is too large, and a
// user that the environment does not have.
const (
	roleAssignmentForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the role assignments of the environment",` +
		`"id":"req-16"}`
	roleAssignmentNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the user does not have the role in the tenant","id":"req-17"}`
	roleAssignmentConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"the user has the role in the tenant already","id":"req-18"}`
	roleAssignmentInvalidBody = `{"detail":[{"loc":["body","user"],` +
		`"msg":"field required","type":"value_error.missing"}]}`
	roleAssignmentPerPageBody = `{"detail":[{"loc":["query","per_page"],` +
		`"msg":"ensure this value is less than or equal to 1000",` +
		`"type":"value_error.number.not_le","ctx":{"limit_value":1000}}]}`
	roleAssignmentUserMissingBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the environment has no user idp|dave@example.com","id":"req-19"}`
)

// The IDs of the reply fixtures of the role assignments: of the roles editor and owner, of the
// tenant tenant-a and of the resource instance document:doc-1, which the fixtures of the roles of
// a user hold too, and of the role assignments.
const (
	roleEditorID         = "00000000-0000-4000-8000-000000000036"
	roleOwnerID          = "00000000-0000-4000-8000-000000000040"
	assignmentTenantID   = "00000000-0000-4000-8000-000000000037"
	assignmentInstanceID = "00000000-0000-4000-8000-000000000039"
	assignmentAliceID    = "00000000-0000-4000-8000-000000000041"
	assignmentBobID      = "00000000-0000-4000-8000-000000000042"
	assignmentDaveID     = "00000000-0000-4000-8000-000000000043"
	assignmentOwnerID    = "00000000-0000-4000-8000-000000000044"
)

// inTenantA returns a, a role assignment of the reply fixtures of the role assignments, with the
// fields that they share: the tenant tenant-a and the scope of the fixtures.
func inTenantA(a models.RoleAssignmentRead) models.RoleAssignmentRead {
	a.Tenant, a.TenantID = "tenant-a", assignmentTenantID
	a.OrganizationID, a.ProjectID = exampleOrganizationID, exampleProjectID
	a.EnvironmentID = exampleEnvironmentID
	return a
}

// listedAssignments are the role assignments of the reply fixture list_role_assignments.200.json:
// the role editor of alice in the tenant tenant-a, and the role owner of bob on the resource
// instance document:doc-1, which the list holds as well.
func listedAssignments() []models.RoleAssignmentRead {
	return []models.RoleAssignmentRead{
		inTenantA(models.RoleAssignmentRead{
			ID: assignmentAliceID, User: "alice", UserID: userAliceID, Role: "editor",
			RoleID: roleEditorID, CreatedAt: at("2026-10-15T14:00:00Z"),
		}),
		inTenantA(models.RoleAssignmentRead{
			ID: assignmentBobID, User: "bob", UserID: userBobID, Role: "owner",
			RoleID: roleOwnerID, ResourceInstance: "document:doc-1",
			ResourceInstanceID: assignmentInstanceID, CreatedAt: at("2026-10-15T14:30:00.250000"),
		}),
	}
}

// detailedAssignments are the role assignments of the reply fixture
// list_role_assignments_detailed.200.json, which are those of listedAssignments with their user,
// their role, their tenant and their resource instance as objects.
func detailedAssignments() []models.RoleAssignmentDetailedRead {
	tenant := models.RoleAssignmentTenant{
		ID: assignmentTenantID, Key: "tenant-a", Name: "Tenant A",
		Attributes: models.Attributes{"tier": "gold"},
	}
	scoped := func(id, created string) models.RoleAssignmentDetailedRead {
		return models.RoleAssignmentDetailedRead{
			ID: id, Tenant: tenant, OrganizationID: exampleOrganizationID,
			ProjectID: exampleProjectID, EnvironmentID: exampleEnvironmentID,
			CreatedAt: at(created),
		}
	}
	alice := scoped(assignmentAliceID, "2026-10-15T14:00:00Z")
	alice.User = models.RoleAssignmentUser{
		ID: userAliceID, Key: "alice", Email: "alice@example.com", FirstName: "Alice",
		LastName: "Liddell", Attributes: models.Attributes{"department": "finance"},
	}
	alice.Role = models.RoleAssignmentRole{
		ID: roleEditorID, Key: "editor", Name: "Editor",
		Permissions: []string{"document:read", "document:write"},
	}
	bob := scoped(assignmentBobID, "2026-10-15T14:30:00.250000")
	bob.User = models.RoleAssignmentUser{ID: userBobID, Key: "bob", Attributes: models.Attributes{}}
	bob.Role = models.RoleAssignmentRole{ID: roleOwnerID, Key: "owner", Name: "Owner"}
	bob.ResourceInstance = &models.RoleAssignmentResourceInstance{
		ID: assignmentInstanceID, Key: "doc-1", Resource: "document",
		Attributes: models.Attributes{"pages": json.Number("12")},
	}
	return []models.RoleAssignmentDetailedRead{alice, bob}
}

// firstOf returns the first n items that seq yields, and ends the loop there, so that it asks for
// no page past them; or the first error that seq yields.
func firstOf[T any](seq iter.Seq2[T, error], n int) ([]T, error) {
	var got []T
	for item, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, item)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// roleAssignmentCases returns the wire cases of the role assignments of an environment, each of
// which the runner runs against the API, with its scope, and against the PDP container under
// /facts/role_assignments, with WithFactsViaPDP. The routes take no key in their path: the keys
// go in the query of a list, which repeats a parameter for each value of a filter and escapes
// the characters of a/b, 50%é and idp|dave@example.com, and in the body of an assignment or an
// unassignment, one or a JSON array of them in bulk, as they are, .hidden too. The lists never
// send the deprecated query parameters detailed and include_total_count, an unassignment never
// sends return_deleted, and a bulk assignment sends missing_user_policy only when it is set.
func roleAssignmentCases() []wireCase {
	const (
		assignments = "/v2/facts/{proj_id}/{env_id}/role_assignments"
		detailed    = assignments + "/detailed"
		bulk        = assignments + "/bulk"
	)
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, roleAssignmentForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-16"
		e.Detail = "the API key may not act on the role assignments of the environment"
		return e
	}
	tooLarge := func(op, route string) *permit.Error {
		e := projectError(op, http.MethodGet, route, http.StatusUnprocessableEntity,
			roleAssignmentPerPageBody)
		e.Detail = "query.per_page: ensure this value is less than or equal to 1000"
		return e
	}
	invalid := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusUnprocessableEntity,
			roleAssignmentInvalidBody)
		e.Detail = "body.user: field required"
		return e
	}
	conflict := func(op, route string) *permit.Error {
		e := projectError(op, http.MethodPost, route, http.StatusConflict,
			roleAssignmentConflictBody)
		e.Code, e.Title, e.RequestID = "DUPLICATE_ENTITY", "Conflict", "req-18"
		e.Detail = "the user has the role in the tenant already"
		return e
	}
	userMissing := projectError("bulk_assign_role", http.MethodPost, bulk, http.StatusNotFound,
		roleAssignmentUserMissingBody)
	userMissing.Code, userMissing.Title, userMissing.RequestID = "NOT_FOUND", "Not Found", "req-19"
	userMissing.Detail = "the environment has no user idp|dave@example.com"
	notFound := projectError("unassign_role", http.MethodDelete, assignments,
		http.StatusNotFound, roleAssignmentNotFoundBody)
	notFound.Code, notFound.Title, notFound.RequestID = "NOT_FOUND", "Not Found", "req-17"
	notFound.Detail = "the user does not have the role in the tenant"
	filters := &permit.RoleAssignmentsListOptions{
		ListOptions: permit.ListOptions{Page: 2, PerPage: 1000},
		Users:       []string{"a/b", "50%é", "idp|dave@example.com"},
		Roles:       []string{"editor", "owner"}, Tenants: []string{"tenant-a"},
		ResourceInstance: "document:doc-1",
	}
	filtersQuery := "page=2&per_page=1000&resource_instance=document%3Adoc-1&role=editor&" +
		"role=owner&tenant=tenant-a&user=a%2Fb&user=50%25%C3%A9&user=idp%7Cdave%40example.com"
	both := []wireMode{apiMode, factsViaPDPMode}
	return []wireCase{
		{
			Method: "RoleAssignmentsService.List",
			Op:     "list_role_assignments",
			Route:  "GET " + assignments,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.RoleAssignments.List(ctx, filters)
			},
			Request: wireRequest{RawPath: "/v2/facts/p/e/role_assignments", RawQuery: filtersQuery},
			Reply:   mockpermit.Reply{Body: replyFixture("list_role_assignments.200.json")},
			WantResult: &permit.Page[models.RoleAssignmentRead]{
				Data: listedAssignments(), Page: 2, PerPage: 1000,
			},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: roleAssignmentPerPageBody,
			},
			WantErr: tooLarge("list_role_assignments", assignments),
		},
		{
			Method: "RoleAssignmentsService.List",
			Op:     "list_role_assignments",
			Route:  "GET " + assignments,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.RoleAssignments.List(ctx, nil)
			},
			Request: wireRequest{RawPath: "/v2/facts/p/e/role_assignments"},
			Reply:   mockpermit.Reply{Body: replyFixture("list_role_assignments.200.page.json")},
			WantResult: &permit.Page[models.RoleAssignmentRead]{
				Data: listedAssignments()[:1], Page: 1, TotalCount: new(3), PageCount: new(3),
			},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden, Body: roleAssignmentForbiddenBody,
			},
			WantErr: forbidden("list_role_assignments", http.MethodGet, assignments),
		},
		{
			Method: "RoleAssignmentsService.All",
			Op:     "list_role_assignments",
			Route:  "GET " + assignments,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstOf(c.RoleAssignments.All(ctx, nil), 2)
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/role_assignments", RawQuery: "page=1&per_page=1000",
			},
			Reply:      mockpermit.Reply{Body: replyFixture("list_role_assignments.200.json")},
			WantResult: listedAssignments(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden, Body: roleAssignmentForbiddenBody,
			},
			WantErr: forbidden("list_role_assignments", http.MethodGet, assignments),
		},
		{
			Method: "RoleAssignmentsService.ListDetailed",
			Op:     "list_role_assignments_detailed",
			Route:  "GET " + detailed,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.RoleAssignments.ListDetailed(ctx, filters)
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/role_assignments/detailed", RawQuery: filtersQuery,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("list_role_assignments_detailed.200.json"),
			},
			WantResult: &permit.Page[models.RoleAssignmentDetailedRead]{
				Data: detailedAssignments(), Page: 2, PerPage: 1000, TotalCount: new(2),
				PageCount: new(1),
			},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: roleAssignmentPerPageBody,
			},
			WantErr: tooLarge("list_role_assignments_detailed", detailed),
		},
		{
			Method: "RoleAssignmentsService.AllDetailed",
			Op:     "list_role_assignments_detailed",
			Route:  "GET " + detailed,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstOf(c.RoleAssignments.AllDetailed(ctx, nil), 2)
			},
			Request: wireRequest{
				RawPath:  "/v2/facts/p/e/role_assignments/detailed",
				RawQuery: "page=1&per_page=1000",
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("list_role_assignments_detailed.200.json"),
			},
			WantResult: detailedAssignments(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden, Body: roleAssignmentForbiddenBody,
			},
			WantErr: forbidden("list_role_assignments_detailed", http.MethodGet, detailed),
		},
		{
			Method: "RoleAssignmentsService.Assign",
			Op:     "assign_role",
			Route:  "POST " + assignments,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.RoleAssignments.Assign(ctx, models.RoleAssignmentCreate{
					User: syncedKey, Role: "editor", Tenant: "tenant-a",
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/role_assignments",
				Body:    `{"user":"idp|dave@example.com","role":"editor","tenant":"tenant-a"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("assign_role.200.json")},
			WantResult: tenantAssignment(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusConflict, Body: roleAssignmentConflictBody,
			},
			WantErr: conflict("assign_role", assignments),
		},
		{
			Method: "RoleAssignmentsService.Assign",
			Op:     "assign_role",
			Route:  "POST " + assignments,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.RoleAssignments.Assign(ctx, models.RoleAssignmentCreate{
					User: "alice", Role: "owner", ResourceInstance: "document:doc-1",
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/role_assignments",
				Body:    `{"user":"alice","role":"owner","resource_instance":"document:doc-1"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("assign_role.200.instance.json")},
			WantResult: instanceAssignment(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: roleAssignmentInvalidBody,
			},
			WantErr: invalid("assign_role", http.MethodPost, assignments),
		},
		{
			Method: "RoleAssignmentsService.Unassign",
			Op:     "unassign_role",
			Route:  "DELETE " + assignments,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.RoleAssignments.Unassign(ctx,
					models.RoleAssignmentRemove{User: "50%é", Role: "editor", Tenant: "a/b"})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/role_assignments",
				Body:    `{"user":"50%é","role":"editor","tenant":"a/b"}`,
			},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusNotFound, Body: roleAssignmentNotFoundBody,
			},
			WantErr: notFound,
		},
		{
			Method: "RoleAssignmentsService.Unassign",
			Op:     "unassign_role",
			Route:  "DELETE " + assignments,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.RoleAssignments.Unassign(ctx, models.RoleAssignmentRemove{
					User: "alice", Role: "owner", Tenant: "tenant-a",
					ResourceInstance: "document:doc-1",
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/role_assignments",
				Body: `{"user":"alice","role":"owner","tenant":"tenant-a",` +
					`"resource_instance":"document:doc-1"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("unassign_role.200.json")},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: roleAssignmentInvalidBody,
			},
			WantErr: invalid("unassign_role", http.MethodDelete, assignments),
		},
		{
			Method: "RoleAssignmentsService.BulkAssign",
			Op:     "bulk_assign_role",
			Route:  "POST " + bulk,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.RoleAssignments.BulkAssign(ctx, []models.RoleAssignmentCreate{
					{User: "a/b", Role: "editor", Tenant: "50%é"},
					{User: syncedKey, Role: "owner", ResourceInstance: "document:.hidden"},
				}, &permit.RoleAssignmentsBulkAssignOptions{
					MissingUserPolicy: models.MissingUserPolicyCreate,
				})
			},
			Request: wireRequest{
				RawPath:  "/v2/facts/p/e/role_assignments/bulk",
				RawQuery: "missing_user_policy=create",
				Body: `[{"user":"a/b","role":"editor","tenant":"50%é"},` +
					`{"user":"idp|dave@example.com","role":"owner",` +
					`"resource_instance":"document:.hidden"}]`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("bulk_assign_role.200.json")},
			WantResult: &models.BulkRoleAssignmentReport{AssignmentsCreated: 2},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusConflict, Body: roleAssignmentConflictBody,
			},
			WantErr: conflict("bulk_assign_role", bulk),
		},
		{
			Method: "RoleAssignmentsService.BulkAssign",
			Op:     "bulk_assign_role",
			Route:  "POST " + bulk,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.RoleAssignments.BulkAssign(ctx, []models.RoleAssignmentCreate{
					{User: syncedKey, Role: "editor", Tenant: "tenant-a"},
				}, nil)
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/role_assignments/bulk",
				Body:    `[{"user":"idp|dave@example.com","role":"editor","tenant":"tenant-a"}]`,
			},
			Reply:      mockpermit.Reply{Body: `{}`},
			WantResult: &models.BulkRoleAssignmentReport{},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusNotFound, Body: roleAssignmentUserMissingBody,
			},
			WantErr: userMissing,
		},
		{
			Method: "RoleAssignmentsService.BulkUnassign",
			Op:     "bulk_unassign_role",
			Route:  "DELETE " + bulk,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.RoleAssignments.BulkUnassign(ctx, []models.RoleAssignmentRemove{
					{User: "50%é", Role: "editor", Tenant: "a/b"},
					{
						User: ".hidden", Role: "owner", Tenant: "tenant-a",
						ResourceInstance: "document:doc-1",
					},
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/role_assignments/bulk",
				Body: `[{"user":"50%é","role":"editor","tenant":"a/b"},` +
					`{"user":".hidden","role":"owner","tenant":"tenant-a",` +
					`"resource_instance":"document:doc-1"}]`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("bulk_unassign_role.200.json")},
			WantResult: &models.BulkRoleUnAssignmentReport{AssignmentsRemoved: 2},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: roleAssignmentInvalidBody,
			},
			WantErr: invalid("bulk_unassign_role", http.MethodDelete, bulk),
		},
	}
}

// tenantAssignment is the role assignment of the reply fixture assign_role.200.json: the role
// editor of idp|dave@example.com in the tenant tenant-a.
func tenantAssignment() *models.RoleAssignmentRead {
	a := inTenantA(models.RoleAssignmentRead{
		ID: assignmentDaveID, User: syncedKey, UserID: userDaveID, Role: "editor",
		RoleID: roleEditorID, CreatedAt: at("2026-10-15T15:00:00Z"),
	})
	return &a
}

// instanceAssignment is the role assignment of the reply fixture assign_role.200.instance.json:
// the role owner of alice on the resource instance document:doc-1, in its tenant tenant-a,
// which the API names in the reply although the request named no tenant.
func instanceAssignment() *models.RoleAssignmentRead {
	a := inTenantA(models.RoleAssignmentRead{
		ID: assignmentOwnerID, User: "alice", UserID: userAliceID, Role: "owner",
		RoleID: roleOwnerID, ResourceInstance: "document:doc-1",
		ResourceInstanceID: assignmentInstanceID, CreatedAt: at("2026-10-15T15:30:00.500000"),
	})
	return &a
}
