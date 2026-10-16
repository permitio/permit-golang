package drift

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/wire"
)

// fixtureFile is the fixture spec of the tests.
const fixtureFile = "testdata/spec.json"

// fixtureSpec reads the fixture spec as the spec control-plane.
func fixtureSpec(t *testing.T) *spec {
	t.Helper()
	s, err := readSpec(defaultSpec, fixtureFile)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// inlineSpec returns the spec name whose components.schemas are schemas, a JSON object.
func inlineSpec(t *testing.T, name, schemas string) *spec {
	t.Helper()
	var doc map[string]any
	if err := wire.Decode([]byte(`{"components": {"schemas": `+schemas+`}}`), &doc); err != nil {
		t.Fatalf("the schemas %s: %v", schemas, err)
	}
	return &spec{name: name, doc: doc}
}

// jsonValue returns the JSON value text with each number as a json.Number.
func jsonValue(t *testing.T, text string) any {
	t.Helper()
	var v any
	if err := wire.Decode([]byte(text), &v); err != nil {
		t.Fatalf("the value %s: %v", text, err)
	}
	return v
}

// jsonObject returns the JSON object text with each number as a json.Number.
func jsonObject(t *testing.T, text string) map[string]any {
	t.Helper()
	object, ok := jsonValue(t, text).(map[string]any)
	if !ok {
		t.Fatalf("%s is not a JSON object", text)
	}
	return object
}

// schemaNode returns the schema name of s, and fails t when s has none or it is not an object.
func schemaNode(t *testing.T, s *spec, name string) map[string]any {
	t.Helper()
	node, ok := schemaOf(t, s, name).(map[string]any)
	if !ok {
		t.Fatalf("the schema %s is not an object", name)
	}
	return node
}

// schemaOf returns the schema name of s, and fails t when s has none.
func schemaOf(t *testing.T, s *spec, name string) any {
	t.Helper()
	schema, ok := s.schema(name)
	if !ok {
		t.Fatalf("the spec has no schema %s", name)
	}
	return schema
}

func TestReadSpecs(t *testing.T) {
	dir := t.TempDir()
	fallback := t.TempDir()
	for _, name := range specNames {
		writeFile(t, filepath.Join(fallback, name+".json"), `{"info": {"title": "`+name+`"}}`)
	}
	writeFile(t, filepath.Join(dir, "pdp-cloud.json"), `{"info": {"title": "live"}}`)
	specs, lines, err := readSpecs(dir, fallback)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"control-plane: " + filepath.Join(fallback, "control-plane.json"),
		"pdp-container: " + filepath.Join(fallback, "pdp-container.json"),
		"pdp-cloud: " + filepath.Join(dir, "pdp-cloud.json"),
		"elements-login: " + filepath.Join(fallback, "elements-login.json"),
	}
	if !slices.Equal(lines, want) {
		t.Errorf("readSpecs read\n%q\nwant\n%q", lines, want)
	}
	if got := specs["pdp-cloud"].doc["info"]; !reflect.DeepEqual(got, map[string]any{
		"title": "live",
	}) {
		t.Errorf("pdp-cloud has the info %v, want the one of the directory", got)
	}
	if specs["control-plane"].name != "control-plane" {
		t.Errorf("the spec control-plane is named %q", specs["control-plane"].name)
	}

	writeFile(t, filepath.Join(dir, "control-plane.json"), `[]`)
	writeFile(t, filepath.Join(dir, "pdp-container.json"), `{"a": 1} {}`)
	for _, tc := range []struct {
		name, dir, want string
	}{
		{"a directory that does not exist", filepath.Join(dir, "missing"), "the spec directory"},
		{"a file", filepath.Join(dir, "pdp-cloud.json"), "is not a directory"},
		{"a snapshot that is not an object", dir, "control-plane.json: json: cannot unmarshal"},
	} {
		if _, _, err := readSpecs(tc.dir, fallback); err == nil ||
			!strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: readSpecs fails with %v, want %q", tc.name, err, tc.want)
		}
	}
	for _, tc := range []struct{ text, want string }{
		{`{"a": 1} {}`, "the body holds more than one JSON value"},
		{`null`, "it is not a JSON object"},
	} {
		file := filepath.Join(dir, "one.json")
		writeFile(t, file, tc.text)
		if _, err := readSpec("one", file); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("readSpec of %s fails with %v, want %q", tc.text, err, tc.want)
		}
	}
	if _, err := readSpec("one", filepath.Join(dir, "none.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("readSpec of a missing file fails with %v, want os.ErrNotExist", err)
	}
}

