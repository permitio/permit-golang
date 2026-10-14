package drift

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strconv"

	"github.com/permitio/permit-golang/v2/internal/wire"
)

// unknownValue is a value that no enum knows and a field name that no model has, as a value or
// a field that the API adds later is.
const unknownValue = "added_later"

// formatSamples are the strings that the generator writes for a string of each format, values
// that every model reads and writes back unchanged.
var formatSamples = map[string]string{
	"date-time": "2026-10-14T09:30:00Z",
	"email":     "user@example.com",
	"uri":       "https://example.com/path",
	"uuid":      "00000000-0000-4000-8000-000000000001",
}

// errCycle is the error of a schema that holds itself where the value cannot leave it out.
var errCycle = errors.New("the schema holds itself")

// maxValues is the most maximal values that the generator writes for one schema. A schema that
// needs more for every option of every choice to be in one of them is one that the walker does
// not implement.
const maxValues = 256

// generator writes the maximal values of a schema: every property of each object, each item
// of a prefixItems and one item of items for each array, and at each choice, an alternative or a
// value of an enum, the next option of that choice. Strings, numbers and booleans are never their
// zero value, so a field tagged omitzero keeps them.
type generator struct {
	// spec is the spec that holds the schemas.
	spec *spec

	// choices are the choices that the values written so far met, by site: where the choice is
	// in the value, with the alternative picked at each choice above it.
	choices map[string]*choice

	// left is the number of options of the choices met that no value written so far picked.
	left int

	// unknown writes unknownValue in place of each value of an enum of strings.
	unknown bool

	// open holds the names of the schemas that the value being written is in.
	open map[string]bool
}

// choice is a choice of the values that a generator writes, at one site.
type choice struct {
	// next is the number of times a value picked an option of the choice: the next value picks
	// option next modulo the number of options.
	next int

	// picked says which options a value picked.
	picked []bool
}

