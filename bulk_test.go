package permit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/models"
)

// bulkServices are the services whose bulk operations the tests call: those of a client, or the
// copies that their WaitForSync returns.
type bulkServices struct {
	users     *UsersService
	tenants   *TenantsService
	instances *ResourceInstancesService
}

// bulkServicesOf returns the services of c.
func bulkServicesOf(c *Client) bulkServices {
	return bulkServices{users: c.Users, tenants: c.Tenants, instances: c.ResourceInstances}
}

// waitingFor returns the copies of s that WaitForSync of timeout returns.
func (s bulkServices) waitingFor(timeout time.Duration) bulkServices {
	return bulkServices{
		users: s.users.WaitForSync(timeout), tenants: s.tenants.WaitForSync(timeout),
		instances: s.instances.WaitForSync(timeout),
	}
}

// bulkCall is a bulk operation as the tests call it: its method, its operation, its route on the
// API and on the PDP container, whether it is Idempotent, and how a call sends items made from
// keys, in field, the array of the body, each as item writes it.
type bulkCall struct {
	method     string
	op         string
	route      string
	pdpRoute   string
	idempotent bool
	field      string
	item       func(key string) string
	send       func(ctx context.Context, s bulkServices, keys []string) error
}

// bulkUserItem, bulkTenantItem and bulkInstanceItem return the JSON of the user, the tenant and
// the resource instance of key that the bulk calls of the tests send, and bulkIdent that of key
// in the idents of a delete.
func bulkUserItem(key string) string   { return `{"key":"` + key + `"}` }
func bulkTenantItem(key string) string { return `{"key":"` + key + `","name":"` + key + `"}` }
func bulkInstanceItem(key string) string {
	return `{"key":"` + key + `","tenant":"tenant-a","resource":"document"}`
}
func bulkIdent(key string) string { return `"` + key + `"` }

// bulkUsers, bulkTenants and bulkInstances return the users, the tenants and the resource
// instances of keys that the bulk calls of the tests send.
func bulkUsers(keys []string) []models.UserCreate {
	users := make([]models.UserCreate, len(keys))
	for i, key := range keys {
		users[i] = models.UserCreate{Key: key}
	}
	return users
}

func bulkTenants(keys []string) []models.TenantCreate {
	tenants := make([]models.TenantCreate, len(keys))
	for i, key := range keys {
		tenants[i] = models.TenantCreate{Key: key, Name: key}
	}
	return tenants
}

func bulkInstances(keys []string) []models.ResourceInstanceCreate {
	instances := make([]models.ResourceInstanceCreate, len(keys))
	for i, key := range keys {
		instances[i] = models.ResourceInstanceCreate{
			Key: key, Tenant: "tenant-a", Resource: "document",
		}
	}
	return instances
}

// bulkCalls are the seven bulk operations of the users, the tenants and the resource instances.
var bulkCalls = []bulkCall{
	{
		method: "UsersService.BulkCreate", op: opBulkCreateUsers,
		route:    "POST /v2/facts/{proj_id}/{env_id}/bulk/users",
		pdpRoute: "POST /facts/bulk/users", field: "operations", item: bulkUserItem,
		send: func(ctx context.Context, s bulkServices, keys []string) error {
			return s.users.BulkCreate(ctx, bulkUsers(keys))
		},
	},
	{
		method: "UsersService.BulkReplace", op: opBulkReplaceUsers,
		route:    "PUT /v2/facts/{proj_id}/{env_id}/bulk/users",
		pdpRoute: "PUT /facts/bulk/users", idempotent: true, field: "operations",
		item: bulkUserItem,
		send: func(ctx context.Context, s bulkServices, keys []string) error {
			return s.users.BulkReplace(ctx, bulkUsers(keys))
		},
	},
	{
		method: "UsersService.BulkDelete", op: opBulkDeleteUsers,
		route:    "DELETE /v2/facts/{proj_id}/{env_id}/bulk/users",
		pdpRoute: "DELETE /facts/bulk/users", idempotent: true, field: "idents",
		item: bulkIdent,
		send: func(ctx context.Context, s bulkServices, keys []string) error {
			return s.users.BulkDelete(ctx, keys)
		},
	},
	{
		method: "TenantsService.BulkCreate", op: opBulkCreateTenants,
		route:    "POST /v2/facts/{proj_id}/{env_id}/bulk/tenants",
		pdpRoute: "POST /facts/bulk/tenants", field: "operations", item: bulkTenantItem,
		send: func(ctx context.Context, s bulkServices, keys []string) error {
			return s.tenants.BulkCreate(ctx, bulkTenants(keys))
		},
	},
	{
		method: "TenantsService.BulkDelete", op: opBulkDeleteTenants,
		route:    "DELETE /v2/facts/{proj_id}/{env_id}/bulk/tenants",
		pdpRoute: "DELETE /facts/bulk/tenants", idempotent: true, field: "idents",
		item: bulkIdent,
		send: func(ctx context.Context, s bulkServices, keys []string) error {
			return s.tenants.BulkDelete(ctx, keys)
		},
	},
	{
		method: "ResourceInstancesService.BulkReplace", op: opBulkReplaceResourceInstances,
		route:    "PUT /v2/facts/{proj_id}/{env_id}/bulk/resource_instances",
		pdpRoute: "PUT /facts/bulk/resource_instances", idempotent: true, field: "operations",
		item: bulkInstanceItem,
		send: func(ctx context.Context, s bulkServices, keys []string) error {
			return s.instances.BulkReplace(ctx, bulkInstances(keys))
		},
	},
	{
		method: "ResourceInstancesService.BulkDelete", op: opBulkDeleteResourceInstances,
		route:    "DELETE /v2/facts/{proj_id}/{env_id}/bulk/resource_instances",
		pdpRoute: "DELETE /facts/bulk/resource_instances", idempotent: true, field: "idents",
		item: bulkIdent,
		send: func(ctx context.Context, s bulkServices, keys []string) error {
			return s.instances.BulkDelete(ctx, keys)
		},
	},
}

