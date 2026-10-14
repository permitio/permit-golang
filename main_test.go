package permit_test

import (
	"os"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// TestMain runs the tests. When the environment variable PERMIT_API_COVERAGE_RECORD names a
// file, it also writes the record of the requests that the tests send to the mockpermit servers
// there, with the session trailer, for tools/cmd/apicoverage; CONTRIBUTING.md ("API coverage")
// says how to run it.
func TestMain(m *testing.M) {
	os.Exit(mockpermit.RunRecorded(m.Run))
}
