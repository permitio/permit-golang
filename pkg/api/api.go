package api

import (
	"context"
	"github.com/permitio/permit-golang/openapi"
	"github.com/permitio/permit-golang/pkg/errors"
	permit "github.com/permitio/permit-golang/pkg/permit"
	"go.uber.org/zap"
)

type PermitBaseApi struct {
	client *openapi.APIClient
	config *permit.PermitConfig
	logger *zap.Logger
}

type IPermitBaseApi interface {
	LazyLoadContext() error
}

func (a *PermitBaseApi) LazyLoadContext(ctx context.Context, methodApiLevelArg ...permit.APIKeyLevel) error {
	var methodApiLevel permit.APIKeyLevel
	permitContext := a.config.Context.GetContext()
	if permitContext == nil {
		permitContext, err := permit.PermitContextFactory(ctx, a.logger, a.client, "", "", false)
		if err != nil {
			return err
		}
		a.config.Context = permitContext
	} else {
		a.logger.Info("Context already loaded")
	}
	if len(methodApiLevelArg) == 0 {
		methodApiLevel = permit.EnvironmentAPIKeyLevel
	} else {
		methodApiLevel = methodApiLevelArg[0]
	}
	if methodApiLevel == permit.ProjectAPIKeyLevel && permitContext.EnvironmentId == "" {
		return errors.NewPermitContextError("You're trying to use an SDK method that's specific to a project," +
			"but you haven't set the current project in your client's context yet," +
			"or you are using an organization level API key." +
			"Please set the context to a specific" +
			"project using `Permit.SetContext()` method.")
	}
	if methodApiLevel == permit.EnvironmentAPIKeyLevel && permitContext.ProjectId == "" && permitContext.EnvironmentId == "" {
		return errors.NewPermitContextError("You're trying to use an SDK method that's specific to an environment," +
			"but you haven't set the current environment in your client's context yet," +
			"or you are using an organization/project level API key." +
			"Please set the context to a specific" +
			"environment using `Permit.SetContext()` method.")
	}
	return nil

}

type PermitApiClient struct {
	ctx                context.Context
	config             *permit.PermitConfig
	logger             *zap.Logger
	Client             *openapi.APIClient
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
	permitContext, err := permit.PermitContextFactory(p.ctx, p.logger, p.Client, project, environment, true)
	if err != nil {
		p.logger.Error("", zap.Error(err))
	}
	p.config.Context = permitContext
}

func NewPermitApiClient(ctx context.Context, config *permit.PermitConfig) *PermitApiClient {
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
