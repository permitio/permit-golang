package transport

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
)

// The operations of the retry tests, with the facts of each class of retry.
var (
	// plainOp is an operation that the server may act on twice, such as a create.
	plainOp = rest.Op{ID: "create_thing", Method: http.MethodPost, Route: "/v2/things"}

	// idempotentOp does the same when it is sent twice, such as a GET or a decision.
	idempotentOp = rest.Op{
		ID: "get_thing", Method: http.MethodGet, Route: "/v2/things/{id}", Idempotent: true,
	}

	// noRetryOp is never sent twice, such as rotating an API key.
	noRetryOp = rest.Op{
		ID: "rotate_thing", Method: http.MethodPost, Route: "/v2/things/rotate", NoRetry: true,
	}
)

// errNoReply is the error of an attempt that got no complete reply.
var errNoReply = errors.New("connection reset")

// policy returns a Policy that retries up to 3 times and waits for the longest backoff.
func policy() Policy {
	return Policy{MaxRetries: 3, MaxBackoff: 5 * time.Second, Jitter: longest}
}

// longest is a Jitter that always waits for the limit.
func longest(limit time.Duration) time.Duration { return limit }

// TestNextRetriesByOperationAndStatus checks which attempts a call sends again, for an
// operation of each class: a 429 on every operation but a NoRetry one, whose server refused it;
// a 500, 502, 503 or 504 and an attempt without a complete reply on an Idempotent operation
// only; and no other status.
func TestNextRetriesByOperationAndStatus(t *testing.T) {
	type outcome struct {
		status  int
		sendErr error
	}
	retryOnAny := []outcome{{429, nil}, {429, errNoReply}}
	retryWhenIdempotent := []outcome{{0, errNoReply}, {200, errNoReply}, {404, errNoReply}}
	for _, status := range []int{500, 502, 503, 504} {
		retryWhenIdempotent = append(retryWhenIdempotent, outcome{status: status})
	}
	var never []outcome
	for _, status := range []int{
		200, 204, 301, 400, 401, 403, 404, 409, 422, 428, 430, 499, 501, 505, 511, 599,
	} {
		never = append(never, outcome{status: status})
	}
	for _, tc := range []struct {
		op   rest.Op
		want map[bool][]outcome
	}{
		{plainOp, map[bool][]outcome{
			true:  retryOnAny,
			false: append(append([]outcome{}, retryWhenIdempotent...), never...),
		}},
		{idempotentOp, map[bool][]outcome{
			true:  append(append([]outcome{}, retryOnAny...), retryWhenIdempotent...),
			false: never,
		}},
		{noRetryOp, map[bool][]outcome{
			false: append(append(append([]outcome{}, retryOnAny...), retryWhenIdempotent...),
				never...),
		}},
	} {
		for want, outcomes := range tc.want {
			for _, o := range outcomes {
				reply := Reply{Status: o.status}
				if _, got := policy().Next(t.Context(), tc.op, 1, reply, o.sendErr); got != want {
					t.Errorf("%s after status %d and the error %v: Next = %t, want %t", tc.op.ID,
						o.status, o.sendErr, got, want)
				}
			}
		}
	}
}

// TestNextStopsAfterMaxRetries checks that a call sends its request at most MaxRetries times
// after its first attempt, and only once with a MaxRetries of 0.
func TestNextStopsAfterMaxRetries(t *testing.T) {
	for _, maxRetries := range []int{0, 1, 3} {
		p := policy()
		p.MaxRetries = maxRetries
		for attempt := 1; attempt <= maxRetries+2; attempt++ {
			_, got := p.Next(t.Context(), idempotentOp, attempt, Reply{Status: 503}, nil)
			if want := attempt <= maxRetries; got != want {
				t.Errorf("MaxRetries %d, attempt %d: Next = %t, want %t", maxRetries, attempt,
					got, want)
			}
		}
	}
	if _, got := (Policy{}).Next(t.Context(), idempotentOp, 1, Reply{Status: 429}, nil); got {
		t.Error("the zero Policy sends a request again")
	}
}

// TestNextEndsWithTheContext checks that a call whose context ended, also one without a
// deadline, never sends its request again.
func TestNextEndsWithTheContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, status := range []int{429, 503} {
		if _, got := policy().Next(ctx, idempotentOp, 1, Reply{Status: status}, nil); got {
			t.Errorf("Next sends a request again after %d with a canceled context", status)
		}
	}
}

