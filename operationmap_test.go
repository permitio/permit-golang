package permit_test

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

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
}

// buildOperationMap returns the operation map of the wire cases.
func buildOperationMap(cases []wireCase) operationMap {
	m := operationMap{Methods: map[string][]string{}}
	for _, c := range cases {
		if !slices.Contains(m.Methods[c.Method], c.Op) {
			m.Methods[c.Method] = append(m.Methods[c.Method], c.Op)
		}
	}
	for _, ops := range m.Methods {
		slices.Sort(ops)
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

// compareOperationMap returns an error that shows the difference when file, the content of
// testdata/operation-map.json, differs from built, the map of the wire cases.
func compareOperationMap(file, built []byte) error {
	if diff := snapshotDiff(string(file), string(built)); diff != "" {
		return fmt.Errorf("%s is stale (-file +wire cases):\n%s\nRewrite it with go test -run "+
			"TestOperationMap -update . and review its diff.", operationMapFile, diff)
	}
	return nil
}

// TestOperationMap checks testdata/operation-map.json against the wire cases, and rewrites it
// with -update. Its count sentinel checks that the map has a method for every public method with
// a wire case. It takes part in the request record, which tools/cmd/apicoverage reads with the
// map.
func TestOperationMap(t *testing.T) {
	mockpermit.RecordTest(t)
	cases := wireCases(t)
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
	t.Logf("operation map: %d methods, %d operations", len(built.Methods), built.operations())
}

func TestBuildOperationMap(t *testing.T) {
	cases := syntheticCases()
	again := cases[1]
	again.Route = "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}/detail"
	other := cases[1]
	other.Op = "a_first_op"
	got := buildOperationMap(append(cases, again, other))
	want := operationMap{Methods: map[string][]string{
		"Client.Scope":                 {"op_Client_Scope"},
		"ImplicitGrantsService.Delete": {"op_ImplicitGrantsService_Delete"},
		"UsersService.Delete":          {"op_UsersService_Delete"},
		"UsersService.Get":             {"a_first_op", "op_UsersService_Get"},
	}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("buildOperationMap differs (-want +got):\n%s", diff)
	}
	if n := got.operations(); n != 5 {
		t.Errorf("operations() = %d, want 5", n)
	}
	if empty := buildOperationMap(nil); empty.Methods == nil || len(empty.Methods) != 0 {
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
	if got := string(buildOperationMap(nil).encode(t)); got != "{\n  \"methods\": {}\n}\n" {
		t.Errorf("the empty map encodes as %q", got)
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
