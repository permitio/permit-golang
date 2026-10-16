package transport

import (
	"context"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"github.com/permitio/permit-golang/v2/internal/rest"
)

// The limits of the waits before a retry.
const (
	// BaseBackoff is the longest wait before the first retry of a call when the reply has no
	// Retry-After header. The longest wait doubles for each retry after the first, up to the
	// MaxBackoff of the Policy.
	BaseBackoff = 200 * time.Millisecond

	// MaxRetryAfter is the longest wait that a call makes for a Retry-After header. A reply that
	// asks for a longer wait ends the call.
	MaxRetryAfter = 60 * time.Second
)

// maxShift is the most times that backoffLimit doubles BaseBackoff, so that the limit fits a
// time.Duration: BaseBackoff doubled 32 times is more than 27 years.
const maxShift = 32

// maxSeconds is the most seconds that a time.Duration holds.
const maxSeconds = int64(math.MaxInt64 / time.Second)

// Policy decides whether a call sends its request again after an attempt that failed, and how
// long it waits before it does. The zero Policy sends each request once.
type Policy struct {
	// MaxRetries is how many times, at most, a call sends its request again after its first
	// attempt.
	MaxRetries int

	// MaxBackoff is the longest wait before a retry when the reply has no Retry-After header. It
	// is 0 or more.
	MaxBackoff time.Duration

	// Jitter returns a random wait from 0 to limit, both included, or is nil for a uniformly
	// random one. A test sets it to choose the waits.
	Jitter func(limit time.Duration) time.Duration
}

// Next returns how long a call of op waits before it sends its request again, after its attempt
// number attempt, from 1, got reply, or got no complete reply when sendErr is not nil; and false
// when the call ends with that attempt. A call sends its request again:
//
//   - after a reply with status 429, unless op is NoRetry;
//   - after a reply with status 500, 502, 503 or 504, or an attempt without a complete reply,
//     when op is Idempotent;
//
// while attempt is at most MaxRetries and ctx has not ended. The wait is what the reply's
// Retry-After header asks for, in seconds or as an HTTP-date, or, without one, a random wait
// from 0 to backoffLimit. The call ends instead when Retry-After asks for more than
// MaxRetryAfter, or when ctx's deadline comes before the wait would end.
func (p Policy) Next(ctx context.Context, op rest.Op, attempt int, reply Reply,
	sendErr error,
) (time.Duration, bool) {
	if attempt > p.MaxRetries || ctx.Err() != nil || !retryable(op, reply.Status, sendErr) {
		return 0, false
	}
	now := time.Now()
	wait, asked := retryAfter(reply.RetryAfter, now)
	if asked && wait > MaxRetryAfter {
		return 0, false
	}
	if !asked {
		wait = p.jitter(backoffLimit(attempt, p.MaxBackoff))
	}
	if deadline, ok := ctx.Deadline(); ok && !now.Add(wait).Before(deadline) {
		return 0, false
	}
	return wait, true
}

// retryable reports whether a call of op may send its request again after an attempt that got a
// reply with status, or got no complete reply when sendErr is not nil. A 429 means that the
// server refused the request without acting on it; after any other failure, the server may have
// acted on it, so only an Idempotent op is sent again.
func retryable(op rest.Op, status int, sendErr error) bool {
	if op.NoRetry {
		return false
	}
	if status == http.StatusTooManyRequests {
		return true
	}
	if sendErr != nil {
		return op.Idempotent
	}
	return op.Idempotent && serverFailure(status)
}

// serverFailure reports whether status says that the server, or a gateway on the way, failed in
// a way that may pass: 500, 502, 503 or 504.
func serverFailure(status int) bool {
	return status == http.StatusInternalServerError || status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

// retryAfter returns the wait that value, a Retry-After header, asks for at now, and true: a
// number of seconds, or an HTTP-date less now, which is 0 for a date before now. It returns
// false when value is neither, such as when it is empty or a negative number.
func retryAfter(value string, now time.Time) (time.Duration, bool) {
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		return time.Duration(min(seconds, maxSeconds)) * time.Second, true
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(date.Sub(now), 0), true
	}
	return 0, false
}

// backoffLimit returns the longest wait before the retry that follows attempt, from 1, when the
// reply has no Retry-After header: BaseBackoff, doubled for each attempt before attempt, and at
// most maxBackoff.
func backoffLimit(attempt int, maxBackoff time.Duration) time.Duration {
	return min(BaseBackoff<<min(attempt-1, maxShift), maxBackoff)
}

// jitter returns a random wait from 0 to limit, both included: what p.Jitter returns, or a
// uniformly random one when it is nil.
func (p Policy) jitter(limit time.Duration) time.Duration {
	if p.Jitter != nil {
		return p.Jitter(limit)
	}
	//nolint:gosec // G404: a wait between retries needs no cryptographic randomness.
	return time.Duration(rand.Int64N(int64(limit) + 1))
}

// Sleep waits for d, and returns nil; or returns the error of ctx as soon as ctx ends, when that
// comes first.
func Sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
