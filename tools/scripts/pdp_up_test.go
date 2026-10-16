package scripts

import (
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// dockerStub stands in for docker. It appends its arguments and the PDP's settings in its
// environment to the file STUB_LOG names, and acts by its first arguments:
//
//   - pull, image inspect and run succeed, and image inspect prints a digest;
//   - container inspect prints STUB_STATE, running when it is not set;
//   - logs prints a PDP's log with the lines of two health checks and a line that looks like a
//     workflow command;
//   - rm prints STUB_RM to standard error and fails when it is set.
const dockerStub = `#!/bin/sh
printf '%s|PDP_API_KEY=%s|PDP_CONTROL_PLANE=%s\n' "$*" "${PDP_API_KEY-unset}" \
	"${PDP_CONTROL_PLANE-unset}" >>"$STUB_LOG"
case "$1 $2" in
"pull "*) echo "pulled" ;;
"image inspect") echo "permitio/pdp-v2@sha256:0123456789abcdef" ;;
"run "*) echo "0123456789ab" ;;
"container inspect") echo "${STUB_STATE:-running}" ;;
"logs "*)
	echo 'INFO: the PDP started'
	echo 'INFO: 127.0.0.1:50000 - "GET /healthy HTTP/1.1" 200 OK'
	echo '::warning::a line of the PDP'
	echo 'INFO: 127.0.0.1:50001 - "GET /ready HTTP/1.1" 200 OK'
	echo 'INFO: the policy is synced'
	;;
"rm "*)
	if [ -n "$STUB_RM" ]; then
		echo "$STUB_RM" >&2
		exit 1
	fi
	echo permit-e2e-pdp
	;;
*) exit 99 ;;
esac
`

// pinnedImage is the image the tests start, with a digest.
const pinnedImage = "permitio/pdp-v2:0.9.16@sha256:" +
	"e3cf30794ec2d256636b4714641df46e51ee58a3f1f0d24c606e214e0bf8669a"

// pdpRun is a run of pdp-up.sh: its result, what the stub docker logged, and how long it took.
type pdpRun struct {
	scriptResult
	docker  string
	elapsed time.Duration
}

// runPDPUp runs pdp-up.sh with args and the stub docker first on PATH: with the pinned image,
// the environment's key, the PDP's port at port and vars replacing or adding to that.
func runPDPUp(t *testing.T, port string, vars map[string]string, args ...string) pdpRun {
	t.Helper()
	root := t.TempDir()
	linkScripts(t, root, "pdp-up.sh")
	writeExecutable(t, root, "stub/docker", dockerStub)
	log := filepath.Join(root, "docker.log")
	inherited := os.Getenv("PATH") //nolint:forbidigo // the stub goes before it
	all := map[string]string{
		"PATH":     filepath.Join(root, "stub") + string(os.PathListSeparator) + inherited,
		"STUB_LOG": log, "STUB_STATE": "", "STUB_RM": "", "PDP_IMAGE": pinnedImage,
		"PDP_PORT": port, "PDP_HEALTH_TIMEOUT": "", "PERMIT_E2E_ENV_API_KEY": fakeEnvKey,
		"PERMIT_E2E_API_URL": "",
	}
	maps.Copy(all, loopbackEnv)
	maps.Copy(all, vars)
	var env []string
	for name, value := range all {
		env = append(env, name+"="+value)
	}
	start := time.Now()
	result := runScript(t, root, env, filepath.Join(root, ".github", "scripts", "pdp-up.sh"),
		args...)
	return pdpRun{scriptResult: result, docker: envFile(t, log), elapsed: time.Since(start)}
}

// pdpHealth is the /healthy of a stand-in PDP: its port, how many times it was asked, and when
// it was first asked.
type pdpHealth struct {
	port  string
	asked atomic.Int64
	mu    sync.Mutex
	first time.Time
}

// firstAsked returns when /healthy was first asked, or the zero time.
func (h *pdpHealth) firstAsked() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.first
}

// startPDPHealth starts a server whose /healthy answers as answer does, given how many times it
// has been asked, this time included.
func startPDPHealth(t *testing.T, answer func(w http.ResponseWriter, r *http.Request,
	asked int64),
) *pdpHealth {
	t.Helper()
	h := &pdpHealth{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthy" {
			t.Errorf("the PDP was asked %s, want /healthy", r.URL.Path)
		}
		h.mu.Lock()
		if h.first.IsZero() {
			h.first = time.Now()
		}
		h.mu.Unlock()
		answer(w, r, h.asked.Add(1))
	}))
	t.Cleanup(server.Close)
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	h.port = port
	return h
}

