package framework

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// Ledger holds what a run creates, each object with its teardown: a removal and a check that the
// object is gone. A scenario registers an object before the request that creates it, so that an
// object whose create reply is lost is still torn down. It is safe for concurrent use.
type Ledger struct {
	mu      sync.Mutex
	entries []*entry
}

// entry is an object of a Ledger.
type entry struct {
	name      string
	remove    func(context.Context) error
	verify    func(context.Context) error
	completed bool
	verified  bool
}

// Register records the object name, such as environment gosdk-ci-1-1-cloud, with remove, which
// deletes it and succeeds when it is gone already, and verify, which checks that it is gone.
// Call it before the request that creates the object.
func (l *Ledger) Register(name string, remove, verify func(context.Context) error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, &entry{name: name, remove: remove, verify: verify})
}

// Teardown tears down the objects of l, the last registered first: it removes each object and
// checks that it is gone before it moves to the next one, so that an object is checked while the
// objects registered before it, such as the environment that holds it, still exist. It returns
// the errors of the removals and the checks, joined. An object whose removal failed is not
// checked, and an object is torn down at most once, however often Teardown runs.
func (l *Ledger) Teardown(ctx context.Context) error {
	l.mu.Lock()
	entries := l.entries
	l.mu.Unlock()
	var errs []error
	for _, e := range slices.Backward(entries) {
		if !l.done(e, false) {
			if err := e.remove(ctx); err != nil {
				errs = append(errs, fmt.Errorf("removing %s: %w", e.name, err))
				continue
			}
			l.mark(e, false)
		}
		if l.done(e, true) {
			continue
		}
		if err := e.verify(ctx); err != nil {
			errs = append(errs, fmt.Errorf("checking that %s is gone: %w", e.name, err))
			continue
		}
		l.mark(e, true)
	}
	return errors.Join(errs...)
}

// done reports whether e is removed, or verified when verified is true.
func (l *Ledger) done(e *entry, verified bool) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if verified {
		return e.verified
	}
	return e.completed
}

// mark marks e as removed, or verified when verified is true.
func (l *Ledger) mark(e *entry, verified bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if verified {
		e.verified = true
	} else {
		e.completed = true
	}
}

// Counts are the counts of a Ledger's objects.
type Counts struct {
	// Registered is the number of objects registered.
	Registered int

	// Completed is the number of objects whose removal succeeded.
	Completed int

	// Verified is the number of objects that were checked to be gone.
	Verified int
}

// String returns the counts as registered=N completed=N verified=N.
func (c Counts) String() string {
	return fmt.Sprintf("registered=%d completed=%d verified=%d", c.Registered, c.Completed,
		c.Verified)
}

// Counts returns the counts of l's objects.
func (l *Ledger) Counts() Counts {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := Counts{Registered: len(l.entries)}
	for _, e := range l.entries {
		if e.completed {
			c.Completed++
		}
		if e.verified {
			c.Verified++
		}
	}
	return c
}

// Reconcile returns nil when every object of l was removed and checked to be gone, so that its
// registered, completed and verified counts are equal, and else an error with the counts that
// names each object whose teardown did not complete or was not verified. A run whose ledger does
// not reconcile may have left objects behind, so its result is not valid.
func (l *Ledger) Reconcile() error {
	counts := l.Counts()
	l.mu.Lock()
	defer l.mu.Unlock()
	var left []string
	for _, e := range l.entries {
		switch {
		case !e.completed:
			left = append(left, e.name+" (not removed)")
		case !e.verified:
			left = append(left, e.name+" (removed, not checked to be gone)")
		}
	}
	if len(left) == 0 {
		return nil
	}
	return fmt.Errorf("the ledger does not reconcile, %s: %s", counts,
		strings.Join(left, ", "))
}
