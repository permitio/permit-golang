package sdk_test

import "fmt"

// ExampleClient links [Client.Nothing]; pkg.go.dev shows the doc comment of an Example above its
// code. [Client] resolves, as the links of an Example name what its package declares.
func ExampleClient() {
	fmt.Println("client")
	// Output: client
}

// helper links [Nothing], which is not checked, as helper is not an Example.
func helper() {}
