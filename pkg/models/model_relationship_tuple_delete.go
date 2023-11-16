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

// checks if the RelationshipTupleDelete type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &RelationshipTupleDelete{}

// RelationshipTupleDelete struct for RelationshipTupleDelete
type RelationshipTupleDelete struct {
	// the resource instance assigned the new relation (accepts either the resource instance id or resource_key:resource_instance_key)
	Subject string `json:"subject"`
	// the relation to assign between the subject and object
	Relation string `json:"relation"`
	// the resource instance on which the new relation is assigned (accepts either the resource instance id or resource_key:resource_instance_key)
	Object string `json:"object"`
}

// NewRelationshipTupleDelete instantiates a new RelationshipTupleDelete object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewRelationshipTupleDelete(subject string, relation string, object string) *RelationshipTupleDelete {
	this := RelationshipTupleDelete{}
	this.Subject = subject
	this.Relation = relation
	this.Object = object
	return &this
}

// NewRelationshipTupleDeleteWithDefaults instantiates a new RelationshipTupleDelete object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewRelationshipTupleDeleteWithDefaults() *RelationshipTupleDelete {
	this := RelationshipTupleDelete{}
	return &this
}

// GetSubject returns the Subject field value
func (o *RelationshipTupleDelete) GetSubject() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Subject
}

// GetSubjectOk returns a tuple with the Subject field value
// and a boolean to check if the value has been set.
func (o *RelationshipTupleDelete) GetSubjectOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Subject, true
}

// SetSubject sets field value
func (o *RelationshipTupleDelete) SetSubject(v string) {
	o.Subject = v
}

// GetRelation returns the Relation field value
func (o *RelationshipTupleDelete) GetRelation() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Relation
}

// GetRelationOk returns a tuple with the Relation field value
// and a boolean to check if the value has been set.
func (o *RelationshipTupleDelete) GetRelationOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Relation, true
}

// SetRelation sets field value
func (o *RelationshipTupleDelete) SetRelation(v string) {
	o.Relation = v
}

// GetObject returns the Object field value
func (o *RelationshipTupleDelete) GetObject() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Object
}

// GetObjectOk returns a tuple with the Object field value
// and a boolean to check if the value has been set.
func (o *RelationshipTupleDelete) GetObjectOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Object, true
}

// SetObject sets field value
func (o *RelationshipTupleDelete) SetObject(v string) {
	o.Object = v
}

func (o RelationshipTupleDelete) MarshalJSON() ([]byte, error) {
	toSerialize,err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o RelationshipTupleDelete) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	toSerialize["subject"] = o.Subject
	toSerialize["relation"] = o.Relation
	toSerialize["object"] = o.Object
	return toSerialize, nil
}

type NullableRelationshipTupleDelete struct {
	value *RelationshipTupleDelete
	isSet bool
}

func (v NullableRelationshipTupleDelete) Get() *RelationshipTupleDelete {
	return v.value
}

func (v *NullableRelationshipTupleDelete) Set(val *RelationshipTupleDelete) {
	v.value = val
	v.isSet = true
}

func (v NullableRelationshipTupleDelete) IsSet() bool {
	return v.isSet
}

func (v *NullableRelationshipTupleDelete) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableRelationshipTupleDelete(val *RelationshipTupleDelete) *NullableRelationshipTupleDelete {
	return &NullableRelationshipTupleDelete{value: val, isSet: true}
}

func (v NullableRelationshipTupleDelete) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableRelationshipTupleDelete) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