// maximal returns the maximal values of the schema that the $ref ref points at, which audit
// accepted: as many as it takes for every option of every choice that they meet, each
// alternative and each value of an enum, to be in one of them, where a choice under another is
// a choice of its own for each alternative above it. With unknown, each value of an enum of
// strings is unknownValue instead. A schema that needs more than maxValues values is an
// *unsupportedError.
func (s *spec) maximal(ref string, unknown bool) ([]any, error) {
	g := &generator{
		spec: s, choices: map[string]*choice{}, unknown: unknown, open: map[string]bool{},
	}
	var values []any
	for len(values) == 0 || g.left > 0 {
		if len(values) == maxValues {
			return nil, unsupported(ref, fmt.Sprintf("the generator stops after %d maximal "+
				"values, and the number of options of their choices that none of them holds is %d",
				maxValues, g.left))
		}
		v, err := g.value(map[string]any{"$ref": ref}, ref, "")
		if err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, nil
}

// pick returns the option of the choice of n options at site that the value being written
// picks: each option in turn.
func (g *generator) pick(site string, n int) int {
	if n < 2 {
		return 0
	}
	c, ok := g.choices[site]
	if !ok {
		c = &choice{picked: make([]bool, n)}
		g.choices[site] = c
		g.left += n
	}
	option := c.next % n
	c.next++
	if !c.picked[option] {
		c.picked[option] = true
		g.left--
	}
	return option
}

// value returns the maximal value of schema, which at locates, and whose choices are at site.
func (g *generator) value(schema any, at, site string) (any, error) {
	alts, err := g.spec.alternatives(schema, at)
	if err != nil {
		return nil, err
	}
	if len(alts) == 0 {
		return nil, unsupported(at, "the schema allows no value")
	}
	option := g.pick(site, len(alts))
	alt := alts[option]
	if len(alts) > 1 {
		site += "|" + strconv.Itoa(option)
	}
	if alt.name != "" {
		if g.open[alt.name] {
			return nil, fmt.Errorf("%s: %w", at, errCycle)
		}
		g.open[alt.name] = true
		defer delete(g.open, alt.name)
	}
	if values, ok := alt.schema["enum"].([]any); ok {
		if g.unknown && typeOf(alt.schema) == "string" {
			return unknownValue, nil
		}
		return values[g.pick(site, len(values))], nil
	}
	switch typeOf(alt.schema) {
	case "null":
		return nil, nil //nolint:nilnil // nil is the JSON value null.
	case "boolean":
		return true, nil
	case "integer":
		return json.Number("1"), nil
	case "number":
		return json.Number("1.5"), nil
	case "array":
		return g.array(alt.schema, at, site)
	case "object":
		return g.object(alt.schema, at, site)
	default:
		format, _ := alt.schema["format"].(string)
		if sample, ok := formatSamples[format]; ok {
			return sample, nil
		}
		return "value", nil
	}
}

// array returns the maximal array of node, which at locates and whose choices are at site: an
// item for each schema of its prefixItems, then one item of its items. An item that would hold
// the array again is left out.
func (g *generator) array(node map[string]any, at, site string) (any, error) {
	items := []any{}
	prefix, _ := node["prefixItems"].([]any)
	for i, schema := range prefix {
		where := "/prefixItems/" + strconv.Itoa(i)
		item, err := g.value(schema, at+where, site+where)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if schema, ok := node["items"]; ok && schema != false {
		item, err := g.value(schema, at+"/items", site+"/items")
		switch {
		case errors.Is(err, errCycle):
		case err != nil:
			return nil, err
		default:
			items = append(items, item)
		}
	}
	return items, nil
}

// object returns the maximal object of node, which at locates and whose choices are at site:
// every property, one more for each of its patternProperties, and, when it lists no property, one
// more for its additionalProperties, a string when it has none. A property that would hold the
// object again is left out when it is not required.
func (g *generator) object(node map[string]any, at, site string) (any, error) {
	shape, err := g.spec.object(node, at)
	if err != nil {
		return nil, err
	}
	object := map[string]any{}
	for _, name := range slices.Sorted(maps.Keys(shape.properties)) {
		where := "/properties/" + escape(name)
		v, err := g.value(shape.properties[name], at+where, site+where)
		switch {
		case errors.Is(err, errCycle) && !shape.required[name]:
		case err != nil:
			return nil, err
		default:
			object[name] = v
		}
	}
	for i, pattern := range slices.Sorted(maps.Keys(shape.patterns)) {
		key := "key-" + strconv.Itoa(i+1)
		if !regexp.MustCompile(pattern).MatchString(key) {
			return nil, unsupported(at, fmt.Sprintf("the generator writes the key %q, which the "+
				"pattern %q does not match", key, pattern))
		}
		where := "/patternProperties/" + escape(pattern)
		v, err := g.value(shape.patterns[pattern], at+where, site+where)
		if err != nil {
			return nil, err
		}
		object[key] = v
	}
	switch additional := shape.additional.(type) {
	case map[string]any:
		if !shape.lists() {
			v, err := g.value(additional, at+"/additionalProperties",
				site+"/additionalProperties")
			if err != nil {
				return nil, err
			}
			object["key"] = v
		}
	case nil:
		if !shape.lists() {
			object["key"] = "value"
		}
	}
	return object, nil
}

// strictDecode decodes data, one JSON value, into v as the drift test's strict decoder does:
// with each number that v holds in an any as a json.Number, and failing on a field that v does
// not have.
func strictDecode(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	return dec.Decode(v)
}

// mismatch is a place where a value differs from what it should be.
type mismatch struct {
	// path is the JSON pointer of the place, such as /data/0/key; empty for the whole value.
	path string

	// text says how it differs.
	text string
}

// roundTrip encodes value as JSON, decodes it into a new value of type t with decode, encodes
// that with wire.Encode, the SDK's encoder, and compares the result with want. value and want
// are JSON values with each number as a json.Number, as the generator writes them. A property
// that is null in want may be left out of the result when its field of t is not a
// models.Optional, as withoutPlainNulls says. It returns each place where the result differs
// from want, or one for the whole value that says what failed; none when the result is want.
func roundTrip(t reflect.Type, value, want any, decode func([]byte, any) error) []mismatch {
	data, err := json.Marshal(value)
	if err != nil {
		return []mismatch{{"", "encoding the value: " + err.Error()}}
	}
	model := reflect.New(t).Interface()
	if err = decode(data, model); err != nil {
		return []mismatch{{"", fmt.Sprintf("%s does not decode into %s: %v", data, t, err)}}
	}
	encoded, err := wire.Encode(model)
	if err != nil {
		return []mismatch{{"", fmt.Sprintf("%s decodes into %s, which does not encode: %v",
			data, t, err)}}
	}
	var got any
	if err = wire.Decode(encoded, &got); err != nil {
		return []mismatch{{"", fmt.Sprintf("%s encodes as %s, which does not decode: %v", t,
			encoded, err)}}
	}
	found := differences(withoutPlainNulls(t, want), withoutPlainNulls(t, got), "")
	for i, m := range found {
		found[i].text = fmt.Sprintf("%s decodes into %s and encodes back as %s: %s", data, t,
			encoded, m.text)
	}
	return found
}

// withoutPlainNulls returns value, a JSON value of type t, without each property, at any depth,
// that is null and whose field is not a models.Optional. The null that a schema allows for such
// a field is left to its null policy, as matchType does: it decodes into the zero value, which
// the field leaves out when it is tagged omitzero. An Optional keeps null, which it sends.
func withoutPlainNulls(t reflect.Type, value any) any {
	if inner := optionalOf(t); inner != nil {
		t = inner
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch v := value.(type) {
	case map[string]any:
		var fields map[string]goField
		if t.Kind() == reflect.Struct {
			fields, _ = jsonFields(t)
		}
		object := make(map[string]any, len(v))
		for name, item := range v {
			switch f, ok := fields[name]; {
			case t.Kind() == reflect.Map:
				object[name] = withoutPlainNulls(t.Elem(), item)
			case !ok:
				object[name] = item
			case item != nil || optionalOf(f.typ) != nil:
				object[name] = withoutPlainNulls(f.typ, item)
			}
		}
		return object
	case []any:
		if t.Kind() != reflect.Slice && t.Kind() != reflect.Array {
			return value
		}
		items := make([]any, 0, len(v))
		for _, item := range v {
			items = append(items, withoutPlainNulls(t.Elem(), item))
		}
		return items
	default:
		return value
	}
}

// differences returns each place below the JSON pointer path where want and got, JSON values
// with each number as a json.Number, differ, in the order of their pointers; none when they are
// the same.
func differences(want, got any, path string) []mismatch {
	wantObject, okWant := want.(map[string]any)
	gotObject, okGot := got.(map[string]any)
	if okWant && okGot {
		names := slices.Collect(maps.Keys(wantObject))
		for name := range gotObject {
			if _, ok := wantObject[name]; !ok {
				names = append(names, name)
			}
		}
		slices.Sort(names)
		var found []mismatch
		for _, name := range names {
			at := path + "/" + escape(name)
			w, inWant := wantObject[name]
			g, inGot := gotObject[name]
			switch {
			case !inGot:
				found = append(found, mismatch{at, pointer(at) + " is missing"})
			case !inWant:
				found = append(found, mismatch{at, pointer(at) + " is new: " + compact(g)})
			default:
				found = append(found, differences(w, g, at)...)
			}
		}
		return found
	}
	wantArray, okWant := want.([]any)
	gotArray, okGot := got.([]any)
	if okWant && okGot && len(wantArray) == len(gotArray) {
		var found []mismatch
		for i := range wantArray {
			found = append(found, differences(wantArray[i], gotArray[i],
				path+"/"+strconv.Itoa(i))...)
		}
		return found
	}
	if reflect.DeepEqual(want, got) {
		return nil
	}
	return []mismatch{{path, fmt.Sprintf("%s is %s, want %s", pointer(path), compact(got),
		compact(want))}}
}

// roundTripFindings returns the findings of the strict round trip of t, the type that name
// maps to the schema that at locates in s: each maximal value of the schema decodes into t with the
// strict decoder and encodes back unchanged; with an unknown value in place of each value of an
// enum, and with an unknown field, each decodes with the SDK's tolerant decoder, wire.Decode, and
// encodes back unchanged, without the unknown field. A value has a finding for each place where
// it differs, whose key is round trip, name and the place's JSON pointer, such as
// round trip UserRead#/email, or # when the value does not decode or encode. Its error is an
// *unsupportedError when the schema holds what the walker does not implement.
func roundTripFindings(name string, t reflect.Type, s *spec, at string) ([]finding, error) {
	schema, _, err := s.resolve(at)
	if err != nil {
		return nil, err
	}
	if err = s.audit(schema, at); err != nil {
		return nil, err
	}
	key := "round trip " + name
	var found []finding
	values, err := s.maximal(at, false)
	if err != nil {
		return nil, err
	}
	for i, value := range values {
		for _, m := range roundTrip(t, value, value, strictDecode) {
			found = append(found, finding{key + pointer(m.path), fmt.Sprintf("maximal value "+
				"%d of %d: %s", i+1, len(values), m.text)})
		}
	}
	unknown, err := s.maximal(at, true)
	if err != nil {
		return nil, err
	}
	for i, value := range unknown {
		for _, m := range roundTrip(t, value, value, wire.Decode) {
			found = append(found, finding{key + pointer(m.path), fmt.Sprintf("maximal value "+
				"%d of %d with an unknown value of each enum: %s", i+1, len(unknown), m.text)})
		}
	}
	if object, ok := values[0].(map[string]any); ok {
		withField := maps.Clone(object)
		withField[unknownValue] = map[string]any{"a": []any{json.Number("1")}}
		for _, m := range roundTrip(t, withField, object, wire.Decode) {
			found = append(found, finding{key + pointer(m.path), "the maximal value with an " +
				"unknown field: " + m.text})
		}
	}
	return found, nil
}
