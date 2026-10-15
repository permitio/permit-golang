package scripts

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// sweptEnv is an environment of the fake project that sweep-envs.sh lists.
type sweptEnv struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	CreatedAt string `json:"created_at"`
}

// projectScope is the scope of a project-level key of the project project-id-1.
const projectScope = `{"organization_id": "org-id-1", "project_id": "project-id-1",
	"environment_id": null}`

// sweepReplies answers the requests of sweep-envs.sh: the key's scope is scope, the project
// project-id-1 holds envs, and each delete answers the status deletes gives the environment's
// ID, or 204.
func sweepReplies(t *testing.T, scope string, envs []sweptEnv,
	deletes map[string]int,
) func(r permitRequest) (int, string) {
	t.Helper()
	return func(r permitRequest) (int, string) {
		switch {
		case r.method == http.MethodGet && r.path == "/v2/api-key/scope":
			return http.StatusOK, scope
		case r.method == http.MethodGet && r.path == "/v2/projects/project-id-1/envs":
			query, err := url.ParseQuery(r.query)
			if err != nil {
				t.Errorf("the query %q: %v", r.query, err)
			}
			page, _ := strconv.Atoi(query.Get("page"))
			perPage, _ := strconv.Atoi(query.Get("per_page"))
			if page < 1 || perPage < 1 {
				t.Errorf("the list asked page %q of %q a page", query.Get("page"),
					query.Get("per_page"))
				return http.StatusUnprocessableEntity, `{"detail": "bad page"}`
			}
			first := min((page-1)*perPage, len(envs))
			listed, err := json.Marshal(envs[first:min(first+perPage, len(envs))])
			if err != nil {
				t.Error(err)
			}
			return http.StatusOK, string(listed)
		case r.method == http.MethodDelete &&
			strings.HasPrefix(r.path, "/v2/projects/project-id-1/envs/"):
			if status, ok := deletes[strings.TrimPrefix(r.path,
				"/v2/projects/project-id-1/envs/")]; ok {
				return status, `{"detail": "::warning::not a command"}`
			}
			return http.StatusNoContent, ""
		}
		t.Errorf("the sweep sent %s %s", r.method, r.path)
		return http.StatusNotFound, `{"detail": "not found"}`
	}
}

// runSweep runs sweep-envs.sh against the API at apiURL with the project's key, and vars
// replacing or adding to that.
func runSweep(t *testing.T, apiURL string, vars map[string]string) scriptResult {
	t.Helper()
	root := t.TempDir()
	copyScripts(t, root, "sweep-envs.sh")
	all := map[string]string{"PERMIT_E2E_PROJECT_KEY": fakeProjectKey, "PERMIT_E2E_API_URL": apiURL}
	maps.Copy(all, loopbackEnv)
	maps.Copy(all, vars)
	var env []string
	for name, value := range all {
		env = append(env, name+"="+value)
	}
	return runScript(t, root, env, filepath.Join(root, ".github", "scripts", "sweep-envs.sh"))
}

// deleted returns the IDs of the environments the fake was asked to delete, in order.
func deleted(f *fakePermit) []string {
	var ids []string
	for _, r := range f.all() {
		if r.method == http.MethodDelete {
			ids = append(ids, strings.TrimPrefix(r.path, "/v2/projects/project-id-1/envs/"))
		}
	}
	return ids
}

