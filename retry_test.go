package permit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// The test-only operations of the retry tests, beyond testOps: a POST that is Idempotent, as a
// decision of the PDP is, and one that is NoRetry, as rotating an API key is.
var (
	opDecide = rest.Op{
		ID: "test_decide", Method: http.MethodPost, Route: rest.TestPrefix + "decide",
		Idempotent: true, KeyLevel: rest.EnvironmentLevel, RequestModel: "TestObject",
		ResponseModel: "TestObject",
	}
	opRotate = rest.Op{
		ID: "test_rotate", Method: http.MethodPost, Route: rest.TestPrefix + "rotate",
		NoRetry: true, KeyLevel: rest.EnvironmentLevel, RequestModel: "TestObject",
		ResponseModel: "TestObject",
	}
)

// sentBody is the body of every request with a body in the retry tests.
const sentBody = `{"key":"a"}`

// step is how a scriptedTransport answers one request: with a reply, or with err and no reply.
type step struct {
	status int

	// retryAfter is the Retry-After header of the reply, or empty for none.
	retryAfter string

	// date, when it is not 0, makes Retry-After the HTTP-date this long after the request came.
	date time.Duration

	body string

	err error
}

// errDial is the error of a request that a scriptedTransport answers without a reply.
var errDial = errors.New("dial tcp: connection refused")

// scriptedTransport is an http.RoundTripper that answers the requests sent through it with its
// steps, in order, without a connection, so that a test can run in a synctest bubble, whose clock
// moves only when every goroutine in it waits. It records when each request came, its body, and
// the bodies of its replies.
type scriptedTransport struct {
	steps []step

	mu       sync.Mutex
	arrivals []time.Time
	bodies   []string
	replies  []*trackedBody
}

func (s *scriptedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		var err error
		if body, err = io.ReadAll(req.Body); err != nil {
			return nil, err
		}
		if err = req.Body.Close(); err != nil {
			return nil, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.arrivals)
	s.arrivals = append(s.arrivals, time.Now())
	s.bodies = append(s.bodies, string(body))
	if n >= len(s.steps) {
		return nil, fmt.Errorf("the test answers %d requests, and this is request %d",
			len(s.steps), n+1)
	}
	st := s.steps[n]
	if st.err != nil {
		return nil, st.err
	}
	header := http.Header{}
	if st.retryAfter != "" {
		header.Set("Retry-After", st.retryAfter)
	}
	if st.date != 0 {
		header.Set("Retry-After", time.Now().Add(st.date).UTC().Format(http.TimeFormat))
	}
	reply := &trackedBody{r: strings.NewReader(st.body)}
	s.replies = append(s.replies, reply)
	return &http.Response{
		StatusCode:    st.status,
		Header:        header,
		Body:          reply,
		ContentLength: int64(len(st.body)),
		Request:       req,
	}, nil
}

// waits returns how long after start each request came.
func (s *scriptedTransport) waits(start time.Time) []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	waits := make([]time.Duration, 0, len(s.arrivals))
	for _, at := range s.arrivals {
		waits = append(waits, at.Sub(start))
	}
	return waits
}

// checkCalls checks that the transport got want requests of op, each with the same body, the
// body of requestOf(op), and that the client read each reply to its end and closed it, so that
// a connection would carry the next request.
func (s *scriptedTransport) checkCalls(t *testing.T, op rest.Op, want int) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.arrivals) != want {
		t.Errorf("the client sent %d requests, want %d", len(s.arrivals), want)
	}
	wantBody := sentBody
	if op.Method == http.MethodGet {
		wantBody = ""
	}
	for i, body := range s.bodies {
		if body != wantBody {
			t.Errorf("request %d has the body %q, want %q in each attempt", i+1, body, wantBody)
		}
	}
	for i, reply := range s.replies {
		if !reply.eof || reply.closes != 1 {
			t.Errorf("reply %d: read to its end %t, closed %d times; want read and closed once",
				i+1, reply.eof, reply.closes)
		}
	}
}

// trackedBody is the body of a reply that records whether it was read to its end and closed.
type trackedBody struct {
	r      io.Reader
	eof    bool
	closes int
}

func (b *trackedBody) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if errors.Is(err, io.EOF) {
		b.eof = true
	}
	return n, err
}

