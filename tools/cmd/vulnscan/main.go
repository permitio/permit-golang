// Command vulnscan runs govulncheck on the SDK and on the tools module, prints a report, and fails
// on every vulnerability that govulncheck reports outside the standard library unless an accept
// file lists it with a reason and an expiry. It was copied from the Terraform provider for
// Permit.io and adapted to this repository.
//
// Usage, from the repository root:
//
//	vulnscan -ceiling dir [-govulncheck path] [-accept file] [-trees file] [-findings file]
//	vulnscan -only floor|tools [-govulncheck path] [-accept file] [-trees file] [-findings file]
//	vulnscan -only ceiling -ceiling dir [-govulncheck path] [-accept file] [-trees file]
//	    [-findings file]
//	vulnscan -consumer dir [-govulncheck path] [-accept file] [-findings file]
//
// vulnscan scans three trees, each with govulncheck -mode source -scan symbol -format json and the
// tree's package patterns:
//
//   - floor: the repository root, the SDK with the dependency versions its go.mod requires, the
//     lowest an application can build it with; ./...
//   - ceiling: the directory that -ceiling names, a copy of the root in which every dependency was
//     moved to its newest version with go get -u -t ./... and go mod tidy; ./...
//   - tools: tools/, the programs that build and check the SDK; ./... tool, so that the programs
//     the tool directives of tools/go.mod pin, such as govulncheck, are scanned too. The root has
//     no tool directive, and there tool would match no package.
//
// Every other directory with a go.mod that git tracks, outside testdata directories, must be listed
// in the trees file with the reason it is not scanned, such as a test-only module that never
// ships. The trees file has one directory per line, relative to the repository root, followed by
// the reason.
//
// -only scans one of the three trees, so that each gets a result of its own, as the scheduled
// security scan reports them. -consumer scans only the module in dir, with ./..., and checks no
// module of the repository: a module outside the repository, such as a throwaway one that
// requires the latest release of the SDK and imports its packages, the way an application
// resolves it. With either, the accept entries are not checked for staleness, since an entry may
// accept a vulnerability of a tree the run does not scan. -findings writes each vulnerability to
// the file as a JSON line in the format secreport (tools/cmd/secreport) reads, with no severity,
// since the Go vulnerability database has none. The fix of an accepted vulnerability is "accepted
// until <last day>: <reason>".
//
// govulncheck itself fails only on vulnerable code that is called, and with -format json it exits 0
// whatever it finds. vulnscan also fails on a vulnerable module that is required or imported but
// not called, so each one is either fixed or accepted on the record; called code cannot be
// accepted. A finding in the standard library never fails: it depends on the Go release that builds
// the application, not on the SDK, so vulnscan reports it with the release that fixes it.
//
// The accept file has one accepted vulnerability per line: its ID, the module it is in, the reason
// and the last day the entry applies, in UTC as YYYY-MM-DD, separated by spaces. An accepted
// vulnerability is still printed, with its reason. An entry fails after its last day, and an entry
// whose vulnerability no tree reports any more fails once every scan completed, so the file holds
// no entry that is out of date. In both files, blank lines and lines that start with # are ignored.
//
// A scan is complete when govulncheck exits 0 or 3 and its output holds its config, of source mode
// and symbol level, a module list, and vulnerability database entries. The module list must have
// every module that go list -deps lists for the tree's package patterns, other than a module
// replaced by a local directory: govulncheck scans such a module's code but leaves it out of its
// module list.
//
// Exit codes:
//
//	0  every vulnerability outside the standard library is accepted, or there is none
//	1  a vulnerability is not accepted, called code is vulnerable, an accept entry expired or is
//	   stale, the trees file lists a directory without a module, or govulncheck exited 3
//	2  vulnscan did not run: the command line, the accept file or the trees file is wrong, git
//	   cannot list the modules, a module is neither scanned nor listed in the trees file, or a
//	   scan did not complete
//
// The last line vulnscan prints is its verdict: PASS, FAIL or DID NOT RUN. Build it, and the
// govulncheck that tools/go.mod pins, before use, because go run reports every non-zero exit as 1.
// .github/scripts/vuln.sh does that and prepares the ceiling:
//
//	go -C tools build -o ../bin/govulncheck golang.org/x/vuln/cmd/govulncheck
//	go -C tools build -o ../bin/vulnscan ./cmd/vulnscan
//	bin/vulnscan -ceiling <dir>
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	exitPassed    = 0
	exitFailed    = 1
	exitDidNotRun = 2
)

