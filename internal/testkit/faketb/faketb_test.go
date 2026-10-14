package faketb

import (
	"fmt"
	"slices"
	"sync"
	"testing"
)

func TestErrorfRecordsTheMessage(t *testing.T) {
	ft := New(t)
	if ft.Failed() {
		t.Error("Failed() = true before any message, want false")
	}

	ft.Errorf("a %s %d", "message", 1)
	ft.Errorf("another")

	if !ft.Failed() {
		t.Error("Failed() = false after Errorf, want true")
	}
	if got, want := ft.Take(), []string{"a message 1", "another"}; !slices.Equal(got, want) {
		t.Errorf("Take() = %q, want %q", got, want)
	}
	if got := ft.Take(); got != nil {
		t.Errorf("Take() = %q after a Take, want nothing", got)
	}
	if !ft.Failed() {
		t.Error("Failed() = false after Take, want it to stay true")
	}
}

func TestFatalfEndsTheGoroutine(t *testing.T) {
	ft := New(t)
	after := false

	returned := ft.Run(func() {
		ft.Fatalf("stop at %d", 1)
		after = true
	})

	if returned || after {
		t.Errorf("Run() = %t, and the code after Fatalf ran: %t; want false and false", returned,
			after)
	}
	if got, want := ft.Take(), []string{"stop at 1"}; !slices.Equal(got, want) {
		t.Errorf("Take() = %q, want %q", got, want)
	}
	if !ft.Failed() {
		t.Error("Failed() = false after Fatalf, want true")
	}
}

func TestRunReportsThatFnReturned(t *testing.T) {
	ft := New(t)
	ran := false

	if !ft.Run(func() { ran = true }) || !ran {
		t.Errorf("Run() = false, or fn did not run: %t; want true and true", ran)
	}
	if ft.Failed() || ft.Take() != nil {
		t.Error("Run recorded a failure for a function that returned")
	}
}

func TestEndRunsTheCleanupsLastFirst(t *testing.T) {
	ft := New(t)
	var ran []string
	ft.Cleanup(func() { ran = append(ran, "first") })
	ft.Cleanup(func() {
		ran = append(ran, "second")
		ft.Cleanup(func() { ran = append(ran, "registered by second") })
	})
	ft.Cleanup(func() { ran = append(ran, "third") })
	if len(ran) != 0 {
		t.Fatalf("the cleanup functions %q ran before End", ran)
	}

	ft.End()

	want := []string{"third", "second", "registered by second", "first"}
	if !slices.Equal(ran, want) {
		t.Errorf("End ran %q, want %q", ran, want)
	}
	ft.Cleanup(func() { ran = append(ran, "after End") })
	ft.End()
	if again := append(want, "after End"); !slices.Equal(ran, again) {
		t.Errorf("a second End ran %q, want %q", ran, again)
	}
}

// TestNewEndsWithTheTest checks that the end of the test runs the cleanup functions that End did
// not run, and none twice.
func TestNewEndsWithTheTest(t *testing.T) {
	var ran []string
	t.Run("test", func(t *testing.T) {
		ft := New(t)
		ft.Cleanup(func() { ran = append(ran, "before End") })
		ft.End()
		ft.Cleanup(func() { ran = append(ran, "after End") })
	})
	if want := []string{"before End", "after End"}; !slices.Equal(ran, want) {
		t.Errorf("the test ran the cleanup functions %q, want %q", ran, want)
	}
}

func TestHelperRecordsNothing(t *testing.T) {
	ft := New(t)
	ft.Helper()
	if ft.Failed() || ft.Take() != nil {
		t.Error("Helper recorded a failure")
	}
}

func TestOtherMethodsAreTheTests(t *testing.T) {
	ft := New(t)
	if ft.Name() != t.Name() {
		t.Errorf("Name() = %q, want the test's name %q", ft.Name(), t.Name())
	}
}

func TestIsSafeForConcurrentUse(t *testing.T) {
	ft := New(t)
	var mu sync.Mutex
	cleanups := 0
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			ft.Errorf("goroutine %d", i)
			ft.Cleanup(func() {
				mu.Lock()
				defer mu.Unlock()
				cleanups++
			})
		})
	}
	wg.Wait()
	ft.End()

	got := ft.Take()
	slices.Sort(got)
	var want []string
	for i := range 8 {
		want = append(want, fmt.Sprintf("goroutine %d", i))
	}
	if !slices.Equal(got, want) || cleanups != 8 {
		t.Errorf("Take() = %q and End ran %d cleanup functions, want %q and 8", got, cleanups,
			want)
	}
}
