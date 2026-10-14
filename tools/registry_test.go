package tools

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/surface"
	"github.com/permitio/permit-golang/v2/internal/wire"
)

// The tests in this file import the SDK's internal packages through the replace directive of
// go.mod, which points at the repository root, and read package models there, outside this
// module. Their names start with TestRepository, which the mutation tests skip: the copies of
// this module that they build hold nothing above it.

// coverageFile holds the SDK's decision for every operation of the specs.
const coverageFile = "specs/coverage.yaml"

// specFiles are the snapshots of the specs whose operations the SDK can send.
var specFiles = []string{
	"specs/control-plane.json", "specs/pdp-container.json", "specs/pdp-cloud.json",
}

// modelsDir is the directory of package models, whose types the operations name.
const modelsDir = "../models"

// coverageRow is the part of a row of coverage.yaml that the registry test reads.
type coverageRow struct {
	Op      string `yaml:"op"`
	Route   string `yaml:"route"`
	Status  string `yaml:"status"`
	SDKOnly string `yaml:"sdk_only"` //nolint:tagliatelle // The file's key.
}

// coverageSections are the sections of coverage.yaml that hold rows.
type coverageSections struct {
	ControlPlane []coverageRow `yaml:"control-plane"` //nolint:tagliatelle // The file's key.
	PDP          []coverageRow `yaml:"pdp"`
}

// readCoverage returns the rows of coverage.yaml that name an op, by their op.
func readCoverage(t *testing.T) map[string]coverageRow {
	t.Helper()
	data, err := os.ReadFile(coverageFile)
	if err != nil {
		t.Fatal(err)
	}
	var sections coverageSections
	if err := yaml.Unmarshal(data, &sections); err != nil {
		t.Fatalf("parsing %s: %v", coverageFile, err)
	}
	rows := map[string]coverageRow{}
	for _, row := range slices.Concat(sections.ControlPlane, sections.PDP) {
		if row.Op != "" {
			rows[row.Op] = row
		}
	}
	if len(rows) == 0 {
		t.Fatalf("%s has no row with an op; the file was not read", coverageFile)
	}
	return rows
}

// specOp is an operation of a spec.
type specOp struct {
	// Route is the operation's method and path, such as
	// "GET /v2/facts/{proj_id}/{env_id}/users/{user_id}".
	Route string

	// ID is the operationId.
	ID string

	// Params are the operation's parameters.
	Params []specParam
}

// specParam is a parameter of an operation of a spec.
type specParam struct {
	Name       string `json:"name"`
	In         string `json:"in"`
	Deprecated bool   `json:"deprecated"`
}

// specMethods are the keys of a path of a spec that name its operations, as OpenAPI writes them.
var specMethods = []string{"get", "post", "put", "patch", "delete"}

// readSpecs returns the operations of the specs in specFiles. It fails the test on a key of a
// path that is not one of specMethods, such as parameters that every operation of the path
// shares, which it does not read.
func readSpecs(t *testing.T) []specOp {
	t.Helper()
	var ops []specOp
	for _, file := range specFiles {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var spec struct {
			Paths map[string]map[string]struct {
				OperationID string      `json:"operationId"` //nolint:tagliatelle // OpenAPI's name.
				Parameters  []specParam `json:"parameters"`
			} `json:"paths"`
		}
		if err := json.Unmarshal(data, &spec); err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		for _, path := range slices.Sorted(maps.Keys(spec.Paths)) {
			for _, method := range slices.Sorted(maps.Keys(spec.Paths[path])) {
				if !slices.Contains(specMethods, method) {
					t.Fatalf("%s: %s has the key %q, which the registry test does not read",
						file, path, method)
				}
				op := spec.Paths[path][method]
				ops = append(ops, specOp{
					Route: strings.ToUpper(method) + " " + path, ID: op.OperationID,
					Params: op.Parameters,
				})
			}
		}
	}
	if len(ops) == 0 {
		t.Fatalf("the specs %s list no operation; they were not read", specFiles)
	}
	return ops
}

// routesByID returns the routes of ops by their operationIds.
func routesByID(ops []specOp) map[string][]string {
	routes := map[string][]string{}
	for _, op := range ops {
		routes[op.ID] = append(routes[op.ID], op.Route)
	}
	return routes
}

