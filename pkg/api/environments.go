package api

import (
	"github.com/google/uuid"
	"github.com/permitio/permit-golang/models"
	"github.com/permitio/permit-golang/openapi"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

type Environments struct {
	permitBaseApi
}

func NewEnvironmentsApi(client *openapi.APIClient, config *config.PermitConfig) *Environments {
	return &Environments{
		permitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

// List the environments in the project of your context
// Usage Example:
// 	`environments, err := PermitClient.Api.Environments.List(ctx, 1, 10)`
func (e *Environments) List(ctx context.Context, page int, perPage int) ([]models.EnvironmentRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		e.logger.Error("error listing environments - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}
	err := e.lazyLoadContext(ctx)
	if err != nil {
		e.logger.Error("", zap.Error(err))
		return nil, err
	}
	environments, httpRes, err := e.client.EnvironmentsApi.ListEnvironments(ctx, e.config.Context.ProjectId).Page(int32(page)).PerPage(int32(perPage)).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		e.logger.Error("error listing environments", zap.Error(err))
		return nil, err
	}
	return environments, nil
}

// Get an environment by key.
// Usage Example:
// 	`environment, err := PermitClient.Api.Environments.Get(ctx, "production")`
func (e *Environments) Get(ctx context.Context, environmentKey string) (*models.EnvironmentRead, error) {
	err := e.lazyLoadContext(ctx)
	if err != nil {
		e.logger.Error("", zap.Error(err))
		return nil, err
	}
	environment, httpRes, err := e.client.EnvironmentsApi.GetEnvironment(ctx, e.config.Context.ProjectId, environmentKey).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		e.logger.Error("error getting environment: "+environmentKey, zap.Error(err))
		return nil, err
	}

	return environment, nil
}

// GetByKey get an environment by key.
// Usage Example:
// 	`environment, err := PermitClient.Api.Environments.GetByKey(ctx, "production")`
func (e *Environments) GetByKey(ctx context.Context, environmentKey string) (*models.EnvironmentRead, error) {
	return e.Get(ctx, environmentKey)
}

// GetById get an environment by id.
// Usage Example:
// 	`environment, err := PermitClient.Api.Environments.GetById(ctx, uuid.New())`
func (e *Environments) GetById(ctx context.Context, environmentId uuid.UUID) (*models.EnvironmentRead, error) {
	return e.Get(ctx, environmentId.String())
}

// Create an environment in the project of your context.
// Usage Example:
//  ```
//  environmentCreate := models.NewEnvironmentCreate("production", "Production")
// 	environment, err := PermitClient.Api.Environments.Create(ctx, *environmentCreate)
//  ```
func (e *Environments) Create(ctx context.Context, environmentCreate models.EnvironmentCreate) (*models.EnvironmentRead, error) {
	err := e.lazyLoadContext(ctx)
	if err != nil {
		e.logger.Error("", zap.Error(err))
		return nil, err
	}
	environment, httpRes, err := e.client.EnvironmentsApi.CreateEnvironment(ctx, e.config.Context.ProjectId).EnvironmentCreate(environmentCreate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		e.logger.Error("error creating environment: "+environmentCreate.GetKey(), zap.Error(err))
		return nil, err
	}

	return environment, nil
}

// Update an environment in the project of your context.
// Usage Example:
//  ```
//  environmentUpdate := models.NewEnvironmentUpdate()
//  environmentUpdate.SetName("Production")
// 	environment, err := PermitClient.Api.Environments.Update(ctx, "production", *environmentUpdate)
//  ```

func (e *Environments) Update(ctx context.Context, environmentKey string, environmentUpdate models.EnvironmentUpdate) (*models.EnvironmentRead, error) {
	err := e.lazyLoadContext(ctx)
	if err != nil {
		e.logger.Error("", zap.Error(err))
		return nil, err
	}
	environment, httpRes, err := e.client.EnvironmentsApi.UpdateEnvironment(ctx, e.config.Context.ProjectId, environmentKey).EnvironmentUpdate(environmentUpdate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		e.logger.Error("error updating environment: "+environmentKey, zap.Error(err))
		return nil, err
	}

	return environment, nil
}

func (e *Environments) Delete(ctx context.Context, environmentKey string) error {
	err := e.lazyLoadContext(ctx)
	if err != nil {
		e.logger.Error("", zap.Error(err))
		return err
	}
	httpRes, err := e.client.EnvironmentsApi.DeleteEnvironment(ctx, e.config.Context.ProjectId, environmentKey).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		e.logger.Error("error deleting environment: "+environmentKey, zap.Error(err))
		return err
	}
	return nil
}
