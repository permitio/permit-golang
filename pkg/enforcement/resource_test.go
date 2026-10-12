package enforcement_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/pkg/enforcement"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// idOnlyObject implements ResourceI with an ID and no Key, as application types usually do.
type idOnlyObject struct {
	id string
}

func (o idOnlyObject) GetID() string                         { return o.id }
func (o idOnlyObject) GetType() string                       { return "doc" }
func (o idOnlyObject) GetTenant() string                     { return "t1" }
func (o idOnlyObject) GetAttributes() map[string]interface{} { return nil }
func (o idOnlyObject) GetContext() map[string]string         { return nil }

func TestChecksSendIDAsKeyWhenKeyIsEmpty(t *testing.T) {
	user := enforcement.UserBuilder("u1").Build()
	withID := enforcement.ResourceBuilder("doc").WithID("doc-1").Build()
	withIDAndKey := enforcement.ResourceBuilder("doc").WithID("legacy-id").WithKey("doc-1").Build()
	const (
		withIDJSON       = `{"type":"doc","id":"doc-1","key":"doc-1","tenant":"default"}`
		withIDAndKeyJSON = `{"type":"doc","id":"legacy-id","key":"doc-1","tenant":"default"}`
	)
	tests := []struct {
		name     string
		response string
		call     func(e *enforcement.PermitEnforcer) error
		wantPath string
		wantBody string
	}{
		{
			name:     "Check with WithID",
			response: `{"allow":true}`,
			call: func(e *enforcement.PermitEnforcer) error {
				_, err := e.Check(user, "read", withID)
				return err
			},
			wantPath: "/allowed",
			wantBody: `{"user":{"key":"u1"},"action":"read","resource":` + withIDJSON + `}`,
		},
		{
			name:     "Check with WithID and WithKey",
			response: `{"allow":true}`,
			call: func(e *enforcement.PermitEnforcer) error {
				_, err := e.Check(user, "read", withIDAndKey)
				return err
			},
			wantPath: "/allowed",
			wantBody: `{"user":{"key":"u1"},"action":"read","resource":` + withIDAndKeyJSON + `}`,
		},
		{
			name:     "BulkCheck with WithID",
			response: `{"allow":[{"allow":true}]}`,
			call: func(e *enforcement.PermitEnforcer) error {
				_, err := e.BulkCheck(*enforcement.NewCheckRequest(user, "read", withID, nil))
				return err
			},
			wantPath: "/allowed/bulk",
			wantBody: `[{"user":{"key":"u1"},"action":"read","resource":` + withIDJSON + `}]`,
		},
		{
			name:     "AllTenantsCheck with WithID",
			response: `{"allowed_tenants":[]}`,
			call: func(e *enforcement.PermitEnforcer) error {
				_, err := e.AllTenantsCheck(user, "read", withID)
				return err
			},
			wantPath: "/allowed/all-tenants",
			wantBody: `{"user":{"key":"u1"},"action":"read","resource":` + withIDJSON + `}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdp := newFakePDP(t, http.StatusOK, tt.response)

			require.NoError(t, tt.call(newTestEnforcer(pdp.URL)))

			assert.Equal(t, []pdpRequest{{tt.wantPath, tt.wantBody}}, pdp.recorded())
		})
	}
}

func TestFilterObjectsSendsEachObjectKey(t *testing.T) {
	user := enforcement.UserBuilder("u1").Build()
	tests := []struct {
		name          string
		objects       []enforcement.ResourceI
		wantResources []string
	}{
		{
			name: "built with WithKey",
			objects: []enforcement.ResourceI{
				enforcement.ResourceBuilder("doc").WithKey("doc-a").WithTenant("t1"),
				enforcement.ResourceBuilder("doc").WithKey("doc-b").WithTenant("t1"),
			},
			wantResources: []string{
				`{"type":"doc","key":"doc-a","tenant":"t1"}`,
				`{"type":"doc","key":"doc-b","tenant":"t1"}`,
			},
		},
		{
			name: "built with WithID",
			objects: []enforcement.ResourceI{
				enforcement.ResourceBuilder("doc").WithID("doc-a").WithTenant("t1"),
				enforcement.ResourceBuilder("doc").WithID("doc-b").WithTenant("t1"),
			},
			wantResources: []string{
				`{"type":"doc","id":"doc-a","key":"doc-a","tenant":"t1"}`,
				`{"type":"doc","id":"doc-b","key":"doc-b","tenant":"t1"}`,
			},
		},
		{
			name:    "application type with an ID",
			objects: []enforcement.ResourceI{idOnlyObject{id: "doc-a"}, idOnlyObject{id: "doc-b"}},
			wantResources: []string{
				`{"type":"doc","id":"doc-a","key":"doc-a","tenant":"t1"}`,
				`{"type":"doc","id":"doc-b","key":"doc-b","tenant":"t1"}`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdp := newFakePDP(t, http.StatusOK, `{"allow":[{"allow":true},{"allow":false}]}`)

			allowed, err := newTestEnforcer(pdp.URL).
				FilterObjects(user, "read", map[string]string{"region": "eu"}, tt.objects...)

			require.NoError(t, err)
			assert.Equal(t, tt.objects[:1], allowed)
			checks := make([]string, len(tt.wantResources))
			for i, resource := range tt.wantResources {
				checks[i] = `{"user":{"key":"u1"},"action":"read","resource":` + resource +
					`,"context":{"region":"eu"}}`
			}
			wantBody := "[" + strings.Join(checks, ",") + "]"
			assert.Equal(t, []pdpRequest{{"/allowed/bulk", wantBody}}, pdp.recorded())
		})
	}
}
