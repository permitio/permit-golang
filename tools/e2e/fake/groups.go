package fake

import (
	"net/http"
	"net/url"
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
)

// groupOps are the operations that group answers.
var groupOps = []string{
	opCreateGroup, opListGroups, opGetGroup, opDeleteGroup, opListGroupUsers, opAssignGroupUser,
	opRemoveGroupUser,
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
//     those of the tenant of the filter tenant when the query holds it, as GroupReadSchema in the
//     paginated object of the page of the query, as envelope says;
//   - a get answers the group as GroupReadSchema, and a delete deletes it;
//   - an assignment adds the user of the path, a user of the environment by its key or its ID, to
//     the group, and answers the group as GroupRead, with the IDs of its users in the order in
//     which they were assigned; a removal removes the user, and answers 204;
//   - the list of the users answers the ID and the key of each user of the group that the
//     environment still has, in the order in which they were assigned, in the paginated object
//     of the page of the query.
//
// A group is named by its instance key or its ID: the spec does not say that the path takes an
// ID, and the lookup by ID is an assumption that is not confirmed yet, which no scenario relies
// on. A group that the environment does not have, and a user that it does not have or that the
// group does not hold, for a removal, get 404. The fake keeps no tenant of a membership: it reads
// the tenant of the body of an assignment or a removal as the check of the body requires it, and
// ignores its value. The filters resource and search, whose meaning the spec does not say, and
// the assignment of a user that the group holds already, whose reply is not confirmed, are
// requests that the fake does not implement. The caller holds f.mu.
func (f *Fake) group(rt route, segments []string, fields object, query url.Values) (int, any) {
	groups := collectionOf(segments[:6])
	switch rt.Op {
	case opCreateGroup:
		if fields == nil {
			return f.unsupported(rt)
		}
		return f.createGroup(groups, fields)
	case opListGroups:
		if query.Has("resource") || query.Has("search") {
			return f.unsupported(rt)
		}
		var listed []object
		for _, g := range f.store[groups] {
			if !query.Has("tenant") || text(g, "group_tenant") == query.Get("tenant") {
				listed = append(listed, groupSchema(g))
			}
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
		for _, id := range groupUsers(g) {
			if u, ok := find(users, id); ok {
				members = append(members, object{"id": u["id"], "key": u["key"]})
			}
		}
		return http.StatusOK, envelope(members, query)
	}
	return f.groupUser(rt, segments, g, fields)
}

// createGroup stores the group of fields in the collection groups, as group says, and answers it.
// The caller holds f.mu.
func (f *Fake) createGroup(groups string, fields object) (int, any) {
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
	members := groupUsers(g)
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

// findGroup returns the group of groups whose ID or instance key is ident.
func findGroup(groups []object, ident string) (object, bool) {
	for _, g := range groups {
		if g["id"] == ident || g["group_instance_key"] == ident {
			return g, true
		}
	}
	return nil, false
}

// groupUsers returns the IDs of the users of g, in the order in which they were assigned.
func groupUsers(g object) []string {
	held, _ := g["users"].([]any)
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
