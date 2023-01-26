package permit

import (
	"context"
	"github.com/permitio/permit-golang/pkg/api"
	config "github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/enforcement"
	"github.com/permitio/permit-golang/pkg/models"
	"go.uber.org/zap"
)

type Client struct {
	ctx         context.Context
	config      config.PermitConfig
	logger      *zap.Logger
	Api         *api.PermitApiClient
	Elements    *api.Elements
	enforcement *enforcement.PermitEnforcer
}

var New = NewPermit

func NewPermit(config config.PermitConfig) *Client {
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	apiClient := api.NewPermitApiClient(ctx, &config)
	enforcerClient := enforcement.NewPermitEnforcerClient(&config)
	return &Client{
		config:      config,
		logger:      logger,
		ctx:         context.Background(),
		Api:         apiClient,
		Elements:    apiClient.Elements,
		enforcement: enforcerClient,
	}
}
func (c *Client) SyncUser(ctx context.Context, user models.UserCreate) (*models.UserRead, error) {
	return c.Api.Users.SyncUser(ctx, user)
}
func (c *Client) Check(user enforcement.User, action enforcement.Action, resource enforcement.Resource) (bool, error) {
	return c.enforcement.Check(user, action, resource)
}

type PermitInterface interface {
	Check() bool
	SyncUser() models.UserRead
	SyncResources() []models.ResourceRead
}
