//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework/seed"
)

// invitesPage is the part of a page of the user invites that the teardown of an invite reads:
// the ID and the key of each invite.
type invitesPage struct {
	Data []invitesPageItem `json:"data"`
}

// invitesPageItem is an invite of an invitesPage.
type invitesPageItem struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

// inviteTarget is the seed that the invites of userInviteLifecycle name, by the keys of a
// resource, of a role of the resource and of a tenant, and the instance doc-1 of the resource in
// the tenant.
type inviteTarget struct {
	resource, role, tenant string
}

// registerUserInvite registers the teardown of the user invite whose key is key with the suite's
// ledger, before the request that creates it, as the ID of the invite is not known before then:
// a deletion of each invite of the key on the first page of 100 invites of the scratch
// environment, by its ID, and the check that the page holds none of the key.
func (s *suite) registerUserInvite(key string) {
	invites := "/v2/facts/" + url.PathEscape(s.env.ProjectID) + "/" + url.PathEscape(s.env.ID) +
		"/user_invites"
	list := framework.Request{Method: http.MethodGet, Path: invites + "?per_page=100"}
	remove := func(ctx context.Context) error {
		var page invitesPage
		if err := s.api.Call(ctx, list, &page); err != nil {
			return err
		}
		for _, invite := range page.Data {
			if invite.Key != key {
				continue
			}
			if err := s.api.Remover(invites + "/" + url.PathEscape(invite.ID))(ctx); err != nil {
				return err
			}
		}
		return nil
	}
	listed := func(body []byte) (bool, error) {
		var page invitesPage
		if err := json.Unmarshal(body, &page); err != nil {
			return false, err
		}
		return slices.ContainsFunc(page.Data, func(i invitesPageItem) bool {
			return i.Key == key
		}), nil
	}
	s.ledger.Register("user invite "+key, remove,
		s.api.Unlisted(list.Path, framework.AbsenceTimeout, listed))
}

// userInviteLifecycle checks the calls of the user invites with the environment-level API key of
// the scratch environment, and no WithScope, so that the client acts on the environment of its
// key, on two invites of its own, whose keys and email addresses start with the run's prefix and
// end with @example.com, and which it registers with the ledger before it creates them. Its seed
// is a resource, a role of the resource, a tenant and an instance of the resource in the tenant,
// whose IDs each invite names. It creates both invites, gets the first by its ID, finds it in a
// page of List of one invite, and both invites with All, one invite to a page; changes the last
// name of the first with Update; approves the first with an attribute of the user, whose user it
// registers with the ledger before, and checks the user of the reply; deletes the first invite,
// waits until a get of it matches ErrNotFound, and deletes it again, which returns nil. The
// ledger deletes the second invite and the user.
func userInviteLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		key := s.cfg.prefix.Key
		target := inviteTarget{
			resource: key("sdk-invite-doc"), role: key("sdk-invite-editor"),
			tenant: key("sdk-invite-tenant"),
		}
		s.seed(t, world{
			resources: []seed.Resource{{Key: target.resource, Actions: []string{"read"}}},
			resourceRoles: []seed.ResourceRole{
				{Resource: target.resource, Key: target.role, Permissions: []string{"read"}},
			},
			tenants: []string{target.tenant},
			instances: []seed.Instance{
				{Resource: target.resource, Key: "doc-1", Tenant: target.tenant},
			},
		})
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		fields := inviteFields(ctx, t, c, target)
		emails := []string{
			key("sdk-invite-a") + "@example.com", key("sdk-invite-b") + "@example.com",
		}
		var created []*models.ElementsUserInviteRead
		for _, email := range emails {
			s.registerUserInvite(email)
			invite := fields
			invite.Key, invite.Email = email, email
			got, err := c.UserInvites.Create(ctx, invite)
			if err != nil {
				t.Fatalf("Create of the invite of %s: %v", email, err)
			}
			if got.ID == "" || got.Key != email || got.Email != email ||
				got.Status != models.UserInviteStatusPending || got.RoleID != fields.RoleID ||
				got.EnvironmentID != s.env.ID {
				t.Fatalf("Create returned %+v, want the pending invite of %s with an ID and the "+
					"role %s", got, email, fields.RoleID)
			}
			created = append(created, got)
		}
		first := created[0]
		if got, err := c.UserInvites.Get(ctx, first.ID); err != nil || got.Key != first.Key {
			t.Errorf("Get of the invite %s returned %+v, %v; want the invite of %s", first.ID,
				got, err, first.Key)
		}
		checkListedInvites(ctx, t, c, []string{created[0].ID, created[1].ID})
		update := models.ElementsUserInviteUpdate(fields)
		update.Key, update.Email, update.LastName = first.Key, first.Email, "Hargreaves"
		if got, err := c.UserInvites.Update(ctx, first.ID, update); err != nil ||
			got.LastName != "Hargreaves" || got.ID != first.ID {
			t.Errorf("Update of the invite %s returned %+v, %v; want it with the last name "+
				"Hargreaves", first.ID, got, err)
		}
		s.registerUser(first.Key, func() string { return "" })
		user, err := c.UserInvites.Approve(ctx, first.ID, models.ElementsUserInviteApprove{
			Email: first.Email, Key: first.Key, Attributes: models.Attributes{"team": "sdk"},
		})
		if err != nil {
			t.Fatalf("Approve of the invite %s: %v", first.ID, err)
		}
		if user.Key != first.Key || user.Email != first.Email || user.Attributes["team"] != "sdk" {
			t.Errorf("Approve returned the user %+v, want %s with the attribute team sdk", user,
				first.Key)
		}
		deleteUserInviteTwice(ctx, t, s, c, first.ID)
	}
}