// writeFile writes text to file.
func writeFile(t *testing.T, file, text string) {
	t.Helper()
	if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFindOperation(t *testing.T) {
	cloud := inlineSpec(t, "pdp-cloud", `{}`)
	cloud.doc["paths"] = map[string]any{"/allowed": map[string]any{
		"post": map[string]any{"operationId": "is_allowed"},
	}}
	specs := map[string]*spec{defaultSpec: fixtureSpec(t), "pdp-cloud": cloud}
	for _, tc := range []struct {
		id, spec, method, path, at string
	}{
		{"get_env", defaultSpec, "GET", "/envs/{env_id}", "#/paths/~1envs~1{env_id}/get"},
		{"update_env", defaultSpec, "PATCH", "/envs/{env_id}", "#/paths/~1envs~1{env_id}/patch"},
		{"is_allowed", "pdp-cloud", "POST", "/allowed", "#/paths/~1allowed/post"},
	} {
		o, ok := findOperation(specs, tc.id)
		if !ok || o.spec.name != tc.spec || o.method != tc.method || o.path != tc.path ||
			o.at != tc.at || o.node["operationId"] != tc.id {
			t.Errorf("findOperation(%s) = %s %s %s at %s, %t; want %s %s %s at %s", tc.id,
				o.spec.name, o.method, o.path, o.at, ok, tc.spec, tc.method, tc.path, tc.at)
		}
	}
	if o, ok := findOperation(specs, "get_nothing"); ok {
		t.Errorf("findOperation(get_nothing) = %+v, want none", o)
	}
}

func TestJSONBody(t *testing.T) {
	node := jsonObject(t, `{"content": {"application/json": {"schema": {"type": "string"}},
		"text/plain": {"schema": {"type": "integer"}}}}`)
	schema, at, ok := jsonBody(node, "#/x")
	if !ok || !reflect.DeepEqual(schema, map[string]any{"type": "string"}) ||
		at != "#/x/content/application~1json/schema" {
		t.Errorf("jsonBody = %v at %s, %t; want the application/json schema", schema, at, ok)
	}
	for _, text := range []string{
		`{}`, `{"content": {"text/plain": {"schema": {}}}}`,
		`{"content": {"application/json": {}}}`,
	} {
		node := jsonObject(t, text)
		if schema, _, ok := jsonBody(node, "#"); ok {
			t.Errorf("jsonBody(%s) = %v, want none", text, schema)
		}
	}
	if _, _, ok := jsonBody(nil, "#"); ok {
		t.Error("jsonBody(nil) found a body")
	}
}

func TestResolve(t *testing.T) {
	s := inlineSpec(t, defaultSpec, `{"A": {"type": "string"}, "a/b~c": {"type": "integer"},
		"L": {"anyOf": [{"type": "string"}, {"type": "boolean"}]}}`)
	s.doc["paths"] = map[string]any{"a": map[string]any{"b": "c"}}
	for _, tc := range []struct {
		ref, name string
		want      any
	}{
		{"#/components/schemas/A", "A", map[string]any{"type": "string"}},
		{"#/components/schemas/a~1b~0c", "a/b~c", map[string]any{"type": "integer"}},
		{"#/components/schemas/L/anyOf/1", "", map[string]any{"type": "boolean"}},
		{"#/components/schemas/A/type", "", "string"},
		{"#/paths/a/b", "", "c"},
	} {
		got, name, err := s.resolve(tc.ref)
		if err != nil || name != tc.name || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("resolve(%s) = %v, %q, %v; want %v, %q", tc.ref, got, name, err, tc.want,
				tc.name)
		}
	}
	for _, tc := range []struct{ ref, want string }{
		{"other.json#/components/schemas/A", "other.json#/components/schemas/A: the $ref points " +
			"outside the spec, and the walker reads only the spec it is in"},
		{"#/components/schemas/B", "#/components/schemas/B: the $ref points at nothing in the " +
			"spec"},
		{"#/components/schemas/L/anyOf/2", "#/components/schemas/L/anyOf/2: the $ref points at " +
			"nothing in the spec"},
		{"#/components/schemas/L/anyOf/-1", "#/components/schemas/L/anyOf/-1: the $ref points " +
			"at nothing in the spec"},
		{"#/components/schemas/L/anyOf/x", "#/components/schemas/L/anyOf/x: the $ref points at " +
			"nothing in the spec"},
		{"#/components/schemas/A/type/x", "#/components/schemas/A/type/x: the $ref points at " +
			"nothing in the spec"},
	} {
		_, _, err := s.resolve(tc.ref)
		if !isUnsupported(err) || err.Error() != tc.want {
			t.Errorf("resolve(%s) fails with %v, want the *unsupportedError %q", tc.ref, err,
				tc.want)
		}
	}
}

