//go:build e2e

package e2e

import (
	"cmp"
	"context"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/concurrent"
	"github.com/permitio/permit-golang/v2/models"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework/seed"
)

// registerUser registers the teardown of the user key with the suite's ledger, before the
// request that creates it: a deletion of the user by the ID that id returns once it holds one,
// and else by its key, and the check that the user is gone.
func (s *suite) registerUser(key string, id func() string) {
	self := func() string {
		return "/v2/facts/" + url.PathEscape(s.env.ProjectID) + "/" + url.PathEscape(s.env.ID) +
			"/users/" + url.PathEscape(cmp.Or(id(), key))
	}
	s.ledger.Register("user "+key,
		func(ctx context.Context) error { return s.api.Remover(self())(ctx) },
		func(ctx context.Context) error {
			return s.api.Absence(self(), framework.AbsenceTimeout)(ctx)
		})
}

// userLifecycle checks the calls of the users with the environment-level API key of the scratch
// environment, and no WithScope, so that the client acts on the environment of its key, on users
// of its own, whose keys start with the run's prefix and which it registers with the ledger
// before it creates them. It creates a user with a role in a tenant of its seed, gets it by its
// key and by its ID, finds it in a page of List that searches for its email and filters by its
// role, and with All, changes it with Update, which removes its first name with null, deletes
// it, waits until a get of its ID matches ErrNotFound, and deletes it again, which returns nil.
// It syncs a second user twice: the first sync creates it, and the second, which leaves its first
// name and its attributes out, replaces it, so that a get returns the user without them.
func userLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		key, synced := s.cfg.prefix.Key("sdk-user"), s.cfg.prefix.Key("sdk-synced-user")
		role, tenant := s.cfg.prefix.Key("sdk-user-role"), s.cfg.prefix.Key("sdk-user-tenant")
		s.seed(t, world{roles: []seed.Role{{Key: role}}, tenants: []string{tenant}})
		var createdID string
		s.registerUser(key, func() string { return createdID })
		s.registerUser(synced, func() string { return "" })
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		email := key + "@example.com"
		created, err := c.Users.Create(ctx, models.UserCreate{
			Key: key, Email: email, FirstName: "Ada",
			Attributes:      models.Attributes{"run": key},
			RoleAssignments: []models.UserRoleCreate{{Role: role, Tenant: tenant}},
		})
		if err != nil {
			t.Fatalf("Create of the user %s: %v", key, err)
		}
		createdID = created.ID
		if created.Key != key || created.ID == "" || created.EnvironmentID != s.env.ID ||
			created.Attributes["run"] != key {
			t.Fatalf("Create returned the user %q (ID %q) of the environment %q with the "+
				"attributes %v, want %s in %s with the attribute run", created.Key, created.ID,
				created.EnvironmentID, created.Attributes, key, s.env.ID)
		}
		for _, ref := range []string{key, created.ID} {
			got, getErr := c.Users.Get(ctx, ref)
			if getErr != nil {
				t.Fatalf("Get of the user %s: %v", ref, getErr)
			}
			if got.ID != created.ID || got.FirstName != "Ada" {
				t.Errorf("Get of %s returned the user %s named %q, want %s named Ada", ref, got.ID,
					got.FirstName, created.ID)
			}
		}
		listUser(ctx, t, c, &permit.UsersListOptions{
			ListOptions: permit.ListOptions{PerPage: 100}, Search: email, Role: role,
		}, created.ID)
		updated, err := c.Users.Update(ctx, key, models.UserUpdate{
			FirstName: models.Null[string](), LastName: models.Some("Lovelace"),
		})
		if err != nil || updated.FirstName != "" || updated.LastName != "Lovelace" ||
			updated.Email != email {
			t.Errorf("Update of the names of %s returned %+v, %v; want the first name removed, "+
				"the last name Lovelace and the email kept", key, updated, err)
		}
		syncTwice(ctx, t, c, synced)
		if err := c.Users.Delete(ctx, key); err != nil {
			t.Fatalf("Delete of the user %s: %v", key, err)
		}
		gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
			func(ctx context.Context) (bool, error) {
				_, err := c.Users.Get(ctx, created.ID)
				if errors.Is(err, permit.ErrNotFound) {
					return true, nil
				}
				return false, err
			})
		if gone != nil {
			t.Fatalf("Get of the deleted user %s: %v", created.ID, gone)
		}
		if err := c.Users.Delete(ctx, created.ID); err != nil {
			t.Errorf("Delete of the user %s that is gone returned %v, want nil", created.ID, err)
		}
	}
}

