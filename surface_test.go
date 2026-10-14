package permit_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/surface"
)

// update makes the tests that compare with a file in testdata rewrite the file instead.
var update = flag.Bool("update", false, "rewrite the testdata files that the tests compare with")

// surfaceDirs are the directories of the packages whose exported API is the SDK's public surface,
// the root package first.
var surfaceDirs = []string{".", "models"}

// surfaceFile is the snapshot of the public surface.
const surfaceFile = "testdata/api-surface.txt"

// surfaceHeader starts the snapshot.
const surfaceHeader = "" +
	"# The exported API of the permit and models packages, one declaration per line, sorted.\n" +
	"# TestSurfaceSnapshot (surface_test.go) fails when the source differs. After an intended\n" +
	"# change, rewrite this file with go test -run TestSurfaceSnapshot -update . and review the\n" +
	"# diff.\n"

// readSurface lists the exported API of the packages in surfaceDirs. It fails the test when the
// root package lists nothing: it exports Version at least, so an empty list means that the source
// was not read.
func readSurface(t *testing.T) []*surface.Package {
	t.Helper()
	var pkgs []*surface.Package
	for _, dir := range surfaceDirs {
		p, err := surface.Read(dir)
		if err != nil {
			t.Fatal(err)
		}
		pkgs = append(pkgs, p)
	}
	if len(pkgs[0].Items) == 0 {
		t.Fatal("the root package lists no exported declaration, but it exports Version at " +
			"least; the surface was not read")
	}
	return pkgs
}

// snapshot returns the snapshot of pkgs: the header, then the line of each item after its
// package name, sorted, each with a newline.
func snapshot(pkgs []*surface.Package) string {
	var lines []string
	for _, p := range pkgs {
		for _, item := range p.Items {
			lines = append(lines, p.Name+": "+item.Line+"\n")
		}
	}
	slices.Sort(lines)
	return surfaceHeader + strings.Join(lines, "")
}

// snapshotDiff returns "" when got is want, and otherwise their difference line by line
// (-want +got), each line with its newline, so that a missing final newline or a carriage
// return shows too.
func snapshotDiff(want, got string) string {
	if want == got {
		return ""
	}
	return cmp.Diff(strings.SplitAfter(want, "\n"), strings.SplitAfter(got, "\n"))
}

func TestSurfaceSnapshot(t *testing.T) {
	got := snapshot(readSurface(t))
	if *update {
		if err := os.WriteFile(surfaceFile, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", surfaceFile)
	}
	want, err := os.ReadFile(surfaceFile)
	if err != nil {
		t.Fatalf("%v; write the snapshot with go test -run TestSurfaceSnapshot -update .", err)
	}
	if diff := snapshotDiff(string(want), got); diff != "" {
		t.Errorf("the public surface differs from %s (-file +source):\n%s\nWhen the change is "+
			"intended, rewrite the file with go test -run TestSurfaceSnapshot -update . and "+
			"review its diff.", surfaceFile, diff)
	}
}

func TestSnapshot(t *testing.T) {
	got := snapshot([]*surface.Package{
		{Name: "permit", Items: []surface.Item{
			{Line: "func New()"},
			{Line: `const Version = "2.0.0"`},
		}},
		{Name: "models", Items: []surface.Item{{Line: "type User struct"}}},
	})
	want := surfaceHeader + "models: type User struct\n" +
		"permit: const Version = \"2.0.0\"\n" +
		"permit: func New()\n"
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("snapshot differs (-want +got):\n%s", diff)
	}
}

func TestSnapshotDiff(t *testing.T) {
	const (
		header  = "# header\n"
		version = "permit: const Version = \"2.0.0\"\n"
		newFunc = "permit: func New()\n"
		source  = header + version + newFunc
	)
	if diff := snapshotDiff(source, source); diff != "" {
		t.Errorf("snapshotDiff of equal texts is %q, want none", diff)
	}
	for name, file := range map[string]string{
		"a deleted line":        header + newFunc,
		"no final newline":      strings.TrimSuffix(source, "\n"),
		"a carriage return":     strings.ReplaceAll(source, "\n", "\r\n"),
		"an added line":         source + "permit: func Old()\n",
		"a line out of order":   header + newFunc + version,
		"a changed declaration": strings.Replace(source, "New()", "New(apiKey string)", 1),
	} {
		if diff := snapshotDiff(file, source); diff == "" {
			t.Errorf("snapshotDiff found no difference for %s", name)
		}
	}
	// go-cmp varies the spaces of its output, so look for a line that starts with + and holds the
	// deleted line.
	diff := snapshotDiff(header+newFunc, source)
	marked := slices.ContainsFunc(strings.Split(diff, "\n"), func(line string) bool {
		return strings.HasPrefix(line, "+") &&
			strings.Contains(line, `"permit: const Version = \"2.0.0\"\n"`)
	})
	if !marked {
		t.Errorf("snapshotDiff of a deleted line is\n%s\nwant the line marked +", diff)
	}
}

func TestNoInternalTypesInSignatures(t *testing.T) {
	for _, problem := range internalUses(readSurface(t)) {
		t.Error(problem)
	}
}

// internalUses returns a problem for each exported declaration in pkgs that mentions a package
// in an internal directory, which applications cannot import.
func internalUses(pkgs []*surface.Package) []string {
	var problems []string
	for _, p := range pkgs {
		for _, item := range p.Items {
			for _, path := range item.Uses {
				if isInternal(path) {
					problems = append(problems, fmt.Sprintf("%s: %s mentions %s, an internal "+
						"package that applications cannot import; use an exported type", p.Name,
						item.Line, path))
				}
			}
		}
	}
	return problems
}

// isInternal reports whether the import path is in an internal directory.
func isInternal(path string) bool {
	return slices.Contains(strings.Split(path, "/"), "internal")
}

func TestInternalUses(t *testing.T) {
	dir := t.TempDir()
	src := `package sdk

import (
	"net/http"

	"example.com/sdk/internal/rest"
	"example.com/sdk/models"
)

type Client struct{ Op rest.Op }

func Leak() *rest.Op { return nil }

func Fine(c *http.Client) models.User { return models.User{} }

func hidden() rest.Op { return rest.Op{} }
`
	if err := os.WriteFile(filepath.Join(dir, "sdk.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := surface.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"sdk: func Leak() *rest.Op mentions example.com/sdk/internal/rest, an internal package " +
			"that applications cannot import; use an exported type",
		"sdk: type Client struct, Op rest.Op mentions example.com/sdk/internal/rest, an internal " +
			"package that applications cannot import; use an exported type",
	}
	if diff := cmp.Diff(want, internalUses([]*surface.Package{p})); diff != "" {
		t.Errorf("internalUses differs (-want +got):\n%s", diff)
	}
}

func TestIsInternal(t *testing.T) {
	for path, want := range map[string]bool{
		"internal":                     true,
		"internal/rest":                true,
		"example.com/sdk/internal":     true,
		"example.com/sdk/internal/x/y": true,
		"example.com/sdk/internals/x":  false,
		"example.com/sdk/myinternal":   false,
		"net/http":                     false,
	} {
		if got := isInternal(path); got != want {
			t.Errorf("isInternal(%q) = %v, want %v", path, got, want)
		}
	}
}
