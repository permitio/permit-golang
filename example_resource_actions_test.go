package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// List the actions of a resource whose attribute risk is low, reading every page, and create an
// action with an attribute that such a filter matches. The calls of ResourceActionsService act on
// the environment of the client's scope: WithScope gives it here, and without it, the client
// looks up the scope of its environment-level API key.
func ExampleResourceActionsService() {
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
	lowRisk := &permit.ResourceActionsListOptions{
		Attributes: []permit.AttributeFilter{permit.AttributeEquals("risk", "low")},
	}
	for action, err := range client.ResourceActions.All(ctx, "document", lowRisk) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(action.PermissionName, "-", action.Name)
	}

	action, err := client.ResourceActions.Create(ctx, "document", models.ResourceActionCreate{
		Key:        "approve",
		Name:       "Approve",
		Attributes: models.Attributes{"risk": "high"},
	})
	if errors.Is(err, permit.ErrConflict) {
		fmt.Println("document has an action approve already")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("created", action.PermissionName, "with the risk", action.Attributes["risk"])
	// Output:
	// document:read - Read
	// document:share - Share
	// created document:approve with the risk high
}
