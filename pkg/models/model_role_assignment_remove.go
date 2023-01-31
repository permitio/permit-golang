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

// RoleAssignmentRemove struct for RoleAssignmentRemove
type RoleAssignmentRemove struct {
	// the role that will be unassigned (accepts either the role id or the role key)
	Role string `json:"role"`
	// the tenant the role is associated with (accepts either the tenant id or the tenant key)
	Tenant string `json:"tenant"`
	// the user the role will be unassigned from (accepts either the user id or the user key)
	User string `json:"user"`
}

// NewRoleAssignmentRemove instantiates a new RoleAssignmentRemove object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewRoleAssignmentRemove(role string, tenant string, user string) *RoleAssignmentRemove {
	this := RoleAssignmentRemove{}
	this.Role = role
	this.Tenant = tenant
	this.User = user
	return &this
}

// NewRoleAssignmentRemoveWithDefaults instantiates a new RoleAssignmentRemove object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewRoleAssignmentRemoveWithDefaults() *RoleAssignmentRemove {
	this := RoleAssignmentRemove{}
	return &this
}

// GetRole returns the Role field value
func (o *RoleAssignmentRemove) GetRole() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Role
}

// GetRoleOk returns a tuple with the Role field value
// and a boolean to check if the value has been set.
func (o *RoleAssignmentRemove) GetRoleOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Role, true
}

// SetRole sets field value
func (o *RoleAssignmentRemove) SetRole(v string) {
	o.Role = v
}

// GetTenant returns the Tenant field value
func (o *RoleAssignmentRemove) GetTenant() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Tenant
}

// GetTenantOk returns a tuple with the Tenant field value
// and a boolean to check if the value has been set.
func (o *RoleAssignmentRemove) GetTenantOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Tenant, true
}

// SetTenant sets field value
func (o *RoleAssignmentRemove) SetTenant(v string) {
	o.Tenant = v
}

// GetUser returns the User field value
func (o *RoleAssignmentRemove) GetUser() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.User
}

// GetUserOk returns a tuple with the User field value
// and a boolean to check if the value has been set.
func (o *RoleAssignmentRemove) GetUserOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.User, true
}

// SetUser sets field value
func (o *RoleAssignmentRemove) SetUser(v string) {
	o.User = v
}

func (o RoleAssignmentRemove) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if true {
		toSerialize["role"] = o.Role
	}
	if true {
		toSerialize["tenant"] = o.Tenant
	}
	if true {
		toSerialize["user"] = o.User
	}
	return json.Marshal(toSerialize)
}

type NullableRoleAssignmentRemove struct {
	value *RoleAssignmentRemove
	isSet bool
}

func (v NullableRoleAssignmentRemove) Get() *RoleAssignmentRemove {
	return v.value
}

func (v *NullableRoleAssignmentRemove) Set(val *RoleAssignmentRemove) {
	v.value = val
	v.isSet = true
}

func (v NullableRoleAssignmentRemove) IsSet() bool {
	return v.isSet
}

func (v *NullableRoleAssignmentRemove) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableRoleAssignmentRemove(val *RoleAssignmentRemove) *NullableRoleAssignmentRemove {
	return &NullableRoleAssignmentRemove{value: val, isSet: true}
}

func (v NullableRoleAssignmentRemove) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableRoleAssignmentRemove) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