// bulkKeys returns n keys, k0000 to k<n-1>, each of four digits.
func bulkKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("k%04d", i)
	}
	return keys
}

// bulkBody returns the body that call sends for keys: a JSON object whose field holds the item
// of each key, in order.
func bulkBody(call bulkCall, keys []string) string {
	items := make([]string, len(keys))
	for i, key := range keys {
		items[i] = call.item(key)
	}
	return `{"` + call.field + `":[` + strings.Join(items, ",") + `]}`
}

// bulkRoute returns the server that a bulk call of the client of viaPDP goes to among api and
// pdp, and call's route there, with the options of the client: WithFactsViaPDP for the PDP
// container, and else the scope p and e.
func bulkRoute(call bulkCall, viaPDP bool, api, pdp *mockpermit.Server,
) (*mockpermit.Server, string, []Option) {
	if viaPDP {
		return pdp, call.pdpRoute, []Option{WithFactsViaPDP()}
	}
	return api, call.route, []Option{WithScope("p", "e")}
}

// TestBulkSendsEveryItemInOneRequest checks that each bulk call sends 1,000 items in one request,
// through the API and through the PDP container: the body holds every item, in order, and no
// item is left for another request.
func TestBulkSendsEveryItemInOneRequest(t *testing.T) {
	keys := bulkKeys(1000)
	for _, call := range bulkCalls {
		for _, viaPDP := range []bool{false, true} {
			t.Run(call.method+" via the PDP "+strconv.FormatBool(viaPDP), func(t *testing.T) {
				api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
				to, route, opts := bulkRoute(call, viaPDP, api, pdp)
				to.Handle(route, mockpermit.Reply{Body: `{}`})
				c := decisionClient(t, api, pdp, opts...)
				if err := call.send(t.Context(), bulkServicesOf(c), keys); err != nil {
					t.Fatalf("the call of 1000 items returned %v", err)
				}
				requests := to.Requests()
				if len(requests) != 1 {
					t.Fatalf("the call of 1000 items sent %d requests, want 1", len(requests))
				}
				if got, want := string(requests[0].Body), bulkBody(call, keys); got != want {
					t.Errorf("the call of 1000 items sent a body of %d bytes that differs from "+
						"the %d bytes of its items", len(got), len(want))
				}
			})
		}
	}
}

