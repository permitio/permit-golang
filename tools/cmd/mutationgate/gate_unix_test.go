//go:build unix

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/tools/internal/execfile"
)

// fakeMutant is a mutant the fake gremlins finds, and the status a test run of it
// gives.
type fakeMutant struct {
	line   int
	status string
}

// gremlinsCall is what the gate gave the fake gremlins on one run.
type gremlinsCall struct {
	dir, pkgDir, pkg string
	dryRun           bool
	scope            map[string][]lineRange
	exclude          []string
}

// fakeGremlins stands in for gremlins. For the package directory it is given, it
// reports the mutants in mutants, by file relative to the repository root, that
// are in that directory and not excluded: SKIPPED when the git shim's diff leaves
// their line out, RUNNABLE or NOT COVERED in a dry run, and their status otherwise.
// It reads the diff the git shim on its PATH answers with; with viaShim, it runs
// git diff --merge-base mutationgate-scope with that PATH, as gremlins does.
type fakeGremlins struct {
	t       *testing.T
	root    string
	mutants map[string][]fakeMutant
	exit    int
	events  string
	// dryEvents are written to exec's log, and dryOnly reported with the status
	// they have, in a dry run only.
	dryEvents string
	dryOnly   map[string][]fakeMutant
	// skipChosen makes it skip the mutants on the lines in scope too.
	skipChosen bool
	viaShim    bool
	calls      []gremlinsCall
}

// scopeDiffOf returns the diff that the git shim in bin answers gremlins' git diff
// with: by running it when viaShim is set, and otherwise from the file it names.
func (f *fakeGremlins) scopeDiffOf(bin string, env []string) ([]byte, error) {
	if f.viaShim {
		shell := exec.CommandContext(context.Background(), "sh", "-c",
			"git diff --merge-base "+scopeRef)
		shell.Env = append(slices.Clone(env), testMainEnv+"=main")
		return shell.Output()
	}
	gitShim, err := os.ReadFile(filepath.Join(bin, "git"))
	if err != nil {
		return nil, err
	}
	diffPath := regexp.MustCompile(`-diff='([^']*)'`).FindStringSubmatch(string(gitShim))[1]
	return os.ReadFile(diffPath)
}

func (f *fakeGremlins) run(_, dir string, args, env []string) (int, error) {
	f.t.Helper()
	call := gremlinsCall{
		dir: dir, pkgDir: args[len(args)-1],
		dryRun: slices.Contains(args, "--dry-run"),
	}
	for i, arg := range args {
		if arg == "--exclude-files" {
			call.exclude = append(call.exclude, args[i+1])
		}
	}
	bin, _, _ := strings.Cut(envValue(env, "PATH"), string(os.PathListSeparator))
	goShim, err := os.ReadFile(filepath.Join(bin, "go"))
	if err != nil {
		return 0, err
	}
	call.pkg = regexp.MustCompile(`-package='([^']*)'`).FindStringSubmatch(string(goShim))[1]
	diff, err := f.scopeDiffOf(bin, env)
	if err != nil {
		return 0, err
	}
	if call.scope, err = parseAddedLines(diff); err != nil {
		return 0, err
	}
	f.calls = append(f.calls, call)

	events := f.events
	if call.dryRun {
		events += f.dryEvents
	}
	if events != "" {
		// The gate appends its -exec to any GOFLAGS it runs with, as when these
		// tests run under the gate itself, so its -log is the last one.
		goflags := envValue(env, "GOFLAGS")
		log := goflags[strings.LastIndex(goflags, "-log=")+len("-log="):]
		log = strings.TrimSuffix(log, "'")
		if err := os.WriteFile(log, []byte(events), 0o600); err != nil {
			return 0, err
		}
	}
	if f.exit != 0 {
		return f.exit, nil
	}
	return 0, f.writeResults(args, call)
}

func (f *fakeGremlins) writeResults(args []string, call gremlinsCall) error {
	pkgRel, err := filepath.Rel(f.root, call.pkgDir)
	if err != nil {
		return err
	}
	type outMutant struct {
		Type   string `json:"type"`
		Status string `json:"status"`
		Line   int    `json:"line"`
		Column int    `json:"column"`
	}
	type outFile struct {
		FileName  string      `json:"file_name"`
		Mutations []outMutant `json:"mutations"`
	}
	names := slices.Collect(maps.Keys(f.mutants))
	if call.dryRun {
		names = append(names, slices.Collect(maps.Keys(f.dryOnly))...)
	}
	slices.Sort(names)
	var files []outFile
	for _, file := range slices.Compact(names) {
		name := path.Base(file)
		if path.Dir(file) != filepath.ToSlash(pkgRel) || slices.ContainsFunc(call.exclude,
			func(p string) bool { return regexp.MustCompile(p).MatchString(name) }) {
			continue
		}
		of := outFile{FileName: name}
		for _, fm := range f.mutants[file] {
			status := fm.status
			switch {
			case f.skipChosen || !isChanged(call.scope, name, fm.line):
				status = statusSkipped
			case call.dryRun && status != statusNotCovered:
				status = statusRunnable
			}
			of.Mutations = append(of.Mutations, outMutant{
				Type:   "CONDITIONALS_NEGATION",
				Status: status, Line: fm.line, Column: 2,
			})
		}
		if call.dryRun {
			for _, fm := range f.dryOnly[file] {
				of.Mutations = append(of.Mutations, outMutant{
					Type:   "CONDITIONALS_NEGATION",
					Status: fm.status, Line: fm.line, Column: 2,
				})
			}
		}
		files = append(files, of)
	}
	if len(files) == 0 {
		return nil
	}
	data, err := json.Marshal(map[string]any{"files": files})
	if err != nil {
		return err
	}
	return os.WriteFile(args[slices.Index(args, "--output")+1], data, 0o600)
}

