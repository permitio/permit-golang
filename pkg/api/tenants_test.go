package api_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/permitio/permit-golang/pkg/api"
	permitErrors "github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const tenantJSON = `{"key":"t1","name":"Tenant 1","id":"tid",` +
	`"organization_id":"org","project_id":"proj","environment_id":"env",` +
	`"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z",` +
	`"last_action_at":"2026-01-01T00:00:00Z"}`

var tenantListMethods = []struct {
	name  string
	call  func(context.Context, *api.Tenants) ([]models.TenantRead, error)
	query url.Values
}{
	{
		name: "List",
		call: func(ctx context.Context, tenants *api.Tenants) ([]models.TenantRead, error) {
			return tenants.List(ctx, 2, 5)
		},
		query: url.Values{"page": {"2"}, "per_page": {"5"}},
	},
	{
		name: "ListByAttributes",
		call: func(ctx context.Context, tenants *api.Tenants) ([]models.TenantRead, error) {
			attributes := map[string]interface{}{"plan": "pro"}
			return tenants.ListByAttributes(ctx, attributes, 2, 5)
		},
		query: url.Values{"attr_plan": {"pro"}, "page": {"2"}, "per_page": {"5"}},
	},
}

// newTenantsClient returns a client whose API and PDP URLs both point at serverURL.
func newTenantsClient(serverURL string) *api.PermitApiClient {
	return newTestClient(newTestConfig(serverURL).WithPdpUrl(serverURL))
}

func TestTenantsListMethodsReturnTenants(t *testing.T) {
	for _, method := range tenantListMethods {
		t.Run(method.name, func(t *testing.T) {
			srv := newFakeAPI(t, http.StatusOK, "["+tenantJSON+"]")
			client := newTenantsClient(srv.URL)

			tenants, err := method.call(context.Background(), client.Tenants)

			require.NoError(t, err)
			keys := []string{}
			for _, tenant := range tenants {
				keys = append(keys, tenant.GetKey())
			}
			assert.Equal(t, []string{"t1"}, keys)
			requests := srv.recorded()
			require.Len(t, requests, 1)
			assert.Equal(t, "/v2/facts/proj/env/tenants", requests[0].path)
			assert.Equal(t, method.query, requests[0].query)
		})
	}
}

func TestTenantsListMethodsReturnRequestErrors(t *testing.T) {
	const failedBody = `{"detail":"request failed"}`
	tests := []struct {
		name     string
		status   int
		body     string
		wantCode permitErrors.ErrorCode
	}{
		{name: "401", status: http.StatusUnauthorized, body: failedBody,
			wantCode: permitErrors.Unauthorized},
		{name: "403", status: http.StatusForbidden, body: failedBody,
			wantCode: permitErrors.ForbiddenAccess},
		{name: "404", status: http.StatusNotFound, body: failedBody,
			wantCode: permitErrors.NotFound},
		{name: "429", status: http.StatusTooManyRequests, body: failedBody,
			wantCode: permitErrors.UnexpectedError},
		{name: "500", status: http.StatusInternalServerError, body: failedBody,
			wantCode: permitErrors.UnexpectedError},
		{name: "200 with a body that is not a list", status: http.StatusOK,
			body: `{"detail":"not a list"}`, wantCode: permitErrors.UnexpectedError},
		{name: "200 with a list entry that does not decode", status: http.StatusOK,
			body: "[" + tenantJSON + `,{"key":5}]`, wantCode: permitErrors.UnexpectedError},
	}
	for _, method := range tenantListMethods {
		for _, tt := range tests {
			t.Run(method.name+"/"+tt.name, func(t *testing.T) {
				srv := newFakeAPI(t, tt.status, tt.body)
				client := newTenantsClient(srv.URL)

				tenants, err := method.call(context.Background(), client.Tenants)

				assert.Nil(t, tenants)
				var permitErr permitErrors.PermitError
				require.True(t, errors.As(err, &permitErr),
					"want a PermitError, got %T: %v", err, err)
				assert.Equal(t, tt.wantCode, permitErr.ErrorCode)
				assert.Equal(t, tt.status, permitErr.StatusCode)
				assert.Len(t, srv.recorded(), 1)
			})
		}
	}
}

func TestTenantsListMethodsReturnConnectionErrors(t *testing.T) {
	for _, method := range tenantListMethods {
		t.Run(method.name, func(t *testing.T) {
			srv := httptest.NewServer(http.NotFoundHandler())
			srv.Close()
			client := newTenantsClient(srv.URL)

			tenants, err := method.call(context.Background(), client.Tenants)

			assert.Nil(t, tenants)
			var opErr *net.OpError
			require.True(t, errors.As(err, &opErr), "want a *net.OpError, got %T: %v", err, err)
			assert.Equal(t, "dial", opErr.Op)
		})
	}
}

func TestTenantsListMethodsReturnCanceledContextErrors(t *testing.T) {
	for _, method := range tenantListMethods {
		t.Run(method.name, func(t *testing.T) {
			srv := newFakeAPI(t, http.StatusOK, "["+tenantJSON+"]")
			client := newTenantsClient(srv.URL)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			tenants, err := method.call(ctx, client.Tenants)

			assert.Nil(t, tenants)
			assert.ErrorIs(t, err, context.Canceled)
		})
	}
}
