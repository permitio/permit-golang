package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the roles of a group and of the groups assigned to a group.
const (
	opAssignRoleToGroup    = "assign_role_to_group" //nolint:gosec // G101: an operation ID
	opRemoveRoleFromGroup  = "remove_role_from_group"
	opListGroupRoles       = "list_group_roles"
	opAssignGroupToGroup   = "assign_group_to_group"
	opRemoveGroupFromGroup = "remove_group_from_group"
	opListGroupChildren    = "list_group_children"
	opListGroupParents     = "list_group_parents"
)

// AssignRole assigns a role on a resource instance to the group of the instance key
// groupInstanceKey, with POST /v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}/roles,
// and returns the group as the API returns it, with the roles assigned to it. The request holds
// the role, the resource, the resource instance and the tenant of role, each of them, as
// [models.GroupAddRole] says. The API's documentation says of the route: "Assign a role to a
// group - It will create relation between the group and the resource, relationship between the
// resource instances and derivation from the member role to this role." So the role applies to
// the members of the group through a derivation of the environment's ReBAC model, from the
// group's member role; what a decision of the PDP then allows a member is not confirmed yet.
//
// AssignRole is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [GroupsService] says. What the API answers to the assignment of a role that the
// group has already is not confirmed, so the call is sent again only after a 429, which the API
// did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have
// assigned the role, so AssignRole returns that error, and [GroupsService.ListRoles] tells
// whether the group holds the role.
//
// AssignRole returns an error, and no group:
//
//   - for a scope without an environment, or a groupInstanceKey that is empty, . or ..: an error
//     that matches [ErrInvalidConfig], and no request for the role; without WithScope, the
//     lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] for a 404, and whose Detail names each field that
//     the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a group: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *GroupsService) AssignRole(ctx context.Context, groupInstanceKey string,
	role models.GroupAddRole,
) (*models.GroupRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opAssignRoleToGroup, target: apiTarget, params: []string{groupInstanceKey}, body: role,
	})
	if err != nil {
		return nil, err
	}
	return do[models.GroupRead](ctx, s.client, r)
}

// RemoveRole removes a role on a resource instance from the group of the instance key
// groupInstanceKey, with DELETE /v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}/roles,
// whose body holds the role, the resource, the resource instance and the tenant of role, as
// [models.GroupAddRole] says. The API's documentation says of the route: "Remove a role from a
// group- It will remove the derivation from the member role to this role." The body names the
// role that the call removes, so a 404 can mean a mistyped key: RemoveRole returns it as an
// [Error] that matches [ErrNotFound], never nil.
//
// RemoveRole is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [GroupsService] says. Removing a role twice leaves it removed, so the call sends
// its request again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429;
// the second request may then get a 404, which RemoveRole returns.
//
// RemoveRole returns an error:
//
//   - for a scope without an environment, or a groupInstanceKey that is empty, . or ..: an error
//     that matches [ErrInvalidConfig], and no request for the role; without WithScope, the
//     lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches ErrNotFound for a 404;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *GroupsService) RemoveRole(ctx context.Context, groupInstanceKey string,
	role models.GroupAddRole,
) error {
	r, err := s.client.inScope(ctx, request{
		op: opRemoveRoleFromGroup, target: apiTarget, params: []string{groupInstanceKey},
		body: role,
	})
	if err != nil {
		return err
	}
	return s.client.doNoValue(ctx, r)
}

