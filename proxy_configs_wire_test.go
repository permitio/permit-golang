package permit_test

import (
	"context"
	"iter"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the proxy configs. None echoes a
// credential: the tests of proxy_configs_test.go check the bodies that do.
const (
	proxyConfigForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the proxy configs of the environment",` +
		`"id":"req-30"}`
	proxyConfigNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the proxy config does not exist","id":"req-31"}`
	proxyConfigConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"a proxy config with the key payments exists already","id":"req-32"}`
	proxyConfigMissingSecretBody = `{"detail":[{"loc":["body","secret"],` +
		`"msg":"field required","type":"value_error.missing"}]}`
)

// The credentials of the reply fixtures of the proxy configs, which no Permit API key looks like,
// so that only the client's Set hides them.
const (
	fixtureBearerValue = "opaque-bearer-value"
	fixtureBasicValue  = "opaque-user:opaque-pass"
	fixtureHeaderValue = "opaque-header-value"
)

// proxyConfigOf returns a proxy config of the reply fixtures, of the environment of the scope
// fixture, with the fields that every config has.
func proxyConfigOf(key string, id int, name string, mechanism models.AuthMechanism,
	secret models.ProxySecret,
) models.ProxyConfigRead {
	return models.ProxyConfigRead{
		Key: key, ID: resourceID(id), OrganizationID: exampleOrganizationID,
		ProjectID: exampleProjectID, EnvironmentID: exampleEnvironmentID, Name: name,
		AuthMechanism: mechanism, Secret: secret,
	}
}

// paymentsRule is the mapping rule of the config payments of the reply fixtures: a regular
// expression of URLs, with a header, an action and a priority.
func paymentsRule() models.MappingRule {
	return models.MappingRule{
		URL: "https://api.example.com/orders/.*", URLType: models.URLTypeRegex,
		HTTPMethod: models.MethodsPost, Resource: "order",
		Headers: map[string]string{"X-Tenant": "tenant-1"}, Action: "create", Priority: new(2),
	}
}

// paymentsConfig is the config payments of the reply fixtures get_proxy_config.200.json and
// create_proxy_config.200.json, whose secret is a header.
func paymentsConfig() *models.ProxyConfigRead {
	c := proxyConfigOf("payments", 303, "Payments API", models.AuthMechanismHeaders,
		models.ProxySecret{Headers: map[string]models.Secret{
			"X-Api-Key": models.NewSecret(fixtureHeaderValue),
		}})
	c.CreatedAt, c.UpdatedAt = at("2026-10-15T08:00:00Z"), at("2026-10-15T08:00:00Z")
	c.MappingRules = []models.MappingRule{paymentsRule()}
	return &c
}

// listedProxyConfigs are the configs of the reply fixture list_proxy_configs.200.json, one of
// each auth mechanism.
func listedProxyConfigs() []models.ProxyConfigRead {
	const created = "2026-10-14T09:30:00Z"
	billing := proxyConfigOf("billing", 301, "Billing API", models.AuthMechanismBearer,
		models.ProxySecret{Bearer: new(models.NewSecret(fixtureBearerValue))})
	billing.CreatedAt, billing.UpdatedAt = at(created), at(created)
	billing.MappingRules = []models.MappingRule{{
		URL: "https://billing.example.com/invoices", HTTPMethod: models.MethodsGet,
		Resource: "invoice", Headers: map[string]string{}, Action: "read",
	}}
	ledger := proxyConfigOf("ledger", 302, "Ledger API", models.AuthMechanismBasic,
		models.ProxySecret{Basic: new(models.NewSecret(fixtureBasicValue))})
	ledger.CreatedAt, ledger.UpdatedAt = at(created), at(created)
	ledger.MappingRules = []models.MappingRule{}
	return []models.ProxyConfigRead{billing, ledger, *paymentsConfig()}
}

// updatedProxyConfig is the config of the reply fixture update_proxy_config.200.json, whose
// secret is now a Basic credential, with the rule that the update added.
func updatedProxyConfig() *models.ProxyConfigRead {
	c := proxyConfigOf("payments", 303, "Payments", models.AuthMechanismBasic,
		models.ProxySecret{Basic: new(models.NewSecret(fixtureBasicValue))})
	c.CreatedAt, c.UpdatedAt = at("2026-10-15T08:00:00Z"), at("2026-10-15T13:00:00.123456")
	c.MappingRules = []models.MappingRule{paymentsRule(), {
		URL: "https://api.example.com/orders", HTTPMethod: models.MethodsGet,
		Resource: "order", Headers: map[string]string{}, Action: "list",
	}}
	return &c
}

// firstProxyConfigs returns the first n configs of seq, stopping the loop there, or the first
// error it yields.
func firstProxyConfigs(seq iter.Seq2[models.ProxyConfigRead, error], n int,
) ([]models.ProxyConfigRead, error) {
	var got []models.ProxyConfigRead
	for config, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, config)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// proxyConfigCases returns the wire cases of the proxy configs of an environment. Each sends one
// request to the API, along a route of /v2/facts/ that the runner's scope fills, p and e, as
// WithScope gives it, and none runs through the PDP container, which does not serve the proxy
// configs. The keys of the configs hold characters that the path escapes, a slash, a percent sign
// and a letter outside ASCII, as a/b and 50%é do, and a key has a leading dot, .hidden, which the
// path sends as it is. Create sends a secret of headers, and Update a Basic credential, each as
// the credentials themselves; the reply of List holds a config of each auth mechanism.
func proxyConfigCases() []wireCase {
	const (
		configsRoute = "/v2/facts/{proj_id}/{env_id}/proxy_configs"
		configRoute  = configsRoute + "/{proxy_config_id}"
		configs      = "/v2/facts/p/e/proxy_configs"
	)
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, proxyConfigForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-30"
		e.Detail = "the API key may not act on the proxy configs of the environment"
		return e
	}
	listErr := projectError("list_proxy_configs", http.MethodGet, configsRoute,
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	getErr := projectError("get_proxy_config", http.MethodGet, configRoute,
		http.StatusNotFound, proxyConfigNotFoundBody)
	getErr.Code, getErr.Title, getErr.RequestID = "NOT_FOUND", "Not Found", "req-31"
	getErr.Detail = "the proxy config does not exist"
	createErr := projectError("create_proxy_config", http.MethodPost, configsRoute,
		http.StatusConflict, proxyConfigConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "DUPLICATE_ENTITY", "Conflict",
		"req-32"
	createErr.Detail = "a proxy config with the key payments exists already"
	updateErr := projectError("update_proxy_config", http.MethodPatch, configRoute,
		http.StatusUnprocessableEntity, proxyConfigMissingSecretBody)
	updateErr.Detail = "body.secret: field required"
	return []wireCase{
		{
			Method: "ProxyConfigsService.List",
			Op:     "list_proxy_configs",
			Route:  "GET " + configsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ProxyConfigs.List(ctx, &permit.ListOptions{Page: 2, PerPage: 10})
			},
			Request: wireRequest{RawPath: configs, RawQuery: "page=2&per_page=10"},
			Reply:   mockpermit.Reply{Body: replyFixture("list_proxy_configs.200.json")},
			WantResult: &permit.Page[models.ProxyConfigRead]{
				Data: listedProxyConfigs(), Page: 2, PerPage: 10,
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "ProxyConfigsService.All",
			Op:     "list_proxy_configs",
			Route:  "GET " + configsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstProxyConfigs(c.ProxyConfigs.All(ctx, nil), 3)
			},
			Request:    wireRequest{RawPath: configs, RawQuery: "page=1&per_page=100"},
			Reply:      mockpermit.Reply{Body: replyFixture("list_proxy_configs.200.json")},
			WantResult: listedProxyConfigs(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden, Body: proxyConfigForbiddenBody,
			},
			WantErr: forbidden("list_proxy_configs", http.MethodGet, configsRoute),
		},
		{
			Method: "ProxyConfigsService.Get",
			Op:     "get_proxy_config",
			Route:  "GET " + configRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ProxyConfigs.Get(ctx, "a/b")
			},
			Request:    wireRequest{RawPath: configs + "/a%2Fb"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_proxy_config.200.json")},
			WantResult: paymentsConfig(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusNotFound, Body: proxyConfigNotFoundBody,
			},
			WantErr: getErr,
		},
		{
			Method: "ProxyConfigsService.Create",
			Op:     "create_proxy_config",
			Route:  "POST " + configsRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ProxyConfigs.Create(ctx, models.ProxyConfigCreate{
					Key:           "payments",
					Name:          "Payments API",
					AuthMechanism: models.AuthMechanismHeaders,
					Secret: models.ProxySecret{Headers: map[string]models.Secret{
						"X-Api-Key": models.NewSecret(fixtureHeaderValue),
					}},
					MappingRules: []models.MappingRule{paymentsRule()},
				})
			},
			Request: wireRequest{
				RawPath: configs,
				Body: `{"key":"payments","name":"Payments API","auth_mechanism":"Headers",` +
					`"mapping_rules":[{"url":"https://api.example.com/orders/.*",` +
					`"url_type":"regex","http_method":"post","resource":"order",` +
					`"headers":{"X-Tenant":"tenant-1"},"action":"create","priority":2}],` +
					`"secret":{"X-Api-Key":"` + fixtureHeaderValue + `"}}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("create_proxy_config.200.json")},
			WantResult: paymentsConfig(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusConflict, Body: proxyConfigConflictBody,
			},
			WantErr: createErr,
		},
		{
			Method: "ProxyConfigsService.Update",
			Op:     "update_proxy_config",
			Route:  "PATCH " + configRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.ProxyConfigs.Update(ctx, ".hidden", models.ProxyConfigUpdate{
					Name: "Payments", AuthMechanism: models.AuthMechanismBasic,
					Secret: models.ProxySecret{Basic: new(models.NewSecret(fixtureBasicValue))},
					MappingRules: []models.MappingRuleUpdate{{
						URL: "https://api.example.com/orders", HTTPMethod: models.MethodsGet,
						Resource: "order", Action: "list",
					}},
				})
			},
			Request: wireRequest{
				RawPath: configs + "/.hidden",
				Body: `{"name":"Payments","auth_mechanism":"Basic","mapping_rules":[` +
					`{"url":"https://api.example.com/orders","http_method":"get",` +
					`"resource":"order","action":"list"}],` +
					`"secret":"` + fixtureBasicValue + `"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("update_proxy_config.200.json")},
			WantResult: updatedProxyConfig(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: proxyConfigMissingSecretBody,
			},
			WantErr: updateErr,
		},
		{
			Method: "ProxyConfigsService.Delete",
			Op:     "delete_proxy_config",
			Route:  "DELETE " + configRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.ProxyConfigs.Delete(ctx, "50%é")
			},
			Request:    wireRequest{RawPath: configs + "/50%25%C3%A9"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusForbidden, Body: proxyConfigForbiddenBody,
			},
			WantErr: forbidden("delete_proxy_config", http.MethodDelete, configRoute),
		},
	}
}
