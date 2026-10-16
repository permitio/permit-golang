package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the groups of an environment and of their users.
const (
	opCreateGroup         = "create_group"
	opListGroups          = "list_direct_group"
	opGetGroup            = "get_direct_group"
	opDeleteGroup         = "delete_group"
	opListGroupUsers      = "list_group_users"
	opAssignUserToGroup   = "assign_user_to_group" //nolint:gosec // G101: an operation ID
	opRemoveUserFromGroup = "remove_user_from_group"
)

// GroupsService lists, reads, creates and deletes the groups of an environment, assigns users to
// a group, lists them and removes them, assigns roles and other groups to a group, lists them and
// removes them, and lists the parent groups of a group; [Client.Groups] holds it. A group is in a
// tenant, and the spec names it by the key of its instance, such as engineering, and the key of
// the resource of that instance, group by default, as [models.GroupCreate] says.
//
// A method that acts on one group takes it as groupInstanceKey and sends it as it is given, as
// one segment of the path, escaped: a key that holds a / or a % names that group, not another
// route. The SDK does not tell an instance key from an ID, so a key in the shape of a UUID is
// sent as it is, as any other key is. The API's documentation says that the path takes "Either
// the unique id of the resource instance, or the URL-friendly key of the resource instance" of
// the group; that the path takes the ID of the group is not confirmed yet. A method that acts on
// one user of a group takes the key or the ID of the user next, as userKey, and sends it in the
// same way. A path can fit two routes: /groups/direct/users is the list of the users of the group
// direct, as [GroupsService.ListUsers] sends it, and the group users, as [GroupsService.Get]
// sends it, and /groups/direct/roles, /groups/direct/children and /groups/direct/parents fit two
// routes in the same way; which of the two the API answers is not confirmed yet.
//
// The SDK lists the groups and reads a group under /groups/direct, and never sends the GET of
// /groups or of /groups/{group_instance_key}, which the API's spec marks deprecated.
//
// Its calls act on the environment of the client's scope, whose project and environment fill the
// route: those that [WithScope] gives, or else those of the client's API key, which the first call
// that needs them looks up once, as [Client.Scope] says. A scope without an environment, such as
// that of a project-level API key without WithScope, is an error that matches [ErrInvalidConfig],
// and the call sends no request for its operation. Each method is sent with an
// environment-level API key of the environment, or with a key of a wider level and WithScope; the
// API answers a key that may not act on the environment with an error status, which the call
// returns as an [Error], such as one that matches [ErrForbidden].
type GroupsService struct {
	client *Client
}

// GroupsListOptions are the page and the filters of [GroupsService.List] and
// [GroupsService.All]. Each filter that is set is sent as it is given; one that is empty is left
// out of the query. The spec does not describe the filters, and what each keeps is not confirmed
// yet.
type GroupsListOptions struct {
	// ListOptions picks the page and the number of groups on it, sent in page and per_page. With
	// a PerPage of 0, List leaves the number to the API, which gives 30, and All asks for 100,
	// the most that the API gives.
	ListOptions

	// Tenant is sent in the query parameter tenant, as it is given, and left out when empty.
	Tenant string

	// Resource is sent in the query parameter resource, as it is given, and left out when empty.
	Resource string

	// Search is sent in the query parameter search, as it is given, and left out when empty.
	Search string
}

// groupsRequest returns the request of the groups that opts filters, with tenant, resource and
// search in its query when opts sets them, and the page of opts. A nil opts is the zero
// GroupsListOptions. The request holds copies of the values of opts, so a later change to opts
// does not change it.
func groupsRequest(opts *GroupsListOptions) (request, ListOptions) {
	var o GroupsListOptions
	if opts != nil {
		o = *opts
	}
	query := &wire.Query{}
	for _, filter := range []struct{ name, value string }{
		{"tenant", o.Tenant}, {"resource", o.Resource}, {"search", o.Search},
	} {
		if filter.value != "" {
			query.AddString(filter.name, &filter.value)
		}
	}
	return request{op: opListGroups, target: apiTarget, query: query}, o.ListOptions
}

// List returns one page of the groups of the environment, in the order of the API's reply, those
// that match each filter of opts, with GET /v2/schema/{proj_id}/{env_id}/groups/direct. A nil
// opts, like the zero GroupsListOptions, asks for the first page of every group, with the API's
// default of 30 groups on it; the API gives at most 100 on a page. The reply is the paginated
// object, and the page holds its counts: the TotalCount of the groups and the PageCount, each nil
// when the reply leaves it out. [GroupsService.All] reads every page.
//
// List is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [GroupsService] says. The call changes nothing, so it sends its request again
// after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// List returns every failure as an error, and never a page with it:
//
//   - a scope without an environment: an error that matches [ErrInvalidConfig], and no request
//     for the list; without WithScope, the lookup of the scope comes first;
//   - a Page or a PerPage less than zero: an error that matches ErrInvalidConfig, and no request
//     for the list;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply;
//   - a reply in 200-299 that is not the paginated JSON object of groups, such as a JSON array:
//     an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *GroupsService) List(ctx context.Context, opts *GroupsListOptions,
) (*Page[models.GroupReadSchema], error) {
	r, page := groupsRequest(opts)
	r, err := s.client.inScope(ctx, r)
	if err != nil {
		return nil, err
	}
	return listPage[models.GroupReadSchema](ctx, s.client, r, pageShape, page)
}

