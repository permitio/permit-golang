package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the relations of a resource.
const (
	opListResourceRelations  = "list_resource_relations"
	opCreateResourceRelation = "create_resource_relation"
	opGetResourceRelation    = "get_resource_relation"
	opDeleteResourceRelation = "delete_resource_relation"
)

// ResourceRelationsService lists, reads, creates and deletes the relations of the resources of an
// environment; [Client.ResourceRelations] holds it. A relation of a resource, the object, to
// another resource, the subject, such as parent from document to folder, lets a relationship
// tuple relate an instance of the subject to an instance of the object, such as
// folder:reports parent document:q3. The API has no update of a relation: delete it and create
// it again to change it.
//
// Each method takes the key or the ID of the object first, such as document, and a method that
// acts on one relation takes the relation's key or its ID next, such as parent. Each key is sent
// as one segment of the path, escaped: a key that holds a / or a % names that resource or that
// relation, not another route.
//
// Its calls act on the environment of the client's scope, whose project and environment fill the
// route: those that [WithScope] gives, or else those of the client's API key, which the first call
// that needs them looks up once, as [Client.Scope] says. A scope without an environment, such as
// that of a project-level API key without WithScope, is an error that matches [ErrInvalidConfig],
// and the call sends no request for its operation. Each method is sent with an
// environment-level API key of the environment, or with a key of a wider level and WithScope; the
// API answers a key that may not act on the environment with an error status, which the call
// returns as an [Error], such as one that matches [ErrForbidden].
type ResourceRelationsService struct {
	client *Client
}

// List returns one page of the relations of the resource whose key or ID is resourceKey, the
// object of each, in the order of the API's reply, with
// GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/relations. A nil opts, like the zero
// ListOptions, asks for the first page, with the API's default of 30 relations on it; the API
// gives at most 100 on a page. The reply is the paginated object, and the page holds its counts:
// the TotalCount of the relations of the resource and the PageCount, each nil when the reply
// leaves it out. [ResourceRelationsService.All] reads every page.
//
// List is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceRelationsService] says. The call changes nothing, so it sends its
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
//   - a reply in 200-299 that is not the paginated JSON object of relations, such as a JSON
//     array: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceRelationsService) List(ctx context.Context, resourceKey string,
	opts *ListOptions,
) (*Page[models.RelationRead], error) {
	r, err := s.client.inScope(ctx, request{
		op: opListResourceRelations, target: apiTarget, params: []string{resourceKey},
	})
	if err != nil {
		return nil, err
	}
	return listPage[models.RelationRead](ctx, s.client, r, pageShape, listOptions(opts))
}

// All returns an iterator over the relations of the resource whose key or ID is resourceKey, for
// a range loop. It reads the pages in turn, as [ResourceRelationsService.List] reads one, from the
// page of opts.Page on, or the first, each of opts.PerPage relations, or else of 100, the most
// that the API gives on a page. It stops after the first page without relations, or, when the loop
// started at the first page, after the page that the total_count of its reply shows to be the last,
// and never because a page holds fewer relations than it asked for. It asks for the next page only
// when the loop wants a relation past the last of a page, so a loop that stops early sends no other
// request. It reads opts when it is called: a later change to opts does not change the iterator.
// Each loop over the iterator reads the relations again, from the first page that opts asks for.
// The pages are requests of their own, so a relation created or deleted during a loop can move the
// others from one page to the next.
//
// All is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as List is; each loop gets the scope as List does before its first page. Each
// page is a call of its own, with its own timeout and retries. When the scope or a page fails,
// with one of the errors that List returns, the iterator yields its error once, with the zero
// RelationRead, and stops.
func (s *ResourceRelationsService) All(ctx context.Context, resourceKey string,
	opts *ListOptions,
) iter.Seq2[models.RelationRead, error] {
	r := request{op: opListResourceRelations, target: apiTarget, params: []string{resourceKey}}
	return allInScope[models.RelationRead](ctx, s.client, r, pageShape, listOptions(opts))
}

// Get returns the relation whose key or ID is relationKey, of the resource whose key or ID is
// resourceKey, its object, with
// GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/relations/{relation_id}.
//
// Get is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceRelationsService] says. The call changes nothing, so it sends its
// request again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no relation:
//
//   - for a scope without an environment, or a resourceKey or a relationKey that is empty, . or
//     ..: an error that matches [ErrInvalidConfig], and no request for the relation; without
//     WithScope, the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the environment has no such resource, or the resource no
//     such relation;
//   - for a reply in 200-299 that is not a JSON object of a relation: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceRelationsService) Get(ctx context.Context, resourceKey, relationKey string,
) (*models.RelationRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opGetResourceRelation, target: apiTarget, params: []string{resourceKey, relationKey},
	})
	if err != nil {
		return nil, err
	}
	return do[models.RelationRead](ctx, s.client, r)
}

// Create creates a relation of the resource whose key or ID is resourceKey, its object, to the
// resource that the SubjectResource of relation names, its subject, with
// POST /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/relations, and returns it as the API
// stores it, with its ID. The request holds the key, the name and the subject of relation, and its
// description when it is set, as [models.RelationCreate] says.
//
// Create is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceRelationsService] says. It is sent again only after a 429, which the
// API did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have
// created the relation, so Create returns that error, and [ResourceRelationsService.Get] of the
// key tells whether the relation exists.
//
// Create returns an error, and no relation:
//
//   - for a scope without an environment, or a resourceKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the relation; without WithScope, the lookup
//     of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the environment has no such resource and [ErrConflict]
//     when the resource has a relation with the key already, and whose Detail names each field
//     that the API rejects for a 422, such as a key that is not URL-friendly;
//   - for a reply in 200-299 that is not a JSON object of a relation: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceRelationsService) Create(ctx context.Context, resourceKey string,
	relation models.RelationCreate,
) (*models.RelationRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opCreateResourceRelation, target: apiTarget, params: []string{resourceKey},
		body: relation,
	})
	if err != nil {
		return nil, err
	}
	return do[models.RelationRead](ctx, s.client, r)
}

// Delete deletes the relation whose key or ID is relationKey, of the resource whose key or ID is
// resourceKey, its object, with
// DELETE /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/relations/{relation_id}. Delete
// returns nil when the API answers 404, as the relation is gone already, such as when an earlier
// attempt of the call deleted it; so a mistyped key returns nil too, and deletes nothing, as does
// a resource, a project or an environment that does not exist, when the API answers it with 404.
//
// Delete is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceRelationsService] says. Deleting a relation twice leaves it deleted,
// so the call sends its request again after a 500, 502, 503 or 504 reply or a failed connection,
// as after a 429.
//
// Delete returns an error:
//
//   - for a scope without an environment, or a resourceKey or a relationKey that is empty, . or
//     ..: an error that matches [ErrInvalidConfig], and no request for the relation; without
//     WithScope, the lookup of the scope comes first;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete the
//     relation;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceRelationsService) Delete(ctx context.Context, resourceKey, relationKey string,
) error {
	r, err := s.client.inScope(ctx, request{
		op: opDeleteResourceRelation, target: apiTarget, params: []string{resourceKey, relationKey},
	})
	if err != nil {
		return err
	}
	return s.client.doNoValue(ctx, r)
}
