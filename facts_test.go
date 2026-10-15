package permit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/testhook"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
	"github.com/permitio/permit-golang/v2/internal/testkit/concurrent"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/internal/validate"
)

// testFactsRoute is the route of the test-only facts operations without its method: TestPrefix
// and the prefix of the facts API, then what follows it.
func testFactsRoute(tail string) string {
	return rest.TestPrefix + "v2/facts/{proj_id}/{env_id}/" + tail
}

// The test-only operations of the facts tests: a write that the PDP container waits on, one that
// it does not wait on, and an operation under /v2/facts that is not one of facts, as a set rule
// is.
var (
	opFactsUpdate = rest.Op{
		ID: "test_facts_update", Method: http.MethodPatch,
		Route: testFactsRoute("objects/{object_id}"), Facts: true, WaitsForSync: true,
		KeyLevel: rest.EnvironmentLevel, RequestModel: "TestObject", ResponseModel: "TestObject",
	}
	opFactsDelete = rest.Op{
		ID: "test_facts_delete", Method: http.MethodDelete,
		Route: testFactsRoute("objects/{object_id}"), Facts: true, Idempotent: true,
		DeleteSelf: true, KeyLevel: rest.EnvironmentLevel,
	}
	opSetRule = rest.Op{
		ID: "test_set_rule", Method: http.MethodPost, Route: testFactsRoute("set_rules"),
		KeyLevel: rest.EnvironmentLevel,
	}
)

// The routes of opFactsUpdate on the API and on the PDP container, as mockpermit.Server.Handle
// takes them.
const (
	apiUpdateRoute = "PATCH /__test/v2/facts/{proj_id}/{env_id}/objects/{object_id}"
	pdpUpdateRoute = "PATCH /__test/facts/objects/{object_id}"
)

