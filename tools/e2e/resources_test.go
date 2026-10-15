//go:build e2e

package e2e

import (
	"context"
	"errors"
	"maps"
	"net/url"
	"slices"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
)

// resourceLifecycle checks the calls of the resources with the environment-level API key of the
// scratch environment, whose scope the client looks up, on a resource of its own, whose key starts
// with the run's prefix, so that no run uses it again, and which it registers with the ledger
// before it creates it. It creates the resource with two actions and an attribute, gets it by its
// key and by its ID, finds it in the page that a search for its key lists and with All, replaces
// its definition with another set of actions, changes its name and description, deletes it, waits
// until a get matches ErrNotFound, and deletes it again, which returns nil.
func resourceLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		key := s.cfg.prefix.Key("sdk-resource")
		self := "/v2/schema/" + url.PathEscape(s.env.ProjectID) + "/" + url.PathEscape(s.env.ID) +
			"/resources/" + url.PathEscape(key)
		s.ledger.Register("resource "+key, s.api.Remover(self),
			s.api.Absence(self, framework.AbsenceTimeout))
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		created, err := c.Resources.Create(ctx, models.ResourceCreate{
			Key: key, Name: key, Description: "A resource of the Go SDK's tests",
			Actions: map[string]models.ActionBlockEditable{"read": {}, "write": {Name: "Write"}},
			Attributes: map[string]models.AttributeBlockEditable{
				"owner": {Type: models.AttributeTypeString},
			},
		})
		if err != nil {
			t.Fatalf("Create of the resource %s: %v", key, err)
		}
		if created.Key != key || created.ID == "" || created.EnvironmentID != s.env.ID ||
			created.ProjectID != s.env.ProjectID {
			t.Fatalf("Create returned the resource %q (ID %q) of the project %q and the "+
				"environment %q, want %s in %s", created.Key, created.ID, created.ProjectID,
				created.EnvironmentID, key, s.env)
		}
		checkActions(t, "Create", created, "read", "write")
		for _, ref := range []string{key, created.ID} {
			got, getErr := c.Resources.Get(ctx, ref)
			if getErr != nil {
				t.Fatalf("Get of the resource %s: %v", ref, getErr)
			}
			if got.ID != created.ID || got.Key != key {
				t.Errorf("Get of %s returned the resource %s (ID %s), want %s (ID %s)", ref,
					got.Key, got.ID, key, created.ID)
			}
		}
		page, err := c.Resources.List(ctx, &permit.ResourcesListOptions{Search: key})
		if err != nil || !slices.ContainsFunc(page.Data, func(r models.ResourceRead) bool {
			return r.ID == created.ID
		}) {
			t.Errorf("List with the search %s returned %+v, %v; want a page with the resource",
				key, page, err)
		}
		if !listedResource(ctx, t, c, created.ID) {
			t.Errorf("All did not list the resource %s", created.ID)
		}
		replaced, err := c.Resources.Replace(ctx, key, models.ResourceReplace{
			Name: key, Actions: map[string]models.ActionBlockEditable{"read": {}, "archive": {}},
		})
		if err != nil {
			t.Fatalf("Replace of the resource %s: %v", key, err)
		}
		checkActions(t, "Replace", replaced, "archive", "read")
		updated, err := c.Resources.Update(ctx, key, models.ResourceUpdate{
			Name: key + "-renamed", Description: models.Some("Updated"),
		})
		if err != nil {
			t.Fatalf("Update of the resource %s: %v", key, err)
		}
		if updated.Name != key+"-renamed" || updated.Description != "Updated" {
			t.Errorf("Update returned the name %q and the description %q, want %s-renamed and "+
				"Updated", updated.Name, updated.Description, key)
		}
		if err := c.Resources.Delete(ctx, key); err != nil {
			t.Fatalf("Delete of the resource %s: %v", key, err)
		}
		gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
			func(ctx context.Context) (bool, error) {
				_, err := c.Resources.Get(ctx, created.ID)
				if errors.Is(err, permit.ErrNotFound) {
					return true, nil
				}
				return false, err
			})
		if gone != nil {
			t.Fatalf("Get of the deleted resource %s: %v", created.ID, gone)
		}
		if err := c.Resources.Delete(ctx, key); err != nil {
			t.Errorf("Delete of the resource %s that is gone returned %v, want nil", key, err)
		}
	}
}