// ListRoles returns one page of the roles assigned to the group of the instance key
// groupInstanceKey, in the order of the API's reply, with
// GET /v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}/roles. A nil opts, like the zero
// ListOptions, asks for the first page, with the API's default of 30 roles on it; the API gives
// at most 100 on a page. The reply is the paginated object, and the page holds its counts: the
// TotalCount of the roles of the group and the PageCount, each nil when the reply leaves it out.
// Each role is its key, and the ID and the key of its resource and of its resource instance.
// [GroupsService.AllRoles] reads every page.
//
// ListRoles is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [GroupsService] says. The call changes nothing, so it sends its request again
// after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// ListRoles returns every failure as an error, and never a page with it:
//
//   - a scope without an environment, or a groupInstanceKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the roles; without WithScope, the lookup of
//     the scope comes first;
//   - a Page or a PerPage less than zero: an error that matches ErrInvalidConfig, and no request
//     for the roles;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply, such as one that matches [ErrNotFound] for a 404;
//   - a reply in 200-299 that is not the paginated JSON object of roles, such as a JSON array:
//     an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *GroupsService) ListRoles(ctx context.Context, groupInstanceKey string,
	opts *ListOptions,
) (*Page[models.GroupRoleReadSchema], error) {
	r, err := s.client.inScope(ctx, request{
		op: opListGroupRoles, target: apiTarget, params: []string{groupInstanceKey},
	})
	if err != nil {
		return nil, err
	}
	return listPage[models.GroupRoleReadSchema](ctx, s.client, r, pageShape, listOptions(opts))
}

// AllRoles returns an iterator over the roles assigned to the group of the instance key
// groupInstanceKey, for a range loop. It reads the pages in turn, as [GroupsService.ListRoles]
// reads one, from the page of opts.Page on, or the first, each of opts.PerPage roles, or else of
// 100, the most that the API gives on a page. It stops after the first page without roles, or, when
// the loop started at the first page, after the page that the total_count of its reply shows to be
// the last, and never because a page holds fewer roles than it asked for. It asks for the next page
// only when the loop wants a role past the last of a page, so a loop that stops early sends no
// other request. It reads opts when it is called: a later change to opts does not change the
// iterator. Each loop over the iterator reads the roles again, from the first page that opts asks
// for. The pages are requests of their own, so a role assigned to the group or removed from it
// during a loop can move the others from one page to the next.
//
// AllRoles is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as ListRoles is; each loop gets the scope as ListRoles does before its first page.
// Each page is a call of its own, with its own timeout and retries. When the scope or a page
// fails, with one of the errors that ListRoles returns, the iterator yields its error once, with
// the zero GroupRoleReadSchema, and stops.
func (s *GroupsService) AllRoles(ctx context.Context, groupInstanceKey string,
	opts *ListOptions,
) iter.Seq2[models.GroupRoleReadSchema, error] {
	r := request{op: opListGroupRoles, target: apiTarget, params: []string{groupInstanceKey}}
	return allInScope[models.GroupRoleReadSchema](ctx, s.client, r, pageShape,
		listOptions(opts))
}

// AssignGroup assigns the group that assignment names to the group of the instance key
// groupInstanceKey, with
// PUT /v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}/assign_group, and returns a group
// as the API returns it; the spec does not say which of the two groups the reply describes, and
// that is not confirmed yet. The request holds the GroupInstanceKey of assignment, as it is
// given, as [models.GroupAssignment] says: the API's documentation says that it takes the ID of
// the group's resource instance, or <resource_key>:<resource_instance_key>, such as
// group:engineering. The API's documentation says of the route: "Assign a group to a group -
// This group will inherit the group's roles.", as it says "This user will inherit the group's
// roles." of the route of [GroupsService.AssignUser]. So the group of assignment is to inherit
// the roles of the group of groupInstanceKey, as one of its children, which
// [GroupsService.ListChildren] lists; which way the API nests the two groups is not confirmed
// yet.
//
// AssignGroup is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [GroupsService] says. What the API answers to the assignment of a group that
// the group has already is not confirmed, so the call is sent again only after a 429, which the
// API did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have
// assigned the group, so AssignGroup returns that error, and [GroupsService.ListChildren] tells
// whether the group holds the other.
//
// AssignGroup returns an error, and no group:
//
//   - for a scope without an environment, or a groupInstanceKey that is empty, . or ..: an error
//     that matches [ErrInvalidConfig], and no request for the group; without WithScope, the
//     lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] for a 404, and whose Detail names each field that
//     the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a group: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *GroupsService) AssignGroup(ctx context.Context, groupInstanceKey string,
	assignment models.GroupAssignment,
) (*models.GroupRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opAssignGroupToGroup, target: apiTarget, params: []string{groupInstanceKey},
		body: assignment,
	})
	if err != nil {
		return nil, err
	}
	return do[models.GroupRead](ctx, s.client, r)
}

