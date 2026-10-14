package drift

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/models"
)

// The types below are models of the comparer's tests, each against the fixture spec's schema
// that model-map.yaml would map it to, and changed from it as its comment says.

// ScopeRenamed is Scope with the JSON tag of OrganizationID renamed.
type ScopeRenamed struct {
	OrganizationID string `json:"organization_idd"`
	ProjectID      string `json:"project_id,omitzero"`
}

// ScopeUpdate is Scope as an update model, with ProjectID an Optional.
type ScopeUpdate struct {
	OrganizationID string                  `json:"organization_id"`
	ProjectID      models.Optional[string] `json:"project_id,omitzero"`
}

// ScopeReadOptional is Scope as a read model, with ProjectID an Optional.
type ScopeReadOptional struct {
	OrganizationID string                  `json:"organization_id"`
	ProjectID      models.Optional[string] `json:"project_id,omitzero"`
}

// ScopeTags is Scope with the tags of its fields changed as the tests need.
type ScopeTags struct {
	OrganizationID string                  `json:"organization_id,omitzero"`
	ProjectID      models.Optional[string] `json:"project_id"`
}

// ScopeOmitempty is Scope with ProjectID tagged omitempty.
type ScopeOmitempty struct {
	OrganizationID string `json:"organization_id"`
	ProjectID      string `json:"project_id,omitempty"`
}

// ScopeRequiredOptional is Scope with OrganizationID an Optional that the body requires.
type ScopeRequiredOptional struct {
	OrganizationID models.Optional[string] `json:"organization_id,omitzero"`
	ProjectID      string                  `json:"project_id,omitzero"`
}

// Embedded is a struct that ScopeEmbedded embeds.
type Embedded struct {
	ProjectID string `json:"project_id,omitzero"`
}

// ScopeEmbedded is Scope with ProjectID from an embedded struct.
type ScopeEmbedded struct {
	*Embedded

	OrganizationID string `json:"organization_id"`
	Ignored        string `json:"-"`
	unexported     string //nolint:unused // A field that jsonFields leaves out, as the test checks.
}

// Fields is a struct whose JSON names the tests read.
type Fields struct {
	Embedded

	Untagged string
	OnlyOpts string `json:",omitzero"` //nolint:tagliatelle // No JSON name, which the test reads.
	Other    struct {
		ProjectID string `json:"project_id"`
	}
}

// Count is a type that is not a struct or a string.
type Count int

// field returns the row of a field of the comparer's tests.
func field(required bool, policy string) fieldRow {
	return fieldRow{Required: &required, Null: policy}
}

// scopeFields are the rows of the fields of Scope, and of the types that differ from it.
func scopeFields() map[string]fieldRow {
	return map[string]fieldRow{
		"organization_id": field(true, "n/a"),
		"project_id":      field(false, "n/a"),
	}
}

// fixtureRows are the rows of the comparer's tests, by type.
func fixtureRows() map[string]modelRow {
	return map[string]modelRow{
		"Color":             {Schema: "Color"},
		"Count":             {Schema: "Scope"},
		"Scope":             {Schema: "Scope", Fields: scopeFields()},
		"ScopeRenamed":      {Schema: "Scope", Fields: scopeFields()},
		"ScopeWithNumbers":  {Schema: "Scope", Fields: scopeFields()},
		"ScopeEmbedded":     {Schema: "Scope", Fields: scopeFields()},
		"ScopeReadOptional": {Schema: "Scope", Fields: scopeFields()},
		"ScopeWithoutProject": {Schema: "Scope", Fields: map[string]fieldRow{
			"organization_id": field(true, "n/a"),
		}},
		"ScopeUpdate": {Schema: "Scope", Fields: map[string]fieldRow{
			"organization_id": field(true, "rejects"),
			"project_id":      field(false, "rejects"),
		}},
		"ScopeRequiredOptional": {Schema: "Scope", Fields: map[string]fieldRow{
			"organization_id": field(true, "accepts"),
			"project_id":      field(false, "rejects"),
		}},
		"ScopeTags": {Schema: "Scope", Fields: map[string]fieldRow{
			"organization_id": field(true, "accepts"),
			"project_id":      field(true, "accepts"),
		}},
		"ScopeOmitempty": {Schema: "Scope", Fields: map[string]fieldRow{
			"organization_id": field(false, "n/a"),
			"project_id":      field(false, "n/a"),
		}},
		"Thing": {Schema: "Thing", Fields: map[string]fieldRow{
			"attributes": field(false, "n/a"),
			"audience":   field(false, "n/a"),
			"color":      field(false, "n/a"),
			"count":      field(true, "n/a"),
			"created_at": field(false, "n/a"),
			"enabled":    field(false, "n/a"),
			"key":        field(true, "n/a"),
			"labels":     field(false, "n/a"),
			"note":       field(false, "accepts"),
			"parent_id":  field(false, "n/a"),
			"ratio":      field(false, "n/a"),
			"roles":      field(false, "n/a"),
			"scope":      field(false, "n/a"),
			"tags":       field(false, "n/a"),
		}},
		"Node": {Schema: "Node", Fields: map[string]fieldRow{
			"children": field(false, "n/a"),
			"key":      field(true, "n/a"),
			"parent":   field(false, "n/a"),
		}},
		"Missing":     {Schema: "Missing"},
		"Odd":         {Schema: "Odd"},
		"NotAnEnum":   {Schema: "Scope"},
		"NotAnObject": {Schema: "Color"},
		"Unlisted":    {Schema: "Scope"},
	}
}

