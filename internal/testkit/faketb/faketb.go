// Package faketb has a fake testing.TB, so that a test can check how a test helper fails the test
// it is given. It is for tests only.
package faketb

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
)

// FakeTB stands in for a test's testing.TB. Errorf and Fatalf record their message instead of
// failing the test, and Cleanup keeps its function until End runs it. Fatalf ends the calling
// goroutine with runtime.Goexit, as testing does, so code that can call it runs through Run. The
// methods FakeTB does not define, such as Context, Name and TempDir, are those of the test's
// own testing.TB. A FakeTB is safe for concurrent use.
type FakeTB struct {
	testing.TB

	mu       sync.Mutex
	messages []string
	failed   bool
	cleanups []func()
}

// New returns a FakeTB for the test tb. When tb ends, it runs the cleanup functions that End has
// not run.
func New(tb testing.TB) *FakeTB {
	tb.Helper()
	f := &FakeTB{TB: tb}
	tb.Cleanup(f.End)
	return f
}

// Helper does nothing: the messages that f records have no file and line to report.
func (f *FakeTB) Helper() {}

// Errorf records the message, and makes Failed report true.
func (f *FakeTB) Errorf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, fmt.Sprintf(format, args...))
	f.failed = true
}

// Fatalf records the message, makes Failed report true, and ends the calling goroutine with
// runtime.Goexit, as testing does.
func (f *FakeTB) Fatalf(format string, args ...any) {
	f.Errorf(format, args...)
	runtime.Goexit()
}

// Failed reports whether Errorf or Fatalf has recorded a message, also one that Take returned.
func (f *FakeTB) Failed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.failed
}

// Take returns the messages that Errorf and Fatalf recorded since the last Take, oldest first,
// and forgets them.
func (f *FakeTB) Take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	messages := f.messages
	f.messages = nil
	return messages
}

// Cleanup registers fn for End to run.
func (f *FakeTB) Cleanup(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanups = append(f.cleanups, fn)
}

// End runs the cleanup functions, last registered first, as testing does when a test ends; a
// function that a cleanup function registers runs next. Each runs once: a later End runs only
// those registered since. A test calls End to check what its cleanup functions report.
func (f *FakeTB) End() {
	for {
		f.mu.Lock()
		last := len(f.cleanups) - 1
		if last < 0 {
			f.mu.Unlock()
			return
		}
		fn := f.cleanups[last]
		f.cleanups = f.cleanups[:last]
		f.mu.Unlock()
		fn()
	}
}

// Run calls fn on its own goroutine, as testing runs a test, waits for it, and reports whether
// fn returned; false means that Fatalf, or another call of runtime.Goexit, ended it.
func (f *FakeTB) Run(fn func()) bool {
	done := make(chan bool)
	go func() {
		returned := false
		defer func() { done <- returned }()
		fn()
		returned = true
	}()
	return <-done
}
