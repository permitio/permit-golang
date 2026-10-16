package fake

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestGroups checks the groups of an environment: a create answers the group without an ID, with
// the resource group by default, and refuses a second group of the same instance key; the list
// answers the groups as the paginated object, those of a tenant with the filter tenant; a get
// reads a group by its instance key or its ID; an assignment adds a user of the environment, by
// its key or its ID, and answers the IDs of the users of the group; the list of the users of a
// group answers the ID and the key of each user that the environment still has; a removal and a
// delete answer 204, and a group or a user that is not there 404. The assignment of a user that
// the group holds already, and a create, an assignment or a removal without a body are requests
// that the fake does not implement.
func TestGroups(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	const groups = "/v2/schema/proj/dev/groups"
	alice := h.expect(200, "POST", "/v2/facts/proj/dev/users", `{"key": "alice"}`, projectKey)
	bob := h.expect(200, "POST", "/v2/facts/proj/dev/users", `{"key": "bob"}`, projectKey)
	created := h.expect(200, "POST", groups, `{"group_instance_key": "g1", "group_tenant": "t1"}`,
		projectKey)
	want := map[string]any{
		"group_instance_key": "g1", "group_tenant": "t1", "group_resource_type_key": "group",
		"users": []any{}, "assigned_roles": []any{},
	}
	if !reflect.DeepEqual(want, created) {
		t.Errorf("the create answered %v, want %v", created, want)
	}
	conflict := h.expect(409, "POST", groups,
		`{"group_instance_key": "g1", "group_tenant": "t2"}`, projectKey)
	if conflict["message"] != "a group has the instance key g1" {
		t.Errorf("a second group g1 answered %v", conflict)
	}
	h.expect(200, "POST", groups,
		`{"group_instance_key": "g2", "group_tenant": "t2", "group_resource_type_key": "team"}`,
		projectKey)
	g1 := h.expect(200, "GET", groups+"/direct/g1", "", projectKey)
	id, _ := g1["id"].(string)
	if id == "" || g1["group_instance_key"] != "g1" || g1["users"] != nil {
		t.Errorf("the get of g1 answered %v, want the group with an ID and without its users", g1)
	}
	if byID := h.expect(200, "GET", groups+"/direct/"+id, "", projectKey); !reflect.DeepEqual(
		g1, byID) {
		t.Errorf("the get of g1 by its ID answered %v, want %v", byID, g1)
	}
	h.expect(404, "GET", groups+"/direct/g3", "", projectKey)
	g2 := h.expect(200, "GET", groups+"/direct/g2", "", projectKey)
	second := `{"group_instance_key":"g2","group_resource_type_key":"team","group_tenant":"t2",` +
		`"id":"` + text(g2, "id") + `"}`
	h.expectGroupPage(groups+"/direct", `[{"group_instance_key":"g1","group_resource_type_key":`+
		`"group","group_tenant":"t1","id":"`+id+`"},`+second+`]`, 2)
	h.expectGroupPage(groups+"/direct?tenant=t2&per_page=1", `[`+second+`]`, 1)
	h.expectGroupPage(groups+"/direct?per_page=1&page=2", `[`+second+`]`, 2)
	users := groups + "/g1/users"
	assigned := h.expect(200, "PUT", users+"/alice", `{"tenant": "t1"}`, projectKey)
	if got := assigned["users"]; !reflect.DeepEqual(got, []any{alice["id"]}) {
		t.Errorf("the assignment of alice answered the users %v, want the ID of alice", got)
	}
	bobID := text(bob, "id")
	assigned = h.expect(200, "PUT", groups+"/"+id+"/users/"+bobID, `{"tenant": "t1"}`, projectKey)
	if got := assigned["users"]; !reflect.DeepEqual(got, []any{alice["id"], bobID}) {
		t.Errorf("the assignment of bob by IDs answered the users %v, want alice and bob", got)
	}
	h.expectGroupPage(users+"?per_page=1&page=2", `[{"id":"`+bobID+`","key":"bob"}]`, 2)
	for path, message := range map[string]string{
		users + "/carol":         "no user carol",
		groups + "/g3/users/bob": "no group g3",
	} {
		if body := h.expect(404, "PUT", path, `{"tenant": "t1"}`, projectKey); body["message"] !=
			message {
			t.Errorf("PUT %s answered %v, want the message %q", path, body, message)
		}
	}
	h.expect(204, "DELETE", users+"/alice", `{"tenant": "t1"}`, projectKey)
	again := h.expect(404, "DELETE", users+"/alice", `{"tenant": "t1"}`, projectKey)
	if again["message"] != "the group g1 does not hold the user alice" {
		t.Errorf("a second removal of alice answered %v", again)
	}
	h.expect(204, "DELETE", "/v2/facts/proj/dev/users/bob", "", projectKey)
	h.expectGroupPage(users, `[]`, 0)
	for _, r := range []struct{ method, path, body string }{
		{"PUT", users + "/" + bobID, ""},
		{"POST", groups, ""},
		{"DELETE", users + "/alice", ""},
	} {
		h.do(r.method, r.path, r.body, projectKey)
		h.expectUnsupported(r.method + " " + r.path)
	}
	h.expect(200, "PUT", users+"/alice", `{"tenant": "t1"}`, projectKey)
	h.expect(501, "PUT", users+"/alice", `{"tenant": "t1"}`, projectKey)
	h.expectUnsupported("a second assignment of alice")
	h.expect(204, "DELETE", groups+"/g1", "", projectKey)
	h.expect(404, "GET", groups+"/direct/g1", "", projectKey)
	h.expect(404, "DELETE", groups+"/g1", "", projectKey)
	h.expect(404, "GET", users, "", projectKey)
}

