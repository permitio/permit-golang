// Command cicheck checks the GitHub Actions workflows in .github/workflows and the scripts in
// .github/scripts against the rules this repository's CI depends on, beyond what actionlint and
// zizmor check, and that Dependabot and the git hooks cover every Go module.
//
// Usage:
//
//	cicheck [-workflows dir]
//	cicheck [-workflows dir] -print-pins
//	cicheck [-workflows dir] -print-allowed-actions [-include-ref ref]...
//
// -workflows names the workflows directory, .github/workflows by default; the repository root is
// the directory two levels above it, and the other paths below are relative to that root.
//
// Without a -print flag, cicheck checks these rules and prints each problem with its file and
// line:
//
//   - One workflow has a job with the id ci and the name CI, the check the ruleset requires. Its
//     if: is always(), its needs lists every other job of that workflow but the jobs that need
//     CI, and a step's env sets EXPECTED_JOBS to the number of jobs it needs. PULL_REQUEST_JOBS,
//     E2E_JOBS and ADVISORY_JOBS name only jobs CI needs, as does SCHEDULED_JOBS, whose entries
//     are job:mode.
//   - A job that needs CI runs after it, to post a notice when CI failed on a push or in a
//     release: its if: is failure() && ((!inputs.mode && github.event_name == 'push') ||
//     inputs.mode == 'release'), and CI does not need it.
//   - A job in E2E_JOBS, an end-to-end job, has the if: (!inputs.mode && github.event_name ==
//     'push') || inputs.mode == 'e2e' || inputs.mode == 'release', which no pull request of the
//     CI workflow matches, and every job of that workflow that runs in the integration
//     environment is in E2E_JOBS. Its env: sets LEG to its leg, one of pinned, latest and
//     cloudpdp, and no other end-to-end job sets the same leg, as e2e-env.sh names the job's
//     scratch environment by it; e2e, the job with the pinned PDP, sets pinned. A command line of
//     a step of it runs go test on ./e2e and passes, in its last -e2e.leg, "$LEG", ${LEG} or the
//     leg itself, and that step's PERMIT_PDP_URL, its own, its job's or its workflow's, is the
//     URL of the leg's PDP: on the leg cloudpdp, the cloud PDP's, the origin of the url of the
//     pdp-cloud entry of tools/specs/lock.json, and on any other leg http://127.0.0.1:7766, the
//     PDP container's. A command line of a step of it starts with checkgotest and passes
//     -require-file with the list of required tests of its leg: tools/e2e/required-cloudpdp.txt
//     for the leg cloudpdp and tools/e2e/required.txt for any other. When a step runs
//     .github/scripts/e2e-env.sh create, a step runs e2e-env.sh delete, and each step that runs
//     it has the if: always(). ADVISORY_JOBS does not name e2e, and names each job of the legs
//     latest and cloudpdp, whose PDP this repository does not pin; in a job that it does not
//     name, each step that runs .github/scripts/pdp-up.sh up has a PDP_IMAGE, its own, its job's
//     or its workflow's, that is <image>@sha256:<64 hex digits> once each ${{ env.NAME }} in it
//     is replaced with the job's or the workflow's value. In a job of the leg latest, the
//     PDP_IMAGE of such a step is <image>:<tag> with no digest: the newest release of the PDP. A
//     job of the leg cloudpdp runs no pdp-up.sh up, and uploads no artifact, so that its request
//     record stays out of the API coverage: its one request to POST /allowed_url gets a 404 from
//     a PDP that does not serve that route. A job that ADVISORY_JOBS names uses its environment
//     with deployment: false, so that it makes no deployment: a pull request into master needs a
//     successful deployment to the integration environment, which a job that may fail must not
//     decide. When a job runs pdp-up.sh up, a step after the first such step runs specsnap
//     -check-pdp with the if: always() && steps.<id>.outcome == 'success', where <id> is the id:
//     of the step that starts the PDP, so the PDP's spec is checked whatever the tests did.
//   - Only a job in the integration environment reads a secret whose name starts with
//     PERMIT_E2E_, and such a job reads no other secret: the environment holds the v1 lane's
//     secrets too. No job reads PERMIT_API_KEY or PERMIT_ENV, the v1 lane's settings, as a secret
//     or a variable. A workflow's top-level env and on: read neither such a secret nor those
//     settings, as every job of the workflow would see them.
//   - A workflow other than the CI one runs on pull_request only when pullRequestLanes lists
//     it, as it is outside the check the ruleset requires: e2e-pr.yml, the pull request lane of
//     the end-to-end tests. Each of its jobs that calls a workflow, runs in an environment or
//     reads a secret has the if: github.event.pull_request.head.repo.full_name ==
//     github.repository && github.actor != 'dependabot[bot]', as GitHub gives a pull request
//     from a fork, or a run that Dependabot triggered, no secrets; and each that calls a
//     workflow passes it mode: e2e.
//   - A job that calls a workflow of this repository, uses: ./.github/workflows/<file>, grants
//     every permission that a job of that workflow asks for, with its own permissions: or its
//     workflow's: GitHub refuses to start the call otherwise, whatever the called job's if:.
//   - A job in SCHEDULED_JOBS runs on exactly one cron and one mode: its if: is
//     (!inputs.mode && github.event.schedule == '<cron>') || inputs.mode == '<mode>', where the
//     workflow's env declares <cron> as <MODE>_CRON. A called run sees its caller's schedule;
//     !inputs.mode keeps the cron from selecting the job in a run that has a mode. A job that is
//     also in ADVISORY_JOBS may run on a push too, with the if:
//     (!inputs.mode && (github.event_name == 'push' || github.event.schedule == '<cron>')) ||
//     inputs.mode == '<mode>'; only such a job, since no pull request runs it before the push.
//   - Every other job of that workflow but CI has the if: of the pull request checks, exactly as
//     ci.yml writes it: (!inputs.mode && (github.event_name == 'pull_request' ||
//     github.event_name == 'push')) || inputs.mode == 'pr' || inputs.mode == 'release'. A job in
//     PULL_REQUEST_JOBS has !inputs.mode && github.event_name == 'pull_request' instead.
//   - The crons in on.schedule are exactly the values of the workflow env's <MODE>_CRON
//     variables, each declared once.
//   - pull_request has no branches, branches-ignore, paths or paths-ignore filter, so every pull
//     request runs CI, and no workflow runs on pull_request_target.
//   - Every uses: outside this repository is pinned to a 40-character commit SHA with a comment
//     that names its version, such as # v7.0.1.
//   - Every actions/checkout step sets persist-credentials: false.
//   - Every actions/setup-go step sets cache: false, and its Go version is an exact patch
//     release such as 1.27.1: a go-version-file that names a go.mod, a literal, or a reference
//     to the workflow env, ${{ env.NAME }} or ${{ env[matrix.KEY] }}, whose values are all
//     exact. check-latest is not set, or is false.
//   - Every job that runs steps runs on ubuntu-24.04 and has a timeout-minutes.
//   - No run: contains ${{; values reach a script through env: only.
//   - Every run: stops workflow commands before it runs anything, as described below, or only
//     calls scripts in .github/scripts that print a ::stop-commands:: line whose token is a
//     variable, so tool output cannot issue workflow commands.
//   - Every multi-line GITHUB_OUTPUT or GITHUB_ENV value, written as name<<delimiter in a run:
//     or a script, takes its delimiter from a variable, so it can be random.
//   - Every concurrency group starts with a literal prefix, and no workflow's prefix starts
//     another workflow's prefix, so a called workflow and its caller never share a group.
//   - No script in .github/scripts hard-codes a version such as 1.2.3 outside a comment line;
//     scripts read versions from tools/binaries.lock.
//   - No file under .github holds a string shaped like a gitleaks fingerprint,
//     <commit>:<path>:<rule>:<line>.
//   - No committed file, one that git ls-files lists, is named trivy.yaml, .trivyignore or
//     .trivyignore.yaml anywhere in the repository: .github/scripts/trivy.sh gives Trivy every
//     setting on its command line, and such a file could weaken a scan that reads it. Files git
//     does not track, such as those of another worktree, are not checked.
//   - Every Go module, a directory with a go.mod that git tracks outside testdata directories,
//     has a gomod entry in .github/dependabot.yml whose directory, or one of whose directories,
//     is / followed by the module's directory, such as /tools, and that has no target-branch, so
//     it updates master. It also has a hook in .pre-commit-config.yaml whose entry is
//     go mod tidy -diff for the root, or go -C <dir> mod tidy -diff. A module in
//     moduleExceptions needs neither and must not have such an entry; each exception names a
//     module and gives the reason. Every such gomod entry names a module.
//   - Every entry of .github/dependabot.yml has a cooldown.default-days of at least 7, and every
//     gomod entry a cooldown.semver-major-days of at least 14; a semver-minor-days or
//     semver-patch-days that is set is at least 7, a semver-major-days at least 14, and no
//     cooldown has an include or exclude list. So Dependabot proposes a release only once it is
//     that many days old.
//
// A run: stops workflow commands when its first three command lines, after any blank and
// comment lines, are these, with one variable name in all three:
//
//	token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
//	echo "::stop-commands::$token"
//	trap 'echo "::$token::"' EXIT
//
// A command line calls a script when it is the path of a script in .github/scripts followed by
// plain or quoted arguments only: a &, |, ;, <, >, backtick or $( after the path makes it an
// ordinary command, so the run: must stop workflow commands itself.
//
// -print-pins prints every uses: outside this repository with its commit SHA and version, and
// every entry of tools/binaries.lock with its version. -print-allowed-actions prints the body of
// GitHub's "set allowed actions" API request: GitHub-owned actions, and a pattern for every other
// action or reusable workflow the workflows use, in the forms GitHub's allow list takes:
// owner/repo@* for an action at the root of its repository, owner/repo/path@* for an action in a
// subdirectory, and owner/repo/.github/workflows/<file>@* for a reusable workflow. -include-ref
// adds the workflows that a git ref holds, such as origin/release/v1, and can be repeated.
//
// Exit codes:
//
//	0  every rule holds, or the -print output was written
//	1  a rule does not hold
//	2  cicheck did not run: the command line is wrong, the workflows directory holds no
//	   workflow or no job, a file cannot be read or parsed, such as .github/dependabot.yml,
//	   .pre-commit-config.yaml or tools/specs/lock.json, the pdp-cloud entry of that lock has no
//	   absolute url, git cannot read a ref or list the committed files, or git tracks no go.mod
//	   at the repository root
//
// The last line cicheck prints when it checks is its verdict: PASS, FAIL or DID NOT RUN. Build it
// before use, because go run reports every non-zero exit as 1:
//
//	go -C tools build -o ../bin/cicheck ./cmd/cicheck
//	bin/cicheck
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	exitPassed    = 0
	exitFailed    = 1
	exitDidNotRun = 2
)

