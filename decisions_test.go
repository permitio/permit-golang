package permit

import (
	"context"
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
	"github.com/permitio/permit-golang/v2/internal/wire"
)

// The routes of the PDP's decisions, as mockpermit.Server.Handle takes them.
const (
	checkRoute     = "POST /allowed"
	bulkCheckRoute = "POST /allowed/bulk"
)

// decisionClient returns a client whose URLs point at api and pdp, with mockpermit's API key and
// opts, and closes it when the test ends.
func decisionClient(t *testing.T, api, pdp *mockpermit.Server, opts ...Option) *Client {
	t.Helper()
	clearEnv(t)
	base := []Option{WithAPIKey(mockpermit.APIKey), WithAPIURL(api.URL), WithPDPURL(pdp.URL)}
	c := mustNew(t, append(base, opts...)...)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// onlyRequest returns the one request that server got, and fails the test when it got another
// number of requests.
func onlyRequest(t *testing.T, server *mockpermit.Server) mockpermit.Request {
	t.Helper()
	requests := server.Requests()
	if len(requests) != 1 {
		t.Fatalf("the PDP got %d requests, want 1", len(requests))
	}
	return requests[0]
}

// The user, the action and the resource of the tests that do not test them.
var (
	alice    = User{Key: "alice"}
	document = Resource{Type: "document", Key: "2026-report"}
)

// TestCheckBodies checks the body that Check sends for each kind of input: every field of the
// user and of the resource, values of every JSON type in the maps of attributes and of context,
// a check of a resource type, a resource without a tenant, which it sends in the tenant default,
// and empty maps, which it leaves out rather than send {} or null. Nothing reaches the API.
func TestCheckBodies(t *testing.T) {
	// typeCheck is the body of a check whether alice may read the resource type document.
	const typeCheck = `{"user":{"key":"alice"},"action":"read",` +
		`"resource":{"type":"document","tenant":"default"}}`
	for _, tc := range []struct {
		name     string
		user     User
		resource Resource
		opts     []CheckOption
		want     string
	}{
		{
			name:     "a resource type",
			user:     alice,
			resource: Resource{Type: "document"},
			want:     typeCheck,
		},
		{
			name:     "an instance without a tenant",
			user:     alice,
			resource: Resource{Type: "document", Key: "2026-report"},
			want: `{"user":{"key":"alice"},"action":"read","resource":{"type":"document",` +
				`"key":"2026-report","tenant":"default"}}`,
		},
		{
			name:     "an instance of a tenant",
			user:     alice,
			resource: Resource{Type: "document", Key: fixtures.UUIDKey, Tenant: "tenant-a"},
			want: `{"user":{"key":"alice"},"action":"read","resource":{"type":"document",` +
				`"key":"` + fixtures.UUIDKey + `","tenant":"tenant-a"}}`,
		},
		{
			name: "every field, with typed values",
			user: User{
				Key: fixtures.TypedKey, FirstName: "Alice", LastName: "Liddell",
				Email: "alice@example.com",
				Attributes: map[string]any{
					"age": 30, "vip": true, "score": 1.5, "id": fixtures.TwoPow53Plus1,
					"none": nil, "tags": []any{"a", 1, false},
					"address": map[string]any{"city": "Paris", "zip": nil},
				},
			},
			resource: Resource{
				Type: "document", Key: "2026-report", Tenant: "tenant-a",
				Attributes: map[string]any{"pages": int64(12), "draft": false},
				Context:    map[string]any{"source": fixtures.HTMLSpecial},
			},
			opts: []CheckOption{WithDecisionContext(map[string]any{
				"now": int64(1730000000), "ratio": 0.25, "ok": true, "nothing": nil,
				"nested": map[string]any{"list": []any{1, "x", map[string]any{"deep": 2}}},
			})},
			want: `{
				"user": {
					"key": "` + fixtures.TypedKey + `", "first_name": "Alice",
					"last_name": "Liddell", "email": "alice@example.com",
					"attributes": {
						"age": 30, "vip": true, "score": 1.5, "id": 9007199254740993,
						"none": null, "tags": ["a", 1, false],
						"address": {"city": "Paris", "zip": null}
					}
				},
				"action": "read",
				"resource": {
					"type": "document", "key": "2026-report", "tenant": "tenant-a",
					"attributes": {"pages": 12, "draft": false},
					"context": {"source": "<>&"}
				},
				"context": {
					"now": 1730000000, "ratio": 0.25, "ok": true, "nothing": null,
					"nested": {"list": [1, "x", {"deep": 2}]}
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
			want: typeCheck,
		},
		{
			name:     "the last context",
			user:     alice,
			resource: Resource{Type: "document"},
			opts: []CheckOption{
				WithDecisionContext(map[string]any{"a": 1}),
				WithDecisionContext(map[string]any{"b": 2}),
			},
			want: `{"user":{"key":"alice"},"action":"read",` +
				`"resource":{"type":"document","tenant":"default"},"context":{"b":2}}`,
		},
		{
			name:     "a nil context last",
			user:     alice,
			resource: Resource{Type: "document"},
			opts: []CheckOption{
				WithDecisionContext(map[string]any{"a": 1}), WithDecisionContext(nil),
			},
			want: typeCheck,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			pdp.Handle(checkRoute, mockpermit.Reply{Body: `{"allow":true}`})
			c := decisionClient(t, api, pdp)
			allowed, err := c.Check(t.Context(), tc.user, "read", tc.resource, tc.opts...)
			if err != nil || !allowed {
				t.Fatalf("Check returned %t, %v; want true, nil", allowed, err)
			}
			onlyRequest(t, pdp).ExpectJSON(t, tc.want)
			api.AssertUntouched()
		})
	}
}

// TestCheckReturnsTheDecision checks that Check returns the decision of the reply, false too.
func TestCheckReturnsTheDecision(t *testing.T) {
	for body, want := range map[string]bool{`{"allow":true}`: true, `{"allow":false}`: false} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(checkRoute, mockpermit.Reply{Body: body})
		allowed, err := decisionClient(t, api, pdp).Check(t.Context(), alice, "read", document)
		if err != nil || allowed != want {
			t.Errorf("Check of the reply %s returned %t, %v; want %t, nil", body, allowed, err,
				want)
		}
	}
}

// TestBulkCheckSendsOneRequestInOrder checks that BulkCheck sends its checks of three tenants in
// one request, in their order, each with its own context, and returns the decisions in that
// order.
func TestBulkCheckSendsOneRequestInOrder(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	pdp.Handle(bulkCheckRoute, mockpermit.Reply{
		Body: `{"allow":[{"allow":true},{"allow":false},{"allow":false},{"allow":true}]}`,
	})
	checks := []CheckRequest{
		{User: alice, Action: "read", Resource: Resource{Type: "document", Tenant: "t3"}},
		{
			User: User{Key: "bob"}, Action: "edit",
			Resource: Resource{Type: "document", Key: "a", Tenant: "t1"},
			Context:  map[string]any{"n": 1},
		},
		{
			User: alice, Action: "read", Resource: Resource{Type: "folder", Tenant: "t2"},
			Context: map[string]any{"n": 2.5},
		},
		{User: alice, Action: "delete", Resource: Resource{Type: "document", Tenant: "t1"}},
	}
	got, err := decisionClient(t, api, pdp).BulkCheck(t.Context(), checks)
	if err != nil {
		t.Fatalf("BulkCheck returned %v", err)
	}
	if diff := cmp.Diff([]bool{true, false, false, true}, got); diff != "" {
		t.Errorf("BulkCheck returned decisions that differ (-want +got):\n%s", diff)
	}
	onlyRequest(t, pdp).ExpectJSON(t, `[
		{"user":{"key":"alice"},"action":"read","resource":{"type":"document","tenant":"t3"}},
		{"user":{"key":"bob"},"action":"edit",
			"resource":{"type":"document","key":"a","tenant":"t1"},"context":{"n":1}},
		{"user":{"key":"alice"},"action":"read","resource":{"type":"folder","tenant":"t2"},
			"context":{"n":2.5}},
		{"user":{"key":"alice"},"action":"delete","resource":{"type":"document","tenant":"t1"}}
	]`)
	api.AssertUntouched()
}

// TestFilterObjectsKeepsEachResource checks that FilterObjects sends one check for each resource,
// with the resource's own key, tenant, attributes and context and the context of the call, and
// returns the resources that the PDP allows as they were given.
func TestFilterObjectsKeepsEachResource(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	pdp.Handle(bulkCheckRoute, mockpermit.Reply{Body: `{"allow":[{"allow":false},{"allow":true}]}`})
	resources := []Resource{
		{
			Type: "document", Key: "a", Tenant: "t1", Attributes: map[string]any{"n": 1},
			Context: map[string]any{"from": "a"},
		},
		{Type: "document", Key: "b", Tenant: "t2", Attributes: map[string]any{}},
	}
	got, err := decisionClient(t, api, pdp).FilterObjects(t.Context(), alice, "read", resources,
		WithDecisionContext(map[string]any{"now": 1}))
	if err != nil {
		t.Fatalf("FilterObjects returned %v", err)
	}
	if diff := cmp.Diff(resources[1:], got); diff != "" {
		t.Errorf("FilterObjects returned resources that differ (-want +got):\n%s", diff)
	}
	onlyRequest(t, pdp).ExpectJSON(t, `[
		{"user":{"key":"alice"},"action":"read","resource":{"type":"document","key":"a",
			"tenant":"t1","attributes":{"n":1},"context":{"from":"a"}},"context":{"now":1}},
		{"user":{"key":"alice"},"action":"read",
			"resource":{"type":"document","key":"b","tenant":"t2"},"context":{"now":1}}
	]`)
	api.AssertUntouched()
}

// TestFilterObjectsWhenNoneIsAllowed checks that FilterObjects returns no resource, without an
// error, when the PDP denies each.
func TestFilterObjectsWhenNoneIsAllowed(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	pdp.Handle(bulkCheckRoute, mockpermit.Reply{Body: `{"allow":[{"allow":false}]}`})
	got, err := decisionClient(t, api, pdp).FilterObjects(t.Context(), alice, "read",
		[]Resource{document})
	if err != nil || len(got) != 0 {
		t.Errorf("FilterObjects returned %v, %v; want no resource and no error", got, err)
	}
}

// TestDecisionsSendTheDefaultTenant checks that each method sends a resource without a tenant in
// the tenant default and a resource with a tenant in its own, and that FilterObjects returns the
// resources as they were given, the one without a tenant too.
func TestDecisionsSendTheDefaultTenant(t *testing.T) {
	resources := []Resource{
		{Type: "document", Key: "a"},
		{Type: "document", Key: "b", Tenant: "tenant-b"},
	}
	sent := []string{
		`{"user":{"key":"alice"},"action":"read",` +
			`"resource":{"type":"document","key":"a","tenant":"default"}}`,
		`{"user":{"key":"alice"},"action":"read",` +
			`"resource":{"type":"document","key":"b","tenant":"tenant-b"}}`,
	}
	t.Run("Check", func(t *testing.T) {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		reply := mockpermit.Reply{Body: `{"allow":true}`}
		pdp.Handle(checkRoute, reply, reply)
		c := decisionClient(t, api, pdp)
		for _, resource := range resources {
			if _, err := c.Check(t.Context(), alice, "read", resource); err != nil {
				t.Fatalf("Check of %+v returned %v", resource, err)
			}
		}
		requests := pdp.Requests()
		if len(requests) != len(sent) {
			t.Fatalf("the PDP got %d requests, want %d", len(requests), len(sent))
		}
		for i, request := range requests {
			request.ExpectJSON(t, sent[i])
		}
	})
	t.Run("BulkCheck", func(t *testing.T) {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(bulkCheckRoute, mockpermit.Reply{Body: `{"allow":[{"allow":true},` +
			`{"allow":true}]}`})
		_, err := decisionClient(t, api, pdp).BulkCheck(t.Context(), []CheckRequest{
			{User: alice, Action: "read", Resource: resources[0]},
			{User: alice, Action: "read", Resource: resources[1]},
		})
		if err != nil {
			t.Fatalf("BulkCheck returned %v", err)
		}
		onlyRequest(t, pdp).ExpectJSON(t, "["+strings.Join(sent, ",")+"]")
	})
	t.Run("FilterObjects", func(t *testing.T) {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(bulkCheckRoute, mockpermit.Reply{Body: `{"allow":[{"allow":true},` +
			`{"allow":true}]}`})
		got, err := decisionClient(t, api, pdp).FilterObjects(t.Context(), alice, "read",
			resources)
		if err != nil {
			t.Fatalf("FilterObjects returned %v", err)
		}
		if diff := cmp.Diff(resources, got); diff != "" {
			t.Errorf("FilterObjects returned resources that differ (-want +got):\n%s", diff)
		}
		onlyRequest(t, pdp).ExpectJSON(t, "["+strings.Join(sent, ",")+"]")
	})
}

// decisionCall is a call of a method of decisions with a client, as the tests make it for each
// method.
type decisionCall struct {
	// method names the method.
	method string

	// op is the ID of the operation that the call sends.
	op string

	// route is the route of the operation, as mockpermit.Server.Handle takes it.
	route string

	// call makes the call with c, and returns its error.
	call func(ctx context.Context, c *Client) error
}

// decisionCalls are a call of each method of decisions, of one check, whose reply is the
// reply to one check or to a list of one check.
var decisionCalls = []decisionCall{
	{"Check", opCheck, checkRoute, func(ctx context.Context, c *Client) error {
		_, err := c.Check(ctx, alice, "read", document)
		return err
	}},
	{"BulkCheck", opBulkCheck, bulkCheckRoute, func(ctx context.Context, c *Client) error {
		_, err := c.BulkCheck(ctx, []CheckRequest{
			{User: alice, Action: "read", Resource: document},
		})
		return err
	}},
	{"FilterObjects", opBulkCheck, bulkCheckRoute, func(ctx context.Context, c *Client) error {
		_, err := c.FilterObjects(ctx, alice, "read", []Resource{document})
		return err
	}},
}

// TestDecisionErrorReplies checks that a reply with an error status is an *Error of the decision,
// with the status and the body of the reply, for each method: one that ends the call, such as 400
// or 421, and a 429 or a 503 after the retries. Nothing reaches the API.
func TestDecisionErrorReplies(t *testing.T) {
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
		{http.StatusMisdirectedRequest, `{"detail":"update the SDK"}`, "update the SDK"},
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
		for _, dc := range decisionCalls {
			t.Run(dc.method+"/"+http.StatusText(tc.status), func(t *testing.T) {
				api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
				reply := mockpermit.Reply{
					Status: tc.status, Body: tc.body,
					Header: http.Header{"Retry-After": {"0"}},
				}
				replies := []mockpermit.Reply{reply}
				if retried {
					replies = append(replies, reply)
				}
				pdp.Handle(dc.route, replies...)
				c := decisionClient(t, api, pdp, WithRetry(RetryPolicy{MaxRetries: 1}))
				err := dc.call(t.Context(), c)
				want := &Error{
					Op:         dc.op,
					Method:     http.MethodPost,
					Route:      strings.TrimPrefix(dc.route, "POST "),
					StatusCode: tc.status,
					Detail:     tc.detail,
					Body:       []byte(tc.body),
				}
				if diff := cmp.Diff(want, err); diff != "" {
					t.Errorf("%s returned an error that differs (-want +got):\n%s", dc.method,
						diff)
				}
				if n := len(pdp.Requests()); n != len(replies) {
					t.Errorf("%s sent %d requests for status %d, want %d", dc.method, n,
						tc.status, len(replies))
				}
				api.AssertUntouched()
			})
		}
	}
}

// TestDecisionsAreRetried checks that a decision whose first reply is a 503 is sent again, as
// the PDP's decisions change nothing, and returns the decision of the second reply.
func TestDecisionsAreRetried(t *testing.T) {
	for _, dc := range decisionCalls {
		t.Run(dc.method, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			body := `{"allow":true}`
			if dc.route == bulkCheckRoute {
				body = `{"allow":[{"allow":true}]}`
			}
			pdp.Handle(dc.route, mockpermit.Reply{
				Status: http.StatusServiceUnavailable, Body: `{"detail":"busy"}`,
				Header: http.Header{"Retry-After": {"0"}},
			}, mockpermit.Reply{Body: body})
			if err := dc.call(t.Context(), decisionClient(t, api, pdp)); err != nil {
				t.Errorf("%s returned %v", dc.method, err)
			}
			if n := len(pdp.Requests()); n != 2 {
				t.Errorf("%s sent %d requests, want 2", dc.method, n)
			}
			api.AssertUntouched()
		})
	}
}

// TestDecisionWithoutAReply checks that a decision whose connection is refused returns an *Error
// of the decision with status 0 whose cause is the *url.Error.
func TestDecisionWithoutAReply(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	for _, dc := range decisionCalls {
		clearEnv(t)
		c := mustNew(t, WithAPIKey(canary.Key("refused")), WithAPIURL(srv.URL),
			WithPDPURL(srv.URL), WithRetry(RetryPolicy{}))
		t.Cleanup(func() { _ = c.Close() })
		err := dc.call(t.Context(), c)
		e := mustError(t, err)
		if _, isURLError := errors.AsType[*url.Error](err); !isURLError || e.StatusCode != 0 ||
			e.Op != dc.op {
			t.Errorf("%s returned %v, want the *Error of %s with status 0 that wraps a "+
				"*url.Error", dc.method, err, dc.op)
		}
	}
}

// TestDecisionRepliesAddNoCredential checks that the reply of a decision, whose query can echo
// the check, adds nothing to the client's Set of credentials, not even a value under a
// credential name such as token, while the reply of an operation of the API still does.
func TestDecisionRepliesAddNoCredential(t *testing.T) {
	const returned = "opaque-returned-value"
	echoed := []string{"opaque-echo-check", "opaque-echo-bulk", "opaque-echo-filter"}
	query := func(value string) string {
		return `{"allow":true,"query":{"context":{"token":"` + value + `"},` +
			`"user":{"key":"alice","attributes":{"password":"` + value + `"}}}}`
	}
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	pdp.Handle(checkRoute, mockpermit.Reply{Body: query(echoed[0])})
	pdp.Handle(bulkCheckRoute,
		mockpermit.Reply{Body: `{"allow":[` + query(echoed[1]) + `]}`},
		mockpermit.Reply{Body: `{"allow":[` + query(echoed[2]) + `]}`})
	api.Handle(route(opGet), mockpermit.Reply{Body: `{"key":"a","token":"` + returned + `"}`})
	c := wireClient(t, api, pdp)
	_, err := c.Check(t.Context(), alice, "read", document,
		WithDecisionContext(map[string]any{"token": echoed[0]}))
	if err != nil {
		t.Fatalf("Check returned %v", err)
	}
	_, err = c.BulkCheck(t.Context(), []CheckRequest{{
		User: alice, Action: "read", Resource: document,
		Context: map[string]any{"token": echoed[1]},
	}})
	if err != nil {
		t.Fatalf("BulkCheck returned %v", err)
	}
	_, err = c.FilterObjects(t.Context(), alice, "read", []Resource{document},
		WithDecisionContext(map[string]any{"token": echoed[2]}))
	if err != nil {
		t.Fatalf("FilterObjects returned %v", err)
	}
	for _, value := range echoed {
		if c.secrets.Contains(value) {
			t.Errorf("the reply of a decision added %s, which it echoed, to the client's Set",
				value)
		}
	}
	if _, err = do[testObject](t.Context(), c, get("a")); err != nil {
		t.Fatalf("the call of the API returned %v", err)
	}
	if !c.secrets.Contains(returned) {
		t.Error("the reply of an operation of the API did not add its token to the client's Set")
	}
}

// TestMalformedDecisions checks that Check returns an *Error that matches ErrMalformedDecision,
// with the status of the reply and nothing of its body, for each reply in 200-299 that is not a
// JSON object with one key allow whose value is true or false: also one whose allow differs in
// case, such as Allow, or whose allow is there twice. It checks that Check ignores the other
// fields of a reply.
func TestMalformedDecisions(t *testing.T) {
	key := canary.Key("decision")
	for _, body := range []string{
		`{}`, `{"allow":null}`, `{"allow":"true"}`, `{"allow":"false"}`, `{"allow":1}`,
		`{"allow":[true]}`, `{"allow":{}}`, `{"result":true}`, `[true]`, `true`, `null`, ``,
		`{"allow":true`, `{"allow":true} {}`, `<html>` + key + `</html>`,
		`{"Allow":true}`, `{"ALLOW":false}`, `{"allow":false,"Allow":true}`,
		`{"aLLow":true,"allow":false}`, `{"allow":true,"allow":true}`,
		`{"allow":false,"allow":true}`, `{"allow":True}`, `{"allow":true,}`, `{"allow":true]`,
		`{1:true}`, `{"allow"}`, `{"a":1 "allow":true}`,
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(checkRoute, mockpermit.Reply{Status: http.StatusOK, Body: body})
		allowed, err := decisionClient(t, api, pdp).Check(t.Context(), alice, "read", document)
		e := mustError(t, err)
		if allowed || !errors.Is(err, ErrMalformedDecision) || e.StatusCode != http.StatusOK ||
			e.Op != opCheck || e.Route != "/allowed" || e.Body != nil ||
			strings.Contains(err.Error(), key) {
			t.Errorf("Check of the reply %q returned %t, %v; want false and a *Error with "+
				"status 200 that matches ErrMalformedDecision and holds nothing of the body",
				body, allowed, err)
		}
	}
	for body, want := range map[string]bool{
		`{"allow":true,"query":{"a":1},"debug":null,"result":false,"new":[1]}`: true,
		`{"result":true,"allow":false}`:                                        false,
		` { "allow" : true , "allowed" : false } `:                             true,
		`{"\u0061llow":false,"query":{"allow":true,"Allow":true}}`:             false,
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(checkRoute, mockpermit.Reply{Body: body})
		allowed, err := decisionClient(t, api, pdp).Check(t.Context(), alice, "read", document)
		if err != nil || allowed != want {
			t.Errorf("Check of the reply %s returned %t, %v; want %t, nil", body, allowed, err,
				want)
		}
	}
}

// TestMalformedBulkDecisions checks that BulkCheck and FilterObjects return an *Error that matches
// ErrMalformedDecision for each reply in 200-299 to two checks that does not hold a decision, true
// or false, for each check, under one key allow: also one whose allow, in the reply or in a
// decision, differs in case or is there twice. It checks that they ignore the other fields of a
// reply.
func TestMalformedBulkDecisions(t *testing.T) {
	two := []CheckRequest{
		{User: alice, Action: "read", Resource: document},
		{User: alice, Action: "read", Resource: Resource{Type: "document", Key: "b"}},
	}
	calls := map[string]func(c *Client) error{
		"BulkCheck": func(c *Client) error {
			_, err := c.BulkCheck(t.Context(), two)
			return err
		},
		"FilterObjects": func(c *Client) error {
			_, err := c.FilterObjects(t.Context(), alice, "read",
				[]Resource{two[0].Resource, two[1].Resource})
			return err
		},
	}
	for _, body := range []string{
		`{}`, `{"allow":null}`, `{"allow":[]}`, `{"allow":[{"allow":true}]}`,
		`{"allow":[{"allow":true},{"allow":true},{"allow":true}]}`,
		`{"allow":[{"allow":true},{}]}`, `{"allow":[{"allow":null},{"allow":true}]}`,
		`{"allow":[{"allow":true},{"allow":"false"}]}`, `{"allow":[true,false]}`,
		`{"allow":[{"allow":true},null]}`, `{"allow":{"0":{"allow":true}}}`,
		`[{"allow":true},{"allow":false}]`, `{"allow":true}`, ``, `<html></html>`,
		`{"Allow":[{"allow":true},{"allow":true}]}`,
		`{"allow":[{"allow":true},{"allow":true}],"ALLOW":[]}`,
		`{"allow":[{"allow":true},{"allow":true}],"allow":[{"allow":true},{"allow":true}]}`,
		`{"allow":[{"Allow":true},{"allow":true}]}`,
		`{"allow":[{"allow":true},{"allow":false,"allow":true}]}`,
		`{"allow":[{"allow":true},{"allow":true,"aLLow":false}]}`,
		`{"allow":[{"allow":true},{"allow":true}]} []`,
	} {
		for name, call := range calls {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			pdp.Handle(bulkCheckRoute, mockpermit.Reply{Body: body})
			err := call(decisionClient(t, api, pdp))
			e := mustError(t, err)
			if !errors.Is(err, ErrMalformedDecision) || e.StatusCode != http.StatusOK ||
				e.Op != opBulkCheck || e.Body != nil {
				t.Errorf("%s of the reply %q returned %v; want a *Error with status 200 that "+
					"matches ErrMalformedDecision", name, body, err)
			}
		}
	}
	for _, body := range []string{
		`{"allow":[{"allow":false,"query":{}},{"allow":true,"debug":{"x":1}}],"new":1}`,
		` { "allow" : [ { "allow" : false } , {"query":{"Allow":true},"allow":true} ] } `,
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(bulkCheckRoute, mockpermit.Reply{Body: body})
		got, err := decisionClient(t, api, pdp).BulkCheck(t.Context(), two)
		if err != nil || !cmp.Equal(got, []bool{false, true}) {
			t.Errorf("BulkCheck of the reply %s returned %v, %v; want [false true]", body, got,
				err)
		}
	}
}

// TestMalformedDecisionTexts checks the texts of the errors of malformed decisions, which say
// what is wrong without quoting the reply.
func TestMalformedDecisionTexts(t *testing.T) {
	for _, tc := range []struct {
		route, body, want string
	}{
		{checkRoute, `{}`, "permit: is_allowed_allowed_post: POST /allowed: status 200: " +
			"permit: malformed decision: the reply is not a JSON object with one key allow whose " +
			"value is true or false"},
		{bulkCheckRoute, `[]`, "permit: is_allowed_bulk_allowed_bulk_post: POST /allowed/bulk: " +
			"status 200: permit: malformed decision: the reply is not a JSON object with one key " +
			"allow whose value is a list of decisions, each a JSON object with one key allow " +
			"whose value is true or false"},
		{bulkCheckRoute, `{"allow":[{"allow":true}]}`, "permit: " +
			"is_allowed_bulk_allowed_bulk_post: POST /allowed/bulk: status 200: permit: " +
			"malformed decision: the number of decisions in the reply, 1, is not the number of " +
			"checks, 2"},
		{bulkCheckRoute, `{"allow":[{"allow":true},{}]}`, "permit: " +
			"is_allowed_bulk_allowed_bulk_post: POST /allowed/bulk: status 200: permit: " +
			"malformed decision: decision 2 of the reply is not a JSON object with one key " +
			"allow whose value is true or false"},
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		pdp.Handle(tc.route, mockpermit.Reply{Body: tc.body})
		c := decisionClient(t, api, pdp)
		var err error
		if tc.route == checkRoute {
			_, err = c.Check(t.Context(), alice, "read", document)
		} else {
			_, err = c.BulkCheck(t.Context(), []CheckRequest{
				{User: alice, Action: "read", Resource: document},
				{User: alice, Action: "edit", Resource: document},
			})
		}
		if err == nil || err.Error() != tc.want {
			t.Errorf("the reply %s gave the error\n%v\nwant\n%s", tc.body, err, tc.want)
		}
	}
}

// TestInvalidDecisions checks that a decision of a user without a key or a resource without a
// type, which every PDP rejects, and one with the zero CheckOption, returns an error that matches
// ErrInvalidConfig and names what is wrong, and sends no request.
func TestInvalidDecisions(t *testing.T) {
	const config = "permit: invalid configuration: "
	const (
		noKey = "the user key is empty; give the key of the user, as the PDP rejects a check " +
			"without one"
		noType = "the resource type is empty; give the key of the resource type, such as " +
			"document, as the PDP rejects a check without one"
		noOption = "the CheckOption is the zero value; build options with the With functions, " +
			"such as WithDecisionContext"
	)
	ok := CheckRequest{User: alice, Action: "read", Resource: document}
	for _, tc := range []struct {
		want string
		call func(ctx context.Context, c *Client) error
	}{
		{"Check: user: " + noKey, func(ctx context.Context, c *Client) error {
			_, err := c.Check(ctx, User{FirstName: "Alice"}, "read", document)
			return err
		}},
		{"Check: resource: " + noType, func(ctx context.Context, c *Client) error {
			_, err := c.Check(ctx, alice, "read", Resource{Key: "2026-report"})
			return err
		}},
		{"Check: option 2: " + noOption, func(ctx context.Context, c *Client) error {
			_, err := c.Check(ctx, alice, "read", document, WithDecisionContext(nil),
				CheckOption{})
			return err
		}},
		{"BulkCheck: checks[1].user: " + noKey, func(ctx context.Context, c *Client) error {
			_, err := c.BulkCheck(ctx, []CheckRequest{ok, {Action: "read", Resource: document}})
			return err
		}},
		{"BulkCheck: checks[2].resource: " + noType, func(ctx context.Context, c *Client) error {
			_, err := c.BulkCheck(ctx, []CheckRequest{ok, ok, {User: alice, Action: "read"}})
			return err
		}},
		{"FilterObjects: user: " + noKey, func(ctx context.Context, c *Client) error {
			_, err := c.FilterObjects(ctx, User{}, "read", []Resource{document})
			return err
		}},
		{"FilterObjects: resources[1]: " + noType, func(ctx context.Context, c *Client) error {
			_, err := c.FilterObjects(ctx, alice, "read", []Resource{document, {Tenant: "t"}})
			return err
		}},
		{"FilterObjects: option 1: " + noOption, func(ctx context.Context, c *Client) error {
			_, err := c.FilterObjects(ctx, alice, "read", []Resource{document}, CheckOption{})
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

// TestDecisionBodyThatIsNotJSON checks that a check whose context holds a value that JSON cannot
// hold returns an error that matches ErrInvalidConfig, and sends no request.
func TestDecisionBodyThatIsNotJSON(t *testing.T) {
	nan := WithDecisionContext(map[string]any{"ratio": math.NaN()})
	for _, dc := range []decisionCall{
		{"Check", opCheck, checkRoute, func(ctx context.Context, c *Client) error {
			_, err := c.Check(ctx, alice, "read", document, nan)
			return err
		}},
		{"BulkCheck", opBulkCheck, bulkCheckRoute, func(ctx context.Context, c *Client) error {
			_, err := c.BulkCheck(ctx, []CheckRequest{{
				User: alice, Action: "read", Resource: document,
				Context: map[string]any{"ratio": math.Inf(1)},
			}})
			return err
		}},
		{"FilterObjects", opBulkCheck, bulkCheckRoute, func(ctx context.Context, c *Client) error {
			_, err := c.FilterObjects(ctx, alice, "read", []Resource{document}, nan)
			return err
		}},
	} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		err := dc.call(t.Context(), decisionClient(t, api, pdp))
		want := "permit: invalid configuration: " + dc.op + ": the request body cannot be " +
			"written as JSON"
		if !errors.Is(err, ErrInvalidConfig) || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("%s returned %v, want an error that starts with %q", dc.method, err, want)
		}
		pdp.AssertUntouched()
	}
}

// TestNoChecksSendNothing checks that BulkCheck and FilterObjects of no checks send no request
// and return an empty list, and after Close an error that matches ErrClosed, as Check does.
func TestNoChecksSendNothing(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	c := decisionClient(t, api, pdp)
	for _, checks := range [][]CheckRequest{nil, {}} {
		if got, err := c.BulkCheck(t.Context(), checks); err != nil || got == nil ||
			len(got) != 0 {
			t.Errorf("BulkCheck(%#v) returned %#v, %v; want an empty list", checks, got, err)
		}
	}
	for _, resources := range [][]Resource{nil, {}} {
		got, err := c.FilterObjects(t.Context(), alice, "read", resources)
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("FilterObjects(%#v) returned %#v, %v; want an empty list", resources, got,
				err)
		}
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	_, checkErr := c.Check(t.Context(), alice, "read", document)
	_, bulkErr := c.BulkCheck(t.Context(), nil)
	_, filterErr := c.FilterObjects(t.Context(), alice, "read", nil)
	for name, err := range map[string]error{
		"Check": checkErr, "BulkCheck": bulkErr, "FilterObjects": filterErr,
	} {
		if !errors.Is(err, ErrClosed) {
			t.Errorf("%s after Close returned %v, want ErrClosed", name, err)
		}
	}
	pdp.AssertUntouched()
	api.AssertUntouched()
}

// fuzzFields returns the n fields of s, a fuzz input, separated by NUL bytes: those that s holds,
// each made valid UTF-8, which encoding/json writes as it is, then empty ones.
func fuzzFields(s string, n int) []string {
	fields := strings.SplitN(s, "\x00", n)
	for i, field := range fields {
		fields[i] = strings.ToValidUTF8(field, "�")
	}
	for len(fields) < n {
		fields = append(fields, "")
	}
	return fields
}

// fuzzObject returns the JSON object that s holds, with each number as a json.Number, or nil when
// s holds none.
func fuzzObject(s string) map[string]any {
	var m map[string]any
	if wire.Decode([]byte(s), &m) != nil {
		return nil
	}
	return m
}

// putText sets m[name] to text, when text is not empty.
func putText(m map[string]any, name, text string) {
	if text != "" {
		m[name] = text
	}
}

// putObject sets m[name] to object, when object is not empty.
func putObject(m map[string]any, name string, object map[string]any) {
	if len(object) > 0 {
		m[name] = object
	}
}

// FuzzCheckBody checks that the body of a check holds each input as it was given, and nothing
// else: the key, the names and the email of the user, the action, and the type and the key of the
// resource, each as given, also when empty for the key of the user, the action and the type,
// which are required, and left out when empty for the others; the tenant of the resource as
// given, or default when empty; and the maps of attributes and of context, with JSON values of
// any type, as given, or left out when empty. The strings of texts and the JSON objects of
// objects are separated by NUL bytes.
func FuzzCheckBody(f *testing.F) {
	f.Add("alice\x00Alice\x00Liddell\x00alice@example.com\x00read\x00document\x002026-report\x00"+
		"tenant-a", []byte(`{"age":30,"big":9007199254740993}`+"\x00"+`{"pages":1.5e3}`+"\x00"+
		`{"from":"<>&"}`+"\x00"+`{"now":1730000000,"nested":{"a":[1,true,null,"x"]}}`))
	f.Add("\x00\x00\x00\x00\x00\x00\x00", []byte("{}\x00null\x00[]\x00\"x\""))
	f.Fuzz(func(t *testing.T, texts string, objects []byte) {
		s := fuzzFields(texts, 8)
		o := fuzzFields(string(objects), 4)
		check := CheckRequest{
			User: User{
				Key: s[0], FirstName: s[1], LastName: s[2], Email: s[3],
				Attributes: fuzzObject(o[0]),
			},
			Action: s[4],
			Resource: Resource{
				Type: s[5], Key: s[6], Tenant: s[7], Attributes: fuzzObject(o[1]),
				Context: fuzzObject(o[2]),
			},
			Context: fuzzObject(o[3]),
		}
		body, err := wire.Encode(check.sent())
		if err != nil {
			t.Fatalf("encoding %#v: %v", check, err)
		}
		var got map[string]any
		if err := wire.Decode(body, &got); err != nil {
			t.Fatalf("the body %s does not decode: %v", body, err)
		}
		user := map[string]any{"key": check.User.Key}
		putText(user, "first_name", check.User.FirstName)
		putText(user, "last_name", check.User.LastName)
		putText(user, "email", check.User.Email)
		putObject(user, "attributes", check.User.Attributes)
		tenant := check.Resource.Tenant
		if tenant == "" {
			tenant = "default"
		}
		resource := map[string]any{"type": check.Resource.Type, "tenant": tenant}
		putText(resource, "key", check.Resource.Key)
		putObject(resource, "attributes", check.Resource.Attributes)
		putObject(resource, "context", check.Resource.Context)
		want := map[string]any{"user": user, "action": check.Action, "resource": resource}
		putObject(want, "context", check.Context)
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("the body %s differs from the check (-want +got):\n%s", body, diff)
		}
	})
}
