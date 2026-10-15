package permit_test

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the resources: a key that may not act
// on the resources of the environment, a resource that does not exist, a key that another
// resource of the environment has already, and a field that the API rejects.
const (
	resourceForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the resources of the environment","id":"req-9"}`
	resourceNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the resource does not exist","id":"req-10"}`
	resourceConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"a resource with the key report exists already","id":"req-11"}`
	resourceInvalidBody = `{"detail":[{"loc":["body","attributes","owner","type"],` +
		`"msg":"value is not a valid enumeration member","type":"type_error.enum"}]}`
)

// resourceID returns the ID of the reply fixtures of the resources whose last digits are n, such
// as 21 for the resource document.
func resourceID(n int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
}

// resourceOf returns a resource of the reply fixtures, of the environment of the scope fixture,
// with the fields that every resource has.
func resourceOf(key string, id int, name, created, updated string) models.ResourceRead {
	return models.ResourceRead{
		Key: key, ID: resourceID(id), OrganizationID: exampleOrganizationID,
		ProjectID: exampleProjectID, EnvironmentID: exampleEnvironmentID, CreatedAt: at(created),
		UpdatedAt: at(updated), Name: name,
	}
}

// fixtureResource is the resource of the reply fixture get_resource.200.json, with every field
// but the v1compat fields, and a role, an action, an attribute and a relation with every field.
func fixtureResource() *models.ResourceRead {
	r := resourceOf("document", 21, "Document", "2026-10-14T09:30:00Z",
		"2026-10-15T10:00:00.123456")
	r.URN, r.Description = "prn:my-project:production:document", "A document of a folder"
	r.Actions = map[string]models.ActionBlockRead{
		"read": {
			Name: "Read", Description: "Read a document",
			Attributes: models.Attributes{"risk": json.Number("1")}, ID: resourceID(31),
			Key: "read",
		},
		"write": {ID: resourceID(32), Key: "write"},
	}
	r.TypeAttributes = models.Attributes{"limit": json.Number("9007199254740993")}
	r.Attributes = map[string]models.AttributeBlockRead{"owner": {
		Type: models.AttributeTypeString, Description: "Who owns it", ID: resourceID(41),
		Key: "owner",
	}}
	r.Roles = map[string]models.ResourceRoleRead{"editor": {
		Name: "Editor", Description: "Edits documents", Permissions: []string{"read", "write"},
		Attributes: models.Attributes{"level": json.Number("2")}, Extends: []string{"viewer"},
		GrantedTo: &models.DerivedRoleBlockRead{
			ID: resourceID(61),
			UsersWithRole: []models.DerivedRoleRuleRead{{
				RoleID: resourceID(52), ResourceID: resourceID(22), RelationID: resourceID(71),
				Role: "admin", OnResource: "folder", LinkedByRelation: "parent",
				When: &models.DerivationSettings{NoDirectRolesOnObject: true},
			}},
			When: &models.DerivationSettings{},
		},
		Key: "editor", ID: resourceID(51), OrganizationID: exampleOrganizationID,
		ProjectID: exampleProjectID, EnvironmentID: exampleEnvironmentID,
		ResourceID: resourceID(21), Resource: "document", CreatedAt: at("2026-10-14T09:30:00Z"),
		UpdatedAt: at("2026-10-14T09:30:00Z"),
	}}
	r.Relations = map[string]models.RelationBlockRead{"parent": {
		Description: "The folder of the document", ResourceID: resourceID(22), Resource: "folder",
	}}
	r.ActionGroups = map[string][]string{"editing": {"read", "write"}}
	return &r
}

// listedResources are the resources of the reply fixture list_resources.200.json.
func listedResources() []models.ResourceRead {
	document := resourceOf("document", 21, "Document", "2026-10-14T09:30:00Z",
		"2026-10-14T09:30:00Z")
	document.Actions = map[string]models.ActionBlockRead{
		"read": {ID: resourceID(31), Key: "read"},
	}
	folder := resourceOf("folder", 22, "Folder", "2026-10-15T08:00:00Z", "2026-10-15T08:00:00Z")
	folder.Description, folder.Actions = "Holds documents", map[string]models.ActionBlockRead{}
	return []models.ResourceRead{document, folder}
}

// firstResources returns the first n resources that seq yields, and ends the loop there, so that
// it asks for no page past them; or the first error that seq yields.
func firstResources(seq iter.Seq2[models.ResourceRead, error], n int,
) ([]models.ResourceRead, error) {
	var got []models.ResourceRead
	for resource, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, resource)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// resourceCases returns the wire cases of the resources of an environment. Each sends one
// request to the API, along a route that the runner's scope fills, p and e, as WithScope gives
// it. The keys of the resources hold characters that the path escapes, a slash, a percent sign
// and a letter outside ASCII, as a/b and 50%é do, and a key has a leading dot, .hidden, which
// the path sends as it is; the search holds characters that the query escapes.
func resourceCases() []wireCase {
	const (
		resourcesRoute = "/v2/schema/{proj_id}/{env_id}/resources"
		resourceRoute  = resourcesRoute + "/{resource_id}"
		resourcesPath  = "/v2/schema/p/e/resources"
	)
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, resourceForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-9"
		e.Detail = "the API key may not act on the resources of the environment"
		return e
	}
	invalid := func(op, method string) *permit.Error {
		e := projectError(op, method, resourceRoute, http.StatusUnprocessableEntity,
			resourceInvalidBody)
		e.Detail = "body.attributes.owner.type: value is not a valid enumeration member"
		return e
	}
	listErr := projectError("list_resources", http.MethodGet, resourcesRoute,
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	getErr := projectError("get_resource", http.MethodGet, resourceRoute, http.StatusNotFound,
		resourceNotFoundBody)
	getErr.Code, getErr.Title, getErr.RequestID = "NOT_FOUND", "Not Found", "req-10"
	getErr.Detail = "the resource does not exist"
	createErr := projectError("create_resource", http.MethodPost, resourcesRoute,
		http.StatusConflict, resourceConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "DUPLICATE_ENTITY", "Conflict", "req-11"
	createErr.Detail = "a resource with the key report exists already"
	return []wireCase{
		{
			Method: "ResourcesService.List",
			Op:     "list_resources",
			Route:  "GET " + resourcesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Resources.List(ctx, &permit.ResourcesListOptions{
					ListOptions: permit.ListOptions{Page: 2, PerPage: 10}, Search: "my doc&é",
				})
			},
			Request: wireRequest{
				RawPath: resourcesPath, RawQuery: "page=2&per_page=10&search=my+doc%26%C3%A9",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_resources.200.json")},
			WantResult: &permit.Page[models.ResourceRead]{
				Data: listedResources(), Page: 2, PerPage: 10,
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "ResourcesService.All",
			Op:     "list_resources",
			Route:  "GET " + resourcesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstResources(c.Resources.All(ctx,
					&permit.ResourcesListOptions{Search: "doc"}), 2)
			},
			Request: wireRequest{
				RawPath: resourcesPath, RawQuery: "page=1&per_page=100&search=doc",
			},
			Reply:      mockpermit.Reply{Body: replyFixture("list_resources.200.json")},
			WantResult: listedResources(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: resourceForbiddenBody},
			WantErr:    forbidden("list_resources", http.MethodGet, resourcesRoute),
		},
		{
			Method: "ResourcesService.Get",
			Op:     "get_resource",
			Route:  "GET " + resourceRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Resources.Get(ctx, "a/b")
			},
			Request:    wireRequest{RawPath: resourcesPath + "/a%2Fb"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_resource.200.json")},
			WantResult: fixtureResource(),
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: resourceNotFoundBody},
			WantErr:    getErr,
		},
		{
			Method: "ResourcesService.Create",
			Op:     "create_resource",
			Route:  "POST " + resourcesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Resources.Create(ctx, newResource())
			},
			Request:    wireRequest{RawPath: resourcesPath, Body: newResourceBody},
			Reply:      mockpermit.Reply{Body: replyFixture("create_resource.200.json")},
			WantResult: createdResource(),
			ErrorReply: mockpermit.Reply{Status: http.StatusConflict, Body: resourceConflictBody},
			WantErr:    createErr,
		},
		{
			Method: "ResourcesService.Replace",
			Op:     "replace_resource",
			Route:  "PUT " + resourceRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Resources.Replace(ctx, "50%é", replacement())
			},
			Request: wireRequest{
				RawPath: resourcesPath + "/50%25%C3%A9", Body: replacementBody,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("replace_resource.200.json")},
			WantResult: replacedResource(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: resourceInvalidBody,
			},
			WantErr: invalid("replace_resource", http.MethodPut),
		},
		{
			Method: "ResourcesService.Update",
			Op:     "update_resource",
			Route:  "PATCH " + resourceRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Resources.Update(ctx, ".hidden", models.ResourceUpdate{
					Name: "Renamed document", Description: models.Null[string](),
					Actions:        map[string]models.ActionBlockEditable{},
					TypeAttributes: models.Attributes{"limit": 10},
				})
			},
			Request: wireRequest{
				RawPath: resourcesPath + "/.hidden",
				Body: `{"name":"Renamed document","description":null,"actions":{},` +
					`"type_attributes":{"limit":10}}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("update_resource.200.json")},
			WantResult: updatedResource(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: resourceInvalidBody,
			},
			WantErr: invalid("update_resource", http.MethodPatch),
		},
		{
			Method: "ResourcesService.Delete",
			Op:     "delete_resource",
			Route:  "DELETE " + resourceRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Resources.Delete(ctx, "a/b")
			},
			Request:    wireRequest{RawPath: resourcesPath + "/a%2Fb"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: resourceForbiddenBody},
			WantErr:    forbidden("delete_resource", http.MethodDelete, resourceRoute),
		},
	}
}

