package config

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

// FactsSyncTimeoutPolicy defines the possible policies for facts synchronization timeout
type FactsSyncTimeoutPolicy string

const (
	// FactsSyncTimeoutPolicyIgnore indicates to ignore timeout and continue
	FactsSyncTimeoutPolicyIgnore FactsSyncTimeoutPolicy = "ignore"
	// FactsSyncTimeoutPolicyFail indicates to fail the request when timeout is reached
	FactsSyncTimeoutPolicyFail FactsSyncTimeoutPolicy = "fail"
)

type PermitConfig struct {
	apiUrl                 string
	token                  string
	pdpUrl                 string
	opaUrl                 string
	debug                  bool
	Context                *PermitContext
	Logger                 *zap.Logger
	httpClient             *http.Client
	proxyFactsViaPDP       bool
	factsSyncTimeout       *time.Duration
	factsSyncTimeoutPolicy FactsSyncTimeoutPolicy
}

type IPermitConfig interface {
	GetApiUrl() string
	GetToken() string
	GetPdpUrl() string
	GetOpaUrl() string
	GetDebug() bool
	GetContext() *PermitContext
	GetLogger() *zap.Logger
	// Deprecated: GetProxyFactsViaPDP will be removed in future versions.
	GetProxyFactsViaPDP() bool
	GetFactsSyncTimeout() *time.Duration
	GetFactsSyncTimeoutPolicy() FactsSyncTimeoutPolicy
	GetHTTPClient() *http.Client
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

func (c *PermitConfig) GetLogger() *zap.Logger {
	return c.Logger
}

func (c *PermitConfig) GetProxyFactsViaPDP() bool {
	return c.proxyFactsViaPDP
}

func (c *PermitConfig) GetFactsSyncTimeout() *time.Duration {
	return c.factsSyncTimeout
}

func (c *PermitConfig) GetFactsSyncTimeoutPolicy() FactsSyncTimeoutPolicy {
	return c.factsSyncTimeoutPolicy
}

func (c *PermitConfig) GetHTTPClient() *http.Client {
	return c.httpClient
}
