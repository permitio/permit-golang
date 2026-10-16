// Command apisurface checks changes to the SDK's public surface: that the migration guide
// covers every part of the v1 API, and that a breaking change to the v2 API is labelled as one.
//
// Usage:
//
//	apisurface -module <module path>@<version> [-spec file] [-out file]
//	apisurface -mode migration [-root dir] [-ids-base rev] [-write]
//	apisurface -mode diff -base rev -apidiff file [-breaking-label] [-root dir]
//	apisurface -mode pr -base rev [-apidiff file] [-breaking-label] [-root dir]
//	apisurface -mode release -version vX.Y.Z -gorelease file [-root dir]
//
// -root names the repository root, the current directory by default; the paths below are
// relative to it. apisurface runs git and the go command, and in the modes that need them the
// apidiff and gorelease that tools/go.mod pins, built with go -C tools build.
//
// # The v1 surface
//
// -module writes tools/migration/v1-surface.txt (-out), the parts of the v1 API that the
// migration check counts, from one module version, github.com/permitio/permit-golang@v1.3.0. It
// downloads the version's zip with go mod download, which verifies it against the checksum
// database (GOSUMDB), into an empty module cache of its own; it refuses to run with GOSUMDB=off,
// and runs the go command with GOENV=off and without GONOSUMDB, GOPRIVATE, GONOPROXY, GOINSECURE
// and GOFLAGS. It reads three blocks from the zip's Go files, tests left out:
//
//   - a: every exported type, function, constant, variable and method of pkg/api, pkg/config,
//     pkg/enforcement, pkg/errors and pkg/permit, as <package>.<name> or
//     <package>.<type>.<method>.
//   - b: the operations of the generated client in pkg/openapi that only it sends: those that the
//     packages of block a never call through a service of its APIClient, and every operation
//     whose route the API spec (-spec, tools/specs/control-plane.json) does not have, each as
//     openapi.<service>.<operation> with its route.
//   - c: every exported type of pkg/models, as models.<name>.
//
// A missing package, a file with a build constraint, an operation whose route cannot be read
// and an empty block stop it.
//
// # Migration mode
//
// Every v1 item, a line of v1-surface.txt, must be covered exactly once: by an entry of
// MIGRATION.md whose v1: line names it, or by a line of tools/migration/pending.txt, which
// assigns it to the area of v2 that will write its entry. A models type (block c) is also
// covered when v2's models package exports a type of the same name. Migration mode fails when:
//
//   - an item is covered by nothing, or more than once: by two entries, two pending lines, or an
//     entry and a pending line;
//   - a pending line matches only models types that v2 exports under the same name, which need
//     no pending line; a pattern may match such a type as long as it matches another item;
//   - a v1: line's reference or a pending line's pattern matches no item;
//   - an entry is malformed. An entry starts with its anchor, <a id="M-<area>-<n>"></a>, alone on
//     a line, then its heading, ### M-<area>-<n>: <title>, under one of the bucket sections
//     ## Compatibility, ## API and ## Wire behaviour. Its fields are list items: one or more
//     "- v1: " lines, exactly one "- v2: " line, and the mark "- Bug fix: could never work
//     before" at most once. A v1: or v2: line that starts with a backquote names a reference in
//     backquotes, and is otherwise prose. A v1: reference is an id or a pattern (path.Match) of
//     v1-surface.txt;
//   - a v2: line names, in backquotes, optionally after "replaced by ", a declaration that v2's
//     permit or models package does not export, as <package>.<name> or
//     <package>.<type>.<name>. v2's API is read from testdata/api-surface.txt, which
//     TestSurfaceSnapshot keeps in step with the source, and so is the list of v2's models
//     types;
//   - the v2: line of an entry that covers v1 items names no v2 declaration and is not a reason
//     from the vocabulary: "not exposed in v2", alone or followed by ": " and one of account
//     administration, dashboard or CLI helper, Elements back-end, operator route, deprecated
//     upstream or EAP;
//   - a line of an entry, from its heading to the next anchor or heading, contains secret,
//     exposes or vulnerab, in any case;
//   - tools/migration/ids.txt does not list exactly the IDs of the entries, each once; with
//     -ids-base, a commit after rev up to HEAD, or the working tree, drops an ID from the
//     ids.txt before it: an ID may only be added;
//   - a link of MIGRATION.md, CHANGELOG.md or README.md to an anchor of one of them, #<anchor>
//     in the same file or <file>#<anchor>, names no anchor of that file: the id of an
//     <a id="..."> tag, or the anchor that GitHub gives a heading, its text in lower case,
//     without punctuation other than - and _ and with a hyphen for each space, then -1, -2 and
//     so on when headings before have the same anchor. Only inline links, [text](target), and
//     headings that start with # are read, so link an anchor inline; reference-style links and
//     HTML links are not checked. A fenced code block holds no link, anchor or heading. Links to
//     other files and to URLs are not checked;
//   - MIGRATION.md has no section "## Calls that could never work in v1", or the section does
//     not link, as #<ID>, each entry marked "Bug fix: could never work before", or it links an
//     entry without the mark. The exceptions, which apisurface lists with their reasons, are the
//     calls that v1.3.0 fixed and v1's OPA mode, which v2 drops: the section must link each of
//     them, and none may have the mark;
//   - the models appendix of MIGRATION.md, between <!-- apisurface:models:begin --> and
//     <!-- apisurface:models:end -->, differs from the one apisurface writes: a table of each
//     models type of v1 that an entry or a v2 type of the same name covers, and the number of
//     types still pending. -write rewrites it.
//
// # Diff mode
//
// Diff mode compares the working tree with the commit -base, both of the same module path, and
// finds the breaking changes: the incompatible changes that apidiff -m (-apidiff) reports for
// the module's packages but internal ones; each validation rule whose ID is in the working
// tree's testdata/validation-rules.txt and not in the base's, because a new rule rejects input
// the base accepted; and each field of a models type of the base's tools/drift/model-map.yaml
// that the working tree's marks required: true where the base's had it not required, or did not
// have it. A models type that is new in the map breaks nothing. apisurface reads only the flag
// at models.<type>.fields.<field>.required of the map, which may be missing from both commits.
// With -breaking-label, which says that the pull request has the breaking label, breaking
// changes are listed and pass. Diff mode also fails when a commit after -base, or the working
// tree, drops an ID from ids.txt.
//
// # Pull request mode
//
// Pull request mode runs migration mode with -ids-base set to -base when the go.mod of -base
// names another module path than the working tree's, as on the pull request that moves the SDK
// to a new major version, and diff mode otherwise.
//
// # Release mode
//
// Release mode checks the release of -version with gorelease (-gorelease), against the newest
// git tag of the same major version before it: with -base=none -version=<version> when there is
// none; with -base=<tag> when that tag is a prerelease, which lists incompatible changes and
// allows them; and with -base=<tag> -version=<version> otherwise, which fails on an incompatible
// change and on a version that does not follow from the changes. It also fails while
// tools/migration/pending.txt has a line that is not blank or a comment: the release needs every
// v1 item to have its entry in MIGRATION.md.
//
// The last line apisurface prints is its verdict: PASS, FAIL or DID NOT RUN, then a colon and
// what it found. The exit codes:
//
//	0  the v1 surface was written, or the check passed
//	1  the check failed: migration mode found a problem, diff mode found a breaking change
//	   without -breaking-label or a dropped entry ID, or gorelease rejected the version or
//	   pending.txt is not empty
//	2  apisurface did not run: the command line is wrong; a file, a commit or the module cannot
//	   be read; the download or its verification failed; v1-surface.txt or the v2 surface cannot
//	   be parsed; or apidiff or gorelease did not complete
//
// Build it before use, because go run reports every non-zero exit as 1:
//
//	go -C tools build -o ../bin/apisurface ./cmd/apisurface
//	bin/apisurface -mode migration
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	exitPassed    = 0
	exitFailed    = 1
	exitDidNotRun = 2
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, system{}))
}

