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

type Resources struct {
	permitBaseApi
}

func NewResourcesApi(client *openapi.APIClient, config *config.PermitConfig) *Resources {
	return &Resources{
		permitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

// List all the resources in the current environment.
// Usage Example:
//
//	`resources, err := PermitClient.Api.Resources.List(ctx, 1, 10)`
func (r *Resources) List(ctx context.Context, page int, perPage int) ([]models.ResourceRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		r.logger.Error("error listing resources - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}
	err := r.lazyLoadPermitContext(ctx)
	if err != nil {
		r.logger.Error("", zap.Error(err))
		return nil, err
	}
	resources, httpRes, err := r.client.ResourcesApi.ListResources(ctx, r.config.Context.GetProject(), r.config.Context.GetEnvironment()).Page(int32(page)).PerPage(int32(perPage)).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		r.logger.Error("error listing resources", zap.Error(err))
		return nil, err
	}
	return resources, nil
}

// Search for resources by key or name.
// Usage Example:
//
//	`resources, err := PermitClient.Api.Resources.List(ctx, 1, 10)`
func (r *Resources) Search(ctx context.Context, page int, perPage int, query string) ([]models.ResourceRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)

	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		r.logger.Error("error listing resources - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}

	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", zap.Error(err))
		return nil, err
	}

	resources, httpRes, err := r.client.ResourcesApi.ListResources(
		ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
	).Page(int32(page)).PerPage(int32(perPage)).Search(query).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error listing resources", zap.Error(err))
		return nil, err
	}

	return resources, nil
}

// Get a resource by its key.
// Usage Example:
//
//	`resource, err := PermitClient.Api.Resources.Get(ctx, "my-resource")`
func (r *Resources) Get(ctx context.Context, resourceKey string) (*models.ResourceRead, error) {
	err := r.lazyLoadPermitContext(ctx)
	if err != nil {
		r.logger.Error("", zap.Error(err))
		return nil, err
	}
	resource, httpRes, err := r.client.ResourcesApi.GetResource(ctx, r.config.Context.GetProject(), r.config.Context.GetEnvironment(), resourceKey).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		r.logger.Error("error getting resource: "+resourceKey, zap.Error(err))
		return nil, err
	}
	return resource, nil
}

// GetByKey gets a resource by its key.
// Usage Example:
//
//	`resource, err := PermitClient.Api.Resources.GetByKey(ctx, "my-resource")`
func (r *Resources) GetByKey(ctx context.Context, resourceKey string) (*models.ResourceRead, error) {
	return r.Get(ctx, resourceKey)
}

// GetById gets a resource by its ID.
// Usage Example:
//
//	`resource, err := PermitClient.Api.Resources.GetById(ctx, uuid.New())`
func (r *Resources) GetById(ctx context.Context, resourceId uuid.UUID) (*models.ResourceRead, error) {
	return r.Get(ctx, resourceId.String())
}

// Create a new resource.
// Usage Example:
// ```
//
//	resourceCreate := models.NewResourceCreate("document", "Document", map[string]models.ActionBlockEditable{"read": {}, "write": {}}
//	resource, err := PermitClient.Api.Resources.Create(ctx, resourceCreate)
//
// ```
func (r *Resources) Create(ctx context.Context, resourceCreate models.ResourceCreate) (*models.ResourceRead, error) {
	err := r.lazyLoadPermitContext(ctx)
	if err != nil {
		r.logger.Error("", zap.Error(err))
		return nil, err
	}
	resource, httpRes, err := r.client.ResourcesApi.CreateResource(ctx, r.config.Context.GetProject(), r.config.Context.GetEnvironment()).ResourceCreate(resourceCreate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		r.logger.Error("error creating resource: "+resourceCreate.GetKey(), zap.Error(err))
		return nil, err
	}
	return resource, nil
}

// Update a resource.
// Usage Example:
// ```
//
//	resourceUpdate := models.NewResourceUpdate()
//	resourceUpdate.SetActions(map[string]models.ActionBlockEditable{"read": {}, "write": {}}
//	resource, err := PermitClient.Api.Resources.Update(ctx, "my-resource", resourceUpdate)
//
// ```
func (r *Resources) Update(ctx context.Context, resourceKey string, resourceUpdate models.ResourceUpdate) (*models.ResourceRead, error) {
	err := r.lazyLoadPermitContext(ctx)
	if err != nil {
		r.logger.Error("", zap.Error(err))
		return nil, err
	}
	resource, httpRes, err := r.client.ResourcesApi.UpdateResource(ctx, r.config.Context.GetProject(), r.config.Context.GetEnvironment(), resourceKey).ResourceUpdate(resourceUpdate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		r.logger.Error("error updating resource: "+resourceKey, zap.Error(err))
		return nil, err
	}
	return resource, nil
}

// Replace a resource.
// Usage Example:
// ```
//
//	resourceReplace := models.NewResourceReplace("Document", map[string]models.ActionBlockEditable{"read": {}, "write": {}}
//	resource, err := PermitClient.Api.Resources.Replace(ctx, "document", resourceReplace)
//
// ```
func (r *Resources) Replace(ctx context.Context, resourceKey string, resourceReplace models.ResourceReplace) (*models.ResourceRead, error) {
	err := r.lazyLoadPermitContext(ctx)
	if err != nil {
		r.logger.Error("", zap.Error(err))
		return nil, err
	}
	resource, httpRes, err := r.client.ResourcesApi.ReplaceResource(ctx, r.config.Context.GetProject(), r.config.Context.GetEnvironment(), resourceKey).ResourceReplace(resourceReplace).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		r.logger.Error("error replacing resource: "+resourceKey, zap.Error(err))
		return nil, err
	}
	return resource, nil
}

// Delete a resource.
// Usage Example:
//
//	`err := PermitClient.Api.Resources.Delete(ctx, "my-resource")`
func (r *Resources) Delete(ctx context.Context, resourceKey string) error {
	err := r.lazyLoadPermitContext(ctx)
	if err != nil {
		r.logger.Error("", zap.Error(err))
		return err
	}
	httpRes, err := r.client.ResourcesApi.DeleteResource(ctx, r.config.Context.GetProject(), r.config.Context.GetEnvironment(), resourceKey).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		r.logger.Error("error deleting resource: "+resourceKey, zap.Error(err))
		return err
	}
	return nil
}
