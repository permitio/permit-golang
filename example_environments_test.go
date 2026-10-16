package permit_test

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// Read the statistics of an environment and the configurations of its PDPs. The client secret
// of a PDP prints as a marker in place of the credential; its Value method returns it.
func ExampleEnvironmentsService_Stats() {
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

	stats, err := client.Environments.Stats(context.Background(), "my-project", "production")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(stats.Key, "has", stats.Stats.Users, "users and", stats.Stats.Roles, "roles")
	for _, config := range stats.PDPConfigs {
		fmt.Println(config.Name, config.ClientSecret)
	}
	// Output:
	// production has 1200 users and 3 roles
	// Production PDP [REDACTED]
}

// Copy an environment into a new one in the background, then read the task of the copy, which
// the API may hold for up to 10 seconds while the copy runs. The wait counts toward the timeout
// of the call, so the client's timeout is longer than the wait.
func ExampleEnvironmentsService_CopyAsync() {
	client, err := permit.New(
		permit.WithAPIKey(exampleAPIKey),
		permit.WithAPIURL(exampleAPIURL),
		permit.WithPDPURL(examplePDPURL),
		permit.WithTimeout(time.Minute),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer client.Close()

	ctx := context.Background()
	task, err := client.Environments.CopyAsync(ctx, "my-project", "production",
		models.EnvironmentCopy{
			TargetEnv: models.EnvironmentCopyTarget{
				New: &models.EnvironmentCreate{Key: "qa-copy", Name: "QA copy"},
			},
		})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("submitted:", task.Status)

	task, err = client.Environments.CopyResult(ctx, "my-project", "production", task.TaskID,
		&permit.CopyResultOptions{Wait: 10 * time.Second})
	if err != nil {
		fmt.Println(err)
		return
	}
	if task.Status == models.TaskStatusSuccess && task.Result != nil {
		fmt.Println("copied into", task.Result.Key)
	}
	// Output:
	// submitted: processing
	// copied into qa-copy
}
