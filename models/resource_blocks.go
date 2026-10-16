package models

// ActionBlockRead is an action of a resource, such as read, as the API returns it in the actions
// of a [ResourceRead].
type ActionBlockRead struct {
	// Name is the action's name, or empty when it has none.
	Name string `json:"name,omitzero"`

	// Description is the action's description, or empty when it has none.
	Description string `json:"description,omitzero"`

	// Attributes are the action's attributes, or nil when the reply holds none.
	Attributes Attributes `json:"attributes,omitzero"`

	// V1CompatPath is the v1compat_path of the reply, which the API's documentation does not
	// describe, or empty when the reply holds none.
	V1CompatPath string `json:"v1compat_path,omitzero"`

	// V1CompatName is the v1compat_name of the reply, which the API's documentation does not
	// describe, or empty when the reply holds none.
	V1CompatName string `json:"v1compat_name,omitzero"`

	// ID is the action's ID, a UUID.
	ID string `json:"id"`

	// Key is the action's key, or empty when the reply holds none.
	Key string `json:"key,omitzero"`
}

// ActionBlockEditable is an action of a resource, as a create, a replace or an update of the
// resource sends it, by its key. Each field that is its zero value is left out of the request,
// so the zero ActionBlockEditable sends {}, an action with its key alone.
type ActionBlockEditable struct {
	// Name is the action's name. Empty leaves it out.
	Name string `json:"name,omitzero"`

	// Description is the action's description. Empty leaves it out.
	Description string `json:"description,omitzero"`

	// Attributes are the action's attributes. Nil leaves them out.
	Attributes Attributes `json:"attributes,omitzero"`
}

// AttributeBlockRead is an attribute that each instance of a resource has, as the API returns it
// in the attributes of a [ResourceRead].
type AttributeBlockRead struct {
	// Type is the type of the attribute's values, such as [AttributeTypeString].
	Type AttributeType `json:"type"`

	// Description is the attribute's description, or empty when it has none.
	Description string `json:"description,omitzero"`

	// ID is the attribute's ID, a UUID.
	ID string `json:"id"`

	// Key is the attribute's key, or empty when the reply holds none.
	Key string `json:"key,omitzero"`
}

// AttributeBlockEditable is an attribute that each instance of a resource has, as a create, a
// replace or an update of the resource sends it, by its key. Type is required, and the request
// always holds it.
type AttributeBlockEditable struct {
	// Type is the type of the attribute's values, such as [AttributeTypeString].
	Type AttributeType `json:"type"`

	// Description is the attribute's description. Empty leaves it out.
	Description string `json:"description,omitzero"`
}

// RelationBlockRead is a relation of a resource to another resource, as the API returns it in
// the relations of a [ResourceRead].
type RelationBlockRead struct {
	// Description is the relation's description, or empty when it has none.
	Description string `json:"description,omitzero"`

	// ResourceID is the ID of the related resource.
	ResourceID string `json:"resource_id"`

	// RelationName is the relation's name, or empty when the reply holds none.
	RelationName string `json:"relation_name,omitzero"`

	// Resource is the key of the related resource.
	Resource string `json:"resource"`
}

// ResourceRoleRead is a role of a resource, as the API returns it in the roles of a
// [ResourceRead], and as the methods of ResourceRolesService of the root package that return a
// role return it.
type ResourceRoleRead struct {
	// Name is the role's name.
	Name string `json:"name"`

	// Description is the role's description, or empty when it has none.
	Description string `json:"description,omitzero"`

	// Permissions are the keys of the actions of the resource that the role grants, or nil when
	// the reply holds none.
	Permissions []string `json:"permissions,omitzero"`

	// Attributes are the role's attributes, its metadata, or nil when the reply holds none.
	Attributes Attributes `json:"attributes,omitzero"`

	// Extends are the keys of the roles whose permissions the role inherits, or nil when the
	// reply holds none.
	Extends []string `json:"extends,omitzero"`

	// GrantedTo is how users get the role through the roles that they have on related
	// resources, or nil when the reply holds none.
	GrantedTo *DerivedRoleBlockRead `json:"granted_to,omitzero"`

	// Key is the role's key, which a call may give in place of its ID.
	Key string `json:"key"`

	// ID is the role's ID, a UUID.
	ID string `json:"id"`

	// OrganizationID is the ID of the role's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the role's project.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the role's environment.
	EnvironmentID string `json:"environment_id"`

	// ResourceID is the ID of the role's resource.
	ResourceID string `json:"resource_id"`

	// Resource is the key of the role's resource.
	Resource string `json:"resource"`

	// CreatedAt is when the role was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the role was last changed.
	UpdatedAt Time `json:"updated_at"`
}