// newResource is the resource that the wire case of ResourcesService.Create creates, with every
// field of the request, and an action, an attribute and a role with every field of their own.
func newResource() models.ResourceCreate {
	return models.ResourceCreate{
		Key: "report", Name: "Report", URN: "prn:my-project:production:report",
		Description: "A report",
		Actions: map[string]models.ActionBlockEditable{
			"read": {
				Name: "Read", Description: "Read a report", Attributes: models.Attributes{},
			},
		},
		TypeAttributes: models.Attributes{"limit": 10},
		Attributes: map[string]models.AttributeBlockEditable{
			"owner": {Type: models.AttributeTypeString, Description: "Who owns it"},
		},
		Roles: map[string]models.RoleBlockEditable{"reader": {
			Name: "Reader", Description: "Reads reports", Permissions: []string{"read"},
			Attributes: models.Attributes{"level": 1}, Extends: []string{},
			GrantedTo: &models.DerivedRoleBlockEdit{
				UsersWithRole: []models.DerivedRoleRuleCreate{{
					Role: "admin", OnResource: "folder", LinkedByRelation: "parent",
					When: &models.DerivationSettings{NoDirectRolesOnObject: true},
				}},
				When: &models.DerivationSettings{},
			},
		}},
		Relations: map[string]string{"parent": "folder"},
	}
}

