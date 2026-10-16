package permit

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/internal/redact"
	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/internal/testkit/concurrent"
	"github.com/permitio/permit-golang/v2/internal/testkit/leak"
)

// connectionServer starts a server that answers every request with 204 No Content, and returns
// it with a channel that receives a value each time one of its connections closes.
func connectionServer(t *testing.T) (*httptest.Server, <-chan struct{}) {
	t.Helper()
	closed := make(chan struct{}, 16)
	noContent := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(noContent))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			select {
			case closed <- struct{}{}:
			default:
			}
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, closed
}

// sendOne sends one request to url through hc and reads the reply, so that its connection goes
// back to the idle connections of hc's transport.
func sendOne(t *testing.T, hc *http.Client, url string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
}

// waitClosed waits up to 10 seconds for a value on closed, and calls collect before each wait of
// 10 ms, such as to run the garbage collector. It fails the test with what when none arrives.
func waitClosed(t *testing.T, closed <-chan struct{}, collect func(), what string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		collect()
		select {
		case <-closed:
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
	t.Fatalf("no connection closed in 10s: %s", what)
}

// defaultTransport returns http.DefaultTransport, which the tests expect to be an
// *http.Transport.
func defaultTransport(t *testing.T) *http.Transport {
	t.Helper()
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatalf("http.DefaultTransport is a %T, not an *http.Transport", http.DefaultTransport)
	}
	return transport
}

// funcPointer returns the code pointer of the function f, so that two function values can be
// compared.
func funcPointer(f any) uintptr {
	return reflect.ValueOf(f).Pointer()
}

func TestOwnHTTPClient(t *testing.T) {
	clearEnv(t)
	base := defaultTransport(t)
	baseIdlePerHost := base.MaxIdleConnsPerHost
	c, other := mustNew(t, localOptions()...), mustNew(t, localOptions()...)
	if c.transport == nil || c.httpClient == nil {
		t.Fatal("the client has no http.Client and transport of its own")
	}
	if c.httpClient.Transport != c.transport {
		t.Error("the client's http.Client does not use the client's transport")
	}
	if c.transport == base {
		t.Error("the client uses the process-wide http.DefaultTransport")
	}
	if c.transport == other.transport || c.httpClient == other.httpClient {
		t.Error("two clients share a transport, so they share their connections")
	}
	if c.httpClient.Timeout != 0 {
		t.Errorf("the http.Client has the Timeout %v, want none: WithTimeout bounds each call",
			c.httpClient.Timeout)
	}
	tr := c.transport
	if funcPointer(tr.Proxy) != funcPointer(http.ProxyFromEnvironment) {
		t.Error("the transport does not take its proxy from the environment")
	}
	if tr.MaxIdleConnsPerHost != 64 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 64", tr.MaxIdleConnsPerHost)
	}
	// Transport.Clone gives the copy a TLS configuration that holds only the HTTP/2 protocols,
	// so the default is what a clone of http.DefaultTransport has: no setting of the SDK's own.
	defaultTLS := base.Clone().TLSClientConfig
	if !reflect.DeepEqual(tr.TLSClientConfig.Clone(), defaultTLS.Clone()) {
		t.Errorf("the TLS configuration is %+v, want the default %+v", tr.TLSClientConfig,
			defaultTLS)
	}
	if tr.TLSClientConfig == defaultTLS || tr.TLSClientConfig == base.TLSClientConfig {
		t.Error("the transport shares its TLS configuration with another transport")
	}
	for _, field := range []struct {
		name      string
		got, want any
	}{
		{"DialContext", funcPointer(tr.DialContext), funcPointer(base.DialContext)},
		{"ForceAttemptHTTP2", tr.ForceAttemptHTTP2, base.ForceAttemptHTTP2},
		{"MaxIdleConns", tr.MaxIdleConns, base.MaxIdleConns},
		{"IdleConnTimeout", tr.IdleConnTimeout, base.IdleConnTimeout},
		{"TLSHandshakeTimeout", tr.TLSHandshakeTimeout, base.TLSHandshakeTimeout},
		{"ExpectContinueTimeout", tr.ExpectContinueTimeout, base.ExpectContinueTimeout},
	} {
		if field.got != field.want {
			t.Errorf("%s = %v, want %v as http.DefaultTransport has it", field.name, field.got,
				field.want)
		}
	}
	if base.MaxIdleConnsPerHost != baseIdlePerHost {
		t.Errorf("New changed http.DefaultTransport's MaxIdleConnsPerHost to %d",
			base.MaxIdleConnsPerHost)
	}
}

