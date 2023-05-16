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

// checks if the PaginatedResultConditionSetRead type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &PaginatedResultConditionSetRead{}

// PaginatedResultConditionSetRead struct for PaginatedResultConditionSetRead
type PaginatedResultConditionSetRead struct {
	// List of Condition Sets
	Data       []ConditionSetRead `json:"data"`
	TotalCount int32              `json:"total_count"`
	PageCount  *int32             `json:"page_count,omitempty"`
}

// NewPaginatedResultConditionSetRead instantiates a new PaginatedResultConditionSetRead object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewPaginatedResultConditionSetRead(data []ConditionSetRead, totalCount int32) *PaginatedResultConditionSetRead {
	this := PaginatedResultConditionSetRead{}
	this.Data = data
	this.TotalCount = totalCount
	var pageCount int32 = 0
	this.PageCount = &pageCount
	return &this
}

// NewPaginatedResultConditionSetReadWithDefaults instantiates a new PaginatedResultConditionSetRead object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewPaginatedResultConditionSetReadWithDefaults() *PaginatedResultConditionSetRead {
	this := PaginatedResultConditionSetRead{}
	var pageCount int32 = 0
	this.PageCount = &pageCount
	return &this
}

// GetData returns the Data field value
func (o *PaginatedResultConditionSetRead) GetData() []ConditionSetRead {
	if o == nil {
		var ret []ConditionSetRead
		return ret
	}

	return o.Data
}

// GetDataOk returns a tuple with the Data field value
// and a boolean to check if the value has been set.
func (o *PaginatedResultConditionSetRead) GetDataOk() ([]ConditionSetRead, bool) {
	if o == nil {
		return nil, false
	}
	return o.Data, true
}

// SetData sets field value
func (o *PaginatedResultConditionSetRead) SetData(v []ConditionSetRead) {
	o.Data = v
}

// GetTotalCount returns the TotalCount field value
func (o *PaginatedResultConditionSetRead) GetTotalCount() int32 {
	if o == nil {
		var ret int32
		return ret
	}

	return o.TotalCount
}

// GetTotalCountOk returns a tuple with the TotalCount field value
// and a boolean to check if the value has been set.
func (o *PaginatedResultConditionSetRead) GetTotalCountOk() (*int32, bool) {
	if o == nil {
		return nil, false
	}
	return &o.TotalCount, true
}

// SetTotalCount sets field value
func (o *PaginatedResultConditionSetRead) SetTotalCount(v int32) {
	o.TotalCount = v
}

// GetPageCount returns the PageCount field value if set, zero value otherwise.
func (o *PaginatedResultConditionSetRead) GetPageCount() int32 {
	if o == nil || IsNil(o.PageCount) {
		var ret int32
		return ret
	}
	return *o.PageCount
}

// GetPageCountOk returns a tuple with the PageCount field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *PaginatedResultConditionSetRead) GetPageCountOk() (*int32, bool) {
	if o == nil || IsNil(o.PageCount) {
		return nil, false
	}
	return o.PageCount, true
}

// HasPageCount returns a boolean if a field has been set.
func (o *PaginatedResultConditionSetRead) HasPageCount() bool {
	if o != nil && !IsNil(o.PageCount) {
		return true
	}

	return false
}

// SetPageCount gets a reference to the given int32 and assigns it to the PageCount field.
func (o *PaginatedResultConditionSetRead) SetPageCount(v int32) {
	o.PageCount = &v
}

func (o PaginatedResultConditionSetRead) MarshalJSON() ([]byte, error) {
	toSerialize, err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o PaginatedResultConditionSetRead) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	toSerialize["data"] = o.Data
	toSerialize["total_count"] = o.TotalCount
	if !IsNil(o.PageCount) {
		toSerialize["page_count"] = o.PageCount
	}
	return toSerialize, nil
}

type NullablePaginatedResultConditionSetRead struct {
	value *PaginatedResultConditionSetRead
	isSet bool
}

func (v NullablePaginatedResultConditionSetRead) Get() *PaginatedResultConditionSetRead {
	return v.value
}

func (v *NullablePaginatedResultConditionSetRead) Set(val *PaginatedResultConditionSetRead) {
	v.value = val
	v.isSet = true
}

func (v NullablePaginatedResultConditionSetRead) IsSet() bool {
	return v.isSet
}

func (v *NullablePaginatedResultConditionSetRead) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullablePaginatedResultConditionSetRead(val *PaginatedResultConditionSetRead) *NullablePaginatedResultConditionSetRead {
	return &NullablePaginatedResultConditionSetRead{value: val, isSet: true}
}

func (v NullablePaginatedResultConditionSetRead) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullablePaginatedResultConditionSetRead) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
