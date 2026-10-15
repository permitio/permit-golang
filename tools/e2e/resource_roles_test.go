//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
)

// resourceRoleLifecycle checks the calls of the roles of a resource with the environment-level API
// key of the scratch environment, whose scope the client looks up, on a resource of its own with
// the actions read, write and share, whose key starts with the run's prefix, and two roles of it,
// base, which grants read and has attributes whose values are a string, a number and an object, and
// child, which extends base. It registers each with the ledger before it creates it. It lists the
// roles of the resource with List, and with All, one role on each page, which list base and child;
// gets child by its key and by its ID; reads the ancestors of child, base alone, and the
// descendants of base, child alone; updates child to grant write, with Extends nil, which keeps
// base in its extends, and then to grant write and to extend no role; and grants it share and
// removes write, each sent as <resource>:<action>. A role holds the keys of the actions of its
// resource, and the checks read a permission given as <resource>:<action> as the key of the action
// too, as the API may return either form. It deletes both roles, waits until a get of each matches
// ErrNotFound, and deletes child again, which returns nil.
func resourceRoleLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		schema := "/v2/schema/" + url.PathEscape(s.env.ProjectID) + "/" + url.PathEscape(s.env.ID)
		register := func(what, path string) {
			s.ledger.Register(what, s.api.Remover(schema+path),
				s.api.Absence(schema+path, framework.AbsenceTimeout))
		}
		resource := s.cfg.prefix.Key("sdk-resource-roles")
		register("resource "+resource, "/resources/"+url.PathEscape(resource))
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		if _, err := c.Resources.Create(ctx, models.ResourceCreate{
			Key: resource, Name: resource,
			Actions: map[string]models.ActionBlockEditable{"read": {}, "write": {}, "share": {}},
		}); err != nil {
			t.Fatalf("setup: Create of the resource %s: %v", resource, err)
		}
		base, child := s.cfg.prefix.Key("sdk-rrole-base"), s.cfg.prefix.Key("sdk-rrole-child")
		for _, key := range []string{base, child} {
			register("role "+key+" of "+resource,
				"/resources/"+url.PathEscape(resource)+"/roles/"+url.PathEscape(key))
		}
		created, err := c.ResourceRoles.Create(ctx, resource, models.ResourceRoleCreate{
			Key: base, Name: base, Permissions: []string{"read"},
			Attributes: models.Attributes{
				"run": string(s.cfg.prefix), "level": 1, "limits": map[string]any{"max": 2},
			},
		})
		if err != nil {
			t.Fatalf("Create of the role %s of %s: %v", base, resource, err)
		}
		limits, _ := created.Attributes["limits"].(map[string]any)
		if created.Key != base || created.ID == "" || created.EnvironmentID != s.env.ID ||
			created.Attributes["level"] != json.Number("1") || limits["max"] != json.Number("2") {
			t.Fatalf("Create returned the role %q (ID %q) of the environment %q with the "+
				"attributes %#v, want %s in %s with the level 1 and the limits max 2",
				created.Key, created.ID, created.EnvironmentID, created.Attributes, base, s.env)
		}
		extending, err := c.ResourceRoles.Create(ctx, resource, models.ResourceRoleCreate{
			Key: child, Name: child, Extends: []string{base},
		})
		if err != nil {
			t.Fatalf("Create of the role %s of %s: %v", child, resource, err)
		}
		listResourceRoles(ctx, t, c, resource, base, child)
		for _, ref := range []string{child, extending.ID} {
			got, getErr := c.ResourceRoles.Get(ctx, resource, ref)
			if getErr != nil {
				t.Fatalf("Get of the role %s of %s: %v", ref, resource, getErr)
			}
			if got.ID != extending.ID || !slices.Equal(got.Extends, []string{base}) {
				t.Errorf("Get of %s returned the role %s extending %q, want %s extending %s",
					ref, got.ID, got.Extends, extending.ID, base)
			}
		}
		resourceRoleRelatives(ctx, t, c, resource, base, child)
		changeResourceRole(ctx, t, c, resource, base, child)
		for _, key := range []string{child, base} {
			if err := c.ResourceRoles.Delete(ctx, resource, key); err != nil {
				t.Fatalf("Delete of the role %s of %s: %v", key, resource, err)
			}
		}
		for _, key := range []string{child, base} {
			gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
				func(ctx context.Context) (bool, error) {
					_, err := c.ResourceRoles.Get(ctx, resource, key)
					if errors.Is(err, permit.ErrNotFound) {
						return true, nil
					}
					return false, err
				})
			if gone != nil {
				t.Fatalf("Get of the deleted role %s of %s: %v", key, resource, gone)
			}
		}
		if err := c.ResourceRoles.Delete(ctx, resource, child); err != nil {
			t.Errorf("Delete of the role %s of %s that is gone returned %v, want nil", child,
				resource, err)
		}
	}
}

