// The example "Sync users" of the README of v1.3.0. Each line that uses v1 ends with a comment
// "v1:" that lists what the scanner must report on it, or "v1 missed:" for a field of a value
// whose v1 type the scanner does not know.
package main

import (
	"context"
	"fmt"

	"github.com/permitio/permit-golang/pkg/config" // v1: github.com/permitio/permit-golang/pkg/config
	"github.com/permitio/permit-golang/pkg/models" // v1: github.com/permitio/permit-golang/pkg/models
	"github.com/permitio/permit-golang/pkg/permit" // v1: github.com/permitio/permit-golang/pkg/permit
)

func main() {
	ctx := context.Background()
	PermitConfig := config.NewConfigBuilder("<YOUR_API_TOKEN>").Build() // v1: config.NewConfigBuilder config.PermitConfig.Build
	Permit := permit.New(PermitConfig)                                  // v1: permit.New

	newUser := models.NewUserCreate("new_user") // v1: models.NewUserCreate
	user, err := Permit.SyncUser(ctx, *newUser) // v1: permit.Client.SyncUser
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(user.Key) // v1 missed: models.UserRead.Key
}