// closeRecorder is a transport that counts the calls of CloseIdleConnections, which
// http.Client.CloseIdleConnections makes.
type closeRecorder struct {
	http.RoundTripper
	closes int
}

func (r *closeRecorder) CloseIdleConnections() { r.closes++ }

// clientFields returns the fields of hc that New and Close must leave as they are.
func clientFields(hc *http.Client) [4]any {
	return [4]any{hc.Transport, hc.Jar, hc.Timeout, funcPointer(hc.CheckRedirect)}
}

func TestCallerHTTPClientIsUsedAsIs(t *testing.T) {
	inner := &http.Transport{MaxIdleConnsPerHost: 3, TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS13,
	}}
	recorder := &closeRecorder{RoundTripper: inner}
	bare := &http.Transport{MaxIdleConnsPerHost: 5, IdleConnTimeout: time.Second}
	for _, tc := range []struct {
		name string
		hc   *http.Client
		// check fails the test when the transport of hc changed.
		check func(t *testing.T)
	}{
		{
			name: "a transport that wraps another",
			hc: &http.Client{
				Transport: recorder,
				Timeout:   7 * time.Second,
				CheckRedirect: func(*http.Request, []*http.Request) error {
					return http.ErrUseLastResponse
				},
			},
			check: func(t *testing.T) {
				t.Helper()
				if inner.MaxIdleConnsPerHost != 3 ||
					inner.TLSClientConfig.MinVersion != tls.VersionTLS13 {
					t.Error("New and Close changed the transport of the http.Client given")
				}
				if recorder.closes != 0 {
					t.Errorf("Close closed the idle connections of the http.Client given %d "+
						"times, want 0", recorder.closes)
				}
			},
		},
		{
			name: "an *http.Transport",
			hc:   &http.Client{Transport: bare},
			check: func(t *testing.T) {
				t.Helper()
				if bare.MaxIdleConnsPerHost != 5 || bare.IdleConnTimeout != time.Second ||
					bare.Proxy != nil || bare.TLSClientConfig != nil {
					t.Error("New and Close changed the *http.Transport of the http.Client given")
				}
			},
		},
		{
			name:  "no transport",
			hc:    &http.Client{},
			check: func(*testing.T) {},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			before := clientFields(tc.hc)
			opts := append(localOptions(), WithHTTPClient(tc.hc), WithTimeout(time.Minute))
			c := mustNew(t, opts...)
			if c.httpClient != tc.hc || c.transport != nil {
				t.Fatalf("the client uses the http.Client %p and owns the transport %p, want "+
					"the one given, %p, and none", c.httpClient, c.transport, tc.hc)
			}
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
			if after := clientFields(tc.hc); after != before {
				t.Errorf("New and Close changed the http.Client given: %v, want %v", after,
					before)
			}
			tc.check(t)
		})
	}
}

