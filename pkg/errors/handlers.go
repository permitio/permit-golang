package errors

import (
	"net/http"
)

func HttpErrorHandle(err error, response *http.Response) error {
	if response == nil {
		return err
	}

	if response.StatusCode == 401 {
		return NewPermitUnauthorizedError(response)
	}
	if response.StatusCode == 403 {
		return NewPermitForbiddenError(response)
	}
	if response.StatusCode == 404 {
		err = NewPermitNotFoundError(err, response)
		return err
	}
	if response.StatusCode == 409 {
		err = NewPermitConflictError(response)
		return err
	}
	if response.StatusCode == 422 {
		return NewPermitUnprocessableEntityError(err, response)
	}
	if response.StatusCode >= 500 {
		return NewPermitUnexpectedError(err, response)
	}
	if err != nil {
		return NewPermitUnexpectedError(err, response)
	}
	return nil
}
