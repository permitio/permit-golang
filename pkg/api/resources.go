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
//  `resources, err := PermitClient.Api.Resources.List(ctx, 1, 10)`
func (r *Resources) List(ctx context.Context, page int, perPage int) ([]models.ResourceRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		r.logger.Error("error listing resources - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}
	err := r.lazyLoadContext(ctx)
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

func (r *Resources) Get(ctx context.Context, resourceKey string) (*models.ResourceRead, error) {
	err := r.lazyLoadContext(ctx)
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

func (r *Resources) GetByKey(ctx context.Context, resourceKey string) (*models.ResourceRead, error) {
	return r.Get(ctx, resourceKey)
}
func (r *Resources) GetById(ctx context.Context, resourceId uuid.UUID) (*models.ResourceRead, error) {
	return r.Get(ctx, resourceId.String())
}

func (r *Resources) Create(ctx context.Context, resourceCreate models.ResourceCreate) (*models.ResourceRead, error) {
	err := r.lazyLoadContext(ctx)
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

func (r *Resources) Update(ctx context.Context, resourceKey string, resourceUpdate models.ResourceUpdate) (*models.ResourceRead, error) {
	err := r.lazyLoadContext(ctx)
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

func (r *Resources) Delete(ctx context.Context, resourceKey string) error {
	err := r.lazyLoadContext(ctx)
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
