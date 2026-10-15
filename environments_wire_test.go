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

// The error bodies of the Permit API in the wire cases of the environments: a key that may not
// act on the environments of the project, an environment that does not exist, a key that another
// environment of the project has already, and a field that the API rejects.
const (
	environmentForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the environments of the project","id":"req-6"}`
	environmentNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the environment does not exist","id":"req-7"}`
	environmentConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"an environment with the key qa exists already","id":"req-8"}`
	environmentInvalidBody = `{"detail":[{"loc":["body","jwks","ttl"],` +
		`"msg":"value is not a valid integer","type":"type_error.integer"}]}`
)

// The IDs of the environments and of the email configuration of the reply fixtures.
const (
	stagingEnvironmentID = "00000000-0000-4000-8000-000000000006"
	exampleEmailConfigID = "00000000-0000-4000-8000-000000000007"
	createdEnvironmentID = "00000000-0000-4000-8000-000000000008"
)

// fixtureEnvironment is the environment of the reply fixture get_environment.200.json, with every
// field.
func fixtureEnvironment() *models.EnvironmentReadWithEmailConfig {
	return &models.EnvironmentReadWithEmailConfig{
		EnvironmentRead: models.EnvironmentRead{
			Key: "production", ID: exampleEnvironmentID, OrganizationID: exampleOrganizationID,
			ProjectID: exampleProjectID, CreatedAt: at("2026-10-14T09:30:00Z"),
			UpdatedAt: at("2026-10-15T10:00:00.123456"), AVPPolicyStoreID: "store-1",
			Name: "Production", Description: "Live traffic",
			CustomBranchName: "production-policy",
			JWKS: &models.JwksConfig{
				Audience: models.OneString("app"),
				Issuer:   models.StringList("https://a.example.com", "https://b.example.com"),
				JWKS: &models.JwksObj{Keys: []models.Attributes{
					{"kid": "k1", "kty": "RSA"},
				}},
				TTL: 600, URL: "https://issuer.example.com/jwks.json",
			},
			JWKSUnpinned: []string{"k0"},
			Settings:     models.Attributes{"limit": json.Number("9007199254740993")},
		},
		EmailConfiguration: exampleEmailConfigID,
	}
}

// listedEnvironments are the environments of the reply fixture list_environments.200.json.
func listedEnvironments() []models.EnvironmentReadWithEmailConfig {
	return []models.EnvironmentReadWithEmailConfig{
		{
			EnvironmentRead: models.EnvironmentRead{
				Key: "production", ID: exampleEnvironmentID,
				OrganizationID: exampleOrganizationID, ProjectID: exampleProjectID,
				CreatedAt: at("2026-10-14T09:30:00Z"), UpdatedAt: at("2026-10-14T09:30:00Z"),
				Name: "Production",
			},
			EmailConfiguration: exampleEmailConfigID,
		},
		{
			EnvironmentRead: models.EnvironmentRead{
				Key: "staging", ID: stagingEnvironmentID, OrganizationID: exampleOrganizationID,
				ProjectID: exampleProjectID, CreatedAt: at("2026-10-15T08:00:00Z"),
				UpdatedAt: at("2026-10-15T08:00:00Z"), Name: "Staging",
				Description: "Tests before a release",
				JWKS:        &models.JwksConfig{Audience: models.StringList("app", "admin")},
			},
			EmailConfiguration: exampleEmailConfigID,
		},
	}
}

// firstEnvironments returns the first n environments that seq yields, and ends the loop there,
// so that it asks for no page past them; or the first error that seq yields.
func firstEnvironments(seq iter.Seq2[models.EnvironmentReadWithEmailConfig, error], n int,
) ([]models.EnvironmentReadWithEmailConfig, error) {
	var got []models.EnvironmentReadWithEmailConfig
	for environment, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, environment)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// environmentCases returns the wire cases of the environments of a project. Each sends one
// request to the API, and none needs the scope of the client: the runner gives the client a
// scope for the routes with {proj_id}, which the calls do not read, as they take the project.
// The keys of the projects and of the environments hold characters that the path escapes, a
// slash, a percent sign and a letter outside ASCII, as a/b and 50%é do, and a key has a leading
// dot, .hidden, which the path sends as it is.
func environmentCases() []wireCase {
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, environmentForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-6"
		e.Detail = "the API key may not act on the environments of the project"
		return e
	}
	const (
		envsRoute = "/v2/projects/{proj_id}/envs"
		envRoute  = envsRoute + "/{env_id}"
	)
	listErr := projectError("list_environments", http.MethodGet, envsRoute,
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	getErr := projectError("get_environment", http.MethodGet, envRoute, http.StatusNotFound,
		environmentNotFoundBody)
	getErr.Code, getErr.Title, getErr.RequestID = "NOT_FOUND", "Not Found", "req-7"
	getErr.Detail = "the environment does not exist"
	createErr := projectError("create_environment", http.MethodPost, envsRoute,
		http.StatusConflict, environmentConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "DUPLICATE_ENTITY", "Conflict", "req-8"
	createErr.Detail = "an environment with the key qa exists already"
	updateErr := projectError("update_environment", http.MethodPatch, envRoute,
		http.StatusUnprocessableEntity, environmentInvalidBody)
	updateErr.Detail = "body.jwks.ttl: value is not a valid integer"
	return []wireCase{
		{
			Method: "EnvironmentsService.List",
			Op:     "list_environments",
			Route:  "GET " + envsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Environments.List(ctx, "a/b", &permit.ListOptions{Page: 2, PerPage: 10})
			},
			Request: wireRequest{
				RawPath: "/v2/projects/a%2Fb/envs", RawQuery: "page=2&per_page=10",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_environments.200.json")},
			WantResult: &permit.Page[models.EnvironmentReadWithEmailConfig]{
				Data: listedEnvironments(), Page: 2, PerPage: 10,
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "EnvironmentsService.All",
			Op:     "list_environments",
			Route:  "GET " + envsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstEnvironments(c.Environments.All(ctx, ".hidden", nil), 2)
			},
			Request: wireRequest{
				RawPath: "/v2/projects/.hidden/envs", RawQuery: "page=1&per_page=100",
			},
			Reply:      mockpermit.Reply{Body: replyFixture("list_environments.200.json")},
			WantResult: listedEnvironments(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden, Body: environmentForbiddenBody,
			},
			WantErr: forbidden("list_environments", http.MethodGet, envsRoute),
		},
		{
			Method: "EnvironmentsService.Get",
			Op:     "get_environment",
			Route:  "GET " + envRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Environments.Get(ctx, "50%é", "a/b")
			},
			Request:    wireRequest{RawPath: "/v2/projects/50%25%C3%A9/envs/a%2Fb"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_environment.200.json")},
			WantResult: fixtureEnvironment(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusNotFound, Body: environmentNotFoundBody,
			},
			WantErr: getErr,
		},
		{
			Method: "EnvironmentsService.Create",
			Op:     "create_environment",
			Route:  "POST " + envsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Environments.Create(ctx, "a/b", newEnvironment())
			},
			Request:    wireRequest{RawPath: "/v2/projects/a%2Fb/envs", Body: newEnvironmentBody},
			Reply:      mockpermit.Reply{Body: replyFixture("create_environment.200.json")},
			WantResult: createdEnvironment(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusConflict, Body: environmentConflictBody,
			},
			WantErr: createErr,
		},
		{
			Method: "EnvironmentsService.Update",
			Op:     "update_environment",
			Route:  "PATCH " + envRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Environments.Update(ctx, ".hidden", "50%é", models.EnvironmentUpdate{
					Name:        "Live",
					Description: models.Null[string](),
					JWKS: models.Some(models.JwksConfig{
						Audience: models.OneString("app"), TTL: 600,
					}),
					Settings: models.Some(models.Attributes{"limit": 10}),
				})
			},
			Request: wireRequest{
				RawPath: "/v2/projects/.hidden/envs/50%25%C3%A9",
				Body: `{"name":"Live","description":null,"jwks":{"audience":"app","ttl":600},` +
					`"settings":{"limit":10}}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("update_environment.200.json")},
			WantResult: updatedEnvironment(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: environmentInvalidBody,
			},
			WantErr: updateErr,
		},
		{
			Method: "EnvironmentsService.Delete",
			Op:     "delete_environment",
			Route:  "DELETE " + envRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Environments.Delete(ctx, "a/b", ".hidden")
			},
			Request:    wireRequest{RawPath: "/v2/projects/a%2Fb/envs/.hidden"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden, Body: environmentForbiddenBody,
			},
			WantErr: forbidden("delete_environment", http.MethodDelete, envRoute),
		},
	}
}

