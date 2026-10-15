package permit_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/permitio/permit-golang/v2"
)

// List the tenants in which a user may read documents. Only the PDP container serves the
// request: the cloud PDP answers it with an error that matches permit.ErrContainerPDPOnly.
func ExampleClient_AllTenantsCheck() {
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

	tenants, err := client.AllTenantsCheck(context.Background(), permit.User{Key: "alice"},
		"read", permit.Resource{Type: "document"})
	if errors.Is(err, permit.ErrContainerPDPOnly) {
		fmt.Println("point permit.WithPDPURL at a PDP container")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, tenant := range tenants {
		fmt.Println(tenant.Key, tenant.Attributes)
	}
	// Output:
	// tenant-a map[plan:pro]
}

// List the tenants of a user: those in which the user has a role.
func ExampleClient_UserTenants() {
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

	tenants, err := client.UserTenants(context.Background(), permit.User{Key: "alice"})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, tenant := range tenants {
		fmt.Println(tenant.Key)
	}
	// Output:
	// tenant-a
	// tenant-b
}

// Ask whether a user may send a request to a URL of an application, which the mapping rules of
// the environment map to a resource and an action.
func ExampleClient_CheckURL() {
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

	allowed, err := client.CheckURL(context.Background(), permit.User{Key: "alice"},
		http.MethodGet, "https://app.example.com/documents/2026-report", "tenant-a")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("alice may read the report:", allowed)
	// Output:
	// alice may read the report: true
}
