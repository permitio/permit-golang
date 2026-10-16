package main

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The commits of the fixtures.
const (
	releaseSHA = "1111111111111111111111111111111111111111"
	otherSHA   = "2222222222222222222222222222222222222222"
	reportSum  = "3333333333333333333333333333333333333333333333333333333333333333"
)

// verdicts is the last line releasecheck prints for each exit code.
var verdicts = map[int]string{
	exitPassed:    "releasecheck: PASS\n",
	exitFailed:    "releasecheck: FAIL\n",
	exitDidNotRun: "releasecheck: DID NOT RUN\n",
}

// repoFiles are the files of a repository that is ready to release v2.1.0.
func repoFiles() map[string]string {
	return map[string]string{
		versionFile: "package permit\n\n// Version is the version.\nconst Version = \"2.1.0\"\n",
		changelogFile: "# Changelog\n\n## v2.1.0 (2026-11-02)\n\n### Client\n\n- A change.\n\n" +
			"## v2.0.0 (2026-10-30)\n\n- The first release.\n",
		ciFile: "name: CI\nenv:\n  GO_FLOOR: \"1.26.0\"\n  GO_126: \"1.26.9\"\n" +
			"  GO_127: \"1.27.2\"\n  EXPECTED_HOOKS: \"28\"\njobs: {}\n",
		pendingFile: "# Pending methods.\n\n",
		localOnlyFile: "# Local-only methods.\n\nProjectsService.List org-key api.projects\n" +
			"APIKeysService.Get org-key api.api_keys\n" +
			"ProjectsService.Create org-key api.projects\n",
		requiredFile: "# Required.\n\nexample.com/e2e TestCloud\nexample.com/e2e TestCloud/a\n" +
			"example.com/e2e TestCloud/b\nexample.com/e2e TestPDP/c\n",
	}
}

// runOf is a run of ci.yml in its attempt, as the GitHub API lists it.
func runOf(id, attempt int, sha, event, status, conclusion string) string {
	return fmt.Sprintf(`{"id": %d, "run_attempt": %d, "head_sha": %q, "event": %q, `+
		`"status": %q, "conclusion": %q}`, id, attempt, sha, event, status, conclusion)
}

// runsOf is the list of the runs of ci.yml, as the GitHub API returns it.
func runsOf(runs ...string) string {
	return fmt.Sprintf(`{"total_count": %d, "workflow_runs": [%s]}`, len(runs),
		strings.Join(runs, ",\n"))
}

// ciRuns are runs of ci.yml for a push of the release commit, the newest of which succeeded.
var ciRuns = runsOf(
	runOf(102, 2, releaseSHA, "push", "completed", "success"),
	runOf(101, 1, releaseSHA, "push", "completed", "failure"),
)

// validEvidence is harness evidence of the release commit that covers every lane and phase.
const validEvidence = `{"sdk_commit": "` + releaseSHA + `", "report_sha256": "` + reportSum +
	`", "lanes": ["go1.26.0", "go1.26.9", "go1.27.2"], ` +
	`"passed_phases": ["api.projects", "api.api_keys", "api.users"]}`

// writeTree writes files, a map from a slash path relative to dir to the content, under dir.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		file := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// runCheck runs releasecheck and returns its exit code and what it printed on stdout.
func runCheck(args ...string) (int, string) {
	var stdout, stderr bytes.Buffer
	exit := run(args, &stdout, &stderr)
	return exit, stdout.String()
}

// assertResult checks the exit code, that the output holds every part of want, and that it ends
// with the verdict of the exit code.
func assertResult(t *testing.T, exit, wantExit int, output string, want ...string) {
	t.Helper()
	if exit != wantExit {
		t.Errorf("exit = %d, want %d; output:\n%s", exit, wantExit, output)
	}
	for _, part := range want {
		if !strings.Contains(output, part) {
			t.Errorf("output does not hold %q:\n%s", part, output)
		}
	}
	if !strings.HasSuffix(output, verdicts[wantExit]) {
		t.Errorf("output does not end with %q:\n%s", verdicts[wantExit], output)
	}
}

// validateCase is a run of validate: changes to the repository and the inputs of the release
// workflow.
type validateCase struct {
	name string
	// files replace files of repoFiles; an empty content removes the file.
	files                              map[string]string
	tag, sha, master, ciRuns, evidence string
	wantExit                           int
	want                               []string
}

