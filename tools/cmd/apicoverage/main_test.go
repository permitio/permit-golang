package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture is a copy of the fixtures in testdata, in a temporary directory, which a test may
// change.
type fixture struct {
	t     *testing.T
	dir   string
	specs string
	opMap string
	wire  string
}

// newFixture copies testdata/specs, testdata/operation-map.json and testdata/wire.jsonl.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	f := &fixture{
		t: t, dir: dir, specs: filepath.Join(dir, "specs"),
		opMap: filepath.Join(dir, "operation-map.json"), wire: filepath.Join(dir, "wire.jsonl"),
	}
	if err := os.CopyFS(f.specs, os.DirFS(filepath.Join("testdata", "specs"))); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"operation-map.json", "wire.jsonl"} {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		f.write(filepath.Join(dir, name), string(data))
	}
	return f
}

// write writes a file of the fixture.
func (f *fixture) write(file, content string) {
	f.t.Helper()
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

// edit replaces the one occurrence of old in a file of the specs directory with replacement.
func (f *fixture) edit(name, old, replacement string) {
	f.t.Helper()
	file := filepath.Join(f.specs, name)
	data, err := os.ReadFile(file)
	if err != nil {
		f.t.Fatal(err)
	}
	if n := strings.Count(string(data), old); n != 1 {
		f.t.Fatalf("%s holds %q %d times, want once", name, old, n)
	}
	f.write(file, strings.Replace(string(data), old, replacement, 1))
}

// record writes a record of the requests and returns its path: the header, the request lines
// and a trailer of a run that passed.
func (f *fixture) record(name string, requests ...string) string {
	f.t.Helper()
	lines := append([]string{`{"type":"header","format":1}`}, requests...)
	lines = append(lines, `{"type":"session","exit_status":0,"tests":1}`)
	return f.raw(name, strings.Join(lines, "\n")+"\n")
}

// raw writes a record of exactly content and returns its path.
func (f *fixture) raw(name, content string) string {
	f.t.Helper()
	file := filepath.Join(f.dir, name)
	f.write(file, content)
	return file
}

// run runs apicoverage on the fixture with the arguments, which follow -specs and -map, and
// returns its exit code and output.
func (f *fixture) run(args ...string) (int, string) {
	f.t.Helper()
	return runApicoverage(f.t, append([]string{"-specs", f.specs, "-map", f.opMap}, args...)...)
}

// runApicoverage runs apicoverage with the arguments and returns its exit code and output.
func runApicoverage(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String() + stderr.String()
}

// wireLine is a wire request line of a record.
func wireLine(target, method, template, path string) string {
	return `{"type":"request","mode":"wire","target":"` + target + `","method":"` + method +
		`","template":"` + template + `","path":"` + path + `","status":200,"test":"TestX"}`
}

// The wire requests of testdata/wire.jsonl, for records that change it.
var (
	getUser = wireLine("api", "GET", "/v2/facts/{proj_id}/{env_id}/users/{user_id}",
		"/v2/facts/p/e/users/u")
	checkAllowed = wireLine("pdp", "POST", "/allowed", "/allowed")
)

func TestRunPasses(t *testing.T) {
	f := newFixture(t)
	code, out := f.run(f.wire)
	if code != exitPassed {
		t.Fatalf("apicoverage exited %d, want 0:\n%s", code, out)
	}
	want := "apicoverage: coverage.yaml and the specs in " + f.specs + "\n" +
		"records: 1 record file, 2 tests, 2 requests: 2 wire, 0 dryrun, 0 live\n" +
		"control-plane: 11 rows: 1 covered, 5 planned, 4 excluded, 1 deferred\n" +
		"  GA 8: 1 covered, 4 planned, 2 excluded, 1 deferred, 0 undecided\n" +
		"  EAP 1: 0 covered, 0 planned, 1 excluded, 0 deferred, 0 undecided\n" +
		"  deprecated 1: 0 covered, 0 planned, 1 excluded, 0 deferred, 0 undecided\n" +
		"  SDK-only 1: 0 covered, 1 planned, 0 excluded, 0 deferred\n" +
		"pdp: 7 rows: 1 covered, 2 planned, 3 excluded, 1 deferred\n" +
		"  GA 5: 1 covered, 1 planned, 2 excluded, 1 deferred, 0 undecided\n" +
		"  deprecated 1: 0 covered, 0 planned, 1 excluded, 0 deferred, 0 undecided\n" +
		"  SDK-only 1: 0 covered, 1 planned, 0 excluded, 0 deferred\n" +
		"e2e of the 2 covered rows: 0 cloud, 0 local, 0 advisory, 0 dry run, 2 none; dry run: " +
		"the records hold no live request, and a request of a scenario run offline is no " +
		"end-to-end evidence\n" +
		"covered: control-plane GET /v2/facts/{proj_id}/{env_id}/users/{user_id} (get_user): " +
		"UsersService.Get; 1 request; e2e none\n" +
		"covered: pdp POST /allowed (is_allowed_allowed_post): Client.Check; 1 request; e2e " +
		"none\n" +
		"PASS: 2 of 18 rows covered, every GA operation decided, and the records agree with " +
		"coverage.yaml\n"
	if out != want {
		t.Errorf("apicoverage printed\n%s\nwant\n%s", out, want)
	}
}

// TestRunFails checks one change to the fixtures per rule that fails the check, each with the
// problem it reports.
func TestRunFails(t *testing.T) {
	const cp, pdp = "control-plane.json", "pdp-container.json"
	for _, tc := range []struct {
		name   string
		change func(f *fixture) []string
		want   string
	}{
		{
			name: "an undecided GA operation",
			change: func(f *fixture) []string {
				f.edit(cp, `"paths": {`, `"paths": {"/v2/tenants": {"get": {"operationId": `+
					`"list_tenants", "tags": ["Tenants"]}},`)
				return []string{f.wire}
			},
			want: "control-plane GET /v2/tenants (list_tenants): a GA operation without a row " +
				"in coverage.yaml; add a row that decides it",
		},
		{
			name: "an undecided GA route that only the extras list",
			change: func(f *fixture) []string {
				f.edit("pdp-container.extra.json", `"routes": [`, `"routes": [{"methods": `+
					`["GET"], "path": "/healthy"},`)
				return []string{f.wire}
			},
			want: "pdp GET /healthy (no operationId): a GA operation without a row",
		},
		{
			name: "a planned operation that a record reaches",
			change: func(f *fixture) []string {
				return []string{f.record("r.jsonl", getUser, checkAllowed,
					wireLine("api", "GET", "/v2/api-key/scope", "/v2/api-key/scope"))}
			},
			want: "control-plane GET /v2/api-key/scope: stale: it is planned, but the records " +
				"hold 1 request to it; mark it covered",
		},
		{
			name: "a planned operation that a wire case sends",
			change: func(f *fixture) []string {
				f.write(f.opMap, `{"methods": {"Client.Check": ["is_allowed_allowed_post"], `+
					`"UsersService.Get": ["get_user"], "Client.Scope": ["get_api_key_scope"]}}`)
				return []string{f.wire}
			},
			want: "control-plane GET /v2/api-key/scope: stale: it is planned, but the wire " +
				"cases of Client.Scope send it; mark it covered",
		},
		{
			name: "an excluded operation gone from the spec",
			change: func(f *fixture) []string {
				f.edit(cp, "    \"/v2/members\": {\n      \"get\": {\n        \"operationId\": "+
					"\"list_organization_members\",\n        \"tags\": [\n          \"Members\"\n"+
					"        ]\n      }\n    },\n", "")
				return []string{f.wire}
			},
			want: "control-plane GET /v2/members: the specs no longer list the route; remove " +
				"the row",
		},
		{
			name: "a request to a test-only route",
			change: func(f *fixture) []string {
				return []string{f.record("r.jsonl", getUser, checkAllowed,
					wireLine("api", "GET", "/__test/echo", "/__test/echo"))}
			},
			want: "r.jsonl:4: a request to the test-only route GET /__test/echo; the record " +
				"must leave out test routes",
		},
		{
			name: "a route that differs from its v1 seed",
			change: func(f *fixture) []string {
				f.edit("v1-routes.json", `"path": "/v2/facts/{proj_id}/{env_id}/users/{user_id}"`,
					`"path": "/v2/users/{user_id}"`)
				return []string{f.wire}
			},
			want: "wire.jsonl:2: the request for get_user went to the route GET " +
				"/v2/facts/{proj_id}/{env_id}/users/{user_id}, but v1 sent GET " +
				"/v2/users/{user_id} (SPECS/v1-routes.json), and SPECS/v1-route-fixes.yaml lists " +
				"no such change; use v1's route, or add the change with its reason",
		},
		{
			name: "a template that does not fit its path",
			change: func(f *fixture) []string {
				return []string{f.record("r.jsonl", wireLine("api", "GET",
					"/v2/facts/{proj_id}/{env_id}/users", "/v2/facts/p/e/users/u"), checkAllowed)}
			},
			want: "r.jsonl:2: the template /v2/facts/{proj_id}/{env_id}/users does not fit the " +
				"path GET /v2/facts/p/e/users/u; record the route that the request was made for",
		},
		{
			name: "a template of another row",
			change: func(f *fixture) []string {
				return []string{f.record("r.jsonl", getUser, checkAllowed, wireLine("api", "GET",
					"/v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}",
					"/v2/schema/p/e/groups/direct"))}
			},
			want: "r.jsonl:4: the template /v2/schema/{proj_id}/{env_id}/groups/" +
				"{group_instance_key} of GET /v2/schema/p/e/groups/direct is the route of " +
				"control-plane GET /v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}, " +
				"but the path reaches control-plane GET /v2/schema/{proj_id}/{env_id}/groups/" +
				"direct; record the route that the request was made for",
		},
		{
			name: "a template of no row",
			change: func(f *fixture) []string {
				return []string{f.record("r.jsonl", getUser, wireLine("pdp", "POST", "/{x}",
					"/allowed"))}
			},
			want: "r.jsonl:3: the template /{x} of POST /allowed is the route of no row, but the " +
				"path reaches pdp POST /allowed",
		},
		{
			name: "a covered operation without a request",
			change: func(f *fixture) []string {
				return []string{f.record("r.jsonl", checkAllowed, checkAllowed)}
			},
			want: "control-plane GET /v2/facts/{proj_id}/{env_id}/users/{user_id}: untested: " +
				"it is covered, but the records hold no request to it",
		},
		{
			name: "a planned operation that only the local stack reaches",
			change: func(f *fixture) []string {
				local := f.raw("local.jsonl", `{"type":"header","format":1}`+"\n"+
					strings.Replace(wireLine("api", "GET", "/v2/api-key/scope",
						"/v2/api-key/scope"), `"wire"`, `"local"`, 1)+"\n"+
					`{"type":"session","exit_status":0,"tests":1}`+"\n")
				return []string{"-local", local, f.wire}
			},
			want: "control-plane GET /v2/api-key/scope: stale: it is planned, but the records " +
				"hold 1 request to it",
		},
		{
			name: "a covered operation that only the local stack reaches",
			change: func(f *fixture) []string {
				local := f.raw("local.jsonl", `{"type":"header","format":1}`+"\n"+
					strings.Replace(getUser, `"wire"`, `"local"`, 1)+"\n"+
					`{"type":"session","exit_status":0,"tests":1}`+"\n")
				return []string{"-local", local, f.record("r.jsonl", checkAllowed, checkAllowed)}
			},
			want: "untested: it is covered, but the records hold no request to it",
		},
		{
			name: "a stage change in coverage.yaml",
			change: func(f *fixture) []string {
				f.edit(coverageFile, `route: "GET /v2/api-key/scope", stage: GA`,
					`route: "GET /v2/api-key/scope", stage: EAP`)
				return []string{f.wire}
			},
			want: "control-plane GET /v2/api-key/scope: stage change: the spec marks it GA, and " +
				"coverage.yaml says EAP",
		},
		{
			name: "a stage change in the spec",
			change: func(f *fixture) []string {
				f.edit(cp, `"operationId": "list_direct_group",`,
					`"deprecated": true, "operationId": "list_direct_group",`)
				return []string{f.wire}
			},
			want: "groups/direct: stage change: the spec marks it deprecated, and coverage.yaml " +
				"says GA",
		},
		{
			name: "an EAP tag in the spec",
			change: func(f *fixture) []string {
				f.edit(cp, `"API Keys"`, `"API Keys (EAP)"`)
				return []string{f.wire}
			},
			want: "GET /v2/api-key/scope: stage change: the spec marks it EAP",
		},
		{
			name: "an unexplained SDK-only request",
			change: func(f *fixture) []string {
				return []string{f.record("r.jsonl", getUser, checkAllowed,
					wireLine("api", "PUT", "/v2/schema/{proj_id}/{env_id}/roles/{role_id}/parents",
						"/v2/schema/p/e/roles/r/parents"))}
			},
			want: "r.jsonl:4: an SDK-only request that no row explains: PUT " +
				"/v2/schema/p/e/roles/r/parents to the api (TestX); add a row with sdk_only",
		},
		{
			name: "a PDP route sent to the API",
			change: func(f *fixture) []string {
				return []string{f.record("r.jsonl", getUser, checkAllowed,
					wireLine("api", "POST", "/allowed", "/allowed"))}
			},
			want: "an SDK-only request that no row explains: POST /allowed to the api",
		},
		{
			name: "an unused sdk_only entry",
			change: func(f *fixture) []string {
				f.edit(coverageFile, "    status: planned\n    sdk_only: \"The login-as",
					"    status: covered\n    sdk_only: \"The login-as")
				return []string{f.record("r.jsonl", getUser, checkAllowed, checkAllowed)}
			},
			want: "control-plane POST /v2/auth/elements_login_as: an unused sdk_only entry: it " +
				"is covered, but the records hold no request to it",
		},
		{
			name: "sdk_only on a route that a spec lists",
			change: func(f *fixture) []string {
				f.edit(coverageFile, `/groups/direct", stage: GA, status: planned}`,
					`/groups/direct", stage: GA, status: planned, sdk_only: "x"}`)
				return []string{f.wire}
			},
			want: "groups/direct: the spec lists the route now; remove sdk_only",
		},
		{
			name: "a planned route that no spec lists, without sdk_only",
			change: func(f *fixture) []string {
				f.edit(coverageFile, "    sdk_only: \"The catch-all of the facts routes.\"\n", "")
				return []string{f.wire}
			},
			want: "pdp ANY /facts/{full_path}: no spec lists the route, so a planned row needs " +
				"sdk_only",
		},
		{
			name: "a reason outside the vocabulary",
			change: func(f *fixture) []string {
				f.edit(coverageFile, "status: excluded, reason: account-admin}",
					"status: excluded, reason: org-admin}")
				return []string{f.wire}
			},
			want: `control-plane GET /v2/members: the reason "org-admin" is not a key of ` +
				"reasons; use one of account-admin, dashboard-only, deprecated, eap, operator",
		},
		{
			name: "a reason that no row gives",
			change: func(f *fixture) []string {
				f.edit(coverageFile, "reasons:\n", "reasons:\n  spare: \"Not used.\"\n")
				return []string{f.wire}
			},
			want: "reasons: no excluded row gives spare; remove it",
		},
		{
			name: "an operation of the map without a row",
			change: func(f *fixture) []string {
				f.write(f.opMap, `{"methods": {"Client.Check": ["is_allowed_allowed_post"], `+
					`"UsersService.Get": ["get_user", "get_user_detail"]}}`)
				return []string{f.wire}
			},
			want: "testdata/operation-map.json: the wire cases of UsersService.Get send the op " +
				"get_user_detail, which no row of coverage.yaml has",
		},
		{
			name: "an excluded operation that a record reaches",
			change: func(f *fixture) []string {
				return []string{f.record("r.jsonl", getUser, checkAllowed,
					wireLine("api", "GET", "/v2/members", "/v2/members"))}
			},
			want: "control-plane GET /v2/members: it is excluded, but the records hold 1 " +
				"request to it; decide it again",
		},
		{
			name: "a deferred operation that a wire case sends",
			change: func(f *fixture) []string {
				f.write(f.opMap, `{"methods": {"Client.Check": ["is_allowed_allowed_post"], `+
					`"UsersService.Get": ["get_user"], "PDPsService.List": ["list_pdp_configs"]}}`)
				return []string{f.wire}
			},
			want: "configs: it is deferred, but the wire cases of PDPsService.List send it",
		},
		{
			name: "an op that is not the spec's operationId",
			change: func(f *fixture) []string {
				f.edit(coverageFile, "{op: list_direct_group,", "{op: list_direct_groups,")
				return []string{f.wire}
			},
			want: `groups/direct: the spec's operationId for the route is list_direct_group, ` +
				`not "list_direct_groups"; fix op`,
		},
		{
			name: "a planned row without an op",
			change: func(f *fixture) []string {
				f.edit(coverageFile, "  - op: elements_login_as\n    route:", "  - route:")
				return []string{f.wire}
			},
			want: "elements_login_as: a planned row needs an op; give a route that no spec " +
				"lists an op of its own",
		},
		{
			name: "an SDK-only op that is the spec's operationId of another route",
			change: func(f *fixture) []string {
				f.edit(coverageFile, "{op: list_direct_group,", "{")
				f.edit(coverageFile, "  - op: elements_login_as\n", "  - op: list_direct_group\n")
				return []string{f.wire}
			},
			want: "elements_login_as: its op list_direct_group is the operationId of another " +
				"route of the spec",
		},
		{
			name: "an SDK-only parameter that the spec lists now",
			change: func(f *fixture) []string {
				f.edit(cp, `"name": "search"`, `"name": "attr_color"`)
				return []string{f.wire}
			},
			want: "sdk_only_params: the spec lists attr_color for list_roles now; remove the " +
				"entry",
		},
		{
			name: "an SDK-only parameter that the spec lists by name",
			change: func(f *fixture) []string {
				f.edit(coverageFile, `param: "attr_*"`, `param: "search"`)
				return []string{f.wire}
			},
			want: "sdk_only_params: the spec lists search for list_roles now",
		},
		{
			name: "an SDK-only parameter of an operation the SDK does not send",
			change: func(f *fixture) []string {
				f.edit(coverageFile, "  - op: list_roles\n", "  - op: list_organization_members\n")
				return []string{f.wire}
			},
			want: "sdk_only_params: list_organization_members is excluded, so the SDK sends it " +
				"no parameter; remove attr_*",
		},
		{
			name: "an SDK-only parameter of no row",
			change: func(f *fixture) []string {
				f.edit(coverageFile, "  - op: list_roles\n", "  - op: list_widgets\n")
				return []string{f.wire}
			},
			want: "sdk_only_params: list_widgets names no row of coverage.yaml",
		},
		{
			name: "a PDP route through the mount that no row explains",
			change: func(f *fixture) []string {
				return []string{f.record("r.jsonl", getUser, checkAllowed,
					wireLine("pdp", "OPTIONS", "", "/v2/facts/p/e/users"))}
			},
			want: "an SDK-only request that no row explains: OPTIONS /v2/facts/p/e/users to the " +
				"pdp",
		},
		{
			name: "an undecided PDP operation",
			change: func(f *fixture) []string {
				f.edit(pdp, `"/callbacks": {`, `"/callbacks": {"post": {"operationId": "reg"},`)
				return []string{f.wire}
			},
			want: "pdp POST /callbacks (reg): a GA operation without a row",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			code, out := f.run(tc.change(f)...)
			if code != exitFailed {
				t.Errorf("apicoverage exited %d, want 1", code)
			}
			want := strings.ReplaceAll(tc.want, "SPECS", f.specs)
			if !strings.Contains(out, "\nproblem: ") || !strings.Contains(out, want) ||
				!strings.Contains(out, "\nFAIL: ") {
				t.Errorf("apicoverage printed\n%s\nwant a problem that contains %q", out, want)
			}
		})
	}
}