const (
	// ciJob is the id of the aggregate job, and ciName its name: the check the ruleset requires.
	ciJob  = "ci"
	ciName = "CI"
	// runner is the runner image every job that runs steps uses.
	runner = "ubuntu-24.04"
	// lockFile is the lock of the release binaries that scripts download.
	lockFile = "tools/binaries.lock"
	// gateIf is the if: of the pull request checks, the jobs of the CI workflow outside
	// SCHEDULED_JOBS and PULL_REQUEST_JOBS: they run on pull_request and push, and in the modes
	// pr and release.
	gateIf = "(!inputs.mode && (github.event_name == 'pull_request' || " +
		"github.event_name == 'push')) || inputs.mode == 'pr' || inputs.mode == 'release'"
	// pullRequestIf is the if: of a job in PULL_REQUEST_JOBS, which runs on pull_request only.
	pullRequestIf = "!inputs.mode && github.event_name == 'pull_request'"
	// afterCIIf is the if: of a job that needs CI: it runs when CI failed on a push or in a
	// release. A scheduled job posts its own result.
	afterCIIf = "failure() && ((!inputs.mode && github.event_name == 'push') || " +
		"inputs.mode == 'release')"
	// e2eIf is the if: of a job in E2E_JOBS, an end-to-end job: it runs on a push, in the mode
	// e2e and in a release, and never on a pull request of the CI workflow.
	e2eIf = "(!inputs.mode && github.event_name == 'push') || inputs.mode == 'e2e' || " +
		"inputs.mode == 'release'"
	// e2eEnvironment is the GitHub environment of the end-to-end jobs, which holds their
	// secrets, and e2eSecrets starts the name of each of those secrets.
	e2eEnvironment = "integration"
	e2eSecrets     = "PERMIT_E2E_" //nolint:gosec // G101: the start of secrets' names.
	// sameRepoIf is the if: of a job of a pull request lane that reads secrets: it runs only for
	// a pull request from a branch of this repository, in a run that Dependabot did not trigger,
	// as GitHub gives the others no secrets.
	sameRepoIf = "github.event.pull_request.head.repo.full_name == github.repository && " +
		"github.actor != 'dependabot[bot]'"
	// pinnedE2EJob is the end-to-end job with the pinned PDP, which a push to master and a
	// release require.
	pinnedE2EJob = "e2e"
	// containerRequiredTests lists the tests that every run of the end-to-end scenarios against
	// the PDP container must pass, and cloudRequiredTests those of a run of the leg cloudPDPLeg.
	containerRequiredTests = "tools/e2e/required.txt"
	cloudRequiredTests     = "tools/e2e/required-cloudpdp.txt"
	// pinnedLeg is the leg of pinnedE2EJob, cloudPDPLeg the leg of the end-to-end jobs whose
	// clients call the cloud PDP, and latestLeg the one that starts the newest release of the PDP
	// container.
	pinnedLeg   = "pinned"
	cloudPDPLeg = "cloudpdp"
	latestLeg   = "latest"
	// containerPDPURL is the URL of the PDP container that .github/scripts/pdp-up.sh starts,
	// which the clients of every leg but cloudPDPLeg call.
	containerPDPURL = "http://127.0.0.1:7766"
	// specLockFile is the lock of the spec snapshots, whose pdp-cloud entry has the url of the
	// cloud PDP's spec.
	specLockFile = "tools/specs/lock.json"
	// stopLines lists, for messages, the three lines that start a run: that stops workflow
	// commands.
	stopLines = `token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n'), ` +
		`echo "::stop-commands::$token" and trap 'echo "::$token::"' EXIT`
)

