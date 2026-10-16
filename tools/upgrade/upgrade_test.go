package upgrade_test

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	v1config "github.com/permitio/permit-golang/pkg/config"
	v1 "github.com/permitio/permit-golang/pkg/permit"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/tools/drift"
	"github.com/permitio/permit-golang/v2/tools/e2e/fake"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
	"github.com/permitio/permit-golang/v2/tools/upgrade"
)

var dryRun = flag.Bool("upgrade.dryrun", false, "run offline against the fake of tools/e2e/fake")

// The settings of a live run: the scratch environment of an end-to-end job, which
// .github/scripts/e2e-env.sh create exports, and the URL of the Permit API.
const (
	envAPIURL    = "PERMIT_E2E_API_URL"
	envProjectID = "PERMIT_E2E_PROJECT_ID"
	envEnvID     = "PERMIT_E2E_ENV_ID"
	envEnvAPIKey = "PERMIT_E2E_ENV_API_KEY"
)

// The project of a dry run, its API key, and the key of the scratch environment that a dry run
// creates in it.
const (
	dryRunProject = "gosdk-upgrade"
	dryRunEnv     = "gosdk-ci-0-1-upgrade"
)

var dryRunProjectKey = canary.Key("upgradeproject")

// teardownTimeout bounds the teardown of a run.
const teardownTimeout = 5 * time.Minute

// TestUpgrade runs the steps of steps in a scratch environment, in order: each writes objects
// with v1, reads each of them with v2, and checks with upgrade.Diff that v2 read each field that
// v1 wrote. A step whose write fails stops the run, as the steps after it need what it writes.
// Each object is registered in the run's ledger before v1 writes it, and the ledger tears the
// objects down at the end, the last written first, and must reconcile.
func TestUpgrade(t *testing.T) {
	r := start(t)
	old := r.v1Client()
	c := r.v2Client(t)
	for _, s := range steps(r) {
		wrote := false
		t.Run(s.name, func(t *testing.T) {
			parts := r.write(t, s, old)
			wrote = true
			compare(t, s, parts, c)
		})
		if !wrote {
			t.Fatalf("the step %s wrote nothing, and the steps after it need what it writes",
				s.name)
		}
	}
}

// run is a run of TestUpgrade in one scratch environment.
type run struct {
	// apiURL is the URL of the Permit API: in a dry run, the fake's.
	apiURL string

	// env is the scratch environment, and api, with its API key, removes what the run writes in
	// it and checks that it is gone, as ledger, the run's ledger, says.
	env    framework.Environment
	api    *framework.API
	ledger *framework.Ledger

	// sent is the transport of v1's client, which keeps what v1 wrote.
	sent *recorder

	// ids are the IDs of the objects that v1's replies gave, by key, for a step that needs one,
	// such as the resource of a resource set.
	ids map[string]string
}

// start starts a run on t: in a dry run, with a fake of the Permit API and a scratch environment
// that it creates; in a live run, in the scratch environment of the end-to-end job. It skips t
// when neither is set up, and registers the teardown of the run's ledger on t.
func start(t *testing.T) *run {
	t.Helper()
	httpClient := &http.Client{Timeout: time.Minute}
	r := &run{
		ledger: &framework.Ledger{},
		sent:   &recorder{next: http.DefaultTransport, sleep: sleepContext},
		ids:    map[string]string{},
	}
	switch {
	case *dryRun:
		r.apiURL = startFake(t).URL()
		api, err := framework.NewAPI(r.apiURL, dryRunProjectKey, httpClient)
		if err != nil {
			t.Fatal(err)
		}
		r.registerTeardown(t)
		r.env, err = framework.CreateEnvironment(t.Context(), api, r.ledger, dryRunProject,
			dryRunEnv)
		if err != nil {
			t.Fatalf("creating the scratch environment: %v", err)
		}
	default:
		//nolint:forbidigo // The settings of a live run, which no client reads.
		getenv := os.Getenv
		if getenv(envEnvAPIKey) == "" {
			t.Skip("TestUpgrade runs with -upgrade.dryrun, or live in the scratch environment " +
				"that .github/scripts/e2e-env.sh create exports")
		}
		for _, name := range []string{envProjectID, envEnvID} {
			if getenv(name) == "" {
				t.Fatalf("%s is set, and %s is not; a live run needs %s, %s and %s, which "+
					".github/scripts/e2e-env.sh create exports", envEnvAPIKey, name, envProjectID,
					envEnvID, envEnvAPIKey)
			}
		}
		r.apiURL = getenv(envAPIURL)
		if r.apiURL == "" {
			r.apiURL = "https://api.permit.io"
		}
		r.env = framework.ExistingEnvironment(getenv(envProjectID), getenv(envEnvID), "",
			getenv(envEnvAPIKey))
		r.registerTeardown(t)
	}
	api, err := framework.NewAPI(r.apiURL, r.env.APIKey(), httpClient)
	if err != nil {
		t.Fatal(err)
	}
	r.api = api
	return r
}

