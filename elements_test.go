package permit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/redact"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The route of the login of Permit Elements, as mockpermit.Server.Handle takes it, and its path.
const (
	elementsLoginRoute = "POST /v2/auth/elements_login_as"
	elementsLoginPath  = "/v2/auth/elements_login_as"
)

// The credentials of the replies of the tests of the redaction of a login. They are not in the
// shape of a Permit API key, which the client hides whatever it holds, so only the client's Set
// hides them.
const (
	loginTokenValue    = "opaque-login-token"
	loginBearerValue   = "opaque-bearer-token"
	loginRedirectValue = "https://api.example.com/v2/auth/login_elements?code=opaque-redirect"
)

// loginReplyBody is a reply of the API to a login, with the credentials of the tests of the
// redaction.
const loginReplyBody = `{"token":"` + loginTokenValue + `","redirect_url":"` +
	loginRedirectValue + `","element_bearer_token":"` + loginBearerValue + `"}`

// someLogin is the login of the tests that do not test the body.
var someLogin = models.UserLoginRequestInput{UserID: "alice", TenantID: "tenant-a"}

// TestElementsLoginAsSendsOnlyTheLogin checks that LoginAs sends one request, to the route of the
// login, and no lookup of the scope of the API key, as the route names no project and no
// environment: without WithScope, with which a call that acts on an environment looks up the
// scope first; with WithScope, whose project and environment it does not read; and with
// WithFactsViaPDP, with which it still goes to the API, and nothing to the PDP.
func TestElementsLoginAsSendsOnlyTheLogin(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []Option
	}{
		{"without WithScope", nil},
		{"with WithScope", []Option{WithScope("scope-project", "scope-environment")}},
		{"with WithFactsViaPDP", []Option{WithFactsViaPDP()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(elementsLoginRoute, mockpermit.Reply{Body: loginReplyBody})
			c := projectClient(t, api, tc.opts...)
			if _, err := c.Elements.LoginAs(t.Context(), someLogin); err != nil {
				t.Fatalf("LoginAs returned %v", err)
			}
			requests := api.Requests()
			if len(requests) != 1 || requests[0].Method != http.MethodPost ||
				requests[0].RawPath != elementsLoginPath {
				t.Fatalf("LoginAs sent %+v, want only POST %s", requests, elementsLoginPath)
			}
			requests[0].ExpectJSON(t, `{"user_id":"alice","tenant_id":"tenant-a"}`)
		})
	}
}

// TestElementsLoginAsBodies checks the body of a login byte for byte against the golden files in
// testdata/golden: the zero UserLoginRequestInput sends both fields, which the API requires, as
// empty strings; and a user and a tenant with characters that an encoder may escape keep <, > and
// &, and send U+2028 and U+2029 escaped, as encoding/json writes them.
func TestElementsLoginAsBodies(t *testing.T) {
	for _, tc := range []struct {
		golden string
		login  models.UserLoginRequestInput
	}{
		{"elements_login_as.zero.golden", models.UserLoginRequestInput{}},
		{
			"elements_login_as.every-field.golden", models.UserLoginRequestInput{
				UserID: fixtures.UUIDKey,
				TenantID: fixtures.HTMLSpecial + " " + fixtures.LineSeparator +
					fixtures.ParagraphSeparator,
			},
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(elementsLoginRoute, mockpermit.Reply{Body: loginReplyBody})
			if _, err := projectClient(t, api).Elements.LoginAs(t.Context(), tc.login); err != nil {
				t.Fatalf("LoginAs returned %v", err)
			}
			requests := api.Requests()
			if len(requests) != 1 {
				t.Fatalf("LoginAs sent %d requests, want 1", len(requests))
			}
			requests[0].ExpectBytes(t, filepath.Join("testdata", "golden", tc.golden))
		})
	}
}

