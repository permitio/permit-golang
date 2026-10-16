// Package concurrent runs a function on many goroutines at once and reads every result, for
// tests of code that must be safe for concurrent use. It is for tests only.
package concurrent

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// result is what one call returned.
type result struct {
	i   int
	err error
}

// Run calls fn(ctx, i) for each i from 0 to n-1, each call on its own goroutine, and starts the
// calls together. It waits for every call to return and returns the error of the call with the
// lowest i that failed, wrapped with i, or nil when every call returned nil.
//
// A call that has not returned when timeout has passed is a deadlock or a goroutine that never
// ends, so Run then fails the test with tb.Fatalf and names every call still running. ctx is
// canceled when Run returns or fails the test, so that such a call can stop. Run also fails the
// test when n is below 1, timeout is not positive or fn is nil.
func Run(tb testing.TB, n int, timeout time.Duration, fn func(ctx context.Context, i int) error,
) error {
	tb.Helper()
	if n < 1 || timeout <= 0 || fn == nil {
		tb.Fatalf("concurrent: Run needs n of 1 or more, a positive timeout and a function; got "+
			"n = %d, timeout = %v and a nil function: %t", n, timeout, fn == nil)
	}
	ctx, cancel := context.WithCancel(tb.Context())
	defer cancel()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	results := make(chan result, n)
	start := make(chan struct{})
	for i := range n {
		go func() {
			<-start
			results <- result{i: i, err: fn(ctx, i)}
		}()
	}
	close(start)

	errs := make([]error, n)
	returned := make([]bool, n)
	record := func(r result) {
		errs[r.i] = r.err
		returned[r.i] = true
	}
	for received := 0; received < n; {
		select {
		case r := <-results:
			record(r)
			received++
		case <-timer.C:
			received += drainReturned(results, record)
			if calls := running(returned); calls != "" {
				tb.Fatalf("concurrent: %d of %d calls did not return within %v: %s", n-received,
					n, timeout, calls)
			}
		}
	}
	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("call %d of %d: %w", i, n, err)
		}
	}
	return nil
}

// drainReturned records each result that is waiting in results without blocking, and returns how
// many it recorded. Run calls it when the timeout passes, so that a call that returned at the
// same moment is not named as still running.
func drainReturned(results <-chan result, record func(result)) int {
	drained := 0
	for len(results) > 0 {
		record(<-results)
		drained++
	}
	return drained
}

// running returns the calls that have not returned, written "call 3, call 7".
func running(returned []bool) string {
	var calls string
	for i, done := range returned {
		if done {
			continue
		}
		if calls != "" {
			calls += ", "
		}
		calls += fmt.Sprintf("call %d", i)
	}
	return calls
}
