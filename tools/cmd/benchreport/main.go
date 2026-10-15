// Command benchreport compares a result of the SDK's benchmarks with the previous one and
// reports each benchmark that got more than 20% slower, in ns/op, or allocates more than 20% more
// often, in allocs/op. It reads both results in the format that go test -bench writes, with
// golang.org/x/perf/benchfmt, the parser of benchstat, and compares the median of each
// benchmark's runs, such as the 6 runs of -count 6, in one with the median in the other.
//
// Usage:
//
//	benchreport previous current
//
// previous and current name files of go test -bench output, whose benchmarks report allocs/op, as
// b.ReportAllocs makes them. A previous file that does not exist, such as when no earlier run
// kept a result, is no previous result, and so is a previous result that has none of the
// benchmarks of current; neither is a failure.
//
// benchreport prints a table with a row for each benchmark of current and each of the two units:
// the median of the previous and of the current runs, and the change, which is "over 20%" for a
// regression. Its last line is its verdict, one of:
//
//	benchreport: PASS: no median more than 20% higher in ns/op or allocs/op in the <n> ...
//	benchreport: REGRESSION: <benchmark> <unit> <change> (<previous> -> <current>); ...
//	benchreport: NO PREVIOUS RESULT: <why>
//	benchreport: DID NOT RUN: <why>
//
// Exit codes:
//
//	0  no benchmark regressed more than 20%, or there is no previous result
//	1  a benchmark regressed more than 20% in ns/op or allocs/op
//	2  benchreport did not run: the command line is wrong, current does not exist, a file cannot
//	   be read, holds no benchmark result, holds a benchmark line that is not valid, or holds a
//	   benchmark without ns/op or allocs/op, or stdout cannot be written
//
// Build it before use, because go run reports every non-zero exit as 1:
//
//	go -C tools build -o ../bin/benchreport ./cmd/benchreport
//	bin/benchreport previous.txt current.txt
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"golang.org/x/perf/benchfmt"
)

const (
	exitPassed    = 0
	exitRegressed = 1
	exitDidNotRun = 2
)

// threshold is the change, in percent, that a benchmark's median may grow by in each unit; a
// median more than this much higher than the previous one is a regression.
const threshold = 20

// units are the units benchreport compares, in the order it prints them.
var units = []string{"ns/op", "allocs/op"}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run reads the previous and the current result that args name, prints the comparison and the
// verdict, and returns the exit code.
func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("benchreport", flag.ContinueOnError)
	flags.SetOutput(stderr)
	if err := flags.Parse(args); err != nil {
		return didNotRun(stdout, err)
	}
	if flags.NArg() != 2 {
		return didNotRun(stdout, fmt.Errorf("want the previous and the current result files, "+
			"got %s", plural(flags.NArg(), "argument")))
	}
	previousPath, currentPath := flags.Arg(0), flags.Arg(1)
	current, err := readResults(currentPath)
	if err != nil {
		return didNotRun(stdout, err)
	}
	previous, err := readResults(previousPath)
	missing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !missing {
		return didNotRun(stdout, err)
	}
	comparisons := compare(previous, current)
	if err = printTable(stdout, comparisons); err != nil {
		return didNotRun(stdout, err)
	}
	if missing {
		fmt.Fprintf(stdout, "benchreport: NO PREVIOUS RESULT: %s does not exist; there is "+
			"nothing to compare the %s with\n", previousPath,
			plural(len(current.names), "benchmark"))
		return exitPassed
	}
	return verdict(stdout, comparisons, len(current.names))
}

// didNotRun prints the verdict of a run that could not compare, and returns exitDidNotRun.
func didNotRun(stdout io.Writer, reason error) int {
	fmt.Fprintf(stdout, "benchreport: DID NOT RUN: %v\n", reason)
	return exitDidNotRun
}

// results are the runs of the benchmarks of one file: for each benchmark, by its name as go test
// prints it, such as BenchmarkCheck-4, the value of each unit of units in each of its runs.
type results struct {
	// names are the benchmarks in the order of their first run.
	names  []string
	values map[string]map[string][]float64
}

// readResults reads the results of a file of go test -bench output. Lines that are not
// benchmark results, such as goos: and PASS, are left out. A line that starts like a result but
// is not valid, a result without ns/op or allocs/op, and a file with no result are errors.
func readResults(path string) (results, error) {
	file, err := os.Open(path)
	if err != nil {
		return results{}, err
	}
	defer func() { _ = file.Close() }()
	read := results{values: map[string]map[string][]float64{}}
	reader := benchfmt.NewReader(file, path)
	for reader.Scan() {
		switch record := reader.Result().(type) {
		case *benchfmt.SyntaxError:
			return results{}, record
		case *benchfmt.Result:
			if err := read.add(record); err != nil {
				return results{}, err
			}
		}
	}
	if err := reader.Err(); err != nil {
		return results{}, err
	}
	if len(read.names) == 0 {
		return results{}, fmt.Errorf("%s holds no benchmark result", path)
	}
	return read, nil
}

