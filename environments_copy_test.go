package permit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/redact"
	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// copyIntoQA is a copy into the existing environment qa, with nothing else set.
var copyIntoQA = models.EnvironmentCopy{TargetEnv: models.EnvironmentCopyTarget{Existing: "qa"}}

// onlyAPIRequest returns the one request that api got, and fails the test when it got another
// number of requests.
func onlyAPIRequest(t *testing.T, api *mockpermit.Server) mockpermit.Request {
	t.Helper()
	requests := api.Requests()
	if len(requests) != 1 {
		t.Fatalf("the API got %d requests, want 1", len(requests))
	}
	return requests[0]
}

// TestEnvironmentCopyBodies checks the body of a copy byte for byte against the golden files in
// testdata/golden, from Copy and from CopyAsync, which send the same body: a copy into an
// existing environment only, which leaves out the zero conflict strategy and the zero scope; one
// with every field, a new environment and text that an encoder may escape among them, an empty
// list of a filter, which sends [], and a nil one, which it leaves out; and a conflict strategy
// that this version of the SDK does not know, which it sends as it is. Each body decodes back
// into an EnvironmentCopy, which encodes as the same body.
func TestEnvironmentCopyBodies(t *testing.T) {
	every := models.EnvironmentCopy{
		TargetEnv: models.EnvironmentCopyTarget{
			Existing: "qa",
			New: &models.EnvironmentCreate{
				Key: "e", Name: "<E> & F", Description: "a" + fixtures.LineSeparator + "b",
				CustomBranchName: "b", Settings: models.Attributes{"n": fixtures.TwoPow53Plus1},
			},
		},
		ConflictStrategy: models.EnvironmentCopyConflictStrategyOverwrite,
		Scope: models.EnvironmentCopyScope{
			Resources: models.EnvironmentCopyScopeFilters{
				Include: []string{"doc-*"}, Exclude: []string{"doc-old"},
			},
			Roles: models.EnvironmentCopyScopeFilters{
				Include: []string{}, Exclude: []string{"admin"},
			},
			UserSets:     models.EnvironmentCopyScopeFilters{Include: []string{"*"}},
			ResourceSets: models.EnvironmentCopyScopeFilters{Exclude: []string{"rs"}},
			CustomPolicies: models.EnvironmentCopyScopeFilters{
				Include: []string{"p<1>"}, Exclude: []string{},
			},
		},
	}
	unknown := copyIntoQA
	unknown.ConflictStrategy = "merge"
	for _, tc := range []struct {
		golden string
		copy   models.EnvironmentCopy
	}{
		{"copy_environment.existing.golden", copyIntoQA},
		{"copy_environment.every-field.golden", every},
		{"copy_environment.unknown-strategy.golden", unknown},
	} {
		for _, call := range []struct {
			route string
			send  func(c *Client) error
		}{
			{copyEnvironmentRoute, func(c *Client) error {
				_, err := c.Environments.Copy(t.Context(), "p", "e", tc.copy)
				return err
			}},
			{copyAsyncRoute, func(c *Client) error {
				_, err := c.Environments.CopyAsync(t.Context(), "p", "e", tc.copy)
				return err
			}},
		} {
			t.Run(tc.golden+" "+call.route, func(t *testing.T) {
				api := mockpermit.NewAPI(t)
				api.Handle(call.route, mockpermit.Reply{Body: `{"task_id":"t","status":"success"}`})
				if err := call.send(projectClient(t, api)); err != nil {
					t.Fatalf("the call returned %v", err)
				}
				request := onlyAPIRequest(t, api)
				request.ExpectBytes(t, filepath.Join("testdata", "golden", tc.golden))
				var decoded models.EnvironmentCopy
				if err := wire.Decode(request.Body, &decoded); err != nil {
					t.Fatalf("the body %s does not decode into an EnvironmentCopy: %v",
						request.Body, err)
				}
				again, err := wire.Encode(decoded)
				if err != nil || !bytes.Equal(again, request.Body) {
					t.Errorf("%s decodes and encodes back as %s, %v", request.Body, again, err)
				}
			})
		}
	}
}

