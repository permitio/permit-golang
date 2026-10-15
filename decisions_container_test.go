package permit

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/internal/transport"
)

// The routes of the decisions that only the PDP container serves, as mockpermit.Server.Handle
// takes them.
const (
	allTenantsRoute  = "POST /allowed/all-tenants"
	userTenantsRoute = "POST /user-tenants"
	checkURLRoute    = "POST /allowed_url"
)

// The reply fixtures of the three routes, in testdata/replies, as the wire cases serve them.
const (
	allTenantsFixture  = "is_allowed_all_tenants_allowed_all_tenants_post.200.json"
	userTenantsFixture = "Get_User_Tenants_user_tenants_post.200.json"
	checkURLFixture    = "is_allowed_url_allowed_url_post.200.json"
)

// reportURL is the URL of the checks of a URL of the tests that do not test it.
const reportURL = "https://app.example.com/documents/2026-report"

// containerCall is a call of a method of the decisions that only the PDP container serves.
type containerCall struct {
	// method names the method.
	method string

	// op is the ID of the operation that the call sends.
	op string

	// route is the route of the operation, as mockpermit.Server.Handle takes it.
	route string

	// valid is a reply body in 200-299 of the route that the method reads.
	valid string

	// zero is the result that the method returns with an error.
	zero any

	// call makes the call with c, and returns its result and its error.
	call func(ctx context.Context, c *Client) (any, error)
}

// containerCalls are a call of each method of the decisions that only the PDP container serves.
var containerCalls = []containerCall{
	{
		"AllTenantsCheck", opAllTenantsCheck, allTenantsRoute, `{"allowed_tenants":[]}`,
		[]TenantDetails(nil),
		func(ctx context.Context, c *Client) (any, error) {
			return c.AllTenantsCheck(ctx, alice, "read", document)
		},
	},
	{
		"UserTenants", opUserTenants, userTenantsRoute, `[]`, []TenantDetails(nil),
		func(ctx context.Context, c *Client) (any, error) {
			return c.UserTenants(ctx, alice)
		},
	},
	{
		"CheckURL", opCheckURL, checkURLRoute, `{"allow":true}`, false,
		func(ctx context.Context, c *Client) (any, error) {
			return c.CheckURL(ctx, alice, http.MethodGet, reportURL, "tenant-a")
		},
	},
}

// TestAllTenantsCheckBodies checks the body that AllTenantsCheck sends for each kind of input: a
// resource type, a resource without a tenant, which it sends in the tenant default, every field of
// the user and of the resource with values of every JSON type, empty maps, which it leaves out
// rather than send {} or null, and contexts given more than once, of which the last wins. Nothing
// reaches the API.
func TestAllTenantsCheckBodies(t *testing.T) {
	const typeBody = `{"user":{"key":"alice"},"action":"read",` +
		`"resource":{"type":"document","tenant":"default"}}`
	for _, tc := range []struct {
		name     string
		user     User
		resource Resource
		opts     []CheckOption
		want     string
	}{
		{
			name: "a resource type", user: alice, resource: Resource{Type: "document"},
			want: typeBody,
		},
		{
			name: "an instance without a tenant", user: alice, resource: document,
			want: `{"user":{"key":"alice"},"action":"read",` +
				`"resource":{"type":"document","key":"2026-report","tenant":"default"}}`,
		},
		{
			name: "every field, with typed values",
			user: User{
				Key: fixtures.TypedKey, FirstName: "Alice", LastName: "Liddell",
				Email:      "alice@example.com",
				Attributes: map[string]any{"age": 30, "id": fixtures.TwoPow53Plus1},
			},
			resource: Resource{
				Type: "document", Key: fixtures.UUIDKey, Tenant: "tenant-a",
				Attributes: map[string]any{"pages": int64(12), "draft": false},
				Context:    map[string]any{"source": fixtures.HTMLSpecial},
			},
			opts: []CheckOption{WithDecisionContext(map[string]any{
				"now": int64(1730000000), "ratio": 0.25, "nothing": nil,
				"nested": map[string]any{"list": []any{1, "x"}},
			})},
			want: `{
				"user": {
					"key": "` + fixtures.TypedKey + `", "first_name": "Alice",
					"last_name": "Liddell", "email": "alice@example.com",
					"attributes": {"age": 30, "id": 9007199254740993}
				},
				"action": "read",
				"resource": {
					"type": "document", "key": "` + fixtures.UUIDKey + `", "tenant": "tenant-a",
					"attributes": {"pages": 12, "draft": false},
					"context": {"source": "<>&"}
				},
				"context": {
					"now": 1730000000, "ratio": 0.25, "nothing": null,
					"nested": {"list": [1, "x"]}
				}
			}`,
		},
		{
			name: "empty maps",
			user: User{Key: "alice", Attributes: map[string]any{}},
			resource: Resource{
				Type: "document", Attributes: map[string]any{}, Context: map[string]any{},
			},
			opts: []CheckOption{WithDecisionContext(map[string]any{})},
			want: typeBody,
		},
		{
			name: "the last context", user: alice, resource: Resource{Type: "document"},
			opts: []CheckOption{
				WithDecisionContext(map[string]any{"a": 1}),
				WithDecisionContext(map[string]any{"b": 2}),
			},
			want: `{"user":{"key":"alice"},"action":"read",` +
				`"resource":{"type":"document","tenant":"default"},"context":{"b":2}}`,
		},
		{
			name: "a nil context last", user: alice, resource: Resource{Type: "document"},
			opts: []CheckOption{
				WithDecisionContext(map[string]any{"a": 1}), WithDecisionContext(nil),
			},
			want: typeBody,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			pdp.Handle(allTenantsRoute, mockpermit.Reply{Body: `{"allowed_tenants":[]}`})
			got, err := decisionClient(t, api, pdp).AllTenantsCheck(t.Context(), tc.user, "read",
				tc.resource, tc.opts...)
			if err != nil || got == nil || len(got) != 0 {
				t.Fatalf("AllTenantsCheck returned %#v, %v; want an empty list", got, err)
			}
			onlyRequest(t, pdp).ExpectJSON(t, tc.want)
			api.AssertUntouched()
		})
	}
}

