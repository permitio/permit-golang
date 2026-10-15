package permit_test

import (
	"context"
	"iter"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the API keys: a key that may not act on
// the API keys of the organization, a key that does not exist, and a field that the API rejects.
// An environment that does not exist answers with the body of the environments' wire cases.
const (
	apiKeyForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the API keys of the organization","id":"req-6"}`
	apiKeyNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the API key does not exist","id":"req-7"}`
	apiKeyInvalidBody = `{"detail":[{"loc":["body","organization_id"],` +
		`"msg":"value is not a valid uuid","type":"type_error.uuid"}]}`
)

// The IDs of the API keys of the reply fixtures.
const (
	environmentAPIKeyID = "00000000-0000-4000-8000-000000000011"
	ciAPIKeyID          = "00000000-0000-4000-8000-000000000012"
	createdAPIKeyID     = "00000000-0000-4000-8000-000000000013"
	reportsAPIKeyID     = "00000000-0000-4000-8000-000000000014"
)

// fixtureEnvironmentKey is the API key of the reply fixture get_environment_api_key.200.json,
// the key of the environment of the scope of the Examples.
func fixtureEnvironmentKey() *models.APIKeyRead {
	return &models.APIKeyRead{
		OrganizationID: exampleOrganizationID, ProjectID: exampleProjectID,
		EnvironmentID: exampleEnvironmentID, ObjectType: models.MemberAccessObjEnv,
		AccessLevel: models.MemberAccessLevelAdmin, OwnerType: models.APIKeyOwnerTypeMember,
		ID: environmentAPIKeyID, Secret: models.NewSecret(canary.Key("environment")),
		CreatedAt: at("2026-10-14T09:30:00Z"),
	}
}

// fixtureAPIKey is the API key of the reply fixture get_api_key.200.json, with every field of
// the model. The reply also holds the member who created the key, which the model leaves out.
func fixtureAPIKey() *models.APIKeyRead {
	return &models.APIKeyRead{
		OrganizationID: exampleOrganizationID, ProjectID: exampleProjectID,
		ObjectType: models.MemberAccessObjProject, AccessLevel: models.MemberAccessLevelWrite,
		OwnerType: models.APIKeyOwnerTypePDPConfig, Name: "CI key", ID: ciAPIKeyID,
		Secret: models.NewSecret(canary.Key("ci")), CreatedAt: at("2026-10-14T09:30:00Z"),
		LastUsedAt: at("2026-10-15T10:00:00.123456"),
		Env: &models.EnvironmentRead{
			Key: "production", ID: exampleEnvironmentID, OrganizationID: exampleOrganizationID,
			ProjectID: exampleProjectID, CreatedAt: at("2026-10-14T09:30:00Z"),
			UpdatedAt: at("2026-10-14T09:30:00Z"), Name: "Production",
		},
		Project: &models.ProjectRead{
			Key: "my-project", ID: exampleProjectID, OrganizationID: exampleOrganizationID,
			CreatedAt: at("2026-10-14T09:30:00Z"), UpdatedAt: at("2026-10-14T09:30:00Z"),
			Name: "My project",
		},
	}
}

// listedAPIKeys are the API keys of the reply fixture list_api_keys.200.json.
func listedAPIKeys() []models.APIKeyRead {
	return []models.APIKeyRead{
		*fixtureEnvironmentKey(),
		{
			OrganizationID: exampleOrganizationID, ObjectType: models.MemberAccessObjOrg,
			AccessLevel: models.MemberAccessLevelRead, OwnerType: models.APIKeyOwnerTypeMember,
			Name: "Reports", ID: reportsAPIKeyID, CreatedAt: at("2026-10-15T08:00:00Z"),
		},
	}
}

// firstAPIKeys returns the first n API keys that seq yields, and ends the loop there, so that it
// asks for no page past them; or the first error that seq yields.
func firstAPIKeys(seq iter.Seq2[models.APIKeyRead, error], n int,
) ([]models.APIKeyRead, error) {
	var got []models.APIKeyRead
	for key, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, key)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// newAPIKey is the API key that the wire case of APIKeysService.Create creates, with every field
// of the request.
func newAPIKey() models.APIKeyCreate {
	return models.APIKeyCreate{
		OrganizationID: exampleOrganizationID, ProjectID: exampleProjectID,
		EnvironmentID: exampleEnvironmentID, ObjectType: models.MemberAccessObjEnv,
		AccessLevel: models.MemberAccessLevelRead, OwnerType: models.APIKeyOwnerTypeMember,
		Name: "Read-only key",
	}
}

// newAPIKeyBody is the body that the call of APIKeysService.Create of newAPIKey sends.
const newAPIKeyBody = `{"organization_id":"00000000-0000-4000-8000-000000000001",` +
	`"project_id":"00000000-0000-4000-8000-000000000002",` +
	`"environment_id":"00000000-0000-4000-8000-000000000003","object_type":"env",` +
	`"access_level":"read","owner_type":"member","name":"Read-only key"}`

// createdAPIKey is the API key of the reply fixture create_api_key.200.json.
func createdAPIKey() *models.APIKeyRead {
	return &models.APIKeyRead{
		OrganizationID: exampleOrganizationID, ProjectID: exampleProjectID,
		EnvironmentID: exampleEnvironmentID, ObjectType: models.MemberAccessObjEnv,
		AccessLevel: models.MemberAccessLevelRead, OwnerType: models.APIKeyOwnerTypeMember,
		Name: "Read-only key", ID: createdAPIKeyID,
		Secret: models.NewSecret(canary.Key("created")), CreatedAt: at("2026-10-15T11:00:00Z"),
	}
}

// rotatedAPIKey is the API key of the reply fixture rotate_api_key.200.json: the key of the
// environment with a new secret.
func rotatedAPIKey() *models.APIKeyRead {
	key := fixtureEnvironmentKey()
	key.Secret = models.NewSecret(canary.Key("rotated"))
	key.LastUsedAt = at("2026-10-15T12:00:00Z")
	return key
}

// apiKeyCases returns the wire cases of the API keys. Each sends one request to the API, and none
// needs the scope of the client: the runner gives the client a scope for the route with
// {proj_id} and {env_id}, which GetEnvironmentKey does not read, as it takes the project and the
// environment. The keys and IDs hold characters that the path escapes, a slash, a percent sign
// and a letter outside ASCII, as a/b and 50%é do, and one ID has a leading dot, .hidden, which
// the path sends as it is.
func apiKeyCases() []wireCase {
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, apiKeyForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-6"
		e.Detail = "the API key may not act on the API keys of the organization"
		return e
	}
	notFound := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusNotFound, apiKeyNotFoundBody)
		e.Code, e.Title, e.RequestID = "NOT_FOUND", "Not Found", "req-7"
		e.Detail = "the API key does not exist"
		return e
	}
	envErr := projectError("get_environment_api_key", http.MethodGet,
		"/v2/api-key/{proj_id}/{env_id}", http.StatusNotFound, environmentNotFoundBody)
	envErr.Code, envErr.Title, envErr.RequestID = "NOT_FOUND", "Not Found", "req-7"
	envErr.Detail = "the environment does not exist"
	listErr := projectError("list_api_keys", http.MethodGet, "/v2/api-key",
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	createErr := projectError("create_api_key", http.MethodPost, "/v2/api-key",
		http.StatusUnprocessableEntity, apiKeyInvalidBody)
	createErr.Detail = "body.organization_id: value is not a valid uuid"
	return []wireCase{
		{
			Method: "APIKeysService.GetEnvironmentKey",
			Op:     "get_environment_api_key",
			Route:  "GET /v2/api-key/{proj_id}/{env_id}",
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.APIKeys.GetEnvironmentKey(ctx, "a/b", "50%é")
			},
			Request:    wireRequest{RawPath: "/v2/api-key/a%2Fb/50%25%C3%A9"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_environment_api_key.200.json")},
			WantResult: fixtureEnvironmentKey(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusNotFound, Body: environmentNotFoundBody,
			},
			WantErr: envErr,
		},
		{
			Method: "APIKeysService.List",
			Op:     "list_api_keys",
			Route:  "GET /v2/api-key",
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.APIKeys.List(ctx, &permit.APIKeysListOptions{
					ListOptions: permit.ListOptions{Page: 2, PerPage: 10},
					ObjectType:  models.MemberAccessObjEnv, Project: "my-project",
				})
			},
			Request: wireRequest{
				RawPath:  "/v2/api-key",
				RawQuery: "object_type=env&page=2&per_page=10&proj_id=my-project",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_api_keys.200.json")},
			WantResult: &permit.Page[models.APIKeyRead]{
				Data: listedAPIKeys(), Page: 2, PerPage: 10, TotalCount: new(2),
				PageCount: new(1),
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "APIKeysService.All",
			Op:     "list_api_keys",
			Route:  "GET /v2/api-key",
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstAPIKeys(c.APIKeys.All(ctx, nil), 2)
			},
			Request:    wireRequest{RawPath: "/v2/api-key", RawQuery: "page=1&per_page=100"},
			Reply:      mockpermit.Reply{Body: replyFixture("list_api_keys.200.json")},
			WantResult: listedAPIKeys(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: apiKeyForbiddenBody},
			WantErr:    forbidden("list_api_keys", http.MethodGet, "/v2/api-key"),
		},
		{
			Method: "APIKeysService.Get",
			Op:     "get_api_key",
			Route:  "GET /v2/api-key/{api_key_id}",
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.APIKeys.Get(ctx, "50%é")
			},
			Request:    wireRequest{RawPath: "/v2/api-key/50%25%C3%A9"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_api_key.200.json")},
			WantResult: fixtureAPIKey(),
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: apiKeyNotFoundBody},
			WantErr:    notFound("get_api_key", http.MethodGet, "/v2/api-key/{api_key_id}"),
		},
		{
			Method: "APIKeysService.Create",
			Op:     "create_api_key",
			Route:  "POST /v2/api-key",
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.APIKeys.Create(ctx, newAPIKey())
			},
			Request:    wireRequest{RawPath: "/v2/api-key", Body: newAPIKeyBody},
			Reply:      mockpermit.Reply{Body: replyFixture("create_api_key.200.json")},
			WantResult: createdAPIKey(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: apiKeyInvalidBody,
			},
			WantErr: createErr,
		},
		{
			Method: "APIKeysService.Delete",
			Op:     "delete_api_key",
			Route:  "DELETE /v2/api-key/{api_key_id}",
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.APIKeys.Delete(ctx, ".hidden")
			},
			Request:    wireRequest{RawPath: "/v2/api-key/.hidden"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: apiKeyForbiddenBody},
			WantErr: forbidden("delete_api_key", http.MethodDelete,
				"/v2/api-key/{api_key_id}"),
		},
		{
			Method: "APIKeysService.RotateSecret",
			Op:     "rotate_api_key",
			Route:  "POST /v2/api-key/{api_key_id}/rotate-secret",
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.APIKeys.RotateSecret(ctx, "a/b")
			},
			Request:    wireRequest{RawPath: "/v2/api-key/a%2Fb/rotate-secret"},
			Reply:      mockpermit.Reply{Body: replyFixture("rotate_api_key.200.json")},
			WantResult: rotatedAPIKey(),
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: apiKeyNotFoundBody},
			WantErr: notFound("rotate_api_key", http.MethodPost,
				"/v2/api-key/{api_key_id}/rotate-secret"),
		},
	}
}