// implicitGrantLifecycle checks the calls of the implicit grants with the environment-level API
// key of the scratch environment, whose scope the client looks up, on two resources of its own,
// whose keys start with the run's prefix: a document and a folder, its subject by the relation
// parent of the document, with the role owner of the folder and the role viewer of the document.
// It registers each resource, the relation and each role with the ledger before it creates it,
// viewer last, so that the teardown deletes viewer first, and its implicit grants with it. It
// creates the implicit grant of viewer to the users that have owner on the parent folder, which
// returns the rule with the IDs of owner, of the folder and of the relation; sets the conditions
// of the implicit grants of viewer so that they leave out an instance with a role assigned on it
// directly, and then back, which returns each; and deletes the grant. It gets viewer after each
// write: the granted_to of viewer holds the rule after the create, and the conditions sent in its
// when after each update, and it no longer holds the rule after the delete, which it waits for.
// It then deletes the grant again, which returns an error that matches ErrNotFound.
func implicitGrantLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		const relationKey = "parent"
		resources := "/v2/schema/" + url.PathEscape(s.env.ProjectID) + "/" +
			url.PathEscape(s.env.ID) + "/resources/"
		register := func(what, path string) {
			s.ledger.Register(what, s.api.Remover(resources+path),
				s.api.Absence(resources+path, framework.AbsenceTimeout))
		}
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		doc, folder := s.cfg.prefix.Key("sdk-grants-doc"), s.cfg.prefix.Key("sdk-grants-folder")
		ids := map[string]string{}
		for _, key := range []string{folder, doc} {
			register("resource "+key, url.PathEscape(key))
			resource, err := c.Resources.Create(ctx, models.ResourceCreate{
				Key: key, Name: key, Actions: map[string]models.ActionBlockEditable{"read": {}},
			})
			if err != nil {
				t.Fatalf("setup: Create of the resource %s: %v", key, err)
			}
			ids[key] = resource.ID
		}
		register("relation "+relationKey+" of "+doc, url.PathEscape(doc)+"/relations/"+relationKey)
		relation, err := c.ResourceRelations.Create(ctx, doc, models.RelationCreate{
			Key: relationKey, Name: "Parent", SubjectResource: folder,
		})
		if err != nil {
			t.Fatalf("setup: Create of the relation %s of %s: %v", relationKey, doc, err)
		}
		owner, viewer := s.cfg.prefix.Key("sdk-grants-owner"), s.cfg.prefix.Key("sdk-grants-viewer")
		for _, r := range [][2]string{{folder, owner}, {doc, viewer}} {
			register("role "+r[1]+" of "+r[0], url.PathEscape(r[0])+"/roles/"+url.PathEscape(r[1]))
			role, roleErr := c.ResourceRoles.Create(ctx, r[0], models.ResourceRoleCreate{
				Key: r[1], Name: r[1], Permissions: []string{"read"},
			})
			if roleErr != nil {
				t.Fatalf("setup: Create of the role %s of %s: %v", r[1], r[0], roleErr)
			}
			ids[r[1]] = role.ID
		}
		rule, err := c.ImplicitGrants.Create(ctx, doc, viewer, models.DerivedRoleRuleCreate{
			Role: owner, OnResource: folder, LinkedByRelation: relationKey,
		})
		if err != nil {
			t.Fatalf("Create of the implicit grant of %s of %s: %v", viewer, doc, err)
		}
		want := models.DerivedRoleRuleRead{
			RoleID: ids[owner], ResourceID: ids[folder], RelationID: relation.ID, Role: owner,
			OnResource: folder, LinkedByRelation: relationKey,
		}
		got := *rule
		got.When = nil
		if got != want {
			t.Errorf("Create of the implicit grant returned the rule %+v, without its When, want "+
				"%+v", got, want)
		}
		grants := func(ctx context.Context) (*models.DerivedRoleBlockRead, error) {
			role, getErr := c.ResourceRoles.Get(ctx, doc, viewer)
			if getErr != nil || role.GrantedTo == nil {
				return &models.DerivedRoleBlockRead{}, getErr
			}
			return role.GrantedTo, nil
		}
		isRule := func(r models.DerivedRoleRuleRead) bool {
			return r.Role == owner && r.OnResource == folder && r.LinkedByRelation == relationKey
		}
		block, getErr := grants(ctx)
		if getErr != nil || !slices.ContainsFunc(block.UsersWithRole, isRule) {
			t.Errorf("Get of the role %s of %s after the create returned the implicit grants "+
				"%+v and %v, want the rule of %s of %s", viewer, doc, block, getErr, owner, folder)
		}
		for _, set := range []bool{true, false} {
			settings, updateErr := c.ImplicitGrants.UpdateConditions(ctx, doc, viewer,
				models.DerivationSettings{NoDirectRolesOnObject: set})
			if updateErr != nil {
				t.Fatalf("UpdateConditions of the implicit grants of %s of %s to %t: %v", viewer,
					doc, set, updateErr)
			}
			if settings.NoDirectRolesOnObject != set {
				t.Errorf("UpdateConditions to %t returned %+v", set, settings)
			}
			block, getErr = grants(ctx)
			if stored := block.When != nil && block.When.NoDirectRolesOnObject; getErr != nil ||
				stored != set {
				t.Errorf("Get of the role %s of %s after UpdateConditions to %t returned the "+
					"conditions %+v and %v", viewer, doc, set, block.When, getErr)
			}
		}
		deleted := models.DerivedRoleRuleDelete{
			Role: owner, OnResource: folder, LinkedByRelation: relationKey,
		}
		err = c.ImplicitGrants.Delete(ctx, doc, viewer, deleted)
		if err != nil {
			t.Fatalf("Delete of the implicit grant of %s of %s: %v", viewer, doc, err)
		}
		gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
			func(ctx context.Context) (bool, error) {
				read, readErr := grants(ctx)
				return readErr == nil && !slices.ContainsFunc(read.UsersWithRole, isRule), readErr
			})
		if gone != nil {
			t.Fatalf("Get of the role %s of %s after the delete of its implicit grant: %v", viewer,
				doc, gone)
		}
		err = c.ImplicitGrants.Delete(ctx, doc, viewer, deleted)
		if !errors.Is(err, permit.ErrNotFound) {
			t.Errorf("Delete of the implicit grant of %s of %s that is gone returned %v, want an "+
				"error that matches ErrNotFound", viewer, doc, err)
		}
	}
}

