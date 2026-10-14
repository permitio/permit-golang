# A fixture of readme_test.go whose marked go blocks compile

<!-- compile -->
```go
package main

import (
	"fmt"

	"github.com/permitio/permit-golang/v2"
)

func main() {
	fmt.Println(permit.Version)
}
```

A go block without the marker is not compiled:

```go
this is not Go
```

<!-- compile -->
~~~go
package main

import _ "github.com/permitio/permit-golang/v2/models"

func main() {}
~~~
