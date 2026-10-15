//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework/seed"
)

// roleOnOneInstance checks the decisions on two instances of a resource, of which a user has a
// resource role on the first only: Check allows the first and denies the second, and
// FilterObjects keeps the first only.
func roleOnOneInstance(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		const tenant, user = "instance-tenant", "instance-user"
		s.seed(t, world{
			resources: []seed.Resource{{Key: "report", Actions: []string{"read"}}},
			resourceRoles: []seed.ResourceRole{
				{Resource: "report", Key: "report-reader", Permissions: []string{"read"}},
			},
			tenants: []string{tenant},
			users:   []string{user},
			instances: []seed.Instance{
				{Resource: "report", Key: "doc-1", Tenant: tenant},
				{Resource: "report", Key: "doc-2", Tenant: tenant},
			},
			assignments: []seed.RoleAssignment{{
				User: user, Role: "report-reader", Tenant: tenant,
				ResourceInstance: "report:doc-1",
			}},
		})
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		who := permit.User{Key: user}
		doc1 := permit.Resource{Type: "report", Key: "doc-1", Tenant: tenant}
		doc2 := permit.Resource{Type: "report", Key: "doc-2", Tenant: tenant}

		await(t, s, "Check of doc-1, on which the user has the role", true,
			func(ctx context.Context) (bool, error) { return c.Check(ctx, who, "read", doc1) })

		if allowed, err := c.Check(ctx, who, "read", doc2); err != nil || allowed {
			t.Errorf("Check of doc-2, on which the user has no role, = %t, %v; want false",
				allowed, err)
		}
		kept, err := c.FilterObjects(ctx, who, "read", []permit.Resource{doc1, doc2})
		if err != nil || !reflect.DeepEqual(kept, []permit.Resource{doc1}) {
			t.Errorf("FilterObjects of doc-1 and doc-2 = %+v, %v; want doc-1 only", kept, err)
		}
	}
}

// decisionContext checks the body of a check that carries a context, as the PDP gets it: the
// numbers of the context keep their digits, a number that a float64 cannot hold too, and its
// booleans their values; and the PDP answers it with 200 and a decision. The allow and the deny
// of an attribute-based rule that reads the context skip: tools/e2e/allowed-skips.txt names the
// ticket and what must happen first.
func decisionContext(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		log := &requestLog{}
		c := s.client(t, s.env.APIKey(), permit.WithHTTPClient(&http.Client{Transport: log}))
		values := map[string]any{
			"count": 42, "big": int64(9007199254740993), "ratio": 0.25,
			"precise": json.Number("12345678901234567890.125"), "yes": true, "no": false,
			"nested": map[string]any{"n": -7, "on": true, "list": []any{1, false, 2.5}},
		}
		const want = `{"big":9007199254740993,"count":42,"nested":{"list":[1,false,2.5],` +
			`"n":-7,"on":true},"no":false,"precise":12345678901234567890.125,"ratio":0.25,` +
			`"yes":true}`

		_, err := c.Check(s.ctx(t), permit.User{Key: "context-user"}, "read",
			permit.Resource{Type: "context-report"}, permit.WithDecisionContext(values))
		if err != nil {
			t.Errorf("Check with a context returned %v, want a decision", err)
		}
		checks := log.to("/allowed")
		if len(checks) != 1 || checks[0].status != http.StatusOK {
			t.Fatalf("the check sent %d requests to /allowed, want one answered with 200: %v",
				len(checks), checks)
		}
		var body struct {
			Context json.RawMessage `json:"context"`
		}
		var got bytes.Buffer
		if err := json.Unmarshal(checks[0].body, &body); err != nil ||
			json.Compact(&got, body.Context) != nil || got.String() != want {
			t.Errorf("the check sent the context %s, want %s", body.Context, want)
		}
		t.Run("abac_allow_and_deny", func(t *testing.T) {
			t.Skip("the allow and the deny of an attribute-based rule wait until condition sets " +
				"reach the PDP; tools/e2e/allowed-skips.txt names the ticket")
		})
	}
}

// twoTenantsBulk checks the decisions of six checks of one request, of a user who reads in one
// tenant and reads and writes in another, which BulkCheck returns in the order of the checks.
func twoTenantsBulk(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		const user = "bulk-user"
		s.seed(t, world{
			resources: []seed.Resource{
				{Key: "ledger", Actions: []string{"read", "write", "delete"}},
			},
			roles: []seed.Role{
				{Key: "ledger-reader", Permissions: []string{"ledger:read"}},
				{Key: "ledger-writer", Permissions: []string{"ledger:read", "ledger:write"}},
			},
			tenants: []string{"bulk-tenant-a", "bulk-tenant-b"},
			users:   []string{user},
			assignments: []seed.RoleAssignment{
				{User: user, Role: "ledger-reader", Tenant: "bulk-tenant-a"},
				{User: user, Role: "ledger-writer", Tenant: "bulk-tenant-b"},
			},
		})
		c := s.client(t, s.env.APIKey())
		check := func(action, tenant string) permit.CheckRequest {
			return permit.CheckRequest{
				User: permit.User{Key: user}, Action: action,
				Resource: permit.Resource{Type: "ledger", Tenant: tenant},
			}
		}
		checks := []permit.CheckRequest{
			check("read", "bulk-tenant-a"), check("write", "bulk-tenant-a"),
			check("read", "bulk-tenant-b"), check("write", "bulk-tenant-b"),
			check("delete", "bulk-tenant-a"), check("delete", "bulk-tenant-b"),
		}

		await(t, s, "BulkCheck of six checks in two tenants",
			[]bool{true, false, true, true, false, false},
			func(ctx context.Context) ([]bool, error) { return c.BulkCheck(ctx, checks) })
	}
}