// runValidateCase writes the repository and the inputs of tc, with those of a release of v2.1.0
// where tc sets none, and runs validate.
func runValidateCase(t *testing.T, tc validateCase) (int, string) {
	t.Helper()
	dir := t.TempDir()
	files := repoFiles()
	for name, content := range tc.files {
		files[name] = content
		if content == "" {
			delete(files, name)
		}
	}
	files["ci-runs.json"] = orDefault(tc.ciRuns, ciRuns)
	files["evidence.json"] = orDefault(tc.evidence, validEvidence)
	if tc.evidence == "-" {
		files["evidence.json"] = ""
	}
	writeTree(t, dir, files)
	return runCheck("validate", "-root", dir, "-tag", orDefault(tc.tag, "v2.1.0"),
		"-sha", orDefault(tc.sha, releaseSHA), "-master", orDefault(tc.master, releaseSHA),
		"-ci-runs", filepath.Join(dir, "ci-runs.json"),
		"-evidence", filepath.Join(dir, "evidence.json"))
}

// orDefault returns value, or def when value is empty.
func orDefault(value, def string) string {
	if value == "" {
		return def
	}
	return value
}

func TestValidate(t *testing.T) {
	unreleased := "# Changelog\n\n## v2.1.0 (unreleased)\n\n- A change.\n"
	tests := []validateCase{
		{
			name: "a release", wantExit: exitPassed,
			want: []string{
				"releasecheck: ok: the tag v2.1.0 is a v2 release tag of the Version of " +
					"version.go, 2.1.0\n",
				"releasecheck: ok: " + releaseSHA + " is the head of master\n",
				"releasecheck: ok: CI succeeded for " + releaseSHA + " on its push to master, " +
					"in run 102, attempt 2\n",
				"releasecheck: ok: CHANGELOG.md has the section \"## v2.1.0 (2026-11-02)\"\n",
				"releasecheck: ok: tools/e2e/pending.txt has no method left\n",
				"releasecheck: harness evidence: commit " + releaseSHA + ", report sha256 " +
					reportSum + ", lanes go1.26.0 go1.26.9 go1.27.2, passed phases " +
					"api.projects api.api_keys api.users; the reviewer of the release " +
					"environment confirms the sha256 against the report\n",
				"releasecheck: ok: the harness evidence is of " + releaseSHA + ", on the lanes " +
					"go1.26.0, go1.26.9, go1.27.2\n",
			},
		},
		{
			name: "a release candidate of an unreleased version", tag: "v2.1.0-rc.12",
			files: map[string]string{changelogFile: unreleased}, wantExit: exitPassed,
			want: []string{"has the section \"## v2.1.0 (unreleased)\"\n"},
		},
		{
			name: "lists with indented comments", wantExit: exitPassed,
			files: map[string]string{
				pendingFile: "# Pending methods.\n  # Client.Check e2e-pending pdp\n\t#\n",
				localOnlyFile: repoFiles()[localOnlyFile] +
					"  # UsersService.Get org-key api.users\n",
			},
			want: []string{
				"releasecheck: ok: tools/e2e/pending.txt has no method left\n",
				"releasecheck: ok: the harness evidence is of ",
			},
		},
		{
			name: "a tag without a patch", tag: "v2.0", wantExit: exitFailed,
			want: []string{
				"releasecheck: FAIL: the tag \"v2.0\" is not a v2 release tag as a whole " +
					"string, v2.<minor>.<patch> or v2.<minor>.<patch>-rc.<n>, such as v2.1.0 or " +
					"v2.1.0-rc.1\n",
			},
		},
		{
			name: "a tag with a suffix", tag: "v2.1.0-beta", wantExit: exitFailed,
			want: []string{"the tag \"v2.1.0-beta\" is not a v2 release tag"},
		},
		{
			name: "a tag with a leading zero", tag: "v2.01.0", wantExit: exitFailed,
			want: []string{"the tag \"v2.01.0\" is not a v2 release tag"},
		},
		{
			name: "release candidate 0", tag: "v2.1.0-rc.0", wantExit: exitFailed,
			want: []string{"the tag \"v2.1.0-rc.0\" is not a v2 release tag"},
		},
		{
			name: "a tag inside other text", tag: "xv2.1.0 ", wantExit: exitFailed,
			want: []string{"the tag \"xv2.1.0 \" is not a v2 release tag"},
		},
		{
			name: "a tag of another major version", tag: "v3.1.0", wantExit: exitFailed,
			want: []string{"the tag \"v3.1.0\" is not a v2 release tag"},
		},
		{
			name: "a tag that is not the committed version", tag: "v2.2.0", wantExit: exitFailed,
			want: []string{
				"releasecheck: FAIL: the tag v2.2.0 is not v2.1.0, the Version of version.go, " +
					"or a release candidate of it, v2.1.0-rc.<n>: change Version in a pull " +
					"request, or release that version\n",
			},
		},
		{
			name: "a release candidate of another version", tag: "v2.2.0-rc.1",
			wantExit: exitFailed, want: []string{"the tag v2.2.0-rc.1 is not v2.1.0"},
		},
		{
			name: "a tag without a patch and an unreleased section", tag: "v2.0",
			files: map[string]string{changelogFile: unreleased}, wantExit: exitFailed,
			want: []string{"the tag \"v2.0\" is not a v2 release tag"},
		},
		{
			name: "a tag of another version and an unreleased section", tag: "v2.2.0",
			files: map[string]string{changelogFile: unreleased}, wantExit: exitFailed,
			want: []string{"the tag v2.2.0 is not v2.1.0"},
		},
		{
			name: "a commit that is not the head of master", master: otherSHA,
			wantExit: exitFailed,
			want: []string{
				"releasecheck: FAIL: the workflow runs on " + releaseSHA + ", which is not the " +
					"head of master, " + otherSHA + ": dispatch the release on master",
			},
		},
		{
			name: "CI that failed", wantExit: exitFailed,
			ciRuns: runsOf(runOf(101, 1, releaseSHA, "push", "completed", "failure")),
			want: []string{
				"releasecheck: FAIL: CI did not succeed for " + releaseSHA + ": the latest run " +
					"of ci.yml for a push of it, run 101, attempt 1, is completed with the " +
					"conclusion \"failure\", not completed with success; wait for it, or fix it\n",
			},
		},
		{
			name: "a scheduled CI that succeeded, and a push CI that failed", wantExit: exitFailed,
			ciRuns: runsOf(runOf(103, 1, releaseSHA, "schedule", "completed", "success"),
				runOf(101, 1, releaseSHA, "push", "completed", "failure")),
			want: []string{"the latest run of ci.yml for a push of it, run 101, attempt 1, is " +
				"completed with the conclusion \"failure\""},
		},
		{
			name: "only a scheduled and a manual CI", wantExit: exitFailed,
			ciRuns: runsOf(runOf(103, 1, releaseSHA, "schedule", "completed", "success"),
				runOf(104, 1, releaseSHA, "workflow_dispatch", "completed", "success")),
			want: []string{
				"releasecheck: FAIL: CI did not succeed for " + releaseSHA + ": ci.yml has no " +
					"run for a push of it (its other runs: schedule run 103 for " + releaseSHA +
					", completed success; workflow_dispatch run 104 for " + releaseSHA +
					", completed success); wait for CI of the push to master\n",
			},
		},
		{
			name: "a re-run that failed after an attempt that succeeded", wantExit: exitFailed,
			ciRuns: runsOf(runOf(101, 1, releaseSHA, "push", "completed", "success"),
				runOf(101, 2, releaseSHA, "push", "completed", "failure")),
			want: []string{"the latest run of ci.yml for a push of it, run 101, attempt 2, is " +
				"completed with the conclusion \"failure\""},
		},
		{
			name: "an older run that succeeded, and a newer one that failed", wantExit: exitFailed,
			ciRuns: runsOf(runOf(101, 3, releaseSHA, "push", "completed", "success"),
				runOf(105, 1, releaseSHA, "push", "completed", "failure")),
			want: []string{"the latest run of ci.yml for a push of it, run 105, attempt 1, is " +
				"completed with the conclusion \"failure\""},
		},
		{
			name: "CI of another commit", wantExit: exitFailed,
			ciRuns: runsOf(runOf(101, 1, otherSHA, "push", "completed", "success")),
			want: []string{"ci.yml has no run for a push of it (its other runs: push run 101 for " +
				otherSHA + ", completed success);"},
		},
		{
			name: "CI that is still running", wantExit: exitFailed,
			ciRuns: runsOf(runOf(101, 1, releaseSHA, "push", "in_progress", "")),
			want: []string{"run 101, attempt 1, is in_progress with the conclusion \"\", not " +
				"completed with success;"},
		},
		{
			name: "no CI", ciRuns: runsOf(), wantExit: exitFailed,
			want: []string{"ci.yml has no run for a push of it (its other runs: none);"},
		},
		{
			name: "no section in the changelog", wantExit: exitFailed,
			files: map[string]string{changelogFile: "# Changelog\n\n## v2.0.0 (2026-10-30)\n"},
			want: []string{
				"releasecheck: FAIL: CHANGELOG.md has no section for the release, a heading " +
					"\"## v2.1.0\"; write its entry\n",
			},
		},
		{
			name: "a release whose section is unreleased", wantExit: exitFailed,
			files: map[string]string{changelogFile: unreleased},
			want: []string{
				"releasecheck: FAIL: the heading \"## v2.1.0 (unreleased)\" of CHANGELOG.md " +
					"still says (unreleased); write the day of the release in its place",
			},
		},
		{
			name: "a pending method", wantExit: exitFailed,
			files: map[string]string{
				pendingFile: "# Pending.\nUsersService.Get e2e-pending users\nClient.Check " +
					"e2e-pending pdp\n",
			},
			want: []string{
				"releasecheck: FAIL: tools/e2e/pending.txt still lists 2 methods, such as " +
					"UsersService.Get: a release needs an end-to-end scenario, or a local-only " +
					"or advisory line, for each\n",
			},
		},
		{
			name: "one pending method", wantExit: exitFailed,
			files: map[string]string{pendingFile: "Client.Check e2e-pending pdp\n"},
			want:  []string{"tools/e2e/pending.txt still lists 1 method, such as Client.Check:"},
		},
		{
			name: "harness evidence of another commit", wantExit: exitFailed,
			evidence: strings.Replace(validEvidence, releaseSHA, otherSHA, 1),
			want: []string{
				"releasecheck: FAIL: the harness evidence is of the commit " + otherSHA +
					", not of the release commit " + releaseSHA + ": run the local full-stack " +
					"suite on the release commit\n",
			},
		},
		{
			name: "harness evidence without a lane", wantExit: exitFailed,
			evidence: strings.Replace(validEvidence, `"go1.26.9", `, "", 1),
			want: []string{
				"releasecheck: FAIL: the harness evidence has no lane go1.26.9: the local " +
					"full-stack suite runs on the Go releases of CI's test legs, go1.26.0, " +
					"go1.26.9, go1.27.2\n",
			},
		},
		{
			name: "a local-only phase that did not pass", wantExit: exitFailed,
			evidence: strings.Replace(validEvidence, `"api.projects", `, "", 1),
			want: []string{
				"releasecheck: FAIL: the phase api.projects, which proves ProjectsService.List, " +
					"ProjectsService.Create of tools/e2e/local-only.txt, is not in the harness " +
					"evidence's passed_phases\n",
			},
		},
		{
			name: "a release that breaks every rule", tag: "v2.1.0-rc.1", master: otherSHA,
			ciRuns: runsOf(), wantExit: exitFailed,
			files: map[string]string{changelogFile: "# Changelog\n", pendingFile: "A.B x y\n"},
			evidence: `{"sdk_commit": "` + otherSHA + `", "report_sha256": "` + reportSum +
				`", "lanes": [], "passed_phases": []}`,
			want: []string{"releasecheck: FAIL: the phase api.api_keys, which proves"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exit, stdout := runValidateCase(t, tt)

			assertResult(t, exit, tt.wantExit, stdout, tt.want...)
			if tt.wantExit == exitFailed && strings.Count(stdout, "releasecheck: FAIL: ") !=
				max(len(tt.want), failuresOf(tt.name)) {
				t.Errorf("output has %d failures, want %d:\n%s",
					strings.Count(stdout, "releasecheck: FAIL: "),
					max(len(tt.want), failuresOf(tt.name)), stdout)
			}
		})
	}
}

