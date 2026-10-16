package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the actions of a resource of an environment.
const (
	opListResourceActions  = "list_resource_actions"
	opCreateResourceAction = "create_resource_action"
	opGetResourceAction    = "get_resource_action"
	opUpdateResourceAction = "update_resource_action"
	opDeleteResourceAction = "delete_resource_action"
)

// ResourceActionsService lists, reads, creates, changes and deletes the actions of a resource of
// an environment, such as read and write of the resource document; [Client.ResourceActions]
// holds it. Each method takes the key or the ID of the resource first, such as document, and a
// method that acts on one action takes the key or the ID of the action after it, such as read.
// The call sends each as one segment of the path, escaped: a key that holds a / or a % names
// that resource or that action, not another route.
//
// Its calls act on the environment of the client's scope, whose project and environment fill the
// route: those that [WithScope] gives, or else those of the client's API key, which the first call
// that needs them looks up once, as [Client.Scope] says. A scope without an environment, such as
// that of a project-level API key without WithScope, is an error that matches [ErrInvalidConfig],
// and the call sends no request for its operation. Each method is sent with an
// environment-level API key of the environment, or with a key of a wider level and WithScope; the
// API answers a key that may not act on the environment with an error status, which the call
// returns as an [Error], such as one that matches [ErrForbidden].
type ResourceActionsService struct {
	client *Client
}

// ResourceActionsListOptions are the page and the attribute filters of
// [ResourceActionsService.List] and [ResourceActionsService.All].
type ResourceActionsListOptions struct {
	// ListOptions picks the page and the number of actions on it, sent in page and per_page.
	// With a PerPage of 0, List leaves the number to the API, which gives 30, and All asks for
	// 100, the most that the API gives.
	ListOptions

	// Attributes keep the actions whose attributes match each filter, each sent in the query
	// parameter attr_<name>, as [AttributeFilter] says, such as
	// permit.AttributeEquals("risk", "low"). The API's documentation of the attributes of an
	// action says that they can filter the actions with query parameters that start with attr_,
	// and its documentation of the list names no such parameter. Nil or empty keeps every
	// action. A filter without a name, one whose float64 is NaN or an infinity, and one whose
	// name an earlier filter has, are not valid.
	Attributes []AttributeFilter
}

// listRequest returns the request of the actions of the resource whose key or ID is
// resourceKey that opts filters, with a parameter attr_<name> in its query for each of the
// Attributes of opts, and the page of opts. A nil opts is the zero ResourceActionsListOptions.
// The request holds copies of the values of opts, so a later change to opts does not change it.
// Before it reads opts, it returns ErrClosed after Close; then an error that matches
// ErrInvalidConfig for an attribute filter that is not valid, as addAttributeFilters says.
func (s *ResourceActionsService) listRequest(resourceKey string,
	opts *ResourceActionsListOptions,
) (request, ListOptions, error) {
	if err := s.client.checkOpen(); err != nil {
		return request{}, ListOptions{}, err
	}
	var o ResourceActionsListOptions
	if opts != nil {
		o = *opts
	}
	query := &wire.Query{}
	if err := addAttributeFilters(query, opListResourceActions, o.Attributes); err != nil {
		return request{}, ListOptions{}, err
	}
	r := request{
		op: opListResourceActions, target: apiTarget, params: []string{resourceKey}, query: query,
	}
	return r, o.ListOptions, nil
}

// List returns one page of the actions of the resource whose key or ID is resourceKey, in the
// order of the API's reply, with GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/actions.
// A nil opts, like the zero ResourceActionsListOptions, asks for the first page of every action,
// with the API's default of 30 actions on it; the API gives at most 100 on a page. The
// attribute filters of opts are sent in the query, as [ResourceActionsListOptions] says. The
// reply reports no counts, so the TotalCount and the PageCount of the page are nil.
// [ResourceActionsService.All] reads every page.
//
// List is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceActionsService] says. The call changes nothing, so it sends its
// request again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// List returns every failure as an error, and never a page with it:
//
//   - an attribute filter of opts that is not valid: an error that matches [ErrInvalidConfig]
//     and names the filter by its place in the Attributes of opts, from 1, and no request, not
//     even the lookup of the scope;
//   - a scope without an environment, or a resourceKey that is empty, . or ..: an error that
//     matches ErrInvalidConfig, and no request for the list; without WithScope, the lookup of
//     the scope comes first;
//   - a Page or a PerPage less than zero: an error that matches ErrInvalidConfig, and no request
//     for the list;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply;
//   - a reply in 200-299 that is not a JSON array of actions: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceActionsService) List(ctx context.Context, resourceKey string,
	opts *ResourceActionsListOptions,
) (*Page[models.ResourceActionRead], error) {
	r, page, err := s.listRequest(resourceKey, opts)
	if err != nil {
		return nil, err
	}
	r, err = s.client.inScope(ctx, r)
	if err != nil {
		return nil, err
	}
	return listPage[models.ResourceActionRead](ctx, s.client, r, arrayShape, page)
}

