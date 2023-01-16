package api

import (
	"github.com/permitio/permit-golang/openapi"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/permit"
	"go.uber.org/zap"
	"golang.org/x/net/context"
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

func (t *Tenants) List(ctx context.Context) []openapi.TenantRead {
	tenants, httpRes, err := t.client.TenantsApi.ListTenants(ctx, t.config.Context.ProjectId, t.config.Context.EnvironmentId).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		t.logger.Error("error listing tenants", zap.Error(err))
	}
	return tenants
}
