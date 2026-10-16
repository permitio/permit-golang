package permit

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/internal/transport"
)

// The routes of the PDP's lists of who may do what, as mockpermit.Server.Handle takes them.
const (
	userPermissionsRoute = "POST /user-permissions"
	authorizedUsersRoute = "POST /authorized_users"
)

// The reply fixtures of the two routes, in testdata/replies, as the wire cases serve them.
const (
	permissionsFixture     = "Get_User_Permissions_user_permissions_post.200.json"
	authorizedUsersFixture = "authorized_users_authorized_users_post.200.json"
)

// replyFixtureOf returns the reply fixture name of testdata/replies, and fails tb when it cannot be
// read.
func replyFixtureOf(tb testing.TB, name string) string {
	tb.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "replies", name))
	if err != nil {
		tb.Fatal(err)
	}
	return string(data)
}

// defaultOp returns the operation id of rest.Default, and fails tb when it holds none.
func defaultOp(tb testing.TB, id string) rest.Op {
	tb.Helper()
	op, ok := rest.Default().Op(id)
	if !ok {
		tb.Fatalf("rest.Default holds no operation %s", id)
	}
	return op
}

// fixturePermissions are the permissions that permissionsFixture holds: those of a tenant, and
// those of a resource instance, each under the key that the PDP gives it.
var fixturePermissions = map[string]ObjectPermissions{
	"__tenant:tenant-a": {
		Tenant:      &TenantDetails{Key: "tenant-a", Attributes: map[string]any{"plan": "pro"}},
		Permissions: []string{"document:read"},
		Roles:       []string{"viewer"},
	},
	"document:2026-report": {
		Tenant: &TenantDetails{Key: "tenant-a", Attributes: map[string]any{}},
		Resource: &ResourceDetails{
			Type: "document", Key: "2026-report",
			Attributes: map[string]any{"pages": json.Number("12")},
		},
		Permissions: []string{"document:read", "document:update"},
		Roles:       []string{"editor"},
	},
}

// fixtureAuthorizedUsers is the result that authorizedUsersFixture holds.
var fixtureAuthorizedUsers = AuthorizedUsersResult{
	Resource: "document:2026-report",
	Tenant:   "tenant-a",
	Users: map[string][]AuthorizedUserAssignment{
		"alice": {{
			User: "alice", Tenant: "tenant-a", Resource: "document:2026-report", Role: "editor",
		}},
		"bob": {{User: "bob", Tenant: "tenant-a", Resource: "__tenant:tenant-a", Role: "viewer"}},
	},
}

// permissionCalls are a call of each method that lists who may do what, whose reply is a valid
// reply of its route.
var permissionCalls = []decisionCall{
	{
		"UserPermissions", opUserPermissions, userPermissionsRoute,
		func(ctx context.Context, c *Client) error {
			_, err := c.UserPermissions(ctx, alice)
			return err
		},
	},
	{
		"AuthorizedUsers", opAuthorizedUsers, authorizedUsersRoute,
		func(ctx context.Context, c *Client) error {
			_, err := c.AuthorizedUsers(ctx, "read", document)
			return err
		},
	},
}

