package config

import (
	"go.uber.org/zap"
)

type PermitBuilder struct {
	PermitConfig
}

func NewConfigBuilder(token string) *PermitBuilder {
	return &PermitBuilder{
		PermitConfig: PermitConfig{
			apiUrl:  DefaultApiUrl,
			token:   token,
			pdpUrl:  DefaultPdpUrl,
			debug:   DefaultDebugMode,
			Context: nil,
			Logger:  nil,
		},
	}
}

func (c *PermitConfig) WithApiUrl(apiUrl string) *PermitConfig {
	c.apiUrl = apiUrl
	return c
}

func (c *PermitConfig) WithDebug(debug bool) *PermitConfig {
	c.debug = debug
	return c
}

func (c *PermitConfig) WithContext(context *PermitContext) *PermitConfig {
	c.Context = context
	return c
}

func (c *PermitConfig) WithLogger(logger *zap.Logger) *PermitConfig {
	c.Logger = logger
	return c
}

func (c *PermitConfig) Build() PermitConfig {
	return *c
}
