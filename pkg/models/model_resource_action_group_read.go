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

// checks if the ResourceActionGroupRead type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &ResourceActionGroupRead{}

// ResourceActionGroupRead struct for ResourceActionGroupRead
type ResourceActionGroupRead struct {
	// The name of the action group
	Name string `json:"name"`
	// An optional longer description of what this action group represents in your system
	Description *string `json:"description,omitempty"`
	// optional dictionary of key-value pairs that can be used to store arbitrary metadata about this action group. This metadata can be used to filter action groups using query parameters with attr_ prefix
	Attributes map[string]interface{} `json:"attributes,omitempty"`
	Actions    []string               `json:"actions,omitempty"`
	// A URL-friendly name of the action group (i.e: slug). You will be able to query later using this key instead of the id (UUID) of the action group.
	Key string `json:"key"`
	// Unique id of the action group
	Id string `json:"id"`
	// Unique id of the organization that the action group belongs to.
	OrganizationId string `json:"organization_id"`
	// Unique id of the project that the action group belongs to.
	ProjectId string `json:"project_id"`
	// Unique id of the environment that the action group belongs to.
	EnvironmentId string `json:"environment_id"`
	// Unique id of the resource that the action group belongs to.
	ResourceId string `json:"resource_id"`
	// Date and time when the action group was created (ISO_8601 format).
	CreatedAt time.Time `json:"created_at"`
	// Date and time when the action group was last updated/modified (ISO_8601 format).
	UpdatedAt time.Time `json:"updated_at"`
}

// NewResourceActionGroupRead instantiates a new ResourceActionGroupRead object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewResourceActionGroupRead(name string, key string, id string, organizationId string, projectId string, environmentId string, resourceId string, createdAt time.Time, updatedAt time.Time) *ResourceActionGroupRead {
	this := ResourceActionGroupRead{}
	this.Name = name
	this.Key = key
	this.Id = id
	this.OrganizationId = organizationId
	this.ProjectId = projectId
	this.EnvironmentId = environmentId
	this.ResourceId = resourceId
	this.CreatedAt = createdAt
	this.UpdatedAt = updatedAt
	return &this
}

// NewResourceActionGroupReadWithDefaults instantiates a new ResourceActionGroupRead object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewResourceActionGroupReadWithDefaults() *ResourceActionGroupRead {
	this := ResourceActionGroupRead{}
	return &this
}

// GetName returns the Name field value
func (o *ResourceActionGroupRead) GetName() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Name
}

// GetNameOk returns a tuple with the Name field value
// and a boolean to check if the value has been set.
func (o *ResourceActionGroupRead) GetNameOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Name, true
}

// SetName sets field value
func (o *ResourceActionGroupRead) SetName(v string) {
	o.Name = v
}

// GetDescription returns the Description field value if set, zero value otherwise.
func (o *ResourceActionGroupRead) GetDescription() string {
	if o == nil || IsNil(o.Description) {
		var ret string
		return ret
	}
	return *o.Description
}

// GetDescriptionOk returns a tuple with the Description field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ResourceActionGroupRead) GetDescriptionOk() (*string, bool) {
	if o == nil || IsNil(o.Description) {
		return nil, false
	}
	return o.Description, true
}

// HasDescription returns a boolean if a field has been set.
func (o *ResourceActionGroupRead) HasDescription() bool {
	if o != nil && !IsNil(o.Description) {
		return true
	}

	return false
}

// SetDescription gets a reference to the given string and assigns it to the Description field.
func (o *ResourceActionGroupRead) SetDescription(v string) {
	o.Description = &v
}

// GetAttributes returns the Attributes field value if set, zero value otherwise.
func (o *ResourceActionGroupRead) GetAttributes() map[string]interface{} {
	if o == nil || IsNil(o.Attributes) {
		var ret map[string]interface{}
		return ret
	}
	return o.Attributes
}

// GetAttributesOk returns a tuple with the Attributes field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ResourceActionGroupRead) GetAttributesOk() (map[string]interface{}, bool) {
	if o == nil || IsNil(o.Attributes) {
		return map[string]interface{}{}, false
	}
	return o.Attributes, true
}

// HasAttributes returns a boolean if a field has been set.
func (o *ResourceActionGroupRead) HasAttributes() bool {
	if o != nil && !IsNil(o.Attributes) {
		return true
	}

	return false
}

// SetAttributes gets a reference to the given map[string]interface{} and assigns it to the Attributes field.
func (o *ResourceActionGroupRead) SetAttributes(v map[string]interface{}) {
	o.Attributes = v
}

// GetActions returns the Actions field value if set, zero value otherwise.
func (o *ResourceActionGroupRead) GetActions() []string {
	if o == nil || IsNil(o.Actions) {
		var ret []string
		return ret
	}
	return o.Actions
}

// GetActionsOk returns a tuple with the Actions field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ResourceActionGroupRead) GetActionsOk() ([]string, bool) {
	if o == nil || IsNil(o.Actions) {
		return nil, false
	}
	return o.Actions, true
}

// HasActions returns a boolean if a field has been set.
func (o *ResourceActionGroupRead) HasActions() bool {
	if o != nil && !IsNil(o.Actions) {
		return true
	}

	return false
}

// SetActions gets a reference to the given []string and assigns it to the Actions field.
func (o *ResourceActionGroupRead) SetActions(v []string) {
	o.Actions = v
}

// GetKey returns the Key field value
func (o *ResourceActionGroupRead) GetKey() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Key
}

