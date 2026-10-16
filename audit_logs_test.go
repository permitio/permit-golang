package permit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/permitio/permit-golang/v2/internal/testkit/fixtures"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
	"github.com/permitio/permit-golang/v2/models"
)

// The routes of the audit logs, as mockpermit.Server.Handle takes them, and the path of the list
// under the scope p and e of resourceClient.
const (
	listAuditLogsRoute = "GET /v2/pdps/{proj_id}/{env_id}/audit_logs"
	getAuditLogRoute   = "GET /v2/pdps/{proj_id}/{env_id}/audit_logs/{log_id}"
	auditLogsPath      = "/v2/pdps/p/e/audit_logs"
)

// auditLogBody is a reply of the API to a get of an audit log, with the fields that the schema
// requires.
const auditLogBody = `{"id":"l1","timestamp":"2026-10-15T09:30:00Z","org_id":"o",` +
	`"project_id":"p","env_id":"e","raw_data":{"engine":"GENERIC",` +
	`"timestamp":"2026-10-15T09:30:00Z","decision":true}}`

// auditLogsOf returns the audit logs l<from> to l<to-1>, with only an ID, and the reply of a page
// of them, the paginated object without counts: an empty list when to is from.
func auditLogsOf(t *testing.T, from, to int) ([]models.AuditLogModel, mockpermit.Reply) {
	t.Helper()
	items := make([]models.AuditLogModel, 0, to-from)
	for i := from; i < to; i++ {
		items = append(items, models.AuditLogModel{ID: "l" + strconv.Itoa(i)})
	}
	return items, pageOf(t, items, "")
}

