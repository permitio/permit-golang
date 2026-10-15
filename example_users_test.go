package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// List the users of the environment that have the role viewer, and get one by its key. Without
// WithScope, the client acts on the environment of its API key, which it looks up with the
// first call.
func ExampleUsersService() {
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
	page, err := client.Users.List(ctx, &permit.UsersListOptions{Role: "viewer"})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, user := range page.Data {
		fmt.Println(user.Key, "-", len(user.AssociatedTenants), "tenants")
	}
	fmt.Println("users:", *page.TotalCount)

	user, err := client.Users.Get(ctx, "alice")
	if errors.Is(err, permit.ErrNotFound) {
		fmt.Println("the environment has no user with the key alice")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, tenant := range user.AssociatedTenants {
		fmt.Println(tenant.Tenant, tenant.Status, tenant.Roles)
	}
	// Output:
	// alice - 0 tenants
	// bob - 1 tenants
	// users: 2
	// tenant-a active [viewer editor]
	// tenant-b pending []
}

// Sync a user twice with one request each: the first creates the user, and the second replaces
// it, so the first name that it leaves out is cleared. Sync sends the whole user: to change some
// fields and keep the others, use Update.
func ExampleUsersService_Sync() {
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
	for _, user := range []models.UserCreate{
		{
			Key: "idp|dave@example.com", Email: "dave@example.com", FirstName: "Dave",
			Attributes: models.Attributes{"department": "support"},
		},
		{Key: "idp|dave@example.com", Email: "dave@example.com"},
	} {
		result, err := client.Users.Sync(ctx, user)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("created: %t, first name: %q\n", result.Created, result.User.FirstName)
	}
	// Output:
	// created: true, first name: "Dave"
	// created: false, first name: ""
}
