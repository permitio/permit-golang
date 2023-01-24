package errors

import "net/http"

func HttpErrorHandle(err error, response *http.Response) error {
	if response == nil {
		return err
	}
	res := getJsonFromHttpResponse(response)
	if res != "" {

	}
	if response.StatusCode == 401 {
		return NewPermitUnauthorizedError()
	}
	if response.StatusCode == 403 {
		return NewPermitForbiddenError()
	}
	if response.StatusCode == 404 {
		err = NewPermitNotFoundError(err)
		return err
	}
	if response.StatusCode == 409 {
		err = NewPermitConflictError()
		return err
	}
	if response.StatusCode == 422 {
		return NewPermitUnprocessableEntityError(err)
	}
	if response.StatusCode >= 500 {
		return NewPermitUnexpectedError(err)
	}
	if err != nil {
		return NewPermitUnexpectedError(err)
	}
	return nil
}
