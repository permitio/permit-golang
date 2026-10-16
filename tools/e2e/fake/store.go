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

	opAssignResourceRolePermissions = "assign_permissions_to_resource_role"
	opRemoveResourceRolePermissions = "remove_permissions_from_resource_role"
	opResourceRoleAncestors         = "get_resource_role_ancestors"
	opResourceRoleDescendants       = "get_resource_role_descendants"

	opCreateImplicitGrant           = "create_implicit_grant"
	opDeleteImplicitGrant           = "delete_implicit_grant"
	opUpdateImplicitGrantConditions = "update_implicit_grants_conditions"

	opConditionSetAncestors   = "get_condition_set_ancestors"
	opConditionSetDescendants = "get_condition_set_descendants"

	opListSetPermissions   = "list_set_permissions"
	opAssignSetPermissions = "assign_set_permissions" //nolint:gosec // G101: an operation ID

	opReplaceUser      = "replace_user"
	opAssignUserRole   = "assign_role_to_user" //nolint:gosec // G101: an operation ID
	opUnassignUserRole = "unassign_role_from_user"

	opListRoleAssignments         = "list_role_assignments"
	opListRoleAssignmentsDetailed = "list_role_assignments_detailed"
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
	case opAssignRolePermissions, opRemoveRolePermissions, opAssignResourceRolePermissions,
		opRemoveResourceRolePermissions:
		if fields != nil {
			return f.rolePermissions(rt.Op, segments, fields)
		}
		return f.unsupported(rt)
	case opRoleAncestors, opRoleDescendants, opResourceRoleAncestors, opResourceRoleDescendants:
		return f.roleRelatives(rt.Op, segments)
	case opCreateImplicitGrant, opDeleteImplicitGrant, opUpdateImplicitGrantConditions:
		if fields != nil {
			return f.implicitGrant(rt.Op, segments, fields)
		}
		return f.unsupported(rt)
	case opBulkReplaceRoles:
		if fields != nil {
			return f.bulkRoles(segments, fields)
		}
		return f.unsupported(rt)
	case opConditionSetAncestors, opConditionSetDescendants:
		return f.conditionSetRelatives(rt.Op, segments, query)
	case opListSetPermissions:
		return http.StatusOK, listed(setRules(f.store[collectionOf(segments)], query), query)
	case opAssignSetPermissions:
		if fields != nil {
			return f.assignSetRule(rt, segments, fields)
		}
		return f.unsupported(rt)
	case opReplaceUser:
		if fields != nil {
			return f.replaceUser(rt, segments, fields)
		}
		return f.unsupported(rt)
	case opAssignUserRole, opUnassignUserRole:
		if fields != nil {
			return f.userRole(rt, segments, fields)
		}
		return f.unsupported(rt)
	case opListRoleAssignments:
		return http.StatusOK, listed(roleAssignments(f.store[collectionOf(segments)], query), query)
	case opListRoleAssignmentsDetailed:
		return f.detailedRoleAssignments(segments, query)
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

// replaceUser answers PUT /v2/facts/{proj_id}/{env_id}/users/{user_id}, whose path has segments,
// with IDs in place of keys, as the Permit API replaces a user: the user of the path becomes the
// user of fields, keeps its ID and the IDs of its scope, and loses each field that fields leaves
// out, with 200; or, when the environment has no such user, the user of fields is created as a
// POST to the users creates it, with 201. The caller holds f.mu.
func (f *Fake) replaceUser(rt route, segments []string, fields object) (int, any) {
	users := segments[:len(segments)-1]
	path := collectionOf(users)
	i := slices.IndexFunc(f.store[path], func(o object) bool {
		return o["id"] == segments[len(segments)-1]
	})
	if i < 0 {
		status, created := f.create(rt, users, fields)
		if status == http.StatusOK {
			status = http.StatusCreated
		}
		return status, created
	}
	replaced := maps.Clone(fields)
	for _, kept := range []string{"id", "organization_id", "project_id", "environment_id"} {
		replaced[kept] = f.store[path][i][kept]
	}
	f.store[path][i] = replaced
	return http.StatusOK, replaced
}