// newResourceBody is the body that the call of ResourcesService.Create of newResource sends.
const newResourceBody = `{"key":"report","name":"Report",` +
	`"urn":"prn:my-project:production:report","description":"A report",` +
	`"actions":{"read":{"name":"Read","description":"Read a report","attributes":{}}},` +
	`"type_attributes":{"limit":10},` +
	`"attributes":{"owner":{"type":"string","description":"Who owns it"}},` +
	`"roles":{"reader":{"name":"Reader","description":"Reads reports","permissions":["read"],` +
	`"attributes":{"level":1},"extends":[],"granted_to":{"users_with_role":[{"role":"admin",` +
	`"on_resource":"folder","linked_by_relation":"parent",` +
	`"when":{"no_direct_roles_on_object":true}}],"when":{}}}},` +
	`"relations":{"parent":"folder"}}`

// createdResource is the resource of the reply fixture create_resource.200.json.
func createdResource() *models.ResourceRead {
	r := resourceOf("report", 23, "Report", "2026-10-15T11:00:00Z", "2026-10-15T11:00:00Z")
	r.Actions = map[string]models.ActionBlockRead{
		"read": {Name: "Read", ID: resourceID(33), Key: "read"},
	}
	r.Attributes = map[string]models.AttributeBlockRead{
		"owner": {Type: models.AttributeTypeString, ID: resourceID(42), Key: "owner"},
	}
	return &r
}

// replacement is the definition that the wire case of ResourcesService.Replace sends: its name,
// its description and two actions, one of them new.
func replacement() models.ResourceReplace {
	return models.ResourceReplace{
		Name: "Document", Description: "Replaced",
		Actions: map[string]models.ActionBlockEditable{"read": {}, "archive": {Name: "Archive"}},
	}
}

// replacementBody is the body that the call of ResourcesService.Replace of replacement sends.
const replacementBody = `{"name":"Document","description":"Replaced",` +
	`"actions":{"archive":{"name":"Archive"},"read":{}}}`

// replacedResource is the resource of the reply fixture replace_resource.200.json.
func replacedResource() *models.ResourceRead {
	r := resourceOf("document", 21, "Document", "2026-10-14T09:30:00Z", "2026-10-15T12:00:00Z")
	r.Description = "Replaced"
	r.Actions = map[string]models.ActionBlockRead{
		"read":    {ID: resourceID(31), Key: "read"},
		"archive": {ID: resourceID(34), Key: "archive"},
	}
	return &r
}

// updatedResource is the resource of the reply fixture update_resource.200.json.
func updatedResource() *models.ResourceRead {
	r := resourceOf("document", 21, "Renamed document", "2026-10-14T09:30:00Z",
		"2026-10-15T13:00:00Z")
	r.Actions = map[string]models.ActionBlockRead{}
	r.TypeAttributes = models.Attributes{"limit": json.Number("10")}
	return &r
}
