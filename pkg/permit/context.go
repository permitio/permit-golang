package permit

import (
	"context"
	"errors"
	"github.com/permitio/permit-golang/openapi"
	PermitErrors "github.com/permitio/permit-golang/pkg/errors"
	"go.uber.org/zap"
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
}

func (p *PermitContext) GetProjectId() string {
	return p.ProjectId
}

func (p *PermitContext) GetEnvironmentId() string {
	return p.EnvironmentId
}

func PermitContextFactory(ctx context.Context, logger *zap.Logger, client *openapi.APIClient, project string, environment string, isUserInput bool) (*PermitContext, error) {
	apiKeysScopeRead, httpRes, err := client.APIKeysApi.GetApiKeyScope(ctx).Execute()
	err = PermitErrors.HttpErrorHandle(err, httpRes)
	if errors.Is(err, PermitErrors.NewPermitForbiddenError()) {
		return nil, PermitErrors.NewPermitContextError(PermitErrors.ForbiddenMessage)
	}
	if errors.Is(err, PermitErrors.NewPermitUnauthorizedError()) {
		return nil, PermitErrors.NewPermitContextError(PermitErrors.UnauthorizedMessage)
	}
	if errors.Is(err, PermitErrors.NewPermitUnexpectedError(err)) || err != nil {
		return nil, PermitErrors.NewPermitContextError(PermitErrors.EmptyErrorMessage)
	}
	apiKeyLevel := GetApiKeyLevel(apiKeysScopeRead)
	if isUserInput {
		return NewPermitContext(apiKeyLevel, *apiKeysScopeRead.ProjectId, *apiKeysScopeRead.EnvironmentId), nil
	}
	if apiKeyLevel == EnvironmentAPIKeyLevel {
		if environment == "" || project == "" {
			return nil, PermitErrors.NewPermitContextError("You initiated the Permit.io " +
				"Client with an Environment level API key, " +
				"please set a context with the API key related environment and project")
		}
	}
	if apiKeyLevel == ProjectAPIKeyLevel {
		if project == "" {
			return nil, PermitErrors.NewPermitContextError("You initiated the Permit.io " +
				"Client with a Project level API key, " +
				"please set a context with the API key related project")
		}
	}
	return NewPermitContext(apiKeyLevel, project, environment), nil
}

func NewPermitContext(apiKeyLevel APIKeyLevel, project string, environment string) *PermitContext {
	return &PermitContext{
		APIKeyLevel:   apiKeyLevel,
		ProjectId:     project,
		EnvironmentId: environment,
	}
}
