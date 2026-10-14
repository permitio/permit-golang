package drift

import (
	"reflect"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// modelTypes holds a value of each type that model-map.yaml maps to a schema, so that the drift
// tests can read the type's fields and decode into it: the types of package models, and the
// decision types of the root package. Add each type that gets a row under models;
// TestRepositoryModelMap fails while the two differ.
var modelTypes = []any{
	models.APIKeyScopeRead{},
	models.AttributeType(""),
	permit.CheckRequest{},
	permit.Resource{},
	permit.User{},
}

// rootPath is the import path of the root package, whose decision types modelTypes holds.
var rootPath = reflect.TypeFor[permit.CheckRequest]().PkgPath()
