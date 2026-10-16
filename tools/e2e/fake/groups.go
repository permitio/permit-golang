package fake

import (
	"net/http"
	"net/url"
	"reflect"
	"slices"
)

// The operations of the groups of an environment and of their users, which group answers.
const (
	opCreateGroup     = "create_group"
	opListGroups      = "list_direct_group"
	opGetGroup        = "get_direct_group"
	opDeleteGroup     = "delete_group"
	opListGroupUsers  = "list_group_users"
	opAssignGroupUser = "assign_user_to_group" //nolint:gosec // G101: an operation ID
	opRemoveGroupUser = "remove_user_from_group"

	opAssignGroupRole   = "assign_role_to_group" //nolint:gosec // G101: an operation ID
	opRemoveGroupRole   = "remove_role_from_group"
	opListGroupRoles    = "list_group_roles"
	opAssignGroupGroup  = "assign_group_to_group"
	opRemoveGroupGroup  = "remove_group_from_group"
	opListGroupChildren = "list_group_children"
	opListGroupParents  = "list_group_parents"
)

// The fields of a group in the store that the API does not answer: the roles assigned to it, and
// the IDs of the groups assigned to it, its children, in the order in which they were assigned.
const (
	groupRolesField    = "roles"
	groupChildrenField = "children"
)

// groupOps are the operations that group answers.
var groupOps = []string{
	opCreateGroup, opListGroups, opGetGroup, opDeleteGroup, opListGroupUsers, opAssignGroupUser,
	opRemoveGroupUser, opAssignGroupRole, opRemoveGroupRole, opListGroupRoles, opAssignGroupGroup,
	opRemoveGroupGroup, opListGroupChildren, opListGroupParents,
}

// defaultGroupResource is the key of the resource of a group whose create leaves it out, the
// default of the spec.
const defaultGroupResource = "group"

