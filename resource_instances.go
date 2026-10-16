package permit

import (
	"context"
	"iter"
	"time"

	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the resource instances.
const (
	opListResourceInstances         = "list_resource_instances"
	opCreateResourceInstance        = "create_resource_instance"
	opListResourceInstancesDetailed = "list_resource_instances_detailed"
	opGetResourceInstance           = "get_resource_instance"
	opUpdateResourceInstance        = "update_resource_instance"
	opDeleteResourceInstance        = "delete_resource_instance"
)

// detailedInstanceID is the last segment of the route of the detailed list of the instances,
// which a get of an instance cannot send as the instance's ID.
const detailedInstanceID = "detailed"

// ResourceInstancesService lists, reads, creates, changes and deletes the resource instances of
// an environment, the objects of its resources in its tenants, such as the document doc-1 in the
// tenant tenant-a, and lists them with their relationship tuples; [Client.ResourceInstances]
// holds it. A method that acts on one instance takes the instance as <resource>:<key>, such as
// document:doc-1, or its ID, and sends it as it is given, as one segment of the path, escaped,
// with its : as it is: an identifier that holds a / or a % names that instance, not another
// route.
//
// Its calls are calls of the facts API. By default they go to the Permit API, under
// /v2/facts/{proj_id}/{env_id}/resource_instances, and act on the environment of the client's
// scope: the project and the environment of [WithScope], or else those of the client's API key,
// which the first call that needs them looks up, as [Client.Scope] says, also when the call then
// fails before its own request. The exceptions are the refusals that
// [ResourceInstancesService.Create] names, of an instance without a Tenant, and that
// [ResourceInstancesService.Get] names, of the instance detailed: they come before the lookup,
// so they send no request at all. A scope without an environment, as that of a project-level API
// key without WithScope, is an error that matches [ErrInvalidConfig], and the call sends no
// request for the instances. With [WithFactsViaPDP], they go to the PDP container under
// /facts/resource_instances instead, need no scope, and act on the environment of the container's
// own API key.
//
// [ResourceInstancesService.WaitForSync] returns a copy whose creates and updates wait until the
// change reaches the policy of the PDP container.
type ResourceInstancesService struct {
	facts facts
}

// ResourceInstancesListOptions are the page and the filters of [ResourceInstancesService.List],
// [ResourceInstancesService.All], [ResourceInstancesService.ListDetailed] and
// [ResourceInstancesService.AllDetailed]. Each filter that is set is sent as it is given; one
// that is empty is left out of the query.
type ResourceInstancesListOptions struct {
	// ListOptions picks the page and the number of instances on it, sent in page and per_page.
	// With a PerPage of 0, List and ListDetailed leave the number to the API, which gives 30, and
	// All and AllDetailed ask for 100, the most that the API gives.
	ListOptions

	// Tenant keeps the instances of the tenant of this key, such as tenant-a, sent in tenant.
	Tenant string

	// Resource keeps the instances of the resource of this key, such as document, sent in
	// resource.
	Resource string

	// Search keeps the instances that the API's text search matches, which v1's documentation
	// of the parameter describes as a text search of the name or the key of an object: a key
	// that holds the text matches, not only the key equal to it, so Search is no filter of one
	// key. Each text that is not empty is sent in a search parameter of its own, in order, such
	// as search=a&search=b; what the API does with more than one text is not confirmed yet. The
	// API's spec allows at most 25 texts in a list and 100 in a detailed list.
	Search []string
}

// resourceInstancesRequest returns the request of op, the list or the detailed list of the
// instances, of those that opts filters, with tenant, resource and each text of search in its
// query when opts sets them, and the page of opts. A nil opts is the zero
// ResourceInstancesListOptions. The request holds copies of the values of opts, so a later
// change to opts does not change it.
func resourceInstancesRequest(op string, opts *ResourceInstancesListOptions,
) (request, ListOptions) {
	var o ResourceInstancesListOptions
	if opts != nil {
		o = *opts
	}
	query := &wire.Query{}
	for _, filter := range []struct{ name, value string }{
		{"tenant", o.Tenant}, {"resource", o.Resource},
	} {
		if filter.value != "" {
			query.AddString(filter.name, &filter.value)
		}
	}
	for _, text := range o.Search {
		if text != "" {
			query.AddString("search", &text)
		}
	}
	return request{op: op, query: query}, o.ListOptions
}

// WaitForSync returns a copy of the service whose creates and updates ask the PDP container to
// wait for sync: to hold its answer until the change reaches its policy, for at most timeout,
// so that a check made after the call returns sees it. A call through the copy sends the
// headers X-Wait-Timeout, the timeout in decimal seconds, such as 0 for a timeout of 0 or 0.5 for
// 500 milliseconds, and X-Timeout-Policy when opts give a policy with [WithSyncPolicy]. The
// service that WaitForSync is called on, and its other copies, keep their own waits: the copy
// shares no state with them.
//
// Only [ResourceInstancesService.Create] and [ResourceInstancesService.Update] wait for sync: of
// the routes that ResourceInstancesService calls, the PDP container waits on
// POST /facts/resource_instances and on PATCH /facts/resource_instances/{instance_id}, and
// answers the lists, the get and the delete of an instance without waiting. A call through the
// copy returns an error that matches [ErrInvalidConfig], and sends no request: from a client
// without [WithFactsViaPDP], as only the PDP container waits; of any other method, such as Get or
// Delete, which the container answers without waiting; for a timeout less than zero; and for a
// [SyncOption] that is not valid, in any place among opts.
func (s *ResourceInstancesService) WaitForSync(timeout time.Duration, opts ...SyncOption,
) *ResourceInstancesService {
	return &ResourceInstancesService{facts: s.facts.withWait(newSyncWait(timeout, opts))}
}

// List returns one page of the resource instances of the environment, in the order of the API's
// reply, those that match each filter of opts, with
// GET /v2/facts/{proj_id}/{env_id}/resource_instances. A nil opts, like the zero
// ResourceInstancesListOptions, asks for the first page of every instance, with the API's default
// of 30 instances on it; the API gives at most 100 on a page. The page holds the counts that the
// reply reports, and none when the reply is a JSON array. [ResourceInstancesService.All] reads
// every page. The request never sends the query parameter detailed: the relationships of each
// instance come with [ResourceInstancesService.ListDetailed].
//
// List needs an environment-level API key, or higher, and the scope of an environment, as
// [ResourceInstancesService] says. The call changes nothing, so it sends its request again after
// a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// List returns every failure as an error, and never a page with it:
//
//   - a Page or a PerPage less than zero: an error that matches [ErrInvalidConfig], and no
//     request for the instances;
//   - through a copy of [ResourceInstancesService.WaitForSync]: an error that matches
//     ErrInvalidConfig, and no request, as the PDP container answers a list without waiting;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply;
//   - a reply in 200-299 that is not a list of instances, a JSON array or the paginated JSON
//     object: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceInstancesService) List(ctx context.Context, opts *ResourceInstancesListOptions,
) (*Page[models.ResourceInstanceRead], error) {
	r, page := resourceInstancesRequest(opListResourceInstances, opts)
	r, err := s.facts.prepare(ctx, r)
	if err != nil {
		return nil, err
	}
	return listPage[models.ResourceInstanceRead](ctx, s.facts.client, r, arrayOrPageShape, page)
}