// TestBulkBodies checks the body of each bulk call byte for byte against the golden files in
// testdata/golden: a create or a replace sends its items in operations, each as the call of one
// item sends it, so a field that is its zero value is left out, unless the item requires it, as
// the resource instance of a tenant alone shows; a delete sends the keys in idents, as they are
// given. Nil and an empty slice each send an empty array, never null.
func TestBulkBodies(t *testing.T) {
	users := []models.UserCreate{
		{
			Key: "idp|dave@example.com", Email: "dave@example.com", FirstName: "Dave",
			LastName: "Smith", Attributes: models.Attributes{"tier": "gold"},
			RoleAssignments: []models.UserRoleCreate{
				{Role: "editor", Tenant: "tenant-a"},
				{Role: "owner", ResourceInstance: "document:doc-1"},
			},
		},
		{Key: "alice"},
	}
	tenants := []models.TenantCreate{
		{
			Key: "tenant-a", Name: "Tenant A", Description: "The first tenant",
			Attributes: models.Attributes{},
		},
		{Key: "tenant-b", Name: "Tenant B"},
	}
	instances := []models.ResourceInstanceCreate{
		{
			Key: "doc-1", Tenant: "tenant-a", Resource: "document",
			Attributes: models.Attributes{"owner": "alice"},
		},
		{Tenant: "tenant-b"},
	}
	for _, tc := range []struct {
		golden string
		route  string
		call   func(s bulkServices) error
	}{
		{
			"bulk_create_users.items.golden", bulkCalls[0].route,
			func(s bulkServices) error { return s.users.BulkCreate(t.Context(), users) },
		},
		{
			"bulk_create_users.empty.golden", bulkCalls[0].route,
			func(s bulkServices) error { return s.users.BulkCreate(t.Context(), nil) },
		},
		{
			"bulk_create_users.empty.golden", bulkCalls[0].route,
			func(s bulkServices) error {
				return s.users.BulkCreate(t.Context(), []models.UserCreate{})
			},
		},
		{
			"bulk_replace_users.items.golden", bulkCalls[1].route,
			func(s bulkServices) error { return s.users.BulkReplace(t.Context(), users) },
		},
		{
			"bulk_replace_users.empty.golden", bulkCalls[1].route,
			func(s bulkServices) error { return s.users.BulkReplace(t.Context(), nil) },
		},
		{
			"bulk_delete_users.items.golden", bulkCalls[2].route,
			func(s bulkServices) error {
				return s.users.BulkDelete(t.Context(), []string{"idp|dave@example.com", "alice"})
			},
		},
		{
			"bulk_delete_users.empty.golden", bulkCalls[2].route,
			func(s bulkServices) error { return s.users.BulkDelete(t.Context(), nil) },
		},
		{
			"bulk_delete_users.empty.golden", bulkCalls[2].route,
			func(s bulkServices) error { return s.users.BulkDelete(t.Context(), []string{}) },
		},
		{
			"bulk_create_tenants.items.golden", bulkCalls[3].route,
			func(s bulkServices) error { return s.tenants.BulkCreate(t.Context(), tenants) },
		},
		{
			"bulk_create_tenants.empty.golden", bulkCalls[3].route,
			func(s bulkServices) error { return s.tenants.BulkCreate(t.Context(), nil) },
		},
		{
			"bulk_delete_tenants.items.golden", bulkCalls[4].route,
			func(s bulkServices) error {
				return s.tenants.BulkDelete(t.Context(), []string{"tenant-a", "tenant-b"})
			},
		},
		{
			"bulk_delete_tenants.empty.golden", bulkCalls[4].route,
			func(s bulkServices) error { return s.tenants.BulkDelete(t.Context(), nil) },
		},
		{
			"bulk_replace_resource_instances.items.golden", bulkCalls[5].route,
			func(s bulkServices) error { return s.instances.BulkReplace(t.Context(), instances) },
		},
		{
			"bulk_replace_resource_instances.empty.golden", bulkCalls[5].route,
			func(s bulkServices) error { return s.instances.BulkReplace(t.Context(), nil) },
		},
		{
			"bulk_delete_resource_instances.items.golden", bulkCalls[6].route,
			func(s bulkServices) error {
				return s.instances.BulkDelete(t.Context(),
					[]string{"document:doc-1", "00000000-0000-4000-8000-000000000041"})
			},
		},
		{
			"bulk_delete_resource_instances.empty.golden", bulkCalls[6].route,
			func(s bulkServices) error { return s.instances.BulkDelete(t.Context(), nil) },
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: `{}`})
			if err := tc.call(bulkServicesOf(userClient(t, api))); err != nil {
				t.Fatalf("the call returned %v", err)
			}
			requests := api.Requests()
			if len(requests) != 1 || requests[0].RawQuery != "" {
				t.Fatalf("the call sent %+v, want one request without a query", requests)
			}
			requests[0].ExpectBytes(t, filepath.Join("testdata", "golden", tc.golden))
		})
	}
}

