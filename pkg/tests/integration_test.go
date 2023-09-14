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
	"os"
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
	return fmt.Sprintf("%s-%s", prefix, string(b))
}

func checkBulk(ctx context.Context, t *testing.T, permitClient *permit.Client, roleKey, tenantKey, resourceKey, actionKey string) {
	// Bulk (un)assignments
	var users []*models.UserCreate
	var bulkAssignments []models.RoleAssignmentCreate
	var bulkUnAssignments []models.RoleAssignmentRemove

	for i := 0; i < 3; i++ {
		bulkUserKey := randKey("user")
		bulkUserCreate := models.NewUserCreate(bulkUserKey)
		users = append(users, models.NewUserCreate(bulkUserKey))
		bulkAssignments = append(bulkAssignments, *models.NewRoleAssignmentCreate(roleKey, tenantKey, bulkUserKey))
		bulkUnAssignments = append(bulkUnAssignments, *models.NewRoleAssignmentRemove(roleKey, tenantKey, bulkUserKey))

		_, err := permitClient.Api.Users.Create(ctx, *bulkUserCreate)
		assert.NoError(t, err)
	}

	assignReport, err := permitClient.Api.Roles.BulkAssignRole(ctx, bulkAssignments)
	assert.NoError(t, err)
	assert.EqualValues(t, 3, *assignReport.AssignmentsCreated)

	for _, u := range users {
		assigned, err := permitClient.Api.Users.GetAssignedRoles(ctx, u.Key, tenantKey, 1, 100)
		assert.NoError(t, err)

		assert.Equal(t, tenantKey, assigned[0].Tenant)
		assert.Equal(t, roleKey, assigned[0].Role)
	}

	time.Sleep(6 * time.Second)
	requests := make([]enforcement.CheckRequest, len(bulkAssignments)+1)
	for i, assignment := range bulkAssignments {
		requests[i] = enforcement.CheckRequest{
			User:     enforcement.UserBuilder(assignment.User).Build(),
			Action:   enforcement.Action(actionKey),
			Resource: enforcement.ResourceBuilder(resourceKey).WithTenant(assignment.Tenant).Build(),
			Context:  nil,
		}
	}
	requests[len(bulkAssignments)] = enforcement.CheckRequest{
		User:     enforcement.UserBuilder(users[0].Key).Build(),
		Action:   "non-existing-action",
		Resource: enforcement.ResourceBuilder(resourceKey).WithTenant(tenantKey).Build(),
		Context:  nil,
	}
	results, _ := permitClient.BulkCheck(requests...)
	assert.Len(t, results, len(bulkAssignments)+1)
	for i := 0; i <= len(bulkAssignments); i++ {
		if i == len(bulkAssignments) {
			assert.False(t, results[i])
		} else {
			assert.True(t, results[i])
		}
	}
	unassignReport, err := permitClient.Api.Roles.BulkUnAssignRole(ctx, bulkUnAssignments)
	assert.NoError(t, err)
	assert.EqualValues(t, 3, *unassignReport.AssignmentsRemoved)
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
	tenantKey := randKey("tenant")
	secondTenantKey := randKey("tenant")

	token := os.Getenv("PDP_API_KEY")
	if token == "" {
		t.Fatal("PDP_API_KEY is not set")
	}
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

	// Create resources
	resourceCreate := *models.NewResourceCreate(resourceKey, resourceKey,
		map[string]models.ActionBlockEditable{
			"read":  {Attributes: map[string]interface{}{"marker": marker}},
			"write": {Attributes: map[string]interface{}{"marker": marker}},
		})
	resourceCreate.SetAttributes(map[string]models.AttributeBlockEditable{
		"secret": *models.NewAttributeBlockEditable(models.BOOL),
	})
	_, err = permitClient.Api.Resources.Create(ctx, resourceCreate)
	assert.NoError(t, err)

	resourceCreate = *models.NewResourceCreate(resourceKey+"-2", resourceKey+"-2",
		map[string]models.ActionBlockEditable{
			"read":  {Attributes: map[string]interface{}{"marker": marker}},
			"write": {Attributes: map[string]interface{}{"marker": marker}},
		})
	_, err = permitClient.Api.Resources.Create(ctx, resourceCreate)
	assert.NoError(t, err)

	list, err := permitClient.Api.Resources.Search(ctx, 1, 100, resourceKey)
	assert.NoError(t, err)
	assert.Len(t, list, 2)

	list, err = permitClient.Api.Resources.Search(ctx, 1, 100, resourceKey+"*")
	assert.NoError(t, err)
	assert.Len(t, list, 2)

	list, err = permitClient.Api.Resources.Search(ctx, 1, 100, resourceKey+"-*")
	assert.NoError(t, err)
	assert.Len(t, list, 1)

	list, err = permitClient.Api.Resources.Search(ctx, 1, 100, resourceKey+"_*")
	assert.NoError(t, err)
	assert.Len(t, list, 0)

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
	assert.Len(t, actions, 3)

	for _, action := range actions {
		assert.Equal(t, action.Attributes["marker"], marker)
	}

	actionGroupCreate := *models.NewResourceActionGroupCreate(actionGroupKey, actionGroupKey)
	actionGroupCreate.SetAttributes(map[string]interface{}{
		"marker": marker,
	})
	_, err = permitClient.Api.ResourceActionGroups.Create(ctx, resourceKey, actionGroupCreate)
	assert.NoError(t, err)

	actionGroups, err := permitClient.Api.ResourceActionGroups.ListByAttributes(ctx, resourceKey, 1, 100, map[string]interface{}{
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
	privateRoleCreate.SetExtends([]string{roleCreate.Key})
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
	assert.Equal(t, []string{roleCreate.Key}, roles[0].Extends)

	tenantCreate := models.NewTenantCreate(tenantKey, tenantKey)
	tenantCreate.SetAttributes(map[string]interface{}{"marker": marker})
	_, err = permitClient.Api.Tenants.Create(ctx, *tenantCreate)
	assert.NoError(t, err)

	secondTenantCreate := models.NewTenantCreate(secondTenantKey, secondTenantKey)
	secondTenantCreate.SetAttributes(map[string]interface{}{"isSecond": true})
	_, err = permitClient.Api.Tenants.Create(ctx, *secondTenantCreate)
	assert.NoError(t, err)

	tenants, err := permitClient.Api.Tenants.ListByAttributes(ctx, map[string]interface{}{
		"marker": marker,
	}, 1, 100)
	assert.NoError(t, err)
	assert.Len(t, tenants, 1)

	// Assign role to user
	_, err = permitClient.Api.Users.AssignRole(ctx, userKey, roleKey, tenantKey)
	assert.NoError(t, err)

	detailedRAs, err := permitClient.Api.RoleAssignments.ListDetailed(ctx, 1, 100, userKey, roleKey, tenantKey)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(*detailedRAs))
	checkBulk(ctx, t, permitClient, roleKey, tenantKey, resourceKey, "read")

	resource, err := permitClient.Api.Resources.GetByKey(ctx, resourceKey)
	assert.NoError(t, err)

	csCreate := *models.NewConditionSetCreate(csKey, csKey)
	csCreate.SetType(models.RESOURCESET)
	csCreate.SetResourceId(models.ResourceId{String: &resource.Id})
	csCreate.SetConditions(map[string]interface{}{
		"allOf": []map[string]interface{}{
			{"resource.secret": map[string]interface{}{
				"equals": true,
			}},
		},
	})

	_, err = permitClient.Api.ConditionSets.Create(ctx, csCreate)
	assert.NoError(t, err)

	csUpdate := *models.NewConditionSetUpdate()
	csUpdate.SetDescription("Top Secrets")
	cs, err := permitClient.Api.ConditionSets.Update(ctx, csKey, csUpdate)
	assert.NoError(t, err)
	assert.Equal(t, "Top Secrets", *cs.Description)

	//// Check if user has permission
	time.Sleep(6 * time.Second)

	userCheck := enforcement.UserBuilder(userKey).Build()
	resourceCheck := enforcement.ResourceBuilder(resourceKey).WithTenant(tenantKey).Build()
	allowed, err := permitClient.Check(userCheck, "read", resourceCheck)
	assert.NoError(t, err)
	assert.True(t, allowed)

	allowedTenants, err := permitClient.AllTenantsCheck(
		userCheck,
		"read",
		resourceCheck.WithTenant("").Build(),
	)
	assert.Len(t, allowedTenants, 1)
	assert.Equal(t, tenantKey, allowedTenants[0].Key)
	assert.True(t, assert.ObjectsAreEqualValues(allowedTenants[0].Attributes, tenantCreate.Attributes))
}
