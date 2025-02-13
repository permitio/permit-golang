package config

import (
	"github.com/permitio/permit-golang/pkg/log"
	"go.uber.org/zap"
	"net/http"
	"time"
)

type PermitConfig struct {
	apiUrl           string
	token            string
	pdpUrl           string
	opaUrl           string
	debug            bool
	Context          *PermitContext
	Logger           log.Logger
	httpClient       *http.Client
	proxyFactsViaPDP bool
	factsSyncTimeout *time.Duration
}

type IPermitConfig interface {
	GetApiUrl() string
	GetToken() string
	GetPdpUrl() string
	GetOpaUrl() string
	GetDebug() bool
	GetContext() *PermitContext
	GetLogger() log.Logger
	GetProxyFactsViaPDP() bool
	GetFactsSyncTimeout() *time.Duration
	GetHTTPClient() *http.Client
}

func NewPermitConfig(apiUrl string, token string, pdpUrl string, debug bool, context *PermitContext, zapLogger *zap.Logger) *PermitConfig {
	return &PermitConfig{
		apiUrl:  apiUrl,
		token:   token,
		pdpUrl:  pdpUrl,
		debug:   debug,
		Context: context,
		Logger:  newLoggerFromZap(zapLogger),
	}
}

func NewPermitConfigWithLogger(apiUrl string, token string, pdpUrl string, debug bool, context *PermitContext, logger log.Logger) *PermitConfig {
	return &PermitConfig{
		apiUrl:  apiUrl,
		token:   token,
		pdpUrl:  pdpUrl,
		debug:   debug,
		Context: context,
		Logger:  logger,
	}
}

func (c *PermitConfig) GetApiUrl() string {
	return c.apiUrl
}

func (c *PermitConfig) GetToken() string {
	return c.token
}

func (c *PermitConfig) GetPdpUrl() string {
	return c.pdpUrl
}

func (c *PermitConfig) GetOpaUrl() string {
	return c.opaUrl
}

func (c *PermitConfig) GetDebug() bool {
	return c.debug
}

func (c *PermitConfig) GetContext() *PermitContext {
	return c.Context
}

func (c *PermitConfig) GetLogger() log.Logger {
	return c.Logger
}

func (c *PermitConfig) GetProxyFactsViaPDP() bool {
	return c.proxyFactsViaPDP
}

func (c *PermitConfig) GetFactsSyncTimeout() *time.Duration {
	return c.factsSyncTimeout
}

func (c *PermitConfig) GetHTTPClient() *http.Client {
	return c.httpClient
}
