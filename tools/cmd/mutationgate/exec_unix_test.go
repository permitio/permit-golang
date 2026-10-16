//go:build unix

package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// testMainEnv makes the test binary act as mutationgate, so tests can run its
// subcommands as processes of their own. "unlimited" runs exec with no memory
// limit, a planted defect the memory limit probe must catch, and "capped" runs it
// with no memory limit and a 2s cap, which stops the probe before any limit. In
// "capped" mode the probe allocates one chunk and then waits to be stopped, so the
// cap does not race a child that starts slowly or one that finishes fast.
// "leave-group" makes it a test process that leaves its process group
// (leaveGroupAndWait).
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
				opts.cap = 2 * time.Second
			}
			noLimit := monitor{interval: pollInterval, orphaned: notOrphaned}
			os.Exit(superviseTest(opts, noLimit, os.Stdout, os.Stderr))
		}
		if mode == "capped" && len(os.Args) > 1 && os.Args[1] == commandAllocate {
			allocateAndWait(os.Stdout)
		}
		os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
	case "leave-group":
		os.Exit(leaveGroupAndWait(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func notOrphaned() bool { return false }

// leaveGroupAndWait moves this process from the process group exec started it in to
// the group whose ID is args[0], writes its PID to the file args[1], whole, as
// childScript does, and sleeps for a minute.
func leaveGroupAndWait(args []string) int {
	if len(args) != 2 {
		_, _ = fmt.Fprintln(os.Stderr, "leave-group: need a process group ID and the PID file")
		return exitDidNotRun
	}
	pgid, err := strconv.Atoi(args[0])
	if err == nil {
		err = syscall.Setpgid(0, pgid)
	}
	if err == nil {
		err = os.WriteFile(args[1]+".tmp", []byte(strconv.Itoa(os.Getpid())), 0o600)
	}
	if err == nil {
		err = os.Rename(args[1]+".tmp", args[1])
	}
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "leave-group: %v\n", err)
		return exitDidNotRun
	}
	time.Sleep(time.Minute)
	return exitPassed
}

// allocateAndWait allocates one chunk, reports it as the allocate command does and
// sleeps until exec stops the process.
func allocateAndWait(stdout io.Writer) {
	chunk := make([]byte, allocateChunkMiB<<20)
	for i := 0; i < len(chunk); i += pageBytes {
		chunk[i] = 1
	}
	_, _ = fmt.Fprintf(stdout, "allocated %d MiB\n", allocateChunkMiB)
	time.Sleep(time.Hour)
	runtime.KeepAlive(chunk)
}

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

// childScript is a shell script, run as sh -c childScript sh FILE, that starts a
// child which sleeps for a minute, writes the child's PID to FILE, and then runs
// rest. The PID file appears whole, so it holds the PID once it exists.
func childScript(rest string) string {
	return `sleep 60 & echo $! > "$1.tmp" && mv "$1.tmp" "$1"; ` + rest
}

// exists reports whether the file at path exists.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// readPID returns the PID that childScript wrote to path.
func readPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the child did not start: %v", err)
	}
	var pid int
	if _, err := fmt.Sscan(string(data), &pid); err != nil || pid <= 1 {
		t.Fatalf("PID file %q: %v", data, err)
	}
	return pid
}

// waitUntil polls done until it returns true, for at most limit.
func waitUntil(limit time.Duration, done func() bool) bool {
	for deadline := time.Now().Add(limit); time.Now().Before(deadline); {
		if done() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return done()
}

// requireGone fails the test unless the process pid is gone within ten seconds: a
// killed process whose parent has exited lasts until init reaps it. A process that
// is still running is killed, so that a failing test leaves nothing behind.
func requireGone(t *testing.T, pid int) {
	t.Helper()
	gone := func() bool { return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) }
	if !waitUntil(10*time.Second, gone) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Errorf("process %d, which the test process started, outlived exec", pid)
	}
}

