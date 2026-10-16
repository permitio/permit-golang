package models

// RelationshipTupleRead is a relationship tuple, as the API returns it: the subject, a resource
// instance, has the relation to the object, another resource instance, such as folder:f1 parent
// document:doc-1, in a tenant.
type RelationshipTupleRead struct {
	// Subject names the subject, the resource instance that has the relation, as
	// <resource>:<key>, such as folder:f1.
	Subject string `json:"subject"`

	// Relation is the key of the relation, such as parent.
	Relation string `json:"relation"`

	// Object names the object, the resource instance that the subject has the relation to, as
	// <resource>:<key>, such as document:doc-1.
	Object string `json:"object"`

	// ID is the relationship tuple's ID, a UUID.
	ID string `json:"id"`

	// Tenant is the key of the tenant of the relationship tuple.
	Tenant string `json:"tenant"`

	// SubjectID is the ID of the subject, a UUID.
	SubjectID string `json:"subject_id"`

	// RelationID is the ID of the relation, a UUID.
	RelationID string `json:"relation_id"`

	// ObjectID is the ID of the object, a UUID, or is empty when the reply has none.
	ObjectID string `json:"object_id,omitzero"`

	// TenantID is the ID of the tenant, a UUID.
	TenantID string `json:"tenant_id"`

	// OrganizationID is the ID of the relationship tuple's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the relationship tuple's project.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the relationship tuple's environment.
	EnvironmentID string `json:"environment_id"`

	// CreatedAt is when the relationship tuple was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the relationship tuple was last changed.
	UpdatedAt Time `json:"updated_at"`

	// SubjectDetails is the subject as an object, or nil when the reply has none.
	SubjectDetails *ResourceInstanceBlockRead `json:"subject_details,omitzero"`

	// RelationDetails is the relation as an object, or nil when the reply has none.
	RelationDetails *StrippedRelationBlockRead `json:"relation_details,omitzero"`

	// ObjectDetails is the object as an object, or nil when the reply has none.
	ObjectDetails *ResourceInstanceBlockRead `json:"object_details,omitzero"`

	// TenantDetails is the tenant as an object, or nil when the reply has none.
	TenantDetails *TenantBlockRead `json:"tenant_details,omitzero"`
}

// RelationshipTupleDetailedRead is a relationship tuple as the detailed list of the API returns
// it, which RelationshipTuplesService.ListDetailed of the root package reads. The API's spec
// gives it the fields of a [RelationshipTupleRead], so it has them, with the details of its
// subject, its relation, its object and its tenant when the reply holds them.
type RelationshipTupleDetailedRead RelationshipTupleRead

// ResourceInstanceBlockRead is a resource instance in the details of a relationship tuple, its
// subject or its object.
type ResourceInstanceBlockRead struct {
	// Key is the instance's key, such as doc-1.
	Key string `json:"key"`

	// Tenant is the key of the instance's tenant.
	Tenant string `json:"tenant"`

	// Resource is the key of the instance's resource, such as document.
	Resource string `json:"resource"`

	// Attributes are the instance's attributes, or nil when the reply has none.
	Attributes Attributes `json:"attributes,omitzero"`
}

// StrippedRelationBlockRead is the relation in the details of a relationship tuple.
type StrippedRelationBlockRead struct {
	// Key is the relation's key, such as parent.
	Key string `json:"key"`

	// Name is the relation's name.
	Name string `json:"name"`

	// Description is the relation's description, or empty when the reply has none.
	Description string `json:"description,omitzero"`
}

// TenantBlockRead is the tenant in the details of a relationship tuple.
type TenantBlockRead struct {
	// Key is the tenant's key, such as tenant-a.
	Key string `json:"key"`

	// Name is the tenant's name.
	Name string `json:"name"`

	// Description is the tenant's description, or empty when the reply has none.
	Description string `json:"description,omitzero"`

	// Attributes are the tenant's attributes, or nil when the reply has none.
	Attributes Attributes `json:"attributes,omitzero"`
}

// RelationshipTupleCreate is a relationship tuple to create: the subject gets the relation to the
// object. Subject, Relation and Object are required, and the request always holds them, also
// when they are empty; Tenant is left out of the request when it is empty.
type RelationshipTupleCreate struct {
	// Subject is the subject, the resource instance that gets the relation, as
	// <resource>:<key>, such as folder:f1, or its ID.
	Subject string `json:"subject"`

	// Relation is the key of the relation, such as parent, a relation of the object's resource
	// whose subjects are instances of the subject's resource.
	Relation string `json:"relation"`

	// Object is the object, the resource instance that the subject gets the relation to, as
	// <resource>:<key>, such as document:doc-1, or its ID.
	Object string `json:"object"`

	// Tenant is the key or the ID of the tenant of the subject and the object. The API's
	// documentation says that it needs the tenant when neither instance exists, and creates
	// both in it, that it rejects a tenant other than that of the one instance that exists, and
	// that it ignores the tenant when both exist. Empty leaves it out, rather than sending an
	// empty string.
	Tenant string `json:"tenant,omitzero"`
}

// RelationshipTupleDelete names a relationship tuple to delete by its subject, its relation and
// its object, which are all required: the request always holds them, also when they are empty.
type RelationshipTupleDelete struct {
	// Subject is the subject, as <resource>:<key>, such as folder:f1, or its ID.
	Subject string `json:"subject"`

	// Relation is the key of the relation, such as parent.
	Relation string `json:"relation"`

	// Object is the object, as <resource>:<key>, such as document:doc-1, or its ID.
	Object string `json:"object"`
}

// RelationshipTupleCreateBulkOperation is the body of a bulk create of relationship tuples,
// which RelationshipTuplesService.BulkCreate of the root package sends.
type RelationshipTupleCreateBulkOperation struct {
	// Operations are the relationship tuples to create, in order, at most 1000, as the API's
	// spec says. The request always holds them: RelationshipTuplesService.BulkCreate sends []
	// for a nil slice.
	Operations []RelationshipTupleCreate `json:"operations"`
}

// RelationshipTupleDeleteBulkOperation is the body of a bulk delete of relationship tuples,
// which RelationshipTuplesService.BulkDelete of the root package sends.
type RelationshipTupleDeleteBulkOperation struct {
	// Idents name the relationship tuples to delete, in order, at most 1000, as the API's spec
	// says. The request always holds them: RelationshipTuplesService.BulkDelete sends [] for a
	// nil slice.
	Idents []RelationshipTupleDelete `json:"idents"`
}
