// Package exitcode_test is a package whose Example passes while the servers of StartExample
// record a problem: TestExampleServersRunFailsTheRun (example_test.go) builds and runs its test
// binary, which must exit 1. It is in testdata, so go test ./... leaves it out.
package exitcode_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

var exampleAPIURL string

func TestMain(m *testing.M) {
	servers := mockpermit.StartExample()
	exampleAPIURL = servers.API.URL
	os.Exit(servers.Run(m))
}

// Example sends a request for a route that the API server does not have. The server answers 501,
// which the Example prints as its output says, and records the request as a problem.
func Example() {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		exampleAPIURL+"/v2/unknown", nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+mockpermit.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println(err)
		return
	}
	_ = resp.Body.Close()
	fmt.Println(resp.StatusCode)
	// Output: 501
}
