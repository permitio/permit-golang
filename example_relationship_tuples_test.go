package permit_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// Make the folder folder:f1 the parent of the document document:doc-1 in the tenant tenant-a,
// list the relationship tuples of the folder, then the same relationship tuples with the details
// of their objects, and delete the new one. Without WithScope, the client acts on the environment
// of its API key, which it looks up with the first call.
func ExampleRelationshipTuplesService() {
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
	created, err := client.RelationshipTuples.Create(ctx, models.RelationshipTupleCreate{
		Subject: "folder:f1", Relation: "parent", Object: "document:doc-1", Tenant: "tenant-a",
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("created:", created.Subject, created.Relation, created.Object)

	opts := &permit.RelationshipTuplesListOptions{Subject: "folder:f1", Relation: "parent"}
	for tuple, err := range client.RelationshipTuples.All(ctx, opts) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(tuple.Subject, tuple.Relation, tuple.Object, "in", tuple.Tenant)
	}

	page, err := client.RelationshipTuples.ListDetailed(ctx, opts)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, tuple := range page.Data {
		if tuple.ObjectDetails != nil {
			fmt.Println(tuple.Object, "-", tuple.ObjectDetails.Resource,
				tuple.ObjectDetails.Attributes)
		}
	}

	err = client.RelationshipTuples.Delete(ctx, models.RelationshipTupleDelete{
		Subject: created.Subject, Relation: created.Relation, Object: created.Object,
	})
	fmt.Println("deleted:", err == nil)
	// Output:
	// created: folder:f1 parent document:doc-1
	// folder:f1 parent document:doc-1 in tenant-a
	// folder:f1 parent document:doc-2 in tenant-a
	// document:doc-1 - document map[pages:12]
	// document:doc-2 - document map[]
	// deleted: true
}

// Make the folder folder:f1 the parent of the documents document:doc-1 and document:doc-2 in one
// request, and then delete both relationship tuples in one request, each named by its subject,
// its relation and its object. Neither call returns a result: the API's reply to each holds no
// field.
func ExampleRelationshipTuplesService_BulkCreate() {
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
	tuples := []models.RelationshipTupleCreate{
		{Subject: "folder:f1", Relation: "parent", Object: "document:doc-1", Tenant: "tenant-a"},
		{Subject: "folder:f1", Relation: "parent", Object: "document:doc-2", Tenant: "tenant-a"},
	}
	if err := client.RelationshipTuples.BulkCreate(ctx, tuples); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("created:", len(tuples))

	keys := make([]models.RelationshipTupleDelete, 0, len(tuples))
	for _, tuple := range tuples {
		keys = append(keys, models.RelationshipTupleDelete{
			Subject: tuple.Subject, Relation: tuple.Relation, Object: tuple.Object,
		})
	}
	if err := client.RelationshipTuples.BulkDelete(ctx, keys); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("deleted:", len(keys))
	// Output:
	// created: 2
	// deleted: 2
}

// Create a relationship tuple through the PDP container, which holds its answer until the tuple
// reaches its policy, for at most 10 seconds, so that a check right after it sees the relation.
// Only a create waits: a delete through the same copy is an error that matches ErrInvalidConfig,
// and sends nothing.
func ExampleRelationshipTuplesService_WaitForSync() {
	client, err := permit.New(
		permit.WithAPIKey(exampleAPIKey),
		permit.WithAPIURL(exampleAPIURL),
		permit.WithPDPURL(examplePDPURL),
		permit.WithFactsViaPDP(),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer client.Close()

	ctx := context.Background()
	tuples := client.RelationshipTuples.WaitForSync(10 * time.Second)
	tuple, err := tuples.Create(ctx, models.RelationshipTupleCreate{
		Subject: "folder:f1", Relation: "parent", Object: "document:doc-1", Tenant: "tenant-a",
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(tuple.Subject, "is the", tuple.Relation, "of", tuple.Object)

	allowed, err := client.Check(ctx, permit.User{Key: "alice"}, "read",
		permit.Resource{Type: "document", Key: "doc-1", Tenant: tuple.Tenant})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("allowed:", allowed)

	err = tuples.Delete(ctx, models.RelationshipTupleDelete{
		Subject: tuple.Subject, Relation: tuple.Relation, Object: tuple.Object,
	})
	if errors.Is(err, permit.ErrInvalidConfig) {
		fmt.Println("a delete does not wait for sync")
	}
	// Output:
	// folder:f1 is the parent of document:doc-1
	// allowed: true
	// a delete does not wait for sync
}