// TestElementsLoginCredentials checks the credentials of the reply to a login, its token, its
// bearer token and its redirect URL. The login that LoginAs returns holds each, which its Value
// method gives, and prints the marker in place of each with fmt's verbs, slog's text and JSON
// handlers and encoding/json. Each joins the client's Set: a later error of the client whose body
// echoes them outside a credential field, as a message can, holds the marker in place of each, in
// its body, its Detail and its text, and so do the client's Debug lines. A client that called
// nothing does not hold them.
func TestElementsLoginCredentials(t *testing.T) {
	values := []string{loginTokenValue, loginBearerValue, loginRedirectValue}
	echo := `{"detail":"the login ` + strings.Join(values, ", ") + ` has expired"}`
	logger, logged := debugLog()
	api := mockpermit.NewAPI(t)
	api.Handle(elementsLoginRoute, mockpermit.Reply{Body: loginReplyBody},
		mockpermit.Reply{Status: http.StatusBadRequest, Body: echo})
	c := projectClient(t, api, WithLogger(logger))
	for _, value := range values {
		if c.secrets.Contains(value) {
			t.Fatalf("the Set of a new client holds %s", value)
		}
	}
	ctx := t.Context()
	login, err := c.Elements.LoginAs(ctx, someLogin)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{
		login.Token.Value(), login.ElementBearerToken.Value(), login.RedirectURL.Value(),
	}
	if diff := cmp.Diff(values, got); diff != "" {
		t.Fatalf("the login holds credentials that differ (-want +got):\n%s", diff)
	}
	for _, value := range values {
		if !c.secrets.Contains(value) {
			t.Errorf("the credential %s of the login did not join the client's Set", value)
		}
	}

	var printed bytes.Buffer
	_, _ = fmt.Fprintf(&printed, "%v %+v %#v %s\n", *login, *login, *login, login.Token)
	text := slog.New(slog.NewTextHandler(&printed, nil))
	text.Info("the login", "login", *login, "credential", login.Token)
	jsonLog := slog.New(slog.NewJSONHandler(&printed, nil))
	jsonLog.Info("the login", "login", *login, "credential", login.Token)
	encoded, err := json.Marshal(login)
	if err != nil {
		t.Fatal(err)
	}
	printed.Write(encoded)
	scanner := canary.NewScanner(values...)
	_, _ = scanner.Write(printed.Bytes())
	scanner.Check(t)
	const markers = 6*3 + 3 // Six printings of the login, and three of the token alone.
	if n := bytes.Count(printed.Bytes(), []byte(redact.Marker)); n != markers {
		t.Errorf("the printed login shows the marker %d times, want it in place of each "+
			"credential, %d times:\n%s", n, markers, printed.Bytes())
	}

	_, err = c.Elements.LoginAs(ctx, someLogin)
	e := mustError(t, err)
	if e.StatusCode != http.StatusBadRequest || strings.Count(e.Detail, redact.Marker) != 3 ||
		!bytes.Contains(e.Body, []byte(redact.Marker)) {
		t.Errorf("the 400 returned %#v, want its detail and body with the marker in place of "+
			"each credential", e)
	}
	scanner = canary.NewScanner(values...)
	_, _ = fmt.Fprintf(scanner, "%+v %#v %s %s", err, e, e.Body, e.Detail)
	_, _ = scanner.Write(logged())
	scanner.Check(t)
	if !bytes.Contains(logged(), []byte(redact.Marker)) {
		t.Errorf("the Debug line of the 400 does not show the marker:\n%s", logged())
	}
}

// TestElementsLoginAsReturnsAFailedLogin checks that a reply in 200-299 that says that the login
// failed, with an error and its code and without a token, is returned as it is, with no error.
func TestElementsLoginAsReturnsAFailedLogin(t *testing.T) {
	api := mockpermit.NewAPI(t)
	api.Handle(elementsLoginRoute, mockpermit.Reply{
		Body: `{"error":"USER_NOT_FOUND","error_code":404,"extra":"the user is not in the tenant"}`,
	})
	login, err := projectClient(t, api).Elements.LoginAs(t.Context(), someLogin)
	if err != nil {
		t.Fatalf("LoginAs returned %v, want the reply", err)
	}
	want := &models.EmbeddedLoginRequestOutput{
		Error: "USER_NOT_FOUND", ErrorCode: http.StatusNotFound,
		Extra: "the user is not in the tenant",
	}
	if diff := cmp.Diff(want, login); diff != "" {
		t.Errorf("LoginAs returned a login that differs (-want +got):\n%s", diff)
	}
}