func envValue(env []string, key string) string {
	i := slices.IndexFunc(env, func(kv string) bool { return strings.HasPrefix(kv, key+"=") })
	if i < 0 {
		return ""
	}
	return strings.TrimPrefix(env[i], key+"=")
}

// gateFixture is a run of the gate in the fixture repository with a fake gremlins.
type gateFixture struct {
	root  string
	tools tools
	fake  *fakeGremlins
	// probes counts the memory limit probes.
	probes int
	// copyTests are the directories and arguments of the go test runs in a copy
	// of a module, and copyErr what they return.
	copyTests []string
	copyErr   error
}

const headSHA = "4f1c1b8e2d7a9c0b3e5f6a7b8c9d0e1f2a3b4c5d"

func newGateFixture(t *testing.T, diff string, mutants map[string][]fakeMutant) *gateFixture {
	t.Helper()
	root, tl := fixtureRepository(t, diff)
	g := &gateFixture{root: root, fake: &fakeGremlins{t: t, root: root, mutants: mutants}}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("the gate tests need git on PATH: %v", err)
	}
	tl.self = testExecutable(t)
	tl.goPath = "/usr/local/go/bin/go"
	tl.gitPath = gitPath
	tl.repo = func() (string, string, error) { return root, headSHA, nil }
	tl.probe = func(gateOptions) (int64, error) { g.probes++; return 1600, nil }
	tl.gremlins = g.fake.run
	// The copy of a module is the directory named copy-<n> above the package.
	tl.goTest = func(dir string, args, env []string) (string, error) {
		copyRoot := dir
		for !strings.HasPrefix(filepath.Base(copyRoot), "copy-") {
			copyRoot = filepath.Dir(copyRoot)
		}
		if _, err := os.Stat(filepath.Join(copyRoot, "go.mod")); err != nil {
			return "", err
		}
		rel, err := filepath.Rel(copyRoot, dir)
		if err != nil {
			return "", err
		}
		g.copyTests = append(g.copyTests, filepath.ToSlash(rel)+" "+strings.Join(args, " ")+
			" with "+envValue(env, "GOFLAGS"))
		return "--- FAIL: TestRepository\nFAIL", g.copyErr
	}
	g.tools = tl
	return g
}

// run runs the gate with args and dirs, relative to the repository root, and returns
// its exit code, its output and the summary file.
func (g *gateFixture) run(t *testing.T, args []string, dirs ...string) (int, string, string) {
	t.Helper()
	for _, dir := range dirs {
		args = append(args, filepath.Join(g.root, filepath.FromSlash(dir)))
	}
	summary := filepath.Join(t.TempDir(), "summary.md")
	var stdout, stderr bytes.Buffer
	exit := runGate(append([]string{"-summary", summary}, args...), &stdout, &stderr, g.tools)
	written, err := os.ReadFile(summary)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return exit, stdout.String() + stderr.String(), string(written)
}

// fixtureDiff changes two lines of a.go, one of cmd/tool/main.go and adds
// tools/x/x.go.
var fixtureDiff = changedDiff(map[string]string{
	"a.go":             "@@ -2 +2,2 @@\n-x\n+y\n+z\n",
	"other_windows.go": "@@ -1 +1 @@\n-x\n+y\n",
	"cmd/tool/main.go": "@@ -4 +4 @@\n-x\n+y\n",
	"tools/x/x.go":     "@@ -0,0 +1,5 @@\n+a\n+b\n+c\n+d\n+e\n",
})

// rootDiff changes the lines of fixtureDiff in the root module only, and toolsDiff
// those in the tools module only.
var (
	rootDiff = changedDiff(map[string]string{
		"a.go": "@@ -2 +2,2 @@\n-x\n+y\n+z\n", "cmd/tool/main.go": "@@ -4 +4 @@\n-x\n+y\n",
	})
	toolsDiff = changedDiff(map[string]string{
		"tools/x/x.go": "@@ -0,0 +1,5 @@\n+a\n+b\n+c\n+d\n+e\n",
	})
)

