package mockpermit

import (
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
)

// ExampleServers holds the servers that StartExample starts for the Examples of a package.
type ExampleServers struct {
	// API is the fake Permit API.
	API *Server

	// PDP is the fake PDP container.
	PDP *Server

	failures *collector
}

// StartExample starts a fake Permit API and a fake PDP container for the Examples of a package.
// An Example has no testing.TB, and setup code in an Example shows where pkg.go.dev renders it,
// so the package's TestMain starts the servers, registers the routes its Examples use, puts the
// URLs in package variables that the Examples read, and runs the tests through Run:
//
//	var exampleAPIURL, examplePDPURL string
//
//	func TestMain(m *testing.M) {
//		servers := mockpermit.StartExample()
//		exampleAPIURL, examplePDPURL = servers.API.URL, servers.PDP.URL
//		servers.PDP.Handle("POST /allowed", mockpermit.Reply{Body: `{"allow": true}`})
//		os.Exit(servers.Run(m))
//	}
//
// The servers check requests as those of NewAPI and NewPDP do, and collect each failure instead
// of failing a test; Run reports them. go test -run can leave Examples out and -count can run
// them again, so a route's replies start over after the last one, and a reply that no request
// used is not a failure: an Example checks its output instead.
func StartExample() *ExampleServers {
	failures := &collector{}
	e := &ExampleServers{
		API:      &Server{name: "API", report: failures, cycle: true},
		PDP:      &Server{name: "PDP", report: failures, cycle: true},
		failures: failures,
	}
	e.API.start()
	e.PDP.start()
	return e
}

// Run runs the tests and Examples of the package with m, stops the servers, writes each failure
// they collected to standard error, and returns the exit code for TestMain to pass to os.Exit:
// the code of m.Run when it is not 0, else 1 when the servers collected a failure, else 0.
func (e *ExampleServers) Run(m *testing.M) int {
	return e.finish(os.Stderr, m.Run())
}

// finish stops the servers, writes each failure they collected to w, and returns the exit code
// for a run of the tests that exited with code.
func (e *ExampleServers) finish(w io.Writer, code int) int {
	e.API.finish()
	e.PDP.finish()
	failures := e.failures.take()
	// The exit code reports the failures even when w cannot take them, so a write error is
	// left out.
	for _, failure := range failures {
		_, _ = fmt.Fprintln(w, failure)
	}
	if code != 0 {
		return code
	}
	if len(failures) > 0 {
		_, _ = fmt.Fprintf(w, "FAIL: the mockpermit servers of the Examples failed the run; "+
			"problems found: %d\n", len(failures))
		return 1
	}
	return 0
}

// collector gathers the failures of the servers that StartExample starts, which have no
// testing.TB.
type collector struct {
	mu       sync.Mutex
	failures []string
}

func (c *collector) Helper() {}

func (c *collector) Errorf(format string, args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failures = append(c.failures, fmt.Sprintf(format, args...))
}

// take returns the failures collected so far and forgets them.
func (c *collector) take() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	failures := c.failures
	c.failures = nil
	return failures
}