// checkActions checks that got, the resource that call returned, has the actions whose keys are
// want, in order, and no other.
func checkActions(t *testing.T, call string, got *models.ResourceRead, want ...string) {
	t.Helper()
	if keys := slices.Sorted(maps.Keys(got.Actions)); !slices.Equal(keys, want) {
		t.Errorf("%s returned the resource %s with the actions %q, want %q", call, got.Key, keys,
			want)
	}
}

// resourceActionLifecycle checks the calls of the actions of a resource with the
// environment-level API key of the scratch environment, whose scope the client looks up, on a
// resource of its own, whose key starts with the run's prefix, which it creates with the action
// read and registers with the ledger before it creates it, as it registers each action. It
// creates two actions with the attribute risk, high for approve and low for comment, gets
// approve by its key and by its ID, and lists the actions whose risk is high with List and those
// whose risk is low with All: each filter keeps its one action, and leaves out the other and
// read, which has no attributes. It changes the name and the description of comment, which keeps
// its attributes, deletes approve, waits until a get matches ErrNotFound, and deletes it again,
// which returns nil.
func resourceActionLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		resourceKey := s.cfg.prefix.Key("sdk-actions")
		self := "/v2/schema/" + url.PathEscape(s.env.ProjectID) + "/" + url.PathEscape(s.env.ID) +
			"/resources/" + url.PathEscape(resourceKey)
		s.ledger.Register("resource "+resourceKey, s.api.Remover(self),
			s.api.Absence(self, framework.AbsenceTimeout))
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		if _, err := c.Resources.Create(ctx, models.ResourceCreate{
			Key: resourceKey, Name: resourceKey,
			Actions: map[string]models.ActionBlockEditable{"read": {}},
		}); err != nil {
			t.Fatalf("setup: Create of the resource %s: %v", resourceKey, err)
		}
		risks := map[string]string{"approve": "high", "comment": "low"}
		var approve *models.ResourceActionRead
		for _, key := range []string{"approve", "comment"} {
			path := self + "/actions/" + url.PathEscape(key)
			s.ledger.Register("action "+key+" of "+resourceKey, s.api.Remover(path),
				s.api.Absence(path, framework.AbsenceTimeout))
			action, err := c.ResourceActions.Create(ctx, resourceKey, models.ResourceActionCreate{
				Key: key, Name: key, Description: "An action of the Go SDK's tests",
				Attributes: models.Attributes{"risk": risks[key]},
			})
			if err != nil {
				t.Fatalf("Create of the action %s of %s: %v", key, resourceKey, err)
			}
			if action.Key != key || action.ID == "" || action.Attributes["risk"] != risks[key] ||
				action.EnvironmentID != s.env.ID {
				t.Fatalf("Create returned the action %q (ID %q) of the environment %q with the "+
					"attributes %v, want %s in %s with the risk %s", action.Key, action.ID,
					action.EnvironmentID, action.Attributes, key, s.env, risks[key])
			}
			if key == "approve" {
				approve = action
			}
		}
		for _, ref := range []string{"approve", approve.ID} {
			got, err := c.ResourceActions.Get(ctx, resourceKey, ref)
			if err != nil {
				t.Fatalf("Get of the action %s of %s: %v", ref, resourceKey, err)
			}
			if got.ID != approve.ID || got.Key != "approve" {
				t.Errorf("Get of %s returned the action %s (ID %s), want approve (ID %s)", ref,
					got.Key, got.ID, approve.ID)
			}
		}
		high, err := c.ResourceActions.List(ctx, resourceKey, &permit.ResourceActionsListOptions{
			Attributes: []permit.AttributeFilter{permit.AttributeEquals("risk", "high")},
		})
		if err != nil {
			t.Fatalf("List of the actions of %s whose risk is high: %v", resourceKey, err)
		}
		if keys := actionKeys(high.Data); !slices.Equal(keys, []string{"approve"}) {
			t.Errorf("List of the actions whose risk is high returned %q, want approve", keys)
		}
		var low []models.ResourceActionRead
		for action, err := range c.ResourceActions.All(ctx, resourceKey,
			&permit.ResourceActionsListOptions{
				Attributes: []permit.AttributeFilter{permit.AttributeEquals("risk", "low")},
			}) {
			if err != nil {
				t.Fatalf("All of the actions of %s whose risk is low: %v", resourceKey, err)
			}
			low = append(low, action)
		}
		if keys := actionKeys(low); !slices.Equal(keys, []string{"comment"}) {
			t.Errorf("All of the actions whose risk is low listed %q, want comment", keys)
		}
		change := models.ResourceActionUpdate{
			Name: "Comment renamed", Description: models.Some("Updated"),
		}
		updated, err := c.ResourceActions.Update(ctx, resourceKey, "comment", change)
		if err != nil {
			t.Fatalf("Update of the action comment of %s: %v", resourceKey, err)
		}
		if updated.Name != "Comment renamed" || updated.Description != "Updated" ||
			updated.Attributes["risk"] != "low" {
			t.Errorf("Update returned the name %q, the description %q and the attributes %v, "+
				"want Comment renamed, Updated and the risk low", updated.Name,
				updated.Description, updated.Attributes)
		}
		if err := c.ResourceActions.Delete(ctx, resourceKey, "approve"); err != nil {
			t.Fatalf("Delete of the action approve of %s: %v", resourceKey, err)
		}
		gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
			func(ctx context.Context) (bool, error) {
				_, err := c.ResourceActions.Get(ctx, resourceKey, approve.ID)
				if errors.Is(err, permit.ErrNotFound) {
					return true, nil
				}
				return false, err
			})
		if gone != nil {
			t.Fatalf("Get of the deleted action %s: %v", approve.ID, gone)
		}
		if err := c.ResourceActions.Delete(ctx, resourceKey, "approve"); err != nil {
			t.Errorf("Delete of the action approve that is gone returned %v, want nil", err)
		}
	}
}

