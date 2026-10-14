package permit_test

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/permitio/permit-golang/v2"
)

// List what a user may do in one tenant, with the context of the request. The PDP keys the
// permissions in a tenant as __tenant:<tenant>, and those on a resource instance as
// <type>:<key>.
func ExampleClient_UserPermissions() {
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

	permissions, err := client.UserPermissions(context.Background(), permit.User{Key: "alice"},
		permit.WithTenants("tenant-a"),
		permit.WithDecisionContext(map[string]any{"business_hours": true}),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, key := range slices.Sorted(maps.Keys(permissions)) {
		fmt.Println(key, permissions[key].Roles, permissions[key].Permissions)
	}
	// Output:
	// __tenant:tenant-a [viewer] [document:read]
	// document:2026-report [editor] [document:read document:update]
}

// List the users that may read one document, each with the role that allows it and where the
// role is assigned: on the document itself, or in its tenant.
func ExampleClient_AuthorizedUsers() {
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

	result, err := client.AuthorizedUsers(context.Background(), "read",
		permit.Resource{Type: "document", Key: "2026-report", Tenant: "tenant-a"})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, user := range slices.Sorted(maps.Keys(result.Users)) {
		for _, assignment := range result.Users[user] {
			fmt.Println(user, "as", assignment.Role, "on", assignment.Resource)
		}
	}
	// Output:
	// alice as editor on document:2026-report
	// bob as viewer on __tenant:tenant-a
}