// factsClient returns a test client with the facts tests' operations whose URLs point at api and
// pdp, with mockpermit's API key and opts, and closes it when the test ends.
func factsClient(t *testing.T, api, pdp *mockpermit.Server, opts ...Option) *Client {
	t.Helper()
	clearEnv(t)
	base := []Option{WithAPIKey(mockpermit.APIKey), WithAPIURL(api.URL), WithPDPURL(pdp.URL)}
	c := newTestClient(t, append(base, opts...), opFactsUpdate, opFactsDelete, opSetRule)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// update makes a call of opFactsUpdate for the object key, with the body {"key": key}, through
// f, as a method of a facts service does.
func update(ctx context.Context, f facts, key string) (*testObject, error) {
	r, err := f.prepare(ctx, request{
		op: opFactsUpdate.ID, params: []string{key}, body: testObject{Key: key},
	})
	if err != nil {
		return nil, err
	}
	return do[testObject](ctx, f.client, r)
}

// deleteObject makes a call of opFactsDelete for the object key through f.
func deleteObject(ctx context.Context, f facts, key string) error {
	r, err := f.prepare(ctx, request{op: opFactsDelete.ID, params: []string{key}})
	if err != nil {
		return err
	}
	return f.client.doNoValue(ctx, r)
}

// TestFactsGoToTheAPI checks that without WithFactsViaPDP a facts call goes to the API, under
// /v2/facts with the project and the environment of the client's scope, and not to the PDP.
func TestFactsGoToTheAPI(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	api.Handle(apiUpdateRoute, mockpermit.Reply{Body: `{"key":"a/b"}`})
	c := factsClient(t, api, pdp, WithScope("my-project", "production"))
	got, err := update(t.Context(), facts{client: c}, "a/b")
	if err != nil || got.Key != "a/b" {
		t.Fatalf("the call returned %+v, %v", got, err)
	}
	requests := api.Requests()
	if len(requests) != 1 {
		t.Fatalf("the API got %d requests, want 1", len(requests))
	}
	if want := "/__test/v2/facts/my-project/production/objects/a%2Fb"; requests[0].RawPath != want {
		t.Errorf("the call went to %s, want %s", requests[0].RawPath, want)
	}
	requests[0].ExpectJSON(t, `{"key":"a/b"}`)
	pdp.AssertUntouched()
}

// TestFactsGoToThePDPContainer checks that with WithFactsViaPDP a facts call goes to the PDP
// container under /facts, without the project and the environment, and sends nothing to the API:
// not even a lookup of the scope, which the call does not need.
func TestFactsGoToThePDPContainer(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	pdp.Handle(pdpUpdateRoute, mockpermit.Reply{Body: `{"key":"a/b"}`})
	pdp.Handle("DELETE /__test/facts/objects/{object_id}",
		mockpermit.Reply{Status: http.StatusNoContent})
	c := factsClient(t, api, pdp, WithFactsViaPDP())
	f := facts{client: c}
	got, err := update(t.Context(), f, "a/b")
	if err != nil || got.Key != "a/b" {
		t.Fatalf("the call returned %+v, %v", got, err)
	}
	if err := deleteObject(t.Context(), f, "c"); err != nil {
		t.Fatalf("the delete returned %v", err)
	}
	requests := pdp.Requests()
	if len(requests) != 2 {
		t.Fatalf("the PDP got %d requests, want 2", len(requests))
	}
	for i, want := range []string{
		"PATCH /__test/facts/objects/a%2Fb",
		"DELETE /__test/facts/objects/c",
	} {
		if got := requests[i].Method + " " + requests[i].RawPath; got != want {
			t.Errorf("request %d is %s, want %s", i+1, got, want)
		}
	}
	requests[0].ExpectJSON(t, `{"key":"a/b"}`)
	api.AssertUntouched()
}

// TestFactsViaTheAPILookUpTheScope checks that without WithScope a facts call to the API looks up
// the scope of the API key first and fills it in.
func TestFactsViaTheAPILookUpTheScope(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	api.Handle(scopeRoute, scopeReply(t, ""))
	api.Handle(apiUpdateRoute, mockpermit.Reply{Body: `{"key":"a"}`})
	c := factsClient(t, api, pdp)
	if _, err := update(t.Context(), facts{client: c}, "a"); err != nil {
		t.Fatal(err)
	}
	want := "/__test/v2/facts/" + scopeProjectID + "/" + scopeEnvironmentID + "/objects/a"
	if requests := api.Requests(); len(requests) != 2 || requests[1].RawPath != want {
		t.Errorf("the API got %d requests, want the lookup and one to %s", len(requests), want)
	}
	pdp.AssertUntouched()
}

// TestFactsOfACloudPDP checks a 404 to a facts call: from the PDP with an empty body, which is
// how the cloud PDP answers every route under /facts, it matches ErrContainerPDPOnly, not
// ErrNotFound, and names the route under /facts; with a body it is a 404 of the container; and
// from the API it is a 404 of the API.
func TestFactsOfACloudPDP(t *testing.T) {
	for _, tc := range []struct {
		name          string
		viaPDP        bool
		body          string
		wantContainer bool
		wantText      string
	}{
		{
			name: "the PDP with an empty body", viaPDP: true, wantContainer: true,
			wantText: "permit: test_facts_update: PATCH /__test/facts/objects/{object_id}: " +
				"status 404: permit: only the PDP container serves this route",
		},
		{
			name: "the PDP with a body", viaPDP: true, body: `{"detail":"Not Found"}`,
			wantText: "permit: test_facts_update: PATCH /__test/facts/objects/{object_id}: " +
				"status 404: Not Found",
		},
		{
			name: "the API with an empty body",
			wantText: "permit: test_facts_update: PATCH " +
				"/__test/v2/facts/{proj_id}/{env_id}/objects/{object_id}: status 404",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			to, other, route := api, pdp, apiUpdateRoute
			opts := []Option{WithScope("p", "e")}
			if tc.viaPDP {
				to, other, route = pdp, api, pdpUpdateRoute
				opts = append(opts, WithFactsViaPDP())
			}
			to.Handle(route, mockpermit.Reply{Status: http.StatusNotFound, Body: tc.body})
			_, err := update(t.Context(), facts{client: factsClient(t, api, pdp, opts...)}, "a")
			other.AssertUntouched()
			e := mustError(t, err)
			if got := errors.Is(err, ErrContainerPDPOnly); got != tc.wantContainer {
				t.Errorf("errors.Is(err, ErrContainerPDPOnly) = %t, want %t", got,
					tc.wantContainer)
			}
			if got := errors.Is(err, ErrNotFound); got == tc.wantContainer {
				t.Errorf("errors.Is(err, ErrNotFound) = %t, want %t", got, !tc.wantContainer)
			}
			if e.Error() != tc.wantText {
				t.Errorf("Error() = %q, want %q", e.Error(), tc.wantText)
			}
		})
	}
}

