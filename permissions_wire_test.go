package permit_test

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// The error body of the PDP in the wire cases of the lists of who may do what: the cloud PDP's,
// with a code and a message, for an API key that may not read the environment.
const pdpForbiddenBody = `{"code":403,"message":"the API key may not read this environment"}`

// permissionCases returns the wire cases of the PDP's lists of who may do what: the permissions
// of a user, and the users that may do an action. They go to the PDP and never to the API.
func permissionCases() []wireCase {
	return []wireCase{
		{
			Method: "Client.UserPermissions",
			Op:     "Get_User_Permissions_user_permissions_post",
			Route:  "POST /user-permissions",
			Modes:  []wireMode{pdpMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.UserPermissions(
					ctx,
					permit.User{Key: "alice", Attributes: map[string]any{"age": 30}},
					permit.WithTenants("tenant-a"),
					permit.WithResources("document:2026-report"),
					permit.WithResourceTypes("document", "__tenant"),
					permit.WithDecisionContext(map[string]any{"now": 1730000000, "vip": true}),
				)
			},
			Request: wireRequest{
				RawPath: "/user-permissions",
				Body: `{"user":{"key":"alice","attributes":{"age":30}},"tenants":["tenant-a"],` +
					`"resources":["document:2026-report"],` +
					`"resource_types":["document","__tenant"],` +
					`"context":{"now":1730000000,"vip":true}}`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("Get_User_Permissions_user_permissions_post.200.json"),
			},
			WantResult: map[string]permit.ObjectPermissions{
				"__tenant:tenant-a": {
					Tenant: &permit.TenantDetails{
						Key: "tenant-a", Attributes: map[string]any{"plan": "pro"},
					},
					Permissions: []string{"document:read"},
					Roles:       []string{"viewer"},
				},
				"document:2026-report": {
					Tenant: &permit.TenantDetails{Key: "tenant-a", Attributes: map[string]any{}},
					Resource: &permit.ResourceDetails{
						Type: "document", Key: "2026-report",
						Attributes: map[string]any{"pages": json.Number("12")},
					},
					Permissions: []string{"document:read", "document:update"},
					Roles:       []string{"editor"},
				},
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: pdpForbiddenBody},
			WantErr: &permit.Error{
				Op: "Get_User_Permissions_user_permissions_post", Method: http.MethodPost,
				Route: "/user-permissions", StatusCode: http.StatusForbidden,
				Detail: "the API key may not read this environment",
				Body:   []byte(pdpForbiddenBody),
			},
		},
		{
			Method: "Client.AuthorizedUsers",
			Op:     "authorized_users_authorized_users_post",
			Route:  "POST /authorized_users",
			Modes:  []wireMode{pdpMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.AuthorizedUsers(ctx, "read", wireReport,
					permit.WithDecisionContext(map[string]any{"now": 1730000000}))
			},
			Request: wireRequest{
				RawPath: "/authorized_users",
				Body: `{"action":"read","resource":{"type":"document","key":"2026-report",` +
					`"tenant":"tenant-a","attributes":{"pages":12}},` +
					`"context":{"now":1730000000}}`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("authorized_users_authorized_users_post.200.json"),
			},
			WantResult: permit.AuthorizedUsersResult{
				Resource: "document:2026-report",
				Tenant:   "tenant-a",
				Users: map[string][]permit.AuthorizedUserAssignment{
					"alice": {{
						User: "alice", Tenant: "tenant-a", Resource: "document:2026-report",
						Role: "editor",
					}},
					"bob": {{
						User: "bob", Tenant: "tenant-a", Resource: "__tenant:tenant-a",
						Role: "viewer",
					}},
				},
			},
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: pdpForbiddenBody},
			WantErr: &permit.Error{
				Op: "authorized_users_authorized_users_post", Method: http.MethodPost,
				Route: "/authorized_users", StatusCode: http.StatusForbidden,
				Detail: "the API key may not read this environment",
				Body:   []byte(pdpForbiddenBody),
			},
		},
	}
}