func (b *trackedBody) Close() error {
	b.closes++
	return nil
}

// scriptedClient returns a test client with testOps, opDecide and opRotate, and opts, that sends
// its requests through a scriptedTransport with steps, and the transport.
func scriptedClient(t *testing.T, steps []step, opts ...Option) (*Client, *scriptedTransport) {
	t.Helper()
	clearEnv(t)
	rt := &scriptedTransport{steps: steps}
	base := []Option{
		WithAPIKey(canary.Key("retry")), WithAPIURL(codeURL), WithPDPURL(codeURL),
		WithHTTPClient(&http.Client{Transport: rt}),
	}
	ops := append(slices.Clone(testOps), opDecide, opRotate)
	return newTestClient(t, append(base, opts...), ops...), rt
}

// requestOf returns the request of a call of op with sentBody, or with no body for a GET.
func requestOf(op rest.Op) request {
	r := request{op: op.ID, target: apiTarget}
	if op.Method == http.MethodGet {
		r.params = []string{"a"}
		return r
	}
	r.body = testObject{Key: "a"}
	return r
}

// checkElapsed checks that a call that started at start took want of the bubble's time.
func checkElapsed(t *testing.T, start time.Time, want time.Duration) {
	t.Helper()
	if got := time.Since(start); got != want {
		t.Errorf("the call took %v, want %v", got, want)
	}
}

// The steps of the retry tests.
var (
	replyCreated     = step{status: http.StatusCreated, body: sentBody}
	replyOK          = step{status: http.StatusOK, body: sentBody}
	replyLimited     = step{status: http.StatusTooManyRequests, body: `{"message":"slow down"}`}
	replyUnavailable = step{status: http.StatusServiceUnavailable, body: `{"message":"later"}`}
	noReply          = step{err: errDial}
)

// limitedFor returns a 429 whose Retry-After asks for seconds.
func limitedFor(seconds string) step {
	s := replyLimited
	s.retryAfter = seconds
	return s
}

// longestJitter is a Jitter that always waits for the limit.
func longestJitter(limit time.Duration) time.Duration { return limit }

// TestRetryARateLimitedCreate checks that a 429 to a create, an operation that is not
// Idempotent, is sent again after the Retry-After of the reply, with the same body, until it
// succeeds: 3 requests and exactly 4 seconds for two 429s that ask for 2 seconds each.
func TestRetryARateLimitedCreate(t *testing.T) {
	c, rt := scriptedClient(t, []step{limitedFor("2"), limitedFor("2"), replyCreated})
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		got, err := do[testObject](t.Context(), c, requestOf(opCreate))
		if err != nil || got.Key != "a" {
			t.Fatalf("do returned %v, %v; want the created object", got, err)
		}
		checkElapsed(t, start, 4*time.Second)
		want := []time.Duration{0, 2 * time.Second, 4 * time.Second}
		if got := rt.waits(start); !slices.Equal(got, want) {
			t.Errorf("the requests came at %v, want %v", got, want)
		}
		rt.checkCalls(t, opCreate, 3)
	})
}

// TestRetryAfterAnHTTPDate checks a Retry-After that is an HTTP-date: the call waits until it,
// and sends the request again at once after a date in the past.
func TestRetryAfterAnHTTPDate(t *testing.T) {
	inThree, past := replyLimited, replyLimited
	inThree.date, past.date = 3*time.Second, -time.Minute
	c, rt := scriptedClient(t, []step{inThree, past, replyCreated})
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		if _, err := do[testObject](t.Context(), c, requestOf(opCreate)); err != nil {
			t.Fatal(err)
		}
		checkElapsed(t, start, 3*time.Second)
		rt.checkCalls(t, opCreate, 3)
	})
}

// TestACreateIsNeverSentTwice checks that a request to an operation that is not Idempotent,
// such as a create that the server may have acted on, is sent once after a 500, 502, 503 or
// 504, and after an attempt without a reply: the call returns the error of that attempt at once.
func TestACreateIsNeverSentTwice(t *testing.T) {
	for _, tc := range []struct {
		name  string
		first step
	}{
		{"500", step{status: 500}},
		{"502", step{status: 502}},
		{"503", replyUnavailable},
		{"504", step{status: 504}},
		{"no reply", noReply},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, rt := scriptedClient(t, []step{tc.first, replyCreated})
			var err error
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				_, err = do[testObject](t.Context(), c, requestOf(opCreate))
				rt.checkCalls(t, opCreate, 1)
				checkElapsed(t, start, 0)
			})
			if e := mustError(t, err); e.StatusCode != tc.first.status {
				t.Errorf("do returned %v, want the error of the first attempt", err)
			}
		})
	}
}

