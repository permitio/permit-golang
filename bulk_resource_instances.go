package permit

import (
	"context"
	"strconv"

	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the bulk operations of the resource instances.
const (
	opBulkReplaceResourceInstances = "bulk_replace_resource_instances"
	opBulkDeleteResourceInstances  = "bulk_delete_resource_instances"
)

// BulkReplace creates or replaces resource instances of the environment in one request, with
// PUT /v2/facts/{proj_id}/{env_id}/bulk/resource_instances. The body is a JSON object whose
// operations holds the instances, in order, each with its Key, its Tenant and its Resource
// always, and its Attributes when they are not nil, as [models.ResourceInstanceCreate] says.
// The API's documentation says that the call creates each instance that the environment does
// not have, and replaces each one that it has, by its key and its resource. Nil sends an empty
// operations, as an empty slice does. The API's spec sets no least or largest number of
// instances, and BulkReplace sends every instance in the one request, however many there are.
// The reply is a JSON object without fields, so BulkReplace reads no body from a reply in
// 200-299, and returns nil for it.
//
// BulkReplace needs an environment-level API key, or higher, and the scope of an environment, as
// [ResourceInstancesService] says. With [WithFactsViaPDP], it goes to the PDP container, under
// /facts/bulk/resource_instances, which sends it on to the API and answers without waiting for
// its policy. Sending the same instances twice stores the same instances, so the call sends its
// request again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// BulkReplace returns an error:
//
//   - for an instance whose Tenant is empty, as [ResourceInstancesService.Create] refuses one: an
//     error that matches [ErrInvalidConfig] and names the instance's index in instances, such as
//     instances[2], and no request, not even the lookup of the scope; the SDK refuses it, as
//     every instance belongs to a tenant;
//   - through a copy of [ResourceInstancesService.WaitForSync]: an error that matches
//     [ErrInvalidConfig], and no request, as ResourceInstancesService.WaitForSync says: the PDP
//     container answers the bulk routes without waiting;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one whose Detail names each field that the API rejects for a 422, which the API's
//     spec lists for the route; with WithFactsViaPDP, a 404 with an empty body, which is how the
//     cloud PDP answers every route under /facts, matches [ErrContainerPDPOnly];
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceInstancesService) BulkReplace(ctx context.Context,
	instances []models.ResourceInstanceCreate,
) error {
	if err := s.facts.client.checkOpen(); err != nil {
		return err
	}
	for i, instance := range instances {
		if instance.Tenant == "" {
			return &configError{
				setting: opBulkReplaceResourceInstances + ": instances[" + strconv.Itoa(i) + "]",
				rule:    validate.ResourceInstanceTenantEmpty,
			}
		}
	}
	r, err := s.facts.prepare(ctx, request{
		op:   opBulkReplaceResourceInstances,
		body: models.ResourceInstanceCreateBulkOperation{Operations: orEmpty(instances)},
	})
	if err != nil {
		return err
	}
	return s.facts.client.doNoValue(ctx, r)
}

// BulkDelete deletes resource instances of the environment in one request, with
// DELETE /v2/facts/{proj_id}/{env_id}/bulk/resource_instances. The body is a JSON object whose
// idents holds instances, in order, as they are given: each as <resource>:<key>, such as
// document:doc-1, or as the instance's ID, not as the Key of the instance alone, such as doc-1.
// Nil sends an empty idents, as an empty slice does. The API's spec sets no least or largest
// number of instances, and BulkDelete sends every instance in the one request, however many
// there are. The reply is a JSON object without fields, so BulkDelete reads no body from a reply
// in 200-299, and returns nil for it.
//
// The body, not the path, names the instances to delete, so a 404 can mean a mistyped
// identifier: BulkDelete returns it as an [Error] that matches [ErrNotFound], never nil. What the
// API answers for an identifier that names no instance is not confirmed yet. With
// [WithFactsViaPDP], a 404 with an empty body, which is how the cloud PDP answers every route
// under /facts, is an error that matches [ErrContainerPDPOnly] instead.
//
// BulkDelete needs an environment-level API key, or higher, and the scope of an environment, as
// [ResourceInstancesService] says. With WithFactsViaPDP, it goes to the PDP container, under
// /facts/bulk/resource_instances, which sends it on to the API and answers without waiting for
// its policy. Deleting instances twice leaves them deleted, so the call sends its request again
// after a 500, 502, 503 or 504 reply or a failed connection, as after a 429; when an attempt that
// got no complete reply deleted the instances, the next attempt can get an error status, which
// BulkDelete returns.
//
// BulkDelete returns an error:
//
//   - through a copy of [ResourceInstancesService.WaitForSync]: an error that matches
//     [ErrInvalidConfig], and no request, as ResourceInstancesService.WaitForSync says: the PDP
//     container answers the bulk routes without waiting;
//   - for a reply with an error status: an Error with the status and the body of the reply, such
//     as one that matches ErrNotFound for a 404, or one whose Detail names each field that the
//     API rejects for a 422;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceInstancesService) BulkDelete(ctx context.Context, instances []string) error {
	r, err := s.facts.prepare(ctx, request{
		op:   opBulkDeleteResourceInstances,
		body: models.ResourceInstanceDeleteBulkOperation{Idents: orEmpty(instances)},
	})
	if err != nil {
		return err
	}
	return s.facts.client.doNoValue(ctx, r)
}