// Missing, Odd, NotAnEnum and NotAnObject are types whose rows the tests break.
type (
	Missing     struct{}
	Odd         struct{}
	NotAnEnum   string
	NotAnObject struct{}
)

// fixtureComparer returns the comparer of the tests, with the fixture spec as control-plane,
// the rows of fixtureRows, the types of this file and generate_cases_test.go, and the constants
// of Color.
func fixtureComparer(t *testing.T) *comparer {
	t.Helper()
	cloud := inlineSpec(t, "pdp-cloud", `{"Scope": {"type": "object"}}`)
	return &comparer{
		specs: map[string]*spec{defaultSpec: fixtureSpec(t), "pdp-cloud": cloud},
		rows:  fixtureRows(),
		types: typesByName([]any{
			Color(""), Count(0),
			Scope{},
			ScopeRenamed{},
			ScopeWithoutProject{},
			ScopeWithNumbers{},
			ScopeUpdate{},
			ScopeReadOptional{},
			ScopeTags{},
			ScopeOmitempty{},
			ScopeEmbedded{},
			ScopeRequiredOptional{},
			Thing{},
			Node{},
			Missing{},
			Odd{},
			NotAnEnum(""),
			NotAnObject{},
		}),
		constants: map[string]map[string]string{
			"Color": {"ColorRed": "red", "ColorGreen": "green", "ColorBlue": "blue"},
		},
		pkgPaths: []string{reflect.TypeFor[Scope]().PkgPath()},
		sent:     map[string]string{},
	}
}

// findingTexts returns the findings as the tests print them.
func findingTexts(found []finding) []string {
	texts := make([]string, 0, len(found))
	for _, f := range found {
		texts = append(texts, f.String())
	}
	return texts
}

