package permit

import "github.com/permitio/permit-golang/openapi"

type APIKeyLevel int

const (
	NoneAPIKeyLevel APIKeyLevel = iota
	OrganizationAPIKeyLevel
	ProjectAPIKeyLevel
	EnvironmentAPIKeyLevel
)

func GetApiKeyLevel(scope *openapi.APIKeyScopeRead) APIKeyLevel {
	if *scope.EnvironmentId != "" {
		return EnvironmentAPIKeyLevel
	}
	if *scope.ProjectId != "" {
		return ProjectAPIKeyLevel
	}
	return OrganizationAPIKeyLevel
}