// TestUserPermissionsBodies checks the body that UserPermissions sends for each option: none,
// each filter, every option with values of every JSON type in the context and the user's
// attributes, empty maps, which it leaves out rather than send {} or null, and options given more
// than once, of which the last of each wins. Nothing reaches the API.
func TestUserPermissionsBodies(t *testing.T) {
	const bare = `{"user":{"key":"alice"}}`
	for _, tc := range []struct {
		name string
		user User
		opts []PermissionsOption
		want string
	}{
		{name: "no option", user: alice, want: bare},
		{
			name: "tenants",
			user: alice,
			opts: []PermissionsOption{WithTenants("tenant-a", "tenant-b")},
			want: `{"user":{"key":"alice"},"tenants":["tenant-a","tenant-b"]}`,
		},
		{
			name: "resources",
			user: alice,
			opts: []PermissionsOption{WithResources("document:2026-report")},
			want: `{"user":{"key":"alice"},"resources":["document:2026-report"]}`,
		},
		{
			name: "resource types",
			user: alice,
			opts: []PermissionsOption{WithResourceTypes("document", "__tenant")},
			want: `{"user":{"key":"alice"},"resource_types":["document","__tenant"]}`,
		},
		{
			name: "every option, with typed values",
			user: User{
				Key: fixtures.TypedKey, FirstName: "Alice", LastName: "Liddell",
				Email:      "alice@example.com",
				Attributes: map[string]any{"age": 30, "vip": true, "id": fixtures.TwoPow53Plus1},
			},
			opts: []PermissionsOption{
				WithTenants("tenant-a"),
				WithResources("document:2026-report", "folder:"+fixtures.UUIDKey),
				WithResourceTypes("document"),
				WithDecisionContext(map[string]any{
					"now": int64(1730000000), "ratio": 0.25, "ok": true, "nothing": nil,
					"source": fixtures.HTMLSpecial, "nested": map[string]any{"list": []any{1, "x"}},
				}),
			},
			want: `{
				"user": {
					"key": "` + fixtures.TypedKey + `", "first_name": "Alice",
					"last_name": "Liddell", "email": "alice@example.com",
					"attributes": {"age": 30, "vip": true, "id": 9007199254740993}
				},
				"tenants": ["tenant-a"],
				"resources": ["document:2026-report", "folder:` + fixtures.UUIDKey + `"],
				"resource_types": ["document"],
				"context": {
					"now": 1730000000, "ratio": 0.25, "ok": true, "nothing": null,
					"source": "<>&", "nested": {"list": [1, "x"]}
				}
			}`,
		},
		{
			name: "empty maps",
			user: User{Key: "alice", Attributes: map[string]any{}},
			opts: []PermissionsOption{WithDecisionContext(map[string]any{})},
			want: bare,
		},
		{
			name: "the last of each option",
			user: alice,
			opts: []PermissionsOption{
				WithTenants("a"), WithResources("x:1"), WithResourceTypes("x"),
				WithDecisionContext(map[string]any{"a": 1}),
				WithTenants("b"), WithResources("y:2"), WithResourceTypes("y"),
				WithDecisionContext(map[string]any{"b": 2}),
			},
			want: `{"user":{"key":"alice"},"tenants":["b"],"resources":["y:2"],` +
				`"resource_types":["y"],"context":{"b":2}}`,
		},
		{
			name: "a nil context last",
			user: alice,
			opts: []PermissionsOption{
				WithDecisionContext(map[string]any{"a": 1}), WithDecisionContext(nil),
			},
			want: bare,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			pdp.Handle(userPermissionsRoute, mockpermit.Reply{Body: `{}`})
			got, err := decisionClient(t, api, pdp).UserPermissions(t.Context(), tc.user,
				tc.opts...)
			if err != nil || got == nil || len(got) != 0 {
				t.Fatalf("UserPermissions returned %#v, %v; want an empty map", got, err)
			}
			onlyRequest(t, pdp).ExpectJSON(t, tc.want)
			api.AssertUntouched()
		})
	}
}

// TestPermissionsFiltersKeepTheirKeys checks that a filter keeps the keys it was given, although
// the caller changes the slice that it passed with ..., between building the option and the call.
func TestPermissionsFiltersKeepTheirKeys(t *testing.T) {
	keys := []string{"a", "b"}
	opts := []PermissionsOption{
		WithTenants(keys...), WithResources(keys...),
		WithResourceTypes(keys...),
	}
	keys[0] = "changed"
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	pdp.Handle(userPermissionsRoute, mockpermit.Reply{Body: `{}`})
	if _, err := decisionClient(t, api, pdp).UserPermissions(t.Context(), alice,
		opts...); err != nil {
		t.Fatalf("UserPermissions returned %v", err)
	}
	onlyRequest(t, pdp).ExpectJSON(t, `{"user":{"key":"alice"},"tenants":["a","b"],`+
		`"resources":["a","b"],"resource_types":["a","b"]}`)
}

// TestUserPermissionsKeepsTheKeys checks that UserPermissions returns each entry of the reply
// under the key that the PDP gives it, __tenant:<tenant> for a tenant and <type>:<key> for a
// resource instance, with its details, and an empty map, not nil, for a user without permissions.
func TestUserPermissionsKeepsTheKeys(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       map[string]ObjectPermissions
	}{
		{"the fixture", replyFixtureOf(t, permissionsFixture), fixturePermissions},
		{"no permissions", `{}`, map[string]ObjectPermissions{}},
		{
			"an entry without details",
			`{"__tenant:t":{"tenant":{"key":"t"},"resource":null,"permissions":[]}}`,
			map[string]ObjectPermissions{
				"__tenant:t": {Tenant: &TenantDetails{Key: "t"}, Permissions: []string{}},
			},
		},
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(userPermissionsRoute, mockpermit.Reply{Body: tc.body})
		got, err := decisionClient(t, api, pdp).UserPermissions(t.Context(), alice)
		if err != nil {
			t.Fatalf("UserPermissions of %s returned %v", tc.name, err)
		}
		if diff := cmp.Diff(tc.want, got); diff != "" {
			t.Errorf("UserPermissions of %s returned permissions that differ (-want +got):\n%s",
				tc.name, diff)
		}
	}
}

