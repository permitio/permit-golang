package permit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/redact"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/models"
)

// The routes of the proxy configs, as mockpermit.Server.Handle takes them.
const (
	listProxyConfigsRoute  = "GET /v2/facts/{proj_id}/{env_id}/proxy_configs"
	createProxyConfigRoute = "POST /v2/facts/{proj_id}/{env_id}/proxy_configs"
	getProxyConfigRoute    = "GET /v2/facts/{proj_id}/{env_id}/proxy_configs/" +
		"{proxy_config_id}"
	updateProxyConfigRoute = "PATCH /v2/facts/{proj_id}/{env_id}/proxy_configs/" +
		"{proxy_config_id}"
	deleteProxyConfigRoute = "DELETE /v2/facts/{proj_id}/{env_id}/proxy_configs/" +
		"{proxy_config_id}"
)

// proxyConfigsPath is the path of the proxy configs in the scope of resourceClient.
const proxyConfigsPath = "/v2/facts/p/e/proxy_configs"

// The credentials of the tests of the secrets of the proxy configs. None has the shape of a
// Permit API key, which the client hides whatever it holds, so only the scrub of the field secret
// and the client's Set hide them.
const (
	proxyBearerValue = "opaque-bearer-value"
	proxyBasicValue  = "opaque-user:opaque-pass"
	proxyHeaderValue = "opaque-header-value"
	proxyRegionValue = "opaque-region-value"
)

// proxyConfigBody returns the body of a reply of the API that holds the proxy config key, named
// as its key, with secret, the JSON text of its secret, and the auth mechanism mechanism, which
// the body leaves out when it is empty.
func proxyConfigBody(key, mechanism, secret string) string {
	body := `{"key":"` + key + `","name":"` + key + `","secret":` + secret
	if mechanism != "" {
		body += `,"auth_mechanism":"` + mechanism + `"`
	}
	return body + "}"
}

// bearerConfigBody is the body of a reply of the API that holds the proxy config payments, whose
// secret is a Bearer token.
var bearerConfigBody = proxyConfigBody("payments", "Bearer", `"`+proxyBearerValue+`"`)

// bearerSecret returns a ProxySecret whose Bearer token is token.
func bearerSecret(token string) models.ProxySecret {
	return models.ProxySecret{Bearer: new(models.NewSecret(token))}
}

// headersSecret is a ProxySecret of two headers, X-Api-Key and X-Region, and headersJSON its
// JSON text as a request sends it and a reply returns it.
var (
	headersSecret = models.ProxySecret{Headers: map[string]models.Secret{
		"X-Api-Key": models.NewSecret(proxyHeaderValue),
		"X-Region":  models.NewSecret(proxyRegionValue),
	}}
	headersJSON = `{"X-Api-Key":"` + proxyHeaderValue + `","X-Region":"` + proxyRegionValue +
		`"}`
)

// proxyConfigsOf returns the configs c<from> to c<to-1>, each named as its key, with the Bearer
// token t<i> and no auth mechanism, and the reply of the API that holds them as a JSON array: an
// empty list when to is from.
func proxyConfigsOf(from, to int) ([]models.ProxyConfigRead, mockpermit.Reply) {
	items := make([]models.ProxyConfigRead, 0, to-from)
	bodies := make([]string, 0, to-from)
	for i := from; i < to; i++ {
		key, token := "c"+strconv.Itoa(i), "t"+strconv.Itoa(i)
		items = append(items, models.ProxyConfigRead{
			Key: key, Name: key, Secret: bearerSecret(token),
		})
		bodies = append(bodies, proxyConfigBody(key, "", `"`+token+`"`))
	}
	return items, mockpermit.Reply{Body: "[" + strings.Join(bodies, ",") + "]"}
}

// proxyConfigCalls are calls of each method of the proxy configs that acts on one config, with
// the ID of its operation, its route and a reply of the API to it.
var proxyConfigCalls = []struct {
	name  string
	op    string
	route string
	reply mockpermit.Reply
	call  func(ctx context.Context, c *Client, proxyConfigKey string) error
}{
	{
		name: "Get", op: opGetProxyConfig, route: getProxyConfigRoute,
		reply: mockpermit.Reply{Body: bearerConfigBody},
		call: func(ctx context.Context, c *Client, proxyConfigKey string) error {
			_, err := c.ProxyConfigs.Get(ctx, proxyConfigKey)
			return err
		},
	},
	{
		name: "Update", op: opUpdateProxyConfig, route: updateProxyConfigRoute,
		reply: mockpermit.Reply{Body: bearerConfigBody},
		call: func(ctx context.Context, c *Client, proxyConfigKey string) error {
			_, err := c.ProxyConfigs.Update(ctx, proxyConfigKey,
				models.ProxyConfigUpdate{Secret: bearerSecret(proxyBearerValue)})
			return err
		},
	},
	{
		name: "Delete", op: opDeleteProxyConfig, route: deleteProxyConfigRoute,
		reply: mockpermit.Reply{Status: http.StatusNoContent},
		call: func(ctx context.Context, c *Client, proxyConfigKey string) error {
			return c.ProxyConfigs.Delete(ctx, proxyConfigKey)
		},
	},
}

// createProxyConfig creates the config k with a Bearer token with c, and returns the error.
func createProxyConfig(ctx context.Context, c *Client) error {
	_, err := c.ProxyConfigs.Create(ctx, models.ProxyConfigCreate{
		Key: "k", Name: "n", Secret: bearerSecret(proxyBearerValue),
	})
	return err
}

