// Package redact removes credentials from text that the SDK prints, logs or keeps in errors.
package redact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Marker is the text that replaces each credential this package removes.
const Marker = "[REDACTED]"

// minKeyLength is the length a configured API key must have before Text and Body replace it
// wherever it appears, so that a short placeholder key, such as one used in a test, does not
// replace unrelated text. Permit API keys are much longer. Values that start with permit_key_ are
// replaced whatever their length.
const minKeyLength = 16

// maxJSONDepth is the deepest nesting of JSON objects and arrays that Body decodes. A body nested
// deeper is replaced by Marker as a whole, so it is never returned with its credentials in it.
const maxJSONDepth = 1000

var permitKeyPattern = regexp.MustCompile(`permit_key_[A-Za-z0-9_-]+`)

// credentialNames are compared with a JSON key or header name after it is lowercased and its '-'
// and '_' characters are removed. A name names a credential when it ends with one of them, so
// "access_token", "client_secret", "X-API-Key", "aws_secret_access_key" and "Set-Cookie" all do.
// A bare "key" is not one of them: it would match names such as "resource_key".
var credentialNames = []string{
	"secret", "token", "apikey", "password", "passwd", "pwd", "authorization", "secretkey",
	"privatekey", "accesskey", "credential", "credentials", "cookie",
}

var nameSeparators = strings.NewReplacer("-", "", "_", "")

// IsCredentialName reports whether a JSON key or header name names a credential.
func IsCredentialName(name string) bool {
	normalized := strings.ToLower(nameSeparators.Replace(name))
	for _, credentialName := range credentialNames {
		if strings.HasSuffix(normalized, credentialName) {
			return true
		}
	}
	return false
}

// Secret returns Marker for a non-empty credential and "" for an empty one, so printed
// configuration still shows whether a credential is set.
func Secret(credential string) string {
	if credential == "" {
		return ""
	}
	return Marker
}

// Headers returns a copy of headers in which the non-empty value of each header whose name names
// a credential, such as Authorization, is Marker. It returns nil for a nil map.
func Headers(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	redacted := make(map[string]string, len(headers))
	for name, value := range headers {
		if IsCredentialName(name) {
			value = Secret(value)
		}
		redacted[name] = value
	}
	return redacted
}

// Text returns s with every value that starts with permit_key_, and key when it has at least 16
// characters, replaced by Marker.
func Text(s string, key string) string {
	if len(key) >= minKeyLength {
		s = strings.ReplaceAll(s, key, Marker)
	}
	return permitKeyPattern.ReplaceAllLiteralString(s, Marker)
}

// Body returns body with credentials replaced by Marker. First key and permit_key_ values are
// replaced in all of body, as Text does. Then, in each JSON document that body starts with, such
// as one document or JSON lines, every non-empty string and every number under a key that names a
// credential is replaced, at any depth: the value itself, or each one inside an object or array.
// So is key or a permit_key_ value that a string or an object key holds written with JSON
// escapes. A document in which this replaces something, or that has a key naming a credential,
// is encoded again, compact and with the keys of each object sorted; any other document, and any
// text after the documents, keeps its bytes. A body with objects or arrays nested more than 1000
// deep is replaced by Marker as a whole.
func Body(body string, key string) string {
	if nestedDeeperThan(body, maxJSONDepth) {
		return Marker
	}
	redacted, ok := redactJSONDocuments(Text(body, key), key)
	if !ok {
		return Marker
	}
	return redacted
}

// nestedDeeperThan reports whether body has more than limit objects or arrays open at once,
// counting the brackets outside JSON strings.
func nestedDeeperThan(body string, limit int) bool {
	depth := 0
	inString, escaped := false, false
	for i := 0; i < len(body); i++ {
		switch c := body[i]; {
		case escaped:
			escaped = false
		case inString && c == '\\':
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
		case c == '{' || c == '[':
			depth++
			if depth > limit {
				return true
			}
		case c == '}' || c == ']':
			depth--
		}
	}
	return false
}

// redactJSONDocuments returns body with the JSON documents at its start redacted as Body
// describes, and the text from the first part that is not a JSON document on unchanged. It
// returns false when a redacted document cannot be encoded again.
func redactJSONDocuments(body string, key string) (string, bool) {
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.UseNumber()
	var redacted strings.Builder
	start := 0
	for {
		var document interface{}
		if err := decoder.Decode(&document); err != nil {
			break
		}
		end := int(decoder.InputOffset())
		segment := body[start:end]
		start = end
		document, encodeAgain := redactJSON(document, false, key)
		if !encodeAgain {
			redacted.WriteString(segment)
			continue
		}
		encoded, err := encodeJSON(document)
		if err != nil {
			// Values decoded by encoding/json always encode again. Should that ever fail, the
			// caller hides the whole body rather than keep a credential.
			return "", false
		}
		redacted.WriteString(segment[:len(segment)-len(strings.TrimLeft(segment, " \t\r\n"))])
		redacted.WriteString(encoded)
	}
	redacted.WriteString(body[start:])
	return redacted.String(), true
}

func encodeJSON(document interface{}) (string, error) {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(document); err != nil {
		return "", err
	}
	return strings.TrimSuffix(encoded.String(), "\n"), nil
}

// redactJSON replaces in value the non-empty strings and the numbers that are under a key naming
// a credential, and key and permit_key_ values in the other strings and in object keys: Text
// could not find those in the bytes because they were written with JSON escapes, which the
// decoder has removed. underCredential is true when value itself is under a key naming a
// credential. It reports whether value must be encoded again: when it replaced something, or when
// value has a key that names a credential even if its value is kept, because the bytes can hold
// an earlier duplicate of that key, with a credential, that the decoder dropped.
func redactJSON(value interface{}, underCredential bool, key string) (interface{}, bool) {
	switch typed := value.(type) {
	case map[string]interface{}:
		redacted := make(map[string]interface{}, len(typed))
		encodeAgain := false
		for name, item := range typed {
			credential := IsCredentialName(name)
			item, itemEncodeAgain := redactJSON(item, underCredential || credential, key)
			redactedName := Text(name, key)
			redacted[redactedName] = item
			encodeAgain = encodeAgain || itemEncodeAgain || credential || redactedName != name
		}
		return redacted, encodeAgain
	case []interface{}:
		encodeAgain := false
		for i, item := range typed {
			redacted, itemEncodeAgain := redactJSON(item, underCredential, key)
			typed[i] = redacted
			encodeAgain = encodeAgain || itemEncodeAgain
		}
		return typed, encodeAgain
	case string:
		if underCredential && typed != "" {
			return Marker, true
		}
		redacted := Text(typed, key)
		return redacted, redacted != typed
	case json.Number:
		if underCredential {
			return Marker, true
		}
		return typed, false
	default:
		return typed, false
	}
}

// FormatDirective returns the fmt directive, such as "%+v" or "%-8.3q", that a Format method was
// called with, so that it can print a substitute value the way the caller asked for.
func FormatDirective(f fmt.State, verb rune) string {
	var directive strings.Builder
	directive.WriteByte('%')
	for _, flag := range "+-# 0" {
		if f.Flag(int(flag)) {
			directive.WriteRune(flag)
		}
	}
	if width, ok := f.Width(); ok {
		directive.WriteString(strconv.Itoa(width))
	}
	if precision, ok := f.Precision(); ok {
		directive.WriteByte('.')
		directive.WriteString(strconv.Itoa(precision))
	}
	directive.WriteRune(verb)
	return directive.String()
}