// startHealth starts a server whose /healthy answers 503 until it has answered failures times,
// and 200 after that.
func startHealth(t *testing.T, failures int64) *pdpHealth {
	t.Helper()
	return startPDPHealth(t, func(w http.ResponseWriter, _ *http.Request, asked int64) {
		if asked <= failures {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
}

// TestPDPUp checks that up starts the image with the environment's key passed by name, on the
// loopback address only, prints the digest it resolved to, and waits until /healthy answers 200.
func TestPDPUp(t *testing.T) {
	t.Parallel()
	health := startHealth(t, 1)
	port := health.port

	run := runPDPUp(t, port, map[string]string{"PERMIT_E2E_API_URL": "https://api.example.com"},
		"up")

	if run.exit != 0 {
		t.Fatalf("exit = %d, want 0; stdout:\n%s\nstderr:\n%s", run.exit, run.stdout, run.stderr)
	}
	if got := health.asked.Load(); got != 2 {
		t.Errorf("/healthy was asked %d times, want 2", got)
	}
	wantRun := "run --detach --name permit-e2e-pdp --publish 127.0.0.1:" + port +
		":7000 --env PDP_API_KEY --env PDP_CONTROL_PLANE " + pinnedImage + "|PDP_API_KEY=" +
		fakeEnvKey + "|PDP_CONTROL_PLANE=https://api.example.com\n"
	if !strings.Contains(run.docker, "pull --quiet "+pinnedImage+"|") ||
		!strings.Contains(run.docker, wantRun) {
		t.Errorf("docker was run with:\n%s\nwant a pull, and the line %q", run.docker, wantRun)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(run.docker), "\n") {
		args, _, _ := strings.Cut(line, "|")
		if strings.Contains(args, fakeEnvKey) {
			t.Errorf("the environment's key is on docker's command line: %s", args)
		}
	}
	for _, want := range []string{
		"pdp-up: " + pinnedImage + " resolved to permitio/pdp-v2@sha256:0123456789abcdef\n",
		"pdp-up: http://127.0.0.1:" + port + "/healthy answered 200 after ",
	} {
		if !strings.Contains(run.stdout, want) {
			t.Errorf("stdout does not contain %q:\n%s", want, run.stdout)
		}
	}
	assertNoSecret(t, run.scriptResult)
}

// TestPDPUpDefaults checks the settings that up takes when they are not set: the PDP syncs from
// https://api.permit.io, listens on 127.0.0.1:7766, the port of the SDK's default PDP URL, and
// has 300 seconds to become healthy. The container has stopped, so up fails before it asks
// /healthy on that fixed port.
func TestPDPUpDefaults(t *testing.T) {
	t.Parallel()

	run := runPDPUp(t, "", map[string]string{"STUB_STATE": "exited"}, "up")

	if run.exit != 2 {
		t.Errorf("exit = %d, want 2, as the container stopped; stdout:\n%s", run.exit, run.stdout)
	}
	wantRun := "run --detach --name permit-e2e-pdp --publish 127.0.0.1:7766:7000 --env " +
		"PDP_API_KEY --env PDP_CONTROL_PLANE " + pinnedImage + "|PDP_API_KEY=" + fakeEnvKey +
		"|PDP_CONTROL_PLANE=https://api.permit.io\n"
	if !strings.Contains(run.docker, wantRun) {
		t.Errorf("docker was run with:\n%s\nwant the line %q", run.docker, wantRun)
	}
	want := "pdp-up: waiting up to 300 seconds for http://127.0.0.1:7766/healthy to answer 200\n"
	if !strings.Contains(run.stdout, want) {
		t.Errorf("stdout does not contain %q:\n%s", want, run.stdout)
	}
}

// TestPDPUpBoundedByTime checks that the wait for /healthy ends once PDP_HEALTH_TIMEOUT seconds
// have passed, however many tries that is, and also when the PDP never answers: not before, and
// at most the two seconds that bash's whole-second clock may add and one try after.
func TestPDPUpBoundedByTime(t *testing.T) {
	t.Parallel()
	const (
		timeout = 3 * time.Second
		// maxTry is the longest that pdp-up.sh waits for one answer of /healthy.
		maxTry = 5 * time.Second
	)
	unhealthy := func(w http.ResponseWriter, _ *http.Request, _ int64) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	hanging := func(_ http.ResponseWriter, r *http.Request, _ int64) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Minute):
		}
	}
	for name, answer := range map[string]func(http.ResponseWriter, *http.Request, int64){
		"unhealthy": unhealthy, "hanging": hanging,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			health := startPDPHealth(t, answer)

			run := runPDPUp(t, health.port, map[string]string{"PDP_HEALTH_TIMEOUT": "3"}, "up")
			end := time.Now()

			want := "::error title=PDP::http://127.0.0.1:" + health.port + "/healthy did not " +
				"answer 200 within 3 seconds"
			if run.exit != 2 || !strings.Contains(run.stdout, want) {
				t.Errorf("exit = %d, want 2 with %q; stdout:\n%s", run.exit, want, run.stdout)
			}
			if run.elapsed < timeout {
				t.Errorf("up took %s, want at least the %s of PDP_HEALTH_TIMEOUT", run.elapsed,
					timeout)
			}
			first := health.firstAsked()
			if first.IsZero() {
				t.Fatal("up never asked /healthy")
			}
			if limit := timeout + 2*time.Second + maxTry; end.Sub(first) > limit {
				t.Errorf("up waited %s after its first try, want at most %s: the %s of "+
					"PDP_HEALTH_TIMEOUT, two seconds of the clock's resolution and one try",
					end.Sub(first), limit, timeout)
			}
		})
	}
}