func TestRunGate(t *testing.T) {
	t.Parallel()
	killedEverywhere := map[string][]fakeMutant{
		"a.go":             {{1, statusKilled}, {2, statusKilled}, {3, statusKilled}},
		"other_windows.go": {{1, statusLived}},
		"cmd/tool/main.go": {{4, statusKilled}, {5, statusLived}},
		"tools/x/x.go":     {{2, statusKilled}, {4, statusTimedOut}},
	}
	base := []string{"-base", "main"}
	both := []string{".", "tools"}
	tests := []struct {
		name string
		// args are the flags, and dirs the directories relative to the repository.
		args       []string
		dirs       []string
		diff       string
		mutants    map[string][]fakeMutant
		files      map[string]string // new content of fixture files, by path in the repository
		edit       func(g *gateFixture)
		wantExit   int
		wantCalls  int
		wantProbes int
		// wantCopies is how many packages' tests ran in a copy of their module.
		wantCopies  int
		wantOutput  []string
		wantSummary []string
	}{
		{
			name: "no Go line changed",
			args: base, dirs: both,
			diff:     changedDiff(map[string]string{"README.md": "@@ -1 +1 @@\n-a\n+b\n"}),
			wantExit: exitPassed,
			wantOutput: []string{
				"### Mutation tests: `.` (m)\n\n**PASS**: No non-test Go line",
				"### Mutation tests: `tools` (m/tools)\n\n**PASS**: No non-test Go line",
				"mutationgate: PASS\n",
			},
		},
		{
			name: "only files the gate does not mutate changed",
			args: base, dirs: both,
			diff:     changedDiff(map[string]string{"testdata/fix.go": "@@ -1 +1 @@\n-a\n+b\n"}),
			wantExit: exitPassed,
			wantOutput: []string{
				"**PASS**: Go files changed since the merge base with main, but none that the gate",
				"- `testdata/fix.go`: test data",
			},
		},
		{
			name: "only comments changed",
			args: base, dirs: both,
			diff: changedDiff(map[string]string{
				"cmd/tool/main.go": "@@ -1 +1,2 @@\n-a\n+b\n+c\n",
			}),
			files: map[string]string{
				"cmd/tool/main.go": "// Command tool is a fixture,\n// with a comment.\n" +
					"package main\n",
			},
			wantExit: exitPassed,
			wantOutput: []string{
				"### Mutation tests: `.` (m)\n\n**PASS**: Go files changed since the merge base " +
					"with main, but none that the gate mutates (listed below); nothing was " +
					"mutated.",
				"- `cmd/tool/main.go`: no code on the changed lines (comments, blank lines, the " +
					"package clause or imports)\n",
				"mutationgate: PASS\n",
			},
		},
		{
			// gremlins reports the mutants of a package-level variable's value, such as
			// those of the strings it joins, as on lines no test covers.
			name: "only a package-level variable's value changed",
			args: base, dirs: both,
			diff: changedDiff(map[string]string{
				"tools/x/x.go": "@@ -6,2 +6,2 @@\n-a\n-b\n+c\n+d\n",
			}),
			files: map[string]string{"tools/x/x.go": bodiesFile},
			mutants: map[string][]fakeMutant{
				"tools/x/x.go": {{6, statusNotCovered}, {11, statusKilled}},
			},
			wantExit: exitPassed,
			wantOutput: []string{
				"### Mutation tests: `tools` (m/tools)\n\n**PASS**: Go files changed since the " +
					"merge base with main, but none that the gate mutates (listed below); " +
					"nothing was mutated.",
				"- `tools/x/x.go`: no code in a function body on the changed lines (declarations " +
					"outside function bodies, which Go's coverage does not count)\n",
				"mutationgate: PASS\n",
			},
		},
		{
			name: "a package-level variable and a function body whose mutants no test covers",
			args: base, dirs: both,
			diff: changedDiff(map[string]string{
				"tools/x/x.go": "@@ -6,2 +6,2 @@\n-a\n-b\n+c\n+d\n@@ -11 +11 @@\n-a\n+b\n",
			}),
			files: map[string]string{"tools/x/x.go": bodiesFile},
			mutants: map[string][]fakeMutant{
				"tools/x/x.go": {{6, statusNotCovered}, {11, statusNotCovered}},
			},
			wantExit: exitDidNotRun, wantCalls: 1, wantProbes: 1,
			wantOutput: []string{
				"### Mutation tests: `tools` (m/tools)\n\n**DID NOT RUN**: No mutant ran: of " +
					"the 1 mutants in scope, 1 are on lines no test covers and 0 do not compile",
				"| `tools/x/x.go:11:2` | CONDITIONALS_NEGATION | NOT COVERED |",
				"mutationgate: DID NOT RUN\n",
			},
		},
		{
			// gremlins reports the mutants of a case clause's expressions as on lines
			// no test covers.
			name: "only a case clause's expression changed",
			args: base, dirs: both,
			diff:  changedDiff(map[string]string{"tools/x/x.go": "@@ -7 +7 @@\n-a\n+b\n"}),
			files: map[string]string{"tools/x/x.go": uncountedFile},
			mutants: map[string][]fakeMutant{
				"tools/x/x.go": {{7, statusNotCovered}, {8, statusKilled}},
			},
			wantExit: exitPassed,
			wantOutput: []string{
				"### Mutation tests: `tools` (m/tools)\n\n**PASS**: Go files changed since the " +
					"merge base with main, but none that the gate mutates (listed below); " +
					"nothing was mutated.",
				"- `tools/x/x.go`: no code that Go's coverage counts on the changed lines (a " +
					"case clause up to its colon, or the rest of a statement after the body of a " +
					"function literal in it)\n",
				"mutationgate: PASS\n",
			},
		},
		{
			name: "only the rest of a statement after a function literal changed",
			args: base, dirs: both,
			diff:  changedDiff(map[string]string{"tools/x/x.go": "@@ -34 +34 @@\n-a\n+b\n"}),
			files: map[string]string{"tools/x/x.go": uncountedFile},
			mutants: map[string][]fakeMutant{
				"tools/x/x.go": {{33, statusKilled}, {34, statusNotCovered}},
			},
			wantExit: exitPassed,
			wantOutput: []string{
				"**PASS**: Go files changed since the merge base with main, but none that the " +
					"gate mutates (listed below); nothing was mutated.",
				"- `tools/x/x.go`: no code that Go's coverage counts on the changed lines",
				"mutationgate: PASS\n",
			},
		},
		{
			name: "a case clause and a function body whose mutants no test covers",
			args: base, dirs: both,
			diff: changedDiff(map[string]string{
				"tools/x/x.go": "@@ -7,2 +7,2 @@\n-a\n-b\n+c\n+d\n",
			}),
			files: map[string]string{"tools/x/x.go": uncountedFile},
			mutants: map[string][]fakeMutant{
				"tools/x/x.go": {{7, statusNotCovered}, {8, statusNotCovered}},
			},
			wantExit: exitDidNotRun, wantCalls: 1, wantProbes: 1,
			wantOutput: []string{
				"### Mutation tests: `tools` (m/tools)\n\n**DID NOT RUN**: No mutant ran: of " +
					"the 1 mutants in scope, 1 are on lines no test covers and 0 do not compile",
				"| `tools/x/x.go:8:2` | CONDITIONALS_NEGATION | NOT COVERED |",
				"mutationgate: DID NOT RUN\n",
			},
		},
		{
			name: "a changed Go file that does not parse",
			args: base, dirs: both, diff: rootDiff,
			files:    map[string]string{"a.go": "package x\n\nfunc {\n"},
			wantExit: exitDidNotRun,
			wantOutput: []string{
				"cannot tell the code lines of a Go file that does not parse: a.go:3:6: ",
				"mutationgate: DID NOT RUN\n",
			},
			wantSummary: []string{"**DID NOT RUN**: cannot tell the code lines of a Go file"},
		},
		{
			name: "a change in a module not given",
			args: base, dirs: []string{"."},
			diff: fixtureDiff, wantExit: exitDidNotRun,
			wantOutput:  []string{"go lines changed in tools/x/x.go", "mutationgate: DID NOT RUN"},
			wantSummary: []string{"### Mutation tests\n\n**DID NOT RUN**: go lines changed in"},
		},
		{
			name: "the memory limit is not enforced",
			args: base, dirs: both, diff: fixtureDiff, wantExit: exitDidNotRun,
			edit: func(g *gateFixture) {
				g.tools.probe = func(gateOptions) (int64, error) {
					return 0, errors.New("the limit is not enforced")
				}
			},
			wantOutput:  []string{"memory limit check: the limit is not enforced", "DID NOT RUN"},
			wantSummary: []string{"**DID NOT RUN**: memory limit check: the limit is not enforced"},
		},
		{
			name: "a wrong command line",
			args: []string{"-base", "main", "-workers", "0"}, dirs: []string{"."},
			diff:       fixtureDiff,
			wantExit:   exitDidNotRun,
			wantOutput: []string{"-workers 0"},
		},
		{
			name: "an executable path GOFLAGS cannot carry",
			args: base, dirs: both, diff: fixtureDiff, wantExit: exitDidNotRun,
			edit:       func(g *gateFixture) { g.tools.self = "/my dir/mutationgate" },
			wantOutput: []string{"GOFLAGS and the shims"},
		},
		{
			name: "a go command path the go shim cannot carry",
			args: base, dirs: both, diff: fixtureDiff, wantExit: exitDidNotRun,
			edit:       func(g *gateFixture) { g.tools.goPath = "/it's/go" },
			wantOutput: []string{`go command "/it's/go"`},
		},
		{
			name: "a git command path the git shim cannot carry",
			args: base, dirs: both, diff: fixtureDiff, wantExit: exitDidNotRun,
			edit:       func(g *gateFixture) { g.tools.gitPath = "" },
			wantOutput: []string{"cannot find the git command"},
		},
		{
			name: "the repository cannot be read",
			args: base, dirs: both, diff: fixtureDiff, wantExit: exitDidNotRun,
			edit: func(g *gateFixture) {
				g.tools.repo = func() (string, string, error) {
					return "", "", errors.New("not a git repository")
				}
			},
			wantOutput: []string{"not a git repository"},
		},
		{
			name: "one module passes and one fails",
			args: base, dirs: both, diff: fixtureDiff, mutants: killedEverywhere,
			edit: func(g *gateFixture) {
				g.fake.mutants["tools/x/x.go"] = []fakeMutant{{2, statusKilled}, {3, statusLived}}
			},
			wantExit: exitFailed, wantCalls: 3, wantProbes: 1, wantCopies: 3,
			wantOutput: []string{
				"### Mutation tests: `.` (m)\n\n**PASS**: The tests detected 3 of the 3 mutants " +
					"that ran: 100.00%",
				"### Mutation tests: `tools` (m/tools)\n\n**FAIL**: The tests detected 1 of the 2",
				"| `tools/x/x.go:3:2` | CONDITIONALS_NEGATION | LIVED |",
				"gremlins ran 2 times on 2 packages",
				"- `other_windows.go`: go does not build it here",
				"mutationgate: FAIL\n",
			},
		},
		{
			name: "gremlins fails on one module",
			args: base, dirs: both, diff: fixtureDiff, mutants: killedEverywhere,
			edit: func(g *gateFixture) {
				g.fake.exit = 1
				g.fake.events = "baseline\tx.test\t4s\ncap\tx.test\tran longer than 1m30s " +
					"without a mutant\n"
			},
			wantExit: exitDidNotRun, wantCalls: 2, wantProbes: 1,
			wantOutput: []string{
				"**DID NOT RUN**: gremlins exited 1 on m; x.test: ran longer than 1m30s",
				"**DID NOT RUN**: gremlins exited 1 on m/tools/x",
				"mutationgate: DID NOT RUN\n",
			},
			wantSummary: []string{"- `x.test` (cap): ran longer than 1m30s without a mutant"},
		},
		{
			name: "gremlins finds no mutant",
			args: base, dirs: both, diff: fixtureDiff, mutants: map[string][]fakeMutant{},
			wantExit: exitPassed, wantCalls: 3, wantProbes: 1,
			wantOutput: []string{"hold nothing gremlins mutates"},
		},
		{
			name: "gremlins skips a mutant on a chosen line",
			args: base, dirs: both, diff: fixtureDiff, mutants: killedEverywhere,
			edit:     func(g *gateFixture) { g.fake.skipChosen = true },
			wantExit: exitDidNotRun, wantCalls: 3, wantProbes: 1,
			wantOutput: []string{
				"other lines than the gate chose for 3 mutants",
				"| `a.go:2:2` | CONDITIONALS_NEGATION | SKIPPED |",
			},
		},
		{
			name: "exec could not apply a limit",
			args: base, dirs: both, diff: fixtureDiff, mutants: killedEverywhere,
			edit: func(g *gateFixture) {
				g.fake.events = "error\tx.test\tcannot limit memory to 4096 MiB: EPERM\n"
			},
			wantExit: exitDidNotRun, wantCalls: 3, wantProbes: 1, wantCopies: 3,
			wantOutput: []string{"could not do its part: x.test: cannot limit memory"},
		},
		{
			name: "the tests fail without a mutant in a copy of the module",
			args: base, dirs: both, diff: fixtureDiff, mutants: killedEverywhere,
			edit:     func(g *gateFixture) { g.copyErr = errors.New("exit status 1") },
			wantExit: exitDidNotRun, wantCalls: 2, wantProbes: 1, wantCopies: 2,
			wantOutput: []string{
				"**DID NOT RUN**: the tests of m fail without a mutant in a copy of its module, " +
					"where gremlins tests the mutants, so every mutant would count as killed",
				"skip it with -skip (exit status 1): --- FAIL: TestRepository | FAIL",
				"**DID NOT RUN**: the tests of m/tools/x fail without a mutant",
			},
		},
		{
			name: "every Go file of a directory",
			args: []string{
				"-all",
				"-threshold",
				"0",
			},
			dirs:       []string{"cmd/tool"},
			mutants:    killedEverywhere,
			wantExit:   exitPassed,
			wantCalls:  1,
			wantProbes: 1,
			wantCopies: 1,
			wantOutput: []string{
				"Scope: every Go file of the packages in `cmd/tool`; files: 1.",
				"The tests detected 1 of the 2 mutants that ran: 50.00%, threshold 0%.",
			},
		},
		{
			name: "every Go file: a module with nothing to mutate beside one whose mutants ran",
			args: []string{"-all"}, dirs: both,
			mutants:  map[string][]fakeMutant{"tools/x/x.go": {{2, statusKilled}}},
			wantExit: exitPassed, wantCalls: 3, wantProbes: 1, wantCopies: 1,
			wantOutput: []string{
				"### Mutation tests: `.` (m)\n\n**PASS**: gremlins found nothing to mutate in `.`.",
				"### Mutation tests: `tools` (m/tools)\n\n**PASS**: The tests detected 1 of the 1",
				"mutationgate: PASS\n",
			},
		},
		{
			name: "every Go file: a package with no code beside one with code",
			args: []string{"-all"}, dirs: []string{"."}, mutants: killedEverywhere,
			files: map[string]string{
				"cmd/tool/main.go": "// Command tool is a fixture.\npackage main\n",
			},
			wantExit: exitPassed, wantCalls: 1, wantProbes: 1, wantCopies: 1,
			wantOutput: []string{
				"**PASS**: The tests detected 3 of the 3 mutants that ran: 100.00%",
				"Scope: every Go file of the packages in `.`; files: 2.",
				"gremlins ran 1 times on 1 packages",
			},
		},
		{
			name: "every Go file: no mutant ran in any module",
			args: []string{"-all"}, dirs: both, mutants: map[string][]fakeMutant{},
			wantExit: exitDidNotRun, wantCalls: 3, wantProbes: 1,
			wantOutput: []string{
				"**PASS**: gremlins found nothing to mutate in `tools`.",
				"mutationgate: no mutant ran in any module; with -all, a run that tests no " +
					"mutant does not pass\n",
				"mutationgate: DID NOT RUN\n",
			},
			wantSummary: []string{"**DID NOT RUN**: no mutant ran in any module"},
		},
		{
			name: "only timed-out mutants are detected",
			args: base, dirs: both, diff: fixtureDiff,
			mutants: map[string][]fakeMutant{
				"a.go": {{2, statusTimedOut}}, "cmd/tool/main.go": {{4, statusTimedOut}},
				"tools/x/x.go": {{2, statusTimedOut}},
			},
			wantExit: exitPassed, wantCalls: 3, wantProbes: 1, wantCopies: 3,
			wantOutput: []string{"The tests detected 2 of the 2 mutants that ran: 100.00%"},
		},
		{
			name: "an exec log the gate cannot read",
			args: base, dirs: both, diff: fixtureDiff, mutants: killedEverywhere,
			edit:     func(g *gateFixture) { g.fake.events = "cap only\n" },
			wantExit: exitDidNotRun, wantCalls: 2, wantProbes: 1,
			wantOutput: []string{"**DID NOT RUN**: exec log: unexpected line \"cap only\""},
		},
		{
			name: "a sample whose dry run fails stops its module",
			args: append(slices.Clone(base), "-max-mutants", "400"), dirs: []string{"."},
			diff: rootDiff, mutants: killedEverywhere,
			edit:     func(g *gateFixture) { g.fake.exit = 1 },
			wantExit: exitDidNotRun, wantCalls: 1, wantProbes: 1,
			wantOutput: []string{
				"**DID NOT RUN**: gremlins exited 1 on m\n",
				"gremlins ran 1 times on 1 packages",
			},
		},
		{
			name: "a sample reports the dry run's events and mismatched mutants",
			args: append(slices.Clone(base), "-max-mutants", "1"), dirs: []string{"tools"},
			diff: toolsDiff,
			mutants: map[string][]fakeMutant{
				"tools/x/x.go": {{1, statusKilled}, {3, statusKilled}},
			},
			edit: func(g *gateFixture) {
				g.fake.dryEvents = "baseline\tx.test\t4s\n"
				g.fake.dryOnly = map[string][]fakeMutant{"tools/x/x.go": {{2, statusSkipped}}}
			},
			wantExit: exitDidNotRun, wantCalls: 2, wantProbes: 1, wantCopies: 1,
			wantOutput: []string{
				"other lines than the gate chose for 1 mutants",
				"| `tools/x/x.go:2:2` | CONDITIONALS_NEGATION | SKIPPED |",
				"Slowest test process without a mutant: `x.test`, 4s.",
				"gremlins ran 2 times on 1 packages",
			},
		},
		{
			name: "a sample whose lines that ran hold only mutants that do not compile",
			args: append(slices.Clone(base), "-max-mutants", "1"), dirs: []string{"tools"},
			diff: toolsDiff,
			mutants: map[string][]fakeMutant{
				"tools/x/x.go": {{1, statusNotViable}, {3, statusNotViable}},
			},
			wantExit: exitPassed, wantCalls: 2, wantProbes: 1,
			wantOutput: []string{
				"**PASS**: No mutant of this module that would run is in the sample: the 1 that " +
					"would were left out.",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mutants := map[string][]fakeMutant{}
			maps.Copy(mutants, tt.mutants)
			g := newGateFixture(t, tt.diff, mutants)
			for name, content := range tt.files {
				writeFile(t, g.root, name, content)
			}
			if tt.edit != nil {
				tt.edit(g)
			}
			exit, output, summary := g.run(t, tt.args, tt.dirs...)
			if exit != tt.wantExit {
				t.Errorf("exit %d, want %d\n%s", exit, tt.wantExit, output)
			}
			if len(g.fake.calls) != tt.wantCalls || g.probes != tt.wantProbes ||
				len(g.copyTests) != tt.wantCopies {
				t.Errorf("gremlins ran %d times, the probe %d times and the tests in a copy %d "+
					"times, want %d, %d and %d", len(g.fake.calls), g.probes, len(g.copyTests),
					tt.wantCalls, tt.wantProbes, tt.wantCopies)
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(output, want) {
					t.Errorf("output lacks %q:\n%s", want, output)
				}
			}
			for _, want := range tt.wantSummary {
				if !strings.Contains(summary, want) {
					t.Errorf("summary lacks %q:\n%s", want, summary)
				}
			}
			if exit != exitDidNotRun || tt.wantCalls > 0 {
				if !strings.HasPrefix(summary, "### Mutation tests: ") {
					t.Errorf("summary %q", summary)
				}
			}
		})
	}
}