// All returns an iterator over the resource instances of the environment that match each filter
// of opts, for a range loop. It reads the pages in turn, as [ResourceInstancesService.List] reads
// one, from the page of opts.Page on, or the first, each of opts.PerPage instances, or else of
// 100, the most that the API gives on a page. It stops after the last page that the replies
// count, or after the first page without instances, and never after a page with fewer instances
// than it asked for. It asks for the next page only when the loop wants an instance past the last
// of a page, so a loop that stops early sends no other request. It reads opts when it is called: a
// later change to opts does not change the iterator. Each loop over the iterator reads the
// instances again, from the first page that opts asks for. The pages are requests of their own,
// so an instance created or deleted during a loop can move the others from one page to the next.
//
// All needs an environment-level API key, or higher, and the scope of an environment, as List
// does; each loop gets the scope before its first page. Each page is a call of its own, with its
// own timeout and retries. When a loop fails with one of the errors that List returns, such as
// for a page less than zero, through a copy of [ResourceInstancesService.WaitForSync], or for a
// page whose call fails, the iterator yields its error once, with the zero ResourceInstanceRead,
// and stops. After [Client.Close], it yields an error that matches [ErrClosed] and sends no
// request.
func (s *ResourceInstancesService) All(ctx context.Context, opts *ResourceInstancesListOptions,
) iter.Seq2[models.ResourceInstanceRead, error] {
	r, page := resourceInstancesRequest(opListResourceInstances, opts)
	return factsAll[models.ResourceInstanceRead](ctx, s.facts, r, arrayOrPageShape, page)
}

