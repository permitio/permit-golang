package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// Create a user set of the users older than 65, under the user set adults, list the condition
// sets of the environment, reading every page, and then list the ancestors of the new set. A
// set's conditions are a models.Attributes, whose values may be of any JSON type. The calls of
// ConditionSetsService act on the environment of the client's scope: WithScope gives it here,
// and without it, the client looks up the scope of its environment-level API key.
func ExampleConditionSetsService() {
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
	olderThan65 := map[string]any{"user.age": map[string]any{"greater-than": 65}}
	set, err := client.ConditionSets.Create(ctx, models.ConditionSetCreate{
		Key: "seniors", Type: models.ConditionSetTypeUserSet, Name: "Seniors",
		Conditions: models.Attributes{
			"allOf": []any{map[string]any{"allOf": []any{olderThan65}}},
		},
		ParentID: "00000000-0000-4000-8000-000000000201",
	})
	if errors.Is(err, permit.ErrConflict) {
		fmt.Println("the environment has a condition set seniors already")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("created", set.Key, "of the type", set.Type)

	for set, err := range client.ConditionSets.All(ctx, nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(set.Key, set.Type)
	}

	for ancestor, err := range client.ConditionSets.AncestorsAll(ctx, "seniors", nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println("seniors is under", ancestor.Key)
	}
	// Output:
	// created seniors of the type userset
	// adults userset
	// seniors userset
	// public_docs resourceset
	// seniors is under adults
}
