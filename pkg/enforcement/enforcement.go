package enforcement

import (
	"github.com/permitio/permit-golang/pkg/config"
	"go.uber.org/zap"
)

type PermitBaseEnforcer struct {
	config *config.PermitConfig
	logger *zap.Logger
}

type PermitEnforcer struct {
	PermitBaseEnforcer
}

func NewPermitEnforcerClient(config *config.PermitConfig) *PermitEnforcer {
	return &PermitEnforcer{
		PermitBaseEnforcer{
			config: config,
			logger: config.Logger,
		},
	}
}
