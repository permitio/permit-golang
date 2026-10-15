package fake

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
)

// The operations that the fake answers with more than its store.
const (
	opScope          = "get_api_key_scope"
	opEnvironmentKey = "get_environment_api_key"
	opStats          = "stats_environments"
	opCopy           = "copy_environment"
	opCopyAsync      = "copy_environment_async"
	opCopyResult     = "get_copy_environment_async_result"

	opAssignRolePermissions = "assign_permissions_to_role"
	opRemoveRolePermissions = "remove_permissions_from_role"
	opRoleAncestors         = "get_role_ancestors"
	opRoleDescendants       = "get_role_descendants"
	opBulkReplaceRoles      = "bulk_create_or_replace_roles"
)

// answer returns the status and the body of the reply to a request to rt, whose path has
// segments, with IDs in place of keys, whose query is query, and whose body holds fields, nil
// without a body, sent with an API key of the scope key. The caller holds f.mu.
func (f *Fake) answer(rt route, segments []string, fields object, key scope,
	query url.Values,
) (int, any) {
	switch rt.Op {
	case opScope:
		body := object{"organization_id": f.org, "project_id": key.project}
		if key.environment != "" {
			body["environment_id"] = key.environment
		}
		return http.StatusOK, body
	case opEnvironmentKey:
		return http.StatusOK, f.issue(scope{project: segments[3], environment: segments[4]})
	case opStats:
		return f.stats(segments)
	case opCopy, opCopyAsync:
		if rt.Method == http.MethodPost && fields != nil {
			return f.copyEnvironment(rt.Op, segments, fields)
		}
		return f.unsupported(rt)
	case opCopyResult:
		return f.copyResult(segments)
	case opAssignRolePermissions, opRemoveRolePermissions:
		if fields != nil {
			return f.rolePermissions(rt.Op, segments, fields)
		}
		return f.unsupported(rt)
	case opRoleAncestors, opRoleDescendants:
		return f.roleRelatives(rt.Op, segments)
	case opBulkReplaceRoles:
		if fields != nil {
			return f.bulkRoles(segments, fields)
		}
		return f.unsupported(rt)
	}
	if !isParam(rt.segments[len(rt.segments)-1]) {
		return f.collection(rt, segments, fields, query)
	}
	return f.item(rt, segments, fields)
}

// issue returns the API key of the environment of s, which it issues once, as the Permit API's
// reply describes an API key. The caller holds f.mu.
func (f *Fake) issue(s scope) object {
	secret := ""
	for k, ks := range f.keys {
		if ks == s {
			secret = k
		}
	}
	if secret == "" {
		secret = "permit_key_FAKEfakeenv" + strings.ReplaceAll(f.newID(), "-", "")
		f.keys[secret] = s
	}
	return object{
		"id": f.newID(), "organization_id": f.org, "project_id": s.project,
		"environment_id": s.environment, "object_type": "env", "owner_type": "member",
		"secret": secret,
	}
}

// collection answers a request to a route whose path ends with a literal segment: a POST
// creates an object, a GET lists the collection, as listed says, or as envelope says for an
// operation that f.paged names, and a DELETE with a body removes the objects that hold each of
// its fields. The caller holds f.mu.
func (f *Fake) collection(rt route, segments []string, fields object, query url.Values,
) (int, any) {
	path := collectionOf(segments)
	switch {
	case rt.Method == http.MethodGet && f.paged != nil && f.paged(rt.Op):
		return http.StatusOK, envelope(f.store[path], query)
	case rt.Method == http.MethodGet:
		return http.StatusOK, listed(f.store[path], query)
	case rt.Method == http.MethodPost && fields != nil:
		return f.create(rt, segments, fields)
	case rt.Method == http.MethodDelete && fields != nil:
		kept := slices.DeleteFunc(slices.Clone(f.store[path]), func(o object) bool {
			return holds(o, fields)
		})
		if len(kept) == len(f.store[path]) {
			return http.StatusNotFound, errorBody("NOT_FOUND", "no object holds the body's fields")
		}
		f.store[path] = kept
		return http.StatusNoContent, nil
	}
	return f.unsupported(rt)
}

