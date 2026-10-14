package permit

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"

	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/transport"
	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/internal/wire"
)

// The IDs of the operations of the PDP that list who may do what: the permissions of a user, and
// the users that may do an action on a resource.
const (
	opUserPermissions = "Get_User_Permissions_user_permissions_post"
	opAuthorizedUsers = "authorized_users_authorized_users_post"
)

// PermissionsOption changes a call of [Client.UserPermissions]. [WithTenants], [WithResources]
// and [WithResourceTypes] build the options that filter the permissions that it returns, which
// no other method takes, so that a filter given to another method does not compile. A
// [CheckOption], such as one of [WithDecisionContext], is a PermissionsOption too, which gives
// the call its context. Only the options of this package implement it. A nil PermissionsOption,
// a nil *CheckOption, the zero CheckOption and a filter without keys are not valid: a call that
// gets one returns an error that matches [ErrInvalidConfig] and sends no request.
type PermissionsOption interface {
	// applyPermissions sets what the option changes in s, or returns the rule that the option
	// breaks and false, and changes nothing, when it is not valid.
	applyPermissions(s *permissionsSettings) (validate.Rule, bool)
}

// permissionsSettings holds what the options of a call of UserPermissions set.
type permissionsSettings struct {
	// checkSettings holds the context of the call, which a CheckOption sets.
	checkSettings

	// tenants, resources and resourceTypes are the keys of the filters of the call, each nil
	// when no option filters by it.
	tenants, resources, resourceTypes []string
}

// applyPermissions makes a CheckOption a PermissionsOption: it sets the context of a call of
// UserPermissions, as it sets the context of the checks of a call, and breaks
// validate.DecisionOptionZero when it is the zero CheckOption.
func (o CheckOption) applyPermissions(s *permissionsSettings) (validate.Rule, bool) {
	if o.apply == nil {
		return validate.DecisionOptionZero, false
	}
	o.apply(&s.checkSettings)
	return validate.Rule{}, true
}

// permissionsFilter is a PermissionsOption that asks for the permissions of some keys only: of
// tenants, of resource instances or of resource types.
type permissionsFilter struct {
	// keys are the keys of the filter.
	keys []string

	// field returns the list of s that the filter sets.
	field func(s *permissionsSettings) *[]string
}

// applyPermissions sets the list of s that f sets to the keys of f, and breaks
// validate.DecisionFilterEmpty when f has no keys.
func (f permissionsFilter) applyPermissions(s *permissionsSettings) (validate.Rule, bool) {
	if len(f.keys) == 0 {
		return validate.DecisionFilterEmpty, false
	}
	*f.field(s) = f.keys
	return validate.Rule{}, true
}

// WithTenants asks [Client.UserPermissions] for the permissions of the user in the tenants whose
// keys are tenants, such as tenant-a, only. Without it, the call asks about every tenant. It
// needs at least one key: WithTenants() is not valid, so that an empty list never asks about
// every tenant. When a call gets the option more than once, the last one wins.
func WithTenants(tenants ...string) PermissionsOption {
	return permissionsFilter{
		keys:  slices.Clone(tenants),
		field: func(s *permissionsSettings) *[]string { return &s.tenants },
	}
}

// WithResources asks [Client.UserPermissions] for the permissions of the user on the resource
// instances whose keys are resources, each as <type>:<key>, such as document:2026-report, only.
// Without it, the call asks about every resource instance. It needs at least one key, as
// [WithTenants] does, and the last one wins.
func WithResources(resources ...string) PermissionsOption {
	return permissionsFilter{
		keys:  slices.Clone(resources),
		field: func(s *permissionsSettings) *[]string { return &s.resources },
	}
}