// All returns an iterator over the actions of the resource whose key or ID is resourceKey that
// match the attribute filters of opts, for a range loop. It reads the pages in turn, as
// [ResourceActionsService.List] reads one, from the page of opts.Page on, or the first, each of
// opts.PerPage actions, or else of 100, the most that the API gives on a page. It stops after the
// first page without actions, and never after a page with fewer actions than it asked for. It
// asks for the next page only when the loop wants an action past the last of a page, so a loop
// that stops early sends no other request. It reads opts when it is called: a later change to
// opts does not change the iterator. Each loop over the iterator reads the actions again, from
// the first page that opts asks for. The pages are requests of their own, so an action created
// or deleted during a loop can move the others from one page to the next.
//
// All is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as List is; each loop gets the scope as List does before its first page. Each
// page is a call of its own, with its own timeout and retries. When an attribute filter of opts
// is not valid, or the scope or a page fails, with one of the errors that List returns, the
// iterator yields its error once, with the zero ResourceActionRead, and stops.
func (s *ResourceActionsService) All(ctx context.Context, resourceKey string,
	opts *ResourceActionsListOptions,
) iter.Seq2[models.ResourceActionRead, error] {
	r, page, err := s.listRequest(resourceKey, opts)
	if err != nil {
		return func(yield func(models.ResourceActionRead, error) bool) {
			yield(models.ResourceActionRead{}, err)
		}
	}
	return allInScope[models.ResourceActionRead](ctx, s.client, r, arrayShape, page)
}

// Get returns the action whose key or ID is actionKey of the resource whose key or ID is
// resourceKey, with GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/actions/{action_id}.
//
// Get is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceActionsService] says. The call changes nothing, so it sends its
// request again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no action:
//
//   - for a scope without an environment, or a resourceKey or an actionKey that is empty, . or
//     ..: an error that matches [ErrInvalidConfig], and no request for the action; without
//     WithScope, the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the resource has no such action;
//   - for a reply in 200-299 that is not a JSON object of an action: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceActionsService) Get(ctx context.Context, resourceKey, actionKey string,
) (*models.ResourceActionRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opGetResourceAction, target: apiTarget, params: []string{resourceKey, actionKey},
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceActionRead](ctx, s.client, r)
}

// Create creates an action of the resource whose key or ID is resourceKey with
// POST /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/actions, and returns it as the API
// stores it, with its ID. The request holds the key and the name of action, and its other fields
// that are set, as [models.ResourceActionCreate] says.
//
// Create is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceActionsService] says. It is sent again only after a 429, which the
// API did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have
// created the action, so Create returns that error, and [ResourceActionsService.Get] of the key
// tells whether the action exists.
//
// Create returns an error, and no action:
//
//   - for a scope without an environment, or a resourceKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the action; without WithScope, the lookup
//     of the scope comes first;
//   - for an attribute of action that cannot be JSON, such as a value that is a channel: an
//     error that matches ErrInvalidConfig, and no request for the action;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrConflict] when the resource has an action with the key
//     already, or one whose Detail names each field that the API rejects for a 422, such as a
//     key that is not URL-friendly;
//   - for a reply in 200-299 that is not a JSON object of an action: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceActionsService) Create(ctx context.Context, resourceKey string,
	action models.ResourceActionCreate,
) (*models.ResourceActionRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opCreateResourceAction, target: apiTarget, params: []string{resourceKey},
		body: action,
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceActionRead](ctx, s.client, r)
}

// Update changes the action whose key or ID is actionKey of the resource whose key or ID is
// resourceKey, with
// PATCH /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/actions/{action_id}, and returns
// it as the API stores it. The request holds only the fields of update that are set, as
// [models.ResourceActionUpdate] says: null for a Description or Attributes of [models.Null],
// which clears it, and the attributes of [models.Some] as a whole.
//
// Update is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceActionsService] says. Sending the same update twice sets the same
// fields, so the call sends its request again after a 500, 502, 503 or 504 reply or a failed
// connection, as after a 429.
//
// Update returns an error, and no action:
//
//   - for a scope without an environment, or a resourceKey or an actionKey that is empty, . or
//     ..: an error that matches [ErrInvalidConfig], and no request for the action; without
//     WithScope, the lookup of the scope comes first;
//   - for an attribute of update that cannot be JSON, such as a value that is a function: an
//     error that matches ErrInvalidConfig, and no request for the action;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the resource has no such action, or one whose
//     Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of an action: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceActionsService) Update(ctx context.Context, resourceKey, actionKey string,
	update models.ResourceActionUpdate,
) (*models.ResourceActionRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opUpdateResourceAction, target: apiTarget, params: []string{resourceKey, actionKey},
		body: update,
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceActionRead](ctx, s.client, r)
}

// Delete deletes the action whose key or ID is actionKey of the resource whose key or ID is
// resourceKey, with
// DELETE /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/actions/{action_id}. It returns
// nil when the API answers 404, as the action is gone already, such as when an earlier attempt
// of the call deleted it; so a mistyped key of the action or of the resource returns nil too,
// and deletes nothing.
//
// Delete is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceActionsService] says. Deleting an action twice leaves it deleted, so
// the call sends its request again after a 500, 502, 503 or 504 reply or a failed connection, as
// after a 429.
//
// Delete returns an error:
//
//   - for a scope without an environment, or a resourceKey or an actionKey that is empty, . or
//     ..: an error that matches [ErrInvalidConfig], and no request for the action; without
//     WithScope, the lookup of the scope comes first;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete the
//     action;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceActionsService) Delete(ctx context.Context, resourceKey, actionKey string,
) error {
	r, err := s.client.inScope(ctx, request{
		op: opDeleteResourceAction, target: apiTarget, params: []string{resourceKey, actionKey},
	})
	if err != nil {
		return err
	}
	return s.client.doNoValue(ctx, r)
}
