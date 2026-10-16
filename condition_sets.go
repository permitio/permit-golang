package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the condition sets of an environment.
const (
	opListConditionSets       = "list_condition_sets"
	opCreateConditionSet      = "create_condition_set"
	opGetConditionSet         = "get_condition_set"
	opUpdateConditionSet      = "update_condition_set"
	opDeleteConditionSet      = "delete_condition_set"
	opConditionSetAncestors   = "get_condition_set_ancestors"
	opConditionSetDescendants = "get_condition_set_descendants"
)

// ConditionSetsService lists, reads, creates, changes and deletes the condition sets of an
// environment, and lists the ancestors and the descendants of a set; [Client.ConditionSets]
// holds it. A condition set is a user set, the users whose attributes match its conditions, such
// as the users whose age is above 18, or a resource set, the instances of a resource whose
// attributes match them, as its [models.ConditionSetType] says. A method that acts on one set
// takes its key or its ID, such as adults, and sends it as one segment of the path, escaped: a
// key that holds a / or a % names that set, not another route.
//
// A set's conditions are a [models.Attributes], whose values may be of any JSON type, each number
// kept as a json.Number with every digit of the reply. A set's type is an open enum: a reply that
// holds a type that this version of the SDK does not know is read rather than rejected, and the
// type's Known method reports false for it.
//
// Its calls act on the environment of the client's scope, whose project and environment fill the
// route: those that [WithScope] gives, or else those of the client's API key, which the first call
// that needs them looks up once, as [Client.Scope] says. A scope without an environment, such as
// that of a project-level API key without WithScope, is an error that matches [ErrInvalidConfig],
// and the call sends no request for its operation. Each method is sent with an
// environment-level API key of the environment, or with a key of a wider level and WithScope; the
// API answers a key that may not act on the environment with an error status, which the call
// returns as an [Error], such as one that matches [ErrForbidden].
type ConditionSetsService struct {
	client *Client
}

// ConditionSetsListOptions are the page and the filters of [ConditionSetsService.List] and
// [ConditionSetsService.All].
type ConditionSetsListOptions struct {
	// ListOptions picks the page and the number of sets on it, sent in page and per_page. With a
	// PerPage of 0, List leaves the number to the API, which gives 30, and All asks for 100, the
	// most that the API gives.
	ListOptions

	// Type keeps the sets of the type, sent as it is in the query parameter type. Empty leaves
	// it out, and keeps the sets of every type.
	Type models.ConditionSetType

	// Search is sent as it is given in the query parameter search, which the API's
	// documentation describes as a text search of the name or the key of a set. Empty leaves it
	// out.
	Search string
}

// conditionSetsRequest returns the request of the condition sets that opts filters, with type
// and search in its query when opts sets them, and the page of opts. A nil opts is the zero
// ConditionSetsListOptions. The request holds copies of the values of opts, so a later change to
// opts does not change it.
func conditionSetsRequest(opts *ConditionSetsListOptions) (request, ListOptions) {
	var o ConditionSetsListOptions
	if opts != nil {
		o = *opts
	}
	query := &wire.Query{}
	if o.Type != "" {
		query.AddString("type", new(string(o.Type)))
	}
	if o.Search != "" {
		query.AddString("search", &o.Search)
	}
	return request{op: opListConditionSets, target: apiTarget, query: query}, o.ListOptions
}

// List returns one page of the condition sets of the environment, in the order of the API's
// reply, with GET /v2/schema/{proj_id}/{env_id}/condition_sets. A nil opts, like the zero
// ConditionSetsListOptions, asks for the first page of every set, with the API's default of 30
// sets on it; the API gives at most 100 on a page. The Type and the Search of opts are sent in
// the query, as [ConditionSetsListOptions] says. The reply holds the sets as a JSON array or as
// the paginated JSON object, and List reads either; the page holds the counts that the reply
// reports, and none when the reply is a JSON array. [ConditionSetsService.All] reads every page.
//
// List is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ConditionSetsService] says. The call changes nothing, so it sends its request
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
//   - a reply in 200-299 that is neither a JSON array of sets nor the paginated JSON object of
//     them, such as an empty body, null, or an object without a data array: an Error that says
//     so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ConditionSetsService) List(ctx context.Context, opts *ConditionSetsListOptions,
) (*Page[models.ConditionSetRead], error) {
	r, page := conditionSetsRequest(opts)
	r, err := s.client.inScope(ctx, r)
	if err != nil {
		return nil, err
	}
	return listPage[models.ConditionSetRead](ctx, s.client, r, arrayOrPageShape, page)
}

