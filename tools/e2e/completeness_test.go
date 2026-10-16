//go:build e2e

package e2e

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/testhook"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework/surface"
)

// The inputs of the completeness gate.
var surfaceFiles = surface.Files{
	OperationMap: "../../testdata/operation-map.json",
	LocalOnly:    "local-only.txt",
	Advisory:     "advisory.txt",
	Pending:      "pending.txt",
	Keys:         "keys.yaml",
	AllowedSkips: "../../.github/allowed-skips.txt",
}

// TestE2ESurfaceComplete runs the cloud scenarios in a dry run of its own, then checks, as
// tools/e2e/framework/surface describes, that none of them sends an operation that needs an
// organization-level key while keys.yaml gives the cloud lane none, and that each public method
// is in exactly one end-to-end state: cloud when a scenario covers it and its dry run sent each
// of the method's operations, or listed in local-only.txt, advisory.txt or pending.txt.
func TestE2ESurfaceComplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dryrun.jsonl")
	record, err := testhook.Create(path)
	if err != nil {
		markBroken(t, "%v", err)
		return
	}
	var parent string
	t.Run("dry_run", func(t *testing.T) {
		parent = t.Name()
		runCloud(t, dryRunConfig, record)
	})
	if err := record.Close(0); err != nil {
		markBroken(t, "the record of the dry run: %v", err)
		return
	}
	requests := readRecord(t, path, testhook.DryRun)
	if len(requests) == 0 {
		markBroken(t, "the dry run recorded no request")
		return
	}
	res := surface.Check(surfaceFiles, scenariosOf(t, parent, requests))
	for _, p := range res.Problems {
		t.Error(p)
	}
	for _, b := range res.Broken {
		markBroken(t, "%s", b)
	}
	t.Logf("end-to-end states: %s: %s", res.Summary(), res.Verdict())
}

// TestRequiredScenarios checks that the lists of required tests agree with the plan of each run:
// .github/required-tests.txt, which the offline test job checks the dry run against, names the
// subtest of TestCloud of every scenario; tools/e2e/required.txt, which a live run against the
// PDP container is checked against, names those of the scenarios that such a run plans and no
// other, as a run does not select a scenario of another PDP, whose subtest then never runs; and
// tools/e2e/required-cloudpdp.txt does so for a live run of the leg cloudpdp, against the cloud
// PDP. The two lists of the live runs name the same tests outside the subtests of TestCloud, as
// each live run runs them all.
func TestRequiredScenarios(t *testing.T) {
	for _, tc := range []struct {
		file string
		cfg  config
	}{
		{"../../.github/required-tests.txt", dryRunConfig},
		{"required.txt", config{mode: testhook.Live, pdp: containerPDP}},
		{"required-cloudpdp.txt", config{mode: testhook.Live, pdp: cloudPDP}},
	} {
		want := []string{"TestCloud/logs_hold_no_key"}
		for _, sc := range tc.cfg.plan(cloudScenarios) {
			want = append(want, "TestCloud/"+sc.name)
		}
		slices.Sort(want)
		if got := requiredScenarios(t, tc.file); !slices.Equal(got, want) {
			t.Errorf("%s names the subtests %q of TestCloud, want %q", tc.file, got, want)
		}
	}
	container, cloud := requiredOutsideScenarios(t, "required.txt"),
		requiredOutsideScenarios(t, "required-cloudpdp.txt")
	if !slices.Equal(cloud, container) {
		t.Errorf("required-cloudpdp.txt names the tests %q outside the subtests of TestCloud, "+
			"want those of required.txt, %q", cloud, container)
	}
}

// requiredScenarios returns the subtests of TestCloud, and not their own subtests, that the list
// of required tests in file names for tools/e2e, sorted.
func requiredScenarios(t *testing.T, file string) []string {
	t.Helper()
	var names []string
	for _, test := range requiredTests(t, file) {
		fields := strings.Fields(test)
		if len(fields) == 2 && fields[0] == "github.com/permitio/permit-golang/v2/tools/e2e" &&
			strings.HasPrefix(fields[1], "TestCloud/") && strings.Count(fields[1], "/") == 1 {
			names = append(names, fields[1])
		}
	}
	slices.Sort(names)
	return names
}

// requiredOutsideScenarios returns the tests that the list of required tests in file names
// outside the subtests of TestCloud, each as its package and its name, sorted.
func requiredOutsideScenarios(t *testing.T, file string) []string {
	t.Helper()
	var tests []string
	for _, test := range requiredTests(t, file) {
		fields := strings.Fields(test)
		if len(fields) != 2 || !strings.HasPrefix(fields[1], "TestCloud/") {
			tests = append(tests, strings.Join(fields, " "))
		}
	}
	slices.Sort(tests)
	return tests
}

