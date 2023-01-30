package api

import (
	"context"
	"github.com/google/uuid"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/openapi"
	"go.uber.org/zap"
)

type ResourceActions struct {
	permitBaseApi
}

func NewResourceActionsApi(client *openapi.APIClient, config *config.PermitConfig) *ResourceActions {
	return &ResourceActions{
		permitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

// List all resource actions of a resource by resource key.
// Usage Example:
//
//	`resourceActions, err := PermitClient.Api.ResourceActions.List(ctx, "resource-key", 1, 10)`
func (a *ResourceActions) List(ctx context.Context, resourceKey string, page int, perPage int) ([]models.ResourceActionRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		a.logger.Error("error listing resource actions - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return nil, err
	}
	resourceActions, _, err := a.client.ResourceActionsApi.ListResourceActions(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey).Page(int32(page)).PerPage(int32(perPage)).Execute()
	if err != nil {
		a.logger.Error("error listing resource actions for resource: "+resourceKey, zap.Error(err))
		return nil, err
	}
	return resourceActions, nil
}

// Get a resource action by resource key and action key.
// Usage Example:
//
//	`resourceAction, err := PermitClient.Api.ResourceActions.Get(ctx, "resource-key", "action-key")`
func (a *ResourceActions) Get(ctx context.Context, resourceKey string, actionKey string) (*models.ResourceActionRead, error) {
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return nil, err
	}
	resourceActions, _, err := a.client.ResourceActionsApi.GetResourceAction(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey, actionKey).Execute()
	if err != nil {
		a.logger.Error("error getting resource action: "+resourceKey+":"+actionKey, zap.Error(err))
		return nil, err
	}
	return resourceActions, nil
}

// GetByKey gets a resource action by resource key and action key.
// Usage Example:
//
//	`resourceAction, err := PermitClient.Api.ResourceActions.GetByKey(ctx, "resource-key", "action-key")`
func (a *ResourceActions) GetByKey(ctx context.Context, resourceKey string, actionKey string) (*models.ResourceActionRead, error) {
	return a.Get(ctx, resourceKey, actionKey)
}

// GetById gets a resource action by resource ID and action ID.
// Usage Example:
//
//	`resourceAction, err := PermitClient.Api.ResourceActions.GetById(ctx, uuid.New(), uuid.New())`
func (a *ResourceActions) GetById(ctx context.Context, resourceKey uuid.UUID, actionKey uuid.UUID) (*models.ResourceActionRead, error) {
	return a.Get(ctx, resourceKey.String(), actionKey.String())
}

// Create a resource action by resource key.
// Usage Example:
// ```
//
//	resourceActionCreate := models.NewResourceActionCreate("action-key", "action-name")
//	resourceAction, err := PermitClient.Api.ResourceActions.Create(ctx, "resource-key", resourceActionCreate)
//
// ```
func (a *ResourceActions) Create(ctx context.Context, resourceKey string, resourceActionCreate models.ResourceActionCreate) (*models.ResourceActionRead, error) {
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return nil, err
	}
	resourceAction, _, err := a.client.ResourceActionsApi.CreateResourceAction(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey).ResourceActionCreate(resourceActionCreate).Execute()
	if err != nil {
		a.logger.Error("error creating resource action: "+resourceKey+":"+resourceActionCreate.GetKey(), zap.Error(err))
		return nil, err
	}
	return resourceAction, nil
}

// Update a resource action by resource key and action key.
// Usage Example:
// ```
//
//	resourceActionUpdate := models.NewResourceActionUpdate()
//	resourceActionUpdate.SetName("new-action-name")
//	resourceAction, err := PermitClient.Api.ResourceActions.Update(ctx, "resource-key", "action-key", resourceActionUpdate)
//
// ```
func (a *ResourceActions) Update(ctx context.Context, resourceKey string, actionKey string, resourceActionUpdate models.ResourceActionUpdate) (*models.ResourceActionRead, error) {
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return nil, err
	}
	resourceAction, _, err := a.client.ResourceActionsApi.UpdateResourceAction(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey, actionKey).ResourceActionUpdate(resourceActionUpdate).Execute()
	if err != nil {
		a.logger.Error("error updating resource action: "+resourceKey+":"+actionKey, zap.Error(err))
		return nil, err
	}
	return resourceAction, nil
}

// Delete a resource action by resource key and action key.
// Usage Example:
//
//	`err := PermitClient.Api.ResourceActions.Delete(ctx, "resource-key", "action-key")`
func (a *ResourceActions) Delete(ctx context.Context, resourceKey string, actionKey string) error {
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return err
	}
	_, err = a.client.ResourceActionsApi.DeleteResourceAction(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey, actionKey).Execute()
	if err != nil {
		a.logger.Error("error deleting resource action: "+resourceKey+":"+actionKey, zap.Error(err))
		return err
	}
	return nil
}