// failuresOf returns the number of failures of the case that breaks every rule: the tag's
// version and changelog section aside, each check fails, the evidence's three included.
func failuresOf(name string) int {
	if name == "a release that breaks every rule" {
		return 8
	}
	return 1
}

func TestValidateDidNotRun(t *testing.T) {
	tests := []validateCase{
		{
			name: "empty harness evidence", evidence: "-",
			want: []string{"releasecheck: DID NOT RUN: the harness evidence is empty: pass the " +
				"summary of the local suite report as harness_evidence, a JSON object with " +
				"sdk_commit, report_sha256, lanes and passed_phases\n"},
		},
		{
			name: "blank harness evidence", evidence: " \n\t",
			want: []string{"the harness evidence is empty"},
		},
		{
			name: "harness evidence that is not JSON", evidence: "passed",
			want: []string{"the harness evidence is not a JSON object with sdk_commit, " +
				"report_sha256, lanes and passed_phases: "},
		},
		{
			name:     "harness evidence with a field of its own",
			evidence: strings.Replace(validEvidence, `"lanes"`, `"lane": 1, "lanes"`, 1),
			want:     []string{`json: unknown field "lane"`},
		},
		{
			name: "harness evidence of two values", evidence: validEvidence + "\n{}",
			want: []string{"the harness evidence holds more than one JSON value\n"},
		},
		{
			name:     "harness evidence with a short commit",
			evidence: strings.Replace(validEvidence, releaseSHA, "1111111", 1),
			want: []string{"the harness evidence's sdk_commit \"1111111\" is not a full " +
				"commit hash\n"},
		},
		{
			name:     "harness evidence with a hash that is not SHA-256",
			evidence: strings.Replace(validEvidence, reportSum, "md5:abc", 1),
			want: []string{"the harness evidence's report_sha256 \"md5:abc\" is not a " +
				"SHA-256 hash in hex\n"},
		},
		{
			name: "a commit that is not a hash", sha: "HEAD",
			want: []string{"releasecheck: DID NOT RUN: -sha \"HEAD\" is not a full commit hash\n"},
		},
		{
			name: "CI runs that are not a list", ciRuns: `{"total_count": 0}`,
			want: []string{"ci-runs.json has no workflow_runs: it is not a list of workflow " +
				"runs\n"},
		},
		{
			name: "CI runs that are not JSON", ciRuns: "<html>",
			want: []string{"ci-runs.json: invalid character"},
		},
		{
			name: "a CI run without an id", ciRuns: runsOf(runOf(0, 1, releaseSHA, "push",
				"completed", "success")),
			want: []string{"ci-runs.json lists a run without an id or a run_attempt: it is not " +
				"a list of workflow runs\n"},
		},
		{
			name: "a CI run without an attempt", ciRuns: runsOf(runOf(101, 0, releaseSHA, "push",
				"completed", "success")),
			want: []string{"ci-runs.json lists a run without an id or a run_attempt"},
		},
		{
			name: "no version.go", files: map[string]string{versionFile: ""},
			want: []string{"version.go: no such file or directory"},
		},
		{
			name: "a version.go without Version",
			files: map[string]string{versionFile: "package permit\n\nconst Name = \"x\"\n" +
				"const Version, Other = 2, \"y\"\n"},
			want: []string{"version.go declares no constant Version with a string value\n"},
		},
		{
			name: "a Version without a value of its own",
			files: map[string]string{versionFile: "package permit\n\nconst (\n" +
				"\tName = \"x\"\n\tVersion\n)\n"},
			want: []string{"version.go declares no constant Version with a string value\n"},
		},
		{
			name:  "a version.go that does not parse",
			files: map[string]string{versionFile: "package permit\nconst (\n"},
			want:  []string{"releasecheck: DID NOT RUN: "},
		},
		{
			name:  "a ci.yml without test legs",
			files: map[string]string{ciFile: "env:\n  EXPECTED_HOOKS: \"28\"\n"},
			want: []string{"its env has no GO_FLOOR or GO_<n> variable, the Go releases of " +
				"the test legs\n"},
		},
		{
			name:  "a test leg that is not an exact release",
			files: map[string]string{ciFile: "env:\n  GO_FLOOR: \"1.26\"\n"},
			want:  []string{"GO_FLOOR is \"1.26\", not an exact Go release such as 1.27.1\n"},
		},
		{
			name: "a ci.yml that is not YAML", files: map[string]string{ciFile: "env: [\n"},
			want: []string{".github/workflows/ci.yml: yaml: "},
		},
		{
			name:  "a local-only line without a phase",
			files: map[string]string{localOnlyFile: "ProjectsService.List org-key\n"},
			want: []string{"\"ProjectsService.List org-key\" is not <Method> <reason> " +
				"<phase>\n"},
		},
		{
			name: "no pending file", files: map[string]string{pendingFile: ""},
			want: []string{"pending.txt: no such file or directory"},
		},
		{
			name: "no changelog", files: map[string]string{changelogFile: ""},
			want: []string{"CHANGELOG.md: no such file or directory"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exit, stdout := runValidateCase(t, tt)

			assertResult(t, exit, exitDidNotRun, stdout, tt.want...)
			if strings.Contains(stdout, "ok: ") {
				t.Errorf("a run that did not run reports checks:\n%s", stdout)
			}
		})
	}
}

