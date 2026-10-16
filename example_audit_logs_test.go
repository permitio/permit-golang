package permit_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
)

// List the audit logs of the checks of a resource, reading every page, and read one with the
// objects that its decision names. A decision that is set is true for allowed and false for
// denied. The result of a decision is JSON, which encoding/json reads into a type of your own.
func ExampleAuditLogsService() {
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

	ctx := context.Background()
	opts := &permit.AuditLogsListOptions{Resources: []string{"document"}}
	for log, err := range client.AuditLogs.All(ctx, opts) {
		if err != nil {
			fmt.Println(err)
			return
		}
		allowed := log.Decision != nil && *log.Decision
		fmt.Println(log.UserKey, log.Action, log.ResourceType, "allowed:", allowed)
	}

	log, err := client.AuditLogs.Get(ctx, "00000000-0000-4000-8000-000000000501")
	if errors.Is(err, permit.ErrNotFound) {
		fmt.Println("the environment has no such audit log")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	var result struct {
		Allow bool `json:"allow"`
	}
	if err := json.Unmarshal(log.Result, &result); err != nil {
		fmt.Println(err)
		return
	}
	if user, tenant := log.Objects.UserObject, log.Objects.TenantObject; user != nil &&
		tenant != nil {
		fmt.Println(user.Email, "in", tenant.Name, "allow:", result.Allow)
	}
	// Output:
	// idp|alice read document allowed: true
	// idp|bob delete document allowed: false
	// alice@example.com in Default allow: true
}
