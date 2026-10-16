package permit_test

import (
	"context"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// Seed an environment with one request for each kind of object, and reset it the same way:
// create two tenants and two users, each with a role in one of the tenants, create or replace a
// resource instance in each tenant, and replace the users with their email addresses; then
// delete the instances, the users and the tenants. Each bulk call sends all its items in one
// request, does not wait for sync, and returns only an error.
func ExampleUsersService_BulkCreate() {
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
	err = client.Tenants.BulkCreate(ctx, []models.TenantCreate{
		{Key: "tenant-a", Name: "Tenant A"},
		{Key: "tenant-b", Name: "Tenant B"},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("created tenant-a and tenant-b")

	editor := models.UserRoleCreate{Role: "editor", Tenant: "tenant-a"}
	viewer := models.UserRoleCreate{Role: "viewer", Tenant: "tenant-b"}
	err = client.Users.BulkCreate(ctx, []models.UserCreate{
		{Key: "alice", RoleAssignments: []models.UserRoleCreate{editor}},
		{Key: "bob", RoleAssignments: []models.UserRoleCreate{viewer}},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("created alice and bob")

	err = client.ResourceInstances.BulkReplace(ctx, []models.ResourceInstanceCreate{
		{Key: "doc-1", Tenant: "tenant-a", Resource: "document"},
		{Key: "doc-2", Tenant: "tenant-b", Resource: "document"},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("created or replaced document:doc-1 and document:doc-2")

	err = client.Users.BulkReplace(ctx, []models.UserCreate{
		{Key: "alice", Email: "alice@example.com"},
		{Key: "bob", Email: "bob@example.com"},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("replaced alice and bob")

	if err := client.ResourceInstances.BulkDelete(ctx,
		[]string{"document:doc-1", "document:doc-2"}); err != nil {
		fmt.Println(err)
		return
	}
	if err := client.Users.BulkDelete(ctx, []string{"alice", "bob"}); err != nil {
		fmt.Println(err)
		return
	}
	if err := client.Tenants.BulkDelete(ctx, []string{"tenant-a", "tenant-b"}); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("deleted the instances, the users and the tenants")
	// Output:
	// created tenant-a and tenant-b
	// created alice and bob
	// created or replaced document:doc-1 and document:doc-2
	// replaced alice and bob
	// deleted the instances, the users and the tenants
}
