package permit

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/internal/testkit/concurrent"
	"github.com/permitio/permit-golang/v2/internal/testkit/leak"
)

// closeRaceKey is the API key of the Close test. It is not a Permit API key, so only the
// client's Set, and no pattern, replaces it.
const closeRaceKey = "close-race-canary-close-race-canary"

// TestCloseWithCallsInFlight calls Close while 20 calls wait for their replies, under -race.
// Each call finishes, with its reply or a *Error; a call after Close returns ErrClosed and sends
// no request; and the Debug lines of the calls in flight still have the API key replaced, which
// the server's error replies echo.
func TestCloseWithCallsInFlight(t *testing.T) {
	leak.Check(t)
	const inFlight = 20
	var requests atomic.Int32
	arrived := make(chan struct{}, inFlight)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		arrived <- struct{}{}
		<-release
		key := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		if n, _ := strconv.Atoi(key); n%2 == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"message":"the key `+closeRaceKey+` is revoked"}`)
			return
		}
		_, _ = io.WriteString(w, `{"key":"`+key+`"}`)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(releaseAll)
	logger, logged := debugLog()
	c := urlClient(t, closeRaceKey, srv.URL, WithLogger(logger))

	closed := make(chan error, 1)
	go func() {
		for range inFlight {
			<-arrived
		}
		closed <- c.Close()
		releaseAll()
	}()
	var errorTexts strings.Builder
	var mu sync.Mutex
	err := concurrent.Run(t, inFlight, 30*time.Second, func(ctx context.Context, i int) error {
		got, err := do[testObject](ctx, c, get(strconv.Itoa(i)))
		if i%2 == 0 {
			if err != nil || got.Key != strconv.Itoa(i) {
				return errors.Join(errors.New("want the reply"), err)
			}
			return nil
		}
		e, ok := errors.AsType[*Error](err)
		if !ok || !errors.Is(err, ErrUnauthorized) {
			return errors.Join(errors.New("want the *Error of the 401"), err)
		}
		mu.Lock()
		errorTexts.WriteString(e.Error() + string(e.Body) + "\n")
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-closed; err != nil {
		t.Errorf("Close returned %v", err)
	}

	before := requests.Load()
	if _, err := do[testObject](t.Context(), c, get("0")); !errors.Is(err, ErrClosed) {
		t.Errorf("a call after Close returned %v, want ErrClosed", err)
	}
	if after := requests.Load(); after != before || before != inFlight {
		t.Errorf("the server got %d requests, then %d after Close, want %d and no more", before,
			after, inFlight)
	}

	lines := jsonLines(t, logged())
	if len(lines) != inFlight {
		t.Errorf("the client wrote %d Debug lines, want one for each call in flight, %d",
			len(lines), inFlight)
	}
	scanner := canary.NewScanner(closeRaceKey)
	_, _ = scanner.Write(logged())
	_, _ = scanner.Write([]byte(errorTexts.String()))
	scanner.Check(t)
}

// TestCallAfterCloseSendsNothing checks that each kind of call after Close returns ErrClosed
// without a request.
func TestCallAfterCloseSendsNothing(t *testing.T) {
	counter := &countingTransport{}
	c := urlClient(t, canary.Key("afterclose"), codeURL,
		WithHTTPClient(&http.Client{Transport: counter}))
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	for name, call := range map[string]func(ctx context.Context) error{
		"do": func(ctx context.Context) error {
			_, err := do[testObject](ctx, c, get("a"))
			return err
		},
		"doNoValue": func(ctx context.Context) error { return c.doNoValue(ctx, get("a")) },
		"doList": func(ctx context.Context) error {
			_, err := doList[testObject](ctx, c, request{op: opList.ID, target: apiTarget},
				arrayShape)
			return err
		},
	} {
		if err := call(t.Context()); !errors.Is(err, ErrClosed) {
			t.Errorf("%s after Close returned %v, want ErrClosed", name, err)
		}
	}
	if n := counter.requests.Load(); n != 0 {
		t.Errorf("the calls after Close sent %d requests, want none", n)
	}
}
