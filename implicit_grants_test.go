package permit

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// implicitGrantsPath is the path template of the implicit grants of a role of a resource.
const implicitGrantsPath = resourceRolePath + "/implicit_grants"

// The routes of the implicit grants of a role of a resource, as mockpermit.Server.Handle takes
// them.
const (
	createImplicitGrantRoute     = "POST " + implicitGrantsPath
	deleteImplicitGrantRoute     = "DELETE " + implicitGrantsPath
	implicitGrantConditionsRoute = "PUT " + implicitGrantsPath + "/conditions"
)

// The bodies of replies of the API: a rule of an implicit grant, the conditions of the implicit
// grants of a role, and a 404.
const (
	implicitGrantBody = `{"role_id":"00000000-0000-4000-8000-000000000041",` +
		`"resource_id":"00000000-0000-4000-8000-000000000022",` +
		`"relation_id":"00000000-0000-4000-8000-000000000051",` +
		`"role":"owner","on_resource":"folder","linked_by_relation":"parent"}`
	implicitGrantConditionsBody = `{"no_direct_roles_on_object":true}`
	implicitGrantNotFoundBody   = `{"error_code":"NOT_FOUND","title":"Not Found",` +
		`"message":"the role has no such implicit grant"}`
)

// ownerOfParentFolder is the rule of the calls of the tests: the users that have the role owner
// on a folder that is the parent of an instance.
var ownerOfParentFolder = models.DerivedRoleRuleCreate{
	Role: "owner", OnResource: "folder", LinkedByRelation: "parent",
}

// implicitGrantCall is a call of a method of the implicit grants of a role of a resource, with
// the ID of its operation, its route and a reply of the API to it. Its call takes the keys of the
// resource and of the role, and reports whether the method returned a result, which Delete never
// does.
type implicitGrantCall struct {
	name  string
	op    string
	route string
	reply mockpermit.Reply
	call  func(ctx context.Context, c *Client, resourceKey, roleKey string) (bool, error)
}

// implicitGrantCalls are calls of each method of the implicit grants of a role of a resource.
var implicitGrantCalls = []implicitGrantCall{
	{
		name: "Create", op: opCreateImplicitGrant, route: createImplicitGrantRoute,
		reply: mockpermit.Reply{Body: implicitGrantBody},
		call: func(ctx context.Context, c *Client, resourceKey, roleKey string) (bool, error) {
			rule, err := c.ImplicitGrants.Create(ctx, resourceKey, roleKey, ownerOfParentFolder)
			return rule != nil, err
		},
	},
	{
		name: "Delete", op: opDeleteImplicitGrant, route: deleteImplicitGrantRoute,
		reply: mockpermit.Reply{Status: http.StatusNoContent},
		call: func(ctx context.Context, c *Client, resourceKey, roleKey string) (bool, error) {
			return false, c.ImplicitGrants.Delete(ctx, resourceKey, roleKey,
				models.DerivedRoleRuleDelete(ownerOfParentFolder))
		},
	},
	{
		name: "UpdateConditions", op: opUpdateImplicitGrantConditions,
		route: implicitGrantConditionsRoute,
		reply: mockpermit.Reply{Body: implicitGrantConditionsBody},
		call: func(ctx context.Context, c *Client, resourceKey, roleKey string) (bool, error) {
			settings, err := c.ImplicitGrants.UpdateConditions(ctx, resourceKey, roleKey,
				models.DerivationSettings{NoDirectRolesOnObject: true})
			return settings != nil, err
		},
	},
}

