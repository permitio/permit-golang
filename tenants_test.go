package permit

import (
	"errors"
	"math"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/models"
)

// The routes of the tenants on the API, under the scope p and e of tenantClient, and on the PDP
// container, as mockpermit.Server.Handle takes them.
const (
	listTenantsRoute     = "GET /v2/facts/{proj_id}/{env_id}/tenants"
	createTenantRoute    = "POST /v2/facts/{proj_id}/{env_id}/tenants"
	getTenantRoute       = "GET /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}"
	updateTenantRoute    = "PATCH /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}"
	deleteTenantRoute    = "DELETE /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}"
	pdpCreateTenantRoute = "POST /facts/tenants"
)

// tenantBody is the body of a reply of the API that holds the tenant tenant-a.
const tenantBody = `{"key":"tenant-a","id":"00000000-0000-4000-8000-000000000021",` +
	`"organization_id":"00000000-0000-4000-8000-000000000001",` +
	`"project_id":"00000000-0000-4000-8000-000000000002",` +
	`"environment_id":"00000000-0000-4000-8000-000000000003",` +
	`"created_at":"2026-10-14T09:30:00Z","updated_at":"2026-10-14T09:30:00Z",` +
	`"last_action_at":"2026-10-14T09:30:00Z","name":"Tenant A","attributes":{}}`

// tenantsOf returns the tenants t<from> to t<to-1>, with only a key and a name: an empty list
// when to is from.
func tenantsOf(from, to int) []models.TenantRead {
	items := make([]models.TenantRead, 0, to-from)
	for i := from; i < to; i++ {
		key := "t" + strconv.Itoa(i)
		items = append(items, models.TenantRead{Key: key, Name: key})
	}
	return items
}

// tenantClient returns a client of api with the scope p and e and opts, whose PDP is a server
// that fails the test when anything reaches it.
func tenantClient(t *testing.T, api *mockpermit.Server, opts ...Option) *Client {
	t.Helper()
	pdp := mockpermit.NewPDP(t)
	t.Cleanup(pdp.AssertUntouched)
	return decisionClient(t, api, pdp, append([]Option{WithScope("p", "e")}, opts...)...)
}

// TestTenantsListReturnsTheError checks that a reply with an error status to List, such as a 500,
// is an *Error with the status, and no page, and that All yields it once: v1 returned an empty
// list without an error for it before v1.3.0.
func TestTenantsListReturnsTheError(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusForbidden} {
		api := mockpermit.NewAPI(t)
		reply := mockpermit.Reply{Status: status, Body: `{"detail":"no"}`}
		api.Handle(listTenantsRoute, reply, reply)
		c := tenantClient(t, api, WithRetry(RetryPolicy{}))
		page, err := c.Tenants.List(t.Context(), nil)
		if e := mustError(t, err); page != nil || e.StatusCode != status || e.Op != opListTenants {
			t.Errorf("List of a reply with status %d returned %+v and %v, want no page and the "+
				"*Error of %s with the status", status, page, err, opListTenants)
		}
		items, errs := collect(t, c.Tenants.All(t.Context(), nil))
		if len(items) != 0 || len(errs) != 1 || mustError(t, errs[0]).StatusCode != status {
			t.Errorf("All of a reply with status %d yielded %d tenants and the errors %v, want "+
				"the error once", status, len(items), errs)
		}
	}
}