// All returns an iterator over the condition sets of the environment that match the Type and the
// Search of opts, for a range loop. It reads the pages in turn, as [ConditionSetsService.List]
// reads one, from the page of opts.Page on, or the first, each of opts.PerPage sets, or else of
// 100, the most that the API gives on a page. It stops after the first page without sets, or, when
// the loop started at the first page, after the page that the total_count of its reply shows to be
// the last, and never because a page holds fewer sets than it asked for. It asks for the next page
// only when the loop wants a set past the last of a page, so a loop that stops early sends no other
// request. It reads opts when it is called: a later change to opts does not change the iterator.
// Each loop over the iterator reads the sets again, from the first page that opts asks for. The
// pages are requests of their own, so a set created or deleted during a loop can move the others
// from one page to the next.
//
// All is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as List is; each loop gets the scope as List does before its first page. Each
// page is a call of its own, with its own timeout and retries. When the scope or a page fails,
// with one of the errors that List returns, the iterator yields its error once, with the zero
// ConditionSetRead, and stops.
func (s *ConditionSetsService) All(ctx context.Context, opts *ConditionSetsListOptions,
) iter.Seq2[models.ConditionSetRead, error] {
	r, page := conditionSetsRequest(opts)
	return allInScope[models.ConditionSetRead](ctx, s.client, r, arrayOrPageShape, page)
}

// Get returns the condition set whose key or ID is conditionSetKey, with
// GET /v2/schema/{proj_id}/{env_id}/condition_sets/{condition_set_id}.
//
// Get is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ConditionSetsService] says. The call changes nothing, so it sends its request
// again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no set:
//
//   - for a scope without an environment, or a conditionSetKey that is empty, . or ..: an error
//     that matches [ErrInvalidConfig], and no request for the set; without WithScope, the lookup
//     of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the environment has no such set;
//   - for a reply in 200-299 that is not a JSON object of a set: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ConditionSetsService) Get(ctx context.Context, conditionSetKey string,
) (*models.ConditionSetRead, error) {
	return s.call(ctx, request{
		op: opGetConditionSet, target: apiTarget, params: []string{conditionSetKey},
	})
}

// Create creates a condition set in the environment with
// POST /v2/schema/{proj_id}/{env_id}/condition_sets, and returns it as the API stores it, with
// its ID. The request holds the key and the name of set, and its other fields that are set, as
// [models.ConditionSetCreate] says.
//
// Create is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ConditionSetsService] says. It is sent again only after a 429, which the API
// did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have
// created the set, so Create returns that error, and [ConditionSetsService.Get] of the key tells
// whether the set exists.
//
// Create returns an error, and no set:
//
//   - for a user set with a resource, a set whose Type is [models.ConditionSetTypeUserSet] or
//     empty and whose ResourceID is not empty, as a user set takes no resource: an error that
//     matches [ErrInvalidConfig], and no request, not even the lookup of the scope;
//   - for a scope without an environment: an error that matches ErrInvalidConfig, and no request
//     for the set; without WithScope, the lookup of the scope comes first;
//   - for a condition of set that cannot be JSON, such as a value that is a channel: an error
//     that matches ErrInvalidConfig, and no request for the set;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrConflict] when the environment has a set with the key
//     already, or one whose Detail names each field that the API rejects for a 422, such as a
//     key that is not URL-friendly;
//   - for a reply in 200-299 that is not a JSON object of a set: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ConditionSetsService) Create(ctx context.Context, set models.ConditionSetCreate,
) (*models.ConditionSetRead, error) {
	if err := s.client.checkOpen(); err != nil {
		return nil, err
	}
	userSet := set.Type == "" || set.Type == models.ConditionSetTypeUserSet
	if userSet && set.ResourceID != "" {
		return nil, &configError{
			setting: opCreateConditionSet + ": ConditionSetCreate",
			rule:    validate.ConditionSetUserSetResource,
		}
	}
	return s.call(ctx, request{op: opCreateConditionSet, target: apiTarget, body: set})
}

