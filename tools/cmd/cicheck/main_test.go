package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	checkoutSHA = "3d3c42e5aac5ba805825da76410c181273ba90b1"
	setupGoSHA  = "b7ad1dad31e06c5925ef5d2fc7ad053ef454303e"
	ciYML       = ".github/workflows/ci.yml"
)

// validWorkflow is a workflow that follows every rule. The tests break one rule at a time by
// replacing a part of it; the line numbers they expect are this file's.
//
//go:embed testdata/ci.yml
var validWorkflow string

// validScript wraps its output in stop-commands and writes a multi-line value with a random
// delimiter.
const validScript = `#!/usr/bin/env bash
# A version in a comment, such as 1.2.3, is allowed.
token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
echo "::stop-commands::$token"
delimiter=$token
echo "report<<$delimiter" >>"$GITHUB_OUTPUT"
curl --proxy http://127.0.0.1:9 https://example.com/
done <<<"$listed"
`

const validLock = `# The lock.
[fake-tool]
version = 1.2.3
url = https://example.com/fake-tool-{version}.tar.gz
`

// thirdParty is a workflow that calls a reusable workflow of another repository, and uses a
// local action, an action in a subdirectory of its repository and two actions out of order, one
// without a version comment.
const thirdParty = "on: push\njobs:\n" +
	"  call:\n    uses: octo/repo/.github/workflows/w.yml@" + checkoutSHA + "  # v1.0.0\n" +
	"  build:\n    runs-on: ubuntu-24.04\n    timeout-minutes: 5\n    steps:\n" +
	"      - uses: ./.github/actions/x\n" +
	"      - uses: octo/tools/setup@" + setupGoSHA + "  # v2.0.0\n" +
	"      - uses: octo/b@" + setupGoSHA + "  # v1.1.0\n" +
	"      - uses: octo/a@" + checkoutSHA + "\n"

// validFiles returns the files of a repository that follows every rule.
func validFiles() map[string]string {
	return map[string]string{
		ciYML:                         validWorkflow,
		".github/scripts/wrap.sh":     validScript,
		".github/scripts/no-version":  "echo 127.0.0.1\n",
		".github/zizmor.yml":          "rules: {}\n",
		"tools/binaries.lock":         validLock,
		".github/workflows/README.md": "not a workflow\n",
	}
}

// change is one change to the valid repository. It replaces from with to in file; when from is
// empty, it writes to as the whole file, and when remove is set, it removes the file.
type change struct {
	file, from, to string
	remove         bool
}

// inCI replaces from with to in ci.yml.
func inCI(from, to string) change {
	return change{file: ciYML, from: from, to: to}
}

// withChanges returns the files of the valid repository with the changes made, and fails the
// test when a file does not have the text a change replaces.
func withChanges(t *testing.T, changes []change) map[string]string {
	t.Helper()
	files := validFiles()
	for _, c := range changes {
		switch {
		case c.remove:
			delete(files, c.file)
		case c.from == "":
			files[c.file] = c.to
		case !strings.Contains(files[c.file], c.from):
			t.Fatalf("%s has no %q", c.file, c.from)
		default:
			files[c.file] = strings.Replace(files[c.file], c.from, c.to, 1)
		}
	}
	return files
}

// writeRepository writes a repository layout under a new directory: files maps a slash path
// relative to the root to its content. It returns the workflows directory.
func writeRepository(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(root, ".github", "workflows")
}

// runCicheck runs cicheck with args and returns its exit code, stdout and stderr.
func runCicheck(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exit := run(t.Context(), args, &stdout, &stderr)
	return exit, stdout.String(), stderr.String()
}

// verdicts is the last line cicheck prints for each exit code when it checks.
var verdicts = map[int]string{
	exitPassed:    "cicheck: PASS\n",
	exitFailed:    "cicheck: FAIL\n",
	exitDidNotRun: "cicheck: DID NOT RUN\n",
}

// checkCase is a repository that TestCheck checks: the valid one with changes made.
type checkCase struct {
	name       string
	changes    []change
	wantExit   int
	wantStdout []string
}