// listResourceRoles checks that List, and All with one role on each page, list the roles base
// and child of resource, and no other role.
func listResourceRoles(ctx context.Context, t *testing.T, c *permit.Client,
	resource, base, child string,
) {
	t.Helper()
	page, err := c.ResourceRoles.List(ctx, resource, nil)
	if err != nil {
		t.Fatalf("List of the roles of %s: %v", resource, err)
	}
	if keys := resourceRoleKeys(page.Data); !slices.Equal(keys, []string{base, child}) {
		t.Errorf("List of the roles of %s returned %q, want %s and %s", resource, keys, base,
			child)
	}
	var all []models.ResourceRoleRead
	for role, err := range c.ResourceRoles.All(ctx, resource, &permit.ListOptions{PerPage: 1}) {
		if err != nil {
			t.Fatalf("All of the roles of %s: %v", resource, err)
		}
		all = append(all, role)
	}
	if keys := resourceRoleKeys(all); !slices.Equal(keys, []string{base, child}) {
		t.Errorf("All of the roles of %s, one on a page, listed %q, want %s and %s", resource,
			keys, base, child)
	}
}

// resourceRoleRelatives checks that Ancestors of the role child of resource, which extends base,
// lists base alone, and that Descendants of base lists child alone.
func resourceRoleRelatives(ctx context.Context, t *testing.T, c *permit.Client,
	resource, base, child string,
) {
	t.Helper()
	ancestors, err := c.ResourceRoles.Ancestors(ctx, resource, child)
	if err != nil {
		t.Fatalf("Ancestors of the role %s of %s: %v", child, resource, err)
	}
	if keys := resourceRoleKeys(ancestors); !slices.Equal(keys, []string{base}) {
		t.Errorf("Ancestors of the role %s returned %q, want %s alone", child, keys, base)
	}
	descendants, err := c.ResourceRoles.Descendants(ctx, resource, base)
	if err != nil {
		t.Fatalf("Descendants of the role %s of %s: %v", base, resource, err)
	}
	if keys := resourceRoleKeys(descendants); !slices.Equal(keys, []string{child}) {
		t.Errorf("Descendants of the role %s returned %q, want %s alone", base, keys, child)
	}
}