// TestAuditLogFilters checks the query of List and All for each filter alone, and for none: a
// filter that is not set is not sent; a list is sent as the parameter repeated for each value, in
// order, the empty string too; a pointer to false sends decision=false, and a pointer to 0 sends
// 0 in timestamp_from and in timestamp_to; and an empty list is not sent.
func TestAuditLogFilters(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  *AuditLogsListOptions
		query string
	}{
		{name: "nil options"},
		{name: "the zero options", opts: &AuditLogsListOptions{}},
		{
			name: "empty lists",
			opts: &AuditLogsListOptions{Users: []string{}, Resources: []string{}},
		},
		{
			name: "pdp_id", opts: &AuditLogsListOptions{PDPID: fixtures.UUIDKey},
			query: "pdp_id=" + fixtures.UUIDKey,
		},
		{
			name: "users", opts: &AuditLogsListOptions{Users: []string{"b@example.com", "", "a"}},
			query: "users=b%40example.com&users=&users=a",
		},
		{
			name: "decision true", opts: &AuditLogsListOptions{Decision: new(true)},
			query: "decision=true",
		},
		{
			name: "decision false", opts: &AuditLogsListOptions{Decision: new(false)},
			query: "decision=false",
		},
		{
			name: "resources", opts: &AuditLogsListOptions{Resources: []string{"folder", "doc"}},
			query: "resources=folder&resources=doc",
		},
		{name: "tenant", opts: &AuditLogsListOptions{Tenant: "a b"}, query: "tenant=a+b"},
		{name: "action", opts: &AuditLogsListOptions{Action: "read"}, query: "action=read"},
		{
			name: "timestamp_from 0", opts: &AuditLogsListOptions{TimestampFrom: new(int64(0))},
			query: "timestamp_from=0",
		},
		{
			name: "timestamp_to 0", opts: &AuditLogsListOptions{TimestampTo: new(int64(0))},
			query: "timestamp_to=0",
		},
		{
			name: "a window",
			opts: &AuditLogsListOptions{
				TimestampFrom: new(int64(1767225600)), TimestampTo: new(int64(1767229200)),
			},
			query: "timestamp_from=1767225600&timestamp_to=1767229200",
		},
		{
			name: "sort_by", opts: &AuditLogsListOptions{SortBy: models.AuditLogSortKeyTimestamp},
			query: "sort_by=timestamp",
		},
		{
			name: "query", opts: &AuditLogsListOptions{Query: models.AuditLogQueryTypeNone},
			query: "query=none",
		},
		{
			name:  "an enum value that the SDK does not know",
			opts:  &AuditLogsListOptions{Query: "added_later", SortBy: "added_later"},
			query: "query=added_later&sort_by=added_later",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listAuditLogsRoute, pageOf(t, []models.AuditLogModel{}, ""),
				pageOf(t, []models.AuditLogModel{}, ""))
			c := resourceClient(t, api)
			if _, err := c.AuditLogs.List(t.Context(), tc.opts); err != nil {
				t.Fatalf("List returned %v", err)
			}
			if _, errs := collect(t, c.AuditLogs.All(t.Context(), tc.opts)); errs != nil {
				t.Fatalf("All yielded %v", errs)
			}
			paged := "page=1&per_page=100"
			if tc.query != "" {
				paged = tc.query + "&" + paged
			}
			want := []string{tc.query, auditLogSortedQuery(paged)}
			if diff := cmp.Diff(want, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// auditLogSortedQuery returns query, parameters joined by &, with the parameters sorted by name,
// and the values of one name in their order, as a request sends them.
func auditLogSortedQuery(query string) string {
	params := strings.Split(query, "&")
	slices.SortStableFunc(params, func(a, b string) int {
		nameA, _, _ := strings.Cut(a, "=")
		nameB, _, _ := strings.Cut(b, "=")
		return strings.Compare(nameA, nameB)
	})
	return strings.Join(params, "&")
}

// TestAuditLogsAllReadsEveryPage checks that All reads the 31 audit logs of a list, in order,
// asking for each page in turn with the filters of the options in each request: up to the empty
// page at a per_page of 30, and at the default per_page of 100, the most that the API gives on a
// page; up to the last page that the replies count; and from page 2 on, when the options start
// there.
func TestAuditLogsAllReadsEveryPage(t *testing.T) {
	all, every := auditLogsOf(t, 0, 31)
	first, firstPage := auditLogsOf(t, 0, 30)
	_, secondPage := auditLogsOf(t, 30, 31)
	_, empty := auditLogsOf(t, 0, 0)
	counted := []mockpermit.Reply{
		pageOf(t, first, `,"total_count":31,"pagination_count":31,"page_count":2`),
		pageOf(t, all[30:], `,"total_count":31,"pagination_count":31,"page_count":2`),
	}
	for _, tc := range []struct {
		name    string
		opts    *AuditLogsListOptions
		pages   []mockpermit.Reply
		want    []models.AuditLogModel
		queries []string
	}{
		{
			name:  "per_page 30",
			opts:  &AuditLogsListOptions{ListOptions: ListOptions{PerPage: 30}, Tenant: "t"},
			pages: []mockpermit.Reply{firstPage, secondPage, empty}, want: all,
			queries: []string{
				"page=1&per_page=30&tenant=t", "page=2&per_page=30&tenant=t",
				"page=3&per_page=30&tenant=t",
			},
		},
		{
			name: "the default per_page", pages: []mockpermit.Reply{every, empty}, want: all,
			queries: []string{"page=1&per_page=100", "page=2&per_page=100"},
		},
		{
			name:  "the pages that the replies count",
			opts:  &AuditLogsListOptions{ListOptions: ListOptions{PerPage: 30}},
			pages: counted, want: all,
			queries: []string{"page=1&per_page=30", "page=2&per_page=30"},
		},
		{
			name:  "from page 2",
			opts:  &AuditLogsListOptions{ListOptions: ListOptions{Page: 2, PerPage: 30}},
			pages: []mockpermit.Reply{secondPage, empty}, want: all[30:],
			queries: []string{"page=2&per_page=30", "page=3&per_page=30"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listAuditLogsRoute, tc.pages...)
			items, errs := collect(t, resourceClient(t, api).AuditLogs.All(t.Context(), tc.opts))
			if diff := cmp.Diff(tc.want, items); diff != "" || errs != nil {
				t.Errorf("All yielded the errors %v and audit logs that differ (-want +got):\n%s",
					errs, diff)
			}
			if diff := cmp.Diff(tc.queries, queries(api)); diff != "" {
				t.Errorf("the queries differ (-want +got):\n%s", diff)
			}
		})
	}
}

// TestAuditLogsAllYieldsAnErrorOnce checks that a page that fails ends the iterator with its
// error, once, after the audit logs of the pages before it; that the options of All are read
// when it is called, so a later change to them, to a filter too, changes no request; and that a
// loop that stops early sends no other request.
func TestAuditLogsAllYieldsAnErrorOnce(t *testing.T) {
	const body = `{"error_code":"PAGINATION_SIZE_OVERREACHED","title":"Bad Request",` +
		`"message":"Max number of results for this API is 10000"}`
	_, first := auditLogsOf(t, 0, 30)
	api := mockpermit.NewAPI(t)
	overreach := mockpermit.Reply{Status: http.StatusBadRequest, Body: body}
	api.Handle(listAuditLogsRoute, first, overreach, first)
	opts := &AuditLogsListOptions{
		ListOptions: ListOptions{PerPage: 30}, Users: []string{"a"}, Decision: new(false),
	}
	c := resourceClient(t, api)
	seq := c.AuditLogs.All(t.Context(), opts)
	opts.PerPage, opts.Page, opts.Users[0], opts.Decision = 50, 4, "b", nil
	items, errs := collect(t, seq)
	if len(items) != 30 || len(errs) != 1 ||
		mustError(t, errs[0]).StatusCode != http.StatusBadRequest {
		t.Errorf("All yielded %d audit logs and the errors %v, want 30 and one error of the "+
			"status 400", len(items), errs)
	}
	var looped []string
	for log, err := range c.AuditLogs.All(t.Context(), &AuditLogsListOptions{
		ListOptions: ListOptions{PerPage: 30},
	}) {
		if err != nil {
			t.Fatalf("the loop got the error %v", err)
		}
		if looped = append(looped, log.ID); len(looped) == 1 {
			break
		}
	}
	if !slices.Equal(looped, []string{"l0"}) {
		t.Errorf("the loop got the audit logs %q, want l0 alone", looped)
	}
	want := []string{
		"decision=false&page=1&per_page=30&users=a", "decision=false&page=2&per_page=30&users=a",
		"page=1&per_page=30",
	}
	if diff := cmp.Diff(want, queries(api)); diff != "" {
		t.Errorf("the queries differ (-want +got):\n%s", diff)
	}
}

// TestAuditLogsListReturnsAPage checks the page that List returns: the audit logs of the reply in
// its order, the page and the per_page that it asked for, page 1 for nil options, and the counts
// that the reply reports. A page without audit logs has an empty list, not nil.
func TestAuditLogsListReturnsAPage(t *testing.T) {
	two, _ := auditLogsOf(t, 0, 2)
	for _, tc := range []struct {
		name  string
		opts  *AuditLogsListOptions
		reply mockpermit.Reply
		query string
		want  *Page[models.AuditLogModel]
	}{
		{
			name: "nil options", reply: pageOf(t, two, `,"total_count":2,"pagination_count":2`),
			want: &Page[models.AuditLogModel]{Data: two, Page: 1, TotalCount: new(2)},
		},
		{
			name: "a page after the last",
			opts: &AuditLogsListOptions{ListOptions: ListOptions{Page: 3, PerPage: 30}},
			reply: pageOf(t, []models.AuditLogModel{}, `,"total_count":2,"pagination_count":2,`+
				`"page_count":0`),
			query: "page=3&per_page=30",
			want: &Page[models.AuditLogModel]{
				Data: []models.AuditLogModel{}, Page: 3, PerPage: 30, TotalCount: new(2),
				PageCount: new(0),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(listAuditLogsRoute, tc.reply)
			got, err := resourceClient(t, api).AuditLogs.List(t.Context(), tc.opts)
			if err != nil {
				t.Fatalf("List returned %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" || got.Data == nil {
				t.Errorf("List returned a page that differs (-want +got), or nil Data:\n%s", diff)
			}
			if requests := api.Requests(); len(requests) != 1 ||
				requests[0].RawPath != auditLogsPath || requests[0].RawQuery != tc.query {
				t.Errorf("List sent %+v, want one request to %s?%s", requests, auditLogsPath,
					tc.query)
			}
		})
	}
}

// TestInvalidAuditLogListOptions checks that a Page or a PerPage less than zero is an error that
// matches ErrInvalidConfig from List and All, and that neither sends a request.
func TestInvalidAuditLogListOptions(t *testing.T) {
	for _, opts := range []AuditLogsListOptions{
		{ListOptions: ListOptions{Page: -1}}, {ListOptions: ListOptions{PerPage: -1}},
	} {
		api := mockpermit.NewAPI(t)
		c := resourceClient(t, api)
		_, err := c.AuditLogs.List(t.Context(), &opts)
		_, errs := collect(t, c.AuditLogs.All(t.Context(), &opts))
		for _, err := range append([]error{err}, errs...) {
			if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("%+v: the call returned %v, want an error that matches ErrInvalidConfig",
					opts, err)
			}
		}
		if len(errs) != 1 {
			t.Errorf("%+v: All yielded the errors %v, want one", opts, errs)
		}
		api.AssertUntouched()
	}
}

// TestAuditLogIDsInThePath checks that Get sends the ID of the audit log as one segment of the
// path, escaped, and that an ID that is empty, . or .. is an error that matches ErrInvalidConfig
// and names {log_id}, without a request.
func TestAuditLogIDsInThePath(t *testing.T) {
	for id, escaped := range map[string]string{
		"a/b":            "a%2Fb",
		"50%":            "50%25",
		"é":              "%C3%A9",
		fixtures.UUIDKey: fixtures.UUIDKey,
		".hidden":        ".hidden",
	} {
		api := mockpermit.NewAPI(t)
		api.Handle(getAuditLogRoute, mockpermit.Reply{Body: auditLogBody})
		log, err := resourceClient(t, api).AuditLogs.Get(t.Context(), id)
		if err != nil || log.ID != "l1" || log.RawData == nil {
			t.Errorf("Get(%q) returned %+v and %v, want the audit log l1 with its raw data", id,
				log, err)
		}
		want := auditLogsPath + "/" + escaped
		if requests := api.Requests(); len(requests) != 1 || requests[0].RawPath != want {
			t.Errorf("Get(%q) sent %+v, want one request to %s", id, requests, want)
		}
	}
	for _, id := range []string{"", ".", ".."} {
		api := mockpermit.NewAPI(t)
		_, err := resourceClient(t, api).AuditLogs.Get(t.Context(), id)
		named := err != nil && strings.Contains(err.Error(), "{log_id}")
		if !errors.Is(err, ErrInvalidConfig) || !named {
			t.Errorf("Get(%q) returned %v, want an error that matches ErrInvalidConfig and "+
				"names {log_id}", id, err)
		}
		api.AssertUntouched()
	}
}

// TestAuditLogsUseTheScope checks that the calls of the audit logs act on the environment of the
// client's scope: without WithScope, the client looks up the scope of its API key once, and each
// call fills the route with the project and the environment of an environment-level key. A scope
// without an environment is an error that matches ErrInvalidConfig and names the operation, and
// no call sends a request but the lookup.
func TestAuditLogsUseTheScope(t *testing.T) {
	const prefix = "/v2/pdps/" + scopeProjectID + "/" + scopeEnvironmentID + "/audit_logs"
	calls := map[string]func(ctx context.Context, c *Client) error{
		opListAuditLogs: func(ctx context.Context, c *Client) error {
			_, err := c.AuditLogs.List(ctx, nil)
			return err
		},
		opGetDetailedAuditLog: func(ctx context.Context, c *Client) error {
			_, err := c.AuditLogs.Get(ctx, "l1")
			return err
		},
	}
	t.Run("an environment-level key", func(t *testing.T) {
		api := mockpermit.NewAPI(t)
		api.Handle(scopeRoute, scopeReply(t, ""))
		api.Handle(listAuditLogsRoute, pageOf(t, []models.AuditLogModel{}, ""))
		api.Handle(getAuditLogRoute, mockpermit.Reply{Body: auditLogBody})
		c := projectClient(t, api)
		for op, call := range calls {
			if err := call(t.Context(), c); err != nil {
				t.Errorf("the call of %s returned %v", op, err)
			}
		}
		requests := api.Requests()
		if len(requests) != 3 || requests[0].RawPath != "/v2/api-key/scope" ||
			!strings.HasPrefix(requests[1].RawPath, prefix) ||
			!strings.HasPrefix(requests[2].RawPath, prefix) {
			t.Errorf("the calls sent %+v, want the lookup of the scope, then a request under %s "+
				"for each call", requests, prefix)
		}
	})
	for _, suffix := range []string{".project", ".organization"} {
		t.Run("the scope of the fixture"+suffix, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(scopeRoute, scopeReply(t, suffix))
			c := projectClient(t, api)
			for op, call := range calls {
				err := call(t.Context(), c)
				if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), op) {
					t.Errorf("the call of %s returned %v, want an error that matches "+
						"ErrInvalidConfig and names %s", op, err, op)
				}
			}
			if _, errs := collect(t, c.AuditLogs.All(t.Context(), nil)); len(errs) != 1 ||
				!errors.Is(errs[0], ErrInvalidConfig) {
				t.Errorf("All yielded %v, want one error that matches ErrInvalidConfig", errs)
			}
			if n := len(api.Requests()); n != 1 {
				t.Errorf("the calls sent %d requests, want only the lookup of the scope", n)
			}
		})
	}
}

