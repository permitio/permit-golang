package permit_test

import (
	"context"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The error bodies of the Permit API in the wire cases of the user invites: a key that may not act
// on the invites of the environment, an invite that does not exist, an invite whose key another
// invite has already, an update without an email address, and the refusal of an approval, whose
// body is the detail alone.
const (
	inviteForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
		`"message":"the API key may not act on the user invites of the environment",` +
		`"id":"req-41"}`
	inviteNotFoundBody = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the user invite does not exist","id":"req-42"}`
	inviteConflictBody = `{"error_code":"DUPLICATE_ENTITY","title":"Conflict",` +
		`"message":"a user invite with the key alice@example.com exists already","id":"req-43"}`
	inviteEmailMissingBody = `{"detail":[{"loc":["body","email"],"msg":"field required",` +
		`"type":"value_error.missing"}]}`
	inviteApprovedBody = `{"detail":"the user invite is approved already"}`
)

// The IDs of the role, the tenant and the resource instance of the invites of the reply fixtures.
const (
	inviteRoleID     = "00000000-0000-4000-8000-000000000411"
	inviteTenantID   = "00000000-0000-4000-8000-000000000412"
	inviteInstanceID = "00000000-0000-4000-8000-000000000413"
)

// inviteOf returns an invite of the reply fixtures, of the environment of the scope fixture, whose
// key and email address are key, with the role, the tenant and the resource instance of the
// fixtures.
func inviteOf(id int, key string, status models.UserInviteStatus, first, last string,
) models.ElementsUserInviteRead {
	return models.ElementsUserInviteRead{
		ID: resourceID(id), OrganizationID: exampleOrganizationID, ProjectID: exampleProjectID,
		EnvironmentID: exampleEnvironmentID, Key: key, Status: status, Email: key,
		FirstName: first, LastName: last, RoleID: inviteRoleID, TenantID: inviteTenantID,
		ResourceInstanceID: inviteInstanceID,
	}
}

// inviteFixture is the invite of the reply fixtures get_user_invite.200.json and
// create_user_invite.200.json, of alice@example.com.
func inviteFixture() *models.ElementsUserInviteRead {
	i := inviteOf(401, "alice@example.com", models.UserInviteStatusPending, "Alice", "Liddell")
	i.CreatedAt, i.UpdatedAt = at("2026-10-15T08:00:00Z"), at("2026-10-15T08:00:00Z")
	return &i
}

// inviteListed are the invites of the reply fixture list_user_invites.200.json: one pending and
// one approved.
func inviteListed() []models.ElementsUserInviteRead {
	bob := inviteOf(402, "bob@example.com", models.UserInviteStatusApproved, "Bob", "Builder")
	bob.CreatedAt, bob.UpdatedAt = at("2026-10-14T09:30:00Z"), at("2026-10-14T10:00:00Z")
	return []models.ElementsUserInviteRead{*inviteFixture(), bob}
}

// inviteUpdated is the invite of the reply fixture update_user_invite.200.json, with its new last
// name.
func inviteUpdated() *models.ElementsUserInviteRead {
	i := inviteFixture()
	i.LastName, i.UpdatedAt = "Hargreaves", at("2026-10-15T13:00:00.123456")
	return i
}

// inviteApprovedUser is the user of the reply fixture approve_user_invite.200.json.
func inviteApprovedUser() *models.UserRead {
	return &models.UserRead{
		Key: "alice@example.com", ID: resourceID(431), OrganizationID: exampleOrganizationID,
		ProjectID: exampleProjectID, EnvironmentID: exampleEnvironmentID,
		CreatedAt: at("2026-10-15T14:00:00Z"), UpdatedAt: at("2026-10-15T14:00:00Z"),
		Email: "alice@example.com", FirstName: "Alice", LastName: "Hargreaves",
		Attributes:        models.Attributes{"department": "finance"},
		AssociatedTenants: []models.UserInTenant{},
	}
}

// inviteBody is the body of the create and the update of the wire cases, the invite of
// alice@example.com with lastName as its last name, and each other field of the fixtures.
func inviteBody(lastName string) string {
	return `{"key":"alice@example.com","status":"pending","email":"alice@example.com",` +
		`"first_name":"Alice","last_name":"` + lastName + `","role_id":"` + inviteRoleID +
		`","tenant_id":"` + inviteTenantID + `","resource_instance_id":"` + inviteInstanceID +
		`"}`
}

// userInviteCases returns the wire cases of the user invites of an environment. Each sends one
// request to the API, along a route of /v2/facts/ that the runner's scope fills, p and e, as
// WithScope gives it, and none runs through the PDP container, as the client sends the invites to
// the API in both modes. The IDs of the invites hold characters that the path escapes, a slash, a
// percent sign and a letter outside ASCII, as a/b and 50%é do; one has a leading dot, .hidden, and
// another the shape of a UUID, which the path sends as they are. The filters of the list hold a
// space, an ampersand and an @, which the query escapes. The reply of the list is the paginated
// object, whose counts the page holds, and the reply of an approval is a user.
func userInviteCases() []wireCase {
	const (
		invitesRoute = "/v2/facts/{proj_id}/{env_id}/user_invites"
		inviteRoute  = invitesRoute + "/{user_invite_id}"
		approveRoute = inviteRoute + "/approve"
		invites      = "/v2/facts/p/e/user_invites"
	)
	forbidden := func(op, method, route string) *permit.Error {
		e := projectError(op, method, route, http.StatusForbidden, inviteForbiddenBody)
		e.Code, e.Title, e.RequestID = "FORBIDDEN", "Forbidden", "req-41"
		e.Detail = "the API key may not act on the user invites of the environment"
		return e
	}
	listErr := projectError("list_user_invites", http.MethodGet, invitesRoute,
		http.StatusUnprocessableEntity, perPageBody)
	listErr.Detail = "query.per_page: ensure this value is less than or equal to 100"
	getErr := projectError("get_user_invite", http.MethodGet, inviteRoute, http.StatusNotFound,
		inviteNotFoundBody)
	getErr.Code, getErr.Title, getErr.RequestID = "NOT_FOUND", "Not Found", "req-42"
	getErr.Detail = "the user invite does not exist"
	createErr := projectError("create_user_invite", http.MethodPost, invitesRoute,
		http.StatusConflict, inviteConflictBody)
	createErr.Code, createErr.Title, createErr.RequestID = "DUPLICATE_ENTITY", "Conflict",
		"req-43"
	createErr.Detail = "a user invite with the key alice@example.com exists already"
	updateErr := projectError("update_user_invite", http.MethodPatch, inviteRoute,
		http.StatusUnprocessableEntity, inviteEmailMissingBody)
	updateErr.Detail = "body.email: field required"
	approveErr := projectError("approve_user_invite", http.MethodPost, approveRoute,
		http.StatusBadRequest, inviteApprovedBody)
	approveErr.Detail = "the user invite is approved already"
	return []wireCase{
		{
			Method: "UserInvitesService.List",
			Op:     "list_user_invites",
			Route:  "GET " + invitesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.UserInvites.List(ctx, &permit.UserInvitesListOptions{
					ListOptions: permit.ListOptions{Page: 2, PerPage: 10},
					Role:        "editor", Tenant: "tenant a&b", Search: "alice@example.com",
				})
			},
			Request: wireRequest{
				RawPath: invites,
				RawQuery: "page=2&per_page=10&role=editor&search=alice%40example.com" +
					"&tenant=tenant+a%26b",
			},
			Reply: mockpermit.Reply{Body: replyFixture("list_user_invites.200.json")},
			WantResult: &permit.Page[models.ElementsUserInviteRead]{
				Data: inviteListed(), Page: 2, PerPage: 10, TotalCount: new(2), PageCount: new(2),
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusUnprocessableEntity, Body: perPageBody},
			WantErr:    listErr,
		},
		{
			Method: "UserInvitesService.All",
			Op:     "list_user_invites",
			Route:  "GET " + invitesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstOf(c.UserInvites.All(ctx, nil), 2)
			},
			Request:    wireRequest{RawPath: invites, RawQuery: "page=1&per_page=100"},
			Reply:      mockpermit.Reply{Body: replyFixture("list_user_invites.200.json")},
			WantResult: inviteListed(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: inviteForbiddenBody},
			WantErr:    forbidden("list_user_invites", http.MethodGet, invitesRoute),
		},
		{
			Method: "UserInvitesService.Get",
			Op:     "get_user_invite",
			Route:  "GET " + inviteRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.UserInvites.Get(ctx, "a/b")
			},
			Request:    wireRequest{RawPath: invites + "/a%2Fb"},
			Reply:      mockpermit.Reply{Body: replyFixture("get_user_invite.200.json")},
			WantResult: inviteFixture(),
			ErrorReply: mockpermit.Reply{Status: http.StatusNotFound, Body: inviteNotFoundBody},
			WantErr:    getErr,
		},
		{
			Method: "UserInvitesService.Create",
			Op:     "create_user_invite",
			Route:  "POST " + invitesRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.UserInvites.Create(ctx, models.ElementsUserInviteCreate{
					Key: "alice@example.com", Status: models.UserInviteStatusPending,
					Email: "alice@example.com", FirstName: "Alice", LastName: "Liddell",
					RoleID: inviteRoleID, TenantID: inviteTenantID,
					ResourceInstanceID: inviteInstanceID,
				})
			},
			Request:    wireRequest{RawPath: invites, Body: inviteBody("Liddell")},
			Reply:      mockpermit.Reply{Body: replyFixture("create_user_invite.200.json")},
			WantResult: inviteFixture(),
			ErrorReply: mockpermit.Reply{Status: http.StatusConflict, Body: inviteConflictBody},
			WantErr:    createErr,
		},
		{
			Method: "UserInvitesService.Update",
			Op:     "update_user_invite",
			Route:  "PATCH " + inviteRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.UserInvites.Update(ctx, ".hidden", models.ElementsUserInviteUpdate{
					Key: "alice@example.com", Status: models.UserInviteStatusPending,
					Email: "alice@example.com", FirstName: "Alice", LastName: "Hargreaves",
					RoleID: inviteRoleID, TenantID: inviteTenantID,
					ResourceInstanceID: inviteInstanceID,
				})
			},
			Request:    wireRequest{RawPath: invites + "/.hidden", Body: inviteBody("Hargreaves")},
			Reply:      mockpermit.Reply{Body: replyFixture("update_user_invite.200.json")},
			WantResult: inviteUpdated(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: inviteEmailMissingBody,
			},
			WantErr: updateErr,
		},
		{
			Method: "UserInvitesService.Approve",
			Op:     "approve_user_invite",
			Route:  "POST " + approveRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				approval := models.ElementsUserInviteApprove{
					Email: "alice@example.com", Key: "alice@example.com",
					Attributes: models.Attributes{"department": "finance"},
				}
				return c.UserInvites.Approve(ctx, fixtures.UUIDKey, approval)
			},
			Request: wireRequest{
				RawPath: invites + "/" + fixtures.UUIDKey + "/approve",
				Body: `{"email":"alice@example.com","key":"alice@example.com",` +
					`"attributes":{"department":"finance"}}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("approve_user_invite.200.json")},
			WantResult: inviteApprovedUser(),
			ErrorReply: mockpermit.Reply{Status: http.StatusBadRequest, Body: inviteApprovedBody},
			WantErr:    approveErr,
		},
		{
			Method: "UserInvitesService.Delete",
			Op:     "delete_user_invite",
			Route:  "DELETE " + inviteRoute,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return wireNoResult{}, c.UserInvites.Delete(ctx, "50%é")
			},
			Request:    wireRequest{RawPath: invites + "/50%25%C3%A9"},
			Reply:      mockpermit.Reply{Status: http.StatusNoContent},
			WantResult: wireNoResult{},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: inviteForbiddenBody},
			WantErr:    forbidden("delete_user_invite", http.MethodDelete, inviteRoute),
		},
	}
}
