package permit

import (
	"context"
	"iter"
	"net/http"
	"time"

	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the users.
const (
	opListUsers   = "list_users"
	opCreateUser  = "create_user"
	opGetUser     = "get_user"
	opReplaceUser = "replace_user"
	opUpdateUser  = "update_user"
	opDeleteUser  = "delete_user"

	opAssignUserRole   = "assign_role_to_user" //nolint:gosec // G101: an operation ID
	opUnassignUserRole = "unassign_role_from_user"
)

// UsersService lists, reads, creates, syncs, changes and deletes the users of an environment,
// and assigns and unassigns their roles; [Client.Users] holds it. A method that acts on one user
// takes its key or its ID, such as alice, and sends it as one segment of the path, escaped: a
// key that holds a / or a % names that user, not another route.
//
// Its calls are calls of the facts API. By default they go to the Permit API, under
// /v2/facts/{proj_id}/{env_id}/users, and act on the environment of the client's scope: the
// project and the environment of [WithScope], or else those of the client's API key, which the
// first call that needs them looks up, as [Client.Scope] says, also when the call then fails
// before its own request. A scope without an environment, as that of a project-level API key
// without WithScope, is an error that matches [ErrInvalidConfig], and the call sends no request
// for the users. With [WithFactsViaPDP], they go to the PDP container under /facts/users
// instead, need no scope, and act on the environment of the container's own API key.
//
// [UsersService.WaitForSync] returns a copy whose creates, syncs, updates and assignments and
// unassignments of roles wait until the change reaches the policy of the PDP container.
type UsersService struct {
	facts facts
}

// UsersListOptions are the page and the filters of [UsersService.List] and [UsersService.All].
type UsersListOptions struct {
	// ListOptions picks the page and the number of users on it, sent in page and per_page.
	// With a PerPage of 0, List leaves the number to the API, which gives 30, and All asks for
	// 100, the most that the API gives.
	ListOptions

	// Search keeps the users that the API's text search for it matches, sent in search, with
	// the API's default search operator, contains; v1's documentation of the parameter names
	// the email of a user. Empty keeps every user.
	Search string

	// Role keeps the users that have the role of this key, sent in role; empty keeps every
	// user.
	Role string
}

// usersRequest returns the request of the users that opts filters, with search and role in its
// query when opts sets them, and the page of opts. A nil opts is the zero UsersListOptions. The
// request holds copies of the values of opts, so a later change to opts does not change it.
func usersRequest(opts *UsersListOptions) (request, ListOptions) {
	var o UsersListOptions
	if opts != nil {
		o = *opts
	}
	query := &wire.Query{}
	if o.Search != "" {
		query.AddString("search", &o.Search)
	}
	if o.Role != "" {
		query.AddString("role", &o.Role)
	}
	return request{op: opListUsers, query: query}, o.ListOptions
}

// UserSyncResult is the result of [UsersService.Sync]: the user as the API stores it, and
// whether the sync created it.
type UserSyncResult struct {
	// User is the user as the API stores it after the sync.
	User models.UserRead

	// Created is true when the reply's status is 201, as the API creates a user that the
	// environment did not have, and false for a 200, as the API replaces a user that it has.
	// With [WithFactsViaPDP], it is the status of the reply of the PDP container, whose spec
	// lists 200 alone.
	Created bool
}

// WaitForSync returns a copy of the service whose creates, syncs, updates and assignments and
// unassignments of roles ask the PDP container to wait for sync: to hold its answer until the
// change reaches its policy, for at most timeout, so that a check made after the call returns
// sees it. A call through the copy sends the headers X-Wait-Timeout, the timeout in decimal
// seconds, such as 0 for a timeout of 0 or 0.5 for 500 milliseconds, and X-Timeout-Policy when
// opts give a policy with [WithSyncPolicy]. The service that WaitForSync is called on, and its
// other copies, keep their own waits: the copy shares no state with them.
//
// Only [UsersService.Create], [UsersService.Sync], [UsersService.Update],
// [UsersService.AssignRole] and [UsersService.UnassignRole] wait for sync: of the routes that
// UsersService calls, the PDP container waits on POST /facts/users, on PUT and PATCH
// /facts/users/{user_id} and on POST and DELETE /facts/users/{user_id}/roles, and answers the
// reads and the delete of a user without waiting. A call through the copy returns an error that
// matches [ErrInvalidConfig], and sends no request: from a client without [WithFactsViaPDP], as
// only the PDP container waits; of any other method, such as Get or Delete, which the container
// answers without waiting; for a timeout less than zero; and for a [SyncOption] that is not
// valid, in any place among opts.
func (s *UsersService) WaitForSync(timeout time.Duration, opts ...SyncOption) *UsersService {
	return &UsersService{facts: s.facts.withWait(newSyncWait(timeout, opts))}
}

// List returns one page of the users of the environment, in the order of the API's reply,
// those that match each filter of opts, with GET /v2/facts/{proj_id}/{env_id}/users. A nil
// opts, like the zero UsersListOptions, asks for the first page of every user, with the API's
// default of 30 users on it; the API gives at most 100 on a page. The page holds the counts that
// the reply reports. [UsersService.All] reads every page. A list does not ask for the users'
// roles on resource instances, so the ResourceInstanceRoles of each of their AssociatedTenants
// may be empty; [UsersService.Get] returns them.
//
// List needs an environment-level API key, or higher, and the scope of an environment, as
// [UsersService] says. The call changes nothing, so it sends its request again after a 500,
// 502, 503 or 504 reply or a failed connection, as after a 429.
//
// List returns every failure as an error, and never a page with it:
//
//   - a Page or a PerPage less than zero: an error that matches [ErrInvalidConfig], and no
//     request for the users;
//   - through a copy of [UsersService.WaitForSync]: an error that matches ErrInvalidConfig,
//     and no request, as the PDP container answers a list without waiting;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply;
//   - a reply in 200-299 that is not the paginated JSON object of a list of users: an Error
//     that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UsersService) List(ctx context.Context, opts *UsersListOptions,
) (*Page[models.UserRead], error) {
	r, page := usersRequest(opts)
	r, err := s.facts.prepare(ctx, r)
	if err != nil {
		return nil, err
	}
	return listPage[models.UserRead](ctx, s.facts.client, r, pageShape, page)
}