func TestEscape(t *testing.T) {
	if got := escape("/v2/a~b/{id}"); got != "~1v2~1a~0b~1{id}" {
		t.Errorf("escape = %q, want ~1v2~1a~0b~1{id}", got)
	}
}

func TestEveryCheckedKeywordIsAudited(t *testing.T) {
	valid := map[string]any{
		"$ref": "#/components/schemas/A", "type": "string", "enum": []any{"a"},
		"properties": map[string]any{}, "required": []any{"a"}, "additionalProperties": true,
		"patternProperties": map[string]any{}, "propertyNames": true, "items": true,
		"prefixItems": []any{true}, "anyOf": []any{true}, "oneOf": []any{true},
		"allOf": []any{true},
	}
	s := inlineSpec(t, defaultSpec, `{"A": {}}`)
	for key, class := range keywords {
		if class != checked {
			continue
		}
		value, ok := valid[key]
		if !ok {
			t.Errorf("the test has no valid value of the checked keyword %s; add one", key)
			continue
		}
		if err := s.auditKeyword(key, value, "#", map[string]bool{}); err != nil {
			t.Errorf("auditing %s: %v", key, err)
		}
	}
	err := s.auditKeyword("format", 1, "#", map[string]bool{})
	if err != nil {
		t.Errorf("auditing an unchecked keyword fails with %v", err)
	}
	keywords["fake"] = checked
	defer delete(keywords, "fake")
	err = s.auditKeyword("fake", true, "#/S", map[string]bool{})
	if err == nil || err.Error() != `#/S: the walker checks the keyword "fake" and audit does `+
		"not read it; add it to auditKeyword" {
		t.Errorf("auditing a checked keyword that audit does not read fails with %v", err)
	}
}

