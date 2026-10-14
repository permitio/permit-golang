package rest

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// testOp returns a valid test-only operation with the ID id, whose route is TestPrefix and id.
func testOp(id string) Op {
	return Op{ID: id, Method: http.MethodGet, Route: TestPrefix + id, KeyLevel: EnvironmentLevel}
}

// sdkOp returns a valid operation of the shape of the SDK's, with the ID id.
func sdkOp(id string) Op {
	return Op{
		ID: id, Method: http.MethodGet, Route: "/v2/facts/{proj_id}/{env_id}/users/{user_id}",
		Idempotent: true, Facts: true, KeyLevel: EnvironmentLevel, ResponseModel: "UserRead",
	}
}

func TestTestPrefix(t *testing.T) {
	if TestPrefix != "/__test/" {
		t.Errorf("TestPrefix = %q, want /__test/, the prefix that the request record leaves out",
			TestPrefix)
	}
}

func TestDefault(t *testing.T) {
	r, err := newRegistry(defaultOps)
	if err != nil {
		t.Fatalf("the operations of defaultOps do not make a registry: %v", err)
	}
	if got, want := len(r.Ops()), len(defaultOps); got != want {
		t.Errorf("the default registry holds %d operations, want the %d of defaultOps", got, want)
	}
	if first, second := Default(), Default(); first != second || first != defaultRegistry {
		t.Error("Default returns another registry on each call, want the one registry")
	}
	for _, op := range Default().Ops() {
		if strings.HasPrefix(op.Route, TestPrefix) {
			t.Errorf("the default registry holds the test-only operation %s", op.ID)
		}
	}
}

func TestCheckAcceptsValidOps(t *testing.T) {
	minimal := Op{ID: "a", Method: http.MethodGet, Route: "/allowed", KeyLevel: EnvironmentLevel}
	for name, change := range map[string]func(op *Op){
		"the minimal op":           func(*Op) {},
		"a facts read":             func(op *Op) { *op = sdkOp("get_user") },
		"a test-only op":           func(op *Op) { *op = testOp("test_echo") },
		"an ID that starts with _": func(op *Op) { op.ID = "_x9" },
		"POST":                     func(op *Op) { op.Method = http.MethodPost },
		"PUT":                      func(op *Op) { op.Method = http.MethodPut },
		"PATCH":                    func(op *Op) { op.Method = http.MethodPatch },
		"DELETE":                   func(op *Op) { op.Method = http.MethodDelete },
		"project level":            func(op *Op) { op.KeyLevel = ProjectLevel },
		"organization level":       func(op *Op) { op.KeyLevel = OrganizationLevel },
		"a list":                   func(op *Op) { op.MaxPerPage = 100 },
		"a self delete":            func(op *Op) { op.Method, op.DeleteSelf = "DELETE", true },
		"idempotent":               func(op *Op) { op.Idempotent = true },
		"no retry":                 func(op *Op) { op.NoRetry = true },
		"container only":           func(op *Op) { op.ContainerOnly = true },
		"sensitive fields":         func(op *Op) { op.SensitiveBodyFields = []string{"secret"} },
		"models":                   func(op *Op) { op.RequestModel, op.ResponseModel = "A", "B" },
		"a waiting fact": func(op *Op) {
			*op = sdkOp("create_user")
			op.Method, op.Route, op.WaitsForSync = http.MethodPost, apiFactsPrefix+"users", true
		},
		"a test-only fact": func(op *Op) {
			*op = testOp("test_fact")
			op.Route, op.Facts = TestPrefix+"v2/facts/{proj_id}/{env_id}/objects", true
		},
	} {
		op := minimal
		change(&op)
		if err := op.check(); err != nil {
			t.Errorf("%s: check() = %v, want nil", name, err)
		}
	}
}

