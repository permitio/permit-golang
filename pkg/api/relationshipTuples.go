package api

import (
	"context"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/openapi"
	"time"
)

type RelationshipTuples struct {
	PermitBaseFactsApi
}

func NewRelationshipTuplesApi(client *openapi.APIClient, config *config.PermitConfig) *RelationshipTuples {
	return &RelationshipTuples{
		PermitBaseFactsApi{
			permitBaseApi{
				client: client,
				config: config,
				logger: config.Logger,
			},
		},
	}
}
func (u *RelationshipTuples) WaitForSync(timeout *time.Duration) *RelationshipTuples {
	return NewRelationshipTuplesApi(u.PermitBaseFactsApi.WaitForSync(timeout).client, u.config)
}

func (r *RelationshipTuples) Create(
	ctx context.Context,
	relationshipTupleCreate models.RelationshipTupleCreate,
) (*models.RelationshipTupleRead, error) {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return nil, err
	}

	relTuple, httpRes, err := r.client.RelationshipTuplesApi.CreateRelationshipTuple(ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
	).RelationshipTupleCreate(relationshipTupleCreate).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error creating relationship tuple", err)
		return nil, err
	}

	return relTuple, nil
}

func (r *RelationshipTuples) Delete(
	ctx context.Context,
	relationshipTupleDelete models.RelationshipTupleDelete,
) error {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return err
	}

	httpRes, err := r.client.RelationshipTuplesApi.DeleteRelationshipTuple(ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
	).RelationshipTupleDelete(relationshipTupleDelete).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error deleting relationship tuple", err)
		return err
	}

	return nil
}

func (r *RelationshipTuples) List(
	ctx context.Context,
	page int,
	perPage int,
	tenantFilter string,
	subjectFilter string,
	relationFilter string,
	objectFilter string,
) (*[]models.RelationshipTupleRead, error) {
	return r.list(ctx, page, perPage, tenantFilter, subjectFilter, relationFilter, objectFilter, false)
}

func (r *RelationshipTuples) ListDetailed(
	ctx context.Context,
	page int,
	perPage int,
	tenantFilter string,
	subjectFilter string,
	relationFilter string,
	objectFilter string,
) (*[]models.RelationshipTupleRead, error) {
	return r.list(ctx, page, perPage, tenantFilter, subjectFilter, relationFilter, objectFilter, true)
}

func (r *RelationshipTuples) list(ctx context.Context,
	page int,
	perPage int,
	tenantFilter,
	subjectFilter,
	relationFilter,
	objectFilter string,
	detailed bool,
) (*[]models.RelationshipTupleRead, error) {
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

	request := r.client.RelationshipTuplesApi.ListRelationshipTuples(
		ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
	).Page(int32(page)).PerPage(int32(perPage))

	if tenantFilter != "" {
		request = request.Tenant(tenantFilter)
	}

	if subjectFilter != "" {
		request = request.Subject(subjectFilter)
	}

	if relationFilter != "" {
		request = request.Relation(relationFilter)
	}

	if objectFilter != "" {
		request = request.Object(objectFilter)
	}

	if detailed {
		request = request.Detailed(detailed)
	}

	relTuples, httpRes, err := request.Execute()

	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		r.logger.Error("error listing relationship tuples", err)
		return nil, err
	}

	return &relTuples, nil
}

func (r *RelationshipTuples) BulkCreate(
	ctx context.Context,
	bulkCreateOperation models.RelationshipTupleCreateBulkOperation,
) error {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return err
	}

	_, httpRes, err := r.client.RelationshipTuplesApi.BulkCreateRelationshipTuples(ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
	).RelationshipTupleCreateBulkOperation(bulkCreateOperation).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error executing bulk relationship tuple creation", err)
		return err
	}

	return nil
}

func (r *RelationshipTuples) BulkDelete(
	ctx context.Context,
	bulkDeleteOperation models.RelationshipTupleDeleteBulkOperation,
) error {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return err
	}

	_, httpRes, err := r.client.RelationshipTuplesApi.BulkDeleteRelationshipTuples(ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
	).RelationshipTupleDeleteBulkOperation(bulkDeleteOperation).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error executing bulk relationship tuple deletion", err)
		return err
	}

	return nil
}
