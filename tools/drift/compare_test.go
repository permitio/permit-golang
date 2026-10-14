package drift

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/permitio/permit-golang/v2/models"
)

var (
	// timeType is models.Time, a timestamp, a string of format date-time.
	timeType = reflect.TypeFor[models.Time]()

	// attributesType is models.Attributes, any JSON object.
	attributesType = reflect.TypeFor[models.Attributes]()

	// numberType is json.Number, any JSON number.
	numberType = reflect.TypeFor[json.Number]()

	// rawType is json.RawMessage, any JSON value.
	rawType = reflect.TypeFor[json.RawMessage]()

	// modelsPath is the import path of package models.
	modelsPath = timeType.PkgPath()
)

// optionalOf returns T when t is a models.Optional[T], and nil otherwise.
func optionalOf(t reflect.Type) reflect.Type {
	if t.PkgPath() != modelsPath || !strings.HasPrefix(t.Name(), "Optional[") {
		return nil
	}
	get, ok := t.MethodByName("Get")
	if !ok {
		return nil
	}
	return get.Type.Out(0)
}

// comparer compares types with their schemas, as the rows of a model map pair them.
type comparer struct {
	// specs are the specs by name.
	specs map[string]*spec

	// rows are the rows of models of the model map.
	rows map[string]modelRow

	// types are the types that rows map, by name.
	types map[string]reflect.Type

	// constants are the values of the constants of each enum type, by the type's name and then
	// by the constant's.
	constants map[string]map[string]string

	// pkgPath is the import path of the package of the types: package models, or the tests' own
	// for their fixtures.
	pkgPath string

	// sent are the types that an operation sends, with the ID of an operation that sends each.
	sent map[string]string
}

// model returns the findings of the type name against its row and schema. Its error is an
// *unsupportedError when the schema holds what the walker does not implement.
func (c *comparer) model(name string) ([]finding, error) {
	row := c.rows[name]
	t, ok := c.types[name]
	if !ok {
		return []finding{{text: "the drift tests have no type " + name + " to compare"}}, nil
	}
	s := c.specs[row.spec()]
	at := schemaPrefix + escape(row.Schema)
	schema, ok := s.schema(row.Schema)
	if !ok {
		return []finding{{key: "model " + name, text: "the spec " + s.name + " has no schema " +
			row.Schema + ", which model-map.yaml maps " + name + " to"}}, nil
	}
	if err := s.audit(schema, at); err != nil {
		return nil, err
	}
	switch t.Kind() {
	case reflect.Struct:
		return c.structModel(name, t, s, at)
	case reflect.String:
		return c.enumModel(name, s, at)
	default:
		return []finding{{text: name + " is a " + t.Kind().String() + "; only a struct or a " +
			"string enum maps to a schema"}}, nil
	}
}

// goField is a field of a struct, as encoding/json reads and writes it.
type goField struct {
	// name is the field's Go name.
	name string

	// json is the field's name in JSON.
	json string

	// typ is the field's type.
	typ reflect.Type

	// omitzero and omitempty are the options of its json tag.
	omitzero, omitempty bool
}

// jsonFields returns the fields of the struct t by their JSON names, with those of the structs
// that it embeds without a name, and a finding for each field without a json tag and each JSON
// name that two fields have.
func jsonFields(t reflect.Type) (map[string]goField, []finding) {
	fields := map[string]goField{}
	var found []finding
	for field := range t.Fields() {
		tag, hasTag := field.Tag.Lookup("json")
		name, options, _ := strings.Cut(tag, ",")
		embedded := field.Type
		if embedded.Kind() == reflect.Pointer {
			embedded = embedded.Elem()
		}
		switch {
		case tag == "-":
			continue
		case field.Anonymous && name == "" && embedded.Kind() == reflect.Struct:
			more, problems := jsonFields(embedded)
			found = append(found, problems...)
			for jsonName, f := range more {
				if _, ok := fields[jsonName]; ok {
					found = append(found, finding{text: t.Name() + ": two fields have the JSON " +
						"name " + jsonName})
				}
				fields[jsonName] = f
			}
			continue
		case !field.IsExported():
			continue
		case !hasTag || name == "":
			found = append(found, finding{text: t.Name() + "." + field.Name + " has no JSON " +
				"name: tag it json:\"<name>\" with the name of its property"})
			continue
		}
		if _, ok := fields[name]; ok {
			found = append(found, finding{text: t.Name() + ": two fields have the JSON name " +
				name})
		}
		opts := strings.Split(options, ",")
		fields[name] = goField{
			name: field.Name, json: name, typ: field.Type,
			omitzero:  slices.Contains(opts, "omitzero"),
			omitempty: slices.Contains(opts, "omitempty"),
		}
	}
	return fields, found
}