// TestAuthorizedUsersBodies checks the body that AuthorizedUsers sends for each kind of input: a
// resource type, a resource without a tenant, which it sends in the tenant default, every field of
// the resource and a context with values of every JSON type, empty maps, which it leaves out, and
// contexts given more than once, of which the last wins. Nothing reaches the API.
func TestAuthorizedUsersBodies(t *testing.T) {
	const typeBody = `{"action":"read","resource":{"type":"document","tenant":"default"}}`
	for _, tc := range []struct {
		name     string
		resource Resource
		opts     []CheckOption
		want     string
	}{
		{name: "a resource type", resource: Resource{Type: "document"}, want: typeBody},
		{
			name:     "an instance without a tenant",
			resource: document,
			want: `{"action":"read","resource":{"type":"document","key":"2026-report",` +
				`"tenant":"default"}}`,
		},
		{
			name: "every field, with typed values",
			resource: Resource{
				Type: "document", Key: fixtures.UUIDKey, Tenant: "tenant-a",
				Attributes: map[string]any{"pages": int64(12), "draft": false},
				Context:    map[string]any{"source": fixtures.HTMLSpecial},
			},
			opts: []CheckOption{WithDecisionContext(map[string]any{
				"now": int64(1730000000), "ratio": 0.25, "nothing": nil,
				"enable_abac_authorized_users": true, "id": fixtures.TwoPow53Plus1,
			})},
			want: `{
				"action": "read",
				"resource": {
					"type": "document", "key": "` + fixtures.UUIDKey + `", "tenant": "tenant-a",
					"attributes": {"pages": 12, "draft": false},
					"context": {"source": "<>&"}
				},
				"context": {
					"now": 1730000000, "ratio": 0.25, "nothing": null,
					"enable_abac_authorized_users": true, "id": 9007199254740993
				}
			}`,
		},
		{
			name: "empty maps",
			resource: Resource{
				Type: "document", Attributes: map[string]any{}, Context: map[string]any{},
			},
			opts: []CheckOption{WithDecisionContext(map[string]any{})},
			want: typeBody,
		},
		{
			name:     "the last context",
			resource: Resource{Type: "document"},
			opts: []CheckOption{
				WithDecisionContext(map[string]any{"a": 1}),
				WithDecisionContext(map[string]any{"b": 2}),
			},
			want: `{"action":"read","resource":{"type":"document","tenant":"default"},` +
				`"context":{"b":2}}`,
		},
		{
			name:     "a nil context last",
			resource: Resource{Type: "document"},
			opts: []CheckOption{
				WithDecisionContext(map[string]any{"a": 1}), WithDecisionContext(nil),
			},
			want: typeBody,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			pdp.Handle(authorizedUsersRoute, mockpermit.Reply{
				Body: `{"resource":"document:*","tenant":"default","users":{}}`,
			})
			got, err := decisionClient(t, api, pdp).AuthorizedUsers(t.Context(), "read",
				tc.resource, tc.opts...)
			want := AuthorizedUsersResult{
				Resource: "document:*", Tenant: "default",
				Users: map[string][]AuthorizedUserAssignment{},
			}
			if diff := cmp.Diff(want, got); err != nil || diff != "" {
				t.Fatalf("AuthorizedUsers returned %v, and a result that differs "+
					"(-want +got):\n%s", err, diff)
			}
			onlyRequest(t, pdp).ExpectJSON(t, tc.want)
			api.AssertUntouched()
		})
	}
}

// TestAuthorizedUsersReturnsTheUsers checks that AuthorizedUsers returns the resource, the tenant
// and the users of the reply, each with its role assignments, and ignores the fields that they do
// not have.
func TestAuthorizedUsersReturnsTheUsers(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       AuthorizedUsersResult
	}{
		{"the fixture", replyFixtureOf(t, authorizedUsersFixture), fixtureAuthorizedUsers},
		{
			"new fields",
			`{"resource":"document:*","tenant":"t","new":1,"users":{"alice":[],` +
				`"bob":[{"user":"bob","tenant":"t","resource":"__tenant:t","role":"admin",` +
				`"since":2026}]}}`,
			AuthorizedUsersResult{
				Resource: "document:*", Tenant: "t",
				Users: map[string][]AuthorizedUserAssignment{
					"alice": {},
					"bob":   {{User: "bob", Tenant: "t", Resource: "__tenant:t", Role: "admin"}},
				},
			},
		},
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(authorizedUsersRoute, mockpermit.Reply{Body: tc.body})
		got, err := decisionClient(t, api, pdp).AuthorizedUsers(t.Context(), "read", document)
		if err != nil {
			t.Fatalf("AuthorizedUsers of %s returned %v", tc.name, err)
		}
		if diff := cmp.Diff(tc.want, got); diff != "" {
			t.Errorf("AuthorizedUsers of %s returned a result that differs (-want +got):\n%s",
				tc.name, diff)
		}
	}
}

