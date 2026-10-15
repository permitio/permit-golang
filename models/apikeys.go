package models

// APIKeyRead is an API key of the organization, as the API returns it. The credential that a
// call sends in its Authorization header is its Secret, which prints a marker in place of the
// credential: read it with [Secret.Value].
type APIKeyRead struct {
	// OrganizationID is the ID of the key's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the project that the key acts on, or empty when it acts on the whole
	// organization.
	ProjectID string `json:"project_id,omitzero"`

	// EnvironmentID is the ID of the environment that the key acts on, or empty when it acts on
	// the whole organization or project.
	EnvironmentID string `json:"environment_id,omitzero"`

	// ObjectType is the level of the key: an organization, a project or an environment, or empty
	// when the reply holds none.
	ObjectType MemberAccessObj `json:"object_type,omitzero"`

	// AccessLevel is what the key may do at its level, such as admin or read, or empty when the
	// reply holds none.
	AccessLevel MemberAccessLevel `json:"access_level,omitzero"`

	// OwnerType is what the key belongs to, such as a member of the organization or the
	// configuration of a PDP.
	OwnerType APIKeyOwnerType `json:"owner_type"`

	// Name is the key's name, or empty when it has none.
	Name string `json:"name,omitzero"`

	// ID is the key's ID, a UUID, which the calls of one key take.
	ID string `json:"id"`

	// Secret is the credential of the key, which prints a marker in place of the credential, or
	// the empty Secret when the reply holds none.
	Secret Secret `json:"secret,omitzero"`

	// CreatedAt is when the key was created.
	CreatedAt Time `json:"created_at"`

	// LastUsedAt is when the key was last used, or the zero Time when the reply holds none.
	LastUsedAt Time `json:"last_used_at,omitzero"`

	// Env is the environment of the key, or nil when the reply holds none.
	Env *EnvironmentRead `json:"env,omitzero"`

	// Project is the project of the key, or nil when the reply holds none.
	Project *ProjectRead `json:"project,omitzero"`
}

// APIKeyCreate is an API key to create. OrganizationID is required; each other field that is its
// zero value (empty for a string) is left out of the request, and the API gives it its default.
type APIKeyCreate struct {
	// OrganizationID is the ID of the organization of the key.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the project that the key acts on. Empty leaves it out.
	ProjectID string `json:"project_id,omitzero"`

	// EnvironmentID is the ID of the environment that the key acts on. Empty leaves it out.
	EnvironmentID string `json:"environment_id,omitzero"`

	// ObjectType is the level of the key: [MemberAccessObjOrg], [MemberAccessObjProject] or
	// [MemberAccessObjEnv]. Empty leaves it out, and the API makes an environment-level key.
	ObjectType MemberAccessObj `json:"object_type,omitzero"`

	// AccessLevel is what the key may do at its level. Empty leaves it out, and the API gives
	// the key [MemberAccessLevelAdmin].
	AccessLevel MemberAccessLevel `json:"access_level,omitzero"`

	// OwnerType is what the key belongs to. Empty leaves it out, and the API makes the key
	// [APIKeyOwnerTypeMember].
	OwnerType APIKeyOwnerType `json:"owner_type,omitzero"`

	// Name is the key's name. Empty leaves it out.
	Name string `json:"name,omitzero"`
}

// MemberAccessObj is the level that an API key acts on: the organization, a project or an
// environment. It is an open enum, as [AttributeType] describes.
type MemberAccessObj string

// The values of MemberAccessObj that this version of the SDK knows.
const (
	// MemberAccessObjOrg is the whole organization.
	MemberAccessObjOrg MemberAccessObj = "org"

	// MemberAccessObjProject is one project.
	MemberAccessObjProject MemberAccessObj = "project"

	// MemberAccessObjEnv is one environment.
	MemberAccessObjEnv MemberAccessObj = "env"
)

// Known reports whether o is one of the constants of MemberAccessObj, the values that this
// version of the SDK knows.
func (o MemberAccessObj) Known() bool {
	switch o {
	case MemberAccessObjOrg, MemberAccessObjProject, MemberAccessObjEnv:
		return true
	default:
		return false
	}
}

// MemberAccessLevel is what an API key may do at its level. It is an open enum, as
// [AttributeType] describes.
type MemberAccessLevel string

// The values of MemberAccessLevel that this version of the SDK knows.
const (
	// MemberAccessLevelAdmin is admin.
	MemberAccessLevelAdmin MemberAccessLevel = "admin"

	// MemberAccessLevelWrite is write.
	MemberAccessLevelWrite MemberAccessLevel = "write"

	// MemberAccessLevelRead is read.
	MemberAccessLevelRead MemberAccessLevel = "read"

	// MemberAccessLevelNoAccess is no_access.
	MemberAccessLevelNoAccess MemberAccessLevel = "no_access"
)

// Known reports whether l is one of the constants of MemberAccessLevel, the values that this
// version of the SDK knows.
func (l MemberAccessLevel) Known() bool {
	switch l {
	case MemberAccessLevelAdmin, MemberAccessLevelWrite, MemberAccessLevelRead,
		MemberAccessLevelNoAccess:
		return true
	default:
		return false
	}
}

// APIKeyOwnerType is what an API key belongs to. It is an open enum, as [AttributeType]
// describes.
type APIKeyOwnerType string

// The values of APIKeyOwnerType that this version of the SDK knows.
//
//nolint:gosec // G101: the kinds of owner of an API key, no credentials
const (
	// APIKeyOwnerTypePDPConfig is pdp_config, the configuration of a PDP.
	APIKeyOwnerTypePDPConfig APIKeyOwnerType = "pdp_config"

	// APIKeyOwnerTypeMember is a member of the organization.
	APIKeyOwnerTypeMember APIKeyOwnerType = "member"

	// APIKeyOwnerTypeElements is Permit Elements.
	APIKeyOwnerTypeElements APIKeyOwnerType = "elements"

	// APIKeyOwnerTypeNATSPDPConfig is nats_pdp_config.
	APIKeyOwnerTypeNATSPDPConfig APIKeyOwnerType = "nats_pdp_config"
)

// Known reports whether t is one of the constants of APIKeyOwnerType, the values that this
// version of the SDK knows.
func (t APIKeyOwnerType) Known() bool {
	switch t {
	case APIKeyOwnerTypePDPConfig, APIKeyOwnerTypeMember, APIKeyOwnerTypeElements,
		APIKeyOwnerTypeNATSPDPConfig:
		return true
	default:
		return false
	}
}
