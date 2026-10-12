//go:build integration

// Package tests holds integration tests that call a live Permit environment and a PDP connected
// to it. Run them with
//
//	go test -tags integration ./pkg/tests
//
// They read PROJECT, ENV, PDP_API_KEY (an API key of that environment), PDP_URL and API_URL from
// the environment. The key of every object they create starts with PERMIT_TEST_RUN_PREFIX and a
// hyphen, or with a random prefix that they log when it is not set. Each test deletes what it
// created when it ends; ./cleanup deletes what a run could not.
package tests

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/permitio/permit-golang/internal/testcleanup"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/enforcement"
	PermitErrors "github.com/permitio/permit-golang/pkg/errors"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

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

func checkBulk(
	ctx context.Context, t *testing.T, permitClient *permit.Client, k keys,
	roleKey, tenantKey, resourceKey, actionKey string,
) {
	// Bulk (un)assignments
	var bulkAssignments []models.RoleAssignmentCreate
	var bulkUnAssignments []models.RoleAssignmentRemove

	for i := 0; i < 3; i++ {
		bulkUserKey := k.of(fmt.Sprintf("bulkuser-%d", i))
		createUser(t, ctx, permitClient, *models.NewUserCreate(bulkUserKey))
		bulkAssignments = append(bulkAssignments, *models.NewRoleAssignmentCreate(roleKey, tenantKey, bulkUserKey))
		bulkUnAssignments = append(bulkUnAssignments, *models.NewRoleAssignmentRemove(roleKey, tenantKey, bulkUserKey))
	}

	assignReport, err := permitClient.Api.Roles.BulkAssignRole(ctx, bulkAssignments)
	require.NoError(t, err)
	unassign := func(ctx context.Context) error {
		_, err := permitClient.Api.Roles.BulkUnAssignRole(ctx, bulkUnAssignments)
		return err
	}
	unassignedByTest := deleteAtEnd(t, "the bulk role assignments", unassign)
	require.NotNil(t, assignReport.AssignmentsCreated)
	assert.EqualValues(t, 3, *assignReport.AssignmentsCreated)

	for _, assignment := range bulkAssignments {
		assigned, err := permitClient.Api.Users.GetAssignedRoles(ctx, assignment.User, tenantKey,
			1, 100)
		require.NoError(t, err)
		require.Len(t, assigned, 1, "roles of %s", assignment.User)
		assert.Equal(t, tenantKey, assigned[0].Tenant)
		assert.Equal(t, roleKey, assigned[0].Role)
	}

	// Type-level checks: no ID or key, so the PDP decides by the user's roles in the resource's
	// tenant. Every user has the role in tenantKey, and the odd one is checked in a tenant that
	// does not exist, where it has no role.
	missingTenant := k.of("missing-tenant")
	requests := make([]enforcement.CheckRequest, len(bulkAssignments))
	want := make([]bool, len(bulkAssignments))
	for i, assignment := range bulkAssignments {
		tenant := assignment.Tenant
		want[i] = i%2 == 0
		if !want[i] {
			tenant = missingTenant
		}
		requests[i] = enforcement.CheckRequest{
			User:     enforcement.UserBuilder(assignment.User).Build(),
			Action:   enforcement.Action(actionKey),
			Resource: enforcement.ResourceBuilder(resourceKey).WithTenant(tenant).Build(),
			Context:  nil,
		}
	}
	waitFor(t, "BulkCheck answers "+fmt.Sprint(want), func() (bool, string) {
		results, err := permitClient.BulkCheck(requests...)
		return err == nil && reflect.DeepEqual(want, results),
			fmt.Sprintf("results %v, error %v", results, err)
	})

	unassignReport, err := permitClient.Api.Roles.BulkUnAssignRole(ctx, bulkUnAssignments)
	require.NoError(t, err)
	unassignedByTest()
	require.NotNil(t, unassignReport.AssignmentsRemoved)
	assert.EqualValues(t, 3, *unassignReport.AssignmentsRemoved)
}

