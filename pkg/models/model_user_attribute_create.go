package models

import "encoding/json"

// UserAttributeCreate is the request body for creating a user (schema) attribute.
// It matches the V2 POST /v2/schema/{proj_id}/{env_id}/users/attributes payload.
type UserAttributeCreate struct {
	// A URL-friendly name of the attribute (i.e: slug).
	Key string `json:"key"`
	// The type of the attribute (bool, number, string, time, array, json, object, object_array).
	Type AttributeType `json:"type"`
	// An optional longer description of what this attribute represents in your system.
	Description *string `json:"description,omitempty"`
}

// NewUserAttributeCreate builds a create request with required key and type.
func NewUserAttributeCreate(key string, type_ AttributeType) *UserAttributeCreate {
	return &UserAttributeCreate{Key: key, Type: type_}
}

// NewUserAttributeCreateWithDefaults instantiates an empty UserAttributeCreate.
func NewUserAttributeCreateWithDefaults() *UserAttributeCreate {
	return &UserAttributeCreate{}
}

func (o *UserAttributeCreate) GetKey() string {
	if o == nil {
		return ""
	}
	return o.Key
}

func (o *UserAttributeCreate) SetKey(v string) {
	o.Key = v
}

func (o *UserAttributeCreate) GetType() AttributeType {
	if o == nil {
		return ""
	}
	return o.Type
}

func (o *UserAttributeCreate) SetType(v AttributeType) {
	o.Type = v
}

func (o *UserAttributeCreate) GetDescription() string {
	if o == nil || IsNil(o.Description) {
		return ""
	}
	return *o.Description
}

func (o *UserAttributeCreate) SetDescription(v string) {
	o.Description = &v
}

func (o UserAttributeCreate) MarshalJSON() ([]byte, error) {
	m := map[string]interface{}{"key": o.Key, "type": o.Type}
	if !IsNil(o.Description) {
		m["description"] = o.Description
	}
	return json.Marshal(m)
}
