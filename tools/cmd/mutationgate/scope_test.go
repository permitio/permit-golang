package main

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fixtureFiles is a repository with two modules the tests give, m at the root and
// m/tools, and one they do not, m/extra. Each Go file has five lines.
var fixtureFiles = []string{
	"go.mod", "a.go", "a_test.go", "other_windows.go", "testdata/fix.go", "cmd/tool/main.go",
	"tools/go.mod", "tools/x/x.go", "extra/go.mod", "extra/e.go", "README.md",
}

// fixturePackages are the packages go list reports, by module root.
var fixturePackages = map[string][]goPackage{
	".": {
		{importPath: "m", files: []string{"a.go"}},
		{importPath: "m/cmd/tool", files: []string{"main.go"}},
	},
	"tools": {{importPath: "m/tools/x", files: []string{"x.go"}}},
	"extra": {{importPath: "m/extra", files: []string{"e.go"}}},
}

const fiveLines = "package x\n\nfunc f() int {\n\treturn 1 + 2\n}\n"

// writeRepository writes the files under root, Go files with five lines.
func writeRepository(t *testing.T, root string, files []string) {
	t.Helper()
	for _, name := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(fiveLines), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// fixtureRepository writes the fixture files to a new repository root and returns
// it, with tools whose module lists fixturePackages.
func fixtureRepository(t *testing.T, diff string) (string, tools) {
	t.Helper()
	root := t.TempDir()
	writeRepository(t, root, fixtureFiles)
	return root, fixtureTools(root, diff)
}

func fixtureTools(root, diff string) tools {
	return tools{
		diff: func(string, string) ([]byte, error) { return []byte(diff), nil },
		module: func(dir, _ string) (goModule, error) {
			rel, err := filepath.Rel(root, dir)
			if err != nil {
				return goModule{}, err
			}
			rel = filepath.ToSlash(rel)
			m := goModule{path: "m", dir: dir}
			if rel != "." {
				m.path = "m/" + rel
			}
			for _, pkg := range fixturePackages[rel] {
				pkgRel := strings.TrimPrefix(strings.TrimPrefix(pkg.importPath, "m"), "/")
				pkg.dir = filepath.Join(root, filepath.FromSlash(pkgRel))
				m.pkgs = append(m.pkgs, pkg)
			}
			return m, nil
		},
	}
}

// changedDiff returns git diff -U0 output that changes lines of each file.
func changedDiff(lines map[string]string) string {
	var b strings.Builder
	for _, file := range slices.Sorted(maps.Keys(lines)) {
		b.WriteString("--- a/" + file + "\n+++ b/" + file + "\n" + lines[file])
	}
	return b.String()
}

func unitSummary(p plan) []string {
	var out []string
	for _, u := range p.units {
		files := slices.Sorted(maps.Keys(u.lines))
		out = append(out, u.pkg.importPath+" "+strings.Join(files, ","))
	}
	return out
}

func TestPlanRunChangedLines(t *testing.T) {
	diff := changedDiff(map[string]string{
		"a.go":             "@@ -2 +2,2 @@\n-x\n+y\n+z\n",
		"a_test.go":        "@@ -1 +1 @@\n-x\n+y\n",
		"other_windows.go": "@@ -1 +1 @@\n-x\n+y\n",
		"testdata/fix.go":  "@@ -1 +1 @@\n-x\n+y\n",
		"cmd/tool/main.go": "@@ -4 +4 @@\n-x\n+y\n",
		"tools/x/x.go":     "@@ -0,0 +1,5 @@\n+a\n+b\n+c\n+d\n+e\n",
		"README.md":        "@@ -1 +1 @@\n-x\n+y\n",
	})
	root, tl := fixtureRepository(t, diff)
	t.Chdir(root)
	p, err := planRun(tl, gateOptions{base: "main", dirs: []string{".", "tools"}}, root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"m a.go", "m/cmd/tool main.go", "m/tools/x x.go"}
	if got := unitSummary(p); !slices.Equal(got, want) {
		t.Errorf("units %q, want %q", got, want)
	}
	if got := p.units[0].lines["a.go"]; !slices.Equal(got, []lineRange{{2, 3}}) {
		t.Errorf("a.go lines %v", got)
	}
	if !slices.Equal(p.units[1].exclude, []string{"/"}) ||
		!slices.Equal(p.units[0].exclude, []string{"/", `^other_windows\.go$`}) {
		t.Errorf("excluded %q and %q", p.units[0].exclude, p.units[1].exclude)
	}
	root0 := p.modules[0]
	wantNot := []notMutated{
		{"other_windows.go", "go does not build it here (another platform, a build tag or an " +
			"ignored name)"},
		{"testdata/fix.go", "test data"},
	}
	if root0.rel != "." || root0.changedFiles != 4 || !slices.Equal(root0.notMutated, wantNot) {
		t.Errorf("root module %q: %d changed files, not mutated %q", root0.rel,
			root0.changedFiles, root0.notMutated)
	}
	if tools0 := p.modules[1]; tools0.rel != "tools" || tools0.changedFiles != 1 ||
		tools0.module.path != "m/tools" {
		t.Errorf("tools module %+v", tools0)
	}
}

// A directory inside a module mutates the packages at or under it only.
func TestPlanRunInADirectory(t *testing.T) {
	diff := changedDiff(map[string]string{
		"a.go": "@@ -2 +2 @@\n-x\n+y\n", "cmd/tool/main.go": "@@ -4 +4 @@\n-x\n+y\n",
	})
	root, tl := fixtureRepository(t, diff)
	t.Chdir(filepath.Join(root, "cmd"))
	p, err := planRun(tl, gateOptions{base: "main", dirs: []string{"tool"}}, root)
	if err != nil {
		t.Fatal(err)
	}
	if got := unitSummary(p); !slices.Equal(got, []string{"m/cmd/tool main.go"}) {
		t.Errorf("units %q", got)
	}
	want := []notMutated{{"a.go", "outside the directories given"}}
	if got := p.modules[0].notMutated; !slices.Equal(got, want) || p.modules[0].rel != "." ||
		!slices.Equal(p.modules[0].dirs, []string{"cmd/tool"}) {
		t.Errorf("module %+v", p.modules[0])
	}
}

func TestPlanRunAll(t *testing.T) {
	root, tl := fixtureRepository(t, "")
	t.Chdir(root)
	p, err := planRun(tl, gateOptions{all: true, dirs: []string{"."}}, root)
	if err != nil {
		t.Fatal(err)
	}
	if got := unitSummary(p); !slices.Equal(got, []string{"m a.go", "m/cmd/tool main.go"}) {
		t.Errorf("units %q", got)
	}
	if got := p.units[1].lines["main.go"]; !slices.Equal(got, []lineRange{{1, 5}}) {
		t.Errorf("main.go lines %v", got)
	}
	if p.modules[0].changedFiles != 2 || len(p.modules) != 1 {
		t.Errorf("modules %+v", p.modules)
	}

	// An empty Go file has no line to mutate.
	if err = os.WriteFile(filepath.Join(root, "cmd", "tool", "main.go"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	p, err = planRun(tl, gateOptions{all: true, dirs: []string{"."}}, root)
	if got := unitSummary(p); err != nil || !slices.Equal(got, []string{"m a.go"}) ||
		p.modules[0].changedFiles != 2 {
		t.Errorf("with an empty file: units %q, %v", got, err)
	}
}

// Directories of one module make one module, which lists each directory once.
func TestPlanRunDirectoriesOfOneModule(t *testing.T) {
	root, tl := fixtureRepository(t, "")
	t.Chdir(root)
	p, err := planRun(tl, gateOptions{all: true, dirs: []string{"cmd/tool", ".", "cmd/tool"}},
		root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.modules) != 1 || !slices.Equal(p.modules[0].dirs, []string{"cmd/tool", "."}) {
		t.Errorf("modules %+v", p.modules)
	}
	if got := unitSummary(p); !slices.Equal(got, []string{"m a.go", "m/cmd/tool main.go"}) {
		t.Errorf("units %q", got)
	}
}

// Only a regular go.mod outside test data makes a module root; files below another
// one belong to the module above it.
func TestPlanRunModuleRoot(t *testing.T) {
	t.Run("a go.mod in test data", func(t *testing.T) {
		root, tl := fixtureRepository(t, changedDiff(map[string]string{
			"testdata/mod/fix.go": "@@ -1 +1 @@\n-a\n+b\n",
		}))
		writeRepository(t, root, []string{"testdata/mod/go.mod", "testdata/mod/fix.go"})
		t.Chdir(root)
		p, err := planRun(tl, gateOptions{base: "main", dirs: []string{"."}}, root)
		want := []notMutated{{"testdata/mod/fix.go", "test data"}}
		if err != nil || len(p.modules) != 1 || !slices.Equal(p.modules[0].notMutated, want) {
			t.Errorf("modules %+v, %v", p.modules, err)
		}
	})
	t.Run("a directory named go.mod", func(t *testing.T) {
		root, tl := fixtureRepository(t, "")
		if err := os.Mkdir(filepath.Join(root, "cmd", "go.mod"), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Chdir(root)
		p, err := planRun(tl, gateOptions{all: true, dirs: []string{"cmd/tool"}}, root)
		if err != nil || len(p.modules) != 1 || p.modules[0].rel != "." {
			t.Errorf("modules %+v, %v", p.modules, err)
		}
	})
}

func TestPlanRunNoChange(t *testing.T) {
	root, tl := fixtureRepository(t, changedDiff(map[string]string{
		"README.md": "@@ -1 +1 @@\n-x\n+y\n",
	}))
	t.Chdir(root)
	p, err := planRun(tl, gateOptions{base: "main", dirs: []string{".", "tools"}}, root)
	if err != nil || len(p.units) != 0 || len(p.modules) != 2 {
		t.Errorf("plan %+v, %v", p, err)
	}
}

func TestPlanRunErrors(t *testing.T) {
	tests := []struct {
		name string
		diff string
		dirs []string
		// edit changes the repository or the tools before the run.
		edit func(t *testing.T, root string, tl *tools)
		want string
	}{
		{
			name: "a change in a module not given",
			diff: changedDiff(map[string]string{
				"a.go": "@@ -2 +2 @@\n-x\n+y\n", "extra/e.go": "@@ -2 +2 @@\n-x\n+y\n",
			}),
			dirs: []string{".", "tools"},
			want: "go lines changed in extra/e.go, in no module of the directories given; give " +
				"the directory of its module too",
		},
		{
			name: "a change in a nested module when only the root is given",
			diff: changedDiff(map[string]string{"tools/x/x.go": "@@ -2 +2 @@\n-x\n+y\n"}),
			dirs: []string{"."}, want: "go lines changed in tools/x/x.go",
		},
		{
			name: "a directory that does not exist",
			dirs: []string{"nope"},
			want: "nope is not a directory",
		},
		{name: "a file", dirs: []string{"a.go"}, want: "a.go is not a directory"},
		{
			name: "a directory outside the repository", dirs: []string{".."},
			want: "is outside the repository",
		},
		{
			name: "a directory beside the repository", dirs: []string{"../sibling"},
			edit: func(t *testing.T, root string, _ *tools) {
				t.Helper()
				sibling := filepath.Join(filepath.Dir(root), "sibling")
				if err := os.Mkdir(sibling, 0o700); err != nil {
					t.Fatal(err)
				}
			},
			want: "is outside the repository",
		},
		{
			name: "a directory in no module", dirs: []string{"."},
			edit: func(t *testing.T, root string, _ *tools) {
				t.Helper()
				if err := os.Remove(filepath.Join(root, "go.mod")); err != nil {
					t.Fatal(err)
				}
			},
			want: "directory . is in no Go module of the repository",
		},
		{
			name: "go list fails", dirs: []string{"."},
			edit: func(_ *testing.T, _ string, tl *tools) {
				tl.module = func(string, string) (goModule, error) {
					return goModule{}, errors.New("go list: broken")
				}
			},
			want: "go list: broken",
		},
		{
			name: "go list reports another module root", dirs: []string{"tools"},
			edit: func(_ *testing.T, root string, tl *tools) {
				tl.module = func(string, string) (goModule, error) {
					return goModule{path: "m", dir: root}, nil
				}
			},
			want: "reports the module root",
		},
		{
			name: "git diff fails", dirs: []string{"."},
			edit: func(_ *testing.T, _ string, tl *tools) {
				tl.diff = func(string, string) ([]byte, error) {
					return nil, errors.New("git diff: unknown revision")
				}
			},
			want: "unknown revision",
		},
		{
			name: "a diff the gate cannot read", dirs: []string{"."},
			diff: "--- a/a.go\n+++ b/a.go\n@@ bad @@\n", want: "cannot read the hunk header",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, tl := fixtureRepository(t, tt.diff)
			if tt.edit != nil {
				tt.edit(t, root, &tl)
			}
			t.Chdir(root)
			_, err := planRun(tl, gateOptions{base: "main", dirs: tt.dirs}, root)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("got %v, want an error with %q", err, tt.want)
			}
		})
	}
}

func TestExcludedFiles(t *testing.T) {
	dir := t.TempDir()
	writeRepository(t, dir, []string{
		"a.go", "a_test.go", "b_windows.go", "c.go", "README.md", "sub/d.go", "e.go.txt",
	})
	got, err := excludedFiles(goPackage{dir: dir, files: []string{"a.go", "c.go"}})
	want := []string{"/", `^b_windows\.go$`}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("got %q, %v; want %q", got, err, want)
	}
	if _, err := excludedFiles(goPackage{dir: filepath.Join(dir, "missing")}); err == nil {
		t.Error("no error for a directory that does not exist")
	}
}

// The diff the git shim gives gremlins adds exactly the lines in scope, and reads
// back as them.
func TestScopeDiff(t *testing.T) {
	lines := map[string][]lineRange{"main.go": {{9, 9}, {3, 4}}, "a.go": {{1, 2}}}
	want := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -0,0 +1,2 @@\n+\n+\n" +
		"diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n" +
		"@@ -2,0 +3,2 @@\n+\n+\n@@ -6,0 +9,1 @@\n+\n"
	diff := scopeDiff(lines)
	if string(diff) != want {
		t.Errorf("got\n%s\nwant\n%s", diff, want)
	}
	got, err := parseAddedLines(diff)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got["a.go"], []lineRange{{1, 2}}) ||
		!slices.Equal(got["main.go"], []lineRange{{3, 4}, {9, 9}}) || len(got) != 2 {
		t.Errorf("reads back as %v", got)
	}
	if len(scopeDiff(nil)) != 0 {
		t.Error("no lines give a diff")
	}
}

