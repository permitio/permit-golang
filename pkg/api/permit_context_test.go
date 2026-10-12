package api_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/permitio/permit-golang/pkg/api"
	"github.com/permitio/permit-golang/pkg/config"
	permitErrors "github.com/permitio/permit-golang/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	scopeJSON      = `{"organization_id":"org","project_id":"proj","environment_id":"env"}`
	scopedUserJSON = `{"key":"u1","id":"11111111-1111-1111-1111-111111111111",` +
		`"organization_id":"org","project_id":"proj","environment_id":"env",` +
		`"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","roles":[]}`
	scopeErrorBody = `{"detail":"scope request rejected"}`
)

type scopeAnswer struct {
	status int
	body   string
}

// scopeAPI is an httptest server that answers GET /v2/api-key/scope with its answers in order,
// repeating the last one, the roles list with an empty list and every other request with a user.
// Scope answers are held until release is closed.
type scopeAPI struct {
	*httptest.Server
	answers       []scopeAnswer
	release       chan struct{}
	releaseOnce   sync.Once
	firstScope    chan struct{}
	scopeRequests atomic.Int32
}

func newScopeAPI(t *testing.T, answers ...scopeAnswer) *scopeAPI {
	s := newGatedScopeAPI(t, answers...)
	s.open()
	return s
}

func newGatedScopeAPI(t *testing.T, answers ...scopeAnswer) *scopeAPI {
	t.Helper()
	s := &scopeAPI{answers: answers, release: make(chan struct{}), firstScope: make(chan struct{})}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	t.Cleanup(s.open)
	return s
}

func (s *scopeAPI) open() {
	s.releaseOnce.Do(func() { close(s.release) })
}

func (s *scopeAPI) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/v2/schema/proj/env/roles" {
		_, _ = w.Write([]byte("[]"))
		return
	}
	if r.URL.Path != "/v2/api-key/scope" {
		_, _ = w.Write([]byte(scopedUserJSON))
		return
	}
	n := int(s.scopeRequests.Add(1))
	if n == 1 {
		close(s.firstScope)
	}
	select {
	case <-s.release:
	case <-r.Context().Done():
		return
	}
	answer := s.answers[len(s.answers)-1]
	if n <= len(s.answers) {
		answer = s.answers[n-1]
	}
	w.WriteHeader(answer.status)
	_, _ = w.Write([]byte(answer.body))
}

func newClientWithoutContext(apiURL string) *api.PermitApiClient {
	cfg := config.NewConfigBuilder("test-token").WithApiUrl(apiURL).WithLogger(zap.NewNop()).Build()
	return api.NewPermitApiClient(&cfg)
}

// waitingContext closes arrived the first time a call asks for its Done channel. A call that waits
// for another call's scope request does that as it starts waiting, and the call that sends the
// request does it while sending, so once a call's context has arrived, that call can no longer
// start a scope request of its own before the scope answer is released.
type waitingContext struct {
	context.Context
	once    sync.Once
	arrived chan struct{}
}

func newWaitingContext() *waitingContext {
	return &waitingContext{Context: context.Background(), arrived: make(chan struct{})}
}

func (c *waitingContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.arrived) })
	return c.Context.Done()
}

// waitArrived fails the test if c's call does not start waiting for the scope request or sending
// it, instead of letting the test hang until its timeout.
func waitArrived(t *testing.T, c *waitingContext) {
	t.Helper()
	select {
	case <-c.arrived:
	case <-time.After(10 * time.Second):
		t.Fatal("a call neither waited for the scope request nor sent it")
	}
}

