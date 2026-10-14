package permit

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// proxyChildEnv is set in the environment of the test binary that TestProxyFromEnvironment runs
// again, so that the test makes the call through the proxy there.
const proxyChildEnv = "PERMIT_TEST_PROXY_CHILD"

// proxiedURL is the API URL of the call through the proxy: a host that is not loopback, which
// http.ProxyFromEnvironment sends through the proxy, and that no resolver knows, as the proxy
// answers for it.
const proxiedURL = "http://api.permit.test"

// proxyEnv are the environment variables that the child must not inherit: those that choose a
// proxy, or that make http.ProxyFromEnvironment ignore one, as REQUEST_METHOD does, and those
// that configure a client or the request record.
var proxyEnv = []string{
	"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy",
	"REQUEST_METHOD", envAPIKey, envAPIURL, envPDPURL, mockpermit.RecordEnv, proxyChildEnv,
}

// TestProxyFromEnvironment checks that a client's own http.Client uses the proxy that
// HTTP_PROXY names. http.ProxyFromEnvironment reads the environment once in a process, and never
// sends a request to a loopback host through a proxy, so the test runs the test binary again
// with HTTP_PROXY set before the first request, and the child calls a host that is not loopback.
// A local server is the proxy and answers the call, so no request leaves the machine.
func TestProxyFromEnvironment(t *testing.T) {
	//nolint:forbidigo // The test reads the variable that it sets for the child it runs.
	if os.Getenv(proxyChildEnv) == "1" {
		callThroughTheProxy(t)
		return
	}
	clearEnv(t)
	if c := mustNew(t, localOptions()...); funcPointer(c.transport.Proxy) !=
		funcPointer(http.ProxyFromEnvironment) {
		t.Error("the client's transport does not take its proxy from the environment")
	}
	var (
		mu   sync.Mutex
		seen []string
	)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.String()+" "+r.Header.Get("Authorization"))
		mu.Unlock()
		_, _ = io.WriteString(w, `{"key":"proxied"}`)
	}))
	t.Cleanup(proxy.Close)

	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestProxyFromEnvironment$",
		"-test.count=1", "-test.v")
	child.Env = append(childEnv(os.Environ()), proxyChildEnv+"=1", "HTTP_PROXY="+proxy.URL)
	out, err := child.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "--- PASS: TestProxyFromEnvironment") {
		t.Fatalf("the child test failed: %v\n%s", err, out)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{"GET " + proxiedURL + "/__test/objects/proxied Bearer " + mockpermit.APIKey}
	if !slices.Equal(seen, want) {
		t.Errorf("the proxy saw %q, want %q", seen, want)
	}
}

// childEnv returns env without the variables of proxyEnv.
func childEnv(env []string) []string {
	return slices.DeleteFunc(slices.Clone(env), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return slices.Contains(proxyEnv, name)
	})
}

// callThroughTheProxy is the child's test: one call to proxiedURL, which the proxy answers.
func callThroughTheProxy(t *testing.T) {
	t.Helper()
	c := urlClient(t, mockpermit.APIKey, proxiedURL)
	got, err := do[testObject](t.Context(), c, get("proxied"))
	if err != nil || got.Key != "proxied" {
		t.Fatalf("the call through the proxy returned %v, %v", got, err)
	}
}

func TestChildEnv(t *testing.T) {
	got := childEnv([]string{
		"PATH=/bin", "HTTP_PROXY=http://127.0.0.1:9", "https_proxy=x", "NO_PROXY=*",
		"REQUEST_METHOD=GET", "PERMIT_API_KEY=k", "PERMIT_API_COVERAGE_RECORD=r",
		proxyChildEnv + "=1", "HOME=/home/a=b",
	})
	if want := []string{"PATH=/bin", "HOME=/home/a=b"}; !slices.Equal(got, want) {
		t.Errorf("childEnv = %q, want %q", got, want)
	}
}