// All returns an iterator over the users of the environment that match each filter of opts,
// for a range loop. It reads the pages in turn, as [UsersService.List] reads one, from the page
// of opts.Page on, or the first, each of opts.PerPage users, or else of 100, the most that the
// API gives on a page. It stops after the last page that the replies count, or after the first
// page without users, and never after a page with fewer users than it asked for. It asks for
// the next page only when the loop wants a user past the last of a page, so a loop that stops
// early sends no other request. It reads opts when it is called: a later change to opts does
// not change the iterator. Each loop over the iterator reads the users again, from the first
// page that opts asks for. The pages are requests of their own, so a user created or deleted
// during a loop can move the others from one page to the next. Like List, All does not ask for
// the users' roles on resource instances, so they may be empty; [UsersService.Get] returns them.
//
// All needs an environment-level API key, or higher, and the scope of an environment, as List
// does; each loop gets the scope before its first page. Each page is a call of its own, with its
// own timeout and retries. When a loop fails with one of the errors that List returns, such as
// for a page less than zero, through a copy of [UsersService.WaitForSync], or for a page whose
// call fails, the iterator yields its error once, with the zero UserRead, and stops. After
// [Client.Close], it yields an error that matches [ErrClosed] and sends no request.
func (s *UsersService) All(ctx context.Context, opts *UsersListOptions,
) iter.Seq2[models.UserRead, error] {
	r, page := usersRequest(opts)
	return factsAll[models.UserRead](ctx, s.facts, r, pageShape, page)
}

// Get returns the user whose key or ID is userKey, with
// GET /v2/facts/{proj_id}/{env_id}/users/{user_id}.
//
// Get needs an environment-level API key, or higher, and the scope of an environment, as
// [UsersService] says. The call changes nothing, so it sends its request again after a 500,
// 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no user:
//
//   - for a userKey that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request for the user;
//   - through a copy of [UsersService.WaitForSync]: an error that matches ErrInvalidConfig,
//     and no request, as the PDP container answers a read without waiting;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the environment has no such user;
//   - for a reply in 200-299 that is not a JSON object of a user, such as an empty body: an
//     Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UsersService) Get(ctx context.Context, userKey string) (*models.UserRead, error) {
	r, err := s.facts.prepare(ctx, request{op: opGetUser, params: []string{userKey}})
	if err != nil {
		return nil, err
	}
	return do[models.UserRead](ctx, s.facts.client, r)
}

