//go:build e2e

package e2e

import (
	"slices"
	"testing"

	"github.com/permitio/permit-golang/v2/models"
)

// getEnvironmentKey checks that a client with the project-level API key of the run, and no
// WithScope, reads the API key of the scratch environment by the IDs of its project and its own,
// and by the keys that the run names them with: each time a key of the scratch environment and
// its project, with an ID and an organization, whose secret is the API key that the run's
// clients of the environment use. The suite logs each key that the calls return at Debug, which
// prints the marker in place of its secret, and logs_hold_no_key scans the logs of the run for
// the secret. local-only.txt lists the other calls of the API keys: they need an
// organization-level API key, which the cloud lane does not have.
func getEnvironmentKey(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		c := s.client(t, s.cfg.projectKey)
		ctx := s.ctx(t)
		byID, err := c.APIKeys.GetEnvironmentKey(ctx, s.env.ProjectID, s.env.ID)
		if err != nil {
			t.Fatalf("GetEnvironmentKey of the environment %s by its ID: %v", s.env.ID, err)
		}
		named, err := c.APIKeys.GetEnvironmentKey(ctx, s.cfg.project, s.env.Key)
		if err != nil {
			t.Fatalf("GetEnvironmentKey of the environment %s of the project %s: %v", s.env.Key,
				s.cfg.project, err)
		}
		for _, key := range []*models.APIKeyRead{byID, named} {
			secret := key.Secret.Value()
			if secret != "" && !slices.Contains(s.secrets, secret) {
				s.secrets = append(s.secrets, secret)
			}
			s.logger.Debug("the API key of the scratch environment", "key", key)
			if secret != s.env.APIKey() || key.EnvironmentID != s.env.ID ||
				key.ProjectID != s.env.ProjectID || key.ID == "" || key.OrganizationID == "" {
				t.Errorf("GetEnvironmentKey returned the key %q of the environment %q in the "+
					"project %q, of the organization %q, whose secret is the scratch "+
					"environment's API key: %t; want a key of the environment %s in the project "+
					"%s, with its API key", key.ID, key.EnvironmentID, key.ProjectID,
					key.OrganizationID, secret == s.env.APIKey(), s.env.ID, s.env.ProjectID)
			}
		}
	}
}
