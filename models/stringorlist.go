package models

import (
	"bytes"
	"errors"
	"fmt"
	"slices"

	"github.com/permitio/permit-golang/v2/internal/wire"
)

// errUnsetStrings is the error of encoding an unset StringOrList, whose field must be left out
// instead.
var errUnsetStrings = errors.New("models: an unset StringOrList has no JSON value; tag its " +
	"field omitzero, so that encoding/json leaves the field out")

// errNotStrings is the error of decoding a JSON value that is neither a string nor an array of
// strings into a StringOrList.
var errNotStrings = errors.New("models: a StringOrList is a JSON string or a JSON array of " +
	"strings")

// stringsForm is the JSON form of a StringOrList. The zero form is unset, so that the zero
// StringOrList is unset.
type stringsForm uint8

const (
	// unsetStrings leaves the field out of the body.
	unsetStrings stringsForm = iota

	// oneString is a JSON string.
	oneString

	// stringList is a JSON array of strings.
	stringList
)

// StringOrList is a value that the API takes and returns as either a JSON string or a JSON
// array of strings, such as the [JwksConfig.Audience] of a token. It keeps the form that it was
// made or decoded in, so that a value read from a reply is sent back as it came:
//
//   - the zero StringOrList is unset: a field tagged omitzero, as every field of this type in
//     package models is, leaves it out of the body;
//   - [OneString] holds one string, and encodes as a JSON string;
//   - [StringList] holds a list, and encodes as a JSON array, also when it holds one string or
//     none.
//
// Decoding reads a JSON string as one string and a JSON array of strings as a list, and fails on
// any other JSON value; null leaves the value unset. [StringOrList.Values] returns the strings in
// either form. A StringOrList holds a slice, so compare two with [StringOrList.Equal], not ==.
type StringOrList struct {
	values []string
	form   stringsForm
}

// OneString returns a StringOrList that holds s, and encodes as the JSON string s.
func OneString(s string) StringOrList {
	return StringOrList{values: []string{s}, form: oneString}
}

// StringList returns a StringOrList that holds values, a copy of them, and encodes as a JSON
// array of them: [] when values is empty.
func StringList(values ...string) StringOrList {
	return StringOrList{values: append([]string{}, values...), form: stringList}
}

// Values returns a copy of the strings of v: the one string of [OneString], the strings of
// [StringList], or nil when v is unset.
func (v StringOrList) Values() []string {
	if v.form == unsetStrings {
		return nil
	}
	return slices.Clone(v.values)
}

// IsList reports whether v is a list, made with [StringList] or decoded from a JSON array, and
// not one string or unset.
func (v StringOrList) IsList() bool {
	return v.form == stringList
}

// IsZero reports whether v is unset, the zero value. encoding/json calls it for a field tagged
// omitzero, and leaves the field out when it returns true.
func (v StringOrList) IsZero() bool {
	return v.form == unsetStrings
}

// Equal reports whether v and other are in the same form and hold the same strings in the same
// order.
func (v StringOrList) Equal(other StringOrList) bool {
	return v.form == other.form && slices.Equal(v.values, other.values)
}

// MarshalJSON returns the JSON string of the one string of v, or the JSON array of the strings of
// a list, with <, > and & as they are, so that the encoder of the whole body decides whether to
// escape them. It fails when v is unset, whose field must be tagged omitzero.
func (v StringOrList) MarshalJSON() ([]byte, error) {
	switch v.form {
	case oneString:
		return wire.Encode(v.values[0])
	case stringList:
		return wire.Encode(v.values)
	default:
		return nil, errUnsetStrings
	}
}

// UnmarshalJSON sets v to the one string that the JSON string data holds, or to the list of
// strings that the JSON array data holds; null sets v to unset. data is one JSON value without
// whitespace around it, as encoding/json passes it. Any other JSON value, such as a number or an
// array that holds a value other than a string, null too, is an error, and leaves v as it is.
func (v *StringOrList) UnmarshalJSON(data []byte) error {
	switch {
	case string(data) == jsonNull:
		*v = StringOrList{}
		return nil
	case bytes.HasPrefix(data, []byte(`"`)):
		var s string
		if err := wire.Decode(data, &s); err != nil {
			return fmt.Errorf("%w: %w", errNotStrings, err)
		}
		*v = OneString(s)
		return nil
	case bytes.HasPrefix(data, []byte("[")):
		var items []any
		if err := wire.Decode(data, &items); err != nil {
			return fmt.Errorf("%w: %w", errNotStrings, err)
		}
		values := make([]string, 0, len(items))
		for _, item := range items {
			s, ok := item.(string)
			if !ok {
				return errNotStrings
			}
			values = append(values, s)
		}
		*v = StringOrList{values: values, form: stringList}
		return nil
	default:
		return errNotStrings
	}
}
