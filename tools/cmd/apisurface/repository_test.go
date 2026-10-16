package main

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot is the repository root, three directories above this package.
const repoRoot = "../../.."

// TestRepositoryMigration runs migration mode on the repository's committed files.
func TestRepositoryMigration(t *testing.T) {
	code, out := runTool(t, system{}, "-mode", "migration", "-root", repoRoot)
	wantRun(t, code, out, exitPassed)
}

// TestRepositoryV1Surface checks the committed v1 surface against the counts of the API of
// github.com/permitio/permit-golang v1.3.0, so that a surface written from another version, or
// read only in part, fails.
func TestRepositoryV1Surface(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(repoRoot, surfaceFile))
	if err != nil {
		t.Fatal(err)
	}
	const header = "\n# module github.com/permitio/permit-golang v1.3.0 " +
		"h1:pzGr1DVrotTLDjsp8jXiEAhBP+/Z5UDgk9psYBuqI2U=\n"
	if !strings.Contains(string(content), header) {
		t.Errorf("%s does not name the module zip of v1.3.0: want the line %q", surfaceFile,
			strings.TrimSpace(header))
	}
	items, err := readSurface(strings.NewReader(string(content)))
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, it := range items {
		key := it.block
		switch it.block {
		case blockIdentifiers:
			pkg, _, _ := strings.Cut(it.id, ".")
			key += " " + pkg
		case blockOperations:
			key += " " + it.details[strings.LastIndex(it.details, " ")+1:]
		}
		counts[key]++
	}
	want := map[string]int{
		// The 298 exported identifiers of the hand-written packages, then pkg/enforcement's 54.
		"a api": 184, "a config": 50, "a errors": 53, "a permit": 11, "a enforcement": 54,
		// The 56 operations of the API spec that only the generated client sends, and the 30
		// generated operations whose route the spec does not have.
		"b " + inSpec: 56, "b " + notInSpec: 30,
		"c": 384,
	}
	if !maps.Equal(counts, want) {
		t.Errorf("%s counts %v, want %v", surfaceFile, counts, want)
	}
}

// TestRepositoryOperationsAreNotV1Routes checks block b against tools/specs/v1-routes.json, the
// routes of the API spec that v1.2.8's hand-written methods sent: an operation that only the
// generated client sends cannot be one of them.
func TestRepositoryOperationsAreNotV1Routes(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(repoRoot, "tools/specs/v1-routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Routes []struct {
			Method string `json:"method"`
			Path   string `json:"path"`
		} `json:"routes"`
	}
	if err = json.Unmarshal(content, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Routes) < 90 {
		t.Fatalf("v1-routes.json lists %d routes; it was not read", len(doc.Routes))
	}
	sent := map[string]bool{}
	for _, r := range doc.Routes {
		sent[r.Method+" "+normalizeRoute(r.Path)] = true
	}
	surfaceContent, err := os.ReadFile(filepath.Join(repoRoot, surfaceFile))
	if err != nil {
		t.Fatal(err)
	}
	items, err := readSurface(strings.NewReader(string(surfaceContent)))
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		fields := strings.Fields(it.details)
		if it.block != blockOperations {
			continue
		}
		if len(fields) != 3 || (fields[2] != inSpec && fields[2] != notInSpec) {
			t.Errorf("%s: the details %q are not <method> <route> %s|%s", it.id, it.details,
				inSpec, notInSpec)
			continue
		}
		if sent[fields[0]+" "+normalizeRoute(fields[1])] {
			t.Errorf("%s sends %s %s, which a hand-written method of v1.2.8 sent too", it.id,
				fields[0], fields[1])
		}
	}
}
