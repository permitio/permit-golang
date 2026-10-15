package models

// UserRead is a user of an environment, as the API returns it.
type UserRead struct {
	// Key is the user's key, such as alice, by which a check names the user, and which a call
	// may give in place of the user's ID.
	Key string `json:"key"`

	// ID is the user's ID, a UUID.
	ID string `json:"id"`

	// OrganizationID is the ID of the user's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the user's project.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the user's environment.
	EnvironmentID string `json:"environment_id"`

	// CreatedAt is when the user was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the user was last changed.
	UpdatedAt Time `json:"updated_at"`

	// Email is the user's email address, or empty when the user has none.
	Email string `json:"email,omitzero"`

	// FirstName is the user's first name, or empty when the user has none.
	FirstName string `json:"first_name,omitzero"`

	// LastName is the user's last name, or empty when the user has none.
	LastName string `json:"last_name,omitzero"`

	// Attributes are the user's attributes, which attribute-based policies read: nil when the
	// reply holds null or none, and an empty map that is not nil when it holds {}.
	Attributes Attributes `json:"attributes,omitzero"`

	// AssociatedTenants are the tenants in which the user has a role, each with the user's
	// roles in it and on its resource instances.
	AssociatedTenants []UserInTenant `json:"associated_tenants,omitzero"`
}

// UserInTenant is a tenant in which a user has a role, as [UserRead] lists it.
type UserInTenant struct {
	// Tenant is the tenant's key.
	Tenant string `json:"tenant"`

	// Roles are the keys of the user's roles in the tenant.
	Roles []string `json:"roles"`

	// Status is the user's status in the tenant.
	Status UserStatus `json:"status"`

	// ResourceInstanceRoles are the user's roles on the resource instances of the tenant. A list
	// of users does not ask for them, so they may be empty in a user that a list returns; a get
	// of the user returns them.
	ResourceInstanceRoles []UserResourceInstanceRole `json:"resource_instance_roles,omitzero"`
}

// UserResourceInstanceRole is a role of a user on a resource instance, as [UserInTenant] lists
// it.
type UserResourceInstanceRole struct {
	// ResourceInstance is the instance's key.
	ResourceInstance string `json:"resource_instance"`

	// Resource is the key of the instance's resource.
	Resource string `json:"resource"`

	// Role is the key of the role.
	Role string `json:"role"`
}

// UserStatus is the status of a user in a tenant. It is an open enum, as [AttributeType]
// describes.
type UserStatus string

// The values of UserStatus that this version of the SDK knows.
const (
	// UserStatusActive is active.
	UserStatusActive UserStatus = "active"

	// UserStatusPending is pending.
	UserStatusPending UserStatus = "pending"
)

// Known reports whether s is one of the constants of UserStatus, the values that this version
// of the SDK knows.
func (s UserStatus) Known() bool {
	switch s {
	case UserStatusActive, UserStatusPending:
		return true
	default:
		return false
	}
}

// UserCreate is a user to create, or the whole user that a sync sends in place of the stored
// one. Key is required; each other field that is its zero value (empty for a string, nil for a
// map or a slice) is left out of the request.
type UserCreate struct {
	// Key is the user's key, such as alice: letters, digits and the characters | @ + - . _, the
	// key by which a check names the user. It cannot be changed later.
	Key string `json:"key"`

	// Email is the user's email address. Empty leaves it out.
	Email string `json:"email,omitzero"`

	// FirstName is the user's first name. Empty leaves it out.
	FirstName string `json:"first_name,omitzero"`

	// LastName is the user's last name. Empty leaves it out.
	LastName string `json:"last_name,omitzero"`

	// Attributes are the user's attributes. Nil leaves them out; an empty map that is not nil
	// sends {}.
	Attributes Attributes `json:"attributes,omitzero"`

	// RoleAssignments are roles to assign to the user, each in a tenant or on a resource
	// instance. Nil leaves them out, and an empty slice that is not nil sends [].
	RoleAssignments []UserRoleCreate `json:"role_assignments,omitzero"`
}

// UserUpdate is a change to a user: the request holds only the fields that it sets, and the
// user keeps the others, so the zero UserUpdate changes nothing. Each field is an [Optional]:
// unset, the zero value, leaves the field out; [Null] sends null; and [Some] sends the value.
type UserUpdate struct {
	// Email is the user's new email address; Null removes it.
	Email Optional[string] `json:"email,omitzero"`

	// FirstName is the user's new first name; Null removes it.
	FirstName Optional[string] `json:"first_name,omitzero"`

	// LastName is the user's new last name; Null removes it.
	LastName Optional[string] `json:"last_name,omitzero"`

	// Attributes are the user's new attributes, which replace the user's attributes as a whole:
	// Null sends null, and Some of an empty map sends {}, which removes every attribute.
	Attributes Optional[Attributes] `json:"attributes,omitzero"`
}

// UserRoleCreate is a role to assign to a user: a role in a tenant, or a role of a resource on
// one of its instances. Role is required; each other field that is empty is left out of the
// request.
type UserRoleCreate struct {
	// Role is the key or the ID of the role, or of the resource's role for an instance.
	Role string `json:"role"`

	// Tenant is the key or the ID of the tenant. Empty leaves it out.
	Tenant string `json:"tenant,omitzero"`

	// ResourceInstance is the instance as <resource>:<key>, such as document:doc-1, or its ID,
	// for a role on the instance. Empty leaves it out, for a role in the tenant.
	ResourceInstance string `json:"resource_instance,omitzero"`
}
