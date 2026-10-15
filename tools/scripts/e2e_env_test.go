package scripts

import (
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// The API keys of the fake Permit API: the project-level key the scripts send, and the API key
// of the scratch environment that it creates.
const (
	fakeProjectKey = "permit_key_FAKEproject"
	fakeEnvKey     = "permit_key_FAKEenvironment"
)

// permitRequest is a request that a fake Permit API received.
type permitRequest struct {
	method, path, query, auth, body string
}

// fakePermit is a fake of the Permit API, which answers each request with reply and keeps it.
type fakePermit struct {
	mu       sync.Mutex
	requests []permitRequest
}

// startFakePermit starts a fake of the Permit API whose replies reply gives, and returns it with
// its URL.
func startFakePermit(t *testing.T, reply func(r permitRequest) (int, string)) (*fakePermit,
	string,
) {
	t.Helper()
	f := &fakePermit{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading the body of %s %s: %v", r.Method, r.URL.Path, err)
		}
		req := permitRequest{
			method: r.Method, path: r.URL.Path, query: r.URL.RawQuery,
			auth: r.Header.Get("Authorization"), body: string(body),
		}
		f.mu.Lock()
		f.requests = append(f.requests, req)
		f.mu.Unlock()
		status, text := reply(req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := io.WriteString(w, text); err != nil {
			t.Errorf("writing the reply to %s %s: %v", r.Method, r.URL.Path, err)
		}
	}))
	t.Cleanup(server.Close)
	return f, server.URL
}

// all returns the requests the fake received, in order.
func (f *fakePermit) all() []permitRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests)
}

// received returns the method and path of each request the fake received, in order.
func (f *fakePermit) received() []string {
	var routes []string
	for _, r := range f.all() {
		routes = append(routes, r.method+" "+r.path)
	}
	return routes
}

// loopbackEnv lets curl reach a local server directly while the proxy variables point every
// other request at an address that refuses connections.
var loopbackEnv = map[string]string{"NO_PROXY": "127.0.0.1", "no_proxy": "127.0.0.1"}

