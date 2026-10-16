package concurrent

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/internal/testkit/faketb"
)

// TestRunStartsTheCallsTogether checks that every call runs at the same time as the others:
// each waits until all have started, which deadlocks unless they run together.
func TestRunStartsTheCallsTogether(t *testing.T) {
	const n = 16
	var mu sync.Mutex
	var called []int
	allStarted := make(chan struct{})
	err := Run(t, n, 10*time.Second, func(ctx context.Context, i int) error {
		mu.Lock()
		called = append(called, i)
		if len(called) == n {
			close(allStarted)
		}
		mu.Unlock()
		select {
		case <-allStarted:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
	slices.Sort(called)
	if want := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}; !slices.Equal(
		called, want) {
		t.Errorf("Run called %v, want %v", called, want)
	}
}

func TestRunOneCall(t *testing.T) {
	calls := 0
	err := Run(t, 1, time.Second, func(context.Context, int) error {
		calls++
		return nil
	})
	if err != nil || calls != 1 {
		t.Errorf("Run() = %v after %d calls, want nil after 1", err, calls)
	}
}

// TestRunReturnsTheLowestFailedCall checks that Run waits for every call, even after one failed,
// and returns the error of the failed call with the lowest index.
func TestRunReturnsTheLowestFailedCall(t *testing.T) {
	errFirst, errSecond := errors.New("first"), errors.New("second")
	var slowReturned atomic.Bool
	err := Run(t, 5, 10*time.Second, func(_ context.Context, i int) error {
		switch i {
		case 1:
			return errFirst
		case 3:
			return errSecond
		case 4:
			time.Sleep(200 * time.Millisecond)
			slowReturned.Store(true)
		}
		return nil
	})
	if !errors.Is(err, errFirst) || err.Error() != "call 1 of 5: first" {
		t.Errorf("Run() = %v, want call 1's error, wrapped", err)
	}
	if !slowReturned.Load() {
		t.Error("Run returned before every call had returned")
	}
}

func TestRunFailsWhenACallNeverReturns(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	ft := faketb.New(t)
	timeout := 300 * time.Millisecond
	var ctxErr atomic.Value

	start := time.Now()
	returned := ft.Run(func() {
		_ = Run(ft, 4, timeout, func(ctx context.Context, i int) error {
			if i == 0 || i == 2 {
				<-release
				ctxErr.Store(fmt.Sprint(ctx.Err()))
			}
			return nil
		})
	})
	elapsed := time.Since(start)

	if returned {
		t.Error("Run returned, want it to end the test with Fatalf")
	}
	want := []string{"concurrent: 2 of 4 calls did not return within 300ms: call 0, call 2"}
	if got := ft.Take(); !slices.Equal(got, want) {
		t.Errorf("Run reported %q, want %q", got, want)
	}
	if elapsed < timeout || elapsed > timeout+5*time.Second {
		t.Errorf("Run failed the test after %v, want it at the timeout, %v", elapsed, timeout)
	}
	release <- struct{}{}
	for ctxErr.Load() == nil {
		time.Sleep(time.Millisecond)
	}
	if got := ctxErr.Load(); got != "context canceled" {
		t.Errorf("a call still running when Run failed the test has ctx.Err() = %v, "+
			"want context canceled", got)
	}
}

func TestRunRejectsBadArguments(t *testing.T) {
	ok := func(context.Context, int) error { return nil }
	tests := []struct {
		name    string
		n       int
		timeout time.Duration
		fn      func(context.Context, int) error
		want    string
	}{
		{
			name: "no calls", n: 0, timeout: time.Second, fn: ok,
			want: "got n = 0, timeout = 1s and a nil function: false",
		},
		{
			name: "negative calls", n: -1, timeout: time.Second, fn: ok,
			want: "got n = -1, timeout = 1s and a nil function: false",
		},
		{
			name: "no timeout", n: 1, timeout: 0, fn: ok,
			want: "got n = 1, timeout = 0s and a nil function: false",
		},
		{
			name: "negative timeout", n: 1, timeout: -time.Second, fn: ok,
			want: "got n = 1, timeout = -1s and a nil function: false",
		},
		{
			name: "no function", n: 1, timeout: time.Second,
			want: "got n = 1, timeout = 1s and a nil function: true",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft := faketb.New(t)
			if ft.Run(func() { _ = Run(ft, tt.n, tt.timeout, tt.fn) }) {
				t.Error("Run returned, want it to end the test with Fatalf")
			}
			want := "concurrent: Run needs n of 1 or more, a positive timeout and a function; " +
				tt.want
			if got := ft.Take(); !slices.Equal(got, []string{want}) {
				t.Errorf("Run reported %q, want %q", got, want)
			}
		})
	}
}

// TestDrainReturned checks that drainReturned records every result that is waiting, in order,
// and returns at once when none is.
func TestDrainReturned(t *testing.T) {
	errFailed := errors.New("failed")
	results := make(chan result, 4)
	results <- result{i: 2}
	results <- result{i: 0, err: errFailed}
	results <- result{i: 3}
	var recorded []result
	record := func(r result) { recorded = append(recorded, r) }

	if got := drainReturned(results, record); got != 3 {
		t.Errorf("drainReturned() = %d with 3 results waiting, want 3", got)
	}
	if want := []result{{i: 2}, {i: 0, err: errFailed}, {i: 3}}; !slices.Equal(recorded, want) {
		t.Errorf("drainReturned recorded %v, want %v", recorded, want)
	}
	if got := drainReturned(results, record); got != 0 || len(recorded) != 3 {
		t.Errorf("drainReturned() = %d with no result waiting and recorded %v, want 0 and no "+
			"more", got, recorded)
	}
}

func TestRunning(t *testing.T) {
	tests := []struct {
		returned []bool
		want     string
	}{
		{returned: []bool{true, true}, want: ""},
		{returned: []bool{false}, want: "call 0"},
		{returned: []bool{true, false, true, false, false}, want: "call 1, call 3, call 4"},
	}
	for _, tt := range tests {
		if got := running(tt.returned); got != tt.want {
			t.Errorf("running(%v) = %q, want %q", tt.returned, got, tt.want)
		}
	}
}