// userRole answers an assignment, op assign_role_to_user, or an unassignment, op
// unassign_role_from_user, of the role of fields in the tenant of fields to or from the user of
// the path, which has segments, with IDs in place of keys, as resolve found it. The fake keeps the
// roles of a user in its associated_tenants, which a get of the user returns, as the Permit API
// lists them: an entry for each tenant, with the keys of the user's roles in it. The role and the
// tenant are those of the environment, by their keys or IDs, or the request gets 404. An
// assignment adds the role to the tenant's entry, which it adds when the user has none, and
// answers the role assignment; an unassignment removes the role from the entry, and answers 204,
// or 404 when the user does not have the role in the tenant, as the API's documentation says. A
// role on a resource instance, a role without a tenant and the assignment of a role that the user
// has already, whose replies are not confirmed, are requests that the fake does not implement.
// The caller holds f.mu.
func (f *Fake) userRole(rt route, segments []string, fields object) (int, any) {
	if _, onInstance := fields["resource_instance"]; onInstance || text(fields, "tenant") == "" {
		return f.unsupported(rt)
	}
	project, environment := segments[3], segments[4]
	roles := collectionOf([]string{"", "v2", "schema", project, environment, "roles"})
	role, ok := find(f.store[roles], text(fields, "role"))
	if !ok {
		return http.StatusNotFound, errorBody("NOT_FOUND", "no role "+text(fields, "role"))
	}
	tenants := collectionOf([]string{"", "v2", "facts", project, environment, "tenants"})
	tenant, ok := find(f.store[tenants], text(fields, "tenant"))
	if !ok {
		return http.StatusNotFound, errorBody("NOT_FOUND", "no tenant "+text(fields, "tenant"))
	}
	user, _ := find(f.store[collectionOf(segments[:6])], segments[6])
	entries, _ := user["associated_tenants"].([]any)
	i := slices.IndexFunc(entries, func(e any) bool {
		entry, _ := e.(object)
		return entry["tenant"] == tenant["key"]
	})
	entry := object{"tenant": tenant["key"], "roles": []any{}, "status": "active"}
	if i >= 0 {
		entry, _ = entries[i].(object)
	}
	held, _ := entry["roles"].([]any)
	has := slices.Contains(held, role["key"])
	switch {
	case rt.Op == opUnassignUserRole && !has:
		return http.StatusNotFound, errorBody("NOT_FOUND", "the user does not have the role "+
			text(role, "key")+" in the tenant "+text(tenant, "key"))
	case rt.Op == opUnassignUserRole:
		entry["roles"] = slices.DeleteFunc(slices.Clone(held), func(r any) bool {
			return r == role["key"]
		})
		return http.StatusNoContent, nil
	case has:
		return f.unsupported(rt)
	}
	entry["roles"] = append(held, role["key"])
	if i < 0 {
		user["associated_tenants"] = append(entries, entry)
	}
	return http.StatusOK, object{
		"id": f.newID(), "user": user["key"], "role": role["key"], "tenant": tenant["key"],
		"user_id": user["id"], "role_id": role["id"], "tenant_id": tenant["id"],
		"organization_id": f.org, "project_id": project, "environment_id": environment,
	}
}

// roleAssignmentFilters are the filters of the lists of the role assignments: each is a query
// parameter and the field of a role assignment that it matches.
var roleAssignmentFilters = []string{"user", "role", "tenant", "resource_instance"}