// GetKeyOk returns a tuple with the Key field value
// and a boolean to check if the value has been set.
func (o *ResourceActionGroupRead) GetKeyOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Key, true
}

// SetKey sets field value
func (o *ResourceActionGroupRead) SetKey(v string) {
	o.Key = v
}

// GetId returns the Id field value
func (o *ResourceActionGroupRead) GetId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Id
}

// GetIdOk returns a tuple with the Id field value
// and a boolean to check if the value has been set.
func (o *ResourceActionGroupRead) GetIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Id, true
}

// SetId sets field value
func (o *ResourceActionGroupRead) SetId(v string) {
	o.Id = v
}

// GetOrganizationId returns the OrganizationId field value
func (o *ResourceActionGroupRead) GetOrganizationId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.OrganizationId
}

// GetOrganizationIdOk returns a tuple with the OrganizationId field value
// and a boolean to check if the value has been set.
func (o *ResourceActionGroupRead) GetOrganizationIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.OrganizationId, true
}

// SetOrganizationId sets field value
func (o *ResourceActionGroupRead) SetOrganizationId(v string) {
	o.OrganizationId = v
}

// GetProjectId returns the ProjectId field value
func (o *ResourceActionGroupRead) GetProjectId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.ProjectId
}

// GetProjectIdOk returns a tuple with the ProjectId field value
// and a boolean to check if the value has been set.
func (o *ResourceActionGroupRead) GetProjectIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.ProjectId, true
}

// SetProjectId sets field value
func (o *ResourceActionGroupRead) SetProjectId(v string) {
	o.ProjectId = v
}

// GetEnvironmentId returns the EnvironmentId field value
func (o *ResourceActionGroupRead) GetEnvironmentId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.EnvironmentId
}

// GetEnvironmentIdOk returns a tuple with the EnvironmentId field value
// and a boolean to check if the value has been set.
func (o *ResourceActionGroupRead) GetEnvironmentIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.EnvironmentId, true
}

// SetEnvironmentId sets field value
func (o *ResourceActionGroupRead) SetEnvironmentId(v string) {
	o.EnvironmentId = v
}

// GetResourceId returns the ResourceId field value
func (o *ResourceActionGroupRead) GetResourceId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.ResourceId
}

// GetResourceIdOk returns a tuple with the ResourceId field value
// and a boolean to check if the value has been set.
func (o *ResourceActionGroupRead) GetResourceIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.ResourceId, true
}

// SetResourceId sets field value
func (o *ResourceActionGroupRead) SetResourceId(v string) {
	o.ResourceId = v
}

// GetCreatedAt returns the CreatedAt field value
func (o *ResourceActionGroupRead) GetCreatedAt() time.Time {
	if o == nil {
		var ret time.Time
		return ret
	}

	return o.CreatedAt
}

// GetCreatedAtOk returns a tuple with the CreatedAt field value
// and a boolean to check if the value has been set.
func (o *ResourceActionGroupRead) GetCreatedAtOk() (*time.Time, bool) {
	if o == nil {
		return nil, false
	}
	return &o.CreatedAt, true
}

// SetCreatedAt sets field value
func (o *ResourceActionGroupRead) SetCreatedAt(v time.Time) {
	o.CreatedAt = v
}

// GetUpdatedAt returns the UpdatedAt field value
func (o *ResourceActionGroupRead) GetUpdatedAt() time.Time {
	if o == nil {
		var ret time.Time
		return ret
	}

	return o.UpdatedAt
}

// GetUpdatedAtOk returns a tuple with the UpdatedAt field value
// and a boolean to check if the value has been set.
func (o *ResourceActionGroupRead) GetUpdatedAtOk() (*time.Time, bool) {
	if o == nil {
		return nil, false
	}
	return &o.UpdatedAt, true
}

// SetUpdatedAt sets field value
func (o *ResourceActionGroupRead) SetUpdatedAt(v time.Time) {
	o.UpdatedAt = v
}

func (o ResourceActionGroupRead) MarshalJSON() ([]byte, error) {
	toSerialize, err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o ResourceActionGroupRead) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	toSerialize["name"] = o.Name
	if !IsNil(o.Description) {
		toSerialize["description"] = o.Description
	}
	if !IsNil(o.Attributes) {
		toSerialize["attributes"] = o.Attributes
	}
	if !IsNil(o.Actions) {
		toSerialize["actions"] = o.Actions
	}
	toSerialize["key"] = o.Key
	toSerialize["id"] = o.Id
	toSerialize["organization_id"] = o.OrganizationId
	toSerialize["project_id"] = o.ProjectId
	toSerialize["environment_id"] = o.EnvironmentId
	toSerialize["resource_id"] = o.ResourceId
	toSerialize["created_at"] = o.CreatedAt
	toSerialize["updated_at"] = o.UpdatedAt
	return toSerialize, nil
}

type NullableResourceActionGroupRead struct {
	value *ResourceActionGroupRead
	isSet bool
}

func (v NullableResourceActionGroupRead) Get() *ResourceActionGroupRead {
	return v.value
}

func (v *NullableResourceActionGroupRead) Set(val *ResourceActionGroupRead) {
	v.value = val
	v.isSet = true
}

func (v NullableResourceActionGroupRead) IsSet() bool {
	return v.isSet
}

func (v *NullableResourceActionGroupRead) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableResourceActionGroupRead(val *ResourceActionGroupRead) *NullableResourceActionGroupRead {
	return &NullableResourceActionGroupRead{value: val, isSet: true}
}

func (v NullableResourceActionGroupRead) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableResourceActionGroupRead) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