func TestCompareModel(t *testing.T) {
	scope := "#/components/schemas/Scope/properties/"
	for _, tc := range []struct {
		name  string
		model string
		sent  string
		want  []string
	}{
		{"a model that fits", "Scope", "", nil},
		{"a struct with nested models, enums, maps, unions and a cycle", "Thing", "", nil},
		{"a recursive model", "Node", "", nil},
		{"an enum that fits", "Color", "", nil},
		{"a renamed JSON tag", "ScopeRenamed", "", []string{
			"model ScopeRenamed field organization_id: the schema Scope has the property " +
				"organization_id, which no field of ScopeRenamed holds: add the field",
			"model-map.yaml has the row models.ScopeRenamed.fields.organization_id, which no " +
				"field of ScopeRenamed has: remove it",
			"model ScopeRenamed field organization_idd: the field OrganizationID is not a " +
				"property of the schema Scope: fix its tag, or remove it",
			"ScopeRenamed.OrganizationID has no row under " +
				"models.ScopeRenamed.fields.organization_idd in model-map.yaml: add it",
		}},
		{"a property without a field", "ScopeWithoutProject", "", []string{
			"model ScopeWithoutProject field project_id: the schema Scope has the property " +
				"project_id, which no field of ScopeWithoutProject holds: add the field",
		}},
		{"fields of another kind", "ScopeWithNumbers", "", []string{
			"model ScopeWithNumbers field organization_id: the field OrganizationID: its type " +
				"int cannot hold a string of format uuid, which the schema at " + scope +
				"organization_id allows",
			"model ScopeWithNumbers field project_id: the field ProjectID: its type int cannot " +
				"hold a string of format uuid, which the schema at " + scope + "project_id allows",
		}},
		{"an Optional whose policy is rejects", "ScopeUpdate", "", []string{
			"ScopeUpdate.ProjectID: it is a models.Optional, and its null policy is rejects; " +
				"only a field whose policy is accepts is an Optional",
		}},
		{"an Optional of a model that no operation sends", "ScopeReadOptional", "", []string{
			"ScopeReadOptional.ProjectID: it is a models.Optional, and its null policy is n/a; " +
				"only a field whose policy is accepts is an Optional",
		}},
		{"tags and policies against the rows", "ScopeTags", "", []string{
			"ScopeTags.OrganizationID: its null policy is accepts, so it is a models.Optional, " +
				"tagged omitzero",
			"ScopeTags.OrganizationID: it is required, so it is not tagged omitzero: the body " +
				"always holds it",
			"ScopeTags.ProjectID: it is a models.Optional, so it is tagged omitzero, which " +
				"leaves it out when unset",
			"model ScopeTags field project_id: the schema Scope does not require project_id, " +
				"and model-map.yaml has required: true",
		}},
		{"omitempty and a required flag", "ScopeOmitempty", "", []string{
			"ScopeOmitempty.OrganizationID: it is not required, so it is tagged omitzero: the " +
				"body leaves it out when unset",
			"model ScopeOmitempty field organization_id: the schema Scope requires " +
				"organization_id, and model-map.yaml has required: false",
			"ScopeOmitempty.ProjectID: it is tagged omitempty; tag it omitzero, which leaves " +
				"out only the zero value",
		}},
		{"a field from an embedded struct", "ScopeEmbedded", "", nil},
		{
			"a required Optional of a model that an operation sends", "ScopeRequiredOptional",
			"create_env", nil,
		},
		{"n/a in a model that an operation sends", "Scope", "create_env", []string{
			"Scope.OrganizationID: the operation create_env sends Scope, so its null policy is " +
				"accepts, rejects or ignores, as the API does with null for it; n/a is for a " +
				"type that no request holds",
			"Scope.ProjectID: the operation create_env sends Scope, so its null policy is " +
				"accepts, rejects or ignores, as the API does with null for it; n/a is for a " +
				"type that no request holds",
		}},
		{"a value missing from the enum", "NotAnEnum", "", []string{
			"model NotAnEnum: the schema Scope is not an enum of strings, which a string type " +
				"maps to",
		}},
		{"a struct mapped to an enum", "NotAnObject", "", []string{
			"model NotAnObject: the schema Color is not one object, which a struct maps to",
		}},
		{"a schema the spec does not have", "Missing", "", []string{
			"model Missing: the spec control-plane has no schema Missing, which model-map.yaml " +
				"maps Missing to",
		}},
		{"a type that is neither a struct nor a string", "Count", "", []string{
			"Count is a int; only a struct or a string enum maps to a schema",
		}},
		{"a row without a type", "Unlisted", "", []string{
			"the drift tests have no type Unlisted to compare",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fixtureComparer(t)
			if tc.sent != "" {
				c.sent[tc.model] = tc.sent
			}
			found, err := c.model(tc.model)
			if err != nil {
				t.Fatal(err)
			}
			if got := findingTexts(found); !slices.Equal(got, tc.want) {
				t.Errorf("model(%s) =\n%s\nwant\n%s", tc.model, strings.Join(got, "\n"),
					strings.Join(tc.want, "\n"))
			}
		})
	}
}

