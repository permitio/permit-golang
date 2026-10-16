//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework/seed"
)

// bulkRegister registers the teardown of the object ident of the collection of the facts API
// collection, such as tenants, with the suite's ledger, before the request that creates it: a
// deletion of the object by its identifier, its key or, for a resource instance,
// <resource>:<key>, and the check that it is gone.
func (s *suite) bulkRegister(collection, ident string) {
	self := "/v2/facts/" + url.PathEscape(s.env.ProjectID) + "/" + url.PathEscape(s.env.ID) +
		"/" + collection + "/" + url.PathEscape(ident)
	s.ledger.Register(collection+" "+ident, s.api.Remover(self),
		s.api.Absence(self, framework.AbsenceTimeout))
}

// bulkLifecycle checks the bulk calls of the users, the tenants and the resource instances with
// the environment-level API key of the scratch environment, and no WithScope, so that the client
// acts on the environment of its key, on two tenants, two users and two instances of its own,
// whose keys start with the run's prefix and which it registers with the ledger, by their keys,
// before it creates them, in a resource of its seed. It creates the tenants and then the users
// with one BulkCreate each; replaces the users with one BulkReplace that gives each a first name;
// creates the instances, one in each tenant, with one BulkReplace, and replaces them with another
// that gives each an attribute; and deletes the instances, the users and the tenants with one
// BulkDelete each. After each call, it waits until the lists of the tenants, the users or the
// instances hold its objects as the call left them, or none of them after a delete.
func bulkLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		resource := s.cfg.prefix.Key("sdk-bulk-resource")
		s.seed(t, world{resources: []seed.Resource{{Key: resource, Actions: []string{"read"}}}})
		tenants := []string{
			s.cfg.prefix.Key("sdk-bulk-tenant-1"), s.cfg.prefix.Key("sdk-bulk-tenant-2"),
		}
		users := []string{s.cfg.prefix.Key("sdk-bulk-user-1"), s.cfg.prefix.Key("sdk-bulk-user-2")}
		docs := []string{s.cfg.prefix.Key("sdk-bulk-doc-1"), s.cfg.prefix.Key("sdk-bulk-doc-2")}
		instances := []string{resource + ":" + docs[0], resource + ":" + docs[1]}
		for _, key := range tenants {
			s.bulkRegister("tenants", key)
		}
		for _, key := range users {
			s.registerUser(key, func() string { return "" })
		}
		for _, name := range instances {
			s.bulkRegister("resource_instances", name)
		}
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)

		listTenants := bulkTenantLister(c, tenants)
		if err := c.Tenants.BulkCreate(ctx, []models.TenantCreate{
			{Key: tenants[0], Name: tenants[0]}, {Key: tenants[1], Name: tenants[1]},
		}); err != nil {
			t.Fatalf("BulkCreate of the tenants %q: %v", tenants, err)
		}
		s.bulkAwait(ctx, t, "the tenants after BulkCreate", listTenants, tenants)

		listUsers := bulkUserLister(c, users)
		if err := c.Users.BulkCreate(ctx, []models.UserCreate{
			{Key: users[0]}, {Key: users[1]},
		}); err != nil {
			t.Fatalf("BulkCreate of the users %q: %v", users, err)
		}
		s.bulkAwait(ctx, t, "the users after BulkCreate", listUsers,
			[]string{users[0] + " ", users[1] + " "})
		if err := c.Users.BulkReplace(ctx, []models.UserCreate{
			{Key: users[0], FirstName: "Ada"}, {Key: users[1], FirstName: "Grace"},
		}); err != nil {
			t.Fatalf("BulkReplace of the users %q: %v", users, err)
		}
		s.bulkAwait(ctx, t, "the users after BulkReplace", listUsers,
			[]string{users[0] + " Ada", users[1] + " Grace"})

		listInstances := bulkInstanceLister(c, resource, docs)
		placed := []models.ResourceInstanceCreate{
			{Key: docs[0], Tenant: tenants[0], Resource: resource},
			{Key: docs[1], Tenant: tenants[1], Resource: resource},
		}
		if err := c.ResourceInstances.BulkReplace(ctx, placed); err != nil {
			t.Fatalf("BulkReplace of the instances %q: %v", instances, err)
		}
		s.bulkAwait(ctx, t, "the instances after the first BulkReplace", listInstances,
			[]string{docs[0] + " " + tenants[0] + " ", docs[1] + " " + tenants[1] + " "})
		for i := range placed {
			placed[i].Attributes = models.Attributes{"stage": "replaced"}
		}
		if err := c.ResourceInstances.BulkReplace(ctx, placed); err != nil {
			t.Fatalf("the second BulkReplace of the instances %q: %v", instances, err)
		}
		s.bulkAwait(ctx, t, "the instances after the second BulkReplace", listInstances,
			[]string{
				docs[0] + " " + tenants[0] + " replaced", docs[1] + " " + tenants[1] + " replaced",
			})

		if err := c.ResourceInstances.BulkDelete(ctx, instances); err != nil {
			t.Fatalf("BulkDelete of the instances %q: %v", instances, err)
		}
		s.bulkAwait(ctx, t, "the instances after BulkDelete", listInstances, nil)
		if err := c.Users.BulkDelete(ctx, users); err != nil {
			t.Fatalf("BulkDelete of the users %q: %v", users, err)
		}
		s.bulkAwait(ctx, t, "the users after BulkDelete", listUsers, nil)
		if err := c.Tenants.BulkDelete(ctx, tenants); err != nil {
			t.Fatalf("BulkDelete of the tenants %q: %v", tenants, err)
		}
		s.bulkAwait(ctx, t, "the tenants after BulkDelete", listTenants, nil)
	}
}

