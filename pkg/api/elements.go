package api

import (
	"context"
	"github.com/permitio/permit-golang/openapi"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/permit"
	"go.uber.org/zap"
)

type Elements struct {
	PermitBaseApi
}

func NewElementsApi(client *openapi.APIClient, config *permit.PermitConfig) *Elements {
	return &Elements{
		PermitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

func (e *Elements) LoginAs(ctx context.Context, userLogin openapi.UserLoginRequestInput) (*openapi.EmbeddedLoginRequestOutput, error) {
	err := e.LazyLoadContext(ctx)
	if err != nil {
		return nil, err
	}
	embeddedLoginRequestOutput, httpRes, err := e.client.AuthenticationApi.ElementsLoginAs(ctx).UserLoginRequestInput(userLogin).Execute()
	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		e.logger.Error("error login as: "+userLogin.GetUserId()+"in tenant: "+userLogin.GetTenantId(), zap.Error(err))
		return nil, err
	}
	return embeddedLoginRequestOutput, nil

}
