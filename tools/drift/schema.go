package drift

import (
	"encoding/json"
	"fmt"
	"maps"
	"math/big"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// keywordClass says how the walker treats a schema keyword.
type keywordClass int

const (
	// The zero keywordClass is a keyword that the walker does not implement.
	_ keywordClass = iota

	// checked is a keyword that the walker checks.
	checked

	// unchecked is a keyword that says nothing about the shape of a value: documentation, a
	// default, a format, or a bound on a value, such as pattern or minimum. The walker checks the
	// shape of a value, not its content.
	unchecked
)

// keywords are the schema keywords of the spec snapshots, each with how the walker treats it. A
// keyword missing from here stops a check as did not run, so that nothing is skipped unseen: add
// a new one with its class, and when it is checked, its check.
var keywords = map[string]keywordClass{
	"$ref":                 checked,
	"type":                 checked,
	"enum":                 checked,
	"properties":           checked,
	"required":             checked,
	"additionalProperties": checked,
	"patternProperties":    checked,
	"propertyNames":        checked,
	"items":                checked,
	"prefixItems":          checked,
	"anyOf":                checked,
	"oneOf":                checked,
	"allOf":                checked,

	"default":            unchecked,
	"deprecated":         unchecked,
	"discriminator":      unchecked,
	"exclusiveMinimum":   unchecked,
	"format":             unchecked,
	"hidden_from_schema": unchecked,
	"maxItems":           unchecked,
	"maxLength":          unchecked,
	"minItems":           unchecked,
	"minLength":          unchecked,
	"minimum":            unchecked,
	"pattern":            unchecked,
	"uniqueItems":        unchecked,
}

// jsonTypes are the types of JSON Schema.
var jsonTypes = []string{"null", "boolean", "object", "array", "number", "string", "integer"}

// audit returns an *unsupportedError for the first schema that schema reaches, through its
// subschemas and $refs, that the walker does not implement: one with a keyword missing from
// keywords, a $ref that it cannot follow, or a keyword whose value is not of the shape that JSON
// Schema gives it. at is where schema is, for the error. Every check runs audit on its schema
// first, so that what it finds does not depend on the value it checks.
func (s *spec) audit(schema any, at string) error {
	return s.auditSchema(schema, at, map[string]bool{})
}

// auditSchema audits schema, which at locates, and each $ref that seen does not hold yet.
func (s *spec) auditSchema(schema any, at string, seen map[string]bool) error {
	switch node := schema.(type) {
	case bool:
		return nil
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(node)) {
			if err := s.auditKeyword(key, node[key], at, seen); err != nil {
				return err
			}
		}
		return nil
	default:
		return unsupported(at, "a schema is a JSON object or a boolean, and this one is "+
			kind(schema))
	}
}

