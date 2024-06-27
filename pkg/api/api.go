package api

import (
	"context"
	"fmt"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/openapi"
	"go.uber.org/zap"
	"time"
)

type permitBaseApi struct {
	client *openapi.APIClient
	config *config.PermitConfig
	logger *zap.Logger
}

type PermitBaseFactsApi struct {
	permitBaseApi
}

type IPermitBaseFactsApi interface {
	lazyLoadPermitContext(ctx context.Context, methodApiLevelArg ...config.APIKeyLevel) error
	WaitForSync(timeout *time.Duration) *PermitBaseFactsApi
}

type IPermitBaseApi interface {
	lazyLoadPermitContext(ctx context.Context, methodApiLevelArg ...config.APIKeyLevel) error
}

func (a *PermitBaseFactsApi) WaitForSync(timeout *time.Duration) *PermitBaseFactsApi {
	if a.config.GetProxyFactsViaPDP() {
		stringTimeout := ""
		if timeout == nil {
			if timeoutFromConfig := a.config.GetFactsSyncTimeout(); timeoutFromConfig != nil {
				stringTimeout = fmt.Sprintf("%d", int64(timeoutFromConfig.Seconds()))
			}
		}

		clientConfig := a.client.GetConfig()
		clientConfig.DefaultHeader["X-Wait-Timeout"] = stringTimeout
		return NewPermitBaseFactsApi(openapi.NewAPIClient(clientConfig), a.config)
	} else {
		a.logger.Warn("Attempted to wait for sync, but 'proxyFactsViaPdp' is not enabled. Ignoring")
		return a
	}
}

func (a *permitBaseApi) lazyLoadPermitContext(ctx context.Context, methodApiLevelArg ...config.APIKeyLevel) error {
	var methodApiLevel config.APIKeyLevel
	permitContext := a.config.Context.GetContext()

	if permitContext == nil {
		newPermitContext, err := config.PermitContextFactory(ctx, a.client, "", "", false)
		if err != nil {
			return err
		}
		a.config.Context = newPermitContext
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
			"project using `PermitClient.SetPermitContext()` method.")
	}
	if methodApiLevel == config.EnvironmentAPIKeyLevel && a.config.GetContext().GetProject() == "" && a.config.GetContext().GetEnvironment() == "" {
		return errors.NewPermitContextError("You're trying to use an SDK method that's specific to an environment," +
			"but you haven't set the current environment in your client's context yet," +
			"or you are using an organization/project level API key." +
			"Please set the context to a specific" +
			"environment using `PermitClient.SetPermitContext()` method.")
	}
	return nil

}

type PermitApiClient struct {
	config               *config.PermitConfig
	logger               *zap.Logger
	client               *openapi.APIClient
	ConditionSets        *ConditionSets
	Elements             *Elements
	Environments         *Environments
	ImplicitGrants       *ImplicitGrants
	Projects             *Projects
	ProxyConfigs         *ProxyConfigs
	RelationshipTuples   *RelationshipTuples
	ResourceActionGroups *ResourceActionGroups
	ResourceActions      *ResourceActions
	ResourceAttributes   *ResourceAttributes
	ResourceInstances    *ResourceInstances
	ResourceRelations    *ResourceRelations
	ResourceRoles        *ResourceRoles
	Resources            *Resources
	RoleAssignments      *RoleAssignments
	Roles                *Roles
	Tenants              *Tenants
	Users                *Users
}

func (p *PermitApiClient) SetContext(ctx context.Context, project string, environment string) {
	permitContext, err := config.PermitContextFactory(ctx, p.client, project, environment, true)
	if err != nil {
		p.logger.Error("", zap.Error(err))
	}
	p.config.Context = permitContext
}
func NewClientConfig(config *config.PermitConfig) *openapi.Configuration {
	clientConfig := openapi.NewConfiguration()
	clientConfig.Host = getHostFromUrl(config.GetApiUrl())
	clientConfig.Scheme = getSchemaFromUrl(config.GetApiUrl())
	clientConfig.AddDefaultHeader("Authorization", "Bearer "+config.GetToken())
	clientConfig.HTTPClient = config.GetHTTPClient()
	return clientConfig
}
func NewFactsClientConfig(config *config.PermitConfig) *openapi.Configuration {
	clientConfig := openapi.NewConfiguration()
	stringTimeout := ""

	//if timeout == nil {
	if timeoutFromConfig := config.GetFactsSyncTimeout(); timeoutFromConfig != nil {
		stringTimeout = fmt.Sprintf("%d", int64(timeoutFromConfig.Seconds()))
	}
	//}
	clientConfig.DefaultHeader["X-Wait-Timeout"] = stringTimeout

	clientConfig.AddDefaultHeader("Authorization", "Bearer "+config.GetToken())
	clientConfig.Host = getHostFromUrl(config.GetPdpUrl())
	clientConfig.Scheme = getSchemaFromUrl(config.GetPdpUrl())
	clientConfig.HTTPClient = config.GetHTTPClient()
	return clientConfig
}

func NewPermitApiClient(config *config.PermitConfig) *PermitApiClient {
	baseClientConfig := NewClientConfig(config)
	factsClientConfig := NewFactsClientConfig(config)
	client := openapi.NewAPIClient(baseClientConfig)
	factsClient := openapi.NewAPIClient(factsClientConfig)
	return &PermitApiClient{
		config:               config,
		logger:               config.Logger,
		client:               client,
		ConditionSets:        NewConditionSetsApi(client, config),
		Elements:             NewElementsApi(client, config),
		Environments:         NewEnvironmentsApi(client, config),
		ImplicitGrants:       NewImplicitGrantsApi(client, config),
		Projects:             NewProjectsApi(client, config),
		ProxyConfigs:         NewProxyConfigsApi(client, config),
		RelationshipTuples:   NewRelationshipTuplesApi(factsClient, config),
		ResourceActionGroups: NewResourceActionGroupsApi(client, config),
		ResourceActions:      NewResourceActionsApi(client, config),
		ResourceAttributes:   NewResourceAttributesApi(client, config),
		ResourceInstances:    NewResourceInstancesApi(factsClient, config),
		ResourceRelations:    NewResourceRelationsApi(client, config),
		ResourceRoles:        NewResourceRolesApi(client, config),
		Resources:            NewResourcesApi(client, config),
		RoleAssignments:      NewRoleAssignmentsApi(factsClient, config),
		Roles:                NewRolesApi(client, config),
		Tenants:              NewTenantsApi(factsClient, config),
		Users:                NewUsersApi(factsClient, config),
	}
}

func NewPermitBaseFactsApi(client *openapi.APIClient, config *config.PermitConfig) *PermitBaseFactsApi {
	return &PermitBaseFactsApi{
		permitBaseApi: permitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}
