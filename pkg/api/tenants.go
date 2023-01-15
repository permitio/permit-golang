package api

import (
	"github.com/permitio/permit-golang/openapi"
	"github.com/permitio/permit-golang/pkg/permit"
	"go.uber.org/zap"
)

type Tenants struct {
	PermitBaseApi
}

func NewTenantsApi(client *openapi.APIClient, config *permit.PermitConfig) *Tenants {
	return &Tenants{
		PermitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

func (t *Tenants) List() []openapi.TenantRead {
	tenants, _, err := t.client.TenantsApi.ListTenants(ctx, t.config.context)
	if err != nil {
		t.logger.Error("error listing tenants", zap.Error(err))
	}
	return tenants
}
