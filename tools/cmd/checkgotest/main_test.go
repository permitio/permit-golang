package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Events as go test -json writes them, one per line.
const (
	pkgNoTests  = `{"Action":"skip","Package":"m/empty"}`
	runA        = `{"Action":"run","Package":"m/a","Test":"TestA"}`
	outA        = `{"Action":"output","Package":"m/a","Test":"TestA","Output":"--- PASS: TestA\n"}`
	passA       = `{"Action":"pass","Package":"m/a","Test":"TestA"}`
	runB        = `{"Action":"run","Package":"m/a","Test":"TestB"}`
	passB       = `{"Action":"pass","Package":"m/a","Test":"TestB"}`
	failB       = `{"Action":"fail","Package":"m/a","Test":"TestB"}`
	skipB       = `{"Action":"skip","Package":"m/a","Test":"TestB"}`
	runBSub     = `{"Action":"run","Package":"m/a","Test":"TestB/sub"}`
	skipBSub    = `{"Action":"skip","Package":"m/a","Test":"TestB/sub"}`
	pkgPass     = `{"Action":"pass","Package":"m/a"}`
	pkgFail     = `{"Action":"fail","Package":"m/a"}`
	buildOutput = `{"ImportPath":"m/b","Action":"build-output","Output":"b.go:1: syntax error\n"}`
	pkgFailB    = `{"Action":"fail","Package":"m/b","FailedBuild":"m/b"}`
	buildFailA  = `{"Action":"fail","Package":"m/a","FailedBuild":"m/a"}`
	noAction    = `{"Package":"m/a","Test":"TestA"}`
)

// outALarge is an output line longer than bufio.Scanner's default 64 KB limit, as a test that
// logs a large request body can write.
var outALarge = `{"Action":"output","Package":"m/a","Test":"TestA","Output":"` +
	strings.Repeat("x", 100<<10) + `\n"}`

// today is the date the tests check allowed skips against.
var today = time.Date(2026, time.October, 13, 0, 0, 0, 0, time.UTC)

// verdicts is the last line checkgotest prints for each exit code.
var verdicts = map[int]string{
	exitPassed:    "checkgotest: PASS\n",
	exitFailed:    "checkgotest: FAIL\n",
	exitDidNotRun: "checkgotest: DID NOT RUN\n",
}

func eventsInput(events ...string) []input {
	return []input{{name: "stdin", r: strings.NewReader(strings.Join(events, "\n"))}}
}

func allowSkip(test, expiry string) allowedSkip {
	date, err := time.Parse(time.DateOnly, expiry)
	if err != nil {
		panic(err)
	}
	return allowedSkip{test: test, ticket: "TICKET-1", expiry: date,
		condition: "the fixture passes"}
}

