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

// checks if the ProxyConfigRead type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &ProxyConfigRead{}

// ProxyConfigRead struct for ProxyConfigRead
type ProxyConfigRead struct {
	// Proxy Config is set to enable the Permit Proxy to make proxied requests as part of the Frontend AuthZ.
	Key string `json:"key"`
	// Unique id of the proxy config
	Id string `json:"id"`
	// Unique id of the organization that the proxy config belongs to.
	OrganizationId string `json:"organization_id"`
	// Unique id of the project that the proxy config belongs to.
	ProjectId string `json:"project_id"`
	// Unique id of the environment that the proxy config belongs to.
	EnvironmentId string `json:"environment_id"`
	// Date and time when the proxy config was created (ISO_8601 format).
	CreatedAt time.Time `json:"created_at"`
	// Date and time when the proxy config was last updated/modified (ISO_8601 format).
	UpdatedAt time.Time `json:"updated_at"`
	Secret    string    `json:"secret"`
	// The name of the proxy config, for example: 'Stripe API'
	Name string `json:"name"`
	// Proxy config mapping rules will include the rules that will be used to map the request to the backend service by a URL and a http method.
	MappingRules  []MappingRule  `json:"mapping_rules,omitempty"`
	AuthMechanism *AuthMechanism `json:"auth_mechanism,omitempty"`
}

// NewProxyConfigRead instantiates a new ProxyConfigRead object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewProxyConfigRead(key string, id string, organizationId string, projectId string, environmentId string, createdAt time.Time, updatedAt time.Time, secret string, name string) *ProxyConfigRead {
	this := ProxyConfigRead{}
	this.Key = key
	this.Id = id
	this.OrganizationId = organizationId
	this.ProjectId = projectId
	this.EnvironmentId = environmentId
	this.CreatedAt = createdAt
	this.UpdatedAt = updatedAt
	this.Secret = secret
	this.Name = name
	var authMechanism AuthMechanism = BEARER
	this.AuthMechanism = &authMechanism
	return &this
}

// NewProxyConfigReadWithDefaults instantiates a new ProxyConfigRead object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewProxyConfigReadWithDefaults() *ProxyConfigRead {
	this := ProxyConfigRead{}
	var authMechanism AuthMechanism = BEARER
	this.AuthMechanism = &authMechanism
	return &this
}

// GetKey returns the Key field value
func (o *ProxyConfigRead) GetKey() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Key
}

// GetKeyOk returns a tuple with the Key field value
// and a boolean to check if the value has been set.
func (o *ProxyConfigRead) GetKeyOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Key, true
}

// SetKey sets field value
func (o *ProxyConfigRead) SetKey(v string) {
	o.Key = v
}

// GetId returns the Id field value
func (o *ProxyConfigRead) GetId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Id
}

// GetIdOk returns a tuple with the Id field value
// and a boolean to check if the value has been set.
func (o *ProxyConfigRead) GetIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Id, true
}

// SetId sets field value
func (o *ProxyConfigRead) SetId(v string) {
	o.Id = v
}

// GetOrganizationId returns the OrganizationId field value
func (o *ProxyConfigRead) GetOrganizationId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.OrganizationId
}

// GetOrganizationIdOk returns a tuple with the OrganizationId field value
// and a boolean to check if the value has been set.
func (o *ProxyConfigRead) GetOrganizationIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.OrganizationId, true
}

// SetOrganizationId sets field value
func (o *ProxyConfigRead) SetOrganizationId(v string) {
	o.OrganizationId = v
}

// GetProjectId returns the ProjectId field value
func (o *ProxyConfigRead) GetProjectId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.ProjectId
}

// GetProjectIdOk returns a tuple with the ProjectId field value
// and a boolean to check if the value has been set.
func (o *ProxyConfigRead) GetProjectIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.ProjectId, true
}

// SetProjectId sets field value
func (o *ProxyConfigRead) SetProjectId(v string) {
	o.ProjectId = v
}

// GetEnvironmentId returns the EnvironmentId field value
func (o *ProxyConfigRead) GetEnvironmentId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.EnvironmentId
}

// GetEnvironmentIdOk returns a tuple with the EnvironmentId field value
// and a boolean to check if the value has been set.
func (o *ProxyConfigRead) GetEnvironmentIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.EnvironmentId, true
}

// SetEnvironmentId sets field value
func (o *ProxyConfigRead) SetEnvironmentId(v string) {
	o.EnvironmentId = v
}

// GetCreatedAt returns the CreatedAt field value
func (o *ProxyConfigRead) GetCreatedAt() time.Time {
	if o == nil {
		var ret time.Time
		return ret
	}

	return o.CreatedAt
}

// GetCreatedAtOk returns a tuple with the CreatedAt field value
// and a boolean to check if the value has been set.
func (o *ProxyConfigRead) GetCreatedAtOk() (*time.Time, bool) {
	if o == nil {
		return nil, false
	}
	return &o.CreatedAt, true
}

// SetCreatedAt sets field value
func (o *ProxyConfigRead) SetCreatedAt(v time.Time) {
	o.CreatedAt = v
}

