package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/permitio/permit-golang/v2/tools/internal/execfile"
)

// commandGit is the subcommand the gate puts first on gremlins' PATH as git.
//
// gremlins 0.6.0 reads which lines changed from git diff --merge-base <ref>, run
// with git's default context and the user's git config, and marks as changed the
// lines that follow each hunk's leading context, which misses the second of two
// edits that share a hunk. It also looks up the paths git prints, relative to the
// repository root, as paths relative to the directory it mutates. The gate passes
// gremlins --diff scopeRef, and git answers git diff --merge-base scopeRef with
// the diff the gate wrote: exactly the lines it chose, by file name in the package
// directory, with no context. It runs the real git for every other command, which
// tests may run.
const commandGit = "git"

// gitShimOptions is the parsed command line of the git subcommand.
type gitShimOptions struct {
	realGit  string
	logPath  string
	diffPath string
	args     []string
}

func runGitShim(args []string, stdout, stderr io.Writer) int {
	opts, err := parseGitShimArgs(args, stderr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "mutationgate git: %v\n", err)
		return exitFailed
	}
	if slices.Equal(opts.args, []string{"diff", "--merge-base", scopeRef}) {
		diff, readErr := os.ReadFile(opts.diffPath)
		if readErr == nil {
			_, readErr = stdout.Write(diff)
		}
		if readErr != nil {
			shimError(opts.logPath, commandGit, stderr,
				fmt.Sprintf("the diff in scope: %v", readErr))
			return exitFailed
		}
		return exitPassed
	}
	//nolint:gosec // G204: the git shim runs the git command the gate found, as a test asks.
	err = syscall.Exec(opts.realGit, append([]string{"git"}, opts.args...), os.Environ())
	shimError(opts.logPath, commandGit, stderr,
		fmt.Sprintf("cannot run %s: %v", opts.realGit, err))
	return exitFailed
}

func parseGitShimArgs(args []string, stderr io.Writer) (gitShimOptions, error) {
	fs := flag.NewFlagSet("mutationgate git", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var opts gitShimOptions
	fs.StringVar(&opts.realGit, "real-git", "", "the git command to run")
	fs.StringVar(&opts.logPath, "log", "", "file to append events to")
	fs.StringVar(&opts.diffPath, "diff", "", "the diff to answer gremlins' git diff with")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	opts.args = fs.Args()
	switch {
	case !filepath.IsAbs(opts.realGit):
		return opts, fmt.Errorf("-real-git %q: need an absolute path", opts.realGit)
	case !filepath.IsAbs(opts.diffPath):
		return opts, fmt.Errorf("-diff %q: need an absolute path", opts.diffPath)
	}
	return opts, nil
}

// writeGitShim writes dir/git, a script that runs the git subcommand of the
// executable self with the real git command realGit, the event log logPath and the
// diff diffPath. Each path must pass checkPlainPath.
func writeGitShim(dir, self, realGit, logPath, diffPath string) error {
	script := fmt.Sprintf("#!/bin/sh\nexec '%s' %s -real-git='%s' -log='%s' -diff='%s' -- \"$@\"\n",
		self, commandGit, realGit, logPath, diffPath)
	return execfile.WriteFile(filepath.Join(dir, commandGit), []byte(script), 0o700)
}
