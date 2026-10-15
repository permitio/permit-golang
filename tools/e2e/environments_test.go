//go:build e2e

package e2e

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testhook"
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

// copyTimeout bounds the wait of environmentStatsAndCopy for a copy in the background to end.
const copyTimeout = 2 * time.Minute

// copyWait is the wait that environmentStatsAndCopy asks of each read of the task of a copy in
// the background, well within the client's timeout of 30 seconds.
const copyWait = 5 * time.Second

// environmentStatsAndCopy checks the statistics and the copies of an environment with the
// project-level API key of the run, and no WithScope. It reads the statistics of the run's scratch
// environment, and adds the credentials of its PDPs to the secrets that the scan of the logs looks
// for: a credential shorter than the scan needs fails it, and so does a dry run that adds none, as
// the fake's OPAL token is in no other reply. Then it copies the scratch environment into a new
// environment of its own, with Copy and with CopyAsync, whose task it reads with CopyResult, with a
// wait, until the task ends; each new environment's key starts with the run's prefix, and the
// scenario registers it with the ledger before the copy, deletes it by its ID, and waits until a
// get of its ID matches ErrNotFound. The ledger removes and checks it by its ID once a copy
// returned one, and by its key only when the copy failed.
func environmentStatsAndCopy(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		c := s.client(t, s.cfg.projectKey)
		ctx := s.ctx(t)
		stats, err := c.Environments.Stats(ctx, s.cfg.project, s.env.ID)
		if err != nil {
			t.Fatalf("Stats of the scratch environment: %v", err)
		}
		if stats.ID != s.env.ID || stats.Key != s.env.Key {
			t.Errorf("Stats returned the environment %q (ID %q), want %s", stats.Key, stats.ID,
				s.env)
		}
		printed := fmt.Sprintf("%v %+v %#v", stats, stats, stats)
		scanned := len(s.secrets)
		for _, config := range stats.PDPConfigs {
			for _, credential := range []string{
				config.ClientSecret.Value(), config.OPALServerAccessToken.Value(),
			} {
				if credential != "" && strings.Contains(printed, credential) {
					t.Errorf("the statistics print a credential of the PDP %s", config.ID)
				}
				switch {
				case credential == "":
				case len(credential) < minScannedKey:
					t.Errorf("a credential of the PDP %s has %d characters; the scan of the "+
						"logs needs %d or more", config.ID, len(credential), minScannedKey)
				case !slices.Contains(s.secrets, credential):
					s.secrets = append(s.secrets, credential)
				}
			}
		}
		if s.cfg.mode == testhook.DryRun && len(s.secrets) == scanned {
			t.Error("the scan of the logs got no credential of a PDP from the statistics, " +
				"though the fake's OPAL token is in no other reply")
		}

		copyKey := s.cfg.prefix.Key("sdk-environment-copy")
		copied := copyInto(t, s, copyKey, func(target models.EnvironmentCopy) (string, error) {
			env, err := c.Environments.Copy(ctx, s.cfg.project, s.env.ID, target)
			if err != nil {
				return "", err
			}
			if env.Key != copyKey || env.ProjectID != s.env.ProjectID {
				t.Errorf("Copy returned the environment %q of the project %q, want %s in %s",
					env.Key, env.ProjectID, copyKey, s.env.ProjectID)
			}
			return env.ID, nil
		})
		deleteCopy(ctx, t, s, c, copied)

		asyncKey := s.cfg.prefix.Key("sdk-environment-copy-async")
		copiedAsync := copyInto(t, s, asyncKey,
			func(target models.EnvironmentCopy) (string, error) {
				task, err := c.Environments.CopyAsync(ctx, s.cfg.project, s.env.ID, target)
				if err != nil {
					return "", err
				}
				return copyResult(ctx, s, c, task, asyncKey)
			})
		deleteCopy(ctx, t, s, c, copiedAsync)
	}
}

// copyInto registers the environment key with the ledger of s, then copies the scratch
// environment into a new environment with that key with send, which returns the ID of the
// environment that the copy went into, and returns that ID. The ledger removes the environment
// and checks that it is gone by that ID, or by key when send fails, which fails t.
func copyInto(t *testing.T, s *suite, key string,
	send func(models.EnvironmentCopy) (string, error),
) string {
	t.Helper()
	envs := "/v2/projects/" + url.PathEscape(s.cfg.project) + "/envs/"
	var id string
	self := func() string { return envs + url.PathEscape(cmp.Or(id, key)) }
	s.ledger.Register("environment "+key,
		func(ctx context.Context) error { return s.api.Remover(self())(ctx) },
		func(ctx context.Context) error {
			return s.api.Absence(self(), framework.AbsenceTimeout)(ctx)
		})
	got, err := send(models.EnvironmentCopy{
		TargetEnv: models.EnvironmentCopyTarget{New: &models.EnvironmentCreate{
			Key: key, Name: key,
			Description: "A copy of a scratch environment of the Go SDK's tests",
		}},
	})
	if err != nil {
		t.Fatalf("the copy into the environment %s: %v", key, err)
	}
	if got == "" {
		t.Fatalf("the copy into the environment %s returned no ID", key)
	}
	id = got
	return id
}

// copyResult reads the task of a copy in the background with CopyResult, with a wait, until it
// is no longer processing, and returns the ID of the environment that it copied into, key; or an
// error when the task did not succeed, did not end within copyTimeout or did not copy into key.
func copyResult(ctx context.Context, s *suite, c *permit.Client,
	task *models.TaskResultEnvironmentRead, key string,
) (string, error) {
	if task.TaskID == "" {
		return "", errors.New("CopyAsync returned a task without an ID")
	}
	last := task
	err := framework.Poll(ctx, copyTimeout, s.cfg.syncInterval,
		func(ctx context.Context) (bool, error) {
			if last.Status != models.TaskStatusProcessing {
				return true, nil
			}
			read, err := c.Environments.CopyResult(ctx, s.cfg.project, s.env.ID, task.TaskID,
				&permit.CopyResultOptions{Wait: copyWait})
			if err != nil {
				return false, err
			}
			last = read
			return last.Status != models.TaskStatusProcessing, nil
		})
	switch {
	case err != nil:
		return "", fmt.Errorf("the task %s of the copy: %w", task.TaskID, err)
	case last.Status != models.TaskStatusSuccess || last.Result == nil:
		return "", fmt.Errorf("the task %s of the copy ended %s with the error %+v and the "+
			"result %+v", task.TaskID, last.Status, last.Error, last.Result)
	case last.Result.Key != key:
		return "", fmt.Errorf("the task %s copied into %q, want %s", task.TaskID,
			last.Result.Key, key)
	}
	return last.Result.ID, nil
}

// deleteCopy deletes the environment whose ID is id, which a copy created, and waits until a get
// of its ID matches ErrNotFound.
func deleteCopy(ctx context.Context, t *testing.T, s *suite, c *permit.Client, id string) {
	t.Helper()
	if err := c.Environments.Delete(ctx, s.cfg.project, id); err != nil {
		t.Fatalf("Delete of the copied environment %s: %v", id, err)
	}
	gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
		func(ctx context.Context) (bool, error) {
			_, err := c.Environments.Get(ctx, s.cfg.project, id)
			if errors.Is(err, permit.ErrNotFound) {
				return true, nil
			}
			return false, err
		})
	if gone != nil {
		t.Fatalf("Get of the deleted environment %s: %v", id, gone)
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
