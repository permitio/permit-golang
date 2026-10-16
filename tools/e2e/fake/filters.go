package fake

import (
	"encoding/json"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// pageParams are the query parameters of the page of a list, which listed and envelope read, and
// which the fake accepts on any request, as a request that is no list ignores them.
var pageParams = []string{"page", "per_page"}

// quietParams are the query parameters, by operation, of requests that are no lists, which the
// fake accepts and which change nothing in it: the wait of a copy of an environment in the
// background, as the fake copies at once, and the policy for a missing user of a bulk assignment
// of roles, as the fake creates no user.
var quietParams = map[string][]string{
	opCopyAsync: {"wait"}, opCopyResult: {"wait"}, opBulkAssignRole: {"missing_user_policy"},
}

// attributeLists are the lists whose queries may hold attribute filters, attr_<name>, as the SDK
// sends them, which attributeMatches applies.
var attributeLists = []string{"list_tenants", "list_roles", "list_resource_actions"}

// opListAuditLogs is the operation of the list of the audit logs.
const opListAuditLogs = "list_audit_logs"

// listRequest is a GET of a list, as a filter reads it: the segments of its path, with IDs in
// place of keys, and its query.
type listRequest struct {
	segments []string
	query    url.Values
}

// filter reports whether o, an object of the list of r, matches values, the values of the
// filter's query parameter, of which there is one but for a filter of repeatable, as refusal
// refuses a repeated value of any other. The caller holds f.mu.
type filter func(f *Fake, r listRequest, o object, values []string) bool

// The filters of the lists of the resource instances, of the relationship tuples and of the role
// assignments, whose plain and detailed lists filter alike.
var (
	instanceFilters = map[string]filter{
		"tenant": equal("tenant"), "resource": equal("resource"), "search": contains("key", "name"),
	}
	tupleFilters = map[string]filter{
		"tenant": equal("tenant"), "subject": equal("subject"), "relation": equal("relation"),
		"object": equal("object"), "subject_type": resourceOf("subject"),
		"object_type": resourceOf("object"),
	}
	assignmentFilters = map[string]filter{
		"user": oneOf("user"), "role": oneOf("role"), "tenant": oneOf("tenant"),
		"resource_instance": equal("resource_instance"),
	}
)

// listFilters are the filters that the fake applies to the lists, by the operation of the list and
// the query parameter of each, as the control plane's spec describes the parameter: a key, an ID
// or a boolean is the field of the same value as the object holds it, exactly. The spec does not
// say what search and the filter users of the audit logs match, so for those the fake applies the
// narrowest reading of the SDK's documentation of the parameter, search semantics from the SDK
// docs, confirm in the harness: search keeps the users, and the users of a tenant, whose email
// holds the text, the resources, the condition sets and the resource instances whose key or name
// holds it, case and all, an instance only when it holds each text of a search with more than
// one, and the invites whose email or key is the text, as an invite has no name; users keeps the
// audit logs whose user key, email or name is one of its values. role keeps the users that have
// the role in a tenant, as withRole says, and the users of a tenant that have it in that tenant,
// as withTenantRole says; user, role and tenant of the role assignments and resources of the
// audit logs keep those with one of the values that the query lists; and timestamp_from and
// timestamp_to keep the audit logs of their window, as inWindow says. Each other parameter of a
// list, and every parameter of a list that is not here, is one that the fake does not know, but
// those of unmodelled.
var listFilters = map[string]map[string]filter{
	"list_users":                     {"search": contains("email"), "role": withRole},
	"list_tenant_users":              {"search": contains("email"), "role": withTenantRole},
	"list_resources":                 {"search": contains("key", "name")},
	"list_condition_sets":            {"type": equal("type"), "search": contains("key", "name")},
	"list_resource_instances":        instanceFilters,
	opDetailedInstances:              instanceFilters,
	opListRelationshipTuples:         tupleFilters,
	opListRelationshipTuplesDetailed: tupleFilters,
	opListRoleAssignments:            assignmentFilters,
	opListRoleAssignmentsDetailed:    assignmentFilters,
	opListSetPermissions: {
		"user_set": equal("user_set"), "permission": equal("permission"),
		"resource_set": equal("resource_set"),
	},
	opListGroups: {"tenant": equal("group_tenant")},
	opListUserInvites: {
		"role": equal("role_id"), "tenant": equal("tenant_id"), "search": oneOf("email", "key"),
	},
	"list_api_keys": {"object_type": equal("object_type"), "proj_id": equal("project_id")},
	opListAuditLogs: {
		"pdp_id": equal("pdp_config_id"), "users": oneOf("user_key", "user_email", "user_name"),
		"decision": equal("decision"), "resources": oneOf("resource_type"),
		"tenant": equal("tenant"), "action": equal("action"), "timestamp_from": inWindow,
		"timestamp_to": inWindow,
	},
}

// repeatable are the filters, by the operation of the list, that take more than one value: those
// that the spec gives as arrays, which the SDK sends once for each value. Each other filter takes
// one value, and the SDK never repeats it.
var repeatable = map[string][]string{
	"list_resource_instances":     {"search"},
	opDetailedInstances:           {"search"},
	opListRoleAssignments:         {"user", "role", "tenant"},
	opListRoleAssignmentsDetailed: {"user", "role", "tenant"},
	opListAuditLogs:               {"users", "resources"},
}

// unmodelled are the filters, by the operation of the list, that the SDK sends and that the fake
// does not model: resource and search of the groups, whose meaning the spec does not say; and of
// the audit logs, sort_by, an order that the fake does not keep, as it lists the audit logs in the
// order of its store, and query, a kind of query of the PDP, which the fake's audit logs do not
// say, and whose default, the checks alone, the fake cannot apply.
var unmodelled = map[string][]string{
	opListGroups:    {"resource", "search"},
	opListAuditLogs: {"sort_by", "query"},
}

// refusal returns the message of the 400 with which the fake refuses a request of the operation
// op with query, for the first parameter of query, by name, that it refuses, and true; or false
// when it refuses none. It refuses a filter of op in unmodelled; a filter of op in listFilters, or
// an attribute filter of a list of attributeLists, that query repeats, unless repeatable names it;
// and, as a parameter that it does not know, each other parameter but those of pageParams and the
// parameters of op in quietParams.
func refusal(op string, query url.Values) (string, bool) {
	for _, name := range slices.Sorted(maps.Keys(query)) {
		_, isFilter := listFilters[op][name]
		isAttribute := strings.HasPrefix(name, "attr_") && slices.Contains(attributeLists, op)
		switch {
		case slices.Contains(unmodelled[op], name):
			return "the query parameter " + name + " is not modelled by the fake", true
		case isFilter || isAttribute:
			if len(query[name]) > 1 && !slices.Contains(repeatable[op], name) {
				return "the query parameter " + name + " takes one value, and the request " +
					"repeats it", true
			}
		case !slices.Contains(pageParams, name) && !slices.Contains(quietParams[op], name):
			return "the fake does not know the query parameter " + name, true
		}
	}
	return "", false
}

// keep returns the objects of objects, in order, that each filter of query keeps, as listFilters
// and attributeMatches say, for the list op whose path has segments, with IDs in place of keys.
// Each other parameter of query is one that refusal accepts and that is no filter, such as page.
// The caller holds f.mu.
func (f *Fake) keep(op string, segments []string, objects []object, query url.Values) []object {
	r := listRequest{segments: segments, query: query}
	return slices.DeleteFunc(slices.Clone(objects), func(o object) bool {
		for name, values := range query {
			if attribute, ok := strings.CutPrefix(name, "attr_"); ok {
				if !attributeMatches(o, attribute, values[0]) {
					return true
				}
				continue
			}
			if match, ok := listFilters[op][name]; ok && !match(f, r, o, values) {
				return true
			}
		}
		return false
	})
}

// scopeOf returns the scope of the path that has segments, with IDs in place of keys, such as
// /v2/facts/{proj_id}/{env_id}/users: its project and its environment.
func scopeOf(segments []string) scope {
	return scope{project: segments[3], environment: segments[4]}
}

// equal returns the filter that keeps the objects whose field name has the value of the filter,
// as hasValue says.
func equal(name string) filter {
	return func(_ *Fake, _ listRequest, o object, values []string) bool {
		return hasValue(o, name, values[0])
	}
}

// oneOf returns the filter that keeps the objects of which one of the fields names equals one of
// the values of the filter.
func oneOf(names ...string) filter {
	return func(_ *Fake, _ listRequest, o object, values []string) bool {
		return slices.ContainsFunc(names, func(name string) bool {
			return slices.Contains(values, text(o, name))
		})
	}
}

// contains returns the filter that keeps the objects that hold each value in one of the fields
// names.
func contains(names ...string) filter {
	return func(_ *Fake, _ listRequest, o object, values []string) bool {
		for _, v := range values {
			if !slices.ContainsFunc(names, func(name string) bool {
				return strings.Contains(text(o, name), v)
			}) {
				return false
			}
		}
		return true
	}
}

// resourceOf returns the filter that keeps the objects whose field name, an instance as
// <resource>:<key>, is an instance of the resource of the filter.
func resourceOf(name string) filter {
	return func(_ *Fake, _ listRequest, o object, values []string) bool {
		resource, _, _ := strings.Cut(text(o, name), ":")
		return resource == values[0]
	}
}

// withRole keeps the users that have the role of the filter, by its key, in a tenant, as
// tenantRoles says. The caller holds f.mu.
func withRole(f *Fake, r listRequest, user object, values []string) bool {
	return slices.ContainsFunc(f.tenantRoles(scopeOf(r.segments), user), func(held object) bool {
		return text(held, "role") == values[0]
	})
}

// withTenantRole keeps the users of the tenant of the path of r, /v2/facts/{proj_id}/{env_id}/
// tenants/{tenant_id}/users, that have the role of the filter, by its key, in that tenant, as
// tenantRoles says. The caller holds f.mu.
func withTenantRole(f *Fake, r listRequest, user object, values []string) bool {
	s := scopeOf(r.segments)
	tenant, _ := find(f.objectsOf(s, "facts", "tenants"), r.segments[6])
	return slices.ContainsFunc(f.tenantRoles(s, user), func(held object) bool {
		return text(held, "role") == values[0] && text(held, "tenant") == text(tenant, "key")
	})
}

// tenantRoles returns the roles that user has in a tenant and not on a resource instance, each as
// an object with the key of the role in role and the key of the tenant in tenant, as the fake
// keeps them: in a role assignment of the environment of s, in an entry of the user's
// associated_tenants, or in a role assignment of the create of the user. The caller holds f.mu.
func (f *Fake) tenantRoles(s scope, user object) []object {
	var held []object
	for _, a := range f.objectsOf(s, "facts", "role_assignments") {
		if text(a, "user") == text(user, "key") && text(a, "resource_instance") == "" {
			held = append(held, a)
		}
	}
	entries, _ := user["associated_tenants"].([]any)
	for _, e := range entries {
		entry, _ := e.(object)
		roles, _ := entry["roles"].([]any)
		for _, role := range roles {
			held = append(held, object{"role": role, "tenant": entry["tenant"]})
		}
	}
	inline, _ := user["role_assignments"].([]any)
	for _, a := range inline {
		if assignment, _ := a.(object); text(assignment, "resource_instance") == "" {
			held = append(held, assignment)
		}
	}
	return held
}

// inWindow keeps the audit logs whose timestamp, a date-time, is in the window of the query of r,
// from timestamp_from to timestamp_to, Unix times in seconds, both included, when the query holds
// both and neither is 0, as the SDK's documentation says that the API filters by time; and keeps
// each audit log otherwise. A bound that is no integer keeps no audit log, and a window keeps
// none whose timestamp is no date-time.
func inWindow(_ *Fake, r listRequest, o object, _ []string) bool {
	from, fromErr := strconv.ParseInt(r.query.Get("timestamp_from"), 10, 64)
	to, toErr := strconv.ParseInt(r.query.Get("timestamp_to"), 10, 64)
	switch {
	case !r.query.Has("timestamp_from") || !r.query.Has("timestamp_to"):
		return true
	case fromErr != nil || toErr != nil:
		return false
	case from == 0 || to == 0:
		return true
	}
	at, err := time.Parse(time.RFC3339, text(o, "timestamp"))
	return err == nil && !at.Before(time.Unix(from, 0)) && !at.After(time.Unix(to, 0))
}

// attributeMatches reports whether o has the attribute name with value, the value of the filter
// attr_<name>, as hasValue says.
func attributeMatches(o object, name, value string) bool {
	attributes, _ := o["attributes"].(object)
	return hasValue(attributes, name, value)
}

// hasValue reports whether o has the field name with value as a query writes it: a string as it
// is, and any other value as its JSON text, such as true or 10.
func hasValue(o object, name, value string) bool {
	field, ok := o[name]
	if !ok {
		return false
	}
	written, ok := field.(string)
	if !ok {
		data, err := json.Marshal(field)
		if err != nil {
			return false
		}
		written = string(data)
	}
	return written == value
}
