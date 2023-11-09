package enforcement

import (
	"github.com/permitio/permit-golang/pkg/config"
	"go.uber.org/zap"
	"net/http"
)

type PermitBaseEnforcer struct {
	config *config.PermitConfig
	logger *zap.Logger
	client *http.Client
}

type PermitEnforcer struct {
	PermitBaseEnforcer
}

func NewPermitEnforcerClient(config *config.PermitConfig) *PermitEnforcer {
	client := config.GetHTTPClient()
	if client == nil {
		client = http.DefaultClient
	}
	return &PermitEnforcer{
		PermitBaseEnforcer{
			config: config,
			logger: config.GetLogger(),
			client: client,
		},
	}
}

func (e *PermitEnforcer) getEndpointByPolicyPackage(name packageName) string {
	operationConfig := policyMap[name]
	if e.config.GetOpaUrl() != "" {
		return e.config.GetOpaUrl() + "/v1/data/" + operationConfig.opaPath
	} else {
		return e.config.GetPdpUrl() + string(operationConfig.sidecarPath)
	}
}