// TestCompareModelOfTwoSpecs checks that a type is compared with each schema of its row, its own
// and those of also, under keys that name the spec of each schema of also, that the rows of its
// fields are checked once, against its own schema, and that the properties of left_out are
// checked against the type and its schemas.
func TestCompareModelOfTwoSpecs(t *testing.T) {
	cloud := inlineSpec(t, "pdp-cloud", `{
		"CloudScope": {"type": "object", "required": ["organization_id", "project_id"],
			"properties": {"organization_id": {"type": "integer"},
				"project_id": {"type": ["string", "null"]}, "region": {"type": "string"}}},
		"CloudColor": {"enum": ["red", "green", "cyan"], "type": "string"},
		"Odd": {"not": {}}
	}`)
	also := func(schema string) []schemaRef {
		return []schemaRef{{Spec: "pdp-cloud", Schema: schema}}
	}
	for _, tc := range []struct {
		name  string
		model string
		row   modelRow
		want  []string
	}{
		{"a struct", "Scope", modelRow{
			Schema: "Scope", Also: also("CloudScope"), Fields: map[string]fieldRow{
				"organization_id": field(true, "n/a"),
				"project_id":      field(false, "n/a"),
				"zone":            field(false, "n/a"),
			},
		}, []string{
			"model-map.yaml has the row models.Scope.fields.zone, which no field of Scope has: " +
				"remove it",
			"model Scope@pdp-cloud field organization_id: the field OrganizationID: its type " +
				"string cannot hold an integer, which the schema at " +
				"#/components/schemas/CloudScope/properties/organization_id allows",
			"model Scope@pdp-cloud field project_id: the schema CloudScope requires " +
				"project_id, and model-map.yaml has required: false",
			"model Scope@pdp-cloud field region: the schema CloudScope has the property region, " +
				"which no field of Scope holds: add the field",
		}},
		{"a struct with left_out", "Scope", modelRow{
			Schema: "Scope", Also: also("CloudScope"), Fields: scopeFields(),
			LeftOut: map[string]string{"region": "r", "project_id": "r", "gone": "r"},
		}, []string{
			"model Scope@pdp-cloud field organization_id: the field OrganizationID: its type " +
				"string cannot hold an integer, which the schema at " +
				"#/components/schemas/CloudScope/properties/organization_id allows",
			"model Scope@pdp-cloud field project_id: the schema CloudScope requires " +
				"project_id, and model-map.yaml has required: false",
			"model Scope@pdp-cloud field region: the schema CloudScope has the property region, " +
				"which no field of Scope holds: add the field",
			"models.Scope.left_out.gone: no schema of Scope has the property gone: remove it " +
				"from left_out",
			"models.Scope.left_out.project_id: the field ProjectID holds project_id, so the " +
				"type does not leave it out: remove it from left_out",
		}},
		{"a struct mapped to an enum of the second spec", "Scope", modelRow{
			Schema: "Scope", Also: also("CloudColor"), Fields: scopeFields(),
		}, []string{
			"model Scope@pdp-cloud: the schema CloudColor is not one object, which a struct " +
				"maps to",
		}},
		{"a schema that the second spec does not have", "Scope", modelRow{
			Schema: "Scope", Also: also("Gone"), Fields: scopeFields(),
		}, []string{
			"model Scope@pdp-cloud: the spec pdp-cloud has no schema Gone, which model-map.yaml " +
				"maps Scope to",
		}},
		{"an enum", "Color", modelRow{
			Schema: "Color", Also: also("CloudColor"),
			Fields:  map[string]fieldRow{"x": {}},
			LeftOut: map[string]string{"x": "r"},
		}, []string{
			"model-map.yaml gives the enum Color fields; remove them",
			"model Color@pdp-cloud value cyan: the schema CloudColor has the value cyan, which " +
				"no constant of Color holds: add one",
			"model Color@pdp-cloud value blue: the constant ColorBlue of Color is blue, which " +
				"is not a value of the schema CloudColor",
			"models.Color.left_out.x: no schema of Color has the property x: remove it from " +
				"left_out",
		}},
		{"an enum mapped to an object of the second spec", "Color", modelRow{
			Schema: "Color", Also: also("CloudScope"),
		}, []string{
			"model Color@pdp-cloud: the schema CloudScope is not an enum of strings, which a " +
				"string type maps to",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fixtureComparer(t)
			c.specs["pdp-cloud"] = cloud
			c.rows[tc.model] = tc.row
			found, err := c.model(tc.model)
			if got := findingTexts(found); err != nil || !slices.Equal(got, tc.want) {
				t.Errorf("model(%s) =\n%s, %v\nwant\n%s", tc.model, strings.Join(got, "\n"),
					err, strings.Join(tc.want, "\n"))
			}
		})
	}
	c := fixtureComparer(t)
	c.specs["pdp-cloud"] = cloud
	c.rows["Scope"] = modelRow{Schema: "Scope", Also: also("Odd"), Fields: scopeFields()}
	if _, err := c.model("Scope"); !isUnsupported(err) {
		t.Errorf("model of a row whose second schema the walker does not implement fails with "+
			"%v, want an *unsupportedError", err)
	}
}

