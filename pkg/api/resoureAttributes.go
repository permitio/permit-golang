package api

import (
	"context"
	"github.com/google/uuid"
	"github.com/permitio/permit-golang/models"
	"github.com/permitio/permit-golang/openapi"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"go.uber.org/zap"
)

type ResourceAttributes struct {
	permitBaseApi
}

func NewResourceAttributesApi(client *openapi.APIClient, config *config.PermitConfig) *ResourceAttributes {
	return &ResourceAttributes{
		permitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

// List all resource attributes of a resource, by resource key.
// Usage Example:
//  `resourceAttributes, err := PermitClient.Api.ResourceAttributes.List(ctx, "resource-key", 1, 10)`
func (a *ResourceAttributes) List(ctx context.Context, resourceKey string, page int, perPage int) ([]models.ResourceAttributeRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		a.logger.Error("error listing resource attributes - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}
	err := a.lazyLoadContext(ctx)
	if err != nil {
		return nil, err
	}
	resourceAttributes, _, err := a.client.ResourceAttributesApi.ListResourceAttributes(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey).Page(int32(page)).PerPage(int32(perPage)).Execute()
	if err != nil {
		a.logger.Error("error listing resource attributes for resource: "+resourceKey, zap.Error(err))
		return nil, err
	}
	return resourceAttributes, nil
}

func (a *ResourceAttributes) Get(ctx context.Context, resourceKey string, attributeKey string) (*models.ResourceAttributeRead, error) {
	err := a.lazyLoadContext(ctx)
	if err != nil {
		return nil, err
	}
	resourceAttribute, _, err := a.client.ResourceAttributesApi.GetResourceAttribute(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey, attributeKey).Execute()
	if err != nil {
		a.logger.Error("error getting resource attribute: "+resourceKey+":"+attributeKey, zap.Error(err))
		return nil, err
	}
	return resourceAttribute, nil
}

func (a *ResourceAttributes) GetByKey(ctx context.Context, resourceKey string, attributeKey string) (*models.ResourceAttributeRead, error) {
	return a.Get(ctx, resourceKey, attributeKey)
}

func (a *ResourceAttributes) GetById(ctx context.Context, resourceKey uuid.UUID, attributeKey uuid.UUID) (*models.ResourceAttributeRead, error) {
	return a.Get(ctx, resourceKey.String(), attributeKey.String())
}

func (a *ResourceAttributes) Create(ctx context.Context, resourceKey string, resourceAttributeCreate models.ResourceAttributeCreate) (*models.ResourceAttributeRead, error) {
	err := a.lazyLoadContext(ctx)
	if err != nil {
		return nil, err
	}
	resourceAttribute, _, err := a.client.ResourceAttributesApi.CreateResourceAttribute(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey).ResourceAttributeCreate(resourceAttributeCreate).Execute()
	if err != nil {
		a.logger.Error("error creating resource attribute: "+resourceKey+":"+resourceAttributeCreate.GetKey(), zap.Error(err))
		return nil, err
	}
	return resourceAttribute, nil
}

func (a *ResourceAttributes) Update(ctx context.Context, resourceKey string, attributeKey string, resourceAttributeUpdate models.ResourceAttributeUpdate) (*models.ResourceAttributeRead, error) {
	err := a.lazyLoadContext(ctx)
	if err != nil {
		return nil, err
	}
	resourceAttribute, _, err := a.client.ResourceAttributesApi.UpdateResourceAttribute(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey, attributeKey).ResourceAttributeUpdate(resourceAttributeUpdate).Execute()
	if err != nil {
		a.logger.Error("error updating resource attribute: "+resourceKey+":"+attributeKey, zap.Error(err))
		return nil, err
	}
	return resourceAttribute, nil
}

func (a *ResourceAttributes) Delete(ctx context.Context, resourceKey string, attributeKey string) error {
	err := a.lazyLoadContext(ctx)
	if err != nil {
		return err
	}
	_, err = a.client.ResourceAttributesApi.DeleteResourceAttribute(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey, attributeKey).Execute()
	if err != nil {
		a.logger.Error("error deleting resource attribute: "+resourceKey+":"+attributeKey, zap.Error(err))
		return err
	}
	return nil
}
