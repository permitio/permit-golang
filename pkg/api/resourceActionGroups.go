package api

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/openapi"
	"go.uber.org/zap"
)

type ResourceActionGroups struct {
	permitBaseApi
}

func NewResourceActionGroupsApi(client *openapi.APIClient, config *config.PermitConfig) *ResourceActionGroups {
	return &ResourceActionGroups{
		permitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

// List all resource action groups of a resource by resource key.
// Usage Example:
//
//	`actionGroups, err := PermitClient.Api.ResourceActionGroups.List(ctx, "resource-key", 1, 10)`
func (a *ResourceActionGroups) List(ctx context.Context, resourceKey string, page int, perPage int) ([]models.ResourceActionGroupRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		a.logger.Error("error listing resource action groups - max per page: "+fmt.Sprint(perPageLimit), zap.Error(err))
		return nil, err
	}
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return nil, err
	}
	resourceActionGroups, _, err := a.client.ResourceActionGroupsApi.ListResourceActionGroups(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey).Page(int32(page)).PerPage(int32(perPage)).Execute()
	if err != nil {
		a.logGeneratedClientError("error listing resource action groups for resource: "+
			resourceKey, err)
		return nil, err
	}
	return resourceActionGroups, nil
}

// ListByAttributes lists all action groups in the current environment by attributes filter
func (a *ResourceActionGroups) ListByAttributes(ctx context.Context, resourceKey string, page int, perPage int, attributesFilter map[string]interface{}) ([]models.ResourceActionGroupRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		a.logger.Error("error listing resource action groups - max per page: "+fmt.Sprint(perPageLimit), zap.Error(err))
		return nil, err
	}
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		a.logger.Error("", zap.Error(err))
		return nil, err
	}
	actionGroups, httpRes, err := a.client.ResourceActionGroupsApi.ListResourceActionGroups(
		ctx,
		a.config.Context.GetProject(),
		a.config.Context.GetEnvironment(),
		resourceKey,
	).Page(int32(page)).PerPage(int32(perPage)).AttributesFilter(attributesFilter).Execute()

	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		a.logger.Error("error listing action groups", zap.Error(err))
		return nil, err
	}
	return actionGroups, nil
}

// Get a resource action group by resource key and action group key.
// Usage Example:
//
//	`actionGroup, err := PermitClient.Api.ResourceActionGroups.Get(ctx, "resource-key", "action-group-key")`
func (a *ResourceActionGroups) Get(ctx context.Context, resourceKey string, actionKey string) (*models.ResourceActionGroupRead, error) {
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return nil, err
	}
	resourceActionsGroups, _, err := a.client.ResourceActionGroupsApi.GetResourceActionGroup(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey, actionKey).Execute()
	if err != nil {
		a.logGeneratedClientError("error getting resource action: "+resourceKey+":"+actionKey, err)
		return nil, err
	}
	return resourceActionsGroups, nil
}

// GetByKey gets a resource action group by resource key and action group key.
// Usage Example:
//
//	`actionGroup, err := PermitClient.Api.ResourceActionGroups.GetByKey(ctx, "resource-key", "action-group-key")`
func (a *ResourceActionGroups) GetByKey(ctx context.Context, resourceKey string, actionKey string) (*models.ResourceActionGroupRead, error) {
	return a.Get(ctx, resourceKey, actionKey)
}

// GetById gets a resource action group by resource ID and action group ID.
// Usage Example:
//
//	`actionGroup, err := PermitClient.Api.ResourceActionGroups.GetById(ctx, uuid.New(), uuid.New())`
func (a *ResourceActionGroups) GetById(ctx context.Context, resourceKey uuid.UUID, actionKey uuid.UUID) (*models.ResourceActionGroupRead, error) {
	return a.Get(ctx, resourceKey.String(), actionKey.String())
}

// Create a resource action group by resource key.
// Usage Example:
// ```
//
//	actionGroupCreate := models.NewResourceActionGroupCreate("action-group-key", "action-group-name")
//	actionGroup, err := PermitClient.Api.ResourceActionGroups.Create(ctx, "resource-key", *actionGroupCreate)
//
// ```
func (a *ResourceActionGroups) Create(ctx context.Context, resourceKey string, resourceActionCreate models.ResourceActionGroupCreate) (*models.ResourceActionGroupRead, error) {
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return nil, err
	}
	resourceActionGroup, _, err := a.client.ResourceActionGroupsApi.CreateResourceActionGroup(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey).ResourceActionGroupCreate(resourceActionCreate).Execute()
	if err != nil {
		a.logGeneratedClientError("error creating resource action: "+resourceKey+":"+
			resourceActionCreate.GetKey(), err)
		return nil, err
	}
	return resourceActionGroup, nil
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
//func (a *ResourceActionGroups) Update(ctx context.Context, resourceKey string, actionKey string, resourceActionUpdate models.ResourceActionGroupUpdate) (*models.ResourceActionRead, error) {
//	err := a.lazyLoadPermitContext(ctx)
//	if err != nil {
//		return nil, err
//	}
//	resourceAction, _, err := a.client.ResourceActionsApi.UpdateResourceAction(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey, actionKey).ResourceActionUpdate(resourceActionUpdate).Execute()
//	if err != nil {
//		a.logger.Error("error updating resource action: "+resourceKey+":"+actionKey, zap.Error(err))
//		return nil, err
//	}
//	return resourceAction, nil
//}

// Delete a resource action group by resource key and action group key.
// Usage Example:
//
//	`err := PermitClient.Api.ResourceActionGroups.Delete(ctx, "resource-key", "action-group-key")`
func (a *ResourceActionGroups) Delete(ctx context.Context, resourceKey string, actionKey string) error {
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return err
	}
	_, err = a.client.ResourceActionGroupsApi.DeleteResourceActionGroup(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), resourceKey, actionKey).Execute()
	if err != nil {
		a.logGeneratedClientError("error deleting resource action group: "+resourceKey+":"+
			actionKey, err)
		return err
	}
	return nil
}
