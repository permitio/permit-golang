package models

// APIKeyScopeRead is the scope of an API key, as the API returns it: the IDs of the
// organization, project and environment that the key acts on. An organization-level key has no
// project and no environment, and a project-level key has no environment.
type APIKeyScopeRead struct {
	// OrganizationID is the ID of the key's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the key's project, or empty for an organization-level key.
	ProjectID string `json:"project_id,omitzero"`

	// EnvironmentID is the ID of the key's environment, or empty for an organization-level or a
	// project-level key.
	EnvironmentID string `json:"environment_id,omitzero"`
}