// gremlins runs once per package, in its module's root, on the package's
// directory, with the lines in scope by file name in that directory, which the git
// shim on its PATH answers its git diff with; the go shim tests the package, also
// package main in a subdirectory.
func TestRunGateRunsGremlinsOncePerPackage(t *testing.T) {
	t.Parallel()
	g := newGateFixture(t, fixtureDiff, map[string][]fakeMutant{
		"a.go": {{2, statusKilled}}, "cmd/tool/main.go": {{4, statusKilled}},
		"tools/x/x.go": {{2, statusKilled}},
	})
	g.fake.viaShim = true
	if exit, output, _ := g.run(t, []string{"-base", "main"}, ".", "tools"); exit != exitPassed {
		t.Fatalf("exit %d\n%s", exit, output)
	}
	toolsDir := filepath.Join(g.root, "tools")
	want := []gremlinsCall{
		{
			dir: g.root, pkgDir: g.root, pkg: "m",
			scope:   map[string][]lineRange{"a.go": {{2, 3}}},
			exclude: []string{"/", `^other_windows\.go$`},
		},
		{
			dir: g.root, pkgDir: filepath.Join(g.root, "cmd", "tool"), pkg: "m/cmd/tool",
			scope: map[string][]lineRange{"main.go": {{4, 4}}}, exclude: []string{"/"},
		},
		{
			dir: toolsDir, pkgDir: filepath.Join(toolsDir, "x"), pkg: "m/tools/x",
			scope: map[string][]lineRange{"x.go": {{1, 5}}}, exclude: []string{"/"},
		},
	}
	if len(g.fake.calls) != len(want) {
		t.Fatalf("calls %+v", g.fake.calls)
	}
	for i, call := range g.fake.calls {
		w := want[i]
		if call.dir != w.dir || call.pkgDir != w.pkgDir || call.pkg != w.pkg || call.dryRun ||
			!slices.Equal(call.exclude, w.exclude) ||
			!maps.EqualFunc(call.scope, w.scope, slices.Equal) {
			t.Errorf("call %d: got %+v, want %+v", i, call, w)
		}
	}
	// Each package's tests ran without a mutant in a copy of its module, with the
	// flags gremlins' test runs get; the root module was copied once.
	wantCopies := []string{". test m", "cmd/tool test m/cmd/tool", "x test m/tools/x"}
	for i, copyTest := range g.copyTests {
		if i >= len(wantCopies) || !strings.HasPrefix(copyTest, wantCopies[i]+" with ") ||
			!strings.Contains(copyTest, "-count=1 -skip=^TestLive -p=2 '-exec=") {
			t.Errorf("test run in a copy %d: %q", i, copyTest)
		}
	}
	if len(g.copyTests) != len(wantCopies) {
		t.Errorf("tests ran in a copy %d times: %q", len(g.copyTests), g.copyTests)
	}
}

