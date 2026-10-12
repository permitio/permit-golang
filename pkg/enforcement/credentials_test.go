package enforcement_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/enforcement"
	permitErrors "github.com/permitio/permit-golang/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const (
	credentialsTestKey = "configured-key-3b7e15"
	// pdpBodyMarker is a part of each PDP body below that holds no credential, so a log line that
	// contains it logged the body.
	pdpBodyMarker = "PDP rejected the query"
)

// credentialValues are the API key and the values that the PDP bodies below put under credential
// keys; none of them may reach an error or a log line.
var credentialValues = []string{
	credentialsTestKey,
	"permit_key_FAKEechoed12345",
	"proxy-secret-value",
	"bearer-token-value",
	"nested-password-value",
}

const credentialFields = `"secret":"proxy-secret-value","token":"bearer-token-value",` +
	`"auth":{"password":"nested-password-value"}`

func newObservedEnforcer(pdpURL string) (*enforcement.PermitEnforcer, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	cfg := config.NewConfigBuilder(credentialsTestKey).
		WithPdpUrl(pdpURL).
		WithLogger(zap.New(core)).
		Build()
	return enforcement.NewPermitEnforcerClient(&cfg), logs
}

// undecodableBody returns a PDP body with the credential fields in which field holds a string, a
// type that no PDP response has there.
func undecodableBody(field string) string {
	return `{"` + field + `":"` + pdpBodyMarker + `",` + credentialFields + `}`
}

func TestPDPErrorsAndLogsHoldNoCredentials(t *testing.T) {
	user := enforcement.UserBuilder("u1").Build()
	resource := enforcement.ResourceBuilder("doc").WithKey("doc-1").Build()
	check := func(e *enforcement.PermitEnforcer) error {
		_, err := e.Check(user, "read", resource)
		return err
	}
	bulkCheck := func(e *enforcement.PermitEnforcer) error {
		_, err := e.BulkCheck(*enforcement.NewCheckRequest(user, "read", resource, nil))
		return err
	}
	filterObjects := func(e *enforcement.PermitEnforcer) error {
		_, err := e.FilterObjects(user, "read", nil, &resource)
		return err
	}
	allTenantsCheck := func(e *enforcement.PermitEnforcer) error {
		_, err := e.AllTenantsCheck(user, "read", resource)
		return err
	}
	getUserPermissions := func(e *enforcement.PermitEnforcer) error {
		_, err := e.GetUserPermissions(user, "t1")
		return err
	}
	getUserPermissionsWithOptions := func(e *enforcement.PermitEnforcer) error {
		_, err := e.GetUserPermissionsWithOptions(user, enforcement.WithTenants([]string{"t1"}))
		return err
	}
	calls := []struct {
		name string
		call func(e *enforcement.PermitEnforcer) error
		// undecodable is a 200 body that the call cannot read.
		undecodable string
	}{
		{name: "Check", call: check, undecodable: undecodableBody("allow")},
		{name: "BulkCheck", call: bulkCheck, undecodable: undecodableBody("allow")},
		{name: "FilterObjects", call: filterObjects, undecodable: undecodableBody("allow")},
		{name: "AllTenantsCheck", call: allTenantsCheck,
			undecodable: undecodableBody("allowed_tenants")},
		{name: "GetUserPermissions", call: getUserPermissions,
			undecodable: undecodableBody("t1")},
		{name: "GetUserPermissionsWithOptions", call: getUserPermissionsWithOptions,
			undecodable: undecodableBody("t1")},
	}
	errorBody := `{"detail":"` + pdpBodyMarker + ` for ` + credentialsTestKey +
		` and permit_key_FAKEechoed12345",` + credentialFields + `}`
	responses := []struct {
		name            string
		status          int
		body            func(undecodable string) string
		bodyLoggedDebug bool
	}{
		{name: "422", status: http.StatusUnprocessableEntity, bodyLoggedDebug: true,
			body: func(string) string { return errorBody }},
		{name: "500", status: http.StatusInternalServerError, bodyLoggedDebug: true,
			body: func(string) string { return errorBody }},
		{name: "undecodable 200", status: http.StatusOK,
			body: func(undecodable string) string { return undecodable }},
	}
	for _, c := range calls {
		for _, r := range responses {
			t.Run(c.name+"/"+r.name, func(t *testing.T) {
				pdp := newFakePDP(t, r.status, r.body(c.undecodable))
				enforcer, logs := newObservedEnforcer(pdp.URL)

				err := c.call(enforcer)

				require.Error(t, err)
				var permitErr permitErrors.PermitError
				if errors.As(err, &permitErr) {
					assertNoCredentials(t, "ResponseBody", permitErr.ResponseBody)
				}
				assertNoCredentials(t, "Error()", err.Error())
				require.NotEmpty(t, logs.All(), "the call logs the failure")
				assertLogsHoldNoCredentials(t, logs, r.bodyLoggedDebug)
				if r.bodyLoggedDebug {
					errorLines := logs.FilterLevelExact(zapcore.ErrorLevel).
						FilterField(zap.Int("status_code", r.status))
					assert.Equal(t, 1, errorLines.Len(),
						"the Error level line gives the status code")
				}
			})
		}
	}
}