// WithResourceTypes asks [Client.UserPermissions] for the permissions of the user on the
// resource types whose keys are resourceTypes, such as document, only; __tenant is the type of
// the tenants themselves, under whose keys the PDP returns the roles and the permissions of the
// user in each tenant. Without it, the call asks about every resource type. It needs at least one
// key, as [WithTenants] does, and the last one wins.
func WithResourceTypes(resourceTypes ...string) PermissionsOption {
	return permissionsFilter{
		keys:  slices.Clone(resourceTypes),
		field: func(s *permissionsSettings) *[]string { return &s.resourceTypes },
	}
}

// permissionsSettingsOf returns the settings that opts, the options of a call of
// UserPermissions, give, or the error of the first that is not valid, which names the option's
// position.
func permissionsSettingsOf(opts []PermissionsOption) (permissionsSettings, error) {
	var s permissionsSettings
	for i, opt := range opts {
		rule, ok := validate.DecisionOptionNil, false
		if p, isPointer := opt.(*CheckOption); isPointer && p == nil {
			opt = nil
		}
		if opt != nil {
			rule, ok = opt.applyPermissions(&s)
		}
		if !ok {
			setting := "UserPermissions: option " + strconv.Itoa(i+1)
			return permissionsSettings{}, &configError{setting: setting, rule: rule}
		}
	}
	return s, nil
}

// UserPermissionsRequest is the body of the request that [Client.UserPermissions] sends to the
// PDP, which the method builds from its user and its options, and which no method takes: it
// shows what each option sends. The PDP container and the cloud PDP both read each field.
type UserPermissionsRequest struct {
	// User is the user whose permissions the request asks for.
	User User `json:"user"`

	// Tenants are the keys of the tenants of [WithTenants], or nil to ask about every tenant.
	Tenants []string `json:"tenants,omitzero"`

	// Resources are the keys of the resource instances of [WithResources], or nil to ask about
	// every resource instance.
	Resources []string `json:"resources,omitzero"`

	// ResourceTypes are the keys of the resource types of [WithResourceTypes], or nil to ask
	// about every resource type.
	ResourceTypes []string `json:"resource_types,omitzero"`

	// Context is the context of the request, of [WithDecisionContext], which the PDP passes to
	// the policy: any JSON values, which reach the PDP as they are. Nil or empty sends none.
	Context map[string]any `json:"context,omitzero"`
}

// sent returns r as the body of a request holds it: with each of its maps of attributes or of
// context that is empty left out, as a nil one is, rather than sent as {}.
func (r UserPermissionsRequest) sent() UserPermissionsRequest {
	r.User.Attributes = nonEmpty(r.User.Attributes)
	r.Context = nonEmpty(r.Context)
	return r
}

// ObjectPermissions are the roles and the permissions of a user on one object: a tenant, or a
// resource instance. [Client.UserPermissions] returns them under the key of the object.
type ObjectPermissions struct {
	// Tenant is the tenant of the object: the tenant itself, or the tenant of the resource
	// instance. It is nil when the reply holds none.
	Tenant *TenantDetails `json:"tenant,omitzero"`

	// Resource is the resource instance, or nil for the permissions in a tenant.
	Resource *ResourceDetails `json:"resource,omitzero"`

	// Permissions are the permissions of the user on the object, each as <type>:<action>, such
	// as document:read. The list is empty, not nil, when the user has none.
	Permissions []string `json:"permissions"`

	// Roles are the keys of the roles of the user on the object, such as editor, or nil when the
	// reply holds none.
	Roles []string `json:"roles,omitzero"`
}

// TenantDetails is a tenant of a reply of the PDP: its key and its attributes.
type TenantDetails struct {
	// Key is the key of the tenant, such as tenant-a.
	Key string `json:"key"`

	// Attributes are the attributes of the tenant, as the PDP knows them, or nil when the reply
	// holds none. A number is a json.Number.
	Attributes map[string]any `json:"attributes,omitzero"`
}

