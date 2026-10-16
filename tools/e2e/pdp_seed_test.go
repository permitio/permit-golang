//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/permitio/permit-golang/v2/tools/e2e/framework"
	"github.com/permitio/permit-golang/v2/tools/e2e/framework/seed"
)

// world is what a PDP scenario seeds in the scratch environment, which every scenario of a run
// shares: so the keys of one scenario's objects are its own.
type world struct {
	resources     []seed.Resource
	roles         []seed.Role
	resourceRoles []seed.ResourceRole
	tenants       []string
	users         []string
	instances     []seed.Instance
	assignments   []seed.RoleAssignment
}

// seed creates the objects of w in the scratch environment with the seed builder, each kind after
// those it refers to, in the ledger of the suite. It fails t at its setup, before the scenario
// asserts anything, when an object cannot be created.
func (s *suite) seed(t *testing.T, w world) {
	t.Helper()
	ctx := t.Context()
	b := seed.New(s.api, s.ledger, s.env)
	var calls []func() error
	for _, r := range w.resources {
		calls = append(calls, func() error { return b.Resource(ctx, r) })
	}
	for _, r := range w.roles {
		calls = append(calls, func() error { return b.Role(ctx, r) })
	}
	for _, r := range w.resourceRoles {
		calls = append(calls, func() error { return b.ResourceRole(ctx, r) })
	}
	for _, key := range w.tenants {
		calls = append(calls, func() error { return b.Tenant(ctx, key) })
	}
	for _, key := range w.users {
		calls = append(calls, func() error { return b.User(ctx, key) })
	}
	for _, i := range w.instances {
		calls = append(calls, func() error { return b.Instance(ctx, i) })
	}
	for _, a := range w.assignments {
		calls = append(calls, func() error { return b.RoleAssignment(ctx, a) })
	}
	for _, call := range calls {
		if err := call(); err != nil {
			t.Fatalf("setup: seeding the scenario: %v", err)
		}
	}
}

// await calls get until it returns want, as awaitValue does, with the sync timeout and interval
// of the suite, as the seed of a scenario reaches a live PDP only after a while, and fails t
// with what awaitValue says when get never returns want. A scenario awaits a decision that only
// its seed allows, so that the checks after it, which it makes once, run after the seed reached
// the PDP: a check that the seed denies passes before then too.
func await[T any](t *testing.T, s *suite, what string, want T,
	get func(context.Context) (T, error),
) {
	t.Helper()
	if err := awaitValue(s.ctx(t), s.cfg, want, get); err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

// awaitValue calls get, with ctx, until it returns want and no error, as reflect.DeepEqual
// compares them, for at most cfg.syncTimeout, with cfg.syncInterval between two calls. When get
// never does, it returns an error with the last answer of get and the error of its last call, as
// the last call can run out of time.
func awaitValue[T any](ctx context.Context, cfg config, want T,
	get func(context.Context) (T, error),
) error {
	var last T
	answered := false
	var lastErr error
	err := framework.Poll(ctx, cfg.syncTimeout, cfg.syncInterval,
		func(ctx context.Context) (bool, error) {
			got, err := get(ctx)
			lastErr = err
			if err == nil {
				last, answered = got, true
			}
			return err == nil && reflect.DeepEqual(got, want), nil
		})
	if err == nil {
		return nil
	}
	if lastErr != nil {
		err = fmt.Errorf("%w; the last call returned %w", err, lastErr)
	}
	if !answered {
		return fmt.Errorf("no call answered: %w", err)
	}
	return fmt.Errorf("got %+v, want %+v: %w", last, want, err)
}

// TestAwaitValue checks the wait for a value: it ends at the first call that returns the value
// without an error, and else fails with the last answer, or says that no call answered, and with
// the error of the last call, which ran before the wait's context ended.
func TestAwaitValue(t *testing.T) {
	cfg := config{syncTimeout: 300 * time.Millisecond, syncInterval: 10 * time.Millisecond}
	failure := errors.New("the PDP is not ready")
	for _, tc := range []struct {
		name string

		// answers are what the calls return, in turn; the last one again after them.
		answers []int
		errs    []error

		calls int

		// want are the texts of the error, none for no error.
		want []string
	}{
		{name: "at once", answers: []int{2}, errs: []error{nil}, calls: 1},
		{
			name: "on the third call", answers: []int{0, 1, 2}, errs: []error{nil, nil, nil},
			calls: 3,
		},
		{name: "after an error", answers: []int{0, 2}, errs: []error{failure, nil}, calls: 2},
		{
			name: "never", answers: []int{1}, errs: []error{nil},
			want: []string{"got 1, want 2: framework: not done after 300ms ("},
		},
		{
			name: "the value with an error", answers: []int{2}, errs: []error{failure},
			want: []string{
				"no call answered: framework: not done after 300ms (",
				"; the last call returned the PDP is not ready",
			},
		},
		{
			name: "an answer, then errors", answers: []int{1, 0}, errs: []error{nil, failure},
			want: []string{
				"got 1, want 2: framework: not done after 300ms (",
				"; the last call returned the PDP is not ready",
			},
		},
	} {
		calls := 0
		err := awaitValue(t.Context(), cfg, 2, func(context.Context) (int, error) {
			i := min(calls, len(tc.answers)-1)
			calls++
			return tc.answers[i], tc.errs[i]
		})
		if tc.want == nil && (err != nil || calls != tc.calls) {
			t.Errorf("%s: awaitValue returned %v after %d calls, want nil after %d", tc.name, err,
				calls, tc.calls)
		}
		for _, want := range tc.want {
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("%s: awaitValue returned %v, want an error with %q", tc.name, err, want)
			}
		}
		if tc.want != nil && errors.Is(err, failure) != slices.Contains(tc.errs, failure) {
			t.Errorf("%s: awaitValue returned %v, which wraps the error of the last call: %t",
				tc.name, err, errors.Is(err, failure))
		}
	}
	cfg = config{syncTimeout: 50 * time.Millisecond, syncInterval: time.Hour}
	err := awaitValue(t.Context(), cfg, 2, func(ctx context.Context) (int, error) {
		if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
			return 0, context.DeadlineExceeded
		}
		return 0, failure
	})
	if !errors.Is(err, failure) || errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("awaitValue of a call that fails once its context ends returned %v, want the "+
			"error of the last call, %v, and not the context's", err, failure)
	}
}

