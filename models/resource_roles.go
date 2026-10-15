package models

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
