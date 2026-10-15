package main

import (
	"strings"
	"testing"
)

// pinnedPDP is a PDP image pinned by digest, a digest of zeros.
const pinnedPDP = "permitio/pdp-v2:0.9.16@sha256:" +
	"0000000000000000000000000000000000000000000000000000000000000000"

// specStep is the step of e2eJob that checks the PDP's spec.
const specStep = "      - name: Check the PDP's spec\n" +
	"        if: always() && steps.pdp.outcome == 'success'\n        run: |\n" +
	"          token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \\n')\n" +
	"          echo \"::stop-commands::$token\"\n" +
	"          trap 'echo \"::$token::\"' EXIT\n" +
	"          \"$RUNNER_TEMP/specsnap\" -check-pdp http://127.0.0.1:7766\n"

// e2eJob is an end-to-end job that follows every rule, for the end of ci.yml.
const e2eJob = "  e2e:\n    if: >-\n" +
	"      (!inputs.mode && github.event_name == 'push') || inputs.mode == 'e2e'\n" +
	"      || inputs.mode == 'release'\n" +
	"    runs-on: ubuntu-24.04\n    timeout-minutes: 30\n    environment: integration\n" +
	"    steps:\n      - name: Create the scratch environment\n        env:\n" +
	"          PERMIT_E2E_PROJECT_KEY: ${{ secrets.PERMIT_E2E_PROJECT_KEY }}\n" +
	"          PERMIT_E2E_PROJECT: ${{ vars.PERMIT_PROJECT }}\n" +
	"        run: .github/scripts/e2e-env.sh create pinned\n" +
	"      - name: Start the PDP\n        id: pdp\n        env:\n" +
	"          PDP_IMAGE: " + pinnedPDP + "\n" +
	"        run: .github/scripts/pdp-up.sh up\n" +
	"      - name: Run the end-to-end tests\n        run: |\n" +
	"          token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \\n')\n" +
	"          echo \"::stop-commands::$token\"\n" +
	"          trap 'echo \"::$token::\"' EXIT\n" +
	"          \"$RUNNER_TEMP/checkgotest\" -require-file tools/e2e/required.txt \\\n" +
	"            -allow-skips tools/e2e/allowed-skips.txt \"$RUNNER_TEMP/e2e.json\"\n" +
	specStep +
	"      - name: Delete the scratch environment\n        if: always()\n" +
	"        run: .github/scripts/e2e-env.sh delete pinned\n"

// e2ePR is the pull request lane of the end-to-end tests, which follows every rule.
const e2ePR = "name: e2e (pull request)\non:\n  pull_request:\npermissions: {}\n" +
	"concurrency:\n  group: e2e-pr-${{ github.event.pull_request.number }}\njobs:\n" +
	"  e2e:\n    if: >-\n" +
	"      github.event.pull_request.head.repo.full_name == github.repository &&\n" +
	"      github.actor != 'dependabot[bot]'\n" +
	"    uses: ./.github/workflows/ci.yml\n    with:\n      mode: e2e\n" +
	"  skipped:\n    if: >-\n" +
	"      github.event.pull_request.head.repo.full_name != github.repository ||\n" +
	"      github.actor == 'dependabot[bot]'\n" +
	"    runs-on: ubuntu-24.04\n    timeout-minutes: 5\n    steps:\n" +
	"      - run: .github/scripts/wrap.sh\n"

// e2ePRFile is the path of the pull request lane.
const e2ePRFile = ".github/workflows/e2e-pr.yml"

// withE2E returns the changes that add the e2e job to ci.yml, with from replaced with to in the
// job, the scripts it runs, and the pull request lane.
func withE2E(from, to string) []change {
	return []change{
		inCI("needs: [test, weekly]", "needs: [test, weekly, e2e]"),
		inCI("EXPECTED_JOBS: 2", "EXPECTED_JOBS: 3"),
		inCI(`ADVISORY_JOBS: ""`, "E2E_JOBS: e2e\n          ADVISORY_JOBS: \"\""),
		{file: ".github/scripts/e2e-env.sh", to: validScript},
		{file: ".github/scripts/pdp-up.sh", to: validScript},
		{file: e2ePRFile, to: e2ePR},
		// The e2e job goes at the end, after CI's step.
		{
			file: ciYML, from: "ADVISORY_JOBS: \"\"\n        run: .github/scripts/wrap.sh\n",
			to: "ADVISORY_JOBS: \"\"\n        run: .github/scripts/wrap.sh\n" +
				strings.Replace(e2eJob, from, to, 1),
		},
	}
}

