package errors

import (
	"io"
	"net/http"
)

func getJsonFromHttpResponse(httpResponse *http.Response) string {
	body, err := io.ReadAll(httpResponse.Body)

	if err != nil {
		return ""
	}

	return string(body)
}
