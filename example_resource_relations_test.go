package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// Count the relations of the resource document, list them, reading every page, and create a
// relation of document to the resource team, its subject, so that a relationship tuple such as
// team:sales team document:q3 relates an instance of team to one of document. The calls of
// ResourceRelationsService act on the environment of the client's scope: WithScope gives it
// here, and without it, the client looks up the scope of its environment-level API key.
func ExampleResourceRelationsService() {
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
	page, err := client.ResourceRelations.List(ctx, "document", nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	if page.TotalCount != nil {
		fmt.Println("document has", *page.TotalCount, "relations")
	}
	for relation, err := range client.ResourceRelations.All(ctx, "document", nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(relation.Key+":", relation.SubjectResource, "to", relation.ObjectResource)
	}

	relation, err := client.ResourceRelations.Create(ctx, "document", models.RelationCreate{
		Key:             "team",
		Name:            "Team",
		SubjectResource: "team",
	})
	if errors.Is(err, permit.ErrConflict) {
		fmt.Println("document has a relation team already")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("created", relation.Key+":", relation.SubjectResource, "to",
		relation.ObjectResource)
	// Output:
	// document has 2 relations
	// parent: folder to document
	// workspace: workspace to document
	// created team: team to document
}