// TestFactsDeleteOfACloudPDP checks that a delete by a path that names the object, which returns
// nil for a 404 as the object is gone already, still returns an error that matches
// ErrContainerPDPOnly for a 404 with an empty body through the PDP, which is how the cloud PDP
// answers every route under /facts/: the delete did not happen.
func TestFactsDeleteOfACloudPDP(t *testing.T) {
	for _, tc := range []struct {
		name          string
		viaPDP        bool
		body          string
		wantContainer bool
	}{
		{name: "the PDP with an empty body", viaPDP: true, wantContainer: true},
		{name: "the PDP with a body", viaPDP: true, body: `{"detail":"Not Found"}`},
		{name: "the API with an empty body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			to, other := api, pdp
			route := "DELETE /__test/v2/facts/{proj_id}/{env_id}/objects/{object_id}"
			opts := []Option{WithScope("p", "e")}
			if tc.viaPDP {
				to, other = pdp, api
				route = "DELETE /__test/facts/objects/{object_id}"
				opts = append(opts, WithFactsViaPDP())
			}
			to.Handle(route, mockpermit.Reply{Status: http.StatusNotFound, Body: tc.body})
			err := deleteObject(t.Context(), facts{client: factsClient(t, api, pdp, opts...)}, "a")
			other.AssertUntouched()
			if !tc.wantContainer {
				if err != nil {
					t.Errorf("the delete of an object that is gone returned %v, want nil", err)
				}
				return
			}
			want := "permit: test_facts_delete: DELETE /__test/facts/objects/{object_id}: " +
				"status 404: permit: only the PDP container serves this route"
			if !errors.Is(err, ErrContainerPDPOnly) || mustError(t, err).Error() != want {
				t.Errorf("the delete returned %v, want an error that matches "+
					"ErrContainerPDPOnly: %s", err, want)
			}
		})
	}
}

// waitHeaders returns the wait headers of a request: X-Wait-Timeout and X-Timeout-Policy, each
// "-" when the request has none.
func waitHeaders(h http.Header) string {
	values := []string{"-", "-"}
	for i, name := range []string{headerWaitTimeout, headerTimeoutPolicy} {
		if v, ok := h[name]; ok {
			values[i] = strings.Join(v, ",")
		}
	}
	return values[0] + " " + values[1]
}

// TestWaitForSyncHeaders checks the headers of a call that waits for sync: the timeout in
// decimal seconds, also 0, and the policy when the wait has one. The calls of the facts that the
// wait was copied from carry neither, before and after the call that waits.
func TestWaitForSyncHeaders(t *testing.T) {
	for _, tc := range []struct {
		wait syncWait
		want string
	}{
		{syncWait{timeout: 0}, "0 -"},
		{syncWait{timeout: 500 * time.Millisecond}, "0.5 -"},
		{syncWait{timeout: 1500 * time.Millisecond}, "1.5 -"},
		{syncWait{timeout: 10 * time.Second, policy: SyncPolicyFail}, "10 fail"},
		{syncWait{timeout: 250 * time.Millisecond, policy: SyncPolicyIgnore}, "0.25 ignore"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			reply := mockpermit.Reply{Body: `{"key":"a"}`}
			pdp.Handle(pdpUpdateRoute, reply, reply, reply)
			plain := facts{client: factsClient(t, api, pdp, WithFactsViaPDP())}
			for _, f := range []facts{plain, plain.withWait(tc.wait), plain} {
				if _, err := update(t.Context(), f, "a"); err != nil {
					t.Fatal(err)
				}
			}
			var got []string
			for _, r := range pdp.Requests() {
				got = append(got, waitHeaders(r.Header))
			}
			if diff := cmp.Diff([]string{"- -", tc.want, "- -"}, got); diff != "" {
				t.Errorf("the wait headers of the three calls differ (-want +got):\n%s", diff)
			}
			api.AssertUntouched()
		})
	}
}

