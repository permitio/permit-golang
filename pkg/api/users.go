package api

import (
	"context"
	"github.com/google/uuid"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/openapi"
	"go.uber.org/zap"
	"strings"
)

type Users struct {
	permitBaseApi
}

func NewUsersApi(client *openapi.APIClient, config *config.PermitConfig) *Users {
	return &Users{
		permitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

// List the users from your context's environment.
// Usage Example:
//
//	`users, err := PermitClient.Api.Users.List(ctx, 1, 10)`
func (u *Users) List(ctx context.Context, page int, perPage int) ([]models.UserRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		u.logger.Error("error listing users - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}
	err := u.lazyLoadPermitContext(ctx)
	if err != nil {
		u.logger.Error("", zap.Error(err))
		return nil, err
	}
	users, httpRes, err := u.client.UsersApi.ListUsers(ctx, u.config.Context.GetProject(), u.config.Context.GetEnvironment()).Page(int32(page)).PerPage(int32(perPage)).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		u.logger.Error("error listing users", zap.Error(err))
		return nil, err
	}
	return users.GetData(), nil
}

// Get a user from your context's environment.
// Usage Example:
//
//	`user, err := PermitClient.Api.Users.Get(ctx, "user-key")`
func (u *Users) Get(ctx context.Context, userKey string) (*models.UserRead, error) {
	err := u.lazyLoadPermitContext(ctx)
	if err != nil {
		u.logger.Error("", zap.Error(err))
		return nil, err
	}
	user, httpRes, err := u.client.UsersApi.GetUser(ctx, u.config.Context.GetProject(), u.config.Context.GetEnvironment(), userKey).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		u.logger.Error("error getting user: "+userKey, zap.Error(err))
		return nil, err
	}
	return user, nil
}

// GetByKey gets a user by key from your context's environment.
// Usage Example:
//
//	`user, err := PermitClient.Api.Users.GetByKey(ctx, "user-key")`
func (u *Users) GetByKey(ctx context.Context, userKey string) (*models.UserRead, error) {
	return u.Get(ctx, userKey)
}

// GetById gets a user by id from your context's environment.
// Usage Example:
//
//	`user, err := PermitClient.Api.Users.GetById(ctx, uuid.New())`
func (u *Users) GetById(ctx context.Context, userId uuid.UUID) (*models.UserRead, error) {
	return u.Get(ctx, userId.String())
}

// Create a user in your context's environment.
// Usage Example:
// ```
//
//	userCreate := models.NewUserCreate("user-key")
//	userCreate.SetEmail("user-email@mail.com")
//	userCreate.SetFirstName("user-first-name")
//	userCreate.SetLastName("user-last-name")
//	user, err := PermitClient.Api.Users.Create(ctx, userCreate)
//
// ```
func (u *Users) Create(ctx context.Context, userCreate models.UserCreate) (*models.UserRead, error) {
	err := u.lazyLoadPermitContext(ctx)

	if err != nil {
		u.logger.Error("", zap.Error(err))
		return nil, err
	}

	user, httpRes, err := u.client.UsersApi.CreateUser(
		ctx,
		u.config.Context.GetProject(),
		u.config.Context.GetEnvironment(),
	).UserCreate(userCreate).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		u.logger.Error("error creating user:"+userCreate.GetKey(), zap.Error(err))
		return nil, err
	}

	return user, nil
}

// Update a user in your context's environment.
// Usage Example:
// ```
//
//	userUpdate := models.NewUserUpdate()
//	userUpdate.SetEmail("new@email.com")
//	userUpdate.SetFirstName("new-first-name")
//	userUpdate.SetLastName("new-last-name")
//	user, err := PermitClient.Api.Users.Update(ctx, "user-key", userUpdate)
//
// ```
func (u *Users) Update(ctx context.Context, userKey string, userUpdate models.UserUpdate) (*models.UserRead, error) {
	err := u.lazyLoadPermitContext(ctx)
	if err != nil {
		u.logger.Error("", zap.Error(err))
		return nil, err
	}
	user, httpRes, err := u.client.UsersApi.UpdateUser(ctx, u.config.Context.GetProject(), u.config.Context.GetEnvironment(), userKey).UserUpdate(userUpdate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		u.logger.Error("error updating user:"+userKey, zap.Error(err))
		return nil, err
	}
	return user, nil
}

// Delete a user from your context's environment.
// Usage Example:
//
//	`err := PermitClient.Api.Users.Delete(ctx, "user-key")`
func (u *Users) Delete(ctx context.Context, userKey string) error {
	err := u.lazyLoadPermitContext(ctx)
	if err != nil {
		u.logger.Error("", zap.Error(err))
		return err
	}
	httpRes, err := u.client.UsersApi.DeleteUser(ctx, u.config.Context.GetProject(), u.config.Context.GetEnvironment(), userKey).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		u.logger.Error("error deleting user:"+userKey, zap.Error(err))
		return err
	}
	return nil
}

// AssignRole assigns a role to a user in your context's environment, by user key, role key and tenant key.
// Usage Example:
// `roleAssignment, err := PermitClient.Api.Users.AssignRole(ctx, "user-key", "role-key", "default")`
func (u *Users) AssignRole(ctx context.Context, userKey string, roleKey string, tenantKey string) (*models.RoleAssignmentRead, error) {
	err := u.lazyLoadPermitContext(ctx)
	if err != nil {
		u.logger.Error("", zap.Error(err))
		return nil, err
	}
	userRoleCreate := *models.NewUserRoleCreate(roleKey, tenantKey)
	roleAssignmentRead, httpRes, err := u.client.UsersApi.AssignRoleToUser(ctx, u.config.Context.GetProject(), u.config.Context.GetEnvironment(), userKey).UserRoleCreate(userRoleCreate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		u.logger.Error("error assigning role:"+roleKey+" to user:"+userKey, zap.Error(err))
		return nil, err
	}
	return roleAssignmentRead, nil
}