// assertResult checks the exit code, that the output contains want, and that the output ends
// with the counts' verdict line, so every exit path reports what it found.
func assertResult(t *testing.T, exit, wantExit int, output, want string) {
	t.Helper()
	if exit != wantExit {
		t.Errorf("exit = %d, want %d; output:\n%s", exit, wantExit, output)
	}
	if !strings.Contains(output, want) {
		t.Errorf("output does not contain %q:\n%s", want, output)
	}
	if !strings.Contains(output, " passed, ") {
		t.Errorf("output has no counts line:\n%s", output)
	}
	if !strings.HasSuffix(output, verdicts[wantExit]) {
		t.Errorf("output does not end with %q:\n%s", verdicts[wantExit], output)
	}
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name       string
		events     []string
		required   []string
		allowed    []allowedSkip
		wantExit   int
		wantOutput string
	}{
		{
			name:     "all passed",
			events:   []string{pkgNoTests, runA, outA, passA, runB, passB, pkgPass},
			required: []string{"m/a TestA"}, wantExit: exitPassed,
			wantOutput: "--- PASS: TestA\ncheckgotest: 2 passed, 0 failed, 0 skipped\n" +
				"checkgotest: 1 of 1 required tests passed\n",
		},
		{
			name:     "a test failed",
			events:   []string{runA, passA, runB, failB, pkgFail},
			wantExit: exitFailed,
			wantOutput: "checkgotest: tests failed:\n    m/a TestB\n" +
				"checkgotest: FAIL\n",
		},
		{
			name:     "a package failed with no failed test",
			events:   []string{runA, passA, pkgFail},
			wantExit: exitFailed,
			wantOutput: "checkgotest: 1 passed, 0 failed, 0 skipped\n" +
				"checkgotest: packages failed to build or run:\n    m/a\n",
		},
		{
			name:     "a package whose test failed in one run and whose build failed in another",
			events:   []string{runA, passA, runB, failB, pkgFail, buildFailA},
			wantExit: exitFailed,
			wantOutput: "checkgotest: tests failed:\n    m/a TestB\n" +
				"checkgotest: packages failed to build or run:\n    m/a\n" +
				"checkgotest: FAIL\n",
		},
		{
			name:     "a test skipped",
			events:   []string{runA, passA, runB, skipB, pkgPass},
			wantExit: exitFailed,
			wantOutput: "checkgotest: 1 passed, 0 failed, 1 skipped\n" +
				"checkgotest: tests skipped; this run allows no skips:\n    m/a TestB\n",
		},
		{
			name:     "a subtest skipped",
			events:   []string{runA, passA, runB, runBSub, skipBSub, passB, pkgPass},
			wantExit: exitFailed, wantOutput: "this run allows no skips:\n    m/a TestB/sub\n",
		},
		{
			name:     "a package failed to build",
			events:   []string{runA, passA, pkgPass, buildOutput, pkgFailB},
			wantExit: exitFailed,
			wantOutput: "b.go:1: syntax error\n" +
				"checkgotest: 1 passed, 0 failed, 0 skipped\n" +
				"checkgotest: packages failed to build or run:\n    m/b\n",
		},
		{
			name:     "a required test skipped",
			events:   []string{runA, passA, runB, skipB, pkgPass},
			required: []string{"m/a TestB"}, wantExit: exitFailed,
			wantOutput: "checkgotest: 1 passed, 0 failed, 1 skipped\n" +
				"checkgotest: 0 of 1 required tests passed\n" +
				"checkgotest: tests skipped; this run allows no skips:\n    m/a TestB\n" +
				"checkgotest: required tests did not pass:\n    m/a TestB\n",
		},
		{
			name:     "a required test skipped with an allowed-skips entry",
			events:   []string{runA, passA, runB, skipB, pkgPass},
			required: []string{"m/a TestB"},
			allowed:  []allowedSkip{allowSkip("m/a TestB", "2026-12-31")},
			wantExit: exitFailed, wantOutput: "required tests did not pass:\n    m/a TestB\n",
		},
		{
			name:     "a required test never ran",
			events:   []string{runA, passA, pkgPass},
			required: []string{"m/a TestA", "m/a TestB"}, wantExit: exitDidNotRun,
			wantOutput: "checkgotest: 1 of 2 required tests passed\n" +
				"checkgotest: DID NOT RUN: required tests never ran:\n    m/a TestB\n",
		},
		{
			name:     "a required test ran only in another package",
			events:   []string{runA, passA, pkgPass},
			required: []string{"m/other TestA"}, wantExit: exitDidNotRun,
			wantOutput: "DID NOT RUN: required tests never ran:\n    m/other TestA\n",
		},
		{
			name:       "an output line longer than 64 KB",
			events:     []string{runA, outALarge, passA, pkgPass},
			required:   []string{"m/a TestA"},
			wantExit:   exitPassed,
			wantOutput: "checkgotest: 1 passed, 0 failed, 0 skipped\n",
		},
		{
			name:     "no input",
			wantExit: exitDidNotRun,
			wantOutput: "checkgotest: 0 passed, 0 failed, 0 skipped\n" +
				"checkgotest: DID NOT RUN: no test passed\n" +
				"checkgotest: DID NOT RUN: no test ran in:\n    stdin\n",
		},
		{
			name:     "only packages without tests",
			events:   []string{pkgNoTests, pkgPass},
			wantExit: exitDidNotRun,
			wantOutput: "DID NOT RUN: no test passed\n" +
				"checkgotest: DID NOT RUN: no test ran in:\n    stdin\n",
		},
		{
			name:     "every test skipped with an allowed-skips entry",
			events:   []string{runB, skipB, pkgPass},
			allowed:  []allowedSkip{allowSkip("m/a TestB", "2026-12-31")},
			wantExit: exitDidNotRun,
			wantOutput: "checkgotest: DID NOT RUN: no test passed\n" +
				"checkgotest: DID NOT RUN\n",
		},
		{
			name:     "not go test -json output",
			events:   []string{runA, passA, "ok  \tm/a\t0.1s"},
			wantExit: exitDidNotRun,
			wantOutput: "ok  \tm/a\t0.1s\n" +
				"checkgotest: 1 passed, 0 failed, 0 skipped\n" +
				"checkgotest: DID NOT RUN: 1 input lines are not go test -json events\n",
		},
		{
			name:     "JSON that is not a go test -json event",
			events:   []string{runA, passA, noAction, pkgPass},
			wantExit: exitDidNotRun,
			wantOutput: noAction + "\n" + "checkgotest: 1 passed, 0 failed, 0 skipped\n" +
				"checkgotest: DID NOT RUN: 1 input lines are not go test -json events\n",
		},
		{
			name:     "failure wins over did not run",
			events:   []string{runB, failB, pkgFail},
			required: []string{"m/a TestA"}, wantExit: exitFailed,
			wantOutput: "tests failed:\n    m/a TestB\n" +
				"checkgotest: DID NOT RUN: required tests never ran:\n    m/a TestA\n" +
				"checkgotest: DID NOT RUN: no test passed\n",
		},
		{
			name:     "an allowed skip",
			events:   []string{runA, passA, runB, skipB, pkgPass},
			allowed:  []allowedSkip{allowSkip("m/a TestB", "2026-12-31")},
			wantExit: exitPassed,
			wantOutput: "checkgotest: 1 passed, 0 failed, 1 skipped\n" +
				"checkgotest: skips allowed by allowed-skips.txt:\n" +
				"    m/a TestB (TICKET-1, until 2026-12-31): the fixture passes\n" +
				"checkgotest: PASS\n",
		},
		{
			name:     "an allowed skip on its last day",
			events:   []string{runA, passA, runB, skipB, pkgPass},
			allowed:  []allowedSkip{allowSkip("m/a TestB", "2026-10-13")},
			wantExit: exitPassed, wantOutput: "    m/a TestB (TICKET-1, until 2026-10-13)",
		},
		{
			name:     "an allowed skip that expired",
			events:   []string{runA, passA, runB, skipB, pkgPass},
			allowed:  []allowedSkip{allowSkip("m/a TestB", "2026-10-12")},
			wantExit: exitFailed,
			wantOutput: "checkgotest: tests skipped without an unexpired entry in " +
				"allowed-skips.txt:\n    m/a TestB\n" +
				"checkgotest: entries in allowed-skips.txt that expired; fix the test or renew " +
				"the entry:\n    m/a TestB (TICKET-1, expired 2026-10-12)\n",
		},
		{
			name:     "an expired allowed skip whose test did not skip",
			events:   []string{runA, passA, runB, passB, pkgPass},
			allowed:  []allowedSkip{allowSkip("m/a TestB", "2026-10-12")},
			wantExit: exitFailed,
			wantOutput: "checkgotest: 2 passed, 0 failed, 0 skipped\n" +
				"checkgotest: entries in allowed-skips.txt that expired; fix the test or renew " +
				"the entry:\n    m/a TestB (TICKET-1, expired 2026-10-12)\ncheckgotest: FAIL\n",
		},
		{
			name:     "an allowed skip whose test did not skip",
			events:   []string{runA, passA, runB, passB, pkgPass},
			allowed:  []allowedSkip{allowSkip("m/a TestB", "2026-12-31")},
			wantExit: exitFailed,
			wantOutput: "checkgotest: entries in allowed-skips.txt whose test did not skip; " +
				"remove them:\n    m/a TestB (TICKET-1)\n",
		},
		{
			name:     "an allowed skip for a test that did not run",
			events:   []string{runA, passA, pkgPass},
			allowed:  []allowedSkip{allowSkip("m/other TestB", "2026-12-31")},
			wantExit: exitFailed, wantOutput: "remove them:\n    m/other TestB (TICKET-1)\n",
		},
		{
			name:     "an allowed skip for another test",
			events:   []string{runA, passA, runB, skipB, pkgPass},
			allowed:  []allowedSkip{allowSkip("m/other TestB", "2026-12-31")},
			wantExit: exitFailed,
			wantOutput: "tests skipped without an unexpired entry in allowed-skips.txt:\n" +
				"    m/a TestB\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config{required: tt.required, allowed: tt.allowed, today: today}
			if tt.allowed != nil {
				cfg.allowedFile = "allowed-skips.txt"
			}
			var output strings.Builder

			exit := check(eventsInput(tt.events...), &output, cfg)

			assertResult(t, exit, tt.wantExit, output.String(), tt.wantOutput)
		})
	}
}

