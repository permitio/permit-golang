package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// Create a proxy config whose secret is a header that the Permit proxy adds to the requests that
// it sends on, with a mapping rule for the URL check of the PDP container, then list the proxy
// configs, reading every page. A secret prints the marker in place of each credential, as do the
// configs that hold one; Value of each Secret returns the credential. The calls of
// ProxyConfigsService act on the environment of the client's scope: WithScope gives it here, and
// without it, the client looks up the scope of its environment-level API key.
func ExampleProxyConfigsService() {
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
	config, err := client.ProxyConfigs.Create(ctx, models.ProxyConfigCreate{
		Key:           "payments",
		Name:          "Payments API",
		AuthMechanism: models.AuthMechanismHeaders,
		Secret: models.ProxySecret{Headers: map[string]models.Secret{
			"X-Api-Key": models.NewSecret("the-key-of-the-backend-service"),
		}},
		MappingRules: []models.MappingRule{{
			URL:        "https://api.example.com/orders/.*",
			URLType:    models.URLTypeRegex,
			HTTPMethod: models.MethodsPost,
			Resource:   "order",
			Action:     "create",
		}},
	})
	if errors.Is(err, permit.ErrConflict) {
		fmt.Println("the environment has a proxy config payments already")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("created", config.Key, "with the secret", config.Secret)

	for config, err := range client.ProxyConfigs.All(ctx, nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(config.Key, config.AuthMechanism, config.Secret)
	}
	// Output:
	// created payments with the secret {Headers:map[X-Api-Key:[REDACTED]]}
	// billing Bearer {Bearer:[REDACTED]}
	// ledger Basic {Basic:[REDACTED]}
	// payments Headers {Headers:map[X-Api-Key:[REDACTED]]}
}
