package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/mod/module"
)

// testLimits are small limits for the fixtures: a per-file cap of 1 KiB and a budget that every
// fixture keeps to unless a test lowers it.
var testLimits = limits{
	fileCap: 1 << 10,
	budget: budget{
		baseline: 1 << 20, measured: "2026-10-14", headroom: 0, reason: "the fixtures",
	},
}

// verdicts is the last line zipcheck prints for each exit code.
var verdicts = map[int]string{
	exitPassed:    "zipcheck: PASS\n",
	exitFailed:    "zipcheck: FAIL\n",
	exitDidNotRun: "zipcheck: DID NOT RUN\n",
}

// validFiles are the files of a repository whose module zip keeps every rule of testLimits. The
// zip leaves out the nested modules tools/ and consumer/, with the specification and the image in
// them, and ignored.txt, which .gitattributes marks export-ignore.
func validFiles() map[string]string {
	return map[string]string{
		".editorconfig":                   "root = true\n",
		".gitattributes":                  "ignored.txt export-ignore\n",
		".github/workflows/ci.yml":        "name: CI\n",
		"AGENTS.md":                       "# Agents\n",
		"CHANGELOG.md":                    "# Changelog\n",
		"CODE_OF_CONDUCT.md":              "# Code of conduct\n",
		"CONTRIBUTING.md":                 "# Contributing\n",
		"LICENSE":                         "MIT License\n",
		"MIGRATION.md":                    "# Migration\n",
		"README.md":                       "# m\n",
		"SECURITY.md":                     "# Security\n",
		"_typos.toml":                     "[files]\n",
		"consumer/go.mod":                 "module example.com/consumer\n",
		"consumer/main_test.go":           "package consumer\n",
		"doc.go":                          "// Package m is a module.\npackage m\n",
		"go.mod":                          "module example.com/m/v2\n\ngo 1.26.0\n",
		"go.sum":                          "",
		"ignored.txt":                     "left out by git archive\n",
		"internal/x/testdata/golden.json": "{}\n",
		"internal/x/x_test.go":            "package x\n",
		"llms.txt":                        "# m\n",
		"testdata/blob.bin":               "\x00\xff binary\n",
		"tools/go.mod":                    "module example.com/m/v2/tools\n",
		"tools/logo.png":                  "\x89PNG\r\n",
		"tools/specs/openapi.json":        "{}\n",
	}
}

// validZip lists the files of the module zip of validFiles.
var validZip = []string{
	".editorconfig", ".gitattributes", ".github/workflows/ci.yml", "AGENTS.md", "CHANGELOG.md",
	"CODE_OF_CONDUCT.md", "CONTRIBUTING.md", "LICENSE", "MIGRATION.md", "README.md",
	"SECURITY.md", "_typos.toml", "doc.go", "go.mod", "go.sum", "internal/x/testdata/golden.json",
	"internal/x/x_test.go", "llms.txt", "testdata/blob.bin",
}

// gitIn runs git in dir with a fixed author, no user or system configuration and no automatic
// maintenance or gc, and returns its output. A process that keeps the output open after git exits
// fails the test after WaitDelay instead of hanging it.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=maintenance.auto", "GIT_CONFIG_VALUE_0=false",
		"GIT_CONFIG_KEY_1=gc.auto", "GIT_CONFIG_VALUE_1=0",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	cmd.WaitDelay = 30 * time.Second
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// A git commit starts git maintenance, which can run git gc, in a background process that outlives
// the commit; gitIn turns both off.
func TestGitInTurnsOffMaintenance(t *testing.T) {
	dir := t.TempDir()
	for key, want := range map[string]string{"maintenance.auto": "false", "gc.auto": "0"} {
		if got := gitIn(t, dir, "config", "--get", key); got != want {
			t.Errorf("git config --get %s = %q, want %q", key, got, want)
		}
	}
}