// TestNextWaitsForRetryAfter checks the wait that a Retry-After asks for: seconds up to
// MaxRetryAfter, also after a 503; and that a longer one ends the call.
func TestNextWaitsForRetryAfter(t *testing.T) {
	for _, tc := range []struct {
		status     int
		retryAfter string
		want       time.Duration
		wantRetry  bool
	}{
		{429, "0", 0, true},
		{429, "2", 2 * time.Second, true},
		{429, "60", MaxRetryAfter, true},
		{429, "61", 0, false},
		{429, "100000000000", 0, false},
		{503, "7", 7 * time.Second, true},
	} {
		reply := Reply{Status: tc.status, RetryAfter: tc.retryAfter}
		wait, retry := policy().Next(t.Context(), idempotentOp, 1, reply, nil)
		if wait != tc.want || retry != tc.wantRetry {
			t.Errorf("%d with Retry-After %q: Next = %v, %t; want %v, %t", tc.status,
				tc.retryAfter, wait, retry, tc.want, tc.wantRetry)
		}
	}
}

// TestNextBacksOff checks the wait without a Retry-After, or with one that is not valid: the
// jitter of the limit of the attempt, BaseBackoff doubled for each attempt before it, up to
// MaxBackoff.
func TestNextBacksOff(t *testing.T) {
	for _, tc := range []struct {
		maxBackoff time.Duration
		retryAfter string
		want       []time.Duration
	}{
		{
			5 * time.Second, "",
			[]time.Duration{BaseBackoff, 2 * BaseBackoff, 4 * BaseBackoff, 8 * BaseBackoff},
		},
		{300 * time.Millisecond, "soon", []time.Duration{BaseBackoff, 300 * time.Millisecond}},
		{100 * time.Millisecond, "-1", []time.Duration{100 * time.Millisecond}},
		{0, "1.5", []time.Duration{0}},
	} {
		var limits []time.Duration
		record := func(limit time.Duration) time.Duration {
			limits = append(limits, limit)
			return limit / 2
		}
		p := Policy{MaxRetries: 10, MaxBackoff: tc.maxBackoff, Jitter: record}
		for i, want := range tc.want {
			reply := Reply{Status: 503, RetryAfter: tc.retryAfter}
			wait, retry := p.Next(t.Context(), idempotentOp, i+1, reply, nil)
			if len(limits) <= i {
				t.Fatalf("MaxBackoff %v, attempt %d: Next did not call Jitter", tc.maxBackoff, i+1)
			}
			if !retry || wait != want/2 || limits[i] != want {
				t.Errorf("MaxBackoff %v, attempt %d: Next = %v, %t with the limit %v; want %v "+
					"with the limit %v", tc.maxBackoff, i+1, wait, retry, limits[i], want/2, want)
			}
		}
	}
}

// TestNextEndsBeforeTheDeadline checks that a call does not wait for a retry that would come at
// or after its deadline, for a Retry-After and for a backoff, and does wait for one before it.
func TestNextEndsBeforeTheDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		for _, tc := range []struct {
			retryAfter string
			maxBackoff time.Duration
			want       bool
		}{
			{"1", 0, true},
			{"2", 0, false},
			{"3", 0, false},
			// The tenth attempt waits for MaxBackoff, as Jitter is longest.
			{"", 2*time.Second - time.Nanosecond, true},
			{"", 2 * time.Second, false},
		} {
			p := Policy{MaxRetries: 10, MaxBackoff: tc.maxBackoff, Jitter: longest}
			reply := Reply{Status: 429, RetryAfter: tc.retryAfter}
			if _, got := p.Next(ctx, plainOp, 10, reply, nil); got != tc.want {
				t.Errorf("Retry-After %q, MaxBackoff %v with 2s left: Next = %t, want %t",
					tc.retryAfter, tc.maxBackoff, got, tc.want)
			}
		}
	})
}

