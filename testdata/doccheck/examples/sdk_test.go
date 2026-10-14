package sdk_test

import (
	"fmt"

	"example.com/sdk"
)

func ExampleNew() {
	sdk.New()
	// Output:
}

func ExampleClient_Check() {
	fmt.Println(sdk.New().Check())
	// Output: true
}

func ExampleRolesAPI_List() {
	fmt.Println(len(sdk.RolesAPI{}.List()))
	// Output: 0
}

func ExampleTenantsService() {
	fmt.Println(sdk.New().Tenants)
}

func ExampleClient_Allowed() {
	fmt.Println(sdk.New().Allowed())
}
