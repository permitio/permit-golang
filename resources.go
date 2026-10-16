package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the resources of an environment.
const (
	opListResources   = "list_resources"
	opCreateResource  = "create_resource"
	opGetResource     = "get_resource"
	opReplaceResource = "replace_resource"
	opUpdateResource  = "update_resource"
	opDeleteResource  = "delete_resource"
)

// ResourcesService lists, reads, creates, replaces, changes and deletes the resources of an
// environment, the types of objects that permissions protect, such as document;
// [Client.Resources] holds it. A method that acts on one resource takes its key or its ID, such
// as document, and sends it as one segment of the path, escaped: a key that holds a / or a %
// names that resource, not another route.
//
// Its calls act on the environment of the client's scope, whose project and environment fill the
// route: those that [WithScope] gives, or else those of the client's API key, which the first call
// that needs them looks up once, as [Client.Scope] says. A scope without an environment, such as
// that of a project-level API key without WithScope, is an error that matches [ErrInvalidConfig],
// and the call sends no request for its operation. Each method is sent with an
// environment-level API key of the environment, or with a key of a wider level and WithScope; the
// API answers a key that may not act on the environment with an error status, which the call
// returns as an [Error], such as one that matches [ErrForbidden].
type ResourcesService struct {
	client *Client
}

// ResourcesListOptions are the page and the filter of [ResourcesService.List] and
// [ResourcesService.All].
type ResourcesListOptions struct {
	// ListOptions picks the page and the number of resources on it, sent in page and per_page.
	// With a PerPage of 0, List leaves the number to the API, which gives 30, and All asks for
	// 100, the most that the API gives.
	ListOptions

	// Search is sent as it is given in the query parameter search, which the API's
	// documentation describes as a text search of the name or the key of a resource. Empty
	// leaves it out.
	Search string
}

// resourcesRequest returns the request of the resources that opts filters, with search in its
// query when opts has a Search, and the page of opts. A nil opts is the zero
// ResourcesListOptions. The request holds copies of the values of opts, so a later change to opts
// does not change it.
func resourcesRequest(opts *ResourcesListOptions) (request, ListOptions) {
	var o ResourcesListOptions
	if opts != nil {
		o = *opts
	}
	query := &wire.Query{}
	if o.Search != "" {
		query.AddString("search", &o.Search)
	}
	return request{op: opListResources, target: apiTarget, query: query}, o.ListOptions
}

// List returns one page of the resources of the environment, in the order of the API's reply,
// with GET /v2/schema/{proj_id}/{env_id}/resources. A nil opts, like the zero
// ResourcesListOptions, asks for the first page of every resource, with the API's default of 30
// resources on it; the API gives at most 100 on a page. A Search of opts is sent in the query, as
// [ResourcesListOptions] says. The page holds the counts that the reply reports, and none when
// the reply is a JSON array. [ResourcesService.All] reads every page.
//
// List is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourcesService] says. The call changes nothing, so it sends its request
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
//   - a reply in 200-299 that is neither a JSON array of resources nor the paginated JSON object
//     of them: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourcesService) List(ctx context.Context, opts *ResourcesListOptions,
) (*Page[models.ResourceRead], error) {
	r, page := resourcesRequest(opts)
	r, err := s.client.inScope(ctx, r)
	if err != nil {
		return nil, err
	}
	return listPage[models.ResourceRead](ctx, s.client, r, arrayOrPageShape, page)
}

// All returns an iterator over the resources of the environment that match the Search of opts,
// for a range loop. It reads the pages in turn, as [ResourcesService.List] reads one, from the
// page of opts.Page on, or the first, each of opts.PerPage resources, or else of 100, the most
// that the API gives on a page. It stops after the first page without resources, or, when the loop
// started at the first page, after the page that the total_count of its reply shows to be the last,
// and never because a page holds fewer resources than it asked for. It asks for the next page only
// when the loop wants a resource past the last of a page, so a loop that stops early sends no other
// request. It reads opts when it is called: a later change to opts does not change the iterator.
// Each loop over the iterator reads the resources again, from the first page that opts asks for.
// The pages are requests of their own, so a resource created or deleted during a loop can move the
// others from one page to the next.
//
// All is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as List is; each loop gets the scope as List does before its first page. Each
// page is a call of its own, with its own timeout and retries. When the scope or a page fails,
// with one of the errors that List returns, the iterator yields its error once, with the zero
// ResourceRead, and stops.
func (s *ResourcesService) All(ctx context.Context, opts *ResourcesListOptions,
) iter.Seq2[models.ResourceRead, error] {
	r, page := resourcesRequest(opts)
	return allInScope[models.ResourceRead](ctx, s.client, r, arrayOrPageShape, page)
}

// Get returns the resource whose key or ID is resourceKey, with
// GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}.
//
// Get is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourcesService] says. The call changes nothing, so it sends its request
// again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no resource:
//
//   - for a scope without an environment, or a resourceKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the resource; without WithScope, the
//     lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the environment has no such resource;
//   - for a reply in 200-299 that is not a JSON object of a resource: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourcesService) Get(ctx context.Context, resourceKey string,
) (*models.ResourceRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opGetResource, target: apiTarget, params: []string{resourceKey},
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceRead](ctx, s.client, r)
}

