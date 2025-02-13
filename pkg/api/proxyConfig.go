package api

import (
	"context"
	"github.com/google/uuid"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/openapi"
)

type ProxyConfigs struct {
	permitBaseApi
}

func NewProxyConfigsApi(client *openapi.APIClient, config *config.PermitConfig) *ProxyConfigs {
	return &ProxyConfigs{
		permitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

// List all ProxyConfigs in the organization, requires Environment level API key, or higher.
// Usage Example:
// `ProxyConfigs, err := PermitClient.Api.ProxyConfigs.List(ctx, 1, 10)`
func (p *ProxyConfigs) List(ctx context.Context, page int, perPage int) ([]models.ProxyConfigRead, error) {
	perPageLimit := int32(DefaultPerPageLimit)
	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		p.logger.Error("error listing ProxyConfigs - max per page: "+string(perPageLimit), err)
		return nil, err
	}
	err := p.lazyLoadPermitContext(ctx, config.EnvironmentAPIKeyLevel)
	if err != nil {
		p.logger.Error("", err)
		return nil, err
	}
	ProxyConfigs, httpRes, err := p.client.ProxyConfigAPI.ListProxyConfigs(ctx, p.config.Context.GetProject(), p.config.Context.GetEnvironment()).Page(int32(page)).PerPage(int32(perPage)).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		p.logger.Error("error listing ProxyConfigs", err)
		return nil, err
	}

	return ProxyConfigs, nil
}

// Get a Proxy Config by key, requires Environment level API key, or higher.
// Usage Example:
// `proxyConfig, err := PermitClient.Api.ProxyConfigs.Get(ctx, "proxyconfig-key")`
func (p *ProxyConfigs) Get(ctx context.Context, proxyConfigKey string) (*models.ProxyConfigRead, error) {
	err := p.lazyLoadPermitContext(ctx, config.EnvironmentAPIKeyLevel)
	if err != nil {
		p.logger.Error("", err)
		return nil, err
	}
	proxyConfig, httpRes, err := p.client.ProxyConfigAPI.GetProxyConfig(ctx, p.config.Context.GetProject(), p.config.Context.GetEnvironment(), proxyConfigKey).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		p.logger.Error("error getting Proxy Config: "+proxyConfigKey, err)
		return nil, err
	}
	return proxyConfig, nil
}

// GetByKey get a project by key, requires Environment level API key, or higher.
// Usage Example:
// `proxyConfig, err := PermitClient.Api.ProxyConfigs.GetByKey(ctx, "proxyconfig-key")`
func (p *ProxyConfigs) GetByKey(ctx context.Context, ProxyConfigKey string) (*models.ProxyConfigRead, error) {
	return p.Get(ctx, ProxyConfigKey)
}

// GetById get a proxy config by id, requires Environment level API key, or higher.
// Usage Example:
// `proxyConfig, err := PermitClient.Api.ProxyConfigs.GetById(ctx, uuid.New())`
func (p *ProxyConfigs) GetById(ctx context.Context, ProxyConfigId uuid.UUID) (*models.ProxyConfigRead, error) {
	return p.Get(ctx, ProxyConfigId.String())
}

// Create a new proxy config, requires Environment level API key, or higher.
// Usage Example:
// ```
// proxyConfigCreate := models.NewProxyConfigCreate("user:pass", "proxyConfigKey", "proxyConfigName")
// proxyConfig, err := PermitClient.Api.ProxyConfigs.Create(ctx, proxyConfigCreate)
// ```
func (p *ProxyConfigs) Create(ctx context.Context, proxyConfigCreate models.ProxyConfigCreate) (*models.ProxyConfigRead, error) {
	err := p.lazyLoadPermitContext(ctx, config.EnvironmentAPIKeyLevel)
	if err != nil {
		p.logger.Error("", err)
		return nil, err
	}
	proxyConfig, httpRes, err := p.client.ProxyConfigAPI.CreateProxyConfig(ctx, p.config.Context.GetProject(), p.config.Context.GetEnvironment()).ProxyConfigCreate(proxyConfigCreate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		p.logger.Error("error creating proxy config: "+proxyConfigCreate.GetKey(), err)
		return nil, err
	}
	return proxyConfig, nil
}

// Update a proxy config, requires Environment level API key, or higher.
// Usage Example:
// ```
// proxyConfigUpdate := models.NewProxyConfigUpdate()
// proxyConfigUpdate.SetName("new-proxy-config-name")
// proxyConfig, err := PermitClient.Api.ProxyConfigs.Update(ctx, "proxy-config-key", proxyConfigUpdate)
// ```
func (p *ProxyConfigs) Update(ctx context.Context, proxyConfigKey string, proxyConfigUpdate models.ProxyConfigUpdate) (*models.ProxyConfigRead, error) {
	err := p.lazyLoadPermitContext(ctx, config.EnvironmentAPIKeyLevel)
	if err != nil {
		p.logger.Error("", err)
		return nil, err
	}
	proxyConfig, httpRes, err := p.client.ProxyConfigAPI.UpdateProxyConfig(ctx, p.config.Context.GetProject(), p.config.Context.GetEnvironment(), proxyConfigKey).ProxyConfigUpdate(proxyConfigUpdate).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		p.logger.Error("error updating proxy config: "+proxyConfigKey, err)
		return nil, err
	}
	return proxyConfig, nil
}

// Delete a proxy config, requires Environment level API key, or higher.
// Usage Example:
// `err := PermitClient.Api.ProxyConfigs.Delete(ctx, "proxy-config-key")`
func (p *ProxyConfigs) Delete(ctx context.Context, proxyConfigKey string) error {
	err := p.lazyLoadPermitContext(ctx, config.EnvironmentAPIKeyLevel)
	if err != nil {
		p.logger.Error("", err)
		return err
	}
	httpRes, err := p.client.ProxyConfigAPI.DeleteProxyConfig(ctx, p.config.Context.GetProject(), p.config.Context.GetEnvironment(), proxyConfigKey).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		p.logger.Error("error deleting proxy config: "+proxyConfigKey, err)
		return err
	}
	return nil
}