// TestSweepEnvs checks that the sweep deletes the environments whose key is a run prefix, or a
// run prefix and a suffix, and that are older than 24 hours, and keeps every other one: another
// key at any age, also one that only looks like a run prefix and a suffix, a young scratch
// environment, and one whose creation time it cannot read. It reads every page, and gives the key
// to curl in a file, not on its command line.
func TestSweepEnvs(t *testing.T) {
	now := time.Now().UTC()
	ago := func(d time.Duration, zone *time.Location, layout string) string {
		return now.Add(-d).In(zone).Format(layout)
	}
	const (
		day      = 24 * time.Hour
		withZone = "2006-01-02T15:04:05.000000-07:00"
		naive    = "2006-01-02T15:04:05.000000"
	)
	plus2 := time.FixedZone("+02:00", 2*3600)
	minus5 := time.FixedZone("-05:30", -(5*3600 + 1800))
	envs := []sweptEnv{
		{ID: "prod", Key: "production", CreatedAt: ago(400*day, time.UTC, time.RFC3339)},
		{ID: "old-pinned", Key: "gosdk-ci-11-1-pinned", CreatedAt: ago(25*time.Hour, time.UTC,
			time.RFC3339)},
		{ID: "young", Key: "gosdk-ci-12-1-latest", CreatedAt: ago(23*time.Hour, time.UTC,
			time.RFC3339)},
		// 25 hours old, with an offset east of UTC: read without the offset, it would be 23.
		{ID: "old-east", Key: "gosdk-ci-13-2-cloud", CreatedAt: ago(25*time.Hour, plus2,
			withZone)},
		// 23 hours old, with an offset west of UTC: read without the offset, it would be 28.5.
		{ID: "young-west", Key: "gosdk-ci-14-1-cloud", CreatedAt: ago(23*time.Hour, minus5,
			withZone)},
		{ID: "old-naive", Key: "gosdk-ci-15-3-pinned", CreatedAt: ago(30*time.Hour, time.UTC,
			naive)},
		// A run prefix and a suffix, as a scenario names an environment of its own.
		{
			ID: "old-suffixed", Key: "gosdk-ci-16-1-pinned-sdk-environment",
			CreatedAt: ago(2*day, time.UTC, time.RFC3339),
		},
		{
			ID: "young-suffixed", Key: "gosdk-ci-21-1-cloudpdp-sdk-environment",
			CreatedAt: ago(23*time.Hour, time.UTC, time.RFC3339),
		},
		// Keys that only look like a run prefix and a suffix.
		{ID: "trailing-hyphen", Key: "gosdk-ci-16-1-pinned-", CreatedAt: ago(2*day, time.UTC,
			time.RFC3339)},
		{ID: "upper-suffix", Key: "gosdk-ci-16-1-pinned-Sdk", CreatedAt: ago(2*day, time.UTC,
			time.RFC3339)},
		{
			ID: "long-suffix", Key: "gosdk-ci-16-1-pinned-" + strings.Repeat("a", 65),
			CreatedAt: ago(2*day, time.UTC, time.RFC3339),
		},
		{ID: "before-prefix", Key: "copy-gosdk-ci-16-1-pinned-sdk", CreatedAt: ago(2*day,
			time.UTC, time.RFC3339)},
		{ID: "upper", Key: "gosdk-ci-17-1-Pinned", CreatedAt: ago(2*day, time.UTC,
			time.RFC3339)},
		{ID: "no-attempt", Key: "gosdk-ci-18-pinned", CreatedAt: ago(2*day, time.UTC,
			time.RFC3339)},
		{ID: "unreadable", Key: "gosdk-ci-19-1-pinned", CreatedAt: "yesterday"},
		{ID: "dev", Key: "dev", CreatedAt: ago(3*day, time.UTC, time.RFC3339)},
	}
	for i := range 100 {
		envs = append(envs, sweptEnv{
			ID: fmt.Sprintf("staging-%d", i), Key: fmt.Sprintf("staging-%d", i),
			CreatedAt: ago(10*day, time.UTC, time.RFC3339),
		})
	}
	// The last page holds a scratch environment too, so a sweep that reads one page misses it.
	envs = append(envs, sweptEnv{
		ID: "old-last-page", Key: "gosdk-ci-20-1-latest",
		CreatedAt: ago(26*time.Hour, time.UTC, time.RFC3339),
	})
	fake, url := startFakePermit(t, sweepReplies(t, projectScope, envs, nil))
	curlVars, curlLog := logCurlArgs(t)

	result := runSweep(t, url, curlVars)

	if result.exit != 0 {
		t.Fatalf("exit = %d, want 0; stdout:\n%s\nstderr:\n%s", result.exit, result.stdout,
			result.stderr)
	}
	want := []string{"old-pinned", "old-east", "old-naive", "old-suffixed", "old-last-page"}
	if got := deleted(fake); !slices.Equal(got, want) {
		t.Errorf("deleted %q, want %q", got, want)
	}
	var pages []string
	for _, r := range fake.all() {
		if r.method == http.MethodGet && strings.HasSuffix(r.path, "/envs") {
			pages = append(pages, r.query)
		}
		if r.auth != "Bearer "+fakeProjectKey {
			t.Errorf("%s %s has the Authorization %q", r.method, r.path, r.auth)
		}
	}
	if wantPages := []string{"page=1&per_page=100", "page=2&per_page=100"}; !slices.Equal(pages,
		wantPages) {
		t.Errorf("listed %q, want %q", pages, wantPages)
	}
	for _, want := range []string{
		"sweep-envs: deleted gosdk-ci-11-1-pinned, 25 hours old\n",
		"sweep-envs: keeping gosdk-ci-12-1-latest: it is 23 hours old\n",
		"sweep-envs: deleted gosdk-ci-16-1-pinned-sdk-environment, 48 hours old\n",
		"sweep-envs: keeping gosdk-ci-21-1-cloudpdp-sdk-environment: it is 23 hours old\n",
		"sweep-envs: keeping gosdk-ci-19-1-pinned: its creation time cannot be read\n",
		"sweep-envs: the project holds 117 environments, 9 of them scratch environments; " +
			"deleted 5 older than 24 hours, and 0 could not be deleted\n",
	} {
		if !strings.Contains(result.stdout, want) {
			t.Errorf("stdout does not contain %q:\n%s", want, result.stdout)
		}
	}
	assertWrapped(t, result.stdout, "the project holds 117 environments")
	assertNoSecret(t, result)
	assertKeyNotInArgs(t, curlLog)
}

