//go:build integration

package tests

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/permitio/permit-golang/internal/testcleanup"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/enforcement"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	// syncTimeout is how long a test waits for a change to reach the PDP, or to show in an API
	// list. Changes usually arrive within seconds.
	syncTimeout  = 90 * time.Second
	pollInterval = time.Second
	// deleteTimeout bounds each delete that runs when a test ends.
	deleteTimeout = 30 * time.Second
	// maxListPages stops listAll when a list never ends.
	maxListPages = 100
	perPage      = 100
)

// settings are the environment variables the tests read.
type settings struct {
	project     string
	environment string
	apiKey      string
	pdpURL      string
	apiURL      string
}

// loadSettings reads the settings, and fails the test when any of them is not set.
func loadSettings(t *testing.T) settings {
	t.Helper()
	values := map[string]string{}
	var missing []string
	for _, name := range []string{"PROJECT", "ENV", "PDP_API_KEY", "PDP_URL", "API_URL"} {
		values[name] = os.Getenv(name)
		if values[name] == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("%s not set", strings.Join(missing, ", "))
	}
	return settings{
		project:     values["PROJECT"],
		environment: values["ENV"],
		apiKey:      values["PDP_API_KEY"],
		pdpURL:      values["PDP_URL"],
		apiURL:      values["API_URL"],
	}
}

// newClient returns a client with the project and environment of s as its context, and a logger
// that writes to the test output.
func newClient(s settings) *permit.Client {
	return permit.New(config.NewConfigBuilder(s.apiKey).
		WithPdpUrl(s.pdpURL).
		WithApiUrl(s.apiURL).
		WithContext(s.permitContext()).
		WithLogger(zap.NewExample()).
		Build())
}

// permitContext returns the project and environment of s as a client context.
func (s settings) permitContext() *config.PermitContext {
	return config.NewPermitContext(config.EnvironmentAPIKeyLevel, s.project, s.environment)
}

var runPrefixOnce struct {
	sync.Once
	prefix string
	err    error
}

// runPrefix returns the prefix of every key this run creates: PERMIT_TEST_RUN_PREFIX, which CI
// sets from the workflow run, or a random gosdk-local-... prefix when it is not set. The cleanup
// command deletes the objects whose key starts with it and a hyphen.
func runPrefix(t *testing.T) string {
	t.Helper()
	runPrefixOnce.Do(func() {
		prefix := os.Getenv(testcleanup.PrefixEnvVar)
		if prefix == "" {
			random := make([]byte, 6)
			if _, err := rand.Read(random); err != nil {
				runPrefixOnce.err = err
				return
			}
			prefix = "gosdk-local-" + hex.EncodeToString(random)
		}
		runPrefixOnce.prefix = prefix
		runPrefixOnce.err = testcleanup.CheckPrefix(prefix)
	})
	require.NoError(t, runPrefixOnce.err, "%s", testcleanup.PrefixEnvVar)
	t.Logf("The keys of this run start with %s-; to delete what it left, run "+
		"%s=%s go run -tags integration ./pkg/tests/cleanup",
		runPrefixOnce.prefix, testcleanup.PrefixEnvVar, runPrefixOnce.prefix)
	return runPrefixOnce.prefix
}

// keys builds the keys of one test's objects from the run prefix and a name for the test, so
// that tests never share an object.
type keys struct {
	base string
}

func newKeys(t *testing.T, test string) keys {
	t.Helper()
	return keys{base: runPrefix(t) + "-" + test}
}

func (k keys) of(name string) string {
	return k.base + "-" + name
}

// deleteAtEnd registers del to run when the test ends. Cleanups run in the reverse order of
// registration, so registering each object right after creating it deletes every object before
// the objects it depends on. A failed delete is a test error; an object that is already gone is
// logged. Call the returned function when the test deletes the object itself.
func deleteAtEnd(
	t *testing.T, what string, del func(ctx context.Context) error,
) (deletedByTest func()) {
	t.Helper()
	deleted := false
	t.Cleanup(func() {
		if deleted {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), deleteTimeout)
		defer cancel()
		err := del(ctx)
		switch {
		case err == nil:
			t.Logf("deleted %s", what)
		case testcleanup.IsNotFound(err):
			t.Logf("%s was already deleted", what)
		default:
			t.Errorf("could not delete %s: %v", what, err)
		}
	})
	return func() { deleted = true }
}

func createUser(t *testing.T, ctx context.Context, client *permit.Client, user models.UserCreate) {
	t.Helper()
	_, err := client.Api.Users.Create(ctx, user)
	require.NoError(t, err, "creating user %s", user.Key)
	deleteAtEnd(t, "user "+user.Key, func(ctx context.Context) error {
		return client.Api.Users.Delete(ctx, user.Key)
	})
}

