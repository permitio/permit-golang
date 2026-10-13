//go:build unix

package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// copyModule copies the directories, which it can always write to, and the regular
// files with their modes, and leaves out symbolic links.
func TestCopyModule(t *testing.T) {
	t.Parallel()
	src := filepath.Join(t.TempDir(), "module")
	writeRepository(t, src, []string{"go.mod", "a/b.go", "a/testdata/c.txt"})
	if err := os.Chmod(filepath.Join(src, "a", "b.go"), 0o755); err != nil {
		t.Fatal(err)
	}
	readOnly := filepath.Join(src, "a", "testdata")
	if err := os.Chmod(readOnly, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) })
	if err := os.Symlink("a/b.go", filepath.Join(src, "link.go")); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "copy")
	if err := copyModule(src, dst); err != nil {
		t.Fatal(err)
	}
	var got []string
	err := filepath.WalkDir(dst, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dst, p)
		if !d.IsDir() {
			data, readErr := os.ReadFile(p)
			info, statErr := d.Info()
			if err = errors.Join(err, readErr, statErr); err == nil {
				rel += " " + info.Mode().Perm().String()
				if string(data) != fiveLines {
					rel += " with other content"
				}
			}
		}
		got = append(got, filepath.ToSlash(rel))
		return err
	})
	want := ". a a/b.go -rwxr-xr-x a/testdata a/testdata/c.txt -rw------- go.mod -rw-------"
	if err != nil || strings.Join(got, " ") != want {
		t.Errorf("the copy holds %q, %v; want %q", got, err, want)
	}
	if err := copyModule(filepath.Join(src, "missing"), t.TempDir()); err == nil {
		t.Error("no error for a module that does not exist")
	}
}

// checkCopy runs a package's tests in a copy of its module and fails when they
// fail there, as one that reads a file above the module root does.
func TestCheckCopy(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	root := filepath.Join(parent, "module")
	files := map[string]string{
		"outside.txt":     "above the module\n",
		"module/go.mod":   "module example.com/m\n\ngo 1.21\n",
		"module/in/in.go": "package in\n\n// F returns 1.\nfunc F() int { return 1 }\n",
		"module/in/in_test.go": "package in\n\nimport \"testing\"\n\n" +
			"func TestF(t *testing.T) {\n\tif F() != 1 {\n\t\tt.Fatal(F())\n\t}\n}\n",
		"module/out/out.go": "package out\n\n// F returns 1.\nfunc F() int { return 1 }\n",
		"module/out/out_test.go": "package out\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\n" +
			"func TestRepositoryFile(t *testing.T) {\n" +
			"\tif _, err := os.ReadFile(\"../../outside.txt\"); err != nil {\n" +
			"\t\tt.Fatal(err)\n\t}\n}\n",
		"module/tagged/tagged.go": "//go:build e2e\n\npackage tagged\n\n" +
			"// F returns 1.\nfunc F() int { return 1 }\n",
		"module/tagged/tagged_test.go": "package tagged\n\nimport \"testing\"\n\n" +
			"func TestF(t *testing.T) {\n\tif F() != 1 {\n\t\tt.Fatal(F())\n\t}\n}\n",
	}
	for name, content := range files {
		p := filepath.Join(parent, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	goPath := lookPath(t, "go")
	r := runner{dir: t.TempDir(), tools: tools{goTest: func(dir string, args, env []string) (
		string, error,
	) {
		return runGoTest(goPath, dir, args, env)
	}}}
	env := append(os.Environ(), "GOFLAGS=-count=1", "GOTOOLCHAIN=local", "GOWORK=off",
		"GOPROXY=off")
	m := goModule{path: "example.com/m", dir: root}
	in := goPackage{dir: filepath.Join(root, "in"), importPath: "example.com/m/in"}
	out := goPackage{dir: filepath.Join(root, "out"), importPath: "example.com/m/out"}
	if err := r.checkCopy(m, in, env); err != nil {
		t.Errorf("tests that pass in a copy: %v", err)
	}
	err := r.checkCopy(m, out, env)
	want := "the tests of example.com/m/out fail without a mutant in a copy of its module"
	if err == nil || !strings.Contains(err.Error(), want) ||
		!strings.Contains(err.Error(), "--- FAIL: TestRepositoryFile") {
		t.Errorf("a test that reads a file above the module: got %v", err)
	}
	if len(r.copies) != 1 {
		t.Errorf("the module was copied %d times", len(r.copies))
	}
	env = append(env, "GOFLAGS=-count=1 -skip=^TestRepository")
	if err := r.checkCopy(m, out, env); err != nil {
		t.Errorf("with the test skipped: %v", err)
	}
	// The tests of a package that builds only with -tags run with the tags.
	tagged := goPackage{dir: filepath.Join(root, "tagged"), importPath: "example.com/m/tagged"}
	r.opts.tags = "e2e"
	if err := r.checkCopy(m, tagged, env); err != nil {
		t.Errorf("with -tags e2e: %v", err)
	}
}