// RoleBlockEditable is a role of a resource, as a create, a replace or an update of the resource
// sends it, by its key. Name is required, and the request always holds it; each other field that
// is its zero value (nil for a slice, a map or a pointer, empty for a string) is left out of the
// request.
type RoleBlockEditable struct {
	// Name is the role's name.
	Name string `json:"name"`

	// Description is the role's description. Empty leaves it out.
	Description string `json:"description,omitzero"`

	// Permissions are the keys of the actions of the resource that the role grants. Nil leaves
	// them out, and an empty slice that is not nil sends [].
	Permissions []string `json:"permissions,omitzero"`

	// Attributes are the role's attributes, its metadata. Nil leaves them out.
	Attributes Attributes `json:"attributes,omitzero"`

	// Extends are the keys of the roles whose permissions the role inherits. Nil leaves them
	// out, and an empty slice that is not nil sends [].
	Extends []string `json:"extends,omitzero"`

	// GrantedTo is how users get the role through the roles that they have on related
	// resources. Nil leaves it out.
	GrantedTo *DerivedRoleBlockEdit `json:"granted_to,omitzero"`
}

// DerivedRoleBlockRead is how users get a role through the roles that they have on related
// resources, as the API returns it in a [ResourceRoleRead].
type DerivedRoleBlockRead struct {
	// ID is the ID of the derivation, a UUID.
	ID string `json:"id"`

	// UsersWithRole are the rules that grant the role, or nil when the reply holds none.
	UsersWithRole []DerivedRoleRuleRead `json:"users_with_role,omitzero"`

	// When are the settings of the derivation, or nil when the reply holds none.
	When *DerivationSettings `json:"when,omitzero"`
}

// DerivedRoleBlockEdit is how users get a role through the roles that they have on related
// resources, as a [RoleBlockEditable] sends it. Each field that is its zero value is left out of
// the request.
type DerivedRoleBlockEdit struct {
	// UsersWithRole are the rules that grant the role. Nil leaves them out, and an empty slice
	// that is not nil sends [].
	UsersWithRole []DerivedRoleRuleCreate `json:"users_with_role,omitzero"`

	// When are the settings of the derivation. Nil leaves them out.
	When *DerivationSettings `json:"when,omitzero"`
}

// DerivedRoleRuleRead is a rule that grants a role to the users that have a role on a related
// resource, as the API returns it in a [DerivedRoleBlockRead], and as
// ImplicitGrantsService.Create of the root package returns it.
type DerivedRoleRuleRead struct {
	// RoleID is the ID of the role that a user has on the related resource.
	RoleID string `json:"role_id"`

	// ResourceID is the ID of the related resource.
	ResourceID string `json:"resource_id"`

	// RelationID is the ID of the relation to the related resource.
	RelationID string `json:"relation_id"`

	// Role is the key of the role that a user has on the related resource.
	Role string `json:"role"`

	// OnResource is the key of the related resource.
	OnResource string `json:"on_resource"`

	// LinkedByRelation is the key of the relation to the related resource.
	LinkedByRelation string `json:"linked_by_relation"`

	// When are the settings of the rule, or nil when the reply holds none.
	When *DerivationSettings `json:"when,omitzero"`
}

// DerivedRoleRuleCreate is a rule that grants a role to the users that have a role on a related
// resource, as a [DerivedRoleBlockEdit] sends it, and as ImplicitGrantsService.Create of the root
// package sends it. Role, OnResource and LinkedByRelation are required, and the request always
// holds them.
type DerivedRoleRuleCreate struct {
	// Role is the key of the role that a user has on the related resource.
	Role string `json:"role"`

	// OnResource is the key of the related resource.
	OnResource string `json:"on_resource"`

	// LinkedByRelation is the key of the relation to the related resource.
	LinkedByRelation string `json:"linked_by_relation"`

	// When are the settings of the rule. Nil leaves them out.
	When *DerivationSettings `json:"when,omitzero"`
}

// DerivationSettings are the settings of how users get a role through the roles that they have on
// related resources, or of one rule of it, and the conditions of the implicit grants of a role,
// which ImplicitGrantsService.UpdateConditions of the root package sends and returns.
type DerivationSettings struct {
	// NoDirectRolesOnObject, when true, says that the derivation, or the rule, does not apply to
	// an instance that has a role assigned on it directly. A request always holds it, false too.
	NoDirectRolesOnObject bool `json:"no_direct_roles_on_object"`
}
