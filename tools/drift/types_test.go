package drift

import (
	"reflect"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

// modelTypes holds a value of each type that model-map.yaml maps to a schema, so that the drift
// tests can read the type's fields and decode into it: the types of package models, and the
// decision types of the root package, those of the requests to the PDP and of its replies. Add
// each type that gets a row under models; TestRepositoryModelMap fails while the two differ.
var modelTypes = []any{
	models.APIKeyCreate{},
	models.APIKeyOwnerType(""),
	models.APIKeyRead{},
	models.APIKeyScopeRead{},
	models.ActionBlockEditable{},
	models.ActionBlockRead{},
	models.AttributeBlockEditable{},
	models.AttributeBlockRead{},
	models.AttributeType(""),
	permit.AuthorizedUserAssignment{},
	permit.AuthorizedUsersRequest{},
	permit.AuthorizedUsersResult{},
	permit.CheckRequest{},
	permit.CheckURLRequest{},
	models.DerivationSettings{},
	models.DerivedRoleBlockEdit{},
	models.DerivedRoleBlockRead{},
	models.DerivedRoleRuleCreate{},
	models.DerivedRoleRuleRead{},
	models.EnvironmentCopy{},
	models.EnvironmentCopyConflictStrategy(""),
	models.EnvironmentCopyScope{},
	models.EnvironmentCopyScopeFilters{},
	models.EnvironmentCopyTarget{},
	models.EnvironmentCreate{},
	models.EnvironmentRead{},
	models.EnvironmentReadWithEmailConfig{},
	models.EnvironmentStatistics{},
	models.EnvironmentStats{},
	models.EnvironmentUpdate{},
	models.ErrorCode(""),
	models.ErrorDetails{},
	models.JwksConfig{},
	models.JwksObj{},
	permit.LocalRoleAssignment{},
	models.MemberAccessLevel(""),
	models.MemberAccessObj(""),
	permit.ObjectPermissions{},
	models.PDPConfigRead{},
	models.ProjectCreate{},
	models.ProjectRead{},
	models.ProjectUpdate{},
	models.RelationBlockRead{},
	permit.Resource{},
	models.ResourceCreate{},
	permit.ResourceDetails{},
	models.ResourceRead{},
	models.ResourceReplace{},
	models.ResourceRoleRead{},
	models.ResourceUpdate{},
	models.RoleBlockEditable{},
	models.TaskResultEnvironmentRead{},
	models.TaskStatus(""),
	permit.TenantDetails{},
	permit.User{},
	permit.UserPermissionsRequest{},
	permit.UserTenantsRequest{},
}

// rootPath is the import path of the root package, whose decision types modelTypes holds.
var rootPath = reflect.TypeFor[permit.CheckRequest]().PkgPath()