// TestRetryAnIdempotentCall checks that a request to an Idempotent operation, a GET or a POST
// that changes nothing, such as a decision of the PDP, is sent again after a 500, 502, 503 or
// 504, after an attempt without a reply and after a 429, with the random wait of the backoff
// before each retry: from 0 to 200ms before the first, and from 0 to 400ms before the second.
func TestRetryAnIdempotentCall(t *testing.T) {
	for _, tc := range []struct {
		name  string
		op    rest.Op
		steps []step
	}{
		{"GET 502 503", opGet, []step{{status: 502}, replyUnavailable, replyOK}},
		{"GET 500 504", opGet, []step{{status: 500}, {status: 504}, replyOK}},
		{"GET without a reply", opGet, []step{noReply, noReply, replyOK}},
		{"POST 503", opDecide, []step{replyUnavailable, replyOK}},
		{"POST 429", opDecide, []step{replyLimited, replyOK}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, rt := scriptedClient(t, tc.steps)
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				if _, err := do[testObject](t.Context(), c, requestOf(tc.op)); err != nil {
					t.Fatal(err)
				}
				waits := rt.waits(start)
				for i := 1; i < len(waits); i++ {
					limit := 200 * time.Millisecond << (i - 1)
					if gap := waits[i] - waits[i-1]; gap < 0 || gap > limit {
						t.Errorf("wait %d is %v, want from 0 to %v", i, gap, limit)
					}
				}
				rt.checkCalls(t, tc.op, len(tc.steps))
			})
		})
	}
}

// TestRetryBacksOff checks the waits without a Retry-After, with a Jitter that waits for the
// limit: 200ms, then twice as long before each retry, up to MaxBackoff; and that a Retry-After
// that is not valid counts as none.
func TestRetryBacksOff(t *testing.T) {
	const ms = time.Millisecond
	doubling := []time.Duration{200 * ms, 400 * ms, 800 * ms}
	for _, tc := range []struct {
		name       string
		maxBackoff time.Duration
		retryAfter string
		want       []time.Duration
	}{
		{"the default", 0, "", doubling},
		{"a MaxBackoff of 300ms", 300 * ms, "", []time.Duration{200 * ms, 300 * ms, 300 * ms}},
		{"a Retry-After that is not valid", 0, "soon", doubling},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failed := replyUnavailable
			failed.retryAfter = tc.retryAfter
			c, rt := scriptedClient(t, []step{failed, failed, failed, replyOK},
				WithRetry(RetryPolicy{MaxRetries: 3, MaxBackoff: tc.maxBackoff}))
			c.retry.Jitter = longestJitter
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				if _, err := do[testObject](t.Context(), c, requestOf(opGet)); err != nil {
					t.Fatal(err)
				}
				waits := rt.waits(start)
				for i, want := range tc.want {
					if gap := waits[i+1] - waits[i]; gap != want {
						t.Errorf("wait %d is %v, want %v", i+1, gap, want)
					}
				}
				rt.checkCalls(t, opGet, 4)
			})
		})
	}
}

// TestRetryUpToMaxRetries checks that a call sends its request at most MaxRetries times after
// the first, 3 by default, and returns the error of its last attempt; and that a MaxRetries of 0
// sends each request once, also after a 429.
func TestRetryUpToMaxRetries(t *testing.T) {
	once := WithRetry(RetryPolicy{})
	for _, tc := range []struct {
		name  string
		opts  []Option
		steps []step
		want  int
	}{
		{"the default", nil, slices.Repeat([]step{replyUnavailable}, 5), 4},
		{
			"1",
			[]Option{WithRetry(RetryPolicy{MaxRetries: 1})},
			[]step{replyUnavailable, replyUnavailable, replyOK},
			2,
		},
		{"0 after a 503", []Option{once}, []step{replyUnavailable, replyOK}, 1},
		{"0 after a 429", []Option{once}, []step{limitedFor("1"), replyOK}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, rt := scriptedClient(t, tc.steps, tc.opts...)
			synctest.Test(t, func(t *testing.T) {
				_, err := do[testObject](t.Context(), c, requestOf(opGet))
				if e := mustError(t, err); e.StatusCode != tc.steps[0].status {
					t.Errorf("do returned %v, want the error of the last attempt", err)
				}
				rt.checkCalls(t, opGet, tc.want)
			})
		})
	}
}

