//go:build e2e

package e2e

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework/seed"
)

// factsWait is the wait for sync of the calls of factsWaitForSync.
const factsWait = 10 * time.Second

// factsWriteMethods are the public methods of the writes of the facts that the scenarios of the
// facts through the PDP container and through the cloud PDP cover.
var factsWriteMethods = []string{
	"RoleAssignmentsService.Assign", "RoleAssignmentsService.Unassign",
}

// factsWaitForSync checks the writes of the role assignments that wait for sync, through the PDP
// container with WithFactsViaPDP: once Assign returns, through a copy of WaitForSync of 10
// seconds, the first Check after it allows what the new role grants, without polling, and once
// Unassign returns, through the same copy, the first Check after it denies it. The seed goes to
// the Permit API, which does not wait, so the scenario first polls, for a bounded time, a decision
// that only its seed allows: that of a second user, whose role the seed assigns.
func factsWaitForSync(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		const resource, role, tenant = "facts-sheet", "facts-sheet-reader", "facts-1"
		const user, witness = "facts-user", "facts-witness"
		s.seed(t, world{
			resources: []seed.Resource{{Key: resource, Actions: []string{"read"}}},
			roles:     []seed.Role{{Key: role, Permissions: []string{resource + ":read"}}},
			tenants:   []string{tenant},
			users:     []string{user, witness},
			assignments: []seed.RoleAssignment{
				{User: witness, Role: role, Tenant: tenant},
			},
		})
		c := s.client(t, s.env.APIKey(), permit.WithFactsViaPDP())
		sheet := permit.Resource{Type: resource, Tenant: tenant}
		await(t, s, "Check of "+witness, true, func(ctx context.Context) (bool, error) {
			return c.Check(ctx, permit.User{Key: witness}, "read", sheet)
		})
		ctx := s.ctx(t)
		waiting := c.RoleAssignments.WaitForSync(factsWait)
		a := seed.RoleAssignment{User: user, Role: role, Tenant: tenant}
		if err := seed.New(s.api, s.ledger, s.env).RegisterRoleAssignment(a); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if _, err := waiting.Assign(ctx, models.RoleAssignmentCreate{
			User: user, Role: role, Tenant: tenant,
		}); err != nil {
			t.Fatalf("Assign through the PDP container with a wait of %s: %v", factsWait, err)
		}
		if allowed, err := c.Check(ctx, permit.User{Key: user}, "read", sheet); err != nil ||
			!allowed {
			t.Errorf("the first Check after Assign returned %t, %v; want an allow, without "+
				"polling, as Assign waited for sync", allowed, err)
		}
		if err := waiting.Unassign(ctx, models.RoleAssignmentRemove{
			User: user, Role: role, Tenant: tenant,
		}); err != nil {
			t.Fatalf("Unassign through the PDP container with a wait of %s: %v", factsWait, err)
		}
		if allowed, err := c.Check(ctx, permit.User{Key: user}, "read", sheet); err != nil ||
			allowed {
			t.Errorf("the first Check after Unassign returned %t, %v; want a denial, without "+
				"polling, as Unassign waited for sync", allowed, err)
		}
	}
}

// factsContainerMethods are the public methods that factsThroughTheContainer calls through the PDP
// container.
var factsContainerMethods = []string{
	"TenantsService.Create", "UsersService.Create", "UsersService.Update", "UsersService.Get",
	"UsersService.AssignRole", "UsersService.UnassignRole", "UsersService.Sync",
	"ResourceInstancesService.Create", "ResourceInstancesService.Update",
	"RelationshipTuplesService.Create",
}

// factsThroughTheContainer checks the writes of the facts through the PDP container with
// WithFactsViaPDP, without a wait, each of which the container sends on to the Permit API, on
// objects of its own, which it registers with the ledger before it creates them: their keys start
// with the run's prefix, but for the resource instances, whose resources' keys do. It creates a
// tenant and a user; changes the user's last name with Update; assigns the user a role of its
// seed in the tenant and removes the role again, as assignAndUnassign says, whose gets of the
// user go through the container's route for every path under /facts that its spec does not list;
// syncs a second user, which creates it; creates a folder and a document in the tenant and
// replaces the document's attributes; and creates the relationship tuple of the folder as the
// parent of the document, a relation of its seed. Only the PDP container serves /facts, so only
// a leg that runs it runs the scenario.
func factsThroughTheContainer(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		tenant, role := s.cfg.prefix.Key("facts-pdp-tenant"), s.cfg.prefix.Key("facts-pdp-role")
		user, synced := s.cfg.prefix.Key("facts-pdp-user"), s.cfg.prefix.Key("facts-pdp-synced")
		folder, document := s.cfg.prefix.Key("facts-pdp-folder"), s.cfg.prefix.Key("facts-pdp-doc")
		s.seed(t, world{
			resources: []seed.Resource{
				{Key: folder, Actions: []string{"read"}},
				{Key: document, Actions: []string{"read"}},
			},
			roles: []seed.Role{{Key: role}},
		})
		b := seed.New(s.api, s.ledger, s.env)
		parent := seed.Relation{Resource: document, Key: "parent", SubjectResource: folder}
		if err := b.Relation(t.Context(), parent); err != nil {
			t.Fatalf("setup: %v", err)
		}
		c := s.client(t, s.env.APIKey(), permit.WithFactsViaPDP())
		ctx := s.ctx(t)
		s.registerFact("tenants", tenant)
		created, err := c.Tenants.Create(ctx, models.TenantCreate{Key: tenant, Name: tenant})
		if err != nil || created.Key != tenant {
			t.Fatalf("Create of the tenant %s through the PDP container returned %+v, %v", tenant,
				created, err)
		}
		s.registerUser(user, func() string { return "" })
		made, err := c.Users.Create(ctx, models.UserCreate{Key: user, Email: user + "@example.com"})
		if err != nil || made.Key != user {
			t.Fatalf("Create of the user %s through the PDP container returned %+v, %v", user,
				made, err)
		}
		updated, err := c.Users.Update(ctx, user, models.UserUpdate{
			LastName: models.Some("Lovelace"),
		})
		if err != nil || updated.LastName != "Lovelace" {
			t.Errorf("Update of the user %s through the PDP container returned %+v, %v; want the "+
				"last name Lovelace", user, updated, err)
		}
		assignAndUnassign(ctx, t, c, user, models.UserRoleCreate{Role: role, Tenant: tenant})
		s.registerUser(synced, func() string { return "" })
		result, err := c.Users.Sync(ctx, models.UserCreate{Key: synced})
		if err != nil || result.User.Key != synced {
			t.Errorf("Sync of the new user %s through the PDP container returned %+v, %v", synced,
				result, err)
		}
		factsInstancesAndTuple(ctx, t, s, c, seed.Tuple{
			Subject: folder + ":f1", Relation: parent.Key, Object: document + ":d1", Tenant: tenant,
		})
	}
}

