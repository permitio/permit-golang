package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// List the environments of a project, reading every page, and get one by its key. The calls of
// EnvironmentsService take the project and need no scope, so a client with a project-level API
// key needs no WithScope.
func ExampleEnvironmentsService() {
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
	for environment, err := range client.Environments.All(ctx, "my-project", nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(environment.Key, "-", environment.Name)
	}

	environment, err := client.Environments.Get(ctx, "my-project", "production")
	if errors.Is(err, permit.ErrNotFound) {
		fmt.Println("the project has no environment production")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(environment.Description)
	if environment.JWKS != nil {
		fmt.Println("audience:", environment.JWKS.Audience.Values())
	}
	// Output:
	// production - Production
	// staging - Staging
	// Live traffic
	// audience: [app]
}

// Change an environment: its settings and the configuration of its JSON Web Key Sets are each
// sent as a whole, and a null description sends null. The fields that the update leaves unset,
// such as the branch name, keep their values.
func ExampleEnvironmentsService_Update() {
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

	environment, err := client.Environments.Update(context.Background(), "my-project",
		"production", models.EnvironmentUpdate{
			Name:        "Live",
			Description: models.Null[string](),
			JWKS: models.Some(models.JwksConfig{
				Audience: models.OneString("app"), TTL: 600,
			}),
			Settings: models.Some(models.Attributes{"limit": 10}),
		})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(environment.Name, environment.Settings["limit"])
	if environment.JWKS != nil {
		fmt.Println("key sets kept for", environment.JWKS.TTL, "seconds")
	}
	// Output:
	// Live 10
	// key sets kept for 600 seconds
}