// envFile returns the content of a file the script wrote, or "" when it does not exist or is a
// directory, which the script cannot write to.
func envFile(t *testing.T, path string) string {
	t.Helper()
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// e2eEnvRun is a run of e2e-env.sh: its result and the files it wrote.
type e2eEnvRun struct {
	scriptResult
	githubEnv, githubOutput string
}

// runE2EEnv runs e2e-env.sh with args against the API at apiURL: as a push to master of run 1234,
// attempt 2, with the project gosdk-e2e and its key, and with vars replacing or adding to that.
func runE2EEnv(t *testing.T, apiURL string, vars map[string]string, args ...string) e2eEnvRun {
	t.Helper()
	root := t.TempDir()
	copyScripts(t, root, "e2e-env.sh")
	all := map[string]string{
		"PERMIT_E2E_PROJECT_KEY": fakeProjectKey, "PERMIT_E2E_PROJECT": "gosdk-e2e",
		"PERMIT_E2E_API_URL": apiURL, "GITHUB_RUN_ID": "1234", "GITHUB_RUN_ATTEMPT": "2",
		"GITHUB_ENV":        filepath.Join(root, "github-env"),
		"GITHUB_OUTPUT":     filepath.Join(root, "github-output"),
		"GITHUB_EVENT_NAME": "push", "GITHUB_REF": "refs/heads/master", "MODE": "",
	}
	maps.Copy(all, loopbackEnv)
	maps.Copy(all, vars)
	var env []string
	for name, value := range all {
		env = append(env, name+"="+value)
	}
	result := runScript(t, root, env, filepath.Join(root, ".github", "scripts", "e2e-env.sh"),
		args...)
	return e2eEnvRun{
		scriptResult: result,
		githubEnv:    envFile(t, all["GITHUB_ENV"]), githubOutput: envFile(t, all["GITHUB_OUTPUT"]),
	}
}

// The routes of the scratch environment gosdk-ci-1234-2-pinned of the project gosdk-e2e.
const (
	envsPath   = "/v2/projects/gosdk-e2e/envs"
	envPath    = envsPath + "/gosdk-ci-1234-2-pinned"
	apiKeyPath = "/v2/api-key/project-id-1/env-id-1"
	createdEnv = `{"id": "env-id-1", "key": "gosdk-ci-1234-2-pinned", "project_id": "project-id-1",
		"name": "gosdk-ci-1234-2-pinned"}`
)

// permitReplies answers the requests of e2e-env.sh as the Permit API does, with the status
// that statuses gives a route, such as "GET " + apiKeyPath, instead of its success.
func permitReplies(statuses map[string]int) func(r permitRequest) (int, string) {
	return func(r permitRequest) (int, string) {
		route := r.method + " " + r.path
		if status, ok := statuses[route]; ok {
			return status, `{"detail": "the fake's reply ::warning::not a command"}`
		}
		switch route {
		case "POST " + envsPath:
			return http.StatusCreated, createdEnv
		case "GET " + apiKeyPath:
			return http.StatusOK, `{"id": "key-id-1", "secret": "` + fakeEnvKey + `"}`
		case "DELETE " + envPath:
			return http.StatusNoContent, ""
		}
		return http.StatusNotFound, `{"detail": "not found"}`
	}
}

// assertNoSecret fails when the output shows the project's key, or the environment's key outside
// the one ::add-mask:: line.
func assertNoSecret(t *testing.T, result scriptResult) {
	t.Helper()
	output := strings.Replace(result.stdout, "::add-mask::"+fakeEnvKey+"\n", "", 1) +
		result.stderr
	for _, secret := range []string{fakeProjectKey, fakeEnvKey} {
		if strings.Contains(output, secret) {
			t.Errorf("the output shows the key %s:\n%s", secret, output)
		}
	}
}

// curlArgsStub stands in for curl: it appends its arguments to the file CURL_LOG names, a line a
// call, and runs the curl that REAL_CURL names with them.
const curlArgsStub = `#!/bin/sh
printf '%s\n' "$*" >>"$CURL_LOG"
exec "$REAL_CURL" "$@"
`

// logCurlArgs returns the settings that put curlArgsStub first on PATH, and the path of its log.
func logCurlArgs(t *testing.T) (map[string]string, string) {
	t.Helper()
	curl, err := exec.LookPath("curl")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeExecutable(t, dir, "stub/curl", curlArgsStub)
	log := filepath.Join(dir, "curl.log")
	inherited := os.Getenv("PATH") //nolint:forbidigo // the stub goes before it
	return map[string]string{
		"PATH":     filepath.Join(dir, "stub") + string(os.PathListSeparator) + inherited,
		"CURL_LOG": log, "REAL_CURL": curl,
	}, log
}

// assertKeyNotInArgs fails when the stub curl that logged to log never ran, or ran with an API
// key on its command line, where other processes of the runner could read it.
func assertKeyNotInArgs(t *testing.T, log string) {
	t.Helper()
	args := envFile(t, log)
	if args == "" {
		t.Fatal("the script did not run curl from PATH")
	}
	for _, secret := range []string{fakeProjectKey, fakeEnvKey} {
		if strings.Contains(args, secret) {
			t.Errorf("curl ran with the key %s on its command line:\n%s", secret, args)
		}
	}
}

// TestE2EEnvCreate checks that create makes the scratch environment, reads its API key, masks
// the key before it exports it, and exports the environment's settings. The project's key goes
// to curl in a file, not on its command line.
func TestE2EEnvCreate(t *testing.T) {
	fake, url := startFakePermit(t, permitReplies(nil))
	curlVars, curlLog := logCurlArgs(t)

	run := runE2EEnv(t, url, curlVars, "create", "pinned")

	if run.exit != 0 {
		t.Fatalf("exit = %d, want 0; stdout:\n%s\nstderr:\n%s", run.exit, run.stdout, run.stderr)
	}
	want := []string{"POST " + envsPath, "GET " + apiKeyPath}
	if got := fake.received(); !slices.Equal(got, want) {
		t.Fatalf("requests = %q, want %q", got, want)
	}
	requests := fake.all()
	var body map[string]string
	if err := json.Unmarshal([]byte(requests[0].body), &body); err != nil {
		t.Fatalf("the body of the create is not JSON: %v", err)
	}
	if body["key"] != "gosdk-ci-1234-2-pinned" || body["name"] != "gosdk-ci-1234-2-pinned" ||
		body["description"] == "" {
		t.Errorf("the create sent %q, want the key and name gosdk-ci-1234-2-pinned and a "+
			"description", body)
	}
	for _, r := range requests {
		if r.auth != "Bearer "+fakeProjectKey {
			t.Errorf("%s %s has the Authorization %q, want the project's key", r.method, r.path,
				r.auth)
		}
	}
	wantEnv := "PERMIT_E2E_ENV_API_KEY=" + fakeEnvKey + "\nPERMIT_E2E_ENV_ID=env-id-1\n" +
		"PERMIT_E2E_PROJECT_ID=project-id-1\n"
	if run.githubEnv != wantEnv || run.githubOutput != "created=true\n" {
		t.Errorf("GITHUB_ENV = %q, GITHUB_OUTPUT = %q; want %q and created=true", run.githubEnv,
			run.githubOutput, wantEnv)
	}
	mask := strings.Index(run.stdout, "::add-mask::"+fakeEnvKey+"\n")
	if mask < 0 {
		t.Fatalf("stdout has no ::add-mask:: line for the environment's key:\n%s", run.stdout)
	}
	for _, block := range stopCommandsPattern.FindAllStringIndex(run.stdout, -1) {
		if mask >= block[0] && mask < block[1] {
			t.Errorf("the ::add-mask:: line is printed with workflow commands stopped:\n%s",
				run.stdout)
		}
	}
	assertWrapped(t, run.stdout, "created the environment gosdk-ci-1234-2-pinned")
	assertNoSecret(t, run.scriptResult)
	assertKeyNotInArgs(t, curlLog)
}

// TestE2EEnvMasksBeforeExport checks that the API key is masked before the script writes it to
// GITHUB_ENV: when the write fails, the mask is out already.
func TestE2EEnvMasksBeforeExport(t *testing.T) {
	_, url := startFakePermit(t, permitReplies(nil))
	readOnly := t.TempDir()

	run := runE2EEnv(t, url, map[string]string{"GITHUB_ENV": readOnly}, "create", "pinned")

	if run.exit != 2 || !strings.Contains(run.stdout, "::add-mask::"+fakeEnvKey+"\n") ||
		!strings.Contains(run.stdout, "writing the environment's settings to GITHUB_ENV failed") {
		t.Errorf("exit = %d, want 2 after the ::add-mask:: line, with the failed write; "+
			"stdout:\n%s\nstderr:\n%s", run.exit, run.stdout, run.stderr)
	}
	assertNoSecret(t, run.scriptResult)
}

// TestE2EEnvCreateFails checks the replies on which create does not run, the key level its
// message names after a 401 or 403, and that it never exports a key then.
func TestE2EEnvCreateFails(t *testing.T) {
	const keyLevel = "The end-to-end jobs need a project-level API key of the project gosdk-e2e"
	tests := []struct {
		name     string
		statuses map[string]int
		reply    func(r permitRequest) (int, string)
		want     []string
		requests int
	}{
		{
			name: "a 401 to the create", statuses: map[string]int{"POST " + envsPath: 401},
			requests: 1,
			want: []string{
				"POST " + envsPath + " answered 401: PERMIT_E2E_PROJECT_KEY cannot create " +
					"environments in it", keyLevel,
			},
		},
		{
			name: "a 403 to the create", statuses: map[string]int{"POST " + envsPath: 403},
			requests: 1, want: []string{"answered 403", keyLevel},
		},
		{
			name: "a 403 to the read of the API key", requests: 2,
			statuses: map[string]int{"GET " + apiKeyPath: 403},
			want: []string{
				"GET " + apiKeyPath + " answered 403: PERMIT_E2E_PROJECT_KEY cannot read the API " +
					"key of the environment", keyLevel,
			},
		},
		{
			name: "a 401 to the read of the API key", requests: 2,
			statuses: map[string]int{"GET " + apiKeyPath: 401}, want: []string{keyLevel},
		},
		{
			name: "a 500 to the create", statuses: map[string]int{"POST " + envsPath: 500},
			requests: 1,
			want: []string{
				"POST " + envsPath + " answered 500, so the environment gosdk-ci-1234-2-pinned " +
					"was not created",
			},
		},
		{
			name: "a 409 to the create", statuses: map[string]int{"POST " + envsPath: 409},
			requests: 1, want: []string{"answered 409"},
		},
		{
			name: "a 500 to the read of the API key", requests: 2,
			statuses: map[string]int{"GET " + apiKeyPath: 500},
			want:     []string{"answered 500, so the environment's API key could not be read"},
		},
		{
			name: "a created environment without a project ID", requests: 1,
			reply: func(r permitRequest) (int, string) {
				return http.StatusCreated, `{"id": "env-id-1", "key": "gosdk-ci-1234-2-pinned"}`
			},
			want: []string{"did not reply with the environment gosdk-ci-1234-2-pinned, its ID"},
		},
		{
			name: "an environment of another key", requests: 1,
			reply: func(r permitRequest) (int, string) {
				return http.StatusCreated, `{"id": "env-id-1", "key": "other",
					"project_id": "project-id-1"}`
			},
			want: []string{"did not reply with the environment gosdk-ci-1234-2-pinned"},
		},
		{
			name: "an API key with a space", requests: 2,
			reply: func(r permitRequest) (int, string) {
				if r.method == http.MethodGet {
					return http.StatusOK, `{"secret": "permit key FAKE"}`
				}
				return http.StatusCreated, createdEnv
			},
			want: []string{"did not reply with a secret of letters, digits, - and _"},
		},
		{
			name: "no API key", requests: 2,
			reply: func(r permitRequest) (int, string) {
				if r.method == http.MethodGet {
					return http.StatusOK, `{"secret": null}`
				}
				return http.StatusCreated, createdEnv
			},
			want: []string{"did not reply with a secret"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply := tt.reply
			if reply == nil {
				reply = permitReplies(tt.statuses)
			}
			fake, url := startFakePermit(t, reply)

			run := runE2EEnv(t, url, nil, "create", "pinned")

			if run.exit != 2 {
				t.Errorf("exit = %d, want 2; stdout:\n%s", run.exit, run.stdout)
			}
			for _, want := range tt.want {
				if !strings.Contains(run.stdout, "::error title=e2e environment::") ||
					!strings.Contains(run.stdout, want) {
					t.Errorf("stdout has no error annotation with %q:\n%s", want, run.stdout)
				}
			}
			if got := len(fake.received()); got != tt.requests {
				t.Errorf("the fake received %d requests, want %d: %q", got, tt.requests,
					fake.received())
			}
			if run.githubEnv != "" || strings.Contains(run.stdout, "::add-mask::") {
				t.Errorf("a create that failed exported %q or masked a key:\n%s", run.githubEnv,
					run.stdout)
			}
			if strings.Contains(run.stdout, "not a command") {
				assertWrapped(t, run.stdout, "::warning::not a command")
			}
			assertNoSecret(t, run.scriptResult)
		})
	}
}

// TestE2EEnvWithoutTheSecret checks the rule for a run without PERMIT_E2E_PROJECT_KEY on every
// trigger of the e2e jobs: a pull request and a manual run of the mode e2e from a ref other than
// master pass with a notice; a push to master, a release and a manual run from master fail.
func TestE2EEnvWithoutTheSecret(t *testing.T) {
	tests := []struct {
		name, event, ref, mode string
		wantExit               int
	}{
		{name: "a pull request", event: "pull_request", ref: "refs/pull/7/merge", wantExit: 0},
		{
			name: "the pull request lane, which calls CI in the mode e2e", event: "pull_request",
			ref: "refs/pull/7/merge", mode: "e2e", wantExit: 0,
		},
		{
			name: "a manual run from another branch", event: "workflow_dispatch",
			ref: "refs/heads/feature", mode: "e2e", wantExit: 0,
		},
		{name: "a push to master", event: "push", ref: "refs/heads/master", wantExit: 2},
		{
			name: "a manual run from master", event: "workflow_dispatch", ref: "refs/heads/master",
			mode: "e2e", wantExit: 2,
		},
		{
			name: "a release", event: "workflow_dispatch", ref: "refs/heads/master",
			mode: "release", wantExit: 2,
		},
		{
			name: "a release from another branch", event: "workflow_dispatch",
			ref: "refs/heads/feature", mode: "release", wantExit: 2,
		},
		{name: "a scheduled run", event: "schedule", ref: "refs/heads/master", wantExit: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake, url := startFakePermit(t, permitReplies(nil))

			run := runE2EEnv(t, url, map[string]string{
				"PERMIT_E2E_PROJECT_KEY": "", "GITHUB_EVENT_NAME": tt.event, "GITHUB_REF": tt.ref,
				"MODE": tt.mode,
			}, "create", "pinned")

			if run.exit != tt.wantExit {
				t.Errorf("exit = %d, want %d; stdout:\n%s", run.exit, tt.wantExit, run.stdout)
			}
			want := "::error title=e2e environment::PERMIT_E2E_PROJECT_KEY is not set: a push " +
				"to master, a release and a manual run from master must run the end-to-end tests"
			wantOutput := ""
			if tt.wantExit == 0 {
				want = "::notice title=e2e::PERMIT_E2E_PROJECT_KEY is not set, so the pinned leg " +
					"does not run the end-to-end tests"
				wantOutput = "created=false\n"
			}
			if !strings.Contains(run.stdout, want) || run.githubOutput != wantOutput {
				t.Errorf("stdout does not have %q, or GITHUB_OUTPUT = %q, want %q; stdout:\n%s",
					want, run.githubOutput, wantOutput, run.stdout)
			}
			if got := fake.received(); len(got) != 0 || run.githubEnv != "" {
				t.Errorf("without the key, the script sent %q and exported %q", got, run.githubEnv)
			}
		})
	}
}

