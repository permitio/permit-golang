package scripts

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// prekStub is a stand-in for prek. It appends its arguments and PREK_NO_FAST_PATH to the file
// STUB_LOG names, prints the file STUB_FAST or, when PREK_NO_FAST_PATH is set, STUB_UPSTREAM,
// and exits with STUB_FAST_EXIT or STUB_UPSTREAM_EXIT.
const prekStub = `#!/bin/sh
printf '%s|%s\n' "$*" "${PREK_NO_FAST_PATH:-}" >>"$STUB_LOG"
if [ -n "${PREK_NO_FAST_PATH:-}" ]; then
	cat "$STUB_UPSTREAM"
	exit "$STUB_UPSTREAM_EXIT"
fi
cat "$STUB_FAST"
exit "$STUB_FAST_EXIT"
`

// hookLine returns the line prek prints for a hook's result, padded with dots to 79 columns.
func hookLine(name, result string) string {
	return name + strings.Repeat(".", 79-len(name)-len(result)) + result + "\n"
}

func TestPrekCount(t *testing.T) {
	t.Parallel()
	const injection = "::error::a hook printed this\n"
	var (
		passedA  = hookLine("check yaml", "Passed")
		noFilesB = hookLine("check json", "(no files to check)Skipped")
		passedB  = hookLine("check json", "Passed")
		passedC  = hookLine("typos", "Passed")
		skippedC = hookLine("typos", "Skipped")
		failedC  = hookLine("typos", "Failed")
		passedD  = hookLine("shfmt", "Passed")
		clean    = passedA + noFilesB + passedC
	)
	type testCase struct {
		name string
		args []string
		// fast and upstream are what prek prints without and with PREK_NO_FAST_PATH, and
		// fastExit and upstreamExit its exit codes.
		fast, upstream         string
		fastExit, upstreamExit int
		// expected is EXPECTED_HOOKS; noPrek runs the script without an installed prek.
		expected   string
		noPrek     bool
		wantExit   int
		wantStdout []string
		wantStderr []string
		// wantWrapped must be printed between stop-commands lines. Without --upstream, the test
		// checks that for what prek printed too.
		wantWrapped string
		// wantRuns are the lines the stub logs: its arguments, then PREK_NO_FAST_PATH.
		wantRuns []string
	}
	const runArgs = "run --all-files --show-diff-on-failure --color=never --no-progress " +
		"--no-hide-status"
	tests := []testCase{
		{
			name: "every hook passed or had no files", fast: clean, expected: "3", wantExit: 0,
			wantStderr: []string{
				"prek-count: 3 of 3 hooks passed or had nothing to check (built-in hooks)\n",
			},
			wantRuns: []string{runArgs + "|"},
		},
		{
			name: "a hook that printed a workflow command", fast: injection + clean,
			expected: "3", wantExit: 0, wantStdout: []string{injection},
		},
		{
			name: "a hook that SKIP skipped", fast: passedA + noFilesB + skippedC, expected: "3",
			wantExit: 2,
			wantStderr: []string{
				"prek-count: 2 of 3 hooks passed or had nothing to check (built-in hooks); a " +
					"hook was skipped, added or removed: run every hook, or change " +
					"EXPECTED_HOOKS\n",
			},
		},
		{
			name: "a hook removed", fast: passedA + passedC, expected: "3", wantExit: 2,
			wantStderr: []string{"prek-count: 2 of 3 hooks passed or had nothing to check"},
		},
		{
			name: "a hook added", fast: clean + passedD, expected: "3", wantExit: 2,
			wantStderr: []string{"prek-count: 4 of 3 hooks passed or had nothing to check"},
		},
		{
			name: "a hook failed", fast: passedA + noFilesB + failedC, fastExit: 1, expected: "3",
			wantExit:   1,
			wantStderr: []string{"prek-count: a hook failed (built-in hooks); see its output"},
		},
		{
			name: "prek did not run the hooks", fast: "error: invalid config\n", fastExit: 2,
			expected: "3", wantExit: 2,
			wantStderr: []string{
				"prek-count: prek exited 2 (built-in hooks), so the hooks did not run",
			},
		},
		{
			name: "no output", fast: "", expected: "3", wantExit: 2,
			wantStderr: []string{"prek-count: 0 of 3 hooks passed or had nothing to check"},
		},
		{
			name: "upstream hooks that match", args: []string{"--upstream"}, fast: clean,
			upstream: clean, expected: "3", wantExit: 0,
			wantStderr: []string{
				"prek-count: 3 of 3 hooks passed or had nothing to check (upstream hooks)\n",
				"prek-count: the built-in and upstream hooks reported every hook the same way\n",
			},
			wantRuns: []string{runArgs + "|", runArgs + "|1"},
		},
		{
			name: "an upstream hook that fails", args: []string{"--upstream"}, fast: clean,
			upstream: passedA + noFilesB + failedC, upstreamExit: 1, expected: "3", wantExit: 1,
			wantStderr: []string{"prek-count: a hook failed (upstream hooks); see its output"},
		},
		{
			name: "an upstream hook that reports differently", args: []string{"--upstream"},
			fast: clean, upstream: passedA + passedB + passedC, expected: "3", wantExit: 1,
			wantStdout:  []string{"-" + noFilesB, "+" + passedB},
			wantWrapped: "--- built-in\n+++ upstream\n",
			wantStderr: []string{
				"prek-count: the built-in and upstream hooks reported hooks differently",
			},
		},
		{
			name:       "a built-in hook that fails before the upstream run",
			args:       []string{"--upstream"},
			fast:       passedA + noFilesB + failedC,
			fastExit:   1,
			upstream:   clean,
			expected:   "3",
			wantExit:   1,
			wantStderr: []string{"prek-count: a hook failed (built-in hooks); see its output"},
			wantRuns:   []string{runArgs + "|"},
		},
		{
			name: "EXPECTED_HOOKS not set", fast: clean, expected: "", wantExit: 2,
			wantStderr: []string{"prek-count: EXPECTED_HOOKS \"\" is not a positive number"},
		},
		{
			name: "EXPECTED_HOOKS zero", fast: clean, expected: "0", wantExit: 2,
			wantStderr: []string{"prek-count: EXPECTED_HOOKS \"0\" is not a positive number"},
		},
		{
			name: "no prek", noPrek: true, expected: "3", wantExit: 2,
			wantStderr: []string{
				"bin/prek is not an executable; install it with .github/scripts/fetch-tool.sh prek",
			},
		},
		{
			name: "an unknown argument", args: []string{"--all"}, expected: "3", wantExit: 2,
			wantStderr: []string{"prek-count: usage: prek-count.sh [--upstream]"},
		},
		{
			name: "two arguments", args: []string{"--upstream", "--upstream"}, fast: clean,
			upstream: clean, expected: "3", wantExit: 2,
			wantStderr: []string{"prek-count: usage: prek-count.sh [--upstream]"},
		},
	}
	prek := writeStub(t, "prek", prekStub)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			linkScripts(t, root, "prek-count.sh")
			files := t.TempDir()
			writeFiles(t, files, map[string]string{"fast": tt.fast, "upstream": tt.upstream})
			if !tt.noPrek {
				linkFile(t, root, "bin/prek", prek)
			}
			log := filepath.Join(files, "log")
			env := []string{
				"EXPECTED_HOOKS=" + tt.expected, "PREK_NO_FAST_PATH=", "STUB_LOG=" + log,
				"STUB_FAST=" + filepath.Join(files, "fast"),
				"STUB_UPSTREAM=" + filepath.Join(files, "upstream"),
				"STUB_FAST_EXIT=" + strconv.Itoa(tt.fastExit),
				"STUB_UPSTREAM_EXIT=" + strconv.Itoa(tt.upstreamExit),
			}

			result := runScript(t, root, env,
				filepath.Join(root, ".github", "scripts", "prek-count.sh"), tt.args...)

			if result.exit != tt.wantExit {
				t.Errorf("exit = %d, want %d; stdout:\n%s\nstderr:\n%s", result.exit,
					tt.wantExit, result.stdout, result.stderr)
			}
			for _, want := range tt.wantStdout {
				if !strings.Contains(result.stdout, want) {
					t.Errorf("stdout does not contain %q:\n%s", want, result.stdout)
				}
			}
			for _, want := range tt.wantStderr {
				if !strings.Contains(result.stderr, want) {
					t.Errorf("stderr does not contain %q:\n%s", want, result.stderr)
				}
			}
			if !tt.noPrek && tt.expected != "" && tt.expected != "0" && tt.args == nil {
				assertWrapped(t, result.stdout, tt.fast)
			}
			if tt.wantWrapped != "" {
				assertWrapped(t, result.stdout, tt.wantWrapped)
			}
			if tt.wantRuns != nil {
				logged, err := os.ReadFile(log)
				if err != nil {
					t.Fatal(err)
				}
				got := strings.Split(strings.TrimSuffix(string(logged), "\n"), "\n")
				if !slices.Equal(got, tt.wantRuns) {
					t.Errorf("prek ran with %q, want %q", got, tt.wantRuns)
				}
			}
		})
	}
}
