package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// The kinds of event exec logs: why it stopped a test process, that a test
// process ran out of memory, that it could not apply a limit, which the gate
// treats as a run that did not complete, or how long an unmutated test process
// ran (baseline).
const (
	eventCap      = "cap"
	eventMemory   = "memory"
	eventOutput   = "output"
	eventOrphan   = "orphan"
	eventSignal   = "signal"
	eventError    = "error"
	eventBaseline = "baseline"
)

// pollInterval is how often exec checks a test process's memory and whether the go
// command that started it is still running.
const pollInterval = 100 * time.Millisecond

// exitGrace is how long exec waits, after it fails to read a test process's
// memory use, for the process to be reaped: one that has exited but is not yet
// reaped cannot be measured.
const exitGrace = time.Second

// outputBudget is the most a test process may write to stdout and stderr together.
// The go command keeps a package's test output in memory, where neither the
// memory limit nor the cap would stop a mutant that prints without end in time.
const outputBudget = 64 << 20

// waitDelay is how long exec waits, once a test process has exited, for processes
// it started that hold its output open.
const waitDelay = 5 * time.Second

// oomMarkers are how the Go runtime reports, when it exits, that an allocation
// failed: under RLIMIT_AS on Linux, a test process that passes the limit.
var oomMarkers = [][]byte{
	[]byte("fatal error: out of memory"),
	[]byte("fatal error: runtime: out of memory"),
}

// oomCarry is how much of a write scanForOOM keeps for the next one: one byte less
// than the longest marker.
var oomCarry = len(slices.MaxFunc(oomMarkers, func(a, b []byte) int { return len(a) - len(b) })) - 1

// execOptions is the parsed command line of the exec subcommand.
type execOptions struct {
	cap       time.Duration
	memoryMiB int64
	logPath   string
	command   []string
}

// monitor is what exec watches while a test process runs.
type monitor struct {
	interval time.Duration
	// memoryKiB returns the memory a process uses, in KiB. It is nil where the
	// kernel enforces the limit.
	memoryKiB func(pid int) (int64, error)
	// orphaned reports whether the go command that started exec has exited.
	orphaned func() bool
}

// runExecCommand is the exec subcommand: go test runs each test binary as
// "mutationgate exec [flags] binary args...", and exec runs it within the limits.
func runExecCommand(args []string, stderr io.Writer) int {
	opts, err := parseExecArgs(args, stderr)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "mutationgate exec: %v\n", err)
		return exitDidNotRun
	}
	memoryKiB, err := limitMemory(opts.memoryMiB)
	if err != nil {
		reportEvent(opts, stderr, eventError,
			fmt.Sprintf("cannot limit memory to %d MiB: %v", opts.memoryMiB, err))
		return exitDidNotRun
	}
	parent := os.Getppid()
	return superviseTest(opts, monitor{
		interval:  pollInterval,
		memoryKiB: memoryKiB,
		orphaned:  func() bool { return os.Getppid() != parent },
	}, os.Stdout, stderr)
}

