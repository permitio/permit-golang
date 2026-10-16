package permit_test

import (
	"context"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// The error bodies of the PDP container in the wire cases of the decisions that only it serves:
// a validation error of the body, as the container answers a request that it cannot read.
const (
	pdpResourceTypeBody = `{"detail":[{"loc":["body","resource","type"],` +
		`"msg":"field required","type":"value_error.missing"}]}`
	pdpUserKeyBody = `{"detail":[{"loc":["body","user","key"],` +
		`"msg":"field required","type":"value_error.missing"}]}`
	pdpURLBody = `{"detail":[{"loc":["body","url"],` +
		`"msg":"ensure this value has at least 1 characters","type":"value_error"}]}`
)

// containerCases returns the wire cases of the decisions that only the PDP container serves: the
// tenants in which a check is allowed, the tenants of a user and the check of a URL. They go to
// the PDP and never to the API.
func containerCases() []wireCase {
	return []wireCase{
		{
			Method: "Client.AllTenantsCheck",
			Op:     "is_allowed_all_tenants_allowed_all_tenants_post",
			Route:  "POST /allowed/all-tenants",
			Modes:  []wireMode{pdpMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.AllTenantsCheck(
					ctx,
					permit.User{Key: "alice", Attributes: map[string]any{"age": 30}},
					"read",
					permit.Resource{Type: "document", Attributes: map[string]any{"pages": 12}},
					permit.WithDecisionContext(map[string]any{"now": 1730000000}),
				)
			},
			Request: wireRequest{
				RawPath: "/allowed/all-tenants",
				Body: `{"user":{"key":"alice","attributes":{"age":30}},"action":"read",` +
					`"resource":{"type":"document","tenant":"default","attributes":{"pages":12}},` +
					`"context":{"now":1730000000}}`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("is_allowed_all_tenants_allowed_all_tenants_post.200.json"),
			},
			WantResult: []permit.TenantDetails{
				{Key: "tenant-a", Attributes: map[string]any{"plan": "pro"}},
			},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: pdpResourceTypeBody,
			},
			WantErr: &permit.Error{
				Op: "is_allowed_all_tenants_allowed_all_tenants_post", Method: http.MethodPost,
				Route: "/allowed/all-tenants", StatusCode: http.StatusUnprocessableEntity,
				Detail: "body.resource.type: field required", Body: []byte(pdpResourceTypeBody),
			},
		},
		{
			Method: "Client.UserTenants",
			Op:     "Get_User_Tenants_user_tenants_post",
			Route:  "POST /user-tenants",
			Modes:  []wireMode{pdpMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.UserTenants(ctx,
					permit.User{Key: "alice", Email: "alice@example.com"},
					permit.WithDecisionContext(map[string]any{"vip": true}))
			},
			Request: wireRequest{
				RawPath: "/user-tenants",
				Body: `{"user":{"key":"alice","email":"alice@example.com"},` +
					`"context":{"vip":true}}`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("Get_User_Tenants_user_tenants_post.200.json"),
			},
			WantResult: []permit.TenantDetails{
				{Key: "tenant-a", Attributes: map[string]any{"plan": "pro"}},
				{Key: "tenant-b", Attributes: map[string]any{}},
			},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: pdpUserKeyBody,
			},
			WantErr: &permit.Error{
				Op: "Get_User_Tenants_user_tenants_post", Method: http.MethodPost,
				Route: "/user-tenants", StatusCode: http.StatusUnprocessableEntity,
				Detail: "body.user.key: field required", Body: []byte(pdpUserKeyBody),
			},
		},
		{
			Method: "Client.CheckURL",
			Op:     "is_allowed_url_allowed_url_post",
			Route:  "POST /allowed_url",
			Modes:  []wireMode{pdpMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.CheckURL(ctx, permit.User{Key: "alice"}, http.MethodGet,
					"https://app.example.com/documents/2026-report?view=full", "tenant-a",
					permit.WithDecisionContext(map[string]any{"business_hours": true}))
			},
			Request: wireRequest{
				RawPath: "/allowed_url",
				Body: `{"user":{"key":"alice"},"http_method":"GET",` +
					`"url":"https://app.example.com/documents/2026-report?view=full",` +
					`"tenant":"tenant-a","context":{"business_hours":true}}`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("is_allowed_url_allowed_url_post.200.json"),
			},
			WantResult: true,
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: pdpURLBody,
			},
			WantErr: &permit.Error{
				Op: "is_allowed_url_allowed_url_post", Method: http.MethodPost,
				Route: "/allowed_url", StatusCode: http.StatusUnprocessableEntity,
				Detail: "body.url: ensure this value has at least 1 characters",
				Body:   []byte(pdpURLBody),
			},
		},
	}
}