// TestReplacedDefaultTransport checks a client built while http.DefaultTransport is not an
// *http.Transport, as when a test replaces it: the client sends through it, and Close leaves it.
func TestReplacedDefaultTransport(t *testing.T) {
	clearEnv(t)
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	recorder := &closeRecorder{RoundTripper: original}
	http.DefaultTransport = recorder
	c := mustNew(t, localOptions()...)
	if c.httpClient.Transport != recorder || c.transport != nil {
		t.Errorf("the client uses the transport %T and owns %v, want http.DefaultTransport and "+
			"no transport of its own", c.httpClient.Transport, c.transport)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if recorder.closes != 0 {
		t.Errorf("Close closed the idle connections of http.DefaultTransport %d times, want 0",
			recorder.closes)
	}
}

func TestCloseTwiceReturnsNil(t *testing.T) {
	clearEnv(t)
	c := mustNew(t, localOptions()...)
	for i := range 2 {
		if err := c.Close(); err != nil {
			t.Errorf("Close %d: %v, want nil", i+1, err)
		}
	}
}

func TestCallAfterCloseIsClosed(t *testing.T) {
	clearEnv(t)
	c := mustNew(t, localOptions()...)
	if err := c.checkOpen(); err != nil {
		t.Fatalf("before Close, checkOpen() = %v, want nil", err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.checkOpen(); !errors.Is(err, ErrClosed) {
		t.Errorf("after Close, checkOpen() = %v, want ErrClosed", err)
	}
}

func TestCloseIsSafeForConcurrentUse(t *testing.T) {
	clearEnv(t)
	c := mustNew(t, localOptions()...)
	err := concurrent.Run(t, 16, 10*time.Second, func(_ context.Context, i int) error {
		if i%2 == 0 {
			return c.Close()
		}
		if err := c.checkOpen(); err != nil && !errors.Is(err, ErrClosed) {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(c.checkOpen(), ErrClosed) {
		t.Error("the client is open after Close")
	}
}

func TestCloseOfNoClient(t *testing.T) {
	var c *Client
	if err := c.Close(); err != nil {
		t.Errorf("Close of a nil *Client = %v, want nil", err)
	}
	if err := new(Client).Close(); err != nil {
		t.Errorf("Close of the zero Client = %v, want nil", err)
	}
}

func TestNewHoldsTheDefaultRegistry(t *testing.T) {
	clearEnv(t)
	c := mustNew(t, localOptions()...)
	if c.registry != rest.Default() {
		t.Errorf("New built a client whose registry is %p, want rest.Default(), %p", c.registry,
			rest.Default())
	}
}

// echoOp returns a test-only operation with the ID id.
func echoOp(id string) rest.Op {
	return rest.Op{
		ID: id, Method: http.MethodGet, Route: rest.TestPrefix + id,
		KeyLevel: rest.EnvironmentLevel, Idempotent: true,
	}
}

// TestTestOpsStayInTheTestClient checks that a test-only operation is in the registry of the test
// client that registers it, and in no other: not in rest.Default, not in a client that New built
// and not in another test client.
func TestTestOpsStayInTheTestClient(t *testing.T) {
	clearEnv(t)
	echo, other := echoOp("test_echo"), echoOp("test_other")
	c := newTestClient(t, localOptions(), echo)
	second := newTestClient(t, localOptions(), other)
	plain := mustNew(t, localOptions()...)

	if got, ok := c.registry.Op(echo.ID); !ok || got.Route != echo.Route {
		t.Errorf("the test client's registry holds %+v, %t; want the operation %s", got, ok,
			echo.ID)
	}
	if got, want := len(c.registry.Ops()), len(rest.Default().Ops())+1; got != want {
		t.Errorf("the test client's registry holds %d operations, want those of rest.Default "+
			"and the test-only one, %d", got, want)
	}
	for name, registry := range map[string]*rest.Registry{
		"rest.Default()":          rest.Default(),
		"a client that New built": plain.registry,
		"another test client":     second.registry,
	} {
		if _, ok := registry.Op(echo.ID); ok {
			t.Errorf("the registry of %s holds the test-only operation %s", name, echo.ID)
		}
	}
	if _, ok := c.registry.Op(other.ID); ok {
		t.Errorf("the test client holds %s, which another test client registered", other.ID)
	}
}

// TestCloseClosesIdleConnections checks that Close closes the connections the client's own
// http.Client keeps open, and that no goroutine of them keeps running.
func TestCloseClosesIdleConnections(t *testing.T) {
	leak.Check(t)
	clearEnv(t)
	srv, closed := connectionServer(t)
	c := mustNew(t, WithAPIKey(canary.Key("close")), WithAPIURL(srv.URL), WithPDPURL(srv.URL))
	sendOne(t, c.httpClient, srv.URL)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	waitClosed(t, closed, func() {}, "Close did not close the idle connection")
	// Collecting c would close the connection too, so c stays reachable until the wait ends.
	runtime.KeepAlive(c)
}

// openIdleConnection builds a client whose URLs point at url, sends one request through the
// client's own http.Client, and drops the client, which leaves one idle connection open.
func openIdleConnection(t *testing.T, url string) {
	t.Helper()
	c := mustNew(t, WithAPIKey(canary.Key("collected")), WithAPIURL(url), WithPDPURL(url))
	sendOne(t, c.httpClient, url)
}

// TestCollectedClientClosesIdleConnections checks that the idle connections of a client that is
// garbage collected without Close are closed.
func TestCollectedClientClosesIdleConnections(t *testing.T) {
	clearEnv(t)
	srv, closed := connectionServer(t)
	openIdleConnection(t, srv.URL)
	waitClosed(t, closed, runtime.GC,
		"the idle connection of a collected client stayed open")
}

// TestCollectedClientLeavesCallerConnections checks that collecting a client leaves the
// connections of the http.Client it was given open.
func TestCollectedClientLeavesCallerConnections(t *testing.T) {
	clearEnv(t)
	srv, closed := connectionServer(t)
	mine := &http.Client{Transport: defaultTransport(t).Clone()}
	t.Cleanup(mine.CloseIdleConnections)
	func() {
		c := mustNew(t, WithAPIKey(canary.Key("mine")), WithAPIURL(srv.URL),
			WithPDPURL(srv.URL), WithHTTPClient(mine))
		sendOne(t, c.httpClient, srv.URL)
	}()
	for range 5 {
		runtime.GC()
	}
	select {
	case <-closed:
		t.Error("collecting the client closed a connection of the http.Client it was given")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestClientString(t *testing.T) {
	clearEnv(t)
	key := canary.Key("string")
	c := mustNew(t, WithAPIKey(key), WithAPIURL("http://127.0.0.1:1/api/"),
		WithPDPURL("http://127.0.0.1:1/"+key), WithTimeout(1500*time.Millisecond))
	want := `permit.Client{APIKey: "[REDACTED]", APIURL: "http://127.0.0.1:1/api", ` +
		`PDPURL: "http://127.0.0.1:1/[REDACTED]", Timeout: 1.5s}`
	if got := c.String(); got != want {
		t.Errorf("String() =\n%s\nwant\n%s", got, want)
	}
	if got := c.GoString(); got != want {
		t.Errorf("GoString() = %q, want %q", got, want)
	}
	if got := c.LogValue().String(); got != want {
		t.Errorf("LogValue() = %q, want %q", got, want)
	}
	checkFormat(t, c, want)

	var none *Client
	if got := fmt.Sprintf("%v %s %#v", none, none, none); got != "<nil> <nil> <nil>" {
		t.Errorf("a nil *Client prints as %q, want <nil>", got)
	}
	if got := none.LogValue().String(); got != "<nil>" {
		t.Errorf("LogValue() of a nil *Client = %q, want <nil>", got)
	}
	zero := `permit.Client{APIKey: "", APIURL: "", PDPURL: "", Timeout: 0s}`
	if got := new(Client).String(); got != zero {
		t.Errorf("the zero Client prints as %q, want %q", got, zero)
	}
}

// TestAPIKeyIsReplacedFromSixteenBytes checks that a client replaces its API key, here one that
// no pattern finds, in its settings, its errors and its log lines when the key has 16 bytes, and
// leaves a key of 15 bytes as it is: a key that short, such as a placeholder in a test, would
// turn up in unrelated text.
func TestAPIKeyIsReplacedFromSixteenBytes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		key      string
		replaced bool
	}{
		{name: "15 bytes", key: "opaque-key-15-b"},
		{name: "16 bytes", key: "opaque-key-16-by", replaced: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			refuse := func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"detail":"the key `+tc.key+` is not valid"}`)
			}
			srv := httptest.NewServer(http.HandlerFunc(refuse))
			t.Cleanup(srv.Close)
			logger, logged := debugLog()
			c := urlClient(t, tc.key, srv.URL+"/"+tc.key, WithLogger(logger))
			shown := tc.key
			if tc.replaced {
				shown = redact.Marker
			}
			wantURL := `APIURL: "` + srv.URL + "/" + shown + `"`
			if !strings.Contains(c.String(), wantURL) {
				t.Errorf("String() = %s, want it to hold %s", c, wantURL)
			}
			_, err := do[testObject](t.Context(), c, get("a"))
			want := "the key " + shown + " is not valid"
			if got := mustError(t, err).Detail; got != want {
				t.Errorf("Detail = %q, want %q", got, want)
			}
			lines := jsonLines(t, logged())
			if len(lines) != 1 {
				t.Fatalf("the call logged %d lines, want 1: %s", len(lines), logged())
			}
			if got, _ := lines[0]["error"].(string); !strings.HasSuffix(got, want) {
				t.Errorf("the Debug line's error is %q, want it to end with %q", got, want)
			}
		})
	}
}

func TestClientInLogsAndJSON(t *testing.T) {
	clearEnv(t)
	key := canary.Key("logs")
	c := mustNew(t, WithAPIKey(key), WithAPIURL(codeURL), WithPDPURL(codeURL))
	scanner := keyScanner(key)
	for name, handler := range map[string]func(io.Writer) slog.Handler{
		"text": func(w io.Writer) slog.Handler { return slog.NewTextHandler(w, nil) },
		"JSON": func(w io.Writer) slog.Handler { return slog.NewJSONHandler(w, nil) },
	} {
		var out bytes.Buffer
		logger := slog.New(handler(&out))
		logger.Info("built", "client", c, "option", WithAPIKey(key),
			"options", []Option{WithAPIKey(key)}, "holder", struct{ Client *Client }{c})
		_, _ = scanner.Write(out.Bytes())
		for _, want := range []string{"permit.Client{", "[REDACTED]", codeURL} {
			if !bytes.Contains(out.Bytes(), []byte(want)) {
				t.Errorf("the %s handler wrote %s, without %s", name, out.String(), want)
			}
		}
	}
	encoded, err := json.Marshal(struct {
		Client *Client `json:"client"`
		Option Option  `json:"option"`
	}{c, WithAPIKey(key)})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = scanner.Write(encoded)
	scanner.Check(t)
}
