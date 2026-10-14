package permit

import (
	"context"
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

// NewTestClient is newTestClient for the tests of package permit_test, such as those of the
// wire-case runner, which run test-only operations.
var NewTestClient = newTestClient

// CallFacts makes a call of the facts operation op, a test-only operation of the client's
// registry, as a method of a facts service does, for the tests of package permit_test: params
// fill the placeholders of its route after {proj_id} and {env_id}, and the call reads no value
// from the reply.
func (c *Client) CallFacts(ctx context.Context, op string, params ...string) error {
	r, err := facts{client: c}.prepare(ctx, request{op: op, params: params})
	if err != nil {
		return err
	}
	return c.doNoValue(ctx, r)
}
