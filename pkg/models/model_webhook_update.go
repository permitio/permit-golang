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

// WebhookUpdate struct for WebhookUpdate
type WebhookUpdate struct {
	// The url to POST the webhook to
	Url *string `json:"url,omitempty"`
	// An optional bearer token to use to authenticate the request
	BearerToken *string `json:"bearer_token,omitempty"`
}

// NewWebhookUpdate instantiates a new WebhookUpdate object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewWebhookUpdate() *WebhookUpdate {
	this := WebhookUpdate{}
	return &this
}

// NewWebhookUpdateWithDefaults instantiates a new WebhookUpdate object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewWebhookUpdateWithDefaults() *WebhookUpdate {
	this := WebhookUpdate{}
	return &this
}

// GetUrl returns the Url field value if set, zero value otherwise.
func (o *WebhookUpdate) GetUrl() string {
	if o == nil || IsNil(o.Url) {
		var ret string
		return ret
	}
	return *o.Url
}

// GetUrlOk returns a tuple with the Url field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *WebhookUpdate) GetUrlOk() (*string, bool) {
	if o == nil || IsNil(o.Url) {
		return nil, false
	}
	return o.Url, true
}

// HasUrl returns a boolean if a field has been set.
func (o *WebhookUpdate) HasUrl() bool {
	if o != nil && !IsNil(o.Url) {
		return true
	}

	return false
}

// SetUrl gets a reference to the given string and assigns it to the Url field.
func (o *WebhookUpdate) SetUrl(v string) {
	o.Url = &v
}

// GetBearerToken returns the BearerToken field value if set, zero value otherwise.
func (o *WebhookUpdate) GetBearerToken() string {
	if o == nil || IsNil(o.BearerToken) {
		var ret string
		return ret
	}
	return *o.BearerToken
}

// GetBearerTokenOk returns a tuple with the BearerToken field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *WebhookUpdate) GetBearerTokenOk() (*string, bool) {
	if o == nil || IsNil(o.BearerToken) {
		return nil, false
	}
	return o.BearerToken, true
}

// HasBearerToken returns a boolean if a field has been set.
func (o *WebhookUpdate) HasBearerToken() bool {
	if o != nil && !IsNil(o.BearerToken) {
		return true
	}

	return false
}

// SetBearerToken gets a reference to the given string and assigns it to the BearerToken field.
func (o *WebhookUpdate) SetBearerToken(v string) {
	o.BearerToken = &v
}

func (o WebhookUpdate) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if !IsNil(o.Url) {
		toSerialize["url"] = o.Url
	}
	if !IsNil(o.BearerToken) {
		toSerialize["bearer_token"] = o.BearerToken
	}
	return json.Marshal(toSerialize)
}

type NullableWebhookUpdate struct {
	value *WebhookUpdate
	isSet bool
}

func (v NullableWebhookUpdate) Get() *WebhookUpdate {
	return v.value
}

func (v *NullableWebhookUpdate) Set(val *WebhookUpdate) {
	v.value = val
	v.isSet = true
}

func (v NullableWebhookUpdate) IsSet() bool {
	return v.isSet
}

func (v *NullableWebhookUpdate) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableWebhookUpdate(val *WebhookUpdate) *NullableWebhookUpdate {
	return &NullableWebhookUpdate{value: val, isSet: true}
}

func (v NullableWebhookUpdate) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableWebhookUpdate) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
