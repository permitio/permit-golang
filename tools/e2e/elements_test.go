//go:build e2e

package e2e

import (
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework/seed"
)

// elementsLoginAs is the scenario of the login of Permit Elements. It seeds a user with a role in
// a tenant, and lists the role assignments of the user with the environment-level API key of the
// run, which must hold that role in that tenant. Its subtest token_and_redirect_url then logs in
// as that user in that tenant with the same key, whose scope the call does not look up, and
// expects a token, a bearer token and a redirect URL, without an error. Which fields the API sets
// in the reply is not confirmed yet, so until the phase api.elements of the local harness
// confirms them, the subtest skips before the login: tools/e2e/allowed-skips.txt names the
// ticket and the condition, and advisory.txt the method.
func elementsLoginAs(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		const tenant, user, role = "elements-tenant", "elements-user", "elements-member"
		s.seed(t, world{
			roles: []seed.Role{{Key: role}}, tenants: []string{tenant}, users: []string{user},
			assignments: []seed.RoleAssignment{{User: user, Role: role, Tenant: tenant}},
		})
		c := s.client(t, s.env.APIKey())
		page, err := c.RoleAssignments.List(s.ctx(t), &permit.RoleAssignmentsListOptions{
			Users: []string{user}, Tenants: []string{tenant},
		})
		if err != nil {
			t.Fatalf("List of the role assignments of the user %s: %v", user, err)
		}
		if len(page.Data) != 1 || page.Data[0].Role != role || page.Data[0].Tenant != tenant {
			t.Fatalf("the user %s has the role assignments %+v, want the role %s in the tenant "+
				"%s, which the login needs", user, page.Data, role, tenant)
		}
		t.Run("token_and_redirect_url", func(t *testing.T) {
			t.Skip("the fields of the reply to a login are not confirmed yet; " +
				"tools/e2e/allowed-skips.txt names the ticket")
			login, err := c.Elements.LoginAs(s.ctx(t), models.UserLoginRequestInput{
				UserID: user, TenantID: tenant,
			})
			if err != nil {
				t.Fatalf("LoginAs of the user %s in the tenant %s: %v", user, tenant, err)
			}
			if login.Error != "" || login.ErrorCode != 0 {
				t.Errorf("LoginAs returned the error %q of the code %d, want a login", login.Error,
					login.ErrorCode)
			}
			for name, credential := range map[string]models.Secret{
				"token": login.Token, "bearer token": login.ElementBearerToken,
				"redirect URL": login.RedirectURL,
			} {
				if credential.Value() == "" {
					t.Errorf("LoginAs returned a login without a %s", name)
				}
			}
		})
	}
}
