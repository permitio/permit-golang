// Package redact keeps credentials out of what the SDK prints, logs and keeps in its errors.
//
// Each client owns a [Set] that holds its own credentials: its API key, and the credentials that
// its calls return. [Scrub] replaces the members of a Set, and the values that a fixed list of
// patterns finds, with [Marker]. Nothing here is shared between clients, so a client's credentials
// are kept only as long as the client's Set.
package redact

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode"
)

// Marker is the text that replaces each credential.
const Marker = "[REDACTED]"

// Secret is a credential that does not print. Create one with [NewSecret] and read the credential
// with [Secret.Value]; the zero Secret is empty.
//
// String, GoString, Format with every verb, flag and width, LogValue and MarshalJSON all give
// [Marker] for a Secret that holds a credential, and "" for an empty one, so printed configuration
// still shows whether a credential is set. A Secret holds only a pointer to its value: fmt cannot
// call the methods of an unexported struct field, so it prints a Secret held in one as an address,
// as it does for the %p verb, and never as the value. A printer that follows pointers by
// reflection, which fmt and slog do not, can still reach the value.
type Secret struct {
	value *string
}

// NewSecret returns a Secret that holds credential.
func NewSecret(credential string) Secret {
	return Secret{value: &credential}
}

// Value returns the credential that s holds, or "" when s is empty.
func (s Secret) Value() string {
	if s.value == nil {
		return ""
	}
	return *s.value
}

// String returns [Marker], or "" when s is empty.
func (s Secret) String() string {
	if s.Value() == "" {
		return ""
	}
	return Marker
}

// GoString returns what [Secret.String] returns, for the %#v verb.
func (s Secret) GoString() string { return s.String() }

// Format writes what [Secret.String] returns, for every verb, flag and width.
func (s Secret) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, s.String()) }

// LogValue returns what [Secret.String] returns as a string value, so slog never sees the
// credential.
func (s Secret) LogValue() slog.Value { return slog.StringValue(s.String()) }

// MarshalJSON returns what [Secret.String] returns as a JSON string, so encoding/json, and slog's
// JSON handler with a struct that holds s, never write the credential. A request body that
// carries the credential takes [Secret.Value].
func (s Secret) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

// Set holds the credentials of one client, which [Scrub] replaces. The zero value is an empty Set
// ready to use. A Set is safe for concurrent use.
//
// A Set only grows: a client that rotates its API key keeps the old key in its Set too, for as
// long as the client lives.
type Set struct {
	mu      sync.RWMutex
	secrets []string
}

// NewSet returns a Set that holds secrets.
func NewSet(secrets ...string) *Set {
	s := &Set{}
	s.Add(secrets...)
	return s
}

// Add adds secrets to s. It skips the empty string, which is no credential, and a secret that s
// holds already.
func (s *Set) Add(secrets ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, secret := range secrets {
		if secret != "" && !slices.Contains(s.secrets, secret) {
			s.secrets = append(s.secrets, secret)
		}
	}
}

// Contains reports whether s holds secret.
func (s *Set) Contains(secret string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Contains(s.secrets, secret)
}

// longestFirst returns the secrets of s, the longest first, so that a secret is replaced before a
// shorter one that is a part of it. A nil Set holds none.
func (s *Set) longestFirst() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	secrets := slices.Clone(s.secrets)
	s.mu.RUnlock()
	slices.SortStableFunc(secrets, func(a, b string) int { return cmp.Compare(len(b), len(a)) })
	return secrets
}

// credentialNames are the words that make a JSON object key name a credential, as isCredential
// compares them.
var credentialNames = []string{
	"secret", "token", "apikey", "password", "passwd", "pwd", "authorization", "privatekey",
	"accesskey", "credential", "cookie",
}