var (
	// pinnedUses matches an action or reusable workflow pinned to a full commit SHA.
	pinnedUses = regexp.MustCompile(
		`^([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+(?:/[^@\s]+)?)@([0-9a-f]{40})$`)
	// versionComment matches the comment that names a pinned action's version.
	versionComment = regexp.MustCompile(`^#\s*(v[0-9]+\.[0-9]+\.[0-9]+)$`)
	// exactGo matches an exact Go patch release.
	exactGo = regexp.MustCompile(`^1\.[0-9]+\.[0-9]+$`)
	// envRef and envMatrixRef match a go-version that reads the workflow env.
	envRef       = regexp.MustCompile(`^\$\{\{\s*env\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}$`)
	envMatrixRef = regexp.MustCompile(
		`^\$\{\{\s*env\[\s*matrix\.([A-Za-z_][A-Za-z0-9_-]*)\s*\]\s*\}\}$`)
	// stopCommands matches a stop-commands line whose token is a variable.
	stopCommands = regexp.MustCompile(`::stop-commands::\$\{?[A-Za-z_]`)
	// stopToken matches the first of the three lines that stop workflow commands, and captures
	// the name of the variable that holds the random token.
	stopToken = regexp.MustCompile(
		`^([A-Za-z_][A-Za-z0-9_]*)=\$\(od -An -N16 -tx1 /dev/urandom \| tr -d ' \\n'\)$`)
	// scriptCall matches a command line that runs a script in .github/scripts, and captures the
	// script's name and what follows it.
	scriptCall = regexp.MustCompile(`^\.github/scripts/([A-Za-z0-9._-]+)(\s.*)?$`)
	// shellOperator matches what makes the rest of a command line more than arguments: a control
	// operator or pipe, a redirection or a command substitution.
	shellOperator = regexp.MustCompile("[&|;<>`]|\\$\\(")
	// heredoc matches the name<<delimiter line of a multi-line GITHUB_OUTPUT or GITHUB_ENV value,
	// and captures the name and the delimiter without quotes.
	heredoc = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_-]*)<<['"]?([^<\s'"]+)`)
	// fingerprint matches a gitleaks fingerprint: <commit>:<path>:<rule>:<line>.
	fingerprint = regexp.MustCompile(`[0-9a-f]{40}:[^\s:]+:[^\s:]+:[0-9]+`)
	// hardVersion matches a version such as 1.2.3 or v1.2.3, but not an IPv4 address.
	hardVersion = regexp.MustCompile(`(?:^|[^0-9.])(v?[0-9]+\.[0-9]+\.[0-9]+)(?:[^0-9.]|$)`)
	// cronName matches the name of a workflow env variable that declares a cron.
	cronName = regexp.MustCompile(`^([A-Z][A-Z0-9]*)_CRON$`)
	// scheduledIf matches the if: of a job in SCHEDULED_JOBS, and captures its cron and mode.
	scheduledIf = regexp.MustCompile(`^\(!inputs\.mode\s*&&\s*github\.event\.schedule\s*==\s*` +
		`'([^']*)'\)\s*\|\|\s*inputs\.mode\s*==\s*'([^']*)'$`)
	// scheduledPushIf matches the if: of a job in SCHEDULED_JOBS and ADVISORY_JOBS that runs on
	// a push too, and captures its cron and mode.
	scheduledPushIf = regexp.MustCompile(`^\(!inputs\.mode\s*&&\s*\(github\.event_name\s*==\s*` +
		`'push'\s*\|\|\s*github\.event\.schedule\s*==\s*'([^']*)'\)\)\s*\|\|\s*` +
		`inputs\.mode\s*==\s*'([^']*)'$`)
	// refersToSchedule matches an if: that depends on the schedule.
	refersToSchedule = regexp.MustCompile(
		`github\.event\.schedule|github\.event_name\s*==\s*'schedule'`)
	// settingRef matches a read of a secret or a variable, secrets.NAME or secrets['NAME'], and
	// captures the context and the name.
	settingRef = regexp.MustCompile(`\b(secrets|vars)\s*(?:\.\s*([A-Za-z_][A-Za-z0-9_]*)|` +
		`\[\s*'([^']*)'\s*\])`)
	// requiredTestsCall matches a command line that runs checkgotest with a list of the tests
	// that a run must pass, and captures the list's path.
	requiredTestsCall = regexp.MustCompile(`^"?[^\s"]*checkgotest"?\s(?:[^&|;]*\s)?` +
		`-require-file[\s=]"?([^\s"]+)"?(?:\s|$)`)
	// e2eTestCall matches a command line that runs go test on the end-to-end scenarios,
	// tools/e2e.
	e2eTestCall = regexp.MustCompile(`^"?(?:[^\s"]*/)?go"?\s(?:[^&|;]*\s)?test\s` +
		`(?:[^&|;]*\s)?\S*/e2e(?:\s|$)`)
	// legFlag matches an -e2e.leg flag and its value, and captures the value in double quotes
	// or, without them, up to the next space.
	legFlag = regexp.MustCompile(`(?:^|\s)--?e2e\.leg(?:=|\s+)(?:"([^"]*)"|(\S*))`)
	// taggedImage matches an image reference with a tag and no digest, as pdp-up.sh takes it.
	taggedImage = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*:[A-Za-z0-9._-]+$`)
	// pdpSpecCall matches a command line that runs specsnap -check-pdp, which checks that a
	// running PDP serves the spec of its snapshot.
	pdpSpecCall = regexp.MustCompile(`^"?[^\s"]*specsnap"?\s(?:[^&|;]*\s)?-check-pdp[\s=]`)
	// envExpression matches a read of the env context, ${{ env.NAME }}, and captures the name.
	envExpression = regexp.MustCompile(`\$\{\{\s*env\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)
	// digestImage matches an image reference pinned by digest.
	digestImage = regexp.MustCompile(`^[^\s@]+@sha256:[0-9a-f]{64}$`)
)

// pullRequestLanes are the workflows that run on pull requests outside the CI workflow, so
// outside the check the ruleset requires, by path, with what each is. Their jobs that read
// secrets run only on a pull request from a branch of this repository.
var pullRequestLanes = map[string]string{
	".github/workflows/e2e-pr.yml": "the pull request lane of the end-to-end tests, whose " +
		"deployment to the integration environment pull requests into master need",
}

// v1Settings are the settings of the integration environment that only the v1 lane reads.
var v1Settings = []string{"PERMIT_API_KEY", "PERMIT_ENV"}

// e2eLegs are the legs of the end-to-end jobs, one job each.
var e2eLegs = []string{pinnedLeg, latestLeg, cloudPDPLeg}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// options is the command line.
type options struct {
	workflows           string
	printPins           bool
	printAllowedActions bool
	includeRefs         []string
}

// run parses the command line, reads the repository and checks it or prints what was asked for.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	opts, err := parseArgs(args, stderr)
	if err != nil {
		return abort(stdout, err)
	}
	root := filepath.Dir(filepath.Dir(opts.workflows))
	workflows, err := readWorkflows(opts.workflows, root)
	if err != nil {
		return abort(stdout, err)
	}
	switch {
	case opts.printPins:
		return printPins(stdout, workflows, root)
	case opts.printAllowedActions:
		for _, ref := range opts.includeRefs {
			fromRef, refErr := readWorkflowsAt(ctx, root, ref, opts.workflows)
			if refErr != nil {
				return abort(stdout, refErr)
			}
			workflows = append(workflows, fromRef...)
		}
		return printAllowedActions(stdout, workflows)
	}
	scripts, err := readScripts(filepath.Join(root, ".github", "scripts"))
	if err != nil {
		return abort(stdout, err)
	}
	files, err := readTree(root, ".github")
	if err != nil {
		return abort(stdout, err)
	}
	committed, err := listCommitted(ctx, root)
	if err != nil {
		return abort(stdout, err)
	}
	modules, err := readModuleSetup(root, committed)
	if err != nil {
		return abort(stdout, err)
	}
	cloudPDP, err := readCloudPDP(root)
	if err != nil {
		return abort(stdout, err)
	}
	return check(stdout, workflows, scripts, files, repository{
		trivyFiles: findTrivyFiles(committed), modules: modules, cloudPDP: cloudPDP,
	})
}

// readCloudPDP returns the URL of the cloud PDP: the origin of the url of the pdp-cloud entry of
// specLockFile, under root.
func readCloudPDP(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(specLockFile)))
	if err != nil {
		return "", err
	}
	var lock map[string]struct {
		URL string `json:"url"`
	}
	if err = json.Unmarshal(data, &lock); err != nil {
		return "", fmt.Errorf("%s: %w", specLockFile, err)
	}
	cloud := lock["pdp-cloud"].URL
	spec, err := url.Parse(cloud)
	if err != nil || spec.Scheme == "" || spec.Host == "" {
		return "", fmt.Errorf("%s: the url of pdp-cloud, %q, is not an absolute URL such as "+
			"https://cloudpdp.api.permit.io/openapi.json; cicheck checks the PERMIT_PDP_URL of "+
			"the leg %s against its origin", specLockFile, cloud, cloudPDPLeg)
	}
	return spec.Scheme + "://" + spec.Host, nil
}

func parseArgs(args []string, stderr io.Writer) (options, error) {
	var opts options
	flags := flag.NewFlagSet("cicheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.workflows, "workflows", filepath.Join(".github", "workflows"),
		"the workflows `directory`; the repository root is two levels above it")
	flags.BoolVar(&opts.printPins, "print-pins", false,
		"print every uses: and "+lockFile+" entry with its version, and do not check")
	flags.BoolVar(&opts.printAllowedActions, "print-allowed-actions", false,
		"print the allowed-actions API request body for the workflows, and do not check")
	flags.Func("include-ref",
		"with -print-allowed-actions, also read the workflows at this git `ref` (repeatable)",
		func(ref string) error {
			opts.includeRefs = append(opts.includeRefs, ref)
			return nil
		})
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	switch {
	case flags.NArg() > 0:
		return options{}, fmt.Errorf("unexpected arguments %q", flags.Args())
	case opts.printPins && opts.printAllowedActions:
		return options{}, errors.New("-print-pins and -print-allowed-actions exclude each other")
	case len(opts.includeRefs) > 0 && !opts.printAllowedActions:
		return options{}, errors.New("-include-ref needs -print-allowed-actions")
	}
	return opts, nil
}

// abort prints why cicheck stopped before it checked anything, then the verdict, and returns
// exitDidNotRun.
func abort(output io.Writer, reason error) int {
	fmt.Fprintf(output, "cicheck: DID NOT RUN: %v\n", reason)
	fmt.Fprintln(output, "cicheck: DID NOT RUN")
	return exitDidNotRun
}

// workflow is a parsed workflow file.
type workflow struct {
	// path is the file's path relative to the repository root, with slashes, and with
	// "<ref>:" in front for a workflow read from a git ref.
	path string
	// doc is the top-level mapping.
	doc *yaml.Node
}

// readWorkflows reads and parses every .yml and .yaml file in dir.
func readWorkflows(dir, root string) ([]workflow, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var workflows []workflow
	for _, entry := range entries {
		if entry.IsDir() || !isYAML(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		w, err := parseWorkflow(filepath.ToSlash(rel), data)
		if err != nil {
			return nil, err
		}
		workflows = append(workflows, w)
	}
	if len(workflows) == 0 {
		return nil, fmt.Errorf("%s holds no .yml or .yaml workflow", dir)
	}
	return workflows, nil
}

// readWorkflowsAt reads and parses the workflows that the git ref holds in the directory that
// dir names in the working tree.
func readWorkflowsAt(ctx context.Context, root, ref, dir string) ([]workflow, error) {
	treeDir := filepath.ToSlash(filepath.Join(filepath.Base(filepath.Dir(dir)),
		filepath.Base(dir)))
	listed, err := gitOutput(ctx, root, "ls-tree", "--name-only", ref+":"+treeDir)
	if err != nil {
		return nil, err
	}
	var workflows []workflow
	for name := range strings.SplitSeq(strings.TrimSpace(listed), "\n") {
		if !isYAML(name) {
			continue
		}
		data, err := gitOutput(ctx, root, "show", ref+":"+treeDir+"/"+name)
		if err != nil {
			return nil, err
		}
		w, err := parseWorkflow(ref+":"+treeDir+"/"+name, []byte(data))
		if err != nil {
			return nil, err
		}
		workflows = append(workflows, w)
	}
	if len(workflows) == 0 {
		return nil, fmt.Errorf("%s holds no .yml or .yaml workflow in %s", ref, treeDir)
	}
	return workflows, nil
}

// gitOutput runs git in dir and returns what it printed, or an error with what it printed on
// stderr.
func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	//nolint:gosec // G204: git lists the files, or reads the ref -include-ref names.
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err,
			strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func isYAML(name string) bool {
	return strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml")
}

// parseWorkflow parses a workflow file whose top level must be a mapping.
func parseWorkflow(path string, data []byte) (workflow, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return workflow{}, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 ||
		resolve(doc.Content[0]).Kind != yaml.MappingNode {
		return workflow{}, fmt.Errorf("%s is not a workflow: its top level is not a mapping", path)
	}
	return workflow{path: path, doc: resolve(doc.Content[0])}, nil
}

// readScripts reads every file in dir; a missing dir holds no scripts.
func readScripts(dir string) (map[string]string, error) {
	scripts := map[string]string{}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return scripts, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		scripts[entry.Name()] = string(data)
	}
	return scripts, nil
}

// readTree reads every regular file under dir, a slash path relative to root, keyed by its slash
// path relative to root. It reads through an os.Root, so a symbolic link cannot lead it out of
// root.
func readTree(root, dir string) (map[string]string, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	tree := r.FS()
	files := map[string]string{}
	walkErr := fs.WalkDir(tree, dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() {
			return err
		}
		data, err := fs.ReadFile(tree, path)
		if err != nil {
			return err
		}
		files[path] = string(data)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return files, nil
}

// trivyFileNames are the names of the files Trivy reads its settings and ignored findings from.
var trivyFileNames = []string{"trivy.yaml", ".trivyignore", ".trivyignore.yaml"}

// listCommitted returns the slash path, relative to root, of every file git tracks. The rules
// about file names compare the names themselves, since git pathspec patterns change meaning with
// GIT_LITERAL_PATHSPECS and GIT_GLOB_PATHSPECS.
func listCommitted(ctx context.Context, root string) ([]string, error) {
	listed, err := gitOutput(ctx, root, "ls-files", "-z")
	if err != nil {
		return nil, fmt.Errorf("listing the committed files: %w", err)
	}
	return strings.Split(strings.TrimSuffix(listed, "\x00"), "\x00"), nil
}

// findTrivyFiles returns the committed files named like a Trivy settings or ignore file.
func findTrivyFiles(committed []string) []string {
	var found []string
	for _, name := range committed {
		if slices.Contains(trivyFileNames, path.Base(name)) {
			found = append(found, name)
		}
	}
	return found
}

// resolve follows an alias to the node it names.
func resolve(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

// pair is a key and its value in a mapping.
type pair struct {
	key   string
	line  int
	value *yaml.Node
}

// pairs returns the keys and values of the mapping m in order, or nothing when m is not a
// mapping.
func pairs(m *yaml.Node) []pair {
	m = resolve(m)
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	var all []pair
	for i := 0; i+1 < len(m.Content); i += 2 {
		key := m.Content[i]
		all = append(all, pair{key: key.Value, line: key.Line, value: resolve(m.Content[i+1])})
	}
	return all
}

// field returns the value of key in the mapping m, or nil.
func field(m *yaml.Node, key string) *yaml.Node {
	for _, p := range pairs(m) {
		if p.key == key {
			return p.value
		}
	}
	return nil
}

// scalar returns the value of n when it is a scalar.
func scalar(n *yaml.Node) (string, bool) {
	if n == nil || n.Kind != yaml.ScalarNode {
		return "", false
	}
	return n.Value, true
}

// items returns the items of n: those of a sequence, or n itself when it is a scalar.
func items(n *yaml.Node) []*yaml.Node {
	switch {
	case n == nil:
		return nil
	case n.Kind == yaml.SequenceNode:
		var all []*yaml.Node
		for _, item := range n.Content {
			all = append(all, resolve(item))
		}
		return all
	case n.Kind == yaml.ScalarNode:
		return []*yaml.Node{n}
	}
	return nil
}

// expression returns the text of an if:, without a ${{ }} around all of it.
func expression(text string) string {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "${{") && strings.HasSuffix(text, "}}") {
		return strings.TrimSpace(text[len("${{") : len(text)-len("}}")])
	}
	return text
}

// finding is a rule that does not hold, at a line of a file.
type finding struct {
	path string
	line int
	msg  string
}

// checker collects findings and counts what it checked.
type checker struct {
	findings    []finding
	jobs, steps int
	// prefixes holds the literal prefix of every concurrency group, by workflow.
	prefixes []groupPrefix
	// cloudPDP is the URL of the cloud PDP, which the clients of the leg cloudpdp call.
	cloudPDP string
}

// groupPrefix is the literal start of a concurrency group.
type groupPrefix struct {
	path   string
	line   int
	prefix string
}

func (c *checker) add(path string, line int, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	c.findings = append(c.findings, finding{path: path, line: line, msg: msg})
}

// repository is what cicheck reads from the repository besides the workflows, the scripts and
// the files under .github.
type repository struct {
	// trivyFiles are the committed files named like Trivy's settings or ignore files.
	trivyFiles []string
	// modules are the Go modules and the configurations that update and tidy them.
	modules moduleSetup
	// cloudPDP is the URL of the cloud PDP, the origin of the url of the pdp-cloud entry of
	// specLockFile.
	cloudPDP string
}

// check checks every rule and prints the findings, the counts and the verdict.
func check(output io.Writer, workflows []workflow, scripts, files map[string]string,
	repo repository,
) int {
	c := &checker{cloudPDP: repo.cloudPDP}
	var ciWorkflows []string
	for _, w := range workflows {
		if c.checkWorkflow(w, scripts) {
			ciWorkflows = append(ciWorkflows, w.path)
		}
	}
	c.checkCalls(workflows)
	// Without any job there is nothing to check, which the verdict reports as not having run.
	switch len(ciWorkflows) {
	case 0:
		if c.jobs > 0 {
			c.add(".github/workflows", 0, "no workflow has a job %q named %q, the required check",
				ciJob, ciName)
		}
	case 1:
	default:
		c.add(".github/workflows", 0, "%d workflows have a job %q named %q: %s; keep one",
			len(ciWorkflows), ciJob, ciName, strings.Join(ciWorkflows, ", "))
	}
	c.checkPrefixes()
	c.checkScripts(scripts)
	c.checkFingerprints(files)
	for _, path := range repo.trivyFiles {
		c.add(path, 0, "Trivy reads its settings or ignored findings from a file of this name; "+
			".github/scripts/trivy.sh gives Trivy every setting on its command line, and this "+
			"file could weaken a scan that reads it; remove it")
	}
	entries := c.checkModules(repo.modules)

	slices.SortStableFunc(c.findings, func(a, b finding) int {
		if a.path != b.path {
			return strings.Compare(a.path, b.path)
		}
		return a.line - b.line
	})
	for _, f := range c.findings {
		if f.line == 0 {
			fmt.Fprintf(output, "cicheck: %s: %s\n", f.path, f.msg)
			continue
		}
		fmt.Fprintf(output, "cicheck: %s:%d: %s\n", f.path, f.line, f.msg)
	}
	fmt.Fprintf(output, "cicheck: checked %d Go modules and %d Dependabot entries\n",
		len(repo.modules.dirs), entries)
	fmt.Fprintf(output, "cicheck: checked %d workflows, %d jobs, %d steps and %d scripts\n",
		len(workflows), c.jobs, c.steps, len(scripts))
	switch {
	case len(c.findings) > 0:
		fmt.Fprintf(output, "cicheck: rules that do not hold: %d\ncicheck: FAIL\n",
			len(c.findings))
		return exitFailed
	case c.jobs == 0:
		fmt.Fprintln(output, "cicheck: DID NOT RUN: the workflows have no jobs")
		fmt.Fprintln(output, "cicheck: DID NOT RUN")
		return exitDidNotRun
	}
	fmt.Fprintln(output, "cicheck: PASS")
	return exitPassed
}

// envVar is a variable of a workflow's top-level env.
type envVar struct {
	value string
	line  int
}

// checkWorkflow checks one workflow and reports whether it has the CI job.
func (c *checker) checkWorkflow(w workflow, scripts map[string]string) bool {
	env := map[string]envVar{}
	for _, p := range pairs(field(w.doc, "env")) {
		value, _ := scalar(p.value)
		env[p.key] = envVar{value: value, line: p.line}
	}
	c.checkTriggers(w, env)
	c.checkWorkflowSettings(w)
	c.collectPrefix(w.path, field(w.doc, "concurrency"))

	jobs := pairs(field(w.doc, "jobs"))
	hasCI := false
	for _, job := range jobs {
		c.jobs++
		c.checkJob(w, job, env, scripts)
		if job.key == ciJob {
			hasCI = true
		}
	}
	switch {
	case hasCI:
		c.checkCI(w, jobs, env)
	case runsOnPullRequest(w):
		c.checkPullRequestLane(w, jobs)
	}
	return hasCI
}

// runsOnPullRequest reports whether a workflow runs on pull_request.
func runsOnPullRequest(w workflow) bool {
	on := field(w.doc, "on")
	return field(on, "pull_request") != nil || slices.ContainsFunc(items(on),
		func(trigger *yaml.Node) bool { return trigger.Value == "pull_request" })
}

// checkPullRequestLane checks a workflow other than the CI workflow that runs on pull requests:
// it must be one of pullRequestLanes; each of its jobs that calls a workflow, runs in an
// environment or reads a secret must run only on a pull request from a branch of this
// repository, in a run that Dependabot did not trigger; and each call passes mode: e2e.
func (c *checker) checkPullRequestLane(w workflow, jobs []pair) {
	if _, ok := pullRequestLanes[w.path]; !ok {
		c.add(w.path, 0, "runs on pull_request outside the CI workflow, so outside the check the "+
			"ruleset requires; check pull requests in the CI workflow, or list this workflow in "+
			"cicheck's pullRequestLanes")
		return
	}
	for _, job := range jobs {
		readsSecret := slices.ContainsFunc(settingRefs(job.value), func(r setting) bool {
			return r.context == "secrets"
		})
		uses, calls := scalar(field(job.value, "uses"))
		if !calls && environment(job.value) == "" && !readsSecret {
			continue
		}
		if cond, _ := scalar(field(job.value, "if")); expression(cond) != sameRepoIf {
			c.add(w.path, job.line, "job %s of the pull request lane can read secrets, so its if: "+
				"must be %s; a pull request from a fork, or a run that Dependabot triggered, gets "+
				"no secrets", job.key, sameRepoIf)
		}
		if mode, _ := scalar(field(field(job.value, "with"), "mode")); calls && mode != "e2e" {
			c.add(w.path, job.line, "job %s of the pull request lane calls %s with mode: %s; "+
				"pass mode: e2e, so that the called run is the end-to-end jobs", job.key, uses,
				mode)
		}
	}
}

// permissionLevels orders the levels of a permission of the GITHUB_TOKEN.
var permissionLevels = map[string]int{"none": 0, "read": 1, "write": 2}

// granted returns the level of the permission scope, such as contents, that a permissions:
// grants: a mapping of scopes, read-all or write-all. Without one it grants nothing.
func granted(permissions *yaml.Node, scope string) int {
	if all, ok := scalar(permissions); ok {
		switch all {
		case "write-all":
			return permissionLevels["write"]
		case "read-all":
			return permissionLevels["read"]
		}
		return permissionLevels["none"]
	}
	level, _ := scalar(field(permissions, scope))
	return permissionLevels[level]
}

// checkCalls checks that each job that calls a workflow of this repository grants every
// permission that a job of that workflow asks for: GitHub refuses to start the call otherwise,
// whatever the if: of the called job.
func (c *checker) checkCalls(workflows []workflow) {
	byPath := map[string]workflow{}
	for _, w := range workflows {
		byPath[w.path] = w
	}
	for _, w := range workflows {
		for _, job := range pairs(field(w.doc, "jobs")) {
			uses, _ := scalar(field(job.value, "uses"))
			path, local := strings.CutPrefix(uses, "./")
			if !local {
				continue
			}
			called, ok := byPath[path]
			if !ok {
				c.add(w.path, job.line, "job %s calls %s, but the workflows directory holds no "+
					"such workflow", job.key, uses)
				continue
			}
			grants := field(job.value, "permissions")
			if grants == nil {
				grants = field(w.doc, "permissions")
			}
			for _, calledJob := range pairs(field(called.doc, "jobs")) {
				asks := field(calledJob.value, "permissions")
				if asks == nil {
					asks = field(called.doc, "permissions")
				}
				for _, p := range pairs(asks) {
					level, _ := scalar(p.value)
					if granted(grants, p.key) < permissionLevels[level] {
						c.add(w.path, job.line, "job %s calls %s, whose job %s asks for %s: %s, "+
							"which job %s does not grant; GitHub refuses to start the call",
							job.key, uses, calledJob.key, p.key, level, job.key)
					}
				}
			}
		}
	}
}

// environment returns the name of the GitHub environment that a job runs in, or "".
func environment(job *yaml.Node) string {
	env := field(job, "environment")
	if name, ok := scalar(env); ok {
		return name
	}
	name, _ := scalar(field(env, "name"))
	return name
}

// setting is a read of a secret or a variable: its context, secrets or vars, its name and its
// line.
type setting struct {
	context, name string
	line          int
}

// settingRefs returns the secrets and variables that the scalars under n read, in their order.
func settingRefs(n *yaml.Node) []setting {
	n = resolve(n)
	if n == nil {
		return nil
	}
	if n.Kind == yaml.ScalarNode {
		var found []setting
		for _, m := range settingRef.FindAllStringSubmatch(n.Value, -1) {
			found = append(found, setting{context: m[1], name: m[2] + m[3], line: n.Line})
		}
		return found
	}
	var found []setting
	for _, child := range n.Content {
		found = append(found, settingRefs(child)...)
	}
	return found
}

// checkSettings checks the secrets and variables that a job reads: a secret of the end-to-end
// jobs only in their environment, no other secret in that environment, which holds the v1
// lane's secrets too, and none of the v1 lane's settings anywhere.
func (c *checker) checkSettings(path string, job pair) {
	env := environment(job.value)
	for _, ref := range settingRefs(job.value) {
		e2e := strings.HasPrefix(ref.name, e2eSecrets)
		switch {
		case slices.Contains(v1Settings, ref.name):
			c.add(path, ref.line, "job %s reads %s.%s, which only the v1 lane reads; the "+
				"end-to-end jobs read the secrets %s* and the variable PERMIT_PROJECT", job.key,
				ref.context, ref.name, e2eSecrets)
		case ref.context != "secrets":
		case e2e && env != e2eEnvironment:
			c.add(path, ref.line, "job %s reads the secret %s outside the %s environment; give "+
				"it environment: %s, so that only the runs that environment allows get it",
				job.key, ref.name, e2eEnvironment, e2eEnvironment)
		case !e2e && env == e2eEnvironment:
			c.add(path, ref.line, "job %s runs in the %s environment and reads the secret %s; "+
				"read only the secrets %s* there", job.key, e2eEnvironment, ref.name, e2eSecrets)
		}
	}
}

// checkWorkflowSettings checks the secrets and variables that a workflow's top-level env and on:
// read, which reach every job of the workflow: neither reads a secret of the end-to-end jobs or
// a setting of the v1 lane.
func (c *checker) checkWorkflowSettings(w workflow) {
	for _, key := range []string{"env", "on"} {
		for _, ref := range settingRefs(field(w.doc, key)) {
			switch {
			case slices.Contains(v1Settings, ref.name):
				c.add(w.path, ref.line, "the workflow's %s: reads %s.%s, which only the v1 lane "+
					"reads", key, ref.context, ref.name)
			case ref.context == "secrets" && strings.HasPrefix(ref.name, e2eSecrets):
				c.add(w.path, ref.line, "the workflow's %s: reads the secret %s, which would "+
					"reach every job of the workflow; read it in the env of a step of a job in "+
					"the %s environment", key, ref.name, e2eEnvironment)
			}
		}
	}
}

// checkTriggers checks the pull_request filters and that on.schedule and the env's crons match.
func (c *checker) checkTriggers(w workflow, env map[string]envVar) {
	on := field(w.doc, "on")
	for _, trigger := range items(on) {
		if trigger.Value == "pull_request_target" {
			c.add(w.path, trigger.Line, "runs on pull_request_target, which runs with the base "+
				"repository's secrets; use pull_request")
		}
	}
	var schedule *yaml.Node
	for _, p := range pairs(on) {
		switch p.key {
		case "pull_request_target":
			c.add(w.path, p.line, "runs on pull_request_target, which runs with the base "+
				"repository's secrets; use pull_request")
		case "pull_request":
			for _, filter := range pairs(p.value) {
				switch filter.key {
				case "branches", "branches-ignore", "paths", "paths-ignore":
					c.add(w.path, filter.line, "pull_request has a %s filter; CI must run on "+
						"every pull request, into any branch", filter.key)
				}
			}
		case "schedule":
			schedule = p.value
		}
	}

	declared := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(env)) {
		if v := env[name]; cronName.MatchString(name) {
			if other, ok := declared[v.value]; ok {
				c.add(w.path, v.line, "env declares the cron %q as both %s and %s", v.value,
					other, name)
			}
			declared[v.value] = name
		}
	}
	scheduled := map[string]bool{}
	for _, entry := range items(schedule) {
		cron, _ := scalar(field(entry, "cron"))
		switch {
		case scheduled[cron]:
			c.add(w.path, entry.Line, "on.schedule lists the cron %q twice", cron)
		case declared[cron] == "":
			c.add(w.path, entry.Line, "on.schedule has the cron %q, which no <MODE>_CRON "+
				"variable in the workflow env declares", cron)
		}
		scheduled[cron] = true
	}
	for cron, name := range declared {
		if !scheduled[cron] {
			c.add(w.path, env[name].line, "env declares %s = %q, which on.schedule does not list",
				name, cron)
		}
	}
}

// collectPrefix records the literal prefix of a concurrency group, and reports a group that
// starts with an expression.
func (c *checker) collectPrefix(path string, concurrency *yaml.Node) {
	if concurrency == nil {
		return
	}
	group, ok := scalar(concurrency)
	line := concurrency.Line
	if !ok {
		groupNode := field(concurrency, "group")
		group, _ = scalar(groupNode)
		if groupNode != nil {
			line = groupNode.Line
		}
	}
	prefix, _, _ := strings.Cut(group, "${{")
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		c.add(path, line, "the concurrency group %q does not start with a literal prefix, such "+
			"as ci-; a called workflow and its caller could share the group", group)
		return
	}
	c.prefixes = append(c.prefixes, groupPrefix{path: path, line: line, prefix: prefix})
}

// checkPrefixes reports concurrency groups of different workflows where one prefix starts the
// other.
func (c *checker) checkPrefixes() {
	for i, a := range c.prefixes {
		for _, b := range c.prefixes[i+1:] {
			if a.path == b.path {
				continue
			}
			if strings.HasPrefix(a.prefix, b.prefix) || strings.HasPrefix(b.prefix, a.prefix) {
				c.add(b.path, b.line, "the concurrency group prefix %q overlaps %q in %s; "+
					"give each workflow its own prefix", b.prefix, a.prefix, a.path)
			}
		}
	}
}

// checkJob checks a job's runner, timeout, concurrency and steps.
func (c *checker) checkJob(w workflow, job pair, env map[string]envVar,
	scripts map[string]string,
) {
	c.collectPrefix(w.path, field(job.value, "concurrency"))
	c.checkSettings(w.path, job)
	if uses := field(job.value, "uses"); uses != nil {
		c.checkUses(w.path, uses)
		return
	}
	if runsOn, _ := scalar(field(job.value, "runs-on")); runsOn != runner {
		c.add(w.path, job.line, "job %s runs on %q; use %s", job.key, runsOn, runner)
	}
	timeout, _ := scalar(field(job.value, "timeout-minutes"))
	if minutes, err := strconv.Atoi(timeout); err != nil || minutes <= 0 {
		c.add(w.path, job.line, "job %s has no timeout-minutes", job.key)
	}
	for _, step := range items(field(job.value, "steps")) {
		c.steps++
		c.checkStep(w, job.value, step, env, scripts)
	}
}

// checkStep checks a step's uses: and run:.
func (c *checker) checkStep(w workflow, job, step *yaml.Node, env map[string]envVar,
	scripts map[string]string,
) {
	if uses := field(step, "uses"); uses != nil {
		c.checkUses(w.path, uses)
		with := field(step, "with")
		switch action, _, _ := strings.Cut(uses.Value, "@"); action {
		case "actions/checkout":
			if value, _ := scalar(field(with, "persist-credentials")); value != "false" {
				c.add(w.path, uses.Line, "actions/checkout needs persist-credentials: false, so "+
					"later steps cannot use the token it stores")
			}
		case "actions/setup-go":
			c.checkSetupGo(w.path, uses.Line, job, with, env)
		}
	}
	runNode := field(step, "run")
	script, ok := scalar(runNode)
	if !ok {
		return
	}
	if strings.Contains(script, "${{") {
		c.add(w.path, runNode.Line, "run: contains ${{; pass the value through env: and use "+
			"the variable")
	}
	c.checkStopCommands(w.path, runNode.Line, script, scripts)
	for line := range strings.SplitSeq(script, "\n") {
		c.checkHeredoc(w.path, runNode.Line, line)
	}
}

// checkUses checks that a uses: outside this repository is pinned to a commit SHA with a
// version comment.
func (c *checker) checkUses(path string, uses *yaml.Node) {
	if strings.HasPrefix(uses.Value, "./") {
		return
	}
	if !pinnedUses.MatchString(uses.Value) {
		c.add(path, uses.Line, "uses: %s is not pinned to a 40-character commit SHA", uses.Value)
		return
	}
	if !versionComment.MatchString(uses.LineComment) {
		c.add(path, uses.Line, "uses: %s has no version comment such as # v1.2.3 after its SHA",
			uses.Value)
	}
}

// checkSetupGo checks that a setup-go step restores no cache and installs an exact Go release.
func (c *checker) checkSetupGo(path string, line int, job, with *yaml.Node,
	env map[string]envVar,
) {
	if cache, _ := scalar(field(with, "cache")); cache != "false" {
		c.add(path, line, "actions/setup-go needs cache: false; a cache from another run must "+
			"not decide a check")
	}
	if latest, _ := scalar(field(with, "check-latest")); latest != "" && latest != "false" {
		c.add(path, line, "actions/setup-go sets check-latest; pin an exact Go release instead")
	}
	versionFile, _ := scalar(field(with, "go-version-file"))
	versionNode := field(with, "go-version")
	version, _ := scalar(versionNode)
	switch {
	case versionNode == nil && filepath.Base(versionFile) == "go.mod":
		return
	case versionNode == nil:
		c.add(path, line, "actions/setup-go needs a go-version, or a go-version-file that "+
			"names a go.mod")
		return
	}
	var candidates []string
	if m := envRef.FindStringSubmatch(version); m != nil {
		candidates = []string{m[1]}
	} else if m := envMatrixRef.FindStringSubmatch(version); m != nil {
		for _, value := range matrixValues(job, m[1]) {
			candidates = append(candidates, value.Value)
		}
		if len(candidates) == 0 {
			c.add(path, line, "go-version %s: the job's matrix has no %s", version, m[1])
		}
	} else {
		if !exactGo.MatchString(version) {
			c.add(path, line, "go-version %q is not an exact Go release such as 1.27.1", version)
		}
		return
	}
	for _, name := range candidates {
		v, ok := env[name]
		switch {
		case !ok:
			c.add(path, line, "go-version %s: the workflow env has no %s", version, name)
		case !exactGo.MatchString(v.value):
			c.add(path, v.line, "%s = %q is not an exact Go release such as 1.27.1", name,
				v.value)
		}
	}
}

// matrixValues returns the values a job's matrix gives key, from a list and from include.
func matrixValues(job *yaml.Node, key string) []*yaml.Node {
	matrix := field(field(job, "strategy"), "matrix")
	var values []*yaml.Node
	for _, value := range items(field(matrix, key)) {
		if value.Kind == yaml.ScalarNode {
			values = append(values, value)
		}
	}
	for _, entry := range items(field(matrix, "include")) {
		if value := field(entry, key); value != nil && value.Kind == yaml.ScalarNode {
			values = append(values, value)
		}
	}
	return values
}

// checkStopCommands checks that a run: stops workflow commands itself, or only calls scripts
// that do.
func (c *checker) checkStopCommands(path string, line int, script string,
	scripts map[string]string,
) {
	commands := commandLines(script)
	if len(commands) == 0 || stopsCommands(commands) {
		return
	}
	for _, command := range commands {
		m := scriptCall.FindStringSubmatch(command)
		if m == nil || shellOperator.MatchString(m[2]) {
			c.add(path, line, "run: does not start with the three lines that stop workflow "+
				"commands and does more than call scripts in .github/scripts, so tool output "+
				"could issue workflow commands; start it with %s", stopLines)
			return
		}
		content, ok := scripts[m[1]]
		switch {
		case !ok:
			c.add(path, line, "run: runs .github/scripts/%s, which does not exist", m[1])
		case !stopCommands.MatchString(content):
			c.add(path, line, "run: runs .github/scripts/%s, which prints no ::stop-commands:: "+
				"line with its token in a variable", m[1])
		}
	}
}

// stopsCommands reports whether the command lines of a run: start with the three lines that stop
// workflow commands until the step ends: a random token in a variable, the stop-commands line
// with that token, and a trap that resumes the commands with it on exit.
func stopsCommands(commands []string) bool {
	if len(commands) < 3 {
		return false
	}
	m := stopToken.FindStringSubmatch(commands[0])
	return m != nil && commands[1] == `echo "::stop-commands::$`+m[1]+`"` &&
		commands[2] == `trap 'echo "::$`+m[1]+`::"' EXIT`
}

// commandLines returns the lines of a script that are not blank or comments, with lines that
// end in a backslash joined to the next.
func commandLines(script string) []string {
	var commands []string
	var current strings.Builder
	for line := range strings.SplitSeq(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if current.Len() == 0 && (trimmed == "" || strings.HasPrefix(trimmed, "#")) {
			continue
		}
		if joined, ok := strings.CutSuffix(trimmed, `\`); ok {
			current.WriteString(joined + " ")
			continue
		}
		current.WriteString(trimmed)
		commands = append(commands, current.String())
		current.Reset()
	}
	if current.Len() > 0 {
		commands = append(commands, current.String())
	}
	return commands
}

// checkHeredoc reports a name<<delimiter line whose delimiter is fixed.
func (c *checker) checkHeredoc(path string, line int, text string) {
	for _, m := range heredoc.FindAllStringSubmatch(text, -1) {
		if !strings.HasPrefix(m[2], "$") {
			c.add(path, line, "the multi-line value %s uses the fixed delimiter %q; take a random "+
				"one from a variable", m[1], m[2])
		}
	}
}

// checkCI checks the CI job of a workflow and the lists of jobs its step reads.
func (c *checker) checkCI(w workflow, jobs []pair, env map[string]envVar) {
	var ci pair
	byID := map[string]*yaml.Node{}
	// after holds the jobs that need CI, which run after it.
	after := map[string]bool{}
	for _, job := range jobs {
		byID[job.key] = job.value
		if job.key == ciJob {
			ci = job
		}
		for _, need := range items(field(job.value, "needs")) {
			if need.Value == ciJob && job.key != ciJob {
				after[job.key] = true
			}
		}
	}
	if name, _ := scalar(field(ci.value, "name")); name != ciName {
		c.add(w.path, ci.line, "job %s is named %q; name it %s, the check the ruleset requires",
			ciJob, name, ciName)
	}
	if cond, _ := scalar(field(ci.value, "if")); expression(cond) != "always()" {
		c.add(w.path, ci.line, "job %s needs if: always(), so it reports when a job it needs "+
			"fails or is skipped", ciJob)
	}

	needs := map[string]bool{}
	for _, need := range items(field(ci.value, "needs")) {
		switch {
		case needs[need.Value]:
			c.add(w.path, need.Line, "CI's needs lists %s twice", need.Value)
		case byID[need.Value] == nil || need.Value == ciJob:
			c.add(w.path, need.Line, "CI's needs lists %s, which is not another job of this "+
				"workflow", need.Value)
		case after[need.Value]:
			c.add(w.path, need.Line, "CI's needs lists %s, which needs CI; a job that runs "+
				"after CI cannot be one CI waits for", need.Value)
		}
		needs[need.Value] = true
	}
	for _, job := range jobs {
		if job.key != ciJob && !needs[job.key] && !after[job.key] {
			c.add(w.path, job.line, "CI's needs does not list job %s, so CI would pass "+
				"whatever it does", job.key)
		}
	}

	stepEnv, line := c.ciStepEnv(w.path, ci)
	if stepEnv == nil {
		return
	}
	expected, _ := scalar(field(stepEnv, "EXPECTED_JOBS"))
	if count, err := strconv.Atoi(expected); err != nil || count != len(needs) {
		c.add(w.path, line, "EXPECTED_JOBS is %q, but CI needs %d jobs", expected, len(needs))
	}
	pullRequest := c.listedJobs(w.path, line, stepEnv, "PULL_REQUEST_JOBS", needs)
	e2e := c.listedJobs(w.path, line, stepEnv, "E2E_JOBS", needs)
	advisory := c.listedJobs(w.path, line, stepEnv, "ADVISORY_JOBS", needs)
	if advisory[pinnedE2EJob] {
		c.add(w.path, line, "ADVISORY_JOBS names %s, the end-to-end job with the pinned PDP, "+
			"which a push to master and a release require; only the other end-to-end jobs may "+
			"be advisory", pinnedE2EJob)
	}
	scheduled := map[string]bool{}
	value, _ := scalar(field(stepEnv, "SCHEDULED_JOBS"))
	for entry := range strings.FieldsSeq(value) {
		name, mode, ok := strings.Cut(entry, ":")
		switch {
		case !ok || name == "" || mode == "":
			c.add(w.path, line, "SCHEDULED_JOBS entry %q is not job:mode", entry)
		case !needs[name]:
			c.add(w.path, line, "SCHEDULED_JOBS names %s, which CI does not need", name)
		default:
			scheduled[name] = true
			c.checkScheduledJob(w.path, scheduledJob{
				name: name, mode: mode, value: byID[name], advisory: advisory[name],
			}, env)
		}
	}
	// legJobs holds the end-to-end job of each leg.
	legJobs := map[string]string{}
	for _, job := range jobs {
		switch {
		case job.key == ciJob || scheduled[job.key]:
		case after[job.key]:
			if cond, _ := scalar(field(job.value, "if")); expression(cond) != afterCIIf {
				c.add(w.path, job.line, "job %s needs CI, so it runs after CI to report a "+
					"failure, and its if: must be %s", job.key, afterCIIf)
			}
		case e2e[job.key]:
			if cond, _ := scalar(field(job.value, "if")); expression(cond) != e2eIf {
				c.add(w.path, job.line, "job %s is in E2E_JOBS, so its if: must be %s", job.key,
					e2eIf)
			}
			leg := c.checkE2EJob(w.path, job, env, advisory[job.key])
			switch other := legJobs[leg]; {
			case leg == "":
			case other != "":
				c.add(w.path, job.line, "job %s sets LEG to %s, which job %s sets too; each leg "+
					"needs a scratch environment of its own, as e2e-env.sh names it by the leg",
					job.key, leg, other)
			default:
				legJobs[leg] = job.key
			}
		case environment(job.value) == e2eEnvironment:
			c.add(w.path, job.line, "job %s runs in the %s environment, so list it in E2E_JOBS, "+
				"whose if: keeps it off this workflow's pull requests, which may come from forks",
				job.key, e2eEnvironment)
		default:
			c.checkGateIf(w.path, job, pullRequest[job.key])
		}
	}
}

// checkE2EJob checks a job in E2E_JOBS and returns its leg. Its env sets LEG to its leg, one of
// e2eLegs, and pinned in pinnedE2EJob; a step of it runs go test on ./e2e with -e2e.leg and that
// leg, and the PERMIT_PDP_URL of the leg's PDP; one runs checkgotest with -require-file and the
// list of required tests of that leg; when one creates a scratch environment with e2e-env.sh
// create, one deletes it with e2e-env.sh delete, and each such delete runs under if: always();
// each step that starts the PDP with pdp-up.sh up gives it an image pinned by digest when the job
// is required, and one with a tag and no digest when the leg is latest, and a later step checks
// the PDP's spec; the job of the leg cloudpdp starts no PDP and uploads no artifact; the jobs of
// the legs latest and cloudpdp are advisory; and an advisory job makes no deployment.
func (c *checker) checkE2EJob(path string, job pair, env map[string]envVar, advisory bool) string {
	leg, _ := scalar(field(field(job.value, "env"), "LEG"))
	knownLeg := slices.Contains(e2eLegs, leg)
	switch {
	case !knownLeg:
		c.add(path, job.line, "job %s is in E2E_JOBS, so its env: sets LEG to its leg, one of %s, "+
			"%s and %s: e2e-env.sh names the job's scratch environment by it, and it picks the "+
			"PDP that the tests call and the list of required tests that the job checks", job.key,
			pinnedLeg, latestLeg, cloudPDPLeg)
	case job.key == pinnedE2EJob && leg != pinnedLeg:
		c.add(path, job.line, "job %s is the end-to-end job with the pinned PDP, which a push to "+
			"master and a release require, so its env: sets LEG to %s, not %s", job.key,
			pinnedLeg, leg)
	}
	required := legRequiredTests(leg)
	requiresTests, runsTests, creates, deletes := false, false, false, false
	for _, step := range items(field(job.value, "steps")) {
		uses, _ := scalar(field(step, "uses"))
		if leg == cloudPDPLeg && strings.HasPrefix(uses, "actions/upload-artifact@") {
			c.add(path, step.Line, "job %s runs the leg %s, so it uploads no artifact: its "+
				"request record must stay out of the API coverage, as its one request to POST "+
				"/allowed_url gets a 404 from a PDP that does not serve that route", job.key, leg)
		}
		script, _ := scalar(field(step, "run"))
		for _, command := range commandLines(script) {
			call := requiredTestsCall.FindStringSubmatch(command)
			switch {
			case call != nil:
				requiresTests = requiresTests || call[1] == required
			case e2eTestCall.MatchString(command):
				runsTests = true
				if knownLeg {
					c.checkTestCall(path, job, step, command, leg)
					c.checkPDPURL(path, job, step, env, leg)
				}
			case runsScript(command, "e2e-env.sh", "create"):
				creates = true
			case runsScript(command, "e2e-env.sh", "delete"):
				deletes = true
				if cond, _ := scalar(field(step, "if")); expression(cond) != "always()" {
					c.add(path, step.Line, "job %s deletes its scratch environment in a step "+
						"whose if: is not always(), so a failed or cancelled step before it "+
						"would leave the environment behind", job.key)
				}
			case runsScript(command, "pdp-up.sh", "up"):
				if !advisory {
					c.checkPinnedPDP(path, job, step, env)
				}
				switch leg {
				case latestLeg:
					c.checkLatestPDP(path, job, step, env)
				case cloudPDPLeg:
					c.add(path, step.Line, "job %s runs the leg %s, whose clients call the cloud "+
						"PDP, so it starts no PDP container with pdp-up.sh up", job.key, leg)
				}
			}
		}
	}
	if knownLeg && !runsTests {
		c.add(path, job.line, "job %s is in E2E_JOBS with the leg %s, so a step of it runs the "+
			"end-to-end tests, go test on ./e2e, with -e2e.leg \"$LEG\"", job.key, leg)
	}
	if knownLeg && !requiresTests {
		c.add(path, job.line, "job %s is in E2E_JOBS with the leg %s, so a step of it runs "+
			"checkgotest with -require-file %s, which fails a run that did not run and pass "+
			"each test of that file", job.key, leg, required)
	}
	if creates && !deletes {
		c.add(path, job.line, "job %s creates a scratch environment with e2e-env.sh create, so a "+
			"step of it under if: always() deletes it with e2e-env.sh delete", job.key)
	}
	if (leg == latestLeg || leg == cloudPDPLeg) && !advisory {
		c.add(path, job.line, "job %s runs the leg %s, against a PDP that this repository does "+
			"not pin, so ADVISORY_JOBS names it: a new release of the PDP, or a change of the "+
			"cloud PDP, must not fail CI", job.key, leg)
	}
	if advisory {
		c.checkNoDeployment(path, job)
	}
	c.checkPDPSpec(path, job)
	return leg
}

// checkTestCall checks that a command line of a step of an end-to-end job that runs go test on
// ./e2e passes the job's leg, "$LEG" or its value, in its last -e2e.leg, which the flag package
// takes: the leg picks the scenarios of the leg's PDP and the prefix of what the run creates.
func (c *checker) checkTestCall(path string, job pair, step *yaml.Node, command, leg string) {
	flags := legFlag.FindAllStringSubmatch(command, -1)
	if len(flags) == 0 {
		c.add(path, step.Line, "job %s runs the end-to-end tests without -e2e.leg, so with the "+
			"leg %s; pass -e2e.leg \"$LEG\", the job's leg, which picks the scenarios of the "+
			"leg's PDP and the prefix of what the run creates", job.key, pinnedLeg)
		return
	}
	last := flags[len(flags)-1]
	value := last[1] + last[2]
	if value != "$LEG" && value != "${LEG}" && value != leg {
		c.add(path, step.Line, "job %s runs the end-to-end tests with %s; pass "+
			"-e2e.leg \"$LEG\", the job's leg, which picks the scenarios of the leg's PDP and the "+
			"prefix of what the run creates", job.key, strings.TrimSpace(last[0]))
	}
}

// checkPDPURL checks the PERMIT_PDP_URL of a step of an end-to-end job of leg that runs the
// tests: the cloud PDP's URL on the leg cloudpdp, and containerPDPURL on any other leg.
func (c *checker) checkPDPURL(path string, job pair, step *yaml.Node, env map[string]envVar,
	leg string,
) {
	pdp := "the PDP container that pdp-up.sh starts, at " + containerPDPURL
	want := containerPDPURL
	if leg == cloudPDPLeg {
		pdp = "the cloud PDP, at " + c.cloudPDP + ", the origin of the url of pdp-cloud in " +
			specLockFile
		want = c.cloudPDP
	}
	value, line, ok := stepEnv(job, step, env, "PERMIT_PDP_URL")
	switch {
	case !ok:
		c.add(path, step.Line, "job %s runs the end-to-end tests without a PERMIT_PDP_URL, so "+
			"the SDK's clients would call its default, http://localhost:7766, but the clients of "+
			"the leg %s call %s", job.key, leg, pdp)
	case value != want:
		c.add(path, line, "job %s runs the end-to-end tests with the PERMIT_PDP_URL %s, but the "+
			"clients of the leg %s call %s", job.key, value, leg, pdp)
	}
}

// legRequiredTests returns the list of the tests that a run of the end-to-end scenarios of leg
// must pass: those of the cloud PDP for the leg cloudpdp, and those of the PDP container for any
// other.
func legRequiredTests(leg string) string {
	if leg == cloudPDPLeg {
		return cloudRequiredTests
	}
	return containerRequiredTests
}

// checkNoDeployment checks that an advisory job in E2E_JOBS uses its environment with
// deployment: false: a pull request into master needs a successful deployment to the integration
// environment, which a job that may fail must not decide.
func (c *checker) checkNoDeployment(path string, job pair) {
	if value, _ := scalar(field(field(job.value, "environment"), "deployment")); value != "false" {
		c.add(path, job.line, "job %s is in ADVISORY_JOBS, so it uses the %s environment with "+
			"deployment: false and makes no deployment: a pull request into master needs a "+
			"successful deployment to that environment, which a job that may fail must not "+
			"decide", job.key, e2eEnvironment)
	}
}

// stepRuns reports whether a command line of the run: of step matches.
func stepRuns(step *yaml.Node, match func(command string) bool) bool {
	script, _ := scalar(field(step, "run"))
	return slices.ContainsFunc(commandLines(script), match)
}

// checkPDPSpec checks that an end-to-end job that starts the PDP container checks that the PDP
// serves the spec of its snapshot: a step after the first one that runs pdp-up.sh up runs
// specsnap -check-pdp with the if: always() && steps.<id>.outcome == 'success', where <id> is the
// id: of the step that starts the PDP, so the check runs once the PDP started, whatever the tests
// did.
func (c *checker) checkPDPSpec(path string, job pair) {
	var up *yaml.Node
	for _, step := range items(field(job.value, "steps")) {
		switch {
		case up == nil && stepRuns(step, func(command string) bool {
			return runsScript(command, "pdp-up.sh", "up")
		}):
			up = step
		case up != nil && stepRuns(step, pdpSpecCall.MatchString):
			id, _ := scalar(field(up, "id"))
			if cond, _ := scalar(field(step, "if")); expression(cond) !=
				"always() && steps."+id+".outcome == 'success'" {
				c.add(path, step.Line, "job %s checks the PDP's spec in a step whose if: is not "+
					"always() && steps.<id>.outcome == 'success', where <id> is the id: of the "+
					"step that starts the PDP, %q, so the check would not run when the tests "+
					"fail, or would run without a PDP", job.key, id)
			}
			return
		}
	}
	if up != nil {
		c.add(path, job.line, "job %s starts the PDP container, so a step after the one that "+
			"starts it runs specsnap -check-pdp with the if: always() && steps.<id>.outcome == "+
			"'success', where <id> is that step's id:, which fails when the PDP does not serve "+
			"the spec of tools/specs/pdp-container.json", job.key)
	}
}

// runsScript reports whether a command line runs the script name of .github/scripts with the
// first argument arg.
func runsScript(command, name, arg string) bool {
	m := scriptCall.FindStringSubmatch(command)
	if m == nil || m[1] != name {
		return false
	}
	args := strings.Fields(m[2])
	return len(args) > 0 && strings.Trim(args[0], `"'`) == arg
}