// Create creates a user in the environment with POST /v2/facts/{proj_id}/{env_id}/users, with
// the roles of its RoleAssignments, and returns it as the API stores it, with its ID.
//
// Create needs an environment-level API key, or higher, and the scope of an environment, as
// [UsersService] says. Through a copy of [UsersService.WaitForSync], with [WithFactsViaPDP], it
// waits for sync: the PDP container holds its answer until the new user reaches its policy, or
// the timeout of the wait passes. It is sent again only after a 429, which the API did not act
// on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have created the
// user, so Create returns that error, and [UsersService.Get] of the key tells whether the user
// exists. [UsersService.Sync] creates a user that the environment does not have, and replaces
// one that it has.
//
// Create returns an error, and no user:
//
//   - for a wait that the call cannot make, as [UsersService.WaitForSync] says: an error that
//     matches [ErrInvalidConfig], and no request;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrConflict] for a 409, such as when the environment has a user with the
//     key already, and [ErrNotFound] for a 404, and whose Detail names each field that the API
//     rejects for a 422, such as an email address that is not valid;
//   - for a reply in 200-299 that is not a JSON object of a user: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UsersService) Create(ctx context.Context, user models.UserCreate,
) (*models.UserRead, error) {
	r, err := s.facts.prepare(ctx, request{op: opCreateUser, body: user})
	if err != nil {
		return nil, err
	}
	return do[models.UserRead](ctx, s.facts.client, r)
}

// Sync replaces the user whose key is the Key of user with user, or creates it when the
// environment has no such user, with one request,
// PUT /v2/facts/{proj_id}/{env_id}/users/{user_id}. It sends the whole user, and the API stores
// the user as the request gives it: each of Email, FirstName, LastName and Attributes that user
// leaves out, such as an empty FirstName or nil Attributes, is cleared, not kept. To change some
// fields of a user and keep the others, use [UsersService.Update]. The request also sends the
// RoleAssignments of user, and none when it has none; what a sync with or without them does to
// the roles that the user has already is not confirmed yet. The result holds the user as the API
// stores it, and Created, true when the reply's status is 201 and false for a 200.
//
// Sync needs an environment-level API key, or higher, and the scope of an environment, as
// [UsersService] says. Through a copy of [UsersService.WaitForSync], with [WithFactsViaPDP], it
// waits for sync: the PDP container holds its answer until the user reaches its policy, or the
// timeout of the wait passes. Sending the same user twice stores the same user, so the call
// sends its request again after a 500, 502, 503 or 504 reply or a failed connection, as after a
// 429; when an earlier attempt created the user, the reply to the later one has the status 200,
// and Created is false.
//
// Sync returns an error, and no result:
//
//   - for a Key that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request for the user;
//   - for a wait that the call cannot make, as [UsersService.WaitForSync] says: an error that
//     matches ErrInvalidConfig, and no request;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] for a 404 and [ErrConflict] for a 409, and whose Detail names
//     each field that the API rejects for a 422, such as a key that holds a character that a key
//     may not hold;
//   - for a reply in 200-299 that is not a JSON object of a user: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UsersService) Sync(ctx context.Context, user models.UserCreate,
) (*UserSyncResult, error) {
	r, err := s.facts.prepare(ctx, request{
		op: opReplaceUser, params: []string{user.Key}, body: user,
	})
	if err != nil {
		return nil, err
	}
	op, reply, err := s.facts.client.exchange(ctx, r, true)
	if err != nil {
		return nil, err
	}
	stored, err := decodeObject[models.UserRead](op, reply)
	if err != nil {
		return nil, err
	}
	return &UserSyncResult{User: *stored, Created: reply.Status == http.StatusCreated}, nil
}

// Update changes the user whose key or ID is userKey with
// PATCH /v2/facts/{proj_id}/{env_id}/users/{user_id}, and returns it as the API stores it. The
// request holds the fields of update that are set, as [models.UserUpdate] says, and the user
// keeps the others: the zero UserUpdate changes nothing. Attributes that are set replace the
// user's attributes as a whole.
//
// Update needs an environment-level API key, or higher, and the scope of an environment, as
// [UsersService] says. Through a copy of [UsersService.WaitForSync], with [WithFactsViaPDP], it
// waits for sync: the PDP container holds its answer until the change reaches its policy, or the
// timeout of the wait passes. Sending the same update twice sets the same fields, so the call
// sends its request again after a 500, 502, 503 or 504 reply or a failed connection, as after a
// 429.
//
// Update returns an error, and no user:
//
//   - for a userKey that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request for the user;
//   - for a wait that the call cannot make, as [UsersService.WaitForSync] says: an error that
//     matches ErrInvalidConfig, and no request;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the environment has no such user, and whose Detail
//     names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a user: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UsersService) Update(ctx context.Context, userKey string, update models.UserUpdate,
) (*models.UserRead, error) {
	r, err := s.facts.prepare(ctx, request{
		op: opUpdateUser, params: []string{userKey}, body: update,
	})
	if err != nil {
		return nil, err
	}
	return do[models.UserRead](ctx, s.facts.client, r)
}

