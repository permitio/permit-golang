package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
)

// Read the API key of an environment with a project-level API key, and build a client of the
// environment with its secret. The secret prints a marker in place of the credential, and its
// Value method reads the credential.
func ExampleAPIKeysService() {
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

	key, err := client.APIKeys.GetEnvironmentKey(context.Background(), "my-project", "production")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(key.ObjectType, key.EnvironmentID)
	fmt.Println("secret:", key.Secret)

	environmentClient, err := permit.New(
		permit.WithAPIKey(key.Secret.Value()),
		permit.WithAPIURL(exampleAPIURL),
		permit.WithPDPURL(examplePDPURL),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer environmentClient.Close()
	// Output:
	// env 00000000-0000-4000-8000-000000000003
	// secret: [REDACTED]
}

// Rotate the secret of an API key. RotateSecret is sent once, also after a 429, so a caller that
// is rate limited decides itself when to try again.
func ExampleAPIKeysService_RotateSecret() {
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
	key, err := client.APIKeys.GetEnvironmentKey(ctx, "my-project", "production")
	if err != nil {
		fmt.Println(err)
		return
	}
	rotated, err := client.APIKeys.RotateSecret(ctx, key.ID)
	if errors.Is(err, permit.ErrRateLimited) {
		fmt.Println("rate limited: the API did not rotate the secret; try again later")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(rotated.ID == key.ID, "a new secret:", !rotated.Secret.Equal(key.Secret))
	// Output:
	// true a new secret: true
}
