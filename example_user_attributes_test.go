package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// Create an attribute of the users whose values are JSON objects, then list the attributes of the
// users, reading every page, with the type of each. These are the schema of what a user may hold;
// the values of a user are a part of the user. A type that this version of the SDK does not know,
// such as geo_point here, is read as it is, and its Known method reports false. The calls of
// UserAttributesService act on the environment of the client's scope: WithScope gives it here,
// and without it, the client looks up the scope of its environment-level API key.
func ExampleUserAttributesService() {
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
	attribute, err := client.UserAttributes.Create(ctx, models.ResourceAttributeCreate{
		Key:         "address",
		Type:        models.AttributeTypeObject,
		Description: "Where the user lives",
	})
	if errors.Is(err, permit.ErrConflict) {
		fmt.Println("the users have an attribute address already")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("created", attribute.Key, "of the type", attribute.Type)

	for attribute, err := range client.UserAttributes.All(ctx, nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		switch {
		case !attribute.Type.Known():
			fmt.Println(attribute.Key, attribute.Type, "(a type that this SDK does not know)")
		case attribute.BuiltIn:
			fmt.Println(attribute.Key, attribute.Type, "(built in)")
		default:
			fmt.Println(attribute.Key, attribute.Type)
		}
	}
	// Output:
	// created address of the type object
	// email string (built in)
	// age number
	// department string
	// verified bool
	// joined time
	// teams array
	// preferences json
	// address object
	// contacts object_array
	// location geo_point (a type that this SDK does not know)
}
