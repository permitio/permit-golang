package models

// ResourceAttributeRead is an attribute of a resource of an environment, an attribute that each
// instance of the resource has, such as owner of document, as the API returns it.
type ResourceAttributeRead struct {
	// Type is the type of the attribute's values, such as [AttributeTypeString]. A type that this
	// version of the SDK does not know, such as one that the API adds later, decodes as it is,
	// and its Known method reports false.
	Type AttributeType `json:"type"`

	// Description is the attribute's description, or empty when it has none.
	Description string `json:"description,omitzero"`

	// Key is the attribute's key, a URL-friendly name such as owner, which a call may give in
	// place of its ID.
	Key string `json:"key"`

	// ID is the attribute's ID, a UUID.
	ID string `json:"id"`

	// ResourceID is the ID of the attribute's resource.
	ResourceID string `json:"resource_id"`

	// ResourceKey is the key of the attribute's resource.
	ResourceKey string `json:"resource_key"`

	// OrganizationID is the ID of the attribute's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the attribute's project.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the attribute's environment.
	EnvironmentID string `json:"environment_id"`

	// CreatedAt is when the attribute was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the attribute was last changed.
	UpdatedAt Time `json:"updated_at"`

	// BuiltIn reports whether the attribute is built in and managed by Permit, rather than
	// created by a call.
	BuiltIn bool `json:"built_in"`
}

// ResourceAttributeCreate is an attribute to create on a resource. Key and Type are required, and
// the request always holds them; Description is left out of the request when it is empty.
type ResourceAttributeCreate struct {
	// Key is the attribute's key, a URL-friendly name of letters, digits, - and _, such as owner.
	// It cannot be changed later.
	Key string `json:"key"`

	// Type is the type of the attribute's values, such as [AttributeTypeString]. The request
	// sends it as it is, also a type that this version of the SDK does not know.
	Type AttributeType `json:"type"`

	// Description is the attribute's description. Empty leaves it out.
	Description string `json:"description,omitzero"`
}

// ResourceAttributeUpdate is a change to an attribute of a resource: the request holds only the
// fields that it sets, and the attribute keeps the others, so the zero ResourceAttributeUpdate
// sends {} and changes nothing.
type ResourceAttributeUpdate struct {
	// Type is the new type of the attribute's values. Empty leaves it out, and the attribute
	// keeps its type.
	Type AttributeType `json:"type,omitzero"`

	// Description is the attribute's new description: unset, the zero value, leaves it out, and
	// the attribute keeps its description; [Null] sends null, which clears it, and [Some] sends
	// the value.
	Description Optional[string] `json:"description,omitzero"`
}
