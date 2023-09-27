package errors

import (
	e "errors"
	"fmt"
	"net/http"
	"strings"
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

func NewPermitError(errorMessage ErrorMessage, errorCode ErrorCode, errorType ErrorType, response *http.Response) PermitError {
	permitError := e.New(fmt.Sprintf("ErrorCode: %s, ErrorType: %s, Message: %s", errorCode, errorType, errorMessage))

	if response == nil {
		return PermitError{
			error:     permitError,
			ErrorCode: errorCode,
			ErrorType: errorType,
		}
	} else {
		return PermitError{
			StatusCode:   response.StatusCode,
			ResponseBody: getJsonFromHttpResponse(response),
			error:        permitError,
			ErrorCode:    errorCode,
			ErrorType:    errorType,
		}
	}
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
	errorMessage := ErrorMessage(err.Error())

	if err == nil {
		errorMessage = UnprocessableEntityMessage
	}

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