func TestCheck(t *testing.T) {
	const (
		weeklyIf = "if: (!inputs.mode && github.event.schedule == '0 1 * * 1') || " +
			"inputs.mode == 'weekly'"
		ciRun     = "        run: .github/scripts/wrap.sh\n"
		cacheStep = "          go-version-file: go.mod\n          cache: false\n"
		matrixGo  = "go-version: ${{ env[matrix.go] }}"
		cron      = "    - cron: \"0 1 * * 1\"\n"
		cronEnv   = "  WEEKLY_CRON: \"0 1 * * 1\"\n"
		otherYML  = ".github/workflows/other.yml"
		release   = ".github/workflows/release.yml"
		tokenLine = "          token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \\n')\n"
		stopLine  = "          echo \"::stop-commands::$token\"\n"
		trapLine  = "          trap 'echo \"::$token::\"' EXIT\n"
		gateExpr  = "(!inputs.mode && (github.event_name == 'pull_request' || " +
			"github.event_name == 'push')) || inputs.mode == 'pr' || inputs.mode == 'release'"
		gateEnd   = "      || inputs.mode == 'pr' || inputs.mode == 'release'\n"
		gateLines = "    if: >-\n      (!inputs.mode && (github.event_name == 'pull_request' || " +
			"github.event_name == 'push'))\n" + gateEnd
		scheduledJobs = "SCHEDULED_JOBS: weekly:weekly"
		weeklyRunsOn  = "    runs-on: ubuntu-24.04\n    timeout-minutes: 10\n    steps"
		notWrapped    = "run: does not start with the three lines that stop workflow commands"
		hardVersion   = "#!/usr/bin/env bash\n# Downloads prek.\n" +
			"curl -o prek.tar.gz https://example.com/prek/v0.5.5/prek.tar.gz\n"
	)
	fingerprintLike := strings.Repeat("ab", 20) + ":internal/x_test.go:generic-api-key:87"
	withoutWeekly := validWorkflow[:strings.Index(validWorkflow, "  weekly:\n")] +
		validWorkflow[strings.Index(validWorkflow, "  ci:\n"):]
	tests := []checkCase{
		{
			name: "a repository that follows every rule", wantExit: exitPassed,
			wantStdout: []string{
				"cicheck: checked 1 workflows, 3 jobs, 6 steps and 2 scripts\ncicheck: PASS\n",
			},
		},
		{
			name: "a job dropped from CI's needs",
			changes: []change{
				inCI("needs: [test, weekly]", "needs: [test]"),
				inCI("EXPECTED_JOBS: 2", "EXPECTED_JOBS: 1"),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:48: CI's needs does not list job weekly, so " +
					"CI would pass whatever it does\n",
				"SCHEDULED_JOBS names weekly, which CI does not need",
				"cicheck: FAIL\n",
			},
		},
		{
			name:     "CI's needs names a job that does not exist",
			changes:  []change{inCI("needs: [test, weekly]", "needs: [test, weekly, lint]")},
			wantExit: exitFailed,
			wantStdout: []string{
				"CI's needs lists lint, which is not another job of this workflow",
				"EXPECTED_JOBS is \"2\", but CI needs 3 jobs",
			},
		},
		{
			name:       "CI's needs lists a job twice",
			changes:    []change{inCI("needs: [test, weekly]", "needs: [test, weekly, test]")},
			wantExit:   exitFailed,
			wantStdout: []string{"CI's needs lists test twice"},
		},
		{
			name:       "CI needs itself",
			changes:    []change{inCI("needs: [test, weekly]", "needs: [test, weekly, ci]")},
			wantExit:   exitFailed,
			wantStdout: []string{"CI's needs lists ci, which is not another job"},
		},
		{
			name: "CI's needs as a scalar",
			changes: []change{
				{file: ciYML, to: withoutWeekly},
				inCI("needs: [test, weekly]", "needs: test"),
				inCI("EXPECTED_JOBS: 2", "EXPECTED_JOBS: 1"),
				inCI(scheduledJobs, `SCHEDULED_JOBS: ""`),
			},
			wantExit: exitPassed,
		},
		{
			name:     "EXPECTED_JOBS off by one",
			changes:  []change{inCI("EXPECTED_JOBS: 2", "EXPECTED_JOBS: 3")},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:70: EXPECTED_JOBS is \"3\", but CI needs 2 " +
					"jobs\n",
			},
		},
		{
			name:       "no EXPECTED_JOBS",
			changes:    []change{inCI("EXPECTED_JOBS: 2", "EXPECTED: 2")},
			wantExit:   exitFailed,
			wantStdout: []string{"no step of job ci sets EXPECTED_JOBS"},
		},
		{
			name: "two steps set EXPECTED_JOBS",
			changes: []change{
				inCI(ciRun, ciRun+"      - env:\n          EXPECTED_JOBS: 2\n"+ciRun),
			},
			wantExit:   exitFailed,
			wantStdout: []string{"2 steps of job ci set EXPECTED_JOBS; keep one"},
		},
		{
			name:       "the CI job under another name",
			changes:    []change{inCI("    name: CI\n", "    name: Aggregate\n")},
			wantExit:   exitFailed,
			wantStdout: []string{"job ci is named \"Aggregate\"; name it CI"},
		},
		{
			name:       "the CI job without if: always()",
			changes:    []change{inCI("if: always()", "if: success()")},
			wantExit:   exitFailed,
			wantStdout: []string{"job ci needs if: always()"},
		},
		{
			name:     "the CI job's if: in an expression",
			changes:  []change{inCI("if: always()", "if: ${{ always() }}")},
			wantExit: exitPassed,
		},
		{
			name:     "no CI job",
			changes:  []change{inCI("  ci:\n    name: CI\n", "  aggregate:\n    name: CI\n")},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows: no workflow has a job \"ci\" named \"CI\"",
			},
		},
		{
			name: "two workflows with a CI job",
			changes: []change{{
				file: otherYML,
				to:   strings.Replace(validWorkflow, "group: ci-", "group: other-", 1),
			}},
			wantExit: exitFailed,
			wantStdout: []string{
				"2 workflows have a job \"ci\" named \"CI\": .github/workflows/ci.yml, " +
					".github/workflows/other.yml; keep one",
			},
		},
		{
			name: "a pull request check without an if:",
			changes: []change{
				inCI(gateLines, ""),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:21: job test is outside SCHEDULED_JOBS and " +
					"PULL_REQUEST_JOBS, so its if: must be " + gateExpr + "\n",
			},
		},
		{
			name: "a pull request check with another if:",
			changes: []change{
				inCI(gateEnd, "      || inputs.mode == 'pr'\n"),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:21: job test is outside SCHEDULED_JOBS and " +
					"PULL_REQUEST_JOBS, so its if: must be",
			},
		},
		{
			name: "a pull request check whose if: is in an expression",
			changes: []change{
				inCI(gateLines, "    if: ${{ "+gateExpr+" }}\n"),
			},
			wantExit: exitPassed,
		},
		{
			name: "a job that is not scheduled but runs on the schedule event",
			changes: []change{
				inCI(gateLines, "    if: github.event_name == 'schedule'\n"),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:21: job test's if: depends on the schedule; " +
					"list it in SCHEDULED_JOBS as test:<mode>",
			},
		},
		{
			name: "a job in PULL_REQUEST_JOBS",
			changes: []change{
				inCI(`PULL_REQUEST_JOBS: ""`, "PULL_REQUEST_JOBS: test"),
				inCI(gateLines,
					"    if: \"!inputs.mode && github.event_name == 'pull_request'\"\n"),
			},
			wantExit: exitPassed,
		},
		{
			name:     "a job in PULL_REQUEST_JOBS with the if: of the pull request checks",
			changes:  []change{inCI(`PULL_REQUEST_JOBS: ""`, "PULL_REQUEST_JOBS: test")},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:21: job test is in PULL_REQUEST_JOBS, so its " +
					"if: must be !inputs.mode && github.event_name == 'pull_request'\n",
			},
		},
		{
			name: "a tag-pinned action",
			changes: []change{
				inCI("actions/checkout@"+checkoutSHA+"  # v7.0.1", "actions/checkout@v7"),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:33: uses: actions/checkout@v7 is not pinned " +
					"to a 40-character commit SHA\n",
			},
		},
		{
			name:       "a short SHA",
			changes:    []change{inCI("actions/checkout@"+checkoutSHA, "actions/checkout@3d3c42e")},
			wantExit:   exitFailed,
			wantStdout: []string{"uses: actions/checkout@3d3c42e is not pinned"},
		},
		{
			name:     "a pin without a version comment",
			changes:  []change{inCI(checkoutSHA+"  # v7.0.1", checkoutSHA)},
			wantExit: exitFailed,
			wantStdout: []string{
				"uses: actions/checkout@" + checkoutSHA + " has no version comment",
			},
		},
		{
			name:       "a pin whose comment is not a version",
			changes:    []change{inCI(checkoutSHA+"  # v7.0.1", checkoutSHA+"  # latest")},
			wantExit:   exitFailed,
			wantStdout: []string{"has no version comment such as # v1.2.3"},
		},
		{
			name: "a local action",
			changes: []change{inCI("      - name: Test\n",
				"      - uses: ./.github/actions/setup\n      - name: Test\n")},
			wantExit: exitPassed,
		},
		{
			name: "a job that calls a reusable workflow by tag",
			changes: []change{{
				file: release,
				to: "on: push\nconcurrency: release-x\njobs:\n  tests:\n" +
					"    uses: ./.github/workflows/ci.yml\n" +
					"  other:\n    uses: octo/repo/.github/workflows/w.yml@main\n",
			}},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/release.yml:7: uses: " +
					"octo/repo/.github/workflows/w.yml@main is not pinned",
			},
		},
		{
			name:       "a checkout that persists credentials",
			changes:    []change{inCI("persist-credentials: false", "persist-credentials: true")},
			wantExit:   exitFailed,
			wantStdout: []string{"actions/checkout needs persist-credentials: false"},
		},
		{
			name: "a checkout without persist-credentials",
			changes: []change{
				inCI("        with:\n          persist-credentials: false\n", ""),
			},
			wantExit:   exitFailed,
			wantStdout: []string{"actions/checkout needs persist-credentials: false"},
		},
		{
			name: "a checkout whose with: is a YAML alias",
			changes: []change{
				inCI("        with:\n          persist-credentials: false\n",
					"        with: &no-credentials\n          persist-credentials: false\n"),
				inCI("      - run: |\n", "      - uses: actions/checkout@"+checkoutSHA+
					"  # v7.0.1\n        with: *no-credentials\n      - run: |\n"),
			},
			wantExit:   exitPassed,
			wantStdout: []string{"cicheck: checked 1 workflows, 3 jobs, 7 steps and 2 scripts\n"},
		},
		{
			name: "a job on ubuntu-latest",
			changes: []change{
				inCI("    runs-on: ubuntu-24.04\n    timeout-minutes: 10\n    strategy",
					"    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    strategy"),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:21: job test runs on \"ubuntu-latest\"; use " +
					"ubuntu-24.04\n",
			},
		},
		{
			name:       "a job without a timeout",
			changes:    []change{inCI("    timeout-minutes: 5\n", "")},
			wantExit:   exitFailed,
			wantStdout: []string{"job ci has no timeout-minutes"},
		},
		{
			name:       "a job with a timeout of zero",
			changes:    []change{inCI("timeout-minutes: 5", "timeout-minutes: 0")},
			wantExit:   exitFailed,
			wantStdout: []string{"job ci has no timeout-minutes"},
		},
		{
			name: "${{ github.event.pull_request.title }} in run:",
			changes: []change{
				inCI("go test ./...", "echo \"${{ github.event.pull_request.title }}\""),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:43: run: contains ${{; pass the value " +
					"through env: and use the variable\n",
			},
		},
		{
			name:     "a scanner step without stop-commands",
			changes:  []change{inCI(ciRun, ciRun+"      - run: gitleaks git --redact\n")},
			wantExit: exitFailed,
			wantStdout: []string{
				"run: does not start with the three lines that stop workflow commands and does " +
					"more than call scripts in .github/scripts, so tool output could issue " +
					"workflow commands; start it with token=$(od -An -N16 -tx1 /dev/urandom | " +
					"tr -d ' \\n'), echo \"::stop-commands::$token\" and " +
					"trap 'echo \"::$token::\"' EXIT\n",
			},
		},
		{
			name:     "a run: that ends in a backslash",
			changes:  []change{inCI(ciRun, ciRun+"      - run: gitleaks git \\\n")},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:75: " + notWrapped,
			},
		},
		{
			name: "stop-commands with a fixed token",
			changes: []change{
				inCI(`echo "::stop-commands::$token"`, `echo "::stop-commands::fixed"`),
			},
			wantExit:   exitFailed,
			wantStdout: []string{"cicheck: .github/workflows/ci.yml:43: " + notWrapped},
		},
		{
			name:       "a fixed token in the variable",
			changes:    []change{inCI(tokenLine, "          token=abc\n")},
			wantExit:   exitFailed,
			wantStdout: []string{"cicheck: .github/workflows/ci.yml:43: " + notWrapped},
		},
		{
			name: "a token read from another device",
			changes: []change{
				inCI(tokenLine, "          token=$(od -An -N16 -tx1 /dev/zero | tr -d ' \\n')\n"),
			},
			wantExit:   exitFailed,
			wantStdout: []string{notWrapped},
		},
		{
			name: "the stop-commands line in a comment",
			changes: []change{
				inCI(stopLine, "          # echo \"::stop-commands::$token\"\n"),
			},
			wantExit:   exitFailed,
			wantStdout: []string{notWrapped},
		},
		{
			name:       "the stop-commands line after another command",
			changes:    []change{inCI(tokenLine, tokenLine+"          go vet ./...\n")},
			wantExit:   exitFailed,
			wantStdout: []string{notWrapped},
		},
		{
			name:       "the stop-commands line with another variable",
			changes:    []change{inCI(stopLine, "          echo \"::stop-commands::$other\"\n")},
			wantExit:   exitFailed,
			wantStdout: []string{notWrapped},
		},
		{
			name:       "no trap that resumes the commands",
			changes:    []change{inCI(trapLine, "")},
			wantExit:   exitFailed,
			wantStdout: []string{notWrapped},
		},
		{
			name:       "a run: of only the token and stop-commands lines",
			changes:    []change{inCI(trapLine+"          go test ./...\n", "")},
			wantExit:   exitFailed,
			wantStdout: []string{"cicheck: .github/workflows/ci.yml:43: " + notWrapped},
		},
		{
			name:       "a trap that resumes with another variable",
			changes:    []change{inCI(trapLine, "          trap 'echo \"::$other::\"' EXIT\n")},
			wantExit:   exitFailed,
			wantStdout: []string{notWrapped},
		},
		{
			name:       "a trap on another condition",
			changes:    []change{inCI(trapLine, "          trap 'echo \"::$token::\"' ERR\n")},
			wantExit:   exitFailed,
			wantStdout: []string{notWrapped},
		},
		{
			name: "the three lines with another variable name",
			changes: []change{
				inCI(tokenLine, "          t=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \\n')\n"),
				inCI(stopLine, "          echo \"::stop-commands::$t\"\n"),
				inCI(trapLine, "          trap 'echo \"::$t::\"' EXIT\n"),
			},
			wantExit: exitPassed,
		},
		{
			name:     "a comment before the three lines",
			changes:  []change{inCI(tokenLine, "          # Stop workflow commands.\n"+tokenLine)},
			wantExit: exitPassed,
		},
		{
			name: "a script that does not stop commands",
			changes: []change{
				{file: ".github/scripts/wrap.sh", to: "#!/usr/bin/env bash\ngitleaks git\n"},
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"run: runs .github/scripts/wrap.sh, which prints no ::stop-commands:: line with " +
					"its token in a variable\n",
			},
		},
		{
			name: "a script that stops commands with a fixed token",
			changes: []change{{
				file: ".github/scripts/wrap.sh", from: `echo "::stop-commands::$token"`,
				to: `echo "::stop-commands::fixed"`,
			}},
			wantExit: exitFailed,
			wantStdout: []string{
				"run: runs .github/scripts/wrap.sh, which prints no ::stop-commands:: line",
			},
		},
		{
			name:     "a script that does not exist",
			changes:  []change{inCI(ciRun, "        run: .github/scripts/missing.sh\n")},
			wantExit: exitFailed,
			wantStdout: []string{
				"run: runs .github/scripts/missing.sh, which does not exist",
			},
		},
		{
			name: "no .github/scripts directory",
			changes: []change{
				{file: ".github/scripts/wrap.sh", remove: true},
				{file: ".github/scripts/no-version", remove: true},
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"run: runs .github/scripts/wrap.sh, which does not exist",
				"cicheck: checked 1 workflows, 3 jobs, 6 steps and 0 scripts\n",
			},
		},
		{
			name: "a directory in .github/scripts",
			changes: []change{
				{file: ".github/scripts/lib/helper.sh", to: validScript},
			},
			wantExit:   exitPassed,
			wantStdout: []string{"cicheck: checked 1 workflows, 3 jobs, 6 steps and 2 scripts\n"},
		},
		{
			name: "a script and another command in one run:",
			changes: []change{inCI(ciRun,
				"        run: |\n          .github/scripts/wrap.sh\n          go vet ./...\n")},
			wantExit:   exitFailed,
			wantStdout: []string{notWrapped},
		},
		{
			name: "a script call followed by && and another tool",
			changes: []change{
				{file: ".github/scripts/fetch-tool.sh", to: validScript},
				inCI(ciRun, "        run: .github/scripts/fetch-tool.sh gitleaks && "+
					"bin/gitleaks git\n"),
			},
			wantExit:   exitFailed,
			wantStdout: []string{"cicheck: .github/workflows/ci.yml:74: " + notWrapped},
		},
		{
			name: "a script call with plain and quoted arguments",
			changes: []change{
				inCI(ciRun, "        run: .github/scripts/wrap.sh \"$A\" b 'c d' --e=f\n"),
			},
			wantExit: exitPassed,
		},
		{
			name:     "a fixed GITHUB_OUTPUT delimiter in run:",
			changes:  []change{inCI("go test ./...", `echo "report<<EOF" >>"$GITHUB_OUTPUT"`)},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:43: the multi-line value report uses the " +
					"fixed delimiter \"EOF\"; take a random one from a variable\n",
			},
		},
		{
			name: "a fixed GITHUB_OUTPUT delimiter in a script",
			changes: []change{
				{file: ".github/scripts/wrap.sh", from: "report<<$delimiter", to: "report<<'END'"},
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/scripts/wrap.sh:6: the multi-line value report uses the " +
					"fixed delimiter \"END\"",
			},
		},
		{
			name:     "setup-go without cache: false",
			changes:  []change{inCI(cacheStep, "          go-version-file: go.mod\n")},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:53: actions/setup-go needs cache: false",
			},
		},
		{
			name:     "go-version 1.27.x",
			changes:  []change{inCI(matrixGo, "go-version: '1.27.x'")},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:36: go-version \"1.27.x\" is not an exact " +
					"Go release such as 1.27.1\n",
			},
		},
		{
			name:       "go-version stable",
			changes:    []change{inCI(matrixGo, "go-version: stable")},
			wantExit:   exitFailed,
			wantStdout: []string{"go-version \"stable\" is not an exact Go release"},
		},
		{
			name:     "go-version as an exact literal",
			changes:  []change{inCI(matrixGo, "go-version: 1.27.1")},
			wantExit: exitPassed,
		},
		{
			name:     "go-version from an env variable",
			changes:  []change{inCI(matrixGo, "go-version: ${{ env.GO_NEW }}")},
			wantExit: exitPassed,
		},
		{
			name:     "an env variable that is not an exact Go release",
			changes:  []change{inCI(`GO_NEW: "1.27.1"`, `GO_NEW: "1.27.x"`)},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:18: GO_NEW = \"1.27.x\" is not an exact Go " +
					"release such as 1.27.1\n",
			},
		},
		{
			name:     "a matrix entry that names no env variable",
			changes:  []change{inCI("- {go: GO_NEW}", "- {go: GO_NEXT}")},
			wantExit: exitFailed,
			wantStdout: []string{
				"go-version ${{ env[matrix.go] }}: the workflow env has no GO_NEXT",
			},
		},
		{
			name: "a matrix list of env variables",
			changes: []change{inCI("        include:\n          - {go: GO_OLD}\n"+
				"          - {go: GO_NEW}\n", "        go: [GO_OLD, GO_NEW]\n")},
			wantExit: exitPassed,
		},
		{
			name: "a matrix list that names no env variable",
			changes: []change{inCI("        include:\n          - {go: GO_OLD}\n"+
				"          - {go: GO_NEW}\n", "        go: [GO_OLD, GO_NEXT]\n")},
			wantExit: exitFailed,
			wantStdout: []string{
				"go-version ${{ env[matrix.go] }}: the workflow env has no GO_NEXT",
			},
		},
		{
			name:     "a go-version that reads a matrix key the job does not have",
			changes:  []change{inCI(matrixGo, "go-version: ${{ env[matrix.version] }}")},
			wantExit: exitFailed,
			wantStdout: []string{
				"go-version ${{ env[matrix.version] }}: the job's matrix has no version",
			},
		},
		{
			name:       "an env variable that does not exist",
			changes:    []change{inCI(matrixGo, "go-version: ${{ env.GO }}")},
			wantExit:   exitFailed,
			wantStdout: []string{"go-version ${{ env.GO }}: the workflow env has no GO"},
		},
		{
			name: "setup-go with check-latest",
			changes: []change{inCI("          go-version-file: go.mod\n",
				"          go-version-file: go.mod\n          check-latest: true\n")},
			wantExit:   exitFailed,
			wantStdout: []string{"actions/setup-go sets check-latest"},
		},
		{
			name: "setup-go with check-latest: false",
			changes: []change{inCI("          go-version-file: go.mod\n",
				"          go-version-file: go.mod\n          check-latest: false\n")},
			wantExit: exitPassed,
		},
		{
			name:     "setup-go without a Go version",
			changes:  []change{inCI("go-version-file: go.mod", "go-version-file: .go-version")},
			wantExit: exitFailed,
			wantStdout: []string{
				"actions/setup-go needs a go-version, or a go-version-file that names a go.mod",
			},
		},
		{
			name:     "setup-go with the go.mod of another module",
			changes:  []change{inCI("go-version-file: go.mod", "go-version-file: tools/go.mod")},
			wantExit: exitPassed,
		},
		{
			name: "a concurrency group that starts with an expression",
			changes: []change{
				inCI(
					"group: ci-${{ github.sha }}",
					"group: ${{ github.workflow }}-${{ github.sha }}",
				),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:15: the concurrency group " +
					"\"${{ github.workflow }}-${{ github.sha }}\" does not start with a literal " +
					"prefix",
			},
		},
		{
			name: "a job's concurrency group with the prefix of its own workflow",
			changes: []change{
				inCI(weeklyRunsOn, "    concurrency: ci-weekly\n"+weeklyRunsOn),
			},
			wantExit: exitPassed,
		},
		{
			name: "a release-like caller with the same concurrency prefix",
			changes: []change{{
				file: release,
				to: "on: push\nconcurrency:\n  group: ci-release-${{ github.sha }}\njobs:\n" +
					"  tests:\n    uses: ./.github/workflows/ci.yml\n",
			}},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/release.yml:3: the concurrency group prefix " +
					"\"ci-release-\" overlaps \"ci-\" in .github/workflows/ci.yml; give each " +
					"workflow its own prefix\n",
			},
		},
		{
			name: "a release with its own concurrency prefix",
			changes: []change{{
				file: release,
				to: "on: push\nconcurrency:\n  group: release-${{ github.sha }}\njobs:\n" +
					"  tests:\n    uses: ./.github/workflows/ci.yml\n",
			}},
			wantExit: exitPassed,
		},
		{
			name: "a job's concurrency group with the CI prefix in another workflow",
			changes: []change{{
				file: release,
				to: "on: push\njobs:\n  tests:\n    concurrency: ci\n" +
					"    uses: ./.github/workflows/ci.yml\n",
			}},
			wantExit:   exitFailed,
			wantStdout: []string{"the concurrency group prefix \"ci\" overlaps \"ci-\""},
		},
		{
			name: "a directory named like a workflow",
			changes: []change{
				{file: ".github/workflows/old.yml/README.md", to: "not a workflow\n"},
			},
			wantExit: exitPassed,
		},
		{
			name: "a branches filter under pull_request",
			changes: []change{
				inCI("  pull_request:\n", "  pull_request:\n    branches: [master]\n"),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:4: pull_request has a branches filter; CI " +
					"must run on every pull request, into any branch\n",
			},
		},
		{
			name: "a branches-ignore filter under pull_request",
			changes: []change{
				inCI("  pull_request:\n", "  pull_request:\n    branches-ignore: [wip/*]\n"),
			},
			wantExit:   exitFailed,
			wantStdout: []string{"pull_request has a branches-ignore filter"},
		},
		{
			name: "a paths filter under pull_request",
			changes: []change{
				inCI("  pull_request:\n", "  pull_request:\n    paths: ['**.go']\n"),
			},
			wantExit:   exitFailed,
			wantStdout: []string{"pull_request has a paths filter"},
		},
		{
			name: "a paths-ignore filter under pull_request",
			changes: []change{
				inCI("  pull_request:\n", "  pull_request:\n    paths-ignore: ['**.md']\n"),
			},
			wantExit:   exitFailed,
			wantStdout: []string{"pull_request has a paths-ignore filter"},
		},
		{
			name:       "pull_request_target",
			changes:    []change{inCI("  pull_request:\n", "  pull_request_target:\n")},
			wantExit:   exitFailed,
			wantStdout: []string{"runs on pull_request_target"},
		},
		{
			name: "pull_request_target in a list of events",
			changes: []change{{
				file: otherYML,
				to: "on: [push, pull_request_target]\njobs:\n  x:\n" +
					"    uses: ./.github/workflows/ci.yml\n",
			}},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/other.yml:1: runs on pull_request_target",
			},
		},
		{
			name: "pull_request_target as the only event",
			changes: []change{{
				file: otherYML,
				to: "on: pull_request_target\njobs:\n  x:\n" +
					"    uses: ./.github/workflows/ci.yml\n",
			}},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/other.yml:1: runs on pull_request_target",
			},
		},
		{
			name:     "a periodic job without a cron and mode condition",
			changes:  []change{inCI(weeklyIf, "if: github.event_name == 'schedule'")},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:49: job weekly is in SCHEDULED_JOBS, so its " +
					"if: must be (!inputs.mode && github.event.schedule == '<WEEKLY_CRON>') || " +
					"inputs.mode == 'weekly'\n",
			},
		},
		{
			name: "a periodic job that a called run selects by its caller's cron",
			changes: []change{inCI(weeklyIf,
				"if: github.event.schedule == '0 1 * * 1' || inputs.mode == 'weekly'")},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:49: job weekly is in SCHEDULED_JOBS, so its " +
					"if: must be (!inputs.mode && github.event.schedule == '<WEEKLY_CRON>')",
			},
		},
		{
			name: "a periodic job with two crons",
			changes: []change{inCI(weeklyIf, "if: (!inputs.mode && "+
				"(github.event.schedule == '0 1 * * 1' || github.event.schedule == '0 2 * * 1')) "+
				"|| inputs.mode == 'weekly'")},
			wantExit:   exitFailed,
			wantStdout: []string{"job weekly is in SCHEDULED_JOBS, so its if: must be"},
		},
		{
			name:     "a periodic job that names another mode",
			changes:  []change{inCI("inputs.mode == 'weekly'", "inputs.mode == 'pr'")},
			wantExit: exitFailed,
			wantStdout: []string{
				"job weekly is in SCHEDULED_JOBS for mode weekly, but its if: names mode pr",
			},
		},
		{
			name: "a periodic job that names a cron its mode does not declare",
			changes: []change{inCI("github.event.schedule == '0 1 * * 1'",
				"github.event.schedule == '0 3 * * 1'")},
			wantExit: exitFailed,
			wantStdout: []string{
				"job weekly's if: names the cron \"0 3 * * 1\", but WEEKLY_CRON is \"0 1 * * 1\"",
			},
		},
		{
			name: "a periodic job in a mode without a cron",
			changes: []change{
				inCI(scheduledJobs, "SCHEDULED_JOBS: weekly:security"),
				inCI("inputs.mode == 'weekly'", "inputs.mode == 'security'"),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"job weekly runs in mode security, but the workflow env has no SECURITY_CRON",
			},
		},
		{
			name:     "a job that depends on the schedule but is not in SCHEDULED_JOBS",
			changes:  []change{inCI(scheduledJobs, "SCHEDULED_JOBS: \"\"")},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:48: job weekly's if: depends on the " +
					"schedule; list it in SCHEDULED_JOBS as weekly:<mode>",
			},
		},
		{
			name:       "a SCHEDULED_JOBS entry that is not job:mode",
			changes:    []change{inCI(scheduledJobs, "SCHEDULED_JOBS: weekly")},
			wantExit:   exitFailed,
			wantStdout: []string{"SCHEDULED_JOBS entry \"weekly\" is not job:mode"},
		},
		{
			name:       "a SCHEDULED_JOBS entry without a job",
			changes:    []change{inCI(scheduledJobs, `SCHEDULED_JOBS: ":weekly"`)},
			wantExit:   exitFailed,
			wantStdout: []string{"SCHEDULED_JOBS entry \":weekly\" is not job:mode"},
		},
		{
			name:       "a SCHEDULED_JOBS entry without a mode",
			changes:    []change{inCI(scheduledJobs, `SCHEDULED_JOBS: "weekly:"`)},
			wantExit:   exitFailed,
			wantStdout: []string{"SCHEDULED_JOBS entry \"weekly:\" is not job:mode"},
		},
		{
			name:       "an ADVISORY_JOBS job that CI does not need",
			changes:    []change{inCI("ADVISORY_JOBS: \"\"", "ADVISORY_JOBS: e2e")},
			wantExit:   exitFailed,
			wantStdout: []string{"ADVISORY_JOBS names e2e, which CI does not need"},
		},
		{
			name:       "a PULL_REQUEST_JOBS job that CI does not need",
			changes:    []change{inCI(`PULL_REQUEST_JOBS: ""`, "PULL_REQUEST_JOBS: lint")},
			wantExit:   exitFailed,
			wantStdout: []string{"PULL_REQUEST_JOBS names lint, which CI does not need"},
		},
		{
			name:     "a cron that the env does not declare",
			changes:  []change{inCI(cron, cron+"    - cron: \"0 0 * * *\"\n")},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/ci.yml:8: on.schedule has the cron \"0 0 * * *\", " +
					"which no <MODE>_CRON variable in the workflow env declares\n",
			},
		},
		{
			name:       "a cron listed twice",
			changes:    []change{inCI(cron, cron+cron)},
			wantExit:   exitFailed,
			wantStdout: []string{"on.schedule lists the cron \"0 1 * * 1\" twice"},
		},
		{
			name:     "a declared cron that is not scheduled",
			changes:  []change{inCI(cronEnv, cronEnv+"  SECURITY_CRON: \"0 2 * * 4\"\n")},
			wantExit: exitFailed,
			wantStdout: []string{
				"env declares SECURITY_CRON = \"0 2 * * 4\", which on.schedule does not list",
			},
		},
		{
			name:       "two env variables that declare one cron",
			changes:    []change{inCI(cronEnv, cronEnv+"  E2E_CRON: \"0 1 * * 1\"\n")},
			wantExit:   exitFailed,
			wantStdout: []string{"env declares the cron \"0 1 * * 1\" as both"},
		},
		{
			name:     "a hard-coded version in a script",
			changes:  []change{{file: ".github/scripts/fetch.sh", to: hardVersion}},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/scripts/fetch.sh:3: hard-codes the version v0.5.5; read " +
					"versions from tools/binaries.lock\n",
			},
		},
		{
			name: "findings in the order of their files and lines",
			changes: []change{
				{file: ".github/scripts/fetch.sh", to: hardVersion},
				inCI(gateEnd, "      || inputs.mode == 'pr'\n"),
				inCI(weeklyRunsOn,
					"    runs-on: ubuntu-latest\n    timeout-minutes: 10\n    steps"),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/scripts/fetch.sh:3: hard-codes the version v0.5.5; read " +
					"versions from tools/binaries.lock\ncicheck: .github/workflows/ci.yml:21: ",
				gateExpr + "\ncicheck: .github/workflows/ci.yml:48: job weekly runs on " +
					"\"ubuntu-latest\"; use ubuntu-24.04\ncicheck: checked",
			},
		},
		{
			name: "a fingerprint-shaped string under .github",
			changes: []change{
				{file: ".github/accept.txt", to: "# accepted\n" + fingerprintLike + "\n"},
			},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/accept.txt:2: holds a string shaped like a gitleaks " +
					"fingerprint (<commit>:<path>:<rule>:<line>); keep accepted findings out of " +
					"the repository\n",
			},
		},
		{
			name: "a workflow with no jobs",
			changes: []change{
				{file: ciYML, remove: true},
				{file: ".github/workflows/empty.yml", to: "on: push\n"},
			},
			wantExit: exitDidNotRun,
			wantStdout: []string{
				"cicheck: checked 1 workflows, 0 jobs, 0 steps and 2 scripts\n" +
					"cicheck: DID NOT RUN: the workflows have no jobs\n",
			},
		},
		{
			name: "a workflow with no jobs and a script that breaks a rule",
			changes: []change{
				{file: ciYML, remove: true},
				{file: ".github/workflows/empty.yml", to: "on: push\n"},
				{file: ".github/scripts/wrap.sh", from: "report<<$delimiter", to: "report<<EOF"},
			},
			wantExit:   exitFailed,
			wantStdout: []string{"uses the fixed delimiter \"EOF\""},
		},
	}
	// A command line that calls a script is a script call only with plain or quoted arguments:
	// a control operator, a pipe, a redirection or a command substitution after the script's
	// path makes it an ordinary command, whose output must be wrapped.
	for _, after := range []string{
		"&& gitleaks git", "|| gitleaks git", "| tee out", "; gitleaks git", "& gitleaks git",
		`"$(gitleaks git)"`, "`gitleaks git`", ">out", "2>&1", "<in",
	} {
		tests = append(tests, checkCase{
			name:       "a script call followed by " + after,
			changes:    []change{inCI(ciRun, "        run: .github/scripts/wrap.sh "+after+"\n")},
			wantExit:   exitFailed,
			wantStdout: []string{"cicheck: .github/workflows/ci.yml:74: " + notWrapped},
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflows := writeRepository(t, withChanges(t, tt.changes))

			exit, stdout, stderr := runCicheck(t, "-workflows", workflows)

			if exit != tt.wantExit {
				t.Errorf("exit = %d, want %d; stdout:\n%s", exit, tt.wantExit, stdout)
			}
			for _, want := range tt.wantStdout {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout does not contain %q:\n%s", want, stdout)
				}
			}
			if strings.Contains(stdout, fingerprintLike) {
				t.Errorf("stdout prints the fingerprint:\n%s", stdout)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want nothing", stderr)
			}
			if !strings.HasSuffix(stdout, verdicts[tt.wantExit]) {
				t.Errorf("stdout does not end with %q:\n%s", verdicts[tt.wantExit], stdout)
			}
		})
	}
}

