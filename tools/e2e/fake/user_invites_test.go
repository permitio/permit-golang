package fake

import (
	"testing"
)

// TestUserInvites checks the user invites of an environment: a create answers the invite with an
// ID, and the list answers the invites as the paginated object of the page of the query; a get
// reads an invite by its ID or its key, and an update changes it; an approval of a pending invite
// with its email address creates the user of the body and approves the invite, whose second
// approval, and an approval of another email address, get 400 with a detail, and an approval of
// an invite that is not there 404; a delete answers 204, and a second delete 404. An approval
// without a body and an approval of a key that a user has already are requests that the fake
// does not implement.
func TestUserInvites(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	const invites = "/v2/facts/proj/dev/user_invites"
	invite := func(key string) string {
		return `{"key":"` + key + `","status":"pending","email":"` + key + `","first_name":"A",` +
			`"last_name":"B","role_id":"r","tenant_id":"t","resource_instance_id":"i"}`
	}
	alice := h.expect(200, "POST", invites, invite("alice@example.com"), projectKey)
	id := text(alice, "id")
	if id == "" || alice["key"] != "alice@example.com" || alice["environment_id"] == "" {
		t.Errorf("the create answered %v, want the invite with an ID and its environment", alice)
	}
	bob := h.expect(200, "POST", invites, invite("bob@example.com"), projectKey)
	h.expectInvitePage(invites+"?per_page=1&page=2", []object{bob}, 2)
	if got := h.expect(200, "GET", invites+"/alice@example.com", "", projectKey); text(got,
		"id") != id {
		t.Errorf("the get of alice by her key answered %v, want the invite %s", got, id)
	}
	changed := h.expect(200, "PATCH", invites+"/"+id,
		`{"key":"alice@example.com","status":"pending","email":"alice@example.com",`+
			`"first_name":"A","last_name":"C","role_id":"r","tenant_id":"t",`+
			`"resource_instance_id":"i"}`, projectKey)
	if changed["last_name"] != "C" || text(changed, "id") != id {
		t.Errorf("the update answered %v, want the invite %s with the last name C", changed, id)
	}
	refused := h.expect(400, "POST", invites+"/"+id+"/approve",
		`{"email":"bob@example.com","key":"alice@example.com","attributes":{}}`, projectKey)
	if refused["detail"] != "the email requested does not match the email of the invite" {
		t.Errorf("the approval of another email address answered %v", refused)
	}
	approval := `{"email":"alice@example.com","key":"alice@example.com","attributes":{"a":"b"}}`
	user := h.expect(200, "POST", invites+"/"+id+"/approve", approval, projectKey)
	if user["key"] != "alice@example.com" || text(user, "id") == "" || user["email"] !=
		"alice@example.com" {
		t.Errorf("the approval answered %v, want the user alice@example.com with an ID", user)
	}
	h.expect(200, "GET", "/v2/facts/proj/dev/users/alice@example.com", "", projectKey)
	if got := h.expect(200, "GET", invites+"/"+id, "", projectKey); got["status"] != "approved" {
		t.Errorf("the get of the approved invite answered %v, want the status approved", got)
	}
	h.expect(400, "POST", invites+"/"+id+"/approve", approval, projectKey)
	h.expect(404, "POST", invites+"/gone/approve", approval, projectKey)
	for _, r := range []struct{ method, path, body string }{
		{"POST", invites + "/" + text(bob, "id") + "/approve", ""},
		{
			"POST", invites + "/" + text(bob, "id") + "/approve",
			`{"email":"bob@example.com","key":"alice@example.com","attributes":{}}`,
		},
	} {
		h.do(r.method, r.path, r.body, projectKey)
		h.expectUnsupported(r.method + " " + r.path + " " + r.body)
	}
	h.expect(204, "DELETE", invites+"/"+id, "", projectKey)
	h.expect(404, "GET", invites+"/"+id, "", projectKey)
	h.expect(404, "DELETE", invites+"/"+id, "", projectKey)
	h.expectInvitePage(invites, []object{bob}, 1)
}

// expectInvitePage fails the test unless a GET of path answers the paginated object whose data
// holds the invites of data, by their IDs, whose total_count is total, and whose page_count is
// the number of the invites of data, as the Permit API sends it.
func (h *harness) expectInvitePage(path string, data []object, total float64) {
	h.t.Helper()
	got := h.expect(200, "GET", path, "", projectKey)
	items, _ := got["data"].([]any)
	if len(items) != len(data) || got["total_count"] != total ||
		got["page_count"] != float64(len(data)) {
		h.t.Errorf("GET %s answered %v; want %d items, %v in total, %d on the page", path, got,
			len(data), total, len(data))
		return
	}
	for i, want := range data {
		item, _ := items[i].(object)
		if text(item, "id") != text(want, "id") {
			h.t.Errorf("GET %s answered the item %v at %d, want %v", path, item, i, want)
		}
	}
}