// TestNoRetryIsSentOnce checks that a request to a NoRetry operation, such as rotating an API
// key, is sent once, also after a 429 or a 503.
func TestNoRetryIsSentOnce(t *testing.T) {
	for _, tc := range []struct {
		name  string
		first step
	}{
		{"429 with Retry-After", limitedFor("1")},
		{"429", replyLimited},
		{"503", replyUnavailable},
		{"no reply", noReply},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, rt := scriptedClient(t, []step{tc.first, replyCreated})
			var err error
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				_, err = do[testObject](t.Context(), c, requestOf(opRotate))
				rt.checkCalls(t, opRotate, 1)
				checkElapsed(t, start, 0)
			})
			if e := mustError(t, err); e.StatusCode != tc.first.status {
				t.Errorf("do returned %v, want the error of the first attempt", err)
			}
		})
	}
}

// TestRetryNeverWaitsPastTheDeadline checks that a call whose deadline comes before the wait
// for its next attempt ends returns the error of its last attempt at once: a 429 that asks for
// 5 seconds with 3 seconds left, from the context or from WithTimeout, matches ErrRateLimited at
// once, as does a 429 that asks for more than 60 seconds; and a backoff past the deadline ends
// the call too.
func TestRetryNeverWaitsPastTheDeadline(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    []Option
		timeout time.Duration
		first   step
	}{
		{"a context's deadline", nil, 3 * time.Second, limitedFor("5")},
		{"WithTimeout", []Option{WithTimeout(3 * time.Second)}, 0, limitedFor("5")},
		{"a Retry-After over 60s", nil, 5 * time.Minute, limitedFor("61")},
		{"a backoff", []Option{WithTimeout(150 * time.Millisecond)}, 0, replyLimited},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, rt := scriptedClient(t, []step{tc.first, replyCreated}, tc.opts...)
			c.retry.Jitter = longestJitter
			synctest.Test(t, func(t *testing.T) {
				ctx := t.Context()
				if tc.timeout != 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, tc.timeout)
					defer cancel()
				}
				start := time.Now()
				_, err := do[testObject](ctx, c, requestOf(opCreate))
				if !errors.Is(err, ErrRateLimited) || errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("do returned %v, want the 429 at once", err)
				}
				checkElapsed(t, start, 0)
				rt.checkCalls(t, opCreate, 1)
			})
		})
	}
}

// TestRetryReturnsTheLastError checks that a call that fails after a retry returns the error of
// its last attempt, not of its first.
func TestRetryReturnsTheLastError(t *testing.T) {
	notFound := step{status: http.StatusNotFound, body: `{"message":"no such object"}`}
	t.Run("503 then 404", func(t *testing.T) {
		c, rt := scriptedClient(t, []step{replyUnavailable, notFound})
		c.retry.Jitter = longestJitter
		synctest.Test(t, func(t *testing.T) {
			_, err := do[testObject](t.Context(), c, requestOf(opGet))
			e, ok := errors.AsType[*Error](err)
			if !ok || e.StatusCode != http.StatusNotFound || !errors.Is(err, ErrNotFound) {
				t.Errorf("do returned %v, want the 404 of the last attempt", err)
			}
			rt.checkCalls(t, opGet, 2)
		})
	})
	t.Run("503 then a 429 past the deadline", func(t *testing.T) {
		c, rt := scriptedClient(t, []step{replyUnavailable, limitedFor("5")})
		c.retry.Jitter = func(time.Duration) time.Duration { return 0 }
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			_, err := do[testObject](ctx, c, requestOf(opGet))
			if !errors.Is(err, ErrRateLimited) {
				t.Errorf("do returned %v, want the 429 of the last attempt", err)
			}
			rt.checkCalls(t, opGet, 2)
		})
	})
}