// TestAuditLogRepliesAddNoCredential checks that the audit logs that List and Get return, whose
// input, context and objects hold the application's values, add nothing to the client's Set of
// credentials, not even a value under a credential name such as token or password, so that a
// later error of the client that holds such a value shows it as it is.
func TestAuditLogRepliesAddNoCredential(t *testing.T) {
	const (
		listed = "opaque-listed-value"
		got    = "opaque-detailed-value"
	)
	api := mockpermit.NewAPI(t)
	api.Handle(listAuditLogsRoute, mockpermit.Reply{Body: `{"data":[{"id":"l1",` +
		`"timestamp":"2026-10-15T09:30:00Z","org_id":"o","project_id":"p","env_id":"e",` +
		`"context":{"token":"` + listed + `"}}],"total_count":1,"pagination_count":1}`})
	detailed := strings.Replace(auditLogBody, `"org_id"`,
		`"input":{"user":{"attributes":{"password":"`+got+`"}}},"org_id"`, 1)
	missing := `{"message":"no audit log of ` + listed + ` or ` + got + `"}`
	api.Handle(getAuditLogRoute, mockpermit.Reply{Body: detailed},
		mockpermit.Reply{Status: http.StatusNotFound, Body: missing})
	c := resourceClient(t, api)
	if _, err := c.AuditLogs.List(t.Context(), nil); err != nil {
		t.Fatalf("List returned %v", err)
	}
	if _, err := c.AuditLogs.Get(t.Context(), "l1"); err != nil {
		t.Fatalf("Get returned %v", err)
	}
	for _, value := range []string{listed, got} {
		if c.secrets.Contains(value) {
			t.Errorf("an audit log added %s, a value of the application, to the client's Set",
				value)
		}
	}
	_, err := c.AuditLogs.Get(t.Context(), "l2")
	if e := mustError(t, err); !strings.Contains(e.Detail, listed) ||
		!strings.Contains(e.Detail, got) {
		t.Errorf("the error of a later call has the detail %q, want both values as they are",
			e.Detail)
	}
}

