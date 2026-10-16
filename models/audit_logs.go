package models

import "encoding/json"

// AuditLogQueryType is the kind of query of the PDP whose decisions a list of the audit logs
// holds, as AuditLogsService.List of the root package sends it in the query parameter query.
type AuditLogQueryType string

// The values of AuditLogQueryType that this version of the SDK knows.
const (
	// AuditLogQueryTypeCheck is a check, one or in bulk: the query that the API lists when a
	// list sends none.
	AuditLogQueryTypeCheck AuditLogQueryType = "check"

	// AuditLogQueryTypeGetUserPermissions is a query of the permissions of a user.
	AuditLogQueryTypeGetUserPermissions AuditLogQueryType = "get_user_permissions"

	// AuditLogQueryTypeGetAuthorizedUsers is a query of the users that may do an action on a
	// resource.
	AuditLogQueryTypeGetAuthorizedUsers AuditLogQueryType = "get_authorized_users"

	// AuditLogQueryTypeNone is every query.
	AuditLogQueryTypeNone AuditLogQueryType = "none"
)

// Known reports whether t is one of the constants of AuditLogQueryType, the values that this
// version of the SDK knows.
func (t AuditLogQueryType) Known() bool {
	switch t {
	case AuditLogQueryTypeCheck, AuditLogQueryTypeGetUserPermissions,
		AuditLogQueryTypeGetAuthorizedUsers, AuditLogQueryTypeNone:
		return true
	default:
		return false
	}
}

// AuditLogSortKey is the order of a list of the audit logs, as AuditLogsService.List of the root
// package sends it in the query parameter sort_by.
type AuditLogSortKey string

// The values of AuditLogSortKey that this version of the SDK knows.
const (
	// AuditLogSortKeyNone sorts the audit logs in no particular order.
	AuditLogSortKeyNone AuditLogSortKey = "None"

	// AuditLogSortKeyTimestamp sorts the audit logs by the time of their decisions, newest
	// first: the order that the API gives when a list sends none.
	AuditLogSortKeyTimestamp AuditLogSortKey = "timestamp"
)

// Known reports whether k is one of the constants of AuditLogSortKey, the values that this
// version of the SDK knows.
func (k AuditLogSortKey) Known() bool {
	switch k {
	case AuditLogSortKeyNone, AuditLogSortKeyTimestamp:
		return true
	default:
		return false
	}
}

// AuditLogModel is the audit log of a decision of a PDP of an environment, as a list of the
// audit logs returns it: who asked for what, and what the PDP decided. Input, Result, Context
// and RawData hold JSON as the reply holds it, which encoding/json reads into a value of the
// caller's type. The API sends null for a field that has no value: a string field is then
// empty, Decision is nil, and a field of JSON holds the JSON null, json.RawMessage("null"), not
// nil. [DetailedAuditLogModel], which AuditLogsService.Get of the root package returns, adds the
// objects that the decision names.
type AuditLogModel struct {
	// ID is the audit log's ID, a UUID.
	ID string `json:"id"`

	// Timestamp is when the PDP made the decision.
	Timestamp Time `json:"timestamp"`

	// CreatedAt is when the audit log was stored, or the zero Time when the reply holds none.
	CreatedAt Time `json:"created_at,omitzero"`

	// Query is the query of the PDP that made the decision, such as permit/root for a check, or
	// empty when the reply holds none.
	Query string `json:"query,omitzero"`

	// UserKey is the key of the user of the decision, or empty when the reply holds none.
	UserKey string `json:"user_key,omitzero"`

	// UserEmail is the email of the user of the decision, or empty when the reply holds none.
	UserEmail string `json:"user_email,omitzero"`

	// UserName is the name of the user of the decision, or empty when the reply holds none.
	UserName string `json:"user_name,omitzero"`

	// ResourceType is the key of the resource of the decision, such as document, or empty when
	// the reply holds none.
	ResourceType string `json:"resource_type,omitzero"`

	// Tenant is the key of the tenant of the decision, or empty when the reply holds none.
	Tenant string `json:"tenant,omitzero"`

	// Action is the action of the decision, such as read, or empty when the reply holds none.
	Action string `json:"action,omitzero"`

	// Decision is the decision, true for allowed and false for denied, or nil when the reply
	// holds none.
	Decision *bool `json:"decision,omitzero"`

	// Reason is why the PDP decided as it did, or empty when the reply holds none.
	Reason string `json:"reason,omitzero"`

	// OrganizationID is the ID of the organization of the decision.
	OrganizationID string `json:"org_id"`

	// ProjectID is the ID of the project of the decision.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the environment of the decision.
	EnvironmentID string `json:"env_id"`

	// PDPConfigID is the ID of the configuration of the PDP that made the decision, or empty
	// when the reply holds none.
	PDPConfigID string `json:"pdp_config_id,omitzero"`

	// Input is the input of the decision, the query that the PDP got, as JSON: the JSON null
	// when the reply holds null, and nil when the field is absent.
	Input json.RawMessage `json:"input,omitzero"`

	// Result is the result of the decision, as the PDP gave it, as JSON: the JSON null when the
	// reply holds null, and nil when the field is absent.
	Result json.RawMessage `json:"result,omitzero"`

	// Context is the context of the decision, as JSON: the JSON null when the reply holds null,
	// and nil when the field is absent.
	Context json.RawMessage `json:"context,omitzero"`

	// RawData is the decision log as the engine of the PDP wrote it, as JSON: the JSON null when
	// the reply holds null, and nil when the field is absent; [DetailedAuditLogModel] always holds
	// it.
	RawData json.RawMessage `json:"raw_data,omitzero"`
}

