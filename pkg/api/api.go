package api

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/permitio/permit-golang/internal/redact"
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

// logGeneratedClientError logs err, an error that the generated client returned and that the
// calling method returns as it is, at Error level, with the API key and permit_key_ values in its
// text replaced by [REDACTED].
func (a *permitBaseApi) logGeneratedClientError(message string, err error) {
	a.logger.Error(message, zap.String("error", redact.Text(err.Error(), a.config.GetToken())))
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
				stringTimeout = formatSeconds(*timeoutFromConfig)
			}
		} else {
			stringTimeout = formatSeconds(*timeout)
		}

		clientConfig := copyClientConfig(a.client.GetConfig())
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

// copyClientConfig returns a copy of cfg with its own DefaultHeader map, so headers set on the
// copy never reach requests made with cfg, which other goroutines may be sending concurrently.
func copyClientConfig(cfg *openapi.Configuration) *openapi.Configuration {
	clone := *cfg
	clone.DefaultHeader = make(map[string]string, len(cfg.DefaultHeader))
	for key, value := range cfg.DefaultHeader {
		clone.DefaultHeader[key] = value
	}
	return &clone
}

// formatSeconds formats d as a decimal number of seconds for the X-Wait-Timeout header, without
// truncating fractions or using an exponent (1.5s is "1.5", 1ns is "0.000000001"). A single
// division keeps millisecond values exact, where d.Seconds() can give "1.1179999999999999".
func formatSeconds(d time.Duration) string {
	return strconv.FormatFloat(float64(d)/float64(time.Second), 'f', -1, 64)
}

func (a *permitBaseApi) lazyLoadPermitContext(ctx context.Context, methodApiLevelArg ...config.APIKeyLevel) error {
	var methodApiLevel config.APIKeyLevel
	if err := a.loadPermitContext(ctx); err != nil {
		return err
	}

	if len(methodApiLevelArg) == 0 {
		methodApiLevel = config.EnvironmentAPIKeyLevel
	} else {
		methodApiLevel = methodApiLevelArg[0]
	}
	if methodApiLevel == config.ProjectAPIKeyLevel && a.config.GetContext().GetEnvironment() == "" {
		return errors.NewPermitContextError("This method needs a project and an environment in " +
			"the client's context. Call client.Api.SetContext(ctx, project, environment) first, " +
			"with both.")
	}
	if methodApiLevel == config.EnvironmentAPIKeyLevel && a.config.GetContext().GetProject() == "" && a.config.GetContext().GetEnvironment() == "" {
		return errors.NewPermitContextError("The client's context has no project or environment. " +
			"Call client.Api.SetContext(ctx, project, environment) first, with the project and " +
			"the environment to use.")
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

// SetContext sets the project and environment that the client's methods use. Call it once,
// after permit.New and before other calls, with an organization or a project API key. It does
// not return an error: when it fails, it logs the error and clears the project and environment
// that an earlier call set.
func (p *PermitApiClient) SetContext(ctx context.Context, project string, environment string) {
	permitContext, err := config.PermitContextFactory(ctx, p.client, project, environment, true)
	if err != nil {
		p.logger.Error(fmt.Sprintf("SetContext could not set project %q and environment %q, "+
			"so the client has no context", project, environment), zap.Error(err))
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
			stringTimeout := formatSeconds(*timeoutFromConfig)
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
	if config.Logger == nil {
		config.Logger = zap.NewNop()
	}
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
