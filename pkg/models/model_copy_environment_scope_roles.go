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

// checks if the CopyEnvironmentScopeRoles type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &CopyEnvironmentScopeRoles{}

// CopyEnvironmentScopeRoles CopyEnvironmentScopeRoles to copy
type CopyEnvironmentScopeRoles struct {
	// Objects to include (use * as wildcard)
	Include []string `json:"include,omitempty"`
	// Object to exclude (use * as wildcard)
	Exclude []string `json:"exclude,omitempty"`
}

// NewCopyEnvironmentScopeRoles instantiates a new CopyEnvironmentScopeRoles object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewCopyEnvironmentScopeRoles() *CopyEnvironmentScopeRoles {
	this := CopyEnvironmentScopeRoles{}
	return &this
}

// NewCopyEnvironmentScopeRolesWithDefaults instantiates a new CopyEnvironmentScopeRoles object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewCopyEnvironmentScopeRolesWithDefaults() *CopyEnvironmentScopeRoles {
	this := CopyEnvironmentScopeRoles{}
	return &this
}

// GetInclude returns the Include field value if set, zero value otherwise.
func (o *CopyEnvironmentScopeRoles) GetInclude() []string {
	if o == nil || IsNil(o.Include) {
		var ret []string
		return ret
	}
	return o.Include
}

// GetIncludeOk returns a tuple with the Include field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *CopyEnvironmentScopeRoles) GetIncludeOk() ([]string, bool) {
	if o == nil || IsNil(o.Include) {
		return nil, false
	}
	return o.Include, true
}

// HasInclude returns a boolean if a field has been set.
func (o *CopyEnvironmentScopeRoles) HasInclude() bool {
	if o != nil && !IsNil(o.Include) {
		return true
	}

	return false
}

// SetInclude gets a reference to the given []string and assigns it to the Include field.
func (o *CopyEnvironmentScopeRoles) SetInclude(v []string) {
	o.Include = v
}

// GetExclude returns the Exclude field value if set, zero value otherwise.
func (o *CopyEnvironmentScopeRoles) GetExclude() []string {
	if o == nil || IsNil(o.Exclude) {
		var ret []string
		return ret
	}
	return o.Exclude
}

// GetExcludeOk returns a tuple with the Exclude field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *CopyEnvironmentScopeRoles) GetExcludeOk() ([]string, bool) {
	if o == nil || IsNil(o.Exclude) {
		return nil, false
	}
	return o.Exclude, true
}

// HasExclude returns a boolean if a field has been set.
func (o *CopyEnvironmentScopeRoles) HasExclude() bool {
	if o != nil && !IsNil(o.Exclude) {
		return true
	}

	return false
}

// SetExclude gets a reference to the given []string and assigns it to the Exclude field.
func (o *CopyEnvironmentScopeRoles) SetExclude(v []string) {
	o.Exclude = v
}

func (o CopyEnvironmentScopeRoles) MarshalJSON() ([]byte, error) {
	toSerialize, err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o CopyEnvironmentScopeRoles) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	if !IsNil(o.Include) {
		toSerialize["include"] = o.Include
	}
	if !IsNil(o.Exclude) {
		toSerialize["exclude"] = o.Exclude
	}
	return toSerialize, nil
}

type NullableCopyEnvironmentScopeRoles struct {
	value *CopyEnvironmentScopeRoles
	isSet bool
}

func (v NullableCopyEnvironmentScopeRoles) Get() *CopyEnvironmentScopeRoles {
	return v.value
}

func (v *NullableCopyEnvironmentScopeRoles) Set(val *CopyEnvironmentScopeRoles) {
	v.value = val
	v.isSet = true
}

func (v NullableCopyEnvironmentScopeRoles) IsSet() bool {
	return v.isSet
}

func (v *NullableCopyEnvironmentScopeRoles) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableCopyEnvironmentScopeRoles(val *CopyEnvironmentScopeRoles) *NullableCopyEnvironmentScopeRoles {
	return &NullableCopyEnvironmentScopeRoles{value: val, isSet: true}
}

func (v NullableCopyEnvironmentScopeRoles) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableCopyEnvironmentScopeRoles) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}