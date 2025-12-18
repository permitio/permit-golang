package api

import (
	"context"
	"fmt"
	"time"

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

type PermitBaseFactsApi struct {
	permitBaseApi
}

// WaitForSyncOptions contains options for configuring facts synchronization behavior
type WaitForSyncOptions struct {
	// Policy specifies what to do when timeout is reached ("ignore" or "fail")
	// When "ignore" is specified, the request will continue processing even if facts sync times out.
	// When "fail" is specified, the request will fail with an error if facts sync times out.
	Policy config.FactsSyncTimeoutPolicy
}

type IPermitBaseFactsApi interface {
	lazyLoadPermitContext(ctx context.Context, methodApiLevelArg ...config.APIKeyLevel) error
	WaitForSync(timeout *time.Duration, options WaitForSyncOptions) *PermitBaseFactsApi
}

type IPermitBaseApi interface {
	lazyLoadPermitContext(ctx context.Context, methodApiLevelArg ...config.APIKeyLevel) error
}

// WaitForSync configures the client to wait for facts synchronization.
//
// Parameters:
//   - timeout: Required duration to wait for synchronization
//   - options: Additional configuration options for facts synchronization
func (a *PermitBaseFactsApi) WaitForSync(timeout *time.Duration, options WaitForSyncOptions) *PermitBaseFactsApi {
	if a.config.GetProxyFactsViaPDP() {
		stringTimeout := ""
		if timeout == nil {
			if timeoutFromConfig := a.config.GetFactsSyncTimeout(); timeoutFromConfig != nil {
				stringTimeout = fmt.Sprintf("%d", int64(timeoutFromConfig.Seconds()))
			}
		} else {
			stringTimeout = fmt.Sprintf("%d", int64(timeout.Seconds()))
		}

		clientConfig := a.client.GetConfig()
		clientConfig.DefaultHeader["X-Wait-Timeout"] = stringTimeout

		// Add the timeout policy header if a policy is provided or set in the config
		if options.Policy != "" {
			clientConfig.DefaultHeader["X-Timeout-Policy"] = string(options.Policy)
		} else if a.config.GetFactsSyncTimeoutPolicy() != "" {
			clientConfig.DefaultHeader["X-Timeout-Policy"] = string(a.config.GetFactsSyncTimeoutPolicy())
		}

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
		// To ensure it's a regular client and not factsClient
		baseClientConfig := NewClientConfig(a.config)
		client := openapi.NewAPIClient(baseClientConfig)
		newPermitContext, err := config.PermitContextFactory(ctx, client, "", "", false)
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

	// Only add timeout-related headers if proxyFactsViaPDP is enabled
	if config.GetProxyFactsViaPDP() {
		// Add X-Wait-Timeout header only if factsSyncTimeout is set
		if timeoutFromConfig := config.GetFactsSyncTimeout(); timeoutFromConfig != nil {
			stringTimeout := fmt.Sprintf("%d", int64(timeoutFromConfig.Seconds()))
			clientConfig.DefaultHeader["X-Wait-Timeout"] = stringTimeout
		}

		// Add X-Timeout-Policy header only if factsSyncTimeoutPolicy is set
		if policy := config.GetFactsSyncTimeoutPolicy(); policy != "" {
			clientConfig.DefaultHeader["X-Timeout-Policy"] = string(policy)
		}
	}

	clientConfig.AddDefaultHeader("Authorization", "Bearer "+config.GetToken())
	clientConfig.Host = getHostFromUrl(config.GetPdpUrl())
	clientConfig.Scheme = getSchemaFromUrl(config.GetPdpUrl())
	clientConfig.HTTPClient = config.GetHTTPClient()
	return clientConfig
}

func NewPermitApiClient(config *config.PermitConfig) *PermitApiClient {
	baseClientConfig := NewClientConfig(config)
	client := openapi.NewAPIClient(baseClientConfig)

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
		RelationshipTuples:   NewRelationshipTuplesApi(client, config),
		ResourceActionGroups: NewResourceActionGroupsApi(client, config),
		ResourceActions:      NewResourceActionsApi(client, config),
		ResourceAttributes:   NewResourceAttributesApi(client, config),
		ResourceInstances:    NewResourceInstancesApi(client, config),
		ResourceRelations:    NewResourceRelationsApi(client, config),
		ResourceRoles:        NewResourceRolesApi(client, config),
		Resources:            NewResourcesApi(client, config),
		RoleAssignments:      NewRoleAssignmentsApi(client, config),
		Roles:                NewRolesApi(client, config),
		Tenants:              NewTenantsApi(client, config),
		Users:                NewUsersApi(client, config),
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