func parseExecArgs(args []string, stderr io.Writer) (execOptions, error) {
	fs := flag.NewFlagSet("mutationgate exec", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var opts execOptions
	fs.DurationVar(&opts.cap, "cap", 0, "longest the test process may run")
	fs.Int64Var(&opts.memoryMiB, "memory-mib", 0, "most memory the test process may use, in MiB")
	fs.StringVar(&opts.logPath, "log", "", "file to append stop events to")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	opts.command = fs.Args()
	switch {
	case len(opts.command) == 0:
		return opts, errors.New("no test binary to run")
	case opts.cap <= 0:
		return opts, fmt.Errorf("-cap %s: need a positive duration", opts.cap)
	case opts.memoryMiB < 1:
		return opts, fmt.Errorf("-memory-mib %d: need at least 1", opts.memoryMiB)
	}
	return opts, nil
}

// isBaselineRun reports whether go test started the test binary for gremlins'
// coverage run, the package's tests without a mutant.
func isBaselineRun(command []string) bool {
	return slices.ContainsFunc(command, func(arg string) bool {
		return strings.HasPrefix(arg, "-test.coverprofile=")
	})
}

// superviseTest runs the test process and returns its exit code. It stops the
// process and returns 1 when the process runs past the cap, uses more memory than
// the limit, writes more than outputBudget, or outlives the go command, and returns
// 1 when a signal kills it, so go test reports the package as failed and gremlins
// counts the mutant as killed. A run without a mutant (gremlins' coverage run) may
// take half the cap, so that a mutant of the package has room before the cap
// counts it as killed.
func superviseTest(opts execOptions, m monitor, stdout, stderr io.Writer) int {
	watch := &outputWatch{exceeded: make(chan struct{})}
	watch.left.Store(outputBudget)
	// exec stops the process itself, so the context is never cancelled.
	//nolint:gosec // G204: exec runs the test binary go test hands it.
	cmd := exec.CommandContext(context.Background(), opts.command[0], opts.command[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = &watchedWriter{w: stdout, watch: watch}
	cmd.Stderr = &watchedWriter{w: stderr, watch: watch, scan: true}
	cmd.WaitDelay = waitDelay
	baseline := isBaselineRun(opts.command)
	limit := opts.cap
	if baseline {
		limit = opts.cap / 2
	}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		reportEvent(opts, stderr, eventError, fmt.Sprintf("cannot start: %v", err))
		return exitFailed
	}
	t := &testProcess{opts: opts, stderr: stderr, cmd: cmd, watch: watch, done: make(chan error, 1)}
	go func() { t.done <- cmd.Wait() }()
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case err := <-t.done:
			if baseline {
				logEvent(opts.logPath, stderr, event{
					kind: eventBaseline,
					binary: commandName(
						opts.command,
					),
					detail: time.Since(start).Round(time.Second).String(),
				})
			}
			return t.exitCode(err)
		case <-deadline.C:
			if baseline {
				return t.stop(eventCap, fmt.Sprintf("ran longer than %s without a mutant, half "+
					"the %s cap, so a mutant of its package could reach the cap and count as "+
					"killed: raise -cap", limit, opts.cap))
			}
			return t.stop(eventCap, fmt.Sprintf("ran longer than the %s cap", opts.cap))
		case <-watch.exceeded:
			return t.stop(
				eventOutput,
				fmt.Sprintf("wrote more than %d MiB of output", outputBudget>>20),
			)
		case <-ticker.C:
			if m.orphaned() {
				return t.stop(eventOrphan,
					"outlived the go command that started it (gremlins' timeout)")
			}
			if m.memoryKiB == nil {
				continue
			}
			kib, err := m.memoryKiB(cmd.Process.Pid)
			if err != nil {
				select {
				case waitErr := <-t.done:
					return t.exitCode(waitErr)
				case <-time.After(exitGrace):
				}
				return t.stop(eventError, fmt.Sprintf("cannot read its memory use: %v", err))
			}
			if kib > opts.memoryMiB<<10 {
				return t.stop(eventMemory,
					fmt.Sprintf("used %d MiB, over the %d MiB limit", kib>>10, opts.memoryMiB))
			}
		}
	}
}

// outputWatch counts what a test process writes to stdout and stderr together, and
// notes when its stderr reports that the Go runtime ran out of memory.
type outputWatch struct {
	left     atomic.Int64
	exceeded chan struct{}
	once     sync.Once
	oom      atomic.Bool
}

// watchedWriter passes a test process's output on until the output budget runs
// out, then drops it, so the process is not blocked on a full pipe before exec
// stops it. os/exec copies each stream from one goroutine, so carry needs no lock.
type watchedWriter struct {
	w     io.Writer
	watch *outputWatch
	scan  bool
	carry []byte
}

