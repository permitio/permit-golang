package permit_test

import (
	"context"
	"fmt"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// Ask every PDP of the environment of the client's scope to read its data again, such as after a
// change in a source of data outside Permit. The reply names the update and the PDPs that it
// asks, and does not wait until they have read the data.
func ExamplePDPsService() {
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

	refresh, err := client.PDPs.Refresh(context.Background(), models.PDPDataRefreshRequest{
		Reason: "the billing data changed",
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("update", refresh.UpdateID)
	fmt.Println("asks", len(refresh.PDPIDs), "PDPs")
	// Output:
	// update 00000000-0000-4000-8000-000000000701
	// asks 2 PDPs
}
