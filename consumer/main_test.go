// Package consumer builds against the SDK the way an application does: it is a module of its own
// whose path is outside the SDK's, and go.mod replaces the SDK with the repository root. The
// tests check what an application gets from the SDK: its module graph, the documentation that
// go doc finds, and code that must not compile. golangci-all.sh lints this module with the
// repository's .golangci.yml, whose staticcheck reports a use of a deprecated identifier
// (SA1019).
package consumer

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// The SDK's packages are imported above and used here, so this module compiles against them.
var (
	_ = permit.Version
	_ = models.Some[string]
)

// sdkPackages are the SDK's packages that an application imports.
var sdkPackages = []string{
	"github.com/permitio/permit-golang/v2",
	"github.com/permitio/permit-golang/v2/models",
}

// wantModules is the module graph of an application that requires only the SDK, sorted: the
// application, the SDK and the SDK's own requirements. go-cmp is a requirement of the SDK's
// tests only, and module graph pruning still lists it. Every requirement of the SDK's go.mod,
// a tool or a test helper too, reaches the graph of every application, so add one here only on
// purpose; tools belong in tools/go.mod.
var wantModules = []string{
	"example.com/consumer",
	"github.com/google/go-cmp",
	"github.com/permitio/permit-golang/v2",
}

// wantDeprecated is the number of deprecation notices in the documentation of sdkPackages, as
// countDeprecated counts them; v2.0.0 has none. Change it only together with a deprecation, which
// staticcheck's SA1019 then reports in code that uses the identifier.
const wantDeprecated = 0

// wantPrefix starts the first line of a fixture in mustfail/; the rest of the line is a part of
// the message that compiling the fixture must fail with.
const wantPrefix = "// want: "

// goCommand runs the go command in dir with env added to the test's environment, and returns
// what it printed and its error. go test puts the go command of the toolchain that runs the
// test first in PATH. GOWORK=off keeps a go.work file out of the build, as an application has
// none.
func goCommand(t *testing.T, dir string, env []string, args ...string) (stdout, stderr string,
	err error,
) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = dir
	cmd.Env = slices.Concat(os.Environ(), []string{"GOWORK=off"}, env)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err = cmd.Run()
	return out.String(), errOut.String(), err
}

// mustGo runs the go command in this module without changing its go.mod or go.sum, and returns
// its standard output; it fails the test when the command fails.
func mustGo(t *testing.T, args ...string) string {
	t.Helper()
	stdout, stderr, err := goCommand(t, ".", []string{"GOFLAGS=-mod=readonly"}, args...)
	if err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, stderr)
	}
	return stdout
}

func TestModuleGraph(t *testing.T) {
	got := strings.Fields(mustGo(t, "list", "-m", "-f", "{{.Path}}", "all"))
	slices.Sort(got)

	if !slices.Equal(got, wantModules) {
		t.Errorf("go list -m all lists the modules\n\t%s\nwant\n\t%s\nA requirement of the "+
			"SDK's go.mod reaches every application's module graph; keep tools and their "+
			"requirements in tools/go.mod", strings.Join(got, "\n\t"),
			strings.Join(wantModules, "\n\t"))
	}
}

func TestGoDoc(t *testing.T) {
	deprecated := 0
	for _, pkg := range sdkPackages {
		doc := mustGo(t, "doc", "-all", pkg)

		first, _, _ := strings.Cut(doc, "\n")
		if !strings.HasPrefix(first, "package ") || !strings.HasSuffix(first, ` // import "`+
			pkg+`"`) {
			t.Errorf("go doc -all %s starts with %q, want the package clause and its import "+
				"path", pkg, first)
		}
		deprecated += countDeprecated(doc)
	}

	if deprecated != wantDeprecated {
		t.Errorf("the documentation of the SDK's packages holds %d deprecation notices, want %d; "+
			"change wantDeprecated together with a deprecation", deprecated, wantDeprecated)
	}
}

// countDeprecated returns the number of deprecation notices in doc, the output of go doc -all:
// the paragraphs that start with "Deprecated: ". go doc prints the documentation of a function,
// type or method as indented text, and the comments on struct fields and on the specs of grouped
// const and var declarations as // lines. A line starts a paragraph when the line before it is
// blank or has another margin, as the first line of a comment does.
func countDeprecated(doc string) int {
	count := 0
	previousMargin, previousText := "", ""
	for line := range strings.Lines(doc) {
		margin, text := splitDocLine(line)
		if strings.HasPrefix(text, "Deprecated: ") &&
			(previousText == "" || margin != previousMargin) {
			count++
		}
		previousMargin, previousText = margin, text
	}
	return count
}

// splitDocLine splits a line of go doc's output into its margin, the indentation and the // that
// starts a comment, and its text.
func splitDocLine(line string) (margin, text string) {
	text = strings.TrimLeft(line, " \t")
	margin = line[:len(line)-len(text)]
	if rest, ok := strings.CutPrefix(text, "//"); ok {
		margin += "//"
		text = rest
	}
	return margin, strings.TrimSpace(text)
}