// factsInstancesAndTuple creates the subject and the object of tuple, resource instances
// <resource>:<key> in its tenant, with c, a client of the suite s whose facts go through the PDP
// container, each registered with the ledger before; replaces the attributes of the object; and
// creates tuple, registered with the ledger before.
func factsInstancesAndTuple(ctx context.Context, t *testing.T, s *suite, c *permit.Client,
	tuple seed.Tuple,
) {
	t.Helper()
	for _, name := range []string{tuple.Subject, tuple.Object} {
		resource, key, _ := strings.Cut(name, ":")
		s.registerFact("resource_instances", name)
		got, err := c.ResourceInstances.Create(ctx, models.ResourceInstanceCreate{
			Key: key, Resource: resource, Tenant: tuple.Tenant,
		})
		if err != nil || got.Key != key || got.Resource != resource {
			t.Fatalf("Create of the instance %s through the PDP container returned %+v, %v",
				name, got, err)
		}
	}
	updateInstanceAttributes(ctx, t, c, tuple.Object, models.Attributes{"stage": "via-pdp"})
	seed.New(s.api, s.ledger, s.env).RegisterTuple(tuple)
	got, err := c.RelationshipTuples.Create(ctx, models.RelationshipTupleCreate{
		Subject: tuple.Subject, Relation: tuple.Relation, Object: tuple.Object,
		Tenant: tuple.Tenant,
	})
	if err != nil || got.Subject != tuple.Subject || got.Relation != tuple.Relation ||
		got.Object != tuple.Object {
		t.Errorf("Create of the relationship tuple %+v through the PDP container returned %+v, "+
			"%v", tuple, got, err)
	}
}

// factsOnCloudPDP checks the writes of the role assignments through the cloud PDP with
// WithFactsViaPDP: the cloud PDP does not serve /facts, so Assign and Unassign each return, after
// one request, an error that matches ErrContainerPDPOnly, and not ErrNotFound, and names the route
// under /facts. The role assignment is registered with the ledger first, as every write is.
func factsOnCloudPDP(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		const route = "/facts/role_assignments"
		log := &requestLog{}
		c := s.client(t, s.env.APIKey(), permit.WithFactsViaPDP(),
			permit.WithHTTPClient(&http.Client{Transport: log}))
		ctx := s.ctx(t)
		a := seed.RoleAssignment{
			User: "facts-cloud-user", Role: "facts-cloud-role", Tenant: "facts-cloud-tenant",
		}
		if err := seed.New(s.api, s.ledger, s.env).RegisterRoleAssignment(a); err != nil {
			t.Fatalf("setup: %v", err)
		}
		for name, call := range map[string]func() error{
			"Assign": func() error {
				_, err := c.RoleAssignments.Assign(ctx, models.RoleAssignmentCreate{
					User: a.User, Role: a.Role, Tenant: a.Tenant,
				})
				return err
			},
			"Unassign": func() error {
				return c.RoleAssignments.Unassign(ctx, models.RoleAssignmentRemove{
					User: a.User, Role: a.Role, Tenant: a.Tenant,
				})
			},
		} {
			before := len(log.to(route))
			err := call()
			perr, ok := errors.AsType[*permit.Error](err)
			if !errors.Is(err, permit.ErrContainerPDPOnly) || errors.Is(err, permit.ErrNotFound) ||
				!ok || perr.Route != route || !strings.Contains(err.Error(), route) {
				t.Errorf("%s through the cloud PDP returned %v, want a *permit.Error that "+
					"matches ErrContainerPDPOnly, not ErrNotFound, and names %s", name, err, route)
			}
			if n := len(log.to(route)) - before; n != 1 {
				t.Errorf("%s through the cloud PDP sent %d requests, want one", name, n)
			}
		}
	}
}
