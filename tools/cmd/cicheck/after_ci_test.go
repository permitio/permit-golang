package main

import (
	"strings"
	"testing"
)

// TestCheckJobAfterCI checks the rules for a job that needs CI, which posts a notice when CI
// failed.
func TestCheckJobAfterCI(t *testing.T) {
	const (
		notifyIf = "      failure() && ((!inputs.mode && github.event_name == 'push') ||\n" +
			"      inputs.mode == 'release')\n"
		notify = "  notify:\n    needs: ci\n    if: >-\n" + notifyIf +
			"    runs-on: ubuntu-24.04\n    timeout-minutes: 5\n    steps:\n" +
			"      - run: .github/scripts/wrap.sh\n"
		wantIf = "failure() && ((!inputs.mode && github.event_name == 'push') || " +
			"inputs.mode == 'release')"
	)
	withNotify := func(from, to string) []change {
		return []change{{file: ciYML, to: validWorkflow + strings.Replace(notify, from, to, 1)}}
	}
	tests := []checkCase{
		{
			name: "a job that needs CI", changes: withNotify("", ""), wantExit: exitPassed,
			wantStdout: []string{"cicheck: checked 1 workflows, 4 jobs, 7 steps and 2 scripts\n"},
		},
		{
			name: "a job that needs CI and another job", wantExit: exitPassed,
			changes: withNotify("needs: ci", "needs: [test, ci]"),
		},
		{
			// A scheduled job posts its own result; a failed scheduled run is red in Actions.
			name: "a job that needs CI and posts on a schedule too",
			changes: withNotify(notifyIf, "      failure() && ((!inputs.mode && "+
				"(github.event_name == 'push' ||\n      github.event_name == 'schedule')) || "+
				"inputs.mode == 'release')\n"),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:75: job notify needs CI, so it runs after CI " +
					"to report a failure, and its if: must be " + wantIf + "\n",
			},
		},
		{
			name: "a job that needs CI with the if: of the pull request checks",
			changes: withNotify(notifyIf, "      (!inputs.mode && (github.event_name == "+
				"'pull_request' || github.event_name == 'push')) || inputs.mode == 'pr' || "+
				"inputs.mode == 'release'\n"),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:75: job notify needs CI, so it runs after CI " +
					"to report a failure, and its if: must be " + wantIf + "\n",
			},
		},
		{
			name:     "a job that needs CI without an if:",
			changes:  withNotify("    if: >-\n", "    if-not: >-\n"),
			wantExit: exitFailed,
			wantStdout: []string{
				"job notify needs CI, so it runs after CI to report a failure",
			},
		},
		{
			name: "CI needs a job that needs CI",
			changes: append(withNotify("", ""),
				inCI("needs: [test, weekly]", "needs: [test, weekly, notify]"),
				inCI("EXPECTED_JOBS: 2", "EXPECTED_JOBS: 3")),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:64: CI's needs lists notify, which needs CI; " +
					"a job that runs after CI cannot be one CI waits for\n",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflows := writeRepository(t, withChanges(t, tt.changes))

			exit, stdout, _ := runCicheck(t, "-workflows", workflows)

			if exit != tt.wantExit {
				t.Errorf("exit = %d, want %d; stdout:\n%s", exit, tt.wantExit, stdout)
			}
			for _, want := range tt.wantStdout {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout does not contain %q:\n%s", want, stdout)
				}
			}
		})
	}
}