func TestAudit(t *testing.T) {
	for _, tc := range []struct {
		name, schemas, want string
	}{
		{"every checked keyword", `{"S": {"$ref": "#/components/schemas/T"}, "T": {
			"type": ["object", "null"], "enum": [{}, null], "required": ["a"],
			"properties": {"a": {"items": {"type": "string"}, "prefixItems": [true, false]}},
			"patternProperties": {"^x-": {"anyOf": [{"type": "integer"}]}},
			"additionalProperties": false, "propertyNames": {"oneOf": [{"type": "string"}]},
			"allOf": [{"$ref": "#/components/schemas/T"}]}}`, ""},
		{"every unchecked keyword", `{"S": {"default": 1, "deprecated": true,
			"discriminator": {"propertyName": "kind", "mapping": {"a": "#/x"}},
			"exclusiveMinimum": 0, "format": "uuid", "hidden_from_schema": true,
			"maxItems": 1, "maxLength": 1, "minItems": 1, "minLength": 1, "minimum": 1,
			"pattern": "(", "uniqueItems": true}}`, ""},
		{"a boolean schema", `{"S": {"items": false, "additionalProperties": true}}`, ""},
		{
			"a keyword the walker does not know", `{"S": {"not": {"type": "string"}}}`,
			`#/components/schemas/S: the keyword "not" is not one the walker implements; add it ` +
				"to keywords in tools/drift/schema.go",
		},
		{
			"one in a property", `{"S": {"properties": {"a/b": {"const": 1}}}}`,
			`#/components/schemas/S/properties/a~1b: the keyword "const" is not one the walker ` +
				"implements; add it to keywords in tools/drift/schema.go",
		},
		{
			"one behind a $ref", `{"S": {"anyOf": [{"$ref": "#/components/schemas/T"}]},
			"T": {"if": true}}`,
			`#/components/schemas/T: the keyword "if" is not one the walker implements; add it ` +
				"to keywords in tools/drift/schema.go",
		},
		{"an extension", `{"S": {"x-order": 1}}`, `#/components/schemas/S: the keyword ` +
			`"x-order" is not one the walker implements; add it to keywords in ` +
			"tools/drift/schema.go"},
		{"a $ref outside the spec", `{"S": {"$ref": "other.json#/S"}}`, "other.json#/S: the " +
			"$ref points outside the spec, and the walker reads only the spec it is in"},
		{
			"a $ref to nothing", `{"S": {"items": {"$ref": "#/components/schemas/T"}}}`,
			"#/components/schemas/T: the $ref points at nothing in the spec",
		},
		{"a $ref that is not a string", `{"S": {"$ref": 1}}`, "#/components/schemas/S/$ref: a " +
			"$ref is a string, and this one is number"},
		{
			"a type that JSON Schema does not have", `{"S": {"type": "HeadersAuth"}}`,
			`#/components/schemas/S/type: type is a JSON type, or a list of them, and this one ` +
				`is "HeadersAuth"`,
		},
		{
			"a list of types with one it does not have", `{"S": {"type": ["string", "int"]}}`,
			`#/components/schemas/S/type: type is a JSON type, or a list of them, and this one ` +
				`is ["string","int"]`,
		},
		{"an empty list of types", `{"S": {"type": []}}`, "#/components/schemas/S/type: type " +
			"is a JSON type, or a list of them, and this one is []"},
		{"an enum that is not a list", `{"S": {"enum": "a"}}`, `#/components/schemas/S/enum: ` +
			`enum is a list of values, and this one is "a"`},
		{"an empty enum", `{"S": {"enum": []}}`, "#/components/schemas/S/enum: enum is a list " +
			"of values, and this one is []"},
		{"required with a number", `{"S": {"required": ["a", 1]}}`, "#/components/schemas/S/" +
			`required: required is a list of names, and this one is ["a",1]`},
		{"properties that are a list", `{"S": {"properties": []}}`, "#/components/schemas/S/" +
			"properties: properties maps names to schemas, and this one is array"},
		{
			"a pattern that Go does not compile", `{"S": {"patternProperties": {"(?<=a)": {}}}}`,
			`#/components/schemas/S/patternProperties: the pattern "(?<=a)" does not compile as ` +
				"a Go regular expression: error parsing regexp: ",
		},
		{
			"items that are a list", `{"S": {"items": [{"type": "string"}]}}`,
			"#/components/schemas/S/items: items holds a list, a draft-04 tuple, which JSON " +
				"Schema 2020-12 writes as prefixItems",
		},
		{"an empty anyOf", `{"S": {"anyOf": []}}`, "#/components/schemas/S/anyOf: anyOf is a " +
			"list of schemas, and this one is []"},
		{"an allOf that is not a list", `{"S": {"allOf": {}}}`, "#/components/schemas/S/allOf: " +
			"allOf is a list of schemas, and this one is {}"},
		{
			"a schema that is a string", `{"S": {"properties": {"a": "string"}}}`,
			"#/components/schemas/S/properties/a: a schema is a JSON object or a boolean, and " +
				"this one is string",
		},
		{
			"one deep in prefixItems", `{"S": {"prefixItems": [true, {"items": {"oneOf": [
			{"additionalProperties": {"propertyNames": {"contains": true}}}]}}]}}`,
			"#/components/schemas/S/prefixItems/1/items/oneOf/0/additionalProperties/" +
				`propertyNames: the keyword "contains" is not one the walker implements; add ` +
				"it to keywords in tools/drift/schema.go",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := inlineSpec(t, defaultSpec, tc.schemas)
			err := s.audit(schemaOf(t, s, "S"), schemaPrefix+"S")
			if tc.want == "" {
				if err != nil {
					t.Errorf("audit fails with %v, want nil", err)
				}
				return
			}
			if !isUnsupported(err) || !strings.HasPrefix(err.Error(), tc.want) {
				t.Errorf("audit fails with\n%v\nwant the *unsupportedError\n%s", err, tc.want)
			}
		})
	}
}