// TestRunReportsARequestOnce checks that a request that tests send many times is one problem,
// which names the first line that holds it.
func TestRunReportsARequestOnce(t *testing.T) {
	f := newFixture(t)
	f.edit("v1-routes.json", `"path": "/v2/facts/{proj_id}/{env_id}/users/{user_id}"`,
		`"path": "/v2/users/{user_id}"`)
	unknown := wireLine("api", "GET", "", "/v2/unknown")
	testRoute := wireLine("api", "GET", "", "/__test/x")
	misfit := wireLine("api", "GET", "/v2/facts/{proj_id}/{env_id}/users",
		"/v2/facts/p/e/users/u")
	code, out := f.run(f.record("r.jsonl", getUser, checkAllowed, unknown, unknown, testRoute,
		testRoute, misfit, misfit))
	if code != exitFailed {
		t.Fatalf("apicoverage exited %d, want 1:\n%s", code, out)
	}
	for _, want := range []string{
		"problem: " + filepath.Join(f.dir, "r.jsonl") + ":2: the request for get_user",
		"problem: " + filepath.Join(f.dir, "r.jsonl") + ":4: an SDK-only request",
		"problem: " + filepath.Join(f.dir, "r.jsonl") + ":6: a request to the test-only route",
		"problem: " + filepath.Join(f.dir, "r.jsonl") + ":8: the template",
		"FAIL: 4 problems with coverage.yaml, the specs and the records\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("apicoverage printed\n%s\nwant %q", out, want)
		}
	}
}

