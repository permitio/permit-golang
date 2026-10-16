package permit_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// operationMapFile maps each public method to the operations that its wire cases send. It is
// built from the code, so that tools/cmd/apicoverage, which reads it with the request record,
// can name the method that covers an operation: a request record holds no method names.
const operationMapFile = "testdata/operation-map.json"

// operationMap is the content of testdata/operation-map.json.
type operationMap struct {
	// Methods maps each public method that has a wire case, such as UsersService.Get, to the
	// operationIds of its cases, sorted.
	Methods map[string][]string `json:"methods"`

	// FactsViaPDP maps each public method whose wire cases run in factsViaPDPMode, such as
	// UsersService.Create, to the routes under /facts that those cases send to the PDP
	// container, such as "POST /facts/users", sorted. The container's spec gives these routes
	// operationIds of their own, which no wire case names, so tools/cmd/apicoverage reads the
	// routes to find the rows of coverage.yaml that each method reaches through the container.
	FactsViaPDP map[string][]string `json:"facts_via_pdp,omitempty"`
}

// buildOperationMap returns the operation map of the wire cases.
func buildOperationMap(cases []wireCase) operationMap {
	m := operationMap{Methods: map[string][]string{}, FactsViaPDP: map[string][]string{}}
	for _, c := range cases {
		if !slices.Contains(m.Methods[c.Method], c.Op) {
			m.Methods[c.Method] = append(m.Methods[c.Method], c.Op)
		}
		route, ok := factsViaPDP(c.Route)
		if ok && slices.Contains(c.Modes, factsViaPDPMode) &&
			!slices.Contains(m.FactsViaPDP[c.Method], route) {
			m.FactsViaPDP[c.Method] = append(m.FactsViaPDP[c.Method], route)
		}
	}
	for _, ops := range m.Methods {
		slices.Sort(ops)
	}
	for _, routes := range m.FactsViaPDP {
		slices.Sort(routes)
	}
	return m
}

// encode returns the content of testdata/operation-map.json for m: JSON with sorted keys and an
// indent of two spaces, and a final newline.
func (m operationMap) encode(t *testing.T) []byte {
	t.Helper()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("encoding the operation map: %v", err)
	}
	return append(data, '\n')
}

// operations returns the number of pairs of a method and an operation in m.
func (m operationMap) operations() int {
	n := 0
	for _, ops := range m.Methods {
		n += len(ops)
	}
	return n
}

// factsRoutes returns the number of pairs of a method and a route under /facts in m.
func (m operationMap) factsRoutes() int {
	n := 0
	for _, routes := range m.FactsViaPDP {
		n += len(routes)
	}
	return n
}

// compareOperationMap returns an error that shows the difference when file, the content of
// testdata/operation-map.json, differs from built, the map of the wire cases.
func compareOperationMap(file, built []byte) error {
	if diff := snapshotDiff(string(file), string(built)); diff != "" {
		return fmt.Errorf("%s is stale (-file +wire cases):\n%s\nRewrite it with go test -run "+
			"TestOperationMap -update . and review its diff.", operationMapFile, diff)
	}
	return nil
}

// unregisteredOps returns a problem for each wire case whose Op the registry does not hold.
func unregisteredOps(cases []wireCase, registry *rest.Registry) []string {
	var problems []string
	for _, c := range cases {
		if _, ok := registry.Op(c.Op); !ok {
			problems = append(problems, fmt.Sprintf("the wire case of %s names the operation %s, "+
				"which the operation registry does not hold; add its rest.Op to internal/rest, "+
				"or fix the case's Op", c.Method, c.Op))
		}
	}
	return problems
}

