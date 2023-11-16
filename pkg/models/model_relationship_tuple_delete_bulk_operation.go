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

// checks if the RelationshipTupleDeleteBulkOperation type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &RelationshipTupleDeleteBulkOperation{}

// RelationshipTupleDeleteBulkOperation struct for RelationshipTupleDeleteBulkOperation
type RelationshipTupleDeleteBulkOperation struct {
	Idents []RelationshipTupleDelete `json:"idents"`
}

// NewRelationshipTupleDeleteBulkOperation instantiates a new RelationshipTupleDeleteBulkOperation object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewRelationshipTupleDeleteBulkOperation(idents []RelationshipTupleDelete) *RelationshipTupleDeleteBulkOperation {
	this := RelationshipTupleDeleteBulkOperation{}
	this.Idents = idents
	return &this
}

// NewRelationshipTupleDeleteBulkOperationWithDefaults instantiates a new RelationshipTupleDeleteBulkOperation object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewRelationshipTupleDeleteBulkOperationWithDefaults() *RelationshipTupleDeleteBulkOperation {
	this := RelationshipTupleDeleteBulkOperation{}
	return &this
}

// GetIdents returns the Idents field value
func (o *RelationshipTupleDeleteBulkOperation) GetIdents() []RelationshipTupleDelete {
	if o == nil {
		var ret []RelationshipTupleDelete
		return ret
	}

	return o.Idents
}

// GetIdentsOk returns a tuple with the Idents field value
// and a boolean to check if the value has been set.
func (o *RelationshipTupleDeleteBulkOperation) GetIdentsOk() ([]RelationshipTupleDelete, bool) {
	if o == nil {
		return nil, false
	}
	return o.Idents, true
}

// SetIdents sets field value
func (o *RelationshipTupleDeleteBulkOperation) SetIdents(v []RelationshipTupleDelete) {
	o.Idents = v
}

func (o RelationshipTupleDeleteBulkOperation) MarshalJSON() ([]byte, error) {
	toSerialize,err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o RelationshipTupleDeleteBulkOperation) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	toSerialize["idents"] = o.Idents
	return toSerialize, nil
}

type NullableRelationshipTupleDeleteBulkOperation struct {
	value *RelationshipTupleDeleteBulkOperation
	isSet bool
}

func (v NullableRelationshipTupleDeleteBulkOperation) Get() *RelationshipTupleDeleteBulkOperation {
	return v.value
}

func (v *NullableRelationshipTupleDeleteBulkOperation) Set(val *RelationshipTupleDeleteBulkOperation) {
	v.value = val
	v.isSet = true
}

func (v NullableRelationshipTupleDeleteBulkOperation) IsSet() bool {
	return v.isSet
}

func (v *NullableRelationshipTupleDeleteBulkOperation) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableRelationshipTupleDeleteBulkOperation(val *RelationshipTupleDeleteBulkOperation) *NullableRelationshipTupleDeleteBulkOperation {
	return &NullableRelationshipTupleDeleteBulkOperation{value: val, isSet: true}
}

func (v NullableRelationshipTupleDeleteBulkOperation) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableRelationshipTupleDeleteBulkOperation) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
