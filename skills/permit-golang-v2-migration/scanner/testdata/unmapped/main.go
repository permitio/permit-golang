// Command unmapped uses v1's pointer helper models.PtrString, a function of pkg/models that no
// entry of the migration guide names: the scanner reports it with no entry and exits 1. Each line
// that uses v1 ends with a comment "v1:" that lists what the scanner must report on it.
package main

import (
	"fmt"

	"github.com/permitio/permit-golang/pkg/models" // v1: github.com/permitio/permit-golang/pkg/models
)

func main() {
	tenant := models.TenantCreate{Key: "default", Name: "Default"} // v1: models.TenantCreate
	tenant.Description = models.PtrString("The default tenant")    // v1: models.TenantCreate.Description models.PtrString
	fmt.Println(tenant)
}
