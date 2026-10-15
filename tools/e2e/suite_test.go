//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testhook"
	"github.com/permitio/permit-golang/v2/tools/drift"
	"github.com/permitio/permit-golang/v2/tools/e2e/fake"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
)

// teardownTimeout bounds the teardown of a suite.
const teardownTimeout = 5 * time.Minute

// scenario is a scenario of the cloud lane, which runs in the suite's scratch environment.
type scenario struct {
	name string

	// methods are the public methods that the scenario covers, such as Client.Scope.
	// TestE2ESurfaceComplete counts such a method as cloud when the scenario's dry run sends
	// each of the method's operations.
	methods []string

	// run returns the body of the scenario's subtest in the suite s.
	run func(s *suite) func(t *testing.T)
}

// cloudScenarios are the scenarios of the cloud lane.
var cloudScenarios = []scenario{
	{"scope_single_flight", []string{"Client.Scope"}, scopeSingleFlight},
	{"unauthorized_key", []string{"Client.Scope"}, unauthorizedKey},
}

// TestCloud runs the scenarios of the cloud lane, then checks that the Debug logs of every
// client of the run hold no API key, and that the record holds a request of each scenario that
// passed: a scenario that passes without one checked nothing against the API.
func TestCloud(t *testing.T) {
	if run.cfg.mode == "" {
		t.Skip(run.skip)
	}
	passed := runCloud(t, run.cfg, run.record)
	requests := readRecord(t, run.recordPath, run.cfg.mode)
	for _, name := range unrecorded(requests, passed) {
		markBroken(t, "%s passed, and the record holds no request of it", name)
	}
	t.Logf("the record holds %d requests of the %d scenarios that passed", len(requests),
		len(passed))
}

// runCloud runs each of cloudScenarios as a subtest of t in a suite of cfg that records to
// record, then checks the suite's logs. It returns the names of the scenarios' subtests that
// passed, and did not skip.
func runCloud(t *testing.T, cfg config, record *testhook.File) []string {
	t.Helper()
	s := startSuite(t, cfg, record)
	var passed []string
	for _, sc := range cloudScenarios {
		var name string
		skipped := false
		ok := t.Run(sc.name, func(t *testing.T) {
			name = t.Name()
			defer func() { skipped = t.Skipped() }()
			sc.run(s)(t)
		})
		// A subtest that -run leaves out has no name, as it never ran.
		if ok && name != "" && !skipped {
			passed = append(passed, name)
		}
	}
	t.Run("logs_hold_no_key", logsHoldNoKey(s))
	return passed
}

// unrecorded returns each of passed, the names of the scenarios' subtests that passed, that sent
// none of requests, nor did a subtest of it.
func unrecorded(requests []recorded, passed []string) []string {
	var missing []string
	for _, name := range passed {
		if !slices.ContainsFunc(requests, func(r recorded) bool {
			return r.Test == name || strings.HasPrefix(r.Test, name+"/")
		}) {
			missing = append(missing, name)
		}
	}
	return missing
}

// TestUnrecorded checks which scenarios that passed the record holds no request of: a request of
// the scenario or of a subtest of it counts, and one of another scenario whose name starts the
// same does not.
func TestUnrecorded(t *testing.T) {
	requests := []recorded{
		{Test: "TestCloud/a"}, {Test: "TestCloud/b/sub"}, {Test: "TestCloud/cc"},
	}

	got := unrecorded(requests, []string{"TestCloud/a", "TestCloud/b", "TestCloud/c"})

	if want := []string{"TestCloud/c"}; !slices.Equal(got, want) {
		t.Errorf("unrecorded = %q, want %q", got, want)
	}
	if got := unrecorded(nil, nil); got != nil {
		t.Errorf("unrecorded of no scenario = %q, want none", got)
	}
}

// suite is a run of the scenarios in one scratch environment.
type suite struct {
	cfg    config
	record *testhook.File

	// apiURL is the URL of the Permit API that the suite's clients call: in a dry run, the
	// fake's, which is their PDP's URL too.
	apiURL string

	// env is the scratch environment.
	env framework.Environment

	// logs holds what the suite's clients log at Debug, and logger writes to it.
	logs   *lockedBuffer
	logger *slog.Logger

	// secrets are the API keys of the run, which no log may hold.
	secrets []string
}

