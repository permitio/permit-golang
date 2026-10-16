package permit_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// List the tenants of the environment whose tier is gold, one on a page, and get one by its key.
// Without WithScope, the client acts on the environment of its API key, which it looks up with
// the first call.
func ExampleTenantsService() {
	client, err := permit.New(
		permit.WithAPIKey(exampleAPIKey),
		permit.WithAPIURL(exampleAPIURL),
		permit.WithPDPURL(examplePDPURL),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer client.Close()

	ctx := context.Background()
	page, err := client.Tenants.List(ctx, &permit.TenantsListOptions{
		ListOptions: permit.ListOptions{PerPage: 1},
		Attributes:  []permit.AttributeFilter{permit.AttributeEquals("tier", "gold")},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, tenant := range page.Data {
		fmt.Println(tenant.Key, "-", tenant.Name, "-", tenant.Attributes["tier"])
	}
	fmt.Println("tenants:", *page.TotalCount)

	tenant, err := client.Tenants.Get(ctx, "tenant-a")
	if errors.Is(err, permit.ErrNotFound) {
		fmt.Println("the environment has no tenant with the key tenant-a")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("seats:", tenant.Attributes["seats"])
	// Output:
	// tenant-a - Tenant A - gold
	// tenants: 11
	// seats: 9007199254740993
}

// Create a tenant through the PDP container, which holds its answer until the tenant reaches its
// policy, for at most 10 seconds, and fails the create when the time runs out. Only the create of
// a tenant waits: a delete through the same copy is an error that matches ErrInvalidConfig, and
// sends nothing.
func ExampleTenantsService_WaitForSync() {
	client, err := permit.New(
		permit.WithAPIKey(exampleAPIKey),
		permit.WithAPIURL(exampleAPIURL),
		permit.WithPDPURL(examplePDPURL),
		permit.WithFactsViaPDP(),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer client.Close()

	ctx := context.Background()
	tenants := client.Tenants.WaitForSync(10*time.Second,
		permit.WithSyncPolicy(permit.SyncPolicyFail))
	tenant, err := tenants.Create(ctx, models.TenantCreate{
		Key: "tenant-c", Name: "Tenant C", Attributes: models.Attributes{"tier": "silver"},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(tenant.Key, tenant.ID)

	if err := tenants.Delete(ctx, "tenant-c"); errors.Is(err, permit.ErrInvalidConfig) {
		fmt.Println("a delete does not wait for sync")
	}
	// Output:
	// tenant-c 00000000-0000-4000-8000-000000000023
	// a delete does not wait for sync
}

// List the users of the tenant tenant-a, add the user erin to it without a role, and remove erin
// from it again. AddUser creates the user, so a key that a user of the environment has already is
// an error that matches ErrConflict.
func ExampleTenantsService_AddUser() {
	client, err := permit.New(
		permit.WithAPIKey(exampleAPIKey),
		permit.WithAPIURL(exampleAPIURL),
		permit.WithPDPURL(examplePDPURL),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer client.Close()

	ctx := context.Background()
	for user, err := range client.Tenants.AllUsers(ctx, "tenant-a", nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		i := slices.IndexFunc(user.AssociatedTenants, func(t models.UserInTenant) bool {
			return t.Tenant == "tenant-a"
		})
		if i >= 0 {
			fmt.Println(user.Key, "has the roles", user.AssociatedTenants[i].Roles)
		}
	}

	user, err := client.Tenants.AddUser(ctx, "tenant-a",
		models.UserCreate{Key: "erin", Email: "erin@example.com"})
	if errors.Is(err, permit.ErrConflict) {
		fmt.Println("the environment has a user with the key erin already")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("added", user.Key, "to tenant-a")

	if err := client.Tenants.RemoveUser(ctx, "tenant-a", "erin"); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("removed erin")
	// Output:
	// alice has the roles []
	// bob has the roles [viewer]
	// added erin to tenant-a
	// removed erin
}