// inviteFields returns the fields of a pending invite of the role, the tenant and the instance
// of target, by the IDs of each that c gets, without a key and an email address.
func inviteFields(ctx context.Context, t *testing.T, c *permit.Client,
	target inviteTarget,
) models.ElementsUserInviteCreate {
	t.Helper()
	role, err := c.ResourceRoles.Get(ctx, target.resource, target.role)
	if err != nil {
		t.Fatalf("Get of the role %s of %s: %v", target.role, target.resource, err)
	}
	tenant, err := c.Tenants.Get(ctx, target.tenant)
	if err != nil {
		t.Fatalf("Get of the tenant %s: %v", target.tenant, err)
	}
	instance, err := c.ResourceInstances.Get(ctx, target.resource+":doc-1")
	if err != nil {
		t.Fatalf("Get of the instance %s:doc-1: %v", target.resource, err)
	}
	return models.ElementsUserInviteCreate{
		Status: models.UserInviteStatusPending, FirstName: "Alice", LastName: "Liddell",
		RoleID: role.ID, TenantID: tenant.ID, ResourceInstanceID: instance.ID,
	}
}

// checkListedInvites checks that the first page of List of one invite holds one of ids, and that
// All, one invite to a page, yields the invites of ids, each once, in any order, and no other, as
// the scratch environment holds no other invite.
func checkListedInvites(ctx context.Context, t *testing.T, c *permit.Client, ids []string) {
	t.Helper()
	opts := &permit.UserInvitesListOptions{ListOptions: permit.ListOptions{PerPage: 1}}
	page, err := c.UserInvites.List(ctx, opts)
	if err != nil || len(page.Data) != 1 || !slices.Contains(ids, page.Data[0].ID) ||
		page.TotalCount != nil && *page.TotalCount != len(ids) {
		t.Errorf("List of one invite returned %+v, %v; want one of %q, of %d", page, err, ids,
			len(ids))
	}
	var got []string
	for invite, err := range c.UserInvites.All(ctx, opts) {
		if err != nil {
			t.Fatalf("All of the invites: %v", err)
		}
		got = append(got, invite.ID)
	}
	if !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(ids))) {
		t.Errorf("All of the invites yielded %q, want %q", got, ids)
	}
}

// deleteUserInviteTwice deletes the invite id with c, waits until a get of it matches
// ErrNotFound, and deletes it again, which must return nil, as the invite is gone.
func deleteUserInviteTwice(ctx context.Context, t *testing.T, s *suite, c *permit.Client,
	id string,
) {
	t.Helper()
	if err := c.UserInvites.Delete(ctx, id); err != nil {
		t.Fatalf("Delete of the invite %s: %v", id, err)
	}
	gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
		func(ctx context.Context) (bool, error) {
			_, err := c.UserInvites.Get(ctx, id)
			if errors.Is(err, permit.ErrNotFound) {
				return true, nil
			}
			return false, err
		})
	if gone != nil {
		t.Fatalf("Get of the deleted invite %s: %v", id, gone)
	}
	if err := c.UserInvites.Delete(ctx, id); err != nil {
		t.Errorf("Delete of the invite %s that is gone returned %v, want nil", id, err)
	}
}
