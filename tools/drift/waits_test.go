package drift

import (
	"errors"
	"fmt"
	"go/doc/comment"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/permitio/permit-golang/v2/internal/rest"
)

// The tests in this file check the routes on which the PDP container waits for sync, those that
// pdp-container.json lists under /facts, against the table of the section Facts and
// wait-for-sync of the root's doc.go, the operations of the registry and coverage.yaml.

const (
	// rootDoc is the file of the package documentation of the root package.
	rootDoc = "../../doc.go"

	// coverageFile holds the SDK's decision for every operation of the specs.
	coverageFile = "coverage.yaml"

	// waitsHeading is the heading of the section of rootDoc whose first code block lists the
	// routes on which the PDP container waits for sync.
	waitsHeading = "Facts and wait-for-sync"

	// apiFactsPrefix and pdpFactsPrefix start the routes of the facts API on the Permit API and
	// on the PDP container, after the method.
	apiFactsPrefix = "/v2/facts/{proj_id}/{env_id}/"
	pdpFactsPrefix = "/facts/"

	// catchAll is the route of the PDP container that forwards every facts route that its spec
	// does not list, without waiting.
	catchAll = "ANY /facts/{full_path}"
)

// waitMethods are the methods of the routes of the table.
var waitMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

// docWaitingRoutes returns the routes of the table of the section waitsHeading of src, the source
// of a Go file with a package doc comment: its first code block, one route a line, a method and
// a path under /facts separated by spaces. It returns an error when the section, its table or a
// route is missing or malformed.
func docWaitingRoutes(src []byte) ([]string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "doc.go", src,
		parser.PackageClauseOnly|parser.ParseComments)
	if err != nil {
		return nil, err
	}
	if f.Doc == nil {
		return nil, errors.New("the file has no package doc comment")
	}
	var p comment.Parser
	blocks := p.Parse(f.Doc.Text()).Content
	start := slices.IndexFunc(blocks, func(b comment.Block) bool {
		h, ok := b.(*comment.Heading)
		return ok && headingText(h) == waitsHeading
	})
	if start < 0 {
		return nil, fmt.Errorf("the package doc comment has no section # %s", waitsHeading)
	}
	for _, b := range blocks[start+1:] {
		if _, ok := b.(*comment.Heading); ok {
			break
		}
		if code, ok := b.(*comment.Code); ok {
			return tableRoutes(code.Text)
		}
	}
	return nil, fmt.Errorf("the section # %s has no table of routes, a code block", waitsHeading)
}

// headingText returns the plain text of a heading.
func headingText(h *comment.Heading) string {
	var b strings.Builder
	for _, t := range h.Text {
		if plain, ok := t.(comment.Plain); ok {
			b.WriteString(string(plain))
		}
	}
	return b.String()
}

// tableRoutes returns the routes of text, the lines of the table: each a method of waitMethods
// and a path under /facts, separated by spaces, written as a route with one space between them.
func tableRoutes(text string) ([]string, error) {
	var routes []string
	for line := range strings.Lines(text) {
		fields := strings.Fields(line)
		if len(fields) != 2 || !slices.Contains(waitMethods, fields[0]) ||
			!strings.HasPrefix(fields[1], pdpFactsPrefix) {
			return nil, fmt.Errorf("the line %q of the table of # %s is not a method and a "+
				"path under /facts, such as POST /facts/users", strings.TrimSpace(line),
				waitsHeading)
		}
		routes = append(routes, fields[0]+" "+fields[1])
	}
	return routes, nil
}

// specFactsRoutes returns the routes under /facts that s, the spec of the PDP container, lists,
// sorted, each as a method in upper case and a path.
func specFactsRoutes(s *spec) []string {
	paths, _ := s.doc["paths"].(map[string]any)
	var routes []string
	for path, item := range paths {
		if !strings.HasPrefix(path, pdpFactsPrefix) {
			continue
		}
		operations, _ := item.(map[string]any)
		for method := range operations {
			routes = append(routes, strings.ToUpper(method)+" "+path)
		}
	}
	slices.Sort(routes)
	return routes
}

// coverageRow is the part of a row of coverage.yaml that the waits check reads.
type coverageRow struct {
	Route  string `yaml:"route"`
	Status string `yaml:"status"`
}