// changeResourceRole checks that Update of the role child of resource, which extends base, with
// the permissions write and Extends nil, which the request leaves out, sets its permissions to
// write and keeps base in its extends; that Update with an empty slice of Extends makes it extend
// no role; that AssignPermissions of share keeps write; and that RemovePermissions of write keeps
// share alone.
func changeResourceRole(ctx context.Context, t *testing.T, c *permit.Client,
	resource, base, child string,
) {
	t.Helper()
	kept, err := c.ResourceRoles.Update(ctx, resource, child, models.ResourceRoleUpdate{
		Permissions: []string{"write"},
	})
	if err != nil {
		t.Fatalf("Update of the role %s of %s without extends: %v", child, resource, err)
	}
	if got := actionsOf(resource, kept.Permissions); !slices.Equal(got, []string{"write"}) ||
		!slices.Equal(kept.Extends, []string{base}) {
		t.Errorf("Update without extends returned the permissions %q and the extends %q, want "+
			"write and %s", kept.Permissions, kept.Extends, base)
	}
	updated, err := c.ResourceRoles.Update(ctx, resource, child, models.ResourceRoleUpdate{
		Permissions: []string{"write"}, Extends: []string{},
	})
	if err != nil {
		t.Fatalf("Update of the role %s of %s: %v", child, resource, err)
	}
	if got := actionsOf(resource, updated.Permissions); !slices.Equal(got, []string{"write"}) ||
		len(updated.Extends) != 0 {
		t.Errorf("Update returned the permissions %q and the extends %q, want write and none",
			updated.Permissions, updated.Extends)
	}
	assigned, err := c.ResourceRoles.AssignPermissions(ctx, resource, child,
		[]string{resource + ":share"})
	if err != nil {
		t.Fatalf("AssignPermissions of share to the role %s of %s: %v", child, resource, err)
	}
	if got := actionsOf(resource, assigned.Permissions); !slices.Equal(got,
		[]string{"share", "write"}) {
		t.Errorf("AssignPermissions returned the permissions %q, want share and write",
			assigned.Permissions)
	}
	removed, err := c.ResourceRoles.RemovePermissions(ctx, resource, child,
		[]string{resource + ":write"})
	if err != nil {
		t.Fatalf("RemovePermissions of write from the role %s of %s: %v", child, resource, err)
	}
	if got := actionsOf(resource, removed.Permissions); !slices.Equal(got, []string{"share"}) {
		t.Errorf("RemovePermissions returned the permissions %q, want share alone",
			removed.Permissions)
	}
}

// actionsOf returns the keys of the actions of permissions, the permissions of a role of
// resource, each the key of an action or <resource>:<action>, sorted.
func actionsOf(resource string, permissions []string) []string {
	actions := []string{}
	for _, p := range permissions {
		actions = append(actions, strings.TrimPrefix(p, resource+":"))
	}
	slices.Sort(actions)
	return actions
}

// resourceRoleKeys returns the keys of roles, sorted.
func resourceRoleKeys(roles []models.ResourceRoleRead) []string {
	keys := []string{}
	for _, r := range roles {
		keys = append(keys, r.Key)
	}
	slices.Sort(keys)
	return keys
}
