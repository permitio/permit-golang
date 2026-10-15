package permit_test

import (
	"net/http"
	"os"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// The IDs of the scope of the API key of the Examples and of the wire case of Client.Scope, as
// the reply fixture testdata/replies/get_api_key_scope.200.json holds them.
const (
	exampleProjectID     = "00000000-0000-4000-8000-000000000002"
	exampleEnvironmentID = "00000000-0000-4000-8000-000000000003"
)

// The URLs and the API key that the Examples pass to the client, which TestMain sets to those of
// the mockpermit servers that it starts for them.
var (
	exampleAPIURL string
	examplePDPURL string
	exampleAPIKey string
)

// TestMain starts the mockpermit servers of the Examples and runs the tests. When the
// environment variable PERMIT_API_COVERAGE_RECORD names a file, it also writes the record of the
// requests that the tests send to the mockpermit servers there, with the session trailer, for
// tools/cmd/apicoverage; CONTRIBUTING.md ("API coverage") says how to run it. The servers of the
// Examples do not write to the record.
func TestMain(m *testing.M) {
	servers := mockpermit.StartExample()
	exampleAPIURL, examplePDPURL = servers.API.URL, servers.PDP.URL
	exampleAPIKey = mockpermit.APIKey
	servers.API.Handle("GET /v2/api-key/scope",
		mockpermit.Reply{Body: replyFixture("get_api_key_scope.200.json")})
	servers.PDP.Handle("POST /allowed",
		mockpermit.Reply{Body: replyFixture("is_allowed_allowed_post.200.json")})
	servers.PDP.Handle("POST /allowed/bulk",
		mockpermit.Reply{Body: replyFixture("is_allowed_bulk_allowed_bulk_post.200.json")})
	servers.PDP.Handle("POST /user-permissions", mockpermit.Reply{
		Body: replyFixture("Get_User_Permissions_user_permissions_post.200.json"),
	})
	servers.PDP.Handle("POST /authorized_users",
		mockpermit.Reply{Body: replyFixture("authorized_users_authorized_users_post.200.json")})
	servers.PDP.Handle("POST /allowed/all-tenants", mockpermit.Reply{
		Body: replyFixture("is_allowed_all_tenants_allowed_all_tenants_post.200.json"),
	})
	servers.PDP.Handle("POST /user-tenants",
		mockpermit.Reply{Body: replyFixture("Get_User_Tenants_user_tenants_post.200.json")})
	servers.PDP.Handle("POST /allowed_url",
		mockpermit.Reply{Body: replyFixture("is_allowed_url_allowed_url_post.200.json")})
	servers.PDP.Handle("GET /local/role_assignments", mockpermit.Reply{
		Body: replyFixture("list_role_assignments_local_role_assignments_get.200.json"),
	}, mockpermit.Reply{Body: `[]`})
	servers.API.Handle("GET /v2/projects", mockpermit.Reply{
		Body: replyFixture("list_projects.200.json"),
	}, mockpermit.Reply{Body: `[]`})
	servers.API.Handle("GET /v2/projects/{proj_id}",
		mockpermit.Reply{Body: replyFixture("get_project.200.json")})
	servers.API.Handle("POST /v2/projects",
		mockpermit.Reply{Body: replyFixture("create_project.200.json")})
	servers.API.Handle("GET /v2/projects/{proj_id}/envs", mockpermit.Reply{
		Body: replyFixture("list_environments.200.json"),
	}, mockpermit.Reply{Body: `[]`})
	servers.API.Handle("GET /v2/projects/{proj_id}/envs/{env_id}",
		mockpermit.Reply{Body: replyFixture("get_environment.200.json")})
	servers.API.Handle("PATCH /v2/projects/{proj_id}/envs/{env_id}",
		mockpermit.Reply{Body: replyFixture("update_environment.200.json")})
	servers.API.Handle("GET /v2/projects/{proj_id}/envs/{env_id}/stats",
		mockpermit.Reply{Body: replyFixture("stats_environments.200.json")})
	servers.API.Handle("POST /v2/projects/{proj_id}/envs/{env_id}/copy/async", mockpermit.Reply{
		Status: http.StatusAccepted, Body: replyFixture("copy_environment_async.202.json"),
	})
	servers.API.Handle("GET /v2/projects/{proj_id}/envs/{env_id}/copy/async/{task_id}/result",
		mockpermit.Reply{Body: replyFixture("get_copy_environment_async_result.200.json")})
	servers.API.Handle("GET /v2/api-key/{proj_id}/{env_id}",
		mockpermit.Reply{Body: replyFixture("get_environment_api_key.200.json")})
	servers.API.Handle("POST /v2/api-key/{api_key_id}/rotate-secret",
		mockpermit.Reply{Body: replyFixture("rotate_api_key.200.json")})
	servers.API.Handle("GET /v2/schema/{proj_id}/{env_id}/resources", mockpermit.Reply{
		Body: replyFixture("list_resources.200.json"),
	}, mockpermit.Reply{Body: `[]`})
	servers.API.Handle("GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}",
		mockpermit.Reply{Body: replyFixture("get_resource.200.json")})
	servers.API.Handle("PUT /v2/schema/{proj_id}/{env_id}/resources/{resource_id}",
		mockpermit.Reply{Body: replyFixture("replace_resource.200.json")})
	servers.API.Handle("GET /v2/facts/{proj_id}/{env_id}/tenants",
		mockpermit.Reply{Body: replyFixture("list_tenants.200.page.json")})
	servers.API.Handle("GET /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}",
		mockpermit.Reply{Body: replyFixture("get_tenant.200.json")})
	servers.PDP.Handle("POST /facts/tenants",
		mockpermit.Reply{Body: replyFixture("create_tenant.200.json")})
	servers.API.Handle("GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/actions",
		mockpermit.Reply{Body: replyFixture("list_resource_actions.200.json")},
		mockpermit.Reply{Body: `[]`})
	servers.API.Handle("POST /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/actions",
		mockpermit.Reply{Body: replyFixture("create_resource_action.200.json")})
	servers.API.Handle("GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/action_groups",
		mockpermit.Reply{Body: replyFixture("list_resource_action_groups.200.json")},
		mockpermit.Reply{Body: `[]`})
	servers.API.Handle("PATCH /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/"+
		"action_groups/{action_group_id}",
		mockpermit.Reply{Body: replyFixture("update_resource_action_group.200.json")})
	servers.API.Handle("GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/attributes",
		mockpermit.Reply{Body: replyFixture("list_resource_attributes.200.json")},
		mockpermit.Reply{Body: replyFixture("list_resource_attributes.200.unknown-type.json")},
		mockpermit.Reply{Body: `[]`})
	servers.API.Handle("POST /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/attributes",
		mockpermit.Reply{Body: replyFixture("create_resource_attribute.200.json")})
	servers.API.Handle("GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/relations",
		mockpermit.Reply{Body: replyFixture("list_resource_relations.200.json")},
		mockpermit.Reply{Body: replyFixture("list_resource_relations.200.json")})
	servers.API.Handle("POST /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/relations",
		mockpermit.Reply{Body: replyFixture("create_resource_relation.200.json")})
	servers.API.Handle("POST /v2/schema/{proj_id}/{env_id}/roles",
		mockpermit.Reply{Body: replyFixture("create_role.200.json")})
	servers.API.Handle("GET /v2/schema/{proj_id}/{env_id}/roles",
		mockpermit.Reply{Body: replyFixture("list_roles.200.json")}, mockpermit.Reply{Body: `[]`})
	servers.API.Handle("PATCH /v2/schema/{proj_id}/{env_id}/roles/{role_id}",
		mockpermit.Reply{Body: replyFixture("update_role.200.json")})
	servers.API.Handle("POST /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles",
		mockpermit.Reply{Body: replyFixture("create_resource_role.200.json")})
	servers.API.Handle("GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles",
		mockpermit.Reply{Body: replyFixture("list_resource_roles.200.json")},
		mockpermit.Reply{Body: `[]`})
	servers.API.Handle("PATCH /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/"+
		"{role_id}", mockpermit.Reply{Body: replyFixture("update_resource_role.200.json")})
	servers.API.Handle("GET /v2/schema/{proj_id}/{env_id}/users/attributes",
		mockpermit.Reply{Body: replyFixture("list_user_attributes.200.json")},
		mockpermit.Reply{Body: replyFixture("list_user_attributes.200.unknown-type.json")},
		mockpermit.Reply{Body: `[]`})
	servers.API.Handle("POST /v2/schema/{proj_id}/{env_id}/users/attributes",
		mockpermit.Reply{Body: replyFixture("create_user_attribute.200.json")})
	os.Exit(mockpermit.RunRecorded(func() int { return servers.Run(m) }))
}