// TestProxyConfigsAllReadsEveryPage checks that ProxyConfigsService.All reads the 31 configs of
// a list, in order, asking for each page in turn: at a per_page of 30, two pages of configs and
// the empty page after them; at the default per_page of 100, the most that the API gives on a
// page, one page of 31 and the empty page after it; and from page 2 on, when the options start
// there.
func TestProxyConfigsAllReadsEveryPage(t *testing.T) {
	all, _ := proxyConfigsOf(0, 31)
	_, first := proxyConfigsOf(0, 30)
	_, second := proxyConfigsOf(30, 31)
	_, every := proxyConfigsOf(0, 31)
	_, empty := proxyConfigsOf(0, 0)
	for _, tc := range []struct {
		name    string
		opts    *ListOptions
		pages   []mockpermit.Reply
		want    []models.ProxyConfigRead
		queries []string
	}{
		{
			name: "per_page 30", opts: &ListOptions{PerPage: 30},
			pages: []mockpermit.Reply{first, second, empty}, want: all,
			queries: []string{
				"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30",
			},
		},
		{
			name: "the default per_page", pages: []mockpermit.Reply{every, empty}, want: all,
			queries: []string{"page=1&per_page=100", "page=2&per_page=100"},
		},
		{
			name: "from page 2", opts: &ListOptions{Page: 2, PerPage: 30},
			pages: []mockpermit.Reply{second, empty}, want: all[30:],
			queries: []string{"page=2&per_page=30", "page=3&per_page=30"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listProxyConfigsRoute, tc.pages...)
			items, errs := collect(t, resourceClient(t, api).ProxyConfigs.All(t.Context(),
				tc.opts))
			if diff := cmp.Diff(tc.want, items); diff != "" || errs != nil {
				t.Errorf("All yielded the errors %v and configs that differ (-want +got):\n%s",
					errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestProxyConfigsAllYieldsAnErrorOnce checks that a page that fails ends the iterator with its
// error, once, after the configs of the pages before it, and that the options of All are read
// when it is called: a later change to them changes no request.
func TestProxyConfigsAllYieldsAnErrorOnce(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	_, first := proxyConfigsOf(0, 30)
	api := mockpermit.NewAPI(t)
	api.Handle(listProxyConfigsRoute, first,
		mockpermit.Reply{Status: http.StatusForbidden, Body: body})
	opts := &ListOptions{PerPage: 30}
	seq := resourceClient(t, api).ProxyConfigs.All(t.Context(), opts)
	opts.PerPage, opts.Page = 50, 4
	items, errs := collect(t, seq)
	if len(items) != 30 || len(errs) != 1 || !errors.Is(errs[0], ErrForbidden) {
		t.Errorf("All yielded %d configs and the errors %v, want 30 and one error that matches "+
			"ErrForbidden", len(items), errs)
	}
	want := []string{"page=1&per_page=30", "page=2&per_page=30"}
	if diff := cmp.Diff(want, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestProxyConfigsListReturnsAPage checks the page that List returns: the configs of the reply in
// its order, the page and the per_page that it asked for, page 1 and no query for nil options and
// for the zero options, and no counts, as the reply is a JSON array. A page without configs has
// an empty list, not nil.
func TestProxyConfigsListReturnsAPage(t *testing.T) {
	two, twoReply := proxyConfigsOf(0, 2)
	one, oneReply := proxyConfigsOf(0, 1)
	none, noneReply := proxyConfigsOf(0, 0)
	for _, tc := range []struct {
		name  string
		opts  *ListOptions
		reply mockpermit.Reply
		query string
		want  *Page[models.ProxyConfigRead]
	}{
		{
			name: "nil options", reply: twoReply,
			want: &Page[models.ProxyConfigRead]{Data: two, Page: 1},
		},
		{
			name: "the zero options", opts: &ListOptions{}, reply: oneReply,
			want: &Page[models.ProxyConfigRead]{Data: one, Page: 1},
		},
		{
			name: "a page after the last", opts: &ListOptions{Page: 3, PerPage: 30},
			reply: noneReply, query: "page=3&per_page=30",
			want: &Page[models.ProxyConfigRead]{Data: none, Page: 3, PerPage: 30},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listProxyConfigsRoute, tc.reply)
			got, err := resourceClient(t, api).ProxyConfigs.List(t.Context(), tc.opts)
			if err != nil {
				t.Fatalf("List returned %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" || got.Data == nil {
				t.Errorf("List returned a page that differs (-want +got), or nil Data:\n%s", diff)
			}
			if requests := api.Requests(); len(requests) != 1 ||
				requests[0].RawPath != proxyConfigsPath || requests[0].RawQuery != tc.query {
				t.Errorf("List sent %+v, want one request to %s?%s", requests, proxyConfigsPath,
					tc.query)
			}
		})
	}
}

// TestInvalidProxyConfigListOptions checks that a Page or a PerPage less than zero is an error
// that matches ErrInvalidConfig from List and All, and that neither sends a request.
func TestInvalidProxyConfigListOptions(t *testing.T) {
	for _, opts := range []ListOptions{{Page: -1}, {PerPage: -1}} {
		api := mockpermit.NewAPI(t)
		c := resourceClient(t, api)
		_, err := c.ProxyConfigs.List(t.Context(), &opts)
		_, errs := collect(t, c.ProxyConfigs.All(t.Context(), &opts))
		for _, err := range append([]error{err}, errs...) {
			if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("%+v: the call returned %v, want an error that matches ErrInvalidConfig",
					opts, err)
			}
		}
		if len(errs) != 1 {
			t.Errorf("%+v: All yielded the errors %v, want one", opts, errs)
		}
		api.AssertUntouched()
	}
}

// TestProxyConfigsUseTheScope checks that the calls of the proxy configs act on the environment
// of the client's scope. Without WithScope, the client looks up the scope of its API key once,
// and each call fills the route with the project and the environment of an environment-level
// key. A scope without an environment, that of a project-level or an organization-level key, is
// an error that matches ErrInvalidConfig and names the operation, and no call sends a request
// but the lookup, which the API fails for any route that the test did not register.
func TestProxyConfigsUseTheScope(t *testing.T) {
	const prefix = "/v2/facts/" + scopeProjectID + "/" + scopeEnvironmentID + "/proxy_configs"
	calls := map[string]func(ctx context.Context, c *Client) error{
		opListProxyConfigs: func(ctx context.Context, c *Client) error {
			_, err := c.ProxyConfigs.List(ctx, nil)
			return err
		},
		opCreateProxyConfig: createProxyConfig,
	}
	for _, m := range proxyConfigCalls {
		calls[m.op] = func(ctx context.Context, c *Client) error {
			return m.call(ctx, c, "payments")
		}
	}
	t.Run("an environment-level key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		api.Handle(listProxyConfigsRoute, mockpermit.Reply{Body: `[]`})
		api.Handle(createProxyConfigRoute, mockpermit.Reply{Body: bearerConfigBody})
		for _, m := range proxyConfigCalls {
			api.Handle(m.route, m.reply)
		}
		c := projectClient(t, api)
		for op, call := range calls {
			if err := call(t.Context(), c); err != nil {
				t.Errorf("the call of %s returned %v", op, err)
			}
		}
		requests := api.Requests()
		if len(requests) != len(calls)+1 || requests[0].RawPath != "/v2/api-key/scope" {
			t.Fatalf("the calls sent %+v, want the lookup of the scope and one request for each "+
				"of the %d calls", requests, len(calls))
		}
		for _, r := range requests[1:] {
			if r.RawPath != prefix && r.RawPath != prefix+"/payments" {
				t.Errorf("a call sent its request to %s, want a path under %s", r.RawPath, prefix)
			}
		}
	})
	for _, suffix := range []string{".project", ".organization"} {
		t.Run("the scope of the fixture"+suffix, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(scopeRoute, scopeReply(t, suffix))
			c := projectClient(t, api)
			for op, call := range calls {
				err := call(t.Context(), c)
				if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), op) {
					t.Errorf("the call of %s returned %v, want an error that matches "+
						"ErrInvalidConfig and names %s", op, err, op)
				}
			}
			seq := c.ProxyConfigs.All(t.Context(), nil)
			if _, errs := collect(t, seq); len(errs) != 1 || !errors.Is(errs[0], ErrInvalidConfig) {
				t.Errorf("All yielded %v, want one error that matches ErrInvalidConfig", errs)
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("the calls sent %d requests, want only the lookup of the scope", n)
			}
		})
	}
}

// TestProxyConfigsStayOnTheAPI checks that a client with WithFactsViaPDP sends each call of the
// proxy configs to the API, under /v2/facts/ with the project and the environment of its scope,
// as a client without it does, with no header of a wait for sync, and sends nothing to the PDP,
// which does not serve the proxy configs.
func TestProxyConfigsStayOnTheAPI(t *testing.T) {
	api := mockpermit.NewAPI(t)
	api.Handle(listProxyConfigsRoute, mockpermit.Reply{Body: `[]`}, mockpermit.Reply{Body: `[]`})
	api.Handle(createProxyConfigRoute, mockpermit.Reply{Body: bearerConfigBody})
	for _, m := range proxyConfigCalls {
		api.Handle(m.route, m.reply)
	}
	c := resourceClient(t, api, WithFactsViaPDP())
	ctx := t.Context()
	_, listErr := c.ProxyConfigs.List(ctx, nil)
	_, allErrs := collect(t, c.ProxyConfigs.All(ctx, nil))
	errs := append([]error{listErr, createProxyConfig(ctx, c)}, allErrs...)
	for _, m := range proxyConfigCalls {
		errs = append(errs, m.call(ctx, c, "payments"))
	}
	for i, err := range errs {
		if err != nil {
			t.Errorf("call %d with WithFactsViaPDP returned %v", i, err)
		}
	}
	requests := api.Requests()
	if len(requests) != 2+1+len(proxyConfigCalls) {
		t.Fatalf("the API got %d requests, want %d", len(requests), 3+len(proxyConfigCalls))
	}
	for _, r := range requests {
		if !strings.HasPrefix(r.RawPath, proxyConfigsPath) ||
			r.Header.Get("X-Wait-Timeout") != "" || r.Header.Get("X-Timeout-Policy") != "" {
			t.Errorf("a call sent %s with the headers %v, want a path under %s without a wait",
				r.RawPath, r.Header, proxyConfigsPath)
		}
	}
}

// TestProxyConfigKeysInThePath checks that each method of the proxy configs that acts on one
// config sends its key as one segment of the path, escaped, after the project and the environment
// of the scope, whatever the key holds: a slash, a percent sign, a space, a question mark and a
// hash, a letter outside ASCII, the shape of a UUID and a leading dot. A key that is empty, . or
// .. would change the route, so it is an error that matches ErrInvalidConfig and names the
// placeholder, and no request.
func TestProxyConfigKeysInThePath(t *testing.T) {
	for key, escaped := range map[string]string{
		"a/b":            "a%2Fb",
		"50%":            "50%25",
		"a b?c#d":        "a%20b%3Fc%23d",
		"é":              "%C3%A9",
		fixtures.UUIDKey: fixtures.UUIDKey,
		".hidden":        ".hidden",
		"doc.v2..x":      "doc.v2..x",
	} {
		for _, m := range proxyConfigCalls {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, m.reply)
			if err := m.call(t.Context(), resourceClient(t, api), key); err != nil {
				t.Errorf("%s(%q) returned %v", m.name, key, err)
			}
			want := proxyConfigsPath + "/" + escaped
			if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want {
				t.Errorf("%s(%q) sent %+v, want one request to %s", m.name, key, requests, want)
			}
		}
	}
	for _, key := range []string{"", ".", ".."} {
		for _, m := range proxyConfigCalls {
			api := mockpermit.NewAPI(t)
			err := m.call(t.Context(), resourceClient(t, api), key)
			named := err != nil && strings.Contains(err.Error(), "{proxy_config_id}")
			if !errors.Is(err, ErrInvalidConfig) || !named {
				t.Errorf("%s(%q) returned %v, want an error that matches ErrInvalidConfig and "+
					"names {proxy_config_id}", m.name, key, err)
			}
			api.AssertUntouched()
		}
	}
}

// TestDeleteProxyConfigThatIsGone checks that Delete returns nil for a 404, with or without a
// body, as the config that the path names is gone already, after one request; and an *Error with
// the status for any other error status, such as 403, 409 or 422.
func TestDeleteProxyConfigThatIsGone(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"NOT_FOUND","title":"Not Found","message":"the config does not exist"}`,
		"",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteProxyConfigRoute,
			mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		err := resourceClient(t, api).ProxyConfigs.Delete(t.Context(), "gone")
		if err != nil {
			t.Errorf("Delete of a config that the API answers with 404 and the body %q "+
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
		api.Handle(deleteProxyConfigRoute,
			mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := resourceClient(t, api).ProxyConfigs.Delete(t.Context(), "payments")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteProxyConfig ||
			e.Detail != "no" || errors.Is(err, ErrNotFound) {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status and the detail", status, err, opDeleteProxyConfig)
		}
	}
}

// TestNotFoundOfOtherProxyConfigCalls checks that a 404 to List, Create, Get and Update, which
// delete nothing, is an *Error that matches ErrNotFound.
func TestNotFoundOfOtherProxyConfigCalls(t *testing.T) {
	const body = `{"error_code":"NOT_FOUND","title":"Not Found","message":"no such object"}`
	notFound := mockpermit.Reply{Status: http.StatusNotFound, Body: body}
	type notFoundCall struct {
		op    string
		route string
		call  func(ctx context.Context, c *Client) error
	}
	calls := []notFoundCall{
		{
			op: opListProxyConfigs, route: listProxyConfigsRoute,
			call: func(ctx context.Context, c *Client) error {
				_, err := c.ProxyConfigs.List(ctx, nil)
				return err
			},
		},
		{op: opCreateProxyConfig, route: createProxyConfigRoute, call: createProxyConfig},
	}
	for _, m := range proxyConfigCalls[:2] {
		calls = append(calls, notFoundCall{
			op: m.op, route: m.route,
			call: func(ctx context.Context, c *Client) error {
				return m.call(ctx, c, "gone")
			},
		})
	}
	for _, m := range calls {
		api := mockpermit.NewAPI(t)
		api.Handle(m.route, notFound)
		err := m.call(t.Context(), resourceClient(t, api))
		e := mustError(t, err)
		if !errors.Is(err, ErrNotFound) || e.StatusCode != http.StatusNotFound || e.Op != m.op {
			t.Errorf("%s of an object that the API answers with 404 returned %v, want an "+
				"*Error that matches ErrNotFound", m.op, err)
		}
	}
}

// TestProxyConfigBodies checks the bodies of Create and Update byte for byte against the golden
// files in testdata/golden, with the credentials of the secret in place of the markers that the
// model's own JSON holds, and the auth mechanism of its branch when the call leaves it empty: a
// create with the required fields only, with a Bearer token; one with a Basic credential, its
// auth mechanism and a mapping rule of the required fields only, which leaves the others out; and
// one with every field, with a secret of headers, a name, a URL and a header that an encoder may
// change, with a line separator and <, > and &, a URL type and an HTTP method that the SDK does
// not know, which it sends as they are, an empty map of headers of a rule, which sends {}, and a
// priority of 0, which it sends; an update of the secret alone, which an update always sends; one
// with a secret of one header and an empty list of rules, which sends []; and one that sets every
// field, with a rule to delete.
func TestProxyConfigBodies(t *testing.T) {
	odd := "<a>&" + fixtures.LineSeparator + "b"
	create := func(config models.ProxyConfigCreate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.ProxyConfigs.Create(t.Context(), config)
			return err
		}
	}
	update := func(change models.ProxyConfigUpdate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.ProxyConfigs.Update(t.Context(), "payments", change)
			return err
		}
	}
	basic := models.ProxySecret{Basic: new(models.NewSecret(proxyBasicValue))}
	for _, tc := range []struct {
		golden string
		route  string
		call   func(c *Client) error
	}{
		{
			"create_proxy_config.required.golden", createProxyConfigRoute,
			create(models.ProxyConfigCreate{
				Key: "k", Name: "n", Secret: bearerSecret(proxyBearerValue),
			}),
		},
		{
			"create_proxy_config.basic.golden", createProxyConfigRoute,
			create(models.ProxyConfigCreate{
				Key: "k", Name: "n", AuthMechanism: models.AuthMechanismBasic, Secret: basic,
				MappingRules: []models.MappingRule{{
					URL: "https://api.example.com/r", HTTPMethod: models.MethodsGet, Resource: "r",
				}},
			}),
		},
		{
			"create_proxy_config.every-field.golden", createProxyConfigRoute,
			create(models.ProxyConfigCreate{
				Key: "payments", Name: odd, AuthMechanism: models.AuthMechanismHeaders,
				Secret: models.ProxySecret{Headers: map[string]models.Secret{
					"X-Api-Key": models.NewSecret(proxyHeaderValue),
					"X-Region":  models.NewSecret("<eu>&" + fixtures.LineSeparator),
				}},
				MappingRules: []models.MappingRule{
					{
						URL: "https://api.example.com/a?b=<c>&d", URLType: "glob",
						HTTPMethod: "connect", Resource: "order", Headers: map[string]string{},
						Action: "create", Priority: new(0),
					},
					{
						URL: "https://api.example.com/x", HTTPMethod: models.MethodsGet,
						Resource: "r", Headers: map[string]string{"X-B": "2", "X-A": "1"},
					},
				},
			}),
		},
		{
			"update_proxy_config.secret-only.golden", updateProxyConfigRoute,
			update(models.ProxyConfigUpdate{Secret: bearerSecret(proxyBearerValue)}),
		},
		{
			"update_proxy_config.empty.golden", updateProxyConfigRoute,
			update(models.ProxyConfigUpdate{
				Secret: models.ProxySecret{Headers: map[string]models.Secret{
					"X-Api-Key": models.NewSecret(proxyHeaderValue),
				}},
				MappingRules: []models.MappingRuleUpdate{},
			}),
		},
		{
			"update_proxy_config.every-field.golden", updateProxyConfigRoute,
			update(models.ProxyConfigUpdate{
				Name: "Payments", AuthMechanism: models.AuthMechanismBasic, Secret: basic,
				MappingRules: []models.MappingRuleUpdate{
					{
						URL: "https://api.example.com/orders", HTTPMethod: models.MethodsGet,
						Resource: "order", ShouldDelete: true,
					},
					{
						URL: "https://api.example.com/invoices/.*", URLType: models.URLTypeRegex,
						HTTPMethod: models.MethodsPost, Resource: "invoice",
						Headers: map[string]string{"X-A": "1"}, Action: "create",
						Priority: new(5),
					},
				},
			}),
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: bearerConfigBody})
			if err := tc.call(resourceClient(t, api)); err != nil {
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

// TestProxyConfigSecretsRoundTrip checks that a secret of each auth mechanism, Basic, Bearer and
// Headers, goes to the API and back unchanged: Create and Update send its credentials in the form
// of its branch, a JSON string or a JSON object of strings, with the auth mechanism; and Create,
// Update and Get each read a reply that holds the secret in that form, with that auth mechanism,
// into the same branch with the same credentials.
func TestProxyConfigSecretsRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		mechanism models.AuthMechanism
		secret    models.ProxySecret
		sent      string
	}{
		{
			models.AuthMechanismBasic,
			models.ProxySecret{Basic: new(models.NewSecret(proxyBasicValue))},
			`"` + proxyBasicValue + `"`,
		},
		{models.AuthMechanismBearer, bearerSecret(proxyBearerValue), `"` + proxyBearerValue + `"`},
		{models.AuthMechanismHeaders, headersSecret, headersJSON},
	} {
		t.Run(string(tc.mechanism), func(t *testing.T) {
			mechanism := string(tc.mechanism)
			reply := mockpermit.Reply{Body: proxyConfigBody("payments", mechanism, tc.sent)}
			api := mockpermit.NewAPI(t)
			api.Handle(createProxyConfigRoute, reply)
			api.Handle(updateProxyConfigRoute, reply)
			api.Handle(getProxyConfigRoute, reply)
			c := resourceClient(t, api)
			ctx := t.Context()
			created, createErr := c.ProxyConfigs.Create(ctx, models.ProxyConfigCreate{
				Key: "payments", Name: "payments", AuthMechanism: tc.mechanism, Secret: tc.secret,
			})
			updated, updateErr := c.ProxyConfigs.Update(ctx, "payments", models.ProxyConfigUpdate{
				AuthMechanism: tc.mechanism, Secret: tc.secret,
			})
			got, getErr := c.ProxyConfigs.Get(ctx, "payments")
			want := &models.ProxyConfigRead{
				Key: "payments", Name: "payments", AuthMechanism: tc.mechanism, Secret: tc.secret,
			}
			for name, read := range map[string]struct {
				config *models.ProxyConfigRead
				err    error
			}{
				"Create": {created, createErr}, "Update": {updated, updateErr},
				"Get": {got, getErr},
			} {
				if diff := cmp.Diff(want, read.config); diff != "" || read.err != nil {
					t.Errorf("%s returned %v and a config that differs (-want +got):\n%s", name,
						read.err, diff)
				}
			}
			requests := api.Requests()
			if len(requests) != 3 {
				t.Fatalf("the calls sent %d requests, want 3", len(requests))
			}
			requests[0].ExpectJSON(t, `{"key":"payments","name":"payments","auth_mechanism":"`+
				mechanism+`","secret":`+tc.sent+`}`)
			requests[1].ExpectJSON(t, `{"auth_mechanism":"`+mechanism+`","secret":`+tc.sent+`}`)
		})
	}
}

