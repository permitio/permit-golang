package models

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/permitio/permit-golang/v2/internal/redact"
)

// Secret is a credential that the API returns, such as the secret of an API key in
// [APIKeyRead.Secret] or the client secret of a PDP in [PDPConfigRead.ClientSecret], which does
// not print. Read the credential with [Secret.Value]; the zero Secret is empty, and [NewSecret]
// makes one that holds a credential.
//
// String, GoString, Format with every verb, flag and width, LogValue and MarshalJSON all give a
// marker, the word REDACTED in square brackets, for a Secret that holds a credential, and "" for
// an empty one, so a printed or logged model still shows whether it holds a credential: fmt's %v,
// %+v and %#v of a model that holds a Secret, slog's text and JSON handlers, and encoding/json
// write the marker in its place.
// UnmarshalJSON reads a JSON string, as a reply holds the credential, and null as the empty
// Secret.
//
// A Secret holds only a pointer to its credential: fmt cannot call the methods of an unexported
// struct field, so it prints a Secret held in one as an address, and never as the credential. A
// printer that follows pointers by reflection, which fmt and slog do not, can still reach it.
type Secret struct {
	secret redact.Secret
}

// errNotAString is the error of UnmarshalJSON for a value that is not a JSON string or null. It
// does not hold the value, which may be a credential.
var errNotAString = errors.New("models: a Secret is read from a JSON string or null")

// NewSecret returns a Secret that holds credential, such as for a test that builds a model.
func NewSecret(credential string) Secret {
	return Secret{secret: redact.NewSecret(credential)}
}

// Value returns the credential that s holds, or "" when s is empty.
func (s Secret) Value() string { return s.secret.Value() }

// String returns the marker, the word REDACTED in square brackets, or "" when s is empty.
func (s Secret) String() string { return s.secret.String() }

// GoString returns what [Secret.String] returns, for the %#v verb.
func (s Secret) GoString() string { return s.secret.GoString() }

// Format writes what [Secret.String] returns, for every verb, flag and width.
func (s Secret) Format(f fmt.State, verb rune) { s.secret.Format(f, verb) }

// LogValue returns what [Secret.String] returns as a string value, so slog never sees the
// credential.
func (s Secret) LogValue() slog.Value { return s.secret.LogValue() }

// MarshalJSON returns what [Secret.String] returns as a JSON string, so encoding/json, and
// slog's JSON handler with a model that holds s, never write the credential.
func (s Secret) MarshalJSON() ([]byte, error) { return s.secret.MarshalJSON() }

// UnmarshalJSON reads a JSON string into s, and null as the empty Secret. Any other JSON value
// is an error that does not hold the value.
func (s *Secret) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*s = Secret{}
		return nil
	}
	var credential string
	if json.Unmarshal(data, &credential) != nil {
		return errNotAString
	}
	*s = NewSecret(credential)
	return nil
}

// Equal reports whether s and other hold the same credential, such as to tell whether a rotated
// API key has a new secret; go-cmp uses it to compare models that hold a Secret. Two empty
// Secrets are equal. It compares the credentials with [subtle.ConstantTimeCompare], whose time
// depends on their lengths, not on their content.
func (s Secret) Equal(other Secret) bool {
	return subtle.ConstantTimeCompare([]byte(s.Value()), []byte(other.Value())) == 1
}
