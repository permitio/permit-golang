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

// Statistics struct for Statistics
type Statistics struct {
	Roles           int32 `json:"roles"`
	Users           int32 `json:"users"`
	Policies        int32 `json:"policies"`
	Resources       int32 `json:"resources"`
	Tenants         int32 `json:"tenants"`
	HasDecisionLogs bool  `json:"has_decision_logs"`
}

// NewStatistics instantiates a new Statistics object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewStatistics(roles int32, users int32, policies int32, resources int32, tenants int32, hasDecisionLogs bool) *Statistics {
	this := Statistics{}
	this.Roles = roles
	this.Users = users
	this.Policies = policies
	this.Resources = resources
	this.Tenants = tenants
	this.HasDecisionLogs = hasDecisionLogs
	return &this
}

// NewStatisticsWithDefaults instantiates a new Statistics object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewStatisticsWithDefaults() *Statistics {
	this := Statistics{}
	return &this
}

// GetRoles returns the Roles field value
func (o *Statistics) GetRoles() int32 {
	if o == nil {
		var ret int32
		return ret
	}

	return o.Roles
}

// GetRolesOk returns a tuple with the Roles field value
// and a boolean to check if the value has been set.
func (o *Statistics) GetRolesOk() (*int32, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Roles, true
}

// SetRoles sets field value
func (o *Statistics) SetRoles(v int32) {
	o.Roles = v
}

// GetUsers returns the Users field value
func (o *Statistics) GetUsers() int32 {
	if o == nil {
		var ret int32
		return ret
	}

	return o.Users
}

// GetUsersOk returns a tuple with the Users field value
// and a boolean to check if the value has been set.
func (o *Statistics) GetUsersOk() (*int32, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Users, true
}

// SetUsers sets field value
func (o *Statistics) SetUsers(v int32) {
	o.Users = v
}

// GetPolicies returns the Policies field value
func (o *Statistics) GetPolicies() int32 {
	if o == nil {
		var ret int32
		return ret
	}

	return o.Policies
}

// GetPoliciesOk returns a tuple with the Policies field value
// and a boolean to check if the value has been set.
func (o *Statistics) GetPoliciesOk() (*int32, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Policies, true
}

// SetPolicies sets field value
func (o *Statistics) SetPolicies(v int32) {
	o.Policies = v
}

// GetResources returns the Resources field value
func (o *Statistics) GetResources() int32 {
	if o == nil {
		var ret int32
		return ret
	}

	return o.Resources
}

// GetResourcesOk returns a tuple with the Resources field value
// and a boolean to check if the value has been set.
func (o *Statistics) GetResourcesOk() (*int32, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Resources, true
}

// SetResources sets field value
func (o *Statistics) SetResources(v int32) {
	o.Resources = v
}

// GetTenants returns the Tenants field value
func (o *Statistics) GetTenants() int32 {
	if o == nil {
		var ret int32
		return ret
	}

	return o.Tenants
}

// GetTenantsOk returns a tuple with the Tenants field value
// and a boolean to check if the value has been set.
func (o *Statistics) GetTenantsOk() (*int32, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Tenants, true
}

// SetTenants sets field value
func (o *Statistics) SetTenants(v int32) {
	o.Tenants = v
}

// GetHasDecisionLogs returns the HasDecisionLogs field value
func (o *Statistics) GetHasDecisionLogs() bool {
	if o == nil {
		var ret bool
		return ret
	}

	return o.HasDecisionLogs
}

// GetHasDecisionLogsOk returns a tuple with the HasDecisionLogs field value
// and a boolean to check if the value has been set.
func (o *Statistics) GetHasDecisionLogsOk() (*bool, bool) {
	if o == nil {
		return nil, false
	}
	return &o.HasDecisionLogs, true
}

// SetHasDecisionLogs sets field value
func (o *Statistics) SetHasDecisionLogs(v bool) {
	o.HasDecisionLogs = v
}

func (o Statistics) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if true {
		toSerialize["roles"] = o.Roles
	}
	if true {
		toSerialize["users"] = o.Users
	}
	if true {
		toSerialize["policies"] = o.Policies
	}
	if true {
		toSerialize["resources"] = o.Resources
	}
	if true {
		toSerialize["tenants"] = o.Tenants
	}
	if true {
		toSerialize["has_decision_logs"] = o.HasDecisionLogs
	}
	return json.Marshal(toSerialize)
}

type NullableStatistics struct {
	value *Statistics
	isSet bool
}

func (v NullableStatistics) Get() *Statistics {
	return v.value
}

func (v *NullableStatistics) Set(val *Statistics) {
	v.value = val
	v.isSet = true
}

func (v NullableStatistics) IsSet() bool {
	return v.isSet
}

func (v *NullableStatistics) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableStatistics(val *Statistics) *NullableStatistics {
	return &NullableStatistics{value: val, isSet: true}
}

func (v NullableStatistics) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableStatistics) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
