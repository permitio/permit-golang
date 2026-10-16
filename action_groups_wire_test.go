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

// The error bodies of the Permit API in the wire cases of the action groups: a key that may not
// act on the resources of the environment, an action group that does not exist, a key that
// another action group of the resource has already, and a field that the API rejects.
const (
	actionGroupForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the resources of the environment","id":"req-9"}`
	actionGroupNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the action group does not exist","id":"req-10"}`
	actionGroupConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"an action group with the key archiving exists already","id":"req-11"}`
	actionGroupInvalidBody = `{"detail":[{"loc":["body","actions",0],` +
		`"msg":"str type expected","type":"type_error.str"}]}`
)

// actionGroupOf returns an action group of the reply fixtures, of the resource document of the
// environment of the scope fixture, with the fields that every action group has and actions.
func actionGroupOf(key string, id int, name, created, updated string,
	actions ...string,
) models.ResourceActionGroupRead {
	return models.ResourceActionGroupRead{
		Name: name, Actions: actions, Key: key, ID: resourceID(id),
		OrganizationID: exampleOrganizationID, ProjectID: exampleProjectID,
		EnvironmentID: exampleEnvironmentID, ResourceID: resourceID(21), CreatedAt: at(created),
		UpdatedAt: at(updated),
	}
}

// fixtureActionGroup is the action group of the reply fixture get_resource_action_group.200.json,
// with every field.
func fixtureActionGroup() *models.ResourceActionGroupRead {
	g := actionGroupOf("editing", 81, "Editing", "2026-10-14T09:30:00Z",
		"2026-10-15T10:00:00.123456", "read", "write")
	g.Description = "Read and write a document"
	g.Attributes = models.Attributes{
		"risk": json.Number("2"), "limit": json.Number("9007199254740993"),
	}
	return &g
}

// listedActionGroups are the action groups of the reply fixture
// list_resource_action_groups.200.json: one without a description and attributes, and one with
// the empty attributes {} and the empty actions [], which stay empty, not nil.
func listedActionGroups() []models.ResourceActionGroupRead {
	viewing := actionGroupOf("viewing", 82, "Viewing", "2026-10-15T08:00:00Z",
		"2026-10-15T08:00:00Z")
	viewing.Description, viewing.Attributes = "Read a document", models.Attributes{}
	viewing.Actions = []string{}
	return []models.ResourceActionGroupRead{
		actionGroupOf("editing", 81, "Editing", "2026-10-14T09:30:00Z", "2026-10-14T09:30:00Z",
			"read", "write"),
		viewing,
	}
}

// firstActionGroups returns the first n action groups that seq yields, and ends the loop there,
// so that it asks for no page past them; or the first error that seq yields.
func firstActionGroups(seq iter.Seq2[models.ResourceActionGroupRead, error], n int,
) ([]models.ResourceActionGroupRead, error) {
	var got []models.ResourceActionGroupRead
	for group, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, group)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// actionGroupCases returns the wire cases of the action groups of a resource. Each sends one
// request to the API, along a route that the runner's scope fills, p and e, as WithScope gives
// it. The keys of the resources and of the action groups hold characters that the path escapes,
// a slash, a percent sign and a letter outside ASCII, as a/b and 50%é do; a key has a leading
// dot, .hidden, and another the shape of a UUID, which the path sends as they are. The route of
// Delete is that of an action group, never that of an action of the resource.
func actionGroupCases() []wireCase {
	const (
		groupsRoute = "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/action_groups"
		groupRoute  = groupsRoute + "/{action_group_id}"
		document    = "/v2/schema/p/e/resources/document/action_groups"
	)
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, actionGroupForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-9"
		e.Detail = "the API key may not act on the resources of the environment"
		return e
	}
	listErr := projectError("list_resource_action_groups", http.MethodGet, groupsRoute,
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	getErr := projectError("get_resource_action_group", http.MethodGet, groupRoute,
		http.StatusNotFound, actionGroupNotFoundBody)
	getErr.Code, getErr.Title, getErr.RequestID = "NOT_FOUND", "Not Found", "req-10"
	getErr.Detail = "the action group does not exist"
	createErr := projectError("create_resource_action_group", http.MethodPost, groupsRoute,
		http.StatusConflict, actionGroupConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "DUPLICATE_ENTITY", "Conflict", "req-11"
	createErr.Detail = "an action group with the key archiving exists already"
	updateErr := projectError("update_resource_action_group", http.MethodPatch, groupRoute,
		http.StatusUnprocessableEntity, actionGroupInvalidBody)
	updateErr.Detail = "body.actions.0: str type expected"
	return []wireCase{
		{
			Method: "ResourceActionGroupsService.List",
			Op:     "list_resource_action_groups",
			Route:  "GET " + groupsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceActionGroups.List(ctx, "a/b",
					&permit.ListOptions{Page: 2, PerPage: 10})
			},
			Request: wireRequest{
				RawPath:  "/v2/schema/p/e/resources/a%2Fb/action_groups",
				RawQuery: "page=2&per_page=10",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_resource_action_groups.200.json")},
			WantResult: &permit.Page[models.ResourceActionGroupRead]{
				Data: listedActionGroups(), Page: 2, PerPage: 10,
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "ResourceActionGroupsService.All",
			Op:     "list_resource_action_groups",
			Route:  "GET " + groupsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstActionGroups(c.ResourceActionGroups.All(ctx, resourceID(21), nil), 2)
			},
			Request: wireRequest{
				RawPath:  "/v2/schema/p/e/resources/" + resourceID(21) + "/action_groups",
				RawQuery: "page=1&per_page=100",
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("list_resource_action_groups.200.json"),
			},
			WantResult: listedActionGroups(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden, Body: actionGroupForbiddenBody,
			},
			WantErr: forbidden("list_resource_action_groups", http.MethodGet, groupsRoute),
		},
		{
			Method: "ResourceActionGroupsService.Get",
			Op:     "get_resource_action_group",
			Route:  "GET " + groupRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceActionGroups.Get(ctx, "document", "50%é")
			},
			Request:    wireRequest{RawPath: document + "/50%25%C3%A9"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_resource_action_group.200.json")},
			WantResult: fixtureActionGroup(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusNotFound, Body: actionGroupNotFoundBody,
			},
			WantErr: getErr,
		},
		{
			Method: "ResourceActionGroupsService.Create",
			Op:     "create_resource_action_group",
			Route:  "POST " + groupsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceActionGroups.Create(ctx, ".hidden",
					models.ResourceActionGroupCreate{
						Key: "archiving", Name: "Archiving", Description: "Archive a document",
						Attributes: models.Attributes{"tier": "gold"},
						Actions:    []string{"read", "archive"},
					})
			},
			Request: wireRequest{
				RawPath: "/v2/schema/p/e/resources/.hidden/action_groups",
				Body: `{"key":"archiving","name":"Archiving","description":"Archive a document",` +
					`"attributes":{"tier":"gold"},"actions":["read","archive"]}`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("create_resource_action_group.200.json"),
			},
			WantResult: createdActionGroup(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusConflict, Body: actionGroupConflictBody,
			},
			WantErr: createErr,
		},
		{
			Method: "ResourceActionGroupsService.Update",
			Op:     "update_resource_action_group",
			Route:  "PATCH " + groupRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceActionGroups.Update(ctx, "document", "a/b",
					models.ResourceActionGroupUpdate{
						Name: "Renamed editing", Description: models.Null[string](),
						Actions: models.Some([]string{"read"}),
					})
			},
			Request: wireRequest{
				RawPath: document + "/a%2Fb",
				Body:    `{"name":"Renamed editing","description":null,"actions":["read"]}`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("update_resource_action_group.200.json"),
			},
			WantResult: updatedActionGroup(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: actionGroupInvalidBody,
			},
			WantErr: updateErr,
		},
		{
			Method: "ResourceActionGroupsService.Delete",
			Op:     "delete_resource_action_group",
			Route:  "DELETE " + groupRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.ResourceActionGroups.Delete(ctx, "document", ".hidden")
			},
			Request:    wireRequest{RawPath: document + "/.hidden"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden, Body: actionGroupForbiddenBody,
			},
			WantErr: forbidden("delete_resource_action_group", http.MethodDelete, groupRoute),
		},
	}
}

// createdActionGroup is the action group of the reply fixture
// create_resource_action_group.200.json.
func createdActionGroup() *models.ResourceActionGroupRead {
	g := actionGroupOf("archiving", 83, "Archiving", "2026-10-15T11:00:00Z",
		"2026-10-15T11:00:00Z", "read", "archive")
	g.Description, g.Attributes = "Archive a document", models.Attributes{"tier": "gold"}
	return &g
}

// updatedActionGroup is the action group of the reply fixture
// update_resource_action_group.200.json.
func updatedActionGroup() *models.ResourceActionGroupRead {
	g := actionGroupOf("editing", 81, "Renamed editing", "2026-10-14T09:30:00Z",
		"2026-10-15T13:00:00Z", "read")
	return &g
}
