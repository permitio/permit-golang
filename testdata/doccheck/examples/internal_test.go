package sdk

import "fmt"

func ExampleClient() {
	fmt.Println(New() != nil)
}

func ExampleUsersService() {
	fmt.Println(UsersService{})
	// Output: {}
}
