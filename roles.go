package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the roles of an environment.
const (
	opListRoles             = "list_roles"
	opCreateRole            = "create_role"
	opGetRole               = "get_role"
	opUpdateRole            = "update_role"
	opDeleteRole            = "delete_role"
	opAssignRolePermissions = "assign_permissions_to_role"
	opRemoveRolePermissions = "remove_permissions_from_role"
	opRoleAncestors         = "get_role_ancestors"
	opRoleDescendants       = "get_role_descendants"
	opBulkReplaceRoles      = "bulk_create_or_replace_roles"
)

// RolesService lists, reads, creates, changes and deletes the roles of an environment, such as
// editor, grants permissions to them and removes permissions from them, reads the ancestors and
// the descendants of a role, and creates or replaces many roles at once; [Client.Roles] holds it.
// A method that acts on one role takes its key or its ID, such as editor, and sends it as one
// segment of the path, escaped: a key that holds a / or a % names that role, not another route. A
// role extends other roles with the Extends of its create or its update, such as
// [models.RoleUpdate]; no method adds or removes one role that it extends.
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
type RolesService struct {
	client *Client
}

// RolesListOptions are the page and the attribute filters of [RolesService.List] and
// [RolesService.All].
type RolesListOptions struct {
	// ListOptions picks the page and the number of roles on it, sent in page and per_page. With
	// a PerPage of 0, List leaves the number to the API, which gives 30, and All asks for 100,
	// the most that the API gives.
	ListOptions

	// Attributes keep the roles whose attributes match each filter, each sent in the query
	// parameter attr_<name>, as [AttributeFilter] says, such as
	// permit.AttributeEquals("tier", "gold"). Nil or empty keeps every role. A filter without a
	// name, one whose float64 is NaN or an infinity, and one whose name an earlier filter has,
	// are not valid.
	Attributes []AttributeFilter
}

// listRequest returns the request of the roles that opts filters, with a parameter
// attr_<name> in its query for each of the Attributes of opts, and the page of opts. A nil opts
// is the zero RolesListOptions. The request holds copies of the values of opts, so a later change
// to opts does not change it. Before it reads opts, it returns ErrClosed after Close; then an
// error that matches ErrInvalidConfig for an attribute filter that is not valid, as
// addAttributeFilters says.
func (s *RolesService) listRequest(opts *RolesListOptions) (request, ListOptions, error) {
	if err := s.client.checkOpen(); err != nil {
		return request{}, ListOptions{}, err
	}
	var o RolesListOptions
	if opts != nil {
		o = *opts
	}
	query := &wire.Query{}
	if err := addAttributeFilters(query, opListRoles, o.Attributes); err != nil {
		return request{}, ListOptions{}, err
	}
	return request{op: opListRoles, target: apiTarget, query: query}, o.ListOptions, nil
}

// List returns one page of the roles of the environment, in the order of the API's reply, with
// GET /v2/schema/{proj_id}/{env_id}/roles. A nil opts, like the zero RolesListOptions, asks for
// the first page of every role, with the API's default of 30 roles on it; the API gives at most
// 100 on a page. The attribute filters of opts are sent in the query, as [RolesListOptions]
// says. The page holds the counts that the reply reports, and none when the reply is a JSON
// array. [RolesService.All] reads every page.
//
// List is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [RolesService] says. The call changes nothing, so it sends its request again
// after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// List returns every failure as an error, and never a page with it:
//
//   - an attribute filter of opts that is not valid: an error that matches [ErrInvalidConfig]
//     and names the filter by its place in the Attributes of opts, from 1, and no request, not
//     even the lookup of the scope;
//   - a scope without an environment: an error that matches ErrInvalidConfig, and no request
//     for the list; without WithScope, the lookup of the scope comes first;
//   - a Page or a PerPage less than zero: an error that matches ErrInvalidConfig, and no request
//     for the list;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply;
//   - a reply in 200-299 that is neither a JSON array of roles nor the paginated JSON object of
//     them: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RolesService) List(ctx context.Context, opts *RolesListOptions,
) (*Page[models.RoleRead], error) {
	r, page, err := s.listRequest(opts)
	if err != nil {
		return nil, err
	}
	r, err = s.client.inScope(ctx, r)
	if err != nil {
		return nil, err
	}
	return listPage[models.RoleRead](ctx, s.client, r, arrayOrPageShape, page)
}

