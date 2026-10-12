package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/permitio/permit-golang/pkg/api"
	"github.com/permitio/permit-golang/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestResourceActionGroupsDeleteCallsActionGroupRoute(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{name: "deleted", status: http.StatusNoContent},
		{name: "missing group", status: http.StatusNotFound, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var requests []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests = append(requests, r.Method+" "+r.URL.EscapedPath())
				mu.Unlock()
				w.WriteHeader(tt.status)
			}))
			t.Cleanup(srv.Close)
			cfg := config.NewConfigBuilder("test-token").
				WithApiUrl(srv.URL).
				WithContext(config.NewPermitContext(config.EnvironmentAPIKeyLevel, "proj", "env")).
				WithLogger(zap.NewNop()).
				Build()
			client := api.NewPermitApiClient(&cfg)

			err := client.ResourceActionGroups.Delete(context.Background(), "doc", "editors")

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			mu.Lock()
			defer mu.Unlock()
			require.Equal(t,
				[]string{"DELETE /v2/schema/proj/env/resources/doc/action_groups/editors"},
				requests)
		})
	}
}