// TestFactsIntegration checks that a role assignment made through the API reaches the PDP.
func TestFactsIntegration(t *testing.T) {
	s := loadSettings(t)
	k := newKeys(t, "facts")
	ctx := context.Background()
	permitClient := newClient(s)

	resourceKey := k.of("resource")
	createResource(t, ctx, permitClient, *models.NewResourceCreate(resourceKey, resourceKey,
		map[string]models.ActionBlockEditable{
			"read": {Attributes: map[string]interface{}{"marker": "marker"}},
		}))

	roleKey := k.of("role")
	roleCreate := models.NewRoleCreate(roleKey, roleKey)
	roleCreate.SetPermissions([]string{fmt.Sprintf("%s:read", resourceKey)})
	createRole(t, ctx, permitClient, *roleCreate)

	userKey := k.of("user")
	userCreate := *models.NewUserCreate(userKey)
	userCreate.SetFirstName("John")
	userCreate.SetLastName("Doe")
	userCreate.SetEmail("john@example.com")
	createUser(t, ctx, permitClient, userCreate)

	// Every environment has the tenant "default"; the test assigns a role in it, and does not
	// create or delete it.
	assignRole(t, ctx, permitClient, *models.NewRoleAssignmentCreate(roleKey, "default", userKey))
	// A type-level check in the tenant "default", which Build sets when no tenant is given.
	waitForAllowed(t, permitClient, userKey, "read",
		enforcement.ResourceBuilder(resourceKey).Build())
}