func TestTypeList(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  []string
		ok    bool
	}{
		{"string", []string{"string"}, true},
		{[]any{"integer", "null"}, []string{"integer", "null"}, true},
		{"int", []string{"int"}, false},
		{[]any{}, nil, false},
		{[]any{"string", 1}, nil, false},
		{[]any{"string", "int"}, nil, false},
		{1, nil, false},
	} {
		got, ok := typeList(tc.value)
		if !slices.Equal(got, tc.want) || ok != tc.ok {
			t.Errorf("typeList(%v) = %q, %t; want %q, %t", tc.value, got, ok, tc.want, tc.ok)
		}
	}
}

func TestAlternatives(t *testing.T) {
	s := inlineSpec(t, defaultSpec, `{
		"Named": {"type": "object", "properties": {"a": {"type": "string"}}},
		"Union": {"anyOf": [{"$ref": "#/components/schemas/Named"}, {"type": "integer"}]},
		"Alias": {"$ref": "#/components/schemas/Named"},
		"Types": {"type": ["string", "null"], "format": "uuid"},
		"One": {"allOf": [{"$ref": "#/components/schemas/Named"}], "default": {}},
		"Two": {"allOf": [{"$ref": "#/components/schemas/Named"}, {"required": ["a"]}]},
		"Nested": {"oneOf": [
			{"anyOf": [{"type": "boolean"}, {"$ref": "#/components/schemas/Union"}]},
			true, false]},
		"RefAndType": {"$ref": "#/components/schemas/Named", "type": "object"},
		"AnyOfAndOneOf": {"anyOf": [true], "oneOf": [true]},
		"AnyOfAndProperties": {"anyOf": [true], "properties": {}},
		"Loop": {"anyOf": [{"$ref": "#/components/schemas/Loop"}]}
	}`)
	named := schemaNode(t, s, "Named")
	for _, tc := range []struct {
		schema string
		want   []alternative
	}{
		{"Named", []alternative{{named, ""}}},
		{"Union", []alternative{
			{named, "Named"}, {map[string]any{"type": "integer"}, ""},
		}},
		{"Alias", []alternative{{named, "Named"}}},
		{"Types", []alternative{
			{map[string]any{"type": "string", "format": "uuid"}, ""},
			{map[string]any{"type": "null", "format": "uuid"}, ""},
		}},
		{"One", []alternative{{named, "Named"}}},
		{"Two", []alternative{{schemaNode(t, s, "Two"), ""}}},
		{"Nested", []alternative{
			{map[string]any{"type": "boolean"}, ""},
			{named, "Named"},
			{map[string]any{"type": "integer"}, ""},
			{map[string]any{}, ""},
		}},
	} {
		got, err := s.alternatives(schemaOf(t, s, tc.schema), schemaPrefix+tc.schema)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("alternatives of %s = %v, %v; want %v", tc.schema, got, err, tc.want)
		}
	}
	for _, tc := range []struct{ schema, want string }{
		{"RefAndType", "#/components/schemas/RefAndType: $ref beside type: the walker does not " +
			"merge type into each alternative"},
		{"AnyOfAndOneOf", "#/components/schemas/AnyOfAndOneOf: anyOf beside oneOf: the walker " +
			"does not merge them"},
		{"AnyOfAndProperties", "#/components/schemas/AnyOfAndProperties: anyOf beside " +
			"properties: the walker does not merge properties into each alternative"},
		{"Loop", "#/components/schemas/Loop: the schema is its own alternative, through $ref, " +
			"anyOf, oneOf or allOf"},
	} {
		_, err := s.alternatives(schemaOf(t, s, tc.schema), schemaPrefix+tc.schema)
		if !isUnsupported(err) || err.Error() != tc.want {
			t.Errorf("alternatives of %s fails with\n%v\nwant\n%s", tc.schema, err, tc.want)
		}
	}
	got, err := s.alternatives(false, "#")
	if err != nil || len(got) != 0 {
		t.Errorf("alternatives of false = %v, %v; want none", got, err)
	}
}

