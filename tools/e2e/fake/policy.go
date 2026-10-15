package fake

import (
	"slices"
	"strings"
)

// assignment is a role assignment that the fake PDP holds.
type assignment struct {
	user, role, tenant string

	// instance is the resource instance of a role on an instance, as <type>:<key>, or empty for
	// a role in the tenant.
	instance string

	// permissions are the permissions that its role grants, each as <type>:<action>.
	permissions []string
}

// local returns a as GET /local/role_assignments lists it, without resource_instance for a role
// in the tenant.
func (a assignment) local() object {
	o := object{"user": a.user, "role": a.role, "tenant": a.tenant}
	if a.instance != "" {
		o["resource_instance"] = a.instance
	}
	return o
}

// objectsOf returns the objects of the collection name of the environment of s under area,
// facts or schema, such as the role assignments under facts. The caller holds f.mu.
func (f *Fake) objectsOf(s scope, area, name string) []object {
	return f.store[collectionOf([]string{"", "v2", area, s.project, s.environment, name})]
}

// text returns the string of the field name of o, or the empty string.
func text(o object, name string) string {
	v, _ := o[name].(string)
	return v
}

// madeBefore returns the object of objects whose ID or identifier is ident, and false when there
// is none or the fake made it after the object whose ID is id: the IDs that the fake gives grow
// with each object that it makes.
func madeBefore(objects []object, ident, id string) (object, bool) {
	o, ok := find(objects, ident)
	return o, ok && text(o, "id") < id
}

// assignments returns the role assignments of the environment of s that the PDP holds, in the
// order in which they were made, each with the permissions that its role grants: those whose
// user, tenant and role, and for a role on an instance whose instance, the fake made before the
// role assignment. The fake control plane takes a role assignment that names an object that does
// not exist, which the Permit API refuses, so a seed that leaves out such an object, or makes it
// after the role assignment, grants nothing. The caller holds f.mu.
func (f *Fake) assignments(s scope) []assignment {
	var list []assignment
	for _, o := range f.objectsOf(s, "facts", "role_assignments") {
		id := text(o, "id")
		a := assignment{
			user: text(o, "user"), role: text(o, "role"), tenant: text(o, "tenant"),
			instance: text(o, "resource_instance"),
		}
		_, user := madeBefore(f.objectsOf(s, "facts", "users"), a.user, id)
		_, tenant := madeBefore(f.objectsOf(s, "facts", "tenants"), a.tenant, id)
		permissions, role := f.permissionsOf(s, a, id)
		if user && tenant && role {
			a.permissions = permissions
			list = append(list, a)
		}
	}
	return list
}

// permissionsOf returns the permissions that a, the role assignment of the environment of s whose
// ID is id, grants, each as <type>:<action>: those of its role, for a role in the tenant, or
// those of the resource role of the instance's type, for a role on an instance, whose actions it
// names with the type or without; and false when the fake did not make its role, or for a role
// on an instance its instance, before it. The caller holds f.mu.
func (f *Fake) permissionsOf(s scope, a assignment, id string) ([]string, bool) {
	roles := f.objectsOf(s, "schema", "roles")
	resourceType, _, onInstance := strings.Cut(a.instance, ":")
	if onInstance {
		resource, ok := find(f.objectsOf(s, "schema", "resources"), resourceType)
		if _, made := madeBefore(f.objectsOf(s, "facts", "resource_instances"), a.instance,
			id); !ok || !made {
			return nil, false
		}
		roles = f.store[collectionOf([]string{
			"", "v2", "schema", s.project, s.environment, "resources", text(resource, "id"),
			"roles",
		})]
	}
	role, ok := madeBefore(roles, a.role, id)
	if !ok {
		return nil, false
	}
	listed, _ := role["permissions"].([]any)
	permissions := make([]string, 0, len(listed))
	for _, p := range listed {
		permission, _ := p.(string)
		if onInstance && !strings.Contains(permission, ":") {
			permission = resourceType + ":" + permission
		}
		permissions = append(permissions, permission)
	}
	return permissions, true
}

// grants reports whether a allows action on r: its role grants <type>:<action>, in the tenant on
// every resource of the tenant, and on an instance on that instance only.
func (a assignment) grants(action string, r pdpResource) bool {
	if a.instance != "" && (r.Key == "" || a.instance != r.Type+":"+r.Key) {
		return false
	}
	return slices.Contains(a.permissions, r.Type+":"+action)
}

// decides returns the decision of whether user may do action on r in the environment of s: true
// when a role assignment of the user in r's tenant grants it. The caller holds f.mu.
func (f *Fake) decides(s scope, user, action string, r pdpResource) bool {
	return slices.ContainsFunc(f.assignments(s), func(a assignment) bool {
		return a.user == user && a.tenant == r.Tenant && a.grants(action, r)
	})
}