// registerTeardown registers on t the teardown of the run's ledger, which must reconcile.
func (r *run) registerTeardown(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), teardownTimeout)
		defer cancel()
		if err := r.ledger.Teardown(ctx); err != nil {
			t.Errorf("tearing down the run: %v", err)
		}
		if err := r.ledger.Reconcile(); err != nil {
			t.Errorf("the run may have left objects behind: %v", err)
		}
	})
}

// startFake starts the fake of the Permit API of the end-to-end dry run, on the routes of the
// control plane's spec snapshot, which checks each request body against the spec.
func startFake(t *testing.T) *fake.Fake {
	t.Helper()
	specs, err := drift.ReadSpecs("../specs")
	if err != nil {
		t.Fatalf("reading the spec snapshots: %v", err)
	}
	var routes []fake.Route
	for _, op := range specs.Operations("control-plane") {
		routes = append(routes, fake.Route{Method: op.Method, Template: op.Route, Op: op.ID})
	}
	return fake.New(t, fake.Config{
		Routes: routes, Check: specs.CheckRequestBody, Project: dryRunProject,
		ProjectAPIKey: dryRunProjectKey, Paged: specs.Paged,
	})
}

// v1Client returns a client of v1.3.0 of the scratch environment, whose requests go through the
// run's recorder.
func (r *run) v1Client() *v1.Client {
	cfg := v1config.NewPermitConfig(r.apiURL, r.env.APIKey(), v1config.DefaultPdpUrl, false, nil,
		nil)
	cfg.WithHTTPClient(&http.Client{Transport: r.sent, Timeout: time.Minute})
	return v1.NewPermit(*cfg)
}

// v2Client returns a client of v2 of the scratch environment, which t closes.
func (r *run) v2Client(t *testing.T) *permit.Client {
	t.Helper()
	c, err := permit.New(permit.WithAPIKey(r.env.APIKey()), permit.WithAPIURL(r.apiURL))
	if err != nil {
		t.Fatalf("permit.New: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("closing the client: %v", err)
		}
	})
	return c
}

// schema returns the path of rest in the schema of the scratch environment, such as
// /v2/schema/<project ID>/<environment ID>/roles/<key> for /roles/<key>.
func (r *run) schema(rest string) string {
	return "/v2/schema/" + url.PathEscape(r.env.ProjectID) + "/" + url.PathEscape(r.env.ID) + rest
}

// facts returns the path of rest in the facts of the scratch environment.
func (r *run) facts(rest string) string {
	return "/v2/facts/" + url.PathEscape(r.env.ProjectID) + "/" + url.PathEscape(r.env.ID) + rest
}

// register registers the object what, whose own route is path, in the run's ledger: its
// teardown deletes path and checks that a get of it answers 404.
func (r *run) register(what, path string) {
	r.ledger.Register(what, r.api.Remover(path), r.api.Absence(path, framework.AbsenceTimeout))
}

// step is one write of v1, in one request, and the reads of v2 that check what it wrote.
type step struct {
	// name names the step's subtest, such as tenant.
	name string

	// register registers each object that write creates in the run's ledger, and fails t when
	// it cannot.
	register func(t *testing.T)

	// write writes with v1. Its error is the only result: the run's recorder keeps what v1 sent.
	write func(ctx context.Context, c *v1.Client) error

	// reads read with v2 each object that the write creates: the one object of a body that is a
	// JSON object, or the object of each element, in order, of a body that is a JSON array.
	reads []read
}

// read is a read of v2 of an object that v1 wrote.
type read struct {
	// name says what is read, such as the tenant upgrade-tenant.
	name string

	// get reads the object with v2, and returns what v2 read, as upgrade.View returns it.
	get func(ctx context.Context, c *permit.Client) (any, error)

	// unordered are the pointers of the arrays of the object whose order the API does not keep,
	// such as the permissions of a role.
	unordered []string

	// writeOnly are the exceptions of the comparison: the members of what v1 wrote that the API
	// takes as an instruction of the request, does not store and never returns, each with the
	// reason. The comparison leaves them out, with upgrade.Without, and fails when v1 did not
	// write one.
	writeOnly map[string]string
}