// TestProxyConfigAuthMechanismOfTheSecret checks the auth mechanism that Create and Update send
// with the secret: the mechanism of the branch that the secret sets, Basic, Bearer or Headers,
// when AuthMechanism is empty; AuthMechanism when it names that branch; and a mechanism that the
// SDK does not know as it is, for the API to decide on.
func TestProxyConfigAuthMechanismOfTheSecret(t *testing.T) {
	basic := models.ProxySecret{Basic: new(models.NewSecret(proxyBasicValue))}
	for _, tc := range []struct {
		name      string
		mechanism models.AuthMechanism
		secret    models.ProxySecret
		sent      string
		want      string
	}{
		{"Basic, left empty", "", basic, `"` + proxyBasicValue + `"`, "Basic"},
		{
			"Bearer, left empty", "", bearerSecret(proxyBearerValue),
			`"` + proxyBearerValue + `"`, "Bearer",
		},
		{"Headers, left empty", "", headersSecret, headersJSON, "Headers"},
		{"Headers, named", models.AuthMechanismHeaders, headersSecret, headersJSON, "Headers"},
		{
			"a mechanism that the SDK does not know", "Digest", bearerSecret(proxyBearerValue),
			`"` + proxyBearerValue + `"`, "Digest",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(createProxyConfigRoute, mockpermit.Reply{Body: bearerConfigBody})
			api.Handle(updateProxyConfigRoute, mockpermit.Reply{Body: bearerConfigBody})
			c := resourceClient(t, api)
			ctx := t.Context()
			_, createErr := c.ProxyConfigs.Create(ctx, models.ProxyConfigCreate{
				Key: "k", Name: "n", AuthMechanism: tc.mechanism, Secret: tc.secret,
			})
			_, updateErr := c.ProxyConfigs.Update(ctx, "k", models.ProxyConfigUpdate{
				AuthMechanism: tc.mechanism, Secret: tc.secret,
			})
			if createErr != nil || updateErr != nil {
				t.Fatalf("Create returned %v and Update %v, want nil", createErr, updateErr)
			}
			requests := api.Requests()
			if len(requests) != 2 {
				t.Fatalf("the calls sent %d requests, want 2", len(requests))
			}
			requests[0].ExpectJSON(t, `{"key":"k","name":"n","auth_mechanism":"`+tc.want+
				`","secret":`+tc.sent+`}`)
			requests[1].ExpectJSON(t, `{"auth_mechanism":"`+tc.want+`","secret":`+tc.sent+`}`)
		})
	}
}