func TestTypeOf(t *testing.T) {
	for text, want := range map[string]string{
		`{"type": "integer", "properties": {}}`: "integer",
		`{"properties": {}}`:                    "object",
		`{"patternProperties": {}}`:             "object",
		`{"additionalProperties": true}`:        "object",
		`{"propertyNames": true}`:               "object",
		`{"required": []}`:                      "object",
		`{"allOf": []}`:                         "object",
		`{"items": true}`:                       "array",
		`{"prefixItems": []}`:                   "array",
		`{"enum": ["a"]}`:                       "string",
		`{"enum": [1]}`:                         "number",
		`{"enum": []}`:                          "",
		`{"format": "uuid"}`:                    "",
		`{}`:                                    "",
	} {
		if got := typeOf(jsonObject(t, text)); got != want {
			t.Errorf("typeOf(%s) = %q, want %q", text, got, want)
		}
	}
}

func TestObject(t *testing.T) {
	s := inlineSpec(t, defaultSpec, `{
		"Base": {"properties": {"id": {"type": "string"}}, "required": ["id"],
			"patternProperties": {"^x-": true}},
		"Child": {"allOf": [{"$ref": "#/components/schemas/Base"},
			{"$ref": "#/components/schemas/Base"},
			{"properties": {"name": {"type": "string"}}, "required": ["name"]}],
			"properties": {"id": {"type": "string"}}, "additionalProperties": false},
		"Clash": {"allOf": [{"$ref": "#/components/schemas/Base"},
			{"properties": {"id": {"type": "integer"}}}]},
		"Lost": {"allOf": [{"$ref": "#/components/schemas/None"}]},
		"Odd": {"allOf": [true, {"$ref": "#/components/schemas/Bool"}]},
		"Bool": true
	}`)
	shape, err := s.object(schemaNode(t, s, "Child"), "#/c")
	want := objectShape{
		properties: map[string]any{
			"id": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"},
		},
		required:   map[string]bool{"id": true, "name": true},
		patterns:   map[string]any{"^x-": true},
		additional: false,
	}
	if err != nil || !reflect.DeepEqual(shape, want) {
		t.Errorf("object(Child) = %+v, %v; want %+v", shape, err, want)
	}
	if !shape.lists() || !shape.listed("name") || !shape.listed("x-a") || shape.listed("y") {
		t.Errorf("the shape of Child lists the wrong names")
	}
	if (objectShape{}).lists() {
		t.Error("an empty shape lists names")
	}
	shape, err = s.object(schemaNode(t, s, "Odd"), "#/o")
	if err != nil || len(shape.properties) != 0 {
		t.Errorf("object(Odd) = %+v, %v; want no properties", shape, err)
	}
	_, err = s.object(schemaNode(t, s, "Clash"), "#/c")
	if err == nil || err.Error() != `#/c/allOf/1: the properties "id" has two schemas in an `+
		"allOf, which the walker does not merge" {
		t.Errorf("object(Clash) fails with %v", err)
	}
	_, err = s.object(schemaNode(t, s, "Lost"), "#/l")
	if err == nil || err.Error() != "#/components/schemas/None: the $ref points at nothing in "+
		"the spec" {
		t.Errorf("object(Lost) fails with %v", err)
	}
}

