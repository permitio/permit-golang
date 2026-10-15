package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// Create a role of the resource document that extends another, with attributes whose values are
// a number and an object, list the roles of the resource, reading every page, read the ancestors
// of the role owner, such as the roles that it extends, and then make a role extend no role, with
// an empty slice, which the update sends as [] to clear the list; nil would leave the list out of
// the request. The calls of ResourceRolesService act on the environment of the client's scope:
// WithScope gives it here, and without it, the client looks up the scope of its
// environment-level API key.
func ExampleResourceRolesService() {
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
	role, err := client.ResourceRoles.Create(ctx, "document", models.ResourceRoleCreate{
		Key: "commenter", Name: "Commenter", Permissions: []string{"read"},
		Attributes: models.Attributes{"level": 2, "scope": map[string]any{"all": true}},
		Extends:    []string{"viewer"},
	})
	if errors.Is(err, permit.ErrConflict) {
		fmt.Println("the resource document has a role commenter already")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("created", role.Key, "extending", role.Extends, "with", role.Attributes)

	for role, err := range client.ResourceRoles.All(ctx, "document", nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(role.Key, role.Attributes)
	}

	ancestors, err := client.ResourceRoles.Ancestors(ctx, "document", "owner")
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, ancestor := range ancestors {
		fmt.Println("an ancestor of owner:", ancestor.Key)
	}

	editor, err := client.ResourceRoles.Update(ctx, "document", "editor",
		models.ResourceRoleUpdate{Extends: []string{}})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(editor.Key, "extends", len(editor.Extends), "roles")
	// Output:
	// created commenter extending [viewer] with map[level:2 scope:map[all:true]]
	// viewer map[tier:free]
	// editor map[seats:10]
	// owner map[quota:map[max:3 unlimited:false]]
	// an ancestor of owner: editor
	// an ancestor of owner: viewer
	// editor extends 0 roles
}
