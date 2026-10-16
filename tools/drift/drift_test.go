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

	// rootDir is the directory of the root package, whose decision types the requests to the
	// PDP hold.
	rootDir = "../.."

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

// repository is what the repository tests read: the specs, the model map, the allowlist,
// package models and the root package.
type repository struct {
	comparer

	// modelMap is model-map.yaml.
	modelMap modelMap

	// allow are the entries of allow.yaml.
	allow []allowEntry

	// exported are the names of the exported types of package models, sorted.
	exported []string

	// roots are the names of the exported types of the root package, sorted.
	roots []string
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
	root, err := surface.Read(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	types := typesByName(modelTypes)
	pkgPaths := []string{modelsPath, rootPath}
	return &repository{
		comparer: comparer{
			specs: specs, rows: m.Models, types: types, constants: enumConstants(p.Items),
			pkgPaths: pkgPaths, sent: sentModels(rest.Default().Ops(), types, pkgPaths),
		},
		modelMap: m, allow: entries, exported: exportedTypes(p.Items),
		roots: exportedTypes(root.Items),
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
// models is a row of models or of blocks, every row is an exported type of package models or of
// the root package, and modelTypes holds each type that the map has a row for, and no other.
func TestRepositoryModelMap(t *testing.T) {
	r := readRepository(t)
	for _, f := range sentinel(r.exported, r.roots, r.modelMap, r.types) {
		t.Error(f)
	}
	roots := 0
	for name := range r.modelMap.Models {
		if slices.Contains(r.roots, name) {
			roots++
		}
	}
	t.Logf("package models exports %d types: %d map to schemas and %d are blocks; %d decision "+
		"types of the root package map to schemas", len(r.exported),
		len(r.modelMap.Models)-roots, len(r.modelMap.Blocks), roots)
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

// TestRepositoryRoundTrip round-trips the maximal values of each schema that model-map.yaml maps
// a type to through the type, without the properties that the rows leave out.
func TestRepositoryRoundTrip(t *testing.T) {
	r := readRepository(t)
	var found []finding
	var errs []error
	for _, name := range slices.Sorted(maps.Keys(r.rows)) {
		typ, ok := r.types[name]
		for _, tg := range r.rows[name].targets(name) {
			s := r.specs[tg.Spec]
			if _, exists := s.schema(tg.Schema); !exists || !ok {
				continue
			}
			more, err := roundTripFindings(tg.label, typ, s, schemaPrefix+escape(tg.Schema),
				leftOutOf(r.rows, tg.Spec))
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", tg.label, err))
			}
			found = append(found, more...)
		}
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
