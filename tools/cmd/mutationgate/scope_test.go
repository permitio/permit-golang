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

// fixtureModFiles are the go.mod files of the fixture's modules. The tools module
// replaces the root module with ../, as the repository's tools module does.
var fixtureModFiles = map[string]string{
	"go.mod":       "module m\n\ngo 1.21\n",
	"tools/go.mod": "module m/tools\n\ngo 1.21\n\nrequire m v0.0.0\n\nreplace m => ../\n",
	"extra/go.mod": "module m/extra\n\ngo 1.21\n",
}

// fiveLines is a Go file each of whose five lines holds code in a function body,
// which the gate mutates when it changes.
const fiveLines = "package x; func f() int { n := 1\n\tn += 2\n\tn *= 3\n\tn -= 4\n\treturn n }\n"

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

// writeFile replaces the content of the file name, relative to root.
func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content),
		0o600); err != nil {
		t.Fatal(err)
	}
}

// fixtureRepository writes the fixture files to a new repository root and returns
// it, with tools whose module lists fixturePackages.
func fixtureRepository(t *testing.T, diff string) (string, tools) {
	t.Helper()
	root := t.TempDir()
	writeRepository(t, root, fixtureFiles)
	for name, content := range fixtureModFiles {
		writeFile(t, root, name, content)
	}
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

	// Only the lines that hold code in a function body are mutated.
	writeFile(t, root, "a.go", codeFile)
	p, err = planRun(tl, gateOptions{all: true, dirs: []string{"."}}, root)
	if err != nil {
		t.Fatal(err)
	}
	want := []lineRange{{9, 9}, {12, 14}}
	if got := p.units[0].lines["a.go"]; !slices.Equal(got, want) {
		t.Errorf("a.go lines %v, want %v", got, want)
	}

	// A package whose Go files hold no code, such as one of only a doc.go, has no
	// line to mutate and is not a unit; the packages beside it still are.
	writeFile(t, root, "cmd/tool/main.go", "// Command tool is a fixture.\npackage main\n")
	p, err = planRun(tl, gateOptions{all: true, dirs: []string{"."}}, root)
	if got := unitSummary(p); err != nil || !slices.Equal(got, []string{"m a.go"}) ||
		p.modules[0].changedFiles != 2 {
		t.Errorf("with a file of no code: units %q, %v", got, err)
	}

	// A Go file that does not parse stops the run.
	writeFile(t, root, "cmd/tool/main.go", "package main\n\nfunc {\n")
	_, err = planRun(tl, gateOptions{all: true, dirs: []string{"."}}, root)
	wantErr := "cannot tell the code lines of a Go file that does not parse: " +
		"cmd/tool/main.go:3:6: "
	if err == nil || !strings.HasPrefix(err.Error(), wantErr) {
		t.Errorf("with a file that does not parse: got %v, want an error that starts with %q",
			err, wantErr)
	}
}

// codeFile is a Go file with lines of each kind.
const codeFile = "// Package x is a fixture.\n" + // 1: a comment
	"package x\n" + // 2: the package clause
	"\n" + // 3
	"import (\n" + // 4: an import
	"\t\"fmt\"\n" + // 5
	")\n" + // 6
	"\n" + // 7
	"//line other.go:100\n" + // 8: a comment that changes the lines go reports
	"func f() { fmt.Print(1)\n" + // 9: code in a function body, after its {
	"\t// a comment in a function\n" + // 10
	"\n" + // 11
	"\tfmt.Println(`a raw string\n" + // 12: code, and a raw string over three lines
	"over\n" + // 13
	"three lines`)\n" + // 14
	"} /* a comment\n" + // 15: the end of the body, and a comment over two lines
	"on two lines */\n" + // 16
	"// v is one.\n" + // 17
	"var v = 1\n" // 18: a declaration outside function bodies

