package api

import (
	"context"
	"github.com/google/uuid"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/openapi"
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
//
//	`resourceAttributes, err := PermitClient.Api.ResourceAttributes.List(ctx, "resource-key", 1, 10)`
func (a *ResourceAttributes) List(ctx context.Context, resourceKey string, page int, perPage int) ([]models.ResourceAttributeRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		a.logger.Error("error listing resource attributes - max per page: "+string(perPageLimit), err)
		return nil, err
	}
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return nil, err
	}
	resourceAttributes, _, err := a.client.ResourceAttributesApi.ListResourceAttributes(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey).Page(int32(page)).PerPage(int32(perPage)).Execute()
	if err != nil {
		a.logger.Error("error listing resource attributes for resource: "+resourceKey, err)
		return nil, err
	}
	return resourceAttributes, nil
}

// Get a resource attribute by resource key and attribute key.
// Usage Example:
//
//	`resourceAttribute, err := PermitClient.Api.ResourceAttributes.Get(ctx, "resource-key", "attribute-key")`
func (a *ResourceAttributes) Get(ctx context.Context, resourceKey string, attributeKey string) (*models.ResourceAttributeRead, error) {
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return nil, err
	}
	resourceAttribute, _, err := a.client.ResourceAttributesApi.GetResourceAttribute(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey, attributeKey).Execute()
	if err != nil {
		a.logger.Error("error getting resource attribute: "+resourceKey+":"+attributeKey, err)
		return nil, err
	}
	return resourceAttribute, nil
}

// GetByKey gets a resource attribute by resource key and attribute key.
// Usage Example:
//
//	`resourceAttribute, err := PermitClient.Api.ResourceAttributes.GetByKey(ctx, "resource-key", "attribute-key")`
func (a *ResourceAttributes) GetByKey(ctx context.Context, resourceKey string, attributeKey string) (*models.ResourceAttributeRead, error) {
	return a.Get(ctx, resourceKey, attributeKey)
}

// GetById gets a resource attribute by resource ID and attribute ID.
// Usage Example:
//
//	`resourceAttribute, err := PermitClient.Api.ResourceAttributes.GetById(ctx, uuid.New(), uuid.New())`
func (a *ResourceAttributes) GetById(ctx context.Context, resourceKey uuid.UUID, attributeKey uuid.UUID) (*models.ResourceAttributeRead, error) {
	return a.Get(ctx, resourceKey.String(), attributeKey.String())
}

// Create a resource attribute by resource key.
// Usage Example:
// ```
//
//	resourceAttributeCreate := models.NewResourceAttributeCreate("attribute-key", models.AttributeType("string"))
//	resourceAttribute, err := PermitClient.Api.ResourceAttributes.Create(ctx, "resource-key", resourceAttributeCreate)
//
// ```
func (a *ResourceAttributes) Create(ctx context.Context, resourceKey string, resourceAttributeCreate models.ResourceAttributeCreate) (*models.ResourceAttributeRead, error) {
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return nil, err
	}
	resourceAttribute, _, err := a.client.ResourceAttributesApi.CreateResourceAttribute(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey).ResourceAttributeCreate(resourceAttributeCreate).Execute()
	if err != nil {
		a.logger.Error("error creating resource attribute: "+resourceKey+":"+resourceAttributeCreate.GetKey(), err)
		return nil, err
	}
	return resourceAttribute, nil
}

// Update a resource attribute by resource key and attribute key.
// Usage Example:
// ```
//
//	resourceAttributeUpdate := models.NewResourceAttributeUpdate()
//	resourceAttributeUpdate.SetType("bool")
//	resourceAttribute, err := PermitClient.Api.ResourceAttributes.Update(ctx, "resource-key", "attribute-key", resourceAttributeUpdate)
//
// ```
func (a *ResourceAttributes) Update(ctx context.Context, resourceKey string, attributeKey string, resourceAttributeUpdate models.ResourceAttributeUpdate) (*models.ResourceAttributeRead, error) {
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return nil, err
	}
	resourceAttribute, _, err := a.client.ResourceAttributesApi.UpdateResourceAttribute(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey, attributeKey).ResourceAttributeUpdate(resourceAttributeUpdate).Execute()
	if err != nil {
		a.logger.Error("error updating resource attribute: "+resourceKey+":"+attributeKey, err)
		return nil, err
	}
	return resourceAttribute, nil
}

// Delete a resource attribute by resource key and attribute key.
// Usage Example:
//
//	`err := PermitClient.Api.ResourceAttributes.Delete(ctx, "resource-key", "attribute-key")`
func (a *ResourceAttributes) Delete(ctx context.Context, resourceKey string, attributeKey string) error {
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return err
	}
	_, err = a.client.ResourceAttributesApi.DeleteResourceAttribute(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey, attributeKey).Execute()
	if err != nil {
		a.logger.Error("error deleting resource attribute: "+resourceKey+":"+attributeKey, err)
		return err
	}
	return nil
}