// With -max-mutants, a dry run finds the mutants that would run, and gremlins then
// tests only the sampled lines.
func TestRunGateSamples(t *testing.T) {
	t.Parallel()
	mutants := map[string][]fakeMutant{
		"a.go":             {{2, statusKilled}, {3, statusKilled}},
		"cmd/tool/main.go": {{4, statusKilled}, {4, statusKilled}},
		"tools/x/x.go":     {{1, statusKilled}, {2, statusNotCovered}, {3, statusKilled}},
	}
	sampled := func(t *testing.T, head, budget string) (string, []gremlinsCall) {
		t.Helper()
		g := newGateFixture(t, fixtureDiff, maps.Clone(mutants))
		g.tools.repo = func() (string, string, error) { return g.root, head, nil }
		exit, output, _ := g.run(t, []string{"-base", "main", "-max-mutants", budget}, ".",
			"tools")
		if exit != exitPassed {
			t.Fatalf("exit %d\n%s", exit, output)
		}
		return output, g.fake.calls
	}
	// runs counts the dry runs and the runs that test mutants.
	runs := func(calls []gremlinsCall) (int, int) {
		var dry, tested int
		for _, call := range calls {
			if call.dryRun {
				dry++
			} else {
				tested++
			}
		}
		return dry, tested
	}
	output, calls := sampled(t, headSHA, "3")
	if dry, tested := runs(calls); dry != 3 || tested < 2 || tested > 3 {
		t.Errorf("%d dry runs and %d runs that test, want 3 and 2 to 3", dry, tested)
	}
	// With a budget of one mutant, one package's line is tested, and the dry runs
	// of the others are their results.
	if _, oneCalls := sampled(t, headSHA, "1"); len(oneCalls) != 4 || oneCalls[3].dryRun {
		t.Errorf("with a budget of 1: calls %+v", oneCalls)
	}
	// Each module's report states its share of the sample, counted in mutants: the
	// budget takes one of a.go's two lines and cmd/tool/main.go's line with two
	// mutants, and leaves out both lines of the tools module.
	chosenBy := "chosen by commit " + headSHA + " with a budget of 3 mutants in all modules.\n"
	for _, want := range []string{
		"Sample: 3 of the 4 mutants that would run, on 2 of 3 lines, in 2 of 2 files (100% of " +
			"the files), " + chosenBy,
		"Sample: 0 of the 2 mutants that would run, on 0 of 2 lines, in 0 of 1 files (0% of " +
			"the files), " + chosenBy,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output lacks %q:\n%s", want, output)
		}
	}
	again, againCalls := sampled(t, headSHA, "3")
	if !slices.EqualFunc(calls, againCalls, func(a, b gremlinsCall) bool {
		return maps.EqualFunc(a.scope, b.scope, slices.Equal) && a.dryRun == b.dryRun
	}) {
		t.Errorf("one commit gave two samples:\n%s\n%s", output, again)
	}
}

