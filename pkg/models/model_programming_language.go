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

// ProgrammingLanguage An enumeration.
type ProgrammingLanguage string

// List of ProgrammingLanguage
const (
	JAVASCRIPT   ProgrammingLanguage = "javascript"
	PYTHON       ProgrammingLanguage = "python"
	DOTNET       ProgrammingLanguage = "dotnet"
	JAVA         ProgrammingLanguage = "java"
	KONG_GATEWAY ProgrammingLanguage = "kong_gateway"
)

// All allowed values of ProgrammingLanguage enum
var AllowedProgrammingLanguageEnumValues = []ProgrammingLanguage{
	"javascript",
	"python",
	"dotnet",
	"java",
	"kong_gateway",
}

func (v *ProgrammingLanguage) UnmarshalJSON(src []byte) error {
	var value string
	err := json.Unmarshal(src, &value)
	if err != nil {
		return err
	}
	enumTypeValue := ProgrammingLanguage(value)
	for _, existing := range AllowedProgrammingLanguageEnumValues {
		if existing == enumTypeValue {
			*v = enumTypeValue
			return nil
		}
	}

	return fmt.Errorf("%+v is not a valid ProgrammingLanguage", value)
}

// NewProgrammingLanguageFromValue returns a pointer to a valid ProgrammingLanguage
// for the value passed as argument, or an error if the value passed is not allowed by the enum
func NewProgrammingLanguageFromValue(v string) (*ProgrammingLanguage, error) {
	ev := ProgrammingLanguage(v)
	if ev.IsValid() {
		return &ev, nil
	} else {
		return nil, fmt.Errorf("invalid value '%v' for ProgrammingLanguage: valid values are %v", v, AllowedProgrammingLanguageEnumValues)
	}
}

// IsValid return true if the value is valid for the enum, false otherwise
func (v ProgrammingLanguage) IsValid() bool {
	for _, existing := range AllowedProgrammingLanguageEnumValues {
		if existing == v {
			return true
		}
	}
	return false
}

// Ptr returns reference to ProgrammingLanguage value
func (v ProgrammingLanguage) Ptr() *ProgrammingLanguage {
	return &v
}

type NullableProgrammingLanguage struct {
	value *ProgrammingLanguage
	isSet bool
}

func (v NullableProgrammingLanguage) Get() *ProgrammingLanguage {
	return v.value
}

func (v *NullableProgrammingLanguage) Set(val *ProgrammingLanguage) {
	v.value = val
	v.isSet = true
}

func (v NullableProgrammingLanguage) IsSet() bool {
	return v.isSet
}

func (v *NullableProgrammingLanguage) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableProgrammingLanguage(val *ProgrammingLanguage) *NullableProgrammingLanguage {
	return &NullableProgrammingLanguage{value: val, isSet: true}
}

func (v NullableProgrammingLanguage) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableProgrammingLanguage) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}