func (ww *watchedWriter) Write(p []byte) (int, error) {
	if ww.watch.left.Add(-int64(len(p))) < 0 {
		ww.watch.once.Do(func() { close(ww.watch.exceeded) })
		return len(p), nil
	}
	if ww.scan && !ww.watch.oom.Load() {
		ww.scanForOOM(p)
	}
	return ww.w.Write(p)
}

// scanForOOM looks for the Go runtime's out of memory report in p and the end of
// the previous write, which a report split across two writes starts in.
func (ww *watchedWriter) scanForOOM(p []byte) {
	text := append(slices.Clip(ww.carry), p...)
	for _, marker := range oomMarkers {
		if bytes.Contains(text, marker) {
			ww.watch.oom.Store(true)
		}
	}
	keep := min(len(text), oomCarry)
	ww.carry = slices.Clone(text[len(text)-keep:])
}

// testProcess is a running test process and the result of its Wait.
type testProcess struct {
	opts   execOptions
	stderr io.Writer
	cmd    *exec.Cmd
	watch  *outputWatch
	done   chan error
}

func (t *testProcess) stop(kind, detail string) int {
	if err := t.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		_, _ = fmt.Fprintf(t.stderr, "mutationgate exec: kill %s: %v\n", t.opts.command[0], err)
	}
	<-t.done
	reportEvent(t.opts, t.stderr, kind, detail)
	return exitFailed
}

func (t *testProcess) exitCode(err error) int {
	if err == nil || errors.Is(err, exec.ErrWaitDelay) {
		return exitPassed
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		reportEvent(t.opts, t.stderr, eventError, fmt.Sprintf("wait: %v", err))
		return exitFailed
	}
	code := exitErr.ExitCode()
	if code < 0 {
		reportEvent(t.opts, t.stderr, eventSignal, "killed by a "+exitErr.String())
		return exitFailed
	}
	if t.watch.oom.Load() {
		reportEvent(t.opts, t.stderr, eventMemory, fmt.Sprintf(
			"ran out of memory under the %d MiB limit and exited %d", t.opts.memoryMiB, code))
	}
	return code
}

func commandName(command []string) string {
	if len(command) == 0 {
		return "(none)"
	}
	return filepath.Base(command[0])
}

// reportEvent prints an event and appends it to the log.
func reportEvent(opts execOptions, stderr io.Writer, kind, detail string) {
	binary := commandName(opts.command)
	_, _ = fmt.Fprintf(stderr, "mutationgate exec: %s: %s\n", binary, detail)
	logEvent(opts.logPath, stderr, event{kind: kind, binary: binary, detail: detail})
}

// logEvent appends an event to the log at logPath, if any, one line per event with
// a single write, so the processes of parallel test runs can share the log.
func logEvent(logPath string, stderr io.Writer, e event) {
	if logPath == "" {
		return
	}
	line := strings.Join([]string{e.kind, e.binary, strings.ReplaceAll(e.detail, "\n", " ")}, "\t")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err == nil {
		_, err = f.WriteString(line + "\n")
		err = errors.Join(err, f.Close())
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "mutationgate: cannot log the event: %v\n", err)
	}
}

// The allocate subcommand allocates memory in chunks of allocateChunkMiB and
// writes to every page, so each chunk is in use, until it has allocated
// -up-to-mib. It pauses allocatePause after each chunk, so it grows by at most
// about 128 MiB between two of exec's memory checks and passes a limit well
// before it reaches twice the limit, which is where it stops by itself.
const (
	allocateChunkMiB = 32
	allocatePause    = 25 * time.Millisecond
	pageBytes        = 4096
)

