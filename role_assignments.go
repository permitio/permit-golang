package permit

import (
	"context"
	"iter"
	"time"

	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the role assignments.
const (
	opListRoleAssignments         = "list_role_assignments"
	opListRoleAssignmentsDetailed = "list_role_assignments_detailed"
	opAssignRole                  = "assign_role"
	opUnassignRole                = "unassign_role"
	opBulkAssignRole              = "bulk_assign_role"
	opBulkUnassignRole            = "bulk_unassign_role"
)

// RoleAssignmentsService lists, assigns and unassigns the role assignments of an environment,
// the roles that its users have in its tenants and on its resource instances, one at a time or in
// bulk; [Client.RoleAssignments] holds it.
//
// Its calls are calls of the facts API. By default they go to the Permit API, under
// /v2/facts/{proj_id}/{env_id}/role_assignments, and act on the environment of the client's
// scope: the project and the environment of [WithScope], or else those of the client's API key,
// which the first call that needs them looks up, as [Client.Scope] says, also when the call then
// fails before its own request. A scope without an environment, as that of a project-level API
// key without WithScope, is an error that matches [ErrInvalidConfig], and the call sends no
// request for the role assignments. With [WithFactsViaPDP], they go to the PDP container under
// /facts/role_assignments instead, need no scope, and act on the environment of the container's
// own API key.
//
// [RoleAssignmentsService.WaitForSync] returns a copy whose Assign and Unassign wait until the
// change reaches the policy of the PDP container.
type RoleAssignmentsService struct {
	facts facts
}

// RoleAssignmentsListOptions are the page and the filters of [RoleAssignmentsService.List],
// [RoleAssignmentsService.All], [RoleAssignmentsService.ListDetailed] and
// [RoleAssignmentsService.AllDetailed]. Each filter that is set keeps only the role assignments
// that match it, and the call sends it in the query parameter that its doc comment names. A
// filter that is not set, as each is in the zero RoleAssignmentsListOptions, is not sent and
// keeps every role assignment: a list without a filter of the tenant holds the role assignments
// of every tenant, and a list without a filter of the resource instance holds both the role
// assignments in a tenant and those on a resource instance.
type RoleAssignmentsListOptions struct {
	// ListOptions picks the page and the number of role assignments on it, sent in page and
	// per_page. With a PerPage of 0, List and ListDetailed leave the number to the API, which
	// gives 30, and All and AllDetailed ask for 1000, the most that the API gives.
	ListOptions

	// Users keeps the role assignments of the users with these keys, such as alice. Each is sent
	// as it is given, the empty string too, in the query parameter user, which the request
	// repeats for each, in order: user=alice&user=bob. Nil or empty keeps the role assignments of
	// every user. The API's spec takes at most 100 of them.
	Users []string

	// Roles keeps the role assignments of the roles with these keys, such as editor, each sent as
	// Users says, in the query parameter role. A key matches a role of the environment, in a
	// tenant, and a role of a resource with that key, on an instance. Nil or empty keeps the role
	// assignments of every role. The API's spec takes at most 100 of them.
	Roles []string

	// Tenants keeps the role assignments in the tenants with these keys, such as tenant-a, each
	// sent as Users says, in the query parameter tenant. Nil or empty keeps the role assignments
	// of every tenant. The API's spec takes at most 100 of them.
	Tenants []string

	// ResourceInstance keeps the role assignments on this resource instance, as
	// <resource>:<key>, such as document:doc-1, sent in the query parameter resource_instance.
	// Empty keeps the role assignments in a tenant and those on every resource instance.
	ResourceInstance string
}

// roleAssignmentsRequest returns the request of the list op of the role assignments that opts
// filters, with a parameter in its query for each value of each filter of opts that is set, and
// the page of opts. A nil opts is the zero RoleAssignmentsListOptions. The request holds copies
// of the values of opts, so a later change to opts does not change it.
func roleAssignmentsRequest(op string, opts *RoleAssignmentsListOptions,
) (request, ListOptions) {
	var o RoleAssignmentsListOptions
	if opts != nil {
		o = *opts
	}
	query := &wire.Query{}
	query.AddStrings("user", o.Users)
	query.AddStrings("role", o.Roles)
	query.AddStrings("tenant", o.Tenants)
	if o.ResourceInstance != "" {
		query.AddString("resource_instance", &o.ResourceInstance)
	}
	return request{op: op, query: query}, o.ListOptions
}

// WaitForSync returns a copy of the service whose Assign and Unassign ask the PDP container to
// wait for sync: to hold its answer until the change reaches its policy, for at most timeout, so
// that a check made after the call returns sees it. A call through the copy sends the headers
// X-Wait-Timeout, the timeout in decimal seconds, such as 0 for a timeout of 0 or 0.5 for 500
// milliseconds, and X-Timeout-Policy when opts give a policy with [WithSyncPolicy]. The service
// that WaitForSync is called on, and its other copies, keep their own waits: the copy shares no
// state with them.
//
// Only [RoleAssignmentsService.Assign] and [RoleAssignmentsService.Unassign] wait for sync: of
// the routes that RoleAssignmentsService calls, the PDP container waits on POST and DELETE
// /facts/role_assignments, and answers the lists and the bulk routes, under
// /facts/role_assignments/bulk, without waiting. A call through the copy returns an error that
// matches [ErrInvalidConfig], and sends no request: from a client without [WithFactsViaPDP], as
// only the PDP container waits; of any other method, such as List, ListDetailed,
// [RoleAssignmentsService.BulkAssign] or [RoleAssignmentsService.BulkUnassign], which the
// container answers without waiting; for a timeout less than zero; and for a [SyncOption] that is
// not valid, in any place among opts.
func (s *RoleAssignmentsService) WaitForSync(timeout time.Duration, opts ...SyncOption,
) *RoleAssignmentsService {
	return &RoleAssignmentsService{facts: s.facts.withWait(newSyncWait(timeout, opts))}
}

// List returns one page of the role assignments of the environment, in the order of the API's
// reply, those that match each filter of opts, with
// GET /v2/facts/{proj_id}/{env_id}/role_assignments. Each role assignment names its user, its
// role and its tenant by their keys, and its resource instance as the reply gives it;
// [RoleAssignmentsService.ListDetailed] returns them as objects. A nil opts, like the zero
// RoleAssignmentsListOptions, asks for the first page of every role assignment, with the API's
// default of 30 role assignments on it; the API gives at most 1000 on a page. The page holds the
// counts that the reply reports, and none when the reply is a JSON array. A page without role
// assignments, such as that of a filter that no role assignment matches, has an empty Data, not
// nil. [RoleAssignmentsService.All] reads every page.
//
// List needs an environment-level API key, or higher, and the scope of an environment, as
// [RoleAssignmentsService] says. The call changes nothing, so it sends its request again after a
// 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// List returns every failure as an error, and never a page with it:
//
//   - a Page or a PerPage less than zero: an error that matches [ErrInvalidConfig], and no
//     request for the role assignments;
//   - through a copy of [RoleAssignmentsService.WaitForSync]: an error that matches
//     ErrInvalidConfig, and no request, as the PDP container answers a list without waiting;
//   - a reply with an error status, such as 422 for a PerPage above 1000: an [Error] with the
//     status and the body of the reply;
//   - a reply in 200-299 that is not a list of role assignments, a JSON array or the paginated
//     JSON object: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RoleAssignmentsService) List(ctx context.Context, opts *RoleAssignmentsListOptions,
) (*Page[models.RoleAssignmentRead], error) {
	r, page := roleAssignmentsRequest(opListRoleAssignments, opts)
	r, err := s.facts.prepare(ctx, r)
	if err != nil {
		return nil, err
	}
	return listPage[models.RoleAssignmentRead](ctx, s.facts.client, r, arrayOrPageShape, page)
}