// roleAssignments returns the role assignments of assignments, in order, whose fields user, role,
// tenant and resource_instance, as their create gave them, each equal one of the values of the
// query parameter of the same name, when query holds it.
func roleAssignments(assignments []object, query url.Values) []object {
	return slices.DeleteFunc(slices.Clone(assignments), func(a object) bool {
		for _, name := range roleAssignmentFilters {
			if query.Has(name) && !slices.Contains(query[name], text(a, name)) {
				return true
			}
		}
		return false
	})
}

// detailedRoleAssignments answers GET /v2/facts/{proj_id}/{env_id}/role_assignments/detailed,
// whose path has segments, with IDs in place of keys: the role assignments of the environment
// that the filters of query keep, as roleAssignments says, each with its user, its role, its
// tenant and its resource instance as the objects of the environment that it names, with the
// fields of the detailed list, in the paginated object of the page of query, as envelope says,
// the only form of the route's reply. A role assignment that names an object that the
// environment does not have holds an empty object for it. The caller holds f.mu.
func (f *Fake) detailedRoleAssignments(segments []string, query url.Values) (int, any) {
	s := scope{project: segments[3], environment: segments[4]}
	kept := roleAssignments(f.store[collectionOf(segments[:len(segments)-1])], query)
	detailed := make([]object, 0, len(kept))
	for _, a := range kept {
		d := object{"role": fieldsOf(f.assignedRole(s, a), "id", "key", "name", "permissions")}
		for _, name := range []string{"id", "organization_id", "project_id", "environment_id"} {
			d[name] = a[name]
		}
		user, _ := find(f.objectsOf(s, "facts", "users"), text(a, "user"))
		d["user"] = fieldsOf(user, "id", "key", "email", "first_name", "last_name", "attributes")
		tenant, _ := find(f.objectsOf(s, "facts", "tenants"), text(a, "tenant"))
		d["tenant"] = fieldsOf(tenant, "id", "key", "name", "attributes")
		if instance := text(a, "resource_instance"); instance != "" {
			o, _ := find(f.objectsOf(s, "facts", "resource_instances"), instance)
			d["resource_instance"] = fieldsOf(o, "id", "key", "resource", "attributes")
		}
		detailed = append(detailed, d)
	}
	return http.StatusOK, envelope(detailed, query)
}

// assignedRole returns the role of a, a role assignment of the environment of s: a role of the
// environment, for a role in a tenant, or a role of the resource of its instance, for a role on
// an instance; nil when the environment has no such role. The caller holds f.mu.
func (f *Fake) assignedRole(s scope, a object) object {
	roles := f.objectsOf(s, "schema", "roles")
	if resourceType, _, ok := strings.Cut(text(a, "resource_instance"), ":"); ok {
		resource, _ := find(f.objectsOf(s, "schema", "resources"), resourceType)
		roles = f.store[collectionOf([]string{
			"", "v2", "schema", s.project, s.environment, "resources", text(resource, "id"),
			"roles",
		})]
	}
	role, _ := find(roles, text(a, "role"))
	return role
}