// newRepo creates a git repository with files, a map from a slash path to its content, in its
// working tree, and returns its directory. Nothing is committed yet.
func newRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init", "--quiet", "--initial-branch=main")
	for name, content := range files {
		file := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// commitAll commits every file in the working tree of the repository in dir.
func commitAll(t *testing.T, dir string) {
	t.Helper()
	gitIn(t, dir, "add", "--all")
	gitIn(t, dir, "commit", "--quiet", "--message", "fixture")
}

// committedRepo creates a git repository with files committed, and returns its directory.
func committedRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := newRepo(t, files)
	commitAll(t, dir)
	return dir
}

// runZipcheck runs zipcheck on the repository in dir with lim and the extra arguments, and
// returns its exit code and what it printed on stdout.
func runZipcheck(t *testing.T, dir string, lim limits, args ...string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	exit := run(t.Context(), append([]string{"-repo", dir}, args...), &stdout, &stderr, lim)
	return exit, stdout.String()
}

// assertResult checks the exit code, that the output holds every part of want, and that it ends
// with the verdict of the exit code.
func assertResult(t *testing.T, exit, wantExit int, output string, want ...string) {
	t.Helper()
	if exit != wantExit {
		t.Errorf("exit = %d, want %d; output:\n%s", exit, wantExit, output)
	}
	for _, part := range want {
		if !strings.Contains(output, part) {
			t.Errorf("output does not hold %q:\n%s", part, output)
		}
	}
	if !strings.HasSuffix(output, verdicts[wantExit]) {
		t.Errorf("output does not end with %q:\n%s", verdicts[wantExit], output)
	}
}

func TestBuildZip(t *testing.T) {
	// CLAUDE.md is a symbolic link, which a module zip leaves out, and untracked.go is not
	// committed.
	dir := newRepo(t, validFiles())
	if err := os.Symlink("AGENTS.md", filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	commitAll(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "untracked.go"), []byte("package m\n"),
		0o600); err != nil {
		t.Fatal(err)
	}

	version, data, err := buildZip(t.Context(), dir, "")
	if err != nil {
		t.Fatal(err)
	}
	files, err := readZip(version, data)
	if err != nil {
		t.Fatal(err)
	}

	want := module.Version{Path: "example.com/m/v2", Version: "v2.0.0"}
	if version != want {
		t.Errorf("version = %v, want %v", version, want)
	}
	var names []string
	for _, f := range files {
		names = append(names, f.name)
	}
	if !slices.Equal(names, validZip) {
		t.Errorf("the zip holds\n\t%s\nwant\n\t%s", strings.Join(names, "\n\t"),
			strings.Join(validZip, "\n\t"))
	}
	blob := files[slices.Index(validZip, "testdata/blob.bin")]
	if blob.text || blob.size != int64(len("\x00\xff binary\n")) || blob.compressed == 0 {
		t.Errorf("testdata/blob.bin is %+v, want a binary file of 10 bytes", blob)
	}
	if license := files[slices.Index(validZip, "LICENSE")]; !license.text {
		t.Errorf("LICENSE is %+v, want a text file", license)
	}
}

func TestBuildZipWithoutAMajorVersionSuffix(t *testing.T) {
	files := validFiles()
	files["go.mod"] = "module example.com/m\n\ngo 1.26.0\n"
	dir := committedRepo(t, files)

	version, data, err := buildZip(t.Context(), dir, "")
	if err != nil {
		t.Fatal(err)
	}

	want := module.Version{Path: "example.com/m", Version: "v0.0.0"}
	if version != want {
		t.Errorf("version = %v, want %v", version, want)
	}
	if _, err := readZip(version, data); err != nil {
		t.Errorf("readZip() of the zip of %v = %v, want nil", version, err)
	}
}

