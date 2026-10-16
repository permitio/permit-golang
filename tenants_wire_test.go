package permit_test

import (
	"context"
	"encoding/json"
	"iter"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the tenants: a key that may not act on
// the tenants of the environment, a tenant that does not exist, a key that another tenant of the
// environment has already, and a field that the API rejects.
const (
	tenantForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the tenants of the environment","id":"req-9"}`
	tenantNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the tenant does not exist","id":"req-10"}`
	tenantConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"a tenant with the key tenant-c exists already","id":"req-11"}`
	tenantInvalidBody = `{"detail":[{"loc":["body","name"],` +
		`"msg":"none is not an allowed value","type":"type_error.none.not_allowed"}]}`
)

// The IDs of the tenants of the reply fixtures.
const (
	tenantAID = "00000000-0000-4000-8000-000000000021"
	tenantBID = "00000000-0000-4000-8000-000000000022"
	tenantCID = "00000000-0000-4000-8000-000000000023"
)

// tenantAt returns a tenant of the reply fixtures with the key, the ID, the name and the
// attributes, and the scope of the fixtures, created, changed and last acted on at created.
func tenantAt(key, id, name, created string, attributes models.Attributes) models.TenantRead {
	return models.TenantRead{
		Key: key, ID: id, OrganizationID: exampleOrganizationID, ProjectID: exampleProjectID,
		EnvironmentID: exampleEnvironmentID, CreatedAt: at(created), UpdatedAt: at(created),
		LastActionAt: at(created), Name: name, Attributes: attributes,
	}
}

// fixtureTenant is the tenant of the reply fixture get_tenant.200.json, with every field and
// attributes of each kind of JSON value.
func fixtureTenant() *models.TenantRead {
	tenant := tenantAt("tenant-a", tenantAID, "Tenant A", "2026-10-14T09:30:00Z", models.Attributes{
		"tier": "gold", "seats": json.Number("9007199254740993"),
		"regions": []any{"eu", "us"}, "billing": map[string]any{"active": true},
	})
	tenant.UpdatedAt = at("2026-10-15T10:00:00.123456")
	tenant.LastActionAt = at("2026-10-15T10:05:00Z")
	tenant.Description = "The first tenant"
	return &tenant
}

// listedTenants are the tenants of the reply fixture list_tenants.200.json: one with attributes
// and one with the empty attributes {}, which stay an empty map, not nil.
func listedTenants() []models.TenantRead {
	return []models.TenantRead{
		tenantAt("tenant-a", tenantAID, "Tenant A", "2026-10-14T09:30:00Z",
			models.Attributes{"tier": "gold"}),
		tenantAt("tenant-b", tenantBID, "Tenant B", "2026-10-15T08:00:00Z", models.Attributes{}),
	}
}

// firstTenants returns the first n tenants that seq yields, and ends the loop there, so that it
// asks for no page past them; or the first error that seq yields.
func firstTenants(seq iter.Seq2[models.TenantRead, error], n int) ([]models.TenantRead, error) {
	var got []models.TenantRead
	for tenant, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, tenant)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// tenantCases returns the wire cases of the tenants of an environment, each of which the runner
// runs against the API, with its scope, and against the PDP container under /facts/tenants,
// with WithFactsViaPDP. The keys of the tenants hold characters that the path escapes, a slash,
// a percent sign and a letter outside ASCII, as a/b and 50%é do, and one key has a leading dot,
// .hidden, which the path sends as it is. The cases of the users in a tenant are those of
// tenantUserCases.
func tenantCases() []wireCase {
	const (
		tenants = "/v2/facts/{proj_id}/{env_id}/tenants"
		tenant  = tenants + "/{tenant_id}"
	)
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, tenantForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-9"
		e.Detail = "the API key may not act on the tenants of the environment"
		return e
	}
	listErr := projectError("list_tenants", http.MethodGet, tenants,
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	getErr := projectError("get_tenant", http.MethodGet, tenant, http.StatusNotFound,
		tenantNotFoundBody)
	getErr.Code, getErr.Title, getErr.RequestID = "NOT_FOUND", "Not Found", "req-10"
	getErr.Detail = "the tenant does not exist"
	createErr := projectError("create_tenant", http.MethodPost, tenants, http.StatusConflict,
		tenantConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "DUPLICATE_ENTITY", "Conflict", "req-11"
	createErr.Detail = "a tenant with the key tenant-c exists already"
	updateErr := projectError("update_tenant", http.MethodPatch, tenant,
		http.StatusUnprocessableEntity, tenantInvalidBody)
	updateErr.Detail = "body.name: none is not an allowed value"
	both := []wireMode{apiMode, factsViaPDPMode}
	return append([]wireCase{
		{
			Method: "TenantsService.List",
			Op:     "list_tenants",
			Route:  "GET " + tenants,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Tenants.List(ctx, &permit.TenantsListOptions{
					ListOptions: permit.ListOptions{Page: 2, PerPage: 1},
					Attributes: []permit.AttributeFilter{
						permit.AttributeEquals("tier", "gold"), permit.AttributeEquals("seats", 10),
					},
				})
			},
			Request: wireRequest{
				RawPath:  "/v2/facts/p/e/tenants",
				RawQuery: "attr_seats=10&attr_tier=gold&page=2&per_page=1",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_tenants.200.page.json")},
			WantResult: &permit.Page[models.TenantRead]{
				Data: []models.TenantRead{tenantAt("tenant-a", tenantAID, "Tenant A",
					"2026-10-14T09:30:00Z",
					models.Attributes{"tier": "gold", "seats": json.Number("10")})},
				Page: 2, PerPage: 1, TotalCount: new(11), PageCount: new(1),
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "TenantsService.All",
			Op:     "list_tenants",
			Route:  "GET " + tenants,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstTenants(c.Tenants.All(ctx, nil), 2)
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/tenants", RawQuery: "page=1&per_page=100",
			},
			Reply:      mockpermit.Reply{Body: replyFixture("list_tenants.200.json")},
			WantResult: listedTenants(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: tenantForbiddenBody},
			WantErr:    forbidden("list_tenants", http.MethodGet, tenants),
		},
		{
			Method: "TenantsService.Get",
			Op:     "get_tenant",
			Route:  "GET " + tenant,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Tenants.Get(ctx, "a/b")
			},
			Request:    wireRequest{RawPath: "/v2/facts/p/e/tenants/a%2Fb"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_tenant.200.json")},
			WantResult: fixtureTenant(),
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: tenantNotFoundBody},
			WantErr:    getErr,
		},
		{
			Method: "TenantsService.Create",
			Op:     "create_tenant",
			Route:  "POST " + tenants,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Tenants.Create(ctx, models.TenantCreate{
					Key: "tenant-c", Name: "Tenant C", Description: "A new tenant",
					Attributes: models.Attributes{"tier": "silver"},
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/tenants",
				Body: `{"key":"tenant-c","name":"Tenant C","description":"A new tenant",` +
					`"attributes":{"tier":"silver"}}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("create_tenant.200.json")},
			WantResult: createdTenant(),
			ErrorReply: mockpermit.Reply{Status: http.StatusConflict, Body: tenantConflictBody},
			WantErr:    createErr,
		},
		{
			Method: "TenantsService.Update",
			Op:     "update_tenant",
			Route:  "PATCH " + tenant,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Tenants.Update(ctx, "50%é", models.TenantUpdate{
					Name: "Renamed Tenant A", Attributes: models.Null[models.Attributes](),
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/tenants/50%25%C3%A9",
				Body:    `{"name":"Renamed Tenant A","attributes":null}`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("update_tenant.200.null-attributes.json"),
			},
			WantResult: updatedTenant(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: tenantInvalidBody,
			},
			WantErr: updateErr,
		},
		{
			Method: "TenantsService.Delete",
			Op:     "delete_tenant",
			Route:  "DELETE " + tenant,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Tenants.Delete(ctx, ".hidden")
			},
			Request:    wireRequest{RawPath: "/v2/facts/p/e/tenants/.hidden"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: tenantForbiddenBody},
			WantErr:    forbidden("delete_tenant", http.MethodDelete, tenant),
		},
	}, tenantUserCases(forbidden)...)
}

