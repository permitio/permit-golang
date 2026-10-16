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

// The error bodies of the Permit API in the wire cases of the roles of a resource: a key that may
// not act on the resources of the environment, a role that does not exist, a key that another
// role of the resource has already, and fields that the API rejects.
const (
	resourceRoleForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the resources of the environment","id":"req-31"}`
	resourceRoleNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the role does not exist","id":"req-32"}`
	resourceRoleConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"a role with the key commenter exists already","id":"req-33"}`
	resourceRoleInvalidBody = `{"detail":[{"loc":["body","extends"],` +
		`"msg":"value is not a valid list","type":"type_error.list"}]}`
)

// resourceRoleOf returns a role of the resource document of the reply fixtures, of the
// environment of the scope fixture, with the fields that every role of a resource has.
func resourceRoleOf(key string, id int, name, created, updated string) models.ResourceRoleRead {
	return models.ResourceRoleRead{
		Name: name, Key: key, ID: resourceID(id), OrganizationID: exampleOrganizationID,
		ProjectID: exampleProjectID, EnvironmentID: exampleEnvironmentID,
		ResourceID: resourceID(21), Resource: "document", CreatedAt: at(created),
		UpdatedAt: at(updated),
	}
}

// fixtureResourceRole is the role of the reply fixture get_resource_role.200.json, with every
// field that the API returns, and attributes of a string, a number, an integer beyond 2^53 and an
// object.
func fixtureResourceRole() *models.ResourceRoleRead {
	r := resourceRoleOf("editor", 112, "Editor", "2026-10-14T09:30:00Z",
		"2026-10-15T10:00:00.123456")
	r.Description = "Edits the document"
	r.Permissions, r.Extends = []string{"read", "write"}, []string{"viewer"}
	r.Attributes = models.Attributes{
		"tier": "gold", "seats": json.Number("10"), "limit": json.Number("9007199254740993"),
		"quota": map[string]any{"max": json.Number("2.5"), "regions": []any{"eu", "us"}},
	}
	r.GrantedTo = &models.DerivedRoleBlockRead{
		ID: resourceID(118),
		UsersWithRole: []models.DerivedRoleRuleRead{{
			RoleID: resourceID(41), ResourceID: resourceID(22), RelationID: resourceID(51),
			Role: "owner", OnResource: "folder", LinkedByRelation: "parent",
			When: &models.DerivationSettings{NoDirectRolesOnObject: true},
		}},
		When: &models.DerivationSettings{},
	}
	return &r
}

// listedResourceRoles are the roles of the reply fixture list_resource_roles.200.json, whose
// attributes hold a string, a number and an object.
func listedResourceRoles() []models.ResourceRoleRead {
	const first, second = "2026-10-14T09:30:00Z", "2026-10-15T08:00:00Z"
	viewer := resourceRoleOf("viewer", 111, "Viewer", first, first)
	viewer.Permissions = []string{"read"}
	viewer.Attributes = models.Attributes{"tier": "free"}
	editor := resourceRoleOf("editor", 112, "Editor", first, first)
	editor.Description, editor.Permissions = "Edits the document", []string{"write"}
	editor.Attributes = models.Attributes{"seats": json.Number("10")}
	editor.Extends = []string{"viewer"}
	owner := resourceRoleOf("owner", 113, "Owner", second, second)
	owner.Attributes = models.Attributes{
		"quota": map[string]any{"max": json.Number("3"), "unlimited": false},
	}
	owner.Extends = []string{"editor"}
	return []models.ResourceRoleRead{viewer, editor, owner}
}

// resourceEditorAt returns the role editor of the reply fixtures of the calls that change it,
// last changed at updated, with permissions, extends and attributes, and the description that it
// keeps.
func resourceEditorAt(updated string, permissions, extends []string,
	attributes models.Attributes,
) *models.ResourceRoleRead {
	r := resourceRoleOf("editor", 112, "Editor", "2026-10-14T09:30:00Z", updated)
	r.Description, r.Permissions, r.Extends = "Edits the document", permissions, extends
	r.Attributes = attributes
	return &r
}

// firstResourceRoles returns the first n roles that seq yields, and ends the loop there, so that
// it asks for no page past them; or the first error that seq yields.
func firstResourceRoles(seq iter.Seq2[models.ResourceRoleRead, error], n int,
) ([]models.ResourceRoleRead, error) {
	var got []models.ResourceRoleRead
	for role, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, role)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// resourceRoleCases returns the wire cases of the roles of a resource. Each sends one request to
// the API, along a route that the runner's scope fills, p and e, as WithScope gives it. The keys
// of the resources and of the roles hold characters that the path escapes, a slash, a percent
// sign and a letter outside ASCII, as a/b and 50%é do, and a key has a leading dot, .hidden,
// which the path sends as it is. The replies of List, All and Get hold attributes whose values
// are numbers and objects; that of Ancestors holds its roles in a JSON array, and that of
// Descendants in the paginated object.
func resourceRoleCases() []wireCase {
	const (
		rolesRoute       = "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles"
		roleRoute        = rolesRoute + "/{role_id}"
		permissionsRoute = roleRoute + "/permissions"
		ancestorsRoute   = roleRoute + "/ancestors"
		descendantsRoute = roleRoute + "/descendants"
		schema           = "/v2/schema/p/e/resources/"
	)
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, resourceRoleForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-31"
		e.Detail = "the API key may not act on the resources of the environment"
		return e
	}
	notFound := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusNotFound, resourceRoleNotFoundBody)
		e.Code, e.Title, e.RequestID = "NOT_FOUND", "Not Found", "req-32"
		e.Detail = "the role does not exist"
		return e
	}
	listErr := projectError("list_resource_roles", http.MethodGet, rolesRoute,
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	createErr := projectError("create_resource_role", http.MethodPost, rolesRoute,
		http.StatusConflict, resourceRoleConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "DUPLICATE_ENTITY", "Conflict",
		"req-33"
	createErr.Detail = "a role with the key commenter exists already"
	updateErr := projectError("update_resource_role", http.MethodPatch, roleRoute,
		http.StatusUnprocessableEntity, resourceRoleInvalidBody)
	updateErr.Detail = "body.extends: value is not a valid list"
	created := resourceRoleOf("commenter", 114, "Commenter", "2026-10-15T08:00:00Z",
		"2026-10-15T08:00:00Z")
	created.Description, created.Permissions = "Comments on the document", []string{"read"}
	created.Attributes = models.Attributes{
		"level": json.Number("2"), "scope": map[string]any{"all": true},
	}
	created.Extends = []string{"viewer"}
	listed := listedResourceRoles()
	viewer, editor, owner := listed[0], listed[1], listed[2]
	seats := models.Attributes{"seats": json.Number("10")}
	return []wireCase{
		{
			Method: "ResourceRolesService.List",
			Op:     "list_resource_roles",
			Route:  "GET " + rolesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceRoles.List(ctx, "a/b", &permit.ListOptions{Page: 2, PerPage: 10})
			},
			Request: wireRequest{
				RawPath: schema + "a%2Fb/roles", RawQuery: "page=2&per_page=10",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_resource_roles.200.json")},
			WantResult: &permit.Page[models.ResourceRoleRead]{
				Data: listedResourceRoles(), Page: 2, PerPage: 10,
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "ResourceRolesService.All",
			Op:     "list_resource_roles",
			Route:  "GET " + rolesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstResourceRoles(c.ResourceRoles.All(ctx, "50%é", nil), 3)
			},
			Request: wireRequest{
				RawPath: schema + "50%25%C3%A9/roles", RawQuery: "page=1&per_page=100",
			},
			Reply:      mockpermit.Reply{Body: replyFixture("list_resource_roles.200.json")},
			WantResult: listedResourceRoles(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden, Body: resourceRoleForbiddenBody,
			},
			WantErr: forbidden("list_resource_roles", http.MethodGet, rolesRoute),
		},
		{
			Method: "ResourceRolesService.Get",
			Op:     "get_resource_role",
			Route:  "GET " + roleRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceRoles.Get(ctx, "document", ".hidden")
			},
			Request:    wireRequest{RawPath: schema + "document/roles/.hidden"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_resource_role.200.json")},
			WantResult: fixtureResourceRole(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusNotFound, Body: resourceRoleNotFoundBody,
			},
			WantErr: notFound("get_resource_role", http.MethodGet, roleRoute),
		},
		{
			Method: "ResourceRolesService.Create",
			Op:     "create_resource_role",
			Route:  "POST " + rolesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceRoles.Create(ctx, ".hidden", models.ResourceRoleCreate{
					Key: "commenter", Name: "Commenter", Description: "Comments on the document",
					Permissions: []string{"read"},
					Attributes: models.Attributes{
						"level": 2, "scope": map[string]any{"all": true},
					},
					Extends: []string{"viewer"},
				})
			},
			Request: wireRequest{
				RawPath: schema + ".hidden/roles",
				Body: `{"key":"commenter","name":"Commenter",` +
					`"description":"Comments on the document","permissions":["read"],` +
					`"attributes":{"level":2,"scope":{"all":true}},"extends":["viewer"]}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("create_resource_role.200.json")},
			WantResult: &created,
			ErrorReply: mockpermit.Reply{
				Status: http.StatusConflict, Body: resourceRoleConflictBody,
			},
			WantErr: createErr,
		},
		{
			Method: "ResourceRolesService.Update",
			Op:     "update_resource_role",
			Route:  "PATCH " + roleRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceRoles.Update(ctx, "document", "a/b", models.ResourceRoleUpdate{
					Permissions: []string{}, Extends: []string{},
					Attributes: models.Some(models.Attributes{"tier": "silver", "seats": 5}),
					GrantedTo:  models.Null[models.DerivedRoleBlockEdit](),
				})
			},
			Request: wireRequest{
				RawPath: schema + "document/roles/a%2Fb",
				Body: `{"permissions":[],"attributes":{"tier":"silver","seats":5},` +
					`"extends":[],"granted_to":null}`,
			},
			Reply: mockpermit.Reply{Body: replyFixture("update_resource_role.200.json")},
			WantResult: resourceEditorAt("2026-10-15T13:00:00Z", []string{}, []string{},
				models.Attributes{"tier": "silver", "seats": json.Number("5")}),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: resourceRoleInvalidBody,
			},
			WantErr: updateErr,
		},
		{
			Method: "ResourceRolesService.Delete",
			Op:     "delete_resource_role",
			Route:  "DELETE " + roleRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.ResourceRoles.Delete(ctx, "a/b", "50%é")
			},
			Request:    wireRequest{RawPath: schema + "a%2Fb/roles/50%25%C3%A9"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden, Body: resourceRoleForbiddenBody,
			},
			WantErr: forbidden("delete_resource_role", http.MethodDelete, roleRoute),
		},
		{
			Method: "ResourceRolesService.AssignPermissions",
			Op:     "assign_permissions_to_resource_role",
			Route:  "POST " + permissionsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceRoles.AssignPermissions(ctx, "50%é", "editor",
					[]string{"document:share"})
			},
			Request: wireRequest{
				RawPath: schema + "50%25%C3%A9/roles/editor/permissions",
				Body:    `{"permissions":["document:share"]}`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("assign_permissions_to_resource_role.200.json"),
			},
			WantResult: resourceEditorAt("2026-10-15T14:00:00Z",
				[]string{"read", "write", "share"}, []string{"viewer"}, seats),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusNotFound, Body: resourceRoleNotFoundBody,
			},
			WantErr: notFound("assign_permissions_to_resource_role", http.MethodPost,
				permissionsRoute),
		},
		{
			Method: "ResourceRolesService.RemovePermissions",
			Op:     "remove_permissions_from_resource_role",
			Route:  "DELETE " + permissionsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceRoles.RemovePermissions(ctx, "document", ".hidden",
					[]string{"document:write", "document:share"})
			},
			Request: wireRequest{
				RawPath: schema + "document/roles/.hidden/permissions",
				Body:    `{"permissions":["document:write","document:share"]}`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("remove_permissions_from_resource_role.200.json"),
			},
			WantResult: resourceEditorAt("2026-10-15T15:00:00Z", []string{"read"},
				[]string{"viewer"}, seats),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusNotFound, Body: resourceRoleNotFoundBody,
			},
			WantErr: notFound("remove_permissions_from_resource_role", http.MethodDelete,
				permissionsRoute),
		},
		{
			Method: "ResourceRolesService.Ancestors",
			Op:     "get_resource_role_ancestors",
			Route:  "GET " + ancestorsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceRoles.Ancestors(ctx, ".hidden", "50%é")
			},
			Request: wireRequest{RawPath: schema + ".hidden/roles/50%25%C3%A9/ancestors"},
			Reply: mockpermit.Reply{
				Body: replyFixture("get_resource_role_ancestors.200.json"),
			},
			WantResult: []models.ResourceRoleRead{editor, viewer},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusNotFound, Body: resourceRoleNotFoundBody,
			},
			WantErr: notFound("get_resource_role_ancestors", http.MethodGet, ancestorsRoute),
		},
		{
			Method: "ResourceRolesService.Descendants",
			Op:     "get_resource_role_descendants",
			Route:  "GET " + descendantsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceRoles.Descendants(ctx, "a/b", ".hidden")
			},
			Request: wireRequest{RawPath: schema + "a%2Fb/roles/.hidden/descendants"},
			Reply: mockpermit.Reply{
				Body: replyFixture("get_resource_role_descendants.200.json"),
			},
			WantResult: []models.ResourceRoleRead{editor, owner},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden, Body: resourceRoleForbiddenBody,
			},
			WantErr: forbidden("get_resource_role_descendants", http.MethodGet,
				descendantsRoute),
		},
	}
}
