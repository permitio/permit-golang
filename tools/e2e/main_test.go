//go:build e2e

// Package e2e holds the end-to-end scenarios of the SDK, which build only with the tag e2e.
//
// TestCloud runs the scenarios of the cloud lane in a scratch environment. With -e2e.dryrun it
// runs offline against the fake of tools/e2e/fake, in an environment that it creates for the
// run and deletes at its end. Otherwise it runs against the Permit API, or the one at
// PERMIT_E2E_API_URL, with the project-level API key in PERMIT_E2E_PROJECT_KEY of the project in
// PERMIT_E2E_PROJECT, in the environment that .github/scripts/e2e-env.sh created for the run and
// deletes after it, whose API key the leg's container PDP uses too; it skips when
// PERMIT_E2E_PROJECT_KEY is not set. A live run runs the scenarios of every PDP and those of the
// PDP that its leg's clients call, at PERMIT_PDP_URL: the cloud PDP on the leg cloudpdp, and the
// PDP container on every other, pinned by default; a dry run runs those of the leg cloudpdp
// against a fake cloud PDP, then the others against a fake PDP container, beside the fake of the
// Permit API. Each call of the SDK carries the request recorder of
// internal/testhook, in the mode dryrun or live, and -e2e.record keeps the record for
// tools/cmd/apicoverage; each scenario that passes must have sent a request of that mode.
// TestE2ESurfaceComplete runs the scenarios in a dry run of its own and checks that each public
// method is in exactly one end-to-end state, as tools/e2e/framework/surface describes.
//
// The run exits 2, DID NOT RUN, when the ledger of what it created does not reconcile, which
// means that it may have left objects behind, when a dry run's fake still holds, after its
// teardown, an object that the run created, which the run's ledger never removes, when its
// configuration or an input of the gate is broken, or when its record cannot be written; 1,
// FAIL, when a test fails; and 0, PASS, otherwise. CONTRIBUTING.md says how to run it.
package e2e

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/internal/testhook"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
)

var (
	dryRun = flag.Bool("e2e.dryrun", false, "run the scenarios offline against the fake of "+
		"tools/e2e/fake")
	recordFile = flag.String("e2e.record", "", "write the request record of TestCloud to `file`, "+
		"which must not exist, for tools/cmd/apicoverage")
	leg = flag.String("e2e.leg", pinnedLeg, "the leg of a live run, the last part of its prefix: "+
		"by default "+pinnedLeg+", the leg of the pinned PDP container that the job e2e of "+
		"ci.yml runs; the leg "+cloudPDPLeg+" runs against the cloud PDP")
)

// pinnedLeg is the leg of the pinned PDP container, which ci.yml passes, and cloudPDPLeg the leg
// whose clients call the cloud PDP. The clients of every leg but cloudPDPLeg call the PDP
// container.
const (
	pinnedLeg   = "pinned"
	cloudPDPLeg = "cloudpdp"
)

// pdpKind is the kind of PDP that the clients of a leg call, and that a scenario needs.
type pdpKind string

const (
	// everyPDP is the PDP of a scenario that every leg runs.
	everyPDP pdpKind = ""

	// containerPDP is the PDP container, such as the one of the leg pinned.
	containerPDP pdpKind = "container"

	// cloudPDP is the cloud PDP, of the leg cloudpdp.
	cloudPDP pdpKind = "cloud"
)

// legPDP returns the kind of PDP that the clients of the leg legName call.
func legPDP(legName string) pdpKind {
	if legName == cloudPDPLeg {
		return cloudPDP
	}
	return containerPDP
}

// How long a scenario waits for its seed to reach the PDP, and how long between two tries: a live
// PDP syncs the seed from the Permit API, and the fake PDP of a dry run reads it at once, so in a
// dry run only a scenario that fails waits it out.
const (
	liveSync           = 2 * time.Minute
	liveSyncInterval   = 2 * time.Second
	dryRunSync         = 2 * time.Second
	dryRunSyncInterval = 100 * time.Millisecond
)