// ListDetailed returns one page of the resource instances of the environment with their
// relationship tuples, in the order of the API's reply, those that match each filter of opts,
// with GET /v2/facts/{proj_id}/{env_id}/resource_instances/detailed. Each instance holds its
// Relationships as the reply gives them. A nil opts, like the zero ResourceInstancesListOptions,
// asks for the first page of every instance, with the API's default of 30 instances on it; the
// API gives at most 100 on a page. The page holds the counts that the reply reports.
// [ResourceInstancesService.AllDetailed] reads every page.
//
// ListDetailed needs an environment-level API key, or higher, and the scope of an environment, as
// [ResourceInstancesService] says. The call changes nothing, so it sends its request again after
// a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// ListDetailed returns every failure as an error, and never a page with it:
//
//   - a Page or a PerPage less than zero: an error that matches [ErrInvalidConfig], and no
//     request for the instances;
//   - through a copy of [ResourceInstancesService.WaitForSync]: an error that matches
//     ErrInvalidConfig, and no request, as the PDP container answers a list without waiting;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply;
//   - a reply in 200-299 that is not the paginated JSON object of a list of instances, such as
//     a JSON array: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceInstancesService) ListDetailed(ctx context.Context,
	opts *ResourceInstancesListOptions,
) (*Page[models.ResourceInstanceDetailedRead], error) {
	r, page := resourceInstancesRequest(opListResourceInstancesDetailed, opts)
	r, err := s.facts.prepare(ctx, r)
	if err != nil {
		return nil, err
	}
	return listPage[models.ResourceInstanceDetailedRead](ctx, s.facts.client, r, pageShape, page)
}

// AllDetailed returns an iterator over the resource instances of the environment, with their
// relationship tuples, that match each filter of opts, for a range loop. It reads the pages in
// turn, as [ResourceInstancesService.ListDetailed] reads one, as [ResourceInstancesService.All]
// reads those of List: from the page of opts.Page on, or the first, each of opts.PerPage
// instances, or else of 100; it stops after the last page that the replies count, or after the
// first page without instances; and it asks for the next page only when the loop wants an
// instance past the last of a page. It reads opts when it is called, and each loop reads the
// instances again, from the first page that opts asks for.
//
// AllDetailed needs an environment-level API key, or higher, and the scope of an environment, as
// ListDetailed does; each loop gets the scope before its first page. Each page is a call of its
// own, with its own timeout and retries. When a loop fails with one of the errors that
// ListDetailed returns, the iterator yields its error once, with the zero
// ResourceInstanceDetailedRead, and stops. After [Client.Close], it yields an error that matches
// [ErrClosed] and sends no request.
func (s *ResourceInstancesService) AllDetailed(ctx context.Context,
	opts *ResourceInstancesListOptions,
) iter.Seq2[models.ResourceInstanceDetailedRead, error] {
	r, page := resourceInstancesRequest(opListResourceInstancesDetailed, opts)
	return factsAll[models.ResourceInstanceDetailedRead](ctx, s.facts, r, pageShape, page)
}

// Get returns the resource instance that instance names, with
// GET /v2/facts/{proj_id}/{env_id}/resource_instances/{instance_id}. Pass instance as
// <resource>:<key>, such as document:doc-1, or as the instance's ID, not as the Key of the
// instance alone, such as doc-1.
//
// Get needs an environment-level API key, or higher, and the scope of an environment, as
// [ResourceInstancesService] says. The call changes nothing, so it sends its request again after
// a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no instance:
//
//   - for an instance that is empty, . or .., or that is detailed, which names the route of
//     [ResourceInstancesService.ListDetailed]: an error that matches [ErrInvalidConfig], and no
//     request for the instance;
//   - through a copy of [ResourceInstancesService.WaitForSync]: an error that matches
//     ErrInvalidConfig, and no request, as the PDP container answers a read without waiting;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the environment has no such instance;
//   - for a reply in 200-299 that is not a JSON object of an instance, such as an empty body: an
//     Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceInstancesService) Get(ctx context.Context, instance string,
) (*models.ResourceInstanceRead, error) {
	c := s.facts.client
	if err := c.checkOpen(); err != nil {
		return nil, err
	}
	if instance == detailedInstanceID {
		op, _ := c.registry.Op(opGetResourceInstance)
		return nil, &configError{
			setting: op.ID + ": {instance_id} of " + op.Route,
			rule:    validate.PathInstanceIDDetailed,
		}
	}
	r, err := s.facts.prepare(ctx, request{
		op: opGetResourceInstance, params: []string{instance},
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceInstanceRead](ctx, c, r)
}

// Create creates a resource instance in the environment with
// POST /v2/facts/{proj_id}/{env_id}/resource_instances, and returns it as the API stores it,
// with its ID. The request holds the Key, the Tenant and the Resource of instance always, and
// its Attributes when they are not nil, as [models.ResourceInstanceCreate] says.
//
// Create needs an environment-level API key, or higher, and the scope of an environment, as
// [ResourceInstancesService] says. Through a copy of [ResourceInstancesService.WaitForSync], with
// [WithFactsViaPDP], it waits for sync: the PDP container holds its answer until the new instance
// reaches its policy, or the timeout of the wait passes. It is sent again only after a 429, which
// the API did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may
// have created the instance, so Create returns that error, and [ResourceInstancesService.Get] of
// <resource>:<key> tells whether the instance exists.
//
// Create returns an error, and no instance:
//
//   - for an instance whose Tenant is empty: an error that matches [ErrInvalidConfig], and no
//     request, not even the lookup of the scope; the SDK refuses it, as every instance belongs
//     to a tenant;
//   - for a wait that the call cannot make, as [ResourceInstancesService.WaitForSync] says: an
//     error that matches ErrInvalidConfig, and no request;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrConflict] for a 409, such as when the resource has an instance with the
//     key already, and [ErrNotFound] for a 404, and whose Detail names each field that the API
//     rejects for a 422, such as a key that holds a character that a key may not hold;
//   - for a reply in 200-299 that is not a JSON object of an instance: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceInstancesService) Create(ctx context.Context,
	instance models.ResourceInstanceCreate,
) (*models.ResourceInstanceRead, error) {
	if err := s.facts.client.checkOpen(); err != nil {
		return nil, err
	}
	if instance.Tenant == "" {
		return nil, &configError{
			setting: opCreateResourceInstance + ": ResourceInstanceCreate",
			rule:    validate.ResourceInstanceTenantEmpty,
		}
	}
	r, err := s.facts.prepare(ctx, request{op: opCreateResourceInstance, body: instance})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceInstanceRead](ctx, s.facts.client, r)
}