// RemoveGroup removes the group that assignment names from the group of the instance key
// groupInstanceKey, with
// DELETE /v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}/assign_group, whose body holds
// the GroupInstanceKey of assignment, as [GroupsService.AssignGroup] sends it. The API's
// documentation says of the route: "Remove a group from a group- This group will be removed from
// the member role of the group, and will no longer inherit the group's roles." The body names the
// group that the call removes, so a 404 can mean a mistyped key: RemoveGroup returns it as an
// [Error] that matches [ErrNotFound], never nil.
//
// RemoveGroup is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [GroupsService] says. Removing a group twice leaves it removed, so the call
// sends its request again after a 500, 502, 503 or 504 reply or a failed connection, as after a
// 429; the second request may then get a 404, which RemoveGroup returns.
//
// RemoveGroup returns an error:
//
//   - for a scope without an environment, or a groupInstanceKey that is empty, . or ..: an error
//     that matches [ErrInvalidConfig], and no request for the group; without WithScope, the
//     lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches ErrNotFound for a 404;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *GroupsService) RemoveGroup(ctx context.Context, groupInstanceKey string,
	assignment models.GroupAssignment,
) error {
	r, err := s.client.inScope(ctx, request{
		op: opRemoveGroupFromGroup, target: apiTarget, params: []string{groupInstanceKey},
		body: assignment,
	})
	if err != nil {
		return err
	}
	return s.client.doNoValue(ctx, r)
}

// ListChildren returns one page of the child groups of the group of the instance key
// groupInstanceKey, in the order of the API's reply, with
// GET /v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}/children, which the API's
// documentation describes as "List group children of a specific group.". A nil opts, like the
// zero ListOptions, asks for the first page, with the API's default of 30 groups on it; the API
// gives at most 100 on a page. The reply is the paginated object, and the page holds its counts:
// the TotalCount of the children and the PageCount, each nil when the reply leaves it out.
// [GroupsService.AllChildren] reads every page.
//
// ListChildren is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [GroupsService] says. The call changes nothing, so it sends its request again
// after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// ListChildren returns every failure as an error, and never a page with it:
//
//   - a scope without an environment, or a groupInstanceKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the groups; without WithScope, the lookup of
//     the scope comes first;
//   - a Page or a PerPage less than zero: an error that matches ErrInvalidConfig, and no request
//     for the groups;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply, such as one that matches [ErrNotFound] for a 404;
//   - a reply in 200-299 that is not the paginated JSON object of groups, such as a JSON array:
//     an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *GroupsService) ListChildren(ctx context.Context, groupInstanceKey string,
	opts *ListOptions,
) (*Page[models.GroupReadSchema], error) {
	r, err := s.client.inScope(ctx, request{
		op: opListGroupChildren, target: apiTarget, params: []string{groupInstanceKey},
	})
	if err != nil {
		return nil, err
	}
	return listPage[models.GroupReadSchema](ctx, s.client, r, pageShape, listOptions(opts))
}

