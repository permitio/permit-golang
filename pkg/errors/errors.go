package errors

import (
	e "errors"
	"strings"
)

type PermitError struct {
	error
}

type PermitUnexpectedError struct {
	PermitError
}

type PermitNotFoundError struct {
	PermitError
}

type PermitTeapotError struct {
	PermitError
}

type PermitUnprocessableEntityError struct {
	PermitError
}

type PermitForbiddenError struct {
	PermitError
}

type PermitUnauthorizedError struct {
	PermitError
}

type PermitDuplicateEntityError struct {
	PermitError
}

type PermitContextError struct {
	PermitError
}

type PermitPaginationError struct {
	PermitError
}
type PermitConnectionError struct {
	PermitError
}

func NewPermitUnexpectedError(err error) *PermitUnexpectedError {
	return &PermitUnexpectedError{NewPermitError(ErrorMessage(err.Error()), UnexpectedError, GENERAL_ERROR)}
}

func NewPermitError(errorMessage ErrorMessage, errorCode ErrorCode, errorType ErrorType) PermitError {
	return PermitError{e.New("ErrorCode: " + string(errorCode) + " ErrorType:" + string(errorType) + " Message:" + string(errorMessage))}
}

func NewPermitNotFoundError(err error) *PermitNotFoundError {
	if err == nil {
		return &PermitNotFoundError{
			NewPermitError(NotFoundMessage, NotFound, API_ERROR),
		}
	}
	return &PermitNotFoundError{
		NewPermitError(ErrorMessage(err.Error()), NotFound, API_ERROR),
	}
}

func NewPermitConflictError() *PermitNotFoundError {
	return &PermitNotFoundError{
		NewPermitError(ConflictMessage, Conflict, API_ERROR),
	}
}

func NewPermitPaginationError() *PermitPaginationError {
	return &PermitPaginationError{
		NewPermitError(PaginationMessage, PaginationError, API_ERROR),
	}
}

func NewPermitUnprocessableEntityError(err error) *PermitUnprocessableEntityError {
	errorMessage := ErrorMessage(err.Error())
	if err == nil {
		errorMessage = UnprocessableEntityMessage
	}
	if strings.Contains(err.Error(), "not a valid email address") {
		errorMessage = "Email is not valid"
	}
	return &PermitUnprocessableEntityError{
		NewPermitError(errorMessage, UnprocessableEntityError, API_ERROR),
	}
}

func NewPermitForbiddenError() *PermitForbiddenError {
	return &PermitForbiddenError{
		NewPermitError(ForbiddenMessage, ForbiddenAccess, API_ERROR),
	}
}

func NewPermitUnauthorizedError() *PermitUnauthorizedError {
	return &PermitUnauthorizedError{
		NewPermitError(UnauthorizedMessage, Unauthorized, API_ERROR),
	}
}

func NewPermitContextError(additionalMessage ErrorMessage) *PermitContextError {
	return &PermitContextError{
		NewPermitError(ContextMessage+SeperatorErrorMessage+additionalMessage, ContextError, GENERAL_ERROR),
	}
}

func NewPermitDuplicateEntityError(err error) *PermitDuplicateEntityError {
	if err == nil {
		return &PermitDuplicateEntityError{
			NewPermitError(DuplicateEntityMessage, DuplicateEntity, API_ERROR),
		}
	}
	return &PermitDuplicateEntityError{
		NewPermitError(ErrorMessage(err.Error()), DuplicateEntity, API_ERROR),
	}
}
func NewPermitConnectionError(err error) *PermitDuplicateEntityError {
	if err == nil {
		return &PermitDuplicateEntityError{
			NewPermitError(ConnectionErrorMessage, ConnectionError, GENERAL_ERROR),
		}
	}
	return &PermitDuplicateEntityError{
		NewPermitError(ErrorMessage(err.Error()), ConnectionError, GENERAL_ERROR),
	}
}
