package models

// RoleAssignmentRead is a role assignment, a role that a user has in a tenant or on a resource
// instance, as the API returns it.
type RoleAssignmentRead struct {
	// ID is the role assignment's ID, a UUID.
	ID string `json:"id"`

	// User names the user that has the role.
	User string `json:"user"`

	// Role names the role.
	Role string `json:"role"`

	// Tenant names the tenant in which the user has the role, or is empty when the reply has
	// none.
	Tenant string `json:"tenant,omitzero"`

	// ResourceInstance names the resource instance on which the user has the role, or is empty
	// for a role in the tenant.
	ResourceInstance string `json:"resource_instance,omitzero"`

	// ResourceInstanceID is the ID of the resource instance, a UUID, or is empty for a role in the
	// tenant.
	ResourceInstanceID string `json:"resource_instance_id,omitzero"`

	// UserID is the ID of the user, a UUID.
	UserID string `json:"user_id"`

	// RoleID is the ID of the role, a UUID.
	RoleID string `json:"role_id"`

	// TenantID is the ID of the tenant, a UUID.
	TenantID string `json:"tenant_id"`

	// OrganizationID is the ID of the role assignment's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the role assignment's project.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the role assignment's environment.
	EnvironmentID string `json:"environment_id"`

	// CreatedAt is when the role was assigned.
	CreatedAt Time `json:"created_at"`
}

// RoleAssignmentCreate is a role to assign to a user: a role in a tenant, or a role of a resource
// on one of its instances. User and Role are required, and the request always holds them, also
// when they are empty; each other field that is empty is left out of the request.
type RoleAssignmentCreate struct {
	// User is the key or the ID of the user.
	User string `json:"user"`

	// Role is the key or the ID of the role, or of the resource's role for an instance.
	Role string `json:"role"`

	// Tenant is the key or the ID of the tenant. Empty leaves it out, rather than sending an
	// empty string.
	Tenant string `json:"tenant,omitzero"`

	// ResourceInstance is the instance as <resource>:<key>, such as document:doc-1, or its ID,
	// for a role on the instance. Empty leaves it out, for a role in the tenant.
	ResourceInstance string `json:"resource_instance,omitzero"`
}

// RoleAssignmentRemove is a role to remove from a user: a role in a tenant, or a role of a
// resource on one of its instances. User, Role and Tenant are required, and the request always
// holds them, also when they are empty; ResourceInstance is left out of the request when it is
// empty.
type RoleAssignmentRemove struct {
	// User is the key or the ID of the user.
	User string `json:"user"`

	// Role is the key or the ID of the role, or of the resource's role for an instance.
	Role string `json:"role"`

	// Tenant is the key or the ID of the tenant.
	Tenant string `json:"tenant"`

	// ResourceInstance is the instance as <resource>:<key>, such as document:doc-1, or its ID,
	// for a role on the instance. Empty leaves it out, for a role in the tenant.
	ResourceInstance string `json:"resource_instance,omitzero"`
}

// MissingUserPolicy is the policy of a bulk assignment of roles for a user that the environment
// does not have, which RoleAssignmentsService.BulkAssign of the root package sends in the query
// parameter missing_user_policy. It is an open enum, as [AttributeType] describes: a request
// sends a value that this version of the SDK does not know as it is.
type MissingUserPolicy string

// The values of MissingUserPolicy that this version of the SDK knows, those that the API's spec
// lists.
const (
	// MissingUserPolicyFail is fail, the default that the API's spec gives.
	MissingUserPolicyFail MissingUserPolicy = "fail"

	// MissingUserPolicyIgnore is ignore.
	MissingUserPolicyIgnore MissingUserPolicy = "ignore"

	// MissingUserPolicyCreate is create.
	MissingUserPolicyCreate MissingUserPolicy = "create"
)

// Known reports whether p is one of the constants of MissingUserPolicy, the values that this
// version of the SDK knows.
func (p MissingUserPolicy) Known() bool {
	switch p {
	case MissingUserPolicyFail, MissingUserPolicyIgnore, MissingUserPolicyCreate:
		return true
	default:
		return false
	}
}

