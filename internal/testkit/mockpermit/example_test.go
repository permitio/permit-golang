package mockpermit

import (
	"net/http"
	"strings"
	"testing"
)

// startExample starts the servers of StartExample and stops them when t ends, in case the test
// did not.
func startExample(t *testing.T) *ExampleServers {
	t.Helper()
	e := StartExample()
	t.Cleanup(e.API.srv.Close)
	t.Cleanup(e.PDP.srv.Close)
	return e
}

func TestStartExampleUnknownRoute(t *testing.T) {
	e := startExample(t)
	got := call{method: http.MethodGet, path: "/v2/unknown"}.send(t, newClient(t), e.API)
	if got.status != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", got.status)
	}

	var report strings.Builder
	code := e.finish(&report, 0)

	if code != 1 {
		t.Errorf("finish() = %d after a passing run, want 1", code)
	}
	want := "mockpermit API: GET /v2/unknown: no route; register it with Handle if the SDK " +
		"should send this request\nFAIL: the mockpermit servers of the Examples failed the run; " +
		"problems found: 1\n"
	if report.String() != want {
		t.Errorf("finish() wrote %q, want %q", report.String(), want)
	}
}

func TestStartExampleBothServers(t *testing.T) {
	e := startExample(t)
	client := newClient(t)
	call{method: http.MethodPost, path: "/allowed", auth: []string{"bearer " + APIKey}}.send(t,
		client, e.PDP)
	call{method: http.MethodGet, path: "/v2//x"}.send(t, client, e.API)

	var report strings.Builder
	code := e.finish(&report, 0)

	lines := strings.Split(strings.TrimSuffix(report.String(), "\n"), "\n")
	if code != 1 || len(lines) != 3 ||
		!strings.HasPrefix(lines[0], "mockpermit PDP: POST /allowed: the Authorization header") ||
		!strings.HasPrefix(lines[1], "mockpermit API: GET /v2//x: the path has an empty") ||
		lines[2] != "FAIL: the mockpermit servers of the Examples failed the run; "+
			"problems found: 2" {
		t.Errorf("finish() = %d and wrote %q, want 1 and both failures", code, report.String())
	}
}

func TestStartExampleRepliesStartOver(t *testing.T) {
	e := startExample(t)
	e.PDP.Handle("POST /allowed", Reply{Body: `{"allow":true}`}, Reply{Body: `{"allow":false}`})
	e.API.Handle("GET /v2/api-key/scope", Reply{})
	client := newClient(t)

	var bodies []string
	for range 3 {
		bodies = append(bodies, call{method: http.MethodPost, path: "/allowed"}.send(t, client,
			e.PDP).body)
	}

	if want := `{"allow":true} {"allow":false} {"allow":true}`; strings.Join(bodies, " ") != want {
		t.Errorf("the replies were %q, want %s", bodies, want)
	}
	var report strings.Builder
	if code := e.finish(&report, 0); code != 0 || report.String() != "" {
		t.Errorf("finish() = %d and wrote %q, want 0 and nothing: an unused reply is no failure "+
			"for Examples", code, report.String())
	}
}

func TestStartExampleExitCode(t *testing.T) {
	e := startExample(t)
	var report strings.Builder
	if code := e.finish(&report, 2); code != 2 || report.String() != "" {
		t.Errorf("finish(2) = %d and wrote %q, want 2 and nothing", code, report.String())
	}

	e = startExample(t)
	call{method: http.MethodGet, path: "/v2/unknown"}.send(t, newClient(t), e.API)
	report.Reset()
	if code := e.finish(&report, 3); code != 3 || !strings.Contains(report.String(), "no route") {
		t.Errorf("finish(3) = %d and wrote %q, want 3 and the failure", code, report.String())
	}
}

func TestStartExampleUnfiredFault(t *testing.T) {
	e := startExample(t)
	e.API.Handle("GET /v2/x", Reply{})
	e.API.FailRequests("GET /v2/x", http.StatusInternalServerError, 0)

	var report strings.Builder
	if code := e.finish(&report, 0); code != 1 ||
		!strings.Contains(
			report.String(),
			`FailRequests("GET /v2/x", 500, 0) answered no request`,
		) {
		t.Errorf("finish() = %d and wrote %q, want 1 and the fault", code, report.String())
	}
}