// Update changes the attributes of the resource instance that instance names, with
// PATCH /v2/facts/{proj_id}/{env_id}/resource_instances/{instance_id}, and returns the instance
// as the API stores it. Pass instance as <resource>:<key>, such as document:doc-1, or as the
// instance's ID, not as the Key of the instance alone, such as doc-1. The request holds the
// Attributes of update when they are set, as [models.ResourceInstanceUpdate] says, and nothing
// else: the zero ResourceInstanceUpdate sends {}, which changes nothing. Attributes that are set
// replace the instance's attributes as a whole: [models.Some] of an empty map sends {}, which
// removes every attribute, and [models.Null] sends null.
//
// Update needs an environment-level API key, or higher, and the scope of an environment, as
// [ResourceInstancesService] says. Through a copy of [ResourceInstancesService.WaitForSync], with
// [WithFactsViaPDP], it waits for sync: the PDP container holds its answer until the change
// reaches its policy, or the timeout of the wait passes. Sending the same update twice sets the
// same attributes, so the call sends its request again after a 500, 502, 503 or 504 reply or a
// failed connection, as after a 429.
//
// Update returns an error, and no instance:
//
//   - for an instance that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request for the instance;
//   - for a wait that the call cannot make, as [ResourceInstancesService.WaitForSync] says: an
//     error that matches ErrInvalidConfig, and no request;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the environment has no such instance, and whose Detail
//     names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of an instance: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceInstancesService) Update(ctx context.Context, instance string,
	update models.ResourceInstanceUpdate,
) (*models.ResourceInstanceRead, error) {
	r, err := s.facts.prepare(ctx, request{
		op: opUpdateResourceInstance, params: []string{instance}, body: update,
	})
	if err != nil {
		return nil, err
	}
	return do[models.ResourceInstanceRead](ctx, s.facts.client, r)
}

// Delete deletes the resource instance that instance names, with
// DELETE /v2/facts/{proj_id}/{env_id}/resource_instances/{instance_id}. Pass instance as
// <resource>:<key>, such as document:doc-1, or as the instance's ID, not as the Key of the
// instance alone, such as doc-1. It returns nil when the API answers 404, as the instance is gone
// already, such as when an earlier attempt of the call deleted it; so a mistyped identifier
// returns nil too, and deletes nothing. With
// [WithFactsViaPDP], a 404 with an empty body, which is how the cloud PDP answers every route
// under /facts, is an error that matches [ErrContainerPDPOnly] instead.
//
// Delete needs an environment-level API key, or higher, and the scope of an environment, as
// [ResourceInstancesService] says. Deleting an instance twice leaves it deleted, so the call
// sends its request again after a 500, 502, 503 or 504 reply or a failed connection, as after a
// 429.
//
// Delete returns an error:
//
//   - for an instance that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request for the instance;
//   - through a copy of [ResourceInstancesService.WaitForSync]: an error that matches
//     ErrInvalidConfig, and no request, as the PDP container answers a delete without waiting;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete the
//     instance;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceInstancesService) Delete(ctx context.Context, instance string) error {
	r, err := s.facts.prepare(ctx, request{
		op: opDeleteResourceInstance, params: []string{instance},
	})
	if err != nil {
		return err
	}
	return s.facts.client.doNoValue(ctx, r)
}
