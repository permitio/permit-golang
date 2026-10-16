// The calls that the text of the README of v1.3.0 names outside its examples: the URL options, the
// context of an API key, the calls that use the PDP URL, and the methods under Permit.Api and
// Permit.Elements. Each line that uses v1 ends with a comment "v1:" that lists what the scanner
// must report on it.
package main

import (
	"context"
	"fmt"

	"github.com/permitio/permit-golang/pkg/config"      // v1: github.com/permitio/permit-golang/pkg/config
	"github.com/permitio/permit-golang/pkg/enforcement" // v1: github.com/permitio/permit-golang/pkg/enforcement
	"github.com/permitio/permit-golang/pkg/models"      // v1: github.com/permitio/permit-golang/pkg/models
	"github.com/permitio/permit-golang/pkg/permit"      // v1: github.com/permitio/permit-golang/pkg/permit
)

func main() {
	ctx := context.Background()
	permitConfig := config.
		NewConfigBuilder("<YOUR_API_TOKEN>"). // v1: config.NewConfigBuilder
		WithApiUrl("https://api.permit.io").  // v1: config.PermitConfig.WithApiUrl
		WithPdpUrl("http://localhost:7766").  // v1: config.PermitConfig.WithPdpUrl
		WithOpaUrl("http://localhost:8181").  // v1: config.PermitConfig.WithOpaUrl
		Build()                               // v1: config.PermitConfig.Build
	Permit := permit.New(permitConfig)                       // v1: permit.New
	Permit.Api.SetContext(ctx, "<project>", "<environment>") // v1: api.PermitApiClient.SetContext

	user := enforcement.UserBuilder("user_id").Build()                                  // v1: enforcement.UserBuilder enforcement.User.Build
	resource := enforcement.ResourceBuilder("document").Build()                         // v1: enforcement.ResourceBuilder enforcement.Resource.Build
	request := enforcement.CheckRequest{User: user, Action: "read", Resource: resource} // v1: enforcement.CheckRequest
	decisions, err := Permit.BulkCheck(request)                                         // v1: permit.Client.BulkCheck
	fmt.Println(decisions, err)
	allowed, err := Permit.FilterObjects(user, "read", nil, &resource) // v1: permit.Client.FilterObjects
	fmt.Println(allowed, err)
	tenants, err := Permit.AllTenantsCheck(user, "read", resource) // v1: permit.Client.AllTenantsCheck
	fmt.Println(tenants, err)
	permissions, err := Permit.GetUserPermissions(user, "default") // v1: permit.Client.GetUserPermissions
	fmt.Println(permissions, err)
	permissions, err = Permit.GetUserPermissionsWithOptions(user, // v1: permit.Client.GetUserPermissionsWithOptions
		enforcement.WithTenants([]string{"default"})) // v1: enforcement.WithTenants
	fmt.Println(permissions, err)

	users, err := Permit.Api.Users.List(ctx, 1, 10) // v1: api.Users.List
	fmt.Println(users, err)
	tenantList, err := Permit.Api.Tenants.List(ctx, 1, 10) // v1: api.Tenants.List
	fmt.Println(tenantList, err)
	roles, err := Permit.Api.Roles.List(ctx, 1, 10) // v1: api.Roles.List
	fmt.Println(roles, err)
	project, err := Permit.Api.Projects.Get(ctx, "<project>") // v1: api.Projects.Get
	fmt.Println(project, err)
	login := models.NewUserLoginRequestInput("user_id", "default") // v1: models.NewUserLoginRequestInput
	session, err := Permit.Elements.LoginAs(ctx, *login)           // v1: api.Elements.LoginAs
	fmt.Println(session, err)
}
