package permit

import (
	"context"

	"github.com/permitio/permit-golang/v2/models"
)

// opElementsLoginAs is the ID of the operation of the login to Permit Elements.
const opElementsLoginAs = "elements_login_as"

// ElementsService logs a user of the application in to Permit Elements, the components of a user
// interface that Permit serves for an application to embed; [Client.Elements] holds it.
//
// Its calls need no scope: the route names no project and no environment, so they send no lookup
// of the scope of the API key, and read none of [WithScope]. The published spec of the API does
// not list the route, so the SDK sends it as v1 and the other Permit SDKs do.
type ElementsService struct {
	client *Client
}

// LoginAs logs in to Permit Elements as the user that login names, in its tenant, with
// POST /v2/auth/elements_login_as, and returns the reply: the URL to send the user's browser to,
// which completes the login, and the tokens of the login. The request's body holds the UserID
// and the TenantID of login, each the key or the ID, both sent also when empty.
//
// LoginAs is sent with an environment-level API key, of the environment of the user and the
// tenant: the route names no project and no environment. What the API answers to a key of a wider
// level is not confirmed yet. The call sends no lookup of the scope of the API key and reads none
// of [WithScope], and with [WithFactsViaPDP] it still goes to the API. It is sent again only
// after a 429, which the API did not act on: after a 500, 502, 503 or 504 reply or a failed
// connection, the API may have logged the user in, so LoginAs returns that error.
//
// The RedirectURL, the Token and the ElementBearerToken of the reply are each a [models.Secret],
// which prints a marker in place of the credential: read it with [models.Secret.Value].
// encoding/json writes the marker for each of the three too, so code that sends the login to a
// browser as JSON builds that JSON from the Value of each field. A reply in 200-299 whose Error
// and ErrorCode say that the login failed is returned as it is, with no error. Which fields the
// API sets in each case is not confirmed yet.
//
// The client keeps each of the three out of its later errors and log lines, as it does its API
// key: each login's token, bearer token and redirect URL stay in the client's set of credentials
// for the life of the client. A server that logs many users in through one client keeps the
// credentials of each login in that set, and scrubs each later error and log line of the client
// of all of them.
//
// LoginAs returns an error, and no reply:
//
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrForbidden] for a key that may not log a user in, or one whose
//     Detail names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a login: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ElementsService) LoginAs(ctx context.Context, login models.UserLoginRequestInput,
) (*models.EmbeddedLoginRequestOutput, error) {
	r := request{op: opElementsLoginAs, target: apiTarget, body: login}
	return do[models.EmbeddedLoginRequestOutput](ctx, s.client, r)
}
