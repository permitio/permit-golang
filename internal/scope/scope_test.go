package scope

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// envScope is the scope that the lookups of the tests return.
var envScope = Scope{Project: "proj", Environment: "env"}

// errLookup is the error of a lookup that fails.
var errLookup = errors.New("the lookup failed")

// gate is a lookup that counts its calls and waits until the test releases it, or until its
// context ends.
type gate struct {
	calls   atomic.Int32
	release chan struct{}

	// results are what the calls return, in order; a call past the last returns the last.
	results []result

	mu sync.Mutex
	// contexts are the contexts of the calls.
	contexts []context.Context
}

// result is what one call of a gate returns.
type result struct {
	scope Scope
	err   error
}

func newGate(results ...result) *gate {
	return &gate{release: make(chan struct{}), results: results}
}

func (g *gate) lookup(ctx context.Context) (Scope, error) {
	n := int(g.calls.Add(1))
	g.mu.Lock()
	g.contexts = append(g.contexts, ctx)
	g.mu.Unlock()
	select {
	case <-g.release:
	case <-ctx.Done():
		return Scope{}, ctx.Err()
	}
	r := g.results[min(n, len(g.results))-1]
	return r.scope, r.err
}

// call is the result of one Get.
type call struct {
	scope Scope
	err   error
}

// getAsync calls r.Get(ctx) on a goroutine of the bubble, and returns the channel that its result
// arrives on.
func getAsync(ctx context.Context, r *Resolver) <-chan call {
	out := make(chan call, 1)
	go func() {
		s, err := r.Get(ctx)
		out <- call{s, err}
	}()
	return out
}

// TestGetLooksUpOnce starts 50 calls of Get on a Resolver that has not looked up yet, waits
// until each one waits, and checks that they share one lookup and each gets its scope.
func TestGetLooksUpOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := newGate(result{scope: envScope})
		r := New(g.lookup)
		const callers = 50
		results := make([]<-chan call, callers)
		for i := range callers {
			results[i] = getAsync(t.Context(), r)
		}
		synctest.Wait()
		if n := g.calls.Load(); n != 1 {
			t.Errorf("%d callers that came together made %d lookups, want 1", callers, n)
		}
		close(g.release)
		for i, c := range results {
			got := <-c
			if got.err != nil || got.scope != envScope {
				t.Errorf("call %d returned %+v, %v; want %+v", i, got.scope, got.err, envScope)
			}
		}
	})
}

// TestGetKeepsTheScope checks that the scope of a lookup that succeeded is returned from then on
// without another lookup, also to a caller whose context has ended.
func TestGetKeepsTheScope(t *testing.T) {
	g := newGate(result{scope: envScope})
	close(g.release)
	r := New(g.lookup)
	for i := range 3 {
		if got, err := r.Get(t.Context()); err != nil || got != envScope {
			t.Fatalf("Get %d returned %+v, %v; want %+v", i+1, got, err, envScope)
		}
	}
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	if got, err := r.Get(ended); err != nil || got != envScope {
		t.Errorf("Get with an ended context returned %+v, %v; want the scope it keeps", got, err)
	}
	if n := g.calls.Load(); n != 1 {
		t.Errorf("the Resolver looked up %d times, want once", n)
	}
}

// TestGetDoesNotKeepAFailure checks that the callers of a lookup that fails get its error, and
// that the next call looks up again.
func TestGetDoesNotKeepAFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := newGate(result{err: errLookup}, result{scope: envScope})
		r := New(g.lookup)
		first, second := getAsync(t.Context(), r), getAsync(t.Context(), r)
		synctest.Wait()
		close(g.release)
		for _, c := range []<-chan call{first, second} {
			if got := <-c; !errors.Is(got.err, errLookup) || got.scope != (Scope{}) {
				t.Errorf("a caller of the failed lookup got %+v, %v; want no scope and %v",
					got.scope, got.err, errLookup)
			}
		}
		if got, err := r.Get(t.Context()); err != nil || got != envScope {
			t.Errorf("Get after the failure returned %+v, %v; want %+v", got, err, envScope)
		}
		if n := g.calls.Load(); n != 2 {
			t.Errorf("the Resolver looked up %d times, want 2: the failure, and again", n)
		}
	})
}

