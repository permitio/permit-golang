package permit

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The routes of the condition set rules of an environment, as mockpermit.Server.Handle takes them.
const (
	listSetRulesRoute   = "GET /v2/facts/{proj_id}/{env_id}/set_rules"
	createSetRuleRoute  = "POST /v2/facts/{proj_id}/{env_id}/set_rules"
	deleteSetRuleRoute  = "DELETE /v2/facts/{proj_id}/{env_id}/set_rules"
	setRulesPathInScope = "/v2/facts/p/e/set_rules"
)

// rulesOf returns the condition set rules r<from> to r<to-1>, with only a key and the user set
// u: an empty list when to is from.
func rulesOf(from, to int) []models.ConditionSetRuleRead {
	items := make([]models.ConditionSetRuleRead, 0, to-from)
	for i := from; i < to; i++ {
		items = append(items, models.ConditionSetRuleRead{Key: "r" + strconv.Itoa(i), UserSet: "u"})
	}
	return items
}

// conditionSetRuleCall is a call of a method of the condition set rules, with the ID of its
// operation, its route and a reply of the API to it.
type conditionSetRuleCall struct {
	name  string
	op    string
	route string
	reply mockpermit.Reply
	call  func(ctx context.Context, c *Client) error
}

// conditionSetRuleCalls are calls of each method of the condition set rules.
var conditionSetRuleCalls = []conditionSetRuleCall{
	{
		name: "List", op: opListSetPermissions, route: listSetRulesRoute,
		reply: mockpermit.Reply{Body: `[]`},
		call: func(ctx context.Context, c *Client) error {
			_, err := c.ConditionSetRules.List(ctx, nil)
			return err
		},
	},
	{
		name: "All", op: opListSetPermissions, route: listSetRulesRoute,
		reply: mockpermit.Reply{Body: `[]`},
		call: func(ctx context.Context, c *Client) error {
			return firstError(c.ConditionSetRules.All(ctx, nil))
		},
	},
	{
		name: "Create", op: opAssignSetPermissions, route: createSetRuleRoute,
		reply: mockpermit.Reply{Body: `[]`},
		call: func(ctx context.Context, c *Client) error {
			_, err := c.ConditionSetRules.Create(ctx, models.ConditionSetRuleCreate{
				UserSet: "adults", Permission: "document:read", ResourceSet: "public_docs",
			})
			return err
		},
	},
	{
		name: "Delete", op: opUnassignSetPermissions, route: deleteSetRuleRoute,
		reply: mockpermit.Reply{Status: http.StatusNoContent},
		call: func(ctx context.Context, c *Client) error {
			return c.ConditionSetRules.Delete(ctx, models.ConditionSetRuleRemove{
				UserSet: "adults", Permission: "document:read", ResourceSet: "public_docs",
			})
		},
	},
}

