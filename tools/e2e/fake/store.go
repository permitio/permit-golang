package fake

import (
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
// creates an object, a GET lists the collection, as listed says, and a DELETE with a body removes
// the objects that hold each of its fields. The caller holds f.mu.
func (f *Fake) collection(rt route, segments []string, fields object, query url.Values,
) (int, any) {
	path := collectionOf(segments)
	switch {
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
// they were created: every object when query holds neither page nor per_page, and else the page
// of page and per_page, the first page of 30 by default, as the Permit API pages a list.
func listed(objects []object, query url.Values) []object {
	if !query.Has("page") && !query.Has("per_page") {
		return append([]object{}, objects...)
	}
	page, perPage := positive(query, "page", 1), positive(query, "per_page", defaultPerPage)
	start := min((page-1)*perPage, len(objects))
	return append([]object{}, objects[start:min(start+perPage, len(objects))]...)
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

// unsupported fails the test for a request that the fake does not implement, such as a POST
// without a body, and answers it with 501.
func (f *Fake) unsupported(rt route) (int, any) {
	f.tb.Errorf("fake: %s %s (%s): the fake does not implement this request", rt.Method,
		rt.Template, rt.Op)
	return http.StatusNotImplemented, errorBody("NOT_IMPLEMENTED", "the fake does not implement "+
		"this request")
}
