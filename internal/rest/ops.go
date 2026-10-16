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

	// The API keys of the organization. Reading the key of an environment needs a project-level
	// key; listing, reading, creating, deleting and rotating keys need an organization-level
	// key. The list is a paginated object of at most 100 keys. A rotation is never sent twice:
	// after a 500, 502, 503 or 504 reply or a failed connection, the API may have rotated the
	// secret, and a second rotation would replace the secret that the first returned; after a
	// 429, which the API did not act on, the call also returns the error, and leaves the retry to
	// the caller. The secret of a reply joins the client's redaction set under its credential
	// name, secret.
	{
		ID: "get_environment_api_key", Method: http.MethodGet,
		Route: "/v2/api-key/{proj_id}/{env_id}", Idempotent: true, KeyLevel: ProjectLevel,
		ResponseModel: "APIKeyRead",
	},
	{
		ID: "list_api_keys", Method: http.MethodGet, Route: "/v2/api-key", Idempotent: true,
		KeyLevel: OrganizationLevel, MaxPerPage: 100, ResponseModel: "APIKeyRead",
	},
	{
		ID: "create_api_key", Method: http.MethodPost, Route: "/v2/api-key",
		KeyLevel: OrganizationLevel, RequestModel: "APIKeyCreate", ResponseModel: "APIKeyRead",
	},
	{
		ID: "get_api_key", Method: http.MethodGet, Route: "/v2/api-key/{api_key_id}",
		Idempotent: true, KeyLevel: OrganizationLevel, ResponseModel: "APIKeyRead",
	},
	{
		ID: "delete_api_key", Method: http.MethodDelete, Route: "/v2/api-key/{api_key_id}",
		Idempotent: true, KeyLevel: OrganizationLevel, DeleteSelf: true,
	},
	{
		ID: "rotate_api_key", Method: http.MethodPost,
		Route: "/v2/api-key/{api_key_id}/rotate-secret", NoRetry: true,
		KeyLevel: OrganizationLevel, ResponseModel: "APIKeyRead",
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

	// The statistics of an environment, and the copies of an environment into another. A copy
	// needs a key of the project, or of its organization: an environment-level key may not copy
	// an environment. The key levels of the statistics, of the copy that runs in the background
	// and of the result of its task are not measured yet, like those above. The statistics hold
	// the credentials of the PDPs of the environment: client_secret, which the client finds by
	// its name, and opal_server_access_token, which is listed here, as its name is not one of
	// those. A copy creates or changes an environment, so a retry sends it again only after a
	// 429; reading the result of its task changes nothing.
	{
		ID: "stats_environments", Method: http.MethodGet,
		Route: "/v2/projects/{proj_id}/envs/{env_id}/stats", Idempotent: true,
		KeyLevel: ProjectLevel, SensitiveBodyFields: []string{"opal_server_access_token"},
		ResponseModel: "EnvironmentStats",
	},
	{
		ID: "copy_environment", Method: http.MethodPost,
		Route: "/v2/projects/{proj_id}/envs/{env_id}/copy", KeyLevel: ProjectLevel,
		RequestModel: "EnvironmentCopy", ResponseModel: "EnvironmentRead",
	},
	{
		ID: "copy_environment_async", Method: http.MethodPost,
		Route: "/v2/projects/{proj_id}/envs/{env_id}/copy/async", KeyLevel: ProjectLevel,
		RequestModel: "EnvironmentCopy", ResponseModel: "TaskResultEnvironmentRead",
	},
	{
		ID: "get_copy_environment_async_result", Method: http.MethodGet,
		Route:      "/v2/projects/{proj_id}/envs/{env_id}/copy/async/{task_id}/result",
		Idempotent: true, KeyLevel: ProjectLevel, ResponseModel: "TaskResultEnvironmentRead",
	},

	// The resources of an environment, each at the environment level, as v1 sends them too.
	// These key levels are not measured yet: the phase api.resources of the local harness
	// confirms them. The list is a JSON array or a paginated object of at most 100 resources. A
	// PUT sends the whole resource, and a PATCH sets the fields that it names to the values that
	// it gives, so each does the same when sent twice; a create does not.
	{
		ID: "list_resources", Method: http.MethodGet,
		Route: "/v2/schema/{proj_id}/{env_id}/resources", Idempotent: true,
		KeyLevel: EnvironmentLevel, MaxPerPage: 100, ResponseModel: "ResourceRead",
	},
	{
		ID: "create_resource", Method: http.MethodPost,
		Route: "/v2/schema/{proj_id}/{env_id}/resources", KeyLevel: EnvironmentLevel,
		RequestModel: "ResourceCreate", ResponseModel: "ResourceRead",
	},
	{
		ID: "get_resource", Method: http.MethodGet,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}", Idempotent: true,
		KeyLevel: EnvironmentLevel, ResponseModel: "ResourceRead",
	},
	{
		ID: "replace_resource", Method: http.MethodPut,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}", Idempotent: true,
		KeyLevel: EnvironmentLevel, RequestModel: "ResourceReplace", ResponseModel: "ResourceRead",
	},
	{
		ID: "update_resource", Method: http.MethodPatch,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}", Idempotent: true,
		KeyLevel: EnvironmentLevel, RequestModel: "ResourceUpdate", ResponseModel: "ResourceRead",
	},
	{
		ID: "delete_resource", Method: http.MethodDelete,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}", Idempotent: true,
		KeyLevel: EnvironmentLevel, DeleteSelf: true,
	},

	// The tenants of an environment, calls of the facts API, which the PDP container also
	// serves under /facts. v1 sent them with an environment-level key, and the PDP container
	// sends them on with its own, an environment-level key. These key levels are not measured
	// yet: the phase api.tenants of the local harness confirms them. The PDP container waits for
	// sync on the create alone. The list is a JSON array or a paginated object of at most 100
	// tenants. A PATCH sets the fields that it names to the values that it gives, so it does the
	// same when sent twice; a create does not.
	{
		ID: "list_tenants", Method: http.MethodGet, Route: "/v2/facts/{proj_id}/{env_id}/tenants",
		Idempotent: true, Facts: true, KeyLevel: EnvironmentLevel, MaxPerPage: 100,
		ResponseModel: "TenantRead",
	},
	{
		ID: "create_tenant", Method: http.MethodPost,
		Route: "/v2/facts/{proj_id}/{env_id}/tenants", Facts: true, WaitsForSync: true,
		KeyLevel: EnvironmentLevel, RequestModel: "TenantCreate", ResponseModel: "TenantRead",
	},
	{
		ID: "get_tenant", Method: http.MethodGet,
		Route:      "/v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}",
		Idempotent: true, Facts: true, KeyLevel: EnvironmentLevel, ResponseModel: "TenantRead",
	},
	{
		ID: "update_tenant", Method: http.MethodPatch,
		Route:      "/v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}",
		Idempotent: true, Facts: true, KeyLevel: EnvironmentLevel, RequestModel: "TenantUpdate",
		ResponseModel: "TenantRead",
	},
	{
		ID: "delete_tenant", Method: http.MethodDelete,
		Route:      "/v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}",
		Idempotent: true, Facts: true, KeyLevel: EnvironmentLevel, DeleteSelf: true,
	},

	// The users in a tenant: their list, the add of a user, which creates it, and the removal of
	// one, calls of the facts API like those of the tenants. v1 sent the list with an
	// environment-level key, and the PDP container sends each of them on with its own, an
	// environment-level key. These key levels are not measured yet: the phase api.tenants of the
	// local harness confirms them. The PDP container waits for sync on none of them. The list is
	// the paginated object of at most 100 users. The add creates a user, so it is not Idempotent;
	// the path of the removal names the tenant and the user, so it is DeleteSelf.
	{
		ID: "list_tenant_users", Method: http.MethodGet,
		Route:      "/v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}/users",
		Idempotent: true, Facts: true, KeyLevel: EnvironmentLevel, MaxPerPage: 100,
		ResponseModel: "UserRead",
	},
	{
		ID: "add_user_to_tenant", Method: http.MethodPost,
		Route: "/v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}/users", Facts: true,
		KeyLevel: EnvironmentLevel, RequestModel: "UserCreate", ResponseModel: "UserRead",
	},
	{
		ID: "delete_tenant_user", Method: http.MethodDelete,
		Route:      "/v2/facts/{proj_id}/{env_id}/tenants/{tenant_id}/users/{user_id}",
		Idempotent: true, Facts: true, KeyLevel: EnvironmentLevel, DeleteSelf: true,
	},

	// The users of an environment, calls of the facts API, which the PDP container also serves
	// under /facts. v1 sent them with an environment-level key, and the PDP container sends them
	// on with its own, an environment-level key. These key levels are not measured yet: the
	// phase api.users of the local harness confirms them. The PDP container waits for sync on
	// the create, the PUT and the PATCH of a user and on the assignment and the unassignment of
	// its roles, and not on its delete. The list is the paginated object of at most 100 users. A
	// PUT sends the whole user, a PATCH sets the fields that it names to the values that it gives,
	// and an unassignment of a role leaves the role removed, so each does the same when sent
	// twice; a create does not, and what the API answers to the assignment of a role that the user
	// has already is not confirmed, so neither the create nor the assignment is Idempotent. An
	// unassignment names the role in its body, not in its path, so it is not DeleteSelf: a 404 can
	// mean a mistyped key.
	{
		ID: "list_users", Method: http.MethodGet, Route: "/v2/facts/{proj_id}/{env_id}/users",
		Idempotent: true, Facts: true, KeyLevel: EnvironmentLevel, MaxPerPage: 100,
		ResponseModel: "UserRead",
	},
	{
		ID: "create_user", Method: http.MethodPost, Route: "/v2/facts/{proj_id}/{env_id}/users",
		Facts: true, WaitsForSync: true, KeyLevel: EnvironmentLevel, RequestModel: "UserCreate",
		ResponseModel: "UserRead",
	},
	{
		ID: "get_user", Method: http.MethodGet,
		Route:      "/v2/facts/{proj_id}/{env_id}/users/{user_id}",
		Idempotent: true, Facts: true, KeyLevel: EnvironmentLevel, ResponseModel: "UserRead",
	},
	{
		ID: "replace_user", Method: http.MethodPut,
		Route:      "/v2/facts/{proj_id}/{env_id}/users/{user_id}",
		Idempotent: true, Facts: true, WaitsForSync: true, KeyLevel: EnvironmentLevel,
		RequestModel: "UserCreate", ResponseModel: "UserRead",
	},
	{
		ID: "update_user", Method: http.MethodPatch,
		Route:      "/v2/facts/{proj_id}/{env_id}/users/{user_id}",
		Idempotent: true, Facts: true, WaitsForSync: true, KeyLevel: EnvironmentLevel,
		RequestModel: "UserUpdate", ResponseModel: "UserRead",
	},
	{
		ID: "delete_user", Method: http.MethodDelete,
		Route:      "/v2/facts/{proj_id}/{env_id}/users/{user_id}",
		Idempotent: true, Facts: true, KeyLevel: EnvironmentLevel, DeleteSelf: true,
	},
	{
		ID: "assign_role_to_user", Method: http.MethodPost,
		Route: "/v2/facts/{proj_id}/{env_id}/users/{user_id}/roles", Facts: true,
		WaitsForSync: true, KeyLevel: EnvironmentLevel, RequestModel: "UserRoleCreate",
		ResponseModel: "RoleAssignmentRead",
	},
	{
		ID: "unassign_role_from_user", Method: http.MethodDelete,
		Route:      "/v2/facts/{proj_id}/{env_id}/users/{user_id}/roles",
		Idempotent: true, Facts: true, WaitsForSync: true, KeyLevel: EnvironmentLevel,
		RequestModel: "UserRoleRemove",
	},

	// The role assignments of an environment, calls of the facts API, which the PDP container
	// also serves under /facts. v1 sent the list and the bulk assignment and unassignment with an
	// environment-level key, and the PDP container sends each call on with its own, an
	// environment-level key: the assignment and the unassignment through the routes that its spec
	// lists, and the lists and the bulk routes through its route for every other path under
	// /facts. These key levels are not measured yet: the phase api.role_assignments of the local
	// harness confirms them. The PDP container waits for sync on the assignment and the
	// unassignment, and not on the lists or the bulk routes. The list is a JSON array or a
	// paginated object, and the detailed list a paginated object, of at most 1000 role
	// assignments. The bulk routes take a JSON array of the role assignments, and answer a report
	// that counts those that they created or removed. An unassignment, one or in bulk, leaves the
	// roles removed, so it does the same when sent twice; the API's spec lists a 409 for an
	// assignment, one or in bulk, so neither is Idempotent. An unassignment names the roles in its
	// body, not in its path, so it is not DeleteSelf: a 404 can mean a mistyped key. The SDK never
	// sends the deprecated query parameters of these routes, detailed, include_total_count and
	// return_deleted.
	{
		ID: "list_role_assignments", Method: http.MethodGet,
		Route: "/v2/facts/{proj_id}/{env_id}/role_assignments", Idempotent: true, Facts: true,
		KeyLevel: EnvironmentLevel, MaxPerPage: 1000, ResponseModel: "RoleAssignmentRead",
	},
	{
		ID: "list_role_assignments_detailed", Method: http.MethodGet,
		Route: "/v2/facts/{proj_id}/{env_id}/role_assignments/detailed", Idempotent: true,
		Facts: true, KeyLevel: EnvironmentLevel, MaxPerPage: 1000,
		ResponseModel: "RoleAssignmentDetailedRead",
	},
	{
		ID: "assign_role", Method: http.MethodPost,
		Route: "/v2/facts/{proj_id}/{env_id}/role_assignments", Facts: true, WaitsForSync: true,
		KeyLevel: EnvironmentLevel, RequestModel: "RoleAssignmentCreate",
		ResponseModel: "RoleAssignmentRead",
	},
	{
		ID: "unassign_role", Method: http.MethodDelete,
		Route: "/v2/facts/{proj_id}/{env_id}/role_assignments", Idempotent: true, Facts: true,
		WaitsForSync: true, KeyLevel: EnvironmentLevel, RequestModel: "RoleAssignmentRemove",
	},
	{
		ID: "bulk_assign_role", Method: http.MethodPost,
		Route: "/v2/facts/{proj_id}/{env_id}/role_assignments/bulk", Facts: true,
		KeyLevel: EnvironmentLevel, RequestModel: "[]RoleAssignmentCreate",
		ResponseModel: "BulkRoleAssignmentReport",
	},
	{
		ID: "bulk_unassign_role", Method: http.MethodDelete,
		Route: "/v2/facts/{proj_id}/{env_id}/role_assignments/bulk", Idempotent: true,
		Facts: true, KeyLevel: EnvironmentLevel, RequestModel: "[]RoleAssignmentRemove",
		ResponseModel: "BulkRoleUnAssignmentReport",
	},

	// The resource instances of an environment, calls of the facts API, which the PDP container
	// also serves under /facts. v1's integration tests created and deleted them with an
	// environment-level key, and the PDP container sends each of them on with its own, an
	// environment-level key. These key levels are not measured yet: the phase
	// api.resource_instances of the local harness confirms them. The PDP container waits for
	// sync on the create and the PATCH of an instance, and not on the lists, the get and the
	// delete. The list is a JSON array or a paginated object of at most 100 instances, and the
	// detailed list, which holds the relationships of each instance, the paginated object alone.
	// A PATCH sets the attributes that it gives, so it does the same when sent twice; a create
	// does not.
	{
		ID: "list_resource_instances", Method: http.MethodGet,
		Route: "/v2/facts/{proj_id}/{env_id}/resource_instances", Idempotent: true, Facts: true,
		KeyLevel: EnvironmentLevel, MaxPerPage: 100, ResponseModel: "ResourceInstanceRead",
	},
	{
		ID: "create_resource_instance", Method: http.MethodPost,
		Route: "/v2/facts/{proj_id}/{env_id}/resource_instances", Facts: true, WaitsForSync: true,
		KeyLevel: EnvironmentLevel, RequestModel: "ResourceInstanceCreate",
		ResponseModel: "ResourceInstanceRead",
	},
	{
		ID: "list_resource_instances_detailed", Method: http.MethodGet,
		Route:      "/v2/facts/{proj_id}/{env_id}/resource_instances/detailed",
		Idempotent: true, Facts: true, KeyLevel: EnvironmentLevel, MaxPerPage: 100,
		ResponseModel: "ResourceInstanceDetailedRead",
	},
	{
		ID: "get_resource_instance", Method: http.MethodGet,
		Route:      "/v2/facts/{proj_id}/{env_id}/resource_instances/{instance_id}",
		Idempotent: true, Facts: true, KeyLevel: EnvironmentLevel,
		ResponseModel: "ResourceInstanceRead",
	},
	{
		ID: "update_resource_instance", Method: http.MethodPatch,
		Route:      "/v2/facts/{proj_id}/{env_id}/resource_instances/{instance_id}",
		Idempotent: true, Facts: true, WaitsForSync: true, KeyLevel: EnvironmentLevel,
		RequestModel: "ResourceInstanceUpdate", ResponseModel: "ResourceInstanceRead",
	},
	{
		ID: "delete_resource_instance", Method: http.MethodDelete,
		Route:      "/v2/facts/{proj_id}/{env_id}/resource_instances/{instance_id}",
		Idempotent: true, Facts: true, KeyLevel: EnvironmentLevel, DeleteSelf: true,
	},

	// The relationship tuples of an environment, calls of the facts API, which the PDP container
	// also serves under /facts. v1 sent each of them with an environment-level key, and the PDP
	// container sends each call on with its own, an environment-level key: the create through
	// the route that its spec lists, and the others through its route for every other path under
	// /facts. These key levels are not measured yet: the phase api.relationship_tuples of the
	// local harness confirms them. The PDP container waits for sync on the create, and not on
	// the delete or the lists. The list is a JSON array or a paginated object, and the detailed
	// list a paginated object, of at most 100 relationship tuples. A delete leaves the tuple
	// deleted, so it does the same when sent twice; what the API answers to the create of a tuple
	// that exists is not confirmed, so the create is not Idempotent. A delete names the tuple in
	// its body, not in its path, so it is not DeleteSelf: a 404 can mean a mistyped key. The SDK
	// never sends the deprecated query parameters of the list, detailed and include_total_count.
	{
		ID: "list_relationship_tuples", Method: http.MethodGet,
		Route: "/v2/facts/{proj_id}/{env_id}/relationship_tuples", Idempotent: true, Facts: true,
		KeyLevel: EnvironmentLevel, MaxPerPage: 100, ResponseModel: "RelationshipTupleRead",
	},
	{
		ID: "list_relationship_tuples_detailed", Method: http.MethodGet,
		Route: "/v2/facts/{proj_id}/{env_id}/relationship_tuples/detailed", Idempotent: true,
		Facts: true, KeyLevel: EnvironmentLevel, MaxPerPage: 100,
		ResponseModel: "RelationshipTupleDetailedRead",
	},
	{
		ID: "create_relationship_tuple", Method: http.MethodPost,
		Route: "/v2/facts/{proj_id}/{env_id}/relationship_tuples", Facts: true,
		WaitsForSync: true, KeyLevel: EnvironmentLevel, RequestModel: "RelationshipTupleCreate",
		ResponseModel: "RelationshipTupleRead",
	},
	{
		ID: "delete_relationship_tuple", Method: http.MethodDelete,
		Route: "/v2/facts/{proj_id}/{env_id}/relationship_tuples", Idempotent: true, Facts: true,
		KeyLevel: EnvironmentLevel, RequestModel: "RelationshipTupleDelete",
	},

	// The bulk operations of the users, the tenants and the resource instances of an
	// environment, calls of the facts API, which the PDP container also serves under /facts,
	// through its route for every path under /facts that its spec does not list. v1 sent none of
	// them, and the PDP container sends each on with its own, an environment-level key, as it
	// sends the other calls of the facts API. These key levels are not measured yet: the phase
	// api.bulk of the local harness confirms them. The PDP container waits for sync on none of
	// them. Each body is a JSON object that holds the items in an array, operations for a create
	// or a replace and idents for a delete, of no least or largest number in the spec; each reply
	// is a JSON object without fields, which the SDK does not read, so none has a ResponseModel. A
	// PUT sends the whole users or instances, and a delete leaves them deleted, so each does the
	// same when sent twice; a create does not. A delete names the objects in its body, not in its
	// path, so it is not DeleteSelf: a 404 can mean a mistyped key.
	{
		ID: "bulk_create_users", Method: http.MethodPost,
		Route: "/v2/facts/{proj_id}/{env_id}/bulk/users", Facts: true,
		KeyLevel: EnvironmentLevel, RequestModel: "UserCreateBulkOperation",
	},
	{
		ID: "bulk_replace_users", Method: http.MethodPut,
		Route: "/v2/facts/{proj_id}/{env_id}/bulk/users", Idempotent: true, Facts: true,
		KeyLevel: EnvironmentLevel, RequestModel: "UserReplaceBulkOperation",
	},
	{
		ID: "bulk_delete_users", Method: http.MethodDelete,
		Route: "/v2/facts/{proj_id}/{env_id}/bulk/users", Idempotent: true, Facts: true,
		KeyLevel: EnvironmentLevel, RequestModel: "UserDeleteBulkOperation",
	},
	{
		ID: "bulk_create_tenants", Method: http.MethodPost,
		Route: "/v2/facts/{proj_id}/{env_id}/bulk/tenants", Facts: true,
		KeyLevel: EnvironmentLevel, RequestModel: "TenantCreateBulkOperation",
	},
	{
		ID: "bulk_delete_tenants", Method: http.MethodDelete,
		Route: "/v2/facts/{proj_id}/{env_id}/bulk/tenants", Idempotent: true, Facts: true,
		KeyLevel: EnvironmentLevel, RequestModel: "TenantDeleteBulkOperation",
	},
	{
		ID: "bulk_replace_resource_instances", Method: http.MethodPut,
		Route:      "/v2/facts/{proj_id}/{env_id}/bulk/resource_instances",
		Idempotent: true, Facts: true, KeyLevel: EnvironmentLevel,
		RequestModel: "ResourceInstanceCreateBulkOperation",
	},
	{
		ID: "bulk_delete_resource_instances", Method: http.MethodDelete,
		Route:      "/v2/facts/{proj_id}/{env_id}/bulk/resource_instances",
		Idempotent: true, Facts: true, KeyLevel: EnvironmentLevel,
		RequestModel: "ResourceInstanceDeleteBulkOperation",
	},

	// The actions of a resource of an environment, each at the environment level, as v1 sends
	// them too. These key levels are not measured yet: the phase api.resource_actions of the
	// local harness confirms them. The list is a JSON array of at most 100 actions. A PATCH sets
	// the fields that it names to the values that it gives, so it does the same when sent twice;
	// a create does not.
	{
		ID: "list_resource_actions", Method: http.MethodGet,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/actions", Idempotent: true,
		KeyLevel: EnvironmentLevel, MaxPerPage: 100, ResponseModel: "ResourceActionRead",
	},
	{
		ID: "create_resource_action", Method: http.MethodPost,
		Route:    "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/actions",
		KeyLevel: EnvironmentLevel, RequestModel: "ResourceActionCreate",
		ResponseModel: "ResourceActionRead",
	},
	{
		ID: "get_resource_action", Method: http.MethodGet,
		Route:      "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/actions/{action_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, ResponseModel: "ResourceActionRead",
	},
	{
		ID: "update_resource_action", Method: http.MethodPatch,
		Route:      "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/actions/{action_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, RequestModel: "ResourceActionUpdate",
		ResponseModel: "ResourceActionRead",
	},
	{
		ID: "delete_resource_action", Method: http.MethodDelete,
		Route:      "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/actions/{action_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, DeleteSelf: true,
	},

	// The action groups of a resource of an environment, each at the environment level, as v1
	// sends them too. These key levels are not measured yet: the phase api.resource_action_groups
	// of the local harness confirms them. The list is a JSON array of at most 100 action groups. A
	// PATCH sets the fields that it names to the values that it gives, so it does the same when
	// sent twice; a create does not.
	{
		ID: "list_resource_action_groups", Method: http.MethodGet,
		Route:      "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/action_groups",
		Idempotent: true, KeyLevel: EnvironmentLevel, MaxPerPage: 100,
		ResponseModel: "ResourceActionGroupRead",
	},
	{
		ID: "create_resource_action_group", Method: http.MethodPost,
		Route:    "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/action_groups",
		KeyLevel: EnvironmentLevel, RequestModel: "ResourceActionGroupCreate",
		ResponseModel: "ResourceActionGroupRead",
	},
	{
		ID: "get_resource_action_group", Method: http.MethodGet,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/action_groups/" +
			"{action_group_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, ResponseModel: "ResourceActionGroupRead",
	},
	{
		ID: "update_resource_action_group", Method: http.MethodPatch,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/action_groups/" +
			"{action_group_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, RequestModel: "ResourceActionGroupUpdate",
		ResponseModel: "ResourceActionGroupRead",
	},
	{
		ID: "delete_resource_action_group", Method: http.MethodDelete,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/action_groups/" +
			"{action_group_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, DeleteSelf: true,
	},

	// The attributes of a resource of an environment, each at the environment level, as v1 sends
	// them too. These key levels are not measured yet: the phase api.resource_attributes of the
	// local harness confirms them. The list is a JSON array of at most 100 attributes. A PATCH
	// sets the fields that it names to the values that it gives, so it does the same when sent
	// twice; a create does not. The spec lists page and per_page on the delete, which names one
	// attribute in its path; the SDK sends neither.
	{
		ID: "list_resource_attributes", Method: http.MethodGet,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/attributes", Idempotent: true,
		KeyLevel: EnvironmentLevel, MaxPerPage: 100, ResponseModel: "ResourceAttributeRead",
	},
	{
		ID: "create_resource_attribute", Method: http.MethodPost,
		Route:    "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/attributes",
		KeyLevel: EnvironmentLevel, RequestModel: "ResourceAttributeCreate",
		ResponseModel: "ResourceAttributeRead",
	},
	{
		ID: "get_resource_attribute", Method: http.MethodGet,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/attributes/" +
			"{attribute_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, ResponseModel: "ResourceAttributeRead",
	},
	{
		ID: "update_resource_attribute", Method: http.MethodPatch,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/attributes/" +
			"{attribute_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, RequestModel: "ResourceAttributeUpdate",
		ResponseModel: "ResourceAttributeRead",
	},
	{
		ID: "delete_resource_attribute", Method: http.MethodDelete,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/attributes/" +
			"{attribute_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, DeleteSelf: true,
	},

	// The relations of a resource of an environment, each at the environment level, as v1 sends
	// them too. These key levels are not measured yet: the phase api.resource_relations of the
	// local harness confirms them. The list is the paginated object of at most 100 relations. The
	// API has no update of a relation; a create does not do the same when sent twice.
	{
		ID: "list_resource_relations", Method: http.MethodGet,
		Route:      "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/relations",
		Idempotent: true, KeyLevel: EnvironmentLevel, MaxPerPage: 100,
		ResponseModel: "RelationRead",
	},
	{
		ID: "create_resource_relation", Method: http.MethodPost,
		Route:    "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/relations",
		KeyLevel: EnvironmentLevel, RequestModel: "RelationCreate", ResponseModel: "RelationRead",
	},
	{
		ID: "get_resource_relation", Method: http.MethodGet,
		Route:      "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/relations/{relation_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, ResponseModel: "RelationRead",
	},
	{
		ID: "delete_resource_relation", Method: http.MethodDelete,
		Route:      "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/relations/{relation_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, DeleteSelf: true,
	},

	// The roles of an environment, each at the environment level, as v1 sends them too; v1 did
	// not send the bulk replace, the ancestors or the descendants, which are operations of the
	// environment's schema too. These key levels are not measured yet: the phase api.roles of the
	// local harness confirms them. The list is a JSON array or a paginated object of at most 100
	// roles. A PATCH sets the fields that it names to the values that it gives, and a PUT the
	// roles that it names, so each does the same when sent twice; so does a grant or a removal of
	// permissions, which v1's documentation says skips a permission that the role has already, or
	// does not have. A create does not. A removal of permissions names them in its body, so a 404
	// to it is an error.
	{
		ID: "list_roles", Method: http.MethodGet, Route: "/v2/schema/{proj_id}/{env_id}/roles",
		Idempotent: true, KeyLevel: EnvironmentLevel, MaxPerPage: 100, ResponseModel: "RoleRead",
	},
	{
		ID: "create_role", Method: http.MethodPost, Route: "/v2/schema/{proj_id}/{env_id}/roles",
		KeyLevel: EnvironmentLevel, RequestModel: "RoleCreate", ResponseModel: "RoleRead",
	},
	{
		ID: "get_role", Method: http.MethodGet,
		Route: "/v2/schema/{proj_id}/{env_id}/roles/{role_id}", Idempotent: true,
		KeyLevel: EnvironmentLevel, ResponseModel: "RoleRead",
	},
	{
		ID: "update_role", Method: http.MethodPatch,
		Route: "/v2/schema/{proj_id}/{env_id}/roles/{role_id}", Idempotent: true,
		KeyLevel: EnvironmentLevel, RequestModel: "RoleUpdate", ResponseModel: "RoleRead",
	},
	{
		ID: "delete_role", Method: http.MethodDelete,
		Route: "/v2/schema/{proj_id}/{env_id}/roles/{role_id}", Idempotent: true,
		KeyLevel: EnvironmentLevel, DeleteSelf: true,
	},
	{
		ID: "assign_permissions_to_role", Method: http.MethodPost,
		Route: "/v2/schema/{proj_id}/{env_id}/roles/{role_id}/permissions", Idempotent: true,
		KeyLevel: EnvironmentLevel, RequestModel: "AddRolePermissions", ResponseModel: "RoleRead",
	},
	{
		ID: "remove_permissions_from_role", Method: http.MethodDelete,
		Route: "/v2/schema/{proj_id}/{env_id}/roles/{role_id}/permissions", Idempotent: true,
		KeyLevel: EnvironmentLevel, RequestModel: "RemoveRolePermissions",
		ResponseModel: "RoleRead",
	},
	{
		ID: "get_role_ancestors", Method: http.MethodGet,
		Route: "/v2/schema/{proj_id}/{env_id}/roles/{role_id}/ancestors", Idempotent: true,
		KeyLevel: EnvironmentLevel, ResponseModel: "RoleList",
	},
	{
		ID: "get_role_descendants", Method: http.MethodGet,
		Route: "/v2/schema/{proj_id}/{env_id}/roles/{role_id}/descendants", Idempotent: true,
		KeyLevel: EnvironmentLevel, ResponseModel: "RoleList",
	},
	{
		ID: "bulk_create_or_replace_roles", Method: http.MethodPut,
		Route: "/v2/schema/{proj_id}/{env_id}/bulk/roles", Idempotent: true,
		KeyLevel: EnvironmentLevel, RequestModel: "RoleCreateBulkOperation",
		ResponseModel: "RoleCreateBulkOperationResult",
	},

	// The roles of a resource of an environment, each at the environment level, as v1 sends them
	// too. These key levels are not measured yet: the phase api.resource_roles of the local
	// harness confirms them. The list is a JSON array of at most 100 roles, and the ancestors and
	// the descendants are a JSON array or a paginated object in the roles of an object. A PATCH
	// sets the fields that it names to the values that it gives, so it does the same when sent
	// twice; so does a grant or a removal of permissions, which v1's documentation says skips a
	// permission that the role has already, or does not have. A create does not. A removal of
	// permissions names them in its body, so a 404 to it is an error.
	{
		ID: "list_resource_roles", Method: http.MethodGet,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles", Idempotent: true,
		KeyLevel: EnvironmentLevel, MaxPerPage: 100, ResponseModel: "ResourceRoleRead",
	},
	{
		ID: "create_resource_role", Method: http.MethodPost,
		Route:    "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles",
		KeyLevel: EnvironmentLevel, RequestModel: "ResourceRoleCreate",
		ResponseModel: "ResourceRoleRead",
	},
	{
		ID: "get_resource_role", Method: http.MethodGet,
		Route:      "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, ResponseModel: "ResourceRoleRead",
	},
	{
		ID: "update_resource_role", Method: http.MethodPatch,
		Route:      "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, RequestModel: "ResourceRoleUpdate",
		ResponseModel: "ResourceRoleRead",
	},
	{
		ID: "delete_resource_role", Method: http.MethodDelete,
		Route:      "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, DeleteSelf: true,
	},
	{
		ID: "assign_permissions_to_resource_role", Method: http.MethodPost,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/" +
			"permissions",
		Idempotent: true, KeyLevel: EnvironmentLevel, RequestModel: "AddRolePermissions",
		ResponseModel: "ResourceRoleRead",
	},
	{
		ID: "remove_permissions_from_resource_role", Method: http.MethodDelete,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/" +
			"permissions",
		Idempotent: true, KeyLevel: EnvironmentLevel, RequestModel: "RemoveRolePermissions",
		ResponseModel: "ResourceRoleRead",
	},
	{
		ID: "get_resource_role_ancestors", Method: http.MethodGet,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/" +
			"ancestors",
		Idempotent: true, KeyLevel: EnvironmentLevel, ResponseModel: "ResourceRoleList",
	},
	{
		ID: "get_resource_role_descendants", Method: http.MethodGet,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/" +
			"descendants",
		Idempotent: true, KeyLevel: EnvironmentLevel, ResponseModel: "ResourceRoleList",
	},

	// The implicit grants of a role of a resource of an environment, each at the environment
	// level, as v1 sends them too. These key levels are not measured yet: the phase
	// api.implicit_grants of the local harness confirms them. A create does not do the same when
	// sent twice; a delete leaves the rule deleted, and a PUT of the conditions sets the same
	// conditions. A delete names the rule in its body, so a 404 to it is an error.
	{
		ID: "create_implicit_grant", Method: http.MethodPost,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/" +
			"implicit_grants",
		KeyLevel: EnvironmentLevel, RequestModel: "DerivedRoleRuleCreate",
		ResponseModel: "DerivedRoleRuleRead",
	},
	{
		ID: "delete_implicit_grant", Method: http.MethodDelete,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/" +
			"implicit_grants",
		Idempotent: true, KeyLevel: EnvironmentLevel, RequestModel: "DerivedRoleRuleDelete",
	},
	{
		ID: "update_implicit_grants_conditions", Method: http.MethodPut,
		Route: "/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/" +
			"implicit_grants/conditions",
		Idempotent: true, KeyLevel: EnvironmentLevel, RequestModel: "DerivationSettings",
		ResponseModel: "DerivationSettings",
	},

	// The attributes of the users of an environment, each at the environment level, as the
	// routes of the schema of an environment are, such as those of the attributes of a resource:
	// v1 sent these routes only through its generated client. These key levels are not measured
	// yet: the phase api.user_attributes of the local harness confirms them, and records whether
	// the API still answers 500 to an update. The list is a JSON array of at most 100 attributes.
	// A PATCH sets the fields that it names to the values that it gives, so it does the same when
	// sent twice; a create does not. The spec lists resource_id on each route, __user by default,
	// and page and per_page on the delete, which names one attribute in its path; the SDK sends
	// none of them, so the calls act on the attributes of the users alone.
	{
		ID: "list_user_attributes", Method: http.MethodGet,
		Route: "/v2/schema/{proj_id}/{env_id}/users/attributes", Idempotent: true,
		KeyLevel: EnvironmentLevel, MaxPerPage: 100, ResponseModel: "ResourceAttributeRead",
	},
	{
		ID: "create_user_attribute", Method: http.MethodPost,
		Route: "/v2/schema/{proj_id}/{env_id}/users/attributes", KeyLevel: EnvironmentLevel,
		RequestModel: "ResourceAttributeCreate", ResponseModel: "ResourceAttributeRead",
	},
	{
		ID: "get_user_attribute", Method: http.MethodGet,
		Route:      "/v2/schema/{proj_id}/{env_id}/users/attributes/{attribute_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, ResponseModel: "ResourceAttributeRead",
	},
	{
		ID: "update_user_attribute", Method: http.MethodPatch,
		Route:      "/v2/schema/{proj_id}/{env_id}/users/attributes/{attribute_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, RequestModel: "ResourceAttributeUpdate",
		ResponseModel: "ResourceAttributeRead",
	},
	{
		ID: "delete_user_attribute", Method: http.MethodDelete,
		Route:      "/v2/schema/{proj_id}/{env_id}/users/attributes/{attribute_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, DeleteSelf: true,
	},

	// The condition sets of an environment, each at the environment level, as v1 sends them too;
	// v1 sent the ancestors and the descendants only through its generated client. These key
	// levels are not measured yet: the phase api.condition_sets of the local harness confirms
	// them. The list is a JSON array or a paginated object of at most 100 sets, and the ancestors
	// and the descendants are each a JSON array of at most 100 sets. A PATCH sets the fields that
	// it names to the values that it gives, so it does the same when sent twice; a create does
	// not.
	{
		ID: "list_condition_sets", Method: http.MethodGet,
		Route: "/v2/schema/{proj_id}/{env_id}/condition_sets", Idempotent: true,
		KeyLevel: EnvironmentLevel, MaxPerPage: 100, ResponseModel: "ConditionSetRead",
	},
	{
		ID: "create_condition_set", Method: http.MethodPost,
		Route: "/v2/schema/{proj_id}/{env_id}/condition_sets", KeyLevel: EnvironmentLevel,
		RequestModel: "ConditionSetCreate", ResponseModel: "ConditionSetRead",
	},
	{
		ID: "get_condition_set", Method: http.MethodGet,
		Route:      "/v2/schema/{proj_id}/{env_id}/condition_sets/{condition_set_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, ResponseModel: "ConditionSetRead",
	},
	{
		ID: "update_condition_set", Method: http.MethodPatch,
		Route:      "/v2/schema/{proj_id}/{env_id}/condition_sets/{condition_set_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, RequestModel: "ConditionSetUpdate",
		ResponseModel: "ConditionSetRead",
	},
	{
		ID: "delete_condition_set", Method: http.MethodDelete,
		Route:      "/v2/schema/{proj_id}/{env_id}/condition_sets/{condition_set_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, DeleteSelf: true,
	},
	{
		ID: "get_condition_set_ancestors", Method: http.MethodGet,
		Route:      "/v2/schema/{proj_id}/{env_id}/condition_sets/{condition_set_id}/ancestors",
		Idempotent: true, KeyLevel: EnvironmentLevel, MaxPerPage: 100,
		ResponseModel: "ConditionSetRead",
	},
	{
		ID: "get_condition_set_descendants", Method: http.MethodGet,
		Route:      "/v2/schema/{proj_id}/{env_id}/condition_sets/{condition_set_id}/descendants",
		Idempotent: true, KeyLevel: EnvironmentLevel, MaxPerPage: 100,
		ResponseModel: "ConditionSetRead",
	},

	// The condition set rules of an environment, each at the environment level, as v1 sends them
	// too. These key levels are not measured yet: the phase api.condition_set_rules of the local
	// harness confirms them. Their routes start with /v2/facts/, but the PDP container does not
	// serve them, so they are not Facts, and none waits for sync. The list is a JSON array of at
	// most 100 rules, and the reply to a create is a JSON array of rules. The API's documentation
	// says that a create skips a permission that is granted already, and a delete one that is
	// not, so each does the same when sent twice. A delete names the rule in its body, not in its
	// path, so it is not DeleteSelf: a 404 can mean a mistyped key.
	{
		ID: "list_set_permissions", Method: http.MethodGet,
		Route: "/v2/facts/{proj_id}/{env_id}/set_rules", Idempotent: true,
		KeyLevel: EnvironmentLevel, MaxPerPage: 100, ResponseModel: "ConditionSetRuleRead",
	},
	{
		ID: "assign_set_permissions", Method: http.MethodPost,
		Route: "/v2/facts/{proj_id}/{env_id}/set_rules", Idempotent: true,
		KeyLevel: EnvironmentLevel, RequestModel: "ConditionSetRuleCreate",
		ResponseModel: "[]ConditionSetRuleRead",
	},
	{
		ID: "unassign_set_permissions", Method: http.MethodDelete,
		Route: "/v2/facts/{proj_id}/{env_id}/set_rules", Idempotent: true,
		KeyLevel: EnvironmentLevel, RequestModel: "ConditionSetRuleRemove",
	},

	// The proxy configs of an environment, each at the environment level, as v1 sends them too.
	// These key levels are not measured yet: the phase api.proxy_config of the local harness
	// confirms them. Their routes start with /v2/facts/, but the PDP container does not serve
	// them, so they are not Facts, and none waits for sync. The list is a JSON array of at most
	// 100 configs. A create does not do the same when sent twice, nor does a PATCH, whose
	// mapping rules the API adds to those that it stores, and removes with should_delete; a
	// delete leaves the config deleted. The secret of a config is a credential, which each reply
	// holds as a string or as an object of headers: secret is listed, so that every
	// string inside it joins the client's redaction set, and not the strings directly under it
	// alone, as for its credential name.
	{
		ID: "list_proxy_configs", Method: http.MethodGet,
		Route: "/v2/facts/{proj_id}/{env_id}/proxy_configs", Idempotent: true,
		KeyLevel: EnvironmentLevel, SensitiveBodyFields: []string{"secret"}, MaxPerPage: 100,
		ResponseModel: "ProxyConfigRead",
	},
	{
		ID: "create_proxy_config", Method: http.MethodPost,
		Route: "/v2/facts/{proj_id}/{env_id}/proxy_configs", KeyLevel: EnvironmentLevel,
		SensitiveBodyFields: []string{"secret"}, RequestModel: "ProxyConfigCreate",
		ResponseModel: "ProxyConfigRead",
	},
	{
		ID: "get_proxy_config", Method: http.MethodGet,
		Route:      "/v2/facts/{proj_id}/{env_id}/proxy_configs/{proxy_config_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, SensitiveBodyFields: []string{"secret"},
		ResponseModel: "ProxyConfigRead",
	},
	{
		ID: "update_proxy_config", Method: http.MethodPatch,
		Route:    "/v2/facts/{proj_id}/{env_id}/proxy_configs/{proxy_config_id}",
		KeyLevel: EnvironmentLevel, SensitiveBodyFields: []string{"secret"},
		RequestModel: "ProxyConfigUpdate", ResponseModel: "ProxyConfigRead",
	},
	{
		ID: "delete_proxy_config", Method: http.MethodDelete,
		Route:      "/v2/facts/{proj_id}/{env_id}/proxy_configs/{proxy_config_id}",
		Idempotent: true, KeyLevel: EnvironmentLevel, DeleteSelf: true,
		SensitiveBodyFields: []string{"secret"},
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
