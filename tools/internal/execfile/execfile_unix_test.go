//go:build unix

package execfile_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/tools/internal/execfile"
)

func TestWriteFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	name := filepath.Join(dir, "script")
	if err := execfile.WriteFile(name, []byte("#!/bin/sh\necho first\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	assertFile(t, name, "#!/bin/sh\necho first\n", 0o700)
	// A rewrite truncates the file and keeps its mode, as os.WriteFile does.
	if err := execfile.WriteFile(name, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertFile(t, name, "#!/bin/sh\n", 0o700)
	// A new file gets the mode given.
	readOnly := filepath.Join(dir, "read-only")
	if err := execfile.WriteFile(readOnly, []byte("data\n"), 0o500); err != nil {
		t.Fatal(err)
	}
	assertFile(t, readOnly, "data\n", 0o500)
	err := execfile.WriteFile(filepath.Join(dir, "missing", "script"), nil, 0o700)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a file in a missing directory: got %v, want an error for the missing directory",
			err)
	}
}

func assertFile(t *testing.T, name, want string, mode fs.FileMode) {
	t.Helper()
	got, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want || info.Mode() != mode {
		t.Errorf("%s holds %q with mode %v, want %q with mode %v", name, got, info.Mode(), want,
			mode)
	}
}

// WriteFile holds syscall.ForkLock for reading from open to close, so no process starts while
// the file is open. The test keeps WriteFile between the two: it writes more to a FIFO than the
// pipe holds, and the test checks the lock before it reads any of it. It does not run in parallel
// with the tests that start processes or take the lock, which would hide a missing lock.
func TestWriteFileHoldsTheLockWhileTheFileIsOpen(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte("x"), 1<<20)
	done := make(chan error, 1)
	go func() { done <- execfile.WriteFile(fifo, data, 0o600) }()
	// Opening a FIFO for reading returns once WriteFile has opened it for writing.
	r, err := os.Open(fifo)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	if syscall.ForkLock.TryLock() {
		syscall.ForkLock.Unlock()
		t.Error("WriteFile had the file open without holding syscall.ForkLock")
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(got) != len(data) {
		t.Errorf("read %d bytes from the FIFO, want %d", len(got), len(data))
	}
}

// WriteFile takes syscall.ForkLock for reading, which it shares with other readers, such as
// another WriteFile: it must not wait for them. The test does not run in parallel with the tests
// that start processes, which would wait for the reader it holds.
func TestWriteFileSharesTheLock(t *testing.T) {
	name := filepath.Join(t.TempDir(), "file")
	syscall.ForkLock.RLock()
	done := make(chan error, 1)
	go func() { done <- execfile.WriteFile(name, []byte("x"), 0o600) }()
	select {
	case err := <-done:
		syscall.ForkLock.RUnlock()
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		syscall.ForkLock.RUnlock()
		<-done
		t.Fatal("WriteFile waited while another reader held syscall.ForkLock")
	}
}

// Starting a process holds syscall.ForkLock for writing while it forks, and WriteFile must not
// open the file until the lock is released. The test holds the lock itself, so it must not start
// a process meanwhile.
func TestWriteFileWaitsWhileAProcessStarts(t *testing.T) {
	t.Parallel()
	name := filepath.Join(t.TempDir(), "script")
	done := make(chan error, 1)
	syscall.ForkLock.Lock()
	go func() { done <- execfile.WriteFile(name, []byte("#!/bin/sh\n"), 0o700) }()
	returned := false
	select {
	case <-done:
		returned = true
	case <-time.After(100 * time.Millisecond):
	}
	_, statErr := os.Stat(name)
	syscall.ForkLock.Unlock()
	if returned || !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("WriteFile wrote %s while a process was starting (stat: %v)", name, statErr)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	assertFile(t, name, "#!/bin/sh\n", 0o700)
}

// On Linux, a script written with os.WriteFile and then run fails now and then with "text file
// busy" while other goroutines start processes; a script written with WriteFile never does.
// macOS runs a file that another process has open for writing, so the race cannot happen there,
// and the test starts no other processes: with go1.26.0 and the race detector, a process started
// on macOS while others start now and then crashes or hangs before it runs its program. There
// the test only writes and runs the script.
func TestWriteFileWhileProcessesStart(t *testing.T) {
	t.Parallel()
	trueCmd, err := exec.LookPath("true")
	if err != nil {
		t.Fatalf("the test starts true, which is not on PATH: %v", err)
	}
	helpers := 4
	if runtime.GOOS == "darwin" {
		helpers = 0
	}
	ctx, stop := context.WithCancel(t.Context())
	var starters sync.WaitGroup
	var started atomic.Int64
	for range helpers {
		starters.Go(func() {
			for ctx.Err() == nil {
				// Only the start matters: how true exits says nothing about the script, and
				// under this load it now and then crashes on some systems.
				err := exec.CommandContext(ctx, trueCmd).Run()
				var exited *exec.ExitError
				if err != nil && !errors.As(err, &exited) && ctx.Err() == nil {
					t.Errorf("starting %s: %v", trueCmd, err)
					return
				}
				started.Add(1)
			}
		})
	}
	defer starters.Wait()
	defer stop()
	// Some systems check an executable the first time it runs, which is slow, so every run
	// rewrites the same file.
	script := filepath.Join(t.TempDir(), "script")
	deadline := time.Now().Add(3 * time.Second)
	runs := 0
	for ; runs < 300 && time.Now().Before(deadline); runs++ {
		want := strconv.Itoa(runs)
		content := []byte("#!/bin/sh\necho " + want + "\n")
		if err := execfile.WriteFile(script, content, 0o700); err != nil {
			t.Fatal(err)
		}
		out, err := exec.CommandContext(t.Context(), script).Output()
		if err != nil {
			t.Fatalf("run %d of the script: %v", runs, err)
		}
		if got := strings.TrimSpace(string(out)); got != want {
			t.Fatalf("run %d of the script printed %q, want %q", runs, got, want)
		}
	}
	if helpers > 0 && started.Load() == 0 {
		t.Error("no process started while the script ran, so the test exercised nothing")
	}
	t.Logf("ran the script %d times while other goroutines started %d processes", runs,
		started.Load())
}