// TestGetReturnsWhenItsContextEnds checks that a caller whose context ends stops waiting at once
// with the context's error, also the caller that started the lookup, and that the lookup runs on
// for the caller that still waits, under a context that did not end.
func TestGetReturnsWhenItsContextEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := newGate(result{scope: envScope})
		r := New(g.lookup)
		canceled, cancel := context.WithCancel(t.Context())
		expiring, stop := context.WithTimeout(t.Context(), time.Second)
		defer stop()
		starter := getAsync(canceled, r)
		synctest.Wait()
		late := getAsync(expiring, r)
		patient := getAsync(t.Context(), r)
		synctest.Wait()

		cancel()
		synctest.Wait()
		if got := <-starter; !errors.Is(got.err, context.Canceled) {
			t.Errorf("the caller whose context was canceled got %+v, %v; want context.Canceled",
				got.scope, got.err)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if got := <-late; !errors.Is(got.err, context.DeadlineExceeded) {
			t.Errorf("the caller whose deadline passed got %+v, %v; want "+
				"context.DeadlineExceeded", got.scope, got.err)
		}
		g.mu.Lock()
		lookupCtx := g.contexts[0]
		g.mu.Unlock()
		if err := lookupCtx.Err(); err != nil {
			t.Fatalf("the callers' contexts ended the lookup's: %v", err)
		}
		if _, ok := lookupCtx.Deadline(); ok {
			t.Error("the lookup's context has a deadline; the caller's must not bound it")
		}
		close(g.release)
		if got := <-patient; got.err != nil || got.scope != envScope {
			t.Errorf("the caller that waited got %+v, %v; want %+v", got.scope, got.err,
				envScope)
		}
		if n := g.calls.Load(); n != 1 {
			t.Errorf("the Resolver looked up %d times, want once", n)
		}
	})
}

// contextKey is the type of the key of the value that TestLookupKeepsTheValuesOfTheContext puts
// in a context.
type contextKey struct{}

// TestLookupKeepsTheValuesOfTheContext checks that the lookup's context holds the values of the
// context of the call that started it, such as a test's request recorder.
func TestLookupKeepsTheValuesOfTheContext(t *testing.T) {
	g := newGate(result{scope: envScope})
	close(g.release)
	r := New(g.lookup)
	ctx := context.WithValue(t.Context(), contextKey{}, "recorder")
	if _, err := r.Get(ctx); err != nil {
		t.Fatal(err)
	}
	if got := g.contexts[0].Value(contextKey{}); got != "recorder" {
		t.Errorf("the lookup's context holds %v under the caller's key, want recorder", got)
	}
}

// TestGetWithAnEndedContextStartsNoLookup checks that a caller whose context has ended starts no
// lookup: a lookup that Get started would be waiting in the gate, and counted, once every
// goroutine of the bubble waits.
func TestGetWithAnEndedContextStartsNoLookup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := newGate(result{scope: envScope})
		r := New(g.lookup)
		ended, cancel := context.WithCancel(t.Context())
		cancel()
		if got, err := r.Get(ended); !errors.Is(err, context.Canceled) || got != (Scope{}) {
			t.Errorf("Get with an ended context returned %+v, %v; want context.Canceled", got,
				err)
		}
		synctest.Wait()
		if n := g.calls.Load(); n != 0 {
			t.Errorf("Get with an ended context started %d lookups, want none", n)
		}
		close(g.release)
	})
}

// TestStopEndsTheLookup checks that Stop ends the context of the lookup in flight, whose error
// the callers that wait get, and of a lookup that starts after it.
func TestStopEndsTheLookup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := newGate(result{scope: envScope})
		r := New(g.lookup)
		waiting := getAsync(t.Context(), r)
		synctest.Wait()
		r.Stop()
		if got := <-waiting; !errors.Is(got.err, context.Canceled) {
			t.Errorf("the caller that waited at Stop got %+v, %v; want the lookup's "+
				"context.Canceled", got.scope, got.err)
		}
		if got, err := r.Get(t.Context()); !errors.Is(err, context.Canceled) {
			t.Errorf("Get after Stop returned %+v, %v; want a lookup whose context ended", got,
				err)
		}
		r.Stop()
		if n := g.calls.Load(); n != 2 {
			t.Errorf("the Resolver looked up %d times, want 2", n)
		}
	})
}

// TestStopWithoutALookup checks that Stop of a Resolver with no lookup in flight changes nothing
// that it knows.
func TestStopWithoutALookup(t *testing.T) {
	r := Known(envScope)
	r.Stop()
	if got, err := r.Get(t.Context()); err != nil || got != envScope {
		t.Errorf("Get after Stop returned %+v, %v; want %+v", got, err, envScope)
	}
}

// TestKnown checks that a Resolver that Known built returns its scope without a lookup, also to
// a caller whose context has ended, and that it can hold an empty environment.
func TestKnown(t *testing.T) {
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	for _, s := range []Scope{envScope, {Project: "proj"}} {
		r := Known(s)
		for _, ctx := range []context.Context{t.Context(), ended} {
			if got, err := r.Get(ctx); err != nil || got != s {
				t.Errorf("Get returned %+v, %v; want %+v", got, err, s)
			}
		}
	}
}