// The files in testdata are go test -json output from a module example.com/fixture with three
// packages: a, with the tests TestA and TestB; empty, with no tests; and parallel, whose tests
// TestC and TestD call t.Parallel. TestB passed in pass.json, failed in fail.json and skipped in
// skip.json; no-tests.json ran go test with a -run pattern that matches no test, and not-json.txt
// is go test output without -json. parallel.json ran only the package parallel.
func TestCheckFixtures(t *testing.T) {
	const testA, testB = "example.com/fixture/a TestA", "example.com/fixture/a TestB"
	tests := []struct {
		fixture    string
		name       string
		required   []string
		allowed    []allowedSkip
		wantExit   int
		wantOutput string
	}{
		{
			fixture: "pass.json", name: "pass", required: []string{testA, testB},
			wantExit: exitPassed,
			wantOutput: "--- PASS: TestB (0.00s)\n" +
				"PASS\n" +
				"ok  \texample.com/fixture/a\t0.342s\n" +
				"checkgotest: 2 passed, 0 failed, 0 skipped\n" +
				"checkgotest: 2 of 2 required tests passed\n",
		},
		{
			fixture: "fail.json", name: "fail", wantExit: exitFailed,
			wantOutput: "    a_test.go:13: planted failure\n" +
				"--- FAIL: TestB (0.00s)\n" +
				"FAIL\n" +
				"FAIL\texample.com/fixture/a\t0.223s\n" +
				"checkgotest: 1 passed, 1 failed, 0 skipped\n" +
				"checkgotest: tests failed:\n    example.com/fixture/a TestB\n" +
				"checkgotest: FAIL\n",
		},
		{
			fixture: "parallel.json", name: "parallel tests",
			required: []string{"example.com/fixture/parallel TestC",
				"example.com/fixture/parallel TestD"},
			wantExit: exitPassed,
			wantOutput: "=== PAUSE TestD\n" +
				"=== CONT  TestC\n" +
				"--- PASS: TestC (0.00s)\n" +
				"=== CONT  TestD\n" +
				"--- PASS: TestD (0.00s)\n" +
				"PASS\n" +
				"ok  \texample.com/fixture/parallel\t0.317s\n" +
				"checkgotest: 2 passed, 0 failed, 0 skipped\n" +
				"checkgotest: 2 of 2 required tests passed\n" +
				"checkgotest: PASS\n",
		},
		{
			fixture: "skip.json", name: "skip", wantExit: exitFailed,
			wantOutput: "checkgotest: 1 passed, 0 failed, 1 skipped\n" +
				"checkgotest: tests skipped; this run allows no skips:\n" +
				"    example.com/fixture/a TestB\n",
		},
		{
			fixture: "no-tests.json", name: "zero tests", wantExit: exitDidNotRun,
			wantOutput: "ok  \texample.com/fixture/a\t0.243s [no tests to run]\n" +
				"checkgotest: 0 passed, 0 failed, 0 skipped\n" +
				"checkgotest: DID NOT RUN: no test passed\n" +
				"checkgotest: DID NOT RUN: no test ran in:\n    no-tests.json\n",
		},
		{
			fixture: "pass.json", name: "missing required test",
			required: []string{testA, "example.com/fixture/a TestC"}, wantExit: exitDidNotRun,
			wantOutput: "checkgotest: 1 of 2 required tests passed\n" +
				"checkgotest: DID NOT RUN: required tests never ran:\n" +
				"    example.com/fixture/a TestC\n",
		},
		{
			fixture: "skip.json", name: "allowed skip",
			allowed: []allowedSkip{allowSkip(testB, "2026-10-13")}, wantExit: exitPassed,
			wantOutput: "checkgotest: skips allowed by allowed-skips.txt:\n" +
				"    example.com/fixture/a TestB (TICKET-1, until 2026-10-13): " +
				"the fixture passes\n",
		},
		{
			fixture: "skip.json", name: "expired allowed skip",
			allowed: []allowedSkip{allowSkip(testB, "2026-10-12")}, wantExit: exitFailed,
			wantOutput: "that expired; fix the test or renew the entry:\n" +
				"    example.com/fixture/a TestB (TICKET-1, expired 2026-10-12)\n",
		},
		{
			fixture: "pass.json", name: "stale allowed skip",
			allowed: []allowedSkip{allowSkip(testB, "2026-12-31")}, wantExit: exitFailed,
			wantOutput: "whose test did not skip; remove them:\n" +
				"    example.com/fixture/a TestB (TICKET-1)\n",
		},
		{
			fixture: "not-json.txt", name: "garbage input", wantExit: exitDidNotRun,
			wantOutput: "ok  \texample.com/fixture/a\t0.229s\n" +
				"?   \texample.com/fixture/empty\t[no test files]\n" +
				"checkgotest: 0 passed, 0 failed, 0 skipped\n" +
				"checkgotest: DID NOT RUN: no test passed\n" +
				"checkgotest: DID NOT RUN: no test ran in:\n    not-json.txt\n" +
				"checkgotest: DID NOT RUN: 2 input lines are not go test -json events\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tt.fixture))
			if err != nil {
				t.Fatal(err)
			}
			cfg := config{required: tt.required, allowed: tt.allowed, today: today}
			if tt.allowed != nil {
				cfg.allowedFile = "allowed-skips.txt"
			}
			var output strings.Builder

			exit := check([]input{{name: tt.fixture, r: bytes.NewReader(data)}}, &output, cfg)

			assertResult(t, exit, tt.wantExit, output.String(), tt.wantOutput)
		})
	}
}

