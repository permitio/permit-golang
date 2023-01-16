package permit

import "go.uber.org/zap"

type PermitConfig struct {
	apiUrl  string
	token   string
	pdpUrl  string
	debug   bool
	Context *PermitContext
	Logger  *zap.Logger
}

func NewPermitConfig(apiUrl string, token string, pdpUrl string, debug bool, context *PermitContext, logger *zap.Logger) *PermitConfig {
	return &PermitConfig{
		apiUrl:  apiUrl,
		token:   token,
		pdpUrl:  pdpUrl,
		debug:   debug,
		Context: context,
		Logger:  logger,
	}
}
