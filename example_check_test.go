package permit_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2"
)

// Ask the PDP whether a user may read one document, with the context of the request. Check
// returns false only when the PDP denies; a reply that is not a decision is an error that matches
// ErrMalformedDecision, never a silent false.
func ExampleClient_Check() {
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

	allowed, err := client.Check(context.Background(),
		permit.User{Key: "alice", Attributes: map[string]any{"department": "finance"}},
		"read",
		permit.Resource{Type: "document", Key: "2026-report", Tenant: "tenant-a"},
		permit.WithDecisionContext(map[string]any{"business_hours": true}),
	)
	if errors.Is(err, permit.ErrMalformedDecision) {
		fmt.Println("the PDP answered with no decision")
		return
	}
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("alice may read the report:", allowed)
	// Output:
	// alice may read the report: true
}

// Ask the PDP for several decisions in one request, for different users, actions, resources and
// tenants. The decisions come back in the order of the checks.
func ExampleClient_BulkCheck() {
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

	decisions, err := client.BulkCheck(context.Background(), []permit.CheckRequest{
		{
			User:     permit.User{Key: "alice"},
			Action:   "read",
			Resource: permit.Resource{Type: "document", Key: "2026-report", Tenant: "tenant-a"},
		},
		{
			User:     permit.User{Key: "bob"},
			Action:   "delete",
			Resource: permit.Resource{Type: "document", Key: "memo", Tenant: "tenant-b"},
			Context:  map[string]any{"business_hours": false},
		},
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(decisions)
	// Output:
	// [true false]
}

// Keep the documents that a user may read, each checked as its own instance, in one request to
// the PDP. FilterObjects returns the allowed resources as they were given.
func ExampleClient_FilterObjects() {
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

	documents := []permit.Resource{
		{Type: "document", Key: "2026-report", Tenant: "tenant-a"},
		{Type: "document", Key: "memo", Tenant: "tenant-a"},
	}
	readable, err := client.FilterObjects(context.Background(), permit.User{Key: "alice"},
		"read", documents)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, document := range readable {
		fmt.Println(document.Key)
	}
	// Output:
	// 2026-report
}