func TestRunDidNotRun(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no subcommand", nil, "releasecheck: DID NOT RUN: usage: releasecheck " +
			"validate|e2e|notes|watchdog|lanes [flags];"},
		{"an unknown subcommand", []string{"publish"}, "unknown subcommand \"publish\"; usage:"},
		{"validate without flags", []string{"validate"}, "releasecheck: DID NOT RUN: " +
			"releasecheck validate needs -ci-runs, -evidence, -master, -sha, -tag\n"},
		{
			"e2e without -record",
			[]string{"e2e", "-jobs", "j", "-ci-job", "c", "-e2e-job", "e"},
			"releasecheck e2e needs -record\n",
		},
		{"notes without -tag", []string{"notes"}, "releasecheck notes needs -tag\n"},
		{
			"an argument after the flags",
			[]string{"notes", "-tag", "v2.1.0", "extra"},
			"unexpected arguments [\"extra\"]\n",
		},
		{"an unknown flag", []string{"e2e", "-unknown"}, "flag provided but not defined: " +
			"-unknown\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exit, stdout := runCheck(tt.args...)

			assertResult(t, exit, exitDidNotRun, stdout, tt.want)
		})
	}
}

// lanes prints the Go release of each test leg of ci.yml, the lanes that validate requires in
// the harness evidence, one per line and in order.
func TestLanes(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{ciFile: "name: CI\nenv:\n  GO_127: \"1.27.2\"\n" +
		"  GO_FLOOR: \"1.26.0\"\n  GO_126: \"1.26.9\"\n  EXPECTED_HOOKS: \"28\"\n" +
		"jobs:\n  test:\n    env:\n      GO_128: \"1.28.0\"\n"})

	exit, stdout := runCheck("lanes", "-root", dir)

	if want := "go1.26.0\ngo1.26.9\ngo1.27.2\n"; exit != exitPassed || stdout != want {
		t.Errorf("lanes exited %d and printed\n%s\nwant 0 and\n%s", exit, stdout, want)
	}
}