// The gate gives gremlins the go shim, which tests the package the gate names,
// and go test runs at most -workers test processes.
func TestRunGateGivesGremlinsTheGoShim(t *testing.T) {
	t.Parallel()
	diff := changedDiff(map[string]string{"cmd/tool/main.go": "@@ -4 +4 @@\n-x\n+y\n"})
	g := newGateFixture(t, diff, map[string][]fakeMutant{"cmd/tool/main.go": {{4, statusKilled}}})
	var shim []byte
	var goflags string
	fake := g.tools.gremlins
	g.tools.gremlins = func(binary, dir string, args, env []string) (int, error) {
		bin, _, _ := strings.Cut(envValue(env, "PATH"), string(os.PathListSeparator))
		var err error
		if shim, err = os.ReadFile(filepath.Join(bin, "go")); err != nil {
			return 0, err
		}
		goflags = envValue(env, "GOFLAGS")
		return fake(binary, dir, args, env)
	}
	args := []string{"-base", "main", "-workers", "1", "-skip", ""}
	if exit, output, _ := g.run(t, args, "cmd/tool"); exit != exitPassed {
		t.Fatalf("exit %d\n%s", exit, output)
	}
	want := "exec '" + g.tools.self + "' go -real-go='/usr/local/go/bin/go' -log='"
	if !strings.HasPrefix(string(shim), "#!/bin/sh\n"+want) ||
		!strings.Contains(string(shim), " -package='m/cmd/tool' -- \"$@\"\n") {
		t.Errorf("go shim:\n%s", shim)
	}
	// The gate appends its flags to any GOFLAGS it runs with, as when these tests run
	// under the gate itself, which sets its own -skip.
	added := goflags[strings.LastIndex(goflags, "-count=1"):]
	if !strings.HasPrefix(added, "-count=1 -p=1 '-exec=") || strings.Contains(added, "-skip") {
		t.Errorf("GOFLAGS %q", goflags)
	}
}

