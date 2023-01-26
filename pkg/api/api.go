package api

import (
	"context"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/openapi"
	"go.uber.org/zap"
)

type permitBaseApi struct {
	client *openapi.APIClient
	config *config.PermitConfig
	logger *zap.Logger
}

type IPermitBaseApi interface {
	lazyLoadContext(ctx context.Context, methodApiLevelArg ...config.APIKeyLevel) error
}

func (a *permitBaseApi) lazyLoadContext(ctx context.Context, methodApiLevelArg ...config.APIKeyLevel) error {
	var methodApiLevel config.APIKeyLevel
	permitContext := a.config.Context.GetContext()
	if permitContext == nil {
		newPermitContext, err := config.PermitContextFactory(ctx, a.client, "", "", false)
		if err != nil {
			return err
		}
		a.config.Context = newPermitContext
	} else {
		a.logger.Info("Context already loaded")
	}
	if len(methodApiLevelArg) == 0 {
		methodApiLevel = config.EnvironmentAPIKeyLevel
	} else {
		methodApiLevel = methodApiLevelArg[0]
	}
	if methodApiLevel == config.ProjectAPIKeyLevel && a.config.GetContext().GetEnvironment() == "" {
		return errors.NewPermitContextError("You're trying to use an SDK method that's specific to a project," +
			"but you haven't set the current project in your client's context yet," +
			"or you are using an organization level API key." +
			"Please set the context to a specific" +
			"project using `PermitClient.SetContext()` method.")
	}
	if methodApiLevel == config.EnvironmentAPIKeyLevel && a.config.GetContext().GetProject() == "" && a.config.GetContext().GetEnvironment() == "" {
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
	permitContext, err := config.PermitContextFactory(p.ctx, p.client, project, environment, true)
	if err != nil {
		p.logger.Error("", zap.Error(err))
	}
	p.config.Context = permitContext
}

func NewPermitApiClient(ctx context.Context, config *config.PermitConfig) *PermitApiClient {
	clientConfig := openapi.NewConfiguration()
	clientConfig.Host = getHostFromUrl(config.GetApiUrl())
	clientConfig.Scheme = getSchemaFromUrl(config.GetApiUrl())
	clientConfig.AddDefaultHeader("Authorization", "Bearer "+config.GetToken())
	client := openapi.NewAPIClient(clientConfig)
	userApi := NewUsersApi(client, config)
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
		Users:              userApi,
		Elements:           NewElementsApi(client, config),
	}
}
