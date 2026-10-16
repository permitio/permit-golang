// Command releasecheck checks a release of the SDK in the release workflow,
// .github/workflows/release.yml, before the workflow publishes it, and writes its release notes.
//
// Usage:
//
//	releasecheck validate -tag tag -sha sha -master sha -ci-runs file -evidence file [-root dir]
//	releasecheck e2e -jobs file -ci-job name -e2e-job name -record file [-root dir]
//	releasecheck notes -tag tag [-surface file] [-root dir]
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
// # Exit codes
//
//	0  every check passed, or the notes were printed
//	1  a check failed
//	2  releasecheck did not run: the command line is wrong; a file cannot be read or parsed,
//	   such as -ci-runs, -jobs or -record; the harness evidence is empty or not the JSON object
//	   above, or its sdk_commit or report_sha256 is not a hash; ci.yml has no GO_FLOOR or GO_<n>
//	   variable; tools/e2e/required.txt lists no subtest; or the notes find no section
//
// The last line validate and e2e print is the verdict: PASS, FAIL or DID NOT RUN. Build it
// before use, because go run reports every non-zero exit as 1:
//
//	go -C tools build -o ../bin/releasecheck ./cmd/releasecheck
//	bin/releasecheck validate -tag v2.1.0 -sha "$sha" -master "$sha" -ci-runs ci-runs.json \
//	    -evidence evidence.json
package main

import (
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
const usage = "usage: releasecheck validate|e2e|notes [flags]; " +
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
