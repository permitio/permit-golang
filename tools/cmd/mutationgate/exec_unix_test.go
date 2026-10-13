//go:build unix

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// testMainEnv makes the test binary act as mutationgate, so tests can run its
// subcommands as processes of their own. "unlimited" runs exec with no memory
// limit, a planted defect the memory limit probe must catch, and "capped" runs it
// with no memory limit and a 300ms cap, which stops the probe before any limit.
const testMainEnv = "MUTATIONGATE_TEST_MAIN"

func TestMain(m *testing.M) {
	//nolint:forbidigo // The tests start this binary as mutationgate with this variable set.
	mode := os.Getenv(testMainEnv)
	switch mode {
	case "main":
		os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
	case "unlimited", "capped":
		if len(os.Args) > 1 && os.Args[1] == commandExec {
			opts, err := parseExecArgs(os.Args[2:], os.Stderr)
			if err != nil {
				os.Exit(exitDidNotRun)
			}
			if mode == "capped" {
				opts.cap = 300 * time.Millisecond
			}
			noLimit := monitor{interval: pollInterval, orphaned: notOrphaned}
			os.Exit(superviseTest(opts, noLimit, os.Stdout, os.Stderr))
		}
		os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

func notOrphaned() bool { return false }

// supervise runs command under superviseTest with m and returns its exit code, what
// it printed and the event log.
func supervise(
	t *testing.T, limit time.Duration, mon monitor, command ...string,
) (int, string, string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "events.log")
	var stderr bytes.Buffer
	opts := execOptions{cap: limit, memoryMiB: 4096, logPath: logPath, command: command}
	exit := superviseTest(opts, mon, io.Discard, &stderr)
	log, err := os.ReadFile(logPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return exit, stderr.String(), string(log)
}

func TestSuperviseTestPassesTheExitCode(t *testing.T) {
	mon := monitor{interval: 10 * time.Millisecond, orphaned: notOrphaned}
	for command, want := range map[string]int{"exit 0": 0, "exit 3": 3} {
		exit, _, log := supervise(t, time.Minute, mon, "sh", "-c", command)
		if exit != want || log != "" {
			t.Errorf("%s: exit %d, log %q", command, exit, log)
		}
	}
}

// A process that has exited but is not yet reaped cannot be measured; exec must
// return its exit code, not report that it could not read its memory use.
func TestSuperviseTestWaitsForTheExit(t *testing.T) {
	gone := func(int) (int64, error) { return 0, errors.New("no such process") }
	mon := monitor{interval: 10 * time.Millisecond, memoryKiB: gone, orphaned: notOrphaned}
	exit, stderr, log := supervise(t, time.Minute, mon, "sh", "-c", "sleep 0.2; exit 3")
	if exit != 3 || log != "" {
		t.Errorf("exit %d, log %q, stderr %q", exit, log, stderr)
	}
}

func TestSuperviseTestStops(t *testing.T) {
	t.Parallel()
	const tick = 10 * time.Millisecond
	overLimit := func(int) (int64, error) { return 5000 << 10, nil }
	unreadable := func(int) (int64, error) { return 0, errors.New("no such process") }
	sleep := []string{"sleep", "30"}
	tests := []struct {
		name    string
		cap     time.Duration
		mon     monitor
		command []string
		wantLog string
	}{
		{
			name: "past the cap", cap: 300 * time.Millisecond,
			mon:     monitor{interval: tick, orphaned: notOrphaned},
			command: sleep, wantLog: "cap\tsleep\tran longer than the 300ms cap\n",
		},
		{
			name: "over the memory limit", cap: time.Minute,
			mon:     monitor{interval: tick, memoryKiB: overLimit, orphaned: notOrphaned},
			command: sleep, wantLog: "memory\tsleep\tused 5000 MiB, over the 4096 MiB limit\n",
		},
		{
			name: "when the go command exits", cap: time.Minute,
			mon:     monitor{interval: tick, orphaned: func() bool { return true }},
			command: sleep,
			wantLog: "orphan\tsleep\toutlived the go command that started it (gremlins' timeout)\n",
		},
		{
			name: "when its memory cannot be read", cap: time.Minute,
			mon:     monitor{interval: tick, memoryKiB: unreadable, orphaned: notOrphaned},
			command: sleep, wantLog: "error\tsleep\tcannot read its memory use: no such process\n",
		},
		{
			name: "killed by a signal", cap: time.Minute,
			mon:     monitor{interval: tick, orphaned: notOrphaned},
			command: []string{"sh", "-c", "kill -KILL $$"},
			wantLog: "signal\tsh\tkilled by a signal: killed\n",
		},
		{
			name: "past the output budget", cap: time.Minute,
			mon:     monitor{interval: tick, orphaned: notOrphaned},
			command: []string{"yes"}, wantLog: "output\tyes\twrote more than 64 MiB of output\n",
		},
		{
			name: "past half the cap without a mutant", cap: 600 * time.Millisecond,
			mon:     monitor{interval: tick, orphaned: notOrphaned},
			command: []string{"sh", "-c", "sleep 30", "sh", "-test.coverprofile=/tmp/c.out"},
			wantLog: "cap\tsh\tran longer than 300ms without a mutant, half the 600ms cap",
		},
		{
			name: "cannot start", cap: time.Minute,
			mon:     monitor{interval: tick, orphaned: notOrphaned},
			command: []string{filepath.Join(t.TempDir(), "missing.test")},
			wantLog: "error\tmissing.test\tcannot start",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			start := time.Now()
			exit, stderr, log := supervise(t, tt.cap, tt.mon, tt.command...)
			if exit != exitFailed {
				t.Errorf("exit %d, want %d; stderr %q", exit, exitFailed, stderr)
			}
			if !strings.HasPrefix(log, tt.wantLog) {
				t.Errorf("log %q, want %q", log, tt.wantLog)
			}
			if elapsed := time.Since(start); elapsed > 10*time.Second {
				t.Errorf("took %s", elapsed)
			}
		})
	}
}

// A test process of gremlins' coverage run logs how long it ran.
func TestSuperviseTestLogsABaseline(t *testing.T) {
	mon := monitor{interval: 10 * time.Millisecond, orphaned: notOrphaned}
	command := []string{"sh", "-c", "exit 0", "sh", "-test.coverprofile=/tmp/c.out"}
	exit, _, log := supervise(t, time.Minute, mon, command...)
	if exit != exitPassed || log != "baseline\tsh\t0s\n" {
		t.Errorf("exit %d, log %q", exit, log)
	}
}

// On Linux the kernel stops a test process at the memory limit: the Go runtime
// exits 2 with "fatal error: out of memory", which exec logs as a memory event.
func TestSuperviseTestLogsOutOfMemory(t *testing.T) {
	mon := monitor{interval: 10 * time.Millisecond, orphaned: notOrphaned}
	crash := `echo "fatal error: runtime: out of memory" >&2; exit 2`
	exit, _, log := supervise(t, time.Minute, mon, "sh", "-c", crash)
	want := "memory\tsh\tran out of memory under the 4096 MiB limit and exited 2\n"
	if exit != 2 || log != want {
		t.Errorf("exit %d, log %q", exit, log)
	}
	exit, _, log = supervise(t, time.Minute, mon, "sh", "-c", "echo out of memory >&2; exit 1")
	if exit != 1 || log != "" {
		t.Errorf("a test that prints out of memory: exit %d, log %q", exit, log)
	}
}

// A test process may use exactly the memory limit; only more stops it.
func TestSuperviseTestAllowsMemoryAtTheLimit(t *testing.T) {
	t.Parallel()
	atLimit := func(int) (int64, error) { return 4096 << 10, nil }
	mon := monitor{interval: 10 * time.Millisecond, memoryKiB: atLimit, orphaned: notOrphaned}
	exit, stderr, log := supervise(t, time.Minute, mon, "sh", "-c", "sleep 0.2")
	if exit != exitPassed || log != "" {
		t.Errorf("exit %d, log %q, stderr %q", exit, log, stderr)
	}
}

// A test process may write exactly the output budget; only more stops it.
func TestWatchedWriterPassesTheWholeBudget(t *testing.T) {
	watch := &outputWatch{exceeded: make(chan struct{})}
	watch.left.Store(5)
	var out bytes.Buffer
	w := &watchedWriter{w: &out, watch: watch}
	if _, err := w.Write([]byte("12345")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-watch.exceeded:
		t.Fatal("writing the whole budget exceeded it")
	default:
	}
	if _, err := w.Write([]byte("6")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-watch.exceeded:
	default:
		t.Error("writing past the budget did not exceed it")
	}
	if out.String() != "12345" {
		t.Errorf("passed on %q", out.String())
	}
}

// logEvent appends an event as one line, and reports a log it cannot write.
func TestLogEvent(t *testing.T) {
	t.Parallel()
	logPath := filepath.Join(t.TempDir(), "events.log")
	var stderr bytes.Buffer
	logEvent(logPath, &stderr, event{kind: eventCap, binary: "a.test", detail: "ran\nlong"})
	logEvent(logPath, &stderr, event{kind: eventError, binary: "go", detail: "broke"})
	got, err := os.ReadFile(logPath)
	if err != nil || string(got) != "cap\ta.test\tran long\nerror\tgo\tbroke\n" ||
		stderr.Len() != 0 {
		t.Errorf("log %q, %v, stderr %q", got, err, stderr.String())
	}
	logEvent(t.TempDir(), &stderr, event{kind: eventCap, binary: "a.test", detail: "x"})
	if !strings.Contains(stderr.String(), "mutationgate: cannot log the event: ") {
		t.Errorf("a log that cannot be written: stderr %q", stderr.String())
	}
}

func TestWatchedWriterFindsASplitReport(t *testing.T) {
	watch := &outputWatch{exceeded: make(chan struct{})}
	watch.left.Store(outputBudget)
	var out bytes.Buffer
	w := &watchedWriter{w: &out, watch: watch, scan: true}
	for _, part := range []string{"goroutine 1\nfatal error: runt", "ime: out of mem", "ory\n"} {
		if _, err := w.Write([]byte(part)); err != nil {
			t.Fatal(err)
		}
	}
	if !watch.oom.Load() || out.String() != "goroutine 1\nfatal error: runtime: out of memory\n" {
		t.Errorf("oom %v, output %q", watch.oom.Load(), out.String())
	}
}

func TestRunExecCommandRejectsBadArguments(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"-cap=1m", "-memory-mib=1"},
		{"-memory-mib=1", "x.test"},
		{"-cap=1m", "x.test"},
		{"-cap=0s", "-memory-mib=1", "x.test"},
	} {
		if exit := runExecCommand(args, &bytes.Buffer{}); exit != exitDidNotRun {
			t.Errorf("%v: exit %d", args, exit)
		}
	}
}

