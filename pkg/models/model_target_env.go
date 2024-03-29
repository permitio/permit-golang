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

// checks if the TargetEnv type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &TargetEnv{}

// TargetEnv If copying a new environment, the environment configuration. If copying to an existing environment, the environment identifier
type TargetEnv struct {
	// Identifier of an existing environment to copy into
	Existing *string `json:"existing,omitempty"`
	New      *New    `json:"new,omitempty"`
}

// NewTargetEnv instantiates a new TargetEnv object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewTargetEnv() *TargetEnv {
	this := TargetEnv{}
	return &this
}

// NewTargetEnvWithDefaults instantiates a new TargetEnv object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewTargetEnvWithDefaults() *TargetEnv {
	this := TargetEnv{}
	return &this
}

// GetExisting returns the Existing field value if set, zero value otherwise.
func (o *TargetEnv) GetExisting() string {
	if o == nil || IsNil(o.Existing) {
		var ret string
		return ret
	}
	return *o.Existing
}

// GetExistingOk returns a tuple with the Existing field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *TargetEnv) GetExistingOk() (*string, bool) {
	if o == nil || IsNil(o.Existing) {
		return nil, false
	}
	return o.Existing, true
}

// HasExisting returns a boolean if a field has been set.
func (o *TargetEnv) HasExisting() bool {
	if o != nil && !IsNil(o.Existing) {
		return true
	}

	return false
}

// SetExisting gets a reference to the given string and assigns it to the Existing field.
func (o *TargetEnv) SetExisting(v string) {
	o.Existing = &v
}

// GetNew returns the New field value if set, zero value otherwise.
func (o *TargetEnv) GetNew() New {
	if o == nil || IsNil(o.New) {
		var ret New
		return ret
	}
	return *o.New
}

// GetNewOk returns a tuple with the New field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *TargetEnv) GetNewOk() (*New, bool) {
	if o == nil || IsNil(o.New) {
		return nil, false
	}
	return o.New, true
}

// HasNew returns a boolean if a field has been set.
func (o *TargetEnv) HasNew() bool {
	if o != nil && !IsNil(o.New) {
		return true
	}

	return false
}

// SetNew gets a reference to the given New and assigns it to the New field.
func (o *TargetEnv) SetNew(v New) {
	o.New = &v
}

func (o TargetEnv) MarshalJSON() ([]byte, error) {
	toSerialize, err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o TargetEnv) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	if !IsNil(o.Existing) {
		toSerialize["existing"] = o.Existing
	}
	if !IsNil(o.New) {
		toSerialize["new"] = o.New
	}
	return toSerialize, nil
}

type NullableTargetEnv struct {
	value *TargetEnv
	isSet bool
}

func (v NullableTargetEnv) Get() *TargetEnv {
	return v.value
}

func (v *NullableTargetEnv) Set(val *TargetEnv) {
	v.value = val
	v.isSet = true
}

func (v NullableTargetEnv) IsSet() bool {
	return v.isSet
}

func (v *NullableTargetEnv) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableTargetEnv(val *TargetEnv) *NullableTargetEnv {
	return &NullableTargetEnv{value: val, isSet: true}
}

func (v NullableTargetEnv) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableTargetEnv) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
