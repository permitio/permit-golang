package api

import (
	"context"
	"github.com/google/uuid"
	"github.com/permitio/permit-golang/openapi"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/permit"
	"go.uber.org/zap"
)

type Users struct {
	PermitBaseApi
}

func NewUsersApi(client *openapi.APIClient, config *permit.PermitConfig) *Users {
	return &Users{
		PermitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

func (u *Users) List(ctx context.Context, page int, perPage int) ([]openapi.UserRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		u.logger.Error("error listing users - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}
	err := u.LazyLoadContext(ctx)
	if err != nil {
		u.logger.Error("", zap.Error(err))
		return nil, err
	}
	users, httpRes, err := u.client.UsersApi.ListUsers(ctx, u.config.Context.GetEnvironment(), u.config.Context.GetProject()).Page(int32(page)).PerPage(int32(perPage)).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		u.logger.Error("error listing users", zap.Error(err))
		return nil, err
	}
	return users.GetData(), nil
}

func (u *Users) Get(ctx context.Context, userKey string) (*openapi.UserRead, error) {
	err := u.LazyLoadContext(ctx)
	if err != nil {
		u.logger.Error("", zap.Error(err))
		return nil, err
	}
	user, httpRes, err := u.client.UsersApi.GetUser(ctx, u.config.Context.GetEnvironment(), u.config.Context.GetProject(), userKey).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		u.logger.Error("error getting user: "+userKey, zap.Error(err))
		return nil, err
	}
	return user, nil
}

func (u *Users) GetByKey(ctx context.Context, userKey string) (*openapi.UserRead, error) {
	return u.Get(ctx, userKey)
}

func (u *Users) GetById(ctx context.Context, userId uuid.UUID) (*openapi.UserRead, error) {
	return u.Get(ctx, userId.String())
}

func (u *Users) Create(ctx context.Context, userCreate openapi.UserCreate) (*openapi.UserRead, error) {
	err := u.LazyLoadContext(ctx)
	if err != nil {
		u.logger.Error("", zap.Error(err))
		return nil, err
	}
	user, httpRes, err := u.client.UsersApi.CreateUser(ctx, u.config.Context.GetEnvironment(), u.config.Context.GetProject()).UserCreate(userCreate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		u.logger.Error("error creating userCreate:"+userCreate.GetKey(), zap.Error(err))
		return nil, err
	}
	return user, nil
}

func (u *Users) Update(ctx context.Context, userKey string, userUpdate openapi.UserUpdate) (*openapi.UserRead, error) {
	err := u.LazyLoadContext(ctx)
	if err != nil {
		u.logger.Error("", zap.Error(err))
		return nil, err
	}
	user, httpRes, err := u.client.UsersApi.UpdateUser(ctx, u.config.Context.GetEnvironment(), u.config.Context.GetProject(), userKey).UserUpdate(userUpdate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		u.logger.Error("error updating user:"+userKey, zap.Error(err))
		return nil, err
	}
	return user, nil
}

func (u *Users) Delete(ctx context.Context, userKey string) error {
	err := u.LazyLoadContext(ctx)
	if err != nil {
		u.logger.Error("", zap.Error(err))
		return err
	}
	httpRes, err := u.client.UsersApi.DeleteUser(ctx, u.config.Context.GetEnvironment(), u.config.Context.GetProject(), userKey).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		u.logger.Error("error deleting user:"+userKey, zap.Error(err))
		return err
	}
	return nil
}

func (u *Users) AssignRole(ctx context.Context, userKey string, roleKey string, tenantKey string) (*openapi.RoleAssignmentRead, error) {
	err := u.LazyLoadContext(ctx)
	if err != nil {
		u.logger.Error("", zap.Error(err))
		return nil, err
	}
	userRoleCreate := *openapi.NewUserRoleCreate(roleKey, tenantKey)
	roleAssignmentRead, httpRes, err := u.client.UsersApi.AssignRoleToUser(ctx, u.config.Context.GetEnvironment(), u.config.Context.GetProject(), userKey).UserRoleCreate(userRoleCreate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		u.logger.Error("error assigning role:"+roleKey+" to user:"+userKey, zap.Error(err))
		return nil, err
	}
	return roleAssignmentRead, nil
}

func (u *Users) UnassignRole(ctx context.Context, userKey string, roleKey string, tenantKey string) (*openapi.UserRead, error) {
	err := u.LazyLoadContext(ctx)
	if err != nil {
		u.logger.Error("", zap.Error(err))
		return nil, err
	}
	UserRoleRemove := *openapi.NewUserRoleRemove(roleKey, tenantKey)
	user, httpRes, err := u.client.UsersApi.UnassignRoleFromUser(ctx, u.config.Context.GetProject(), u.config.Context.GetEnvironment(), userKey).UserRoleRemove(UserRoleRemove).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		u.logger.Error("error unassigning role:"+roleKey+" from user:"+userKey, zap.Error(err))
		return nil, err
	}
	return user, nil
}
func (u *Users) GetAssignedRoles(ctx context.Context, userKey string, tenantKey string, page int, perPage int) ([]openapi.RoleAssignmentRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		u.logger.Error("error listing users - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}
	err := u.LazyLoadContext(ctx)
	if err != nil {
		u.logger.Error("", zap.Error(err))
		return nil, err
	}
	roleAssignments, httpRes, err := u.client.RoleAssignmentsApi.ListRoleAssignments(ctx, u.config.Context.GetEnvironment(), u.config.Context.GetProject()).
		User(userKey).
		Tenant(tenantKey).
		Page(int32(page)).PerPage(int32(perPage)).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		u.logger.Error("error listing roles for user:"+userKey, zap.Error(err))
		return nil, err
	}
	return roleAssignments, nil
}