// write registers the objects of s, writes them with v1 and returns the part of what v1 sent
// that each read of s reads, as upgrade.Objects returns it. It fails t when the write fails, or
// v1 sent another number of bodies than one or of objects than s reads.
func (r *run) write(t *testing.T, s step, c *v1.Client) []any {
	t.Helper()
	s.register(t)
	if err := s.write(t.Context(), c); err != nil {
		t.Fatalf("v1 could not write: %v", err)
	}
	parts, err := upgrade.Objects(r.sent.take(), len(s.reads))
	if err != nil {
		t.Fatalf("the step reads %d objects: %v", len(s.reads), err)
	}
	return parts
}

// compare reads with v2 each object of s, whose part of what v1 wrote is in parts, and reports
// on t each place where what v2 read differs from it, but for the members of writeOnly, and
// each member of writeOnly that v1 did not write.
func compare(t *testing.T, s step, parts []any, c *permit.Client) {
	t.Helper()
	for i, rd := range s.reads {
		names := slices.Sorted(maps.Keys(rd.writeOnly))
		written, err := upgrade.Without(parts[i], names...)
		if err != nil {
			t.Errorf("%s: %v; change its writeOnly", rd.name, err)
			continue
		}
		for _, name := range names {
			t.Logf("%s: the comparison leaves out #/%s: %s", rd.name, name, rd.writeOnly[name])
		}
		got, err := rd.get(t.Context(), c)
		if err != nil {
			t.Errorf("v2 could not read %s: %v", rd.name, err)
			continue
		}
		for _, place := range upgrade.Diff(written, got, rd.unordered...) {
			t.Errorf("%s: %s", rd.name, place)
		}
	}
}

// view returns what v2 read, model, as upgrade.View returns it, or the error of the read.
func view(model any, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return upgrade.View(model)
}

// only returns the view of the one item of page, the page of a list of what, or an error when
// the list failed or holds another number of items.
func only[T any](what string, page *permit.Page[T], err error) (any, error) {
	switch {
	case err != nil:
		return nil, err
	case len(page.Data) != 1:
		return nil, fmt.Errorf("the list of %s holds %d items, want 1: %+v", what,
			len(page.Data), page.Data)
	}
	return upgrade.View(page.Data[0])
}

// The retries of a request of v1 that the API answers with status 429: the most attempts, the
// first wait of the backoff, which a reply without a Retry-After header in seconds gets, and the
// longest wait.
const (
	retryAttempts = 6
	retryFirst    = time.Second
	retryMax      = 30 * time.Second
)

// recorder is the transport of v1's client. It keeps the body of each request that v1 sends with
// one, which is what v1 wrote, and sends a request again after a reply with status 429, up to
// retryAttempts in all, so that the test survives the rate limits of a shared project: v1 does
// not retry. Before it sends a request again it waits the seconds that the reply's Retry-After
// header gives, 0 included, and else the backoff, which starts at retryFirst for each request,
// doubles after each wait that it gives, and starts again from retryFirst after a reply with a
// Retry-After; no wait is longer than retryMax. It is safe for concurrent use.
type recorder struct {
	next http.RoundTripper

	// sleep waits d before a request is sent again, or until ctx is done, and then returns
	// ctx's error: sleepContext, or a fake of a test.
	sleep func(ctx context.Context, d time.Duration) error

	mu     sync.Mutex
	bodies [][]byte
}

// sleepContext waits d, or until ctx is done, and then returns ctx's error.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RoundTrip records the body of req and sends req with next.
func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if closeErr := req.Body.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, fmt.Errorf("reading the body of %s %s: %w", req.Method, req.URL.Path, err)
		}
	}
	if len(body) > 0 {
		r.mu.Lock()
		r.bodies = append(r.bodies, body)
		r.mu.Unlock()
	}
	backoff := retryFirst
	for attempt := 1; ; attempt++ {
		try := req.Clone(req.Context())
		try.Body = http.NoBody
		if len(body) > 0 {
			try.Body = io.NopCloser(bytes.NewReader(body))
		}
		resp, err := r.next.RoundTrip(try)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests || attempt == retryAttempts {
			return resp, nil
		}
		_, _ = io.Copy(io.Discard, resp.Body) // The reply of a 429 that is sent again.
		_ = resp.Body.Close()
		wait := backoff
		if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil &&
			seconds >= 0 {
			wait, backoff = time.Duration(seconds)*time.Second, retryFirst
		} else {
			backoff *= 2
		}
		if err := r.sleep(req.Context(), min(wait, retryMax)); err != nil {
			return nil, err
		}
	}
}

// take returns the bodies that v1 sent since the last take, and forgets them.
func (r *recorder) take() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	bodies := r.bodies
	r.bodies = nil
	return bodies
}

