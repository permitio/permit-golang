package permit_test

import (
	"context"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the PDPs: an environment without a
// PDP, and the ID of a PDP that is not a UUID.
const (
	pdpNoConfigBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"No PDP configurations found in this environment","id":"req-40"}`
	pdpInvalidIDBody = `{"detail":[{"loc":["path","pdp_id"],` +
		`"msg":"value is not a valid uuid","type":"type_error.uuid"}]}`
)

// The IDs of the PDPs of the reply fixtures of the refreshes.
const (
	pdpProductionID = "00000000-0000-4000-8000-000000000601"
	pdpStagingID    = "00000000-0000-4000-8000-000000000602"
)

// pdpCases returns the wire cases of the refreshes of the data of the PDPs of an environment.
// Each sends one request to the API, along a route of /v2/pdps/ that the runner's scope fills, p
// and e, as WithScope gives it. Refresh sends a reason that holds characters that an encoder may
// escape, <, > and &, and RefreshPDP the shard 0, which a pointer to 0 sends, to a PDP whose ID
// holds a slash, which the path escapes.
func pdpCases() []wireCase {
	const (
		refreshRoute    = "/v2/pdps/{proj_id}/{env_id}/configs/refresh"
		refreshPDPRoute = "/v2/pdps/{proj_id}/{env_id}/configs/{pdp_id}/refresh"
	)
	refreshErr := projectError("refresh_environment_pdp_data", http.MethodPost, refreshRoute,
		http.StatusNotFound, pdpNoConfigBody)
	refreshErr.Code, refreshErr.Title, refreshErr.RequestID = "NOT_FOUND", "Not Found", "req-40"
	refreshErr.Detail = "No PDP configurations found in this environment"
	refreshPDPErr := projectError("refresh_pdp_data", http.MethodPost, refreshPDPRoute,
		http.StatusUnprocessableEntity, pdpInvalidIDBody)
	refreshPDPErr.Detail = "path.pdp_id: value is not a valid uuid"
	return []wireCase{
		{
			Method: "PDPsService.Refresh",
			Op:     "refresh_environment_pdp_data",
			Route:  "POST " + refreshRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.PDPs.Refresh(ctx, models.PDPDataRefreshRequest{
					Reason: "<sync> & reload",
				})
			},
			Request: wireRequest{
				RawPath: "/v2/pdps/p/e/configs/refresh", Body: `{"reason":"<sync> & reload"}`,
			},
			Reply: mockpermit.Reply{Body: replyFixture("refresh_environment_pdp_data.200.json")},
			WantResult: &models.PDPDataRefreshResponse{
				UpdateID: "00000000-0000-4000-8000-000000000701",
				PDPIDs:   []string{pdpProductionID, pdpStagingID},
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: pdpNoConfigBody},
			WantErr:    refreshErr,
		},
		{
			Method: "PDPsService.RefreshPDP",
			Op:     "refresh_pdp_data",
			Route:  "POST " + refreshPDPRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.PDPs.RefreshPDP(ctx, "a/b", models.PDPDataRefreshRequest{ShardID: new(0)})
			},
			Request: wireRequest{
				RawPath: "/v2/pdps/p/e/configs/a%2Fb/refresh", Body: `{"shard_id":0}`,
			},
			Reply: mockpermit.Reply{Body: replyFixture("refresh_pdp_data.200.json")},
			WantResult: &models.PDPDataRefreshResponse{
				UpdateID: "00000000-0000-4000-8000-000000000702", PDPIDs: []string{pdpProductionID},
			},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: pdpInvalidIDBody,
			},
			WantErr: refreshPDPErr,
		},
	}
}
