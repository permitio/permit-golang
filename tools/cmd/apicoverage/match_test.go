package main

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// mustTemplate parses a template that a test knows is valid.
func mustTemplate(t *testing.T, path string) template {
	t.Helper()
	tmpl, err := parseTemplate(path)
	if err != nil {
		t.Fatal(err)
	}
	return tmpl
}

func TestParseTemplate(t *testing.T) {
	for path, want := range map[string]template{
		"/allowed": {segments: []segment{{literal: "allowed"}}},
		"/v2/{proj_id}/x": {segments: []segment{
			{literal: "v2"}, {param: true}, {literal: "x"},
		}},
		"/facts/{full_path}": {segments: []segment{{literal: "facts"}, {param: true}}, rest: true},
		"/{full_path}/x":     {segments: []segment{{param: true}, {literal: "x"}}},
		"/v2/email_templates/": {segments: []segment{
			{literal: "v2"}, {literal: "email_templates"}, {literal: ""},
		}},
	} {
		got, err := parseTemplate(path)
		if err != nil {
			t.Errorf("parseTemplate(%q) returned %v", path, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseTemplate(%q) = %+v, want %+v", path, got, want)
		}
	}
	for path, want := range map[string]string{
		"":         "does not start with / and a segment",
		"/":        "does not start with / and a segment",
		"v2/x":     "does not start with /",
		"/v2//x":   `has an empty segment`,
		"/v2/{x":   `has the segment "{x"`,
		"/v2/x}":   `has the segment "x}"`,
		"/v2/{}":   `has the segment "{}"`,
		"/v2/a{x}": `has the segment "a{x}"`,
		"/v2/{{x}": `has the segment "{{x}"`,
	} {
		if _, err := parseTemplate(path); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("parseTemplate(%q) returned %v, want an error that contains %q", path, err,
				want)
		}
	}
}

func TestParseRoute(t *testing.T) {
	for route, want := range map[string][2]string{
		"GET /v2/x":          {"GET", "/v2/x"},
		"ANY /facts/{x}":     {"ANY", "/facts/{x}"},
		"OPTIONS /callbacks": {"OPTIONS", "/callbacks"},
	} {
		method, path, err := parseRoute(route)
		if err != nil || method != want[0] || path != want[1] {
			t.Errorf("parseRoute(%q) = %q, %q, %v; want %q", route, method, path, err, want)
		}
	}
	for _, route := range []string{"", "GET", "get /v2/x", "FETCH /v2/x", "GET  /v2/x", "GET v2"} {
		if _, _, err := parseRoute(route); err == nil {
			t.Errorf("parseRoute(%q) returned no error", route)
		}
	}
}

func TestSplitPath(t *testing.T) {
	for path, want := range map[string][]string{
		"/v2/x":             {"v2", "x"},
		"/v2/a%2Fb":         {"v2", "a/b"},
		"/v2/%66acts/":      {"v2", "facts", ""},
		"/":                 {""},
		"/v2//x":            {"v2", "", "x"},
		"/v2/a%20b/%7Bx%7D": {"v2", "a b", "{x}"},
	} {
		got, err := splitPath(path)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("splitPath(%q) = %q, %v; want %q", path, got, err, want)
		}
	}
	for _, path := range []string{"", "v2/x", "/v2/%zz", "/v2/%"} {
		if _, err := splitPath(path); err == nil {
			t.Errorf("splitPath(%q) returned no error", path)
		}
	}
}