func TestValidate(t *testing.T) {
	s := inlineSpec(t, defaultSpec, `{
		"Scope": {"type": "object", "properties": {"org": {"type": "string"},
			"tags": {"type": "array", "items": {"type": "string"}}}, "required": ["org"],
			"additionalProperties": false},
		"Open": {"type": "object", "properties": {"a": {"type": "integer"}}},
		"Bag": {"type": "object"},
		"Map": {"type": "object", "additionalProperties": {"type": "integer"}},
		"Roles": {"type": "object", "patternProperties": {"^[a-z]+$": {"type": "string"}}},
		"Names": {"type": "object", "propertyNames": {"enum": ["a", "b"]}},
		"Pair": {"type": "array", "prefixItems": [{"type": "string"}, {"type": "integer"}],
			"items": {"type": "boolean"}},
		"Color": {"enum": ["red", 1, null]},
		"Nullable": {"type": ["string", "null"]},
		"Int": {"type": "integer"},
		"Any": {"anyOf": [{"type": "string"}, {"$ref": "#/components/schemas/Scope"}]},
		"One": {"oneOf": [{"type": "integer"}, {"type": "number"}]},
		"Both": {"allOf": [{"$ref": "#/components/schemas/Open"},
			{"properties": {"b": {"type": "string"}}}]},
		"Wrapped": {"allOf": [{"$ref": "#/components/schemas/Scope"}], "default": {}},
		"Never": false,
		"Values": {"type": "string", "pattern": "^a$", "minLength": 5, "format": "uuid"}
	}`)
	for _, tc := range []struct {
		schema, value string
		want          []string
	}{
		{"Scope", `{"org": "o", "tags": ["a"]}`, nil},
		{"Scope", `{"tags": [1], "extra": true}`, []string{
			"#: has no property org, which the schema at S requires",
			"#/extra: is a value where the schema at S/additionalProperties allows none",
			"#/tags/0: is number, and the schema at S/properties/tags/items allows string",
		}},
		{"Scope", `[]`, []string{"#: is array, and the schema at S allows object"}},
		{"Open", `{"a": 1, "b": 2}`, []string{
			"#/b: is a property that the schema at S does not list",
		}},
		{"Bag", `{"a": 1, "b": {"c": []}}`, nil},
		{"Map", `{"a": 1, "b": "x"}`, []string{
			"#/b: is string, and the schema at S/additionalProperties allows integer",
		}},
		{"Roles", `{"admin": "r", "Admin": "r", "viewer": 1}`, []string{
			"#/Admin: is a property that the schema at S does not list",
			"#/viewer: is number, and the schema at S/patternProperties/^[a-z]+$ allows string",
		}},
		{"Names", `{"a": 1, "c": 2}`, []string{
			`#/c: has a name that is "c", which is not one of the values of the enum at ` +
				`S/propertyNames, ["a","b"]`,
		}},
		{"Pair", `["a", 1, true, false]`, nil},
		{"Pair", `[1, "a", "b"]`, []string{
			"#/0: is number, and the schema at S/prefixItems/0 allows string",
			"#/1: is string, and the schema at S/prefixItems/1 allows integer",
			"#/2: is string, and the schema at S/items allows boolean",
		}},
		{"Color", `"red"`, nil},
		{"Color", `1.0`, nil},
		{"Color", `null`, nil},
		{"Color", `"blue"`, []string{
			`#: is "blue", which is not one of the values of the enum at S, ["red",1,null]`,
		}},
		{"Nullable", `null`, nil},
		{"Nullable", `true`, []string{"#: is boolean, and the schema at S allows string or null"}},
		{"Int", `2`, nil},
		{"Int", `2.0`, nil},
		{"Int", `-1e3`, nil},
		{"Int", `2.5`, []string{"#: is number, and the schema at S allows integer"}},
		{"Any", `"a"`, nil},
		{"Any", `{"org": "o"}`, nil},
		{"Any", `{"tags": "x"}`, []string{
			"#: fits none of the 2 alternatives of anyOf at S; 0: # is object, and the schema " +
				"at S/anyOf/0 allows string; 1: # has no property org, which the schema at " +
				"#/components/schemas/Scope requires",
		}},
		{"One", `1.5`, nil},
		{"One", `1`, []string{
			"#: fits 2 of the alternatives of oneOf at S, which must fit one",
		}},
		{"One", `"a"`, []string{
			"#: fits none of the 2 alternatives of oneOf at S; 0: # is string, and the schema " +
				"at S/oneOf/0 allows integer; 1: # is string, and the schema at S/oneOf/1 " +
				"allows number",
		}},
		{"Both", `{"a": 1, "b": "x"}`, nil},
		{"Both", `{"a": 1, "b": "x", "c": 3}`, []string{
			"#/c: is a property that the schema at S does not list",
		}},
		{"Wrapped", `{"org": "o", "extra": 1}`, []string{
			"#/extra: is a value where the schema at #/components/schemas/Scope/" +
				"additionalProperties allows none",
		}},
		{"Never", `1`, []string{"#: is a value where the schema at S allows none"}},
		{"Values", `"b"`, nil},
	} {
		value := jsonValue(t, tc.value)
		var got []string
		for _, v := range s.validate(value, schemaOf(t, s, tc.schema), "S") {
			got = append(got, pointer(v.at)+": "+v.text)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("validate(%s) against %s =\n%q\nwant\n%q", tc.value, tc.schema, got, tc.want)
		}
	}
}