// All returns an iterator over the groups of the environment that match each filter of opts, for
// a range loop. It reads the pages in turn, as [GroupsService.List] reads one, from the page of
// opts.Page on, or the first, each of opts.PerPage groups, or else of 100, the most that the API
// gives on a page. It stops after the last page that the replies count, or after the first page
// without groups, and never after a page with fewer groups than it asked for. It asks for the
// next page only when the loop wants a group past the last of a page, so a loop that stops early
// sends no other request. It reads opts when it is called: a later change to opts does not change
// the iterator. Each loop over the iterator reads the groups again, from the first page that opts
// asks for. The pages are requests of their own, so a group created or deleted during a loop can
// move the others from one page to the next.
//
// All is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as List is; each loop gets the scope as List does before its first page. Each
// page is a call of its own, with its own timeout and retries. When the scope or a page fails,
// with one of the errors that List returns, the iterator yields its error once, with the zero
// GroupReadSchema, and stops.
func (s *GroupsService) All(ctx context.Context, opts *GroupsListOptions,
) iter.Seq2[models.GroupReadSchema, error] {
	r, page := groupsRequest(opts)
	return allInScope[models.GroupReadSchema](ctx, s.client, r, pageShape, page)
}

// Get returns the group of the instance key groupInstanceKey, with
// GET /v2/schema/{proj_id}/{env_id}/groups/direct/{group_instance_key}, as the API returns it,
// with its ID.
//
// Get is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [GroupsService] says. The call changes nothing, so it sends its request again
// after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no group:
//
//   - for a scope without an environment, or a groupInstanceKey that is empty, . or ..: an error
//     that matches [ErrInvalidConfig], and no request for the group; without WithScope, the
//     lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the environment has no such group;
//   - for a reply in 200-299 that is not a JSON object of a group: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *GroupsService) Get(ctx context.Context, groupInstanceKey string,
) (*models.GroupReadSchema, error) {
	r, err := s.client.inScope(ctx, request{
		op: opGetGroup, target: apiTarget, params: []string{groupInstanceKey},
	})
	if err != nil {
		return nil, err
	}
	return do[models.GroupReadSchema](ctx, s.client, r)
}

// Create creates a group with POST /v2/schema/{proj_id}/{env_id}/groups, and returns it as the
// API returns it, with the roles assigned to it and the IDs of its users. The request holds the
// instance key and the tenant of group, and its GroupResourceTypeKey when it is set, as
// [models.GroupCreate] says.
//
// Create is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [GroupsService] says. It is sent again only after a 429, which the API did not
// act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have created the
// group, so Create returns that error, and [GroupsService.Get] of the key tells whether the group
// exists.
//
// Create returns an error, and no group:
//
//   - for a scope without an environment: an error that matches [ErrInvalidConfig], and no
//     request for the group; without WithScope, the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrConflict] for a 409, or [ErrNotFound] for a 404, and whose
//     Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a group: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *GroupsService) Create(ctx context.Context, group models.GroupCreate,
) (*models.GroupRead, error) {
	r, err := s.client.inScope(ctx, request{op: opCreateGroup, target: apiTarget, body: group})
	if err != nil {
		return nil, err
	}
	return do[models.GroupRead](ctx, s.client, r)
}

// Delete deletes the group of the instance key groupInstanceKey with
// DELETE /v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}. Delete returns nil when the
// API answers 404, as the group is gone already, such as when an earlier attempt of the call
// deleted it; so a mistyped key returns nil too, and deletes nothing, as does a project or an
// environment that does not exist, when the API answers it with 404.
//
// Delete is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [GroupsService] says. Deleting a group twice leaves it deleted, so the call
// sends its request again after a 500, 502, 503 or 504 reply or a failed connection, as after a
// 429.
//
// Delete returns an error:
//
//   - for a scope without an environment, or a groupInstanceKey that is empty, . or ..: an error
//     that matches [ErrInvalidConfig], and no request for the group; without WithScope, the
//     lookup of the scope comes first;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete the
//     group;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *GroupsService) Delete(ctx context.Context, groupInstanceKey string) error {
	r, err := s.client.inScope(ctx, request{
		op: opDeleteGroup, target: apiTarget, params: []string{groupInstanceKey},
	})
	if err != nil {
		return err
	}
	return s.client.doNoValue(ctx, r)
}