// TestCopyAsyncSubmitsOnce checks that CopyAsync sends one request, without a query, and
// returns the task of the reply as it is: also a task that is still processing, one that failed
// and one whose status this version of the SDK does not know. It does not read the result of
// the task, whose route the test does not register, so that a request to it would fail the test.
func TestCopyAsyncSubmitsOnce(t *testing.T) {
	for _, status := range []models.TaskStatus{
		models.TaskStatusProcessing, models.TaskStatusFailure, "queued",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(copyAsyncRoute, mockpermit.Reply{
			Status: http.StatusAccepted, Body: `{"task_id":"t1","status":"` + string(status) + `"}`,
		})
		task, err := projectClient(t, api).Environments.CopyAsync(t.Context(), "p", "e",
			copyIntoQA)
		want := &models.TaskResultEnvironmentRead{TaskID: "t1", Status: status}
		if diff := cmp.Diff(want, task); diff != "" || err != nil {
			t.Errorf("CopyAsync of a task that is %s returned %v and a task that differs "+
				"(-want +got):\n%s", status, err, diff)
		}
		if r := onlyAPIRequest(t, api); r.RawQuery != "" {
			t.Errorf("CopyAsync sent the query %q, want none", r.RawQuery)
		}
	}
}

// TestCopyResultWait checks the query of CopyResult: none for nil options, for the zero options
// and for a zero Wait; and the Wait in seconds otherwise, a fraction of a second too, and a Wait
// above the 60 seconds of the API's spec, which the client sends as it is. A Wait less than zero
// is an error that matches ErrInvalidConfig and names the operation, the options and the rule,
// and the call sends no request.
func TestCopyResultWait(t *testing.T) {
	for _, tc := range []struct {
		opts  *CopyResultOptions
		query string
	}{
		{nil, ""},
		{&CopyResultOptions{}, ""},
		{&CopyResultOptions{Wait: 250 * time.Millisecond}, "wait=0.25"},
		{&CopyResultOptions{Wait: time.Minute}, "wait=60"},
		{&CopyResultOptions{Wait: 90 * time.Second}, "wait=90"},
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(copyResultRoute, mockpermit.Reply{Body: `{"task_id":"t","status":"success"}`})
		if _, err := projectClient(t, api).Environments.CopyResult(t.Context(), "p", "e", "t",
			tc.opts); err != nil {
			t.Errorf("CopyResult with %+v returned %v", tc.opts, err)
		}
		if r := onlyAPIRequest(t, api); r.RawQuery != tc.query {
			t.Errorf("CopyResult with %+v sent the query %q, want %q", tc.opts, r.RawQuery,
				tc.query)
		}
	}
	for _, wait := range []time.Duration{-1, -time.Second} {
		api := mockpermit.NewAPI(t)
		_, err := projectClient(t, api).Environments.CopyResult(t.Context(), "p", "e", "t",
			&CopyResultOptions{Wait: wait})
		want := "get_copy_environment_async_result: CopyResultOptions: Wait is less than zero"
		if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), want) {
			t.Errorf("CopyResult with a Wait of %s returned %v, want an error that matches "+
				"ErrInvalidConfig and says %q", wait, err, want)
		}
		api.AssertUntouched()
	}
}

// TestCopyResultOfATaskThatFailed checks that CopyResult returns the task of the reply fixture
// get_copy_environment_async_result.200.failure.json, a copy that failed, with the error that
// ended it: its code and its additional information, whose numbers keep every digit, and no
// environment.
func TestCopyResultOfATaskThatFailed(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "replies",
		"get_copy_environment_async_result.200.failure.json"))
	if err != nil {
		t.Fatal(err)
	}
	api := mockpermit.NewAPI(t)
	api.Handle(copyResultRoute, mockpermit.Reply{Body: string(body)})
	task, err := projectClient(t, api).Environments.CopyResult(t.Context(), "p", "e",
		"00000000-0000-4000-8000-000000000012", nil)
	want := &models.TaskResultEnvironmentRead{
		TaskID: "00000000-0000-4000-8000-000000000012", Status: models.TaskStatusFailure,
		Error: &models.ErrorDetails{
			ID: "err-9", Title: "Conflict", ErrorCode: models.ErrorCodeDuplicateEntity,
			Message:     "the role admin exists in the target",
			SupportLink: "https://docs.example.com/errors",
			AdditionalInfo: map[string]any{
				"conflicts": []any{map[string]any{"role": "admin"}},
				"count":     json.Number("9007199254740993"),
			},
		},
	}
	if diff := cmp.Diff(want, task); diff != "" || err != nil {
		t.Errorf("CopyResult returned %v and a task that differs (-want +got):\n%s", err, diff)
	}
}