// AllChildren returns an iterator over the child groups of the group of the instance key
// groupInstanceKey, for a range loop. It reads the pages in turn, as [GroupsService.ListChildren]
// reads one, from the page of opts.Page on, or the first, each of opts.PerPage groups, or else of
// 100, the most that the API gives on a page. It stops after the first page without groups, or,
// when the loop started at the first page, after the page that the total_count of its reply shows
// to be the last, and never because a page holds fewer groups than it asked for. It asks for the
// next page only when the loop wants a group past the last of a page, so a loop that stops early
// sends no other request. It reads opts when it is called: a later change to opts does not change
// the iterator. Each loop over the iterator reads the groups again, from the first page that opts
// asks for. The pages are requests of their own, so a group assigned or removed during a loop can
// move the others from one page to the next.
//
// AllChildren is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as ListChildren is; each loop gets the scope as ListChildren does before its first
// page. Each page is a call of its own, with its own timeout and retries. When the scope or a page
// fails, with one of the errors that ListChildren returns, the iterator yields its error once,
// with the zero GroupReadSchema, and stops.
func (s *GroupsService) AllChildren(ctx context.Context, groupInstanceKey string,
	opts *ListOptions,
) iter.Seq2[models.GroupReadSchema, error] {
	r := request{op: opListGroupChildren, target: apiTarget, params: []string{groupInstanceKey}}
	return allInScope[models.GroupReadSchema](ctx, s.client, r, pageShape, listOptions(opts))
}

// ListParents returns one page of the parent groups of the group of the instance key
// groupInstanceKey, in the order of the API's reply, with
// GET /v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}/parents, which the API's
// documentation describes as "List group parents of a specific group.". A nil opts, like the
// zero ListOptions, asks for the first page, with the API's default of 30 groups on it; the API
// gives at most 100 on a page. The reply is the paginated object, and the page holds its counts:
// the TotalCount of the parents and the PageCount, each nil when the reply leaves it out.
// [GroupsService.AllParents] reads every page.
//
// ListParents is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [GroupsService] says. The call changes nothing, so it sends its request again
// after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// ListParents returns every failure as an error, and never a page with it:
//
//   - a scope without an environment, or a groupInstanceKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the groups; without WithScope, the lookup of
//     the scope comes first;
//   - a Page or a PerPage less than zero: an error that matches ErrInvalidConfig, and no request
//     for the groups;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply, such as one that matches [ErrNotFound] for a 404;
//   - a reply in 200-299 that is not the paginated JSON object of groups, such as a JSON array:
//     an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *GroupsService) ListParents(ctx context.Context, groupInstanceKey string,
	opts *ListOptions,
) (*Page[models.GroupReadSchema], error) {
	r, err := s.client.inScope(ctx, request{
		op: opListGroupParents, target: apiTarget, params: []string{groupInstanceKey},
	})
	if err != nil {
		return nil, err
	}
	return listPage[models.GroupReadSchema](ctx, s.client, r, pageShape, listOptions(opts))
}

// AllParents returns an iterator over the parent groups of the group of the instance key
// groupInstanceKey, for a range loop. It reads the pages in turn, as [GroupsService.ListParents]
// reads one, from the page of opts.Page on, or the first, each of opts.PerPage groups, or else of
// 100, the most that the API gives on a page. It stops after the first page without groups, or,
// when the loop started at the first page, after the page that the total_count of its reply shows
// to be the last, and never because a page holds fewer groups than it asked for. It asks for the
// next page only when the loop wants a group past the last of a page, so a loop that stops early
// sends no other request. It reads opts when it is called: a later change to opts does not change
// the iterator. Each loop over the iterator reads the groups again, from the first page that opts
// asks for. The pages are requests of their own, so a group assigned or removed during a loop can
// move the others from one page to the next.
//
// AllParents is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as ListParents is; each loop gets the scope as ListParents does before its first
// page. Each page is a call of its own, with its own timeout and retries. When the scope or a page
// fails, with one of the errors that ListParents returns, the iterator yields its error once, with
// the zero GroupReadSchema, and stops.
func (s *GroupsService) AllParents(ctx context.Context, groupInstanceKey string,
	opts *ListOptions,
) iter.Seq2[models.GroupReadSchema, error] {
	r := request{op: opListGroupParents, target: apiTarget, params: []string{groupInstanceKey}}
	return allInScope[models.GroupReadSchema](ctx, s.client, r, pageShape, listOptions(opts))
}
