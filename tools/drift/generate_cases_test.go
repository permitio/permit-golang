package drift

import (
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/redact"
	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The types below are the models of the tests' fixture spec, testdata/spec.json: each type
// maps to the schema of its name, with the fields of the schema's properties, or differs from it
// as its comment says.

// Color is the enum Color.
type Color string

// Scope is Scope.
type Scope struct {
	OrganizationID string `json:"organization_id"`
	ProjectID      string `json:"project_id,omitzero"`
}

// ScopeWithoutProject is Scope without the property project_id.
type ScopeWithoutProject struct {
	OrganizationID string `json:"organization_id"`
}

// ScopeWithNumbers is Scope with the IDs as numbers.
type ScopeWithNumbers struct {
	OrganizationID int `json:"organization_id"`
	ProjectID      int `json:"project_id,omitzero"`
}

// ScopeThatLoses is Scope whose ProjectID is not encoded.
type ScopeThatLoses struct {
	OrganizationID string `json:"organization_id"`
	ProjectID      string `json:"project_id,omitzero"`
}

// MarshalJSON leaves out ProjectID.
func (s ScopeThatLoses) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"organization_id": s.OrganizationID})
}

// Thing is Thing.
type Thing struct {
	Attributes models.Attributes       `json:"attributes,omitzero"`
	Audience   any                     `json:"audience,omitzero"`
	Color      Color                   `json:"color,omitzero"`
	Count      int                     `json:"count"`
	CreatedAt  models.Time             `json:"created_at,omitzero"`
	Enabled    bool                    `json:"enabled,omitzero"`
	Key        string                  `json:"key"`
	Labels     map[string]string       `json:"labels,omitzero"`
	Note       models.Optional[string] `json:"note,omitzero"`
	ParentID   string                  `json:"parent_id,omitzero"`
	Ratio      float64                 `json:"ratio,omitzero"`
	Roles      map[string]string       `json:"roles,omitzero"`
	Scope      Scope                   `json:"scope,omitzero"`
	Tags       []string                `json:"tags,omitzero"`
}

// unencodable is a type that encoding/json cannot encode.
type unencodable struct {
	C chan int `json:"c"`
}

// Node is Node.
type Node struct {
	Children []Node `json:"children,omitzero"`
	Key      string `json:"key"`
	Parent   *Node  `json:"parent,omitzero"`
}

// The types below are Note, whose property note allows null, each with a field of another kind.

// NoteOptional holds note in an Optional, which keeps null.
type NoteOptional struct {
	Note models.Optional[string] `json:"note,omitzero"`
}

// NotePlain holds note in a string tagged omitzero, which leaves null out.
type NotePlain struct {
	Note string `json:"note,omitzero"`
}

// NotePointer holds note in a pointer without omitzero, which keeps null.
type NotePointer struct {
	Note *string `json:"note"`
}

// NoteRequired holds note in a string without omitzero, which writes null back as "".
type NoteRequired struct {
	Note string `json:"note"`
}

// NoteThatLoses holds note in an Optional, and does not encode it.
type NoteThatLoses struct {
	Note models.Optional[string] `json:"note,omitzero"`
}

// MarshalJSON leaves out Note.
func (NoteThatLoses) MarshalJSON() ([]byte, error) {
	return []byte("{}"), nil
}

// The types below are Key, whose property secret is a credential.

// KeySecret holds secret in a models.Secret, which encodes as the marker.
type KeySecret struct {
	Secret models.Secret `json:"secret,omitzero"`
}

// KeyPlain holds secret in a string, which encodes the credential as it is.
type KeyPlain struct {
	Secret string `json:"secret"`
}

// Keys holds KeySecret below other types, and KeyPlain.
type Keys struct {
	ByKey map[string]KeySecret                `json:"by_key"`
	List  []KeySecret                         `json:"list"`
	One   *KeySecret                          `json:"one"`
	Opt   models.Optional[KeySecret]          `json:"opt,omitzero"`
	Plain KeyPlain                            `json:"plain"`
	Pairs [1]map[string]models.Optional[bool] `json:"pairs"`
}

// Notes holds NotePlain below other types.
type Notes struct {
	Any   any                                `json:"any"`
	ByKey map[string]NotePlain               `json:"by_key"`
	List  []NotePlain                        `json:"list"`
	One   *NotePlain                         `json:"one"`
	Opt   models.Optional[NotePlain]         `json:"opt,omitzero"`
	Pairs [1]map[string]models.Optional[int] `json:"pairs"`
}