// TestTenantsAllReadsEveryPage checks that TenantsService.All reads the 31 tenants of a list, in
// order, asking for each page in turn: at a per_page of 30, two pages of JSON arrays and the empty
// page after them; at the default per_page of 100, one page of 31 and the empty page after it;
// and paginated objects up to the page_count that they report, without the page after it.
func TestTenantsAllReadsEveryPage(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts *TenantsListOptions
		// pages are the tenants of each reply, a JSON array, or the paginated object with
		// counts after its data when counts is not empty.
		pages   [][]models.TenantRead
		counts  string
		queries []string
	}{
		{
			name:    "per_page 30",
			opts:    &TenantsListOptions{ListOptions: ListOptions{PerPage: 30}},
			pages:   [][]models.TenantRead{tenantsOf(0, 30), tenantsOf(30, 31), {}},
			queries: []string{"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30"},
		},
		{
			name:    "the default per_page",
			pages:   [][]models.TenantRead{tenantsOf(0, 31), {}},
			queries: []string{"page=1&per_page=100", "page=2&per_page=100"},
		},
		{
			name:    "paginated objects with a page_count",
			opts:    &TenantsListOptions{ListOptions: ListOptions{PerPage: 30}},
			pages:   [][]models.TenantRead{tenantsOf(0, 30), tenantsOf(30, 31)},
			counts:  `,"total_count":31,"page_count":2`,
			queries: []string{"page=1&per_page=30", "page=2&per_page=30"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			var replies []mockpermit.Reply
			for _, page := range tc.pages {
				reply := arrayOf(t, page)
				if tc.counts != "" {
					reply = pageOf(t, page, tc.counts)
				}
				replies = append(replies, reply)
			}
			api.Handle(listTenantsRoute, replies...)
			items, errs := collect(t, tenantClient(t, api).Tenants.All(t.Context(), tc.opts))
			if diff := cmp.Diff(tenantsOf(0, 31), items); diff != "" || errs != nil {
				t.Errorf("All yielded the errors %v and tenants that differ (-want +got):\n%s",
					errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestTenantsAllReadsTheOptionsOnce checks that the iterator keeps the options of its call, the
// attribute filters too, so that a later change to them changes no request; that a loop that
// breaks asks for no other page; and that each loop reads the tenants again from the first page,
// along the same path, through the API and through the PDP container.
func TestTenantsAllReadsTheOptionsOnce(t *testing.T) {
	for _, viaPDP := range []bool{false, true} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		to, route, path := api, listTenantsRoute, "/v2/facts/p/e/tenants"
		opts := []Option{WithScope("p", "e")}
		if viaPDP {
			to, route, path = pdp, "GET /facts/tenants", "/facts/tenants"
			opts = append(opts, WithFactsViaPDP())
		}
		two := arrayOf(t, tenantsOf(0, 2))
		to.Handle(route, two, two)
		filters := []AttributeFilter{AttributeEquals("tier", "gold")}
		options := &TenantsListOptions{ListOptions: ListOptions{PerPage: 2}, Attributes: filters}
		seq := decisionClient(t, api, pdp, opts...).Tenants.All(t.Context(), options)
		options.Page, options.PerPage = 5, 50
		filters[0] = AttributeEquals("tier", "silver")
		options.Attributes = append(options.Attributes, AttributeEquals("seats", 1))
		for loop := 1; loop <= 2; loop++ {
			for tenant, err := range seq {
				if err != nil || tenant.Key != "t0" {
					t.Errorf("via the PDP %t: loop %d yielded %+v and %v first, want t0", viaPDP,
						loop, tenant, err)
				}
				break
			}
		}
		var got []string
		for _, r := range to.Requests() {
			got = append(got, r.RawPath+"?"+r.RawQuery)
		}
		first := path + "?attr_tier=gold&page=1&per_page=2"
		if diff := cmp.Diff([]string{first, first}, got); diff != "" {
			t.Errorf("via the PDP %t: the requests differ (-want +got):\n%s", viaPDP, diff)
		}
	}
}

// TestTenantAttributeFilters checks the query of the attribute filters of each type of value,
// with the names sorted, the name and the value escaped, a float64 as encoding/json writes it,
// and no attr_ parameter without a filter.
func TestTenantAttributeFilters(t *testing.T) {
	for _, tc := range []struct {
		name    string
		filters []AttributeFilter
		query   string
	}{
		{"none", nil, ""},
		{"empty", []AttributeFilter{}, ""},
		{"a string", []AttributeFilter{AttributeEquals("tier", "gold")}, "attr_tier=gold"},
		{"an empty string", []AttributeFilter{AttributeEquals("tier", "")}, "attr_tier="},
		{
			"bools",
			[]AttributeFilter{AttributeEquals("on", true), AttributeEquals("off", false)},
			"attr_off=false&attr_on=true",
		},
		{
			"integers",
			[]AttributeFilter{
				AttributeEquals("n", -3), AttributeEquals("big", fixtures.TwoPow53Plus1),
			},
			"attr_big=9007199254740993&attr_n=-3",
		},
		{
			"floats",
			[]AttributeFilter{
				AttributeEquals("half", 0.5), AttributeEquals("huge", 1e21),
				AttributeEquals("zero", fixtures.NegativeZero()), AttributeEquals("million", 1e6),
				AttributeEquals("tiny", 1e-7), AttributeEquals("small", 1e-5),
			},
			"attr_half=0.5&attr_huge=1e%2B21&attr_million=1000000&attr_small=0.00001&" +
				"attr_tiny=1e-7&attr_zero=-0",
		},
		{
			"characters that the query escapes",
			[]AttributeFilter{AttributeEquals("a b&c", "x=y&z é")},
			"attr_a+b%26c=x%3Dy%26z+%C3%A9",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listTenantsRoute, mockpermit.Reply{Body: `[]`})
			_, err := tenantClient(t, api).Tenants.List(t.Context(),
				&TenantsListOptions{Attributes: tc.filters})
			if err != nil {
				t.Fatalf("List returned %v", err)
			}
			if diff := cmp.Diff([]string{tc.query}, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestInvalidTenantListOptions checks that List and All return an error that matches
// ErrInvalidConfig, which names the rule and the place of the filter, for a Page or a PerPage less
// than zero and for an attribute filter that is not valid: the zero AttributeFilter, one without
// a name, a float64 that is NaN or an infinity, also after a valid filter, and one whose name an
// earlier filter has; and that neither sends a request. A client without WithScope sends no
// lookup of the scope for a filter either.
func TestInvalidTenantListOptions(t *testing.T) {
	valid := AttributeEquals("tier", "gold")
	for _, tc := range []struct {
		name string
		opts *TenantsListOptions
		want string
	}{
		{"a negative page", &TenantsListOptions{ListOptions: ListOptions{Page: -1}}, "Page is"},
		{
			"a negative per_page", &TenantsListOptions{ListOptions: ListOptions{PerPage: -1}},
			"PerPage is",
		},
		{
			"the zero filter", &TenantsListOptions{Attributes: []AttributeFilter{{}}},
			"attribute filter 1: " + validate.ListAttributeFilterNameEmpty.Message,
		},
		{
			"a filter without a name",
			&TenantsListOptions{Attributes: []AttributeFilter{valid, AttributeEquals("", 1)}},
			"attribute filter 2: " + validate.ListAttributeFilterNameEmpty.Message,
		},
		{
			"NaN",
			&TenantsListOptions{Attributes: []AttributeFilter{AttributeEquals("n", math.NaN())}},
			"attribute filter 1: " + validate.ListAttributeFilterNotFinite.Message,
		},
		{
			"+Inf",
			&TenantsListOptions{Attributes: []AttributeFilter{AttributeEquals("n", math.Inf(1))}},
			"attribute filter 1: " + validate.ListAttributeFilterNotFinite.Message,
		},
		{
			"-Inf",
			&TenantsListOptions{
				Attributes: []AttributeFilter{valid, AttributeEquals("n", math.Inf(-1))},
			},
			"attribute filter 2: " + validate.ListAttributeFilterNotFinite.Message,
		},
		{
			"a repeated name",
			&TenantsListOptions{
				Attributes: []AttributeFilter{valid, AttributeEquals("tier", "silver")},
			},
			"attribute filter 2: " + validate.ListAttributeFilterRepeated.Message,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			var opts []Option
			if tc.opts.Attributes == nil {
				opts = append(opts, WithScope("p", "e"))
			}
			c := decisionClient(t, api, pdp, opts...)
			_, err := c.Tenants.List(t.Context(), tc.opts)
			_, errs := collect(t, c.Tenants.All(t.Context(), tc.opts))
			for _, err := range append([]error{err}, errs...) {
				if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), tc.want) {
					t.Errorf("the call returned %v, want an error that matches ErrInvalidConfig "+
						"and holds %q", err, tc.want)
				}
			}
			if len(errs) != 1 {
				t.Errorf("All yielded the errors %v, want one", errs)
			}
			api.AssertUntouched()
			pdp.AssertUntouched()
		})
	}
}

// TestTenantsScope checks where the calls of the tenants go: without WithScope, to the project
// and the environment of the API key, which the first call looks up once, and All only when a
// loop asks for a tenant; with the scope of a project-level key, which has no environment, to no
// tenant route, with an error that matches ErrInvalidConfig; and with WithFactsViaPDP, to the PDP
// container under /facts/tenants without a lookup.
func TestTenantsScope(t *testing.T) {
	t.Run("the scope of the API key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		api.Handle(getTenantRoute, mockpermit.Reply{Body: tenantBody},
			mockpermit.Reply{Body: tenantBody})
		c := decisionClient(t, api, mockpermit.NewPDP(t))
		for range 2 {
			if _, err := c.Tenants.Get(t.Context(), "tenant-a"); err != nil {
				t.Fatal(err)
			}
		}
		want := "/v2/facts/" + scopeProjectID + "/" + scopeEnvironmentID + "/tenants/tenant-a"
		requests := api.Requests()
		if len(requests) != 3 || requests[1].RawPath != want || requests[2].RawPath != want {
			t.Errorf("the API got %d requests, want the lookup and two to %s", len(requests), want)
		}
	})
	t.Run("All looks the scope up in the loop", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		api.Handle(listTenantsRoute, mockpermit.Reply{Body: `[]`})
		seq := decisionClient(t, api, mockpermit.NewPDP(t)).Tenants.All(t.Context(), nil)
		if n := len(api.Requests()); n != 0 {
			t.Fatalf("All sent %d requests before a loop, want none", n)
		}
		if _, errs := collect(t, seq); errs != nil {
			t.Fatal(errs)
		}
		want := "/v2/facts/" + scopeProjectID + "/" + scopeEnvironmentID + "/tenants"
		if requests := api.Requests(); len(requests) != 2 || requests[1].RawPath != want {
			t.Errorf("the loop sent %d requests, want the lookup and one to %s", len(requests),
				want)
		}
	})
	t.Run("a scope without an environment", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ".project"))
		c := decisionClient(t, api, mockpermit.NewPDP(t))
		_, err := c.Tenants.Create(t.Context(), models.TenantCreate{Key: "k", Name: "n"})
		want := "permit: invalid configuration: create_tenant: " +
			validate.ScopeEnvironmentMissing.Message
		if !errors.Is(err, ErrInvalidConfig) || err.Error() != want {
			t.Errorf("Create returned %v, want %q", err, want)
		}
		if n := len(api.Requests()); n != 1 {
			t.Errorf("the API got %d requests, want the lookup alone", n)
		}
	})
	t.Run("the PDP container", func(t *testing.T) {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle("GET /facts/tenants/{tenant_id}", mockpermit.Reply{Body: tenantBody})
		c := decisionClient(t, api, pdp, WithFactsViaPDP())
		if _, err := c.Tenants.Get(t.Context(), "a/b"); err != nil {
			t.Fatal(err)
		}
		if requests := pdp.Requests(); len(requests) != 1 ||
			requests[0].RawPath != "/facts/tenants/a%2Fb" {
			t.Errorf("the PDP got %+v, want one request to /facts/tenants/a%%2Fb", requests)
		}
		api.AssertUntouched()
	})
}

