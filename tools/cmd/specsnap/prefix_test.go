package main

import (
	"reflect"
	"strings"
	"testing"
)

// controlPlane returns the parsed control plane fixture, which has the three defects.
func controlPlane(t *testing.T) map[string]any {
	t.Helper()
	return mustParse(t, readFixture(t, "control-plane.json"))
}

// secretBranches returns the anyOf branches of a proxy config schema's secret.
func secretBranches(t *testing.T, doc map[string]any, schema string) []any {
	t.Helper()
	branches, ok := lookup(doc, "components", "schemas", schema, "properties", "secret",
		"anyOf").([]any)
	if !ok {
		t.Fatalf("%s has no secret anyOf", schema)
	}
	return branches
}

// callbackBranches returns the anyOf branches of OPALUpdateCallback's callbacks items.
func callbackBranches(t *testing.T, doc map[string]any) []any {
	t.Helper()
	branches, ok := lookup(doc, "components", "schemas", "OPALUpdateCallback", "properties",
		"callbacks", "items", "anyOf").([]any)
	if !ok {
		t.Fatal("OPALUpdateCallback has no callbacks items anyOf")
	}
	return branches
}

// asList returns v as an array or fails the test.
func asList(t *testing.T, v any) []any {
	t.Helper()
	list, ok := v.([]any)
	if !ok {
		t.Fatalf("%v is not an array", v)
	}
	return list
}

func TestPrefixFixesTheDefects(t *testing.T) {
	doc := controlPlane(t)
	lines, err := prefix(doc, controlPlanePatches)
	if err != nil {
		t.Fatalf("prefix: %v", err)
	}
	wantLines := []string{
		`pre-fix "ProxyConfig secret types": fixed 9 places`,
		`pre-fix "OPALUpdateCallback tuple items": fixed 1 place`,
		`pre-fix "repeated Bulk Operations tag": fixed 1 place`,
	}
	if !reflect.DeepEqual(lines, wantLines) {
		t.Errorf("lines = %q, want %q", lines, wantLines)
	}
	for _, schema := range []string{"ProxyConfigCreate", "ProxyConfigRead", "ProxyConfigUpdate"} {
		branches := secretBranches(t, doc, schema)
		headers, _ := branches[0].(map[string]any)
		basic, _ := branches[1].(map[string]any)
		bearer, _ := branches[2].(map[string]any)
		if headers["type"] != "object" ||
			!reflect.DeepEqual(headers["additionalProperties"], map[string]any{"type": "string"}) {
			t.Errorf("%s HeadersAuth branch = %v", schema, headers)
		}
		if basic["type"] != "string" || basic["pattern"] != "^.+:.+$" {
			t.Errorf("%s BasicAuth branch = %v", schema, basic)
		}
		if bearer["type"] != "string" || bearer["minLength"] == nil {
			t.Errorf("%s BearerAuth branch = %v", schema, bearer)
		}
	}
	tuple, _ := callbackBranches(t, doc)[1].(map[string]any)
	if _, ok := tuple["items"]; ok {
		t.Errorf("the tuple still has items: %v", tuple)
	}
	if prefixItems, _ := tuple["prefixItems"].([]any); len(prefixItems) != 2 {
		t.Errorf("the tuple's prefixItems = %v, want its two items", tuple["prefixItems"])
	}
	var names []string
	for _, tag := range asList(t, doc["tags"]) {
		name, _ := lookup(tag, "name").(string)
		names = append(names, name)
	}
	if want := []string{"Bulk Operations", "Things"}; !reflect.DeepEqual(names, want) {
		t.Errorf("tags = %q, want %q", names, want)
	}
}

// TestPrefixFixNoLongerNeeded checks that each patch fails when the spec no longer has its
// defect, as when the API fixes its spec.
func TestPrefixFixNoLongerNeeded(t *testing.T) {
	for _, tc := range []struct {
		name  string
		patch string
		fix   func(t *testing.T, doc map[string]any)
	}{
		{
			name:  "secret types are JSON Schema types",
			patch: "ProxyConfig secret types",
			fix: func(t *testing.T, doc map[string]any) {
				t.Helper()
				fixSecretTypes(doc)
			},
		},
		{
			name:  "proxy config schemas are gone",
			patch: "ProxyConfig secret types",
			fix: func(t *testing.T, doc map[string]any) {
				t.Helper()
				schemas, _ := lookup(doc, "components", "schemas").(map[string]any)
				delete(schemas, "ProxyConfigCreate")
				delete(schemas, "ProxyConfigRead")
				delete(schemas, "ProxyConfigUpdate")
			},
		},
		{
			name:  "the tuple uses prefixItems",
			patch: "OPALUpdateCallback tuple items",
			fix: func(t *testing.T, doc map[string]any) {
				t.Helper()
				fixTupleItems(doc)
			},
		},
		{
			name:  "the tag is declared once",
			patch: "repeated Bulk Operations tag",
			fix: func(t *testing.T, doc map[string]any) {
				t.Helper()
				doc["tags"] = asList(t, doc["tags"])[:2]
			},
		},
		{
			name:  "the spec has no tags",
			patch: "repeated Bulk Operations tag",
			fix: func(t *testing.T, doc map[string]any) {
				t.Helper()
				delete(doc, "tags")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := controlPlane(t)
			tc.fix(t, doc)
			_, err := prefix(doc, controlPlanePatches)
			if exitCode(err) != exitFailed {
				t.Fatalf("prefix error = %v, want a failure", err)
			}
			want := `pre-fix "` + tc.patch + `": fix no longer needed`
			if !strings.Contains(err.Error(), want) ||
				!strings.Contains(err.Error(), "remove the patch") {
				t.Errorf("error = %q, want it to contain %q and say to remove the patch", err, want)
			}
		})
	}
}

