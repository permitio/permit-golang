package api

import (
	"github.com/google/uuid"
	"github.com/permitio/permit-golang/models"
	"github.com/permitio/permit-golang/openapi"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/permit"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

type Environments struct {
	PermitBaseApi
}

func NewEnvironmentsApi(client *openapi.APIClient, config *permit.PermitConfig) *Environments {
	return &Environments{
		PermitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

func (e *Environments) List(ctx context.Context, page int, perPage int) ([]models.EnvironmentRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		e.logger.Error("error listing environments - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}
	err := e.LazyLoadContext(ctx)
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

func (e *Environments) Get(ctx context.Context, environmentKey string) *models.EnvironmentRead {
	err := e.LazyLoadContext(ctx)
	if err != nil {
		e.logger.Error("", zap.Error(err))
	}
	environment, httpRes, err := e.client.EnvironmentsApi.GetEnvironment(ctx, e.config.Context.ProjectId, environmentKey).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		e.logger.Error("error getting environment: "+environmentKey, zap.Error(err))
	}

	return environment
}

func (e *Environments) GetByKey(ctx context.Context, environmentKey string) *models.EnvironmentRead {
	return e.Get(ctx, environmentKey)
}

func (e *Environments) GetById(ctx context.Context, environmentId uuid.UUID) *models.EnvironmentRead {
	return e.Get(ctx, environmentId.String())
}

func (e *Environments) Create(ctx context.Context, environmentCreate models.EnvironmentCreate) *models.EnvironmentRead {
	err := e.LazyLoadContext(ctx)
	if err != nil {
		e.logger.Error("", zap.Error(err))
	}
	environment, httpRes, err := e.client.EnvironmentsApi.CreateEnvironment(ctx, e.config.Context.ProjectId).EnvironmentCreate(environmentCreate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		e.logger.Error("error creating environment: "+environmentCreate.GetKey(), zap.Error(err))
	}

	return environment
}

func (e *Environments) Update(ctx context.Context, environmentKey string, environmentUpdate models.EnvironmentUpdate) *models.EnvironmentRead {
	err := e.LazyLoadContext(ctx)
	if err != nil {
		e.logger.Error("", zap.Error(err))
	}
	environment, httpRes, err := e.client.EnvironmentsApi.UpdateEnvironment(ctx, e.config.Context.ProjectId, environmentKey).EnvironmentUpdate(environmentUpdate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		e.logger.Error("error updating environment: "+environmentKey, zap.Error(err))
	}

	return environment
}

func (e *Environments) Delete(ctx context.Context, environmentKey string) {
	err := e.LazyLoadContext(ctx)
	if err != nil {
		e.logger.Error("", zap.Error(err))
	}
	httpRes, err := e.client.EnvironmentsApi.DeleteEnvironment(ctx, e.config.Context.ProjectId, environmentKey).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		e.logger.Error("error deleting environment: "+environmentKey, zap.Error(err))
	}
}
