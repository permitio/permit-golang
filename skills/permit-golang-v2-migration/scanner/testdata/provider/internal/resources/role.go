package resources

import (
	"context"

	"github.com/permitio/permit-golang/pkg/models" // v1: github.com/permitio/permit-golang/pkg/models
	"github.com/permitio/permit-golang/pkg/permit" // v1: github.com/permitio/permit-golang/pkg/permit
)

// RoleModel is the state of a role.
type RoleModel struct {
	Key         string
	Name        string
	Permissions []string
}

// Role manages roles.
type Role struct {
	client *permit.Client // v1: permit.Client
}

// Create creates the role.
func (r *Role) Create(ctx context.Context, plan RoleModel) error {
	body := models.NewRoleCreate(plan.Key, plan.Name) // v1: models.NewRoleCreate
	body.SetPermissions(plan.Permissions)             // v1: models.RoleCreate.SetPermissions
	_, err := r.client.Api.Roles.Create(ctx, *body)   // v1: api.Roles.Create
	return err
}

// Update changes the name and the permissions of the role.
func (r *Role) Update(ctx context.Context, plan RoleModel) error {
	change := models.RoleUpdate{Name: &plan.Name, Permissions: plan.Permissions} // v1: models.RoleUpdate
	_, err := r.client.Api.Roles.Update(ctx, plan.Key, change)                   // v1: api.Roles.Update
	return err
}

// Delete deletes the role.
func (r *Role) Delete(ctx context.Context, key string) error {
	roles := r.client.Api.Roles
	return roles.Delete(ctx, key) // v1: api.Roles.Delete
}
