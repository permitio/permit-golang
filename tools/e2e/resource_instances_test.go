//go:build e2e

package e2e

import (
	"cmp"
	"context"
	"errors"
	"net/url"
	"slices"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework/seed"
)

// resourceInstanceLifecycle checks the calls of the resource instances with the
// environment-level API key of the scratch environment, and no WithScope, so that the client acts
// on the environment of its key, on an instance of its own in a resource and a tenant of its
// seed, whose keys start with the run's prefix. It registers the instance with the ledger before
// it creates it, creates it with an attribute, gets it as <resource>:<key> and by its ID,
// replaces its attributes and then removes them with {}, finds it in a page of List that filters
// by its resource, its tenant and a search for its key, and with All, and in a page of
// ListDetailed and with AllDetailed, each with a list of relationships, deletes it, waits until a
// get of its ID matches ErrNotFound, and deletes it again, which returns nil. The lists never
// hold an instance of its seed of the same resource in another tenant, whose key holds its key,
// so that only the filter of its tenant leaves that one out. The ledger removes and checks the
// instance by its ID once Create returned one, and as <resource>:<key> only when Create failed.
func resourceInstanceLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		resource := s.cfg.prefix.Key("sdk-instance-resource")
		tenant := s.cfg.prefix.Key("sdk-instance-tenant")
		key := s.cfg.prefix.Key("sdk-instance")
		other := seed.Instance{
			Resource: resource, Key: s.cfg.prefix.Key("sdk-instance-other"),
			Tenant: s.cfg.prefix.Key("sdk-instance-other-tenant"),
		}
		s.seed(t, world{
			resources: []seed.Resource{{Key: resource, Actions: []string{"read"}}},
			tenants:   []string{tenant, other.Tenant},
			instances: []seed.Instance{other},
		})
		name := resource + ":" + key
		var createdID string
		self := func() string {
			return "/v2/facts/" + url.PathEscape(s.env.ProjectID) + "/" + url.PathEscape(s.env.ID) +
				"/resource_instances/" + url.PathEscape(cmp.Or(createdID, name))
		}
		s.ledger.Register("resource instance "+name,
			func(ctx context.Context) error { return s.api.Remover(self())(ctx) },
			func(ctx context.Context) error {
				return s.api.Absence(self(), framework.AbsenceTimeout)(ctx)
			})
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		created, err := c.ResourceInstances.Create(ctx, models.ResourceInstanceCreate{
			Key: key, Tenant: tenant, Resource: resource,
			Attributes: models.Attributes{"run": key},
		})
		if err != nil {
			t.Fatalf("Create of the instance %s: %v", name, err)
		}
		createdID = created.ID
		if created.Key != key || created.Resource != resource || created.Tenant != tenant ||
			created.ID == "" || created.EnvironmentID != s.env.ID ||
			created.Attributes["run"] != key {
			t.Fatalf("Create returned the instance %s:%s (ID %q) of the tenant %q and the "+
				"environment %q with the attributes %v, want %s in %s and %s with the attribute "+
				"run", created.Resource, created.Key, created.ID, created.Tenant,
				created.EnvironmentID, created.Attributes, name, tenant, s.env.ID)
		}
		for _, ref := range []string{name, created.ID} {
			got, getErr := c.ResourceInstances.Get(ctx, ref)
			if getErr != nil {
				t.Fatalf("Get of the instance %s: %v", ref, getErr)
			}
			if got.ID != created.ID || got.Attributes["run"] != key {
				t.Errorf("Get of %s returned the instance %s with the attributes %v, want %s "+
					"with the attribute run", ref, got.ID, got.Attributes, created.ID)
			}
		}
		updateInstanceAttributes(ctx, t, c, name, models.Attributes{"stage": "updated"})
		updateInstanceAttributes(ctx, t, c, name, models.Attributes{})
		filters := &permit.ResourceInstancesListOptions{
			ListOptions: permit.ListOptions{PerPage: 100}, Tenant: tenant, Resource: resource,
			Search: []string{key},
		}
		listInstance(ctx, t, c, filters, created.ID, other.Key)
		if err := c.ResourceInstances.Delete(ctx, name); err != nil {
			t.Fatalf("Delete of the instance %s: %v", name, err)
		}
		gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
			func(ctx context.Context) (bool, error) {
				_, err := c.ResourceInstances.Get(ctx, created.ID)
				if errors.Is(err, permit.ErrNotFound) {
					return true, nil
				}
				return false, err
			})
		if gone != nil {
			t.Fatalf("Get of the deleted instance %s: %v", created.ID, gone)
		}
		if err := c.ResourceInstances.Delete(ctx, created.ID); err != nil {
			t.Errorf("Delete of the instance %s that is gone returned %v, want nil", created.ID,
				err)
		}
	}
}

