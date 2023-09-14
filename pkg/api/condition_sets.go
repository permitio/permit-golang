package api

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/openapi"
	"go.uber.org/zap"
)

type ConditionSets struct {
	permitBaseApi
}

func NewConditionSetsApi(client *openapi.APIClient, config *config.PermitConfig) *ConditionSets {
	return &ConditionSets{
		permitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

// List all condition sets in the current environment.
// Usage Example:
// `condition_sets, err := PermitClient.Api.ConditionSets.List(ctx,1, 10)`
func (c *ConditionSets) List(ctx context.Context, page int, perPage int) ([]models.ConditionSetRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)

	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		c.logger.Error("error listing condition sets - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}

	err := c.lazyLoadPermitContext(ctx)

	if err != nil {
		c.logger.Error("", zap.Error(err))
		return nil, err
	}

	conditionSets, httpRes, err := c.client.ConditionSetsApi.ListConditionSets(
		ctx,
		c.config.Context.GetProject(),
		c.config.Context.GetEnvironment(),
	).Page(int32(page)).PerPage(int32(perPage)).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		c.logger.Error("error listing condition sets", zap.Error(err))
		return nil, err
	}

	return conditionSets.PaginatedResultConditionSetRead.GetData(), nil
}

// Get a condition set by key.
// Usage Example:
// `conditionSet, err := PermitClient.Api.ConditionSets.Get(ctx, "cs-key")`
func (c *ConditionSets) Get(ctx context.Context, conditionSetKey string) (*models.ConditionSetRead, error) {
	err := c.lazyLoadPermitContext(ctx)

	if err != nil {
		c.logger.Error("", zap.Error(err))
		return nil, err
	}

	conditionSet, httpRes, err := c.client.ConditionSetsApi.GetConditionSet(
		ctx,
		c.config.Context.GetProject(),
		c.config.Context.GetEnvironment(), conditionSetKey,
	).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		c.logger.Error("error getting condition set: "+conditionSetKey, zap.Error(err))
		return nil, err
	}

	return conditionSet, nil
}

// GetByKey gets a condition set by key from your context's environment.
// Usage Example:
//
//	`cs, err := PermitClient.Api.ConditionSets.GetByKey(ctx, "cs-key")`
func (c *ConditionSets) GetByKey(ctx context.Context, conditionSetKey string) (*models.ConditionSetRead, error) {
	return c.Get(ctx, conditionSetKey)
}

// GetById gets a condition set by id from your context's environment.
// Usage Example:
//
//	`cs, err := PermitClient.Api.ConditionSets.GetById(ctx, uuid.New())`
func (c *ConditionSets) GetById(ctx context.Context, conditionSetId uuid.UUID) (*models.ConditionSetRead, error) {
	return c.Get(ctx, conditionSetId.String())
}

// Create a new condition set.
// Usage Example:
// ```
// csCreate := models.NewConditionSetCreate("cs-key", "cs-name")
// cs, err := PermitClient.Api.ConditionSets.Create(ctx, csCreate)
// ```
func (c *ConditionSets) Create(ctx context.Context, csCreate models.ConditionSetCreate) (*models.ConditionSetRead, error) {
	err := c.lazyLoadPermitContext(ctx)

	if err != nil {
		c.logger.Error("", zap.Error(err))
		return nil, err
	}

	conditionSet, httpRes, err := c.client.ConditionSetsApi.CreateConditionSet(
		ctx, c.config.Context.GetProject(),
		c.config.Context.GetEnvironment(),
	).ConditionSetCreate(csCreate).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		c.logger.Error("error creating condition set: "+csCreate.GetKey(), zap.Error(err))
		return nil, err
	}

	return conditionSet, nil
}

// Update a condition set.
func (c *ConditionSets) Update(ctx context.Context, conditionSetKey string, conditionSetUpdate models.ConditionSetUpdate) (*models.ConditionSetRead, error) {
	err := c.lazyLoadPermitContext(ctx)

	if err != nil {
		c.logger.Error("", zap.Error(err))
		return nil, err
	}

	conditionSet, httpRes, err := c.client.ConditionSetsApi.UpdateConditionSet(
		ctx, c.config.Context.GetProject(),
		c.config.Context.GetEnvironment(),
		conditionSetKey,
	).ConditionSetUpdate(conditionSetUpdate).Execute()

	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		c.logger.Error("error updating condition set: "+conditionSetKey, zap.Error(err))
		return nil, err
	}
	return conditionSet, nil
}

// Delete a condition set.
func (c *ConditionSets) Delete(ctx context.Context, conditionSetKey string) error {
	err := c.lazyLoadPermitContext(ctx)

	if err != nil {
		c.logger.Error("", zap.Error(err))
		return err
	}

	httpRes, err := c.client.ConditionSetsApi.DeleteConditionSet(
		ctx,
		c.config.Context.GetProject(),
		c.config.Context.GetEnvironment(),
		conditionSetKey,
	).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		c.logger.Error("error deleting condition set: "+conditionSetKey, zap.Error(err))
		return err
	}

	return nil
}

func (c *ConditionSets) AssignSetPermissions(ctx context.Context, userSetKey string, permission string, resourceSetKey string) ([]models.ConditionSetRuleRead, error) {
	err := c.lazyLoadPermitContext(ctx)

	if err != nil {
		c.logger.Error("", zap.Error(err))
		return nil, err
	}

	rule, httpRes, err := c.client.ConditionSetRulesApi.AssignSetPermissions(
		ctx,
		c.config.Context.GetProject(),
		c.config.Context.GetEnvironment(),
	).ConditionSetRuleCreate(*models.NewConditionSetRuleCreate(userSetKey, permission, resourceSetKey)).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		errString := fmt.Sprintf("error creating condition set rule %s, %s, %s", userSetKey, permission, resourceSetKey)
		c.logger.Error("error creating condition set rule: "+errString, zap.Error(err))
		return nil, err
	}

	return rule, nil
}

func (c *ConditionSets) UnassignSetPermissions(ctx context.Context, userSetKey string, permission string, resourceSetKey string) error {
	err := c.lazyLoadPermitContext(ctx)

	if err != nil {
		c.logger.Error("", zap.Error(err))
		return err
	}

	httpRes, err := c.client.ConditionSetRulesApi.UnassignSetPermissions(
		ctx,
		c.config.Context.GetProject(),
		c.config.Context.GetEnvironment(),
	).ConditionSetRuleRemove(*models.NewConditionSetRuleRemove(userSetKey, permission, resourceSetKey)).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		errString := fmt.Sprintf("error creating condition set rule %s, %s, %s", userSetKey, permission, resourceSetKey)
		c.logger.Error("error creating condition set rule: "+errString, zap.Error(err))
		return err
	}

	return nil
}