// checkPinnedPDP checks that a step of a required end-to-end job that starts the PDP gives it an
// image pinned by digest in its PDP_IMAGE.
func (c *checker) checkPinnedPDP(path string, job pair, step *yaml.Node,
	env map[string]envVar,
) {
	image, line, ok := stepEnv(job, step, env, "PDP_IMAGE")
	switch {
	case !ok:
		c.add(path, step.Line, "job %s starts the PDP without a PDP_IMAGE; a required "+
			"end-to-end job runs <image>@sha256:<64 hex digits>", job.key)
	case !digestImage.MatchString(image):
		c.add(path, line, "job %s starts the PDP image %s, which is not pinned by digest; a "+
			"required end-to-end job runs <image>@sha256:<64 hex digits>", job.key, image)
	}
}

// checkLatestPDP checks that a step of an end-to-end job of the leg latest that starts the PDP
// gives it the newest release, an image with a tag and no digest, in its PDP_IMAGE.
func (c *checker) checkLatestPDP(path string, job pair, step *yaml.Node,
	env map[string]envVar,
) {
	image, line, ok := stepEnv(job, step, env, "PDP_IMAGE")
	switch {
	case !ok:
		c.add(path, step.Line, "job %s starts the PDP without a PDP_IMAGE; the leg %s runs "+
			"<image>:<tag>, the newest release, with no digest", job.key, latestLeg)
	case !taggedImage.MatchString(image):
		c.add(path, line, "job %s is the leg %s, so it starts the newest release of the PDP by "+
			"its tag: its PDP_IMAGE is <image>:<tag>, with no digest, and not %s", job.key,
			latestLeg, image)
	}
}