// TestCopyResultTaskInThePath checks that CopyResult sends the ID of the task as one segment of
// the path, escaped, whatever it holds; and that an ID that is empty, . or .., which would change
// the route, is an error that matches ErrInvalidConfig and names {task_id}, and no request.
func TestCopyResultTaskInThePath(t *testing.T) {
	for taskID, escaped := range map[string]string{
		"a/b": "a%2Fb", "50%é": "50%25%C3%A9", ".hidden": ".hidden",
		fixtures.UUIDKey: fixtures.UUIDKey,
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(copyResultRoute, mockpermit.Reply{Body: `{"task_id":"t","status":"success"}`})
		if _, err := projectClient(t, api).Environments.CopyResult(t.Context(), "p", "e", taskID,
			nil); err != nil {
			t.Errorf("CopyResult of the task %q returned %v", taskID, err)
		}
		want := "/v2/projects/p/envs/e/copy/async/" + escaped + "/result"
		if r := onlyAPIRequest(t, api); r.RawPath != want {
			t.Errorf("CopyResult of the task %q sent %s, want %s", taskID, r.RawPath, want)
		}
	}
	for _, taskID := range []string{"", ".", ".."} {
		api := mockpermit.NewAPI(t)
		_, err := projectClient(t, api).Environments.CopyResult(t.Context(), "p", "e", taskID,
			nil)
		if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), "{task_id}") {
			t.Errorf("CopyResult of the task %q returned %v, want an error that matches "+
				"ErrInvalidConfig and names {task_id}", taskID, err)
		}
		api.AssertUntouched()
	}
}

// The credentials of the PDP of the replies of TestStatsCredentialsJoinTheRedaction: values that
// no pattern of the redaction finds in the message of an error body, so that only the client's
// set of credentials can replace them there.
const (
	pdpClientValue = "pdpclient-pdpclient"
	pdpAccessValue = "pdpaccess-pdpaccess"
)

