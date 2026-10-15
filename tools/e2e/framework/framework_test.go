package framework

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewPrefix(t *testing.T) {
	p, err := NewPrefix("1234", "2", "cloud")
	if err != nil || p != "gosdk-ci-1234-2-cloud" {
		t.Fatalf("NewPrefix = %q, %v; want gosdk-ci-1234-2-cloud", p, err)
	}
	if got := p.Key("user-1"); got != "gosdk-ci-1234-2-cloud-user-1" {
		t.Errorf("Key(user-1) = %q", got)
	}
	for _, tc := range []struct{ run, attempt, leg, want string }{
		{"", "1", "cloud", `the run "" is not a decimal number`},
		{"12a", "1", "cloud", `the run "12a"`},
		{"123456789012345678901", "1", "cloud", "the run"},
		{"1", "", "cloud", `the attempt "" is not a decimal number`},
		{"1", "-1", "cloud", `the attempt "-1"`},
		{"1", "1", "", `the leg ""`},
		{"1", "1", "Cloud", `the leg "Cloud"`},
		{"1", "1", "1cloud", `the leg "1cloud"`},
		{"1", "1", "cloud-a", `the leg "cloud-a"`},
		{"1", "1", "abcdefghijklmnopq", "the leg"},
	} {
		if _, err := NewPrefix(tc.run, tc.attempt, tc.leg); err == nil ||
			!strings.Contains(err.Error(), tc.want) {
			t.Errorf("NewPrefix(%q, %q, %q) returned %v, want an error with %q", tc.run,
				tc.attempt, tc.leg, err, tc.want)
		}
	}
	if p, err := NewPrefix("0", "1", "abcdefghijklmnop"); err != nil || len(p) != 29 {
		t.Errorf("a leg of 16 characters: NewPrefix = %q, %v", p, err)
	}
}

func TestPrefixKeyPanics(t *testing.T) {
	for _, bad := range []string{"", "User", "-user", "a_b", strings.Repeat("a", 65)} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Key(%q) did not panic", bad)
				}
			}()
			Prefix("gosdk-ci-1-1-cloud").Key(bad)
		}()
	}
	if got := Prefix("p").Key(strings.Repeat("a", 64)); len(got) != 66 {
		t.Errorf("a name of 64 characters gives the key %q", got)
	}
}