// GetUpdatedAt returns the UpdatedAt field value
func (o *ProxyConfigRead) GetUpdatedAt() time.Time {
	if o == nil {
		var ret time.Time
		return ret
	}

	return o.UpdatedAt
}

// GetUpdatedAtOk returns a tuple with the UpdatedAt field value
// and a boolean to check if the value has been set.
func (o *ProxyConfigRead) GetUpdatedAtOk() (*time.Time, bool) {
	if o == nil {
		return nil, false
	}
	return &o.UpdatedAt, true
}

// SetUpdatedAt sets field value
func (o *ProxyConfigRead) SetUpdatedAt(v time.Time) {
	o.UpdatedAt = v
}

// GetSecret returns the Secret field value
func (o *ProxyConfigRead) GetSecret() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Secret
}

// GetSecretOk returns a tuple with the Secret field value
// and a boolean to check if the value has been set.
func (o *ProxyConfigRead) GetSecretOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Secret, true
}

// SetSecret sets field value
func (o *ProxyConfigRead) SetSecret(v string) {
	o.Secret = v
}

// GetName returns the Name field value
func (o *ProxyConfigRead) GetName() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Name
}

// GetNameOk returns a tuple with the Name field value
// and a boolean to check if the value has been set.
func (o *ProxyConfigRead) GetNameOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Name, true
}

// SetName sets field value
func (o *ProxyConfigRead) SetName(v string) {
	o.Name = v
}

// GetMappingRules returns the MappingRules field value if set, zero value otherwise.
func (o *ProxyConfigRead) GetMappingRules() []MappingRule {
	if o == nil || IsNil(o.MappingRules) {
		var ret []MappingRule
		return ret
	}
	return o.MappingRules
}

// GetMappingRulesOk returns a tuple with the MappingRules field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ProxyConfigRead) GetMappingRulesOk() ([]MappingRule, bool) {
	if o == nil || IsNil(o.MappingRules) {
		return nil, false
	}
	return o.MappingRules, true
}

// HasMappingRules returns a boolean if a field has been set.
func (o *ProxyConfigRead) HasMappingRules() bool {
	if o != nil && !IsNil(o.MappingRules) {
		return true
	}

	return false
}

// SetMappingRules gets a reference to the given []MappingRule and assigns it to the MappingRules field.
func (o *ProxyConfigRead) SetMappingRules(v []MappingRule) {
	o.MappingRules = v
}

// GetAuthMechanism returns the AuthMechanism field value if set, zero value otherwise.
func (o *ProxyConfigRead) GetAuthMechanism() AuthMechanism {
	if o == nil || IsNil(o.AuthMechanism) {
		var ret AuthMechanism
		return ret
	}
	return *o.AuthMechanism
}

// GetAuthMechanismOk returns a tuple with the AuthMechanism field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ProxyConfigRead) GetAuthMechanismOk() (*AuthMechanism, bool) {
	if o == nil || IsNil(o.AuthMechanism) {
		return nil, false
	}
	return o.AuthMechanism, true
}

// HasAuthMechanism returns a boolean if a field has been set.
func (o *ProxyConfigRead) HasAuthMechanism() bool {
	if o != nil && !IsNil(o.AuthMechanism) {
		return true
	}

	return false
}

// SetAuthMechanism gets a reference to the given AuthMechanism and assigns it to the AuthMechanism field.
func (o *ProxyConfigRead) SetAuthMechanism(v AuthMechanism) {
	o.AuthMechanism = &v
}

func (o ProxyConfigRead) MarshalJSON() ([]byte, error) {
	toSerialize, err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o ProxyConfigRead) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	toSerialize["key"] = o.Key
	toSerialize["id"] = o.Id
	toSerialize["organization_id"] = o.OrganizationId
	toSerialize["project_id"] = o.ProjectId
	toSerialize["environment_id"] = o.EnvironmentId
	toSerialize["created_at"] = o.CreatedAt
	toSerialize["updated_at"] = o.UpdatedAt
	toSerialize["secret"] = o.Secret
	toSerialize["name"] = o.Name
	if !IsNil(o.MappingRules) {
		toSerialize["mapping_rules"] = o.MappingRules
	}
	if !IsNil(o.AuthMechanism) {
		toSerialize["auth_mechanism"] = o.AuthMechanism
	}
	return toSerialize, nil
}

type NullableProxyConfigRead struct {
	value *ProxyConfigRead
	isSet bool
}

func (v NullableProxyConfigRead) Get() *ProxyConfigRead {
	return v.value
}

func (v *NullableProxyConfigRead) Set(val *ProxyConfigRead) {
	v.value = val
	v.isSet = true
}

func (v NullableProxyConfigRead) IsSet() bool {
	return v.isSet
}

func (v *NullableProxyConfigRead) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableProxyConfigRead(val *ProxyConfigRead) *NullableProxyConfigRead {
	return &NullableProxyConfigRead{value: val, isSet: true}
}

func (v NullableProxyConfigRead) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableProxyConfigRead) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
