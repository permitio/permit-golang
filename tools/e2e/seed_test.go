//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework/seed"
)

// TestSeedBuilder runs every call of the seed builder offline against the fake of the dry run,
// which serves the routes of the control plane's spec and checks each body, the bodies of the
// teardowns too, with the structural walker of tools/drift. It seeds a scratch environment as the
// scenarios will, in the ledger that holds the environment, which checks that each seed object is
// gone before it deletes the environment; the run reconciles the ledger.
func TestSeedBuilder(t *testing.T) {
	api, err := framework.NewAPI(startFake(t).URL(), dryRunProjectKey,
		&http.Client{Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	ledger := newLedger(t.Name())
	t.Cleanup(func() {
		teardownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), teardownTimeout)
		defer cancel()
		if teardownErr := ledger.Teardown(teardownCtx); teardownErr != nil {
			t.Errorf("tearing down the run: %v", teardownErr)
		}
	})
	env, err := framework.CreateEnvironment(ctx, api, ledger, dryRunProject, string(dryRunPrefix))
	if err != nil {
		t.Fatal(err)
	}
	b := seed.New(api, ledger, env)
	for _, call := range []func() error{
		func() error {
			return b.Resource(ctx, seed.Resource{Key: "document", Actions: []string{"read"}})
		},
		func() error { return b.Resource(ctx, seed.Resource{Key: "folder"}) },
		func() error {
			return b.Role(ctx, seed.Role{Key: "admin", Permissions: []string{"document:read"}})
		},
		func() error {
			return b.ResourceRole(ctx, seed.ResourceRole{
				Resource: "document", Key: "viewer", Permissions: []string{"read"},
			})
		},
		func() error {
			return b.Relation(ctx, seed.Relation{
				Resource: "document", Key: "parent", SubjectResource: "folder",
			})
		},
		func() error { return b.Tenant(ctx, "t1") },
		func() error { return b.User(ctx, "u1") },
		func() error {
			return b.Instance(ctx, seed.Instance{Resource: "document", Key: "doc-1", Tenant: "t1"})
		},
		func() error {
			return b.Instance(ctx, seed.Instance{Resource: "folder", Key: "f1", Tenant: "t1"})
		},
		func() error {
			admin := seed.RoleAssignment{User: "u1", Role: "admin", Tenant: "t1"}
			return b.RoleAssignment(ctx, admin)
		},
		func() error {
			return b.RoleAssignment(ctx, seed.RoleAssignment{
				User: "u1", Role: "viewer", Tenant: "t1", ResourceInstance: "document:doc-1",
			})
		},
		func() error {
			return b.Tuple(ctx, seed.Tuple{
				Subject: "folder:f1", Relation: "parent", Object: "document:doc-1", Tenant: "t1",
			})
		},
	} {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	if got := ledger.Counts(); got != (framework.Counts{Registered: 13}) {
		t.Errorf("the ledger holds %v, want the environment and 12 objects", got)
	}
}
