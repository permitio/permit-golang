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

// InviteRead struct for InviteRead
type InviteRead struct {
	// The invited member's email address
	Email string `json:"email"`
	// The role the member will be assigned with
	Role string `json:"role"`
	// Unique id of the invite
	Id string `json:"id"`
	// Unique id of the organization that the invite belongs to.
	OrganizationId string `json:"organization_id"`
	// The invite code that is sent to the member's email
	InviteCode string `json:"invite_code"`
	// Date and time when the invite was created (ISO_8601 format).
	CreatedAt time.Time `json:"created_at"`
	// The status of the invite (pending, failed, etc)
	Status InviteStatus `json:"status"`
	// if failed, the reason the invitation failed
	FailedReason *string `json:"failed_reason,omitempty"`
}

// NewInviteRead instantiates a new InviteRead object
// This constructor will assign default values to properties that have it defined,
// and makes sure properties required by API are set, but the set of arguments
// will change when the set of required properties is changed
func NewInviteRead(email string, role string, id string, organizationId string, inviteCode string, createdAt time.Time, status InviteStatus) *InviteRead {
	this := InviteRead{}
	this.Email = email
	this.Role = role
	this.Id = id
	this.OrganizationId = organizationId
	this.InviteCode = inviteCode
	this.CreatedAt = createdAt
	this.Status = status
	return &this
}

// NewInviteReadWithDefaults instantiates a new InviteRead object
// This constructor will only assign default values to properties that have it defined,
// but it doesn't guarantee that properties required by API are set
func NewInviteReadWithDefaults() *InviteRead {
	this := InviteRead{}
	return &this
}

// GetEmail returns the Email field value
func (o *InviteRead) GetEmail() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Email
}

// GetEmailOk returns a tuple with the Email field value
// and a boolean to check if the value has been set.
func (o *InviteRead) GetEmailOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Email, true
}

// SetEmail sets field value
func (o *InviteRead) SetEmail(v string) {
	o.Email = v
}

// GetRole returns the Role field value
func (o *InviteRead) GetRole() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Role
}

// GetRoleOk returns a tuple with the Role field value
// and a boolean to check if the value has been set.
func (o *InviteRead) GetRoleOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Role, true
}

// SetRole sets field value
func (o *InviteRead) SetRole(v string) {
	o.Role = v
}

// GetId returns the Id field value
func (o *InviteRead) GetId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.Id
}

// GetIdOk returns a tuple with the Id field value
// and a boolean to check if the value has been set.
func (o *InviteRead) GetIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Id, true
}

// SetId sets field value
func (o *InviteRead) SetId(v string) {
	o.Id = v
}

// GetOrganizationId returns the OrganizationId field value
func (o *InviteRead) GetOrganizationId() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.OrganizationId
}

// GetOrganizationIdOk returns a tuple with the OrganizationId field value
// and a boolean to check if the value has been set.
func (o *InviteRead) GetOrganizationIdOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.OrganizationId, true
}

// SetOrganizationId sets field value
func (o *InviteRead) SetOrganizationId(v string) {
	o.OrganizationId = v
}

// GetInviteCode returns the InviteCode field value
func (o *InviteRead) GetInviteCode() string {
	if o == nil {
		var ret string
		return ret
	}

	return o.InviteCode
}

// GetInviteCodeOk returns a tuple with the InviteCode field value
// and a boolean to check if the value has been set.
func (o *InviteRead) GetInviteCodeOk() (*string, bool) {
	if o == nil {
		return nil, false
	}
	return &o.InviteCode, true
}

// SetInviteCode sets field value
func (o *InviteRead) SetInviteCode(v string) {
	o.InviteCode = v
}

// GetCreatedAt returns the CreatedAt field value
func (o *InviteRead) GetCreatedAt() time.Time {
	if o == nil {
		var ret time.Time
		return ret
	}

	return o.CreatedAt
}

// GetCreatedAtOk returns a tuple with the CreatedAt field value
// and a boolean to check if the value has been set.
func (o *InviteRead) GetCreatedAtOk() (*time.Time, bool) {
	if o == nil {
		return nil, false
	}
	return &o.CreatedAt, true
}

// SetCreatedAt sets field value
func (o *InviteRead) SetCreatedAt(v time.Time) {
	o.CreatedAt = v
}

// GetStatus returns the Status field value
func (o *InviteRead) GetStatus() InviteStatus {
	if o == nil {
		var ret InviteStatus
		return ret
	}

	return o.Status
}

// GetStatusOk returns a tuple with the Status field value
// and a boolean to check if the value has been set.
func (o *InviteRead) GetStatusOk() (*InviteStatus, bool) {
	if o == nil {
		return nil, false
	}
	return &o.Status, true
}

// SetStatus sets field value
func (o *InviteRead) SetStatus(v InviteStatus) {
	o.Status = v
}

// GetFailedReason returns the FailedReason field value if set, zero value otherwise.
func (o *InviteRead) GetFailedReason() string {
	if o == nil || IsNil(o.FailedReason) {
		var ret string
		return ret
	}
	return *o.FailedReason
}

// GetFailedReasonOk returns a tuple with the FailedReason field value if set, nil otherwise
// and a boolean to check if the value has been set.
func (o *InviteRead) GetFailedReasonOk() (*string, bool) {
	if o == nil || IsNil(o.FailedReason) {
		return nil, false
	}
	return o.FailedReason, true
}

// HasFailedReason returns a boolean if a field has been set.
func (o *InviteRead) HasFailedReason() bool {
	if o != nil && !IsNil(o.FailedReason) {
		return true
	}

	return false
}

// SetFailedReason gets a reference to the given string and assigns it to the FailedReason field.
func (o *InviteRead) SetFailedReason(v string) {
	o.FailedReason = &v
}

func (o InviteRead) MarshalJSON() ([]byte, error) {
	toSerialize := map[string]interface{}{}
	if true {
		toSerialize["email"] = o.Email
	}
	if true {
		toSerialize["role"] = o.Role
	}
	if true {
		toSerialize["id"] = o.Id
	}
	if true {
		toSerialize["organization_id"] = o.OrganizationId
	}
	if true {
		toSerialize["invite_code"] = o.InviteCode
	}
	if true {
		toSerialize["created_at"] = o.CreatedAt
	}
	if true {
		toSerialize["status"] = o.Status
	}
	if !IsNil(o.FailedReason) {
		toSerialize["failed_reason"] = o.FailedReason
	}
	return json.Marshal(toSerialize)
}

type NullableInviteRead struct {
	value *InviteRead
	isSet bool
}

func (v NullableInviteRead) Get() *InviteRead {
	return v.value
}

func (v *NullableInviteRead) Set(val *InviteRead) {
	v.value = val
	v.isSet = true
}

func (v NullableInviteRead) IsSet() bool {
	return v.isSet
}

func (v *NullableInviteRead) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableInviteRead(val *InviteRead) *NullableInviteRead {
	return &NullableInviteRead{value: val, isSet: true}
}

func (v NullableInviteRead) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableInviteRead) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
