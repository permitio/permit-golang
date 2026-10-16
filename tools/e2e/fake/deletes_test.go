package fake

import (
	"testing"
)

// The role assignments of alice of the role editor in the tenant t1, and on the instance
// document:d1 in t1, as a body sends them and as nameOf names them.
const (
	tenantRole   = `{"user": "alice", "role": "editor", "tenant": "t1"}`
	instanceRole = `{"user": "alice", "role": "editor", "tenant": "t1", ` +
		`"resource_instance": "document:d1"}`
	tenantRoleName   = "user=alice role=editor tenant=t1"
	instanceRoleName = tenantRoleName + " resource_instance=document:d1"
)

// TestExactDeletes checks the DELETE of a collection with a body, which removes the object whose
// identity the body names, as names says, and answers 404 when there is none. The unassignment of
// a role in a tenant removes the role assignment without a resource instance, also when the body
// gives the instance as null, and never the role on an instance with the same user, role and
// tenant; that of a role on an instance removes that one alone, and one that differs in the
// instance or the tenant none. The delete of a relationship tuple names it by its subject,
// relation and object, whatever its tenant, and a body that leaves out the object names none. The
// unassignment of a condition set rule reads is_role and is_resource as false where the body or
// the rule leaves them out.
func TestExactDeletes(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	const facts = "/v2/facts/proj/dev"
	assignments := facts + "/role_assignments"
	h.expect(200, "POST", assignments, tenantRole, projectKey)
	h.expect(200, "POST", assignments, instanceRole, projectKey)
	h.expect(204, "DELETE", assignments, `{"user": "alice", "role": "editor", "tenant": "t1", `+
		`"resource_instance": null}`, projectKey)
	h.expectNames(assignments, instanceRoleName)
	if got := h.expect(404, "DELETE", assignments, tenantRole, projectKey); got["message"] !=
		"no object has the identity that the body names" {
		t.Errorf("the unassignment of a role in a tenant that alice has on an instance alone "+
			"answered %v", got)
	}
	for _, other := range []string{
		`{"user": "alice", "role": "editor", "tenant": "t1", "resource_instance": "document:d2"}`,
		`{"user": "alice", "role": "editor", "tenant": "t2", "resource_instance": "document:d1"}`,
	} {
		h.expect(404, "DELETE", assignments, other, projectKey)
	}
	h.expectNames(assignments, instanceRoleName)
	h.expect(200, "POST", assignments, tenantRole, projectKey)
	h.expect(204, "DELETE", assignments, instanceRole, projectKey)
	h.expectNames(assignments, tenantRoleName)

	tuples := facts + "/relationship_tuples"
	h.expect(200, "POST", tuples, `{"subject": "folder:f1", "relation": "parent", `+
		`"object": "document:d1", "tenant": "t1"}`, projectKey)
	h.expect(404, "DELETE", tuples, `{"subject": "folder:f1", "relation": "parent"}`, projectKey)
	h.expect(204, "DELETE", tuples, `{"subject": "folder:f1", "relation": "parent", `+
		`"object": "document:d1"}`, projectKey)
	h.expectNames(tuples)

	rules := facts + "/set_rules"
	rule := func(userSet, flags string) string {
		return `{"user_set": "` + userSet + `", "permission": "doc:read", "resource_set": ` +
			`"public"` + flags + `}`
	}
	h.expect(200, "POST", rules, rule("adults", `, "is_role": false`), projectKey)
	h.expect(200, "POST", rules, rule("admin", `, "is_role": true`), projectKey)
	h.expect(200, "POST", rules, rule("docs", ""), projectKey)
	h.expect(404, "DELETE", rules, rule("admin", ""), projectKey)
	h.expect(404, "DELETE", rules, rule("docs", `, "is_resource": true`), projectKey)
	h.expect(204, "DELETE", rules, rule("adults", ""), projectKey)
	h.expect(204, "DELETE", rules, rule("admin", `, "is_role": true`), projectKey)
	h.expect(204, "DELETE", rules, rule("docs", `, "is_role": false, "is_resource": false`),
		projectKey)
	h.expectNames(rules)
}

// TestBulkDeletesAreExact checks that the bulk unassignment of roles and the bulk delete of
// relationship tuples remove, for each item, the object that it names, as a DELETE of one does:
// an item of a role in a tenant leaves the role on an instance with the same user, role and
// tenant, and an item of a tuple that leaves out its object removes none.
func TestBulkDeletesAreExact(t *testing.T) {
	h := newHarness(t)
	h.createEnv("dev")
	const facts = "/v2/facts/proj/dev"
	assignments := facts + "/role_assignments"
	h.expect(200, "POST", assignments+"/bulk", `[`+tenantRole+`,`+instanceRole+`]`, projectKey)
	for _, tc := range []struct {
		body string
		left []string
	}{
		{`[` + tenantRole + `]`, []string{instanceRoleName}},
		{`[` + instanceRole + `]`, nil},
	} {
		removed := h.expect(200, "DELETE", assignments+"/bulk", tc.body, projectKey)
		if removed["assignments_removed"] != 1.0 {
			t.Errorf("the bulk unassignment of %s answered %v, want the report of 1", tc.body,
				removed)
		}
		h.expectNames(assignments, tc.left...)
	}

	tuples := facts + "/relationship_tuples"
	const (
		d1 = `{"subject": "folder:f1", "relation": "parent", "object": "document:d1"}`
		d2 = `{"subject": "folder:f1", "relation": "parent", "object": "document:d2"}`
	)
	h.expect(200, "POST", tuples+"/bulk", `{"operations": [`+d1+`, `+d2+`]}`, projectKey)
	h.expect(200, "DELETE", tuples+"/bulk",
		`{"idents": [{"subject": "folder:f1", "relation": "parent"}]}`, projectKey)
	h.expectNames(tuples, "subject=folder:f1 relation=parent object=document:d1",
		"subject=folder:f1 relation=parent object=document:d2")
	h.expect(200, "DELETE", tuples+"/bulk", `{"idents": [`+d1+`]}`, projectKey)
	h.expectNames(tuples, "subject=folder:f1 relation=parent object=document:d2")
}
