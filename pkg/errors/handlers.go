package errors

import (
	e "errors"
	"fmt"
	"net/http"
)

// HttpErrorHandle converts err and response into a PermitError. With a nil response it returns err
// unchanged. Statuses 401, 403, 404, 409 and 422 map to their own error codes, and any other status
// outside 200-299 maps to UnexpectedError, also when err is nil. For a status in 200-299 it returns
// nil when err is nil, and UnexpectedError otherwise.
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
	if response.StatusCode < 200 || response.StatusCode > 299 {
		status := response.Status
		if status == "" {
			status = fmt.Sprintf("unexpected HTTP status %d", response.StatusCode)
		}
		return NewPermitUnexpectedError(e.New(status), response)
	}
	return nil
}
