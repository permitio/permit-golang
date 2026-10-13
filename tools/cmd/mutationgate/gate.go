package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Defaults bound a run: gremlins' timeout coefficient at most maxCoefficient, and
// each test process at most defaultCap and defaultMemoryMiB. Two workers keep at
// most two mutated test processes running at a time, and go test -p the same
// number of unmutated ones in gremlins' coverage run.
const (
	defaultWorkers     = 2
	defaultCoefficient = 5
	maxCoefficient     = 5
	defaultCap         = 3 * time.Minute
	defaultMemoryMiB   = 4096
	defaultThreshold   = 90
	// defaultSkip names the live tests, which need Permit and never run offline.
	defaultSkip = "^TestLive"
)

// gateOptions is the parsed command line of the gate.
type gateOptions struct {
	base        string
	all         bool
	gremlins    string
	workers     int
	coefficient int
	cap         time.Duration
	memoryMiB   int64
	threshold   float64
	skip        string
	tags        string
	maxMutants  int
	summary     string
	// dirs are the directories to mutate, as given.
	dirs []string
}

// tools are the commands the gate runs, replaced in tests.
type tools struct {
	// self is the path of this executable, which go test runs as -exec.
	self string
	// goPath and gitPath are the paths of the go and git commands, which the go and
	// git subcommands run.
	goPath  string
	gitPath string
	// repo returns the root of the repository the working directory is in, and the
	// commit HEAD names.
	repo func() (root, head string, err error)
	// diff returns git diff -U0 from the merge base with base to the work tree, run
	// in root.
	diff func(root, base string) ([]byte, error)
	// module returns the module whose root is dir, with the packages go builds with
	// tags.
	module func(dir, tags string) (goModule, error)
	// probe runs a process that allocates without end under the limits, and
	// returns how much it allocated before it was stopped.
	probe func(opts gateOptions) (stoppedAtMiB int64, err error)
	// gremlins runs gremlins with args and env in dir, streaming its output, and
	// returns its exit code.
	gremlins func(binary, dir string, args, env []string) (int, error)
	// goTest runs the go command with args and env in dir, and returns its output,
	// and an error when it fails.
	goTest func(dir string, args, env []string) (string, error)
}

func systemTools() (tools, error) {
	self, err := os.Executable()
	if err != nil {
		return tools{}, fmt.Errorf("cannot find the path of this executable: %w", err)
	}
	paths := map[string]string{"go": "", "git": ""}
	for name := range paths {
		p, err := exec.LookPath(name)
		if err == nil {
			p, err = filepath.Abs(p)
		}
		if err != nil {
			return tools{}, fmt.Errorf("cannot find the %s command: %w", name, err)
		}
		paths[name] = p
	}
	return tools{
		self:     self,
		goPath:   paths["go"],
		gitPath:  paths["git"],
		repo:     gitRepo,
		diff:     gitDiff,
		module:   goListModule,
		probe:    func(opts gateOptions) (int64, error) { return probeMemoryLimit(self, opts) },
		gremlins: runGremlins,
		goTest: func(dir string, args, env []string) (string, error) {
			return runGoTest(paths["go"], dir, args, env)
		},
	}, nil
}