func TestValidateReportsAStaleRef(t *testing.T) {
	s := inlineSpec(t, defaultSpec, `{}`)
	got := s.validate("a", map[string]any{"$ref": "#/components/schemas/Gone"}, "S")
	want := []violation{{"", "#/components/schemas/Gone: the $ref points at nothing in the spec"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("validate with a $ref to nothing = %v, want %v", got, want)
	}
	got = s.validate(map[string]any{"a": "x"}, map[string]any{"allOf": []any{
		map[string]any{"properties": map[string]any{"a": map[string]any{"type": "string"}}},
		map[string]any{"properties": map[string]any{"a": map[string]any{"type": "integer"}}},
	}}, "S")
	want = []violation{
		{"", `S/allOf/1: the properties "a" has two schemas in an allOf, which the walker does ` +
			"not merge"},
		{"/a", "is string, and the schema at S/allOf/1/properties/a allows integer"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("validate with a clash in allOf = %v, want %v", got, want)
	}
}

func TestClosesAtHolder(t *testing.T) {
	for text, want := range map[string]bool{
		`{}`:                                  false,
		`{"allOf": [true]}`:                   false,
		`{"allOf": [true, true]}`:             true,
		`{"allOf": [true], "properties": {}}`: true,
		`{"allOf": [true], "patternProperties": {}}`:    true,
		`{"properties": {}, "patternProperties": {}}`:   false,
		`{"allOf": [true, true], "properties": {}}`:     true,
		`{"allOf": [], "properties": {}}`:               false,
		`{"allOf": [true], "additionalProperties": {}}`: false,
	} {
		if got := closesAtHolder(jsonObject(t, text)); got != want {
			t.Errorf("closesAtHolder(%s) = %t, want %t", text, got, want)
		}
	}
}

func TestKindAndJSONEqual(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  string
	}{
		{nil, "null"},
		{true, "boolean"},
		{json.Number("1"), "number"},
		{"a", "string"},
		{[]any{}, "array"},
		{map[string]any{}, "object"},
		{1, "a Go int"},
	} {
		if got := kind(tc.value); got != tc.want {
			t.Errorf("kind(%#v) = %q, want %q", tc.value, got, tc.want)
		}
	}
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{`1`, `1.0`, true},
		{`1`, `1e0`, true},
		{`1`, `2`, false},
		{`1`, `"1"`, false},
		{`"a"`, `"a"`, true},
		{`"a"`, `"b"`, false},
		{`null`, `null`, true},
		{`null`, `false`, false},
		{`[1, {"a": 2}]`, `[1.0, {"a": 2e0}]`, true},
		{`[1]`, `[1, 1]`, false},
		{`[1]`, `{}`, false},
		{`{"a": 1}`, `{"a": 1, "b": 1}`, false},
		{`{"a": 1}`, `{"b": 1}`, false},
		{`{}`, `[]`, false},
	} {
		if got := jsonEqual(jsonValue(t, tc.a), jsonValue(t, tc.b)); got != tc.want {
			t.Errorf("jsonEqual(%s, %s) = %t, want %t", tc.a, tc.b, got, tc.want)
		}
	}
	if jsonEqual(json.Number("x"), json.Number("x")) {
		t.Error("jsonEqual of a number that does not parse is true")
	}
	if got := compact(map[string]any{"b": []any{1}}); got != `{"b":[1]}` {
		t.Errorf("compact = %s", got)
	}
	if got := compact(func() {}); !strings.HasPrefix(got, "0x") {
		t.Errorf("compact of a func = %s, want its %%v", got)
	}
}
