package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// Invite alice@example.com with a role, a tenant and a resource instance of the environment, each
// by its ID, list the invites, reading every page, approve the invite with an attribute of the
// user, which returns the user of the API's reply, and delete the invite. The calls of
// UserInvitesService act on the environment of the client's scope: WithScope gives it here, and
// without it, the client looks up the scope of its environment-level API key.
func ExampleUserInvitesService() {
	client, err := permit.New(
		permit.WithAPIKey(exampleAPIKey),
		permit.WithAPIURL(exampleAPIURL),
		permit.WithPDPURL(examplePDPURL),
		permit.WithScope("my-project", "production"),
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer client.Close()

	ctx := context.Background()
	invite, err := client.UserInvites.Create(ctx, models.ElementsUserInviteCreate{
		Key:                "alice@example.com",
		Status:             models.UserInviteStatusPending,
		Email:              "alice@example.com",
		FirstName:          "Alice",
		LastName:           "Liddell",
		RoleID:             "00000000-0000-4000-8000-000000000411",
		TenantID:           "00000000-0000-4000-8000-000000000412",
		ResourceInstanceID: "00000000-0000-4000-8000-000000000413",
	})
	if errors.Is(err, permit.ErrConflict) {
		fmt.Println("the environment has an invite of alice@example.com already")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("invited", invite.Email, "as", invite.Status)

	for invite, err := range client.UserInvites.All(ctx, nil) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println("invite:", invite.Email, invite.Status)
	}

	user, err := client.UserInvites.Approve(ctx, invite.ID, models.ElementsUserInviteApprove{
		Email:      invite.Email,
		Key:        invite.Key,
		Attributes: models.Attributes{"department": "finance"},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("approved, the user", user.Key, "of", user.Attributes["department"])

	if err := client.UserInvites.Delete(ctx, invite.ID); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("deleted the invite")
	// Output:
	// invited alice@example.com as pending
	// invite: alice@example.com pending
	// invite: bob@example.com approved
	// approved, the user alice@example.com of finance
	// deleted the invite
}