// TestUserTenantsBodies checks the body that UserTenants sends for each kind of input: a user
// alone, every field of the user and a context with values of every JSON type, empty maps, which
// it leaves out, and contexts given more than once, of which the last wins. Nothing reaches the
// API.
func TestUserTenantsBodies(t *testing.T) {
	const bare = `{"user":{"key":"alice"}}`
	for _, tc := range []struct {
		name string
		user User
		opts []CheckOption
		want string
	}{
		{name: "no option", user: alice, want: bare},
		{
			name: "every field, with typed values",
			user: User{
				Key: fixtures.TypedKey, FirstName: "Alice", LastName: "Liddell",
				Email:      "alice@example.com",
				Attributes: map[string]any{"age": 30, "vip": true, "id": fixtures.TwoPow53Plus1},
			},
			opts: []CheckOption{WithDecisionContext(map[string]any{
				"now": int64(1730000000), "ratio": 0.25, "nothing": nil,
				"source": fixtures.HTMLSpecial,
			})},
			want: `{
				"user": {
					"key": "` + fixtures.TypedKey + `", "first_name": "Alice",
					"last_name": "Liddell", "email": "alice@example.com",
					"attributes": {"age": 30, "vip": true, "id": 9007199254740993}
				},
				"context": {"now": 1730000000, "ratio": 0.25, "nothing": null, "source": "<>&"}
			}`,
		},
		{
			name: "empty maps",
			user: User{Key: "alice", Attributes: map[string]any{}},
			opts: []CheckOption{WithDecisionContext(map[string]any{})},
			want: bare,
		},
		{
			name: "the last context",
			user: alice,
			opts: []CheckOption{
				WithDecisionContext(map[string]any{"a": 1}),
				WithDecisionContext(map[string]any{"b": 2}),
			},
			want: `{"user":{"key":"alice"},"context":{"b":2}}`,
		},
		{
			name: "a nil context last",
			user: alice,
			opts: []CheckOption{
				WithDecisionContext(map[string]any{"a": 1}), WithDecisionContext(nil),
			},
			want: bare,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			pdp.Handle(userTenantsRoute, mockpermit.Reply{Body: `[]`})
			got, err := decisionClient(t, api, pdp).UserTenants(t.Context(), tc.user, tc.opts...)
			if err != nil || got == nil || len(got) != 0 {
				t.Fatalf("UserTenants returned %#v, %v; want an empty list", got, err)
			}
			onlyRequest(t, pdp).ExpectJSON(t, tc.want)
			api.AssertUntouched()
		})
	}
}

