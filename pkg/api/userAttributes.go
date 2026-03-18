package api

import (
	"context"

	"github.com/google/uuid"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/openapi"
	"go.uber.org/zap"
)

// DefaultUserAttributeResourceID is the default resource_id query parameter for user schema attributes (V2 API).
const DefaultUserAttributeResourceID = "__user"

type UserAttributes struct {
	permitBaseApi
}

func NewUserAttributesApi(client *openapi.APIClient, config *config.PermitConfig) *UserAttributes {
	return &UserAttributes{
		permitBaseApi: permitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

// List returns all user attributes for the User resource (resource_id __user).
//
//	attrs, err := PermitClient.Api.UserAttributes.List(ctx, 1, 30)
func (a *UserAttributes) List(ctx context.Context, page int, perPage int) ([]models.UserAttributeRead, error) {
	return a.ListForResource(ctx, DefaultUserAttributeResourceID, page, perPage)
}

// ListForResource lists user attributes when using a non-default user resource id.
func (a *UserAttributes) ListForResource(ctx context.Context, resourceID string, page int, perPage int) ([]models.UserAttributeRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		a.logger.Error("error listing user attributes - max per page exceeded", zap.Error(err))
		return nil, err
	}
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return nil, err
	}
	attrs, _, err := a.client.UserAttributesApi.ListUserAttributes(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment()).
		ResourceId(resourceID).Page(int32(page)).PerPage(int32(perPage)).Execute()
	if err != nil {
		a.logger.Error("error listing user attributes", zap.Error(err))
		return nil, err
	}
	return attrs, nil
}

// Get returns a user attribute by id or key (slug).
//
//	attr, err := PermitClient.Api.UserAttributes.Get(ctx, "clearance_level")
func (a *UserAttributes) Get(ctx context.Context, attributeIDOrKey string) (*models.UserAttributeRead, error) {
	return a.GetForResource(ctx, DefaultUserAttributeResourceID, attributeIDOrKey)
}

// GetForResource gets a user attribute with an explicit resource_id query.
func (a *UserAttributes) GetForResource(ctx context.Context, resourceID, attributeIDOrKey string) (*models.UserAttributeRead, error) {
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return nil, err
	}
	attr, _, err := a.client.UserAttributesApi.GetUserAttribute(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), attributeIDOrKey).
		ResourceId(resourceID).Execute()
	if err != nil {
		a.logger.Error("error getting user attribute: "+attributeIDOrKey, zap.Error(err))
		return nil, err
	}
	return attr, nil
}

// GetByKey is an alias for Get (attribute key is the slug).
func (a *UserAttributes) GetByKey(ctx context.Context, attributeKey string) (*models.UserAttributeRead, error) {
	return a.Get(ctx, attributeKey)
}

// GetById gets a user attribute by attribute UUID (uses default user resource __user).
func (a *UserAttributes) GetById(ctx context.Context, attributeID uuid.UUID) (*models.UserAttributeRead, error) {
	return a.Get(ctx, attributeID.String())
}

// Create adds a user schema attribute.
//
//	create := models.NewUserAttributeCreate("department", models.STRING)
//	create.SetDescription("User department")
//	attr, err := PermitClient.Api.UserAttributes.Create(ctx, *create)
func (a *UserAttributes) Create(ctx context.Context, body models.UserAttributeCreate) (*models.UserAttributeRead, error) {
	return a.CreateForResource(ctx, DefaultUserAttributeResourceID, body)
}

// CreateForResource creates a user attribute with an explicit resource_id query.
func (a *UserAttributes) CreateForResource(ctx context.Context, resourceID string, body models.UserAttributeCreate) (*models.UserAttributeRead, error) {
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return nil, err
	}
	attr, _, err := a.client.UserAttributesApi.CreateUserAttribute(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment()).
		UserAttributeCreate(body).ResourceId(resourceID).Execute()
	if err != nil {
		a.logger.Error("error creating user attribute: "+body.GetKey(), zap.Error(err))
		return nil, err
	}
	return attr, nil
}

// Update partially updates a user attribute (id or key).
func (a *UserAttributes) Update(ctx context.Context, attributeIDOrKey string, body models.UserAttributeUpdate) (*models.UserAttributeRead, error) {
	return a.UpdateForResource(ctx, DefaultUserAttributeResourceID, attributeIDOrKey, body)
}

// UpdateForResource updates with an explicit resource_id query.
func (a *UserAttributes) UpdateForResource(ctx context.Context, resourceID, attributeIDOrKey string, body models.UserAttributeUpdate) (*models.UserAttributeRead, error) {
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return nil, err
	}
	attr, _, err := a.client.UserAttributesApi.UpdateUserAttribute(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), attributeIDOrKey).
		UserAttributeUpdate(body).ResourceId(resourceID).Execute()
	if err != nil {
		a.logger.Error("error updating user attribute: "+attributeIDOrKey, zap.Error(err))
		return nil, err
	}
	return attr, nil
}

// Delete removes a user attribute (id or key).
func (a *UserAttributes) Delete(ctx context.Context, attributeIDOrKey string) error {
	return a.DeleteForResource(ctx, DefaultUserAttributeResourceID, attributeIDOrKey)
}

// DeleteForResource deletes with an explicit resource_id query.
func (a *UserAttributes) DeleteForResource(ctx context.Context, resourceID, attributeIDOrKey string) error {
	err := a.lazyLoadPermitContext(ctx)
	if err != nil {
		return err
	}
	_, err = a.client.UserAttributesApi.DeleteUserAttribute(ctx, a.config.Context.GetProject(), a.config.Context.GetEnvironment(), attributeIDOrKey).
		ResourceId(resourceID).Execute()
	if err != nil {
		a.logger.Error("error deleting user attribute: "+attributeIDOrKey, zap.Error(err))
		return err
	}
	return nil
}
