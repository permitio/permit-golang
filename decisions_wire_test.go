package permit_test

import (
	"context"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// The error bodies of the PDP in the wire cases of the decisions: the cloud PDP's, with a code
// and a message, and the PDP container's, with a detail.
const (
	pdpBadRequestBody  = `{"code":400,"message":"the query is not valid"}`
	pdpMisdirectedBody = `{"detail":"the PDP does not serve this version of the SDK"}`
	pdpValidationBody  = `{"detail":[{"loc":["body",0,"resource","type"],` +
		`"msg":"field required","type":"value_error.missing"}]}`
)

// The resources of the wire cases of the decisions: two instances of a type, in two tenants.
var (
	wireReport = permit.Resource{
		Type: "document", Key: "2026-report", Tenant: "tenant-a",
		Attributes: map[string]any{"pages": 12},
	}
	wireMemo = permit.Resource{Type: "document", Key: "memo", Tenant: "tenant-b"}
)

// decisionCases returns the wire cases of the PDP's decisions, which go to the PDP and never to
// the API.
func decisionCases() []wireCase {
	return []wireCase{
		{
			Method: "Client.Check",
			Op:     "is_allowed_allowed_post",
			Route:  "POST /allowed",
			Modes:  []wireMode{pdpMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Check(
					ctx,
					permit.User{Key: "alice", Attributes: map[string]any{"age": 30}},
					"read",
					wireReport,
					permit.WithDecisionContext(map[string]any{"now": 1730000000, "trusted": true}),
				)
			},
			Request: wireRequest{
				RawPath: "/allowed",
				Body: `{"user":{"key":"alice","attributes":{"age":30}},"action":"read",` +
					`"resource":{"type":"document","key":"2026-report","tenant":"tenant-a",` +
					`"attributes":{"pages":12}},"context":{"now":1730000000,"trusted":true}}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("is_allowed_allowed_post.200.json")},
			WantResult: true,
			ErrorReply: mockpermit.Reply{Status: http.StatusBadRequest, Body: pdpBadRequestBody},
			WantErr: &permit.Error{
				Op: "is_allowed_allowed_post", Method: http.MethodPost, Route: "/allowed",
				StatusCode: http.StatusBadRequest, Detail: "the query is not valid",
				Body: []byte(pdpBadRequestBody),
			},
		},
		{
			Method: "Client.BulkCheck",
			Op:     "is_allowed_bulk_allowed_bulk_post",
			Route:  "POST /allowed/bulk",
			Modes:  []wireMode{pdpMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.BulkCheck(ctx, []permit.CheckRequest{
					{User: permit.User{Key: "alice"}, Action: "read", Resource: wireReport},
					{
						User: permit.User{Key: "bob"}, Action: "edit", Resource: wireMemo,
						Context: map[string]any{"ip": "192.0.2.1"},
					},
				})
			},
			Request: wireRequest{
				RawPath: "/allowed/bulk",
				Body: `[{"user":{"key":"alice"},"action":"read","resource":{"type":"document",` +
					`"key":"2026-report","tenant":"tenant-a","attributes":{"pages":12}}},` +
					`{"user":{"key":"bob"},"action":"edit","resource":{"type":"document",` +
					`"key":"memo","tenant":"tenant-b"},"context":{"ip":"192.0.2.1"}}]`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("is_allowed_bulk_allowed_bulk_post.200.json"),
			},
			WantResult: []bool{true, false},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusMisdirectedRequest, Body: pdpMisdirectedBody,
			},
			WantErr: &permit.Error{
				Op: "is_allowed_bulk_allowed_bulk_post", Method: http.MethodPost,
				Route: "/allowed/bulk", StatusCode: http.StatusMisdirectedRequest,
				Detail: "the PDP does not serve this version of the SDK",
				Body:   []byte(pdpMisdirectedBody),
			},
		},
		{
			Method: "Client.FilterObjects",
			Op:     "is_allowed_bulk_allowed_bulk_post",
			Route:  "POST /allowed/bulk",
			Modes:  []wireMode{pdpMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.FilterObjects(ctx, permit.User{Key: "alice"}, "read",
					[]permit.Resource{wireReport, wireMemo},
					permit.WithDecisionContext(map[string]any{"now": 1730000000}))
			},
			Request: wireRequest{
				RawPath: "/allowed/bulk",
				Body: `[{"user":{"key":"alice"},"action":"read","resource":{"type":"document",` +
					`"key":"2026-report","tenant":"tenant-a","attributes":{"pages":12}},` +
					`"context":{"now":1730000000}},` +
					`{"user":{"key":"alice"},"action":"read","resource":{"type":"document",` +
					`"key":"memo","tenant":"tenant-b"},"context":{"now":1730000000}}]`,
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("is_allowed_bulk_allowed_bulk_post.200.json"),
			},
			WantResult: []permit.Resource{wireReport},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: pdpValidationBody,
			},
			WantErr: &permit.Error{
				Op: "is_allowed_bulk_allowed_bulk_post", Method: http.MethodPost,
				Route: "/allowed/bulk", StatusCode: http.StatusUnprocessableEntity,
				Detail: "body.0.resource.type: field required", Body: []byte(pdpValidationBody),
			},
		},
	}
}
