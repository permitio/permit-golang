package models_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/permitio/permit-golang/v2/internal/surface"
	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// openEnum is an enum of package models: a string type with a Known method.
type openEnum interface {
	~string
	Known() bool
}

// unknownValue is a value that no enum of package models knows, as a value that the API adds
// later is.
const unknownValue = "added_later"

// checkOpenEnum checks E in a subtest named after it, and returns its name. Each of known, every
// constant of E, is Known and encodes as its string; the empty string is not Known; and a value
// that E does not know decodes as it is, is not Known and encodes back unchanged.
func checkOpenEnum[E openEnum](t *testing.T, known ...E) string {
	t.Helper()
	name := reflect.TypeFor[E]().Name()
	t.Run(name, func(t *testing.T) {
		for _, value := range known {
			if !value.Known() {
				t.Errorf("%q is not Known", value)
			}
			encoded, err := wire.Encode(value)
			if err != nil {
				t.Fatal(err)
			}
			if want := `"` + string(value) + `"`; string(encoded) != want {
				t.Errorf("Encode(%q) = %s, want %s", value, encoded, want)
			}
		}
		if E("").Known() {
			t.Error(`"" is Known`)
		}

		var got E
		if err := json.Unmarshal([]byte(`"`+unknownValue+`"`), &got); err != nil {
			t.Fatalf("a value that the SDK does not know fails to decode: %v; an enum must "+
				"have no UnmarshalJSON that rejects it", err)
		}
		if got != E(unknownValue) || got.Known() {
			t.Errorf("%q decodes to %q with Known() = %t, want %q with Known() = false",
				unknownValue, got, got.Known(), unknownValue)
		}
		encoded, err := wire.Encode(got)
		if err != nil {
			t.Fatal(err)
		}
		if want := `"` + unknownValue + `"`; string(encoded) != want {
			t.Errorf("Encode(%q) = %s, want %s", got, encoded, want)
		}
	})
	return name
}

// TestOpenEnums checks each enum of package models with all its constants, and fails when the
// package has a type with a Known method that it does not check: add each new enum here.
func TestOpenEnums(t *testing.T) {
	checked := []string{
		checkOpenEnum(t, models.AttributeTypeBool, models.AttributeTypeNumber,
			models.AttributeTypeString, models.AttributeTypeTime, models.AttributeTypeArray,
			models.AttributeTypeJSON, models.AttributeTypeObject,
			models.AttributeTypeObjectArray),
		checkOpenEnum(t, models.MemberAccessObjOrg, models.MemberAccessObjProject,
			models.MemberAccessObjEnv),
		checkOpenEnum(t, models.MemberAccessLevelAdmin, models.MemberAccessLevelWrite,
			models.MemberAccessLevelRead, models.MemberAccessLevelNoAccess),
		checkOpenEnum(t, models.APIKeyOwnerTypePDPConfig, models.APIKeyOwnerTypeMember,
			models.APIKeyOwnerTypeElements, models.APIKeyOwnerTypeNATSPDPConfig),
		checkOpenEnum(t, models.EnvironmentCopyConflictStrategyFail,
			models.EnvironmentCopyConflictStrategyOverwrite),
		checkOpenEnum(t, models.TaskStatusProcessing, models.TaskStatusSuccess,
			models.TaskStatusFailure, models.TaskStatusCancelled),
		checkOpenEnum(t, models.ConditionSetTypeUserSet, models.ConditionSetTypeResourceSet),
		checkOpenEnum(t, models.ErrorCodeInvalidInput, models.ErrorCodeInvalidJWKSConfiguration,
			models.ErrorCodeUnexpectedError, models.ErrorCodeNotFound,
			models.ErrorCodeDuplicateEntity, models.ErrorCodeEmptyDecisionLogs,
			models.ErrorCodeMissingRequestAttribute, models.ErrorCodeForbiddenAccess,
			models.ErrorCodeInvalidPermissionFormat, models.ErrorCodeMissingPermissions,
			models.ErrorCodeUnsupportedAttributeType, models.ErrorCodeMissingResourceAttribute,
			models.ErrorCodeMismatchAttributesTypes, models.ErrorCodeInvalidPolicyRepoStatus,
			models.ErrorCodeInvalidSMTPConfiguration,
			models.ErrorCodeMismatchResourceInstancesTenants,
			models.ErrorCodePaginationSizeOverreached, models.ErrorCodeMismatchRelationType,
			models.ErrorCodeConcurrentOperationDisallowed, models.ErrorCodeUnauthorized),
		checkOpenEnum(t, models.UserStatusActive, models.UserStatusPending),
	}

	p, err := surface.Read(".")
	if err != nil {
		t.Fatal(err)
	}
	var withKnown []string
	for _, item := range p.Items {
		if item.Kind == surface.Method && item.Name == "Known" {
			withKnown = append(withKnown, item.Recv)
		}
	}
	slices.Sort(withKnown)
	slices.Sort(checked)
	if !slices.Equal(withKnown, checked) {
		t.Errorf("package models has Known methods on %q, and TestOpenEnums checks %q; check "+
			"each enum in TestOpenEnums", withKnown, checked)
	}
}
