package mockpermit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
)

// ExpectJSON fails the test unless the request body is the same JSON value as want, compared
// exactly. Object key order and whitespace do not matter; a missing, extra or changed field
// does, and a null is not a missing field. Numbers compare as written, so 9007199254740993 does
// not match 9007199254740992, 1.0 does not match 1, and -0 does not match 0. Strings compare
// decoded, so "\u003c" matches "<"; ExpectBytes compares the bytes. An escaped lone surrogate,
// such as "\ud800", decodes to U+FFFD, so ExpectJSON cannot tell lone surrogates apart from each
// other or from U+FFFD, and a test of them uses ExpectBytes. A body or want that is not valid
// UTF-8, is not one JSON value or repeats a key of an object fails the test too. The failure
// shows the difference.
func (r Request) ExpectJSON(tb testing.TB, want string) {
	tb.Helper()
	wantValue, err := decodeExact([]byte(want))
	if err != nil {
		tb.Errorf("mockpermit: ExpectJSON: want is not exact JSON: %v", err)
		return
	}
	got, err := decodeExact(r.Body)
	if err != nil {
		tb.Errorf("mockpermit: %s %s: the body is not exact JSON: %v\nbody: %q", r.Method,
			r.RawPath, err, r.Body)
		return
	}
	if diff := cmp.Diff(wantValue, got); diff != "" {
		tb.Errorf("mockpermit: %s %s: the body differs (-want +got):\n%s", r.Method, r.RawPath,
			diff)
	}
}

// ExpectBytes fails the test unless the request body is byte for byte the golden file less its
// final newline: a golden file holds the body and then one newline, as .editorconfig asks of
// every file. golden is a path relative to the test's package directory, such as
// testdata/create_user.golden. The failure shows the difference and the body.
func (r Request) ExpectBytes(tb testing.TB, golden string) {
	tb.Helper()
	data, err := os.ReadFile(golden) //nolint:gosec // G304: the test names its own golden file.
	if err != nil {
		tb.Errorf("mockpermit: ExpectBytes: %v", err)
		return
	}
	want, ok := bytes.CutSuffix(data, []byte("\n"))
	if !ok {
		tb.Errorf("mockpermit: ExpectBytes: %s does not end with a newline; a golden file holds "+
			"the body and then one newline", golden)
		return
	}
	if diff := cmp.Diff(string(want), string(r.Body)); diff != "" {
		tb.Errorf("mockpermit: %s %s: the body differs from %s (-want +got):\n%s\nbody: %q",
			r.Method, r.RawPath, golden, diff, r.Body)
	}
}

// decodeExact decodes data, which must be one JSON value in valid UTF-8 whose objects repeat no
// key, keeping each number as the json.Number it is written as.
func decodeExact(data []byte) (any, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("it is not valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := decodeValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("it has more after the first JSON value")
	}
	return value, nil
}

// decodeValue decodes the next JSON value from decoder: an object as a map[string]any, an array
// as a []any, and anything else as the json.Decoder token, such as a json.Number.
func decodeValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	switch {
	case !ok:
		return token, nil
	case delim == '[':
		return decodeArray(decoder)
	default:
		return decodeObject(decoder)
	}
}

// decodeArray decodes the rest of a JSON array after its "[".
func decodeArray(decoder *json.Decoder) (any, error) {
	values := []any{}
	for decoder.More() {
		value, err := decodeValue(decoder)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return values, nil
}

// decodeObject decodes the rest of a JSON object after its "{".
func decodeObject(decoder *json.Decoder) (any, error) {
	object := map[string]any{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("an object key is %v, not a string", token)
		}
		if _, repeated := object[key]; repeated {
			return nil, fmt.Errorf("an object repeats the key %q", key)
		}
		value, err := decodeValue(decoder)
		if err != nil {
			return nil, err
		}
		object[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return object, nil
}
