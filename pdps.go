package permit

import (
	"context"

	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the PDPs of an environment.
const (
	opRefreshEnvironmentPDPData = "refresh_environment_pdp_data"
	opRefreshPDPData            = "refresh_pdp_data"
)

// PDPsService asks the PDPs of an environment to read their data again; [Client.PDPs] holds it.
// A PDP reads the data of its environment, such as its users and their roles, from Permit in the
// background; a refresh asks it to read all of it again at once, rather than at its next poll,
// such as after a change in a source of data outside Permit. The API publishes the refresh and
// answers at once: the reply does not wait until the PDPs have read the data.
//
// Its calls act on the environment of the client's scope, whose project and environment fill the
// route: those that [WithScope] gives, or else those of the client's API key, which the first call
// that needs them looks up once, as [Client.Scope] says. A scope without an environment, such as
// that of a project-level API key without WithScope, is an error that matches [ErrInvalidConfig],
// and the call sends no request for its operation. Each method is sent with an
// environment-level API key of the environment, or with a key of a wider level and WithScope; the
// API answers a key that may not change the configurations of the PDPs with an error status,
// which the call returns as an [Error], such as one that matches [ErrForbidden].
type PDPsService struct {
	client *Client
}

// refresh makes the call of r, a refresh of the data of PDPs, in the environment of the client's
// scope, and returns the reply.
func (s *PDPsService) refresh(ctx context.Context, r request,
) (*models.PDPDataRefreshResponse, error) {
	r, err := s.client.inScope(ctx, r)
	if err != nil {
		return nil, err
	}
	return do[models.PDPDataRefreshResponse](ctx, s.client, r)
}

// Refresh asks every PDP of the environment to read its data again, with
// POST /v2/pdps/{proj_id}/{env_id}/configs/refresh, and returns the ID of the update and the IDs
// of the PDPs that it asks. The request's body holds the fields of refresh that are set, as
// [models.PDPDataRefreshRequest] says: the zero PDPDataRefreshRequest sends {}. The API's
// documentation says that this refresh takes no ShardID, a shard of one PDP: the call sends
// what refresh holds, and the API rejects a ShardID with an error status; [PDPsService.RefreshPDP]
// takes one.
//
// Refresh is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [PDPsService] says. A second refresh asks the PDPs to read the same data again,
// so the call sends its request again after a 500, 502, 503 or 504 reply or a failed connection,
// as after a 429.
//
// Refresh returns an error, and no reply:
//
//   - for a scope without an environment: an error that matches [ErrInvalidConfig], and no
//     request for the refresh; without WithScope, the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the environment has no PDP, or one whose Detail
//     says why the API rejects the body for a 422;
//   - for a reply in 200-299 that is not a JSON object of a refresh: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *PDPsService) Refresh(ctx context.Context, refresh models.PDPDataRefreshRequest,
) (*models.PDPDataRefreshResponse, error) {
	return s.refresh(ctx, request{
		op: opRefreshEnvironmentPDPData, target: apiTarget, body: refresh,
	})
}

// RefreshPDP asks the PDP whose configuration's ID is pdpID, a UUID, to read its data again, with
// POST /v2/pdps/{proj_id}/{env_id}/configs/{pdp_id}/refresh, and returns the ID of the update and
// the ID of the PDP. It sends pdpID as one segment of the path, escaped. The request's body holds
// the fields of refresh that are set, as [models.PDPDataRefreshRequest] says, its ShardID too,
// which a pointer to 0 sends as 0: the zero PDPDataRefreshRequest sends {}.
//
// RefreshPDP is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [PDPsService] says. A second refresh asks the PDP to read the same data again,
// so the call sends its request again after a 500, 502, 503 or 504 reply or a failed connection,
// as after a 429.
//
// RefreshPDP returns an error, and no reply:
//
//   - for a scope without an environment, or a pdpID that is empty, . or ..: an error that matches
//     [ErrInvalidConfig], and no request for the refresh; without WithScope, the lookup of the
//     scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the environment has no such PDP, or one whose
//     Detail names each field that the API rejects for a 422, such as a pdpID that is not a UUID;
//   - for a reply in 200-299 that is not a JSON object of a refresh: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *PDPsService) RefreshPDP(ctx context.Context, pdpID string,
	refresh models.PDPDataRefreshRequest,
) (*models.PDPDataRefreshResponse, error) {
	return s.refresh(ctx, request{
		op: opRefreshPDPData, target: apiTarget, params: []string{pdpID}, body: refresh,
	})
}