// TestConditionSetRulesAllReadsEveryPage checks that All reads the 31 condition set rules of a
// list, in order, asking for each page in turn with the filters of its options, up to the empty
// page after the last: at a per_page of 30, two pages of rules; at the default per_page of 100,
// the most that the API gives on a page, one page of 31; and from page 2 on, when the options
// start there.
func TestConditionSetRulesAllReadsEveryPage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    *ConditionSetRulesListOptions
		pages   [][]models.ConditionSetRuleRead
		want    []models.ConditionSetRuleRead
		queries []string
	}{
		{
			name: "per_page 30",
			opts: &ConditionSetRulesListOptions{
				ListOptions: ListOptions{PerPage: 30}, UserSet: "u", ResourceSet: "s",
			},
			pages: [][]models.ConditionSetRuleRead{rulesOf(0, 30), rulesOf(30, 31), {}},
			want:  rulesOf(0, 31),
			queries: []string{
				"page=1&per_page=30&resource_set=s&user_set=u",
				"page=2&per_page=30&resource_set=s&user_set=u",
				"page=3&per_page=30&resource_set=s&user_set=u",
			},
		},
		{
			name:  "the default per_page",
			opts:  &ConditionSetRulesListOptions{Permission: "document:read"},
			pages: [][]models.ConditionSetRuleRead{rulesOf(0, 31), {}},
			want:  rulesOf(0, 31),
			queries: []string{
				"page=1&per_page=100&permission=document%3Aread",
				"page=2&per_page=100&permission=document%3Aread",
			},
		},
		{
			name:    "from page 2",
			opts:    &ConditionSetRulesListOptions{ListOptions: ListOptions{Page: 2, PerPage: 30}},
			pages:   [][]models.ConditionSetRuleRead{rulesOf(30, 31), {}},
			want:    rulesOf(30, 31),
			queries: []string{"page=2&per_page=30", "page=3&per_page=30"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			var replies []mockpermit.Reply
			for _, items := range tc.pages {
				replies = append(replies, arrayOf(t, items))
			}
			api.Handle(listSetRulesRoute, replies...)
			items, errs := collect(t, resourceClient(t, api).ConditionSetRules.All(t.Context(),
				tc.opts))
			if diff := cmp.Diff(tc.want, items); diff != "" || errs != nil {
				t.Errorf("All yielded the errors %v and rules that differ (-want +got):\n%s",
					errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestConditionSetRulesAllYieldsAnErrorOnce checks that a page that fails ends the iterator with
// its error, once, after the rules of the pages before it; that a loop that stops early asks for
// no other page; and that the options of All are read when it is called: a later change to them
// changes no request.
func TestConditionSetRulesAllYieldsAnErrorOnce(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(listSetRulesRoute, arrayOf(t, rulesOf(0, 30)),
		mockpermit.Reply{Status: http.StatusForbidden, Body: body}, arrayOf(t, rulesOf(0, 30)))
	opts := &ConditionSetRulesListOptions{ListOptions: ListOptions{PerPage: 30}, UserSet: "a"}
	seq := resourceClient(t, api).ConditionSetRules.All(t.Context(), opts)
	opts.PerPage, opts.UserSet, opts.Permission, opts.ResourceSet = 50, "b", "c", "d"
	items, errs := collect(t, seq)
	if len(items) != 30 || len(errs) != 1 || !errors.Is(errs[0], ErrForbidden) {
		t.Errorf("All yielded %d rules and the errors %v, want 30 and one error that matches "+
			"ErrForbidden", len(items), errs)
	}
	for range seq {
		break
	}
	want := []string{
		"page=1&per_page=30&user_set=a", "page=2&per_page=30&user_set=a",
		"page=1&per_page=30&user_set=a",
	}
	if diff := cmp.Diff(want, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestConditionSetRuleListQueries checks the query of List: none for nil options and for the
// zero options, so no empty user_set, permission or resource_set, which v1 sent; each filter
// alone, escaped, and only when it is set; and every option at once. All sends the page and the
// per_page of its first page, and no filter that its options leave empty.
func TestConditionSetRuleListQueries(t *testing.T) {
	type listCall func(ctx context.Context, c *Client) (*Page[models.ConditionSetRuleRead], error)
	list := func(opts *ConditionSetRulesListOptions) listCall {
		return func(ctx context.Context, c *Client) (*Page[models.ConditionSetRuleRead], error) {
			return c.ConditionSetRules.List(ctx, opts)
		}
	}
	for _, tc := range []struct {
		name, query string
		call        listCall
		page        int
	}{
		{name: "nil options", call: list(nil)},
		{name: "the zero options", call: list(&ConditionSetRulesListOptions{})},
		{
			name: "a user set", query: "user_set=a%2Fb",
			call: list(&ConditionSetRulesListOptions{UserSet: "a/b"}),
		},
		{
			name: "a permission", query: "permission=document%3Aread",
			call: list(&ConditionSetRulesListOptions{Permission: "document:read"}),
		},
		{
			name: "a resource set", query: "resource_set=50%25%C3%A9",
			call: list(&ConditionSetRulesListOptions{ResourceSet: "50%é"}),
		},
		{
			name: "every option",
			query: "page=3&per_page=7&permission=" + fixtures.UUIDKey +
				"&resource_set=s+t&user_set=.u",
			call: list(&ConditionSetRulesListOptions{
				ListOptions: ListOptions{Page: 3, PerPage: 7}, UserSet: ".u",
				Permission: fixtures.UUIDKey, ResourceSet: "s t",
			}),
			page: 3,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listSetRulesRoute, mockpermit.Reply{Body: `[]`})
			got, err := tc.call(t.Context(), resourceClient(t, api))
			if err != nil {
				t.Fatalf("List returned %v", err)
			}
			if got.Page != max(tc.page, 1) || got.Data == nil || len(got.Data) != 0 ||
				got.TotalCount != nil || got.PageCount != nil {
				t.Errorf("List returned %+v, want an empty page %d without counts", got, tc.page)
			}
			if requests := api.Requests(); len(requests) != 1 ||
				requests[0].RawPath != setRulesPathInScope || requests[0].RawQuery != tc.query {
				t.Errorf("List sent %+v, want one request to %s?%s", requests,
					setRulesPathInScope, tc.query)
			}
		})
	}
	api := mockpermit.NewAPI(t)
	api.Handle(listSetRulesRoute, mockpermit.Reply{Body: `[]`})
	if _, errs := collect(t, resourceClient(t, api).ConditionSetRules.All(t.Context(),
		&ConditionSetRulesListOptions{})); errs != nil {
		t.Fatalf("All yielded the errors %v", errs)
	}
	if got := queries(api); !cmp.Equal(got, []string{"page=1&per_page=100"}) {
		t.Errorf("All of the zero options sent the queries %q, want page=1&per_page=100", got)
	}
}

// TestInvalidConditionSetRuleListOptions checks that a Page or a PerPage less than zero is an
// error that matches ErrInvalidConfig from List and from All, and that neither sends a request.
func TestInvalidConditionSetRuleListOptions(t *testing.T) {
	for _, page := range []ListOptions{{Page: -1}, {PerPage: -1}} {
		api := mockpermit.NewAPI(t)
		c := resourceClient(t, api)
		opts := &ConditionSetRulesListOptions{ListOptions: page, UserSet: "u"}
		_, listErr := c.ConditionSetRules.List(t.Context(), opts)
		_, allErrs := collect(t, c.ConditionSetRules.All(t.Context(), opts))
		if len(allErrs) != 1 {
			t.Errorf("%+v: All yielded the errors %v, want one", page, allErrs)
		}
		for i, err := range append([]error{listErr}, allErrs...) {
			if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("%+v: call %d returned %v, want an error that matches ErrInvalidConfig",
					page, i, err)
			}
		}
		api.AssertUntouched()
	}
}

// TestConditionSetRulesUseTheScope checks that the calls of the condition set rules act on the
// environment of the client's scope. Without WithScope, the client looks up the scope of its API
// key once, and each call fills the route with the project and the environment of an
// environment-level key. A scope without an environment, that of a project-level or an
// organization-level key, is an error that matches ErrInvalidConfig and names the operation, and
// no call sends a request but the lookup.
func TestConditionSetRulesUseTheScope(t *testing.T) {
	const path = "/v2/facts/" + scopeProjectID + "/" + scopeEnvironmentID + "/set_rules"
	t.Run("an environment-level key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		replies := map[string][]mockpermit.Reply{}
		for _, m := range conditionSetRuleCalls {
			replies[m.route] = append(replies[m.route], m.reply)
		}
		for route, r := range replies {
			api.Handle(route, r...)
		}
		c := projectClient(t, api)
		for _, m := range conditionSetRuleCalls {
			if err := m.call(t.Context(), c); err != nil {
				t.Errorf("%s returned %v", m.name, err)
			}
		}
		var paths []string
		for _, r := range api.Requests() {
			paths = append(paths, r.RawPath)
		}
		want := []string{"/v2/api-key/scope", path, path, path, path}
		if diff := cmp.Diff(want, paths); diff != "" {
			t.Errorf("the requests differ (-want +got):\n%s", diff)
		}
	})
	for _, suffix := range []string{".project", ".organization"} {
		t.Run("the scope of the fixture"+suffix, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(scopeRoute, scopeReply(t, suffix))
			c := projectClient(t, api)
			for _, m := range conditionSetRuleCalls {
				err := m.call(t.Context(), c)
				if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), m.op) {
					t.Errorf("%s returned %v, want an error that matches ErrInvalidConfig and "+
						"names %s", m.name, err, m.op)
				}
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("the calls sent %d requests, want only the lookup of the scope", n)
			}
		})
	}
}

