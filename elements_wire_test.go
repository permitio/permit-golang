package permit_test

import (
	"context"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// elementsForbiddenBody is the error body of the Permit API in the wire case of the login of
// Permit Elements: a key that may not log a user in.
const elementsForbiddenBody = `{"error_code":"FORBIDDEN","title":"Forbidden",` +
	`"message":"the API key may not log a user in to Permit Elements","id":"req-50"}`

// fixtureLogin is the login of the reply fixture elements_login_as.200.json.
func fixtureLogin() *models.EmbeddedLoginRequestOutput {
	return &models.EmbeddedLoginRequestOutput{
		Token: models.NewSecret("fake-login-token-000000"),
		RedirectURL: models.NewSecret(
			"https://api.example.com/v2/auth/login_elements?token=fake-login-token-000000"),
		ElementBearerToken: models.NewSecret("fake-bearer-token-000000"),
	}
}

// elementsCases returns the wire case of the login of Permit Elements. It sends one request to
// the API, along a route without a project or an environment, so the runner gives the client no
// scope, and the call sends no lookup of the scope, which the server would reject. The user and
// the tenant hold characters that an encoder may escape, <, > and &, a slash, a percent sign, a
// letter outside ASCII and U+2028; the body sends the same values, escaped as encoding/json
// writes them, which keeps <, > and & and escapes U+2028.
func elementsCases() []wireCase {
	const route = "/v2/auth/elements_login_as"
	loginErr := projectError("elements_login_as", http.MethodPost, route, http.StatusForbidden,
		elementsForbiddenBody)
	loginErr.Code, loginErr.Title, loginErr.RequestID = "FORBIDDEN", "Forbidden", "req-50"
	loginErr.Detail = "the API key may not log a user in to Permit Elements"
	return []wireCase{
		{
			Method: "ElementsService.LoginAs",
			Op:     "elements_login_as",
			Route:  "POST " + route,
			Modes:  []wireMode{apiMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.Elements.LoginAs(ctx, models.UserLoginRequestInput{
					UserID: "a/b <&>", TenantID: "50%é" + fixtures.LineSeparator,
				})
			},
			Request: wireRequest{
				RawPath: route, Body: `{"user_id":"a/b <&>","tenant_id":"50%é\u2028"}`,
			},
			Reply:      mockpermit.Reply{Body: replyFixture("elements_login_as.200.json")},
			WantResult: fixtureLogin(),
			ErrorReply: mockpermit.Reply{Status: http.StatusForbidden, Body: elementsForbiddenBody},
			WantErr:    loginErr,
		},
	}
}