func TestLanesDidNotRun(t *testing.T) {
	tests := []struct {
		name string
		// ci is the content of ci.yml, which is missing when it is empty.
		ci   string
		args []string
		want string
	}{
		{name: "no ci.yml", want: "ci.yml: no such file or directory\n"},
		{
			name: "no test leg", ci: "name: CI\nenv:\n  EXPECTED_HOOKS: \"28\"\n",
			want: "ci.yml: its env has no GO_FLOOR or GO_<n> variable, the Go releases of the " +
				"test legs\n",
		},
		{
			name: "a test leg that is no exact release", ci: "env:\n  GO_FLOOR: \"1.26\"\n",
			want: "ci.yml: GO_FLOOR is \"1.26\", not an exact Go release such as 1.27.1\n",
		},
		{
			name: "an argument", ci: repoFiles()[ciFile], args: []string{"ci.yml"},
			want: "unexpected arguments [\"ci.yml\"]\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.ci != "" {
				writeTree(t, dir, map[string]string{ciFile: tt.ci})
			}

			exit, stdout := runCheck(append([]string{"lanes", "-root", dir}, tt.args...)...)

			assertResult(t, exit, exitDidNotRun, stdout, tt.want)
			if strings.Contains(stdout, "go1.") {
				t.Errorf("lanes that did not run printed a lane:\n%s", stdout)
			}
		})
	}
}

