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
