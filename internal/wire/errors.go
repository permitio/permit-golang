// Package wire writes what the SDK sends to the Permit API and the PDP, and reads what they
// answer, one way for each shape: the bodies of requests with [Encode], their paths with [Path]
// and their queries with [Query]; the bodies of replies with [Decode], those that hold a list
// with [DecodeList], and those of error replies with [DecodeErrorBody].
package wire

import (
	"encoding/json"
	"strconv"
	"strings"
)

// maxSummaryErrors is the most validation errors that [ErrorBody.Summary] lists.
const maxSummaryErrors = 10

// ErrorBody is what the body of an error reply says, as [DecodeErrorBody] reads it. Each field is
// empty when the body does not have it.
type ErrorBody struct {
	// ID, Title, ErrorCode and Message are the fields id, title, error_code and message of the
	// API's error body, the ErrorDetails schema of its spec.
	ID        string
	Title     string
	ErrorCode string
	Message   string

	// Detail is the field detail when it is a string, as in {"detail": "Not Found"}.
	Detail string

	// Validation is the field detail when it is a list, as in a validation error body (the
	// HTTPValidationError schema of the spec): the fields that failed, each with its message.
	Validation []ValidationError
}

// ValidationError is one entry of the detail list of a validation error body, the
// ValidationError schema of the spec.
type ValidationError struct {
	// Loc is the location of the field that failed, such as ["body", "email"]: strings and
	// integers, each as JSON text.
	Loc []json.RawMessage `json:"loc"`

	// Msg says why the field failed.
	Msg string `json:"msg"`

	// Type names the kind of failure, such as value_error.missing.
	Type string `json:"type"`
}

// DecodeErrorBody reads body, the body of an error reply, as a JSON object. It returns false when
// body is not one, such as an empty, a text or an HTML body. Unknown fields are ignored and a
// known field of another type is left empty. An entry of a detail list that is not a
// ValidationError with a message, such as null, is dropped.
func DecodeErrorBody(body []byte) (ErrorBody, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return ErrorBody{}, false
	}
	b := ErrorBody{
		ID:        stringField(fields["id"]),
		Title:     stringField(fields["title"]),
		ErrorCode: stringField(fields["error_code"]),
		Message:   stringField(fields["message"]),
		Detail:    stringField(fields["detail"]),
	}
	var entries []json.RawMessage
	if json.Unmarshal(fields["detail"], &entries) == nil {
		for _, entry := range entries {
			var v ValidationError
			if json.Unmarshal(entry, &v) == nil && v.Msg != "" {
				b.Validation = append(b.Validation, v)
			}
		}
	}
	return b, true
}

// stringField returns the JSON string raw, or "" when raw is missing or not a string.
func stringField(raw json.RawMessage) string {
	s, _ := jsonString(raw)
	return s
}

// jsonString returns the JSON string raw, and false when raw is not a string.
func jsonString(raw json.RawMessage) (string, bool) {
	var s *string
	if json.Unmarshal(raw, &s) != nil || s == nil {
		return "", false
	}
	return *s, true
}

// Summary returns what the body says went wrong, on one line: Message, else Detail, else the
// first 10 validation errors, each as its location and message, such as "body.email: value is
// not a valid email address", and then how many more there are, such as "and 4 more", joined by
// "; ". It returns "" when the body says none of these.
func (b ErrorBody) Summary() string {
	if b.Message != "" {
		return b.Message
	}
	if b.Detail != "" {
		return b.Detail
	}
	listed := b.Validation[:min(len(b.Validation), maxSummaryErrors)]
	parts := make([]string, 0, len(listed)+1)
	for _, v := range listed {
		parts = append(parts, v.Location()+": "+v.Msg)
	}
	if more := len(b.Validation) - len(listed); more > 0 {
		parts = append(parts, "and "+strconv.Itoa(more)+" more")
	}
	return strings.Join(parts, "; ")
}

// Location returns Loc joined by dots, such as body.email or body.0.key: each string without its
// quotes, each other value as its JSON text.
func (v ValidationError) Location() string {
	parts := make([]string, 0, len(v.Loc))
	for _, raw := range v.Loc {
		if s, ok := jsonString(raw); ok {
			parts = append(parts, s)
			continue
		}
		parts = append(parts, string(raw))
	}
	return strings.Join(parts, ".")
}
