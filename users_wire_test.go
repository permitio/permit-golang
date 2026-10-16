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

// The error bodies of the Permit API in the wire cases of the users: a key that may not act on
// the users of the environment, a user that does not exist, a key that another user of the
// environment has already, and a field that the API rejects.
const (
	userForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the users of the environment","id":"req-12"}`
	userNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the user does not exist","id":"req-13"}`
	userConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"a user with the key carol exists already","id":"req-14"}`
	userInvalidBody = `{"detail":[{"loc":["body","email"],` +
		`"msg":"value is not a valid email address","type":"value_error.email"}]}`
)

// The error bodies of the Permit API in the wire cases of the roles of a user: a role that the
// user does not have in the tenant, and a role that the body leaves out.
const (
	userRoleNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the user does not have the role in the tenant","id":"req-15"}`
	userRoleInvalidBody = `{"detail":[{"loc":["body","role"],` +
		`"msg":"field required","type":"value_error.missing"}]}`
)

// The IDs of the users of the reply fixtures.
const (
	userAliceID = "00000000-0000-4000-8000-000000000031"
	userBobID   = "00000000-0000-4000-8000-000000000032"
	userCarolID = "00000000-0000-4000-8000-000000000033"
	userDaveID  = "00000000-0000-4000-8000-000000000034"
)

// syncedKey is the key of the user of the wire cases of Sync: a key of the shape that an
// identity provider gives, whose | the path escapes and whose @ it sends as it is.
const syncedKey = "idp|dave@example.com"

// userAt returns a user of the reply fixtures with the key and the ID, and the scope of the
// fixtures, created and changed at created.
func userAt(key, id, created string) models.UserRead {
	return models.UserRead{
		Key: key, ID: id, OrganizationID: exampleOrganizationID, ProjectID: exampleProjectID,
		EnvironmentID: exampleEnvironmentID, CreatedAt: at(created), UpdatedAt: at(created),
	}
}

// fixtureUser is the user of the reply fixture get_user.200.json, with every field, attributes
// of each kind of JSON value, and a tenant with a role on a resource instance.
func fixtureUser() *models.UserRead {
	user := userAt("alice", userAliceID, "2026-10-14T09:30:00Z")
	user.UpdatedAt = at("2026-10-15T10:00:00.123456")
	user.Email, user.FirstName, user.LastName = "alice@example.com", "Alice", "Liddell"
	user.Attributes = models.Attributes{
		"department": "finance", "clearance": json.Number("9007199254740993"),
		"regions": []any{"eu", "us"}, "manager": map[string]any{"active": true},
	}
	user.AssociatedTenants = []models.UserInTenant{
		{
			Tenant: "tenant-a", Roles: []string{"viewer", "editor"},
			Status: models.UserStatusActive,
			ResourceInstanceRoles: []models.UserResourceInstanceRole{
				{ResourceInstance: "doc-1", Resource: "document", Role: "owner"},
			},
		},
		{
			Tenant: "tenant-b", Roles: []string{}, Status: models.UserStatusPending,
			ResourceInstanceRoles: []models.UserResourceInstanceRole{},
		},
	}
	return &user
}

// listedUsers are the users of the reply fixture list_users.200.json: one with attributes and
// no tenant, and one with the empty attributes {}, which stay an empty map, not nil.
func listedUsers() []models.UserRead {
	alice := userAt("alice", userAliceID, "2026-10-14T09:30:00Z")
	alice.Email = "alice@example.com"
	alice.Attributes = models.Attributes{"department": "finance"}
	alice.AssociatedTenants = []models.UserInTenant{}
	bob := userAt("bob", userBobID, "2026-10-15T08:00:00Z")
	bob.Attributes = models.Attributes{}
	bob.AssociatedTenants = []models.UserInTenant{
		{Tenant: "tenant-a", Roles: []string{"viewer"}, Status: models.UserStatusActive},
	}
	return []models.UserRead{alice, bob}
}

// firstUsers returns the first n users that seq yields, and ends the loop there, so that it asks
// for no page past them; or the first error that seq yields.
func firstUsers(seq iter.Seq2[models.UserRead, error], n int) ([]models.UserRead, error) {
	var got []models.UserRead
	for user, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, user)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// userCases returns the wire cases of the users of an environment, each of which the runner runs
