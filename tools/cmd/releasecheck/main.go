// Command releasecheck checks a release of the SDK in the release workflow,
// .github/workflows/release.yml, before the workflow publishes it, and writes its release notes.
//
// Usage:
//
//	releasecheck validate -tag tag -sha sha -master sha -ci-runs file -evidence file [-root dir]
//	releasecheck e2e -jobs file -ci-job name -e2e-job name -record file [-root dir]
//	releasecheck notes -tag tag [-surface file] [-root dir]
//	releasecheck watchdog -repo owner/name -tags file -release-runs file -releases file \
//	    -ci-runs file -attestations file [-root dir]
//	releasecheck lanes [-root dir]
//
// -root names the repository root, the current directory by default; the paths below are
// relative to it.
//
// # validate
//
// validate runs before the gates, and checks:
//
//   - -tag, the release's tag, is a v2 release tag as a whole string: it matches
//     ^v2\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-rc\.[1-9]\d*)?$, so v2.1.0 or v2.1.0-rc.1, and not
//     v2.0 or v2.1.0 with anything around it;
//   - the tag is v followed by Version, the constant of version.go, or a release candidate of
//     that version, v<Version>-rc.<n>;
//   - -sha, the commit the workflow runs on, is -master, the head of master;
//   - CI succeeded for that commit on its push to master: -ci-runs, the runs of ci.yml for a push
//     of the commit as the GitHub API lists them
//     (repos/<repo>/actions/workflows/ci.yml/runs?head_sha=<sha>&event=push), has a run of the
//     event push for that commit, and the newest such run completed with the conclusion success
//     in its latest attempt; a scheduled or manual run, an earlier attempt or an older run does
//     not count;
//   - CHANGELOG.md has a section for the version, a ## v<Version> heading, which for a release
//     that is not a release candidate no longer says (unreleased);
//   - tools/e2e/pending.txt has no line other than blank lines and comments, lines whose first
//     field starts with #: every public method is covered by an end-to-end scenario, local-only
//     or advisory;
//   - -evidence, the harness evidence, the summary of the report of the maintainer-run local
//     full-stack suite, is a JSON object {"sdk_commit": ..., "report_sha256": ..., "lanes": [...],
//     "passed_phases": [...]}, whose sdk_commit is -sha; whose lanes include the Go release of
//     each test leg of CI, go plus the value of each GO_FLOOR and GO_<n> variable of the env of
//     .github/workflows/ci.yml, such as go1.26.0; and whose passed_phases include the phase of
//     each line of tools/e2e/local-only.txt, the methods that the cloud lane cannot call. The
//     workflow cannot read the report itself: the reviewer of the release environment confirms
//     its report_sha256, which validate prints, against the report.
//
// # e2e
//
// e2e runs after the gates, and checks that the end-to-end job of the run proved the release:
// -jobs, the jobs of the run as the GitHub API lists them (repos/<repo>/actions/runs/<id>/jobs,
// one or more pages), has the job -ci-job, the CI job of the called workflow, and the job
// -e2e-job, its end-to-end job with the pinned PDP, each concluded success; and -record, the
// request record that the end-to-end job uploaded, holds a session trailer with exit status 0
// and at least as many live requests as tools/e2e/required.txt lists scenarios, its subtests (a
// count sentinel).
//
// # notes
//
// notes prints the release notes of -tag: the body of the section of CHANGELOG.md for its
// version. With -surface, the output of apisurface -mode release, it adds the incompatible
// changes that gorelease listed, which a release after a release candidate may make.
//
// # watchdog
//
// watchdog runs in .github/workflows/tag-watchdog.yml on a push of a tag and twice a week, and
// checks that each version tag was published by the release rules. -repo is the repository,
// <owner>/<name>. Four inputs are lists of the GitHub API, each one or more pages, one after the
// other, as gh api --paginate prints them: -tags, the tags (repos/<repo>/tags), each with the
// commit it names; -release-runs, the runs of release.yml
// (repos/<repo>/actions/workflows/release.yml/runs); -releases, the GitHub releases
// (repos/<repo>/releases); and -ci-runs, the runs of ci.yml for a push of the commit of each v1
// tag (repos/<repo>/actions/workflows/ci.yml/runs?head_sha=<sha>&event=push). -attestations is
// what the workflow found for the module zip of each published v2 release, a JSON array of
// {"tag": ..., "sha256": ..., "error": ..., "verified": [...]}: the SHA-256 of the release's
// asset permit-golang-<tag>.zip, and what gh attestation verify --format json printed for it, or
// why the zip could not be downloaded or verified. It checks:
//
//   - a tag that .github/accepted-tags.txt lists, <tag> <commit> <reason> per line, names that
//     commit; the file accepts the tags published before the release rules, a v1 tag that
//     passed once its runs are near the end of their retention, and a tag created by hand once
//     a maintainer handled it, as Releases in CONTRIBUTING.md says;
//   - any other v2 tag has a published GitHub release whose module zip has a build provenance
//     attestation (https://slsa.dev/provenance/v1) signed by .github/workflows/release.yml of
//     -repo at refs/heads/master, from that ref, for the tag's commit, whose subjects include the
//     zip by its name and SHA-256. Workflow runs are deleted after the retention period, and
//     attestations are kept, so a run only says why a tag has none: while a run of release.yml
//     named release <tag>, from master by workflow_dispatch, for the tag's commit, is
//     in_progress, as when publish has just created the tag and verify has not attested the zip,
//     the tag is pending and passes, and the next run checks it; when such a run completed, the
//     failure names it and its conclusion; and without one, the tag was created outside
//     release.yml;
//   - any other v1 tag has a published GitHub release, and a run of ci.yml for a push of its
//     commit that completed with success in its latest attempt, on master for a tag up to
//     v1.3.0, the last v1 version released from master, and on release/v1 for a later one;
//   - no other tag that is a semantic version, such as v0.0.20 or v3.0.0, exists: no workflow
//     publishes it.
//
// A tag that is not a semantic version, such as 1.2.1, is no version of the module, and is not
// checked.
//
// # lanes
//
// lanes prints the Go releases that validate requires in the harness evidence's lanes, one per
// line: go plus the value of each GO_FLOOR and GO_<n> variable of the env of
// .github/workflows/ci.yml. .github/scripts/release-dryrun.sh makes up its evidence with them.
//
// # Exit codes
//
//	0  every check passed, or the notes or the lanes were printed
//	1  a check failed
//	2  releasecheck did not run: the command line is wrong; a file cannot be read or parsed,
//	   such as -ci-runs, -jobs, -record or -tags, or a list of the GitHub API holds no page; the
//	   harness evidence is empty or not the JSON object above, or its sdk_commit or report_sha256
//	   is not a hash; ci.yml has no GO_FLOOR or GO_<n> variable; tools/e2e/required.txt lists no
//	   subtest; a line of .github/accepted-tags.txt is not <tag> <commit> <reason>, or a tag is
//	   on it twice; -attestations lists a zip without a tag, or without either an error or its
//	   SHA-256 and verified attestations, or a tag twice; or the notes find no section
//
// The last line validate, e2e and watchdog print is the verdict: PASS, FAIL or DID NOT RUN.
// Build it before use, because go run reports every non-zero exit as 1:
//
//	go -C tools build -o ../bin/releasecheck ./cmd/releasecheck
//	bin/releasecheck validate -tag v2.1.0 -sha "$sha" -master "$sha" -ci-runs ci-runs.json \
//	    -evidence evidence.json
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const (
	exitPassed    = 0
	exitFailed    = 1
	exitDidNotRun = 2
)

