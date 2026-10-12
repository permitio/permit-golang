package config_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/permitio/permit-golang/pkg/api"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/enforcement"
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
