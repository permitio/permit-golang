package fake

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// pdpHarness is a harness whose fake has an environment, dev, with a seed, and a fake PDP of each
// kind.
type pdpHarness struct {
	*harness

	// envKey is dev's API key.
	envKey string

	// container and cloud are the base URLs of the fake PDPs.
	container, cloud string
}

// newPDPHarness starts a pdpHarness. dev holds the resource document with the actions read and
// write, the role editor, which grants both, the resource role viewer of document, which grants
// read, the tenants t1 and t2, the users u1 to u3, the instance document:d1 in t1, and three role
// assignments: editor to u1 in t1, viewer to u2 on document:d1 in t1, and editor to u3 in t2.
func newPDPHarness(t *testing.T) *pdpHarness {
	t.Helper()
	h := &pdpHarness{harness: newHarness(t)}
	_, h.envKey = h.createEnv("dev")
	for _, seed := range []struct{ path, body string }{
		{"/v2/schema/proj/dev/resources", `{"key": "document", "actions": {"read": {}, ` +
			`"write": {}}}`},
		{"/v2/schema/proj/dev/roles", `{"key": "editor", "permissions": ["document:read", ` +
			`"document:write"]}`},
		{"/v2/schema/proj/dev/resources/document/roles", `{"key": "viewer", "permissions": ` +
			`["read"]}`},
		{"/v2/facts/proj/dev/tenants", `{"key": "t1"}`},
		{"/v2/facts/proj/dev/tenants", `{"key": "t2"}`},
		{"/v2/facts/proj/dev/users", `{"key": "u1"}`},
		{"/v2/facts/proj/dev/users", `{"key": "u2"}`},
		{"/v2/facts/proj/dev/users", `{"key": "u3"}`},
		{"/v2/facts/proj/dev/resource_instances", `{"resource": "document", "key": "d1", ` +
			`"tenant": "t1"}`},
		{"/v2/facts/proj/dev/role_assignments", `{"user": "u1", "role": "editor", "tenant": "t1"}`},
		{"/v2/facts/proj/dev/role_assignments", `{"user": "u2", "role": "viewer", ` +
			`"tenant": "t1", "resource_instance": "document:d1"}`},
		{"/v2/facts/proj/dev/role_assignments", `{"user": "u3", "role": "editor", "tenant": "t2"}`},
	} {
		h.expect(http.StatusOK, http.MethodPost, seed.path, seed.body, projectKey)
	}
	h.container = h.fake.StartPDP(ContainerPDP)
	h.cloud = h.fake.StartPDP(CloudPDP)
	return h
}

// send sends method to path of the PDP at base with body, unless it is empty, and key, and
// returns the status and the body of the reply.
func (h *pdpHarness) send(base, method, path, body, key string) (int, []byte) {
	h.t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(h.t.Context(), method, base+path, reader)
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
	return resp.StatusCode, data
}

// ask sends method to path of the container PDP with body and dev's key, fails the test unless
// the reply is 200, and returns its body decoded.
func (h *pdpHarness) ask(method, path, body string) any {
	h.t.Helper()
	status, data := h.send(h.container, method, path, body, h.envKey)
	var reply any
	if err := json.Unmarshal(data, &reply); status != http.StatusOK || err != nil {
		h.t.Fatalf("%s %s %s answered %d %s, want 200 with JSON", method, path, body, status, data)
	}
	return reply
}

// checkBody returns the body of a check of user, action and the resource document, with key,
// in tenant.
func checkBody(user, action, key, tenant string) string {
	resource := `{"type": "document", "tenant": "` + tenant + `"`
	if key != "" {
		resource += `, "key": "` + key + `"`
	}
	return `{"user": {"key": "` + user + `"}, "action": "` + action + `", "resource": ` +
		resource + `}}`
}

