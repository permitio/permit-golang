package permit

import (
	"context"
	"iter"
	"time"

	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the relationship tuples.
const (
	opListRelationshipTuples         = "list_relationship_tuples"
	opListRelationshipTuplesDetailed = "list_relationship_tuples_detailed"
	opCreateRelationshipTuple        = "create_relationship_tuple"
	opDeleteRelationshipTuple        = "delete_relationship_tuple"
)

// RelationshipTuplesService lists, creates and deletes the relationship tuples of an environment,
// the relations between its resource instances, such as folder:f1 parent document:doc-1;
// [Client.RelationshipTuples] holds it.
//
// Its calls are calls of the facts API. By default they go to the Permit API, under
// /v2/facts/{proj_id}/{env_id}/relationship_tuples, and act on the environment of the client's
// scope: the project and the environment of [WithScope], or else those of the client's API key,
// which the first call that needs them looks up, as [Client.Scope] says, also when the call then
// fails before its own request. A scope without an environment, as that of a project-level API
// key without WithScope, is an error that matches [ErrInvalidConfig], and the call sends no
// request for the relationship tuples. With [WithFactsViaPDP], they go to the PDP container
// under /facts/relationship_tuples instead, need no scope, and act on the environment of the
// container's own API key.
//
// [RelationshipTuplesService.WaitForSync] returns a copy whose creates wait until the new
// relationship tuple reaches the policy of the PDP container.
type RelationshipTuplesService struct {
	facts facts
}

// RelationshipTuplesListOptions are the page and the filters of [RelationshipTuplesService.List],
// [RelationshipTuplesService.All], [RelationshipTuplesService.ListDetailed] and
// [RelationshipTuplesService.AllDetailed]. Each filter that is set keeps only the relationship
// tuples that match it, and the call sends it in the query parameter that its doc comment names,
// as it is given. A filter that is empty, as each is in the zero RelationshipTuplesListOptions,
// is not sent and keeps every relationship tuple.
type RelationshipTuplesListOptions struct {
	// ListOptions picks the page and the number of relationship tuples on it, sent in page and
	// per_page. With a PerPage of 0, List and ListDetailed leave the number to the API, which
	// gives 30, and All and AllDetailed ask for 100, the most that the API gives.
	ListOptions

	// Tenant keeps the relationship tuples in the tenant with this key or ID, such as tenant-a,
	// sent in the query parameter tenant.
	Tenant string

	// Subject keeps the relationship tuples of this subject, as <resource>:<key>, such as
	// folder:f1, or its ID, sent in the query parameter subject.
	Subject string

	// Relation keeps the relationship tuples of the relation with this key or ID, such as
	// parent, sent in the query parameter relation.
	Relation string

	// Object keeps the relationship tuples of this object, as <resource>:<key>, such as
	// document:doc-1, or its ID, sent in the query parameter object.
	Object string

	// SubjectType keeps the relationship tuples whose subject is an instance of the resource
	// with this key or ID, such as folder, sent in the query parameter subject_type.
	SubjectType string

	// ObjectType keeps the relationship tuples whose object is an instance of the resource with
	// this key or ID, such as document, sent in the query parameter object_type.
	ObjectType string
}

// relationshipTuplesRequest returns the request of the list op of the relationship tuples that
// opts filters, with a parameter in its query for each filter of opts that is set, and the page
// of opts. A nil opts is the zero RelationshipTuplesListOptions. The request holds copies of the
// values of opts, so a later change to opts does not change it.
func relationshipTuplesRequest(op string, opts *RelationshipTuplesListOptions,
) (request, ListOptions) {
	var o RelationshipTuplesListOptions
	if opts != nil {
		o = *opts
	}
	query := &wire.Query{}
	for _, filter := range []struct{ name, value string }{
		{"tenant", o.Tenant},
		{"subject", o.Subject},
		{"relation", o.Relation},
		{"object", o.Object},
		{"subject_type", o.SubjectType},
		{"object_type", o.ObjectType},
	} {
		if filter.value != "" {
			query.AddString(filter.name, &filter.value)
		}
	}
	return request{op: op, query: query}, o.ListOptions
}

// WaitForSync returns a copy of the service whose creates ask the PDP container to wait for sync:
// to hold its answer until the new relationship tuple reaches its policy, for at most timeout,
// so that a check made after the call returns sees it. A call through the copy sends the headers
// X-Wait-Timeout, the timeout in decimal seconds, such as 0 for a timeout of 0 or 0.5 for 500
// milliseconds, and X-Timeout-Policy when opts give a policy with [WithSyncPolicy]. The service
// that WaitForSync is called on, and its other copies, keep their own waits: the copy shares no
// state with them.
//
// Only [RelationshipTuplesService.Create] waits for sync: of the routes that
// RelationshipTuplesService calls, the PDP container waits on POST /facts/relationship_tuples
// alone, and answers the delete and the lists without waiting. A call through the copy returns an
// error that matches [ErrInvalidConfig], and sends no request: from a client without
// [WithFactsViaPDP], as only the PDP container waits; of any other method, such as
// [RelationshipTuplesService.Delete] or List, which the container answers without waiting; for a
// timeout less than zero; and for a [SyncOption] that is not valid, in any place among opts.
func (s *RelationshipTuplesService) WaitForSync(timeout time.Duration, opts ...SyncOption,
) *RelationshipTuplesService {
	return &RelationshipTuplesService{facts: s.facts.withWait(newSyncWait(timeout, opts))}
}

// List returns one page of the relationship tuples of the environment, in the order of the API's
// reply, those that match each filter of opts, with
// GET /v2/facts/{proj_id}/{env_id}/relationship_tuples. It never sends the query parameters
// detailed and include_total_count, which the API marks deprecated;
// [RelationshipTuplesService.ListDetailed] reads the detailed list. A nil opts, like the zero
// RelationshipTuplesListOptions, asks for the first page of every relationship tuple, with the
// API's default of 30 relationship tuples on it; the API gives at most 100 on a page. The page
// holds the counts that the reply reports, and none when the reply is a JSON array. A page
// without relationship tuples, such as that of a filter that no relationship tuple matches, has
// an empty Data, not nil. [RelationshipTuplesService.All] reads every page.
//
// List needs an environment-level API key, or higher, and the scope of an environment, as
// [RelationshipTuplesService] says. The call changes nothing, so it sends its request again after
// a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// List returns every failure as an error, and never a page with it:
//
//   - a Page or a PerPage less than zero: an error that matches [ErrInvalidConfig], and no
//     request for the relationship tuples;
//   - through a copy of [RelationshipTuplesService.WaitForSync]: an error that matches
//     ErrInvalidConfig, and no request, as the PDP container answers a list without waiting;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply;
//   - a reply in 200-299 that is not a list of relationship tuples, a JSON array or the
//     paginated JSON object: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RelationshipTuplesService) List(ctx context.Context,
	opts *RelationshipTuplesListOptions,
) (*Page[models.RelationshipTupleRead], error) {
	r, page := relationshipTuplesRequest(opListRelationshipTuples, opts)
	r, err := s.facts.prepare(ctx, r)
	if err != nil {
		return nil, err
	}
	return listPage[models.RelationshipTupleRead](ctx, s.facts.client, r, arrayOrPageShape, page)
}

