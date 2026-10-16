// The example "Check for permissions" of the README of v1.3.0. Each line that uses v1 ends with a
// comment "v1:" that lists what the scanner must report on it.
package main

import (
	"fmt"

	"github.com/permitio/permit-golang/pkg/config"      // v1: github.com/permitio/permit-golang/pkg/config
	"github.com/permitio/permit-golang/pkg/enforcement" // v1: github.com/permitio/permit-golang/pkg/enforcement
	"github.com/permitio/permit-golang/pkg/permit"      // v1: github.com/permitio/permit-golang/pkg/permit
)

func main() {
	permitConfig := config.NewConfigBuilder("<YOUR_API_TOKEN>").Build() // v1: config.NewConfigBuilder config.PermitConfig.Build
	Permit := permit.New(permitConfig)                                  // v1: permit.New

	user := enforcement.UserBuilder("user_id").Build()               // v1: enforcement.UserBuilder enforcement.User.Build
	resource := enforcement.ResourceBuilder("resource_type").Build() // v1: enforcement.ResourceBuilder enforcement.Resource.Build
	permitted, err := Permit.Check(user, "read", resource)           // v1: permit.Client.Check
	if err != nil {
		// The check failed, for example because the PDP could not be reached; this is not a deny.
		fmt.Println(err)
		return
	}
	if permitted {
		// Let the user read the resource
	} else {
		// Deny access
	}
}