// TestOperationMap checks testdata/operation-map.json against the wire cases, and rewrites it
// with -update. Its count sentinel checks that the map has a method for every public method with
// a wire case, and it checks that the operation registry holds the Op of every wire case. It
// takes part in the request record, which tools/cmd/apicoverage reads with the map.
func TestOperationMap(t *testing.T) {
	mockpermit.RecordTest(t)
	cases := wireCases(t)
	for _, problem := range unregisteredOps(cases, rest.Default()) {
		t.Error(problem)
	}
	built := buildOperationMap(cases)
	cov := checkCoverage(readSurface(t)[0].Items, cases, readExemptions(t))
	if len(built.Methods) != cov.covered {
		t.Errorf("count sentinel: the operation map has %d methods, but %d public methods have "+
			"a wire case", len(built.Methods), cov.covered)
	}
	data := built.encode(t)
	if *update {
		if err := os.WriteFile(operationMapFile, data, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", operationMapFile)
	}
	file, err := os.ReadFile(operationMapFile)
	if err != nil {
		t.Fatalf("%v; write the map with go test -run TestOperationMap -update .", err)
	}
	if err := compareOperationMap(file, data); err != nil {
		t.Error(err)
	}
	t.Logf("operation map: %d methods, %d operations, %d routes of %d methods through the PDP "+
		"container", len(built.Methods), built.operations(), built.factsRoutes(),
		len(built.FactsViaPDP))
}

func TestUnregisteredOps(t *testing.T) {
	cases := syntheticCases()
	var ops []rest.Op
	for _, c := range cases[1:3] {
		ops = append(ops, rest.Op{
			ID: c.Op, Method: http.MethodGet, Route: rest.TestPrefix + c.Op,
			KeyLevel: rest.EnvironmentLevel,
		})
	}
	registry, err := rest.Default().WithTestOps(ops...)
	if err != nil {
		t.Fatal(err)
	}
	if problems := unregisteredOps(cases[1:3], registry); problems != nil {
		t.Errorf("unregisteredOps of registered cases = %q, want none", problems)
	}
	got := unregisteredOps(cases, registry)
	want := []string{
		"the wire case of Client.Scope names the operation op_Client_Scope, which the operation " +
			"registry does not hold; add its rest.Op to internal/rest, or fix the case's Op",
		"the wire case of ImplicitGrantsService.Delete names the operation " +
			"op_ImplicitGrantsService_Delete, which the operation registry does not hold; add " +
			"its rest.Op to internal/rest, or fix the case's Op",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unregisteredOps differs (-want +got):\n%s", diff)
	}
	if problems := unregisteredOps(nil, rest.Default()); problems != nil {
		t.Errorf("unregisteredOps of no cases = %q, want none", problems)
	}
}

func TestBuildOperationMap(t *testing.T) {
	cases := syntheticCases()
	again := cases[1]
	again.Route = "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}/detail"
	other := cases[1]
	other.Op = "a_first_op"
	both := []wireMode{apiMode, factsViaPDPMode}
	detail, get, deleteUser := again, cases[1], cases[2]
	detail.Modes, get.Modes, deleteUser.Modes = both, both, both
	got := buildOperationMap(append(cases, detail, other, get, deleteUser, get))
	want := operationMap{
		Methods: map[string][]string{
			"Client.Scope":                 {"op_Client_Scope"},
			"ImplicitGrantsService.Delete": {"op_ImplicitGrantsService_Delete"},
			"UsersService.Delete":          {"op_UsersService_Delete"},
			"UsersService.Get":             {"a_first_op", "op_UsersService_Get"},
		},
		FactsViaPDP: map[string][]string{
			"UsersService.Delete": {"DELETE /facts/users/{user_id}"},
			"UsersService.Get": {
				"GET /facts/users/{user_id}", "GET /facts/users/{user_id}/detail",
			},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("buildOperationMap differs (-want +got):\n%s", diff)
	}
	if n := got.operations(); n != 5 {
		t.Errorf("operations() = %d, want 5", n)
	}
	if n := got.factsRoutes(); n != 3 {
		t.Errorf("factsRoutes() = %d, want 3", n)
	}
	empty := buildOperationMap(nil)
	if empty.Methods == nil || len(empty.Methods) != 0 || len(empty.FactsViaPDP) != 0 {
		t.Errorf("buildOperationMap(nil) = %#v, want an empty map that encodes as {}", empty)
	}
}

func TestOperationMapEncode(t *testing.T) {
	got := string(operationMap{Methods: map[string][]string{
		"UsersService.Get": {"get_user"},
		"Client.Scope":     {"get_api_key_scope"},
	}}.encode(t))
	want := "{\n" +
		"  \"methods\": {\n" +
		"    \"Client.Scope\": [\n" +
		"      \"get_api_key_scope\"\n" +
		"    ],\n" +
		"    \"UsersService.Get\": [\n" +
		"      \"get_user\"\n" +
		"    ]\n" +
		"  }\n" +
		"}\n"
	if got != want {
		t.Errorf("encode() =\n%s\nwant\n%s", got, want)
	}
	if empty := string(buildOperationMap(nil).encode(t)); empty != "{\n  \"methods\": {}\n}\n" {
		t.Errorf("the empty map encodes as %q", empty)
	}
	got = string(operationMap{
		Methods:     map[string][]string{"UsersService.Create": {"create_user"}},
		FactsViaPDP: map[string][]string{"UsersService.Create": {"POST /facts/users"}},
	}.encode(t))
	want = "{\n" +
		"  \"methods\": {\n" +
		"    \"UsersService.Create\": [\n" +
		"      \"create_user\"\n" +
		"    ]\n" +
		"  },\n" +
		"  \"facts_via_pdp\": {\n" +
		"    \"UsersService.Create\": [\n" +
		"      \"POST /facts/users\"\n" +
		"    ]\n" +
		"  }\n" +
		"}\n"
	if got != want {
		t.Errorf("encode() of a map with facts_via_pdp =\n%s\nwant\n%s", got, want)
	}
}

func TestCompareOperationMap(t *testing.T) {
	built := buildOperationMap(syntheticCases()).encode(t)
	if err := compareOperationMap(built, built); err != nil {
		t.Errorf("compareOperationMap of equal maps returned %v", err)
	}
	stale := buildOperationMap(syntheticCases()[1:]).encode(t)
	err := compareOperationMap(stale, built)
	if err == nil {
		t.Fatal("compareOperationMap found no difference in a stale map")
	}
	for _, want := range []string{
		"testdata/operation-map.json is stale (-file +wire cases)", "Client.Scope",
		"go test -run TestOperationMap -update .",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("compareOperationMap returned %q, want it to contain %q", err, want)
		}
	}
}
