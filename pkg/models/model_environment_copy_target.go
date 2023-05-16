/*
Permit.io API

 Authorization as a service

API version: 2.0.0
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package models

import (
	"encoding/json"
)

// checks if the EnvironmentCopyTarget type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &EnvironmentCopyTarget{}

// EnvironmentCopyTarget struct for EnvironmentCopyTarget
type EnvironmentCopyTarget struct {
	// Identifier of an existing environment to copy into
	Existing *string `json:"existing,omitempty"`
	New      *New    `json:"new,omitempty"`
}

// NewEnvironmentCopyTarget instantiates a new EnvironmentCopyTarget object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewEnvironmentCopyTarget() *EnvironmentCopyTarget {
	this := EnvironmentCopyTarget{}
	return &this
}

// NewEnvironmentCopyTargetWithDefaults instantiates a new EnvironmentCopyTarget object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewEnvironmentCopyTargetWithDefaults() *EnvironmentCopyTarget {
	this := EnvironmentCopyTarget{}
	return &this
}

// GetExisting returns the Existing field value if set, zero value otherwise.
func (o *EnvironmentCopyTarget) GetExisting() string {
	if o == nil || IsNil(o.Existing) {
		var ret string
		return ret
	}
	return *o.Existing
}

// GetExistingOk returns a tuple with the Existing field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *EnvironmentCopyTarget) GetExistingOk() (*string, bool) {
	if o == nil || IsNil(o.Existing) {
		return nil, false
	}
	return o.Existing, true
}

// HasExisting returns a boolean if a field has been set.
func (o *EnvironmentCopyTarget) HasExisting() bool {
	if o != nil && !IsNil(o.Existing) {
		return true
	}

	return false
}

// SetExisting gets a reference to the given string and assigns it to the Existing field.
func (o *EnvironmentCopyTarget) SetExisting(v string) {
	o.Existing = &v
}

// GetNew returns the New field value if set, zero value otherwise.
func (o *EnvironmentCopyTarget) GetNew() New {
	if o == nil || IsNil(o.New) {
		var ret New
		return ret
	}
	return *o.New
}

// GetNewOk returns a tuple with the New field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *EnvironmentCopyTarget) GetNewOk() (*New, bool) {
	if o == nil || IsNil(o.New) {
		return nil, false
	}
	return o.New, true
}

// HasNew returns a boolean if a field has been set.
func (o *EnvironmentCopyTarget) HasNew() bool {
	if o != nil && !IsNil(o.New) {
		return true
	}

	return false
}

// SetNew gets a reference to the given New and assigns it to the New field.
func (o *EnvironmentCopyTarget) SetNew(v New) {
	o.New = &v
}

func (o EnvironmentCopyTarget) MarshalJSON() ([]byte, error) {
	toSerialize, err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o EnvironmentCopyTarget) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	if !IsNil(o.Existing) {
		toSerialize["existing"] = o.Existing
	}
	if !IsNil(o.New) {
		toSerialize["new"] = o.New
	}
	return toSerialize, nil
}

type NullableEnvironmentCopyTarget struct {
	value *EnvironmentCopyTarget
	isSet bool
}

func (v NullableEnvironmentCopyTarget) Get() *EnvironmentCopyTarget {
	return v.value
}

func (v *NullableEnvironmentCopyTarget) Set(val *EnvironmentCopyTarget) {
	v.value = val
	v.isSet = true
}

func (v NullableEnvironmentCopyTarget) IsSet() bool {
	return v.isSet
}

func (v *NullableEnvironmentCopyTarget) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableEnvironmentCopyTarget(val *EnvironmentCopyTarget) *NullableEnvironmentCopyTarget {
	return &NullableEnvironmentCopyTarget{value: val, isSet: true}
}

func (v NullableEnvironmentCopyTarget) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableEnvironmentCopyTarget) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}