// coverageSections are the sections of coverage.yaml that hold rows.
type coverageSections struct {
	ControlPlane []coverageRow `yaml:"control-plane"` //nolint:tagliatelle // The file's key.
	PDP          []coverageRow `yaml:"pdp"`
}

// apiRoute returns route, a route of the PDP container under /facts, as the Permit API serves it,
// under /v2/facts/{proj_id}/{env_id}.
func apiRoute(route string) string {
	method, path, _ := strings.Cut(route, " ")
	return method + " " + apiFactsPrefix + strings.TrimPrefix(path, pdpFactsPrefix)
}

// waitsInput is what the waits check reads.
type waitsInput struct {
	// doc are the routes of the table of the section waitsHeading of the root's doc.go.
	doc []string

	// spec are the routes under /facts that pdp-container.json lists, on which the PDP
	// container waits for sync.
	spec []string

	// ops are the operations of the registry.
	ops []rest.Op

	// coverage are the rows of coverage.yaml.
	coverage coverageSections
}

// waitsProblems returns the problems of in: the table of the documentation lists exactly the
// routes of the spec; an operation waits for sync, WaitsForSync, exactly when the PDP container
// waits on its route, which the operation reaches under /facts as rest.Op.ViaPDP gives it; the
// routes of the spec on which no operation waits yet are exactly those whose row of the control
// plane in coverage.yaml is planned, and the row of the PDP of each has the status of that row;
// and the PDP section has no row under /facts for a route outside the spec but the catch-all.
func waitsProblems(in waitsInput) []string {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	spec := map[string]bool{}
	for _, route := range in.spec {
		spec[route] = true
	}
	documented := map[string]bool{}
	for _, route := range in.doc {
		switch {
		case documented[route]:
			add("doc.go: the table of # %s lists %s twice", waitsHeading, route)
		case !spec[route]:
			add("doc.go: the table of # %s lists %s, on which the PDP container does not wait: "+
				"pdp-container.json does not list it", waitsHeading, route)
		}
		documented[route] = true
	}
	waiting := map[string]string{}
	for _, op := range in.ops {
		viaPDP := op.Method + " " + op.ViaPDP().Route
		if op.WaitsForSync {
			waiting[viaPDP] = op.ID
		}
		switch {
		case op.WaitsForSync && !spec[viaPDP]:
			add("the operation %s is WaitsForSync, and the PDP container does not wait on its "+
				"route, %s", op.ID, viaPDP)
		case !op.WaitsForSync && spec[routeOf(op)]:
			add("the operation %s is not WaitsForSync, and the PDP container waits on its "+
				"route, %s; set Facts and WaitsForSync", op.ID, routeOf(op))
		}
	}
	apiRows := rowsByRoute(in.coverage.ControlPlane)
	pdpRows := rowsByRoute(in.coverage.PDP)
	for _, route := range in.spec {
		if !documented[route] {
			add("doc.go: the table of # %s lacks %s, on which the PDP container waits",
				waitsHeading, route)
		}
		api, apiOK := apiRows[apiRoute(route)]
		pdp, pdpOK := pdpRows[route]
		if !apiOK || !pdpOK {
			add("%s: %s has no row in control-plane or in pdp; give it a row in each", coverageFile,
				route)
			continue
		}
		id, registered := waiting[route]
		switch {
		case registered && api.Status == "planned":
			add("%s: the row of %s is planned, and the operation %s waits on it; mark the row "+
				"covered", coverageFile, api.Route, id)
		case !registered && api.Status != "planned":
			add("%s: the row of %s is %s, and no operation of the registry waits on it; add "+
				"the operation with WaitsForSync, or mark the row planned", coverageFile,
				api.Route, api.Status)
		}
		if pdp.Status != api.Status {
			add("%s: the row of %s is %s, and the row of %s is %s; give both one status",
				coverageFile, route, pdp.Status, api.Route, api.Status)
		}
	}
	for _, row := range in.coverage.PDP {
		_, path, _ := strings.Cut(row.Route, " ")
		if strings.HasPrefix(path, pdpFactsPrefix) && row.Route != catchAll && !spec[row.Route] {
			add("%s: the pdp row of %s is %s, and the PDP container does not list the route: "+
				"it reaches %s; remove the row", coverageFile, row.Route, row.Status, catchAll)
		}
	}
	return problems
}