// lockedBuffer is a bytes.Buffer that is safe for concurrent use.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// Bytes returns a copy of what the buffer holds.
func (b *lockedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.buf.Bytes())
}

// newLogger returns a logger that writes what it logs at Debug and above to w as text, as each
// client of a suite logs.
func newLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// startSuite starts a suite of cfg on t, whose clients record to record. A dry run starts a fake
// and creates the scratch environment, whose teardown it registers on t first; a live run uses
// the one that e2e-env.sh created, and deletes after the run.
func startSuite(t *testing.T, cfg config, record *testhook.File) *suite {
	t.Helper()
	s := &suite{
		cfg: cfg, record: record, apiURL: cfg.apiURL, logs: &lockedBuffer{},
		secrets: []string{cfg.projectKey},
	}
	s.logger = newLogger(s.logs)
	if cfg.mode == testhook.DryRun {
		s.apiURL = startFake(t).URL()
	}
	api, apiErr := framework.NewAPI(s.apiURL, cfg.projectKey, &http.Client{Timeout: time.Minute})
	if apiErr != nil {
		markBroken(t, "%v", apiErr)
		t.FailNow()
	}
	ledger := newLedger(t.Name())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), teardownTimeout)
		defer cancel()
		if err := ledger.Teardown(ctx); err != nil {
			t.Errorf("tearing down the run: %v", err)
		}
	})
	if cfg.mode == testhook.Live {
		s.env = cfg.env
	} else {
		var err error
		s.env, err = framework.CreateEnvironment(t.Context(), api, ledger, cfg.project,
			string(cfg.prefix))
		if err != nil {
			t.Fatalf("creating the scratch environment: %v", err)
		}
	}
	s.secrets = append(s.secrets, s.env.APIKey())
	return s
}

// specs returns the committed spec snapshots, which it reads once.
var specs = sync.OnceValues(func() (*drift.Specs, error) {
	return drift.ReadSpecs("../specs")
})

// startFake starts a fake of the routes of the control plane's spec, which checks each request
// body with the structural walker of tools/drift.
func startFake(t *testing.T) *fake.Fake {
	t.Helper()
	s, err := specs()
	if err != nil {
		markBroken(t, "reading the spec snapshots: %v", err)
		t.FailNow()
	}
	var routes []fake.Route
	for _, op := range s.Operations("control-plane") {
		routes = append(routes, fake.Route{Method: op.Method, Template: op.Route, Op: op.ID})
	}
	return fake.New(t, fake.Config{
		Routes: routes, Check: s.CheckRequestBody, Project: dryRunProject,
		ProjectAPIKey: dryRunProjectKey,
	})
}

// client returns a client of the scratch environment that authenticates with key, has opts,
// and logs to the suite's logs at Debug. In a dry run, its PDP is the fake too. Its calls take
// the context that ctx returns.
func (s *suite) client(t *testing.T, key string, opts ...permit.Option) *permit.Client {
	t.Helper()
	all := []permit.Option{
		permit.WithAPIKey(key), permit.WithAPIURL(s.apiURL), permit.WithLogger(s.logger),
	}
	if s.cfg.mode == testhook.DryRun {
		all = append(all, permit.WithPDPURL(s.apiURL))
	}
	all = append(all, opts...)
	c, err := permit.New(all...)
	if err != nil {
		t.Fatalf("permit.New: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("closing the client: %v", err)
		}
	})
	s.logger.Debug("a client of the run", "client", c)
	return c
}

// ctx returns the context of t, which carries the recorder of t's requests.
func (s *suite) ctx(t *testing.T) context.Context {
	t.Helper()
	rec, err := testhook.NewRecorder(s.record, s.cfg.mode, t.Name())
	if err != nil {
		markBroken(t, "%v", err)
		t.FailNow()
	}
	return testhook.WithRecorder(t.Context(), rec)
}
