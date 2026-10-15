//go:build unix

package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// scratchModule writes a module with two packages: greet, whose Greeting has body,
// and the program cmd/hello, whose Hello has body too. Each has a test that
// asserts what its function returns.
func scratchModule(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	test := func(pkg, fn string) string {
		return "package " + pkg + "\n\nimport \"testing\"\n\n" +
			"func Test" + fn + "(t *testing.T) {\n" +
			"\tif got := " + fn + "(\"x\"); got != \"hello, x!\" {\n\t\tt.Fatal(got)\n\t}\n}\n"
	}
	files := map[string]string{
		"go.mod": "module example.com/m\n\ngo 1.21\n",
		"greet/greet.go": "package greet\n\n" +
			"func Greeting(name string) string { " + body + " }\n",
		"greet/greet_test.go": test("greet", "Greeting"),
		"cmd/hello/main.go": "package main\n\nfunc main() { println(Hello(\"x\")) }\n\n" +
			"func Hello(name string) string { " + body + " }\n",
		"cmd/hello/main_test.go": test("main", "Hello"),
	}
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// shimResult is a shim's exit code, its output and the event log.
type shimResult struct {
	exit        int
	output, log string
}

// runShim runs the shim named command that the gate writes, with this test binary
// as mutationgate, in dir with args, as gremlins does through its PATH.
func runShim(t *testing.T, dir, command string, write func(bin, self, logPath string) error,
	args ...string,
) shimResult {
	t.Helper()
	self := testExecutable(t)
	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "events.log")
	for what, p := range map[string]string{"test binary": self, "log": logPath} {
		if err := checkPlainPath(what, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := write(bin, self, logPath); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), filepath.Join(bin, command), args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), testMainEnv+"=main", "GOFLAGS=", "GOTOOLCHAIN=local",
		"GOWORK=off", "GOPROXY=off")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatal(err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return shimResult{cmd.ProcessState.ExitCode(), out.String(), string(log)}
}

func lookPath(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err == nil {
		p, err = filepath.Abs(p)
	}
	if err != nil {
		t.Fatalf("the shim tests need %s on PATH: %v", name, err)
	}
	return p
}

// goShimRun runs the go shim for the package pkg, which it builds with the module
// file modFile.
func goShimRun(t *testing.T, dir, modFile, pkg string, args ...string) shimResult {
	t.Helper()
	realGo := lookPath(t, "go")
	return runShim(t, dir, commandGo, func(bin, self, logPath string) error {
		return writeGoShim(bin, self, goShimOptions{
			realGo: realGo, logPath: logPath, modFile: modFile, pkg: pkg,
		})
	}, args...)
}

// moduleFileOf writes the module file the gate builds the module whose root is dir
// with, and returns its path.
func moduleFileOf(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "go.mod")
	if err := writeModuleFile(dir, p); err != nil {
		t.Fatal(err)
	}
	return p
}

// mutantRunArgs is gremlins' go test of a mutant: a package in a subdirectory
// named after the module, as gremlins names it for package main.
var mutantRunArgs = []string{"test", "-timeout", "1m", "-failfast", "example.com/m"}

func TestGoShimTestsAMutantThatCompiles(t *testing.T) {
	t.Parallel()
	for _, pkg := range []string{"example.com/m/greet", "example.com/m/cmd/hello"} {
		t.Run(pkg, func(t *testing.T) {
			t.Parallel()
			dir := scratchModule(t, `return "hello, " + name + "!"`)
			r := goShimRun(t, dir, moduleFileOf(t, dir), pkg, mutantRunArgs...)
			if r.exit != 0 || !strings.Contains(r.output, "ok  \t"+pkg) || r.log != "" {
				t.Errorf("exit %d, log %q, output:\n%s", r.exit, r.log, r.output)
			}
			dir = scratchModule(t, `return "hello, " + name + "?"`)
			r = goShimRun(t, dir, moduleFileOf(t, dir), pkg, mutantRunArgs...)
			if r.exit != 1 || !strings.Contains(r.output, "FAIL\t"+pkg) || r.log != "" {
				t.Errorf("a mutant the test detects: exit %d, log %q, output:\n%s", r.exit,
					r.log, r.output)
			}
		})
	}
}