// requestLine is a line of a request record of mode.
func requestLine(mode string) string {
	return `{"type":"request","mode":"` + mode + `","target":"api","method":"GET",` +
		`"template":"/v2/projects","path":"/v2/projects","status":200,"test":"TestCloud/a"}`
}

// recordOf returns a request record with the requests, and a session trailer with the exit
// status when it is not empty.
func recordOf(exitStatus string, requests ...string) string {
	lines := append([]string{`{"type":"header","format":1}`}, requests...)
	if exitStatus != "" {
		lines = append(lines, `{"type":"session","exit_status":`+exitStatus+`,"tests":3}`)
	}
	return strings.Join(lines, "\n") + "\n"
}

// jobOf is a completed job of a run, as the GitHub API lists it, with its conclusion.
func jobOf(name, conclusion string) string {
	return `{"name": "` + name + `", "status": "completed", "conclusion": "` + conclusion + `"}`
}

// jobsPages returns the jobs of a run in two pages, in which the e2e job has the conclusion e2e
// and every other job succeeded.
func jobsPages(e2e string) string {
	return `{"total_count": 3, "jobs": [` + jobOf("validate", "success") + ", " +
		jobOf("gates / e2e (pinned PDP)", e2e) + "]}\n" +
		`{"total_count": 3, "jobs": [` + jobOf("gates / CI", "success") + "]}\n"
}