// group answers a request of the groups of an environment to rt, whose path has segments, with
// IDs in place of the keys of the project and the environment, whose body holds fields, nil
// without a body, and whose query is query, as the Permit API answers it as far as its spec says:
//
//   - a create stores the group of fields, under its group_instance_key, with
//     group_resource_type_key group when fields leave it out, no user and no assigned role, and
//     answers it as GroupRead; a group of the environment with the same instance key is a
//     conflict;
//   - the list answers the groups of the environment, in the order in which they were created,
//     those that the filters of the query keep, the filter tenant those of the tenant, as
//     GroupReadSchema in the paginated object of the page of the query, as envelope says;
//   - a get answers the group as GroupReadSchema, and a delete deletes it;
//   - an assignment adds the user of the path, a user of the environment by its key or its ID, to
//     the group, and answers the group as GroupRead, with the IDs of its users in the order in
//     which they were assigned; a removal removes the user, and answers 204;
//   - the list of the users answers the ID and the key of each user of the group that the
//     environment still has, in the order in which they were assigned, in the paginated object
//     of the page of the query;
//   - an assignment of a role adds the role of fields to the group, as groupRoleOf says, and
//     answers the group as GroupRead, with the key of each of its roles in assigned_roles; a
//     removal removes it, and answers 204; the list of the roles answers each role, with the ID
//     and the key of its resource and of its resource instance, in the order in which they were
//     assigned, in the paginated object of the page of the query;
//   - an assignment of a group adds the group of fields to the group of the path as a child, and
//     answers the group of the path as GroupRead; a removal removes it, and answers 204; the list
//     of the children answers the children of the group that the environment still has, in the
//     order in which they were assigned, and the list of the parents the groups that hold it as a
//     child, in the order in which they were created, each as GroupReadSchema in the paginated
//     object of the page of the query.
//
// A group is named by its instance key or its ID, as the API's documentation says of the path; the
// lookup by ID is an assumption that is not confirmed yet, which no scenario relies on. The body of
// an assignment or a removal of a group names the group by its ID or as
// <group_resource_type_key>:<group_instance_key>, as the API's documentation says. A group that
// the environment does not have, and a user that it does not have or that the group does not
// hold, for a removal, get 404, and so do a role or a group of a body that the environment does
// not have or that the group does not hold, for a removal. The fake keeps no tenant of a
// membership: it reads the tenant of the body of an assignment or a removal as the check of the
// body requires it, and ignores its value. The filters resource and search, whose meaning the
// spec does not say, are filters that the fake does not model, as unmodelled says. The
// assignment of a user, a role or a group that the group holds already, whose reply is not
// confirmed, and the assignment of a group to itself are requests that the fake does not
// implement. The caller holds f.mu.
func (f *Fake) group(rt route, segments []string, fields object, query url.Values) (int, any) {
	groups := collectionOf(segments[:6])
	switch rt.Op {
	case opCreateGroup:
		if fields == nil {
			return f.unsupported(rt)
		}
		return f.createGroup(groups, segments[4], fields)
	case opListGroups:
		var listed []object
		for _, g := range f.keep(rt.Op, segments, f.store[groups], query) {
			listed = append(listed, groupSchema(g))
		}
		return http.StatusOK, envelope(listed, query)
	}
	key := segments[6]
	if rt.Op == opGetGroup {
		key = segments[7]
	}
	g, ok := findGroup(f.store[groups], key)
	if !ok {
		return http.StatusNotFound, errorBody("NOT_FOUND", "no group "+key)
	}
	switch rt.Op {
	case opGetGroup:
		return http.StatusOK, groupSchema(g)
	case opDeleteGroup:
		f.store[groups] = slices.DeleteFunc(f.store[groups], func(o object) bool {
			return o["id"] == g["id"]
		})
		return http.StatusNoContent, nil
	case opListGroupUsers:
		users := f.objectsOf(scope{project: segments[3], environment: segments[4]}, "facts",
			"users")
		var members []object
		for _, id := range groupIDs(g, "users") {
			if u, ok := find(users, id); ok {
				members = append(members, object{"id": u["id"], "key": u["key"]})
			}
		}
		return http.StatusOK, envelope(members, query)
	case opAssignGroupRole, opRemoveGroupRole, opListGroupRoles:
		return f.groupRole(rt, segments, g, fields, query)
	case opAssignGroupGroup, opRemoveGroupGroup, opListGroupChildren, opListGroupParents:
		return f.groupNesting(rt, groups, g, fields, query)
	}
	return f.groupUser(rt, segments, g, fields)
}

// createGroup stores the group of fields in the collection groups of the environment whose ID is
// env, as group says, and answers it. The caller holds f.mu.
func (f *Fake) createGroup(groups, env string, fields object) (int, any) {
	key := text(fields, "group_instance_key")
	if _, taken := findGroup(f.store[groups], key); taken {
		return http.StatusConflict, errorBody("CONFLICT", "a group has the instance key "+key)
	}
	g := object{
		"id": f.newID(), "group_instance_key": key, "group_tenant": text(fields, "group_tenant"),
		"group_resource_type_key": defaultGroupResource, "users": []any{},
		"assigned_roles": []any{},
	}
	if resource := text(fields, "group_resource_type_key"); resource != "" {
		g["group_resource_type_key"] = resource
	}
	f.store[groups] = append(f.store[groups], g)
	f.origins[text(g, "id")] = origin{request: f.request, environment: env}
	return http.StatusOK, groupRead(g)
}

// groupUser answers the assignment, op assign_user_to_group, or the removal, op
// remove_user_from_group, of the user of the path, which has segments, to or from g, as group
// says. The caller holds f.mu.
func (f *Fake) groupUser(rt route, segments []string, g object, fields object) (int, any) {
	if fields == nil {
		return f.unsupported(rt)
	}
	users := collectionOf([]string{"", "v2", "facts", segments[3], segments[4], "users"})
	user, ok := find(f.store[users], segments[8])
	if !ok {
		return http.StatusNotFound, errorBody("NOT_FOUND", "no user "+segments[8])
	}
	members := groupIDs(g, "users")
	held := slices.Contains(members, text(user, "id"))
	switch {
	case rt.Op == opRemoveGroupUser && !held:
		return http.StatusNotFound, errorBody("NOT_FOUND", "the group "+
			text(g, "group_instance_key")+" does not hold the user "+text(user, "key"))
	case rt.Op == opRemoveGroupUser:
		g["users"] = toAny(slices.DeleteFunc(members, func(id string) bool {
			return id == user["id"]
		}))
		return http.StatusNoContent, nil
	case held:
		return f.unsupported(rt)
	}
	g["users"] = toAny(append(members, text(user, "id")))
	return http.StatusOK, groupRead(g)
}

