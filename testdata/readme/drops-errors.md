# A fixture of readme_test.go whose marked go blocks compile, and the first drops errors

The first block drops an error in each way that the check finds; the second only what it allows.

<!-- compile -->
```go
package main

import (
	"context"
	"strconv"

	"github.com/permitio/permit-golang/v2"
)

func main() {
	client, _ := permit.New()
	n, _ := strconv.Atoi("1")
	_ = client.Close()
	var _ error = client.Close()
	ctx := context.Background()
	for tenant, _ := range client.Tenants.All(ctx, nil) {
		_ = tenant
	}
	for tenant := range client.Tenants.All(ctx, nil) {
		_ = tenant
	}
	client.Close()
	strconv.Atoi("2")
	_, _ = n, ctx
}
```

<!-- compile -->
```go
package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/permitio/permit-golang/v2"
)

func main() {
	client, err := permit.New()
	if err != nil {
		panic(err)
	}
	defer client.Close()
	_, err = strconv.Atoi("1")
	if err != nil {
		panic(err)
	}
	for _, err := range client.Tenants.All(context.Background(), nil) {
		if _, ok := errors.AsType[*permit.Error](err); ok {
			panic("the API rejected the list: " + err.Error())
		}
		if err != nil {
			panic(err)
		}
	}
	counts := map[string]int{}
	_, ok := counts["tenant-a"]
	var _ = ok
	for range 3 {
	}
	fmt.Println("listed the tenants")
}
```
