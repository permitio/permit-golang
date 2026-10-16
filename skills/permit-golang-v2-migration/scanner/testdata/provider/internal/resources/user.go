package resources

import (
	"context"

	"github.com/permitio/permit-golang/pkg/models" // v1: github.com/permitio/permit-golang/pkg/models
	"github.com/permitio/permit-golang/pkg/permit" // v1: github.com/permitio/permit-golang/pkg/permit
)

// UserModel is the state of a user and the role it has in a tenant.
type UserModel struct {
	Key    string
	Email  string
	Role   string
	Tenant string
}

// User manages users.
type User struct {
	client *permit.Client // v1: permit.Client
}

// Create creates or updates the user, then assigns its role.
func (u *User) Create(ctx context.Context, plan UserModel) error {
	user := models.NewUserCreate(plan.Key)                             // v1: models.NewUserCreate
	user.SetEmail(plan.Email)                                          // v1: models.UserCreate.SetEmail
	if _, err := u.client.Api.Users.SyncUser(ctx, *user); err != nil { // v1: api.Users.SyncUser
		return err
	}
	_, err := u.client.Api.Users.AssignRole(ctx, plan.Key, plan.Role, plan.Tenant) // v1: api.Users.AssignRole
	return err
}

// Read reads the user's email, and reports false when the user is gone.
func (u *User) Read(ctx context.Context, key string) (string, bool, error) {
	user, err := u.client.Api.Users.Get(ctx, key) // v1: api.Users.Get
	if isNotFound(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return user.GetEmail(), true, nil // v1 missed: models.UserRead.GetEmail
}

// Delete deletes the user.
func (u *User) Delete(ctx context.Context, key string) error {
	return u.client.Api.Users.Delete(ctx, key) // v1: api.Users.Delete
}