// groupRole answers the assignment, the removal and the list of the roles of g, whose path has
// segments, as group says. The caller holds f.mu.
func (f *Fake) groupRole(rt route, segments []string, g, fields object, query url.Values,
) (int, any) {
	roles, _ := g[groupRolesField].([]any)
	if rt.Op == opListGroupRoles {
		listed := make([]object, 0, len(roles))
		for _, r := range roles {
			o, _ := r.(object)
			listed = append(listed, fieldsOf(o, "key", "resource", "resource_instance"))
		}
		return http.StatusOK, envelope(listed, query)
	}
	if fields == nil {
		return f.unsupported(rt)
	}
	role, message := f.groupRoleOf(scope{project: segments[3], environment: segments[4]}, fields)
	if role == nil {
		return http.StatusNotFound, errorBody("NOT_FOUND", message)
	}
	i := slices.IndexFunc(roles, func(r any) bool { return reflect.DeepEqual(r, role) })
	switch {
	case rt.Op == opRemoveGroupRole && i < 0:
		return http.StatusNotFound, errorBody("NOT_FOUND", "the group "+
			text(g, "group_instance_key")+" does not hold the role "+text(fields, "role"))
	case rt.Op == opRemoveGroupRole:
		setGroupRoles(g, slices.Delete(roles, i, i+1))
		return http.StatusNoContent, nil
	case i >= 0:
		return f.unsupported(rt)
	}
	setGroupRoles(g, append(roles, role))
	return http.StatusOK, groupRead(g)
}

// groupRoleOf returns the role of fields, the body of an assignment or a removal of a role, as
// the store of a group holds it: the key of the role, the ID and the key of its resource and of
// its resource instance, and the ID of its tenant. The body names each by its key or its ID, and
// the resource instance also as <resource>:<key>, an assumption that is not confirmed yet, which
// no scenario relies on. It returns nil and a message when the environment of s does not have
// one of them, or when the role or the resource instance is not one of the resource. The caller
// holds f.mu.
func (f *Fake) groupRoleOf(s scope, fields object) (object, string) {
	resource, ok := find(f.objectsOf(s, "schema", "resources"), text(fields, "resource"))
	if !ok {
		return nil, "no resource " + text(fields, "resource")
	}
	roles := f.store[collectionOf([]string{
		"", "v2", "schema", s.project, s.environment, "resources", text(resource, "id"), "roles",
	})]
	role, ok := find(roles, text(fields, "role"))
	if !ok {
		return nil, "no role " + text(fields, "role") + " of the resource " + text(resource, "key")
	}
	instances := f.objectsOf(s, "facts", "resource_instances")
	instance, ok := find(instances, text(fields, "resource_instance"))
	if !ok {
		instance, ok = find(instances, text(resource, "key")+":"+text(fields, "resource_instance"))
	}
	if !ok || text(instance, "resource") != text(resource, "key") {
		return nil, "no resource instance " + text(fields, "resource_instance") + " of the " +
			"resource " + text(resource, "key")
	}
	tenant, ok := find(f.objectsOf(s, "facts", "tenants"), text(fields, "tenant"))
	if !ok {
		return nil, "no tenant " + text(fields, "tenant")
	}
	return object{
		"key":               role["key"],
		"resource":          object{"id": resource["id"], "key": resource["key"]},
		"resource_instance": object{"id": instance["id"], "key": instance["key"]},
		"tenant":            tenant["id"],
	}, ""
}