// TestImplicitGrantKeysInThePath checks that each method sends the key of the resource first and
// the key of the role second, each as one segment of the path, escaped, after the project and the
// environment of the scope, whatever the key holds: escaped where the key holds a slash, a
// percent sign, a space, a question mark and a hash, or a letter outside ASCII, and as it is for
// the shape of a UUID, a leading dot and dots inside a key. A key that is empty, . or .. would
// change the route, so it is an error that matches ErrInvalidConfig and names its placeholder,
// and no request.
func TestImplicitGrantKeysInThePath(t *testing.T) {
	const prefix = "/v2/schema/p/e/resources/"
	for key, segment := range map[string]string{
		"a/b":            "a%2Fb",
		"50%":            "50%25",
		"a b?c#d":        "a%20b%3Fc%23d",
		"é":              "%C3%A9",
		fixtures.UUIDKey: fixtures.UUIDKey,
		".hidden":        ".hidden",
		"role.v2..x":     "role.v2..x",
	} {
		for _, m := range implicitGrantCalls {
			_, suffix, _ := strings.Cut(m.route, "{role_id}")
			for keys, want := range map[[2]string]string{
				{key, "viewer"}:   prefix + segment + "/roles/viewer" + suffix,
				{"document", key}: prefix + "document/roles/" + segment + suffix,
			} {
				api := mockpermit.NewAPI(t)
				api.Handle(m.route, m.reply)
				if _, err := m.call(t.Context(), resourceClient(t, api), keys[0],
					keys[1]); err != nil {
					t.Errorf("%s(%q, %q) returned %v", m.name, keys[0], keys[1], err)
				}
				if requests := api.Requests(); len(requests) != 1 ||
					requests[0].RawPath != want || requests[0].RawQuery != "" {
					t.Errorf("%s(%q, %q) sent %+v, want one request to %s without a query",
						m.name, keys[0], keys[1], requests, want)
				}
			}
		}
	}
	for _, key := range []string{"", ".", ".."} {
		for _, m := range implicitGrantCalls {
			api := mockpermit.NewAPI(t)
			c := resourceClient(t, api)
			for placeholder, keys := range map[string][2]string{
				"{resource_id}": {key, "viewer"}, "{role_id}": {"document", key},
			} {
				_, err := m.call(t.Context(), c, keys[0], keys[1])
				named := err != nil && strings.Contains(err.Error(), placeholder)
				if !errors.Is(err, ErrInvalidConfig) || !named {
					t.Errorf("%s(%q, %q) returned %v, want an error that matches "+
						"ErrInvalidConfig and names %s", m.name, keys[0], keys[1], err,
						placeholder)
				}
			}
			api.AssertUntouched()
		}
	}
}

// TestImplicitGrantsUseTheScope checks that the calls of the implicit grants act on the
// environment of the client's scope. Without WithScope, the client looks up the scope of its API
// key once, and each call fills the route with the project and the environment of an
// environment-level key. A scope without an environment, that of a project-level or an
// organization-level key, is an error that matches ErrInvalidConfig and names the operation, and
// no call sends a request but the lookup.
func TestImplicitGrantsUseTheScope(t *testing.T) {
	const prefix = "/v2/schema/" + scopeProjectID + "/" + scopeEnvironmentID +
		"/resources/document/roles/viewer/implicit_grants"
	t.Run("an environment-level key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		for _, m := range implicitGrantCalls {
			api.Handle(m.route, m.reply)
		}
		c := projectClient(t, api)
		for _, m := range implicitGrantCalls {
			if _, err := m.call(t.Context(), c, "document", "viewer"); err != nil {
				t.Errorf("the call of %s returned %v", m.op, err)
			}
		}
		requests := api.Requests()
		if len(requests) != len(implicitGrantCalls)+1 ||
			requests[0].RawPath != "/v2/api-key/scope" {
			t.Fatalf("the calls sent %+v, want the lookup of the scope and one request for each "+
				"of the %d calls", requests, len(implicitGrantCalls))
		}
		for _, r := range requests[1:] {
			if r.RawPath != prefix && r.RawPath != prefix+"/conditions" {
				t.Errorf("a call sent its request to %s, want %s or its conditions", r.RawPath,
					prefix)
			}
		}
	})
	for _, suffix := range []string{".project", ".organization"} {
		t.Run("the scope of the fixture"+suffix, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(scopeRoute, scopeReply(t, suffix))
			c := projectClient(t, api)
			for _, m := range implicitGrantCalls {
				_, err := m.call(t.Context(), c, "document", "viewer")
				if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), m.op) {
					t.Errorf("the call of %s returned %v, want an error that matches "+
						"ErrInvalidConfig and names %s", m.op, err, m.op)
				}
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("the calls sent %d requests, want only the lookup of the scope", n)
			}
		})
	}
}

