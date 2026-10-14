package mockpermit

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

// TestExampleServersRunFailsTheRun builds the test binary of testdata/exitcode, whose Example
// passes while the API server records a request for a route it does not have, and runs it: Run
// must make the binary exit 1 and write the problem to standard error. The go command runs
// offline, without a go.work file.
func TestExampleServersRunFailsTheRun(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "exitcode.test")
	build := exec.CommandContext(t.Context(), "go", "test", "-c", "-o", binary,
		"./testdata/exitcode")
	build.Env = slices.Concat(os.Environ(),
		[]string{"GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off"})
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go test -c ./testdata/exitcode: %v\n%s", err, out)
	}

	run := exec.CommandContext(t.Context(), binary, "-test.v", "-test.count=1")
	var stdout, stderr bytes.Buffer
	run.Stdout, run.Stderr = &stdout, &stderr
	err := run.Run()

	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Errorf("the test binary returned %v, want exit status 1", err)
	}
	if out := stdout.String(); !strings.Contains(out, "--- PASS: Example") ||
		!strings.HasSuffix(out, "PASS\n") {
		t.Errorf("the test binary wrote\n%s\nto standard output; want the Example and the "+
			"run to pass, so that only the servers fail it", out)
	}
	want := "mockpermit API: GET /v2/unknown: no route; register it with Handle if the SDK " +
		"should send this request\nFAIL: the mockpermit servers of the Examples failed the run; " +
		"problems found: 1\n"
	if stderr.String() != want {
		t.Errorf("the test binary wrote %q to standard error, want %q", stderr.String(), want)
	}
}
