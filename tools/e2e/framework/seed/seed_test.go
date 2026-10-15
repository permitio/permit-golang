package seed

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/tools/e2e/fake"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
)

// projectKey is the project's API key of the fakes of these tests.
const projectKey = "permit_key_FAKEseed"

// startFake starts a fake of the routes of the control plane's spec that the builder and these
// tests send, whose check accepts every body. These tests do not import tools/drift, which
// builds against the SDK, so that the mutation tests can run them in a copy of the tools module;
// TestSeedBuilder in tools/e2e runs the builder against the routes and the structural walker of
// the spec.
func startFake(t *testing.T) *fake.Fake {
	t.Helper()
	routes := []fake.Route{
		{Method: "POST", Template: "/v2/projects/{proj_id}/envs", Op: "create_environment"},
		{Method: "GET", Template: "/v2/api-key/{proj_id}/{env_id}", Op: "get_environment_api_key"},
	}
	for _, c := range []struct{ collection, item string }{
		{"/v2/schema/{proj_id}/{env_id}/resources", "{resource_id}"},
		{"/v2/schema/{proj_id}/{env_id}/roles", "{role_id}"},
		{"/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles", "{role_id}"},
		{"/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/relations", "{relation_id}"},
		{"/v2/facts/{proj_id}/{env_id}/tenants", "{tenant_id}"},
		{"/v2/facts/{proj_id}/{env_id}/users", "{user_id}"},
		{"/v2/facts/{proj_id}/{env_id}/resource_instances", "{instance_id}"},
		{"/v2/facts/{proj_id}/{env_id}/role_assignments", ""},
		{"/v2/facts/{proj_id}/{env_id}/relationship_tuples", ""},
	} {
		routes = append(routes, fake.Route{Method: "POST", Template: c.collection, Op: "create"},
			fake.Route{Method: "GET", Template: c.collection, Op: "list"})
		if c.item == "" {
			remove := fake.Route{Method: "DELETE", Template: c.collection, Op: "remove"}
			routes = append(routes, remove)
			continue
		}
		item := c.collection + "/" + c.item
		routes = append(routes, fake.Route{Method: "GET", Template: item, Op: "get"},
			fake.Route{Method: "DELETE", Template: item, Op: "delete"})
	}
	return fake.New(t, fake.Config{
		Routes: routes, Check: func(string, []byte) ([]string, error) { return nil, nil },
		Project: "project", ProjectAPIKey: projectKey,
	})
}

// newAPI returns an API of f with the project's key that sends its requests with client.
func newAPI(t *testing.T, f *fake.Fake, client *http.Client) *framework.API {
	t.Helper()
	api, err := framework.NewAPI(f.URL(), projectKey, client)
	if err != nil {
		t.Fatal(err)
	}
	return api
}

// sent is a request that a harness's builder sent, with the number of objects that the ledger
// held when it was sent.
type sent struct {
	method, uri, body string
	registered        int
}

// harness is a builder of an environment of a fake, whose ledger does not hold the environment,
// with the requests that the builder sent.
type harness struct {
	t       *testing.T
	ledger  *framework.Ledger
	builder *Builder

	// plain is an API of the fake that the harness does not record.
	plain *framework.API

	// schema and facts are the paths of the environment's schema and facts.
	schema, facts string

	mu   sync.Mutex
	sent []sent

	// unapplied reports whether to answer a request with 204 and not send it, as an API that
	// acknowledges a removal and does not apply it; nil sends every request.
	unapplied func(*http.Request) bool
}

// newHarness starts a fake and creates an environment there apart from the harness's ledger, so
// that the environment stays after the ledger's teardown.
func newHarness(t *testing.T) *harness {
	t.Helper()
	f := startFake(t)
	h := &harness{t: t, ledger: &framework.Ledger{}, plain: newAPI(t, f, &http.Client{})}
	env, err := framework.CreateEnvironment(t.Context(), h.plain, &framework.Ledger{}, "project",
		"gosdk-ci-0-1-seed")
	if err != nil {
		t.Fatal(err)
	}
	h.builder = New(newAPI(t, f, &http.Client{Transport: h}), h.ledger, env)
	h.schema = "/v2/schema/" + env.ProjectID + "/" + env.ID
	h.facts = "/v2/facts/" + env.ProjectID + "/" + env.ID
	return h
}