// waitFirstScope waits until srv has received its first scope request. It fails the test if
// firstCall, the call that sends that request, returns first, or if the request does not arrive
// in time, instead of letting the test hang until its timeout.
func waitFirstScope(t *testing.T, srv *scopeAPI, firstCall <-chan error) {
	t.Helper()
	select {
	case <-srv.firstScope:
	case err := <-firstCall:
		t.Fatalf("the first call returned before its scope request arrived: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("no scope request arrived")
	}
}

// callConcurrently makes n calls at once on client, alternating Users.Get and Roles.List, and
// returns their errors. It releases the scope answers once every call is waiting for the scope
// request or sending it.
func callConcurrently(t *testing.T, srv *scopeAPI, client *api.PermitApiClient, n int) []error {
	t.Helper()
	errs := make([]error, n)
	contexts := make([]*waitingContext, n)
	var finished sync.WaitGroup
	for i := 0; i < n; i++ {
		contexts[i] = newWaitingContext()
		finished.Add(1)
		go func(i int) {
			defer finished.Done()
			if i%2 == 0 {
				_, errs[i] = client.Users.Get(contexts[i], "u1")
			} else {
				_, errs[i] = client.Roles.List(contexts[i], 1, 10)
			}
		}(i)
	}
	for _, ctx := range contexts {
		waitArrived(t, ctx)
	}
	srv.open()
	finished.Wait()
	return errs
}

func requireContextError(t *testing.T, err error) permitErrors.PermitError {
	t.Helper()
	var permitErr permitErrors.PermitError
	require.True(t, errors.As(err, &permitErr), "want a PermitError, got %T: %v", err, err)
	require.Equal(t, permitErrors.ContextError, permitErr.ErrorCode, err.Error())
	return permitErr
}

func TestPermitContextLoadsOnceForConcurrentCalls(t *testing.T) {
	srv := newGatedScopeAPI(t, scopeAnswer{http.StatusOK, scopeJSON})
	client := newClientWithoutContext(srv.URL)

	for i, err := range callConcurrently(t, srv, client, 50) {
		require.NoError(t, err, "call %d", i)
	}
	_, err := client.Users.Get(context.Background(), "u1")
	require.NoError(t, err)

	assert.Equal(t, int32(1), srv.scopeRequests.Load())
}

func TestPermitContextLoadFailureIsSharedThenRetried(t *testing.T) {
	srv := newGatedScopeAPI(t,
		scopeAnswer{http.StatusInternalServerError, scopeErrorBody},
		scopeAnswer{http.StatusOK, scopeJSON})
	client := newClientWithoutContext(srv.URL)

	for _, err := range callConcurrently(t, srv, client, 50) {
		permitErr := requireContextError(t, err)
		assert.Equal(t, http.StatusInternalServerError, permitErr.StatusCode)
	}
	require.Equal(t, int32(1), srv.scopeRequests.Load())

	_, err := client.Users.Get(context.Background(), "u1")
	require.NoError(t, err)
	_, err = client.Users.Get(context.Background(), "u1")
	require.NoError(t, err)
	assert.Equal(t, int32(2), srv.scopeRequests.Load())
}

func TestPermitContextLoadErrorKeepsStatusBodyAndCause(t *testing.T) {
	tests := []struct {
		status    int
		wantCause permitErrors.ErrorCode
	}{
		{status: http.StatusUnauthorized, wantCause: permitErrors.Unauthorized},
		{status: http.StatusForbidden, wantCause: permitErrors.ForbiddenAccess},
		{status: http.StatusTooManyRequests, wantCause: permitErrors.UnexpectedError},
		{status: http.StatusInternalServerError, wantCause: permitErrors.UnexpectedError},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.status), func(t *testing.T) {
			srv := newScopeAPI(t, scopeAnswer{tt.status, scopeErrorBody})
			client := newClientWithoutContext(srv.URL)

			_, err := client.Users.Get(context.Background(), "u1")

			permitErr := requireContextError(t, err)
			assert.Equal(t, tt.status, permitErr.StatusCode)
			assert.Equal(t, scopeErrorBody, permitErr.ResponseBody)
			assert.Contains(t, err.Error(), fmt.Sprintf("(status %d)", tt.status))
			cause := errors.Unwrap(err)
			require.NotNil(t, cause)
			var causeErr permitErrors.PermitError
			require.True(t, errors.As(cause, &causeErr))
			assert.Equal(t, tt.wantCause, causeErr.ErrorCode)
			assert.Equal(t, tt.status, causeErr.StatusCode)
			assert.Contains(t, err.Error(), causeErr.Error())
		})
	}
}