func TestPoll(t *testing.T) {
	tries := 0
	err := Poll(t.Context(), time.Second, time.Millisecond, func(context.Context) (bool, error) {
		tries++
		return tries == 3, nil
	})
	if err != nil || tries != 3 {
		t.Errorf("Poll = %v after %d tries, want nil after 3", err, tries)
	}
	boom := errors.New("boom")
	err = Poll(t.Context(), time.Second, time.Millisecond, func(context.Context) (bool, error) {
		return true, boom
	})
	if !errors.Is(err, boom) {
		t.Errorf("Poll returned %v, want the error of try", err)
	}
	start := time.Now()
	err = Poll(t.Context(), 30*time.Millisecond, time.Hour, func(context.Context) (bool, error) {
		return false, nil
	})
	if err == nil || !strings.Contains(err.Error(), "not done after 30ms (2 tries)") {
		t.Errorf("Poll that never finishes returned %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("Poll ran %s, past its timeout and its interval", elapsed)
	}
	slow, stop := context.WithTimeout(t.Context(), 10*time.Second)
	defer stop()
	start = time.Now()
	err = Poll(slow, 20*time.Millisecond, time.Hour, func(ctx context.Context) (bool, error) {
		<-ctx.Done()
		return false, ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) ||
		!strings.Contains(err.Error(), "not done after 20ms (1 tries): context deadline exceeded") {
		t.Errorf("Poll of a try that waits for its context returned %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("a slow try stretched Poll to %s", elapsed)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = Poll(ctx, time.Hour, time.Hour, func(context.Context) (bool, error) {
		return false, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Poll with an ended context returned %v", err)
	}
}

func TestNewAPIErrors(t *testing.T) {
	client := &http.Client{}
	for _, tc := range []struct{ url, key, want string }{
		{"://x", "k", "the API URL"},
		{"ftp://example.com", "k", "is not an http or https URL"},
		{"https://", "k", "is not an http or https URL"},
		{"https://example.com?a=1", "k", "has a query"},
		{"https://example.com#f", "k", "has a query"},
		{"https://u@example.com", "k", "has a query, a fragment or a user"},
		{"https://example.com", "", "needs an API key"},
	} {
		if _, err := NewAPI(tc.url, tc.key, client); err == nil ||
			!strings.Contains(err.Error(), tc.want) {
			t.Errorf("NewAPI(%q, %q) returned %v, want an error with %q", tc.url, tc.key, err,
				tc.want)
		}
	}
	if _, err := NewAPI("https://example.com", "k", nil); err == nil {
		t.Error("NewAPI without a client returned no error")
	}
	if _, err := NewAPI("http://example.com/base", "k", client); err != nil {
		t.Errorf("NewAPI of a URL with a path: %v", err)
	}
}

// exchange is a request that a test server received.
type exchange struct {
	method, path, auth, accept, contentType, body string
}

// script is a test server that answers each request with the next of its replies, and records
// the requests.
type script struct {
	mu       sync.Mutex
	replies  []func(w http.ResponseWriter)
	received []exchange
}

func (s *script) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.received = append(s.received, exchange{
		r.Method, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Accept"),
		r.Header.Get("Content-Type"), string(body),
	})
	if len(s.replies) == 0 {
		http.Error(w, "no reply left", http.StatusTeapot)
		return
	}
	next := s.replies[0]
	s.replies = s.replies[1:]
	next(w)
}

// answer returns a reply of status with header and body.
func answer(status int, body string, header ...string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		for i := 0; i+1 < len(header); i += 2 {
			w.Header().Set(header[i], header[i+1])
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

// newScripted returns an API of a test server that answers with replies, and records each wait
// of the API in waits instead of waiting.
func newScripted(t *testing.T, replies ...func(w http.ResponseWriter)) (*API, *script,
	*[]time.Duration,
) {
	t.Helper()
	s := &script{replies: replies}
	server := httptest.NewServer(s)
	t.Cleanup(server.Close)
	api, err := NewAPI(server.URL, "permit_key_FAKEapi", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	waits := &[]time.Duration{}
	api.sleep = func(ctx context.Context, d time.Duration) error {
		*waits = append(*waits, d)
		return ctx.Err()
	}
	api.jitter = func() float64 { return 0.5 }
	api.interval = time.Millisecond
	return api, s, waits
}

func TestAPIDo(t *testing.T) {
	api, s, _ := newScripted(t, answer(201, `{"id": "e1"}`), answer(200, `ok`))
	reply, err := api.Do(t.Context(), Request{
		Method: "POST", Path: "/v2/a%2Fb",
		Body: map[string]string{"key": "k"},
	})
	if err != nil || reply.Status != 201 || string(reply.Body) != `{"id": "e1"}` {
		t.Errorf("Do = %+v, %v", reply, err)
	}
	if _, err := api.Do(t.Context(), Request{Method: "GET", Path: "/v2/x"}); err != nil {
		t.Fatal(err)
	}
	want := []exchange{
		{
			"POST", "/v2/a/b", "Bearer permit_key_FAKEapi", "application/json", "application/json",
			`{"key":"k"}`,
		},
		{"GET", "/v2/x", "Bearer permit_key_FAKEapi", "application/json", "", ""},
	}
	if !slices.Equal(s.received, want) {
		t.Errorf("the server received %+v, want %+v", s.received, want)
	}
	if _, err := api.Do(t.Context(), Request{
		Method: "POST", Path: "/x",
		Body: func() {},
	}); err == nil || !strings.Contains(err.Error(), "encoding the body") {
		t.Errorf("Do of a body that JSON cannot encode returned %v", err)
	}
	if _, err := api.Do(t.Context(), Request{Method: "BAD METHOD", Path: "/x"}); err == nil {
		t.Error("Do of a request that cannot be built returned no error")
	}
}

func TestAPIDoRetries429(t *testing.T) {
	date := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)
	api, s, waits := newScripted(t,
		answer(429, "", "Retry-After", "2"),
		answer(429, ""),
		answer(429, "", "Retry-After", "nonsense"),
		answer(429, "", "Retry-After", "120"),
		answer(429, "", "Retry-After", date),
		answer(200, "done"),
	)
	reply, err := api.Do(t.Context(), Request{Method: "DELETE", Path: "/v2/x"})
	if err != nil || reply.Status != 200 {
		t.Fatalf("Do = %+v, %v; want the 200 after the 429s", reply, err)
	}
	want := []time.Duration{
		2 * time.Second,         // Retry-After in seconds
		750 * time.Millisecond,  // half of the backoff of 1s, and half of the other half
		1500 * time.Millisecond, // the backoff doubled
		30 * time.Second,        // Retry-After past Max
		30 * time.Second,        // a date an hour away, past Max
	}
	if !slices.Equal(*waits, want) {
		t.Errorf("the waits were %v, want %v", *waits, want)
	}
	if len(s.received) != 6 {
		t.Errorf("the server received %d attempts, want 6", len(s.received))
	}
}

func TestAPIDoGivesUp(t *testing.T) {
	var replies []func(http.ResponseWriter)
	for range defaultRetry.Attempts {
		replies = append(replies, answer(429, "slow down"))
	}
	api, s, waits := newScripted(t, replies...)
	reply, err := api.Do(t.Context(), Request{Method: "GET", Path: "/v2/x"})
	if err == nil || !strings.Contains(err.Error(), "GET /v2/x: still 429 after 6 attempts") ||
		reply.Status != 429 {
		t.Errorf("Do = %+v, %v; want the last 429 and an error", reply, err)
	}
	if len(s.received) != 6 || len(*waits) != 5 {
		t.Errorf("%d attempts and %d waits, want 6 and 5", len(s.received), len(*waits))
	}
	wantWaits := []time.Duration{
		750 * time.Millisecond, 1500 * time.Millisecond,
		3 * time.Second, 6 * time.Second, 12 * time.Second,
	}
	if !slices.Equal(*waits, wantWaits) {
		t.Errorf("the waits were %v, want %v", *waits, wantWaits)
	}
	api, _, _ = newScripted(t, answer(429, ""), answer(200, ""))
	api.sleep = func(context.Context, time.Duration) error { return context.DeadlineExceeded }
	_, err = api.Do(t.Context(), Request{Method: "GET", Path: "/v2/x"})
	if !errors.Is(err, context.DeadlineExceeded) ||
		!strings.Contains(err.Error(), "GET /v2/x: waiting to retry a 429") {
		t.Errorf("Do whose wait ends returned %v", err)
	}
}

func TestAPIDoBacksOffToMax(t *testing.T) {
	var replies []func(http.ResponseWriter)
	for range 7 {
		replies = append(replies, answer(429, ""))
	}
	api, _, waits := newScripted(t, append(replies, answer(204, ""))...)
	api.retry = retryPolicy{Attempts: 8, Base: 8 * time.Second, Max: 20 * time.Second}
	if _, err := api.Do(t.Context(), Request{Method: "GET", Path: "/v2/x"}); err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{
		6 * time.Second, 12 * time.Second, 15 * time.Second,
		15 * time.Second, 15 * time.Second, 15 * time.Second, 15 * time.Second,
	}
	if !slices.Equal(*waits, want) {
		t.Errorf("the waits were %v, want %v", *waits, want)
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		value string
		want  time.Duration
		ok    bool
	}{
		{"0", 0, true},
		{"7", 7 * time.Second, true},
		{"Thu, 15 Oct 2026 12:00:09 GMT", 9 * time.Second, true},
		{"Thu, 15 Oct 2026 11:00:00 GMT", 0, true},
		{"", 0, false},
		{"-1", 0, false},
		{"soon", 0, false},
	} {
		got, ok := retryAfter(tc.value, now)
		if got != tc.want || ok != tc.ok {
			t.Errorf("retryAfter(%q) = %s, %t; want %s, %t", tc.value, got, ok, tc.want, tc.ok)
		}
	}
}

func TestAPICall(t *testing.T) {
	long := strings.Repeat("x", 400)
	api, _, _ := newScripted(t, answer(200, `{"id": "e1"}`), answer(299, ""),
		answer(422, long), answer(200, "not json"), answer(302, ""), answer(500, long[:300]))
	var out struct {
		ID string `json:"id"`
	}
	r := Request{Method: "GET", Path: "/v2/x"}
	if err := api.Call(t.Context(), r, &out); err != nil || out.ID != "e1" {
		t.Errorf("Call = %v, %+v", err, out)
	}
	if err := api.Call(t.Context(), r, nil); err != nil {
		t.Errorf("Call of a 299 without out: %v", err)
	}
	err := api.Call(t.Context(), r, nil)
	if err == nil || !strings.Contains(err.Error(), "GET /v2/x answered 422: "+long[:300]+"...") ||
		strings.Contains(err.Error(), long[:301]) {
		t.Errorf("Call of a 422 returned %v, want the status and 300 bytes of the body", err)
	}
	if err := api.Call(t.Context(), r, &out); err == nil ||
		!strings.Contains(err.Error(), "decoding the reply") {
		t.Errorf("Call of a reply that is not JSON returned %v", err)
	}
	if err := api.Call(t.Context(), r, nil); err == nil ||
		!strings.Contains(err.Error(), "answered 302") {
		t.Errorf("Call of a 302 returned %v", err)
	}
	if err := api.Call(t.Context(), r, nil); err == nil ||
		!strings.HasSuffix(err.Error(), "answered 500: "+long[:300]) {
		t.Errorf("Call of a 500 with a body of 300 bytes returned %v, want the whole body", err)
	}
	api, _, _ = newScripted(t)
	api.client = &http.Client{Transport: failing{}}
	if err := api.Call(t.Context(), r, nil); err == nil {
		t.Error("Call without a reply returned no error")
	}
}

// failing is an http.RoundTripper that never gets a reply.
type failing struct{}

func (failing) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("no route to host")
}

func TestRemover(t *testing.T) {
	api, s, _ := newScripted(t, answer(200, ""), answer(204, ""), answer(404, ""),
		answer(409, "in use"))
	remove := api.Remover("/v2/x/1")
	for range 3 {
		if err := remove(t.Context()); err != nil {
			t.Errorf("a removal answered 200, 204 or 404 returned %v", err)
		}
	}
	if err := remove(t.Context()); err == nil || !strings.Contains(err.Error(),
		"DELETE /v2/x/1 answered 409: in use") {
		t.Errorf("a removal answered 409 returned %v", err)
	}
	if s.received[0].method != "DELETE" || s.received[0].path != "/v2/x/1" {
		t.Errorf("the removal sent %+v", s.received[0])
	}
	api.client = &http.Client{Transport: failing{}}
	if err := remove(t.Context()); err == nil {
		t.Error("a removal without a reply returned no error")
	}
}

func TestAbsence(t *testing.T) {
	api, s, _ := newScripted(t, answer(200, ""), answer(200, ""), answer(404, ""),
		answer(500, "down"))
	if err := api.Absence("/v2/x/1", time.Minute)(t.Context()); err != nil {
		t.Errorf("an object that goes away after two tries: %v", err)
	}
	if len(s.received) != 3 || s.received[2].method != "GET" {
		t.Errorf("the check sent %+v, want three GETs", s.received)
	}
	if err := api.Absence("/v2/x/1", time.Minute)(t.Context()); err == nil ||
		!strings.Contains(err.Error(), "GET /v2/x/1 answered 500") {
		t.Errorf("a check answered 500 returned %v", err)
	}
	api, _, _ = newScripted(t, answer(200, ""), answer(200, ""), answer(200, ""))
	if err := api.Absence("/v2/x/1", 0)(t.Context()); err == nil ||
		!strings.Contains(err.Error(), "not done after 0s (1 tries)") {
		t.Errorf("a check of an object that stays returned %v", err)
	}
	api.client = &http.Client{Transport: failing{}}
	if err := api.Absence("/v2/x/1", time.Minute)(t.Context()); err == nil {
		t.Error("a check without a reply returned no error")
	}
}

func TestRemoverWithBody(t *testing.T) {
	api, s, _ := newScripted(t, answer(204, ""), answer(404, ""), answer(422, "bad body"),
		answer(200, ""))
	remove := api.RemoverWithBody("/v2/x", map[string]string{"user": "u1"})
	for range 2 {
		if err := remove(t.Context()); err != nil {
			t.Errorf("a removal answered 204 or 404 returned %v", err)
		}
	}
	if err := remove(t.Context()); err == nil || !strings.Contains(err.Error(),
		"DELETE /v2/x answered 422: bad body") {
		t.Errorf("a removal answered 422 returned %v", err)
	}
	if err := api.Remover("/v2/y")(t.Context()); err != nil {
		t.Fatal(err)
	}
	want := []exchange{
		{
			"DELETE", "/v2/x", "Bearer permit_key_FAKEapi", "application/json", "application/json",
			`{"user":"u1"}`,
		},
		{"DELETE", "/v2/y", "Bearer permit_key_FAKEapi", "application/json", "", ""},
	}
	if got := []exchange{s.received[0], s.received[3]}; !slices.Equal(got, want) {
		t.Errorf("the removals sent %+v, want %+v", got, want)
	}
}

func TestUnlisted(t *testing.T) {
	listed := func(body []byte) (bool, error) {
		if string(body) == "not a list" {
			return false, errors.New("decoding the list: no")
		}
		return strings.Contains(string(body), `"u1"`), nil
	}
	api, s, _ := newScripted(t, answer(200, `["u1", "u2"]`), answer(200, `["u2"]`),
		answer(404, ""))
	check := api.Unlisted("/v2/x?user=u1", time.Minute, listed)
	if err := check(t.Context()); err != nil {
		t.Errorf("an object that leaves the list after one try: %v", err)
	}
	if err := check(t.Context()); err != nil {
		t.Errorf("a list that is gone: %v", err)
	}
	if len(s.received) != 3 || s.received[1].method != "GET" || s.received[1].path != "/v2/x" {
		t.Errorf("the check sent %+v, want three GETs of /v2/x", s.received)
	}
	for _, tc := range []struct {
		reply func(http.ResponseWriter)
		want  string
	}{
		{answer(500, "down"), "GET /v2/x?user=u1 answered 500: down"},
		{answer(204, ""), "GET /v2/x?user=u1 answered 204"},
		{answer(200, "not a list"), "GET /v2/x?user=u1: decoding the list: no"},
	} {
		failing, _, _ := newScripted(t, tc.reply)
		err := failing.Unlisted("/v2/x?user=u1", time.Minute, listed)(t.Context())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("the check returned %v, want an error with %q", err, tc.want)
		}
	}
	api, _, _ = newScripted(t, answer(200, `["u1"]`))
	if err := api.Unlisted("/v2/x", 0, listed)(t.Context()); err == nil ||
		!strings.Contains(err.Error(), "not done after 0s (1 tries)") {
		t.Errorf("a check of an object that stays listed returned %v", err)
	}
}

func TestLedger(t *testing.T) {
	var order []string
	failing := map[string]error{}
	step := func(what string) func(context.Context) error {
		return func(context.Context) error {
			order = append(order, what)
			return failing[what]
		}
	}
	var l Ledger
	if err := l.Reconcile(); err != nil || l.Counts() != (Counts{}) {
		t.Errorf("an empty ledger: %v, %v", err, l.Counts())
	}
	boom := errors.New("boom")
	failing["remove b"], failing["verify c"] = boom, boom
	for _, name := range []string{"a", "b", "c"} {
		l.Register(name, step("remove "+name), step("verify "+name))
	}
	err := l.Teardown(t.Context())
	if err == nil || !strings.Contains(err.Error(), "removing b: boom") ||
		!strings.Contains(err.Error(), "checking that c is gone: boom") {
		t.Errorf("Teardown returned %v", err)
	}
	wantOrder := []string{"remove c", "verify c", "remove b", "remove a", "verify a"}
	if !slices.Equal(order, wantOrder) {
		t.Errorf("Teardown ran %v, want %v", order, wantOrder)
	}
	if got := l.Counts(); got != (Counts{Registered: 3, Completed: 2, Verified: 1}) {
		t.Errorf("Counts = %v", got)
	}
	err = l.Reconcile()
	want := "the ledger does not reconcile, registered=3 completed=2 verified=1: " +
		"b (not removed), c (removed, not checked to be gone)"
	if err == nil || err.Error() != want {
		t.Errorf("Reconcile = %v, want %q", err, want)
	}
	order = nil
	clear(failing)
	l.Register("b2", step("remove b2"), step("verify b2"))
	if err := l.Teardown(t.Context()); err != nil {
		t.Errorf("a second Teardown, with what failed fixed: %v", err)
	}
	wantOrder = []string{"remove b2", "verify b2", "verify c", "remove b", "verify b"}
	if !slices.Equal(order, wantOrder) {
		t.Errorf("a second Teardown ran %v, want %v", order, wantOrder)
	}
	if err := l.Reconcile(); err != nil {
		t.Errorf("Reconcile after the teardown completed: %v", err)
	}
	if got := l.Counts().String(); got != "registered=4 completed=4 verified=4" {
		t.Errorf("Counts().String() = %q", got)
	}
}

// TestLedgerChecksBeforeTheNextRemoval tears down one ledger that holds an environment and a user
// in it, whose removal does nothing. Once the environment is deleted, the user reads as gone, as
// every object of a deleted environment does, so the user must be checked before that.
func TestLedgerChecksBeforeTheNextRemoval(t *testing.T) {
	var order []string
	environment := true
	var l Ledger
	l.Register("environment e", func(context.Context) error {
		order = append(order, "remove e")
		environment = false
		return nil
	}, func(context.Context) error {
		order = append(order, "verify e")
		return nil
	})
	l.Register("user u", func(context.Context) error {
		order = append(order, "remove u")
		return nil
	}, func(context.Context) error {
		order = append(order, "verify u")
		if environment {
			return errors.New("still there")
		}
		return nil
	})
	err := l.Teardown(t.Context())
	if want := "checking that user u is gone: still there"; err == nil || err.Error() != want {
		t.Errorf("Teardown returned %v, want %q", err, want)
	}
	wantOrder := []string{"remove u", "verify u", "remove e", "verify e"}
	if !slices.Equal(order, wantOrder) {
		t.Errorf("Teardown ran %v, want %v", order, wantOrder)
	}
	want := "the ledger does not reconcile, registered=2 completed=2 verified=1: " +
		"user u (removed, not checked to be gone)"
	if err := l.Reconcile(); err == nil || err.Error() != want {
		t.Errorf("Reconcile = %v, want %q", err, want)
	}
}

func TestReadKeys(t *testing.T) {
	dir := t.TempDir()
	write := func(content string) string {
		path := filepath.Join(dir, "keys.yaml")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	for content, want := range map[string]bool{
		"organization_key: true\n":             true,
		"# a comment\norganization_key: false": false,
	} {
		if keys, err := ReadKeys(write(content)); err != nil || keys.OrganizationKey != want {
			t.Errorf("ReadKeys(%q) = %+v, %v", content, keys, err)
		}
	}
	for content, wantErr := range map[string]string{
		"":                                 "parsing",
		"organization_key: maybe\n":        "parsing",
		"organization_key: true\nextra: 1": "field extra not found",
		"# nothing set\n{}":                "does not set organization_key",
	} {
		if _, err := ReadKeys(write(content)); err == nil ||
			!strings.Contains(err.Error(), wantErr) {
			t.Errorf("ReadKeys(%q) returned %v, want an error with %q", content, err, wantErr)
		}
	}
	if _, err := ReadKeys(filepath.Join(dir, "none.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ReadKeys of a missing file returned %v", err)
	}
}