// against the API, with its scope, and against the PDP container under /facts/users, with
// WithFactsViaPDP. The keys of the users hold characters that the path escapes, a slash, a
// percent sign, a letter outside ASCII and a |, as a/b, 50%é and idp|dave@example.com do, and
// one key has a leading dot, .hidden, which the path sends as it is. The cases of the roles of a
// user are those of userRoleCases.
func userCases() []wireCase {
	const (
		users = "/v2/facts/{proj_id}/{env_id}/users"
		user  = users + "/{user_id}"
	)
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, userForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-12"
		e.Detail = "the API key may not act on the users of the environment"
		return e
	}
	invalid := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusUnprocessableEntity, userInvalidBody)
		e.Detail = "body.email: value is not a valid email address"
		return e
	}
	listErr := projectError("list_users", http.MethodGet, users, http.StatusUnprocessableEntity,
		perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	getErr := projectError("get_user", http.MethodGet, user, http.StatusNotFound,
		userNotFoundBody)
	getErr.Code, getErr.Title, getErr.RequestID = "NOT_FOUND", "Not Found", "req-13"
	getErr.Detail = "the user does not exist"
	createErr := projectError("create_user", http.MethodPost, users, http.StatusConflict,
		userConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "DUPLICATE_ENTITY", "Conflict", "req-14"
	createErr.Detail = "a user with the key carol exists already"
	synced := models.UserCreate{
		Key: syncedKey, Email: "dave@example.com", FirstName: "Dave",
		Attributes: models.Attributes{"department": "support"},
	}
	syncPath := "/v2/facts/p/e/users/idp%7Cdave@example.com"
	both := []wireMode{apiMode, factsViaPDPMode}
	return append([]wireCase{
		{
			Method: "UsersService.List",
			Op:     "list_users",
			Route:  "GET " + users,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Users.List(ctx, &permit.UsersListOptions{
					ListOptions: permit.ListOptions{Page: 2, PerPage: 1},
					Search:      "a&b c", Role: "viewer",
				})
			},
			Request: wireRequest{
				RawPath:  "/v2/facts/p/e/users",
				RawQuery: "page=2&per_page=1&role=viewer&search=a%26b+c",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_users.200.json")},
			WantResult: &permit.Page[models.UserRead]{
				Data: listedUsers(), Page: 2, PerPage: 1, TotalCount: new(2), PageCount: new(2),
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "UsersService.All",
			Op:     "list_users",
			Route:  "GET " + users,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstUsers(c.Users.All(ctx, nil), 2)
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/users", RawQuery: "page=1&per_page=100",
			},
			Reply:      mockpermit.Reply{Body: replyFixture("list_users.200.json")},
			WantResult: listedUsers(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: userForbiddenBody},
			WantErr:    forbidden("list_users", http.MethodGet, users),
		},
		{
			Method: "UsersService.Get",
			Op:     "get_user",
			Route:  "GET " + user,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Users.Get(ctx, "a/b")
			},
			Request:    wireRequest{RawPath: "/v2/facts/p/e/users/a%2Fb"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_user.200.json")},
			WantResult: fixtureUser(),
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: userNotFoundBody},
			WantErr:    getErr,
		},
		{
			Method: "UsersService.Create",
			Op:     "create_user",
			Route:  "POST " + users,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Users.Create(ctx, models.UserCreate{
					Key: "carol", Email: "carol@example.com", FirstName: "Carol",
					Attributes:      models.Attributes{"department": "sales"},
					RoleAssignments: []models.UserRoleCreate{{Role: "editor", Tenant: "tenant-a"}},
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/users",
				Body: `{"key":"carol","email":"carol@example.com","first_name":"Carol",` +
					`"attributes":{"department":"sales"},` +
					`"role_assignments":[{"role":"editor","tenant":"tenant-a"}]}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("create_user.200.json")},
			WantResult: createdUser(),
			ErrorReply: mockpermit.Reply{Status: http.StatusConflict, Body: userConflictBody},
			WantErr:    createErr,
		},
		{
			Method: "UsersService.Sync",
			Op:     "replace_user",
			Route:  "PUT " + user,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Users.Sync(ctx, synced)
			},
			Request: wireRequest{
				RawPath: syncPath,
				Body: `{"key":"idp|dave@example.com","email":"dave@example.com",` +
					`"first_name":"Dave","attributes":{"department":"support"}}`,
			},
			Reply: mockpermit.Reply{
				Status: http.StatusCreated, Body: replyFixture("replace_user.201.json"),
			},
			WantResult: &permit.UserSyncResult{User: syncedUser(), Created: true},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: userInvalidBody,
			},
			WantErr: invalid("replace_user", http.MethodPut, user),
		},
		{
			Method: "UsersService.Sync",
			Op:     "replace_user",
			Route:  "PUT " + user,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Users.Sync(ctx, models.UserCreate{
					Key: syncedKey, Email: "dave@example.com", Attributes: models.Attributes{},
				})
			},
			Request: wireRequest{
				RawPath: syncPath,
				Body: `{"key":"idp|dave@example.com","email":"dave@example.com",` +
					`"attributes":{}}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("replace_user.200.json")},
			WantResult: &permit.UserSyncResult{User: replacedUser()},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: userForbiddenBody},
			WantErr:    forbidden("replace_user", http.MethodPut, user),
		},
		{
			Method: "UsersService.Update",
			Op:     "update_user",
			Route:  "PATCH " + user,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Users.Update(ctx, "50%é", models.UserUpdate{
					Email:      models.Some("alice@example.org"),
					LastName:   models.Null[string](),
					Attributes: models.Some(models.Attributes{"department": "legal"}),
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/users/50%25%C3%A9",
				Body: `{"email":"alice@example.org","last_name":null,` +
					`"attributes":{"department":"legal"}}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("update_user.200.json")},
			WantResult: updatedUser(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: userInvalidBody,
			},
			WantErr: invalid("update_user", http.MethodPatch, user),
		},
		{
			Method: "UsersService.Delete",
			Op:     "delete_user",
			Route:  "DELETE " + user,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Users.Delete(ctx, ".hidden")
			},
			Request:    wireRequest{RawPath: "/v2/facts/p/e/users/.hidden"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: userForbiddenBody},
			WantErr:    forbidden("delete_user", http.MethodDelete, user),
		},
	}, userRoleCases(forbidden)...)
}

// userRoleCases returns the wire cases of the assignments and the unassignments of the roles of
// a user, in both modes, with the error of a 403 that forbidden returns: a role in a tenant and
// a role on a resource instance without a tenant, whose body leaves the tenant out; and an
// unassignment that gets 204 without a body and one that gets 200 with the role assignment,
// both of which return nil, and a 404, which stays an error that matches ErrNotFound.
func userRoleCases(forbidden func(op, method, route string) *permit.Error) []wireCase {
	const roles = "/v2/facts/{proj_id}/{env_id}/users/{user_id}/roles"
	invalid := func(op, method string) *permit.Error {
		e := projectError(op, method, roles, http.StatusUnprocessableEntity, userRoleInvalidBody)
		e.Detail = "body.role: field required"
		return e
	}
	notFound := projectError("unassign_role_from_user", http.MethodDelete, roles,
		http.StatusNotFound, userRoleNotFoundBody)
	notFound.Code, notFound.Title, notFound.RequestID = "NOT_FOUND", "Not Found", "req-15"
	notFound.Detail = "the user does not have the role in the tenant"
	both := []wireMode{apiMode, factsViaPDPMode}
	return []wireCase{
		{
			Method: "UsersService.AssignRole",
			Op:     "assign_role_to_user",
			Route:  "POST " + roles,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Users.AssignRole(ctx, "50%é",
					models.UserRoleCreate{Role: "editor", Tenant: "tenant-a"})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/users/50%25%C3%A9/roles",
				Body:    `{"role":"editor","tenant":"tenant-a"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("assign_role_to_user.200.json")},
			WantResult: tenantRoleAssignment(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: userRoleInvalidBody,
			},
			WantErr: invalid("assign_role_to_user", http.MethodPost),
		},
		{
			Method: "UsersService.AssignRole",
			Op:     "assign_role_to_user",
			Route:  "POST " + roles,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Users.AssignRole(ctx, syncedKey,
					models.UserRoleCreate{Role: "owner", ResourceInstance: "document:doc-1"})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/users/idp%7Cdave@example.com/roles",
				Body:    `{"role":"owner","resource_instance":"document:doc-1"}`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("assign_role_to_user.200.instance.json"),
			},
			WantResult: instanceRoleAssignment(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: userForbiddenBody},
			WantErr:    forbidden("assign_role_to_user", http.MethodPost, roles),
		},
		{
			Method: "UsersService.UnassignRole",
			Op:     "unassign_role_from_user",
			Route:  "DELETE " + roles,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Users.UnassignRole(ctx, ".hidden",
					models.UserRoleRemove{Role: "editor", Tenant: "tenant-a"})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/users/.hidden/roles",
				Body:    `{"role":"editor","tenant":"tenant-a"}`,
			},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: userRoleNotFoundBody},
			WantErr:    notFound,
		},
		{
			Method: "UsersService.UnassignRole",
			Op:     "unassign_role_from_user",
			Route:  "DELETE " + roles,
			Modes:  both,
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.Users.UnassignRole(ctx, "a/b", models.UserRoleRemove{
					Role: "owner", Tenant: "tenant-a", ResourceInstance: "document:doc-1",
				})
			},
			Request: wireRequest{
				RawPath: "/v2/facts/p/e/users/a%2Fb/roles",
				Body: `{"role":"owner","tenant":"tenant-a",` +
					`"resource_instance":"document:doc-1"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("unassign_role_from_user.200.json")},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: userRoleInvalidBody,
			},
			WantErr: invalid("unassign_role_from_user", http.MethodDelete),
		},
	}
}

// roleAssignment returns a role assignment of the reply fixtures of the roles of a user: of the
// role whose ID is roleID to alice in the tenant tenant-a, with the ID id and the scope of the
// fixtures, created at created.
func roleAssignment(id, role, roleID, created string) *models.RoleAssignmentRead {
	return &models.RoleAssignmentRead{
		ID: id, User: "alice", Role: role, Tenant: "tenant-a", UserID: userAliceID,
		RoleID: roleID, TenantID: "00000000-0000-4000-8000-000000000037",
		OrganizationID: exampleOrganizationID, ProjectID: exampleProjectID,
		EnvironmentID: exampleEnvironmentID, CreatedAt: at(created),
	}
}

// tenantRoleAssignment is the role assignment of the reply fixture
// assign_role_to_user.200.json: the role editor in the tenant tenant-a.
func tenantRoleAssignment() *models.RoleAssignmentRead {
	return roleAssignment("00000000-0000-4000-8000-000000000035", "editor",
		"00000000-0000-4000-8000-000000000036", "2026-10-15T14:00:00Z")
}

// instanceRoleAssignment is the role assignment of the reply fixture
// assign_role_to_user.200.instance.json: the role owner on the resource instance document:doc-1.
func instanceRoleAssignment() *models.RoleAssignmentRead {
	assignment := roleAssignment("00000000-0000-4000-8000-000000000038", "owner",
		"00000000-0000-4000-8000-000000000040", "2026-10-15T14:30:00.250000")
	assignment.ResourceInstance = "document:doc-1"
	assignment.ResourceInstanceID = "00000000-0000-4000-8000-000000000039"
	return assignment
}

// createdUser is the user of the reply fixture create_user.200.json, with the role of its
// create in its tenant.
func createdUser() *models.UserRead {
	user := userAt("carol", userCarolID, "2026-10-15T11:00:00Z")
	user.Email, user.FirstName = "carol@example.com", "Carol"
	user.Attributes = models.Attributes{"department": "sales"}
	user.AssociatedTenants = []models.UserInTenant{{
		Tenant: "tenant-a", Roles: []string{"editor"}, Status: models.UserStatusActive,
		ResourceInstanceRoles: []models.UserResourceInstanceRole{},
	}}
	return &user
}

// syncedUser is the user of the reply fixture replace_user.201.json, which a sync created.
func syncedUser() models.UserRead {
	user := userAt(syncedKey, userDaveID, "2026-10-15T12:00:00Z")
	user.Email, user.FirstName = "dave@example.com", "Dave"
	user.Attributes = models.Attributes{"department": "support"}
	user.AssociatedTenants = []models.UserInTenant{}
	return user
}

// replacedUser is the user of the reply fixture replace_user.200.json, which a sync replaced
// without a first name, so that the API cleared it.
func replacedUser() models.UserRead {
	user := userAt(syncedKey, userDaveID, "2026-10-15T12:00:00Z")
	user.UpdatedAt = at("2026-10-15T12:30:00Z")
	user.Email = "dave@example.com"
	user.Attributes = models.Attributes{}
	user.AssociatedTenants = []models.UserInTenant{}
	return user
}

// updatedUser is the user of the reply fixture update_user.200.json, whose last name an update
// removed.
func updatedUser() *models.UserRead {
	user := userAt("alice", userAliceID, "2026-10-14T09:30:00Z")
	user.UpdatedAt = at("2026-10-15T13:00:00Z")
	user.Email, user.FirstName = "alice@example.org", "Alice"
	user.Attributes = models.Attributes{"department": "legal"}
	user.AssociatedTenants = []models.UserInTenant{}
	return &user
}