var (
	// tagPattern matches a v2 release tag as a whole string, and captures the release
	// candidate's suffix.
	tagPattern = regexp.MustCompile(`^v2\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-rc\.[1-9][0-9]*)?$`)
	// commitPattern matches a full commit hash.
	commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// usage is the command line, for messages.
const usage = "usage: releasecheck validate|e2e|notes|watchdog|lanes [flags]; " +
	"go -C tools doc ./cmd/releasecheck describes each"

// run runs the subcommand that args name and returns the exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return abort(stdout, errors.New(usage))
	}
	switch args[0] {
	case "validate":
		return runValidate(args[1:], stdout, stderr)
	case "e2e":
		return runE2E(args[1:], stdout, stderr)
	case "notes":
		return runNotes(args[1:], stdout, stderr)
	case "watchdog":
		return runWatchdog(args[1:], stdout, stderr)
	case "lanes":
		return runLanes(args[1:], stdout, stderr)
	}
	return abort(stdout, fmt.Errorf("unknown subcommand %q; %s", args[0], usage))
}

// newFlags returns a flag set for a subcommand with the -root flag.
func newFlags(name string, stderr io.Writer, root *string) *flag.FlagSet {
	flags := flag.NewFlagSet("releasecheck "+name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(root, "root", ".", "the repository root `directory`")
	return flags
}

// parse parses args with flags, and fails when an argument is left over or a required flag is
// empty.
func parse(flags *flag.FlagSet, args []string, required map[string]*string) error {
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected arguments %q", flags.Args())
	}
	var missing []string
	flags.VisitAll(func(f *flag.Flag) {
		if value, ok := required[f.Name]; ok && *value == "" {
			missing = append(missing, "-"+f.Name)
		}
	})
	if len(missing) > 0 {
		return fmt.Errorf("%s needs %s", flags.Name(), strings.Join(missing, ", "))
	}
	return nil
}

// abort prints why releasecheck did not run, then the verdict, and returns exitDidNotRun.
func abort(stdout io.Writer, reason error) int {
	fmt.Fprintf(stdout, "releasecheck: DID NOT RUN: %v\n", reason)
	fmt.Fprintln(stdout, "releasecheck: DID NOT RUN")
	return exitDidNotRun
}

// readPages reads the JSON values of the file, one after the other, as gh api --paginate prints
// the pages of a list of the GitHub API, into values of type P. what describes the list, for the
// error that a file without a value returns.
func readPages[P any](file, what string) ([]P, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var pages []P
	for {
		var page P
		err := dec.Decode(&page)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		pages = append(pages, page)
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("%s is empty: it is not %s", file, what)
	}
	return pages, nil
}

// plural returns n and the noun, plural unless n is 1.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// report collects what a check found.
type report struct {
	out      io.Writer
	failures []string
}

// passf prints a check that passed.
func (r *report) passf(format string, args ...any) {
	fmt.Fprintf(r.out, "releasecheck: ok: %s\n", fmt.Sprintf(format, args...))
}

// notef prints a line that is neither a check that passed nor one that failed, after its label.
func (r *report) notef(label, format string, args ...any) {
	fmt.Fprintf(r.out, "releasecheck: %s: %s\n", label, fmt.Sprintf(format, args...))
}

// failf records a check that failed.
func (r *report) failf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

// finish prints the failures and the verdict, and returns the exit code.
func (r *report) finish() int {
	for _, f := range r.failures {
		fmt.Fprintf(r.out, "releasecheck: FAIL: %s\n", f)
	}
	if len(r.failures) > 0 {
		fmt.Fprintln(r.out, "releasecheck: FAIL")
		return exitFailed
	}
	fmt.Fprintln(r.out, "releasecheck: PASS")
	return exitPassed
}