func TestCompareEnum(t *testing.T) {
	c := fixtureComparer(t)
	c.constants["Color"] = map[string]string{
		"ColorRed": "red", "ColorGreen": "green", "ColorPurple": "purple",
	}
	c.rows["Color"] = modelRow{Schema: "Color", Fields: map[string]fieldRow{"x": {}}}
	found, err := c.model("Color")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"model-map.yaml gives the enum Color fields; remove them",
		"model Color value blue: the schema Color has the value blue, which no constant of " +
			"Color holds: add one",
		"model Color value purple: the constant ColorPurple of Color is purple, which is not a " +
			"value of the schema Color",
	}
	if got := findingTexts(found); !slices.Equal(got, want) {
		t.Errorf("model(Color) =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestCompareModelDidNotRun(t *testing.T) {
	c := fixtureComparer(t)
	_, err := c.model("Odd")
	if !isUnsupported(err) || err.Error() != `#/components/schemas/Odd: the `+
		`keyword "not" is not one the walker implements; add it to keywords in `+
		"tools/drift/schema_test.go" {
		t.Errorf("model(Odd) fails with %v, want the *unsupportedError of not", err)
	}
}

func TestMatchType(t *testing.T) {
	s := inlineSpec(t, defaultSpec, `{
		"Scope": {"type": "object", "properties": {"organization_id": {"type": "string"}}},
		"Color": {"enum": ["red"], "type": "string"},
		"Loop": {"anyOf": [{"$ref": "#/components/schemas/Loop"}]}
	}`)
	c := fixtureComparer(t)
	c.specs[defaultSpec] = s
	c.rows["Scope"] = modelRow{Schema: "Scope"}
	c.rows["Color"] = modelRow{Schema: "Color"}
	c.rows["Remote"] = modelRow{Spec: "pdp-cloud", Schema: "Scope"}
	c.rows["Both"] = modelRow{
		Spec: "pdp-cloud", Schema: "Scope", Also: []schemaRef{{Spec: defaultSpec, Schema: "Scope"}},
	}
	type (
		Remote   struct{}
		Both     struct{}
		Unmapped struct{}
	)
	for _, tc := range []struct {
		name   string
		typ    reflect.Type
		schema string
		want   []string
	}{
		{"a string", reflect.TypeFor[string](), `{"type": "string"}`, nil},
		{"a pointer to a bool", reflect.TypeFor[*bool](), `{"type": "boolean"}`, nil},
		{
			"an Optional of an int64", reflect.TypeFor[models.Optional[int64]](),
			`{"type": ["integer", "null"]}`, nil,
		},
		{"a float64", reflect.TypeFor[float64](), `{"type": "number"}`, nil},
		{
			"a json.Number", reflect.TypeFor[json.Number](),
			`{"anyOf": [{"type": "number"}, {"type": "integer"}]}`, nil,
		},
		{
			"a slice of strings", reflect.TypeFor[[]string](),
			`{"type": "array", "items": {"type": "string"}}`, nil,
		},
		{"an array", reflect.TypeFor[[2]any](), `{"type": "array"}`, nil},
		{
			"a map of strings", reflect.TypeFor[map[string]string](),
			`{"type": "object", "additionalProperties": {"type": "string"}}`, nil,
		},
		{
			"a map of a pattern", reflect.TypeFor[map[string]int](),
			`{"type": "object", "patternProperties": {"^a": {"type": "integer"}}}`, nil,
		},
		{"a map of anything", reflect.TypeFor[map[string]any](), `{"type": "object"}`, nil},
		{
			"Attributes", reflect.TypeFor[models.Attributes](),
			`{"type": "object", "additionalProperties": {}}`, nil,
		},
		{
			"Attributes of an open object", reflect.TypeFor[models.Attributes](),
			`{"type": "object", "additionalProperties": true}`, nil,
		},
		{
			"a Time", reflect.TypeFor[models.Time](), `{"type": "string", "format": "date-time"}`,
			nil,
		},
		{
			"any for a union", reflect.TypeFor[any](),
			`{"anyOf": [{"type": "string"}, {"type": "integer"}]}`, nil,
		},
		{"json.RawMessage for any value", reflect.TypeFor[json.RawMessage](), `{}`, nil},
		{
			"a mapped model", reflect.TypeFor[Scope](), `{"$ref": "#/components/schemas/Scope"}`,
			nil,
		},
		{
			"a mapped enum", reflect.TypeFor[Color](),
			`{"allOf": [{"$ref": "#/components/schemas/Color"}]}`, nil,
		},
		{
			"a string for an inline enum", reflect.TypeFor[string](),
			`{"enum": ["a", "b"], "type": "string"}`, nil,
		},
		{
			"any for one kind", reflect.TypeFor[any](), `{"type": "string", "format": "uuid"}`,
			[]string{"its type interface {} holds any JSON value, and the schema at #/p allows " +
				"only a string of format uuid: give it a type of that kind"},
		},
		{
			"a string for any value", reflect.TypeFor[string](), `{"format": "uuid"}`,
			[]string{"its type string cannot hold any JSON value, which the schema at #/p " +
				"allows"},
		},
		{
			"only null", reflect.TypeFor[string](), `{"type": "null"}`,
			[]string{"the schema at #/p allows only null"},
		},
		{
			"a union of two kinds", reflect.TypeFor[string](),
			`{"anyOf": [{"type": "string"}, {"type": "array", "items": {"type": "string"}}]}`,
			[]string{"its type string cannot hold an array, which the schema at #/p allows"},
		},
		{
			"a float64 for an integer", reflect.TypeFor[float64](), `{"type": "integer"}`,
			[]string{"its type float64 cannot hold an integer, which the schema at #/p allows"},
		},
		{
			"an int for a number", reflect.TypeFor[uint8](), `{"type": "number"}`,
			[]string{"its type uint8 cannot hold a number, which the schema at #/p allows"},
		},
		{
			"a struct of another package", reflect.TypeFor[time.Time](),
			`{"type": "string", "format": "date-time"}`,
			[]string{"its type time.Time cannot hold a string of format date-time, which the " +
				"schema at #/p allows"},
		},
		{
			"a struct of another package for an object", reflect.TypeFor[time.Location](),
			`{"type": "object"}`,
			[]string{"its type time.Location is a struct that package models does not map; " +
				"use a type of package models, or models.Time for a timestamp"},
		},
		{
			"a string for a timestamp", reflect.TypeFor[string](),
			`{"type": "string", "format": "date-time"}`,
			[]string{"its type string is not models.Time, and the schema at #/p is a " +
				"timestamp, a string of format date-time"},
		},
		{
			"a Time for a plain string", reflect.TypeFor[models.Time](), `{"type": "string"}`,
			[]string{"its type models.Time cannot hold a string, which the schema at #/p " +
				"allows"},
		},
		{
			"a string for an enum schema", reflect.TypeFor[string](),
			`{"$ref": "#/components/schemas/Color"}`,
			[]string{"its type string is not the enum type that maps to the schema Color at " +
				"#/p: use the type of package models whose row names it"},
		},
		{
			"Attributes for an object with properties", reflect.TypeFor[models.Attributes](),
			`{"$ref": "#/components/schemas/Scope"}`,
			[]string{"its type models.Attributes cannot hold the schema Scope, which the " +
				"schema at #/p allows"},
		},
		{
			"Attributes for a closed object", reflect.TypeFor[models.Attributes](),
			`{"type": "object", "additionalProperties": {"type": "string"}}`,
			[]string{"its type models.Attributes cannot hold an object, which the schema at " +
				"#/p allows"},
		},
		{
			"Attributes for a string", reflect.TypeFor[models.Attributes](), `{"type": "string"}`,
			[]string{"its type models.Attributes cannot hold a string, which the schema at #/p " +
				"allows"},
		},
		{
			"a model for another schema", reflect.TypeFor[Scope](),
			`{"$ref": "#/components/schemas/Color"}`,
			[]string{"its type Scope maps to the schema Scope of control-plane, and the schema " +
				"at #/p allows the schema Color of control-plane"},
		},
		{
			"a model for an inline object", reflect.TypeFor[Scope](), `{"type": "object"}`,
			[]string{"its type Scope maps to the schema Scope of control-plane, and the schema " +
				"at #/p allows an object of control-plane"},
		},
		{
			"a model of another spec", reflect.TypeFor[Remote](),
			`{"$ref": "#/components/schemas/Scope"}`,
			[]string{"its type Remote maps to the schema Scope of pdp-cloud, and the schema at " +
				"#/p allows the schema Scope of control-plane"},
		},
		{
			"a model of the second spec of its row", reflect.TypeFor[Both](),
			`{"$ref": "#/components/schemas/Scope"}`, nil,
		},
		{
			"a model of two specs for another schema", reflect.TypeFor[Both](),
			`{"$ref": "#/components/schemas/Color"}`,
			[]string{"its type Both maps to the schema Scope of pdp-cloud, or Scope of " +
				"control-plane, and the schema at #/p allows the schema Color of control-plane"},
		},
		{
			"a type of the package without a row", reflect.TypeFor[Unmapped](),
			`{"type": "object"}`,
			[]string{"its type Unmapped has no row under models in model-map.yaml"},
		},
		{
			"a slice of another kind", reflect.TypeFor[[]int](),
			`{"type": "array", "items": {"type": "string"}}`,
			[]string{"its type int cannot hold a string, which the schema at #/p/items allows"},
		},
		{
			"a slice for a tuple", reflect.TypeFor[[]string](),
			`{"type": "array", "prefixItems": [{"type": "string"}]}`,
			[]string{"its type []string cannot hold an array, which the schema at #/p allows"},
		},
		{
			"a slice for an array of anything", reflect.TypeFor[[]string](), `{"type": "array"}`,
			[]string{"its type string cannot hold any JSON value, which the schema at #/p/items " +
				"allows"},
		},
		{
			"a map of another kind", reflect.TypeFor[map[string]int](),
			`{"type": "object", "additionalProperties": {"type": "string"}}`,
			[]string{"its type int cannot hold a string, which the schema at " +
				"#/p/additionalProperties allows"},
		},
		{
			"a map of a pattern of another kind", reflect.TypeFor[map[string]int](),
			`{"type": "object", "patternProperties": {"^a/b": {"type": "string"}}}`,
			[]string{"its type int cannot hold a string, which the schema at " +
				"#/p/patternProperties/^a~1b allows"},
		},
		{
			"a map of two patterns", reflect.TypeFor[map[string]string](),
			`{"type": "object", "patternProperties": {"^a": {}, "^b": {}}}`,
			[]string{"its type map[string]string is a map of one type of value, and the schema " +
				"at #/p has more than one patternProperties"},
		},
		{
			"a map for an object with properties", reflect.TypeFor[map[string]string](),
			`{"type": "object", "properties": {"a": {"type": "string"}}}`,
			[]string{"its type map[string]string is a map, and the schema at #/p lists " +
				"properties, which a struct of package models holds"},
		},
		{
			"a map with keys that are not strings", reflect.TypeFor[map[int]string](),
			`{"type": "object"}`,
			[]string{"its type map[int]string has keys that are not strings"},
		},
		{
			"a map of strings for an open object", reflect.TypeFor[map[string]string](),
			`{"type": "object"}`,
			[]string{"its type string cannot hold any JSON value, which the schema at " +
				"#/p/additionalProperties allows"},
		},
		{
			"a func", reflect.TypeFor[func()](), `{"type": "string"}`,
			[]string{"its type func() cannot hold a string, which the schema at #/p allows"},
		},
	} {
		var schema any
		if err := json.Unmarshal([]byte(tc.schema), &schema); err != nil {
			t.Fatal(err)
		}
		got, err := c.matchType(tc.typ, schema, "#/p", s)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("%s: matchType =\n%q, %v\nwant\n%q", tc.name, got, err, tc.want)
		}
	}
	_, err := c.matchType(reflect.TypeFor[string](), map[string]any{
		"$ref": "#/components/schemas/Loop",
	}, "#/p", s)
	if !isUnsupported(err) {
		t.Errorf("matchType of a schema that is its own alternative fails with %v", err)
	}
}

