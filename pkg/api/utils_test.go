package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/permitio/permit-golang/pkg/api"
	"github.com/permitio/permit-golang/pkg/config"
	permitErrors "github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/openapi"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
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

// pageSizeListMethods calls each list method that checks its page size against
// api.DefaultPerPageLimit, with perPage as the page size; message is what the method logs when
// the page size is over the limit.
var pageSizeListMethods = []struct {
	name    string
	message string
	call    func(ctx context.Context, client *api.PermitApiClient, perPage int) error
}{
	{
		name:    "ConditionSets.List",
		message: "error listing condition sets - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.ConditionSets.List(ctx, 1, perPage)
			return err
		},
	},
	{
		name:    "Environments.List",
		message: "error listing environments - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.Environments.List(ctx, 1, perPage)
			return err
		},
	},
	{
		name:    "Projects.List",
		message: "error listing projects - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.Projects.List(ctx, 1, perPage)
			return err
		},
	},
	{
		name:    "ProxyConfigs.List",
		message: "error listing ProxyConfigs - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.ProxyConfigs.List(ctx, 1, perPage)
			return err
		},
	},
	{
		name:    "RelationshipTuples.List",
		message: "error listing relationship tuples - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.RelationshipTuples.List(ctx, 1, perPage, "", "", "", "")
			return err
		},
	},
	{
		name:    "ResourceActionGroups.List",
		message: "error listing resource action groups - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.ResourceActionGroups.List(ctx, "doc", 1, perPage)
			return err
		},
	},
	{
		name:    "ResourceActionGroups.ListByAttributes",
		message: "error listing resource action groups - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.ResourceActionGroups.ListByAttributes(ctx, "doc", 1, perPage, nil)
			return err
		},
	},
	{
		name:    "ResourceActions.List",
		message: "error listing resource actions - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.ResourceActions.List(ctx, "doc", 1, perPage)
			return err
		},
	},
	{
		name:    "ResourceActions.ListByAttributes",
		message: "error listing resource actions - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.ResourceActions.ListByAttributes(ctx, "doc", 1, perPage, nil)
			return err
		},
	},
	{
		name:    "ResourceAttributes.List",
		message: "error listing resource attributes - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.ResourceAttributes.List(ctx, "doc", 1, perPage)
			return err
		},
	},
	{
		name:    "ResourceInstances.List",
		message: "error listing resource instances - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.ResourceInstances.List(ctx, 1, perPage, "", "", "")
			return err
		},
	},
	{
		name:    "ResourceRelations.List",
		message: "error listing resource relations - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.ResourceRelations.List(ctx, 1, perPage, "doc")
			return err
		},
	},
	{
		name:    "ResourceRoles.List",
		message: "error listing resource roles - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.ResourceRoles.List(ctx, 1, perPage, "doc")
			return err
		},
	},
	{
		name:    "Resources.List",
		message: "error listing resources - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.Resources.List(ctx, 1, perPage)
			return err
		},
	},
	{
		name:    "Resources.Search",
		message: "error listing resources - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.Resources.Search(ctx, 1, perPage, "doc")
			return err
		},
	},
	{
		name:    "RoleAssignments.List",
		message: "error listing role assignments - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.RoleAssignments.List(ctx, 1, perPage, "", "", "")
			return err
		},
	},
	{
		name:    "Roles.List",
		message: "error listing roles - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.Roles.List(ctx, 1, perPage)
			return err
		},
	},
	{
		name:    "Roles.ListByAttributes",
		message: "error listing roles - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.Roles.ListByAttributes(ctx, 1, perPage, nil)
			return err
		},
	},
	{
		name:    "Tenants.List",
		message: "error listing tenants - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.Tenants.List(ctx, 1, perPage)
			return err
		},
	},
	{
		name:    "Tenants.ListByAttributes",
		message: "error listing tenants - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.Tenants.ListByAttributes(ctx, nil, 1, perPage)
			return err
		},
	},
	{
		name:    "Tenants.ListTenantUsers",
		message: "error listing tenant users - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.Tenants.ListTenantUsers(ctx, "t1", 1, perPage)
			return err
		},
	},
	{
		name:    "Users.GetAssignedRoles",
		message: "error listing assigned roles - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.Users.GetAssignedRoles(ctx, "u1", "t1", 1, perPage)
			return err
		},
	},
	{
		name:    "Users.List",
		message: "error listing users - max per page: 100",
		call: func(ctx context.Context, client *api.PermitApiClient, perPage int) error {
			_, err := client.Users.List(ctx, 1, perPage)
			return err
		},
	},
}

func TestListMethodsLogThePageSizeLimit(t *testing.T) {
	for _, method := range pageSizeListMethods {
		t.Run(method.name, func(t *testing.T) {
			srv := newFakeAPI(t, http.StatusOK, "[]")
			core, logs := observer.New(zapcore.ErrorLevel)
			cfg := newTestConfig(srv.URL).WithPdpUrl(srv.URL).WithLogger(zap.New(core))
			client := newTestClient(cfg)

			err := method.call(context.Background(), client, api.DefaultPerPageLimit+1)

			var permitErr permitErrors.PermitError
			require.ErrorAs(t, err, &permitErr)
			assert.Equal(t, permitErrors.PaginationError, permitErr.ErrorCode)
			assert.Empty(t, srv.recorded())
			limitEntries := logs.FilterMessageSnippet("max per page").All()
			require.Len(t, limitEntries, 1)
			assert.Equal(t, method.message, limitEntries[0].Message)
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
