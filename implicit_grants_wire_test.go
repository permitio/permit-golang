package permit_test

import (
	"context"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the implicit grants of a role of a
// resource: a rule that the role has already, a rule that it does not have, and a field that the
// API rejects.
const (
	implicitGrantConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"the role has the implicit grant already","id":"req-41"}`
	implicitGrantNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the role has no such implicit grant","id":"req-42"}`
	implicitGrantInvalidBody = `{"detail":[{"loc":["body","no_direct_roles_on_object"],` +
		`"msg":"value could not be parsed to a boolean","type":"type_error.bool"}]}`
)

// fixtureImplicitGrant is the rule of the reply fixture create_implicit_grant.200.json, with
// every field that the API returns: the users that have the role owner on a folder that is the
// parent of an instance get the role of the route on it.
func fixtureImplicitGrant() *models.DerivedRoleRuleRead {
	return &models.DerivedRoleRuleRead{
		RoleID: resourceID(41), ResourceID: resourceID(22), RelationID: resourceID(51),
		Role: "owner", OnResource: "folder", LinkedByRelation: "parent",
		When: &models.DerivationSettings{NoDirectRolesOnObject: true},
	}
}

// implicitGrantCases returns the wire cases of the implicit grants of a role of a resource. Each
// sends one request to the API, along a route that the runner's scope fills, p and e, as
// WithScope gives it. The keys of the resources and of the roles hold characters that the path
// escapes, a slash, a percent sign and a letter outside ASCII, as a/b and 50%é do, and a key has
// a leading dot, .hidden, which the path sends as it is; each method takes the key of the
// resource first and that of the role second, as the route names them. A 404 to Delete is an
// error that matches ErrNotFound, as its body names the rule to delete.
func implicitGrantCases() []wireCase {
	const (
		grantsRoute = "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/" +
			"implicit_grants"
		conditionsRoute = grantsRoute + "/conditions"
		schema          = "/v2/schema/p/e/resources/"
		rule            = `{"role":"owner","on_resource":"folder","linked_by_relation":"parent"`
	)
	createErr := projectError("create_implicit_grant", http.MethodPost, grantsRoute,
		http.StatusConflict, implicitGrantConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "DUPLICATE_ENTITY", "Conflict",
		"req-41"
	createErr.Detail = "the role has the implicit grant already"
	deleteErr := projectError("delete_implicit_grant", http.MethodDelete, grantsRoute,
		http.StatusNotFound, implicitGrantNotFoundBody)
	deleteErr.Code, deleteErr.Title, deleteErr.RequestID = "NOT_FOUND", "Not Found", "req-42"
	deleteErr.Detail = "the role has no such implicit grant"
	conditionsErr := projectError("update_implicit_grants_conditions", http.MethodPut,
		conditionsRoute, http.StatusUnprocessableEntity, implicitGrantInvalidBody)
	conditionsErr.Detail = "body.no_direct_roles_on_object: value could not be parsed to a boolean"
	return []wireCase{
		{
			Method: "ImplicitGrantsService.Create",
			Op:     "create_implicit_grant",
			Route:  "POST " + grantsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ImplicitGrants.Create(ctx, ".hidden", "a/b", models.DerivedRoleRuleCreate{
					Role: "owner", OnResource: "folder", LinkedByRelation: "parent",
					When: &models.DerivationSettings{NoDirectRolesOnObject: true},
				})
			},
			Request: wireRequest{
				RawPath: schema + ".hidden/roles/a%2Fb/implicit_grants",
				Body:    rule + `,"when":{"no_direct_roles_on_object":true}}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("create_implicit_grant.200.json")},
			WantResult: fixtureImplicitGrant(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusConflict, Body: implicitGrantConflictBody,
			},
			WantErr: createErr,
		},
		{
			Method: "ImplicitGrantsService.Delete",
			Op:     "delete_implicit_grant",
			Route:  "DELETE " + grantsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.ImplicitGrants.Delete(ctx, "a/b", "50%é",
					models.DerivedRoleRuleDelete{
						Role: "owner", OnResource: "folder", LinkedByRelation: "parent",
					})
			},
			Request: wireRequest{
				RawPath: schema + "a%2Fb/roles/50%25%C3%A9/implicit_grants", Body: rule + `}`,
			},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusNotFound, Body: implicitGrantNotFoundBody,
			},
			WantErr: deleteErr,
		},
		{
			Method: "ImplicitGrantsService.UpdateConditions",
			Op:     "update_implicit_grants_conditions",
			Route:  "PUT " + conditionsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ImplicitGrants.UpdateConditions(ctx, "50%é", ".hidden",
					models.DerivationSettings{NoDirectRolesOnObject: true})
			},
			Request: wireRequest{
				RawPath: schema + "50%25%C3%A9/roles/.hidden/implicit_grants/conditions",
				Body:    `{"no_direct_roles_on_object":true}`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("update_implicit_grants_conditions.200.json"),
			},
			WantResult: &models.DerivationSettings{NoDirectRolesOnObject: true},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: implicitGrantInvalidBody,
			},
			WantErr: conditionsErr,
		},
	}
}