// TestPermissionsOfBothPDPs checks that the PDP container and the cloud PDP both serve the two
// routes: each method returns the reply of each, and sends one request to it.
func TestPermissionsOfBothPDPs(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind mockpermit.PDPKind
	}{{"container", mockpermit.Container}, {"cloud", mockpermit.Cloud}} {
		kind := tc.name
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t, tc.kind)
		pdp.Handle(userPermissionsRoute,
			mockpermit.Reply{Body: replyFixtureOf(t, permissionsFixture)})
		pdp.Handle(authorizedUsersRoute,
			mockpermit.Reply{Body: replyFixtureOf(t, authorizedUsersFixture)})
		c := decisionClient(t, api, pdp)
		permissions, err := c.UserPermissions(t.Context(), alice)
		if diff := cmp.Diff(fixturePermissions, permissions); err != nil || diff != "" {
			t.Errorf("UserPermissions of the %s PDP returned %v, and permissions that differ "+
				"(-want +got):\n%s", kind, err, diff)
		}
		users, err := c.AuthorizedUsers(t.Context(), "read", document)
		if diff := cmp.Diff(fixtureAuthorizedUsers, users); err != nil || diff != "" {
			t.Errorf("AuthorizedUsers of the %s PDP returned %v, and a result that differs "+
				"(-want +got):\n%s", kind, err, diff)
		}
		if n := len(pdp.Requests()); n != 2 {
			t.Errorf("the %s PDP got %d requests, want 2", kind, n)
		}
		api.AssertUntouched()
	}
}

// TestPermissionsErrorReplies checks that a reply with an error status is an *Error of the call,
// with the status and the body of the reply, for each method: one that ends the call, such as 400,
// 403, which matches ErrForbidden, or 422, and a 429 or a 503 after the retries. Nothing reaches
// the API.
func TestPermissionsErrorReplies(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		// detail is the Detail that the Error reads from the body.
		detail string
	}{
		{
			http.StatusBadRequest, `{"code":400,"message":"the query is not valid"}`,
			"the query is not valid",
		},
		{
			http.StatusForbidden, `{"code":403,"message":"the API key may not read this ` +
				`environment"}`, "the API key may not read this environment",
		},
		{
			http.StatusUnprocessableEntity,
			`{"detail":[{"loc":["body","user","key"],"msg":"field required"}]}`,
			"body.user.key: field required",
		},
		{http.StatusTooManyRequests, `{"detail":"Too Many Requests"}`, "Too Many Requests"},
		{http.StatusServiceUnavailable, `<html>busy</html>`, ""},
	} {
		retried := tc.status == http.StatusTooManyRequests ||
			tc.status == http.StatusServiceUnavailable
		for _, pc := range permissionCalls {
			t.Run(pc.method+"/"+http.StatusText(tc.status), func(t *testing.T) {
				api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
				reply := mockpermit.Reply{
					Status: tc.status, Body: tc.body,
					Header: http.Header{"Retry-After": {"0"}},
				}
				replies := []mockpermit.Reply{reply}
				if retried {
					replies = append(replies, reply)
				}
				pdp.Handle(pc.route, replies...)
				c := decisionClient(t, api, pdp, WithRetry(RetryPolicy{MaxRetries: 1}))
				err := pc.call(t.Context(), c)
				want := &Error{
					Op:         pc.op,
					Method:     http.MethodPost,
					Route:      strings.TrimPrefix(pc.route, "POST "),
					StatusCode: tc.status,
					Detail:     tc.detail,
					Body:       []byte(tc.body),
				}
				if diff := cmp.Diff(want, err); diff != "" {
					t.Errorf("%s returned an error that differs (-want +got):\n%s", pc.method,
						diff)
				}
				if forbidden := tc.status == http.StatusForbidden; errors.Is(err,
					ErrForbidden) != forbidden {
					t.Errorf("%s returned %v, which matches ErrForbidden: %t, want %t",
						pc.method, err, !forbidden, forbidden)
				}
				if n := len(pdp.Requests()); n != len(replies) {
					t.Errorf("%s sent %d requests for status %d, want %d", pc.method, n,
						tc.status, len(replies))
				}
				api.AssertUntouched()
			})
		}
	}
}

// TestPermissionsAreRetried checks that a call whose first reply is a 503 is sent again, as the
// two routes change nothing, and returns the result of the second reply.
func TestPermissionsAreRetried(t *testing.T) {
	bodies := map[string]string{
		userPermissionsRoute: `{}`,
		authorizedUsersRoute: `{"resource":"document:*","tenant":"default","users":{}}`,
	}
	for _, pc := range permissionCalls {
		t.Run(pc.method, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			pdp.Handle(pc.route, mockpermit.Reply{
				Status: http.StatusServiceUnavailable, Body: `{"detail":"busy"}`,
				Header: http.Header{"Retry-After": {"0"}},
			}, mockpermit.Reply{Body: bodies[pc.route]})
			if err := pc.call(t.Context(), decisionClient(t, api, pdp)); err != nil {
				t.Errorf("%s returned %v", pc.method, err)
			}
			if n := len(pdp.Requests()); n != 2 {
				t.Errorf("%s sent %d requests, want 2", pc.method, n)
			}
			api.AssertUntouched()
		})
	}
}