// The gate mutates only the changed lines that hold code in a function body. A
// changed file whose changed lines hold no code at all is not mutated, and its
// package is not a unit.
func TestPlanRunChangedCode(t *testing.T) {
	tests := []struct {
		name  string
		hunks string
		// want are the lines of a.go to mutate, none when it is not mutated.
		want []lineRange
	}{
		{name: "a comment", hunks: "@@ -1 +1 @@\n-x\n+y\n"},
		{name: "the package clause", hunks: "@@ -2 +2 @@\n-x\n+y\n"},
		{name: "an import", hunks: "@@ -5 +5 @@\n-x\n+y\n"},
		{
			name:  "a comment and a blank line in a function",
			hunks: "@@ -10,2 +10,2 @@\n-x\n-x\n+y\n+y\n",
		},
		{name: "the end of a comment after code", hunks: "@@ -16 +16 @@\n-x\n+y\n"},
		{
			name: "a comment and the code below it", hunks: "@@ -8,2 +8,2 @@\n-x\n-x\n+y\n+y\n",
			want: []lineRange{{9, 9}},
		},
		{
			name:  "two hunks, one of them with code",
			hunks: "@@ -1 +1 @@\n-x\n+y\n@@ -9 +9 @@\n-x\n+y\n",
			want:  []lineRange{{9, 9}},
		},
		{
			name: "the inside of a raw string", hunks: "@@ -13 +13 @@\n-x\n+y\n",
			want: []lineRange{{13, 13}},
		},
		{
			name: "a new file", hunks: "@@ -0,0 +1,18 @@\n" + strings.Repeat("+y\n", 18),
			want: []lineRange{{9, 9}, {12, 14}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, tl := fixtureRepository(t, changedDiff(map[string]string{"a.go": tt.hunks}))
			writeFile(t, root, "a.go", codeFile)
			t.Chdir(root)
			p, err := planRun(tl, gateOptions{base: "main", dirs: []string{"."}}, root)
			if err != nil {
				t.Fatal(err)
			}
			if p.modules[0].changedFiles != 1 {
				t.Errorf("%d changed files, want 1", p.modules[0].changedFiles)
			}
			if tt.want != nil {
				got := unitSummary(p)
				if !slices.Equal(got, []string{"m a.go"}) || len(p.modules[0].notMutated) != 0 {
					t.Fatalf("units %q, not mutated %q; want a unit of a.go", got,
						p.modules[0].notMutated)
				}
				if lines := p.units[0].lines["a.go"]; !slices.Equal(lines, tt.want) {
					t.Errorf("a.go lines %v, want %v", lines, tt.want)
				}
				return
			}
			want := []notMutated{{"a.go", "no code on the changed lines (comments, blank lines, " +
				"the package clause or imports)"}}
			if len(p.units) != 0 || !slices.Equal(p.modules[0].notMutated, want) {
				t.Errorf("units %q, not mutated %q; want no unit and %q", unitSummary(p),
					p.modules[0].notMutated, want)
			}
		})
	}
}

// bodiesFile is a Go file with code inside and outside function bodies. Go's
// coverage counts only the code between the braces of a function's body, so
// gremlins reports every mutant outside them as on a line no test covers.
const bodiesFile = "package x\n" + // 1
	"\n" + // 2
	"type pair struct{ a, b int }\n" + // 3: a type declaration
	"\n" + // 4
	"var limits = struct{ reason string }{\n" + // 5: a package-level composite literal
	"\treason: \"strings joined \" +\n" + // 6: with strings joined over two lines
	"\t\t\"over two lines\",\n" + // 7
	"}\n" + // 8
	"\n" + // 9
	"var dec = func(n int) int {\n" + // 10: a function literal in a package-level variable
	"\treturn n - 1\n" + // 11: its body
	"}(2) * 3\n" + // 12: the end of its body, and code after it
	"\n" + // 13
	"func (p pair) sum() int {\n" + // 14: a method
	"\treturn p.a + p.b\n" + // 15: its body
	"}\n" + // 16
	"\n" + // 17
	"func grow(a [2 + 1]int) [2 * 3]int {\n" + // 18: a signature with mutants
	"\treturn [2 * 3]int{a[0] - 1} }\n" + // 19: the last line of a body, with its }
	"\n" + // 20
	"// one is one.\n" + // 21
	"func one() int { return 2 - 1 }\n" + // 22: a function on one line
	"\n" + // 23
	"func _() { _ = func() int { return 2 - 1 }() }\n" + // 24: coverage leaves out func _
	"\n" + // 25
	"const limit = 1 << 10\n" // 26: a constant

