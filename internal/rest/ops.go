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
}
