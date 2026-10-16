package permit_test

import (
	"context"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the roles of a group and of the groups
// assigned to a group: an assignment of a role without the role, and of a group without its key.
const (
	groupRoleMissingBody = `{"detail":[{"loc":["body","role"],"msg":"field required",` +
		`"type":"value_error.missing"}]}`
	groupKeyMissingBody = `{"detail":[{"loc":["body","group_instance_key"],` +
		`"msg":"field required","type":"value_error.missing"}]}`
)

// groupRoleOf returns a role of the reply fixture list_group_roles.200.json, on an instance of
// the resource document: the key of the role, and the key and the last digits of the ID of the
// instance, as resourceID takes them.
func groupRoleOf(key, instance string, id int) models.GroupRoleReadSchema {
	return models.GroupRoleReadSchema{
		Key:              key,
		Resource:         models.GroupRoleResourceReadOnly{ID: resourceID(91), Key: "document"},
		ResourceInstance: models.GroupRoleResourceReadOnly{ID: resourceID(id), Key: instance},
	}
}

// listedGroupRoles are the roles of the reply fixture list_group_roles.200.json.
func listedGroupRoles() []models.GroupRoleReadSchema {
	return []models.GroupRoleReadSchema{
		groupRoleOf("editor", "doc-1", 92), groupRoleOf("viewer", "doc-2", 93),
	}
}

// listedGroupChildren are the groups of the reply fixture list_group_children.200.json: one with
// its resource, and one without.
func listedGroupChildren() []models.GroupReadSchema {
	backend := groupOf("backend", 83)
	backend.GroupResourceTypeKey = "group"
	return []models.GroupReadSchema{backend, groupOf("frontend", 84)}
}

// groupRoleCases returns the wire cases of the roles of a group and of the groups assigned to a
// group. Each sends one request to the API, along a route that the runner's scope fills, p and e,
// as WithScope gives it. The instance keys of the groups in the path hold characters that the
// path escapes, a slash, a percent sign and a letter outside ASCII, as a/b and 50%é do; one has a
// leading dot, .hidden, and another the shape of a UUID, which the path sends as they are. The
// bodies hold such keys too, which they send as they are. Each list's reply is the paginated
// object, whose counts the page holds.
func groupRoleCases() []wireCase {
	const (
		rolesRoute     = "/v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}/roles"
		assignRoute    = "/v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}/assign_group"
		childrenRoute  = "/v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}/children"
		parentsRoute   = "/v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}/parents"
		groups         = "/v2/schema/p/e/groups"
		groupNotFound  = "the group does not exist"
		groupForbidden = "the API key may not act on the groups of the environment"
	)
	role := models.GroupAddRole{
		Role: "editor", Resource: "50%é", ResourceInstance: "a/b", Tenant: fixtures.UUIDKey,
	}
	roleBody := `{"role":"editor","resource":"50%é","resource_instance":"a/b",` +
		`"tenant":"` + fixtures.UUIDKey + `"}`
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, groupForbiddenBody)
		e.Code, e.Title, e.RequestID, e.Detail = "FORBIDDEN", "Forbidden", "req-21", groupForbidden
		return e
	}
	notFound := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusNotFound, groupNotFoundBody)
		e.Code, e.Title, e.RequestID, e.Detail = "NOT_FOUND", "Not Found", "req-22", groupNotFound
		return e
	}
	missing := func(op, method, route, body, detail string) *permit.Error {
		e := projectError(op, method, route, http.StatusUnprocessableEntity, body)
		e.Detail = detail
		return e
	}
	perPage := missing("list_group_children", http.MethodGet, childrenRoute, perPageBody,
		"query.per_page: ensure this value is less than or equal to 100")
	return []wireCase{
		{
			Method: "GroupsService.AssignRole",
			Op:     "assign_role_to_group",
			Route:  "POST " + rolesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Groups.AssignRole(ctx, "a/b", role)
			},
			Request: wireRequest{RawPath: groups + "/a%2Fb/roles", Body: roleBody},
			Reply:   mockpermit.Reply{Body: replyFixture("assign_role_to_group.200.json")},
			WantResult: &models.GroupRead{
				AssignedRoles: []string{"editor"}, GroupInstanceKey: "engineering",
				GroupResourceTypeKey: "group", GroupTenant: "tenant-a",
				Users: []string{resourceID(31)},
			},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: groupRoleMissingBody,
			},
			WantErr: missing("assign_role_to_group", http.MethodPost, rolesRoute,
				groupRoleMissingBody, "body.role: field required"),
		},
		{
			Method: "GroupsService.RemoveRole",
			Op:     "remove_role_from_group",
			Route:  "DELETE " + rolesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Groups.RemoveRole(ctx, ".hidden", role)
			},
			Request:    wireRequest{RawPath: groups + "/.hidden/roles", Body: roleBody},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: groupNotFoundBody},
			WantErr:    notFound("remove_role_from_group", http.MethodDelete, rolesRoute),
		},
		{
			Method: "GroupsService.ListRoles",
			Op:     "list_group_roles",
			Route:  "GET " + rolesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Groups.ListRoles(ctx, fixtures.UUIDKey,
					&permit.ListOptions{Page: 2, PerPage: 2})
			},
			Request: wireRequest{
				RawPath:  groups + "/" + fixtures.UUIDKey + "/roles",
				RawQuery: "page=2&per_page=2",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_group_roles.200.json")},
			WantResult: &permit.Page[models.GroupRoleReadSchema]{
				Data: listedGroupRoles(), Page: 2, PerPage: 2, TotalCount: new(2),
				PageCount: new(1),
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: groupNotFoundBody},
			WantErr:    notFound("list_group_roles", http.MethodGet, rolesRoute),
		},
		{
			Method: "GroupsService.AllRoles",
			Op:     "list_group_roles",
			Route:  "GET " + rolesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstOf(c.Groups.AllRoles(ctx, "50%é", nil), 2)
			},
			Request: wireRequest{
				RawPath: groups + "/50%25%C3%A9/roles", RawQuery: "page=1&per_page=100",
			},
			Reply:      mockpermit.Reply{Body: replyFixture("list_group_roles.200.json")},
			WantResult: listedGroupRoles(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: groupForbiddenBody},
			WantErr:    forbidden("list_group_roles", http.MethodGet, rolesRoute),
		},
		{
			Method: "GroupsService.AssignGroup",
			Op:     "assign_group_to_group",
			Route:  "PUT " + assignRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Groups.AssignGroup(ctx, "a/b",
					models.GroupAssignment{GroupInstanceKey: "group:50%é"})
			},
			Request: wireRequest{
				RawPath: groups + "/a%2Fb/assign_group",
				Body:    `{"group_instance_key":"group:50%é"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("assign_group_to_group.200.json")},
			WantResult: groupWithUsers([]string{}...),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: groupKeyMissingBody,
			},
			WantErr: missing("assign_group_to_group", http.MethodPut, assignRoute,
				groupKeyMissingBody, "body.group_instance_key: field required"),
		},
		{
			Method: "GroupsService.RemoveGroup",
			Op:     "remove_group_from_group",
			Route:  "DELETE " + assignRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Groups.RemoveGroup(ctx, fixtures.UUIDKey,
					models.GroupAssignment{GroupInstanceKey: "a/b"})
			},
			Request: wireRequest{
				RawPath: groups + "/" + fixtures.UUIDKey + "/assign_group",
				Body:    `{"group_instance_key":"a/b"}`,
			},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: groupNotFoundBody},
			WantErr:    notFound("remove_group_from_group", http.MethodDelete, assignRoute),
		},
		{
			Method: "GroupsService.ListChildren",
			Op:     "list_group_children",
			Route:  "GET " + childrenRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Groups.ListChildren(ctx, ".hidden",
					&permit.ListOptions{Page: 1, PerPage: 10})
			},
			Request: wireRequest{
				RawPath: groups + "/.hidden/children", RawQuery: "page=1&per_page=10",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_group_children.200.json")},
			WantResult: &permit.Page[models.GroupReadSchema]{
				Data: listedGroupChildren(), Page: 1, PerPage: 10, TotalCount: new(2),
				PageCount: new(1),
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    perPage,
		},
		{
			Method: "GroupsService.AllChildren",
			Op:     "list_group_children",
			Route:  "GET " + childrenRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstOf(c.Groups.AllChildren(ctx, "a/b", nil), 2)
			},
			Request: wireRequest{
				RawPath: groups + "/a%2Fb/children", RawQuery: "page=1&per_page=100",
			},
			Reply:      mockpermit.Reply{Body: replyFixture("list_group_children.200.json")},
			WantResult: listedGroupChildren(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: groupForbiddenBody},
			WantErr:    forbidden("list_group_children", http.MethodGet, childrenRoute),
		},
		{
			Method: "GroupsService.ListParents",
			Op:     "list_group_parents",
			Route:  "GET " + parentsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Groups.ListParents(ctx, "50%é", nil)
			},
			Request: wireRequest{RawPath: groups + "/50%25%C3%A9/parents"},
			Reply:   mockpermit.Reply{Body: replyFixture("list_group_parents.200.json")},
			WantResult: &permit.Page[models.GroupReadSchema]{
				Data: []models.GroupReadSchema{*fixtureGroup()}, Page: 1, TotalCount: new(1),
				PageCount: new(1),
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: groupNotFoundBody},
			WantErr:    notFound("list_group_parents", http.MethodGet, parentsRoute),
		},
		{
			Method: "GroupsService.AllParents",
			Op:     "list_group_parents",
			Route:  "GET " + parentsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstOf(c.Groups.AllParents(ctx, fixtures.UUIDKey, nil), 1)
			},
			Request: wireRequest{
				RawPath:  groups + "/" + fixtures.UUIDKey + "/parents",
				RawQuery: "page=1&per_page=100",
			},
			Reply:      mockpermit.Reply{Body: replyFixture("list_group_parents.200.json")},
			WantResult: []models.GroupReadSchema{*fixtureGroup()},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: groupForbiddenBody},
			WantErr:    forbidden("list_group_parents", http.MethodGet, parentsRoute),
		},
	}
}