// RoundTrip records req, with the number of objects that the ledger holds, and sends it, or
// answers it with 204 when unapplied reports so.
func (h *harness) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.GetBody != nil {
		reader, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		if body, err = io.ReadAll(reader); err != nil {
			return nil, err
		}
	}
	h.mu.Lock()
	h.sent = append(h.sent, sent{
		req.Method, req.URL.RequestURI(), string(body), h.ledger.Counts().Registered,
	})
	unapplied := h.unapplied
	h.mu.Unlock()
	if unapplied != nil && unapplied(req) {
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Header:     http.Header{},
			Body:       http.NoBody,
			Request:    req,
		}, nil
	}
	return http.DefaultTransport.RoundTrip(req)
}

// take returns the requests that the builder sent, and forgets them.
func (h *harness) take() []sent {
	h.mu.Lock()
	defer h.mu.Unlock()
	taken := h.sent
	h.sent = nil
	return taken
}

// count returns the number of objects of the list at path, which it reads apart from the
// builder.
func (h *harness) count(path string) int {
	h.t.Helper()
	var list []json.RawMessage
	err := h.plain.Call(h.t.Context(), framework.Request{Method: "GET", Path: path}, &list)
	if err != nil {
		h.t.Fatal(err)
	}
	return len(list)
}

// counts returns the number of objects of each list at paths.
func (h *harness) counts(paths []string) []int {
	h.t.Helper()
	var n []int
	for _, path := range paths {
		n = append(n, h.count(path))
	}
	return n
}

// step is a call of a builder, with the object that it registers and the requests that create,
// remove and check that the object is gone.
type step struct {
	// name is the object's name in the ledger.
	name string

	call func(context.Context, *Builder) error

	// create, remove and check are the requests, with registered left 0.
	create, remove, check sent
}

