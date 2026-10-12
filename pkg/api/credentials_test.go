package api_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/permitio/permit-golang/pkg/api"
	"github.com/permitio/permit-golang/pkg/config"
	permitErrors "github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/openapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const credentialsTestKey = "configured-key-3b7e15"

// credentialValues are the API key and the values that the error bodies below put under
// credential keys; none of them may reach an error or a log line.
var credentialValues = []string{
	credentialsTestKey,
	"permit_key_FAKEechoed12345",
	"proxy-secret-value",
	"bearer-token-value",
	"nested-password-value",
	"header-secret-value",
}

const credentialFields = `"secret":"proxy-secret-value","token":"bearer-token-value",` +
	`"auth":{"password":"nested-password-value"}`

func newObservedClient(t *testing.T, apiURL string) (*api.PermitApiClient, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	cfg := config.NewConfigBuilder(credentialsTestKey).
		WithApiUrl(apiURL).
		WithContext(config.NewPermitContext(config.EnvironmentAPIKeyLevel, "proj", "env")).
		WithLogger(zap.New(core)).
		Build()
	return api.NewPermitApiClient(&cfg), logs
}

func TestAPIErrorsAndLogsHoldNoCredentials(t *testing.T) {
	ctx := context.Background()
	getUser := func(client *api.PermitApiClient) error {
		_, err := client.Users.Get(ctx, "u1")
		return err
	}
	tests := []struct {
		name      string
		status    int
		body      string
		call      func(client *api.PermitApiClient) error
		wantCode  permitErrors.ErrorCode
		wantParts []string
	}{
		{
			name:   "422",
			status: http.StatusUnprocessableEntity,
			body: `{"detail":[{"loc":["body","key"],"msg":"invalid key ` + credentialsTestKey +
				` or permit_key_FAKEechoed12345","type":"value_error"}],` + credentialFields + `}`,
			call:     getUser,
			wantCode: permitErrors.UnprocessableEntityError,
			wantParts: []string{
				`"loc":["body","key"]`,
				`"msg":"invalid key [REDACTED] or [REDACTED]"`,
				`"type":"value_error"`,
				`"auth":{"password":"[REDACTED]"}`,
				`"secret":"[REDACTED]"`,
			},
		},
		{
			name:   "500",
			status: http.StatusInternalServerError,
			body: `{"detail":"failed for ` + credentialsTestKey +
				` and permit_key_FAKEechoed12345",` + credentialFields + `}`,
			call:     getUser,
			wantCode: permitErrors.UnexpectedError,
			wantParts: []string{
				`"detail":"failed for [REDACTED] and [REDACTED]"`,
				`"token":"[REDACTED]"`,
			},
		},
		{
			name:   "proxy config whose secret does not decode",
			status: http.StatusOK,
			body: `{"key":"pc1","id":"p1","organization_id":"org","project_id":"proj",` +
				`"environment_id":"env","created_at":"2026-01-01T00:00:00Z",` +
				`"updated_at":"2026-01-01T00:00:00Z",` +
				`"secret":{"X-Api-Key":"header-secret-value"},` +
				`"name":"Proxy","auth_mechanism":"Headers"}`,
			call: func(client *api.PermitApiClient) error {
				_, err := client.ProxyConfigs.Get(ctx, "pc1")
				return err
			},
			wantCode: permitErrors.UnexpectedError,
			wantParts: []string{
				`"key":"pc1"`,
				`"name":"Proxy"`,
				`"secret":{"X-Api-Key":"[REDACTED]"}`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newFakeAPI(t, tt.status, tt.body)
			client, logs := newObservedClient(t, srv.URL)

			err := tt.call(client)

			var permitErr permitErrors.PermitError
			require.True(t, errors.As(err, &permitErr), "want a PermitError, got %T: %v", err, err)
			assert.Equal(t, tt.wantCode, permitErr.ErrorCode)
			assert.Equal(t, tt.status, permitErr.StatusCode)
			for _, part := range tt.wantParts {
				assert.Contains(t, permitErr.ResponseBody, part)
			}
			require.NotEmpty(t, logs.All())
			for _, value := range credentialValues {
				assert.NotContains(t, permitErr.ResponseBody, value, "ResponseBody")
				assert.NotContains(t, permitErr.Error(), value, "Error()")
				for _, entry := range logs.All() {
					line := fmt.Sprint(entry.Message, " ", entry.ContextMap())
					assert.NotContains(t, line, value, "%s log line", entry.Level)
				}
			}
			requests := srv.recorded()
			require.Len(t, requests, 1)
			assert.Equal(t, "Bearer "+credentialsTestKey, requests[0].header.Get("Authorization"))
		})
	}
}

