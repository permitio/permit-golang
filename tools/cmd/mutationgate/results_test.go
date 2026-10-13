package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func m(file string, line int, status string) mutation {
	return mutation{
		file:   file,
		line:   line,
		column: 1,
		kind:   "CONDITIONALS_BOUNDARY",
		status: status,
	}
}

func TestEvaluate(t *testing.T) {
	diffOpts := func(threshold float64) gateOptions {
		return gateOptions{base: "main", threshold: threshold}
	}
	allOpts := func(threshold float64) gateOptions {
		return gateOptions{all: true, threshold: threshold}
	}
	tests := []struct {
		name       string
		opts       gateOptions
		mutations  []mutation
		events     []event
		mismatched []mutation
		failure    error
		sampledOut int
		// noRun is a module the gate ran gremlins on no package of.
		noRun      bool
		notMutated []notMutated
		wantExit   int
		wantNotice string
	}{
		{
			name: "efficacy equal to the threshold passes", opts: diffOpts(75),
			mutations: []mutation{
				m("a.go", 10, statusKilled),
				m("a.go", 10, statusKilled),
				m(
					"a.go",
					11,
					statusKilled,
				),
				m("a.go", 12, statusLived),
				m("a.go", 3, statusSkipped),
				m("b.go", 1, statusSkipped),
			},
			wantExit: exitPassed, wantNotice: "detected 3 of the 4 mutants that ran: 75.00%",
		},
		{
			name: "efficacy below the threshold fails", opts: diffOpts(75.01),
			mutations: []mutation{
				m("a.go", 10, statusKilled), m("a.go", 10, statusKilled),
				m("a.go", 11, statusKilled), m("a.go", 12, statusLived),
			},
			wantExit: exitFailed, wantNotice: "75.00%, threshold 75.01%",
		},
		{
			name: "timed-out mutants count as detected", opts: diffOpts(90),
			mutations: []mutation{
				m("a.go", 10, statusTimedOut), m("a.go", 11, statusKilled),
				m("a.go", 12, statusNotCovered), m("a.go", 12, statusNotViable),
			},
			wantExit: exitPassed, wantNotice: "detected 2 of the 2 mutants that ran: 100.00%",
		},
		{
			name:       "changed lines with nothing to mutate pass",
			opts:       diffOpts(90),
			mutations:  []mutation{m("a.go", 3, statusSkipped), m("b.go", 1, statusSkipped)},
			wantExit:   exitPassed,
			wantNotice: "since the merge base with main hold nothing gremlins",
		},
		{
			name: "packages with no mutant at all pass", opts: diffOpts(90),
			wantExit: exitPassed, wantNotice: "hold nothing gremlins mutates",
		},
		{
			name: "no test covers the changed lines", opts: diffOpts(90),
			mutations: []mutation{
				m("a.go", 10, statusNotCovered), m("a.go", 11, statusNotCovered),
				m("a.go", 3, statusSkipped),
			},
			wantExit:   exitDidNotRun,
			wantNotice: "No mutant ran: of the 2 mutants in scope, 2 are on lines no test",
		},
		{
			name: "gremlins skipped or tested other lines", opts: diffOpts(90),
			mutations:  []mutation{m("a.go", 10, statusKilled), m("a.go", 11, statusSkipped)},
			mismatched: []mutation{m("a.go", 11, statusSkipped)},
			wantExit:   exitDidNotRun, wantNotice: "other lines than the gate chose for 1 mutants",
		},
		{
			name: "a mutant was left unrun", opts: diffOpts(90),
			mutations: []mutation{m("a.go", 10, statusKilled), m("a.go", 11, statusRunnable)},
			wantExit:  exitDidNotRun, wantNotice: "left 1 mutants unrun",
		},
		{
			name: "exec could not apply a limit", opts: diffOpts(90),
			mutations: []mutation{m("a.go", 10, statusKilled)},
			events: []event{
				{kind: eventCap, binary: "a.test", detail: "ran long"},
				{kind: eventError, binary: "a.test", detail: "EPERM"},
			},
			wantExit: exitDidNotRun, wantNotice: "could not do its part: a.test: EPERM",
		},
		{
			name: "gremlins failed on a package", opts: diffOpts(90),
			failure:  errors.New("gremlins exited 1 on m/pkg"),
			wantExit: exitDidNotRun, wantNotice: "gremlins exited 1 on m/pkg",
		},
		{
			name: "mutants that do not compile are not counted", opts: diffOpts(50),
			mutations: []mutation{
				m("a.go", 10, statusKilled), m("a.go", 11, statusLived),
				m("a.go", 11, statusNotViable), m("a.go", 12, statusNotViable),
				m("a.go", 12, statusNotViable),
			},
			wantExit: exitPassed,
			wantNotice: "detected 1 of the 2 mutants that ran: 50.00%, threshold 50%. " +
				"3 mutants that do not compile are not counted.",
		},
		{
			name: "a weak test fails once mutants that do not compile are left out",
			opts: diffOpts(50),
			mutations: []mutation{
				m("a.go", 10, statusKilled), m("a.go", 11, statusLived),
				m("a.go", 12, statusLived), m("a.go", 12, statusNotViable),
			},
			wantExit: exitFailed, wantNotice: "detected 1 of the 3 mutants that ran: 33.33%",
		},
		{
			name: "changed lines whose mutants do not compile pass", opts: diffOpts(90),
			mutations: []mutation{
				m("a.go", 10, statusNotViable), m("a.go", 11, statusNotViable),
				m("a.go", 3, statusSkipped),
			},
			wantExit: exitPassed, wantNotice: "None of the 2 mutants on the lines changed",
		},
		{
			name: "no mutant ran, and some are on lines no test covers", opts: diffOpts(90),
			mutations: []mutation{m("a.go", 10, statusNotViable), m("a.go", 11, statusNotCovered)},
			wantExit:  exitDidNotRun,
			wantNotice: "No mutant ran: of the 2 mutants in scope, 1 are on lines no test covers " +
				"and 1 do not compile",
		},
		{
			name: "whole module where no mutant compiles", opts: allOpts(50),
			mutations: []mutation{m("a.go", 1, statusNotViable)},
			wantExit:  exitDidNotRun, wantNotice: "No mutant ran",
		},
		{
			name: "stopped test processes do not fail a run", opts: diffOpts(90),
			mutations: []mutation{m("a.go", 10, statusKilled)},
			events:    []event{{kind: eventMemory, binary: "a.test", detail: "used 1700 MiB"}},
			wantExit:  exitPassed, wantNotice: "100.00%",
		},
		{
			name: "whole module", opts: allOpts(50),
			mutations: []mutation{m("a.go", 1, statusKilled), m("b.go", 2, statusLived)},
			wantExit:  exitPassed, wantNotice: "detected 1 of the 2 mutants that ran: 50.00%",
		},
		{
			name: "whole module with nothing to mutate", opts: allOpts(50),
			wantExit: exitPassed, wantNotice: "gremlins found nothing to mutate in `.`.",
		},
		{
			name: "whole module whose mutants are on lines no test covers", opts: allOpts(50),
			mutations: []mutation{m("a.go", 1, statusNotCovered)},
			wantExit:  exitDidNotRun, wantNotice: "No mutant ran: of the 1 mutants in scope, 1 are",
		},
		{
			name: "whole module with no Go file", opts: allOpts(50), noRun: true,
			wantExit: exitDidNotRun, wantNotice: "`.` holds no Go code to mutate",
		},
		{
			name: "no Go line changed", opts: diffOpts(90), noRun: true,
			wantExit: exitPassed, wantNotice: "No non-test Go line was added or changed since " +
				"the merge base with main",
		},
		{
			name: "only files the gate does not mutate changed", opts: diffOpts(90), noRun: true,
			notMutated: []notMutated{{"testdata/x.go", "test data"}},
			wantExit:   exitPassed, wantNotice: "but none that the gate mutates",
		},
		{
			name: "every mutant that would run was left out of the sample", opts: diffOpts(90),
			mutations: []mutation{m("a.go", 10, statusSkipped)}, sampledOut: 3,
			wantExit: exitPassed, wantNotice: "the 3 that would were left out",
		},
		{
			name: "only uncovered lines are left after the sample", opts: diffOpts(90),
			mutations: []mutation{m("a.go", 10, statusNotCovered)}, sampledOut: 2,
			wantExit: exitPassed, wantNotice: "the 2 that would were left out",
		},
		{
			name: "a sample that ran is measured", opts: diffOpts(90),
			mutations: []mutation{m("a.go", 10, statusLived)}, sampledOut: 5,
			wantExit: exitFailed, wantNotice: "detected 0 of the 1 mutants that ran",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := report{
				opts: tt.opts, mutations: tt.mutations, events: tt.events,
				mismatched: tt.mismatched, failure: tt.failure, sampledOut: tt.sampledOut,
				scope: moduleScope{rel: ".", dirs: []string{"."}, notMutated: tt.notMutated},
			}
			if !tt.noRun {
				r.runs = 1
			}
			o := r.evaluate()
			if o.exit != tt.wantExit || !strings.Contains(o.notice, tt.wantNotice) {
				t.Errorf(
					"got exit %d %q, want exit %d and %q",
					o.exit,
					o.notice,
					tt.wantExit,
					tt.wantNotice,
				)
			}
		})
	}
}

