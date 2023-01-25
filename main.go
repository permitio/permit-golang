package main

import (
	"fmt"
	"go.uber.org/zap"
	"net/http"

	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/enforcement"
	"github.com/permitio/permit-golang/pkg/permit"
)

const (
	port         = 4000
	resourceName = "document"
)

func main() {
	// This line initializes the SDK and connects your Go app
	// to the Permit.io PDP container (you've set in the previous step), with the API key provided.
	permitClient := permit.NewPermit(
		// Building new config for Permit client
		config.NewConfigBuilder(
			// your api key
			"<PUT_TOKEN_HERE").
			// Set the PDP URL
			WithLogger(zap.NewExample()).
			WithPdpUrl("http://localhost:7766").
			Build(),
	)

	// You can open http://localhost:4000 to invoke this http
	// endpoint, and see the outcome of the permission check.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// This user was defined by you in the previous step and
		// is already assigned with a role in the permission system.
		user := enforcement.UserBuilder("user-key").
			WithFirstName("first").
			WithLastName("last").
			WithEmail("email@wow.com").
			Build()

		// This resource was defined by you in the previous step
		// and is already has actions assigned in the permission system.
		resource := enforcement.ResourceBuilder(resourceName).Build()

		// After we created this user in the previous step, we also synced the user's identifier
		// to permit.io servers with permitClient.syncUser(user)).
		// The user identifier can be anything (email, db id, etc) but must be unique for each user.
		// Now that the user is synced, we can use its identifier to check permissions with `permit.check()`.
		permitted, err := permitClient.Check(user, "read", resource)
		if err != nil {
			fmt.Println(err)
			return
		}
		if permitted {
			w.WriteHeader(http.StatusOK)
			_, err = w.Write([]byte(user.FirstName + " " + user.LastName + " is PERMITTED to read doc!"))
		} else {
			w.WriteHeader(http.StatusForbidden)
			_, err = w.Write([]byte(fmt.Sprintf(user.FirstName + " " + user.LastName + " is NOT PERMITTED to read doc!")))
		}
	})
	fmt.Printf("Listening on http://localhost:%d", port)
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}
