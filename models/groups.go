package models

// GroupCreate is a group to create in an environment, in a tenant, to which users can then be
// assigned: the spec names it by the key of its instance and the key of the resource of that
// instance, group by default. GroupInstanceKey and GroupTenant are required, and the request
// always holds them; a GroupResourceTypeKey that is empty is left out of the request, and the API
// takes group, its default.
type GroupCreate struct {
	// GroupInstanceKey is the group's instance key, such as engineering, which the calls of a
	// group take in their path.
	GroupInstanceKey string `json:"group_instance_key"`

	// GroupResourceTypeKey is the key of the resource of the group's instance. Empty leaves it
	// out, and the API takes its default, group.
	GroupResourceTypeKey string `json:"group_resource_type_key,omitzero"`

	// GroupTenant is the key of the group's tenant, such as tenant-a.
	GroupTenant string `json:"group_tenant"`
}

// GroupRead is a group as the API returns it to a create and to the assignment of a user: its
// instance key, its tenant, the roles assigned to it and the IDs of its users.
type GroupRead struct {
	// AssignedRoles are the roles assigned to the group, as the reply lists them: nil when the
	// reply holds none, and an empty slice that is not nil when it holds [].
	AssignedRoles []string `json:"assigned_roles,omitzero"`

	// GroupInstanceKey is the group's instance key, such as engineering.
	GroupInstanceKey string `json:"group_instance_key"`

	// GroupResourceTypeKey is the key of the resource of the group's instance, such as group, or
	// empty when the reply leaves it out.
	GroupResourceTypeKey string `json:"group_resource_type_key,omitzero"`

	// GroupTenant is the key of the group's tenant.
	GroupTenant string `json:"group_tenant"`

	// Users are the IDs of the users in the group, each a UUID: nil when the reply holds none, and
	// an empty slice that is not nil when it holds [].
	Users []string `json:"users,omitzero"`
}

// GroupReadSchema is a group as the API returns it to a get and in a list: its instance key, its
// tenant and its ID.
type GroupReadSchema struct {
	// GroupInstanceKey is the group's instance key, such as engineering.
	GroupInstanceKey string `json:"group_instance_key"`

	// GroupResourceTypeKey is the key of the resource of the group's instance, such as group, or
	// empty when the reply leaves it out.
	GroupResourceTypeKey string `json:"group_resource_type_key,omitzero"`

	// GroupTenant is the key of the group's tenant.
	GroupTenant string `json:"group_tenant"`

	// ID is the group's ID, a UUID.
	ID string `json:"id"`
}

// GroupAssignUser names the tenant of the membership of a user in a group, as the assignment of
// the user to the group and its removal send it. Tenant is required, and the request always holds
// it.
type GroupAssignUser struct {
	// Tenant is the key of the tenant, such as tenant-a.
	Tenant string `json:"tenant"`
}

// GroupUserReadSchema is a user in a group, as the list of the users of a group returns it.
type GroupUserReadSchema struct {
	// ID is the user's ID, a UUID.
	ID string `json:"id"`

	// Key is the user's key, such as a user ID of the application's identity provider.
	Key string `json:"key"`
}

// GroupAddRole is a role on a resource instance to assign to a group or to remove from it: the
// key or the ID of the role, of its resource, of the resource instance and of the tenant. Each
// field is required, and the request always holds it.
type GroupAddRole struct {
	// Role is the key or the ID of the role, such as editor.
	Role string `json:"role"`

	// Resource is the key or the ID of the resource that the role belongs to, such as document.
	Resource string `json:"resource"`

	// ResourceInstance is the key or the ID of the resource instance that the role belongs to,
	// such as doc-1.
	ResourceInstance string `json:"resource_instance"`

	// Tenant is the key or the ID of the tenant that the role belongs to, such as tenant-a.
	Tenant string `json:"tenant"`
}

// GroupAssignment names a group to assign to another group or to remove from it.
// GroupInstanceKey is required, and the request always holds it.
type GroupAssignment struct {
	// GroupInstanceKey names the group: the API's documentation says that it takes the ID of the
	// group's resource instance, or its resource and its instance key as
	// <resource_key>:<resource_instance_key>, such as group:engineering.
	GroupInstanceKey string `json:"group_instance_key"`
}

// GroupRoleReadSchema is a role of a group, as the list of the roles of a group returns it: the
// key of the role, and the resource and the resource instance that it belongs to.
type GroupRoleReadSchema struct {
	// Key is the role's key, such as editor.
	Key string `json:"key"`

	// Resource is the resource that the role belongs to.
	Resource GroupRoleResourceReadOnly `json:"resource"`

	// ResourceInstance is the resource instance that the role belongs to.
	ResourceInstance GroupRoleResourceReadOnly `json:"resource_instance"`
}

// GroupRoleResourceReadOnly is the resource or the resource instance of a [GroupRoleReadSchema]:
// its ID and its key.
type GroupRoleResourceReadOnly struct {
	// ID is the ID of the resource or of the resource instance, a UUID.
	ID string `json:"id"`

	// Key is the key of the resource or of the resource instance, as the reply gives it.
	Key string `json:"key"`
}
