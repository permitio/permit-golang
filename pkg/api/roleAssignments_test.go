package api_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	roleAssignmentJSON = `{"id":"ra-1","user":"u1","role":"admin","tenant":"t1",` +
		`"user_id":"uid","role_id":"rid","tenant_id":"tid",` +
		`"organization_id":"org","project_id":"proj","environment_id":"env",` +
		`"created_at":"2026-01-01T00:00:00Z"}`
	detailedRoleAssignmentJSON = `{"id":"ra-1","role":{"id":"rid","key":"admin"},` +
		`"user":{"id":"uid","key":"u1"},"tenant":{"id":"tid","key":"t1","name":"T1"},` +
		`"organization_id":"org","project_id":"proj","environment_id":"env",` +
		`"created_at":"2026-01-01T00:00:00Z"}`
)

func TestRoleAssignmentsListEmptyResponses(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantRoles []string
	}{
		{name: "one assignment", body: "[" + roleAssignmentJSON + "]", wantRoles: []string{"admin"}},
		{name: "empty array", body: "[]", wantRoles: []string{}},
		{name: "empty body", body: "", wantRoles: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newListClient(t, tt.body)

			assignments, err := client.RoleAssignments.List(context.Background(), 1, 10, "", "", "")

			require.NoError(t, err)
			require.NotNil(t, assignments)
			roles := []string{}
			for _, assignment := range *assignments {
				roles = append(roles, assignment.GetRole())
			}
			assert.Equal(t, tt.wantRoles, roles)
		})
	}
}

func TestRoleAssignmentsListDetailedEmptyResponses(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantRoles []string
	}{
		{
			name:      "one assignment",
			body:      "[" + detailedRoleAssignmentJSON + "]",
			wantRoles: []string{"admin"},
		},
		{name: "empty array", body: "[]", wantRoles: []string{}},
		{name: "empty body", body: "", wantRoles: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newListClient(t, tt.body)

			assignments, err := client.RoleAssignments.ListDetailed(
				context.Background(), 1, 10, "", "", "",
			)

			require.NoError(t, err)
			require.NotNil(t, assignments)
			roles := []string{}
			for _, assignment := range *assignments {
				roles = append(roles, assignment.Role.GetKey())
			}
			assert.Equal(t, tt.wantRoles, roles)
		})
	}
}
