package permit_test

import (
	"context"
	"iter"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the condition set rules: a key that may
// not act on the rules of the environment, a set that does not exist, and a field that the API
// rejects.
const (
	conditionSetRuleForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the condition set rules of the environment",` +
		`"id":"req-34"}`
	conditionSetRuleNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the condition set does not exist","id":"req-35"}`
	conditionSetRuleInvalidBody = `{"detail":[{"loc":["body","permission"],` +
		`"msg":"str type expected","type":"type_error.str"}]}`
)

// conditionSetRuleOf returns a condition set rule of the reply fixtures, of the environment of
// the scope fixture, with the key key, which grants the users of the user set userSet the
// permission on the resources of the resource set public_docs, created and last changed at
// created.
func conditionSetRuleOf(id int, key, userSet, permission, created string,
) models.ConditionSetRuleRead {
	return models.ConditionSetRuleRead{
		ID: resourceID(id), Key: key, UserSet: userSet, Permission: permission,
		ResourceSet: "public_docs", OrganizationID: exampleOrganizationID,
		ProjectID: exampleProjectID, EnvironmentID: exampleEnvironmentID, CreatedAt: at(created),
		UpdatedAt: at(created),
	}
}

// adultsReadRule is the rule of the reply fixtures that grants the users of the user set adults
// the permission document:read on the resource set public_docs.
func adultsReadRule() models.ConditionSetRuleRead {
	return conditionSetRuleOf(301, "adults_document-read_public_docs", "adults", "document:read",
		"2026-10-15T09:00:00Z")
}

// listedConditionSetRules are the condition set rules of the reply fixture
// list_set_permissions.200.json: the second was last changed at a time without a zone, which the
// SDK reads as UTC.
func listedConditionSetRules() []models.ConditionSetRuleRead {
	seniors := conditionSetRuleOf(302, "seniors_document-write_public_docs", "seniors",
		"document:write", "2026-10-15T09:05:00Z")
	seniors.UpdatedAt = at("2026-10-15T09:05:00.25Z")
	return []models.ConditionSetRuleRead{adultsReadRule(), seniors}
}

// firstConditionSetRules returns the first n condition set rules that seq yields, and ends the
// loop there, so that it asks for no page past them; or the first error that seq yields.
func firstConditionSetRules(seq iter.Seq2[models.ConditionSetRuleRead, error], n int,
) ([]models.ConditionSetRuleRead, error) {
	var got []models.ConditionSetRuleRead
	for rule, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, rule)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// conditionSetRuleCases returns the wire cases of the condition set rules of an environment.
// Each sends one request to the API, along a route under /v2/facts/ that the runner's scope
// fills, p and e, as WithScope gives it; no case runs with WithFactsViaPDP, as only the API
// serves the rules. The filters and the bodies name sets and permissions with characters that
// the query escapes, a slash, a colon, a percent sign and a letter outside ASCII, as a/b,
// document:read and 50%é do, a key with a leading dot, .hidden, and an ID of an action in the
// shape of a UUID.
func conditionSetRuleCases() []wireCase {
	const (
		rulesRoute = "/v2/facts/{proj_id}/{env_id}/set_rules"
		rules      = "/v2/facts/p/e/set_rules"
	)
	forbidden := projectError("list_set_permissions", http.MethodGet, rulesRoute,
		http.StatusForbidden, conditionSetRuleForbiddenBody)
	forbidden.Code, forbidden.Title, forbidden.RequestID = "FORBIDDEN", "Forbidden", "req-34"
	forbidden.Detail = "the API key may not act on the condition set rules of the environment"
	listErr := projectError("list_set_permissions", http.MethodGet, rulesRoute,
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	createErr := projectError("assign_set_permissions", http.MethodPost, rulesRoute,
		http.StatusUnprocessableEntity, conditionSetRuleInvalidBody)
	createErr.Detail = "body.permission: str type expected"
	deleteErr := projectError("unassign_set_permissions", http.MethodDelete, rulesRoute,
		http.StatusNotFound, conditionSetRuleNotFoundBody)
	deleteErr.Code, deleteErr.Title, deleteErr.RequestID = "NOT_FOUND", "Not Found", "req-35"
	deleteErr.Detail = "the condition set does not exist"
	return []wireCase{
		{
			Method: "ConditionSetRulesService.List",
			Op:     "list_set_permissions",
			Route:  "GET " + rulesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ConditionSetRules.List(ctx, &permit.ConditionSetRulesListOptions{
					ListOptions: permit.ListOptions{Page: 2, PerPage: 10},
					UserSet:     "a/b", Permission: "document:read", ResourceSet: "50%é",
				})
			},
			Request: wireRequest{
				RawPath: rules,
				RawQuery: "page=2&per_page=10&permission=document%3Aread&" +
					"resource_set=50%25%C3%A9&user_set=a%2Fb",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_set_permissions.200.json")},
			WantResult: &permit.Page[models.ConditionSetRuleRead]{
				Data: listedConditionSetRules(), Page: 2, PerPage: 10,
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "ConditionSetRulesService.All",
			Op:     "list_set_permissions",
			Route:  "GET " + rulesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstConditionSetRules(c.ConditionSetRules.All(ctx,
					&permit.ConditionSetRulesListOptions{UserSet: ".hidden"}), 2)
			},
			Request: wireRequest{RawPath: rules, RawQuery: "page=1&per_page=100&user_set=.hidden"},
			Reply:   mockpermit.Reply{Body: replyFixture("list_set_permissions.200.json")},
			// The reply holds rules of other user sets, which the filter would leave out: the
			// SDK reads the list as the API gives it.
			WantResult: listedConditionSetRules(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden, Body: conditionSetRuleForbiddenBody,
			},
			WantErr: forbidden,
		},
		{
			Method: "ConditionSetRulesService.Create",
			Op:     "assign_set_permissions",
			Route:  "POST " + rulesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ConditionSetRules.Create(ctx, models.ConditionSetRuleCreate{
					UserSet: "a/b", Permission: "document:read", ResourceSet: "50%é",
				})
			},
			Request: wireRequest{
				RawPath: rules,
				Body:    `{"user_set":"a/b","permission":"document:read","resource_set":"50%é"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("assign_set_permissions.200.json")},
			WantResult: []models.ConditionSetRuleRead{adultsReadRule()},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: conditionSetRuleInvalidBody,
			},
			WantErr: createErr,
		},
		{
			Method: "ConditionSetRulesService.Delete",
			Op:     "unassign_set_permissions",
			Route:  "DELETE " + rulesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.ConditionSetRules.Delete(ctx,
					models.ConditionSetRuleRemove{
						UserSet:     ".hidden",
						Permission:  fixtures.UUIDKey,
						ResourceSet: "public_docs",
					})
			},
			Request: wireRequest{
				RawPath: rules,
				Body: `{"user_set":".hidden","permission":"` + fixtures.UUIDKey + `",` +
					`"resource_set":"public_docs"}`,
			},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusNotFound, Body: conditionSetRuleNotFoundBody,
			},
			WantErr: deleteErr,
		},
	}
}