func TestBulkCheckUnexpectedResultCountHoldsNoCredentials(t *testing.T) {
	body := `{"allow":[],"detail":"` + pdpBodyMarker + `",` + credentialFields + `}`
	pdp := newFakePDP(t, http.StatusOK, body)
	enforcer, logs := newObservedEnforcer(pdp.URL)
	user := enforcement.UserBuilder("u1").Build()
	resource := enforcement.ResourceBuilder("doc").WithKey("doc-1").Build()

	_, err := enforcer.BulkCheck(*enforcement.NewCheckRequest(user, "read", resource, nil))

	var permitErr permitErrors.PermitError
	require.True(t, errors.As(err, &permitErr), "want a PermitError, got %T: %v", err, err)
	assert.Contains(t, permitErr.Error(), "unexpected number of results")
	assert.Equal(t, `{"allow":[],"auth":{"password":"[REDACTED]"},"detail":"`+pdpBodyMarker+
		`","secret":"[REDACTED]","token":"[REDACTED]"}`, permitErr.ResponseBody)
	assertNoCredentials(t, "Error()", permitErr.Error())
	assertLogsHoldNoCredentials(t, logs, false)
}

func TestPDPErrorBodyKeepsNonCredentialParts(t *testing.T) {
	body := `{"detail":"` + pdpBodyMarker + ` for ` + credentialsTestKey + `",` +
		credentialFields + `}`
	pdp := newFakePDP(t, http.StatusUnprocessableEntity, body)
	enforcer, logs := newObservedEnforcer(pdp.URL)
	user := enforcement.UserBuilder("u1").Build()

	_, err := enforcer.Check(user, "read", enforcement.ResourceBuilder("doc").Build())

	var permitErr permitErrors.PermitError
	require.True(t, errors.As(err, &permitErr), "want a PermitError, got %T: %v", err, err)
	assert.Equal(t, http.StatusUnprocessableEntity, permitErr.StatusCode)
	want := `{"auth":{"password":"[REDACTED]"},"detail":"` + pdpBodyMarker +
		` for [REDACTED]","secret":"[REDACTED]","token":"[REDACTED]"}`
	assert.Equal(t, want, permitErr.ResponseBody)
	debugBodies := logs.FilterLevelExact(zapcore.DebugLevel).
		FilterField(zap.String("response_body", want))
	assert.Equal(t, 1, debugBodies.Len(), "the redacted body is logged once at Debug level")
}

func assertNoCredentials(t *testing.T, name string, text string) {
	t.Helper()
	for _, value := range credentialValues {
		assert.NotContains(t, text, value, name)
	}
}

// assertLogsHoldNoCredentials fails when a log line holds a credential, or when a line above
// Debug level holds a part of the PDP body. When bodyLoggedDebug is true, it also fails unless a
// Debug line holds the body.
func assertLogsHoldNoCredentials(t *testing.T, logs *observer.ObservedLogs, bodyLoggedDebug bool) {
	t.Helper()
	bodyLogged := false
	for _, entry := range logs.All() {
		line := fmt.Sprint(entry.Message, " ", entry.ContextMap())
		assertNoCredentials(t, entry.Level.String()+" log line", line)
		if entry.Level > zapcore.DebugLevel {
			assert.NotContains(t, line, pdpBodyMarker, "%s log line holds the PDP body",
				entry.Level)
		} else if entry.ContextMap()["response_body"] != nil {
			assert.Contains(t, line, pdpBodyMarker)
			bodyLogged = true
		}
	}
	if bodyLoggedDebug {
		assert.True(t, bodyLogged, "the PDP body is not logged at Debug level")
	}
}
