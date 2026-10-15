package models

import (
	"encoding/json"
	"fmt"

	"github.com/permitio/permit-golang/v2/internal/wire"
)

// RoleRead is a role of an environment, such as editor, the permissions that a user gets in a
// tenant that the user has the role in, as the API returns it.
type RoleRead struct {
	// Name is the role's name.
	Name string `json:"name"`

	// Description is the role's description, or empty when it has none.
	Description string `json:"description,omitzero"`

	// Permissions are the permissions that the role grants, each <resource>:<action>, such as
	// document:read, or nil when the reply holds none.
	Permissions []string `json:"permissions,omitzero"`

	// Attributes are the role's attributes, its metadata, with values of any JSON type, or nil
	// when the reply holds none.
	Attributes Attributes `json:"attributes,omitzero"`

	// Extends are the keys of the roles whose permissions the role inherits, or nil when the
	// reply holds none.
	Extends []string `json:"extends,omitzero"`

	// GrantedTo is how users get the role through the roles that they have on related
	// resources, or nil when the reply holds none.
	GrantedTo *DerivedRoleBlockRead `json:"granted_to,omitzero"`

	// Key is the role's key, a URL-friendly name such as editor, which a call may give in place
	// of its ID.
	Key string `json:"key"`

	// ID is the role's ID, a UUID.
	ID string `json:"id"`

	// OrganizationID is the ID of the role's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the role's project.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the role's environment.
	EnvironmentID string `json:"environment_id"`

	// CreatedAt is when the role was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the role was last changed.
	UpdatedAt Time `json:"updated_at"`

	// V1CompatSettings is the v1compat_settings of the reply, which the API's documentation does
	// not describe, or nil when the reply holds none.
	V1CompatSettings Attributes `json:"v1compat_settings,omitzero"`

	// V1CompatAttributes is the v1compat_attributes of the reply, which the API's documentation
	// does not describe, or nil when the reply holds none.
	V1CompatAttributes Attributes `json:"v1compat_attributes,omitzero"`
}

// RoleCreate is a role to create. Key and Name are required, and the request always holds them;
// each other field that is its zero value (nil for a slice, a map or a pointer, empty for a
// string) is left out of the request.
type RoleCreate struct {
	// Key is the role's key, a URL-friendly name of letters, digits, - and _, such as editor. It
	// cannot be changed later.
	Key string `json:"key"`

	// Name is the role's name.
	Name string `json:"name"`

	// Description is the role's description. Empty leaves it out.
	Description string `json:"description,omitzero"`

	// Permissions are the permissions that the role grants, each <resource>:<action>, such as
	// document:read. Nil leaves them out, and an empty slice that is not nil sends [].
	Permissions []string `json:"permissions,omitzero"`

	// Attributes are the role's attributes, its metadata, with values of any JSON type. Nil
	// leaves them out, and an empty map that is not nil sends {}.
	Attributes Attributes `json:"attributes,omitzero"`

	// Extends are the keys of the roles whose permissions the role inherits. Nil leaves them
	// out, and an empty slice that is not nil sends [].
	Extends []string `json:"extends,omitzero"`

	// GrantedTo is how users get the role through the roles that they have on related
	// resources. Nil leaves it out.
	GrantedTo *DerivedRoleBlockEdit `json:"granted_to,omitzero"`
}

// RoleUpdate is a change to a role: the request holds only the fields that it sets, and the role
// keeps the others, so the zero RoleUpdate sends {} and changes nothing. A list is sent as a
// whole, in place of the role's list: nil leaves it out, and the role keeps its list; an empty
// slice that is not nil sends [], which clears it. The SDK never sends null for a list, as the
// API reads null for a list as the list left out.
type RoleUpdate struct {
	// Name is the role's new name. Empty leaves it out, and the role keeps its name.
	Name string `json:"name,omitzero"`

	// Description is the role's new description: unset, the zero value, leaves it out, and the
	// role keeps its description; [Null] sends null, which clears it, and [Some] sends the
	// value.
	Description Optional[string] `json:"description,omitzero"`

	// Permissions are the permissions that the role grants, which replace its permissions as a
	// whole. Nil leaves them out, and an empty slice that is not nil sends [], which removes
	// every permission of the role.
	Permissions []string `json:"permissions,omitzero"`

	// Attributes are the role's new attributes, which replace its attributes as a whole: unset
	// leaves them out, and the role keeps its attributes; [Null] sends null, which clears them,
	// and [Some] sends the map, an empty map as {}.
	Attributes Optional[Attributes] `json:"attributes,omitzero"`

	// Extends are the keys of the roles whose permissions the role inherits, which replace the
	// roles that it extends as a whole. Nil leaves them out, and an empty slice that is not nil
	// sends [], so that the role extends no role.
	Extends []string `json:"extends,omitzero"`

	// GrantedTo is how users get the role through the roles that they have on related
	// resources: unset leaves it out, and the role keeps it; [Null] sends null, which clears
	// it, and [Some] sends the value.
	GrantedTo Optional[DerivedRoleBlockEdit] `json:"granted_to,omitzero"`
}

