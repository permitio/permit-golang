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