// TestSweepEnvsDeleteFails checks that a delete that fails is warned about and stops neither the
// other deletes nor the job, as the sweep of a later run tries again, and that a 404 means the
// environment is gone.
func TestSweepEnvsDeleteFails(t *testing.T) {
	old := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	envs := []sweptEnv{
		{ID: "a", Key: "gosdk-ci-1-1-pinned", CreatedAt: old},
		{ID: "b", Key: "gosdk-ci-2-1-pinned", CreatedAt: old},
		{ID: "c", Key: "gosdk-ci-3-1-pinned", CreatedAt: old},
	}
	fake, url := startFakePermit(t, sweepReplies(t, projectScope, envs,
		map[string]int{"a": 500, "b": 404}))

	result := runSweep(t, url, nil)

	if result.exit != 0 {
		t.Errorf("exit = %d, want 0; stdout:\n%s", result.exit, result.stdout)
	}
	if got := deleted(fake); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("deleted %q, want a, b and c", got)
	}
	for _, want := range []string{
		"sweep-envs: deleting gosdk-ci-1-1-pinned answered 500:",
		"deleted 2 older than 24 hours, and 1 could not be deleted",
		"::warning title=Sweep::1 scratch environments older than 24 hours could not be " +
			"deleted; the log of this step lists them, and the sweep of a later run tries again\n",
	} {
		if !strings.Contains(result.stdout, want) {
			t.Errorf("stdout does not contain %q:\n%s", want, result.stdout)
		}
	}
	assertWrapped(t, result.stdout, "::warning::not a command")
}

// scopeThen answers the read of the key's scope with a project-level scope, and every other
// request with status and body.
func scopeThen(status int, body string) func(r permitRequest) (int, string) {
	return func(r permitRequest) (int, string) {
		if r.path == "/v2/api-key/scope" {
			return http.StatusOK, projectScope
		}
		return status, body
	}
}

// TestSweepEnvsGivesUp checks the replies on which the sweep stops with a warning and passes,
// as a later run sweeps again: a scope or a list that cannot be read for a reason other than
// the key. It deletes nothing then.
func TestSweepEnvsGivesUp(t *testing.T) {
	old := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	envs := []sweptEnv{{ID: "a", Key: "gosdk-ci-1-1-pinned", CreatedAt: old}}
	tests := []struct {
		name  string
		reply func(r permitRequest) (int, string)
		vars  map[string]string
		want  string
	}{
		{
			name: "a 500 to the scope",
			reply: func(r permitRequest) (int, string) {
				return http.StatusInternalServerError, `{"detail": "::warning::not a command"}`
			},
			want: "GET /v2/api-key/scope answered 500, so the environments were not swept",
		},
		{
			name: "no reply", reply: sweepReplies(t, projectScope, envs, nil),
			vars: map[string]string{"PERMIT_E2E_API_URL": "http://127.0.0.1:9"},
			want: "GET /v2/api-key/scope answered 000, so the environments were not swept",
		},
		{
			name: "a scope that is not JSON", reply: sweepReplies(t, `not json`, envs, nil),
			want: "GET /v2/api-key/scope did not reply with a scope, so the environments were " +
				"not swept",
		},
		{
			name: "a scope that is a list", reply: sweepReplies(t, `[]`, envs, nil),
			want: "GET /v2/api-key/scope did not reply with a scope, so the environments were " +
				"not swept",
		},
		{
			name:  "a 503 to the list",
			reply: scopeThen(http.StatusServiceUnavailable, `{"detail": "busy"}`),
			want:  "GET /v2/projects/project-id-1/envs answered 503, so the environments were not",
		},
		{
			name:  "a list that is not a list",
			reply: scopeThen(http.StatusOK, `{"items": []}`),
			want: "GET /v2/projects/project-id-1/envs did not reply with a list of environments, " +
				"so the environments were not swept",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake, url := startFakePermit(t, tt.reply)

			result := runSweep(t, url, tt.vars)

			want := "::warning title=Sweep::" + tt.want
			if result.exit != 0 || !strings.Contains(result.stdout, want) ||
				!strings.Contains(result.stdout, "; a later run sweeps them\n") {
				t.Errorf("exit = %d, want 0 with %q and a later run; stdout:\n%s", result.exit,
					want, result.stdout)
			}
			if strings.Contains(result.stdout, "::error") {
				t.Errorf("stdout has an error:\n%s", result.stdout)
			}
			if got := deleted(fake); len(got) != 0 {
				t.Errorf("deleted %q", got)
			}
			if strings.Contains(result.stdout, "not a command") {
				assertWrapped(t, result.stdout, "::warning::not a command")
			}
		})
	}
}