// TestBuildZipIgnoresGitConfiguration checks that the zip is the same with or without git
// configuration that would change what git archive prints, as the module proxy has none.
func TestBuildZipIgnoresGitConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		configure func(t *testing.T, dir string)
	}{
		{
			// It would end every text line with CRLF.
			name: "core.autocrlf in the repository",
			configure: func(t *testing.T, dir string) {
				t.Helper()
				gitIn(t, dir, "config", "core.autocrlf", "true")
			},
		},
		{
			name: "a global configuration",
			configure: func(t *testing.T, _ string) {
				t.Helper()
				t.Setenv("GIT_CONFIG_GLOBAL", exportIgnoreConfig(t))
			},
		},
		{
			name: "a system configuration",
			configure: func(t *testing.T, _ string) {
				t.Helper()
				t.Setenv("GIT_CONFIG_SYSTEM", exportIgnoreConfig(t))
			},
		},
		{
			// git reads $XDG_CONFIG_HOME/git/attributes when no configuration names one.
			name: "the user's attributes file",
			configure: func(t *testing.T, _ string) {
				t.Helper()
				home := t.TempDir()
				if err := os.Mkdir(filepath.Join(home, "git"), 0o700); err != nil {
					t.Fatal(err)
				}
				attributes := filepath.Join(home, "git", "attributes")
				if err := os.WriteFile(attributes, []byte("LICENSE export-ignore\n"),
					0o600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("XDG_CONFIG_HOME", home)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := committedRepo(t, validFiles())
			version, want, err := buildZip(t.Context(), dir, "")
			if err != nil {
				t.Fatal(err)
			}
			tt.configure(t, dir)

			_, got, err := buildZip(t.Context(), dir, "")
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(got, want) {
				gotFiles, _ := readZip(version, got)
				wantFiles, _ := readZip(version, want)
				t.Errorf("the zip changed with %s; it holds\n\t%+v\nwant\n\t%+v", tt.name,
					gotFiles, wantFiles)
			}
		})
	}
}

