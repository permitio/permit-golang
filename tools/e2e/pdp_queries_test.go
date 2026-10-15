//go:build e2e

package e2e

import (
	"context"
	"maps"
	"slices"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework/seed"
)

// userPermissions checks the permissions of a user who has a role in two tenants and a resource
// role on an instance in the first: UserPermissions lists an entry for each, and its filters of
// tenants and of resource types, with a context, keep the entries they name.
func userPermissions(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		const user = "permissions-user"
		s.seed(t, world{
			resources: []seed.Resource{{Key: "page", Actions: []string{"read", "edit"}}},
			roles: []seed.Role{
				{Key: "page-editor", Permissions: []string{"page:read", "page:edit"}},
			},
			resourceRoles: []seed.ResourceRole{
				{Resource: "page", Key: "page-owner", Permissions: []string{"read", "edit"}},
			},
			tenants:   []string{"permissions-a", "permissions-b"},
			users:     []string{user},
			instances: []seed.Instance{{Resource: "page", Key: "home", Tenant: "permissions-a"}},
			assignments: []seed.RoleAssignment{
				{User: user, Role: "page-editor", Tenant: "permissions-a"},
				{User: user, Role: "page-editor", Tenant: "permissions-b"},
				{
					User: user, Role: "page-owner", Tenant: "permissions-a",
					ResourceInstance: "page:home",
				},
			},
		})
		c := s.client(t, s.env.APIKey())
		who := permit.User{Key: user}
		const tenantA, tenantB, home = "__tenant:permissions-a", "__tenant:permissions-b",
			"page:home"

		await(t, s, "the entries of UserPermissions", []string{tenantA, tenantB, home},
			func(ctx context.Context) ([]string, error) {
				permissions, err := c.UserPermissions(ctx, who)
				return entriesOf(permissions, tenantA, tenantB, home), err
			})

		ctx := s.ctx(t)
		permissions, err := c.UserPermissions(ctx, who, permit.WithTenants("permissions-a"),
			permit.WithResourceTypes("__tenant", "page"),
			permit.WithDecisionContext(map[string]any{"request": 1}))
		if err != nil {
			t.Fatalf("UserPermissions of a tenant and two resource types: %v", err)
		}
		if got := slices.Sorted(maps.Keys(permissions)); !slices.Equal(got,
			[]string{tenantA, home}) {
			t.Errorf("UserPermissions of the tenant permissions-a has the entries %q, want %s "+
				"and %s", got, tenantA, home)
		}
		if p := permissions[tenantA]; !slices.Contains(p.Roles, "page-editor") ||
			!slices.Contains(p.Permissions, "page:edit") || p.Tenant == nil ||
			p.Tenant.Key != "permissions-a" {
			t.Errorf("the entry %s is %+v, want the role page-editor and page:edit", tenantA, p)
		}
		if p := permissions[home]; !slices.Contains(p.Roles, "page-owner") ||
			!slices.Contains(p.Permissions, "page:read") || p.Resource == nil ||
			p.Resource.Type != "page" || p.Resource.Key != "home" {
			t.Errorf("the entry %s is %+v, want the role page-owner and page:read", home, p)
		}
		pages, err := c.UserPermissions(ctx, who, permit.WithTenants("permissions-a"),
			permit.WithResourceTypes("page"))
		if got := slices.Sorted(maps.Keys(pages)); err != nil || !slices.Equal(got,
			[]string{home}) {
			t.Errorf("UserPermissions of the resource type page has the entries %q, %v; want %s",
				got, err, home)
		}
	}
}

// entriesOf returns those of keys that permissions has an entry under, in the order of keys.
func entriesOf(permissions map[string]permit.ObjectPermissions, keys ...string) []string {
	return slices.DeleteFunc(slices.Clone(keys), func(key string) bool {
		_, ok := permissions[key]
		return !ok
	})
}

// authorizedUsers checks the users that may view an instance in its tenant: a user with a role
// in the tenant and one with a resource role on the instance, each with that role assignment,
// and not a user with the role in another tenant.
func authorizedUsers(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		const tenant, tenantUser, owner, outsider = "photos", "photos-viewer", "summer-owner",
			"archive-viewer"
		s.seed(t, world{
			resources: []seed.Resource{{Key: "album", Actions: []string{"view"}}},
			roles:     []seed.Role{{Key: "album-viewer", Permissions: []string{"album:view"}}},
			resourceRoles: []seed.ResourceRole{
				{Resource: "album", Key: "album-owner", Permissions: []string{"view"}},
			},
			tenants:   []string{tenant, "archive"},
			users:     []string{tenantUser, owner, outsider},
			instances: []seed.Instance{{Resource: "album", Key: "summer", Tenant: tenant}},
			assignments: []seed.RoleAssignment{
				{User: tenantUser, Role: "album-viewer", Tenant: tenant},
				{
					User: owner, Role: "album-owner", Tenant: tenant,
					ResourceInstance: "album:summer",
				},
				{User: outsider, Role: "album-viewer", Tenant: "archive"},
			},
		})
		c := s.client(t, s.env.APIKey())
		summer := permit.Resource{Type: "album", Key: "summer", Tenant: tenant}
		var result permit.AuthorizedUsersResult

		await(t, s, "the users of AuthorizedUsers", []string{tenantUser, owner},
			func(ctx context.Context) ([]string, error) {
				var err error
				result, err = c.AuthorizedUsers(ctx, "view", summer,
					permit.WithDecisionContext(map[string]any{"request": true}))
				return slices.Sorted(maps.Keys(result.Users)), err
			})

		if result.Resource != "album:summer" || result.Tenant != tenant {
			t.Errorf("AuthorizedUsers is about %s in %s, want album:summer in %s",
				result.Resource, result.Tenant, tenant)
		}
		for user, want := range map[string]permit.AuthorizedUserAssignment{
			tenantUser: {
				User: tenantUser, Tenant: tenant, Resource: "__tenant:" + tenant,
				Role: "album-viewer",
			},
			owner: {User: owner, Tenant: tenant, Resource: "album:summer", Role: "album-owner"},
		} {
			if !slices.Contains(result.Users[user], want) {
				t.Errorf("the role assignments of %s are %+v, want one of %+v", user,
					result.Users[user], want)
			}
		}
	}
}