// TestSweepEnvsDidNotRun checks the configurations with which the sweep does not run, and that
// it then deletes nothing: a key whose scope is not one project, a key the API refuses, and a
// setting or an argument that is wrong.
func TestSweepEnvsDidNotRun(t *testing.T) {
	old := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	envs := []sweptEnv{{ID: "a", Key: "gosdk-ci-1-1-pinned", CreatedAt: old}}
	notProject := "PERMIT_E2E_PROJECT_KEY is not a project-level API key"
	refused := func(request string, status int) string {
		return request + " answered " + strconv.Itoa(status) + ": the API refused " +
			"PERMIT_E2E_PROJECT_KEY. The sweep needs a project-level API key, which can list " +
			"and delete the environments of its project"
	}
	tests := []struct {
		name  string
		reply func(r permitRequest) (int, string)
		vars  map[string]string
		want  string
	}{
		{
			name: "an environment-level key",
			reply: sweepReplies(t, `{"organization_id": "o", "project_id": "project-id-1",
				"environment_id": "env-id-1"}`, envs, nil),
			want: notProject,
		},
		{
			name: "an organization-level key",
			reply: sweepReplies(t, `{"organization_id": "o", "project_id": null,
				"environment_id": null}`, envs, nil),
			want: notProject,
		},
		{
			name: "a 401 to the scope",
			reply: func(r permitRequest) (int, string) {
				return http.StatusUnauthorized, `{"detail": "unauthorized"}`
			},
			want: refused("GET /v2/api-key/scope", http.StatusUnauthorized),
		},
		{
			name: "a 403 to the scope",
			reply: func(r permitRequest) (int, string) {
				return http.StatusForbidden, `{"detail": "forbidden"}`
			},
			want: refused("GET /v2/api-key/scope", http.StatusForbidden),
		},
		{
			name:  "a 403 to the list",
			reply: scopeThen(http.StatusForbidden, `{"detail": "forbidden"}`),
			want:  refused("GET /v2/projects/project-id-1/envs", http.StatusForbidden),
		},
		{
			name:  "an API URL with a path",
			reply: sweepReplies(t, projectScope, envs, nil),
			vars:  map[string]string{"PERMIT_E2E_API_URL": "https://api.example.com/v2"},
			want:  "PERMIT_E2E_API_URL is not an http or https URL of a host",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake, url := startFakePermit(t, tt.reply)

			result := runSweep(t, url, tt.vars)

			if result.exit != 2 || !strings.Contains(result.stdout, "::error title=Sweep::"+
				tt.want) {
				t.Errorf("exit = %d, want 2 with %q; stdout:\n%s", result.exit, tt.want,
					result.stdout)
			}
			if got := deleted(fake); len(got) != 0 {
				t.Errorf("deleted %q", got)
			}
		})
	}

	t.Run("an argument", func(t *testing.T) {
		root := t.TempDir()
		copyScripts(t, root, "sweep-envs.sh")

		result := runScript(t, root, nil, filepath.Join(root, ".github", "scripts",
			"sweep-envs.sh"), "x")

		if result.exit != 2 || !strings.Contains(result.stdout, "usage: sweep-envs.sh") {
			t.Errorf("exit = %d, want 2 with the usage; stdout:\n%s", result.exit, result.stdout)
		}
	})
}

// TestSweepEnvsWithoutTheKey checks that without PERMIT_E2E_PROJECT_KEY there is nothing to
// sweep, and the sweep sends no request.
func TestSweepEnvsWithoutTheKey(t *testing.T) {
	fake, url := startFakePermit(t, sweepReplies(t, projectScope, nil, nil))

	result := runSweep(t, url, map[string]string{"PERMIT_E2E_PROJECT_KEY": ""})

	if result.exit != 0 || !strings.Contains(result.stdout,
		"PERMIT_E2E_PROJECT_KEY is not set, so there is nothing to sweep") {
		t.Errorf("exit = %d, want 0 with nothing to sweep; stdout:\n%s", result.exit,
			result.stdout)
	}
	if got := fake.received(); len(got) != 0 {
		t.Errorf("the sweep sent %q", got)
	}
}
