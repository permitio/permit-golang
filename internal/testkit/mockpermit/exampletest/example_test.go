package exampletest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// The URLs and the API key the Example uses, which TestMain sets. An SDK package's Examples pass
// them to the client, so that pkg.go.dev shows the client code and none of the setup.
var (
	exampleAPIURL string
	examplePDPURL string
	exampleAPIKey string
)

func TestMain(m *testing.M) {
	servers := mockpermit.StartExample()
	exampleAPIURL, examplePDPURL = servers.API.URL, servers.PDP.URL
	exampleAPIKey = mockpermit.APIKey
	servers.API.Handle("GET /v2/api-key/scope",
		mockpermit.Reply{Body: `{"project_id":"proj","environment_id":"env"}`})
	servers.PDP.Handle("POST /allowed", mockpermit.Reply{Body: `{"allow":true}`})
	os.Exit(servers.Run(m))
}

// Example sends one request to each server, the way a client of the SDK would.
func Example() {
	client := &http.Client{}
	for _, request := range []struct{ method, url, body string }{
		{http.MethodGet, exampleAPIURL + "/v2/api-key/scope", ""},
		{http.MethodPost, examplePDPURL + "/allowed", `{"user":"u","action":"read"}`},
	} {
		req, err := http.NewRequestWithContext(context.Background(), request.method,
			request.url, strings.NewReader(request.body))
		if err != nil {
			fmt.Println(err)
			return
		}
		req.Header.Set("Authorization", "Bearer "+exampleAPIKey)
		resp, err := client.Do(req)
		if err != nil {
			fmt.Println(err)
			return
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(resp.StatusCode, string(body))
	}
	// Output:
	// 200 {"project_id":"proj","environment_id":"env"}
	// 200 {"allow":true}
}
