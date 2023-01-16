package errors

type ErrorCode string

const (
	UnexpectedError          ErrorCode = "UnexpectedError"
	NotFound                 ErrorCode = "NotFound"
	TeapotError              ErrorCode = "TeapotError"
	ContextError             ErrorCode = "ContextError"
	DuplicateEntity          ErrorCode = "DuplicateEntity"
	UnprocessableEntityError ErrorCode = "UnprocessableEntityError"
	EmptyDecisionLogs        ErrorCode = "EmptyDecisionLogs"
	MissingRequestAttribute  ErrorCode = "MissingRequestAttribute"
	ForbiddenAccess          ErrorCode = "ForbiddenAccess"
	Unauthorized             ErrorCode = "Unauthorized"
	InvalidPermissionFormat  ErrorCode = "InvalidPermissionFormat"
	MissingPermissions       ErrorCode = "MissingPermissions"
	UnsupportedAttributeType ErrorCode = "UnsupportedAttributeType"
	MissingResourceAttribute ErrorCode = "MissingResourceAttribute"
	InvalidPolicyRepoStatus  ErrorCode = "InvalidPolicyRepoStatus"
	MismatchAttributesTypes  ErrorCode = "MismatchAttributesTypes"
)

type ErrorType string

const (
	GENERAL_ERROR         ErrorType = "general_error"
	API_ERROR             ErrorType = "api_error"
	CACHE_ERROR           ErrorType = "cache_error"
	INVALID_REQUEST_ERROR ErrorType = "invalid_request_error"
)

type ErrorMessage string

const (
	EmptyErrorMessage        ErrorMessage = ""
	SeperatorErrorMessage    ErrorMessage = " - "
	ForbiddenMessage         ErrorMessage = "The access for this object is forbidden using the provided API key"
	ContextMessage           ErrorMessage = "The context is missing or invalid"
	ContextUnexpectedMessage ErrorMessage = "The context is missing or invalid"
	UnauthorizedMessage      ErrorMessage = "The access for this object is not authorized using the provided API key"
)