// TestE2EEnvDelete checks that delete removes the environment, that a 404 means it is gone, and
// that any other reply warns that it leaked and passes.
func TestE2EEnvDelete(t *testing.T) {
	tests := []struct {
		name     string
		statuses map[string]int
		vars     map[string]string
		want     string
		requests []string
	}{
		{
			name: "deleted", want: "e2e-env: deleted the environment gosdk-ci-1234-2-pinned\n",
			requests: []string{"DELETE " + envPath},
		},
		{
			name:     "gone already",
			statuses: map[string]int{"DELETE " + envPath: 404},
			want:     "environment gosdk-ci-1234-2-pinned is gone already, or was never created",
			requests: []string{"DELETE " + envPath},
		},
		{
			name: "leaked", statuses: map[string]int{"DELETE " + envPath: 500},
			want: "::warning title=e2e environment leaked::The scratch environment " +
				"gosdk-ci-1234-2-pinned of the project gosdk-e2e leaked: DELETE answered 500.",
			requests: []string{"DELETE " + envPath},
		},
		{
			name: "no API", vars: map[string]string{"PERMIT_E2E_API_URL": "http://127.0.0.1:9"},
			want: "leaked: DELETE answered 000",
		},
		{
			name: "no key", vars: map[string]string{"PERMIT_E2E_PROJECT_KEY": ""},
			want: "PERMIT_E2E_PROJECT_KEY is not set, so the leg created no environment",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake, url := startFakePermit(t, permitReplies(tt.statuses))

			run := runE2EEnv(t, url, tt.vars, "delete", "pinned")

			if run.exit != 0 || !strings.Contains(run.stdout, tt.want) {
				t.Errorf("exit = %d, want 0 with %q; stdout:\n%s\nstderr:\n%s", run.exit, tt.want,
					run.stdout, run.stderr)
			}
			if got := fake.received(); !slices.Equal(got, tt.requests) {
				t.Errorf("requests = %q, want %q", got, tt.requests)
			}
			assertNoSecret(t, run.scriptResult)
		})
	}
}

