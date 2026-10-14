package permit_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// unauthorizedBody is the API's error body for an API key that it does not accept.
const unauthorizedBody = `{"error_code":"UNAUTHORIZED","title":"Unauthorized",` +
	`"message":"the API key is not valid","id":"req-1"}`

// scopeCases returns the wire cases of the scope of the client's API key.
func scopeCases() []wireCase {
	return []wireCase{{
		Method:     "Client.Scope",
		Op:         "get_api_key_scope",
		Route:      "GET /v2/api-key/scope",
		Modes:      []wireMode{apiMode},
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

// scopeCall makes the call of method, the method of a scope wire case, with c.
func scopeCall(ctx context.Context, c *permit.Client, method string) (any, error) {
	switch method {
	case "Client.Scope":
		return c.Scope(ctx)
	default:
		return nil, errors.New("scopeCall has no call for " + method + "; add one")
	}
}

// TestScopeWire runs each case of scopeCases against mockpermit: the request that the method
// sends, the result it returns for the case's reply, and the error it returns for the case's
// error reply. Until the wire-case registry has its runner, a later commit, this test runs the
// cases of this file.
func TestScopeWire(t *testing.T) {
	for _, c := range scopeCases() {
		t.Run(c.Method, func(t *testing.T) {
			if !slices.Equal(c.Modes, []wireMode{apiMode}) {
				t.Fatalf("the case has the modes %q; the scope runner runs api alone", c.Modes)
			}
			method, _, _ := strings.Cut(c.Route, " ")

			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			api.Handle(c.Route, c.Reply)
			got, err := scopeCall(t.Context(), wireTestClient(t, api, pdp), c.Method)
			if err != nil {
				t.Fatalf("%s returned %v", c.Method, err)
			}
			if diff := cmp.Diff(c.WantResult, got); diff != "" {
				t.Errorf("%s returned a result that differs (-want +got):\n%s", c.Method, diff)
			}
			requests := api.Requests()
			if len(requests) != 1 {
				t.Fatalf("%s sent %d requests, want 1", c.Method, len(requests))
			}
			sent := requests[0]
			if sent.Method != method || sent.RawPath != c.Request.RawPath ||
				sent.RawQuery != c.Request.RawQuery || len(sent.Body) != 0 {
				t.Errorf("%s sent %s %s?%s with the body %q, want %s %s?%s and no body",
					c.Method, sent.Method, sent.RawPath, sent.RawQuery, sent.Body, method,
					c.Request.RawPath, c.Request.RawQuery)
			}
			pdp.AssertUntouched()

			failing := mockpermit.NewAPI(t)
			failing.Handle(c.Route, c.ErrorReply)
			_, err = scopeCall(t.Context(), wireTestClient(t, failing, pdp), c.Method)
			if diff := cmp.Diff(c.WantErr, err); diff != "" {
				t.Errorf("%s returned an error that differs (-want +got):\n%s", c.Method, diff)
			}
			pdp.AssertUntouched()
		})
	}
}