// TestMatchTypeOfTwoPackages checks that a struct of each package of pkgPaths, as package models
// and the root package are, maps to a schema through its row, and that a struct of another
// package does not.
func TestMatchTypeOfTwoPackages(t *testing.T) {
	s := inlineSpec(t, defaultSpec, `{"Scope": {"type": "object"}}`)
	c := fixtureComparer(t)
	c.specs[defaultSpec] = s
	c.rows["APIKeyScopeRead"] = modelRow{Schema: "Scope"}
	scope := map[string]any{"$ref": "#/components/schemas/Scope"}
	read := reflect.TypeFor[models.APIKeyScopeRead]()
	for _, tc := range []struct {
		name     string
		pkgPaths []string
		mapped   bool
		want     []string
	}{
		{"a package of its own", c.pkgPaths, true, []string{"its type " +
			"models.APIKeyScopeRead is a struct that package models does not map; use a type " +
			"of package models, or models.Time for a timestamp"}},
		{"a second package", append(slices.Clone(c.pkgPaths), modelsPath), true, nil},
		{
			"a second package, without a row", append(slices.Clone(c.pkgPaths), modelsPath),
			false,
			[]string{"its type APIKeyScopeRead has no row under models in model-map.yaml"},
		},
	} {
		c.pkgPaths = tc.pkgPaths
		if !tc.mapped {
			delete(c.rows, "APIKeyScopeRead")
		}
		got, err := c.matchType(read, scope, "#/p", s)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("%s: matchType =\n%q, %v\nwant\n%q", tc.name, got, err, tc.want)
		}
	}
}