func TestCheckRejectsInvalidOps(t *testing.T) {
	valid := sdkOp("get_user")
	for _, tc := range []struct {
		name   string
		change func(op *Op)
		want   string
	}{
		{"no ID", func(op *Op) { op.ID = "" }, `the ID "" is not an operationId, such as get_user`},
		{"an ID with a hyphen", func(op *Op) { op.ID = "get-user" }, `the ID "get-user" is not`},
		{"an ID that starts with a digit", func(op *Op) { op.ID = "1get" }, `the ID "1get" is`},
		{"an ID with a space", func(op *Op) { op.ID = "get user" }, `the ID "get user" is not`},
		{
			"no method", func(op *Op) { op.Method = "" },
			`the method "" is not one of GET, POST, PUT, PATCH, DELETE`,
		},
		{"a lower-case method", func(op *Op) { op.Method = "get" }, `the method "get" is not`},
		{"HEAD", func(op *Op) { op.Method = http.MethodHead }, `the method "HEAD" is not one of`},
		{"no route", func(op *Op) { op.Route = "" }, `the route "" does not start with a slash`},
		{
			"a route with a method", func(op *Op) { op.Route = "GET /v2/x" },
			`the route "GET /v2/x" does not start with a slash`,
		},
		{"a route with //", func(op *Op) { op.Route = "/v2//x" }, "has an empty segment"},
		{"a broken placeholder", func(op *Op) { op.Route = "/v2/{x" }, `has the segment "{x"`},
		{
			"no key level", func(op *Op) { op.KeyLevel = 0 },
			"the key level 0 is not EnvironmentLevel, ProjectLevel or OrganizationLevel",
		},
		{"a key level above", func(op *Op) { op.KeyLevel = 4 }, "the key level 4 is not"},
		{"a negative key level", func(op *Op) { op.KeyLevel = -1 }, "the key level -1 is not"},
		{
			"a negative page size", func(op *Op) { op.MaxPerPage = -1 },
			"MaxPerPage is -1; give the largest per_page of a list, or 0",
		},
		{
			"DeleteSelf on a GET", func(op *Op) { op.DeleteSelf = true },
			"DeleteSelf is set on a GET; only a DELETE deletes its object",
		},
		{
			"WaitsForSync without Facts", func(op *Op) { op.WaitsForSync, op.Facts = true, false },
			"WaitsForSync is set without Facts; only a facts operation waits",
		},
		{
			"Facts outside the facts API", func(op *Op) { op.Route = "/v2/schema/{proj_id}/x" },
			"the route /v2/schema/{proj_id}/x has Facts set and does not start with " +
				"/v2/facts/{proj_id}/{env_id}/; only a route of the facts API goes through the " +
				"PDP container",
		},
		{
			"Facts on the prefix alone", func(op *Op) { op.Route = "/v2/facts/{proj_id}/{env_id}" },
			"has Facts set and does not start with",
		},
		{
			"Facts on the PDP's route", func(op *Op) { op.Route = "/facts/users/{user_id}" },
			"has Facts set and does not start with",
		},
		{
			"Facts with other placeholders",
			func(op *Op) { op.Route = "/v2/facts/{project}/{env_id}/users" },
			"has Facts set and does not start with",
		},
		{
			"Facts after a prefix that is not TestPrefix",
			func(op *Op) { op.Route = "/__testing/v2/facts/{proj_id}/{env_id}/users" },
			"has Facts set and does not start with",
		},
		{
			"NoRetry and Idempotent", func(op *Op) { op.NoRetry = true },
			"NoRetry and Idempotent are both set",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			op := valid
			tc.change(&op)
			err := op.check()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("check() = %v, want an error that contains %q", err, tc.want)
			}
		})
	}
}

func TestNewRegistry(t *testing.T) {
	ops := []Op{sdkOp("get_user"), sdkOp("delete_user"), sdkOp("create_user")}
	r, err := newRegistry(ops)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 3)
	for _, op := range r.Ops() {
		ids = append(ids, op.ID)
	}
	if diff := cmp.Diff([]string{"create_user", "delete_user", "get_user"}, ids); diff != "" {
		t.Errorf("Ops() differs (-want +got):\n%s", diff)
	}
	got, ok := r.Op("delete_user")
	if !ok {
		t.Fatal("Op(delete_user) = false, want the operation")
	}
	if diff := cmp.Diff(ops[1], got); diff != "" {
		t.Errorf("Op(delete_user) differs (-want +got):\n%s", diff)
	}
	if op, ok := r.Op("list_users"); ok {
		t.Errorf("Op(list_users) = %+v, true; want false", op)
	}
	if empty, err := newRegistry(nil); err != nil || len(empty.Ops()) != 0 {
		t.Errorf("newRegistry(nil) = %v, %v; want an empty registry", empty, err)
	}
}

func TestNewRegistryRejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		ops  []Op
		want string
	}{
		{
			"a duplicate ID",
			[]Op{sdkOp("get_user"), sdkOp("list_users"), sdkOp("get_user")},
			"rest: the operation get_user is registered already; give each operation its own ID",
		},
		{
			"a test-only route",
			[]Op{sdkOp("get_user"), testOp("test_echo")},
			"rest: the operation test_echo: the route /__test/test_echo starts with /__test/, " +
				"which only a test-only operation may use",
		},
		{
			"an op that is not valid",
			[]Op{{ID: "get_user", Method: "GET", Route: "/v2/x"}},
			`rest: the operation "get_user": the key level 0 is not`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := newRegistry(tc.ops)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("newRegistry = %v, %v; want an error that contains %q", r, err, tc.want)
			}
		})
	}
}

func TestMustRegistryPanics(t *testing.T) {
	defer func() {
		got, _ := recover().(error)
		if got == nil || !strings.Contains(got.Error(), "get_user is registered already") {
			t.Errorf("mustRegistry panicked with %v, want the error of the duplicate", got)
		}
	}()
	mustRegistry([]Op{sdkOp("get_user"), sdkOp("get_user")})
	t.Error("mustRegistry of a duplicate returned, want a panic")
}

func TestWithTestOps(t *testing.T) {
	base, err := newRegistry([]Op{sdkOp("get_user")})
	if err != nil {
		t.Fatal(err)
	}
	r, err := base.WithTestOps(testOp("test_b"), testOp("test_a"))
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 3)
	for _, op := range r.Ops() {
		ids = append(ids, op.ID)
	}
	if diff := cmp.Diff([]string{"get_user", "test_a", "test_b"}, ids); diff != "" {
		t.Errorf("the registry with test ops differs (-want +got):\n%s", diff)
	}
	if _, ok := base.Op("test_a"); ok || len(base.Ops()) != 1 {
		t.Errorf("WithTestOps changed the registry it extends: %+v", base.Ops())
	}
	if again, err := r.WithTestOps(); err != nil || len(again.Ops()) != 3 || again == r {
		t.Errorf("WithTestOps() = %v, %v; want a new registry of the same 3 operations", again, err)
	}
	var zero Registry
	if one, err := zero.WithTestOps(testOp("test_a")); err != nil || len(one.Ops()) != 1 {
		t.Errorf("WithTestOps on the zero Registry = %v, %v; want one operation", one, err)
	}
}

func TestWithTestOpsRejects(t *testing.T) {
	base, err := newRegistry([]Op{sdkOp("get_user")})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		ops  []Op
		want string
	}{
		{
			"a route outside the prefix",
			[]Op{testOp("test_a"), sdkOp("list_users")},
			"rest: the test-only operation list_users: the route " +
				"/v2/facts/{proj_id}/{env_id}/users/{user_id} does not start with /__test/",
		},
		{
			"a route that only looks like the prefix",
			[]Op{{ID: "a", Method: "GET", Route: "/__testing/a", KeyLevel: EnvironmentLevel}},
			"the route /__testing/a does not start with /__test/",
		},
		{
			"the ID of an SDK operation",
			[]Op{{ID: "get_user", Method: "GET", Route: "/__test/u", KeyLevel: EnvironmentLevel}},
			"rest: the operation get_user is registered already",
		},
		{
			"a duplicate test ID",
			[]Op{testOp("test_a"), testOp("test_a")},
			"rest: the operation test_a is registered already",
		},
		{
			"a test op that is not valid",
			[]Op{{ID: "test_a", Method: "GET", Route: "/__test/a", MaxPerPage: -1, KeyLevel: 1}},
			`rest: the operation "test_a": MaxPerPage is -1`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := base.WithTestOps(tc.ops...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("WithTestOps = %v, %v; want an error that contains %q", r, err, tc.want)
			}
			if len(base.Ops()) != 1 {
				t.Errorf("a rejected WithTestOps changed the registry: %+v", base.Ops())
			}
		})
	}
}

