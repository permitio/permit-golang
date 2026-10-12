package api_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/permitio/permit-golang/pkg/api"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const (
	testToken = "test-token"
	userJSON  = `{"key":"u1","id":"11111111-1111-1111-1111-111111111111",` +
		`"organization_id":"org","project_id":"proj","environment_id":"env",` +
		`"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","roles":[]}`
)

type recordedRequest struct {
	method string
	path   string
	query  url.Values
	header http.Header
}

// fakeAPI is an httptest server that records every request and answers each one with the same
// status and body.
type fakeAPI struct {
	*httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
}

func newFakeAPI(t *testing.T, status int, body string) *fakeAPI {
	t.Helper()
	f := &fakeAPI{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, recordedRequest{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.Query(),
			header: r.Header.Clone(),
		})
		f.mu.Unlock()
		if body != "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeAPI) recorded() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedRequest(nil), f.requests...)
}

func newTestConfig(apiURL string) *config.PermitConfig {
	return config.NewConfigBuilder(testToken).
		WithApiUrl(apiURL).
		WithContext(config.NewPermitContext(config.EnvironmentAPIKeyLevel, "proj", "env")).
		WithLogger(zap.NewNop())
}

func newTestClient(cfg *config.PermitConfig) *api.PermitApiClient {
	built := cfg.Build()
	return api.NewPermitApiClient(&built)
}

// newListClient returns a client whose API answers every request with 200 and body.
func newListClient(t *testing.T, body string) (*api.PermitApiClient, *fakeAPI) {
	t.Helper()
	srv := newFakeAPI(t, http.StatusOK, body)
	return newTestClient(newTestConfig(srv.URL)), srv
}

func TestWaitForSyncHeadersOnlyOnReturnedClient(t *testing.T) {
	srv := newFakeAPI(t, http.StatusOK, userJSON)
	client := newTestClient(newTestConfig(srv.URL).WithProxyFactsViaPDP(true))
	ctx := context.Background()

	timeout := 7 * time.Second
	options := api.WaitForSyncOptions{Policy: config.FactsSyncTimeoutPolicyFail}
	_, err := client.Users.WaitForSync(&timeout, options).Get(ctx, "waited")
	require.NoError(t, err)
	_, err = client.Users.Get(ctx, "plain")
	require.NoError(t, err)

	requests := srv.recorded()
	require.Len(t, requests, 2)
	assert.Equal(t, "7", requests[0].header.Get("X-Wait-Timeout"))
	assert.Equal(t, "fail", requests[0].header.Get("X-Timeout-Policy"))
	assert.Equal(t, "Bearer "+testToken, requests[0].header.Get("Authorization"))
	assert.NotContains(t, requests[1].header, "X-Wait-Timeout")
	assert.NotContains(t, requests[1].header, "X-Timeout-Policy")
	assert.Equal(t, "Bearer "+testToken, requests[1].header.Get("Authorization"))
}

func TestWaitForSyncSendsTimeoutInDecimalSeconds(t *testing.T) {
	durationPtr := func(d time.Duration) *time.Duration { return &d }
	tests := []struct {
		name          string
		timeout       *time.Duration
		configTimeout time.Duration
		want          string
	}{
		{name: "whole seconds", timeout: durationPtr(7 * time.Second), want: "7"},
		{name: "fractional seconds", timeout: durationPtr(1500 * time.Millisecond), want: "1.5"},
		{name: "under one second", timeout: durationPtr(250 * time.Millisecond), want: "0.25"},
		{name: "milliseconds", timeout: durationPtr(1118 * time.Millisecond), want: "1.118"},
		{name: "one nanosecond", timeout: durationPtr(time.Nanosecond), want: "0.000000001"},
		{name: "large", timeout: durationPtr(1000000 * time.Second), want: "1000000"},
		{name: "zero", timeout: durationPtr(0), want: "0"},
		{name: "builder default", want: "10"},
		{name: "configured timeout", configTimeout: 2500 * time.Millisecond, want: "2.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newFakeAPI(t, http.StatusOK, userJSON)
			cfg := newTestConfig(srv.URL).WithProxyFactsViaPDP(true)
			if tt.configTimeout != 0 {
				cfg = cfg.WithFactsSyncTimeout(tt.configTimeout)
			}
			client := newTestClient(cfg)

			_, err := client.Users.WaitForSync(tt.timeout, api.WaitForSyncOptions{}).
				Get(context.Background(), "u1")
			require.NoError(t, err)

			requests := srv.recorded()
			require.Len(t, requests, 1)
			assert.Equal(t, tt.want, requests[0].header.Get("X-Wait-Timeout"))
		})
	}
}

func TestNewFactsClientConfigSendsFractionalTimeout(t *testing.T) {
	cfg := newTestConfig("http://127.0.0.1:1").
		WithProxyFactsViaPDP(true).
		WithFactsSyncTimeout(1500 * time.Millisecond).
		Build()

	clientConfig := api.NewFactsClientConfig(&cfg)

	assert.Equal(t, "1.5", clientConfig.DefaultHeader["X-Wait-Timeout"])
}

func TestWaitForSyncConcurrentUseKeepsHeadersApart(t *testing.T) {
	const goroutines = 20
	srv := newFakeAPI(t, http.StatusOK, userJSON)
	client := newTestClient(newTestConfig(srv.URL).WithProxyFactsViaPDP(true))
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make(chan error, 2*goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			timeout := time.Duration(i) * time.Second
			_, err := client.Users.WaitForSync(&timeout, api.WaitForSyncOptions{}).
				Get(ctx, fmt.Sprintf("waited-%d", i))
			errs <- err
		}(i)
		go func(i int) {
			defer wg.Done()
			_, err := client.Users.Get(ctx, fmt.Sprintf("plain-%d", i))
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	requests := srv.recorded()
	require.Len(t, requests, 2*goroutines)
	for _, req := range requests {
		userKey := path.Base(req.path)
		if strings.HasPrefix(userKey, "waited-") {
			want := strings.TrimPrefix(userKey, "waited-")
			assert.Equal(t, want, req.header.Get("X-Wait-Timeout"), userKey)
		} else {
			assert.NotContains(t, req.header, "X-Wait-Timeout", userKey)
		}
	}
}
