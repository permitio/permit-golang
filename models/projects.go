package models

// ProjectRead is a project of the organization, as the API returns it.
type ProjectRead struct {
	// Key is the project's key, a URL-friendly name such as my-project, which a call may give in
	// place of its ID.
	Key string `json:"key"`

	// URNNamespace is the namespace of the project's URNs, or empty when they are made from its
	// key.
	URNNamespace string `json:"urn_namespace,omitzero"`

	// ID is the project's ID, a UUID.
	ID string `json:"id"`

	// OrganizationID is the ID of the project's organization.
	OrganizationID string `json:"organization_id"`

	// CreatedAt is when the project was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the project was last changed.
	UpdatedAt Time `json:"updated_at"`

	// Name is the project's name.
	Name string `json:"name"`

	// Description is the project's description, or empty when it has none.
	Description string `json:"description,omitzero"`

	// Settings are the project's settings, or nil when the reply holds none.
	Settings Attributes `json:"settings,omitzero"`

	// ActivePolicyRepoID is the ID of the policy Git repository that the project uses, or empty
	// when it uses none.
	ActivePolicyRepoID string `json:"active_policy_repo_id,omitzero"`
}

// ProjectCreate is a project to create. Key and Name are required; each other field that is its
// zero value (nil for a slice or a map, empty for a string) is left out of the request.
type ProjectCreate struct {
	// Key is the project's key, a URL-friendly name of letters, digits, - and _, such as
	// my-project. It cannot be changed later.
	Key string `json:"key"`

	// URNNamespace is the namespace of the project's URNs, of at least two lower-case letters,
	// digits or -. Empty leaves it out, and the API makes the URNs from the key.
	URNNamespace string `json:"urn_namespace,omitzero"`

	// Name is the project's name.
	Name string `json:"name"`

	// Description is the project's description. Empty leaves it out.
	Description string `json:"description,omitzero"`

	// Settings are the project's settings. Nil leaves them out.
	Settings Attributes `json:"settings,omitzero"`

	// ActivePolicyRepoID is the ID of the policy Git repository that the project uses. Empty
	// leaves it out.
	ActivePolicyRepoID string `json:"active_policy_repo_id,omitzero"`

	// InitialEnvironments are the environments that the API creates in the new project. Nil
	// leaves the field out, and the API creates two: Development, with the key dev, and
	// Production, with the key production. An empty slice that is not nil sends [], an empty
	// list, for a project whose environments you create yourself.
	InitialEnvironments []EnvironmentCreate `json:"initial_environments,omitzero"`
}

// ProjectUpdate is a change to a project: the request holds only the fields that it sets, and
// the project keeps the others.
type ProjectUpdate struct {
	// Name is the project's new name. Empty leaves it out, and the project keeps its name.
	Name string `json:"name,omitzero"`

	// Description is the project's new description: unset, the zero value, leaves it out, and
	// the project keeps its description; [Null] sends null, and [Some] sends the value.
	Description Optional[string] `json:"description,omitzero"`

	// Settings are the project's new settings. Nil leaves them out, and the project keeps its
	// settings; an empty map that is not nil sends {}.
	Settings Attributes `json:"settings,omitzero"`

	// ActivePolicyRepoID is the ID of the policy Git repository that the project is to use.
	// Empty leaves it out, and the project keeps the one it uses.
	ActivePolicyRepoID string `json:"active_policy_repo_id,omitzero"`
}