// bulkLister returns what a list holds of the objects of a scenario, each as a text, in the
// order of the list.
type bulkLister func(ctx context.Context) ([]string, error)

// bulkTenantLister returns the lister of the tenants of keys that All of the tenants yields, each
// as its key.
func bulkTenantLister(c *permit.Client, keys []string) bulkLister {
	return func(ctx context.Context) ([]string, error) {
		var got []string
		for tenant, err := range c.Tenants.All(ctx, nil) {
			if err != nil {
				return nil, err
			}
			if slices.Contains(keys, tenant.Key) {
				got = append(got, tenant.Key)
			}
		}
		return got, nil
	}
}

// bulkUserLister returns the lister of the users of keys that All of the users yields, each as
// its key and its first name.
func bulkUserLister(c *permit.Client, keys []string) bulkLister {
	return func(ctx context.Context) ([]string, error) {
		var got []string
		for user, err := range c.Users.All(ctx, nil) {
			if err != nil {
				return nil, err
			}
			if slices.Contains(keys, user.Key) {
				got = append(got, user.Key+" "+user.FirstName)
			}
		}
		return got, nil
	}
}

// bulkInstanceLister returns the lister of the instances of the resource whose keys are among
// keys that All of the instances of the resource yields, each as its key, its tenant and its
// attribute stage, empty when it has none.
func bulkInstanceLister(c *permit.Client, resource string, keys []string) bulkLister {
	return func(ctx context.Context) ([]string, error) {
		var got []string
		opts := &permit.ResourceInstancesListOptions{Resource: resource}
		for instance, err := range c.ResourceInstances.All(ctx, opts) {
			if err != nil {
				return nil, err
			}
			if instance.Resource == resource && slices.Contains(keys, instance.Key) {
				stage := ""
				if v, ok := instance.Attributes["stage"]; ok {
					stage = fmt.Sprint(v)
				}
				got = append(got, instance.Key+" "+instance.Tenant+" "+stage)
			}
		}
		return got, nil
	}
}

// bulkAwait polls list until it holds want, sorted, for at most framework.AbsenceTimeout, and
// fails t with what, the state that it waits for, when it does not.
func (s *suite) bulkAwait(ctx context.Context, t *testing.T, what string, list bulkLister,
	want []string,
) {
	t.Helper()
	var got []string
	err := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
		func(ctx context.Context) (bool, error) {
			listed, err := list(ctx)
			if err != nil {
				return false, err
			}
			got = slices.Sorted(slices.Values(listed))
			return slices.Equal(got, want), nil
		})
	if err != nil {
		t.Fatalf("%s: the lists hold %q, want %q: %v", what, got, want, err)
	}
}
