package models

// UserInviteStatus is the status of a user invite. It is an open enum, as [AttributeType]
// describes.
type UserInviteStatus string

// The values of UserInviteStatus that this version of the SDK knows.
const (
	// UserInviteStatusPending is pending.
	UserInviteStatusPending UserInviteStatus = "pending"

	// UserInviteStatusApproved is approved.
	UserInviteStatusApproved UserInviteStatus = "approved"
)

// Known reports whether s is one of the constants of UserInviteStatus, the values that this
// version of the SDK knows.
func (s UserInviteStatus) Known() bool {
	switch s {
	case UserInviteStatusPending, UserInviteStatusApproved:
		return true
	default:
		return false
	}
}

// ElementsUserInviteCreate is an invite of a user to an environment to create: the key, the email
// address and the name of the user, the role, the tenant and the resource instance of the invite,
// each by its ID, and the status of the invite. Each field is required, and the request always
// holds it, also when it is empty.
type ElementsUserInviteCreate struct {
	// Key is the key of the invited user, such as alice@example.com: letters, digits and the
	// characters | @ + - . _.
	Key string `json:"key"`

	// Status is the status of the invite, such as UserInviteStatusPending.
	Status UserInviteStatus `json:"status"`

	// Email is the email address of the invited user.
	Email string `json:"email"`

	// FirstName is the first name of the invited user.
	FirstName string `json:"first_name"`

	// LastName is the last name of the invited user.
	LastName string `json:"last_name"`

	// RoleID is the ID of the role of the invited user, a UUID.
	RoleID string `json:"role_id"`

	// TenantID is the ID of the tenant of the invited user, a UUID.
	TenantID string `json:"tenant_id"`

	// ResourceInstanceID is the ID of the resource instance of the invited user, a UUID. The SDK
	// always sends it as a string, also when it is empty, so an invite without a resource
	// instance, one whose target is a tenant, cannot be created through the SDK yet; whether the
	// API takes null here is not confirmed yet.
	ResourceInstanceID string `json:"resource_instance_id"`
}

// ElementsUserInviteUpdate is the new content of a user invite, as an update sends it. The spec
// requires each field of an update, as it does of a create, so the request always holds each
// field, also when it is empty: an update sends the whole invite, and leaves no field out.
type ElementsUserInviteUpdate struct {
	// Key is the key of the invited user, such as alice@example.com: letters, digits and the
	// characters | @ + - . _.
	Key string `json:"key"`

	// Status is the status of the invite, such as UserInviteStatusPending.
	Status UserInviteStatus `json:"status"`

	// Email is the email address of the invited user.
	Email string `json:"email"`

	// FirstName is the first name of the invited user.
	FirstName string `json:"first_name"`

	// LastName is the last name of the invited user.
	LastName string `json:"last_name"`

	// RoleID is the ID of the role of the invited user, a UUID.
	RoleID string `json:"role_id"`

	// TenantID is the ID of the tenant of the invited user, a UUID.
	TenantID string `json:"tenant_id"`

	// ResourceInstanceID is the ID of the resource instance of the invited user, a UUID. The SDK
	// always sends it as a string, also when it is empty, so an invite without a resource
	// instance, one whose target is a tenant, cannot be updated through the SDK yet; whether the
	// API takes null here is not confirmed yet.
	ResourceInstanceID string `json:"resource_instance_id"`
}

// ElementsUserInviteRead is a user invite of an environment, as the API returns it.
type ElementsUserInviteRead struct {
	// ID is the invite's ID, a UUID.
	ID string `json:"id"`

	// OrganizationID is the ID of the invite's organization.
	OrganizationID string `json:"organization_id"`

	// ProjectID is the ID of the invite's project.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the invite's environment.
	EnvironmentID string `json:"environment_id"`

	// CreatedAt is when the invite was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the invite was last changed.
	UpdatedAt Time `json:"updated_at"`

	// Key is the key of the invited user.
	Key string `json:"key"`

	// Status is the status of the invite.
	Status UserInviteStatus `json:"status"`

	// Email is the email address of the invited user.
	Email string `json:"email"`

	// FirstName is the first name of the invited user.
	FirstName string `json:"first_name"`

	// LastName is the last name of the invited user.
	LastName string `json:"last_name"`

	// RoleID is the ID of the role of the invited user.
	RoleID string `json:"role_id"`

	// TenantID is the ID of the tenant of the invited user.
	TenantID string `json:"tenant_id"`

	// ResourceInstanceID is the ID of the resource instance of the invited user.
	ResourceInstanceID string `json:"resource_instance_id"`
}

// ElementsUserInviteApprove is the approval of a user invite: the email address and the key of
// the invited user, as the invite holds them, and the attributes of the user. Each field is
// required, and the request always holds it: UserInvitesService.Approve of the root package sends
// {} for nil Attributes.
type ElementsUserInviteApprove struct {
	// Email is the email address of the invited user. The API's documentation says that the API
	// answers 400 when it is not the email address of the invite.
	Email string `json:"email"`

	// Key is the key of the invite, the key of the invited user that the invite holds, such as
	// alice@example.com.
	Key string `json:"key"`

	// Attributes are the attributes of the user, which the request always holds:
	// UserInvitesService.Approve sends {} for nil, as it does for an empty map.
	Attributes Attributes `json:"attributes"`
}
