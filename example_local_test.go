package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
)

// List the role assignments of a user that the PDP container holds, reading every page. Only the
// PDP container serves the request: the cloud PDP answers it with an error that matches
// permit.ErrContainerPDPOnly.
func ExampleClient_LocalRoleAssignmentsAll() {
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

	opts := &permit.LocalRoleAssignmentsOptions{User: "alice"}
	for assignment, err := range client.LocalRoleAssignmentsAll(context.Background(), opts) {
		if errors.Is(err, permit.ErrContainerPDPOnly) {
			fmt.Println("point permit.WithPDPURL at a PDP container")
			return
		}
		if err != nil {
			fmt.Println(err)
			return
		}
		if assignment.ResourceInstance == nil {
			fmt.Println(assignment.User, "is", assignment.Role, "in", assignment.Tenant)
			continue
		}
		fmt.Println(assignment.User, "is", assignment.Role, "of", *assignment.ResourceInstance,
			"in", assignment.Tenant)
	}
	// Output:
	// alice is admin in tenant-a
	// alice is editor of document:2026-report in tenant-a
}