// requiredTests returns the lines of the list of required tests in file that name a test: those
// that are not blank and do not start with #.
func requiredTests(t *testing.T, file string) []string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		markBroken(t, "%v", err)
		t.FailNow()
	}
	var tests []string
	for line := range strings.SplitSeq(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			tests = append(tests, line)
		}
	}
	return tests
}

// TestContainerOnlyScenarios checks that the leg of the cloud PDP runs no scenario that covers a
// method of a route that only the PDP container serves, as testdata/operation-map.json and the
// SDK's registry say, but the scenarios of the cloud PDP itself, which check that such a route
// fails there: each other scenario that covers such a method needs the PDP container.
func TestContainerOnlyScenarios(t *testing.T) {
	data, err := os.ReadFile(surfaceFiles.OperationMap)
	var operationMap struct {
		Methods map[string][]string `json:"methods"`
	}
	if err == nil {
		err = json.Unmarshal(data, &operationMap)
	}
	if err != nil {
		markBroken(t, "reading the operation map: %v", err)
		t.FailNow()
	}
	containerOnly := map[string]bool{}
	for _, op := range rest.Default().Ops() {
		containerOnly[op.ID] = op.ContainerOnly
	}
	var methods []string
	for method, ops := range operationMap.Methods {
		if slices.ContainsFunc(ops, func(id string) bool { return containerOnly[id] }) {
			methods = append(methods, method)
		}
	}
	if len(methods) == 0 {
		t.Fatalf("%s names no method of a route that only the PDP container serves",
			surfaceFiles.OperationMap)
	}
	for _, sc := range (config{mode: testhook.Live, pdp: cloudPDP}).plan(cloudScenarios) {
		for _, method := range sc.methods {
			if sc.pdp != cloudPDP && slices.Contains(methods, method) {
				t.Errorf("the leg %s runs %s, which covers %s, a method of a route that only the "+
					"PDP container serves; give the scenario pdp: containerPDP", cloudPDPLeg,
					sc.name, method)
			}
		}
	}
}

// scenariosOf returns cloudScenarios with the operations that each sent, of requests, the
// requests of the test parent, which ran each scenario as a subtest. A request outside every
// scenario and one to a route of no operation of the registry fail t.
func scenariosOf(t *testing.T, parent string, requests []recorded) []surface.Scenario {
	t.Helper()
	scenarios := make([]surface.Scenario, len(cloudScenarios))
	for i, sc := range cloudScenarios {
		scenarios[i] = surface.Scenario{Name: sc.name, Methods: sc.methods}
	}
	for _, r := range requests {
		i := slices.IndexFunc(cloudScenarios, func(sc scenario) bool {
			name := parent + "/" + sc.name
			return r.Test == name || strings.HasPrefix(r.Test, name+"/")
		})
		op, ok := registryOp(r.Method, r.Template)
		switch {
		case i < 0:
			t.Errorf("%s sent %s %s outside every cloud scenario", r.Test, r.Method, r.Template)
		case !ok:
			t.Errorf("%s sent %s %s, which is the route of no operation of the registry", r.Test,
				r.Method, r.Template)
		default:
			scenarios[i].Sent = append(scenarios[i].Sent, surface.Op{
				ID: op.ID, OrganizationLevel: op.KeyLevel == rest.OrganizationLevel,
			})
		}
	}
	return scenarios
}

// recorded is a request of a record.
type recorded struct {
	Type     string `json:"type"`
	Mode     string `json:"mode"`
	Method   string `json:"method"`
	Template string `json:"template"`
	Test     string `json:"test"`
}

// readRecord returns the requests of the record in path, which must all be of the mode mode.
func readRecord(t *testing.T, path string, mode testhook.Mode) []recorded {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		markBroken(t, "%v", err)
		t.FailNow()
	}
	defer f.Close()
	var requests []recorded
	lines := bufio.NewScanner(f)
	for lines.Scan() {
		var r recorded
		if err := json.Unmarshal(lines.Bytes(), &r); err != nil {
			markBroken(t, "a line of the record: %v", err)
			t.FailNow()
		}
		if r.Type != "request" {
			continue
		}
		if r.Mode != string(mode) {
			markBroken(t, "a run in the mode %s recorded a request in the mode %q", mode, r.Mode)
		}
		requests = append(requests, r)
	}
	if err := lines.Err(); err != nil {
		markBroken(t, "reading the record: %v", err)
	}
	return requests
}

// registryOp returns the operation of the SDK's registry whose method is method and whose
// route, or its route through the PDP container, is template.
func registryOp(method, template string) (rest.Op, bool) {
	for _, op := range rest.Default().Ops() {
		if op.Method == method && (op.Route == template || op.ViaPDP().Route == template) {
			return op, true
		}
	}
	return rest.Op{}, false
}