func TestReadResults(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	good := write("good.json", `{"files":[`+
		`{"file_name":"b.go","mutations":[{"type":"T","status":"LIVED","line":2,"column":1}]},`+
		`{"file_name":"a.go","mutations":[{"type":"T","status":"TIMED OUT","line":9,"column":4},`+
		`{"type":"T","status":"KILLED","line":3,"column":7}]}]}`)
	got, err := readResults(good)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, mu := range got {
		order = append(order, mu.String()+" "+mu.status)
	}
	want := "a.go:3:7 KILLED, a.go:9:4 TIMED OUT, b.go:2:1 LIVED"
	if strings.Join(order, ", ") != want {
		t.Errorf("got %s, want %s", strings.Join(order, ", "), want)
	}
	if got, err := readResults(filepath.Join(dir, "missing.json")); got != nil || err != nil {
		t.Errorf("no results file, as when gremlins finds no mutant: got %v, %v", got, err)
	}

	oneMutation := func(file, mutation string) string {
		return `{"files":[{` + file + `"mutations":[` + mutation + `]}]}`
	}
	bad := map[string]string{
		"a directory": dir,
		"not JSON":    write("bad.json", `{"files":`),
		"unknown status": write("status.json",
			oneMutation(`"file_name":"a.go",`, `{"status":"DEAD","line":1}`)),
		"no line": write("line.json",
			oneMutation(`"file_name":"a.go",`, `{"status":"KILLED"}`)),
		"no file name": write("name.json",
			oneMutation("", `{"status":"KILLED","line":1}`)),
	}
	for name, p := range bad {
		if got, err := readResults(p); err == nil {
			t.Errorf("%s: got %v and no error", name, got)
		}
	}
}

