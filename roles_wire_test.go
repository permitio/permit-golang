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

// The error bodies of the Permit API in the wire cases of the roles: a key that may not act on
// the roles of the environment, a role that does not exist, a key that another role of the
// environment has already, and fields that the API rejects, of an update and of a bulk replace.
const (
	roleForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the roles of the environment","id":"req-21"}`
	roleNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the role does not exist","id":"req-22"}`
	roleConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"a role with the key auditor exists already","id":"req-23"}`
	roleInvalidBody = `{"detail":[{"loc":["body","extends"],` +
		`"msg":"value is not a valid list","type":"type_error.list"}]}`
	bulkRolesInvalidBody = `{"detail":[{"loc":["body","operations",0,"key"],` +
		`"msg":"string does not match regex","type":"value_error.str.regex"}]}`
)

// roleOf returns a role of the reply fixtures, of the environment of the scope fixture, with the
// fields that every role has.
func roleOf(key string, id int, name, created, updated string) models.RoleRead {
	return models.RoleRead{
		Name: name, Key: key, ID: resourceID(id), OrganizationID: exampleOrganizationID,
		ProjectID: exampleProjectID, EnvironmentID: exampleEnvironmentID, CreatedAt: at(created),
		UpdatedAt: at(updated),
	}
}

// fixtureRole is the role of the reply fixture get_role.200.json, with every field that the API
// returns, and attributes of a string, a number, an integer beyond 2^53 and an object.
func fixtureRole() *models.RoleRead {
	r := roleOf("editor", 102, "Editor", "2026-10-14T09:30:00Z", "2026-10-15T10:00:00.123456")
	r.Description = "Edits documents"
	r.Permissions, r.Extends = []string{"document:read", "document:write"}, []string{"viewer"}
	r.Attributes = models.Attributes{
		"tier": "gold", "seats": json.Number("10"), "limit": json.Number("9007199254740993"),
		"quota": map[string]any{"max": json.Number("2.5"), "regions": []any{"eu", "us"}},
	}
	r.GrantedTo = &models.DerivedRoleBlockRead{
		ID: resourceID(108),
		UsersWithRole: []models.DerivedRoleRuleRead{{
			RoleID: resourceID(41), ResourceID: resourceID(22), RelationID: resourceID(51),
			Role: "owner", OnResource: "folder", LinkedByRelation: "parent",
			When: &models.DerivationSettings{NoDirectRolesOnObject: true},
		}},
		When: &models.DerivationSettings{},
	}
	return &r
}

// listedRoles are the roles of the reply fixture list_roles.200.json, whose attributes hold a
// string, a number and an object.
func listedRoles() []models.RoleRead {
	const first, second = "2026-10-14T09:30:00Z", "2026-10-15T08:00:00Z"
	viewer := roleOf("viewer", 101, "Viewer", first, first)
	viewer.Permissions, viewer.Attributes = []string{"document:read"}, models.Attributes{
		"tier": "free",
	}
	editor := roleOf("editor", 102, "Editor", first, first)
	editor.Description, editor.Permissions = "Edits documents", []string{"document:write"}
	editor.Attributes = models.Attributes{"seats": json.Number("10")}
	editor.Extends = []string{"viewer"}
	admin := roleOf("admin", 103, "Admin", second, second)
	admin.Attributes = models.Attributes{
		"quota": map[string]any{"max": json.Number("3"), "unlimited": false},
	}
	admin.Extends = []string{"editor"}
	return []models.RoleRead{viewer, editor, admin}
}

// editorAt returns the role editor of the reply fixtures of the calls that change it, last
// changed at updated, with permissions and extends, and the description that it keeps.
func editorAt(updated string, permissions, extends []string, attributes models.Attributes,
) *models.RoleRead {
	r := roleOf("editor", 102, "Editor", "2026-10-14T09:30:00Z", updated)
	r.Description, r.Permissions, r.Extends = "Edits documents", permissions, extends
	r.Attributes = attributes
	return &r
}

// firstRoles returns the first n roles that seq yields, and ends the loop there, so that it
// asks for no page past them; or the first error that seq yields.
func firstRoles(seq iter.Seq2[models.RoleRead, error], n int) ([]models.RoleRead, error) {
	var got []models.RoleRead
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

// roleCases returns the wire cases of the roles of an environment. Each sends one request to the
// API, along a route that the runner's scope fills, p and e, as WithScope gives it. The keys of
// the roles hold characters that the path escapes, a slash, a percent sign and a letter outside
// ASCII, as a/b and 50%é do, and a key has a leading dot, .hidden, which the path sends as it is;
// the attribute filters hold one value of each type, and a string and the key of an attribute
// that the query escapes. The replies of List, All and Get hold attributes whose values are
// numbers and objects; that of Ancestors holds its roles in a JSON array, and that of Descendants
// in the paginated object.
func roleCases() []wireCase {
	const (
		rolesRoute       = "/v2/schema/{proj_id}/{env_id}/roles"
		roleRoute        = rolesRoute + "/{role_id}"
		permissionsRoute = roleRoute + "/permissions"
		ancestorsRoute   = roleRoute + "/ancestors"
		descendantsRoute = roleRoute + "/descendants"
		bulkRoute        = "/v2/schema/{proj_id}/{env_id}/bulk/roles"
		roles            = "/v2/schema/p/e/roles"
	)
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, roleForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-21"
		e.Detail = "the API key may not act on the roles of the environment"
		return e
	}
	notFound := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusNotFound, roleNotFoundBody)
		e.Code, e.Title, e.RequestID = "NOT_FOUND", "Not Found", "req-22"
		e.Detail = "the role does not exist"
		return e
	}
	invalid := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusUnprocessableEntity, roleInvalidBody)
		e.Detail = "body.extends: value is not a valid list"
		return e
	}
	listErr := projectError("list_roles", http.MethodGet, rolesRoute,
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	createErr := projectError("create_role", http.MethodPost, rolesRoute, http.StatusConflict,
		roleConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "DUPLICATE_ENTITY", "Conflict", "req-23"
	createErr.Detail = "a role with the key auditor exists already"
	created := roleOf("auditor", 104, "Auditor", "2026-10-15T08:00:00Z", "2026-10-15T08:00:00Z")
	created.Description, created.Permissions = "Reads the audit log", []string{"document:read"}
	created.Attributes = models.Attributes{
		"level": json.Number("2"), "scope": map[string]any{"all": true},
	}
	created.Extends = []string{"viewer"}
	bulkErr := projectError("bulk_create_or_replace_roles", http.MethodPut, bulkRoute,
		http.StatusUnprocessableEntity, bulkRolesInvalidBody)
	bulkErr.Detail = "body.operations.0.key: string does not match regex"
	listed := listedRoles()
	viewer, editor, admin := listed[0], listed[1], listed[2]
	return []wireCase{
		{
			Method: "RolesService.List",
			Op:     "list_roles",
			Route:  "GET " + rolesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Roles.List(ctx, &permit.RolesListOptions{
					ListOptions: permit.ListOptions{Page: 2, PerPage: 10},
					Attributes: []permit.AttributeFilter{
						permit.AttributeEquals("tier name", "gold&é"),
						permit.AttributeEquals("billable", true),
						permit.AttributeEquals("seats", 10),
						permit.AttributeEquals("ratio", 0.5),
					},
				})
			},
			Request: wireRequest{
				RawPath: roles,
				RawQuery: "attr_billable=true&attr_ratio=0.5&attr_seats=10&" +
					"attr_tier+name=gold%26%C3%A9&page=2&per_page=10",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_roles.200.json")},
			WantResult: &permit.Page[models.RoleRead]{
				Data: listedRoles(), Page: 2, PerPage: 10,
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "RolesService.All",
			Op:     "list_roles",
			Route:  "GET " + rolesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstRoles(c.Roles.All(ctx, &permit.RolesListOptions{
					Attributes: []permit.AttributeFilter{permit.AttributeEquals("a/b", "50%é")},
				}), 3)
			},
			Request: wireRequest{
				RawPath: roles, RawQuery: "attr_a%2Fb=50%25%C3%A9&page=1&per_page=100",
			},
			Reply:      mockpermit.Reply{Body: replyFixture("list_roles.200.json")},
			WantResult: listedRoles(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: roleForbiddenBody},
			WantErr:    forbidden("list_roles", http.MethodGet, rolesRoute),
		},
		{
			Method: "RolesService.Get",
			Op:     "get_role",
			Route:  "GET " + roleRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Roles.Get(ctx, ".hidden")
			},
			Request:    wireRequest{RawPath: roles + "/.hidden"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_role.200.json")},
			WantResult: fixtureRole(),
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: roleNotFoundBody},
			WantErr:    notFound("get_role", http.MethodGet, roleRoute),
		},
		{
			Method: "RolesService.Create",
			Op:     "create_role",
			Route:  "POST " + rolesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Roles.Create(ctx, models.RoleCreate{
					Key: "auditor", Name: "Auditor", Description: "Reads the audit log",
					Permissions: []string{"document:read"},
					Attributes: models.Attributes{
						"level": 2, "scope": map[string]any{"all": true},
					},
					Extends: []string{"viewer"},
				})
			},
			Request: wireRequest{
				RawPath: roles,
				Body: `{"key":"auditor","name":"Auditor","description":"Reads the audit log",` +
					`"permissions":["document:read"],"attributes":{"level":2,` +
					`"scope":{"all":true}},"extends":["viewer"]}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("create_role.200.json")},
			WantResult: &created,
			ErrorReply: mockpermit.Reply{Status: http.StatusConflict, Body: roleConflictBody},
			WantErr:    createErr,
		},
		{
			Method: "RolesService.Update",
			Op:     "update_role",
			Route:  "PATCH " + roleRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Roles.Update(ctx, "a/b", models.RoleUpdate{
					Permissions: []string{}, Extends: []string{},
					Attributes: models.Some(models.Attributes{"tier": "silver", "seats": 5}),
					GrantedTo:  models.Null[models.DerivedRoleBlockEdit](),
				})
			},
			Request: wireRequest{
				RawPath: roles + "/a%2Fb",
				Body: `{"permissions":[],"attributes":{"tier":"silver","seats":5},` +
					`"extends":[],"granted_to":null}`,
			},
			Reply: mockpermit.Reply{Body: replyFixture("update_role.200.json")},
			WantResult: editorAt("2026-10-15T13:00:00Z", []string{}, []string{},
				models.Attributes{"tier": "silver", "seats": json.Number("5")}),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: roleInvalidBody,
			},
			WantErr: invalid("update_role", http.MethodPatch, roleRoute),
		},
		{
			Method: "RolesService.Delete",
			Op:     "delete_role",
			Route:  "DELETE " + roleRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Roles.Delete(ctx, "50%é")
			},
			Request:    wireRequest{RawPath: roles + "/50%25%C3%A9"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: roleForbiddenBody},
			WantErr:    forbidden("delete_role", http.MethodDelete, roleRoute),
		},
		{
			Method: "RolesService.AssignPermissions",
			Op:     "assign_permissions_to_role",
			Route:  "POST " + permissionsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Roles.AssignPermissions(ctx, "a/b", []string{"document:share"})
			},
			Request: wireRequest{
				RawPath: roles + "/a%2Fb/permissions",
				Body:    `{"permissions":["document:share"]}`,
			},
			Reply: mockpermit.Reply{Body: replyFixture("assign_permissions_to_role.200.json")},
			WantResult: editorAt("2026-10-15T14:00:00Z",
				[]string{"document:read", "document:write", "document:share"},
				[]string{"viewer"}, models.Attributes{"seats": json.Number("10")}),
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: roleNotFoundBody},
			WantErr: notFound("assign_permissions_to_role", http.MethodPost,
				permissionsRoute),
		},
		{
			Method: "RolesService.RemovePermissions",
			Op:     "remove_permissions_from_role",
			Route:  "DELETE " + permissionsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Roles.RemovePermissions(ctx, ".hidden",
					[]string{"document:write", "document:share"})
			},
			Request: wireRequest{
				RawPath: roles + "/.hidden/permissions",
				Body:    `{"permissions":["document:write","document:share"]}`,
			},
			Reply: mockpermit.Reply{Body: replyFixture("remove_permissions_from_role.200.json")},
			WantResult: editorAt("2026-10-15T15:00:00Z", []string{"document:read"},
				[]string{"viewer"}, models.Attributes{"seats": json.Number("10")}),
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: roleNotFoundBody},
			WantErr: notFound("remove_permissions_from_role", http.MethodDelete,
				permissionsRoute),
		},
		{
			Method: "RolesService.Ancestors",
			Op:     "get_role_ancestors",
			Route:  "GET " + ancestorsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Roles.Ancestors(ctx, "50%é")
			},
			Request:    wireRequest{RawPath: roles + "/50%25%C3%A9/ancestors"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_role_ancestors.200.json")},
			WantResult: []models.RoleRead{editor, viewer},
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: roleNotFoundBody},
			WantErr:    notFound("get_role_ancestors", http.MethodGet, ancestorsRoute),
		},
		{
			Method: "RolesService.Descendants",
			Op:     "get_role_descendants",
			Route:  "GET " + descendantsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Roles.Descendants(ctx, "a/b")
			},
			Request:    wireRequest{RawPath: roles + "/a%2Fb/descendants"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_role_descendants.200.json")},
			WantResult: []models.RoleRead{editor, admin},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: roleForbiddenBody},
			WantErr:    forbidden("get_role_descendants", http.MethodGet, descendantsRoute),
		},
		{
			Method: "RolesService.BulkReplace",
			Op:     "bulk_create_or_replace_roles",
			Route:  "PUT " + bulkRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Roles.BulkReplace(ctx, []models.RoleCreateBulk{
					{Key: "auditor", Name: "Auditor", Permissions: []string{"document:read"}},
					{Key: "viewer", Name: "Viewer", Attributes: models.Attributes{"seats": 1}},
					{Key: "editor", Name: "Editor", Extends: []string{"viewer"}},
				})
			},
			Request: wireRequest{
				RawPath: "/v2/schema/p/e/bulk/roles",
				Body: `{"operations":[{"key":"auditor","name":"Auditor",` +
					`"permissions":["document:read"]},{"key":"viewer","name":"Viewer",` +
					`"attributes":{"seats":1}},{"key":"editor","name":"Editor",` +
					`"extends":["viewer"]}]}`,
			},
			Reply: mockpermit.Reply{Body: replyFixture("bulk_create_or_replace_roles.200.json")},
			WantResult: &models.RoleCreateBulkOperationResult{
				Created: []string{"auditor"}, Updated: []string{"viewer", "editor"},
			},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: bulkRolesInvalidBody,
			},
			WantErr: bulkErr,
		},
	}
}