func TestTemplateMatches(t *testing.T) {
	for _, tc := range []struct {
		template, path string
		want           bool
	}{
		{"/v2/users/{id}", "/v2/users/u", true},
		{"/v2/users/{id}", "/v2/users/a%2Fb", true},
		{"/v2/users/{id}", "/v2/users/", false},
		{"/v2/users/{id}", "/v2/users", false},
		{"/v2/users/{id}", "/v2/users/u/x", false},
		{"/v2/users/{id}", "/v2/user/u", false},
		{"/v2/users/{id}", "/v2/%75sers/u", true},
		{"/v2/users", "/v2/users/", false},
		{"/v2/templates/", "/v2/templates/", true},
		{"/v2/templates/", "/v2/templates", false},
		{"/facts/{full_path}", "/facts/users", true},
		{"/facts/{full_path}", "/facts/users/u/roles", true},
		{"/facts/{full_path}", "/facts", false},
		{"/facts/{full_path}", "/facts/", false},
		{"/facts/{full_path}", "/facts/users//x", false},
		{"/facts/{full_path}", "/fact/users", false},
		{"/{full_path}/x", "/a/x", true},
		{"/{full_path}/x", "/a/b/x", false},
	} {
		segments, err := splitPath(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if got := mustTemplate(t, tc.template).matches(segments); got != tc.want {
			t.Errorf("%q matches %q: %v, want %v", tc.template, tc.path, got, tc.want)
		}
	}
}

func TestMoreLiteral(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"/groups/direct", "/groups/{key}", true},
		{"/groups/{key}", "/groups/direct", false},
		{"/a/b/{x}", "/a/{x}/c", true},
		{"/a/{x}/c", "/a/b/{x}", false},
		{"/facts/users", "/facts/{full_path}", true},
		{"/facts/{full_path}", "/facts/users", false},
		{"/facts/{x}", "/facts/{full_path}", true},
		{"/facts/{full_path}", "/facts/{x}", false},
		{"/a/{x}", "/a/{y}", false},
		{"/a/{x}/{y}", "/{x}/b/c", false},
		{"/{x}/b/c", "/a/{x}/{y}", true},
	} {
		if got := moreLiteral(mustTemplate(t, tc.a), mustTemplate(t, tc.b)); got != tc.want {
			t.Errorf("moreLiteral(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestMountResolve(t *testing.T) {
	m := mount{prefix: mustTemplate(t, "/v2/facts/{proj_id}/{env_id}"), sameAs: []string{"facts"}}
	for path, want := range map[string][]string{
		"/v2/facts/p/e/users":   {"facts", "users"},
		"/v2/facts/p/e/users/u": {"facts", "users", "u"},
	} {
		segments, err := splitPath(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := m.resolve(segments)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("resolve(%q) = %q, %v; want %q", path, got, err, want)
		}
	}
	for _, path := range []string{
		"/v2/facts/p/e", "/v2/facts//e/users", "/v2/fact/p/e/users",
		"/facts/users",
	} {
		segments, err := splitPath(path)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := m.resolve(segments); err == nil {
			t.Errorf("resolve(%q) = %q, want errNoMount", path, got)
		}
	}
}

// TestMatch checks which row of the fixture each request matches.
func TestMatch(t *testing.T) {
	f := newFixture(t)
	in, err := readInputs(options{specs: f.specs, opMap: f.opMap, records: []string{f.wire}})
	if err != nil {
		t.Fatal(err)
	}
	c := &checker{in: in}
	for _, tc := range []struct {
		target, method, path string
		want                 string
	}{
		{"api", "GET", "/v2/schema/p/e/groups/direct", "GET /v2/schema/{proj_id}/{env_id}/" +
			"groups/direct"},
		{"api", "GET", "/v2/schema/p/e/groups/g", "GET /v2/schema/{proj_id}/{env_id}/groups/" +
			"{group_instance_key}"},
		{"api", "GET", "/v2/schema/p/e/groups/a%2Fb", "GET /v2/schema/{proj_id}/{env_id}/" +
			"groups/{group_instance_key}"},
		{"api", "GET", "/v2/schema/p/e/groups/direct/users", "GET /v2/schema/{proj_id}/" +
			"{env_id}/groups/{group_instance_key}/users"},
		{"api", "GET", "/v2/schema/p/e/groups/", ""},
		{"api", "GET", "/v2/facts/p/e/email_templates/", "GET /v2/facts/{proj_id}/{env_id}/" +
			"email_templates/"},
		{"api", "GET", "/v2/facts/p/e/email_templates", ""},
		{"api", "GET", "/v2/%66acts/p/e/users/u", "GET /v2/facts/{proj_id}/{env_id}/users/" +
			"{user_id}"},
		{"api", "POST", "/v2/api-key/scope", ""},
		{"api", "POST", "/v2/auth/elements_login_as", "POST /v2/auth/elements_login_as"},
		{"api", "POST", "/allowed", ""},
		{"api", "GET", "/v2/x/%zz", ""},
		{"pdp", "POST", "/allowed", "POST /allowed"},
		{"pdp", "POST", "/facts/users", "POST /facts/users"},
		{"pdp", "GET", "/facts/users/u", "ANY /facts/{full_path}"},
		{"pdp", "DELETE", "/facts/users/u/roles", "ANY /facts/{full_path}"},
		{"pdp", "POST", "/v2/facts/p/e/users", "POST /facts/users"},
		{"pdp", "GET", "/v2/facts/p/e/tenants/t/users", "ANY /facts/{full_path}"},
		{"pdp", "OPTIONS", "/facts/users/u", ""},
		{"pdp", "GET", "/facts", ""},
		{"pdp", "GET", "/health", "GET /health"},
		{"pdp", "GET", "/v2/api-key/scope", ""},
	} {
		got := c.match(request{target: tc.target, method: tc.method, path: tc.path})
		route := ""
		if got != nil {
			route = got.Route
		}
		if route != tc.want {
			t.Errorf("%s %s to the %s matched %q, want %q", tc.method, tc.path, tc.target,
				route, tc.want)
		}
	}
}

func TestRowMatchesMethod(t *testing.T) {
	get := &row{method: "GET"}
	anyRow := &row{method: anyMethod}
	listed := &known{methods: []string{"GET", "POST"}}
	for _, tc := range []struct {
		r      *row
		method string
		k      *known
		want   bool
	}{
		{get, "GET", nil, true},
		{get, "POST", nil, false},
		{anyRow, "POST", listed, true},
		{anyRow, "PUT", listed, false},
		{anyRow, "PUT", nil, true},
		{anyRow, "PUT", &known{}, true},
	} {
		if got := tc.r.matchesMethod(tc.method, tc.k); got != tc.want {
			t.Errorf("%s matches %s with %v: %v, want %v", tc.r.method, tc.method, tc.k, got,
				tc.want)
		}
	}
}

func TestShape(t *testing.T) {
	for route, want := range map[string]string{
		"GET /v2/users/{user_id}":            "GET /v2/users/{}",
		"GET /v2/{proj_id}/x/{id}/":          "GET /v2/{}/x/{}/",
		"ANY /facts/{full_path}":             "ANY /facts/{}",
		"GET /v2/users":                      "GET /v2/users",
		"DELETE /v2/{a}/{b}/roles/{role_id}": "DELETE /v2/{}/{}/roles/{}",
	} {
		if got := shape(route); got != want {
			t.Errorf("shape(%q) = %q, want %q", route, got, want)
		}
	}
}

func TestV1SeedAllows(t *testing.T) {
	seed := v1Seed{
		routes: map[string]string{"get_user": "GET /v2/users/{user_id}"},
		fixes:  map[string][]string{"GET /v2/users/{}": {"GET /v2/members/{}"}},
	}
	for _, tc := range []struct {
		id, route string
		want      bool
	}{
		{"get_user", "GET /v2/users/{user_id}", true},
		{"get_user", "GET /v2/users/{id}", true},
		{"get_user", "GET /v2/members/{member_key}", true},
		{"get_user", "GET /v2/users/{user_id}/x", false},
		{"get_user", "GET /v2/users/u", false},
		{"get_user", "POST /v2/users/{user_id}", false},
		{"get_user", "POST /v2/members/{member_key}", false},
		{"list_users", "GET /v2/anything", true},
	} {
		v1, got := seed.allows(tc.id, tc.route)
		if got != tc.want {
			t.Errorf("allows(%q, %q) = %v, want %v", tc.id, tc.route, got, tc.want)
		}
		if tc.id == "get_user" && v1 != "GET /v2/users/{user_id}" {
			t.Errorf("allows(%q, %q) returned v1's route %q", tc.id, tc.route, v1)
		}
	}
}

func TestStageOf(t *testing.T) {
	for _, tc := range []struct {
		op   openAPIOperation
		want string
	}{
		{openAPIOperation{}, stageGA},
		{openAPIOperation{Tags: []string{"Users"}}, stageGA},
		{openAPIOperation{Tags: []string{"Groups", "Policy Guards (EAP)"}}, stageEAP},
		{openAPIOperation{Tags: []string{"OPAL Data ( EAP )"}}, stageEAP},
		{openAPIOperation{Tags: []string{"EAPs"}}, stageGA},
		{
			openAPIOperation{Tags: []string{"Access Requests (EAP)"}, Deprecated: true},
			stageDeprecated,
		},
	} {
		if got := stageOf(tc.op); got != tc.want {
			t.Errorf("stageOf(%+v) = %q, want %q", tc.op, got, tc.want)
		}
	}
}

func TestCount(t *testing.T) {
	for n, want := range map[int]string{0: "0 requests", 1: "1 request", 2: "2 requests"} {
		if got := count(n, "request"); got != want {
			t.Errorf("count(%d) = %q, want %q", n, got, want)
		}
	}
	if got := cell("a | b\nc"); got != `a \| b c` {
		t.Errorf("cell escaped %q", got)
	}
}

// coveredRequests returns a wire request line for each covered row of the coverage.yaml in
// specs: the row's method, or the first that an ANY row takes, its path as the template, and
// that path with the parameter's name in each parameter.
func coveredRequests(t *testing.T, specs string) []string {
	t.Helper()
	cov, err := readCoverage(filepath.Join(specs, coverageFile))
	if err != nil {
		t.Fatal(err)
	}
	inv, err := readInventory(specs, "")
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, r := range append(slices.Clone(cov.ControlPlane), cov.PDP...) {
		if r.Status != statusCovered {
			continue
		}
		target, listed := targetAPI, inv.controlPlane
		if r.section == sectionPDP {
			target, listed = targetPDP, inv.pdp
		}
		i := slices.IndexFunc(routeMethods, func(m string) bool {
			return r.matchesMethod(m, listed[r.Route])
		})
		if i < 0 {
			t.Fatalf("%s takes none of the methods %v", r.name(), routeMethods)
		}
		path := paramSegment.ReplaceAllStringFunc(r.path, func(p string) string {
			return strings.Trim(p, "{}")
		})
		lines = append(lines, wireLine(target, routeMethods[i], r.path, path))
	}
	return lines
}

func TestCoveredRequests(t *testing.T) {
	f := newFixture(t)
	f.edit(coverageFile, "    status: planned\n    sdk_only: \"The catch-all",
		"    status: covered\n    sdk_only: \"The catch-all")
	want := []string{
		wireLine("api", "GET", "/v2/facts/{proj_id}/{env_id}/users/{user_id}",
			"/v2/facts/proj_id/env_id/users/user_id"),
		wireLine("pdp", "POST", "/allowed", "/allowed"),
		wireLine("pdp", "GET", "/facts/{full_path}", "/facts/full_path"),
	}
	if got := coveredRequests(t, f.specs); !slices.Equal(got, want) {
		t.Errorf("coveredRequests returned\n%s\nwant\n%s", strings.Join(got, "\n"),
			strings.Join(want, "\n"))
	}
}

// TestCommittedCoverage checks tools/specs/coverage.yaml against the committed specs, with a
// record that holds a request for each covered row, as the wire tests of its methods send. Its
// counts are the sentinel of the classification.
func TestCommittedCoverage(t *testing.T) {
	f := newFixture(t)
	f.write(f.opMap, `{"methods": {}}`)
	specs := filepath.Join("..", "..", "specs")
	code, out := runApicoverage(t, "-specs", specs, "-map", f.opMap,
		f.record("r.jsonl", coveredRequests(t, specs)...))
	if code != exitPassed {
		t.Fatalf("apicoverage exited %d on tools/specs, want 0:\n%s", code, out)
	}
	for _, want := range []string{
		"control-plane: 264 rows: 6 covered, 148 planned, 109 excluded, 1 deferred\n",
		"  GA 213: 6 covered, 147 planned, 59 excluded, 1 deferred, 0 undecided\n",
		"  EAP 42: 0 covered, 0 planned, 42 excluded, 0 deferred, 0 undecided\n",
		"  deprecated 8: 0 covered, 0 planned, 8 excluded, 0 deferred, 0 undecided\n",
		"  SDK-only 1: 0 covered, 1 planned, 0 excluded, 0 deferred\n",
		"pdp: 47 rows: 8 covered, 12 planned, 21 excluded, 6 deferred\n",
		"  GA 45: 8 covered, 11 planned, 20 excluded, 6 deferred, 0 undecided\n",
		"  deprecated 1: 0 covered, 0 planned, 1 excluded, 0 deferred, 0 undecided\n",
		"PASS: 14 of 311 rows covered",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("apicoverage printed\n%s\nwant %q", out, want)
		}
	}
}

// TestRepositoryRecordFormat reads the golden record of internal/testhook, the writer of the
// records, which is outside this module.
func TestRepositoryRecordFormat(t *testing.T) {
	file := filepath.Join("..", "..", "..", "internal", "testhook", "testdata", "record.jsonl")
	rec, err := readRecord(file, []string{modeWire, modeDryRun, modeLive, modeLocal})
	if err != nil {
		t.Fatal(err)
	}
	var modes []string
	for _, r := range rec.requests {
		modes = append(modes, r.mode)
	}
	if !slices.Equal(modes, []string{modeWire, modeDryRun, modeLive, modeLocal}) ||
		rec.tests != 4 {
		t.Errorf("the golden record holds requests of the modes %q and %d tests, want one of "+
			"each mode and 4 tests", modes, rec.tests)
	}
}
