package permit_test

import (
	"os"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// The IDs of the scope of the API key of the Examples and of the wire case of Client.Scope, as
// the reply fixture testdata/replies/get_api_key_scope.200.json holds them.
const (
	exampleProjectID     = "00000000-0000-4000-8000-000000000002"
	exampleEnvironmentID = "00000000-0000-4000-8000-000000000003"
)

// The URLs and the API key that the Examples pass to the client, which TestMain sets to those of
// the mockpermit servers that it starts for them.
var (
	exampleAPIURL string
	examplePDPURL string
	exampleAPIKey string
)

// TestMain starts the mockpermit servers of the Examples and runs the tests. When the
// environment variable PERMIT_API_COVERAGE_RECORD names a file, it also writes the record of the
// requests that the tests send to the mockpermit servers there, with the session trailer, for
// tools/cmd/apicoverage; CONTRIBUTING.md ("API coverage") says how to run it. The servers of the
// Examples do not write to the record.
func TestMain(m *testing.M) {
	servers := mockpermit.StartExample()
	exampleAPIURL, examplePDPURL = servers.API.URL, servers.PDP.URL
	exampleAPIKey = mockpermit.APIKey
	servers.API.Handle("GET /v2/api-key/scope",
		mockpermit.Reply{Body: replyFixture("get_api_key_scope.200.json")})
	os.Exit(mockpermit.RunRecorded(func() int { return servers.Run(m) }))
}
