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
	{"PUT", "/v2/facts/{proj_id}/{env_id}/users/{user_id}", opReplaceUser},
	{"DELETE", "/v2/facts/{proj_id}/{env_id}/users", "delete_users"},
	{"POST", "/v2/schema/{proj_id}/{env_id}/resources", "create_resource"},
	{"POST", "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles", "create_resource_role"},
	{"GET", "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles", "list_resource_roles"},
	{
		"GET", "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}",
		"get_resource_role",
	},
	{
		"POST", "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/permissions",
		"assign_permissions_to_resource_role",
	},
	{
		"DELETE",
		"/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/permissions",
		"remove_permissions_from_resource_role",
	},
	{
		"GET", "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/ancestors",
		"get_resource_role_ancestors",
	},
	{
		"GET", "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/descendants",
		"get_resource_role_descendants",
	},
	{
		"POST",
		"/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/implicit_grants",
		"create_implicit_grant",
	},
	{
		"DELETE",
		"/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/implicit_grants",
		"delete_implicit_grant",
	},
	{
		"PUT",
		"/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/implicit_grants/" +
			"conditions",
		"update_implicit_grants_conditions",
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
	{
		"GET", "/v2/facts/{proj_id}/{env_id}/resource_instances/detailed",
		"list_resource_instances_detailed",
	},
	{"GET", "/v2/schema/{proj_id}/{env_id}/roles/{role_id}/ancestors", "get_role_ancestors"},
	{"GET", "/v2/schema/{proj_id}/{env_id}/roles/{role_id}/descendants", "get_role_descendants"},
	{"POST", "/v2/schema/{proj_id}/{env_id}/roles", "create_role"},
	{"GET", "/v2/schema/{proj_id}/{env_id}/roles/{role_id}", "get_role"},
	{
		"POST", "/v2/schema/{proj_id}/{env_id}/roles/{role_id}/permissions",
		"assign_permissions_to_role",
	},
	{
		"DELETE", "/v2/schema/{proj_id}/{env_id}/roles/{role_id}/permissions",
		"remove_permissions_from_role",
	},
	{"PUT", "/v2/schema/{proj_id}/{env_id}/bulk/roles", "bulk_create_or_replace_roles"},
	{"POST", "/v2/schema/{proj_id}/{env_id}/condition_sets", "create_condition_set"},
	{
		"PATCH", "/v2/schema/{proj_id}/{env_id}/condition_sets/{condition_set_id}",
		"update_condition_set",
	},
	{
		"GET", "/v2/schema/{proj_id}/{env_id}/condition_sets/{condition_set_id}/ancestors",
		"get_condition_set_ancestors",
	},
	{
		"GET", "/v2/schema/{proj_id}/{env_id}/condition_sets/{condition_set_id}/descendants",
		"get_condition_set_descendants",
	},
	{"GET", "/v2/facts/{proj_id}/{env_id}/set_rules", "list_set_permissions"},
	{"POST", "/v2/facts/{proj_id}/{env_id}/set_rules", "assign_set_permissions"},
	{"DELETE", "/v2/facts/{proj_id}/{env_id}/set_rules", "unassign_set_permissions"},
	{"POST", "/v2/facts/{proj_id}/{env_id}/proxy_configs", "create_proxy_config"},
	{"POST", "/v2/facts/{proj_id}/{env_id}/tenants", "create_tenant"},
	{"GET", "/v2/facts/{proj_id}/{env_id}/tenants", "list_tenants"},
	{"POST", "/v2/facts/{proj_id}/{env_id}/role_assignments", "assign_role"},
	{"GET", "/v2/facts/{proj_id}/{env_id}/role_assignments", opListRoleAssignments},
	{
		"GET", "/v2/facts/{proj_id}/{env_id}/role_assignments/detailed",
		opListRoleAssignmentsDetailed,
	},
	{"POST", "/v2/facts/{proj_id}/{env_id}/role_assignments/bulk", opBulkAssignRole},
	{"DELETE", "/v2/facts/{proj_id}/{env_id}/role_assignments/bulk", opBulkUnassignRole},
	{"POST", "/v2/facts/{proj_id}/{env_id}/relationship_tuples", "create_relationship_tuple"},
	{"GET", "/v2/facts/{proj_id}/{env_id}/relationship_tuples", opListRelationshipTuples},
	{"DELETE", "/v2/facts/{proj_id}/{env_id}/relationship_tuples", "delete_relationship_tuple"},
	{
		"GET", "/v2/facts/{proj_id}/{env_id}/relationship_tuples/detailed",
		opListRelationshipTuplesDetailed,
	},
	{
		"POST", "/v2/facts/{proj_id}/{env_id}/relationship_tuples/bulk",
		opBulkCreateRelationshipTuples,
	},
	{
		"DELETE", "/v2/facts/{proj_id}/{env_id}/relationship_tuples/bulk",
		opBulkDeleteRelationshipTuples,
	},
	{"POST", "/v2/facts/{proj_id}/{env_id}/bulk/users", opBulkCreateUsers},
	{"PUT", "/v2/facts/{proj_id}/{env_id}/bulk/users", opBulkReplaceUsers},
	{"DELETE", "/v2/facts/{proj_id}/{env_id}/bulk/users", opBulkDeleteUsers},
	{"POST", "/v2/facts/{proj_id}/{env_id}/bulk/tenants", opBulkCreateTenants},
	{"DELETE", "/v2/facts/{proj_id}/{env_id}/bulk/tenants", opBulkDeleteTenants},
	{
		"PUT", "/v2/facts/{proj_id}/{env_id}/bulk/resource_instances",
		opBulkReplaceResourceInstances,
	},
	{
		"DELETE", "/v2/facts/{proj_id}/{env_id}/bulk/resource_instances",
		opBulkDeleteResourceInstances,
	},
	{"GET", "/v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}/users", "list_tenant_users"},
	{"POST", "/v2/facts/{proj_id}/{env_id}/users/{user_id}/roles", "assign_role_to_user"},
	{"DELETE", "/v2/facts/{proj_id}/{env_id}/users/{user_id}/roles", "unassign_role_from_user"},
	{
		"POST", "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/relations",
		"create_resource_relation",
	},
	{
		"GET", "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/relations",
		"list_resource_relations",
	},
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
// check one that holds "unknown", and which lists the relations of a resource as the paginated
// object of the Permit API.
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
	f := New(tb, Config{
		Routes: routes, Check: check, Project: "proj", ProjectAPIKey: projectKey,
		Paged: func(op string) bool { return op == "list_resource_relations" },
	})
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

// TestPagedLists checks that a GET of the relations of a resource, whose operation Config.Paged
// names, answers the paginated object of the Permit API: the page of page and per_page of the
// relations in data, the first page of 30 also without either, an empty data, not null, after
// the last page and for a resource without relations, the number of the relations in
// total_count, and the number of pages of per_page of them in page_count. A list that Paged does
// not name, that of the roles, answers a JSON array, as every list of a fake without Paged does.
func TestPagedLists(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	h.expect(200, "POST", "/v2/schema/proj/dev/resources", `{"key": "document"}`, projectKey)
	relations := "/v2/schema/proj/dev/resources/document/relations"
	var keys []any
	for i := range 31 {
		key := fmt.Sprintf("r%d", i+1)
		h.expect(200, "POST", relations, `{"key": "`+key+`", "subject_resource": "folder"}`,
			projectKey)
		keys = append(keys, key)
	}
	for _, tc := range []struct {
		query        string
		want         []any
		pages, total float64
	}{
		{"", keys[:30], 2, 31},
		{"?page=2", keys[30:], 2, 31},
		{"?page=2&per_page=30", keys[30:], 2, 31},
		{"?page=3&per_page=30", []any{}, 2, 31},
		{"?per_page=100", keys, 1, 31},
		{"?page=16&per_page=2", keys[30:], 16, 31},
	} {
		page := h.expect(200, "GET", relations+tc.query, "", projectKey)
		data, ok := page["data"].([]any)
		got := []any{}
		for _, r := range data {
			o, _ := r.(map[string]any)
			got = append(got, o["key"])
		}
		if !ok || !slices.Equal(got, tc.want) || page["total_count"] != tc.total ||
			page["page_count"] != tc.pages || len(page) != 3 {
			t.Errorf("GET %s%s answered %v, want the relations %v, the total count %v and "+
				"the page count %v", relations, tc.query, page, tc.want, tc.total, tc.pages)
		}
	}
	h.expect(200, "POST", "/v2/schema/proj/dev/resources", `{"key": "folder"}`, projectKey)
	empty := h.expect(200, "GET", "/v2/schema/proj/dev/resources/folder/relations", "",
		projectKey)
	if data, ok := empty["data"].([]any); !ok || len(data) != 0 ||
		empty["total_count"] != 0.0 || empty["page_count"] != 0.0 {
		t.Errorf("the relations of a resource without relations are %v", empty)
	}
	if list := h.list("/v2/schema/proj/dev/resources/document/roles", projectKey); len(list) != 0 {
		t.Errorf("the roles of the resource are %v, want an empty JSON array", list)
	}
	accept := func(string, []byte) ([]string, error) { return nil, nil }
	plain := &harness{t: t, tb: &recordingTB{T: t}, fake: New(t, Config{
		Routes: routes, Check: accept, Project: "proj", ProjectAPIKey: projectKey,
	})}
	plain.createEnv("dev")
	plain.expect(200, "POST", "/v2/schema/proj/dev/resources", `{"key": "document"}`, projectKey)
	plain.expect(200, "POST", relations, `{"key": "r1", "subject_resource": "folder"}`, projectKey)
	if list := plain.list(relations, projectKey); len(list) != 1 {
		t.Errorf("the relations of a fake without Paged are %v, want a JSON array of one", list)
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

// TestDetailedResourceInstances checks the detailed list of the resource instances: the paginated
// object of the instances of the environment, in the order in which they were created, on the
// page of page and per_page, with the counts of every instance, each instance with an empty list
// of relationships, and the instances that the fake stores without them.
func TestDetailedResourceInstances(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	instances := "/v2/facts/proj/dev/resource_instances"
	for _, key := range []string{"a", "b"} {
		h.expect(200, "POST", instances,
			`{"resource": "document", "key": "`+key+`", "tenant": "t"}`, projectKey)
	}
	for page, want := range map[string]string{"1": "a", "2": "b"} {
		got := h.expect(200, "GET", instances+"/detailed?per_page=1&page="+page, "", projectKey)
		data, _ := got["data"].([]any)
		var instance map[string]any
		if len(data) == 1 {
			instance, _ = data[0].(map[string]any)
		}
		relationships, ok := instance["relationships"].([]any)
		if instance["key"] != want || !ok || len(relationships) != 0 ||
			got["total_count"] != 2.0 || got["page_count"] != 2.0 {
			t.Errorf("page %s of the detailed list is %v, want the instance %s with no "+
				"relationship, of 2 instances on 2 pages", page, got, want)
		}
	}
	if got := h.expect(200, "GET", instances+"/document:a", "", projectKey); got["relationships"] !=
		nil {
		t.Errorf("the instance document:a is %v after the detailed list, want no relationships",
			got)
	}
}

// TestReplaceUser checks the PUT of a user: of a user that the environment does not have, it
// creates the user, with 201 and an ID; of one that it has, by its key or its ID, it replaces the
// user with the body, with 200, and the user keeps its ID and the IDs of its organization, its
// project and its environment, and loses each field that the body leaves out; and a PUT without a
// body is a request that the fake does not implement.
func TestReplaceUser(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	users := "/v2/facts/proj/dev/users/"
	created := h.expect(201, "PUT", users+"alice",
		`{"key": "alice", "first_name": "Alice", "attributes": {"tier": "gold"}}`, projectKey)
	id, _ := created["id"].(string)
	if id == "" || created["first_name"] != "Alice" || created["organization_id"] == nil ||
		created["project_id"] != h.fake.ProjectID() || created["environment_id"] == nil {
		t.Fatalf("the PUT of a new user answered %v, want the user with an ID and a scope", created)
	}
	for _, ident := range []string{"alice", id} {
		replaced := h.expect(200, "PUT", users+ident, `{"key": "alice", "email": "a@example.com"}`,
			projectKey)
		got := h.expect(200, "GET", users+"alice", "", projectKey)
		for _, user := range []map[string]any{replaced, got} {
			_, hasName := user["first_name"]
			_, hasAttributes := user["attributes"]
			if user["id"] != id || user["email"] != "a@example.com" || hasName || hasAttributes {
				t.Errorf("the PUT of %s left the user %v, want %s with the email alone", ident,
					user, id)
			}
			for _, kept := range []string{"organization_id", "project_id", "environment_id"} {
				if user[kept] != created[kept] {
					t.Errorf("the PUT of %s left the %s %v, want %v", ident, kept, user[kept],
						created[kept])
				}
			}
		}
	}
	if n := len(h.list("/v2/facts/proj/dev/users", projectKey)); n != 1 {
		t.Errorf("the environment has %d users after the PUTs of one, want 1", n)
	}
	h.expect(501, "PUT", users+"alice", "", projectKey)
	if errs := h.tb.take(); len(errs) != 1 ||
		!strings.Contains(errs[0], "does not implement this request") {
		t.Errorf("the PUT without a body reported %q, want that it is not implemented", errs)
	}
}

// TestUserRoles checks the assignments and the unassignments of the roles of a user: an
// assignment of a role of the environment in a tenant, by their keys, answers the role
// assignment with the IDs of the user, the role and the tenant and the scope of the user, and a
// get of the user, by its key or its ID, holds the role in the tenant's entry of its
// associated_tenants; a second role, by the IDs of the role and the tenant, joins the entry; an
// unassignment removes one role and keeps the other, with 204, and a second one, or one in a
// tenant in which the user has no role, gets 404; a role or a tenant that the environment does
// not have gets 404; and a role on a resource instance, a role without a tenant, the assignment
// of a role that the user has already and a request without a body are requests that the fake
// does not implement.
func TestUserRoles(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	user := h.expect(200, "POST", "/v2/facts/proj/dev/users", `{"key": "alice"}`, projectKey)
	editor := h.expect(200, "POST", "/v2/schema/proj/dev/roles", `{"key": "editor"}`, projectKey)
	viewer := h.expect(200, "POST", "/v2/schema/proj/dev/roles", `{"key": "viewer"}`, projectKey)
	tenant := h.expect(200, "POST", "/v2/facts/proj/dev/tenants", `{"key": "t1", "name": "T1"}`,
		projectKey)
	h.expect(200, "POST", "/v2/facts/proj/dev/tenants", `{"key": "t2", "name": "T2"}`, projectKey)
	roles := "/v2/facts/proj/dev/users/alice/roles"
	assigned := h.expect(200, "POST", roles, `{"role": "editor", "tenant": "t1"}`, projectKey)
	want := map[string]any{
		"user": "alice", "role": "editor", "tenant": "t1", "user_id": user["id"],
		"role_id": editor["id"], "tenant_id": tenant["id"],
		"organization_id": user["organization_id"], "project_id": user["project_id"],
		"environment_id": user["environment_id"],
	}
	for field, value := range want {
		if assigned[field] != value {
			t.Errorf("the assignment answered the %s %v, want %v", field, assigned[field], value)
		}
	}
	if id, _ := assigned["id"].(string); id == "" || id == user["id"] {
		t.Errorf("the assignment answered the ID %q, want an ID of its own", id)
	}
	byIDs := fmt.Sprintf(`{"role": %q, "tenant": %q}`, viewer["id"], tenant["id"])
	h.expect(200, "POST", roles, byIDs, projectKey)
	h.expectTenants(user, `[{"roles":["editor","viewer"],"status":"active","tenant":"t1"}]`)
	h.expect(204, "DELETE", roles, `{"role": "editor", "tenant": "t1"}`, projectKey)
	h.expectTenants(user, `[{"roles":["viewer"],"status":"active","tenant":"t1"}]`)
	for body, message := range map[string]string{
		`{"role": "editor", "tenant": "t1"}`: "the user does not have the role editor in the " +
			"tenant t1",
		`{"role": "viewer", "tenant": "t2"}`: "the user does not have the role viewer in the " +
			"tenant t2",
		`{"role": "admin", "tenant": "t1"}`:  "no role admin",
		`{"role": "viewer", "tenant": "t3"}`: "no tenant t3",
	} {
		if got := h.expect(404, "DELETE", roles, body, projectKey); got["message"] != message {
			t.Errorf("the unassignment of %s answered %v, want the message %q", body, got,
				message)
		}
	}
	for body, message := range map[string]string{
		`{"role": "admin", "tenant": "t1"}`:  "no role admin",
		`{"role": "editor", "tenant": "t3"}`: "no tenant t3",
	} {
		if got := h.expect(404, "POST", roles, body, projectKey); got["message"] != message {
			t.Errorf("the assignment of %s answered %v, want the message %q", body, got, message)
		}
	}
	for _, r := range []struct{ method, body string }{
		{"POST", `{"role": "viewer", "tenant": "t1"}`},
		{"POST", `{"role": "editor"}`},
		{"POST", `{"role": "editor", "tenant": "t1", "resource_instance": "document:a"}`},
		{"DELETE", `{"role": "viewer", "tenant": "t1", "resource_instance": "document:a"}`},
		{"POST", ""},
		{"DELETE", ""},
	} {
		h.expect(501, r.method, roles, r.body, projectKey)
		if errs := h.tb.take(); len(errs) != 1 ||
			!strings.Contains(errs[0], "does not implement this request") {
			t.Errorf("%s %s reported %q, want that it is not implemented", r.method, r.body, errs)
		}
	}
	h.expectTenants(user, `[{"roles":["viewer"],"status":"active","tenant":"t1"}]`)
}

// TestRoleAssignmentLists checks the lists of the role assignments: the list keeps those whose
// user, role, tenant and resource instance each equal one of the values of the query parameter
// of the same name, in order, as a JSON array, and a page of them with page and per_page; the
// detailed list gives each with the objects of its user, its role, its tenant and its resource
// instance, the role of the resource of the instance for a role on an instance, and an empty
// object for one that the environment does not have, in the paginated object.
func TestRoleAssignmentLists(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	ids := map[string]any{}
	for name, seed := range map[string]struct{ path, body string }{
		"role": {"/v2/schema/proj/dev/roles", `{"key": "editor", "name": "Editor", ` +
			`"permissions": ["document:read"]}`},
		"resource": {"/v2/schema/proj/dev/resources", `{"key": "document"}`},
		"tenant":   {"/v2/facts/proj/dev/tenants", `{"key": "t1", "name": "T1"}`},
		"user":     {"/v2/facts/proj/dev/users", `{"key": "alice", "email": "a@example.com"}`},
	} {
		ids[name] = h.expect(200, "POST", seed.path, seed.body, projectKey)["id"]
	}
	ids["resource role"] = h.expect(200, "POST", "/v2/schema/proj/dev/resources/document/roles",
		`{"key": "editor", "name": "Document editor"}`, projectKey)["id"]
	ids["instance"] = h.expect(200, "POST", "/v2/facts/proj/dev/resource_instances",
		`{"resource": "document", "key": "d1", "tenant": "t1"}`, projectKey)["id"]
	assignments := "/v2/facts/proj/dev/role_assignments"
	for _, body := range []string{
		`{"user": "alice", "role": "editor", "tenant": "t1"}`,
		`{"user": "alice", "role": "editor", "tenant": "t1", "resource_instance": "document:d1"}`,
		`{"user": "bob", "role": "viewer", "tenant": "t2"}`,
	} {
		h.expect(200, "POST", assignments, body, projectKey)
	}
	const alice = "alice editor t1;alice editor t1 document:d1"
	for query, want := range map[string]string{
		"":                               alice + ";bob viewer t2",
		"?user=alice":                    alice,
		"?user=bob&user=alice":           alice + ";bob viewer t2",
		"?role=editor&tenant=t2":         "",
		"?resource_instance=document:d1": "alice editor t1 document:d1",
		"?user=alice&page=2&per_page=1":  "alice editor t1 document:d1",
	} {
		var got []string
		for _, item := range h.list(assignments+query, projectKey) {
			a, _ := item.(map[string]any)
			got = append(got, strings.TrimSpace(text(a, "user")+" "+text(a, "role")+" "+
				text(a, "tenant")+" "+text(a, "resource_instance")))
		}
		if strings.Join(got, ";") != want {
			t.Errorf("GET %s%s listed %q, want %q", assignments, query, got, want)
		}
	}
	page := h.expect(200, "GET", assignments+"/detailed?user=alice&user=bob", "", projectKey)
	items, _ := page["data"].([]any)
	if len(items) != 3 || page["total_count"] != 3.0 || page["page_count"] != 1.0 {
		t.Fatalf("the detailed list answered %v, want the page of the three role assignments",
			page)
	}
	user := fmt.Sprintf(`{"email":"a@example.com","id":%q,"key":"alice"}`, ids["user"])
	tenant := fmt.Sprintf(`{"id":%q,"key":"t1","name":"T1"}`, ids["tenant"])
	for i, want := range [][4]string{
		{
			fmt.Sprintf(`{"id":%q,"key":"editor","name":"Editor","permissions":["document:read"]}`,
				ids["role"]),
			user, tenant, "null",
		},
		{
			fmt.Sprintf(`{"id":%q,"key":"editor","name":"Document editor"}`, ids["resource role"]),
			user, tenant,
			fmt.Sprintf(`{"id":%q,"key":"d1","resource":"document"}`, ids["instance"]),
		},
		{"{}", "{}", "{}", "null"},
	} {
		item, _ := items[i].(map[string]any)
		var got [4]string
		for j, name := range []string{"role", "user", "tenant", "resource_instance"} {
			data, err := json.Marshal(item[name])
			if err != nil {
				t.Fatal(err)
			}
			got[j] = string(data)
		}
		if got != want || text(item, "id") == "" || text(item, "environment_id") == "" {
			t.Errorf("the detailed role assignment %d is %v, want its role, user, tenant and "+
				"instance %q and its IDs", i, item, want)
		}
	}
}

// TestRelationshipTuples checks the relationship tuples: the list keeps those whose tenant,
// subject, relation and object each equal the query parameter of the same name, as a JSON array,
// and the detailed list keeps the same in the paginated object; a delete of one removes it, and
// gets 404 for a tuple that the fake does not hold.
func TestRelationshipTuples(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	tuples := "/v2/facts/proj/dev/relationship_tuples"
	for _, body := range []string{
		`{"subject": "folder:f1", "relation": "parent", "object": "document:d1", "tenant": "t1"}`,
		`{"subject": "folder:f1", "relation": "parent", "object": "document:d2", "tenant": "t1"}`,
		`{"subject": "folder:f2", "relation": "owner", "object": "document:d1", "tenant": "t2"}`,
	} {
		h.expect(200, "POST", tuples, body, projectKey)
	}
	listed := func(items []any) string {
		var got []string
		for _, item := range items {
			tuple, _ := item.(map[string]any)
			if text(tuple, "id") == "" || text(tuple, "environment_id") == "" {
				t.Errorf("the relationship tuple %v has no IDs", tuple)
			}
			got = append(got, text(tuple, "subject")+" "+text(tuple, "relation")+" "+
				text(tuple, "object")+" "+text(tuple, "tenant"))
		}
		return strings.Join(got, ";")
	}
	const d1, d2 = "folder:f1 parent document:d1 t1", "folder:f1 parent document:d2 t1"
	const f2 = "folder:f2 owner document:d1 t2"
	for query, want := range map[string]string{
		"":                                       d1 + ";" + d2 + ";" + f2,
		"?subject=folder%3Af1":                   d1 + ";" + d2,
		"?relation=owner":                        f2,
		"?object=document%3Ad1&tenant=t1":        d1,
		"?subject=folder%3Af1&page=2&per_page=1": d2,
	} {
		if got := listed(h.list(tuples+query, projectKey)); got != want {
			t.Errorf("GET %s%s listed %q, want %q", tuples, query, got, want)
		}
		page := h.expect(200, "GET", tuples+"/detailed"+query, "", projectKey)
		items, _ := page["data"].([]any)
		if got := listed(items); got != want || page["total_count"] == nil {
			t.Errorf("GET %s/detailed%s answered %v, want the paginated object of %q", tuples,
				query, page, want)
		}
	}
	key := `{"subject": "folder:f2", "relation": "owner", "object": "document:d1"}`
	h.expect(204, "DELETE", tuples, key, projectKey)
	h.expect(404, "DELETE", tuples, key, projectKey)
	if got, want := listed(h.list(tuples, projectKey)), d1+";"+d2; got != want {
		t.Errorf("the relationship tuples are %q after the deletes, want %q", got, want)
	}
}

// TestBulkRelationshipTuples checks the bulk routes of the relationship tuples, whose bodies are
// JSON objects that the check of their operation sees: a bulk create creates each tuple of its
// operations, in order, in the scope of its environment, skips an item that is not an object,
// and answers an empty object, also for no tuple; a bulk delete removes each tuple that holds the
// fields of an item of its idents, to which an item that matches none adds nothing, and answers
// an empty object; a body that the check rejects gets 422 and fails the test; and a bulk call
// without a body is a request that the fake does not implement.
func TestBulkRelationshipTuples(t *testing.T) {
	h := newHarness(t)
	env, _ := h.createEnv("dev")
	const (
		tuples = "/v2/facts/proj/dev/relationship_tuples"
		bulk   = tuples + "/bulk"
		d1     = `{"subject": "folder:f1", "relation": "parent", "object": "document:d1"}`
		d2     = `{"subject": "folder:f1", "relation": "parent", "object": "document:d2"}`
		f2     = `{"subject": "folder:f2", "relation": "owner", "object": "document:d1"}`
	)
	listed := func() []string {
		t.Helper()
		var got []string
		for _, item := range h.list(tuples, projectKey) {
			tuple, _ := item.(map[string]any)
			if tuple["environment_id"] != env["id"] || text(tuple, "id") == "" {
				t.Errorf("the relationship tuple %v has no ID or another environment", tuple)
			}
			got = append(got, text(tuple, "subject")+" "+text(tuple, "relation")+" "+
				text(tuple, "object"))
		}
		return got
	}
	before := len(h.checked)
	created := h.expect(200, "POST", bulk, `{"operations": [`+d1+`, 1, `+d2+`, `+f2+`]}`,
		projectKey)
	if got := h.expect(200, "POST", bulk, `{"operations": []}`, projectKey); len(created) != 0 ||
		len(got) != 0 {
		t.Errorf("the bulk creates answered %v and %v, want empty objects", created, got)
	}
	want := []string{
		"folder:f1 parent document:d1", "folder:f1 parent document:d2",
		"folder:f2 owner document:d1",
	}
	if got := listed(); !slices.Equal(got, want) {
		t.Errorf("the relationship tuples are %q after the bulk create, want %q", got, want)
	}
	removed := h.expect(200, "DELETE", bulk, `{"idents": [`+d1+`, "x", `+d2+`, `+
		`{"subject": "folder:f9", "relation": "parent", "object": "document:d9"}]}`, projectKey)
	if len(removed) != 0 {
		t.Errorf("the bulk delete answered %v, want an empty object", removed)
	}
	if got := listed(); !slices.Equal(got, want[2:]) {
		t.Errorf("the relationship tuples are %q after the bulk delete, want %q", got, want[2:])
	}
	if got := h.checked[before:]; !slices.Equal(got, []string{
		opBulkCreateRelationshipTuples, opBulkCreateRelationshipTuples,
		opBulkDeleteRelationshipTuples,
	}) {
		t.Errorf("the check saw the bodies of %q, want those of the bulk routes", got)
	}
	h.expect(422, "DELETE", bulk, `{"idents": "bad"}`, projectKey)
	if errs := h.tb.take(); len(errs) != 1 {
		t.Errorf("the rejected body got the errors %q, want one", errs)
	}
	for _, method := range []string{"POST", "DELETE"} {
		h.do(method, bulk, "", projectKey)
		if errs := h.tb.take(); len(errs) != 1 || !strings.Contains(errs[0],
			"relationship_tuples): the fake does not implement") {
			t.Errorf("a bulk %s without a body reported %q, want that the fake does not "+
				"implement it", method, errs)
		}
	}
	if got := listed(); !slices.Equal(got, want[2:]) {
		t.Errorf("the relationship tuples are %q after the rejected requests, want %q", got,
			want[2:])
	}
}

// TestBulkRoleAssignments checks the bulk routes of the role assignments, whose bodies are JSON
// arrays that the check of their operation sees: a bulk assignment creates each item, in order,
// in the scope of its environment, and reports their number, 0 for []; a bulk unassignment
// removes the role assignments that hold the fields of an item, and reports the number that it
// removed, to which an item that matches none adds nothing; and a body that is not a JSON array
// of objects, or that the check rejects, gets 422 and fails the test.
func TestBulkRoleAssignments(t *testing.T) {
	h := newHarness(t)
	env, _ := h.createEnv("dev")
	const (
		assignments = "/v2/facts/proj/dev/role_assignments"
		bulk        = assignments + "/bulk"
		editor      = `{"user": "alice", "role": "editor", "tenant": "t1"}`
		owner       = `{"user": "alice", "role": "owner", "tenant": "t1", ` +
			`"resource_instance": "document:d1"}`
	)
	before := len(h.checked)
	created := h.expect(200, "POST", bulk,
		`[`+editor+`,`+owner+`,{"user": "bob", "role": "editor", "tenant": "t1"}]`, projectKey)
	if len(created) != 1 || created["assignments_created"] != 3.0 {
		t.Errorf("the bulk assignment answered %v, want the report of 3", created)
	}
	if got := h.expect(200, "POST", bulk, `[]`, projectKey); got["assignments_created"] != 0.0 {
		t.Errorf("the bulk assignment of [] answered %v, want the report of 0", got)
	}
	listed := func() []string {
		t.Helper()
		var got []string
		for _, item := range h.list(assignments, projectKey) {
			a, _ := item.(map[string]any)
			if a["environment_id"] != env["id"] || text(a, "id") == "" {
				t.Errorf("the role assignment %v has no ID or another environment", a)
			}
			got = append(got, strings.TrimSpace(text(a, "user")+" "+text(a, "role")+" "+
				text(a, "resource_instance")))
		}
		return got
	}
	if want := []string{"alice editor", "alice owner document:d1", "bob editor"}; !slices.Equal(
		listed(), want) {
		t.Errorf("the role assignments are %q after the bulk assignment, want %q", listed(), want)
	}
	removed := h.expect(200, "DELETE", bulk,
		`[`+editor+`,`+owner+`,{"user": "carol", "role": "editor", "tenant": "t1"}]`, projectKey)
	if len(removed) != 1 || removed["assignments_removed"] != 2.0 {
		t.Errorf("the bulk unassignment answered %v, want the report of 2", removed)
	}
	if want := []string{"bob editor"}; !slices.Equal(listed(), want) {
		t.Errorf("the role assignments are %q after the bulk unassignment, want %q", listed(),
			want)
	}
	if got := h.checked[before:]; !slices.Equal(got, []string{
		opBulkAssignRole, opBulkAssignRole, opBulkUnassignRole,
	}) {
		t.Errorf("the check saw the bodies of %q, want those of the bulk routes", got)
	}
	for _, body := range []string{`{"user": "a"}`, `[1]`, `[null]`, `null`, `[{"key": "bad"}]`} {
		h.expect(422, "POST", bulk, body, projectKey)
		if errs := h.tb.take(); len(errs) != 1 {
			t.Errorf("the body %s got the errors %q, want one", body, errs)
		}
	}
	if want := []string{"bob editor"}; !slices.Equal(listed(), want) {
		t.Errorf("the role assignments are %q after the rejected bodies, want %q", listed(), want)
	}
}

// TestBulkFacts checks the bulk routes of the users, the tenants and the resource instances,
// whose bodies are JSON objects that the check of their operation sees, each answered with 200
// and {}: a bulk create creates each object of operations, in order, in the scope of its
// environment, and a 409 for a key that the collection has ends it and writes nothing, not even
// the objects before it; a bulk replace replaces each object that the collection has, by its
// identifier, which keeps its ID and the IDs of its organization, project and environment and
// loses the fields that its operation leaves out, and creates each that it does not have;
// a bulk delete deletes each object that an ident names, by its key, its ID or, for an instance,
// <resource>:<key>, and an ident that names none deletes nothing. A body that the check rejects
// gets 422 and fails the test.
func TestBulkFacts(t *testing.T) {
	h := newHarness(t)
	env, _ := h.createEnv("dev")
	const (
		facts = "/v2/facts/proj/dev"
		bulk  = facts + "/bulk"
	)
	before := len(h.checked)
	keys := func(path string) []string {
		t.Helper()
		var got []string
		for _, item := range h.list(path, projectKey) {
			o, _ := item.(map[string]any)
			if o["environment_id"] != env["id"] || text(o, "id") == "" {
				t.Errorf("the object %v of %s has no ID or another environment", o, path)
			}
			got = append(got, text(o, "key"))
		}
		return got
	}
	answered := func(want int, method, path, body string) {
		t.Helper()
		if got := h.expect(want, method, path, body, projectKey); want == 200 && len(got) != 0 {
			t.Errorf("%s %s answered %v, want {}", method, path, got)
		}
	}
	answered(200, "POST", bulk+"/tenants",
		`{"operations": [{"key": "t1", "name": "T1"}, {"key": "t2", "name": "T2"}]}`)
	if got, want := keys(facts+"/tenants"), []string{"t1", "t2"}; !slices.Equal(got, want) {
		t.Errorf("the tenants are %q after the bulk create, want %q", got, want)
	}
	answered(200, "POST", bulk+"/users",
		`{"operations": [{"key": "alice", "email": "alice@example.com"}, {"key": "bob"}]}`)
	answered(409, "POST", bulk+"/users",
		`{"operations": [{"key": "carol"}, {"key": "alice"}, {"key": "dave"}]}`)
	if got, want := keys(facts+"/users"), []string{"alice", "bob"}; !slices.Equal(got, want) {
		t.Errorf("the users are %q after the bulk creates, want %q", got, want)
	}
	alice := h.expect(200, "GET", facts+"/users/alice", "", projectKey)
	bob := h.expect(200, "GET", facts+"/users/bob", "", projectKey)
	answered(200, "PUT", bulk+"/users",
		`{"operations": [{"key": "alice", "first_name": "Alice"}, {"key": "erin"}]}`)
	replaced := h.expect(200, "GET", facts+"/users/alice", "", projectKey)
	if replaced["first_name"] != "Alice" || replaced["email"] != nil {
		t.Errorf("alice is %v after the bulk replace, want the first name alone", replaced)
	}
	for _, kept := range []string{"id", "organization_id", "project_id", "environment_id"} {
		if replaced[kept] == nil || replaced[kept] != alice[kept] {
			t.Errorf("alice has the %s %v after the bulk replace, want %v", kept, replaced[kept],
				alice[kept])
		}
	}
	answered(200, "PUT", bulk+"/resource_instances",
		`{"operations": [{"key": "d1", "tenant": "t1", "resource": "document"}]}`)
	instance := h.expect(200, "GET", facts+"/resource_instances/document:d1", "", projectKey)
	answered(200, "PUT", bulk+"/resource_instances",
		`{"operations": [{"key": "d1", "tenant": "t2", "resource": "document"}]}`)
	moved := h.expect(200, "GET", facts+"/resource_instances/document:d1", "", projectKey)
	if moved["id"] != instance["id"] || moved["tenant"] != "t2" {
		t.Errorf("the instance is %v after the second bulk replace, want %v in t2", moved,
			instance["id"])
	}
	answered(200, "DELETE", bulk+"/resource_instances", `{"idents": ["document:d1"]}`)
	h.expect(404, "GET", facts+"/resource_instances/document:d1", "", projectKey)
	answered(200, "DELETE", bulk+"/users",
		`{"idents": ["alice", "`+text(bob, "id")+`", "nobody"]}`)
	if got, want := keys(facts+"/users"), []string{"erin"}; !slices.Equal(got, want) {
		t.Errorf("the users are %q after the bulk delete, want %q", got, want)
	}
	answered(200, "DELETE", bulk+"/tenants", `{"idents": ["t1", "t2"]}`)
	if got := keys(facts + "/tenants"); len(got) != 0 {
		t.Errorf("the tenants are %q after the bulk delete, want none", got)
	}
	if got := h.checked[before:]; !slices.Equal(got, []string{
		opBulkCreateTenants, opBulkCreateUsers, opBulkCreateUsers, opBulkReplaceUsers,
		opBulkReplaceResourceInstances, opBulkReplaceResourceInstances,
		opBulkDeleteResourceInstances, opBulkDeleteUsers, opBulkDeleteTenants,
	}) {
		t.Errorf("the check saw the bodies of %q, want those of the bulk routes", got)
	}
	h.expect(422, "POST", bulk+"/users", `{"operations": [{"key": "bad"}]}`, projectKey)
	if errs := h.tb.take(); len(errs) != 1 {
		t.Errorf("the rejected body got the errors %q, want one", errs)
	}
}

// expectTenants checks that a get of user, by its key and by its ID, answers the
// associated_tenants of want, as JSON with its keys in order.
func (h *harness) expectTenants(user map[string]any, want string) {
	h.t.Helper()
	id, _ := user["id"].(string)
	for _, ident := range []string{"alice", id} {
		got := h.expect(200, "GET", "/v2/facts/proj/dev/users/"+ident, "", projectKey)
		data, err := json.Marshal(got["associated_tenants"])
		if err != nil || string(data) != want {
			h.t.Errorf("the user %s has the associated_tenants %s (%v), want %s", ident, data, err,
				want)
		}
	}
}

// TestAttributeFilters checks that a GET of a collection lists the objects whose attributes
// match each attr_<name> of its query: a string as it is, a bool and a number as their JSON text,
// every filter at once, and no object for an attribute that none has, for a name given twice
// with two values, or for an attribute whose value is an object or an array; and that it pages
// what the filters keep.
func TestAttributeFilters(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	tenants := "/v2/facts/proj/dev/tenants"
	for _, body := range []string{
		`{"key": "a", "name": "a", "attributes": {"tier": "gold", "seats": 10, "on": true}}`,
		`{"key": "b", "name": "b", "attributes": {"tier": "gold", "seats": 2}}`,
		`{"key": "c", "name": "c"}`,
		`{"key": "d", "name": "d", "attributes": {"tier": {"value": "gold"}, "seats": [10]}}`,
	} {
		h.expect(200, "POST", tenants, body, projectKey)
	}
	for query, want := range map[string][]any{
		"":                                    {"a", "b", "c", "d"},
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

// TestRolePermissions checks the grants and removals of the permissions of a role, which keep the
// permissions that the role has, skip one that it has already or does not have, and answer 404
// for a role that does not exist.
func TestRolePermissions(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	roles := "/v2/schema/proj/dev/roles"
	viewer := h.expect(200, "POST", roles, `{"key": "viewer", "name": "Viewer"}`, projectKey)
	for _, tc := range []struct{ method, body, want string }{
		{"POST", `{"permissions": ["doc:read", "doc:write"]}`, "[doc:read doc:write]"},
		{"POST", `{"permissions": ["doc:read", "doc:share"]}`, "[doc:read doc:write doc:share]"},
		{"DELETE", `{"permissions": ["doc:write", "doc:none"]}`, "[doc:read doc:share]"},
		{"DELETE", `{"permissions": ["doc:read", "doc:share"]}`, "[]"},
	} {
		role := h.expect(200, tc.method, roles+"/viewer/permissions", tc.body, projectKey)
		if got := fmt.Sprint(role["permissions"]); got != tc.want || role["id"] != viewer["id"] {
			t.Errorf("%s %s answered the role %v, want the permissions %s", tc.method, tc.body,
				role, tc.want)
		}
	}
	h.expect(404, "POST", roles+"/none/permissions", `{"permissions": ["doc:read"]}`, projectKey)
}

// TestRoleRelatives checks the ancestors and the descendants of a role: the roles that it
// extends, or that extend it, directly or through other roles, each once, in the order in which
// the fake finds them, as a JSON array in roles, which is empty, not null, for a role without
// any; a role named by its ID; a cycle of roles that extend each other, which ends; and 404 for a
// role that does not exist.
func TestRoleRelatives(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	roles := "/v2/schema/proj/dev/roles"
	for _, body := range []string{
		`{"key": "viewer"}`, `{"key": "editor", "extends": ["viewer"]}`,
		`{"key": "admin", "extends": ["editor", "viewer"]}`, `{"key": "other"}`,
		`{"key": "x", "extends": ["y"]}`, `{"key": "y", "extends": ["x"]}`,
	} {
		h.expect(200, "POST", roles, body, projectKey)
	}
	viewer := h.expect(200, "GET", roles+"/viewer", "", projectKey)
	viewerID, _ := viewer["id"].(string)
	for _, tc := range []struct{ path, want string }{
		{"/admin/ancestors", "[viewer editor]"},
		{"/editor/ancestors", "[viewer]"},
		{"/viewer/ancestors", "[]"},
		{"/viewer/descendants", "[editor admin]"},
		{"/" + viewerID + "/descendants", "[editor admin]"},
		{"/admin/descendants", "[]"},
		{"/x/ancestors", "[y]"},
		{"/x/descendants", "[y]"},
	} {
		reply := h.expect(200, "GET", roles+tc.path, "", projectKey)
		list, ok := reply["roles"].([]any)
		keys := []any{}
		for _, role := range list {
			o, _ := role.(map[string]any)
			keys = append(keys, o["key"])
		}
		if got := fmt.Sprint(keys); !ok || len(reply) != 1 || got != tc.want {
			t.Errorf("GET %s answered %v, want the roles %s alone", tc.path, reply, tc.want)
		}
	}
	h.expect(404, "GET", roles+"/none/ancestors", "", projectKey)
	h.expect(404, "GET", roles+"/none/descendants", "", projectKey)
}

// TestConditionSetRelatives checks the ancestors and the descendants of a condition set, by its
// key or its ID: the sets that its parent_id leads to, its parent first, and the sets whose
// parent_id leads to it, each once, as a JSON array of the page of the query, and never the set
// itself, also when the parent_id fields of the sets make a cycle; and 404 for a set that the
// environment does not have.
func TestConditionSetRelatives(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	sets := "/v2/schema/proj/dev/condition_sets"
	ids := map[string]string{}
	for _, tc := range []struct{ key, parent string }{
		{"root", ""}, {"mid", "root"}, {"leaf", "mid"}, {"twin", "root"}, {"other", ""},
	} {
		body := `{"key": "` + tc.key + `"}`
		if tc.parent != "" {
			body = `{"key": "` + tc.key + `", "parent_id": "` + ids[tc.parent] + `"}`
		}
		created := h.expect(200, "POST", sets, body, projectKey)
		ids[tc.key], _ = created["id"].(string)
	}
	for _, tc := range []struct{ path, want string }{
		{"/leaf/ancestors", "[mid root]"},
		{"/" + ids["leaf"] + "/ancestors", "[mid root]"},
		{"/root/ancestors", "[]"},
		{"/root/descendants", "[mid twin leaf]"},
		{"/root/descendants?page=2&per_page=2", "[leaf]"},
		{"/leaf/descendants", "[]"},
	} {
		h.expectRelatives(sets+tc.path, tc.want)
	}
	h.expect(200, "PATCH", sets+"/root", `{"parent_id": "`+ids["leaf"]+`"}`, projectKey)
	h.expectRelatives(sets+"/leaf/ancestors", "[mid root]")
	h.expectRelatives(sets+"/root/descendants", "[mid twin leaf]")
	h.expect(404, "GET", sets+"/none/ancestors", "", projectKey)
	h.expect(404, "GET", sets+"/none/descendants", "", projectKey)
}

// expectRelatives fails the test unless GET of path, with the project's API key, answers 200 and
// a JSON array of the condition sets whose keys, in order, print as want.
func (h *harness) expectRelatives(path, want string) {
	h.t.Helper()
	var keys []any
	for _, set := range h.list(path, projectKey) {
		o, _ := set.(map[string]any)
		keys = append(keys, o["key"])
	}
	if got := fmt.Sprint(keys); got != want {
		h.t.Errorf("GET %s answered the sets %s, want %s", path, got, want)
	}
}

// TestSetRules checks the condition set rules: a create answers a JSON array of the rule, with an
// ID and the IDs of its organization, project and environment; the list keeps the rules that
// match each of the filters user_set, permission and resource_set of its query, in the order of
// their creates, one page of them with page or per_page; a create without a body, or of a rule
// whose fields another rule has, is not implemented; and a delete of a rule by its fields removes
// it, with 404 for a rule that the fake does not hold, as for a DELETE of a collection with a body.
// The API's documentation says that it skips the delete of a rule that it does not hold; what the
// API answers then is not confirmed yet.
func TestSetRules(t *testing.T) {
	h := newHarness(t)
	env, _ := h.createEnv("dev")
	rules := "/v2/facts/proj/dev/set_rules"
	body := func(userSet, permission, resourceSet string) string {
		return `{"user_set": "` + userSet + `", "permission": "` + permission +
			`", "resource_set": "` + resourceSet + `"}`
	}
	for _, tc := range [][3]string{
		{"adults", "doc:read", "public"},
		{"adults", "doc:write", "public"},
		{"seniors", "doc:read", "public"},
		{"adults", "doc:read", "private"},
	} {
		status, reply := h.do("POST", rules, body(tc[0], tc[1], tc[2]), projectKey)
		created, _ := reply.([]any)
		var rule map[string]any
		if len(created) == 1 {
			rule, _ = created[0].(map[string]any)
		}
		if status != 200 || rule == nil || rule["user_set"] != tc[0] ||
			rule["permission"] != tc[1] || rule["resource_set"] != tc[2] || rule["id"] == nil ||
			rule["organization_id"] == nil || rule["project_id"] == nil ||
			rule["environment_id"] != env["id"] {
			t.Errorf("POST of the rule %v answered %d %v, want a JSON array of the rule with "+
				"its IDs", tc, status, reply)
		}
	}
	for _, tc := range []struct{ query, want string }{
		{"", "[adults/doc:read/public adults/doc:write/public seniors/doc:read/public " +
			"adults/doc:read/private]"},
		{"?user_set=adults", "[adults/doc:read/public adults/doc:write/public " +
			"adults/doc:read/private]"},
		{
			"?user_set=adults&permission=doc%3Aread",
			"[adults/doc:read/public adults/doc:read/private]",
		},
		{"?resource_set=private", "[adults/doc:read/private]"},
		{"?user_set=adults&page=2&per_page=2", "[adults/doc:read/private]"},
		{"?user_set=none", "[]"},
	} {
		h.expectRules(rules+tc.query, tc.want)
	}
	for _, request := range []string{"", body("adults", "doc:read", "public")} {
		h.do("POST", rules, request, projectKey)
		if errs := h.tb.take(); len(errs) != 1 || !strings.Contains(errs[0],
			"assign_set_permissions): the fake does not implement this request") {
			t.Errorf("POST of %q reported %q, want that the fake does not implement it", request,
				errs)
		}
	}
	h.expect(204, "DELETE", rules, body("adults", "doc:read", "public"), projectKey)
	h.expectRules(
		rules+"?permission=doc%3Aread",
		"[seniors/doc:read/public adults/doc:read/private]",
	)
	h.expect(404, "DELETE", rules, body("adults", "doc:read", "public"), projectKey)
}

// expectRules fails the test unless GET of path, with the project's API key, answers 200 and a
// JSON array of the condition set rules that print as want, each as its user set, its
// permission and its resource set joined by slashes, in order.
func (h *harness) expectRules(path, want string) {
	h.t.Helper()
	got := []string{}
	for _, rule := range h.list(path, projectKey) {
		o, _ := rule.(map[string]any)
		got = append(got, fmt.Sprint(o["user_set"], "/", o["permission"], "/", o["resource_set"]))
	}
	if fmt.Sprint(got) != want {
		h.t.Errorf("GET %s answered the rules %v, want %s", path, got, want)
	}
}

// TestBulkRoles checks the bulk replace of roles, which creates a role that does not exist and
// replaces the definition of one that does, keeping its ID and the IDs of its organization,
// project and environment, and answers with the keys of each; that a create that conflicts, of a
// role whose key is the ID of another role, ends it with the 409 of the create, and lists no key
// as created; and that a bulk replace without a body is a request that the fake does not
// implement.
func TestBulkRoles(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	roles := "/v2/schema/proj/dev/roles"
	bulk := "/v2/schema/proj/dev/bulk/roles"
	viewer := h.expect(200, "POST", roles, `{"key": "viewer", "name": "Viewer", "permissions": `+
		`["doc:read"]}`, projectKey)
	result := h.expect(200, "PUT", bulk, `{"operations": [`+
		`{"key": "viewer", "name": "Reader"}, {"key": "editor", "name": "Editor"}]}`, projectKey)
	if fmt.Sprint(result) != "map[created:[editor] updated:[viewer]]" {
		t.Errorf("the bulk replace answered %v, want editor created and viewer updated", result)
	}
	got := h.expect(200, "GET", roles+"/viewer", "", projectKey)
	if got["name"] != "Reader" || got["permissions"] != nil {
		t.Errorf("the replaced role is %v, want viewer with the name Reader and no permissions",
			got)
	}
	for _, kept := range []string{"id", "organization_id", "project_id", "environment_id"} {
		if got[kept] == nil || got[kept] != viewer[kept] {
			t.Errorf("the replaced role has the %s %v, want %v, as before the replace", kept,
				got[kept], viewer[kept])
		}
	}
	editor := h.expect(200, "GET", roles+"/editor", "", projectKey)
	if editor["id"] == nil || editor["name"] != "Editor" ||
		editor["environment_id"] != viewer["environment_id"] {
		t.Errorf("the created role is %v, want editor with an ID in the environment", editor)
	}
	viewerID, _ := viewer["id"].(string)
	conflict := h.expect(409, "PUT", bulk, `{"operations": [{"key": "`+viewerID+`", "name": `+
		`"Other"}]}`, projectKey)
	if conflict["message"] != "an object has the key "+viewerID || conflict["created"] != nil {
		t.Errorf("a bulk replace that creates a role whose key is the ID of viewer answered %v, "+
			"want the conflict of the create", conflict)
	}
	h.do("PUT", bulk, "", projectKey)
	if errs := h.tb.take(); len(errs) != 1 || !strings.Contains(errs[0],
		"bulk_create_or_replace_roles): the fake does not implement") {
		t.Errorf("a bulk replace without a body reported %q, want that the fake does not "+
			"implement it", errs)
	}
}

// TestResourceRolePermissions checks the grants and removals of the permissions of a role of a
// resource, which keep the permissions that the role has and skip one that it has already or
// does not have, a permission given as <resource>:<action> being the action's key, and change
// that role alone, not the role of another resource or of the environment with its key; that a
// grant to a role of the environment whose key is a resource's keeps <resource>:<action> as it
// is given; and that a role or a resource that does not exist gets 404.
func TestResourceRolePermissions(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	schema := "/v2/schema/proj/dev"
	h.expect(200, "POST", schema+"/resources", `{"key": "doc"}`, projectKey)
	h.expect(200, "POST", schema+"/resources", `{"key": "folder"}`, projectKey)
	for _, path := range []string{"/resources/doc/roles", "/resources/folder/roles", "/roles"} {
		h.expect(200, "POST", schema+path, `{"key": "editor", "permissions": ["read"]}`,
			projectKey)
	}
	roles := schema + "/resources/doc/roles"
	for _, tc := range []struct{ method, body, want string }{
		{"POST", `{"permissions": ["read", "write"]}`, "[read write]"},
		{"POST", `{"permissions": ["doc:read", "share"]}`, "[read write share]"},
		{"DELETE", `{"permissions": ["doc:write", "none"]}`, "[read share]"},
	} {
		reply := h.expect(200, tc.method, roles+"/editor/permissions", tc.body, projectKey)
		if got := fmt.Sprint(reply["permissions"]); got != tc.want {
			t.Errorf("%s %s answered %v, want the permissions %s", tc.method, tc.body, reply,
				tc.want)
		}
	}
	for _, path := range []string{"/resources/folder/roles/editor", "/roles/editor"} {
		if got := h.expect(200, "GET", schema+path, "", projectKey); fmt.Sprint(
			got["permissions"]) != "[read]" {
			t.Errorf("GET %s answered %v, want the permissions [read] that it was created with",
				path, got)
		}
	}
	h.expect(200, "POST", schema+"/roles", `{"key": "doc"}`, projectKey)
	granted := h.expect(200, "POST", schema+"/roles/doc/permissions",
		`{"permissions": ["doc:read"]}`, projectKey)
	if got := fmt.Sprint(granted["permissions"]); got != "[doc:read]" {
		t.Errorf("POST /roles/doc/permissions of doc:read answered %v, want the permissions "+
			"[doc:read] as given", granted)
	}
	h.expect(404, "POST", roles+"/none/permissions", `{"permissions": ["read"]}`, projectKey)
	h.expect(404, "DELETE", schema+"/resources/none/roles/editor/permissions",
		`{"permissions": ["read"]}`, projectKey)
}

// TestResourceRoleRelatives checks the ancestors and the descendants of a role of a resource: the
// roles of its resource that it extends, or that extend it, directly or through other roles, each
// once, in the order in which the fake finds them, as a JSON array in roles, which is empty, not
// null, for a role without any; not the roles of another resource or of the environment with the
// same keys; a role named by its ID; and 404 for a role or a resource that does not exist.
func TestResourceRoleRelatives(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	schema := "/v2/schema/proj/dev"
	h.expect(200, "POST", schema+"/resources", `{"key": "doc"}`, projectKey)
	h.expect(200, "POST", schema+"/resources", `{"key": "folder"}`, projectKey)
	for path, top := range map[string]string{
		"/resources/doc/roles": "owner", "/resources/folder/roles": "manager", "/roles": "admin",
	} {
		for _, body := range []string{
			`{"key": "viewer"}`, `{"key": "editor", "extends": ["viewer"]}`,
			`{"key": "` + top + `", "extends": ["editor", "viewer"]}`,
		} {
			h.expect(200, "POST", schema+path, body, projectKey)
		}
	}
	roles := schema + "/resources/doc/roles"
	viewer := h.expect(200, "GET", roles+"/viewer", "", projectKey)
	viewerID, _ := viewer["id"].(string)
	for _, tc := range []struct{ path, want string }{
		{"/owner/ancestors", "[viewer editor]"},
		{"/editor/ancestors", "[viewer]"},
		{"/viewer/ancestors", "[]"},
		{"/viewer/descendants", "[editor owner]"},
		{"/" + viewerID + "/descendants", "[editor owner]"},
		{"/owner/descendants", "[]"},
	} {
		reply := h.expect(200, "GET", roles+tc.path, "", projectKey)
		list, ok := reply["roles"].([]any)
		keys := []any{}
		for _, role := range list {
			o, _ := role.(map[string]any)
			keys = append(keys, o["key"])
		}
		if got := fmt.Sprint(keys); !ok || len(reply) != 1 || got != tc.want {
			t.Errorf("GET %s answered %v, want the roles %s alone", tc.path, reply, tc.want)
		}
	}
	h.expect(404, "GET", roles+"/none/ancestors", "", projectKey)
	h.expect(404, "GET", roles+"/none/descendants", "", projectKey)
	h.expect(404, "GET", schema+"/resources/none/roles/viewer/ancestors", "", projectKey)
}

// TestImplicitGrants checks the implicit grants of the role viewer of the resource doc: a create
// of the rule of the role owner of folder, linked by the relation parent of doc, named by their
// keys or by their IDs, answers the rule with the IDs of the role, the resource and the relation,
// and adds it to the users_with_role of the granted_to of viewer; a rule whose resource, role or
// relation does not exist gets 404 with a message that names it, and a role of the path that
// does not exist 404; a change of the conditions answers them and sets the when of the
// granted_to, each time as a whole; a delete removes each rule that names the role, the resource
// and the relation of its body, and answers 204, or 404 when there is none, such as for a body
// that differs from a rule in one of the three; and a request without a body is not implemented.
func TestImplicitGrants(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	schema := "/v2/schema/proj/dev"
	folder := h.expect(200, "POST", schema+"/resources", `{"key": "folder"}`, projectKey)
	h.expect(200, "POST", schema+"/resources", `{"key": "doc"}`, projectKey)
	relation := h.expect(200, "POST", schema+"/resources/doc/relations",
		`{"key": "parent", "subject_resource": "folder"}`, projectKey)
	owner := h.expect(200, "POST", schema+"/resources/folder/roles", `{"key": "owner"}`,
		projectKey)
	h.expect(200, "POST", schema+"/resources/doc/roles", `{"key": "viewer"}`, projectKey)
	jsonOf := func(v any) string {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	grantedTo := func() string {
		viewer := h.expect(200, "GET", schema+"/resources/doc/roles/viewer", "", projectKey)
		return jsonOf(viewer["granted_to"])
	}
	grants := schema + "/resources/doc/roles/viewer/implicit_grants"
	byKey := `{"role": "owner", "on_resource": "folder", "linked_by_relation": "parent"}`
	byID := jsonOf(map[string]any{
		"role": owner["id"], "on_resource": folder["id"], "linked_by_relation": relation["id"],
	})
	ids := `"relation_id":"` + text(relation, "id") + `","resource_id":"` + text(folder, "id") +
		`","role":"%s","role_id":"` + text(owner, "id") + `"`
	keyRule := `{"linked_by_relation":"parent","on_resource":"folder",` +
		fmt.Sprintf(ids, "owner") + `}`
	idRule := `{"linked_by_relation":"` + text(relation, "id") + `","on_resource":"` +
		text(folder, "id") + `",` + fmt.Sprintf(ids, text(owner, "id")) + `}`
	if got := jsonOf(h.expect(200, "POST", grants, byKey, projectKey)); got != keyRule {
		t.Errorf("a create of the rule by keys answered %s, want %s", got, keyRule)
	}
	if got := jsonOf(h.expect(200, "POST", grants, byID, projectKey)); got != idRule {
		t.Errorf("a create of the rule by IDs answered %s, want %s", got, idRule)
	}
	if got, want := grantedTo(), `{"users_with_role":[`+keyRule+`,`+idRule+`]}`; got != want {
		t.Errorf("viewer has the granted_to %s, want %s", got, want)
	}
	for _, tc := range []struct{ role, resource, relation, want string }{
		{"owner", "none", "parent", "no resource none"},
		{"viewer", "folder", "parent", "no role viewer"},
		{"owner", "folder", "none", "no relation none"},
	} {
		body := jsonOf(map[string]string{
			"role": tc.role, "on_resource": tc.resource, "linked_by_relation": tc.relation,
		})
		if got := h.expect(404, "POST", grants, body, projectKey); got["message"] != tc.want {
			t.Errorf("a create of %s answered %v, want 404 with the message %q", body, got,
				tc.want)
		}
	}
	h.expect(404, "POST", schema+"/resources/doc/roles/none/implicit_grants", byKey, projectKey)
	for _, when := range []string{`{"no_direct_roles_on_object":true}`, `{}`} {
		if got := jsonOf(h.expect(200, "PUT", grants+"/conditions", when, projectKey)); got !=
			when {
			t.Errorf("a change of the conditions to %s answered %s", when, got)
		}
	}
	for _, other := range []map[string]string{
		{"role": "viewer", "on_resource": "folder", "linked_by_relation": "parent"},
		{"role": "owner", "on_resource": "doc", "linked_by_relation": "parent"},
		{"role": "owner", "on_resource": "folder", "linked_by_relation": "child"},
	} {
		h.expect(404, "DELETE", grants, jsonOf(other), projectKey)
	}
	h.expect(204, "DELETE", grants, byKey, projectKey)
	if got, want := grantedTo(), `{"users_with_role":[`+idRule+`],"when":{}}`; got != want {
		t.Errorf("after the delete of the rule by keys, viewer has the granted_to %s, want %s",
			got, want)
	}
	h.expect(404, "DELETE", grants, byKey, projectKey)
	h.expect(204, "DELETE", grants, byID, projectKey)
	for _, tc := range []struct{ method, path string }{
		{"POST", grants}, {"DELETE", grants}, {"PUT", grants + "/conditions"},
	} {
		h.do(tc.method, tc.path, "", projectKey)
		if errs := h.tb.take(); len(errs) != 1 || !strings.Contains(errs[0],
			"the fake does not implement") {
			t.Errorf("%s %s without a body reported %q, want that the fake does not implement "+
				"it", tc.method, tc.path, errs)
		}
	}
}