// options is the command line.
type options struct {
	module, spec, out                  string
	mode, root, idsBase, base, apidiff string
	gorelease, version                 string
	write, breakingLabel               bool
}

// modeFlags lists the flags each mode takes, -module's included.
var modeFlags = map[string][]string{
	"module":    {"module", "spec", "out"},
	"migration": {"mode", "root", "ids-base", "write"},
	"diff":      {"mode", "root", "base", "apidiff", "breaking-label"},
	"pr":        {"mode", "root", "base", "apidiff", "breaking-label"},
	"release":   {"mode", "root", "version", "gorelease"},
}

// run parses the command line and runs its mode.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, sys system) int {
	opts, err := parseArgs(args, stderr)
	if err != nil {
		printVerdict(stdout, "DID NOT RUN", err.Error())
		return exitDidNotRun
	}
	if opts.module != "" {
		if err := generate(ctx, opts, stdout, sys); err != nil {
			printVerdict(stdout, "DID NOT RUN", err.Error())
			return exitDidNotRun
		}
		return exitPassed
	}
	r := &report{out: stdout}
	switch opts.mode {
	case "migration":
		migrationMode(ctx, opts, sys, r)
	case "diff":
		diffMode(ctx, opts, sys, r)
	case "pr":
		prMode(ctx, opts, sys, r)
	default:
		releaseMode(ctx, opts, sys, r)
	}
	return r.finish()
}

