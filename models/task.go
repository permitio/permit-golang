package models

// TaskStatus is the status of a task of the API that runs in the background, such as the copy of
// an environment in [TaskResultEnvironmentRead]. It is an open enum, as [AttributeType]
// describes.
type TaskStatus string

// The values of TaskStatus that this version of the SDK knows.
const (
	// TaskStatusProcessing is processing.
	TaskStatusProcessing TaskStatus = "processing"

	// TaskStatusSuccess is success.
	TaskStatusSuccess TaskStatus = "success"

	// TaskStatusFailure is failure.
	TaskStatusFailure TaskStatus = "failure"

	// TaskStatusCancelled is cancelled.
	TaskStatusCancelled TaskStatus = "cancelled"
)

// Known reports whether s is one of the constants of TaskStatus, the values that this version
// of the SDK knows.
func (s TaskStatus) Known() bool {
	switch s {
	case TaskStatusProcessing, TaskStatusSuccess, TaskStatusFailure, TaskStatusCancelled:
		return true
	default:
		return false
	}
}

// ErrorDetails is an error of the API that a reply holds as a value, such as the error that ended
// a task in [TaskResultEnvironmentRead.Error]. A call whose reply has an error status returns
// the error as an Error of the root package instead.
type ErrorDetails struct {
	// ID is the ID of the error.
	ID string `json:"id"`

	// Title is the title of the error.
	Title string `json:"title"`

	// ErrorCode is the code of the error, such as [ErrorCodeNotFound].
	ErrorCode ErrorCode `json:"error_code"`

	// Message says what went wrong, or is empty when the error holds no message.
	Message string `json:"message,omitzero"`

	// SupportLink is the URL of a page about the error, or empty when the error holds none.
	SupportLink string `json:"support_link,omitzero"`

	// AdditionalInfo is any JSON value that the error adds, with each number as a [json.Number],
	// or nil when the error holds none.
	AdditionalInfo any `json:"additional_info,omitzero"`
}

// ErrorCode is the code of an error of the API, as [ErrorDetails] holds it. It is an open enum,
// as [AttributeType] describes.
type ErrorCode string

// The values of ErrorCode that this version of the SDK knows.
const (
	// ErrorCodeInvalidInput is INVALID_INPUT.
	ErrorCodeInvalidInput ErrorCode = "INVALID_INPUT"

	// ErrorCodeInvalidJWKSConfiguration is INVALID_JWKS_CONFIGURATION.
	ErrorCodeInvalidJWKSConfiguration ErrorCode = "INVALID_JWKS_CONFIGURATION"

	// ErrorCodeUnexpectedError is UNEXPECTED_ERROR.
	ErrorCodeUnexpectedError ErrorCode = "UNEXPECTED_ERROR"

	// ErrorCodeNotFound is NOT_FOUND.
	ErrorCodeNotFound ErrorCode = "NOT_FOUND"

	// ErrorCodeDuplicateEntity is DUPLICATE_ENTITY.
	ErrorCodeDuplicateEntity ErrorCode = "DUPLICATE_ENTITY"

	// ErrorCodeEmptyDecisionLogs is EMPTY_DECISION_LOGS.
	ErrorCodeEmptyDecisionLogs ErrorCode = "EMPTY_DECISION_LOGS"

	// ErrorCodeMissingRequestAttribute is MISSING_REQUEST_ATTRIBUTE.
	ErrorCodeMissingRequestAttribute ErrorCode = "MISSING_REQUEST_ATTRIBUTE"

	// ErrorCodeForbiddenAccess is FORBIDDEN_ACCESS.
	ErrorCodeForbiddenAccess ErrorCode = "FORBIDDEN_ACCESS"

	// ErrorCodeInvalidPermissionFormat is INVALID_PERMISSION_FORMAT.
	ErrorCodeInvalidPermissionFormat ErrorCode = "INVALID_PERMISSION_FORMAT"

	// ErrorCodeMissingPermissions is MISSING_PERMISSIONS.
	ErrorCodeMissingPermissions ErrorCode = "MISSING_PERMISSIONS"

	// ErrorCodeUnsupportedAttributeType is UNSUPPORTED_ATTRIBUTE_TYPE.
	ErrorCodeUnsupportedAttributeType ErrorCode = "UNSUPPORTED_ATTRIBUTE_TYPE"

	// ErrorCodeMissingResourceAttribute is MISSING_RESOURCE_ATTRIBUTE.
	ErrorCodeMissingResourceAttribute ErrorCode = "MISSING_RESOURCE_ATTRIBUTE"

	// ErrorCodeMismatchAttributesTypes is MISMATCH_ATTRIBUTES_TYPES.
	ErrorCodeMismatchAttributesTypes ErrorCode = "MISMATCH_ATTRIBUTES_TYPES"

	// ErrorCodeInvalidPolicyRepoStatus is INVALID_POLICY_REPO_STATUS.
	ErrorCodeInvalidPolicyRepoStatus ErrorCode = "INVALID_POLICY_REPO_STATUS"

	// ErrorCodeInvalidSMTPConfiguration is INVALID_SMTP_CONFIGURATION.
	ErrorCodeInvalidSMTPConfiguration ErrorCode = "INVALID_SMTP_CONFIGURATION"

	// ErrorCodeMismatchResourceInstancesTenants is MISMATCH_RESOURCE_INSTANCES_TENANTS.
	ErrorCodeMismatchResourceInstancesTenants ErrorCode = "MISMATCH_RESOURCE_INSTANCES_TENANTS"

	// ErrorCodePaginationSizeOverreached is PAGINATION_SIZE_OVERREACHED.
	ErrorCodePaginationSizeOverreached ErrorCode = "PAGINATION_SIZE_OVERREACHED"

	// ErrorCodeMismatchRelationType is MISMATCH_RELATION_TYPE.
	ErrorCodeMismatchRelationType ErrorCode = "MISMATCH_RELATION_TYPE"

	// ErrorCodeConcurrentOperationDisallowed is CONCURRENT_OPERATION_DISALLOWED.
	ErrorCodeConcurrentOperationDisallowed ErrorCode = "CONCURRENT_OPERATION_DISALLOWED"

	// ErrorCodeUnauthorized is UNAUTHORIZED.
	ErrorCodeUnauthorized ErrorCode = "UNAUTHORIZED"
)

// Known reports whether c is one of the constants of ErrorCode, the values that this version of
// the SDK knows.
func (c ErrorCode) Known() bool {
	switch c {
	case ErrorCodeInvalidInput, ErrorCodeInvalidJWKSConfiguration, ErrorCodeUnexpectedError,
		ErrorCodeNotFound, ErrorCodeDuplicateEntity, ErrorCodeEmptyDecisionLogs,
		ErrorCodeMissingRequestAttribute, ErrorCodeForbiddenAccess,
		ErrorCodeInvalidPermissionFormat, ErrorCodeMissingPermissions,
		ErrorCodeUnsupportedAttributeType, ErrorCodeMissingResourceAttribute,
		ErrorCodeMismatchAttributesTypes, ErrorCodeInvalidPolicyRepoStatus,
		ErrorCodeInvalidSMTPConfiguration, ErrorCodeMismatchResourceInstancesTenants,
		ErrorCodePaginationSizeOverreached, ErrorCodeMismatchRelationType,
		ErrorCodeConcurrentOperationDisallowed, ErrorCodeUnauthorized:
		return true
	default:
		return false
	}
}
