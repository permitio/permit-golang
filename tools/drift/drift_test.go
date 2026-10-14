// Package drift holds the drift tests of the SDK against the spec snapshots in tools/specs.
// model-map.yaml maps each type of package models to its schema, with a required flag and a
// null policy for each field of a struct; the tests compare each type with its schema in both
// directions, round-trip the maximal values of the schema through the type with a strict
// decoder, compare the RequestModel and ResponseModel of each operation of the registry with its
// request and reply schemas, and validate each reply fixture in the root's testdata/replies
// against its reply schema. allow.yaml accepts the findings that come from known defects of the
// specs, each with a reason and a trigger, and an entry that accepts nothing fails. The tests of
// waits_test.go check the routes on which the PDP container waits for sync, which the spec of
// the container lists, against the root's doc.go, the registry and coverage.yaml.
//
// The checks share one walker of the part of JSON Schema that the snapshots use. A schema with
// a keyword that the walker does not know stops the check that meets it, which then reports DID
// NOT RUN with the keyword's name, so that nothing is skipped unseen.
//
// The tests whose names start with TestRepository read the files of the repository: package
// models, the root's testdata/replies, and the snapshots in tools/specs, or those in the
// directory that -specs names, such as the live snapshots of specsnap -check-live -out. The
// other tests check the walker and the checks on the fixtures in testdata. CONTRIBUTING.md,
// section Model drift, says how to write a row of the map, an entry of the allowlist and a reply
// fixture.
package drift

import (
	"flag"
	"fmt"
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/surface"
)

const (
	// committedSpecs is the directory of the committed spec snapshots.
	committedSpecs = "../specs"

	// modelsDir is the directory of package models.
	modelsDir = "../../models"

	// repliesDir is the directory of the reply fixtures of the SDK's tests.
	repliesDir = "../../testdata/replies"

	// modelMapFile is the drift map of package models.
	modelMapFile = "model-map.yaml"

	// allowFile is the allowlist of the drift findings.
	allowFile = "allow.yaml"
)

// specsDir is the directory of the spec snapshots that the repository tests check against.
var specsDir = flag.String("specs", committedSpecs, "the `directory` of the spec snapshots to "+
	"check against, such as the live snapshots of specsnap -check-live -out; a snapshot that "+
	"it does not hold is read from tools/specs")

// repository is what the repository tests read: the specs, the model map, the allowlist, and
// package models.
type repository struct {
	comparer

	// modelMap is model-map.yaml.
	modelMap modelMap

	// allow are the entries of allow.yaml.
	allow []allowEntry

	// exported are the names of the exported types of package models, sorted.
	exported []string
}

// readRepository reads what the repository tests read, and fails t when a file cannot be read
// or parsed: with DID NOT RUN when it is a spec snapshot, which -specs may name.
func readRepository(t *testing.T) *repository {
	t.Helper()
	specs, lines, err := readSpecs(*specsDir, committedSpecs)
	if err != nil {
		t.Fatalf("DID NOT RUN: %v", err)
	}
	for _, line := range lines {
		t.Log("the spec " + line)
	}
	data, err := os.ReadFile(modelMapFile)
	if err != nil {
		t.Fatal(err)
	}
	m, err := parseModelMap(data)
	if err != nil {
		t.Fatalf("%s: %v", modelMapFile, err)
	}
	data, err = os.ReadFile(allowFile)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := parseAllowList(data)
	if err != nil {
		t.Fatalf("%s: %v", allowFile, err)
	}
	p, err := surface.Read(modelsDir)
	if err != nil {
		t.Fatal(err)
	}
	types := typesByName(modelTypes)
	return &repository{
		comparer: comparer{
			specs: specs, rows: m.Models, types: types, constants: enumConstants(p.Items),
			pkgPath: modelsPath, sent: sentModels(rest.Default().Ops(), types, modelsPath),
		},
		modelMap: m, allow: entries, exported: exportedTypes(p.Items),
	}
}

// report fails tb with each finding that the allowlist does not accept, each entry whose key
// starts with area that accepts none, and each error, which says that a check did not run.
func (r *repository) report(tb testing.TB, area string, findings []finding, errs []error) {
	tb.Helper()
	open, stale := accept(findings, r.allow, area)
	for _, f := range open {
		tb.Errorf("%s", f)
	}
	for _, entry := range stale {
		tb.Errorf("%s: the entry %q accepts no finding: remove it", allowFile, entry.Key)
	}
	for _, err := range errs {
		tb.Errorf("DID NOT RUN: %v", err)
	}
}

// TestRepositoryModelMap checks the counts of model-map.yaml: every exported type of package
// models is a row of models or of blocks, every row is an exported type, and modelTypes holds
// each type that the map has a row for, and no other.
func TestRepositoryModelMap(t *testing.T) {
	r := readRepository(t)
	for _, f := range sentinel(r.exported, r.modelMap, r.types) {
		t.Error(f)
	}
	t.Logf("package models exports %d types: %d map to schemas and %d are blocks",
		len(r.exported), len(r.modelMap.Models), len(r.modelMap.Blocks))
}

// TestRepositoryModels compares each type that model-map.yaml maps with its schema.
func TestRepositoryModels(t *testing.T) {
	r := readRepository(t)
	var found []finding
	var errs []error
	for _, name := range slices.Sorted(maps.Keys(r.rows)) {
		more, err := r.model(name)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
		found = append(found, more...)
	}
	r.report(t, "model ", found, errs)
}

// TestRepositoryRoundTrip round-trips the maximal values of the schema of each type that
// model-map.yaml maps through the type.
func TestRepositoryRoundTrip(t *testing.T) {
	r := readRepository(t)
	var found []finding
	var errs []error
	for _, name := range slices.Sorted(maps.Keys(r.rows)) {
		row := r.rows[name]
		s := r.specs[row.spec()]
		typ, ok := r.types[name]
		if _, exists := s.schema(row.Schema); !exists || !ok {
			continue
		}
		more, err := roundTripFindings(name, typ, s, schemaPrefix+escape(row.Schema))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
		found = append(found, more...)
	}
	r.report(t, "round trip ", found, errs)
}

// TestRepositoryOperations compares the models of each operation of the registry with the
// operation's request and reply schemas.
func TestRepositoryOperations(t *testing.T) {
	r := readRepository(t)
	ops := rest.Default().Ops()
	var found []finding
	var errs []error
	for _, op := range ops {
		more, err := r.operation(op)
		if err != nil {
			errs = append(errs, fmt.Errorf("the operation %s: %w", op.ID, err))
		}
		found = append(found, more...)
	}
	r.report(t, "operation ", found, errs)
	t.Logf("checked the %d operations of the registry", len(ops))
}

// TestRepositoryReplies validates each reply fixture in the root's testdata/replies against the
// schema of its reply.
func TestRepositoryReplies(t *testing.T) {
	r := readRepository(t)
	found, errs, n, err := replyFindings(repliesDir, r.specs)
	if err != nil {
		t.Fatal(err)
	}
	r.report(t, "reply ", found, errs)
	t.Logf("validated %d reply fixtures in %s", n, repliesDir)
}