func TestRunNamesACoveredRowWithoutAWireCase(t *testing.T) {
	f := newFixture(t)
	f.write(f.opMap, `{"methods": {"Client.Check": ["is_allowed_allowed_post"]}}`)
	code, out := f.run(f.wire)
	if code != exitPassed || !strings.Contains(out, "(get_user): no wire case names it; 1 "+
		"request; e2e none\n") {
		t.Errorf("apicoverage exited %d and printed\n%s\nwant 0 and the row without a method",
			code, out)
	}
}

// TestRunLeavesOutEmptyLines checks that a section without SDK-only rows has no line for them.
func TestRunLeavesOutEmptyLines(t *testing.T) {
	f := newFixture(t)
	f.edit(
		coverageFile,
		"  - op: elements_login_as\n    route: \"POST /v2/auth/elements_login_as\"\n"+
			"    stage: GA\n    status: planned\n    sdk_only: \"The login-as route is not in the "+
			"spec.\"\n",
		"",
	)
	code, out := f.run(f.wire)
	if code != exitPassed {
		t.Fatalf("apicoverage exited %d, want 0:\n%s", code, out)
	}
	want := "  deprecated 1: 0 covered, 0 planned, 1 excluded, 0 deferred, 0 undecided\n" +
		"pdp: 7 rows"
	if !strings.Contains(out, want) || strings.Count(out, "SDK-only") != 1 {
		t.Errorf("apicoverage printed\n%s\nwant the SDK-only line of pdp only", out)
	}
}

