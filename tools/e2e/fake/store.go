package fake

import (
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
)

// The operations that the fake answers with more than its store.
const (
	opScope          = "get_api_key_scope"
	opEnvironmentKey = "get_environment_api_key"
)

// answer returns the status and the body of the reply to a request to rt, whose path has
// segments, with IDs in place of keys, and whose body holds fields, nil without a body, sent with
// an API key of the scope key. The caller holds f.mu.
func (f *Fake) answer(rt route, segments []string, fields object, key scope) (int, any) {
	switch rt.Op {
	case opScope:
		body := object{"organization_id": f.org, "project_id": key.project}
		if key.environment != "" {
			body["environment_id"] = key.environment
		}
		return http.StatusOK, body
	case opEnvironmentKey:
		return http.StatusOK, f.issue(scope{project: segments[3], environment: segments[4]})
	}
	if !isParam(rt.segments[len(rt.segments)-1]) {
		return f.collection(rt, segments, fields)
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
// creates an object, a GET lists the collection and a DELETE with a body removes the objects that
// hold each of its fields. The caller holds f.mu.
func (f *Fake) collection(rt route, segments []string, fields object) (int, any) {
	path := collectionOf(segments)
	switch {
	case rt.Method == http.MethodGet:
		return http.StatusOK, append([]object{}, f.store[path]...)
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

// unsupported fails the test for a request that the fake does not implement, such as a POST
// without a body, and answers it with 501.
func (f *Fake) unsupported(rt route) (int, any) {
	f.tb.Errorf("fake: %s %s (%s): the fake does not implement this request", rt.Method,
		rt.Template, rt.Op)
	return http.StatusNotImplemented, errorBody("NOT_IMPLEMENTED", "the fake does not implement "+
		"this request")
}