func TestCheckReadError(t *testing.T) {
	longLine := `{"Action":"output","Output":"` + strings.Repeat("x", maxLineBytes) + `"}`
	var output strings.Builder

	exit := check([]input{
		{name: "first.json", r: strings.NewReader(runA + "\n" + passA + "\n")},
		{name: "second.json", r: strings.NewReader(longLine)},
	}, &output, config{today: today})

	assertResult(t, exit, exitDidNotRun, output.String(),
		"checkgotest: 1 passed, 0 failed, 0 skipped\n"+
			"checkgotest: DID NOT RUN: reading second.json: bufio.Scanner: token too long\n")
}

// writeFile writes content to a new file in a test directory and returns its path.
func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRun(t *testing.T) {
	passingRun := strings.Join([]string{runA, passA, pkgPass}, "\n")
	skippingRun := strings.Join([]string{runA, passA, runB, skipB, pkgPass}, "\n")
	required := writeFile(t, "required-tests.txt",
		"# Tests that must run.\n\nm/a TestA\n  # indented comment\nm/b\tTestB\n")
	allowed := writeFile(t, "allowed-skips.txt",
		"# Tests that may skip.\n\nm/a TestB TICKET-123 2026-12-31 the API returns the field\n")
	expired := writeFile(t, "expired.txt", "m/a TestB TICKET-123 2026-10-01 the API returns it\n")
	otherRun := writeFile(t, "other.json", strings.Join([]string{
		`{"Action":"run","Package":"m/b","Test":"TestB"}`,
		`{"Action":"pass","Package":"m/b","Test":"TestB"}`,
		`{"Action":"pass","Package":"m/b"}`,
	}, "\n"))
	firstRun := writeFile(t, "first.json", passingRun+"\n")
	emptyRun := writeFile(t, "empty.json", "")
	filteredRun := writeFile(t, "filtered.json", strings.Join([]string{
		`{"Action":"start","Package":"m/c"}`,
		`{"Action":"output","Package":"m/c","Output":"ok  \tm/c\t0.1s [no tests to run]\n"}`,
		`{"Action":"pass","Package":"m/c"}`,
	}, "\n"))
	tests := []struct {
		name       string
		args       []string
		stdin      string
		wantExit   int
		wantStdout string
	}{
		{
			name: "required test ran and passed", args: []string{"-require", "m/a TestA"},
			stdin: passingRun, wantExit: exitPassed,
			wantStdout: "checkgotest: 1 passed, 0 failed, 0 skipped\n" +
				"checkgotest: 1 of 1 required tests passed\n",
		},
		{
			name: "no required tests", stdin: passingRun, wantExit: exitPassed,
			wantStdout: "checkgotest: 1 passed, 0 failed, 0 skipped\ncheckgotest: PASS\n",
		},
		{
			name: "input files instead of stdin", args: []string{"-require-file", required,
				firstRun, otherRun},
			stdin: "not read", wantExit: exitPassed,
			wantStdout: "checkgotest: 2 passed, 0 failed, 0 skipped\n" +
				"checkgotest: 2 of 2 required tests passed\n",
		},
		{
			name: "-require-file and -require together",
			args: []string{"-require", "m/a TestC", "-require-file", required,
				firstRun, otherRun},
			wantExit: exitDidNotRun,
			wantStdout: "checkgotest: 2 of 3 required tests passed\n" +
				"checkgotest: DID NOT RUN: required tests never ran:\n    m/a TestC\n",
		},
		{
			name: "an empty input file", args: []string{firstRun, emptyRun},
			wantExit: exitDidNotRun,
			wantStdout: "checkgotest: 1 passed, 0 failed, 0 skipped\n" +
				"checkgotest: DID NOT RUN: no test ran in:\n    " + emptyRun + "\n" +
				"checkgotest: DID NOT RUN\n",
		},
		{
			name: "an input file in which -run matched no test",
			args: []string{firstRun, filteredRun}, wantExit: exitDidNotRun,
			wantStdout: "ok  \tm/c\t0.1s [no tests to run]\n" +
				"checkgotest: 1 passed, 0 failed, 0 skipped\n" +
				"checkgotest: DID NOT RUN: no test ran in:\n    " + filteredRun + "\n" +
				"checkgotest: DID NOT RUN\n",
		},
		{
			name: "an allowed skip", args: []string{"-allow-skips", allowed},
			stdin: skippingRun, wantExit: exitPassed,
			wantStdout: "checkgotest: skips allowed by " + allowed + ":\n" +
				"    m/a TestB (TICKET-123, until 2026-12-31): the API returns the field\n",
		},
		{
			name: "an expired allowed skip", args: []string{"-allow-skips", expired},
			stdin: skippingRun, wantExit: exitFailed,
			wantStdout: "    m/a TestB (TICKET-123, expired 2026-10-01)\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr strings.Builder

			exit := run(tt.args, strings.NewReader(tt.stdin), &stdout, &stderr, testNow)

			if exit != tt.wantExit {
				t.Errorf("run() = %d, want %d; stdout:\n%s\nstderr:\n%s",
					exit, tt.wantExit, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout does not contain %q:\n%s", tt.wantStdout, stdout.String())
			}
			if stderr.Len() > 0 {
				t.Errorf("stderr = %q, want nothing from a command line that was accepted",
					stderr.String())
			}
		})
	}
}