// TestRunCountsAStageOverride checks that a row counts at the stage of its stage_override, and
// that the override is no stage change.
func TestRunCountsAStageOverride(t *testing.T) {
	f := newFixture(t)
	f.edit(coverageFile, "    stage_override:\n      stage: GA\n",
		"    stage_override:\n      stage: EAP\n")
	code, out := f.run(f.wire)
	if code != exitPassed {
		t.Fatalf("apicoverage exited %d, want 0:\n%s", code, out)
	}
	for _, want := range []string{
		"  GA 7: 1 covered, 3 planned, 2 excluded, 1 deferred, 0 undecided\n",
		"  EAP 2: 0 covered, 1 planned, 1 excluded, 0 deferred, 0 undecided\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("apicoverage printed\n%s\nwant %q", out, want)
		}
	}
}

func TestRunListsUndecidedOperationsThatAreNotGA(t *testing.T) {
	f := newFixture(t)
	f.edit("control-plane.json", `"paths": {`, `"paths": {"/v2/guards": {"get": {"operationId": `+
		`"list_guards", "tags": ["Guards (EAP)"]}}, "/v2/old": {"get": {"deprecated": true, `+
		`"operationId": "old"}},`)
	code, out := f.run(f.wire)
	if code != exitPassed {
		t.Fatalf("apicoverage exited %d, want 0:\n%s", code, out)
	}
	for _, want := range []string{
		"  EAP 1: 0 covered, 0 planned, 1 excluded, 0 deferred, 1 undecided\n",
		"  deprecated 1: 0 covered, 0 planned, 1 excluded, 0 deferred, 1 undecided\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("apicoverage printed\n%s\nwant %q", out, want)
		}
	}
}

// TestRunAcceptsTheRoutesOfV1RouteFixes checks that a route that v1-route-fixes.yaml lists as
// the one v2 sends instead of v1's passes, whatever the names of the parameters in either file.
func TestRunAcceptsTheRoutesOfV1RouteFixes(t *testing.T) {
	f := newFixture(t)
	f.edit("v1-routes.json", `"path": "/v2/facts/{proj_id}/{env_id}/users/{user_id}"`,
		`"path": "/v2/users/{user_id}"`)
	f.edit("v1-route-fixes.yaml", "        v1: GET /v2/facts/{proj_id}/{env_id}/users/{user_id}\n",
		"        v1: GET /v2/users/{id}\n")
	code, out := f.run(f.record("r.jsonl", checkAllowed, getUser,
		wireLine("api", "GET", "", "/v2/facts/p/e/users/v")))
	if code != exitPassed {
		t.Errorf("apicoverage exited %d, want 0:\n%s", code, out)
	}
}

// TestRunIgnoresTheNamesInATemplate checks that a template whose parameters have other names
// than the spec's, or that a test wrote with literal segments, passes the v1 route rule.
func TestRunIgnoresTheNamesInATemplate(t *testing.T) {
	f := newFixture(t)
	code, out := f.run(f.record("r.jsonl", checkAllowed,
		wireLine("api", "GET", "/v2/facts/{p}/{e}/users/{id}", "/v2/facts/p/e/users/u"),
		wireLine("api", "GET", "/v2/facts/p/e/users/u", "/v2/facts/p/e/users/u")))
	if code != exitPassed {
		t.Errorf("apicoverage exited %d, want 0:\n%s", code, out)
	}
}

// TestRunReportsTheActionGroupTemplateOnAnActionPath records a request for the route that
// deletes an action group on the path of an action, against the committed specs and
// v1-route-fixes.yaml, whose action-group-delete fix lists the action group's route.
func TestRunReportsTheActionGroupTemplateOnAnActionPath(t *testing.T) {
	f := newFixture(t)
	f.write(f.opMap, `{"methods": {}}`)
	code, out := runApicoverage(t, "-specs", filepath.Join("..", "..", "specs"), "-map",
		f.opMap, f.record("r.jsonl", wireLine("api", "DELETE",
			"/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/action_groups/{action_group_id}",
			"/v2/schema/p/e/resources/r/actions/g")))
	want := "r.jsonl:2: the template /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/" +
		"action_groups/{action_group_id} does not fit the path DELETE " +
		"/v2/schema/p/e/resources/r/actions/g"
	if code != exitFailed || !strings.Contains(out, want) {
		t.Errorf("apicoverage exited %d and printed\n%s\nwant 1 and %q", code, out, want)
	}
}

// TestDryRunIsNoLiveEvidence checks that a request of a scenario run offline does not make a
// row covered in the cloud, and that a live one does.
func TestDryRunIsNoLiveEvidence(t *testing.T) {
	f := newFixture(t)
	dryRun := strings.Replace(getUser, `"wire"`, `"dryrun"`, 1)
	code, out := f.run(f.record("r.jsonl", getUser, checkAllowed, dryRun))
	if code != exitPassed {
		t.Fatalf("apicoverage exited %d, want 0:\n%s", code, out)
	}
	for _, want := range []string{
		"e2e of the 2 covered rows: 0 cloud, 0 local, 0 advisory, 1 dry run, 1 none; dry run: " +
			"the records hold no live request",
		"(get_user): UsersService.Get; 2 requests; e2e dry run\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("apicoverage printed\n%s\nwant %q", out, want)
		}
	}
	live := strings.Replace(getUser, `"wire"`, `"live"`, 1)
	code, out = f.run(f.record("live.jsonl", getUser, checkAllowed, dryRun, live))
	if code != exitPassed {
		t.Fatalf("apicoverage exited %d, want 0:\n%s", code, out)
	}
	for _, want := range []string{
		"e2e of the 2 covered rows: 1 cloud, 0 local, 0 advisory, 0 dry run, 1 none\n",
		"(get_user): UsersService.Get; 3 requests; e2e cloud\n",
		"records: 1 record file, 1 test, 4 requests: 2 wire, 1 dryrun, 1 live\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("apicoverage printed\n%s\nwant %q, and no dry-run note", out, want)
		}
	}
}

