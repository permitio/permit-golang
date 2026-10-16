//go:build e2e

package e2e

import (
	"testing"

	permit "github.com/permitio/permit-golang/v2"
)

// auditLogOfACheck is the scenario of the audit logs: a check of a user of the run, which the PDP
// answers with a decision. Its subtest in_the_audit_logs is to poll List for the audit log of
// that check for at most 120 seconds and read it with Get. A decision appears in the audit logs
// only after Permit ingests it, so until the latency of that ingest is measured, the subtest
// only skips: tools/e2e/allowed-skips.txt names the ticket and the condition, and advisory.txt
// the methods.
func auditLogOfACheck(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		c := s.client(t, s.env.APIKey())
		user := permit.User{Key: s.cfg.prefix.Key("audit-user")}
		_, err := c.Check(s.ctx(t), user, "read", permit.Resource{Type: "audit-report"})
		if err != nil {
			t.Fatalf("Check of the user %s returned %v, want a decision", user.Key, err)
		}
		t.Run("in_the_audit_logs", func(t *testing.T) {
			t.Skip("the audit log of a check waits until the latency of the ingest of " +
				"decisions is measured; tools/e2e/allowed-skips.txt names the ticket")
		})
	}
}
