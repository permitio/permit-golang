package permit_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// List the projects of the organization, reading every page, and get one by its key. The calls
// of ProjectsService need no scope, so a client with an organization-level API key, whose scope
// has no project and no environment, needs no WithScope.
func ExampleProjectsService() {
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
	for project, err := range client.Projects.All(ctx, nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(project.Key, "-", project.Name)
	}

	project, err := client.Projects.Get(ctx, "my-project")
	if errors.Is(err, permit.ErrNotFound) {
		fmt.Println("the organization has no project my-project")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(project.Description, "since", project.CreatedAt.Format(time.DateOnly))
	// Output:
	// my-project - My project
	// staging - Staging
	// The main project since 2026-10-14
}

// Create a project without the environments that the API creates by default, by giving an
// empty list of initial environments. A key that another project of the organization has is an
// error that matches ErrConflict.
func ExampleProjectsService_Create() {
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

	project, err := client.Projects.Create(context.Background(), models.ProjectCreate{
		Key:                 "new-project",
		Name:                "New project",
		Description:         "A project of its own",
		InitialEnvironments: []models.EnvironmentCreate{},
	})
	if errors.Is(err, permit.ErrConflict) {
		fmt.Println("the organization has a project new-project already")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(project.Key, project.ID)
	// Output:
	// new-project 00000000-0000-4000-8000-000000000005
}
