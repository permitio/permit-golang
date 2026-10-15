package fake

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
)

// projectKey is the project's API key of the fakes of these tests.
const projectKey = "permit_key_FAKEfaketest"

// routes are the routes of the fakes of these tests.
var routes = []Route{
	{"GET", "/v2/api-key/scope", opScope},
	{"GET", "/v2/api-key/{proj_id}/{env_id}", opEnvironmentKey},
	{"GET", "/v2/projects/{proj_id}/envs", "list_environments"},
	{"POST", "/v2/projects/{proj_id}/envs", "create_environment"},
	{"GET", "/v2/projects/{proj_id}/envs/{env_id}", "get_environment"},
	{"PATCH", "/v2/projects/{proj_id}/envs/{env_id}", "update_environment"},
	{"DELETE", "/v2/projects/{proj_id}/envs/{env_id}", "delete_environment"},
	{"PUT", "/v2/projects/{proj_id}/envs/{env_id}/copy", "copy_environment"},
	{"GET", "/v2/projects/{proj_id}/envs/{env_id}/stats", "stats_environments"},
	{"POST", "/v2/projects/{proj_id}/envs/{env_id}/copy", "copy_environment"},
	{"POST", "/v2/projects/{proj_id}/envs/{env_id}/copy/async", "copy_environment_async"},
	{
		"GET", "/v2/projects/{proj_id}/envs/{env_id}/copy/async/{task_id}/result",
		"get_copy_environment_async_result",
	},
	{"POST", "/v2/facts/{proj_id}/{env_id}/users", "create_user"},
	{"GET", "/v2/facts/{proj_id}/{env_id}/users", "list_users"},
	{"GET", "/v2/facts/{proj_id}/{env_id}/users/{user_id}", "get_user"},
	{"GET", "/v2/facts/{proj_id}/{env_id}/users/bulk", "bulk_users"},
	{"POST", "/v2/facts/{proj_id}/{env_id}/users/{user_id}", "post_user"},
	{"DELETE", "/v2/facts/{proj_id}/{env_id}/users", "delete_users"},
	{"POST", "/v2/schema/{proj_id}/{env_id}/resources", "create_resource"},
	{"POST", "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles", "create_resource_role"},
	{"GET", "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles", "list_resource_roles"},
	{
		"GET", "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}",
		"get_resource_role",
	},
	{"POST", "/v2/facts/{proj_id}/{env_id}/resource_instances", "create_resource_instance"},
	{
		"GET", "/v2/facts/{proj_id}/{env_id}/resource_instances/{instance_id}",
		"get_resource_instance",
	},
	{
		"DELETE", "/v2/facts/{proj_id}/{env_id}/resource_instances/{instance_id}",
		"delete_resource_instance",
	},
	{"GET", "/v2/schema/{proj_id}/{env_id}/roles/{role_id}/ancestors", "get_role_ancestors"},
	{"POST", "/v2/schema/{proj_id}/{env_id}/roles", "create_role"},
	{"POST", "/v2/facts/{proj_id}/{env_id}/tenants", "create_tenant"},
	{"GET", "/v2/facts/{proj_id}/{env_id}/tenants", "list_tenants"},
	{"POST", "/v2/facts/{proj_id}/{env_id}/role_assignments", "assign_role"},
	{"GET", "/v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}/users", "list_tenant_users"},
	{"POST", "/v2/facts/{proj_id}/{env_id}/users/{user_id}/roles", "assign_role_to_user"},
	{"POST", "/v2/schema/{proj_id}/{env_id}/groups", "create_group"},
	{"GET", "/v2/schema/{proj_id}/{env_id}/groups/{group_instance_key}/roles", "list_group_roles"},
	{"POST", "/v2/same/{first}", "same_first"},
	{"POST", "/v2/same/{second}", "same_second"},
}

// recordingTB is a testing.TB that keeps the errors that the fake reports, rather than failing
// the test.
type recordingTB struct {
	*testing.T

	mu     sync.Mutex
	errors []string
}