func TestCheckDidNotRun(t *testing.T) {
	tests := []struct {
		name string
		// files is the repository, and args the arguments after -workflows.
		files      map[string]string
		args       []string
		wantStdout string
	}{
		{
			name:       "no workflows directory",
			files:      map[string]string{"README.md": "x\n"},
			wantStdout: ".github/workflows: no such file or directory\n",
		},
		{
			name:       "no workflow in the directory",
			files:      map[string]string{".github/workflows/README.md": "x\n"},
			wantStdout: "holds no .yml or .yaml workflow\n",
		},
		{
			name:       "a workflow that is not YAML",
			files:      map[string]string{ciYML: "jobs: [\n"},
			wantStdout: "cicheck: DID NOT RUN: .github/workflows/ci.yml: yaml: line ",
		},
		{
			name:  "a workflow whose top level is a list",
			files: map[string]string{ciYML: "- on: push\n"},
			wantStdout: ".github/workflows/ci.yml is not a workflow: its top level is not a " +
				"mapping",
		},
		{
			name:       "an empty workflow",
			files:      map[string]string{ciYML: ""},
			wantStdout: ".github/workflows/ci.yml is not a workflow",
		},
		{
			name:       "an unknown flag",
			files:      validFiles(),
			args:       []string{"-fix"},
			wantStdout: "cicheck: DID NOT RUN: flag provided but not defined: -fix\n",
		},
		{
			name:       "an argument",
			files:      validFiles(),
			args:       []string{"ci.yml"},
			wantStdout: "cicheck: DID NOT RUN: unexpected arguments [\"ci.yml\"]\n",
		},
		{
			name:       "both print flags",
			files:      validFiles(),
			args:       []string{"-print-pins", "-print-allowed-actions"},
			wantStdout: "-print-pins and -print-allowed-actions exclude each other",
		},
		{
			name:       "-include-ref without -print-allowed-actions",
			files:      validFiles(),
			args:       []string{"-include-ref", "main"},
			wantStdout: "cicheck: DID NOT RUN: -include-ref needs -print-allowed-actions\n",
		},
		{
			name:       "-print-pins without a binaries lock",
			files:      map[string]string{ciYML: validWorkflow},
			args:       []string{"-print-pins"},
			wantStdout: "binaries.lock: no such file or directory",
		},
		{
			name: "-print-pins with a lock entry that has no version",
			files: map[string]string{
				ciYML:                 validWorkflow,
				"tools/binaries.lock": "[fake-tool]\nurl = https://example.com/x\n",
			},
			args:       []string{"-print-pins"},
			wantStdout: "tools/binaries.lock:1: [fake-tool] has no version",
		},
		{
			name: "-print-pins with a lock version outside an entry",
			files: map[string]string{
				ciYML:                 validWorkflow,
				"tools/binaries.lock": "# The lock.\nversion = 9.9.9\n" + validLock,
			},
			args: []string{"-print-pins"},
			wantStdout: "cicheck: DID NOT RUN: tools/binaries.lock:2: version is set outside an " +
				"entry; put it after the [name] line of its entry\n",
		},
		{
			name: "-print-pins with an empty lock",
			files: map[string]string{
				ciYML:                 validWorkflow,
				"tools/binaries.lock": "# nothing\n",
			},
			args:       []string{"-print-pins"},
			wantStdout: "tools/binaries.lock has no entries",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflows := writeRepository(t, tt.files)
			t.Chdir(filepath.Dir(filepath.Dir(workflows)))
			args := append([]string{"-workflows", filepath.Join(".github", "workflows")},
				tt.args...)

			exit, stdout, _ := runCicheck(t, args...)

			if exit != exitDidNotRun {
				t.Errorf("exit = %d, want %d; stdout:\n%s", exit, exitDidNotRun, stdout)
			}
			if !strings.Contains(stdout, tt.wantStdout) {
				t.Errorf("stdout does not contain %q:\n%s", tt.wantStdout, stdout)
			}
			if !strings.HasSuffix(stdout, verdicts[exitDidNotRun]) {
				t.Errorf("stdout does not end with the verdict:\n%s", stdout)
			}
		})
	}
}

