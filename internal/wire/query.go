package wire

import (
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// deprecatedParams are the query parameters that the API's spec marks deprecated, on every
// operation that has them, sorted. A Query never sends them. TestRepositoryDeprecatedParams, in
// the tools module, checks this list against the specs.
var deprecatedParams = []string{"detailed", "include_total_count", "return_deleted"}

// DeprecatedParams returns the query parameters that a Query never sends, sorted.
func DeprecatedParams() []string {
	return slices.Clone(deprecatedParams)
}

// Query is the query of a request. Each Add method adds a parameter of one type, and leaves out
// a parameter that is not set, a nil pointer or an empty list, so that a filter the caller did
// not set is never sent. A parameter that the API marks deprecated, one of [DeprecatedParams],
// is never sent either. The zero Query is empty and ready to use.
type Query struct {
	values url.Values
}

// AddBool adds the parameter name as true or false, unless value is nil.
func (q *Query) AddBool(name string, value *bool) {
	if value != nil {
		q.add(name, strconv.FormatBool(*value))
	}
}

// AddInt adds the parameter name as the decimal digits of value, unless value is nil.
func (q *Query) AddInt(name string, value *int64) {
	if value != nil {
		q.add(name, strconv.FormatInt(*value, 10))
	}
}

// AddFloat adds the parameter name as the text that [FormatFloat] gives for value, unless value
// is nil.
func (q *Query) AddFloat(name string, value *float64) {
	if value != nil {
		q.add(name, FormatFloat(*value))
	}
}

// AddString adds the parameter name with value, unless value is nil. A pointer to the empty
// string adds the parameter with an empty value.
func (q *Query) AddString(name string, value *string) {
	if value != nil {
		q.add(name, *value)
	}
}

// AddStrings adds the parameter name once for each of values, in order, such as user=a&user=b,
// the form the API reads a list in. An empty list adds nothing.
func (q *Query) AddStrings(name string, values []string) {
	for _, v := range values {
		q.add(name, v)
	}
}

// add adds the parameter name with value, unless the API marks name deprecated.
func (q *Query) add(name, value string) {
	if slices.Contains(deprecatedParams, name) {
		return
	}
	if q.values == nil {
		q.values = url.Values{}
	}
	q.values.Add(name, value)
}

// Encode returns the query without the "?": its parameters sorted by name, the values of one
// name in the order they were added, each name and value escaped with url.QueryEscape. It
// returns "" for an empty query.
func (q *Query) Encode() string {
	return q.values.Encode()
}

// FormatFloat returns f as a query sends it, the text that encoding/json writes for it: the
// shortest decimal that reads back as the same float64, such as 0.1 or 9007199254740992, and in
// e notation below 1e-6 and from 1e21 on, such as 1e-7 or 1e+300. It formats f with 64 bits, so
// no digit of a float64 is lost. NaN and the infinities, which JSON cannot hold, are NaN, +Inf
// and -Inf, as strconv writes them, which the API does not read as numbers: a method that takes
// a float filter must reject them before it sends the request.
func FormatFloat(f float64) string {
	abs := math.Abs(f)
	if abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		s := strconv.FormatFloat(f, 'e', -1, 64)
		// strconv writes an exponent below 10 with two digits, such as 1e-07; JSON, one.
		if mantissa, exponent, ok := strings.Cut(s, "e-0"); ok {
			return mantissa + "e-" + exponent
		}
		return s
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
