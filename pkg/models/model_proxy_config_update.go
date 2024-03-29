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

// checks if the ProxyConfigUpdate type satisfies the MappedNullable interface at compile time
var _ MappedNullable = &ProxyConfigUpdate{}

// ProxyConfigUpdate struct for ProxyConfigUpdate
type ProxyConfigUpdate struct {
	Secret *string `json:"secret,omitempty"`
	// The name of the proxy config, for example: 'Stripe API'
	Name *string `json:"name,omitempty"`
	// Proxy config mapping rules will include the rules that will be used to map the request to the backend service by a URL and a http method.
	MappingRules  []MappingRule  `json:"mapping_rules,omitempty"`
	AuthMechanism *AuthMechanism `json:"auth_mechanism,omitempty"`
}

// NewProxyConfigUpdate instantiates a new ProxyConfigUpdate object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewProxyConfigUpdate() *ProxyConfigUpdate {
	this := ProxyConfigUpdate{}

	return &this
}

// NewProxyConfigUpdateWithDefaults instantiates a new ProxyConfigUpdate object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewProxyConfigUpdateWithDefaults() *ProxyConfigUpdate {
	this := ProxyConfigUpdate{}
	var authMechanism AuthMechanism = BEARER
	this.AuthMechanism = &authMechanism
	return &this
}

// GetSecret returns the Secret field value if set, zero value otherwise.
func (o *ProxyConfigUpdate) GetSecret() string {
	if o == nil || IsNil(o.Secret) {
		var ret string
		return ret
	}
	return *o.Secret
}

// GetSecretOk returns a tuple with the Secret field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ProxyConfigUpdate) GetSecretOk() (*string, bool) {
	if o == nil || IsNil(o.Secret) {
		return nil, false
	}
	return o.Secret, true
}

// HasSecret returns a boolean if a field has been set.
func (o *ProxyConfigUpdate) HasSecret() bool {
	if o != nil && !IsNil(o.Secret) {
		return true
	}

	return false
}

// SetSecret gets a reference to the given Secret and assigns it to the Secret field.
func (o *ProxyConfigUpdate) SetSecret(v string) {
	o.Secret = &v
}

// GetName returns the Name field value if set, zero value otherwise.
func (o *ProxyConfigUpdate) GetName() string {
	if o == nil || IsNil(o.Name) {
		var ret string
		return ret
	}
	return *o.Name
}

// GetNameOk returns a tuple with the Name field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ProxyConfigUpdate) GetNameOk() (*string, bool) {
	if o == nil || IsNil(o.Name) {
		return nil, false
	}
	return o.Name, true
}

// HasName returns a boolean if a field has been set.
func (o *ProxyConfigUpdate) HasName() bool {
	if o != nil && !IsNil(o.Name) {
		return true
	}

	return false
}

// SetName gets a reference to the given string and assigns it to the Name field.
func (o *ProxyConfigUpdate) SetName(v string) {
	o.Name = &v
}

// GetMappingRules returns the MappingRules field value if set, zero value otherwise.
func (o *ProxyConfigUpdate) GetMappingRules() []MappingRule {
	if o == nil || IsNil(o.MappingRules) {
		var ret []MappingRule
		return ret
	}
	return o.MappingRules
}

// GetMappingRulesOk returns a tuple with the MappingRules field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ProxyConfigUpdate) GetMappingRulesOk() ([]MappingRule, bool) {
	if o == nil || IsNil(o.MappingRules) {
		return nil, false
	}
	return o.MappingRules, true
}

// HasMappingRules returns a boolean if a field has been set.
func (o *ProxyConfigUpdate) HasMappingRules() bool {
	if o != nil && !IsNil(o.MappingRules) {
		return true
	}

	return false
}

// SetMappingRules gets a reference to the given []MappingRule and assigns it to the MappingRules field.
func (o *ProxyConfigUpdate) SetMappingRules(v []MappingRule) {
	o.MappingRules = v
}

// GetAuthMechanism returns the AuthMechanism field value if set, zero value otherwise.
func (o *ProxyConfigUpdate) GetAuthMechanism() AuthMechanism {
	if o == nil || IsNil(o.AuthMechanism) {
		var ret AuthMechanism
		return ret
	}
	return *o.AuthMechanism
}

// GetAuthMechanismOk returns a tuple with the AuthMechanism field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *ProxyConfigUpdate) GetAuthMechanismOk() (*AuthMechanism, bool) {
	if o == nil || IsNil(o.AuthMechanism) {
		return nil, false
	}
	return o.AuthMechanism, true
}

// HasAuthMechanism returns a boolean if a field has been set.
func (o *ProxyConfigUpdate) HasAuthMechanism() bool {
	if o != nil && !IsNil(o.AuthMechanism) {
		return true
	}

	return false
}

// SetAuthMechanism gets a reference to the given AuthMechanism and assigns it to the AuthMechanism field.
func (o *ProxyConfigUpdate) SetAuthMechanism(v AuthMechanism) {
	o.AuthMechanism = &v
}

func (o ProxyConfigUpdate) MarshalJSON() ([]byte, error) {
	toSerialize, err := o.ToMap()
	if err != nil {
		return []byte{}, err
	}
	return json.Marshal(toSerialize)
}

func (o ProxyConfigUpdate) ToMap() (map[string]interface{}, error) {
	toSerialize := map[string]interface{}{}
	if !IsNil(o.Secret) {
		toSerialize["secret"] = o.Secret
	}
	if !IsNil(o.Name) {
		toSerialize["name"] = o.Name
	}
	if !IsNil(o.MappingRules) {
		toSerialize["mapping_rules"] = o.MappingRules
	}
	if !IsNil(o.AuthMechanism) {
		toSerialize["auth_mechanism"] = o.AuthMechanism
	}
	return toSerialize, nil
}

type NullableProxyConfigUpdate struct {
	value *ProxyConfigUpdate
	isSet bool
}

func (v NullableProxyConfigUpdate) Get() *ProxyConfigUpdate {
	return v.value
}

func (v *NullableProxyConfigUpdate) Set(val *ProxyConfigUpdate) {
	v.value = val
	v.isSet = true
}

func (v NullableProxyConfigUpdate) IsSet() bool {
	return v.isSet
}

func (v *NullableProxyConfigUpdate) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableProxyConfigUpdate(val *ProxyConfigUpdate) *NullableProxyConfigUpdate {
	return &NullableProxyConfigUpdate{value: val, isSet: true}
}

func (v NullableProxyConfigUpdate) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableProxyConfigUpdate) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
