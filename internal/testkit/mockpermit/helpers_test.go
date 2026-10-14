package mockpermit

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/testkit/faketb"
)

// wantFailures fails t unless the failures that ft recorded since the last Take are exactly one
// per entry of want, each containing that entry, in order.
func wantFailures(t *testing.T, ft *faketb.FakeTB, want ...string) {
	t.Helper()
	got := ft.Take()
	ok := len(got) == len(want)
	for i := 0; ok && i < len(got); i++ {
		ok = strings.Contains(got[i], want[i])
	}
	if !ok {
		t.Errorf("the mock failed the test with %q, want one failure containing each of %q", got,
			want)
	}
}

// newClient returns an HTTP client that keeps its connections alive until t ends.
func newClient(t *testing.T) *http.Client {
	t.Helper()
	transport := &http.Transport{}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport}
}

// call is a request to send to a server.
type call struct {
	method string
	// path is the path and the query, as the client sends them.
	path string
	body string
	// auth are the Authorization header values; nil sends "Bearer " + APIKey.
	auth []string
}

// response is a server's answer.
type response struct {
	status int
	header http.Header
	body   string
}

// send sends c to s with client and returns the answer.
func (c call) send(t *testing.T, client *http.Client, s *Server) response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), c.method, s.URL+c.path,
		strings.NewReader(c.body))
	if err != nil {
		t.Fatalf("building %s %s: %v", c.method, c.path, err)
	}
	auth := c.auth
	if auth == nil {
		auth = []string{"Bearer " + APIKey}
	}
	for _, value := range auth {
		req.Header.Add("Authorization", value)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", c.method, c.path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: reading the answer: %v", c.method, c.path, err)
	}
	return response{status: resp.StatusCode, header: resp.Header, body: string(body)}
}
