package main

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"time"
)

// The statuses gremlins 0.6.0 gives a mutant in its JSON output.
const (
	statusKilled     = "KILLED"
	statusLived      = "LIVED"
	statusTimedOut   = "TIMED OUT"
	statusNotCovered = "NOT COVERED"
	statusNotViable  = "NOT VIABLE"
	statusSkipped    = "SKIPPED"
	statusRunnable   = "RUNNABLE"
)

var knownStatuses = []string{
	statusKilled, statusLived, statusTimedOut, statusNotCovered, statusNotViable, statusSkipped,
	statusRunnable,
}

// mutation is one mutant gremlins reports.
type mutation struct {
	file   string
	line   int
	column int
	kind   string
	status string
}

func (m mutation) String() string { return fmt.Sprintf("%s:%d:%d", m.file, m.line, m.column) }

// in returns m with its file name, relative to dir, made relative to the
// repository root, given dir relative to it.
func (m mutation) in(dir string) mutation {
	m.file = path.Join(dir, m.file)
	return m
}

type gremlinsOutput struct {
	Files []struct {
		FileName  string `json:"file_name"`
		Mutations []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
			Line   int    `json:"line"`
			Column int    `json:"column"`
		} `json:"mutations"`
	} `json:"files"`
}

// readResults reads gremlins' --output file. gremlins writes none when it finds no
// mutant at all, so a missing file means that the package holds nothing to mutate.
func readResults(p string) ([]mutation, error) {
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out gremlinsOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("gremlins results: %w", err)
	}
	var mutations []mutation
	for _, f := range out.Files {
		for _, m := range f.Mutations {
			if f.FileName == "" || m.Line < 1 || !slices.Contains(knownStatuses, m.Status) {
				return nil, fmt.Errorf("gremlins results: unexpected mutant %q line %d status %q",
					f.FileName, m.Line, m.Status)
			}
			mutations = append(mutations, mutation{
				file: f.FileName, line: m.Line, column: m.Column, kind: m.Type, status: m.Status,
			})
		}
	}
	sortMutations(mutations)
	return mutations, nil
}

func sortMutations(mutations []mutation) {
	slices.SortFunc(mutations, func(a, b mutation) int {
		return cmp.Or(cmp.Compare(a.file, b.file), cmp.Compare(a.line, b.line),
			cmp.Compare(a.column, b.column), cmp.Compare(a.kind, b.kind))
	})
}

// event is a test process that exec stopped, or a limit it could not apply.
type event struct {
	kind   string
	binary string
	detail string
}

// readEvents reads the log exec appends to. No log means no event.
func readEvents(p string) ([]event, error) {
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var events []event
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		fields := strings.SplitN(sc.Text(), "\t", 3)
		if len(fields) != 3 {
			return nil, fmt.Errorf("exec log: unexpected line %q", sc.Text())
		}
		events = append(events, event{kind: fields[0], binary: fields[1], detail: fields[2]})
	}
	return events, sc.Err()
}

// report is everything the gate knows about one module.
type report struct {
	opts   gateOptions
	limits string
	scope  moduleScope
	// packages counts the packages gremlins ran on.
	packages   int
	runs       int
	elapsed    time.Duration
	events     []event
	mutations  []mutation
	mismatched []mutation
	failure    error
	// sample is the module's share of the sample, when there was one.
	sample *sampleShare
	head   string
	// sampledOut counts the mutants on lines the sample left out.
	sampledOut int
}

// outcome is a module's verdict.
type outcome struct {
	exit     int
	notice   string
	counts   map[string]int
	detected int
	tested   int
	efficacy float64
}

// reports returns the report of each module, from the results of its units. With
// no results, nothing was mutated.
func (p *plan) reports(opts gateOptions, limits string, res *runResults) []report {
	out := make([]report, len(p.modules))
	for i, m := range p.modules {
		out[i] = report{opts: opts, limits: limits, scope: m}
	}
	if res == nil {
		return out
	}
	for i, u := range p.units {
		r := &out[u.module]
		ur := res.units[i]
		if ur.ran {
			r.packages++
		}
		r.runs += ur.runs
		r.elapsed += ur.elapsed
		r.events = append(r.events, ur.events...)
		r.mutations = append(r.mutations, ur.mutations...)
		r.mismatched = append(r.mismatched, ur.mismatched...)
		r.sampledOut += ur.sampledOut
		if r.failure == nil && ur.failure != nil {
			r.failure = ur.failure
		}
		if res.sampled {
			if r.sample == nil {
				r.sample = &sampleShare{}
				r.head = res.head
			}
			r.sample.add(ur.candidates, res.chosen)
		}
	}
	for i := range out {
		sortMutations(out[i].mutations)
		sortMutations(out[i].mismatched)
	}
	return out
}

// hasUnits reports whether the run mutated any package of the module.
func (r report) hasUnits() bool { return r.runs > 0 || r.failure != nil }