// ResourceActions, ResourceActionGroups and ResourceAttributes return the generated client's
// error as it is; the SDK's own log line for it must still hold no credential.
func TestGeneratedClientErrorLogLinesHoldNoCredentials(t *testing.T) {
	ctx := context.Background()
	actionCreate := *models.NewResourceActionCreate("read", "Read")
	groupCreate := *models.NewResourceActionGroupCreate("readers", "Readers")
	attributeCreate := *models.NewResourceAttributeCreate("owner", models.AttributeType("string"))
	calls := map[string]func(client *api.PermitApiClient) error{
		"ResourceActions.List": func(client *api.PermitApiClient) error {
			_, err := client.ResourceActions.List(ctx, "doc", 1, 10)
			return err
		},
		"ResourceActions.Get": func(client *api.PermitApiClient) error {
			_, err := client.ResourceActions.Get(ctx, "doc", "read")
			return err
		},
		"ResourceActions.Create": func(client *api.PermitApiClient) error {
			_, err := client.ResourceActions.Create(ctx, "doc", actionCreate)
			return err
		},
		"ResourceActions.Update": func(client *api.PermitApiClient) error {
			_, err := client.ResourceActions.Update(ctx, "doc", "read",
				*models.NewResourceActionUpdate())
			return err
		},
		"ResourceActions.Delete": func(client *api.PermitApiClient) error {
			return client.ResourceActions.Delete(ctx, "doc", "read")
		},
		"ResourceActionGroups.List": func(client *api.PermitApiClient) error {
			_, err := client.ResourceActionGroups.List(ctx, "doc", 1, 10)
			return err
		},
		"ResourceActionGroups.Get": func(client *api.PermitApiClient) error {
			_, err := client.ResourceActionGroups.Get(ctx, "doc", "readers")
			return err
		},
		"ResourceActionGroups.Create": func(client *api.PermitApiClient) error {
			_, err := client.ResourceActionGroups.Create(ctx, "doc", groupCreate)
			return err
		},
		"ResourceActionGroups.Delete": func(client *api.PermitApiClient) error {
			return client.ResourceActionGroups.Delete(ctx, "doc", "readers")
		},
		"ResourceAttributes.List": func(client *api.PermitApiClient) error {
			_, err := client.ResourceAttributes.List(ctx, "doc", 1, 10)
			return err
		},
		"ResourceAttributes.Get": func(client *api.PermitApiClient) error {
			_, err := client.ResourceAttributes.Get(ctx, "doc", "owner")
			return err
		},
		"ResourceAttributes.Create": func(client *api.PermitApiClient) error {
			_, err := client.ResourceAttributes.Create(ctx, "doc", attributeCreate)
			return err
		},
		"ResourceAttributes.Update": func(client *api.PermitApiClient) error {
			_, err := client.ResourceAttributes.Update(ctx, "doc", "owner",
				*models.NewResourceAttributeUpdate())
			return err
		},
		"ResourceAttributes.Delete": func(client *api.PermitApiClient) error {
			return client.ResourceAttributes.Delete(ctx, "doc", "owner")
		},
	}
	body := `{"detail":[{"loc":["body","key"],"msg":"invalid key ` + credentialsTestKey +
		` or permit_key_FAKEechoed12345","type":"value_error"}]}`
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			srv := newFakeAPI(t, http.StatusUnprocessableEntity, body)
			client, logs := newObservedClient(t, srv.URL)

			err := call(client)

			var generatedErr *openapi.GenericOpenAPIError
			require.True(t, errors.As(err, &generatedErr),
				"want the generated client's error, got %T: %v", err, err)
			require.Len(t, logs.All(), 1)
			entry := logs.All()[0]
			line := fmt.Sprint(entry.Message, " ", entry.ContextMap())
			assert.Contains(t, line, "invalid key [REDACTED] or [REDACTED]")
			for _, value := range credentialValues {
				assert.NotContains(t, line, value, "%s log line", entry.Level)
			}
		})
	}
}
