package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/redact"
	"github.com/permitio/permit-golang/v2/internal/testhook"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
)

// testVersion is the SDK version that the tests' senders name.
const testVersion = "2.0.0-test"

// server is a local server that counts the connections that clients open to it.
type server struct {
	*httptest.Server

	mu          sync.Mutex
	connections int
}

// newServer starts a server that answers every request with handler, and stops it when the test
// ends.
func newServer(t *testing.T, handler http.HandlerFunc) *server {
	t.Helper()
	s := &server{}
	s.Server = httptest.NewUnstartedServer(handler)
	s.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			s.mu.Lock()
			s.connections++
			s.mu.Unlock()
		}
	}
	s.Start()
	t.Cleanup(s.Close)
	return s
}

// Connections returns how many connections clients have opened to the server.
func (s *server) Connections() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connections
}

// newSender returns a Sender with the API key given that sends through an http.Client of its own,
// whose idle connections close when the test ends.
func newSender(t *testing.T, key string) *Sender {
	t.Helper()
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatalf("http.DefaultTransport is a %T, not an *http.Transport", http.DefaultTransport)
	}
	hc := &http.Client{Transport: base.Clone()}
	t.Cleanup(hc.CloseIdleConnections)
	return New(hc, redact.NewSecret(key), testVersion)
}

// get returns a Request for GET path of srv that reads the body of a reply in 200-299 when
// readBody is set.
func get(srv *server, path string, readBody bool) Request {
	return Request{
		Method: http.MethodGet, BaseURL: srv.URL, Path: path, Template: path,
		Target: testhook.API, ReadBody: readBody,
	}
}

// mustSend sends r and fails the test when Send returns an error.
func mustSend(t *testing.T, s *Sender, r Request) Reply {
	t.Helper()
	reply, err := s.Send(t.Context(), r)
	if err != nil {
		t.Fatalf("Send(%s %s): %v", r.Method, r.Path, err)
	}
	return reply
}

func TestUserAgent(t *testing.T) {
	want := "permit-golang/2.0.0 (" + runtime.Version() + ")"
	if got := UserAgent("2.0.0"); got != want {
		t.Errorf("UserAgent(2.0.0) = %q, want %q", got, want)
	}
}

// TestSendSetsTheHeaders is the golden test of the headers of a request without a body and of
// one with a body: the server sees these headers and no others, the API key after exactly
// "Bearer ". net/http adds Accept-Encoding, and Content-Length for a body.
func TestSendSetsTheHeaders(t *testing.T) {
	key := canary.Key("headers")
	var (
		mu  sync.Mutex
		got []http.Header
	)
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.Header.Clone())
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	s := newSender(t, key)
	mustSend(t, s, get(srv, "/v2/x", false))
	post := get(srv, "/v2/x", false)
	post.Method, post.Body = http.MethodPost, []byte(`{"key":"a"}`)
	mustSend(t, s, post)

	common := http.Header{
		"Accept":                {"application/json"},
		"Accept-Encoding":       {"gzip"},
		"Authorization":         {"Bearer " + key},
		"User-Agent":            {"permit-golang/" + testVersion + " (" + runtime.Version() + ")"},
		"X-Permit-Sdk-Language": {"go"},
		"X-Permit-Sdk-Version":  {testVersion},
	}
	withBody := common.Clone()
	withBody["Content-Length"] = []string{"11"}
	withBody["Content-Type"] = []string{"application/json"}
	if diff := cmp.Diff([]http.Header{common, withBody}, got); diff != "" {
		t.Errorf("the headers differ (-want +got):\n%s", diff)
	}
}

// TestSendSetsTheHeadersOfTheRequest checks that a request carries the headers of its
// Request.Header, each value of a header that has more than one, in their canonical form; that a
// header that Send sets itself replaces one of the same name there; that Send leaves the map as
// it was; and that the next request without them carries none.
func TestSendSetsTheHeadersOfTheRequest(t *testing.T) {
	key := canary.Key("ownheaders")
	got := make(chan http.Header, 2)
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	})
	s := newSender(t, key)
	own := http.Header{
		"x-wait-timeout": {"0.5"},
		"X-Many":         {"a", "b"},
		"Authorization":  {"Bearer forged"},
		"User-Agent":     {"forged"},
	}
	before := own.Clone()
	r := get(srv, "/v2/x", false)
	r.Header = own
	mustSend(t, s, r)
	mustSend(t, s, get(srv, "/v2/x", false))

	sent := <-got
	for name, want := range map[string][]string{
		"X-Wait-Timeout": {"0.5"},
		"X-Many":         {"a", "b"},
		"Authorization":  {"Bearer " + key},
		"User-Agent":     {UserAgent(testVersion)},
	} {
		if diff := cmp.Diff(want, sent.Values(name)); diff != "" {
			t.Errorf("the request has the header %s that differs (-want +got):\n%s", name, diff)
		}
	}
	if diff := cmp.Diff(before, own); diff != "" {
		t.Errorf("Send changed Request.Header (-before +after):\n%s", diff)
	}
	next := <-got
	if next.Get("X-Wait-Timeout") != "" || next.Get("X-Many") != "" {
		t.Errorf("the next request, without them, carries %v", next)
	}
}

