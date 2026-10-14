package drift

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/surface"
	"github.com/permitio/permit-golang/v2/internal/testkit/faketb"
	"github.com/permitio/permit-golang/v2/models"
)

func TestParseModelMap(t *testing.T) {
	m, err := parseModelMap([]byte("models:\n" +
		"  User:\n" +
		"    spec: pdp-cloud\n" +
		"    schema: UserRead\n" +
		"    fields:\n" +
		"      key: {required: true, null_policy: rejects}\n" +
		"      email: {required: false, null_policy: accepts}\n" +
		"  Kind: {schema: Kind}\n" +
		"blocks:\n" +
		"  Time: a timestamp\n"))
	if err != nil {
		t.Fatal(err)
	}
	yes, no := true, false
	want := modelMap{
		Models: map[string]modelRow{
			"User": {Spec: "pdp-cloud", Schema: "UserRead", Fields: map[string]fieldRow{
				"key":   {Required: &yes, Null: "rejects"},
				"email": {Required: &no, Null: "accepts"},
			}},
			"Kind": {Schema: "Kind"},
		},
		Blocks: map[string]string{"Time": "a timestamp"},
	}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("parseModelMap = %+v, want %+v", m, want)
	}
	if m.Models["Kind"].spec() != defaultSpec || m.Models["User"].spec() != "pdp-cloud" {
		t.Error("a row's spec is not its own, or control-plane when it names none")
	}
	for _, tc := range []struct{ name, text, want string }{
		{"no models", "blocks: {Time: a timestamp}\n", "its models key maps no type"},
		{"an unknown key", "models: {A: {schema: A, table: B}}\n", "field table not found"},
		{"an unknown key of a field", "models: {A: {schema: A, fields: {a: {required: true, " +
			"null_policy: n/a, nullable: true}}}}\n", "field nullable not found"},
		{"not YAML", "models: [\n", "yaml:"},
		{"an unknown spec", "models: {A: {spec: pdp, schema: A}}\n", `models.A: the spec "pdp" ` +
			"is not one of control-plane, pdp-container, pdp-cloud"},
		{"no schema", "models: {A: {fields: {}}}\n", "models.A names no schema"},
		{
			"no required flag", "models: {A: {schema: A, fields: {a: {null_policy: n/a}}}}\n",
			"models.A.fields.a has no required flag",
		},
		{"an unknown null policy", "models: {A: {schema: A, fields: {a: {required: true, " +
			"null_policy: maybe}}}}\n", `models.A.fields.a: the null policy "maybe" is not one ` +
			"of accepts, rejects, n/a"},
		{
			"no null policy", "models: {A: {schema: A, fields: {a: {required: true}}}}\n",
			`models.A.fields.a: the null policy "" is not one of accepts, rejects, n/a`,
		},
		{
			"a block without a reason", "models: {A: {schema: A}}\nblocks: {B: ' '}\n",
			"blocks.B does not say what it stands for",
		},
		{
			"a row and a block", "models: {A: {schema: A}}\nblocks: {A: a type}\n",
			"A is under both models and blocks; a type that a schema describes is a row of " +
				"models",
		},
	} {
		if _, err := parseModelMap([]byte(tc.text)); err == nil ||
			!strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: parseModelMap fails with %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestParseAllowList(t *testing.T) {
	entries, err := parseAllowList([]byte("allow:\n" +
		"  - key: model UserRead field email\n" +
		"    reason: the API returns email, which the published spec does not list\n" +
		"    trigger: when the published spec lists email\n" +
		"  - {key: reply get_user.200.json#/email, reason: r, trigger: t}\n"))
	want := []allowEntry{
		{"model UserRead field email", "the API returns email, which the published spec does " +
			"not list", "when the published spec lists email"},
		{"reply get_user.200.json#/email", "r", "t"},
	}
	if err != nil || !slices.Equal(entries, want) {
		t.Errorf("parseAllowList = %+v, %v; want %+v", entries, err, want)
	}
	if entries, err := parseAllowList([]byte("allow: []\n")); err != nil || len(entries) != 0 {
		t.Errorf("parseAllowList of no entries = %v, %v", entries, err)
	}
	for _, tc := range []struct{ name, text, want string }{
		{
			"an unknown area", "allow: [{key: models A, reason: r, trigger: t}]\n",
			`entry 1, "models A": a key starts with one of ["model " "round trip " "operation " ` +
				`"reply "], as the failing test prints it`,
		},
		{
			"an area alone", "allow: [{key: 'model ', reason: r, trigger: t}]\n",
			`entry 1, "model ": a key starts with one of`,
		},
		{"a key twice", "allow: [{key: model A, reason: r, trigger: t}, {key: model A, reason: " +
			"r, trigger: t}]\n", `entry 2, "model A": another entry has the key`},
		{
			"no reason", "allow: [{key: model A, trigger: t}]\n",
			`entry 1, "model A": give it a reason and a trigger`,
		},
		{
			"a blank trigger", "allow: [{key: model A, reason: r, trigger: ' '}]\n",
			`entry 1, "model A": give it a reason and a trigger`,
		},
		{
			"a ticket in place of a trigger", "allow: [{key: model A, reason: r, ticket: t}]\n",
			"field ticket not found",
		},
		{"not YAML", "allow: {\n", "yaml:"},
	} {
		if _, err := parseAllowList([]byte(tc.text)); err == nil ||
			!strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: parseAllowList fails with %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestAccept(t *testing.T) {
	findings := []finding{
		{"model A field a", "one"},
		{"model A field a", "two"},
		{"model B", "three"},
		{"", "a finding of the SDK's own files"},
		{"reply r.200.json#", "four"},
	}
	entries := []allowEntry{
		{Key: "model A field a"},
		{Key: "model C"},
		{Key: "reply r.200.json#"},
		{Key: "reply s.200.json#"},
		{Key: ""},
	}
	open, stale := accept(findings, entries, "model ")
	wantOpen := []finding{{"model B", "three"}, {"", "a finding of the SDK's own files"}}
	if !slices.Equal(open, wantOpen) {
		t.Errorf("accept leaves %v, want %v", open, wantOpen)
	}
	if want := []allowEntry{{Key: "model C"}}; !slices.Equal(stale, want) {
		t.Errorf("accept finds the stale entries %v, want %v", stale, want)
	}
	_, stale = accept(findings, entries, "reply ")
	if want := []allowEntry{{Key: "reply s.200.json#"}}; !slices.Equal(stale, want) {
		t.Errorf("accept finds the stale replies %v, want %v", stale, want)
	}
	open, stale = accept(nil, nil, "model ")
	if open != nil || stale != nil {
		t.Errorf("accept of nothing = %v, %v", open, stale)
	}
	if got := (finding{"model A", "differs"}).String(); got != "model A: differs" {
		t.Errorf("String = %q", got)
	}
	if got := (finding{text: "differs"}).String(); got != "differs" {
		t.Errorf("String without a key = %q", got)
	}
}

func TestSentinel(t *testing.T) {
	m := modelMap{
		Models: map[string]modelRow{"User": {}, "Kind": {}, "Gone": {}},
		Blocks: map[string]string{"Time": "t", "Lost": "l"},
	}
	types := typesByName([]any{Scope{}, Color("")})
	types["User"], types["Kind"] = types["Scope"], types["Color"]
	delete(types, "Color")
	found := sentinel([]string{"Kind", "New", "Time", "User"}, m, types)
	want := []string{
		"the type New of package models has no row in model-map.yaml: map it to its schema " +
			"under models, or list it under blocks with what it stands for",
		"model-map.yaml maps Gone, which package models does not export: remove its row",
		"model-map.yaml maps Gone, which modelTypes in tools/drift/types_test.go does not " +
			"hold: add it there",
		"model-map.yaml lists the block Lost, which package models does not export: remove it",
		"modelTypes in tools/drift/types_test.go holds Scope, which model-map.yaml does not " +
			"map: map it, or remove it there",
	}
	if got := findingTexts(found); !slices.Equal(got, want) {
		t.Errorf("sentinel =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	delete(types, "Scope")
	m.Models, m.Blocks = map[string]modelRow{"User": {}, "Kind": {}}, map[string]string{"Time": ""}
	if found := sentinel([]string{"Kind", "Time", "User"}, m, types); found != nil {
		t.Errorf("sentinel of counts that agree = %v", findingTexts(found))
	}
}

func TestReadersOfPackageModels(t *testing.T) {
	items := []surface.Item{
		{Kind: surface.Type, Name: "User", Line: "type User struct"},
		{Kind: surface.Type, Name: "Kind", Line: "type Kind string"},
		{Kind: surface.Const, Name: "KindA", Line: `const KindA Kind = "a"`},
		{Kind: surface.Const, Name: "KindB", Line: `const KindB Kind = "b\"q"`},
		{Kind: surface.Const, Name: "Other", Line: `const Other Level = "x"`},
		{Kind: surface.Const, Name: "Untyped", Line: `const Untyped = "u"`},
		{Kind: surface.Const, Name: "Number", Line: `const Number Level = 1`},
		{Kind: surface.Var, Name: "KindC", Line: `const KindC Kind = "c"`},
		{Kind: surface.Const, Name: "Bad", Line: `const Bad Kind = "\q"`},
		{Kind: surface.Func, Name: "New", Line: "func New() User"},
	}
	if got := exportedTypes(items); !slices.Equal(got, []string{"Kind", "User"}) {
		t.Errorf("exportedTypes = %q", got)
	}
	want := map[string]map[string]string{
		"Kind": {"KindA": "a", "KindB": `b"q`}, "Level": {"Other": "x"},
	}
	if got := enumConstants(items); !reflect.DeepEqual(got, want) {
		t.Errorf("enumConstants = %v, want %v", got, want)
	}
	types := typesByName([]any{Scope{}, Color(""), models.Time{}})
	if got := slices.Sorted(maps.Keys(types)); !slices.Equal(got, []string{
		"Color", "Scope", "Time",
	}) || types["Time"] != timeType {
		t.Errorf("typesByName = %v", types)
	}
}

func TestRepositoryReadersOfPackageModels(t *testing.T) {
	p, err := surface.Read(modelsDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := enumConstants(p.Items)["AttributeType"]["AttributeTypeObjectArray"]; got !=
		"object_array" {
		t.Errorf("the constant AttributeTypeObjectArray reads as %q, want object_array", got)
	}
	if got := exportedTypes(p.Items); !slices.Contains(got, "APIKeyScopeRead") {
		t.Errorf("the exported types of package models are %q, without APIKeyScopeRead", got)
	}
}

func TestSentModels(t *testing.T) {
	type (
		Leaf    struct{ Name string }
		Branch  struct{ Leaves []*Leaf }
		Request struct {
			Branch  models.Optional[map[string][2]Branch]
			Again   *Request
			Kind    Color
			Outside models.Time
		}
		Alone struct{ Leaf Leaf }
	)
	types := typesByName([]any{Request{}, Alone{}, Leaf{}})
	ops := []rest.Op{
		{ID: "create", RequestModel: "Request"},
		{ID: "update", RequestModel: "Request"},
		{ID: "get", ResponseModel: "Alone"},
		{ID: "lost", RequestModel: "Lost"},
		{ID: "leaf", RequestModel: "Leaf"},
	}
	got := sentModels(ops, types, reflect.TypeFor[Leaf]().PkgPath())
	want := map[string]string{"Request": "create", "Branch": "create", "Leaf": "create"}
	if !maps.Equal(got, want) {
		t.Errorf("sentModels = %v, want %v", got, want)
	}
}

func TestReport(t *testing.T) {
	r := &repository{allow: []allowEntry{{Key: "model A"}, {Key: "model B"}, {Key: "reply x"}}}
	tb := faketb.New(t)
	r.report(tb, "model ", []finding{{"model A", "accepted"}, {"model C", "open"}},
		[]error{errors.New("Odd: a keyword")})
	want := []string{
		"model C: open",
		`allow.yaml: the entry "model B" accepts no finding: remove it`,
		"DID NOT RUN: Odd: a keyword",
	}
	if got := tb.Take(); !slices.Equal(got, want) {
		t.Errorf("report fails with\n%s\nwant\n%s", strings.Join(got, "\n"),
			strings.Join(want, "\n"))
	}
	r.report(tb, "model ", []finding{{"model A", "accepted"}, {"model B", "accepted"}}, nil)
	if got := tb.Take(); got != nil {
		t.Errorf("report of accepted findings fails with %q", got)
	}
}