// The environment variables of a live run. .github/scripts/e2e-env.sh create exports the last
// three, the scratch environment's.
const (
	envProjectKey = "PERMIT_E2E_PROJECT_KEY"
	envProject    = "PERMIT_E2E_PROJECT"
	envAPIURL     = "PERMIT_E2E_API_URL"
	envRun        = "GITHUB_RUN_ID"
	envAttempt    = "GITHUB_RUN_ATTEMPT"
	envProjectID  = "PERMIT_E2E_PROJECT_ID"
	envEnvID      = "PERMIT_E2E_ENV_ID"
	envEnvAPIKey  = "PERMIT_E2E_ENV_API_KEY"
)

// The project, its API key and the run prefix of a dry run.
const dryRunProject = "gosdk-e2e"

var (
	dryRunProjectKey = canary.Key("e2eproject")
	dryRunPrefix     = framework.Prefix("gosdk-ci-0-1-dryrun")
)

// config is how a run reaches the Permit API.
type config struct {
	// mode is dryrun or live.
	mode testhook.Mode

	// apiURL is the URL of the Permit API, empty in a dry run, which starts a fake.
	apiURL string

	// project is the key or the ID of the project of the scratch environments.
	project string

	// projectKey is the project-level API key of the project.
	projectKey string

	// prefix is the run prefix, the scratch environment's key.
	prefix framework.Prefix

	// env is the scratch environment of a live run, which e2e-env.sh created; a dry run creates
	// its own.
	env framework.Environment

	// pdp is the kind of PDP that the clients of a live run call, as its leg says. A dry run
	// fakes both and runs the scenarios of each leg, as legs says.
	pdp pdpKind

	// syncTimeout bounds the wait of a scenario for its seed to reach the PDP, and syncInterval
	// is the wait between two tries.
	syncTimeout, syncInterval time.Duration
}

// legs returns the kinds of PDP of the legs whose scenarios a run of c runs, in turn: the one
// that a live run's clients call; and in a dry run, the cloud PDP, then the PDP container. The
// fake cloud PDP answers the routes that both serve as the fake PDP container does, so a dry run
// runs each scenario of every PDP against it, which shows that the scenario sends no route that
// only the PDP container serves.
func (c config) legs() []pdpKind {
	if c.mode == testhook.DryRun {
		return []pdpKind{cloudPDP, containerPDP}
	}
	return []pdpKind{c.pdp}
}

// selects reports whether a leg whose clients call the PDP kind runs sc: a scenario of every PDP,
// or of that PDP. A scenario that a leg does not select does not run on it, and does not skip.
func selects(kind pdpKind, sc scenario) bool {
	return sc.pdp == everyPDP || sc.pdp == kind
}

// planned is a scenario that a run runs, with the kind of PDP that its clients call: in a dry
// run, the fake of which.
type planned struct {
	scenario

	calls pdpKind
}

// plan returns the scenarios of all that a run of c runs, in the order in which runCloud runs
// them: those that the leg of each of c.legs() selects and no leg before it, each with that leg's
// kind of PDP. A live run thus runs what its leg selects, and a dry run every scenario once, as a
// leg that selects it would.
func (c config) plan(all []scenario) []planned {
	var p []planned
	for _, kind := range c.legs() {
		for _, sc := range all {
			if selects(kind, sc) && !slices.ContainsFunc(p, func(other planned) bool {
				return other.name == sc.name
			}) {
				p = append(p, planned{scenario: sc, calls: kind})
			}
		}
	}
	return p
}

// dryRunConfig is the config of a dry run.
var dryRunConfig = config{
	mode: testhook.DryRun, project: dryRunProject, projectKey: dryRunProjectKey,
	prefix: dryRunPrefix, syncTimeout: dryRunSync, syncInterval: dryRunSyncInterval,
}

