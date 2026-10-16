// Package scope resolves the scope of a client's API key once: the project and the environment
// that the client's calls act on. A [Resolver] looks the scope up with the first call that needs
// it, lets the calls that need it meanwhile wait for that one lookup, and keeps the scope of the
// first lookup that succeeds for its life. A lookup that fails is not kept, so the next call
// looks up again.
//
// The lookup runs on a goroutine of its own, under a context that keeps the values of the call
// that started it but not its cancellation or its deadline, so that a caller whose context ends
// stops waiting at once and leaves the lookup to the callers that still wait. [Resolver.Stop]
// ends the lookup in flight.
package scope

import (
	"context"
	"sync"
)

// Scope is the project and the environment that a client's calls act on, each a key or an ID,
// or empty when the client has none.
type Scope struct {
	// Project is the key or the ID of the project.
	Project string

	// Environment is the key or the ID of the environment.
	Environment string
}

// Resolver looks up the scope of a client, at most one lookup at a time, and keeps the scope of
// the first one that succeeds. Build one with [New], or with [Known] for a scope that needs no
// lookup. It is safe for concurrent use.
type Resolver struct {
	// lookup looks the scope up, or returns why it could not; it is nil in a Resolver that
	// Known built.
	lookup func(ctx context.Context) (Scope, error)

	// mu guards the fields below.
	mu sync.Mutex

	// known says that scope holds the scope, which Get returns from then on.
	known bool
	scope Scope

	// flight is the lookup in flight, or nil.
	flight *flight

	// stopped says that Stop was called: a lookup that starts after it runs under a context that
	// has ended.
	stopped bool
}

// flight is one lookup of the scope, which every caller of Get that comes while it runs waits
// for.
type flight struct {
	// done is closed when the lookup has returned, after scope and err are set.
	done chan struct{}

	// cancel ends the context of the lookup.
	cancel context.CancelFunc

	// scope and err are what the lookup returned.
	scope Scope
	err   error
}

// New returns a Resolver that looks the scope up with lookup. lookup returns the scope, or an
// error that Get returns to the callers that waited for it.
func New(lookup func(ctx context.Context) (Scope, error)) *Resolver {
	return &Resolver{lookup: lookup}
}

// Known returns a Resolver whose Get returns s and never looks it up.
func Known(s Scope) *Resolver {
	return &Resolver{known: true, scope: s}
}

// Get returns the scope. When the Resolver knows it, Get returns it at once, also when ctx has
// ended. Otherwise Get waits for the lookup in flight, or starts one, and returns what it
// returned; but when ctx ends first, Get returns the error of ctx at once, and the lookup runs on
// for the callers that still wait for it. A lookup runs under a context that keeps the values of
// ctx of the call that starts it, but not its cancellation or its deadline, and ends at Stop.
// When ctx has ended already and no lookup is in flight, Get returns the error of ctx and starts
// none.
func (r *Resolver) Get(ctx context.Context) (Scope, error) {
	r.mu.Lock()
	if r.known {
		s := r.scope
		r.mu.Unlock()
		return s, nil
	}
	f := r.flight
	if f == nil {
		if err := ctx.Err(); err != nil {
			r.mu.Unlock()
			return Scope{}, err
		}
		f = r.start(ctx)
	}
	r.mu.Unlock()
	select {
	case <-f.done:
		return f.scope, f.err
	case <-ctx.Done():
		return Scope{}, ctx.Err()
	}
}

// start starts a lookup under a context with the values of ctx, and returns its flight. The
// caller holds r.mu.
func (r *Resolver) start(ctx context.Context) *flight {
	lookupCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	if r.stopped {
		cancel()
	}
	f := &flight{done: make(chan struct{}), cancel: cancel}
	r.flight = f
	go r.run(lookupCtx, f)
	return f
}

// run runs the lookup of f, keeps its scope when it succeeds, and then lets the callers that
// wait for f have its result.
func (r *Resolver) run(ctx context.Context, f *flight) {
	s, err := r.lookup(ctx)
	f.cancel()
	r.mu.Lock()
	if err == nil {
		r.known, r.scope = true, s
	}
	r.flight = nil
	r.mu.Unlock()
	f.scope, f.err = s, err
	close(f.done)
}

// Stop ends the context of the lookup in flight, and of every lookup that starts after it, so
// that no lookup keeps a goroutine running after the client is closed. The callers that wait for
// a lookup get what it returns once its context has ended. Stop is safe to call more than once.
func (r *Resolver) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	if r.flight != nil {
		r.flight.cancel()
	}
}