// structModel returns the findings of the struct name, of type t, against its schema in s, which
// at locates: each property of the schema has a field and each field a property, each field has
// its row in the model map, the tags and the types of the fields follow their rows, the required
// flags of the rows are the schema's, and the type of each field holds the values of its
// property.
func (c *comparer) structModel(name string, t reflect.Type, s *spec, at string) ([]finding, error) {
	row := c.rows[name]
	schema, _ := s.schema(row.Schema)
	alts, err := s.alternatives(schema, at)
	if err != nil {
		return nil, err
	}
	if len(alts) != 1 || typeOf(alts[0].schema) != "object" {
		return []finding{{key: "model " + name, text: "the schema " + row.Schema + " is not " +
			"one object, which a struct maps to"}}, nil
	}
	shape, err := s.object(alts[0].schema, at)
	if err != nil {
		return nil, err
	}
	fields, found := jsonFields(t)
	names := slices.Concat(slices.Collect(maps.Keys(shape.properties)),
		slices.Collect(maps.Keys(fields)), slices.Collect(maps.Keys(row.Fields)))
	slices.Sort(names)
	for _, jsonName := range slices.Compact(names) {
		key := "model " + name + " field " + jsonName
		f, inGo := fields[jsonName]
		property, inSpec := shape.properties[jsonName]
		r, inMap := row.Fields[jsonName]
		switch {
		case inGo && !inSpec:
			found = append(found, finding{key, "the field " + f.name + " is not a property of " +
				"the schema " + row.Schema + ": fix its tag, or remove it"})
		case inSpec && !inGo:
			found = append(found, finding{key, "the schema " + row.Schema + " has the property " +
				jsonName + ", which no field of " + name + " holds: add the field"})
		}
		switch {
		case inGo && !inMap:
			found = append(found, finding{text: name + "." + f.name + " has no row under " +
				"models." + name + ".fields." + jsonName + " in model-map.yaml: add it"})
		case inMap && !inGo:
			found = append(found, finding{text: "model-map.yaml has the row models." + name +
				".fields." + jsonName + ", which no field of " + name + " has: remove it"})
		case inGo:
			found = append(found, c.fieldRules(name, f, r)...)
		}
		if inSpec && inMap && r.Required != nil && *r.Required != shape.required[jsonName] {
			requires := "requires"
			if !shape.required[jsonName] {
				requires = "does not require"
			}
			found = append(found, finding{key, fmt.Sprintf("the schema %s %s %s, and "+
				"model-map.yaml has required: %t", row.Schema, requires, jsonName, *r.Required)})
		}
		if inGo && inSpec {
			problems, err := c.matchType(f.typ, property, at+"/properties/"+escape(jsonName), s)
			if err != nil {
				return nil, err
			}
			for _, p := range problems {
				found = append(found, finding{key, "the field " + f.name + ": " + p})
			}
		}
	}
	return found, nil
}

// fieldRules returns the findings of the field f of the struct name against its row r: a field
// that the API accepts null for is a models.Optional, and no other; an Optional is tagged
// omitzero, a required field that is not one has no omitzero, an optional field has omitzero,
// and none has omitempty; and a field of a type that an operation sends has the null policy
// accepts or rejects.
func (c *comparer) fieldRules(name string, f goField, r fieldRow) []finding {
	var found []finding
	add := func(format string, args ...any) {
		found = append(found, finding{text: name + "." + f.name + ": " + fmt.Sprintf(format,
			args...)})
	}
	optional := optionalOf(f.typ) != nil
	switch {
	case optional && r.Null != "accepts":
		add("it is a models.Optional, and its null policy is %s; only a field whose policy is "+
			"accepts is an Optional", r.Null)
	case !optional && r.Null == "accepts":
		add("its null policy is accepts, so it is a models.Optional, tagged omitzero")
	}
	required := r.Required != nil && *r.Required
	switch {
	case f.omitempty:
		add("it is tagged omitempty; tag it omitzero, which leaves out only the zero value")
	case optional && !f.omitzero:
		add("it is a models.Optional, so it is tagged omitzero, which leaves it out when unset")
	case !optional && required && f.omitzero:
		add("it is required, so it is not tagged omitzero: the body always holds it")
	case !optional && !required && !f.omitzero:
		add("it is not required, so it is tagged omitzero: the body leaves it out when unset")
	}
	if op := c.sent[name]; op != "" && r.Null == "n/a" {
		add("the operation %s sends %s, so its null policy is accepts or rejects, as the API "+
			"does with null for it; n/a is for a type that no request holds", op, name)
	}
	return found
}

