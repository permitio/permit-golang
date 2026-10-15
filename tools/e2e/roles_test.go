//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
)

// roleLifecycle checks the calls of the roles with the environment-level API key of the scratch
// environment, whose scope the client looks up, on a resource and roles of its own, whose keys
// start with the run's prefix, each of which it registers with the ledger before it creates it.
// It creates the resource with the actions read, write and share, a role base that grants read,
// and a role child that extends base, both with the attribute run, the run's prefix, base with
// attributes whose values are a number and an object too, and child with the attribute kind. It
// lists the roles whose kind is child with List, which lists child alone, and those whose run is
// the run's prefix with All, one role on each page, which lists both; each call sends one filter,
// of a string. It gets child by its key and by its ID, updates child to grant write and to extend
// no role, then grants it share and removes write. It deletes both roles, waits until a get of
// each matches ErrNotFound, and deletes child again, which returns nil.
func roleLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		schema := "/v2/schema/" + url.PathEscape(s.env.ProjectID) + "/" + url.PathEscape(s.env.ID)
		register := func(what, path string) {
			s.ledger.Register(what, s.api.Remover(schema+path),
				s.api.Absence(schema+path, framework.AbsenceTimeout))
		}
		resource := s.cfg.prefix.Key("sdk-roles")
		register("resource "+resource, "/resources/"+url.PathEscape(resource))
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		if _, err := c.Resources.Create(ctx, models.ResourceCreate{
			Key: resource, Name: resource,
			Actions: map[string]models.ActionBlockEditable{"read": {}, "write": {}, "share": {}},
		}); err != nil {
			t.Fatalf("setup: Create of the resource %s: %v", resource, err)
		}
		run := string(s.cfg.prefix)
		base, child := s.cfg.prefix.Key("sdk-role-base"), s.cfg.prefix.Key("sdk-role-child")
		for _, key := range []string{base, child} {
			register("role "+key, "/roles/"+url.PathEscape(key))
		}
		created, err := c.Roles.Create(ctx, models.RoleCreate{
			Key: base, Name: base, Permissions: []string{resource + ":read"},
			Attributes: models.Attributes{
				"run": run, "level": 1, "limits": map[string]any{"max": 2},
			},
		})
		if err != nil {
			t.Fatalf("Create of the role %s: %v", base, err)
		}
		limits, _ := created.Attributes["limits"].(map[string]any)
		if created.Key != base || created.ID == "" || created.EnvironmentID != s.env.ID ||
			created.Attributes["level"] != json.Number("1") || limits["max"] != json.Number("2") {
			t.Fatalf("Create returned the role %q (ID %q) of the environment %q with the "+
				"attributes %#v, want %s in %s with the level 1 and the limits max 2",
				created.Key, created.ID, created.EnvironmentID, created.Attributes, base, s.env)
		}
		extending, err := c.Roles.Create(ctx, models.RoleCreate{
			Key: child, Name: child, Extends: []string{base},
			Attributes: models.Attributes{"run": run, "kind": "child"},
		})
		if err != nil {
			t.Fatalf("Create of the role %s: %v", child, err)
		}
		if !slices.Equal(extending.Extends, []string{base}) {
			t.Errorf("Create returned the role %s extending %q, want %s", child,
				extending.Extends, base)
		}
		listRoles(ctx, t, c, run, base, child)
		for _, ref := range []string{child, extending.ID} {
			got, getErr := c.Roles.Get(ctx, ref)
			if getErr != nil {
				t.Fatalf("Get of the role %s: %v", ref, getErr)
			}
			if got.ID != extending.ID || !slices.Equal(got.Extends, []string{base}) {
				t.Errorf("Get of %s returned the role %s extending %q, want %s extending %s",
					ref, got.ID, got.Extends, extending.ID, base)
			}
		}
		write, share := resource+":write", resource+":share"
		updated, err := c.Roles.Update(ctx, child, models.RoleUpdate{
			Permissions: []string{write}, Extends: []string{},
		})
		if err != nil {
			t.Fatalf("Update of the role %s: %v", child, err)
		}
		if !slices.Equal(updated.Permissions, []string{write}) || len(updated.Extends) != 0 {
			t.Errorf("Update returned the permissions %q and the extends %q, want %s and none",
				updated.Permissions, updated.Extends, write)
		}
		assigned, err := c.Roles.AssignPermissions(ctx, child, []string{share})
		if err != nil {
			t.Fatalf("AssignPermissions of %s to the role %s: %v", share, child, err)
		}
		if got := slices.Sorted(slices.Values(assigned.Permissions)); !slices.Equal(got,
			slices.Sorted(slices.Values([]string{write, share}))) {
			t.Errorf("AssignPermissions returned the permissions %q, want %s and %s", got,
				write, share)
		}
		removed, err := c.Roles.RemovePermissions(ctx, child, []string{write})
		if err != nil {
			t.Fatalf("RemovePermissions of %s from the role %s: %v", write, child, err)
		}
		if !slices.Equal(removed.Permissions, []string{share}) {
			t.Errorf("RemovePermissions returned the permissions %q, want %s",
				removed.Permissions, share)
		}
		for _, key := range []string{child, base} {
			if err := c.Roles.Delete(ctx, key); err != nil {
				t.Fatalf("Delete of the role %s: %v", key, err)
			}
		}
		for _, key := range []string{child, base} {
			gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
				func(ctx context.Context) (bool, error) {
					_, err := c.Roles.Get(ctx, key)
					if errors.Is(err, permit.ErrNotFound) {
						return true, nil
					}
					return false, err
				})
			if gone != nil {
				t.Fatalf("Get of the deleted role %s: %v", key, gone)
			}
		}
		if err := c.Roles.Delete(ctx, child); err != nil {
			t.Errorf("Delete of the role %s that is gone returned %v, want nil", child, err)
		}
	}
}

// listRoles checks that List, with the attribute filter kind child, lists child alone, as base
// has no kind, and that All, with the attribute filter run and one role on each page, lists base
// and child, and no other role. Each call sends one string filter, the case that a check against
// the API has measured.
func listRoles(ctx context.Context, t *testing.T, c *permit.Client, run, base, child string) {
	t.Helper()
	page, err := c.Roles.List(ctx, &permit.RolesListOptions{
		Attributes: []permit.AttributeFilter{permit.AttributeEquals("kind", "child")},
	})
	if err != nil {
		t.Fatalf("List of the roles whose kind is child: %v", err)
	}
	if keys := roleKeys(page.Data); !slices.Equal(keys, []string{child}) {
		t.Errorf("List of the roles whose kind is child returned %q, want %s alone", keys, child)
	}
	var all []models.RoleRead
	for role, err := range c.Roles.All(ctx, &permit.RolesListOptions{
		ListOptions: permit.ListOptions{PerPage: 1},
		Attributes:  []permit.AttributeFilter{permit.AttributeEquals("run", run)},
	}) {
		if err != nil {
			t.Fatalf("All of the roles whose run is %s: %v", run, err)
		}
		all = append(all, role)
	}
	if keys := roleKeys(all); !slices.Equal(keys, []string{base, child}) {
		t.Errorf("All of the roles whose run is %s, one on a page, listed %q, want %s and %s",
			run, keys, base, child)
	}
}

// roleKeys returns the keys of roles, sorted.
func roleKeys(roles []models.RoleRead) []string {
	keys := []string{}
	for _, r := range roles {
		keys = append(keys, r.Key)
	}
	slices.Sort(keys)
	return keys
}
