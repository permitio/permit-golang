package main

import (
	"fmt"
)

// patch fixes one construct of a published spec that is not valid OpenAPI 3.1.
type patch struct {
	// name names the patch in messages.
	name string
	// defect says what the patch fixes, for the message when it is gone.
	defect string
	// places is the number of places the patch fixes in the published spec.
	places int
	// fix fixes the defect in doc and returns the number of places it fixed.
	fix func(doc map[string]any) int
}

// controlPlanePatches is the control plane spec's pre-fix.
var controlPlanePatches = []patch{
	{
		name: "ProxyConfig secret types",
		defect: "secret anyOf branches of type HeadersAuth, BasicAuth or BearerAuth in " +
			"ProxyConfigCreate, ProxyConfigRead and ProxyConfigUpdate",
		places: 9,
		fix:    fixSecretTypes,
	},
	{
		name:   "OPALUpdateCallback tuple items",
		defect: "an items array, a draft-04 tuple, in OPALUpdateCallback's callbacks",
		places: 1,
		fix:    fixTupleItems,
	},
	{
		name:   "repeated Bulk Operations tag",
		defect: "a second declaration of the tag Bulk Operations",
		places: 1,
		fix:    fixRepeatedTag,
	},
}

// prefix applies the patches to doc and returns a line for each. Its error is a failure when a
// patch found its defect in no place ("fix no longer needed") or in another number of places
// than it fixes ("the defect changed").
func prefix(doc map[string]any, patches []patch) ([]string, error) {
	var lines []string
	for _, p := range patches {
		n := p.fix(doc)
		switch {
		case n == 0:
			return nil, failure{fmt.Errorf("pre-fix %q: fix no longer needed: the spec no longer "+
				"has %s; remove the patch from tools/cmd/specsnap", p.name, p.defect)}
		case n != p.places:
			return nil, failure{fmt.Errorf("pre-fix %q: the defect changed: it is in %s, and "+
				"the patch fixes %s; update the patch in tools/cmd/specsnap", p.name,
				count(n, "place"), count(p.places, "place"))}
		}
		lines = append(lines, fmt.Sprintf("pre-fix %q: fixed %s", p.name, count(n, "place")))
	}
	return lines, nil
}

// count returns n and the noun, plural unless n is 1, such as "1 place" or "9 places".
func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// fixSecretTypes gives the proxy config secret's anyOf branches JSON Schema types: HeadersAuth,
// a map of header names to values, becomes an object of strings; BasicAuth, "user:password",
// and BearerAuth, a token, become strings.
func fixSecretTypes(doc map[string]any) int {
	n := 0
	for _, schema := range []string{"ProxyConfigCreate", "ProxyConfigRead", "ProxyConfigUpdate"} {
		branches, _ := lookup(doc, "components", "schemas", schema, "properties", "secret",
			"anyOf").([]any)
		for _, item := range branches {
			branch, _ := item.(map[string]any)
			switch branch["type"] {
			case "HeadersAuth":
				branch["type"] = "object"
				branch["additionalProperties"] = map[string]any{"type": "string"}
				n++
			case "BasicAuth", "BearerAuth":
				branch["type"] = "string"
				n++
			}
		}
	}
	return n
}

// fixTupleItems turns the draft-04 tuple in OPALUpdateCallback's callbacks, an items array, into
// prefixItems, its OpenAPI 3.1 form.
func fixTupleItems(doc map[string]any) int {
	branches, _ := lookup(doc, "components", "schemas", "OPALUpdateCallback", "properties",
		"callbacks", "items", "anyOf").([]any)
	n := 0
	for _, item := range branches {
		branch, _ := item.(map[string]any)
		if tuple, ok := branch["items"].([]any); ok {
			delete(branch, "items")
			branch["prefixItems"] = tuple
			n++
		}
	}
	return n
}

// fixRepeatedTag leaves out every declaration of the tag Bulk Operations after the first.
func fixRepeatedTag(doc map[string]any) int {
	tags, _ := doc["tags"].([]any)
	var kept []any
	seen := false
	n := 0
	for _, tag := range tags {
		if name, _ := lookup(tag, "name").(string); name == "Bulk Operations" {
			if seen {
				n++
				continue
			}
			seen = true
		}
		kept = append(kept, tag)
	}
	if n > 0 {
		doc["tags"] = kept
	}
	return n
}

// lookup returns the value at the path of keys under v, or nil when there is none.
func lookup(v any, keys ...string) any {
	for _, key := range keys {
		object, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = object[key]
	}
	return v
}