// testNow is 2026-10-13 in UTC, late in the day.
var testNow = time.Date(2026, time.October, 13, 23, 0, 0, 0, time.UTC)

// TestRunRejects covers command lines and list files that checkgotest cannot use. Each exits 2
// before it reads any input, and prints only the reason and the verdict on stdout.
func TestRunRejects(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.txt")
	tests := []struct {
		name       string
		args       []string
		wantReason string
	}{
		{
			name: "an input file that does not exist", args: []string{"TestA"},
			wantReason: "checkgotest: DID NOT RUN: open TestA: no such file or directory\n",
		},
		{
			name: "an input file after -require", args: []string{"-require", "m/a TestA", "TestB"},
			wantReason: "checkgotest: DID NOT RUN: open TestB: no such file or directory\n",
		},
		{
			name: "-require without the package", args: []string{"-require", "TestA"},
			wantReason: "want an import path and a test name separated by a space",
		},
		{
			name: "-require without the test", args: []string{"-require", "m/a "},
			wantReason: "want an import path and a test name separated by a space",
		},
		{
			name: "-require with an extra word", args: []string{"-require", "m/a TestA x"},
			wantReason: "want an import path and a test name separated by a space",
		},
		{
			name: "-require without a value", args: []string{"-require"},
			wantReason: "flag needs an argument",
		},
		{
			name: "an unknown flag", args: []string{"-required", "m/a TestA"},
			wantReason: "flag provided but not defined",
		},
		{
			name: "a test required twice", args: []string{"-require", "m/a TestA",
				"-require-file", writeFile(t, "twice.txt", "m/a TestA\n")},
			wantReason: `checkgotest: DID NOT RUN: required test "m/a TestA" is named twice`,
		},
		{
			name: "-require-file that does not exist", args: []string{"-require-file", missing},
			wantReason: "checkgotest: DID NOT RUN: open " + missing +
				": no such file or directory\n",
		},
		{
			name: "-require-file with a line that names no test",
			args: []string{"-require-file", writeFile(t, "r.txt", "# comment\nm/a\n")},
			wantReason: "r.txt:2: want an import path and a test name separated by a space, " +
				`got "m/a"`,
		},
		{
			name: "-require-file with a comment after the test",
			args: []string{"-require-file",
				writeFile(t, "r.txt", "m/a TestA # why\n")},
			wantReason: "r.txt:1: want an import path and a test name",
		},
		{
			name: "-allow-skips that does not exist", args: []string{"-allow-skips", missing},
			wantReason: "checkgotest: DID NOT RUN: open " + missing +
				": no such file or directory\n",
		},
		{
			name: "-allow-skips with no un-skip condition",
			args: []string{"-allow-skips", writeFile(t, "a.txt",
				"\n# comment\nm/a TestB TICKET-1 2026-12-31\n")},
			wantReason: "a.txt:3: want 'import/path TestName TICKET-123 YYYY-MM-DD un-skip " +
				"condition'",
		},
		{
			name: "-allow-skips with no ticket",
			args: []string{"-allow-skips", writeFile(t, "a.txt",
				"m/a TestB 2026-12-31 the API returns it\n")},
			wantReason: `a.txt:1: ticket "2026-12-31" is not an issue ID such as TICKET-123`,
		},
		{
			name: "-allow-skips with a lowercase ticket",
			args: []string{"-allow-skips", writeFile(t, "a.txt",
				"m/a TestB ticket-1 2026-12-31 the API returns it\n")},
			wantReason: `a.txt:1: ticket "ticket-1" is not an issue ID such as TICKET-123`,
		},
		{
			name: "-allow-skips with a ticket number of zero",
			args: []string{"-allow-skips", writeFile(t, "a.txt",
				"m/a TestB TICKET-0 2026-12-31 the API returns it\n")},
			wantReason: `a.txt:1: ticket "TICKET-0" is not an issue ID such as TICKET-123`,
		},
		{
			name: "-allow-skips with a ticket inside other text",
			args: []string{"-allow-skips", writeFile(t, "a.txt",
				"m/a TestB xTICKET-1x 2026-12-31 the API returns it\n")},
			wantReason: `a.txt:1: ticket "xTICKET-1x" is not an issue ID such as TICKET-123`,
		},
		{
			name: "-allow-skips with a date that is not YYYY-MM-DD",
			args: []string{"-allow-skips", writeFile(t, "a.txt",
				"m/a TestB TICKET-1 31/12/2026 the API returns it\n")},
			wantReason: `a.txt:1: expiry "31/12/2026" is not a date such as 2026-12-31`,
		},
		{
			name: "-allow-skips that names a test twice",
			args: []string{"-allow-skips", writeFile(t, "a.txt",
				"m/a TestB TICKET-1 2026-12-31 one\nm/a TestB TICKET-2 2026-12-31 two\n")},
			wantReason: `a.txt:2: "m/a TestB" is named twice`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			stdin := strings.Join([]string{runA, passA, pkgPass}, "\n")

			exit := run(tt.args, strings.NewReader(stdin), &stdout, &stderr, testNow)

			if exit != exitDidNotRun {
				t.Errorf("run() = %d, want %d; stdout:\n%s", exit, exitDidNotRun, stdout.String())
			}
			got := stdout.String()
			if !strings.Contains(got, tt.wantReason) {
				t.Errorf("stdout does not contain %q:\n%s", tt.wantReason, got)
			}
			lines := strings.SplitAfter(got, "\n")
			if len(lines) != 3 || !strings.HasPrefix(lines[0], "checkgotest: DID NOT RUN: ") ||
				lines[1] != verdicts[exitDidNotRun] {
				t.Errorf("stdout = %q, want a DID NOT RUN reason and then %q", got,
					verdicts[exitDidNotRun])
			}
		})
	}
}

