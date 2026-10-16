package permit

import (
	"context"
	"iter"
	"time"

	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the tenants and of the users in them.
const (
	opListTenants  = "list_tenants"
	opCreateTenant = "create_tenant"
	opGetTenant    = "get_tenant"
	opUpdateTenant = "update_tenant"
	opDeleteTenant = "delete_tenant"

	opListTenantUsers  = "list_tenant_users"
	opAddTenantUser    = "add_user_to_tenant"
	opRemoveTenantUser = "delete_tenant_user"
)

// TenantsService lists, reads, creates, changes and deletes the tenants of an environment, and
// lists, adds and removes the users in a tenant; [Client.Tenants] holds it. A method that acts on
// one tenant takes its key or its ID, such as tenant-a, and one that acts on a user in it the key
// or the ID of the user too, such as alice, and sends each as one segment of the path, escaped: a
// key that holds a / or a % names that tenant or that user, not another route.
//
// Its calls are calls of the facts API. By default they go to the Permit API, under
// /v2/facts/{proj_id}/{env_id}/tenants, and act on the environment of the client's scope: the
// project and the environment of [WithScope], or else those of the client's API key, which the
// first call that needs them looks up, as [Client.Scope] says, also when the call then fails
// before its own request. A scope without an environment, as that of a project-level API key
// without WithScope, is an error that matches [ErrInvalidConfig], and the call sends no request
// for the tenants. With [WithFactsViaPDP], they go to the PDP container under /facts/tenants
// instead, need no scope, and act on the environment of the container's own API key.
//
// [TenantsService.WaitForSync] returns a copy whose creates of tenants wait until the new tenant
// reaches the policy of the PDP container.
type TenantsService struct {
	facts facts
}

// TenantsListOptions are the page and the filters of [TenantsService.List] and
// [TenantsService.All].
type TenantsListOptions struct {
	// ListOptions picks the page and the number of tenants on it, sent in page and per_page.
	// With a PerPage of 0, List leaves the number to the API, which gives 30, and All asks for
	// 100, the most that the API gives.
	ListOptions

	// Attributes keep the tenants whose attributes match each filter, as [AttributeFilter]
	// says, such as permit.AttributeEquals("tier", "gold"); nil or empty keeps every tenant.
	Attributes []AttributeFilter
}

// tenantsRequest returns the request of the tenants that opts filters, with the parameter of
// each attribute filter of opts in its query, and the page of opts. A nil opts is the zero
// TenantsListOptions. The request holds copies of the values of opts, so a later change to opts
// does not change it. It returns the error of an attribute filter that is not valid.
func tenantsRequest(opts *TenantsListOptions) (request, ListOptions, error) {
	var o TenantsListOptions
	if opts != nil {
		o = *opts
	}
	query := &wire.Query{}
	if err := addAttributeFilters(query, opListTenants, o.Attributes); err != nil {
		return request{}, ListOptions{}, err
	}
	return request{op: opListTenants, query: query}, o.ListOptions, nil
}

// TenantUsersListOptions are the page and the filters of [TenantsService.ListUsers] and
// [TenantsService.AllUsers].
type TenantUsersListOptions struct {
	// ListOptions picks the page and the number of users on it, sent in page and per_page.
	// With a PerPage of 0, ListUsers leaves the number to the API, which gives 30, and AllUsers
	// asks for 100, the most that the API gives.
	ListOptions

	// Search keeps the users that the API's text search for it matches, sent in search, which
	// the API's documentation describes as a text search of the email of a user. Empty keeps
	// every user in the tenant.
	Search string

	// Role keeps the users that have the role of this key, sent in role; empty keeps every user
	// in the tenant.
	Role string
}

// tenantUsersRequest returns the request of the users in the tenant tenantKey that opts filters,
// with search and role in its query when opts sets them, and the page of opts. A nil opts is the
// zero TenantUsersListOptions. The request holds copies of the values of opts, so a later change
// to opts does not change it.
func tenantUsersRequest(tenantKey string, opts *TenantUsersListOptions) (request, ListOptions) {
	var o TenantUsersListOptions
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
	r := request{op: opListTenantUsers, params: []string{tenantKey}, query: query}
	return r, o.ListOptions
}

// WaitForSync returns a copy of the service whose creates ask the PDP container to wait for
// sync: to hold its answer to the create until the new tenant reaches its policy, for at most
// timeout, so that a check made after the create returns sees the tenant. A call through the
// copy sends the headers X-Wait-Timeout, the timeout in decimal seconds, such as 0 for a timeout
// of 0 or 0.5 for 500 milliseconds, and X-Timeout-Policy when opts give a policy with
// [WithSyncPolicy]. The service that WaitForSync is called on, and its other copies, keep their
// own waits: the copy shares no state with them.
//
// Only [TenantsService.Create] waits for sync, as the PDP container waits on POST /facts/tenants
// alone. A call through the copy returns an error that matches [ErrInvalidConfig], and sends no
// request: from a client without [WithFactsViaPDP], as only the PDP container waits; of any
// other method, such as AddUser, Update or Delete, which the container answers without waiting;
// for a timeout less than zero; and for a [SyncOption] that is not valid, in any place among opts.
func (s *TenantsService) WaitForSync(timeout time.Duration, opts ...SyncOption) *TenantsService {
	return &TenantsService{facts: s.facts.withWait(newSyncWait(timeout, opts))}
}

// List returns one page of the tenants of the environment, in the order of the API's reply,
// those that match each attribute filter of opts, with GET /v2/facts/{proj_id}/{env_id}/tenants.
// A nil opts, like the zero TenantsListOptions, asks for the first page of every tenant, with
// the API's default of 30 tenants on it; the API gives at most 100 on a page. The page holds the
// counts that the reply reports, and none when the reply is a JSON array.
// [TenantsService.All] reads every page.
//
// List needs an environment-level API key, or higher, and the scope of an environment, as
// [TenantsService] says. The call changes nothing, so it sends its request again after a 500,
// 502, 503 or 504 reply or a failed connection, as after a 429.
//
// List returns every failure as an error, and never a page with it:
//
//   - a Page or a PerPage less than zero, or an attribute filter that is not valid: an error
//     that matches [ErrInvalidConfig], and no request for the tenants;
//   - through a copy of [TenantsService.WaitForSync]: an error that matches ErrInvalidConfig,
//     and no request, as the PDP container answers a list without waiting;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply;
//   - a reply in 200-299 that is not a list of tenants, a JSON array or the paginated JSON
//     object: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *TenantsService) List(ctx context.Context, opts *TenantsListOptions,
) (*Page[models.TenantRead], error) {
	if err := s.facts.client.checkOpen(); err != nil {
		return nil, err
	}
	r, page, err := tenantsRequest(opts)
	if err != nil {
		return nil, err
	}
	if r, err = s.facts.prepare(ctx, r); err != nil {
		return nil, err
	}
	return listPage[models.TenantRead](ctx, s.facts.client, r, arrayOrPageShape, page)
}

