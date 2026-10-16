package models

// AttributeType is the type of the values of an attribute, such as string or number.
//
// It is an open enum, as every enum of this package is: a string type with a constant for each
// value that this version of the SDK knows, whose name starts with the type's name, and a Known
// method. It has no UnmarshalJSON method, so a value that the API adds later decodes as it is
// and Known reports false for it: a reply that holds the value is read rather than rejected. A
// request sends any value as it is, known or not.
type AttributeType string

// The values of AttributeType that this version of the SDK knows.
const (
	// AttributeTypeBool is a boolean, true or false.
	AttributeTypeBool AttributeType = "bool"

	// AttributeTypeNumber is a number.
	AttributeTypeNumber AttributeType = "number"

	// AttributeTypeString is a string.
	AttributeTypeString AttributeType = "string"

	// AttributeTypeTime is a time.
	AttributeTypeTime AttributeType = "time"

	// AttributeTypeArray is an array.
	AttributeTypeArray AttributeType = "array"

	// AttributeTypeJSON is any JSON value.
	AttributeTypeJSON AttributeType = "json"

	// AttributeTypeObject is a JSON object.
	AttributeTypeObject AttributeType = "object"

	// AttributeTypeObjectArray is an array of JSON objects.
	AttributeTypeObjectArray AttributeType = "object_array"
)

// Known reports whether t is one of the constants of AttributeType, the values that this
// version of the SDK knows.
func (t AttributeType) Known() bool {
	switch t {
	case AttributeTypeBool, AttributeTypeNumber, AttributeTypeString, AttributeTypeTime,
		AttributeTypeArray, AttributeTypeJSON, AttributeTypeObject, AttributeTypeObjectArray:
		return true
	default:
		return false
	}
}
