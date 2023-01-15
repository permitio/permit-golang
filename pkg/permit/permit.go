package permit

import (
	"context"
	"github.com/permitio/permit-golang/openapi"
	"github.com/permitio/permit-golang/pkg/api"
	"go.uber.org/zap"
)

type Permit struct {
	config   PermitConfig
	logger   *zap.Logger
	api      api.PermitBaseApi
	elements api.Elements
}

func NewPermit(apiUrl string, token string, pdpUrl string, permitContext PermitContext, debugMode bool) *Permit {
	logger := zap.New()
	ctx := context.Background()
	config := NewPermitConfig(apiUrl, token, pdpUrl, debugMode, permitContext, logger)
	apiClient := api.NewPermitApiClient()
	return &Permit{
		config:   *config,
		logger:   logger,
		ctx:      context.Background(),
		api:      apiClient,
		elements: apiClient.Elements,
	}
}

type PermitInterface interface {
	Check() bool
	SyncUser() openapi.UserRead
	SyncResources() []openapi.ResourceRead
}

def my_func(lst):
	return sum(lst) / len(lst)