// BulkRoleAssignmentReport is the reply of a bulk assignment of roles, which
// RoleAssignmentsService.BulkAssign of the root package returns.
type BulkRoleAssignmentReport struct {
	// AssignmentsCreated is the number of the role assignments that the call created, or 0 when
	// the reply has none.
	AssignmentsCreated int `json:"assignments_created,omitzero"`
}

// BulkRoleUnAssignmentReport is the reply of a bulk unassignment of roles, which
// RoleAssignmentsService.BulkUnassign of the root package returns.
type BulkRoleUnAssignmentReport struct {
	// AssignmentsRemoved is the number of the role assignments that the call removed, or 0 when
	// the reply has none.
	AssignmentsRemoved int `json:"assignments_removed,omitzero"`
}

// RoleAssignmentDetailedRead is a role assignment as the detailed list of the API returns it:
// its user, its role, its tenant and its resource instance each as an object with its key, its
// ID and other fields, such as its name, its attributes or its permissions, in place of the keys
// and the IDs of a [RoleAssignmentRead].
type RoleAssignmentDetailedRead struct {
	// ID is the role assignment's ID, a UUID.
	ID string `json:"id"`

	// Role is the role that the user has.
	Role RoleAssignmentRole `json:"role"`

	// User is the user that has the role.
	User RoleAssignmentUser `json:"user"`

	// Tenant is the tenant in which the user has the role.
	Tenant RoleAssignmentTenant `json:"tenant"`

	// ResourceInstance is the resource instance on which the user has the role, or nil for a
	// role in the tenant.
	ResourceInstance *RoleAssignmentResourceInstance `json:"resource_instance,omitzero"`

	// OrganizationID is the ID of the role assignment's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the role assignment's project.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the role assignment's environment.
	EnvironmentID string `json:"environment_id"`

	// CreatedAt is when the role was assigned.
	CreatedAt Time `json:"created_at"`
}

// RoleAssignmentRole is the role of a [RoleAssignmentDetailedRead].
type RoleAssignmentRole struct {
	// ID is the role's ID, a UUID.
	ID string `json:"id"`

	// Key is the role's key, such as editor.
	Key string `json:"key"`

	// Name is the role's name.
	Name string `json:"name"`

	// Permissions are the permissions that the role grants, or nil when the reply has none.
	Permissions []string `json:"permissions,omitzero"`
}

// RoleAssignmentUser is the user of a [RoleAssignmentDetailedRead].
type RoleAssignmentUser struct {
	// ID is the user's ID, a UUID.
	ID string `json:"id"`

	// Key is the user's key, such as alice.
	Key string `json:"key"`

	// Email is the user's email address, or empty when the reply has none.
	Email string `json:"email,omitzero"`

	// FirstName is the user's first name, or empty when the reply has none.
	FirstName string `json:"first_name,omitzero"`

	// LastName is the user's last name, or empty when the reply has none.
	LastName string `json:"last_name,omitzero"`

	// Attributes are the user's attributes, or nil when the reply has none.
	Attributes Attributes `json:"attributes,omitzero"`
}

// RoleAssignmentTenant is the tenant of a [RoleAssignmentDetailedRead].
type RoleAssignmentTenant struct {
	// ID is the tenant's ID, a UUID.
	ID string `json:"id"`

	// Key is the tenant's key, such as tenant-a.
	Key string `json:"key"`

	// Name is the tenant's name.
	Name string `json:"name"`

	// Attributes are the tenant's attributes, or nil when the reply has none.
	Attributes Attributes `json:"attributes,omitzero"`
}

// RoleAssignmentResourceInstance is the resource instance of a [RoleAssignmentDetailedRead].
type RoleAssignmentResourceInstance struct {
	// ID is the instance's ID, a UUID.
	ID string `json:"id"`

	// Key is the instance's key, such as doc-1.
	Key string `json:"key"`

	// Resource is the key of the instance's resource, such as document.
	Resource string `json:"resource"`

	// Attributes are the instance's attributes, or nil when the reply has none.
	Attributes Attributes `json:"attributes,omitzero"`
}