// enumModel returns the findings of the enum name against its schema in s, which at locates:
// each value of the schema has a constant of the type, and each constant is a value of the
// schema.
func (c *comparer) enumModel(name string, s *spec, at string) ([]finding, error) {
	row := c.rows[name]
	schema, _ := s.schema(row.Schema)
	alts, err := s.alternatives(schema, at)
	if err != nil {
		return nil, err
	}
	var values []string
	if len(alts) == 1 && typeOf(alts[0].schema) == "string" {
		values, _ = stringList(alts[0].schema["enum"])
	}
	if len(values) == 0 {
		return []finding{{key: "model " + name, text: "the schema " + row.Schema + " is not an " +
			"enum of strings, which a string type maps to"}}, nil
	}
	var found []finding
	if len(row.Fields) > 0 {
		found = append(found, finding{text: "model-map.yaml gives the enum " + name + " fields; " +
			"remove them"})
	}
	constants := c.constants[name]
	for _, value := range values {
		if !slices.Contains(slices.Collect(maps.Values(constants)), value) {
			found = append(found, finding{"model " + name + " value " + value, "the schema " +
				row.Schema + " has the value " + value + ", which no constant of " + name +
				" holds: add one"})
		}
	}
	for _, constant := range slices.Sorted(maps.Keys(constants)) {
		if value := constants[constant]; !slices.Contains(values, value) {
			found = append(found, finding{"model " + name + " value " + value, "the constant " +
				constant + " of " + name + " is " + value + ", which is not a value of the " +
				"schema " + row.Schema})
		}
	}
	return found, nil
}

