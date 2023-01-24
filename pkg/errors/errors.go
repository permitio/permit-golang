package errors

import (
	e "errors"
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
			NewPermitError(NotFoundMessage, NotFound, GENERAL_ERROR),
		}
	}
	return &PermitNotFoundError{
		NewPermitError(ErrorMessage(err.Error()), NotFound, GENERAL_ERROR),
	}
}

func NewPermitConflictError() *PermitNotFoundError {
	return &PermitNotFoundError{
		NewPermitError(ConflictMessage, Conflict, GENERAL_ERROR),
	}
}

func NewPermitPaginationError() *PermitPaginationError {
	return &PermitPaginationError{
		NewPermitError(PaginationMessage, PaginationError, GENERAL_ERROR),
	}
}

func NewPermitUnprocessableEntityError(err error) *PermitUnprocessableEntityError {
	if err == nil {
		return &PermitUnprocessableEntityError{
			NewPermitError(UnprocessableEntityMessage, UnprocessableEntityError, GENERAL_ERROR),
		}
	}
	return &PermitUnprocessableEntityError{
		NewPermitError(ErrorMessage(err.Error()), UnprocessableEntityError, GENERAL_ERROR),
	}
}

func NewPermitForbiddenError() *PermitForbiddenError {
	return &PermitForbiddenError{
		NewPermitError(ForbiddenMessage, ForbiddenAccess, GENERAL_ERROR),
	}
}

func NewPermitUnauthorizedError() *PermitUnauthorizedError {
	return &PermitUnauthorizedError{
		NewPermitError(UnauthorizedMessage, Unauthorized, GENERAL_ERROR),
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
			NewPermitError(DuplicateEntityMessage, DuplicateEntity, GENERAL_ERROR),
		}
	}
	return &PermitDuplicateEntityError{
		NewPermitError(ErrorMessage(err.Error()), DuplicateEntity, GENERAL_ERROR),
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