// TestStatsCredentialsJoinTheRedaction checks that the credentials of the PDPs that a reply of
// Stats holds, its client_secret, which the client finds by its name, and its
// opal_server_access_token, which the registry lists as a sensitive field, join the client's
// redaction: after Stats, the error of a later call whose reply echoes them in its message holds
// the marker in their place, in its text, its Detail and its Body, and so does the client's Debug
// log; the statistics print them as the marker with %+v. A client that did not read the
// statistics keeps them in the same error, which shows that the reply of Stats added them.
func TestStatsCredentialsJoinTheRedaction(t *testing.T) {
	statsBody := `{"key":"production","id":"e","name":"Production","pdp_configs":[{"id":"c",` +
		`"client_secret":"` + pdpClientValue + `","opal_server_access_token":"` +
		pdpAccessValue + `"}],"stats":{}}`
	echo := mockpermit.Reply{
		Status: http.StatusBadRequest,
		Body: `{"error_code":"BAD_REQUEST","title":"Bad Request","message":"the PDP ` +
			pdpClientValue + ` with ` + pdpAccessValue + ` failed"}`,
	}
	values := []string{pdpClientValue, pdpAccessValue}
	holdsAny := func(text string) bool {
		return strings.Contains(text, pdpClientValue) || strings.Contains(text, pdpAccessValue)
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	api := mockpermit.NewAPI(t)
	api.Handle(statsEnvironmentRoute, mockpermit.Reply{Body: statsBody})
	api.Handle(getEnvironmentRoute, echo)
	c := projectClient(t, api, WithLogger(logger))
	stats, err := c.Environments.Stats(t.Context(), "p", "e")
	if err != nil {
		t.Fatalf("Stats returned %v", err)
	}
	config := stats.PDPConfigs[0]
	if config.ClientSecret.Value() != pdpClientValue ||
		config.OPALServerAccessToken.Value() != pdpAccessValue {
		t.Errorf("Stats returned the credentials %q and %q, want %q", config.ClientSecret.Value(),
			config.OPALServerAccessToken.Value(), values)
	}
	if printed := fmt.Sprintf("%+v %#v", stats, stats); holdsAny(printed) ||
		strings.Count(printed, redact.Marker) != 4 {
		t.Errorf("%%+v and %%#v of the statistics print %s, want the marker in place of each "+
			"credential", printed)
	}
	_, err = c.Environments.Get(t.Context(), "p", "e")
	e := mustError(t, err)
	if holdsAny(e.Error()) || holdsAny(e.Detail) || holdsAny(string(e.Body)) ||
		!strings.Contains(e.Detail, redact.Marker) {
		t.Errorf("the error of a reply that echoes the credentials is %q, with the Detail %q "+
			"and the body %q; want the marker in place of each", e.Error(), e.Detail, e.Body)
	}
	if holdsAny(logs.String()) || !strings.Contains(logs.String(), redact.Marker) {
		t.Errorf("the Debug log holds a credential of the PDP, or no marker:\n%s", &logs)
	}

	control := mockpermit.NewAPI(t)
	control.Handle(getEnvironmentRoute, echo)
	_, err = projectClient(t, control).Environments.Get(t.Context(), "p", "e")
	if e := mustError(t, err); !strings.Contains(e.Detail, pdpClientValue) ||
		!strings.Contains(e.Detail, pdpAccessValue) {
		t.Errorf("a client that did not read the statistics returned the Detail %q, want the "+
			"credentials as they are, as no pattern finds them", e.Detail)
	}
}

// TestMalformedStatsAndCopyReplies checks that a reply in 200-299 that is not the statistics of
// an environment, an environment or a task is an *Error that names the model, and no value: a
// credential of a PDP that is not a string, whose error does not hold it, and the environment of
// a task whose audience is neither a string nor a list of strings among them.
func TestMalformedStatsAndCopyReplies(t *testing.T) {
	const prefix = "permit: "
	malformed := []string{``, `null`, `[]`, `"e"`, `{} {}`}
	for _, tc := range []struct {
		route, op, model string
		bodies           []string
		call             func(c *Client) (any, error)
	}{
		{
			statsEnvironmentRoute,
			"stats_environments: GET /v2/projects/{proj_id}/envs/{env_id}/stats",
			"models.EnvironmentStats",
			[]string{
				`{"pdp_configs":[{"client_secret":5}]}`,
				`{"pdp_configs":[{"opal_server_access_token":["opaque-listed-value"]}]}`,
				`{"stats":{"users":"many"}}`,
			},
			func(c *Client) (any, error) { return c.Environments.Stats(t.Context(), "p", "e") },
		},
		{
			copyEnvironmentRoute,
			"copy_environment: POST /v2/projects/{proj_id}/envs/{env_id}/copy",
			"models.EnvironmentRead",
			[]string{`{"key":1}`, `{"jwks":{"audience":7}}`},
			func(c *Client) (any, error) {
				return c.Environments.Copy(t.Context(), "p", "e", copyIntoQA)
			},
		},
		{
			copyAsyncRoute,
			"copy_environment_async: POST /v2/projects/{proj_id}/envs/{env_id}/copy/async",
			"models.TaskResultEnvironmentRead",
			[]string{`{"task_id":1}`, `{"result":{"jwks":{"issuer":[null]}}}`},
			func(c *Client) (any, error) {
				return c.Environments.CopyAsync(t.Context(), "p", "e", copyIntoQA)
			},
		},
		{
			copyResultRoute, "get_copy_environment_async_result: GET " +
				"/v2/projects/{proj_id}/envs/{env_id}/copy/async/{task_id}/result",
			"models.TaskResultEnvironmentRead",
			[]string{`{"status":["success"]}`, `{"error":{"title":2}}`},
			func(c *Client) (any, error) {
				return c.Environments.CopyResult(t.Context(), "p", "e", "t", nil)
			},
		},
	} {
		for _, body := range append(malformed, tc.bodies...) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: body})
			got, err := tc.call(projectClient(t, api))
			want := prefix + tc.op + ": status 200: the reply is not a valid " + tc.model
			if e := mustError(t, err); !isNilResult(got) || e.Error() != want ||
				strings.Contains(fmt.Sprintf("%+v", e), "opaque") {
				t.Errorf("the reply %q returned %+v and\n%v\nwant no value and\n%s", body, got,
					err, want)
			}
		}
	}
}