// TestInvalidProxyConfigSecrets checks that Create and Update of a secret that holds no
// credential, as it sets none of Basic, Bearer and Headers or the one that it sets is empty or
// has an empty value, or that sets more than one, also when one of them is empty, or with an
// auth mechanism that names another branch than the one that it sets, return an error that
// matches ErrInvalidConfig and names the operation, the secret or the auth mechanism and the rule
// that the call breaks, and send no request, not even the lookup of the scope, which the client
// without WithScope would send first; that none of the credentials of such a secret joins the
// client's Set; and that a client closed before returns ErrClosed first.
func TestInvalidProxyConfigSecrets(t *testing.T) {
	const basic, token, header = "opaque-rejected-user:pw", "opaque-rejected-token", "opaque-x"
	headers := map[string]models.Secret{"X-A": models.NewSecret(header)}
	for name, tc := range map[string]struct {
		mechanism models.AuthMechanism
		secret    models.ProxySecret
		rule      validate.Rule
	}{
		"the zero ProxySecret": {"", models.ProxySecret{}, validate.ProxyConfigSecretNone},
		"an empty Basic": {
			"",
			models.ProxySecret{Basic: new(models.NewSecret(""))},
			validate.ProxyConfigSecretNone,
		},
		"an empty Bearer": {
			models.AuthMechanismBearer,
			models.ProxySecret{Bearer: &models.Secret{}},
			validate.ProxyConfigSecretNone,
		},
		"an empty map of headers": {
			"",
			models.ProxySecret{Headers: map[string]models.Secret{}},
			validate.ProxyConfigSecretNone,
		},
		"a header without a value": {
			models.AuthMechanismHeaders,
			models.ProxySecret{Headers: map[string]models.Secret{
				"X-A": models.NewSecret(header), "X-B": {},
			}},
			validate.ProxyConfigSecretNone,
		},
		"Basic and Bearer": {
			"",
			models.ProxySecret{
				Basic: new(models.NewSecret(basic)), Bearer: new(models.NewSecret(token)),
			},
			validate.ProxyConfigSecretMany,
		},
		"Basic and Headers": {
			"",
			models.ProxySecret{Basic: new(models.NewSecret(basic)), Headers: headers},
			validate.ProxyConfigSecretMany,
		},
		"Bearer and an empty map of headers": {
			"",
			models.ProxySecret{
				Bearer: new(models.NewSecret(token)), Headers: map[string]models.Secret{},
			},
			validate.ProxyConfigSecretMany,
		},
		"all three": {
			"",
			models.ProxySecret{
				Basic: new(models.NewSecret(basic)), Bearer: new(models.NewSecret(token)),
				Headers: headers,
			},
			validate.ProxyConfigSecretMany,
		},
		"Basic for a Bearer token": {
			models.AuthMechanismBasic,
			models.ProxySecret{Bearer: new(models.NewSecret(token))},
			validate.ProxyConfigAuthMechanismMismatch,
		},
		"Bearer for headers": {
			models.AuthMechanismBearer,
			models.ProxySecret{Headers: headers},
			validate.ProxyConfigAuthMechanismMismatch,
		},
		"Headers for a Basic credential": {
			models.AuthMechanismHeaders,
			models.ProxySecret{Basic: new(models.NewSecret(basic))},
			validate.ProxyConfigAuthMechanismMismatch,
		},
	} {
		t.Run(name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			c := projectClient(t, api)
			ctx := t.Context()
			_, createErr := c.ProxyConfigs.Create(ctx, models.ProxyConfigCreate{
				Key: "k", Name: "n", AuthMechanism: tc.mechanism, Secret: tc.secret,
			})
			_, updateErr := c.ProxyConfigs.Update(ctx, "k", models.ProxyConfigUpdate{
				AuthMechanism: tc.mechanism, Secret: tc.secret,
			})
			setting := "secret"
			if tc.rule == validate.ProxyConfigAuthMechanismMismatch {
				setting = "auth_mechanism"
			}
			for op, err := range map[string]error{
				opCreateProxyConfig: createErr, opUpdateProxyConfig: updateErr,
			} {
				want := ErrInvalidConfig.Error() + ": " + op + ": " + setting + ": " +
					tc.rule.Message
				if !errors.Is(err, ErrInvalidConfig) || err.Error() != want {
					t.Errorf("%s returned %v, want\n%s", op, err, want)
				}
			}
			api.AssertUntouched()
			for _, value := range []string{basic, token, header} {
				if c.secrets.Contains(value) {
					t.Errorf("the credential %s of a secret that was not sent joined the Set",
						value)
				}
			}
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
			_, createErr = c.ProxyConfigs.Create(ctx, models.ProxyConfigCreate{Secret: tc.secret})
			_, updateErr = c.ProxyConfigs.Update(ctx, "k",
				models.ProxyConfigUpdate{Secret: tc.secret})
			if !errors.Is(createErr, ErrClosed) || !errors.Is(updateErr, ErrClosed) {
				t.Errorf("after Close, Create returned %v and Update %v, want ErrClosed",
					createErr, updateErr)
			}
		})
	}
}

