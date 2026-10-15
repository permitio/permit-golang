package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the roles of a resource of an environment.
const (
	opListResourceRoles             = "list_resource_roles"
	opCreateResourceRole            = "create_resource_role"
	opGetResourceRole               = "get_resource_role"
	opUpdateResourceRole            = "update_resource_role"
	opDeleteResourceRole            = "delete_resource_role"
	opAssignResourceRolePermissions = "assign_permissions_to_resource_role"
	opRemoveResourceRolePermissions = "remove_permissions_from_resource_role"
)

// ResourceRolesService lists, reads, creates, changes and deletes the roles of a resource of an
// environment, such as editor of the resource document, and grants permissions to them and
// removes permissions from them; [Client.ResourceRoles] holds it. Each method takes the key or the
// ID of the resource first, such as document, and a method that acts on one role takes the key
// or the ID of the role after it, such as editor. The call sends each as one segment of the path,
// escaped: a key that holds a / or a % names that resource or that role, not another route. A
// role extends other roles of its resource with the Extends of its create or its update, such as
// [models.ResourceRoleUpdate]; no method adds or removes one role that it extends.
//
// A role's attributes are a [models.Attributes], whose values may be of any JSON type, such as a
// number or an object, each number kept as a json.Number with every digit of the reply.
//
// Its calls act on the environment of the client's scope, whose project and environment fill the
// route: those that [WithScope] gives, or else those of the client's API key, which the first call
// that needs them looks up once, as [Client.Scope] says. A scope without an environment, such as
// that of a project-level API key without WithScope, is an error that matches [ErrInvalidConfig],
// and the call sends no request for its operation. Each method is sent with an
// environment-level API key of the environment, or with a key of a wider level and WithScope; the
// API answers a key that may not act on the environment with an error status, which the call
// returns as an [Error], such as one that matches [ErrForbidden].
type ResourceRolesService struct {
	client *Client
}

// List returns one page of the roles of the resource whose key or ID is resourceKey, in the
// order of the API's reply, with GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles.
// A nil opts, like the zero ListOptions, asks for the first page, with the API's default of 30
// roles on it; the API gives at most 100 on a page. The reply reports no counts, so the
// TotalCount and the PageCount of the page are nil. [ResourceRolesService.All] reads every page.
//
// List is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceRolesService] says. The call changes nothing, so it sends its request
// again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
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
//   - a reply in 200-299 that is not a JSON array of roles: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceRolesService) List(ctx context.Context, resourceKey string, opts *ListOptions,
) (*Page[models.ResourceRoleRead], error) {
	r, err := s.client.inScope(ctx, request{
		op: opListResourceRoles, target: apiTarget, params: []string{resourceKey},
	})
	if err != nil {
		return nil, err
	}
	return listPage[models.ResourceRoleRead](ctx, s.client, r, arrayShape, listOptions(opts))
}

// All returns an iterator over the roles of the resource whose key or ID is resourceKey, for a
// range loop. It reads the pages in turn, as [ResourceRolesService.List] reads one, from the page
// of opts.Page on, or the first, each of opts.PerPage roles, or else of 100, the most that the API
// gives on a page. It stops after the first page without roles, and never after a page with fewer
// roles than it asked for. It asks for the next page only when the loop wants a role past the
// last of a page, so a loop that stops early sends no other request. It reads opts when it is
// called: a later change to opts does not change the iterator. Each loop over the iterator reads
// the roles again, from the first page that opts asks for. The pages are requests of their own,
// so a role created or deleted during a loop can move the others from one page to the next.
//
// All is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as List is; each loop gets the scope as List does before its first page. Each
// page is a call of its own, with its own timeout and retries. When the scope or a page fails,
// with one of the errors that List returns, the iterator yields its error once, with the zero
// ResourceRoleRead, and stops.
func (s *ResourceRolesService) All(ctx context.Context, resourceKey string, opts *ListOptions,
) iter.Seq2[models.ResourceRoleRead, error] {
	r := request{op: opListResourceRoles, target: apiTarget, params: []string{resourceKey}}
	return allInScope[models.ResourceRoleRead](ctx, s.client, r, arrayShape, listOptions(opts))
}