// With no arguments at all, the gate does not run: it needs the directories.
func TestDispatchWithoutArguments(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	exit := dispatch(nil, &stdout, &stderr)
	if exit != exitDidNotRun || !strings.Contains(stderr.String(), "name the directories") ||
		stdout.String() != "mutationgate: DID NOT RUN\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", exit, stdout.String(), stderr.String())
	}
}

// writeExecutable writes script to an executable file named name in dir.
func writeExecutable(t *testing.T, dir, name, script string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := execfile.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
}

// runGremlins finds a relative -gremlins path from the gate's working directory, not
// from the module directory it runs gremlins in, as CI's -gremlins bin/gremlins
// needs, and returns gremlins' exit code.
func TestRunGremlins(t *testing.T) {
	work := t.TempDir()
	writeExecutable(t, filepath.Join(work, "bin"), "gremlins", "#!/bin/sh\n: >\"$1\"\nexit 3\n")
	moduleDir := t.TempDir()
	t.Chdir(work)
	code, err := runGremlins("bin/gremlins", moduleDir, []string{"ran-here"}, os.Environ())
	if err != nil || code != 3 {
		t.Fatalf("got exit %d, %v; want exit 3", code, err)
	}
	if _, err := os.Stat(filepath.Join(moduleDir, "ran-here")); err != nil {
		t.Errorf("gremlins did not run in %s: %v", moduleDir, err)
	}
	if _, err := runGremlins("bin/missing", moduleDir, nil, os.Environ()); err == nil {
		t.Error("no error for a gremlins that does not exist")
	}
}

