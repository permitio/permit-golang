package api

import (
	"context"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/openapi"
	"go.uber.org/zap"
)

type ImplicitGrants struct {
	permitBaseApi
}

func NewImplicitGrantsApi(client *openapi.APIClient, config *config.PermitConfig) *ImplicitGrants {
	return &ImplicitGrants{
		permitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

func (r *ImplicitGrants) Create(
	ctx context.Context,
	resourceId string,
	roleId string,
	derivedRuleCreate models.DerivedRoleRuleCreate,
) (*models.DerivedRoleRuleRead, error) {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", zap.Error(err))
		return nil, err
	}

	derivedRoleRead, httpRes, err := r.client.ImplicitGrantsApi.CreateImplicitGrant(ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
		roleId,
	).DerivedRoleRuleCreate(derivedRuleCreate).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error creating derived role", zap.Error(err))
		return nil, err
	}

	return derivedRoleRead, nil
}

func (r *ImplicitGrants) Delete(
	ctx context.Context,
	roleId string,
	resourceId string,
	derivedRoleRuleDelete models.DerivedRoleRuleDelete,
) error {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", zap.Error(err))
		return err
	}

	httpRes, err := r.client.ImplicitGrantsApi.DeleteImplicitGrant(ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
		roleId,
	).DerivedRoleRuleDelete(derivedRoleRuleDelete).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error creating derived role", zap.Error(err))
		return err
	}

	return nil
}

func (r *ImplicitGrants) UpdateConditions(
	ctx context.Context,
	resourceId string,
	roleId string,
	roleDerivationSettings models.PermitBackendSchemasSchemaDerivedRoleRuleDerivationSettings,
) (*models.PermitBackendSchemasSchemaDerivedRoleRuleDerivationSettings, error) {
	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", zap.Error(err))
		return nil, err
	}

	updatedSettings, httpRes, err := r.client.ImplicitGrantsApi.UpdateImplicitGrantsConditions(ctx,
		r.config.Context.GetProject(),
		r.config.Context.GetEnvironment(),
		resourceId,
		roleId,
	).PermitBackendSchemasSchemaDerivedRoleRuleDerivationSettings(roleDerivationSettings).Execute()

	err = errors.HttpErrorHandle(err, httpRes)

	if err != nil {
		r.logger.Error("error creating derived role", zap.Error(err))
		return nil, err
	}

	return updatedSettings, nil
}
