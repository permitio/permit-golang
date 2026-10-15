package permit_test

import (
	"context"
	"iter"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the attributes of a resource: a key
// that may not act on the resources of the environment, an attribute that does not exist, a key
// that another attribute of the resource has already, and a type that the API rejects.
const (
	attributeForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the resources of the environment","id":"req-15"}`
	attributeNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the attribute does not exist","id":"req-16"}`
	attributeConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"an attribute with the key address exists already","id":"req-17"}`
	attributeInvalidBody = `{"detail":[{"loc":["body","type"],` +
		`"msg":"value is not a valid enumeration member","type":"type_error.enum"}]}`
)

// attributeOf returns an attribute of the resource document of the reply fixtures, of the
// environment of the scope fixture, with the fields that every attribute has.
func attributeOf(key string, id int, typ models.AttributeType, created, updated string,
) models.ResourceAttributeRead {
	return models.ResourceAttributeRead{
		Type: typ, Key: key, ID: resourceID(id), ResourceID: resourceID(21),
		ResourceKey: "document", OrganizationID: exampleOrganizationID,
		ProjectID: exampleProjectID, EnvironmentID: exampleEnvironmentID, CreatedAt: at(created),
		UpdatedAt: at(updated),
	}
}

// listedAttributes are the attributes of the reply fixture list_resource_attributes.200.json, one
// of each type that the SDK knows, the first of them built in.
func listedAttributes() []models.ResourceAttributeRead {
	const first, second = "2026-10-14T09:30:00Z", "2026-10-15T08:00:00Z"
	archived := attributeOf("archived", 91, models.AttributeTypeBool, first, first)
	archived.BuiltIn = true
	owner := attributeOf("owner", 93, models.AttributeTypeString, first, first)
	owner.Description = "Who owns it"
	address := attributeOf("address", 97, models.AttributeTypeObject, second, second)
	address.Description = "Where it is kept"
	return []models.ResourceAttributeRead{
		archived,
		attributeOf("pages", 92, models.AttributeTypeNumber, first, first),
		owner,
		attributeOf("due", 94, models.AttributeTypeTime, first, first),
		attributeOf("tags", 95, models.AttributeTypeArray, first, first),
		attributeOf("settings", 96, models.AttributeTypeJSON, second, second),
		address,
		attributeOf("contacts", 98, models.AttributeTypeObjectArray, second, second),
	}
}

// fixtureAttribute is the attribute of the reply fixture get_resource_attribute.200.json.
func fixtureAttribute() *models.ResourceAttributeRead {
	a := attributeOf("owner", 93, models.AttributeTypeString, "2026-10-14T09:30:00Z",
		"2026-10-15T10:00:00.123456")
	a.Description = "Who owns it"
	return &a
}

// firstAttributes returns the first n attributes that seq yields, and ends the loop there, so
// that it asks for no page past them; or the first error that seq yields.
func firstAttributes(seq iter.Seq2[models.ResourceAttributeRead, error], n int,
) ([]models.ResourceAttributeRead, error) {
	var got []models.ResourceAttributeRead
	for attribute, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, attribute)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// resourceAttributeCases returns the wire cases of the attributes of a resource of an
// environment. Each sends one request to the API, along a route that the runner's scope fills, p
// and e, as WithScope gives it. The keys of the resources and the attributes hold characters that
// the path escapes, a slash, a percent sign and a letter outside ASCII, as a/b and 50%é do, and a
// key has a leading dot, .hidden, which the path sends as it is. The replies of List, All and Get
// hold attributes of each type that the SDK knows.
func resourceAttributeCases() []wireCase {
	const (
		attributesRoute = "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/attributes"
		attributeRoute  = attributesRoute + "/{attribute_id}"
		resources       = "/v2/schema/p/e/resources"
	)
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, attributeForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-15"
		e.Detail = "the API key may not act on the resources of the environment"
		return e
	}
	listErr := projectError("list_resource_attributes", http.MethodGet, attributesRoute,
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	getErr := projectError("get_resource_attribute", http.MethodGet, attributeRoute,
		http.StatusNotFound, attributeNotFoundBody)
	getErr.Code, getErr.Title, getErr.RequestID = "NOT_FOUND", "Not Found", "req-16"
	getErr.Detail = "the attribute does not exist"
	createErr := projectError("create_resource_attribute", http.MethodPost, attributesRoute,
		http.StatusConflict, attributeConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "DUPLICATE_ENTITY", "Conflict",
		"req-17"
	createErr.Detail = "an attribute with the key address exists already"
	updateErr := projectError("update_resource_attribute", http.MethodPatch, attributeRoute,
		http.StatusUnprocessableEntity, attributeInvalidBody)
	updateErr.Detail = "body.type: value is not a valid enumeration member"
	return []wireCase{
		{
			Method: "ResourceAttributesService.List",
			Op:     "list_resource_attributes",
			Route:  "GET " + attributesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceAttributes.List(ctx, "a/b",
					&permit.ListOptions{Page: 2, PerPage: 10})
			},
			Request: wireRequest{
				RawPath: resources + "/a%2Fb/attributes", RawQuery: "page=2&per_page=10",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_resource_attributes.200.json")},
			WantResult: &permit.Page[models.ResourceAttributeRead]{
				Data: listedAttributes(), Page: 2, PerPage: 10,
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "ResourceAttributesService.All",
			Op:     "list_resource_attributes",
			Route:  "GET " + attributesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstAttributes(c.ResourceAttributes.All(ctx, "50%é", nil), 8)
			},
			Request: wireRequest{
				RawPath: resources + "/50%25%C3%A9/attributes", RawQuery: "page=1&per_page=100",
			},
			Reply:      mockpermit.Reply{Body: replyFixture("list_resource_attributes.200.json")},
			WantResult: listedAttributes(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden,
				Body:   attributeForbiddenBody,
			},
			WantErr: forbidden("list_resource_attributes", http.MethodGet, attributesRoute),
		},
		{
			Method: "ResourceAttributesService.Get",
			Op:     "get_resource_attribute",
			Route:  "GET " + attributeRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceAttributes.Get(ctx, ".hidden", "a/b")
			},
			Request:    wireRequest{RawPath: resources + "/.hidden/attributes/a%2Fb"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_resource_attribute.200.json")},
			WantResult: fixtureAttribute(),
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: attributeNotFoundBody},
			WantErr:    getErr,
		},
		{
			Method: "ResourceAttributesService.Create",
			Op:     "create_resource_attribute",
			Route:  "POST " + attributesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceAttributes.Create(ctx, "a/b", models.ResourceAttributeCreate{
					Key: "address", Type: models.AttributeTypeObject,
					Description: "Where it is kept",
				})
			},
			Request: wireRequest{
				RawPath: resources + "/a%2Fb/attributes",
				Body:    `{"key":"address","type":"object","description":"Where it is kept"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("create_resource_attribute.200.json")},
			WantResult: createdAttribute(),
			ErrorReply: mockpermit.Reply{Status: http.StatusConflict, Body: attributeConflictBody},
			WantErr:    createErr,
		},
		{
			Method: "ResourceAttributesService.Update",
			Op:     "update_resource_attribute",
			Route:  "PATCH " + attributeRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceAttributes.Update(ctx, "50%é", ".hidden",
					models.ResourceAttributeUpdate{
						Type: models.AttributeTypeObjectArray, Description: models.Null[string](),
					})
			},
			Request: wireRequest{
				RawPath: resources + "/50%25%C3%A9/attributes/.hidden",
				Body:    `{"type":"object_array","description":null}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("update_resource_attribute.200.json")},
			WantResult: updatedAttribute(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: attributeInvalidBody,
			},
			WantErr: updateErr,
		},
		{
			Method: "ResourceAttributesService.Delete",
			Op:     "delete_resource_attribute",
			Route:  "DELETE " + attributeRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.ResourceAttributes.Delete(ctx, "a/b", "50%é")
			},
			Request:    wireRequest{RawPath: resources + "/a%2Fb/attributes/50%25%C3%A9"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden,
				Body:   attributeForbiddenBody,
			},
			WantErr: forbidden("delete_resource_attribute", http.MethodDelete, attributeRoute),
		},
	}
}

// createdAttribute is the attribute of the reply fixture create_resource_attribute.200.json.
func createdAttribute() *models.ResourceAttributeRead {
	a := attributeOf("address", 97, models.AttributeTypeObject, "2026-10-15T08:00:00Z",
		"2026-10-15T08:00:00Z")
	a.Description = "Where it is kept"
	return &a
}

// updatedAttribute is the attribute of the reply fixture update_resource_attribute.200.json,
// which holds null for the description that the update cleared, as the API answers.
func updatedAttribute() *models.ResourceAttributeRead {
	a := attributeOf("address", 97, models.AttributeTypeObjectArray, "2026-10-15T08:00:00Z",
		"2026-10-15T13:00:00Z")
	return &a
}