// Create creates a resource in the environment with
// POST /v2/schema/{proj_id}/{env_id}/resources, and returns it as the API stores it, with its
// ID. The request holds the fields of resource that are set, as [models.ResourceCreate] says,
// and always its actions: give a resource without actions an empty map, which sends {}, as a nil
// map sends null, which the API's documentation does not allow for the required actions.
//
// Create is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourcesService] says. It is sent again only after a 429, which the API did
// not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have
// created the resource, so Create returns that error, and [ResourcesService.Get] of the key
// tells whether the resource exists.
//
// Create returns an error, and no resource:
//
//   - for a scope without an environment: an error that matches [ErrInvalidConfig], and no
//     request for the resource; without WithScope, the lookup of the scope comes first;
//   - for a field of resource that cannot be JSON, such as a type attribute whose value is a
//     channel: an error that matches ErrInvalidConfig, and no request for the resource;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrConflict] when the environment has a resource with the key already, and
//     whose Detail names each field that the API rejects for a 422, such as a key that is not
//     URL-friendly;
//   - for a reply in 200-299 that is not a JSON object of a resource: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourcesService) Create(ctx context.Context, resource models.ResourceCreate,
) (*models.ResourceRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opCreateResource, target: apiTarget, body: resource,
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceRead](ctx, s.client, r)
}

// Replace sends the whole definition of the resource whose key or ID is resourceKey, with
// PUT /v2/schema/{proj_id}/{env_id}/resources/{resource_id}, and returns the resource as the API
// stores it. The request holds every field of resource that is set, as [models.ResourceReplace]
// says, and always its name and its actions. The API's documentation says that the call
// "Replaces the resource definition, or creates it if it doesn't exist", and that "If some
// actions are removed or renamed from the resource definition, role and permissions assignments
// for these actions will be revoked": the actions of resource are the whole set of actions of
// the resource. Give a resource without actions an empty map, which sends {}, as a nil map sends
// null, which the API's documentation does not allow for the required actions.
//
// Replace is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourcesService] says. Sending the same definition twice leaves the resource
// with it, so the call sends its request again after a 500, 502, 503 or 504 reply or a failed
// connection, as after a 429.
//
// Replace returns an error, and no resource:
//
//   - for a scope without an environment, or a resourceKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the resource; without WithScope, the
//     lookup of the scope comes first;
//   - for a field of resource that cannot be JSON: an error that matches ErrInvalidConfig, and
//     no request for the resource;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     whose Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a resource: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourcesService) Replace(ctx context.Context, resourceKey string,
	resource models.ResourceReplace,
) (*models.ResourceRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opReplaceResource, target: apiTarget, params: []string{resourceKey}, body: resource,
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceRead](ctx, s.client, r)
}

// Update changes the resource whose key or ID is resourceKey with
// PATCH /v2/schema/{proj_id}/{env_id}/resources/{resource_id}, and returns it as the API stores
// it. The request holds only the fields of update that are set, as [models.ResourceUpdate] says:
// each map, such as the actions, as it is given, as a whole; nil leaves the map out, an empty map
// that is not nil sends {}, and the SDK never sends null for a map. The API's documentation says
// that "Fields that will be provided will be completely overwritten".
//
// Update is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourcesService] says. Sending the same update twice sets the same fields,
// so the call sends its request again after a 500, 502, 503 or 504 reply or a failed
// connection, as after a 429.
//
// Update returns an error, and no resource:
//
//   - for a scope without an environment, or a resourceKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the resource; without WithScope, the
//     lookup of the scope comes first;
//   - for a field of update that cannot be JSON, such as an attribute of a role whose value is
//     a function: an error that matches ErrInvalidConfig, and no request for the resource;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the environment has no such resource, and whose Detail
//     names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a resource: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourcesService) Update(ctx context.Context, resourceKey string,
	update models.ResourceUpdate,
) (*models.ResourceRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opUpdateResource, target: apiTarget, params: []string{resourceKey}, body: update,
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceRead](ctx, s.client, r)
}

// Delete deletes the resource whose key or ID is resourceKey, with all its data, with
// DELETE /v2/schema/{proj_id}/{env_id}/resources/{resource_id}. It returns nil when the API
// answers 404, as the resource is gone already, such as when an earlier attempt of the call
// deleted it; so a mistyped key returns nil too, and deletes nothing, and so does a scope whose
// project or environment does not exist, when the API answers it with 404.
//
// Delete is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourcesService] says. Deleting a resource twice leaves it deleted, so the
// call sends its request again after a 500, 502, 503 or 504 reply or a failed connection, as
// after a 429.
//
// Delete returns an error:
//
//   - for a scope without an environment, or a resourceKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the resource; without WithScope, the
//     lookup of the scope comes first;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete the
//     resource;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourcesService) Delete(ctx context.Context, resourceKey string) error {
	r, err := s.client.inScope(ctx, request{
		op: opDeleteResource, target: apiTarget, params: []string{resourceKey},
	})
	if err != nil {
		return err
	}
	return s.client.doNoValue(ctx, r)
}
