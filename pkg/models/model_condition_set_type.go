/*
Permit.io API

 Authorization as a service

API version: 2.0.0
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package models

import (
	"encoding/json"
	"fmt"
)

// ConditionSetType An enumeration.
type ConditionSetType string

// List of ConditionSetType
const (
	USERSET     ConditionSetType = "userset"
	RESOURCESET ConditionSetType = "resourceset"
)

// All allowed values of ConditionSetType enum
var AllowedConditionSetTypeEnumValues = []ConditionSetType{
	"userset",
	"resourceset",
}

func (v *ConditionSetType) UnmarshalJSON(src []byte) error {
	var value string
	err := json.Unmarshal(src, &value)
	if err != nil {
		return err
	}
	enumTypeValue := ConditionSetType(value)
	for _, existing := range AllowedConditionSetTypeEnumValues {
		if existing == enumTypeValue {
			*v = enumTypeValue
			return nil
		}
	}

	return fmt.Errorf("%+v is not a valid ConditionSetType", value)
}

// NewConditionSetTypeFromValue returns a pointer to a valid ConditionSetType
// for the value passed as argument, or an error if the value passed is not allowed by the enum
func NewConditionSetTypeFromValue(v string) (*ConditionSetType, error) {
	ev := ConditionSetType(v)
	if ev.IsValid() {
		return &ev, nil
	} else {
		return nil, fmt.Errorf("invalid value '%v' for ConditionSetType: valid values are %v", v, AllowedConditionSetTypeEnumValues)
	}
}

// IsValid return true if the value is valid for the enum, false otherwise
func (v ConditionSetType) IsValid() bool {
	for _, existing := range AllowedConditionSetTypeEnumValues {
		if existing == v {
			return true
		}
	}
	return false
}

// Ptr returns reference to ConditionSetType value
func (v ConditionSetType) Ptr() *ConditionSetType {
	return &v
}

type NullableConditionSetType struct {
	value *ConditionSetType
	isSet bool
}

func (v NullableConditionSetType) Get() *ConditionSetType {
	return v.value
}

func (v *NullableConditionSetType) Set(val *ConditionSetType) {
	v.value = val
	v.isSet = true
}

func (v NullableConditionSetType) IsSet() bool {
	return v.isSet
}

func (v *NullableConditionSetType) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableConditionSetType(val *ConditionSetType) *NullableConditionSetType {
	return &NullableConditionSetType{value: val, isSet: true}
}

func (v NullableConditionSetType) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableConditionSetType) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
