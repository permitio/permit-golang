//go:build e2e

package e2e

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
)

// headerSecret returns the secret of a proxy config of one header, X-Api-Key, whose value is
// value.
func headerSecret(value string) models.ProxySecret {
	return models.ProxySecret{Headers: map[string]models.Secret{
		"X-Api-Key": models.NewSecret(value),
	}}
}

// headerValue returns the value of the header X-Api-Key of the secret of config, or the empty
// string when its secret is no set of headers.
func headerValue(config *models.ProxyConfigRead) string {
	return config.Secret.Headers["X-Api-Key"].Value()
}

// proxyConfigLifecycle checks the calls of the proxy configs with the environment-level API key
// of the scratch environment, whose scope the client looks up, on a proxy config of its own,
// whose key starts with the run's prefix, so that no run uses it again, and which it registers
// with the ledger before it creates it. It creates the config with a secret of headers and no
// auth mechanism, which the SDK sends as Headers, the mechanism of the secret, gets it by its key
// and by its ID with the value of its header, lists a page of one config, finds it with All,
// updates its name and its secret, which an update always sends, with another value of the
// header and again no auth mechanism, deletes it, waits until a get matches ErrNotFound, and
// deletes it again, which returns nil. The values of the header join the secrets that no Debug
// log of the run may hold.
func proxyConfigLifecycle(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		key := s.cfg.prefix.Key("sdk-proxy")
		self := "/v2/facts/" + url.PathEscape(s.env.ProjectID) + "/" + url.PathEscape(s.env.ID) +
			"/proxy_configs/" + url.PathEscape(key)
		s.ledger.Register("proxy config "+key, s.api.Remover(self),
			s.api.Absence(self, framework.AbsenceTimeout))
		first := s.cfg.prefix.Key("proxy-header-first")
		second := s.cfg.prefix.Key("proxy-header-second")
		s.secrets = append(s.secrets, first, second)
		c := s.client(t, s.env.APIKey())
		ctx := s.ctx(t)
		created, err := c.ProxyConfigs.Create(ctx, models.ProxyConfigCreate{
			Key: key, Name: key, Secret: headerSecret(first),
		})
		if err != nil {
			t.Fatalf("Create of the proxy config %s: %v", key, err)
		}
		if created.Key != key || created.ID == "" || created.EnvironmentID != s.env.ID ||
			created.AuthMechanism != models.AuthMechanismHeaders || headerValue(created) != first {
			t.Fatalf("Create returned the proxy config %q (ID %q) of the environment %q with the "+
				"auth mechanism %q and the secret %v, want %s in %s with the header that it sent",
				created.Key, created.ID, created.EnvironmentID, created.AuthMechanism,
				created.Secret, key, s.env)
		}
		for _, ref := range []string{key, created.ID} {
			got, getErr := c.ProxyConfigs.Get(ctx, ref)
			if getErr != nil {
				t.Fatalf("Get of the proxy config %s: %v", ref, getErr)
			}
			if got.ID != created.ID || got.Key != key || headerValue(got) != first {
				t.Errorf("Get of %s returned the proxy config %s (ID %s) with the secret %v, "+
					"want %s (ID %s) with the header that Create sent", ref, got.Key, got.ID,
					got.Secret, key, created.ID)
			}
		}
		page, err := c.ProxyConfigs.List(ctx, &permit.ListOptions{PerPage: 1})
		if err != nil || len(page.Data) != 1 {
			t.Errorf("List of a page of one proxy config returned %+v, %v; want one config",
				page, err)
		}
		if !listedProxyConfig(ctx, t, c, created.ID) {
			t.Errorf("All did not list the proxy config %s", created.ID)
		}
		updated, err := c.ProxyConfigs.Update(ctx, key, models.ProxyConfigUpdate{
			Name: key + "-renamed", Secret: headerSecret(second),
		})
		if err != nil {
			t.Fatalf("Update of the proxy config %s: %v", key, err)
		}
		if updated.Name != key+"-renamed" || headerValue(updated) != second ||
			updated.AuthMechanism != models.AuthMechanismHeaders {
			t.Errorf("Update returned the name %q, the auth mechanism %q and the secret %v, want "+
				"%s-renamed, Headers and the header that it sent", updated.Name,
				updated.AuthMechanism, updated.Secret, key)
		}
		if err := c.ProxyConfigs.Delete(ctx, key); err != nil {
			t.Fatalf("Delete of the proxy config %s: %v", key, err)
		}
		gone := framework.Poll(ctx, framework.AbsenceTimeout, s.cfg.syncInterval,
			func(ctx context.Context) (bool, error) {
				_, err := c.ProxyConfigs.Get(ctx, created.ID)
				if errors.Is(err, permit.ErrNotFound) {
					return true, nil
				}
				return false, err
			})
		if gone != nil {
			t.Fatalf("Get of the deleted proxy config %s: %v", created.ID, gone)
		}
		if err := c.ProxyConfigs.Delete(ctx, key); err != nil {
			t.Errorf("Delete of the proxy config %s that is gone returned %v, want nil", key, err)
		}
	}
}

// listedProxyConfig reports whether All of c lists the proxy config whose ID is id, and fails t
// when the iterator yields an error.
func listedProxyConfig(ctx context.Context, t *testing.T, c *permit.Client, id string) bool {
	t.Helper()
	var ids []string
	for config, err := range c.ProxyConfigs.All(ctx, nil) {
		if err != nil {
			t.Fatalf("All of the proxy configs: %v", err)
		}
		ids = append(ids, config.ID)
	}
	return slices.Contains(ids, id)
}
