package models

import "github.com/permitio/permit-golang/v2/internal/wire"

// Attributes are the attributes of an object, such as a user or a tenant: a map from each
// attribute's key to its value, any JSON value.
//
// Decoding keeps each number as a [json.Number], with every digit of its text, also in nested
// objects and arrays, also under a plain json.Unmarshal: read the number with its Int64, Float64 or
// String method. A float64 cannot hold every integer beyond 2^53, and would read
// 9007199254740993 as 9007199254740992. Encoding writes a json.Number as its text, so a number
// read from a reply is sent back unchanged.
//
// A field of type Attributes in an update model is tagged omitzero: nil leaves the field out,
// and an empty Attributes that is not nil sends {}.
type Attributes map[string]any

// UnmarshalJSON sets a to the JSON object that data holds, with each number as a json.Number,
// in place of what a held; null sets a to nil.
func (a *Attributes) UnmarshalJSON(data []byte) error {
	var m map[string]any
	if err := wire.Decode(data, &m); err != nil {
		return err
	}
	*a = m
	return nil
}