// ListUsers returns one page of the users in the group of the instance key groupInstanceKey, in
// the order of the API's reply, with
// GET /v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}/users. A nil opts, like the zero
// ListOptions, asks for the first page, with the API's default of 30 users on it; the API gives
// at most 100 on a page. The reply is the paginated object, and the page holds its counts: the
// TotalCount of the users in the group and the PageCount, each nil when the reply leaves it out.
// Each user is its ID and its key. [GroupsService.AllUsers] reads every page.
//
// ListUsers is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [GroupsService] says. The call changes nothing, so it sends its request again
// after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// ListUsers returns every failure as an error, and never a page with it:
//
//   - a scope without an environment, or a groupInstanceKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the users; without WithScope, the lookup of
//     the scope comes first;
//   - a Page or a PerPage less than zero: an error that matches ErrInvalidConfig, and no request
//     for the users;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply, which matches [ErrNotFound] when the environment has no
//     such group;
//   - a reply in 200-299 that is not the paginated JSON object of users, such as a JSON array:
//     an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *GroupsService) ListUsers(ctx context.Context, groupInstanceKey string,
	opts *ListOptions,
) (*Page[models.GroupUserReadSchema], error) {
	r, err := s.client.inScope(ctx, request{
		op: opListGroupUsers, target: apiTarget, params: []string{groupInstanceKey},
	})
	if err != nil {
		return nil, err
	}
	return listPage[models.GroupUserReadSchema](ctx, s.client, r, pageShape, listOptions(opts))
}

// AllUsers returns an iterator over the users in the group of the instance key groupInstanceKey,
// for a range loop. It reads the pages in turn, as [GroupsService.ListUsers] reads one, from the
// page of opts.Page on, or the first, each of opts.PerPage users, or else of 100, the most that
// the API gives on a page. It stops after the last page that the replies count, or after the
// first page without users, and never after a page with fewer users than it asked for. It asks
// for the next page only when the loop wants a user past the last of a page, so a loop that stops
// early sends no other request. It reads opts when it is called: a later change to opts does not
// change the iterator. Each loop over the iterator reads the users again, from the first page that
// opts asks for. The pages are requests of their own, so a user assigned to the group or removed
// from it during a loop can move the others from one page to the next.
//
// AllUsers is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as ListUsers is; each loop gets the scope as ListUsers does before its first page.
// Each page is a call of its own, with its own timeout and retries. When the scope or a page
// fails, with one of the errors that ListUsers returns, the iterator yields its error once, with
// the zero GroupUserReadSchema, and stops.
func (s *GroupsService) AllUsers(ctx context.Context, groupInstanceKey string,
	opts *ListOptions,
) iter.Seq2[models.GroupUserReadSchema, error] {
	r := request{op: opListGroupUsers, target: apiTarget, params: []string{groupInstanceKey}}
	return allInScope[models.GroupUserReadSchema](ctx, s.client, r, pageShape,
		listOptions(opts))
}

// AssignUser assigns the user whose key or ID is userKey to the group of the instance key
// groupInstanceKey, in the tenant of membership, with
// PUT /v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}/users/{user_id}, and returns the
// group as the API returns it, with the IDs of its users. The request holds the tenant of
// membership, as [models.GroupAssignUser] says.
//
// AssignUser is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [GroupsService] says. What the API answers to the assignment of a user that the
// group has already is not confirmed, so the call is sent again only after a 429, which the API
// did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have
// assigned the user, so AssignUser returns that error, and [GroupsService.ListUsers] tells
// whether the group holds the user.
//
// AssignUser returns an error, and no group:
//
//   - for a scope without an environment, or a groupInstanceKey or a userKey that is empty, . or
//     ..: an error that matches [ErrInvalidConfig], and no request for the user; without
//     WithScope, the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] for a 404, and whose Detail names each field that
//     the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a group: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *GroupsService) AssignUser(ctx context.Context, groupInstanceKey, userKey string,
	membership models.GroupAssignUser,
) (*models.GroupRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opAssignUserToGroup, target: apiTarget, params: []string{groupInstanceKey, userKey},
		body: membership,
	})
	if err != nil {
		return nil, err
	}
	return do[models.GroupRead](ctx, s.client, r)
}

// RemoveUser removes the user whose key or ID is userKey from the group of the instance key
// groupInstanceKey, in the tenant of membership, with
// DELETE /v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}/users/{user_id}, whose body
// holds the tenant of membership, as [models.GroupAssignUser] says. The body names the tenant of
// the membership, so a 404 can mean a mistyped tenant: RemoveUser returns it as an [Error] that
// matches [ErrNotFound], never nil.
//
// RemoveUser is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [GroupsService] says. Removing a user twice leaves it removed, so the call
// sends its request again after a 500, 502, 503 or 504 reply or a failed connection, as after a
// 429; the second request may then get a 404, which RemoveUser returns.
//
// RemoveUser returns an error:
//
//   - for a scope without an environment, or a groupInstanceKey or a userKey that is empty, . or
//     ..: an error that matches [ErrInvalidConfig], and no request for the user; without
//     WithScope, the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches ErrNotFound for a 404;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *GroupsService) RemoveUser(ctx context.Context, groupInstanceKey, userKey string,
	membership models.GroupAssignUser,
) error {
	r, err := s.client.inScope(ctx, request{
		op: opRemoveUserFromGroup, target: apiTarget,
		params: []string{groupInstanceKey, userKey}, body: membership,
	})
	if err != nil {
		return err
	}
	return s.client.doNoValue(ctx, r)
}