// TestBulkReplies checks that each bulk call returns nil for any reply in 200-299, after one
// request, through the API and through the PDP container: it reads no body, so a body that is
// empty, that of a 204, or that is not the JSON object without fields of the spec, such as an
// array or text that is not JSON, is no error.
func TestBulkReplies(t *testing.T) {
	replies := []mockpermit.Reply{
		{Body: `{}`},
		{Body: ``},
		{Status: http.StatusNoContent},
		{Status: http.StatusCreated, Body: `[]`},
		{Body: `not JSON`},
		{Status: http.StatusAccepted, Body: `{"created":["a"]}`},
	}
	for _, call := range bulkCalls {
		for _, viaPDP := range []bool{false, true} {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			to, route, opts := bulkRoute(call, viaPDP, api, pdp)
			to.Handle(route, replies...)
			s := bulkServicesOf(decisionClient(t, api, pdp, opts...))
			for _, reply := range replies {
				if err := call.send(t.Context(), s, []string{"alice"}); err != nil {
					t.Errorf("%s via the PDP %t of the reply %d %q returned %v, want nil",
						call.method, viaPDP, reply.Status, reply.Body, err)
				}
			}
			if n := len(to.Requests()); n != len(replies) {
				t.Errorf("%s via the PDP %t sent %d requests for %d calls, want one each",
					call.method, viaPDP, n, len(replies))
			}
		}
	}
}

// TestBulkRetries checks when a bulk call sends its request again: each after a 429, which the
// API did not act on; and a create, which the API may have acted on, only then, so a 503 ends it
// after one request, while a replace and a delete, which do the same when sent twice, send it
// again after a 503 too.
func TestBulkRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	for _, call := range bulkCalls {
		for _, first := range []mockpermit.Reply{limited, unavailable} {
			name := call.method + " after a " + strconv.Itoa(first.Status)
			t.Run(name, func(t *testing.T) {
				api := mockpermit.NewAPI(t)
				retried := call.idempotent || first.Status == http.StatusTooManyRequests
				if retried {
					api.Handle(call.route, first, mockpermit.Reply{Body: `{}`})
				} else {
					api.Handle(call.route, first)
				}
				c := userClient(t, api, WithRetry(RetryPolicy{MaxRetries: 1}))
				err := call.send(t.Context(), bulkServicesOf(c), []string{"alice"})
				want := 1
				if retried {
					want = 2
				}
				if n := len(api.Requests()); n != want {
					t.Errorf("the call sent %d requests, want %d", n, want)
				}
				switch {
				case retried && err != nil:
					t.Errorf("the call returned %v, want nil after the second request", err)
				case !retried && mustError(t, err).StatusCode != first.Status:
					t.Errorf("the call returned %v, want the error of the %d", err, first.Status)
				}
			})
		}
	}
}

// TestBulkNotFound checks the 404 of each bulk call, through the API and through the PDP
// container: no bulk operation names its objects in its path, so a 404 with a body is an *Error
// that matches ErrNotFound, never nil, after one request, also with a retry policy that allows
// one more; and, through the container, a 404 with an empty body, as the cloud PDP answers, is
// an error that matches ErrContainerPDPOnly, not ErrNotFound.
func TestBulkNotFound(t *testing.T) {
	for _, call := range bulkCalls {
		for _, tc := range []struct {
			viaPDP        bool
			body          string
			containerOnly bool
		}{
			{viaPDP: false, body: `{"error_code":"NOT_FOUND"}`},
			{viaPDP: true, body: `{"error_code":"NOT_FOUND"}`},
			{viaPDP: true, body: "", containerOnly: true},
		} {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			to, route, opts := bulkRoute(call, tc.viaPDP, api, pdp)
			to.Handle(route, mockpermit.Reply{Status: http.StatusNotFound, Body: tc.body})
			opts = append(opts, WithRetry(RetryPolicy{MaxRetries: 1}))
			s := bulkServicesOf(decisionClient(t, api, pdp, opts...))
			err := call.send(t.Context(), s, []string{"alice"})
			e := mustError(t, err)
			if e.StatusCode != http.StatusNotFound || e.Op != call.op ||
				errors.Is(err, ErrNotFound) == tc.containerOnly ||
				errors.Is(err, ErrContainerPDPOnly) != tc.containerOnly {
				t.Errorf("%s via the PDP %t of a 404 with the body %q returned %v; want an "+
					"*Error that matches ErrContainerPDPOnly: %t, and else ErrNotFound",
					call.method, tc.viaPDP, tc.body, err, tc.containerOnly)
			}
			if n := len(to.Requests()); n != 1 {
				t.Errorf("%s via the PDP %t of a 404 sent %d requests, want 1", call.method,
					tc.viaPDP, n)
			}
		}
	}
}