func TestBuilder(t *testing.T) {
	h := newHarness(t)
	schema, facts := h.schema, h.facts
	instances, assignments := facts+"/resource_instances", facts+"/role_assignments"
	tuples := facts + "/relationship_tuples"
	steps := []step{
		{
			"resource document", func(ctx context.Context, b *Builder) error {
				actions := []string{"write", "read"}
				return b.Resource(ctx, Resource{Key: "document", Actions: actions})
			},
			sent{method: "POST", uri: schema + "/resources", body: `{"key":"document",` +
				`"name":"document","actions":{"read":{},"write":{}}}`},
			sent{method: "DELETE", uri: schema + "/resources/document"},
			sent{method: "GET", uri: schema + "/resources/document"},
		},
		{
			"resource folder", func(ctx context.Context, b *Builder) error {
				return b.Resource(ctx, Resource{Key: "folder"})
			},
			sent{method: "POST", uri: schema + "/resources", body: `{"key":"folder",` +
				`"name":"folder","actions":{}}`},
			sent{method: "DELETE", uri: schema + "/resources/folder"},
			sent{method: "GET", uri: schema + "/resources/folder"},
		},
		{
			"role admin", func(ctx context.Context, b *Builder) error {
				return b.Role(ctx, Role{Key: "admin", Permissions: []string{"document:write"}})
			},
			sent{method: "POST", uri: schema + "/roles", body: `{"key":"admin","name":"admin",` +
				`"permissions":["document:write"]}`},
			sent{method: "DELETE", uri: schema + "/roles/admin"},
			sent{method: "GET", uri: schema + "/roles/admin"},
		},
		{
			"role member", func(ctx context.Context, b *Builder) error {
				return b.Role(ctx, Role{Key: "member"})
			},
			sent{method: "POST", uri: schema + "/roles", body: `{"key":"member","name":"member"}`},
			sent{method: "DELETE", uri: schema + "/roles/member"},
			sent{method: "GET", uri: schema + "/roles/member"},
		},
		{
			"resource role viewer of document", func(ctx context.Context, b *Builder) error {
				return b.ResourceRole(ctx, ResourceRole{
					Resource: "document", Key: "viewer", Permissions: []string{"read"},
				})
			},
			sent{method: "POST", uri: schema + "/resources/document/roles", body: `{"key":` +
				`"viewer","name":"viewer","permissions":["read"]}`},
			sent{method: "DELETE", uri: schema + "/resources/document/roles/viewer"},
			sent{method: "GET", uri: schema + "/resources/document/roles/viewer"},
		},
		{
			"relation parent of document", func(ctx context.Context, b *Builder) error {
				return b.Relation(ctx, Relation{
					Resource: "document", Key: "parent", SubjectResource: "folder",
				})
			},
			sent{method: "POST", uri: schema + "/resources/document/relations", body: `{"key":` +
				`"parent","name":"parent","subject_resource":"folder"}`},
			sent{method: "DELETE", uri: schema + "/resources/document/relations/parent"},
			sent{method: "GET", uri: schema + "/resources/document/relations/parent"},
		},
		{
			"tenant t1", func(ctx context.Context, b *Builder) error {
				return b.Tenant(ctx, "t1")
			},
			sent{method: "POST", uri: facts + "/tenants", body: `{"key":"t1","name":"t1"}`},
			sent{method: "DELETE", uri: facts + "/tenants/t1"},
			sent{method: "GET", uri: facts + "/tenants/t1"},
		},
		{
			"user u1@example.com", func(ctx context.Context, b *Builder) error {
				return b.User(ctx, "u1@example.com")
			},
			sent{method: "POST", uri: facts + "/users", body: `{"key":"u1@example.com"}`},
			sent{method: "DELETE", uri: facts + "/users/u1@example.com"},
			sent{method: "GET", uri: facts + "/users/u1@example.com"},
		},
		{
			"resource instance document:doc-1", func(ctx context.Context, b *Builder) error {
				return b.Instance(ctx, Instance{Resource: "document", Key: "doc-1", Tenant: "t1"})
			},
			sent{method: "POST", uri: instances, body: `{"resource":"document","key":"doc-1",` +
				`"tenant":"t1"}`},
			sent{method: "DELETE", uri: instances + "/document:doc-1"},
			sent{method: "GET", uri: instances + "/document:doc-1"},
		},
		{
			"resource instance folder:doc-1", func(ctx context.Context, b *Builder) error {
				return b.Instance(ctx, Instance{Resource: "folder", Key: "doc-1", Tenant: "t1"})
			},
			sent{method: "POST", uri: instances, body: `{"resource":"folder","key":"doc-1",` +
				`"tenant":"t1"}`},
			sent{method: "DELETE", uri: instances + "/folder:doc-1"},
			sent{method: "GET", uri: instances + "/folder:doc-1"},
		},
		{
			"resource instance document:doc-2", func(ctx context.Context, b *Builder) error {
				return b.Instance(ctx, Instance{Resource: "document", Key: "doc-2", Tenant: "t1"})
			},
			sent{method: "POST", uri: instances, body: `{"resource":"document","key":"doc-2",` +
				`"tenant":"t1"}`},
			sent{method: "DELETE", uri: instances + "/document:doc-2"},
			sent{method: "GET", uri: instances + "/document:doc-2"},
		},
		{
			"role assignment of member to u1@example.com in t1",
			func(ctx context.Context, b *Builder) error {
				return b.RoleAssignment(ctx, RoleAssignment{
					User: "u1@example.com", Role: "member", Tenant: "t1",
				})
			},
			sent{method: "POST", uri: assignments, body: `{"user":"u1@example.com",` +
				`"role":"member","tenant":"t1"}`},
			sent{method: "DELETE", uri: assignments, body: `{"user":"u1@example.com",` +
				`"role":"member","tenant":"t1"}`},
			sent{method: "GET", uri: assignments + "?role=member&tenant=t1&user=u1%40example.com"},
		},
		{
			"role assignment of viewer to u1@example.com in t1 on document:doc-1",
			func(ctx context.Context, b *Builder) error {
				return b.RoleAssignment(ctx, RoleAssignment{
					User: "u1@example.com", Role: "viewer", Tenant: "t1",
					ResourceInstance: "document:doc-1",
				})
			},
			sent{method: "POST", uri: assignments, body: `{"user":"u1@example.com",` +
				`"role":"viewer","tenant":"t1","resource_instance":"document:doc-1"}`},
			sent{method: "DELETE", uri: assignments, body: `{"user":"u1@example.com",` +
				`"role":"viewer","tenant":"t1","resource_instance":"document:doc-1"}`},
			sent{method: "GET", uri: assignments + "?resource_instance=document%3Adoc-1&" +
				"role=viewer&tenant=t1&user=u1%40example.com"},
		},
		{
			"relationship tuple folder:doc-1 parent document:doc-1",
			func(ctx context.Context, b *Builder) error {
				return b.Tuple(ctx, Tuple{
					Subject: "folder:doc-1", Relation: "parent", Object: "document:doc-1",
					Tenant: "t1",
				})
			},
			sent{method: "POST", uri: tuples, body: `{"subject":"folder:doc-1",` +
				`"relation":"parent","object":"document:doc-1","tenant":"t1"}`},
			sent{method: "DELETE", uri: tuples, body: `{"subject":"folder:doc-1",` +
				`"relation":"parent","object":"document:doc-1"}`},
			sent{method: "GET", uri: tuples + "?object=document%3Adoc-1&relation=parent&" +
				"subject=folder%3Adoc-1"},
		},
		{
			"relationship tuple folder:doc-1 parent document:doc-2",
			func(ctx context.Context, b *Builder) error {
				return b.Tuple(ctx, Tuple{
					Subject: "folder:doc-1", Relation: "parent", Object: "document:doc-2",
				})
			},
			sent{method: "POST", uri: tuples, body: `{"subject":"folder:doc-1",` +
				`"relation":"parent","object":"document:doc-2"}`},
			sent{method: "DELETE", uri: tuples, body: `{"subject":"folder:doc-1",` +
				`"relation":"parent","object":"document:doc-2"}`},
			sent{method: "GET", uri: tuples + "?object=document%3Adoc-2&relation=parent&" +
				"subject=folder%3Adoc-1"},
		},
	}
	var left []string
	for i, s := range steps {
		if err := s.call(t.Context(), h.builder); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		want := s.create
		want.registered = i + 1
		if got := h.take(); !slices.Equal(got, []sent{want}) {
			t.Errorf("%s sent %+v, want %+v, after the ledger held it", s.name, got, want)
		}
		left = append(left, s.name+" (not removed)")
	}
	err := h.ledger.Reconcile()
	wantErr := "the ledger does not reconcile, registered=15 completed=0 verified=0: " +
		strings.Join(left, ", ")
	if err == nil || err.Error() != wantErr {
		t.Errorf("the ledger before the teardown: %v, want %q", err, wantErr)
	}
	collections := []string{
		schema + "/resources", schema + "/roles", facts + "/tenants", facts + "/users",
		instances, assignments, tuples,
	}
	all := slices.Concat(collections, []string{
		schema + "/resources/document/roles", schema + "/resources/document/relations",
	})
	if got, want := h.counts(all), []int{2, 2, 1, 1, 3, 2, 2, 1, 1}; !slices.Equal(got, want) {
		t.Errorf("the fake holds %v objects of %v, want %v", got, all, want)
	}

	if err := h.ledger.Teardown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := h.ledger.Reconcile(); err != nil {
		t.Error(err)
	}
	var want []sent
	for _, s := range slices.Backward(steps) {
		want = append(want, s.remove, s.check)
	}
	for i := range want {
		want[i].registered = len(steps)
	}
	if got := h.take(); !slices.Equal(got, want) {
		t.Errorf("the teardown sent\n%+v\nwant\n%+v", got, want)
	}
	if got := h.counts(collections); !slices.Equal(got, make([]int, len(collections))) {
		t.Errorf("the fake holds %v objects of %v after the teardown, want none", got,
			collections)
	}
}