// TestTenantKeysInThePath checks that each method that names a tenant sends its key as one
// segment of the path, escaped, whatever the key holds. A key that is empty, . or .. would change
// the route, so it is an error that matches ErrInvalidConfig and names the placeholder, and no
// request.
func TestTenantKeysInThePath(t *testing.T) {
	methods := []struct {
		name  string
		route string
		reply mockpermit.Reply
		call  func(c *Client, key string) error
	}{
		{
			name: "Get", route: getTenantRoute, reply: mockpermit.Reply{Body: tenantBody},
			call: func(c *Client, key string) error {
				_, err := c.Tenants.Get(t.Context(), key)
				return err
			},
		},
		{
			name: "Update", route: updateTenantRoute, reply: mockpermit.Reply{Body: tenantBody},
			call: func(c *Client, key string) error {
				_, err := c.Tenants.Update(t.Context(), key, models.TenantUpdate{Name: "n"})
				return err
			},
		},
		{
			name: "Delete", route: deleteTenantRoute,
			reply: mockpermit.Reply{Status: http.StatusNoContent},
			call:  func(c *Client, key string) error { return c.Tenants.Delete(t.Context(), key) },
		},
	}
	for key, want := range map[string]string{
		"a/b":            "/v2/facts/p/e/tenants/a%2Fb",
		"50%":            "/v2/facts/p/e/tenants/50%25",
		"a b?c#d":        "/v2/facts/p/e/tenants/a%20b%3Fc%23d",
		"é":              "/v2/facts/p/e/tenants/%C3%A9",
		fixtures.UUIDKey: "/v2/facts/p/e/tenants/" + fixtures.UUIDKey,
		".hidden":        "/v2/facts/p/e/tenants/.hidden",
	} {
		for _, m := range methods {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, m.reply)
			if err := m.call(tenantClient(t, api), key); err != nil {
				t.Errorf("%s(%q) returned %v", m.name, key, err)
			}
			if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want {
				t.Errorf("%s(%q) sent %+v, want one request to %s", m.name, key, requests, want)
			}
		}
	}
	for _, key := range []string{"", ".", ".."} {
		for _, m := range methods {
			api := mockpermit.NewAPI(t)
			err := m.call(tenantClient(t, api), key)
			if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), "{tenant_id}") {
				t.Errorf("%s(%q) returned %v, want an error that matches ErrInvalidConfig and "+
					"names {tenant_id}", m.name, key, err)
			}
			api.AssertUntouched()
		}
	}
}

