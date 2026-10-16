package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// Create the group engineering in the tenant tenant-a, list the groups of the tenant, assign the
// user alice to the group, list its users, reading every page, and remove alice again. The calls
// of GroupsService act on the environment of the client's scope: WithScope gives it here, and
// without it, the client looks up the scope of its environment-level API key.
func ExampleGroupsService() {
	client, err := permit.New(
		permit.WithAPIKey(exampleAPIKey),
		permit.WithAPIURL(exampleAPIURL),
		permit.WithPDPURL(examplePDPURL),
		permit.WithScope("my-project", "production"),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer client.Close()

	ctx := context.Background()
	group, err := client.Groups.Create(ctx, models.GroupCreate{
		GroupInstanceKey: "engineering",
		GroupTenant:      "tenant-a",
	})
	if errors.Is(err, permit.ErrConflict) {
		fmt.Println("the group engineering exists already")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("created", group.GroupInstanceKey, "in", group.GroupTenant)

	page, err := client.Groups.List(ctx, &permit.GroupsListOptions{Tenant: "tenant-a"})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, g := range page.Data {
		fmt.Println("group:", g.GroupInstanceKey)
	}

	membership := models.GroupAssignUser{Tenant: "tenant-a"}
	assigned, err := client.Groups.AssignUser(ctx, "engineering", "alice", membership)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("engineering has", len(assigned.Users), "users")
	for user, err := range client.Groups.AllUsers(ctx, "engineering", nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println("member:", user.Key)
	}
	if err := client.Groups.RemoveUser(ctx, "engineering", "alice", membership); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("removed alice")
	// Output:
	// created engineering in tenant-a
	// group: engineering
	// group: sales
	// engineering has 2 users
	// member: alice
	// member: bob
	// removed alice
}