func TestWaitSeconds(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0:                       "0",
		time.Nanosecond:         "0.000000001",
		time.Microsecond:        "0.000001",
		120 * time.Millisecond:  "0.12",
		500 * time.Millisecond:  "0.5",
		time.Second:             "1",
		1118 * time.Millisecond: "1.118",
		1500 * time.Millisecond: "1.5",
		10 * time.Second:        "10",
		time.Minute:             "60",
		math.MaxInt64:           "9223372036.854775807",
	} {
		if got := waitSeconds(d); got != want {
			t.Errorf("waitSeconds(%v) = %q, want %q", d, got, want)
		}
	}
}

// TestWithWaitReturnsACopy checks that withWait leaves the facts it is called on as it was, and
// that the waits of two copies stay apart.
func TestWithWaitReturnsACopy(t *testing.T) {
	plain := facts{client: &Client{}}
	short := plain.withWait(syncWait{timeout: time.Second})
	long := short.withWait(syncWait{timeout: time.Minute, policy: SyncPolicyFail})
	if plain.wait != nil {
		t.Errorf("withWait gave the facts it was called on the wait %+v", *plain.wait)
	}
	if short.wait == nil || *short.wait != (syncWait{timeout: time.Second}) {
		t.Errorf("the first copy has the wait %+v, want 1s without a policy", short.wait)
	}
	if long.wait == nil || *long.wait != (syncWait{timeout: time.Minute, policy: SyncPolicyFail}) ||
		long.client != plain.client {
		t.Errorf("the second copy has the wait %+v, want 1m and fail, and the same client",
			long.wait)
	}
}

// TestWaitForSyncMisuse checks the waits that cannot work: from a client that sends facts to the
// API, on an operation that the PDP container does not wait on, and with a negative timeout.
// Each returns an error that matches ErrInvalidConfig and names the operation and the rule, and
// no call sends a request, not even a lookup of the scope.
func TestWaitForSyncMisuse(t *testing.T) {
	wait := syncWait{timeout: time.Second, policy: SyncPolicyFail}
	for _, tc := range []struct {
		name   string
		viaPDP bool
		wait   syncWait
		call   func(ctx context.Context, f facts) error
		op     string
		rule   validate.Rule
	}{
		{
			name: "facts sent to the API", wait: wait, op: opFactsUpdate.ID,
			call: func(ctx context.Context, f facts) error {
				_, err := update(ctx, f, "a")
				return err
			},
			rule: validate.WaitForSyncFactsViaAPI,
		},
		{
			name: "facts sent to the API and an operation that does not wait", wait: wait,
			op: opFactsDelete.ID, call: func(ctx context.Context, f facts) error {
				return deleteObject(ctx, f, "a")
			},
			rule: validate.WaitForSyncFactsViaAPI,
		},
		{
			name: "an operation that does not wait", viaPDP: true, wait: wait,
			op: opFactsDelete.ID, call: func(ctx context.Context, f facts) error {
				return deleteObject(ctx, f, "a")
			},
			rule: validate.WaitForSyncNotWaiting,
		},
		{
			name: "a negative timeout", viaPDP: true, wait: syncWait{timeout: -time.Nanosecond},
			op: opFactsUpdate.ID, call: func(ctx context.Context, f facts) error {
				_, err := update(ctx, f, "a")
				return err
			},
			rule: validate.WaitForSyncTimeoutNegative,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
			var opts []Option
			if tc.viaPDP {
				opts = append(opts, WithFactsViaPDP())
			}
			f := facts{client: factsClient(t, api, pdp, opts...)}.withWait(tc.wait)
			err := tc.call(t.Context(), f)
			want := "permit: invalid configuration: " + tc.op + ": " + tc.rule.Message
			if err == nil || err.Error() != want || !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("the call returned %v, want %q, which matches ErrInvalidConfig", err,
					want)
			}
			api.AssertUntouched()
			pdp.AssertUntouched()
		})
	}
}