// TestDeleteTenantThatIsGone checks that Delete returns nil for a 404 of the API, with or without
// a body, as the tenant that the path names is gone already, after one request; an *Error with
// the status for any other error status; and, through the PDP container, an error that matches
// ErrContainerPDPOnly for a 404 with an empty body, as the cloud PDP answers so, and nil for a 404
// of the container with a body.
func TestDeleteTenantThatIsGone(t *testing.T) {
	for _, body := range []string{`{"detail":"the tenant does not exist"}`, ""} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteTenantRoute, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		if err := tenantClient(t, api).Tenants.Delete(t.Context(), "gone"); err != nil {
			t.Errorf("Delete of a tenant that the API answers with 404 and the body %q "+
				"returned %v, want nil", body, err)
		}
		if n := len(api.Requests()); n != 1 {
			t.Errorf("Delete sent %d requests, want 1", n)
		}
	}
	for _, status := range []int{
		http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteTenantRoute, mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := tenantClient(t, api).Tenants.Delete(t.Context(), "tenant-a")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteTenant ||
			errors.Is(err, ErrNotFound) {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status", status, err, opDeleteTenant)
		}
	}
	for body, wantContainer := range map[string]bool{"": true, `{"detail":"gone"}`: false} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle("DELETE /facts/tenants/{tenant_id}",
			mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		c := decisionClient(t, api, pdp, WithFactsViaPDP())
		err := c.Tenants.Delete(t.Context(), "tenant-a")
		if got := errors.Is(err, ErrContainerPDPOnly); got != wantContainer ||
			(!wantContainer && err != nil) {
			t.Errorf("Delete through the PDP of a 404 with the body %q returned %v, want an "+
				"error that matches ErrContainerPDPOnly: %t", body, err, wantContainer)
		}
		api.AssertUntouched()
	}
}