// Get returns the role whose key or ID is roleKey of the resource whose key or ID is
// resourceKey, with GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}.
//
// Get is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceRolesService] says. The call changes nothing, so it sends its request
// again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no role:
//
//   - for a scope without an environment, or a resourceKey or a roleKey that is empty, . or ..:
//     an error that matches [ErrInvalidConfig], and no request for the role; without WithScope,
//     the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the resource has no such role;
//   - for a reply in 200-299 that is not a JSON object of a role: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceRolesService) Get(ctx context.Context, resourceKey, roleKey string,
) (*models.ResourceRoleRead, error) {
	return s.call(ctx, request{
		op: opGetResourceRole, target: apiTarget, params: []string{resourceKey, roleKey},
	})
}

// Create creates a role of the resource whose key or ID is resourceKey with
// POST /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles, and returns it as the API
// stores it, with its ID. The request holds the key and the name of role, and its other fields
// that are set, as [models.ResourceRoleCreate] says.
//
// Create is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceRolesService] says. It is sent again only after a 429, which the API
// did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have
// created the role, so Create returns that error, and [ResourceRolesService.Get] of the key tells
// whether the role exists.
//
// Create returns an error, and no role:
//
//   - for a scope without an environment, or a resourceKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the role; without WithScope, the lookup of
//     the scope comes first;
//   - for an attribute of role that cannot be JSON, such as a value that is a channel: an error
//     that matches ErrInvalidConfig, and no request for the role;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrConflict] when the resource has a role with the key already,
//     or one whose Detail names each field that the API rejects for a 422, such as a key that is
//     not URL-friendly;
//   - for a reply in 200-299 that is not a JSON object of a role: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceRolesService) Create(ctx context.Context, resourceKey string,
	role models.ResourceRoleCreate,
) (*models.ResourceRoleRead, error) {
	return s.call(ctx, request{
		op: opCreateResourceRole, target: apiTarget, params: []string{resourceKey}, body: role,
	})
}

// Update changes the role whose key or ID is roleKey of the resource whose key or ID is
// resourceKey, with PATCH /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id},
// and returns it as the API stores it. The request holds only the fields of update that are set,
// as [models.ResourceRoleUpdate] says: each list, such as the permissions and the roles that it
// extends, as a whole, in place of the role's list, an empty slice that is not nil as [], and nil
// not at all; null for a Description, Attributes or GrantedTo of [models.Null].
//
// Update is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceRolesService] says. Sending the same update twice sets the same
// fields, so the call sends its request again after a 500, 502, 503 or 504 reply or a failed
// connection, as after a 429.
//
// Update returns an error, and no role:
//
//   - for a scope without an environment, or a resourceKey or a roleKey that is empty, . or ..:
//     an error that matches [ErrInvalidConfig], and no request for the role; without WithScope,
//     the lookup of the scope comes first;
//   - for an attribute of update that cannot be JSON, such as a value that is a function: an
//     error that matches ErrInvalidConfig, and no request for the role;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the resource has no such role, or one whose
//     Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a role: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceRolesService) Update(ctx context.Context, resourceKey, roleKey string,
	update models.ResourceRoleUpdate,
) (*models.ResourceRoleRead, error) {
	return s.call(ctx, request{
		op: opUpdateResourceRole, target: apiTarget, params: []string{resourceKey, roleKey},
		body: update,
	})
}