// add adds the values of one run of a benchmark, in the units of units, as go test printed them.
func (r *results) add(run *benchfmt.Result) error {
	name := "Benchmark" + run.Name.String()
	printed := map[string]float64{}
	for _, v := range run.Values {
		// benchfmt converts ns/op to sec/op, and keeps the value go test printed as OrigValue.
		unit, value := v.Unit, v.Value
		if v.OrigUnit != "" {
			unit, value = v.OrigUnit, v.OrigValue
		}
		printed[unit] = value
	}
	for _, unit := range units {
		if _, ok := printed[unit]; !ok {
			file, line := run.Pos()
			return fmt.Errorf("%s:%d: %s reports no %s; benchreport compares ns/op and "+
				"allocs/op, so call b.ReportAllocs in the benchmark", file, line, name, unit)
		}
	}
	runs, ok := r.values[name]
	if !ok {
		runs = map[string][]float64{}
		r.values[name] = runs
		r.names = append(r.names, name)
	}
	for _, unit := range units {
		runs[unit] = append(runs[unit], printed[unit])
	}
	return nil
}

// comparison is the median of a benchmark's runs in one unit, now and in the previous result.
type comparison struct {
	name, unit        string
	previous, current float64
	// hasPrevious is false when the previous result does not have the benchmark.
	hasPrevious bool
}

// regressed reports whether the current median is more than threshold percent higher than the
// previous one.
func (c comparison) regressed() bool {
	return c.hasPrevious && c.current*100 > c.previous*(100+threshold)
}

// change returns the change from the previous median to the current one in percent, such as
// +25.0%, or +Inf% from 0.
func (c comparison) change() string {
	if c.current == c.previous {
		return "+0.0%"
	}
	return fmt.Sprintf("%+.1f%%", (c.current-c.previous)/c.previous*100)
}

// compare returns a comparison for each benchmark of current, in its order, and each unit.
func compare(previous, current results) []comparison {
	var all []comparison
	for _, name := range current.names {
		for _, unit := range units {
			c := comparison{name: name, unit: unit, current: median(current.values[name][unit])}
			if runs, ok := previous.values[name]; ok {
				c.previous, c.hasPrevious = median(runs[unit]), true
			}
			all = append(all, c)
		}
	}
	return all
}

// median returns the median of values, which is not empty.
func median(values []float64) float64 {
	sorted := slices.Sorted(slices.Values(values))
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

// printTable prints a row for each comparison: the benchmark, the unit, the previous and the
// current median, and the change.
func printTable(stdout io.Writer, comparisons []comparison) error {
	table := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "benchmark\tunit\tprevious\tcurrent\tchange")
	for _, c := range comparisons {
		previous, change := "-", "no previous result"
		if c.hasPrevious {
			previous, change = format(c.previous), c.change()
		}
		if c.regressed() {
			change += ", over " + strconv.Itoa(threshold) + "%"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", c.name, c.unit, previous, format(c.current),
			change)
	}
	return table.Flush()
}

// verdict prints the verdict of a comparison with a previous result, of n benchmarks, and
// returns the exit code.
func verdict(stdout io.Writer, comparisons []comparison, n int) int {
	compared := 0
	var regressions []string
	for _, c := range comparisons {
		if c.hasPrevious {
			compared++
		}
		if c.regressed() {
			regressions = append(regressions, fmt.Sprintf("%s %s %s (%s -> %s)", c.name, c.unit,
				c.change(), format(c.previous), format(c.current)))
		}
	}
	switch {
	case len(regressions) > 0:
		fmt.Fprintf(stdout, "benchreport: REGRESSION: %s\n", strings.Join(regressions, "; "))
		return exitRegressed
	case compared == 0:
		fmt.Fprintf(stdout, "benchreport: NO PREVIOUS RESULT: the previous result has none of "+
			"the current benchmarks; there is nothing to compare the %s with\n",
			plural(n, "benchmark"))
		return exitPassed
	}
	fmt.Fprintf(stdout, "benchreport: PASS: no median more than %d%% higher in ns/op or allocs/op "+
		"in the %s compared\n", threshold, plural(compared/len(units), "benchmark"))
	return exitPassed
}

// plural returns n and noun, with an s unless n is 1, such as "1 benchmark" or "4 benchmarks".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// format returns a median as go test prints a value, with no more digits than it needs.
func format(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
