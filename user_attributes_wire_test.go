package permit_test

import (
	"context"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// userAttributeConflictBody is the error body of the Permit API in the wire case of the create of
// an attribute of the users whose key another attribute of the users has already. The other
// cases reuse the error bodies of the attributes of a resource.
const userAttributeConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
	`"message":"an attribute with the key address exists already","id":"req-18"}`

// userAttributeOf returns an attribute of the users of the reply fixtures, of the environment of
// the scope fixture, with the fields that every attribute has: the users' attributes are those
// of the resource __user.
func userAttributeOf(key string, id int, typ models.AttributeType, created, updated string,
) models.ResourceAttributeRead {
	return models.ResourceAttributeRead{
		Type: typ, Key: key, ID: resourceID(id), ResourceID: resourceID(100),
		ResourceKey: "__user", OrganizationID: exampleOrganizationID,
		ProjectID: exampleProjectID, EnvironmentID: exampleEnvironmentID, CreatedAt: at(created),
		UpdatedAt: at(updated),
	}
}

// listedUserAttributes are the attributes of the reply fixture list_user_attributes.200.json,
// one of each type that the SDK knows and a second string, the first of them built in.
func listedUserAttributes() []models.ResourceAttributeRead {
	const first, second = "2026-10-14T09:30:00Z", "2026-10-15T08:00:00Z"
	email := userAttributeOf("email", 101, models.AttributeTypeString, first, first)
	email.BuiltIn = true
	department := userAttributeOf("department", 103, models.AttributeTypeString, first, first)
	department.Description = "Where the user works"
	address := userAttributeOf("address", 108, models.AttributeTypeObject, second, second)
	address.Description = "Where the user lives"
	return []models.ResourceAttributeRead{
		email,
		userAttributeOf("age", 102, models.AttributeTypeNumber, first, first),
		department,
		userAttributeOf("verified", 104, models.AttributeTypeBool, first, first),
		userAttributeOf("joined", 105, models.AttributeTypeTime, first, first),
		userAttributeOf("teams", 106, models.AttributeTypeArray, first, first),
		userAttributeOf("preferences", 107, models.AttributeTypeJSON, second, second),
		address,
		userAttributeOf("contacts", 109, models.AttributeTypeObjectArray, second, second),
	}
}

// fixtureUserAttribute is the attribute of the reply fixture get_user_attribute.200.json, of the
// type object_array.
func fixtureUserAttribute() *models.ResourceAttributeRead {
	a := userAttributeOf("contacts", 109, models.AttributeTypeObjectArray, "2026-10-15T08:00:00Z",
		"2026-10-15T10:00:00.123456")
	a.Description = "How to reach the user"
	return &a
}

// createdUserAttribute is the attribute of the reply fixture create_user_attribute.200.json.
func createdUserAttribute() *models.ResourceAttributeRead {
	a := userAttributeOf("address", 108, models.AttributeTypeObject, "2026-10-15T08:00:00Z",
		"2026-10-15T08:00:00Z")
	a.Description = "Where the user lives"
	return &a
}

// updatedUserAttribute is the attribute of the reply fixture update_user_attribute.200.json,
// without the description that the update cleared.
func updatedUserAttribute() *models.ResourceAttributeRead {
	a := userAttributeOf("address", 108, models.AttributeTypeObjectArray, "2026-10-15T08:00:00Z",
		"2026-10-15T13:00:00Z")
	return &a
}

// userAttributeCases returns the wire cases of the attributes of the users of an environment.
// Each sends one request to the API, along a route that the runner's scope fills, p and e, as
// WithScope gives it, and none sends resource_id, which the API's documentation lists on each
// route: the query of each case is the whole query that its call sends. The keys of the attributes
// hold characters that the path escapes, a slash, a percent sign and a letter outside ASCII, as
// a/b and 50%é do, and a key has a leading dot, .hidden, which the path sends as it is. The
// replies of List and All hold an attribute of each type that the SDK knows.
func userAttributeCases() []wireCase {
	const (
		attributesRoute = "/v2/schema/{proj_id}/{env_id}/users/attributes"
		attributeRoute  = attributesRoute + "/{attribute_id}"
		attributes      = "/v2/schema/p/e/users/attributes"
	)
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, attributeForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-15"
		e.Detail = "the API key may not act on the resources of the environment"
		return e
	}
	listErr := projectError("list_user_attributes", http.MethodGet, attributesRoute,
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	getErr := projectError("get_user_attribute", http.MethodGet, attributeRoute,
		http.StatusNotFound, attributeNotFoundBody)
	getErr.Code, getErr.Title, getErr.RequestID = "NOT_FOUND", "Not Found", "req-16"
	getErr.Detail = "the attribute does not exist"
	createErr := projectError("create_user_attribute", http.MethodPost, attributesRoute,
		http.StatusConflict, userAttributeConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "DUPLICATE_ENTITY", "Conflict",
		"req-18"
	createErr.Detail = "an attribute with the key address exists already"
	updateErr := projectError("update_user_attribute", http.MethodPatch, attributeRoute,
		http.StatusUnprocessableEntity, attributeInvalidBody)
	updateErr.Detail = "body.type: value is not a valid enumeration member"
	return []wireCase{
		{
			Method: "UserAttributesService.List",
			Op:     "list_user_attributes",
			Route:  "GET " + attributesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.UserAttributes.List(ctx, &permit.ListOptions{Page: 2, PerPage: 10})
			},
			Request: wireRequest{RawPath: attributes, RawQuery: "page=2&per_page=10"},
			Reply:   mockpermit.Reply{Body: replyFixture("list_user_attributes.200.json")},
			WantResult: &permit.Page[models.ResourceAttributeRead]{
				Data: listedUserAttributes(), Page: 2, PerPage: 10,
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "UserAttributesService.All",
			Op:     "list_user_attributes",
			Route:  "GET " + attributesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstAttributes(c.UserAttributes.All(ctx, nil), 9)
			},
			Request:    wireRequest{RawPath: attributes, RawQuery: "page=1&per_page=100"},
			Reply:      mockpermit.Reply{Body: replyFixture("list_user_attributes.200.json")},
			WantResult: listedUserAttributes(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden,
				Body:   attributeForbiddenBody,
			},
			WantErr: forbidden("list_user_attributes", http.MethodGet, attributesRoute),
		},
		{
			Method: "UserAttributesService.Get",
			Op:     "get_user_attribute",
			Route:  "GET " + attributeRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.UserAttributes.Get(ctx, "a/b")
			},
			Request:    wireRequest{RawPath: attributes + "/a%2Fb"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_user_attribute.200.json")},
			WantResult: fixtureUserAttribute(),
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: attributeNotFoundBody},
			WantErr:    getErr,
		},
		{
			Method: "UserAttributesService.Create",
			Op:     "create_user_attribute",
			Route:  "POST " + attributesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.UserAttributes.Create(ctx, models.ResourceAttributeCreate{
					Key: "address", Type: models.AttributeTypeObject,
					Description: "Where the user lives",
				})
			},
			Request: wireRequest{
				RawPath: attributes,
				Body:    `{"key":"address","type":"object","description":"Where the user lives"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("create_user_attribute.200.json")},
			WantResult: createdUserAttribute(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusConflict, Body: userAttributeConflictBody,
			},
			WantErr: createErr,
		},
		{
			Method: "UserAttributesService.Update",
			Op:     "update_user_attribute",
			Route:  "PATCH " + attributeRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.UserAttributes.Update(ctx, ".hidden", models.ResourceAttributeUpdate{
					Type: models.AttributeTypeObjectArray, Description: models.Null[string](),
				})
			},
			Request: wireRequest{
				RawPath: attributes + "/.hidden",
				Body:    `{"type":"object_array","description":null}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("update_user_attribute.200.json")},
			WantResult: updatedUserAttribute(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: attributeInvalidBody,
			},
			WantErr: updateErr,
		},
		{
			Method: "UserAttributesService.Delete",
			Op:     "delete_user_attribute",
			Route:  "DELETE " + attributeRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.UserAttributes.Delete(ctx, "50%é")
			},
			Request:    wireRequest{RawPath: attributes + "/50%25%C3%A9"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden,
				Body:   attributeForbiddenBody,
			},
			WantErr: forbidden("delete_user_attribute", http.MethodDelete, attributeRoute),
		},
	}
}
