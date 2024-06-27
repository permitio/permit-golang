package api

import (
	"context"
	"github.com/google/uuid"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/openapi"
	"go.uber.org/zap"
	"time"
)

type Tenants struct {
	PermitBaseFactsApi
}

func NewTenantsApi(client *openapi.APIClient, config *config.PermitConfig) *Tenants {
	return &Tenants{
		PermitBaseFactsApi{
			permitBaseApi{
				client: client,
				config: config,
				logger: config.Logger,
			},
		},
	}
}

func (t *Tenants) WaitForSync(timeout *time.Duration) *Tenants {
	return NewTenantsApi(t.PermitBaseFactsApi.WaitForSync(timeout).client, t.config)
}

// List all tenants under the context's environment.
// Usage Example:
// `tenants, err := PermitClient.Api.Tenants.List(ctx, 1, 10)`
func (t *Tenants) List(ctx context.Context, page int, perPage int) ([]models.TenantRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		t.logger.Error("error listing tenants - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}
	err := t.lazyLoadPermitContext(ctx)
	if err != nil {
		t.logger.Error("", zap.Error(err))
		return nil, err
	}
	tenants, httpRes, err := t.client.TenantsApi.ListTenants(ctx, t.config.Context.ProjectId, t.config.Context.EnvironmentId).Page(int32(page)).PerPage(int32(perPage)).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		t.logger.Error("error listing tenants", zap.Error(err))
	}
	return tenants, nil
}

// ListTenantsByAttributes gets tenants under the context's environment - by a given attributes.
// Usage Example:
// `tenants, err := PermitClient.Api.Tenants.ListTenantsByAttributes(ctx, map[string]string{"key": "value"})`
func (t *Tenants) ListByAttributes(ctx context.Context, attributes map[string]interface{}, page int, perPage int) ([]models.TenantRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		t.logger.Error("error listing tenants - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}
	err := t.lazyLoadPermitContext(ctx)
	if err != nil {
		t.logger.Error("", zap.Error(err))
		return nil, err
	}
	tenants, httpRes, err := t.client.TenantsApi.ListTenants(ctx, t.config.Context.ProjectId, t.config.Context.EnvironmentId).Page(int32(page)).PerPage(int32(perPage)).AttributeFilter(attributes).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		t.logger.Error("error listing tenants", zap.Error(err))
	}
	return tenants, nil

}

// Get a tenant under the context's environment - by a given tenant key.
// Usage Example:
// `tenant, err := PermitClient.Api.Tenants.Get(ctx, "tenant-key")`
func (t *Tenants) Get(ctx context.Context, tenantKey string) (*models.TenantRead, error) {
	err := t.lazyLoadPermitContext(ctx)
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

// GetByKey gets a tenant under the context's environment - by a given tenant key.
// Usage Example:
// `tenant, err := PermitClient.Api.Tenants.GetByKey(ctx, "tenant-key")`
func (t *Tenants) GetByKey(ctx context.Context, tenantKey string) (*models.TenantRead, error) {
	return t.Get(ctx, tenantKey)
}

// GetById gets a tenant under the context's environment - by a given tenant id.
// Usage Example:
// `tenant, err := PermitClient.Api.Tenants.GetById(ctx, uuid.New())`
func (t *Tenants) GetById(ctx context.Context, tenantId uuid.UUID) (*models.TenantRead, error) {
	return t.Get(ctx, tenantId.String())
}

// Create a new tenant under the context's environment.
// Usage Example:
// ```
// tenantCreate := models.NewTenantCreate("tenant-key", "tenant-name")
// tenant, err := PermitClient.Api.Tenants.Create(ctx, tenantCreate)
// ```
func (t *Tenants) Create(ctx context.Context, tenantCreate models.TenantCreate) (*models.TenantRead, error) {
	err := t.lazyLoadPermitContext(ctx)
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

	t.logger.Debug("tenant created",
		zap.String("type", "tenant"),
		zap.String("key", tenant.GetKey()),
		zap.String("id", tenant.Id),
	)

	return tenant, nil
}

// Update a tenant under the context's environment.
// Usage Example:
// ```
// tenantUpdate := models.NewTenantUpdate()
// tenantUpdate.SetName("new-tenant-name")
// tenant, err := PermitClient.Api.Tenants.Update(ctx, "tenant-key", tenantUpdate)
// ```
func (t *Tenants) Update(ctx context.Context, tenantKey string, tenantUpdate models.TenantUpdate) (*models.TenantRead, error) {
	err := t.lazyLoadPermitContext(ctx)
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

// Delete a tenant under the context's environment.
// Usage Example:
// `err := PermitClient.Api.Tenants.Delete(ctx, "tenant-key")`
func (t *Tenants) Delete(ctx context.Context, tenantKey string) error {
	err := t.lazyLoadPermitContext(ctx)
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