func TestPrintPins(t *testing.T) {
	files := validFiles()
	files[".github/workflows/third-party.yml"] = thirdParty
	workflows := writeRepository(t, files)

	exit, stdout, _ := runCicheck(t, "-workflows", workflows, "-print-pins")

	want := "action actions/checkout v7.0.1 " + checkoutSHA + " .github/workflows/ci.yml:33\n" +
		"action actions/setup-go v7.0.0 " + setupGoSHA + " .github/workflows/ci.yml:36\n" +
		"action actions/setup-go v7.0.0 " + setupGoSHA + " .github/workflows/ci.yml:53\n" +
		"action octo/repo/.github/workflows/w.yml v1.0.0 " + checkoutSHA +
		" .github/workflows/third-party.yml:4\n" +
		"action octo/tools/setup v2.0.0 " + setupGoSHA + " .github/workflows/third-party.yml:10\n" +
		"action octo/b v1.1.0 " + setupGoSHA + " .github/workflows/third-party.yml:11\n" +
		"action octo/a (no version comment) " + checkoutSHA +
		" .github/workflows/third-party.yml:12\n" +
		"binary fake-tool 1.2.3 tools/binaries.lock:2\n"
	if exit != exitPassed || stdout != want {
		t.Errorf("exit = %d, stdout:\n%s\nwant exit 0, stdout:\n%s", exit, stdout, want)
	}
}

