package models

import (
	"encoding/json"
	"time"
)

// UserAttributeRead is a user schema attribute as returned by the V2 API.
type UserAttributeRead struct {
	Type           AttributeType `json:"type"`
	Description    *string       `json:"description,omitempty"`
	Key            string        `json:"key"`
	Id             string        `json:"id"`
	ResourceId     string        `json:"resource_id"`
	ResourceKey    string        `json:"resource_key"`
	OrganizationId string        `json:"organization_id"`
	ProjectId      string        `json:"project_id"`
	EnvironmentId  string        `json:"environment_id"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
	BuiltIn        bool          `json:"built_in"`
}

func NewUserAttributeReadWithDefaults() *UserAttributeRead {
	return &UserAttributeRead{}
}

func (o *UserAttributeRead) GetKey() string {
	if o == nil {
		return ""
	}
	return o.Key
}

func (o *UserAttributeRead) GetId() string {
	if o == nil {
		return ""
	}
	return o.Id
}

func (o *UserAttributeRead) GetType() AttributeType {
	if o == nil {
		return ""
	}
	return o.Type
}

func (o *UserAttributeRead) GetBuiltIn() bool {
	if o == nil {
		return false
	}
	return o.BuiltIn
}

func (o *UserAttributeRead) GetDescription() string {
	if o == nil || IsNil(o.Description) {
		return ""
	}
	return *o.Description
}

func (o UserAttributeRead) MarshalJSON() ([]byte, error) {
	m := map[string]interface{}{
		"type": o.Type, "key": o.Key, "id": o.Id,
		"resource_id": o.ResourceId, "resource_key": o.ResourceKey,
		"organization_id": o.OrganizationId, "project_id": o.ProjectId,
		"environment_id": o.EnvironmentId, "created_at": o.CreatedAt,
		"updated_at": o.UpdatedAt, "built_in": o.BuiltIn,
	}
	if !IsNil(o.Description) {
		m["description"] = o.Description
	}
	return json.Marshal(m)
}