func TestRegistryKeepsItsOwnSensitiveFields(t *testing.T) {
	fields := []string{"secret", "client_secret"}
	op := sdkOp("get_proxy_config")
	op.SensitiveBodyFields = fields
	r, err := newRegistry([]Op{op})
	if err != nil {
		t.Fatal(err)
	}
	test := testOp("test_proxy")
	test.SensitiveBodyFields = fields
	tr, err := r.WithTestOps(test)
	if err != nil {
		t.Fatal(err)
	}
	fields[0] = "changed"
	for _, registry := range []*Registry{r, tr} {
		for _, op := range registry.Ops() {
			if !slices.Equal(op.SensitiveBodyFields, []string{"secret", "client_secret"}) {
				t.Errorf("%s holds the fields %q after the caller changed its slice, want its "+
					"own copy", op.ID, op.SensitiveBodyFields)
			}
		}
	}
}

// TestViaPDP checks the route of each Facts operation under the PDP container's /facts mount, of
// the SDK and test-only, with ContainerOnly set and the other fields kept, and that ViaPDP leaves
// any other operation as it is.
func TestViaPDP(t *testing.T) {
	for _, tc := range []struct {
		route, want string
	}{
		{apiFactsPrefix + "users", "/facts/users"},
		{apiFactsPrefix + "users/{user_id}/roles", "/facts/users/{user_id}/roles"},
		{apiFactsPrefix + "relationship_tuples/bulk", "/facts/relationship_tuples/bulk"},
		{
			TestPrefix + "v2/facts/{proj_id}/{env_id}/objects/{object_id}",
			"/__test/facts/objects/{object_id}",
		},
	} {
		op := sdkOp("get_user")
		op.Route, op.SensitiveBodyFields = tc.route, []string{"secret"}
		want := op
		want.Route, want.ContainerOnly = tc.want, true
		if diff := cmp.Diff(want, op.ViaPDP()); diff != "" {
			t.Errorf("ViaPDP of %s differs (-want +got):\n%s", tc.route, diff)
		}
	}
	notFacts := sdkOp("list_set_permissions")
	notFacts.Route, notFacts.Facts = apiFactsPrefix+"set_rules", false
	decision := Op{ID: "is_allowed", Method: http.MethodPost, Route: "/allowed"}
	for _, op := range []Op{notFacts, decision} {
		if diff := cmp.Diff(op, op.ViaPDP()); diff != "" {
			t.Errorf("ViaPDP changed %s, which is not a Facts operation (-want +got):\n%s",
				op.ID, diff)
		}
	}
}

func TestFactsRoute(t *testing.T) {
	for route, want := range map[string][2]string{
		"/v2/facts/{proj_id}/{env_id}/users":                {"", "users"},
		"/v2/facts/{proj_id}/{env_id}/users/{user_id}":      {"", "users/{user_id}"},
		"/__test/v2/facts/{proj_id}/{env_id}/objects":       {"/__test", "objects"},
		"/__test/v2/facts/{proj_id}/{env_id}/a/{object_id}": {"/__test", "a/{object_id}"},
	} {
		head, tail, ok := factsRoute(route)
		if !ok || head != want[0] || tail != want[1] {
			t.Errorf("factsRoute(%q) = %q, %q, %t; want %q, %q, true", route, head, tail, ok,
				want[0], want[1])
		}
	}
	for _, route := range []string{
		"", "/", "/v2/facts/{proj_id}/{env_id}", "/v2/facts/{proj_id}/{env_id}/",
		"/v2/facts/{proj_id}/users", "/facts/users", "/v2/schema/{proj_id}/{env_id}/users",
		"/__test/facts/objects", "/__test/v2/facts/{proj_id}/{env_id}/", "/__testv2/facts/x",
		"/__testing/v2/facts/{proj_id}/{env_id}/users", "v2/facts/{proj_id}/{env_id}/users",
		"/x/v2/facts/{proj_id}/{env_id}/users",
	} {
		if head, tail, ok := factsRoute(route); ok {
			t.Errorf("factsRoute(%q) = %q, %q, true; want false", route, head, tail)
		}
	}
}
