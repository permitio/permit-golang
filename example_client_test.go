package permit_test

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/permitio/permit-golang/v2"
)

// Build a client with options, and close it when you are done with it. An option overrides the
// environment variable of its setting: leave WithAPIKey out to read the key from PERMIT_API_KEY.
// Printing the client shows a marker in place of the key; this Example prints one field per line.
func ExampleNew() {
	client, err := permit.New(
		permit.WithAPIKey("permit_key_FAKEexample"),
		permit.WithAPIURL("https://permit.example.com/api/"),
		permit.WithPDPURL("http://localhost:7766"),
		permit.WithTimeout(10*time.Second),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer client.Close()

	for field := range strings.SplitSeq(fmt.Sprint(client), ", ") {
		fmt.Println(field)
	}
	// Output:
	// permit.Client{APIKey: "[REDACTED]"
	// APIURL: "https://permit.example.com/api"
	// PDPURL: "http://localhost:7766"
	// Timeout: 10s}
}

// New checks the configuration before it returns, and sends no request: a URL without a scheme
// is an error that matches ErrInvalidConfig.
func ExampleNew_invalidConfiguration() {
	_, err := permit.New(
		permit.WithAPIKey("permit_key_FAKEexample"),
		permit.WithAPIURL("api.permit.io"),
	)
	fmt.Println(errors.Is(err, permit.ErrInvalidConfig))
	fmt.Println(err)
	// Output:
	// true
	// permit: invalid configuration: WithAPIURL: the URL must start with http:// or https://
}