func parseArgs(args []string, stderr io.Writer) (options, error) {
	var opts options
	flags := flag.NewFlagSet("apisurface", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.module, "module", "",
		"write the v1 surface of this `module path@version`")
	flags.StringVar(&opts.spec, "spec", "tools/specs/control-plane.json",
		"the API spec `file` that block b's routes are looked up in")
	flags.StringVar(&opts.out, "out", surfaceFile, "the v1 surface `file` to write")
	flags.StringVar(&opts.mode, "mode", "", "the check: migration, diff, pr or release")
	flags.StringVar(&opts.root, "root", ".", "the repository root `directory`")
	flags.StringVar(&opts.idsBase, "ids-base", "",
		"check that ids.txt only grew in the commits after this `rev`")
	flags.BoolVar(&opts.write, "write", false, "rewrite the models appendix of MIGRATION.md")
	flags.StringVar(&opts.base, "base", "", "the base `rev` of the pull request")
	flags.StringVar(&opts.apidiff, "apidiff", "", "the apidiff `binary`")
	flags.BoolVar(&opts.breakingLabel, "breaking-label", false,
		"the pull request has the breaking label")
	flags.StringVar(&opts.version, "version", "", "the `version` to release, such as v2.1.0")
	flags.StringVar(&opts.gorelease, "gorelease", "", "the gorelease `binary`")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected arguments %q", flags.Args())
	}
	mode := opts.mode
	switch {
	case opts.module != "" && mode != "":
		return options{}, errors.New("give -module or -mode, not both")
	case opts.module != "":
		mode = "module"
	case modeFlags[mode] == nil || mode == "module":
		return options{}, fmt.Errorf("-mode %q is not migration, diff, pr or release; or give "+
			"-module", mode)
	}
	var extra []string
	flags.Visit(func(f *flag.Flag) {
		if !slices.Contains(modeFlags[mode], f.Name) {
			extra = append(extra, "-"+f.Name)
		}
	})
	if len(extra) > 0 {
		return options{}, fmt.Errorf("%s does not take %s", modeName(mode),
			strings.Join(extra, ", "))
	}
	return opts, checkModeArgs(mode, opts)
}

// modeName names a mode for a message.
func modeName(mode string) string {
	if mode == "module" {
		return "-module"
	}
	return "-mode " + mode
}

// checkModeArgs checks the flags a mode needs.
func checkModeArgs(mode string, opts options) error {
	switch {
	case mode == "module" && (opts.spec == "" || opts.out == ""):
		return errors.New("-spec and -out must name files")
	case opts.root == "":
		return errors.New("-root is empty")
	case (mode == "diff" || mode == "pr") && opts.base == "":
		return fmt.Errorf("-mode %s needs -base, the pull request's base commit", mode)
	case mode == "diff" && opts.apidiff == "":
		return errors.New("-mode diff needs -apidiff, the apidiff binary")
	case mode == "release" && (opts.version == "" || opts.gorelease == ""):
		return errors.New("-mode release needs -version and -gorelease, the gorelease binary")
	}
	return nil
}

