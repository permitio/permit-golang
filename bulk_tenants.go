package permit

import (
	"context"

	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the bulk operations of the tenants.
const (
	opBulkCreateTenants = "bulk_create_tenants"
	opBulkDeleteTenants = "bulk_delete_tenants"
)

// BulkCreate creates tenants in the environment in one request, with
// POST /v2/facts/{proj_id}/{env_id}/bulk/tenants. The body is a JSON object whose operations
// holds the tenants, in order, each as [TenantsService.Create] sends one, as
// [models.TenantCreate] says. Nil sends an empty operations, as an empty slice does. The API's
// spec sets no least or largest number of tenants, and BulkCreate sends every tenant in the one
// request, however many there are. The reply is a JSON object without fields, so BulkCreate
// reads no body from a reply in 200-299, and returns nil for it.
//
// BulkCreate needs an environment-level API key, or higher, and the scope of an environment, as
// [TenantsService] says. With [WithFactsViaPDP], it goes to the PDP container, under
// /facts/bulk/tenants, which sends it on to the API and answers without waiting for its policy.
// It is sent again only after a 429, which the API did not act on: after a 500, 502, 503 or 504
// reply or a failed connection, the API may have created the tenants, so BulkCreate returns that
// error, and [TenantsService.Get] of a key tells whether the tenant exists.
//
// BulkCreate returns an error:
//
//   - through a copy of [TenantsService.WaitForSync]: an error that matches [ErrInvalidConfig],
//     and no request, as TenantsService.WaitForSync says: the PDP container answers the bulk
//     routes without waiting;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one whose Detail names each field that the API rejects for a 422, which the API's
//     spec lists for the route; with WithFactsViaPDP, a 404 with an empty body, which is how the
//     cloud PDP answers every route under /facts, matches [ErrContainerPDPOnly];
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *TenantsService) BulkCreate(ctx context.Context, tenants []models.TenantCreate) error {
	r, err := s.facts.prepare(ctx, request{
		op:   opBulkCreateTenants,
		body: models.TenantCreateBulkOperation{Operations: orEmpty(tenants)},
	})
	if err != nil {
		return err
	}
	return s.facts.client.doNoValue(ctx, r)
}

// BulkDelete deletes tenants of the environment in one request, with
// DELETE /v2/facts/{proj_id}/{env_id}/bulk/tenants. The body is a JSON object whose idents holds
// tenantKeys, the keys or the IDs of the tenants, in order, as they are given. Nil sends an empty
// idents, as an empty slice does. The API's spec sets no least or largest number of tenants, and
// BulkDelete sends every key in the one request, however many there are. The reply is a JSON
// object without fields, so BulkDelete reads no body from a reply in 200-299, and returns nil for
// it.
//
// The body, not the path, names the tenants to delete, so a 404 can mean a mistyped key:
// BulkDelete returns it as an [Error] that matches [ErrNotFound], never nil. What the API answers
// for a key that names no tenant is not confirmed yet. With [WithFactsViaPDP], a 404 with an
// empty body, which is how the cloud PDP answers every route under /facts, is an error that
// matches [ErrContainerPDPOnly] instead.
//
// BulkDelete needs an environment-level API key, or higher, and the scope of an environment, as
// [TenantsService] says. With WithFactsViaPDP, it goes to the PDP container, under
// /facts/bulk/tenants, which sends it on to the API and answers without waiting for its policy.
// Deleting tenants twice leaves them deleted, so the call sends its request again after a 500,
// 502, 503 or 504 reply or a failed connection, as after a 429; when an attempt that got no
// complete reply deleted the tenants, the next attempt can get an error status, which BulkDelete
// returns.
//
// BulkDelete returns an error:
//
//   - through a copy of [TenantsService.WaitForSync]: an error that matches [ErrInvalidConfig],
//     and no request, as TenantsService.WaitForSync says: the PDP container answers the bulk
//     routes without waiting;
//   - for a reply with an error status: an Error with the status and the body of the reply, such
//     as one that matches ErrNotFound for a 404, or one whose Detail names each field that the
//     API rejects for a 422;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *TenantsService) BulkDelete(ctx context.Context, tenantKeys []string) error {
	r, err := s.facts.prepare(ctx, request{
		op:   opBulkDeleteTenants,
		body: models.TenantDeleteBulkOperation{Idents: orEmpty(tenantKeys)},
	})
	if err != nil {
		return err
	}
	return s.facts.client.doNoValue(ctx, r)
}
