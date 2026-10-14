package permit

import (
	"cmp"
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/internal/validate"
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

// Page is one page of a list, as a List method returns it: its items, the page and the number of
// items that the call asked for, and the counts of the whole list that the reply reports.
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

	// PageCount is the number of pages, or nil when the reply does not report it.
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
// does not hold fails at the first page. The iterator stops after a page without items, or after
// the page whose number is the page_count that its reply reports; never after a page with fewer
// items than it asked for, as the API may give fewer than that on every page. It relies on the
// route to honour page: a route that answered every page with the same items would be read until
// a call fails or ctx ends. It asks for the next page only when the loop wants an item past the
// last item of a page, so a loop that stops early sends no other request.
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
			if len(page.Data) == 0 || page.PageCount != nil && next.Page >= *page.PageCount {
				return
			}
			next.Page++
		}
	}
}
