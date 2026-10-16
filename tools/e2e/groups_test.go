//go:build e2e

package e2e

import (
	"context"
	"errors"
	"iter"
	"net/url"
	"slices"
	"strconv"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework/seed"
)

// groupMembers is the number of the users that groupLifecycle assigns to its group: more than one
// page of groupPerPage, so that AllUsers reads past the first page.
const (
	groupMembers = 31
	groupPerPage = 10
)

// groupLifecycle checks the calls of the groups with the environment-level API key of the scratch
// environment, and no WithScope, so that the client acts on the environment of its key, on a group
// of its own in a tenant of its seed, whose keys start with the run's prefix. It registers each
// group with the ledger before it creates it, and the ledger deletes it by its instance key and
// checks that a get of it answers 404. It creates the group and a second group in another tenant
// of its seed, gets the first, finds it alone in a page of List and in All with the filter of its
// tenant, assigns it the 31 users of its seed, reads them all through AllUsers, 10 users to a page,
// and through a first page of ListUsers, removes one, which AllUsers then no longer yields, deletes
// the group, waits until a get of it matches ErrNotFound, and deletes it again, which returns nil.
// The ledger deletes the second group.
func groupLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		key, tenant := s.cfg.prefix.Key("sdk-group"), s.cfg.prefix.Key("sdk-group-tenant")
		otherKey := s.cfg.prefix.Key("sdk-group-other")
		otherTenant := s.cfg.prefix.Key("sdk-group-other-tenant")
		users := make([]string, groupMembers)
		for i := range users {
			users[i] = s.cfg.prefix.Key("sdk-group-user-" + strconv.Itoa(i))
		}
		s.seed(t, world{tenants: []string{tenant, otherTenant}, users: users})
		groups := "/v2/schema/" + url.PathEscape(s.env.ProjectID) + "/" +
			url.PathEscape(s.env.ID) + "/groups/"
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		for _, g := range []models.GroupCreate{
			{GroupInstanceKey: key, GroupTenant: tenant},
			{GroupInstanceKey: otherKey, GroupTenant: otherTenant},
		} {
			s.ledger.Register("group "+g.GroupInstanceKey,
				s.api.Remover(groups+url.PathEscape(g.GroupInstanceKey)),
				s.api.Absence(groups+"direct/"+url.PathEscape(g.GroupInstanceKey),
					framework.AbsenceTimeout))
			created, err := c.Groups.Create(ctx, g)
			if err != nil {
				t.Fatalf("Create of the group %s: %v", g.GroupInstanceKey, err)
			}
			if created.GroupInstanceKey != g.GroupInstanceKey ||
				created.GroupTenant != g.GroupTenant {
				t.Fatalf("Create returned the group %q of the tenant %q, want %s of %s",
					created.GroupInstanceKey, created.GroupTenant, g.GroupInstanceKey,
					g.GroupTenant)
			}
		}
		got, err := c.Groups.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get of the group %s: %v", key, err)
		}
		if got.GroupInstanceKey != key || got.GroupTenant != tenant || got.ID == "" {
			t.Errorf("Get returned the group %q (ID %q) of the tenant %q, want %s of %s with an "+
				"ID", got.GroupInstanceKey, got.ID, got.GroupTenant, key, tenant)
		}
		checkListedGroup(ctx, t, c, tenant, got.ID)
		membership := models.GroupAssignUser{Tenant: tenant}
		for _, user := range users {
			if _, err := c.Groups.AssignUser(ctx, key, user, membership); err != nil {
				t.Fatalf("AssignUser of the user %s to the group %s: %v", user, key, err)
			}
		}
		checkGroupMembers(ctx, t, c, key, users)
		if err := c.Groups.RemoveUser(ctx, key, users[0], membership); err != nil {
			t.Fatalf("RemoveUser of the user %s from the group %s: %v", users[0], key, err)
		}
		checkGroupMembers(ctx, t, c, key, users[1:])
		if err := c.Groups.Delete(ctx, key); err != nil {
			t.Fatalf("Delete of the group %s: %v", key, err)
		}
		gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
			func(ctx context.Context) (bool, error) {
				_, err := c.Groups.Get(ctx, key)
				if errors.Is(err, permit.ErrNotFound) {
					return true, nil
				}
				return false, err
			})
		if gone != nil {
			t.Fatalf("Get of the deleted group %s: %v", key, gone)
		}
		if err := c.Groups.Delete(ctx, key); err != nil {
			t.Errorf("Delete of the group %s that is gone returned %v, want nil", key, err)
		}
	}
}

