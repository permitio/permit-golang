# A fixture of readme_test.go whose marked go blocks do not compile

The first block uses a name that the SDK does not export, and the second is not a program.
<!-- compile -->
```go
package main

import (
	"fmt"

	"github.com/permitio/permit-golang/v2"
)

func main() {
	fmt.Println(permit.Nothing)
}
```

<!-- compile -->
```go
package readme

func Run() {}
```