// The error bodies of the Permit API in the wire cases of the users in a tenant: a user that
// exists already, and a path that the API rejects.
const (
	tenantUserConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"a user with the key erin exists already","id":"req-16"}`
	tenantUserInvalidBody = `{"detail":[{"loc":["path","user_id"],` +
		`"msg":"field required","type":"value_error.missing"}]}`
)

// tenantUserCases returns the wire cases of the users in a tenant, in both modes, with the error
// of a 403 that forbidden returns. The keys of the tenants and of the users hold characters that
// the path escapes, a slash, a percent sign, a letter outside ASCII and a |, as a/b, 50%é and
// idp|dave@example.com do, and one key has a leading dot, .hidden, which the path sends as it is:
// a removal sends one of them in each of its two keys.
func tenantUserCases(forbidden func(op, method, route string) *permit.Error) []wireCase {
	const (
		users = "/v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}/users"
		user  = users + "/{user_id}"
	)
	listErr := projectError("list_tenant_users", http.MethodGet, users,
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	addErr := projectError("add_user_to_tenant", http.MethodPost, users, http.StatusConflict,
		tenantUserConflictBody)
	addErr.Code, addErr.Title, addErr.RequestID = "DUPLICATE_ENTITY", "Conflict", "req-16"
	addErr.Detail = "a user with the key erin exists already"
	removeErr := projectError("delete_tenant_user", http.MethodDelete, user,
		http.StatusUnprocessableEntity, tenantUserInvalidBody)
	removeErr.Detail = "path.user_id: field required"
	both := []wireMode{apiMode, factsViaPDPMode}
	return []wireCase{
		{
			Method: "TenantsService.ListUsers",
			Op:     "list_tenant_users",
			Route:  "GET " + users,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Tenants.ListUsers(ctx, "50%é", &permit.TenantUsersListOptions{
					ListOptions: permit.ListOptions{Page: 2, PerPage: 1},
					Search:      "a&b c", Role: "viewer",
				})
			},
			Request: wireRequest{
				RawPath:  "/v2/facts/p/e/tenants/50%25%C3%A9/users",
				RawQuery: "page=2&per_page=1&role=viewer&search=a%26b+c",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_tenant_users.200.json")},
			WantResult: &permit.Page[models.UserRead]{
				Data: tenantUsers(), Page: 2, PerPage: 1, TotalCount: new(2), PageCount: new(2),
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "TenantsService.AllUsers",
			Op:     "list_tenant_users",
			Route:  "GET " + users,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstUsers(c.Tenants.AllUsers(ctx, ".hidden", nil), 2)
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/tenants/.hidden/users", RawQuery: "page=1&per_page=100",
			},
			Reply:      mockpermit.Reply{Body: replyFixture("list_tenant_users.200.json")},
			WantResult: tenantUsers(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: tenantForbiddenBody},
			WantErr:    forbidden("list_tenant_users", http.MethodGet, users),
		},
		{
			Method: "TenantsService.AddUser",
			Op:     "add_user_to_tenant",
			Route:  "POST " + users,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Tenants.AddUser(ctx, "a/b",
					models.UserCreate{Key: "erin", Email: "erin@example.com"})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/tenants/a%2Fb/users",
				Body:    `{"key":"erin","email":"erin@example.com"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("add_user_to_tenant.200.json")},
			WantResult: addedTenantUser(),
			ErrorReply: mockpermit.Reply{Status: http.StatusConflict, Body: tenantUserConflictBody},
			WantErr:    addErr,
		},
		{
			Method: "TenantsService.RemoveUser",
			Op:     "delete_tenant_user",
			Route:  "DELETE " + user,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Tenants.RemoveUser(ctx, "50%é", syncedKey)
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/tenants/50%25%C3%A9/users/idp%7Cdave@example.com",
			},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: tenantForbiddenBody},
			WantErr:    forbidden("delete_tenant_user", http.MethodDelete, user),
		},
		{
			Method: "TenantsService.RemoveUser",
			Op:     "delete_tenant_user",
			Route:  "DELETE " + user,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Tenants.RemoveUser(ctx, ".hidden", "a/b")
			},
			Request:    wireRequest{RawPath: "/v2/facts/p/e/tenants/.hidden/users/a%2Fb"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: tenantUserInvalidBody,
			},
			WantErr: removeErr,
		},
	}
}

