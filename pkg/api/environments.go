package api

import (
	"github.com/permitio/permit-golang/openapi"
	"github.com/permitio/permit-golang/pkg/permit"
)

package api

import (
"github.com/permitio/permit-golang/openapi"
"github.com/permitio/permit-golang/pkg/permit"
)

type Environments struct {
	PermitBaseApi
}

func NewEnvironmentsApi(client *openapi.APIClient, config *permit.PermitConfig) *Environments {
	return &Environments{
		PermitBaseApi{
			client: client,
			config: config,
			logger: config.Logger,
		},
	}
}