// Update changes the condition set whose key or ID is conditionSetKey with
// PATCH /v2/schema/{proj_id}/{env_id}/condition_sets/{condition_set_id}, and returns it as the
// API stores it. The request holds only the fields of update that are set, as
// [models.ConditionSetUpdate] says: its conditions, when they are not nil, as a whole, in place
// of the set's conditions.
//
// Update is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ConditionSetsService] says. Sending the same update twice sets the same
// fields, so the call sends its request again after a 500, 502, 503 or 504 reply or a failed
// connection, as after a 429.
//
// Update returns an error, and no set:
//
//   - for a scope without an environment, or a conditionSetKey that is empty, . or ..: an error
//     that matches [ErrInvalidConfig], and no request for the set; without WithScope, the lookup
//     of the scope comes first;
//   - for a condition of update that cannot be JSON, such as a value that is a function: an
//     error that matches ErrInvalidConfig, and no request for the set;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the environment has no such set, or one whose
//     Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a set: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ConditionSetsService) Update(ctx context.Context, conditionSetKey string,
	update models.ConditionSetUpdate,
) (*models.ConditionSetRead, error) {
	return s.call(ctx, request{
		op: opUpdateConditionSet, target: apiTarget, params: []string{conditionSetKey},
		body: update,
	})
}

// Delete deletes the condition set whose key or ID is conditionSetKey, with
// DELETE /v2/schema/{proj_id}/{env_id}/condition_sets/{condition_set_id}. It returns nil when the
// API answers 404, as the set is gone already, such as when an earlier attempt of the call
// deleted it; so a mistyped key returns nil too, and deletes nothing.
//
// Delete is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ConditionSetsService] says. Deleting a set twice leaves it deleted, so the
// call sends its request again after a 500, 502, 503 or 504 reply or a failed connection, as
// after a 429.
//
// Delete returns an error:
//
//   - for a scope without an environment, or a conditionSetKey that is empty, . or ..: an error
//     that matches [ErrInvalidConfig], and no request for the set; without WithScope, the lookup
//     of the scope comes first;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete the
//     set;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ConditionSetsService) Delete(ctx context.Context, conditionSetKey string) error {
	r, err := s.client.inScope(ctx, request{
		op: opDeleteConditionSet, target: apiTarget, params: []string{conditionSetKey},
	})
	if err != nil {
		return err
	}
	return s.client.doNoValue(ctx, r)
}

// Ancestors returns one page of the condition sets that the API lists as the ancestors of the
// set whose key or ID is conditionSetKey, its parent, the parent of its parent and so on, in the
// order of the reply, with
// GET /v2/schema/{proj_id}/{env_id}/condition_sets/{condition_set_id}/ancestors. A nil opts, like
// the zero ListOptions, asks for the first page, with the API's default of 30 sets on it; the API
// gives at most 100 on a page. The reply reports no counts, so the TotalCount and the PageCount
// of the page are nil. [ConditionSetsService.AncestorsAll] reads every page.
//
// Ancestors is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ConditionSetsService] says. The call changes nothing, so it sends its request
// again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Ancestors returns every failure as an error, and never a page with it:
//
//   - a scope without an environment, or a conditionSetKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the sets; without WithScope, the lookup of
//     the scope comes first;
//   - a Page or a PerPage less than zero: an error that matches ErrInvalidConfig, and no request
//     for the sets;
//   - a reply with an error status: an [Error] with the status and the body of the reply, such
//     as one that matches [ErrNotFound] for a 404;
//   - a reply in 200-299 that is not a JSON array of sets: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ConditionSetsService) Ancestors(ctx context.Context, conditionSetKey string,
	opts *ListOptions,
) (*Page[models.ConditionSetRead], error) {
	return s.relatives(ctx, opConditionSetAncestors, conditionSetKey, opts)
}

// AncestorsAll returns an iterator over the condition sets that the API lists as the ancestors of
// the set whose key or ID is conditionSetKey, for a range loop. It reads the pages in turn, as
// [ConditionSetsService.Ancestors] reads one, from the page of opts.Page on, or the first, each of
// opts.PerPage sets, or else of 100, the most that the API gives on a page. It stops after the
// first page without sets, and never after a page with fewer sets than it asked for. It asks for
// the next page only when the loop wants a set past the last of a page, so a loop that stops
// early sends no other request. It reads opts when it is called: a later change to opts does not
// change the iterator. Each loop over the iterator reads the sets again, from the first page that
// opts asks for.
//
// AncestorsAll is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as Ancestors is; each loop gets the scope as Ancestors does before its first page.
// Each page is a call of its own, with its own timeout and retries. When the scope or a page
// fails, with one of the errors that Ancestors returns, the iterator yields its error once, with
// the zero ConditionSetRead, and stops.
func (s *ConditionSetsService) AncestorsAll(ctx context.Context, conditionSetKey string,
	opts *ListOptions,
) iter.Seq2[models.ConditionSetRead, error] {
	r := request{op: opConditionSetAncestors, target: apiTarget, params: []string{conditionSetKey}}
	return allInScope[models.ConditionSetRead](ctx, s.client, r, arrayShape, listOptions(opts))
}

