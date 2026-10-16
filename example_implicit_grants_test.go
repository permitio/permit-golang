package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// Grant the role viewer of the resource document to the users that have the role owner on a
// folder that is the parent of a document, with an implicit grant, also called a role derivation;
// stop the grant from applying to an instance that has a role assigned on it directly; and then
// delete the grant. Each method takes the key of the resource first and the key of its role
// second, as the route names them. The delete names the rule in its body, so a 404 to it is an
// error that matches ErrNotFound. The calls of ImplicitGrantsService act on the environment of
// the client's scope: WithScope gives it here, and without it, the client looks up the scope of
// its environment-level API key.
func ExampleImplicitGrantsService() {
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
	rule := models.DerivedRoleRuleCreate{
		Role: "owner", OnResource: "folder", LinkedByRelation: "parent",
	}
	grant, err := client.ImplicitGrants.Create(ctx, "document", "viewer", rule)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("viewer of document from", grant.Role, "of", grant.OnResource, "by",
		grant.LinkedByRelation)

	settings, err := client.ImplicitGrants.UpdateConditions(ctx, "document", "viewer",
		models.DerivationSettings{NoDirectRolesOnObject: true})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("no direct roles on the object:", settings.NoDirectRolesOnObject)

	err = client.ImplicitGrants.Delete(ctx, "document", "viewer", models.DerivedRoleRuleDelete{
		Role: "owner", OnResource: "folder", LinkedByRelation: "parent",
	})
	if errors.Is(err, permit.ErrNotFound) {
		fmt.Println("viewer of document has no such implicit grant")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("deleted the implicit grant")
	// Output:
	// viewer of document from owner of folder by parent
	// no direct roles on the object: true
	// deleted the implicit grant
}
