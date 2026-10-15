//go:build e2e

package e2e

import (
	"cmp"
	"context"
	"errors"
	"net/url"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
)

// tenantLifecycle checks the calls of the tenants with the environment-level API key of the
// scratch environment, and no WithScope, so that the client acts on the environment of its key,
// on a tenant of its own, whose key starts with the run's prefix and which it registers with the
// ledger before it creates it. It creates the tenant with a description and attributes, gets it
// by its key and by its ID, lists it with an attribute filter that only it matches, as a page
// and with All, replaces its attributes, sets them to {} and then to null, deletes it, waits
// until a get of its ID matches ErrNotFound, and deletes it again, which returns nil. The ledger
// removes and checks the tenant by its ID once Create returned one, and by its key only when
// Create failed.
func tenantLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		key := s.cfg.prefix.Key("sdk-tenant")
		tenants := "/v2/facts/" + url.PathEscape(s.env.ProjectID) + "/" +
			url.PathEscape(s.env.ID) + "/tenants/"
		var createdID string
		self := func() string { return tenants + url.PathEscape(cmp.Or(createdID, key)) }
		s.ledger.Register("tenant "+key,
			func(ctx context.Context) error { return s.api.Remover(self())(ctx) },
			func(ctx context.Context) error {
				return s.api.Absence(self(), framework.AbsenceTimeout)(ctx)
			})
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		created, err := c.Tenants.Create(ctx, models.TenantCreate{
			Key: key, Name: key, Description: "A tenant of the Go SDK's tests",
			Attributes: models.Attributes{"run": key, "tier": "gold"},
		})
		if err != nil {
			t.Fatalf("Create of the tenant %s: %v", key, err)
		}
		createdID = created.ID
		if created.Key != key || created.ID == "" || created.EnvironmentID != s.env.ID ||
			created.Attributes["run"] != key {
			t.Fatalf("Create returned the tenant %q (ID %q) of the environment %q with the "+
				"attributes %v, want %s in %s with the attribute run", created.Key, created.ID,
				created.EnvironmentID, created.Attributes, key, s.env.ID)
		}
		for _, ref := range []string{key, created.ID} {
			got, getErr := c.Tenants.Get(ctx, ref)
			if getErr != nil {
				t.Fatalf("Get of the tenant %s: %v", ref, getErr)
			}
			if got.ID != created.ID || got.Attributes["tier"] != "gold" {
				t.Errorf("Get of %s returned the tenant %s with the attributes %v, want %s with "+
					"tier gold", ref, got.ID, got.Attributes, created.ID)
			}
		}
		filter := &permit.TenantsListOptions{
			ListOptions: permit.ListOptions{PerPage: 10},
			Attributes:  []permit.AttributeFilter{permit.AttributeEquals("run", key)},
		}
		page, err := c.Tenants.List(ctx, filter)
		if err != nil || len(page.Data) != 1 || page.Data[0].ID != created.ID {
			t.Errorf("List with the filter run=%s returned %+v, %v; want the tenant alone", key,
				page, err)
		}
		var all []string
		for tenant, allErr := range c.Tenants.All(ctx, filter) {
			if allErr != nil {
				t.Fatalf("All with the filter run=%s: %v", key, allErr)
			}
			all = append(all, tenant.ID)
		}
		if len(all) != 1 || all[0] != created.ID {
			t.Errorf("All with the filter run=%s yielded %q, want %s alone", key, all, created.ID)
		}
		// updateAttributes sets the attributes of the tenant to attributes with Update, and
		// checks that the tenant that it returns has want attributes: -1 for nil, as after null,
		// and else that many, an empty map that is not nil for 0.
		updateAttributes := func(attributes models.Optional[models.Attributes], want int) {
			t.Helper()
			updated, err := c.Tenants.Update(ctx, key, models.TenantUpdate{Attributes: attributes})
			if err != nil {
				t.Fatalf("Update of the attributes of the tenant %s: %v", key, err)
			}
			got := len(updated.Attributes)
			if updated.Attributes == nil {
				got = -1
			}
			if got != want {
				t.Errorf("Update returned the attributes %#v, want %d of them (-1 for nil)",
					updated.Attributes, want)
			}
		}
		updateAttributes(models.Some(models.Attributes{"run": key}), 1)
		updateAttributes(models.Some(models.Attributes{}), 0)
		updateAttributes(models.Null[models.Attributes](), -1)
		if err := c.Tenants.Delete(ctx, key); err != nil {
			t.Fatalf("Delete of the tenant %s: %v", key, err)
		}
		gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
			func(ctx context.Context) (bool, error) {
				_, err := c.Tenants.Get(ctx, created.ID)
				if errors.Is(err, permit.ErrNotFound) {
					return true, nil
				}
				return false, err
			})
		if gone != nil {
			t.Fatalf("Get of the deleted tenant %s: %v", created.ID, gone)
		}
		if err := c.Tenants.Delete(ctx, created.ID); err != nil {
			t.Errorf("Delete of the tenant %s that is gone returned %v, want nil", created.ID,
				err)
		}
	}
}