// gitIn runs git in dir with a fixed author and no user or system configuration.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

func TestPrintAllowedActions(t *testing.T) {
	// The valid repository, with a workflow that uses third-party actions, is a git repository
	// with one commit on main. A branch v1 adds a workflow with a third-party action, used
	// twice, and a GitHub-owned one; a branch empty removes every workflow.
	const scan = "on: push\njobs:\n  scan:\n    runs-on: ubuntu-24.04\n    steps:\n" +
		"      - uses: octo/scan@" + checkoutSHA + "  # v1.0.0\n" +
		"      - uses: octo/scan@" + setupGoSHA + "  # v1.1.0\n" +
		"      - uses: github/codeql-action/init@" + setupGoSHA + "  # v4.0.0\n"
	files := validFiles()
	files[".github/workflows/third-party.yml"] = thirdParty
	workflows := writeRepository(t, files)
	root := filepath.Dir(filepath.Dir(workflows))
	gitIn(t, root, "init", "--quiet", "--initial-branch=main")
	gitIn(t, root, "add", "--all")
	gitIn(t, root, "commit", "--quiet", "-m", "valid")
	gitIn(t, root, "checkout", "--quiet", "-b", "v1")
	if err := os.WriteFile(filepath.Join(workflows, "scan.yaml"), []byte(scan),
		0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "add", "--all")
	gitIn(t, root, "commit", "--quiet", "-m", "scan")
	gitIn(t, root, "checkout", "--quiet", "-b", "empty", "main")
	gitIn(t, root, "rm", "--quiet", ".github/workflows/ci.yml",
		".github/workflows/third-party.yml")
	gitIn(t, root, "commit", "--quiet", "-m", "no workflows")
	gitIn(t, root, "checkout", "--quiet", "main")
	thirdPartyPatterns := []string{
		"octo/a@*", "octo/b@*", "octo/repo/.github/workflows/w.yml@*", "octo/tools/setup@*",
	}

	tests := []struct {
		name         string
		args         []string
		wantExit     int
		wantPatterns []string
		wantStdout   string
	}{
		{name: "the working tree", wantExit: exitPassed, wantPatterns: thirdPartyPatterns},
		{
			name: "the working tree and a branch", args: []string{"-include-ref", "v1"},
			wantExit: exitPassed,
			wantPatterns: []string{
				"octo/a@*", "octo/b@*", "octo/repo/.github/workflows/w.yml@*", "octo/scan@*",
				"octo/tools/setup@*",
			},
		},
		{
			name: "a ref that does not exist", args: []string{"-include-ref", "v9"},
			wantExit: exitDidNotRun, wantStdout: "cicheck: DID NOT RUN: git ls-tree",
		},
		{
			name: "a ref without workflows", args: []string{"-include-ref", "empty"},
			wantExit: exitDidNotRun,
			wantStdout: "cicheck: DID NOT RUN: empty holds no .yml or .yaml workflow in " +
				".github/workflows\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"-workflows", workflows, "-print-allowed-actions"},
				tt.args...)

			exit, stdout, _ := runCicheck(t, args...)

			if exit != tt.wantExit {
				t.Fatalf("exit = %d, want %d; stdout:\n%s", exit, tt.wantExit, stdout)
			}
			if tt.wantExit != exitPassed {
				if !strings.Contains(stdout, tt.wantStdout) {
					t.Errorf("stdout does not contain %q:\n%s", tt.wantStdout, stdout)
				}
				return
			}
			var body allowedActions
			if err := json.Unmarshal([]byte(stdout), &body); err != nil {
				t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
			}
			if !body.GithubOwnedAllowed || body.VerifiedAllowed ||
				strings.Join(body.PatternsAllowed, " ") != strings.Join(tt.wantPatterns, " ") {
				t.Errorf("body = %+v, want GitHub-owned actions and patterns %q", body,
					tt.wantPatterns)
			}
			if !strings.Contains(stdout, `"patterns_allowed": [`) {
				t.Errorf("stdout has no patterns_allowed list:\n%s", stdout)
			}
		})
	}
}

// TestRepository checks this repository's workflows and scripts, as the prek job in CI does.
func TestRepository(t *testing.T) {
	workflows := filepath.Join("..", "..", "..", ".github", "workflows")

	exit, stdout, _ := runCicheck(t, "-workflows", workflows)

	if exit != exitPassed {
		t.Errorf("exit = %d, want %d; stdout:\n%s", exit, exitPassed, stdout)
	}
}
