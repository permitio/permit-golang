// Package fake serves a small stateful fake of the Permit API for the dry run of the end-to-end
// scenarios of tools/e2e, so that they run offline on every pull request without a scripted
// conversation for each; the strict fakes of internal/testkit/mockpermit stay for the wire tests.
//
// The fake serves the routes that it is given, those of a spec snapshot, and keeps the objects of
// each collection in memory, a collection being the path of a route without its last parameter:
// a POST to a collection creates an object with an ID, a GET, a PATCH, a PUT or a DELETE of an
// item reads, changes or deletes it, a GET of a collection lists the objects whose attributes match
// each attr_<name> of the query, as a JSON array, one page of it when the query holds page or
// per_page, or, for a route that [Config.Paged] names, as the paginated object, always one page,
// the first 30 by default; and a DELETE of a collection with a body removes the objects that hold
// each field of the body. A parameter matches an object by its ID or its identifier: its key, or
// for a resource instance, an object with a resource, <resource>:<key>, as the Permit API names it.
// A parameter that is not the last segment of its route and follows condition_sets, resources,
// roles, tenants or users, such as {resource_id} in .../resources/{resource_id}/roles, must match
// an object, or the request gets 404; another parameter, such as {group_instance_key}, stays as it
// is when it matches none. Deleting an environment revokes its API keys. Each request body goes
// through the check that the fake is given, such as the structural walker of tools/drift, and a
// body that the check rejects gets 422. The fake also serves the scope of an API key and the API
// key of an environment, which it makes up, the statistics and the copies of an environment, as
// stats and copyEnvironment say, the grants and removals of the permissions of a role or of a role
// of a resource, its ancestors and its descendants, and the bulk replace of roles, as
// rolePermissions, roleRelatives and bulkRoles say, the implicit grants of a role of a resource
// and their conditions, as implicitGrant says, the ancestors and the descendants of a condition
// set, as conditionSetRelatives says, the list of the condition set rules, with its filters, and
// their creates, which answer a JSON array, as setRules and assignSetRule say, the PUT of a user,
// which replaces it or creates it, as replaceUser says, the assignment and the unassignment of a
// role of a user in a tenant, which its associated_tenants hold, as userRole says, and the
// detailed list of the resource instances, as detailedInstances says. It filters the lists of the
// role assignments by their user, role, tenant and resource instance, and answers the detailed
// list of them, as roleAssignments and detailedRoleAssignments say, and their bulk assignment and
// unassignment, whose bodies are JSON arrays, as bulkRoleAssignments says. It filters the lists
// of the relationship tuples by their tenant, subject, relation and object, as relationshipTuples
// says, and answers their detailed list with the same tuples in the paginated object, as answer
// says. A delete of a condition set rule that the fake does not hold gets 404, as a DELETE of a
// collection with a body does; the API's documentation says that it skips such a delete, and what
// the API answers is not confirmed yet.
//
// A request to a route that the fake was not given fails the test, whatever its API key. Each
// other request must carry an API key that the fake knows: the project's, or one it issued. An
// unknown key gets 401, and a key whose scope does not hold the route's project or environment
// 403. A body that the check rejects and a check that cannot run fail the test too.
//
// [Fake.StartPDP] starts a fake of the PDP container or of the cloud PDP beside it, which answers
// the decisions and the lists of the PDP that the scenarios send from the same store: an
// approximation that proves that a scenario's calls and its seed reach a PDP, never how the PDP
// decides.
package fake

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
)

// Route is a route that the fake serves.
type Route struct {
	// Method is the HTTP method, such as POST.
	Method string

	// Template is the route's path template, such as /v2/facts/{proj_id}/{env_id}/users.
	Template string

	// Op is the route's operationId, which the fake passes to the check of a body.
	Op string
}

// Check checks body, the body of a request of the operation op, and returns what does not fit
// its schema, or an error when it cannot check it.
type Check func(op string, body []byte) ([]string, error)

// Config configures a Fake.
type Config struct {
	// Routes are the routes it serves.
	Routes []Route

	// Check checks each request body.
	Check Check

	// Project is the key of its one project.
	Project string

	// ProjectAPIKey is the project's API key, a project-level key.
	ProjectAPIKey string

	// Paged reports whether the GET of a collection whose route's operation is op answers the
	// paginated object of the Permit API, as the spec of the route says, such as
	// list_resource_relations. A nil Paged answers every list with a JSON array.
	Paged func(op string) bool
}

// Fake is a fake of the Permit API that an httptest server serves. It is safe for concurrent
// use.
type Fake struct {
	tb     testing.TB
	check  Check
	paged  func(op string) bool
	routes []route
	server *httptest.Server

	// org and project are the IDs of its organization and its project.
	org, project string

	mu     sync.Mutex
	keys   map[string]scope
	store  map[string][]object
	nextID int
}

