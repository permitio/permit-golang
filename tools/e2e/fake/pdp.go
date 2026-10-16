package fake

import (
	"bytes"
	"cmp"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// PDPKind is the kind of PDP that a fake PDP stands in for.
type PDPKind int

const (
	// ContainerPDP is the PDP container, which serves each route of pdpRoutes.
	ContainerPDP PDPKind = iota

	// CloudPDP is the cloud PDP, which answers each route that only the PDP container serves with
	// 404 and an empty body.
	CloudPDP
)

// pdpRoute is a route of the PDP that a fake PDP serves.
type pdpRoute struct {
	method, path string

	// op is the route's operationId in the spec of the PDP container, which the fake passes to
	// the check of a body.
	op string

	// containerOnly says that only the PDP container serves the route.
	containerOnly bool

	// answer answers a request to the route, with the body of the request, unless it is nil,
	// and the query of its URL, made with an API key of the environment of s. The fake holds
	// f.mu.
	answer func(f *Fake, s scope, body []byte, query url.Values) (int, any)
}

// pdpRoutes are the routes of the PDP that a fake PDP serves: those that the scenarios send.
var pdpRoutes = []pdpRoute{
	{http.MethodPost, "/allowed", "is_allowed_allowed_post", false, (*Fake).allowed},
	{
		http.MethodPost, "/allowed/bulk", "is_allowed_bulk_allowed_bulk_post", false,
		(*Fake).allowedBulk,
	},
	{
		http.MethodPost, "/allowed/all-tenants", "is_allowed_all_tenants_allowed_all_tenants_post",
		true, (*Fake).allowedTenants,
	},
	{
		http.MethodPost, "/allowed_url", "is_allowed_url_allowed_url_post", true,
		(*Fake).allowedURL,
	},
	{
		http.MethodPost, "/user-tenants", "Get_User_Tenants_user_tenants_post", true,
		(*Fake).userTenants,
	},
	{
		http.MethodPost, "/user-permissions", "Get_User_Permissions_user_permissions_post", false,
		(*Fake).userPermissions,
	},
	{
		http.MethodPost, "/authorized_users", "authorized_users_authorized_users_post", false,
		(*Fake).authorizedUsers,
	},
	{
		http.MethodGet, "/local/role_assignments",
		"list_role_assignments_local_role_assignments_get", true, (*Fake).localRoleAssignments,
	},
	{
		http.MethodPost, "/facts/role_assignments",
		"create_role_assignment_facts_role_assignments_post", true, (*Fake).factsAssignRole,
	},
	{
		http.MethodDelete, "/facts/role_assignments",
		"delete_role_assignment_facts_role_assignments_delete", true, (*Fake).factsUnassignRole,
	},
}

// forwarded are the operations of the routes under /facts that the PDP container sends on to the
// Permit API, whose bodies the container's spec does not describe, each with the operation of the
// route of the API that the container sends it on to, whose body the check of a body reads
// instead.
var forwarded = map[string]string{
	"create_role_assignment_facts_role_assignments_post":   "assign_role",
	"delete_role_assignment_facts_role_assignments_delete": "unassign_role",
}

// StartPDP starts a fake PDP of the kind kind, which f's test closes when it ends, and returns
// its base URL, such as http://127.0.0.1:52345.
//
// The fake PDP answers the decisions and the lists that the scenarios of tools/e2e send to a
// PDP, those of pdpRoutes, from f's store: the role assignments of the environment of the API
// key of the request, by user, role, tenant and resource instance, those whose user, tenant, role
// and instance f made before them, and the permissions of its roles, each as
// <resource>:<action>, and of its resource roles, each an action of the resource. A role in a
// tenant grants its permissions on the resources of the tenant; a resource role on an instance
// grants its actions on the instance. A check of a URL is the check of the action of the first
// mapping rule of the environment's proxy configs that matches its method and URL, on the rule's
// resource. It knows no other policy: no attributes, no conditions, no
// relations and no role derivations, and it lists tenants without attributes. It is an
// approximation that proves that a scenario's calls and its seed, in the order of the seed,
// reach a PDP, never how the PDP decides; the live legs prove that. The fake PDP also serves the
// assignment and the unassignment of a role under /facts, which it makes in f's store as the
// PDP container sends them on to the Permit API, as factsRoleAssignment says.
//
// Each request must carry the API key of an environment that f issued, or it gets 401. Each body
// goes through f's check, which the scenarios give the spec of the PDP container, or for a route
// of forwarded that of the API's route, and a body that the check rejects gets 422 and fails the
// test, as a request to a route of no pdpRoutes and a request that the fake does not implement
// do. As the cloud PDP, the fake answers each route that only the PDP container serves with 404
// and an empty body, as the cloud PDP does, whatever the request's API key.
func (f *Fake) StartPDP(kind PDPKind) string {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.servePDP(kind, w, r)
	}))
	f.tb.Cleanup(server.Close)
	return server.URL
}