// stepEnv returns the value of the variable name that a step of a job sees: the step's, the job's
// or the workflow's, with each ${{ env.NAME }} in it replaced with the job's or the workflow's
// value of NAME, and the line that sets it. ok is false when none of them sets name.
func stepEnv(job pair, step *yaml.Node, env map[string]envVar, name string) (string, int, bool) {
	jobEnv := field(job.value, "env")
	node := field(field(step, "env"), name)
	if node == nil {
		node = field(jobEnv, name)
	}
	var value string
	var line int
	switch workflowValue, ok := env[name]; {
	case node != nil:
		value, _ = scalar(node)
		line = node.Line
	case ok:
		value, line = workflowValue.value, workflowValue.line
	default:
		return "", 0, false
	}
	resolved := envExpression.ReplaceAllStringFunc(value, func(ref string) string {
		refName := envExpression.FindStringSubmatch(ref)[1]
		if refValue, ok := scalar(field(jobEnv, refName)); ok {
			return refValue
		}
		if refValue, ok := env[refName]; ok {
			return refValue.value
		}
		return ref
	})
	return resolved, line, true
}

// listedJobs returns the jobs that a list in the CI step's env names, and reports each job that
// CI does not need.
func (c *checker) listedJobs(path string, line int, env *yaml.Node, list string,
	needs map[string]bool,
) map[string]bool {
	listed := map[string]bool{}
	value, _ := scalar(field(env, list))
	for name := range strings.FieldsSeq(value) {
		if !needs[name] {
			c.add(path, line, "%s names %s, which CI does not need", list, name)
		}
		listed[name] = true
	}
	return listed
}

