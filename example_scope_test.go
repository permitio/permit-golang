package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
)

// Look up the scope of the API key after New, to check the key at once: Scope returns the project
// and the environment that the client's calls act on, or the error of the lookup, such as one
// that matches ErrUnauthorized for a key that the API does not accept. The client keeps the
// scope, so the calls that need it send no other lookup.
func ExampleClient_Scope() {
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

	scope, err := client.Scope(context.Background())
	if errors.Is(err, permit.ErrUnauthorized) {
		fmt.Println("the API does not accept the API key")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("project:", scope.Project)
	fmt.Println("environment:", scope.Environment)
	// Output:
	// project: 00000000-0000-4000-8000-000000000002
	// environment: 00000000-0000-4000-8000-000000000003
}

// Give the project and the environment that the calls act on, by key or by ID, such as for an
// organization-level API key, whose scope has neither. The client then sends no request for the
// scope of its key. An environment without a project is an error that matches ErrInvalidConfig.
func ExampleWithScope() {
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

	scope, err := client.Scope(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(scope.Project, scope.Environment)

	_, err = permit.New(
		permit.WithAPIKey(exampleAPIKey),
		permit.WithScope("", "production"),
	)
	fmt.Println(errors.Is(err, permit.ErrInvalidConfig))
	// Output:
	// my-project production
	// true
}
