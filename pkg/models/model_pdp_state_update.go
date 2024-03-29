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

// PDPStateUpdate struct for PDPStateUpdate
type PDPStateUpdate struct {
	PdpInstanceId string   `json:"pdp_instance_id"`
	State         PDPState `json:"state"`
}

// NewPDPStateUpdate instantiates a new PDPStateUpdate object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewPDPStateUpdate(pdpInstanceId string, state PDPState) *PDPStateUpdate {
	this := PDPStateUpdate{}
	this.PdpInstanceId = pdpInstanceId
	this.State = state
	return &this
}

// NewPDPStateUpdateWithDefaults instantiates a new PDPStateUpdate object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewPDPStateUpdateWithDefaults() *PDPStateUpdate {
	this := PDPStateUpdate{}
	return &this
}

// GetPdpInstanceId returns the PdpInstanceId field value
func (o *PDPStateUpdate) GetPdpInstanceId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.PdpInstanceId
}

// GetPdpInstanceIdOk returns a tuple with the PdpInstanceId field value
// and a boolean to check if the value has been set.
func (o *PDPStateUpdate) GetPdpInstanceIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.PdpInstanceId, true
}

// SetPdpInstanceId sets field value
func (o *PDPStateUpdate) SetPdpInstanceId(v string) {
	o.PdpInstanceId = v
}

// GetState returns the State field value
func (o *PDPStateUpdate) GetState() PDPState {
	if o == nil {
		var ret PDPState
		return ret
	}

	return o.State
}

// GetStateOk returns a tuple with the State field value
// and a boolean to check if the value has been set.
func (o *PDPStateUpdate) GetStateOk() (*PDPState, bool) {
	if o == nil {
		return nil, false
	}
	return &o.State, true
}

// SetState sets field value
func (o *PDPStateUpdate) SetState(v PDPState) {
	o.State = v
}

func (o PDPStateUpdate) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if true {
		toSerialize["pdp_instance_id"] = o.PdpInstanceId
	}
	if true {
		toSerialize["state"] = o.State
	}
	return json.Marshal(toSerialize)
}

type NullablePDPStateUpdate struct {
	value *PDPStateUpdate
	isSet bool
}

func (v NullablePDPStateUpdate) Get() *PDPStateUpdate {
	return v.value
}

func (v *NullablePDPStateUpdate) Set(val *PDPStateUpdate) {
	v.value = val
	v.isSet = true
}

func (v NullablePDPStateUpdate) IsSet() bool {
	return v.isSet
}

func (v *NullablePDPStateUpdate) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullablePDPStateUpdate(val *PDPStateUpdate) *NullablePDPStateUpdate {
	return &NullablePDPStateUpdate{value: val, isSet: true}
}

func (v NullablePDPStateUpdate) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullablePDPStateUpdate) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
