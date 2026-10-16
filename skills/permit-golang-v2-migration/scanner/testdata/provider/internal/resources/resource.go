// Package resources holds the resources of a sample shaped like a Terraform provider: each keeps
// the provider's v1 client and maps its create, read, update and delete to the client's calls.
// Each line that uses v1 ends with a comment "v1:" that lists what the scanner must report on it,
// or "v1 missed:" for a method or a field of a value whose v1 type the scanner does not know.
package resources

import (
	"context"

	"github.com/permitio/permit-golang/pkg/models" // v1: github.com/permitio/permit-golang/pkg/models
	"github.com/permitio/permit-golang/pkg/permit" // v1: github.com/permitio/permit-golang/pkg/permit
)

// ResourceModel is the state of a resource type.
type ResourceModel struct {
	Key         string
	Name        string
	Description string
	Actions     map[string]string
}

// Resource manages resource types.
type Resource struct {
	client *permit.Client // v1: permit.Client
}

// NewResource returns the resource of resource types.
func NewResource(client *permit.Client) *Resource { // v1: permit.Client
	return &Resource{client: client}
}

// Create creates the resource type.
func (r *Resource) Create(ctx context.Context, plan ResourceModel) (ResourceModel, error) {
	actions := map[string]models.ActionBlockEditable{} // v1: models.ActionBlockEditable
	for key, name := range plan.Actions {
		action := models.NewActionBlockEditable() // v1: models.NewActionBlockEditable
		action.SetName(name)                      // v1: models.ActionBlockEditable.SetName
		actions[key] = *action
	}
	body := models.NewResourceCreate(plan.Key, plan.Name, actions) // v1: models.NewResourceCreate
	body.SetDescription(plan.Description)                          // v1: models.ResourceCreate.SetDescription
	created, err := r.client.Api.Resources.Create(ctx, *body)      // v1: api.Resources.Create
	if err != nil {
		return ResourceModel{}, err
	}
	return read(created), nil
}

// Read reads the resource type, and reports false when it is gone.
func (r *Resource) Read(ctx context.Context, key string) (ResourceModel, bool, error) {
	got, err := r.client.Api.Resources.Get(ctx, key) // v1: api.Resources.Get
	if isNotFound(err) {
		return ResourceModel{}, false, nil
	}
	if err != nil {
		return ResourceModel{}, false, err
	}
	return read(got), true, nil
}

// Update changes the name and the description of the resource type.
func (r *Resource) Update(ctx context.Context, plan ResourceModel) (ResourceModel, error) {
	change := models.NewResourceUpdate()                                  // v1: models.NewResourceUpdate
	change.SetName(plan.Name)                                             // v1: models.ResourceUpdate.SetName
	change.SetDescription(plan.Description)                               // v1: models.ResourceUpdate.SetDescription
	updated, err := r.client.Api.Resources.Update(ctx, plan.Key, *change) // v1: api.Resources.Update
	if err != nil {
		return ResourceModel{}, err
	}
	return read(updated), nil
}

// Delete deletes the resource type.
func (r *Resource) Delete(ctx context.Context, key string) error {
	return r.client.Api.Resources.Delete(ctx, key) // v1: api.Resources.Delete
}

func read(resource *models.ResourceRead) ResourceModel { // v1: models.ResourceRead
	state := ResourceModel{Key: resource.Key, Name: resource.Name, Actions: map[string]string{}} // v1: models.ResourceRead.Key models.ResourceRead.Name
	if resource.Description != nil {                                                             // v1: models.ResourceRead.Description
		state.Description = *resource.Description // v1: models.ResourceRead.Description
	}
	if resource.Actions != nil { // v1: models.ResourceRead.Actions
		for key, action := range *resource.Actions { // v1: models.ResourceRead.Actions
			state.Actions[key] = action.GetName() // v1 missed: models.ActionBlockRead.GetName
		}
	}
	return state
}
