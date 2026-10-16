package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the user invites of an environment.
const (
	opListUserInvites   = "list_user_invites"
	opCreateUserInvite  = "create_user_invite"
	opGetUserInvite     = "get_user_invite"
	opUpdateUserInvite  = "update_user_invite"
	opDeleteUserInvite  = "delete_user_invite"
	opApproveUserInvite = "approve_user_invite"
)

// UserInvitesService lists, reads, creates, changes, approves and deletes the user invites of an
// environment; [Client.UserInvites] holds it. An invite names a user to come, by the key, the
// email address and the name that the invite gives the user, and a role, a tenant and a resource
// instance of the environment, each by its ID, as [models.ElementsUserInviteCreate] says.
//
// A method that acts on one invite takes it as userInviteID, such as the ID of the invite that
// Create returns, and sends it as it is given, as one segment of the path, escaped: a value that
// holds a / or a % names that invite, not another route. The API's documentation says that the
// path takes "Either the unique id of the user_invite, or the URL-friendly key of the
// user_invite"; that it takes the key of an invite is not confirmed yet.
//
// The routes of the invites start with /v2/facts/, as those of the facts API do, but the client
// sends them to the API in both modes: a client with [WithFactsViaPDP] sends them to the API too,
// with the project and the environment of its scope, and never to the PDP, and no call waits for
// sync.
//
// Its calls act on the environment of the client's scope, whose project and environment fill the
// route: those that [WithScope] gives, or else those of the client's API key, which the first call
// that needs them looks up once, as [Client.Scope] says. A scope without an environment, such as
// that of a project-level API key without WithScope, is an error that matches [ErrInvalidConfig],
// and the call sends no request for its operation. Each method is sent with an
// environment-level API key of the environment, or with a key of a wider level and WithScope; the
// API answers a key that may not act on the environment with an error status, which the call
// returns as an [Error], such as one that matches [ErrForbidden].
type UserInvitesService struct {
	client *Client
}

// UserInvitesListOptions are the page and the filters of [UserInvitesService.List] and
// [UserInvitesService.All]. Each filter that is set is sent as it is given; one that is empty is
// left out of the query. The API's documentation describes each filter, as each field says; which
// form of a role or a tenant, its key or its ID, the filter takes is not confirmed yet.
type UserInvitesListOptions struct {
	// ListOptions picks the page and the number of invites on it, sent in page and per_page.
	// With a PerPage of 0, List leaves the number to the API, which gives 30, and All asks for
	// 100, the most that the API gives.
	ListOptions

	// Role is sent in the query parameter role. The API's documentation says that it "will only
	// return invited users with this role".
	Role string

	// Tenant is sent in the query parameter tenant. The API's documentation says that it "will
	// only return invited users in that tenant".
	Tenant string

	// Search is sent in the query parameter search. The API's documentation says that it "will
	// only return invited users to that email, key or name".
	Search string
}

// userInvitesRequest returns the request of the invites that opts filters, with role, tenant and
// search in its query when opts sets them, and the page of opts. A nil opts is the zero
// UserInvitesListOptions. The request holds copies of the values of opts, so a later change to
// opts does not change it.
func userInvitesRequest(opts *UserInvitesListOptions) (request, ListOptions) {
	var o UserInvitesListOptions
	if opts != nil {
		o = *opts
	}
	query := &wire.Query{}
	for _, filter := range []struct{ name, value string }{
		{"role", o.Role}, {"tenant", o.Tenant}, {"search", o.Search},
	} {
		if filter.value != "" {
			query.AddString(filter.name, &filter.value)
		}
	}
	return request{op: opListUserInvites, target: apiTarget, query: query}, o.ListOptions
}

// call makes the call of r, a request of one invite, in the environment of the client's scope,
// and returns the invite of its reply.
func (s *UserInvitesService) call(ctx context.Context, r request,
) (*models.ElementsUserInviteRead, error) {
	r, err := s.client.inScope(ctx, r)
	if err != nil {
		return nil, err
	}
	return do[models.ElementsUserInviteRead](ctx, s.client, r)
}

