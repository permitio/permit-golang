package api_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const conditionSetJSON = `{"key":"cs1","type":"userset","id":"cs-id",` +
	`"organization_id":"org","project_id":"proj","environment_id":"env",` +
	`"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","name":"CS 1"}`

func TestConditionSetsListReadsArrayAndEnvelope(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantKeys []string
	}{
		{name: "bare array", body: "[" + conditionSetJSON + "]", wantKeys: []string{"cs1"}},
		{
			name:     "paginated envelope",
			body:     `{"data":[` + conditionSetJSON + `],"total_count":1,"page_count":1}`,
			wantKeys: []string{"cs1"},
		},
		{name: "empty array", body: "[]", wantKeys: []string{}},
		{name: "empty object", body: "{}", wantKeys: []string{}},
		{name: "null data", body: `{"data":null}`, wantKeys: []string{}},
		{name: "null", body: "null", wantKeys: []string{}},
		{name: "empty body", body: "", wantKeys: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, srv := newListClient(t, tt.body)

			conditionSets, err := client.ConditionSets.List(context.Background(), 2, 7)

			require.NoError(t, err)
			assert.NotNil(t, conditionSets)
			keys := []string{}
			for _, conditionSet := range conditionSets {
				keys = append(keys, conditionSet.GetKey())
			}
			assert.Equal(t, tt.wantKeys, keys)

			requests := srv.recorded()
			require.Len(t, requests, 1)
			assert.Equal(t, "/v2/schema/proj/env/condition_sets", requests[0].path)
			assert.Equal(t, "2", requests[0].query.Get("page"))
			assert.Equal(t, "7", requests[0].query.Get("per_page"))
			assert.Equal(t, "Bearer "+testToken, requests[0].header.Get("Authorization"))
		})
	}
}
