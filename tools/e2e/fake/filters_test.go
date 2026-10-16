package fake

import (
	"slices"
	"strings"
	"testing"
)

// expectNames fails the test unless a GET of path answers 200 with the objects whose names, as
// nameOf gives them, are want, in order: a JSON array of them, or the paginated object with them
// in data.
func (h *harness) expectNames(path string, want ...string) {
	h.t.Helper()
	status, reply := h.do("GET", path, "", projectKey)
	items, ok := reply.([]any)
	if page, isPage := reply.(map[string]any); isPage {
		items, ok = page["data"].([]any)
	}
	got := []string{}
	for _, item := range items {
		o, _ := item.(map[string]any)
		got = append(got, nameOf(o))
	}
	if status != 200 || !ok || !slices.Equal(got, append([]string{}, want...)) {
		h.t.Errorf("GET %s answered %d %v, the objects %q; want 200 and the objects %q", path,
			status, reply, got, want)
	}
}

// TestShortListRoutes checks that the lists whose path names no environment, those of the
// projects and of the API keys, answer 200 to a GET without a query, with each object of the
// collection, as no filter reads their path.
func TestShortListRoutes(t *testing.T) {
	h := newHarness(t)
	key := h.expect(200, "POST", "/v2/api-key", `{"object_type": "org"}`, projectKey)
	h.expectNames("/v2/projects", "proj")
	h.expectNames("/v2/api-key", "with the ID "+text(key, "id"))
}