// TestCheckE2E checks the rules of the end-to-end jobs: where their secrets may be read, which
// runs select them, and the pull request lane outside the CI workflow.
func TestCheckE2E(t *testing.T) {
	const (
		ciE2E      = ".github/workflows/ci.yml"
		secretRead = "${{ secrets.PERMIT_E2E_PROJECT_KEY }}"
		sameRepo   = "github.event.pull_request.head.repo.full_name == github.repository && " +
			"github.actor != 'dependabot[bot]'"
		laneIf = "      github.event.pull_request.head.repo.full_name == github.repository &&\n" +
			"      github.actor != 'dependabot[bot]'\n"
	)
	inLane := func(from, to string) change { return change{file: e2ePRFile, from: from, to: to} }
	tests := []checkCase{
		{
			name: "an e2e job and the pull request lane", changes: withE2E("", ""),
			wantExit: exitPassed,
			wantStdout: []string{
				"cicheck: checked 2 workflows, 6 jobs, 12 steps and 4 scripts\ncicheck: PASS\n",
			},
		},
		{
			name: "an e2e job whose tests do not check the required tests",
			changes: withE2E("checkgotest\" -require-file tools/e2e/required.txt \\\n",
				"checkgotest\" \\\n"),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":76: job e2e is in E2E_JOBS, so a step of it runs " +
					"checkgotest with -require-file tools/e2e/required.txt, which fails a run " +
					"that did not run and pass each test of that file\n",
			},
		},
		{
			name: "an e2e job that checks another list of required tests",
			changes: withE2E("-require-file tools/e2e/required.txt",
				"-require-file .github/required-tests.txt"),
			wantExit:   exitFailed,
			wantStdout: []string{"job e2e is in E2E_JOBS, so a step of it runs checkgotest"},
		},
		{
			name: "an e2e job whose checkgotest is only built",
			changes: withE2E("\"$RUNNER_TEMP/checkgotest\" -require-file tools/e2e/required.txt",
				"go -C tools build -o \"$RUNNER_TEMP/checkgotest\" ./cmd/checkgotest && echo "+
					"-require-file tools/e2e/required.txt"),
			wantExit:   exitFailed,
			wantStdout: []string{"job e2e is in E2E_JOBS, so a step of it runs checkgotest"},
		},
		{
			name:     "an e2e job that starts a PDP without a digest",
			changes:  withE2E(pinnedPDP, "permitio/pdp-v2:0.9.16"),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":92: job e2e starts the PDP image permitio/pdp-v2:0.9.16, " +
					"which is not pinned by digest; a required end-to-end job runs " +
					"<image>@sha256:<64 hex digits>\n",
			},
		},
		{
			name:     "an e2e job that starts a PDP without an image",
			changes:  withE2E("        env:\n          PDP_IMAGE: "+pinnedPDP+"\n", ""),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":89: job e2e starts the PDP without a PDP_IMAGE; a " +
					"required end-to-end job runs <image>@sha256:<64 hex digits>\n",
			},
		},
		{
			name: "an e2e job that starts a PDP pinned in the workflow env", wantExit: exitPassed,
			changes: append(withE2E("PDP_IMAGE: "+pinnedPDP,
				"PDP_IMAGE: ${{ env.PDP_TAG }}@${{env.PDP_DIGEST}}"),
				inCI("  WEEKLY_CRON: \"0 1 * * 1\"\n", "  WEEKLY_CRON: \"0 1 * * 1\"\n"+
					"  PDP_TAG: permitio/pdp-v2:0.9.16\n  PDP_DIGEST: sha256:"+
					strings.Repeat("0", 64)+"\n")),
		},
		{
			name: "an e2e job that starts a PDP of the workflow env without a digest",
			changes: append(withE2E("PDP_IMAGE: "+pinnedPDP, "PDP_IMAGE: ${{ env.PDP_TAG }}"),
				inCI("  WEEKLY_CRON: \"0 1 * * 1\"\n", "  WEEKLY_CRON: \"0 1 * * 1\"\n"+
					"  PDP_TAG: permitio/pdp-v2:0.9.16\n")),
			wantExit: exitFailed,
			wantStdout: []string{
				"job e2e starts the PDP image permitio/pdp-v2:0.9.16, which is not pinned",
			},
		},
		{
			name: "an e2e job that starts a PDP of a variable the env does not set",
			changes: withE2E("PDP_IMAGE: "+pinnedPDP,
				"PDP_IMAGE: ${{ env.PDP_TAG }}@sha256:"+strings.Repeat("0", 64)),
			wantExit:   exitFailed,
			wantStdout: []string{"job e2e starts the PDP image ${{ env.PDP_TAG }}@sha256:"},
		},
		{
			name: "an e2e job that runs pdp-up.sh without a command", wantExit: exitPassed,
			changes: withE2E("run: .github/scripts/pdp-up.sh up", "run: .github/scripts/pdp-up.sh"),
		},
		{
			name: "an advisory e2e job that starts the newest PDP", wantExit: exitPassed,
			changes: append(withE2E("", ""),
				inCI("needs: [test, weekly, e2e]", "needs: [test, weekly, e2e, e2e-latest]"),
				inCI("EXPECTED_JOBS: 3", "EXPECTED_JOBS: 4"),
				inCI("E2E_JOBS: e2e\n", "E2E_JOBS: e2e e2e-latest\n"),
				inCI(`ADVISORY_JOBS: ""`, "ADVISORY_JOBS: e2e-latest"),
				inCI("run: .github/scripts/e2e-env.sh delete pinned\n",
					"run: .github/scripts/e2e-env.sh delete pinned\n"+strings.Replace(
						strings.Replace(e2eJob, "  e2e:", "  e2e-latest:", 1), pinnedPDP,
						"permitio/pdp-v2:latest", 1))),
		},
		{
			name:     "the pinned e2e job in ADVISORY_JOBS",
			changes:  append(withE2E("", ""), inCI(`ADVISORY_JOBS: ""`, "ADVISORY_JOBS: e2e")),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":70: ADVISORY_JOBS names e2e, the end-to-end job with the " +
					"pinned PDP, which a push to master and a release require; only the other " +
					"end-to-end jobs may be advisory\n",
			},
		},
		{
			name: "an e2e job that deletes the environment without always()",
			changes: withE2E("        if: always()\n        run: .github/scripts/e2e-env.sh "+
				"delete", "        run: .github/scripts/e2e-env.sh delete"),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":108: job e2e deletes its scratch environment in a step " +
					"whose if: is not always(), so a failed or cancelled step before it would " +
					"leave the environment behind\n",
			},
		},
		{
			name: "an e2e job that deletes the environment only on success",
			changes: withE2E("        if: always()\n        run: .github/scripts/e2e-env.sh "+
				"delete", "        if: success()\n        run: .github/scripts/e2e-env.sh delete"),
			wantExit:   exitFailed,
			wantStdout: []string{"job e2e deletes its scratch environment in a step whose if:"},
		},
		{
			name: "an e2e job that never deletes the environment",
			changes: withE2E("      - name: Delete the scratch environment\n        if: always()\n"+
				"        run: .github/scripts/e2e-env.sh delete pinned\n", ""),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":76: job e2e creates a scratch environment with " +
					"e2e-env.sh create, so a step of it under if: always() deletes it with " +
					"e2e-env.sh delete\n",
			},
		},
		{
			name:     "an e2e job that does not check the PDP's spec",
			changes:  withE2E(specStep, ""),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":76: job e2e starts the pinned PDP, so a step after the " +
					"one that starts it runs specsnap -check-pdp with the if: always() && " +
					"steps.<id>.outcome == 'success', where <id> is that step's id:, which fails " +
					"when the PDP does not serve the spec of tools/specs/pdp-container.json\n",
			},
		},
		{
			name: "an e2e job that checks the PDP's spec before it starts the PDP",
			changes: append(withE2E("      - name: Start the PDP\n",
				specStep+"      - name: Start the PDP\n"),
				inCI(specStep+"      - name: Delete", "      - name: Delete")),
			wantExit:   exitFailed,
			wantStdout: []string{"job e2e starts the pinned PDP, so a step after the one that"},
		},
		{
			name: "an e2e job that checks the PDP's spec only when the tests pass",
			changes: withE2E("if: always() && steps.pdp.outcome",
				"if: steps.pdp.outcome"),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":101: job e2e checks the PDP's spec in a step whose if: " +
					"is not always() && steps.<id>.outcome == 'success', where <id> is the id: " +
					"of the step that starts the PDP, \"pdp\", so the check would not run when " +
					"the tests fail, or would run without a PDP\n",
			},
		},
		{
			name:     "an e2e job that checks the PDP's spec after another step's outcome",
			changes:  withE2E("steps.pdp.outcome", "steps.tests.outcome"),
			wantExit: exitFailed,
			wantStdout: []string{
				"job e2e checks the PDP's spec in a step whose if: is not always() && " +
					"steps.<id>.outcome == 'success', where <id> is the id: of the step that " +
					"starts the PDP, \"pdp\"",
			},
		},
		{
			name:     "an e2e job that starts the PDP in a step without an id",
			changes:  withE2E("        id: pdp\n", ""),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":100: job e2e checks the PDP's spec in a step whose if: " +
					"is not always() && steps.<id>.outcome == 'success', where <id> is the id: " +
					"of the step that starts the PDP, \"\", so",
			},
		},
		{
			name:    "an e2e job reading the secret outside the integration environment",
			changes: withE2E("    environment: integration\n", ""), wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":85: job e2e reads the secret PERMIT_E2E_PROJECT_KEY " +
					"outside the integration environment; give it environment: integration",
			},
		},
		{
			name:     "an e2e job in another environment",
			changes:  withE2E("environment: integration", "environment: production"),
			wantExit: exitFailed,
			wantStdout: []string{
				"job e2e reads the secret PERMIT_E2E_PROJECT_KEY outside the integration " +
					"environment",
			},
		},
		{
			name: "an e2e job whose environment is a mapping", wantExit: exitPassed,
			changes: withE2E("environment: integration",
				"environment:\n      name: integration\n      url: https://example.com"),
		},
		{
			name: "an e2e job with the if: of the pull request checks",
			changes: withE2E("      (!inputs.mode && github.event_name == 'push') || "+
				"inputs.mode == 'e2e'\n      || inputs.mode == 'release'\n",
				"      (!inputs.mode && (github.event_name == 'pull_request' || "+
					"github.event_name == 'push'))\n      || inputs.mode == 'pr' || "+
					"inputs.mode == 'release'\n"),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":76: job e2e is in E2E_JOBS, so its if: must be " +
					"(!inputs.mode && github.event_name == 'push') || inputs.mode == 'e2e' || " +
					"inputs.mode == 'release'\n",
			},
		},
		{
			name:     "a job in the integration environment outside E2E_JOBS",
			changes:  append(withE2E("", ""), inCI("E2E_JOBS: e2e", `E2E_JOBS: ""`)),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":76: job e2e runs in the integration environment, so " +
					"list it in E2E_JOBS, whose if: keeps it off this workflow's pull requests",
			},
		},
		{
			name:     "E2E_JOBS with a job CI does not need",
			changes:  append(withE2E("", ""), inCI("E2E_JOBS: e2e", "E2E_JOBS: e2e lint")),
			wantExit: exitFailed,
			wantStdout: []string{"cicheck: " + ciE2E + ":70: E2E_JOBS names lint, which CI " +
				"does not need\n"},
		},
		{
			name: "another secret in the integration environment",
			changes: withE2E(secretRead, secretRead+"\n          WEBHOOK: "+
				"${{ secrets.SLACK_WEBHOOK_URL }}"),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":87: job e2e runs in the integration environment and " +
					"reads the secret SLACK_WEBHOOK_URL; read only the secrets PERMIT_E2E_* " +
					"there\n",
			},
		},
		{
			name: "the v1 lane's API key",
			changes: withE2E(secretRead, secretRead+"\n          V1: "+
				"${{ secrets.PERMIT_API_KEY }}"),
			wantExit: exitFailed,
			wantStdout: []string{
				"job e2e reads secrets.PERMIT_API_KEY, which only the v1 lane reads",
			},
		},
		{
			name: "the v1 lane's environment as a variable in another job",
			changes: []change{inCI("GO_VERSION: ${{ env[matrix.go] }}",
				"GO_VERSION: ${{ env[matrix.go] }}\n          ENV: ${{ vars.PERMIT_ENV }}")},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":43: job test reads vars.PERMIT_ENV, which only the v1 " +
					"lane reads",
			},
		},
		{
			name: "the secret in brackets in a job without an environment",
			changes: []change{inCI("GO_VERSION: ${{ env[matrix.go] }}",
				"GO_VERSION: ${{ env[matrix.go] }}\n          KEY: "+
					"${{ secrets[ 'PERMIT_E2E_PROJECT_KEY' ] }}")},
			wantExit: exitFailed,
			wantStdout: []string{
				"job test reads the secret PERMIT_E2E_PROJECT_KEY outside the integration " +
					"environment",
			},
		},
		{
			name: "the secret after another read in one value",
			changes: []change{inCI("GO_VERSION: ${{ env[matrix.go] }}",
				"GO_VERSION: ${{ env[matrix.go] }}\n          KEY: "+
					"${{ vars.PERMIT_PROJECT }}-${{ secrets.PERMIT_E2E_PROJECT_KEY }}")},
			wantExit: exitFailed,
			wantStdout: []string{
				"job test reads the secret PERMIT_E2E_PROJECT_KEY outside the integration " +
					"environment",
			},
		},
		{
			name: "the workflow's env reads the secret of the e2e jobs",
			changes: append(withE2E("", ""), inCI("  WEEKLY_CRON: \"0 1 * * 1\"\n",
				"  WEEKLY_CRON: \"0 1 * * 1\"\n  LEAK: "+secretRead+"\n")),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":20: the workflow's env: reads the secret " +
					"PERMIT_E2E_PROJECT_KEY, which would reach every job of the workflow; read " +
					"it in the env of a step of a job in the integration environment\n",
			},
		},
		{
			name: "the workflow's env reads the v1 lane's environment",
			changes: []change{inCI("  WEEKLY_CRON: \"0 1 * * 1\"\n",
				"  WEEKLY_CRON: \"0 1 * * 1\"\n  V1: ${{ vars.PERMIT_ENV }}\n")},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":20: the workflow's env: reads vars.PERMIT_ENV, which " +
					"only the v1 lane reads\n",
			},
		},
		{
			name: "the workflow's on: reads the v1 lane's API key",
			changes: []change{inCI("        options: [pr, weekly]\n",
				"        options: [pr, weekly]\n        default: ${{ secrets.PERMIT_API_KEY }}\n")},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":13: the workflow's on: reads secrets.PERMIT_API_KEY, " +
					"which only the v1 lane reads\n",
			},
		},
		{
			name: "the pull request lane without the same-repo condition",
			changes: append(withE2E("", ""), inLane(laneIf,
				"      github.actor != 'dependabot[bot]'\n")),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + e2ePRFile + ":8: job e2e of the pull request lane can read " +
					"secrets, so its if: must be " + sameRepo + "; a pull request from a fork, " +
					"or a run that Dependabot triggered, gets no secrets\n",
			},
		},
		{
			name: "the pull request lane calls CI in the mode pr",
			changes: append(withE2E("", ""), inLane("      mode: e2e\n",
				"      mode: pr\n")),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + e2ePRFile + ":8: job e2e of the pull request lane calls " +
					"./.github/workflows/ci.yml with mode: pr; pass mode: e2e, so that the " +
					"called run is the end-to-end jobs\n",
			},
		},
		{
			name: "the pull request lane calls CI without a mode",
			changes: append(withE2E("", ""), inLane("    with:\n      mode: e2e\n",
				"")),
			wantExit: exitFailed,
			wantStdout: []string{
				"job e2e of the pull request lane calls ./.github/workflows/ci.yml with mode: ;",
			},
		},
		{
			name:       "the pull request lane without an if:",
			changes:    append(withE2E("", ""), inLane("    if: >-\n"+laneIf, "")),
			wantExit:   exitFailed,
			wantStdout: []string{"job e2e of the pull request lane can read secrets"},
		},
		{
			name: "a job of the lane in an environment",
			changes: append(withE2E("", ""), inLane("    timeout-minutes: 5\n",
				"    timeout-minutes: 5\n    environment: integration\n")),
			wantExit:   exitFailed,
			wantStdout: []string{"job skipped of the pull request lane can read secrets"},
		},
		{
			name: "a job of the lane that reads a secret",
			changes: append(withE2E("", ""), inLane("      - run: .github/scripts/wrap.sh\n",
				"      - run: .github/scripts/wrap.sh\n        env:\n          TOKEN: "+
					"${{ secrets.ANY }}\n")),
			wantExit:   exitFailed,
			wantStdout: []string{"job skipped of the pull request lane can read secrets"},
		},
		{
			name: "a call that does not grant a permission the called workflow asks for",
			changes: append(withE2E("", ""), inCI("    timeout-minutes: 10\n    strategy:",
				"    timeout-minutes: 10\n    permissions:\n      contents: read\n"+
					"      pull-requests: write\n    strategy:")),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + e2ePRFile + ":8: job e2e calls ./.github/workflows/ci.yml, whose " +
					"job test asks for contents: read, which job e2e does not grant; GitHub " +
					"refuses to start the call\n",
				"job test asks for pull-requests: write, which job e2e does not grant",
			},
		},
		{
			name: "a call that grants less than the called workflow asks for",
			changes: append(withE2E("", ""),
				inCI("    timeout-minutes: 10\n    strategy:", "    timeout-minutes: 10\n"+
					"    permissions:\n      pull-requests: write\n    strategy:"),
				inLane("      mode: e2e\n", "      mode: e2e\n    permissions:\n"+
					"      pull-requests: read\n")),
			wantExit:   exitFailed,
			wantStdout: []string{"job test asks for pull-requests: write, which job e2e does not"},
		},
		{
			name: "a call that grants what the called workflow asks for", wantExit: exitPassed,
			changes: append(withE2E("", ""),
				inCI("    timeout-minutes: 10\n    strategy:", "    timeout-minutes: 10\n"+
					"    permissions:\n      contents: read\n      checks: read\n    strategy:"),
				inLane("      mode: e2e\n", "      mode: e2e\n    permissions:\n"+
					"      contents: write\n      checks: read\n")),
		},
		{
			name: "a workflow that grants everything to the call", wantExit: exitPassed,
			changes: append(withE2E("", ""),
				inCI("    timeout-minutes: 10\n    strategy:", "    timeout-minutes: 10\n"+
					"    permissions:\n      pull-requests: write\n    strategy:"),
				inLane("permissions: {}\n", "permissions: write-all\n")),
		},
		{
			name: "a workflow that grants only reads to the call",
			changes: append(withE2E("", ""),
				inCI("    timeout-minutes: 10\n    strategy:", "    timeout-minutes: 10\n"+
					"    permissions:\n      pull-requests: write\n    strategy:"),
				inLane("permissions: {}\n", "permissions: read-all\n")),
			wantExit:   exitFailed,
			wantStdout: []string{"job test asks for pull-requests: write, which job e2e does not"},
		},
		{
			name: "a call of a workflow that does not exist",
			changes: append(withE2E("", ""), inLane("uses: ./.github/workflows/ci.yml",
				"uses: ./.github/workflows/gone.yml")),
			wantExit: exitFailed,
			wantStdout: []string{
				"job e2e calls ./.github/workflows/gone.yml, but the workflows directory holds " +
					"no such workflow\n",
			},
		},
		{
			name: "another workflow on pull requests",
			changes: []change{{
				file: ".github/workflows/lint.yml",
				to: "on: [pull_request, push]\njobs:\n  lint:\n    runs-on: ubuntu-24.04\n" +
					"    timeout-minutes: 5\n    steps:\n      - run: .github/scripts/wrap.sh\n",
			}},
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: .github/workflows/lint.yml: runs on pull_request outside the CI " +
					"workflow, so outside the check the ruleset requires; check pull requests " +
					"in the CI workflow, or list this workflow in cicheck's pullRequestLanes\n",
			},
		},
		{
			name: "another workflow on pull_request as a mapping key",
			changes: []change{{
				file: ".github/workflows/lint.yml",
				to: "on:\n  pull_request:\n    types: [opened]\njobs:\n  lint:\n" +
					"    runs-on: ubuntu-24.04\n    timeout-minutes: 5\n    steps:\n" +
					"      - run: .github/scripts/wrap.sh\n",
			}},
			wantExit:   exitFailed,
			wantStdout: []string{"lint.yml: runs on pull_request outside the CI workflow"},
		},
		{
			name: "another workflow on a push only", wantExit: exitPassed,
			changes: []change{{
				file: ".github/workflows/lint.yml",
				to: "on: push\njobs:\n  lint:\n    runs-on: ubuntu-24.04\n" +
					"    timeout-minutes: 5\n    steps:\n      - run: .github/scripts/wrap.sh\n",
			}},
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