// TestTenantBodies checks the bodies of Create and Update byte for byte against the golden files
// in testdata/golden: a tenant with the required fields only, which leaves the description and
// the attributes out; one with every field and text that an encoder may escape; one with the
// empty attributes {}; the zero update, which changes nothing; an update that sends null for the
// attributes; one that sends {}, which removes every attribute; and one that sets each field,
// the description to null.
func TestTenantBodies(t *testing.T) {
	create := func(tenant models.TenantCreate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.Tenants.Create(t.Context(), tenant)
			return err
		}
	}
	update := func(change models.TenantUpdate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.Tenants.Update(t.Context(), "tenant-a", change)
			return err
		}
	}
	for _, tc := range []struct {
		golden string
		route  string
		call   func(c *Client) error
	}{
		{
			"create_tenant.required.golden", createTenantRoute,
			create(models.TenantCreate{Key: "tenant-a", Name: "Tenant A"}),
		},
		{
			"create_tenant.every-field.golden", createTenantRoute,
			create(models.TenantCreate{
				Key: "tenant-a", Name: "<Tenant A> & B",
				Description: "a" + fixtures.LineSeparator + "b",
				Attributes: models.Attributes{
					"n": fixtures.TwoPow53Plus1, "list": []any{"a", 1}, "nested": map[string]any{},
				},
			}),
		},
		{
			"create_tenant.empty-attributes.golden", createTenantRoute,
			create(models.TenantCreate{
				Key: "tenant-a", Name: "Tenant A", Attributes: models.Attributes{},
			}),
		},
		{"update_tenant.unset.golden", updateTenantRoute, update(models.TenantUpdate{})},
		{
			"update_tenant.null.golden", updateTenantRoute,
			update(models.TenantUpdate{Attributes: models.Null[models.Attributes]()}),
		},
		{
			"update_tenant.empty.golden", updateTenantRoute,
			update(models.TenantUpdate{Attributes: models.Some(models.Attributes{})}),
		},
		{
			"update_tenant.values.golden", updateTenantRoute,
			update(models.TenantUpdate{
				Name: "Tenant A", Description: models.Null[string](),
				Attributes: models.Some(models.Attributes{"tier": "gold"}),
			}),
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: tenantBody})
			if err := tc.call(tenantClient(t, api)); err != nil {
				t.Fatalf("the call returned %v", err)
			}
			requests := api.Requests()
			if len(requests) != 1 {
				t.Fatalf("the call sent %d requests, want 1", len(requests))
			}
			requests[0].ExpectBytes(t, filepath.Join("testdata", "golden", tc.golden))
		})
	}
}