// TestProxyConfigErrorsHoldNoSecret checks that a 422 to Create or Update whose body echoes the
// secret of the request, of each auth mechanism, leaves no credential in the error: not in its
// Body, its Detail, its Title or its text, nor in the client's Debug line of the call, each of
// which shows the marker instead. The body echoes the secret as the API's validation errors do,
// in the input of an error whose loc names the secret, or of one whose loc names the whole body;
// or as text, in the message of a Permit error or in a detail that is a string. The credentials
// are not in the shape of a Permit API key: the field secret, and the client's Set, to which the
// call adds the credentials that it sends, hide them.
func TestProxyConfigErrorsHoldNoSecret(t *testing.T) {
	secrets := map[string]struct {
		secret models.ProxySecret
		sent   string
		values []string
	}{
		"Basic": {
			models.ProxySecret{Basic: new(models.NewSecret(proxyBasicValue))},
			`"` + proxyBasicValue + `"`,
			[]string{proxyBasicValue},
		},
		"Bearer": {
			bearerSecret(proxyBearerValue), `"` + proxyBearerValue + `"`,
			[]string{proxyBearerValue},
		},
		"Headers": {headersSecret, headersJSON, []string{proxyHeaderValue, proxyRegionValue}},
	}
	echoes := map[string]func(sent string, values []string) string{
		"the input of the secret": func(sent string, _ []string) string {
			return `{"detail":[{"loc":["body","secret"],"msg":"value is not valid",` +
				`"type":"value_error","input":` + sent + `}]}`
		},
		"the input of the body": func(sent string, _ []string) string {
			return `{"detail":[{"loc":["body"],"msg":"the body is not valid",` +
				`"type":"value_error","input":{"key":"k","secret":` + sent + `}}]}`
		},
		"a message": func(_ string, values []string) string {
			return `{"error_code":"VALIDATION_ERROR","title":"Unprocessable",` +
				`"message":"the credential ` + strings.Join(values, " and ") + ` is not valid"}`
		},
		"a detail": func(_ string, values []string) string {
			return `{"detail":"invalid credentials: ` + strings.Join(values, ", ") + `"}`
		},
	}
	calls := map[string]struct {
		route string
		call  func(ctx context.Context, c *Client, secret models.ProxySecret) error
	}{
		"Create": {createProxyConfigRoute, func(ctx context.Context, c *Client,
			secret models.ProxySecret,
		) error {
			_, err := c.ProxyConfigs.Create(ctx, models.ProxyConfigCreate{
				Key: "k", Name: "n", Secret: secret,
			})
			return err
		}},
		"Update": {updateProxyConfigRoute, func(ctx context.Context, c *Client,
			secret models.ProxySecret,
		) error {
			_, err := c.ProxyConfigs.Update(ctx, "k", models.ProxyConfigUpdate{Secret: secret})
			return err
		}},
	}
	for secretName, s := range secrets {
		for echoName, echo := range echoes {
			for callName, m := range calls {
				t.Run(secretName+"/"+echoName+"/"+callName, func(t *testing.T) {
					logger, logged := debugLog()
					api := mockpermit.NewAPI(t)
					api.Handle(m.route, mockpermit.Reply{
						Status: http.StatusUnprocessableEntity, Body: echo(s.sent, s.values),
					})
					c := resourceClient(t, api, WithLogger(logger))
					err := m.call(t.Context(), c, s.secret)
					e := mustError(t, err)
					if e.StatusCode != http.StatusUnprocessableEntity ||
						!bytes.Contains(e.Body, []byte(redact.Marker)) {
						t.Errorf("the call returned %+v, want the 422 with the marker in its body",
							err)
					}
					scanner := canary.NewScanner(s.values...)
					_, _ = fmt.Fprintf(scanner, "%v %+v %#v %s %s %s %s", err, err, e, e.Body,
						e.Detail, e.Title, e.Error())
					_, _ = scanner.Write(logged())
					scanner.Check(t)
				})
			}
		}
	}
}

