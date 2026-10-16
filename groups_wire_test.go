package permit_test

import (
	"context"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the groups: a key that may not act on
// the groups of the environment, a group that does not exist, a group whose instance key another
// group has already, and an assignment of a user without a tenant.
const (
	groupForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the groups of the environment","id":"req-21"}`
	groupNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the group does not exist","id":"req-22"}`
	groupConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"a group with the key engineering exists already","id":"req-23"}`
	groupTenantMissingBody = `{"detail":[{"loc":["body","tenant"],"msg":"field required",` +
		`"type":"value_error.missing"}]}`
)

// groupOf returns a group of the reply fixtures of a get and a list, in the tenant tenant-a: its
// instance key and the last digits of its ID, as resourceID takes them.
func groupOf(key string, id int) models.GroupReadSchema {
	return models.GroupReadSchema{
		GroupInstanceKey: key,
		GroupTenant:      "tenant-a",
		ID:               resourceID(id),
	}
}

// fixtureGroup is the group of the reply fixture get_direct_group.200.json, with every field.
func fixtureGroup() *models.GroupReadSchema {
	g := groupOf("engineering", 81)
	g.GroupResourceTypeKey = "group"
	return &g
}

// listedGroups are the groups of the reply fixture list_direct_group.200.json: one with its
// resource, and one without.
func listedGroups() []models.GroupReadSchema {
	return []models.GroupReadSchema{*fixtureGroup(), groupOf("sales", 82)}
}

// groupWithUsers returns the group engineering of the reply fixtures of a create and an
// assignment, with the role viewer and users, the IDs of its users.
func groupWithUsers(users ...string) *models.GroupRead {
	return &models.GroupRead{
		AssignedRoles: []string{"viewer"}, GroupInstanceKey: "engineering",
		GroupResourceTypeKey: "group", GroupTenant: "tenant-a", Users: users,
	}
}

// listedGroupUsers are the users of the reply fixture list_group_users.200.json.
func listedGroupUsers() []models.GroupUserReadSchema {
	return []models.GroupUserReadSchema{
		{ID: resourceID(31), Key: "alice"}, {ID: resourceID(32), Key: "bob"},
	}
}

// groupCases returns the wire cases of the groups of an environment and of their users. Each
// sends one request to the API, along a route that the runner's scope fills, p and e, as
// WithScope gives it. The instance keys of the groups and the keys of the users hold characters
// that the path escapes, a slash, a percent sign and a letter outside ASCII, as a/b and 50%é do;
// one of each has a leading dot, .hidden, and another the shape of a UUID, which the path sends as
// they are, as the SDK does not tell an instance key from an ID. The filters of the list hold a
// space and an ampersand, which the query escapes. Each list's reply is the paginated object,
// whose counts the page holds.
func groupCases() []wireCase {
	const (
		groupsRoute    = "/v2/schema/{proj_id}/{env_id}/groups"
		directRoute    = groupsRoute + "/direct"
		groupRoute     = groupsRoute + "/{group_instance_key}"
		usersRoute     = groupRoute + "/users"
		userRoute      = usersRoute + "/{user_id}"
		groups         = "/v2/schema/p/e/groups"
		tenantMissing  = "body.tenant: field required"
		groupNotFound  = "the group does not exist"
		groupForbidden = "the API key may not act on the groups of the environment"
	)
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
	listErr := projectError("list_direct_group", http.MethodGet, directRoute,
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	createErr := projectError("create_group", http.MethodPost, groupsRoute, http.StatusConflict,
		groupConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "DUPLICATE_ENTITY", "Conflict", "req-23"
	createErr.Detail = "a group with the key engineering exists already"
	assignErr := projectError("assign_user_to_group", http.MethodPut, userRoute,
		http.StatusUnprocessableEntity, groupTenantMissingBody)
	assignErr.Detail = tenantMissing
	return []wireCase{
		{
			Method: "GroupsService.List",
			Op:     "list_direct_group",
			Route:  "GET " + directRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Groups.List(ctx, &permit.GroupsListOptions{
					ListOptions: permit.ListOptions{Page: 1, PerPage: 10},
					Tenant:      "tenant a&b", Resource: "group", Search: "eng",
				})
			},
			Request: wireRequest{
				RawPath:  groups + "/direct",
				RawQuery: "page=1&per_page=10&resource=group&search=eng&tenant=tenant+a%26b",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_direct_group.200.json")},
			WantResult: &permit.Page[models.GroupReadSchema]{
				Data: listedGroups(), Page: 1, PerPage: 10, TotalCount: new(2), PageCount: new(1),
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "GroupsService.All",
			Op:     "list_direct_group",
			Route:  "GET " + directRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstOf(c.Groups.All(ctx, nil), 2)
			},
			Request:    wireRequest{RawPath: groups + "/direct", RawQuery: "page=1&per_page=100"},
			Reply:      mockpermit.Reply{Body: replyFixture("list_direct_group.200.json")},
			WantResult: listedGroups(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: groupForbiddenBody},
			WantErr:    forbidden("list_direct_group", http.MethodGet, directRoute),
		},
		{
			Method: "GroupsService.Get",
			Op:     "get_direct_group",
			Route:  "GET " + directRoute + "/{group_instance_key}",
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Groups.Get(ctx, "a/b")
			},
			Request:    wireRequest{RawPath: groups + "/direct/a%2Fb"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_direct_group.200.json")},
			WantResult: fixtureGroup(),
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: groupNotFoundBody},
			WantErr: notFound("get_direct_group", http.MethodGet,
				directRoute+"/{group_instance_key}"),
		},
		{
			Method: "GroupsService.Create",
			Op:     "create_group",
			Route:  "POST " + groupsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Groups.Create(ctx, models.GroupCreate{
					GroupInstanceKey: "engineering", GroupResourceTypeKey: "group",
					GroupTenant: "tenant-a",
				})
			},
			Request: wireRequest{
				RawPath: groups,
				Body: `{"group_instance_key":"engineering","group_resource_type_key":"group",` +
					`"group_tenant":"tenant-a"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("create_group.200.json")},
			WantResult: groupWithUsers([]string{}...),
			ErrorReply: mockpermit.Reply{Status: http.StatusConflict, Body: groupConflictBody},
			WantErr:    createErr,
		},
		{
			Method: "GroupsService.Delete",
			Op:     "delete_group",
			Route:  "DELETE " + groupRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Groups.Delete(ctx, "50%é")
			},
			Request:    wireRequest{RawPath: groups + "/50%25%C3%A9"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: groupForbiddenBody},
			WantErr:    forbidden("delete_group", http.MethodDelete, groupRoute),
		},
		{
			Method: "GroupsService.ListUsers",
			Op:     "list_group_users",
			Route:  "GET " + usersRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Groups.ListUsers(ctx, fixtures.UUIDKey,
					&permit.ListOptions{Page: 2, PerPage: 2})
			},
			Request: wireRequest{
				RawPath:  groups + "/" + fixtures.UUIDKey + "/users",
				RawQuery: "page=2&per_page=2",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_group_users.200.json")},
			WantResult: &permit.Page[models.GroupUserReadSchema]{
				Data: listedGroupUsers(), Page: 2, PerPage: 2, TotalCount: new(2),
				PageCount: new(1),
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: groupNotFoundBody},
			WantErr:    notFound("list_group_users", http.MethodGet, usersRoute),
		},
		{
			Method: "GroupsService.AllUsers",
			Op:     "list_group_users",
			Route:  "GET " + usersRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstOf(c.Groups.AllUsers(ctx, ".hidden", nil), 2)
			},
			Request: wireRequest{
				RawPath: groups + "/.hidden/users", RawQuery: "page=1&per_page=100",
			},
			Reply:      mockpermit.Reply{Body: replyFixture("list_group_users.200.json")},
			WantResult: listedGroupUsers(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: groupForbiddenBody},
			WantErr:    forbidden("list_group_users", http.MethodGet, usersRoute),
		},
		{
			Method: "GroupsService.AssignUser",
			Op:     "assign_user_to_group",
			Route:  "PUT " + userRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Groups.AssignUser(ctx, "a/b", "50%é",
					models.GroupAssignUser{Tenant: "tenant-a"})
			},
			Request: wireRequest{
				RawPath: groups + "/a%2Fb/users/50%25%C3%A9", Body: `{"tenant":"tenant-a"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("assign_user_to_group.200.json")},
			WantResult: groupWithUsers(resourceID(31), resourceID(32)),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: groupTenantMissingBody,
			},
			WantErr: assignErr,
		},
		{
			Method: "GroupsService.RemoveUser",
			Op:     "remove_user_from_group",
			Route:  "DELETE " + userRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Groups.RemoveUser(ctx, ".hidden", fixtures.UUIDKey,
					models.GroupAssignUser{Tenant: "tenant-a"})
			},
			Request: wireRequest{
				RawPath: groups + "/.hidden/users/" + fixtures.UUIDKey,
				Body:    `{"tenant":"tenant-a"}`,
			},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: groupNotFoundBody},
			WantErr:    notFound("remove_user_from_group", http.MethodDelete, userRoute),
		},
	}
}