// All returns an iterator over the tenants of the environment that match each attribute filter
// of opts, for a range loop. It reads the pages in turn, as [TenantsService.List] reads one, from
// the page of opts.Page on, or the first, each of opts.PerPage tenants, or else of 100, the most
// that the API gives on a page. It stops after the last page that the replies count, or after
// the first page without tenants, and never after a page with fewer tenants than it asked for.
// It asks for the next page only when the loop wants a tenant past the last of a page, so a loop
// that stops early sends no other request. It reads opts when it is called: a later change to
// opts does not change the iterator. Each loop over the iterator reads the tenants again, from
// the first page that opts asks for. The pages are requests of their own, so a tenant created
// or deleted during a loop can move the others from one page to the next.
//
// All needs an environment-level API key, or higher, and the scope of an environment, as List
// does; each loop gets the scope before its first page. Each page is a call of its own, with its
// own timeout and retries. When a loop fails with one of the errors that List returns, such as
// for options that are not valid, through a copy of [TenantsService.WaitForSync], or for a page
// whose call fails, the iterator yields its error once, with the zero TenantRead, and stops.
// After [Client.Close], it yields an error that matches [ErrClosed] and sends no request.
func (s *TenantsService) All(ctx context.Context, opts *TenantsListOptions,
) iter.Seq2[models.TenantRead, error] {
	r, page, optsErr := tenantsRequest(opts)
	if optsErr != nil {
		return func(yield func(models.TenantRead, error) bool) {
			err := s.facts.client.checkOpen()
			if err == nil {
				err = optsErr
			}
			yield(models.TenantRead{}, err)
		}
	}
	return factsAll[models.TenantRead](ctx, s.facts, r, arrayOrPageShape, page)
}

