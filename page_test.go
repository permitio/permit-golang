package permit

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"math"
	"net/http"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/rest"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/internal/wire"
)

// opPaged is the test-only list operation of the page tests: its largest per_page is 30.
var opPaged = rest.Op{
	ID: "test_paged", Method: http.MethodGet, Route: rest.TestPrefix + "paged",
	Idempotent: true, KeyLevel: rest.EnvironmentLevel, MaxPerPage: 30,
	ResponseModel: "TestObject",
}

// paged is the request of opPaged, without a query.
var paged = request{op: opPaged.ID, target: apiTarget}

// pageClient returns a test client with opPaged whose URLs point at api, with mockpermit's API
// key and opts.
func pageClient(t *testing.T, api *mockpermit.Server, opts ...Option) *Client {
	t.Helper()
	clearEnv(t)
	base := []Option{WithAPIKey(mockpermit.APIKey), WithAPIURL(api.URL), WithPDPURL(api.URL)}
	return newTestClient(t, append(base, opts...), opPaged)
}

// objects returns the test objects k<from> to k<to-1>, an empty list when to is from.
func objects(from, to int) []testObject {
	items := make([]testObject, 0, to-from)
	for i := from; i < to; i++ {
		items = append(items, testObject{Key: "k" + strconv.Itoa(i)})
	}
	return items
}

// arrayOf returns a reply whose body is the JSON array of items, of any type of item, for the
// tests of every list.
func arrayOf[T any](t *testing.T, items []T) mockpermit.Reply {
	t.Helper()
	body, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	return mockpermit.Reply{Body: string(body)}
}

// pageOf returns a reply whose body is the paginated object that holds items in data, of any type
// of item, followed by counts, the members of the counts, such as `,"page_count":2`.
func pageOf[T any](t *testing.T, items []T, counts string) mockpermit.Reply {
	t.Helper()
	return mockpermit.Reply{Body: `{"data":` + arrayOf(t, items).Body + counts + `}`}
}

// collect ranges over seq, the iterator of an All method or of all, to its end, and returns the
// items and the errors it yields, for the tests of every iterator. It fails the test when seq
// yields an item that is not the zero T with an error, or anything after an error.
func collect[T any](t *testing.T, seq iter.Seq2[T, error]) ([]T, []error) {
	t.Helper()
	var (
		items []T
		errs  []error
	)
	for item, err := range seq {
		if len(errs) > 0 {
			t.Errorf("the iterator yielded %+v and %v after the error %v", item, err, errs[0])
		}
		if err != nil {
			if !reflect.ValueOf(&item).Elem().IsZero() {
				t.Errorf("the iterator yielded the item %+v with the error %v", item, err)
			}
			errs = append(errs, err)
			continue
		}
		items = append(items, item)
	}
	return items, errs
}

// queries returns the query of each request that server received, in order.
func queries(server *mockpermit.Server) []string {
	var got []string
	for _, r := range server.Requests() {
		got = append(got, r.RawQuery)
	}
	return got
}

