package api_test

import (
	"context"
	"testing"

	"github.com/permitio/permit-golang/pkg/api"
	"github.com/permitio/permit-golang/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const resourceRoleJSON = `{"key":"viewer","name":"Viewer","id":"rr-id",` +
	`"resource_id":"r-id","resource":"doc",` +
	`"organization_id":"org","project_id":"proj","environment_id":"env",` +
	`"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`

func TestResourceRolesAncestorsAndDescendantsEmptyResponses(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantKeys []string
	}{
		{name: "bare array", body: `{"roles":[` + resourceRoleJSON + `]}`, wantKeys: []string{"viewer"}},
		{
			name:     "paginated envelope",
			body:     `{"roles":{"data":[` + resourceRoleJSON + `],"total_count":1,"page_count":1}}`,
			wantKeys: []string{"viewer"},
		},
		{name: "no roles", body: `{}`, wantKeys: []string{}},
		{name: "empty body", body: "", wantKeys: []string{}},
	}
	methods := []struct {
		name string
		call func(*api.ResourceRoles) (*[]models.ResourceRoleRead, error)
	}{
		{
			name: "GetAncestors",
			call: func(r *api.ResourceRoles) (*[]models.ResourceRoleRead, error) {
				return r.GetAncestors(context.Background(), "doc", "viewer")
			},
		},
		{
			name: "GetDescendants",
			call: func(r *api.ResourceRoles) (*[]models.ResourceRoleRead, error) {
				return r.GetDescendants(context.Background(), "doc", "viewer")
			},
		},
	}
	for _, method := range methods {
		for _, tt := range tests {
			t.Run(method.name+"/"+tt.name, func(t *testing.T) {
				client, _ := newListClient(t, tt.body)

				roles, err := method.call(client.ResourceRoles)

				require.NoError(t, err)
				require.NotNil(t, roles)
				keys := []string{}
				for _, role := range *roles {
					keys = append(keys, role.GetKey())
				}
				assert.Equal(t, tt.wantKeys, keys)
			})
		}
	}
}