// auditKeyword audits the keyword key, whose value is value, of the schema that at locates.
func (s *spec) auditKeyword(key string, value any, at string, seen map[string]bool) error {
	where := at + "/" + escape(key)
	switch keywords[key] {
	case unchecked:
		return nil
	case checked:
	default:
		return unsupported(at, fmt.Sprintf("the keyword %q is not one the walker implements; add "+
			"it to keywords in tools/drift/schema.go", key))
	}
	switch key {
	case "$ref":
		ref, ok := value.(string)
		if !ok {
			return unsupported(where, "a $ref is a string, and this one is "+kind(value))
		}
		if seen[ref] {
			return nil
		}
		seen[ref] = true
		target, _, err := s.resolve(ref)
		if err != nil {
			return err
		}
		return s.auditSchema(target, ref, seen)
	case "type":
		if _, ok := typeList(value); !ok {
			return unsupported(where, "type is a JSON type, or a list of them, and this one is "+
				compact(value))
		}
	case "enum":
		if values, ok := value.([]any); !ok || len(values) == 0 {
			return unsupported(where, "enum is a list of values, and this one is "+compact(value))
		}
	case "required":
		if _, ok := stringList(value); !ok {
			return unsupported(where, "required is a list of names, and this one is "+
				compact(value))
		}
	case "properties", "patternProperties":
		return s.auditMap(key, value, where, seen)
	case "items":
		if _, ok := value.([]any); ok {
			return unsupported(where, "items holds a list, a draft-04 tuple, which JSON Schema "+
				"2020-12 writes as prefixItems")
		}
		return s.auditSchema(value, where, seen)
	case "additionalProperties", "propertyNames":
		return s.auditSchema(value, where, seen)
	case "prefixItems", "anyOf", "oneOf", "allOf":
		list, ok := value.([]any)
		if !ok || len(list) == 0 {
			return unsupported(where, key+" is a list of schemas, and this one is "+compact(value))
		}
		for i, item := range list {
			if err := s.auditSchema(item, where+"/"+strconv.Itoa(i), seen); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("%s: the walker checks the keyword %q and audit does not read it; "+
			"add it to auditKeyword", at, key)
	}
	return nil
}

// auditMap audits the schemas of properties or patternProperties, value, at where; each key of
// patternProperties must compile as a Go regular expression.
func (s *spec) auditMap(key string, value any, where string, seen map[string]bool) error {
	schemas, ok := value.(map[string]any)
	if !ok {
		return unsupported(where, key+" maps names to schemas, and this one is "+kind(value))
	}
	for _, name := range slices.Sorted(maps.Keys(schemas)) {
		if key == "patternProperties" {
			if _, err := regexp.Compile(name); err != nil {
				return unsupported(where, fmt.Sprintf("the pattern %q does not compile as a Go "+
					"regular expression: %v", name, err))
			}
		}
		if err := s.auditSchema(schemas[name], where+"/"+escape(name), seen); err != nil {
			return err
		}
	}
	return nil
}

// typeList returns the types that the value of a type keyword names, and false when it is not
// a JSON type or a list of them.
func typeList(value any) ([]string, bool) {
	if name, ok := value.(string); ok {
		return []string{name}, slices.Contains(jsonTypes, name)
	}
	names, ok := stringList(value)
	if !ok || len(names) == 0 {
		return nil, false
	}
	for _, name := range names {
		if !slices.Contains(jsonTypes, name) {
			return nil, false
		}
	}
	return names, true
}

// stringList returns value as a list of strings, and false when it is not one.
func stringList(value any) ([]string, bool) {
	list, ok := value.([]any)
	if !ok {
		return nil, false
	}
	names := make([]string, 0, len(list))
	for _, item := range list {
		name, ok := item.(string)
		if !ok {
			return nil, false
		}
		names = append(names, name)
	}
	return names, true
}

// alternative is one shape of the values that a schema allows: a schema without anyOf, oneOf, a
// $ref, an allOf of one schema or a list of types, and the name of the schema of
// components.schemas that a $ref led to, when one did.
type alternative struct {
	schema map[string]any
	name   string
}

// alternatives returns the alternatives of schema, which audit accepted and which is at the
// place that at names: it follows each $ref, opens anyOf and oneOf into their branches and an
// allOf of one schema into that schema, and splits a list of types into one alternative for each
// type. A schema that opener rejects is an *unsupportedError.
func (s *spec) alternatives(schema any, at string) ([]alternative, error) {
	return s.alternativesOf(schema, at, map[string]bool{})
}

// alternativesOf returns the alternatives of schema, following each $ref that open does not hold,
// the refs it is in.
func (s *spec) alternativesOf(schema any, at string, open map[string]bool) ([]alternative, error) {
	node, ok := schema.(map[string]any)
	if !ok {
		if schema == true {
			return []alternative{{schema: map[string]any{}}}, nil
		}
		return nil, nil
	}
	key, err := opener(node, at)
	if err != nil {
		return nil, err
	}
	switch key {
	case "$ref":
		ref, _ := node["$ref"].(string)
		return s.refAlternatives(ref, open)
	case "anyOf", "oneOf", "allOf":
		branches, _ := node[key].([]any)
		var alts []alternative
		for i, branch := range branches {
			more, err := s.alternativesOf(branch, at+"/"+key+"/"+strconv.Itoa(i), open)
			if err != nil {
				return nil, err
			}
			alts = append(alts, more...)
		}
		return alts, nil
	}
	types, _ := typeList(node["type"])
	if len(types) < 2 {
		return []alternative{{schema: node}}, nil
	}
	alts := make([]alternative, 0, len(types))
	for _, t := range types {
		one := maps.Clone(node)
		one["type"] = t
		alts = append(alts, alternative{schema: one})
	}
	return alts, nil
}

// opener returns the keyword of node that opens it into other schemas: $ref, anyOf, oneOf, or
// an allOf of one schema; empty when it has none. Two of them, or one beside another checked
// keyword, is an *unsupportedError, located by at: the walker does not merge the keyword into each
// alternative.
func opener(node map[string]any, at string) (string, error) {
	var found string
	for _, key := range []string{"$ref", "anyOf", "oneOf", "allOf"} {
		value, ok := node[key]
		members, _ := value.([]any)
		if !ok || key == "allOf" && len(members) != 1 {
			continue
		}
		if found != "" {
			return "", unsupported(at, found+" beside "+key+": the walker does not merge them")
		}
		found = key
	}
	for key := range node {
		if found != "" && key != found && keywords[key] == checked {
			return "", unsupported(at, found+" beside "+key+": the walker does not merge "+key+
				" into each alternative")
		}
	}
	return found, nil
}

// refAlternatives returns the alternatives of the schema that ref points at: the schema itself,
// with the ref's name, when it is a single alternative, and else its alternatives. A ref that
// open holds already, one that leads back to itself without a value in between, is an
// *unsupportedError.
func (s *spec) refAlternatives(ref string, open map[string]bool) ([]alternative, error) {
	if open[ref] {
		return nil, unsupported(ref, "the schema is its own alternative, through $ref, anyOf, "+
			"oneOf or allOf")
	}
	target, name, err := s.resolve(ref)
	if err != nil {
		return nil, err
	}
	if node, ok := target.(map[string]any); ok {
		key, err := opener(node, ref)
		if err != nil {
			return nil, err
		}
		if types, _ := typeList(node["type"]); key == "" && len(types) < 2 {
			return []alternative{{schema: node, name: name}}, nil
		}
	}
	open[ref] = true
	defer delete(open, ref)
	return s.alternativesOf(target, ref, open)
}

// typeOf returns the JSON type of an alternative: its type keyword, or what its other keywords
// imply, such as object for one with properties; empty when it allows any value.
func typeOf(alt map[string]any) string {
	if t, ok := alt["type"].(string); ok {
		return t
	}
	for _, key := range []string{
		"properties", "patternProperties", "additionalProperties",
		"propertyNames", "required", "allOf",
	} {
		if _, ok := alt[key]; ok {
			return "object"
		}
	}
	for _, key := range []string{"items", "prefixItems"} {
		if _, ok := alt[key]; ok {
			return "array"
		}
	}
	if values, ok := alt["enum"].([]any); ok && len(values) > 0 {
		return kind(values[0])
	}
	return ""
}

// objectShape is what an object schema lists: its properties, the ones it requires, and its
// patternProperties, with those of each schema of its allOf merged in, and its own
// additionalProperties.
type objectShape struct {
	properties map[string]any
	required   map[string]bool
	patterns   map[string]any

	// additional is the schema's additionalProperties, or nil when it has none.
	additional any
}

// lists reports whether the shape lists any property, by name or by pattern.
func (o objectShape) lists() bool {
	return len(o.properties) > 0 || len(o.patterns) > 0
}

// listed reports whether the shape lists the property name, by name or by pattern.
func (o objectShape) listed(name string) bool {
	if _, ok := o.properties[name]; ok {
		return true
	}
	for pattern := range o.patterns {
		if regexp.MustCompile(pattern).MatchString(name) {
			return true
		}
	}
	return false
}

// object returns the shape of node, an object schema that audit accepted and at locates. A
// property that two schemas of the merge give different schemas is an *unsupportedError.
func (s *spec) object(node map[string]any, at string) (objectShape, error) {
	shape := objectShape{
		properties: map[string]any{}, required: map[string]bool{}, patterns: map[string]any{},
		additional: node["additionalProperties"],
	}
	return shape, s.mergeObject(&shape, node, at, map[string]bool{})
}

// mergeObject merges the properties, required names and patternProperties of node, and of each
// schema of its allOf, following the $ref of each, into shape.
func (s *spec) mergeObject(shape *objectShape, node map[string]any, at string,
	seen map[string]bool,
) error {
	for _, merge := range []struct {
		key  string
		into map[string]any
	}{{"properties", shape.properties}, {"patternProperties", shape.patterns}} {
		values, _ := node[merge.key].(map[string]any)
		for name, schema := range values {
			if old, ok := merge.into[name]; ok && !reflect.DeepEqual(old, schema) {
				return unsupported(at, fmt.Sprintf("the %s %q has two schemas in an allOf, which "+
					"the walker does not merge", merge.key, name))
			}
			merge.into[name] = schema
		}
	}
	names, _ := stringList(node["required"])
	for _, name := range names {
		shape.required[name] = true
	}
	members, _ := node["allOf"].([]any)
	for i, item := range members {
		member, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if ref, ok := member["$ref"].(string); ok && !seen[ref] {
			seen[ref] = true
			target, _, err := s.resolve(ref)
			if err != nil {
				return err
			}
			if target, ok := target.(map[string]any); ok {
				if err := s.mergeObject(shape, target, ref, seen); err != nil {
					return err
				}
			}
		}
		if err := s.mergeObject(shape, member, at+"/allOf/"+strconv.Itoa(i), seen); err != nil {
			return err
		}
	}
	return nil
}

// closesAtHolder reports whether node, an object schema with an allOf, decides itself which
// properties a value may hold, from the properties that it and the schemas of its allOf list:
// when it lists properties of its own, or its allOf has more than one schema. Otherwise the one
// schema of its allOf decides.
func closesAtHolder(node map[string]any) bool {
	members, _ := node["allOf"].([]any)
	_, properties := node["properties"]
	_, patterns := node["patternProperties"]
	return len(members) > 1 || len(members) == 1 && (properties || patterns)
}

// violation is a place where a value does not fit its schema.
type violation struct {
	// at is the JSON pointer of the value, such as /data/0/key; empty for the whole value.
	at string

	// text says what does not fit.
	text string
}

// validate returns the places where value, a JSON value decoded with each number as a json.Number,
// does not fit schema, which audit accepted and at locates. It follows JSON Schema 2020-12 for the
// keywords that keywords marks checked, with one difference: an object schema without
// additionalProperties accepts no property that it does not list, when it lists any, so that a
// value cannot hold a property that the spec does not name. Where closesAtHolder says so, the
// schema that holds an allOf decides this, from what it and the schemas of its allOf list.
func (s *spec) validate(value, schema any, at string) []violation {
	return s.check(value, schema, at, "", false)
}

// check returns the violations of value, at the JSON pointer path, against schema, which at
// locates. member says that schema is a schema of an allOf whose holder decides which properties
// a value may hold.
func (s *spec) check(value, schema any, at, path string, member bool) []violation {
	node, ok := schema.(map[string]any)
	if !ok {
		if schema == false {
			return []violation{{path, "is a value where the schema at " + at + " allows none"}}
		}
		return nil
	}
	if types, ok := typeList(node["type"]); ok && !fitsType(value, types) {
		return []violation{{path, "is " + kind(value) + ", and the schema at " + at + " allows " +
			strings.Join(types, " or ")}}
	}
	var found []violation
	if ref, ok := node["$ref"].(string); ok {
		target, _, err := s.resolve(ref)
		if err != nil {
			return []violation{{path, err.Error()}}
		}
		found = append(found, s.check(value, target, ref, path, member)...)
	}
	if values, ok := node["enum"].([]any); ok && !slices.ContainsFunc(values, func(v any) bool {
		return jsonEqual(v, value)
	}) {
		found = append(found, violation{path, "is " + compact(value) + ", which is not one of " +
			"the values of the enum at " + at + ", " + compact(values)})
	}
	switch value := value.(type) {
	case map[string]any:
		found = append(found, s.checkObject(value, node, at, path, member)...)
	case []any:
		found = append(found, s.checkArray(value, node, at, path)...)
	}
	return append(found, s.checkCombinations(value, node, at, path)...)
}

// checkObject returns the violations of value, an object, against the object keywords of node.
func (s *spec) checkObject(value, node map[string]any, at, path string, member bool,
) []violation {
	var found []violation
	names, _ := stringList(node["required"])
	for _, name := range names {
		if _, ok := value[name]; !ok {
			found = append(found, violation{path, "has no property " + name + ", which the " +
				"schema at " + at + " requires"})
		}
	}
	properties, _ := node["properties"].(map[string]any)
	patterns, _ := node["patternProperties"].(map[string]any)
	additional, hasAdditional := node["additionalProperties"]
	shape := objectShape{properties: properties, patterns: patterns}
	if closesAtHolder(node) {
		merged, err := s.object(node, at)
		if err != nil {
			return append(found, violation{path, err.Error()})
		}
		shape = merged
	}
	for _, name := range slices.Sorted(maps.Keys(value)) {
		where := path + "/" + escape(name)
		listed := false
		if schema, ok := properties[name]; ok {
			listed = true
			found = append(found, s.check(value[name], schema, at+"/properties/"+escape(name),
				where, false)...)
		}
		for _, pattern := range slices.Sorted(maps.Keys(patterns)) {
			if regexp.MustCompile(pattern).MatchString(name) {
				listed = true
				found = append(found, s.check(value[name], patterns[pattern],
					at+"/patternProperties/"+escape(pattern), where, false)...)
			}
		}
		switch {
		case listed:
		case hasAdditional:
			found = append(found, s.check(value[name], additional,
				at+"/additionalProperties", where, false)...)
		case !member && shape.lists() && !shape.listed(name):
			found = append(found, violation{where, "is a property that the schema at " + at +
				" does not list"})
		}
		if nameSchema, ok := node["propertyNames"]; ok {
			for _, v := range s.check(name, nameSchema, at+"/propertyNames", where, false) {
				found = append(found, violation{where, "has a name that " + v.text})
			}
		}
	}
	return found
}

// checkArray returns the violations of value, an array, against the array keywords of node.
func (s *spec) checkArray(value []any, node map[string]any, at, path string) []violation {
	var found []violation
	prefix, _ := node["prefixItems"].([]any)
	for i, item := range value {
		where := path + "/" + strconv.Itoa(i)
		if i < len(prefix) {
			found = append(found, s.check(item, prefix[i], at+"/prefixItems/"+strconv.Itoa(i),
				where, false)...)
		} else if items, ok := node["items"]; ok {
			found = append(found, s.check(item, items, at+"/items", where, false)...)
		}
	}
	return found
}

// checkCombinations returns the violations of value against the allOf, anyOf and oneOf of node.
func (s *spec) checkCombinations(value any, node map[string]any, at, path string) []violation {
	var found []violation
	members, _ := node["allOf"].([]any)
	for i, member := range members {
		found = append(found, s.check(value, member, at+"/allOf/"+strconv.Itoa(i), path,
			closesAtHolder(node))...)
	}
	for _, key := range []string{"anyOf", "oneOf"} {
		branches, ok := node[key].([]any)
		if !ok {
			continue
		}
		var fits int
		var misses []string
		for i, branch := range branches {
			got := s.check(value, branch, at+"/"+key+"/"+strconv.Itoa(i), path, false)
			if len(got) == 0 {
				fits++
				continue
			}
			misses = append(misses, fmt.Sprintf("%d: %s %s", i, pointer(got[0].at), got[0].text))
		}
		switch {
		case fits == 0:
			found = append(found, violation{path, fmt.Sprintf("fits none of the %d alternatives "+
				"of %s at %s; %s", len(branches), key, at, strings.Join(misses, "; "))})
		case key == "oneOf" && fits > 1:
			found = append(found, violation{path, fmt.Sprintf("fits %d of the alternatives of "+
				"oneOf at %s, which must fit one", fits, at)})
		}
	}
	return found
}

// fitsType reports whether value, a JSON value with each number as a json.Number, is of one of
// types: an integer is a number without a fraction, such as 1 or 1.0.
func fitsType(value any, types []string) bool {
	k := kind(value)
	if slices.Contains(types, k) {
		return true
	}
	if k != "number" || !slices.Contains(types, "integer") {
		return false
	}
	number, _ := value.(json.Number)
	r, ok := new(big.Rat).SetString(string(number))
	return ok && r.IsInt()
}

// kind returns the JSON type of value, a JSON value with each number as a json.Number, or a
// description of what else it is.
func kind(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case json.Number:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("a Go %T", value)
	}
}

// jsonEqual reports whether a and b, JSON values with each number as a json.Number, are the
// same value: numbers are equal when their values are, such as 1 and 1.0.
func jsonEqual(a, b any) bool {
	switch a := a.(type) {
	case json.Number:
		b, ok := b.(json.Number)
		if !ok {
			return false
		}
		x, okA := new(big.Rat).SetString(string(a))
		y, okB := new(big.Rat).SetString(string(b))
		return okA && okB && x.Cmp(y) == 0
	case []any:
		b, ok := b.([]any)
		return ok && slices.EqualFunc(a, b, jsonEqual)
	case map[string]any:
		b, ok := b.(map[string]any)
		return ok && maps.EqualFunc(a, b, jsonEqual)
	default:
		return a == b
	}
}

// compact returns value as compact JSON, for a message.
func compact(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

// pointer returns the JSON pointer path as a message shows it: # for the whole value, and #
// before the pointer otherwise, such as #/data/0/key.
func pointer(path string) string {
	return "#" + path
}