// TestBulkWaitForSyncMisuse checks each bulk call through a copy of WaitForSync: the PDP
// container answers the bulk routes without waiting, so the call returns an error that matches
// ErrInvalidConfig, names the operation and the rule wait-for-sync.not-waiting, and sends no
// request; from a client that sends facts to the API, the rule is wait-for-sync.facts-via-api.
// Neither sends a request, not even the lookup of the scope.
func TestBulkWaitForSyncMisuse(t *testing.T) {
	for _, call := range bulkCalls {
		for _, viaPDP := range []bool{true, false} {
			t.Run(call.method+" via the PDP "+strconv.FormatBool(viaPDP), func(t *testing.T) {
				rule := validate.WaitForSyncNotWaiting
				var opts []Option
				if viaPDP {
					opts = append(opts, WithFactsViaPDP())
				} else {
					rule = validate.WaitForSyncFactsViaAPI
				}
				api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
				s := bulkServicesOf(decisionClient(t, api, pdp, opts...)).waitingFor(time.Second)
				err := call.send(t.Context(), s, []string{"alice"})
				want := "permit: invalid configuration: " + call.op + ": " + rule.Message
				if err == nil || err.Error() != want || !errors.Is(err, ErrInvalidConfig) {
					t.Errorf("the call returned %v, want %q, which matches ErrInvalidConfig",
						err, want)
				}
				api.AssertUntouched()
				pdp.AssertUntouched()
			})
		}
	}
}

// TestBulkReplaceInstanceWithoutTenant checks that BulkReplace of resource instances, one of
// which has no tenant, is an error that matches ErrInvalidConfig with the rule
// resource-instance.tenant.empty and the index of the first such instance, as Create refuses one,
// and sends no request, not even the lookup of the scope of a client without WithScope, also
// through the PDP container and through a copy of WaitForSync.
func TestBulkReplaceInstanceWithoutTenant(t *testing.T) {
	instances := []models.ResourceInstanceCreate{
		{Key: "doc-1", Tenant: "tenant-a", Resource: "document"},
		{Key: "doc-2", Tenant: "tenant-a", Resource: "document"},
		{Key: "doc-3", Resource: "document", Attributes: models.Attributes{"a": 1}},
		{Key: "doc-4", Resource: "document"},
	}
	want := "permit: invalid configuration: bulk_replace_resource_instances: instances[2]: " +
		validate.ResourceInstanceTenantEmpty.Message
	for _, opts := range [][]Option{nil, {WithScope("p", "e")}, {WithFactsViaPDP()}} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		s := decisionClient(t, api, pdp, opts...).ResourceInstances
		for _, s := range []*ResourceInstancesService{s, s.WaitForSync(time.Second)} {
			err := s.BulkReplace(t.Context(), instances)
			if !errors.Is(err, ErrInvalidConfig) || err.Error() != want {
				t.Errorf("BulkReplace of an instance without a tenant with %d options returned "+
					"%v; want %q", len(opts), err, want)
			}
		}
		api.AssertUntouched()
		pdp.AssertUntouched()
	}
}

// TestBulkAfterClose checks that each bulk call returns ErrClosed after Close, and sends
// nothing, not even the lookup of the scope, with and without a scope and WithFactsViaPDP: also
// through a copy of WaitForSync, whose call returns ErrInvalidConfig before Close.
func TestBulkAfterClose(t *testing.T) {
	for _, opts := range [][]Option{nil, {WithScope("p", "e")}, {WithFactsViaPDP()}} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		c := decisionClient(t, api, pdp, opts...)
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		services := bulkServicesOf(c)
		for _, s := range []bulkServices{services, services.waitingFor(time.Second)} {
			for _, call := range bulkCalls {
				if err := call.send(t.Context(), s, []string{"alice"}); !errors.Is(err,
					ErrClosed) {
					t.Errorf("%s after Close returned %v, want ErrClosed", call.method, err)
				}
			}
		}
		api.AssertUntouched()
		pdp.AssertUntouched()
	}
}
