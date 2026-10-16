package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// diffFixture is git diff -U0 output: an edit and an addition in a.go, a deletion,
// a test file, a deleted file, a non-Go file, a new file, and a removed line that
// reads like a file header.
const diffFixture = `diff --git a/a.go b/a.go
index 1111111..2222222 100644
--- a/a.go
+++ b/a.go
@@ -3 +3 @@ func a() {
-	x := 1
+	x := 2
@@ -10,0 +11,2 @@ func b() {
+	y := 3
+	z := 4
@@ -20,2 +21,0 @@ func c() {
--- looks like a header
-	gone
diff --git a/a_test.go b/a_test.go
--- a/a_test.go
+++ b/a_test.go
@@ -1 +1 @@
-x
+y
diff --git a/old.go b/old.go
deleted file mode 100644
--- a/old.go
+++ /dev/null
@@ -1,2 +0,0 @@
-package x
-
diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -1 +1 @@
-a
+b
diff --git a/pkg/new.go b/pkg/new.go
new file mode 100644
--- /dev/null
+++ b/pkg/new.go
@@ -0,0 +1,3 @@
+package pkg
+
+func n() {}
\ No newline at end of file
`

func TestParseAddedLines(t *testing.T) {
	got, err := parseAddedLines([]byte(diffFixture))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]lineRange{
		"a.go":       {{3, 3}, {11, 12}},
		"pkg/new.go": {{1, 3}},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for file, ranges := range want {
		if !slices.Equal(got[file], ranges) {
			t.Errorf("%s: got %v, want %v", file, got[file], ranges)
		}
	}
}

func TestParseAddedLinesErrors(t *testing.T) {
	tests := map[string]string{
		"quoted file name": "--- a/x.go\n+++ \"b/sp\\tace.go\"\n",
		"bad hunk header":  "--- a/x.go\n+++ b/x.go\n@@ bad @@\n",
		"hunk cut short":   "--- a/x.go\n+++ b/x.go\n@@ -1 +1,2 @@\n-a\n+b\n",
		"hunk ends early":  "--- a/x.go\n+++ b/x.go\n@@ -1 +1,2 @@\n-a\n+b\ndiff --git a/y b/y\n",
	}
	for name, diff := range tests {
		t.Run(name, func(t *testing.T) {
			if got, err := parseAddedLines([]byte(diff)); err == nil {
				t.Fatalf("got %v and no error", got)
			}
		})
	}
}

