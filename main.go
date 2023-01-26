package main

import (
	"context"
	"fmt"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
)

func main() {
	ctx := context.Background()
	PermitConfig := config.NewConfigBuilder("<YOUR_API_TOKEN>").Build()
	Permit := permit.New(PermitConfig)

	newUser := models.NewUserCreate("new_user")
	user, _ := Permit.SyncUser(ctx, *newUser)
	fmt.Println(user.Key)
}