// TestTenantAttributesReadBack checks that a tenant keeps its attributes as the reply holds
// them: nil for null and for a reply without them, and an empty map that is not nil for {}.
func TestTenantAttributesReadBack(t *testing.T) {
	without := strings.Replace(tenantBody, `,"attributes":{}`, "", 1)
	for body, wantNil := range map[string]bool{
		tenantBody: false,
		strings.Replace(tenantBody, `"attributes":{}`, `"attributes":null`, 1): true,
		without: true,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(getTenantRoute, mockpermit.Reply{Body: body})
		tenant, err := tenantClient(t, api).Tenants.Get(t.Context(), "tenant-a")
		if err != nil {
			t.Fatal(err)
		}
		if (tenant.Attributes == nil) != wantNil || len(tenant.Attributes) != 0 {
			t.Errorf("Get of the reply %s returned the attributes %#v, want nil: %t", body,
				tenant.Attributes, wantNil)
		}
	}
}

// TestTenantRetries checks when a call of the tenants sends its request again: Create, which the
// API may have acted on, after a 429 only, so a 503 ends it after one request; Update, Get,
// Delete and List, which do the same when sent twice, after a 503 too.
func TestTenantRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	ok := mockpermit.Reply{Body: tenantBody}
	createTenant := func(c *Client) error {
		_, err := c.Tenants.Create(t.Context(), models.TenantCreate{Key: "k", Name: "n"})
		return err
	}
	for _, tc := range []struct {
		name    string
		route   string
		replies []mockpermit.Reply
		call    func(c *Client) error
		status  int
	}{
		{
			name: "Create after a 429", route: createTenantRoute,
			replies: []mockpermit.Reply{limited, ok}, call: createTenant,
		},
		{
			name: "Create after a 503", route: createTenantRoute,
			replies: []mockpermit.Reply{unavailable}, call: createTenant,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "Update after a 503", route: updateTenantRoute,
			replies: []mockpermit.Reply{unavailable, ok},
			call: func(c *Client) error {
				_, err := c.Tenants.Update(t.Context(), "tenant-a", models.TenantUpdate{Name: "n"})
				return err
			},
		},
		{
			name: "Get after a 503", route: getTenantRoute,
			replies: []mockpermit.Reply{unavailable, ok},
			call: func(c *Client) error {
				_, err := c.Tenants.Get(t.Context(), "tenant-a")
				return err
			},
		},
		{
			name: "Delete after a 503", route: deleteTenantRoute,
			replies: []mockpermit.Reply{unavailable, {Status: http.StatusNoContent}},
			call:    func(c *Client) error { return c.Tenants.Delete(t.Context(), "tenant-a") },
		},
		{
			name: "List after a 503", route: listTenantsRoute,
			replies: []mockpermit.Reply{unavailable, {Body: `[]`}},
			call: func(c *Client) error {
				_, err := c.Tenants.List(t.Context(), nil)
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, tc.replies...)
			err := tc.call(tenantClient(t, api, WithRetry(RetryPolicy{MaxRetries: 1})))
			if tc.status == 0 && err != nil {
				t.Errorf("the call returned %v, want the result of the second request", err)
			}
			if tc.status != 0 && mustError(t, err).StatusCode != tc.status {
				t.Errorf("the call returned %v, want the error of status %d", err, tc.status)
			}
			if n := len(api.Requests()); n != len(tc.replies) {
				t.Errorf("the call sent %d requests, want %d", n, len(tc.replies))
			}
		})
	}
}