// checkGateIf checks the if: of a job of the CI workflow outside SCHEDULED_JOBS: the if: of the
// pull request checks, or the one of a job that runs on pull requests only.
func (c *checker) checkGateIf(path string, job pair, pullRequestOnly bool) {
	cond, _ := scalar(field(job.value, "if"))
	want, where := gateIf, "is outside SCHEDULED_JOBS and PULL_REQUEST_JOBS"
	if pullRequestOnly {
		want, where = pullRequestIf, "is in PULL_REQUEST_JOBS"
	}
	switch {
	case expression(cond) == want:
	case refersToSchedule.MatchString(cond):
		c.add(path, job.line, "job %s's if: depends on the schedule; list it in "+
			"SCHEDULED_JOBS as %s:<mode>, with the if: that rule describes", job.key, job.key)
	default:
		c.add(path, job.line, "job %s %s, so its if: must be %s", job.key, where, want)
	}
}

// ciStepEnv returns the env of the CI step that sets EXPECTED_JOBS, and its line.
func (c *checker) ciStepEnv(path string, ci pair) (*yaml.Node, int) {
	var found []*yaml.Node
	for _, step := range items(field(ci.value, "steps")) {
		if env := field(step, "env"); field(env, "EXPECTED_JOBS") != nil {
			found = append(found, env)
		}
	}
	switch len(found) {
	case 0:
		c.add(path, ci.line, "no step of job %s sets EXPECTED_JOBS, the number of jobs CI needs",
			ciJob)
		return nil, 0
	case 1:
		return found[0], field(found[0], "EXPECTED_JOBS").Line
	}
	c.add(path, ci.line, "%d steps of job %s set EXPECTED_JOBS; keep one", len(found), ciJob)
	return nil, 0
}