func TestE2EStates(t *testing.T) {
	f := newFixture(t)
	local := f.raw("local.jsonl", `{"type":"header","format":1}`+"\n"+
		strings.Replace(getUser, `"wire"`, `"local"`, 1)+"\n"+
		`{"type":"session","exit_status":0,"tests":3}`+"\n")
	advisory := f.raw("advisory.txt", "# Advisory scenarios.\n\nClient.Check needs a PDP "+
		"image that may lag\n")
	code, out := f.run("-local", local, "-advisory", advisory, f.wire)
	if code != exitPassed {
		t.Fatalf("apicoverage exited %d, want 0:\n%s", code, out)
	}
	for _, want := range []string{
		"e2e of the 2 covered rows: 0 cloud, 1 local, 1 advisory, 0 dry run, 0 none;",
		"(get_user): UsersService.Get; 1 request; e2e local\n",
		"(is_allowed_allowed_post): Client.Check; 1 request; e2e advisory\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("apicoverage printed\n%s\nwant %q", out, want)
		}
	}
}

func TestRunLive(t *testing.T) {
	f := newFixture(t)
	live := filepath.Join(f.dir, "live")
	if err := os.Mkdir(live, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"control-plane.json", "pdp-cloud.json"} {
		data, err := os.ReadFile(filepath.Join(f.specs, name))
		if err != nil {
			t.Fatal(err)
		}
		f.write(filepath.Join(live, name), string(data))
	}
	code, out := f.run("-live", live, f.wire)
	if code != exitPassed || !strings.Contains(out, "apicoverage: coverage.yaml and the specs "+
		"in "+f.specs+", with the live control-plane and cloud PDP specs in "+live+"\n") {
		t.Fatalf("apicoverage exited %d, want 0 and the live directory named:\n%s", code, out)
	}
	cloud := filepath.Join(live, "pdp-cloud.json")
	data, err := os.ReadFile(cloud)
	if err != nil {
		t.Fatal(err)
	}
	f.write(cloud, strings.Replace(string(data), `"paths": {`, `"paths": {"/allowed/bulk": `+
		`{"post": {"operationId": "bulk_allowed_handler", "tags": ["authz"]}},`, 1))
	code, out = f.run("-live", live, f.wire)
	if code != exitFailed || !strings.Contains(out, "problem: pdp POST /allowed/bulk "+
		"(bulk_allowed_handler): a GA operation without a row") {
		t.Errorf("apicoverage exited %d, want 1 for the live spec's new operation:\n%s", code,
			out)
	}
	if err := os.Remove(cloud); err != nil {
		t.Fatal(err)
	}
	code, out = f.run("-live", live, f.wire)
	if code != exitDidNotRun || !strings.Contains(out, "DID NOT RUN: open "+cloud) {
		t.Errorf("apicoverage exited %d, want 2 without the live cloud spec:\n%s", code, out)
	}
}

func TestRunWritesTheSummary(t *testing.T) {
	f := newFixture(t)
	summary := f.raw("summary.md", "earlier steps\n")
	code, out := f.run("-summary", summary, f.record("r.jsonl", getUser, checkAllowed,
		wireLine("api", "GET", "/v2/api-key/scope", "/v2/api-key/scope")))
	if code != exitFailed {
		t.Fatalf("apicoverage exited %d, want 1:\n%s", code, out)
	}
	data, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	want := "earlier steps\n### API coverage\n\n" +
		"**FAIL**: 1 problem with coverage.yaml, the specs and the records\n\n" +
		"coverage.yaml and the specs in " + f.specs + ". Records: 1 record file, 1 test, 3 " +
		"requests: 3 wire, 0 dryrun, 0 live.\n\n" +
		"| Section | Stage | Rows | Covered | Planned | Excluded | Deferred | Undecided |\n" +
		"| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |\n" +
		"| control-plane | GA | 8 | 1 | 4 | 2 | 1 | 0 |\n" +
		"| control-plane | EAP | 1 | 0 | 0 | 1 | 0 | 0 |\n" +
		"| control-plane | deprecated | 1 | 0 | 0 | 1 | 0 | 0 |\n" +
		"| control-plane | SDK-only | 1 | 0 | 1 | 0 | 0 |  |\n" +
		"| pdp | GA | 5 | 1 | 1 | 2 | 1 | 0 |\n" +
		"| pdp | deprecated | 1 | 0 | 0 | 1 | 0 | 0 |\n" +
		"| pdp | SDK-only | 1 | 0 | 1 | 0 | 0 |  |\n\n" +
		"e2e of the 2 covered rows: 0 cloud, 0 local, 0 advisory, 0 dry run, 2 none; dry run: " +
		"the records hold no live request, and a request of a scenario run offline is no " +
		"end-to-end evidence.\n\n" +
		"| Route | Op | Methods | Requests | e2e (dry run) |\n" +
		"| --- | --- | --- | ---: | --- |\n" +
		"| control-plane `GET /v2/facts/{proj_id}/{env_id}/users/{user_id}` | `get_user` | " +
		"UsersService.Get | 1 | none |\n" +
		"| pdp `POST /allowed` | `is_allowed_allowed_post` | Client.Check | 1 | none |\n\n" +
		"#### Problems\n\n" +
		"- control-plane GET /v2/api-key/scope: stale: it is planned, but the records hold 1 " +
		"request to it; mark it covered\n\n"
	if string(data) != want {
		t.Errorf("the summary is\n%s\nwant\n%s", data, want)
	}
}

func TestRunWritesTheSummaryOfARunThatPassed(t *testing.T) {
	f := newFixture(t)
	f.edit(coverageFile, `/users/{user_id}", stage: GA, status: covered}`,
		`/users/{user_id}", stage: GA, status: planned}`)
	f.edit(coverageFile, `route: "POST /allowed", stage: GA, status: covered}`,
		`route: "POST /allowed", stage: GA, status: planned}`)
	f.write(f.opMap, `{"methods": {}}`)
	summary := filepath.Join(f.dir, "summary.md")
	code, out := f.run("-summary", summary, f.record("r.jsonl"))
	if code != exitPassed {
		t.Fatalf("apicoverage exited %d, want 0:\n%s", code, out)
	}
	data, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"**PASS**: 0 of 18 rows covered, every GA operation decided",
		"\nNo row is covered yet.\n",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("the summary is\n%s\nwant %q", data, want)
		}
	}
	if strings.Contains(string(data), "Problems") {
		t.Errorf("the summary of a run that passed lists problems:\n%s", data)
	}
}

// healthMethods are the methods of the health route in the fixture of pdp-container.extra.json.
const healthMethods = "\"methods\": [\n        \"GET\"\n      ],"

