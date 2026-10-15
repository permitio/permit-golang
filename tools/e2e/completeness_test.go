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