// matchType returns a problem for each alternative of schema, which at locates in s, that a value
// of type t cannot hold, and for a type t that holds values that schema does not allow. A
// models.Optional or a pointer is compared as its value; the null that a schema allows is left to
// the null policy.
func (c *comparer) matchType(t reflect.Type, schema any, at string, s *spec) ([]string, error) {
	if inner := optionalOf(t); inner != nil {
		t = inner
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	alts, err := s.alternatives(schema, at)
	if err != nil {
		return nil, err
	}
	alts = slices.DeleteFunc(alts, func(alt alternative) bool {
		return typeOf(alt.schema) == "null"
	})
	if len(alts) == 0 {
		return []string{"the schema at " + at + " allows only null"}, nil
	}
	if t.Kind() == reflect.Interface || t == rawType {
		if len(alts) == 1 && typeOf(alts[0].schema) != "" {
			problem := "its type " + t.String() + " holds any JSON value, and the schema at " +
				at + " allows only " + describe(alts[0]) + ": give it a type of that kind"
			return []string{problem}, nil
		}
		return nil, nil
	}
	var problems []string
	for _, alt := range alts {
		more, err := c.matchAlternative(t, alt, at, s)
		if err != nil {
			return nil, err
		}
		problems = append(problems, more...)
	}
	return problems, nil
}

// matchAlternative returns the problems of type t, which is not an interface, against alt, an
// alternative of the schema that at locates in s.
func (c *comparer) matchAlternative(t reflect.Type, alt alternative, at string, s *spec,
) ([]string, error) {
	typ := typeOf(alt.schema)
	format, _ := alt.schema["format"].(string)
	mismatch := []string{"its type " + t.String() + " cannot hold " + describe(alt) +
		", which the schema at " + at + " allows"}
	row, mapped := c.rows[t.Name()]
	switch {
	case typ == "":
		return mismatch, nil
	case t == timeType:
		if typ != "string" || format != "date-time" {
			return mismatch, nil
		}
		return nil, nil
	case t == attributesType:
		_, properties := alt.schema["properties"]
		_, patterns := alt.schema["patternProperties"]
		additional, ok := alt.schema["additionalProperties"]
		if typ != "object" || properties || patterns ||
			ok && additional != true && !reflect.DeepEqual(additional, map[string]any{}) {
			return mismatch, nil
		}
		return nil, nil
	case t.PkgPath() == c.pkgPath && t.Name() != "" && mapped:
		if alt.name != row.Schema || row.spec() != s.name {
			return []string{"its type " + t.Name() + " maps to the schema " + row.Schema +
				" of " + row.spec() + ", and the schema at " + at + " allows " + describe(alt) +
				" of " + s.name}, nil
		}
		return nil, nil
	case t.PkgPath() == c.pkgPath && t.Name() != "":
		return []string{"its type " + t.Name() + " has no row under models in model-map.yaml"},
			nil
	}
	if !slices.Contains(jsonTypesOf(t), typ) {
		return mismatch, nil
	}
	switch t.Kind() {
	case reflect.String:
		return matchString(t, alt, at), nil
	case reflect.Slice, reflect.Array:
		if _, ok := alt.schema["prefixItems"]; ok {
			return mismatch, nil
		}
		items, ok := alt.schema["items"]
		if !ok {
			items = true
		}
		return c.matchType(t.Elem(), items, at+"/items", s)
	case reflect.Map:
		return c.matchMap(t, alt, at, s)
	case reflect.Struct:
		return []string{"its type " + t.String() + " is a struct that package models does not " +
			"map; use a type of package models, or models.Time for a timestamp"}, nil
	default:
		return nil, nil
	}
}

// matchString returns the problems of t, a string type that package models does not map, against
// alt, a string alternative of the schema that at locates: an enum of components.schemas maps to
// the enum type of package models whose row names it, and a timestamp to models.Time.
func matchString(t reflect.Type, alt alternative, at string) []string {
	if _, ok := alt.schema["enum"]; ok && alt.name != "" {
		return []string{"its type " + t.String() + " is not the enum type that maps to the " +
			"schema " + alt.name + " at " + at + ": use the type of package models whose row " +
			"names it"}
	}
	if format, _ := alt.schema["format"].(string); format == "date-time" {
		return []string{"its type " + t.String() + " is not models.Time, and the schema at " +
			at + " is a timestamp, a string of format date-time"}
	}
	return nil
}

// matchMap returns the problems of t, a map type, against alt, an object alternative of the schema
// that at locates in s: a map has string keys, its schema lists no property, and the type of its
// values holds the values of its additionalProperties, or of its one patternProperties.
func (c *comparer) matchMap(t reflect.Type, alt alternative, at string, s *spec,
) ([]string, error) {
	if t.Key().Kind() != reflect.String {
		return []string{"its type " + t.String() + " has keys that are not strings"}, nil
	}
	if _, ok := alt.schema["properties"]; ok {
		return []string{"its type " + t.String() + " is a map, and the schema at " + at +
			" lists properties, which a struct of package models holds"}, nil
	}
	values, ok := alt.schema["additionalProperties"]
	where := at + "/additionalProperties"
	patterns, _ := alt.schema["patternProperties"].(map[string]any)
	switch {
	case len(patterns) > 1:
		return []string{"its type " + t.String() + " is a map of one type of value, and the " +
			"schema at " + at + " has more than one patternProperties"}, nil
	case len(patterns) == 1:
		for pattern, schema := range patterns {
			values, where = schema, at+"/patternProperties/"+escape(pattern)
		}
	case !ok:
		values = true
	}
	return c.matchType(t.Elem(), values, where, s)
}

// jsonTypesOf returns the JSON types that a value of t, a type that is not an interface, can
// decode from.
func jsonTypesOf(t reflect.Type) []string {
	if t == numberType {
		return []string{"number", "integer"}
	}
	switch t.Kind() {
	case reflect.String:
		return []string{"string"}
	case reflect.Bool:
		return []string{"boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return []string{"integer"}
	case reflect.Float32, reflect.Float64:
		return []string{"number"}
	case reflect.Slice, reflect.Array:
		return []string{"array"}
	case reflect.Map, reflect.Struct:
		return []string{"object"}
	default:
		return nil
	}
}

// describe returns alt as a message names it, such as the schema UserRead, a string of format
// uuid or an array.
func describe(alt alternative) string {
	if alt.name != "" {
		return "the schema " + alt.name
	}
	typ := typeOf(alt.schema)
	format, _ := alt.schema["format"].(string)
	switch {
	case typ == "":
		return "any JSON value"
	case format != "":
		return "a " + typ + " of format " + format
	case typ == "array" || typ == "object" || typ == "integer":
		return "an " + typ
	default:
		return "a " + typ
	}
}
