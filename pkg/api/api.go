package api

import (
	"context"
	"github.com/permitio/permit-golang/openapi"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"go.uber.org/zap"
)

type permitBaseApi struct {
	client *openapi.APIClient
	config *config.PermitConfig
	logger *zap.Logger
}

type IPermitBaseApi interface {
	LazyLoadContext() error
}

func (a *permitBaseApi) lazyLoadContext(ctx context.Context, methodApiLevelArg ...config.APIKeyLevel) error {
	var methodApiLevel config.APIKeyLevel
	permitContext := a.config.Context.GetContext()
	if permitContext == nil {
		permitContext, err := config.PermitContextFactory(ctx, a.logger, a.client, "", "", false)
		if err != nil {
			return err
		}
		a.config.Context = permitContext
	} else {
		a.logger.Info("Context already loaded")
	}
	if len(methodApiLevelArg) == 0 {
		methodApiLevel = config.EnvironmentAPIKeyLevel
	} else {
		methodApiLevel = methodApiLevelArg[0]
	}
	if methodApiLevel == config.ProjectAPIKeyLevel && permitContext.EnvironmentId == "" {
		return errors.NewPermitContextError("You're trying to use an SDK method that's specific to a project," +
			"but you haven't set the current project in your client's context yet," +
			"or you are using an organization level API key." +
			"Please set the context to a specific" +
			"project using `PermitClient.SetContext()` method.")
	}
	if methodApiLevel == config.EnvironmentAPIKeyLevel && permitContext.ProjectId == "" && permitContext.EnvironmentId == "" {
		return errors.NewPermitContextError("You're trying to use an SDK method that's specific to an environment," +
			"but you haven't set the current environment in your client's context yet," +
			"or you are using an organization/project level API key." +
			"Please set the context to a specific" +
			"environment using `PermitClient.SetContext()` method.")
	}
	return nil

}

type PermitApiClient struct {
	ctx                context.Context
	config             *config.PermitConfig
	logger             *zap.Logger
	client             *openapi.APIClient
	Tenants            *Tenants
	Environments       *Environments
	Projects           *Projects
	ResourceActions    *ResourceActions
	ResourceAttributes *ResourceAttributes
	Resources          *Resources
	Roles              *Roles
	Users              *Users
	Elements           *Elements
}

func (p *PermitApiClient) SetContext(project string, environment string) {
	permitContext, err := config.PermitContextFactory(p.ctx, p.logger, p.client, project, environment, true)
	if err != nil {
		p.logger.Error("", zap.Error(err))
	}
	p.config.Context = permitContext
}

func NewPermitApiClient(ctx context.Context, config *config.PermitConfig) *PermitApiClient {
	client := openapi.NewAPIClient(openapi.NewConfiguration())
	return &PermitApiClient{
		config:             config,
		ctx:                ctx,
		logger:             config.Logger,
		Tenants:            NewTenantsApi(client, config),
		Environments:       NewEnvironmentsApi(client, config),
		Projects:           NewProjectsApi(client, config),
		ResourceActions:    NewResourceActionsApi(client, config),
		ResourceAttributes: NewResourceAttributesApi(client, config),
		Resources:          NewResourcesApi(client, config),
		Roles:              NewRolesApi(client, config),
		Users:              NewUsersApi(client, config),
		Elements:           NewElementsApi(client, config),
	}
}