// TestBuilderChecksUnappliedRemovals tears down objects whose removal the API acknowledges with
// 204 and does not apply. The checks that the role assignment and the tuple are gone, which look
// for them in a list, must fail, and the objects with a route of their own must be removed and
// checked.
func TestBuilderChecksUnappliedRemovals(t *testing.T) {
	h := newHarness(t)
	h.builder.timeout = 500 * time.Millisecond
	ctx := t.Context()
	for _, call := range []func() error{
		func() error { return h.builder.Role(ctx, Role{Key: "member"}) },
		func() error { return h.builder.Tenant(ctx, "t1") },
		func() error { return h.builder.User(ctx, "u1@example.com") },
		func() error {
			return h.builder.RoleAssignment(ctx, RoleAssignment{
				User: "u1@example.com", Role: "member", Tenant: "t1",
			})
		},
		func() error {
			return h.builder.Tuple(ctx, Tuple{
				Subject: "folder:f1", Relation: "parent", Object: "document:doc-1",
			})
		},
	} {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	assignments, tuples := h.facts+"/role_assignments", h.facts+"/relationship_tuples"
	h.unapplied = func(req *http.Request) bool {
		return req.Method == http.MethodDelete &&
			(req.URL.Path == assignments || req.URL.Path == tuples)
	}
	err := h.ledger.Teardown(ctx)
	assignment := "role assignment of member to u1@example.com in t1"
	tuple := "relationship tuple folder:f1 parent document:doc-1"
	for _, name := range []string{assignment, tuple} {
		want := "checking that " + name + " is gone: framework: not done after 500ms"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Teardown returned %v, want an error with %q", err, want)
		}
	}
	want := "the ledger does not reconcile, registered=5 completed=5 verified=3: " + assignment +
		" (removed, not checked to be gone), " + tuple + " (removed, not checked to be gone)"
	if err := h.ledger.Reconcile(); err == nil || err.Error() != want {
		t.Errorf("Reconcile = %v, want %q", err, want)
	}
	collections := []string{
		h.schema + "/roles", h.facts + "/tenants", h.facts + "/users", assignments, tuples,
	}
	if got, want := h.counts(collections), []int{0, 0, 0, 1, 1}; !slices.Equal(got, want) {
		t.Errorf("the fake holds %v objects of %v after the teardown, want %v", got, collections,
			want)
	}
}

