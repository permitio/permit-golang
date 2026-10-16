package models

// ResourceInstanceRead is a resource instance of an environment, an object of a resource in a
// tenant, as the API returns it.
type ResourceInstanceRead struct {
	// Key is the instance's key, such as doc-1. A call names the instance as
	// <resource>:<key>, such as document:doc-1, or by its ID.
	Key string `json:"key"`

	// Tenant is the key of the instance's tenant.
	Tenant string `json:"tenant"`

	// Resource is the key of the instance's resource, such as document.
	Resource string `json:"resource"`

	// ID is the instance's ID, a UUID.
	ID string `json:"id"`

	// OrganizationID is the ID of the instance's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the instance's project.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the instance's environment.
	EnvironmentID string `json:"environment_id"`

	// CreatedAt is when the instance was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the instance was last changed.
	UpdatedAt Time `json:"updated_at"`

	// ResourceID is the ID of the instance's resource.
	ResourceID string `json:"resource_id"`

	// TenantID is the ID of the instance's tenant.
	TenantID string `json:"tenant_id"`

	// Attributes are the instance's attributes: nil when the reply holds null or none, and an
	// empty map that is not nil when it holds {}.
	Attributes Attributes `json:"attributes,omitzero"`

	// Relationships are the relationship tuples of the instance, when the reply holds them: nil
	// when it holds none. [ResourceInstanceDetailedRead] always holds them.
	Relationships []RelationshipTupleBlockRead `json:"relationships,omitzero"`
}

// ResourceInstanceDetailedRead is a resource instance of an environment with its relationship
// tuples, as ResourceInstancesService.ListDetailed of the root package returns it.
type ResourceInstanceDetailedRead struct {
	// Key is the instance's key, such as doc-1. A call names the instance as
	// <resource>:<key>, such as document:doc-1, or by its ID.
	Key string `json:"key"`

	// Tenant is the key of the instance's tenant.
	Tenant string `json:"tenant"`

	// Resource is the key of the instance's resource, such as document.
	Resource string `json:"resource"`

	// ID is the instance's ID, a UUID.
	ID string `json:"id"`

	// OrganizationID is the ID of the instance's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the instance's project.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the instance's environment.
	EnvironmentID string `json:"environment_id"`

	// CreatedAt is when the instance was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the instance was last changed.
	UpdatedAt Time `json:"updated_at"`

	// ResourceID is the ID of the instance's resource.
	ResourceID string `json:"resource_id"`

	// TenantID is the ID of the instance's tenant.
	TenantID string `json:"tenant_id"`

	// Attributes are the instance's attributes: nil when the reply holds null or none, and an
	// empty map that is not nil when it holds {}.
	Attributes Attributes `json:"attributes,omitzero"`

	// Relationships are the relationship tuples of the instance, each of which relates it to
	// another object: an empty slice that is not nil when the reply holds [].
	Relationships []RelationshipTupleBlockRead `json:"relationships"`
}

// RelationshipTupleBlockRead is a relationship tuple of a resource instance, as
// [ResourceInstanceDetailedRead] lists it: the subject has the relation to the object.
type RelationshipTupleBlockRead struct {
	// Subject is the instance that has the relation, as <resource>:<key>, such as folder:f1.
	Subject string `json:"subject"`

	// Relation is the key of the relation, such as parent.
	Relation string `json:"relation"`

	// Object is the instance that the subject has the relation to, as <resource>:<key>, such as
	// document:doc-1.
	Object string `json:"object"`
}

// ResourceInstanceCreate is a resource instance to create, which ResourceInstancesService.Create
// of the root package sends. Key, Tenant and Resource are required, and the request always holds
// them; Attributes are left out of the request when they are nil.
type ResourceInstanceCreate struct {
	// Key is the instance's key, such as doc-1: letters, digits, - and _. It cannot be changed
	// later.
	Key string `json:"key"`

	// Tenant is the key of the instance's tenant, such as tenant-a.
	// ResourceInstancesService.Create and ResourceInstancesService.BulkReplace send no request
	// when it is empty.
	Tenant string `json:"tenant"`

	// Resource is the key of the instance's resource, such as document.
	Resource string `json:"resource"`

	// Attributes are the instance's attributes. Nil leaves them out; an empty map that is not
	// nil sends {}.
	Attributes Attributes `json:"attributes,omitzero"`
}

// ResourceInstanceUpdate is a change to a resource instance, of its attributes alone: the zero
// ResourceInstanceUpdate changes nothing.
type ResourceInstanceUpdate struct {
	// Attributes are the instance's new attributes, which replace its attributes as a whole:
	// unset, the zero value, leaves them out, and the instance keeps its attributes; [Null]
	// sends null; and [Some] sends the map, an empty map as {}, which removes every attribute.
	Attributes Optional[Attributes] `json:"attributes,omitzero"`
}