// The stateless patterns. Each finds a credential whatever client the text comes from.
var (
	// permitKey matches a Permit API key.
	permitKey = regexp.MustCompile(`permit_key_[A-Za-z0-9_-]+`)

	// authorizationHeader matches the value of an Authorization or Proxy-Authorization header
	// written as text, "Authorization: Bearer <token>", with its scheme. The value ends at a space
	// or at a character that ends a quoted or listed value, so the rest of the text is kept.
	authorizationHeader = regexp.MustCompile(
		`(?i)(\bauthorization[ \t]*:[ \t]*)(?:[a-z][a-z0-9-]*[ \t]+)?[^\s"',;\\]+`)

	// jsonPair matches a "key": "value" pair of JSON strings, in text that is not JSON.
	jsonPair = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"(\s*:\s*)"(?:[^"\\]|\\.)*"`)
)

// Scrub returns a copy of data with each credential that it finds replaced by [Marker]:
//
//   - every member of set, which may be nil;
//   - every Permit API key: permit_key_ and one or more ASCII letters, digits, '_' and '-';
//   - the value of an Authorization header written as text, "Authorization: Bearer <token>";
//   - in each JSON document that data starts with, one or several such as JSON lines: each string
//     and number at any depth under an object key that names a credential, or under the key input
//     of an object whose loc list names one, as a validation error echoes a field; each other
//     string is scrubbed as data is, and each object key as text;
//   - in data that is not only JSON documents: the string of each "key": "value" pair whose key
//     names a credential.
//
// A key names a credential when, in lower case and without '-' and '_', it contains secret,
// token, apikey, password, passwd, pwd, authorization, privatekey, accesskey, credential or
// cookie, or one of sensitiveFields written the same way. sensitiveFields adds the fields that a
// route's body holds credentials in.
//
// A JSON document in which Scrub replaces something, or that has a key that names a credential,
// is encoded again, compact and with the keys of each object sorted; other data keeps its bytes,
// apart from the values replaced in it. When a member of set would still be in the result, as it
// is or written with JSON escapes, which happens only for a member that overlaps Marker's own text
// or has characters that JSON escapes, Scrub returns Marker alone, and a JSON string that would
// hold one becomes Marker.
func Scrub(set *Set, data []byte, sensitiveFields ...string) []byte {
	return newScrubber(set, sensitiveFields).scrub(data)
}

// ScrubString returns text with credentials replaced as [Scrub] replaces them.
func ScrubString(set *Set, text string) string {
	return string(Scrub(set, []byte(text)))
}

// scrubber holds what one call of Scrub replaces.
type scrubber struct {
	// secrets are the members of the Set, the longest first.
	secrets []string

	// written are secrets as JSON writes them in a string, in the same order.
	written [][]byte

	// names are the words that make an object key name a credential, normalized.
	names []string
}

func newScrubber(set *Set, sensitiveFields []string) *scrubber {
	names := slices.Clone(credentialNames)
	for _, field := range sensitiveFields {
		if name := normalize(field); name != "" {
			names = append(names, name)
		}
	}
	secrets := set.longestFirst()
	written := make([][]byte, 0, len(secrets))
	for _, secret := range secrets {
		written = append(written, jsonText(secret))
	}
	return &scrubber{secrets: secrets, written: written, names: names}
}

// normalize returns name in lower case without '-' and '_', so that api_key, API-Key and apiKey
// are the same name.
func normalize(name string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '_' {
			return -1
		}
		return unicode.ToLower(r)
	}, name)
}

// isCredential reports whether the object key names a credential.
func (s *scrubber) isCredential(key string) bool {
	key = normalize(key)
	return slices.ContainsFunc(s.names, func(name string) bool {
		return strings.Contains(key, name)
	})
}

// locNamesCredential reports whether loc, the decoded location of a validation error such as
// ["body", "secret"], has a string that names a credential.
func (s *scrubber) locNamesCredential(loc any) bool {
	parts, _ := loc.([]any)
	return slices.ContainsFunc(parts, func(part any) bool {
		name, ok := part.(string)
		return ok && s.isCredential(name)
	})
}

// scrub returns a copy of data with its credentials replaced, as Scrub describes.
func (s *scrubber) scrub(data []byte) []byte {
	out, onlyJSON := s.scrubJSON(data)
	if !onlyJSON {
		out = s.pairs(out)
	}
	out = s.text(out)
	for i, secret := range s.secrets {
		if bytes.Contains(out, []byte(secret)) || bytes.Contains(out, s.written[i]) {
			return []byte(Marker)
		}
	}
	return out
}

// text returns b with Authorization header values, Permit API keys and the members of the Set
// replaced.
func (s *scrubber) text(b []byte) []byte {
	b = authorizationHeader.ReplaceAll(b, []byte("${1}"+Marker))
	b = permitKey.ReplaceAllLiteral(b, []byte(Marker))
	for _, secret := range s.secrets {
		b = bytes.ReplaceAll(b, []byte(secret), []byte(Marker))
	}
	return b
}

// pairs returns b with the string of each "key": "value" pair whose key names a credential
// replaced.
func (s *scrubber) pairs(b []byte) []byte {
	return jsonPair.ReplaceAllFunc(b, func(pair []byte) []byte {
		m := jsonPair.FindSubmatch(pair)
		if !s.isCredential(string(m[1])) {
			return pair
		}
		return slices.Concat([]byte(`"`), m[1], []byte(`"`), m[2], []byte(`"`+Marker+`"`))
	})
}

// scrubJSON returns a copy of data in which each JSON document that data starts with has its
// credentials replaced, and the rest of data, from the first byte that does not start a JSON
// document, is kept. It reports whether data holds JSON documents and white space only.
func (s *scrubber) scrubJSON(data []byte) ([]byte, bool) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var out bytes.Buffer
	start := 0
	for {
		var value any
		if err := decoder.Decode(&value); err != nil {
			out.Write(data[start:])
			return out.Bytes(), errors.Is(err, io.EOF)
		}
		end := int(decoder.InputOffset())
		document := data[start:end]
		start = end
		value, changed := s.walk(value, false)
		if !changed {
			out.Write(document)
			continue
		}
		out.Write(document[:len(document)-len(bytes.TrimLeft(document, " \t\r\n"))])
		out.Write(encode(value))
	}
}

// encode returns value, a decoded JSON value, as compact JSON with the HTML characters kept.
func encode(value any) []byte {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		// A decoded value always encodes again; should one not, keep nothing of it.
		return []byte(`"` + Marker + `"`)
	}
	return bytes.TrimSuffix(out.Bytes(), []byte("\n"))
}

// jsonText returns text as encode writes it inside a JSON string, without the quotes.
func jsonText(text string) []byte {
	quoted := encode(text)
	return quoted[1 : len(quoted)-1]
}

// walk returns value, a decoded JSON value, with its credentials replaced, and whether it is to be
// encoded again: because it replaced something, or because value has an object key that names a
// credential, whose earlier duplicate in the bytes, which the decoder dropped, can hold one.
// hidden says that value is under a key that names a credential, so that its strings and numbers
// are replaced whatever they hold.
func (s *scrubber) walk(value any, hidden bool) (any, bool) {
	switch value := value.(type) {
	case string:
		if hidden {
			return Marker, true
		}
		scrubbed := string(s.scrub([]byte(value)))
		return scrubbed, scrubbed != value
	case json.Number:
		if hidden {
			return Marker, true
		}
		return value, false
	case []any:
		changed := false
		for i, element := range value {
			var c bool
			value[i], c = s.walk(element, hidden)
			changed = changed || c
		}
		return value, changed
	case map[string]any:
		echoesCredential := s.locNamesCredential(value["loc"])
		changed := false
		for key, element := range value {
			credential := s.isCredential(key) || key == "input" && echoesCredential
			var c bool
			// The key stays as decoded, so that two keys never become one; the text pass of
			// scrub replaces what it holds once the value is encoded again.
			value[key], c = s.walk(element, hidden || credential)
			changed = changed || c || credential || !bytes.Equal(s.text([]byte(key)), []byte(key))
		}
		return value, changed
	default:
		return value, false
	}
}