// listed returns the objects of a collection that a GET with query lists, in the order in which
// they were created: those whose attributes match each attribute filter of query, as matches
// says; all of them when query holds neither page nor per_page, and else the page of page and
// per_page, the first page of 30 by default, as the Permit API pages a list.
func listed(objects []object, query url.Values) []object {
	objects = slices.DeleteFunc(append([]object{}, objects...), func(o object) bool {
		return !matches(o, query)
	})
	if !query.Has("page") && !query.Has("per_page") {
		return objects
	}
	page, perPage := positive(query, "page", 1), positive(query, "per_page", defaultPerPage)
	start := min((page-1)*perPage, len(objects))
	return objects[start:min(start+perPage, len(objects))]
}

// envelope returns the paginated object that the Permit API answers a GET of a collection with
// query, on a route that answers no JSON array: in data, the objects that match each attribute
// filter of query, as matches says, in the order in which they were created, on the page of page
// and per_page, the first page of 30 by default, also when query holds neither; in total_count,
// the number of the objects that match; and in page_count, the number of pages of per_page of
// them.
func envelope(objects []object, query url.Values) object {
	matching := slices.DeleteFunc(append([]object{}, objects...), func(o object) bool {
		return !matches(o, query)
	})
	page, perPage := positive(query, "page", 1), positive(query, "per_page", defaultPerPage)
	start := min((page-1)*perPage, len(matching))
	return object{
		"data":        matching[start:min(start+perPage, len(matching))],
		"total_count": len(matching),
		"page_count":  (len(matching) + perPage - 1) / perPage,
	}
}

// matches reports whether the attributes of o match each attribute filter of query, a parameter
// attr_<name>: o has the attribute name, and each value of the parameter is the attribute's
// value as a query writes it, a string as it is and any other value as its JSON text, such as
// true or 10.
func matches(o object, query url.Values) bool {
	attributes, _ := o["attributes"].(object)
	for param, values := range query {
		name, ok := strings.CutPrefix(param, "attr_")
		if !ok {
			continue
		}
		value, ok := attributes[name]
		if !ok {
			return false
		}
		text, ok := value.(string)
		if !ok {
			data, err := json.Marshal(value)
			if err != nil {
				return false
			}
			text = string(data)
		}
		for _, want := range values {
			if want != text {
				return false
			}
		}
	}
	return true
}

// holds reports whether o holds each field of fields with the same value.
func holds(o, fields object) bool {
	for k, v := range fields {
		if !reflect.DeepEqual(o[k], v) {
			return false
		}
	}
	return true
}

// create adds an object with fields to the collection of a POST to rt, whose path has segments,
// with a new ID and the IDs of the organization, the project and the environment of the path. An
// object of the collection with the identifier of fields is a conflict. The caller holds f.mu.
func (f *Fake) create(rt route, segments []string, fields object) (int, any) {
	path := collectionOf(segments)
	if n, ok := identOf(fields); ok {
		if _, taken := find(f.store[path], n); taken {
			return http.StatusConflict, errorBody("CONFLICT", "an object has the key "+n)
		}
	}
	o := maps.Clone(fields)
	o["id"] = f.newID()
	for i, s := range rt.segments {
		switch s {
		case "{proj_id}":
			o["organization_id"], o["project_id"] = f.org, segments[i]
		case "{env_id}":
			o["environment_id"] = segments[i]
		}
	}
	f.store[path] = append(f.store[path], o)
	return http.StatusOK, o
}

// item answers a request to a route whose path ends with a parameter, which names an object of
// the collection before it: a GET reads it, a PATCH or a PUT with a body sets its fields, and a
// DELETE deletes it and revokes the API keys of an environment. The caller holds f.mu.
func (f *Fake) item(rt route, segments []string, fields object) (int, any) {
	path := collectionOf(segments[:len(segments)-1])
	o, ok := find(f.store[path], segments[len(segments)-1])
	if !ok {
		return http.StatusNotFound, errorBody("NOT_FOUND", "no object "+
			segments[len(segments)-1])
	}
	id, _ := o["id"].(string)
	switch {
	case rt.Method == http.MethodGet:
		return http.StatusOK, o
	case (rt.Method == http.MethodPatch || rt.Method == http.MethodPut) && fields != nil:
		maps.Copy(o, fields)
		o["id"] = id
		return http.StatusOK, o
	case rt.Method == http.MethodDelete:
		f.store[path] = slices.DeleteFunc(f.store[path], func(other object) bool {
			return other["id"] == id
		})
		maps.DeleteFunc(f.keys, func(_ string, s scope) bool { return s.environment == id })
		return http.StatusNoContent, nil
	}
	return f.unsupported(rt)
}