// The gate mutates only the changed lines with code in a function body, the only
// code Go's coverage counts. A changed file whose changed lines hold code outside
// function bodies only, such as a package-level variable's value, is not mutated,
// and its package is not a unit. With -all, the same lines are mutated.
func TestPlanRunFunctionBodies(t *testing.T) {
	tests := []struct {
		name  string
		hunks string
		// want are the lines of a.go to mutate, none when it is not mutated.
		want []lineRange
	}{
		{
			name:  "strings joined in a package-level composite literal",
			hunks: "@@ -6,2 +6,2 @@\n-x\n-x\n+y\n+y\n",
		},
		{name: "a type declaration", hunks: "@@ -3 +3 @@\n-x\n+y\n"},
		{name: "a constant", hunks: "@@ -26 +26 @@\n-x\n+y\n"},
		{
			name:  "a comment and a declaration",
			hunks: "@@ -21 +21 @@\n-x\n+y\n@@ -26 +26 @@\n-x\n+y\n",
		},
		{
			name:  "a function literal in a package-level variable",
			hunks: "@@ -10,3 +10,3 @@\n-x\n-x\n-x\n+y\n+y\n+y\n",
			want:  []lineRange{{11, 11}},
		},
		{name: "code after the body of a function literal", hunks: "@@ -12 +12 @@\n-x\n+y\n"},
		{
			name:  "a method",
			hunks: "@@ -14,3 +14,3 @@\n-x\n-x\n-x\n+y\n+y\n+y\n",
			want:  []lineRange{{15, 15}},
		},
		{name: "a signature with mutants", hunks: "@@ -18 +18 @@\n-x\n+y\n"},
		{
			name: "the last line of a body, with its }", hunks: "@@ -19 +19 @@\n-x\n+y\n",
			want: []lineRange{{19, 19}},
		},
		{
			name: "a function on one line", hunks: "@@ -22 +22 @@\n-x\n+y\n",
			want: []lineRange{{22, 22}},
		},
		{name: "a function named _", hunks: "@@ -24 +24 @@\n-x\n+y\n"},
		{
			name: "a new file", hunks: "@@ -0,0 +1,26 @@\n" + strings.Repeat("+y\n", 26),
			want: []lineRange{{11, 11}, {15, 15}, {19, 19}, {22, 22}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, tl := fixtureRepository(t, changedDiff(map[string]string{"a.go": tt.hunks}))
			writeFile(t, root, "a.go", bodiesFile)
			t.Chdir(root)
			p, err := planRun(tl, gateOptions{base: "main", dirs: []string{"."}}, root)
			if err != nil {
				t.Fatal(err)
			}
			if tt.want != nil {
				got := unitSummary(p)
				if !slices.Equal(got, []string{"m a.go"}) || len(p.modules[0].notMutated) != 0 {
					t.Fatalf("units %q, not mutated %q; want a unit of a.go", got,
						p.modules[0].notMutated)
				}
				if lines := p.units[0].lines["a.go"]; !slices.Equal(lines, tt.want) {
					t.Errorf("a.go lines %v, want %v", lines, tt.want)
				}
				return
			}
			want := []notMutated{{"a.go", "no code in a function body on the changed lines " +
				"(declarations outside function bodies, which Go's coverage does not count)"}}
			if len(p.units) != 0 || !slices.Equal(p.modules[0].notMutated, want) ||
				p.modules[0].changedFiles != 1 {
				t.Errorf("units %q, not mutated %q, %d changed files; want no unit, %q and 1",
					unitSummary(p), p.modules[0].notMutated, p.modules[0].changedFiles, want)
			}
		})
	}
	t.Run("every Go file", func(t *testing.T) {
		root, tl := fixtureRepository(t, "")
		writeFile(t, root, "a.go", bodiesFile)
		t.Chdir(root)
		p, err := planRun(tl, gateOptions{all: true, dirs: []string{"."}}, root)
		if err != nil {
			t.Fatal(err)
		}
		want := []lineRange{{11, 11}, {15, 15}, {19, 19}, {22, 22}}
		if got := p.units[0].lines["a.go"]; !slices.Equal(got, want) {
			t.Errorf("a.go lines %v, want %v", got, want)
		}
	})
}

