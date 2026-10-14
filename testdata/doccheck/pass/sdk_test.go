package sdk_test

import (
	"context"
	"fmt"

	"example.com/sdk"
)

func ExampleNew() {
	fmt.Println(sdk.New().Users != nil)
	// Output:
	// true
}

func ExampleClient_Check() {
	allowed, err := sdk.New().Check(context.Background(), "alice")
	fmt.Println(allowed, err)
	// Output: true <nil>
}

func ExampleUsersService() {
	user, err := sdk.New().Users.Get(context.Background(), "alice")
	fmt.Println(user.Key, err)
	// Output: alice <nil>
}
