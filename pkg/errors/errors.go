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

func NewPermitUnexpectedError(err error) *PermitUnexpectedError {
	return &PermitUnexpectedError{NewPermitError(ErrorMessage(err.Error()), UnexpectedError, GENERAL_ERROR)}
}

func NewPermitError(errorMessage ErrorMessage, errorCode ErrorCode, errorType ErrorType) PermitError {
	return PermitError{e.New("ErrorCode: " + string(errorCode) + " ErrorType:" + string(errorType) + " Message:" + string(errorMessage))}
}

func NewPermitNotFoundError(err error) *PermitNotFoundError {
	return &PermitNotFoundError{
		NewPermitError(ErrorMessage(err.Error()), NotFound, GENERAL_ERROR),
	}
}

func NewPermitTeapotError(err error) *PermitNotFoundError {
	return &PermitNotFoundError{
		NewPermitError(ErrorMessage(err.Error()), NotFound, GENERAL_ERROR),
	}
}

func NewPermitUnprocessableEntityError(err error) *PermitUnprocessableEntityError {
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
	return &PermitDuplicateEntityError{
		NewPermitError(ErrorMessage(err.Error()), DuplicateEntity, GENERAL_ERROR),
	}
}