// Stopping a test process kills the processes it started too.
func TestSuperviseTestStopsTheWholeGroup(t *testing.T) {
	t.Parallel()
	const tick = 10 * time.Millisecond
	tests := []struct {
		name string
		cap  time.Duration
		// mon returns the monitor, which stops the test process only once started
		// reports that its child runs.
		mon  func(started func() bool) monitor
		rest string
		// kind and detail are the event that exec logs and prints.
		kind, detail string
	}{
		{
			// The cap leaves sh room to start the child on a busy machine.
			name: "past the cap", cap: 5 * time.Second,
			mon: func(func() bool) monitor {
				return monitor{interval: tick, orphaned: notOrphaned}
			},
			rest: "wait", kind: eventCap, detail: "ran longer than the 5s cap",
		},
		{
			name: "over the memory limit", cap: time.Minute,
			mon: func(started func() bool) monitor {
				memoryKiB := func(int) (int64, error) {
					if started() {
						return 5000 << 10, nil
					}
					return 0, nil
				}
				return monitor{interval: tick, memoryKiB: memoryKiB, orphaned: notOrphaned}
			},
			rest: "wait", kind: eventMemory, detail: "used 5000 MiB, over the 4096 MiB limit",
		},
		{
			name: "past the output budget", cap: time.Minute,
			mon: func(func() bool) monitor {
				return monitor{interval: tick, orphaned: notOrphaned}
			},
			rest: "exec yes", kind: eventOutput, detail: "wrote more than 64 MiB of output",
		},
		{
			name: "when the go command exits", cap: time.Minute,
			mon: func(started func() bool) monitor {
				return monitor{interval: tick, orphaned: started}
			},
			rest: "wait", kind: eventOrphan,
			detail: "outlived the go command that started it (gremlins' timeout)",
		},
		{
			name: "when exec gets a signal", cap: time.Minute,
			mon: func(started func() bool) monitor {
				signals := make(chan os.Signal, 1)
				go func() {
					if waitUntil(10*time.Second, started) {
						signals <- syscall.SIGTERM
					}
				}()
				return monitor{interval: tick, orphaned: notOrphaned, signals: signals}
			},
			rest: "wait", kind: eventSignal, detail: "stopped when exec got a signal: terminated",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pidFile := filepath.Join(t.TempDir(), "child.pid")
			mon := tt.mon(func() bool { return exists(pidFile) })
			exit, stderr, log := supervise(t, tt.cap, mon, "sh", "-c", childScript(tt.rest),
				"sh", pidFile)
			wantLog := tt.kind + "\tsh\t" + tt.detail + "\n"
			wantStderr := "mutationgate exec: sh: " + tt.detail + "\n"
			if exit != exitFailed || log != wantLog || stderr != wantStderr {
				t.Errorf("exit %d, log %q, stderr %q; want log %q, stderr %q", exit, log, stderr,
					wantLog, wantStderr)
			}
			requireGone(t, readPID(t, pidFile))
		})
	}
}

// A test process that exits leaves none of the processes it started behind: exec
// kills them once Wait has waited waitDelay for those that hold its output open.
// Where exec polls memory use, which it cannot read once Wait has reaped the test
// process, it waits for Wait too.
func TestSuperviseTestKillsWhatATestProcessLeft(t *testing.T) {
	t.Parallel()
	reaped := func(pid int) (int64, error) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return 0, errors.New("no such process")
		}
		return 1, nil
	}
	for name, tt := range map[string]struct {
		script    string
		memoryKiB func(int) (int64, error)
	}{
		"a child":                       {script: "exec >/dev/null 2>&1; " + childScript("exit 0")},
		"a child that holds the output": {script: childScript("exit 0")},
		"a child that holds the output, with memory polled": {
			script: childScript("exit 0"), memoryKiB: reaped,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mon := monitor{
				interval: 10 * time.Millisecond, memoryKiB: tt.memoryKiB, orphaned: notOrphaned,
			}
			pidFile := filepath.Join(t.TempDir(), "child.pid")
			exit, stderr, log := supervise(t, time.Minute, mon, "sh", "-c", tt.script, "sh",
				pidFile)
			if exit != exitPassed || log != "" || stderr != "" {
				t.Errorf("exit %d, log %q, stderr %q", exit, log, stderr)
			}
			requireGone(t, readPID(t, pidFile))
		})
	}
}

// A test process that leaves the process group exec started it in is stopped too:
// killGroup kills it by its PID as well as its group.
func TestSuperviseTestStopsATestProcessThatLeftItsGroup(t *testing.T) {
	t.Parallel()
	// The test process moves to the group of this process, which waits meanwhile.
	other := exec.CommandContext(t.Context(), "sleep", "60")
	startInGroup(other)
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = killGroup(other)
		_ = other.Wait()
	})
	pidFile := filepath.Join(t.TempDir(), "test.pid")
	mon := monitor{interval: 10 * time.Millisecond, orphaned: notOrphaned}
	start := time.Now()
	// The cap leaves the test binary room to start and leave its group first.
	exit, stderr, log := supervise(t, 5*time.Second, mon, "env", testMainEnv+"=leave-group",
		testExecutable(t), strconv.Itoa(other.Process.Pid), pidFile)
	elapsed := time.Since(start)
	if !exists(pidFile) {
		t.Fatalf("the test process did not leave its group within the cap; stderr %q", stderr)
	}
	pid := readPID(t, pidFile)
	if exit != exitFailed || log != "cap\tenv\tran longer than the 5s cap\n" {
		t.Errorf("exit %d, log %q, stderr %q", exit, log, stderr)
	}
	if elapsed > 10*time.Second {
		t.Errorf("took %s", elapsed)
	}
	requireGone(t, pid)
}

