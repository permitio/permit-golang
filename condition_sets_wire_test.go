package permit_test

import (
	"context"
	"encoding/json"
	"iter"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the condition sets: a key that may not
// act on the condition sets of the environment, a set that does not exist, a key that another set
// of the environment has already, and a field that the API rejects.
const (
	conditionSetForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the condition sets of the environment",` +
		`"id":"req-31"}`
	conditionSetNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the condition set does not exist","id":"req-32"}`
	conditionSetConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"a condition set with the key seniors exists already","id":"req-33"}`
	conditionSetInvalidBody = `{"detail":[{"loc":["body","conditions"],` +
		`"msg":"value is not a valid dict","type":"type_error.dict"}]}`
)

// condition returns the condition that compares the attribute with value by operator, such as
// {"user.age": {"greater-than": 18}}.
func condition(attribute, operator string, value any) map[string]any {
	return map[string]any{attribute: map[string]any{operator: value}}
}

// allOf returns the conditions of a condition set that hold when each of conditions holds, nested
// as the API nests them: an allOf of one allOf of the conditions.
func allOf(conditions ...map[string]any) models.Attributes {
	inner := make([]any, 0, len(conditions))
	for _, c := range conditions {
		inner = append(inner, c)
	}
	return models.Attributes{"allOf": []any{map[string]any{"allOf": inner}}}
}

// conditionSetOf returns a condition set of the reply fixtures, of the environment of the scope
// fixture, with the fields that every set has.
func conditionSetOf(key string, id int, name string, typ models.ConditionSetType, created,
	updated string,
) models.ConditionSetRead {
	return models.ConditionSetRead{
		Key: key, Type: typ, ID: resourceID(id), OrganizationID: exampleOrganizationID,
		ProjectID: exampleProjectID, EnvironmentID: exampleEnvironmentID, CreatedAt: at(created),
		UpdatedAt: at(updated), Name: name,
	}
}

// The condition sets of the reply fixtures: the user sets adults and seniors, whose parent is
// adults, the resource set public_docs of the resource document, and the user set that the API
// created itself for the role member.
func adultsSet() models.ConditionSetRead {
	s := conditionSetOf("adults", 201, "Adults", models.ConditionSetTypeUserSet,
		"2026-10-14T09:30:00Z", "2026-10-14T09:30:00Z")
	s.Description = "Users older than 18"
	s.Conditions = allOf(condition("user.age", "greater-than", json.Number("18")))
	return s
}

// seniorsSet is the user set seniors of the reply fixtures, whose parent is adults.
func seniorsSet() models.ConditionSetRead {
	s := conditionSetOf("seniors", 202, "Seniors", models.ConditionSetTypeUserSet,
		"2026-10-14T09:30:00Z", "2026-10-14T09:30:00Z")
	s.Conditions = allOf(condition("user.age", "greater-than", json.Number("65")))
	s.ParentID = resourceID(201)
	return s
}

// publicDocsSet is the resource set public_docs of the reply fixtures, last changed at updated,
// with its resource, document, and conditions whose number is beyond 2^53.
func publicDocsSet(updated string) models.ConditionSetRead {
	s := conditionSetOf("public_docs", 203, "Public documents",
		models.ConditionSetTypeResourceSet, "2026-10-15T08:00:00Z", updated)
	s.ResourceID = resourceID(21)
	document := resourceOf("document", 21, "Document", "2026-10-14T09:30:00Z",
		"2026-10-14T09:30:00Z")
	s.Resource = &document
	s.Conditions = allOf(condition("resource.private", "equals", false),
		condition("resource.size", "less-than", json.Number("9007199254740993")))
	return s
}

// listedConditionSets are the condition sets of the reply fixtures list_condition_sets.200.json
// and list_condition_sets.200.page.json.
func listedConditionSets() []models.ConditionSetRead {
	return []models.ConditionSetRead{
		adultsSet(), seniorsSet(), publicDocsSet("2026-10-15T08:00:00Z"),
	}
}

// firstConditionSets returns the first n condition sets that seq yields, and ends the loop
// there, so that it asks for no page past them; or the first error that seq yields.
func firstConditionSets(seq iter.Seq2[models.ConditionSetRead, error], n int,
) ([]models.ConditionSetRead, error) {
	var got []models.ConditionSetRead
	for set, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, set)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// conditionSetCases returns the wire cases of the condition sets of an environment. Each sends
// one request to the API, along a route that the runner's scope fills, p and e, as WithScope
// gives it. The keys of the sets hold characters that the path escapes, a slash, a percent sign
// and a letter outside ASCII, as a/b and 50%é do, a key has a leading dot, .hidden, which the
// path sends as it is, and one has the shape of a UUID; the search holds characters that the
// query escapes. The reply of List is the paginated object, and that of All a JSON array.
func conditionSetCases() []wireCase {
	const (
		setsRoute        = "/v2/schema/{proj_id}/{env_id}/condition_sets"
		setRoute         = setsRoute + "/{condition_set_id}"
		ancestorsRoute   = setRoute + "/ancestors"
		descendantsRoute = setRoute + "/descendants"
		sets             = "/v2/schema/p/e/condition_sets"
	)
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, conditionSetForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-31"
		e.Detail = "the API key may not act on the condition sets of the environment"
		return e
	}
	notFound := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusNotFound, conditionSetNotFoundBody)
		e.Code, e.Title, e.RequestID = "NOT_FOUND", "Not Found", "req-32"
		e.Detail = "the condition set does not exist"
		return e
	}
	listErr := projectError("list_condition_sets", http.MethodGet, setsRoute,
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	createErr := projectError("create_condition_set", http.MethodPost, setsRoute,
		http.StatusConflict, conditionSetConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "DUPLICATE_ENTITY", "Conflict",
		"req-33"
	createErr.Detail = "a condition set with the key seniors exists already"
	updateErr := projectError("update_condition_set", http.MethodPatch, setRoute,
		http.StatusUnprocessableEntity, conditionSetInvalidBody)
	updateErr.Detail = "body.conditions: value is not a valid dict"
	created := seniorsSet()
	created.Description = "Users older than 65"
	updated := conditionSetOf("adults", 201, "Grown-ups", models.ConditionSetTypeUserSet,
		"2026-10-14T09:30:00Z", "2026-10-15T13:00:00Z")
	updated.Description, updated.Conditions = "Users older than 18", models.Attributes{}
	member := conditionSetOf("__autogen_member", 204, "member", models.ConditionSetTypeUserSet,
		"2026-10-15T08:00:00Z", "2026-10-15T08:00:00Z")
	member.Autogenerated, member.Conditions = true, models.Attributes{}
	return []wireCase{
		{
			Method: "ConditionSetsService.List",
			Op:     "list_condition_sets",
			Route:  "GET " + setsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ConditionSets.List(ctx, &permit.ConditionSetsListOptions{
					ListOptions: permit.ListOptions{Page: 2, PerPage: 10},
					Type:        models.ConditionSetTypeResourceSet, Search: "docs & é",
				})
			},
			Request: wireRequest{
				RawPath:  sets,
				RawQuery: "page=2&per_page=10&search=docs+%26+%C3%A9&type=resourceset",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_condition_sets.200.page.json")},
			WantResult: &permit.Page[models.ConditionSetRead]{
				Data: listedConditionSets(), Page: 2, PerPage: 10, TotalCount: new(3),
				PageCount: new(3),
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "ConditionSetsService.All",
			Op:     "list_condition_sets",
			Route:  "GET " + setsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstConditionSets(c.ConditionSets.All(ctx,
					&permit.ConditionSetsListOptions{Type: models.ConditionSetTypeUserSet}), 3)
			},
			Request: wireRequest{RawPath: sets, RawQuery: "page=1&per_page=100&type=userset"},
			Reply:   mockpermit.Reply{Body: replyFixture("list_condition_sets.200.json")},
			// The reply holds a resource set that the filter would leave out: the SDK reads
			// the list as the API gives it.
			WantResult: listedConditionSets(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden, Body: conditionSetForbiddenBody,
			},
			WantErr: forbidden("list_condition_sets", http.MethodGet, setsRoute),
		},
		{
			Method: "ConditionSetsService.Get",
			Op:     "get_condition_set",
			Route:  "GET " + setRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ConditionSets.Get(ctx, "a/b")
			},
			Request: wireRequest{RawPath: sets + "/a%2Fb"},
			Reply:   mockpermit.Reply{Body: replyFixture("get_condition_set.200.json")},
			WantResult: func() *models.ConditionSetRead {
				s := publicDocsSet("2026-10-15T10:00:00.123456")
				s.Description = "Documents that anyone may read"
				return &s
			}(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusNotFound, Body: conditionSetNotFoundBody,
			},
			WantErr: notFound("get_condition_set", http.MethodGet, setRoute),
		},
		{
			Method: "ConditionSetsService.Create",
			Op:     "create_condition_set",
			Route:  "POST " + setsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ConditionSets.Create(ctx, models.ConditionSetCreate{
					Key: "seniors", Type: models.ConditionSetTypeUserSet, Name: "Seniors",
					Description: "Users older than 65",
					Conditions:  allOf(condition("user.age", "greater-than", 65)),
					ParentID:    "00000000-0000-4000-8000-000000000201",
				})
			},
			Request: wireRequest{
				RawPath: sets,
				Body: `{"key":"seniors","type":"userset","name":"Seniors",` +
					`"description":"Users older than 65","conditions":{"allOf":[{"allOf":` +
					`[{"user.age":{"greater-than":65}}]}]},` +
					`"parent_id":"00000000-0000-4000-8000-000000000201"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("create_condition_set.200.json")},
			WantResult: &created,
			ErrorReply: mockpermit.Reply{
				Status: http.StatusConflict, Body: conditionSetConflictBody,
			},
			WantErr: createErr,
		},
		{
			Method: "ConditionSetsService.Update",
			Op:     "update_condition_set",
			Route:  "PATCH " + setRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ConditionSets.Update(ctx, ".hidden", models.ConditionSetUpdate{
					Name: "Grown-ups", Conditions: models.Attributes{},
				})
			},
			Request: wireRequest{
				RawPath: sets + "/.hidden", Body: `{"name":"Grown-ups","conditions":{}}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("update_condition_set.200.json")},
			WantResult: &updated,
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: conditionSetInvalidBody,
			},
			WantErr: updateErr,
		},
		{
			Method: "ConditionSetsService.Delete",
			Op:     "delete_condition_set",
			Route:  "DELETE " + setRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.ConditionSets.Delete(ctx, "50%é")
			},
			Request:    wireRequest{RawPath: sets + "/50%25%C3%A9"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden, Body: conditionSetForbiddenBody,
			},
			WantErr: forbidden("delete_condition_set", http.MethodDelete, setRoute),
		},
		{
			Method: "ConditionSetsService.Ancestors",
			Op:     "get_condition_set_ancestors",
			Route:  "GET " + ancestorsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ConditionSets.Ancestors(ctx, fixtures.UUIDKey,
					&permit.ListOptions{Page: 2, PerPage: 5})
			},
			Request: wireRequest{
				RawPath:  sets + "/" + fixtures.UUIDKey + "/ancestors",
				RawQuery: "page=2&per_page=5",
			},
			Reply: mockpermit.Reply{Body: replyFixture("get_condition_set_ancestors.200.json")},
			WantResult: &permit.Page[models.ConditionSetRead]{
				Data: []models.ConditionSetRead{adultsSet()}, Page: 2, PerPage: 5,
			},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusNotFound, Body: conditionSetNotFoundBody,
			},
			WantErr: notFound("get_condition_set_ancestors", http.MethodGet, ancestorsRoute),
		},
		{
			Method: "ConditionSetsService.AncestorsAll",
			Op:     "get_condition_set_ancestors",
			Route:  "GET " + ancestorsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstConditionSets(c.ConditionSets.AncestorsAll(ctx, "a/b", nil), 1)
			},
			Request: wireRequest{
				RawPath: sets + "/a%2Fb/ancestors", RawQuery: "page=1&per_page=100",
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("get_condition_set_ancestors.200.json"),
			},
			WantResult: []models.ConditionSetRead{adultsSet()},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden, Body: conditionSetForbiddenBody,
			},
			WantErr: forbidden("get_condition_set_ancestors", http.MethodGet, ancestorsRoute),
		},
		{
			Method: "ConditionSetsService.Descendants",
			Op:     "get_condition_set_descendants",
			Route:  "GET " + descendantsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ConditionSets.Descendants(ctx, "50%é", nil)
			},
			Request: wireRequest{RawPath: sets + "/50%25%C3%A9/descendants"},
			Reply: mockpermit.Reply{
				Body: replyFixture("get_condition_set_descendants.200.json"),
			},
			WantResult: &permit.Page[models.ConditionSetRead]{
				Data: []models.ConditionSetRead{seniorsSet(), member}, Page: 1,
			},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusNotFound, Body: conditionSetNotFoundBody,
			},
			WantErr: notFound("get_condition_set_descendants", http.MethodGet,
				descendantsRoute),
		},
		{
			Method: "ConditionSetsService.DescendantsAll",
			Op:     "get_condition_set_descendants",
			Route:  "GET " + descendantsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstConditionSets(c.ConditionSets.DescendantsAll(ctx, ".hidden",
					&permit.ListOptions{PerPage: 50}), 2)
			},
			Request: wireRequest{
				RawPath: sets + "/.hidden/descendants", RawQuery: "page=1&per_page=50",
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("get_condition_set_descendants.200.json"),
			},
			WantResult: []models.ConditionSetRead{seniorsSet(), member},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden, Body: conditionSetForbiddenBody,
			},
			WantErr: forbidden("get_condition_set_descendants", http.MethodGet,
				descendantsRoute),
		},
	}
}
