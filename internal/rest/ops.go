package rest

// defaultOps are the operations of the SDK, the operations of Default. The commits that add the
// SDK's methods add their operations here; until then the list is empty. TestRepositoryRegistry,
// in the tools module, checks each against tools/specs: its ID is a planned or covered row of
// coverage.yaml, its method and route are the spec's, and its models are types of package models.
var defaultOps []Op
