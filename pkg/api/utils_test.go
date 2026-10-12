package api_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/permitio/permit-golang/pkg/api"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/openapi"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

var shortUrls = []string{"", "x", "http", "https", "abcdef", "https:/"}

// newUrlConfig uses NewPermitConfig, which keeps an empty or short URL as given; WithApiUrl and
// WithPdpUrl ignore "".
func newUrlConfig(url string) *config.PermitConfig {
	permitContext := config.NewPermitContext(config.EnvironmentAPIKeyLevel, "proj", "env")
	return config.NewPermitConfig(url, testToken, url, false, permitContext, zap.NewNop())
}

func TestNewPermitWithShortApiUrl(t *testing.T) {
	for _, url := range shortUrls {
		t.Run(fmt.Sprintf("%q", url), func(t *testing.T) {
			var client *permit.Client
			if !assert.NotPanics(t, func() { client = permit.NewPermit(*newUrlConfig(url)) }) {
				return
			}

			_, err := client.Api.Users.Get(context.Background(), "u1")

			assert.Error(t, err)
			assert.Equal(t, "", api.NewClientConfig(newUrlConfig(url)).Host)
		})
	}
}

func TestNewFactsClientConfigWithShortPdpUrl(t *testing.T) {
	for _, url := range shortUrls {
		t.Run(fmt.Sprintf("%q", url), func(t *testing.T) {
			var clientConfig *openapi.Configuration
			buildConfig := func() { clientConfig = api.NewFactsClientConfig(newUrlConfig(url)) }
			if !assert.NotPanics(t, buildConfig) {
				return
			}

			assert.Equal(t, "", clientConfig.Host)
		})
	}
}

func TestNewClientConfigSplitsSchemeAndHost(t *testing.T) {
	tests := []struct {
		url        string
		wantScheme string
		wantHost   string
	}{
		{url: "https://api.example.invalid", wantScheme: "https", wantHost: "api.example.invalid"},
		{url: "http://127.0.0.1:7766", wantScheme: "http", wantHost: "127.0.0.1:7766"},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			clientConfig := api.NewClientConfig(newUrlConfig(tt.url))

			assert.Equal(t, tt.wantScheme, clientConfig.Scheme)
			assert.Equal(t, tt.wantHost, clientConfig.Host)
		})
	}
}