// scheduledJob is a job in SCHEDULED_JOBS: its id, its mode, the job, and whether ADVISORY_JOBS
// names it too.
type scheduledJob struct {
	name, mode string
	value      *yaml.Node
	advisory   bool
}

// checkScheduledJob checks that a job in SCHEDULED_JOBS runs on one declared cron and one mode,
// and on a push too only when it is in ADVISORY_JOBS: no pull request runs such a job before the
// push, so its failure must not fail CI on master.
func (c *checker) checkScheduledJob(path string, job scheduledJob, env map[string]envVar) {
	name, mode := job.name, job.mode
	condNode := field(job.value, "if")
	cond, _ := scalar(condNode)
	line := 0
	if condNode != nil {
		line = condNode.Line
	}
	cronVar := strings.ToUpper(mode) + "_CRON"
	want := fmt.Sprintf("(!inputs.mode && github.event.schedule == '<%s>') || "+
		"inputs.mode == '%s'", cronVar, mode)
	if job.advisory {
		want += fmt.Sprintf(", or, as it is in ADVISORY_JOBS, to run on a push too: "+
			"(!inputs.mode && (github.event_name == 'push' || github.event.schedule == '<%s>')) "+
			"|| inputs.mode == '%s'", cronVar, mode)
	}
	m := scheduledIf.FindStringSubmatch(expression(cond))
	onPush := false
	if m == nil {
		m = scheduledPushIf.FindStringSubmatch(expression(cond))
		onPush = m != nil
	}
	if m == nil {
		c.add(path, line, "job %s is in SCHEDULED_JOBS, so its if: must be %s", name, want)
		return
	}
	if onPush && !job.advisory {
		c.add(path, line, "job %s runs on a push too, which no pull request checks first; list "+
			"it in ADVISORY_JOBS, so that it cannot fail CI on master", name)
	}
	declared, ok := env[cronVar]
	switch {
	case m[2] != mode:
		c.add(path, line, "job %s is in SCHEDULED_JOBS for mode %s, but its if: names mode %s",
			name, mode, m[2])
	case !ok:
		c.add(path, line, "job %s runs in mode %s, but the workflow env has no %s", name, mode,
			cronVar)
	case declared.value != m[1]:
		c.add(path, line, "job %s's if: names the cron %q, but %s is %q", name, m[1], cronVar,
			declared.value)
	}
}

