package main

import (
	"fmt"
	"path/filepath"
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

// e2eIfLines is the if: of an end-to-end job, as a job of ci.yml writes it.
const e2eIfLines = "    if: >-\n" +
	"      (!inputs.mode && github.event_name == 'push') || inputs.mode == 'e2e'\n" +
	"      || inputs.mode == 'release'\n"

// containerPDPEnv gives the step that runs the end-to-end tests of a leg of the PDP container the
// URL of that container, and goTestLine runs the tests of the job's leg.
const (
	containerPDPEnv = "        env:\n          PERMIT_PDP_URL: http://127.0.0.1:7766\n"
	goTestLine      = "          go -C tools test -json -tags e2e ./e2e -args -e2e.leg \"$LEG\" " +
		">e2e.json\n"
)

// e2eJob is an end-to-end job that follows every rule, for the end of ci.yml.
const e2eJob = "  e2e:\n" + e2eIfLines +
	"    runs-on: ubuntu-24.04\n    timeout-minutes: 30\n    environment: integration\n" +
	"    env:\n      LEG: pinned\n" +
	"    steps:\n      - name: Create the scratch environment\n        env:\n" +
	"          PERMIT_E2E_PROJECT_KEY: ${{ secrets.PERMIT_E2E_PROJECT_KEY }}\n" +
	"          PERMIT_E2E_PROJECT: ${{ vars.PERMIT_PROJECT }}\n" +
	"        run: .github/scripts/e2e-env.sh create pinned\n" +
	"      - name: Start the PDP\n        id: pdp\n        env:\n" +
	"          PDP_IMAGE: " + pinnedPDP + "\n" +
	"        run: .github/scripts/pdp-up.sh up\n" +
	"      - name: Run the end-to-end tests\n" + containerPDPEnv + "        run: |\n" +
	"          token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \\n')\n" +
	"          echo \"::stop-commands::$token\"\n" +
	"          trap 'echo \"::$token::\"' EXIT\n" + goTestLine +
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
				"cicheck: " + ciE2E + ":76: job e2e is in E2E_JOBS with the leg pinned, so a " +
					"step of it runs checkgotest with -require-file tools/e2e/required.txt, " +
					"which fails a run that did not run and pass each test of that file\n",
			},
		},
		{
			name: "an e2e job that checks another list of required tests",
			changes: withE2E("-require-file tools/e2e/required.txt",
				"-require-file .github/required-tests.txt"),
			wantExit:   exitFailed,
			wantStdout: []string{"job e2e is in E2E_JOBS with the leg pinned, so a step of it"},
		},
		{
			name: "an e2e job whose checkgotest is only built",
			changes: withE2E("\"$RUNNER_TEMP/checkgotest\" -require-file tools/e2e/required.txt",
				"go -C tools build -o \"$RUNNER_TEMP/checkgotest\" ./cmd/checkgotest && echo "+
					"-require-file tools/e2e/required.txt"),
			wantExit:   exitFailed,
			wantStdout: []string{"job e2e is in E2E_JOBS with the leg pinned, so a step of it"},
		},
		{
			name:     "the e2e job of the leg latest",
			changes:  withE2E("      LEG: pinned\n", "      LEG: latest\n"),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":76: job e2e is the end-to-end job with the pinned PDP, " +
					"which a push to master and a release require, so its env: sets LEG to " +
					"pinned, not latest\n",
			},
		},
		{
			name:     "an e2e job that starts a PDP without a digest",
			changes:  withE2E(pinnedPDP, "permitio/pdp-v2:0.9.16"),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":94: job e2e starts the PDP image permitio/pdp-v2:0.9.16, " +
					"which is not pinned by digest; a required end-to-end job runs " +
					"<image>@sha256:<64 hex digits>\n",
			},
		},
		{
			name:     "an e2e job that starts a PDP without an image",
			changes:  withE2E("        env:\n          PDP_IMAGE: "+pinnedPDP+"\n", ""),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":91: job e2e starts the PDP without a PDP_IMAGE; a " +
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
				"cicheck: " + ciE2E + ":113: job e2e deletes its scratch environment in a step " +
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
				"cicheck: " + ciE2E + ":76: job e2e starts the PDP container, so a step after " +
					"the one that starts it runs specsnap -check-pdp with the if: always() && " +
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
			wantStdout: []string{"job e2e starts the PDP container, so a step after the one"},
		},
		{
			name: "an e2e job that checks the PDP's spec only when the tests pass",
			changes: withE2E("if: always() && steps.pdp.outcome",
				"if: steps.pdp.outcome"),
			wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":106: job e2e checks the PDP's spec in a step whose if: " +
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
				"cicheck: " + ciE2E + ":105: job e2e checks the PDP's spec in a step whose if: " +
					"is not always() && steps.<id>.outcome == 'success', where <id> is the id: " +
					"of the step that starts the PDP, \"\", so",
			},
		},
		{
			name:    "an e2e job reading the secret outside the integration environment",
			changes: withE2E("    environment: integration\n", ""), wantExit: exitFailed,
			wantStdout: []string{
				"cicheck: " + ciE2E + ":87: job e2e reads the secret PERMIT_E2E_PROJECT_KEY " +
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
				"cicheck: " + ciE2E + ":89: job e2e runs in the integration environment and " +
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

// stopRun is the start of a run: that stops workflow commands, in a step of a job of ci.yml.
const stopRun = "          token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \\n')\n" +
	"          echo \"::stop-commands::$token\"\n" +
	"          trap 'echo \"::$token::\"' EXIT\n"

// advisoryLegStart is the start of an advisory end-to-end job up to its env: the if: of the
// end-to-end jobs, and the integration environment without a deployment.
const advisoryLegStart = e2eIfLines +
	"    runs-on: ubuntu-24.04\n    timeout-minutes: 30\n" +
	"    environment:\n      name: integration\n      deployment: false\n"

// createStep and deleteStep create and delete the scratch environment of a job's leg.
const (
	createStep = "      - name: Create the scratch environment\n        env:\n" +
		"          PERMIT_E2E_PROJECT_KEY: ${{ secrets.PERMIT_E2E_PROJECT_KEY }}\n" +
		"        run: .github/scripts/e2e-env.sh create \"$LEG\"\n"
	deleteStep = "      - name: Delete the scratch environment\n        if: always()\n" +
		"        run: .github/scripts/e2e-env.sh delete \"$LEG\"\n"
)

// uploadStep uploads the request record of an end-to-end job.
const uploadStep = "      - name: Upload the request record\n" +
	"        uses: actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9  # v7.0.2\n" +
	"        with:\n          name: e2e-record\n          path: e2e-record.jsonl\n"

// e2eLatestJob is the advisory leg latest, which follows every rule: the newest release of the
// PDP container by tag, its tests run against it and checked against tools/e2e/required.txt, and
// its spec checked.
const e2eLatestJob = "  e2e-latest:\n" + advisoryLegStart +
	"    env:\n      LEG: latest\n    steps:\n" + createStep +
	"      - name: Start the PDP\n        id: pdp\n        env:\n" +
	"          PDP_IMAGE: permitio/pdp-v2:latest\n" +
	"        run: .github/scripts/pdp-up.sh up\n" +
	"      - name: Run the end-to-end tests\n" + containerPDPEnv + "        run: |\n" + stopRun +
	goTestLine +
	"          \"$RUNNER_TEMP/checkgotest\" -require-file tools/e2e/required.txt e2e.json\n" +
	specStep + deleteStep

// cloudPDPEnv gives the step that runs the end-to-end tests of the leg cloudpdp the URL of the
// cloud PDP, the origin of the pdp-cloud url of validSpecLock.
const cloudPDPEnv = "        env:\n          PERMIT_PDP_URL: https://cloudpdp.api.permit.io\n"

// e2eCloudJob is the advisory leg cloudpdp, which follows every rule: no PDP container, its
// tests run against the cloud PDP and checked against tools/e2e/required-cloudpdp.txt.
const e2eCloudJob = "  e2e-cloudpdp:\n" + advisoryLegStart +
	"    env:\n      LEG: cloudpdp\n    steps:\n" + createStep +
	"      - name: Run the end-to-end tests\n" + cloudPDPEnv + "        run: |\n" + stopRun +
	goTestLine +
	"          \"$RUNNER_TEMP/checkgotest\" -require-file tools/e2e/required-cloudpdp.txt " +
	"e2e.json\n" + deleteStep

// The ids of the advisory legs' jobs.
const (
	latestJob = "e2e-latest"
	cloudJob  = "e2e-cloudpdp"
)

// withLegs returns the changes that add the e2e job and the advisory legs latest and cloudpdp
// to ci.yml, with from replaced with to in the job job, one of latestJob and cloudJob, or in
// neither when from is empty.
func withLegs(t *testing.T, job, from, to string) []change {
	t.Helper()
	jobs := map[string]string{latestJob: e2eLatestJob, cloudJob: e2eCloudJob}
	if from != "" {
		if !strings.Contains(jobs[job], from) {
			t.Fatalf("job %s has no %q", job, from)
		}
		jobs[job] = strings.Replace(jobs[job], from, to, 1)
	}
	return append(withE2E("", ""),
		inCI("needs: [test, weekly, e2e]",
			"needs: [test, weekly, e2e, e2e-latest, e2e-cloudpdp]"),
		inCI("EXPECTED_JOBS: 3", "EXPECTED_JOBS: 5"),
		inCI("E2E_JOBS: e2e\n", "E2E_JOBS: e2e e2e-latest e2e-cloudpdp\n"),
		inCI(`ADVISORY_JOBS: ""`, "ADVISORY_JOBS: e2e-latest e2e-cloudpdp"),
		inCI("run: .github/scripts/e2e-env.sh delete pinned\n",
			"run: .github/scripts/e2e-env.sh delete pinned\n"+jobs[latestJob]+jobs[cloudJob]))
}

// TestCheckE2ELegs checks the rules of the legs of the end-to-end jobs: each sets its own leg, one
// of pinned, latest and cloudpdp, runs the tests of that leg against the PDP of that leg, checks
// its run against the list of required tests of that leg, deletes its scratch environment under
// always() and reads the secret only in the integration environment; the leg latest starts the
// newest release of the PDP, by tag; the leg cloudpdp starts no PDP; and an advisory leg makes no
// deployment. Each case adds the e2e job and the advisory legs to ci.yml, with from replaced with
// to in the job job, and then makes the changes extra.
func TestCheckE2ELegs(t *testing.T) {
	const (
		at         = "cicheck: .github/workflows/ci.yml:"
		cloudList  = "-require-file tools/e2e/required-cloudpdp.txt "
		noDeploy   = "    environment:\n      name: integration\n      deployment: false\n"
		latestPDP  = "PDP_IMAGE: permitio/pdp-v2:latest"
		deleteIf   = "        if: always()\n        run: .github/scripts/e2e-env.sh delete"
		deleteNoIf = "        run: .github/scripts/e2e-env.sh delete"
		testStep   = "      - name: Run the end-to-end tests\n"
		legArg     = "-e2e.leg \"$LEG\""
		cloudURL   = "PERMIT_PDP_URL: https://cloudpdp.api.permit.io"
		cloudMsg   = "job e2e-cloudpdp is in E2E_JOBS with the leg cloudpdp, so a step of " +
			"it runs checkgotest with -require-file tools/e2e/required-cloudpdp.txt, which " +
			"fails a run that did not run and pass each test of that file\n"
		legMsg = "is in E2E_JOBS, so its env: sets LEG to its leg, one of pinned, latest and " +
			"cloudpdp: e2e-env.sh names the job's scratch environment by it, and it picks the " +
			"PDP that the tests call and the list of required tests that the job checks\n"
		legArgMsg = "; pass -e2e.leg \"$LEG\", the job's leg, which picks the scenarios of " +
			"the leg's PDP and the prefix of what the run creates\n"
		cloudPDPMsg = "but the clients of the leg cloudpdp call the cloud PDP, at " +
			"https://cloudpdp.api.permit.io, the origin of the url of pdp-cloud in " +
			"tools/specs/lock.json\n"
		lockMsg = "cicheck: DID NOT RUN: tools/specs/lock.json: the url of pdp-cloud, %q, is " +
			"not an absolute URL such as https://cloudpdp.api.permit.io/openapi.json; cicheck " +
			"checks the PERMIT_PDP_URL of the leg cloudpdp against its origin\n"
		latestMsg = "job e2e-latest is the leg latest, so it starts the newest release of the " +
			"PDP by its tag: its PDP_IMAGE is <image>:<tag>, with no digest, and not "
		unpinnedMsg = ", against a PDP that this repository does not pin, so ADVISORY_JOBS " +
			"names it: a new release of the PDP, or a change of the cloud PDP, must not fail CI\n"
		deployMsg = "is in ADVISORY_JOBS, so it uses the integration environment with " +
			"deployment: false and makes no deployment: a pull request into master needs a " +
			"successful deployment to that environment, which a job that may fail must not " +
			"decide\n"
	)
	tests := []struct {
		name          string
		job, from, to string
		extra         []change
		wantExit      int
		wantStdout    []string
	}{
		{
			name: "the e2e job and the advisory legs", wantExit: exitPassed,
			wantStdout: []string{
				"cicheck: checked 2 workflows, 8 jobs, 20 steps and 4 scripts\ncicheck: PASS\n",
			},
		},
		{
			name: "the cloud leg that checks the list of the PDP container",
			job:  cloudJob, from: cloudList, to: "-require-file tools/e2e/required.txt ",
			wantExit: exitFailed, wantStdout: []string{at + "156: " + cloudMsg},
		},
		{
			name: "the cloud leg without a list of required tests",
			job:  cloudJob, from: cloudList, to: "",
			wantExit: exitFailed, wantStdout: []string{at + "156: " + cloudMsg},
		},
		{
			name: "the latest leg that checks the list of the cloud PDP", job: latestJob,
			from: "-require-file tools/e2e/required.txt",
			to:   "-require-file tools/e2e/required-cloudpdp.txt", wantExit: exitFailed,
			wantStdout: []string{
				at + "116: job e2e-latest is in E2E_JOBS with the leg latest, so a step of it " +
					"runs checkgotest with -require-file tools/e2e/required.txt",
			},
		},
		{
			name: "the pinned job that checks the list of the cloud PDP",
			extra: []change{inCI("-require-file tools/e2e/required.txt \\\n",
				"-require-file tools/e2e/required-cloudpdp.txt \\\n")},
			wantExit: exitFailed,
			wantStdout: []string{
				at + "76: job e2e is in E2E_JOBS with the leg pinned, so a step of it runs " +
					"checkgotest with -require-file tools/e2e/required.txt",
			},
		},
		{
			name: "a leg without LEG",
			job:  cloudJob, from: "    env:\n      LEG: cloudpdp\n", to: "",
			wantExit: exitFailed, wantStdout: []string{at + "156: job e2e-cloudpdp " + legMsg},
		},
		{
			name: "a leg from a matrix",
			job:  cloudJob, from: "LEG: cloudpdp", to: "LEG: ${{ matrix.leg }}",
			wantExit: exitFailed, wantStdout: []string{"job e2e-cloudpdp " + legMsg},
		},
		{
			name: "a leg renamed",
			job:  latestJob, from: "LEG: latest", to: "LEG: newest",
			wantExit: exitFailed, wantStdout: []string{at + "116: job e2e-latest " + legMsg},
		},
		{
			name: "a leg of 17 characters",
			job:  cloudJob, from: "LEG: cloudpdp", to: "LEG: abcdefghijklmnopq",
			wantExit: exitFailed, wantStdout: []string{at + "156: job e2e-cloudpdp " + legMsg},
		},
		{
			name: "two jobs of the leg pinned",
			job:  latestJob, from: "LEG: latest", to: "LEG: pinned", wantExit: exitFailed,
			wantStdout: []string{
				at + "116: job e2e-latest sets LEG to pinned, which job e2e sets too; each leg " +
					"needs a scratch environment of its own, as e2e-env.sh names it by the leg\n",
			},
		},
		{
			name: "the cloud leg's list in quotes", wantExit: exitPassed,
			job: cloudJob, from: cloudList,
			to: "-require-file \"tools/e2e/required-cloudpdp.txt\" ",
		},
		{
			name: "a leg that does not run the end-to-end tests",
			job:  cloudJob, from: goTestLine, to: "", wantExit: exitFailed,
			wantStdout: []string{
				at + "156: job e2e-cloudpdp is in E2E_JOBS with the leg cloudpdp, so a step of " +
					"it runs the end-to-end tests, go test on ./e2e, with -e2e.leg \"$LEG\"\n",
			},
		},
		{
			name: "a leg that runs go test on another package",
			job:  cloudJob, from: "./e2e -args", to: "./cmd/checkgotest -args",
			wantExit: exitFailed,
			wantStdout: []string{
				at + "156: job e2e-cloudpdp is in E2E_JOBS with the leg cloudpdp, so a step of " +
					"it runs the end-to-end tests, go test on ./e2e, with -e2e.leg \"$LEG\"\n",
			},
		},
		{
			name: "a leg's tests without -e2e.leg",
			job:  cloudJob, from: " -args " + legArg, to: "", wantExit: exitFailed,
			wantStdout: []string{
				at + "172: job e2e-cloudpdp runs the end-to-end tests without -e2e.leg, so with " +
					"the leg pinned" + legArgMsg,
			},
		},
		{
			name: "a leg's tests with the leg of another job",
			job:  latestJob, from: legArg, to: "-e2e.leg pinned", wantExit: exitFailed,
			wantStdout: []string{
				at + "137: job e2e-latest runs the end-to-end tests with -e2e.leg pinned" +
					legArgMsg,
			},
		},
		{
			name: "a leg's tests with its leg, then another",
			job:  cloudJob, from: legArg, to: legArg + " -e2e.leg pinned", wantExit: exitFailed,
			wantStdout: []string{"job e2e-cloudpdp runs the end-to-end tests with -e2e.leg pinned"},
		},
		{
			name: "a leg's tests with the leg in single quotes",
			job:  latestJob, from: legArg, to: "-e2e.leg '$LEG'", wantExit: exitFailed,
			wantStdout: []string{"job e2e-latest runs the end-to-end tests with -e2e.leg '$LEG';"},
		},
		{
			name: "a leg's tests with its leg by name", wantExit: exitPassed,
			job: cloudJob, from: legArg, to: "-e2e.leg=cloudpdp",
		},
		{
			name: "the cloud leg calls the PDP container",
			job:  cloudJob, from: cloudURL, to: "PERMIT_PDP_URL: http://127.0.0.1:7766",
			wantExit: exitFailed,
			wantStdout: []string{
				at + "174: job e2e-cloudpdp runs the end-to-end tests with the PERMIT_PDP_URL " +
					"http://127.0.0.1:7766, " + cloudPDPMsg,
			},
		},
		{
			name: "the cloud leg without a PERMIT_PDP_URL",
			job:  cloudJob, from: cloudPDPEnv, to: "", wantExit: exitFailed,
			wantStdout: []string{
				at + "172: job e2e-cloudpdp runs the end-to-end tests without a PERMIT_PDP_URL, " +
					"so the SDK's clients would call its default, http://localhost:7766, " +
					cloudPDPMsg,
			},
		},
		{
			name: "the latest leg calls the cloud PDP",
			job:  latestJob, from: "PERMIT_PDP_URL: http://127.0.0.1:7766", to: cloudURL,
			wantExit: exitFailed,
			wantStdout: []string{
				at + "139: job e2e-latest runs the end-to-end tests with the PERMIT_PDP_URL " +
					"https://cloudpdp.api.permit.io, but the clients of the leg latest call the " +
					"PDP container that pdp-up.sh starts, at http://127.0.0.1:7766\n",
			},
		},
		{
			name: "the cloud leg and a lock of another cloud PDP",
			extra: []change{{
				file: specLock, to: `{"pdp-cloud": {"url": "https://pdp.example.com/spec.json"}}`,
			}},
			wantExit: exitFailed,
			wantStdout: []string{
				at + "174: job e2e-cloudpdp runs the end-to-end tests with the PERMIT_PDP_URL " +
					"https://cloudpdp.api.permit.io, but the clients of the leg cloudpdp call " +
					"the cloud PDP, at https://pdp.example.com, the origin",
			},
		},
		{
			name: "the cloud leg starts the PDP container",
			job:  cloudJob, from: testStep, wantExit: exitFailed,
			to: "      - name: Start the PDP\n        id: pdp\n        env:\n" +
				"          " + latestPDP + "\n        run: .github/scripts/pdp-up.sh up\n" +
				testStep,
			wantStdout: []string{
				at + "172: job e2e-cloudpdp runs the leg cloudpdp, whose clients call the cloud " +
					"PDP, so it starts no PDP container with pdp-up.sh up\n",
			},
		},
		{
			name:       "no lock of the spec snapshots",
			extra:      []change{{file: specLock, remove: true}},
			wantExit:   exitDidNotRun,
			wantStdout: []string{filepath.FromSlash(specLock) + ": no such file or directory\n"},
		},
		{
			name:       "a lock of the spec snapshots that is not JSON",
			extra:      []change{{file: specLock, to: "{\n"}},
			wantExit:   exitDidNotRun,
			wantStdout: []string{"cicheck: DID NOT RUN: tools/specs/lock.json: unexpected end"},
		},
		{
			name: "a lock without the cloud PDP",
			extra: []change{{
				file: specLock, to: `{"pdp-container": {"image": "permitio/pdp-v2:0.9.16"}}`,
			}},
			wantExit:   exitDidNotRun,
			wantStdout: []string{fmt.Sprintf(lockMsg, "")},
		},
		{
			name: "a lock with a relative URL of the cloud PDP",
			extra: []change{{
				file: specLock, to: `{"pdp-cloud": {"url": "cloudpdp.api.permit.io/openapi.json"}}`,
			}},
			wantExit:   exitDidNotRun,
			wantStdout: []string{fmt.Sprintf(lockMsg, "cloudpdp.api.permit.io/openapi.json")},
		},
		{
			name: "the cloud leg reading the secret outside the integration environment",
			job:  cloudJob, from: noDeploy, to: "", wantExit: exitFailed,
			wantStdout: []string{
				at + "167: job e2e-cloudpdp reads the secret PERMIT_E2E_PROJECT_KEY outside the " +
					"integration environment",
			},
		},
		{
			name: "the cloud leg deletes its environment without always()",
			job:  cloudJob, from: deleteIf, to: deleteNoIf, wantExit: exitFailed,
			wantStdout: []string{
				at + "181: job e2e-cloudpdp deletes its scratch environment in a step whose " +
					"if: is not always(), so a failed or cancelled step before it would leave " +
					"the environment behind\n",
			},
		},
		{
			name: "the latest leg deletes its environment only on success",
			job:  latestJob, from: deleteIf, to: "        if: success()\n" + deleteNoIf,
			wantExit:   exitFailed,
			wantStdout: []string{"job e2e-latest deletes its scratch environment in a step"},
		},
		{
			name: "the latest leg pinned by digest",
			job:  latestJob, from: latestPDP, to: "PDP_IMAGE: " + pinnedPDP,
			wantExit: exitFailed, wantStdout: []string{at + "135: " + latestMsg + pinnedPDP + "\n"},
		},
		{
			name: "the latest leg without a tag",
			job:  latestJob, from: latestPDP, to: "PDP_IMAGE: permitio/pdp-v2",
			wantExit: exitFailed, wantStdout: []string{latestMsg + "permitio/pdp-v2\n"},
		},
		{
			name: "the latest leg without a PDP_IMAGE",
			job:  latestJob, from: "        env:\n          " + latestPDP + "\n", to: "",
			wantExit: exitFailed,
			wantStdout: []string{
				at + "132: job e2e-latest starts the PDP without a PDP_IMAGE; the leg latest " +
					"runs <image>:<tag>, the newest release, with no digest\n",
			},
		},
		{
			name: "the latest leg's image from the workflow env", wantExit: exitPassed,
			job: latestJob, from: latestPDP, to: "PDP_IMAGE: ${{ env.PDP_IMAGE_LATEST }}",
			extra: []change{inCI("  WEEKLY_CRON: \"0 1 * * 1\"\n",
				"  WEEKLY_CRON: \"0 1 * * 1\"\n  PDP_IMAGE_LATEST: permitio/pdp-v2:latest\n")},
		},
		{
			name: "the latest leg's image from a variable the env does not set",
			job:  latestJob, from: latestPDP, to: "PDP_IMAGE: ${{ env.PDP_IMAGE_LATEST }}",
			wantExit:   exitFailed,
			wantStdout: []string{latestMsg + "${{ env.PDP_IMAGE_LATEST }}\n"},
		},
		{
			name: "the latest leg outside ADVISORY_JOBS",
			extra: []change{
				inCI("ADVISORY_JOBS: e2e-latest e2e-cloudpdp", "ADVISORY_JOBS: e2e-cloudpdp"),
			},
			wantExit: exitFailed,
			wantStdout: []string{
				at + "116: job e2e-latest runs the leg latest" + unpinnedMsg,
				at + "135: job e2e-latest starts the PDP image permitio/pdp-v2:latest, which " +
					"is not pinned by digest; a required end-to-end job runs " +
					"<image>@sha256:<64 hex digits>\n",
			},
		},
		{
			name: "the cloud leg outside ADVISORY_JOBS",
			extra: []change{
				inCI("ADVISORY_JOBS: e2e-latest e2e-cloudpdp", "ADVISORY_JOBS: e2e-latest"),
			},
			wantExit:   exitFailed,
			wantStdout: []string{at + "156: job e2e-cloudpdp runs the leg cloudpdp" + unpinnedMsg},
		},
		{
			name: "the latest leg without the spec check",
			job:  latestJob, from: specStep, to: "", wantExit: exitFailed,
			wantStdout: []string{
				at + "116: job e2e-latest starts the PDP container, so a step after the one " +
					"that starts it runs specsnap -check-pdp",
			},
		},
		{
			name: "the latest leg checks the spec only when the tests pass",
			job:  latestJob, from: "if: always() && steps.pdp.outcome", to: "if: steps.pdp.outcome",
			wantExit:   exitFailed,
			wantStdout: []string{"job e2e-latest checks the PDP's spec in a step whose if: is not"},
		},
		{
			name: "the cloud leg uploads its request record",
			job:  cloudJob, from: deleteStep, to: uploadStep + deleteStep, wantExit: exitFailed,
			wantStdout: []string{
				at + "181: job e2e-cloudpdp runs the leg cloudpdp, so it uploads no artifact: " +
					"its request record must stay out of the API coverage, as its one request " +
					"to POST /allowed_url gets a 404 from a PDP that does not serve that route\n",
			},
		},
		{
			name: "the latest leg uploads its request record", wantExit: exitPassed,
			job: latestJob, from: deleteStep, to: uploadStep + deleteStep,
		},
		{
			name: "an advisory leg that makes a deployment",
			job:  cloudJob, from: noDeploy, to: "    environment: integration\n",
			wantExit: exitFailed, wantStdout: []string{at + "156: job e2e-cloudpdp " + deployMsg},
		},
		{
			name: "an advisory leg with deployment: true",
			job:  latestJob, from: "deployment: false", to: "deployment: true",
			wantExit: exitFailed, wantStdout: []string{at + "116: job e2e-latest " + deployMsg},
		},
		{
			name: "an advisory leg whose deployment is an expression", job: latestJob,
			from: "deployment: false", to: "deployment: ${{ github.ref_name == 'master' }}",
			wantExit: exitFailed, wantStdout: []string{"job e2e-latest " + deployMsg},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes := append(withLegs(t, tt.job, tt.from, tt.to), tt.extra...)
			workflows := writeRepository(t, withChanges(t, changes))

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