// All returns an iterator over the relationship tuples of the environment that match each filter
// of opts, for a range loop. It reads the pages in turn, as [RelationshipTuplesService.List] reads
// one, from the page of opts.Page on, or the first, each of opts.PerPage relationship tuples, or
// else of 100, the most that the API gives on a page, so that 101 relationship tuples take two
// pages, and the empty page after them when the replies do not count the pages. It stops after
// the last page that the replies count, or after the first page without relationship tuples, and
// never after a page with fewer relationship tuples than it asked for. It asks for the next page
// only when the loop wants a relationship tuple past the last of a page, so a loop that stops
// early sends no other request. It reads opts when it is called: a later change to opts does not
// change the iterator. Each loop over the iterator reads the relationship tuples again, from the
// first page that opts asks for. The pages are requests of their own, so a relationship tuple
// created or deleted during a loop can move the others from one page to the next.
//
// All needs an environment-level API key, or higher, and the scope of an environment, as List
// does; each loop gets the scope before its first page. Each page is a call of its own, with its
// own timeout and retries. When a loop fails with one of the errors that List returns, such as
// for a page less than zero, through a copy of [RelationshipTuplesService.WaitForSync], or for a
// page whose call fails, the iterator yields its error once, with the zero RelationshipTupleRead,
// and stops. After [Client.Close], it yields an error that matches [ErrClosed] and sends no
// request.
func (s *RelationshipTuplesService) All(ctx context.Context, opts *RelationshipTuplesListOptions,
) iter.Seq2[models.RelationshipTupleRead, error] {
	r, page := relationshipTuplesRequest(opListRelationshipTuples, opts)
	return factsAll[models.RelationshipTupleRead](ctx, s.facts, r, arrayOrPageShape, page)
}

