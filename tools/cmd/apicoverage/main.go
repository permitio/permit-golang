// Command apicoverage reports which operations of the Permit API and the PDP the SDK covers,
// measured from the requests that its tests send, and checks the decisions of
// tools/specs/coverage.yaml against the API specs, the operation map and those requests.
//
// Usage:
//
//	apicoverage [-specs dir] [-live dir] [-map file] [-local file] [-local-only file]
//	            [-advisory file] [-release] [-summary file] record...
//
// Each record is a request record in the JSONL format that internal/testhook describes, such as
// the one the root package's tests write when PERMIT_API_COVERAGE_RECORD names a file. Its
// requests are those of the offline wire tests (the mode wire), and of the end-to-end scenarios
// run offline (dryrun) or against the Permit cloud (live). The test count of its session trailer
// is the number of tests that wrote to the record, not of every test that ran.
//
//   - -specs names the directory of the specs and of coverage.yaml, tools/specs by default.
//   - -live names a directory that holds snapshots of the live control-plane.json and
//     pdp-cloud.json, such as the one specsnap -check-live -out writes, to read instead of the
//     committed ones.
//   - -map names the operation map, testdata/operation-map.json by default: the operations that
//     the wire cases of each public method send, and in facts_via_pdp the routes under /facts
//     that those of a method of the facts API send to the PDP container, which TestOperationMap
//     keeps. A row of pdp names each method that sends one of its routes through the container.
//   - -local names the record of a run against a full local stack: a record in the same JSONL
//     format, whose requests all have the mode local, as that run will write it.
//   - -local-only names the list of the methods that a run against a full local stack proves
//     instead of the cloud lane, tools/e2e/local-only.txt: one <Type>.<Method>, a space and the
//     reason and phase per line; blank lines and lines that start with # are ignored.
//   - -advisory names the list of the methods whose end-to-end scenario is advisory: one
//     <Type>.<Method>, a space and the reason per line; blank lines and lines that start with #
//     are ignored. A method of the facts API on this list or on -local-only also marks each row
//     of pdp that it sends through the PDP container, for most of them the catch-all
//     ANY /facts/{full_path}, so that one entry waives the row's live proof under -release.
//   - -release checks the coverage of a release, as described below.
//   - -summary names a file to append the report to in Markdown, such as $GITHUB_STEP_SUMMARY.
//
// A request matches the row of coverage.yaml for its target, control-plane for the API and pdp
// for the PDP, whose route takes its method and whose template matches its path. A parameter
// matches one segment that is not empty, and {full_path} at the end of a template one or more;
// a literal segment matches the segment with its percent escapes decoded, so a parameter's
// value may hold an escaped /. When more than one template matches, the one with the most
// literal segments wins, then the one without {full_path}, then the one whose literal segment
// comes first. A path of the PDP under a mount of pdp-container.extra.json, such as
// /v2/facts/{proj_id}/{env_id}/users, matches as the path it stands for, /facts/users.
//
// Each covered row gets an end-to-end state: cloud when a live request reached it, else local
// when the -local record did, else local-only when one of its methods is on the -local-only
// list, else advisory when one is on the -advisory list, else dry run when only an end-to-end
// scenario run offline did, else none. A dry-run request is no end-to-end evidence, and the
// report says so when the records hold no live request.
//
// apicoverage fails, exit 1, on each of these:
//
//   - with -release: a planned row, as a release covers every operation it plans, and a covered
//     row whose end-to-end state is dry run or none. The release workflow passes the records of
//     the wire tests and of its own e2e job, and the release needs each local-only method's phase
//     in the harness evidence, which tools/cmd/releasecheck checks;
//   - an undecided GA operation: a GA operation of a spec without a row;
//   - a stale row: a planned row that a request or a wire case reaches, which must be covered
//     now, or an excluded or deferred row that one reaches;
//   - an untested row: a covered row that no request of the records reaches;
//   - a stage change: a row whose stage is not the one the spec gives the operation;
//   - an unexplained SDK-only request: a request that matches no row;
//   - an unused sdk_only entry: a covered row with sdk_only that no request reaches, or one
//     whose route a spec lists now, and an entry of sdk_only_params for a parameter that the
//     spec lists now or an operation that the SDK does not send;
//   - a row whose route the specs no longer list, such as an excluded operation gone from the
//     spec, or whose op is not the spec's operationId;
//   - a request for an operation that v1.2.8 had, to a route of another shape than the one in
//     v1-routes.json, which v1-route-fixes.yaml does not list as the route v2 sends instead:
//     routes of one shape differ at most in the names of their parameters;
//   - a request whose template does not fit its path, or is the route of another row than the
//     one its path reaches;
//   - a request to a test-only route, whose path starts with /__test/;
//   - an excluded row whose reason is not a key of reasons, a key of reasons that no row gives,
//     an operation of the operation map that no row has, and a route of its facts_via_pdp that
//     no row of pdp takes.
//
// It did not run, exit 2, when the command line is wrong; a file cannot be read or parsed;
// coverage.yaml does not have the shape its header describes; a route of facts_via_pdp of the
// operation map is not a method, a space and a path under /facts/; v1-routes.json names an
// operation that is not the op of a control-plane row; a record is malformed, such as a path with
// a query, has no session trailer, comes from a run whose tests failed or to which no test wrote,
// holds a request of a mode it must not, or the records hold fewer requests than coverage.yaml has
// covered rows, which means that tests did not run; or the summary cannot be written.
//
// The last line apicoverage prints is its verdict: PASS, FAIL or DID NOT RUN, a colon and what
// it found. Build it before use, because go run reports every non-zero exit as 1:
//
//	go -C tools build -o ../bin/apicoverage ./cmd/apicoverage
//	bin/apicoverage record.jsonl
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	exitPassed    = 0
	exitFailed    = 1
	exitDidNotRun = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// errNoRecord says that no record was given.
