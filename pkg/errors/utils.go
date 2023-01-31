package errors

import (
	"io/ioutil"
	"net/http"
)

func getJsonFromHttpResponse(httpResponse *http.Response) string {
	body, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		return ""
	}
	return string(body)
}
