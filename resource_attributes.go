package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the attributes of a resource of an environment.
const (
	opListResourceAttributes  = "list_resource_attributes"
	opCreateResourceAttribute = "create_resource_attribute"
	opGetResourceAttribute    = "get_resource_attribute"
	opUpdateResourceAttribute = "update_resource_attribute"
	opDeleteResourceAttribute = "delete_resource_attribute"
)

// ResourceAttributesService lists, reads, creates, changes and deletes the attributes of a
// resource of an environment, the attributes that each instance of the resource has, such as
// owner of the resource document; [Client.ResourceAttributes] holds it. Each method takes the key
// or the ID of the resource first, such as document, and a method that acts on one attribute
// takes the key or the ID of the attribute after it, such as owner. The call sends each as one
// segment of the path, escaped: a key that holds a / or a % names that resource or that
// attribute, not another route.
//
// An attribute's type is a [models.AttributeType], an open enum: a reply that holds a type that
// this version of the SDK does not know, such as one that the API adds later, is read rather than
// rejected, and the type's Known method reports false for it.
//
// Its calls act on the environment of the client's scope, whose project and environment fill the
// route: those that [WithScope] gives, or else those of the client's API key, which the first call
// that needs them looks up once, as [Client.Scope] says. A scope without an environment, such as
// that of a project-level API key without WithScope, is an error that matches [ErrInvalidConfig],
// and the call sends no request for its operation. Each method is sent with an
// environment-level API key of the environment, or with a key of a wider level and WithScope; the
// API answers a key that may not act on the environment with an error status, which the call
// returns as an [Error], such as one that matches [ErrForbidden].
type ResourceAttributesService struct {
	client *Client
}

// List returns one page of the attributes of the resource whose key or ID is resourceKey, in the
// order of the API's reply, with
// GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/attributes. A nil opts, like the zero
// ListOptions, asks for the first page, with the API's default of 30 attributes on it; the API
// gives at most 100 on a page. Each attribute's type is read as the reply gives it, as
// [ResourceAttributesService] says. The reply reports no counts, so the TotalCount and the
// PageCount of the page are nil. [ResourceAttributesService.All] reads every page.
//
// List is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceAttributesService] says. The call changes nothing, so it sends its
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
//     status and the body of the reply;
//   - a reply in 200-299 that is not a JSON array of attributes: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceAttributesService) List(ctx context.Context, resourceKey string,
	opts *ListOptions,
) (*Page[models.ResourceAttributeRead], error) {
	r, err := s.client.inScope(ctx, request{
		op: opListResourceAttributes, target: apiTarget, params: []string{resourceKey},
	})
	if err != nil {
		return nil, err
	}
	return listPage[models.ResourceAttributeRead](ctx, s.client, r, arrayShape,
		listOptions(opts))
}

// All returns an iterator over the attributes of the resource whose key or ID is resourceKey, for
// a range loop. It reads the pages in turn, as [ResourceAttributesService.List] reads one, from
// the page of opts.Page on, or the first, each of opts.PerPage attributes, or else of 100, the
// most that the API gives on a page. It stops after the first page without attributes, and never
// after a page with fewer attributes than it asked for. It asks for the next page only when the
// loop wants an attribute past the last of a page, so a loop that stops early sends no other
// request. It reads opts when it is called: a later change to opts does not change the iterator.
// Each loop over the iterator reads the attributes again, from the first page that opts asks for.
// The pages are requests of their own, so an attribute created or deleted during a loop can move
// the others from one page to the next.
//
// All is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as List is; each loop gets the scope as List does before its first page. Each
// page is a call of its own, with its own timeout and retries. When the scope or a page fails,
// with one of the errors that List returns, the iterator yields its error once, with the zero
// ResourceAttributeRead, and stops.
func (s *ResourceAttributesService) All(ctx context.Context, resourceKey string,
	opts *ListOptions,
) iter.Seq2[models.ResourceAttributeRead, error] {
	r := request{op: opListResourceAttributes, target: apiTarget, params: []string{resourceKey}}
	return allInScope[models.ResourceAttributeRead](ctx, s.client, r, arrayShape,
		listOptions(opts))
}