// gremlins tests a mutant in a copy of its module, where the inner module's
// replacement of the outer one with ../ points nowhere. The shim builds the mutant
// with the module file the gate wrote, so its tests run against the outer module
// and the mutant is killed or lives, and never counts as not viable.
func TestGoShimTestsMutantsOfAModuleThatReplacesItsParent(t *testing.T) {
	t.Parallel()
	inner := replacingModules(t)
	r := runner{dir: t.TempDir()}
	modFile, err := r.moduleFile(inner)
	if err != nil {
		t.Fatal(err)
	}
	pkg := inner.pkgs[0].importPath
	for _, tt := range []struct {
		name, from, to string
		wantExit       int
		wantOutput     string
	}{
		{"killed", `name == ""`, `name != ""`, 1, "FAIL\t" + pkg},
		{"lived", `"world"`, `"there"`, 0, "ok  \t" + pkg},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			copyRoot := filepath.Join(t.TempDir(), "wd")
			if err := copyModule(inner.dir, copyRoot); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(copyRoot, "greet", "greet.go")
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			mutant := strings.Replace(string(data), tt.from, tt.to, 1)
			if mutant == string(data) {
				t.Fatalf("greet.go holds no %s", tt.from)
			}
			writeFile(t, copyRoot, "greet/greet.go", mutant)
			got := goShimRun(t, filepath.Join(copyRoot, "greet"), modFile, pkg,
				"test", "-timeout", "1m", "-failfast", pkg)
			if got.exit != tt.wantExit || !strings.Contains(got.output, tt.wantOutput) ||
				got.log != "" {
				t.Errorf("exit %d, want %d; log %q, output:\n%s", got.exit, tt.wantExit, got.log,
					got.output)
			}
		})
	}
}

// go test exits 1 when a mutant does not compile, which gremlins would count as
// killed; the shim must exit 2, which gremlins counts as not viable.
func TestGoShimReportsANonCompilingMutant(t *testing.T) {
	t.Parallel()
	dir := scratchModule(t, `return "hello, " - name + "!"`)
	r := goShimRun(t, dir, moduleFileOf(t, dir), "example.com/m/cmd/hello", mutantRunArgs...)
	if r.exit != exitNotViable ||
		!strings.Contains(r.output, "does not compile with this mutant") ||
		r.log != "" {
		t.Errorf("exit %d, log %q, output:\n%s", r.exit, r.log, r.output)
	}
}

func TestGoShimFailsWhenThePackageCannotLoad(t *testing.T) {
	t.Parallel()
	dir := scratchModule(t, `return "hello, " + name + "!"`)
	r := goShimRun(t, dir, moduleFileOf(t, dir), "example.com/m/missing", mutantRunArgs...)
	want := "error\tgo\t" +
		"go build example.com/m/missing failed, and go list cannot load the package"
	if r.exit != exitFailed || !strings.HasPrefix(r.log, want) {
		t.Errorf("exit %d, log %q, output:\n%s", r.exit, r.log, r.output)
	}
}

// Other commands, which tests may run, get no -modfile: the module file here does
// not exist, which a command given it would fail on.
func TestGoShimRunsOtherCommandsAsIs(t *testing.T) {
	t.Parallel()
	dir := scratchModule(t, `return "hello, " - name + "!"`)
	missing := filepath.Join(t.TempDir(), "missing.mod")
	r := goShimRun(t, dir, missing, "example.com/m/greet", "env", "GOOS")
	if r.exit != 0 || strings.TrimSpace(r.output) != runtime.GOOS || r.log != "" {
		t.Errorf("exit %d, log %q, output:\n%s", r.exit, r.log, r.output)
	}
	r = goShimRun(t, dir, missing, "example.com/m/greet", "test", "-count=1", "./...")
	if r.exit != 1 || !strings.Contains(r.output, "[build failed]") {
		t.Errorf("a go test that is not a mutant run: exit %d, output:\n%s", r.exit, r.output)
	}
}

// gitShimRun runs the git shim with a diff file that holds diff.
func gitShimRun(t *testing.T, dir, diff string, args ...string) shimResult {
	t.Helper()
	realGit := lookPath(t, "git")
	diffPath := filepath.Join(t.TempDir(), "scope.diff")
	if diff != "" {
		if err := os.WriteFile(diffPath, []byte(diff), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return runShim(t, dir, commandGit, func(bin, self, logPath string) error {
		return writeGitShim(bin, self, realGit, logPath, diffPath)
	}, args...)
}

// The git shim answers gremlins' git diff with the diff in scope, and runs git for
// every other command.
func TestGitShim(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	diff := string(scopeDiff(map[string][]lineRange{"main.go": {{3, 4}}}))
	r := gitShimRun(t, dir, diff, "diff", "--merge-base", scopeRef)
	if r.exit != 0 || r.output != diff || r.log != "" {
		t.Errorf("gremlins' git diff: exit %d, log %q, output:\n%s", r.exit, r.log, r.output)
	}
	r = gitShimRun(t, dir, diff, "--version")
	if r.exit != 0 || !strings.HasPrefix(r.output, "git version ") || r.log != "" {
		t.Errorf("git --version: exit %d, log %q, output:\n%s", r.exit, r.log, r.output)
	}
	r = gitShimRun(t, dir, diff, "diff", "--merge-base", "main")
	if r.exit == 0 || strings.Contains(r.output, "main.go") {
		t.Errorf("a git diff for another ref: exit %d, output:\n%s", r.exit, r.output)
	}
	r = gitShimRun(t, dir, "", "diff", "--merge-base", scopeRef)
	want := "error\tgit\tthe diff in scope: open "
	if r.exit != exitFailed || !strings.HasPrefix(r.log, want) {
		t.Errorf("no diff file: exit %d, log %q, output:\n%s", r.exit, r.log, r.output)
	}
}