// TestSendSendsThePathAndQueryAsGiven checks that the server gets the path with its escapes as
// they are, after the path of the base URL, and the query.
func TestSendSendsThePathAndQueryAsGiven(t *testing.T) {
	uris := make(chan string, 2)
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		uris <- r.RequestURI
		w.WriteHeader(http.StatusNoContent)
	})
	s := newSender(t, canary.Key("path"))
	r := get(srv, "/v2/facts/p/e/users/a%2Fb%3Bc%20d", false)
	r.BaseURL += "/permit"
	mustSend(t, s, r)
	r.Query = "a=%20&user=b&user=c"
	mustSend(t, s, r)
	for _, want := range []string{
		"/permit/v2/facts/p/e/users/a%2Fb%3Bc%20d",
		"/permit/v2/facts/p/e/users/a%2Fb%3Bc%20d?a=%20&user=b&user=c",
	} {
		if got := <-uris; got != want {
			t.Errorf("the server got %q, want %q", got, want)
		}
	}
}

// replyWith returns a handler that answers with status and body, with a Content-Length header,
// so that a test can tell what the client read of the body from whether it kept the connection.
func replyWith(status int, body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}
}

func TestSendReadsTheBodyAsAsked(t *testing.T) {
	long := bytes.Repeat([]byte("e"), MaxErrorBody+100)
	for _, tc := range []struct {
		name     string
		status   int
		body     []byte
		readBody bool
		want     []byte
	}{
		{"200 read", http.StatusOK, []byte(`{"a":1}`), true, []byte(`{"a":1}`)},
		{"200 not read", http.StatusOK, []byte(`{"a":1}`), false, nil},
		{"299 read", 299, []byte(`[1]`), true, []byte(`[1]`)},
		{"299 not read", 299, []byte(`[1]`), false, nil},
		{"204 read", http.StatusNoContent, nil, true, []byte{}},
		{"300 not read", http.StatusMultipleChoices, []byte("multiple"), false, []byte("multiple")},
		{"404 not read", http.StatusNotFound, []byte("gone"), false, []byte("gone")},
		{"500 read", http.StatusInternalServerError, []byte("oops"), true, []byte("oops")},
		{"a long error body", http.StatusBadGateway, long, false, long[:MaxErrorBody]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, replyWith(tc.status, tc.body))
			reply := mustSend(t, newSender(t, canary.Key("read")), get(srv, "/v2/x", tc.readBody))
			if reply.Status != tc.status {
				t.Errorf("Status = %d, want %d", reply.Status, tc.status)
			}
			if !bytes.Equal(reply.Body, tc.want) || (reply.Body == nil) != (tc.want == nil) {
				t.Errorf("Body = %.40q (%d bytes, nil: %t), want %.40q (%d bytes, nil: %t)",
					reply.Body, len(reply.Body), reply.Body == nil, tc.want, len(tc.want),
					tc.want == nil)
			}
		})
	}
}

// TestSendDrainsTheBody sends two requests whose replies have a body that the client does not
// read in full, and checks whether the second went over the connection of the first: it does
// when what is left of the body is at most MaxDrain bytes, which the client reads and discards.
// The bodies have a Content-Length above 256 KiB, the most that the Close of a reply's body
// drains by itself on Go 1.27, so the test checks the client's own drain on every Go version.
func TestSendDrainsTheBody(t *testing.T) {
	for _, tc := range []struct {
		name            string
		status          int
		size            int
		wantConnections int
	}{
		{"a 200 with MaxDrain bytes", http.StatusOK, MaxDrain, 1},
		{"a 200 with MaxDrain+1 bytes", http.StatusOK, MaxDrain + 1, 2},
		{"a 500 with MaxErrorBody+MaxDrain bytes", 500, MaxErrorBody + MaxDrain, 1},
		{"a 500 with MaxErrorBody+MaxDrain+1 bytes", 500, MaxErrorBody + MaxDrain + 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, replyWith(tc.status, bytes.Repeat([]byte("d"), tc.size)))
			s := newSender(t, canary.Key("drain"))
			for range 2 {
				mustSend(t, s, get(srv, "/v2/x", false))
			}
			if got := srv.Connections(); got != tc.wantConnections {
				t.Errorf("two requests opened %d connections, want %d", got, tc.wantConnections)
			}
		})
	}
}