// List returns one page of the invites of the environment, in the order of the API's reply, those
// that match each filter of opts, with GET /v2/facts/{proj_id}/{env_id}/user_invites. A nil opts,
// like the zero UserInvitesListOptions, asks for the first page of every invite, with the API's
// default of 30 invites on it; the API gives at most 100 on a page. The reply is the paginated
// object, and the page holds its counts: the TotalCount of the invites and the PageCount, each
// nil when the reply leaves it out. [UserInvitesService.All] reads every page.
//
// List is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [UserInvitesService] says. The call changes nothing, so it sends its request
// again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// List returns every failure as an error, and never a page with it:
//
//   - a scope without an environment: an error that matches [ErrInvalidConfig], and no request
//     for the list; without WithScope, the lookup of the scope comes first;
//   - a Page or a PerPage less than zero: an error that matches ErrInvalidConfig, and no request
//     for the list;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply;
//   - a reply in 200-299 that is not the paginated JSON object of invites, such as a JSON array:
//     an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UserInvitesService) List(ctx context.Context, opts *UserInvitesListOptions,
) (*Page[models.ElementsUserInviteRead], error) {
	r, page := userInvitesRequest(opts)
	r, err := s.client.inScope(ctx, r)
	if err != nil {
		return nil, err
	}
	return listPage[models.ElementsUserInviteRead](ctx, s.client, r, pageShape, page)
}

// All returns an iterator over the invites of the environment that match each filter of opts,
// for a range loop. It reads the pages in turn, as [UserInvitesService.List] reads one, from the
// page of opts.Page on, or the first, each of opts.PerPage invites, or else of 100, the most that
// the API gives on a page. It stops after the first page without invites, or, when the loop
// started at the first page, after the page that the total_count of its reply shows to be the
// last, and never because a page holds fewer invites than it asked for. It asks for the next page
// only when the loop wants an invite past the last of a page, so a loop that stops early sends no
// other request. It reads opts when it is called: a later change to opts does not change the
// iterator. Each loop over the iterator reads the invites again, from the first page that opts
// asks for. The pages are requests of their own, so an invite created or deleted during a loop
// can move the others from one page to the next.
//
// All is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as List is; each loop gets the scope as List does before its first page. Each
// page is a call of its own, with its own timeout and retries. When the scope or a page fails,
// with one of the errors that List returns, the iterator yields its error once, with the zero
// ElementsUserInviteRead, and stops.
func (s *UserInvitesService) All(ctx context.Context, opts *UserInvitesListOptions,
) iter.Seq2[models.ElementsUserInviteRead, error] {
	r, page := userInvitesRequest(opts)
	return allInScope[models.ElementsUserInviteRead](ctx, s.client, r, pageShape, page)
}

// Get returns the invite whose ID is userInviteID, with
// GET /v2/facts/{proj_id}/{env_id}/user_invites/{user_invite_id}.
//
// Get is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [UserInvitesService] says. The call changes nothing, so it sends its request
// again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no invite:
//
//   - for a scope without an environment, or a userInviteID that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the invite; without WithScope, the lookup of
//     the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] for a 404;
//   - for a reply in 200-299 that is not a JSON object of an invite: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UserInvitesService) Get(ctx context.Context, userInviteID string,
) (*models.ElementsUserInviteRead, error) {
	return s.call(ctx, request{
		op: opGetUserInvite, target: apiTarget, params: []string{userInviteID},
	})
}

// Create creates an invite in the environment with
// POST /v2/facts/{proj_id}/{env_id}/user_invites, and returns it as the API returns it, with its
// ID. The request holds each field of invite, also when it is empty, as
// [models.ElementsUserInviteCreate] says.
//
// Create is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [UserInvitesService] says. It is sent again only after a 429, which the API did
// not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have created
// the invite, so Create returns that error, and [UserInvitesService.List] tells whether the
// environment holds the invite.
//
// Create returns an error, and no invite:
//
//   - for a scope without an environment: an error that matches [ErrInvalidConfig], and no
//     request for the invite; without WithScope, the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrConflict] for a 409, or one whose Detail names each field that
//     the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of an invite: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UserInvitesService) Create(ctx context.Context, invite models.ElementsUserInviteCreate,
) (*models.ElementsUserInviteRead, error) {
	return s.call(ctx, request{op: opCreateUserInvite, target: apiTarget, body: invite})
}

