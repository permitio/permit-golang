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
	"reflect"
	"testing"
	"time"
)

var runId = randId()

func init() {
	rand.Seed(time.Now().UnixNano())
	println("Run ID: ", runId)
}

type MyResource struct {
	UniqueID     string
	Type         string
	Organization string
}

func (m MyResource) GetID() string {
	return m.UniqueID
}

func (m MyResource) GetType() string {
	if m.Type != "" {
		return m.Type
	}
	if t := reflect.TypeOf(m); t.Kind() == reflect.Ptr {
		return t.Elem().Name()
	} else {
		return t.Name()
	}
}

func (m MyResource) GetTenant() string {
	return m.Organization
}

func (m MyResource) GetAttributes() map[string]interface{} {
	return make(map[string]interface{})
}

func (m MyResource) GetContext() map[string]string {
	return make(map[string]string)
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randId() string {
	const n = 10
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func randKey(postfix string) string {
	return runId + "-" + postfix
}

func checkBulk(ctx context.Context, t *testing.T, permitClient *permit.Client, roleKey, tenantKey, resourceKey, actionKey string) {
	// Bulk (un)assignments
	var users []*models.UserCreate
	var bulkAssignments []models.RoleAssignmentCreate
	var bulkUnAssignments []models.RoleAssignmentRemove

	for i := 0; i < 3; i++ {
		bulkUserKey := randKey(fmt.Sprintf("bulkuser-%d", i))
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
	requests := make([]enforcement.CheckRequest, len(bulkAssignments))
	for i, assignment := range bulkAssignments {
		var tenant string
		if i%2 == 0 {
			tenant = assignment.Tenant
		} else {
			tenant = "non-existing-tenant"
		}
		requests[i] = enforcement.CheckRequest{
			User:     enforcement.UserBuilder(assignment.User).Build(),
			Action:   enforcement.Action(actionKey),
			Resource: enforcement.ResourceBuilder(resourceKey).WithTenant(tenant).Build(),
			Context:  nil,
		}
	}

	results, err := permitClient.BulkCheck(requests...)
	assert.NoError(t, err)
	assert.Len(t, results, len(bulkAssignments))
	for i := 0; i < len(bulkAssignments); i++ {
		if i%2 == 0 {
			assert.True(t, results[i])
		} else {
			assert.False(t, results[i])
		}
	}
	unassignReport, err := permitClient.Api.Roles.BulkUnAssignRole(ctx, bulkUnAssignments)
	assert.NoError(t, err)
	assert.EqualValues(t, 3, *unassignReport.AssignmentsRemoved)
}

func factsApi(ctx context.Context, t *testing.T, permitContext *config.PermitContext, logger *zap.Logger, token string) {
	permitClient := permit.New(config.NewConfigBuilder(token).
		WithPdpUrl(os.Getenv("PDP_URL")).
		WithApiUrl(os.Getenv("API_URL")).
		WithContext(permitContext).
		WithLogger(logger).
		WithProxyFactsViaPDP(true).
		WithFactsSyncTimeout(10 * time.Second).
		Build())

	resourceKey := randKey("resource")
	resourceCreate := *models.NewResourceCreate(resourceKey, resourceKey,
		map[string]models.ActionBlockEditable{
			"read": {Attributes: map[string]interface{}{"marker": "marker"}},
		})
	_, err := permitClient.Api.Resources.Create(ctx, resourceCreate)
	assert.NoError(t, err)

	roleKey := randKey("role")
	roleCreate := models.NewRoleCreate(roleKey, roleKey)
	roleCreate.SetPermissions([]string{fmt.Sprintf("%s:read", resourceKey)})
	_, err = permitClient.Api.Roles.Create(ctx, *roleCreate)
	assert.NoError(t, err)

	userKey := randKey("user")
	userCreate := *models.NewUserCreate(userKey)
	userCreate.SetFirstName("John")
	userCreate.SetLastName("Doe")
	userCreate.SetEmail("john@example.com")
	_, err = permitClient.Api.Users.Create(ctx, userCreate)
	assert.NoError(t, err)

	_, err = permitClient.Api.Users.AssignRole(ctx, userKey, roleKey, "default")
	assert.NoError(t, err)
	// check if user has permission immediately
	allowed, err := permitClient.Check(enforcement.UserBuilder(userKey).Build(), "read", enforcement.ResourceBuilder(resourceKey).Build())
	assert.NoError(t, err)
	assert.True(t, allowed)
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
	tenantKey := randKey("tenant-1")
	secondTenantKey := randKey("tenant-2")
	resourceSetKey := randKey("resourceset")
	userSetKey := randKey("userset")
	proxyConfigKey := randKey("proxyconfig")

	project := os.Getenv("PROJECT")

	if project == "" {
		t.Fatal("PROJECT is not set")
	}

	env := os.Getenv("ENV")

	if env == "" {
		t.Fatal("ENV is not set")
	}

	token := os.Getenv("PDP_API_KEY")
	if token == "" {
		t.Fatal("PDP_API_KEY is not set")
	}
	permitContext := config.NewPermitContext(config.EnvironmentAPIKeyLevel, project, env)
	permitClient := permit.New(config.NewConfigBuilder(token).
		WithPdpUrl(os.Getenv("PDP_URL")).
		WithApiUrl(os.Getenv("API_URL")).
		WithContext(permitContext).
		WithLogger(logger).
		Build())

	// Test Facts API
	factsApi(ctx, t, permitContext, logger, token)

	// Create a user
	userCreate := *models.NewUserCreate(userKey)
	userCreate.SetFirstName("John")
	userCreate.SetLastName("Doe")
	userCreate.SetEmail("john@example.com")
	_, err := permitClient.Api.Users.Create(ctx, userCreate)
	assert.NoError(t, err)
	res, err := permitClient.Api.Users.GetAssignedRoles(ctx, userKey, tenantKey, 1, 100)
	assert.NoError(t, err)
	assert.Len(t, res, 0)
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
	resourceRead, err := permitClient.Api.Resources.Create(ctx, resourceCreate)
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
	time.Sleep(30 * time.Second)

	// Testing List Tenants Users
	// Note - Dependent on the user creation above -- consider decoupling this test from the user creation

	// Test 1: Basic functionality - tenant with assigned user
	tenantUsers, err := permitClient.Api.Tenants.ListTenantUsers(ctx, tenantKey, 1, 100)
	assert.NoError(t, err)
	assert.Len(t, tenantUsers, 1)
	assert.Equal(t, userKey, tenantUsers[0].GetKey())

	// Test 2: Verify user object properties
	assert.Greater(t, len(tenantUsers), 0, "Should have at least one user")
	user := tenantUsers[0]
	assert.NotEmpty(t, user.GetKey())
	assert.NotEmpty(t, user.GetEmail())
	// Note - Dependent on the user creation above -- consider decoupling this test from the user creation
	assert.Equal(t, userKey, user.GetKey())
	assert.Equal(t, "John", user.GetFirstName())
	assert.Equal(t, "Doe", user.GetLastName())
	assert.Equal(t, "john@example.com", user.GetEmail())

	// Test 3: Function signature verification
	assert.IsType(t, []models.UserRead{}, tenantUsers, "Return type should be []models.UserRead")

	// Test 4: Pagination test
	page1Users, err := permitClient.Api.Tenants.ListTenantUsers(ctx, tenantKey, 1, 1)
	assert.NoError(t, err)
	assert.Len(t, page1Users, 1)
	assert.Equal(t, userKey, page1Users[0].GetKey())
	
	// Page 2 should be empty since we only have 1 user
	page2Users, err := permitClient.Api.Tenants.ListTenantUsers(ctx, tenantKey, 2, 1)
	assert.NoError(t, err)
	assert.Len(t, page2Users, 0)

	// Test 5: Edge cases - invalid pagination values
	_, err = permitClient.Api.Tenants.ListTenantUsers(ctx, tenantKey, 0, 10)
	assert.Error(t, err, "page=0 should return an error")
	
	_, err = permitClient.Api.Tenants.ListTenantUsers(ctx, tenantKey, 1, 0)
	assert.Error(t, err, "limit=0 should return an error")

	// Test 6: Test with different page sizes
	allUsers, err := permitClient.Api.Tenants.ListTenantUsers(ctx, tenantKey, 1, 100)
	assert.NoError(t, err)
	smallPageUsers, err := permitClient.Api.Tenants.ListTenantUsers(ctx, tenantKey, 1, 1)
	assert.NoError(t, err)
	assert.LessOrEqual(t, len(smallPageUsers), len(allUsers), "Small page should have <= users than full page")

	// Test 7: Verify consistency across calls
	secondCall, err := permitClient.Api.Tenants.ListTenantUsers(ctx, tenantKey, 1, 100)
	assert.NoError(t, err)
	assert.Equal(t, len(tenantUsers), len(secondCall), "Multiple calls should return same number of users")
	if len(tenantUsers) > 0 && len(secondCall) > 0 {
		assert.Equal(t, tenantUsers[0].GetKey(), secondCall[0].GetKey(), "Same user should be returned")
	}

	userPermissions, err := permitClient.GetUserPermissions(enforcement.UserBuilder(userKey).Build())
	assert.NoError(t, err)
	userPermissionsInTenant, found := userPermissions["__tenant:"+tenantKey]
	assert.True(t, found)
	assert.Equal(t, tenantKey, userPermissionsInTenant.Tenant.Key)
	assert.True(t, assert.ObjectsAreEqual(tenantCreate.Attributes, userPermissionsInTenant.Tenant.Attributes))
	assert.ElementsMatch(t, userPermissionsInTenant.Permissions, permissions)

	detailedRAs, err := permitClient.Api.RoleAssignments.ListDetailed(ctx, 1, 100, userKey, roleKey, tenantKey)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(*detailedRAs))
	checkBulk(ctx, t, permitClient, roleKey, tenantKey, resourceKey, "read")

	userSetCreate := *models.NewConditionSetCreate(userSetKey, userSetKey)
	userSetCreate.SetType(models.USERSET)
	userSetCreate.SetConditions(map[string]interface{}{
		"allOf": []map[string]interface{}{
			{
				"allOf": []map[string]interface{}{
					{"subject.email": map[string]interface{}{
						"contains": "@admin",
					}},
				},
			},
		},
	})

	_, err = permitClient.Api.ConditionSets.Create(ctx, userSetCreate)
	assert.NoError(t, err)

	resourceSet := *models.NewConditionSetCreate(resourceSetKey, resourceSetKey)
	resourceSet.SetType(models.RESOURCESET)
	resourceSet.SetResourceId(models.ResourceId{String: &resourceRead.Id})
	resourceSet.SetConditions(map[string]interface{}{
		"allOf": []map[string]interface{}{
			{
				"allOf": []map[string]interface{}{
					{"resource.secret": map[string]interface{}{
						"equals": true,
					}},
				},
			},
		},
	})

	_, err = permitClient.Api.ConditionSets.Create(ctx, resourceSet)
	assert.NoError(t, err)

	csUpdate := *models.NewConditionSetUpdate()
	csUpdate.SetDescription("Top Secrets")
	cs, err := permitClient.Api.ConditionSets.Update(ctx, resourceSetKey, csUpdate)
	assert.NoError(t, err)
	assert.Equal(t, "Top Secrets", *cs.Description)

	_, err = permitClient.Api.ConditionSets.AssignSetPermissions(ctx, userSetKey, resourceKey+":"+actionKey, resourceSetKey)
	assert.NoError(t, err)

	rules, err := permitClient.Api.ConditionSets.ListSetPermissions(ctx, userSetKey, actionKey, resourceSetKey)
	assert.NoError(t, err)
	assert.Len(t, rules, 1)

	//// Check if user has permission
	time.Sleep(6 * time.Second)

	userCheck := enforcement.UserBuilder(userKey).Build()
	resourceCheck := enforcement.ResourceBuilder(resourceKey).WithTenant(tenantKey).Build()
	allowed, err := permitClient.Check(userCheck, "read", resourceCheck)
	assert.NoError(t, err)
	assert.True(t, allowed)

	myResources := []enforcement.ResourceI{
		MyResource{
			UniqueID:     "my-random-id",
			Organization: tenantKey,
			Type:         resourceKey,
		},
		MyResource{
			UniqueID:     "my-random-id-2",
			Organization: tenantKey,
		},
	}
	filteredResources, err := permitClient.FilterObjects(userCheck, "read", nil, myResources...)
	assert.NoError(t, err)
	assert.Len(t, filteredResources, 1)
	assert.True(t, assert.ObjectsAreEqual(&filteredResources[0], &myResources[0]))

	allowedTenants, err := permitClient.AllTenantsCheck(
		userCheck,
		"read",
		resourceCheck.WithTenant("").Build(),
	)
	assert.Len(t, allowedTenants, 1)
	assert.Equal(t, tenantKey, allowedTenants[0].Key)
	assert.True(t, assert.ObjectsAreEqualValues(allowedTenants[0].Attributes, tenantCreate.Attributes))

	// Create a Proxy Config
	mappingRules := []models.MappingRule{}
	action := "read"
	mappingRules = append(mappingRules, models.MappingRule{
		Url:        "https://asdfasdf.com",
		HttpMethod: "delete",
		Resource:   resourceKey,
		Action:     &action,
	})
	secret := "user:pass"
	proxyConfigCreate := *models.NewProxyConfigCreate(secret, proxyConfigKey, proxyConfigKey)
	proxyConfigCreate.SetMappingRules(mappingRules)
	_, err = permitClient.Api.ProxyConfigs.Create(ctx, proxyConfigCreate)
	assert.NoError(t, err)
	proxyConfigUpdate := models.NewProxyConfigUpdate()
	mappingRules = append(mappingRules, models.MappingRule{
		Url:        "https://mushy.com",
		HttpMethod: "post",
		Resource:   resourceKey,
		Action:     &action,
	})
	authMechanism := models.BASIC
	proxyConfigUpdate.SetAuthMechanism(authMechanism)
	proxyConfigUpdate.SetSecret(secret)
	proxyConfigUpdate.SetMappingRules(mappingRules)
	_, err = permitClient.Api.ProxyConfigs.Update(ctx, "pxcf", *proxyConfigUpdate)
}