// TestMalformedProxyConfigReplies checks that a reply in 200-299 that is not a config, such as one
// whose secret is a number, a list or an object of anything but strings, or for List and All not
// a JSON array of configs, is an *Error that names the model and holds nothing of the body, and
// no config, no page and no config of the iterator; and that the client's Debug lines of these
// calls hold no credential of the bodies.
func TestMalformedProxyConfigReplies(t *testing.T) {
	const prefix = "permit: "
	logger, logged := debugLog()
	for _, body := range []string{
		``, `null`, `[]`, `"opaque-string-value"`, `{"secret":5}`, `{} {}`,
		`{"key":"k","secret":["opaque-listed-value"]}`,
		`{"key":"k","secret":{"X-A":{"v":"opaque-nested-value"}}}`,
		`{"key":"k","secret":{"X-A":"opaque-header-value","X-B":1}}`,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(getProxyConfigRoute, mockpermit.Reply{Body: body})
		c := resourceClient(t, api, WithLogger(logger))
		config, err := c.ProxyConfigs.Get(t.Context(), "payments")
		want := prefix + "get_proxy_config: GET /v2/facts/{proj_id}/{env_id}/proxy_configs/" +
			"{proxy_config_id}: status 200: the reply is not a valid models.ProxyConfigRead"
		if e := mustError(t, err); config != nil || e.Error() != want || e.Body != nil {
			t.Errorf("Get of the reply %q returned %+v and\n%v\nwant no config and\n%s", body,
				config, err, want)
		}
		if strings.Contains(fmt.Sprintf("%+v %#v", err, err), "opaque") {
			t.Errorf("the error of the reply %q holds a part of its body: %+v", body, err)
		}
	}
	for _, body := range []string{
		``, `null`, `"a"`, `[1]`, `{"data":[]}`, `[{"key":"k","secret":5}]`,
		`[{"key":"k","secret":{"X-A":["opaque-listed-value"]}}]`,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(listProxyConfigsRoute, mockpermit.Reply{Body: body},
			mockpermit.Reply{Body: body})
		c := resourceClient(t, api, WithLogger(logger))
		page, err := c.ProxyConfigs.List(t.Context(), nil)
		want := prefix + "list_proxy_configs: GET /v2/facts/{proj_id}/{env_id}/proxy_configs: " +
			"status 200: the reply is not a valid list of models.ProxyConfigRead"
		if e := mustError(t, err); page != nil || e.Error() != want || e.Body != nil {
			t.Errorf("List of the reply %q returned %+v and\n%v\nwant no page and\n%s", body,
				page, err, want)
		}
		items, errs := collect(t, c.ProxyConfigs.All(t.Context(), nil))
		if items != nil || len(errs) != 1 || mustError(t, errs[0]).Error() != want {
			t.Errorf("All of the reply %q yielded %+v and %v, want only the error\n%s", body,
				items, errs, want)
		}
	}
	if bytes.Contains(logged(), []byte("opaque")) {
		t.Errorf("the Debug lines hold a credential of a reply:\n%s", logged())
	}
}