// TestIntegration runs the API methods for users, resources, actions, roles, tenants, role
// assignments, condition sets and proxy configs, and checks RBAC decisions in the PDP.
func TestIntegration(t *testing.T) {
	s := loadSettings(t)
	k := newKeys(t, "rbac")
	ctx := context.Background()
	userKey := k.of("user")
	resourceKey := k.of("resource")
	roleKey := k.of("role")
	marker := k.of("marker")
	actionKey := k.of("action")
	actionGroupKey := k.of("actiongroup")
	tenantKey := k.of("tenant-1")
	secondTenantKey := k.of("tenant-2")
	resourceSetKey := k.of("resourceset")
	userSetKey := k.of("userset")
	proxyConfigKey := k.of("proxyconfig")
	permitClient := newClient(s)

	// Create a user
	userCreate := *models.NewUserCreate(userKey)
	userCreate.SetFirstName("John")
	userCreate.SetLastName("Doe")
	userCreate.SetEmail("john@example.com")
	createUser(t, ctx, permitClient, userCreate)
	res, err := permitClient.Api.Users.GetAssignedRoles(ctx, userKey, tenantKey, 1, 100)
	assert.NoError(t, err)
	assert.Len(t, res, 0)
	// Check error codes when creating a user with existing name
	_, err = permitClient.Api.Users.Create(ctx, userCreate)
	var permitError PermitErrors.PermitError
	require.True(t, errors.As(err, &permitError), "creating the user again: %v", err)
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
	resourceRead := createResource(t, ctx, permitClient, resourceCreate)

	resourceCreate = *models.NewResourceCreate(resourceKey+"-2", resourceKey+"-2",
		map[string]models.ActionBlockEditable{
			"read":  {Attributes: map[string]interface{}{"marker": marker}},
			"write": {Attributes: map[string]interface{}{"marker": marker}},
		})
	createResource(t, ctx, permitClient, resourceCreate)

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
	require.NoError(t, err)
	deleteAtEnd(t, "action "+actionKey, func(ctx context.Context) error {
		return permitClient.Api.ResourceActions.Delete(ctx, resourceKey, actionKey)
	})

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
	require.NoError(t, err)
	actionGroupDeletedByTest := deleteAtEnd(t, "action group "+actionGroupKey,
		func(ctx context.Context) error {
			return permitClient.Api.ResourceActionGroups.Delete(ctx, resourceKey, actionGroupKey)
		})

	// The SDK does not send the attribute filter of action groups, so this lists every group of
	// the resource: the one created above.
	actionGroups, err := permitClient.Api.ResourceActionGroups.ListByAttributes(ctx, resourceKey, 1, 100, map[string]interface{}{
		"marker": marker,
	})
	assert.NoError(t, err)
	assert.Len(t, actionGroups, 1)

	// ResourceActionGroups.Delete deletes the action group, and leaves the resource's actions.
	// Before v1.3.0 it sent the request to the route of the action with the group's key, which
	// does not exist, so it failed and the group stayed.
	err = permitClient.Api.ResourceActionGroups.Delete(ctx, resourceKey, actionGroupKey)
	require.NoError(t, err)
	actionGroupDeletedByTest()
	waitFor(t, "the deleted action group is not found", func() (bool, string) {
		_, err := permitClient.Api.ResourceActionGroups.Get(ctx, resourceKey, actionGroupKey)
		return testcleanup.IsNotFound(err), fmt.Sprintf("error %v", err)
	})
	actions, err = permitClient.Api.ResourceActions.List(ctx, resourceKey, 1, 100)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"read", "write", actionKey},
		keysOf(actions, func(a models.ResourceActionRead) string { return a.Key }))

	// Create a role
	permissions := []string{
		resourceKey + ":read",
		resourceKey + ":write",
	}
	roleCreate := models.NewRoleCreate(roleKey, roleKey)
	roleCreate.SetPermissions(permissions)
	createRole(t, ctx, permitClient, *roleCreate)

	privateRoleCreate := models.NewRoleCreate(roleKey+"-private", roleKey+"-private")
	privateRoleCreate.SetExtends([]string{roleCreate.Key})
	privateRoleCreate.SetPermissions(permissions)
	privateRoleCreate.SetAttributes(map[string]string{
		"marker": marker,
	})
	createRole(t, ctx, permitClient, *privateRoleCreate)

	roles, err := permitClient.Api.Roles.ListByAttributes(ctx, 1, 100, map[string]interface{}{
		"marker": marker,
	})
	assert.NoError(t, err)
	if assert.Len(t, roles, 1) {
		assert.Equal(t, []string{roleCreate.Key}, roles[0].Extends)
	}

	tenantCreate := models.NewTenantCreate(tenantKey, tenantKey)
	tenantCreate.SetAttributes(map[string]interface{}{"marker": marker})
	createTenant(t, ctx, permitClient, *tenantCreate)

	secondTenantCreate := models.NewTenantCreate(secondTenantKey, secondTenantKey)
	secondTenantCreate.SetAttributes(map[string]interface{}{"isSecond": true})
	createTenant(t, ctx, permitClient, *secondTenantCreate)

	// ListByAttributes returns only the tenant with the marker, and List, read page by page,
	// returns both tenants.
	tenants, err := permitClient.Api.Tenants.ListByAttributes(ctx, map[string]interface{}{
		"marker": marker,
	}, 1, 100)
	assert.NoError(t, err)
	tenantKeyOf := func(t models.TenantRead) string { return t.Key }
	assert.Equal(t, []string{tenantKey}, keysOf(tenants, tenantKeyOf))
	allTenants := listAll(t, "tenants", func(page, perPage int) ([]models.TenantRead, error) {
		return permitClient.Api.Tenants.List(ctx, page, perPage)
	})
	assert.Subset(t, keysOf(allTenants, tenantKeyOf), []string{tenantKey, secondTenantKey})

	// Assign role to user
	assignRole(t, ctx, permitClient, *models.NewRoleAssignmentCreate(roleKey, tenantKey, userKey))

	// Testing List Tenants Users
	// Note - Dependent on the user creation above -- consider decoupling this test from the user creation

	// Test 1: Basic functionality - tenant with assigned user
	waitFor(t, "the tenant lists its user", func() (bool, string) {
		tenantUsers, err := permitClient.Api.Tenants.ListTenantUsers(ctx, tenantKey, 1, 100)
		got := keysOf(tenantUsers, func(u models.UserRead) string { return u.Key })
		return err == nil && reflect.DeepEqual([]string{userKey}, got),
			fmt.Sprintf("users %v, error %v", got, err)
	})

	var userPermissions enforcement.UserPermissions
	waitFor(t, "the PDP has the user's permissions in "+tenantKey, func() (bool, string) {
		userPermissions, err = permitClient.GetUserPermissions(
			enforcement.UserBuilder(userKey).Build())
		_, found := userPermissions["__tenant:"+tenantKey]
		return err == nil && found, fmt.Sprintf("permissions %v, error %v", userPermissions, err)
	})
	userPermissionsInTenant := userPermissions["__tenant:"+tenantKey]
	assert.Equal(t, tenantKey, userPermissionsInTenant.Tenant.Key)
	assert.True(t, assert.ObjectsAreEqual(tenantCreate.Attributes, userPermissionsInTenant.Tenant.Attributes))
	assert.ElementsMatch(t, userPermissionsInTenant.Permissions, permissions)

	detailedRAs, err := permitClient.Api.RoleAssignments.ListDetailed(ctx, 1, 100, userKey, roleKey, tenantKey)
	assert.NoError(t, err)
	if assert.NotNil(t, detailedRAs) {
		assert.Equal(t, 1, len(*detailedRAs))
	}
	checkBulk(ctx, t, permitClient, k, roleKey, tenantKey, resourceKey, "read")

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
	require.NoError(t, err)
	deleteAtEnd(t, "condition set "+userSetKey, func(ctx context.Context) error {
		return permitClient.Api.ConditionSets.Delete(ctx, userSetKey)
	})

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
	require.NoError(t, err)
	deleteAtEnd(t, "condition set "+resourceSetKey, func(ctx context.Context) error {
		return permitClient.Api.ConditionSets.Delete(ctx, resourceSetKey)
	})

	// ConditionSets.List reads the bare JSON array that the API answers with. Before v1.3.0 it
	// read only the paginated form, so it always returned an empty list.
	listConditionSets := func(page, perPage int) ([]models.ConditionSetRead, error) {
		return permitClient.Api.ConditionSets.List(ctx, page, perPage)
	}
	conditionSets := listAll(t, "condition sets", listConditionSets)
	assert.Subset(t, keysOf(conditionSets, func(c models.ConditionSetRead) string { return c.Key }),
		[]string{userSetKey, resourceSetKey})

	csUpdate := *models.NewConditionSetUpdate()
	csUpdate.SetDescription("Top Secrets")
	cs, err := permitClient.Api.ConditionSets.Update(ctx, resourceSetKey, csUpdate)
	assert.NoError(t, err)
	if assert.NotNil(t, cs) && assert.NotNil(t, cs.Description) {
		assert.Equal(t, "Top Secrets", *cs.Description)
	}

	_, err = permitClient.Api.ConditionSets.AssignSetPermissions(ctx, userSetKey, resourceKey+":"+actionKey, resourceSetKey)
	require.NoError(t, err)
	deleteAtEnd(t, "the permission of condition set "+userSetKey, func(ctx context.Context) error {
		return permitClient.Api.ConditionSets.UnassignSetPermissions(ctx, userSetKey,
			resourceKey+":"+actionKey, resourceSetKey)
	})

	rules, err := permitClient.Api.ConditionSets.ListSetPermissions(ctx, userSetKey, actionKey, resourceSetKey)
	assert.NoError(t, err)
	assert.Len(t, rules, 1)

	// A type-level check: no ID or key, so the PDP decides by the user's role in tenantKey.
	userCheck := enforcement.UserBuilder(userKey).Build()
	resourceCheck := enforcement.ResourceBuilder(resourceKey).WithTenant(tenantKey).Build()
	waitForAllowed(t, permitClient, userKey, "read", resourceCheck)

	// FilterObjects sends each object's GetID() as its key since v1.3.0. A role in a tenant
	// allows every instance of the resource in that tenant, so the instance of resourceKey is
	// allowed although no role is assigned on it; the object of a resource type that does not
	// exist is not.
	myResources := []enforcement.ResourceI{
		MyResource{
			UniqueID:     k.of("doc-1"),
			Organization: tenantKey,
			Type:         resourceKey,
		},
		MyResource{
			UniqueID:     k.of("doc-2"),
			Organization: tenantKey,
		},
	}
	filteredResources, err := permitClient.FilterObjects(userCheck, "read", nil, myResources...)
	assert.NoError(t, err)
	assert.Equal(t, myResources[:1], filteredResources)

	// A type-level check in every tenant: Build sets the tenant "default", and the PDP checks
	// each tenant the user has a role in instead. Only tenantKey allows the action.
	allowedTenants, err := permitClient.AllTenantsCheck(
		userCheck,
		"read",
		resourceCheck.WithTenant("").Build(),
	)
	assert.NoError(t, err)
	if assert.Len(t, allowedTenants, 1) {
		assert.Equal(t, tenantKey, allowedTenants[0].Key)
		assert.True(t, assert.ObjectsAreEqualValues(allowedTenants[0].Attributes,
			tenantCreate.Attributes))
	}

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
	require.NoError(t, err)
	deleteAtEnd(t, "proxy config "+proxyConfigKey, func(ctx context.Context) error {
		return permitClient.Api.ProxyConfigs.Delete(ctx, proxyConfigKey)
	})
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
	_, err = permitClient.Api.ProxyConfigs.Update(ctx, proxyConfigKey, *proxyConfigUpdate)
	assert.NoError(t, err)
}