// createEnvironment is the route of the create of an environment, as which a copy into a new
// environment creates it.
var createEnvironment = route{
	Route:    Route{Method: http.MethodPost, Template: "/v2/projects/{proj_id}/envs"},
	segments: []string{"", "v2", "projects", "{proj_id}", "envs"},
}

// environmentsOf returns the key of the collection of the environments of the project whose ID
// is project.
func environmentsOf(project string) string {
	return collectionOf([]string{"", "v2", "projects", project, "envs"})
}

// stats answers GET /v2/projects/{proj_id}/envs/{env_id}/stats, whose path has segments, with
// IDs in place of keys: the environment, with statistics of zero and one PDP configuration,
// whose client secret is the environment's API key, as a PDP authenticates with it, and whose
// OPAL token is a fake key of the environment that no other reply holds. The caller holds f.mu.
func (f *Fake) stats(segments []string) (int, any) {
	project, id := segments[3], segments[5]
	env, _ := find(f.store[environmentsOf(project)], id)
	o := maps.Clone(env)
	key := f.issue(scope{project: project, environment: id})
	o["pdp_configs"] = []object{{
		"id": f.newID(), "organization_id": f.org, "project_id": project,
		"environment_id": id, "client_secret": key["secret"],
		"opal_server_access_token": "permit_key_FAKEopal" + strings.ReplaceAll(id, "-", ""),
	}}
	o["stats"] = object{
		"roles": 0, "users": 0, "policies": 0, "resources": 0, "tenants": 0,
		"has_decision_logs": false, "members": []object{}, "mau": 0,
	}
	return http.StatusOK, o
}

// copyEnvironment answers a copy of the environment of the path, which has segments, with IDs in
// place of keys, into the target_env of fields: a new environment, which it creates in the
// project as a POST to its environments does, or an existing one, by its key or its ID. It copies
// no object, as the fake keeps none that a scenario reads from a copy. A copy, op copy_environment,
// answers 201 with the environment of the target; a copy in the background, op
// copy_environment_async, answers 202 with a task that is processing, whose result the fake keeps
// for copyResult, as having succeeded. A target that names neither, a new environment whose key
// the project has, and an existing environment that it does not have get 422, 409 and 404. The
// caller holds f.mu.
func (f *Fake) copyEnvironment(op string, segments []string, fields object) (int, any) {
	project := segments[3]
	target, _ := fields["target_env"].(object)
	var env object
	switch created, existing := target["new"], target["existing"]; {
	case created != nil:
		attrs, _ := created.(object)
		status, answer := f.create(createEnvironment, segments[:5], attrs)
		if status != http.StatusOK {
			return status, answer
		}
		env, _ = answer.(object)
	case existing != nil:
		ident, _ := existing.(string)
		found, ok := find(f.store[environmentsOf(project)], ident)
		if !ok {
			return http.StatusNotFound, errorBody("NOT_FOUND", "no environment "+ident)
		}
		env = found
	default:
		return http.StatusUnprocessableEntity, errorBody("VALIDATION_ERROR",
			"the target names no environment")
	}
	if op == opCopy {
		return http.StatusCreated, env
	}
	tasks := collectionOf(segments) + "/tasks"
	task := object{"task_id": f.newID(), "status": "processing"}
	f.store[tasks] = append(f.store[tasks], object{
		"task_id": task["task_id"], "status": "success", "result": env,
	})
	return http.StatusAccepted, task
}

// copyResult answers GET /v2/projects/{proj_id}/envs/{env_id}/copy/async/{task_id}/result, whose
// path has segments: the task of a copy in the background of the environment of the path, or 404
// for a task that it does not have. The caller holds f.mu.
func (f *Fake) copyResult(segments []string) (int, any) {
	tasks := collectionOf(segments[:len(segments)-2]) + "/tasks"
	for _, task := range f.store[tasks] {
		if task["task_id"] == segments[len(segments)-2] {
			return http.StatusOK, task
		}
	}
	return http.StatusNotFound, errorBody("NOT_FOUND", "no task "+segments[len(segments)-2])
}