// routeOf returns the route of op under /facts when it is an operation of the facts API that the
// PDP container serves there, and else its own route, each after its method.
func routeOf(op rest.Op) string {
	route := op.Method + " " + op.Route
	if tail, ok := strings.CutPrefix(route, op.Method+" "+apiFactsPrefix); ok {
		return op.Method + " " + pdpFactsPrefix + tail
	}
	return route
}

// rowsByRoute returns rows by their routes.
func rowsByRoute(rows []coverageRow) map[string]coverageRow {
	byRoute := map[string]coverageRow{}
	for _, row := range rows {
		byRoute[row.Route] = row
	}
	return byRoute
}

// TestRepositoryWaits checks the routes on which the PDP container waits for sync, as
// waitsProblems describes, on the root's doc.go, pdp-container.json, the registry and
// coverage.yaml.
func TestRepositoryWaits(t *testing.T) {
	src, err := os.ReadFile(rootDoc)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := docWaitingRoutes(src)
	if err != nil {
		t.Fatalf("%s: %v", rootDoc, err)
	}
	s, err := readSpec("pdp-container", filepath.Join(committedSpecs, "pdp-container.json"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(committedSpecs, coverageFile))
	if err != nil {
		t.Fatal(err)
	}
	var sections coverageSections
	if err := yaml.Unmarshal(data, &sections); err != nil {
		t.Fatalf("parsing %s: %v", coverageFile, err)
	}
	in := waitsInput{
		doc: doc, spec: specFactsRoutes(s), ops: rest.Default().Ops(),
		coverage: sections,
	}
	if len(in.spec) == 0 || len(sections.ControlPlane) == 0 || len(sections.PDP) == 0 {
		t.Fatalf("DID NOT RUN: pdp-container.json lists %d routes under /facts, and %s has %d "+
			"rows of the control plane and %d of the PDP", len(in.spec), coverageFile,
			len(sections.ControlPlane), len(sections.PDP))
	}
	for _, problem := range waitsProblems(in) {
		t.Error(problem)
	}
	t.Logf("the PDP container waits on %d routes", len(in.spec))
}

// The tests below check the waits check on inputs written for them.

// waitsSpec are the routes of the spec of the waits tests.
var waitsSpec = []string{"PATCH /facts/users/{user_id}", "POST /facts/users"}

// waitsOp returns a Facts operation of the registry with the ID id on route, a route of the Permit
// API after its method, that waits for sync when waits is set.
func waitsOp(id, route string, waits bool) rest.Op {
	method, path, _ := strings.Cut(route, " ")
	return rest.Op{ID: id, Method: method, Route: path, Facts: true, WaitsForSync: waits}
}

// waitsCoverage returns the rows of the routes of waitsSpec, each with status in both sections,
// the row of a route that the spec does not list and the catch-all.
func waitsCoverage(status string) coverageSections {
	return coverageSections{
		ControlPlane: []coverageRow{
			{"POST /v2/facts/{proj_id}/{env_id}/users", status},
			{"PATCH /v2/facts/{proj_id}/{env_id}/users/{user_id}", status},
			{"GET /v2/facts/{proj_id}/{env_id}/users", "planned"},
		},
		PDP: []coverageRow{
			{"POST /facts/users", status},
			{"PATCH /facts/users/{user_id}", status},
			{catchAll, "planned"},
			{"POST /allowed", "covered"},
		},
	}
}

// waitsValid returns the inputs of a repository whose operations wait on both routes of
// waitsSpec.
func waitsValid() waitsInput {
	return waitsInput{
		doc:  slices.Clone(waitsSpec),
		spec: slices.Clone(waitsSpec),
		ops: []rest.Op{
			waitsOp("create_user", "POST "+apiFactsPrefix+"users", true),
			waitsOp("update_user", "PATCH "+apiFactsPrefix+"users/{user_id}", true),
			waitsOp("list_users", "GET "+apiFactsPrefix+"users", false),
			{ID: "is_allowed", Method: "POST", Route: "/allowed"},
		},
		coverage: waitsCoverage("covered"),
	}
}

func TestWaitsProblemsPass(t *testing.T) {
	if problems := waitsProblems(waitsValid()); problems != nil {
		t.Errorf("waitsProblems of a repository whose operations wait = %q, want none", problems)
	}
	before := waitsValid()
	before.ops = before.ops[2:]
	before.coverage = waitsCoverage("planned")
	if problems := waitsProblems(before); problems != nil {
		t.Errorf("waitsProblems of a repository without the operations = %q, want none",
			problems)
	}
}

func TestWaitsProblemsFail(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(in *waitsInput)
		want   []string
	}{
		{
			name:   "a waiting route missing from the table",
			change: func(in *waitsInput) { in.doc = in.doc[:1] },
			want: []string{"doc.go: the table of # Facts and wait-for-sync lacks POST " +
				"/facts/users, on which the PDP container waits"},
		},
		{
			name:   "a route in the table that does not wait",
			change: func(in *waitsInput) { in.doc = append(in.doc, "GET /facts/users") },
			want: []string{"doc.go: the table of # Facts and wait-for-sync lists GET " +
				"/facts/users, on which the PDP container does not wait: pdp-container.json " +
				"does not list it"},
		},
		{
			name:   "a route twice in the table",
			change: func(in *waitsInput) { in.doc = append(in.doc, "POST /facts/users") },
			want: []string{"doc.go: the table of # Facts and wait-for-sync lists POST " +
				"/facts/users twice"},
		},
		{
			name: "an operation that waits on a route that does not",
			change: func(in *waitsInput) {
				in.ops[2].WaitsForSync = true
			},
			want: []string{"the operation list_users is WaitsForSync, and the PDP container " +
				"does not wait on its route, GET /facts/users"},
		},
		{
			name: "an operation on a waiting route that does not wait",
			change: func(in *waitsInput) {
				in.ops[0].WaitsForSync = false
			},
			want: []string{
				"the operation create_user is not WaitsForSync, and the PDP container waits on " +
					"its route, POST /facts/users; set Facts and WaitsForSync",
				"coverage.yaml: the row of POST /v2/facts/{proj_id}/{env_id}/users is covered, " +
					"and no operation of the registry waits on it; add the operation with " +
					"WaitsForSync, or mark the row planned",
			},
		},
		{
			name: "an operation on a waiting route that is not Facts",
			change: func(in *waitsInput) {
				in.ops[1].Facts, in.ops[1].WaitsForSync = false, false
			},
			want: []string{
				"the operation update_user is not WaitsForSync, and the PDP container waits on " +
					"its route, PATCH /facts/users/{user_id}; set Facts and WaitsForSync",
				"coverage.yaml: the row of PATCH /v2/facts/{proj_id}/{env_id}/users/{user_id} " +
					"is covered, and no operation of the registry waits on it; add the " +
					"operation with WaitsForSync, or mark the row planned",
			},
		},
		{
			name: "a planned row of a route that an operation waits on",
			change: func(in *waitsInput) {
				in.coverage = waitsCoverage("planned")
			},
			want: []string{
				"coverage.yaml: the row of PATCH /v2/facts/{proj_id}/{env_id}/users/{user_id} " +
					"is planned, and the operation update_user waits on it; mark the row covered",
				"coverage.yaml: the row of POST /v2/facts/{proj_id}/{env_id}/users is planned, " +
					"and the operation create_user waits on it; mark the row covered",
			},
		},
		{
			name: "rows of a route with two statuses",
			change: func(in *waitsInput) {
				in.coverage.PDP[0].Status = "planned"
			},
			want: []string{"coverage.yaml: the row of POST /facts/users is planned, and the " +
				"row of POST /v2/facts/{proj_id}/{env_id}/users is covered; give both one status"},
		},
		{
			name: "a waiting route without a row",
			change: func(in *waitsInput) {
				in.coverage.ControlPlane = in.coverage.ControlPlane[1:]
			},
			want: []string{"coverage.yaml: POST /facts/users has no row in control-plane or in " +
				"pdp; give it a row in each"},
		},
		{
			name: "a planned row for a route outside the spec",
			change: func(in *waitsInput) {
				in.coverage.PDP = append(in.coverage.PDP,
					coverageRow{"GET /facts/users", "planned"})
			},
			want: []string{"coverage.yaml: the pdp row of GET /facts/users is planned, and the " +
				"PDP container does not list the route: it reaches ANY /facts/{full_path}; " +
				"remove the row"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := waitsValid()
			tc.change(&in)
			if got := waitsProblems(in); !slices.Equal(got, tc.want) {
				t.Errorf("waitsProblems =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestDocWaitingRoutes(t *testing.T) {
	const header = "// Package p is a package.\n//\n"
	got, err := docWaitingRoutes([]byte(header + "// # Facts and wait-for-sync\n//\n" +
		"// It waits on these:\n//\n//\tPOST   /facts/users\n//\tPATCH  /facts/users/{user_id}\n" +
		"//\n// # Another section\n//\n//\tGET /facts/other\npackage p\n"))
	want := []string{"POST /facts/users", "PATCH /facts/users/{user_id}"}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("docWaitingRoutes = %q, %v; want %q", got, err, want)
	}
	for name, tc := range map[string]struct{ src, want string }{
		"no package doc": {"package p\n", "the file has no package doc comment"},
		"no section": {
			header + "// # Other\n//\n//\tPOST /facts/users\npackage p\n",
			"the package doc comment has no section # Facts and wait-for-sync",
		},
		"a section without a table": {
			header + "// # Facts and wait-for-sync\n//\n// None.\n//\n// # Next\n//\n" +
				"//\tPOST /facts/users\npackage p\n",
			"the section # Facts and wait-for-sync has no table of routes, a code block",
		},
		"a line without a path": {
			header + "// # Facts and wait-for-sync\n//\n//\tPOST\npackage p\n",
			`the line "POST" of the table of # Facts and wait-for-sync is not a method and a ` +
				"path under /facts",
		},
		"a path outside /facts": {
			header + "// # Facts and wait-for-sync\n//\n//\tPOST /v2/users\npackage p\n",
			`the line "POST /v2/users" of the table`,
		},
		"an unknown method": {
			header + "// # Facts and wait-for-sync\n//\n//\tSEND /facts/users\npackage p\n",
			`the line "SEND /facts/users" of the table`,
		},
		"three fields": {
			header + "// # Facts and wait-for-sync\n//\n//\tPOST /facts/users waits\npackage p\n",
			`the line "POST /facts/users waits" of the table`,
		},
		"not Go": {"// Package p.\npackage\n", "expected 'IDENT'"},
	} {
		if got, err := docWaitingRoutes([]byte(tc.src)); err == nil ||
			!strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: docWaitingRoutes = %q, %v; want an error that contains %q", name, got,
				err, tc.want)
		}
	}
}

func TestSpecFactsRoutes(t *testing.T) {
	s := &spec{name: "pdp-container", doc: map[string]any{"paths": map[string]any{
		"/facts/users":           map[string]any{"post": map[string]any{}},
		"/facts/users/{user_id}": map[string]any{"put": map[string]any{}, "patch": nil},
		"/allowed":               map[string]any{"post": map[string]any{}},
		"/v2/facts/x":            map[string]any{"post": map[string]any{}},
	}}}
	got := specFactsRoutes(s)
	want := []string{
		"PATCH /facts/users/{user_id}", "POST /facts/users",
		"PUT /facts/users/{user_id}",
	}
	if !slices.Equal(got, want) {
		t.Errorf("specFactsRoutes = %q, want %q", got, want)
	}
	if got := specFactsRoutes(&spec{doc: map[string]any{}}); got != nil {
		t.Errorf("specFactsRoutes of a spec without paths = %q, want none", got)
	}
}

func TestAPIRouteAndRouteOf(t *testing.T) {
	if got := apiRoute("PUT /facts/users/{user_id}"); got !=
		"PUT /v2/facts/{proj_id}/{env_id}/users/{user_id}" {
		t.Errorf("apiRoute = %q", got)
	}
	for op, want := range map[*rest.Op]string{
		{Method: "PUT", Route: apiFactsPrefix + "users/{user_id}"}: "PUT /facts/users/{user_id}",
		{Method: "POST", Route: "/allowed"}:                        "POST /allowed",
		{Method: "GET", Route: "/v2/facts/{proj_id}/users"}:        "GET /v2/facts/{proj_id}/users",
	} {
		if got := routeOf(*op); got != want {
			t.Errorf("routeOf(%s %s) = %q, want %q", op.Method, op.Route, got, want)
		}
	}
	keys := slices.Collect(maps.Keys(rowsByRoute([]coverageRow{
		{"A /a", "planned"},
		{"B /b", "covered"},
	})))
	slices.Sort(keys)
	if !slices.Equal(keys, []string{"A /a", "B /b"}) {
		t.Errorf("rowsByRoute has the routes %q", keys)
	}
}