// exportIgnoreConfig writes a git configuration whose attributes file marks LICENSE
// export-ignore, which leaves it out of what git archive prints, and returns its path.
func exportIgnoreConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	attributes := filepath.Join(dir, "attributes")
	config := filepath.Join(dir, "config")
	if err := os.WriteFile(attributes, []byte("LICENSE export-ignore\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("[core]\n\tattributesFile = %q\n", attributes)
	if err := os.WriteFile(config, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return config
}

func TestRunPasses(t *testing.T) {
	dir := committedRepo(t, validFiles())

	exit, stdout := runZipcheck(t, dir, testLimits)

	assertResult(t, exit, exitPassed, stdout,
		"zipcheck: example.com/m/v2@v2.0.0: 19 files, ",
		" bytes compressed, of a budget of 1048576 bytes\n"+
			"zipcheck: the 10 largest files, in bytes uncompressed and compressed:\n"+
			"zipcheck:         36 ")
	if strings.Contains(stdout, "FAIL") {
		t.Errorf("output reports a failure:\n%s", stdout)
	}
}

func TestRunFails(t *testing.T) {
	big := strings.Repeat("x", 1<<10+1)
	tests := []struct {
		name   string
		change map[string]string // files to add to validFiles, or to remove when ""
		lim    limits
		want   string
	}{
		{
			name:   "a file over the per-file cap",
			change: map[string]string{"testdata/big.txt": big},
			want:   "zipcheck: FAIL: testdata/big.txt: 1025 bytes, over the per-file cap of 1024",
		},
		{
			name:   "an image",
			change: map[string]string{"internal/x/testdata/logo.PNG": "\x89PNG\r\n"},
			want:   "zipcheck: FAIL: internal/x/testdata/logo.PNG: an image;",
		},
		{
			name:   "an API specification",
			change: map[string]string{"testdata/openapi.json": "{}\n"},
			want:   "zipcheck: FAIL: testdata/openapi.json: an API specification;",
		},
		{
			name:   "tools/ without its go.mod",
			change: map[string]string{"tools/go.mod": ""},
			want:   "zipcheck: FAIL: tools/logo.png: tools/ must stay out of the module zip;",
		},
		{
			name:   "no LICENSE",
			change: map[string]string{"LICENSE": ""},
			want:   "zipcheck: FAIL: LICENSE: not in the zip;",
		},
		{
			name:   "a file at the root that is not on the allow list",
			change: map[string]string{"notes.txt": "notes\n"},
			want:   "zipcheck: FAIL: notes.txt: not on the allow list of the module zip;",
		},
		{
			name:   "a root file in a subdirectory",
			change: map[string]string{"internal/README.md": "# internal\n"},
			want:   "zipcheck: FAIL: internal/README.md: not on the allow list",
		},
		{
			name:   "a binary file outside testdata",
			change: map[string]string{".github/blob": "a\x00b\n"},
			want:   "zipcheck: FAIL: .github/blob: not text,",
		},
		{
			name:   "a Go file that is not UTF-8",
			change: map[string]string{"bad.go": "package m // \xff\n"},
			want:   "zipcheck: FAIL: bad.go: not text,",
		},
		{
			name: "a zip over the budget",
			lim: limits{fileCap: 1 << 10, budget: budget{
				baseline: 100, measured: "2026-10-14", headroom: 50, reason: "the fixtures",
			}},
			want: " bytes compressed, over the budget of 150: 100 measured on 2026-10-14 plus " +
				"50 of headroom;",
		},
		{
			name:   "a file over the cap of its exception",
			change: map[string]string{"testdata/big.txt": big + "x"},
			lim: limits{fileCap: 1 << 10, budget: testLimits.budget, exceptions: []exception{
				{path: "testdata/big.txt", maxBytes: 1<<10 + 1, reason: "a fixture"},
			}},
			want: "zipcheck: FAIL: testdata/big.txt: 1026 bytes, over the cap of 1025 bytes " +
				"that its exception",
		},
		{
			name: "an exception for a file that is not in the zip",
			lim: limits{fileCap: 1 << 10, budget: testLimits.budget, exceptions: []exception{
				{path: "testdata/gone.txt", maxBytes: 1 << 11, reason: "a fixture"},
			}},
			want: "zipcheck: FAIL: testdata/gone.txt: not in the zip, but limits.go has an " +
				"exception for it;",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := validFiles()
			for name, content := range tt.change {
				if content == "" {
					delete(files, name)
				} else {
					files[name] = content
				}
			}
			lim := tt.lim
			if lim.fileCap == 0 {
				lim = testLimits
			}
			dir := committedRepo(t, files)

			exit, stdout := runZipcheck(t, dir, lim)

			assertResult(t, exit, exitFailed, stdout, tt.want,
				"zipcheck: the 10 largest files, in bytes uncompressed and compressed:\n")
		})
	}
}

func TestRunPassesAFileWithAnException(t *testing.T) {
	files := validFiles()
	files["testdata/big.txt"] = strings.Repeat("x", 1<<10+1)
	dir := committedRepo(t, files)
	lim := testLimits
	lim.exceptions = []exception{
		{path: "testdata/big.txt", maxBytes: 1<<10 + 1, reason: "a fixture"},
	}

	exit, stdout := runZipcheck(t, dir, lim)

	assertResult(t, exit, exitPassed, stdout,
		"zipcheck:       1025 ", "  testdata/big.txt\n")
}

// TestRunFromASubdirectory checks that zipcheck checks the whole repository from any directory in
// it, a nested module's too, as go -C tools run ./cmd/zipcheck runs it.
func TestRunFromASubdirectory(t *testing.T) {
	dir := committedRepo(t, validFiles())
	_, want := runZipcheck(t, dir, testLimits)

	for _, sub := range []string{"internal/x", "tools"} {
		t.Run(sub, func(t *testing.T) {
			exit, stdout := runZipcheck(t, filepath.Join(dir, filepath.FromSlash(sub)), testLimits)

			assertResult(t, exit, exitPassed, stdout)
			if stdout != want {
				t.Errorf("zipcheck in %s printed\n%s\nwant what it prints at the root:\n%s", sub,
					stdout, want)
			}
		})
	}
}

// TestRunFailsOnAFileTheZipCannotHold checks that a committed file whose name the module zip
// format rejects is a failure that names the file: the module proxy cannot serve such a commit.
func TestRunFailsOnAFileTheZipCannotHold(t *testing.T) {
	files := validFiles()
	files["testdata/aux.json"] = "{}\n" // Windows reserves the name aux.
	files["testdata/a:b.txt"] = "a\n"
	dir := committedRepo(t, files)

	exit, stdout := runZipcheck(t, dir, testLimits)

	assertResult(t, exit, exitFailed, stdout,
		"zipcheck: FAIL: testdata/a:b.txt: malformed file path \"testdata/a:b.txt\": ",
		"zipcheck: FAIL: testdata/aux.json: malformed file path \"testdata/aux.json\": "+
			"\"aux\" disallowed as path element component on Windows; a module zip cannot hold "+
			"it, so the module proxy cannot serve the commit: rename or remove it\n")
}

func TestRunDidNotRun(t *testing.T) {
	noModule := validFiles()
	delete(noModule, "go.mod")
	noPath := validFiles()
	noPath["go.mod"] = "go 1.26.0\n"
	badPath := validFiles()
	badPath["go.mod"] = "module m\n"
	badLimits := testLimits
	badLimits.fileCap = 0
	tests := []struct {
		name string
		// files are the files of the repository's working tree; with none, the directory is not
		// a git repository.
		files  map[string]string
		commit bool
		args   []string
		lim    limits
		want   string
	}{
		{
			name: "a directory that is not a git repository",
			want: "zipcheck: DID NOT RUN: git rev-parse --show-toplevel in ",
		},
		{
			name:  "a repository without a commit",
			files: validFiles(),
			want:  "zipcheck: DID NOT RUN: git archive HEAD in ",
		},
		{
			name:  "a commit without go.mod",
			files: noModule, commit: true,
			want: "zipcheck: DID NOT RUN: the commit has no go.mod at its root,",
		},
		{
			name:  "a go.mod without a module path",
			files: noPath, commit: true,
			want: "zipcheck: DID NOT RUN: the commit's go.mod names no module path\n",
		},
		{
			name:  "a module path that is not valid",
			files: badPath, commit: true,
			want: "zipcheck: DID NOT RUN: building the module zip of m: ",
		},
		{
			name:  "an argument",
			files: validFiles(), commit: true,
			args: []string{"extra"},
			want: "zipcheck: DID NOT RUN: unexpected arguments [\"extra\"]\n",
		},
		{
			name:  "an unknown flag",
			files: validFiles(), commit: true,
			args: []string{"-unknown"},
			want: "zipcheck: DID NOT RUN: flag provided but not defined: -unknown\n",
		},
		{
			name:  "limits that are not valid",
			files: validFiles(), commit: true,
			lim: badLimits,
			want: "zipcheck: DID NOT RUN: the limits in limits.go are not valid: the per-file " +
				"cap is 0 bytes;",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.files != nil {
				dir = newRepo(t, tt.files)
			}
			if tt.commit {
				commitAll(t, dir)
			}
			lim := tt.lim
			if lim.budget.reason == "" {
				lim = testLimits
			}

			exit, stdout := runZipcheck(t, dir, lim, tt.args...)

			assertResult(t, exit, exitDidNotRun, stdout, tt.want)
			if strings.Contains(stdout, "largest") {
				t.Errorf("output lists the largest files of a zip it did not check:\n%s", stdout)
			}
		})
	}
}

