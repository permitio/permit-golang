package permit_test

import (
	"context"
	"iter"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the relations: a key that may not act
// on the resources of the environment, a relation that does not exist, and a key that another
// relation of the resource has already.
const (
	relationForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the resources of the environment","id":"req-9"}`
	relationNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the relation does not exist","id":"req-10"}`
	relationConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"a relation with the key team exists already","id":"req-11"}`
)

// relationOf returns a relation of the reply fixtures, of the resource document of the
// environment of the scope fixture, with the fields that every relation has: its key, its name,
// the key of its subject, ids, the last digits of its ID and of its subject's, as resourceID
// takes them, and times, when it was created and last changed.
func relationOf(key, name, subject string, ids [2]int, times [2]string) models.RelationRead {
	return models.RelationRead{
		SubjectResource: subject, Key: key, Name: name, ID: resourceID(ids[0]),
		OrganizationID: exampleOrganizationID, ProjectID: exampleProjectID,
		EnvironmentID: exampleEnvironmentID, CreatedAt: at(times[0]), UpdatedAt: at(times[1]),
		ObjectResourceID: resourceID(21), ObjectResource: "document",
		SubjectResourceID: resourceID(ids[1]),
	}
}

// fixtureRelation is the relation of the reply fixture get_resource_relation.200.json, with every
// field.
func fixtureRelation() *models.RelationRead {
	r := relationOf("parent", "Parent", "folder", [2]int{71, 22},
		[2]string{"2026-10-14T09:30:00Z", "2026-10-15T10:00:00.123456"})
	r.Description = "The folder of the document"
	return &r
}

// listedRelations are the relations of the reply fixture list_resource_relations.200.json: one
// with a description, and one without.
func listedRelations() []models.RelationRead {
	parent := relationOf("parent", "Parent", "folder", [2]int{71, 22},
		[2]string{"2026-10-14T09:30:00Z", "2026-10-14T09:30:00Z"})
	parent.Description = "The folder of the document"
	return []models.RelationRead{
		parent,
		relationOf("workspace", "Workspace", "workspace", [2]int{72, 23},
			[2]string{"2026-10-15T08:00:00Z", "2026-10-15T08:00:00Z"}),
	}
}

// createdRelation is the relation of the reply fixture create_resource_relation.200.json.
func createdRelation() *models.RelationRead {
	r := relationOf("team", "Team", "team", [2]int{73, 24},
		[2]string{"2026-10-15T11:00:00Z", "2026-10-15T11:00:00Z"})
	r.Description = "The team of the document"
	return &r
}

// firstRelations returns the first n relations that seq yields, and ends the loop there, so that
// it asks for no page past them; or the first error that seq yields.
func firstRelations(seq iter.Seq2[models.RelationRead, error], n int,
) ([]models.RelationRead, error) {
	var got []models.RelationRead
	for relation, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, relation)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// relationCases returns the wire cases of the relations of a resource. Each sends one request to
// the API, along a route that the runner's scope fills, p and e, as WithScope gives it. The keys
// of the resources and of the relations hold characters that the path escapes, a slash, a percent
// sign and a letter outside ASCII, as a/b and 50%é do; a key has a leading dot, .hidden, and
// another the shape of a UUID, which the path sends as they are. The list's reply is the
// paginated object, whose counts the page holds.
func relationCases() []wireCase {
	const (
		relationsRoute = "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/relations"
		relationRoute  = relationsRoute + "/{relation_id}"
		document       = "/v2/schema/p/e/resources/document/relations"
	)
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, relationForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-9"
		e.Detail = "the API key may not act on the resources of the environment"
		return e
	}
	listErr := projectError("list_resource_relations", http.MethodGet, relationsRoute,
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	getErr := projectError("get_resource_relation", http.MethodGet, relationRoute,
		http.StatusNotFound, relationNotFoundBody)
	getErr.Code, getErr.Title, getErr.RequestID = "NOT_FOUND", "Not Found", "req-10"
	getErr.Detail = "the relation does not exist"
	createErr := projectError("create_resource_relation", http.MethodPost, relationsRoute,
		http.StatusConflict, relationConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "DUPLICATE_ENTITY", "Conflict", "req-11"
	createErr.Detail = "a relation with the key team exists already"
	return []wireCase{
		{
			Method: "ResourceRelationsService.List",
			Op:     "list_resource_relations",
			Route:  "GET " + relationsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceRelations.List(ctx, "a/b",
					&permit.ListOptions{Page: 1, PerPage: 10})
			},
			Request: wireRequest{
				RawPath:  "/v2/schema/p/e/resources/a%2Fb/relations",
				RawQuery: "page=1&per_page=10",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_resource_relations.200.json")},
			WantResult: &permit.Page[models.RelationRead]{
				Data: listedRelations(), Page: 1, PerPage: 10, TotalCount: new(2),
				PageCount: new(2),
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "ResourceRelationsService.All",
			Op:     "list_resource_relations",
			Route:  "GET " + relationsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstRelations(c.ResourceRelations.All(ctx, resourceID(21), nil), 2)
			},
			Request: wireRequest{
				RawPath:  "/v2/schema/p/e/resources/" + resourceID(21) + "/relations",
				RawQuery: "page=1&per_page=100",
			},
			Reply:      mockpermit.Reply{Body: replyFixture("list_resource_relations.200.json")},
			WantResult: listedRelations(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: relationForbiddenBody},
			WantErr:    forbidden("list_resource_relations", http.MethodGet, relationsRoute),
		},
		{
			Method: "ResourceRelationsService.Get",
			Op:     "get_resource_relation",
			Route:  "GET " + relationRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceRelations.Get(ctx, "document", "50%é")
			},
			Request:    wireRequest{RawPath: document + "/50%25%C3%A9"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_resource_relation.200.json")},
			WantResult: fixtureRelation(),
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: relationNotFoundBody},
			WantErr:    getErr,
		},
		{
			Method: "ResourceRelationsService.Create",
			Op:     "create_resource_relation",
			Route:  "POST " + relationsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ResourceRelations.Create(ctx, ".hidden", models.RelationCreate{
					Key: "team", Name: "Team", Description: "The team of the document",
					SubjectResource: "team",
				})
			},
			Request: wireRequest{
				RawPath: "/v2/schema/p/e/resources/.hidden/relations",
				Body: `{"key":"team","name":"Team","description":"The team of the document",` +
					`"subject_resource":"team"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("create_resource_relation.200.json")},
			WantResult: createdRelation(),
			ErrorReply: mockpermit.Reply{Status: http.StatusConflict, Body: relationConflictBody},
			WantErr:    createErr,
		},
		{
			Method: "ResourceRelationsService.Delete",
			Op:     "delete_resource_relation",
			Route:  "DELETE " + relationRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.ResourceRelations.Delete(ctx, "document", ".hidden")
			},
			Request:    wireRequest{RawPath: document + "/.hidden"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: relationForbiddenBody},
			WantErr:    forbidden("delete_resource_relation", http.MethodDelete, relationRoute),
		},
	}
}
