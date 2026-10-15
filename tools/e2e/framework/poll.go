package framework

import (
	"context"
	"fmt"
	"time"
)

// Poll calls try until it reports done, it returns an error, or timeout has passed since Poll
// started, and waits interval, or what is left of timeout when less, between two calls. Each try
// gets a context that ends when timeout has passed, so the elapsed time bounds Poll, a slow try
// included, and not a count of tries, and no fixed sleep stands in for a check. It returns nil
// when try reported done; an error that gives timeout, the number of tries and the error of the
// last try, if any, when time ran out; and else the error of try or of ctx.
func Poll(ctx context.Context, timeout, interval time.Duration,
	try func(context.Context) (bool, error),
) error {
	deadline := time.Now().Add(timeout)
	bounded, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	for tries := 1; ; tries++ {
		done, err := try(bounded)
		timedOut := ctx.Err() == nil && !time.Now().Before(deadline)
		switch {
		case done && err == nil:
			return nil
		case timedOut && err != nil:
			return fmt.Errorf("framework: not done after %s (%d tries): %w", timeout, tries, err)
		case timedOut:
			return fmt.Errorf("framework: not done after %s (%d tries)", timeout, tries)
		case err != nil:
			return err
		}
		if err := sleep(ctx, min(interval, time.Until(deadline))); err != nil {
			return fmt.Errorf("framework: polling: %w", err)
		}
	}
}
