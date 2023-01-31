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

// ResourceReplace struct for ResourceReplace
type ResourceReplace struct {
	// The name of the resource
	Name string `json:"name"`
	// The [URN](https://en.wikipedia.org/wiki/Uniform_Resource_Name) (Uniform Resource Name) of the resource
	Urn *string `json:"urn,omitempty"`
	// An optional longer description of what this resource respresents in your system
	Description *string `json:"description,omitempty"`
	//          A actions definition block, typically contained within a resource type definition block.         The actions represents the ways you can interact with a protected resource.
	Actions map[string]ActionBlockEditable `json:"actions"`
	// Attributes that each resource of this type defines, and can be used in your ABAC policies.
	Attributes *map[string]AttributeBlockEditable `json:"attributes,omitempty"`
}

// NewResourceReplace instantiates a new ResourceReplace object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewResourceReplace(name string, actions map[string]ActionBlockEditable) *ResourceReplace {
	this := ResourceReplace{}
	this.Name = name
	this.Actions = actions
	return &this
}

// NewResourceReplaceWithDefaults instantiates a new ResourceReplace object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewResourceReplaceWithDefaults() *ResourceReplace {
	this := ResourceReplace{}
	return &this
}

// GetName returns the Name field value
func (o *ResourceReplace) GetName() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Name
}

// GetNameOk returns a tuple with the Name field value
// and a boolean to check if the value has been set.
func (o *ResourceReplace) GetNameOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Name, true
}

// SetName sets field value
func (o *ResourceReplace) SetName(v string) {
	o.Name = v
}

// GetUrn returns the Urn field value if set, zero value otherwise.
func (o *ResourceReplace) GetUrn() string {
	if o == nil || IsNil(o.Urn) {
		var ret string
		return ret
	}
	return *o.Urn
}

// GetUrnOk returns a tuple with the Urn field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ResourceReplace) GetUrnOk() (*string, bool) {
	if o == nil || IsNil(o.Urn) {
		return nil, false
	}
	return o.Urn, true
}

// HasUrn returns a boolean if a field has been set.
func (o *ResourceReplace) HasUrn() bool {
	if o != nil && !IsNil(o.Urn) {
		return true
	}

	return false
}

// SetUrn gets a reference to the given string and assigns it to the Urn field.
func (o *ResourceReplace) SetUrn(v string) {
	o.Urn = &v
}

// GetDescription returns the Description field value if set, zero value otherwise.
func (o *ResourceReplace) GetDescription() string {
	if o == nil || IsNil(o.Description) {
		var ret string
		return ret
	}
	return *o.Description
}

// GetDescriptionOk returns a tuple with the Description field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ResourceReplace) GetDescriptionOk() (*string, bool) {
	if o == nil || IsNil(o.Description) {
		return nil, false
	}
	return o.Description, true
}

// HasDescription returns a boolean if a field has been set.
func (o *ResourceReplace) HasDescription() bool {
	if o != nil && !IsNil(o.Description) {
		return true
	}

	return false
}

// SetDescription gets a reference to the given string and assigns it to the Description field.
func (o *ResourceReplace) SetDescription(v string) {
	o.Description = &v
}

// GetActions returns the Actions field value
func (o *ResourceReplace) GetActions() map[string]ActionBlockEditable {
	if o == nil {
		var ret map[string]ActionBlockEditable
		return ret
	}

	return o.Actions
}

// GetActionsOk returns a tuple with the Actions field value
// and a boolean to check if the value has been set.
func (o *ResourceReplace) GetActionsOk() (*map[string]ActionBlockEditable, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Actions, true
}

// SetActions sets field value
func (o *ResourceReplace) SetActions(v map[string]ActionBlockEditable) {
	o.Actions = v
}

// GetAttributes returns the Attributes field value if set, zero value otherwise.
func (o *ResourceReplace) GetAttributes() map[string]AttributeBlockEditable {
	if o == nil || IsNil(o.Attributes) {
		var ret map[string]AttributeBlockEditable
		return ret
	}
	return *o.Attributes
}

// GetAttributesOk returns a tuple with the Attributes field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ResourceReplace) GetAttributesOk() (*map[string]AttributeBlockEditable, bool) {
	if o == nil || IsNil(o.Attributes) {
		return nil, false
	}
	return o.Attributes, true
}

// HasAttributes returns a boolean if a field has been set.
func (o *ResourceReplace) HasAttributes() bool {
	if o != nil && !IsNil(o.Attributes) {
		return true
	}

	return false
}

// SetAttributes gets a reference to the given map[string]AttributeBlockEditable and assigns it to the Attributes field.
func (o *ResourceReplace) SetAttributes(v map[string]AttributeBlockEditable) {
	o.Attributes = &v
}

func (o ResourceReplace) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if true {
		toSerialize["name"] = o.Name
	}
	if !IsNil(o.Urn) {
		toSerialize["urn"] = o.Urn
	}
	if !IsNil(o.Description) {
		toSerialize["description"] = o.Description
	}
	if true {
		toSerialize["actions"] = o.Actions
	}
	if !IsNil(o.Attributes) {
		toSerialize["attributes"] = o.Attributes
	}
	return json.Marshal(toSerialize)
}

type NullableResourceReplace struct {
	value *ResourceReplace
	isSet bool
}

func (v NullableResourceReplace) Get() *ResourceReplace {
	return v.value
}

func (v *NullableResourceReplace) Set(val *ResourceReplace) {
	v.value = val
	v.isSet = true
}

func (v NullableResourceReplace) IsSet() bool {
	return v.isSet
}

func (v *NullableResourceReplace) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableResourceReplace(val *ResourceReplace) *NullableResourceReplace {
	return &NullableResourceReplace{value: val, isSet: true}
}

func (v NullableResourceReplace) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableResourceReplace) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