// DetailedAuditLogModel is the audit log of a decision of a PDP of an environment, as
// AuditLogsService.Get of the root package returns it: the fields of an [AuditLogModel], with
// the decision log of the engine always set, and the objects that the decision names. A field
// for which the reply holds null is as [AuditLogModel] says.
type DetailedAuditLogModel struct {
	// ID is the audit log's ID, a UUID.
	ID string `json:"id"`

	// Timestamp is when the PDP made the decision.
	Timestamp Time `json:"timestamp"`

	// CreatedAt is when the audit log was stored, or the zero Time when the reply holds none.
	CreatedAt Time `json:"created_at,omitzero"`

	// Query is the query of the PDP that made the decision, such as permit/root for a check, or
	// empty when the reply holds none.
	Query string `json:"query,omitzero"`

	// UserKey is the key of the user of the decision, or empty when the reply holds none.
	UserKey string `json:"user_key,omitzero"`

	// UserEmail is the email of the user of the decision, or empty when the reply holds none.
	UserEmail string `json:"user_email,omitzero"`

	// UserName is the name of the user of the decision, or empty when the reply holds none.
	UserName string `json:"user_name,omitzero"`

	// ResourceType is the key of the resource of the decision, such as document, or empty when
	// the reply holds none.
	ResourceType string `json:"resource_type,omitzero"`

	// Tenant is the key of the tenant of the decision, or empty when the reply holds none.
	Tenant string `json:"tenant,omitzero"`

	// Action is the action of the decision, such as read, or empty when the reply holds none.
	Action string `json:"action,omitzero"`

	// Decision is the decision, true for allowed and false for denied, or nil when the reply
	// holds none.
	Decision *bool `json:"decision,omitzero"`

	// Reason is why the PDP decided as it did, or empty when the reply holds none.
	Reason string `json:"reason,omitzero"`

	// OrganizationID is the ID of the organization of the decision.
	OrganizationID string `json:"org_id"`

	// ProjectID is the ID of the project of the decision.
	ProjectID string `json:"project_id"`

	// EnvironmentID is the ID of the environment of the decision.
	EnvironmentID string `json:"env_id"`

	// PDPConfigID is the ID of the configuration of the PDP that made the decision, or empty
	// when the reply holds none.
	PDPConfigID string `json:"pdp_config_id,omitzero"`

	// Input is the input of the decision, the query that the PDP got, as JSON: the JSON null
	// when the reply holds null, and nil when the field is absent.
	Input json.RawMessage `json:"input,omitzero"`

	// Result is the result of the decision, as the PDP gave it, as JSON: the JSON null when the
	// reply holds null, and nil when the field is absent.
	Result json.RawMessage `json:"result,omitzero"`

	// Context is the context of the decision, as JSON: the JSON null when the reply holds null,
	// and nil when the field is absent.
	Context json.RawMessage `json:"context,omitzero"`

	// RawData is the decision log as the engine of the PDP wrote it, as JSON: an object whose
	// engine field names the engine, such as OPA.
	RawData json.RawMessage `json:"raw_data"`

	// Objects are the objects that the decision names, as the environment held them, or the
	// zero AuditLogObjectsModel when the reply holds none.
	Objects AuditLogObjectsModel `json:"objects,omitzero"`
}

