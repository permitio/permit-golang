// Command zipcheck builds the module zip of the commit that HEAD names, the way the Go module
// proxy builds it for a version tagged on that commit, and checks what the zip holds. The module
// zip is what every application that requires the SDK downloads.
//
// Usage:
//
//	zipcheck [-repo dir]
//
// -repo names a directory in the repository, the current directory by default. zipcheck checks
// the whole repository from any directory in it, so go -C tools run ./cmd/zipcheck checks it too.
//
// zipcheck lists the committed files and their content with git archive, as the proxy does, so
// the commit's .gitattributes applies, and files that are not committed are not in the zip:
// commit a change before you check it. golang.org/x/mod/zip then leaves out what a module zip
// never holds: the files of nested modules, directories with a go.mod of their own such as tools/
// and consumer/, most files under vendor/, and symbolic links. The zip's module path is the one
// the committed go.mod names, and its version is v<N>.0.0 for the path's major version N.
//
// zipcheck checks these rules:
//
//   - Every file has a name that a module zip can hold, as golang.org/x/mod/zip checks: not one
//     with an element that Windows reserves, such as aux, or a character such as ':'. The module
//     proxy cannot serve a commit with such a file, and zipcheck checks no other rule then.
//   - Every file is on the allow list: Go source files, files in testdata directories, go.mod,
//     go.sum, LICENSE, README.md, MIGRATION.md, CHANGELOG.md and llms.txt, the governance files
//     AGENTS.md, CODE_OF_CONDUCT.md, CONTRIBUTING.md and SECURITY.md, the configuration files at
//     the root (each file whose name starts with a dot, and _typos.toml), and the files under
//     .github. All but the Go sources, testdata and .github are at the root only.
//   - No file is an image or an API specification, wherever it is, and no file is under tools/,
//     consumer/ or skills/, which a go.mod of their own, or export-ignore in .gitattributes,
//     keeps out of the zip.
//   - Every file outside testdata directories is text: valid UTF-8 without a NUL byte.
//   - LICENSE is in the zip: pkg.go.dev shows the documentation of a module only when it detects
//     a license it allows.
//   - No file is larger than the per-file cap, uncompressed, unless an exception in limits.go
//     names it with a cap of its own and the reason; every exception names a file of the zip.
//   - The zip is no larger, compressed, than the budget in limits.go: a size measured on a given
//     day plus headroom, with the reason.
//
// It prints the module path and version, the number of files, the zip's size and its budget, and
// then the 10 largest files with their sizes uncompressed and compressed, on every run that
// builds the zip.
//
// Exit codes:
//
//	0  every rule holds
//	1  a rule does not hold
//	2  zipcheck did not run: the command line is wrong, the limits in limits.go are not valid,
//	   the directory is not in a git repository, git cannot archive HEAD, the commit has no
//	   go.mod at its root or it names no valid module path, the zip cannot be built for a reason
//	   other than a file it cannot hold, the zip cannot be read, or it holds no file
//
// The last line zipcheck prints is its verdict: PASS, FAIL or DID NOT RUN. Build it before use,
// because go run reports every non-zero exit as 1:
//
//	go -C tools build -o ../bin/zipcheck ./cmd/zipcheck
//	bin/zipcheck
package main