// route is a Route with its template split into segments.
type route struct {
	Route

	segments []string
}

// scope is the scope of an API key: the IDs of its project and of its environment, which is
// empty for the key of a project.
type scope struct {
	project, environment string
}

// object is an object of a collection, as JSON values.
type object = map[string]any

// projects is the collection of the fake's project.
const projects = "/v2/projects"

// keyed are the collections, by the last segment of their path, whose objects the Permit API
// names by key and whose routes go on after the object's parameter: a parameter that follows one
// of them and is not the last segment of its route must match an object.
var keyed = map[string]bool{
	"condition_sets": true, "resources": true, "roles": true, "tenants": true, "users": true,
}

// collectionOf returns the key of the collection whose path has segments, unescaped: the
// path with each segment escaped, so that a segment that holds a slash stays one segment.
func collectionOf(segments []string) string {
	escaped := make([]string, len(segments))
	for i, s := range segments {
		escaped[i] = url.PathEscape(s)
	}
	return strings.Join(escaped, "/")
}

// New starts a Fake with cfg, which tb closes when it ends. A problem of a request fails tb.
func New(tb testing.TB, cfg Config) *Fake {
	tb.Helper()
	switch {
	case cfg.Check == nil:
		tb.Fatal("fake: New needs a Check of the request bodies")
	case cfg.Project == "" || cfg.ProjectAPIKey == "":
		tb.Fatal("fake: New needs the key of the project and the project's API key")
	}
	f := &Fake{
		tb: tb, check: cfg.Check, paged: cfg.Paged, keys: map[string]scope{},
		store: map[string][]object{},
	}
	for _, r := range cfg.Routes {
		f.routes = append(f.routes, route{Route: r, segments: strings.Split(r.Template, "/")})
	}
	f.org, f.project = f.newID(), f.newID()
	f.store[projects] = []object{{
		"id": f.project, "key": cfg.Project, "name": cfg.Project, "organization_id": f.org,
	}}
	f.keys[cfg.ProjectAPIKey] = scope{project: f.project}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	tb.Cleanup(f.server.Close)
	return f
}

// URL returns the base URL of the fake, such as http://127.0.0.1:52345.
func (f *Fake) URL() string {
	return f.server.URL
}

// ProjectID returns the ID of the fake's project.
func (f *Fake) ProjectID() string {
	return f.project
}

// newID returns a new ID in the shape of a UUID. The caller holds f.mu, or is New.
func (f *Fake) newID() string {
	f.nextID++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", f.nextID)
}

// reply writes status and, unless it is nil, body as JSON.
func reply(w http.ResponseWriter, status int, body any) {
	if body == nil {
		w.WriteHeader(status)
		return
	}
	data, err := json.Marshal(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data) // The client sees a short body; there is no one else to tell.
}

// errorBody returns an error body of the shape that the Permit API sends.
func errorBody(code, message string) object {
	return object{"error_code": code, "title": code, "message": message}
}

// serve serves one request.
func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		reply(w, http.StatusBadRequest, errorBody("BAD_REQUEST", err.Error()))
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	rt, values, ok := f.match(r.Method, r.URL.EscapedPath())
	if !ok {
		f.tb.Errorf("fake: no route that the fake serves matches %s %s", r.Method, r.URL.Path)
		reply(w, http.StatusNotFound, errorBody("NOT_FOUND", "no such route"))
		return
	}
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	key, ok := f.keys[token]
	if !ok {
		reply(w, http.StatusUnauthorized, errorBody("UNAUTHORIZED",
			"the API key is not one that the fake knows"))
		return
	}
	segments, status, message := f.resolve(rt, values, key)
	if status != 0 {
		reply(w, status, errorBody(http.StatusText(status), message))
		return
	}
	var (
		fields object
		items  []object
	)
	ok = true
	switch {
	case arrayBodies[rt.Op]:
		items, ok = f.checkItems(rt, body)
	case len(body) > 0:
		fields, ok = f.checkBody(rt, body)
	}
	if !ok {
		reply(w, http.StatusUnprocessableEntity, errorBody("VALIDATION_ERROR",
			"the body does not fit its schema"))
		return
	}
	var answer any
	if arrayBodies[rt.Op] {
		status, answer = f.bulkRoleAssignments(rt, segments, items)
	} else {
		status, answer = f.answer(rt, segments, fields, key, r.URL.Query())
	}
	reply(w, status, answer)
}

