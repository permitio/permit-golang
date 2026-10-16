package permit

import (
	"context"

	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the bulk operations of the users.
const (
	opBulkCreateUsers  = "bulk_create_users"
	opBulkReplaceUsers = "bulk_replace_users"
	opBulkDeleteUsers  = "bulk_delete_users"
)

// BulkCreate creates users in the environment in one request, with
// POST /v2/facts/{proj_id}/{env_id}/bulk/users. The body is a JSON object whose operations holds
// the users, in order, each as [UsersService.Create] sends one, as [models.UserCreate] says. Nil
// sends an empty operations, as an empty slice does. The API's spec sets no least or largest
// number of users, and BulkCreate sends every user in the one request, however many there are.
// The reply is a JSON object without fields, so BulkCreate reads no body from a reply in 200-299,
// and returns nil for it.
//
// BulkCreate needs an environment-level API key, or higher, and the scope of an environment, as
// [UsersService] says. With [WithFactsViaPDP], it goes to the PDP container, under
// /facts/bulk/users, which sends it on to the API and answers without waiting for its policy.
// It is sent again only after a 429, which the API did not act on: after a 500, 502, 503 or 504
// reply or a failed connection, the API may have created the users, so BulkCreate returns that
// error, and [UsersService.Get] of a key tells whether the user exists.
//
// BulkCreate returns an error:
//
//   - through a copy of [UsersService.WaitForSync]: an error that matches [ErrInvalidConfig], and
//     no request, as UsersService.WaitForSync says: the PDP container answers the bulk routes
//     without waiting;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrConflict] for a 409, such as when the environment has one of the users
//     already, for which the API's documentation says that it creates none of them, and
//     [ErrNotFound] for a 404, such as for a role or a tenant of RoleAssignments that the
//     environment does not have, and whose Detail names each field that the API rejects for a
//     422; with WithFactsViaPDP, a 404 with an empty body, which is how the cloud PDP answers
//     every route under /facts, matches [ErrContainerPDPOnly] instead of ErrNotFound;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UsersService) BulkCreate(ctx context.Context, users []models.UserCreate) error {
	r, err := s.facts.prepare(ctx, request{
		op: opBulkCreateUsers, body: models.UserCreateBulkOperation{Operations: orEmpty(users)},
	})
	if err != nil {
		return err
	}
	return s.facts.client.doNoValue(ctx, r)
}

// BulkReplace replaces users of the environment in one request, with
// PUT /v2/facts/{proj_id}/{env_id}/bulk/users. The body is a JSON object whose operations holds
// the whole users, in order, each as [UsersService.Sync] sends one: each of Email, FirstName,
// LastName and Attributes that a user leaves out is left out of its item, and what the API
// keeps of the user's stored fields then, and what it does with a user that the environment
// does not have, is not confirmed yet. The request also sends the RoleAssignments of each user,
// and none when it has none; what a replace with or without them does to the roles that the
// user has already is not confirmed yet. Nil sends an empty operations, as an empty slice does.
// The API's spec sets no least or largest number of users, and BulkReplace sends every user in
// the one request, however many there are; the API's documentation says that it answers a
// request above the API's bulk limit with a 403, and replaces none of the users. The reply is a
// JSON object without fields, so BulkReplace reads no body from a reply in 200-299, and returns
// nil for it.
//
// BulkReplace needs an environment-level API key, or higher, and the scope of an environment, as
// [UsersService] says. With [WithFactsViaPDP], it goes to the PDP container, under
// /facts/bulk/users, which sends it on to the API and answers without waiting for its policy.
// Sending the same users twice stores the same users, so the call sends its request again after
// a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// BulkReplace returns an error:
//
//   - through a copy of [UsersService.WaitForSync]: an error that matches [ErrInvalidConfig], and
//     no request, as UsersService.WaitForSync says: the PDP container answers the bulk routes
//     without waiting;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrForbidden] for a 403, [ErrNotFound] for a 404, such as for a role or a
//     tenant of RoleAssignments that the environment does not have, and [ErrConflict] for a 409,
//     each of which the API's spec lists for the route, and whose Detail names each field that
//     the API rejects for a 422; with WithFactsViaPDP, a 404 with an empty body, which is how the
//     cloud PDP answers every route under /facts, matches [ErrContainerPDPOnly] instead of
//     ErrNotFound;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UsersService) BulkReplace(ctx context.Context, users []models.UserCreate) error {
	r, err := s.facts.prepare(ctx, request{
		op: opBulkReplaceUsers, body: models.UserReplaceBulkOperation{Operations: orEmpty(users)},
	})
	if err != nil {
		return err
	}
	return s.facts.client.doNoValue(ctx, r)
}

// BulkDelete deletes users of the environment in one request, with
// DELETE /v2/facts/{proj_id}/{env_id}/bulk/users. The body is a JSON object whose idents holds
// userKeys, the keys or the IDs of the users, in order, as they are given. Nil sends an empty
// idents, as an empty slice does, which the API's documentation says that it answers with a 400.
// The API's spec sets no largest number of users, and BulkDelete sends every key in the one
// request, however many there are; the API's documentation says that it answers a request above
// the API's bulk limit with a 403. The reply is a JSON object without fields, so BulkDelete reads
// no body from a reply in 200-299, and returns nil for it.
//
// The body, not the path, names the users to delete, so a 404 can mean a mistyped key:
// BulkDelete returns it as an [Error] that matches [ErrNotFound], never nil. What the API answers
// for a key that names no user is not confirmed yet. With [WithFactsViaPDP], a 404 with an empty
// body, which is how the cloud PDP answers every route under /facts, is an error that matches
// [ErrContainerPDPOnly] instead.
//
// BulkDelete needs an environment-level API key, or higher, and the scope of an environment, as
// [UsersService] says. With WithFactsViaPDP, it goes to the PDP container, under
// /facts/bulk/users, which sends it on to the API and answers without waiting for its policy.
// Deleting users twice leaves them deleted, so the call sends its request again after a 500,
// 502, 503 or 504 reply or a failed connection, as after a 429; when an attempt that got no
// complete reply deleted the users, the next attempt can get an error status, which BulkDelete
// returns.
//
// BulkDelete returns an error:
//
//   - through a copy of [UsersService.WaitForSync]: an error that matches [ErrInvalidConfig], and
//     no request, as UsersService.WaitForSync says: the PDP container answers the bulk routes
//     without waiting;
//   - for a reply with an error status: an Error with the status and the body of the reply, such
//     as one that matches [ErrForbidden] for a 403, or ErrNotFound for a 404, or one whose Detail
//     names each field that the API rejects for a 422;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *UsersService) BulkDelete(ctx context.Context, userKeys []string) error {
	r, err := s.facts.prepare(ctx, request{
		op: opBulkDeleteUsers, body: models.UserDeleteBulkOperation{Idents: orEmpty(userKeys)},
	})
	if err != nil {
		return err
	}
	return s.facts.client.doNoValue(ctx, r)
}