// run is the state of the test binary's run, which TestMain reads after the tests.
var run struct {
	// cfg is the config of TestCloud, and skip why it skips when cfg has no mode.
	cfg  config
	skip string

	// record is the request record of TestCloud, and recordPath its file.
	record     *testhook.File
	recordPath string

	mu sync.Mutex

	// ledgers are the ledgers of the suites, by the name of the test that ran each.
	ledgers map[string]*framework.Ledger

	// broken are the reasons that the run is not valid.
	broken []string
}

// newLedger returns a new ledger of the test name, which TestMain reconciles after the tests.
func newLedger(name string) *framework.Ledger {
	l := &framework.Ledger{}
	run.mu.Lock()
	defer run.mu.Unlock()
	if run.ledgers == nil {
		run.ledgers = map[string]*framework.Ledger{}
	}
	run.ledgers[name] = l
	return l
}

// markBroken fails tb, and makes the run exit 2, as it did not run as it should.
func markBroken(tb testing.TB, format string, args ...any) {
	tb.Helper()
	text := fmt.Sprintf(format, args...)
	tb.Error(text)
	run.mu.Lock()
	defer run.mu.Unlock()
	run.broken = append(run.broken, tb.Name()+": "+text)
}

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(runMain(m))
}

// runMain runs the tests and returns the exit code of the run, after it prints the verdict.
func runMain(m *testing.M) int {
	cfg, skip, err := configure()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: DID NOT RUN: %v\n", err)
		return 2
	}
	run.cfg, run.skip = cfg, skip
	path := *recordFile
	if path == "" {
		dir, tempErr := os.MkdirTemp("", "e2e-record-")
		if tempErr != nil {
			fmt.Fprintf(os.Stderr, "e2e: DID NOT RUN: %v\n", tempErr)
			return 2
		}
		defer os.RemoveAll(dir)
		path = filepath.Join(dir, "record.jsonl")
	}
	if run.record, err = testhook.Create(path); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: DID NOT RUN: %v\n", err)
		return 2
	}
	run.recordPath = path
	code := m.Run()
	if err := run.record.Close(code); err != nil {
		run.broken = append(run.broken, err.Error())
	}
	return verdict(os.Stderr, code, run.broken, run.ledgers)
}

// verdict returns the exit code of a run whose tests exited code: 2 when broken holds a reason
// that the run is not valid or a ledger of ledgers, by the name of its test, does not reconcile,
// else code. It prints each such reason and then the verdict to w.
func verdict(w io.Writer, code int, broken []string, ledgers map[string]*framework.Ledger) int {
	broken = slices.Clone(broken)
	for _, name := range slices.Sorted(maps.Keys(ledgers)) {
		if err := ledgers[name].Reconcile(); err != nil {
			broken = append(broken, name+": "+err.Error())
		}
	}
	switch {
	case len(broken) > 0:
		for _, b := range broken {
			fmt.Fprintf(w, "e2e: %s\n", b)
		}
		fmt.Fprintln(w, "e2e: DID NOT RUN")
		return 2
	case code != 0:
		fmt.Fprintln(w, "e2e: FAIL")
		return code
	}
	fmt.Fprintln(w, "e2e: PASS")
	return 0
}

