package api

import (
	"context"
	"time"

	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/openapi"
	"go.uber.org/zap"
)

type RoleAssignments struct {
	PermitBaseFactsApi
}

func NewRoleAssignmentsApi(client *openapi.APIClient, config *config.PermitConfig) *RoleAssignments {
	return &RoleAssignments{
		PermitBaseFactsApi{
			permitBaseApi{
				client: client,
				config: config,
				logger: config.Logger,
			},
		},
	}
}

// WaitForSync configures the client to wait for facts synchronization.
//
// Parameters:
//   - timeout: Optional duration to wait for synchronization.
//   - options: Additional configuration options for facts synchronization
func (r *RoleAssignments) WaitForSync(timeout *time.Duration, options WaitForSyncOptions) *RoleAssignments {
	return NewRoleAssignmentsApi(r.PermitBaseFactsApi.WaitForSync(timeout, options).client, r.config)
}

func (r *RoleAssignments) List(ctx context.Context, page int, perPage int, userFilter, roleFilter, tenantFilter string) (*[]models.RoleAssignmentRead, error) {
	response, err := r.list(ctx, page, perPage, userFilter, roleFilter, tenantFilter, false)
	if err != nil {
		return nil, err
	}
	return response.RoleAssignmentRead, nil
}

func (r *RoleAssignments) ListDetailed(ctx context.Context, page int, perPage int, userFilter, roleFilter, tenantFilter string) (*[]models.RoleAssignmentDetailedRead, error) {
	response, err := r.list(ctx, page, perPage, userFilter, roleFilter, tenantFilter, true)
	if err != nil {
		return nil, err
	}
	return response.RoleAssignmentDetailedRead, nil
}

func (r *RoleAssignments) list(ctx context.Context, page int, perPage int, userFilter, roleFilter, tenantFilter string, detailed bool) (*models.ResponseListRoleAssignmentsV2FactsProjIdEnvIdRoleAssignmentsGet, error) {
	perPageLimit := int32(DefaultPerPageLimit)

	if !isPaginationInLimit(int32(page), int32(perPage), perPageLimit) {
		err := errors.NewPermitPaginationError()
		r.logger.Error("error listing users - max per page: "+string(perPageLimit), zap.Error(err))
		return nil, err
	}

	err := r.lazyLoadPermitContext(ctx)

	if err != nil {
		r.logger.Error("", zap.Error(err))
		return nil, err
	}

	request := r.client.RoleAssignmentsApi.ListRoleAssignments(ctx, r.config.Context.GetProject(), r.config.Context.GetEnvironment()).
		Page(int32(page)).PerPage(int32(perPage))

	if userFilter != "" {
		request = request.User(userFilter)
	}

	if roleFilter != "" {
		request = request.Role(roleFilter)
	}

	if tenantFilter != "" {
		request = request.Tenant(tenantFilter)
	}

	if detailed {
		request = request.Detailed(detailed)
	}

	roleAssignments, httpRes, err := request.Execute()

	err = errors.HttpErrorHandle(err, httpRes)
	if err != nil {
		r.logger.Error("error listing roles assignments", zap.Error(err))
		return nil, err
	}

	return roleAssignments, nil
}