// TestImplicitGrantErrors checks that a 404 to each call of the implicit grants, with or without
// a body, is an *Error that matches ErrNotFound: Delete too, as its path names the role and its
// body the rule to delete, so a 404 to it can mean a mistyped key, not a delete that happened
// already. Any other error status is an *Error with the status and the detail, after one request.
func TestImplicitGrantErrors(t *testing.T) {
	for _, m := range implicitGrantCalls {
		for _, reply := range []mockpermit.Reply{
			{Status: http.StatusNotFound, Body: implicitGrantNotFoundBody},
			{Status: http.StatusNotFound},
			{Status: http.StatusForbidden, Body: `{"detail":"no"}`},
			{Status: http.StatusConflict, Body: `{"detail":"no"}`},
			{Status: http.StatusUnprocessableEntity, Body: `{"detail":"no"}`},
		} {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, reply)
			returned, err := m.call(t.Context(), resourceClient(t, api), "document", "viewer")
			e := mustError(t, err)
			if returned || e.StatusCode != reply.Status || e.Op != m.op ||
				errors.Is(err, ErrNotFound) != (reply.Status == http.StatusNotFound) {
				t.Errorf("%s of a reply with the status %d returned a result (%t) and %v, want "+
					"only the *Error of %s with the status", m.name, reply.Status, returned, err,
					m.op)
			}
			if reply.Body == `{"detail":"no"}` && e.Detail != "no" {
				t.Errorf("%s returned the detail %q, want no", m.name, e.Detail)
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("%s of a reply with the status %d sent %d requests, want 1", m.name,
					reply.Status, n)
			}
		}
	}
}

// TestImplicitGrantBodies checks the bodies of the calls of the implicit grants byte for byte
// against the golden files in testdata/golden: a create and a delete of a rule with the required
// fields only, whose nil When is left out; a create whose When is the zero DerivationSettings,
// which sends no_direct_roles_on_object false; a create and a delete with every field, whose role
// holds <, > and & and a line separator, which an encoder may change; and conditions whose
// NoDirectRolesOnObject is false, which send it as false, and true.
func TestImplicitGrantBodies(t *testing.T) {
	role := "<owner>&" + fixtures.LineSeparator
	every := models.DerivedRoleRuleCreate{
		Role: role, OnResource: "folder", LinkedByRelation: "parent",
		When: &models.DerivationSettings{NoDirectRolesOnObject: true},
	}
	emptyWhen := ownerOfParentFolder
	emptyWhen.When = &models.DerivationSettings{}
	create := func(rule models.DerivedRoleRuleCreate) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.ImplicitGrants.Create(t.Context(), "document", "viewer", rule)
			return err
		}
	}
	remove := func(rule models.DerivedRoleRuleDelete) func(c *Client) error {
		return func(c *Client) error {
			return c.ImplicitGrants.Delete(t.Context(), "document", "viewer", rule)
		}
	}
	conditions := func(settings models.DerivationSettings) func(c *Client) error {
		return func(c *Client) error {
			_, err := c.ImplicitGrants.UpdateConditions(t.Context(), "document", "viewer",
				settings)
			return err
		}
	}
	for _, tc := range []struct {
		golden string
		route  string
		reply  mockpermit.Reply
		call   func(c *Client) error
	}{
		{
			"create_implicit_grant.required.golden", createImplicitGrantRoute,
			mockpermit.Reply{Body: implicitGrantBody},
			create(ownerOfParentFolder),
		},
		{
			"create_implicit_grant.empty-when.golden", createImplicitGrantRoute,
			mockpermit.Reply{Body: implicitGrantBody},
			create(emptyWhen),
		},
		{
			"create_implicit_grant.every-field.golden", createImplicitGrantRoute,
			mockpermit.Reply{Body: implicitGrantBody},
			create(every),
		},
		{
			"delete_implicit_grant.required.golden", deleteImplicitGrantRoute,
			mockpermit.Reply{Status: http.StatusNoContent},
			remove(models.DerivedRoleRuleDelete(ownerOfParentFolder)),
		},
		{
			"delete_implicit_grant.every-field.golden", deleteImplicitGrantRoute,
			mockpermit.Reply{Status: http.StatusNoContent},
			remove(models.DerivedRoleRuleDelete(every)),
		},
		{
			"update_implicit_grants_conditions.false.golden", implicitGrantConditionsRoute,
			mockpermit.Reply{Body: `{}`},
			conditions(models.DerivationSettings{}),
		},
		{
			"update_implicit_grants_conditions.true.golden", implicitGrantConditionsRoute,
			mockpermit.Reply{Body: implicitGrantConditionsBody},
			conditions(models.DerivationSettings{NoDirectRolesOnObject: true}),
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, tc.reply)
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

// TestImplicitGrantResults checks what Create and UpdateConditions return: the rule of the
// reply, with the IDs of the role, the resource and the relation that it names, and nil When
// when the reply holds none; and the conditions of the reply, false for {}.
func TestImplicitGrantResults(t *testing.T) {
	api := mockpermit.NewAPI(t)
	api.Handle(createImplicitGrantRoute, mockpermit.Reply{Body: implicitGrantBody})
	api.Handle(implicitGrantConditionsRoute, mockpermit.Reply{Body: `{}`},
		mockpermit.Reply{Body: implicitGrantConditionsBody})
	c := resourceClient(t, api)
	rule, err := c.ImplicitGrants.Create(t.Context(), "document", "viewer", ownerOfParentFolder)
	want := models.DerivedRoleRuleRead{
		RoleID:     "00000000-0000-4000-8000-000000000041",
		ResourceID: "00000000-0000-4000-8000-000000000022",
		RelationID: "00000000-0000-4000-8000-000000000051",
		Role:       "owner", OnResource: "folder", LinkedByRelation: "parent",
	}
	if err != nil || rule == nil || *rule != want {
		t.Errorf("Create returned %+v and %v, want %+v", rule, err, want)
	}
	for _, wantSet := range []bool{false, true} {
		settings, err := c.ImplicitGrants.UpdateConditions(t.Context(), "document", "viewer",
			models.DerivationSettings{NoDirectRolesOnObject: wantSet})
		if err != nil || settings.NoDirectRolesOnObject != wantSet {
			t.Errorf("UpdateConditions returned %+v and %v, want NoDirectRolesOnObject %t",
				settings, err, wantSet)
		}
	}
}

// TestImplicitGrantRetries checks when a call of the implicit grants sends its request again:
// Create, which the API may have acted on, after a 429 only, so a 503 ends it after one request;
// Delete and UpdateConditions, which do the same when sent twice, after a 503 too.
func TestImplicitGrantRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	for _, m := range implicitGrantCalls {
		replies := [][]mockpermit.Reply{{unavailable, m.reply}, {limited, m.reply}}
		statuses := []int{0, 0}
		if m.op == opCreateImplicitGrant {
			replies[0] = []mockpermit.Reply{unavailable}
			statuses[0] = http.StatusServiceUnavailable
		}
		for i, r := range replies {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, r...)
			c := resourceClient(t, api, WithRetry(RetryPolicy{MaxRetries: 1}))
			_, err := m.call(t.Context(), c, "document", "viewer")
			if statuses[i] == 0 && err != nil {
				t.Errorf("%s after %d returned %v, want the result of the second request",
					m.name, r[0].Status, err)
			}
			if statuses[i] != 0 && mustError(t, err).StatusCode != statuses[i] {
				t.Errorf("%s returned %v, want the error of status %d", m.name, err, statuses[i])
			}
			if n := len(api.Requests()); n != len(r) {
				t.Errorf("%s after %d sent %d requests, want %d", m.name, r[0].Status, n, len(r))
			}
		}
	}
}

