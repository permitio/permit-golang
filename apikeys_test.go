package permit

import (
	"bytes"
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
	"github.com/permitio/permit-golang/v2/models"
)

// The routes of the API keys, as mockpermit.Server.Handle takes them.
const (
	environmentKeyRoute = "GET /v2/api-key/{proj_id}/{env_id}"
	listAPIKeysRoute    = "GET /v2/api-key"
	createAPIKeyRoute   = "POST /v2/api-key"
	getAPIKeyRoute      = "GET /v2/api-key/{api_key_id}"
	deleteAPIKeyRoute   = "DELETE /v2/api-key/{api_key_id}"
	rotateAPIKeyRoute   = "POST /v2/api-key/{api_key_id}/rotate-secret"
)

// The credentials of the replies of the tests of the redaction. They are not in the shape of a
// Permit API key, which the client hides whatever it holds, so only the client's Set hides them.
const (
	environmentValue = "opaque-environment-value"
	createdValue     = "opaque-created-value"
	rotatedValue     = "opaque-rotated-value"
)

// apiKeyBody returns the body of a reply of the API that holds the API key id, with value as its
// secret, or without a secret when value is empty.
func apiKeyBody(id, value string) string {
	body := `{"organization_id":"00000000-0000-4000-8000-000000000001","owner_type":"member",` +
		`"id":"` + id + `","created_at":"2026-10-15T11:00:00Z"`
	if value != "" {
		body += `,"secret":"` + value + `"`
	}
	return body + "}"
}

// apiKeysOf returns the API keys k<from> to k<to-1>, with only an ID and a name: an empty list
// when to is from.
func apiKeysOf(from, to int) []models.APIKeyRead {
	items := make([]models.APIKeyRead, 0, to-from)
	for i := from; i < to; i++ {
		id := "k" + strconv.Itoa(i)
		items = append(items, models.APIKeyRead{ID: id, Name: id})
	}
	return items
}

