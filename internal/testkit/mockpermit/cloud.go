package mockpermit

import "strings"

// containerOnlyRoutes are the routes of the PDP container (permitio/pdp-v2 0.9.16) that the
// cloud PDP does not serve: it answers each with 404 and an empty body, as it does any route it
// does not have.
var containerOnlyRoutes = []struct{ method, path string }{
	{"POST", "/allowed/all-tenants"},
	{"POST", "/allowed_url"},
	{"POST", "/user-tenants"},
	{"GET", "/local/role_assignments"},
	{"GET", "/ready"},
	{"POST", "/kong"},
	{"POST", "/nginx_allowed"},
	{"GET", "/callbacks"},
	{"POST", "/callbacks"},
	{"GET", "/callbacks/{key}"},
	{"DELETE", "/callbacks/{key}"},
	{"POST", "/data-updater/trigger"},
	{"POST", "/policy-updater/trigger"},
	{"GET", "/healthchecks/opa/healthy"},
	{"GET", "/healthchecks/opa/ready"},
	{"GET", "/healthchecks/opa/system"},
	{"GET", "/opal-server/connectivity"},
	{"POST", "/opal-server/connectivity/disable"},
	{"POST", "/opal-server/connectivity/enable"},
	{"GET", "/policy-store/config"},
	{"POST", "/trino/allowed"},
	{"POST", "/trino/row-filter"},
	{"POST", "/trino/batch-column-masking"},
}

// containerOnlyPrefixes are the two mounts of the PDP container's facts routes, /facts and
// /v2/facts/{proj_id}/{env_id}, in which a {name} segment fits any one segment. The container
// serves every method on every path below a mount, and the cloud PDP none.
var containerOnlyPrefixes = [][]string{{"facts"}, {"v2", "facts", "{proj_id}", "{env_id}"}}

// containerOnly reports whether a route that only the PDP container serves fits method and the
// path segments, with fits deciding whether a container-only route's segments fit them: matches
// for a request's path, overlaps for a route's.
func containerOnly(method string, segments []string, fits func(a, b []string) bool) bool {
	for _, prefix := range containerOnlyPrefixes {
		if len(segments) > len(prefix) && fits(prefix, segments[:len(prefix)]) {
			return true
		}
	}
	for _, rt := range containerOnlyRoutes {
		if rt.method == method && fits(strings.Split(rt.path, "/")[1:], segments) {
			return true
		}
	}
	return false
}
