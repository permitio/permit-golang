package errors

import (
	e "errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/permitio/permit-golang/internal/redact"
)

type PermitError struct {
	error
	StatusCode   int
	ResponseBody string
	ErrorCode    ErrorCode
	ErrorType    ErrorType
}

func NewPermitUnexpectedError(err error, response *http.Response) PermitError {
	if err == nil {
		return NewPermitError(UnexpectedErrorMessage, UnexpectedError, GENERAL_ERROR, response)
	}
	return NewPermitError(ErrorMessage(err.Error()), UnexpectedError, GENERAL_ERROR, response)
}

// NewPermitError returns a PermitError whose text and ResponseBody have the API key that the
// request was sent with and the values that start with permit_key_ replaced by [REDACTED]. In a
// JSON ResponseBody, the values under keys that name a credential, such as secret, token,
// api_key, password and authorization, are replaced too. The API key is taken from the
// Authorization header of response.Request, and is replaced only when it has at least 16
// characters; with a transport that does not set Response.Request, the other values are still
// replaced.
func NewPermitError(errorMessage ErrorMessage, errorCode ErrorCode, errorType ErrorType, response *http.Response) PermitError {
	apiKey := requestAPIKey(response)
	message := redact.Text(string(errorMessage), apiKey)
	permitError := e.New(fmt.Sprintf("ErrorCode: %s, ErrorType: %s, Message: %s",
		errorCode, errorType, message))

	if response == nil {
		return PermitError{
			error:     permitError,
			ErrorCode: errorCode,
			ErrorType: errorType,
		}
	} else {
		return PermitError{
			StatusCode:   response.StatusCode,
			ResponseBody: redact.Body(getJsonFromHttpResponse(response), apiKey),
			error:        permitError,
			ErrorCode:    errorCode,
			ErrorType:    errorType,
		}
	}
}

// requestAPIKey returns the credential in the Authorization header of the request that response
// answers, without its scheme, or "" when there is none. The SDK sends the configured API key
// there, so this is the key to remove from what the response is turned into.
func requestAPIKey(response *http.Response) string {
	if response == nil || response.Request == nil {
		return ""
	}
	authorization := response.Request.Header.Get("Authorization")
	if _, credential, found := strings.Cut(authorization, " "); found {
		return strings.TrimSpace(credential)
	}
	return authorization
}

func NewPermitNotFoundError(err error, response *http.Response) PermitError {
	if err == nil {
		return NewPermitError(NotFoundMessage, NotFound, API_ERROR, response)
	}

	return NewPermitError(ErrorMessage(err.Error()), NotFound, API_ERROR, response)
}

func NewPermitConflictError(response *http.Response) PermitError {
	return NewPermitError(ConflictMessage, Conflict, API_ERROR, response)
}

func NewPermitPaginationError() PermitError {
	return NewPermitError(PaginationMessage, PaginationError, API_ERROR, nil)
}

func NewPermitUnprocessableEntityError(err error, response *http.Response) PermitError {
	if err == nil {
		return NewPermitError(UnprocessableEntityMessage, UnprocessableEntityError, API_ERROR, response)
	}
	errorMessage := ErrorMessage(err.Error())
	if strings.Contains(err.Error(), "not a valid email address") {
		errorMessage = "Email is not valid"
	}
	return NewPermitError(errorMessage, UnprocessableEntityError, API_ERROR, response)
}

func NewPermitForbiddenError(response *http.Response) PermitError {
	return NewPermitError(ForbiddenMessage, ForbiddenAccess, API_ERROR, response)
}

func NewPermitUnauthorizedError(response *http.Response) PermitError {
	return NewPermitError(UnauthorizedMessage, Unauthorized, API_ERROR, response)
}

func NewPermitContextError(additionalMessage ErrorMessage) PermitError {
	return NewPermitError(ContextMessage+" - "+additionalMessage, ContextError, GENERAL_ERROR, nil)
}

func NewPermitDuplicateEntityError(err error, response *http.Response) PermitError {
	if err == nil {
		return NewPermitError(DuplicateEntityMessage, DuplicateEntity, API_ERROR, response)
	}
	return NewPermitError(ErrorMessage(err.Error()), DuplicateEntity, API_ERROR, response)
}

func NewPermitConnectionError(err error) PermitError {
	if err == nil {
		return NewPermitError(ConnectionErrorMessage, ConnectionError, GENERAL_ERROR, nil)
	}

	return NewPermitError(ErrorMessage(err.Error()), ConnectionError, GENERAL_ERROR, nil)
}

// NewPermitContextErrorWithCause returns a ContextError that wraps cause, so errors.Is, errors.As
// and errors.Unwrap reach it, and whose text ends with the text of cause. When cause is a
// PermitError with a status code, the text also gives that status, and StatusCode and ResponseBody
// are copied from cause.
func NewPermitContextErrorWithCause(additionalMessage ErrorMessage, cause error) PermitError {
	permitError := NewPermitContextError(additionalMessage)
	if cause == nil {
		return permitError
	}
	var causePermitError PermitError
	if e.As(cause, &causePermitError) && causePermitError.StatusCode != 0 {
		permitError.StatusCode = causePermitError.StatusCode
		permitError.ResponseBody = causePermitError.ResponseBody
		permitError.error = fmt.Errorf("%s (status %d): %w",
			permitError.Error(), permitError.StatusCode, cause)
		return permitError
	}
	permitError.error = fmt.Errorf("%s: %w", permitError.Error(), cause)
	return permitError
}

// Unwrap returns the error that caused p, or nil when p records no cause.
func (p PermitError) Unwrap() error {
	return e.Unwrap(p.error)
}
