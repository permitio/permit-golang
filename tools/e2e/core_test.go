//go:build e2e

package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	permit "github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/canary"
)

// scopeRoute is the route of the lookup of an API key's scope.
const scopeRoute = "/v2/api-key/scope"

// scopeLookups is an http.RoundTripper that counts the lookups of a scope that go through it.
type scopeLookups struct {
	n atomic.Int64
}

func (l *scopeLookups) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodGet && r.URL.Path == scopeRoute {
		l.n.Add(1)
	}
	return http.DefaultTransport.RoundTrip(r)
}

// scopeSingleFlight checks that the callers of Client.Scope that come while a lookup runs wait
// for that one lookup, which finds the scratch environment, and that the client keeps the scope.
func scopeSingleFlight(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		lookups := &scopeLookups{}
		c := s.client(t, s.env.APIKey(), permit.WithHTTPClient(&http.Client{Transport: lookups}))
		ctx := s.ctx(t)
		const callers = 16
		scopes := make([]permit.Scope, callers)
		errs := make([]error, callers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range callers {
			wg.Go(func() {
				<-start
				scopes[i], errs[i] = c.Scope(ctx)
			})
		}
		close(start)
		wg.Wait()
		want := permit.Scope{Project: s.env.ProjectID, Environment: s.env.ID}
		for i := range callers {
			if errs[i] != nil || scopes[i] != want {
				t.Errorf("caller %d got %+v, %v; want %+v", i, scopes[i], errs[i], want)
			}
		}
		if _, err := c.Scope(ctx); err != nil {
			t.Errorf("Scope after the lookup: %v", err)
		}
		if n := lookups.n.Load(); n != 1 {
			t.Errorf("%d callers and one more sent %d lookups of the scope, want 1", callers, n)
		}
	}
}

// unauthorizedKey checks that a key that the API does not know fails Client.Scope with a typed
// error of status 401 that does not hold the key.
func unauthorizedKey(s *suite) func(t *testing.T) {
	return func(t *testing.T) { //nolint:thelper // A subtest that runCloud runs.
		key := canary.Key("e2eunknown")
		s.secrets = append(s.secrets, key)
		_, err := s.client(t, key).Scope(s.ctx(t))
		s.logger.Debug("a lookup with an unknown key failed", "error", err)
		if !errors.Is(err, permit.ErrUnauthorized) {
			t.Fatalf("Scope with an unknown key returned %v, want an error that matches "+
				"ErrUnauthorized", err)
		}
		perr, ok := errors.AsType[*permit.Error](err)
		if !ok || perr.StatusCode != http.StatusUnauthorized || perr.Op != "get_api_key_scope" ||
			perr.Method != http.MethodGet || perr.Route != scopeRoute {
			t.Errorf("Scope with an unknown key returned %#v, want a *permit.Error of status "+
				"401 for get_api_key_scope, GET %s", err, scopeRoute)
		}
		if strings.Contains(fmt.Sprintf("%+v", err), key) {
			t.Error("the error of a lookup with an unknown key holds the key")
		}
	}
}

// leak is a secret that logs hold: its index in the secrets of the scan, and how often they hold
// it.
type leak struct {
	key, count int
}

// leaks returns each of keys that logs hold, with how often they hold it.
func leaks(logs []byte, keys []string) []leak {
	var found []leak
	for i, key := range keys {
		if n := bytes.Count(logs, []byte(key)); n != 0 {
			found = append(found, leak{key: i, count: n})
		}
	}
	return found
}

// minScannedKey is the length of the shortest secret that logsHoldNoKey scans for: a shorter one
// is a mistake of the run, as a scan for it could match by chance.
const minScannedKey = 20

// logsHoldNoKey checks that what every client of the suite logged at Debug holds no API key of
// the run, nor a credential of a PDP that a scenario read, and, as a control, that the same scan
// finds each of them exactly once in what a logger built like the clients' logs when it logs it
// at Debug.
func logsHoldNoKey(s *suite) func(t *testing.T) {
	return func(t *testing.T) {
		logs := s.logs.Bytes()
		if len(logs) == 0 {
			t.Fatal("the clients of the run logged nothing at Debug, so the scan proves nothing")
		}
		for i, key := range s.secrets {
			if len(key) < minScannedKey {
				t.Errorf("secret %d of the run has %d characters; the scan needs %d or more", i,
					len(key), minScannedKey)
			}
		}
		for _, l := range leaks(logs, s.secrets) {
			t.Errorf("the Debug logs of the run hold secret %d %d times", l.key, l.count)
		}
		var control bytes.Buffer
		logger := newLogger(&control)
		want := make([]leak, len(s.secrets))
		for i, key := range s.secrets {
			logger.Debug("a control line", "key", key)
			want[i] = leak{key: i, count: 1}
		}
		if got := leaks(control.Bytes(), s.secrets); !slices.Equal(got, want) {
			t.Errorf("the scan of a control log that holds each secret once found %v, want %v",
				got, want)
		}
		t.Logf("scanned %d bytes of Debug logs for %d secrets", len(logs), len(s.secrets))
	}
}