// TestE2EEnvDidNotRun checks the arguments and settings without which the script does not run,
// and that it sends no request then.
func TestE2EEnvDidNotRun(t *testing.T) {
	tests := []struct {
		name string
		args []string
		vars map[string]string
		want string
	}{
		{name: "no arguments", want: "usage: e2e-env.sh {create | delete} <leg>"},
		{
			name: "another command", args: []string{"list", "pinned"},
			want: "usage: e2e-env.sh {create | delete} <leg>",
		},
		{
			name: "a leg with an upper-case letter", args: []string{"create", "Pinned"},
			want: "the leg is not 1 to 16 lower-case letters and digits",
		},
		{
			name: "a project with a slash", args: []string{"create", "pinned"},
			vars: map[string]string{"PERMIT_E2E_PROJECT": "a/b"},
			want: "PERMIT_E2E_PROJECT is not the key or the ID of a project",
		},
		{
			name: "no project", args: []string{"delete", "pinned"},
			vars: map[string]string{"PERMIT_E2E_PROJECT": ""},
			want: "PERMIT_E2E_PROJECT is not the key or the ID of a project",
		},
		{
			name: "an API URL with a path", args: []string{"create", "pinned"},
			vars: map[string]string{"PERMIT_E2E_API_URL": "https://api.example.com/v2"},
			want: "PERMIT_E2E_API_URL is not an http or https URL of a host",
		},
		{
			name: "a run that is not a number", args: []string{"create", "pinned"},
			vars: map[string]string{"GITHUB_RUN_ID": "12a"},
			want: "GITHUB_RUN_ID and GITHUB_RUN_ATTEMPT must be decimal numbers",
		},
		{
			name: "no attempt", args: []string{"delete", "pinned"},
			vars: map[string]string{"GITHUB_RUN_ATTEMPT": ""},
			want: "GITHUB_RUN_ID and GITHUB_RUN_ATTEMPT must be decimal numbers",
		},
		{
			name: "no GITHUB_ENV", args: []string{"create", "pinned"},
			vars: map[string]string{"GITHUB_ENV": ""},
			want: "GITHUB_ENV is not set, so nothing can be exported",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake, url := startFakePermit(t, permitReplies(nil))

			run := runE2EEnv(t, url, tt.vars, tt.args...)

			if run.exit != 2 || !strings.Contains(run.stdout, "::error title=e2e environment::"+
				tt.want) {
				t.Errorf("exit = %d, want 2 with %q; stdout:\n%s", run.exit, tt.want, run.stdout)
			}
			if got := fake.received(); len(got) != 0 {
				t.Errorf("the script sent %q", got)
			}
		})
	}
}
