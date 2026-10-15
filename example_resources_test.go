package permit_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// List the resources of an environment, reading every page, and get one by its key. The calls
// of ResourcesService act on the environment of the client's scope: WithScope gives it here, and
// without it, the client looks up the scope of its environment-level API key.
func ExampleResourcesService() {
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
	for resource, err := range client.Resources.All(ctx, nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(resource.Key, "-", resource.Name)
	}

	resource, err := client.Resources.Get(ctx, "document")
	if errors.Is(err, permit.ErrNotFound) {
		fmt.Println("the environment has no resource document")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("actions:", slices.Sorted(maps.Keys(resource.Actions)))
	fmt.Println("editor grants:", resource.Roles["editor"].Permissions)
	// Output:
	// document - Document
	// folder - Folder
	// actions: [read write]
	// editor grants: [read write]
}

// Replace the definition of a resource: the actions that it sends are the whole set of actions
// of the resource, so write, which this definition leaves out, is removed, and archive is added.
func ExampleResourcesService_Replace() {
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

	resource, err := client.Resources.Replace(context.Background(), "document",
		models.ResourceReplace{
			Name:        "Document",
			Description: "Replaced",
			Actions: map[string]models.ActionBlockEditable{
				"read":    {},
				"archive": {Name: "Archive"},
			},
		})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(resource.Description, slices.Sorted(maps.Keys(resource.Actions)))
	// Output:
	// Replaced [archive read]
}