func TestRunAllocate(t *testing.T) {
	var stdout bytes.Buffer
	exit := runAllocate([]string{"-up-to-mib", "64"}, &stdout, &bytes.Buffer{})
	if exit != exitPassed {
		t.Errorf("exit %d", exit)
	}
	if got := stdout.String(); got != "allocated 32 MiB\nallocated 64 MiB\n" {
		t.Errorf("output %q", got)
	}
	stdout.Reset()
	exit = runAllocate([]string{"-up-to-mib", "1"}, &stdout, &bytes.Buffer{})
	if exit != exitPassed || stdout.String() != "allocated 32 MiB\n" {
		t.Errorf("the smallest ceiling: exit %d, output %q", exit, stdout.String())
	}
	for _, args := range [][]string{{}, {"-up-to-mib", "0"}, {"-up-to-mib", "1", "x"}, {"-nope"}} {
		if exit := runAllocate(args, &bytes.Buffer{}, &bytes.Buffer{}); exit != exitDidNotRun {
			t.Errorf("%v: exit %d", args, exit)
		}
	}
}

// probeLimitMiB is small enough to allocate in a test, large enough on Linux for
// the Go runtime's address space reservations, which count against RLIMIT_AS, and
// elsewhere far enough below twice itself that the probe cannot pass both between
// two of exec's memory checks.
func probeLimitMiB() int64 {
	if runtime.GOOS == "linux" {
		return 2048
	}
	return 512
}