func TestMaximal(t *testing.T) {
	fixture := fixtureSpec(t)
	nested := inlineSpec(t, "pdp-cloud", `{
		"Kind": {"type": "string", "enum": ["k0", "k1", "k2", "k3"]},
		"Holder": {"type": "object", "properties": {
			"kind": {"anyOf": [{"$ref": "#/components/schemas/Kind"}, {"type": "null"}]}}},
		"Inner": {"type": "object", "properties": {
			"p": {"anyOf": [{"type": "string"}, {"type": "integer"}]}}},
		"Outer": {"type": "object", "properties": {
			"inner": {"anyOf": [{"$ref": "#/components/schemas/Inner"}, {"type": "null"}]}}},
		"Evaluations": {"type": "object", "properties": {
			"action": {"oneOf": [{"type": "null"}, {"$ref": "#/components/schemas/Action"}]}}},
		"Action": {"type": "object", "required": ["name"], "properties": {
			"name": {"type": "string"},
			"properties": {"additionalProperties": {}, "propertyNames": {"type": "string"},
				"type": ["object", "null"]}}},
		"Either": {"type": "object", "properties": {
			"u": {"anyOf": [{"$ref": "#/components/schemas/Two"},
				{"$ref": "#/components/schemas/Three"}]}}},
		"Two": {"type": "object", "properties": {"x": {"enum": ["a", "b"]}}},
		"Three": {"type": "object", "properties": {"x": {"enum": ["c", "d", "e"]}}},
		"Tuple": {"type": "array", "prefixItems": [{"enum": ["a", "b"]}],
			"items": {"enum": ["c", "d", "e"]}}
	}`)
	for _, tc := range []struct {
		spec    *spec
		schema  string
		unknown bool
		want    []string
	}{
		{fixture, "Scope", false, []string{`{"organization_id":` +
			`"00000000-0000-4000-8000-000000000001",` +
			`"project_id":"00000000-0000-4000-8000-000000000001"}`}},
		{fixture, "Color", false, []string{`"red"`, `"green"`, `"blue"`}},
		{fixture, "Color", true, []string{`"added_later"`}},
		{fixture, "Node", false, []string{`{"children":[],"key":"value"}`}},
		{fixture, "PaginatedResult_EnvRead_", false, []string{`{"data":[{"id":` +
			`"00000000-0000-4000-8000-000000000001","key":"value"}],"page_count":1,` +
			`"total_count":1}`}},
		{nested, "Holder", false, []string{
			`{"kind":"k0"}`, `{"kind":null}`, `{"kind":"k1"}`, `{"kind":null}`, `{"kind":"k2"}`,
			`{"kind":null}`, `{"kind":"k3"}`,
		}},
		{nested, "Holder", true, []string{`{"kind":"added_later"}`, `{"kind":null}`}},
		{nested, "Outer", false, []string{
			`{"inner":{"p":"value"}}`, `{"inner":null}`, `{"inner":{"p":1}}`,
		}},
		{nested, "Evaluations", false, []string{
			`{"action":null}`, `{"action":{"name":"value","properties":{"key":"value"}}}`,
			`{"action":null}`, `{"action":{"name":"value","properties":null}}`,
		}},
		{nested, "Either", false, []string{
			`{"u":{"x":"a"}}`, `{"u":{"x":"c"}}`, `{"u":{"x":"b"}}`, `{"u":{"x":"d"}}`,
			`{"u":{"x":"a"}}`, `{"u":{"x":"e"}}`,
		}},
		{nested, "Tuple", false, []string{`["a","c"]`, `["b","d"]`, `["a","e"]`}},
		{fixture, "Thing", false, []string{
			`{"attributes":{"key":"value"},"audience":"value","color":"red","count":1,` +
				`"created_at":"2026-10-14T09:30:00Z","enabled":true,"key":"value",` +
				`"labels":{"key":"value"},"note":"value","parent_id":"value","ratio":1.5,` +
				`"roles":{"key-1":"value"},"scope":{"organization_id":` +
				`"00000000-0000-4000-8000-000000000001","project_id":` +
				`"00000000-0000-4000-8000-000000000001"},"tags":["value"]}`,
			`{"attributes":{"key":"value"},"audience":["value"],"color":"green","count":1,` +
				`"created_at":"2026-10-14T09:30:00Z","enabled":true,"key":"value",` +
				`"labels":{"key":"value"},"note":null,"parent_id":` +
				`"00000000-0000-4000-8000-000000000001","ratio":1.5,"roles":{"key-1":"value"},` +
				`"scope":{"organization_id":"00000000-0000-4000-8000-000000000001","project_id":` +
				`"00000000-0000-4000-8000-000000000001"},"tags":["value"]}`,
			`{"attributes":{"key":"value"},"audience":"value","color":"blue","count":1,` +
				`"created_at":"2026-10-14T09:30:00Z","enabled":true,"key":"value",` +
				`"labels":{"key":"value"},"note":"value","parent_id":"value","ratio":1.5,` +
				`"roles":{"key-1":"value"},"scope":{"organization_id":` +
				`"00000000-0000-4000-8000-000000000001","project_id":` +
				`"00000000-0000-4000-8000-000000000001"},"tags":["value"]}`,
		}},
	} {
		values, err := tc.spec.maximal(schemaPrefix+tc.schema, tc.unknown, nil)
		if err != nil {
			t.Fatalf("maximal(%s): %v", tc.schema, err)
		}
		got := make([]string, 0, len(values))
		for _, v := range values {
			got = append(got, compact(v))
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("maximal(%s, unknown %t) =\n%s\nwant\n%s", tc.schema, tc.unknown,
				strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
		}
		for i, v := range values {
			found := tc.spec.validate(v, schemaOf(t, tc.spec, tc.schema), "S")
			if !tc.unknown && found != nil {
				t.Errorf("maximal value %d of %s does not fit it: %v", i, tc.schema, found)
			}
		}
	}
}

func TestMaximalKinds(t *testing.T) {
	s := inlineSpec(t, defaultSpec, `{
		"Kinds": {"type": "object", "properties": {"b": {"type": "boolean"},
			"i": {"type": "integer"},
			"n": {"type": "number"}, "z": {"type": "null"}, "s": {"type": "string"},
			"e": {"format": "email", "type": "string"}, "u": {"format": "uri", "type": "string"},
			"x": {"format": "binary", "type": "string"}, "a": {}, "nums": {"enum": [1, 2]},
			"pair": {"prefixItems": [{"type": "integer"}, {"type": "boolean"}], "items": false},
			"closed": {"type": "object", "additionalProperties": false},
			"tree": {"$ref": "#/components/schemas/Tree"}}},
		"Tree": {"type": "object", "properties": {"kids": {"type": "array",
			"items": {"$ref": "#/components/schemas/Tree"}}}, "required": ["kids"]}
	}`)
	values, err := s.maximal(schemaPrefix+"Kinds", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`{"a":"value","b":true,"closed":{},"e":"user@example.com","i":1,"n":1.5,"nums":1,` +
			`"pair":[1,true],"s":"value","tree":{"kids":[]},"u":"https://example.com/path",` +
			`"x":"value","z":null}`,
		`{"a":"value","b":true,"closed":{},"e":"user@example.com","i":1,"n":1.5,"nums":2,` +
			`"pair":[1,true],"s":"value","tree":{"kids":[]},"u":"https://example.com/path",` +
			`"x":"value","z":null}`,
	}
	got := []string{compact(values[0]), compact(values[1])}
	if len(values) != 2 || !slices.Equal(got, want) {
		t.Errorf(
			"maximal(Kinds) =\n%s\nwant\n%s",
			strings.Join(got, "\n"),
			strings.Join(want, "\n"),
		)
	}
}

func TestMaximalFails(t *testing.T) {
	s := inlineSpec(t, defaultSpec, `{
		"Self": {"type": "object", "properties": {"self": {"$ref": "#/components/schemas/Self"}},
			"required": ["self"]},
		"Holder": {"type": "object", "properties": {"self": {"$ref": "#/components/schemas/Self"}}},
		"Nothing": {"type": "object", "properties": {"never": false}},
		"Keys": {"type": "object", "patternProperties": {"^[0-9]+$": {"type": "string"}}},
		"Lost": {"type": "object", "properties": {"a": {"$ref": "#/components/schemas/Gone"}}},
		"LostItem": {"type": "array", "items": {"$ref": "#/components/schemas/Gone"}},
		"LostPrefix": {"type": "array", "prefixItems": [{"$ref": "#/components/schemas/Gone"}]},
		"LostKey": {"type": "object",
			"patternProperties": {"^key": {"$ref": "#/components/schemas/Gone"}}},
		"LostValue": {"type": "object",
			"additionalProperties": {"$ref": "#/components/schemas/Gone"}},
		"Clash": {"allOf": [{"properties": {"a": {"type": "string"}}},
			{"properties": {"a": {"type": "integer"}}}]}
	}`)
	gone := "#/components/schemas/Gone: the $ref points at nothing in the spec"
	for _, tc := range []struct{ schema, want string }{
		{"Self", "#/components/schemas/Self/properties/self: the schema holds itself"},
		{"Nothing", "#/components/schemas/Nothing/properties/never: the schema allows no value"},
		{"Keys", `#/components/schemas/Keys: the generator writes the key "key-1", which the ` +
			`pattern "^[0-9]+$" does not match`},
		{"Lost", gone},
		{"LostItem", gone},
		{"LostPrefix", gone},
		{"LostKey", gone},
		{"LostValue", gone},
		{"Clash", `#/components/schemas/Clash/allOf/1: the properties "a" has two schemas in an ` +
			"allOf, which the walker does not merge"},
	} {
		_, err := s.maximal(schemaPrefix+tc.schema, false, nil)
		if err == nil || err.Error() != tc.want {
			t.Errorf("maximal(%s) fails with\n%v\nwant\n%s", tc.schema, err, tc.want)
		}
	}
	values, err := s.maximal(schemaPrefix+"Holder", false, nil)
	if err != nil || len(values) != 1 || compact(values[0]) != "{}" {
		t.Errorf("maximal(Holder) = %v, %v; want {}, which leaves out the schema that requires "+
			"itself", values, err)
	}
	options := make([]string, maxValues+1)
	for i := range options {
		options[i] = strconv.Itoa(i)
	}
	most := inlineSpec(t, defaultSpec, `{
		"Most": {"enum": [`+strings.Join(options[:maxValues], ", ")+`]},
		"TooMany": {"enum": [`+strings.Join(options, ", ")+`]}
	}`)
	values, err = most.maximal(schemaPrefix+"Most", false, nil)
	if err != nil || len(values) != maxValues {
		t.Errorf("maximal(Most) = %d values, %v; want %d", len(values), err, maxValues)
	}
	_, err = most.maximal(schemaPrefix+"TooMany", false, nil)
	want := "#/components/schemas/TooMany: the generator stops after 256 maximal values, and the " +
		"number of options of their choices that none of them holds is 1"
	if !isUnsupported(err) || err.Error() != want {
		t.Errorf("maximal(TooMany) fails with\n%v\nwant\n%s", err, want)
	}
}

func TestRoundTrip(t *testing.T) {
	scope := map[string]any{"organization_id": "o", "project_id": "p"}
	note := map[string]any{"note": nil}
	for _, tc := range []struct {
		name   string
		typ    reflect.Type
		value  any
		decode func([]byte, any) error
		want   []mismatch
	}{
		{"a type that holds the value", reflect.TypeFor[Scope](), scope, strictDecode, nil},
		{
			"a type without a property", reflect.TypeFor[ScopeWithoutProject](), scope,
			strictDecode,
			[]mismatch{{"", `{"organization_id":"o","project_id":"p"} does not ` +
				`decode into drift.ScopeWithoutProject: json: unknown field "project_id"`}},
		},
		{
			"the tolerant decoder", reflect.TypeFor[ScopeWithoutProject](), scope, wire.Decode,
			[]mismatch{{"/project_id", `{"organization_id":"o","project_id":"p"} decodes into ` +
				`drift.ScopeWithoutProject and encodes back as {"organization_id":"o"}: ` +
				"#/project_id is missing"}},
		},
		{
			"a type that loses every field", reflect.TypeFor[Missing](), scope, wire.Decode,
			[]mismatch{
				{"/organization_id", `{"organization_id":"o","project_id":"p"} decodes into ` +
					"drift.Missing and encodes back as {}: #/organization_id is missing"},
				{"/project_id", `{"organization_id":"o","project_id":"p"} decodes into ` +
					"drift.Missing and encodes back as {}: #/project_id is missing"},
			},
		},
		{
			"a type of another kind", reflect.TypeFor[ScopeWithNumbers](), scope, strictDecode,
			[]mismatch{{"", `{"organization_id":"o","project_id":"p"} does not decode into ` +
				"drift.ScopeWithNumbers: json: cannot unmarshal string into Go struct field " +
				"ScopeWithNumbers.organization_id of type int"}},
		},
		{
			"a type that loses a field", reflect.TypeFor[ScopeThatLoses](), scope, strictDecode,
			[]mismatch{{"/project_id", `{"organization_id":"o","project_id":"p"} decodes into ` +
				`drift.ScopeThatLoses and encodes back as {"organization_id":"o"}: ` +
				"#/project_id is missing"}},
		},
		{
			"a value that does not encode", reflect.TypeFor[Scope](), func() {}, strictDecode,
			[]mismatch{{"", "encoding the value: json: unsupported type: func()"}},
		},
		{
			"a value that does not encode back", reflect.TypeFor[unencodable](),
			map[string]any{},
			strictDecode,
			[]mismatch{{"", "{} decodes into " +
				"drift.unencodable, which does not encode: json: unsupported type: chan int"}},
		},
		{"null in an Optional", reflect.TypeFor[NoteOptional](), note, strictDecode, nil},
		{"null left out by a string", reflect.TypeFor[NotePlain](), note, strictDecode, nil},
		{"null kept by a pointer", reflect.TypeFor[NotePointer](), note, strictDecode, nil},
		{
			"null written back as the zero value", reflect.TypeFor[NoteRequired](), note,
			strictDecode,
			[]mismatch{{"/note", `{"note":null} decodes into drift.NoteRequired ` +
				`and encodes back as {"note":""}: #/note is new: ""`}},
		},
		{
			"null that an Optional loses", reflect.TypeFor[NoteThatLoses](), note, strictDecode,
			[]mismatch{{"/note", `{"note":null} decodes into drift.NoteThatLoses and encodes ` +
				"back as {}: #/note is missing"}},
		},
		{
			"nulls of plain fields below other types", reflect.TypeFor[Notes](),
			map[string]any{
				"any": []any{nil, note}, "by_key": map[string]any{"a": note},
				"list": []any{note}, "one": note, "opt": note,
				"pairs": []any{map[string]any{"a": nil}},
			},
			strictDecode, nil,
		},
		{
			"a credential that a Secret holds", reflect.TypeFor[KeySecret](),
			map[string]any{"secret": "s"},
			strictDecode, nil,
		},
		{
			"an empty credential", reflect.TypeFor[KeySecret](),
			map[string]any{"secret": ""},
			strictDecode, nil,
		},
		{
			"null in a Secret", reflect.TypeFor[KeySecret](),
			map[string]any{"secret": nil},
			strictDecode, nil,
		},
		{
			"credentials of Secrets below other types and of a plain string",
			reflect.TypeFor[Keys](),
			map[string]any{
				"by_key": map[string]any{"a": map[string]any{"secret": "s"}},
				"list":   []any{map[string]any{"secret": "s"}},
				"one":    map[string]any{"secret": "s"}, "opt": map[string]any{"secret": "s"},
				"plain": map[string]any{"secret": "s"}, "pairs": []any{map[string]any{"a": true}},
			},
			strictDecode, nil,
		},
		{
			"a credential of a Secret where the value is another kind",
			reflect.TypeFor[KeySecret](),
			map[string]any{"secret": json.Number("1")},
			strictDecode,
			[]mismatch{{"", `{"secret":1} does not decode into drift.KeySecret: models: a ` +
				"Secret is read from a JSON string or null"}},
		},
	} {
		if got := roundTrip(tc.typ, tc.value, tc.value, tc.decode); !slices.Equal(got, tc.want) {
			t.Errorf("%s: roundTrip =\n%q\nwant\n%q", tc.name, got, tc.want)
		}
	}
	marked := markedSecrets(reflect.TypeFor[KeySecret](), map[string]any{"secret": "s"})
	if want := map[string]any{"secret": redact.Marker}; !reflect.DeepEqual(marked, want) {
		t.Errorf("markedSecrets of a credential = %v, want %v", marked, want)
	}
	withField := map[string]any{"organization_id": "o", "project_id": "p", "added_later": true}
	if got := roundTrip(reflect.TypeFor[Scope](), withField, scope, wire.Decode); got != nil {
		t.Errorf("roundTrip of an unknown field with the tolerant decoder = %q", got)
	}
}

func TestDifference(t *testing.T) {
	for _, tc := range []struct {
		want, got string
		diff      []mismatch
	}{
		{`{"a": [1, {"b": "x"}]}`, `{"a": [1, {"b": "x"}]}`, nil},
		{`{"a": 1, "b": 2}`, `{"a": 1}`, []mismatch{{"/b", "#/b is missing"}}},
		{`{"a": 1}`, `{"a": 1, "b/c": 2}`, []mismatch{{"/b~1c", "#/b~1c is new: 2"}}},
		{`{"a": [1, 2]}`, `{"a": [1, 3]}`, []mismatch{{"/a/1", "#/a/1 is 3, want 2"}}},
		{`{"a": [1, 2]}`, `{"a": [1]}`, []mismatch{{"/a", "#/a is [1], want [1,2]"}}},
		{`{"a": {"b": 1}}`, `{"a": {"b": 1.0}}`, []mismatch{{"/a/b", "#/a/b is 1.0, want 1"}}},
		{`{"a": 1}`, `[1]`, []mismatch{{"", `# is [1], want {"a":1}`}}},
		{`"x"`, `"x"`, nil},
		{`{"a": [1, 2], "b": 1, "c": 1}`, `{"a": [0, 2], "c": 2, "d": 1}`, []mismatch{
			{"/a/0", "#/a/0 is 0, want 1"},
			{"/b", "#/b is missing"},
			{"/c", "#/c is 2, want 1"},
			{"/d", "#/d is new: 1"},
		}},
	} {
		got := differences(jsonValue(t, tc.want), jsonValue(t, tc.got), "")
		if !slices.Equal(got, tc.diff) {
			t.Errorf("differences(%s, %s) =\n%q\nwant\n%q", tc.want, tc.got, got, tc.diff)
		}
	}
}

// TestMaximalLeavesOut checks that the maximal values leave out the properties that leftOut gives
// for the schema of components.schemas that lists them, at any depth, and keep a property of that
// name in another schema or an inline one.
func TestMaximalLeavesOut(t *testing.T) {
	s := inlineSpec(t, defaultSpec, `{
		"Query": {"type": "object", "properties": {"sdk": {"type": "string"},
			"user": {"$ref": "#/components/schemas/User"},
			"meta": {"type": "object", "properties": {"sdk": {"type": "string"}}}}},
		"User": {"type": "object", "properties": {"firstName": {"type": "string"},
			"first_name": {"type": "string"}, "sdk": {"type": "string"}}}
	}`)
	values, err := s.maximal(schemaPrefix+"Query", false, map[string][]string{
		"Query": {"sdk"}, "User": {"firstName"},
	})
	want := []any{map[string]any{
		"meta": map[string]any{"sdk": "value"},
		"user": map[string]any{"first_name": "value", "sdk": "value"},
	}}
	if err != nil || !reflect.DeepEqual(values, want) {
		t.Errorf("maximal(Query) = %v, %v; want %v", values, err, want)
	}
	found, err := roundTripFindings("ScopeWithoutProject", reflect.TypeFor[ScopeWithoutProject](),
		fixtureSpec(t), schemaPrefix+"Scope", map[string][]string{"Scope": {"project_id"}})
	if err != nil || found != nil {
		t.Errorf("roundTripFindings of a type without a property that it leaves out = %v, %v; "+
			"want none", findingTexts(found), err)
	}
}

func TestRoundTripFindings(t *testing.T) {
	s := fixtureSpec(t)
	for _, tc := range []struct {
		name, schema string
		typ          reflect.Type
		want         []string
	}{
		{"Scope", "Scope", reflect.TypeFor[Scope](), nil},
		{"Color", "Color", reflect.TypeFor[Color](), nil},
		{"Thing", "Thing", reflect.TypeFor[Thing](), nil},
		{"Node", "Node", reflect.TypeFor[Node](), nil},
		{"NotePlain", "Note", reflect.TypeFor[NotePlain](), nil},
		{"NoteOptional", "Note", reflect.TypeFor[NoteOptional](), nil},
		{"NoteThatLoses", "Note", reflect.TypeFor[NoteThatLoses](), []string{
			`round trip NoteThatLoses#/note: maximal value 1 of 2: {"note":"value"} decodes ` +
				"into drift.NoteThatLoses and encodes back as {}: #/note is missing",
			`round trip NoteThatLoses#/note: maximal value 2 of 2: {"note":null} decodes into ` +
				"drift.NoteThatLoses and encodes back as {}: #/note is missing",
			`round trip NoteThatLoses#/note: maximal value 1 of 2 with an unknown value of ` +
				`each enum: {"note":"value"} decodes into drift.NoteThatLoses and encodes back ` +
				"as {}: #/note is missing",
			`round trip NoteThatLoses#/note: maximal value 2 of 2 with an unknown value of ` +
				`each enum: {"note":null} decodes into drift.NoteThatLoses and encodes back as ` +
				"{}: #/note is missing",
			`round trip NoteThatLoses#/note: the maximal value with an unknown field: ` +
				`{"added_later":{"a":[1]},"note":"value"} decodes into drift.NoteThatLoses and ` +
				"encodes back as {}: #/note is missing",
		}},
		{"ScopeWithoutProject", "Scope", reflect.TypeFor[ScopeWithoutProject](), []string{
			`round trip ScopeWithoutProject#: maximal value 1 of 1: {"organization_id":` +
				`"00000000-0000-4000-8000-000000000001","project_id":` +
				`"00000000-0000-4000-8000-000000000001"} does not decode into ` +
				`drift.ScopeWithoutProject: json: unknown field "project_id"`,
			`round trip ScopeWithoutProject#/project_id: maximal value 1 of 1 with an unknown ` +
				`value of each enum: {"organization_id":"00000000-0000-4000-8000-000000000001",` +
				`"project_id":"00000000-0000-4000-8000-000000000001"} decodes into ` +
				`drift.ScopeWithoutProject and encodes back as {"organization_id":` +
				`"00000000-0000-4000-8000-000000000001"}: #/project_id is missing`,
			`round trip ScopeWithoutProject#/project_id: the maximal value with an unknown ` +
				`field: {"added_later":{"a":[1]},"organization_id":` +
				`"00000000-0000-4000-8000-000000000001","project_id":` +
				`"00000000-0000-4000-8000-000000000001"} decodes into ` +
				`drift.ScopeWithoutProject and encodes back as {"organization_id":` +
				`"00000000-0000-4000-8000-000000000001"}: #/project_id is missing`,
		}},
		{"ColorAsInt", "Color", reflect.TypeFor[int](), []string{
			`round trip ColorAsInt#: maximal value 1 of 3: "red" does not decode into int: ` +
				"json: cannot unmarshal string into Go value of type int",
			`round trip ColorAsInt#: maximal value 2 of 3: "green" does not decode into int: ` +
				"json: cannot unmarshal string into Go value of type int",
			`round trip ColorAsInt#: maximal value 3 of 3: "blue" does not decode into int: ` +
				"json: cannot unmarshal string into Go value of type int",
			`round trip ColorAsInt#: maximal value 1 of 1 with an unknown value of each enum: ` +
				`"added_later" does not decode into int: json: cannot unmarshal string into Go ` +
				"value of type int",
		}},
	} {
		found, err := roundTripFindings(tc.name, tc.typ, s, schemaPrefix+tc.schema, nil)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		var got []string
		for _, f := range found {
			got = append(got, f.String())
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("roundTripFindings(%s) =\n%s\nwant\n%s", tc.name, strings.Join(got, "\n"),
				strings.Join(tc.want, "\n"))
		}
	}
	for _, tc := range []struct{ schema, want string }{
		{"Gone", "#/components/schemas/Gone: the $ref points at nothing in the spec"},
		{"Odd", `#/components/schemas/Odd: the keyword "not" is not one the walker implements`},
	} {
		_, err := roundTripFindings(tc.schema, reflect.TypeFor[Scope](), s,
			schemaPrefix+tc.schema, nil)
		if !isUnsupported(err) || !strings.HasPrefix(err.Error(), tc.want) {
			t.Errorf("roundTripFindings(%s) fails with %v, want %q", tc.schema, err, tc.want)
		}
	}
}