func TestPermitContextLoadErrorWrapsDialError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	client := newClientWithoutContext(srv.URL)

	_, err := client.Users.Get(context.Background(), "u1")

	permitErr := requireContextError(t, err)
	assert.Zero(t, permitErr.StatusCode)
	var opErr *net.OpError
	require.True(t, errors.As(err, &opErr), "want a *net.OpError cause: %v", err)
	assert.Equal(t, "dial", opErr.Op)
	assert.Contains(t, err.Error(), opErr.Error())
}

func TestPermitContextLoadWithCanceledContextSendsNoRequest(t *testing.T) {
	srv := newScopeAPI(t, scopeAnswer{http.StatusOK, scopeJSON})
	client := newClientWithoutContext(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Users.Get(ctx, "u1")

	requireContextError(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int32(0), srv.scopeRequests.Load())
}

func TestPermitContextLoadCanceledWhileWaiting(t *testing.T) {
	srv := newGatedScopeAPI(t, scopeAnswer{http.StatusOK, scopeJSON})
	client := newClientWithoutContext(srv.URL)
	firstCall := make(chan error, 1)
	go func() {
		_, err := client.Users.Get(context.Background(), "u1")
		firstCall <- err
	}()
	waitFirstScope(t, srv, firstCall)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Users.Get(ctx, "u1")

	requireContextError(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	srv.open()
	require.NoError(t, <-firstCall)
	assert.Equal(t, int32(1), srv.scopeRequests.Load())
}

func TestPermitContextLoadRetriedWhenSenderCanceled(t *testing.T) {
	srv := newGatedScopeAPI(t, scopeAnswer{http.StatusOK, scopeJSON})
	client := newClientWithoutContext(srv.URL)
	senderCtx, cancelSender := context.WithCancel(context.Background())
	senderCall := make(chan error, 1)
	go func() {
		_, err := client.Users.Get(senderCtx, "u1")
		senderCall <- err
	}()
	waitFirstScope(t, srv, senderCall)
	waiterCtx := newWaitingContext()
	waiterCall := make(chan error, 1)
	go func() {
		_, err := client.Users.Get(waiterCtx, "u1")
		waiterCall <- err
	}()
	waitArrived(t, waiterCtx)

	cancelSender()
	senderErr := <-senderCall
	srv.open()

	requireContextError(t, senderErr)
	assert.ErrorIs(t, senderErr, context.Canceled)
	require.NoError(t, <-waiterCall)
	assert.Equal(t, int32(2), srv.scopeRequests.Load())
}

func TestPermitContextLoadSharesResultOnlyWithinItsConfig(t *testing.T) {
	failingSrv := newGatedScopeAPI(t, scopeAnswer{http.StatusInternalServerError, scopeErrorBody})
	healthySrv := newGatedScopeAPI(t, scopeAnswer{http.StatusOK, scopeJSON})
	failingClient := newClientWithoutContext(failingSrv.URL)
	healthyClient := newClientWithoutContext(healthySrv.URL)
	failingCall := make(chan error, 1)
	go func() {
		_, err := failingClient.Users.Get(context.Background(), "u1")
		failingCall <- err
	}()
	waitFirstScope(t, failingSrv, failingCall)
	healthyCtx := newWaitingContext()
	healthyCall := make(chan error, 1)
	go func() {
		_, err := healthyClient.Users.Get(healthyCtx, "u1")
		healthyCall <- err
	}()
	waitArrived(t, healthyCtx)

	failingSrv.open()
	requireContextError(t, <-failingCall)
	healthySrv.open()

	require.NoError(t, <-healthyCall)
	assert.Equal(t, int32(1), failingSrv.scopeRequests.Load())
	assert.Equal(t, int32(1), healthySrv.scopeRequests.Load())
}