// decoded returns the JSON value of text, for a comparison with a reply.
func decoded(t *testing.T, text string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// TestPDPDecisions checks the decisions of the fake PDP: a role in a tenant allows its
// permissions on the resources of the tenant only, the type and every instance, and a resource
// role on an instance allows its actions on the instance only; a role assignment allows nothing
// when its user, tenant, role or instance does not exist, or was made after it; each check of a
// bulk is decided in order.
func TestPDPDecisions(t *testing.T) {
	h := newPDPHarness(t)
	const assignments = "/v2/facts/proj/dev/role_assignments"
	for _, seed := range []struct{ path, body string }{
		{"/v2/facts/proj/dev/users", `{"key": "u5"}`},
		{"/v2/facts/proj/dev/users", `{"key": "u6"}`},
		{"/v2/facts/proj/dev/users", `{"key": "u8"}`},
		{"/v2/facts/proj/dev/users", `{"key": "u9"}`},
		{assignments, `{"user": "u5", "role": "viewer", "tenant": "t1", ` +
			`"resource_instance": "document:d3"}`},
		{assignments, `{"user": "u6", "role": "editor", "tenant": "t9"}`},
		{assignments, `{"user": "u7", "role": "editor", "tenant": "t1"}`},
		{assignments, `{"user": "u9", "role": "owner", "tenant": "t1"}`},
		{assignments, `{"user": "u8", "role": "viewer", "tenant": "t1", ` +
			`"resource_instance": "document:d4"}`},
		{"/v2/facts/proj/dev/resource_instances", `{"resource": "document", "key": "d4", ` +
			`"tenant": "t1"}`},
		{assignments, `{"user": "u9", "role": "author", "tenant": "t2"}`},
		{"/v2/schema/proj/dev/roles", `{"key": "author", "permissions": ["document:read"]}`},
	} {
		h.expect(http.StatusOK, http.MethodPost, seed.path, seed.body, projectKey)
	}
	for _, tc := range []struct {
		user, action, key, tenant string
		want                      bool
	}{
		{"u1", "read", "", "t1", true},
		{"u1", "write", "d9", "t1", true},
		{"u1", "read", "", "t2", false},
		{"u1", "delete", "", "t1", false},
		{"u2", "read", "d1", "t1", true},
		{"u2", "read", "d2", "t1", false},
		{"u2", "read", "", "t1", false},
		{"u2", "write", "d1", "t1", false},
		{"u2", "read", "d1", "t2", false},
		{"u3", "write", "", "t2", true},
		{"u4", "read", "", "t1", false},
		{"u5", "read", "d3", "t1", false},
		{"u6", "read", "", "t9", false},
		{"u7", "read", "", "t1", false},
		{"u9", "read", "", "t1", false},
		{"u8", "read", "d4", "t1", false},
		{"u9", "read", "", "t2", false},
	} {
		body := checkBody(tc.user, tc.action, tc.key, tc.tenant)
		if got := h.ask(http.MethodPost, "/allowed", body); !reflect.DeepEqual(got,
			map[string]any{"allow": tc.want}) {
			t.Errorf("POST /allowed %s answered %v, want allow %t", body, got, tc.want)
		}
	}
	bulk := "[" + checkBody("u1", "read", "", "t2") + ", " +
		checkBody("u2", "read", "d1", "t1") + ", " +
		checkBody("u1", "read", "", "t1") + "]"
	want := decoded(t, `{"allow": [{"allow": false}, {"allow": true}, {"allow": true}]}`)
	if got := h.ask(http.MethodPost, "/allowed/bulk", bulk); !reflect.DeepEqual(got, want) {
		t.Errorf("POST /allowed/bulk answered %v, want %v", got, want)
	}
	if got := h.ask(http.MethodPost, "/allowed/bulk", "[]"); !reflect.DeepEqual(got,
		decoded(t, `{"allow": []}`)) {
		t.Errorf("POST /allowed/bulk of no checks answered %v", got)
	}
}

// TestPDPTenants checks the tenants of the fake PDP: those of the environment in which a check is
// allowed, in the order in which they were created, and those in which a user has a role, each
// once.
func TestPDPTenants(t *testing.T) {
	h := newPDPHarness(t)
	for _, body := range []string{
		`{"user": "u1", "role": "editor", "tenant": "t2"}`,
		`{"user": "u1", "role": "viewer", "tenant": "t1", "resource_instance": "document:d1"}`,
	} {
		h.expect(http.StatusOK, http.MethodPost, "/v2/facts/proj/dev/role_assignments", body,
			projectKey)
	}
	t1 := `{"key": "t1", "attributes": {}}`
	t2 := `{"key": "t2", "attributes": {}}`
	for _, tc := range []struct{ body, want string }{
		{
			checkBody("u1", "read", "", "default"),
			`{"allowed_tenants": [{"allow": true, "tenant": ` + t1 + `}, {"allow": true, ` +
				`"tenant": ` + t2 + `}]}`,
		},
		{
			checkBody("u2", "read", "d1", ""),
			`{"allowed_tenants": [{"allow": true, "tenant": ` + t1 + `}]}`,
		},
		{checkBody("u2", "write", "d1", ""), `{"allowed_tenants": []}`},
	} {
		if got := h.ask(http.MethodPost, "/allowed/all-tenants", tc.body); !reflect.DeepEqual(got,
			decoded(t, tc.want)) {
			t.Errorf("POST /allowed/all-tenants %s answered %v, want %s", tc.body, got, tc.want)
		}
	}
	for user, want := range map[string]string{
		"u1": "[" + t1 + ", " + t2 + "]", "u2": "[" + t1 + "]", "u4": "[]",
	} {
		body := `{"user": {"key": "` + user + `"}}`
		if got := h.ask(http.MethodPost, "/user-tenants", body); !reflect.DeepEqual(got,
			decoded(t, want)) {
			t.Errorf("POST /user-tenants %s answered %v, want %s", body, got, want)
		}
	}
}

// TestPDPUserPermissions checks the permissions of a user that the fake PDP lists: an entry for
// each tenant in which the user has a role and for each instance on which the user has a
// resource role, with each of its roles and permissions once, which the filters of tenants and of
// resource types narrow.
func TestPDPUserPermissions(t *testing.T) {
	h := newPDPHarness(t)
	const assignments = "/v2/facts/proj/dev/role_assignments"
	for _, seed := range []struct{ path, body string }{
		{"/v2/schema/proj/dev/roles", `{"key": "reader", "permissions": ["document:read"]}`},
		{assignments, `{"user": "u2", "role": "editor", "tenant": "t2"}`},
		{assignments, `{"user": "u2", "role": "reader", "tenant": "t2"}`},
		{assignments, `{"user": "u2", "role": "editor", "tenant": "t2"}`},
	} {
		h.expect(http.StatusOK, http.MethodPost, seed.path, seed.body, projectKey)
	}
	tenantEntry := `{"tenant": {"key": "t2", "attributes": {}}, "permissions": ` +
		`["document:read", "document:write"], "roles": ["editor", "reader"]}`
	instanceEntry := `{"tenant": {"key": "t1", "attributes": {}}, "resource": {"type": ` +
		`"document", "key": "d1", "attributes": {}}, "permissions": ["document:read"], ` +
		`"roles": ["viewer"]}`
	for _, tc := range []struct{ filters, want string }{
		{"", `{"document:d1": ` + instanceEntry + `, "__tenant:t2": ` + tenantEntry + `}`},
		{`, "tenants": ["t1"]`, `{"document:d1": ` + instanceEntry + `}`},
		{`, "resource_types": ["__tenant"]`, `{"__tenant:t2": ` + tenantEntry + `}`},
		{`, "tenants": ["t1"], "resource_types": ["__tenant"]`, `{}`},
		{`, "context": {"n": 1}`, `{"document:d1": ` + instanceEntry + `, "__tenant:t2": ` +
			tenantEntry + `}`},
	} {
		body := `{"user": {"key": "u2"}` + tc.filters + `}`
		if got := h.ask(http.MethodPost, "/user-permissions", body); !reflect.DeepEqual(got,
			decoded(t, tc.want)) {
			t.Errorf("POST /user-permissions %s answered %v, want %s", body, got, tc.want)
		}
	}
	if got := h.ask(http.MethodPost, "/user-permissions", `{"user": {"key": "u4"}}`); !reflect.
		DeepEqual(got, map[string]any{}) {
		t.Errorf("the permissions of a user without roles are %v, want none", got)
	}
	status, _ := h.send(h.container, http.MethodPost, "/user-permissions",
		`{"user": {"key": "u2"}, "resources": ["document:d1"]}`, h.envKey)
	if errs := h.tb.take(); status != http.StatusNotImplemented || len(errs) != 1 ||
		!strings.Contains(errs[0], "does not implement the filter resources") {
		t.Errorf("the filter resources answered %d and reported %q", status, errs)
	}
}

// TestPDPAuthorizedUsers checks the users that the fake PDP allows to do an action on a resource,
// each with the role assignments that allow it.
func TestPDPAuthorizedUsers(t *testing.T) {
	h := newPDPHarness(t)
	u1 := `"u1": [{"user": "u1", "tenant": "t1", "resource": "__tenant:t1", "role": "editor"}]`
	for _, tc := range []struct{ body, want string }{
		{
			`{"action": "read", "resource": {"type": "document", "key": "d1", "tenant": "t1"}}`,
			`{"resource": "document:d1", "tenant": "t1", "users": {` + u1 + `, "u2": [{"user": ` +
				`"u2", "tenant": "t1", "resource": "document:d1", "role": "viewer"}]}}`,
		},
		{
			`{"action": "read", "resource": {"type": "document", "tenant": "t1"}}`,
			`{"resource": "document:*", "tenant": "t1", "users": {` + u1 + `}}`,
		},
		{
			`{"action": "delete", "resource": {"type": "document", "tenant": "t1"}}`,
			`{"resource": "document:*", "tenant": "t1", "users": {}}`,
		},
	} {
		if got := h.ask(http.MethodPost, "/authorized_users", tc.body); !reflect.DeepEqual(got,
			decoded(t, tc.want)) {
			t.Errorf("POST /authorized_users %s answered %v, want %s", tc.body, got, tc.want)
		}
	}
}

// TestPDPLocalRoleAssignments checks the role assignments that the fake PDP lists: the filter
// user keeps those of the user, the page and the number on it pick a part of the list, the first
// page of 30 when they hold no number from 1, and another filter fails the test.
func TestPDPLocalRoleAssignments(t *testing.T) {
	h := newPDPHarness(t)
	u1 := `{"user": "u1", "role": "editor", "tenant": "t1"}`
	u2 := `{"user": "u2", "role": "viewer", "tenant": "t1", "resource_instance": "document:d1"}`
	u3 := `{"user": "u3", "role": "editor", "tenant": "t2"}`
	for query, want := range map[string]string{
		"":                    "[" + u1 + ", " + u2 + ", " + u3 + "]",
		"?user=u2":            "[" + u2 + "]",
		"?user=u4":            "[]",
		"?per_page=1":         "[" + u1 + "]",
		"?page=2&per_page=1":  "[" + u2 + "]",
		"?page=2&per_page=2":  "[" + u3 + "]",
		"?page=3&per_page=2":  "[]",
		"?page=0&per_page=x":  "[" + u1 + ", " + u2 + ", " + u3 + "]",
		"?user=u3&per_page=0": "[" + u3 + "]",
	} {
		if got := h.ask(http.MethodGet, "/local/role_assignments"+query, ""); !reflect.DeepEqual(
			got, decoded(t, want)) {
			t.Errorf("GET /local/role_assignments%s answered %v, want %s", query, got, want)
		}
	}
	status, _ := h.send(h.container, http.MethodGet, "/local/role_assignments?role=editor", "",
		h.envKey)
	if errs := h.tb.take(); status != http.StatusNotImplemented || len(errs) != 1 ||
		!strings.Contains(errs[0], "does not implement the filter role of") {
		t.Errorf("the filter role answered %d and reported %q", status, errs)
	}
	if slices.Contains(h.checked, "list_role_assignments_local_role_assignments_get") {
		t.Error("a request without a body went to the check, which has no schema for it")
	}
}

// TestPDPFactsRoleAssignments checks the role assignments under /facts of the fake PDP container:
// an assignment makes the role assignment in the environment of its API key, which the next check
// allows and the Permit API lists; an unassignment removes it, so that the next check denies, and
// a second unassignment gets 404; and the check of each body is that of the API's operation, as
// the container's spec does not describe the bodies.
func TestPDPFactsRoleAssignments(t *testing.T) {
	h := newPDPHarness(t)
	const body = `{"user": "u3", "role": "editor", "tenant": "t1"}`
	check := checkBody("u3", "read", "", "t1")
	allows := func(want bool) {
		t.Helper()
		if got := h.ask(http.MethodPost, "/allowed", check); !reflect.DeepEqual(got,
			map[string]any{"allow": want}) {
			t.Errorf("the check of u3 in t1 answered %v, want an allow: %t", got, want)
		}
	}
	allows(false)
	before := len(h.checked)
	assigned, ok := h.ask(http.MethodPost, "/facts/role_assignments", body).(map[string]any)
	if !ok || assigned["user"] != "u3" || assigned["tenant"] != "t1" || assigned["id"] == nil {
		t.Errorf("the assignment answered %v, want the role assignment", assigned)
	}
	allows(true)
	if n := len(h.list("/v2/facts/proj/dev/role_assignments?user=u3&tenant=t1",
		projectKey)); n != 1 {
		t.Errorf("the API lists %d role assignments of u3 in t1, want the new one", n)
	}
	for _, want := range []int{http.StatusNoContent, http.StatusNotFound} {
		if status, data := h.send(h.container, http.MethodDelete, "/facts/role_assignments", body,
			h.envKey); status != want {
			t.Errorf("the unassignment answered %d %s, want %d", status, data, want)
		}
	}
	allows(false)
	var facts []string
	for _, op := range h.checked[before:] {
		if op != "is_allowed_allowed_post" {
			facts = append(facts, op)
		}
	}
	if want := []string{"assign_role", "unassign_role", "unassign_role"}; !slices.Equal(facts,
		want) {
		t.Errorf("the bodies of the routes under /facts went to the checks of %q, want %q", facts,
			want)
	}
}

// TestPDPURLChecks checks the URL checks of the fake PDP: the first mapping rule of the proxy
// configs of the environment whose HTTP method, in any case, and URL match the request decides it,
// as a check of its action on its resource in the request's tenant; a rule of the URL type regex
// matches the whole URL with its regular expression, and one that does not compile matches
// nothing; a URL that no rule maps, a method that no rule names and a rule without an action are
// denied.
func TestPDPURLChecks(t *testing.T) {
	h := newPDPHarness(t)
	for _, config := range []string{
		`{"key": "docs", "name": "Docs", "secret": "t", "mapping_rules": [` +
			`{"url": "https://app.example.com/documents", "http_method": "get", ` +
			`"resource": "document", "action": "read"}, ` +
			`{"url": "https://app.example.com/documents/[0-9]+", "url_type": "regex", ` +
			`"http_method": "put", "resource": "document", "action": "write"}, ` +
			`{"url": "https://app.example.com/drafts", "http_method": "get", ` +
			`"resource": "document"}, ` +
			`{"url": "(", "url_type": "regex", "http_method": "delete", ` +
			`"resource": "document", "action": "write"}]}`,
		`{"key": "late", "name": "Late", "secret": "t", "mapping_rules": [` +
			`{"url": "https://app.example.com/documents", "http_method": "get", ` +
			`"resource": "document", "action": "approve"}]}`,
	} {
		h.expect(http.StatusOK, http.MethodPost, "/v2/facts/proj/dev/proxy_configs", config,
			projectKey)
	}
	urlCheck := func(user, method, target, tenant string) string {
		return `{"user": {"key": "` + user + `"}, "http_method": "` + method + `", "url": "` +
			target + `", "tenant": "` + tenant + `"}`
	}
	for _, tc := range []struct {
		user, method, target, tenant string
		allow                        bool
	}{
		{"u1", "GET", "https://app.example.com/documents", "t1", true},
		{"u1", "get", "https://app.example.com/documents", "t1", true},
		{"u1", "GET", "https://app.example.com/documents", "t2", false},
		{"u2", "GET", "https://app.example.com/documents", "t1", false},
		{"u3", "GET", "https://app.example.com/documents", "t2", true},
		{"u1", "PUT", "https://app.example.com/documents/12", "t1", true},
		{"u1", "PUT", "https://app.example.com/documents/12/x", "t1", false},
		{"u1", "PUT", "https://x.example.com/?https://app.example.com/documents/12", "t1", false},
		{"u1", "PUT", "https://app.example.com/documents", "t1", false},
		{"u1", "POST", "https://app.example.com/documents", "t1", false},
		{"u1", "GET", "https://app.example.com/drafts", "t1", false},
		{"u1", "DELETE", "(", "t1", false},
		{"u1", "GET", "https://app.example.com/other", "t1", false},
	} {
		body := urlCheck(tc.user, tc.method, tc.target, tc.tenant)
		got := h.ask(http.MethodPost, "/allowed_url", body)
		if want := map[string]any{"allow": tc.allow}; !reflect.DeepEqual(got, want) {
			t.Errorf("POST /allowed_url %s answered %v, want %v", body, got, want)
		}
	}
	if status, _ := h.send(h.container, http.MethodPost, "/allowed_url", `{"user": "u1"}`,
		h.envKey); status != http.StatusUnprocessableEntity {
		t.Errorf("POST /allowed_url of a body that does not decode answered %d, want 422", status)
	}
	if errs := h.tb.take(); len(errs) != 1 || !strings.Contains(errs[0], "decoding the body") {
		t.Errorf("the fake reported %q, want the error of the body that does not decode", errs)
	}
	if last := h.checked[len(h.checked)-1]; last != "is_allowed_url_allowed_url_post" {
		t.Errorf("the last body went to the check of %s", last)
	}
}

// TestPDPAccess checks the requests that the fake PDP refuses: without the API key of an
// environment that its fake issued, to a route that it does not serve, and with a body that the
// check rejects or cannot check. An environment's key reads that environment only.
func TestPDPAccess(t *testing.T) {
	h := newPDPHarness(t)
	body := checkBody("u1", "read", "", "t1")
	for _, key := range []string{"", "permit_key_FAKEunknown", projectKey} {
		if status, data := h.send(h.container, http.MethodPost, "/allowed", body,
			key); status != http.StatusUnauthorized || !strings.Contains(string(data), "detail") {
			t.Errorf("POST /allowed with the key %q answered %d %s, want 401", key, status, data)
		}
	}
	_, prodKey := h.createEnv("prod")
	if status, data := h.send(h.container, http.MethodPost, "/allowed", body, prodKey); status !=
		http.StatusOK || string(data) != `{"allow":false}` {
		t.Errorf("POST /allowed with another environment's key answered %d %s, want a denial",
			status, data)
	}
	for _, tc := range []struct {
		method, path, body string
		status             int
		want               string
	}{
		{"GET", "/allowed", "", 404, "no route that the fake PDP serves matches GET /allowed"},
		{"POST", "/kong", "{}", 404, "no route that the fake PDP serves matches POST /kong"},
		{
			"POST", "/allowed", `{"user": {"key": "bad"}}`, 422,
			"fake: POST /allowed (is_allowed_allowed_post): the body does not fit its schema: " +
				"#/key is bad",
		},
		{
			"POST", "/allowed", `{"user": {"key": "unknown"}}`, 422,
			"fake: POST /allowed: the body could not be checked: no schema for " +
				"is_allowed_allowed_post",
		},
		{
			"POST", "/allowed", `{"user": "u1"}`, 422,
			"fake: decoding the body of a request to the fake PDP: ",
		},
	} {
		status, _ := h.send(h.container, tc.method, tc.path, tc.body, h.envKey)
		if errs := h.tb.take(); status != tc.status || len(errs) != 1 ||
			!strings.Contains(errs[0], tc.want) {
			t.Errorf("%s %s %s answered %d and reported %q, want %d and %q", tc.method, tc.path,
				tc.body, status, errs, tc.status, tc.want)
		}
	}
	if last := h.checked[len(h.checked)-1]; last != "is_allowed_allowed_post" {
		t.Errorf("the last body went to the check of %s", last)
	}
}

// TestCloudPDP checks that the fake cloud PDP answers each route that only the PDP container
// serves with 404 and an empty body, whatever the API key, and the others as the PDP container
// does.
func TestCloudPDP(t *testing.T) {
	h := newPDPHarness(t)
	for _, rt := range pdpRoutes {
		if !rt.containerOnly {
			continue
		}
		for _, key := range []string{h.envKey, ""} {
			status, data := h.send(h.cloud, rt.method, rt.path, "{}", key)
			if status != http.StatusNotFound || len(data) != 0 {
				t.Errorf("the cloud PDP answered %s %s with %d %q, want 404 and no body",
					rt.method, rt.path, status, data)
			}
		}
	}
	body := checkBody("u1", "read", "", "t1")
	if status, data := h.send(h.cloud, http.MethodPost, "/allowed", body, h.envKey); status !=
		http.StatusOK || string(data) != `{"allow":true}` {
		t.Errorf("the cloud PDP answered POST /allowed with %d %s, want an allow", status, data)
	}
	if status, data := h.send(h.container, http.MethodPost, "/user-tenants",
		`{"user": {"key": "u1"}}`, h.envKey); status != http.StatusOK || len(data) == 0 {
		t.Errorf("the PDP container answered POST /user-tenants with %d %q", status, data)
	}
}

// specOperations returns the operationId of each operation of the spec snapshot name of
// tools/specs, such as pdp-container, by its method, upper case, and its path, such as
// "POST /allowed". It reads the snapshot itself rather than through tools/drift, which needs the
// SDK's module, which the copy of this module that the mutation tests run in does not hold.
func specOperations(t *testing.T, name string) map[string]string {
	t.Helper()
	data, err := os.ReadFile("../../specs/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string `json:"operationId"` //nolint:tagliatelle // OpenAPI's name.
		} `json:"paths"`
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parsing the spec %s: %v", name, err)
	}
	ops := map[string]string{}
	for path, methods := range spec.Paths {
		for method, op := range methods {
			ops[strings.ToUpper(method)+" "+path] = op.OperationID
		}
	}
	if len(ops) == 0 {
		t.Fatalf("the spec %s has no operations", name)
	}
	return ops
}

// TestPDPRoutes checks the routes of the fake PDPs against the spec snapshots: each is a route of
// the PDP container's spec with its operationId, and only the PDP container serves it exactly
// when the cloud PDP's spec does not list it.
func TestPDPRoutes(t *testing.T) {
	container := specOperations(t, "pdp-container")
	cloud := specOperations(t, "pdp-cloud")
	for _, rt := range pdpRoutes {
		route := rt.method + " " + rt.path
		if id, ok := container[route]; !ok || id != rt.op {
			t.Errorf("%s is the operation %q of the PDP container's spec, want %s", route, id,
				rt.op)
		}
		if _, inCloud := cloud[route]; rt.containerOnly == inCloud {
			t.Errorf("%s: containerOnly is %t, and the cloud PDP's spec lists it: %t", route,
				rt.containerOnly, inCloud)
		}
	}
}