// TestSendDrainsAChunkedBody checks the drain of a body without a Content-Length, which the
// server sends in chunks: the connection goes back to the pool after it.
func TestSendDrainsAChunkedBody(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("the server's ResponseWriter cannot flush")
			return
		}
		for range 4 {
			_, _ = w.Write(bytes.Repeat([]byte("c"), 64<<10))
			flusher.Flush()
		}
	})
	s := newSender(t, canary.Key("chunked"))
	for range 3 {
		mustSend(t, s, get(srv, "/v2/x", false))
	}
	if got := srv.Connections(); got != 1 {
		t.Errorf("three requests opened %d connections, want 1", got)
	}
}

// trackingBody is the body of a reply that counts what was read of it and whether it was closed.
type trackingBody struct {
	r       io.Reader
	readErr error
	read    int
	closes  int
}

func (b *trackingBody) Read(p []byte) (int, error) {
	if b.readErr != nil {
		return 0, b.readErr
	}
	n, err := b.r.Read(p)
	b.read += n
	return n, err
}

func (b *trackingBody) Close() error {
	b.closes++
	return errors.New("close failed")
}

// roundTripper answers every request with its reply, without a connection.
type roundTripper struct {
	status int
	body   *trackingBody
}

func (rt roundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: rt.status, Body: rt.body, Header: http.Header{}}, nil
}

// TestSendClosesTheBodyOnEveryPath checks that Send closes the body of every reply, once, after
// it read what it needs of it and drained up to MaxDrain bytes of the rest, also when a read
// fails.
func TestSendClosesTheBodyOnEveryPath(t *testing.T) {
	readFailed := errors.New("connection reset")
	size := MaxErrorBody + MaxDrain + 10
	for _, tc := range []struct {
		name     string
		status   int
		readBody bool
		readErr  error
		wantRead int
	}{
		{"a 200 that the caller reads", http.StatusOK, true, nil, size},
		{"a 200 that the caller does not read", http.StatusOK, false, nil, MaxDrain},
		{"an error reply", http.StatusInternalServerError, false, nil, MaxErrorBody + MaxDrain},
		{"a read that fails", http.StatusOK, true, readFailed, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackingBody{r: bytes.NewReader(make([]byte, size)), readErr: tc.readErr}
			hc := &http.Client{Transport: roundTripper{status: tc.status, body: body}}
			s := New(hc, redact.NewSecret(canary.Key("close")), testVersion)
			reply, err := s.Send(t.Context(), Request{
				Method: http.MethodGet, BaseURL: "http://127.0.0.1:1", Path: "/v2/x",
				ReadBody: tc.readBody,
			})
			if !errors.Is(err, tc.readErr) || (tc.readErr == nil) != (err == nil) {
				t.Errorf("Send returned %v, want %v", err, tc.readErr)
			}
			if reply.Status != tc.status {
				t.Errorf("Status = %d, want %d also when the body cannot be read", reply.Status,
					tc.status)
			}
			if body.closes != 1 || body.read != tc.wantRead {
				t.Errorf("Send closed the body %d times and read %d bytes of it, want once and %d",
					body.closes, body.read, tc.wantRead)
			}
		})
	}
}

// TestSendWithoutAReply checks a request that gets no reply: Send returns the error with status
// 0, and how long the attempt took.
func TestSendWithoutAReply(t *testing.T) {
	srv := newServer(t, replyWith(http.StatusOK, nil))
	srv.Close()
	reply, err := newSender(t, canary.Key("noreply")).Send(t.Context(), get(srv, "/v2/x", true))
	if err == nil || reply.Status != 0 || reply.Body != nil || reply.Duration <= 0 {
		t.Errorf("Send to a closed server returned %+v, %v; want status 0, no body, a duration "+
			"and an error", reply, err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	live := newServer(t, replyWith(http.StatusOK, nil))
	s := newSender(t, canary.Key("noreply"))
	if _, err := s.Send(ctx, get(live, "/v2/x", true)); !errors.Is(err, context.Canceled) {
		t.Errorf("Send with a canceled context returned %v, want context.Canceled", err)
	}
	if got := live.Connections(); got != 0 {
		t.Errorf("Send with a canceled context opened %d connections, want 0", got)
	}

	bad := get(live, "/v2/x", true)
	bad.BaseURL = "http://[::1"
	if reply, err := s.Send(t.Context(), bad); err == nil || reply.Status != 0 {
		t.Errorf("Send to a URL that does not parse returned %+v, %v; want an error", reply, err)
	}
}

// TestSendWithACutBody checks a reply whose connection closes before the body that its
// Content-Length announces ends: Send returns the error with the status of the reply.
func TestSendWithACutBody(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		conn, buf, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = buf.WriteString("HTTP/1.1 502 Bad Gateway\r\nContent-Length: 100\r\n\r\ncut")
		_ = buf.Flush()
	})
	reply, err := newSender(t, canary.Key("cut")).Send(t.Context(), get(srv, "/v2/x", true))
	if !errors.Is(err, io.ErrUnexpectedEOF) || reply.Status != http.StatusBadGateway {
		t.Errorf("Send returned %+v, %v; want status 502 and an unexpected EOF", reply, err)
	}
}