// All returns an iterator over the roles of the environment that match the attribute filters of
// opts, for a range loop. It reads the pages in turn, as [RolesService.List] reads one, from the
// page of opts.Page on, or the first, each of opts.PerPage roles, or else of 100, the most that
// the API gives on a page. It stops after the first page without roles, or, when the loop started
// at the first page, after the page that the total_count of its reply shows to be the last, and
// never because a page holds fewer roles than it asked for. It asks for the next page only when the
// loop wants a role past the last of a page, so a loop that stops early sends no other request. It
// reads opts when it is called: a later change to opts does not change the iterator. Each loop over
// the iterator reads the roles again, from the first page that opts asks for. The pages are
// requests of their own, so a role created or deleted during a loop can move the others from one
// page to the next.
//
// All is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as List is; each loop gets the scope as List does before its first page. Each
// page is a call of its own, with its own timeout and retries. When an attribute filter of opts
// is not valid, or the scope or a page fails, with one of the errors that List returns, the
// iterator yields its error once, with the zero RoleRead, and stops.
func (s *RolesService) All(ctx context.Context, opts *RolesListOptions,
) iter.Seq2[models.RoleRead, error] {
	r, page, err := s.listRequest(opts)
	if err != nil {
		return func(yield func(models.RoleRead, error) bool) {
			yield(models.RoleRead{}, err)
		}
	}
	return allInScope[models.RoleRead](ctx, s.client, r, arrayOrPageShape, page)
}

// Get returns the role whose key or ID is roleKey, with
// GET /v2/schema/{proj_id}/{env_id}/roles/{role_id}.
//
// Get is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [RolesService] says. The call changes nothing, so it sends its request again
// after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no role:
//
//   - for a scope without an environment, or a roleKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the role; without WithScope, the lookup of
//     the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the environment has no such role;
//   - for a reply in 200-299 that is not a JSON object of a role: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RolesService) Get(ctx context.Context, roleKey string) (*models.RoleRead, error) {
	return s.call(ctx, request{op: opGetRole, target: apiTarget, params: []string{roleKey}})
}

// Create creates a role in the environment with POST /v2/schema/{proj_id}/{env_id}/roles, and
// returns it as the API stores it, with its ID. The request holds the key and the name of role,
// and its other fields that are set, as [models.RoleCreate] says.
//
// Create is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [RolesService] says. It is sent again only after a 429, which the API did not
// act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have created
// the role, so Create returns that error, and [RolesService.Get] of the key tells whether the
// role exists.
//
// Create returns an error, and no role:
//
//   - for a scope without an environment: an error that matches [ErrInvalidConfig], and no
//     request for the role; without WithScope, the lookup of the scope comes first;
//   - for an attribute of role that cannot be JSON, such as a value that is a channel: an error
//     that matches ErrInvalidConfig, and no request for the role;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrConflict] when the environment has a role with the key
//     already, or one whose Detail names each field that the API rejects for a 422, such as a
//     key that is not URL-friendly;
//   - for a reply in 200-299 that is not a JSON object of a role: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RolesService) Create(ctx context.Context, role models.RoleCreate,
) (*models.RoleRead, error) {
	return s.call(ctx, request{op: opCreateRole, target: apiTarget, body: role})
}

// Update changes the role whose key or ID is roleKey with
// PATCH /v2/schema/{proj_id}/{env_id}/roles/{role_id}, and returns it as the API stores it. The
// request holds only the fields of update that are set, as [models.RoleUpdate] says: each list,
// such as the permissions and the roles that it extends, as a whole, in place of the role's
// list, an empty slice that is not nil as [], which clears it, and nil not at all; null for a
// Description, Attributes or GrantedTo of [models.Null], which clears it.
//
// Update is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [RolesService] says. Sending the same update twice sets the same fields, so the
// call sends its request again after a 500, 502, 503 or 504 reply or a failed connection, as
// after a 429.
//
// Update returns an error, and no role:
//
//   - for a scope without an environment, or a roleKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the role; without WithScope, the lookup of
//     the scope comes first;
//   - for an attribute of update that cannot be JSON, such as a value that is a function: an
//     error that matches ErrInvalidConfig, and no request for the role;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the environment has no such role, or one whose
//     Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a role: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RolesService) Update(ctx context.Context, roleKey string, update models.RoleUpdate,
) (*models.RoleRead, error) {
	return s.call(ctx, request{
		op: opUpdateRole, target: apiTarget, params: []string{roleKey}, body: update,
	})
}