// pdpError returns an error body of the shape that the PDP sends.
func pdpError(detail string) object {
	return object{"detail": detail}
}

// servePDP serves one request to a fake PDP of the kind kind.
func (f *Fake) servePDP(kind PDPKind, w http.ResponseWriter, r *http.Request) {
	i := slices.IndexFunc(pdpRoutes, func(rt pdpRoute) bool {
		return rt.method == r.Method && rt.path == r.URL.Path
	})
	if i < 0 {
		f.tb.Errorf("fake: no route that the fake PDP serves matches %s %s", r.Method, r.URL.Path)
		reply(w, http.StatusNotFound, pdpError("Not Found"))
		return
	}
	rt := pdpRoutes[i]
	if kind == CloudPDP && rt.containerOnly {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		reply(w, http.StatusBadRequest, pdpError(err.Error()))
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	key, ok := f.keys[token]
	if !ok || key.environment == "" {
		reply(w, http.StatusUnauthorized, pdpError("the API key is not one of an environment "+
			"that the fake knows"))
		return
	}
	if len(body) > 0 && !f.checkPDPBody(rt, body) {
		reply(w, http.StatusUnprocessableEntity, pdpError("the body does not fit its schema"))
		return
	}
	status, answer := rt.answer(f, key, body, r.URL.Query())
	reply(w, status, answer)
}

// checkPDPBody reports whether f's check accepts body, the body of a request to rt, as the
// schema of the operation of rt, or for a route of forwarded, of the API's operation, describes
// it; when it does not, or cannot run, it fails the test.
func (f *Fake) checkPDPBody(rt pdpRoute, body []byte) bool {
	problems, err := f.check(cmp.Or(forwarded[rt.op], rt.op), body)
	switch {
	case err != nil:
		f.tb.Errorf("fake: %s %s: the body could not be checked: %v", rt.method, rt.path, err)
		return false
	case len(problems) > 0:
		f.tb.Errorf("fake: %s %s (%s): the body does not fit its schema: %s", rt.method,
			rt.path, rt.op, strings.Join(problems, "; "))
		return false
	}
	return true
}

// decode decodes body, a JSON value that the check of its route accepted, into v, or fails the
// test and returns false.
func (f *Fake) decode(body []byte, v any) bool {
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(v); err != nil {
		f.tb.Errorf("fake: decoding the body of a request to the fake PDP: %v", err)
		return false
	}
	return true
}

// badBody is the reply to a request whose body the fake PDP cannot decode.
var badBody = pdpError("the fake PDP cannot decode the body")

// check is a check of the PDP: whether a user may do an action on a resource.
type check struct {
	User struct {
		Key string `json:"key"`
	} `json:"user"`
	Action   string      `json:"action"`
	Resource pdpResource `json:"resource"`
}

// pdpResource is a resource of a request to the PDP: a resource type, and for an instance its
// key, in a tenant.
type pdpResource struct {
	Type   string `json:"type"`
	Key    string `json:"key"`
	Tenant string `json:"tenant"`
}

// instance returns r as the PDP names an instance, <type>:<key>, or <type>:* for a type.
func (r pdpResource) instance() string {
	if r.Key == "" {
		return r.Type + ":*"
	}
	return r.Type + ":" + r.Key
}

// decision is the reply to a check.
func decision(allow bool) object {
	return object{"allow": allow}
}

// allowed answers POST /allowed.
func (f *Fake) allowed(s scope, body []byte, _ url.Values) (int, any) {
	var c check
	if !f.decode(body, &c) {
		return http.StatusUnprocessableEntity, badBody
	}
	return http.StatusOK, decision(f.decides(s, c.User.Key, c.Action, c.Resource))
}

// allowedBulk answers POST /allowed/bulk.
func (f *Fake) allowedBulk(s scope, body []byte, _ url.Values) (int, any) {
	var checks []check
	if !f.decode(body, &checks) {
		return http.StatusUnprocessableEntity, badBody
	}
	decisions := make([]object, len(checks))
	for i, c := range checks {
		decisions[i] = decision(f.decides(s, c.User.Key, c.Action, c.Resource))
	}
	return http.StatusOK, object{"allow": decisions}
}

// urlCheck is a check of the PDP of the URL of a request: whether a user may send it.
type urlCheck struct {
	User struct {
		Key string `json:"key"`
	} `json:"user"`
	HTTPMethod string `json:"http_method"`
	URL        string `json:"url"`
	Tenant     string `json:"tenant"`
}

// allowedURL answers POST /allowed_url: the decision of the first mapping rule of the proxy
// configs of the environment, in the order in which they were made, whose HTTP method is the
// request's, in any case, and whose URL matches the request's, as urlMatches says, as a check of
// the rule's action on the rule's resource in the tenant of the request. A URL that no rule maps
// is denied.
func (f *Fake) allowedURL(s scope, body []byte, _ url.Values) (int, any) {
	var c urlCheck
	if !f.decode(body, &c) {
		return http.StatusUnprocessableEntity, badBody
	}
	for _, config := range f.objectsOf(s, "facts", "proxy_configs") {
		rules, _ := config["mapping_rules"].([]any)
		for _, r := range rules {
			rule, _ := r.(object)
			if !strings.EqualFold(text(rule, "http_method"), c.HTTPMethod) ||
				!urlMatches(rule, c.URL) {
				continue
			}
			resource := pdpResource{Type: text(rule, "resource"), Tenant: c.Tenant}
			return http.StatusOK, decision(f.decides(s, c.User.Key, text(rule, "action"),
				resource))
		}
	}
	return http.StatusOK, decision(false)
}

// urlMatches reports whether rule, a mapping rule, matches target, the URL of a request: as a
// regular expression of the whole URL for a rule of the URL type regex, and else as the same
// text. A regular expression that does not compile matches nothing.
func urlMatches(rule object, target string) bool {
	if text(rule, "url_type") != "regex" {
		return text(rule, "url") == target
	}
	re, err := regexp.Compile("^(?:" + text(rule, "url") + ")$")
	return err == nil && re.MatchString(target)
}

// tenantDetails is a tenant as a reply of the PDP holds it.
func tenantDetails(key string) object {
	return object{"key": key, "attributes": object{}}
}

// allowedTenants answers POST /allowed/all-tenants: the tenants of the environment, in the
// order in which they were created, in which the check is allowed.
func (f *Fake) allowedTenants(s scope, body []byte, _ url.Values) (int, any) {
	var c check
	if !f.decode(body, &c) {
		return http.StatusUnprocessableEntity, badBody
	}
	allowed := []object{}
	for _, tenant := range f.objectsOf(s, "facts", "tenants") {
		key, _ := tenant["key"].(string)
		resource := c.Resource
		resource.Tenant = key
		if f.decides(s, c.User.Key, c.Action, resource) {
			allowed = append(allowed, object{"allow": true, "tenant": tenantDetails(key)})
		}
	}
	return http.StatusOK, object{"allowed_tenants": allowed}
}

// userTenants answers POST /user-tenants: the tenants in which the user has a role, in the order
// of the first role assignment in each.
func (f *Fake) userTenants(s scope, body []byte, _ url.Values) (int, any) {
	var request struct {
		User struct {
			Key string `json:"key"`
		} `json:"user"`
	}
	if !f.decode(body, &request) {
		return http.StatusUnprocessableEntity, badBody
	}
	tenants := []object{}
	var seen []string
	for _, a := range f.assignments(s) {
		if a.user == request.User.Key && !slices.Contains(seen, a.tenant) {
			seen = append(seen, a.tenant)
			tenants = append(tenants, tenantDetails(a.tenant))
		}
	}
	return http.StatusOK, tenants
}

// permissionsEntry is the entry of a reply of POST /user-permissions of a tenant or of a
// resource instance.
type permissionsEntry struct {
	tenant, resourceType, resourceKey string
	permissions, roles                []string
}

// userPermissions answers POST /user-permissions: an entry for each tenant in which the user has
// a role, under __tenant:<tenant>, and for each instance on which the user has a resource role,
// under <type>:<key>, each with the permissions and the roles of the user there. The filters
// tenants and resource_types keep the entries of those tenants and types, __tenant being the type
// of the entries of the tenants. The fake does not implement the filter resources.
func (f *Fake) userPermissions(s scope, body []byte, _ url.Values) (int, any) {
	var request struct {
		User struct {
			Key string `json:"key"`
		} `json:"user"`
		Tenants       []string `json:"tenants"`
		Resources     []string `json:"resources"`
		ResourceTypes []string `json:"resource_types"`
	}
	if !f.decode(body, &request) {
		return http.StatusUnprocessableEntity, badBody
	}
	if request.Resources != nil {
		f.tb.Errorf("fake: the fake PDP does not implement the filter resources of " +
			"POST /user-permissions")
		return http.StatusNotImplemented, pdpError("not implemented")
	}
	entries := map[string]*permissionsEntry{}
	var order []string
	for _, a := range f.assignments(s) {
		resourceType, resourceKey, onInstance := strings.Cut(a.instance, ":")
		if !onInstance {
			resourceType = "__tenant"
		}
		switch {
		case a.user != request.User.Key:
			continue
		case request.Tenants != nil && !slices.Contains(request.Tenants, a.tenant):
			continue
		case request.ResourceTypes != nil && !slices.Contains(request.ResourceTypes, resourceType):
			continue
		}
		key := "__tenant:" + a.tenant
		if onInstance {
			key = a.instance
		}
		e, ok := entries[key]
		if !ok {
			e = &permissionsEntry{tenant: a.tenant, resourceType: resourceType}
			if onInstance {
				e.resourceKey = resourceKey
			}
			entries[key] = e
			order = append(order, key)
		}
		e.roles = appendNew(e.roles, a.role)
		for _, p := range a.permissions {
			e.permissions = appendNew(e.permissions, p)
		}
	}
	reply := object{}
	for _, key := range order {
		e := entries[key]
		entry := object{
			"tenant": tenantDetails(e.tenant), "permissions": e.permissions, "roles": e.roles,
		}
		if e.resourceKey != "" {
			entry["resource"] = object{
				"type": e.resourceType, "key": e.resourceKey, "attributes": object{},
			}
		}
		reply[key] = entry
	}
	return http.StatusOK, reply
}

// appendNew appends v to list unless list holds it.
func appendNew(list []string, v string) []string {
	if slices.Contains(list, v) {
		return list
	}
	return append(list, v)
}

// authorizedUsers answers POST /authorized_users: the users with a role assignment in the
// resource's tenant that allows the action on the resource, each with those role assignments.
func (f *Fake) authorizedUsers(s scope, body []byte, _ url.Values) (int, any) {
	var request struct {
		Action   string      `json:"action"`
		Resource pdpResource `json:"resource"`
	}
	if !f.decode(body, &request) {
		return http.StatusUnprocessableEntity, badBody
	}
	r := request.Resource
	users := object{}
	for _, a := range f.assignments(s) {
		if a.tenant != r.Tenant || !a.grants(request.Action, r) {
			continue
		}
		at := "__tenant:" + a.tenant
		if a.instance != "" {
			at = a.instance
		}
		list, _ := users[a.user].([]object)
		users[a.user] = append(list, object{
			"user": a.user, "tenant": a.tenant, "resource": at, "role": a.role,
		})
	}
	return http.StatusOK, object{"resource": r.instance(), "tenant": r.Tenant, "users": users}
}

// defaultPerPage is the number of items on a page of a list that the PDP container gives without
// per_page, as GET /local/role_assignments does, and that the Permit API gives too.
const defaultPerPage = 30

// localRoleAssignments answers GET /local/role_assignments: the role assignments of the user that
// the filter user names, or every one without it, in the order in which they were made, on the
// page of page and per_page, the first page of 30 by default. The fake implements no other
// filter, and fails the test for one.
func (f *Fake) localRoleAssignments(s scope, _ []byte, query url.Values) (int, any) {
	for name := range query {
		if name != "user" && name != "page" && name != "per_page" {
			f.tb.Errorf("fake: the fake PDP does not implement the filter %s of "+
				"GET /local/role_assignments", name)
			return http.StatusNotImplemented, pdpError("not implemented")
		}
	}
	page, perPage := positive(query, "page", 1), positive(query, "per_page", defaultPerPage)
	matches := []object{}
	for _, a := range f.assignments(s) {
		if user := query.Get("user"); user == "" || a.user == user {
			matches = append(matches, a.local())
		}
	}
	start := min((page-1)*perPage, len(matches))
	return http.StatusOK, matches[start:min(start+perPage, len(matches))]
}

// The routes of the Permit API that the PDP container sends POST and DELETE
// /facts/role_assignments on to.
var (
	assignRole = route{
		Route: Route{
			Method: http.MethodPost, Template: "/v2/facts/{proj_id}/{env_id}/role_assignments",
			Op: "assign_role",
		},
		segments: []string{"", "v2", "facts", "{proj_id}", "{env_id}", "role_assignments"},
	}
	unassignRole = route{
		Route: Route{
			Method: http.MethodDelete, Template: "/v2/facts/{proj_id}/{env_id}/role_assignments",
			Op: "unassign_role",
		},
		segments: assignRole.segments,
	}
)

// factsAssignRole answers POST /facts/role_assignments, as factsRoleAssignment says.
func (f *Fake) factsAssignRole(s scope, body []byte, _ url.Values) (int, any) {
	return f.factsRoleAssignment(assignRole, s, body)
}

// factsUnassignRole answers DELETE /facts/role_assignments, as factsRoleAssignment says.
func (f *Fake) factsUnassignRole(s scope, body []byte, _ url.Values) (int, any) {
	return f.factsRoleAssignment(unassignRole, s, body)
}

// factsRoleAssignment answers a request to a route of the role assignments under /facts, whose
// body the check accepted, as the Permit API answers the request to rt, its route, in the
// environment of s, to which the PDP container sends it on: an assignment makes the role
// assignment, and an unassignment removes each that holds the fields of body, or gets 404. The
// fake PDP decides from f's store, so a check after the call sees the change at once, as it would
// after a wait for sync of the container; the fake reads no header of the wait. The caller holds
// f.mu.
func (f *Fake) factsRoleAssignment(rt route, s scope, body []byte) (int, any) {
	var fields object
	if !f.decode(body, &fields) {
		return http.StatusUnprocessableEntity, badBody
	}
	segments := []string{"", "v2", "facts", s.project, s.environment, "role_assignments"}
	return f.collection(rt, segments, fields, nil)
}

// positive returns the number from 1 that the parameter name of query holds, or byDefault when it
// holds none.
func positive(query url.Values, name string, byDefault int) int {
	n, err := strconv.Atoi(query.Get(name))
	if err != nil || n < 1 {
		return byDefault
	}
	return n
}