// ResourceDetails is a resource instance of a reply of the PDP: its type, its key and its
// attributes.
type ResourceDetails struct {
	// Type is the key of the resource type, such as document.
	Type string `json:"type"`

	// Key is the key of the resource instance, such as 2026-report.
	Key string `json:"key"`

	// Attributes are the attributes of the resource instance, as the PDP knows them, or nil when
	// the reply holds none. A number is a json.Number.
	Attributes map[string]any `json:"attributes,omitzero"`
}

// permissionsKey is the key of the list of permissions in each entry of a reply of
// UserPermissions.
const permissionsKey = "permissions"

// UserPermissions returns the roles and the permissions of user that the PDP finds, each under
// the key that the PDP gives its object: __tenant:<tenant> for those in a tenant, such as
// __tenant:tenant-a, and <type>:<key> for those on a resource instance, such as
// document:2026-report. [WithTenants], [WithResources] and [WithResourceTypes] ask for those of
// some tenants, resource instances or resource types only, and with [WithDecisionContext] the
// call carries a context that the policy reads. A user without permissions gets an empty map.
//
// UserPermissions sends one request, POST /user-permissions, to the PDP at the PDP URL,
// [WithPDPURL]: the PDP container and the cloud PDP both serve it. It needs an environment-level
// API key, the key of the environment whose policy the PDP decides on. The call changes
// nothing, so it sends its request again as [Client.Check] does.
//
// UserPermissions returns every failure as an error, and never an empty map with it:
//
//   - a user without a key, a nil PermissionsOption, the zero CheckOption or a filter without
//     keys: an error that matches [ErrInvalidConfig], and no request;
//   - a reply with an error status, such as 403 or 422, or 429 after the retries: an [Error]
//     with the status and the body of the reply;
//   - a reply in 200-299 that is not a JSON object whose every member is a JSON object with one
//     key permissions, spelled so, whose value is a list of strings, such as null or the
//     permissions inside another object, or that holds a key twice: an Error that matches
//     [ErrMalformedDecision];
//   - a request that got no reply, such as one whose connection was refused: an Error whose Err
//     wraps the cause, such as a *url.Error.
func (c *Client) UserPermissions(ctx context.Context, user User, opts ...PermissionsOption,
) (map[string]ObjectPermissions, error) {
	s, err := permissionsSettingsOf(opts)
	if err != nil {
		return nil, err
	}
	if err = userError("UserPermissions: user", user); err != nil {
		return nil, err
	}
	body := UserPermissionsRequest{
		User: user, Tenants: s.tenants, Resources: s.resources, ResourceTypes: s.resourceTypes,
		Context: s.context,
	}
	op, reply, err := c.exchange(ctx, request{
		op: opUserPermissions, target: pdpTarget, body: body.sent(),
	}, true)
	if err != nil {
		return nil, err
	}
	return readPermissions(op, reply)
}

// readPermissions returns the permissions that reply, the reply in 200-299 to a call of
// UserPermissions of op, holds, by their keys: a JSON object whose every member is a JSON object
// with one key permissions, as fieldOf reads it, whose value is a list of strings. Any other
// reply, and one that holds a key twice, is an Error that matches ErrMalformedDecision. The
// other fields of each member decode as encoding/json reads them, and those that
// ObjectPermissions does not have are ignored.
func readPermissions(op rest.Op, reply transport.Reply) (map[string]ObjectPermissions, error) {
	failed := opCall(op)
	entries, ok := membersOf(reply.Body)
	if !ok {
		return nil, failed.malformedDecision(reply.Status, "the reply is not a JSON object that "+
			"maps the key of each tenant or resource instance to its permissions")
	}
	permissions := make(map[string]ObjectPermissions, len(entries))
	for i, entry := range entries {
		at := "entry " + strconv.Itoa(i+1) + " of the reply "
		p, ok := objectPermissionsOf(entry.value)
		if !ok {
			return nil, failed.malformedDecision(reply.Status, at+"is not a JSON object with "+
				"one key permissions whose value is a list of strings")
		}
		if _, twice := permissions[entry.key]; twice {
			return nil, failed.malformedDecision(reply.Status, at+"has the key of an earlier "+
				"entry")
		}
		permissions[entry.key] = p
	}
	return permissions, nil
}

