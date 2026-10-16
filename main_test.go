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
	servers.API.Handle("GET /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}/users",
		mockpermit.Reply{Body: replyFixture("list_tenant_users.200.json")})
	servers.API.Handle("POST /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}/users",
		mockpermit.Reply{Body: replyFixture("add_user_to_tenant.200.json")})
	servers.API.Handle("DELETE /v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}/users/{user_id}",
		mockpermit.Reply{Status: http.StatusNoContent})
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
	servers.API.Handle("GET /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/"+
		"{role_id}/ancestors",
		mockpermit.Reply{Body: replyFixture("get_resource_role_ancestors.200.json")})
	servers.API.Handle("PATCH /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/"+
		"{role_id}", mockpermit.Reply{Body: replyFixture("update_resource_role.200.json")})
	servers.API.Handle("POST /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/"+
		"{role_id}/implicit_grants",
		mockpermit.Reply{Body: replyFixture("create_implicit_grant.200.json")})
	servers.API.Handle("PUT /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/"+
		"{role_id}/implicit_grants/conditions",
		mockpermit.Reply{Body: replyFixture("update_implicit_grants_conditions.200.json")})
	servers.API.Handle("DELETE /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/"+
		"{role_id}/implicit_grants", mockpermit.Reply{Status: http.StatusNoContent})
	servers.API.Handle("GET /v2/schema/{proj_id}/{env_id}/users/attributes",
		mockpermit.Reply{Body: replyFixture("list_user_attributes.200.json")},
		mockpermit.Reply{Body: replyFixture("list_user_attributes.200.unknown-type.json")},
		mockpermit.Reply{Body: `[]`})
	servers.API.Handle("POST /v2/schema/{proj_id}/{env_id}/users/attributes",
		mockpermit.Reply{Body: replyFixture("create_user_attribute.200.json")})
	servers.API.Handle("POST /v2/schema/{proj_id}/{env_id}/condition_sets",
		mockpermit.Reply{Body: replyFixture("create_condition_set.200.json")})
	servers.API.Handle("GET /v2/schema/{proj_id}/{env_id}/condition_sets",
		mockpermit.Reply{Body: replyFixture("list_condition_sets.200.json")},
		mockpermit.Reply{Body: `[]`})
	servers.API.Handle("GET /v2/schema/{proj_id}/{env_id}/condition_sets/{condition_set_id}/"+
		"ancestors", mockpermit.Reply{Body: replyFixture("get_condition_set_ancestors.200.json")},
		mockpermit.Reply{Body: `[]`})
	servers.API.Handle("POST /v2/facts/{proj_id}/{env_id}/set_rules",
		mockpermit.Reply{Body: replyFixture("assign_set_permissions.200.json")})
	servers.API.Handle("GET /v2/facts/{proj_id}/{env_id}/set_rules",
		mockpermit.Reply{Body: replyFixture("list_set_permissions.200.json")},
		mockpermit.Reply{Body: `[]`})
	servers.API.Handle("DELETE /v2/facts/{proj_id}/{env_id}/set_rules",
		mockpermit.Reply{Status: http.StatusNoContent})
	servers.API.Handle("GET /v2/facts/{proj_id}/{env_id}/users",
		mockpermit.Reply{Body: replyFixture("list_users.200.json")})
	servers.API.Handle("GET /v2/facts/{proj_id}/{env_id}/users/{user_id}",
		mockpermit.Reply{Body: replyFixture("get_user.200.json")})
	servers.API.Handle("PUT /v2/facts/{proj_id}/{env_id}/users/{user_id}", mockpermit.Reply{
		Status: http.StatusCreated, Body: replyFixture("replace_user.201.json"),
	}, mockpermit.Reply{Body: replyFixture("replace_user.200.json")})
	servers.API.Handle("POST /v2/facts/{proj_id}/{env_id}/resource_instances",
		mockpermit.Reply{Body: replyFixture("create_resource_instance.200.json")})
	servers.API.Handle("GET /v2/facts/{proj_id}/{env_id}/resource_instances/detailed",
		mockpermit.Reply{Body: replyFixture("list_resource_instances_detailed.200.json")})
	servers.API.Handle("PATCH /v2/facts/{proj_id}/{env_id}/resource_instances/{instance_id}",
		mockpermit.Reply{Body: replyFixture("update_resource_instance.200.json")})
	servers.API.Handle("DELETE /v2/facts/{proj_id}/{env_id}/resource_instances/{instance_id}",
		mockpermit.Reply{Status: http.StatusNoContent})
	servers.API.Handle("POST /v2/facts/{proj_id}/{env_id}/proxy_configs",
		mockpermit.Reply{Body: replyFixture("create_proxy_config.200.json")})
	servers.API.Handle("GET /v2/facts/{proj_id}/{env_id}/proxy_configs",
		mockpermit.Reply{Body: replyFixture("list_proxy_configs.200.json")},
		mockpermit.Reply{Body: `[]`})
	servers.API.Handle("POST /v2/facts/{proj_id}/{env_id}/users/{user_id}/roles",
		mockpermit.Reply{Body: replyFixture("assign_role_to_user.200.json")})
	servers.API.Handle("DELETE /v2/facts/{proj_id}/{env_id}/users/{user_id}/roles",
		mockpermit.Reply{Status: http.StatusNoContent})
	servers.API.Handle("GET /v2/facts/{proj_id}/{env_id}/role_assignments",
		mockpermit.Reply{Body: replyFixture("list_role_assignments.200.json")},
		mockpermit.Reply{Body: `[]`})
	servers.API.Handle("GET /v2/facts/{proj_id}/{env_id}/role_assignments/detailed",
		mockpermit.Reply{Body: replyFixture("list_role_assignments_detailed.200.json")})
	servers.PDP.Handle("POST /facts/role_assignments",
		mockpermit.Reply{Body: replyFixture("assign_role.200.json")})
	servers.API.Handle("POST /v2/facts/{proj_id}/{env_id}/role_assignments/bulk",
		mockpermit.Reply{Body: replyFixture("bulk_assign_role.200.json")})
	servers.API.Handle("DELETE /v2/facts/{proj_id}/{env_id}/role_assignments/bulk",
		mockpermit.Reply{Body: replyFixture("bulk_unassign_role.200.json")})
	servers.API.Handle("POST /v2/facts/{proj_id}/{env_id}/relationship_tuples",
		mockpermit.Reply{Body: replyFixture("create_relationship_tuple.200.json")})
	servers.API.Handle("GET /v2/facts/{proj_id}/{env_id}/relationship_tuples",
		mockpermit.Reply{Body: replyFixture("list_relationship_tuples.200.json")},
		mockpermit.Reply{Body: `[]`})
	servers.API.Handle("GET /v2/facts/{proj_id}/{env_id}/relationship_tuples/detailed",
		mockpermit.Reply{Body: replyFixture("list_relationship_tuples_detailed.200.json")})
	servers.API.Handle("DELETE /v2/facts/{proj_id}/{env_id}/relationship_tuples",
		mockpermit.Reply{Status: http.StatusNoContent})
	servers.PDP.Handle("POST /facts/relationship_tuples",
		mockpermit.Reply{Body: replyFixture("create_relationship_tuple.200.json")})
	const bulk = "/v2/facts/{proj_id}/{env_id}/bulk/"
	for _, b := range []struct{ route, op string }{
		{"POST " + bulk + "users", "bulk_create_users"},
		{"PUT " + bulk + "users", "bulk_replace_users"},
		{"DELETE " + bulk + "users", "bulk_delete_users"},
		{"POST " + bulk + "tenants", "bulk_create_tenants"},
		{"DELETE " + bulk + "tenants", "bulk_delete_tenants"},
		{"PUT " + bulk + "resource_instances", "bulk_replace_resource_instances"},
		{"DELETE " + bulk + "resource_instances", "bulk_delete_resource_instances"},
	} {
		servers.API.Handle(b.route, mockpermit.Reply{Body: replyFixture(b.op + ".200.json")})
	}
	os.Exit(mockpermit.RunRecorded(func() int { return servers.Run(m) }))
}