// TestRetryStopsAtClose checks that a call that waits to retry when the client is closed sends no
// other request and returns the error of its last attempt.
func TestRetryStopsAtClose(t *testing.T) {
	c, rt := scriptedClient(t, []step{replyUnavailable, replyOK})
	c.retry.Jitter = longestJitter
	synctest.Test(t, func(t *testing.T) {
		done := make(chan error, 1)
		go func() {
			_, err := do[testObject](t.Context(), c, requestOf(opGet))
			done <- err
		}()
		synctest.Wait()
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		err := <-done
		e, ok := errors.AsType[*Error](err)
		if !ok || e.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("do returned %v, want the 503 of its only attempt", err)
		}
		rt.checkCalls(t, opGet, 1)
	})
}

// TestRetryWaitsUpTo60Seconds checks that a call waits for a Retry-After of 60 seconds, the
// longest it waits for, when its deadline is later.
func TestRetryWaitsUpTo60Seconds(t *testing.T) {
	c, rt := scriptedClient(t, []step{limitedFor("60"), replyCreated})
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
		defer cancel()
		start := time.Now()
		if _, err := do[testObject](ctx, c, requestOf(opCreate)); err != nil {
			t.Fatal(err)
		}
		checkElapsed(t, start, time.Minute)
		rt.checkCalls(t, opCreate, 2)
	})
}

// TestRetryWaitEndsWithTheContext checks that a call whose context is canceled while it waits to
// send its request again returns at once, with a *Error that matches context.Canceled.
func TestRetryWaitEndsWithTheContext(t *testing.T) {
	c, rt := scriptedClient(t, []step{limitedFor("10"), replyCreated})
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		time.AfterFunc(time.Second, cancel)
		start := time.Now()
		_, err := do[testObject](ctx, c, requestOf(opCreate))
		if e := mustError(t, err); !errors.Is(err, context.Canceled) || e.StatusCode != 0 ||
			e.Op != opCreate.ID {
			t.Errorf("do returned %v, want a *Error of %s that matches context.Canceled", err,
				opCreate.ID)
		}
		checkElapsed(t, start, time.Second)
		rt.checkCalls(t, opCreate, 1)
	})
}

// TestRetryDebugLines checks the Debug line of each attempt of a call that retries: the
// attempt's number from 1, its status, and the error of each attempt that failed.
func TestRetryDebugLines(t *testing.T) {
	logger, logged := debugLog()
	c, _ := scriptedClient(t, []step{{status: 502}, noReply, replyUnavailable, replyOK},
		WithLogger(logger))
	synctest.Test(t, func(t *testing.T) {
		if _, err := do[testObject](t.Context(), c, requestOf(opGet)); err != nil {
			t.Fatal(err)
		}
	})
	lines := jsonLines(t, logged())
	if len(lines) != 4 {
		t.Fatalf("the client wrote %d lines, want 4:\n%s", len(lines), logged())
	}
	for i, status := range []float64{502, 0, 503, 200} {
		line := lines[i]
		if line["attempt"] != float64(i+1) || line["status"] != status ||
			(line["error"] == nil) != (status == 200) {
			t.Errorf("line %d is %v, want attempt %d with the status %v", i, line, i+1, status)
		}
	}
}

// TestRetryKeepsTheConnection sends a GET that fails with 503 three times, with bodies that the
// client does not read in full, and then succeeds, to a server that counts connections: the
// four attempts go over one connection, as the client drains each body before the next attempt.
func TestRetryKeepsTheConnection(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	failed := mockpermit.Reply{Status: http.StatusServiceUnavailable, Body: unreadBody}
	api.Handle(route(opGet), failed, failed, failed, mockpermit.Reply{Body: `{"key":"a"}`})
	c := wireClient(t, api, pdp,
		WithRetry(RetryPolicy{MaxRetries: 3, MaxBackoff: time.Millisecond}))
	if _, err := do[testObject](t.Context(), c, get("a")); err != nil {
		t.Fatal(err)
	}
	if got := len(api.Requests()); got != 4 {
		t.Errorf("the API got %d requests, want 4", got)
	}
	if got := api.Connections(); got != 1 {
		t.Errorf("4 attempts opened %d connections, want 1", got)
	}
}
