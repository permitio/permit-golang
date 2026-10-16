package models

// ResourceRead is a resource of an environment, a type of object that permissions protect, such
// as document, as the API returns it.
type ResourceRead struct {
	// Key is the resource's key, a URL-friendly name such as document, which a call may give in
	// place of its ID.
	Key string `json:"key"`

	// ID is the resource's ID, a UUID.
	ID string `json:"id"`

	// OrganizationID is the ID of the resource's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the resource's project.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the resource's environment.
	EnvironmentID string `json:"environment_id"`

	// CreatedAt is when the resource was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the resource was last changed.
	UpdatedAt Time `json:"updated_at"`

	// Name is the resource's name.
	Name string `json:"name"`

	// URN is the resource's Uniform Resource Name, or empty when the reply holds none.
	URN string `json:"urn,omitzero"`

	// Description is the resource's description, or empty when it has none.
	Description string `json:"description,omitzero"`

	// Actions are the actions of the resource, such as read, by their keys, or nil when the reply
	// holds none.
	Actions map[string]ActionBlockRead `json:"actions,omitzero"`

	// TypeAttributes are the resource's own attributes, its metadata, or nil when the reply holds
	// none.
	TypeAttributes Attributes `json:"type_attributes,omitzero"`

	// Attributes are the attributes that each instance of the resource has, such as owner, by
	// their keys, or nil when the reply holds none.
	Attributes map[string]AttributeBlockRead `json:"attributes,omitzero"`

	// Roles are the roles of the resource, by their keys, or nil when the reply holds none.
	Roles map[string]ResourceRoleRead `json:"roles,omitzero"`

	// Relations are the relations of the resource to other resources, by their keys, or nil
	// when the reply holds none.
	Relations map[string]RelationBlockRead `json:"relations,omitzero"`

	// V1CompatPath is the v1compat_path of the reply, which the API's documentation does not
	// describe, or empty when the reply holds none.
	V1CompatPath string `json:"v1compat_path,omitzero"`

	// V1CompatType is the v1compat_type of the reply, which the API's documentation does not
	// describe, or empty when the reply holds none.
	V1CompatType string `json:"v1compat_type,omitzero"`

	// V1CompatName is the v1compat_name of the reply, which the API's documentation does not
	// describe, or empty when the reply holds none.
	V1CompatName string `json:"v1compat_name,omitzero"`

	// ActionGroups are the action groups of the resource, each the keys of its actions, by the
	// keys of the groups, or nil when the reply holds none.
	ActionGroups map[string][]string `json:"action_groups,omitzero"`
}

// ResourceCreate is a resource to create. Key, Name and Actions are required, and the request
// always holds them; each other field that is its zero value (nil for a map, empty for a string)
// is left out of the request.
type ResourceCreate struct {
	// Key is the resource's key, a URL-friendly name of letters, digits, - and _, such as
	// document. It cannot be changed later.
	Key string `json:"key"`

	// Name is the resource's name.
	Name string `json:"name"`

	// URN is the resource's Uniform Resource Name. Empty leaves it out.
	URN string `json:"urn,omitzero"`

	// Description is the resource's description. Empty leaves it out.
	Description string `json:"description,omitzero"`

	// Actions are the actions of the resource, such as read, by their keys. The request always
	// holds them: an empty map that is not nil sends {}, and nil sends null.
	Actions map[string]ActionBlockEditable `json:"actions"`

	// TypeAttributes are the resource's own attributes, its metadata. Nil leaves them out.
	TypeAttributes Attributes `json:"type_attributes,omitzero"`

	// Attributes are the attributes that each instance of the resource has, by their keys. Nil
	// leaves them out.
	Attributes map[string]AttributeBlockEditable `json:"attributes,omitzero"`

	// Roles are the roles of the resource, by their keys. Nil leaves them out.
	Roles map[string]RoleBlockEditable `json:"roles,omitzero"`

	// Relations are the relations of the resource, each the key of the related resource, by the
	// keys of the relations. Nil leaves them out.
	Relations map[string]string `json:"relations,omitzero"`
}

// ResourceReplace is the whole definition of a resource, which a replace sends in place of the
// one that the API stores. Name and Actions are required, and the request always holds them;
// each other field that is its zero value (nil for a map, empty for a string) is left out of the
// request.
type ResourceReplace struct {
	// Name is the resource's name.
	Name string `json:"name"`

	// URN is the resource's Uniform Resource Name. Empty leaves it out.
	URN string `json:"urn,omitzero"`

	// Description is the resource's description. Empty leaves it out.
	Description string `json:"description,omitzero"`

	// Actions are the actions of the resource, by their keys, which replace the whole set of its
	// actions. The request always holds them: an empty map that is not nil sends {}, and nil
	// sends null.
	Actions map[string]ActionBlockEditable `json:"actions"`

	// TypeAttributes are the resource's own attributes, its metadata. Nil leaves them out.
	TypeAttributes Attributes `json:"type_attributes,omitzero"`

	// Attributes are the attributes that each instance of the resource has, by their keys. Nil
	// leaves them out.
	Attributes map[string]AttributeBlockEditable `json:"attributes,omitzero"`

	// Roles are the roles of the resource, by their keys. Nil leaves them out.
	Roles map[string]RoleBlockEditable `json:"roles,omitzero"`

	// Relations are the relations of the resource, each the key of the related resource, by the
	// keys of the relations. Nil leaves them out.
	Relations map[string]string `json:"relations,omitzero"`
}

// ResourceUpdate is a change to a resource: the request holds only the fields that it sets, and
// the zero ResourceUpdate sends {}. A map field is sent as it is given, as a whole: nil leaves
// the field out, an empty map that is not nil sends {}, and a map with some entries sends those
// entries only. The SDK never sends null for a map.
type ResourceUpdate struct {
	// Name is the resource's new name. Empty leaves it out.
	Name string `json:"name,omitzero"`

	// URN is the resource's new Uniform Resource Name. Empty leaves it out.
	URN string `json:"urn,omitzero"`

	// Description is the resource's new description: unset, the zero value, leaves it out;
	// [Null] sends null, and [Some] sends the value.
	Description Optional[string] `json:"description,omitzero"`

	// Actions are the actions of the resource, by their keys. Nil leaves them out.
	Actions map[string]ActionBlockEditable `json:"actions,omitzero"`

	// TypeAttributes are the resource's own attributes, its metadata. Nil leaves them out.
	TypeAttributes Attributes `json:"type_attributes,omitzero"`

	// Attributes are the attributes that each instance of the resource has, by their keys. Nil
	// leaves them out.
	Attributes map[string]AttributeBlockEditable `json:"attributes,omitzero"`

	// Roles are the roles of the resource, by their keys. Nil leaves them out.
	Roles map[string]RoleBlockEditable `json:"roles,omitzero"`

	// Relations are the relations of the resource, each the key of the related resource, by the
	// keys of the relations. Nil leaves them out.
	Relations map[string]string `json:"relations,omitzero"`
}
