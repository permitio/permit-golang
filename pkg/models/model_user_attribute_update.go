package models

import "encoding/json"

// UserAttributeUpdate is the request body for PATCH user attribute.
// It matches the V2 PATCH /users/attributes/{attribute_id} payload.
type UserAttributeUpdate struct {
	Type        *AttributeType `json:"type,omitempty"`
	Description *string        `json:"description,omitempty"`
}

// NewUserAttributeUpdate returns an empty update (valid for partial PATCH).
func NewUserAttributeUpdate() *UserAttributeUpdate {
	return &UserAttributeUpdate{}
}

func (o *UserAttributeUpdate) GetType() AttributeType {
	if o == nil || IsNil(o.Type) {
		return ""
	}
	return *o.Type
}

func (o *UserAttributeUpdate) SetType(v AttributeType) {
	o.Type = &v
}

func (o *UserAttributeUpdate) HasType() bool {
	return o != nil && !IsNil(o.Type)
}

func (o *UserAttributeUpdate) GetDescription() string {
	if o == nil || IsNil(o.Description) {
		return ""
	}
	return *o.Description
}

func (o *UserAttributeUpdate) SetDescription(v string) {
	o.Description = &v
}

func (o *UserAttributeUpdate) HasDescription() bool {
	return o != nil && !IsNil(o.Description)
}

func (o UserAttributeUpdate) MarshalJSON() ([]byte, error) {
	m := map[string]interface{}{}
	if !IsNil(o.Type) {
		m["type"] = o.Type
	}
	if !IsNil(o.Description) {
		m["description"] = o.Description
	}
	return json.Marshal(m)
}