import (
	"archive/zip"
	"bytes"
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

const (
	exitPassed    = 0
	exitFailed    = 1
	exitDidNotRun = 2
)

// largestShown is the number of the zip's largest files that zipcheck prints.
const largestShown = 10

// rootFiles are the files the zip may hold at its root, besides Go source files and the files
// whose name starts with a dot.
var rootFiles = []string{
	"go.mod", "go.sum", "LICENSE", "README.md", "MIGRATION.md", "CHANGELOG.md", "llms.txt",
	// Governance: how to take part, report a vulnerability, and work in the repository.
	"AGENTS.md", "CODE_OF_CONDUCT.md", "CONTRIBUTING.md", "SECURITY.md",
	// The configuration of typos, which reads it from the root under this name.
	"_typos.toml",
}

// excludedDirs are the directories at the root whose files must stay out of the zip. A go.mod of
// its own keeps a directory out, as the zip leaves out nested modules, and so does export-ignore
// in .gitattributes, as git archive leaves out the files it marks.
var excludedDirs = []string{"tools", "consumer", "skills"}

// imageExtensions are the extensions of image files, in lower case.
var imageExtensions = []string{
	".avif", ".bmp", ".gif", ".heic", ".ico", ".jpeg", ".jpg", ".png", ".svg", ".tif", ".tiff",
	".webp",
}

// limits are the sizes that the zip must keep to.
type limits struct {
	// fileCap is the most bytes a file may have, uncompressed, unless exceptions names it.
	fileCap int64
	// exceptions are the files that may have more than fileCap bytes, each with its own cap.
	exceptions []exception
	// budget is the most bytes the zip may have, compressed.
	budget budget
}

// exception lets one file of the zip be larger than the per-file cap.
type exception struct {
	// path is the file's path in the module, with slashes, such as testdata/api-surface.txt.
	path string
	// maxBytes is the most bytes the file may have, uncompressed.
	maxBytes int64
	// reason says why the file needs more than the per-file cap.
	reason string
}

// budget is the most bytes the zip may have, compressed: its size on a given day plus headroom.
type budget struct {
	// baseline is the zip's compressed size in bytes, as zipcheck printed it on the day measured.
	baseline int64
	// measured is that day, YYYY-MM-DD.
	measured string
	// headroom is the bytes the zip may grow by beyond baseline.
	headroom int64
	// reason says why the budget is what it is, and why it was raised when it was.
	reason string
}

// bytes returns the most bytes the zip may have, compressed.
func (b budget) bytes() int64 {
	return b.baseline + b.headroom
}

// zipFile is a file of the module zip.
type zipFile struct {
	// name is the file's path in the module, without the zip's module@version/ prefix.
	name string
	// size is the file's size in bytes, uncompressed.
	size int64
	// compressed is its size in bytes in the zip, as the zip records it.
	compressed uint64
	// text reports whether the file is valid UTF-8 without a NUL byte.
	text bool
}

// archiveFile is a file of git archive's output, as golang.org/x/mod/zip reads it.
type archiveFile struct {
	file *zip.File
}

// Path returns the file's path in the commit.
func (a archiveFile) Path() string { return a.file.Name }

// Lstat returns the file's mode and size as git archive records them.
func (a archiveFile) Lstat() (fs.FileInfo, error) { return a.file.FileInfo(), nil }

// Open opens the file's content.
func (a archiveFile) Open() (io.ReadCloser, error) { return a.file.Open() }

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr, zipLimits))
}

// run parses the command line, builds the module zip of the repository's HEAD and checks it
// against lim.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, lim limits) int {
	flags := flag.NewFlagSet("zipcheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repo := flags.String("repo", ".", "a `directory` in the repository")
	if err := flags.Parse(args); err != nil {
		return abort(stdout, err)
	}
	if flags.NArg() > 0 {
		return abort(stdout, fmt.Errorf("unexpected arguments %q", flags.Args()))
	}
	if err := lim.validate(); err != nil {
		return abort(stdout, fmt.Errorf("the limits in limits.go are not valid: %w", err))
	}
	version, data, err := buildZip(ctx, *repo)
	if invalid, ok := errors.AsType[modzip.FileErrorList](err); ok {
		return reportInvalid(stdout, invalid)
	}
	if err != nil {
		return abort(stdout, err)
	}
	files, err := readZip(version, data)
	if err != nil {
		return abort(stdout, err)
	}
	return report(stdout, version, files, int64(len(data)), lim)
}

// abort prints why zipcheck stopped before it checked the zip, then the verdict, and returns
// exitDidNotRun.
func abort(w io.Writer, reason error) int {
	fmt.Fprintf(w, "zipcheck: DID NOT RUN: %v\n", reason)
	fmt.Fprintln(w, "zipcheck: DID NOT RUN")
	return exitDidNotRun
}