// All returns an iterator over the role assignments of the environment that match each filter
// of opts, for a range loop. It reads the pages in turn, as [RoleAssignmentsService.List] reads
// one, from the page of opts.Page on, or the first, each of opts.PerPage role assignments, or
// else of 1000, the most that the API gives on a page, so that 1001 role assignments take two
// pages, and the empty page after them when the replies do not report a total_count. It stops after
// the first page without role assignments, or, when the loop started at the first page, after the
// page that the total_count of its reply shows to be the last, and never because a page holds fewer
// role assignments than it asked for. It asks for the next page only when the loop wants a role
// assignment past the last of a page, so a loop that stops early sends no other request. It reads
// opts when it is called: a later change to opts does not change the iterator. Each loop over the
// iterator reads the role assignments again, from the first page that opts asks for. The pages are
// requests of their own, so a role assignment made or removed during a loop can move the others
// from one page to the next.
//
// All needs an environment-level API key, or higher, and the scope of an environment, as List
// does; each loop gets the scope before its first page. Each page is a call of its own, with its
// own timeout and retries. When a loop fails with one of the errors that List returns, such as
// for a page less than zero, through a copy of [RoleAssignmentsService.WaitForSync], or for a
// page whose call fails, the iterator yields its error once, with the zero RoleAssignmentRead,
// and stops. After [Client.Close], it yields an error that matches [ErrClosed] and sends no
// request.
func (s *RoleAssignmentsService) All(ctx context.Context, opts *RoleAssignmentsListOptions,
) iter.Seq2[models.RoleAssignmentRead, error] {
	r, page := roleAssignmentsRequest(opListRoleAssignments, opts)
	return factsAll[models.RoleAssignmentRead](ctx, s.facts, r, arrayOrPageShape, page)
}