// TestAuditLogRetries checks that each call of the audit logs, which changes nothing, sends its
// request again after a 503, as after a 429, with the default policy of retries.
func TestAuditLogRetries(t *testing.T) {
	again := http.Header{"Retry-After": {"0"}}
	unavailable := mockpermit.Reply{
		Status: http.StatusServiceUnavailable, Header: again, Body: `{}`,
	}
	limited := mockpermit.Reply{Status: http.StatusTooManyRequests, Header: again, Body: `{}`}
	for _, tc := range []struct {
		name  string
		route string
		reply mockpermit.Reply
		call  func(c *Client) error
	}{
		{
			name: "List", route: listAuditLogsRoute,
			reply: pageOf(t, []models.AuditLogModel{}, ""),
			call: func(c *Client) error {
				_, err := c.AuditLogs.List(t.Context(), nil)
				return err
			},
		},
		{
			name: "Get", route: getAuditLogRoute, reply: mockpermit.Reply{Body: auditLogBody},
			call: func(c *Client) error {
				_, err := c.AuditLogs.Get(t.Context(), "l1")
				return err
			},
		},
	} {
		for _, failed := range []mockpermit.Reply{unavailable, limited} {
			t.Run(tc.name+" after "+strconv.Itoa(failed.Status), func(t *testing.T) {
				api := mockpermit.NewAPI(t)
				api.Handle(tc.route, failed, tc.reply)
				if err := tc.call(resourceClient(t, api)); err != nil {
					t.Errorf("the call returned %v, want the result of the second request", err)
				}
				if n := len(api.Requests()); n != 2 {
					t.Errorf("the call sent %d requests, want 2", n)
				}
			})
		}
	}
}

