package enforcement

import (
	"github.com/permitio/permit-golang/openapi"
	"github.com/permitio/permit-golang/pkg/config"
	"go.uber.org/zap"
)

type PermitBaseEnforcer struct {
	client *openapi.APIClient
	config *config.PermitConfig
	logger *zap.Logger
}

type PermitEnforcer struct {
	PermitBaseEnforcer
}

func NewPermitEnforcerClient(config *config.PermitConfig) *PermitEnforcer {
	client := openapi.NewAPIClient(openapi.NewConfiguration())
	return &PermitEnforcer{
		PermitBaseEnforcer{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}
