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
