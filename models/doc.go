// Package models holds the request and response types of the Permit API that the permit
// package sends and receives.
//
// # Update fields
//
// A field of an update model that the API accepts null for is an [Optional], which has three
// states: unset, its zero value, leaves the field out of the body, so the API keeps what it
// stores; [Null] sends null; and [Some] sends a value. A nested map that the API does not accept
// null for is a plain map whose field is tagged omitzero instead: nil leaves the field out, and
// an empty map that is not nil sends {}.
//
// # Enums
//
// An enum, such as [AttributeType], is a string type with a constant for each value that this
// version of the SDK knows, and a Known method. A value that the API adds later decodes as it is,
// and Known reports false for it, so a reply that holds one is read rather than rejected.
//
// # Numbers and times
//
// [Attributes] keeps each number as a [json.Number], with every digit of its text, and [Time]
// reads a timestamp with or without a zone.
package models
