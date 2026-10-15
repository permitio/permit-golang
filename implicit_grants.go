package permit

import (
	"context"

	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the implicit grants of a role of a resource of an environment.
const (
	opCreateImplicitGrant           = "create_implicit_grant"
	opDeleteImplicitGrant           = "delete_implicit_grant"
	opUpdateImplicitGrantConditions = "update_implicit_grants_conditions"
)

// ImplicitGrantsService creates and deletes the implicit grants of a role of a resource of an
// environment, also called role derivations, and sets their conditions; [Client.ImplicitGrants]
// holds it. An implicit grant is a rule that grants the role, such as viewer of the resource
// document, to the users that have a role on a related resource, such as owner of folder, linked
// by a relation, such as parent, as [models.DerivedRoleRuleCreate] says. The GrantedTo of a
// create or an update of a role of a resource, such as [models.ResourceRoleUpdate], sends such
// rules as a whole; this service sends one rule at a time.
//
// Each method takes the key or the ID of the resource first, such as document, and then the key
// or the ID of the role of the resource that the rule grants, such as viewer, in the order in
// which the route names them. The call sends each as one segment of the path, escaped: a key that
// holds a / or a % names that resource or that role, not another route.
//
// Its calls act on the environment of the client's scope, whose project and environment fill the
// route: those that [WithScope] gives, or else those of the client's API key, which the first call
// that needs them looks up once, as [Client.Scope] says. A scope without an environment, such as
// that of a project-level API key without WithScope, is an error that matches [ErrInvalidConfig],
// and the call sends no request for its operation. Each method is sent with an
// environment-level API key of the environment, or with a key of a wider level and WithScope; the
// API answers a key that may not act on the environment with an error status, which the call
// returns as an [Error], such as one that matches [ErrForbidden].
type ImplicitGrantsService struct {
	client *Client
}

// Create creates an implicit grant of the role whose key or ID is roleKey of the resource whose
// key or ID is resourceKey, with
// POST /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/implicit_grants, and
// returns the rule as the API stores it, with the IDs of the role, the resource and the relation
// that it names. The request holds the role, the resource and the relation of rule always, and
// its When when it is set, as [models.DerivedRoleRuleCreate] says.
//
// Create is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ImplicitGrantsService] says. It is sent again only after a 429, which the API
// did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the API may have
// created the grant, so Create returns that error.
//
// Create returns an error, and no rule:
//
//   - for a scope without an environment, or a resourceKey or a roleKey that is empty, . or ..:
//     an error that matches [ErrInvalidConfig], and no request for the grant; without WithScope,
//     the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] for a 404, or one whose Detail names each field that
//     the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a rule: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ImplicitGrantsService) Create(ctx context.Context, resourceKey, roleKey string,
	rule models.DerivedRoleRuleCreate,
) (*models.DerivedRoleRuleRead, error) {
	r, err := s.client.inScope(ctx, request{
		op: opCreateImplicitGrant, target: apiTarget, params: []string{resourceKey, roleKey},
		body: rule,
	})
	if err != nil {
		return nil, err
	}
	return do[models.DerivedRoleRuleRead](ctx, s.client, r)
}

// Delete deletes the implicit grant of rule of the role whose key or ID is roleKey of the
// resource whose key or ID is resourceKey, with
// DELETE /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/implicit_grants,
// whose body holds the role, the resource and the relation of rule always, and its When when it
// is set, as [models.DerivedRoleRuleDelete] says. A 404 is an error that matches [ErrNotFound],
// not a delete that happened already: the path names the role, and the body the rule to delete,
// so a 404 can mean a mistyped key.
//
// Delete is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ImplicitGrantsService] says. Deleting a rule twice leaves it deleted, so the
// call sends its request again after a 500, 502, 503 or 504 reply or a failed connection, as
// after a 429; when an attempt that got no complete reply deleted the rule, the next attempt can
// get an error status, such as a 404, which Delete returns.
//
// Delete returns an error:
//
//   - for a scope without an environment, or a resourceKey or a roleKey that is empty, . or ..:
//     an error that matches [ErrInvalidConfig], and no request for the grant; without WithScope,
//     the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches ErrNotFound for a 404, or one whose Detail names each field that
//     the API rejects for a 422;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ImplicitGrantsService) Delete(ctx context.Context, resourceKey, roleKey string,
	rule models.DerivedRoleRuleDelete,
) error {
	r, err := s.client.inScope(ctx, request{
		op: opDeleteImplicitGrant, target: apiTarget, params: []string{resourceKey, roleKey},
		body: rule,
	})
	if err != nil {
		return err
	}
	return s.client.doNoValue(ctx, r)
}

// UpdateConditions sets the conditions of the implicit grants of the role whose key or ID is
// roleKey of the resource whose key or ID is resourceKey to settings, with PUT
// /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/implicit_grants/conditions
// and returns them as the API stores them. The request always holds NoDirectRolesOnObject, as
// [models.DerivationSettings] says: {"no_direct_roles_on_object":true} when it is true, and
// {"no_direct_roles_on_object":false} when it is false, as the PUT replaces the conditions.
//
// UpdateConditions is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [ImplicitGrantsService] says. Sending the same conditions twice sets the same
// conditions, so the call sends its request again after a 500, 502, 503 or 504 reply or a failed
// connection, as after a 429.
//
// UpdateConditions returns an error, and no conditions:
//
//   - for a scope without an environment, or a resourceKey or a roleKey that is empty, . or ..:
//     an error that matches [ErrInvalidConfig], and no request for the conditions; without
//     WithScope, the lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] for a 404, or one whose Detail names each field that
//     the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of the conditions: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ImplicitGrantsService) UpdateConditions(ctx context.Context, resourceKey, roleKey string,
	settings models.DerivationSettings,
) (*models.DerivationSettings, error) {
	r, err := s.client.inScope(ctx, request{
		op: opUpdateImplicitGrantConditions, target: apiTarget,
		params: []string{resourceKey, roleKey}, body: settings,
	})
	if err != nil {
		return nil, err
	}
	return do[models.DerivationSettings](ctx, s.client, r)
}