// TestAuditLogNullFields checks what List and Get return for each optional field of an audit log
// for which the reply holds null, as the API sends a field that has no value: a string or a time
// is empty; a decision, an object of the user, the action, the resource or the tenant, and the
// attributes and the roles of an object are nil; the objects of a detailed audit log are the zero
// AuditLogObjectsModel; and a field of JSON holds the JSON null, not nil. A field of JSON that
// the reply leaves out is nil.
func TestAuditLogNullFields(t *testing.T) {
	const (
		required = `"id":"l1","timestamp":"2026-10-15T09:30:00Z","org_id":"o","project_id":"p",` +
			`"env_id":"e"`
		nulls = `,"created_at":null,"query":null,"user_key":null,"user_email":null,` +
			`"user_name":null,"resource_type":null,"tenant":null,"action":null,` +
			`"decision":null,"reason":null,"pdp_config_id":null,"input":null,"result":null,` +
			`"context":null`
		rawData = `{"engine":"GENERIC","timestamp":"2026-10-15T09:30:00Z","decision":true}`
		created = `"created_at":"2026-10-01T08:00:00Z","updated_at":"2026-10-01T08:00:00Z"`
		objects = `,"objects":{"id":null,"created_at":null,"organization_object":null,` +
			`"project_object":null,"environment_object":null,"pdp_config_object":null,` +
			`"user_object":{"id":"u1","key":"alice",` + created + `,"email":null,` +
			`"first_name":null,"last_name":null,"attributes":null,"assigned_roles":null},` +
			`"action_object":null,"resource_type_object":{"id":"r1","key":"document",` +
			created + `,"name":null,"attributes":null},"tenant_object":{"id":"t1",` +
			`"key":"default",` + created + `,"name":null,"attributes":null}}`
	)
	jsonNull := json.RawMessage("null")
	timestamp := models.Time{Time: time.Date(2026, 10, 15, 9, 30, 0, 0, time.UTC)}
	createdAt := models.Time{Time: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)}
	listed := models.AuditLogModel{
		ID: "l1", Timestamp: timestamp, OrganizationID: "o", ProjectID: "p", EnvironmentID: "e",
	}
	nullListed := listed
	nullListed.Input, nullListed.Result, nullListed.Context, nullListed.RawData = jsonNull,
		jsonNull, jsonNull, jsonNull
	detailed := models.DetailedAuditLogModel{
		ID: "l1", Timestamp: timestamp, OrganizationID: "o", ProjectID: "p", EnvironmentID: "e",
		Input: jsonNull, Result: jsonNull, Context: jsonNull, RawData: json.RawMessage(rawData),
	}
	nullObjects := detailed
	nullObjects.Objects = models.AuditLogObjectsModel{
		OrganizationObject: jsonNull, ProjectObject: jsonNull, EnvironmentObject: jsonNull,
		PDPConfigObject: jsonNull,
		UserObject: &models.UserObj{
			ID: "u1", Key: "alice", CreatedAt: createdAt, UpdatedAt: createdAt,
		},
		ResourceTypeObject: &models.ResourceTypeObj{
			ID: "r1", Key: "document", CreatedAt: createdAt, UpdatedAt: createdAt,
		},
		TenantObject: &models.TenantObj{
			ID: "t1", Key: "default", CreatedAt: createdAt, UpdatedAt: createdAt,
		},
	}
	list := func(c *Client) (any, error) {
		page, err := c.AuditLogs.List(t.Context(), nil)
		if err != nil {
			return nil, err
		}
		return page.Data, nil
	}
	get := func(c *Client) (any, error) {
		return c.AuditLogs.Get(t.Context(), "l1")
	}
	for _, tc := range []struct {
		name  string
		route string
		body  string
		call  func(c *Client) (any, error)
		want  any
	}{
		{
			name: "a listed audit log with null in each optional field", route: listAuditLogsRoute,
			body: `{"data":[{` + required + nulls + `,"raw_data":null}]}`, call: list,
			want: []models.AuditLogModel{nullListed},
		},
		{
			name: "a listed audit log without its optional fields", route: listAuditLogsRoute,
			body: `{"data":[{` + required + `}]}`, call: list,
			want: []models.AuditLogModel{listed},
		},
		{
			name: "a detailed audit log with null objects", route: getAuditLogRoute,
			body: `{` + required + nulls + `,"raw_data":` + rawData + `,"objects":null}`,
			call: get, want: &detailed,
		},
		{
			name: "a detailed audit log with null in the objects", route: getAuditLogRoute,
			body: `{` + required + nulls + `,"raw_data":` + rawData + objects + `}`,
			call: get, want: &nullObjects,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := mockpermit.NewAPI(t)
			api.Handle(tc.route, mockpermit.Reply{Body: tc.body})
			got, err := tc.call(resourceClient(t, api))
			if err != nil {
				t.Fatalf("the call returned %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("the call returned an audit log that differs (-want +got):\n%s", diff)
			}
		})
	}
}