// govulncheckFound is govulncheck's exit code when it finds vulnerabilities in its text output.
// With -format json it exits 0 and the findings are in the output.
const govulncheckFound = 3

// stdlib is the module path govulncheck gives the standard library.
const stdlib = "stdlib"

// vulnIDPattern matches a Go vulnerability database ID.
var vulnIDPattern = regexp.MustCompile(`^GO-[0-9]{4}-[0-9]{4,}$`)

// output is what a command printed and its exit code.
type output struct {
	stdout, stderr []byte
	code           int
}

// command runs the program name with args in dir. It returns an error only when the program could
// not be started or did not exit on its own.
type command func(ctx context.Context, dir, name string, args ...string) (output, error)

// system is what vulnscan uses from the machine it runs on: the current time and a way to run
// commands.
type system struct {
	now     time.Time
	command command
}

func main() {
	sys := system{now: time.Now(), command: runCommand}
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, sys))
}

func runCommand(ctx context.Context, dir, name string, args ...string) (output, error) {
	//nolint:gosec // G204: vulnscan runs git, go and the govulncheck named on its command line.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	result := output{stdout: stdout.Bytes(), stderr: stderr.Bytes()}
	if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) && exitErr.Exited() {
		result.code = exitErr.ExitCode()
		return result, nil
	}
	return result, err
}

// tree is a module that vulnscan scans, with the package patterns go list and govulncheck get.
type tree struct {
	name, dir string
	patterns  []string
}

// options is the parsed command line, with the accept and trees files read.
type options struct {
	govulncheck string
	trees       []tree
	accepted    []acceptEntry
	// skipped maps each directory of the trees file to the reason it is not scanned.
	skipped    map[string]string
	treesFile  string
	acceptFile string
	// partial says which tree a run of one tree scans; it is empty when the run scans all three.
	partial string
	// consumer is set when the run scans a module outside the repository.
	consumer bool
	// findings is the file to write the findings to, when it is set.
	findings string
}

// run parses the command line, scans every tree and prints the report. A command line or file it
// cannot use counts as not having run, so a mistyped CI step cannot pass.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, sys system) int {
	opts, err := parseArgs(args, stderr)
	if err != nil {
		fmt.Fprintf(stdout, "vulnscan: DID NOT RUN: %v\nvulnscan: DID NOT RUN\n", err)
		return exitDidNotRun
	}
	year, month, day := sys.now.UTC().Date()
	r := &report{
		opts:  opts,
		today: time.Date(year, month, day, 0, 0, 0, 0, time.UTC),
		vulns: map[vulnKey]*vuln{},
	}
	if !opts.consumer {
		r.checkModules(ctx, sys.command)
	}
	for _, t := range opts.trees {
		r.scan(ctx, sys.command, t)
	}
	code := r.print(stdout)
	if opts.findings != "" {
		if err := r.writeFindings(opts.findings); err != nil {
			fmt.Fprintf(stdout, "vulnscan: DID NOT RUN: writing the findings: %v\n"+
				"vulnscan: DID NOT RUN\n", err)
			return exitDidNotRun
		}
	}
	return code
}

