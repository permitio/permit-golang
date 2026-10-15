package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// Create a role that extends another, with attributes whose values are a number and an object,
// list the roles of the environment, reading every page, with their attributes of any JSON type,
// and then make a role extend no role, with an empty slice, which the update sends as [] to clear
// the list; nil would leave the list as it is. The calls of RolesService act on the environment
// of the client's scope: WithScope gives it here, and without it, the client looks up the scope of
// its environment-level API key.
func ExampleRolesService() {
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
	role, err := client.Roles.Create(ctx, models.RoleCreate{
		Key: "auditor", Name: "Auditor", Permissions: []string{"document:read"},
		Attributes: models.Attributes{"level": 2, "scope": map[string]any{"all": true}},
		Extends:    []string{"viewer"},
	})
	if errors.Is(err, permit.ErrConflict) {
		fmt.Println("the environment has a role auditor already")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("created", role.Key, "extending", role.Extends, "with", role.Attributes)

	for role, err := range client.Roles.All(ctx, nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(role.Key, role.Attributes)
	}

	editor, err := client.Roles.Update(ctx, "editor", models.RoleUpdate{Extends: []string{}})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(editor.Key, "extends", len(editor.Extends), "roles")
	// Output:
	// created auditor extending [viewer] with map[level:2 scope:map[all:true]]
	// viewer map[tier:free]
	// editor map[seats:10]
	// admin map[quota:map[max:3 unlimited:false]]
	// editor extends 0 roles
}
