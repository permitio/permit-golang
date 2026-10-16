package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The lines that go test -bench writes before and after the benchmarks of a package.
const (
	header = "goos: linux\ngoarch: amd64\npkg: github.com/permitio/permit-golang/v2\n" +
		"cpu: Example CPU @ 2.00GHz\n"
	footer = "PASS\nok  \tgithub.com/permitio/permit-golang/v2\t3.210s\n"
)

// run1 returns the line that go test -bench writes for one run of the benchmark name that took
// ns nanoseconds and made allocs allocations for each operation.
func run1(name string, ns, allocs float64) string {
	return fmt.Sprintf("%s \t  100000\t%10g ns/op\t  2048 B/op\t%6g allocs/op\n", name, ns,
		allocs)
}

// output returns the output of go test -bench with the lines of runs between its header and
// its footer.
func output(runs ...string) string {
	return header + strings.Join(runs, "") + footer
}

// writeFile writes content to a file named name in a new directory and returns its path.
func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// runReport runs benchreport with args and returns its exit code, stdout and stderr.
func runReport(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exit := run(args, &stdout, &stderr)
	return exit, stdout.String(), stderr.String()
}

// lastLine returns the last line of output, without its newline.
func lastLine(output string) string {
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	return lines[len(lines)-1]
}

// checkOutput checks the exit code and the output of a run of benchreport: the verdict as its
// last line, each of wantStdout in stdout, none of notStdout, and wantStderr in stderr, or
// nothing on stderr when wantStderr is empty.
func checkOutput(t *testing.T, exit int, stdout, stderr string, want reportCase) {
	t.Helper()
	if exit != want.wantExit {
		t.Errorf("exit = %d, want %d; stdout:\n%s", exit, want.wantExit, stdout)
	}
	if got := lastLine(stdout); got != want.wantVerdict {
		t.Errorf("the last line is\n\t%q\nwant\n\t%q\nstdout:\n%s", got, want.wantVerdict,
			stdout)
	}
	for _, s := range want.wantStdout {
		if !strings.Contains(stdout, s) {
			t.Errorf("stdout does not contain %q:\n%s", s, stdout)
		}
	}
	for _, s := range want.notStdout {
		if strings.Contains(stdout, s) {
			t.Errorf("stdout contains %q:\n%s", s, stdout)
		}
	}
	switch {
	case want.wantStderr == "" && stderr != "":
		t.Errorf("stderr = %q, want nothing", stderr)
	case !strings.Contains(stderr, want.wantStderr):
		t.Errorf("stderr = %q, want it to contain %q", stderr, want.wantStderr)
	}
}

// reportCase is a comparison of a previous and a current result that TestRun checks.
type reportCase struct {
	name string
	// previous is the previous result file's content; noPrevious leaves the file out.
	previous   string
	noPrevious bool
	current    string

	wantExit    int
	wantVerdict string
	wantStdout  []string
	notStdout   []string
	// wantStderr is a part of stderr; when it is empty, stderr must be empty.
	wantStderr string
}

