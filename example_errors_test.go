package permit_test

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/permitio/permit-golang/v2"
)

// getUserError returns an error as a call that gets a user who does not exist returns it, wrapped
// by the caller.
func getUserError() error {
	return fmt.Errorf("load the profile: %w", &permit.Error{
		Op:         "get_user",
		Method:     http.MethodGet,
		Route:      "/v2/facts/{proj_id}/{env_id}/users/{user_id}",
		StatusCode: http.StatusNotFound,
		Code:       "NOT_FOUND",
		Title:      "Not Found",
		Detail:     "no user with the key alice",
		RequestID:  "8f2c1d",
	})
}

// Check what failed with errors.Is and a sentinel, through any wrapping.
func ExampleError_Is() {
	err := getUserError()

	switch {
	case errors.Is(err, permit.ErrNotFound):
		fmt.Println("no such user: create it")
	case errors.Is(err, permit.ErrRateLimited):
		fmt.Println("slow down and retry")
	default:
		fmt.Println("failed:", err)
	}
	// Output:
	// no such user: create it
}

// Read the request, the status and the API's error details with errors.AsType, or errors.As.
func ExampleError() {
	err := getUserError()

	if permitErr, ok := errors.AsType[*permit.Error](err); ok {
		fmt.Println(permitErr.Op, permitErr.Method, permitErr.Route)
		fmt.Println(permitErr.StatusCode, permitErr.Code, permitErr.RequestID)
		fmt.Println(permitErr.Detail)
	}
	// Output:
	// get_user GET /v2/facts/{proj_id}/{env_id}/users/{user_id}
	// 404 NOT_FOUND 8f2c1d
	// no user with the key alice
}

// The text of an Error names the request and says what the reply said, without the body.
func ExampleError_Error() {
	err := &permit.Error{
		Op:         "get_api_key_scope",
		Method:     http.MethodGet,
		Route:      "/v2/api-key/scope",
		StatusCode: http.StatusUnauthorized,
		Code:       "UNAUTHORIZED",
		Detail:     "invalid API key",
		Body:       []byte(`{"error_code":"UNAUTHORIZED","message":"invalid API key"}`),
	}

	fmt.Println(err)
	// Output:
	// permit: get_api_key_scope: GET /v2/api-key/scope: status 401 UNAUTHORIZED: invalid API key
}
