// Package permit is the Go SDK for Permit.io.
//
// This is v2 of the SDK, imported as github.com/permitio/permit-golang/v2. v2 is under
// development on this branch and is not released.
//
// Applications that use v1, github.com/permitio/permit-golang, should stay on v1.3.x until v2 is
// released. v1.4.0 will be the last v1 release, and v1 gets no fixes after it.
//
// # Facts and wait-for-sync
//
// Facts are the data that policies decide on: users, tenants and the users in them, role
// assignments, resource instances and relationship tuples. A client writes and reads them
// through the facts API of Permit, under /v2/facts/{proj_id}/{env_id}/, and a PDP gets each
// change from Permit after the write returns. With [WithFactsViaPDP], the client sends these
// calls to the PDP container under /facts/ instead, and the container sends each on to Permit.
//
// The PDP container can hold its answer to a write until the write reaches its policy, when the
// request asks it to wait, so that a check made after the write returns sees it. It waits on
// these routes only:
//
//	POST   /facts/relationship_tuples
//	POST   /facts/resource_instances
//	PATCH  /facts/resource_instances/{instance_id}
//	DELETE /facts/role_assignments
//	POST   /facts/role_assignments
//	POST   /facts/tenants
//	POST   /facts/users
//	PATCH  /facts/users/{user_id}
//	PUT    /facts/users/{user_id}
//	DELETE /facts/users/{user_id}/roles
//	POST   /facts/users/{user_id}/roles
//
// The container sends every other facts call on to Permit and answers without waiting for its
// policy, such as the reads, the bulk writes, adding a user to a tenant, updating a tenant, and
// deleting a user, a tenant, a resource instance or a relationship tuple. Only the PDP container
// serves /facts/: the cloud PDP answers each call there with 404 and an empty body, which the
// call returns as an error that matches [ErrContainerPDPOnly].
package permit
