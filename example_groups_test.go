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

// Assign the role editor on the instance doc-1 of the resource document to the group
// engineering, list the roles of the group, reading every page, assign the group backend to
// engineering, list the children of engineering, and remove the group backend and the role from
// engineering again.
func ExampleGroupsService_AssignRole() {
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
	role := models.GroupAddRole{
		Role: "editor", Resource: "document", ResourceInstance: "doc-1", Tenant: "tenant-a",
	}
	group, err := client.Groups.AssignRole(ctx, "engineering", role)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(group.GroupInstanceKey, "has the roles", group.AssignedRoles)
	for r, err := range client.Groups.AllRoles(ctx, "engineering", nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println("role:", r.Key, "on", r.Resource.Key, r.ResourceInstance.Key)
	}

	backend := models.GroupAssignment{GroupInstanceKey: "group:backend"}
	if _, err := client.Groups.AssignGroup(ctx, "engineering", backend); err != nil {
		fmt.Println(err)
		return
	}
	for child, err := range client.Groups.AllChildren(ctx, "engineering", nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println("child:", child.GroupInstanceKey)
	}
	if err := client.Groups.RemoveGroup(ctx, "engineering", backend); err != nil {
		fmt.Println(err)
		return
	}
	if err := client.Groups.RemoveRole(ctx, "engineering", role); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("removed backend and editor from engineering")
	// Output:
	// engineering has the roles [editor]
	// role: editor on document doc-1
	// role: viewer on document doc-2
	// child: backend
	// child: frontend
	// removed backend and editor from engineering
}