// checkScripts checks the scripts in .github/scripts for hard-coded versions and fixed
// heredoc delimiters.
func (c *checker) checkScripts(scripts map[string]string) {
	for name, content := range scripts {
		path := ".github/scripts/" + name
		for i, line := range strings.Split(content, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
			c.checkHeredoc(path, i+1, line)
			if m := hardVersion.FindStringSubmatch(line); m != nil {
				c.add(path, i+1, "hard-codes the version %s; read versions from %s", m[1],
					lockFile)
			}
		}
	}
}

// checkFingerprints reports files that hold a string shaped like a gitleaks fingerprint,
// without printing it.
func (c *checker) checkFingerprints(files map[string]string) {
	for path, content := range files {
		for i, line := range strings.Split(content, "\n") {
			if fingerprint.MatchString(line) {
				c.add(path, i+1, "holds a string shaped like a gitleaks fingerprint "+
					"(<commit>:<path>:<rule>:<line>); keep accepted findings out of the repository")
			}
		}
	}
}

// pin is a uses: outside this repository.
type pin struct {
	path, action, ref, version string
	line                       int
}

// pins returns every uses: outside this repository in the workflows, in file order.
func pins(workflows []workflow) []pin {
	var all []pin
	for _, w := range workflows {
		var nodes []*yaml.Node
		for _, job := range pairs(field(w.doc, "jobs")) {
			if uses := field(job.value, "uses"); uses != nil {
				nodes = append(nodes, uses)
			}
			for _, step := range items(field(job.value, "steps")) {
				if uses := field(step, "uses"); uses != nil {
					nodes = append(nodes, uses)
				}
			}
		}
		for _, uses := range nodes {
			if strings.HasPrefix(uses.Value, "./") {
				continue
			}
			action, ref, _ := strings.Cut(uses.Value, "@")
			version := strings.TrimSpace(strings.TrimPrefix(uses.LineComment, "#"))
			all = append(all, pin{
				path: w.path, line: uses.Line, action: action, ref: ref,
				version: version,
			})
		}
	}
	return all
}

// printPins prints every uses: and every entry of the binaries lock with its version.
func printPins(output io.Writer, workflows []workflow, root string) int {
	entries, err := readLock(filepath.Join(root, filepath.FromSlash(lockFile)))
	if err != nil {
		return abort(output, err)
	}
	for _, p := range pins(workflows) {
		version := p.version
		if version == "" {
			version = "(no version comment)"
		}
		fmt.Fprintf(output, "action %s %s %s %s:%d\n", p.action, version, p.ref, p.path, p.line)
	}
	for _, e := range entries {
		fmt.Fprintf(output, "binary %s %s %s:%d\n", e.name, e.version, lockFile, e.line)
	}
	return exitPassed
}

// lockEntry is an entry of the binaries lock.
type lockEntry struct {
	name, version string
	line          int
}

// readLock reads the name and version of every entry of the binaries lock.
func readLock(path string) ([]lockEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []lockEntry
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if name, ok := strings.CutPrefix(line, "["); ok && strings.HasSuffix(name, "]") {
			entries = append(entries, lockEntry{name: strings.TrimSuffix(name, "]"), line: i + 1})
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "version" {
			continue
		}
		if len(entries) == 0 {
			return nil, fmt.Errorf("%s:%d: version is set outside an entry; put it after the "+
				"[name] line of its entry", lockFile, i+1)
		}
		entries[len(entries)-1].version = strings.TrimSpace(value)
	}
	for _, e := range entries {
		if e.version == "" {
			return nil, fmt.Errorf("%s:%d: [%s] has no version", lockFile, e.line, e.name)
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("%s has no entries", lockFile)
	}
	return entries, nil
}

// allowedActions is the body of GitHub's "set allowed actions and reusable workflows" request.
type allowedActions struct {
	GithubOwnedAllowed bool     `json:"github_owned_allowed"`
	VerifiedAllowed    bool     `json:"verified_allowed"`
	PatternsAllowed    []string `json:"patterns_allowed"`
}

// printAllowedActions prints the allowed-actions request body for the workflows: GitHub-owned
// actions, and a pattern for each other action or reusable workflow they use, its full path
// followed by @*, which GitHub's allow list takes.
func printAllowedActions(output io.Writer, workflows []workflow) int {
	body := allowedActions{GithubOwnedAllowed: true, PatternsAllowed: []string{}}
	for _, p := range pins(workflows) {
		owner, _, _ := strings.Cut(p.action, "/")
		if owner == "actions" || owner == "github" {
			continue
		}
		pattern := p.action + "@*"
		if !slices.Contains(body.PatternsAllowed, pattern) {
			body.PatternsAllowed = append(body.PatternsAllowed, pattern)
		}
	}
	slices.Sort(body.PatternsAllowed)
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return abort(output, err)
	}
	fmt.Fprintf(output, "%s\n", data)
	return exitPassed
}