// reportInvalid prints a failure for each file that the module zip cannot hold, then the
// verdict, and returns exitFailed.
func reportInvalid(w io.Writer, invalid modzip.FileErrorList) int {
	for _, f := range invalid {
		fmt.Fprintf(w, "zipcheck: FAIL: %s: %v; a module zip cannot hold it, so the module proxy "+
			"cannot serve the commit: rename or remove it\n", f.Path, f.Err)
	}
	fmt.Fprintln(w, "zipcheck: FAIL")
	return exitFailed
}

// validate returns what is wrong with the limits: a size that is not positive, an exception
// without a reason or whose cap is not above the per-file cap, two exceptions for one file, or a
// budget without a reason or a valid day.
func (l limits) validate() error {
	var errs []error
	if l.fileCap <= 0 {
		errs = append(errs, fmt.Errorf("the per-file cap is %d bytes; it must be positive",
			l.fileCap))
	}
	b := l.budget
	if b.baseline <= 0 || b.headroom < 0 {
		errs = append(errs, fmt.Errorf("the budget's baseline is %d bytes and its headroom %d; "+
			"the baseline must be positive and the headroom not negative", b.baseline,
			b.headroom))
	}
	if _, err := time.Parse(time.DateOnly, b.measured); err != nil {
		errs = append(errs, fmt.Errorf("the budget was measured on %q, which is not a day "+
			"written as YYYY-MM-DD", b.measured))
	}
	if strings.TrimSpace(b.reason) == "" {
		errs = append(errs, errors.New("the budget has no reason"))
	}
	seen := map[string]bool{}
	for _, e := range l.exceptions {
		switch {
		case !fs.ValidPath(e.path) || e.path == ".":
			errs = append(errs, fmt.Errorf("the exception for %q does not name a file by its "+
				"path in the module, such as testdata/api-surface.txt", e.path))
		case seen[e.path]:
			errs = append(errs, fmt.Errorf("two exceptions name %s; keep one", e.path))
		case e.maxBytes <= l.fileCap:
			errs = append(errs, fmt.Errorf("the exception for %s allows %d bytes, which is not "+
				"above the per-file cap of %d; remove it", e.path, e.maxBytes, l.fileCap))
		case strings.TrimSpace(e.reason) == "":
			errs = append(errs, fmt.Errorf("the exception for %s has no reason", e.path))
		}
		seen[e.path] = true
	}
	return errors.Join(errs...)
}

// buildZip builds the module zip of the commit that HEAD names in the repository that holds the
// directory repo, from the repository's top level, as the Go module proxy builds it: git archive
// lists the committed files and their content, and golang.org/x/mod/zip leaves out what a module
// zip never holds. It returns the zip's module and version, and the zip. zip.CreateFromVCS does
// the same, but it does not accept a git worktree.
func buildZip(ctx context.Context, repo string) (module.Version, []byte, error) {
	out, err := git(ctx, repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return module.Version{}, nil, fmt.Errorf("git rev-parse --show-toplevel in %s: %w", repo,
			err)
	}
	top := strings.TrimSuffix(string(out), "\n")
	out, err = git(ctx, top, "-c", "core.autocrlf=input", "-c", "core.eol=lf", "archive",
		"--format=zip", "HEAD")
	if err != nil {
		return module.Version{}, nil, fmt.Errorf("git archive HEAD in %s: %w", top, err)
	}
	archive, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		return module.Version{}, nil, fmt.Errorf("reading the output of git archive: %w", err)
	}
	var files []modzip.File
	var goMod *zip.File
	for _, f := range archive.File {
		if strings.HasSuffix(f.Name, "/") {
			continue
		}
		files = append(files, archiveFile{f})
		if f.Name == "go.mod" {
			goMod = f
		}
	}
	if goMod == nil {
		return module.Version{}, nil, errors.New("the commit has no go.mod at its root, so it " +
			"is not the root of a module")
	}
	content, err := readFile(goMod)
	if err != nil {
		return module.Version{}, nil, err
	}
	modulePath := modfile.ModulePath(content)
	if modulePath == "" {
		return module.Version{}, nil, errors.New("the commit's go.mod names no module path")
	}
	version := module.Version{Path: modulePath, Version: zipVersion(modulePath)}
	var zipped bytes.Buffer
	if err := modzip.Create(&zipped, version, files); err != nil {
		return module.Version{}, nil, fmt.Errorf("building the module zip of %s: %w", modulePath,
			err)
	}
	return version, zipped.Bytes(), nil
}