// evaluate decides the module's verdict from gremlins' results and exec's log.
func (r report) evaluate() outcome {
	if !r.hasUnits() {
		if r.opts.all {
			return outcome{
				exit:   exitDidNotRun,
				notice: r.dirList() + " holds no code that Go's coverage counts to mutate",
			}
		}
		return outcome{exit: exitPassed, notice: r.noChangeNotice()}
	}
	if r.failure != nil {
		return outcome{exit: exitDidNotRun, notice: r.failure.Error()}
	}
	for _, e := range r.events {
		if e.kind == eventError {
			return outcome{exit: exitDidNotRun, notice: fmt.Sprintf(
				"the gate's exec, go or git subcommand could not do its part: %s: %s", e.binary,
				e.detail)}
		}
	}
	if len(r.mismatched) > 0 {
		return outcome{exit: exitDidNotRun, notice: fmt.Sprintf(
			"gremlins skipped or tested other lines than the gate chose for %d mutants (listed "+
				"below)", len(r.mismatched))}
	}
	counts := map[string]int{}
	for _, m := range r.mutations {
		counts[m.status]++
	}
	if counts[statusRunnable] > 0 {
		return outcome{exit: exitDidNotRun, counts: counts, notice: fmt.Sprintf(
			"gremlins left %d mutants unrun", counts[statusRunnable])}
	}
	inScope := len(r.mutations) - counts[statusSkipped]
	o := outcome{counts: counts, detected: counts[statusKilled] + counts[statusTimedOut]}
	o.tested = o.detected + counts[statusLived]
	switch {
	case inScope == 0 && r.sampledOut > 0, o.tested == 0 && r.sampledOut > 0:
		o.notice = fmt.Sprintf("No mutant of this module that would run is in the sample: the "+
			"%d that would were left out.", r.sampledOut)
		return o
	case inScope == 0 && r.opts.all:
		o.notice = "gremlins found nothing to mutate in " + r.dirList() + "."
		return o
	case inScope == 0:
		o.notice = fmt.Sprintf("The lines changed since the merge base with %s hold nothing "+
			"gremlins mutates.", r.opts.base)
		return o
	case o.tested == 0 && !r.opts.all && counts[statusNotViable] == inScope:
		o.notice = fmt.Sprintf("None of the %d mutants on the lines changed since the merge base "+
			"with %s compiles, such as - between two strings, so there is nothing for the tests "+
			"to detect.", inScope, r.opts.base)
		return o
	case o.tested == 0:
		o.exit = exitDidNotRun
		o.notice = fmt.Sprintf("No mutant ran: of the %d mutants in scope, %d are on lines no "+
			"test covers and %d do not compile", inScope, counts[statusNotCovered],
			counts[statusNotViable])
		return o
	}
	o.efficacy = 100 * float64(o.detected) / float64(o.tested)
	o.notice = fmt.Sprintf(
		"The tests detected %d of the %d mutants that ran: %.2f%%, threshold %g%%.",
		o.detected,
		o.tested,
		o.efficacy,
		r.opts.threshold,
	)
	if n := counts[statusNotViable]; n > 0 {
		o.notice += fmt.Sprintf(" %d mutants that do not compile are not counted.", n)
	}
	if o.efficacy < r.opts.threshold {
		o.exit = exitFailed
	}
	return o
}

func (r report) noChangeNotice() string {
	if len(r.scope.notMutated) > 0 {
		return fmt.Sprintf("Go files changed since the merge base with %s, but none that the "+
			"gate mutates (listed below); nothing was mutated.", r.opts.base)
	}
	return fmt.Sprintf("No non-test Go line was added or changed since the merge base with %s; "+
		"nothing to mutate.", r.opts.base)
}

// dirList names the directories of the module the gate was given.
func (r report) dirList() string {
	quoted := make([]string, len(r.scope.dirs))
	for i, dir := range r.scope.dirs {
		quoted[i] = "`" + dir + "`"
	}
	return strings.Join(quoted, ", ")
}

// verdicts are the words for each exit code.
var verdicts = map[int]string{exitPassed: "PASS", exitFailed: "FAIL", exitDidNotRun: "DID NOT RUN"}

// finishAll prints each module's report, appends it to the summary file and
// returns the exit code: 2 when a module did not run, or with -all when no mutant
// ran in any module, else 1 when one failed.
func finishAll(out sink, reports []report) int {
	exit := exitPassed
	all, tested := false, 0
	for _, r := range reports {
		o := r.evaluate()
		text := r.markdown(o)
		_, _ = io.WriteString(out.stdout, text)
		if out.summary != "" {
			if err := appendFile(out.summary, text); err != nil {
				out.summary = ""
				return out.didNotRun(fmt.Errorf("job summary: %w", err))
			}
		}
		exit = max(exit, o.exit)
		all = all || r.opts.all
		tested += o.tested
	}
	if exit == exitPassed && all && tested == 0 {
		return out.didNotRun(errors.New("no mutant ran in any module; with -all, a run that " +
			"tests no mutant does not pass"))
	}
	_, _ = fmt.Fprintf(out.stdout, "mutationgate: %s\n", verdicts[exit])
	return exit
}

