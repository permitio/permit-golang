package enforcement_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/enforcement"
	permitErrors "github.com/permitio/permit-golang/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type pdpRequest struct {
	path string
	body string
}

// fakePDP is an httptest server that records every request and answers each one with the same
// status and body.
type fakePDP struct {
	*httptest.Server
	mu       sync.Mutex
	requests []pdpRequest
}

func newFakePDP(t *testing.T, status int, body string) *fakePDP {
	t.Helper()
	f := &fakePDP{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestBody, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.requests = append(f.requests, pdpRequest{r.URL.Path, string(requestBody)})
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakePDP) recorded() []pdpRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]pdpRequest(nil), f.requests...)
}

func newTestEnforcer(pdpURL string) *enforcement.PermitEnforcer {
	cfg := config.NewConfigBuilder("test-token").WithPdpUrl(pdpURL).WithLogger(zap.NewNop()).Build()
	return enforcement.NewPermitEnforcerClient(&cfg)
}

func TestPDPErrorStatusReturnsPermitErrorWithBody(t *testing.T) {
	const body = `{"detail":"the PDP rejected the request"}`
	user := enforcement.UserBuilder("u1").Build()
	resource := enforcement.ResourceBuilder("doc").WithKey("doc-1").Build()
	statuses := []struct {
		status   int
		wantCode permitErrors.ErrorCode
	}{
		{status: http.StatusBadRequest, wantCode: permitErrors.UnexpectedError},
		{status: http.StatusForbidden, wantCode: permitErrors.ForbiddenAccess},
		{status: http.StatusMisdirectedRequest, wantCode: permitErrors.UnexpectedError},
		{status: http.StatusTooManyRequests, wantCode: permitErrors.UnexpectedError},
		{status: http.StatusInternalServerError, wantCode: permitErrors.UnexpectedError},
	}
	// Each call reports whether it returned any allow, so a failure can never read as a grant.
	calls := []struct {
		name string
		call func(e *enforcement.PermitEnforcer) (bool, error)
	}{
		{name: "Check", call: func(e *enforcement.PermitEnforcer) (bool, error) {
			return e.Check(user, "read", resource)
		}},
		{name: "BulkCheck", call: func(e *enforcement.PermitEnforcer) (bool, error) {
			results, err := e.BulkCheck(*enforcement.NewCheckRequest(user, "read", resource, nil))
			return len(results) > 0, err
		}},
		{name: "FilterObjects", call: func(e *enforcement.PermitEnforcer) (bool, error) {
			objects, err := e.FilterObjects(user, "read", nil, &resource)
			return len(objects) > 0, err
		}},
		{name: "AllTenantsCheck", call: func(e *enforcement.PermitEnforcer) (bool, error) {
			tenants, err := e.AllTenantsCheck(user, "read", resource)
			return len(tenants) > 0, err
		}},
		{name: "GetUserPermissions", call: func(e *enforcement.PermitEnforcer) (bool, error) {
			permissions, err := e.GetUserPermissions(user, "t1")
			return len(permissions) > 0, err
		}},
		{name: "GetUserPermissionsWithOptions", call: func(e *enforcement.PermitEnforcer) (bool, error) {
			permissions, err := e.GetUserPermissionsWithOptions(user)
			return len(permissions) > 0, err
		}},
	}
	for _, s := range statuses {
		for _, c := range calls {
			t.Run(fmt.Sprintf("%s/%d", c.name, s.status), func(t *testing.T) {
				pdp := newFakePDP(t, s.status, body)

				granted, err := c.call(newTestEnforcer(pdp.URL))

				assert.False(t, granted)
				var permitErr permitErrors.PermitError
				require.True(t, errors.As(err, &permitErr), "want a PermitError, got %T: %v", err, err)
				assert.Equal(t, s.wantCode, permitErr.ErrorCode)
				assert.Equal(t, s.status, permitErr.StatusCode)
				assert.Equal(t, body, permitErr.ResponseBody)
			})
		}
	}
}