// newEnvironment is the environment that the wire case of EnvironmentsService.Create creates,
// with every field of the request: its settings, and the configuration of its JSON Web Key Sets
// with an audience that is a list and an issuer that is one string.
func newEnvironment() models.EnvironmentCreate {
	return models.EnvironmentCreate{
		Key: "qa", Name: "QA", Description: "Checks by hand", CustomBranchName: "qa-policy",
		JWKS: &models.JwksConfig{
			Audience: models.StringList("app", "admin"),
			Issuer:   models.OneString("https://issuer.example.com"),
			JWKS:     &models.JwksObj{Keys: []models.Attributes{{"kty": "RSA", "kid": "k1"}}},
			TTL:      60, URL: "https://issuer.example.com/jwks.json",
		},
		Settings: models.Attributes{"debug": true},
	}
}

// newEnvironmentBody is the body that the call of EnvironmentsService.Create of newEnvironment
// sends.
const newEnvironmentBody = `{"key":"qa","name":"QA","description":"Checks by hand",` +
	`"custom_branch_name":"qa-policy","jwks":{"audience":["app","admin"],` +
	`"issuer":"https://issuer.example.com","jwks":{"keys":[{"kid":"k1","kty":"RSA"}]},` +
	`"ttl":60,"url":"https://issuer.example.com/jwks.json"},"settings":{"debug":true}}`

// createdEnvironment is the environment of the reply fixture create_environment.200.json.
func createdEnvironment() *models.EnvironmentRead {
	return &models.EnvironmentRead{
		Key: "qa", ID: createdEnvironmentID, OrganizationID: exampleOrganizationID,
		ProjectID: exampleProjectID, CreatedAt: at("2026-10-15T11:00:00Z"),
		UpdatedAt: at("2026-10-15T11:00:00Z"), Name: "QA", Description: "Checks by hand",
		JWKS: &models.JwksConfig{
			Audience: models.StringList("app", "admin"),
			Issuer:   models.OneString("https://issuer.example.com"), TTL: 60,
			URL: "https://issuer.example.com/jwks.json",
		},
		Settings: models.Attributes{"debug": true},
	}
}

// updatedEnvironment is the environment of the reply fixture update_environment.200.json.
func updatedEnvironment() *models.EnvironmentRead {
	return &models.EnvironmentRead{
		Key: "production", ID: exampleEnvironmentID, OrganizationID: exampleOrganizationID,
		ProjectID: exampleProjectID, CreatedAt: at("2026-10-14T09:30:00Z"),
		UpdatedAt: at("2026-10-15T12:00:00Z"), Name: "Live",
		JWKS:     &models.JwksConfig{Audience: models.OneString("app"), TTL: 600},
		Settings: models.Attributes{"limit": json.Number("10")},
	}
}
