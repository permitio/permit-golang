package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/permitio/permit-golang/v2/tools/internal/execfile"
)

// commandGo is the subcommand the gate puts first on gremlins' PATH as go.
//
// gremlins 0.6.0 counts a mutant as not viable only when go test exits 2, but go
// test exits 1 when the build fails, so a mutant that does not compile, such as +
// turned into - between two strings, would count as killed. For a gremlins mutant
// run, go first builds the mutated package and exits 2 when it does not compile.
// It then replaces itself with the real go command, as it does for every other
// command, so gremlins' timeout still kills the process it started. A mutant that
// breaks only the package's test files still counts as killed.
//
// gremlins names the package to test after the nearest directory that ends with
// the file's package name, or the module root, which is wrong for package main in
// a subdirectory. gremlins runs in one package's directory at a time, so go tests
// the package the gate names with -package instead.
//
// gremlins tests a mutant in a copy of the module that holds nothing above its
// root, where a replacement by a relative directory, such as the tools module's
// replacement of the SDK with ../, points nowhere, so a package that imports the
// replaced module would not build and every mutant would count as not viable. go
// builds, lists and tests a mutant with -modfile, the module file the gate wrote
// with those directories made absolute. Every other command, which tests may run,
// gets no -modfile.
const commandGo = "go"

// exitNotViable is the go test exit code gremlins reads as a mutant that does not
// compile.
const exitNotViable = 2

// goShimOptions is the parsed command line of the go subcommand.
type goShimOptions struct {
	realGo  string
	logPath string
	// modFile is the module file to build a mutant with, by absolute path.
	modFile string
	pkg     string
	args    []string
}

func runGoShim(args []string, stderr io.Writer) int {
	opts, err := parseGoShimArgs(args, stderr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "mutationgate go: %v\n", err)
		return exitFailed
	}
	if buildFlags, ok := mutantTestRun(opts.args); ok {
		modFile := "-modfile=" + opts.modFile
		opts.args[len(opts.args)-1] = opts.pkg
		opts.args = slices.Insert(opts.args, 1, modFile)
		if code, done := checkMutantBuilds(opts, append(buildFlags, modFile), stderr); done {
			return code
		}
	}
	//nolint:gosec // G204: the go shim runs the go command the gate found, as gremlins asks.
	err = syscall.Exec(opts.realGo, append([]string{"go"}, opts.args...), os.Environ())
	shimError(opts.logPath, commandGo, stderr, fmt.Sprintf("cannot run %s: %v", opts.realGo, err))
	return exitFailed
}

func parseGoShimArgs(args []string, stderr io.Writer) (goShimOptions, error) {
	fs := flag.NewFlagSet("mutationgate go", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var opts goShimOptions
	fs.StringVar(&opts.realGo, "real-go", "", "the go command to run")
	fs.StringVar(&opts.logPath, "log", "", "file to append events to")
	fs.StringVar(&opts.modFile, "modfile", "", "the module file to build a mutant with")
	fs.StringVar(&opts.pkg, "package", "", "the import path of the package a mutant is in")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	opts.args = fs.Args()
	switch {
	case !filepath.IsAbs(opts.realGo):
		return opts, fmt.Errorf("-real-go %q: need an absolute path", opts.realGo)
	case !filepath.IsAbs(opts.modFile):
		return opts, fmt.Errorf("-modfile %q: need an absolute path", opts.modFile)
	case opts.pkg == "" || strings.HasPrefix(opts.pkg, "-"):
		return opts, fmt.Errorf("-package %q: need an import path", opts.pkg)
	}
	return opts, nil
}

// mutantTestRun reports whether args are gremlins 0.6.0's go test of one mutant,
// "test [-tags T] -timeout D -failfast [-cpu N] package", and returns the flags to
// build the package with.
func mutantTestRun(args []string) ([]string, bool) {
	if len(args) < 2 || args[0] != "test" || !slices.Contains(args, "-failfast") {
		return nil, false
	}
	pkg := args[len(args)-1]
	if strings.HasPrefix(pkg, "-") || strings.HasSuffix(pkg, "...") {
		return nil, false
	}
	var buildFlags []string
	if i := slices.Index(args, "-tags"); i > 0 && i+1 < len(args)-1 {
		buildFlags = []string{"-tags", args[i+1]}
	}
	return buildFlags, true
}

// checkMutantBuilds builds the mutated package. When it does not compile, it
// returns exitNotViable and done. A build that fails for another reason, such as a
// package go list cannot load, is logged as an error, which fails the gate.
func checkMutantBuilds(opts goShimOptions, buildFlags []string, stderr io.Writer) (int, bool) {
	//nolint:gosec // G204: the go shim builds the package the gate names.
	build := exec.CommandContext(context.Background(), opts.realGo,
		slices.Concat([]string{"build", "-o", os.DevNull}, buildFlags, []string{opts.pkg})...)
	build.Stdout = stderr
	build.Stderr = stderr
	err := build.Run()
	if err == nil {
		return 0, false
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		shimError(opts.logPath, commandGo, stderr, fmt.Sprintf("go build %s: %v", opts.pkg, err))
		return exitFailed, true
	}
	//nolint:gosec // G204: the go shim lists the package the gate names.
	list := exec.CommandContext(context.Background(), opts.realGo,
		slices.Concat([]string{"list"}, buildFlags, []string{opts.pkg})...)
	list.Stdout = io.Discard
	list.Stderr = stderr
	if err := list.Run(); err != nil {
		shimError(opts.logPath, commandGo, stderr,
			fmt.Sprintf("go build %s failed, and go list cannot load the package: %v", opts.pkg,
				err))
		return exitFailed, true
	}
	_, _ = fmt.Fprintf(stderr, "mutationgate go: %s does not compile with this mutant\n", opts.pkg)
	return exitNotViable, true
}

// shimError prints an error of the go or git subcommand and logs it, which fails
// the gate.
func shimError(logPath, command string, stderr io.Writer, detail string) {
	_, _ = fmt.Fprintf(stderr, "mutationgate %s: %s\n", command, detail)
	logEvent(logPath, stderr, event{kind: eventError, binary: command, detail: detail})
}

// writeGoShim writes dir/go, a script that runs the go subcommand of the
// executable self with the real go command, the event log, the module file and the
// package of opts. self, each path and the package must pass checkPlainPath.
func writeGoShim(dir, self string, opts goShimOptions) error {
	script := fmt.Sprintf(
		"#!/bin/sh\nexec '%s' %s -real-go='%s' -log='%s' -modfile='%s' -package='%s' -- \"$@\"\n",
		self,
		commandGo,
		opts.realGo,
		opts.logPath,
		opts.modFile,
		opts.pkg,
	)
	return execfile.WriteFile(filepath.Join(dir, commandGo), []byte(script), 0o700)
}
