package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// Grant the users of the user set adults the permission document:read on the documents of the
// resource set public_docs, list the condition set rules of the environment, reading every page,
// and then take the permission away. Both sets are condition sets of the environment, which
// ConditionSetsService creates. The calls of ConditionSetRulesService act on the environment of
// the client's scope: WithScope gives it here, and without it, the client looks up the scope of
// its environment-level API key.
func ExampleConditionSetRulesService() {
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
	rules, err := client.ConditionSetRules.Create(ctx, models.ConditionSetRuleCreate{
		UserSet: "adults", Permission: "document:read", ResourceSet: "public_docs",
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, rule := range rules {
		fmt.Println("granted", rule.Permission, "to", rule.UserSet, "on", rule.ResourceSet)
	}

	for rule, err := range client.ConditionSetRules.All(ctx, nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(rule.UserSet, rule.Permission, rule.ResourceSet)
	}

	err = client.ConditionSetRules.Delete(ctx, models.ConditionSetRuleRemove{
		UserSet: "adults", Permission: "document:read", ResourceSet: "public_docs",
	})
	if errors.Is(err, permit.ErrNotFound) {
		fmt.Println("a key of the rule is not found in the environment")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("removed document:read from adults on public_docs")
	// Output:
	// granted document:read to adults on public_docs
	// adults document:read public_docs
	// seniors document:write public_docs
	// removed document:read from adults on public_docs
}