// runGate works out which lines of which packages to mutate, checks that the
// memory limit holds, runs gremlins once per package and reports the result of
// each module. Anything that keeps it from measuring the tests counts as not
// having run, so a broken setup cannot pass.
func runGate(args []string, stdout, stderr io.Writer, t tools) int {
	out := sink{stdout: stdout, stderr: stderr}
	opts, err := parseGateArgs(args, stderr)
	if err != nil {
		return out.didNotRun(err)
	}
	out.summary = opts.summary
	for what, p := range map[string]string{
		"path of this executable": t.self, "go command": t.goPath, "git command": t.gitPath,
	} {
		if pathErr := checkPlainPath(what, p); pathErr != nil {
			return out.didNotRun(pathErr)
		}
	}
	root, head, err := t.repo()
	if err != nil {
		return out.didNotRun(err)
	}
	p, err := planRun(t, opts, root)
	if err != nil {
		return out.didNotRun(err)
	}
	if len(p.units) == 0 {
		return finishAll(out, p.reports(opts, "", nil))
	}

	stoppedAt, err := t.probe(opts)
	if err != nil {
		return out.didNotRun(fmt.Errorf("memory limit check: %w", err))
	}
	limits := fmt.Sprintf("%d workers, go test -p %d, timeout coefficient %d, each test process "+
		"at most %s (half that without a mutant), %d MiB and %d MiB of output (a probe "+
		"allocating without end was stopped by the memory limit after %d MiB)",
		opts.workers, opts.workers, opts.coefficient, opts.cap, opts.memoryMiB, outputBudget>>20,
		stoppedAt)
	_, _ = fmt.Fprintf(stdout, "mutationgate: %s\n", limits)

	dir, err := os.MkdirTemp("", "mutationgate-")
	if err != nil {
		return out.didNotRun(err)
	}
	defer removeWorkDir(dir, stderr)
	if err := checkPlainPath("work directory", dir); err != nil {
		return out.didNotRun(err)
	}
	r := runner{opts: opts, tools: t, dir: dir, stdout: stdout}
	res := r.runUnits(p, head)
	return finishAll(out, p.reports(opts, limits, &res))
}

// sink is where the gate reports: stdout, stderr and the job summary file.
type sink struct {
	stdout, stderr io.Writer
	summary        string
}

// didNotRun reports a run that did not complete before any module was measured,
// and returns exitDidNotRun.
func (s sink) didNotRun(err error) int {
	_, _ = fmt.Fprintf(s.stderr, "mutationgate: %v\n", err)
	if s.summary != "" {
		text := fmt.Sprintf("### Mutation tests\n\n**DID NOT RUN**: %s\n\n", err)
		if summaryErr := appendFile(s.summary, text); summaryErr != nil {
			_, _ = fmt.Fprintf(s.stderr, "mutationgate: job summary: %v\n", summaryErr)
		}
	}
	_, _ = fmt.Fprintln(s.stdout, "mutationgate: DID NOT RUN")
	return exitDidNotRun
}

// stopReasons lists the test processes exec stopped, and the errors exec and the
// shims logged, for the message of a gremlins run that failed.
func stopReasons(events []event) string {
	var reasons []string
	for _, e := range events {
		if e.kind != eventBaseline {
			reasons = append(reasons, e.binary+": "+e.detail)
		}
	}
	if len(reasons) == 0 {
		return ""
	}
	return "; " + strings.Join(reasons, "; ")
}

func removeWorkDir(dir string, stderr io.Writer) {
	if err := os.RemoveAll(dir); err != nil {
		_, _ = fmt.Fprintf(stderr, "mutationgate: could not remove %s: %v\n", dir, err)
	}
}

// plainValue matches a -skip value that GOFLAGS carries as one word.
var plainValue = regexp.MustCompile(`^[^\s'"\\]*$`)

// buildTags matches a -tags value: comma-separated build tags.
var buildTags = regexp.MustCompile(`^[A-Za-z0-9_.,-]*$`)

