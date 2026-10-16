package permit

import (
	"cmp"
	"context"
	"iter"
	"math"
	"strconv"

	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/internal/wire"
)

// ListOptions picks the page of a list that a List method returns, and the page that an All
// method starts at and the number of items it asks for on each page. The zero ListOptions starts
// at the first page, with the number of items on a page that PerPage gives for 0. A Page or a
// PerPage less than zero is an error that matches [ErrInvalidConfig], and the call sends no
// request.
type ListOptions struct {
	// Page is the number of the page, from 1; 0 is the first page.
	Page int

	// PerPage is the number of items on a page. With 0, a List method leaves it to the API's
	// default, and an All method asks for the largest number that the route accepts. The client
	// sends a PerPage that is not 0 as it is, without a limit of its own, also when it is above
	// the route's limit: the API may then reject the request, or give fewer items on a page.
	PerPage int
}

// AttributeFilter keeps the items of a list whose attribute has a value, such as the tenants
// whose tier is gold. A list sends it as the query parameter attr_<name>, with the value as
// text, and the API keeps the items whose attribute matches it; the API's spec does not list
// these parameters. Build one with [AttributeEquals]. The zero AttributeFilter is not valid, nor
// is a filter that names the attribute of an earlier filter of the same list: a call that gets
// one returns an error that matches [ErrInvalidConfig] and sends no request.
type AttributeFilter struct {
	// name is the name of the attribute, which the parameter's name holds after attr_.
	name string

	// value is the value as the query sends it.
	value string

	// notFinite says that the value is a float64 that is NaN or an infinity, which the API does
	// not read as a number.
	notFinite bool
}

// AttributeEquals returns the filter that keeps the items whose attribute name equals value: a
// string, a bool, an int, an int64 or a float64, such as AttributeEquals("tier", "gold") or
// AttributeEquals("seats", 10). The query sends a string as it is, a bool as true or false, an
// integer as its decimal digits, and a float64 as encoding/json writes it, such as 0.5, 1000000,
// 1e-7 or 1e+21. An empty name, and a float64 that is NaN or an infinity, make a filter that is
// not valid, as does a name that an earlier filter of the same list has, whatever the values: a
// call that gets it returns an error that matches [ErrInvalidConfig] and sends no request.
func AttributeEquals[T string | bool | int | int64 | float64](name string, value T,
) AttributeFilter {
	f := AttributeFilter{name: name}
	switch v := any(value).(type) {
	case string:
		f.value = v
	case bool:
		f.value = strconv.FormatBool(v)
	case int:
		f.value = strconv.Itoa(v)
	case int64:
		f.value = strconv.FormatInt(v, 10)
	case float64:
		f.value = wire.FormatFloat(v)
		f.notFinite = math.IsNaN(v) || math.IsInf(v, 0)
	}
	return f
}

// addAttributeFilters adds each of filters to query as the parameter attr_<name>, in order, for
// a call of the operation op. It returns the error of the first filter that is not valid, which
// names op and the filter's place in filters, and then adds none. It checks each filter against
// the rules in this order, and returns the first that it breaks: a name that is empty, a value
// that is NaN or an infinity, and a name that an earlier filter has.
func addAttributeFilters(query *wire.Query, op string, filters []AttributeFilter) error {
	seen := make(map[string]bool, len(filters))
	for i, f := range filters {
		setting := op + ": attribute filter " + strconv.Itoa(i+1)
		switch {
		case f.name == "":
			return &configError{setting: setting, rule: validate.ListAttributeFilterNameEmpty}
		case f.notFinite:
			return &configError{setting: setting, rule: validate.ListAttributeFilterNotFinite}
		case seen[f.name]:
			return &configError{setting: setting, rule: validate.ListAttributeFilterRepeated}
		}
		seen[f.name] = true
	}
	for _, f := range filters {
		query.AddString("attr_"+f.name, &f.value)
	}
	return nil
}

// listOptions returns a copy of the options that opts points to, or the zero ListOptions for a
// nil opts, for a List or an All method that takes a *ListOptions: a later change to opts
// changes no request.
func listOptions(opts *ListOptions) ListOptions {
	if opts == nil {
		return ListOptions{}
	}
	return *opts
}

