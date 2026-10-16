package scripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// editorconfigChecker is the tool that tools/go.mod pins and the prek hook runs.
const editorconfigChecker = "github.com/editorconfig-checker/editorconfig-checker/v4/cmd/" +
	"editorconfig-checker"

// buildEditorconfigChecker builds editorconfigChecker at the version tools/go.mod pins and
// returns the path of the binary. The go command needs the module in the module cache, or
// access to the Go module proxy.
func buildEditorconfigChecker(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "editorconfig-checker")
	result := runScript(t, ".", nil, "go", "build", "-o", bin, editorconfigChecker)
	if result.exit != 0 {
		t.Fatalf("go build %s exited %d; it needs the module in the module cache or access to "+
			"the Go module proxy:\n%s", editorconfigChecker, result.exit, result.stderr)
	}
	return bin
}

func TestEditorconfig(t *testing.T) {
	t.Parallel()
	checker := buildEditorconfigChecker(t)
	editorconfig, err := os.ReadFile(repositoryFile(".editorconfig"))
	if err != nil {
		t.Fatal(err)
	}
	// parentEditorconfig sits in the directory above the repository. It asks for tabs, which the
	// repository's .editorconfig leaves unset for Markdown and text, so the space-indented
	// Markdown and text fixtures pass only while that file says root = true.
	const parentEditorconfig = "root = true\n\n[*]\nindent_style = tab\n"
	line := func(n int) string { return "// " + strings.Repeat("x", n-3) + "\n" }
	goFile := "package a\n\n// F returns 1.\nfunc F() int {\n\treturn 1\n}\n"
	tests := []struct {
		name string
		// files are written into the repository; the test runs the checker on each of them.
		files      map[string]string
		wantExit   int
		wantOutput string
	}{
		{
			name: "files that follow every rule, under a parent .editorconfig (root = true)",
			files: map[string]string{
				"a.go": goFile, "go.mod": "module example.com/a\n\ngo 1.26.0\n",
				"README.md": "# Title\n\n- a list\n  - in a list\n", "x.yml": "a:\n  b: 1\n",
				"x.yaml": "a:\n  - 1\n", "run.sh": "#!/bin/sh\nif true; then\n  echo hi\nfi\n",
				"notes.txt": "\u00e9t\u00e9\n    indented\n",
			},
			wantExit: 0,
		},
		{
			name: "CRLF line endings", files: map[string]string{"a.go": "package a\r\n"},
			wantExit: 1, wantOutput: "wrong line endings or no final newline",
		},
		{
			name: "no final newline", files: map[string]string{"a.txt": "text"},
			wantExit: 1, wantOutput: "wrong line endings or no final newline",
		},
		{
			name: "trailing whitespace in Go", files: map[string]string{"a.go": "package a \n"},
			wantExit: 1, wantOutput: "trailing whitespace",
		},
		{
			name: "trailing whitespace in Markdown", files: map[string]string{"a.md": "text  \n"},
			wantExit: 1, wantOutput: "trailing whitespace",
		},
		{
			name:     "spaces that indent Go",
			files:    map[string]string{"a.go": strings.Replace(goFile, "\t", "    ", 1)},
			wantExit: 1, wantOutput: "wrong indentation type (spaces instead of tabs)",
		},
		{
			name: "a 100-column Go line", files: map[string]string{"a.go": line(100)},
			wantExit: 0,
		},
		{
			name: "a 101-column Go line", files: map[string]string{"a.go": line(101)},
			wantExit: 1, wantOutput: "line too long (101 instead of 100)",
		},
		{
			name: "a 101-column line in another file", files: map[string]string{"a.txt": line(101)},
			wantExit: 1, wantOutput: "line too long (101 instead of 100)",
		},
		{
			name: "a 120-column Markdown line", files: map[string]string{"a.md": line(120)},
			wantExit: 0,
		},
		{
			name: "long lines in test fixtures, spec snapshots and the binary lock",
			files: map[string]string{
				"testdata/a.json": line(300), "pkg/testdata/a.json": line(300),
				"tools/specs/openapi.json": line(300), "tools/binaries.lock": line(130),
			},
			wantExit: 0,
		},
		{
			name: "a tab that indents YAML", files: map[string]string{"a.yml": "a:\n\tb: 1\n"},
			wantExit: 1, wantOutput: "wrong indent style found (tabs instead of spaces)",
		},
		{
			name:       "three spaces that indent YAML",
			files:      map[string]string{"a.yaml": "a:\n   b: 1\n"},
			wantExit:   1,
			wantOutput: "wrong amount of left-padding spaces(want multiple of 2)",
		},
		{
			name:       "a tab that indents shell",
			files:      map[string]string{"a.sh": "if x; then\n\ty\nfi\n"},
			wantExit:   1,
			wantOutput: "wrong indent style found (tabs instead of spaces)",
		},
		{
			name: "a Latin-1 file",
			files: map[string]string{
				"a.txt": strings.Repeat("Le th\xe9 est fran\xe7ais et tr\xe8s bon.\n", 5),
			},
			wantExit: 1, wantOutput: `wrong character encoding ("ISO-8859-1" instead of "utf-8")`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parent := t.TempDir()
			root := filepath.Join(parent, "repo")
			writeFiles(t, parent, map[string]string{".editorconfig": parentEditorconfig})
			writeFiles(t, root, map[string]string{".editorconfig": string(editorconfig)})
			writeFiles(t, root, tt.files)
			args := []string{"-no-color"}
			for name := range tt.files {
				args = append(args, filepath.FromSlash(name))
			}

			result := runScript(t, root, nil, checker, args...)

			output := result.stdout + result.stderr
			if result.exit != tt.wantExit {
				t.Errorf("exit = %d, want %d; output:\n%s", result.exit, tt.wantExit, output)
			}
			if !strings.Contains(output, tt.wantOutput) {
				t.Errorf("output does not contain %q:\n%s", tt.wantOutput, output)
			}
		})
	}
}
