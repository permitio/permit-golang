package permit_test

import (
	"context"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// Log in to Permit Elements as a user in a tenant, and send the user's browser to the URL of the
// reply, which completes the login. The credentials of the reply print a marker in their place,
// and encoding/json writes the marker too; read each with its Value method, such as to redirect
// the browser with http.Redirect(w, r, login.RedirectURL.Value(), http.StatusFound), or to send
// the login to the browser as JSON, with json.NewEncoder(w).Encode(forBrowser).
func ExampleElementsService() {
	client, err := permit.New(
		permit.WithAPIKey(exampleAPIKey),
		permit.WithAPIURL(exampleAPIURL),
		permit.WithPDPURL(examplePDPURL),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer client.Close()

	login, err := client.Elements.LoginAs(context.Background(), models.UserLoginRequestInput{
		UserID: "alice", TenantID: "tenant-a",
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("token:", login.Token)
	fmt.Println("bearer token:", login.ElementBearerToken)
	fmt.Println("redirect URL:", login.RedirectURL)
	forBrowser := map[string]string{
		"token":                login.Token.Value(),
		"redirect_url":         login.RedirectURL.Value(),
		"element_bearer_token": login.ElementBearerToken.Value(),
	}
	fmt.Println("has a redirect URL:", forBrowser["redirect_url"] != "")
	// Output:
	// token: [REDACTED]
	// bearer token: [REDACTED]
	// redirect URL: [REDACTED]
	// has a redirect URL: true
}
