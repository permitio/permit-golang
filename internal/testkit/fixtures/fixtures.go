// Package fixtures holds values that a careless JSON encoder or decoder changes, and keys that a
// careless client puts in the wrong place, for wire tests that check the SDK sends and reads them
// exactly. It is for tests only.
//
// The tests of this package show what encoding/json does with each value.
package fixtures

import "math"

// Numbers that a round trip through float64, or an encoder's choice of notation, changes.
const (
	// TwoPow53Plus1 is 2^53+1, 9007199254740993, the smallest positive integer that a float64
	// cannot hold. Decoded into an any without json.Decoder.UseNumber, it becomes the float64
	// 9007199254740992.
	TwoPow53Plus1 int64 = 1<<53 + 1

	// MaxInt64 is the largest int64, 9223372036854775807; a float64 rounds it to 2^63, which
	// overflows an int64.
	MaxInt64 int64 = math.MaxInt64

	// Huge is 1e300, a float64 far beyond every integer type. encoding/json writes it as 1e+300.
	Huge float64 = 1e300
)

// NegativeZero returns -0.0. It equals 0, so only its sign bit or its JSON text, -0, tells it
// apart; a Go constant cannot hold it.
func NegativeZero() float64 {
	return math.Copysign(0, -1)
}

// Strings that an encoder escapes, or that a decoder must accept.
const (
	// LineSeparator is U+2028 and ParagraphSeparator U+2029. JSON allows them raw in a string,
	// JavaScript before ES2019 did not, and encoding/json always escapes them.
	LineSeparator      = "\u2028"
	ParagraphSeparator = "\u2029"

	// HTMLSpecial holds the three characters that encoding/json escapes as \u003c, \u003e and
	// \u0026 unless HTML escaping is turned off.
	HTMLSpecial = "<>&"

	// LoneSurrogateJSON is a JSON string literal that holds an escaped lone surrogate: valid
	// JSON, but not valid UTF-16. encoding/json decodes it to [LoneSurrogateDecoded], and so does
	// the ExpectJSON of mockpermit, which then cannot tell it from another lone surrogate or from
	// U+FFFD; a test that checks the SDK sends it unchanged compares the bytes with ExpectBytes.
	LoneSurrogateJSON = `"\ud800"`

	// LoneSurrogateDecoded is what encoding/json decodes [LoneSurrogateJSON] to: U+FFFD, the
	// replacement character.
	LoneSurrogateDecoded = "\ufffd"
)

// Keys and identifiers that a client must not mistake for something else.
const (
	// UUIDKey is a key shaped like a version 4 UUID. The API takes an object's key or its ID where
	// a path names an object, so a client must send it as given and not treat it as an ID. Its
	// digits are mostly zeros, so that the secret scan does not take it for a credential.
	UUIDKey = "00000000-0000-4000-8000-000000000001"

	// TypedKey is an identifier in the "type:key" form that the PDP uses for a resource
	// instance, such as in a role assignment's resource_instance.
	TypedKey = "document:2026-annual-report"
)
