// Command checkgotest reads `go test -json` output, prints the test output, and fails a test run
// that did less than it should. go test exits 0 when a test skips, when -run matches no test and
// when a build constraint leaves a package out, so a run that tested nothing can pass.
//
// checkgotest fails a run in which no test passed or no test ran in one of the input files. A
// package that -run filters out or a build constraint leaves out, in an input in which other
// tests ran, is caught only when one of its tests is required, such as by a line in
// .github/required-tests.txt.
//
// Usage:
//
//	checkgotest [-require 'import/path TestName']... [-require-file file] [-allow-skips file]
//		[file ...]
//
// checkgotest reads the events from each file in turn, or from stdin when no file is named, so
// the output of several go test runs, such as one per module, can be checked together.
//
// A required test is named by its package import path and its name, separated by a space, the
// way checkgotest lists tests. Naming the package keeps a test of the same name in another
// package from standing in for it. -require names one required test and can be repeated;
// -require-file names a file of required tests, one per line.
//
// No test may skip unless -allow-skips names a file with an entry for it. Each line of that file
// names a test, the ticket that tracks the skip, the last day the skip is allowed (UTC), and what
// must happen before the test can run again:
//
//	import/path TestName TICKET-123 2026-12-31 the API returns the field the test checks
//
// In both files, blank lines and lines that start with # are ignored.
//
// Exit codes:
//
//	0  every test passed or skipped with an allowed-skips entry, at least one test passed, and
//	   every required test ran and passed
//	1  a test failed or skipped without an unexpired allowed-skips entry, a package failed to
//	   build or run, a required test failed or skipped, or an allowed-skips entry expired or its
//	   test did not skip
//	2  no test passed, no test ran in one of the inputs, a required test never ran, the input is
//	   not go test -json output, or the command line or a list file is wrong
//
// The last line checkgotest prints is its verdict: PASS, FAIL or DID NOT RUN.
//
// Build it before use, because go run reports every non-zero exit as 1. Run the tests of both
// modules and check their output together:
//
//	go -C tools build -o ../bin/checkgotest ./cmd/checkgotest
//	go test -race -count=1 -shuffle=on -json ./... >bin/root.json
//	go -C tools test -race -count=1 -shuffle=on -json ./... >bin/tools.json
//	bin/checkgotest -require-file .github/required-tests.txt \
//		-allow-skips .github/allowed-skips.txt bin/root.json bin/tools.json
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	exitPassed    = 0
	exitFailed    = 1
	exitDidNotRun = 2
)

// maxLineBytes bounds one line of go test -json output; a test that logs a large request or
// response body can write a long line.
const maxLineBytes = 16 << 20

