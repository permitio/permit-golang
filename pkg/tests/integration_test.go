package tests

import (
	"context"
	"github.com/permitio/permit-golang/models"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/enforcement"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"strings"
	"testing"
	"time"
)

func TestIntegration(t *testing.T) {
	logger := zap.NewExample()
	ctx := context.Background()
	const userKey = "test-user3"
	const resourceKey = "document3"
	const roleKey = "editor3"
	permitContext := config.NewPermitContext(config.EnvironmentAPIKeyLevel, "test", "staging")
	permitClient := permit.New(config.NewConfigBuilder("", "http://localhost:7766").WithContext(permitContext).WithLogger(logger).Build())

	// Create a user
	newUser := *models.NewUserCreate(userKey)
	newUser.SetFirstName("tesasdt")
	_, err := permitClient.SyncUser(ctx, newUser)
	if err != nil {
		t.Error(err)
	}

	// Create a resource
	_, err = permitClient.Api.Resources.Create(ctx, *models.NewResourceCreate(resourceKey, resourceKey, map[string]models.ActionBlockEditable{"read": {}, "write": {}}))
	if err != nil {
		if !strings.Contains(err.Error(), string(errors.ConflictMessage)) {
			t.Error(err)
		}
	}

	// Create a role
	permissions := []string{resourceKey + ":read", resourceKey + ":write"}
	roleCreate := models.NewRoleCreate(roleKey, roleKey)
	roleCreate.SetPermissions(permissions)
	_, err = permitClient.Api.Roles.Create(ctx, *roleCreate)
	if err != nil {
		if !strings.Contains(err.Error(), string(errors.ConflictMessage)) {
			t.Error(err)
		}
	}

	// Assign role to user
	_, err = permitClient.Api.Users.AssignRole(ctx, userKey, roleKey, "default")
	if err != nil {
		if !strings.Contains(err.Error(), string(errors.ConflictMessage)) {
			t.Error(err)
		}
	}

	// Check if user has permission
	time.Sleep(6 * time.Second)

	userCheck := enforcement.UserBuilder(userKey).Build()
	resourceCheck := enforcement.ResourceBuilder(resourceKey).WithTenant("default").Build()
	allowed, err := permitClient.Check(userCheck, "read", resourceCheck)
	if err != nil {
		t.Error(err)
	}

	assert.True(t, allowed)
}
