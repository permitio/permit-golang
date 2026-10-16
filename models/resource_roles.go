package models

import (
	"encoding/json"
	"fmt"

	"github.com/permitio/permit-golang/v2/internal/wire"
)

// ResourceRoleCreate is a role of a resource to create. Key and Name are required, and the
// request always holds them; each other field that is its zero value (nil for a slice, a map or a
// pointer, empty for a string) is left out of the request.
type ResourceRoleCreate struct {
	// Key is the role's key, a URL-friendly name of letters, digits, - and _, such as editor. It
	// cannot be changed later.
	Key string `json:"key"`

	// Name is the role's name.
	Name string `json:"name"`

	// Description is the role's description. Empty leaves it out.
	Description string `json:"description,omitzero"`

	// Permissions are the keys of the actions of the resource that the role grants, such as
	// read. Nil leaves them out, and an empty slice that is not nil sends [].
	Permissions []string `json:"permissions,omitzero"`

	// Attributes are the role's attributes, its metadata, with values of any JSON type. Nil
	// leaves them out, and an empty map that is not nil sends {}.
	Attributes Attributes `json:"attributes,omitzero"`

	// Extends are the keys of the roles of the resource whose permissions the role inherits. Nil
	// leaves them out, and an empty slice that is not nil sends [].
	Extends []string `json:"extends,omitzero"`

	// GrantedTo is how users get the role through the roles that they have on related
	// resources. Nil leaves it out.
	GrantedTo *DerivedRoleBlockEdit `json:"granted_to,omitzero"`
}

// ResourceRoleUpdate is a change to a role of a resource: the request holds only the fields that
// it sets, so the zero ResourceRoleUpdate sends {}. A list is sent as a whole, in place of the
// role's list: nil leaves it out of the request, and an empty slice that is not nil sends [],
// which clears it. The SDK never sends null for a list.
type ResourceRoleUpdate struct {
	// Name is the role's new name. Empty leaves it out, and the role keeps its name.
	Name string `json:"name,omitzero"`

	// Description is the role's new description: unset, the zero value, leaves it out, and the
	// role keeps its description; [Null] sends null, and [Some] sends the value.
	Description Optional[string] `json:"description,omitzero"`

	// Permissions are the keys of the actions of the resource that the role grants, which
	// replace its permissions as a whole. Nil leaves them out, and an empty slice that is not nil
	// sends [], which removes every permission of the role.
	Permissions []string `json:"permissions,omitzero"`

	// Attributes are the role's new attributes, which replace its attributes as a whole: unset
	// leaves them out, and the role keeps its attributes; [Null] sends null, and [Some] sends the
	// map, an empty map as {}.
	Attributes Optional[Attributes] `json:"attributes,omitzero"`

	// Extends are the keys of the roles of the resource whose permissions the role inherits,
	// which replace the roles that it extends as a whole. Nil leaves them out, and an empty slice
	// that is not nil sends [], so that the role extends no role.
	Extends []string `json:"extends,omitzero"`

	// GrantedTo is how users get the role through the roles that they have on related
	// resources: unset leaves it out, and the role keeps it; [Null] sends null, and [Some] sends
	// the value.
	GrantedTo Optional[DerivedRoleBlockEdit] `json:"granted_to,omitzero"`
}

// ResourceRoleList is the reply of the ancestors or the descendants of a role of a resource,
// which ResourceRolesService.Ancestors and ResourceRolesService.Descendants of the root package
// read. The reply holds the roles in roles, either as a JSON array of them or as the paginated
// JSON object whose data array holds them; decoding reads either form into Roles, as
// [ResourceRoleList.UnmarshalJSON] says.
type ResourceRoleList struct {
	// Roles are the roles of the reply, in its order: empty, not nil, when it holds none.
	Roles []ResourceRoleRead `json:"roles"`
}

// UnmarshalJSON sets l to the roles of data, a JSON object whose roles is a JSON array of roles
// or the paginated JSON object whose data array holds them; the counts of the paginated object
// are not kept. A roles that is left out or null, and an empty array, set Roles to an empty
// slice that is not nil. Data that is not a JSON object, and any other value of roles, such as a
// string or an object without a data array, is an error, and leaves l as it is; null leaves l as
// it is too, as encoding/json does.
func (l *ResourceRoleList) UnmarshalJSON(data []byte) error {
	if string(data) == jsonNull {
		return nil
	}
	var reply struct {
		Roles json.RawMessage `json:"roles"`
	}
	if err := wire.Decode(data, &reply); err != nil {
		return err
	}
	roles := []ResourceRoleRead{}
	if len(reply.Roles) > 0 && string(reply.Roles) != jsonNull {
		list, err := wire.DecodeList[ResourceRoleRead](reply.Roles)
		if err != nil {
			return fmt.Errorf("models: the roles of a ResourceRoleList: %w", err)
		}
		roles = list.Data
	}
	l.Roles = roles
	return nil
}
