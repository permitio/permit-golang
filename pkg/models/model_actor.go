/*
Permit.io API

 Authorization as a service

API version: 2.0.0
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package models

import (
	"encoding/json"
	"fmt"
)

// Actor - struct for Actor
type Actor struct {
	AuthnMeAPIKeyRead *AuthnMeAPIKeyRead
	AuthnMeMemberRead *AuthnMeMemberRead
	AuthnMeUserRead   *AuthnMeUserRead
}

// AuthnMeAPIKeyReadAsActor is a convenience function that returns AuthnMeAPIKeyRead wrapped in Actor
func AuthnMeAPIKeyReadAsActor(v *AuthnMeAPIKeyRead) Actor {
	return Actor{
		AuthnMeAPIKeyRead: v,
	}
}

// AuthnMeMemberReadAsActor is a convenience function that returns AuthnMeMemberRead wrapped in Actor
func AuthnMeMemberReadAsActor(v *AuthnMeMemberRead) Actor {
	return Actor{
		AuthnMeMemberRead: v,
	}
}

// AuthnMeUserReadAsActor is a convenience function that returns AuthnMeUserRead wrapped in Actor
func AuthnMeUserReadAsActor(v *AuthnMeUserRead) Actor {
	return Actor{
		AuthnMeUserRead: v,
	}
}

// Unmarshal JSON data into one of the pointers in the struct
func (dst *Actor) UnmarshalJSON(data []byte) error {
	var err error
	match := 0
	// try to unmarshal data into AuthnMeAPIKeyRead
	err = newStrictDecoder(data).Decode(&dst.AuthnMeAPIKeyRead)
	if err == nil {
		jsonAuthnMeAPIKeyRead, _ := json.Marshal(dst.AuthnMeAPIKeyRead)
		if string(jsonAuthnMeAPIKeyRead) == "{}" { // empty struct
			dst.AuthnMeAPIKeyRead = nil
		} else {
			match++
		}
	} else {
		dst.AuthnMeAPIKeyRead = nil
	}

	// try to unmarshal data into AuthnMeMemberRead
	err = newStrictDecoder(data).Decode(&dst.AuthnMeMemberRead)
	if err == nil {
		jsonAuthnMeMemberRead, _ := json.Marshal(dst.AuthnMeMemberRead)
		if string(jsonAuthnMeMemberRead) == "{}" { // empty struct
			dst.AuthnMeMemberRead = nil
		} else {
			match++
		}
	} else {
		dst.AuthnMeMemberRead = nil
	}

	// try to unmarshal data into AuthnMeUserRead
	err = newStrictDecoder(data).Decode(&dst.AuthnMeUserRead)
	if err == nil {
		jsonAuthnMeUserRead, _ := json.Marshal(dst.AuthnMeUserRead)
		if string(jsonAuthnMeUserRead) == "{}" { // empty struct
			dst.AuthnMeUserRead = nil
		} else {
			match++
		}
	} else {
		dst.AuthnMeUserRead = nil
	}

	if match > 1 { // more than 1 match
		// reset to nil
		dst.AuthnMeAPIKeyRead = nil
		dst.AuthnMeMemberRead = nil
		dst.AuthnMeUserRead = nil

		return fmt.Errorf("data matches more than one schema in oneOf(Actor)")
	} else if match == 1 {
		return nil // exactly one match
	} else { // no match
		return fmt.Errorf("data failed to match schemas in oneOf(Actor)")
	}
}

// Marshal data from the first non-nil pointers in the struct to JSON
func (src Actor) MarshalJSON() ([]byte, error) {
	if src.AuthnMeAPIKeyRead != nil {
		return json.Marshal(&src.AuthnMeAPIKeyRead)
	}

	if src.AuthnMeMemberRead != nil {
		return json.Marshal(&src.AuthnMeMemberRead)
	}

	if src.AuthnMeUserRead != nil {
		return json.Marshal(&src.AuthnMeUserRead)
	}

	return nil, nil // no data in oneOf schemas
}

// Get the actual instance
func (obj *Actor) GetActualInstance() interface{} {
	if obj == nil {
		return nil
	}
	if obj.AuthnMeAPIKeyRead != nil {
		return obj.AuthnMeAPIKeyRead
	}

	if obj.AuthnMeMemberRead != nil {
		return obj.AuthnMeMemberRead
	}

	if obj.AuthnMeUserRead != nil {
		return obj.AuthnMeUserRead
	}

	// all schemas are nil
	return nil
}

type NullableActor struct {
	value *Actor
	isSet bool
}

func (v NullableActor) Get() *Actor {
	return v.value
}

func (v *NullableActor) Set(val *Actor) {
	v.value = val
	v.isSet = true
}

func (v NullableActor) IsSet() bool {
	return v.isSet
}

func (v *NullableActor) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableActor(val *Actor) *NullableActor {
	return &NullableActor{value: val, isSet: true}
}

func (v NullableActor) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableActor) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