// AssignResourceRole assigns a role to a user in your context's environment, by user key, role key and tenant key.
// Usage Example:
// `roleAssignment, err := PermitClient.Api.Users.AssignRole(ctx, "user-key", "role-key", "default", "document:mydoc")`
func (u *Users) AssignResourceRole(ctx context.Context, userKey string, roleKey string, tenantKey string, resourceInstance string) (*models.RoleAssignmentRead, error) {
	err := u.lazyLoadPermitContext(ctx)
	if err != nil {
		u.logger.Error("", zap.Error(err))
		return nil, err
	}
	userRoleCreate := *models.NewUserRoleCreate(roleKey, tenantKey)
	userRoleCreate.SetResourceInstance(resourceInstance)
	roleAssignmentRead, httpRes, err := u.client.UsersApi.AssignRoleToUser(ctx, u.config.Context.GetProject(), u.config.Context.GetEnvironment(), userKey).UserRoleCreate(userRoleCreate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		u.logger.Error("error assigning role:"+roleKey+" to user:"+userKey, zap.Error(err))
		return nil, err
	}
	return roleAssignmentRead, nil
}

// UnassignRole unassigns a role from a user in your context's environment, by user key, role key and tenant key.
// Usage Example:
// `err := PermitClient.Api.Users.UnassignRole(ctx, "user-key", "role-key", "default")`
func (u *Users) UnassignRole(ctx context.Context, userKey string, roleKey string, tenantKey string) (*models.UserRead, error) {
	err := u.lazyLoadPermitContext(ctx)
	if err != nil {
		u.logger.Error("", zap.Error(err))
		return nil, err
	}
	UserRoleRemove := *models.NewUserRoleRemove(roleKey, tenantKey)
	user, httpRes, err := u.client.UsersApi.UnassignRoleFromUser(ctx, u.config.Context.GetProject(), u.config.Context.GetEnvironment(), userKey).UserRoleRemove(UserRoleRemove).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		u.logger.Error("error unassigning role:"+roleKey+" from user:"+userKey, zap.Error(err))
		return nil, err
	}
	return user, nil
}

// GetAssignedRoles lists all roles assigned to a user in your context's environment, by user key, tenant key and pagination options.
// Usage Example:
// ```
//
//	`roleAssignmentList, err := PermitClient.Api.Users.GetAssignedRoles(ctx, "user-key", "default", 1, 10)`
func (u *Users) GetAssignedRoles(ctx context.Context, userKey string, tenantKey string, page int, perPage int) ([]models.RoleAssignmentRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		u.logger.Error("error listing users - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}
	err := u.lazyLoadPermitContext(ctx)
	if err != nil {
		u.logger.Error("", zap.Error(err))
		return nil, err
	}
	roleAssignments, httpRes, err := u.client.RoleAssignmentsApi.ListRoleAssignments(ctx, u.config.Context.GetProject(), u.config.Context.GetEnvironment()).
		User(userKey).
		Tenant(tenantKey).
		Page(int32(page)).PerPage(int32(perPage)).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		u.logger.Error("error listing roles for user:"+userKey, zap.Error(err))
		return nil, err
	}
	if roleAssignments == nil || roleAssignments.RoleAssignmentRead == nil {
		if roleAssignments.RoleAssignmentRead == nil {
			emptyRoleAssignments := make([]models.RoleAssignmentRead, 0)
			return emptyRoleAssignments, nil
		}
	}
	return *roleAssignments.RoleAssignmentRead, nil
}

// SyncUser syncs a user in your context's environment, by user.
// Usage Example:
// ```
// userCreate := *models.NewUserCreate("user-key")
// userCreate.SetEmail("user-email")
// userCreate.SetFirstName("user-first-name")
// userCreate.SetLastName("user-last-name")
// user, err := PermitClient.Api.Users.SyncUser(ctx, userCreate)
// ```
func (u *Users) SyncUser(ctx context.Context, user models.UserCreate) (*models.UserRead, error) {
	err := u.lazyLoadPermitContext(ctx)
	if err != nil {
		u.logger.Error("", zap.Error(err))
		return nil, err
	}
	existUser, err := u.Get(ctx, user.GetKey())
	if err != nil {
		if !strings.Contains(err.Error(), string(errors.NotFound)) {
			u.logger.Error("", zap.Error(err))
			return nil, err
		}
	}
	if existUser != nil {
		u.logger.Info("User already exists, updating it...", zap.String("user", user.GetKey()))
		userUpdate := models.NewUserUpdate()
		if email := user.GetEmail(); email != "" {
			userUpdate.SetEmail(user.GetEmail())
		}
		if firstName := user.GetFirstName(); firstName != "" {
			userUpdate.SetFirstName(user.GetFirstName())
		}
		if lastName := user.GetLastName(); lastName != "" {
			userUpdate.SetLastName(user.GetLastName())
		}
		userUpdate.SetAttributes(user.GetAttributes())
		userRead, err := u.Update(ctx, user.GetKey(), *userUpdate)
		if err != nil {
			u.logger.Error("error updating user: "+user.GetKey(), zap.Error(err))
			return nil, err
		}
		return userRead, nil
	}
	u.logger.Info("User does not exist, creating it...", zap.String("user", user.GetKey()))
	userRead, err := u.Create(ctx, user)
	if err != nil {
		u.logger.Error("error creating user: "+user.GetKey(), zap.Error(err))
		return nil, err
	}
	return userRead, err
}
