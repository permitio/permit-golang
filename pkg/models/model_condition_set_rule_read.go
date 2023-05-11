/*
Permit.io API

 Authorization as a service

API version: 2.0.0
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package models

import (
	"encoding/json"
	"time"
)

// ConditionSetRuleRead struct for ConditionSetRuleRead
type ConditionSetRuleRead struct {
	// Unique id of the condition set rule
	Id string `json:"id"`
	// A unique id by which Permit will identify this condition set rule.
	Key string `json:"key"`
	// the userset that is currently granted permissions, i.e: all the users matching this rule are granted the permission on the resourceset
	UserSet string `json:"user_set"`
	// a permission that is currently granted to the userset *on* the resourceset.
	Permission string `json:"permission"`
	// the resourceset that represents the resources that are currently granted for access, i.e: all the resources matching this rule can be accessed by the userset to perform the granted *permission*
	ResourceSet string `json:"resource_set"`
	// Unique id of the organization that the condition set rule belongs to.
	OrganizationId string `json:"organization_id"`
	// Unique id of the project that the condition set rule belongs to.
	ProjectId string `json:"project_id"`
	// Unique id of the environment that the condition set rule belongs to.
	EnvironmentId string `json:"environment_id"`
	// Date and time when the condition set rule was created (ISO_8601 format).
	CreatedAt time.Time `json:"created_at"`
	// Date and time when the condition set rule was last updated/modified (ISO_8601 format).
	UpdatedAt time.Time `json:"updated_at"`
}

// NewConditionSetRuleRead instantiates a new ConditionSetRuleRead object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewConditionSetRuleRead(id string, key string, userSet string, permission string, resourceSet string, organizationId string, projectId string, environmentId string, createdAt time.Time, updatedAt time.Time) *ConditionSetRuleRead {
	this := ConditionSetRuleRead{}
	this.Id = id
	this.Key = key
	this.UserSet = userSet
	this.Permission = permission
	this.ResourceSet = resourceSet
	this.OrganizationId = organizationId
	this.ProjectId = projectId
	this.EnvironmentId = environmentId
	this.CreatedAt = createdAt
	this.UpdatedAt = updatedAt
	return &this
}

// NewConditionSetRuleReadWithDefaults instantiates a new ConditionSetRuleRead object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewConditionSetRuleReadWithDefaults() *ConditionSetRuleRead {
	this := ConditionSetRuleRead{}
	return &this
}

// GetId returns the Id field value
func (o *ConditionSetRuleRead) GetId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Id
}

// GetIdOk returns a tuple with the Id field value
// and a boolean to check if the value has been set.
func (o *ConditionSetRuleRead) GetIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Id, true
}

// SetId sets field value
func (o *ConditionSetRuleRead) SetId(v string) {
	o.Id = v
}

// GetKey returns the Key field value
func (o *ConditionSetRuleRead) GetKey() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Key
}

// GetKeyOk returns a tuple with the Key field value
// and a boolean to check if the value has been set.
func (o *ConditionSetRuleRead) GetKeyOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Key, true
}

// SetKey sets field value
func (o *ConditionSetRuleRead) SetKey(v string) {
	o.Key = v
}

// GetUserSet returns the UserSet field value
func (o *ConditionSetRuleRead) GetUserSet() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.UserSet
}

// GetUserSetOk returns a tuple with the UserSet field value
// and a boolean to check if the value has been set.
func (o *ConditionSetRuleRead) GetUserSetOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.UserSet, true
}

// SetUserSet sets field value
func (o *ConditionSetRuleRead) SetUserSet(v string) {
	o.UserSet = v
}

// GetPermission returns the Permission field value
func (o *ConditionSetRuleRead) GetPermission() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Permission
}

// GetPermissionOk returns a tuple with the Permission field value
// and a boolean to check if the value has been set.
func (o *ConditionSetRuleRead) GetPermissionOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Permission, true
}

// SetPermission sets field value
func (o *ConditionSetRuleRead) SetPermission(v string) {
	o.Permission = v
}

// GetResourceSet returns the ResourceSet field value
func (o *ConditionSetRuleRead) GetResourceSet() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.ResourceSet
}

// GetResourceSetOk returns a tuple with the ResourceSet field value
// and a boolean to check if the value has been set.
func (o *ConditionSetRuleRead) GetResourceSetOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.ResourceSet, true
}

// SetResourceSet sets field value
func (o *ConditionSetRuleRead) SetResourceSet(v string) {
	o.ResourceSet = v
}

// GetOrganizationId returns the OrganizationId field value
func (o *ConditionSetRuleRead) GetOrganizationId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.OrganizationId
}

// GetOrganizationIdOk returns a tuple with the OrganizationId field value
// and a boolean to check if the value has been set.
func (o *ConditionSetRuleRead) GetOrganizationIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.OrganizationId, true
}

// SetOrganizationId sets field value
func (o *ConditionSetRuleRead) SetOrganizationId(v string) {
	o.OrganizationId = v
}

// GetProjectId returns the ProjectId field value
func (o *ConditionSetRuleRead) GetProjectId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.ProjectId
}

// GetProjectIdOk returns a tuple with the ProjectId field value
// and a boolean to check if the value has been set.
func (o *ConditionSetRuleRead) GetProjectIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.ProjectId, true
}

// SetProjectId sets field value
func (o *ConditionSetRuleRead) SetProjectId(v string) {
	o.ProjectId = v
}

// GetEnvironmentId returns the EnvironmentId field value
func (o *ConditionSetRuleRead) GetEnvironmentId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.EnvironmentId
}

// GetEnvironmentIdOk returns a tuple with the EnvironmentId field value
// and a boolean to check if the value has been set.
func (o *ConditionSetRuleRead) GetEnvironmentIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.EnvironmentId, true
}

// SetEnvironmentId sets field value
func (o *ConditionSetRuleRead) SetEnvironmentId(v string) {
	o.EnvironmentId = v
}

// GetCreatedAt returns the CreatedAt field value
func (o *ConditionSetRuleRead) GetCreatedAt() time.Time {
	if o == nil {
		var ret time.Time
		return ret
	}

	return o.CreatedAt
}

// GetCreatedAtOk returns a tuple with the CreatedAt field value
// and a boolean to check if the value has been set.
func (o *ConditionSetRuleRead) GetCreatedAtOk() (*time.Time, bool) {
	if o == nil {
		return nil, false
	}
	return &o.CreatedAt, true
}

// SetCreatedAt sets field value
func (o *ConditionSetRuleRead) SetCreatedAt(v time.Time) {
	o.CreatedAt = v
}

// GetUpdatedAt returns the UpdatedAt field value
func (o *ConditionSetRuleRead) GetUpdatedAt() time.Time {
	if o == nil {
		var ret time.Time
		return ret
	}

	return o.UpdatedAt
}

// GetUpdatedAtOk returns a tuple with the UpdatedAt field value
// and a boolean to check if the value has been set.
func (o *ConditionSetRuleRead) GetUpdatedAtOk() (*time.Time, bool) {
	if o == nil {
		return nil, false
	}
	return &o.UpdatedAt, true
}

// SetUpdatedAt sets field value
func (o *ConditionSetRuleRead) SetUpdatedAt(v time.Time) {
	o.UpdatedAt = v
}

func (o ConditionSetRuleRead) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if true {
		toSerialize["id"] = o.Id
	}
	if true {
		toSerialize["key"] = o.Key
	}
	if true {
		toSerialize["user_set"] = o.UserSet
	}
	if true {
		toSerialize["permission"] = o.Permission
	}
	if true {
		toSerialize["resource_set"] = o.ResourceSet
	}
	if true {
		toSerialize["organization_id"] = o.OrganizationId
	}
	if true {
		toSerialize["project_id"] = o.ProjectId
	}
	if true {
		toSerialize["environment_id"] = o.EnvironmentId
	}
	if true {
		toSerialize["created_at"] = o.CreatedAt
	}
	if true {
		toSerialize["updated_at"] = o.UpdatedAt
	}
	return json.Marshal(toSerialize)
}

type NullableConditionSetRuleRead struct {
	value *ConditionSetRuleRead
	isSet bool
}

func (v NullableConditionSetRuleRead) Get() *ConditionSetRuleRead {
	return v.value
}

func (v *NullableConditionSetRuleRead) Set(val *ConditionSetRuleRead) {
	v.value = val
	v.isSet = true
}

func (v NullableConditionSetRuleRead) IsSet() bool {
	return v.isSet
}

func (v *NullableConditionSetRuleRead) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableConditionSetRuleRead(val *ConditionSetRuleRead) *NullableConditionSetRuleRead {
	return &NullableConditionSetRuleRead{value: val, isSet: true}
}

func (v NullableConditionSetRuleRead) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableConditionSetRuleRead) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}