// ListDetailed returns one page of the role assignments of the environment, as List does, with
// their user, their role, their tenant and their resource instance each as an object, with its
// key, its ID and its attributes or its permissions, with
// GET /v2/facts/{proj_id}/{env_id}/role_assignments/detailed. It sends the filters of opts as
// List does, and never the query parameter detailed, which the API marks deprecated. The API
// gives 30 role assignments on a page by default and at most 1000, and its reply is the
// paginated JSON object, whose counts the page holds. A page without role assignments has an
// empty Data, not nil. [RoleAssignmentsService.AllDetailed] reads every page.
//
// ListDetailed needs an environment-level API key, or higher, and the scope of an environment,
// as [RoleAssignmentsService] says. The call changes nothing, so it sends its request again after
// a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// ListDetailed returns every failure as an error, and never a page with it:
//
//   - a Page or a PerPage less than zero: an error that matches [ErrInvalidConfig], and no
//     request for the role assignments;
//   - through a copy of [RoleAssignmentsService.WaitForSync]: an error that matches
//     ErrInvalidConfig, and no request, as the PDP container answers a list without waiting;
//   - a reply with an error status, such as 422 for a PerPage above 1000: an [Error] with the
//     status and the body of the reply;
//   - a reply in 200-299 that is not the paginated JSON object of a list of role assignments,
//     such as a JSON array: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RoleAssignmentsService) ListDetailed(ctx context.Context,
	opts *RoleAssignmentsListOptions,
) (*Page[models.RoleAssignmentDetailedRead], error) {
	r, page := roleAssignmentsRequest(opListRoleAssignmentsDetailed, opts)
	r, err := s.facts.prepare(ctx, r)
	if err != nil {
		return nil, err
	}
	return listPage[models.RoleAssignmentDetailedRead](ctx, s.facts.client, r, pageShape, page)
}

// AllDetailed returns an iterator over the role assignments of the environment that match each
// filter of opts, with their user, their role, their tenant and their resource instance as
// objects, for a range loop. It reads the pages in turn, as [RoleAssignmentsService.ListDetailed]
// reads one, as All does: from the page of opts.Page on, or the first, each of opts.PerPage role
// assignments, or else of 1000, the most that the API gives on a page. It stops after the first
// page without role assignments, or, when the loop started at the first page, after the page that
// the total_count of its reply shows to be the last, so that 1001 role assignments take two pages,
// and never because a page holds fewer role assignments than it asked for. It asks for the next
// page only when the loop wants a role assignment past the last of a page, so a loop that stops
// early sends no other request. It reads opts when it is called, and each loop over the iterator
// reads the role assignments again, as All does.
//
// AllDetailed needs an environment-level API key, or higher, and the scope of an environment, as
// ListDetailed does; each loop gets the scope before its first page. Each page is a call of its
// own, with its own timeout and retries. When a loop fails with one of the errors that
// ListDetailed returns, the iterator yields its error once, with the zero
// RoleAssignmentDetailedRead, and stops. After [Client.Close], it yields an error that matches
// [ErrClosed] and sends no request.
func (s *RoleAssignmentsService) AllDetailed(ctx context.Context,
	opts *RoleAssignmentsListOptions,
) iter.Seq2[models.RoleAssignmentDetailedRead, error] {
	r, page := roleAssignmentsRequest(opListRoleAssignmentsDetailed, opts)
	return factsAll[models.RoleAssignmentDetailedRead](ctx, s.facts, r, pageShape, page)
}

