package wire

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// errEmptyBody is the error of a body that holds no JSON value.
var errEmptyBody = errors.New("the body is empty")

// errTrailingData is the error of a body that holds more than one JSON value.
var errTrailingData = errors.New("the body holds more than one JSON value")

// Encode returns v as JSON, the body of a request. It is the one encoder of the SDK's request
// bodies: encoding/json's encoding, with <, > and & left as they are rather than escaped as
// \u003c, \u003e and \u0026, and without a newline at the end. encoding/json still escapes U+2028
// and U+2029, which a JSON reader reads back as the same characters.
func Encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// Decode reads data, the body of a reply, as one JSON value into v. A number that v holds as an
// any, such as a value of a map[string]any, becomes a json.Number with every digit of the text,
// rather than a float64, which cannot hold every integer beyond 2^53. Unknown fields are ignored.
// Decode fails on an empty body and on a body that holds anything but white space after the value.
func Decode(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return errEmptyBody
		}
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errTrailingData
	}
	return nil
}