func TestJSONFields(t *testing.T) {
	fields, found := jsonFields(reflect.TypeFor[Fields]())
	var names []string
	for name, f := range fields {
		names = append(names, name+" "+f.name)
	}
	slices.Sort(names)
	want := []string{"project_id ProjectID"}
	if !slices.Equal(names, want) {
		t.Errorf("jsonFields(Fields) = %q, want %q", names, want)
	}
	wantFound := []string{
		"Fields.Untagged has no JSON name: tag it json:\"<name>\" with the name of its property",
		"Fields.OnlyOpts has no JSON name: tag it json:\"<name>\" with the name of its property",
		"Fields.Other has no JSON name: tag it json:\"<name>\" with the name of its property",
	}
	if got := findingTexts(found); !slices.Equal(got, wantFound) {
		t.Errorf("jsonFields(Fields) finds\n%s\nwant\n%s", strings.Join(got, "\n"),
			strings.Join(wantFound, "\n"))
	}
	// go vet rejects a struct type with two fields of one JSON name, so these are built here.
	text := reflect.TypeFor[string]()
	for _, tc := range []struct {
		fields []reflect.StructField
		want   string
	}{
		{[]reflect.StructField{
			{Name: "A", Type: text, Tag: `json:"twice"`},
			{Name: "B", Type: text, Tag: `json:"twice,omitzero"`},
		}, ": two fields have the JSON name twice"},
		{[]reflect.StructField{
			{Name: "Embedded", Type: reflect.TypeFor[Embedded](), Anonymous: true},
			{Name: "Project", Type: text, Tag: `json:"project_id"`},
		}, ": two fields have the JSON name project_id"},
		{[]reflect.StructField{
			{Name: "Project", Type: text, Tag: `json:"project_id"`},
			{Name: "Embedded", Type: reflect.TypeFor[Embedded](), Anonymous: true},
		}, ": two fields have the JSON name project_id"},
	} {
		_, found := jsonFields(reflect.StructOf(tc.fields))
		if got := findingTexts(found); !slices.Equal(got, []string{tc.want}) {
			t.Errorf("jsonFields(%v) finds %q, want %q", tc.fields, got, tc.want)
		}
	}
	dash := reflect.StructOf([]reflect.StructField{{Name: "Dash", Type: text, Tag: `json:"-,"`}})
	if dashFields, _ := jsonFields(dash); dashFields["-"].name != "Dash" {
		t.Errorf(`jsonFields reads json:"-," as %v, want the JSON name -`, dashFields)
	}
	fields, _ = jsonFields(reflect.TypeFor[ScopeTags]())
	if f := fields["organization_id"]; !f.omitzero || f.omitempty || f.name != "OrganizationID" ||
		f.typ != reflect.TypeFor[string]() {
		t.Errorf("jsonFields(ScopeTags) reads organization_id as %+v", f)
	}
	if f := fields["project_id"]; f.omitzero || optionalOf(f.typ) != reflect.TypeFor[string]() {
		t.Errorf("jsonFields(ScopeTags) reads project_id as %+v", f)
	}
}

