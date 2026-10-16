//go:build e2e

package e2e

import (
	"testing"

	"github.com/permitio/permit-golang/v2/models"
)

// pdpDataRefresh checks the refreshes of the data of the PDPs of the scratch environment, with
// its environment-level API key, whose scope the client looks up. The refresh of every PDP gets
// 2xx, and the reply names the update and at least one PDP, as an environment has a PDP of its
// own. Then the scenario reads the ID of that PDP from the statistics of the environment, with
// the project-level API key of the run, and the refresh of that PDP by its ID gets 2xx and a
// reply that names the update and at least one PDP. A refresh asks the PDPs to read the data
// that they hold already, so it changes nothing that another scenario reads.
func pdpDataRefresh(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		refresh := models.PDPDataRefreshRequest{Reason: "an end-to-end run of the Go SDK"}
		all, err := c.PDPs.Refresh(ctx, refresh)
		if err != nil {
			t.Fatalf("Refresh of the PDPs of %s: %v", s.env, err)
		}
		if all.UpdateID == "" || len(all.PDPIDs) == 0 {
			t.Errorf("Refresh returned the update %q of the PDPs %q, want an update of at least "+
				"one PDP", all.UpdateID, all.PDPIDs)
		}

		stats, err := s.client(t, s.cfg.projectKey).Environments.Stats(ctx, s.cfg.project,
			s.env.ID)
		if err != nil {
			t.Fatalf("Stats of %s: %v", s.env, err)
		}
		if len(stats.PDPConfigs) == 0 {
			t.Fatalf("Stats of %s lists no PDP configuration, want the PDP that Refresh asked to "+
				"read its data again", s.env)
		}
		pdpID := stats.PDPConfigs[0].ID
		one, err := c.PDPs.RefreshPDP(ctx, pdpID, refresh)
		if err != nil {
			t.Fatalf("RefreshPDP of the PDP %s of %s: %v", pdpID, s.env, err)
		}
		if one.UpdateID == "" || len(one.PDPIDs) == 0 {
			t.Errorf("RefreshPDP of the PDP %s returned the update %q of the PDPs %q, want an "+
				"update of at least one PDP", pdpID, one.UpdateID, one.PDPIDs)
		}
	}
}