// objectPermissionsOf returns the permissions that entry, a member of a reply of
// UserPermissions, holds, and false when entry is not a JSON object with one key permissions, as
// fieldOf reads it, whose value is a list of strings, or does not decode into an
// ObjectPermissions.
func objectPermissionsOf(entry json.RawMessage) (ObjectPermissions, bool) {
	fields, ok := membersOf(entry)
	if !ok {
		return ObjectPermissions{}, false
	}
	list, ok := fieldOf(fields, permissionsKey)
	var p ObjectPermissions
	if !ok || !isListOf(list, '"') || wire.Decode(entry, &p) != nil {
		return ObjectPermissions{}, false
	}
	return p, true
}

// isListOf reports whether data is a JSON array whose every item starts with first: '"' for a
// list of strings, and '{' for a list of objects.
func isListOf(data json.RawMessage, first byte) bool {
	var items []json.RawMessage
	if firstByte(data) != '[' || wire.Decode(data, &items) != nil {
		return false
	}
	for _, item := range items {
		if firstByte(item) != first {
			return false
		}
	}
	return true
}

// AuthorizedUsersRequest is the body of the request that [Client.AuthorizedUsers] sends to the
// PDP, which the method builds from its arguments and its options, and which no method takes: it
// shows what each argument sends.
type AuthorizedUsersRequest struct {
	// Action is the key of the action, such as read.
	Action string `json:"action"`

	// Resource is the resource that the request asks about.
	Resource Resource `json:"resource"`

	// Context is the context of the request, of [WithDecisionContext], which the PDP passes to
	// the policy: any JSON values, which reach the PDP as they are. Nil or empty sends none.
	Context map[string]any `json:"context,omitzero"`
}

// sent returns r as the body of a request holds it: with the tenant default for a resource
// without a tenant, and with each of its maps of attributes or of context that is empty left
// out.
func (r AuthorizedUsersRequest) sent() AuthorizedUsersRequest {
	r.Resource = r.Resource.sent()
	r.Context = nonEmpty(r.Context)
	return r
}

// AuthorizedUsersResult is what [Client.AuthorizedUsers] returns: the users that the PDP allows to
// do the action on the resource, each with the role assignments that allow it.
type AuthorizedUsersResult struct {
	// Resource is the resource that the result is about, as the PDP names it: <type>:<key> for a
	// resource instance, such as document:2026-report, and <type>:* for a resource type, such
	// as document:*.
	Resource string `json:"resource"`

	// Tenant is the key of the tenant that the result is about.
	Tenant string `json:"tenant"`

	// Users maps the key of each user that may do the action to the role assignments that allow
	// it. It is empty, not nil, when no user may.
	Users map[string][]AuthorizedUserAssignment `json:"users"`
}

// AuthorizedUserAssignment is a role assignment that allows a user of an [AuthorizedUsersResult]
// to do the action.
type AuthorizedUserAssignment struct {
	// User is the key of the user.
	User string `json:"user"`

	// Tenant is the key of the tenant of the role assignment.
	Tenant string `json:"tenant"`

	// Resource is where the role is assigned, as the PDP names it: <type>:<key> for a resource
	// instance, such as document:2026-report, and __tenant:<tenant> for a role in a tenant,
	// such as __tenant:tenant-a.
	Resource string `json:"resource"`

	// Role is the key of the role, such as editor.
	Role string `json:"role"`
}

// The keys of a reply of AuthorizedUsers that the SDK checks: the resource and the tenant of the
// result, and its users.
const (
	resourceKey = "resource"
	tenantKey   = "tenant"
	usersKey    = "users"
)

