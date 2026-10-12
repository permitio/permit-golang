package api_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsersGetAssignedRolesEmptyResponses(t *testing.T) {
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

			assignments, err := client.Users.GetAssignedRoles(context.Background(), "u1", "t1", 1, 10)

			require.NoError(t, err)
			require.NotNil(t, assignments)
			roles := []string{}
			for _, assignment := range assignments {
				roles = append(roles, assignment.GetRole())
			}
			assert.Equal(t, tt.wantRoles, roles)
		})
	}
}