// report collects what a check prints and decides its exit code.
type report struct {
	out      io.Writer
	problems []string
	// notRun is the reason the check did not run, if it did not.
	notRun error
	// passed is the verdict's text when the check passed.
	passed string
}

// failf records a problem.
func (r *report) failf(format string, args ...any) {
	r.problems = append(r.problems, fmt.Sprintf(format, args...))
}

// printVerdict prints the verdict, such as FAIL, and what was found as the last line. When msg
// has more than one line, such as the output of a command, it prints msg whole first, and its
// first line in the verdict.
func printVerdict(w io.Writer, verdict, msg string) {
	msg = strings.TrimRight(msg, " \n")
	first, _, multiline := strings.Cut(msg, "\n")
	if multiline {
		fmt.Fprintln(w, msg)
	}
	fmt.Fprintf(w, "%s: %s\n", verdict, first)
}

// finish prints the problems and the verdict, and returns the exit code.
func (r *report) finish() int {
	if r.notRun != nil {
		printVerdict(r.out, "DID NOT RUN", r.notRun.Error())
		return exitDidNotRun
	}
	for _, p := range r.problems {
		fmt.Fprintf(r.out, "- %s\n", p)
	}
	if len(r.problems) > 0 {
		fmt.Fprintf(r.out, "FAIL: %s\n", counted(len(r.problems), "problem"))
		return exitFailed
	}
	fmt.Fprintf(r.out, "PASS: %s\n", r.passed)
	return exitPassed
}

// migrationMode runs the migration check.
func migrationMode(ctx context.Context, opts options, sys system, r *report) {
	in, err := readMigration(opts.root)
	if err != nil {
		r.notRun = err
		return
	}
	problems, cov := checkMigration(in)
	r.problems = problems
	if opts.idsBase != "" {
		dropped, err := sys.checkIDsGrow(ctx, opts.root, opts.idsBase, in.ids)
		if err != nil {
			r.notRun = err
			return
		}
		r.problems = append(r.problems, dropped...)
	}
	if in.guide.endLine != 0 {
		content, changed := in.guide.withAppendix(appendix(in.items, cov, in.v2))
		switch {
		case changed && opts.write:
			if err := writeFileAtomic(
				filepath.Join(opts.root, migrationFile),
				content,
			); err != nil {
				r.notRun = err
				return
			}
			fmt.Fprintf(r.out, "wrote the models appendix of %s\n", migrationFile)
		case changed:
			r.failf("the models appendix of %s is not up to date; run apisurface -mode "+
				"migration -write", migrationFile)
		}
	}
	counts := blockCounts(in.items)
	byEntry, byPending, bySameName, uncovered := 0, 0, 0, 0
	for _, it := range in.items {
		switch {
		case len(cov.entries[it.id]) > 0:
			byEntry++
		case sameNameInV2(it, in.v2):
			bySameName++
		case len(cov.pending[it.id]) > 0:
			byPending++
		default:
			uncovered++
		}
	}
	byNothing := ""
	if uncovered > 0 {
		byNothing = fmt.Sprintf(", %d covered by nothing", uncovered)
	}
	fmt.Fprintf(r.out, "v1 surface: %d identifiers (a), %d operations (b), %d models types (c); "+
		"%d in entries, %d pending, %d by a v2 models type of the same name%s; %d entries, %d "+
		"pending lines\n", counts[blockIdentifiers], counts[blockOperations], counts[blockModels],
		byEntry, byPending, bySameName, byNothing, len(in.guide.entries), len(in.pending))
	r.passed = fmt.Sprintf("each of the %d v1 items is covered once, every entry is well "+
		"formed, and every link to an anchor resolves", len(in.items))
}

// diffMode runs the diff check.
func diffMode(ctx context.Context, opts options, sys system, r *report) {
	modulePath, err := sys.modulePathAt(ctx, opts.root, "")
	if err != nil {
		r.notRun = err
		return
	}
	runDiff(ctx, opts, sys, r, modulePath)
}

