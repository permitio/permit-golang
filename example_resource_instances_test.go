package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// Create the instance doc-3 of the resource document in the tenant tenant-a, list every instance
// with its relationship tuples, replace the attributes of doc-1, and delete doc-3. An instance
// belongs to a tenant: Create refuses one without a tenant, and sends nothing.
func ExampleResourceInstancesService() {
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
	_, err = client.ResourceInstances.Create(ctx,
		models.ResourceInstanceCreate{Key: "doc-4", Resource: "document"})
	if errors.Is(err, permit.ErrInvalidConfig) {
		fmt.Println("doc-4 has no tenant")
	}

	created, err := client.ResourceInstances.Create(ctx, models.ResourceInstanceCreate{
		Key: "doc-3", Tenant: "tenant-a", Resource: "document",
		Attributes: models.Attributes{"owner": "carol"},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("created", created.Resource+":"+created.Key, "in", created.Tenant)

	for instance, err := range client.ResourceInstances.AllDetailed(ctx, nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(instance.Resource+":"+instance.Key, "has",
			len(instance.Relationships), "relationships")
		for _, tuple := range instance.Relationships {
			fmt.Println("  ", tuple.Subject, tuple.Relation, tuple.Object)
		}
	}

	updated, err := client.ResourceInstances.Update(ctx, "document:doc-1",
		models.ResourceInstanceUpdate{Attributes: models.Some(models.Attributes{"owner": "bob"})})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("the owner of doc-1:", updated.Attributes["owner"])

	if err := client.ResourceInstances.Delete(ctx, "document:doc-3"); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("deleted document:doc-3")
	// Output:
	// doc-4 has no tenant
	// created document:doc-3 in tenant-a
	// document:doc-1 has 2 relationships
	//    folder:f1 parent document:doc-1
	//    document:doc-1 attachment document:doc-2
	// document:doc-2 has 0 relationships
	// the owner of doc-1: bob
	// deleted document:doc-3
}