// TestMalformedTenantReplies checks that a reply in 200-299 that is not a tenant, or for List not
// a list of tenants, a JSON array or a paginated object, is an *Error that names the model, and
// no tenant.
func TestMalformedTenantReplies(t *testing.T) {
	for _, body := range []string{``, `null`, `[]`, `"t"`, `{"key":1}`, `{} {}`} {
		api := mockpermit.NewAPI(t)
		api.Handle(getTenantRoute, mockpermit.Reply{Body: body})
		tenant, err := tenantClient(t, api).Tenants.Get(t.Context(), "tenant-a")
		want := "permit: get_tenant: GET /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}: " +
			"status 200: the reply is not a valid models.TenantRead"
		if e := mustError(t, err); tenant != nil || e.Error() != want {
			t.Errorf("Get of the reply %q returned %+v and\n%v\nwant no tenant and\n%s", body,
				tenant, err, want)
		}
	}
	for _, body := range []string{``, `null`, `"t"`, `[1]`, `[{"key":1}]`, `{"data":[1]}`} {
		api := mockpermit.NewAPI(t)
		api.Handle(listTenantsRoute, mockpermit.Reply{Body: body})
		page, err := tenantClient(t, api).Tenants.List(t.Context(), nil)
		want := "permit: list_tenants: GET /v2/facts/{proj_id}/{env_id}/tenants: status 200: " +
			"the reply is not a valid list of models.TenantRead"
		if e := mustError(t, err); page != nil || e.Error() != want {
			t.Errorf("List of the reply %q returned %+v and\n%v\nwant no page and\n%s", body, page,
				err, want)
		}
	}
}

// TestTenantsAfterClose checks that each method returns ErrClosed after Close, and sends nothing,
// not even the lookup of the scope: also List and All with options that are not valid, and each
// method of a copy of WaitForSync whose wait is not valid, which return ErrInvalidConfig before
// Close.
func TestTenantsAfterClose(t *testing.T) {
	invalid := &TenantsListOptions{
		ListOptions: ListOptions{Page: -1}, Attributes: []AttributeFilter{{}},
	}
	for _, opts := range [][]Option{nil, {WithScope("p", "e")}, {WithFactsViaPDP()}} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		c := decisionClient(t, api, pdp, opts...)
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
		ctx := t.Context()
		for _, s := range []*TenantsService{c.Tenants, c.Tenants.WaitForSync(-time.Second)} {
			var errs []error
			for _, listOpts := range []*TenantsListOptions{nil, invalid} {
				_, listErr := s.List(ctx, listOpts)
				_, allErrs := collect(t, s.All(ctx, listOpts))
				if len(allErrs) != 1 {
					t.Errorf("All after Close yielded %v, want ErrClosed once", allErrs)
				}
				errs = append(append(errs, listErr), allErrs...)
			}
			_, getErr := s.Get(ctx, "tenant-a")
			_, createErr := s.Create(ctx, models.TenantCreate{Key: "k", Name: "n"})
			_, updateErr := s.Update(ctx, "tenant-a", models.TenantUpdate{})
			deleteErr := s.Delete(ctx, "tenant-a")
			for i, err := range append(errs, getErr, createErr, updateErr, deleteErr) {
				if !errors.Is(err, ErrClosed) {
					t.Errorf("call %d after Close returned %v, want ErrClosed", i, err)
				}
			}
		}
		api.AssertUntouched()
		pdp.AssertUntouched()
	}
}

// TestTenantsWaitForSync checks the creates of a copy of WaitForSync through the PDP container:
// each carries the timeout in decimal seconds, 0 too, and the policy of the last WithSyncPolicy,
// and the calls of the service that WaitForSync was called on carry neither, before and after.
func TestTenantsWaitForSync(t *testing.T) {
	for _, tc := range []struct {
		timeout time.Duration
		opts    []SyncOption
		want    string
	}{
		{0, nil, "0 -"},
		{500 * time.Millisecond, []SyncOption{WithSyncPolicy(SyncPolicyIgnore)}, "0.5 ignore"},
		{
			1500 * time.Millisecond,
			[]SyncOption{WithSyncPolicy(SyncPolicyIgnore), WithSyncPolicy(SyncPolicyFail)},
			"1.5 fail",
		},
	} {
		t.Run(tc.want, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			reply := mockpermit.Reply{Body: tenantBody}
			pdp.Handle(pdpCreateTenantRoute, reply, reply, reply)
			plain := decisionClient(t, api, pdp, WithFactsViaPDP()).Tenants
			waiting := plain.WaitForSync(tc.timeout, tc.opts...)
			tenant := models.TenantCreate{Key: "k", Name: "n"}
			for _, s := range []*TenantsService{plain, waiting, plain} {
				if _, err := s.Create(t.Context(), tenant); err != nil {
					t.Fatal(err)
				}
			}
			var got []string
			for _, r := range pdp.Requests() {
				got = append(got, waitHeaders(r.Header))
			}
			if diff := cmp.Diff([]string{"- -", tc.want, "- -"}, got); diff != "" {
				t.Errorf("the wait headers of the three creates differ (-want +got):\n%s", diff)
			}
			api.AssertUntouched()
		})
	}
}