// The report lists the limits, the slowest unmutated test process and the files not
// mutated only when there are such things to report.
func TestMarkdown(t *testing.T) {
	quiet := report{
		opts: gateOptions{base: "main"}, limits: "2 workers",
		scope: moduleScope{rel: "tools", dirs: []string{"tools"}, module: goModule{path: "m"}},
	}
	text := quiet.markdown(quiet.evaluate())
	for _, absent := range []string{"Limits:", "Slowest", "not mutated", "gremlins ran"} {
		if strings.Contains(text, absent) {
			t.Errorf("a module with nothing to mutate reports %q:\n%s", absent, text)
		}
	}
	busy := quiet
	busy.runs, busy.packages = 2, 1
	busy.mutations = []mutation{m("tools/a.go", 1, statusKilled)}
	busy.events = []event{
		{kind: eventBaseline, binary: "a.test", detail: "4s"},
		{kind: eventCap, binary: "b.test", detail: "9s"},
		{kind: eventBaseline, binary: "c.test", detail: "2s"},
	}
	busy.scope.notMutated = []notMutated{{"tools/x_windows.go", "go does not build it here"}}
	text = busy.markdown(busy.evaluate())
	for _, want := range []string{
		"Limits: 2 workers.\n", "gremlins ran 2 times on 1 packages",
		"Slowest test process without a mutant: `a.test`, 4s.\n",
		"- `b.test` (cap): 9s\n",
		"#### Changed Go files not mutated (1)\n\n- `tools/x_windows.go`: go does not build",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("report lacks %q:\n%s", want, text)
		}
	}
	if got := percent(2, 4); got != "50%" {
		t.Errorf("percent(2, 4) = %s", got)
	}
}