// git runs git with args in dir and returns what it printed on stdout. It runs without the
// user's and the system's configuration and without the user's attributes file (by default
// $XDG_CONFIG_HOME/git/attributes), which the module proxy does not have.
func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	args = append([]string{"-c", "core.attributesFile=" + os.DevNull}, args...)
	//nolint:gosec // G204: buildZip passes fixed arguments; only the directory varies.
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// zipVersion returns the version zipcheck gives the zip: v<N>.0.0 for a module path whose major
// version suffix is N, such as /v2, and v0.0.0 for a path without one. Only the names of the
// zip's files hold it.
func zipVersion(modulePath string) string {
	_, pathMajor, _ := module.SplitPathVersion(modulePath)
	if major := module.PathMajorPrefix(pathMajor); major != "" {
		return major + ".0.0"
	}
	return "v0.0.0"
}

// readFile returns the content of a file of a zip.
func readFile(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("opening %s in the zip: %w", f.Name, err)
	}
	defer r.Close()
	content, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading %s in the zip: %w", f.Name, err)
	}
	return content, nil
}

// readZip returns the files of the module zip data, sorted by name.
func readZip(version module.Version, data []byte) ([]zipFile, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("reading the module zip: %w", err)
	}
	prefix := version.Path + "@" + version.Version + "/"
	files := make([]zipFile, 0, len(r.File))
	for _, f := range r.File {
		name, ok := strings.CutPrefix(f.Name, prefix)
		if !ok {
			return nil, fmt.Errorf("the module zip holds %s, which is outside %s", f.Name, prefix)
		}
		content, err := readFile(f)
		if err != nil {
			return nil, err
		}
		files = append(files, zipFile{
			name:       name,
			size:       int64(len(content)),
			compressed: f.CompressedSize64,
			text:       utf8.Valid(content) && !bytes.Contains(content, []byte{0}),
		})
	}
	slices.SortFunc(files, func(a, b zipFile) int { return strings.Compare(a.name, b.name) })
	return files, nil
}

// report prints what the zip holds and every rule it breaks, then the verdict, and returns the
// exit code.
func report(w io.Writer, version module.Version, files []zipFile, zipSize int64,
	lim limits,
) int {
	if len(files) == 0 {
		return abort(w, fmt.Errorf("the zip of %s holds no file, so there is nothing to check",
			version))
	}
	fmt.Fprintf(w, "zipcheck: %s: %d files, %d bytes compressed, of a budget of %d bytes\n",
		version, len(files), zipSize, lim.budget.bytes())
	printLargest(w, files)
	problems := check(files, zipSize, lim)
	for _, problem := range problems {
		fmt.Fprintf(w, "zipcheck: FAIL: %s\n", problem)
	}
	if len(problems) > 0 {
		fmt.Fprintln(w, "zipcheck: FAIL")
		return exitFailed
	}
	fmt.Fprintln(w, "zipcheck: PASS")
	return exitPassed
}

// printLargest prints the largest files, at most largestShown of them, largest first.
func printLargest(w io.Writer, files []zipFile) {
	largest := slices.Clone(files)
	slices.SortStableFunc(largest, func(a, b zipFile) int { return cmp.Compare(b.size, a.size) })
	largest = largest[:min(len(largest), largestShown)]
	fmt.Fprintf(w, "zipcheck: the %d largest files, in bytes uncompressed and compressed:\n",
		len(largest))
	for _, f := range largest {
		fmt.Fprintf(w, "zipcheck: %10d %10d  %s\n", f.size, f.compressed, f.name)
	}
}