// The exec subcommand stops the test process with its group when it gets a signal
// that would end it, which the group, in a process group of its own, does not get.
func TestExecStopsTheGroupOnASignal(t *testing.T) {
	t.Parallel()
	bin := gateBinary(t)
	for _, sig := range []syscall.Signal{
		syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT,
	} {
		t.Run(sig.String(), func(t *testing.T) {
			t.Parallel()
			// exec inherits this process's signals and leaves one that it started with
			// ignored as it is.
			if signal.Ignored(sig) {
				t.Skipf("this process started with %s ignored", sig)
			}
			dir := t.TempDir()
			pidFile := filepath.Join(dir, "child.pid")
			logPath := filepath.Join(dir, "events.log")
			cmd := exec.CommandContext(t.Context(), bin, commandExec, "-cap=1m",
				"-memory-mib=4096", "-log="+logPath, "sh", "-c", childScript("wait"), "sh", pidFile)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			// A binary just built can take seconds to start on a busy machine.
			if !waitUntil(time.Minute, func() bool { return exists(pidFile) }) {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatalf("the child did not start; stderr %q", stderr.String())
			}
			pid := readPID(t, pidFile)
			if err := cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			err := cmd.Wait()
			detail := "stopped when exec got a signal: " + sig.String()
			if exitErr, ok := errors.AsType[*exec.ExitError](err); !ok ||
				exitErr.ExitCode() != exitFailed ||
				stderr.String() != "mutationgate exec: sh: "+detail+"\n" {
				t.Errorf("exec: %v, stderr %q; want exit %d", err, stderr.String(), exitFailed)
			}
			log, err := os.ReadFile(logPath)
			if want := "signal\tsh\t" + detail + "\n"; err != nil || string(log) != want {
				t.Errorf("log %q, %v, want %q", log, err, want)
			}
			requireGone(t, pid)
		})
	}
}

// A signal that exec started with ignored, as nohup leaves a hangup, stays ignored:
// the test process inherits it as ignored, as it did before exec ran it in a process
// group of its own, so it does not stop a test that passes.
func TestExecKeepsAnIgnoredSignalIgnored(t *testing.T) {
	t.Parallel()
	bin := gateBinary(t)
	dir := t.TempDir()
	started := filepath.Join(dir, "started")
	logPath := filepath.Join(dir, "events.log")
	cmd := exec.CommandContext(t.Context(), "sh", "-c", `trap "" HUP; exec "$0" "$@"`, bin,
		commandExec, "-cap=1m", "-memory-mib=4096", "-log="+logPath,
		"sh", "-c", `: >"$1"; sleep 2`, "sh", started)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if !waitUntil(time.Minute, func() bool { return exists(started) }) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("the test process did not start; output %q", output.String())
	}
	if err := cmd.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	err := cmd.Wait()
	log, readErr := os.ReadFile(logPath)
	if err != nil || output.Len() != 0 || !errors.Is(readErr, os.ErrNotExist) {
		t.Errorf("exec: %v, output %q, log %q (%v); want exit 0, no output and no log", err,
			output.String(), log, readErr)
	}
}

// When the go command that reads exec's output exits, as it does when gremlins'
// timeout kills it, exec's next write finds the pipe closed. exec must not die of
// SIGPIPE, which would leave the test process's group running, but stop the group
// as an orphan.
func TestExecStopsTheGroupWhenItsOutputCloses(t *testing.T) {
	t.Parallel()
	bin := gateBinary(t)
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	logPath := filepath.Join(dir, "events.log")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	cmd := exec.CommandContext(t.Context(), bin, commandExec, "-cap=1m", "-memory-mib=4096",
		"-log="+logPath, "sh", "-c", childScript("while :; do echo x; sleep 0.01; done"), "sh",
		pidFile)
	cmd.Stdout = w
	cmd.Stderr = w
	if err = errors.Join(cmd.Start(), w.Close()); err != nil {
		t.Fatal(err)
	}
	// The test process writes once its child runs.
	if _, err = bufio.NewReader(r).ReadString('\n'); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("no output: %v", err)
	}
	pid := readPID(t, pidFile)
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); !ok || exitErr.ExitCode() != exitFailed {
		t.Errorf("exec: %v, want exit %d", err, exitFailed)
	}
	log, err := os.ReadFile(logPath)
	want := "orphan\tsh\toutlived the go command that started it, which closed exec's output " +
		"(gremlins' timeout)\n"
	if err != nil || string(log) != want {
		t.Errorf("log %q, %v, want %q", log, err, want)
	}
	requireGone(t, pid)
}

// killGroup reports nothing for a process that has exited, while its group holds
// only it, not yet reaped (which macOS reports as EPERM), and once Wait has reaped
// it and the group is gone.
func TestKillGroupOfAnExitedProcess(t *testing.T) {
	t.Parallel()
	cmd := exec.CommandContext(t.Context(), "sh", "-c", "exit 0")
	startInGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := strconv.Itoa(cmd.Process.Pid)
	exited := func() bool {
		out, err := exec.CommandContext(t.Context(), "ps", "-o", "stat=", "-p", pid).Output()
		return err == nil && strings.HasPrefix(strings.TrimSpace(string(out)), "Z")
	}
	if !waitUntil(10*time.Second, exited) {
		_ = cmd.Wait()
		t.Fatal("the process did not exit")
	}
	if err := killGroup(cmd); err != nil {
		t.Errorf("before Wait: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := killGroup(cmd); err != nil {
		t.Errorf("after Wait: %v", err)
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