// TestAllReadsEveryPage checks that all reads the 31 items of a list on a route whose largest
// page holds 30, asking for each page in turn with that per_page and nothing else in the query:
// up to the first empty page when the replies do not report page_count, a bare array or a
// paginated object, and up to the last page when they do.
func TestAllReadsEveryPage(t *testing.T) {
	untilEmpty := []string{"page=1&per_page=30", "page=2&per_page=30", "page=3&per_page=30"}
	for _, tc := range []struct {
		name    string
		replies []mockpermit.Reply
		want    []string
	}{
		{
			name: "arrays",
			replies: []mockpermit.Reply{
				arrayOf(t, objects(0, 30)), arrayOf(t, objects(30, 31)), arrayOf(t, objects(0, 0)),
			},
			want: untilEmpty,
		},
		{
			name: "pages without page_count",
			replies: []mockpermit.Reply{
				pageOf(t, objects(0, 30), `,"total_count":31`), pageOf(t, objects(30, 31), ""),
				pageOf(t, objects(0, 0), ""),
			},
			want: untilEmpty,
		},
		{
			name: "pages with page_count",
			replies: []mockpermit.Reply{
				pageOf(t, objects(0, 30), `,"total_count":31,"page_count":2`),
				pageOf(t, objects(30, 31), `,"total_count":31,"page_count":2`),
			},
			want: []string{"page=1&per_page=30", "page=2&per_page=30"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(route(opPaged), tc.replies...)
			c := pageClient(t, api)
			items, errs := collect(t, all[testObject](t.Context(), c, paged, arrayOrPageShape,
				ListOptions{}))
			if diff := cmp.Diff(objects(0, 31), items); diff != "" || errs != nil {
				t.Errorf("all yielded the errors %v and items that differ (-want +got):\n%s",
					errs, diff)
			}
			if diff := cmp.Diff(tc.want, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestAllStartsWhereTheOptionsSay checks that all starts at the page of the options, asks for
// their per_page as given, above the route's largest too, and reads on from there.
func TestAllStartsWhereTheOptionsSay(t *testing.T) {
	for _, tc := range []struct {
		opts ListOptions
		want []string
	}{
		{ListOptions{Page: 3, PerPage: 5}, []string{"page=3&per_page=5", "page=4&per_page=5"}},
		{ListOptions{PerPage: 1000}, []string{"page=1&per_page=1000", "page=2&per_page=1000"}},
		{ListOptions{Page: 2}, []string{"page=2&per_page=30", "page=3&per_page=30"}},
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(route(opPaged), arrayOf(t, objects(0, 5)), arrayOf(t, objects(0, 0)))
		c := pageClient(t, api)
		items, errs := collect(t, all[testObject](t.Context(), c, paged, arrayShape, tc.opts))
		if len(items) != 5 || errs != nil {
			t.Errorf("%+v: all yielded %d items and the errors %v, want 5 items", tc.opts,
				len(items), errs)
		}
		if diff := cmp.Diff(tc.want, queries(api)); diff != "" {
			t.Errorf("%+v: the queries differ (-want +got):\n%s", tc.opts, diff)
		}
	}
}

// TestAllReadsPastAShortPage checks that a page with fewer items than all asked for does not end
// the list: a server that gives at most 10 items when asked for 100 still yields every item.
func TestAllReadsPastAShortPage(t *testing.T) {
	api := mockpermit.NewAPI(t)
	api.Handle(route(opPaged), arrayOf(t, objects(0, 10)), arrayOf(t, objects(10, 20)),
		arrayOf(t, objects(20, 25)), arrayOf(t, objects(0, 0)))
	c := pageClient(t, api)
	items, errs := collect(t, all[testObject](t.Context(), c, paged, arrayShape,
		ListOptions{PerPage: 100}))
	if diff := cmp.Diff(objects(0, 25), items); diff != "" || errs != nil {
		t.Errorf("all yielded the errors %v and items that differ (-want +got):\n%s", errs, diff)
	}
	want := []string{
		"page=1&per_page=100", "page=2&per_page=100", "page=3&per_page=100", "page=4&per_page=100",
	}
	if diff := cmp.Diff(want, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestAllYieldsAnErrorOnce checks that a page that fails ends the list: all yields the items of
// the pages before it, then the error once with no item, and sends no other request. The client
// sends each request once, so that the failure of page 2 is the call's result.
func TestAllYieldsAnErrorOnce(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply mockpermit.Reply
		want  string
	}{
		{
			name: "an error status",
			reply: mockpermit.Reply{
				Status: http.StatusForbidden,
				Body:   `{"error_code":"FORBIDDEN","title":"Forbidden","message":"no access"}`,
			},
			want: "permit: test_paged: GET /__test/paged: status 403 FORBIDDEN: no access",
		},
		{
			name:  "an object without data",
			reply: mockpermit.Reply{Body: `{"total_count":31,"page_count":2}`},
			want: "permit: test_paged: GET /__test/paged: status 200: the reply is not a valid " +
				"list of models.TestObject",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(route(opPaged), arrayOf(t, objects(0, 30)), tc.reply)
			c := pageClient(t, api, WithRetry(RetryPolicy{}))
			items, errs := collect(t, all[testObject](t.Context(), c, paged, arrayOrPageShape,
				ListOptions{}))
			if diff := cmp.Diff(objects(0, 30), items); diff != "" {
				t.Errorf("all yielded items that differ (-want +got):\n%s", diff)
			}
			if len(errs) != 1 || mustError(t, errs[0]).Error() != tc.want {
				t.Errorf("all yielded the errors %v, want one: %s", errs, tc.want)
			}
			want := []string{"page=1&per_page=30", "page=2&per_page=30"}
			if diff := cmp.Diff(want, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestAllOfAnUnknownOperation checks that all of an operation that the client's registry does
// not hold yields the error of its first page once, and sends nothing.
func TestAllOfAnUnknownOperation(t *testing.T) {
	api := mockpermit.NewAPI(t)
	r := request{op: "test_unknown", target: apiTarget}
	items, errs := collect(t, all[testObject](t.Context(), pageClient(t, api), r, arrayShape,
		ListOptions{}))
	want := "permit: the operation test_unknown is not in the client's operation registry"
	if items != nil || len(errs) != 1 || errs[0].Error() != want {
		t.Errorf("all yielded %v and the errors %v, want one error: %s", items, errs, want)
	}
	api.AssertUntouched()
}

// TestAllSendsNoRequestAfterABreak checks that a loop that stops early sends no request for a
// page it did not reach, also when it stops at the last item of a page: all reads no page ahead.
func TestAllSendsNoRequestAfterABreak(t *testing.T) {
	for _, stopAt := range []int{1, 30} {
		api := mockpermit.NewAPI(t)
		api.Handle(route(opPaged), arrayOf(t, objects(0, 30)))
		c := pageClient(t, api)
		read := 0
		for _, err := range all[testObject](t.Context(), c, paged, arrayShape, ListOptions{}) {
			if err != nil {
				t.Fatalf("all yielded %v", err)
			}
			read++
			if read == stopAt {
				break
			}
		}
		if read != stopAt {
			t.Errorf("a loop that stops at item %d read %d items", stopAt, read)
		}
		if diff := cmp.Diff([]string{"page=1&per_page=30"}, queries(api)); diff != "" {
			t.Errorf("a loop that stops at item %d sent queries that differ (-want +got):\n%s",
				stopAt, diff)
		}
	}
}

// TestAllStopsWhenTheContextEnds checks that a context that ends between two pages ends the
// list with an error that matches the context's error, and that the next page is not sent.
func TestAllStopsWhenTheContextEnds(t *testing.T) {
	api := mockpermit.NewAPI(t)
	api.Handle(route(opPaged), arrayOf(t, objects(0, 30)))
	c := pageClient(t, api)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var read int
	var errs []error
	for _, err := range all[testObject](ctx, c, paged, arrayShape, ListOptions{}) {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		read++
		if read == 30 {
			cancel()
		}
	}
	if read != 30 || len(errs) != 1 || !errors.Is(errs[0], context.Canceled) {
		t.Errorf("all yielded %d items and the errors %v, want 30 items and context.Canceled",
			read, errs)
	}
	if diff := cmp.Diff([]string{"page=1&per_page=30"}, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestAllGivesEachPageItsOwnTimeout checks that the client's timeout bounds each page, not the
// whole loop: a loop that spends longer than the timeout on the items of a page still reads the
// next page.
func TestAllGivesEachPageItsOwnTimeout(t *testing.T) {
	const timeout = 500 * time.Millisecond
	api := mockpermit.NewAPI(t)
	api.Handle(route(opPaged), arrayOf(t, objects(0, 1)), arrayOf(t, objects(1, 2)),
		arrayOf(t, objects(0, 0)))
	c := pageClient(t, api, WithTimeout(timeout))
	var items []testObject
	for item, err := range all[testObject](t.Context(), c, paged, arrayShape, ListOptions{}) {
		if err != nil {
			t.Fatalf("all yielded %v after %d items", err, len(items))
		}
		items = append(items, item)
		if len(items) == 1 {
			time.Sleep(timeout + 250*time.Millisecond)
		}
	}
	if diff := cmp.Diff(objects(0, 2), items); diff != "" {
		t.Errorf("all yielded items that differ (-want +got):\n%s", diff)
	}
}

// TestAllRetriesAPage checks that each page is a call of its own, which the client sends again
// as its retry policy allows: a 429 on page 2 sends page 2 again, and the list reads on.
func TestAllRetriesAPage(t *testing.T) {
	api := mockpermit.NewAPI(t)
	api.Handle(route(opPaged), arrayOf(t, objects(0, 30)),
		mockpermit.Reply{Status: http.StatusTooManyRequests, Header: http.Header{
			"Retry-After": {"0"},
		}},
		arrayOf(t, objects(30, 31)), arrayOf(t, objects(0, 0)))
	c := pageClient(t, api)
	items, errs := collect(t, all[testObject](t.Context(), c, paged, arrayShape, ListOptions{}))
	if len(items) != 31 || errs != nil {
		t.Errorf("all yielded %d items and the errors %v, want 31 items", len(items), errs)
	}
	want := []string{
		"page=1&per_page=30", "page=2&per_page=30", "page=2&per_page=30", "page=3&per_page=30",
	}
	if diff := cmp.Diff(want, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestAllCanBeRangedAgain checks that each loop over the iterator that all returns reads the
// list from the page of the options again.
func TestAllCanBeRangedAgain(t *testing.T) {
	api := mockpermit.NewAPI(t)
	api.Handle(route(opPaged), arrayOf(t, objects(0, 1)), arrayOf(t, objects(0, 0)),
		arrayOf(t, objects(0, 1)), arrayOf(t, objects(0, 0)))
	seq := all[testObject](t.Context(), pageClient(t, api), paged, arrayShape, ListOptions{})
	for range 2 {
		if items, errs := collect(t, seq); len(items) != 1 || errs != nil {
			t.Errorf("all yielded %d items and the errors %v, want 1 item", len(items), errs)
		}
	}
	want := []string{
		"page=1&per_page=30", "page=2&per_page=30", "page=1&per_page=30", "page=2&per_page=30",
	}
	if diff := cmp.Diff(want, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestListPage checks the page that listPage returns and the query it sends: page and per_page
// only when the options set them, per_page as given, after the request's own parameters, which
// stay as they were. Page holds the page and per_page that the call asked for, the first page
// for a Page of 0, and the counts when the reply reports them.
func TestListPage(t *testing.T) {
	data := []testObject{{Key: "a"}, {Key: "b"}}
	for _, tc := range []struct {
		name      string
		opts      ListOptions
		body      string
		wantQuery string
		want      Page[testObject]
	}{
		{
			name: "an array with no options", body: `[{"key":"a"},{"key":"b"}]`,
			wantQuery: "search=x", want: Page[testObject]{Data: data, Page: 1},
		},
		{
			name: "a page with counts", opts: ListOptions{Page: 2, PerPage: 1000},
			body:      `{"data":[{"key":"a"},{"key":"b"}],"total_count":1002,"page_count":2}`,
			wantQuery: "page=2&per_page=1000&search=x",
			want: Page[testObject]{
				Data: data, Page: 2, PerPage: 1000, TotalCount: new(1002), PageCount: new(2),
			},
		},
		{
			name: "an empty page without counts", opts: ListOptions{PerPage: 5},
			body: `{"data":[]}`, wantQuery: "per_page=5&search=x",
			want: Page[testObject]{Data: []testObject{}, Page: 1, PerPage: 5},
		},
		{
			name: "a page number alone", opts: ListOptions{Page: 3}, body: `[]`,
			wantQuery: "page=3&search=x", want: Page[testObject]{Data: []testObject{}, Page: 3},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(route(opPaged), mockpermit.Reply{Body: tc.body})
			var query wire.Query
			query.AddString("search", new("x"))
			r := request{op: opPaged.ID, target: apiTarget, query: &query}
			got, err := listPage[testObject](t.Context(), pageClient(t, api), r,
				arrayOrPageShape, tc.opts)
			if err != nil {
				t.Fatalf("listPage returned %v", err)
			}
			if diff := cmp.Diff(&tc.want, got); diff != "" {
				t.Errorf("listPage returned a page that differs (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff([]string{tc.wantQuery}, queries(api)); diff != "" {
				t.Errorf("the query differs (-want +got):\n%s", diff)
			}
			if query.Encode() != "search=x" {
				t.Errorf("listPage changed the request's query to %q", query.Encode())
			}
		})
	}
}

// TestListPageOfAnObjectWithoutData checks that a reply that is an object without a data array
// is an error that names the model, not an empty page.
func TestListPageOfAnObjectWithoutData(t *testing.T) {
	api := mockpermit.NewAPI(t)
	api.Handle(route(opPaged), mockpermit.Reply{Body: `{"total_count":0,"page_count":0}`})
	got, err := listPage[testObject](t.Context(), pageClient(t, api), paged, arrayOrPageShape,
		ListOptions{})
	want := "permit: test_paged: GET /__test/paged: status 200: the reply is not a valid list " +
		"of models.TestObject"
	if got != nil || mustError(t, err).Error() != want {
		t.Errorf("listPage returned %+v and %v, want no page and %s", got, err, want)
	}
}

// TestNegativeListOptions checks that a Page or a PerPage less than zero is an error that matches
// ErrInvalidConfig and names the operation and the rule, from listPage and once from all, before
// any request.
func TestNegativeListOptions(t *testing.T) {
	const prefix = "permit: invalid configuration: test_paged: ListOptions: "
	for _, tc := range []struct {
		opts ListOptions
		want string
	}{
		{
			ListOptions{Page: -1, PerPage: 30},
			prefix + "Page is less than zero; give 0 or 1 for the first page, or the number " +
				"of a later page",
		},
		{
			ListOptions{Page: 2, PerPage: -1},
			prefix + "PerPage is less than zero; give 0 to leave the number of items on a " +
				"page to the method, or a number from 1",
		},
	} {
		api := mockpermit.NewAPI(t)
		c := pageClient(t, api)
		page, err := listPage[testObject](t.Context(), c, paged, arrayShape, tc.opts)
		if page != nil || !errors.Is(err, ErrInvalidConfig) || err.Error() != tc.want {
			t.Errorf("listPage(%+v) returned %+v and %v, want no page and %s", tc.opts, page,
				err, tc.want)
		}
		items, errs := collect(t, all[testObject](t.Context(), c, paged, arrayShape, tc.opts))
		if items != nil || len(errs) != 1 || !errors.Is(errs[0], ErrInvalidConfig) ||
			errs[0].Error() != tc.want {
			t.Errorf("all(%+v) yielded %v and the errors %v, want one error: %s", tc.opts,
				items, errs, tc.want)
		}
		api.AssertUntouched()
	}
}

// TestAttributeFilterValues checks the parameter that addAttributeFilters adds for a value at the
// edges of its type: an int of 0, an int64 at its limits, and a float64 with every digit, without
// a fraction, at its largest and at its smallest above zero.
func TestAttributeFilterValues(t *testing.T) {
	for _, tc := range []struct {
		filter AttributeFilter
		want   string
	}{
		{AttributeEquals("k", 0), "attr_k=0"},
		{AttributeEquals("k", int64(math.MaxInt64)), "attr_k=9223372036854775807"},
		{AttributeEquals("k", int64(math.MinInt64)), "attr_k=-9223372036854775808"},
		{AttributeEquals("k", 0.1), "attr_k=0.1"},
		{AttributeEquals("k", 1.0), "attr_k=1"},
		{AttributeEquals("k", math.MaxFloat64), "attr_k=1.7976931348623157e%2B308"},
		{AttributeEquals("k", math.SmallestNonzeroFloat64), "attr_k=5e-324"},
	} {
		query := &wire.Query{}
		if err := addAttributeFilters(query, "op", []AttributeFilter{tc.filter}); err != nil {
			t.Errorf("%+v: addAttributeFilters returned %v", tc.filter, err)
		}
		if got := query.Encode(); got != tc.want {
			t.Errorf("%+v: the query is %q, want %q", tc.filter, got, tc.want)
		}
	}
}

// TestAttributeFiltersThatAreNotValid checks the rule and the place of the filter that the error
// of addAttributeFilters names, and that the query then holds no filter: a filter that breaks two
// rules gets the first of name-empty, not-finite and repeated, and a repeat is a name that an
// earlier filter has, whatever the type and the value of either.
func TestAttributeFiltersThatAreNotValid(t *testing.T) {
	valid := AttributeEquals("a", 1.0)
	for _, tc := range []struct {
		name    string
		filters []AttributeFilter
		place   int
		rule    validate.Rule
	}{
		{
			"an empty name and NaN",
			[]AttributeFilter{AttributeEquals("", math.NaN())},
			1, validate.ListAttributeFilterNameEmpty,
		},
		{
			"NaN for the name of an earlier filter",
			[]AttributeFilter{valid, AttributeEquals("a", math.NaN())},
			2, validate.ListAttributeFilterNotFinite,
		},
		{
			"a repeat after another name",
			[]AttributeFilter{valid, AttributeEquals("b", "x"), AttributeEquals("a", true)},
			3, validate.ListAttributeFilterRepeated,
		},
		{
			"a repeat of the same value",
			[]AttributeFilter{valid, valid},
			2, validate.ListAttributeFilterRepeated,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := &wire.Query{}
			err := addAttributeFilters(query, "op", tc.filters)
			want := &configError{
				setting: "op: attribute filter " + strconv.Itoa(tc.place), rule: tc.rule,
			}
			if !errors.Is(err, ErrInvalidConfig) || err.Error() != want.Error() {
				t.Errorf("addAttributeFilters returned %v, want %v", err, want)
			}
			if got := query.Encode(); got != "" {
				t.Errorf("the query is %q, want no filter", got)
			}
		})
	}
}