// Assign assigns a role to a user with POST /v2/facts/{proj_id}/{env_id}/role_assignments, and
// returns the role assignment as the API stores it. The request holds the User and the Role of
// assignment always, and its Tenant and its ResourceInstance when each is set, as
// [models.RoleAssignmentCreate] says: a role in a tenant names the tenant, and a role of a
// resource on one of its instances names the instance too, such as document:doc-1. An empty
// Tenant is left out of the request, not sent as an empty string.
//
// Assign needs an environment-level API key, or higher, and the scope of an environment, as
// [RoleAssignmentsService] says. Through a copy of [RoleAssignmentsService.WaitForSync], with
// [WithFactsViaPDP], it waits for sync: the PDP container holds its answer until the new role
// assignment reaches its policy, or the timeout of the wait passes, so that a check made after
// Assign returns sees the role. It is sent again only after a 429, which the API did not act on:
// after a 500, 502, 503 or 504 reply or a failed connection, the API may have assigned the role,
// so Assign returns that error, and a List of the user's role assignments tells whether the user
// has the role.
//
// Assign returns an error, and no role assignment:
//
//   - for a wait that the call cannot make, as [RoleAssignmentsService.WaitForSync] says: an
//     error that matches [ErrInvalidConfig], and no request;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrConflict] for a 409, which the API's spec lists for an assignment, and
//     [ErrNotFound] for a 404, and whose Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a role assignment, such as an empty
//     body: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RoleAssignmentsService) Assign(ctx context.Context,
	assignment models.RoleAssignmentCreate,
) (*models.RoleAssignmentRead, error) {
	r, err := s.facts.prepare(ctx, request{op: opAssignRole, body: assignment})
	if err != nil {
		return nil, err
	}
	return do[models.RoleAssignmentRead](ctx, s.facts.client, r)
}

// Unassign removes a role from a user with DELETE /v2/facts/{proj_id}/{env_id}/role_assignments
// and a body that holds the User, the Role and the Tenant of assignment always, and its
// ResourceInstance when it is set, as [models.RoleAssignmentRemove] says. It never sends the
// query parameter return_deleted, which the API marks deprecated, and reads no body from a reply
// in 200-299, for which it returns nil: the API's spec lists a 204 without a body. The body, not
// the path, names the role assignment to remove, so a 404 can mean a mistyped key: Unassign
// returns it as an [Error] that matches [ErrNotFound], never nil, also when the user does not
// have the role. With [WithFactsViaPDP], a 404 with an empty body, which is how the cloud PDP
// answers every route under /facts, is an error that matches [ErrContainerPDPOnly] instead.
//
// Unassign needs an environment-level API key, or higher, and the scope of an environment, as
// [RoleAssignmentsService] says. Through a copy of [RoleAssignmentsService.WaitForSync], with
// [WithFactsViaPDP], it waits for sync: the PDP container holds its answer until the removal
// reaches its policy, or the timeout of the wait passes. Removing a role twice leaves it
// removed, so the call sends its request again after a 500, 502, 503 or 504 reply or a failed
// connection, as after a 429; when an attempt that got no complete reply removed the role, the
// next attempt can get an error status, such as a 404, which Unassign returns.
//
// Unassign returns an error:
//
//   - for a wait that the call cannot make, as [RoleAssignmentsService.WaitForSync] says: an
//     error that matches [ErrInvalidConfig], and no request;
//   - for a reply with an error status: an Error with the status and the body of the reply,
//     such as one that matches ErrNotFound for a 404, or one whose Detail names each field that
//     the API rejects for a 422;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RoleAssignmentsService) Unassign(ctx context.Context,
	assignment models.RoleAssignmentRemove,
) error {
	r, err := s.facts.prepare(ctx, request{op: opUnassignRole, body: assignment})
	if err != nil {
		return err
	}
	return s.facts.client.doNoValue(ctx, r)
}

// RoleAssignmentsBulkAssignOptions are the options of [RoleAssignmentsService.BulkAssign]. The
// zero RoleAssignmentsBulkAssignOptions, like a nil one, sends none of them.
type RoleAssignmentsBulkAssignOptions struct {
	// MissingUserPolicy is the policy of the call for a user that the environment does not have,
	// sent as it is given in the query parameter missing_user_policy, also a value that
	// [models.MissingUserPolicy] does not list. Empty leaves the parameter out, and the policy to
	// the API, whose spec gives fail as its default.
	MissingUserPolicy models.MissingUserPolicy
}

