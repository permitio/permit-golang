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

// ElementsUserRoleCreate struct for ElementsUserRoleCreate
type ElementsUserRoleCreate struct {
	// the role that will be assigned (accepts either the role id or the role key)
	Role string `json:"role"`
}

// NewElementsUserRoleCreate instantiates a new ElementsUserRoleCreate object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewElementsUserRoleCreate(role string) *ElementsUserRoleCreate {
	this := ElementsUserRoleCreate{}
	this.Role = role
	return &this
}

// NewElementsUserRoleCreateWithDefaults instantiates a new ElementsUserRoleCreate object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewElementsUserRoleCreateWithDefaults() *ElementsUserRoleCreate {
	this := ElementsUserRoleCreate{}
	return &this
}

// GetRole returns the Role field value
func (o *ElementsUserRoleCreate) GetRole() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Role
}

// GetRoleOk returns a tuple with the Role field value
// and a boolean to check if the value has been set.
func (o *ElementsUserRoleCreate) GetRoleOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Role, true
}

// SetRole sets field value
func (o *ElementsUserRoleCreate) SetRole(v string) {
	o.Role = v
}

func (o ElementsUserRoleCreate) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if true {
		toSerialize["role"] = o.Role
	}
	return json.Marshal(toSerialize)
}

type NullableElementsUserRoleCreate struct {
	value *ElementsUserRoleCreate
	isSet bool
}

func (v NullableElementsUserRoleCreate) Get() *ElementsUserRoleCreate {
	return v.value
}

func (v *NullableElementsUserRoleCreate) Set(val *ElementsUserRoleCreate) {
	v.value = val
	v.isSet = true
}

func (v NullableElementsUserRoleCreate) IsSet() bool {
	return v.isSet
}

func (v *NullableElementsUserRoleCreate) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableElementsUserRoleCreate(val *ElementsUserRoleCreate) *NullableElementsUserRoleCreate {
	return &NullableElementsUserRoleCreate{value: val, isSet: true}
}

func (v NullableElementsUserRoleCreate) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableElementsUserRoleCreate) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}