func createTenant(
	t *testing.T, ctx context.Context, client *permit.Client, tenant models.TenantCreate,
) {
	t.Helper()
	_, err := client.Api.Tenants.Create(ctx, tenant)
	require.NoError(t, err, "creating tenant %s", tenant.Key)
	deleteAtEnd(t, "tenant "+tenant.Key, func(ctx context.Context) error {
		return client.Api.Tenants.Delete(ctx, tenant.Key)
	})
}

func createResource(
	t *testing.T, ctx context.Context, client *permit.Client, resource models.ResourceCreate,
) *models.ResourceRead {
	t.Helper()
	created, err := client.Api.Resources.Create(ctx, resource)
	require.NoError(t, err, "creating resource %s", resource.Key)
	deleteAtEnd(t, "resource "+resource.Key, func(ctx context.Context) error {
		return client.Api.Resources.Delete(ctx, resource.Key)
	})
	return created
}

func createRole(t *testing.T, ctx context.Context, client *permit.Client, role models.RoleCreate) {
	t.Helper()
	_, err := client.Api.Roles.Create(ctx, role)
	require.NoError(t, err, "creating role %s", role.Key)
	deleteAtEnd(t, "role "+role.Key, func(ctx context.Context) error {
		return client.Api.Roles.Delete(ctx, role.Key)
	})
}

// assignRole assigns a.Role to a.User in a.Tenant with Users.AssignRole, which takes no resource
// instance, so a.ResourceInstance must be nil.
func assignRole(
	t *testing.T, ctx context.Context, client *permit.Client, a models.RoleAssignmentCreate,
) {
	t.Helper()
	require.Nil(t, a.ResourceInstance, "assignRole assigns roles in a tenant only")
	_, err := client.Api.Users.AssignRole(ctx, a.User, a.Role, a.Tenant)
	require.NoError(t, err, "assigning role %s to user %s in tenant %s", a.Role, a.User, a.Tenant)
	what := fmt.Sprintf("role %s of user %s in tenant %s", a.Role, a.User, a.Tenant)
	deleteAtEnd(t, what, func(ctx context.Context) error {
		_, err := client.Api.Users.UnassignRole(ctx, a.User, a.Role, a.Tenant)
		return err
	})
}

// waitFor calls check every pollInterval until it returns true, and fails the test when it has
// not within syncTimeout. check also returns what it saw, for the failure message.
func waitFor(t *testing.T, what string, check func() (bool, string)) {
	t.Helper()
	start := time.Now()
	for {
		ok, saw := check()
		if ok {
			t.Logf("%s after %s", what, time.Since(start).Round(100*time.Millisecond))
			return
		}
		if time.Since(start) > syncTimeout {
			t.Fatalf("%s: still not true after %s; last saw %s", what, syncTimeout, saw)
		}
		time.Sleep(pollInterval)
	}
}

// waitForAllowed waits until the PDP allows user to do action on resource.
func waitForAllowed(
	t *testing.T, client *permit.Client, user string, action string, resource enforcement.Resource,
) {
	t.Helper()
	what := fmt.Sprintf("the PDP allows %s to %s %+v", user, action, resource)
	waitFor(t, what, func() (bool, string) {
		allowed, err := client.Check(enforcement.UserBuilder(user).Build(),
			enforcement.Action(action), resource)
		return err == nil && allowed, fmt.Sprintf("allowed %v, error %v", allowed, err)
	})
}

// assertDecision checks the PDP's decision on req once, with Check, which sends no context, so
// req.Context must be nil. Call it for a deny only after waitForAllowed has seen an allow that the
// same change brought, or the deny may only mean that the change has not reached the PDP yet.
func assertDecision(
	t *testing.T, client *permit.Client, req enforcement.CheckRequest, want bool, why string,
) {
	t.Helper()
	require.Nil(t, req.Context, "Check sends no context")
	allowed, err := client.Check(req.User, req.Action, req.Resource)
	if assert.NoError(t, err, why) {
		assert.Equal(t, want, allowed, "%s: allowed for %s to %s %+v", why, req.User.Key,
			req.Action, req.Resource)
	}
}

// listAll pages through list and returns every item.
func listAll[T any](t *testing.T, what string, list func(page, perPage int) ([]T, error)) []T {
	t.Helper()
	var all []T
	for page := 1; page <= maxListPages; page++ {
		items, err := list(page, perPage)
		require.NoError(t, err, "listing %s, page %d", what, page)
		all = append(all, items...)
		if len(items) < perPage {
			return all
		}
	}
	t.Fatalf("listing %s: more than %d pages", what, maxListPages)
	return nil
}

func keysOf[T any](items []T, key func(T) string) []string {
	result := make([]string, len(items))
	for i, item := range items {
		result[i] = key(item)
	}
	return result
}
