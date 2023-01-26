package config

import (
	"github.com/permitio/permit-golang/pkg/models"
)

type APIKeyLevel int

const (
	NoneAPIKeyLevel APIKeyLevel = iota
	OrganizationAPIKeyLevel
	ProjectAPIKeyLevel
	EnvironmentAPIKeyLevel
)

func GetApiKeyLevel(scope *models.APIKeyScopeRead) APIKeyLevel {
	if *scope.EnvironmentId != "" {
		return EnvironmentAPIKeyLevel
	}
	if *scope.ProjectId != "" {
		return ProjectAPIKeyLevel
	}
	return OrganizationAPIKeyLevel
}