// TestPrepareRejectsOtherOperations checks that prepare sends nothing for an operation that is
// not one of facts, such as a set rule under /v2/facts, and for one that the registry does not
// hold, with or without WithFactsViaPDP.
func TestPrepareRejectsOtherOperations(t *testing.T) {
	for _, viaPDP := range []bool{false, true} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		opts := []Option{WithScope("p", "e")}
		if viaPDP {
			opts = append(opts, WithFactsViaPDP())
		}
		f := facts{client: factsClient(t, api, pdp, opts...)}
		for op, want := range map[string]string{
			opSetRule.ID: "permit: test_set_rule is not an operation of the facts API; send it " +
				"to its own server",
			"test_nothing": "permit: the operation test_nothing is not in the client's " +
				"operation registry",
		} {
			r, err := f.prepare(t.Context(), request{op: op})
			if err == nil || err.Error() != want || errors.Is(err, ErrInvalidConfig) {
				t.Errorf("prepare of %s with WithFactsViaPDP %t returned %+v, %v; want %q", op,
					viaPDP, r, err, want)
			}
		}
		api.AssertUntouched()
		pdp.AssertUntouched()
	}
}

// TestOtherOperationsStayOnTheAPI checks that WithFactsViaPDP moves only the Facts operations:
// a call of another operation under /v2/facts, as a set rule is, goes to the API as its method
// sends it, with its route as it is.
func TestOtherOperationsStayOnTheAPI(t *testing.T) {
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	api.Handle(route(opSetRule), mockpermit.Reply{Status: http.StatusNoContent})
	c := factsClient(t, api, pdp, WithScope("p", "e"), WithFactsViaPDP())
	project, environment, err := c.scopeFor(t.Context(), opSetRule.ID)
	if err != nil {
		t.Fatal(err)
	}
	err = c.doNoValue(t.Context(), request{
		op: opSetRule.ID, target: apiTarget, params: []string{project, environment},
	})
	if err != nil {
		t.Fatal(err)
	}
	if requests := api.Requests(); len(requests) != 1 ||
		requests[0].RawPath != "/__test/v2/facts/p/e/set_rules" {
		t.Errorf("the API got %d requests, want one to /__test/v2/facts/p/e/set_rules",
			len(requests))
	}
	pdp.AssertUntouched()
}

