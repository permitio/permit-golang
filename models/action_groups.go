package models

// ResourceActionGroupRead is an action group of a resource, a named set of the resource's actions,
// such as editing for read and write, as the API returns it.
type ResourceActionGroupRead struct {
	// Name is the action group's name.
	Name string `json:"name"`

	// Description is the action group's description, or empty when it has none.
	Description string `json:"description,omitzero"`

	// Attributes are the action group's attributes, its metadata, or nil when the reply holds
	// none.
	Attributes Attributes `json:"attributes,omitzero"`

	// Actions are the keys of the actions of the resource that the group holds, such as read and
	// write: nil when the reply holds none, and an empty slice that is not nil when it holds [].
	Actions []string `json:"actions,omitzero"`

	// Key is the action group's key, a URL-friendly name such as editing, which a call may give
	// in place of its ID.
	Key string `json:"key"`

	// ID is the action group's ID, a UUID.
	ID string `json:"id"`

	// OrganizationID is the ID of the action group's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the action group's project.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the action group's environment.
	EnvironmentID string `json:"environment_id"`

	// ResourceID is the ID of the action group's resource.
	ResourceID string `json:"resource_id"`

	// CreatedAt is when the action group was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the action group was last changed.
	UpdatedAt Time `json:"updated_at"`
}

// ResourceActionGroupCreate is an action group to create in a resource. Key and Name are
// required, and the request always holds them; each other field that is its zero value (nil for
// a slice or a map, empty for a string) is left out of the request.
type ResourceActionGroupCreate struct {
	// Key is the action group's key, a URL-friendly name of letters, digits, - and _, such as
	// editing. It cannot be changed later.
	Key string `json:"key"`

	// Name is the action group's name.
	Name string `json:"name"`

	// Description is the action group's description. Empty leaves it out.
	Description string `json:"description,omitzero"`

	// Attributes are the action group's attributes, its metadata. Nil leaves them out, and an
	// empty map that is not nil sends {}.
	Attributes Attributes `json:"attributes,omitzero"`

	// Actions are the keys of the actions of the resource that the group holds, such as read and
	// write. Nil leaves them out, and the API's documentation gives an empty list as their
	// default; an empty slice that is not nil sends [].
	Actions []string `json:"actions,omitzero"`
}

// ResourceActionGroupUpdate is a change to an action group: the request holds only the fields
// that it sets, and the zero ResourceActionGroupUpdate sends {}.
//
// Set Actions in each update, to the keys of the actions that the group keeps, also when the
// update changes only another field: the API has been seen to remove every action of a group
// when an update leaves its actions out.
type ResourceActionGroupUpdate struct {
	// Name is the action group's new name. Empty leaves it out.
	Name string `json:"name,omitzero"`

	// Description is the action group's new description: unset, the zero value, leaves it out;
	// [Null] sends null, and [Some] sends the value.
	Description Optional[string] `json:"description,omitzero"`

	// Attributes are the action group's new attributes, its metadata. Nil leaves them out, and a
	// map that is not nil sends that map, {} for an empty one.
	Attributes Attributes `json:"attributes,omitzero"`

	// Actions are the keys of the actions of the resource that the group holds after the update,
	// which replace those it holds: unset, the zero value, leaves them out, as the type says;
	// [Some] sends the keys, and Some of an empty slice sends [], a group without actions; and
	// [Null] sends null.
	Actions Optional[[]string] `json:"actions,omitzero"`
}