// TestPermissionsWithoutAReply checks that a call whose connection is refused returns an *Error
// of the call with status 0 whose cause is the *url.Error.
func TestPermissionsWithoutAReply(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	for _, pc := range permissionCalls {
		clearEnv(t)
		c := mustNew(t, WithAPIKey(canary.Key("refused")), WithAPIURL(srv.URL),
			WithPDPURL(srv.URL), WithRetry(RetryPolicy{}))
		t.Cleanup(func() { _ = c.Close() })
		err := pc.call(t.Context(), c)
		e := mustError(t, err)
		if _, isURLError := errors.AsType[*url.Error](err); !isURLError || e.StatusCode != 0 ||
			e.Op != pc.op {
			t.Errorf("%s returned %v, want the *Error of %s with status 0 that wraps a "+
				"*url.Error", pc.method, err, pc.op)
		}
	}
}

// TestPermissionsRepliesAddNoCredential checks that the replies of the two routes, whose
// attributes and assignments are the application's values, add nothing to the client's Set of
// credentials, not even a value under a credential name such as token, while the reply of an
// operation of the API still does.
func TestPermissionsRepliesAddNoCredential(t *testing.T) {
	const returned = "opaque-returned-value"
	echoed := []string{"opaque-echo-permissions", "opaque-echo-users"}
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	pdp.Handle(userPermissionsRoute, mockpermit.Reply{
		Body: `{"__tenant:t":{"tenant":{"key":"t","attributes":{"token":"` + echoed[0] +
			`"}},"permissions":[]}}`,
	})
	pdp.Handle(authorizedUsersRoute, mockpermit.Reply{
		Body: `{"resource":"document:*","tenant":"t","users":{"alice":[{"user":"alice",` +
			`"tenant":"t","resource":"__tenant:t","role":"admin","password":"` + echoed[1] +
			`"}]}}`,
	})
	api.Handle(route(opGet), mockpermit.Reply{Body: `{"key":"a","token":"` + returned + `"}`})
	c := wireClient(t, api, pdp)
	if _, err := c.UserPermissions(t.Context(), alice); err != nil {
		t.Fatalf("UserPermissions returned %v", err)
	}
	if _, err := c.AuthorizedUsers(t.Context(), "read", document); err != nil {
		t.Fatalf("AuthorizedUsers returned %v", err)
	}
	for _, value := range echoed {
		if c.secrets.Contains(value) {
			t.Errorf("a reply of the PDP added %s to the client's Set", value)
		}
	}
	if _, err := do[testObject](t.Context(), c, get("a")); err != nil {
		t.Fatalf("the call of the API returned %v", err)
	}
	if !c.secrets.Contains(returned) {
		t.Error("the reply of an operation of the API did not add its token to the client's Set")
	}
}

// expectMalformed checks that err, the error of a call of op whose reply had status 200 and a
// body that holds key, is a *Error with status 200 that matches ErrMalformedDecision and holds
// nothing of the body.
func expectMalformed(t *testing.T, err error, op, body, key string) {
	t.Helper()
	e := mustError(t, err)
	if !errors.Is(err, ErrMalformedDecision) || e.StatusCode != http.StatusOK || e.Op != op ||
		e.Body != nil || strings.Contains(err.Error(), key) {
		t.Errorf("the reply %q of %s gave %v; want a *Error with status 200 that matches "+
			"ErrMalformedDecision and holds nothing of the body", body, op, err)
	}
}