// ListDetailed returns one page of the relationship tuples of the environment, as List does,
// from the detailed list of the API, with GET
// /v2/facts/{proj_id}/{env_id}/relationship_tuples/detailed, whose relationship tuples can hold
// their subject, their relation, their object and their tenant as objects too, as
// [models.RelationshipTupleDetailedRead] says. It sends the filters of opts as List does, and
// never the query parameter detailed, which the API marks deprecated. The API gives 30
// relationship tuples on a page by default and at most 100, and its reply is the paginated JSON
// object, whose counts the page holds. A page without relationship tuples has an empty Data, not
// nil. [RelationshipTuplesService.AllDetailed] reads every page.
//
// ListDetailed needs an environment-level API key, or higher, and the scope of an environment,
// as [RelationshipTuplesService] says. The call changes nothing, so it sends its request again
// after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// ListDetailed returns every failure as an error, and never a page with it:
//
//   - a Page or a PerPage less than zero: an error that matches [ErrInvalidConfig], and no
//     request for the relationship tuples;
//   - through a copy of [RelationshipTuplesService.WaitForSync]: an error that matches
//     ErrInvalidConfig, and no request, as the PDP container answers a list without waiting;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply;
//   - a reply in 200-299 that is not the paginated JSON object of a list of relationship tuples,
//     such as a JSON array: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RelationshipTuplesService) ListDetailed(ctx context.Context,
	opts *RelationshipTuplesListOptions,
) (*Page[models.RelationshipTupleDetailedRead], error) {
	r, page := relationshipTuplesRequest(opListRelationshipTuplesDetailed, opts)
	r, err := s.facts.prepare(ctx, r)
	if err != nil {
		return nil, err
	}
	return listPage[models.RelationshipTupleDetailedRead](ctx, s.facts.client, r, pageShape, page)
}

// AllDetailed returns an iterator over the relationship tuples of the environment that match each
// filter of opts, from the detailed list, for a range loop. It reads the pages in turn, as
// [RelationshipTuplesService.ListDetailed] reads one, as All does: from the page of opts.Page on,
// or the first, each of opts.PerPage relationship tuples, or else of 100, the most that the API
// gives on a page. It stops after the last page that the replies count, so that 101 relationship
// tuples take two pages, or after the first page without relationship tuples, and never after a
// page with fewer relationship tuples than it asked for. It asks for the next page only when the
// loop wants a relationship tuple past the last of a page, so a loop that stops early sends no
// other request. It reads opts when it is called, and each loop over the iterator reads the
// relationship tuples again, as All does.
//
// AllDetailed needs an environment-level API key, or higher, and the scope of an environment, as
// ListDetailed does; each loop gets the scope before its first page. Each page is a call of its
// own, with its own timeout and retries. When a loop fails with one of the errors that
// ListDetailed returns, the iterator yields its error once, with the zero
// RelationshipTupleDetailedRead, and stops. After [Client.Close], it yields an error that matches
// [ErrClosed] and sends no request.
func (s *RelationshipTuplesService) AllDetailed(ctx context.Context,
	opts *RelationshipTuplesListOptions,
) iter.Seq2[models.RelationshipTupleDetailedRead, error] {
	r, page := relationshipTuplesRequest(opListRelationshipTuplesDetailed, opts)
	return factsAll[models.RelationshipTupleDetailedRead](ctx, s.facts, r, pageShape, page)
}