// Get returns the tenant whose key or ID is tenantKey, with
// GET /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}.
//
// Get needs an environment-level API key, or higher, and the scope of an environment, as
// [TenantsService] says. The call changes nothing, so it sends its request again after a 500,
// 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no tenant:
//
//   - for a tenantKey that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request for the tenant;
//   - through a copy of [TenantsService.WaitForSync]: an error that matches ErrInvalidConfig,
//     and no request, as the PDP container answers a read without waiting;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the environment has no such tenant;
//   - for a reply in 200-299 that is not a JSON object of a tenant: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *TenantsService) Get(ctx context.Context, tenantKey string) (*models.TenantRead, error) {
	r, err := s.facts.prepare(ctx, request{op: opGetTenant, params: []string{tenantKey}})
	if err != nil {
		return nil, err
	}
	return do[models.TenantRead](ctx, s.facts.client, r)
}

// Create creates a tenant in the environment with POST /v2/facts/{proj_id}/{env_id}/tenants,
// and returns it as the API stores it, with its ID.
//
// Create needs an environment-level API key, or higher, and the scope of an environment, as
// [TenantsService] says. Through a copy of [TenantsService.WaitForSync], with [WithFactsViaPDP],
// it waits for sync: the PDP container holds its answer until the new tenant reaches its policy,
// or the timeout of the wait passes. It is sent again only after a 429, which the API did not act
// on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have created the
// tenant, so Create returns that error, and [TenantsService.Get] of the key tells whether the
// tenant exists.
//
// Create returns an error, and no tenant:
//
//   - for a wait that the call cannot make, as [TenantsService.WaitForSync] says: an error that
//     matches [ErrInvalidConfig], and no request;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrConflict] when the environment has a tenant with the key already, and
//     whose Detail names each field that the API rejects for a 422, such as a key that is not
//     URL-friendly;
//   - for a reply in 200-299 that is not a JSON object of a tenant: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *TenantsService) Create(ctx context.Context, tenant models.TenantCreate,
) (*models.TenantRead, error) {
	r, err := s.facts.prepare(ctx, request{op: opCreateTenant, body: tenant})
	if err != nil {
		return nil, err
	}
	return do[models.TenantRead](ctx, s.facts.client, r)
}

// Update changes the tenant whose key or ID is tenantKey with
// PATCH /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}, and returns it as the API stores it.
// The request holds the fields of update that are set, as [models.TenantUpdate] says, and the
// tenant keeps the others: the zero TenantUpdate changes nothing. Attributes that are set
// replace the tenant's attributes as a whole.
//
// Update needs an environment-level API key, or higher, and the scope of an environment, as
// [TenantsService] says. Sending the same update twice sets the same fields, so the call sends
// its request again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Update returns an error, and no tenant:
//
//   - for a tenantKey that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request for the tenant;
//   - through a copy of [TenantsService.WaitForSync]: an error that matches ErrInvalidConfig,
//     and no request, as the PDP container answers an update without waiting;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the environment has no such tenant, and whose Detail
//     names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a tenant: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *TenantsService) Update(ctx context.Context, tenantKey string,
	update models.TenantUpdate,
) (*models.TenantRead, error) {
	r, err := s.facts.prepare(ctx, request{
		op: opUpdateTenant, params: []string{tenantKey}, body: update,
	})
	if err != nil {
		return nil, err
	}
	return do[models.TenantRead](ctx, s.facts.client, r)
}