// goListModule lists a module and its packages, each with the files go builds with
// the tags given, and fails for a module with no package and outside a module.
func TestGoListModule(t *testing.T) {
	t.Parallel()
	dir, empty := t.TempDir(), t.TempDir()
	for name, content := range map[string]string{
		filepath.Join(dir, "go.mod"):      "module example.com/m\n\ngo 1.21\n",
		filepath.Join(dir, "p", "p.go"):   "package p\n",
		filepath.Join(dir, "p", "e2e.go"): "//go:build e2e\n\npackage p\n",
		filepath.Join(empty, "go.mod"):    "module example.com/empty\n\ngo 1.21\n",
	} {
		if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for tags, want := range map[string][]string{"": {"p.go"}, "e2e": {"e2e.go", "p.go"}} {
		m, err := goListModule(dir, tags)
		if err != nil {
			t.Fatalf("tags %q: %v", tags, err)
		}
		if m.path != "example.com/m" || !sameDir(m.dir, dir) || len(m.pkgs) != 1 ||
			m.pkgs[0].importPath != "example.com/m/p" ||
			!sameDir(m.pkgs[0].dir, filepath.Join(dir, "p")) ||
			!slices.Equal(m.pkgs[0].files, want) {
			t.Errorf("tags %q: got %+v, want example.com/m/p with %q", tags, m, want)
		}
	}
	if _, err := goListModule(empty, ""); err == nil ||
		!strings.Contains(err.Error(), "found no packages") {
		t.Errorf("a module with no package: got %v", err)
	}
	if _, err := goListModule(t.TempDir(), ""); err == nil {
		t.Error("no error outside a module")
	}
}

// goListModule fails on go list output it cannot read, rather than misread it.
func TestGoListModuleRejectsUnexpectedOutput(t *testing.T) {
	bin := t.TempDir()
	writeExecutable(t, bin, "go", "#!/bin/sh\n"+
		"if [ \"$2\" != -m ]; then\n\techo /m\n"+
		"elif [ \"$FAKE_GO\" = no-tab ]; then\n\techo example.com/m\n"+
		"else\n\tprintf 'example.com/m\\t/m\\n'\nfi\n")
	t.Setenv("PATH", bin)
	dir := t.TempDir()
	for fake, want := range map[string]string{
		"no-tab": "go list -m in " + dir + ": unexpected output \"example.com/m\\n\"",
		"":       "go list in " + dir + ": unexpected line \"/m\\n\"",
	} {
		t.Setenv("FAKE_GO", fake)
		if m, err := goListModule(dir, ""); err == nil || err.Error() != want {
			t.Errorf("FAKE_GO=%s: got %+v, %v; want %s", fake, m, err, want)
		}
	}
}

// gitRepo reads the repository root and the commit HEAD names from git rev-parse,
// and fails on output that does not hold exactly those two lines.
func TestGitRepo(t *testing.T) {
	bin := t.TempDir()
	writeExecutable(t, bin, "git", "#!/bin/sh\nprintf '%s' \"$FAKE_GIT_OUTPUT\"\n")
	t.Setenv("PATH", bin)
	const unexpected = "  git rev-parse --show-toplevel HEAD: unexpected output "
	for output, want := range map[string]string{
		"/repo\nabc\n":       "/repo abc <nil>",
		"/repo\n":            unexpected + `"/repo\n"`,
		"/repo\nabc\nmore\n": unexpected + `"/repo\nabc\nmore\n"`,
	} {
		t.Setenv("FAKE_GIT_OUTPUT", output)
		root, head, err := gitRepo()
		if got := fmt.Sprintf("%s %s %v", root, head, err); got != want {
			t.Errorf("git printed %q: got %q, want %q", output, got, want)
		}
	}
}