// match returns the route of method whose template matches escaped, an escaped path, and the
// values of the template's segments, unescaped: of the routes that match, the one with the most
// literal segments, then the first. A path with an empty segment matches none.
func (f *Fake) match(method, escaped string) (route, []string, bool) {
	raw := strings.Split(escaped, "/")
	values := make([]string, len(raw))
	for i, s := range raw {
		v, err := url.PathUnescape(s)
		if err != nil || i > 0 && v == "" {
			return route{}, nil, false
		}
		values[i] = v
	}
	var best *route
	literals := 0
	for i, rt := range f.routes {
		if rt.Method != method || len(rt.segments) != len(values) {
			continue
		}
		n, fits := 0, true
		for j, s := range rt.segments {
			switch {
			case isParam(s):
			case s == values[j]:
				n++
			default:
				fits = false
			}
		}
		if fits && (best == nil || n > literals) {
			best, literals = &f.routes[i], n
		}
	}
	if best == nil {
		return route{}, nil, false
	}
	return *best, values, true
}

// isParam reports whether s, a segment of a template, is a parameter, such as {user_id}.
func isParam(s string) bool {
	return strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")
}

// resolve returns the segments of the path of the request to rt whose segments are values, with
// each parameter that names an object by its identifier replaced by the object's ID, or the
// status and the message of an error: 404 when the route's project or environment does not
// exist, or the object of a parameter that is not the route's last segment and follows a keyed
// collection, and 403 when key's scope does not hold the project or the environment.
func (f *Fake) resolve(rt route, values []string, key scope) ([]string, int, string) {
	path := make([]string, len(values))
	project, environment := "", ""
	for i, s := range rt.segments {
		path[i] = values[i]
		if !isParam(s) {
			continue
		}
		parent := collectionOf(path[:i])
		switch s {
		case "{proj_id}":
			parent = projects
		case "{env_id}":
			parent = collectionOf([]string{"", "v2", "projects", project, "envs"})
		}
		o, found := find(f.store[parent], values[i])
		if found {
			path[i], _ = o["id"].(string)
		}
		switch {
		case s == "{proj_id}" && !found:
			return nil, http.StatusNotFound, "no project " + values[i]
		case s == "{env_id}" && !found:
			return nil, http.StatusNotFound, "no environment " + values[i]
		case !found && i < len(rt.segments)-1 && keyed[rt.segments[i-1]]:
			return nil, http.StatusNotFound, "no object " + values[i]
		case s == "{proj_id}":
			project = path[i]
		case s == "{env_id}":
			environment = path[i]
		}
	}
	switch {
	case project != "" && project != key.project:
		return nil, http.StatusForbidden, "the API key's project is not the route's"
	case key.environment != "" && project != "" && environment != key.environment:
		return nil, http.StatusForbidden, "the API key's environment is not the route's"
	}
	return path, 0, ""
}

// find returns the object of objects whose ID or identifier is ident.
func find(objects []object, ident string) (object, bool) {
	for _, o := range objects {
		if n, ok := identOf(o); o["id"] == ident || (ok && n == ident) {
			return o, true
		}
	}
	return nil, false
}

// identOf returns the identifier of o, by which a parameter names it as well as by its ID, and
// which is not the API's name field: its key, or for a resource instance, an object with a
// resource, <resource>:<key>, as the Permit API names an instance; false when o has no key.
func identOf(o object) (string, bool) {
	key, ok := o["key"].(string)
	if !ok {
		return "", false
	}
	if resource, ok := o["resource"].(string); ok {
		return resource + ":" + key, true
	}
	return key, true
}

// checkSchema reports whether the check of rt's operation accepts body; when it does not, or
// cannot run, it fails the test.
func (f *Fake) checkSchema(rt route, body []byte) bool {
	problems, err := f.check(rt.Op, body)
	if err != nil {
		f.tb.Errorf("fake: %s %s: the body could not be checked: %v", rt.Method, rt.Template, err)
		return false
	}
	if len(problems) > 0 {
		f.tb.Errorf("fake: %s %s (%s): the body does not fit its schema: %s", rt.Method,
			rt.Template, rt.Op, strings.Join(problems, "; "))
		return false
	}
	return true
}

// checkItems returns the items of body, a JSON array of objects that the check of rt's operation
// accepts, or false after it failed the test: the check rejects body or cannot run, or body is
// not an array of objects.
func (f *Fake) checkItems(rt route, body []byte) ([]object, bool) {
	if !f.checkSchema(rt, body) {
		return nil, false
	}
	var items []object
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&items); err != nil || items == nil ||
		slices.ContainsFunc(items, func(o object) bool { return o == nil }) {
		f.tb.Errorf("fake: %s %s: the body is not a JSON array of objects", rt.Method,
			rt.Template)
		return nil, false
	}
	return items, true
}

// checkBody returns the fields of body, a JSON object that the check of rt's operation accepts,
// or false after it failed the test: the check rejects body or cannot run, or body is not an
// object.
func (f *Fake) checkBody(rt route, body []byte) (object, bool) {
	if !f.checkSchema(rt, body) {
		return nil, false
	}
	var fields object
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&fields); err != nil || fields == nil {
		f.tb.Errorf("fake: %s %s: the body is not a JSON object", rt.Method, rt.Template)
		return nil, false
	}
	return fields, true
}
