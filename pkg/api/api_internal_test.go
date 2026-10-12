package api

import (
	"context"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// No method passes config.ProjectAPIKeyLevel to lazyLoadPermitContext, so this test calls it
// directly to check the error of that level.
func TestProjectLevelContextErrorNamesWhatToSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"organization_id":"org","project_id":null,"environment_id":null}`))
	}))
	t.Cleanup(srv.Close)
	cfg := config.NewConfigBuilder("test-token").
		WithApiUrl(srv.URL).
		WithPdpUrl(srv.URL).
		WithContext(config.NewPermitContext(config.OrganizationAPIKeyLevel, "proj", "")).
		WithLogger(zap.NewNop()).
		Build()
	client := NewPermitApiClient(&cfg)
	ctx := context.Background()

	err := client.Users.lazyLoadPermitContext(ctx, config.ProjectAPIKeyLevel)

	var permitErr errors.PermitError
	require.True(t, stderrors.As(err, &permitErr), "want a PermitError, got %T: %v", err, err)
	assert.Equal(t, errors.ContextError, permitErr.ErrorCode)
	wantErr := errors.NewPermitContextError("This method needs a project and an environment in " +
		"the client's context. Call client.Api.SetContext(ctx, project, environment) first, " +
		"with both.")
	assert.Equal(t, wantErr.Error(), err.Error())

	client.SetContext(ctx, "proj", "env")

	assert.NoError(t, client.Users.lazyLoadPermitContext(ctx, config.ProjectAPIKeyLevel))
}