func TestRunDidNotRun(t *testing.T) {
	header := `{"type":"header","format":1}`
	session := `{"type":"session","exit_status":0,"tests":1}`
	for _, tc := range []struct {
		name   string
		change func(f *fixture) []string
		want   string
	}{
		{"no record", func(*fixture) []string { return nil }, "no request record given"},
		{
			"a bad flag", func(f *fixture) []string { return []string{"-x", f.wire} },
			"flag provided but not defined: -x",
		},
		{"a flag after a record", func(f *fixture) []string {
			return []string{f.wire, "-summary", "x"}
		}, "the flag -summary comes after a record"},
		{"an empty -specs", func(f *fixture) []string {
			return []string{"-specs", "", f.wire}
		}, "-specs is empty"},
		{"an empty -map", func(f *fixture) []string {
			return []string{"-map", "", f.wire}
		}, "-map is empty"},
		{"a missing record", func(f *fixture) []string {
			return []string{filepath.Join(f.dir, "none.jsonl")}
		}, "none.jsonl: no such file"},
		{"a truncated record without its trailer", func(f *fixture) []string {
			return []string{f.raw("r.jsonl", header+"\n"+getUser+"\n")}
		}, "r.jsonl has no session trailer: the run that wrote it did not finish"},
		{"an empty record", func(f *fixture) []string {
			return []string{f.raw("r.jsonl", "")}
		}, "r.jsonl is empty"},
		{"a record of tests that failed", func(f *fixture) []string {
			return []string{f.raw("r.jsonl", header+"\n"+getUser+"\n"+
				`{"type":"session","exit_status":1,"tests":4}`+"\n")}
		}, "r.jsonl:3: the tests that wrote the record failed (exit status 1)"},
		{"a record in which no test took part", func(f *fixture) []string {
			return []string{f.raw("r.jsonl", header+"\n"+
				`{"type":"session","exit_status":0,"tests":0}`+"\n")}
		}, "r.jsonl:2: no test took part in the run that wrote the record (tests: 0)"},
		{"a record without its header", func(f *fixture) []string {
			return []string{f.raw("r.jsonl", getUser+"\n"+session+"\n")}
		}, "r.jsonl:1: the first line is not the header of a record of format 1"},
		{"a record of another format", func(f *fixture) []string {
			return []string{f.raw("r.jsonl", `{"type":"header","format":2}`+"\n"+session+"\n")}
		}, "the first line is not the header"},
		{"a header with a request's field", func(f *fixture) []string {
			return []string{f.raw("r.jsonl", `{"type":"header","format":1,"test":"x"}`+"\n"+
				session+"\n")}
		}, "the first line is not the header"},
		{"a line after the trailer", func(f *fixture) []string {
			return []string{f.raw("r.jsonl", header+"\n"+session+"\n"+getUser+"\n")}
		}, "r.jsonl:3: a line after the session trailer"},
		{"two trailers", func(f *fixture) []string {
			return []string{f.raw("r.jsonl", header+"\n"+session+"\n"+session+"\n")}
		}, "r.jsonl:3: a line after the session trailer"},
		{"a line that is not JSON", func(f *fixture) []string {
			return []string{f.record("r.jsonl", "GET /v2/x")}
		}, "r.jsonl:2: not a line of a request record"},
		{"two values on a line", func(f *fixture) []string {
			return []string{f.record("r.jsonl", getUser+" "+getUser)}
		}, "r.jsonl:2: more than one JSON value"},
		{"an unknown field", func(f *fixture) []string {
			return []string{f.record("r.jsonl",
				strings.Replace(getUser, `"test"`, `"query":"a=b","test"`, 1))}
		}, `unknown field "query"`},
		{"an unknown line type", func(f *fixture) []string {
			return []string{f.record("r.jsonl", `{"type":"note"}`)}
		}, `the type "note" is not request or session`},
		{"a trailer with a request's field", func(f *fixture) []string {
			return []string{f.raw("r.jsonl", header+"\n"+
				`{"type":"session","exit_status":0,"tests":1,"test":"x"}`+"\n")}
		}, `the session trailer is {"type":"session","exit_status":<code>,"tests":<count>}`},
		{"a trailer without a count", func(f *fixture) []string {
			return []string{f.raw("r.jsonl", header+"\n"+`{"type":"session","exit_status":0}`+
				"\n")}
		}, "the session trailer is"},
		{"a request with a trailer's field", func(f *fixture) []string {
			return []string{f.record("r.jsonl",
				strings.Replace(getUser, `"test"`, `"tests":3,"test"`, 1))}
		}, "a request line has a field of another line type"},
		{"a local request in a record", func(f *fixture) []string {
			return []string{f.record("r.jsonl", strings.Replace(getUser, `"wire"`, `"local"`, 1))}
		}, `the mode "local" is not one of [wire dryrun live] here`},
		{"a wire request in the local record", func(f *fixture) []string {
			return []string{"-local", f.record("local.jsonl", getUser), f.wire}
		}, `local.jsonl:2: the mode "wire" is not one of [local] here`},
		{"an unknown target", func(f *fixture) []string {
			return []string{f.record("r.jsonl", strings.Replace(getUser, `"api"`, `"opa"`, 1))}
		}, `the target "opa" is not api or pdp`},
		{"a lower-case method", func(f *fixture) []string {
			return []string{f.record("r.jsonl", strings.Replace(getUser, `"GET"`, `"get"`, 1))}
		}, `the method "get" is not a request method`},
		{"a relative path", func(f *fixture) []string {
			return []string{f.record("r.jsonl", wireLine("api", "GET", "", "v2/x"))}
		}, `the path "v2/x" does not start with /`},
		{"a bad escape", func(f *fixture) []string {
			return []string{f.record("r.jsonl", wireLine("api", "GET", "", "/v2/%zz"))}
		}, `the path "/v2/%zz" has a bad escape`},
		{"a path with a query", func(f *fixture) []string {
			return []string{f.record("r.jsonl", getUser, checkAllowed,
				wireLine("api", "GET", "", "/v2/api-key/scope?email=a"))}
		}, `r.jsonl:4: the path "/v2/api-key/scope?email=a" holds a query or a fragment, which a ` +
			"record leaves out"},
		{"a path with a fragment", func(f *fixture) []string {
			return []string{f.record("r.jsonl", wireLine("api", "GET", "", "/v2/api-key/scope#a"))}
		}, `the path "/v2/api-key/scope#a" holds a query or a fragment`},
		{"a status of 600", func(f *fixture) []string {
			return []string{f.record("r.jsonl", strings.Replace(getUser, "200", "600", 1))}
		}, "the status must be 0, for no response, or 100 to 599"},
		{"a status of 99", func(f *fixture) []string {
			return []string{f.record("r.jsonl", strings.Replace(getUser, "200", "99", 1))}
		}, "the status must be 0"},
		{"no status", func(f *fixture) []string {
			return []string{f.record("r.jsonl", strings.Replace(getUser, `"status":200,`, "", 1))}
		}, "the status must be 0"},
		{"no test", func(f *fixture) []string {
			return []string{f.record("r.jsonl", strings.Replace(getUser, `"TestX"`, `""`, 1))}
		}, "the request names no test"},
		{"a malformed template", func(f *fixture) []string {
			return []string{f.record("r.jsonl", wireLine("api", "GET", "/v2/{x", "/v2/x"))}
		}, `the template: the path "/v2/{x" has the segment "{x"`},
		{"fewer requests than covered rows", func(f *fixture) []string {
			return []string{f.record("r.jsonl", getUser)}
		}, "the records hold 1 request, fewer than the 2 covered rows of coverage.yaml: tests " +
			"did not run"},
		{"a missing spec", func(f *fixture) []string {
			if err := os.Remove(filepath.Join(f.specs, "pdp-container.json")); err != nil {
				f.t.Fatal(err)
			}
			return []string{f.wire}
		}, "pdp-container.json: no such file"},
		{"a spec that is not JSON", func(f *fixture) []string {
			f.write(filepath.Join(f.specs, "pdp-cloud.json"), "{")
			return []string{f.wire}
		}, "parsing " + "SPECS/pdp-cloud.json"},
		{"a spec without operations", func(f *fixture) []string {
			f.write(filepath.Join(f.specs, "pdp-cloud.json"),
				`{"paths": {"/x": {"parameters": []}}}`)
			return []string{f.wire}
		}, "pdp-cloud.json lists no operation"},
		{"an operation without an operationId", func(f *fixture) []string {
			f.edit("pdp-cloud.json", `"operationId": "allowed_handler",`, "")
			return []string{f.wire}
		}, "pdp-cloud.json: POST /allowed has no operationId"},
		{"an operation that is not an object", func(f *fixture) []string {
			f.write(filepath.Join(f.specs, "pdp-cloud.json"), `{"paths": {"/x": {"get": 1}}}`)
			return []string{f.wire}
		}, "pdp-cloud.json: GET /x: json: cannot unmarshal"},
		{"a malformed path in a spec", func(f *fixture) []string {
			f.edit("pdp-cloud.json", `"/allowed": {`, `"/allowed/{x": {`)
			return []string{f.wire}
		}, `pdp-cloud.json: the path "/allowed/{x" has the segment "{x"`},
		{"extras that are not JSON", func(f *fixture) []string {
			f.write(filepath.Join(f.specs, "pdp-container.extra.json"), "[")
			return []string{f.wire}
		}, "parsing SPECS/pdp-container.extra.json"},
		{"a missing extras file", func(f *fixture) []string {
			if err := os.Remove(filepath.Join(f.specs, "pdp-container.extra.json")); err != nil {
				f.t.Fatal(err)
			}
			return []string{f.wire}
		}, "pdp-container.extra.json: no such file"},
		{"an extra route without methods", func(f *fixture) []string {
			f.edit("pdp-container.extra.json", healthMethods, `"methods": [],`)
			return []string{f.wire}
		}, "pdp-container.extra.json: /health has no methods"},
		{"an extra route with an unknown method", func(f *fixture) []string {
			f.edit("pdp-container.extra.json", healthMethods, `"methods": ["FETCH"],`)
			return []string{f.wire}
		}, `pdp-container.extra.json: /health has the method "FETCH"`},
		{"a malformed extra route", func(f *fixture) []string {
			f.edit("pdp-container.extra.json", `"path": "/health"`, `"path": "health"`)
			return []string{f.wire}
		}, `pdp-container.extra.json: the path "health" does not start with /`},
		{"a malformed mount", func(f *fixture) []string {
			f.edit("pdp-container.extra.json", `"prefix": "/v2/facts/{proj_id}/{env_id}"`,
				`"prefix": "v2"`)
			return []string{f.wire}
		}, "pdp-container.extra.json: the mount v2:"},
		{"a mount onto a parameter", func(f *fixture) []string {
			f.edit("pdp-container.extra.json", `"same_as": "/facts"`, `"same_as": "/{x}"`)
			return []string{f.wire}
		}, `the mount /v2/facts/{proj_id}/{env_id} is the same as "/{x}", which is not a path ` +
			"of literal segments"},
		{"a malformed mount target", func(f *fixture) []string {
			f.edit("pdp-container.extra.json", `"same_as": "/facts"`, `"same_as": "facts"`)
			return []string{f.wire}
		}, `is the same as "facts"`},
		{"v1 routes that are not JSON", func(f *fixture) []string {
			f.write(filepath.Join(f.specs, "v1-routes.json"), "{")
			return []string{f.wire}
		}, "parsing v1-routes.json"},
		{"missing v1 routes", func(f *fixture) []string {
			if err := os.Remove(filepath.Join(f.specs, "v1-routes.json")); err != nil {
				f.t.Fatal(err)
			}
			return []string{f.wire}
		}, "v1-routes.json: no such file"},
		{"no v1 routes", func(f *fixture) []string {
			f.write(filepath.Join(f.specs, "v1-routes.json"), `{"routes": []}`)
			return []string{f.wire}
		}, "v1-routes.json lists no route"},
		{"a v1 route without its operation", func(f *fixture) []string {
			f.edit("v1-routes.json", `"operation_id": "get_user",`, "")
			return []string{f.wire}
		}, "v1-routes.json: a route lacks its operation_id, method or path"},
		{"a v1 route of a PDP operation", func(f *fixture) []string {
			f.edit("v1-routes.json", `"operation_id": "get_user",`,
				`"operation_id": "is_allowed_allowed_post",`)
			return []string{f.wire}
		}, "SPECS/v1-routes.json: is_allowed_allowed_post is not the op of a control-plane row " +
			"of coverage.yaml; the file lists only operations of the control plane"},
		{"a v1 route of an operation without a row", func(f *fixture) []string {
			f.edit("v1-routes.json", `"operation_id": "list_roles",`,
				`"operation_id": "list_widgets",`)
			return []string{f.wire}
		}, "SPECS/v1-routes.json: list_widgets is not the op of a control-plane row"},
		{"v1 route fixes that are not YAML", func(f *fixture) []string {
			f.write(filepath.Join(f.specs, "v1-route-fixes.yaml"), "fixes: [")
			return []string{f.wire}
		}, "parsing v1-route-fixes.yaml"},
		{"missing v1 route fixes", func(f *fixture) []string {
			if err := os.Remove(filepath.Join(f.specs, "v1-route-fixes.yaml")); err != nil {
				f.t.Fatal(err)
			}
			return []string{f.wire}
		}, "v1-route-fixes.yaml: no such file"},
		{"a fix without v2", func(f *fixture) []string {
			f.edit("v1-route-fixes.yaml", "        v2: GET /v2/schema/{proj_id}/{env_id}/roles\n",
				"")
			return []string{f.wire}
		}, "v1-route-fixes.yaml: a change of the fix role-list lacks v1 or v2"},
		{"a missing operation map", func(f *fixture) []string {
			return []string{"-map", filepath.Join(f.dir, "none.json"), f.wire}
		}, "none.json: no such file"},
		{"an operation map without methods", func(f *fixture) []string {
			f.write(f.opMap, `{}`)
			return []string{f.wire}
		}, "operation-map.json has no methods object"},
		{"an operation map with an unknown field", func(f *fixture) []string {
			f.write(f.opMap, `{"methods": {}, "ops": {}}`)
			return []string{f.wire}
		}, `unknown field "ops"`},
		{"a missing advisory list", func(f *fixture) []string {
			return []string{"-advisory", filepath.Join(f.dir, "none.txt"), f.wire}
		}, "none.txt: no such file"},
		{"an advisory line without a method", func(f *fixture) []string {
			return []string{"-advisory", f.raw("a.txt", "check it\n"), f.wire}
		}, `a.txt:1: "check it" does not start with a method, such as UsersService.Get`},
		{"an advisory line without a reason", func(f *fixture) []string {
			return []string{"-advisory", f.raw("a.txt", "# x\nClient.Check \n"), f.wire}
		}, "a.txt:2: Client.Check has no reason"},
		{"an advisory method listed twice", func(f *fixture) []string {
			return []string{
				"-advisory", f.raw("a.txt", "Client.Check a\nClient.Check b\n"),
				f.wire,
			}
		}, "a.txt:2: Client.Check is listed twice"},
		{"a summary that cannot be written", func(f *fixture) []string {
			return []string{"-summary", f.dir, f.wire}
		}, "writing the summary: open " + "DIR"},
		{"a missing coverage.yaml", func(f *fixture) []string {
			if err := os.Remove(filepath.Join(f.specs, coverageFile)); err != nil {
				f.t.Fatal(err)
			}
			return []string{f.wire}
		}, "coverage.yaml: no such file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			code, out := f.run(tc.change(f)...)
			want := strings.NewReplacer("SPECS", f.specs, "DIR", f.dir).Replace(tc.want)
			if code != exitDidNotRun || !strings.Contains(out, want) ||
				!strings.Contains(out, "DID NOT RUN: ") {
				t.Errorf("apicoverage exited %d and printed\n%s\nwant 2 and %q", code, out, want)
			}
		})
	}
}

