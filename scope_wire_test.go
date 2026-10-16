package permit_test

import (
	"context"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// unauthorizedBody is the API's error body for an API key that it does not accept.
const unauthorizedBody = `{"error_code":"UNAUTHORIZED","title":"Unauthorized",` +
	`"message":"the API key is not valid","id":"req-1"}`

// scopeCases returns the wire cases of the scope of the client's API key.
func scopeCases() []wireCase {
	return []wireCase{{
		Method: "Client.Scope",
		Op:     "get_api_key_scope",
		Route:  "GET /v2/api-key/scope",
		Modes:  []wireMode{apiMode},
		Call: func(ctx context.Context, c *permit.Client) (any, error) {
			return c.Scope(ctx)
		},
		Request:    wireRequest{RawPath: "/v2/api-key/scope"},
		Reply:      mockpermit.Reply{Body: replyFixture("get_api_key_scope.200.json")},
		WantResult: permit.Scope{Project: exampleProjectID, Environment: exampleEnvironmentID},
		ErrorReply: mockpermit.Reply{Status: http.StatusUnauthorized, Body: unauthorizedBody},
		WantErr: &permit.Error{
			Op: "get_api_key_scope", Method: http.MethodGet, Route: "/v2/api-key/scope",
			StatusCode: http.StatusUnauthorized, Code: "UNAUTHORIZED", Title: "Unauthorized",
			Detail: "the API key is not valid", RequestID: "req-1", Body: []byte(unauthorizedBody),
		},
	}}
}