// Descendants returns one page of the condition sets that the API lists as the descendants of
// the set whose key or ID is conditionSetKey, the sets whose parent it is, their own descendants
// and so on, in the order of the reply, with
// GET /v2/schema/{proj_id}/{env_id}/condition_sets/{condition_set_id}/descendants. A nil opts,
// like the zero ListOptions, asks for the first page, with the API's default of 30 sets on it;
// the API gives at most 100 on a page. The reply reports no counts, so the TotalCount and the
// PageCount of the page are nil. [ConditionSetsService.DescendantsAll] reads every page.
//
// Descendants is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ConditionSetsService] says. The call changes nothing, so it sends its request
// again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Descendants returns every failure as an error, and never a page with it:
//
//   - a scope without an environment, or a conditionSetKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the sets; without WithScope, the lookup of
//     the scope comes first;
//   - a Page or a PerPage less than zero: an error that matches ErrInvalidConfig, and no request
//     for the sets;
//   - a reply with an error status: an [Error] with the status and the body of the reply, such
//     as one that matches [ErrNotFound] for a 404;
//   - a reply in 200-299 that is not a JSON array of sets: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ConditionSetsService) Descendants(ctx context.Context, conditionSetKey string,
	opts *ListOptions,
) (*Page[models.ConditionSetRead], error) {
	return s.relatives(ctx, opConditionSetDescendants, conditionSetKey, opts)
}

// DescendantsAll returns an iterator over the condition sets that the API lists as the
// descendants of the set whose key or ID is conditionSetKey, for a range loop. It reads the pages
// in turn, as [ConditionSetsService.Descendants] reads one, from the page of opts.Page on, or the
// first, each of opts.PerPage sets, or else of 100, the most that the API gives on a page. It
// stops after the first page without sets, and never after a page with fewer sets than it asked
// for. It asks for the next page only when the loop wants a set past the last of a page, so a
// loop that stops early sends no other request. It reads opts when it is called: a later change
// to opts does not change the iterator. Each loop over the iterator reads the sets again, from
// the first page that opts asks for.
//
// DescendantsAll is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as Descendants is; each loop gets the scope as Descendants does before its first
// page. Each page is a call of its own, with its own timeout and retries. When the scope or a
// page fails, with one of the errors that Descendants returns, the iterator yields its error
// once, with the zero ConditionSetRead, and stops.
func (s *ConditionSetsService) DescendantsAll(ctx context.Context, conditionSetKey string,
	opts *ListOptions,
) iter.Seq2[models.ConditionSetRead, error] {
	r := request{
		op: opConditionSetDescendants, target: apiTarget, params: []string{conditionSetKey},
	}
	return allInScope[models.ConditionSetRead](ctx, s.client, r, arrayShape, listOptions(opts))
}

// call makes the call of r, a request of an operation of the condition sets whose reply holds a
// set, with the project and the environment of the client's scope, and returns the set.
func (s *ConditionSetsService) call(ctx context.Context, r request,
) (*models.ConditionSetRead, error) {
	r, err := s.client.inScope(ctx, r)
	if err != nil {
		return nil, err
	}
	return do[models.ConditionSetRead](ctx, s.client, r)
}

// relatives returns the page of opts of the call of op, the ancestors or the descendants of the
// set whose key or ID is conditionSetKey, with the project and the environment of the client's
// scope.
func (s *ConditionSetsService) relatives(ctx context.Context, op, conditionSetKey string,
	opts *ListOptions,
) (*Page[models.ConditionSetRead], error) {
	r, err := s.client.inScope(ctx, request{
		op: op, target: apiTarget, params: []string{conditionSetKey},
	})
	if err != nil {
		return nil, err
	}
	return listPage[models.ConditionSetRead](ctx, s.client, r, arrayShape, listOptions(opts))
}
