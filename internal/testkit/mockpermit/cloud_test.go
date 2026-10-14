package mockpermit

import (
	"net/http"
	"slices"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/testkit/faketb"
)

func TestCloudPDPAnswers404(t *testing.T) {
	ft := faketb.New(t)
	pdp := NewPDP(ft, Cloud)
	client := newClient(t)
	requests := []call{
		{method: http.MethodPost, path: "/user-tenants", body: `{"user":{"key":"u"}}`},
		{method: http.MethodPost, path: "/allowed/all-tenants"},
		{method: http.MethodPost, path: "/allowed_url"},
		{method: http.MethodGet, path: "/local/role_assignments?user=u"},
		{method: http.MethodGet, path: "/ready"},
		{method: http.MethodDelete, path: "/callbacks/k"},
		{method: http.MethodPost, path: "/trino/batch-column-masking"},
		{method: http.MethodPost, path: "/facts/users"},
		{method: http.MethodPut, path: "/facts/users/u%2F1"},
		{method: http.MethodDelete, path: "/facts/users/u/roles"},
		{method: http.MethodPost, path: "/v2/facts/proj/env/users"},
		{method: http.MethodGet, path: "/v2/facts/proj/env/tenants/t/users"},
	}
	for _, c := range requests {
		got := c.send(t, client, pdp)
		if got.status != http.StatusNotFound || got.body != "" ||
			got.header.Get("Content-Type") != "" {
			t.Errorf("%s %s: got %d, Content-Type %q, body %q; want 404 with an empty body",
				c.method, c.path, got.status, got.header.Get("Content-Type"), got.body)
		}
	}
	if got := len(pdp.Requests()); got != len(requests) {
		t.Errorf("the cloud PDP recorded %d requests, want %d", got, len(requests))
	}
	ft.End()
	wantFailures(t, ft)
}

func TestCloudPDPFailsOtherRequests(t *testing.T) {
	ft := faketb.New(t)
	pdp := NewPDP(ft, Cloud)
	pdp.Handle("POST /allowed", Reply{Body: `{"allow":true}`})
	client := newClient(t)

	if got := (call{method: http.MethodPost, path: "/allowed"}).send(t, client, pdp); got.status !=
		http.StatusOK || got.body != `{"allow":true}` {
		t.Errorf("POST /allowed: got %d %q, want the reply", got.status, got.body)
	}
	for _, tt := range []struct {
		c      call
		status int
		want   string
	}{
		{
			c:      call{method: http.MethodPost, path: "/allowed/bulk"},
			status: http.StatusNotImplemented,
			want:   "mockpermit cloud PDP: POST /allowed/bulk: no route",
		},
		{
			c:      call{method: http.MethodPost, path: "/facts"},
			status: http.StatusNotImplemented, want: "POST /facts: no route",
		},
		{
			c:      call{method: http.MethodPost, path: "/v2/facts"},
			status: http.StatusNotImplemented, want: "POST /v2/facts: no route",
		},
		{
			c:      call{method: http.MethodGet, path: "/v2/facts/p"},
			status: http.StatusNotImplemented, want: "GET /v2/facts/p: no route",
		},
		{
			c:      call{method: http.MethodGet, path: "/v2/facts/proj/users"},
			status: http.StatusNotImplemented, want: "GET /v2/facts/proj/users: no route",
		},
		{
			c:      call{method: http.MethodGet, path: "/v2/facts/proj/env"},
			status: http.StatusNotImplemented, want: "GET /v2/facts/proj/env: no route",
		},
		{
			c:      call{method: http.MethodGet, path: "/allowed_url"},
			status: http.StatusNotImplemented, want: "GET /allowed_url: no route",
		},
		{
			c:      call{method: http.MethodPost, path: "/user-tenants", auth: []string{}},
			status: http.StatusUnauthorized, want: "POST /user-tenants: the Authorization header",
		},
		{
			c:      call{method: http.MethodPost, path: "/facts//users"},
			status: http.StatusBadRequest, want: "POST /facts//users: the path has an empty",
		},
	} {
		if got := tt.c.send(t, client, pdp); got.status != tt.status {
			t.Errorf("%s %s: status %d, want %d", tt.c.method, tt.c.path, got.status, tt.status)
		}
		wantFailures(t, ft, tt.want)
	}
}

func TestCloudPDPHandleRejects(t *testing.T) {
	ft := faketb.New(t)
	pdp := NewPDP(ft, Cloud)
	for _, route := range []string{
		"POST /user-tenants",
		"GET /callbacks/{id}",
		"POST /facts/users",
		"PATCH /facts/{collection}/{key}",
		"GET /v2/facts/{proj_id}/{env_id}/users",
		"POST /{version}/facts/users",
		"POST /{x}",
	} {
		pdp.Handle(route, Reply{})
		wantFailures(t, ft, "mockpermit cloud PDP: Handle(\""+route+"\"): only the PDP container "+
			"serves the route; the cloud PDP answers it with 404 and an empty body without a route")
	}
	for _, route := range []string{
		"POST /allowed", "GET /facts", "GET /user-tenants", "POST /v2/facts", "POST /allowed/bulk",
		"GET /v2/facts/{proj_id}", "GET /v2/facts/{proj_id}/{env_id}",
	} {
		pdp.Handle(route, Reply{})
		wantFailures(t, ft)
	}
	ft.End()
	if got := ft.Take(); len(got) != 7 {
		t.Errorf("the end of the test reported %q, want the 7 routes' unused replies", got)
	}
}

func TestContainerPDPServesAllRoutes(t *testing.T) {
	ft := faketb.New(t)
	pdp := NewPDP(ft)
	pdp.Handle("POST /user-tenants", Reply{Body: `[]`})
	pdp.Handle("POST /facts/users", Reply{Status: http.StatusCreated})
	client := newClient(t)

	tenants := call{method: http.MethodPost, path: "/user-tenants"}.send(t, client, pdp)
	users := call{method: http.MethodPost, path: "/facts/users"}.send(t, client, pdp)

	if tenants.status != http.StatusOK || tenants.body != "[]" ||
		users.status != http.StatusCreated {
		t.Errorf("the PDP container answered %d %q and %d, want its replies", tenants.status,
			tenants.body, users.status)
	}
	ft.End()
	wantFailures(t, ft)
}

func TestContainerOnlyRoutesAreRoutes(t *testing.T) {
	for _, rt := range containerOnlyRoutes {
		pattern := rt.method + " " + rt.path
		parsed, err := parseRoute(pattern)
		if err != nil {
			t.Errorf("containerOnlyRoutes has %q: %v", pattern, err)
			continue
		}
		if !containerOnly(parsed.method, parsed.segments, matches) {
			t.Errorf("containerOnly(%q) = false, want true", pattern)
		}
	}
	for _, prefix := range containerOnlyPrefixes {
		if len(prefix) == 0 || slices.Contains(prefix, "") {
			t.Errorf("containerOnlyPrefixes has %q", prefix)
		}
	}
}
