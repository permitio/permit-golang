package scripts

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// stopCommandsPattern matches a stop-commands line and the line that resumes the commands, and
// captures the token of each.
var stopCommandsPattern = regexp.MustCompile(
	`(?s)::stop-commands::([0-9a-f]{32})\n(.*?)::([0-9a-f]{32})::\n`)

// assertWrapped checks that output stops workflow commands with a random-looking token and
// resumes them with the same token, and that one of its stopped parts contains inside.
func assertWrapped(t *testing.T, output, inside string) {
	t.Helper()
	blocks := stopCommandsPattern.FindAllStringSubmatch(output, -1)
	if blocks == nil {
		t.Errorf("output has no ::stop-commands::<token> ... ::<token>:: block:\n%s", output)
		return
	}
	found := false
	for _, m := range blocks {
		if m[1] != m[3] {
			t.Errorf("stop-commands token %s is resumed with %s", m[1], m[3])
		}
		found = found || strings.Contains(m[2], inside)
	}
	if !found {
		t.Errorf("no stopped part contains %q:\n%s", inside, output)
	}
}

func TestCIAggregate(t *testing.T) {
	const (
		securityCron = "23 4 * * 1,4"
		weeklyCron   = "41 6 * * 1"
	)
	// needs returns toJSON(needs) for the results of the four jobs ci.yml needs, as
	// "prek test crossbuild prek-upstream".
	needs := func(prek, test, crossbuild, upstream string) string {
		return `{"prek": {"result": "` + prek + `", "outputs": {}}, "test": {"result": "` + test +
			`", "outputs": {}}, "crossbuild": {"result": "` + crossbuild +
			`", "outputs": {}}, "prek-upstream": {"result": "` + upstream + `", "outputs": {}}}`
	}
	allPassed := needs("success", "success", "success", "skipped")
	weeklyPassed := needs("skipped", "skipped", "skipped", "success")
	noneRan := needs("skipped", "skipped", "skipped", "skipped")
	type testCase struct {
		name string
		// env replaces or adds to the variables the CI job sets: EVENT pull_request, NEEDS
		// allPassed, EXPECTED_JOBS 4 and SCHEDULED_JOBS prek-upstream:weekly.
		env        map[string]string
		args       []string
		wantExit   int
		wantStdout []string
	}
	tests := []testCase{
		{
			name: "a pull request", wantExit: 0,
			wantStdout: []string{
				"ci-aggregate: mode pull_request: 4 job results\n",
				"::notice title=CI::prek-upstream was not selected in mode pull_request\n",
				"ci-aggregate: every job that mode pull_request selects succeeded\n",
			},
		},
		{
			name: "a push", env: map[string]string{"EVENT": "push"}, wantExit: 0,
			wantStdout: []string{"ci-aggregate: every job that mode push selects succeeded\n"},
		},
		{
			name: "a run in mode pr",
			env:  map[string]string{"EVENT": "workflow_dispatch", "INPUT_MODE": "pr"}, wantExit: 0,
			wantStdout: []string{"ci-aggregate: every job that mode pr selects succeeded\n"},
		},
		{
			name: "a release, which calls CI with the caller's push event",
			env:  map[string]string{"EVENT": "push", "INPUT_MODE": "release"}, wantExit: 0,
			wantStdout: []string{"ci-aggregate: every job that mode release selects succeeded\n"},
		},
		{
			name: "the weekly cron",
			env: map[string]string{
				"EVENT": "schedule", "SCHEDULE": weeklyCron, "NEEDS": weeklyPassed,
			},
			wantExit: 0,
			wantStdout: []string{
				"ci-aggregate: mode weekly: 4 job results\n",
				"::notice title=CI::prek was not selected in mode weekly\n",
				"ci-aggregate: every job that mode weekly selects succeeded\n",
			},
		},
		{
			name: "mode weekly",
			env: map[string]string{
				"EVENT": "workflow_dispatch", "INPUT_MODE": "weekly", "NEEDS": weeklyPassed,
			},
			wantExit:   0,
			wantStdout: []string{"ci-aggregate: every job that mode weekly selects succeeded\n"},
		},
		{
			name: "the security cron, which selects none of these jobs",
			env: map[string]string{
				"EVENT": "schedule", "SCHEDULE": securityCron, "NEEDS": noneRan,
			},
			wantExit: 0,
			wantStdout: []string{
				"::notice title=CI::prek-upstream was not selected in mode security\n",
				"ci-aggregate: every job that mode security selects succeeded\n",
			},
		},
		{
			name: "mode security",
			env: map[string]string{
				"EVENT": "workflow_dispatch", "INPUT_MODE": "security", "NEEDS": noneRan,
			},
			wantExit: 0,
			wantStdout: []string{
				"::notice title=CI::prek was not selected in mode security\n",
				"ci-aggregate: every job that mode security selects succeeded\n",
			},
		},
		{
			name: "mode e2e",
			env: map[string]string{
				"EVENT": "workflow_dispatch", "INPUT_MODE": "e2e", "NEEDS": noneRan,
			},
			wantExit:   0,
			wantStdout: []string{"ci-aggregate: every job that mode e2e selects succeeded\n"},
		},
		{
			name:     "a failed job",
			env:      map[string]string{"NEEDS": needs("success", "failure", "success", "skipped")},
			wantExit: 1,
			wantStdout: []string{
				"::error title=CI::Jobs that did not succeed in mode pull_request: test failure\n",
			},
		},
		{
			name: "a cancelled job",
			env: map[string]string{
				"NEEDS": needs("success", "success", "cancelled", "skipped"),
			},
			wantExit: 1,
			wantStdout: []string{
				"::error title=CI::Jobs that did not succeed in mode pull_request: " +
					"crossbuild cancelled\n",
			},
		},
		{
			name:     "a pull request check skipped on a pull request",
			env:      map[string]string{"NEEDS": needs("skipped", "success", "success", "skipped")},
			wantExit: 1,
			wantStdout: []string{
				"::error title=CI::Jobs that did not succeed in mode pull_request: prek skipped, " +
					"although mode pull_request selects it\n",
			},
		},
		{
			name: "a pull request check skipped on a push",
			env: map[string]string{
				"EVENT": "push", "NEEDS": needs("success", "skipped", "success", "skipped"),
			},
			wantExit: 1,
			wantStdout: []string{
				"::error title=CI::Jobs that did not succeed in mode push: test skipped, " +
					"although mode push selects it\n",
			},
		},
		{
			name: "a pull request check skipped in mode pr",
			env: map[string]string{
				"EVENT": "workflow_dispatch", "INPUT_MODE": "pr",
				"NEEDS": needs("success", "success", "skipped", "skipped"),
			},
			wantExit:   1,
			wantStdout: []string{"crossbuild skipped, although mode pr selects it"},
		},
		{
			name: "a pull request check skipped in a release",
			env: map[string]string{
				"EVENT": "push", "INPUT_MODE": "release",
				"NEEDS": needs("success", "skipped", "success", "skipped"),
			},
			wantExit: 1,
			wantStdout: []string{
				"test skipped, although mode release selects it",
			},
		},
		{
			name: "a scheduled job skipped on its cron",
			env: map[string]string{
				"EVENT": "schedule", "SCHEDULE": weeklyCron, "NEEDS": noneRan,
			},
			wantExit: 1,
			wantStdout: []string{
				"prek-upstream skipped, although mode weekly selects it",
			},
		},
		{
			name: "a scheduled job that failed",
			env: map[string]string{
				"EVENT": "workflow_dispatch", "INPUT_MODE": "weekly",
				"NEEDS": needs("skipped", "skipped", "skipped", "failure"),
			},
			wantExit:   1,
			wantStdout: []string{"in mode weekly: prek-upstream failure\n"},
		},
		{
			name: "a failed advisory job",
			env: map[string]string{
				"ADVISORY_JOBS": "crossbuild",
				"NEEDS":         needs("success", "success", "failure", "skipped"),
			},
			wantExit: 0,
			wantStdout: []string{
				"::warning title=CI::crossbuild failure; it is advisory, so CI does not fail\n",
				"ci-aggregate: every job that mode pull_request selects succeeded\n",
			},
		},
		{
			name: "an advisory scheduled job that failed on a push",
			env: map[string]string{
				"EVENT": "push", "ADVISORY_JOBS": "prek-upstream",
				"NEEDS": needs("success", "success", "success", "failure"),
			},
			wantExit: 0,
			wantStdout: []string{
				"::warning title=CI::prek-upstream failure; it is advisory, so CI does not fail\n",
				"ci-aggregate: every job that mode push selects succeeded\n",
			},
		},
		{
			name: "an advisory job skipped in a run that selects it",
			env: map[string]string{
				"ADVISORY_JOBS": "crossbuild",
				"NEEDS":         needs("success", "success", "skipped", "skipped"),
			},
			wantExit:   1,
			wantStdout: []string{"crossbuild skipped, although mode pull_request selects it"},
		},
		{
			name: "a pull request only job skipped on a push",
			env: map[string]string{
				"EVENT": "push", "PULL_REQUEST_JOBS": "crossbuild",
				"NEEDS": needs("success", "success", "skipped", "skipped"),
			},
			wantExit: 0,
			wantStdout: []string{
				"::notice title=CI::crossbuild was not selected in mode push\n",
			},
		},
		{
			name: "a pull request only job skipped on a pull request",
			env: map[string]string{
				"PULL_REQUEST_JOBS": "crossbuild",
				"NEEDS":             needs("success", "success", "skipped", "skipped"),
			},
			wantExit:   1,
			wantStdout: []string{"crossbuild skipped, although mode pull_request selects it"},
		},
		{
			name: "EXPECTED_JOBS off by one", env: map[string]string{"EXPECTED_JOBS": "5"},
			wantExit: 2,
			wantStdout: []string{
				"::error title=CI::4 job results, but EXPECTED_JOBS is 5; CI did not check every " +
					"job\n",
			},
		},
		{
			name: "EXPECTED_JOBS not a number", env: map[string]string{"EXPECTED_JOBS": "four"},
			wantExit:   2,
			wantStdout: []string{"EXPECTED_JOBS \"four\" is not a positive number"},
		},
		{
			name: "NEEDS that is not JSON", env: map[string]string{"NEEDS": "{prek: success"},
			wantExit:   2,
			wantStdout: []string{"::error title=CI::NEEDS is not a JSON object of job results\n"},
		},
		{
			name: "NEEDS that is a JSON array", env: map[string]string{"NEEDS": `["prek"]`},
			wantExit:   2,
			wantStdout: []string{"::error title=CI::NEEDS is not a JSON object of job results\n"},
		},
		{
			name: "no NEEDS", env: map[string]string{"NEEDS": ""}, wantExit: 2,
			wantStdout: []string{"0 job results, but EXPECTED_JOBS is 4"},
		},
		{
			name: "a result CI does not know",
			env: map[string]string{
				"NEEDS": needs("success", "neutral", "success", "skipped"),
			},
			wantExit:   2,
			wantStdout: []string{"job test has the result \"neutral\", which CI does not know"},
		},
		{
			name:     "a mode CI does not know",
			env:      map[string]string{"EVENT": "workflow_dispatch", "INPUT_MODE": "nightly"},
			wantExit: 2,
			wantStdout: []string{
				"the mode \"nightly\" is not one of pr, release, e2e, security and weekly",
			},
		},
		{
			name: "a mode input that tries to issue a workflow command",
			env: map[string]string{
				"EVENT": "workflow_call", "INPUT_MODE": "pr\n::warning::injected",
			},
			wantExit: 2,
			wantStdout: []string{
				"::error title=CI::the mode input is not one of pr, release, e2e, security and " +
					"weekly\n",
			},
		},
		{
			name: "a job id with a space",
			env: map[string]string{
				"EXPECTED_JOBS": "1", "SCHEDULED_JOBS": "",
				"NEEDS": `{"a job": {"result": "success"}}`,
			},
			wantExit: 2,
			wantStdout: []string{
				"NEEDS has a job id with characters other than letters, digits, - and _",
			},
		},
		{
			name: "an event without a mode",
			env:  map[string]string{"EVENT": "workflow_dispatch"}, wantExit: 2,
			wantStdout: []string{
				"CI does not run on the event \"workflow_dispatch\" without a mode",
			},
		},
		{
			name: "a cron that no <MODE>_CRON declares",
			env:  map[string]string{"EVENT": "schedule", "SCHEDULE": "0 0 * * *"}, wantExit: 2,
			wantStdout: []string{"the cron \"0 0 * * *\" is not the value of any <MODE>_CRON"},
		},
		{
			name: "a cron for a mode CI does not know",
			env: map[string]string{
				"EVENT": "schedule", "SCHEDULE": "0 0 * * *", "NIGHTLY_CRON": "0 0 * * *",
			},
			wantExit:   2,
			wantStdout: []string{"NIGHTLY_CRON names the mode nightly, which is not one of"},
		},
		{
			name: "two crons with the same value",
			env: map[string]string{
				"EVENT": "schedule", "SCHEDULE": weeklyCron, "E2E_CRON": weeklyCron,
			},
			wantExit:   2,
			wantStdout: []string{"two <MODE>_CRON variables are \"41 6 * * 1\""},
		},
		{
			name: "ADVISORY_JOBS with a job CI does not need",
			env:  map[string]string{"ADVISORY_JOBS": "e2e"}, wantExit: 2,
			wantStdout: []string{"ADVISORY_JOBS names e2e, which CI does not need"},
		},
		{
			name: "PULL_REQUEST_JOBS with a job CI does not need",
			env:  map[string]string{"PULL_REQUEST_JOBS": "lint"}, wantExit: 2,
			wantStdout: []string{"::error title=CI::PULL_REQUEST_JOBS names lint, which CI does " +
				"not need\n"},
		},
		{
			name: "SCHEDULED_JOBS with a job CI does not need",
			env:  map[string]string{"SCHEDULED_JOBS": "lint:weekly"}, wantExit: 2,
			wantStdout: []string{"::error title=CI::SCHEDULED_JOBS names lint, which CI does " +
				"not need\n"},
		},
		{
			name: "a SCHEDULED_JOBS entry that is not job:mode",
			env:  map[string]string{"SCHEDULED_JOBS": "prek-upstream"}, wantExit: 2,
			wantStdout: []string{"SCHEDULED_JOBS entry \"prek-upstream\" is not job:<mode>"},
		},
		{
			name: "a SCHEDULED_JOBS entry for a mode that is not periodic",
			env:  map[string]string{"SCHEDULED_JOBS": "prek-upstream:pr"}, wantExit: 2,
			wantStdout: []string{
				"SCHEDULED_JOBS entry \"prek-upstream:pr\" is not job:<mode>",
			},
		},
		{
			name: "arguments", args: []string{"x"}, wantExit: 2,
			wantStdout: []string{"usage: ci-aggregate.sh (it takes no arguments)"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			copyScripts(t, root, "ci-aggregate.sh")
			vars := map[string]string{
				"EVENT": "pull_request", "INPUT_MODE": "", "SCHEDULE": "", "NEEDS": allPassed,
				"EXPECTED_JOBS": "4", "PULL_REQUEST_JOBS": "", "ADVISORY_JOBS": "",
				"SCHEDULED_JOBS": "prek-upstream:weekly", "SECURITY_CRON": securityCron,
				"WEEKLY_CRON": weeklyCron,
			}
			maps.Copy(vars, tt.env)
			var env []string
			for name, value := range vars {
				env = append(env, name+"="+value)
			}

			result := runScript(t, root, env,
				filepath.Join(root, ".github", "scripts", "ci-aggregate.sh"), tt.args...)

			if result.exit != tt.wantExit {
				t.Errorf("exit = %d, want %d; stdout:\n%s\nstderr:\n%s", result.exit,
					tt.wantExit, result.stdout, result.stderr)
			}
			for _, want := range tt.wantStdout {
				if !strings.Contains(result.stdout, want) {
					t.Errorf("stdout does not contain %q:\n%s", want, result.stdout)
				}
			}
			if tt.wantExit != 2 {
				assertWrapped(t, result.stdout, "prek ")
			}
			if strings.Contains(result.stdout, "injected") {
				t.Errorf("stdout prints the mode input:\n%s", result.stdout)
			}
		})
	}

	t.Run("without jq", func(t *testing.T) {
		root := t.TempDir()
		copyScripts(t, root, "ci-aggregate.sh")
		bin := t.TempDir()
		for _, tool := range []string{"bash", "od", "tr"} {
			path, err := exec.LookPath(tool)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(path, filepath.Join(bin, tool)); err != nil {
				t.Fatal(err)
			}
		}
		env := []string{
			"PATH=" + bin, "EVENT=pull_request", "NEEDS=" + allPassed, "EXPECTED_JOBS=4",
		}

		result := runScript(t, root, env,
			filepath.Join(root, ".github", "scripts", "ci-aggregate.sh"))

		if result.exit != 2 || !strings.Contains(result.stdout, "jq is not installed") {
			t.Errorf("exit = %d, want 2 with a message about jq; stdout:\n%s\nstderr:\n%s",
				result.exit, result.stdout, result.stderr)
		}
	})

	t.Run("a new token in each run", func(t *testing.T) {
		root := t.TempDir()
		copyScripts(t, root, "ci-aggregate.sh")
		env := []string{
			"EVENT=pull_request", "NEEDS=" + allPassed, "EXPECTED_JOBS=4",
			"SCHEDULED_JOBS=prek-upstream:weekly",
		}
		script := filepath.Join(root, ".github", "scripts", "ci-aggregate.sh")
		first := stopCommandsPattern.FindStringSubmatch(runScript(t, root, env, script).stdout)
		second := stopCommandsPattern.FindStringSubmatch(runScript(t, root, env, script).stdout)
		if first == nil || second == nil || first[1] == second[1] {
			t.Errorf("want a different stop-commands token in each run, got %q and %q",
				first, second)
		}
	})
}