func TestMutationIn(t *testing.T) {
	mu := m("main.go", 3, statusKilled)
	if got := mu.in("tools/cmd/x").String(); got != "tools/cmd/x/main.go:3:1" {
		t.Errorf("got %s", got)
	}
	if got := mu.in(".").String(); got != "main.go:3:1" {
		t.Errorf("in the root: got %s", got)
	}
}

func TestReadEvents(t *testing.T) {
	dir := t.TempDir()
	if events, err := readEvents(filepath.Join(dir, "none.log")); err != nil || events != nil {
		t.Errorf("no log: %v, %v", events, err)
	}
	p := filepath.Join(dir, "events.log")
	if err := os.WriteFile(p, []byte("cap\ta.test\tran long\tstill detail\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	events, err := readEvents(p)
	want := event{eventCap, "a.test", "ran long\tstill detail"}
	if err != nil || len(events) != 1 || events[0] != want {
		t.Errorf("got %v, %v", events, err)
	}
	if err := os.WriteFile(p, []byte("cap only\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if events, err := readEvents(p); err == nil {
		t.Errorf("malformed log: got %v and no error", events)
	}
	if events, err := readEvents(dir); err == nil {
		t.Errorf("a directory: got %v and no error", events)
	}
}

// finishAll exits 2 when any module did not run, else 1 when any failed, and ends
// with the verdict.
func TestFinishAll(t *testing.T) {
	passed := report{opts: gateOptions{base: "main"}, scope: moduleScope{rel: "."}}
	failed := report{
		opts: gateOptions{base: "main", threshold: 90}, runs: 1,
		mutations: []mutation{m("a.go", 1, statusLived)}, scope: moduleScope{rel: "tools"},
	}
	notRun := report{
		opts: gateOptions{base: "main"}, runs: 1, failure: errors.New("gremlins exited 1"),
		scope: moduleScope{rel: "x"},
	}
	tests := []struct {
		name    string
		reports []report
		want    int
		verdict string
	}{
		{"every module passed", []report{passed, passed}, exitPassed, "PASS"},
		{"a module failed", []report{passed, failed}, exitFailed, "FAIL"},
		{"a module did not run", []report{failed, notRun, passed}, exitDidNotRun, "DID NOT RUN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := filepath.Join(t.TempDir(), "summary.md")
			var stdout bytes.Buffer
			exit := finishAll(sink{stdout: &stdout, stderr: &bytes.Buffer{}, summary: summary},
				tt.reports)
			if exit != tt.want ||
				!strings.HasSuffix(stdout.String(), "mutationgate: "+tt.verdict+"\n") {
				t.Errorf("exit %d, output:\n%s", exit, stdout.String())
			}
			written, err := os.ReadFile(summary)
			if err != nil ||
				strings.Count(string(written), "### Mutation tests: ") != len(tt.reports) {
				t.Errorf("summary %q, %v", written, err)
			}
		})
	}
	t.Run("a summary file that cannot be written", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exit := finishAll(sink{stdout: &stdout, stderr: &stderr, summary: t.TempDir()},
			[]report{passed})
		if exit != exitDidNotRun || !strings.Contains(stderr.String(), "job summary") {
			t.Errorf("exit %d, stderr %q", exit, stderr.String())
		}
	})
}
