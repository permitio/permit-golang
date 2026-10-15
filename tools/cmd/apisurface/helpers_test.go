package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
	t.Parallel()
	dir := t.TempDir()
	for key, want := range map[string]string{"maintenance.auto": "false", "gc.auto": "0"} {
		if got := gitIn(t, dir, "config", "--get", key); got != want {
			t.Errorf("git config --get %s = %q, want %q", key, got, want)
		}
	}
}

// writeFiles writes files, a map from a slash path to its content, under dir. An empty content
// removes the file.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		file := filepath.Join(dir, filepath.FromSlash(name))
		if content == "" {
			if err := os.Remove(file); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// newRepo creates a git repository whose working tree holds files, commits them and returns its
// directory.
func newRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init", "--quiet", "--initial-branch=main")
	writeFiles(t, dir, files)
	commitAll(t, dir, "base")
	return dir
}

// commitAll commits every file in the working tree of the repository in dir and returns the
// commit's SHA.
func commitAll(t *testing.T, dir, message string) string {
	t.Helper()
	gitIn(t, dir, "add", "--all")
	gitIn(t, dir, "commit", "--quiet", "--allow-empty", "--message", message)
	return gitIn(t, dir, "rev-parse", "HEAD")
}

// runTool runs apisurface with args and sys, and returns its exit code and what it printed.
func runTool(t *testing.T, sys system, args ...string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(t.Context(), args, &stdout, &stderr, sys)
	return code, stdout.String() + stderr.String()
}

// wantRun checks the exit code of a run and that its output holds each of want.
func wantRun(t *testing.T, code int, out string, wantCode int, want ...string) {
	t.Helper()
	if code != wantCode {
		t.Errorf("exit code %d, want %d; output:\n%s", code, wantCode, out)
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("output does not contain %q:\n%s", w, out)
		}
	}
}

// lastLine returns the last line of out.
func lastLine(out string) string {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	return lines[len(lines)-1]
}

// buildTool builds a Go command of the tools module, such as golang.org/x/exp/cmd/apidiff, and
// returns its path. The module cache must hold its modules, as go -C tools mod download leaves
// it, since the tests run offline.
func buildTool(t *testing.T, pkg string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), filepath.Base(pkg))
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", bin, pkg)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v: %s", pkg, err, out)
	}
	return bin
}