func TestSendMeasuresTheAttempt(t *testing.T) {
	const delay = 50 * time.Millisecond
	srv := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(delay)
		w.WriteHeader(http.StatusNoContent)
	})
	reply := mustSend(t, newSender(t, canary.Key("time")), get(srv, "/v2/x", true))
	if reply.Duration < delay {
		t.Errorf("Duration = %v, want at least the server's delay, %v", reply.Duration, delay)
	}
}

// recordLines returns the request lines of the record file at path, decoded.
func recordLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var lines []map[string]any
	for line := range bytes.Lines(data) {
		var decoded map[string]any
		if err := json.Unmarshal(line, &decoded); err != nil {
			t.Fatalf("the record holds %q: %v", line, err)
		}
		if decoded["type"] == "request" {
			lines = append(lines, decoded)
		}
	}
	return lines
}

// TestSendRecordsEachAttempt checks that Send writes each attempt to the record of the Recorder
// that the context carries, in its mode, with the route's path and template and the status, and
// writes nothing for a context without a Recorder.
func TestSendRecordsEachAttempt(t *testing.T) {
	for _, mode := range []testhook.Mode{testhook.DryRun, testhook.Live} {
		t.Run(string(mode), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "record.jsonl")
			file, err := testhook.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			recorder, err := testhook.NewRecorder(file, mode, t.Name())
			if err != nil {
				t.Fatal(err)
			}
			srv := newServer(t, replyWith(http.StatusCreated, nil))
			s := newSender(t, canary.Key("record"))
			ctx := testhook.WithRecorder(t.Context(), recorder)
			r := Request{
				Method: http.MethodPost, BaseURL: srv.URL + "/permit",
				Path: "/v2/facts/p/e/users/a%2Fb", Query: "x=1",
				Template: "/v2/facts/{proj_id}/{env_id}/users/{user_id}", Target: testhook.PDP,
			}
			if _, err := s.Send(ctx, r); err != nil {
				t.Fatal(err)
			}
			mustSend(t, s, r) // without a Recorder
			srv.Close()
			if _, err := s.Send(ctx, r); err == nil {
				t.Fatal("Send to a closed server returned no error")
			}
			if err := file.Close(0); err != nil {
				t.Fatal(err)
			}
			want := map[string]any{
				"type": "request", "mode": string(mode), "target": "pdp", "method": "POST",
				"template": "/v2/facts/{proj_id}/{env_id}/users/{user_id}",
				"path":     "/v2/facts/p/e/users/a%2Fb", "test": t.Name(),
			}
			created, failed := maps.Clone(want), maps.Clone(want)
			created["status"], failed["status"] = 201.0, 0.0
			if diff := cmp.Diff([]map[string]any{created, failed},
				recordLines(t, path)); diff != "" {
				t.Errorf("the record differs (-want +got):\n%s", diff)
			}
		})
	}
}

// TestSendIsSafeForConcurrentUse sends from many goroutines through one Sender, under -race.
func TestSendIsSafeForConcurrentUse(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.TrimPrefix(r.URL.Path, "/v2/"))
	})
	s := newSender(t, canary.Key("concurrent"))
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			want := strconv.Itoa(i)
			reply, err := s.Send(t.Context(), get(srv, "/v2/"+want, true))
			if err != nil || string(reply.Body) != want {
				t.Errorf("request %d got %q, %v", i, reply.Body, err)
			}
		})
	}
	wg.Wait()
}

// TestDrainStopsAtMaxDrain checks the bound of the drain on a body without an end.
func TestDrainStopsAtMaxDrain(t *testing.T) {
	body := &trackingBody{r: endless{}}
	drain(body)
	if body.read != MaxDrain {
		t.Errorf("drain read %d bytes, want %d", body.read, MaxDrain)
	}
}

// endless is a reader without an end.
type endless struct{}

func (endless) Read(p []byte) (int, error) { return len(p), nil }