// Create creates a relationship tuple with POST /v2/facts/{proj_id}/{env_id}/relationship_tuples,
// and returns it as the API stores it. The request holds the Subject, the Relation and the
// Object of tuple always, and its Tenant when it is set, as [models.RelationshipTupleCreate]
// says: an empty Tenant is left out of the request, not sent as an empty string.
//
// Create needs an environment-level API key, or higher, and the scope of an environment, as
// [RelationshipTuplesService] says. Through a copy of [RelationshipTuplesService.WaitForSync],
// with [WithFactsViaPDP], it waits for sync: the PDP container holds its answer until the new
// relationship tuple reaches its policy, or the timeout of the wait passes, so that a check made
// after Create returns sees the relation. It is sent again only after a 429, which the API did
// not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have
// created the relationship tuple, so Create returns that error, and a List with the subject, the
// relation and the object of the tuple tells whether it exists.
//
// Create returns an error, and no relationship tuple:
//
//   - for a wait that the call cannot make, as [RelationshipTuplesService.WaitForSync] says: an
//     error that matches [ErrInvalidConfig], and no request;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one for a 400, which the API's spec lists for the route, one that matches
//     [ErrNotFound] for a 404, or one whose Detail names each field that the API rejects for a
//     422;
//   - for a reply in 200-299 that is not a JSON object of a relationship tuple, such as an empty
//     body: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RelationshipTuplesService) Create(ctx context.Context,
	tuple models.RelationshipTupleCreate,
) (*models.RelationshipTupleRead, error) {
	r, err := s.facts.prepare(ctx, request{op: opCreateRelationshipTuple, body: tuple})
	if err != nil {
		return nil, err
	}
	return do[models.RelationshipTupleRead](ctx, s.facts.client, r)
}

// Delete deletes a relationship tuple with DELETE
// /v2/facts/{proj_id}/{env_id}/relationship_tuples and a body that holds the Subject, the
// Relation and the Object of tuple, as [models.RelationshipTupleDelete] says, and reads no body
// from a reply in 200-299, for which it returns nil: the API's spec lists a 204 without a body.
// The body, not the path, names the relationship tuple to delete, so a 404 can mean a mistyped
// key: Delete returns it as an [Error] that matches [ErrNotFound], never nil, also when the
// relationship tuple does not exist. With [WithFactsViaPDP], a 404 with an empty body, which is
// how the cloud PDP answers every route under /facts, is an error that matches
// [ErrContainerPDPOnly] instead.
//
// Delete needs an environment-level API key, or higher, and the scope of an environment, as
// [RelationshipTuplesService] says. With WithFactsViaPDP, it goes to the PDP container, under
// /facts/relationship_tuples, which sends it on to the API and answers without waiting for its
// policy. Deleting a relationship tuple twice leaves it deleted, so the call sends its request
// again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429; when an
// attempt that got no complete reply deleted the relationship tuple, the next attempt can get an
// error status, such as a 404, which Delete returns.
//
// Delete returns an error:
//
//   - through a copy of [RelationshipTuplesService.WaitForSync]: an error that matches
//     [ErrInvalidConfig], and no request, as RelationshipTuplesService.WaitForSync says: the PDP
//     container answers a delete without waiting;
//   - for a reply with an error status: an Error with the status and the body of the reply,
//     such as one that matches ErrNotFound for a 404, or one whose Detail names each field that
//     the API rejects for a 422;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RelationshipTuplesService) Delete(ctx context.Context,
	tuple models.RelationshipTupleDelete,
) error {
	r, err := s.facts.prepare(ctx, request{op: opDeleteRelationshipTuple, body: tuple})
	if err != nil {
		return err
	}
	return s.facts.client.doNoValue(ctx, r)
}