// TestTenantsWaitForSyncMisuse checks the calls through a copy of WaitForSync that cannot wait:
// every method but Create, which the PDP container answers without waiting; Create from a client
// that sends facts to the API; and Create with a negative timeout, with the zero SyncOption or with
// a policy that is not one, after or before a valid SyncOption. Each returns an error that matches
// ErrInvalidConfig and names the operation and the rule, and sends no request, not even the lookup
// of the scope.
func TestTenantsWaitForSyncMisuse(t *testing.T) {
	create := func(s *TenantsService) error {
		_, err := s.Create(t.Context(), models.TenantCreate{Key: "k", Name: "n"})
		return err
	}
	notWaiting := []struct {
		op   string
		call func(s *TenantsService) error
	}{
		{opListTenants, func(s *TenantsService) error {
			_, err := s.List(t.Context(), nil)
			return err
		}},
		{opListTenants, func(s *TenantsService) error {
			_, errs := collect(t, s.All(t.Context(), nil))
			return errors.Join(errs...)
		}},
		{opGetTenant, func(s *TenantsService) error {
			_, err := s.Get(t.Context(), "tenant-a")
			return err
		}},
		{opUpdateTenant, func(s *TenantsService) error {
			_, err := s.Update(t.Context(), "tenant-a", models.TenantUpdate{Name: "n"})
			return err
		}},
		{opDeleteTenant, func(s *TenantsService) error {
			return s.Delete(t.Context(), "tenant-a")
		}},
	}
	type misuse struct {
		name    string
		viaPDP  bool
		timeout time.Duration
		opts    []SyncOption
		op      string
		call    func(s *TenantsService) error
		rule    validate.Rule
	}
	var cases []misuse
	for _, m := range notWaiting {
		cases = append(cases, misuse{
			name: m.op + " does not wait", viaPDP: true, timeout: time.Second, op: m.op,
			call: m.call, rule: validate.WaitForSyncNotWaiting,
		})
	}
	cases = append(cases,
		misuse{
			name: "facts sent to the API", timeout: time.Second, op: opCreateTenant, call: create,
			rule: validate.WaitForSyncFactsViaAPI,
		},
		misuse{
			name: "a negative timeout", viaPDP: true, timeout: -time.Nanosecond,
			op: opCreateTenant, call: create, rule: validate.WaitForSyncTimeoutNegative,
		},
		misuse{
			name: "the zero SyncOption", viaPDP: true, opts: []SyncOption{{}},
			op: opCreateTenant, call: create, rule: validate.WaitForSyncPolicyUnknown,
		},
		misuse{
			name: "a policy that is not one", viaPDP: true,
			opts: []SyncOption{WithSyncPolicy(SyncPolicyFail), WithSyncPolicy("FAIL")},
			op:   opCreateTenant, call: create, rule: validate.WaitForSyncPolicyUnknown,
		},
		misuse{
			name: "the zero SyncOption before a valid one", viaPDP: true,
			opts: []SyncOption{{}, WithSyncPolicy(SyncPolicyFail)},
			op:   opCreateTenant, call: create, rule: validate.WaitForSyncPolicyUnknown,
		},
		misuse{
			name: "a policy that is not one before a valid one", viaPDP: true,
			opts: []SyncOption{WithSyncPolicy("FAIL"), WithSyncPolicy(SyncPolicyIgnore)},
			op:   opCreateTenant, call: create, rule: validate.WaitForSyncPolicyUnknown,
		},
	)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			var opts []Option
			if tc.viaPDP {
				opts = append(opts, WithFactsViaPDP())
			}
			s := decisionClient(t, api, pdp, opts...).Tenants.WaitForSync(tc.timeout, tc.opts...)
			err := tc.call(s)
			want := "permit: invalid configuration: " + tc.op + ": " + tc.rule.Message
			if err == nil || err.Error() != want || !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("the call returned %v, want %q, which matches ErrInvalidConfig", err,
					want)
			}
			api.AssertUntouched()
			pdp.AssertUntouched()
		})
	}
}