// checkListedGroup checks that List and All with the filter of tenant hold the group whose ID is
// id alone, as the tenant is the scenario's own and the environment holds a group of another
// tenant too.
func checkListedGroup(ctx context.Context, t *testing.T, c *permit.Client, tenant, id string) {
	t.Helper()
	opts := &permit.GroupsListOptions{Tenant: tenant}
	page, err := c.Groups.List(ctx, opts)
	if err != nil || len(page.Data) != 1 || page.Data[0].ID != id {
		t.Errorf("List with the filter tenant=%s returned %+v, %v; want the group %s alone",
			tenant, page, err, id)
	}
	var all []string
	for g, err := range c.Groups.All(ctx, opts) {
		if err != nil {
			t.Fatalf("All with the filter tenant=%s: %v", tenant, err)
		}
		all = append(all, g.ID)
	}
	if len(all) != 1 || all[0] != id {
		t.Errorf("All with the filter tenant=%s yielded %q, want %s alone", tenant, all, id)
	}
}

// checkGroupMembers checks that AllUsers of the group whose instance key is key yields the users
// whose keys are want, each once, in any order, reading pages of groupPerPage users, and that the
// first page of ListUsers holds groupPerPage of them, of len(want) in all when the reply counts
// them.
func checkGroupMembers(ctx context.Context, t *testing.T, c *permit.Client, key string,
	want []string,
) {
	t.Helper()
	opts := &permit.ListOptions{PerPage: groupPerPage}
	var got []string
	for user, err := range c.Groups.AllUsers(ctx, key, opts) {
		if err != nil {
			t.Fatalf("AllUsers of the group %s: %v", key, err)
		}
		got = append(got, user.Key)
	}
	if !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))) {
		t.Errorf("AllUsers of the group %s yielded %d users %q, want the %d users %q", key,
			len(got), got, len(want), want)
	}
	page, err := c.Groups.ListUsers(ctx, key, opts)
	if err != nil || len(page.Data) != groupPerPage ||
		page.TotalCount != nil && *page.TotalCount != len(want) {
		t.Errorf("ListUsers of the group %s returned %+v, %v; want %d users of %d", key, page,
			err, groupPerPage, len(want))
	}
}

// groupRolesAndNesting checks the roles and the nesting of groups with the environment-level API
// key of the scratch environment, and no WithScope, on groups of its own in a tenant of its seed,
// whose keys start with the run's prefix. Its seed is a resource, two roles of the resource, the
// tenant and an instance of the resource in it. It registers each group with the ledger before it
// creates it. It assigns both roles on the instance to the parent group, reads them through
// AllRoles, one role to a page, and a first page of ListRoles; assigns the two child groups to the
// parent, each named as group:<key>, as the API's documentation names a group in the body, reads
// them through AllChildren, one group to a page, and a first page of ListChildren, and the parent
// through AllParents and ListParents of a child; then removes a child and a role, which the lists
// then no longer hold. The ledger deletes the groups.
//
// The live run confirms what the spec does not say: that the body takes group:<key>, that the
// assigned group is listed as a child of the group of the path, and how the reply names the
// resource of a role. That a member of the group gets the role through the group is left to the
// local harness, as the fake PDP of the dry run knows no role derivations.
func groupRolesAndNesting(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		key := s.cfg.prefix.Key
		resource, tenant := key("sdk-group-doc"), key("sdk-nest-tenant")
		roles := []string{key("sdk-group-editor"), key("sdk-group-viewer")}
		s.seed(t, world{
			resources: []seed.Resource{{Key: resource, Actions: []string{"read"}}},
			resourceRoles: []seed.ResourceRole{
				{Resource: resource, Key: roles[0], Permissions: []string{"read"}},
				{Resource: resource, Key: roles[1], Permissions: []string{"read"}},
			},
			tenants:   []string{tenant},
			instances: []seed.Instance{{Resource: resource, Key: "doc-1", Tenant: tenant}},
		})
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		parent, children := key("sdk-nest-parent"), []string{key("sdk-nest-a"), key("sdk-nest-b")}
		for _, g := range append([]string{parent}, children...) {
			groupCreate(ctx, t, s, c, models.GroupCreate{GroupInstanceKey: g, GroupTenant: tenant})
		}
		grants := make([]models.GroupAddRole, 0, len(roles))
		for _, role := range roles {
			grant := models.GroupAddRole{
				Role: role, Resource: resource, ResourceInstance: "doc-1", Tenant: tenant,
			}
			got, err := c.Groups.AssignRole(ctx, parent, grant)
			if err != nil {
				t.Fatalf("AssignRole of the role %s to the group %s: %v", role, parent, err)
			}
			if got.GroupInstanceKey != parent {
				t.Errorf("AssignRole returned the group %q, want %s", got.GroupInstanceKey, parent)
			}
			grants = append(grants, grant)
		}
		checkGroupRoles(ctx, t, c, parent, grants)
		for _, child := range children {
			assignment := models.GroupAssignment{GroupInstanceKey: "group:" + child}
			if _, err := c.Groups.AssignGroup(ctx, parent, assignment); err != nil {
				t.Fatalf("AssignGroup of the group %s to the group %s: %v", child, parent, err)
			}
		}
		checkGroupList(t, "AllChildren of "+parent, c.Groups.AllChildren(ctx, parent,
			&permit.ListOptions{PerPage: 1}), children)
		page, err := c.Groups.ListChildren(ctx, parent, &permit.ListOptions{PerPage: 1})
		if err != nil || len(page.Data) != 1 ||
			page.TotalCount != nil && *page.TotalCount != len(children) {
			t.Errorf("ListChildren of the group %s returned %+v, %v; want 1 group of %d", parent,
				page, err, len(children))
		}
		checkGroupList(t, "AllParents of "+children[0],
			c.Groups.AllParents(ctx, children[0], nil), []string{parent})
		page, err = c.Groups.ListParents(ctx, children[0], nil)
		if err != nil || len(page.Data) != 1 || page.Data[0].GroupInstanceKey != parent {
			t.Errorf("ListParents of the group %s returned %+v, %v; want the group %s", children[0],
				page, err, parent)
		}
		removed := models.GroupAssignment{GroupInstanceKey: "group:" + children[0]}
		if err := c.Groups.RemoveGroup(ctx, parent, removed); err != nil {
			t.Fatalf("RemoveGroup of the group %s from the group %s: %v", children[0], parent, err)
		}
		checkGroupList(t, "AllChildren of "+parent+" after RemoveGroup",
			c.Groups.AllChildren(ctx, parent, nil), children[1:])
		if err := c.Groups.RemoveRole(ctx, parent, grants[0]); err != nil {
			t.Fatalf("RemoveRole of the role %s from the group %s: %v", roles[0], parent, err)
		}
		checkGroupRoles(ctx, t, c, parent, grants[1:])
	}
}