// setGroupRoles sets the roles of g to roles, and its assigned_roles to the key of each.
func setGroupRoles(g object, roles []any) {
	keys := make([]any, 0, len(roles))
	for _, r := range roles {
		o, _ := r.(object)
		keys = append(keys, o["key"])
	}
	g[groupRolesField], g["assigned_roles"] = roles, keys
}

// groupNesting answers the assignment and the removal of a group to and from g, and the lists of
// the children and of the parents of g, which are in the collection groups, as group says. The
// caller holds f.mu.
func (f *Fake) groupNesting(rt route, groups string, g, fields object, query url.Values,
) (int, any) {
	children := groupIDs(g, groupChildrenField)
	var listed []object
	switch rt.Op {
	case opListGroupChildren:
		for _, id := range children {
			if child, ok := findGroup(f.store[groups], id); ok {
				listed = append(listed, groupSchema(child))
			}
		}
		return http.StatusOK, envelope(listed, query)
	case opListGroupParents:
		for _, parent := range f.store[groups] {
			if slices.Contains(groupIDs(parent, groupChildrenField), text(g, "id")) {
				listed = append(listed, groupSchema(parent))
			}
		}
		return http.StatusOK, envelope(listed, query)
	}
	if fields == nil {
		return f.unsupported(rt)
	}
	ref := text(fields, "group_instance_key")
	child, ok := findGroupRef(f.store[groups], ref)
	if !ok {
		return http.StatusNotFound, errorBody("NOT_FOUND", "no group "+ref)
	}
	id := text(child, "id")
	held := slices.Contains(children, id)
	switch {
	case rt.Op == opRemoveGroupGroup && !held:
		return http.StatusNotFound, errorBody("NOT_FOUND", "the group "+
			text(g, "group_instance_key")+" does not hold the group "+ref)
	case rt.Op == opRemoveGroupGroup:
		g[groupChildrenField] = toAny(slices.DeleteFunc(children, func(c string) bool {
			return c == id
		}))
		return http.StatusNoContent, nil
	case held || id == text(g, "id"):
		return f.unsupported(rt)
	}
	g[groupChildrenField] = toAny(append(children, id))
	return http.StatusOK, groupRead(g)
}

// findGroup returns the group of groups whose ID or instance key is ident.
func findGroup(groups []object, ident string) (object, bool) {
	for _, g := range groups {
		if g["id"] == ident || g["group_instance_key"] == ident {
			return g, true
		}
	}
	return nil, false
}

// findGroupRef returns the group of groups that ref, the group_instance_key of a body, names: by
// its ID, or as <group_resource_type_key>:<group_instance_key>.
func findGroupRef(groups []object, ref string) (object, bool) {
	for _, g := range groups {
		if g["id"] == ref ||
			text(g, "group_resource_type_key")+":"+text(g, "group_instance_key") == ref {
			return g, true
		}
	}
	return nil, false
}

// groupIDs returns the IDs that g holds in its field of that name, such as users, in the order in
// which they were assigned.
func groupIDs(g object, field string) []string {
	held, _ := g[field].([]any)
	ids := make([]string, 0, len(held))
	for _, id := range held {
		if s, ok := id.(string); ok {
			ids = append(ids, s)
		}
	}
	return ids
}

// toAny returns ids as the []any of a JSON array.
func toAny(ids []string) []any {
	values := make([]any, 0, len(ids))
	for _, id := range ids {
		values = append(values, id)
	}
	return values
}

// groupRead returns g as the Permit API answers a create and an assignment: GroupRead, which has
// no ID.
func groupRead(g object) object {
	return fieldsOf(g, "group_instance_key", "group_resource_type_key", "group_tenant",
		"assigned_roles", "users")
}

// groupSchema returns g as the Permit API answers a get and a list: GroupReadSchema, with its ID
// and without its users and its roles.
func groupSchema(g object) object {
	return fieldsOf(g, "id", "group_instance_key", "group_resource_type_key", "group_tenant")
}
