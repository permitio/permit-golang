//go:build e2e

package e2e

import (
	"cmp"
	"context"
	"slices"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework/seed"
)

// assigned is a role assignment as a scenario compares it: its user, its role and its tenant, and
// whether it is on a resource instance, whose form the reply of a list may give in its own way.
type assigned struct {
	user, role, tenant string
	onInstance         bool
}

// sortedAssignments returns the assigned of each of list, which key gives, sorted by role.
func sortedAssignments[T any](list []T, key func(T) assigned) []assigned {
	got := make([]assigned, len(list))
	for i, a := range list {
		got[i] = key(a)
	}
	slices.SortFunc(got, func(a, b assigned) int { return cmp.Compare(a.role, b.role) })
	return got
}

// plainAssigned returns a, a role assignment of List, as a scenario compares it.
func plainAssigned(a models.RoleAssignmentRead) assigned {
	return assigned{a.User, a.Role, a.Tenant, a.ResourceInstance != ""}
}

// detailedAssigned returns a, a role assignment of ListDetailed, as a scenario compares it.
func detailedAssigned(a models.RoleAssignmentDetailedRead) assigned {
	return assigned{a.User.Key, a.Role.Key, a.Tenant.Key, a.ResourceInstance != nil}
}

// roleAssignmentLifecycle checks the calls of the role assignments with the environment-level API
// key of the scratch environment, and no WithScope, on a user, a role, a tenant, and a role of a
// resource on one of its instances of its own seed, whose keys start with the run's prefix. A list
// of the role assignments of the user is an empty page before any. It assigns the role in the
// tenant and the role of the resource on the instance with Assign, each registered with the
// ledger before, and finds both, the one on the instance too, in the lists of the user's role
// assignments of List, All, ListDetailed and AllDetailed. It removes both with Unassign, after
// which List of the user is an empty page again.
func roleAssignmentLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		user, tenant := s.cfg.prefix.Key("sdk-ra-user"), s.cfg.prefix.Key("sdk-ra-tenant")
		role, viewer := s.cfg.prefix.Key("sdk-ra-role"), s.cfg.prefix.Key("sdk-ra-viewer")
		resource, instance := s.cfg.prefix.Key("sdk-ra-doc"), "doc-1"
		s.seed(t, world{
			resources: []seed.Resource{{Key: resource, Actions: []string{"read"}}},
			roles:     []seed.Role{{Key: role}},
			resourceRoles: []seed.ResourceRole{
				{Resource: resource, Key: viewer, Permissions: []string{"read"}},
			},
			tenants:   []string{tenant},
			users:     []string{user},
			instances: []seed.Instance{{Resource: resource, Key: instance, Tenant: tenant}},
		})
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		mine := &permit.RoleAssignmentsListOptions{Users: []string{user}}
		expectNone(ctx, t, c, mine)
		assignments := []seed.RoleAssignment{
			{User: user, Role: role, Tenant: tenant},
			{User: user, Role: viewer, Tenant: tenant, ResourceInstance: resource + ":" + instance},
		}
		b := seed.New(s.api, s.ledger, s.env)
		for _, a := range assignments {
			if err := b.RegisterRoleAssignment(a); err != nil {
				t.Fatalf("setup: %v", err)
			}
			got, err := c.RoleAssignments.Assign(ctx, models.RoleAssignmentCreate{
				User: a.User, Role: a.Role, Tenant: a.Tenant, ResourceInstance: a.ResourceInstance,
			})
			if err != nil || got.ID == "" || plainAssigned(*got) != (assigned{
				a.User, a.Role, a.Tenant, a.ResourceInstance != "",
			}) {
				t.Fatalf("Assign of %+v returned %+v, %v; want the role assignment", a, got, err)
			}
		}
		want := sortedAssignments(assignments, func(a seed.RoleAssignment) assigned {
			return assigned{a.User, a.Role, a.Tenant, a.ResourceInstance != ""}
		})
		listAssignments(ctx, t, c, mine, want)
		for _, a := range assignments {
			if err := c.RoleAssignments.Unassign(ctx, models.RoleAssignmentRemove{
				User: a.User, Role: a.Role, Tenant: a.Tenant, ResourceInstance: a.ResourceInstance,
			}); err != nil {
				t.Fatalf("Unassign of %+v: %v", a, err)
			}
		}
		expectNone(ctx, t, c, mine)
	}
}

// expectNone checks that List with opts returns a page without role assignments, whose Data is
// empty and not nil.
func expectNone(ctx context.Context, t *testing.T, c *permit.Client,
	opts *permit.RoleAssignmentsListOptions,
) {
	t.Helper()
	page, err := c.RoleAssignments.List(ctx, opts)
	if err != nil || page.Data == nil || len(page.Data) != 0 {
		t.Fatalf("List with the users %q returned %#v, %v; want a page whose Data is empty and "+
			"not nil", opts.Users, page, err)
	}
}

// listAssignments checks that List and ListDetailed with opts, and All and AllDetailed, each
// return the role assignments of want, and no other.
func listAssignments(ctx context.Context, t *testing.T, c *permit.Client,
	opts *permit.RoleAssignmentsListOptions, want []assigned,
) {
	t.Helper()
	page, err := c.RoleAssignments.List(ctx, opts)
	if err != nil {
		t.Fatalf("List with the users %q: %v", opts.Users, err)
	}
	detailed, err := c.RoleAssignments.ListDetailed(ctx, opts)
	if err != nil {
		t.Fatalf("ListDetailed with the users %q: %v", opts.Users, err)
	}
	var all []models.RoleAssignmentRead
	for a, err := range c.RoleAssignments.All(ctx, opts) {
		if err != nil {
			t.Fatalf("All with the users %q: %v", opts.Users, err)
		}
		all = append(all, a)
	}
	var allDetailed []models.RoleAssignmentDetailedRead
	for a, err := range c.RoleAssignments.AllDetailed(ctx, opts) {
		if err != nil {
			t.Fatalf("AllDetailed with the users %q: %v", opts.Users, err)
		}
		allDetailed = append(allDetailed, a)
	}
	for name, got := range map[string][]assigned{
		"List":         sortedAssignments(page.Data, plainAssigned),
		"All":          sortedAssignments(all, plainAssigned),
		"ListDetailed": sortedAssignments(detailed.Data, detailedAssigned),
		"AllDetailed":  sortedAssignments(allDetailed, detailedAssigned),
	} {
		if !slices.Equal(got, want) {
			t.Errorf("%s with the users %q returned %+v, want %+v", name, opts.Users, got, want)
		}
	}
}