// Delete deletes the tenant whose key or ID is tenantKey with
// DELETE /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}. It returns nil when the API answers
// 404, as the tenant is gone already, such as when an earlier attempt of the call deleted it; so
// a mistyped key returns nil too, and deletes nothing. With [WithFactsViaPDP], a 404 with an
// empty body, which is how the cloud PDP answers every route under /facts, is an error that
// matches [ErrContainerPDPOnly] instead.
//
// Delete needs an environment-level API key, or higher, and the scope of an environment, as
// [TenantsService] says. Deleting a tenant twice leaves it deleted, so the call sends its
// request again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Delete returns an error:
//
//   - for a tenantKey that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request for the tenant;
//   - through a copy of [TenantsService.WaitForSync]: an error that matches ErrInvalidConfig,
//     and no request, as the PDP container answers a delete without waiting;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete the
//     tenant;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *TenantsService) Delete(ctx context.Context, tenantKey string) error {
	r, err := s.facts.prepare(ctx, request{op: opDeleteTenant, params: []string{tenantKey}})
	if err != nil {
		return err
	}
	return s.facts.client.doNoValue(ctx, r)
}

// ListUsers returns one page of the users in the tenant whose key or ID is tenantKey, in the
// order of the API's reply, those that match each filter of opts, with
// GET /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}/users. A nil opts, like the zero
// TenantUsersListOptions, asks for the first page of every user in the tenant, with the API's
// default of 30 users on it; the API gives at most 100 on a page. The page holds the counts that
// the reply reports. [TenantsService.AllUsers] reads every page. A list does not ask for the
// users' roles on resource instances, so the ResourceInstanceRoles of each of their
// AssociatedTenants may be empty; [UsersService.Get] returns them.
//
// ListUsers needs an environment-level API key, or higher, and the scope of an environment, as
// [TenantsService] says. The call changes nothing, so it sends its request again after a 500,
// 502, 503 or 504 reply or a failed connection, as after a 429.
//
// ListUsers returns every failure as an error, and never a page with it:
//
//   - a tenantKey that is empty, . or .., or a Page or a PerPage less than zero: an error that
//     matches [ErrInvalidConfig], and no request for the users;
//   - through a copy of [TenantsService.WaitForSync]: an error that matches ErrInvalidConfig,
//     and no request, as the PDP container answers a list without waiting;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply;
//   - a reply in 200-299 that is not the paginated JSON object of a list of users: an Error
//     that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *TenantsService) ListUsers(ctx context.Context, tenantKey string,
	opts *TenantUsersListOptions,
) (*Page[models.UserRead], error) {
	r, page := tenantUsersRequest(tenantKey, opts)
	r, err := s.facts.prepare(ctx, r)
	if err != nil {
		return nil, err
	}
	return listPage[models.UserRead](ctx, s.facts.client, r, pageShape, page)
}

// AllUsers returns an iterator over the users in the tenant whose key or ID is tenantKey that
// match each filter of opts, for a range loop. It reads the pages in turn, as
// [TenantsService.ListUsers] reads one, from the page of opts.Page on, or the first, each of
// opts.PerPage users, or else of 100, the most that the API gives on a page. It stops after the
// last page that the replies count, or after the first page without users, and never after a
// page with fewer users than it asked for. It asks for the next page only when the loop wants a
// user past the last of a page, so a loop that stops early sends no other request. It reads
// opts when it is called: a later change to opts does not change the iterator. Each loop over
// the iterator reads the users again, from the first page that opts asks for. The pages are
// requests of their own, so a user added to the tenant or removed from it during a loop can move
// the others from one page to the next. Like ListUsers, AllUsers does not ask for the users'
// roles on resource instances, so they may be empty; [UsersService.Get] returns them.
//
// AllUsers needs an environment-level API key, or higher, and the scope of an environment, as
// ListUsers does; each loop gets the scope before its first page. Each page is a call of its own,
// with its own timeout and retries. When a loop fails with one of the errors that ListUsers
// returns, such as for a tenantKey that is empty, through a copy of [TenantsService.WaitForSync],
// or for a page whose call fails, the iterator yields its error once, with the zero UserRead,
// and stops. After [Client.Close], it yields an error that matches [ErrClosed] and sends no
// request.
func (s *TenantsService) AllUsers(ctx context.Context, tenantKey string,
	opts *TenantUsersListOptions,
) iter.Seq2[models.UserRead, error] {
	r, page := tenantUsersRequest(tenantKey, opts)
	return factsAll[models.UserRead](ctx, s.facts, r, pageShape, page)
}

