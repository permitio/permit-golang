package api

import (
	"context"
	"github.com/google/uuid"
	"github.com/permitio/permit-golang/openapi"
	"github.com/permitio/permit-golang/pkg/errors"
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

func (t *Tenants) List(ctx context.Context, page int, perPage int) ([]openapi.TenantRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		t.logger.Error("error listing tenants - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}
	err := t.LazyLoadContext(ctx)
	if err != nil {
		t.logger.Error("", zap.Error(err))
		return nil, err
	}
	tenants, httpRes, err := t.client.TenantsApi.ListTenants(ctx, t.config.Context.ProjectId, t.config.Context.EnvironmentId).Page(int32(page)).PerPage(int32(page)).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		t.logger.Error("error listing tenants", zap.Error(err))
	}
	return tenants, nil
}

func (t *Tenants) Get(ctx context.Context, tenantKey string) (*openapi.TenantRead, error) {
	err := t.LazyLoadContext(ctx)
	if err != nil {
		t.logger.Error("", zap.Error(err))
		return nil, err
	}
	tenant, httpRes, err := t.client.TenantsApi.GetTenant(ctx, t.config.Context.ProjectId, t.config.Context.EnvironmentId, tenantKey).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		t.logger.Error("error getting tenant: "+tenantKey, zap.Error(err))
		return nil, err
	}

	return tenant, nil
}

func (t *Tenants) GetByKey(ctx context.Context, tenantKey string) (*openapi.TenantRead, error) {
	return t.Get(ctx, tenantKey)
}

func (t *Tenants) GetById(ctx context.Context, tenantId uuid.UUID) (*openapi.TenantRead, error) {
	return t.Get(ctx, tenantId.String())
}

func (t *Tenants) Create(ctx context.Context, tenantCreate openapi.TenantCreate) (*openapi.TenantRead, error) {
	err := t.LazyLoadContext(ctx)
	if err != nil {
		t.logger.Error("", zap.Error(err))
		return nil, err
	}
	tenant, httpRes, err := t.client.TenantsApi.CreateTenant(ctx, t.config.Context.ProjectId, t.config.Context.EnvironmentId).TenantCreate(tenantCreate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		t.logger.Error("error creating tenant: "+tenantCreate.GetKey(), zap.Error(err))
		return nil, err
	}
	return tenant, nil
}

func (t *Tenants) Update(ctx context.Context, tenantKey string, tenantUpdate openapi.TenantUpdate) (*openapi.TenantRead, error) {
	err := t.LazyLoadContext(ctx)
	if err != nil {
		t.logger.Error("", zap.Error(err))
		return nil, err
	}
	tenant, httpRes, err := t.client.TenantsApi.UpdateTenant(ctx, t.config.Context.ProjectId, t.config.Context.EnvironmentId, tenantKey).TenantUpdate(tenantUpdate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		t.logger.Error("error updating tenant: "+tenantKey, zap.Error(err))
		return nil, err
	}
	return tenant, nil
}

// Delete
// Deletes a tenant under the context's environment - by a given tenant key.
//
// Usage Example:
// ```
// Permit := permit.NewPermitClient("https://api")
// err := api.Tenants().Delete(ctx, "tenant-key")
// ```
func (t *Tenants) Delete(ctx context.Context, tenantKey string) error {
	err := t.LazyLoadContext(ctx)
	if err != nil {
		t.logger.Error("", zap.Error(err))
		return err
	}
	httpRes, err := t.client.TenantsApi.DeleteTenant(ctx, t.config.Context.ProjectId, t.config.Context.EnvironmentId, tenantKey).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		t.logger.Error("error deleting tenant: "+tenantKey, zap.Error(err))
		return err
	}
	return nil
}
