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