// Get returns the attribute whose key or ID is attributeKey of the resource whose key or ID is
// resourceKey, with
// GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/attributes/{attribute_id}. Its type
// is read as the reply gives it, as [ResourceAttributesService] says.
//
// Get is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceAttributesService] says. The call changes nothing, so it sends its
// request again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no attribute:
//
//   - for a scope without an environment, or a resourceKey or an attributeKey that is empty, .
//     or ..: an error that matches [ErrInvalidConfig], and no request for the attribute; without
//     WithScope, the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the resource has no such attribute;
//   - for a reply in 200-299 that is not a JSON object of an attribute: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceAttributesService) Get(ctx context.Context, resourceKey, attributeKey string,
) (*models.ResourceAttributeRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opGetResourceAttribute, target: apiTarget, params: []string{resourceKey, attributeKey},
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceAttributeRead](ctx, s.client, r)
}

// Create creates an attribute of the resource whose key or ID is resourceKey with
// POST /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/attributes, and returns it as the
// API stores it, with its ID. The request holds the key and the type of attribute, and its
// description when it is set, as [models.ResourceAttributeCreate] says.
//
// Create is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceAttributesService] says. It is sent again only after a 429, which the
// API did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have
// created the attribute, so Create returns that error, and [ResourceAttributesService.Get] of the
// key tells whether the attribute exists.
//
// Create returns an error, and no attribute:
//
//   - for a scope without an environment, or a resourceKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the attribute; without WithScope, the lookup
//     of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrConflict] when the resource has an attribute with the key
//     already, or one whose Detail names each field that the API rejects for a 422, such as a
//     key that is not URL-friendly or a type that the API does not know;
//   - for a reply in 200-299 that is not a JSON object of an attribute: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceAttributesService) Create(ctx context.Context, resourceKey string,
	attribute models.ResourceAttributeCreate,
) (*models.ResourceAttributeRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opCreateResourceAttribute, target: apiTarget, params: []string{resourceKey},
		body: attribute,
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceAttributeRead](ctx, s.client, r)
}

// Update changes the attribute whose key or ID is attributeKey of the resource whose key or ID is
// resourceKey, with
// PATCH /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/attributes/{attribute_id}, and
// returns it as the API stores it. The request holds only the fields of update that are set, as
// [models.ResourceAttributeUpdate] says: null for a Description of [models.Null], which clears
// it.
//
// Update is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceAttributesService] says. Sending the same update twice sets the same
// fields, so the call sends its request again after a 500, 502, 503 or 504 reply or a failed
// connection, as after a 429.
//
// Update returns an error, and no attribute:
//
//   - for a scope without an environment, or a resourceKey or an attributeKey that is empty, .
//     or ..: an error that matches [ErrInvalidConfig], and no request for the attribute; without
//     WithScope, the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the resource has no such attribute, or one
//     whose Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of an attribute: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceAttributesService) Update(ctx context.Context, resourceKey, attributeKey string,
	update models.ResourceAttributeUpdate,
) (*models.ResourceAttributeRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opUpdateResourceAttribute, target: apiTarget,
		params: []string{resourceKey, attributeKey}, body: update,
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceAttributeRead](ctx, s.client, r)
}

// Delete deletes the attribute whose key or ID is attributeKey of the resource whose key or ID
// is resourceKey, with
// DELETE /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/attributes/{attribute_id}. It
// sends no query, though the API's documentation lists page and per_page for the route. It
// returns nil when the API answers 404, as the attribute is gone already, such as when an earlier
// attempt of the call deleted it; so a mistyped key of the attribute or of the resource returns
// nil too, and deletes nothing.
//
// Delete is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceAttributesService] says. Deleting an attribute twice leaves it
// deleted, so the call sends its request again after a 500, 502, 503 or 504 reply or a failed
// connection, as after a 429.
//
// Delete returns an error:
//
//   - for a scope without an environment, or a resourceKey or an attributeKey that is empty, .
//     or ..: an error that matches [ErrInvalidConfig], and no request for the attribute; without
//     WithScope, the lookup of the scope comes first;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete the
//     attribute;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceAttributesService) Delete(ctx context.Context, resourceKey, attributeKey string,
) error {
	r, err := s.client.inScope(ctx, request{
		op: opDeleteResourceAttribute, target: apiTarget,
		params: []string{resourceKey, attributeKey},
	})
	if err != nil {
		return err
	}
	return s.client.doNoValue(ctx, r)
}