func parseArgs(args []string, stderr io.Writer) (options, error) {
	flags := flag.NewFlagSet("vulnscan", flag.ContinueOnError)
	flags.SetOutput(stderr)
	ceiling := flags.String("ceiling", "",
		"the `directory` of the ceiling tree: a copy of the root with every dependency updated")
	govulncheck := flags.String("govulncheck", filepath.Join("bin", "govulncheck"),
		"the govulncheck `binary`, built from the version tools/go.mod pins")
	acceptFile := flags.String("accept", filepath.Join(".github", "vulnscan-accept.txt"),
		"the accept `file`")
	treesFile := flags.String("trees", filepath.Join(".github", "vulnscan-trees.txt"),
		"the `file` that lists the modules vulnscan does not scan")
	only := flags.String("only", "", "scan only this `tree`: floor, ceiling or tools")
	consumer := flags.String("consumer", "",
		"scan only the module in this `directory`, which is outside the repository")
	findings := flags.String("findings", "", "write the findings to this `file` as JSON lines")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected arguments %q", flags.Args())
	}
	trees := []tree{
		{name: "floor", dir: ".", patterns: []string{"./..."}},
		{name: "ceiling", dir: *ceiling, patterns: []string{"./..."}},
		{name: "tools", dir: "tools", patterns: []string{"./...", "tool"}},
	}
	opts := options{findings: *findings}
	switch {
	case *consumer != "" && (*only != "" || *ceiling != ""):
		return options{}, errors.New("-consumer scans one module; drop -only and -ceiling")
	case *consumer != "":
		trees = []tree{{name: "consumer", dir: *consumer, patterns: []string{"./..."}}}
		opts.consumer = true
		opts.partial = "only the consumer module was scanned"
	case *only == "":
	case *only != "floor" && *only != "ceiling" && *only != "tools":
		return options{}, fmt.Errorf("-only %q is not floor, ceiling or tools", *only)
	default:
		index := slices.IndexFunc(trees, func(t tree) bool { return t.name == *only })
		trees = trees[index : index+1]
		opts.partial = "only the " + *only + " tree was scanned"
	}
	for _, t := range trees {
		info, err := os.Stat(filepath.Join(t.dir, "go.mod"))
		switch {
		case t.name == "ceiling" && *ceiling == "":
			return options{}, errors.New("-ceiling is missing: name the copy of the root in " +
				"which go get -u -t ./... and go mod tidy moved every dependency to its newest " +
				"version")
		case t.name == "ceiling" && (err != nil || !info.Mode().IsRegular()):
			return options{}, fmt.Errorf("-ceiling %s has no go.mod", *ceiling)
		case t.name == "consumer" && (err != nil || !info.Mode().IsRegular()):
			return options{}, fmt.Errorf("-consumer %s has no go.mod", *consumer)
		}
	}
	if *ceiling != "" && !slices.ContainsFunc(trees, func(t tree) bool {
		return t.name == "ceiling"
	}) {
		return options{}, fmt.Errorf("-ceiling is set, but -only %s does not scan it", *only)
	}
	binary, err := filepath.Abs(*govulncheck)
	if err != nil {
		return options{}, err
	}
	if info, statErr := os.Stat(binary); statErr != nil || !info.Mode().IsRegular() {
		return options{}, fmt.Errorf("govulncheck %s is missing; build it with "+
			"go -C tools build -o ../bin/govulncheck golang.org/x/vuln/cmd/govulncheck",
			*govulncheck)
	}
	if opts.accepted, err = readAcceptFile(*acceptFile); err != nil {
		return options{}, err
	}
	if !opts.consumer {
		if opts.skipped, err = readTreesFile(*treesFile); err != nil {
			return options{}, err
		}
	}
	opts.govulncheck, opts.trees = binary, trees
	opts.treesFile, opts.acceptFile = *treesFile, *acceptFile
	return opts, nil
}

// acceptEntry is one line of the accept file.
type acceptEntry struct {
	id, module, reason string
	// expiry is the last day the entry applies, at midnight UTC.
	expiry time.Time
}

