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

// PermissionLevelRoleRead struct for PermissionLevelRoleRead
type PermissionLevelRoleRead struct {
	// Unique id of the elements_env
	Id string `json:"id"`
	// A URL-friendly name of the elements_env (i.e: slug). You will be able to query later using this key instead of the id (UUID) of the elements_env.
	Key  string `json:"key"`
	Name string `json:"name"`
}

// NewPermissionLevelRoleRead instantiates a new PermissionLevelRoleRead object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewPermissionLevelRoleRead(id string, key string, name string) *PermissionLevelRoleRead {
	this := PermissionLevelRoleRead{}
	this.Id = id
	this.Key = key
	this.Name = name
	return &this
}

// NewPermissionLevelRoleReadWithDefaults instantiates a new PermissionLevelRoleRead object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewPermissionLevelRoleReadWithDefaults() *PermissionLevelRoleRead {
	this := PermissionLevelRoleRead{}
	return &this
}

// GetId returns the Id field value
func (o *PermissionLevelRoleRead) GetId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Id
}

// GetIdOk returns a tuple with the Id field value
// and a boolean to check if the value has been set.
func (o *PermissionLevelRoleRead) GetIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Id, true
}

// SetId sets field value
func (o *PermissionLevelRoleRead) SetId(v string) {
	o.Id = v
}

// GetKey returns the Key field value
func (o *PermissionLevelRoleRead) GetKey() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Key
}

// GetKeyOk returns a tuple with the Key field value
// and a boolean to check if the value has been set.
func (o *PermissionLevelRoleRead) GetKeyOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Key, true
}

// SetKey sets field value
func (o *PermissionLevelRoleRead) SetKey(v string) {
	o.Key = v
}

// GetName returns the Name field value
func (o *PermissionLevelRoleRead) GetName() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Name
}

// GetNameOk returns a tuple with the Name field value
// and a boolean to check if the value has been set.
func (o *PermissionLevelRoleRead) GetNameOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Name, true
}

// SetName sets field value
func (o *PermissionLevelRoleRead) SetName(v string) {
	o.Name = v
}

func (o PermissionLevelRoleRead) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if true {
		toSerialize["id"] = o.Id
	}
	if true {
		toSerialize["key"] = o.Key
	}
	if true {
		toSerialize["name"] = o.Name
	}
	return json.Marshal(toSerialize)
}

type NullablePermissionLevelRoleRead struct {
	value *PermissionLevelRoleRead
	isSet bool
}

func (v NullablePermissionLevelRoleRead) Get() *PermissionLevelRoleRead {
	return v.value
}

func (v *NullablePermissionLevelRoleRead) Set(val *PermissionLevelRoleRead) {
	v.value = val
	v.isSet = true
}

func (v NullablePermissionLevelRoleRead) IsSet() bool {
	return v.isSet
}

func (v *NullablePermissionLevelRoleRead) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullablePermissionLevelRoleRead(val *PermissionLevelRoleRead) *NullablePermissionLevelRoleRead {
	return &NullablePermissionLevelRoleRead{value: val, isSet: true}
}

func (v NullablePermissionLevelRoleRead) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullablePermissionLevelRoleRead) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