// TestListFilters checks each filter of a list that the fake applies, as listFilters says: a key
// or an ID that the object holds, exactly, and a boolean as its JSON text; one of the values of a
// filter of the role assignments, and of the users and the resources of the audit logs, the
// users by their key, email or name; the resource of the subject or the object of a relationship
// tuple; the text of a search in the fields that it reads, case and all, each text of a search of
// the resource instances, and the email or the key of an invite, exactly; the roles of a user in
// a tenant, from a role assignment of the user, its associated_tenants or a role assignment of
// its create, and never a role on an instance or a role assignment of another user, and for the
// users of a tenant those in that tenant alone; and the time window of the audit logs, which
// holds its bounds and applies only when the query holds both and neither is 0. The plain and the
// detailed list of the resource instances and of the relationship tuples filter alike, and the
// attribute filters of the roles and the actions apply as those of the tenants do.
func TestListFilters(t *testing.T) {
	h := newHarness(t)
	env, _ := h.createEnv("dev")
	const (
		schema = "/v2/schema/proj/dev"
		facts  = "/v2/facts/proj/dev"
		logs   = "/v2/pdps/proj/dev/audit_logs"
	)
	for _, seed := range []struct{ path, body string }{
		{schema + "/roles", `{"key": "editor", "attributes": {"kind": "parent"}}`},
		{schema + "/roles", `{"key": "viewer", "attributes": {"kind": "child"}}`},
		{schema + "/roles", `{"key": "owner"}`},
		{schema + "/resources", `{"key": "document", "name": "Paper"}`},
		{schema + "/resources", `{"key": "folder", "name": "Documents"}`},
		{schema + "/resources/document/actions", `{"key": "read", "attributes": {"risk": "low"}}`},
		{schema + "/resources/document/actions", `{"key": "wipe", "attributes": {"risk": "high"}}`},
		{schema + "/condition_sets", `{"key": "adults", "name": "Adults", "type": "userset"}`},
		{schema + "/condition_sets", `{"key": "docs", "name": "Public", "type": "resourceset"}`},
		{facts + "/tenants", `{"key": "t1", "name": "T1"}`},
		{facts + "/tenants", `{"key": "t2", "name": "T2", "attributes": {"tier": "gold"}}`},
		{facts + "/users", `{"key": "alice", "email": "alice@example.com", ` +
			`"role_assignments": [{"role": "editor", "tenant": "t1"}]}`},
		{facts + "/users", `{"key": "bob", "email": "bob@example.org"}`},
		{facts + "/users", `{"key": "carol", "email": "carol@example.com", "role_assignments": ` +
			`[{"role": "owner", "tenant": "t1", "resource_instance": "document:d1"}]}`},
		{facts + "/users", `{"key": "dave"}`},
		{facts + "/users", `{"key": "erin"}`},
		{facts + "/role_assignments", `{"user": "dave", "role": "viewer", "tenant": "t1"}`},
		{facts + "/role_assignments", `{"user": "dave", "role": "owner", "tenant": "t1", ` +
			`"resource_instance": "document:d1"}`},
		{facts + "/role_assignments", `{"user": "nobody", "role": "editor", "tenant": "t2"}`},
		{facts + "/role_assignments", `{"user": "gina", "role": "editor", "tenant": "t1"}`},
		{facts + "/users/erin/roles", `{"role": "viewer", "tenant": "t2"}`},
		{facts + "/tenants/t1/users", `{"key": "frank", "email": "frank@example.com", ` +
			`"role_assignments": [{"role": "viewer", "tenant": "t1"}, ` +
			`{"role": "owner", "tenant": "t1", "resource_instance": "document:d1"}]}`},
		{facts + "/tenants/t1/users", `{"key": "gina", "email": "gina@example.org", ` +
			`"role_assignments": [{"role": "viewer", "tenant": "t2"}]}`},
		{facts + "/tenants/t1/users", `{"key": "hank", "associated_tenants": ` +
			`[{"tenant": "t1", "roles": ["viewer"]}, {"tenant": "t2", "roles": ["owner"]}]}`},
		{facts + "/resource_instances", `{"resource": "document", "key": "d1", "tenant": "t1"}`},
		{facts + "/resource_instances", `{"resource": "document", "key": "d2", "tenant": "t2"}`},
		{facts + "/resource_instances", `{"resource": "folder", "key": "d1x", "tenant": "t1"}`},
		{facts + "/relationship_tuples", `{"subject": "folder:d1x", "relation": "parent", ` +
			`"object": "document:d1"}`},
		{facts + "/relationship_tuples", `{"subject": "team:a", "relation": "member", ` +
			`"object": "folder:d1x"}`},
		{facts + "/user_invites", `{"key": "i1", "email": "i1@example.com", "role_id": "r1", ` +
			`"tenant_id": "t1"}`},
		{facts + "/user_invites", `{"key": "i2", "email": "i2@example.com", "role_id": "r2", ` +
			`"tenant_id": "t2"}`},
	} {
		h.expect(200, "POST", seed.path, seed.body, projectKey)
	}
	project := h.fake.ProjectID()
	keyName := func(body string) string {
		return "with the ID " + text(h.expect(200, "POST", "/v2/api-key", body, projectKey), "id")
	}
	envKey := keyName(`{"object_type": "env", "project_id": "` + project + `", ` +
		`"environment_id": "` + text(env, "id") + `"}`)
	projectLevelKey := keyName(`{"object_type": "project", "project_id": "` + project + `"}`)
	keyName(`{"object_type": "org"}`)
	// No route creates an audit log, so the test puts the audit logs in the fake's store.
	auditLogs := collectionOf([]string{"", "v2", "pdps", project, text(env, "id"), "audit_logs"})
	h.fake.mu.Lock()
	h.fake.store[auditLogs] = []object{
		{
			"key": "l1", "pdp_config_id": "p1", "user_key": "alice",
			"user_email": "alice@example.com", "decision": true, "resource_type": "document",
			"tenant": "t1", "action": "read", "timestamp": "2026-01-01T00:00:00Z",
		},
		{
			"key": "l2", "pdp_config_id": "p2", "user_key": "bob", "user_name": "Bob",
			"decision": false, "resource_type": "folder", "tenant": "t2", "action": "wipe",
			"timestamp": "2026-01-01T00:01:40Z",
		},
		{
			"key": "l3", "user_name": "Carol", "decision": true, "resource_type": "document",
			"tenant": "t2", "action": "read", "timestamp": "2026-01-01T00:03:20Z",
		},
	}
	h.fake.mu.Unlock()
	const (
		parent = "subject=folder:d1x relation=parent object=document:d1"
		member = "subject=team:a relation=member object=folder:d1x"
		owner  = "user=dave role=owner tenant=t1 resource_instance=document:d1"
	)
	allLogs := []string{"l1", "l2", "l3"}
	for _, tc := range []struct {
		path string
		want []string
	}{
		{facts + "/users?search=example.com", []string{"alice", "carol"}},
		{facts + "/users?search=Example", nil},
		{facts + "/users?role=editor", []string{"alice"}},
		{facts + "/users?role=viewer", []string{"dave", "erin"}},
		{facts + "/users?role=owner", nil},
		{facts + "/users?role=viewer&search=example.com", nil},
		{facts + "/tenants/t1/users?search=example.com", []string{"frank"}},
		{facts + "/tenants/t1/users?role=viewer", []string{"frank", "hank"}},
		{facts + "/tenants/t1/users?role=editor", []string{"gina"}},
		{facts + "/tenants/t1/users?role=owner", nil},
		{facts + "/tenants/t1/users?role=viewer&search=example.com", []string{"frank"}},
		{schema + "/resources?search=document", []string{"document"}},
		{schema + "/resources?search=Doc", []string{"folder"}},
		{schema + "/condition_sets?type=resourceset", []string{"docs"}},
		{schema + "/condition_sets?search=Adu", []string{"adults"}},
		{schema + "/condition_sets?type=userset&search=docs", nil},
		{facts + "/resource_instances?tenant=t1", []string{"document:d1", "folder:d1x"}},
		{facts + "/resource_instances?resource=document", []string{"document:d1", "document:d2"}},
		{facts + "/resource_instances?search=d1", []string{"document:d1", "folder:d1x"}},
		{facts + "/resource_instances?search=d1&search=x", []string{"folder:d1x"}},
		{
			facts + "/resource_instances/detailed?tenant=t2&resource=document",
			[]string{"document:d2"},
		},
		{facts + "/resource_instances/detailed?search=d2&tenant=t1", nil},
		{facts + "/relationship_tuples?subject_type=folder", []string{parent}},
		{facts + "/relationship_tuples/detailed?object_type=folder", []string{member}},
		{facts + "/relationship_tuples?subject_type=team&object_type=document", nil},
		{facts + "/role_assignments?resource_instance=document%3Ad1", []string{owner}},
		{
			facts + "/role_assignments?tenant=t2&tenant=t9&role=editor",
			[]string{"user=nobody role=editor tenant=t2"},
		},
		{facts + "/user_invites?role=r1", []string{"i1"}},
		{facts + "/user_invites?tenant=t2", []string{"i2"}},
		{facts + "/user_invites?role=r1&tenant=t2", nil},
		{facts + "/user_invites?search=i2@example.com", []string{"i2"}},
		{facts + "/user_invites?search=i1", []string{"i1"}},
		{facts + "/user_invites?search=example.com", nil},
		{"/v2/api-key?object_type=env", []string{envKey}},
		{"/v2/api-key?proj_id=" + project, []string{envKey, projectLevelKey}},
		{"/v2/api-key?object_type=project&proj_id=" + project, []string{projectLevelKey}},
		{logs + "?pdp_id=p1", []string{"l1"}},
		{logs + "?users=alice&users=Carol", []string{"l1", "l3"}},
		{logs + "?users=alice@example.com", []string{"l1"}},
		{logs + "?decision=false", []string{"l2"}},
		{logs + "?decision=true&resources=document&tenant=t2", []string{"l3"}},
		{logs + "?resources=folder&resources=document&action=wipe", []string{"l2"}},
		{logs + "?timestamp_from=1767225700&timestamp_to=1767225800", []string{"l2", "l3"}},
		{logs + "?timestamp_from=1767225601&timestamp_to=1767225799", []string{"l2"}},
		{logs + "?timestamp_to=1767225601", allLogs},
		{logs + "?timestamp_from=0&timestamp_to=1767225601", allLogs},
		{logs + "?timestamp_from=1767225601&timestamp_to=0", allLogs},
		{logs + "?timestamp_from=soon&timestamp_to=1767225800", nil},
		{schema + "/roles?attr_kind=child", []string{"viewer"}},
		{schema + "/resources/document/actions?attr_risk=high", []string{"wipe"}},
		{facts + "/tenants?attr_tier=gold", []string{"t2"}},
	} {
		h.expectNames(tc.path, tc.want...)
	}
}