// TestMalformedUserPermissions checks that UserPermissions returns an *Error that matches
// ErrMalformedDecision, never an empty map, for each reply in 200-299 that is not a JSON object of
// entries, each a JSON object with one key permissions, spelled so, whose value is a list of
// strings: also one whose permissions differ in case or are there twice, one that holds a key
// twice, and one that wraps the permissions in another object, as OPA's reply does. It checks
// that UserPermissions ignores fields that ObjectPermissions does not have, and reads null
// details as none.
func TestMalformedUserPermissions(t *testing.T) {
	key := canary.Key("permissions")
	for _, body := range []string{
		``, `null`, `[]`, `true`, `"__tenant:a"`, `{`, `{} {}`, `{"__tenant:a":{}`,
		`<html>` + key + `</html>`,
		`{"__tenant:a":null}`, `{"__tenant:a":[]}`, `{"__tenant:a":"document:read"}`,
		`{"__tenant:a":{}}`, `{"__tenant:a":{"permissions":null}}`,
		`{"__tenant:a":{"permissions":"document:read"}}`, `{"__tenant:a":{"permissions":{}}}`,
		`{"__tenant:a":{"permissions":[1]}}`, `{"__tenant:a":{"permissions":[null]}}`,
		`{"__tenant:a":{"permissions":["document:read",null]}}`,
		`{"__tenant:a":{"permissions":["document:read",{}]}}`,
		`{"__tenant:a":{"Permissions":["document:read"]}}`,
		`{"__tenant:a":{"permissions":["document:read"],"PERMISSIONS":[]}}`,
		`{"__tenant:a":{"permissions":[],"permissions":["document:read"]}}`,
		`{"__tenant:a":{"permissions":[]},"__tenant:a":{"permissions":[]}}`,
		`{"__tenant:a":{"permissions":[]},"document:b":{"permissions":[]},` +
			`"__tenant:a":{"permissions":[]}}`,
		`{"permissions":{"__tenant:a":{"permissions":["document:read"]}}}`,
		`{"result":{"permissions":{"__tenant:a":{"permissions":["document:read"]}}}}`,
		`{"__tenant:a":{"permissions":[],"tenant":"a"}}`,
		`{"__tenant:a":{"permissions":[],"roles":"admin"}}`,
		`{"__tenant:a":{"permissions":[]}} []`, `{"__tenant:a":{"permissions":[],}}`,
		`{"__tenant:a":{"permissions":[]},"` + key + `":1}`,
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(userPermissionsRoute, mockpermit.Reply{Status: http.StatusOK, Body: body})
		got, err := decisionClient(t, api, pdp).UserPermissions(t.Context(), alice)
		if got != nil {
			t.Errorf("UserPermissions of the reply %q returned %v with its error", body, got)
		}
		expectMalformed(t, err, opUserPermissions, body, key)
	}
	for body, want := range map[string]map[string]ObjectPermissions{
		` { } `: {},
		`{"__tenant:a":{"permissions":["document:read"],"tenant":null,"resource":null,` +
			`"roles":null,"new":[1]}}`: {
			"__tenant:a": {Permissions: []string{"document:read"}},
		},
		`{"__tenant:a":{"permissions":[],"query":{"Permissions":[1]}}}`: {
			"__tenant:a": {Permissions: []string{}},
		},
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(userPermissionsRoute, mockpermit.Reply{Body: body})
		got, err := decisionClient(t, api, pdp).UserPermissions(t.Context(), alice)
		if diff := cmp.Diff(want, got); err != nil || diff != "" {
			t.Errorf("UserPermissions of the reply %s returned %v, and permissions that differ "+
				"(-want +got):\n%s", body, err, diff)
		}
	}
}

// TestMalformedAuthorizedUsers checks that AuthorizedUsers returns an *Error that matches
// ErrMalformedDecision, never a result without users, for each reply in 200-299 that is not a JSON
// object with one key each of resource and tenant, spelled so, whose values are strings, and
// users, whose value is a JSON object that maps each user to a list of JSON objects: also one with
// a key that differs in case or is there twice, and one whose users hold a key twice.
func TestMalformedAuthorizedUsers(t *testing.T) {
	key := canary.Key("users")
	const rt = `"resource":"document:*","tenant":"t",`
	for _, body := range []string{
		``, `null`, `[]`, `true`, `{}`, `{"resource":"document:*","tenant":"t"}`, `{`,
		`<html>` + key + `</html>`, `{` + rt + `"users":{}} {}`, `{` + rt + `"users":{},}`,
		`{` + rt + `"users":null}`, `{` + rt + `"users":[]}`, `{` + rt + `"users":"alice"}`,
		`{"tenant":"t","users":{}}`, `{"resource":"document:*","users":{}}`,
		`{"resource":null,"tenant":"t","users":{}}`, `{"resource":"document:*","tenant":1,` +
			`"users":{}}`, `{"resource":{},"tenant":"t","users":{}}`,
		`{` + rt + `"Users":{}}`, `{` + rt + `"users":{},"USERS":{}}`,
		`{` + rt + `"users":{},"users":{}}`, `{"Resource":"x",` + rt + `"users":{}}`,
		`{"resource":"document:*","resource":"document:*","tenant":"t","users":{}}`,
		`{"resource":"document:*","tenant":"t","Tenant":"t","users":{}}`,
		`{"resource":"document:*","tenant":null,"users":{}}`,
		`{"resource":"document:*","tenant":"t","tenant":"t","users":{}}`,
		`{` + rt + `"users":{"alice":null}}`, `{` + rt + `"users":{"alice":{}}}`,
		`{` + rt + `"users":{"alice":"admin"}}`, `{` + rt + `"users":{"alice":[null]}}`,
		`{` + rt + `"users":{"alice":[{},null]}}`, `{` + rt + `"users":{"alice":["admin"]}}`,
		`{` + rt + `"users":{"alice":[{"user":1}]}}`,
		`{` + rt + `"users":{"alice":[],"alice":[]}}`,
		`{` + rt + `"users":{"alice":[],"bob":[],"alice":[{}]}}`,
		`{"result":{` + rt + `"users":{}}}`, `{` + rt + `"users":{"` + key + `":null}}`,
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(authorizedUsersRoute, mockpermit.Reply{Status: http.StatusOK, Body: body})
		got, err := decisionClient(t, api, pdp).AuthorizedUsers(t.Context(), "read", document)
		if !cmp.Equal(got, AuthorizedUsersResult{}) {
			t.Errorf("AuthorizedUsers of the reply %q returned %v with its error", body, got)
		}
		expectMalformed(t, err, opAuthorizedUsers, body, key)
	}
}

