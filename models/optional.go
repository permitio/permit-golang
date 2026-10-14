package models

import (
	"errors"
	"fmt"

	"github.com/permitio/permit-golang/v2/internal/wire"
)

// jsonNull is the JSON text of null.
const jsonNull = "null"

// errUnset is the error of encoding an unset Optional, whose field must be left out instead.
var errUnset = errors.New("models: an unset Optional has no JSON value; tag its field " +
	"omitzero, so that encoding/json leaves the field out")

// errSomeNull is the error of encoding an Optional whose value encodes as null.
var errSomeNull = errors.New("models: the value of an Optional encodes as null, such as a nil " +
	"slice, map or pointer; use Null to send null, or an empty slice or map")

// state is which of its three states an Optional is in. The zero state is unset, so that the
// zero Optional is unset.
type state uint8

const (
	// unset leaves the field out of the body.
	unset state = iota

	// null sends null.
	null

	// set sends the value.
	set
)

// Optional is the type of a field of an update model that the API accepts null for. It has three
// states, and the body of a request holds the field in each differently:
//
//   - unset, the zero value: the body leaves the field out, so the API keeps what it stores;
//   - null, from [Null]: the body holds null for the field;
//   - a value, from [Some]: the body holds the value, also when it is the zero value of T, such
//     as "", 0 or false.
//
// A field of type Optional is tagged omitzero, such as `json:"description,omitzero"`, so that
// encoding/json calls [Optional.IsZero] and leaves the field out when it is unset. Encoding an
// unset Optional fails, as there is no JSON text for it: a field without omitzero, such as one
// tagged omitempty, makes the encoding fail rather than send null in place of leaving the field
// out.
//
// Models use Optional only for the fields that the API accepts null for. A nested map that the
// API does not accept null for is a plain map whose field is tagged omitzero instead: nil leaves
// the field out, so the API keeps what it stores, and an empty map that is not nil sends {}.
//
// Decoding reads null as null and any other JSON value as a value; a field that the body leaves
// out is left as it is, unset in a new value. A number that the value holds in an any, such as
// in a map[string]any, decodes as a [json.Number] with every digit of its text.
type Optional[T any] struct {
	value T
	state state
}

// Some returns an Optional that holds v. Encoding it fails when v encodes as null, such as a nil
// slice, map or pointer, so that a value never sends null: use [Null] to send null.
func Some[T any](v T) Optional[T] {
	return Optional[T]{value: v, state: set}
}

// Null returns an Optional that sends null.
func Null[T any]() Optional[T] {
	return Optional[T]{state: null}
}

// Get returns the value of o and true when o holds a value, and the zero value of T and false
// when o is unset or null; [Optional.IsNull] and [Optional.IsZero] tell those two apart.
func (o Optional[T]) Get() (T, bool) {
	return o.value, o.state == set
}

// IsNull reports whether o is null.
func (o Optional[T]) IsNull() bool {
	return o.state == null
}

// IsZero reports whether o is unset, the zero value. encoding/json calls it for a field tagged
// omitzero, and leaves the field out when it returns true.
func (o Optional[T]) IsZero() bool {
	return o.state == unset
}

// MarshalJSON returns null when o is null, and the JSON text of its value when o holds one, with
// <, > and & as they are, so that the encoder of the whole body decides whether to escape them.
// It fails when o is unset, whose field must be tagged omitzero, and when the value encodes as
// null.
func (o Optional[T]) MarshalJSON() ([]byte, error) {
	switch o.state {
	case null:
		return []byte(jsonNull), nil
	case set:
		// Through a pointer, so that a MarshalJSON that T has on its pointer is called, as it is
		// for a field of type T; o is a copy, so the pointer leaves the caller's value alone.
		data, err := wire.Encode(&o.value)
		if err != nil {
			return nil, err
		}
		if string(data) == jsonNull {
			return nil, errSomeNull
		}
		return data, nil
	default:
		return nil, errUnset
	}
}

// UnmarshalJSON sets o to null when data is null, and else to the value that data holds,
// decoded into a T with each number that T holds in an any as a [json.Number].
func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	if string(data) == jsonNull {
		*o = Null[T]()
		return nil
	}
	var v T
	if err := wire.Decode(data, &v); err != nil {
		return fmt.Errorf("models: decode Optional[%T]: %w", v, err)
	}
	*o = Some(v)
	return nil
}
