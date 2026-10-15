package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the attributes of the users of an environment.
const (
	opListUserAttributes  = "list_user_attributes"
	opCreateUserAttribute = "create_user_attribute"
	opGetUserAttribute    = "get_user_attribute"
	opUpdateUserAttribute = "update_user_attribute"
	opDeleteUserAttribute = "delete_user_attribute"
)

// UserAttributesService lists, reads, creates, changes and deletes the attributes of the users of
// an environment, the schema of what a user may hold, such as department, with the type of its
// values; [Client.UserAttributes] holds it. The values that a user holds for them are a part of
// the user, in its attributes, which this service neither reads nor changes. A method that acts on
// one attribute takes its key or its ID, such as department, which the call sends as one segment
// of the path, escaped: a key that holds a / or a % names that attribute, not another route.
//
// The API takes and returns an attribute of the users as it does an attribute of a resource, so
// the calls take a [models.ResourceAttributeCreate] or a [models.ResourceAttributeUpdate], and
// return a [models.ResourceAttributeRead]. The API's documentation lists a query parameter
// resource_id on each route, __user by default, the resource that holds the attributes of the
// users. The calls never send it, so they act on the attributes of the users alone: the
// attributes of another resource are those of [ResourceAttributesService].
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
type UserAttributesService struct {
	client *Client
}

// List returns one page of the attributes of the users, in the order of the API's reply, with
// GET /v2/schema/{proj_id}/{env_id}/users/attributes. A nil opts, like the zero ListOptions, asks
// for the first page, with the API's default of 30 attributes on it; the API gives at most 100 on
// a page. The request's query holds page and per_page when opts sets them, and nothing else. Each
// attribute's type is read as the reply gives it, as [UserAttributesService] says. The reply
// reports no counts, so the TotalCount and the PageCount of the page are nil.
// [UserAttributesService.All] reads every page.
//
// List is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [UserAttributesService] says. The call changes nothing, so it sends its request
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
//   - a reply in 200-299 that is not a JSON array of attributes: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UserAttributesService) List(ctx context.Context, opts *ListOptions,
) (*Page[models.ResourceAttributeRead], error) {
	r, err := s.client.inScope(ctx, request{op: opListUserAttributes, target: apiTarget})
	if err != nil {
		return nil, err
	}
	return listPage[models.ResourceAttributeRead](ctx, s.client, r, arrayShape,
		listOptions(opts))
}

// All returns an iterator over the attributes of the users, for a range loop. It reads the pages
// in turn, as [UserAttributesService.List] reads one, from the page of opts.Page on, or the
// first, each of opts.PerPage attributes, or else of 100, the most that the API gives on a page.
// It stops after the first page without attributes, and never after a page with fewer attributes
// than it asked for. It asks for the next page only when the loop wants an attribute past the
// last of a page, so a loop that stops early sends no other request. It reads opts when it is
// called: a later change to opts does not change the iterator. Each loop over the iterator reads
// the attributes again, from the first page that opts asks for. The pages are requests of their
// own, so an attribute created or deleted during a loop can move the others from one page to the
// next.
//
// All is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as List is; each loop gets the scope as List does before its first page. Each
// page is a call of its own, with its own timeout and retries. When the scope or a page fails,
// with one of the errors that List returns, the iterator yields its error once, with the zero
// ResourceAttributeRead, and stops.
func (s *UserAttributesService) All(ctx context.Context, opts *ListOptions,
) iter.Seq2[models.ResourceAttributeRead, error] {
	r := request{op: opListUserAttributes, target: apiTarget}
	return allInScope[models.ResourceAttributeRead](ctx, s.client, r, arrayShape,
		listOptions(opts))
}