// Delete deletes the role whose key or ID is roleKey, with
// DELETE /v2/schema/{proj_id}/{env_id}/roles/{role_id}. It returns nil when the API answers 404,
// as the role is gone already, such as when an earlier attempt of the call deleted it; so a
// mistyped key returns nil too, and deletes nothing.
//
// Delete is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [RolesService] says. Deleting a role twice leaves it deleted, so the call sends
// its request again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Delete returns an error:
//
//   - for a scope without an environment, or a roleKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the role; without WithScope, the lookup of
//     the scope comes first;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete the
//     role;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RolesService) Delete(ctx context.Context, roleKey string) error {
	r, err := s.client.inScope(ctx, request{
		op: opDeleteRole, target: apiTarget, params: []string{roleKey},
	})
	if err != nil {
		return err
	}
	return s.client.doNoValue(ctx, r)
}

// AssignPermissions grants permissions, each <resource>:<action>, such as document:read, to the
// role whose key or ID is roleKey, with
// POST /v2/schema/{proj_id}/{env_id}/roles/{role_id}/permissions, and returns the role as the
// API stores it. The role keeps the permissions that it has. The request holds permissions as
// they are given; nil sends [], as an empty slice does.
//
// AssignPermissions is sent with an environment-level API key, or with a key of a wider level
// and [WithScope], as [RolesService] says. v1's documentation says that the API skips a
// permission that the role has already, so the call sends its request again after a 500, 502,
// 503 or 504 reply or a failed connection, as after a 429.
//
// AssignPermissions returns an error, and no role:
//
//   - for a scope without an environment, or a roleKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the role; without WithScope, the lookup of
//     the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the environment has no such role, or one whose
//     Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a role: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RolesService) AssignPermissions(ctx context.Context, roleKey string,
	permissions []string,
) (*models.RoleRead, error) {
	return s.call(ctx, request{
		op: opAssignRolePermissions, target: apiTarget, params: []string{roleKey},
		body: models.AddRolePermissions{Permissions: orEmpty(permissions)},
	})
}

// RemovePermissions removes permissions, each <resource>:<action>, such as document:read, from
// the role whose key or ID is roleKey, with
// DELETE /v2/schema/{proj_id}/{env_id}/roles/{role_id}/permissions, and returns the role as the
// API stores it. The request holds permissions as they are given; nil sends [], as an empty slice
// does. A 404 is an error, not a removal that happened already: the path names the role, and the
// body what to remove.
//
// RemovePermissions is sent with an environment-level API key, or with a key of a wider level
// and [WithScope], as [RolesService] says. v1's documentation says that the API skips a
// permission that the role does not have, so the call sends its request again after a 500, 502,
// 503 or 504 reply or a failed connection, as after a 429.
//
// RemovePermissions returns an error, and no role:
//
//   - for a scope without an environment, or a roleKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the role; without WithScope, the lookup of
//     the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the environment has no such role, or one whose
//     Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a role: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RolesService) RemovePermissions(ctx context.Context, roleKey string,
	permissions []string,
) (*models.RoleRead, error) {
	return s.call(ctx, request{
		op: opRemoveRolePermissions, target: apiTarget, params: []string{roleKey},
		body: models.RemoveRolePermissions{Permissions: orEmpty(permissions)},
	})
}

