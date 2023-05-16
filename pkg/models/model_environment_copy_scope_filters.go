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

// checks if the EnvironmentCopyScopeFilters type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &EnvironmentCopyScopeFilters{}

// EnvironmentCopyScopeFilters struct for EnvironmentCopyScopeFilters
type EnvironmentCopyScopeFilters struct {
	// Objects to include (use * as wildcard)
	Include []string `json:"include,omitempty"`
	// Object to exclude (use * as wildcard)
	Exclude []string `json:"exclude,omitempty"`
}

// NewEnvironmentCopyScopeFilters instantiates a new EnvironmentCopyScopeFilters object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewEnvironmentCopyScopeFilters() *EnvironmentCopyScopeFilters {
	this := EnvironmentCopyScopeFilters{}
	return &this
}

// NewEnvironmentCopyScopeFiltersWithDefaults instantiates a new EnvironmentCopyScopeFilters object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewEnvironmentCopyScopeFiltersWithDefaults() *EnvironmentCopyScopeFilters {
	this := EnvironmentCopyScopeFilters{}
	return &this
}

// GetInclude returns the Include field value if set, zero value otherwise.
func (o *EnvironmentCopyScopeFilters) GetInclude() []string {
	if o == nil || IsNil(o.Include) {
		var ret []string
		return ret
	}
	return o.Include
}

// GetIncludeOk returns a tuple with the Include field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *EnvironmentCopyScopeFilters) GetIncludeOk() ([]string, bool) {
	if o == nil || IsNil(o.Include) {
		return nil, false
	}
	return o.Include, true
}

// HasInclude returns a boolean if a field has been set.
func (o *EnvironmentCopyScopeFilters) HasInclude() bool {
	if o != nil && !IsNil(o.Include) {
		return true
	}

	return false
}

// SetInclude gets a reference to the given []string and assigns it to the Include field.
func (o *EnvironmentCopyScopeFilters) SetInclude(v []string) {
	o.Include = v
}

// GetExclude returns the Exclude field value if set, zero value otherwise.
func (o *EnvironmentCopyScopeFilters) GetExclude() []string {
	if o == nil || IsNil(o.Exclude) {
		var ret []string
		return ret
	}
	return o.Exclude
}

// GetExcludeOk returns a tuple with the Exclude field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *EnvironmentCopyScopeFilters) GetExcludeOk() ([]string, bool) {
	if o == nil || IsNil(o.Exclude) {
		return nil, false
	}
	return o.Exclude, true
}

// HasExclude returns a boolean if a field has been set.
func (o *EnvironmentCopyScopeFilters) HasExclude() bool {
	if o != nil && !IsNil(o.Exclude) {
		return true
	}

	return false
}

// SetExclude gets a reference to the given []string and assigns it to the Exclude field.
func (o *EnvironmentCopyScopeFilters) SetExclude(v []string) {
	o.Exclude = v
}

func (o EnvironmentCopyScopeFilters) MarshalJSON() ([]byte, error) {
	toSerialize, err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o EnvironmentCopyScopeFilters) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	if !IsNil(o.Include) {
		toSerialize["include"] = o.Include
	}
	if !IsNil(o.Exclude) {
		toSerialize["exclude"] = o.Exclude
	}
	return toSerialize, nil
}

type NullableEnvironmentCopyScopeFilters struct {
	value *EnvironmentCopyScopeFilters
	isSet bool
}

func (v NullableEnvironmentCopyScopeFilters) Get() *EnvironmentCopyScopeFilters {
	return v.value
}

func (v *NullableEnvironmentCopyScopeFilters) Set(val *EnvironmentCopyScopeFilters) {
	v.value = val
	v.isSet = true
}

func (v NullableEnvironmentCopyScopeFilters) IsSet() bool {
	return v.isSet
}

func (v *NullableEnvironmentCopyScopeFilters) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableEnvironmentCopyScopeFilters(val *EnvironmentCopyScopeFilters) *NullableEnvironmentCopyScopeFilters {
	return &NullableEnvironmentCopyScopeFilters{value: val, isSet: true}
}

func (v NullableEnvironmentCopyScopeFilters) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableEnvironmentCopyScopeFilters) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}