// TestFactsConcurrentWaits makes 100 calls at once under -race, which a start barrier releases
// together: the even ones wait for sync, each with a timeout of its own and alternately each
// policy, through copies of one facts, and the odd ones do not wait, through that facts itself.
// Each request must carry the wait of its own call, and a call that does not wait none.
func TestFactsConcurrentWaits(t *testing.T) {
	const calls = 100
	api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
	replies := make([]mockpermit.Reply, calls)
	for i := range replies {
		replies[i] = mockpermit.Reply{Body: `{"key":"k"}`}
	}
	pdp.Handle(pdpUpdateRoute, replies...)
	plain := facts{client: factsClient(t, api, pdp, WithFactsViaPDP())}
	policies := []SyncPolicy{SyncPolicyFail, SyncPolicyIgnore}
	want := func(i int) string {
		if i%2 == 1 {
			return "- -"
		}
		seconds := strconv.FormatFloat(float64(i)/1000, 'f', -1, 64)
		return seconds + " " + string(policies[i/2%2])
	}
	err := concurrent.Run(t, calls, 30*time.Second, func(ctx context.Context, i int) error {
		f := plain
		if i%2 == 0 {
			f = plain.withWait(syncWait{
				timeout: time.Duration(i) * time.Millisecond, policy: policies[i/2%2],
			})
		}
		_, err := update(ctx, f, "k"+strconv.Itoa(i))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	requests := pdp.Requests()
	if len(requests) != calls {
		t.Fatalf("the PDP got %d requests, want %d", len(requests), calls)
	}
	for _, r := range requests {
		i, err := strconv.Atoi(strings.TrimPrefix(r.RawPath, "/__test/facts/objects/k"))
		if err != nil {
			t.Fatalf("a request went to %s: %v", r.RawPath, err)
		}
		if got := waitHeaders(r.Header); got != want(i) {
			t.Errorf("the request of call %d carries the wait %q, want %q", i, got, want(i))
		}
	}
	api.AssertUntouched()
}

// TestFactsAreNotRecorded checks that the calls of the test-only facts operations, to the API and
// to the PDP container, never reach a request record: their routes start with /__test/ either
// way.
func TestFactsAreNotRecorded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record.jsonl")
	file, err := testhook.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := testhook.NewRecorder(file, testhook.DryRun, t.Name())
	if err != nil {
		t.Fatal(err)
	}
	ctx := testhook.WithRecorder(t.Context(), recorder)
	for _, viaPDP := range []bool{false, true} {
		api, pdp := mockpermit.NewAPI(t), mockpermit.NewPDP(t)
		to, route, opts := api, apiUpdateRoute, []Option{WithScope("p", "e")}
		if viaPDP {
			to, route = pdp, pdpUpdateRoute
			opts = append(opts, WithFactsViaPDP())
		}
		to.Handle(route, mockpermit.Reply{Body: `{"key":"a"}`})
		f := facts{client: factsClient(t, api, pdp, opts...)}
		if _, callErr := update(ctx, f, "a"); callErr != nil {
			t.Fatal(callErr)
		}
	}
	if err = file.Close(0); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"type":"header","format":1}` + "\n" +
		`{"type":"session","exit_status":0,"tests":1}` + "\n"
	if string(data) != want {
		t.Errorf("the record holds\n%s\nwant the header and the trailer alone", data)
	}
}

// TestNewWarnsAboutFactsViaTheCloudPDP checks that New warns once, with the PDP URL, when
// WithFactsViaPDP sends facts to the cloud PDP, whether an option or the environment gives its
// URL, and not for a PDP container or without the option.
func TestNewWarnsAboutFactsViaTheCloudPDP(t *testing.T) {
	const cloud = "https://cloudpdp.api.permit.io"
	for _, tc := range []struct {
		name string
		// pdpURL is the PDP URL from an option; empty to leave the option out.
		pdpURL string
		// envPDPURL is the value of PERMIT_PDP_URL; empty to leave it unset.
		envPDPURL string
		viaPDP    bool
		warn      bool
	}{
		{name: "the cloud PDP", pdpURL: cloud, viaPDP: true, warn: true},
		{name: "in upper case", pdpURL: "HTTPS://CLOUDPDP.API.PERMIT.IO", viaPDP: true, warn: true},
		{
			name: "with a port, a path and a dot at the end of the host", viaPDP: true, warn: true,
			pdpURL: "http://cloudpdp.api.permit.io.:443/x/",
		},
		{name: "from the environment", envPDPURL: cloud, viaPDP: true, warn: true},
		{name: "without the option", pdpURL: cloud},
		{name: "a PDP container", pdpURL: "http://pdp.example.com:7766", viaPDP: true},
		{name: "the default PDP URL", viaPDP: true},
		{name: "another host of permit.io", pdpURL: "https://api.permit.io", viaPDP: true},
		{
			name: "a host that ends in the cloud PDP's", viaPDP: true,
			pdpURL: "https://eu.cloudpdp.api.permit.io",
		},
		{
			name: "a host that starts with the cloud PDP's", viaPDP: true,
			pdpURL: "https://cloudpdp.api.permit.io.example.com",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			if tc.envPDPURL != "" {
				t.Setenv(envPDPURL, tc.envPDPURL)
			}
			var out bytes.Buffer
			opts := []Option{
				WithAPIKey(canary.Key("cloudpdp")), WithAPIURL(codeURL),
				WithLogger(slog.New(logHandlers["JSON"](&out))),
			}
			if tc.pdpURL != "" {
				opts = append(opts, WithPDPURL(tc.pdpURL))
			}
			if tc.viaPDP {
				opts = append(opts, WithFactsViaPDP())
			}
			c := mustNew(t, opts...)
			got := jsonLines(t, out.Bytes())
			if !tc.warn {
				if len(got) != 0 {
					t.Errorf("New logged %d lines, want none: %v", len(got), got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("New logged %d lines, want 1 warning: %v", len(got), got)
			}
			if got[0]["level"] != "WARN" || got[0]["url"] != c.pdpURL ||
				!strings.HasPrefix(fmt.Sprint(got[0]["msg"]), "permit: ") {
				t.Errorf("New logged %v, want a warning whose message starts with permit: and "+
					"whose url is %s", got[0], c.pdpURL)
			}
		})
	}
}