// TestRunExpiryIsUTC checks that an allowed skip expires at the end of its day in UTC, whatever
// the local time zone.
func TestRunExpiryIsUTC(t *testing.T) {
	allowed := writeFile(t, "allowed-skips.txt",
		"m/a TestB TICKET-1 2026-10-13 the API returns it\n")
	skippingRun := strings.Join([]string{runA, passA, runB, skipB, pkgPass}, "\n")
	east := time.FixedZone("UTC+3", 3*60*60)
	tests := []struct {
		name     string
		now      time.Time
		wantExit int
	}{
		{"the last day in UTC, the next day in UTC+3", time.Date(2026, 10, 14, 2, 59, 0, 0, east),
			exitPassed},
		{"the next day in UTC", time.Date(2026, 10, 14, 3, 0, 0, 0, east), exitFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr strings.Builder

			exit := run([]string{"-allow-skips", allowed}, strings.NewReader(skippingRun),
				&stdout, &stderr, tt.now)

			if exit != tt.wantExit {
				t.Errorf("run() = %d, want %d; stdout:\n%s", exit, tt.wantExit, stdout.String())
			}
		})
	}
}

// TestRepositoryLists checks that the list files CI passes to checkgotest parse.
func TestRepositoryLists(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	required, err := readRequired(filepath.Join(root, ".github", "required-tests.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(required) == 0 {
		t.Error(".github/required-tests.txt names no tests")
	}
	if _, err := readAllowedSkips(filepath.Join(root, ".github", "allowed-skips.txt")); err != nil {
		t.Fatal(err)
	}
}