func (tb *recordingTB) Errorf(format string, args ...any) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.errors = append(tb.errors, fmt.Sprintf(format, args...))
}

func (tb *recordingTB) Fatal(args ...any) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.errors = append(tb.errors, fmt.Sprint(args...))
}

// take returns the errors that tb kept, and forgets them.
func (tb *recordingTB) take() []string {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	errs := tb.errors
	tb.errors = nil
	return errs
}

// harness is a fake with the TB that it reports to.
type harness struct {
	t    *testing.T
	tb   *recordingTB
	fake *Fake

	// checked are the operations whose bodies the check saw.
	mu      sync.Mutex
	checked []string
}

// newHarness starts a fake of routes, whose check rejects a body that holds "bad" and cannot
// check one that holds "unknown".
func newHarness(t *testing.T) *harness {
	t.Helper()
	tb := &recordingTB{T: t}
	h := &harness{t: t, tb: tb}
	check := func(op string, body []byte) ([]string, error) {
		h.mu.Lock()
		h.checked = append(h.checked, op)
		h.mu.Unlock()
		switch {
		case strings.Contains(string(body), "unknown"):
			return nil, errors.New("no schema for " + op)
		case strings.Contains(string(body), "bad"):
			return []string{"#/key is bad"}, nil
		}
		return nil, nil
	}
	f := New(tb, Config{Routes: routes, Check: check, Project: "proj", ProjectAPIKey: projectKey})
	t.Cleanup(func() {
		if errs := tb.take(); errs != nil {
			t.Errorf("the fake reported errors that the test did not take: %q", errs)
		}
	})
	h.fake = f
	return h
}