// runAllocate is the memory limit probe: a process that allocates without end, up
// to a ceiling so that it cannot exhaust memory where no limit stops it.
func runAllocate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mutationgate allocate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	upTo := fs.Int64("up-to-mib", 0, "stop after allocating this much, in MiB")
	if err := fs.Parse(args); err != nil {
		return exitDidNotRun
	}
	if *upTo < 1 || fs.NArg() > 0 {
		_, _ = fmt.Fprintln(stderr,
			"mutationgate allocate: need -up-to-mib of at least 1 and no arguments")
		return exitDidNotRun
	}
	var chunks [][]byte
	for allocated := int64(0); allocated < *upTo; {
		chunk := make([]byte, allocateChunkMiB<<20)
		for i := 0; i < len(chunk); i += pageBytes {
			chunk[i] = 1
		}
		chunks = append(chunks, chunk)
		allocated += allocateChunkMiB
		_, _ = fmt.Fprintf(stdout, "allocated %d MiB\n", allocated)
		time.Sleep(allocatePause)
	}
	runtime.KeepAlive(chunks)
	return exitPassed
}

// probeCap bounds the memory limit probe in time. Allocating twice the limit takes
// seconds.
const probeCap = 2 * time.Minute

// probeMarginMiB is how far past the limit the probe may get before exec stops it:
// on macOS exec polls, and the probe grows by at most about 128 MiB between polls.
const probeMarginMiB = 512

// probeMemoryLimit runs the allocate probe through exec with the gate's memory
// limit and returns how much it allocated before the limit stopped it. The probe
// stops by itself at twice the limit, so a limit that is not enforced shows as a
// probe that exits 0. exec must log that the limit stopped it: a stop by anything
// else, such as a memory watchdog with a lower limit, does not show that the limit
// holds.
func probeMemoryLimit(self string, opts gateOptions) (int64, error) {
	logDir, err := os.MkdirTemp("", "mutationgate-probe-")
	if err != nil {
		return 0, err
	}
	defer func() { _ = os.RemoveAll(logDir) }()
	logPath := filepath.Join(logDir, "events.log")
	upTo := 2 * opts.memoryMiB
	//nolint:gosec // G204: the probe runs this executable's own subcommands.
	cmd := exec.CommandContext(context.Background(), self, commandExec,
		"-cap="+probeCap.String(), fmt.Sprintf("-memory-mib=%d", opts.memoryMiB),
		"-log="+logPath, self, commandAllocate, fmt.Sprintf("-up-to-mib=%d", upTo))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	allocated := lastAllocated(stdout.Bytes())
	events, readErr := readEvents(logPath)
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0, fmt.Errorf(
			"a process allocated %d MiB under a %d MiB limit and was not stopped: "+
				"the limit is not enforced",
			allocated,
			opts.memoryMiB,
		)
	case !errors.As(err, &exitErr):
		return 0, fmt.Errorf("the probe did not start: %w", err)
	case readErr != nil:
		return 0, fmt.Errorf("the probe's event log: %w", readErr)
	case allocated == 0:
		return 0, fmt.Errorf("the probe failed before it allocated anything under a %d MiB "+
			"limit (%v): %s", opts.memoryMiB, err, lastLines(stderr.String(), 5))
	case !slices.ContainsFunc(events, func(e event) bool { return e.kind == eventMemory }):
		return 0, fmt.Errorf(
			"the probe stopped after allocating %d MiB, but not at the %d MiB limit "+
				"(%v; %s): something else, such as a memory watchdog below -memory-mib, stopped it",
			allocated,
			opts.memoryMiB,
			err,
			describeEvents(events),
		)
	case allocated > opts.memoryMiB+probeMarginMiB:
		return 0, fmt.Errorf("the probe allocated %d MiB before the %d MiB limit stopped it: the "+
			"limit is not enforced in time", allocated, opts.memoryMiB)
	}
	return allocated, nil
}

func describeEvents(events []event) string {
	if len(events) == 0 {
		return "exec logged nothing"
	}
	var parts []string
	for _, e := range events {
		parts = append(parts, e.kind+": "+e.detail)
	}
	return strings.Join(parts, "; ")
}

func lastAllocated(out []byte) int64 {
	var last int64
	for line := range strings.Lines(string(out)) {
		var mib int64
		if _, err := fmt.Sscanf(line, "allocated %d MiB", &mib); err == nil {
			last = mib
		}
	}
	return last
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.Join(lines[max(0, len(lines)-n):], " | ")
}