func TestReportWithoutFiles(t *testing.T) {
	var stdout bytes.Buffer
	version := module.Version{Path: "example.com/m", Version: "v0.0.0"}

	exit := report(&stdout, version, nil, 22, testLimits)

	assertResult(t, exit, exitDidNotRun, stdout.String(),
		"zipcheck: DID NOT RUN: the zip of example.com/m@v0.0.0 holds no file,")
}

func TestCheck(t *testing.T) {
	license := zipFile{name: "LICENSE", size: 10, compressed: 8, text: true}
	lim := limits{
		fileCap: 100,
		exceptions: []exception{
			{path: "testdata/big.txt", maxBytes: 200, reason: "a fixture"},
		},
		budget: budget{baseline: 1000, measured: "2026-10-14", headroom: 500, reason: "fixtures"},
	}
	tests := []struct {
		name    string
		files   []zipFile
		zipSize int64
		want    []string
	}{
		{
			name: "every file at its cap and the zip at the budget",
			files: []zipFile{
				license,
				{name: "a.go", size: 100, text: true},
				{name: "testdata/big.txt", size: 200},
			},
			zipSize: 1500,
		},
		{
			name: "one byte over each cap and over the budget",
			files: []zipFile{
				license,
				{name: "a.go", size: 101, text: true},
				{name: "testdata/big.txt", size: 201},
			},
			zipSize: 1501,
			want: []string{
				"a.go: 101 bytes, over the per-file cap of 100 bytes; make it smaller, or add " +
					"an exception for it to limits.go with its own cap and the reason",
				"testdata/big.txt: 201 bytes, over the cap of 200 bytes that its exception in " +
					"limits.go allows; make it smaller, or raise the cap with the reason",
				"the zip has 1501 bytes compressed, over the budget of 1500: 1000 measured on " +
					"2026-10-14 plus 500 of headroom; make it smaller, or raise the budget in " +
					"limits.go with the reason",
			},
		},
		{
			name: "every rule broken by a file",
			files: []zipFile{
				{name: "a.png", size: 1, text: false},
				{name: "testdata/blob", size: 1, text: false},
			},
			zipSize: 1,
			want: []string{
				"a.png: an image; images stay out of the module zip, so link to them from the " +
					"docs",
				"a.png: not text, as it holds a NUL byte or is not valid UTF-8; only files in " +
					"testdata directories may be binary",
				"LICENSE: not in the zip; pkg.go.dev shows the documentation of a module only " +
					"when it detects a license it allows",
				"testdata/big.txt: not in the zip, but limits.go has an exception for it; " +
					"remove the exception",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := check(tt.files, tt.zipSize, lim)

			if !slices.Equal(got, tt.want) {
				t.Errorf("check returned\n\t%s\nwant\n\t%s", strings.Join(got, "\n\t"),
					strings.Join(tt.want, "\n\t"))
			}
		})
	}
}