// AuthorizedUsers returns the users that the PDP allows to do action on resource, such as the
// users that may read the document 2026-report, each with the role assignments that allow it. A
// resource with a Key asks about that instance of its type, and one without about the type; a
// resource without a Tenant is asked about in the tenant default, as [Client.Check] does. With
// [WithDecisionContext], the call carries a context that the policy reads.
//
// AuthorizedUsers sends one request, POST /authorized_users, to the PDP at the PDP URL,
// [WithPDPURL]: the PDP container and the cloud PDP both serve it. It needs an environment-level
// API key. The call changes nothing, so it sends its request again as [Client.Check] does.
//
// AuthorizedUsers returns every failure as an error, and never a result without users with it:
//
//   - a resource without a type, or the zero CheckOption: an error that matches
//     [ErrInvalidConfig], and no request;
//   - a reply with an error status, such as 403 or 422, or 429 after the retries: an [Error]
//     with the status and the body of the reply;
//   - a reply in 200-299 that is not a JSON object with one key each of resource and tenant,
//     spelled so, whose values are strings, and users, whose value is a JSON object that maps
//     the key of each user, once, to a list of role assignments, each a JSON object: an Error
//     that matches [ErrMalformedDecision];
//   - a request that got no reply, such as one whose connection was refused: an Error whose Err
//     wraps the cause, such as a *url.Error.
func (c *Client) AuthorizedUsers(ctx context.Context, action string, resource Resource,
	opts ...CheckOption,
) (AuthorizedUsersResult, error) {
	s, err := settingsOf("AuthorizedUsers", opts)
	if err != nil {
		return AuthorizedUsersResult{}, err
	}
	if err = resourceError("AuthorizedUsers: resource", resource); err != nil {
		return AuthorizedUsersResult{}, err
	}
	body := AuthorizedUsersRequest{Action: action, Resource: resource, Context: s.context}
	op, reply, err := c.exchange(ctx, request{
		op: opAuthorizedUsers, target: pdpTarget, body: body.sent(),
	}, true)
	if err != nil {
		return AuthorizedUsersResult{}, err
	}
	return readAuthorizedUsers(op, reply)
}

// readAuthorizedUsers returns the result that reply, the reply in 200-299 to a call of
// AuthorizedUsers of op, holds: a JSON object with one key each of resource and tenant, as
// fieldOf reads them, whose values are strings, and users, whose value is a JSON object that maps
// the key of each user to a list of role assignments, each a JSON object. Any other reply, and
// one whose users hold a key twice, is an Error that matches ErrMalformedDecision. The fields of
// the assignments decode as encoding/json reads them, and other fields are ignored.
func readAuthorizedUsers(op rest.Op, reply transport.Reply) (AuthorizedUsersResult, error) {
	failed := opCall(op)
	fields, ok := membersOf(reply.Body)
	resource, hasResource := fieldOf(fields, resourceKey)
	tenant, hasTenant := fieldOf(fields, tenantKey)
	users, hasUsers := fieldOf(fields, usersKey)
	entries, isObject := membersOf(users)
	var result AuthorizedUsersResult
	if !ok || !hasResource || firstByte(resource) != '"' || !hasTenant ||
		firstByte(tenant) != '"' || !hasUsers || !isObject ||
		wire.Decode(reply.Body, &result) != nil {
		return AuthorizedUsersResult{}, failed.malformedDecision(reply.Status, "the reply is not "+
			"a JSON object with one key each of resource and tenant, whose values are strings, "+
			"and users, whose value is a JSON object")
	}
	seen := make(map[string]bool, len(entries))
	for i, entry := range entries {
		at := "user " + strconv.Itoa(i+1) + " of the reply "
		if !isListOf(entry.value, '{') {
			return AuthorizedUsersResult{}, failed.malformedDecision(reply.Status, at+"is not "+
				"a list of role assignments, each a JSON object")
		}
		if seen[entry.key] {
			return AuthorizedUsersResult{}, failed.malformedDecision(reply.Status, at+"has the "+
				"key of an earlier user")
		}
		seen[entry.key] = true
	}
	return result, nil
}