// TestCheckURLBodies checks the body that CheckURL sends: the method, the URL and the tenant as
// they are given, with <, > and & kept, also an empty method, the tenant default for an empty
// tenant, every field of the user and a context with values of every JSON type, empty maps, which
// it leaves out, and contexts given more than once, of which the last wins. Nothing reaches the
// API.
func TestCheckURLBodies(t *testing.T) {
	const bare = `{"user":{"key":"alice"},"http_method":"GET","url":"` + reportURL + `",` +
		`"tenant":"t"}`
	for _, tc := range []struct {
		name, method, url, tenant string
		user                      User
		opts                      []CheckOption
		want                      string
	}{
		{
			name: "a tenant", user: alice, method: http.MethodGet, url: reportURL,
			tenant: "tenant-a",
			want: `{"user":{"key":"alice"},"http_method":"GET","url":"` + reportURL + `",` +
				`"tenant":"tenant-a"}`,
		},
		{
			name: "no tenant", user: alice, method: http.MethodDelete, url: reportURL,
			want: `{"user":{"key":"alice"},"http_method":"DELETE","url":"` + reportURL + `",` +
				`"tenant":"default"}`,
		},
		{
			name: "every field, with typed values",
			user: User{
				Key: fixtures.TypedKey, FirstName: "Alice", LastName: "Liddell",
				Email:      "alice@example.com",
				Attributes: map[string]any{"age": 30, "id": fixtures.TwoPow53Plus1},
			},
			method: "post",
			url:    "https://app.example.com/search?q=" + fixtures.HTMLSpecial + "&page=ü",
			tenant: fixtures.UUIDKey,
			opts: []CheckOption{WithDecisionContext(map[string]any{
				"now": int64(1730000000), "ratio": 0.25, "nothing": nil,
				"nested": map[string]any{"list": []any{1, "x"}}, "source": fixtures.HTMLSpecial,
			})},
			want: `{
				"user": {
					"key": "` + fixtures.TypedKey + `", "first_name": "Alice",
					"last_name": "Liddell", "email": "alice@example.com",
					"attributes": {"age": 30, "id": 9007199254740993}
				},
				"http_method": "post",
				"url": "https://app.example.com/search?q=<>&&page=ü",
				"tenant": "` + fixtures.UUIDKey + `",
				"context": {
					"now": 1730000000, "ratio": 0.25, "nothing": null,
					"nested": {"list": [1, "x"]}, "source": "<>&"
				}
			}`,
		},
		{
			name: "an empty method", user: alice, url: reportURL, tenant: "t",
			want: `{"user":{"key":"alice"},"http_method":"","url":"` + reportURL + `",` +
				`"tenant":"t"}`,
		},
		{
			name: "empty maps", user: User{Key: "alice", Attributes: map[string]any{}},
			method: http.MethodGet, url: reportURL, tenant: "t",
			opts: []CheckOption{WithDecisionContext(map[string]any{})},
			want: bare,
		},
		{
			name: "the last context", user: alice, method: http.MethodGet, url: reportURL,
			tenant: "t",
			opts: []CheckOption{
				WithDecisionContext(map[string]any{"a": 1}),
				WithDecisionContext(map[string]any{"b": 2}),
			},
			want: `{"user":{"key":"alice"},"http_method":"GET","url":"` + reportURL + `",` +
				`"tenant":"t","context":{"b":2}}`,
		},
		{
			name: "a nil context last", user: alice, method: http.MethodGet, url: reportURL,
			tenant: "t",
			opts: []CheckOption{
				WithDecisionContext(map[string]any{"a": 1}), WithDecisionContext(nil),
			},
			want: bare,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			pdp.Handle(checkURLRoute, mockpermit.Reply{Body: `{"allow":false}`})
			allowed, err := decisionClient(t, api, pdp).CheckURL(t.Context(), tc.user,
				tc.method, tc.url, tc.tenant, tc.opts...)
			if err != nil || allowed {
				t.Fatalf("CheckURL returned %t, %v; want false, nil", allowed, err)
			}
			onlyRequest(t, pdp).ExpectJSON(t, tc.want)
			api.AssertUntouched()
		})
	}
}

// TestAllTenantsCheckReturnsTheAllowedTenants checks that AllTenantsCheck returns the tenant of
// each decision of the reply whose allow is true, in the order of the reply, and none of those
// whose allow is false; an empty list, not nil, when the PDP allows none; and that it ignores the
// other fields of the reply and of each decision, such as result.
func TestAllTenantsCheckReturnsTheAllowedTenants(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       []TenantDetails
	}{
		{
			"the fixture", replyFixtureOf(t, allTenantsFixture),
			[]TenantDetails{{Key: "tenant-a", Attributes: map[string]any{"plan": "pro"}}},
		},
		{"no tenant", `{"allowed_tenants":[]}`, []TenantDetails{}},
		{
			"only denials",
			`{"allowed_tenants":[{"allow":false,"tenant":{"key":"a"}},` +
				`{"allow":false,"tenant":{"key":"b"}}]}`,
			[]TenantDetails{},
		},
		{
			"denials between allowed tenants",
			`{"allowed_tenants":[{"allow":true,"tenant":{"key":"c"}},` +
				`{"allow":false,"tenant":{"key":"b"}},` +
				`{"allow":true,"tenant":{"key":"a","attributes":{"seats":10}}}]}`,
			[]TenantDetails{
				{Key: "c"}, {Key: "a", Attributes: map[string]any{"seats": json.Number("10")}},
			},
		},
		{
			"other fields",
			`{"new":1,"allowed_tenants":[{"allow":true,"result":false,"query":{"allow":false},` +
				`"debug":null,"tenant":{"key":"a","attributes":null,"new":[1]}},` +
				`{"result":true,"allow":false,"tenant":{"key":"b"}}]}`,
			[]TenantDetails{{Key: "a"}},
		},
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(allTenantsRoute, mockpermit.Reply{Body: tc.body})
		got, err := decisionClient(t, api, pdp).AllTenantsCheck(t.Context(), alice, "read",
			document)
		if err != nil {
			t.Fatalf("AllTenantsCheck of %s returned %v", tc.name, err)
		}
		if diff := cmp.Diff(tc.want, got); diff != "" {
			t.Errorf("AllTenantsCheck of %s returned tenants that differ (-want +got):\n%s",
				tc.name, diff)
		}
	}
}

