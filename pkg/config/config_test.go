package config_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/pkg/api"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/enforcement"
	"github.com/permitio/permit-golang/pkg/openapi"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

const testToken = "test-token"

func TestNewPermitConfigWithNilLoggerUsesNoopLogger(t *testing.T) {
	cfg := config.NewPermitConfig("", testToken, "", false, nil, nil)

	assert.NotPanics(t, func() { cfg.GetLogger().Error("logged") })
}

func TestNilLoggerDoesNotPanicOnLoggedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"detail":"unavailable"}`))
	}))
	t.Cleanup(srv.Close)
	permitContext := config.NewPermitContext(config.EnvironmentAPIKeyLevel, "proj", "env")
	configs := []struct {
		name   string
		config func() config.PermitConfig
	}{
		{
			name: "NewPermitConfig",
			config: func() config.PermitConfig {
				return *config.NewPermitConfig(srv.URL, testToken, srv.URL, false, permitContext, nil)
			},
		},
		{
			name: "builder chain without Build",
			config: func() config.PermitConfig {
				return *config.NewConfigBuilder(testToken).
					WithApiUrl(srv.URL).
					WithPdpUrl(srv.URL).
					WithContext(permitContext)
			},
		},
		{
			name: "zero-value PermitConfig literal",
			config: func() config.PermitConfig {
				cfg := config.PermitConfig{}
				return *cfg.WithApiUrl(srv.URL).WithPdpUrl(srv.URL).WithContext(permitContext)
			},
		},
		{
			name: "WithLogger(nil)",
			config: func() config.PermitConfig {
				cfg := config.NewConfigBuilder(testToken).
					WithApiUrl(srv.URL).
					WithPdpUrl(srv.URL).
					WithContext(permitContext).
					WithLogger(zap.NewNop()).
					Build()
				return *cfg.WithLogger(nil)
			},
		},
	}
	ctx := context.Background()
	user := enforcement.UserBuilder("u1").Build()
	resource := enforcement.ResourceBuilder("doc").Build()
	for _, tt := range configs {
		t.Run(tt.name, func(t *testing.T) {
			client := permit.NewPermit(tt.config())
			apiConfig := tt.config()
			apiClient := api.NewPermitApiClient(&apiConfig)
			enforcerConfig := tt.config()
			enforcer := enforcement.NewPermitEnforcerClient(&enforcerConfig)

			assert.NotPanics(t, func() {
				_, err := client.Api.Users.Get(ctx, "u1")
				assert.Error(t, err)
			}, "NewPermit API call")
			assert.NotPanics(t, func() {
				_, err := client.Check(user, "read", resource)
				assert.Error(t, err)
			}, "NewPermit Check")
			assert.NotPanics(t, func() {
				_, err := apiClient.Users.Get(ctx, "u1")
				assert.Error(t, err)
			}, "NewPermitApiClient API call")
			assert.NotPanics(t, func() {
				_, err := enforcer.Check(user, "read", resource)
				assert.Error(t, err)
			}, "NewPermitEnforcerClient Check")
		})
	}
}

// printVerbs are fmt directives that apply to structs and pointers, and ones that do not: fmt
// prints the fields of a struct with them anyway, and prints in full what an unexported pointer
// field points to.
var printVerbs = []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%-12.4v", "%10s",
	"%+q", "%#q", "%#x", "% x", "%08v", "%t", "%e"}

// assertPrintsWithoutSecret prints value with each of printVerbs, fmt.Sprint and fmt.Sprintln,
// and fails when an output holds secret as text or in one of the hexadecimal forms that %x, %X
// and % x print.
func assertPrintsWithoutSecret(t *testing.T, value interface{}, secret string) {
	t.Helper()
	hexSecret := hex.EncodeToString([]byte(secret))
	forms := []string{secret, hexSecret, strings.ToUpper(hexSecret),
		strings.TrimSpace(fmt.Sprintf("% x", secret))}
	outputs := map[string]string{"Sprint": fmt.Sprint(value), "Sprintln": fmt.Sprintln(value)}
	for _, verb := range printVerbs {
		outputs[verb] = fmt.Sprintf(verb, value)
	}
	for name, output := range outputs {
		for _, form := range forms {
			assert.NotContains(t, output, form, "%s printed the API key", name)
		}
	}
}

type embedsPermitConfig struct {
	config.PermitConfig
	DatabaseURL string
}

type holdsPermitConfig struct {
	config config.PermitConfig
}

type printedValue struct {
	name  string
	value interface{}
}

func TestPrintingConfigurationHidesTheAPIKey(t *testing.T) {
	const apiKey = "config-print-key-5f2c9a"
	builder := config.NewConfigBuilder(apiKey)
	configured := builder.
		WithApiUrl("https://api.example.test").
		WithPdpUrl("http://pdp.example.test:7766").
		WithContext(config.NewPermitContext(config.EnvironmentAPIKeyLevel, "proj", "env")).
		WithLogger(zap.NewNop())
	built := configured.Build()
	newPermitConfig := config.NewPermitConfig("https://api.example.test", apiKey, "", false, nil,
		nil)
	proxyFacts := config.NewConfigBuilder(apiKey).WithProxyFactsViaPDP(true).
		WithLogger(zap.NewNop()).Build()
	clientConfig := api.NewClientConfig(&built)
	factsClientConfig := api.NewFactsClientConfig(&built)
	client := permit.NewPermit(built)
	proxyFactsClient := permit.NewPermit(proxyFacts)
	enforcer := enforcement.NewPermitEnforcerClient(&built)
	var nilConfig *config.PermitConfig
	values := []printedValue{
		{name: "PermitConfig", value: built},
		{name: "*PermitConfig", value: &built},
		{name: "*PermitBuilder", value: builder},
		{name: "PermitBuilder", value: *builder},
		{name: "*PermitConfig from the builder chain", value: configured},
		{name: "NewPermitConfig", value: newPermitConfig},
		{name: "exported struct field", value: struct{ Config config.PermitConfig }{built}},
		{name: "unexported struct field", value: holdsPermitConfig{built}},
		{name: "embedded", value: embedsPermitConfig{PermitConfig: built}},
		{name: "slice", value: []config.PermitConfig{built}},
		{name: "map", value: map[string]*config.PermitConfig{"c": &built}},
		{name: "nil *PermitConfig", value: nilConfig},
		{name: "PermitContext", value: *built.GetContext()},
		{name: "*PermitContext", value: built.GetContext()},
		{name: "*openapi.Configuration from NewClientConfig", value: clientConfig},
		{name: "openapi.Configuration from NewClientConfig", value: *clientConfig},
		{name: "*openapi.Configuration from NewFactsClientConfig", value: factsClientConfig},
		{name: "openapi.Configuration from NewFactsClientConfig", value: *factsClientConfig},
		{name: "*openapi.APIClient", value: openapi.NewAPIClient(clientConfig)},
		{name: "*permit.Client", value: client},
		{name: "permit.Client", value: *client},
		{name: "*api.PermitApiClient", value: client.Api},
		{name: "api.PermitApiClient", value: *client.Api},
		{name: "*enforcement.PermitEnforcer", value: enforcer},
		{name: "enforcement.PermitEnforcer", value: *enforcer},
		{name: "WaitForSync without proxyFactsViaPDP",
			value: client.Api.Users.WaitForSync(nil, api.WaitForSyncOptions{})},
		{name: "WaitForSync with proxyFactsViaPDP",
			value: proxyFactsClient.Api.Users.WaitForSync(nil, api.WaitForSyncOptions{})},
	}
	apis := reflect.ValueOf(*client.Api)
	for i := 0; i < apis.NumField(); i++ {
		if field := apis.Type().Field(i); field.IsExported() {
			values = append(values,
				printedValue{name: "api " + field.Name, value: apis.Field(i).Interface()})
		}
	}
	for _, tt := range values {
		t.Run(tt.name, func(t *testing.T) {
			assertPrintsWithoutSecret(t, tt.value, apiKey)
		})
	}

	t.Run("the other fields are printed", func(t *testing.T) {
		assert.Contains(t, fmt.Sprintf("%v", built), "{https://api.example.test 0x")
		plus := fmt.Sprintf("%+v", &built)
		assert.Contains(t, plus, "&{apiUrl:https://api.example.test token:0x")
		assert.Contains(t, plus, " pdpUrl:http://pdp.example.test:7766 ")
		assert.Contains(t, plus, "factsSyncTimeoutPolicy:")
		goSyntax := fmt.Sprintf("%#v", built)
		assert.True(t, strings.HasPrefix(goSyntax,
			`config.PermitConfig{apiUrl:"https://api.example.test", token:(*string)(0x`), goSyntax)
		assert.Contains(t, fmt.Sprintf("%+v", embedsPermitConfig{PermitConfig: built,
			DatabaseURL: "postgres://db"}), "DatabaseURL:postgres://db")
		assert.Equal(t, "<nil>", fmt.Sprintf("%v", nilConfig))

		assert.Contains(t, fmt.Sprintf("%+v", clientConfig),
			"Host:api.example.test Scheme:https DefaultHeader:map[Authorization:[REDACTED]]")
		assert.Contains(t, fmt.Sprintf("%#v", *factsClientConfig),
			`openapi.Configuration{Host:"pdp.example.test:7766"`)
		assert.Equal(t, map[string]string{"Authorization": "Bearer " + apiKey},
			clientConfig.DefaultHeader, "printing must not change the headers that are sent")
	})

	t.Run("the API key is kept", func(t *testing.T) {
		copied := built
		assert.True(t, copied == built, "copies of a config are equal")
		assert.Equal(t, apiKey, copied.GetToken())
		assert.Equal(t, apiKey, builder.GetToken())
		assert.Equal(t, apiKey, newPermitConfig.GetToken())
		assert.Equal(t, "", (&config.PermitConfig{}).GetToken())
	})
}
