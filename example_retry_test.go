package permit_test

import (
	"errors"
	"fmt"
	"time"

	"github.com/permitio/permit-golang/v2"
)

// Retry up to 5 times, waiting up to 2 seconds before each retry when the reply has no
// Retry-After header. An option prints as the call that builds it. WithRetry(RetryPolicy{})
// sends each request once, and a negative MaxRetries or MaxBackoff is an error that matches
// ErrInvalidConfig.
func ExampleWithRetry() {
	retry := permit.WithRetry(permit.RetryPolicy{MaxRetries: 5, MaxBackoff: 2 * time.Second})
	fmt.Println(retry)

	client, err := permit.New(
		permit.WithAPIKey("permit_key_FAKEexample"),
		permit.WithAPIURL("https://permit.example.com/api"),
		permit.WithPDPURL("http://localhost:7766"),
		retry,
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer client.Close()

	_, err = permit.New(
		permit.WithAPIKey("permit_key_FAKEexample"),
		permit.WithAPIURL("https://permit.example.com/api"),
		permit.WithRetry(permit.RetryPolicy{MaxRetries: -1}),
	)
	fmt.Println(errors.Is(err, permit.ErrInvalidConfig))
	// Output:
	// permit.WithRetry(permit.RetryPolicy{MaxRetries: 5, MaxBackoff: 2s})
	// true
}