func TestE2E(t *testing.T) {
	live := requestLine("live")
	three := recordOf("0", live, live, requestLine("dryrun"), live)
	tests := []struct {
		name string
		// files replace files of the repository and the inputs: jobs.json and record.jsonl.
		files    map[string]string
		wantExit int
		want     []string
	}{
		{
			name: "a run whose end-to-end job proved the release", wantExit: exitPassed,
			want: []string{
				"releasecheck: ok: the job \"gates / CI\" succeeded\n",
				"releasecheck: ok: the job \"gates / e2e (pinned PDP)\" succeeded\n",
				"releasecheck: ok: the e2e record holds 3 live requests, at least as many as " +
					"the 3 scenarios of tools/e2e/required.txt (a count sentinel)\n",
			},
		},
		{
			name: "an end-to-end job that failed", wantExit: exitFailed,
			files: map[string]string{"jobs.json": jobsPages("failure")},
			want: []string{
				"releasecheck: FAIL: the job \"gates / e2e (pinned PDP)\" is completed with the " +
					"conclusion \"failure\", not success: the release needs it to succeed in " +
					"this run\n",
			},
		},
		{
			name: "a run without the CI job", wantExit: exitFailed,
			files: map[string]string{"jobs.json": `{"jobs": [{"name": "gates / e2e (pinned ` +
				`PDP)", "status": "completed", "conclusion": "success"}]}`},
			want: []string{"releasecheck: FAIL: the run has no job \"gates / CI\", so the " +
				"gate did not run\n"},
		},
		{
			name: "fewer live requests than scenarios", wantExit: exitFailed,
			files: map[string]string{"record.jsonl": recordOf("0", live, live,
				requestLine("dryrun"), requestLine("wire"))},
			want: []string{
				"releasecheck: FAIL: the e2e record holds 2 live requests, fewer than the 3 " +
					"scenarios of tools/e2e/required.txt: the scenarios did not run against " +
					"Permit, or did not record\n",
			},
		},
		{
			name: "a record of a run that failed", wantExit: exitFailed,
			files: map[string]string{"record.jsonl": recordOf("1", live, live, live)},
			want:  []string{"releasecheck: FAIL: the e2e record is of a run that exited 1\n"},
		},
		{
			name: "a record without its session trailer", wantExit: exitFailed,
			files: map[string]string{"record.jsonl": recordOf("", live, live, live)},
			want: []string{"releasecheck: FAIL: the e2e record has no session trailer: its " +
				"run did not finish\n"},
		},
		{
			name: "jobs that are not JSON", wantExit: exitDidNotRun,
			files: map[string]string{"jobs.json": "Not Found"},
			want:  []string{"releasecheck: DID NOT RUN: "},
		},
		{
			name:     "a page without jobs",
			wantExit: exitDidNotRun,
			files: map[string]string{
				"jobs.json": jobsPages("success") + `{"message": "Not Found"}`,
			},
			want: []string{"jobs.json: page 3 has no jobs: it is not a list of a run's jobs\n"},
		},
		{
			name: "no page", wantExit: exitDidNotRun,
			files: map[string]string{"jobs.json": "\n"},
			want:  []string{"jobs.json is empty: it is not a list of a run's jobs\n"},
		},
		{
			name: "a record with a line that is not JSON", wantExit: exitDidNotRun,
			files: map[string]string{"record.jsonl": three + "{\n"},
			want:  []string{"record.jsonl:7: unexpected end of JSON input\n"},
		},
		{
			name: "a record with a line of another type", wantExit: exitDidNotRun,
			files: map[string]string{"record.jsonl": three + `{"type": "test"}` + "\n"},
			want: []string{"record.jsonl:7: a line of type \"test\", which a request record " +
				"does not hold\n"},
		},
		{
			name: "a session trailer without its exit status", wantExit: exitDidNotRun,
			files: map[string]string{"record.jsonl": `{"type":"session","tests":1}` + "\n"},
			want:  []string{"record.jsonl:1: a session trailer without exit_status\n"},
		},
		{
			name: "no record", wantExit: exitDidNotRun,
			files: map[string]string{"record.jsonl": ""},
			want:  []string{"record.jsonl: no such file or directory\n"},
		},
		{
			name: "a list of required tests without a scenario", wantExit: exitDidNotRun,
			files: map[string]string{requiredFile: "example.com/e2e TestCloud\n"},
			want: []string{"tools/e2e/required.txt names no subtest, so the e2e record has no " +
				"count to reach\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			files := repoFiles()
			files["jobs.json"] = jobsPages("success")
			files["record.jsonl"] = three
			maps.Copy(files, tt.files)
			for name, content := range files {
				if content == "" {
					delete(files, name)
				}
			}
			writeTree(t, dir, files)

			exit, stdout := runCheck("e2e", "-root", dir,
				"-jobs", filepath.Join(dir, "jobs.json"), "-ci-job", "gates / CI",
				"-e2e-job", "gates / e2e (pinned PDP)",
				"-record", filepath.Join(dir, "record.jsonl"))

			assertResult(t, exit, tt.wantExit, stdout, tt.want...)
		})
	}
}