// deprecatedSource is a package with a deprecation notice in each place that go doc prints one,
// seven in all, and with two lines that start with "Deprecated: " in the middle of a paragraph.
const deprecatedSource = `// Package deprecated marks identifiers deprecated.
package deprecated

// Func is deprecated.
//
// Deprecated: a paragraph of its own.
func Func() {}

// Deprecated: the first line of the comment.
func First() {}

// NotDeprecated has a paragraph that go doc wraps after this line, at the end.
// Deprecated: is a word in this paragraph.
func NotDeprecated() {}

// Type has deprecated fields.
type Type struct {
	// Field is deprecated.
	//
	// Deprecated: a paragraph of its own.
	Field int
	// Deprecated: the first line of the comment.
	First int
	// Kept is not deprecated, though a line of its comment starts with the word:
	// Deprecated: is a word in this paragraph.
	Kept int
}

// Method is deprecated.
//
// Deprecated: a paragraph of its own.
func (Type) Method() {}

const (
	// A is deprecated.
	//
	// Deprecated: a paragraph of its own.
	A = 1
	// B is not deprecated.
	B = 2
)

var (
	// Deprecated: the first line of the comment.
	V = 1
	W = 2
)
`

// TestCountDeprecated runs go doc -all on a module with deprecatedSource and checks that
// countDeprecated counts each deprecated identifier once, and no other line.
func TestCountDeprecated(t *testing.T) {
	dir := t.TempDir()
	goMod := []byte("module example.com/deprecated\n\ngo 1.26.0\n")
	writeFile(t, filepath.Join(dir, "go.mod"), goMod)
	writeFile(t, filepath.Join(dir, "deprecated.go"), []byte(deprecatedSource))
	doc, stderr, err := goCommand(t, dir, nil, "doc", "-all", ".")
	if err != nil {
		t.Fatalf("go doc -all: %v\n%s", err, stderr)
	}

	if got := countDeprecated(doc); got != 7 {
		t.Errorf("countDeprecated() = %d, want 7, of this output of go doc -all:\n%s", got, doc)
	}
}

// TestMustFail compiles each fixture in mustfail/ as the main package of a module of its own,
// with the go.mod and go.sum of this module, so that its path is outside the SDK's, as an
// application's is. Each fixture is code that an application must not be able to write, and must
// fail to compile with the message its first line names. Every file in mustfail/ must be a
// fixture, and as many fixtures must compile as there are files.
func TestMustFail(t *testing.T) {
	entries, err := os.ReadDir("mustfail")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("mustfail/ holds no fixture")
	}
	goMod, goSum := readModule(t)
	compiled := 0
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() || !strings.HasSuffix(name, ".go.txt") {
			t.Errorf("mustfail/%s is not a fixture; every file in mustfail/ is a <name>.go.txt "+
				"fixture", name)
			continue
		}
		t.Run(strings.TrimSuffix(name, ".go.txt"), func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("mustfail", name))
			if err != nil {
				t.Fatal(err)
			}
			first, _, _ := strings.Cut(string(source), "\n")
			want, ok := strings.CutPrefix(first, wantPrefix)
			if !ok || strings.TrimSpace(want) == "" {
				t.Fatalf("mustfail/%s starts with %q; its first line must be %q followed by "+
					"a part of the message it fails to compile with", name, first, wantPrefix)
			}
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "go.mod"), goMod)
			writeFile(t, filepath.Join(dir, "go.sum"), goSum)
			writeFile(t, filepath.Join(dir, "main.go"), source)

			_, stderr, err := goCommand(t, dir, []string{"GOFLAGS=-mod=mod"},
				"build", "-o", os.DevNull, ".")
			compiled++

			switch {
			case err == nil:
				t.Errorf("mustfail/%s compiles; it must fail with %q", name, want)
			case !strings.Contains(stderr, want):
				t.Errorf("mustfail/%s fails without %q in its message:\n%s", name, want, stderr)
			}
		})
	}

	if compiled != len(entries) {
		t.Errorf("compiled %d fixtures of the %d files in mustfail/", compiled, len(entries))
	}
}

// readModule returns this module's go.mod, with the replace directive pointing at the SDK's
// absolute path, quoted, so that it works in another directory, and its go.sum.
func readModule(t *testing.T) (goMod, goSum []byte) {
	t.Helper()
	sdk, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	goMod, err = os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	const replace = "=> ../\n"
	if n := bytes.Count(goMod, []byte(replace)); n != 1 {
		t.Fatalf("go.mod has %d replace targets %q, want 1", n, replace)
	}
	goMod = bytes.Replace(goMod, []byte(replace), []byte("=> "+strconv.Quote(sdk)+"\n"), 1)
	goSum, err = os.ReadFile("go.sum")
	if err != nil {
		t.Fatal(err)
	}
	return goMod, goSum
}

func writeFile(t *testing.T, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(name, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