// TestAPIKeysAllReadsEveryPage checks that APIKeysService.All reads the 31 keys of a list, in
// order, asking for each page in turn: up to the last page that the replies count, at a per_page
// of 30 and at the default per_page of 100, the most that the API gives on a page; up to the
// empty page when the replies do not count the pages; and from page 2 on, when the options start
// there, with the filters of the options in each request.
func TestAPIKeysAllReadsEveryPage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    *APIKeysListOptions
		replies []mockpermit.Reply
		want    []models.APIKeyRead
		queries []string
	}{
		{
			name: "per_page 30",
			opts: &APIKeysListOptions{ListOptions: ListOptions{PerPage: 30}},
			replies: []mockpermit.Reply{
				pageOf(t, apiKeysOf(0, 30), `,"total_count":31,"page_count":2`),
				pageOf(t, apiKeysOf(30, 31), `,"total_count":31,"page_count":2`),
			},
			want:    apiKeysOf(0, 31),
			queries: []string{"page=1&per_page=30", "page=2&per_page=30"},
		},
		{
			name: "the default per_page",
			replies: []mockpermit.Reply{
				pageOf(t, apiKeysOf(0, 31), `,"total_count":31,"page_count":1`),
			},
			want:    apiKeysOf(0, 31),
			queries: []string{"page=1&per_page=100"},
		},
		{
			name: "pages without page_count",
			opts: &APIKeysListOptions{ListOptions: ListOptions{PerPage: 30}},
			replies: []mockpermit.Reply{
				pageOf(t, apiKeysOf(0, 30), `,"total_count":31`),
				pageOf(t, apiKeysOf(30, 31), `,"total_count":31`),
				pageOf(t, apiKeysOf(0, 0), `,"total_count":31`),
			},
			want: apiKeysOf(0, 31),
			queries: []string{
				"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30",
			},
		},
		{
			name: "from page 2 with filters",
			opts: &APIKeysListOptions{
				ListOptions: ListOptions{Page: 2, PerPage: 30},
				ObjectType:  models.MemberAccessObjProject, Project: "my-project",
			},
			replies: []mockpermit.Reply{
				pageOf(t, apiKeysOf(30, 31), `,"total_count":31,"page_count":2`),
			},
			want: apiKeysOf(30, 31),
			queries: []string{
				"object_type=project&page=2&per_page=30&proj_id=my-project",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listAPIKeysRoute, tc.replies...)
			items, errs := collect(t, projectClient(t, api).APIKeys.All(t.Context(), tc.opts))
			if diff := cmp.Diff(tc.want, items); diff != "" || errs != nil {
				t.Errorf("All yielded the errors %v and keys that differ (-want +got):\n%s", errs,
					diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestAPIKeysAllReadsTheOptionsOnce checks that the iterator keeps the options of its call, with
// their filters, so that a later change to them changes no request, that a loop that breaks asks
// for no other page, and that each loop over the iterator reads the keys again from the first
// page.
func TestAPIKeysAllReadsTheOptionsOnce(t *testing.T) {
	api := mockpermit.NewAPI(t)
	two := pageOf(t, apiKeysOf(0, 2), `,"total_count":4,"page_count":2`)
	api.Handle(listAPIKeysRoute, two, two)
	opts := &APIKeysListOptions{ListOptions: ListOptions{PerPage: 2}, Project: "p"}
	seq := projectClient(t, api).APIKeys.All(t.Context(), opts)
	opts.Page, opts.PerPage, opts.Project = 5, 50, "other"
	opts.ObjectType = models.MemberAccessObjOrg
	for loop := 1; loop <= 2; loop++ {
		for key, err := range seq {
			if err != nil || key.ID != "k0" {
				t.Errorf("loop %d yielded %+v and %v first, want k0", loop, key, err)
			}
			break
		}
	}
	first := "page=1&per_page=2&proj_id=p"
	if diff := cmp.Diff([]string{first, first}, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestAPIKeysAllYieldsAnErrorOnce checks that a page that fails ends the iterator with its error,
// once, after the keys of the pages before it.
func TestAPIKeysAllYieldsAnErrorOnce(t *testing.T) {
	const body = `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no"}`
	api := mockpermit.NewAPI(t)
	api.Handle(listAPIKeysRoute, pageOf(t, apiKeysOf(0, 30), `,"total_count":60`),
		mockpermit.Reply{Status: http.StatusForbidden, Body: body})
	items, errs := collect(t, projectClient(t, api).APIKeys.All(t.Context(),
		&APIKeysListOptions{ListOptions: ListOptions{PerPage: 30}}))
	if len(items) != 30 || len(errs) != 1 || !errors.Is(errs[0], ErrForbidden) {
		t.Errorf("All yielded %d keys and the errors %v, want 30 and one error that matches "+
			"ErrForbidden", len(items), errs)
	}
}

// TestAPIKeysListReturnsAPage checks the page that List returns: the keys of the reply in its
// order, the page and the per_page that it asked for, page 1 and no query for nil options, the
// counts that the reply reports, and each filter that is not empty in the query, escaped. A page
// without keys has an empty list, not nil.
func TestAPIKeysListReturnsAPage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  *APIKeysListOptions
		reply mockpermit.Reply
		query string
		want  *Page[models.APIKeyRead]
	}{
		{
			name: "nil options", reply: pageOf(t, apiKeysOf(0, 2), `,"total_count":2`),
			want: &Page[models.APIKeyRead]{Data: apiKeysOf(0, 2), Page: 1, TotalCount: new(2)},
		},
		{
			name: "empty filters", opts: &APIKeysListOptions{},
			reply: pageOf(t, apiKeysOf(0, 1), `,"total_count":1,"page_count":1`),
			want: &Page[models.APIKeyRead]{
				Data: apiKeysOf(0, 1), Page: 1, TotalCount: new(1), PageCount: new(1),
			},
		},
		{
			name:  "the level",
			opts:  &APIKeysListOptions{ObjectType: models.MemberAccessObjEnv},
			reply: pageOf(t, apiKeysOf(0, 1), `,"total_count":1`), query: "object_type=env",
			want: &Page[models.APIKeyRead]{Data: apiKeysOf(0, 1), Page: 1, TotalCount: new(1)},
		},
		{
			name:  "a project whose key the query escapes",
			opts:  &APIKeysListOptions{Project: "a b&c=d/é"},
			reply: pageOf(t, apiKeysOf(0, 1), `,"total_count":1`),
			query: "proj_id=a+b%26c%3Dd%2F%C3%A9",
			want:  &Page[models.APIKeyRead]{Data: apiKeysOf(0, 1), Page: 1, TotalCount: new(1)},
		},
		{
			name: "a page after the last",
			opts: &APIKeysListOptions{
				ListOptions: ListOptions{Page: 3, PerPage: 30}, ObjectType: "added_later",
			},
			reply: pageOf(t, []models.APIKeyRead{}, `,"total_count":31,"page_count":2`),
			query: "object_type=added_later&page=3&per_page=30",
			want: &Page[models.APIKeyRead]{
				Data: []models.APIKeyRead{}, Page: 3, PerPage: 30, TotalCount: new(31),
				PageCount: new(2),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listAPIKeysRoute, tc.reply)
			got, err := projectClient(t, api).APIKeys.List(t.Context(), tc.opts)
			if err != nil {
				t.Fatalf("List returned %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" || got.Data == nil {
				t.Errorf("List returned a page that differs (-want +got), or nil Data:\n%s", diff)
			}
			if diff := cmp.Diff([]string{tc.query}, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestInvalidAPIKeyListOptions checks that a Page or a PerPage less than zero is an error that
// matches ErrInvalidConfig from List and All, and that neither sends a request.
func TestInvalidAPIKeyListOptions(t *testing.T) {
	for _, opts := range []*APIKeysListOptions{
		{ListOptions: ListOptions{Page: -1}}, {ListOptions: ListOptions{PerPage: -1}},
	} {
		api := mockpermit.NewAPI(t)
		c := projectClient(t, api)
		_, err := c.APIKeys.List(t.Context(), opts)
		_, errs := collect(t, c.APIKeys.All(t.Context(), opts))
		for _, err := range append([]error{err}, errs...) {
			if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("%+v: the call returned %v, want an error that matches ErrInvalidConfig",
					*opts, err)
			}
		}
		if len(errs) != 1 {
			t.Errorf("%+v: All yielded the errors %v, want one", *opts, errs)
		}
		api.AssertUntouched()
	}
}

// TestAPIKeysNeedNoScope checks that the calls of the API keys look up no scope and check none:
// a client without WithScope sends no request but the call's, as the API fails each request to a
// route that the test did not register, such as the lookup of the scope; and GetEnvironmentKey
// sends the project and the environment of its parameters, not those of WithScope.
func TestAPIKeysNeedNoScope(t *testing.T) {
	for _, opts := range [][]Option{nil, {WithScope("scope-project", "scope-environment")}} {
		api := mockpermit.NewAPI(t)
		key := mockpermit.Reply{Body: apiKeyBody("k", "")}
		api.Handle(environmentKeyRoute, key)
		api.Handle(listAPIKeysRoute, pageOf(t, apiKeysOf(0, 0), `,"total_count":0`),
			pageOf(t, apiKeysOf(0, 0), `,"total_count":0`))
		api.Handle(getAPIKeyRoute, key)
		api.Handle(createAPIKeyRoute, key)
		api.Handle(deleteAPIKeyRoute, mockpermit.Reply{Status: http.StatusNoContent})
		api.Handle(rotateAPIKeyRoute, key)
		c := projectClient(t, api, opts...)
		ctx := t.Context()
		if _, err := c.APIKeys.GetEnvironmentKey(ctx, "p", "e"); err != nil {
			t.Errorf("GetEnvironmentKey returned %v", err)
		}
		if _, err := c.APIKeys.List(ctx, nil); err != nil {
			t.Errorf("List returned %v", err)
		}
		if _, errs := collect(t, c.APIKeys.All(ctx, nil)); errs != nil {
			t.Errorf("All yielded %v", errs)
		}
		if _, err := c.APIKeys.Get(ctx, "k"); err != nil {
			t.Errorf("Get returned %v", err)
		}
		if _, err := c.APIKeys.Create(ctx, models.APIKeyCreate{OrganizationID: "o"}); err != nil {
			t.Errorf("Create returned %v", err)
		}
		if err := c.APIKeys.Delete(ctx, "k"); err != nil {
			t.Errorf("Delete returned %v", err)
		}
		if _, err := c.APIKeys.RotateSecret(ctx, "k"); err != nil {
			t.Errorf("RotateSecret returned %v", err)
		}
		const first = "/v2/api-key/p/e"
		if requests := api.Requests(); len(requests) != 7 || requests[0].RawPath != first {
			t.Errorf("the calls sent %+v, want 7 requests, the first to %s", requests, first)
		}
	}
}

// TestAPIKeyIDsInThePath checks that each method that names an API key sends its ID as one
// segment of the path, escaped, whatever the ID holds: a slash, a percent sign, a space, a
// question mark and a hash, a letter outside ASCII, the shape of a UUID, a leading dot, and a
// word that only looks like scope; and that GetEnvironmentKey sends the key of the project and
// that of the environment so too. An ID or a key that is empty, . or .. would change the route,
// so it is an error that matches ErrInvalidConfig and names the placeholder, and no request. The
// API key ID scope would reach the route of the scope of the API key, so it is such an error
// too, which says so, for each method that names an API key; GetEnvironmentKey sends the key
// scope as it is, as its route has two segments after /v2/api-key.
func TestAPIKeyIDsInThePath(t *testing.T) {
	key := mockpermit.Reply{Body: apiKeyBody("k", "")}
	methods := []struct {
		name, route, placeholder, prefix, suffix string
		reply                                    mockpermit.Reply
		call                                     func(c *Client, id string) error
	}{
		{
			name: "Get", route: getAPIKeyRoute, placeholder: "{api_key_id}",
			prefix: "/v2/api-key/", reply: key,
			call: func(c *Client, id string) error {
				_, err := c.APIKeys.Get(t.Context(), id)
				return err
			},
		},
		{
			name: "Delete", route: deleteAPIKeyRoute, placeholder: "{api_key_id}",
			prefix: "/v2/api-key/", reply: mockpermit.Reply{Status: http.StatusNoContent},
			call: func(c *Client, id string) error { return c.APIKeys.Delete(t.Context(), id) },
		},
		{
			name: "RotateSecret", route: rotateAPIKeyRoute, placeholder: "{api_key_id}",
			prefix: "/v2/api-key/", suffix: "/rotate-secret", reply: key,
			call: func(c *Client, id string) error {
				_, err := c.APIKeys.RotateSecret(t.Context(), id)
				return err
			},
		},
		{
			name: "GetEnvironmentKey of the project", route: environmentKeyRoute,
			placeholder: "{proj_id}", prefix: "/v2/api-key/", suffix: "/e", reply: key,
			call: func(c *Client, project string) error {
				_, err := c.APIKeys.GetEnvironmentKey(t.Context(), project, "e")
				return err
			},
		},
		{
			name: "GetEnvironmentKey of the environment", route: environmentKeyRoute,
			placeholder: "{env_id}", prefix: "/v2/api-key/p/", reply: key,
			call: func(c *Client, environment string) error {
				_, err := c.APIKeys.GetEnvironmentKey(t.Context(), "p", environment)
				return err
			},
		},
	}
	for id, escaped := range map[string]string{
		"a/b":            "a%2Fb",
		"50%":            "50%25",
		"a b?c#d":        "a%20b%3Fc%23d",
		"é":              "%C3%A9",
		fixtures.UUIDKey: fixtures.UUIDKey,
		".hidden":        ".hidden",
		"key.v2..x":      "key.v2..x",
		"Scope":          "Scope",
		"scopes":         "scopes",
		"scope/":         "scope%2F",
	} {
		for _, m := range methods {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, m.reply)
			if err := m.call(projectClient(t, api), id); err != nil {
				t.Errorf("%s(%q) returned %v", m.name, id, err)
			}
			want := m.prefix + escaped + m.suffix
			if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want {
				t.Errorf("%s(%q) sent %+v, want one request to %s", m.name, id, requests, want)
			}
		}
	}
	for _, id := range []string{"", ".", ".."} {
		for _, m := range methods {
			api := mockpermit.NewAPI(t)
			err := m.call(projectClient(t, api), id)
			if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), m.placeholder) {
				t.Errorf("%s(%q) returned %v, want an error that matches ErrInvalidConfig and "+
					"names %s", m.name, id, err, m.placeholder)
			}
			api.AssertUntouched()
		}
	}
	for _, m := range methods {
		api := mockpermit.NewAPI(t)
		keyID := m.placeholder == "{api_key_id}"
		if !keyID {
			api.Handle(m.route, m.reply)
		}
		err := m.call(projectClient(t, api), "scope")
		if !keyID {
			want := m.prefix + "scope" + m.suffix
			if requests := api.Requests(); err != nil || len(requests) != 1 ||
				requests[0].RawPath != want {
				t.Errorf("%s(%q) returned %v and sent %+v, want one request to %s", m.name,
					"scope", err, requests, want)
			}
			continue
		}
		if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), m.placeholder) ||
			!strings.Contains(err.Error(), "names the route of the scope of the API key") {
			t.Errorf("%s(%q) returned %v, want an error that matches ErrInvalidConfig, names %s "+
				"and says that scope names the route of the scope of the API key", m.name,
				"scope", err, m.placeholder)
		}
		api.AssertUntouched()
	}
}

// TestDeleteAPIKeyThatIsGone checks that Delete returns nil for a 404, with or without a body,
// as the key that the path names is gone already, after one request; and an *Error with the
// status for any other error status, such as 403, 409 or 422.
func TestDeleteAPIKeyThatIsGone(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"NOT_FOUND","title":"Not Found","message":"the API key does not exist"}`,
		"",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteAPIKeyRoute, mockpermit.Reply{Status: http.StatusNotFound, Body: body})
		if err := projectClient(t, api).APIKeys.Delete(t.Context(), "gone"); err != nil {
			t.Errorf("Delete of a key that the API answers with 404 and the body %q returned "+
				"%v, want nil", body, err)
		}
		if n := len(api.Requests()); n != 1 {
			t.Errorf("Delete sent %d requests, want 1", n)
		}
	}
	for _, status := range []int{
		http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(deleteAPIKeyRoute, mockpermit.Reply{Status: status, Body: `{"detail":"no"}`})
		err := projectClient(t, api).APIKeys.Delete(t.Context(), "k")
		if e := mustError(t, err); e.StatusCode != status || e.Op != opDeleteAPIKey ||
			e.Detail != "no" || errors.Is(err, ErrNotFound) {
			t.Errorf("Delete of a reply with status %d returned %v, want the *Error of %s with "+
				"the status and the detail", status, err, opDeleteAPIKey)
		}
	}
}

// TestNotFoundOfOtherAPIKeyCalls checks that a 404 is an *Error that matches ErrNotFound for
// each call of the API keys that does not delete, as the path of none of them names an object
// that the call removes.
func TestNotFoundOfOtherAPIKeyCalls(t *testing.T) {
	notFound := mockpermit.Reply{Status: http.StatusNotFound, Body: `{"detail":"no"}`}
	for _, tc := range []struct {
		route string
		call  func(c *Client) error
	}{
		{environmentKeyRoute, func(c *Client) error {
			_, err := c.APIKeys.GetEnvironmentKey(t.Context(), "p", "e")
			return err
		}},
		{getAPIKeyRoute, func(c *Client) error {
			_, err := c.APIKeys.Get(t.Context(), "k")
			return err
		}},
		{rotateAPIKeyRoute, func(c *Client) error {
			_, err := c.APIKeys.RotateSecret(t.Context(), "k")
			return err
		}},
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(tc.route, notFound)
		if err := tc.call(projectClient(t, api)); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s answered 404 returned %v, want an error that matches ErrNotFound",
				tc.route, err)
		}
	}
}

// TestAPIKeyCreateBodies checks the bodies of Create byte for byte against the golden files in
// testdata/golden: a key with the required field only, and one with every field and a name that
// an encoder may escape.
func TestAPIKeyCreateBodies(t *testing.T) {
	for _, tc := range []struct {
		golden string
		key    models.APIKeyCreate
	}{
		{"create_api_key.required.golden", models.APIKeyCreate{OrganizationID: fixtures.UUIDKey}},
		{
			"create_api_key.every-field.golden", models.APIKeyCreate{
				OrganizationID: fixtures.UUIDKey, ProjectID: "p", EnvironmentID: "e",
				ObjectType: models.MemberAccessObjEnv, AccessLevel: models.MemberAccessLevelWrite,
				OwnerType: models.APIKeyOwnerTypeElements,
				Name:      "<K> & " + fixtures.LineSeparator + fixtures.ParagraphSeparator,
			},
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(createAPIKeyRoute, mockpermit.Reply{Body: apiKeyBody("k", "")})
			if _, err := projectClient(t, api).APIKeys.Create(t.Context(), tc.key); err != nil {
				t.Fatalf("Create returned %v", err)
			}
			requests := api.Requests()
			if len(requests) != 1 {
				t.Fatalf("Create sent %d requests, want 1", len(requests))
			}
			requests[0].ExpectBytes(t, filepath.Join("testdata", "golden", tc.golden))
		})
	}
}

// TestAPIKeyRetries checks when a call of the API keys sends its request again, with the default
// policy of 3 retries: RotateSecret never, so a 429, a 503 and a failed connection each end it
// after one request; Create, which the API may have acted on, after a 429 only; and
// GetEnvironmentKey, List, Get and Delete, which do the same when sent twice, after a 503 and a
// failed connection too.
func TestAPIKeyRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	key := mockpermit.Reply{Body: apiKeyBody("k", "")}
	rotate := func(c *Client) error {
		_, err := c.APIKeys.RotateSecret(t.Context(), "k")
		return err
	}
	create := func(c *Client) error {
		_, err := c.APIKeys.Create(t.Context(), models.APIKeyCreate{OrganizationID: "o"})
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
			name: "RotateSecret after a 503", route: rotateAPIKeyRoute,
			replies: []mockpermit.Reply{unavailable}, call: rotate,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "RotateSecret after a 429", route: rotateAPIKeyRoute,
			replies: []mockpermit.Reply{limited}, call: rotate,
			status: http.StatusTooManyRequests,
		},
		{
			name: "Create after a 429", route: createAPIKeyRoute,
			replies: []mockpermit.Reply{limited, key}, call: create,
		},
		{
			name: "Create after a 503", route: createAPIKeyRoute,
			replies: []mockpermit.Reply{unavailable}, call: create,
			status: http.StatusServiceUnavailable,
		},
		{
			name: "GetEnvironmentKey after a 503", route: environmentKeyRoute,
			replies: []mockpermit.Reply{unavailable, key},
			call: func(c *Client) error {
				_, err := c.APIKeys.GetEnvironmentKey(t.Context(), "p", "e")
				return err
			},
		},
		{
			name: "List after a 503", route: listAPIKeysRoute,
			replies: []mockpermit.Reply{unavailable, {Body: `{"data":[],"total_count":0}`}},
			call: func(c *Client) error {
				_, err := c.APIKeys.List(t.Context(), nil)
				return err
			},
		},
		{
			name: "Get after a 503", route: getAPIKeyRoute,
			replies: []mockpermit.Reply{unavailable, key},
			call: func(c *Client) error {
				_, err := c.APIKeys.Get(t.Context(), "k")
				return err
			},
		},
		{
			name: "Delete after a 503", route: deleteAPIKeyRoute,
			replies: []mockpermit.Reply{unavailable, {Status: http.StatusNoContent}},
			call:    func(c *Client) error { return c.APIKeys.Delete(t.Context(), "k") },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, tc.replies...)
			err := tc.call(projectClient(t, api))
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
	for _, tc := range []struct {
		name  string
		call  func(c *Client) error
		calls int32
	}{
		{"RotateSecret", rotate, 1},
		{"Create", create, 1},
		{"Get", func(c *Client) error {
			_, err := c.APIKeys.Get(t.Context(), "k")
			return err
		}, 2},
	} {
		transport := &countingTransport{}
		api := mockpermit.NewAPI(t)
		err := tc.call(projectClient(t, api, WithHTTPClient(&http.Client{Transport: transport}),
			WithRetry(RetryPolicy{MaxRetries: 1, MaxBackoff: 1})))
		if e := mustError(t, err); e.StatusCode != 0 || transport.requests.Load() != tc.calls {
			t.Errorf("%s over a connection that fails returned %v after %d attempts, want an "+
				"*Error without a status after %d", tc.name, err, transport.requests.Load(),
				tc.calls)
		}
	}
}

// TestMalformedAPIKeyReplies checks that a reply in 200-299 that is not an API key, such as one
// whose secret is not a string, or for List not the paginated object of API keys, is an *Error
// that names the model, and no key; and that the error does not hold the value of the secret.
func TestMalformedAPIKeyReplies(t *testing.T) {
	const prefix = "permit: "
	for _, body := range []string{
		``, `null`, `[]`, `"k"`, `{"id":1}`, `{} {}`, `{"secret":5}`,
		`{"secret":{"value":"opaque-nested-value"}}`, `{"secret":["opaque-listed-value"]}`,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(getAPIKeyRoute, mockpermit.Reply{Body: body})
		key, err := projectClient(t, api).APIKeys.Get(t.Context(), "k")
		want := prefix + "get_api_key: GET /v2/api-key/{api_key_id}: status 200: the reply is " +
			"not a valid models.APIKeyRead"
		if e := mustError(t, err); key != nil || e.Error() != want {
			t.Errorf("Get of the reply %q returned %+v and\n%v\nwant no key and\n%s", body, key,
				err, want)
		}
		if strings.Contains(fmt.Sprintf("%+v", err), "opaque") {
			t.Errorf("the error of the reply %q holds its secret: %+v", body, err)
		}
	}
	for _, body := range []string{
		``, `null`, `[]`, `[{"id":"k"}]`, `{"data":[1]}`, `{"data":[{"secret":5}]}`,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(listAPIKeysRoute, mockpermit.Reply{Body: body})
		page, err := projectClient(t, api).APIKeys.List(t.Context(), nil)
		want := prefix + "list_api_keys: GET /v2/api-key: status 200: the reply is not a valid " +
			"list of models.APIKeyRead"
		if e := mustError(t, err); page != nil || e.Error() != want {
			t.Errorf("List of the reply %q returned %+v and\n%v\nwant no page and\n%s", body,
				page, err, want)
		}
	}
}

// TestAPIKeySecretsJoinTheClientSet checks that the secret of the key that GetEnvironmentKey,
// Create, RotateSecret, Get and List return joins the client's Set: a later error of the client
// whose body echoes each secret outside a credential field, as a message can, holds the marker
// in its place, in its body, its Detail and its text, and so does the client's Debug line of
// that error. The secrets are not in the shape of a Permit API key, so only the Set hides them.
// A client that called nothing does not hold them.
func TestAPIKeySecretsJoinTheClientSet(t *testing.T) {
	const (
		gotValue    = "opaque-got-value"
		listedValue = "opaque-listed-value"
	)
	values := []string{environmentValue, createdValue, rotatedValue, gotValue, listedValue}
	echo := `{"detail":"the keys ` + strings.Join(values, ", ") + ` are revoked"}`
	logger, logged := debugLog()
	api := mockpermit.NewAPI(t)
	api.Handle(environmentKeyRoute, mockpermit.Reply{Body: apiKeyBody("e", environmentValue)})
	api.Handle(createAPIKeyRoute, mockpermit.Reply{Body: apiKeyBody("c", createdValue)})
	api.Handle(rotateAPIKeyRoute, mockpermit.Reply{Body: apiKeyBody("c", rotatedValue)})
	api.Handle(getAPIKeyRoute, mockpermit.Reply{Body: apiKeyBody("g", gotValue)},
		mockpermit.Reply{Status: http.StatusBadRequest, Body: echo})
	api.Handle(listAPIKeysRoute, mockpermit.Reply{
		Body: `{"data":[` + apiKeyBody("l", listedValue) + `],"total_count":1}`,
	})
	c := projectClient(t, api, WithLogger(logger))
	for _, value := range values {
		if c.secrets.Contains(value) {
			t.Fatalf("the Set of a new client holds %s", value)
		}
	}
	ctx := t.Context()
	read := func(key *models.APIKeyRead, err error) string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return key.Secret.Value()
	}
	got := []string{
		read(c.APIKeys.GetEnvironmentKey(ctx, "p", "e")),
		read(c.APIKeys.Create(ctx, models.APIKeyCreate{OrganizationID: "o"})),
		read(c.APIKeys.RotateSecret(ctx, "c")),
		read(c.APIKeys.Get(ctx, "g")),
	}
	page, err := c.APIKeys.List(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, page.Data[0].Secret.Value())
	if diff := cmp.Diff(values, got); diff != "" {
		t.Fatalf("the calls returned secrets that differ (-want +got):\n%s", diff)
	}
	for _, value := range values {
		if !c.secrets.Contains(value) {
			t.Errorf("the secret %s that a call returned did not join the client's Set", value)
		}
	}
	_, err = c.APIKeys.Get(ctx, "g")
	e := mustError(t, err)
	if e.StatusCode != http.StatusBadRequest || strings.Count(e.Detail, redact.Marker) != 5 ||
		!bytes.Contains(e.Body, []byte(redact.Marker)) {
		t.Errorf("the 400 returned %#v, want its detail and body with the marker in place of "+
			"each secret", e)
	}
	scanner := canary.NewScanner(values...)
	_, _ = fmt.Fprintf(scanner, "%+v %#v %s %s", err, e, e.Body, e.Detail)
	_, _ = scanner.Write(logged())
	scanner.Check(t)
	if !bytes.Contains(logged(), []byte(redact.Marker)) {
		t.Errorf("the Debug line of the 400 does not show the marker:\n%s", logged())
	}
}

// TestAPIKeyCallsLogNoSecret checks that a recording logger at Debug sees no secret while Create
// and RotateSecret run: neither in the client's Debug lines, those of a call that fails with a
// body that echoes the secrets among them, nor in what the application logs of the keys that the
// calls return, with slog's text and JSON handlers, which show the marker in place of each
// secret. The secrets are those of the reply fixtures, in the shape of a Permit API key, and the
// opaque ones that only the client's Set hides.
func TestAPIKeyCallsLogNoSecret(t *testing.T) {
	var text, jsonLog bytes.Buffer
	textLogger := slog.New(slog.NewTextHandler(&text, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	jsonLogger := slog.New(slog.NewJSONHandler(&jsonLog, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	values := []string{canary.Key("created"), canary.Key("rotated"), createdValue, rotatedValue}
	echo := `{"detail":"` + strings.Join(values, " ") + `"}`
	for _, logger := range []*slog.Logger{textLogger, jsonLogger} {
		api := mockpermit.NewAPI(t)
		api.Handle(createAPIKeyRoute,
			mockpermit.Reply{Body: replyFixtureOf(t, "create_api_key.200.json")},
			mockpermit.Reply{Body: apiKeyBody("c", createdValue)})
		api.Handle(rotateAPIKeyRoute,
			mockpermit.Reply{Body: replyFixtureOf(t, "rotate_api_key.200.json")},
			mockpermit.Reply{Body: apiKeyBody("c", rotatedValue)},
			mockpermit.Reply{Status: http.StatusConflict, Body: echo})
		c := projectClient(t, api, WithLogger(logger))
		ctx := t.Context()
		for range 2 {
			created, err := c.APIKeys.Create(ctx, models.APIKeyCreate{OrganizationID: "o"})
			if err != nil {
				t.Fatal(err)
			}
			rotated, err := c.APIKeys.RotateSecret(ctx, created.ID)
			if err != nil {
				t.Fatal(err)
			}
			logger.DebugContext(ctx, "the application's keys", "created", created,
				"rotated", *rotated, "credential", rotated.Secret)
		}
		if _, err := c.APIKeys.RotateSecret(ctx, "c"); !errors.Is(err, ErrConflict) {
			t.Fatalf("the third rotation returned %v, want the 409", err)
		}
	}
	for name, out := range map[string][]byte{"text": text.Bytes(), "JSON": jsonLog.Bytes()} {
		scanner := canary.NewScanner(values...)
		_, _ = scanner.Write(out)
		scanner.Check(t)
		if n := bytes.Count(out, []byte(redact.Marker)); n < 6 {
			t.Errorf("the %s log shows the marker %d times, want it in place of each secret "+
				"that the application logged:\n%s", name, n, out)
		}
	}
}

// TestAPIKeysAfterClose checks that each method returns ErrClosed after Close, and sends
// nothing, also for the API key ID scope, which a client that is open rejects.
func TestAPIKeysAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	_, envErr := c.APIKeys.GetEnvironmentKey(ctx, "p", "e")
	_, listErr := c.APIKeys.List(ctx, nil)
	_, allErrs := collect(t, c.APIKeys.All(ctx, nil))
	_, getErr := c.APIKeys.Get(ctx, "k")
	_, createErr := c.APIKeys.Create(ctx, models.APIKeyCreate{OrganizationID: "o"})
	deleteErr := c.APIKeys.Delete(ctx, "k")
	_, rotateErr := c.APIKeys.RotateSecret(ctx, "k")
	_, getScopeErr := c.APIKeys.Get(ctx, "scope")
	deleteScopeErr := c.APIKeys.Delete(ctx, "scope")
	_, rotateScopeErr := c.APIKeys.RotateSecret(ctx, "scope")
	for i, err := range append([]error{
		envErr, listErr, getErr, createErr, deleteErr, rotateErr, getScopeErr, deleteScopeErr,
		rotateScopeErr,
	}, allErrs...) {
		if !errors.Is(err, ErrClosed) {
			t.Errorf("call %d after Close returned %v, want ErrClosed", i, err)
		}
	}
	if len(allErrs) != 1 {
		t.Errorf("All after Close yielded %v, want ErrClosed once", allErrs)
	}
	api.AssertUntouched()
}
