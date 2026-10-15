package permit_test

import (
	"context"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// List the action groups of the resource document, reading every page, and rename one. The
// update sends the actions that the group keeps, as an update that leaves them out may remove
// them all. The calls of ResourceActionGroupsService act on the environment of the client's
// scope: WithScope gives it here, and without it, the client looks up the scope of its
// environment-level API key.
func ExampleResourceActionGroupsService() {
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
	for group, err := range client.ResourceActionGroups.All(ctx, "document", nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(group.Key, "-", group.Name, group.Actions)
	}

	group, err := client.ResourceActionGroups.Update(ctx, "document", "editing",
		models.ResourceActionGroupUpdate{
			Name:    "Renamed editing",
			Actions: models.Some([]string{"read"}),
		})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(group.Name, group.Actions)
	// Output:
	// editing - Editing [read write]
	// viewing - Viewing []
	// Renamed editing [read]
}