// TestProxyConfigSecretsJoinTheClientSet checks that the credentials of the secrets that Get and
// List return join the client's Set: the value of each header of a secret of headers, which only
// the field secret of the operations' SensitiveBodyFields adds, as the credential name secret
// adds a string under it and not the strings of an object under it, and the Basic credential and
// the Bearer token. A later error of the client whose body echoes each credential outside a
// credential field, as a message can, holds the marker in its place, in its body, its Detail and
// its text, and so does the client's Debug line of that error. A client that called nothing does
// not hold them.
func TestProxyConfigSecretsJoinTheClientSet(t *testing.T) {
	const (
		gotHeader    = "opaque-got-header"
		gotRegion    = "opaque-got-region"
		listedBasic  = "opaque-listed-user:opaque-listed-pass"
		listedBearer = "opaque-listed-bearer"
	)
	values := []string{gotHeader, gotRegion, listedBasic, listedBearer}
	echo := `{"detail":"the credentials ` + strings.Join(values, ", ") + ` are revoked"}`
	logger, logged := debugLog()
	api := mockpermit.NewAPI(t)
	api.Handle(getProxyConfigRoute, mockpermit.Reply{Body: proxyConfigBody("payments", "Headers",
		`{"X-Api-Key":"`+gotHeader+`","X-Region":"`+gotRegion+`"}`)},
		mockpermit.Reply{Status: http.StatusBadRequest, Body: echo})
	api.Handle(listProxyConfigsRoute, mockpermit.Reply{
		Body: "[" + proxyConfigBody("ledger", "Basic", `"`+listedBasic+`"`) + "," +
			proxyConfigBody("billing", "Bearer", `"`+listedBearer+`"`) + "]",
	})
	c := resourceClient(t, api, WithLogger(logger))
	for _, value := range values {
		if c.secrets.Contains(value) {
			t.Fatalf("the Set of a new client holds %s", value)
		}
	}
	ctx := t.Context()
	got, err := c.ProxyConfigs.Get(ctx, "payments")
	if err != nil {
		t.Fatal(err)
	}
	page, err := c.ProxyConfigs.List(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	read := []string{
		got.Secret.Headers["X-Api-Key"].Value(), got.Secret.Headers["X-Region"].Value(),
		page.Data[0].Secret.Basic.Value(), page.Data[1].Secret.Bearer.Value(),
	}
	if diff := cmp.Diff(values, read); diff != "" {
		t.Fatalf("the calls returned credentials that differ (-want +got):\n%s", diff)
	}
	for _, value := range values {
		if !c.secrets.Contains(value) {
			t.Errorf("the credential %s that a call returned did not join the client's Set",
				value)
		}
	}
	_, err = c.ProxyConfigs.Get(ctx, "payments")
	e := mustError(t, err)
	if e.StatusCode != http.StatusBadRequest || strings.Count(e.Detail, redact.Marker) != 4 ||
		!bytes.Contains(e.Body, []byte(redact.Marker)) {
		t.Errorf("the 400 returned %#v, want its detail and body with the marker in place of "+
			"each credential", e)
	}
	scanner := canary.NewScanner(values...)
	_, _ = fmt.Fprintf(scanner, "%+v %#v %s %s", err, e, e.Body, e.Detail)
	_, _ = scanner.Write(logged())
	scanner.Check(t)
	if !bytes.Contains(logged(), []byte(redact.Marker)) {
		t.Errorf("the Debug line of the 400 does not show the marker:\n%s", logged())
	}
}

// TestProxyConfigCallsLogNoSecret checks that a recording logger at Debug sees no credential
// while Create, Get and List run, neither in the client's Debug lines nor in what the
// application logs of the configs that the calls return, with slog's text and JSON handlers,
// which show the marker in place of each credential; and that fmt's %v and %+v of a config that
// a call returns show the marker, and no credential.
func TestProxyConfigCallsLogNoSecret(t *testing.T) {
	var text, jsonLog bytes.Buffer
	textLogger := slog.New(slog.NewTextHandler(&text, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	jsonLogger := slog.New(slog.NewJSONHandler(&jsonLog, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	values := []string{proxyHeaderValue, proxyRegionValue, proxyBasicValue, proxyBearerValue}
	printed := canary.NewScanner(values...)
	for _, logger := range []*slog.Logger{textLogger, jsonLogger} {
		api := mockpermit.NewAPI(t)
		api.Handle(createProxyConfigRoute,
			mockpermit.Reply{Body: proxyConfigBody("payments", "Headers", headersJSON)})
		api.Handle(getProxyConfigRoute, mockpermit.Reply{
			Body: proxyConfigBody("ledger", "Basic", `"`+proxyBasicValue+`"`),
		})
		api.Handle(listProxyConfigsRoute, mockpermit.Reply{Body: "[" + bearerConfigBody + "]"})
		c := resourceClient(t, api, WithLogger(logger))
		ctx := t.Context()
		created, err := c.ProxyConfigs.Create(ctx, models.ProxyConfigCreate{
			Key: "payments", Name: "payments", AuthMechanism: models.AuthMechanismHeaders,
			Secret: headersSecret,
		})
		if err != nil {
			t.Fatal(err)
		}
		got, err := c.ProxyConfigs.Get(ctx, "ledger")
		if err != nil {
			t.Fatal(err)
		}
		page, err := c.ProxyConfigs.List(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		logger.DebugContext(ctx, "the application's configs", "created", created, "got", *got,
			"listed", page.Data)
		for _, format := range []string{"%v", "%+v"} {
			out := fmt.Sprintf(format, *got)
			_, _ = printed.Write([]byte(out))
			if !strings.Contains(out, redact.Marker) {
				t.Errorf("Sprintf(%q) of the config = %s, want the marker", format, out)
			}
		}
	}
	printed.Check(t)
	for name, out := range map[string][]byte{"text": text.Bytes(), "JSON": jsonLog.Bytes()} {
		scanner := canary.NewScanner(values...)
		_, _ = scanner.Write(out)
		scanner.Check(t)
		if n := bytes.Count(out, []byte(redact.Marker)); n != 4 {
			t.Errorf("the %s log shows the marker %d times, want it in place of each of the "+
				"4 credentials that the application logged:\n%s", name, n, out)
		}
	}
}

// TestProxyConfigRetries checks when a call of the proxy configs sends its request again: Create
// and Update, which the API may have acted on, after a 429 only, so a 503 ends each after one
// request; Get, Delete and List, which do the same when sent twice, after a 503 too.
func TestProxyConfigRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	ok := mockpermit.Reply{Body: bearerConfigBody}
	create := func(c *Client) error { return createProxyConfig(t.Context(), c) }
	update := func(c *Client) error { return proxyConfigCalls[1].call(t.Context(), c, "payments") }
	type retryCase struct {
		name    string
		route   string
		replies []mockpermit.Reply
		call    func(c *Client) error
		status  int
	}
	tests := []retryCase{
		{
			name: "Create after a 429", route: createProxyConfigRoute,
			replies: []mockpermit.Reply{limited, ok}, call: create,
		},
		{
			name: "Create after a 503", route: createProxyConfigRoute,
			replies: []mockpermit.Reply{unavailable}, call: create,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "Update after a 429", route: updateProxyConfigRoute,
			replies: []mockpermit.Reply{limited, ok}, call: update,
		},
		{
			name: "Update after a 503", route: updateProxyConfigRoute,
			replies: []mockpermit.Reply{unavailable}, call: update,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "List after a 503", route: listProxyConfigsRoute,
			replies: []mockpermit.Reply{unavailable, {Body: `[]`}},
			call: func(c *Client) error {
				_, err := c.ProxyConfigs.List(t.Context(), nil)
				return err
			},
		},
	}
	for _, m := range []int{0, 2} {
		call := proxyConfigCalls[m]
		tests = append(tests, retryCase{
			name: call.name + " after a 503", route: call.route,
			replies: []mockpermit.Reply{unavailable, call.reply},
			call:    func(c *Client) error { return call.call(t.Context(), c, "payments") },
		})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, tc.replies...)
			err := tc.call(resourceClient(t, api, WithRetry(RetryPolicy{MaxRetries: 1})))
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

// TestProxyConfigsAfterClose checks that each method returns ErrClosed after Close, and sends
// nothing, also without WithScope, whose lookup it does not send either.
func TestProxyConfigsAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	_, listErr := c.ProxyConfigs.List(ctx, nil)
	_, allErrs := collect(t, c.ProxyConfigs.All(ctx, nil))
	errs := []error{listErr, createProxyConfig(ctx, c)}
	for _, m := range proxyConfigCalls {
		errs = append(errs, m.call(ctx, c, "payments"))
	}
	for i, err := range append(errs, allErrs...) {
		if !errors.Is(err, ErrClosed) {
			t.Errorf("call %d after Close returned %v, want ErrClosed", i, err)
		}
	}
	if len(allErrs) != 1 {
		t.Errorf("All after Close yielded %v, want ErrClosed once", allErrs)
	}
	api.AssertUntouched()
}