// TestMalformedPermissionsTexts checks the texts of the errors of malformed replies of the two
// routes, which say what is wrong without quoting the reply.
func TestMalformedPermissionsTexts(t *testing.T) {
	const (
		permissions = "permit: Get_User_Permissions_user_permissions_post: POST " +
			"/user-permissions: status 200: permit: malformed decision: "
		users = "permit: authorized_users_authorized_users_post: POST /authorized_users: " +
			"status 200: permit: malformed decision: "
		rt = `"resource":"document:*","tenant":"t",`
	)
	for _, tc := range []struct {
		route, body, want string
	}{
		{userPermissionsRoute, `[]`, permissions + "the reply is not a JSON object that maps " +
			"the key of each tenant or resource instance to its permissions"},
		{userPermissionsRoute, `{"a:b":{"permissions":[]},"c:d":{}}`, permissions + "entry 2 " +
			"of the reply is not a JSON object with one key permissions whose value is a list " +
			"of strings"},
		{
			userPermissionsRoute, `{"a:b":{"permissions":[]},"a:b":{"permissions":[]}}`,
			permissions + "entry 2 of the reply has the key of an earlier entry",
		},
		{authorizedUsersRoute, `{}`, users + "the reply is not a JSON object with one key " +
			"each of resource and tenant, whose values are strings, and users, whose value is " +
			"a JSON object"},
		{authorizedUsersRoute, `{` + rt + `"users":{"alice":[],"bob":[null]}}`, users +
			"user 2 of the reply is not a list of role assignments, each a JSON object"},
		{authorizedUsersRoute, `{` + rt + `"users":{"alice":[],"alice":[]}}`, users +
			"user 2 of the reply has the key of an earlier user"},
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(tc.route, mockpermit.Reply{Body: tc.body})
		c := decisionClient(t, api, pdp)
		var err error
		if tc.route == userPermissionsRoute {
			_, err = c.UserPermissions(t.Context(), alice)
		} else {
			_, err = c.AuthorizedUsers(t.Context(), "read", document)
		}
		if err == nil || err.Error() != tc.want {
			t.Errorf("the reply %s gave the error\n%v\nwant\n%s", tc.body, err, tc.want)
		}
	}
}

// TestInvalidPermissionsCalls checks that a call of a user without a key or of a resource without
// a type, which every PDP rejects, and one with a nil PermissionsOption, the zero CheckOption or a
// filter without keys, returns an error that matches ErrInvalidConfig and names what is wrong,
// and sends no request.
func TestInvalidPermissionsCalls(t *testing.T) {
	const config = "permit: invalid configuration: "
	const (
		noKey = "the user key is empty; give the key of the user, as the PDP rejects a request " +
			"without one"
		noType = "the resource type is empty; give the key of the resource type, such as " +
			"document, as the PDP rejects a request without one"
		noOption = "the CheckOption is the zero value; build options with the With functions, " +
			"such as WithDecisionContext"
		nilOption = "the PermissionsOption is nil; build options with the With functions, such " +
			"as WithTenants"
		noKeys = "the filter has no keys; give at least one, or leave the option out to get " +
			"the permissions of every tenant, resource or resource type"
	)
	for _, tc := range []struct {
		want string
		call func(ctx context.Context, c *Client) error
	}{
		{"UserPermissions: user: " + noKey, func(ctx context.Context, c *Client) error {
			_, err := c.UserPermissions(ctx, User{Email: "alice@example.com"},
				WithTenants("t"))
			return err
		}},
		{"UserPermissions: option 2: " + nilOption, func(ctx context.Context, c *Client) error {
			_, err := c.UserPermissions(ctx, alice, WithTenants("t"), nil)
			return err
		}},
		{"UserPermissions: option 1: " + noOption, func(ctx context.Context, c *Client) error {
			_, err := c.UserPermissions(ctx, alice, CheckOption{})
			return err
		}},
		{"UserPermissions: option 1: " + nilOption, func(ctx context.Context, c *Client) error {
			var option *CheckOption
			_, err := c.UserPermissions(ctx, alice, option)
			return err
		}},
		{"UserPermissions: option 1: " + noKeys, func(ctx context.Context, c *Client) error {
			_, err := c.UserPermissions(ctx, alice, WithTenants())
			return err
		}},
		{"UserPermissions: option 2: " + noKeys, func(ctx context.Context, c *Client) error {
			_, err := c.UserPermissions(ctx, alice, WithTenants("t"), WithResources())
			return err
		}},
		{"UserPermissions: option 3: " + noKeys, func(ctx context.Context, c *Client) error {
			_, err := c.UserPermissions(ctx, alice, WithTenants("t"),
				WithDecisionContext(nil), WithResourceTypes([]string{}...))
			return err
		}},
		{"AuthorizedUsers: resource: " + noType, func(ctx context.Context, c *Client) error {
			_, err := c.AuthorizedUsers(ctx, "read", Resource{Key: "2026-report"})
			return err
		}},
		{"AuthorizedUsers: option 2: " + noOption, func(ctx context.Context, c *Client) error {
			_, err := c.AuthorizedUsers(ctx, "read", document, WithDecisionContext(nil),
				CheckOption{})
			return err
		}},
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		err := tc.call(t.Context(), decisionClient(t, api, pdp))
		if !errors.Is(err, ErrInvalidConfig) || err.Error() != config+tc.want {
			t.Errorf("the call returned\n%v\nwant\n%s", err, config+tc.want)
		}
		pdp.AssertUntouched()
		api.AssertUntouched()
	}
}

