// Package provider builds the v1 client of a sample shaped like a Terraform provider: the
// provider reads its settings, builds one client, and hands it to each resource. Each line that
// uses v1 ends with a comment "v1:" that lists what the scanner must report on it.
package provider

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/permitio/permit-golang/pkg/config" // v1: github.com/permitio/permit-golang/pkg/config
	"github.com/permitio/permit-golang/pkg/permit" // v1: github.com/permitio/permit-golang/pkg/permit
)

// Settings are the provider's settings.
type Settings struct {
	APIKey      string
	APIURL      string
	Project     string
	Environment string
	Timeout     time.Duration
}

// Configure returns the client that the resources share.
func Configure(ctx context.Context, settings Settings) (*permit.Client, error) { // v1: permit.Client
	if settings.APIKey == "" {
		return nil, errors.New("api_key must be set")
	}
	builder := config.NewConfigBuilder(settings.APIKey) // v1: config.NewConfigBuilder
	if settings.APIURL != "" {
		builder.WithApiUrl(settings.APIURL) // v1: config.PermitConfig.WithApiUrl
	}
	permitConfig := builder.
		WithTimeout(settings.Timeout).                           // v1: config.PermitConfig.WithTimeout
		WithHTTPClient(&http.Client{Timeout: settings.Timeout}). // v1: config.PermitConfig.WithHTTPClient
		WithProxyFactsViaPDP(false).                             // v1: config.PermitConfig.WithProxyFactsViaPDP
		Build()                                                  // v1: config.PermitConfig.Build
	client := permit.NewPermit(permitConfig) // v1: permit.NewPermit
	if settings.Project != "" {
		client.Api.SetContext(ctx, settings.Project, settings.Environment) // v1: api.PermitApiClient.SetContext
	}
	return client, nil
}
