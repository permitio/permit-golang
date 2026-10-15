//go:build e2e

package e2e

import (
	"testing"

	"github.com/permitio/permit-golang/v2/models"
)

// getProject checks that a client with the project-level API key of the run, and no WithScope,
// gets the project of the scratch environment by its ID and by the key or the ID that the run
// names it with: the same project, with the scratch environment's project ID, a key, a name and
// its organization. local-only.txt lists the other calls of the projects: List, All and Create
// need an organization-level API key, which the cloud lane does not have, and Update and Delete
// would change the project that the lane runs in.
func getProject(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		c := s.client(t, s.cfg.projectKey)
		ctx := s.ctx(t)
		byID, err := c.Projects.Get(ctx, s.env.ProjectID)
		if err != nil {
			t.Fatalf("Get of the project %s by its ID: %v", s.env.ProjectID, err)
		}
		named, err := c.Projects.Get(ctx, s.cfg.project)
		if err != nil {
			t.Fatalf("Get of the project %s: %v", s.cfg.project, err)
		}
		for _, p := range []*models.ProjectRead{byID, named} {
			if p.ID != s.env.ProjectID || p.Key == "" || p.Name == "" || p.OrganizationID == "" {
				t.Errorf("Get returned the project %q (ID %q, name %q, organization %q), want "+
					"the project %s with a key, a name and an organization", p.Key, p.ID, p.Name,
					p.OrganizationID, s.env.ProjectID)
			}
		}
		if s.cfg.project != named.Key && s.cfg.project != named.ID {
			t.Errorf("Get of the project %s returned the project %q (ID %q), whose key and ID "+
				"differ from it", s.cfg.project, named.Key, named.ID)
		}
	}
}
