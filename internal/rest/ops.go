package rest

import "net/http"

// defaultOps are the operations of the SDK, the operations of Default. The commits that add the
// SDK's methods add their operations here. TestRepositoryRegistry, in the tools module, checks
// each against tools/specs: its ID is a planned or covered row of coverage.yaml, its method and
// route are the spec's, and its models are types of package models.
var defaultOps = []Op{
	// Any API key may read its own scope, an environment-level key too.
	{
		ID: "get_api_key_scope", Method: http.MethodGet, Route: "/v2/api-key/scope",
		Idempotent: true, KeyLevel: EnvironmentLevel, ResponseModel: "APIKeyScopeRead",
	},
}
