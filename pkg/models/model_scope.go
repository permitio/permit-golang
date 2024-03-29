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

// checks if the Scope type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &Scope{}

// Scope Filters to include and exclude copied objects
type Scope struct {
	Resources    *Resources    `json:"resources,omitempty"`
	Roles        *Roles        `json:"roles,omitempty"`
	UserSets     *UserSets     `json:"user_sets,omitempty"`
	ResourceSets *ResourceSets `json:"resource_sets,omitempty"`
}

// NewScope instantiates a new Scope object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewScope() *Scope {
	this := Scope{}
	return &this
}

// NewScopeWithDefaults instantiates a new Scope object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewScopeWithDefaults() *Scope {
	this := Scope{}
	return &this
}

// GetResources returns the Resources field value if set, zero value otherwise.
func (o *Scope) GetResources() Resources {
	if o == nil || IsNil(o.Resources) {
		var ret Resources
		return ret
	}
	return *o.Resources
}

// GetResourcesOk returns a tuple with the Resources field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *Scope) GetResourcesOk() (*Resources, bool) {
	if o == nil || IsNil(o.Resources) {
		return nil, false
	}
	return o.Resources, true
}

// HasResources returns a boolean if a field has been set.
func (o *Scope) HasResources() bool {
	if o != nil && !IsNil(o.Resources) {
		return true
	}

	return false
}

// SetResources gets a reference to the given Resources and assigns it to the Resources field.
func (o *Scope) SetResources(v Resources) {
	o.Resources = &v
}

// GetRoles returns the Roles field value if set, zero value otherwise.
func (o *Scope) GetRoles() Roles {
	if o == nil || IsNil(o.Roles) {
		var ret Roles
		return ret
	}
	return *o.Roles
}

// GetRolesOk returns a tuple with the Roles field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *Scope) GetRolesOk() (*Roles, bool) {
	if o == nil || IsNil(o.Roles) {
		return nil, false
	}
	return o.Roles, true
}

// HasRoles returns a boolean if a field has been set.
func (o *Scope) HasRoles() bool {
	if o != nil && !IsNil(o.Roles) {
		return true
	}

	return false
}

// SetRoles gets a reference to the given Roles and assigns it to the Roles field.
func (o *Scope) SetRoles(v Roles) {
	o.Roles = &v
}

// GetUserSets returns the UserSets field value if set, zero value otherwise.
func (o *Scope) GetUserSets() UserSets {
	if o == nil || IsNil(o.UserSets) {
		var ret UserSets
		return ret
	}
	return *o.UserSets
}

// GetUserSetsOk returns a tuple with the UserSets field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *Scope) GetUserSetsOk() (*UserSets, bool) {
	if o == nil || IsNil(o.UserSets) {
		return nil, false
	}
	return o.UserSets, true
}

// HasUserSets returns a boolean if a field has been set.
func (o *Scope) HasUserSets() bool {
	if o != nil && !IsNil(o.UserSets) {
		return true
	}

	return false
}

// SetUserSets gets a reference to the given UserSets and assigns it to the UserSets field.
func (o *Scope) SetUserSets(v UserSets) {
	o.UserSets = &v
}

// GetResourceSets returns the ResourceSets field value if set, zero value otherwise.
func (o *Scope) GetResourceSets() ResourceSets {
	if o == nil || IsNil(o.ResourceSets) {
		var ret ResourceSets
		return ret
	}
	return *o.ResourceSets
}

// GetResourceSetsOk returns a tuple with the ResourceSets field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *Scope) GetResourceSetsOk() (*ResourceSets, bool) {
	if o == nil || IsNil(o.ResourceSets) {
		return nil, false
	}
	return o.ResourceSets, true
}

// HasResourceSets returns a boolean if a field has been set.
func (o *Scope) HasResourceSets() bool {
	if o != nil && !IsNil(o.ResourceSets) {
		return true
	}

	return false
}

// SetResourceSets gets a reference to the given ResourceSets and assigns it to the ResourceSets field.
func (o *Scope) SetResourceSets(v ResourceSets) {
	o.ResourceSets = &v
}

func (o Scope) MarshalJSON() ([]byte, error) {
	toSerialize, err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o Scope) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	if !IsNil(o.Resources) {
		toSerialize["resources"] = o.Resources
	}
	if !IsNil(o.Roles) {
		toSerialize["roles"] = o.Roles
	}
	if !IsNil(o.UserSets) {
		toSerialize["user_sets"] = o.UserSets
	}
	if !IsNil(o.ResourceSets) {
		toSerialize["resource_sets"] = o.ResourceSets
	}
	return toSerialize, nil
}

type NullableScope struct {
	value *Scope
	isSet bool
}

func (v NullableScope) Get() *Scope {
	return v.value
}

func (v *NullableScope) Set(val *Scope) {
	v.value = val
	v.isSet = true
}

func (v NullableScope) IsSet() bool {
	return v.isSet
}

func (v *NullableScope) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableScope(val *Scope) *NullableScope {
	return &NullableScope{value: val, isSet: true}
}

func (v NullableScope) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableScope) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
