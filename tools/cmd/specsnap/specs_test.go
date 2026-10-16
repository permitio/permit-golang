package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// specsDir is the committed snapshot directory, tools/specs.
var specsDir = filepath.Join("..", "..", "specs")

// sourcePattern matches a file:line citation.
var sourcePattern = regexp.MustCompile(`^[a-z][A-Za-z0-9_/.-]*\.(go|rs|py):[1-9][0-9]*$`)

// readSpecsJSON decodes a JSON file of tools/specs, rejecting unknown fields.
func readSpecsJSON(t *testing.T, name string, v any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(specsDir, name))
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

// specOperations returns the operation IDs of a committed snapshot by "METHOD path".
func specOperations(t *testing.T, name string) map[string]string {
	t.Helper()
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	data, err := os.ReadFile(filepath.Join(specsDir, name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	ops := map[string]string{}
	for path, item := range spec.Paths {
		for method, raw := range item {
			if !slices.Contains(httpMethods, method) {
				continue
			}
			var op struct {
				OperationID string `json:"operationId"` //nolint:tagliatelle // OpenAPI's name.
			}
			if err := json.Unmarshal(raw, &op); err != nil {
				t.Fatal(err)
			}
			ops[strings.ToUpper(method)+" "+path] = op.OperationID
		}
	}
	return ops
}

// TestCommittedSnapshots checks that the committed snapshots match lock.json, and that they are
// the specs the SDK is built against.
func TestCommittedSnapshots(t *testing.T) {
	sys := system{sources: sources}
	code, out := runSpecsnap(t, sys, "-dir", specsDir, "-verify")
	if code != exitPassed {
		t.Fatalf("specsnap -verify on tools/specs exited %d:\n%s", code, out)
	}
	lock := readLockFile(t, specsDir)
	want := map[string]lockEntry{
		"control-plane": {
			URL: "https://api.permit.io/v2/openapi.json", Fetched: "2026-10-14",
			SourceSHA256: "15ab6a09bb7cd99bddca0bf3d1c380ac39dbcc6de35edeec1e2cc8454f8e8215",
			Operations:   263,
		},
		"pdp-cloud": {
			URL: "https://cloudpdp.api.permit.io/openapi.json", Fetched: "2026-10-11",
			SourceSHA256: "8fb3ad79b93679b24ddd90596e17b7f6baa6b662e2c09a96f1a3ffc938292a29",
			Operations:   10,
		},
		"pdp-container": {
			Image:        "permitio/pdp-v2:0.9.16",
			Digest:       "sha256:e3cf30794ec2d256636b4714641df46e51ee58a3f1f0d24c606e214e0bf8669a",
			Fetched:      "2026-09-30",
			SourceSHA256: "863635167e13483f21ccce5b3f38d93a2f8625c4e480d5fdea306c56a461b46f",
			Operations:   34,
		},
	}
	for name, entry := range want {
		got := lock[name]
		if got.URL != entry.URL || got.Image != entry.Image || got.Digest != entry.Digest ||
			got.Fetched != entry.Fetched || got.SourceSHA256 != entry.SourceSHA256 ||
			got.Operations != entry.Operations {
			t.Errorf("lock.json %s = %+v, want %+v", name, got, entry)
		}
	}
	minimums := map[string]int{"control-plane": 250, "pdp-cloud": 8, "pdp-container": 30}
	for _, src := range sources {
		if src.url != lock[src.name].URL || src.minOperations != minimums[src.name] {
			t.Errorf("source %s fetches %q with at least %d operations; lock.json has %q, and "+
				"the package doc says %d", src.name, src.url, src.minOperations,
				lock[src.name].URL, minimums[src.name])
		}
	}
	if hubTagsURL != "https://hub.docker.com/v2/repositories/permitio/pdp-v2/tags?"+
		"page_size=100&ordering=last_updated" {
		t.Errorf("Docker Hub's tag list is read from %s", hubTagsURL)
	}
}

// v1Routes is v1-routes.json: the routes v1.2.8 sent that are in the control plane's spec.
type v1Routes struct {
	Tag    string `json:"tag"`
	Commit string `json:"commit"`
	Routes []struct {
		Method      string `json:"method"`
		Path        string `json:"path"`
		OperationID string `json:"operation_id"`
		Source      string `json:"source"`
		Callers     []struct {
			SDKMethod string `json:"sdk_method"`
			Source    string `json:"source"`
		} `json:"callers"`
	} `json:"routes"`
}

// TestV1Routes checks v1-routes.json: 91 routes, each once, each an operation of the control
// plane's spec with its ID, each with the line of v1.2.8 that holds the route and the methods
// that send it.
func TestV1Routes(t *testing.T) {
	var doc v1Routes
	readSpecsJSON(t, "v1-routes.json", &doc)
	if doc.Tag != "v1.2.8" || doc.Commit != "78500657b487c63cba5fde8247d53d3bbc83fee7" {
		t.Errorf("v1-routes.json is of %s at %s, want v1.2.8", doc.Tag, doc.Commit)
	}
	if len(doc.Routes) != 91 {
		t.Errorf("v1-routes.json has %d routes, want 91", len(doc.Routes))
	}
	ops := specOperations(t, "control-plane.json")
	seen := map[string]bool{}
	for _, route := range doc.Routes {
		key := route.Method + " " + route.Path
		if seen[key] {
			t.Errorf("%s is listed twice", key)
		}
		seen[key] = true
		if id, ok := ops[key]; !ok || id != route.OperationID {
			t.Errorf("%s: operation %q, and the spec has %q (in it: %v)", key, route.OperationID,
				id, ok)
		}
		if !sourcePattern.MatchString(route.Source) ||
			!strings.HasPrefix(route.Source, "pkg/openapi/") {
			t.Errorf("%s: source %q is not a pkg/openapi file:line", key, route.Source)
		}
		if len(route.Callers) == 0 {
			t.Errorf("%s: no caller", key)
		}
		for _, caller := range route.Callers {
			if caller.SDKMethod == "" || !sourcePattern.MatchString(caller.Source) {
				t.Errorf("%s: caller %+v", key, caller)
			}
		}
	}
}

// routeFixes is v1-route-fixes.yaml.
type routeFixes struct {
	Fixes []struct {
		ID      string `yaml:"id"`
		Reason  string `yaml:"reason"`
		Changes []struct {
			V1Method string `yaml:"v1_method"` //nolint:tagliatelle // the file uses snake case.
			Source   string `yaml:"source"`
			V1       string `yaml:"v1"`
			V2       string `yaml:"v2"`
		} `yaml:"changes"`
	} `yaml:"fixes"`
}

// TestV1RouteFixes checks v1-route-fixes.yaml: the five fixes, each change with its v1.2.8
// line, and every route of the spec that a change names is in the spec, and in v1-routes.json
// when v1 sent it.
func TestV1RouteFixes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(specsDir, "v1-route-fixes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc routeFixes
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, fix := range doc.Fixes {
		ids = append(ids, fix.ID)
	}
	wantIDs := []string{
		"action-group-delete", "implicit-grant-delete", "parent-roles", "detailed-lists",
		"facts-mount",
	}
	if !slices.Equal(ids, wantIDs) {
		t.Errorf("fixes %q, want %q", ids, wantIDs)
	}
	var v1 v1Routes
	readSpecsJSON(t, "v1-routes.json", &v1)
	sent := map[string]bool{}
	for _, route := range v1.Routes {
		sent[route.Method+" "+route.Path] = true
	}
	ops := specOperations(t, "control-plane.json")
	routePattern := regexp.MustCompile(`^(GET|PUT|POST|PATCH|DELETE|ANY) /[^ ]*$`)
	changes := 0
	for _, fix := range doc.Fixes {
		if strings.TrimSpace(fix.Reason) == "" || len(fix.Changes) == 0 {
			t.Errorf("%s has no reason or no change", fix.ID)
		}
		for _, change := range fix.Changes {
			changes++
			if change.V1Method == "" || !sourcePattern.MatchString(change.Source) ||
				!strings.HasPrefix(change.Source, "pkg/") {
				t.Errorf("%s: change %+v has no v1 method or source", fix.ID, change)
			}
			if !routePattern.MatchString(change.V1) {
				t.Errorf("%s: v1 route %q", fix.ID, change.V1)
			}
			if _, inSpec := ops[change.V1]; inSpec && !sent[change.V1] {
				t.Errorf("%s: v1 route %q is in the spec but not in v1-routes.json", fix.ID,
					change.V1)
			}
			if change.V2 == "none" || strings.HasPrefix(change.V2, "ANY ") {
				continue
			}
			if _, inSpec := ops[change.V2]; !inSpec {
				t.Errorf("%s: v2 route %q is not in the control plane's spec", fix.ID, change.V2)
			}
		}
	}
	if changes != 10 {
		t.Errorf("%d changes, want 10", changes)
	}
}

// containerExtras is pdp-container.extra.json: what the container PDP serves without listing
// it in its /openapi.json.
type containerExtras struct {
	Image      string `json:"image"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Routes     []struct {
		Group   string   `json:"group"`
		Methods []string `json:"methods"`
		Path    string   `json:"path"`
		Source  string   `json:"source"`
	} `json:"routes"`
	Mounts []struct {
		Prefix string `json:"prefix"`
		SameAs string `json:"same_as"`
		Source string `json:"source"`
	} `json:"mounts"`
}

// TestContainerExtras checks pdp-container.extra.json: it describes the image of the snapshot,
// lists AuthZEN's 6 routes, the 3 health routes, Trino's 3, the facts catch-all and the
// compatibility mount, each with its source, and none of them is in pdp-container.json.
func TestContainerExtras(t *testing.T) {
	var doc containerExtras
	readSpecsJSON(t, "pdp-container.extra.json", &doc)
	lock := readLockFile(t, specsDir)
	if doc.Image != lock["pdp-container"].Image || doc.Tag != "0.9.16" ||
		doc.Repository != "https://github.com/permitio/PDP" {
		t.Errorf("pdp-container.extra.json is of %s at %s %s, and the snapshot of %s", doc.Image,
			doc.Repository, doc.Tag, lock["pdp-container"].Image)
	}
	ops := specOperations(t, "pdp-container.json")
	groups := map[string]int{}
	for _, route := range doc.Routes {
		groups[route.Group]++
		if !sourcePattern.MatchString(route.Source) || len(route.Methods) == 0 {
			t.Errorf("route %+v has no source or method", route)
		}
		for _, method := range route.Methods {
			if _, listed := ops[method+" "+route.Path]; listed {
				t.Errorf("%s %s is in pdp-container.json", method, route.Path)
			}
		}
	}
	want := map[string]int{"authzen": 6, "health": 3, "trino": 3, "facts": 1}
	if len(groups) != len(want) {
		t.Errorf("groups %v, want %v", groups, want)
	}
	for group, n := range want {
		if groups[group] != n {
			t.Errorf("%d %s routes, want %d", groups[group], group, n)
		}
	}
	if len(doc.Mounts) != 1 || doc.Mounts[0].Prefix != "/v2/facts/{proj_id}/{env_id}" ||
		doc.Mounts[0].SameAs != "/facts" || !sourcePattern.MatchString(doc.Mounts[0].Source) {
		t.Errorf("mounts %+v, want the facts compatibility mount", doc.Mounts)
	}
}