// TestPrefixDefectChanged checks that a patch fails when its defect is in other places than the
// ones it fixes.
func TestPrefixDefectChanged(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(t *testing.T, doc map[string]any)
		want   string
	}{
		{
			name: "one secret branch is fixed",
			change: func(t *testing.T, doc map[string]any) {
				t.Helper()
				branches, _ := lookup(doc, "components", "schemas", "ProxyConfigRead",
					"properties", "secret", "anyOf").([]any)
				branch, _ := branches[1].(map[string]any)
				branch["type"] = "string"
			},
			want: `pre-fix "ProxyConfig secret types": the defect changed: it is in 8 places, ` +
				`and the patch fixes 9 places`,
		},
		{
			name: "a fourth secret branch",
			change: func(t *testing.T, doc map[string]any) {
				t.Helper()
				secret, _ := lookup(doc, "components", "schemas", "ProxyConfigUpdate",
					"properties", "secret").(map[string]any)
				secret["anyOf"] = append(asList(t, secret["anyOf"]),
					map[string]any{"type": "BearerAuth"})
			},
			want: "it is in 10 places, and the patch fixes 9 places",
		},
		{
			name: "a second tuple",
			change: func(t *testing.T, doc map[string]any) {
				t.Helper()
				items, _ := lookup(doc, "components", "schemas", "OPALUpdateCallback",
					"properties", "callbacks", "items").(map[string]any)
				items["anyOf"] = append(asList(t, items["anyOf"]),
					map[string]any{"items": []any{map[string]any{"type": "integer"}}})
			},
			want: `pre-fix "OPALUpdateCallback tuple items": the defect changed: it is in 2 ` +
				`places, and the patch fixes 1 place`,
		},
		{
			name: "the tag three times",
			change: func(t *testing.T, doc map[string]any) {
				t.Helper()
				doc["tags"] = append(asList(t, doc["tags"]),
					map[string]any{"name": "Bulk Operations"})
			},
			want: `pre-fix "repeated Bulk Operations tag": the defect changed: it is in 2 places`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := controlPlane(t)
			tc.change(t, doc)
			_, err := prefix(doc, controlPlanePatches)
			if exitCode(err) != exitFailed {
				t.Fatalf("prefix error = %v, want a failure", err)
			}
			if !strings.Contains(err.Error(), tc.want) ||
				!strings.Contains(err.Error(), "update the patch") {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

// TestFixesIgnoreOtherShapes checks that the patches leave alone what is not their defect.
func TestFixesIgnoreOtherShapes(t *testing.T) {
	doc := mustParse(t, []byte(`{
		"components": {"schemas": {
			"ProxyConfigCreate": {"properties": {"secret": {"anyOf": ["HeadersAuth",
				{"type": "string"}, {"type": ["BasicAuth"]}]}}},
			"ProxyConfigRead": {"properties": {"secret": {"type": "HeadersAuth"}}},
			"OPALUpdateCallback": {"properties": {"callbacks": {"items": {"anyOf": [
				{"items": {"type": "string"}}, "items"]}}}}
		}},
		"tags": ["Bulk Operations", {"name": "Bulk"}, {"name": "Bulk Operations"}]
	}`))
	before := string(mustCanonical(t, doc))
	for _, p := range controlPlanePatches {
		if n := p.fix(doc); n != 0 {
			t.Errorf("%s fixed %d places, want 0", p.name, n)
		}
	}
	if after := string(mustCanonical(t, doc)); after != before {
		t.Errorf("the patches changed the spec:\n%s\nwant:\n%s", after, before)
	}
}

// TestFixRepeatedTagAddsNoTags checks that the tag patch does not add a tag list to a spec that
// has none.
func TestFixRepeatedTagAddsNoTags(t *testing.T) {
	doc := map[string]any{"paths": map[string]any{}}
	if n := fixRepeatedTag(doc); n != 0 {
		t.Errorf("fixRepeatedTag = %d, want 0", n)
	}
	if _, ok := doc["tags"]; ok {
		t.Errorf("fixRepeatedTag added tags: %v", doc)
	}
}

func TestCount(t *testing.T) {
	for n, want := range map[int]string{0: "0 places", 1: "1 place", 2: "2 places"} {
		if got := count(n, "place"); got != want {
			t.Errorf("count(%d, place) = %q, want %q", n, got, want)
		}
	}
}
