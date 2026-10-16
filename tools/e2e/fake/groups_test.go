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
// delete answer 204, and a group or a user that is not there 404. The filters resource and
// search, the assignment of a user that the group holds already, and a create, an assignment or
// a removal without a body are requests that the fake does not implement.
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
		`"group","group_tenant":"t1","id":"`+id+`"},`+second+`]`, 2, 1)
	h.expectGroupPage(groups+"/direct?tenant=t2&per_page=1", `[`+second+`]`, 1, 1)
	h.expectGroupPage(groups+"/direct?per_page=1&page=2", `[`+second+`]`, 2, 2)
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
	h.expectGroupPage(users+"?per_page=1&page=2", `[{"id":"`+bobID+`","key":"bob"}]`, 2, 2)
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
	h.expectGroupPage(users, `[]`, 0, 0)
	for _, r := range []struct{ method, path, body string }{
		{"GET", groups + "/direct?resource=group", ""},
		{"GET", groups + "/direct?search=g", ""},
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

// expectGroupPage sends GET to path and fails the test unless the reply is the paginated object
// whose data is the JSON text data, of total items on pages pages.
func (h *harness) expectGroupPage(path, data string, total, pages float64) {
	h.t.Helper()
	got := h.expect(200, "GET", path, "", projectKey)
	var want []any
	if err := json.Unmarshal([]byte(data), &want); err != nil {
		h.t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got["data"]) || got["total_count"] != total ||
		got["page_count"] != pages {
		h.t.Errorf("GET %s answered %v; want the data %v, %v in total on %v pages", path, got,
			want, total, pages)
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