// TestPDPUpStopped checks that up fails at once when the container stops running.
func TestPDPUpStopped(t *testing.T) {
	t.Parallel()
	health := startHealth(t, 0)

	run := runPDPUp(t, health.port, map[string]string{"STUB_STATE": "exited"}, "up")

	want := "::error title=PDP::the PDP container permit-e2e-pdp is exited, not running"
	if run.exit != 2 || !strings.Contains(run.stdout, want) || health.asked.Load() != 0 {
		t.Errorf("exit = %d, want 2 with %q and no health check; stdout:\n%s", run.exit, want,
			run.stdout)
	}
}

// TestPDPLogs checks that logs prints the PDP's log without the lines of the health checks,
// with workflow commands stopped.
func TestPDPLogs(t *testing.T) {
	t.Parallel()

	run := runPDPUp(t, "7766", nil, "logs")

	if run.exit != 0 {
		t.Fatalf("exit = %d, want 0; stdout:\n%s", run.exit, run.stdout)
	}
	for _, want := range []string{"the PDP started", "the policy is synced"} {
		if !strings.Contains(run.stdout, want) {
			t.Errorf("stdout does not contain %q:\n%s", want, run.stdout)
		}
	}
	for _, health := range []string{"GET /healthy", "GET /ready"} {
		if strings.Contains(run.stdout, health) {
			t.Errorf("stdout has the health check %q:\n%s", health, run.stdout)
		}
	}
	assertWrapped(t, run.stdout, "::warning::a line of the PDP")
	if !strings.Contains(run.docker, "logs permit-e2e-pdp|") {
		t.Errorf("docker was run with:\n%s\nwant logs permit-e2e-pdp", run.docker)
	}
}

// TestPDPDown checks that down removes the container, and passes when there is none.
func TestPDPDown(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, rm string
		wantExit int
		want     string
	}{
		{name: "removed", wantExit: 0, want: "pdp-up: removed permit-e2e-pdp\n"},
		{
			name: "no container", wantExit: 0,
			rm:   "Error response from daemon: No such container: permit-e2e-pdp",
			want: "pdp-up: permit-e2e-pdp does not exist, so there is nothing to remove\n",
		},
		{
			name: "docker fails", wantExit: 2, rm: "permission denied",
			want: "::error title=PDP::docker rm --force permit-e2e-pdp failed\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := runPDPUp(t, "7766", map[string]string{"STUB_RM": tt.rm}, "down")

			if run.exit != tt.wantExit || !strings.Contains(run.stdout, tt.want) {
				t.Errorf("exit = %d, want %d with %q; stdout:\n%s", run.exit, tt.wantExit,
					tt.want, run.stdout)
			}
			if !strings.Contains(run.docker, "rm --force permit-e2e-pdp|") {
				t.Errorf("docker was run with:\n%s\nwant rm --force permit-e2e-pdp", run.docker)
			}
		})
	}
}

// TestPDPUpDidNotRun checks the arguments and settings without which pdp-up.sh does not run, and
// that it starts no container then.
func TestPDPUpDidNotRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
		vars map[string]string
		want string
	}{
		{name: "no command", want: "usage: pdp-up.sh {up | logs | down}"},
		{name: "two commands", args: []string{"up", "down"}, want: "usage: pdp-up.sh"},
		{
			name: "no key", args: []string{"up"},
			vars: map[string]string{"PERMIT_E2E_ENV_API_KEY": ""},
			want: "PERMIT_E2E_ENV_API_KEY is not set; e2e-env.sh create exports it",
		},
		{
			name: "no image", args: []string{"up"}, vars: map[string]string{"PDP_IMAGE": ""},
			want: "PDP_IMAGE is not an image reference",
		},
		{
			name: "an image with a space", args: []string{"up"},
			vars: map[string]string{"PDP_IMAGE": "permitio/pdp-v2 --privileged"},
			want: "PDP_IMAGE is not an image reference",
		},
		{
			name: "a port that is not a number", args: []string{"up"},
			vars: map[string]string{"PDP_PORT": "77a"}, want: "PDP_PORT is not a port number",
		},
		{
			name: "a timeout that is not a number", args: []string{"up"},
			vars: map[string]string{"PDP_HEALTH_TIMEOUT": "5m"},
			want: "PDP_HEALTH_TIMEOUT is not a number of seconds",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := runPDPUp(t, "7766", tt.vars, tt.args...)

			if run.exit != 2 || !strings.Contains(run.stdout, "::error title=PDP::"+tt.want) {
				t.Errorf("exit = %d, want 2 with %q; stdout:\n%s", run.exit, tt.want, run.stdout)
			}
			if run.docker != "" {
				t.Errorf("docker was run with:\n%s", run.docker)
			}
		})
	}
}
