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

// OrganizationCreate struct for OrganizationCreate
type OrganizationCreate struct {
	// A URL-friendly name of the organization (i.e: slug). You will be able to query later using this key instead of the id (UUID) of the organization.
	Key string `json:"key"`
	// The name of the organization, usually it's your company's name.
	Name string `json:"name"`
	// the settings for this project
	Settings map[string]interface{} `json:"settings,omitempty"`
}

// NewOrganizationCreate instantiates a new OrganizationCreate object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewOrganizationCreate(key string, name string) *OrganizationCreate {
	this := OrganizationCreate{}
	this.Key = key
	this.Name = name
	return &this
}

// NewOrganizationCreateWithDefaults instantiates a new OrganizationCreate object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewOrganizationCreateWithDefaults() *OrganizationCreate {
	this := OrganizationCreate{}
	return &this
}

// GetKey returns the Key field value
func (o *OrganizationCreate) GetKey() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Key
}

// GetKeyOk returns a tuple with the Key field value
// and a boolean to check if the value has been set.
func (o *OrganizationCreate) GetKeyOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Key, true
}

// SetKey sets field value
func (o *OrganizationCreate) SetKey(v string) {
	o.Key = v
}

// GetName returns the Name field value
func (o *OrganizationCreate) GetName() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Name
}

// GetNameOk returns a tuple with the Name field value
// and a boolean to check if the value has been set.
func (o *OrganizationCreate) GetNameOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Name, true
}

// SetName sets field value
func (o *OrganizationCreate) SetName(v string) {
	o.Name = v
}

// GetSettings returns the Settings field value if set, zero value otherwise.
func (o *OrganizationCreate) GetSettings() map[string]interface{} {
	if o == nil || IsNil(o.Settings) {
		var ret map[string]interface{}
		return ret
	}
	return o.Settings
}

// GetSettingsOk returns a tuple with the Settings field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *OrganizationCreate) GetSettingsOk() (map[string]interface{}, bool) {
	if o == nil || IsNil(o.Settings) {
		return map[string]interface{}{}, false
	}
	return o.Settings, true
}

// HasSettings returns a boolean if a field has been set.
func (o *OrganizationCreate) HasSettings() bool {
	if o != nil && !IsNil(o.Settings) {
		return true
	}

	return false
}

// SetSettings gets a reference to the given map[string]interface{} and assigns it to the Settings field.
func (o *OrganizationCreate) SetSettings(v map[string]interface{}) {
	o.Settings = v
}

func (o OrganizationCreate) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if true {
		toSerialize["key"] = o.Key
	}
	if true {
		toSerialize["name"] = o.Name
	}
	if !IsNil(o.Settings) {
		toSerialize["settings"] = o.Settings
	}
	return json.Marshal(toSerialize)
}

type NullableOrganizationCreate struct {
	value *OrganizationCreate
	isSet bool
}

func (v NullableOrganizationCreate) Get() *OrganizationCreate {
	return v.value
}

func (v *NullableOrganizationCreate) Set(val *OrganizationCreate) {
	v.value = val
	v.isSet = true
}

func (v NullableOrganizationCreate) IsSet() bool {
	return v.isSet
}

func (v *NullableOrganizationCreate) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableOrganizationCreate(val *OrganizationCreate) *NullableOrganizationCreate {
	return &NullableOrganizationCreate{value: val, isSet: true}
}

func (v NullableOrganizationCreate) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableOrganizationCreate) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