// BulkAssign assigns roles to users in one request, with
// POST /v2/facts/{proj_id}/{env_id}/role_assignments/bulk, and returns the report of the reply,
// which counts the role assignments that the call created. The body is a JSON array of
// assignments, in order, each as [RoleAssignmentsService.Assign] sends one: with the User and the
// Role always, and the Tenant and the ResourceInstance when each is set, as
// [models.RoleAssignmentCreate] says, so an empty Tenant is left out of its item, not sent as an
// empty string. Nil sends [], as an empty slice does: the API's spec sets no least number of
// items. The query holds missing_user_policy when opts sets it, as
// [RoleAssignmentsBulkAssignOptions] says, and is empty otherwise; a nil opts is the zero
// RoleAssignmentsBulkAssignOptions.
//
// BulkAssign needs an environment-level API key, or higher, and the scope of an environment, as
// [RoleAssignmentsService] says. With [WithFactsViaPDP], it goes to the PDP container, under
// /facts/role_assignments/bulk, which sends it on to the API and answers without waiting for its
// policy. The API's spec lists a 409 for it, so it is sent again only after a 429, which the API
// did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have
// assigned some of the roles, so BulkAssign returns that error, and a List of the users' role
// assignments tells which roles they have.
//
// BulkAssign returns an error, and no report:
//
//   - through a copy of [RoleAssignmentsService.WaitForSync]: an error that matches
//     [ErrInvalidConfig], and no request, as RoleAssignmentsService.WaitForSync says: the PDP
//     container answers the bulk routes without waiting;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrConflict] for a 409 and [ErrNotFound] for a 404, both of which the API's
//     spec lists for the route, and whose Detail names each field that the API rejects for a
//     422; with WithFactsViaPDP, a 404 with an empty body, which is how the cloud PDP answers
//     every route under /facts, matches [ErrContainerPDPOnly] instead of ErrNotFound;
//   - for a reply in 200-299 that is not a JSON object of a report, such as an empty body: an
//     Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RoleAssignmentsService) BulkAssign(ctx context.Context,
	assignments []models.RoleAssignmentCreate, opts *RoleAssignmentsBulkAssignOptions,
) (*models.BulkRoleAssignmentReport, error) {
	var o RoleAssignmentsBulkAssignOptions
	if opts != nil {
		o = *opts
	}
	query := &wire.Query{}
	if o.MissingUserPolicy != "" {
		query.AddString("missing_user_policy", new(string(o.MissingUserPolicy)))
	}
	r, err := s.facts.prepare(ctx, request{
		op: opBulkAssignRole, query: query, body: orEmpty(assignments),
	})
	if err != nil {
		return nil, err
	}
	return do[models.BulkRoleAssignmentReport](ctx, s.facts.client, r)
}

// BulkUnassign removes roles from users in one request, with
// DELETE /v2/facts/{proj_id}/{env_id}/role_assignments/bulk, and returns the report of the
// reply, which counts the role assignments that its request removed. The body is a JSON array of
// unassignments, in order, each as [RoleAssignmentsService.Unassign] sends one: with the User,
// the Role and the Tenant always, also when they are empty, and the ResourceInstance when it is
// set, as [models.RoleAssignmentRemove] says. Nil sends [], as an empty slice does: the API's
// spec sets no least number of items. The body, not the path, names the role assignments to
// remove, so a 404 can mean a mistyped key: BulkUnassign returns it as an [Error] that matches
// [ErrNotFound], never a report. With [WithFactsViaPDP], a 404 with an empty body, which is how
// the cloud PDP answers every route under /facts, is an error that matches [ErrContainerPDPOnly]
// instead.
//
// BulkUnassign needs an environment-level API key, or higher, and the scope of an environment,
// as [RoleAssignmentsService] says. With WithFactsViaPDP, it goes to the PDP container, under
// /facts/role_assignments/bulk, which sends it on to the API and answers without waiting for its
// policy. Removing roles twice leaves them removed, so the call sends its request again after a
// 500, 502, 503 or 504 reply or a failed connection, as after a 429. BulkUnassign returns the
// report of the last attempt, which may not count roles that an earlier attempt without a
// complete reply removed; that attempt can also get an error status, which BulkUnassign returns.
//
// BulkUnassign returns an error, and no report:
//
//   - through a copy of [RoleAssignmentsService.WaitForSync]: an error that matches
//     [ErrInvalidConfig], and no request, as RoleAssignmentsService.WaitForSync says: the PDP
//     container answers the bulk routes without waiting;
//   - for a reply with an error status: an Error with the status and the body of the reply,
//     such as one that matches ErrNotFound for a 404, or one whose Detail names each field that
//     the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a report, such as an empty body: an
//     Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RoleAssignmentsService) BulkUnassign(ctx context.Context,
	unassignments []models.RoleAssignmentRemove,
) (*models.BulkRoleUnAssignmentReport, error) {
	r, err := s.facts.prepare(ctx, request{op: opBulkUnassignRole, body: orEmpty(unassignments)})
	if err != nil {
		return nil, err
	}
	return do[models.BulkRoleUnAssignmentReport](ctx, s.facts.client, r)
}