// uncountedFile is a Go file with code in function bodies that Go's coverage counts
// in no block, so gremlins reports every mutant there as on a line no test covers:
// a switch, type switch or select clause up to its colon, and the rest of a
// statement after the body of a function literal in it, up to the { of the
// statement's own body. Around such code it holds code that coverage counts.
const uncountedFile = "package x\n" + // 1
	"\n" + // 2
	"func apply(f func() int, n int) int { return f() + n }\n" + // 3
	"\n" + // 4
	"func clauses(a, b int, ch chan int, x any) int {\n" + // 5
	"\tswitch {\n" + // 6: counted, up to its {
	"\tcase a+1 > b:\n" + // 7: a case clause's expression
	"\t\treturn 1\n" + // 8
	"\tcase a*2 < b,\n" + // 9: an expression list over two lines
	"\t\ta-3 == b:\n" + // 10
	"\t\treturn 2\n" + // 11
	"\tcase a > 9: return 3\n" + // 12: a case clause and counted code on one line
	"\tcase a > 8:break\n" + // 13: and counted code right after its colon
	"\tdefault:\n" + // 14
	"\t}\n" + // 15
	"\tselect {\n" + // 16
	"\tcase ch <- a + 1:\n" + // 17: a select clause's send
	"\tdefault:\n" + // 18
	"\t}\n" + // 19
	"\tswitch x.(type) {\n" + // 20
	"\tcase [2 + 1]int:\n" + // 21: a type switch clause
	"\t}\n" + // 22
	"\tswitch {\n" + // 23
	"\tcase apply(func() int {\n" + // 24: a function literal in a case clause
	"\t\treturn a + 1\n" + // 25: its body, which coverage counts
	"\t}, 0) > 2:\n" + // 26
	"\t}\n" + // 27
	"\treturn 0\n" + // 28
	"}\n" + // 29
	"\n" + // 30
	"func tails(n int) (r int) {\n" + // 31
	"\tx := apply(func() int {\n" + // 32: counted, up to the function literal's {
	"\t\treturn n * 2\n" + // 33: its body
	"\t}, n) * 3\n" + // 34: the rest of the statement
	"\tx = apply(func() int { return n - 1 }, n) * 3\n" + // 35: a function literal and the rest
	"\tif v := apply(func() int {\n" + // 36
	"\t\treturn n\n" + // 37
	"\t}, n); v > 2 {\n" + // 38: the rest of an if statement's head
	"\t\tx++\n" + // 39
	"\t} else if apply(func() int {\n" + // 40
	"\t\treturn n\n" + // 41
	"\t}, 0) > 0 {\n" + // 42: the rest of an else if's head
	"\t}\n" + // 43
	"\tfor i := apply(func() int {\n" + // 44
	"\t\treturn 0\n" + // 45
	"\t}, 0); i < n; i++ {\n" + // 46: the rest of a for statement's head
	"\t}\n" + // 47
	"\tfor range apply(func() int {\n" + // 48
	"\t\treturn n\n" + // 49
	"\t}, 0) + 1 {\n" + // 50: the rest of a range expression
	"\t}\n" + // 51
	"\tswitch apply(func() int {\n" + // 52
	"\t\treturn n\n" + // 53
	"\t}, 1) % 2 {\n" + // 54: the rest of a switch statement's head
	"\t}\n" + // 55
	"\tswitch y := apply(func() int {\n" + // 56
	"\t\treturn n\n" + // 57
	"\t}, 0) - 1; any(y).(type) {\n" + // 58: the rest of a type switch's head
	"\t}\n" + // 59
	"\tswitch any(apply(func() int {\n" + // 60: a function literal in a type switch's x.(type),
	"\t\treturn n\n" + // 61
	"\t}, n-1)).(type) {\n" + // 62: which coverage counts, as it looks at the init only
	"\t}\n" + // 63
	"L:\n" + // 64
	"\tx = apply(func() int {\n" + // 65
	"\t\treturn n\n" + // 66
	"\t}, 0) + 1\n" + // 67: the rest of a labeled statement
	"\tif x < 0 {\n" + // 68
	"\t\tgoto L\n" + // 69
	"\t}\n" + // 70
	"\tdefer func() {\n" + // 71
	"\t\tr += x\n" + // 72
	"\t}()\n" + // 73: the call of a deferred function literal
	"\treturn apply(func() int {\n" + // 74
	"\t\treturn n\n" + // 75
	"\t}, apply(func() int {\n" + // 76: between two function literals
	"\t\treturn n + 2\n" + // 77: the body of the second, which coverage counts
	"\t}, n-1)) / 2\n" + // 78: after them
	"}\n" + // 79
	"\n" + // 80
	"func more(n int, ch chan int) (r int) {\n" + // 81
	"\tswitch {\n" + // 82
	"\tcase n > 1:\n" + // 83
	"\t\tr = apply(func() int {\n" + // 84: a function literal in a case clause's body
	"\t\t\treturn n + 1\n" + // 85
	"\t\t}, 0) * 2\n" + // 86: the rest of its statement
	"\t\tr -= n\n" + // 87: the next statement, which coverage counts
	"\t}\n" + // 88
	"\tselect {\n" + // 89
	"\tcase ch <- apply(func() int {\n" + // 90: a function literal in a select clause's send
	"\t\treturn n - 1\n" + // 91: its body, which coverage counts
	"\t}, 0) + 1:\n" + // 92
	"\t\tr = apply(func() int {\n" + // 93: a function literal in a select clause's body
	"\t\t\treturn n % 3\n" + // 94
	"\t\t}, 0) / 2\n" + // 95: the rest of its statement
	"\t\tr += n\n" + // 96: the next statement
	"\tdefault:\n" + // 97
	"\t}\n" + // 98
	"\t{\n" + // 99
	"\t\tr = apply(func() int {\n" + // 100: a function literal in a block
	"\t\t\treturn n\n" + // 101
	"\t\t}, 0) << 1\n" + // 102: the rest of its statement
	"\t\tr |= n\n" + // 103: the next statement
	"\t}\n" + // 104
	"L:\n" + // 105
	"\tfor i := 0; i < apply(func() int {\n" + // 106: a function literal in a condition
	"\t\treturn n & 1\n" + // 107
	"\t}, 0)+n; i++ {\n" + // 108: the rest of a labeled for statement's head
	"\t\tr ^= i\n" + // 109: its body
	"\t\tcontinue L\n" + // 110
	"\t}\n" + // 111
	"\tfor i := 0; i < n; i = apply(func() int {\n" + // 112: a function literal in a post
	"\t\treturn i + 1\n" + // 113
	"\t}, 0) + i {\n" + // 114: the rest of the for statement's head
	"\t\tr--\n" + // 115
	"\t}\n" + // 116
	"\tswitch y := apply(func() int {\n" + // 117: a function literal in a switch's init
	"\t\treturn n\n" + // 118
	"\t}, 0) * 2; y {\n" + // 119: the rest of the switch statement's head
	"\t}\n" + // 120
	"\tswitch y := apply(func() int {\n" + // 121: a function literal in a type switch's init
	"\t\treturn n\n" + // 122
	"\t}, 0) - 1; any(apply(func() int {\n" + // 123: and one in its x.(type)
	"\t\treturn y + 2\n" + // 124: the body of the second, which coverage counts
	"\t}, 0)).(type) {\n" + // 125
	"\t}\n" + // 126
	"\tr = apply(func() int {\n" + // 127
	"\t\tv := apply(func() int {\n" + // 128: a function literal in a function literal
	"\t\t\treturn n\n" + // 129
	"\t\t}, 0)\n" + // 130
	"\t\treturn v + 3\n" + // 131: the outer one's next statement, which coverage counts
	"\t}, 0) - 1\n" + // 132: the rest of the outer statement
	"\treturn r\n" + // 133
	"}\n" // 134