// Page is one page of a list, as a List method returns it: its items, the page and the number of
// items that the call asked for, and the counts that the reply reports.
type Page[T any] struct {
	// Data holds the items of the page, in the order of the reply. It is empty, not nil, for a
	// page without items, such as one after the last page.
	Data []T

	// Page is the number of the page that the call asked for, from 1.
	Page int

	// PerPage is the number of items on a page that the call asked for, or 0 when it left the
	// number to the API's default.
	PerPage int

	// TotalCount is the number of items on all the pages, or nil when the reply does not report
	// it, such as a reply that is a JSON array.
	TotalCount *int

	// PageCount is the page_count of the reply as the API sends it, or nil when the reply has
	// none. The SDK does not use it to tell where a list ends.
	PageCount *int
}

// listPage makes the call of r, a request of a list whose replies have shape, for the page of
// opts, and returns the page. It adds page and per_page, each only when opts sets it, to a copy of
// the query of r, which stays as it is. It never sends a parameter that the API marks deprecated,
// such as include_total_count: the counts are those of the replies that report them. Before any
// request, it returns ErrClosed after Close, then an error that matches ErrInvalidConfig for a
// Page or a PerPage less than zero; then the errors of doList.
func listPage[T any](ctx context.Context, c *Client, r request, shape listShape,
	opts ListOptions,
) (*Page[T], error) {
	if err := c.checkOpen(); err != nil {
		return nil, err
	}
	if opts.Page < 0 {
		return nil, &configError{setting: r.op + ": ListOptions", rule: validate.ListPageNegative}
	}
	if opts.PerPage < 0 {
		return nil, &configError{
			setting: r.op + ": ListOptions", rule: validate.ListPerPageNegative,
		}
	}
	query := r.query.Clone()
	if opts.Page != 0 {
		query.AddInt("page", new(int64(opts.Page)))
	}
	if opts.PerPage != 0 {
		query.AddInt("per_page", new(int64(opts.PerPage)))
	}
	r.query = query
	list, err := doList[T](ctx, c, r, shape)
	if err != nil {
		return nil, err
	}
	return &Page[T]{
		Data:       list.Data,
		Page:       cmp.Or(opts.Page, 1),
		PerPage:    opts.PerPage,
		TotalCount: list.TotalCount,
		PageCount:  list.PageCount,
	}, nil
}

// all returns an iterator over the items of every page of the list that r requests, whose
// replies have shape, from the page of opts on, or the first. Each page asks for the PerPage of
// opts, or else for the MaxPerPage of the operation of r; an operation that the client's registry
// does not hold fails at the first page. The iterator stops after a page without items, or, when
// the loop started at the first page, after the page whose reply reports a total_count equal to
// the number of items that the loop has read up to the end of that page. A loop that started at
// a later page cannot tell how many items the pages before it hold, so it reads on to the page
// without items, as does a loop whose reply reports a total_count below the items read. It never
// stops because a page holds fewer items than it asked for, as the API may give fewer than that
// on every page, nor because of a page_count. It relies on the route to honour page: a route that
// answered every page with the same items could be read until a call fails or ctx ends. It asks
// for the next page only when the loop wants an item past the last item of a page, so a loop
// that stops early sends no other request.
//
// Each page is a call of listPage under ctx, with its own timeout, retries and Debug lines. When
// a call fails, such as when ctx ends, the iterator yields its error once, with the zero T, and
// stops. Each loop over the iterator reads the list again, from the page of opts.
func all[T any](ctx context.Context, c *Client, r request, shape listShape,
	opts ListOptions,
) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		op, _ := c.registry.Op(r.op)
		next := ListOptions{
			Page:    cmp.Or(opts.Page, 1),
			PerPage: cmp.Or(opts.PerPage, op.MaxPerPage),
		}
		fromFirst, read := next.Page == 1, 0
		for {
			page, err := listPage[T](ctx, c, r, shape, next)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			for _, item := range page.Data {
				if !yield(item, nil) {
					return
				}
			}
			if len(page.Data) == 0 {
				return
			}
			read += len(page.Data)
			if fromFirst && page.TotalCount != nil && *page.TotalCount == read {
				return
			}
			next.Page++
		}
	}
}

// allInScope returns the iterator of all over the list that r requests, for an operation whose
// route's placeholders start with {proj_id} and {env_id}, which inScope fills. Each loop gets
// the scope, as inScope does, before its first page, not when allInScope is called; when inScope
// fails, the iterator yields its error once, with the zero T, and stops.
func allInScope[T any](ctx context.Context, c *Client, r request, shape listShape,
	opts ListOptions,
) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		scoped, err := c.inScope(ctx, r)
		if err != nil {
			var zero T
			yield(zero, err)
			return
		}
		for item, err := range all[T](ctx, c, scoped, shape, opts) {
			if !yield(item, err) {
				return
			}
		}
	}
}
