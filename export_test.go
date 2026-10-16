package permit

import (
	"testing"

	"github.com/permitio/permit-golang/v2/internal/rest"
)

// newTestClient returns a client that New builds with opts, whose registry holds the operations
// of rest.Default and ops, the test-only operations of the test, whose routes start with
// rest.TestPrefix. It fails the test when New fails or the registry rejects an op. Only test
// files build test-only operations into a client: rest.Default rejects them, and the clients
// that New builds hold rest.Default.
func newTestClient(tb testing.TB, opts []Option, ops ...rest.Op) *Client {
	tb.Helper()
	c := mustNew(tb, opts...)
	registry, err := c.registry.WithTestOps(ops...)
	if err != nil {
		tb.Fatalf("registering the test-only operations: %v", err)
	}
	c.registry = registry
	return c
}