// Get returns the attribute of the users whose key or ID is attributeKey, with
// GET /v2/schema/{proj_id}/{env_id}/users/attributes/{attribute_id}, without a query. Its type is
// read as the reply gives it, as [UserAttributesService] says.
//
// Get is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [UserAttributesService] says. The call changes nothing, so it sends its request
// again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no attribute:
//
//   - for a scope without an environment, or an attributeKey that is empty, . or ..: an error
//     that matches [ErrInvalidConfig], and no request for the attribute; without WithScope, the
//     lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the users have no such attribute;
//   - for a reply in 200-299 that is not a JSON object of an attribute: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UserAttributesService) Get(ctx context.Context, attributeKey string,
) (*models.ResourceAttributeRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opGetUserAttribute, target: apiTarget, params: []string{attributeKey},
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceAttributeRead](ctx, s.client, r)
}

// Create creates an attribute of the users with
// POST /v2/schema/{proj_id}/{env_id}/users/attributes, without a query, and returns it as the API
// stores it, with its ID. The request holds the key and the type of attribute, and its
// description when it is set, as [models.ResourceAttributeCreate] says.
//
// Create is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [UserAttributesService] says. It is sent again only after a 429, which the API
// did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have
// created the attribute, so Create returns that error, and [UserAttributesService.Get] of the
// key tells whether the attribute exists.
//
// Create returns an error, and no attribute:
//
//   - for a scope without an environment: an error that matches [ErrInvalidConfig], and no
//     request for the attribute; without WithScope, the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrConflict] when the users have an attribute with the key
//     already, or one whose Detail names each field that the API rejects for a 422, such as a
//     key that is not URL-friendly or a type that the API does not know;
//   - for a reply in 200-299 that is not a JSON object of an attribute: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UserAttributesService) Create(ctx context.Context,
	attribute models.ResourceAttributeCreate,
) (*models.ResourceAttributeRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opCreateUserAttribute, target: apiTarget, body: attribute,
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceAttributeRead](ctx, s.client, r)
}

// Update changes the attribute of the users whose key or ID is attributeKey, with
// PATCH /v2/schema/{proj_id}/{env_id}/users/attributes/{attribute_id}, without a query, and
// returns it as the API stores it. The request holds only the fields of update that are set, as
// [models.ResourceAttributeUpdate] says: it sends null for a Description of [models.Null].
//
// Update is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [UserAttributesService] says. Sending the same update twice sets the same
// fields, so the call sends its request again after a 500, 502, 503 or 504 reply or a failed
// connection, as after a 429.
//
// Update returns an error, and no attribute:
//
//   - for a scope without an environment, or an attributeKey that is empty, . or ..: an error
//     that matches [ErrInvalidConfig], and no request for the attribute; without WithScope, the
//     lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the users have no such attribute, or one whose
//     Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of an attribute: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UserAttributesService) Update(ctx context.Context, attributeKey string,
	update models.ResourceAttributeUpdate,
) (*models.ResourceAttributeRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opUpdateUserAttribute, target: apiTarget, params: []string{attributeKey},
		body: update,
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceAttributeRead](ctx, s.client, r)
}

// Delete deletes the attribute of the users whose key or ID is attributeKey, with
// DELETE /v2/schema/{proj_id}/{env_id}/users/attributes/{attribute_id}. It sends no query, though
// the API's documentation lists page and per_page for the route too. It returns nil when the API
// answers 404, as the attribute is gone already, such as when an earlier attempt of the call
// deleted it; so a mistyped key returns nil too, and deletes nothing.
//
// Delete is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [UserAttributesService] says. Deleting an attribute twice leaves it deleted, so
// the call sends its request again after a 500, 502, 503 or 504 reply or a failed connection, as
// after a 429.
//
// Delete returns an error:
//
//   - for a scope without an environment, or an attributeKey that is empty, . or ..: an error
//     that matches [ErrInvalidConfig], and no request for the attribute; without WithScope, the
//     lookup of the scope comes first;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete the
//     attribute;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UserAttributesService) Delete(ctx context.Context, attributeKey string) error {
	r, err := s.client.inScope(ctx, request{
		op: opDeleteUserAttribute, target: apiTarget, params: []string{attributeKey},
	})
	if err != nil {
		return err
	}
	return s.client.doNoValue(ctx, r)
}