// AddUser adds user to the tenant whose key or ID is tenantKey with
// POST /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}/users, and returns the user as the API
// stores it, with its ID. The request holds user as [UsersService.Create] sends it, as
// [models.UserCreate] says, with its RoleAssignments when it has any, and none when it has none.
// The API's documentation describes the call as one that creates the user and adds it to the
// tenant, so a user without RoleAssignments is in the tenant without a role; it says that the API
// applies each of the RoleAssignments as the create of a user does, in the tenant that it names,
// which can be another tenant, and adds the user to that tenant too.
//
// AddUser needs an environment-level API key, or higher, and the scope of an environment, as
// [TenantsService] says. It is sent again only after a 429, which the API did not act on: after a
// 500, 502, 503 or 504 reply or a failed connection, the API may have created the user, so
// AddUser returns that error, and [UsersService.Get] of the key tells whether the user exists.
//
// AddUser returns an error, and no user:
//
//   - for a tenantKey that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request for the tenant;
//   - through a copy of [TenantsService.WaitForSync]: an error that matches ErrInvalidConfig,
//     and no request, as the PDP container answers the add of a user without waiting;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrConflict] for a 409, which the API's documentation lists for causes such
//     as a user that exists already, and [ErrNotFound] for a 404, such as for a tenant that the
//     environment does not have, and whose Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a user, such as an empty body: an
//     Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *TenantsService) AddUser(ctx context.Context, tenantKey string, user models.UserCreate,
) (*models.UserRead, error) {
	r, err := s.facts.prepare(ctx, request{
		op: opAddTenantUser, params: []string{tenantKey}, body: user,
	})
	if err != nil {
		return nil, err
	}
	return do[models.UserRead](ctx, s.facts.client, r)
}

// RemoveUser removes the user whose key or ID is userKey from the tenant whose key or ID is
// tenantKey with DELETE /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}/users/{user_id}, which
// the API's documentation describes as the delete of a user under a tenant; what it does to the
// user's roles in the tenant, and to the user in the environment, is not confirmed yet. It
// returns nil when the API answers 404, as the path names a user that is not in the tenant, such
// as when an earlier attempt of the call removed it; so a mistyped key returns nil too, and
// removes nothing. With [WithFactsViaPDP], a 404 with an empty body, which is how the cloud PDP
// answers every route under /facts, is an error that matches [ErrContainerPDPOnly] instead.
//
// RemoveUser needs an environment-level API key, or higher, and the scope of an environment, as
// [TenantsService] says. Removing a user twice leaves it removed, so the call sends its request
// again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// RemoveUser returns an error:
//
//   - for a tenantKey or a userKey that is empty, . or ..: an error that matches
//     [ErrInvalidConfig], and no request for the user;
//   - through a copy of [TenantsService.WaitForSync]: an error that matches ErrInvalidConfig,
//     and no request, as the PDP container answers the removal of a user without waiting;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not remove the
//     user;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *TenantsService) RemoveUser(ctx context.Context, tenantKey, userKey string) error {
	r, err := s.facts.prepare(ctx, request{
		op: opRemoveTenantUser, params: []string{tenantKey, userKey},
	})
	if err != nil {
		return err
	}
	return s.facts.client.doNoValue(ctx, r)
}
