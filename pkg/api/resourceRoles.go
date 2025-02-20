package api

import (
	"context"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/openapi"
)

type ResourceRoles struct {
	permitBaseApi
}

func NewResourceRolesApi(client *openapi.APIClient, config *config.PermitConfig) *ResourceRoles {
	return &ResourceRoles{
		permitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

func (r *ResourceRoles) Create(
	ctx context.Context,
	resourceId string,
	resourceRoleCreate models.ResourceRoleCreate,
) (*models.ResourceRoleRead, error) {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return nil, err
	}

	created, httpRes, err := r.client.ResourceRoles.CreateResourceRole(ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
	).ResourceRoleCreate(resourceRoleCreate).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error creating resource role", err)
		return nil, err
	}

	return created, nil
}

func (r *ResourceRoles) Delete(
	ctx context.Context,
	resourceId string,
	roleId string,
) error {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return err
	}

	httpRes, err := r.client.ResourceRoles.DeleteResourceRole(ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
		roleId,
	).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error deleting resource role", err)
		return err
	}

	return nil
}

func (r *ResourceRoles) Get(
	ctx context.Context,
	resourceId string,
	roleId string,
) (*models.ResourceRoleRead, error) {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return nil, err
	}

	retrieved, httpRes, err := r.client.ResourceRoles.GetResourceRole(ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
		roleId,
	).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error deleting resource role", err)
		return nil, err
	}

	return retrieved, nil
}

func (r *ResourceRoles) List(
	ctx context.Context,
	page int,
	perPage int,
	resourceId string,
) (*[]models.ResourceRoleRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)

	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		r.logger.Error("error listing resource roles - max per page: "+string(perPageLimit), err)
		return nil, err
	}

	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return nil, err
	}

	request := r.client.ResourceRoles.ListResourceRoles(
		ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
	).Page(int32(page)).PerPage(int32(perPage))

	retrieved, httpRes, err := request.Execute()

	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		r.logger.Error("error listing resource roles", err)
		return nil, err
	}

	return &retrieved, nil
}

func (r *ResourceRoles) Update(
	ctx context.Context,
	resourceId string,
	roleId string,
	resourceInstanceUpdate models.ResourceRoleUpdate,
) (*models.ResourceRoleRead, error) {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return nil, err
	}

	updated, httpRes, err := r.client.ResourceRoles.UpdateResourceRole(
		ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
		roleId,
	).ResourceRoleUpdate(resourceInstanceUpdate).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error updating resource role", err)
		return nil, err
	}

	return updated, nil
}

func (r *ResourceRoles) AddParent(
	ctx context.Context,
	resourceId string,
	roleId string,
	parentRoleId string,
) (*models.ResourceRoleRead, error) {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return nil, err
	}

	updated, httpRes, err := r.client.ResourceRoles.AddParentResourceRole(ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
		roleId,
		parentRoleId,
	).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error adding parent to resource role", err)
		return nil, err
	}

	return updated, nil
}

func (r *ResourceRoles) RemoveParent(
	ctx context.Context,
	resourceId string,
	roleId string,
	parentRoleId string,
) (*models.ResourceRoleRead, error) {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return nil, err
	}

	updated, httpRes, err := r.client.ResourceRoles.RemoveParentResourceRole(ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
		roleId,
		parentRoleId,
	).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error creating resource instance", err)
		return nil, err
	}

	return updated, nil
}

func (r *ResourceRoles) AssignPermissions(
	ctx context.Context,
	resourceId string,
	roleId string,
	addRolePermissions models.AddRolePermissions,
) (*models.ResourceRoleRead, error) {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return nil, err
	}

	updated, httpRes, err := r.client.ResourceRoles.AssignPermissionsToResourceRole(
		ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
		roleId,
	).AddRolePermissions(addRolePermissions).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error assigning permissions", err)
		return nil, err
	}

	return updated, nil
}

func (r *ResourceRoles) RemovePermissions(
	ctx context.Context,
	resourceId string,
	roleId string,
	removeRolePermissions models.RemoveRolePermissions,
) (*models.ResourceRoleRead, error) {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return nil, err
	}

	updated, httpRes, err := r.client.ResourceRoles.RemovePermissionsFromResourceRole(
		ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
		roleId,
	).RemoveRolePermissions(removeRolePermissions).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error removing permissions", err)
		return nil, err
	}

	return updated, nil
}

func (r *ResourceRoles) GetAncestors(
	ctx context.Context,
	resourceId string,
	roleId string,
) (*[]models.ResourceRoleRead, error) {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return nil, err
	}

	retrieved, httpRes, err := r.client.ResourceRoles.GetResourceRoleAncestors(
		ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
		roleId,
	).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error assigning permissions", err)
		return nil, err
	}

	if retrieved.Roles.PaginatedResultResourceRoleRead != nil {
		return &retrieved.Roles.PaginatedResultResourceRoleRead.Data, nil
	} else {
		return retrieved.Roles.ResourceRoleRead, nil
	}
}

func (r *ResourceRoles) GetDescendants(
	ctx context.Context,
	resourceId string,
	roleId string,
) (*[]models.ResourceRoleRead, error) {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", err)
		return nil, err
	}

	retrieved, httpRes, err := r.client.ResourceRoles.GetResourceRoleDescendants(
		ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
		roleId,
	).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error assigning permissions", err)
		return nil, err
	}

	if retrieved.Roles.PaginatedResultResourceRoleRead != nil {
		return &retrieved.Roles.PaginatedResultResourceRoleRead.Data, nil
	} else {
		return retrieved.Roles.ResourceRoleRead, nil
	}
}