// updateInstanceAttributes replaces the attributes of the instance name with attributes, and
// checks that Update returns the instance with them, an empty map for {}.
func updateInstanceAttributes(ctx context.Context, t *testing.T, c *permit.Client,
	name string, attributes models.Attributes,
) {
	t.Helper()
	updated, err := c.ResourceInstances.Update(ctx, name,
		models.ResourceInstanceUpdate{Attributes: models.Some(attributes)})
	if err != nil {
		t.Fatalf("Update of the attributes of the instance %s: %v", name, err)
	}
	if len(updated.Attributes) != len(attributes) ||
		updated.Attributes["stage"] != attributes["stage"] {
		t.Errorf("Update of %s to the attributes %v returned the attributes %v", name,
			attributes, updated.Attributes)
	}
}

// listInstance checks that the pages of List and ListDetailed with opts, filters that the
// instance whose ID is id matches, hold the instance, the second with a list of relationships,
// and that All and AllDetailed with opts yield it once each; and that none of them holds the
// instance whose key is excluded, which the filters leave out.
func listInstance(ctx context.Context, t *testing.T, c *permit.Client,
	opts *permit.ResourceInstancesListOptions, id, excluded string,
) {
	t.Helper()
	page, err := c.ResourceInstances.List(ctx, opts)
	if err != nil || !slices.ContainsFunc(page.Data, func(i models.ResourceInstanceRead) bool {
		return i.ID == id
	}) || slices.ContainsFunc(page.Data, func(i models.ResourceInstanceRead) bool {
		return i.Key == excluded
	}) {
		t.Errorf("List with the filters %+v returned %+v, %v; want a page with the instance %s "+
			"and without %s", opts, page, err, id, excluded)
	}
	detailed, err := c.ResourceInstances.ListDetailed(ctx, opts)
	if err != nil || !slices.ContainsFunc(detailed.Data,
		func(i models.ResourceInstanceDetailedRead) bool {
			return i.ID == id && i.Relationships != nil
		}) || slices.ContainsFunc(detailed.Data, func(i models.ResourceInstanceDetailedRead) bool {
		return i.Key == excluded
	}) {
		t.Errorf("ListDetailed with the filters %+v returned %+v, %v; want a page with the "+
			"instance %s and its relationships, and without %s", opts, detailed, err, id,
			excluded)
	}
	found, left := 0, 0
	for instance, allErr := range c.ResourceInstances.All(ctx, opts) {
		if allErr != nil {
			t.Fatalf("All of the instances: %v", allErr)
		}
		if instance.ID == id {
			found++
		}
		if instance.Key == excluded {
			left++
		}
	}
	for instance, allErr := range c.ResourceInstances.AllDetailed(ctx, opts) {
		if allErr != nil {
			t.Fatalf("AllDetailed of the instances: %v", allErr)
		}
		if instance.ID == id && instance.Relationships != nil {
			found++
		}
		if instance.Key == excluded {
			left++
		}
	}
	if found != 2 || left != 0 {
		t.Errorf("All and AllDetailed with the filters %+v yielded the instance %s %d times, "+
			"want once each, and %s %d times, want never", opts, id, found, excluded, left)
	}
}
