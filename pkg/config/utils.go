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
	if scope.HasEnvironmentId() {
		return EnvironmentAPIKeyLevel
	}
	if scope.HasProjectId() {
		return ProjectAPIKeyLevel
	}
	return OrganizationAPIKeyLevel
}