// ticketPattern matches an issue ID such as TICKET-123.
var ticketPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*-[1-9][0-9]*$`)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, time.Now()))
}

// input is a stream of go test -json events and the name to report it by.
type input struct {
	name string
	r    io.Reader
}

// config is what checkgotest checks the test results against.
type config struct {
	// required lists the tests that must run and pass, as "import/path TestName".
	required []string
	// allowed lists the tests that may skip, read from allowedFile.
	allowed     []allowedSkip
	allowedFile string
	// today is the current date in UTC; an allowed skip expires after its expiry date.
	today time.Time
}

// allowedSkip is one entry of an allowed-skips file.
type allowedSkip struct {
	test      string
	ticket    string
	expiry    time.Time
	condition string
}

// run parses the command line, reads the list files and checks the go test -json events in the
// named files or on stdin. A command line or list file it cannot use counts as not having run, so
// a mistyped CI step cannot pass.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer, now time.Time) int {
	flags := flag.NewFlagSet("checkgotest", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: checkgotest [flags] [go-test-json-file ...]")
		flags.PrintDefaults()
	}
	var required []string
	flags.Func("require",
		"a test that must run and pass, as `'import/path TestName'` (repeatable)",
		func(value string) error {
			test, err := parseTestName(value)
			if err != nil {
				return err
			}
			required = append(required, test)
			return nil
		})
	requireFile := flags.String("require-file", "",
		"a `file` of tests that must run and pass, one 'import/path TestName' per line")
	allowFile := flags.String("allow-skips", "",
		"a `file` of tests that may skip, one 'import/path TestName TICKET-123 YYYY-MM-DD "+
			"un-skip condition' per line")
	if err := flags.Parse(args); err != nil {
		return abort(stdout, err)
	}

	cfg, err := loadConfig(required, *requireFile, *allowFile)
	if err != nil {
		return abort(stdout, err)
	}
	year, month, day := now.UTC().Date()
	cfg.today = time.Date(year, month, day, 0, 0, 0, 0, time.UTC)

	inputs := []input{{name: "stdin", r: stdin}}
	if flags.NArg() > 0 {
		files, err := openAll(flags.Args())
		defer closeAll(files)
		if err != nil {
			return abort(stdout, err)
		}
		inputs = make([]input, 0, len(files))
		for _, file := range files {
			inputs = append(inputs, input{name: file.Name(), r: file})
		}
	}
	return check(inputs, stdout, cfg)
}

// abort prints why checkgotest stopped before it checked the test results, then the verdict, and
// returns exitDidNotRun.
func abort(output io.Writer, reason error) int {
	fmt.Fprintf(output, "checkgotest: DID NOT RUN: %v\n", reason)
	fmt.Fprintln(output, "checkgotest: DID NOT RUN")
	return exitDidNotRun
}

// openAll opens every named file before any is read, so a missing file stops the run before it
// prints any test output. It returns the files it opened, also on an error.
func openAll(names []string) ([]*os.File, error) {
	var files []*os.File
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return files, err
		}
		files = append(files, file)
	}
	return files, nil
}

// closeAll closes files that were only read, so a close error cannot lose data.
func closeAll(files []*os.File) {
	for _, file := range files {
		_ = file.Close()
	}
}

// loadConfig reads the list files and merges the required tests named on the command line with
// those in requireFile.
func loadConfig(required []string, requireFile, allowFile string) (config, error) {
	cfg := config{required: required, allowedFile: allowFile}
	if requireFile != "" {
		fromFile, err := readRequired(requireFile)
		if err != nil {
			return config{}, err
		}
		cfg.required = append(cfg.required, fromFile...)
	}
	named := map[string]bool{}
	for _, test := range cfg.required {
		if named[test] {
			return config{}, fmt.Errorf("required test %q is named twice", test)
		}
		named[test] = true
	}
	if allowFile != "" {
		allowed, err := readAllowedSkips(allowFile)
		if err != nil {
			return config{}, err
		}
		cfg.allowed = allowed
	}
	return cfg, nil
}

// parseTestName parses "import/path TestName" and returns it with a single space.
func parseTestName(value string) (string, error) {
	fields := strings.Fields(value)
	if len(fields) != 2 {
		return "", fmt.Errorf("want an import path and a test name separated by a space, got %q",
			value)
	}
	return fields[0] + " " + fields[1], nil
}

// readRequired reads a required-tests file.
func readRequired(path string) ([]string, error) {
	var tests []string
	err := readList(path, func(line string) error {
		test, err := parseTestName(line)
		if err != nil {
			return err
		}
		tests = append(tests, test)
		return nil
	})
	return tests, err
}

// readAllowedSkips reads an allowed-skips file.
func readAllowedSkips(path string) ([]allowedSkip, error) {
	var entries []allowedSkip
	named := map[string]bool{}
	err := readList(path, func(line string) error {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return fmt.Errorf("want 'import/path TestName TICKET-123 YYYY-MM-DD un-skip "+
				"condition', got %q", line)
		}
		entry := allowedSkip{
			test:      fields[0] + " " + fields[1],
			ticket:    fields[2],
			condition: strings.Join(fields[4:], " "),
		}
		if !ticketPattern.MatchString(entry.ticket) {
			return fmt.Errorf("ticket %q is not an issue ID such as TICKET-123", entry.ticket)
		}
		expiry, err := time.Parse(time.DateOnly, fields[3])
		if err != nil {
			return fmt.Errorf("expiry %q is not a date such as 2026-12-31", fields[3])
		}
		entry.expiry = expiry
		if named[entry.test] {
			return fmt.Errorf("%q is named twice", entry.test)
		}
		named[entry.test] = true
		entries = append(entries, entry)
		return nil
	})
	return entries, err
}

// readList calls parse for each line of the file at path that is not blank or a comment, and
// returns the first error with the file name and line number.
func readList(path string, parse func(line string) error) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if err := parse(line); err != nil {
			return fmt.Errorf("%s:%d: %w", path, i+1, err)
		}
	}
	return nil
}

// testEvent is the part of a go test -json event (see `go doc test2json`) that checkgotest reads.
type testEvent struct {
	Action      string
	Package     string
	Test        string
	Output      string
	FailedBuild string
}

// results is what the events reported.
type results struct {
	passed, failed, skipped []string
	failedPackages          []failedPackage
	// testFailedIn holds the packages in which a test failed.
	testFailedIn         map[string]bool
	started, passedNames map[string]bool
	// inputsWithoutTests names the inputs that have no event for a test.
	inputsWithoutTests []string
	malformedLines     int
}

// failedPackage is a package whose go test run failed.
type failedPackage struct {
	name string
	// build is whether the failure was a build failure, which go test -json reports in the
	// FailedBuild field.
	build bool
}

// check reads the events from each input in turn, echoes the test output, prints what it found
// and returns the exit code.
func check(inputs []input, output io.Writer, cfg config) int {
	r := results{
		testFailedIn: map[string]bool{}, started: map[string]bool{},
		passedNames: map[string]bool{},
	}
	for _, in := range inputs {
		sawTest, err := r.read(in.r, output)
		if err != nil {
			r.printCounts(output)
			return abort(output, fmt.Errorf("reading %s: %w", in.name, err))
		}
		if !sawTest {
			r.inputsWithoutTests = append(r.inputsWithoutTests, in.name)
		}
	}
	return r.verdict(output, cfg)
}

// read echoes the test output from the events on input, sorts the test results, and returns
// whether any event was for a test.
func (r *results) read(input io.Reader, output io.Writer) (bool, error) {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(nil, maxLineBytes)
	sawTest := false
	for scanner.Scan() {
		var event testEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil || event.Action == "" {
			fmt.Fprintf(output, "%s\n", scanner.Bytes())
			r.malformedLines++
			continue
		}
		fmt.Fprint(output, event.Output)
		if event.Test == "" {
			if event.Action == "fail" {
				r.failedPackages = append(r.failedPackages,
					failedPackage{name: event.Package, build: event.FailedBuild != ""})
			}
			continue
		}
		sawTest = true
		name := event.Package + " " + event.Test
		r.started[name] = true
		switch event.Action {
		case "pass":
			r.passed = append(r.passed, name)
			r.passedNames[name] = true
		case "fail":
			r.failed = append(r.failed, name)
			r.testFailedIn[event.Package] = true
		case "skip":
			r.skipped = append(r.skipped, name)
		}
	}
	return sawTest, scanner.Err()
}

// packageFailures returns the packages that failed to build, and those that failed with no failed
// test of their own, such as when TestMain exits with a non-zero code. A package in which a test
// failed is reported by that test.
func (r *results) packageFailures() []string {
	var names []string
	for _, pkg := range r.failedPackages {
		if pkg.build || !r.testFailedIn[pkg.name] {
			names = append(names, pkg.name)
		}
	}
	return names
}

func (r *results) printCounts(output io.Writer) {
	fmt.Fprintf(output, "checkgotest: %d passed, %d failed, %d skipped\n",
		len(r.passed), len(r.failed), len(r.skipped))
}

// verdict prints the counts and every problem it finds, then the verdict, and returns the exit
// code. A failure wins over not having run.
func (r *results) verdict(output io.Writer, cfg config) int {
	r.printCounts(output)
	var missing, notPassed []string
	for _, name := range cfg.required {
		switch {
		case !r.started[name]:
			missing = append(missing, name)
		case !r.passedNames[name]:
			notPassed = append(notPassed, name)
		}
	}
	if len(cfg.required) > 0 {
		fmt.Fprintf(output, "checkgotest: %d of %d required tests passed\n",
			len(cfg.required)-len(missing)-len(notPassed), len(cfg.required))
	}
	skips := sortSkips(r.skipped, cfg)
	report(output, "skips allowed by "+cfg.allowedFile, skips.allowed)

	failed := report(output, "tests failed", r.failed)
	failed = report(output, "packages failed to build or run", r.packageFailures()) || failed
	skipHeading := "tests skipped; this run allows no skips"
	if cfg.allowedFile != "" {
		skipHeading = "tests skipped without an unexpired entry in " + cfg.allowedFile
	}
	failed = report(output, skipHeading, skips.notAllowed) || failed
	failed = report(output, "required tests did not pass", notPassed) || failed
	failed = report(output, "entries in "+cfg.allowedFile+
		" that expired; fix the test or renew the entry", skips.expired) || failed
	failed = report(output, "entries in "+cfg.allowedFile+
		" whose test did not skip; remove them", skips.stale) || failed

	didNotRun := report(output, "DID NOT RUN: required tests never ran", missing)
	if len(r.passed) == 0 {
		fmt.Fprintln(output, "checkgotest: DID NOT RUN: no test passed")
		didNotRun = true
	}
	didNotRun = report(output, "DID NOT RUN: no test ran in", r.inputsWithoutTests) || didNotRun
	if r.malformedLines > 0 {
		fmt.Fprintf(output,
			"checkgotest: DID NOT RUN: %d input lines are not go test -json events\n",
			r.malformedLines)
		didNotRun = true
	}

	switch {
	case failed:
		fmt.Fprintln(output, "checkgotest: FAIL")
		return exitFailed
	case didNotRun:
		fmt.Fprintln(output, "checkgotest: DID NOT RUN")
		return exitDidNotRun
	}
	fmt.Fprintln(output, "checkgotest: PASS")
	return exitPassed
}

// skipResults sorts the skipped tests and the allowed-skips entries.
type skipResults struct {
	// allowed and notAllowed are the skipped tests with and without an unexpired entry.
	allowed, notAllowed []string
	// expired and stale are the entries that expired and the unexpired entries whose test did not
	// skip.
	expired, stale []string
}

func sortSkips(skipped []string, cfg config) skipResults {
	var s skipResults
	entries := map[string]allowedSkip{}
	for _, entry := range cfg.allowed {
		entries[entry.test] = entry
	}
	didSkip := map[string]bool{}
	for _, name := range skipped {
		didSkip[name] = true
		entry, ok := entries[name]
		if !ok || cfg.today.After(entry.expiry) {
			s.notAllowed = append(s.notAllowed, name)
			continue
		}
		s.allowed = append(s.allowed, fmt.Sprintf("%s (%s, until %s): %s",
			name, entry.ticket, entry.expiry.Format(time.DateOnly), entry.condition))
	}
	for _, entry := range cfg.allowed {
		switch {
		case cfg.today.After(entry.expiry):
			s.expired = append(s.expired, fmt.Sprintf("%s (%s, expired %s)",
				entry.test, entry.ticket, entry.expiry.Format(time.DateOnly)))
		case !didSkip[entry.test]:
			s.stale = append(s.stale, fmt.Sprintf("%s (%s)", entry.test, entry.ticket))
		}
	}
	return s
}

// report prints the names under a heading and returns whether there were any.
func report(output io.Writer, heading string, names []string) bool {
	if len(names) == 0 {
		return false
	}
	fmt.Fprintf(output, "checkgotest: %s:\n    %s\n", heading, strings.Join(names, "\n    "))
	return true
}