// TestUserTenantsReturnsTheTenants checks that UserTenants returns each tenant of the reply, in
// its order, with its attributes; an empty list, not nil, for a user without tenants; and that it
// ignores the fields that TenantDetails does not have.
func TestUserTenantsReturnsTheTenants(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       []TenantDetails
	}{
		{
			"the fixture", replyFixtureOf(t, userTenantsFixture),
			[]TenantDetails{
				{Key: "tenant-a", Attributes: map[string]any{"plan": "pro"}},
				{Key: "tenant-b", Attributes: map[string]any{}},
			},
		},
		{"no tenant", ` [ ] `, []TenantDetails{}},
		{
			"other fields",
			`[{"key":"b","new":1,"attributes":{"seats":10}},{"key":"a","attributes":null}]`,
			[]TenantDetails{
				{Key: "b", Attributes: map[string]any{"seats": json.Number("10")}}, {Key: "a"},
			},
		},
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(userTenantsRoute, mockpermit.Reply{Body: tc.body})
		got, err := decisionClient(t, api, pdp).UserTenants(t.Context(), alice)
		if err != nil {
			t.Fatalf("UserTenants of %s returned %v", tc.name, err)
		}
		if diff := cmp.Diff(tc.want, got); diff != "" {
			t.Errorf("UserTenants of %s returned tenants that differ (-want +got):\n%s", tc.name,
				diff)
		}
	}
}

// TestCheckURLReturnsTheDecision checks that CheckURL returns the allow of the reply, true or
// false, and ignores the other fields, such as result.
func TestCheckURLReturnsTheDecision(t *testing.T) {
	for body, want := range map[string]bool{
		replyFixtureOf(t, checkURLFixture):                          true,
		`{"allow":false}`:                                           false,
		`{"allow":false,"result":true,"query":{"allow":true}}`:      false,
		`{"debug":{"Allow":false},"allow":true,"new":[1],"x":null}`: true,
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(checkURLRoute, mockpermit.Reply{Body: body})
		allowed, err := decisionClient(t, api, pdp).CheckURL(t.Context(), alice, http.MethodGet,
			reportURL, "tenant-a")
		if err != nil || allowed != want {
			t.Errorf("CheckURL of the reply %s returned %t, %v; want %t, nil", body, allowed,
				err, want)
		}
	}
}

// TestContainerOnlyDecisionsOfTheCloudPDP checks each method against the cloud PDP, which answers
// its route with 404 and an empty body: the call returns an *Error that matches
// ErrContainerPDPOnly, not ErrNotFound, and names the operation and the route, and no result,
// after one request, as a 404 is never sent again. Nothing reaches the API.
func TestContainerOnlyDecisionsOfTheCloudPDP(t *testing.T) {
	for _, cc := range containerCalls {
		t.Run(cc.method, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t, mockpermit.Cloud)
			got, err := cc.call(t.Context(), decisionClient(t, api, pdp))
			e := mustError(t, err)
			want := "permit: " + cc.op + ": " + cc.route + ": status 404: permit: only the PDP " +
				"container serves this route"
			if !errors.Is(err, ErrContainerPDPOnly) || errors.Is(err, ErrNotFound) ||
				e.Error() != want || e.StatusCode != http.StatusNotFound {
				t.Errorf("%s returned\n%v\nwant an error that matches ErrContainerPDPOnly and "+
					"not ErrNotFound:\n%s", cc.method, err, want)
			}
			if diff := cmp.Diff(cc.zero, got); diff != "" {
				t.Errorf("%s returned a result with its error (-want +got):\n%s", cc.method, diff)
			}
			if n := len(pdp.Requests()); n != 1 {
				t.Errorf("%s sent %d requests to the cloud PDP, want 1", cc.method, n)
			}
			api.AssertUntouched()
		})
	}
}

// TestContainerOnlyDecisionsNotFound checks that a 404 with a JSON body, which the PDP container
// or a proxy before it sends, is an *Error that matches ErrNotFound, not ErrContainerPDPOnly, with
// the body, after one request.
func TestContainerOnlyDecisionsNotFound(t *testing.T) {
	const body = `{"detail":"Not Found"}`
	for _, cc := range containerCalls {
		t.Run(cc.method, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			pdp.Handle(cc.route, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
			_, err := cc.call(t.Context(), decisionClient(t, api, pdp))
			want := &Error{
				Op: cc.op, Method: http.MethodPost, Route: strings.TrimPrefix(cc.route, "POST "),
				StatusCode: http.StatusNotFound, Detail: "Not Found", Body: []byte(body),
			}
			if diff := cmp.Diff(want, err); diff != "" {
				t.Errorf("%s returned an error that differs (-want +got):\n%s", cc.method, diff)
			}
			if !errors.Is(err, ErrNotFound) || errors.Is(err, ErrContainerPDPOnly) {
				t.Errorf("%s returned %v, want an error that matches ErrNotFound only",
					cc.method, err)
			}
			if n := len(pdp.Requests()); n != 1 {
				t.Errorf("%s sent %d requests, want 1", cc.method, n)
			}
			api.AssertUntouched()
		})
	}
}