// tenantUsers are the users of the reply fixture list_tenant_users.200.json, both in the tenant
// tenant-a: alice without a role there, and bob with the role viewer.
func tenantUsers() []models.UserRead {
	alice := userAt("alice", userAliceID, "2026-10-14T09:30:00Z")
	alice.Email = "alice@example.com"
	alice.Attributes = models.Attributes{"department": "finance"}
	alice.AssociatedTenants = []models.UserInTenant{
		{Tenant: "tenant-a", Roles: []string{}, Status: models.UserStatusActive},
	}
	bob := userAt("bob", userBobID, "2026-10-15T08:00:00Z")
	bob.Attributes = models.Attributes{}
	bob.AssociatedTenants = []models.UserInTenant{
		{Tenant: "tenant-a", Roles: []string{"viewer"}, Status: models.UserStatusActive},
	}
	return []models.UserRead{alice, bob}
}

// addedTenantUser is the user of the reply fixture add_user_to_tenant.200.json, which an add
// created in the tenant tenant-a without a role.
func addedTenantUser() *models.UserRead {
	user := userAt("erin", "00000000-0000-4000-8000-000000000051", "2026-10-16T09:00:00Z")
	user.Email = "erin@example.com"
	user.Attributes = models.Attributes{}
	user.AssociatedTenants = []models.UserInTenant{{
		Tenant: "tenant-a", Roles: []string{}, Status: models.UserStatusActive,
		ResourceInstanceRoles: []models.UserResourceInstanceRole{},
	}}
	return &user
}

// createdTenant is the tenant of the reply fixture create_tenant.200.json.
func createdTenant() *models.TenantRead {
	tenant := tenantAt("tenant-c", tenantCID, "Tenant C", "2026-10-15T11:00:00Z",
		models.Attributes{"tier": "silver"})
	tenant.Description = "A new tenant"
	return &tenant
}

// updatedTenant is the tenant of the reply fixture update_tenant.200.null-attributes.json, whose
// attributes an update set to null: they stay nil, not an empty map.
func updatedTenant() *models.TenantRead {
	tenant := tenantAt("tenant-a", tenantAID, "Renamed Tenant A", "2026-10-14T09:30:00Z", nil)
	tenant.UpdatedAt = at("2026-10-15T12:00:00Z")
	tenant.LastActionAt = at("2026-10-15T12:00:00Z")
	return &tenant
}
