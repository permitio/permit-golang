package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the action groups of a resource.
const (
	opListResourceActionGroups  = "list_resource_action_groups"
	opCreateResourceActionGroup = "create_resource_action_group"
	opGetResourceActionGroup    = "get_resource_action_group"
	opUpdateResourceActionGroup = "update_resource_action_group"
	opDeleteResourceActionGroup = "delete_resource_action_group"
)

// ResourceActionGroupsService lists, reads, creates, changes and deletes the action groups of the
// resources of an environment, each a named set of the actions of its resource, such as editing
// for read and write; [Client.ResourceActionGroups] holds it. Each method takes the key or the ID
// of the resource first, such as document, and a method that acts on one action group takes the
// group's key or its ID next, such as editing. Each key is sent as one segment of the path,
// escaped: a key that holds a / or a % names that resource or that group, not another route.
//
// Its calls act on the environment of the client's scope, whose project and environment fill the
// route: those that [WithScope] gives, or else those of the client's API key, which the first call
// that needs them looks up once, as [Client.Scope] says. A scope without an environment, such as
// that of a project-level API key without WithScope, is an error that matches [ErrInvalidConfig],
// and the call sends no request for its operation. Each method is sent with an
// environment-level API key of the environment, or with a key of a wider level and WithScope; the
// API answers a key that may not act on the environment with an error status, which the call
// returns as an [Error], such as one that matches [ErrForbidden].
type ResourceActionGroupsService struct {
	client *Client
}

// List returns one page of the action groups of the resource whose key or ID is resourceKey, in
// the order of the API's reply, with
// GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/action_groups. A nil opts, like the
// zero ListOptions, asks for the first page, with the API's default of 30 action groups on it;
// the API gives at most 100 on a page. The reply is a JSON array, which reports no counts, so the
// TotalCount and the PageCount of the page are nil. [ResourceActionGroupsService.All] reads every
// page.
//
// List is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceActionGroupsService] says. The call changes nothing, so it sends its
// request again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// List returns every failure as an error, and never a page with it:
//
//   - a scope without an environment, or a resourceKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the list; without WithScope, the lookup of
//     the scope comes first;
//   - a Page or a PerPage less than zero: an error that matches ErrInvalidConfig, and no request
//     for the list;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply, which matches [ErrNotFound] when the environment has no
//     such resource;
//   - a reply in 200-299 that is not a JSON array of action groups: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceActionGroupsService) List(ctx context.Context, resourceKey string,
	opts *ListOptions,
) (*Page[models.ResourceActionGroupRead], error) {
	r, err := s.client.inScope(ctx, request{
		op: opListResourceActionGroups, target: apiTarget, params: []string{resourceKey},
	})
	if err != nil {
		return nil, err
	}
	return listPage[models.ResourceActionGroupRead](ctx, s.client, r, arrayShape,
		listOptions(opts))
}

// All returns an iterator over the action groups of the resource whose key or ID is
// resourceKey, for a range loop. It reads the pages in turn, as [ResourceActionGroupsService.List]
// reads one, from the page of opts.Page on, or the first, each of opts.PerPage action groups, or
// else of 100, the most that the API gives on a page. It stops after the first page without
// action groups, and never after a page with fewer action groups than it asked for. It asks for
// the next page only when the loop wants an action group past the last of a page, so a loop that
// stops early sends no other request. It reads opts when it is called: a later change to opts
// does not change the iterator. Each loop over the iterator reads the action groups again, from
// the first page that opts asks for. The pages are requests of their own, so an action group
// created or deleted during a loop can move the others from one page to the next.
//
// All is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as List is; each loop gets the scope as List does before its first page. Each
// page is a call of its own, with its own timeout and retries. When the scope or a page fails,
// with one of the errors that List returns, the iterator yields its error once, with the zero
// ResourceActionGroupRead, and stops.
func (s *ResourceActionGroupsService) All(ctx context.Context, resourceKey string,
	opts *ListOptions,
) iter.Seq2[models.ResourceActionGroupRead, error] {
	r := request{
		op: opListResourceActionGroups, target: apiTarget, params: []string{resourceKey},
	}
	return allInScope[models.ResourceActionGroupRead](ctx, s.client, r, arrayShape,
		listOptions(opts))
}

// Get returns the action group whose key or ID is actionGroupKey, of the resource whose key or
// ID is resourceKey, with
// GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/action_groups/{action_group_id}.
//
// Get is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceActionGroupsService] says. The call changes nothing, so it sends its
// request again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no action group:
//
//   - for a scope without an environment, or a resourceKey or an actionGroupKey that is empty,
//     . or ..: an error that matches [ErrInvalidConfig], and no request for the action group;
//     without WithScope, the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the environment has no such resource, or the resource no
//     such action group;
//   - for a reply in 200-299 that is not a JSON object of an action group: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceActionGroupsService) Get(ctx context.Context,
	resourceKey, actionGroupKey string,
) (*models.ResourceActionGroupRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opGetResourceActionGroup, target: apiTarget,
		params: []string{resourceKey, actionGroupKey},
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceActionGroupRead](ctx, s.client, r)
}

