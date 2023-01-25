package api

import (
	"context"
	"github.com/permitio/permit-golang/models"
	"github.com/permitio/permit-golang/openapi"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"go.uber.org/zap"
)

type Elements struct {
	permitBaseApi
}

func NewElementsApi(client *openapi.APIClient, config *config.PermitConfig) *Elements {
	return &Elements{
		permitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}

// LoginAs login as a given user.
// Usage Example:
// ```
// userLogin := models.NewUserLoginRequestInput("user-id", "tenant-id")
// embeddedLoginRequestOutput, err := PermitClient.Api.Elements.LoginAs(ctx, userLogin)
// ```
func (e *Elements) LoginAs(ctx context.Context, userLogin models.UserLoginRequestInput) (*models.EmbeddedLoginRequestOutput, error) {
	err := e.lazyLoadContext(ctx)
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