// sent is a request that a client of a scenario sent: its path, its query, its body and the
// status of its reply.
type sent struct {
	path   string
	query  url.Values
	body   []byte
	status int
}

// String returns the path of r, the status of its reply and its body, for a message.
func (r sent) String() string {
	return fmt.Sprintf("%s answered %d to %s", r.path, r.status, r.body)
}

// requestLog is an http.RoundTripper that keeps each request that goes through it, with its
// body, which it reads from a copy, and the status of its reply.
type requestLog struct {
	mu       sync.Mutex
	requests []sent
}

func (l *requestLog) RoundTrip(r *http.Request) (*http.Response, error) {
	var body []byte
	if r.GetBody != nil {
		copied, err := r.GetBody()
		if err != nil {
			return nil, fmt.Errorf("copying the body of %s %s: %w", r.Method, r.URL.Path, err)
		}
		body, err = io.ReadAll(copied)
		if err != nil {
			return nil, fmt.Errorf("reading the body of %s %s: %w", r.Method, r.URL.Path, err)
		}
	}
	resp, err := http.DefaultTransport.RoundTrip(r)
	if err != nil {
		l.add(sent{path: r.URL.Path, query: r.URL.Query(), body: body})
		return nil, err
	}
	l.add(sent{path: r.URL.Path, query: r.URL.Query(), body: body, status: resp.StatusCode})
	return resp, nil
}

// add keeps r.
func (l *requestLog) add(r sent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.requests = append(l.requests, r)
}

// to returns the requests that went to path, in order.
func (l *requestLog) to(path string) []sent {
	l.mu.Lock()
	defer l.mu.Unlock()
	var list []sent
	for _, r := range l.requests {
		if r.path == path {
			list = append(list, r)
		}
	}
	return list
}

// sortedKeys returns the keys of tenants, sorted, as the PDP lists tenants in an order of its
// own.
func sortedKeys[T any](tenants []T, key func(T) string) []string {
	keys := make([]string, len(tenants))
	for i, tenant := range tenants {
		keys[i] = key(tenant)
	}
	slices.Sort(keys)
	return keys
}
