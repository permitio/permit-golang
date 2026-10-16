package wire

import "errors"

// List is a list that a reply holds, as [DecodeList] reads it: the items, and the counts that a
// paginated envelope reports.
type List[T any] struct {
	// Data holds the items, in the order of the reply; it is empty, not nil, for an empty list.
	Data []T

	// TotalCount is the envelope's total_count, the number of items of every page, or nil when
	// the reply does not report it, such as a bare array.
	TotalCount *int

	// PageCount is the envelope's page_count as the reply sends it, or nil when the reply does
	// not report it.
	PageCount *int
}

// envelope is the paginated envelope of a list, the PaginatedResult schemas of the API's spec.
type envelope[T any] struct {
	Data       *[]T `json:"data"`
	TotalCount *int `json:"total_count"`
	PageCount  *int `json:"page_count"`
}

// DecodeList reads body, the reply of a route that answers with a list, by its first byte that is
// not white space: [ starts a bare array of items, and { a paginated envelope, an object whose
// data field holds the array and whose total_count and page_count fields, when it has them, hold
// the counts. The API answers some list routes in either shape, so the shape decides, never a
// trial decode. DecodeList decodes as [Decode] does, and fails on any other body, such as null
// or an empty one, and on an object without an array in data, such as a single object where a
// list was expected.
func DecodeList[T any](body []byte) (List[T], error) {
	switch firstByte(body) {
	case '[':
		var data []T
		if err := Decode(body, &data); err != nil {
			return List[T]{}, err
		}
		return List[T]{Data: data}, nil
	case '{':
		var env envelope[T]
		if err := Decode(body, &env); err != nil {
			return List[T]{}, err
		}
		if env.Data == nil {
			return List[T]{}, errNoData
		}
		return List[T]{Data: *env.Data, TotalCount: env.TotalCount, PageCount: env.PageCount}, nil
	default:
		return List[T]{}, errNotAList
	}
}

// The errors of a list reply that is not a list.
var (
	errNotAList = errors.New("the reply is neither a JSON array nor a paginated object")
	errNoData   = errors.New("the reply is an object without a data array, not a page of a list")
)

// firstByte returns the first byte of data that is not JSON white space, or 0 when there is none.
func firstByte(data []byte) byte {
	for _, b := range data {
		switch b {
		case ' ', '\t', '\r', '\n':
		default:
			return b
		}
	}
	return 0
}
