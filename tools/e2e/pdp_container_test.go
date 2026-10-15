//go:build e2e

package e2e

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework/seed"
)

// containerOnlyMethods are the public methods of the routes that only the PDP container serves,
// which the scenario of the PDP container and the one of the cloud PDP cover. Client.CheckURL
// sends one of those routes too, and stays pending until a scenario checks its decisions.
var containerOnlyMethods = []string{
	"Client.AllTenantsCheck", "Client.UserTenants", "Client.LocalRoleAssignments",
	"Client.LocalRoleAssignmentsAll",
}

// localAssignment is the user, the role and the tenant of a role assignment that the PDP
// container holds, as a scenario compares them.
type localAssignment struct {
	user, role, tenant string
}

// localAssignments returns the user, the role and the tenant of each of list, sorted by tenant.
func localAssignments(list []permit.LocalRoleAssignment) []localAssignment {
	got := make([]localAssignment, len(list))
	for i, a := range list {
		got[i] = localAssignment{user: a.User, role: a.Role, tenant: a.Tenant}
	}
	slices.SortFunc(got, func(a, b localAssignment) int {
		return strings.Compare(a.tenant, b.tenant)
	})
	return got
}

// tenantKey returns the key of a tenant of a reply.
func tenantKey(tenant permit.TenantDetails) string {
	return tenant.Key
}

// tenantsAndLocalRoleAssignments checks the routes that only the PDP container serves, for a
// user with a role in two of three tenants: AllTenantsCheck and UserTenants return those two,
// and LocalRoleAssignments and LocalRoleAssignmentsAll, a page of one at a time, the two role
// assignments, which takes LocalRoleAssignmentsAll three pages, the last one empty.
func tenantsAndLocalRoleAssignments(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		const user, role = "auditor", "sheet-auditor"
		s.seed(t, world{
			resources: []seed.Resource{{Key: "sheet", Actions: []string{"read"}}},
			roles:     []seed.Role{{Key: role, Permissions: []string{"sheet:read"}}},
			tenants:   []string{"audit-1", "audit-2", "audit-3"},
			users:     []string{user},
			assignments: []seed.RoleAssignment{
				{User: user, Role: role, Tenant: "audit-1"},
				{User: user, Role: role, Tenant: "audit-2"},
			},
		})
		log := &requestLog{}
		c := s.client(t, s.env.APIKey(), permit.WithHTTPClient(&http.Client{Transport: log}))
		who := permit.User{Key: user}
		tenants := []string{"audit-1", "audit-2"}

		await(t, s, "the tenants of AllTenantsCheck", tenants,
			func(ctx context.Context) ([]string, error) {
				allowed, err := c.AllTenantsCheck(ctx, who, "read", permit.Resource{Type: "sheet"})
				return sortedKeys(allowed, tenantKey), err
			})

		ctx := s.ctx(t)
		if got, err := c.UserTenants(ctx, who); err != nil ||
			!slices.Equal(sortedKeys(got, tenantKey), tenants) {
			t.Errorf("UserTenants = %+v, %v; want the tenants %q", got, err, tenants)
		}
		want := []localAssignment{{user, role, "audit-1"}, {user, role, "audit-2"}}
		page, err := c.LocalRoleAssignments(ctx, &permit.LocalRoleAssignmentsOptions{User: user})
		if err != nil {
			t.Errorf("LocalRoleAssignments of the user: %v", err)
		} else if got := localAssignments(page.Data); !slices.Equal(got, want) {
			t.Errorf("LocalRoleAssignments of the user = %+v, want %+v", got, want)
		}
		var all []permit.LocalRoleAssignment
		opts := &permit.LocalRoleAssignmentsOptions{
			ListOptions: permit.ListOptions{PerPage: 1}, User: user,
		}
		before := len(log.to("/local/role_assignments"))
		for a, err := range c.LocalRoleAssignmentsAll(ctx, opts) {
			if err != nil {
				t.Fatalf("LocalRoleAssignmentsAll of the user, one on a page: %v", err)
			}
			all = append(all, a)
		}
		if got := localAssignments(all); !slices.Equal(got, want) {
			t.Errorf("LocalRoleAssignmentsAll of the user, one on a page, = %+v; want %+v", got,
				want)
		}
		var pages []string
		for _, r := range log.to("/local/role_assignments")[before:] {
			pages = append(pages, r.query.Get("page")+"/"+r.query.Get("per_page"))
		}
		if want := []string{"1/1", "2/1", "3/1"}; !slices.Equal(pages, want) {
			t.Errorf("LocalRoleAssignmentsAll of the user, one on a page, asked for the pages %q, "+
				"as page/per_page; want %q", pages, want)
		}
	}
}

// containerOnlyRoutesOnCloudPDP checks each route that only the PDP container serves against the
// cloud PDP: each call returns, after one request, an error that matches ErrContainerPDPOnly,
// and not ErrNotFound, and names the route.
func containerOnlyRoutesOnCloudPDP(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		log := &requestLog{}
		c := s.client(t, s.env.APIKey(), permit.WithHTTPClient(&http.Client{Transport: log}))
		ctx := s.ctx(t)
		who := permit.User{Key: "cloud-user"}
		for _, call := range []struct {
			route string
			call  func() error
		}{
			{"/allowed/all-tenants", func() error {
				_, err := c.AllTenantsCheck(ctx, who, "read", permit.Resource{Type: "sheet"})
				return err
			}},
			{"/user-tenants", func() error {
				_, err := c.UserTenants(ctx, who)
				return err
			}},
			{"/allowed_url", func() error {
				_, err := c.CheckURL(ctx, who, http.MethodGet, "https://app.example.com/sheets",
					"")
				return err
			}},
			{"/local/role_assignments", func() error {
				_, err := c.LocalRoleAssignments(ctx, nil)
				return err
			}},
			{"/local/role_assignments", func() error {
				for _, err := range c.LocalRoleAssignmentsAll(ctx, nil) {
					return err
				}
				return errors.New("LocalRoleAssignmentsAll yielded nothing")
			}},
		} {
			before := len(log.to(call.route))
			err := call.call()
			perr, ok := errors.AsType[*permit.Error](err)
			if !errors.Is(err, permit.ErrContainerPDPOnly) || errors.Is(err, permit.ErrNotFound) ||
				!ok || perr.Route != call.route || perr.StatusCode != http.StatusNotFound ||
				!strings.Contains(err.Error(), call.route) {
				t.Errorf("the call of %s returned %v, want a *permit.Error of status 404 that "+
					"matches ErrContainerPDPOnly, not ErrNotFound, and names the route",
					call.route, err)
			}
			if n := len(log.to(call.route)) - before; n != 1 {
				t.Errorf("the call of %s sent %d requests, want one", call.route, n)
			}
		}
	}
}