// TestContainerDecisionErrorReplies checks that a reply with an error status is an *Error of the
// call, with the status and the body of the reply, for each method: one that ends the call, such
// as 400, 403 or 422, and a 429 or a 503 after the retries. Nothing reaches the API.
func TestContainerDecisionErrorReplies(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		// detail is the Detail that the Error reads from the body.
		detail string
	}{
		{http.StatusBadRequest, `{"detail":"the query is not valid"}`, "the query is not valid"},
		{
			http.StatusForbidden, `{"detail":"the API key may not read this environment"}`,
			"the API key may not read this environment",
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
		for _, cc := range containerCalls {
			t.Run(cc.method+"/"+http.StatusText(tc.status), func(t *testing.T) {
				api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
				reply := mockpermit.Reply{
					Status: tc.status, Body: tc.body,
					Header: http.Header{"Retry-After": {"0"}},
				}
				replies := []mockpermit.Reply{reply}
				if retried {
					replies = append(replies, reply)
				}
				pdp.Handle(cc.route, replies...)
				c := decisionClient(t, api, pdp, WithRetry(RetryPolicy{MaxRetries: 1}))
				got, err := cc.call(t.Context(), c)
				want := &Error{
					Op:         cc.op,
					Method:     http.MethodPost,
					Route:      strings.TrimPrefix(cc.route, "POST "),
					StatusCode: tc.status,
					Detail:     tc.detail,
					Body:       []byte(tc.body),
				}
				if diff := cmp.Diff(want, err); diff != "" {
					t.Errorf("%s returned an error that differs (-want +got):\n%s", cc.method,
						diff)
				}
				if diff := cmp.Diff(cc.zero, got); diff != "" {
					t.Errorf("%s returned a result with its error (-want +got):\n%s", cc.method,
						diff)
				}
				if n := len(pdp.Requests()); n != len(replies) {
					t.Errorf("%s sent %d requests for status %d, want %d", cc.method, n,
						tc.status, len(replies))
				}
				api.AssertUntouched()
			})
		}
	}
}

// TestContainerDecisionsAreRetried checks that a call whose first reply is a 503 is sent again,
// as the three routes change nothing, and returns the result of the second reply.
func TestContainerDecisionsAreRetried(t *testing.T) {
	for _, cc := range containerCalls {
		t.Run(cc.method, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			pdp.Handle(cc.route, mockpermit.Reply{
				Status: http.StatusServiceUnavailable, Body: `{"detail":"busy"}`,
				Header: http.Header{"Retry-After": {"0"}},
			}, mockpermit.Reply{Body: cc.valid})
			if _, err := cc.call(t.Context(), decisionClient(t, api, pdp)); err != nil {
				t.Errorf("%s returned %v", cc.method, err)
			}
			if n := len(pdp.Requests()); n != 2 {
				t.Errorf("%s sent %d requests, want 2", cc.method, n)
			}
			api.AssertUntouched()
		})
	}
}

// TestContainerDecisionsWithoutAReply checks that a call whose connection is refused returns an
// *Error of the call with status 0 whose cause is the *url.Error.
func TestContainerDecisionsWithoutAReply(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	for _, cc := range containerCalls {
		clearEnv(t)
		c := mustNew(t, WithAPIKey(canary.Key("refused")), WithAPIURL(srv.URL),
			WithPDPURL(srv.URL), WithRetry(RetryPolicy{}))
		t.Cleanup(func() { _ = c.Close() })
		_, err := cc.call(t.Context(), c)
		e := mustError(t, err)
		if _, isURLError := errors.AsType[*url.Error](err); !isURLError || e.StatusCode != 0 ||
			e.Op != cc.op {
			t.Errorf("%s returned %v, want the *Error of %s with status 0 that wraps a "+
				"*url.Error", cc.method, err, cc.op)
		}
	}
}