// AddRolePermissions is the body of a grant of permissions to a role, which
// RolesService.AssignPermissions and ResourceRolesService.AssignPermissions of the root package
// send.
type AddRolePermissions struct {
	// Permissions are the permissions to grant, each <resource>:<action>, such as
	// document:read. The request always holds them: each method that sends the body sends [] for
	// a nil slice.
	Permissions []string `json:"permissions"`
}

// RemoveRolePermissions is the body of a removal of permissions from a role, which
// RolesService.RemovePermissions and ResourceRolesService.RemovePermissions of the root package
// send.
type RemoveRolePermissions struct {
	// Permissions are the permissions to remove, each <resource>:<action>, such as
	// document:read. The request always holds them: each method that sends the body sends [] for
	// a nil slice.
	Permissions []string `json:"permissions"`
}

// RoleCreateBulk is a role that a bulk replace of the roles of an environment creates, or
// replaces when the environment has a role with its key. Key and Name are required, and the
// request always holds them; each other field that is its zero value (nil for a slice, a map or a
// pointer, empty for a string) is left out of the request.
type RoleCreateBulk struct {
	// Key is the role's key, a URL-friendly name of letters, digits, - and _, such as editor.
	Key string `json:"key"`

	// Name is the role's name.
	Name string `json:"name"`

	// Description is the role's description. Empty leaves it out.
	Description string `json:"description,omitzero"`

	// Permissions are the permissions that the role grants, each <resource>:<action>, such as
	// document:read. Nil leaves them out, and an empty slice that is not nil sends [].
	Permissions []string `json:"permissions,omitzero"`

	// Attributes are the role's attributes, its metadata, with values of any JSON type. Nil
	// leaves them out, and an empty map that is not nil sends {}.
	Attributes Attributes `json:"attributes,omitzero"`

	// Extends are the keys of the roles whose permissions the role inherits. Nil leaves them
	// out, and an empty slice that is not nil sends [].
	Extends []string `json:"extends,omitzero"`

	// GrantedTo is how users get the role through the roles that they have on related
	// resources. Nil leaves it out.
	GrantedTo *DerivedRoleBlockEdit `json:"granted_to,omitzero"`

	// Resource is sent in the field resource, a string that the API's documentation lists
	// without describing it. Empty leaves it out.
	Resource string `json:"resource,omitzero"`
}

// RoleCreateBulkOperation is the body of a bulk replace of the roles of an environment, which
// RolesService.BulkReplace of the root package sends.
type RoleCreateBulkOperation struct {
	// Operations are the roles to create or replace, in order. The request always holds them:
	// RolesService.BulkReplace sends [] for a nil slice.
	Operations []RoleCreateBulk `json:"operations"`
}

// RoleCreateBulkOperationResult is the reply of a bulk replace of the roles of an environment:
// the roles that it created and those that it replaced, as the API lists them.
type RoleCreateBulkOperationResult struct {
	// Created are the roles that the call created, as the reply names them, in its order.
	Created []string `json:"created"`

	// Updated are the roles that the call replaced, as the reply names them, in its order.
	Updated []string `json:"updated"`
}

// RoleList is the reply of the ancestors or the descendants of a role, which
// RolesService.Ancestors and RolesService.Descendants of the root package read. The reply holds
// the roles in roles, either as a JSON array of them or as the paginated JSON object whose data
// array holds them; decoding reads either form into Roles, as [RoleList.UnmarshalJSON] says.
type RoleList struct {
	// Roles are the roles of the reply, in its order: empty, not nil, when it holds none.
	Roles []RoleRead `json:"roles"`
}

// UnmarshalJSON sets l to the roles of data, a JSON object whose roles is a JSON array of roles
// or the paginated JSON object whose data array holds them; the counts of the paginated object
// are not kept. A roles that is left out or null, and an empty array, set Roles to an empty
// slice that is not nil. Data that is not a JSON object, and any other value of roles, such as a
// string or an object without a data array, is an error, and leaves l as it is; null leaves l as
// it is too, as encoding/json does.
func (l *RoleList) UnmarshalJSON(data []byte) error {
	if string(data) == jsonNull {
		return nil
	}
	var reply struct {
		Roles json.RawMessage `json:"roles"`
	}
	if err := wire.Decode(data, &reply); err != nil {
		return err
	}
	roles := []RoleRead{}
	if len(reply.Roles) > 0 && string(reply.Roles) != jsonNull {
		list, err := wire.DecodeList[RoleRead](reply.Roles)
		if err != nil {
			return fmt.Errorf("models: the roles of a RoleList: %w", err)
		}
		roles = list.Data
	}
	l.Roles = roles
	return nil
}