func TestWithoutLines(t *testing.T) {
	lines := map[string][]lineRange{"a.go": {{1, 5}, {8, 8}}, "b.go": {{2, 3}}}
	got := withoutLines(lines, map[string][]int{"a.go": {1, 3, 8}, "b.go": {2, 3}})
	want := map[string][]lineRange{"a.go": {{2, 2}, {4, 5}}}
	if !maps.EqualFunc(got, want, func(a, b []lineRange) bool { return slices.Equal(a, b) }) {
		t.Errorf("got %v, want %v", got, want)
	}
	got = withoutLines(lines, nil)
	if !maps.EqualFunc(got, lines, func(a, b []lineRange) bool { return slices.Equal(a, b) }) {
		t.Errorf("dropping nothing: got %v", got)
	}
}

func TestIsChanged(t *testing.T) {
	lines := map[string][]lineRange{"a.go": {{2, 4}}}
	for line, want := range map[int]bool{1: false, 2: true, 4: true, 5: false} {
		if got := isChanged(lines, "a.go", line); got != want {
			t.Errorf("line %d: %v", line, got)
		}
	}
	if isChanged(lines, "b.go", 2) {
		t.Error("b.go:2 is changed")
	}
}

func TestLineCount(t *testing.T) {
	for text, want := range map[string]int{"": 0, "a": 1, "a\n": 1, "a\nb": 2, "\n\n": 2} {
		if got := lineCount([]byte(text)); got != want {
			t.Errorf("%q: %d lines, want %d", text, got, want)
		}
	}
}
