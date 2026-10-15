//go:build e2e

package e2e

import (
	"cmp"
	"context"
	"errors"
	"net/url"
	"slices"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
)

// environmentLifecycle checks the calls of the environments with the project-level API key of
// the run, and no WithScope, on an environment of its own in the run's project, whose key starts
// with the run's prefix, so that no run uses it again, and which it registers with the ledger
// before it creates it. It creates the environment with settings and the configuration of its
// JSON Web Key Sets, gets it by its key and by its ID, lists a page of one environment, finds it
// with All, changes its name, description and settings, deletes it, waits until a get of its ID
// matches ErrNotFound, and deletes it again, which returns nil.
//
// A get by key is not trusted to answer 404 right after a delete, as the API may find a deleted
// environment by its key for a while. So the scenario waits on a get by ID, and its teardown
// removes the environment and checks that it is gone by its ID once Create returned one, and by
// its key only when Create failed.
func environmentLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		key := s.cfg.prefix.Key("sdk-environment")
		envs := "/v2/projects/" + url.PathEscape(s.cfg.project) + "/envs/"
		var createdID string
		self := func() string { return envs + url.PathEscape(cmp.Or(createdID, key)) }
		s.ledger.Register("environment "+key,
			func(ctx context.Context) error { return s.api.Remover(self())(ctx) },
			func(ctx context.Context) error {
				return s.api.Absence(self(), framework.AbsenceTimeout)(ctx)
			})
		c := s.client(t, s.cfg.projectKey)
		ctx := s.ctx(t)
		jwks := &models.JwksConfig{
			Audience: models.StringList("gosdk-e2e", "gosdk-e2e-admin"),
			Issuer:   models.OneString("https://issuer.example.com"),
			JWKS:     &models.JwksObj{Keys: []models.Attributes{}},
		}
		created, err := c.Environments.Create(ctx, s.cfg.project, models.EnvironmentCreate{
			Key: key, Name: key, Description: "An environment of the Go SDK's tests",
			JWKS: jwks, Settings: models.Attributes{"run": key},
		})
		if err != nil {
			t.Fatalf("Create of the environment %s: %v", key, err)
		}
		createdID = created.ID
		if created.Key != key || created.ID == "" || created.ProjectID != s.env.ProjectID {
			t.Fatalf("Create returned the environment %q (ID %q) of the project %q, want %s in "+
				"%s", created.Key, created.ID, created.ProjectID, key, s.env.ProjectID)
		}
		checkEnvironment(t, "Create", created, key, jwks)
		for _, ref := range []string{key, created.ID} {
			got, getErr := c.Environments.Get(ctx, s.cfg.project, ref)
			if getErr != nil {
				t.Fatalf("Get of the environment %s: %v", ref, getErr)
			}
			if got.ID != created.ID {
				t.Errorf("Get of %s returned the environment %s, want %s", ref, got.ID, created.ID)
			}
			checkEnvironment(t, "Get", &got.EnvironmentRead, key, jwks)
		}
		page, err := c.Environments.List(ctx, s.cfg.project, &permit.ListOptions{PerPage: 1})
		if err != nil || page.Page != 1 || page.PerPage != 1 || len(page.Data) != 1 {
			t.Errorf("List of a page of one returned %+v, %v; want page 1 of one environment",
				page, err)
		}
		if !listed(ctx, t, c, s.cfg.project, created.ID) {
			t.Errorf("All did not list the environment %s", created.ID)
		}
		updated, err := c.Environments.Update(ctx, s.cfg.project, key, models.EnvironmentUpdate{
			Name: key + "-renamed", Description: models.Some(""),
			Settings: models.Some(models.Attributes{"run": key, "updated": true}),
		})
		if err != nil {
			t.Fatalf("Update of the environment %s: %v", key, err)
		}
		if updated.Name != key+"-renamed" || updated.Description != "" ||
			updated.Settings["updated"] != true {
			t.Errorf("Update returned the name %q, the description %q and the settings %v, "+
				"want %s-renamed, none and updated true", updated.Name, updated.Description,
				updated.Settings, key)
		}
		if err := c.Environments.Delete(ctx, s.cfg.project, key); err != nil {
			t.Fatalf("Delete of the environment %s: %v", key, err)
		}
		gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
			func(ctx context.Context) (bool, error) {
				_, err := c.Environments.Get(ctx, s.cfg.project, created.ID)
				if errors.Is(err, permit.ErrNotFound) {
					return true, nil
				}
				return false, err
			})
		if gone != nil {
			t.Fatalf("Get of the deleted environment %s: %v", created.ID, gone)
		}
		if err := c.Environments.Delete(ctx, s.cfg.project, created.ID); err != nil {
			t.Errorf("Delete of the environment %s that is gone returned %v, want nil",
				created.ID, err)
		}
	}
}

// checkEnvironment checks that got, the environment that call returned, has the key, the
// settings that the scenario created it with, and the audience and the issuer of jwks.
func checkEnvironment(t *testing.T, call string, got *models.EnvironmentRead, key string,
	jwks *models.JwksConfig,
) {
	t.Helper()
	if got.Key != key || got.Settings["run"] != key {
		t.Errorf("%s returned the environment %q with the settings %v, want %s with the "+
			"settings run %s", call, got.Key, got.Settings, key, key)
	}
	if got.JWKS == nil || !slices.Equal(got.JWKS.Audience.Values(), jwks.Audience.Values()) ||
		!slices.Equal(got.JWKS.Issuer.Values(), jwks.Issuer.Values()) {
		t.Errorf("%s returned the JWKS configuration %+v, want the audience %q and the issuer %q",
			call, got.JWKS, jwks.Audience.Values(), jwks.Issuer.Values())
	}
}

// listed reports whether All lists the environment whose ID is id in the project, with pages of
// the default size, 100 environments, and stops at the environment. When a loop over every page
// misses it, it reads them all once more before it reports false: other runs create and delete
// environments of the project during the loop, which moves the environments after them from one
// page to the next.
func listed(ctx context.Context, t *testing.T, c *permit.Client, project, id string) bool {
	t.Helper()
	for range 2 {
		for environment, err := range c.Environments.All(ctx, project, nil) {
			if err != nil {
				t.Errorf("All: %v", err)
				return false
			}
			if environment.ID == id {
				return true
			}
		}
	}
	return false
}