func parseGateArgs(args []string, stderr io.Writer) (gateOptions, error) {
	fs := flag.NewFlagSet("mutationgate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var opts gateOptions
	fs.StringVar(&opts.base, "base", "",
		"mutate the lines changed since the merge base with this git ref")
	fs.BoolVar(&opts.all, "all", false, "mutate every Go file of the packages in the directories")
	fs.StringVar(&opts.gremlins, "gremlins", "gremlins", "gremlins executable")
	fs.IntVar(&opts.workers, "workers", defaultWorkers,
		"mutants tested, and go test -p, at a time (at least 1)")
	fs.IntVar(&opts.coefficient, "timeout-coefficient", defaultCoefficient,
		"gremlins' timeout: the coverage run's time times this (1 to 5)")
	fs.DurationVar(&opts.cap, "cap", defaultCap, "longest one test process may run")
	fs.Int64Var(&opts.memoryMiB, "memory-mib", defaultMemoryMiB,
		"most memory one test process may use, in MiB")
	fs.Float64Var(&opts.threshold, "threshold", defaultThreshold,
		"lowest percentage of mutants that ran the tests must detect (0 to 100)")
	fs.StringVar(&opts.skip, "skip", defaultSkip,
		"go test -skip pattern for the tests that never run offline (empty: none)")
	fs.StringVar(&opts.tags, "tags", "", "comma-separated build tags for go list and go test")
	fs.IntVar(&opts.maxMutants, "max-mutants", 0,
		"most mutants to test, sampled by the commit HEAD names (0: no limit)")
	fs.StringVar(&opts.summary, "summary", "", "file to append the Markdown report to")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	opts.dirs = fs.Args()
	switch {
	case len(opts.dirs) == 0:
		return opts, errors.New("name the directories to mutate, such as . tools")
	case opts.all == (opts.base != ""):
		return opts, errors.New("give exactly one of -base <ref> and -all")
	case opts.workers < 1:
		return opts, fmt.Errorf("-workers %d: need at least 1 (gremlins reads 0 as one per CPU)",
			opts.workers)
	case opts.coefficient < 1 || opts.coefficient > maxCoefficient:
		return opts, fmt.Errorf("-timeout-coefficient %d: need 1 to %d",
			opts.coefficient, maxCoefficient)
	case opts.cap <= 0:
		return opts, fmt.Errorf("-cap %s: need a positive duration", opts.cap)
	case opts.memoryMiB < 1:
		return opts, fmt.Errorf("-memory-mib %d: need at least 1", opts.memoryMiB)
	case opts.threshold < 0 || opts.threshold > 100:
		return opts, fmt.Errorf("-threshold %g: need 0 to 100", opts.threshold)
	case opts.maxMutants < 0:
		return opts, fmt.Errorf("-max-mutants %d: need 0 (no limit) or more", opts.maxMutants)
	case !plainValue.MatchString(opts.skip):
		return opts, fmt.Errorf("-skip %q has a space, quote or backslash, which GOFLAGS cannot "+
			"carry", opts.skip)
	case !buildTags.MatchString(opts.tags):
		return opts, fmt.Errorf("-tags %q: want comma-separated build tags", opts.tags)
	}
	if _, err := regexp.Compile(opts.skip); err != nil {
		return opts, fmt.Errorf("-skip %q: %w", opts.skip, err)
	}
	return opts, nil
}

// checkPlainPath rejects a path that GOFLAGS' quoting, go test's splitting of the
// -exec value or the shims' shell quoting would break.
func checkPlainPath(what, p string) error {
	if p == "" {
		return fmt.Errorf("cannot find the %s", what)
	}
	if strings.ContainsAny(p, " \t\n'\"\\") {
		return fmt.Errorf("%s %q has a space, quote or backslash, which GOFLAGS and the shims "+
			"cannot carry", what, p)
	}
	return nil
}

// lineRange is a run of added or edited lines, first to last.
type lineRange struct{ first, last int }

var hunkHeader = regexp.MustCompile(`^@@ -[0-9]+(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@`)

// parseAddedLines reads git diff -U0 output and returns, for each non-test Go file
// with added or edited lines, those lines. With no context lines each hunk's new
// side is one run of added lines, and a hunk that only deletes has a count of 0.
// Each hunk's body is skipped by its line counts, so a removed or added line that
// starts with "--- " or "+++ " is not read as a file header.
func parseAddedLines(diff []byte) (map[string][]lineRange, error) {
	changed := map[string][]lineRange{}
	file := ""
	bodyLeft := 0
	sc := bufio.NewScanner(bytes.NewReader(diff))
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, `\`):
			continue // "\ No newline at end of file" belongs to neither side of a hunk.
		case bodyLeft > 0:
			if line == "" || !strings.ContainsAny(line[:1], "+- ") {
				return nil, fmt.Errorf("git diff: hunk ends early at %q", line)
			}
			bodyLeft--
		case strings.HasPrefix(line, "+++ "):
			name := strings.TrimPrefix(line, "+++ ")
			switch {
			case name == "/dev/null":
				file = ""
			case strings.HasPrefix(name, "b/"):
				file = strings.TrimPrefix(name, "b/")
			default:
				return nil, fmt.Errorf("git diff: cannot read the file name in %q", line)
			}
			if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
				file = ""
			}
		case strings.HasPrefix(line, "@@ "):
			m := hunkHeader.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("git diff: cannot read the hunk header %q", line)
			}
			removed, first, added := hunkCount(m[1]), hunkCount(m[2]), hunkCount(m[3])
			bodyLeft = removed + added
			if file != "" && added > 0 {
				changed[file] = append(changed[file], lineRange{first, first + added - 1})
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	if bodyLeft > 0 {
		return nil, fmt.Errorf("git diff: the last hunk is missing %d lines", bodyLeft)
	}
	return changed, nil
}

// hunkCount reads a hunk header number; git omits a line count of 1.
func hunkCount(s string) int {
	if s == "" {
		return 1
	}
	n, _ := strconv.Atoi(s)
	return n
}

// buildGremlinsArgs returns gremlins' command line for one run in the package
// directory pkgDir: --diff scopeRef, which the git shim answers with the lines in
// scope, and the files of the directory to leave out.
func buildGremlinsArgs(opts gateOptions, dryRun bool, w workFiles, pkgDir string,
	exclude []string,
) []string {
	args := []string{
		"unleash",
		"--config", w.config,
		"--workers", strconv.Itoa(opts.workers),
		"--timeout-coefficient", strconv.Itoa(opts.coefficient),
		"--output", w.results,
		"--diff", scopeRef,
	}
	if dryRun {
		args = append(args, "--dry-run")
	}
	if opts.tags != "" {
		args = append(args, "--tags", opts.tags)
	}
	for _, pattern := range exclude {
		args = append(args, "--exclude-files", pattern)
	}
	return append(args, pkgDir)
}

// gremlinsEnv returns the environment for gremlins: the gate's own, without
// GREMLINS_ settings, with the shims first on PATH and go test flags added to
// GOFLAGS.
func gremlinsEnv(environ []string, self string, opts gateOptions, w workFiles) []string {
	var env []string
	goflags := ""
	searchPath := []string{w.bin}
	for _, kv := range environ {
		key, value, _ := strings.Cut(kv, "=")
		switch {
		case strings.HasPrefix(key, "GREMLINS_"):
			continue
		case key == "GOFLAGS":
			goflags = value
			continue
		case key == "PATH":
			if value != "" {
				searchPath = append(searchPath, value)
			}
			continue
		}
		env = append(env, kv)
	}
	flags := []string{goflags, "-count=1"}
	if opts.skip != "" {
		flags = append(flags, "-skip="+opts.skip)
	}
	flags = append(flags, fmt.Sprintf("-p=%d", opts.workers),
		fmt.Sprintf("'-exec=%s %s -cap=%s -memory-mib=%d -log=%s'",
			self, commandExec, opts.cap, opts.memoryMiB, w.events))
	goflags = strings.TrimSpace(strings.Join(flags, " "))
	return append(env, "PATH="+strings.Join(searchPath, string(os.PathListSeparator)),
		"GOFLAGS="+goflags)
}

// isolatedGitVars are the environment variables that could change what the gate's
// git diff prints: an external diff tool, for one, leaves it an empty diff.
var isolatedGitVars = []string{
	"GIT_EXTERNAL_DIFF", "GIT_DIFF_OPTS", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM",
	"GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT",
}

// gitIsolation keeps git from reading the user's and the system's git config, such
// as diff.algorithm.
var gitIsolation = []string{"GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1"}

// isolateGit returns environ without isolatedGitVars and with gitIsolation.
func isolateGit(environ []string) []string {
	var env []string
	for _, kv := range environ {
		key, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(isolatedGitVars, key) {
			env = append(env, kv)
		}
	}
	return append(env, gitIsolation...)
}

// gitOutput runs git with args in dir, without the user's and the system's git
// config.
func gitOutput(dir string, args ...string) ([]byte, error) {
	//nolint:gosec // G204: the gate runs git with the ref its command line names.
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = isolateGit(os.Environ())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// gitRepo returns the root of the repository the working directory is in, and the
// commit HEAD names.
func gitRepo() (string, string, error) {
	out, err := gitOutput("", "rev-parse", "--show-toplevel", "HEAD")
	if err != nil {
		return "", "", err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		return "", "", fmt.Errorf("git rev-parse --show-toplevel HEAD: unexpected output %q", out)
	}
	return lines[0], lines[1], nil
}

// gitDiff runs git diff -U0 in root, with paths relative to root, without the
// user's and the system's git config.
func gitDiff(root, base string) ([]byte, error) {
	return gitOutput(root, "diff", "--merge-base", "-U0", "--no-color", "--no-ext-diff",
		"--no-relative", "--src-prefix=a/", "--dst-prefix=b/", base)
}

// goListModule runs go list in dir, which must be a module root, and returns the
// module and the packages go builds with tags.
func goListModule(dir, tags string) (goModule, error) {
	modOut, err := goList(dir, "-m", "-f", "{{.Path}}\t{{.Dir}}")
	if err != nil {
		return goModule{}, err
	}
	path, root, ok := strings.Cut(strings.TrimSpace(modOut), "\t")
	if !ok {
		return goModule{}, fmt.Errorf("go list -m in %s: unexpected output %q", dir, modOut)
	}
	args := []string{"-f", "{{.Dir}}\t{{.ImportPath}}" +
		"{{range .GoFiles}}\t{{.}}{{end}}{{range .CgoFiles}}\t{{.}}{{end}}"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	pkgOut, err := goList(dir, append(args, "./...")...)
	if err != nil {
		return goModule{}, err
	}
	m := goModule{path: path, dir: root}
	for line := range strings.Lines(pkgOut) {
		fields := strings.Split(strings.TrimRight(line, "\n"), "\t")
		if len(fields) < 2 {
			return goModule{}, fmt.Errorf("go list in %s: unexpected line %q", dir, line)
		}
		m.pkgs = append(m.pkgs, goPackage{
			dir: fields[0], importPath: fields[1], files: fields[2:],
		})
	}
	if len(m.pkgs) == 0 {
		return goModule{}, fmt.Errorf("go list ./... in %s found no packages", dir)
	}
	return m, nil
}

func sameDir(a, b string) bool {
	aInfo, errA := os.Stat(a)
	bInfo, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(aInfo, bInfo)
}

func goList(dir string, args ...string) (string, error) {
	//nolint:gosec // G204: the gate lists the packages with the tags its command line names.
	cmd := exec.CommandContext(context.Background(), "go", append([]string{"list"}, args...)...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go list %s in %s: %w: %s",
			strings.Join(args, " "), dir, err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// runGremlins runs binary, found from the gate's working directory or on PATH, in
// dir.
func runGremlins(binary, dir string, args, env []string) (int, error) {
	found, err := exec.LookPath(binary)
	if err == nil {
		found, err = filepath.Abs(found)
	}
	if err != nil {
		return 0, err
	}
	//nolint:gosec // G204: the gate runs the gremlins its command line names.
	cmd := exec.CommandContext(context.Background(), found, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitErr.ExitCode(), nil
	}
	return 0, err
}