// configure returns the config of TestCloud: a dry run with -e2e.dryrun, else a live run when
// PERMIT_E2E_PROJECT_KEY is set, else no mode and why TestCloud skips. Its error says what a
// live run lacks.
func configure() (config, string, error) {
	if *dryRun {
		return dryRunConfig, "", nil
	}
	//nolint:forbidigo // The settings of a live run, which no client reads.
	getenv := os.Getenv
	if getenv(envProjectKey) == "" {
		return config{}, "TestCloud runs with -e2e.dryrun, or live with " + envProjectKey +
			" and " + envProject + " set", nil
	}
	for _, name := range []string{
		envProject, envRun, envAttempt, envProjectID, envEnvID, envEnvAPIKey,
	} {
		if getenv(name) == "" {
			return config{}, "", fmt.Errorf("%s is set, and %s is not; a live run needs %s, %s, "+
				"%s and %s, and the scratch environment's %s, %s and %s, which "+
				".github/scripts/e2e-env.sh create exports", envProjectKey, name, envProjectKey,
				envProject, envRun, envAttempt, envProjectID, envEnvID, envEnvAPIKey)
		}
	}
	prefix, err := framework.NewPrefix(getenv(envRun), getenv(envAttempt), *leg)
	if err != nil {
		return config{}, "", err
	}
	apiURL := getenv(envAPIURL)
	if apiURL == "" {
		apiURL = "https://api.permit.io"
	}
	return config{
		mode: testhook.Live, apiURL: apiURL, project: getenv(envProject),
		projectKey: getenv(envProjectKey), prefix: prefix,
		env: framework.ExistingEnvironment(getenv(envProjectID), getenv(envEnvID), string(prefix),
			getenv(envEnvAPIKey)),
		pdp: legPDP(*leg), syncTimeout: liveSync, syncInterval: liveSyncInterval,
	}, "", nil
}

// TestVerdict checks the exit code and the verdict that TestMain gives a run: a ledger that does
// not reconcile, which means that the run may have left objects behind, or another reason that
// the run is not valid makes it DID NOT RUN, with exit code 2.
func TestVerdict(t *testing.T) {
	nothing := func(context.Context) error { return nil }
	left := &framework.Ledger{}
	left.Register("environment e1", nothing, nothing)
	done := &framework.Ledger{}
	done.Register("environment e2", nothing, nothing)
	if err := done.Teardown(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		code    int
		broken  []string
		ledgers map[string]*framework.Ledger
		want    int
		out     string
	}{
		{
			name: "pass", ledgers: map[string]*framework.Ledger{"TestA": done},
			want: 0, out: "e2e: PASS\n",
		},
		{
			name: "fail", code: 1, ledgers: map[string]*framework.Ledger{"TestA": done},
			want: 1, out: "e2e: FAIL\n",
		},
		{
			name: "ledger", code: 1,
			ledgers: map[string]*framework.Ledger{"TestA": done, "TestB": left},
			want:    2,
			out: "e2e: TestB: the ledger does not reconcile, registered=1 completed=0 " +
				"verified=0: environment e1 (not removed)\ne2e: DID NOT RUN\n",
		},
		{
			name: "broken", broken: []string{"the record: disk full"},
			want: 2, out: "e2e: the record: disk full\ne2e: DID NOT RUN\n",
		},
	} {
		var out bytes.Buffer
		if got := verdict(&out, tc.code, tc.broken, tc.ledgers); got != tc.want ||
			out.String() != tc.out {
			t.Errorf("%s: verdict = %d, printing %q; want %d, printing %q", tc.name, got,
				out.String(), tc.want, tc.out)
		}
	}
}