// TestContainerDecisionRepliesAddNoCredential checks that the replies of the three routes, whose
// tenants and queries are the application's values, add nothing to the client's Set of
// credentials, not even a value under a credential name such as token, while the reply of an
// operation of the API still does.
func TestContainerDecisionRepliesAddNoCredential(t *testing.T) {
	const returned = "opaque-returned-value"
	echoed := []string{"opaque-echo-all-tenants", "opaque-echo-user-tenants", "opaque-echo-url"}
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	pdp.Handle(allTenantsRoute, mockpermit.Reply{
		Body: `{"allowed_tenants":[{"allow":true,"query":{"context":{"token":"` + echoed[0] +
			`"}},"tenant":{"key":"t","attributes":{"api_key":"` + echoed[0] + `"}}}]}`,
	})
	pdp.Handle(userTenantsRoute, mockpermit.Reply{
		Body: `[{"key":"t","attributes":{"password":"` + echoed[1] + `"}}]`,
	})
	pdp.Handle(checkURLRoute, mockpermit.Reply{
		Body: `{"allow":true,"query":{"user":{"key":"alice","attributes":{"secret":"` +
			echoed[2] + `"}}}}`,
	})
	api.Handle(route(opGet), mockpermit.Reply{Body: `{"key":"a","token":"` + returned + `"}`})
	c := wireClient(t, api, pdp)
	for _, cc := range containerCalls {
		if _, err := cc.call(t.Context(), c); err != nil {
			t.Fatalf("%s returned %v", cc.method, err)
		}
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

// TestMalformedAllTenantsChecks checks that AllTenantsCheck returns an *Error that matches
// ErrMalformedDecision, never a list of tenants, for each reply in 200-299 that is not a JSON
// object with one key allowed_tenants, spelled so, whose value is a list of decisions, each a JSON
// object with one key allow, whose value is true or false, and one key tenant, whose value is a
// tenant with a key that is a string: also one with a key that differs in case or is there twice,
// and a denial that is malformed, which it does not drop.
func TestMalformedAllTenantsChecks(t *testing.T) {
	key := canary.Key("tenants")
	entry := func(fields string) string { return `{"allowed_tenants":[` + fields + `]}` }
	for _, body := range []string{
		``, `null`, `[]`, `true`, `{}`, `{`, `<html>` + key + `</html>`,
		`{"allowed_tenants":null}`, `{"allowed_tenants":{}}`, `{"allowed_tenants":"a"}`,
		`{"allowed_tenants":[]} {}`, `{"allowed_tenants":[],}`, `{"Allowed_tenants":[]}`,
		`{"allowed_tenants":[],"ALLOWED_TENANTS":[]}`,
		`{"allowed_tenants":[],"allowed_tenants":[]}`,
		`{"result":{"allowed_tenants":[]}}`,
		entry(`null`), entry(`[]`), entry(`"a"`), entry(`true`), entry(`{}`),
		entry(`{"tenant":{"key":"a"}}`), entry(`{"allow":null,"tenant":{"key":"a"}}`),
		entry(`{"allow":"true","tenant":{"key":"a"}}`), entry(`{"allow":1,"tenant":{"key":"a"}}`),
		entry(`{"Allow":true,"tenant":{"key":"a"}}`),
		entry(`{"allow":true,"ALLOW":true,"tenant":{"key":"a"}}`),
		entry(`{"allow":false,"allow":true,"tenant":{"key":"a"}}`),
		entry(`{"allow":true}`), entry(`{"allow":true,"tenant":null}`),
		entry(`{"allow":true,"tenant":"a"}`), entry(`{"allow":true,"tenant":[]}`),
		entry(`{"allow":true,"tenant":{}}`), entry(`{"allow":true,"tenant":{"key":null}}`),
		entry(`{"allow":true,"tenant":{"key":1}}`), entry(`{"allow":true,"tenant":{"Key":"a"}}`),
		entry(`{"allow":true,"tenant":{"key":"a","KEY":"a"}}`),
		entry(`{"allow":true,"tenant":{"key":"a","key":"b"}}`),
		entry(`{"allow":true,"tenant":{"key":"a","attributes":"x"}}`),
		entry(`{"allow":true,"tenant":{"key":"a","attributes":[]}}`),
		entry(`{"allow":true,"Tenant":{"key":"a"}}`),
		entry(`{"allow":true,"tenant":{"key":"a"},"tenant":{"key":"b"}}`),
		entry(`{"allow":true,"tenant":{"key":"a"}},{"allow":false}`),
		entry(`{"allow":false,"tenant":{"key":"a"}},{"allow":false,"tenant":{}}`),
		entry(`{"allow":true,"tenant":{"key":"a"}},null`),
		entry(`{"allow":true,"tenant":{"key":"` + key + `","attributes":1}}`),
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(allTenantsRoute, mockpermit.Reply{Status: http.StatusOK, Body: body})
		got, err := decisionClient(t, api, pdp).AllTenantsCheck(t.Context(), alice, "read",
			document)
		if got != nil {
			t.Errorf("AllTenantsCheck of the reply %q returned %v with its error", body, got)
		}
		expectMalformed(t, err, opAllTenantsCheck, body, key)
	}
}

// TestMalformedUserTenants checks that UserTenants returns an *Error that matches
// ErrMalformedDecision, never a list of tenants, for each reply in 200-299 that is not a JSON
// array of tenants, each a JSON object with one field key, spelled so, whose value is a string,
// and attributes that are a JSON object or null: also one with a key that differs in case or is
// there twice, and one that wraps the tenants in another object.
func TestMalformedUserTenants(t *testing.T) {
	key := canary.Key("usertenants")
	for _, body := range []string{
		``, `null`, `{}`, `true`, `"a"`, `[`, `[] []`, `[],`, `<html>` + key + `</html>`,
		`[null]`, `[[]]`, `["a"]`, `[1]`, `[{}]`, `[{"key":null}]`, `[{"key":1}]`,
		`[{"key":{}}]`, `[{"Key":"a"}]`, `[{"key":"a","KEY":"a"}]`, `[{"key":"a","key":"b"}]`,
		`[{"key":"a","attributes":"x"}]`, `[{"key":"a","attributes":[]}]`,
		`[{"key":"a"},null]`, `[{"key":"a"},{}]`, `[{"key":"a"},]`,
		`{"tenants":[{"key":"a"}]}`, `{"result":[{"key":"a"}]}`,
		`[{"key":"` + key + `","attributes":1}]`,
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(userTenantsRoute, mockpermit.Reply{Status: http.StatusOK, Body: body})
		got, err := decisionClient(t, api, pdp).UserTenants(t.Context(), alice)
		if got != nil {
			t.Errorf("UserTenants of the reply %q returned %v with its error", body, got)
		}
		expectMalformed(t, err, opUserTenants, body, key)
	}
}

// TestMalformedURLDecisions checks that CheckURL returns an *Error that matches
// ErrMalformedDecision, and false, for each reply in 200-299 that is not a JSON object with one
// key allow whose value is true or false, as Check reads it.
func TestMalformedURLDecisions(t *testing.T) {
	key := canary.Key("url")
	for _, body := range []string{
		``, `null`, `[]`, `true`, `{}`, `{`, `<html>` + key + `</html>`, `{"allow":null}`,
		`{"allow":"true"}`, `{"allow":1}`, `{"result":true}`, `{"Allow":true}`,
		`{"allow":false,"ALLOW":true}`, `{"allow":true,"allow":false}`, `{"allow":true} {}`,
		`{"allow":true,}`, `{"allow":[true]}`, `{"` + key + `":true}`,
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(checkURLRoute, mockpermit.Reply{Status: http.StatusOK, Body: body})
		allowed, err := decisionClient(t, api, pdp).CheckURL(t.Context(), alice, http.MethodGet,
			reportURL, "tenant-a")
		if allowed {
			t.Errorf("CheckURL of the reply %q returned true with its error", body)
		}
		expectMalformed(t, err, opCheckURL, body, key)
	}
}

// TestMalformedContainerDecisionTexts checks the texts of the errors of malformed replies of the
// three routes, which say what is wrong without quoting the reply.
func TestMalformedContainerDecisionTexts(t *testing.T) {
	const (
		allTenants = "permit: is_allowed_all_tenants_allowed_all_tenants_post: POST " +
			"/allowed/all-tenants: status 200: permit: malformed decision: "
		userTenants = "permit: Get_User_Tenants_user_tenants_post: POST /user-tenants: " +
			"status 200: permit: malformed decision: "
		checkURL = "permit: is_allowed_url_allowed_url_post: POST /allowed_url: status 200: " +
			"permit: malformed decision: "
	)
	for _, tc := range []struct {
		call, body, want string
	}{
		{"AllTenantsCheck", `[]`, allTenants + "the reply is not a JSON object with one key " +
			"allowed_tenants whose value is a list of decisions"},
		{
			"AllTenantsCheck",
			`{"allowed_tenants":[{"allow":false,"tenant":{"key":"a"}},{"tenant":{"key":"b"}}]}`,
			allTenants + "decision 2 of the reply is not a JSON object with one key allow " +
				"whose value is true or false and one key tenant whose value is a tenant",
		},
		{"UserTenants", `{}`, userTenants + "the reply is not a JSON array of tenants"},
		{"UserTenants", `[{"key":"a"},{"key":1}]`, userTenants + "tenant 2 of the reply is not " +
			"a JSON object with one field key whose value is a string"},
		{"CheckURL", `{}`, checkURL + "the reply is not a JSON object with one key allow " +
			"whose value is true or false"},
	} {
		for _, cc := range containerCalls {
			if cc.method != tc.call {
				continue
			}
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			pdp.Handle(cc.route, mockpermit.Reply{Body: tc.body})
			_, err := cc.call(t.Context(), decisionClient(t, api, pdp))
			if err == nil || err.Error() != tc.want {
				t.Errorf("%s of the reply %s gave the error\n%v\nwant\n%s", cc.method, tc.body,
					err, tc.want)
			}
		}
	}
}

// TestInvalidContainerDecisions checks that a call of a user without a key, of a resource without
// a type or of an empty URL, which every PDP rejects, and one with the zero CheckOption, returns
// an error that matches ErrInvalidConfig and names what is wrong, and sends no request.
func TestInvalidContainerDecisions(t *testing.T) {
	const config = "permit: invalid configuration: "
	const (
		noKey = "the user key is empty; give the key of the user, as the PDP rejects a request " +
			"without one"
		noType = "the resource type is empty; give the key of the resource type, such as " +
			"document, as the PDP rejects a request without one"
		noOption = "the CheckOption is the zero value; build options with the With functions, " +
			"such as WithDecisionContext"
		noURL = "the URL is empty; give the URL of the request to check, as the PDP rejects a " +
			"request without one"
	)
	for _, tc := range []struct {
		want string
		call func(ctx context.Context, c *Client) error
	}{
		{"AllTenantsCheck: user: " + noKey, func(ctx context.Context, c *Client) error {
			_, err := c.AllTenantsCheck(ctx, User{Email: "alice@example.com"}, "read", document)
			return err
		}},
		{"AllTenantsCheck: resource: " + noType, func(ctx context.Context, c *Client) error {
			_, err := c.AllTenantsCheck(ctx, alice, "read", Resource{Tenant: "t"})
			return err
		}},
		{"AllTenantsCheck: option 2: " + noOption, func(ctx context.Context, c *Client) error {
			_, err := c.AllTenantsCheck(ctx, alice, "read", document, WithDecisionContext(nil),
				CheckOption{})
			return err
		}},
		{"UserTenants: user: " + noKey, func(ctx context.Context, c *Client) error {
			_, err := c.UserTenants(ctx, User{FirstName: "Alice"})
			return err
		}},
		{"UserTenants: option 1: " + noOption, func(ctx context.Context, c *Client) error {
			_, err := c.UserTenants(ctx, alice, CheckOption{})
			return err
		}},
		{"CheckURL: user: " + noKey, func(ctx context.Context, c *Client) error {
			_, err := c.CheckURL(ctx, User{}, http.MethodGet, reportURL, "tenant-a")
			return err
		}},
		{"CheckURL: url: " + noURL, func(ctx context.Context, c *Client) error {
			_, err := c.CheckURL(ctx, alice, http.MethodGet, "", "tenant-a")
			return err
		}},
		{"CheckURL: option 2: " + noOption, func(ctx context.Context, c *Client) error {
			_, err := c.CheckURL(ctx, alice, http.MethodGet, reportURL, "tenant-a",
				WithDecisionContext(map[string]any{"a": 1}), CheckOption{})
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

// TestContainerDecisionBodyThatIsNotJSON checks that a call whose body holds a value that JSON
// cannot hold, in the context or in the attributes of the user, returns an error that matches
// ErrInvalidConfig, and sends no request.
func TestContainerDecisionBodyThatIsNotJSON(t *testing.T) {
	nan := WithDecisionContext(map[string]any{"ratio": math.NaN()})
	odd := User{Key: "alice", Attributes: map[string]any{"ratio": math.Inf(-1)}}
	for _, cc := range []containerCall{
		{
			method: "AllTenantsCheck", op: opAllTenantsCheck,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.AllTenantsCheck(ctx, alice, "read", document, nan)
			},
		},
		{
			method: "UserTenants", op: opUserTenants,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.UserTenants(ctx, alice, nan)
			},
		},
		{
			method: "CheckURL", op: opCheckURL,
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.CheckURL(ctx, odd, http.MethodGet, reportURL, "tenant-a")
			},
		},
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		_, err := cc.call(t.Context(), decisionClient(t, api, pdp))
		want := "permit: invalid configuration: " + cc.op + ": the request body cannot be " +
			"written as JSON"
		if !errors.Is(err, ErrInvalidConfig) || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("%s returned %v, want an error that starts with %q", cc.method, err, want)
		}
		pdp.AssertUntouched()
	}
}

// TestContainerDecisionsAfterClose checks that the three methods return an error that matches
// ErrClosed after Close, and send no request.
func TestContainerDecisionsAfterClose(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	c := decisionClient(t, api, pdp)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	for _, cc := range containerCalls {
		if _, err := cc.call(t.Context(), c); !errors.Is(err, ErrClosed) {
			t.Errorf("%s after Close returned %v, want ErrClosed", cc.method, err)
		}
	}
	pdp.AssertUntouched()
	api.AssertUntouched()
}

// FuzzContainerDecisionReplies checks the readers of the replies of the tenants on any body: each
// returns a list of tenants or an *Error that matches ErrMalformedDecision, never both and never
// a panic. The tenants that readAllowedTenants returns are at most the decisions of the reply.
func FuzzContainerDecisionReplies(f *testing.F) {
	for _, body := range []string{
		replyFixtureOf(f, allTenantsFixture), replyFixtureOf(f, userTenantsFixture),
		`{"allowed_tenants":[]}`, `[]`, `null`, `{"allowed_tenants":[{"allow":true}]}`,
		`[{"Key":"a"}]`, `{"allowed_tenants":[{"allow":false,"tenant":{"key":"a","key":"b"}}]}`,
	} {
		f.Add([]byte(body))
	}
	allTenantsOp, userTenantsOp := defaultOp(f, opAllTenantsCheck), defaultOp(f, opUserTenants)
	f.Fuzz(func(t *testing.T, body []byte) {
		reply := transport.Reply{Status: http.StatusOK, Body: body}
		allowed, err := readAllowedTenants(allTenantsOp, reply)
		checkFuzzResult(t, "readAllowedTenants", body, allowed != nil, err)
		var decisions struct {
			AllowedTenants []json.RawMessage `json:"allowed_tenants"`
		}
		if err == nil && (json.Unmarshal(body, &decisions) != nil ||
			len(allowed) > len(decisions.AllowedTenants)) {
			t.Errorf("readAllowedTenants of %q returned %d tenants of %d decisions", body,
				len(allowed), len(decisions.AllowedTenants))
		}
		tenants, err := readTenants(userTenantsOp, reply)
		checkFuzzResult(t, "readTenants", body, tenants != nil, err)
		var items []json.RawMessage
		if err == nil && (json.Unmarshal(body, &items) != nil || len(items) != len(tenants)) {
			t.Errorf("readTenants of %q returned %d tenants of %d items", body, len(tenants),
				len(items))
		}
	})
}