// TestMalformedImplicitGrantReplies checks that a reply in 200-299 to Create that is not a JSON
// object of a rule, and to UpdateConditions one that is not a JSON object of the conditions, is
// an *Error that names the operation, its route and the model, and no result.
func TestMalformedImplicitGrantReplies(t *testing.T) {
	notJSONObjects := []string{``, `null`, `[]`, `"a"`, `{} {}`}
	replies := map[string]struct {
		model  string
		bodies []string
	}{
		opCreateImplicitGrant: {
			"models.DerivedRoleRuleRead",
			slices.Concat(notJSONObjects, []string{`{"role":1}`, `{"when":[]}`}),
		},
		opUpdateImplicitGrantConditions: {
			"models.DerivationSettings",
			slices.Concat(notJSONObjects, []string{`{"no_direct_roles_on_object":"a"}`}),
		},
	}
	for _, m := range implicitGrantCalls {
		r, ok := replies[m.op]
		if !ok {
			continue
		}
		want := "permit: " + m.op + ": " + m.route + ": status 200: the reply is not a valid " +
			r.model
		for _, body := range r.bodies {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, mockpermit.Reply{Body: body})
			returned, err := m.call(t.Context(), resourceClient(t, api), "document", "viewer")
			if e := mustError(t, err); returned || e.Error() != want {
				t.Errorf("%s of the reply %q returned a result (%t) and\n%v\nwant no result "+
					"and\n%s", m.name, body, returned, err, want)
			}
		}
	}
}

// TestImplicitGrantsAfterClose checks that each method returns ErrClosed after Close, and sends
// nothing, also without WithScope, whose lookup it does not send either.
func TestImplicitGrantsAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	for _, m := range implicitGrantCalls {
		if _, err := m.call(t.Context(), c, "document", "viewer"); !errors.Is(err, ErrClosed) {
			t.Errorf("%s after Close returned %v, want ErrClosed", m.name, err)
		}
	}
	api.AssertUntouched()
}
