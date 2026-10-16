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

// The routes of the refreshes of the data of the PDPs, as mockpermit.Server.Handle takes them,
// and the path of the refresh of every PDP under the scope p and e of resourceClient.
const (
	pdpRefreshRoute    = "POST /v2/pdps/{proj_id}/{env_id}/configs/refresh"
	pdpRefreshPDPRoute = "POST /v2/pdps/{proj_id}/{env_id}/configs/{pdp_id}/refresh"
	pdpRefreshPath     = "/v2/pdps/p/e/configs/refresh"
)

// pdpRefreshBody is a reply of the API to a refresh.
const pdpRefreshBody = `{"update_id":"u1","pdp_ids":["pdp-1","pdp-2"]}`

// pdpRefreshCalls are the calls of the refreshes, each with the route that it sends its request to,
// and pdpID for RefreshPDP, the ID of the PDP that it names.
var pdpRefreshCalls = []struct {
	name  string
	route string
	call  func(ctx context.Context, c *Client, pdpID string,
		refresh models.PDPDataRefreshRequest) (*models.PDPDataRefreshResponse, error)
}{
	{
		name: "Refresh", route: pdpRefreshRoute,
		call: func(ctx context.Context, c *Client, _ string,
			refresh models.PDPDataRefreshRequest,
		) (*models.PDPDataRefreshResponse, error) {
			return c.PDPs.Refresh(ctx, refresh)
		},
	},
	{
		name: "RefreshPDP", route: pdpRefreshPDPRoute,
		call: func(ctx context.Context, c *Client, pdpID string,
			refresh models.PDPDataRefreshRequest,
		) (*models.PDPDataRefreshResponse, error) {
			return c.PDPs.RefreshPDP(ctx, pdpID, refresh)
		},
	},
}

// TestPDPRefreshBodies checks the bodies of a refresh byte for byte against the golden files in
// testdata/golden, from Refresh and from RefreshPDP, which send the same body: {} for the zero
// PDPDataRefreshRequest; a reason that an encoder may escape; the shard 0, which a pointer to 0
// sends; and a shard with a reason. Refresh sends a ShardID as it is given, which the API's
// documentation says that it rejects.
func TestPDPRefreshBodies(t *testing.T) {
	reason := "<sync> & " + fixtures.LineSeparator + fixtures.ParagraphSeparator
	for _, tc := range []struct {
		golden  string
		refresh models.PDPDataRefreshRequest
	}{
		{"refresh_pdp_data.empty.golden", models.PDPDataRefreshRequest{}},
		{"refresh_pdp_data.reason.golden", models.PDPDataRefreshRequest{Reason: reason}},
		{"refresh_pdp_data.shard-0.golden", models.PDPDataRefreshRequest{ShardID: new(0)}},
		{
			"refresh_pdp_data.every-field.golden",
			models.PDPDataRefreshRequest{Reason: "reload", ShardID: new(3)},
		},
	} {
		for _, m := range pdpRefreshCalls {
			t.Run(m.name+"/"+tc.golden, func(t *testing.T) {
				api := mockpermit.NewAPI(t)
				api.Handle(m.route, mockpermit.Reply{Body: pdpRefreshBody})
				if _, err := m.call(t.Context(), resourceClient(t, api), "pdp-1",
					tc.refresh); err != nil {
					t.Fatalf("%s returned %v", m.name, err)
				}
				requests := api.Requests()
				if len(requests) != 1 {
					t.Fatalf("%s sent %d requests, want 1", m.name, len(requests))
				}
				requests[0].ExpectBytes(t, filepath.Join("testdata", "golden", tc.golden))
			})
		}
	}
}

// TestPDPRefreshRoutes checks the route of each refresh: Refresh sends the refresh of every PDP
// of the environment, never the route of one PDP; RefreshPDP sends the ID of the PDP as one
// segment of the path, escaped, and an ID that is empty, . or .. is an error that matches
// ErrInvalidConfig and names {pdp_id}, without a request. Each returns the reply.
func TestPDPRefreshRoutes(t *testing.T) {
	want := &models.PDPDataRefreshResponse{UpdateID: "u1", PDPIDs: []string{"pdp-1", "pdp-2"}}
	api := mockpermit.NewAPI(t)
	api.Handle(pdpRefreshRoute, mockpermit.Reply{Body: pdpRefreshBody})
	got, err := resourceClient(t, api).PDPs.Refresh(t.Context(), models.PDPDataRefreshRequest{})
	if diff := cmp.Diff(want, got); diff != "" || err != nil {
		t.Errorf("Refresh returned the error %v and a reply that differs (-want +got):\n%s", err,
			diff)
	}
	if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != pdpRefreshPath ||
		requests[0].Method != http.MethodPost {
		t.Errorf("Refresh sent %+v, want one POST to %s", requests, pdpRefreshPath)
	}
	for id, escaped := range map[string]string{
		"a/b":            "a%2Fb",
		"50%":            "50%25",
		"é":              "%C3%A9",
		fixtures.UUIDKey: fixtures.UUIDKey,
		".hidden":        ".hidden",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(pdpRefreshPDPRoute, mockpermit.Reply{Body: pdpRefreshBody})
		got, err := resourceClient(t, api).PDPs.RefreshPDP(t.Context(), id,
			models.PDPDataRefreshRequest{})
		if diff := cmp.Diff(want, got); diff != "" || err != nil {
			t.Errorf("RefreshPDP(%q) returned the error %v and a reply that differs "+
				"(-want +got):\n%s", id, err, diff)
		}
		path := "/v2/pdps/p/e/configs/" + escaped + "/refresh"
		if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != path {
			t.Errorf("RefreshPDP(%q) sent %+v, want one request to %s", id, requests, path)
		}
	}
	for _, id := range []string{"", ".", ".."} {
		api := mockpermit.NewAPI(t)
		_, err := resourceClient(t, api).PDPs.RefreshPDP(t.Context(), id,
			models.PDPDataRefreshRequest{})
		named := err != nil && strings.Contains(err.Error(), "{pdp_id}")
		if !errors.Is(err, ErrInvalidConfig) || !named {
			t.Errorf("RefreshPDP(%q) returned %v, want an error that matches ErrInvalidConfig "+
				"and names {pdp_id}", id, err)
		}
		api.AssertUntouched()
	}
}

