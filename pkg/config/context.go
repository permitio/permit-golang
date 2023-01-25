package config

import (
	"context"
	"github.com/permitio/permit-golang/openapi"
	PermitErrors "github.com/permitio/permit-golang/pkg/errors"
	"strings"
)

type PermitContext struct {
	APIKeyLevel   APIKeyLevel
	ProjectId     string
	EnvironmentId string
}

type PermitContextInterface interface {
	GetProjectId() string
	GetEnvironmentId() string
	SetContext(project string, environment string)
	GetContext() *PermitContext
}

func (p *PermitContext) SetContext(project string, environment string, apiKeyLevel APIKeyLevel) {
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
		if strings.Contains(err.Error(), string(PermitErrors.ForbiddenMessage)) {
			return nil, PermitErrors.NewPermitContextError(PermitErrors.ForbiddenMessage)
		}
		if strings.Contains(err.Error(), string(PermitErrors.UnauthorizedMessage)) {
			return nil, PermitErrors.NewPermitContextError(PermitErrors.UnauthorizedMessage)
		}
		return nil, PermitErrors.NewPermitContextError(PermitErrors.EmptyErrorMessage)
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
					"client with a Project level API key, " +
					"please set a context with the API key related project")
			}
		}
		return NewPermitContext(apiKeyLevel, *apiKeysScopeRead.ProjectId, *apiKeysScopeRead.EnvironmentId), nil
	}
	return NewPermitContext(apiKeyLevel, *apiKeysScopeRead.ProjectId, *apiKeysScopeRead.EnvironmentId), nil
}

func NewPermitContext(apiKeyLevel APIKeyLevel, project string, environment string) *PermitContext {
	return &PermitContext{
		APIKeyLevel:   apiKeyLevel,
		ProjectId:     project,
		EnvironmentId: environment,
	}
}