// uncountedCode are the lines of uncountedFile that the gate mutates: those with
// code that Go's coverage counts.
var uncountedCode = []lineRange{
	{3, 3},
	{6, 6},
	{8, 8},
	{11, 13},
	{15, 16},
	{19, 20},
	{22, 23},
	{25, 25},
	{27, 28},
	{32, 33},
	{35, 37},
	{39, 41},
	{43, 45},
	{47, 49},
	{51, 53},
	{55, 57},
	{59, 66},
	{68, 72},
	{74, 75},
	{77, 77},
	{82, 82},
	{84, 85},
	{87, 89},
	{91, 91},
	{93, 94},
	{96, 96},
	{98, 101},
	{103, 107},
	{109, 113},
	{115, 118},
	{120, 122},
	{124, 124},
	{126, 129},
	{131, 131},
	{133, 133},
}

// The gate mutates only the changed lines with code that Go's coverage counts. In a
// function body, coverage counts no block for a switch, type switch or select clause
// up to its colon, or for the rest of a statement after the body of a function
// literal in it, up to the { of the statement's own body, so a changed file whose
// changed lines hold only such code is not mutated, and its package is not a unit. A
// line that also holds code that coverage counts is mutated. With -all, the same
// lines are mutated.
func TestPlanRunUncountedCode(t *testing.T) {
	tests := []struct {
		name  string
		hunks string
		// want are the lines of a.go to mutate, none when it is not mutated.
		want []lineRange
	}{
		{name: "a case clause's expression", hunks: "@@ -7 +7 @@\n-x\n+y\n"},
		{
			name:  "an expression list over two lines",
			hunks: "@@ -9,2 +9,2 @@\n-x\n-x\n+y\n+y\n",
		},
		{name: "a default clause", hunks: "@@ -14 +14 @@\n-x\n+y\n"},
		{name: "a select clause's send", hunks: "@@ -17 +17 @@\n-x\n+y\n"},
		{name: "a type switch clause", hunks: "@@ -21 +21 @@\n-x\n+y\n"},
		{
			name:  "a switch and its first clause",
			hunks: "@@ -6,3 +6,3 @@\n-x\n-x\n-x\n+y\n+y\n+y\n",
			want:  []lineRange{{6, 6}, {8, 8}},
		},
		{
			name: "a case clause and code after its colon", hunks: "@@ -12 +12 @@\n-x\n+y\n",
			want: []lineRange{{12, 12}},
		},
		{
			name: "code right after a case clause's colon", hunks: "@@ -13 +13 @@\n-x\n+y\n",
			want: []lineRange{{13, 13}},
		},
		{
			name:  "a function literal in a case clause",
			hunks: "@@ -24,3 +24,3 @@\n-x\n-x\n-x\n+y\n+y\n+y\n",
			want:  []lineRange{{25, 25}},
		},
		{
			name:  "a statement with a function literal",
			hunks: "@@ -32,3 +32,3 @@\n-x\n-x\n-x\n+y\n+y\n+y\n",
			want:  []lineRange{{32, 33}},
		},
		{
			name:  "the rest of a statement after a function literal",
			hunks: "@@ -34 +34 @@\n-x\n+y\n",
		},
		{
			name:  "a function literal and the rest of its statement on one line",
			hunks: "@@ -35 +35 @@\n-x\n+y\n",
			want:  []lineRange{{35, 35}},
		},
		{name: "the rest of an if statement's head", hunks: "@@ -38 +38 @@\n-x\n+y\n"},
		{name: "the rest of an else if's head", hunks: "@@ -42 +42 @@\n-x\n+y\n"},
		{name: "the rest of a for statement's head", hunks: "@@ -46 +46 @@\n-x\n+y\n"},
		{name: "the rest of a range expression", hunks: "@@ -50 +50 @@\n-x\n+y\n"},
		{name: "the rest of a switch statement's head", hunks: "@@ -54 +54 @@\n-x\n+y\n"},
		{name: "the rest of a type switch's head", hunks: "@@ -58 +58 @@\n-x\n+y\n"},
		{
			name:  "a function literal in a type switch's x.(type)",
			hunks: "@@ -62 +62 @@\n-x\n+y\n",
			want:  []lineRange{{62, 62}},
		},
		{name: "the rest of a labeled statement", hunks: "@@ -67 +67 @@\n-x\n+y\n"},
		{name: "the call of a deferred function literal", hunks: "@@ -73 +73 @@\n-x\n+y\n"},
		{name: "between two function literals", hunks: "@@ -76 +76 @@\n-x\n+y\n"},
		{name: "after two function literals", hunks: "@@ -78 +78 @@\n-x\n+y\n"},
		{
			name:  "two function literals and the body of the second",
			hunks: "@@ -76,3 +76,3 @@\n-x\n-x\n-x\n+y\n+y\n+y\n",
			want:  []lineRange{{77, 77}},
		},
		{
			name:  "a case clause's body with a function literal",
			hunks: "@@ -84,4 +84,4 @@\n-x\n-x\n-x\n-x\n+y\n+y\n+y\n+y\n",
			want:  []lineRange{{84, 85}, {87, 87}},
		},
		{
			name:  "a function literal in a select clause's send",
			hunks: "@@ -90,3 +90,3 @@\n-x\n-x\n-x\n+y\n+y\n+y\n",
			want:  []lineRange{{91, 91}},
		},
		{
			name:  "a select clause's body with a function literal",
			hunks: "@@ -93,4 +93,4 @@\n-x\n-x\n-x\n-x\n+y\n+y\n+y\n+y\n",
			want:  []lineRange{{93, 94}, {96, 96}},
		},
		{
			name:  "a block with a function literal",
			hunks: "@@ -100,4 +100,4 @@\n-x\n-x\n-x\n-x\n+y\n+y\n+y\n+y\n",
			want:  []lineRange{{100, 101}, {103, 103}},
		},
		{
			name:  "a labeled for statement with a function literal in its condition",
			hunks: "@@ -105,7 +105,7 @@\n" + strings.Repeat("-x\n", 7) + strings.Repeat("+y\n", 7),
			want:  []lineRange{{105, 107}, {109, 111}},
		},
		{name: "the rest of a for statement's post statement", hunks: "@@ -114 +114 @@\n-x\n+y\n"},
		{name: "the rest of a switch statement's init", hunks: "@@ -119 +119 @@\n-x\n+y\n"},
		{
			name:  "function literals in a type switch's init and x.(type)",
			hunks: "@@ -123,3 +123,3 @@\n-x\n-x\n-x\n+y\n+y\n+y\n",
			want:  []lineRange{{124, 124}},
		},
		{
			name:  "a function literal in a function literal",
			hunks: "@@ -127,6 +127,6 @@\n" + strings.Repeat("-x\n", 6) + strings.Repeat("+y\n", 6),
			want:  []lineRange{{127, 129}, {131, 131}},
		},
		{
			name: "a new file", hunks: "@@ -0,0 +1,134 @@\n" + strings.Repeat("+y\n", 134),
			want: uncountedCode,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, tl := fixtureRepository(t, changedDiff(map[string]string{"a.go": tt.hunks}))
			writeFile(t, root, "a.go", uncountedFile)
			t.Chdir(root)
			p, err := planRun(tl, gateOptions{base: "main", dirs: []string{"."}}, root)
			if err != nil {
				t.Fatal(err)
			}
			if tt.want != nil {
				got := unitSummary(p)
				if !slices.Equal(got, []string{"m a.go"}) || len(p.modules[0].notMutated) != 0 {
					t.Fatalf("units %q, not mutated %q; want a unit of a.go", got,
						p.modules[0].notMutated)
				}
				if lines := p.units[0].lines["a.go"]; !slices.Equal(lines, tt.want) {
					t.Errorf("a.go lines %v, want %v", lines, tt.want)
				}
				return
			}
			want := []notMutated{{"a.go", "no code that Go's coverage counts on the changed " +
				"lines (a case clause up to its colon, or the rest of a statement after the body " +
				"of a function literal in it)"}}
			if len(p.units) != 0 || !slices.Equal(p.modules[0].notMutated, want) ||
				p.modules[0].changedFiles != 1 {
				t.Errorf("units %q, not mutated %q, %d changed files; want no unit, %q and 1",
					unitSummary(p), p.modules[0].notMutated, p.modules[0].changedFiles, want)
			}
		})
	}
	t.Run("every Go file", func(t *testing.T) {
		root, tl := fixtureRepository(t, "")
		writeFile(t, root, "a.go", uncountedFile)
		t.Chdir(root)
		p, err := planRun(tl, gateOptions{all: true, dirs: []string{"."}}, root)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.units[0].lines["a.go"]; !slices.Equal(got, uncountedCode) {
			t.Errorf("a.go lines %v, want %v", got, uncountedCode)
		}
	})
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
			name: "a changed Go file that does not parse", dirs: []string{"."},
			diff: changedDiff(map[string]string{"a.go": "@@ -3 +3 @@\n-x\n+y\n"}),
			edit: func(t *testing.T, root string, _ *tools) {
				t.Helper()
				writeFile(t, root, "a.go", "package x\n\nfunc {\n")
			},
			want: "cannot tell the code lines of a Go file that does not parse: a.go:3:6: ",
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