func testExecutable(t *testing.T) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return self
}

// gateBinary builds mutationgate as go build does, without -race even when the
// tests run with it: the race detector's address space reservations cannot start
// under RLIMIT_AS on Linux, and the gate never runs with it.
func gateBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "mutationgate")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func TestProbeMemoryLimit(t *testing.T) {
	t.Parallel()
	limit := probeLimitMiB()
	stoppedAt, err := probeMemoryLimit(gateBinary(t), gateOptions{memoryMiB: limit})
	if err != nil {
		t.Fatal(err)
	}
	if stoppedAt <= 0 || stoppedAt > limit+probeMarginMiB {
		t.Errorf("stopped after %d MiB under a %d MiB limit", stoppedAt, limit)
	}
}

// A probe stopped by anything but the memory limit, here the cap, does not show
// that the limit holds.
func TestProbeMemoryLimitFailsOnAnotherStop(t *testing.T) {
	t.Setenv(testMainEnv, "capped")
	_, err := probeMemoryLimit(testExecutable(t), gateOptions{memoryMiB: 1024})
	want := "but not at the 1024 MiB limit"
	if err == nil || !strings.Contains(err.Error(), want) ||
		!strings.Contains(err.Error(), "cap: ") {
		t.Errorf("got %v", err)
	}
}

func TestProbeMemoryLimitFailsWithoutALimit(t *testing.T) {
	t.Setenv(testMainEnv, "unlimited")
	_, err := probeMemoryLimit(testExecutable(t), gateOptions{memoryMiB: 64})
	want := "allocated 128 MiB under a 64 MiB limit and was not stopped"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("got %v", err)
	}
}

func TestProbeMemoryLimitFailsIfItCannotRun(t *testing.T) {
	_, err := probeMemoryLimit("false", gateOptions{memoryMiB: 64})
	if err == nil || !strings.Contains(err.Error(), "failed before it allocated anything") {
		t.Errorf("got %v", err)
	}
}