// TestElementsLoginAsRetries checks when LoginAs sends its request again, with the default policy
// of 3 retries: after a 429, which the API did not act on, and not after a 503 or a failed
// connection, after which the API may have logged the user in.
func TestElementsLoginAsRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	t.Run("after a 429", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(elementsLoginRoute,
			mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`},
			mockpermit.Reply{Body: loginReplyBody})
		if _, err := projectClient(t, api).Elements.LoginAs(t.Context(), someLogin); err != nil {
			t.Errorf("LoginAs returned %v, want the reply of the second request", err)
		}
		if n := len(api.Requests()); n != 2 {
			t.Errorf("LoginAs sent %d requests, want 2", n)
		}
	})
	t.Run("after a 503", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(elementsLoginRoute,
			mockpermit.Reply{Status: http.StatusServiceUnavailable, Header: again, Body: `{}`})
		_, err := projectClient(t, api).Elements.LoginAs(t.Context(), someLogin)
		if e := mustError(t, err); e.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("LoginAs returned %v, want the error of the 503", err)
		}
		if n := len(api.Requests()); n != 1 {
			t.Errorf("LoginAs sent %d requests, want 1", n)
		}
	})
	t.Run("after a failed connection", func(t *testing.T) {
		transport := &countingTransport{}
		c := projectClient(t, mockpermit.NewAPI(t),
			WithHTTPClient(&http.Client{Transport: transport}),
			WithRetry(RetryPolicy{MaxRetries: 1, MaxBackoff: 1}))
		_, err := c.Elements.LoginAs(t.Context(), someLogin)
		if e := mustError(t, err); e.StatusCode != 0 || transport.requests.Load() != 1 {
			t.Errorf("LoginAs over a connection that fails returned %v after %d attempts, want "+
				"an *Error without a status after 1", err, transport.requests.Load())
		}
	})
}

// TestMalformedElementsLoginReplies checks that a reply in 200-299 that is not a JSON object of a
// login, such as one whose token is not a string, is an *Error that names the model, and no
// login; and that the error does not hold the value of the token.
func TestMalformedElementsLoginReplies(t *testing.T) {
	for _, body := range []string{
		``, `null`, `[]`, `"a"`, `{"error_code":"x"}`, `{} {}`, `{"redirect_url":5}`,
		`{"token":{"value":"opaque-nested-token"}}`, `{"token":["opaque-listed-token"]}`,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(elementsLoginRoute, mockpermit.Reply{Body: body})
		login, err := projectClient(t, api).Elements.LoginAs(t.Context(), someLogin)
		want := "permit: elements_login_as: " + elementsLoginRoute + ": status 200: the reply " +
			"is not a valid models.EmbeddedLoginRequestOutput"
		if e := mustError(t, err); login != nil || e.Error() != want {
			t.Errorf("LoginAs of the reply %q returned %+v and\n%v\nwant no login and\n%s", body,
				login, err, want)
		}
		if strings.Contains(fmt.Sprintf("%+v", err), "opaque") {
			t.Errorf("the error of the reply %q holds its token: %+v", body, err)
		}
	}
}

// TestElementsLoginAsAfterClose checks that LoginAs returns ErrClosed after Close, and sends
// nothing.
func TestElementsLoginAsAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Elements.LoginAs(t.Context(), someLogin); !errors.Is(err, ErrClosed) {
		t.Errorf("LoginAs after Close returned %v, want ErrClosed", err)
	}
	api.AssertUntouched()
}