func TestNotes(t *testing.T) {
	changelog := "# Changelog\n\n## v2.1.0 (unreleased)\n\n### Client\n\n- A change.\n\n\n" +
		"## v2.0.0 (2026-10-30)\n\n- The first release.\n"
	surface := "apisurface: release\ngorelease -base=v2.1.0-rc.1\n" +
		"  incompatible: UsersService.Sync: removed\n" +
		"  incompatible: models.User.Email: changed from string to *string\n" +
		"  # summary\nPASS: v2.1.0: 2 incompatible changes since the prerelease v2.1.0-rc.1\n"
	tests := []struct {
		name, tag, surface string
		changelog          string
		wantExit           int
		want               string
	}{
		{
			name: "the notes of a release candidate", tag: "v2.1.0-rc.2", changelog: changelog,
			wantExit: exitPassed, want: "### Client\n\n- A change.\n",
		},
		{
			name: "the notes of the last section", tag: "v2.0.0", changelog: changelog,
			wantExit: exitPassed, want: "- The first release.\n",
		},
		{
			name: "the notes of a section on the first line", tag: "v2.1.0",
			changelog: "## v2.1.0\n- A change.\n", wantExit: exitPassed, want: "- A change.\n",
		},
		{
			name: "the notes with the incompatible changes after a release candidate",
			tag:  "v2.1.0", changelog: changelog, surface: surface, wantExit: exitPassed,
			want: "### Client\n\n- A change.\n\n### Incompatible changes since v2.1.0-rc.1\n\n" +
				"- UsersService.Sync: removed\n" +
				"- models.User.Email: changed from string to *string\n",
		},
		{
			name: "no incompatible change", tag: "v2.1.0", changelog: changelog,
			surface:  "gorelease -base=none -version=v2.1.0\n# summary\n",
			wantExit: exitPassed, want: "### Client\n\n- A change.\n",
		},
		{
			name: "no section", tag: "v2.2.0", changelog: changelog, wantExit: exitDidNotRun,
			want: "releasecheck: DID NOT RUN: CHANGELOG.md has no section \"## v2.2.0\"\n" +
				verdicts[exitDidNotRun],
		},
		{
			name: "an empty section", tag: "v2.1.0", changelog: "## v2.1.0\n\n## v2.0.0\n",
			wantExit: exitDidNotRun,
			want: "releasecheck: DID NOT RUN: the section \"## v2.1.0\" of CHANGELOG.md is " +
				"empty\n" + verdicts[exitDidNotRun],
		},
		{
			name: "a tag that is not a release tag", tag: "2.1.0", changelog: changelog,
			wantExit: exitDidNotRun,
			want: "releasecheck: DID NOT RUN: the tag \"2.1.0\" is not a v2 release tag, such " +
				"as v2.1.0\n" + verdicts[exitDidNotRun],
		},
		{
			name: "no changelog", tag: "v2.1.0", wantExit: exitDidNotRun,
			want: "CHANGELOG.md: no such file or directory\n" + verdicts[exitDidNotRun],
		},
		{
			name: "no surface output", tag: "v2.1.0", changelog: changelog,
			surface: "-", wantExit: exitDidNotRun,
			want: "surface.txt: no such file or directory\n" + verdicts[exitDidNotRun],
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			files := map[string]string{}
			if tt.changelog != "" {
				files[changelogFile] = tt.changelog
			}
			if tt.surface != "" && tt.surface != "-" {
				files["surface.txt"] = tt.surface
			}
			writeTree(t, dir, files)
			args := []string{"notes", "-root", dir, "-tag", tt.tag}
			if tt.surface != "" {
				args = append(args, "-surface", filepath.Join(dir, "surface.txt"))
			}

			exit, stdout := runCheck(args...)

			if exit != tt.wantExit || !strings.HasSuffix(stdout, tt.want) {
				t.Errorf("notes exited %d and printed\n%s\nwant %d and an end of\n%s", exit,
					stdout, tt.wantExit, tt.want)
			}
			if tt.wantExit == exitPassed && (strings.Contains(stdout, "## v") ||
				strings.HasPrefix(stdout, "\n") || strings.Contains(stdout, "\n\n\n")) {
				t.Errorf("notes printed a heading or extra blank lines:\n%q", stdout)
			}
		})
	}
}