// groupCreate registers the group g with the ledger of s, which deletes it by its instance key
// and checks that a get of it answers 404, and then creates it with c.
func groupCreate(ctx context.Context, t *testing.T, s *suite, c *permit.Client,
	g models.GroupCreate,
) {
	t.Helper()
	groups := "/v2/schema/" + url.PathEscape(s.env.ProjectID) + "/" + url.PathEscape(s.env.ID) +
		"/groups/"
	s.ledger.Register("group "+g.GroupInstanceKey,
		s.api.Remover(groups+url.PathEscape(g.GroupInstanceKey)),
		s.api.Absence(groups+"direct/"+url.PathEscape(g.GroupInstanceKey),
			framework.AbsenceTimeout))
	if _, err := c.Groups.Create(ctx, g); err != nil {
		t.Fatalf("Create of the group %s: %v", g.GroupInstanceKey, err)
	}
}

// checkGroupRoles checks that AllRoles of the group key yields the roles of the grants want, each
// once, in any order, each of the resource that its grant names, reading pages of one role, and
// that the first page of ListRoles holds one of them, of len(want) in all when the reply counts
// them.
func checkGroupRoles(ctx context.Context, t *testing.T, c *permit.Client, key string,
	want []models.GroupAddRole,
) {
	t.Helper()
	resources := make(map[string]string, len(want))
	roles := make([]string, 0, len(want))
	for _, grant := range want {
		resources[grant.Role] = grant.Resource
		roles = append(roles, grant.Role)
	}
	opts := &permit.ListOptions{PerPage: 1}
	var got []string
	for role, err := range c.Groups.AllRoles(ctx, key, opts) {
		if err != nil {
			t.Fatalf("AllRoles of the group %s: %v", key, err)
		}
		if role.Resource.Key != resources[role.Key] || role.ResourceInstance.ID == "" {
			t.Errorf("AllRoles of the group %s yielded the role %+v, want one of the resource %q "+
				"on an instance with an ID", key, role, resources[role.Key])
		}
		got = append(got, role.Key)
	}
	if !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(roles))) {
		t.Errorf("AllRoles of the group %s yielded the roles %q, want %q", key, got, roles)
	}
	page, err := c.Groups.ListRoles(ctx, key, opts)
	if err != nil || len(page.Data) != 1 ||
		page.TotalCount != nil && *page.TotalCount != len(want) {
		t.Errorf("ListRoles of the group %s returned %+v, %v; want 1 role of %d", key, page, err,
			len(want))
	}
}

// checkGroupList checks that seq, the iterator that what names, yields the groups whose instance
// keys are want, each once, in any order.
func checkGroupList(t *testing.T, what string, seq iter.Seq2[models.GroupReadSchema, error],
	want []string,
) {
	t.Helper()
	var got []string
	for g, err := range seq {
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		got = append(got, g.GroupInstanceKey)
	}
	if !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))) {
		t.Errorf("%s yielded the groups %q, want %q", what, got, want)
	}
}
