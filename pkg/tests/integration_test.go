package tests

import (
	"context"
	"fmt"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/enforcement"
	PermitErrors "github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"math/rand"
	"testing"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randKey(prefix string) string {
	const n = 10
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return fmt.Sprintf("%s-%s", string(b), prefix)
}

func TestIntegration(t *testing.T) {
	logger := zap.NewExample()
	ctx := context.Background()
	userKey := randKey("user")
	resourceKey := randKey("resource")
	roleKey := randKey("role")
	marker := randKey("marker")
	actionKey := randKey("action")
	actionGroupKey := randKey("actiongroup")

	const token = ""
	permitContext := config.NewPermitContext(config.EnvironmentAPIKeyLevel, "default-project", "golang-test")
	permitClient := permit.New(config.NewConfigBuilder(token).WithContext(permitContext).WithLogger(logger).Build())

	// Create a user
	userCreate := *models.NewUserCreate(userKey)
	userCreate.SetFirstName("John")
	userCreate.SetLastName("Doe")
	userCreate.SetEmail("john@example.com")
	_, err := permitClient.Api.Users.Create(ctx, userCreate)
	assert.NoError(t, err)

	// Check error codes when creating a user with existing name
	_, err = permitClient.Api.Users.Create(ctx, userCreate)
	assert.Error(t, err)
	permitError := err.(PermitErrors.PermitError)
	assert.Equal(t, 409, permitError.StatusCode)
	assert.Equal(t, PermitErrors.Conflict, permitError.ErrorCode)
	assert.Equal(t, PermitErrors.API_ERROR, permitError.ErrorType)

	// Create a resource
	resourceCreate := *models.NewResourceCreate(resourceKey, resourceKey, map[string]models.ActionBlockEditable{"read": {}, "write": {}})
	_, err = permitClient.Api.Resources.Create(ctx, resourceCreate)
	assert.NoError(t, err)

	actionCreate := *models.NewResourceActionCreate(actionKey, actionKey)
	actionCreate.SetAttributes(map[string]interface{}{
		"marker": marker,
	})
	_, err = permitClient.Api.ResourceActions.Create(ctx, resourceKey, actionCreate)
	assert.NoError(t, err)

	actions, err := permitClient.Api.ResourceActions.ListByAttributes(ctx, resourceKey, 1, 100, map[string]interface{}{
		"marker": marker,
	})
	assert.NoError(t, err)
	assert.Len(t, actions, 1)

	actionGroupCreate := *models.NewResourceActionGroupCreate(actionGroupKey, actionGroupKey)
	actionGroupCreate.SetAttributes(map[string]interface{}{
		"marker": marker,
	})
	_, err = permitClient.Api.ResourceActionGroups.Create(ctx, resourceKey, actionGroupCreate)
	assert.NoError(t, err)

	actionGroups, err := permitClient.Api.ResourceActions.ListByAttributes(ctx, resourceKey, 1, 100, map[string]interface{}{
		"marker": marker,
	})
	assert.NoError(t, err)
	assert.Len(t, actionGroups, 1)

	// Create a role
	permissions := []string{
		resourceKey + ":read",
		resourceKey + ":write",
	}
	roleCreate := models.NewRoleCreate(roleKey, roleKey)
	roleCreate.SetPermissions(permissions)
	_, err = permitClient.Api.Roles.Create(ctx, *roleCreate)
	assert.NoError(t, err)

	privateRoleCreate := models.NewRoleCreate(roleKey+"-private", roleKey+"-private")
	privateRoleCreate.SetPermissions(permissions)
	privateRoleCreate.SetAttributes(map[string]string{
		"marker": marker,
	})
	_, err = permitClient.Api.Roles.Create(ctx, *privateRoleCreate)
	assert.NoError(t, err)

	roles, err := permitClient.Api.Roles.ListByAttributes(ctx, 1, 100, map[string]interface{}{
		"marker": marker,
	})
	assert.NoError(t, err)
	assert.Len(t, roles, 1)

	// Assign role to user
	_, err = permitClient.Api.Users.AssignRole(ctx, userKey, roleKey, "default")
	assert.NoError(t, err)

	// Check if user has permission
	time.Sleep(6 * time.Second)

	userCheck := enforcement.UserBuilder(userKey).Build()
	resourceCheck := enforcement.ResourceBuilder(resourceKey).WithTenant("default").Build()
	allowed, err := permitClient.Check(userCheck, "read", resourceCheck)
	assert.NoError(t, err)
	assert.True(t, allowed)
}