// TestRefusedQueryParameters checks that a request with a query parameter that the fake refuses
// for its operation gets 400 with a message that says why, for the first such parameter by name,
// and fails the test with the operation and the message: a parameter that the fake does not
// know, such as a filter of the spec that the SDK does not send, an attribute filter of a list
// whose objects the SDK filters by none, and a parameter of another operation; a filter that the
// SDK sends and that the fake does not model; and a filter that takes one value, an attribute
// filter too, repeated. page and per_page, on a list and on any other request, the parameters of
// quietParams, on their operations, and the repeated values of a filter that takes more than one
// are not refused; and a request with an API key that the fake does not know gets 401 first.
func TestRefusedQueryParameters(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	const (
		facts  = "/v2/facts/proj/dev"
		logs   = "/v2/pdps/proj/dev/audit_logs"
		groups = "/v2/schema/proj/dev/groups/direct"
	)
	h.expect(200, "POST", facts+"/tenants", `{"key": "t1", "name": "T1"}`, projectKey)
	unknown := func(name string) string {
		return "the fake does not know the query parameter " + name
	}
	unmodelled := func(name string) string {
		return "the query parameter " + name + " is not modelled by the fake"
	}
	repeated := func(name string) string {
		return "the query parameter " + name + " takes one value, and the request repeats it"
	}
	for _, tc := range []struct{ path, op, message string }{
		{facts + "/users?search_operator=contains", "list_users", unknown("search_operator")},
		{facts + "/users?role=r&x=1&y=2", "list_users", unknown("x")},
		{
			facts + "/tenants/t1/users?include_resource_instance_roles=true", "list_tenant_users",
			unknown("include_resource_instance_roles"),
		},
		{facts + "/role_assignments?resource=document", opListRoleAssignments, unknown("resource")},
		{"/v2/projects/proj/envs?attr_tier=gold", "list_environments", unknown("attr_tier")},
		{facts + "/users/alice?wait=1", "get_user", unknown("wait")},
		{groups + "?resource=group", opListGroups, unmodelled("resource")},
		{groups + "?search=g&tenant=t1", opListGroups, unmodelled("search")},
		{logs + "?sort_by=timestamp", "list_audit_logs", unmodelled("sort_by")},
		{logs + "?query=check&tenant=t1", "list_audit_logs", unmodelled("query")},
		{
			facts + "/resource_instances?tenant=t1&tenant=t2", "list_resource_instances",
			repeated("tenant"),
		},
		{facts + "/users?search=a&search=b", "list_users", repeated("search")},
		{facts + "/tenants?attr_tier=a&attr_tier=b", "list_tenants", repeated("attr_tier")},
	} {
		got := h.expect(400, "GET", tc.path, "", projectKey)
		if got["message"] != tc.message {
			t.Errorf("GET %s answered %v, want the message %q", tc.path, got, tc.message)
		}
		want := "(" + tc.op + "): " + tc.message + ","
		if errs := h.tb.take(); len(errs) != 1 || !strings.Contains(errs[0], want) {
			t.Errorf("GET %s reported %q, want an error with %q", tc.path, errs, want)
		}
	}
	h.expect(200, "GET", "/v2/projects/proj/envs/dev?page=1&per_page=2", "", projectKey)
	h.expect(200, "GET", facts+"/resource_instances?search=a&search=b", "", projectKey)
	h.expect(200, "GET", logs+"?users=a&users=b&resources=r&resources=s", "", projectKey)
	task := h.expect(202, "POST", "/v2/projects/proj/envs/dev/copy/async?wait=1",
		`{"target_env":{"new":{"key":"qa","name":"QA"}}}`, projectKey)
	h.expect(200, "GET", "/v2/projects/proj/envs/dev/copy/async/"+text(task, "task_id")+
		"/result?wait=1", "", projectKey)
	h.expect(200, "POST", facts+"/role_assignments/bulk?missing_user_policy=fail", `[]`,
		projectKey)
	h.expect(401, "GET", facts+"/users?x=1", "", "permit_key_FAKEunknown")
}
