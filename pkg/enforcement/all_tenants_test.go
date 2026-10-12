package enforcement_test

import (
	"net/http"
	"testing"

	"github.com/permitio/permit-golang/pkg/enforcement"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllTenantsCheckReturnsOnlyAllowedTenants(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantTenants []enforcement.TenantDetails
	}{
		{
			name: "mixed decisions",
			body: `{"allowed_tenants":[` +
				`{"allow":true,"tenant":{"key":"t1","attributes":{"tier":"gold"}}},` +
				`{"allow":false,"tenant":{"key":"t2","attributes":{}}},` +
				`{"allow":true,"tenant":{"key":"t3","attributes":{}}}]}`,
			wantTenants: []enforcement.TenantDetails{
				{Key: "t1", Attributes: map[string]interface{}{"tier": "gold"}},
				{Key: "t3", Attributes: map[string]interface{}{}},
			},
		},
		{
			name:        "all denied",
			body:        `{"allowed_tenants":[{"allow":false,"tenant":{"key":"t2","attributes":{}}}]}`,
			wantTenants: []enforcement.TenantDetails{},
		},
		{
			name:        "no tenants",
			body:        `{"allowed_tenants":[]}`,
			wantTenants: []enforcement.TenantDetails{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdp := newFakePDP(t, http.StatusOK, tt.body)
			user := enforcement.UserBuilder("u1").Build()
			resource := enforcement.ResourceBuilder("doc").Build()

			tenants, err := newTestEnforcer(pdp.URL).AllTenantsCheck(user, "read", resource)

			require.NoError(t, err)
			assert.Equal(t, tt.wantTenants, tenants)
		})
	}
}