// TestConfigure checks the settings of a run without -e2e.dryrun: a live run in the scratch
// environment whose ID, project ID and API key .github/scripts/e2e-env.sh create exports, a run
// that fails without one of them, and one that skips without the project's key.
func TestConfigure(t *testing.T) {
	saved := *dryRun
	*dryRun = false
	t.Cleanup(func() { *dryRun = saved })
	live := map[string]string{
		envProjectKey: "permit_key_FAKEproject", envProject: "gosdk-e2e", envAPIURL: "",
		envRun: "1234", envAttempt: "2", envProjectID: "project-id-1", envEnvID: "env-id-1",
		envEnvAPIKey: "permit_key_FAKEenvironment",
	}
	setenv := func(t *testing.T, without string) {
		t.Helper()
		for name, value := range live {
			if name == without {
				value = ""
			}
			t.Setenv(name, value)
		}
	}

	t.Run("live", func(t *testing.T) {
		setenv(t, "")
		cfg, skip, err := configure()
		prefix := "gosdk-ci-1234-2-" + *leg
		want := framework.ExistingEnvironment("project-id-1", "env-id-1", prefix,
			"permit_key_FAKEenvironment")
		if err != nil || skip != "" || cfg.mode != testhook.Live || cfg.env != want ||
			string(cfg.prefix) != prefix || cfg.project != "gosdk-e2e" ||
			cfg.projectKey != "permit_key_FAKEproject" || cfg.apiURL != "https://api.permit.io" ||
			cfg.pdp != legPDP(*leg) || cfg.syncTimeout != liveSync ||
			cfg.syncInterval != liveSyncInterval {
			t.Errorf("configure() = %+v (environment %s), %q, %v; want a live run in %s", cfg,
				cfg.env, skip, err, want)
		}
	})
	t.Run("the cloud PDP's leg", func(t *testing.T) {
		savedLeg := *leg
		*leg = cloudPDPLeg
		t.Cleanup(func() { *leg = savedLeg })
		setenv(t, "")
		cfg, _, err := configure()
		if err != nil || cfg.pdp != cloudPDP || string(cfg.prefix) != "gosdk-ci-1234-2-cloudpdp" {
			t.Errorf("configure() = %+v, %v; want a live run of the cloud PDP", cfg, err)
		}
	})
	for _, name := range []string{envEnvID, envProjectID, envEnvAPIKey, envRun} {
		t.Run("without "+name, func(t *testing.T) {
			setenv(t, name)
			_, _, err := configure()
			if err == nil || !strings.Contains(err.Error(), name+" is not") ||
				!strings.Contains(err.Error(), ".github/scripts/e2e-env.sh create exports") {
				t.Errorf("configure() returned %v, want an error that names %s and e2e-env.sh",
					err, name)
			}
		})
	}
	t.Run("without the project's key", func(t *testing.T) {
		setenv(t, envProjectKey)
		cfg, skip, err := configure()
		if err != nil || cfg.mode != "" || !strings.Contains(skip, envProjectKey) {
			t.Errorf("configure() = %+v, %q, %v; want no mode and why TestCloud skips", cfg,
				skip, err)
		}
	})
}

// TestLegPDP checks the kind of PDP that the clients of each leg call: the cloud PDP on the leg
// cloudpdp, and the PDP container on every other, such as pinned, the leg of a run without
// -e2e.leg, latest and local.
func TestLegPDP(t *testing.T) {
	for legName, want := range map[string]pdpKind{
		"cloudpdp": cloudPDP, "pinned": containerPDP, "latest": containerPDP,
		"local": containerPDP,
	} {
		if got := legPDP(legName); got != want {
			t.Errorf("legPDP(%q) = %q, want %q", legName, got, want)
		}
	}
	if f := flag.Lookup("e2e.leg"); f == nil || f.DefValue != pinnedLeg {
		t.Errorf("-e2e.leg is %+v, want the default %s, the leg that ci.yml's job e2e passes", f,
			pinnedLeg)
	}
}

// TestPlan checks the scenarios that each run runs, in order, and the PDP that their clients
// call: a live run those of every PDP and those of the PDP of its leg, against that PDP; a dry
// run those of the leg of the cloud PDP against it, then those of the PDP container against it.
func TestPlan(t *testing.T) {
	all := []scenario{
		{name: "every"},
		{name: "container", pdp: containerPDP},
		{name: "cloud", pdp: cloudPDP},
		{name: "every-2"},
	}
	for _, tc := range []struct {
		name string
		cfg  config
		want []string
	}{
		{"dry run", dryRunConfig, []string{
			"every@cloud", "cloud@cloud", "every-2@cloud", "container@container",
		}},
		{"container", config{mode: testhook.Live, pdp: containerPDP}, []string{
			"every@container", "container@container", "every-2@container",
		}},
		{"cloud", config{mode: testhook.Live, pdp: cloudPDP}, []string{
			"every@cloud", "cloud@cloud", "every-2@cloud",
		}},
	} {
		var got []string
		for _, p := range tc.cfg.plan(all) {
			got = append(got, p.name+"@"+string(p.calls))
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: plan = %q, want %q", tc.name, got, tc.want)
		}
	}
}