// TestInstanceChecks checks decisions on resource instances, for a user whose only role is on
// one instance. Since v1.3.0, Check, BulkCheck and FilterObjects send each resource's key: a
// resource built with WithID sends the ID, and FilterObjects sends GetID() for other types.
// Before, the PDP got no key and decided for the resource type, where the user has no role, so
// the checks below that expect an allow were denied.
func TestInstanceChecks(t *testing.T) {
	s := loadSettings(t)
	k := newKeys(t, "instance")
	ctx := context.Background()
	permitClient := newClient(s)
	resourceKey := k.of("document")
	tenantKey := k.of("tenant")
	userKey := k.of("user")
	viewer := "viewer"
	doc1 := k.of("doc-1")
	doc2 := k.of("doc-2")

	createResource(t, ctx, permitClient, *models.NewResourceCreate(resourceKey, resourceKey,
		map[string]models.ActionBlockEditable{"read": {}, "write": {}}))
	viewerCreate := models.NewResourceRoleCreate(viewer, "Viewer")
	viewerCreate.SetPermissions([]string{"read"})
	_, err := permitClient.Api.ResourceRoles.Create(ctx, resourceKey, *viewerCreate)
	require.NoError(t, err)
	deleteAtEnd(t, "resource role "+resourceKey+"#"+viewer, func(ctx context.Context) error {
		return permitClient.Api.ResourceRoles.Delete(ctx, resourceKey, viewer)
	})
	createTenant(t, ctx, permitClient, *models.NewTenantCreate(tenantKey, tenantKey))
	createUser(t, ctx, permitClient, *models.NewUserCreate(userKey))

	instanceCreate := models.NewResourceInstanceCreate(doc1, resourceKey)
	instanceCreate.SetTenant(tenantKey)
	instance, err := permitClient.Api.ResourceInstances.Create(ctx, *instanceCreate)
	require.NoError(t, err)
	deleteAtEnd(t, "resource instance "+resourceKey+":"+doc1, func(ctx context.Context) error {
		return permitClient.Api.ResourceInstances.Delete(ctx, instance.Id)
	})

	_, err = permitClient.Api.Users.AssignResourceRole(ctx, userKey, viewer, tenantKey,
		resourceKey+":"+doc1)
	require.NoError(t, err)
	deleteAtEnd(t, "role "+viewer+" of user "+userKey+" on "+doc1, func(ctx context.Context) error {
		_, err := permitClient.Api.Users.UnassignResourceRole(ctx, userKey, viewer, tenantKey,
			resourceKey+":"+doc1)
		return err
	})

	user := enforcement.UserBuilder(userKey).Build()
	request := func(a enforcement.Action, r enforcement.Resource) enforcement.CheckRequest {
		return *enforcement.NewCheckRequest(user, a, r, nil)
	}
	doc1Check := enforcement.ResourceBuilder(resourceKey).WithID(doc1).WithTenant(tenantKey).Build()
	doc2Check := enforcement.ResourceBuilder(resourceKey).WithID(doc2).WithTenant(tenantKey).Build()
	typeCheck := enforcement.ResourceBuilder(resourceKey).WithTenant(tenantKey).Build()
	keyCheck := enforcement.ResourceBuilder(resourceKey).WithKey(doc1).WithTenant(tenantKey).Build()
	// WithID sends doc-1 as the key, and the viewer role on doc-1 allows read.
	waitForAllowed(t, permitClient, userKey, "read", doc1Check)
	assertDecision(t, permitClient, request("read", doc2Check), false,
		"the role is on doc-1, not on doc-2 of the same type and tenant")
	assertDecision(t, permitClient, request("read", typeCheck), false,
		"a check without an ID or key is for the type, where the user has no role")
	assertDecision(t, permitClient, request("write", doc1Check), false,
		"the viewer role allows read only")
	assertDecision(t, permitClient, request("read", keyCheck), true, "WithKey sends the key too")

	results, err := permitClient.BulkCheck(request("read", doc1Check), request("read", doc2Check))
	assert.NoError(t, err)
	assert.Equal(t, []bool{true, false}, results, "BulkCheck sends each resource's key")

	objects := []enforcement.ResourceI{
		MyResource{UniqueID: doc1, Type: resourceKey, Organization: tenantKey},
		MyResource{UniqueID: doc2, Type: resourceKey, Organization: tenantKey},
		enforcement.ResourceBuilder(resourceKey).WithKey(doc1).WithTenant(tenantKey),
	}
	filtered, err := permitClient.FilterObjects(user, "read", nil, objects...)
	assert.NoError(t, err)
	assert.Equal(t, []enforcement.ResourceI{objects[0], objects[2]}, filtered,
		"FilterObjects sends GetID() of other types, and the key of a Resource, as the key")
}

