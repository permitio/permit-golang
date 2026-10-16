// Package leak fails a test that leaves goroutines running. It is for tests only.
//
// It is meant for tests that run real servers and connections outside a testing/synctest bubble:
// synctest.Test already waits for the goroutines of its bubble and reports those that cannot
// exit. It uses only the standard library, because go.uber.org/goleak requires testify, which
// the SDK keeps out of its module graph.
package leak

import (
	"fmt"
	"maps"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// grace is how long Check waits for goroutines that are still stopping, such as the
// connection goroutines of a server that was just closed.
const grace = 5 * time.Second

// poll is how often Check looks at the goroutines again while it waits.
const poll = 10 * time.Millisecond

// Check records the goroutines that are running now. When the test ends, it waits up to 5
// seconds for every goroutine started since to exit, then fails the test with the stack of each
// one still running. Call it first in a test, so that its check runs after the cleanup functions
// registered after it, such as those that stop servers and close connections.
//
// A test that calls Check must not run in parallel with other tests (t.Parallel): it would
// report their goroutines too.
func Check(tb testing.TB) {
	tb.Helper()
	check(tb, grace)
}

func check(tb testing.TB, wait time.Duration) {
	tb.Helper()
	before, err := goroutines()
	if err != nil {
		tb.Fatalf("leak: %v", err)
	}
	tb.Cleanup(func() {
		tb.Helper()
		deadline := time.Now().Add(wait)
		ticker := time.NewTicker(poll)
		defer ticker.Stop()
		for {
			leaked, err := startedSince(before)
			switch {
			case err != nil:
				tb.Errorf("leak: %v", err)
				return
			case len(leaked) == 0:
				return
			case time.Now().After(deadline):
				tb.Errorf("leak: %d goroutines that the test started still run %v after it "+
					"ended:\n\n%s", len(leaked), wait, strings.Join(leaked, "\n\n"))
				return
			}
			<-ticker.C
		}
	})
}

// startedSince returns the stack of each goroutine that is running now and not in before,
// ordered by goroutine ID.
func startedSince(before map[uint64]string) ([]string, error) {
	now, err := goroutines()
	if err != nil {
		return nil, err
	}
	var leaked []string
	for _, id := range slices.Sorted(maps.Keys(now)) {
		if _, ok := before[id]; !ok {
			leaked = append(leaked, now[id])
		}
	}
	return leaked, nil
}

// goroutines returns the stack of every goroutine, by goroutine ID.
func goroutines() (map[uint64]string, error) {
	buf := make([]byte, 1024)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return parseStacks(string(buf[:n]))
		}
		buf = make([]byte, 2*len(buf))
	}
}

// parseStacks splits the output of runtime.Stack with all goroutines into the stack of each, by
// goroutine ID. Each stack starts with a line such as "goroutine 7 [chan receive]:".
func parseStacks(dump string) (map[uint64]string, error) {
	stacks := map[uint64]string{}
	for stack := range strings.SplitSeq(strings.TrimSpace(dump), "\n\n") {
		header, _, _ := strings.Cut(stack, "\n")
		rest, ok := strings.CutPrefix(header, "goroutine ")
		idText, _, _ := strings.Cut(rest, " ")
		id, err := strconv.ParseUint(idText, 10, 64)
		if !ok || err != nil {
			return nil, fmt.Errorf("the stack dump has a stack that does not start with "+
				"\"goroutine <ID> \": %q", header)
		}
		stacks[id] = stack
	}
	return stacks, nil
}