// runDiff finds the breaking changes and the dropped entry IDs since -base.
func runDiff(ctx context.Context, opts options, sys system, r *report, modulePath string) {
	changes, notes, err := diff(ctx, opts, sys, modulePath)
	if err != nil {
		r.notRun = err
		return
	}
	current, problems, err := readIDsFile(opts.root)
	if err != nil {
		r.notRun = err
		return
	}
	r.problems = append(r.problems, problems...)
	dropped, err := sys.checkIDsGrow(ctx, opts.root, opts.base, current)
	if err != nil {
		r.notRun = err
		return
	}
	r.problems = append(r.problems, dropped...)
	for _, note := range notes {
		fmt.Fprintln(r.out, note)
	}
	if len(changes) == 0 {
		r.passed = "no breaking change since " + opts.base
		return
	}
	fmt.Fprintf(r.out, "breaking changes since %s:\n", opts.base)
	for _, c := range changes {
		fmt.Fprintf(r.out, "  %s: %s\n", c.source, c.text)
	}
	if opts.breakingLabel {
		r.passed = counted(len(changes), "breaking change") +
			", and the pull request has the breaking label"
		return
	}
	r.failf("%s, and the pull request does not have the breaking label: add the label when "+
		"the change is intended, and describe it in CHANGELOG.md",
		counted(len(changes), "breaking change"))
}

// counted returns n and the noun, in the plural unless n is 1, such as "2 problems".
func counted(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// readIDsFile reads the working tree's ids.txt.
func readIDsFile(root string) ([]string, []string, error) {
	f, err := os.Open(filepath.Join(root, idsFile))
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	return parseIDs(f, idsFile)
}

// prMode runs migration mode when -base is of another module path, and diff mode otherwise.
func prMode(ctx context.Context, opts options, sys system, r *report) {
	head, err := sys.modulePathAt(ctx, opts.root, "")
	if err != nil {
		r.notRun = err
		return
	}
	base, err := sys.modulePathAt(ctx, opts.root, opts.base)
	if err != nil {
		r.notRun = err
		return
	}
	if base != head {
		fmt.Fprintf(r.out, "the base is %s and the working tree %s: migration mode\n", base, head)
		opts.idsBase = opts.base
		migrationMode(ctx, opts, sys, r)
		return
	}
	fmt.Fprintf(r.out, "the base and the working tree are %s: diff mode\n", head)
	if opts.apidiff == "" {
		r.notRun = errors.New("diff mode needs -apidiff, the apidiff binary")
		return
	}
	runDiff(ctx, opts, sys, r, head)
}

// releaseMode runs the release check.
func releaseMode(ctx context.Context, opts options, sys system, r *report) {
	modulePath, err := sys.modulePathAt(ctx, opts.root, "")
	if err != nil {
		r.notRun = err
		return
	}
	res, err := checkRelease(ctx, opts, sys, modulePath)
	if err != nil {
		r.notRun = err
		return
	}
	fmt.Fprintf(r.out, "gorelease %s\n", strings.Join(res.args, " "))
	for _, line := range res.incompatible {
		fmt.Fprintf(r.out, "  incompatible: %s\n", line)
	}
	for _, line := range res.summary {
		fmt.Fprintf(r.out, "  %s\n", line)
	}
	switch {
	case !res.valid:
		r.failf("gorelease rejects %s against %s", opts.version, res.base)
	case res.allowed && len(res.incompatible) > 0:
		r.passed = fmt.Sprintf("%s: %s since the prerelease %s, allowed after a prerelease",
			opts.version, counted(len(res.incompatible), "incompatible change"), res.base)
	default:
		r.passed = fmt.Sprintf("gorelease accepts %s against %s", opts.version, res.base)
	}
	pending, err := countPending(opts.root)
	if err != nil {
		r.notRun = err
		return
	}
	if pending > 0 {
		r.failf("%s still has %s: the release needs every v1 item to have its entry in %s; "+
			"write the entries and remove the lines", pendingFile, counted(pending, "line"),
			migrationFile)
	}
}