func TestBuilderRegistersFailedWrites(t *testing.T) {
	h := newHarness(t)
	ctx := t.Context()
	if err := h.builder.Tenant(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	h.take()
	for i, tc := range []struct {
		name string
		call func() error
		want string
	}{
		{"a second tenant t1", func() error { return h.builder.Tenant(ctx, "t1") }, "answered 409"},
		{"a role of a resource that does not exist", func() error {
			return h.builder.ResourceRole(ctx, ResourceRole{Resource: "nothing", Key: "viewer"})
		}, "answered 404"},
		{"a relation of a resource that does not exist", func() error {
			return h.builder.Relation(ctx, Relation{
				Resource: "nothing", Key: "parent", SubjectResource: "folder",
			})
		}, "answered 404"},
	} {
		if err := tc.call(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s returned %v, want an error with %q", tc.name, err, tc.want)
		}
		if got := h.take(); len(got) != 1 || got[0].registered != i+2 {
			t.Errorf("%s sent %+v, want one request after the ledger held it", tc.name, got)
		}
	}
	for _, tc := range []struct {
		name string
		call func() error
		want string
	}{
		{"a role assignment without a tenant", func() error {
			return h.builder.RoleAssignment(ctx, RoleAssignment{User: "u1", Role: "viewer"})
		}, "seed: the role assignment of viewer to u1 has no tenant, which its removal needs"},
		{"a resource instance without a tenant", func() error {
			return h.builder.Instance(ctx, Instance{Resource: "document", Key: "doc-1"})
		}, "seed: the resource instance document:doc-1 has no tenant, which the API requires"},
	} {
		if err := tc.call(); err == nil || err.Error() != tc.want {
			t.Errorf("%s returned %v, want %q", tc.name, err, tc.want)
		}
		if got := h.take(); len(got) != 0 || h.ledger.Counts().Registered != 4 {
			t.Errorf("%s sent %+v, and the ledger holds %v", tc.name, got, h.ledger.Counts())
		}
	}
	if err := h.ledger.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.ledger.Reconcile(); err != nil {
		t.Error(err)
	}
}

func TestLists(t *testing.T) {
	onDoc := RoleAssignment{
		User: "u1", Role: "viewer", Tenant: "t1", ResourceInstance: "document:doc-1",
	}
	inTenant := RoleAssignment{User: "u1", Role: "viewer", Tenant: "t1"}
	docRow := `{"id": "a1", "user": "u1", "role": "viewer", "tenant": "t1", ` +
		`"resource_instance": "document:doc-1", "user_id": "u"}`
	tenantRow := `{"id": "a2", "user": "u1", "role": "viewer", "tenant": "t1", ` +
		`"resource_instance": null}`
	tuple := tupleKey{Subject: "folder:f1", Relation: "parent", Object: "document:doc-1"}
	tupleRow := `{"subject": "folder:f1", "relation": "parent", "object": "document:doc-1", ` +
		`"tenant": "t1"}`
	for _, tc := range []struct {
		name  string
		check func([]byte) (bool, error)
		body  string
		want  bool
	}{
		{"the role on the instance", lists(onDoc), "[" + tenantRow + "," + docRow + "]", true},
		{"the role in the tenant only", lists(onDoc), "[" + tenantRow + "]", false},
		{"the role on the instance only", lists(inTenant), "[" + docRow + "]", false},
		{"the role in the tenant", lists(inTenant), "[" + docRow + "," + tenantRow + "]", true},
		{"no role", lists(inTenant), "[]", false},
		{"the tuple, in a tenant", lists(tuple), "[" + tupleRow + "]", true},
		{"another subject", lists(tuple), strings.Replace("["+tupleRow+"]", "f1", "f2", 1), false},
	} {
		if got, err := tc.check([]byte(tc.body)); err != nil || got != tc.want {
			t.Errorf("%s: lists = %t, %v; want %t", tc.name, got, err, tc.want)
		}
	}
	if _, err := lists(inTenant)([]byte(tenantRow)); err == nil ||
		!strings.Contains(err.Error(), "decoding the list") {
		t.Errorf("a body that is not a list returned %v", err)
	}
}