// listUser checks that the page of List with opts, filters that the user whose ID is id matches,
// holds the user, and that All yields it once.
func listUser(ctx context.Context, t *testing.T, c *permit.Client, opts *permit.UsersListOptions,
	id string,
) {
	t.Helper()
	page, err := c.Users.List(ctx, opts)
	if err != nil || !slices.ContainsFunc(page.Data, func(u models.UserRead) bool {
		return u.ID == id
	}) {
		t.Errorf("List with the search %q and the role %q returned %+v, %v; want a page with "+
			"the user %s", opts.Search, opts.Role, page, err, id)
	}
	found := 0
	for user, allErr := range c.Users.All(ctx, nil) {
		if allErr != nil {
			t.Fatalf("All of the users: %v", allErr)
		}
		if user.ID == id {
			found++
		}
	}
	if found != 1 {
		t.Errorf("All yielded the user %s %d times, want once", id, found)
	}
}

// syncTwice syncs the user key twice: the first sync creates it, with its first name and its
// attributes, and the second, without them, replaces it, so that a get returns the user without
// them.
func syncTwice(ctx context.Context, t *testing.T, c *permit.Client, key string) {
	t.Helper()
	first, err := c.Users.Sync(ctx, models.UserCreate{
		Key: key, Email: "grace@example.com", FirstName: "Grace",
		Attributes: models.Attributes{"run": key},
	})
	if err != nil || !first.Created || first.User.FirstName != "Grace" {
		t.Fatalf("the first Sync of the user %s returned %+v, %v; want it created with its "+
			"first name", key, first, err)
	}
	second, err := c.Users.Sync(ctx, models.UserCreate{Key: key, Email: "grace@example.com"})
	if err != nil || second.Created || second.User.ID != first.User.ID {
		t.Fatalf("the second Sync of the user %s returned %+v, %v; want %s replaced", key,
			second, err, first.User.ID)
	}
	got, err := c.Users.Get(ctx, key)
	if err != nil || got.FirstName != "" || len(got.Attributes) != 0 ||
		got.Email != "grace@example.com" {
		t.Errorf("Get of the user %s after the second Sync returned %+v, %v; want the first "+
			"name and the attributes that it left out cleared, and the email kept", key, got,
			err)
	}
}

// concurrentSyncs is the number of syncs of one new user that concurrentSyncOfANewUser sends at
// once.
const concurrentSyncs = 10

// concurrentSyncOfANewUser checks that concurrentSyncs syncs of one user that the environment
// does not have, sent at once from one client, all succeed, as each is one PUT that creates or
// replaces the user, and that a get then returns the user. The ledger holds the user before the
// first sync. Each sync's context carries the scenario's recorder, and is canceled when the
// context that concurrent.Run gives the call is, so that a sync still running when Run gives up
// stops.
func concurrentSyncOfANewUser(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		key := s.cfg.prefix.Key("sdk-concurrent-user")
		s.registerUser(key, func() string { return "" })
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		var created atomic.Int32
		err := concurrent.Run(t, concurrentSyncs, 2*time.Minute,
			func(runCtx context.Context, i int) error {
				callCtx, cancel := context.WithCancel(ctx)
				defer cancel()
				stop := context.AfterFunc(runCtx, cancel)
				defer stop()
				result, err := c.Users.Sync(callCtx, models.UserCreate{
					Key: key, Attributes: models.Attributes{"sync": strconv.Itoa(i)},
				})
				if err == nil && result.Created {
					created.Add(1)
				}
				return err
			})
		if err != nil {
			t.Fatalf("%d syncs of the new user %s at once: %v", concurrentSyncs, key, err)
		}
		got, err := c.Users.Get(ctx, key)
		if err != nil || got.Key != key {
			t.Fatalf("Get of the user %s after the syncs returned %+v, %v", key, got, err)
		}
		t.Logf("%d of the %d syncs created the user", created.Load(), concurrentSyncs)
	}
}