// TestGroupRolesAndNesting checks the roles of a group and the groups assigned to it. An assignment
// of a role names the role of a resource, the resource, its instance and the tenant by key or ID,
// the instance also as <resource>:<key>, as the fake assumes, and answers the group with the key of
// the role in assigned_roles; the list of the roles answers each with the ID and the key of its
// resource and of its instance; a role, a resource, an instance or a tenant that the environment
// does not have gets 404, and so do an instance of another resource and the removal of a role that
// the group does not hold. An assignment of a group names it by its ID or as <resource>:<key>, not
// by its key alone, and makes it a child of the group of the path, which the lists of the children
// and of the parents answer; its removal removes it, and a second removal gets 404. A second
// assignment of a role or of a group, the assignment of a group to itself, and an assignment
// without a body are requests that the fake does not implement.
func TestGroupRolesAndNesting(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	const groups = "/v2/schema/proj/dev/groups"
	doc := h.expect(200, "POST", "/v2/schema/proj/dev/resources",
		`{"key": "document", "name": "document", "actions": {}}`, projectKey)
	h.expect(200, "POST", "/v2/schema/proj/dev/resources/document/roles",
		`{"key": "editor", "name": "editor"}`, projectKey)
	h.expect(200, "POST", "/v2/facts/proj/dev/tenants", `{"key": "t1", "name": "t1"}`, projectKey)
	instance := h.expect(200, "POST", "/v2/facts/proj/dev/resource_instances",
		`{"resource": "document", "key": "doc-1", "tenant": "t1"}`, projectKey)
	h.expect(200, "POST", "/v2/schema/proj/dev/resources",
		`{"key": "photo", "name": "photo", "actions": {}}`, projectKey)
	photo := h.expect(200, "POST", "/v2/facts/proj/dev/resource_instances",
		`{"resource": "photo", "key": "photo-1", "tenant": "t1"}`, projectKey)
	for _, key := range []string{"g1", "g2"} {
		h.expect(200, "POST", groups, `{"group_instance_key": "`+key+`", "group_tenant": "t1"}`,
			projectKey)
	}
	role := `{"role": "editor", "resource": "document", "resource_instance": "doc-1", ` +
		`"tenant": "t1"}`
	assigned := h.expect(200, "POST", groups+"/g1/roles", role, projectKey)
	if !reflect.DeepEqual(assigned["assigned_roles"], []any{"editor"}) {
		t.Errorf("the assignment of editor answered %v, want the role in assigned_roles", assigned)
	}
	h.expectGroupPage(groups+"/g1/roles", `[{"key":"editor","resource":{"id":"`+text(doc, "id")+
		`","key":"document"},"resource_instance":{"id":"`+text(instance, "id")+
		`","key":"doc-1"}}]`, 1)
	byIDs := `{"role": "editor", "resource": "` + text(doc, "id") + `", "resource_instance": ` +
		`"document:doc-1", "tenant": "t1"}`
	h.expect(501, "POST", groups+"/g1/roles", byIDs, projectKey)
	h.expectUnsupported("a second assignment of editor")
	for _, tc := range []struct{ field, value, message string }{
		{"role", "viewer", "no role viewer of the resource document"},
		{"resource", "folder", "no resource folder"},
		{"resource_instance", "doc-2", "no resource instance doc-2 of the resource document"},
		{
			"resource_instance", text(photo, "id"),
			"no resource instance " + text(photo, "id") + " of the resource document",
		},
		{"tenant", "t2", "no tenant t2"},
	} {
		fields := map[string]string{
			"role": "editor", "resource": "document", "resource_instance": "doc-1", "tenant": "t1",
		}
		fields[tc.field] = tc.value
		body, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		got := h.expect(404, "POST", groups+"/g1/roles", string(body), projectKey)
		if got["message"] != tc.message {
			t.Errorf("the assignment of %s answered %v, want the message %q", body, got,
				tc.message)
		}
	}
	h.expect(204, "DELETE", groups+"/g1/roles", byIDs, projectKey)
	if again := h.expect(404, "DELETE", groups+"/g1/roles", role, projectKey); again["message"] !=
		"the group g1 does not hold the role editor" {
		t.Errorf("a second removal of editor answered %v", again)
	}
	h.expectGroupPage(groups+"/g1/roles", `[]`, 0)

	g1 := h.expect(200, "GET", groups+"/direct/g1", "", projectKey)
	g2 := h.expect(200, "GET", groups+"/direct/g2", "", projectKey)
	schema := func(g map[string]any, key string) string {
		return `{"group_instance_key":"` + key + `","group_resource_type_key":"group",` +
			`"group_tenant":"t1","id":"` + text(g, "id") + `"}`
	}
	h.expect(200, "PUT", groups+"/g1/assign_group", `{"group_instance_key": "group:g2"}`,
		projectKey)
	h.expectGroupPage(groups+"/g1/children", `[`+schema(g2, "g2")+`]`, 1)
	h.expectGroupPage(groups+"/g2/parents", `[`+schema(g1, "g1")+`]`, 1)
	h.expectGroupPage(groups+"/g2/children", `[]`, 0)
	if body := h.expect(404, "PUT", groups+"/g1/assign_group", `{"group_instance_key": "g2"}`,
		projectKey); body["message"] != "no group g2" {
		t.Errorf("an assignment of g2 by its key alone answered %v", body)
	}
	for name, body := range map[string]string{
		"a second assignment of g2":  `{"group_instance_key": "` + text(g2, "id") + `"}`,
		"an assignment of g1 to g1":  `{"group_instance_key": "group:g1"}`,
		"an assignment without body": "",
	} {
		h.expect(501, "PUT", groups+"/g1/assign_group", body, projectKey)
		h.expectUnsupported(name)
	}
	h.expect(204, "DELETE", groups+"/g1/assign_group",
		`{"group_instance_key": "`+text(g2, "id")+`"}`, projectKey)
	if again := h.expect(404, "DELETE", groups+"/g1/assign_group",
		`{"group_instance_key": "group:g2"}`, projectKey); again["message"] !=
		"the group g1 does not hold the group group:g2" {
		t.Errorf("a second removal of g2 answered %v", again)
	}
	h.expectGroupPage(groups+"/g1/children", `[]`, 0)
	h.expectGroupPage(groups+"/g2/parents", `[]`, 0)
}

// expectGroupPage sends GET to path and fails the test unless the reply is the paginated object
// whose data is the JSON text data, of total items, with the number of the items of data in
// page_count, as the Permit API sends it.
func (h *harness) expectGroupPage(path, data string, total float64) {
	h.t.Helper()
	got := h.expect(200, "GET", path, "", projectKey)
	var want []any
	if err := json.Unmarshal([]byte(data), &want); err != nil {
		h.t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got["data"]) || got["total_count"] != total ||
		got["page_count"] != float64(len(want)) {
		h.t.Errorf("GET %s answered %v; want the data %v, %v in total, %d on the page", path,
			got, want, total, len(want))
	}
}

// expectUnsupported fails the test unless the fake reported one error, that it does not
// implement the request of what.
func (h *harness) expectUnsupported(what string) {
	h.t.Helper()
	if errs := h.tb.take(); len(errs) != 1 ||
		!strings.Contains(errs[0], "does not implement this request") {
		h.t.Errorf("%s reported %q, want that it is not implemented", what, errs)
	}
}
