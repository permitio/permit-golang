package drift

import "github.com/permitio/permit-golang/v2/models"

// modelTypes holds a value of each type that model-map.yaml maps to a schema, so that the drift
// tests can read the type's fields and decode into it. Add each type that gets a row under
// models; TestRepositoryModelMap fails while the two differ.
var modelTypes = []any{
	models.APIKeyScopeRead{},
	models.AttributeType(""),
}
