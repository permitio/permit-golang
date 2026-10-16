// Command none uses v2, and a package whose path starts like v1's: the scanner must report
// nothing.
package main

import (
	"fmt"

	extras "github.com/permitio/permit-golang-extras/pkg/permit"
	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

func main() {
	client, err := permit.New()
	fmt.Println(client, err, models.UserCreate{Key: "alice"}, extras.Version)
}