func appendFile(p, text string) error {
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(text); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (r report) markdown(o outcome) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### Mutation tests: `%s` (%s)\n\n**%s**: %s\n\n", r.scope.rel,
		r.scope.module.path, verdicts[o.exit], o.notice)
	if r.opts.all {
		fmt.Fprintf(&b, "Scope: every Go file of the packages in %s; files: %d.\n\n", r.dirList(),
			r.scope.changedFiles)
	} else {
		fmt.Fprintf(&b, "Scope: the lines changed in %s since the merge base with `%s`; changed "+
			"Go files: %d.\n\n", r.dirList(), r.opts.base, r.scope.changedFiles)
	}
	if r.sample != nil {
		s := r.sample
		fmt.Fprintf(&b, "Sample: %d of the %d mutants that would run, on %d of %d lines, in %d of "+
			"%d files (%s of the files), chosen by commit %s with a budget of %d mutants in all "+
			"modules.\n\n", s.chosenMutants, s.mutants, s.chosenLines, s.lines, s.chosenFiles,
			s.files, percent(s.chosenFiles, s.files), r.head, r.opts.maxMutants)
	}
	if r.limits != "" && r.hasUnits() {
		fmt.Fprintf(&b, "Limits: %s.\n\n", r.limits)
	}
	if r.runs > 0 {
		fmt.Fprintf(&b, "gremlins ran %d times on %d packages, for %s.\n\n", r.runs, r.packages,
			r.elapsed.Round(time.Second))
	}
	if o.counts != nil {
		b.WriteString("| Killed | Timed out | Lived | Not covered | Not viable |\n" +
			"|---|---|---|---|---|\n")
		fmt.Fprintf(&b, "| %d | %d | %d | %d | %d |\n\n", o.counts[statusKilled],
			o.counts[statusTimedOut], o.counts[statusLived], o.counts[statusNotCovered],
			o.counts[statusNotViable])
		b.WriteString("Killed and timed-out mutants count as detected; efficacy is detected / " +
			"(detected + lived). Mutants that do not compile are not counted.\n\n")
	}
	writeBaseline(&b, r.events)
	writeEvents(&b, r.events)
	writeMutations(&b, "Surviving mutants", r.withStatus(statusLived))
	writeMutations(&b, "Mutants on lines no test covers", r.withStatus(statusNotCovered))
	writeMutations(&b, "Mutants on other lines than the gate chose", r.mismatched)
	if len(r.scope.notMutated) > 0 {
		fmt.Fprintf(&b, "#### Changed Go files not mutated (%d)\n\n", len(r.scope.notMutated))
		for _, f := range r.scope.notMutated {
			fmt.Fprintf(&b, "- `%s`: %s\n", f.file, f.reason)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// percent returns part of whole as a rounded percentage.
func percent(part, whole int) string {
	if whole == 0 {
		return "0%"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(part)/float64(whole))
}

func (r report) withStatus(status string) []mutation {
	var out []mutation
	for _, m := range r.mutations {
		if m.status == status {
			out = append(out, m)
		}
	}
	return out
}

// writeBaseline reports the slowest test process of gremlins' coverage runs, which
// ran the tests without a mutant.
func writeBaseline(b *strings.Builder, events []event) {
	var slowest event
	var longest time.Duration
	for _, e := range events {
		if e.kind != eventBaseline {
			continue
		}
		if d, err := time.ParseDuration(e.detail); err == nil && d >= longest {
			slowest, longest = e, d
		}
	}
	if slowest.binary != "" {
		fmt.Fprintf(
			b,
			"Slowest test process without a mutant: `%s`, %s.\n\n",
			slowest.binary,
			longest,
		)
	}
}

// writeEvents lists the test processes exec stopped or saw run out of memory, and
// the errors exec and the shims logged.
func writeEvents(b *strings.Builder, events []event) {
	var listed []event
	for _, e := range events {
		if e.kind != eventBaseline {
			listed = append(listed, e)
		}
	}
	if len(listed) == 0 {
		return
	}
	fmt.Fprintf(b, "#### Test processes exec stopped, and errors (%d)\n\nA mutant whose test "+
		"process was stopped or ran out of memory counts as killed.\n\n", len(listed))
	for _, e := range listed {
		fmt.Fprintf(b, "- `%s` (%s): %s\n", e.binary, e.kind, e.detail)
	}
	b.WriteString("\n")
}

func writeMutations(b *strings.Builder, title string, mutations []mutation) {
	if len(mutations) == 0 {
		return
	}
	fmt.Fprintf(b, "#### %s (%d)\n\n| Mutant | Type | Status |\n|---|---|---|\n",
		title, len(mutations))
	for _, m := range mutations {
		fmt.Fprintf(b, "| `%s` | %s | %s |\n", m, m.kind, m.status)
	}
	b.WriteString("\n")
}
