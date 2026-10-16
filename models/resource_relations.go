package models

// RelationRead is a relation of a resource, the object, to another resource, the subject, such as
// parent from document to folder, as the API returns it. A relationship tuple of the relation
// relates an instance of the subject to an instance of the object, such as
// folder:reports parent document:q3.
type RelationRead struct {
	// Description is the relation's description, or empty when it has none.
	Description string `json:"description,omitzero"`

	// SubjectResource names the subject, the related resource, such as folder: the API's
	// documentation calls it the key or the ID of the resource. SubjectResourceID is its ID.
	SubjectResource string `json:"subject_resource"`

	// Key is the relation's key, a URL-friendly name such as parent, which a call may give in
	// place of its ID.
	Key string `json:"key"`

	// Name is the relation's name.
	Name string `json:"name"`

	// ID is the relation's ID, a UUID.
	ID string `json:"id"`

	// OrganizationID is the ID of the relation's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the relation's project.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the relation's environment.
	EnvironmentID string `json:"environment_id"`

	// CreatedAt is when the relation was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the relation was last changed.
	UpdatedAt Time `json:"updated_at"`

	// ObjectResourceID is the ID of the object, the resource that has the relation.
	ObjectResourceID string `json:"object_resource_id"`

	// ObjectResource is the key of the object, the resource that has the relation, such as
	// document.
	ObjectResource string `json:"object_resource"`

	// SubjectResourceID is the ID of the subject, the related resource.
	SubjectResourceID string `json:"subject_resource_id"`
}

// RelationCreate is a relation to create in a resource, the object, to another resource, the
// subject. Key, Name and SubjectResource are required, and the request always holds them; a
// Description that is empty is left out of the request.
type RelationCreate struct {
	// Key is the relation's key, a URL-friendly name of letters, digits, - and _, such as
	// parent.
	Key string `json:"key"`

	// Name is the relation's name.
	Name string `json:"name"`

	// Description is the relation's description. Empty leaves it out.
	Description string `json:"description,omitzero"`

	// SubjectResource is the key or the ID of the subject, the related resource, such as folder.
	SubjectResource string `json:"subject_resource"`
}