// TestConditionSetRulesStayOnTheAPI checks that a client with WithFactsViaPDP sends each call of
// the condition set rules to the API, under /v2/facts/ with the project and the environment of
// its scope, as a client without it does, with no header of a wait for sync, and sends nothing
// to the PDP, which only the API's routes of the facts move to.
func TestConditionSetRulesStayOnTheAPI(t *testing.T) {
	api := mockpermit.NewAPI(t)
	replies := map[string][]mockpermit.Reply{}
	for _, m := range conditionSetRuleCalls {
		replies[m.route] = append(replies[m.route], m.reply)
	}
	for route, r := range replies {
		api.Handle(route, r...)
	}
	c := resourceClient(t, api, WithFactsViaPDP())
	for _, m := range conditionSetRuleCalls {
		if err := m.call(t.Context(), c); err != nil {
			t.Errorf("%s with WithFactsViaPDP returned %v", m.name, err)
		}
	}
	requests := api.Requests()
	if len(requests) != len(conditionSetRuleCalls) {
		t.Fatalf("the API got %d requests, want %d", len(requests), len(conditionSetRuleCalls))
	}
	for i, r := range requests {
		if r.RawPath != setRulesPathInScope || r.Header.Get("X-Wait-Timeout") != "" ||
			r.Header.Get("X-Timeout-Policy") != "" {
			t.Errorf("%s sent %s with the headers %v, want %s without a wait",
				conditionSetRuleCalls[i].name, r.RawPath, r.Header, setRulesPathInScope)
		}
	}
}

