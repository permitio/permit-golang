package enforcement

import (
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
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
	if config.Logger == nil {
		config.Logger = zap.NewNop()
	}
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

// logErrorResponse logs err, made from a PDP answer with an error status, at Error level with
// its status code, and the response body it holds only at Debug level. errors.NewPermitError has
// already replaced the credentials in that body with [REDACTED].
func (e *PermitEnforcer) logErrorResponse(operation string, err error) {
	message := "erroneous http response from PDP for " + operation
	permitErr, ok := err.(errors.PermitError)
	if !ok {
		e.logger.Error(message, zap.Error(err))
		return
	}
	e.logger.Error(message, zap.Error(permitErr), zap.Int("status_code", permitErr.StatusCode))
	e.logger.Debug("response body from PDP for "+operation,
		zap.String("response_body", permitErr.ResponseBody))
}

func (e *PermitEnforcer) getEndpointByPolicyPackage(name packageName) string {
	operationConfig := policyMap[name]
	if e.config.GetOpaUrl() != "" {
		return e.config.GetOpaUrl() + "/v1/data/" + operationConfig.opaPath
	} else {
		return e.config.GetPdpUrl() + string(operationConfig.sidecarPath)
	}
}