// Ancestors returns the roles that the API lists as the ancestors of the role whose key or ID is
// roleKey, such as a role that it extends, with
// GET /v2/schema/{proj_id}/{env_id}/roles/{role_id}/ancestors, in the order of the reply. The
// reply holds them as a JSON array or as a paginated JSON object, and Ancestors reads either, as
// [models.RoleList] says; a reply without roles returns an empty slice, not nil.
//
// Ancestors is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [RolesService] says. The call changes nothing, so it sends its request again
// after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Ancestors returns an error, and no roles:
//
//   - for a scope without an environment, or a roleKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the roles; without WithScope, the lookup of
//     the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] for a 404;
//   - for a reply in 200-299 that is not a JSON object whose roles are a list of roles: an Error
//     that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RolesService) Ancestors(ctx context.Context, roleKey string) ([]models.RoleRead, error) {
	return s.relatives(ctx, opRoleAncestors, roleKey)
}

// Descendants returns the roles that the API lists as the descendants of the role whose key or ID
// is roleKey, such as a role that extends it, with
// GET /v2/schema/{proj_id}/{env_id}/roles/{role_id}/descendants, in the order of the reply. The
// reply holds them as a JSON array or as a paginated JSON object, and Descendants reads either,
// as [models.RoleList] says; a reply without roles returns an empty slice, not nil.
//
// Descendants is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [RolesService] says. The call changes nothing, so it sends its request again
// after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Descendants returns an error, and no roles:
//
//   - for a scope without an environment, or a roleKey that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the roles; without WithScope, the lookup of
//     the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] for a 404;
//   - for a reply in 200-299 that is not a JSON object whose roles are a list of roles: an Error
//     that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RolesService) Descendants(ctx context.Context, roleKey string,
) ([]models.RoleRead, error) {
	return s.relatives(ctx, opRoleDescendants, roleKey)
}

// BulkReplace creates each role of roles that the environment does not have, and replaces each
// role that it has, by its key, with PUT /v2/schema/{proj_id}/{env_id}/bulk/roles. It returns the
// reply's lists of the roles that the call created and of those that it replaced, as
// [models.RoleCreateBulkOperationResult] says, never the roles of the request. The request holds
// roles in order, each as [models.RoleCreateBulk] says; nil sends [], as an empty slice does.
//
// BulkReplace is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [RolesService] says. Sending the same roles twice leaves the environment with
// them, so the call sends its request again after a 500, 502, 503 or 504 reply or a failed
// connection, as after a 429; the reply to a request sent again can then list a role that an
// earlier attempt of the call created as replaced.
//
// BulkReplace returns an error, and no result:
//
//   - for a scope without an environment: an error that matches [ErrInvalidConfig], and no
//     request for the roles; without WithScope, the lookup of the scope comes first;
//   - for an attribute of a role that cannot be JSON, such as a value that is a channel: an
//     error that matches ErrInvalidConfig, and no request for the roles;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one whose Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of the result: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *RolesService) BulkReplace(ctx context.Context, roles []models.RoleCreateBulk,
) (*models.RoleCreateBulkOperationResult, error) {
	r, err := s.client.inScope(ctx, request{
		op: opBulkReplaceRoles, target: apiTarget,
		body: models.RoleCreateBulkOperation{Operations: orEmpty(roles)},
	})
	if err != nil {
		return nil, err
	}
	return do[models.RoleCreateBulkOperationResult](ctx, s.client, r)
}

// call makes the call of r, a request of an operation of the roles whose reply holds a role,
// with the project and the environment of the client's scope, and returns the role.
func (s *RolesService) call(ctx context.Context, r request) (*models.RoleRead, error) {
	r, err := s.client.inScope(ctx, r)
	if err != nil {
		return nil, err
	}
	return do[models.RoleRead](ctx, s.client, r)
}

// relatives makes the call of op, the ancestors or the descendants of the role whose key or ID is
// roleKey, with the project and the environment of the client's scope, and returns the roles of
// its reply.
func (s *RolesService) relatives(ctx context.Context, op, roleKey string,
) ([]models.RoleRead, error) {
	r, err := s.client.inScope(ctx, request{op: op, target: apiTarget, params: []string{roleKey}})
	if err != nil {
		return nil, err
	}
	list, err := do[models.RoleList](ctx, s.client, r)
	if err != nil {
		return nil, err
	}
	return list.Roles, nil
}

// orEmpty returns items, or an empty slice that is not nil when items is nil, so that a request
// sends [] for it rather than null.
func orEmpty[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