// TestConditionSetRuleNotFound checks that a 404 to each call of the condition set rules, with or
// without a body, is an *Error that matches ErrNotFound after one request, also from Delete,
// whose body names the rule, so that a mistyped key is never taken for a rule that is gone; and
// that another error status to Delete, such as 403, 409 or 422, is an *Error with the status that
// does not match ErrNotFound.
func TestConditionSetRuleNotFound(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"NOT_FOUND","title":"Not Found","message":"no such condition set"}`, "",
	} {
		for _, m := range conditionSetRuleCalls {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
			err := m.call(t.Context(), resourceClient(t, api))
			e := mustError(t, err)
			if !errors.Is(err, ErrNotFound) || e.StatusCode != http.StatusNotFound || e.Op != m.op {
				t.Errorf("%s of a reply 404 with the body %q returned %v, want an *Error of %s "+
					"that matches ErrNotFound", m.name, body, err, m.op)
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("%s sent %d requests, want 1", m.name, n)
			}
		}
	}
	remove := conditionSetRuleCalls[3]
	for _, status := range []int{
		http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(remove.route, mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := remove.call(t.Context(), resourceClient(t, api))
		if e := mustError(t, err); e.StatusCode != status || e.Op != opUnassignSetPermissions ||
			e.Detail != "no" || errors.Is(err, ErrNotFound) {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status and the detail", status, err, opUnassignSetPermissions)
		}
	}
}

// TestConditionSetRuleBodies checks the bodies of Create and Delete byte for byte against the
// golden files in testdata/golden: with the required fields alone, which the body always holds,
// also when they are empty; and with every field, the flags set, and keys that an encoder may
// change, a line separator and <, > and &.
func TestConditionSetRuleBodies(t *testing.T) {
	odd := "<a>&" + fixtures.LineSeparator + "b"
	for _, tc := range []struct {
		golden string
		route  string
		call   func(ctx context.Context, c *Client) error
	}{
		{
			"assign_set_permissions.required.golden", createSetRuleRoute,
			func(ctx context.Context, c *Client) error {
				_, err := c.ConditionSetRules.Create(ctx, models.ConditionSetRuleCreate{
					UserSet: "adults", Permission: "document:read", ResourceSet: "public_docs",
				})
				return err
			},
		},
		{
			"assign_set_permissions.empty.golden", createSetRuleRoute,
			func(ctx context.Context, c *Client) error {
				_, err := c.ConditionSetRules.Create(ctx, models.ConditionSetRuleCreate{})
				return err
			},
		},
		{
			"assign_set_permissions.every-field.golden", createSetRuleRoute,
			func(ctx context.Context, c *Client) error {
				_, err := c.ConditionSetRules.Create(ctx, models.ConditionSetRuleCreate{
					UserSet: "member", Permission: odd, ResourceSet: "document", IsRole: true,
					IsResource: true,
				})
				return err
			},
		},
		{
			"unassign_set_permissions.required.golden", deleteSetRuleRoute,
			func(ctx context.Context, c *Client) error {
				return c.ConditionSetRules.Delete(ctx, models.ConditionSetRuleRemove{
					UserSet: "adults", Permission: "document:read", ResourceSet: "public_docs",
				})
			},
		},
		{
			"unassign_set_permissions.every-field.golden", deleteSetRuleRoute,
			func(ctx context.Context, c *Client) error {
				return c.ConditionSetRules.Delete(ctx, models.ConditionSetRuleRemove{
					UserSet: "member", Permission: odd, ResourceSet: "document", IsRole: true,
					IsResource: true,
				})
			},
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: `[]`})
			if err := tc.call(t.Context(), resourceClient(t, api)); err != nil {
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

// TestConditionSetRuleCreateReply checks that Create returns the rules of the API's reply, a
// JSON array, in its order: the two rules of a reply that holds two, and an empty slice, not nil,
// for an empty array.
func TestConditionSetRuleCreateReply(t *testing.T) {
	for _, want := range [][]models.ConditionSetRuleRead{rulesOf(0, 2), rulesOf(0, 0)} {
		api := mockpermit.NewAPI(t)
		api.Handle(createSetRuleRoute, arrayOf(t, want))
		got, err := resourceClient(t, api).ConditionSetRules.Create(t.Context(),
			models.ConditionSetRuleCreate{UserSet: "u", Permission: "r:a", ResourceSet: "s"})
		if diff := cmp.Diff(want, got); diff != "" || err != nil || got == nil {
			t.Errorf("Create returned %v and rules that differ, or nil (-want +got):\n%s", err,
				diff)
		}
	}
}

// TestConditionSetRuleRetries checks that each call of the condition set rules, which does the
// same when it is sent twice, sends its request again after a 503 and after a 429: a create of a
// permission that is granted already, and a delete of one that is not, are skipped.
func TestConditionSetRuleRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusTooManyRequests} {
		for _, m := range conditionSetRuleCalls {
			api := mockpermit.NewAPI(t)
			api.Handle(
				m.route,
				mockpermit.Reply{Status: status, Header: again, Body: `{}`},
				m.reply,
			)
			c := resourceClient(t, api, WithRetry(RetryPolicy{MaxRetries: 1}))
			if err := m.call(t.Context(), c); err != nil {
				t.Errorf("%s after a %d returned %v, want the result of the second request",
					m.name, status, err)
			}
			if n := len(api.Requests()); n != 2 {
				t.Errorf("%s after a %d sent %d requests, want 2", m.name, status, n)
			}
		}
	}
}

// TestMalformedConditionSetRuleReplies checks that a reply in 200-299 to List, All and Create
// that is not a JSON array of rules, such as an empty body, null, an object, the paginated
// object, or an array with an item that is not a rule, is an *Error that names the operation,
// its route and the model, and no page, no rule of the iterator and no rules, never an empty
// list.
func TestMalformedConditionSetRuleReplies(t *testing.T) {
	want := func(op, route string) string {
		return "permit: " + op + ": " + route + ": status 200: the reply is not a valid list of " +
			"models.ConditionSetRuleRead"
	}
	listWant := want(opListSetPermissions, listSetRulesRoute)
	createWant := want(opAssignSetPermissions, createSetRuleRoute)
	for _, body := range []string{
		``, `null`, `{}`, `{"data":[],"total_count":0}`, `"a"`, `[1]`, `[{"id":1}]`, `[] []`,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(listSetRulesRoute, mockpermit.Reply{Body: body}, mockpermit.Reply{Body: body})
		api.Handle(createSetRuleRoute, mockpermit.Reply{Body: body})
		c := resourceClient(t, api)
		page, err := c.ConditionSetRules.List(t.Context(), nil)
		if e := mustError(t, err); page != nil || e.Error() != listWant {
			t.Errorf("List of the reply %q returned %+v and\n%v\nwant no page and\n%s", body,
				page, err, listWant)
		}
		items, errs := collect(t, c.ConditionSetRules.All(t.Context(), nil))
		if items != nil || len(errs) != 1 || mustError(t, errs[0]).Error() != listWant {
			t.Errorf("All of the reply %q yielded %+v and %v, want only the error\n%s", body,
				items, errs, listWant)
		}
		rules, err := c.ConditionSetRules.Create(t.Context(),
			models.ConditionSetRuleCreate{UserSet: "u", Permission: "r:a", ResourceSet: "s"})
		if e := mustError(t, err); rules != nil || e.Error() != createWant {
			t.Errorf("Create of the reply %q returned %+v and\n%v\nwant no rules and\n%s", body,
				rules, err, createWant)
		}
	}
}

// TestConditionSetRulesAfterClose checks that each method returns ErrClosed after Close, and
// sends nothing, also without WithScope, whose lookup it does not send either.
func TestConditionSetRulesAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	_, allErrs := collect(t, c.ConditionSetRules.All(t.Context(), nil))
	if len(allErrs) != 1 {
		t.Errorf("All after Close yielded %v, want ErrClosed once", allErrs)
	}
	errs := allErrs
	for _, m := range conditionSetRuleCalls {
		errs = append(errs, m.call(t.Context(), c))
	}
	for i, err := range errs {
		if !errors.Is(err, ErrClosed) {
			t.Errorf("call %d after Close returned %v, want ErrClosed", i, err)
		}
	}
	api.AssertUntouched()
}