// Delete deletes the user whose key or ID is userKey with
// DELETE /v2/facts/{proj_id}/{env_id}/users/{user_id}. It returns nil when the API answers
// 404, as the user is gone already, such as when an earlier attempt of the call deleted it; so a
// mistyped key returns nil too, and deletes nothing. With [WithFactsViaPDP], a 404 with an empty
// body, which is how the cloud PDP answers every route under /facts, is an error that matches
// [ErrContainerPDPOnly] instead.
//
// Delete needs an environment-level API key, or higher, and the scope of an environment, as
// [UsersService] says. Deleting a user twice leaves it deleted, so the call sends its request
// again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Delete returns an error:
//
//   - for a userKey that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request for the user;
//   - through a copy of [UsersService.WaitForSync]: an error that matches ErrInvalidConfig,
//     and no request, as the PDP container answers a delete without waiting;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete the
//     user;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UsersService) Delete(ctx context.Context, userKey string) error {
	r, err := s.facts.prepare(ctx, request{op: opDeleteUser, params: []string{userKey}})
	if err != nil {
		return err
	}
	return s.facts.client.doNoValue(ctx, r)
}

// AssignRole assigns role to the user whose key or ID is userKey with
// POST /v2/facts/{proj_id}/{env_id}/users/{user_id}/roles, and returns the role assignment as
// the API stores it. The request holds the Role of role always, and its Tenant and its
// ResourceInstance when each is set, as [models.UserRoleCreate] says: a role in a tenant names
// the tenant, and a role of a resource on one of its instances names the instance too, such as
// document:doc-1. An empty Tenant is left out of the request, not sent as an empty string. The
// API's documentation says that it creates a resource instance that does not exist when the
// request names a tenant.
//
// AssignRole needs an environment-level API key, or higher, and the scope of an environment, as
// [UsersService] says. Through a copy of [UsersService.WaitForSync], with [WithFactsViaPDP], it
// waits for sync: the PDP container holds its answer until the new role reaches its policy, or
// the timeout of the wait passes. It is sent again only after a 429, which the API did not act
// on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have assigned the
// role, so AssignRole returns that error, and the AssociatedTenants of [UsersService.Get] of the
// user tell whether the user has the role.
//
// AssignRole returns an error, and no role assignment:
//
//   - for a userKey that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request for the user;
//   - for a wait that the call cannot make, as [UsersService.WaitForSync] says: an error that
//     matches ErrInvalidConfig, and no request;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] for a 404, and whose Detail names each field that the API
//     rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a role assignment, such as an empty
//     body: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UsersService) AssignRole(ctx context.Context, userKey string, role models.UserRoleCreate,
) (*models.RoleAssignmentRead, error) {
	r, err := s.facts.prepare(ctx, request{
		op: opAssignUserRole, params: []string{userKey}, body: role,
	})
	if err != nil {
		return nil, err
	}
	return do[models.RoleAssignmentRead](ctx, s.facts.client, r)
}

// UnassignRole removes role from the user whose key or ID is userKey with
// DELETE /v2/facts/{proj_id}/{env_id}/users/{user_id}/roles and a body that holds the Role and
// the Tenant of role always, and its ResourceInstance when it is set, as [models.UserRoleRemove]
// says. It reads no body from a reply in 200-299 and returns nil for it: the API's spec lists a
// 204 without a body, and a 200 with the role assignment for the deprecated query parameter
// return_deleted, which the SDK does not send. The body, not the path, names the role to remove,
// so a 404 can mean a mistyped key: UnassignRole returns it as an [Error] that matches
// [ErrNotFound], never nil, also when the user does not have the role, for which the API's
// documentation says that it answers 404. With [WithFactsViaPDP], a 404 with an empty body,
// which is how the cloud PDP answers every route under /facts, is an error that matches
// [ErrContainerPDPOnly] instead.
//
// UnassignRole needs an environment-level API key, or higher, and the scope of an environment, as
// [UsersService] says. Through a copy of [UsersService.WaitForSync], with [WithFactsViaPDP], it
// waits for sync: the PDP container holds its answer until the removal reaches its policy, or the
// timeout of the wait passes. Removing a role twice leaves it removed, so the call sends its
// request again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429; when
// an attempt that got no complete reply removed the role, the next attempt can get an error
// status, such as a 404, which UnassignRole returns.
//
// UnassignRole returns an error:
//
//   - for a userKey that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request for the user;
//   - for a wait that the call cannot make, as [UsersService.WaitForSync] says: an error that
//     matches ErrInvalidConfig, and no request;
//   - for a reply with an error status: an Error with the status and the body of the reply,
//     such as one that matches ErrNotFound for a 404, or one whose Detail names each field that
//     the API rejects for a 422;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UsersService) UnassignRole(ctx context.Context, userKey string,
	role models.UserRoleRemove,
) error {
	r, err := s.facts.prepare(ctx, request{
		op: opUnassignUserRole, params: []string{userKey}, body: role,
	})
	if err != nil {
		return err
	}
	return s.facts.client.doNoValue(ctx, r)
}