// Create creates an action group in the resource whose key or ID is resourceKey, with
// POST /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/action_groups, and returns it as the
// API stores it, with its ID. The request holds the key and the name of group, and each other
// field of it that is set, as [models.ResourceActionGroupCreate] says.
//
// Create is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceActionGroupsService] says. It is sent again only after a 429, which
// the API did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may
// have created the action group, so Create returns that error, and
// [ResourceActionGroupsService.Get] of the key tells whether the action group exists.
//
// Create returns an error, and no action group:
//
//   - for a scope without an environment, or a resourceKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the action group; without WithScope, the
//     lookup of the scope comes first;
//   - for a field of group that cannot be JSON, such as an attribute whose value is a channel:
//     an error that matches ErrInvalidConfig, and no request for the action group;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the environment has no such resource and [ErrConflict]
//     when the resource has an action group with the key already, and whose Detail names each
//     field that the API rejects for a 422, such as a key that is not URL-friendly;
//   - for a reply in 200-299 that is not a JSON object of an action group: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceActionGroupsService) Create(ctx context.Context, resourceKey string,
	group models.ResourceActionGroupCreate,
) (*models.ResourceActionGroupRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opCreateResourceActionGroup, target: apiTarget, params: []string{resourceKey},
		body: group,
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceActionGroupRead](ctx, s.client, r)
}

// Update changes the action group whose key or ID is actionGroupKey, of the resource whose key
// or ID is resourceKey, with
// PATCH /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/action_groups/{action_group_id},
// and returns it as the API stores it. The request holds only the fields of update that are set,
// as [models.ResourceActionGroupUpdate] says: an update whose Actions are unset sends no actions.
//
// Set the Actions of update to the keys of the actions that the group keeps, also when the update
// changes only another field, such as its name: the API has been seen to remove every action of
// a group when an update leaves its actions out.
//
// Update is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceActionGroupsService] says. Sending the same update twice sets the
// same fields, so the call sends its request again after a 500, 502, 503 or 504 reply or a
// failed connection, as after a 429.
//
// Update returns an error, and no action group:
//
//   - for a scope without an environment, or a resourceKey or an actionGroupKey that is empty,
//     . or ..: an error that matches [ErrInvalidConfig], and no request for the action group;
//     without WithScope, the lookup of the scope comes first;
//   - for a field of update that cannot be JSON, such as an attribute whose value is a function,
//     or Actions that are [models.Some] of a nil slice: an error that matches ErrInvalidConfig,
//     and no request for the action group;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the environment has no such resource, or the resource no
//     such action group, and whose Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of an action group: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceActionGroupsService) Update(ctx context.Context,
	resourceKey, actionGroupKey string, update models.ResourceActionGroupUpdate,
) (*models.ResourceActionGroupRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opUpdateResourceActionGroup, target: apiTarget,
		params: []string{resourceKey, actionGroupKey}, body: update,
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceActionGroupRead](ctx, s.client, r)
}

// Delete deletes the action group whose key or ID is actionGroupKey, of the resource whose key
// or ID is resourceKey, with
// DELETE /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/action_groups/{action_group_id}.
// The route names the action group, never an action of the resource, also when an action has
// the group's key. Delete returns nil when the API answers 404, as the action group is gone
// already, such as when an earlier attempt of the call deleted it; so a mistyped key returns nil
// too, and deletes nothing, as does a resource, a project or an environment that does not exist,
// when the API answers it with 404.
//
// Delete is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceActionGroupsService] says. Deleting an action group twice leaves it
// deleted, so the call sends its request again after a 500, 502, 503 or 504 reply or a failed
// connection, as after a 429.
//
// Delete returns an error:
//
//   - for a scope without an environment, or a resourceKey or an actionGroupKey that is empty,
//     . or ..: an error that matches [ErrInvalidConfig], and no request for the action group;
//     without WithScope, the lookup of the scope comes first;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete the
//     action group;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceActionGroupsService) Delete(ctx context.Context,
	resourceKey, actionGroupKey string,
) error {
	r, err := s.client.inScope(ctx, request{
		op: opDeleteResourceActionGroup, target: apiTarget,
		params: []string{resourceKey, actionGroupKey},
	})
	if err != nil {
		return err
	}
	return s.client.doNoValue(ctx, r)
}