func TestRun(t *testing.T) {
	const check = "BenchmarkCheck-4"
	// steady is a result of three benchmarks, one of them a sub-benchmark.
	steady := output(
		run1(check, 5000, 40),
		run1("BenchmarkBulkCheck/checks=10-4", 20000, 200),
		run1("BenchmarkFilterObjects/resources=100-4", 150000, 1500),
	)
	tests := []reportCase{
		{
			name: "the same result", previous: steady, current: steady, wantExit: exitPassed,
			wantVerdict: "benchreport: PASS: no median more than 20% higher in ns/op or " +
				"allocs/op in the 3 benchmarks compared",
			wantStdout: []string{
				"benchmark  ", "unit  ", "previous  ", "current  ", "change\n",
				"BenchmarkFilterObjects/resources=100-4  allocs/op  1500      1500     +0.0%\n",
			},
			notStdout: []string{"over 20%", "no previous result"},
		},
		{
			name:     "allocs/op 25% higher",
			previous: output(run1(check, 5000, 40)), current: output(run1(check, 5000, 50)),
			wantExit:    exitRegressed,
			wantVerdict: "benchreport: REGRESSION: BenchmarkCheck-4 allocs/op +25.0% (40 -> 50)",
			wantStdout: []string{
				"BenchmarkCheck-4  ns/op      5000      5000     +0.0%\n",
				"BenchmarkCheck-4  allocs/op  40        50       +25.0%, over 20%\n",
			},
		},
		{
			name:     "allocs/op 15% higher",
			previous: output(run1(check, 5000, 40)), current: output(run1(check, 5000, 46)),
			wantExit: exitPassed,
			wantVerdict: "benchreport: PASS: no median more than 20% higher in ns/op or " +
				"allocs/op in the 1 benchmark compared",
			wantStdout: []string{"allocs/op  40        46       +15.0%\n"},
			notStdout:  []string{"over 20%"},
		},
		{
			name:     "allocs/op exactly 20% higher, which is not over 20%",
			previous: output(run1(check, 5000, 40)), current: output(run1(check, 5000, 48)),
			wantExit: exitPassed,
			wantVerdict: "benchreport: PASS: no median more than 20% higher in ns/op or " +
				"allocs/op in the 1 benchmark compared",
			notStdout: []string{"over 20%"},
		},
		{
			name:     "ns/op 25% higher",
			previous: output(run1(check, 4000, 40)), current: output(run1(check, 5000, 40)),
			wantExit:    exitRegressed,
			wantVerdict: "benchreport: REGRESSION: BenchmarkCheck-4 ns/op +25.0% (4000 -> 5000)",
		},
		{
			name:     "ns/op exactly 20% higher, which is not over 20%",
			previous: output(run1(check, 4000, 40)), current: output(run1(check, 4800, 40)),
			wantExit: exitPassed,
			wantVerdict: "benchreport: PASS: no median more than 20% higher in ns/op or " +
				"allocs/op in the 1 benchmark compared",
		},
		{
			name:     "faster and fewer allocations",
			previous: output(run1(check, 4000, 40)), current: output(run1(check, 2000, 30)),
			wantExit: exitPassed,
			wantVerdict: "benchreport: PASS: no median more than 20% higher in ns/op or " +
				"allocs/op in the 1 benchmark compared",
			wantStdout: []string{"ns/op      4000      2000     -50.0%\n", "-25.0%\n"},
		},
		{
			name: "two regressions, in the order of the current result",
			previous: output(run1("BenchmarkA-4", 1000, 10), run1("BenchmarkB-4", 1000, 10),
				run1("BenchmarkC-4", 1000, 10)),
			current: output(run1("BenchmarkC-4", 1000, 13), run1("BenchmarkB-4", 1000, 10),
				run1("BenchmarkA-4", 1300, 10)),
			wantExit: exitRegressed,
			wantVerdict: "benchreport: REGRESSION: BenchmarkC-4 allocs/op +30.0% (10 -> 13); " +
				"BenchmarkA-4 ns/op +30.0% (1000 -> 1300)",
		},
		{
			// The mean of the previous runs is 4000 ns/op, so 2100 ns/op would be faster than
			// it; their median is 1000 ns/op, so 2100 is a regression.
			name: "the median of an odd number of runs",
			previous: output(run1(check, 1000, 40), run1(check, 10000, 40),
				run1(check, 1000, 40)),
			current:     output(run1(check, 2100, 40)),
			wantExit:    exitRegressed,
			wantVerdict: "benchreport: REGRESSION: BenchmarkCheck-4 ns/op +110.0% (1000 -> 2100)",
		},
		{
			name: "the median of an even number of runs",
			previous: output(run1(check, 1000, 40), run1(check, 4000, 40),
				run1(check, 2000, 40), run1(check, 1000, 40)),
			current:  output(run1(check, 1500, 40), run1(check, 1500, 40)),
			wantExit: exitPassed,
			wantVerdict: "benchreport: PASS: no median more than 20% higher in ns/op or " +
				"allocs/op in the 1 benchmark compared",
			wantStdout: []string{"ns/op      1500      1500     +0.0%\n"},
		},
		{
			name:     "no allocations before or now",
			previous: output(run1(check, 5000, 0)), current: output(run1(check, 5000, 0)),
			wantExit: exitPassed,
			wantVerdict: "benchreport: PASS: no median more than 20% higher in ns/op or " +
				"allocs/op in the 1 benchmark compared",
			wantStdout: []string{"allocs/op  0         0        +0.0%\n"},
		},
		{
			name:     "allocations where there were none",
			previous: output(run1(check, 5000, 0)), current: output(run1(check, 5000, 2)),
			wantExit:    exitRegressed,
			wantVerdict: "benchreport: REGRESSION: BenchmarkCheck-4 allocs/op +Inf% (0 -> 2)",
		},
		{
			name:     "a benchmark that the previous result does not have",
			previous: output(run1(check, 5000, 40)),
			current:  output(run1(check, 5000, 40), run1("BenchmarkNew-4", 300, 3)),
			wantExit: exitPassed,
			wantVerdict: "benchreport: PASS: no median more than 20% higher in ns/op or " +
				"allocs/op in the 1 benchmark compared",
			wantStdout: []string{
				"BenchmarkNew-4    ns/op      -         300      no previous result\n",
				"BenchmarkNew-4    allocs/op  -         3        no previous result\n",
			},
		},
		{
			name:     "a previous result with none of the benchmarks",
			previous: output(run1("BenchmarkOld-4", 5000, 40)),
			current:  output(run1(check, 9000, 90), run1("BenchmarkNew-4", 300, 3)),
			wantExit: exitPassed,
			wantVerdict: "benchreport: NO PREVIOUS RESULT: the previous result has none of the " +
				"current benchmarks; there is nothing to compare the 2 benchmarks with",
			wantStdout: []string{
				"BenchmarkCheck-4  ns/op      -         9000     no previous result",
			},
		},
		{
			name:       "no previous result file",
			noPrevious: true,
			current:    output(run1(check, 5000, 40)),
			wantExit:   exitPassed,
			wantVerdict: "benchreport: NO PREVIOUS RESULT: previous.txt does not exist; there " +
				"is nothing to compare the 1 benchmark with",
			wantStdout: []string{
				"BenchmarkCheck-4  allocs/op  -         40       no previous result",
			},
		},
		{
			name:     "go test -v output, whose lines without a result are left out",
			previous: output(check+"\n", run1(check, 5000, 40)),
			current:  output("=== RUN   "+check+"\n", check+"\n", run1(check, 5000, 50)),
			wantExit: exitRegressed,
			wantVerdict: "benchreport: REGRESSION: BenchmarkCheck-4 allocs/op +25.0% " +
				"(40 -> 50)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			previous := filepath.Join(dir, "previous.txt")
			if !tt.noPrevious {
				if err := os.WriteFile(previous, []byte(tt.previous), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			current := writeFile(t, "current.txt", tt.current)
			t.Chdir(dir)

			exit, stdout, stderr := runReport(t, "previous.txt", current)

			checkOutput(t, exit, stdout, stderr, tt)
		})
	}
}

// TestRunDidNotRun checks that benchreport does not pass when it cannot compare: a command line
// without both files, a current result that is missing, empty or not benchmark output, a
// benchmark without ns/op or allocs/op, a previous result that exists but cannot be read or is
// not benchmark output, and a stdout that cannot be written. Each is DID NOT RUN, and none is a
// missing previous result.
func TestRunDidNotRun(t *testing.T) {
	valid := output(run1("BenchmarkCheck-4", 5000, 40))
	noAllocs := output("BenchmarkCheck-4 \t  100000\t      5000 ns/op\n")
	noTime := output("BenchmarkCheck-4 \t  100000\t        40 allocs/op\n")
	tests := []struct {
		name string
		// previous and current are the files' contents; a missing one is not written, and a
		// directory one is a directory.
		previous, current string
		args              []string
		// failingStdout gives benchreport a stdout whose every write fails.
		failingStdout bool
		wantVerdict   string
		wantStderr    string
	}{
		{
			name: "no arguments", args: []string{}, previous: valid, current: valid,
			wantVerdict: "benchreport: DID NOT RUN: want the previous and the current result " +
				"files, got 0 arguments",
		},
		{
			name: "one argument", args: []string{"current.txt"}, previous: valid,
			current: valid,
			wantVerdict: "benchreport: DID NOT RUN: want the previous and the current result " +
				"files, got 1 argument",
		},
		{
			name: "an unknown flag", args: []string{"-x", "previous.txt", "current.txt"},
			previous: valid, current: valid,
			wantVerdict: "benchreport: DID NOT RUN: flag provided but not defined: -x",
			wantStderr:  "flag provided but not defined: -x\nUsage of benchreport:\n",
		},
		{
			name: "no current result", previous: valid, current: "missing",
			wantVerdict: "benchreport: DID NOT RUN: open current.txt: no such file or directory",
		},
		{
			name: "an empty current result", previous: valid, current: "",
			wantVerdict: "benchreport: DID NOT RUN: current.txt holds no benchmark result",
		},
		{
			name: "a current result of a failed run", previous: valid,
			current:     header + "--- FAIL: BenchmarkCheck-4\nFAIL\n",
			wantVerdict: "benchreport: DID NOT RUN: current.txt holds no benchmark result",
		},
		{
			name: "a current result that is not benchmark output", previous: valid,
			current: output("BenchmarkCheck-4 \t  100000\t  fast ns/op\n"),
			wantVerdict: "benchreport: DID NOT RUN: current.txt:5: parsing measurement: " +
				"invalid syntax",
		},
		{
			name: "a benchmark without allocs/op", previous: valid, current: noAllocs,
			wantVerdict: "benchreport: DID NOT RUN: current.txt:5: BenchmarkCheck-4 reports no " +
				"allocs/op; benchreport compares ns/op and allocs/op, so call b.ReportAllocs in " +
				"the benchmark",
		},
		{
			name: "a benchmark without ns/op", previous: valid, current: noTime,
			wantVerdict: "benchreport: DID NOT RUN: current.txt:5: BenchmarkCheck-4 reports no " +
				"ns/op; benchreport compares ns/op and allocs/op, so call b.ReportAllocs in " +
				"the benchmark",
		},
		{
			name: "an empty previous result", previous: "", current: valid,
			wantVerdict: "benchreport: DID NOT RUN: previous.txt holds no benchmark result",
		},
		{
			name: "a previous result that is not benchmark output", current: valid,
			previous: output("BenchmarkCheck-4 \t  many\t  5000 ns/op\n"),
			wantVerdict: "benchreport: DID NOT RUN: previous.txt:5: parsing iteration count: " +
				"invalid syntax",
		},
		{
			name: "a previous benchmark without allocs/op", previous: noAllocs, current: valid,
			wantVerdict: "benchreport: DID NOT RUN: previous.txt:5: BenchmarkCheck-4 reports " +
				"no allocs/op; benchreport compares ns/op and allocs/op, so call b.ReportAllocs " +
				"in the benchmark",
		},
		{
			name: "a previous result that is a directory", previous: "directory", current: valid,
			wantVerdict: "benchreport: DID NOT RUN: previous.txt:0: read previous.txt: is a " +
				"directory",
		},
		{
			// The verdict is lost with the table; the exit code still says that benchreport
			// did not run.
			name: "a stdout that cannot be written", previous: valid, current: valid,
			failingStdout: true, wantVerdict: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			for name, content := range map[string]string{
				"previous.txt": tt.previous, "current.txt": tt.current,
			} {
				var err error
				switch content {
				case "missing":
				case "directory":
					err = os.Mkdir(name, 0o755)
				default:
					err = os.WriteFile(name, []byte(content), 0o644)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			args := tt.args
			if args == nil {
				args = []string{"previous.txt", "current.txt"}
			}

			var stdout, stderr bytes.Buffer
			var out io.Writer = &stdout
			if tt.failingStdout {
				out = failingWriter{}
			}

			exit := run(args, out, &stderr)

			checkOutput(t, exit, stdout.String(), stderr.String(), reportCase{
				wantExit: exitDidNotRun, wantVerdict: tt.wantVerdict, wantStderr: tt.wantStderr,
				notStdout: []string{"NO PREVIOUS RESULT", "PASS"},
			})
		})
	}
}

// failingWriter is a stdout whose every write fails, as on a full disk.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("no space left on device")
}