// TestPDPRefreshesUseTheScope checks that the refreshes act on the environment of the client's
// scope: without WithScope, the client looks up the scope of its API key once, and each call
// fills the route with the project and the environment of an environment-level key. A scope
// without an environment is an error that matches ErrInvalidConfig and names the operation, and
// no call sends a request but the lookup.
func TestPDPRefreshesUseTheScope(t *testing.T) {
	const prefix = "/v2/pdps/" + scopeProjectID + "/" + scopeEnvironmentID + "/configs/"
	ops := []string{opRefreshEnvironmentPDPData, opRefreshPDPData}
	t.Run("an environment-level key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		c := projectClient(t, api)
		for _, m := range pdpRefreshCalls {
			api.Handle(m.route, mockpermit.Reply{Body: pdpRefreshBody})
			_, err := m.call(t.Context(), c, "pdp-1", models.PDPDataRefreshRequest{})
			if err != nil {
				t.Errorf("%s returned %v", m.name, err)
			}
		}
		requests := api.Requests()
		if len(requests) != 3 || requests[0].RawPath != "/v2/api-key/scope" ||
			requests[1].RawPath != prefix+"refresh" ||
			requests[2].RawPath != prefix+"pdp-1/refresh" {
			t.Errorf("the calls sent %+v, want the lookup of the scope, then a request under %s "+
				"for each call", requests, prefix)
		}
	})
	for _, suffix := range []string{".project", ".organization"} {
		t.Run("the scope of the fixture"+suffix, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(scopeRoute, scopeReply(t, suffix))
			c := projectClient(t, api)
			for i, m := range pdpRefreshCalls {
				_, err := m.call(t.Context(), c, "pdp-1", models.PDPDataRefreshRequest{})
				if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), ops[i]) {
					t.Errorf("%s returned %v, want an error that matches ErrInvalidConfig and "+
						"names %s", m.name, err, ops[i])
				}
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("the calls sent %d requests, want only the lookup of the scope", n)
			}
		})
	}
}

// TestPDPRefreshRetries checks that each refresh, which a second request repeats without harm,
// sends its request again after a 503, as after a 429, with the default policy of retries.
func TestPDPRefreshRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	for _, m := range pdpRefreshCalls {
		for _, status := range []int{http.StatusServiceUnavailable, http.StatusTooManyRequests} {
			t.Run(m.name+" after "+strconv.Itoa(status), func(t *testing.T) {
				api := mockpermit.NewAPI(t)
				api.Handle(m.route, mockpermit.Reply{Status: status, Header: again, Body: `{}`},
					mockpermit.Reply{Body: pdpRefreshBody})
				_, err := m.call(t.Context(), resourceClient(t, api), "pdp-1",
					models.PDPDataRefreshRequest{Reason: "reload"})
				if err != nil {
					t.Errorf("%s returned %v, want the reply of the second request", m.name, err)
				}
				requests := api.Requests()
				if len(requests) != 2 {
					t.Fatalf("%s sent %d requests, want 2", m.name, len(requests))
				}
				for _, r := range requests {
					r.ExpectJSON(t, `{"reason":"reload"}`)
				}
			})
		}
	}
}

// TestMalformedPDPRefreshReplies checks that a reply in 200-299 that is not a JSON object of a
// refresh is an *Error that says so, and no reply.
func TestMalformedPDPRefreshReplies(t *testing.T) {
	for _, body := range []string{``, `null`, `[]`, `"a"`, `{"pdp_ids":"pdp-1"}`, `{} {}`} {
		for i, m := range pdpRefreshCalls {
			api := mockpermit.NewAPI(t)
			api.Handle(m.route, mockpermit.Reply{Body: body})
			got, err := m.call(t.Context(), resourceClient(t, api), "pdp-1",
				models.PDPDataRefreshRequest{})
			op := []string{opRefreshEnvironmentPDPData, opRefreshPDPData}[i]
			want := "permit: " + op + ": " + m.route + ": status 200: the reply is not a valid " +
				"models.PDPDataRefreshResponse"
			if e := mustError(t, err); got != nil || e.Error() != want {
				t.Errorf("%s of the reply %q returned %+v and\n%v\nwant no reply and\n%s", m.name,
					body, got, err, want)
			}
		}
	}
}

// TestPDPRefreshesAfterClose checks that each refresh returns ErrClosed after Close, and sends
// nothing, also without WithScope, whose lookup it does not send either.
func TestPDPRefreshesAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	for _, m := range pdpRefreshCalls {
		_, err := m.call(t.Context(), c, "pdp-1", models.PDPDataRefreshRequest{})
		if !errors.Is(err, ErrClosed) {
			t.Errorf("%s after Close returned %v, want ErrClosed", m.name, err)
		}
	}
	api.AssertUntouched()
}
