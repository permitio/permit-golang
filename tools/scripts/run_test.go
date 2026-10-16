package scripts

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/tools/internal/execfile"
)

// scriptResult is a script's exit code and what it printed.
type scriptResult struct {
	exit           int
	stdout, stderr string
}

// runScript runs the program at path with args in dir, in the test's environment plus env. A
// process that keeps the output open after the program exits is cut off after WaitDelay instead of
// hanging the test; the call then fails the test when the program exited 0.
func runScript(t *testing.T, dir string, env []string, path string, args ...string) scriptResult {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), path, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.WaitDelay = 30 * time.Second
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	result := scriptResult{stdout: stdout.String(), stderr: stderr.String()}
	if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) {
		result.exit = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("%s %s: %v", path, strings.Join(args, " "), err)
	}
	return result
}

// repositoryFile returns the path of a file in the repository, given relative to its root.
func repositoryFile(name string) string {
	return filepath.Join("..", "..", filepath.FromSlash(name))
}

// linkScripts links the named scripts of .github/scripts into the same place under root. A script
// finds the repository root from the path it runs by, so run by its link it takes root for the
// repository root. Unlike a copy, a link is not a new executable: some systems, such as macOS,
// check each new executable the first time it runs, one at a time, which makes the tests that run
// in parallel wait for each other. Stubs that many subtests run are linked from one file per test
// for the same reason.
func linkScripts(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		source, err := filepath.Abs(repositoryFile(".github/scripts/" + name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(source); err != nil {
			t.Fatal(err)
		}
		linkFile(t, root, ".github/scripts/"+name, source)
	}
}

// writeStub writes content to an executable file named name in a new directory, which the tests'
// repositories link to, and returns its path.
func writeStub(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	writeExecutable(t, dir, name, content)
	return filepath.Join(dir, name)
}

// linkFile links name, a path relative to root, to target, and makes the directory it is in.
func linkFile(t *testing.T, root, name, target string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

// writeExecutable writes content to an executable file at name, a path relative to root.
func writeExecutable(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := execfile.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

// writeFiles writes each file in files, a map from a path relative to root to its content.
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// gitEnv returns environment variables that keep git from reading the configuration of the
// machine that runs the test, from running automatic maintenance or gc, and from finding a
// repository above root.
func gitEnv(root string) []string {
	return []string{
		"GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=maintenance.auto", "GIT_CONFIG_VALUE_0=false",
		"GIT_CONFIG_KEY_1=gc.auto", "GIT_CONFIG_VALUE_1=0",
		"GIT_CEILING_DIRECTORIES=" + filepath.Dir(root),
	}
}

// A git commit starts git maintenance, which can run git gc, in a background process that outlives
// the commit; gitEnv turns both off.
// TestScriptsFindTheirRootByLink checks that no script of .github/scripts resolves links when it
// finds its own directory: the tests run each script through a link from a temporary root, as
// linkScripts says, so a script that resolved the link would act on the repository itself.
func TestScriptsFindTheirRootByLink(t *testing.T) {
	t.Parallel()
	scripts, err := filepath.Glob(repositoryFile(".github/scripts/*.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if len(scripts) == 0 {
		t.Fatal("found no script in .github/scripts")
	}
	resolvers := []string{"pwd -P", "cd -P", "realpath", "readlink"}
	for _, script := range scripts {
		content, err := os.ReadFile(script)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(content), "\n") {
			if !strings.Contains(line, "BASH_SOURCE") {
				continue
			}
			for _, resolver := range resolvers {
				if strings.Contains(line, resolver) {
					t.Errorf("%s:%d: finds its directory with %s, which resolves the link "+
						"that the tests run it by", filepath.Base(script), i+1, resolver)
				}
			}
		}
	}
}

func TestGitEnvTurnsOffMaintenance(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for key, want := range map[string]string{"maintenance.auto": "false", "gc.auto": "0"} {
		result := runScript(t, root, gitEnv(root), "git", "config", "--get", key)
		if got := strings.TrimSpace(result.stdout); result.exit != 0 || got != want {
			t.Errorf("git config --get %s: exit %d, stdout %q, stderr %q; want exit 0, stdout %q",
				key, result.exit, got, result.stderr, want)
		}
	}
}

// gitInit makes root a git repository that tracks every file in it.
func gitInit(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "--all"}} {
		result := runScript(t, root, gitEnv(root), "git", args...)
		if result.exit != 0 {
			t.Fatalf("git %s exited %d: %s", strings.Join(args, " "), result.exit,
				result.stderr)
		}
	}
}