// check returns a problem for each rule that the zip's files, or its compressed size, break.
func check(files []zipFile, zipSize int64, lim limits) []string {
	var problems []string
	exceptions := map[string]exception{}
	for _, e := range lim.exceptions {
		exceptions[e.path] = e
	}
	inZip := map[string]bool{}
	for _, f := range files {
		inZip[f.name] = true
		if why := disallowed(f.name); why != "" {
			problems = append(problems, f.name+": "+why)
		}
		if !f.text && !inTestdata(f.name) {
			problems = append(problems, f.name+": not text, as it holds a NUL byte or is not "+
				"valid UTF-8; only files in testdata directories may be binary")
		}
		if e, ok := exceptions[f.name]; ok {
			if f.size > e.maxBytes {
				problems = append(problems, fmt.Sprintf("%s: %d bytes, over the cap of %d "+
					"bytes that its exception in limits.go allows; make it smaller, or raise "+
					"the cap with the reason", f.name, f.size, e.maxBytes))
			}
		} else if f.size > lim.fileCap {
			problems = append(problems, fmt.Sprintf("%s: %d bytes, over the per-file cap of %d "+
				"bytes; make it smaller, or add an exception for it to limits.go with its own "+
				"cap and the reason", f.name, f.size, lim.fileCap))
		}
	}
	if !inZip["LICENSE"] {
		problems = append(problems, "LICENSE: not in the zip; pkg.go.dev shows the "+
			"documentation of a module only when it detects a license it allows")
	}
	for _, e := range lim.exceptions {
		if !inZip[e.path] {
			problems = append(problems, e.path+": not in the zip, but limits.go has an "+
				"exception for it; remove the exception")
		}
	}
	if b := lim.budget; zipSize > b.bytes() {
		problems = append(problems, fmt.Sprintf("the zip has %d bytes compressed, over the "+
			"budget of %d: %d measured on %s plus %d of headroom; make it smaller, or raise the "+
			"budget in limits.go with the reason", zipSize, b.bytes(), b.baseline, b.measured,
			b.headroom))
	}
	return problems
}

// disallowed returns why a file with this path must not be in the zip, or "" when it may be.
func disallowed(name string) string {
	top, _, nested := strings.Cut(name, "/")
	base := strings.ToLower(path.Base(name))
	switch {
	case nested && slices.Contains(excludedDirs, top):
		return fmt.Sprintf("%s/ must stay out of the module zip; keep it out with a go.mod of "+
			"its own, or export-ignore in .gitattributes", top)
	case slices.Contains(imageExtensions, path.Ext(base)):
		return "an image; images stay out of the module zip, so link to them from the docs"
	case isSpec(name):
		return "an API specification; specifications stay out of the module zip, in tools/"
	case allowed(name):
		return ""
	}
	return "not on the allow list of the module zip; CONTRIBUTING.md says what the zip may hold"
}

// isSpec reports whether the file with this path is an API specification: a file other than a Go
// source file whose name holds openapi or swagger, or a file in a specs directory.
func isSpec(name string) bool {
	base := strings.ToLower(path.Base(name))
	if strings.Contains("/"+path.Dir(name)+"/", "/specs/") {
		return true
	}
	return path.Ext(base) != ".go" &&
		(strings.Contains(base, "openapi") || strings.Contains(base, "swagger"))
}

// allowed reports whether the allow list takes the file with this path.
func allowed(name string) bool {
	dir, base := path.Split(name)
	switch {
	case path.Ext(name) == ".go", inTestdata(name), strings.HasPrefix(name, ".github/"):
		return true
	case dir == "":
		return slices.Contains(rootFiles, base) || strings.HasPrefix(base, ".")
	}
	return false
}

// inTestdata reports whether the file with this path is in a testdata directory.
func inTestdata(name string) bool {
	return strings.Contains("/"+path.Dir(name)+"/", "/testdata/")
}