func TestOptionalOf(t *testing.T) {
	for _, tc := range []struct {
		typ, want reflect.Type
	}{
		{reflect.TypeFor[models.Optional[[]int]](), reflect.TypeFor[[]int]()},
		{reflect.TypeFor[models.Time](), nil},
		{reflect.TypeFor[string](), nil},
		{reflect.TypeFor[struct{ Optional int }](), nil},
	} {
		if got := optionalOf(tc.typ); got != tc.want {
			t.Errorf("optionalOf(%s) = %v, want %v", tc.typ, got, tc.want)
		}
	}
}

func TestDescribe(t *testing.T) {
	for _, tc := range []struct {
		alt  alternative
		want string
	}{
		{alternative{map[string]any{"type": "object"}, "Scope"}, "the schema Scope"},
		{alternative{map[string]any{}, ""}, "any JSON value"},
		{
			alternative{map[string]any{"type": "string", "format": "uuid"}, ""},
			"a string of format uuid",
		},
		{alternative{map[string]any{"type": "integer"}, ""}, "an integer"},
		{alternative{map[string]any{"properties": map[string]any{}}, ""}, "an object"},
		{alternative{map[string]any{"type": "boolean"}, ""}, "a boolean"},
	} {
		if got := describe(tc.alt); got != tc.want {
			t.Errorf("describe(%v) = %q, want %q", tc.alt, got, tc.want)
		}
	}
}