// TestClientWithoutContextUsesTheKeysScope checks that a client built without WithContext reads
// the project and environment of its environment API key from the API, also when its first calls
// start at the same time.
func TestClientWithoutContextUsesTheKeysScope(t *testing.T) {
	s := loadSettings(t)
	k := newKeys(t, "scope")
	ctx := context.Background()
	newScopeClient := func() *permit.Client {
		return permit.New(config.NewConfigBuilder(s.apiKey).
			WithPdpUrl(s.pdpURL).
			WithApiUrl(s.apiURL).
			WithLogger(zap.NewExample()).
			Build())
	}
	tenantKey := k.of("tenant")
	createTenant(t, ctx, newScopeClient(), *models.NewTenantCreate(tenantKey, tenantKey))

	// The client with the context from PROJECT and ENV finds the tenant, so the scope is the
	// same environment.
	tenant, err := newClient(s).Api.Tenants.Get(ctx, tenantKey)
	require.NoError(t, err)
	assert.Equal(t, tenantKey, tenant.Key)

	client := newScopeClient()
	errs := make([]error, 4)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var tenant *models.TenantRead
			tenant, errs[i] = client.Api.Tenants.Get(ctx, tenantKey)
			if errs[i] == nil && tenant.Key != tenantKey {
				errs[i] = fmt.Errorf("got tenant %s", tenant.Key)
			}
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		assert.NoError(t, err, "concurrent first call %d", i)
	}
}
