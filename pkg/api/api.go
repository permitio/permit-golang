package api

import (
	"context"
	"github.com/permitio/permit-golang/openapi"
	permit "github.com/permitio/permit-golang/pkg/permit"
	"go.uber.org/zap"
)

type PermitBaseApi struct {
	client *openapi.APIClient
	config *permit.PermitConfig
	logger *zap.Logger
}

type IPermitBaseApi interface {
	List(ctx context.Context) []interface{}
}

type PermitApiClient struct {
	ctx                context.Context
	config             *permit.PermitConfig
	logger             *zap.Logger
	Client             *openapi.APIClient
	tenants            *Tenants
	environments       *Environments
	projects           *Projects
	resourceActions    *ResourceActions
	resourceAttributes *ResourceAttributes
	resources          *Resources
	roles              *Roles
	users              *Users
	Elements           *Elements
}

func (p *PermitApiClient) SetContext(project string, environment string) {
	permitContext, err := permit.PermitContextFactory(p.ctx, p.logger, p.Client, project, environment, true)
	if err != nil {
		p.logger.Error("", zap.Error(err))
	}
	p.config.Context = permitContext
}

func NewPermitApiClient(ctx context.Context, config *permit.PermitConfig) *PermitApiClient {
	client := openapi.NewAPIClient(openapi.NewConfiguration())
	return &PermitApiClient{
		config:             config,
		ctx:                ctx,
		logger:             config.Logger,
		tenants:            NewTenantsApi(client, config),
		environments:       NewEnvironmentsApi(client, config),
		projects:           NewProjectsApi(client, config),
		resourceActions:    NewResourceActionsApi(client, config),
		resourceAttributes: NewResourceAttributesApi(client, config),
		resources:          NewResourcesApi(client, config),
		roles:              NewRolesApi(client, config),
		users:              NewUsersApi(client, config),
		Elements:           NewElementsApi(client, config),
	}
}
