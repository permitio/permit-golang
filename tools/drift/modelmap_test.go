package drift

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/surface"
)

// nullPolicies are the null policies of a field of model-map.yaml: what the API does with null
// for the field in a request. accepts makes the field a models.Optional; n/a is for a type that
// no request holds.
var nullPolicies = []string{"accepts", "rejects", "n/a"}

// modelMap is model-map.yaml.
type modelMap struct {
	// Models are the types of package models that a spec schema describes, by name.
	Models map[string]modelRow `yaml:"models"`

	// Blocks are the types of package models that no schema describes, which the models use for
	// their fields, each with what it stands for.
	Blocks map[string]string `yaml:"blocks"`
}

// modelRow is a row of models in model-map.yaml.
type modelRow struct {
	// Spec is the spec whose schema the type maps to, one of specNames; empty for defaultSpec.
	Spec string `yaml:"spec"`

	// Schema is the name of the schema under components.schemas.
	Schema string `yaml:"schema"`

	// Fields are the fields of a struct, by their JSON names.
	Fields map[string]fieldRow `yaml:"fields"`
}

// spec returns the name of the row's spec.
func (r modelRow) spec() string {
	if r.Spec == "" {
		return defaultSpec
	}
	return r.Spec
}

// fieldRow is a field of a row of model-map.yaml.
type fieldRow struct {
	// Required says that the schema requires the field. apisurface reads it, to find a field
	// that turned required.
	Required *bool `yaml:"required"`

	// Null is the field's null policy, one of nullPolicies.
	Null string `yaml:"null_policy"` //nolint:tagliatelle // The file's key.
}