func TestDisallowed(t *testing.T) {
	const (
		notAllowed = "not on the allow list of the module zip; CONTRIBUTING.md says what the " +
			"zip may hold"
		image    = "an image; images stay out of the module zip, so link to them from the docs"
		spec     = "an API specification; specifications stay out of the module zip, in tools/"
		outOfZip = " must stay out of the module zip; keep it out with a go.mod of its own, or " +
			"export-ignore in .gitattributes"
	)
	tests := map[string]string{
		"doc.go":                         "",
		"internal/x/x_test.go":           "",
		"openapi.go":                     "",
		"go.mod":                         "",
		"go.sum":                         "",
		"LICENSE":                        "",
		"README.md":                      "",
		"MIGRATION.md":                   "",
		"CHANGELOG.md":                   "",
		"llms.txt":                       "",
		"AGENTS.md":                      "",
		"CODE_OF_CONDUCT.md":             "",
		"CONTRIBUTING.md":                "",
		"SECURITY.md":                    "",
		"_typos.toml":                    "",
		".golangci.yml":                  "",
		".github/scripts/vuln.sh":        "",
		".github/CODEOWNERS":             "",
		"testdata/api-surface.txt":       "",
		"internal/x/testdata/reply.json": "",
		"testdata":                       notAllowed,
		"mytestdata/a.json":              notAllowed,
		"internal/go.sum":                notAllowed,
		"internal/LICENSE":               notAllowed,
		"internal/.hidden":               notAllowed,
		"docs/guide.md":                  notAllowed,
		"Makefile":                       notAllowed,
		"x.github/a.yml":                 notAllowed,
		".githooks/pre-commit":           notAllowed,
		".gitlab/ci.yml":                 notAllowed,
		"logo.png":                       image,
		"docs/banner.JPEG":               image,
		"testdata/diagram.svg":           image,
		"testdata/openapi.json":          spec,
		"testdata/Swagger.yaml":          spec,
		"testdata/specs/api.json":        spec,
		"specs/api.json":                 spec,
		"tools/main.go":                  "tools/" + outOfZip,
		"consumer/go.sum":                "consumer/" + outOfZip,
		"skills/a/SKILL.md":              "skills/" + outOfZip,
		"tools":                          notAllowed,
		"internal/tools/a.go":            "",
	}
	for _, ext := range []string{
		".avif", ".bmp", ".gif", ".heic", ".ico", ".jpeg", ".jpg", ".png", ".svg", ".tif", ".tiff",
		".webp",
	} {
		tests["testdata/a"+ext] = image
		tests["docs/a"+strings.ToUpper(ext)] = image
	}
	for name, want := range tests {
		if got := disallowed(name); got != want {
			t.Errorf("disallowed(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestValidate(t *testing.T) {
	valid := limits{
		fileCap: 1,
		exceptions: []exception{
			{path: "testdata/a.txt", maxBytes: 2, reason: "a fixture"},
			{path: "b.go", maxBytes: 3, reason: "a fixture"},
		},
		budget: budget{baseline: 1, measured: "2026-10-14", headroom: 0, reason: "fixtures"},
	}
	tests := []struct {
		name   string
		change func(l *limits)
		want   string
	}{
		{
			name:   "a per-file cap of zero",
			change: func(l *limits) { l.fileCap = 0 },
			want:   "the per-file cap is 0 bytes; it must be positive",
		},
		{
			name:   "a baseline of zero",
			change: func(l *limits) { l.budget.baseline = 0 },
			want: "the budget's baseline is 0 bytes and its headroom 0; the baseline must be " +
				"positive and the headroom not negative",
		},
		{
			name:   "a negative headroom",
			change: func(l *limits) { l.budget.headroom = -1 },
			want: "the budget's baseline is 1 bytes and its headroom -1; the baseline must be " +
				"positive and the headroom not negative",
		},
		{
			name:   "a day that does not exist",
			change: func(l *limits) { l.budget.measured = "2026-13-01" },
			want:   `the budget was measured on "2026-13-01", which is not a day written as`,
		},
		{
			name:   "a budget without a reason",
			change: func(l *limits) { l.budget.reason = " " },
			want:   "the budget has no reason",
		},
		{
			name:   "an exception for the module's root",
			change: func(l *limits) { l.exceptions[0].path = "." },
			want:   `the exception for "." does not name a file by its path in the module,`,
		},
		{
			name:   "an exception for a path that is not clean",
			change: func(l *limits) { l.exceptions[0].path = "testdata/../a.txt" },
			want:   `the exception for "testdata/../a.txt" does not name a file`,
		},
		{
			name:   "two exceptions for one file",
			change: func(l *limits) { l.exceptions[1].path = "testdata/a.txt" },
			want:   "two exceptions name testdata/a.txt; keep one",
		},
		{
			name:   "an exception that allows no more than the per-file cap",
			change: func(l *limits) { l.exceptions[0].maxBytes = 1 },
			want: "the exception for testdata/a.txt allows 1 bytes, which is not above the " +
				"per-file cap of 1; remove it",
		},
		{
			name:   "an exception without a reason",
			change: func(l *limits) { l.exceptions[1].reason = "" },
			want:   "the exception for b.go has no reason",
		},
	}
	if err := valid.validate(); err != nil {
		t.Fatalf("validate() of the valid limits = %v, want nil", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lim := valid
			lim.exceptions = slices.Clone(valid.exceptions)
			tt.change(&lim)

			err := lim.validate()

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("validate() = %v, want an error that holds %q", err, tt.want)
			}
		})
	}
}

func TestZipLimits(t *testing.T) {
	if err := zipLimits.validate(); err != nil {
		t.Errorf("zipLimits are not valid: %v", err)
	}
	if zipLimits.fileCap != 256<<10 {
		t.Errorf("the per-file cap is %d bytes, want 256 KiB", zipLimits.fileCap)
	}
}

func TestPrintLargest(t *testing.T) {
	var files []zipFile
	for i := range 11 {
		files = append(files, zipFile{
			name: fmt.Sprintf("f%02d.go", i), size: int64(i % 10), compressed: uint64(i),
		})
	}
	var stdout bytes.Buffer

	printLargest(&stdout, files)

	want := "zipcheck: the 10 largest files, in bytes uncompressed and compressed:\n" +
		"zipcheck:          9          9  f09.go\n" +
		"zipcheck:          8          8  f08.go\n" +
		"zipcheck:          7          7  f07.go\n" +
		"zipcheck:          6          6  f06.go\n" +
		"zipcheck:          5          5  f05.go\n" +
		"zipcheck:          4          4  f04.go\n" +
		"zipcheck:          3          3  f03.go\n" +
		"zipcheck:          2          2  f02.go\n" +
		"zipcheck:          1          1  f01.go\n" +
		"zipcheck:          0          0  f00.go\n"
	if stdout.String() != want {
		t.Errorf("printLargest printed\n%s\nwant\n%s", stdout.String(), want)
	}

	stdout.Reset()
	printLargest(&stdout, files[:2])

	want = "zipcheck: the 2 largest files, in bytes uncompressed and compressed:\n" +
		"zipcheck:          1          1  f01.go\n" +
		"zipcheck:          0          0  f00.go\n"
	if stdout.String() != want {
		t.Errorf("printLargest of two files printed\n%s\nwant\n%s", stdout.String(), want)
	}
}

// zipOf returns a zip that holds an empty file under each of the names, in their order.
func zipOf(t *testing.T, names ...string) []byte {
	t.Helper()
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	for _, name := range names {
		if _, err := w.Create(name); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestReadZip(t *testing.T) {
	version := module.Version{Path: "example.com/m", Version: "v0.0.0"}

	files, err := readZip(version, zipOf(t, "example.com/m@v0.0.0/b.go",
		"example.com/m@v0.0.0/a.go"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, f.name)
	}
	if want := []string{"a.go", "b.go"}; !slices.Equal(names, want) {
		t.Errorf("readZip() of a zip whose files are out of order returned %v, want %v", names,
			want)
	}

	_, err = readZip(version, zipOf(t, "example.com/m@v0.0.0/a.go", "other/b.go"))

	want := "the module zip holds other/b.go, which is outside example.com/m@v0.0.0/"
	if err == nil || err.Error() != want {
		t.Errorf("readZip() = %v, want %q", err, want)
	}

	_, err = readZip(version, []byte("not a zip"))

	if err == nil || !strings.HasPrefix(err.Error(), "reading the module zip: ") {
		t.Errorf("readZip() of bytes that are not a zip = %v, want a read error", err)
	}
}

func TestZipVersion(t *testing.T) {
	tests := map[string]string{
		"example.com/m":    "v0.0.0",
		"example.com/m/v2": "v2.0.0",
		"example.com/m/v3": "v3.0.0",
		"gopkg.in/yaml.v3": "v3.0.0",
	}
	for modulePath, want := range tests {
		if got := zipVersion(modulePath); got != want {
			t.Errorf("zipVersion(%q) = %q, want %q", modulePath, got, want)
		}
	}
}
