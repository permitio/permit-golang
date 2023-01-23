package errors

import "net/http"

func HttpErrorHandle(err error, response *http.Response) error {
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
		return NewPermitNotFoundError(err)
	}
	if response.StatusCode == 418 {
		return NewPermitTeapotError(err)
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
	return http.ErrNoLocation
}
