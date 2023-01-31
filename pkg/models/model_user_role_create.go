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

// UserRoleCreate struct for UserRoleCreate
type UserRoleCreate struct {
	// the role that will be assigned (accepts either the role id or the role key)
	Role string `json:"role"`
	// the tenant the role is associated with (accepts either the tenant id or the tenant key)
	Tenant string `json:"tenant"`
}

// NewUserRoleCreate instantiates a new UserRoleCreate object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewUserRoleCreate(role string, tenant string) *UserRoleCreate {
	this := UserRoleCreate{}
	this.Role = role
	this.Tenant = tenant
	return &this
}

// NewUserRoleCreateWithDefaults instantiates a new UserRoleCreate object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewUserRoleCreateWithDefaults() *UserRoleCreate {
	this := UserRoleCreate{}
	return &this
}

// GetRole returns the Role field value
func (o *UserRoleCreate) GetRole() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Role
}

// GetRoleOk returns a tuple with the Role field value
// and a boolean to check if the value has been set.
func (o *UserRoleCreate) GetRoleOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Role, true
}

// SetRole sets field value
func (o *UserRoleCreate) SetRole(v string) {
	o.Role = v
}

// GetTenant returns the Tenant field value
func (o *UserRoleCreate) GetTenant() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Tenant
}

// GetTenantOk returns a tuple with the Tenant field value
// and a boolean to check if the value has been set.
func (o *UserRoleCreate) GetTenantOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Tenant, true
}

// SetTenant sets field value
func (o *UserRoleCreate) SetTenant(v string) {
	o.Tenant = v
}

func (o UserRoleCreate) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if true {
		toSerialize["role"] = o.Role
	}
	if true {
		toSerialize["tenant"] = o.Tenant
	}
	return json.Marshal(toSerialize)
}

type NullableUserRoleCreate struct {
	value *UserRoleCreate
	isSet bool
}

func (v NullableUserRoleCreate) Get() *UserRoleCreate {
	return v.value
}

func (v *NullableUserRoleCreate) Set(val *UserRoleCreate) {
	v.value = val
	v.isSet = true
}

func (v NullableUserRoleCreate) IsSet() bool {
	return v.isSet
}

func (v *NullableUserRoleCreate) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableUserRoleCreate(val *UserRoleCreate) *NullableUserRoleCreate {
	return &NullableUserRoleCreate{value: val, isSet: true}
}

func (v NullableUserRoleCreate) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableUserRoleCreate) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