// TestRetryAfter checks how a Retry-After header reads: seconds, the three forms of an
// HTTP-date, a date in the past as no wait, and a value of neither form as no header.
func TestRetryAfter(t *testing.T) {
	now := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		value  string
		want   time.Duration
		wantOK bool
	}{
		{"", 0, false},
		{"0", 0, true},
		{"2", 2 * time.Second, true},
		{"120", 2 * time.Minute, true},
		{strconv.FormatInt(math.MaxInt64, 10), time.Duration(maxSeconds) * time.Second, true},
		{"99999999999999999999", 0, false},
		{"-1", 0, false},
		{"1.5", 0, false},
		{"soon", 0, false},
		{"Sat, 01 Jan 2000 00:00:03 GMT", 3 * time.Second, true},
		{"Saturday, 01-Jan-00 00:00:05 GMT", 5 * time.Second, true},
		{"Sat Jan  1 00:00:07 2000", 7 * time.Second, true},
		{"Fri, 31 Dec 1999 23:59:00 GMT", 0, true},
		{"Sat, 01 Jan 2000 00:00:00 GMT", 0, true},
	} {
		got, ok := retryAfter(tc.value, now)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("retryAfter(%q) = %v, %t; want %v, %t", tc.value, got, ok, tc.want,
				tc.wantOK)
		}
	}
}

// TestBackoffLimit checks the limit of each wait: BaseBackoff, doubled for each attempt before,
// up to the most a Duration can double it, and at most maxBackoff.
func TestBackoffLimit(t *testing.T) {
	for _, tc := range []struct {
		attempt    int
		maxBackoff time.Duration
		want       time.Duration
	}{
		{1, 5 * time.Second, 200 * time.Millisecond},
		{2, 5 * time.Second, 400 * time.Millisecond},
		{5, 5 * time.Second, 3200 * time.Millisecond},
		{6, 5 * time.Second, 5 * time.Second},
		{1, 50 * time.Millisecond, 50 * time.Millisecond},
		{1, 0, 0},
		{33, math.MaxInt64, BaseBackoff << 32},
		{1000, math.MaxInt64, BaseBackoff << 32},
	} {
		if got := backoffLimit(tc.attempt, tc.maxBackoff); got != tc.want {
			t.Errorf("backoffLimit(%d, %v) = %v, want %v", tc.attempt, tc.maxBackoff, got, tc.want)
		}
	}
}

// TestJitter checks the random wait of a Policy without a Jitter: from 0 to the limit, both
// included, and spread over it.
func TestJitter(t *testing.T) {
	var p Policy
	if got := p.jitter(0); got != 0 {
		t.Errorf("jitter(0) = %v, want 0", got)
	}
	if got := p.jitter(1); got != 0 && got != 1 {
		t.Errorf("jitter(1ns) = %v, want 0 or 1ns", got)
	}
	const limit = 200 * time.Millisecond
	lowest, highest := time.Duration(math.MaxInt64), time.Duration(0)
	for range 1000 {
		got := p.jitter(limit)
		if got < 0 || got > limit {
			t.Fatalf("jitter(%v) = %v, want a wait from 0 to the limit", limit, got)
		}
		lowest, highest = min(lowest, got), max(highest, got)
	}
	if lowest > limit/4 || highest < limit*3/4 {
		t.Errorf("1000 waits are from %v to %v, want them spread from 0 to %v", lowest, highest,
			limit)
	}
}

// TestSleep checks that Sleep waits for its time, and returns the error of a context that ends
// first at once.
func TestSleep(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		if err := Sleep(t.Context(), 3*time.Second); err != nil ||
			time.Since(start) != 3*time.Second {
			t.Errorf("Sleep(3s) returned %v after %v, want nil after 3s", err, time.Since(start))
		}
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(time.Second, cancel)
		start = time.Now()
		if err := Sleep(ctx, time.Minute); !errors.Is(err, context.Canceled) ||
			time.Since(start) != time.Second {
			t.Errorf("Sleep(1m) with a context canceled after 1s returned %v after %v, want "+
				"context.Canceled after 1s", err, time.Since(start))
		}
	})
}

// TestSendReadsRetryAfter checks that Send returns the Retry-After header of a reply, and an
// empty one for a reply without it.
func TestSendReadsRetryAfter(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/limited" {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
		}
	})
	s := newSender(t, canary.Key("retryafter"))
	if reply := mustSend(t, s, get(srv, "/limited", false)); reply.RetryAfter != "7" {
		t.Errorf("RetryAfter = %q, want 7", reply.RetryAfter)
	}
	if reply := mustSend(t, s, get(srv, "/plain", false)); reply.RetryAfter != "" {
		t.Errorf("RetryAfter = %q, want none", reply.RetryAfter)
	}
}