// readModels returns the names of the exported types of package models.
func readModels(t *testing.T) map[string]bool {
	t.Helper()
	p, err := surface.Read(modelsDir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "models" {
		t.Fatalf("%s holds the package %q, want models", modelsDir, p.Name)
	}
	return modelTypes(p.Items)
}

// modelTypes returns the names of the types among items, the exported declarations of package
// models: a RequestModel or ResponseModel must name a type, not a constant, variable, function,
// method or field.
func modelTypes(items []surface.Item) map[string]bool {
	types := map[string]bool{}
	for _, item := range items {
		if item.Kind == surface.Type {
			types[item.Name] = true
		}
	}
	return types
}

// registryProblems returns a problem for each operation of ops whose ID is not the op of a row of
// coverage.yaml that is planned or covered, whose method and route are not those that a spec
// lists for its ID, or that the row's route of an SDK-only row is, and whose RequestModel or
// ResponseModel names no exported type of package models. rows are the rows of coverage.yaml by
// their op, routes the routes of the specs by operationId, and models the names of the exported
// types of package models.
func registryProblems(ops []rest.Op, rows map[string]coverageRow, routes map[string][]string,
	models map[string]bool,
) []string {
	var problems []string
	add := func(op rest.Op, format string, args ...any) {
		problems = append(problems, "the operation "+op.ID+": "+fmt.Sprintf(format, args...))
	}
	for _, op := range ops {
		row, ok := rows[op.ID]
		switch {
		case !ok:
			add(op, "no row of %s has this op; give the operation's operationId, or add its row",
				coverageFile)
		case row.Status != "planned" && row.Status != "covered":
			add(op, "its row of %s is %s; the row of an operation that the SDK sends is planned "+
				"or covered", coverageFile, row.Status)
		}
		templates := routes[op.ID]
		if row.SDKOnly != "" {
			templates = []string{row.Route}
		}
		route := op.Method + " " + op.Route
		switch {
		case len(templates) == 0:
			add(op, "no spec lists it, and no SDK-only row of %s has it", coverageFile)
		case !slices.Contains(templates, route):
			quoted := make([]string, 0, len(templates))
			for _, template := range templates {
				quoted = append(quoted, strconv.Quote(template))
			}
			add(op, "the method and route %q differ from the spec's, %s", route,
				strings.Join(quoted, " or "))
		}
		for _, model := range []struct{ field, name string }{
			{"RequestModel", op.RequestModel}, {"ResponseModel", op.ResponseModel},
		} {
			if model.name != "" && !models[model.name] {
				add(op, "its %s %q names no exported type of package models", model.field,
					model.name)
			}
		}
	}
	return problems
}

// TestRepositoryRegistry checks every operation of the SDK's registry, rest.Default, against
// tools/specs and package models, as registryProblems does.
func TestRepositoryRegistry(t *testing.T) {
	ops := rest.Default().Ops()
	problems := registryProblems(ops, readCoverage(t), routesByID(readSpecs(t)), readModels(t))
	for _, problem := range problems {
		t.Error(problem)
	}
	t.Logf("checked the %d operations of the registry", len(ops))
}

// TestRepositoryRegistryReadsTheFiles checks the readers of the registry test on operations of
// the real files: one of the control plane, one of the PDP and an SDK-only one.
// TestRepositoryModelTypes checks that only a type of package models counts as a model.
func TestRepositoryModelTypes(t *testing.T) {
	var items []surface.Item
	for _, kind := range []surface.Kind{
		surface.Const, surface.Var, surface.Type, surface.Member, surface.Func, surface.Method,
	} {
		items = append(items, surface.Item{Kind: kind, Name: fmt.Sprintf("Name%d", kind)})
	}
	want := map[string]bool{fmt.Sprintf("Name%d", surface.Type): true}
	if got := modelTypes(items); !maps.Equal(got, want) {
		t.Errorf("modelTypes = %v, want %v", got, want)
	}
}

func TestRepositoryRegistryReadsTheFiles(t *testing.T) {
	ops := []rest.Op{
		{
			ID: "get_user", Method: http.MethodGet,
			Route: "/v2/facts/{proj_id}/{env_id}/users/{user_id}",
		},
		{ID: "is_allowed_allowed_post", Method: http.MethodPost, Route: "/allowed"},
		{ID: "elements_login_as", Method: http.MethodPost, Route: "/v2/auth/elements_login_as"},
	}
	problems := registryProblems(ops, readCoverage(t), routesByID(readSpecs(t)), readModels(t))
	if problems != nil {
		t.Errorf("registryProblems of operations that the files list = %q, want none", problems)
	}
}

func TestRepositoryRegistryProblems(t *testing.T) {
	rows := map[string]coverageRow{
		"get_user":     {Op: "get_user", Route: "GET /v2/users/{user_id}", Status: "planned"},
		"delete_user":  {Op: "delete_user", Route: "DELETE /v2/users/{user_id}", Status: "covered"},
		"list_history": {Op: "list_history", Route: "GET /v2/history", Status: "excluded"},
		"login_as": {
			Op: "login_as", Route: "POST /v2/login_as", Status: "planned", SDKOnly: "not listed",
		},
		"lost": {Op: "lost", Route: "GET /v2/lost", Status: "planned"},
	}
	routes := map[string][]string{
		"get_user":     {"GET /v2/users/{user_id}"},
		"delete_user":  {"DELETE /v2/users/{user_id}", "DELETE /facts/users/{user_id}"},
		"list_history": {"GET /v2/history"},
		"get_role":     {"GET /v2/roles/{role_id}"},
	}
	models := map[string]bool{"UserRead": true, "UserCreate": true}
	valid := []rest.Op{
		{
			ID: "get_user", Method: http.MethodGet, Route: "/v2/users/{user_id}",
			ResponseModel: "UserRead",
		},
		{ID: "delete_user", Method: http.MethodDelete, Route: "/facts/users/{user_id}"},
		{
			ID: "login_as", Method: http.MethodPost, Route: "/v2/login_as",
			RequestModel: "UserCreate", ResponseModel: "UserRead",
		},
	}
	if problems := registryProblems(valid, rows, routes, models); problems != nil {
		t.Errorf("registryProblems of valid operations = %q, want none", problems)
	}
	for _, tc := range []struct {
		name string
		op   rest.Op
		want []string
	}{
		{
			name: "an ID missing from coverage.yaml",
			op:   rest.Op{ID: "get_role", Method: http.MethodGet, Route: "/v2/roles/{role_id}"},
			want: []string{"the operation get_role: no row of specs/coverage.yaml has this op; " +
				"give the operation's operationId, or add its row"},
		},
		{
			name: "an excluded row",
			op:   rest.Op{ID: "list_history", Method: http.MethodGet, Route: "/v2/history"},
			want: []string{"the operation list_history: its row of specs/coverage.yaml is " +
				"excluded; the row of an operation that the SDK sends is planned or covered"},
		},
		{
			name: "a route that differs from its template",
			op:   rest.Op{ID: "get_user", Method: http.MethodGet, Route: "/v2/user/{user_id}"},
			want: []string{`the operation get_user: the method and route "GET ` +
				`/v2/user/{user_id}" differ from the spec's, "GET /v2/users/{user_id}"`},
		},
		{
			name: "a method that differs",
			op:   rest.Op{ID: "delete_user", Method: http.MethodPost, Route: "/v2/users/{user_id}"},
			want: []string{`the operation delete_user: the method and route "POST ` +
				`/v2/users/{user_id}" differ from the spec's, "DELETE /v2/users/{user_id}" or ` +
				`"DELETE /facts/users/{user_id}"`},
		},
		{
			name: "an SDK-only route that differs from its row",
			op:   rest.Op{ID: "login_as", Method: http.MethodPost, Route: "/v2/auth/login_as"},
			want: []string{`the operation login_as: the method and route "POST ` +
				`/v2/auth/login_as" differ from the spec's, "POST /v2/login_as"`},
		},
		{
			name: "a planned row that no spec lists",
			op:   rest.Op{ID: "lost", Method: http.MethodGet, Route: "/v2/lost"},
			want: []string{"the operation lost: no spec lists it, and no SDK-only row of " +
				"specs/coverage.yaml has it"},
		},
		{
			name: "a ResponseModel that names no type",
			op: rest.Op{
				ID: "get_user", Method: http.MethodGet, Route: "/v2/users/{user_id}",
				ResponseModel: "UserReadd",
			},
			want: []string{`the operation get_user: its ResponseModel "UserReadd" names no ` +
				"exported type of package models"},
		},
		{
			name: "a RequestModel that names no type",
			op: rest.Op{
				ID: "get_user", Method: http.MethodGet, Route: "/v2/users/{user_id}",
				RequestModel: "models.UserCreate",
			},
			want: []string{`the operation get_user: its RequestModel "models.UserCreate" names ` +
				"no exported type of package models"},
		},
		{
			name: "everything wrong",
			op: rest.Op{
				ID: "nothing", Method: http.MethodGet, Route: "/v2/nothing",
				RequestModel: "A", ResponseModel: "B",
			},
			want: []string{
				"the operation nothing: no row of specs/coverage.yaml has this op; give the " +
					"operation's operationId, or add its row",
				"the operation nothing: no spec lists it, and no SDK-only row of " +
					"specs/coverage.yaml has it",
				`the operation nothing: its RequestModel "A" names no exported type of package ` +
					"models",
				`the operation nothing: its ResponseModel "B" names no exported type of package ` +
					"models",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := registryProblems(append(slices.Clone(valid), tc.op), rows, routes, models)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("registryProblems =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

// deprecatedParamProblems returns a problem for each query parameter that an operation of ops
// marks deprecated and never does not list, and for each name of never that no operation marks
// deprecated, or that an operation takes without marking it deprecated: never is the list of the
// query parameters that the SDK never sends, by name.
func deprecatedParamProblems(ops []specOp, never []string) []string {
	deprecated := map[string]bool{}
	takenBy := map[string][]string{}
	for _, op := range ops {
		for _, p := range op.Params {
			switch {
			case p.In != "query":
			case p.Deprecated:
				deprecated[p.Name] = true
			default:
				takenBy[p.Name] = append(takenBy[p.Name], op.ID)
			}
		}
	}
	var problems []string
	for _, name := range slices.Sorted(maps.Keys(deprecated)) {
		if !slices.Contains(never, name) {
			problems = append(problems, fmt.Sprintf("the specs mark the query parameter %s "+
				"deprecated; add it to the deprecated parameters of internal/wire, so that the "+
				"SDK never sends it", name))
		}
	}
	for _, name := range never {
		switch {
		case !deprecated[name]:
			problems = append(problems, fmt.Sprintf("internal/wire never sends the query "+
				"parameter %s, which no spec marks deprecated; remove it from its deprecated "+
				"parameters", name))
		case len(takenBy[name]) > 0:
			problems = append(problems, fmt.Sprintf("internal/wire never sends the query "+
				"parameter %s, which the operations %s take without marking it deprecated; "+
				"send it to them", name, strings.Join(takenBy[name], ", ")))
		}
	}
	return problems
}

// TestRepositoryDeprecatedParams checks the query parameters that the SDK never sends,
// wire.DeprecatedParams, against the specs: the list holds every query parameter that a spec
// marks deprecated, and no name that an operation takes without marking it deprecated.
func TestRepositoryDeprecatedParams(t *testing.T) {
	for _, problem := range deprecatedParamProblems(readSpecs(t), wire.DeprecatedParams()) {
		t.Error(problem)
	}
}

func TestRepositoryDeprecatedParamProblems(t *testing.T) {
	ops := []specOp{
		{ID: "list_a", Params: []specParam{
			{Name: "detailed", In: "query", Deprecated: true},
			{Name: "page", In: "query"},
			{Name: "proj_id", In: "path", Deprecated: true},
		}},
		{ID: "list_b", Params: []specParam{
			{Name: "include_total_count", In: "query", Deprecated: true},
			{Name: "page", In: "query"},
		}},
		{ID: "list_c", Params: []specParam{
			{Name: "include_total_count", In: "query"},
			{Name: "x-wait", In: "header", Deprecated: true},
		}},
	}
	complete := []string{"detailed", "include_total_count"}
	if problems := deprecatedParamProblems(ops[:2], complete); problems != nil {
		t.Errorf("deprecatedParamProblems of a complete list = %q, want none", problems)
	}
	got := deprecatedParamProblems(ops, []string{"include_total_count", "page_size"})
	want := []string{
		"the specs mark the query parameter detailed deprecated; add it to the deprecated " +
			"parameters of internal/wire, so that the SDK never sends it",
		"internal/wire never sends the query parameter include_total_count, which the " +
			"operations list_c take without marking it deprecated; send it to them",
		"internal/wire never sends the query parameter page_size, which no spec marks " +
			"deprecated; remove it from its deprecated parameters",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("deprecatedParamProblems =\n%q\nwant\n%q", got, want)
	}
}
