package api

import (
	"context"
	"fmt"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/openapi"
)

type ResourceRelations struct {
	permitBaseApi
}

func NewResourceRelationsApi(client *openapi.APIClient, config *config.PermitConfig) *ResourceRelations {
	return &ResourceRelations{
		permitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

func (r *ResourceRelations) Create(
	ctx context.Context,
	resourceId string,
	relationCreate models.RelationCreate,
) (*models.RelationRead, error) {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return nil, err
	}

	created, httpRes, err := r.client.ResourceRelations.CreateResourceRelation(
		ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
	).RelationCreate(relationCreate).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error creating resource relation", err)
		return nil, err
	}

	return created, nil
}

func (r *ResourceRelations) Delete(
	ctx context.Context,
	resourceId string,
	relationId string,
) error {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return err
	}

	httpRes, err := r.client.ResourceRelations.DeleteResourceRelation(ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
		relationId,
	).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error deleting resource instance", err)
		return err
	}

	return nil
}

func (r *ResourceRelations) Get(
	ctx context.Context,
	resourceId string,
	relationId string,
) (*models.RelationRead, error) {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return nil, err
	}

	retrieved, httpRes, err := r.client.ResourceRelations.GetResourceRelation(ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
		relationId,
	).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error deleting resource instance", err)
		return nil, err
	}

	return retrieved, nil
}

func (r *ResourceRelations) List(
	ctx context.Context,
	page int,
	perPage int,
	resourceId string,
) (*[]models.RelationRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)

	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		r.logger.Error("error listing relationship tuples - max per page: "+string(perPageLimit), err)
		return nil, err
	}

	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return nil, err
	}

	request := r.client.ResourceRelations.ListResourceRelations(
		ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
	).Page(int32(page)).PerPage(int32(perPage))

	retrieved, httpRes, err := request.Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error listing resource instances", err)
		return nil, err
	}

	if retrieved == nil {
		err = fmt.Errorf("error listing resource instances - retrieved is nil")
		r.logger.Error("", err)
		return nil, err
	}

	return &retrieved.Data, nil
}