func TestParseGateArgs(t *testing.T) {
	good := [][]string{
		{"-base", "origin/master", "."},
		{"-all", "-workers", "1", "-timeout-coefficient", "1", "-threshold", "0", "tools"},
		{
			"-base", "HEAD", "-timeout-coefficient", "5", "-threshold", "100", "-memory-mib", "512",
			".", "tools",
		},
		{"-base", "HEAD", "-skip", "", "-tags", "e2e,integration", "-max-mutants", "400", "."},
		{"-base", "HEAD", "-skip", "^(TestLive|TestSlow)", "."},
	}
	for _, args := range good {
		if _, err := parseGateArgs(args, &bytes.Buffer{}); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
	bad := map[string][]string{
		"no directory":            {"-base", "main"},
		"no scope":                {"."},
		"both scopes":             {"-base", "main", "-all", "."},
		"no workers":              {"-base", "main", "-workers", "0", "."},
		"coefficient 6":           {"-base", "main", "-timeout-coefficient", "6", "."},
		"coefficient 0":           {"-base", "main", "-timeout-coefficient", "0", "."},
		"no cap":                  {"-base", "main", "-cap", "0s", "."},
		"no memory":               {"-base", "main", "-memory-mib", "0", "."},
		"threshold over 100":      {"-base", "main", "-threshold", "100.5", "."},
		"negative threshold":      {"-base", "main", "-threshold", "-1", "."},
		"negative budget":         {"-base", "main", "-max-mutants", "-1", "."},
		"skip with a space":       {"-base", "main", "-skip", "^Test Live", "."},
		"skip with a quote":       {"-base", "main", "-skip", "^Test'", "."},
		"skip that is no pattern": {"-base", "main", "-skip", "^(Test", "."},
		"tags with a space":       {"-base", "main", "-tags", "e2e integration", "."},
		"unknown flag":            {"-base", "main", "-nope", "."},
	}
	for name, args := range bad {
		if _, err := parseGateArgs(args, &bytes.Buffer{}); err == nil {
			t.Errorf("%s: %v: no error", name, args)
		}
	}
}

func TestParseGateArgsDefaults(t *testing.T) {
	t.Setenv("GITHUB_STEP_SUMMARY", "/tmp/summary.md")
	opts, err := parseGateArgs([]string{"-base", "main", ".", "tools"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if opts.workers != 2 || opts.coefficient != 5 || opts.cap != 3*time.Minute ||
		opts.memoryMiB != 4096 || opts.threshold != 90 || opts.skip != "^TestLive" ||
		opts.tags != "" || opts.maxMutants != 0 || opts.summary != "" ||
		opts.gremlins != "gremlins" {
		t.Errorf("defaults: %+v", opts)
	}
	if !slices.Equal(opts.dirs, []string{".", "tools"}) {
		t.Errorf("directories %q", opts.dirs)
	}
}

func TestGremlinsEnv(t *testing.T) {
	environ := []string{
		"HOME=/home/u", "PATH=/bin", "GOFLAGS=-mod=mod", "GREMLINS_UNLEASH_WORKERS=16",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=a.b",
	}
	opts := gateOptions{workers: 2, cap: defaultCap, memoryMiB: 1536, skip: defaultSkip}
	w := workFiles{events: "/tmp/w/events.log", bin: "/tmp/w/bin"}
	got := gremlinsEnv(environ, "/opt/mutationgate", opts, w)
	want := []string{
		"HOME=/home/u", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=a.b",
		"PATH=/tmp/w/bin" + string(os.PathListSeparator) + "/bin",
		"GOFLAGS=-mod=mod -count=1 -skip=^TestLive -p=2 " +
			"'-exec=/opt/mutationgate exec -cap=3m0s -memory-mib=1536 -log=/tmp/w/events.log'",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}

	got = gremlinsEnv(nil, "/opt/mutationgate", gateOptions{
		workers: 1, cap: time.Minute,
		memoryMiB: 64,
	}, w)
	want = []string{"PATH=/tmp/w/bin", "GOFLAGS=-count=1 -p=1 " +
		"'-exec=/opt/mutationgate exec -cap=1m0s -memory-mib=64 -log=/tmp/w/events.log'"}
	if !slices.Equal(got, want) {
		t.Errorf("an empty environment and no -skip: got\n%q\nwant\n%q", got, want)
	}
}

func TestBuildGremlinsArgs(t *testing.T) {
	w := workFiles{results: "/w/results.json", config: "/w/gremlins.yaml"}
	opts := gateOptions{workers: 2, coefficient: 5}
	got := strings.Join(buildGremlinsArgs(opts, false, w, "/repo/tools/cmd/x",
		[]string{"/", `^x_linux\.go$`}), " ")
	want := "unleash --config /w/gremlins.yaml --workers 2 --timeout-coefficient 5 " +
		"--output /w/results.json --diff mutationgate-scope --exclude-files / " +
		`--exclude-files ^x_linux\.go$ /repo/tools/cmd/x`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	opts.tags = "e2e"
	got = strings.Join(buildGremlinsArgs(opts, true, w, "/repo", []string{"/"}), " ")
	want = "unleash --config /w/gremlins.yaml --workers 2 --timeout-coefficient 5 " +
		"--output /w/results.json --diff mutationgate-scope --dry-run --tags e2e " +
		"--exclude-files / /repo"
	if got != want {
		t.Errorf("a dry run with tags: got  %s\nwant %s", got, want)
	}
}

func TestIsolateGit(t *testing.T) {
	environ := []string{
		"HOME=/home/u", "GIT_EXTERNAL_DIFF=true", "GIT_CONFIG_GLOBAL=/home/u/g",
		"GIT_DIR=/r/.git", "GIT_CONFIG_COUNT=1",
	}
	got := isolateGit(environ)
	want := []string{
		"HOME=/home/u", "GIT_DIR=/r/.git", "GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// systemTools finds this executable and the go and git commands as absolute paths,
// and fails when a command is not on PATH.
func TestSystemTools(t *testing.T) {
	tl, err := systemTools()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{tl.self, tl.goPath, tl.gitPath} {
		if !filepath.IsAbs(p) {
			t.Errorf("%q is not an absolute path", p)
		}
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := systemTools(); err == nil || !strings.Contains(err.Error(), "cannot find the ") {
		t.Errorf("no go or git on PATH: got %v", err)
	}
}

// A run that does not complete says so on stdout, in the summary file and on stderr,
// and reports a summary file it cannot write.
func TestSinkDidNotRun(t *testing.T) {
	summary := filepath.Join(t.TempDir(), "summary.md")
	var stdout, stderr bytes.Buffer
	exit := sink{stdout: &stdout, stderr: &stderr, summary: summary}.didNotRun(
		errors.New("the probe failed"))
	written, err := os.ReadFile(summary)
	if exit != exitDidNotRun || stdout.String() != "mutationgate: DID NOT RUN\n" ||
		stderr.String() != "mutationgate: the probe failed\n" || err != nil ||
		string(written) != "### Mutation tests\n\n**DID NOT RUN**: the probe failed\n\n" {
		t.Errorf("exit %d, stdout %q, stderr %q, summary %q, %v", exit, stdout.String(),
			stderr.String(), written, err)
	}
	stderr.Reset()
	sink{stdout: &stdout, stderr: &stderr, summary: t.TempDir()}.didNotRun(errors.New("x"))
	if !strings.Contains(stderr.String(), "mutationgate: job summary: ") {
		t.Errorf("a summary that cannot be written: stderr %q", stderr.String())
	}
}

func TestRemoveWorkDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "work")
	if err := os.MkdirAll(filepath.Join(dir, "1", "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	removeWorkDir(dir, &stderr)
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) || stderr.Len() != 0 {
		t.Errorf("stat %v, stderr %q", err, stderr.String())
	}
}

func TestCheckPlainPath(t *testing.T) {
	if err := checkPlainPath("gate", "/usr/local/bin/mutationgate"); err != nil {
		t.Error(err)
	}
	for _, p := range []string{"", "/my dir/mutationgate", "/it's/mutationgate", `C:\gate`} {
		if checkPlainPath("gate", p) == nil {
			t.Errorf("%q: no error", p)
		}
	}
}
