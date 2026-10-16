package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// Create an attribute of a resource whose values are JSON objects, then list the attributes of
// the resource, reading every page, with the type of each. A type that this version of the SDK
// does not know, such as geo_point here, is read as it is, and its Known method reports false.
// The calls of ResourceAttributesService act on the environment of the client's scope: WithScope
// gives it here, and without it, the client looks up the scope of its environment-level API key.
func ExampleResourceAttributesService() {
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
	attribute, err := client.ResourceAttributes.Create(ctx, "document",
		models.ResourceAttributeCreate{
			Key:         "address",
			Type:        models.AttributeTypeObject,
			Description: "Where it is kept",
		})
	if errors.Is(err, permit.ErrConflict) {
		fmt.Println("document has an attribute address already")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("created", attribute.Key, "of the type", attribute.Type)

	for attribute, err := range client.ResourceAttributes.All(ctx, "document", nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		if !attribute.Type.Known() {
			fmt.Println(attribute.Key, attribute.Type, "(a type that this SDK does not know)")
			continue
		}
		fmt.Println(attribute.Key, attribute.Type)
	}
	// Output:
	// created address of the type object
	// archived bool
	// pages number
	// owner string
	// due time
	// tags array
	// settings json
	// address object
	// contacts object_array
	// location geo_point (a type that this SDK does not know)
}