// TestRecorder checks the recorder of v1's requests: it keeps the body of each request that
// has one, and only once when it sends the request again after a 429, which it does with the
// same body until the reply is not 429 or the last attempt got 429. Before it sends a request
// again it waits the seconds of the reply's Retry-After, 0 included, and else the backoff, which
// starts at retryFirst for each request, doubles after each wait that it gives, and starts again
// from retryFirst after a reply with a Retry-After; no wait is longer than retryMax.
func TestRecorder(t *testing.T) {
	type reply struct {
		status     int
		retryAfter string
	}
	var (
		mu       sync.Mutex
		received []string
		// The replies to the POST, in order; the GET after it always gets 429 without a
		// Retry-After.
		replies = []reply{
			{http.StatusTooManyRequests, ""},
			{http.StatusTooManyRequests, "soon"},
			{http.StatusTooManyRequests, "0"},
			{http.StatusTooManyRequests, ""},
			{http.StatusTooManyRequests, "120"},
			{http.StatusOK, ""},
		}
		waits []time.Duration
	)
	next := roundTripper(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		var data []byte
		if req.Body != nil {
			var err error
			if data, err = io.ReadAll(req.Body); err != nil {
				return nil, err
			}
		}
		received = append(received, req.Method+" "+string(data))
		answer := reply{status: http.StatusTooManyRequests}
		if len(replies) > 0 {
			answer, replies = replies[0], replies[1:]
		}
		header := http.Header{}
		if answer.retryAfter != "" {
			header.Set("Retry-After", answer.retryAfter)
		}
		return &http.Response{
			StatusCode: answer.status, Header: header,
			Body: io.NopCloser(strings.NewReader("{}")),
		}, nil
	})
	rec := &recorder{next: next, sleep: func(_ context.Context, d time.Duration) error {
		mu.Lock()
		defer mu.Unlock()
		waits = append(waits, d)
		return nil
	}}
	client := &http.Client{Transport: rec}
	send := func(method, body string) int {
		t.Helper()
		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}
		req, err := http.NewRequestWithContext(t.Context(), method, "http://127.0.0.1:1/x", reader)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	if status := send(http.MethodPost, `{"key":"k"}`); status != http.StatusOK {
		t.Errorf("POST after five 429s: status %d, want 200", status)
	}
	if status := send(http.MethodGet, ""); status != http.StatusTooManyRequests {
		t.Errorf("GET that always gets 429: status %d, want 429 after the last attempt", status)
	}

	var want []string
	for range retryAttempts {
		want = append(want, `POST {"key":"k"}`)
	}
	for range retryAttempts {
		want = append(want, "GET ")
	}
	if strings.Join(received, "\n") != strings.Join(want, "\n") {
		t.Errorf("the transport received\n%s\nwant\n%s", strings.Join(received, "\n"),
			strings.Join(want, "\n"))
	}
	wantWaits := []time.Duration{
		// The POST: the backoff without a Retry-After, and again, doubled, after one that gives
		// no seconds; the Retry-After of 0; the backoff, which starts again from retryFirst; and
		// retryMax in place of the Retry-After of 120 seconds.
		retryFirst, 2 * retryFirst, 0, retryFirst, retryMax,
		// The GET: the backoff, from retryFirst, doubled after each wait.
		retryFirst, 2 * retryFirst, 4 * retryFirst, 8 * retryFirst, 16 * retryFirst,
	}
	if !slices.Equal(waits, wantWaits) {
		t.Errorf("the recorder waited %v, want %v", waits, wantWaits)
	}
	if bodies := rec.take(); len(bodies) != 1 || string(bodies[0]) != `{"key":"k"}` {
		t.Errorf("take() = %q, want the POST's body once", bodies)
	}
	if bodies := rec.take(); len(bodies) != 0 {
		t.Errorf("a second take() = %q, want nothing", bodies)
	}
}

// TestRecorderStopsWaitingWhenTheContextEnds checks that the recorder, with sleepContext, stops
// waiting to send a request again after a 429 when the request's context ends, and returns the
// context's error, and that sleepContext otherwise waits and returns nil.
func TestRecorderStopsWaitingWhenTheContextEnds(t *testing.T) {
	if err := sleepContext(t.Context(), time.Millisecond); err != nil {
		t.Errorf("sleepContext with a live context: %v, want nil", err)
	}
	attempts := 0
	rec := &recorder{
		next: roundTripper(func(*http.Request) (*http.Response, error) {
			attempts++
			return &http.Response{
				StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": {"60"}},
				Body: io.NopCloser(strings.NewReader("{}")),
			}, nil
		}),
		sleep: sleepContext,
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:1/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := rec.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Errorf("RoundTrip with a context that ended: %v after %d attempts, want %v after 1",
			err, attempts, context.Canceled)
	}
}

// roundTripper is an http.RoundTripper that is a function.
type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