// Update changes the invite whose ID is userInviteID with
// PATCH /v2/facts/{proj_id}/{env_id}/user_invites/{user_invite_id}, and returns it as the API
// returns it. The request holds each field of update, also when it is empty, as
// [models.ElementsUserInviteUpdate] says: the spec requires each, so an update sends the whole
// invite.
//
// Update is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [UserInvitesService] says. The request names every field, so sending it twice
// leaves the invite as sending it once does, and the call sends its request again after a 500,
// 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Update returns an error, and no invite:
//
//   - for a scope without an environment, or a userInviteID that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the invite; without WithScope, the lookup of
//     the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] for a 404, or one whose Detail names each field that
//     the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of an invite: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UserInvitesService) Update(ctx context.Context, userInviteID string,
	update models.ElementsUserInviteUpdate,
) (*models.ElementsUserInviteRead, error) {
	return s.call(ctx, request{
		op: opUpdateUserInvite, target: apiTarget, params: []string{userInviteID}, body: update,
	})
}

// Approve approves the invite whose ID is userInviteID with
// POST /v2/facts/{proj_id}/{env_id}/user_invites/{user_invite_id}/approve, and returns the user of
// the API's reply, a [models.UserRead]. The request holds the email address, the key and the
// attributes of approval, as [models.ElementsUserInviteApprove] says, and {} for its attributes
// when they are nil. What the approval does to the invite and to the users of the environment,
// such as whether the user of the reply is one that it creates, is not confirmed yet.
//
// Approve is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [UserInvitesService] says. The API's documentation says that it answers 400 to
// an invite that is approved already, so the call is sent again only after a 429, which the API
// did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have
// approved the invite, so Approve returns that error, and [UserInvitesService.Get] tells the
// status of the invite.
//
// Approve returns an error, and no user:
//
//   - for a scope without an environment, or a userInviteID that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the invite; without WithScope, the lookup of
//     the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] for a 404, or one whose Detail names each field that
//     the API rejects for a 422; the API's documentation says that it answers 400 when the invite
//     is approved already, when the email address of approval is not that of the invite, when the
//     resource instance of the invite does not exist in its environment and tenant, or when the
//     role of the invite belongs to another resource than the invite's tenant or resource
//     instance, and the Error's Detail then holds the detail of the reply;
//   - for a reply in 200-299 that is not a JSON object of a user: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UserInvitesService) Approve(ctx context.Context, userInviteID string,
	approval models.ElementsUserInviteApprove,
) (*models.UserRead, error) {
	if approval.Attributes == nil {
		approval.Attributes = models.Attributes{}
	}
	r, err := s.client.inScope(ctx, request{
		op: opApproveUserInvite, target: apiTarget, params: []string{userInviteID},
		body: approval,
	})
	if err != nil {
		return nil, err
	}
	return do[models.UserRead](ctx, s.client, r)
}

// Delete deletes the invite whose ID is userInviteID with
// DELETE /v2/facts/{proj_id}/{env_id}/user_invites/{user_invite_id}. It returns nil when the API
// answers 404, as the invite is gone already, such as when an earlier attempt of the call deleted
// it; so a mistyped ID returns nil too, and deletes nothing.
//
// Delete is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [UserInvitesService] says. Deleting an invite twice leaves it deleted, so the
// call sends its request again after a 500, 502, 503 or 504 reply or a failed connection, as
// after a 429.
//
// Delete returns an error:
//
//   - for a scope without an environment, or a userInviteID that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the invite; without WithScope, the lookup of
//     the scope comes first;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete the
//     invite;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UserInvitesService) Delete(ctx context.Context, userInviteID string) error {
	r, err := s.client.inScope(ctx, request{
		op: opDeleteUserInvite, target: apiTarget, params: []string{userInviteID},
	})
	if err != nil {
		return err
	}
	return s.client.doNoValue(ctx, r)
}