// Delete deletes the role whose key or ID is roleKey of the resource whose key or ID is
// resourceKey, with DELETE /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}.
// It returns nil when the API answers 404, as the role is gone already, such as when an earlier
// attempt of the call deleted it; so a mistyped key of the role or of the resource returns nil
// too, and deletes nothing.
//
// Delete is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ResourceRolesService] says. Deleting a role twice leaves it deleted, so the
// call sends its request again after a 500, 502, 503 or 504 reply or a failed connection, as
// after a 429.
//
// Delete returns an error:
//
//   - for a scope without an environment, or a resourceKey or a roleKey that is empty, . or ..:
//     an error that matches [ErrInvalidConfig], and no request for the role; without WithScope,
//     the lookup of the scope comes first;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete the
//     role;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceRolesService) Delete(ctx context.Context, resourceKey, roleKey string) error {
	r, err := s.client.inScope(ctx, request{
		op: opDeleteResourceRole, target: apiTarget, params: []string{resourceKey, roleKey},
	})
	if err != nil {
		return err
	}
	return s.client.doNoValue(ctx, r)
}

// AssignPermissions grants permissions, each <resource>:<action>, such as document:read, to the
// role whose key or ID is roleKey of the resource whose key or ID is resourceKey, with
// POST /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/permissions, and
// returns the role as the API stores it. The role keeps the permissions that it has. The request
// holds permissions as they are given; nil sends [], as an empty slice does.
//
// AssignPermissions is sent with an environment-level API key, or with a key of a wider level
// and [WithScope], as [ResourceRolesService] says. v1's documentation says that the API skips a
// permission that the role has already, so the call sends its request again after a 500, 502,
// 503 or 504 reply or a failed connection, as after a 429.
//
// AssignPermissions returns an error, and no role:
//
//   - for a scope without an environment, or a resourceKey or a roleKey that is empty, . or ..:
//     an error that matches [ErrInvalidConfig], and no request for the role; without WithScope,
//     the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the resource has no such role, or one whose
//     Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a role: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceRolesService) AssignPermissions(ctx context.Context, resourceKey, roleKey string,
	permissions []string,
) (*models.ResourceRoleRead, error) {
	return s.call(ctx, request{
		op: opAssignResourceRolePermissions, target: apiTarget,
		params: []string{resourceKey, roleKey},
		body:   models.AddRolePermissions{Permissions: orEmpty(permissions)},
	})
}

// RemovePermissions removes permissions, each <resource>:<action>, such as document:read, from
// the role whose key or ID is roleKey of the resource whose key or ID is resourceKey, with
// DELETE /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/permissions, and
// returns the role as the API stores it. The request holds permissions as they are given; nil
// sends [], as an empty slice does. A 404 is an error, not a removal that happened already: the
// path names the role, and the body what to remove.
//
// RemovePermissions is sent with an environment-level API key, or with a key of a wider level
// and [WithScope], as [ResourceRolesService] says. v1's documentation says that the API skips a
// permission that the role does not have, so the call sends its request again after a 500, 502,
// 503 or 504 reply or a failed connection, as after a 429.
//
// RemovePermissions returns an error, and no role:
//
//   - for a scope without an environment, or a resourceKey or a roleKey that is empty, . or ..:
//     an error that matches [ErrInvalidConfig], and no request for the role; without WithScope,
//     the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the resource has no such role, or one whose
//     Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a role: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ResourceRolesService) RemovePermissions(ctx context.Context, resourceKey, roleKey string,
	permissions []string,
) (*models.ResourceRoleRead, error) {
	return s.call(ctx, request{
		op: opRemoveResourceRolePermissions, target: apiTarget,
		params: []string{resourceKey, roleKey},
		body:   models.RemoveRolePermissions{Permissions: orEmpty(permissions)},
	})
}

// call makes the call of r, a request of an operation of the roles of a resource whose reply
// holds a role, with the project and the environment of the client's scope, and returns the role.
func (s *ResourceRolesService) call(ctx context.Context, r request,
) (*models.ResourceRoleRead, error) {
	r, err := s.client.inScope(ctx, r)
	if err != nil {
		return nil, err
	}
	return do[models.ResourceRoleRead](ctx, s.client, r)
}