// do sends method to path with body, unless it is empty, and key, and returns the status and
// the decoded reply.
func (h *harness) do(method, path, body, key string) (int, any) {
	h.t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(h.t.Context(), method, h.fake.URL()+path, reader)
	if err != nil {
		h.t.Fatal(err)
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	var reply any
	if len(data) > 0 {
		if err := json.Unmarshal(data, &reply); err != nil {
			h.t.Fatalf("%s %s: the reply %q is not JSON", method, path, data)
		}
	}
	return resp.StatusCode, reply
}

// expect sends the request as do does and fails the test unless the status is want; it
// returns the reply as an object, or nil when it is not one.
func (h *harness) expect(want int, method, path, body, key string) map[string]any {
	h.t.Helper()
	status, reply := h.do(method, path, body, key)
	if status != want {
		h.t.Fatalf("%s %s %s answered %d %v, want %d", method, path, body, status, reply, want)
	}
	o, _ := reply.(map[string]any)
	return o
}

// list sends GET to path with key, fails the test unless the reply is 200 with a JSON array, and
// returns the array.
func (h *harness) list(path, key string) []any {
	h.t.Helper()
	status, reply := h.do("GET", path, "", key)
	list, ok := reply.([]any)
	if status != 200 || !ok {
		h.t.Fatalf("GET %s answered %d %v, want 200 and a list", path, status, reply)
	}
	return list
}

// createEnv creates the environment key in the project, and returns it and its API key.
func (h *harness) createEnv(key string) (map[string]any, string) {
	h.t.Helper()
	env := h.expect(200, "POST", "/v2/projects/proj/envs", `{"key": "`+key+`"}`, projectKey)
	secret := h.expect(200, "GET", "/v2/api-key/proj/"+key, "", projectKey)
	s, _ := secret["secret"].(string)
	return env, s
}

func TestEnvironmentLifecycle(t *testing.T) {
	h := newHarness(t)
	org := h.expect(200, "GET", "/v2/api-key/scope", "", projectKey)
	if len(org) != 2 || org["project_id"] != h.fake.ProjectID() || org["organization_id"] == "" {
		t.Errorf("the scope of the project's key is %v", org)
	}
	env, envKey := h.createEnv("dev")
	id, _ := env["id"].(string)
	if env["key"] != "dev" || id == "" || env["project_id"] != h.fake.ProjectID() ||
		env["organization_id"] != org["organization_id"] {
		t.Errorf("the created environment is %v", env)
	}
	if !regexp.MustCompile(`^permit_key_FAKEfakeenv[0-9]{32}$`).MatchString(envKey) {
		t.Errorf("the environment's API key is %q", envKey)
	}
	uuid := regexp.MustCompile(`^00000000-0000-4000-8000-[0-9]{12}$`)
	if !uuid.MatchString(id) || !uuid.MatchString(h.fake.ProjectID()) {
		t.Errorf("the IDs %q and %q are not in the shape of a UUID", id, h.fake.ProjectID())
	}
	if _, again := h.createEnv("prod"); again == envKey {
		t.Error("two environments have one API key")
	}
	if secret := h.expect(200, "GET", "/v2/api-key/"+h.fake.ProjectID()+"/"+id, "",
		projectKey); secret["secret"] != envKey || secret["environment_id"] != id {
		t.Errorf("the API key of the environment by IDs is %v, want the one issued", secret)
	}
	scope := h.expect(200, "GET", "/v2/api-key/scope", "", envKey)
	if scope["environment_id"] != id || scope["project_id"] != h.fake.ProjectID() {
		t.Errorf("the scope of the environment's key is %v", scope)
	}
	h.expect(409, "POST", "/v2/projects/proj/envs", `{"key": "dev"}`, projectKey)
	if got := h.expect(200, "GET", "/v2/projects/proj/envs/"+id, "", projectKey); got["key"] !=
		"dev" {
		t.Errorf("the environment by ID is %v", got)
	}
	patched := h.expect(200, "PATCH", "/v2/projects/proj/envs/dev", `{"name": "Dev", "id": "x"}`,
		projectKey)
	if patched["name"] != "Dev" || patched["id"] != id {
		t.Errorf("the patched environment is %v", patched)
	}
	if list := h.list("/v2/projects/proj/envs", projectKey); len(list) != 2 {
		t.Errorf("the list of environments is %v", list)
	}
	user := h.expect(200, "POST", "/v2/facts/proj/dev/users", `{"key": "u1"}`, envKey)
	if user["environment_id"] != id || user["project_id"] != h.fake.ProjectID() {
		t.Errorf("the created user is %v", user)
	}
	h.expect(200, "GET", "/v2/facts/proj/"+id+"/users/u1", "", envKey)
	h.expect(204, "DELETE", "/v2/projects/proj/envs/dev", "", projectKey)
	h.expect(404, "GET", "/v2/projects/proj/envs/"+id, "", projectKey)
	h.expect(404, "DELETE", "/v2/projects/proj/envs/dev", "", projectKey)
	h.expect(401, "GET", "/v2/api-key/scope", "", envKey)
	_, envKey = h.createEnv("dev")
	if list := h.list("/v2/facts/proj/dev/users", envKey); len(list) != 0 {
		t.Errorf("the users of a new environment of the deleted one's key are %v", list)
	}
}

func TestAccess(t *testing.T) {
	h := newHarness(t)
	_, devKey := h.createEnv("dev")
	h.createEnv("prod")
	for _, tc := range []struct {
		method, path, key string
		want              int
	}{
		{"GET", "/v2/api-key/scope", "", 401},
		{"GET", "/v2/api-key/scope", "permit_key_FAKEunknown", 401},
		{"GET", "/v2/projects/other/envs", projectKey, 404},
		{"GET", "/v2/projects/proj/envs/staging", projectKey, 404},
		{"GET", "/v2/facts/proj/staging/users", projectKey, 404},
		{"GET", "/v2/facts/proj/prod/users", devKey, 403},
		{"GET", "/v2/projects/proj/envs", devKey, 403},
		{"GET", "/v2/facts/proj/dev/users", devKey, 200},
		{"GET", "/v2/facts/proj/prod/users", projectKey, 200},
		{"GET", "/v2/facts/proj/dev/users/nobody", devKey, 404},
	} {
		if status, reply := h.do(tc.method, tc.path, "", tc.key); status != tc.want {
			t.Errorf("%s %s with key %q answered %d %v, want %d", tc.method, tc.path, tc.key,
				status, reply, tc.want)
		}
	}
	if body := h.expect(401, "GET", "/v2/api-key/scope", "", ""); body["error_code"] !=
		"UNAUTHORIZED" || body["message"] == nil {
		t.Errorf("the reply to an unknown key is %v, want an error body", body)
	}
}

func TestBodies(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	h.expect(422, "POST", "/v2/facts/proj/dev/users", `{"key": "bad"}`, projectKey)
	wantErrs := []string{"fake: POST /v2/facts/{proj_id}/{env_id}/users (create_user): the body " +
		"does not fit its schema: #/key is bad"}
	if errs := h.tb.take(); !slices.Equal(errs, wantErrs) {
		t.Errorf("the fake reported %q, want %q", errs, wantErrs)
	}
	h.expect(422, "POST", "/v2/facts/proj/dev/users", `{"key": "unknown"}`, projectKey)
	if errs := h.tb.take(); len(errs) != 1 || !strings.Contains(errs[0],
		"the body could not be checked: no schema for create_user") {
		t.Errorf("the fake reported %q for a body it cannot check", errs)
	}
	h.expect(422, "POST", "/v2/facts/proj/dev/users", `["u1"]`, projectKey)
	if errs := h.tb.take(); len(errs) != 1 || !strings.Contains(errs[0],
		"the body is not a JSON object") {
		t.Errorf("the fake reported %q for an array", errs)
	}
	h.expect(200, "POST", "/v2/facts/proj/dev/users", `{"key": "u1", "n": 1}`, projectKey)
	h.expect(200, "POST", "/v2/facts/proj/dev/users", `{"key": "u2", "n": 2}`, projectKey)
	h.expect(404, "DELETE", "/v2/facts/proj/dev/users", `{"n": 3}`, projectKey)
	h.expect(204, "DELETE", "/v2/facts/proj/dev/users", `{"n": 1}`, projectKey)
	var keys []any
	for _, user := range h.list("/v2/facts/proj/dev/users", projectKey) {
		o, _ := user.(map[string]any)
		keys = append(keys, o["key"])
	}
	if !slices.Equal(keys, []any{"u2"}) {
		t.Errorf("the keys of the users after a removal by body are %v, want u2", keys)
	}
}

// TestPages checks that a GET of a collection of 31 objects lists every object without page and
// per_page, and with either, the page that they pick, in the order in which the objects were
// created: pages of 30 when per_page holds no number from 1, the first page when page holds
// none, and an empty page after the last.
func TestPages(t *testing.T) {
	h := newHarness(t)
	var keys []any
	for i := range 31 {
		key := fmt.Sprintf("e%d", i+1)
		h.createEnv(key)
		keys = append(keys, key)
	}
	for query, want := range map[string][]any{
		"":                    keys,
		"?per_page=2":         keys[:2],
		"?page=2&per_page=2":  keys[2:4],
		"?page=16&per_page=2": keys[30:],
		"?page=17&per_page=2": {},
		"?page=1":             keys[:30],
		"?page=2":             keys[30:],
		"?page=3":             {},
		"?page=0&per_page=x":  keys[:30],
		"?page=2&per_page=-1": keys[30:],
	} {
		keys := []any{}
		for _, env := range h.list("/v2/projects/proj/envs"+query, projectKey) {
			o, _ := env.(map[string]any)
			keys = append(keys, o["key"])
		}
		if !slices.Equal(keys, want) {
			t.Errorf("GET /v2/projects/proj/envs%s listed %v, want %v", query, keys, want)
		}
	}
}

// TestStatsAndCopies checks the statistics of an environment: the environment, with statistics
// of zero and one PDP configuration whose client secret is the environment's API key, and 404
// for an environment that does not exist. It checks a copy into a new environment, which it
// creates with the key and the IDs of a create, and which a second copy with that key conflicts
// with; into an existing one, by key or by ID, which it answers; and 404 for an existing one that
// does not exist and 422 for a target that names neither. A copy in the background creates the
// environment too, answers 202 with a task that is processing, and its result is the task, which
// succeeded with the environment; a task that the environment does not have gets 404.
func TestStatsAndCopies(t *testing.T) {
	h := newHarness(t)
	dev, devKey := h.createEnv("dev")
	stats := h.expect(200, "GET", "/v2/projects/proj/envs/dev/stats", "", projectKey)
	configs, _ := stats["pdp_configs"].([]any)
	config, _ := configs[0].(map[string]any)
	counts, _ := stats["stats"].(map[string]any)
	if stats["id"] != dev["id"] || len(configs) != 1 || config["client_secret"] != devKey ||
		config["environment_id"] != dev["id"] || counts["users"] != 0.0 ||
		counts["has_decision_logs"] != false {
		t.Errorf("the statistics of dev are %v", stats)
	}
	opal, _ := config["opal_server_access_token"].(string)
	if !regexp.MustCompile(`^permit_key_FAKEopal[0-9]{32}$`).MatchString(opal) {
		t.Errorf("the OPAL token of dev's PDP is %q, want a fake key that is not dev's API key",
			opal)
	}
	h.expect(404, "GET", "/v2/projects/proj/envs/staging/stats", "", projectKey)

	copies := "/v2/projects/proj/envs/dev/copy"
	created := h.expect(201, "POST", copies, `{"target_env":{"new":{"key":"qa","name":"QA"}}}`,
		projectKey)
	if created["key"] != "qa" || created["id"] == dev["id"] ||
		created["project_id"] != h.fake.ProjectID() {
		t.Errorf("the copy into a new environment answered %v", created)
	}
	if got := h.expect(200, "GET", "/v2/projects/proj/envs/qa", "", projectKey); got["id"] !=
		created["id"] {
		t.Errorf("the environment of the copy is %v, want %v", got, created)
	}
	h.expect(409, "POST", copies, `{"target_env":{"new":{"key":"qa","name":"QA"}}}`, projectKey)
	id, _ := created["id"].(string)
	for _, existing := range []string{"qa", id} {
		if got := h.expect(201, "POST", copies, `{"target_env":{"existing":"`+existing+`"}}`,
			projectKey); got["id"] != id {
			t.Errorf("the copy into the existing environment %s answered %v", existing, got)
		}
	}
	h.expect(404, "POST", copies, `{"target_env":{"existing":"nope"}}`, projectKey)
	h.expect(422, "POST", copies, `{"target_env":{}}`, projectKey)

	async := `{"target_env":{"new":{"key":"qa2","name":"QA"}}}`
	task := h.expect(202, "POST", copies+"/async", async, projectKey)
	taskID, _ := task["task_id"].(string)
	if task["status"] != "processing" || taskID == "" || len(task) != 2 {
		t.Errorf("the copy in the background answered %v", task)
	}
	result := h.expect(200, "GET", copies+"/async/"+taskID+"/result", "", projectKey)
	env, _ := result["result"].(map[string]any)
	if result["task_id"] != taskID || result["status"] != "success" || env["key"] != "qa2" {
		t.Errorf("the result of the task is %v", result)
	}
	h.expect(200, "GET", "/v2/projects/proj/envs/qa2", "", projectKey)
	h.expect(404, "GET", copies+"/async/no-task/result", "", projectKey)
	h.expect(404, "GET", "/v2/projects/proj/envs/qa/copy/async/"+taskID+"/result", "",
		projectKey)
}

func TestRoutes(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	h.expect(200, "POST", "/v2/facts/proj/dev/users", `{"key": "a/b"}`, projectKey)
	if got := h.expect(200, "GET", "/v2/facts/proj/dev/users/a%2Fb", "", projectKey); got["key"] !=
		"a/b" {
		t.Errorf("a key with an escaped slash reads %v", got)
	}
	h.expect(404, "POST", "/v2/same/x", `{"a": 1}`, projectKey)
	if last := h.checked[len(h.checked)-1]; last != "same_first" {
		t.Errorf("of two routes with as many literal segments, the body went to %s, want the "+
			"first, same_first", last)
	}
	if list := h.list("/v2/facts/proj/dev/users/bulk", projectKey); len(list) != 0 {
		t.Errorf("GET of the literal route bulk answered %v, want the empty list of its own "+
			"collection, not the user bulk", list)
	}
	for _, tc := range []struct{ method, path, body, want string }{
		{"GET", "/v2/nothing", "", "no route that the fake serves matches GET /v2/nothing"},
		{"GET", "/v2/projects//envs", "", "matches GET /v2/projects//envs"},
		{"PUT", "/v2/projects/proj/envs/dev/copy", `{"key": "x"}`, "copy_environment): the " +
			"fake does not implement this request"},
		{"POST", "/v2/facts/proj/dev/users", "", "create_user): the fake does not implement"},
		{"DELETE", "/v2/facts/proj/dev/users", "", "delete_users): the fake does not implement"},
		{"POST", "/v2/facts/proj/dev/users/a%2Fb", `{"x": 1}`, "post_user): the fake does not"},
	} {
		h.do(tc.method, tc.path, tc.body, projectKey)
		if errs := h.tb.take(); len(errs) != 1 || !strings.Contains(errs[0], tc.want) {
			t.Errorf("%s %s reported %q, want an error with %q", tc.method, tc.path, errs,
				tc.want)
		}
	}
	for _, key := range []string{"", "permit_key_FAKEunknown"} {
		status, _ := h.do("GET", "/v2/nothing", "", key)
		if errs := h.tb.take(); status != 404 || len(errs) != 1 || !strings.Contains(errs[0],
			"no route that the fake serves matches GET /v2/nothing") {
			t.Errorf("a route that the fake does not serve, with the key %q, answered %d and "+
				"reported %q", key, status, errs)
		}
	}
}

func TestParents(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	roles := "/v2/schema/proj/dev/resources/document/roles"
	body := h.expect(404, "POST", roles, `{"key": "viewer"}`, projectKey)
	if body["message"] != "no object document" {
		t.Errorf("a create under a resource that does not exist answered %v", body)
	}
	h.expect(404, "GET", roles, "", projectKey)
	h.expect(404, "GET", roles+"/viewer", "", projectKey)
	resource := h.expect(200, "POST", "/v2/schema/proj/dev/resources", `{"key": "document"}`,
		projectKey)
	h.expect(200, "POST", roles, `{"key": "viewer"}`, projectKey)
	id, _ := resource["id"].(string)
	if got := h.expect(200, "GET", "/v2/schema/proj/dev/resources/"+id+"/roles/viewer", "",
		projectKey); got["key"] != "viewer" {
		t.Errorf("the role by its resource's ID is %v", got)
	}
	if list := h.list(roles, projectKey); len(list) != 1 {
		t.Errorf("the roles of the resource are %v", list)
	}
	if body := h.expect(404, "GET", roles+"/editor", "", projectKey); body["message"] !=
		"no object editor" {
		t.Errorf("a role that does not exist of a resource that does answered %v", body)
	}
	for _, tc := range []struct{ method, path, body, parent string }{
		{"GET", "/v2/schema/proj/dev/roles/admin/ancestors", "", "admin"},
		{"GET", "/v2/facts/proj/dev/tenants/t1/users", "", "t1"},
		{"POST", "/v2/facts/proj/dev/users/u1/roles", `{"role": "admin"}`, "u1"},
	} {
		if body := h.expect(404, tc.method, tc.path, tc.body, projectKey); body["message"] !=
			"no object "+tc.parent {
			t.Errorf("%s %s under an object that does not exist answered %v", tc.method, tc.path,
				body)
		}
	}
	h.expect(200, "POST", "/v2/schema/proj/dev/groups",
		`{"group_instance_key": "g1", "group_tenant": "t1"}`, projectKey)
	if list := h.list("/v2/schema/proj/dev/groups/g1/roles", projectKey); len(list) != 0 {
		t.Errorf("the roles of a group, which the fake does not name by key, are %v", list)
	}
}

func TestResourceInstances(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	instances := "/v2/facts/proj/dev/resource_instances"
	doc := h.expect(200, "POST", instances, `{"resource": "document", "key": "a", "tenant": "t"}`,
		projectKey)
	h.expect(200, "POST", instances, `{"resource": "folder", "key": "a", "tenant": "t"}`,
		projectKey)
	if body := h.expect(409, "POST", instances, `{"resource": "document", "key": "a"}`,
		projectKey); body["message"] != "an object has the key document:a" {
		t.Errorf("a second instance document:a answered %v", body)
	}
	id, _ := doc["id"].(string)
	for _, ident := range []string{"document:a", id} {
		if got := h.expect(200, "GET", instances+"/"+ident, "", projectKey); got["id"] != id {
			t.Errorf("the instance %s is %v, want document:a", ident, got)
		}
	}
	h.expect(404, "GET", instances+"/a", "", projectKey)
	h.expect(204, "DELETE", instances+"/document:a", "", projectKey)
	h.expect(404, "GET", instances+"/document:a", "", projectKey)
	if got := h.expect(200, "GET", instances+"/folder:a", "", projectKey); got["resource"] !=
		"folder" {
		t.Errorf("the instance folder:a is %v after the deletion of document:a", got)
	}
}

// TestAttributeFilters checks that a GET of a collection lists the objects whose attributes
// match each attr_<name> of its query: a string as it is, a bool and a number as their JSON text,
// every filter at once, and no object for an attribute that none has or for a name given twice
// with two values; and that it pages what the filters keep.
func TestAttributeFilters(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	tenants := "/v2/facts/proj/dev/tenants"
	for _, body := range []string{
		`{"key": "a", "name": "a", "attributes": {"tier": "gold", "seats": 10, "on": true}}`,
		`{"key": "b", "name": "b", "attributes": {"tier": "gold", "seats": 2}}`,
		`{"key": "c", "name": "c"}`,
	} {
		h.expect(200, "POST", tenants, body, projectKey)
	}
	for query, want := range map[string][]any{
		"":                                    {"a", "b", "c"},
		"?attr_tier=gold":                     {"a", "b"},
		"?attr_tier=gold&attr_seats=10":       {"a"},
		"?attr_on=true":                       {"a"},
		"?attr_tier=silver":                   {},
		"?attr_region=eu":                     {},
		"?attr_tier=gold&attr_tier=silver":    {},
		"?attr_tier=gold&page=2&per_page=1":   {"b"},
		"?attr_seats=2&page=1&per_page=30&x=": {"b"},
	} {
		keys := []any{}
		for _, tenant := range h.list(tenants+query, projectKey) {
			o, _ := tenant.(map[string]any)
			keys = append(keys, o["key"])
		}
		if !slices.Equal(keys, want) {
			t.Errorf("GET %s%s listed %v, want %v", tenants, query, keys, want)
		}
	}
}

func TestNewChecksConfig(t *testing.T) {
	tb := &recordingTB{T: t}
	New(tb, Config{Project: "p", ProjectAPIKey: "k"})
	New(tb, Config{Check: func(string, []byte) ([]string, error) { return nil, nil }})
	want := []string{
		"fake: New needs a Check of the request bodies",
		"fake: New needs the key of the project and the project's API key",
	}
	if errs := tb.take(); !slices.Equal(errs, want) {
		t.Errorf("New reported %q, want %q", errs, want)
	}
}