// AuditLogObjectsModel holds the objects that the decision of an audit log names, as the
// environment held them. The user, the action, the resource and the tenant are each nil when the
// reply holds null for it or leaves it out. The organization, the project, the environment and
// the configuration of the PDP are JSON objects as the reply holds them, in the form of the API's
// schema of each, or another, which encoding/json reads into a value of the caller's type: each
// is the JSON null when the reply holds null, and nil when the field is absent.
type AuditLogObjectsModel struct {
	// ID is the ID of the objects, or empty when the reply holds none.
	ID string `json:"id,omitzero"`

	// CreatedAt is when the objects were stored, or the zero Time when the reply holds none.
	CreatedAt Time `json:"created_at,omitzero"`

	// OrganizationObject is the organization, as JSON.
	OrganizationObject json.RawMessage `json:"organization_object,omitzero"`

	// ProjectObject is the project, as JSON.
	ProjectObject json.RawMessage `json:"project_object,omitzero"`

	// EnvironmentObject is the environment, as JSON.
	EnvironmentObject json.RawMessage `json:"environment_object,omitzero"`

	// PDPConfigObject is the configuration of the PDP, as JSON.
	PDPConfigObject json.RawMessage `json:"pdp_config_object,omitzero"`

	// UserObject is the user.
	UserObject *UserObj `json:"user_object,omitzero"`

	// ActionObject is the action.
	ActionObject *ActionObj `json:"action_object,omitzero"`

	// ResourceTypeObject is the resource.
	ResourceTypeObject *ResourceTypeObj `json:"resource_type_object,omitzero"`

	// TenantObject is the tenant.
	TenantObject *TenantObj `json:"tenant_object,omitzero"`
}

// UserObj is the user of a decision, as [AuditLogObjectsModel] holds it.
type UserObj struct {
	// ID is the user's ID, a UUID.
	ID string `json:"id"`

	// Key is the user's key.
	Key string `json:"key"`

	// Email is the user's email, or empty when the reply holds none.
	Email string `json:"email,omitzero"`

	// FirstName is the user's first name, or empty when the reply holds none.
	FirstName string `json:"first_name,omitzero"`

	// LastName is the user's last name, or empty when the reply holds none.
	LastName string `json:"last_name,omitzero"`

	// Attributes are the user's attributes, or nil when the reply holds none.
	Attributes Attributes `json:"attributes,omitzero"`

	// AssignedRoles are the keys of the user's roles, or nil when the reply holds none.
	AssignedRoles []string `json:"assigned_roles,omitzero"`

	// CreatedAt is when the user was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the user was last changed.
	UpdatedAt Time `json:"updated_at"`
}

// ActionObj is the action of a decision, as [AuditLogObjectsModel] holds it.
type ActionObj struct {
	// ID is the action's ID, a UUID.
	ID string `json:"id"`

	// Key is the action's key, such as read.
	Key string `json:"key"`

	// Name is the action's name, or empty when the reply holds none.
	Name string `json:"name,omitzero"`

	// CreatedAt is when the action was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the action was last changed.
	UpdatedAt Time `json:"updated_at"`
}

// ResourceTypeObj is the resource of a decision, as [AuditLogObjectsModel] holds it.
type ResourceTypeObj struct {
	// ID is the resource's ID, a UUID.
	ID string `json:"id"`

	// Key is the resource's key, such as document.
	Key string `json:"key"`

	// Name is the resource's name, or empty when the reply holds none.
	Name string `json:"name,omitzero"`

	// Attributes are the attributes of the resource, each with its type, or nil when the reply
	// holds none.
	Attributes []ResourceAttributes `json:"attributes,omitzero"`

	// CreatedAt is when the resource was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the resource was last changed.
	UpdatedAt Time `json:"updated_at"`
}

// ResourceAttributes is an attribute of the resource of a decision, as [ResourceTypeObj] holds
// it.
type ResourceAttributes struct {
	// Key is the attribute's key.
	Key string `json:"key"`

	// Type is the type of the attribute's values.
	Type AttributeType `json:"type"`
}

// TenantObj is the tenant of a decision, as [AuditLogObjectsModel] holds it.
type TenantObj struct {
	// ID is the tenant's ID, a UUID.
	ID string `json:"id"`

	// Key is the tenant's key.
	Key string `json:"key"`

	// Name is the tenant's name, or empty when the reply holds none.
	Name string `json:"name,omitzero"`

	// Attributes are the tenant's attributes, or nil when the reply holds none.
	Attributes Attributes `json:"attributes,omitzero"`

	// CreatedAt is when the tenant was created.
	CreatedAt Time `json:"created_at"`

	// UpdatedAt is when the tenant was last changed.
	UpdatedAt Time `json:"updated_at"`
}