// TestPermissionsBodyThatIsNotJSON checks that a call whose context holds a value that JSON
// cannot hold returns an error that matches ErrInvalidConfig, and sends no request.
func TestPermissionsBodyThatIsNotJSON(t *testing.T) {
	nan := WithDecisionContext(map[string]any{"ratio": math.NaN()})
	for _, pc := range []decisionCall{
		{
			"UserPermissions", opUserPermissions, userPermissionsRoute,
			func(ctx context.Context, c *Client) error {
				_, err := c.UserPermissions(ctx, alice, nan)
				return err
			},
		},
		{
			"AuthorizedUsers", opAuthorizedUsers, authorizedUsersRoute,
			func(ctx context.Context, c *Client) error {
				_, err := c.AuthorizedUsers(ctx, "read", document, nan)
				return err
			},
		},
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		err := pc.call(t.Context(), decisionClient(t, api, pdp))
		want := "permit: invalid configuration: " + pc.op + ": the request body cannot be " +
			"written as JSON"
		if !errors.Is(err, ErrInvalidConfig) || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("%s returned %v, want an error that starts with %q", pc.method, err, want)
		}
		pdp.AssertUntouched()
	}
}

// TestPermissionsAfterClose checks that both methods return an error that matches ErrClosed after
// Close, and send no request.
func TestPermissionsAfterClose(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	c := decisionClient(t, api, pdp)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	for _, pc := range permissionCalls {
		if err := pc.call(t.Context(), c); !errors.Is(err, ErrClosed) {
			t.Errorf("%s after Close returned %v, want ErrClosed", pc.method, err)
		}
	}
	pdp.AssertUntouched()
	api.AssertUntouched()
}

// FuzzPermissionsReplies checks the readers of the two replies on any body: each returns a result
// or an *Error that matches ErrMalformedDecision, never both and never a panic. A result of
// UserPermissions has an entry for each key of the reply, each with a list of permissions, and
// one of AuthorizedUsers has a list of assignments for each of its users.
func FuzzPermissionsReplies(f *testing.F) {
	for _, body := range []string{
		replyFixtureOf(f, permissionsFixture), replyFixtureOf(f, authorizedUsersFixture),
		`{}`, `null`, `{"a:b":{"permissions":[]},"a:b":{"permissions":[]}}`,
		`{"resource":"r","tenant":"t","users":{"alice":[{}]}}`,
		`{"a:b":{"Permissions":[]}}`, `{"permissions":{"a:b":{"permissions":["x:y"]}}}`,
	} {
		f.Add([]byte(body))
	}
	permissionsOp, usersOp := defaultOp(f, opUserPermissions), defaultOp(f, opAuthorizedUsers)
	f.Fuzz(func(t *testing.T, body []byte) {
		reply := transport.Reply{Status: http.StatusOK, Body: body}
		permissions, err := readPermissions(permissionsOp, reply)
		checkFuzzResult(t, "readPermissions", body, permissions != nil, err)
		for key, p := range permissions {
			if p.Permissions == nil {
				t.Errorf("readPermissions of %q returned no list of permissions under %q", body,
					key)
			}
		}
		var keys map[string]json.RawMessage
		if err == nil && (json.Unmarshal(body, &keys) != nil || len(keys) != len(permissions)) {
			t.Errorf("readPermissions of %q returned %d entries for the keys %v", body,
				len(permissions), keys)
		}
		users, err := readAuthorizedUsers(usersOp, reply)
		checkFuzzResult(t, "readAuthorizedUsers", body, users.Users != nil, err)
		for user, assignments := range users.Users {
			if assignments == nil {
				t.Errorf("readAuthorizedUsers of %q returned no assignments for %q", body, user)
			}
		}
	})
}

// checkFuzzResult checks that a reader of a reply, name, returned a result, as got says, or an
// error that matches ErrMalformedDecision, and not both, for body.
func checkFuzzResult(t *testing.T, name string, body []byte, got bool, err error) {
	t.Helper()
	if got == (err != nil) || err != nil && !errors.Is(err, ErrMalformedDecision) {
		t.Errorf("%s of %q returned a result: %t, and the error %v; want a result or an error "+
			"that matches ErrMalformedDecision", name, body, got, err)
	}
}