// rolePermissions answers a grant, op assign_permissions_to_role, or a removal, op
// remove_permissions_from_role, of the permissions of fields to or from the role of the path,
// which has segments, with IDs in place of keys, as resolve found it: the role with the
// permissions that it did not have added, or with those of fields left out, as the Permit API
// skips a permission that the role has already, or does not have. The caller holds f.mu.
func (f *Fake) rolePermissions(op string, segments []string, fields object) (int, any) {
	role, _ := find(f.store[collectionOf(segments[:6])], segments[6])
	have, _ := role["permissions"].([]any)
	given, _ := fields["permissions"].([]any)
	kept := slices.DeleteFunc(append([]any{}, have...), func(p any) bool {
		return op == opRemoveRolePermissions && slices.Contains(given, p)
	})
	if op == opAssignRolePermissions {
		for _, p := range given {
			if !slices.Contains(kept, p) {
				kept = append(kept, p)
			}
		}
	}
	role["permissions"] = kept
	return http.StatusOK, role
}

// roleRelatives answers the ancestors, op get_role_ancestors, or the descendants, op
// get_role_descendants, of the role of the path, which has segments, with IDs in place of keys, as
// resolve found it: the roles that it extends, or that extend it, directly or through other
// roles, in the order in which the fake finds them, as a JSON array in the roles of an object.
// The caller holds f.mu.
func (f *Fake) roleRelatives(op string, segments []string) (int, any) {
	roles := f.store[collectionOf(segments[:6])]
	start, _ := find(roles, segments[6])
	found := []object{}
	queue := []object{start}
	for len(queue) > 0 {
		role := queue[0]
		queue = queue[1:]
		for _, other := range roles {
			step := op == opRoleAncestors && extends(role, other) ||
				op == opRoleDescendants && extends(other, role)
			known := other["id"] == start["id"] || slices.ContainsFunc(found, func(o object) bool {
				return o["id"] == other["id"]
			})
			if step && !known {
				found = append(found, other)
				queue = append(queue, other)
			}
		}
	}
	return http.StatusOK, object{"roles": found}
}

// extends reports whether role extends the role other: whether the extends of role hold the key
// of other.
func extends(role, other object) bool {
	keys, _ := role["extends"].([]any)
	return slices.Contains(keys, other["key"])
}

// createRole is the route of the create of a role, as which a bulk replace creates a role.
var createRole = route{
	Route:    Route{Method: http.MethodPost, Template: "/v2/schema/{proj_id}/{env_id}/roles"},
	segments: []string{"", "v2", "schema", "{proj_id}", "{env_id}", "roles"},
}

// bulkRoles answers PUT /v2/schema/{proj_id}/{env_id}/bulk/roles, whose path has segments, with
// IDs in place of keys: it creates each role of the operations of fields whose key the
// environment does not have, as a POST to its roles does, and replaces the definition of each
// that it has, keeping its ID, and answers with the keys of the roles that it created and of
// those that it replaced. A create that the POST would answer with an error, such as 409 for a
// key that is the ID of another role, ends the bulk replace with that answer, after the
// operations before it. The caller holds f.mu.
func (f *Fake) bulkRoles(segments []string, fields object) (int, any) {
	scope := append(slices.Clone(segments[:5]), "roles")
	roles := collectionOf(scope)
	created, updated := []any{}, []any{}
	operations, _ := fields["operations"].([]any)
	for _, operation := range operations {
		definition, _ := operation.(object)
		key, _ := definition["key"].(string)
		i := slices.IndexFunc(f.store[roles], func(o object) bool { return o["key"] == key })
		if i < 0 {
			if status, body := f.create(createRole, scope, definition); status != http.StatusOK {
				return status, body
			}
			created = append(created, key)
			continue
		}
		replaced := maps.Clone(definition)
		for _, kept := range []string{"id", "organization_id", "project_id", "environment_id"} {
			replaced[kept] = f.store[roles][i][kept]
		}
		f.store[roles][i] = replaced
		updated = append(updated, key)
	}
	return http.StatusOK, object{"created": created, "updated": updated}
}

// unsupported fails the test for a request that the fake does not implement, such as a POST
// without a body, and answers it with 501.
func (f *Fake) unsupported(rt route) (int, any) {
	f.tb.Errorf("fake: %s %s (%s): the fake does not implement this request", rt.Method,
		rt.Template, rt.Op)
	return http.StatusNotImplemented, errorBody("NOT_IMPLEMENTED", "the fake does not implement "+
		"this request")
}