// fieldsOf returns the fields of o whose names are among names, those that o holds; an empty
// object for a nil o.
func fieldsOf(o object, names ...string) object {
	fields := object{}
	for _, name := range names {
		if v, ok := o[name]; ok {
			fields[name] = v
		}
	}
	return fields
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

// rolePermissions answers a grant, op assign_permissions_to_role or
// assign_permissions_to_resource_role, or a removal, op remove_permissions_from_role or
// remove_permissions_from_resource_role, of the permissions of fields to or from the role of the
// path, which has segments, with IDs in place of keys, as resolve found it: the role with the
// permissions that it did not have added, or with those of fields left out, as the Permit API
// skips a permission that the role has already, or does not have. A permission of a role of a
// resource given as <resource>:<action> is the action's key, as the role holds the keys of the
// actions of its resource. The caller holds f.mu.
func (f *Fake) rolePermissions(op string, segments []string, fields object) (int, any) {
	role, _ := find(f.store[collectionOf(segments[:len(segments)-2])], segments[len(segments)-2])
	have, _ := role["permissions"].([]any)
	given, _ := fields["permissions"].([]any)
	if op == opAssignResourceRolePermissions || op == opRemoveResourceRolePermissions {
		resource, _ := find(f.store[collectionOf(segments[:6])], segments[6])
		prefix := text(resource, "key") + ":"
		given = slices.Clone(given)
		for i, p := range given {
			if s, ok := p.(string); ok {
				given[i] = strings.TrimPrefix(s, prefix)
			}
		}
	}
	remove := op == opRemoveRolePermissions || op == opRemoveResourceRolePermissions
	kept := slices.DeleteFunc(append([]any{}, have...), func(p any) bool {
		return remove && slices.Contains(given, p)
	})
	if !remove {
		for _, p := range given {
			if !slices.Contains(kept, p) {
				kept = append(kept, p)
			}
		}
	}
	role["permissions"] = kept
	return http.StatusOK, role
}

// roleRelatives answers the ancestors, op get_role_ancestors or get_resource_role_ancestors, or
// the descendants, op get_role_descendants or get_resource_role_descendants, of the role of the
// path, which has segments, with IDs in place of keys, as resolve found it: the roles of its
// collection that it extends, or that extend it, directly or through other roles, in the order
// in which the fake finds them, as a JSON array in the roles of an object. The caller holds f.mu.
func (f *Fake) roleRelatives(op string, segments []string) (int, any) {
	roles := f.store[collectionOf(segments[:len(segments)-2])]
	start, _ := find(roles, segments[len(segments)-2])
	ancestors := op == opRoleAncestors || op == opResourceRoleAncestors
	found := []object{}
	queue := []object{start}
	for len(queue) > 0 {
		role := queue[0]
		queue = queue[1:]
		for _, other := range roles {
			step := ancestors && extends(role, other) || !ancestors && extends(other, role)
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

// implicitGrant answers a create, op create_implicit_grant, or a delete, op
// delete_implicit_grant, of the implicit grant of the rule of fields of the role of the path,
// which has segments, with IDs in place of keys, as resolve found it, or a change of the
// conditions of its implicit grants, op update_implicit_grants_conditions, to fields. The fake
// keeps the rules in the users_with_role of the granted_to of the role, and the conditions in its
// when, as the Permit API describes a role of a resource. A create finds the resource, its role
// and the relation of the resource of the path that the rule names, by their keys or IDs, or gets
// 404, adds the rule with their IDs, and answers it. A delete removes each rule that names the
// role, the resource and the relation of fields as they are given, and answers 204, or 404 when
// there is none. A change of the conditions sets them to fields, and answers them. The caller
// holds f.mu.
func (f *Fake) implicitGrant(op string, segments []string, fields object) (int, any) {
	role, _ := find(f.store[collectionOf(segments[:8])], segments[8])
	grantedTo, ok := role["granted_to"].(object)
	if !ok {
		grantedTo = object{}
		role["granted_to"] = grantedTo
	}
	rules, _ := grantedTo["users_with_role"].([]any)
	switch op {
	case opUpdateImplicitGrantConditions:
		grantedTo["when"] = fields
		return http.StatusOK, fields
	case opDeleteImplicitGrant:
		kept := slices.DeleteFunc(slices.Clone(rules), func(r any) bool {
			rule, _ := r.(object)
			return rule["role"] == fields["role"] && rule["on_resource"] == fields["on_resource"] &&
				rule["linked_by_relation"] == fields["linked_by_relation"]
		})
		if len(kept) == len(rules) {
			return http.StatusNotFound, errorBody("NOT_FOUND", "the role has no such rule")
		}
		grantedTo["users_with_role"] = kept
		return http.StatusNoContent, nil
	}
	resources := segments[:6]
	related, ok := find(f.store[collectionOf(resources)], text(fields, "on_resource"))
	if !ok {
		return http.StatusNotFound, errorBody("NOT_FOUND", "no resource "+
			text(fields, "on_resource"))
	}
	relatedRoles := collectionOf(append(slices.Clone(resources), text(related, "id"), "roles"))
	relatedRole, ok := find(f.store[relatedRoles], text(fields, "role"))
	if !ok {
		return http.StatusNotFound, errorBody("NOT_FOUND", "no role "+text(fields, "role"))
	}
	relations := collectionOf(append(slices.Clone(segments[:7]), "relations"))
	relation, ok := find(f.store[relations], text(fields, "linked_by_relation"))
	if !ok {
		return http.StatusNotFound, errorBody("NOT_FOUND", "no relation "+
			text(fields, "linked_by_relation"))
	}
	rule := maps.Clone(fields)
	rule["role_id"], rule["resource_id"] = relatedRole["id"], related["id"]
	rule["relation_id"] = relation["id"]
	grantedTo["users_with_role"] = append(rules, rule)
	return http.StatusOK, rule
}

// extends reports whether role extends the role other: whether the extends of role hold the key
// of other.
func extends(role, other object) bool {
	keys, _ := role["extends"].([]any)
	return slices.Contains(keys, other["key"])
}

// conditionSetRelatives answers the ancestors, op get_condition_set_ancestors, or the
// descendants, op get_condition_set_descendants, of the condition set of the path, which has
// segments, with IDs in place of keys, as resolve found it: the sets that its parent_id leads to,
// its parent first, or the sets whose parent_id leads to it, in the order in which the fake finds
// them, as the page of query of a JSON array, as listed says. The caller holds f.mu.
func (f *Fake) conditionSetRelatives(op string, segments []string, query url.Values,
) (int, any) {
	sets := f.store[collectionOf(segments[:6])]
	start, _ := find(sets, segments[6])
	found := []object{}
	queue := []object{start}
	for len(queue) > 0 {
		set := queue[0]
		queue = queue[1:]
		for _, other := range sets {
			step := op == opConditionSetAncestors && set["parent_id"] == other["id"] ||
				op == opConditionSetDescendants && other["parent_id"] == set["id"]
			known := other["id"] == start["id"] || slices.ContainsFunc(found, func(o object) bool {
				return o["id"] == other["id"]
			})
			if step && !known {
				found = append(found, other)
				queue = append(queue, other)
			}
		}
	}
	return http.StatusOK, listed(found, query)
}

// setRuleFields are the fields of a condition set rule that say what it grants: each is a filter
// of the list of the rules, and a create of a rule whose three fields another rule has grants
// nothing new.
var setRuleFields = []string{"user_set", "permission", "resource_set"}

// setRules returns the condition set rules of rules, in order, whose fields user_set, permission
// and resource_set each equal the query parameter of the same name, when query holds it.
func setRules(rules []object, query url.Values) []object {
	return slices.DeleteFunc(slices.Clone(rules), func(rule object) bool {
		for _, name := range setRuleFields {
			if query.Has(name) && rule[name] != query.Get(name) {
				return true
			}
		}
		return false
	})
}

// assignSetRule answers POST /v2/facts/{proj_id}/{env_id}/set_rules, whose path has segments,
// with IDs in place of keys: it creates the rule of fields, as a POST to a collection creates an
// object, and answers with a JSON array of the rule, as the Permit API answers. The API skips a
// rule that grants what a rule that it holds grants already, with a reply that is not confirmed,
// so the fake does not implement such a create. The caller holds f.mu.
func (f *Fake) assignSetRule(rt route, segments []string, fields object) (int, any) {
	granted := url.Values{}
	for _, name := range setRuleFields {
		value, _ := fields[name].(string)
		granted.Set(name, value)
	}
	if len(setRules(f.store[collectionOf(segments)], granted)) > 0 {
		return f.unsupported(rt)
	}
	status, rule := f.create(rt, segments, fields)
	return status, []any{rule}
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
