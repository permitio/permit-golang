// Command mutationgate runs gremlins mutation testing on the Go lines a change adds
// or edits, or on every Go file, of the packages in the directories it is given,
// with every test run it starts bounded in time, memory and output, and fails when
// the tests detect too few of the mutants.
//
// It is copied from the mutation gate of the Permit.io Terraform provider
// (internal/acctest/mutationgate at commit 379892f) and adapted to a repository
// with more than one module, whose programs are package main in subdirectories.
//
// gremlins alone cannot serve as the gate, so mutationgate wraps it:
//
//   - gremlins bounds a test run only by a timeout, the coverage run's time times
//     a coefficient (-timeout-coefficient, 1 to 5), and on that timeout kills the
//     go command but not the test process it started. mutationgate runs every
//     test process through its own exec subcommand (go test -exec, set in
//     GOFLAGS), which stops the process when it runs longer than -cap, uses more
//     than -memory-mib, writes more than 64 MiB of output, or outlives the go
//     command. On Linux the kernel enforces the memory limit (RLIMIT_AS, as
//     ulimit -v does); macOS does not, so there exec polls the process's physical
//     footprint every 100ms. Before gremlins starts, a probe process that
//     allocates without end must be stopped by the limit, or the run does not
//     start. GOFLAGS also sets go test -p to -workers, so gremlins' coverage run,
//     which tests the packages without a mutant, runs at most that many test
//     processes at a time too. A test process of that run may take half of -cap,
//     so that a mutant of its package has room before the cap counts it as killed.
//   - gremlins counts a mutant as not viable only when go test exits 2, but go test
//     exits 1 when the build fails, so a mutant that does not compile would count
//     as killed. mutationgate puts its go subcommand first on gremlins' PATH, which
//     builds a mutated package before testing it and exits 2 when it does not
//     compile. Mutants that do not compile are not counted.
//   - gremlins counts a timed-out mutant as neither killed nor lived, and its
//     efficacy threshold fails a result equal to it. mutationgate counts killed and
//     timed-out mutants as detected, and fails when detected / (detected + lived)
//     is below -threshold.
//   - gremlins exits 0 when it finds no mutant at all. With -base, mutationgate
//     does not start gremlins when no Go line changed, passes with a notice then or
//     when the changed lines hold nothing to mutate or only mutants that do not
//     compile, and otherwise fails with exit code 2 when no mutant ran. With -all,
//     a module that holds nothing to mutate passes with a notice, one whose
//     mutants are all on lines no test covers or do not compile fails with exit
//     code 2, and so does a run in which no mutant ran in any module.
//   - gremlins 0.6.0 tests a mutant with the package named by the nearest
//     directory that ends with the file's package name, or the module root: the
//     wrong package for package main in a subdirectory, such as every program in
//     tools/cmd, or for a package whose name differs from its directory.
//     mutationgate runs gremlins once per package, on the package's directory with
//     its subdirectories left out, and its go subcommand tests that package.
//   - gremlins reads which lines changed from git diff --merge-base, with git's
//     default context and the user's git config, and marks as changed the lines
//     that follow each hunk's leading context, which misses the second of two
//     edits that share a hunk; it also looks up the paths git prints, relative to
//     the repository root, as paths relative to the directory it mutates.
//     mutationgate reads the changed lines itself, with git diff -U0 and without
//     the user's and the system's git config, and its git subcommand, first on
//     gremlins' PATH, answers gremlins' git diff with exactly the lines the gate
//     chose. Every other git command runs the real git. The gate fails when
//     gremlins skips a mutant on a chosen line or tests one on another line.
//   - gremlins mutates every Go file in the directory, also one that go does not
//     build here, such as a file for another operating system, whose mutants would
//     live. mutationgate leaves those files out and lists them as not mutated.
//   - gremlins tests each mutant in a copy of the module that holds nothing above
//     the module's root, so a test that reads a file outside the module, such as
//     one of the repository's workflows, fails for every mutant, which then counts
//     as killed. When a package has detected mutants, mutationgate runs its tests
//     without a mutant in such a copy, and the run does not pass when they fail
//     there. Name such tests TestRepository... and skip them with -skip.
//
// With -max-mutants, at most that many mutants run in all the directories, or the
// mutants of the sample's first line when that line alone holds more. gremlins
// first finds the mutants on the lines in scope without testing them (--dry-run),
// and when more of them would run than the budget, the gate tests a sample: it
// goes round-robin over the files, each round taking the next line of every file,
// with the files and the lines of each in an order fixed by the commit HEAD names.
// While there are no more files than the budget, every file gets a line; with
// more, the order of the files decides which of them the sample covers. Each
// module's report states the share of the mutants, lines and files the sample
// covers.
//
// Test runs skip the tests that -skip matches, by default the live tests
// (^TestLive), which need Permit, and do not use the test cache (-count=1). -tags
// passes build tags to go list, gremlins and go test.
//
// mutationgate runs on Linux and macOS, anywhere in a git repository. Give it the
// directories to mutate: a module's root mutates every package of the module, and
// another directory the packages at or under it. With -base, a changed Go file in
// a module that none of the directories is in fails the run. -gremlins names the
// gremlins executable: a path from the working directory, or a name on PATH.
// bin/mutationgate -h lists every flag with its default. Build it first, because
// go run reports every non-zero exit as 1:
//
//	go -C tools build -o ../bin/mutationgate ./cmd/mutationgate
//	bin/mutationgate -gremlins bin/gremlins -skip '^(TestLive|TestRepository)' \
//	  -base origin/master . tools
//	bin/mutationgate -gremlins bin/gremlins -skip '^(TestLive|TestRepository)' \
//	  -all tools/cmd/cicheck
//
// Locally, pass a -memory-mib below any memory watchdog on the machine, so that the
// gate's limit, not the watchdog, stops a runaway test process.
//
// The report has a section for each module. It goes to stdout and, with -summary,
// to that file as Markdown. The last line mutationgate prints is its verdict:
// PASS, FAIL or DID NOT RUN.
//
// Exit codes:
//
//	0  in every module, the tests detect at least -threshold percent of the
//	   mutants that ran, no Go line changed, the changed lines hold nothing to
//	   mutate or only mutants that do not compile, the module holds nothing to
//	   mutate (with -all, while mutants ran in another module), or the sample holds
//	   none of the module's mutants that would run
//	1  in a module, the tests detect fewer than -threshold percent of the mutants
//	   that ran
//	2  the run did not complete: the command line is wrong, a directory is in no
//	   module, a changed Go file is in a module that none of the directories is in,
//	   a Go file to mutate does not parse, the memory limit is not enforced,
//	   gremlins failed, a mutant was left unrun or on other lines than the gate
//	   chose, exec or the shims could not do their part, a package's tests fail
//	   without a mutant in a copy of its module, Go lines changed and no mutant on
//	   them ran, or with -all a directory holds no code that Go's coverage counts to
//	   mutate, none of a module's mutants ran, or no mutant ran in any module; 2 wins
//	   over 1
package main

import (
	"fmt"
	"io"
	"os"
	"runtime"
)

const (
	exitPassed    = 0
	exitFailed    = 1
	exitDidNotRun = 2
)

const (
	commandExec     = "exec"
	commandAllocate = "allocate"
)

func main() {
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
}

// dispatch runs the gate, or one of the subcommands the gate runs itself as: exec,
// which go test runs each test binary with, allocate, the memory limit probe, and
// go and git, which gremlins runs as the go and git commands.
func dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case commandExec:
			return runExecCommand(args[1:], stderr)
		case commandAllocate:
			return runAllocate(args[1:], stdout, stderr)
		case commandGo:
			return runGoShim(args[1:], stderr)
		case commandGit:
			return runGitShim(args[1:], stdout, stderr)
		}
	}
	if runtime.GOOS == "windows" {
		_, _ = fmt.Fprintln(stderr, "mutationgate: runs on Linux and macOS only")
		_, _ = fmt.Fprintln(stdout, "mutationgate: DID NOT RUN")
		return exitDidNotRun
	}
	t, err := systemTools()
	if err != nil {
		return sink{stdout: stdout, stderr: stderr}.didNotRun(err)
	}
	return runGate(args, stdout, stderr, t)
}
