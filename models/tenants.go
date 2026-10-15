package models

// TenantRead is a tenant of an environment, as the API returns it.
type TenantRead struct {
	// Key is the tenant's key, a URL-friendly name such as tenant-a, which a call may give in
	// place of its ID.
	Key string `json:"key"`

	// ID is the tenant's ID, a UUID.
	ID string `json:"id"`

	// OrganizationID is the ID of the tenant's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the tenant's project.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the tenant's environment.
	EnvironmentID string `json:"environment_id"`

	// CreatedAt is when the tenant was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the tenant was last changed.
	UpdatedAt Time `json:"updated_at"`

	// LastActionAt is when the tenant was last acted on.
	LastActionAt Time `json:"last_action_at"`

	// Name is the tenant's name.
	Name string `json:"name"`

	// Description is the tenant's description, or empty when it has none.
	Description string `json:"description,omitzero"`

	// Attributes are the tenant's attributes: nil when the reply holds null or none, as for a
	// tenant whose attributes an update set to null, and an empty map that is not nil when it
	// holds {}.
	Attributes Attributes `json:"attributes,omitzero"`
}

// TenantCreate is a tenant to create. Key and Name are required; each other field that is its
// zero value (empty for a string, nil for a map) is left out of the request.
type TenantCreate struct {
	// Key is the tenant's key, a URL-friendly name of letters, digits, - and _, such as
	// tenant-a. It cannot be changed later.
	Key string `json:"key"`

	// Name is the tenant's name.
	Name string `json:"name"`

	// Description is the tenant's description. Empty leaves it out.
	Description string `json:"description,omitzero"`

	// Attributes are the tenant's attributes. Nil leaves them out; an empty map that is not nil
	// sends {}.
	Attributes Attributes `json:"attributes,omitzero"`
}

// TenantUpdate is a change to a tenant: the request holds only the fields that it sets, and the
// tenant keeps the others, so the zero TenantUpdate changes nothing.
type TenantUpdate struct {
	// Name is the tenant's new name. Empty leaves it out, and the tenant keeps its name.
	Name string `json:"name,omitzero"`

	// Description is the tenant's new description: unset, the zero value, leaves it out, and the
	// tenant keeps its description; [Null] sends null, which removes it; and [Some] sends the
	// value.
	Description Optional[string] `json:"description,omitzero"`

	// Attributes are the tenant's new attributes, which replace its attributes as a whole: unset,
	// the zero value, leaves them out, and the tenant keeps its attributes; [Null] sends null,
	// which the API stores, so that a read of the tenant returns nil Attributes; and [Some] sends
	// the map, an empty map as {}, which removes every attribute.
	Attributes Optional[Attributes] `json:"attributes,omitzero"`
}
