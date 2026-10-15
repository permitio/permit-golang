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

// The error bodies of the Permit API in the wire cases of the actions of a resource: a key that
// may not act on the resources of the environment, an action that does not exist, a key that
// another action of the resource has already, and a field that the API rejects.
const (
	actionForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the resources of the environment","id":"req-12"}`
	actionNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the action does not exist","id":"req-13"}`
	actionConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"an action with the key approve exists already","id":"req-14"}`
	actionInvalidBody = `{"detail":[{"loc":["body","name"],` +
		`"msg":"none is not an allowed value","type":"type_error.none.not_allowed"}]}`
)

// actionOf returns an action of the resource document of the reply fixtures, of the environment
// of the scope fixture, with the fields that every action has.
func actionOf(key string, id int, name, created, updated string) models.ResourceActionRead {
	return models.ResourceActionRead{
		Name: name, Key: key, ID: resourceID(id), PermissionName: "document:" + key,
		OrganizationID: exampleOrganizationID, ProjectID: exampleProjectID,
		EnvironmentID: exampleEnvironmentID, ResourceID: resourceID(21), CreatedAt: at(created),
		UpdatedAt: at(updated),
	}
}

// fixtureAction is the action of the reply fixture get_resource_action.200.json, with every
// field.
func fixtureAction() *models.ResourceActionRead {
	a := actionOf("read", 81, "Read", "2026-10-14T09:30:00Z", "2026-10-15T10:00:00.123456")
	a.Description = "Read a document"
	a.Attributes = models.Attributes{"risk": "low", "limit": json.Number("9007199254740993")}
	a.V1CompatPath, a.V1CompatIsBuiltIn, a.V1CompatName = "/documents/read", true, "read"
	return &a
}

// listedActions are the actions of the reply fixture list_resource_actions.200.json.
func listedActions() []models.ResourceActionRead {
	read := actionOf("read", 81, "Read", "2026-10-14T09:30:00Z", "2026-10-14T09:30:00Z")
	read.Attributes = models.Attributes{"risk": "low"}
	share := actionOf("share", 82, "Share", "2026-10-15T08:00:00Z", "2026-10-15T08:00:00Z")
	share.Description, share.Attributes = "Share a document", models.Attributes{"risk": "low"}
	return []models.ResourceActionRead{read, share}
}

// firstActions returns the first n actions that seq yields, and ends the loop there, so that it
// asks for no page past them; or the first error that seq yields.
func firstActions(seq iter.Seq2[models.ResourceActionRead, error], n int,
) ([]models.ResourceActionRead, error) {
	var got []models.ResourceActionRead
	for action, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, action)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// resourceActionCases returns the wire cases of the actions of a resource of an environment.
// Each sends one request to the API, along a route that the runner's scope fills, p and e, as
// WithScope gives it. The keys of the resources and the actions hold characters that the path
// escapes, a slash, a percent sign and a letter outside ASCII, as a/b and 50%é do, and a key has
// a leading dot, .hidden, which the path sends as it is; the attribute filters hold one value of
// each type, and a string and the key of an attribute that the query escapes.
func resourceActionCases() []wireCase {
	const (
		actionsRoute = "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/actions"
		actionRoute  = actionsRoute + "/{action_id}"
		resources    = "/v2/schema/p/e/resources"
	)
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, actionForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-12"
		e.Detail = "the API key may not act on the resources of the environment"
		return e
	}
	listErr := projectError("list_resource_actions", http.MethodGet, actionsRoute,
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	getErr := projectError("get_resource_action", http.MethodGet, actionRoute,
		http.StatusNotFound, actionNotFoundBody)
	getErr.Code, getErr.Title, getErr.RequestID = "NOT_FOUND", "Not Found", "req-13"
	getErr.Detail = "the action does not exist"
	createErr := projectError("create_resource_action", http.MethodPost, actionsRoute,
		http.StatusConflict, actionConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "DUPLICATE_ENTITY", "Conflict", "req-14"
	createErr.Detail = "an action with the key approve exists already"
	updateErr := projectError("update_resource_action", http.MethodPatch, actionRoute,
		http.StatusUnprocessableEntity, actionInvalidBody)
	updateErr.Detail = "body.name: none is not an allowed value"
	return []wireCase{
		{
			Method: "ResourceActionsService.List",
			Op:     "list_resource_actions",
			Route:  "GET " + actionsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceActions.List(ctx, "a/b", &permit.ResourceActionsListOptions{
					ListOptions: permit.ListOptions{Page: 2, PerPage: 10},
					Attributes: []permit.AttributeFilter{
						permit.AttributeEquals("risk level", "low&é"),
						permit.AttributeEquals("audited", true),
						permit.AttributeEquals("level", -3),
						permit.AttributeEquals("score", 0.5),
					},
				})
			},
			Request: wireRequest{
				RawPath: resources + "/a%2Fb/actions",
				RawQuery: "attr_audited=true&attr_level=-3&attr_risk+level=low%26%C3%A9&" +
					"attr_score=0.5&page=2&per_page=10",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_resource_actions.200.json")},
			WantResult: &permit.Page[models.ResourceActionRead]{
				Data: listedActions(), Page: 2, PerPage: 10,
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "ResourceActionsService.All",
			Op:     "list_resource_actions",
			Route:  "GET " + actionsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstActions(c.ResourceActions.All(ctx, "50%é",
					&permit.ResourceActionsListOptions{
						Attributes: []permit.AttributeFilter{permit.AttributeEquals("risk", "low")},
					}), 2)
			},
			Request: wireRequest{
				RawPath:  resources + "/50%25%C3%A9/actions",
				RawQuery: "attr_risk=low&page=1&per_page=100",
			},
			Reply:      mockpermit.Reply{Body: replyFixture("list_resource_actions.200.json")},
			WantResult: listedActions(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: actionForbiddenBody},
			WantErr:    forbidden("list_resource_actions", http.MethodGet, actionsRoute),
		},
		{
			Method: "ResourceActionsService.Get",
			Op:     "get_resource_action",
			Route:  "GET " + actionRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceActions.Get(ctx, ".hidden", "a/b")
			},
			Request:    wireRequest{RawPath: resources + "/.hidden/actions/a%2Fb"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_resource_action.200.json")},
			WantResult: fixtureAction(),
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: actionNotFoundBody},
			WantErr:    getErr,
		},
		{
			Method: "ResourceActionsService.Create",
			Op:     "create_resource_action",
			Route:  "POST " + actionsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceActions.Create(ctx, "a/b", models.ResourceActionCreate{
					Key: "approve", Name: "Approve", Description: "Approve a document",
					Attributes: models.Attributes{"risk": "high"},
				})
			},
			Request: wireRequest{
				RawPath: resources + "/a%2Fb/actions",
				Body: `{"key":"approve","name":"Approve","description":"Approve a document",` +
					`"attributes":{"risk":"high"}}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("create_resource_action.200.json")},
			WantResult: createdAction(),
			ErrorReply: mockpermit.Reply{Status: http.StatusConflict, Body: actionConflictBody},
			WantErr:    createErr,
		},
		{
			Method: "ResourceActionsService.Update",
			Op:     "update_resource_action",
			Route:  "PATCH " + actionRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceActions.Update(ctx, "50%é", ".hidden",
					models.ResourceActionUpdate{
						Name: "Read a document", Description: models.Null[string](),
						Attributes: models.Null[models.Attributes](),
					})
			},
			Request: wireRequest{
				RawPath: resources + "/50%25%C3%A9/actions/.hidden",
				Body:    `{"name":"Read a document","description":null,"attributes":null}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("update_resource_action.200.json")},
			WantResult: updatedAction(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: actionInvalidBody,
			},
			WantErr: updateErr,
		},
		{
			Method: "ResourceActionsService.Delete",
			Op:     "delete_resource_action",
			Route:  "DELETE " + actionRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.ResourceActions.Delete(ctx, "a/b", "50%é")
			},
			Request:    wireRequest{RawPath: resources + "/a%2Fb/actions/50%25%C3%A9"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: actionForbiddenBody},
			WantErr:    forbidden("delete_resource_action", http.MethodDelete, actionRoute),
		},
	}
}

// createdAction is the action of the reply fixture create_resource_action.200.json.
func createdAction() *models.ResourceActionRead {
	a := actionOf("approve", 83, "Approve", "2026-10-15T11:00:00Z", "2026-10-15T11:00:00Z")
	a.Description, a.Attributes = "Approve a document", models.Attributes{"risk": "high"}
	return &a
}

// updatedAction is the action of the reply fixture update_resource_action.200.json, which holds
// null for the description and the attributes that the update cleared, as the API answers.
func updatedAction() *models.ResourceActionRead {
	a := actionOf("read", 81, "Read a document", "2026-10-14T09:30:00Z", "2026-10-15T13:00:00Z")
	return &a
}