// parseModelMap parses data, a model map, and checks its shape: it maps at least one type; each
// row names a schema and a spec of specNames, and each of its fields has a required flag and a
// null policy; each block says what it stands for; and no type is a row and a block.
func parseModelMap(data []byte) (modelMap, error) {
	var m modelMap
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return modelMap{}, err
	}
	if len(m.Models) == 0 {
		return modelMap{}, errors.New("its models key maps no type")
	}
	var problems []error
	for _, name := range slices.Sorted(maps.Keys(m.Models)) {
		row := m.Models[name]
		if !slices.Contains(specNames, row.spec()) {
			problems = append(problems, fmt.Errorf("models.%s: the spec %q is not one of %s",
				name, row.Spec, strings.Join(specNames, ", ")))
		}
		if row.Schema == "" {
			problems = append(problems, fmt.Errorf("models.%s names no schema", name))
		}
		for _, field := range slices.Sorted(maps.Keys(row.Fields)) {
			f := row.Fields[field]
			if f.Required == nil {
				problems = append(problems, fmt.Errorf("models.%s.fields.%s has no required "+
					"flag", name, field))
			}
			if !slices.Contains(nullPolicies, f.Null) {
				problems = append(problems, fmt.Errorf("models.%s.fields.%s: the null policy %q "+
					"is not one of %s", name, field, f.Null, strings.Join(nullPolicies, ", ")))
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(m.Blocks)) {
		if strings.TrimSpace(m.Blocks[name]) == "" {
			problems = append(problems, fmt.Errorf("blocks.%s does not say what it stands for",
				name))
		}
		if _, ok := m.Models[name]; ok {
			problems = append(problems, fmt.Errorf("%s is under both models and blocks; a type "+
				"that a schema describes is a row of models", name))
		}
	}
	return m, errors.Join(problems...)
}

// allowAreas start the key of each finding, by the check that finds it.
var allowAreas = []string{"model ", "round trip ", "operation ", "reply "}

// allowEntry is an entry of allow.yaml: a finding that the drift tests accept.
type allowEntry struct {
	// Key is the key of the finding, as the failing test prints it.
	Key string `yaml:"key"`

	// Reason says why the finding is accepted, such as that the published spec does not list a
	// field that the API returns.
	Reason string `yaml:"reason"`

	// Trigger says when the entry can go, such as when the published spec lists the field.
	Trigger string `yaml:"trigger"`
}

// parseAllowList parses data, an allowlist, and checks each entry: its key starts with one of
// allowAreas and no other entry has it, and it has a reason and a trigger.
func parseAllowList(data []byte) ([]allowEntry, error) {
	var list struct {
		Allow []allowEntry `yaml:"allow"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&list); err != nil {
		return nil, err
	}
	var problems []error
	seen := map[string]bool{}
	for i, entry := range list.Allow {
		label := "entry " + strconv.Itoa(i+1) + ", " + strconv.Quote(entry.Key)
		if !slices.ContainsFunc(allowAreas, func(area string) bool {
			return strings.HasPrefix(entry.Key, area) && len(entry.Key) > len(area)
		}) {
			problems = append(problems, fmt.Errorf("%s: a key starts with one of %q, as the "+
				"failing test prints it", label, allowAreas))
		}
		if seen[entry.Key] {
			problems = append(problems, fmt.Errorf("%s: another entry has the key", label))
		}
		seen[entry.Key] = true
		if strings.TrimSpace(entry.Reason) == "" || strings.TrimSpace(entry.Trigger) == "" {
			problems = append(problems, fmt.Errorf("%s: give it a reason and a trigger", label))
		}
	}
	return list.Allow, errors.Join(problems...)
}

// finding is a difference that a drift check found.
type finding struct {
	// key names the finding for allow.yaml, such as "model UserRead field email"; empty for a
	// finding that no entry may accept, which the SDK's own files must fix.
	key string

	// text says what differs, and what to do.
	text string
}

// String returns the finding as a test prints it: its key, when it has one, and its text.
func (f finding) String() string {
	if f.key == "" {
		return f.text
	}
	return f.key + ": " + f.text
}

// accept returns the findings that no entry of entries accepts, and the entries whose keys
// start with area that accept none of findings: the stale ones. A finding without a key is
// never accepted.
func accept(findings []finding, entries []allowEntry, area string) ([]finding, []allowEntry) {
	used := map[string]bool{}
	var open []finding
	for _, f := range findings {
		if f.key != "" && slices.ContainsFunc(entries, func(e allowEntry) bool {
			return e.Key == f.key
		}) {
			used[f.key] = true
			continue
		}
		open = append(open, f)
	}
	var stale []allowEntry
	for _, entry := range entries {
		if strings.HasPrefix(entry.Key, area) && !used[entry.Key] {
			stale = append(stale, entry)
		}
	}
	return open, stale
}

// sentinel returns the findings of the counts: every type of exported, the exported types of
// package models, is a row of models or of blocks of m, and every row is one of exported; and
// types, the types that the drift tests reflect on, are the rows of models.
func sentinel(exported []string, m modelMap, types map[string]reflect.Type) []finding {
	var found []finding
	for _, name := range exported {
		_, mapped := m.Models[name]
		_, block := m.Blocks[name]
		if !mapped && !block {
			found = append(found, finding{text: "the type " + name + " of package models has " +
				"no row in model-map.yaml: map it to its schema under models, or list it under " +
				"blocks with what it stands for"})
		}
	}
	for _, name := range slices.Sorted(maps.Keys(m.Models)) {
		if !slices.Contains(exported, name) {
			found = append(found, finding{text: "model-map.yaml maps " + name + ", which " +
				"package models does not export: remove its row"})
		}
		if _, ok := types[name]; !ok {
			found = append(found, finding{text: "model-map.yaml maps " + name + ", which " +
				"modelTypes in tools/drift/types_test.go does not hold: add it there"})
		}
	}
	for _, name := range slices.Sorted(maps.Keys(m.Blocks)) {
		if !slices.Contains(exported, name) {
			found = append(found, finding{text: "model-map.yaml lists the block " + name +
				", which package models does not export: remove it"})
		}
	}
	for _, name := range slices.Sorted(maps.Keys(types)) {
		if _, ok := m.Models[name]; !ok {
			found = append(found, finding{text: "modelTypes in tools/drift/types_test.go holds " +
				name + ", which model-map.yaml does not map: map it, or remove it there"})
		}
	}
	return found
}

// exportedTypes returns the names of the types among items, the exported declarations of a
// package, sorted.
func exportedTypes(items []surface.Item) []string {
	var names []string
	for _, item := range items {
		if item.Kind == surface.Type {
			names = append(names, item.Name)
		}
	}
	slices.Sort(names)
	return names
}

// constLine matches the line of a typed string constant of surface.Item, such as
// const AttributeTypeBool AttributeType = "bool".
var constLine = regexp.MustCompile(`^const (\w+) (\w+) = ("(?:[^"\\]|\\.)*")$`)

// enumConstants returns the values of the typed string constants among items, by the name of
// their type and then by their own name.
func enumConstants(items []surface.Item) map[string]map[string]string {
	constants := map[string]map[string]string{}
	for _, item := range items {
		m := constLine.FindStringSubmatch(item.Line)
		if item.Kind != surface.Const || m == nil {
			continue
		}
		value, err := strconv.Unquote(m[3])
		if err != nil {
			continue
		}
		if constants[m[2]] == nil {
			constants[m[2]] = map[string]string{}
		}
		constants[m[2]][m[1]] = value
	}
	return constants
}

// typesByName returns the types of values by their names.
func typesByName(values []any) map[string]reflect.Type {
	types := map[string]reflect.Type{}
	for _, v := range values {
		t := reflect.TypeOf(v)
		types[t.Name()] = t
	}
	return types
}

// sentModels returns the types of package pkgPath that ops send, by name, with the ID of an
// operation that sends each: each RequestModel that types holds, and each named struct of the
// package that a field of one holds, at any depth, through slices, maps, pointers and
// models.Optional.
func sentModels(ops []rest.Op, types map[string]reflect.Type, pkgPath string) map[string]string {
	sent := map[string]string{}
	var visit func(t reflect.Type, op string)
	visit = func(t reflect.Type, op string) {
		for {
			if inner := optionalOf(t); inner != nil {
				t = inner
				continue
			}
			switch t.Kind() {
			case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
				t = t.Elem()
				continue
			default:
			}
			break
		}
		if t.Kind() != reflect.Struct || t.PkgPath() != pkgPath || t.Name() == "" ||
			sent[t.Name()] != "" {
			return
		}
		sent[t.Name()] = op
		for field := range t.Fields() {
			visit(field.Type, op)
		}
	}
	for _, op := range ops {
		if t, ok := types[op.RequestModel]; ok {
			visit(t, op.ID)
		}
	}
	return sent
}
