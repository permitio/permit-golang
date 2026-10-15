package rest

import "net/http"

// defaultOps are the operations of the SDK, the operations of Default. The commits that add the
// SDK's methods add their operations here. TestRepositoryRegistry, in the tools module, checks
// each against tools/specs: its ID is a planned or covered row of coverage.yaml, its method and
// route are the spec's, and its models are types of package models. TestRepositoryWaits, in
// tools/drift, checks that an operation is WaitsForSync exactly when its route, under /facts, is
// one that the spec of the PDP container lists, as those are the routes that the container
// waits on.
var defaultOps = []Op{
	// Any API key may read its own scope, an environment-level key too.
	{
		ID: "get_api_key_scope", Method: http.MethodGet, Route: "/v2/api-key/scope",
		Idempotent: true, KeyLevel: EnvironmentLevel, ResponseModel: "APIKeyScopeRead",
	},

	// The projects of the organization. Listing and creating them needs an organization-level
	// key; reading, changing and deleting one needs a key of the project, or of its
	// organization. The list is a JSON array of at most 100 projects. A PATCH sets the fields
	// that it names to the values that it gives, so it does the same when sent twice; a create
	// does not.
	{
		ID: "list_projects", Method: http.MethodGet, Route: "/v2/projects", Idempotent: true,
		KeyLevel: OrganizationLevel, MaxPerPage: 100, ResponseModel: "ProjectRead",
	},
	{
		ID: "create_project", Method: http.MethodPost, Route: "/v2/projects",
		KeyLevel: OrganizationLevel, RequestModel: "ProjectCreate", ResponseModel: "ProjectRead",
	},
	{
		ID: "get_project", Method: http.MethodGet, Route: "/v2/projects/{proj_id}",
		Idempotent: true, KeyLevel: ProjectLevel, ResponseModel: "ProjectRead",
	},
	{
		ID: "update_project", Method: http.MethodPatch, Route: "/v2/projects/{proj_id}",
		Idempotent: true, KeyLevel: ProjectLevel, RequestModel: "ProjectUpdate",
		ResponseModel: "ProjectRead",
	},
	{
		ID: "delete_project", Method: http.MethodDelete, Route: "/v2/projects/{proj_id}",
		Idempotent: true, KeyLevel: ProjectLevel, DeleteSelf: true,
	},

	// The environments of a project, each of which is sent with a key of the project, or of its
	// organization. These key levels are not measured yet: the phase api.environments of the
	// local harness confirms them.
	// The list is a JSON array of at most 100 environments, and a list and a get return the
	// environment with its email configuration. A PATCH sets the fields that it names to the
	// values that it gives, so it does the same when sent twice; a create does not.
	{
		ID: "list_environments", Method: http.MethodGet, Route: "/v2/projects/{proj_id}/envs",
		Idempotent: true, KeyLevel: ProjectLevel, MaxPerPage: 100,
		ResponseModel: "EnvironmentReadWithEmailConfig",
	},
	{
		ID: "create_environment", Method: http.MethodPost, Route: "/v2/projects/{proj_id}/envs",
		KeyLevel: ProjectLevel, RequestModel: "EnvironmentCreate", ResponseModel: "EnvironmentRead",
	},
	{
		ID: "get_environment", Method: http.MethodGet,
		Route: "/v2/projects/{proj_id}/envs/{env_id}", Idempotent: true, KeyLevel: ProjectLevel,
		ResponseModel: "EnvironmentReadWithEmailConfig",
	},
	{
		ID: "update_environment", Method: http.MethodPatch,
		Route: "/v2/projects/{proj_id}/envs/{env_id}", Idempotent: true, KeyLevel: ProjectLevel,
		RequestModel: "EnvironmentUpdate", ResponseModel: "EnvironmentRead",
	},
	{
		ID: "delete_environment", Method: http.MethodDelete,
		Route: "/v2/projects/{proj_id}/envs/{env_id}", Idempotent: true, KeyLevel: ProjectLevel,
		DeleteSelf: true,
	},

	// The decisions of the PDP, which the PDP container and the cloud PDP both serve. A decision
	// changes nothing, so a retry may send it again. The bulk route takes a JSON array of the
	// checks. The SDK reads the allow of a reply itself, as a strict boolean, so the replies name
	// no model; the drift tests check the reply fixtures of the wire tests against the specs. A
	// reply can echo the check, whose values are the application's, not credentials.
	{
		ID: "is_allowed_allowed_post", Method: http.MethodPost, Route: "/allowed",
		Idempotent: true, KeyLevel: EnvironmentLevel, NoReplyCredentials: true,
		RequestModel: "CheckRequest",
	},
	{
		ID: "is_allowed_bulk_allowed_bulk_post", Method: http.MethodPost, Route: "/allowed/bulk",
		Idempotent: true, KeyLevel: EnvironmentLevel, NoReplyCredentials: true,
		RequestModel: "[]CheckRequest",
	},

	// The lists of the PDP of who may do what, which the PDP container and the cloud PDP both
	// serve: the permissions of a user, and the users that may do an action on a resource. Like
	// a decision, a list changes nothing, and its reply holds the application's values, not
	// credentials. The SDK reads each reply itself, strictly where it holds the answer, so the
	// replies name no model; the drift tests check the reply types against the schemas of their
	// rows, and the reply fixtures of the wire tests against the specs.
	{
		ID: "Get_User_Permissions_user_permissions_post", Method: http.MethodPost,
		Route: "/user-permissions", Idempotent: true, KeyLevel: EnvironmentLevel,
		NoReplyCredentials: true, RequestModel: "UserPermissionsRequest",
	},
	{
		ID: "authorized_users_authorized_users_post", Method: http.MethodPost,
		Route: "/authorized_users", Idempotent: true, KeyLevel: EnvironmentLevel,
		NoReplyCredentials: true, RequestModel: "AuthorizedUsersRequest",
	},

	// The decisions of the PDP that only the PDP container serves: the tenants in which a check
	// is allowed, the tenants of a user, and the check of a URL. The cloud PDP answers each with
	// 404 and an empty body, and its spec does not list them, so the drift tests check them
	// against the container's spec only. Like a decision, each changes nothing, and its reply
	// holds the application's values, not credentials; the SDK reads each reply itself, so the
	// replies name no model.
	{
		ID: "is_allowed_all_tenants_allowed_all_tenants_post", Method: http.MethodPost,
		Route: "/allowed/all-tenants", Idempotent: true, ContainerOnly: true,
		KeyLevel: EnvironmentLevel, NoReplyCredentials: true, RequestModel: "CheckRequest",
	},
	{
		ID: "Get_User_Tenants_user_tenants_post", Method: http.MethodPost, Route: "/user-tenants",
		Idempotent: true, ContainerOnly: true, KeyLevel: EnvironmentLevel,
		NoReplyCredentials: true, RequestModel: "UserTenantsRequest",
	},
	{
		ID: "is_allowed_url_allowed_url_post", Method: http.MethodPost, Route: "/allowed_url",
		Idempotent: true, ContainerOnly: true, KeyLevel: EnvironmentLevel,
		NoReplyCredentials: true, RequestModel: "CheckURLRequest",
	},

	// The role assignments that the PDP container holds, a list that only the container serves:
	// the cloud PDP answers it with 404 and an empty body, and its spec does not list the route.
	// The list changes nothing, and its reply holds the application's keys, not credentials. Its
	// reply is a JSON array of at most 100 role assignments. They decode into a type of the root
	// package, LocalRoleAssignment, so the reply names no model of package models; the drift
	// tests check the type against the schema of its row, and the reply fixture of the wire tests
	// against the container's spec.
	{
		ID: "list_role_assignments_local_role_assignments_get", Method: http.MethodGet,
		Route: "/local/role_assignments", Idempotent: true, ContainerOnly: true,
		KeyLevel: EnvironmentLevel, NoReplyCredentials: true, MaxPerPage: 100,
	},
}