// TestMalformedAuditLogReplies checks that a reply in 200-299 that is not an audit log, or not
// the paginated object of audit logs, such as a JSON array, is an *Error that says so, from Get,
// List and All, and no audit log.
func TestMalformedAuditLogReplies(t *testing.T) {
	for _, body := range []string{``, `null`, `[]`, `"a"`, `{"id":5}`, `{} {}`} {
		api := mockpermit.NewAPI(t)
		api.Handle(getAuditLogRoute, mockpermit.Reply{Body: body})
		log, err := resourceClient(t, api).AuditLogs.Get(t.Context(), "l1")
		want := "permit: get_detailed_audit_log: GET /v2/pdps/{proj_id}/{env_id}/audit_logs/" +
			"{log_id}: status 200: the reply is not a valid models.DetailedAuditLogModel"
		if e := mustError(t, err); log != nil || e.Error() != want {
			t.Errorf("Get of the reply %q returned %+v and\n%v\nwant no audit log and\n%s", body,
				log, err, want)
		}
	}
	for _, body := range []string{``, `null`, `[]`, `[{"id":"l1"}]`, `{"data":{}}`, `{"x":1}`} {
		api := mockpermit.NewAPI(t)
		api.Handle(listAuditLogsRoute, mockpermit.Reply{Body: body},
			mockpermit.Reply{Body: body})
		c := resourceClient(t, api)
		page, err := c.AuditLogs.List(t.Context(), nil)
		want := "permit: list_audit_logs: GET /v2/pdps/{proj_id}/{env_id}/audit_logs: " +
			"status 200: the reply is not a valid list of models.AuditLogModel"
		if e := mustError(t, err); page != nil || e.Error() != want {
			t.Errorf("List of the reply %q returned %+v and\n%v\nwant no page and\n%s", body,
				page, err, want)
		}
		items, errs := collect(t, c.AuditLogs.All(t.Context(), nil))
		if items != nil || len(errs) != 1 || mustError(t, errs[0]).Error() != want {
			t.Errorf("All of the reply %q yielded %+v and %v, want only the error\n%s", body,
				items, errs, want)
		}
	}
}

// TestAuditLogsAfterClose checks that each method returns ErrClosed after Close, and sends
// nothing, also without WithScope, whose lookup it does not send either.
func TestAuditLogsAfterClose(t *testing.T) {
	api := mockpermit.NewAPI(t)
	c := projectClient(t, api)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	_, listErr := c.AuditLogs.List(ctx, nil)
	_, getErr := c.AuditLogs.Get(ctx, "l1")
	_, allErrs := collect(t, c.AuditLogs.All(ctx, nil))
	for i, err := range append([]error{listErr, getErr}, allErrs...) {
		if !errors.Is(err, ErrClosed) {
			t.Errorf("call %d after Close returned %v, want ErrClosed", i, err)
		}
	}
	if len(allErrs) != 1 {
		t.Errorf("All after Close yielded %v, want ErrClosed once", allErrs)
	}
	api.AssertUntouched()
}