var errNoRecord = errors.New("no request record given; name the record files after the flags")

// options is the command line.
type options struct {
	specs, live, opMap, local, localOnly, advisory, summary string
	release                                                 bool
	records                                                 []string
}

// run runs apicoverage and returns its exit code.
func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseArgs(args, stderr)
	if err != nil {
		return didNotRun(stdout, err)
	}
	in, err := readInputs(opts)
	if err != nil {
		return didNotRun(stdout, err)
	}
	res := check(in)
	sources := "coverage.yaml and the specs in " + opts.specs
	if opts.live != "" {
		sources += ", with the live control-plane and cloud PDP specs in " + opts.live
	}
	if opts.summary != "" {
		var md bytes.Buffer
		writeMarkdown(&md, res, sources)
		if err := appendFile(opts.summary, md.Bytes()); err != nil {
			return didNotRun(stdout, fmt.Errorf("writing the summary: %w", err))
		}
	}
	writeText(stdout, res, sources)
	switch verdict, _ := res.verdict(); verdict {
	case verdictPass:
		return exitPassed
	case verdictFail:
		return exitFailed
	}
	return exitDidNotRun
}

// didNotRun prints the verdict of a run that did not run, and returns its exit code.
func didNotRun(stdout io.Writer, err error) int {
	fmt.Fprintf(stdout, "%s: %v\n", verdictDidNotRun, err)
	return exitDidNotRun
}

// parseArgs parses the command line.
func parseArgs(args []string, stderr io.Writer) (options, error) {
	var opts options
	flags := flag.NewFlagSet("apicoverage", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.specs, "specs", "tools/specs",
		"the `directory` of the specs and coverage.yaml")
	flags.StringVar(&opts.live, "live", "",
		"a `directory` with the live control-plane.json and pdp-cloud.json")
	flags.StringVar(&opts.opMap, "map", operationMapFile, "the operation map `file`")
	flags.StringVar(&opts.local, "local", "",
		"the record `file` of a run against a local stack, in the JSONL format, mode local")
	flags.StringVar(&opts.localOnly, "local-only", "",
		"the `file` that lists the methods that a run against a full local stack proves")
	flags.StringVar(&opts.advisory, "advisory", "",
		"the `file` that lists the methods whose end-to-end scenario is advisory")
	flags.BoolVar(&opts.release, "release", false,
		"fail on a planned row, and on a covered row without end-to-end evidence")
	flags.StringVar(&opts.summary, "summary", "", "a `file` to append the Markdown report to")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	opts.records = flags.Args()
	if len(opts.records) == 0 {
		return options{}, errNoRecord
	}
	for name, value := range map[string]string{"-specs": opts.specs, "-map": opts.opMap} {
		if value == "" {
			return options{}, fmt.Errorf("%s is empty", name)
		}
	}
	for _, file := range opts.records {
		if strings.HasPrefix(file, "-") {
			return options{}, fmt.Errorf("the flag %s comes after a record; give the flags first",
				file)
		}
	}
	return opts, nil
}

// readInputs reads what the command line names.
func readInputs(opts options) (inputs, error) {
	var in inputs
	var err error
	if in.inv, err = readInventory(opts.specs, opts.live); err != nil {
		return inputs{}, err
	}
	if in.cov, err = readCoverage(filepath.Join(opts.specs, coverageFile)); err != nil {
		return inputs{}, err
	}
	if in.seed, err = readV1Seed(opts.specs, in.cov); err != nil {
		return inputs{}, err
	}
	if in.ops, err = readOperationMap(opts.opMap); err != nil {
		return inputs{}, err
	}
	if opts.advisory != "" {
		if in.advisory, err = readMethods(opts.advisory); err != nil {
			return inputs{}, err
		}
	}
	if opts.localOnly != "" {
		if in.localOnly, err = readMethods(opts.localOnly); err != nil {
			return inputs{}, err
		}
	}
	in.release = opts.release
	for _, file := range opts.records {
		rec, recErr := readRecord(file, []string{modeWire, modeDryRun, modeLive})
		if recErr != nil {
			return inputs{}, recErr
		}
		in.records = append(in.records, rec)
	}
	if opts.local != "" {
		if in.local, err = readRecord(opts.local, []string{modeLocal}); err != nil {
			return inputs{}, err
		}
	}
	return in, nil
}

// appendFile appends data to the file, creating it when it is missing.
func appendFile(file string, data []byte) error {
	//nolint:gosec // G302: a job summary that the runner reads.
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	return errors.Join(writeErr, f.Close())
}
