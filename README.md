# Go SDK for Permit.io

Instructions for installing Go SDK for interacting with Permit.io.

## Overview

This guide will walk you through the steps of installing the Permit.io Go SDK and integrating it into your code.

## Installation

Install the SDK using the following command:

```shell
go get github.com/permitio/permit-golang
```

Put the package under your project folder and add the following in import:

```golang
import "github.com/permitio/permit-golang/pkg/permit"
```

## Usage

### Init the SDK
To init the SDK, you need to create a new Permit client with the API key you got from the Permit.io dashboard.

First we will create a new Config object so we can pass it to the Permit client.

Second, we will create a new Permit client with the Config object we created.

```golang
package main

import "github.com/permitio/permit-golang/pkg/permit"
import "github.com/permitio/permit-golang/pkg/config"

func main() {
	PermitConfig := config.NewConfigBuilder("<YOUR_API_TOKEN>").Build()
	Permit := permit.New(PermitConfig)
}
```

### Check for permissions
To check permissions using our `Permit.Check()` method, you will have to create a new User and Resource models using the `enforcement` models
The models are located in `github.com/permitio/permit-golang/pkg/enforcement`

Follow the example below:

```golang
package main

import "github.com/permitio/permit-golang/pkg/permit"
import "github.com/permitio/permit-golang/pkg/config"
import "github.com/permitio/permit-golang/pkg/enforcement"

func main() {
	permitConfig := config.NewConfigBuilder("<YOUR_API_TOKEN>").Build()
	Permit := permit.New(permitConfig)

	user := enforcement.UserBuilder("user_id").Build()
	resource := enforcement.ResourceBuilder("resource_id").Build()
	permitted, err := Permit.Check(user, "read", resource)
	if err != nil {
		return
	}
	if permitted {
		// Let the user read the resource
	} else {
		// Deny access
	}
}
```

### Sync users
To sync users to the Permit.io API, using the `Permit.SyncUsers()` method,
you will have to create a User model using our main Models package.
The models are located in `github.com/permitio/permit-golang/pkg/models`

Follow the example below:
```golang
package main

import (
	"additionalContext"
	"fmt"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
)

func main() {
	ctx := additionalContext.Background()
	PermitConfig := config.NewConfigBuilder("<YOUR_API_TOKEN>").Build()
	Permit := permit.New(PermitConfig)

	newUser := models.NewUserCreate("new_user")
	user, _ := Permit.SyncUser(ctx, *newUser)
	fmt.Println(user.Key)
}
```
