package permit_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// List the role assignments of the users alice and bob in the tenant tenant-a, both those in the
// tenant and those on its resource instances, and then the same role assignments with the
// details of their users and their roles. Without WithScope, the client acts on the environment
// of its API key, which it looks up with the first call.
func ExampleRoleAssignmentsService() {
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
	opts := &permit.RoleAssignmentsListOptions{
		Users: []string{"alice", "bob"}, Tenants: []string{"tenant-a"},
	}
	for assignment, err := range client.RoleAssignments.All(ctx, opts) {
		if err != nil {
			fmt.Println(err)
			return
		}
		where := "in " + assignment.Tenant
		if assignment.ResourceInstance != "" {
			where += " on " + assignment.ResourceInstance
		}
		fmt.Println(assignment.User, assignment.Role, where)
	}

	page, err := client.RoleAssignments.ListDetailed(ctx, opts)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, assignment := range page.Data {
		fmt.Println(assignment.User.Key, "-", assignment.Role.Name, assignment.Role.Permissions)
	}
	fmt.Println("role assignments:", *page.TotalCount)
	// Output:
	// alice editor in tenant-a
	// bob owner in tenant-a on document:doc-1
	// alice - Editor [document:read document:write]
	// bob - Owner []
	// role assignments: 2
}

// Assign a role through the PDP container, which holds its answer until the role assignment
// reaches its policy, for at most 10 seconds, so that a check right after it sees the role. Only
// the assignment and the unassignment of a role wait: a list through the same copy is an error
// that matches ErrInvalidConfig, and sends nothing.
func ExampleRoleAssignmentsService_WaitForSync() {
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
	assignments := client.RoleAssignments.WaitForSync(10 * time.Second)
	assignment, err := assignments.Assign(ctx, models.RoleAssignmentCreate{
		User: "idp|dave@example.com", Role: "editor", Tenant: "tenant-a",
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(assignment.User, "has the role", assignment.Role, "in", assignment.Tenant)

	allowed, err := client.Check(ctx, permit.User{Key: assignment.User}, "read",
		permit.Resource{Type: "document", Tenant: assignment.Tenant})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("allowed:", allowed)

	if _, err := assignments.List(ctx, nil); errors.Is(err, permit.ErrInvalidConfig) {
		fmt.Println("a list does not wait for sync")
	}
	// Output:
	// idp|dave@example.com has the role editor in tenant-a
	// allowed: true
	// a list does not wait for sync
}
