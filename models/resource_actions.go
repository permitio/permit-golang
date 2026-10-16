package models

// ResourceActionRead is an action of a resource of an environment, such as read of document, as
// the API returns it.
type ResourceActionRead struct {
	// Name is the action's name.
	Name string `json:"name"`

	// Description is the action's description, or empty when it has none.
	Description string `json:"description,omitzero"`

	// Attributes are the action's attributes, its metadata, or nil when it has none.
	Attributes Attributes `json:"attributes,omitzero"`

	// Key is the action's key, a URL-friendly name such as read, which a call may give in place
	// of its ID.
	Key string `json:"key"`

	// ID is the action's ID, a UUID.
	ID string `json:"id"`

	// PermissionName is the name of the action prefixed by the resource that it acts on, as the
	// API's documentation describes it, such as document:read.
	PermissionName string `json:"permission_name"`

	// OrganizationID is the ID of the action's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the action's project.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the action's environment.
	EnvironmentID string `json:"environment_id"`

	// ResourceID is the ID of the action's resource.
	ResourceID string `json:"resource_id"`

	// CreatedAt is when the action was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the action was last changed.
	UpdatedAt Time `json:"updated_at"`

	// V1CompatPath is the v1compat_path of the reply, which the API's documentation does not
	// describe, or empty when the reply holds none.
	V1CompatPath string `json:"v1compat_path,omitzero"`

	// V1CompatIsBuiltIn is the v1compat_is_built_in of the reply, which the API's documentation
	// does not describe, or false when the reply holds none.
	V1CompatIsBuiltIn bool `json:"v1compat_is_built_in,omitzero"`

	// V1CompatName is the v1compat_name of the reply, which the API's documentation does not
	// describe, or empty when the reply holds none.
	V1CompatName string `json:"v1compat_name,omitzero"`
}

// ResourceActionCreate is an action to create on a resource. Key and Name are required, and the
// request always holds them; Description and Attributes are left out of the request when they
// are their zero value (empty for a string, nil for a map).
type ResourceActionCreate struct {
	// Key is the action's key, a URL-friendly name of letters, digits, - and _, such as read. It
	// cannot be changed later.
	Key string `json:"key"`

	// Name is the action's name.
	Name string `json:"name"`

	// Description is the action's description. Empty leaves it out.
	Description string `json:"description,omitzero"`

	// Attributes are the action's attributes, its metadata. Nil leaves them out, and an empty
	// map that is not nil sends {}.
	Attributes Attributes `json:"attributes,omitzero"`
}

// ResourceActionUpdate is a change to an action of a resource: the request holds only the fields
// that it sets, and the action keeps the others, so the zero ResourceActionUpdate sends {} and
// changes nothing.
type ResourceActionUpdate struct {
	// Name is the action's new name. Empty leaves it out, and the action keeps its name.
	Name string `json:"name,omitzero"`

	// Description is the action's new description: unset, the zero value, leaves it out, and
	// the action keeps its description; [Null] sends null, which clears it, and [Some] sends the
	// value.
	Description Optional[string] `json:"description,omitzero"`

	// Attributes are the action's new attributes, as a whole: unset leaves them out, and the
	// action keeps its attributes; [Null] sends null, which clears them, and [Some] sends the
	// map, an empty map as {}.
	Attributes Optional[Attributes] `json:"attributes,omitzero"`
}