// readLines calls parse with the fields of each line of the file that is not blank or a comment,
// and returns the first error parse returns, with the file's name and the line's number.
func readLines(name string, parse func(fields []string) error) error {
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	lines := bufio.NewScanner(file)
	for number := 1; lines.Scan(); number++ {
		line := strings.TrimSpace(lines.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if err := parse(strings.Fields(line)); err != nil {
			return fmt.Errorf("%s:%d: %w", name, number, err)
		}
	}
	if err := lines.Err(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func readAcceptFile(name string) ([]acceptEntry, error) {
	var entries []acceptEntry
	err := readLines(name, func(fields []string) error {
		if len(fields) < 4 {
			return errors.New("want an ID, a module, a reason and the last day the entry " +
				"applies (YYYY-MM-DD)")
		}
		entry := acceptEntry{
			id:     fields[0],
			module: fields[1],
			reason: strings.Join(fields[2:len(fields)-1], " "),
		}
		last := fields[len(fields)-1]
		switch {
		case !vulnIDPattern.MatchString(entry.id):
			return fmt.Errorf("%q is not a Go vulnerability ID such as GO-2026-1234", entry.id)
		case entry.module == stdlib:
			return fmt.Errorf("%s names the standard library, whose findings never fail; remove "+
				"the entry", entry.id)
		}
		expiry, err := time.Parse(time.DateOnly, last)
		if err != nil {
			return fmt.Errorf("the last field, %q, is not a date such as 2026-12-31", last)
		}
		entry.expiry = expiry
		for _, other := range entries {
			if other.id == entry.id && other.module == entry.module {
				return fmt.Errorf("%s in %s is listed twice", entry.id, entry.module)
			}
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func readTreesFile(name string) (map[string]string, error) {
	skipped := map[string]string{}
	err := readLines(name, func(fields []string) error {
		if len(fields) < 2 {
			return errors.New("want a directory and the reason its module is not scanned")
		}
		dir := fields[0]
		switch {
		case path.IsAbs(dir) || path.Clean(dir) != dir || dir == ".." ||
			strings.HasPrefix(dir, "../"):
			return fmt.Errorf("%q is not a clean path relative to the repository root, such as "+
				"examples/consumer", dir)
		case dir == "." || dir == "tools":
			return fmt.Errorf("%s is a tree vulnscan scans; remove the entry", dir)
		case skipped[dir] != "":
			return fmt.Errorf("%s is listed twice", dir)
		}
		skipped[dir] = strings.Join(fields[1:], " ")
		return nil
	})
	if err != nil {
		return nil, err
	}
	return skipped, nil
}

// level is how far govulncheck traced a vulnerability into the scanned code.
type level int

const (
	levelRequired level = iota // the module is in the build; no vulnerable package is imported
	levelImported              // a vulnerable package is imported; no vulnerable symbol is called
	levelCalled                // a vulnerable symbol is called
)

// levelNames are the names of the levels, in order.
var levelNames = [...]string{"required, not imported", "imported, not called", "called"}

func (l level) String() string {
	return levelNames[l]
}

// vulnKey identifies a vulnerability in one module: one ID can affect more than one module, such
// as the standard library and a golang.org/x module.
type vulnKey struct {
	id, module string
}

// vuln is one vulnerability in one module, across every scanned tree.
type vuln struct {
	vulnKey
	summary string
	level   level
	symbols []string
	// found lists the trees that report the vulnerability, each with the module's version there
	// and the version that fixes it, which can differ: Go 1.26 and Go 1.27 get separate fixes.
	found []string
	// versions and fixes are the module's versions and the versions that fix it, without
	// duplicates, for the findings file.
	versions, fixes []string
}

// message is one object of govulncheck's -format json output, a stream of objects that each have
// one field.
type message struct {
	Config  *scanConfig `json:"config"`
	SBOM    *sbom       `json:"SBOM"` //nolint:tagliatelle // govulncheck's name.
	OSV     *osvEntry   `json:"osv"`
	Finding *finding    `json:"finding"`
}

type scanConfig struct {
	ScannerVersion string `json:"scanner_version"`
	DBLastModified string `json:"db_last_modified"`
	ScanLevel      string `json:"scan_level"`
	ScanMode       string `json:"scan_mode"`
}

type sbom struct {
	GoVersion string `json:"go_version"`
	Modules   []struct {
		Path string `json:"path"`
	} `json:"modules"`
}

type osvEntry struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
}

type finding struct {
	OSV          string  `json:"osv"`
	FixedVersion string  `json:"fixed_version"`
	Trace        []frame `json:"trace"`
}

type frame struct {
	Module   string `json:"module"`
	Version  string `json:"version"`
	Package  string `json:"package"`
	Function string `json:"function"`
	Receiver string `json:"receiver"`
}

// report collects the module check and the scans of every tree, and decides the exit code.
type report struct {
	opts  options
	today time.Time
	lines []string
	vulns map[vulnKey]*vuln
	// scanned counts the trees whose scan completed.
	scanned int
	// incomplete is set when the modules could not be listed, a module is not scanned, or a
	// scan did not complete.
	incomplete bool
	// failed is set when a rule fails that is not about one vulnerability: govulncheck exited 3,
	// or the trees file lists a directory without a module.
	failed bool
}

func (r *report) logf(format string, args ...any) {
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
}

// checkModules lists the go.mod files git tracks, and checks that each module outside testdata is
// a tree vulnscan scans or is listed in the trees file, and that each listed directory is a module.
func (r *report) checkModules(ctx context.Context, cmd command) {
	out, err := cmd(ctx, ".", "git", "ls-files", "-z", "--", "go.mod", "*/go.mod")
	if err == nil && out.code != 0 {
		err = fmt.Errorf("exit %d: %s", out.code, strings.TrimSpace(string(out.stderr)))
	}
	if err != nil {
		r.incomplete = true
		r.logf("modules: git ls-files could not list the go.mod files: %v", err)
		return
	}
	modules := map[string]bool{}
	for name := range strings.SplitSeq(string(out.stdout), "\x00") {
		dir := path.Dir(name)
		if name == "" || strings.Contains("/"+dir+"/", "/testdata/") {
			continue
		}
		modules[dir] = true
	}
	for _, scanned := range []string{".", "tools"} {
		if !modules[scanned] {
			r.incomplete = true
			r.logf("modules: git tracks no %s; vulnscan scans the root and tools/",
				path.Join(scanned, "go.mod"))
		}
	}
	for _, dir := range slices.Sorted(maps.Keys(modules)) {
		reason, listed := r.opts.skipped[dir]
		switch {
		case dir == "." || dir == "tools":
		case listed:
			r.logf("modules: %s is not scanned: %s", dir, reason)
		default:
			r.incomplete = true
			r.logf("modules: %s has a go.mod, but vulnscan does not scan it and %s does not list "+
				"it; list it there with the reason it never ships, or scan it", dir,
				r.opts.treesFile)
		}
	}
	for _, dir := range slices.Sorted(maps.Keys(r.opts.skipped)) {
		if !modules[dir] {
			r.failed = true
			r.logf("modules: FAIL, %s lists %s, which has no go.mod that git tracks; remove "+
				"the entry", r.opts.treesFile, dir)
		}
	}
}

// moduleTemplate is the go list template that prints the module of each package of the build,
// once per package, except a module that a replace directive points at a local directory: such a
// replacement has no version, and govulncheck leaves it out of its module list while it scans its
// code with the rest of the build.
const moduleTemplate = "{{with .Module}}{{if or (not .Replace) .Replace.Version}}{{.Path}}" +
	"{{end}}{{end}}"

// scan runs go list and govulncheck on one tree and adds the findings to the report.
func (r *report) scan(ctx context.Context, cmd command, t tree) {
	golist := "go list -deps " + strings.Join(t.patterns, " ")
	listed, err := cmd(ctx, t.dir, "go", slices.Concat(
		[]string{"list", "-deps", "-f", moduleTemplate}, t.patterns)...)
	if err == nil && listed.code != 0 {
		err = fmt.Errorf("exit %d: %s", listed.code, strings.TrimSpace(string(listed.stderr)))
	}
	if err != nil {
		r.incomplete = true
		r.logf("%s: %s could not list the modules of the build: %v", t.name, golist, err)
		return
	}
	want := map[string]bool{}
	for module := range strings.FieldsSeq(string(listed.stdout)) {
		want[module] = true
	}
	if len(want) == 0 {
		r.incomplete = true
		r.logf("%s: %s lists no module: the tree has no packages to scan", t.name, golist)
		return
	}

	out, err := cmd(ctx, t.dir, r.opts.govulncheck, slices.Concat(
		[]string{"-mode", "source", "-scan", "symbol", "-format", "json"}, t.patterns)...)
	switch {
	case err != nil:
		r.incomplete = true
		r.logf("%s: could not run govulncheck: %v", t.name, err)
		return
	case out.code == govulncheckFound:
		r.failed = true
		r.logf("%s: govulncheck exit 3: vulnerabilities found", t.name)
	case out.code != 0:
		r.incomplete = true
		r.logf("%s: govulncheck exit %d: %s", t.name, out.code,
			strings.TrimSpace(string(out.stderr)))
		return
	}
	if err := r.parse(t, out.stdout, want); err != nil {
		r.incomplete = true
		r.logf("%s: %v", t.name, err)
		return
	}
	r.scanned++
}

// parse reads one govulncheck -format json output and adds its findings to the report when the
// output is complete: govulncheck writes its config, then the modules it found, then the
// vulnerability database entries for them, and the standard library always has some.
func (r *report) parse(t tree, data []byte, want map[string]bool) error {
	var config *scanConfig
	var modules *sbom
	summaries := map[string]string{}
	var findings []finding
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var m message
		err := decoder.Decode(&m)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("govulncheck output is not complete JSON: %w", err)
		}
		switch {
		case m.Config != nil:
			config = m.Config
		case m.SBOM != nil:
			modules = m.SBOM
		case m.OSV != nil:
			summaries[m.OSV.ID] = m.OSV.Summary
		case m.Finding != nil:
			if m.Finding.OSV == "" || len(m.Finding.Trace) == 0 {
				return errors.New("govulncheck output has a finding without an ID or a trace")
			}
			findings = append(findings, *m.Finding)
		}
	}

	switch {
	case config == nil:
		return errors.New("govulncheck output has no config: it is empty or not -format json")
	case config.ScanMode != "source":
		return fmt.Errorf("govulncheck output is of scan mode %q, want source", config.ScanMode)
	case config.ScanLevel != "symbol":
		return fmt.Errorf("govulncheck output is of scan level %q, want symbol", config.ScanLevel)
	case modules == nil:
		return errors.New("govulncheck output has no module list: it was cut short")
	case len(summaries) == 0:
		return errors.New("govulncheck output has no vulnerability database entries: it was " +
			"cut short")
	}
	found := map[string]bool{}
	for _, m := range modules.Modules {
		found[m.Path] = true
	}
	var missing []string
	for module := range want {
		if !found[module] {
			missing = append(missing, module)
		}
	}
	patterns := strings.Join(t.patterns, " ")
	if len(missing) > 0 {
		slices.Sort(missing)
		return fmt.Errorf("govulncheck found %d modules, but its module list lacks %d of the %d "+
			"that go list -deps %s lists: %s", len(modules.Modules), len(missing), len(want),
			patterns, strings.Join(missing, ", "))
	}

	for _, f := range findings {
		r.add(t, f, summaries[f.OSV])
	}
	r.logf("%s: %d modules, among them the %d that go list -deps %s lists; govulncheck %s, "+
		"%s, database of %s", t.name, len(modules.Modules), len(want), patterns,
		config.ScannerVersion, modules.GoVersion, config.DBLastModified)
	return nil
}

// add merges one finding into the report. govulncheck reports a vulnerability at the module level
// first, then again for each package and symbol it traces it to.
func (r *report) add(t tree, f finding, summary string) {
	top := f.Trace[0]
	key := vulnKey{id: f.OSV, module: top.Module}
	v, ok := r.vulns[key]
	if !ok {
		v = &vuln{vulnKey: key, summary: summary}
		r.vulns[key] = v
	}
	found := levelRequired
	switch {
	case top.Function != "":
		found = levelCalled
		symbol := path.Base(top.Package) + "."
		if top.Receiver != "" {
			symbol += strings.TrimPrefix(top.Receiver, "*") + "."
		}
		symbol += top.Function
		if !slices.Contains(v.symbols, symbol) {
			v.symbols = append(v.symbols, symbol)
		}
	case top.Package != "":
		found = levelImported
	}
	v.level = max(v.level, found)
	fixed := "no fixed version"
	if f.FixedVersion != "" {
		fixed = "fixed in " + version(top.Module, f.FixedVersion)
		v.fixes = appendNew(v.fixes, version(top.Module, f.FixedVersion))
	}
	v.versions = appendNew(v.versions, version(top.Module, top.Version))
	where := strings.TrimSpace(t.name+" "+version(top.Module, top.Version)) + ", " + fixed
	if !slices.Contains(v.found, where) {
		v.found = append(v.found, where)
	}
}

// version names a module version as its module spells it: the standard library's v1.27.1 is
// go1.27.1.
func version(module, v string) string {
	if module == stdlib && v != "" {
		return "go" + strings.TrimPrefix(v, "v")
	}
	return v
}

// print writes the report and returns the exit code.
func (r *report) print(w io.Writer) int {
	var out strings.Builder
	for _, line := range r.lines {
		out.WriteString("vulnscan: " + line + "\n")
	}

	keys := slices.SortedFunc(maps.Keys(r.vulns), func(a, b vulnKey) int {
		return strings.Compare(a.id+" "+a.module, b.id+" "+b.module)
	})
	toFix, accepted, standard := 0, 0, 0
	for _, k := range keys {
		if k.module == stdlib {
			continue
		}
		if r.printVuln(&out, r.vulns[k]) {
			accepted++
		} else {
			toFix++
		}
	}
	for _, k := range keys {
		if k.module == stdlib {
			r.printVuln(&out, r.vulns[k])
			standard++
		}
	}

	// An entry is stale only when every scan of every tree completed: a scan that did not, or a
	// tree that was not scanned, may hold the vulnerability it accepts.
	allScanned := r.scanned == len(r.opts.trees) && r.opts.partial == ""
	expired, stale := 0, 0
	for _, e := range r.opts.accepted {
		_, reported := r.vulns[vulnKey{id: e.id, module: e.module}]
		switch {
		case r.today.After(e.expiry):
			expired++
			fmt.Fprintf(&out, "\n%s  FAIL, the accept entry for %s expired on %s; fix the "+
				"vulnerability, or renew the entry with a reason that still holds\n",
				e.id, e.module, e.expiry.Format(time.DateOnly))
		case allScanned && !reported:
			stale++
			fmt.Fprintf(&out, "\n%s  FAIL, stale accept entry: no tree reports it in %s; "+
				"remove it from %s\n", e.id, e.module, r.opts.acceptFile)
		}
	}
	switch {
	case allScanned || len(r.opts.accepted) == 0:
	case r.opts.partial != "":
		fmt.Fprintf(&out, "\nvulnscan: accept entries not checked for staleness: %s\n",
			r.opts.partial)
	default:
		out.WriteString("\nvulnscan: accept entries not checked for staleness: not every scan " +
			"completed\n")
	}

	code := exitPassed
	switch {
	case r.incomplete:
		code = exitDidNotRun
	case toFix > 0 || expired > 0 || stale > 0 || r.failed:
		code = exitFailed
	}
	verdict := map[int]string{
		exitPassed:    "PASS",
		exitFailed:    "FAIL",
		exitDidNotRun: "DID NOT RUN",
	}
	fmt.Fprintf(&out, "\nvulnscan: %d to fix, %d accepted, %d expired and %d stale accept "+
		"entries, %d in the standard library (reported only); %d of %d trees scanned\n",
		toFix, accepted, expired, stale, standard, r.scanned, len(r.opts.trees))
	fmt.Fprintf(&out, "vulnscan: %s\n", verdict[code])
	_, _ = io.WriteString(w, out.String())
	return code
}

// appendNew appends s to list unless s is empty or in the list already.
func appendNew(list []string, s string) []string {
	if s == "" || slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}

// findingLine is one line of the findings file, in the format secreport reads.
type findingLine struct {
	ID      string `json:"id"`
	Package string `json:"package"`
	Version string `json:"version"`
	Fix     string `json:"fix"`
	URL     string `json:"url"`
}

// writeFindings writes every vulnerability the scans found to the file, one JSON line each.
func (r *report) writeFindings(name string) error {
	keys := slices.SortedFunc(maps.Keys(r.vulns), func(a, b vulnKey) int {
		return strings.Compare(a.id+" "+a.module, b.id+" "+b.module)
	})
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	for _, k := range keys {
		v := r.vulns[k]
		fix := strings.Join(v.fixes, ", ")
		if entry, _, accepted := r.acceptedBy(v); accepted {
			fix = "accepted until " + entry.expiry.Format(time.DateOnly) + ": " + entry.reason
		}
		err := encoder.Encode(findingLine{
			ID: v.id, Package: v.module, Version: strings.Join(v.versions, ", "),
			Fix: fix, URL: "https://pkg.go.dev/vuln/" + v.id,
		})
		if err != nil {
			return err
		}
	}
	return os.WriteFile(name, b.Bytes(), 0o600)
}

// acceptedBy returns the accept entry that lists the vulnerability, whether there is one, and
// whether it accepts the vulnerability: the entry has not expired, and the code is not called.
func (r *report) acceptedBy(v *vuln) (entry acceptEntry, listed, accepted bool) {
	for _, e := range r.opts.accepted {
		if e.id == v.id && e.module == v.module {
			entry, listed = e, true
		}
	}
	accepted = listed && !r.today.After(entry.expiry) && v.level != levelCalled
	return entry, listed, accepted
}

// printVuln writes one vulnerability and reports whether it is accepted.
func (r *report) printVuln(out io.Writer, v *vuln) bool {
	entry, listed, isAccepted := r.acceptedBy(v)
	expired := listed && r.today.After(entry.expiry)
	verdict := "FAIL, " + v.level.String() + ", not accepted"
	switch {
	case v.module == stdlib:
		verdict = "reported only, the standard library, " + v.level.String()
	case isAccepted:
		verdict = "accepted, " + v.level.String()
	case listed && v.level == levelCalled:
		verdict = "FAIL, called; the accept entry does not apply: called code cannot be accepted"
	case expired:
		verdict += ": its accept entry expired on " + entry.expiry.Format(time.DateOnly)
	}
	fmt.Fprintf(out, "\n%s  %s\n    %s in %s\n    %s\n", v.id, verdict, v.module,
		strings.Join(v.found, "; "), v.summary)
	if len(v.symbols) > 0 {
		fmt.Fprintf(out, "    calls: %s\n", strings.Join(v.symbols, ", "))
	}
	if isAccepted {
		fmt.Fprintf(out, "    accepted until %s: %s\n", entry.expiry.Format(time.DateOnly),
			entry.reason)
	}
	return isAccepted
}