// TestRunRejectsMalformedCoverage checks that a coverage.yaml whose shape is wrong stops the
// check.
func TestRunRejectsMalformedCoverage(t *testing.T) {
	for _, tc := range []struct {
		name, old, replacement, want string
	}{
		{"not YAML", "control-plane:\n", "control-plane: [\n", "parsing "},
		{
			"an unknown field", "{op: get_api_key_scope,", "{op: get_api_key_scope, tier: P1,",
			"field tier not found",
		},
		{
			"an unknown section", "sdk_only_params:\n", "notes: x\nsdk_only_params:\n",
			"field notes not found",
		},
		{
			"no reasons",
			"  account-admin: \"Account administration.\"\n" +
				"  dashboard-only: \"Dashboard setup.\"\n" +
				"  deprecated: \"Deprecated in the spec.\"\n" +
				"  eap: \"In early access.\"\n" +
				"  operator: \"Routes for whoever runs a PDP.\"\n", "",
			"reasons lists no reason for an excluded row",
		},
		{"a reason key in capitals", "  eap: ", "  EAP: ", `reasons: "EAP" needs a key`},
		{
			"a reason without a sentence", `eap: "In early access."`, `eap: " "`,
			`reasons: "eap" needs a key`,
		},
		{
			"a malformed route", `route: "GET /v2/api-key/scope"`, `route: "GET v2/api-key/scope"`,
			`the path "v2/api-key/scope" does not start with /`,
		},
		{
			"an unknown method", `route: "GET /v2/api-key/scope"`,
			`route: "FETCH /v2/api-key/scope"`,
			`"FETCH /v2/api-key/scope" is not a method`,
		},
		{
			"a malformed op", "{op: get_api_key_scope,", "{op: get-api-key-scope,",
			`op "get-api-key-scope" is not an operationId`,
		},
		{
			"an unknown stage", `"GET /v2/api-key/scope", stage: GA`,
			`"GET /v2/api-key/scope", stage: beta`,
			`stage "beta" is not one of GA, EAP, deprecated`,
		},
		{
			"an unknown status", "stage: GA, status: planned}\n  - {op: list_templates",
			"stage: GA, status: todo}\n  - {op: list_templates",
			`status "todo" is not one of covered, planned, excluded, deferred`,
		},
		{
			"an excluded row without a reason", "status: excluded, reason: account-admin}",
			"status: excluded}", "an excluded row needs a reason",
		},
		{
			"an excluded row with a trigger", "status: excluded, reason: account-admin}",
			"status: excluded, reason: account-admin, trigger: x}", "and no trigger",
		},
		{
			"a deferred row without a trigger", "    trigger: \"Decide again when it is.\"\n", "",
			"a deferred row needs a reason and a trigger",
		},
		{
			"a planned row with a reason", "stage: GA, status: planned}\n  - {op: list_templates",
			"stage: GA, status: planned, reason: x}\n  - {op: list_templates",
			"a planned row has no reason or trigger",
		},
		{
			"an excluded row with sdk_only", "status: excluded, reason: account-admin}",
			"status: excluded, reason: account-admin, sdk_only: x}",
			"sdk_only is for a row that the SDK sends, covered or planned, and this one is " +
				"excluded",
		},
		{
			"an empty sdk_only", `sdk_only: "The login-as route is not in the spec."`,
			`sdk_only: " "`, "sdk_only is empty",
		},
		{
			"a stage override without a reason",
			`      reason: "The summary says EAP; the tag is GA."`,
			`      reason: ""`, "stage_override needs a stage and a reason",
		},
		{
			"a stage override of an unknown stage", "    stage_override:\n      stage: GA\n",
			"    stage_override:\n      stage: preview\n", "stage_override needs a stage",
		},
		{
			"two rows of a route", "  - {op: list_direct_group,",
			"  - {op: list_direct_group2, route: \"GET /v2/schema/{proj_id}/{env_id}/groups/" +
				"direct\", stage: GA, status: planned}\n  - {op: list_direct_group,",
			"control-plane GET /v2/schema/{proj_id}/{env_id}/groups/direct has two rows",
		},
		{
			"two rows with an op", "{op: list_direct_group,", "{op: get_api_key_scope,",
			"and control-plane GET /v2/schema/{proj_id}/{env_id}/groups/direct have the same " +
				"op get_api_key_scope",
		},
		{"an empty row", "pdp:\n", "pdp:\n  -\n", "pdp: row 1 is empty"},
		{
			"a malformed SDK-only parameter", `param: "attr_*"`, `param: "attr *"`,
			"sdk_only_params: entry 1 needs an op, a param",
		},
		{
			"an SDK-only parameter without a reason",
			`    reason: "Attribute filters that the spec does not list."`, `    reason: ""`,
			"sdk_only_params: entry 1 needs",
		},
		{
			"an SDK-only parameter listed twice", "sdk_only_params:\n",
			"sdk_only_params:\n  - {op: list_roles, param: \"attr_*\", reason: x}\n",
			"sdk_only_params: list_roles attr_* is listed twice",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.edit(coverageFile, tc.old, tc.replacement)
			code, out := f.run(f.wire)
			if code != exitDidNotRun || !strings.Contains(out, tc.want) {
				t.Errorf("apicoverage exited %d and printed\n%s\nwant 2 and %q", code, out,
					tc.want)
			}
		})
	}
}

func TestRunRejectsCoverageWithoutRows(t *testing.T) {
	f := newFixture(t)
	f.write(filepath.Join(f.specs, coverageFile), "reasons:\n  eap: x\ncontrol-plane: []\n"+
		"pdp: []\n")
	code, out := f.run(f.wire)
	if code != exitDidNotRun || !strings.Contains(out, "control-plane and pdp must each have "+
		"rows") {
		t.Errorf("apicoverage exited %d and printed\n%s\nwant 2", code, out)
	}
}