// actionKeys returns the keys of actions, in order.
func actionKeys(actions []models.ResourceActionRead) []string {
	keys := []string{}
	for _, a := range actions {
		keys = append(keys, a.Key)
	}
	return keys
}

// listedResource reports whether All lists the resource whose ID is id in the environment, with
// pages of the default size, 100 resources, and stops at the resource. When a loop over every
// page misses it, it reads them all once more before it reports false, as the resources of other
// scenarios can move it from one page to the next.
func listedResource(ctx context.Context, t *testing.T, c *permit.Client, id string) bool {
	t.Helper()
	for range 2 {
		for resource, err := range c.Resources.All(ctx, nil) {
			if err != nil {
				t.Errorf("All: %v", err)
				return false
			}
			if resource.ID == id {
				return true
			}
		}
	}
	return false
}

// resourceActionGroupLifecycle checks the calls of the action groups with the environment-level
// API key of the scratch environment, on a resource of its own, whose key starts with the run's
// prefix, and on its action group editing, each of which it registers with the ledger before it
// creates it. The resource has the actions read and write, and editing, an action whose key is
// the group's key. The scenario creates the group with read and write, gets it by its key and by
// its ID, finds it in the page that List returns and with All, changes its name and its actions,
// and deletes it. Then it checks that the resource still has its three actions, as a delete sent
// to the route of the action editing would have deleted that action and left the group, waits
// until a get of the group's ID matches ErrNotFound, and deletes the group again, which returns
// nil.
func resourceActionGroupLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		const group = "editing"
		key := s.cfg.prefix.Key("sdk-action-groups")
		self := "/v2/schema/" + url.PathEscape(s.env.ProjectID) + "/" + url.PathEscape(s.env.ID) +
			"/resources/" + url.PathEscape(key)
		s.ledger.Register("resource "+key, s.api.Remover(self),
			s.api.Absence(self, framework.AbsenceTimeout))
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		resource, err := c.Resources.Create(ctx, models.ResourceCreate{
			Key: key, Name: key,
			Actions: map[string]models.ActionBlockEditable{"read": {}, "write": {}, group: {}},
		})
		if err != nil {
			t.Fatalf("setup: Create of the resource %s: %v", key, err)
		}
		groupSelf := self + "/action_groups/" + group
		s.ledger.Register("action group "+group+" of "+key, s.api.Remover(groupSelf),
			s.api.Absence(groupSelf, framework.AbsenceTimeout))
		created, err := c.ResourceActionGroups.Create(ctx, key, models.ResourceActionGroupCreate{
			Key: group, Name: "Editing", Description: "An action group of the Go SDK's tests",
			Attributes: models.Attributes{"run": key}, Actions: []string{"read", "write"},
		})
		if err != nil {
			t.Fatalf("Create of the action group %s of %s: %v", group, key, err)
		}
		if created.Key != group || created.ID == "" || created.EnvironmentID != s.env.ID ||
			created.Attributes["run"] != key {
			t.Fatalf("Create returned the action group %q (ID %q) of the environment %q with the "+
				"attributes %v, want %s in %s with the attribute run", created.Key, created.ID,
				created.EnvironmentID, created.Attributes, group, s.env.ID)
		}
		checkGroupActions(t, "Create", created, "read", "write")
		for _, ref := range [][2]string{{key, group}, {resource.ID, created.ID}} {
			got, getErr := c.ResourceActionGroups.Get(ctx, ref[0], ref[1])
			if getErr != nil {
				t.Fatalf("Get of the action group %s of %s: %v", ref[1], ref[0], getErr)
			}
			if got.ID != created.ID || got.Key != group {
				t.Errorf("Get of %s of %s returned the action group %s (ID %s), want %s (ID %s)",
					ref[1], ref[0], got.Key, got.ID, group, created.ID)
			}
		}
		isCreated := func(g models.ResourceActionGroupRead) bool { return g.ID == created.ID }
		page, err := c.ResourceActionGroups.List(ctx, key, nil)
		if err != nil || !slices.ContainsFunc(page.Data, isCreated) {
			t.Errorf("List of the action groups of %s returned %+v, %v; want a page with %s", key,
				page, err, group)
		}
		var all []string
		for g, allErr := range c.ResourceActionGroups.All(ctx, key, nil) {
			if allErr != nil {
				t.Fatalf("All of the action groups of %s: %v", key, allErr)
			}
			all = append(all, g.ID)
		}
		if !slices.Contains(all, created.ID) {
			t.Errorf("All of the action groups of %s yielded %q, want %s among them", key, all,
				created.ID)
		}
		updated, err := c.ResourceActionGroups.Update(ctx, key, group,
			models.ResourceActionGroupUpdate{
				Name: "Renamed editing", Actions: models.Some([]string{"read"}),
			})
		if err != nil {
			t.Fatalf("Update of the action group %s of %s: %v", group, key, err)
		}
		if updated.Name != "Renamed editing" {
			t.Errorf("Update returned the name %q, want Renamed editing", updated.Name)
		}
		checkGroupActions(t, "Update", updated, "read")
		if deleteErr := c.ResourceActionGroups.Delete(ctx, key, group); deleteErr != nil {
			t.Fatalf("Delete of the action group %s of %s: %v", group, key, deleteErr)
		}
		after, err := c.Resources.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get of the resource %s after the delete of its action group: %v", key, err)
		}
		checkActions(t, "Get after the delete of the action group", after, group, "read", "write")
		gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
			func(ctx context.Context) (bool, error) {
				_, getErr := c.ResourceActionGroups.Get(ctx, key, created.ID)
				if errors.Is(getErr, permit.ErrNotFound) {
					return true, nil
				}
				return false, getErr
			})
		if gone != nil {
			t.Fatalf("Get of the deleted action group %s of %s: %v", created.ID, key, gone)
		}
		if err := c.ResourceActionGroups.Delete(ctx, key, group); err != nil {
			t.Errorf("Delete of the action group %s of %s that is gone returned %v, want nil",
				group, key, err)
		}
	}
}

// checkGroupActions checks that got, the action group that call returned, holds the actions
// whose keys are want, in any order, and no other.
func checkGroupActions(t *testing.T, call string, got *models.ResourceActionGroupRead,
	want ...string,
) {
	t.Helper()
	if keys := slices.Sorted(slices.Values(got.Actions)); !slices.Equal(keys, want) {
		t.Errorf("%s returned the action group %s with the actions %q, want %q", call, got.Key,
			keys, want)
	}
}
