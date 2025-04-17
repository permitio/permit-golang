package api

import (
	"context"

	"time"

	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/openapi"
	"go.uber.org/zap"
)

type ResourceInstances struct {
	PermitBaseFactsApi
}

func NewResourceInstancesApi(client *openapi.APIClient, config *config.PermitConfig) *ResourceInstances {
	return &ResourceInstances{
		PermitBaseFactsApi{
			permitBaseApi{
				client: client,
				config: config,
				logger: config.Logger,
			},
		},
	}
}

// WaitForSync configures the client to wait for facts synchronization.
//
// Parameters:
//   - timeout: Optional duration to wait for synchronization.
//   - options: Additional configuration options for facts synchronization
func (r *ResourceInstances) WaitForSync(timeout *time.Duration, options WaitForSyncOptions) *ResourceInstances {
	return NewResourceInstancesApi(r.PermitBaseFactsApi.WaitForSync(timeout, options).client, r.config)
}

func (r *ResourceInstances) Create(
	ctx context.Context,
	resourceInstanceCreate models.ResourceInstanceCreate,
) (*models.ResourceInstanceRead, error) {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", zap.Error(err))
		return nil, err
	}

	created, httpRes, err := r.client.ResourceInstancesApi.CreateResourceInstance(ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
	).ResourceInstanceCreate(resourceInstanceCreate).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error creating resource instance", zap.Error(err))
		return nil, err
	}

	r.logger.Debug("resource instance created",
		zap.String("type", "resource_instance"),
		zap.String("resource", created.GetResource()),
		zap.String("key", created.GetKey()),
		zap.String("id", created.Id),
	)

	return created, nil
}

func (r *ResourceInstances) Delete(
	ctx context.Context,
	instanceId string,
) error {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", zap.Error(err))
		return err
	}

	httpRes, err := r.client.ResourceInstancesApi.DeleteResourceInstance(ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		instanceId,
	).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error deleting resource instance", zap.Error(err))
		return err
	}

	return nil
}

func (r *ResourceInstances) Get(
	ctx context.Context,
	instanceId string,
) (*models.ResourceInstanceRead, error) {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", zap.Error(err))
		return nil, err
	}

	retrieved, httpRes, err := r.client.ResourceInstancesApi.GetResourceInstance(ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		instanceId,
	).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error deleting resource instance", zap.Error(err))
		return nil, err
	}

	return retrieved, nil
}

func (r *ResourceInstances) List(
	ctx context.Context,
	page int,
	perPage int,
	tenantFilter string,
	resourceFilter string,
	keyFilter string,
) (*[]models.ResourceInstanceRead, error) {
	return r.list(ctx, page, perPage, tenantFilter, resourceFilter, keyFilter, false)
}

func (r *ResourceInstances) ListDetailed(
	ctx context.Context,
	page int,
	perPage int,
	tenantFilter string,
	resourceFilter string,
	keyFilter string,
) (*[]models.ResourceInstanceRead, error) {
	return r.list(ctx, page, perPage, tenantFilter, resourceFilter, keyFilter, true)
}

func (r *ResourceInstances) list(ctx context.Context,
	page int,
	perPage int,
	tenantFilter string,
	resourceFilter string,
	keyFilter string,
	detailed bool,
) (*[]models.ResourceInstanceRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)

	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		r.logger.Error("error listing relationship tuples - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}

	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", zap.Error(err))
		return nil, err
	}

	request := r.client.ResourceInstancesApi.ListResourceInstances(
		ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
	).Page(int32(page)).PerPage(int32(perPage))

	if tenantFilter != "" {
		request = request.Tenant(tenantFilter)
	}

	if resourceFilter != "" {
		request = request.Resource(resourceFilter)
	}

	if keyFilter != "" {
		request = request.Search(keyFilter)
	}

	if detailed {
		request = request.Detailed(detailed)
	}

	relTuples, httpRes, err := request.Execute()

	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		r.logger.Error("error listing resource instances", zap.Error(err))
		return nil, err
	}

	return &relTuples, nil
}

func (r *ResourceInstances) Update(
	ctx context.Context,
	instanceId string,
	resourceInstanceUpdate models.ResourceInstanceUpdate,
) (*models.ResourceInstanceRead, error) {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", zap.Error(err))
		return nil, err
	}

	updated, httpRes, err := r.client.ResourceInstancesApi.UpdateResourceInstance(
		ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		instanceId,
	).ResourceInstanceUpdate(resourceInstanceUpdate).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error updating resource instance", zap.Error(err))
		return nil, err
	}

	return updated, nil
}
