package config

import (
	"context"
	PermitErrors "github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/openapi"
)

type PermitContext struct {
	APIKeyLevel   APIKeyLevel
	ProjectId     string
	EnvironmentId string
}

type PermitContextInterface interface {
	SetPermitContext(project string, environment string, apiKeyLevel APIKeyLevel)
	GetProject() string
	GetEnvironment() string
	GetContext() *PermitContext
}

func (p *PermitContext) SetPermitContext(project string, environment string, apiKeyLevel APIKeyLevel) {
	p.ProjectId = project
	p.EnvironmentId = environment
	p.APIKeyLevel = apiKeyLevel
}

func (p *PermitContext) GetContext() *PermitContext {
	return p
}

func (p *PermitContext) GetEnvironment() string {
	return p.EnvironmentId
}

func (p *PermitContext) GetProject() string {
	return p.ProjectId
}

func PermitContextFactory(ctx context.Context, client *openapi.APIClient, project string, environment string, isUserInput bool) (*PermitContext, error) {
	apiKeysScopeRead, httpRes, err := client.APIKeysApi.GetApiKeyScope(ctx).Execute()
	err = PermitErrors.HttpErrorHandle(err, httpRes)
	if err != nil {
		return nil, PermitErrors.NewPermitContextErrorWithCause("the API key scope request failed", err)
	}
	apiKeyLevel := GetApiKeyLevel(apiKeysScopeRead)
	if isUserInput {
		if apiKeyLevel == EnvironmentAPIKeyLevel {
			if environment == "" || project == "" {
				return nil, PermitErrors.NewPermitContextError("You initiated the Permit.io " +
					"client with an Environment level API key, " +
					"please set a context with the API key related environment and project")
			}
		}
		if apiKeyLevel == ProjectAPIKeyLevel {
			if project == "" {
				return nil, PermitErrors.NewPermitContextError("You initiated the Permit.io " +
					"client with a project API key and gave SetContext no project. Pass the " +
					"API key's project, and an environment for the methods that work in an " +
					"environment, such as Users")
			}
		}
		return NewPermitContext(apiKeyLevel, project, environment), nil
	}
	return NewPermitContext(apiKeyLevel, apiKeysScopeRead.GetProjectId(), apiKeysScopeRead.GetEnvironmentId()), nil
}

func NewPermitContext(apiKeyLevel APIKeyLevel, project string, environment string) *PermitContext {
	return &PermitContext{
		APIKeyLevel:   apiKeyLevel,
		ProjectId:     project,
		EnvironmentId: environment,
	}
}
