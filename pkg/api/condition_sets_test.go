package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/permitio/permit-golang/pkg/config"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestConditionSetsListHandlesArrayResponse(t *testing.T) {
	client := newConditionSetsTestClient(t, `[{
		"key": "team",
		"id": "cs-1",
		"organization_id": "org",
		"project_id": "proj",
		"environment_id": "env",
		"created_at": "2026-01-01T00:00:00Z",
		"updated_at": "2026-01-01T00:00:00Z",
		"name": "Team"
	}]`)

	conditionSets, err := client.Api.ConditionSets.List(context.Background(), 1, 100)

	require.NoError(t, err)
	require.Len(t, conditionSets, 1)
	require.Equal(t, "team", conditionSets[0].Key)
}

func TestConditionSetsListHandlesPaginatedResponse(t *testing.T) {
	client := newConditionSetsTestClient(t, `{
		"data": [{
			"key": "team",
			"id": "cs-1",
			"organization_id": "org",
			"project_id": "proj",
			"environment_id": "env",
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-01T00:00:00Z",
			"name": "Team"
		}],
		"total_count": 1,
		"page_count": 1
	}`)

	conditionSets, err := client.Api.ConditionSets.List(context.Background(), 1, 100)

	require.NoError(t, err)
	require.Len(t, conditionSets, 1)
	require.Equal(t, "team", conditionSets[0].Key)
}

func newConditionSetsTestClient(t *testing.T, responseBody string) *permit.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v2/schema/proj/env/condition_sets", r.URL.Path)
		require.Equal(t, "1", r.URL.Query().Get("page"))
		require.Equal(t, "100", r.URL.Query().Get("per_page"))
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(responseBody))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	permitConfig := config.NewConfigBuilder("test-token").
		WithApiUrl(server.URL).
		WithContext(config.NewPermitContext(config.EnvironmentAPIKeyLevel, "proj", "env")).
		WithLogger(zap.NewNop()).
		Build()

	return permit.New(permitConfig)
}
