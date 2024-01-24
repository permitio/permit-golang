package config

import (
	"go.uber.org/zap"
	"net/http"
	"time"
)

type PermitBuilder struct {
	PermitConfig
}

func NewConfigBuilder(token string) *PermitBuilder {
	return &PermitBuilder{
		PermitConfig: PermitConfig{
			apiUrl:     DefaultApiUrl,
			token:      token,
			pdpUrl:     DefaultPdpUrl,
			debug:      DefaultDebugMode,
			Context:    nil,
			Logger:     nil,
			httpClient: &http.Client{Timeout: DefaultTimeout},
		},
	}
}

func (c *PermitConfig) WithHTTPClient(client *http.Client) *PermitConfig {
	c.httpClient = client
	return c
}

func (c *PermitConfig) WithApiUrl(apiUrl string) *PermitConfig {
	if apiUrl != "" {
		c.apiUrl = apiUrl
	}

	return c
}

func (c *PermitConfig) WithPdpUrl(pdpUrl string) *PermitConfig {
	if pdpUrl != "" {
		c.pdpUrl = pdpUrl
	}

	return c
}

func (c *PermitConfig) WithOpaUrl(opaUrl string) *PermitConfig {
	c.opaUrl = opaUrl
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

func (c *PermitConfig) WithTimeout(timeout time.Duration) *PermitConfig {
	c.httpClient.Timeout = timeout
	return c
}

func (c *PermitConfig) WithLogger(logger *zap.Logger) *PermitConfig {
	c.Logger = logger
	return c
}

func (c *PermitConfig) Build() PermitConfig {
	if c.Logger == nil {
		logger, err := zap.NewProduction()
		if err != nil {
			panic(err)
		}
		c.Logger = logger
	}
	return *c
}
