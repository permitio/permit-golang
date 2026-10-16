package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the audit logs.
const (
	opListAuditLogs       = "list_audit_logs"
	opGetDetailedAuditLog = "get_detailed_audit_log"
)

// AuditLogsService lists and reads the audit logs of an environment, the decisions that its PDPs
// made, with who asked for what and what the PDP decided; [Client.AuditLogs] holds it. A PDP
// sends its decisions to Permit, which stores them as audit logs after a while, so a decision
// appears in the audit logs some time after the PDP made it.
//
// Its calls act on the environment of the client's scope, whose project and environment fill the
// route: those that [WithScope] gives, or else those of the client's API key, which the first call
// that needs them looks up once, as [Client.Scope] says. A scope without an environment, such as
// that of a project-level API key without WithScope, is an error that matches [ErrInvalidConfig],
// and the call sends no request for its operation. Each method is sent with an
// environment-level API key of the environment, or with a key of a wider level and WithScope; the
// API answers a key that may not read the audit logs with an error status, which the call returns
// as an [Error], such as one that matches [ErrForbidden].
//
// An audit log holds the values of the application's checks, such as the attributes of a user,
// as they were sent; the client does not add them to the credentials that it keeps out of its
// errors and log lines.
type AuditLogsService struct {
	client *Client
}

// AuditLogsListOptions are the page and the filters of [AuditLogsService.List] and
// [AuditLogsService.All]. Each filter that is set keeps only the audit logs that match it, and the
// call sends it in the query parameter that its doc comment names. A filter that is not set, as
// each is in the zero AuditLogsListOptions, is not sent, and the API's default applies: the zero
// AuditLogsListOptions lists the audit logs of the checks of every user, newest first.
type AuditLogsListOptions struct {
	// ListOptions picks the page and the number of audit logs on it, sent in page and per_page.
	// With a PerPage of 0, List leaves the number to the API, which gives 30, and All asks for
	// 100, the most that the API gives.
	ListOptions

	// PDPID keeps the audit logs of the PDP whose configuration has this ID, a UUID, sent as it
	// is given in the query parameter pdp_id. Empty keeps those of every PDP.
	PDPID string

	// Users keeps the audit logs of the users with these keys, emails or names: the API keeps an
	// audit log whose user key, email or name matches a value. Each is sent as it is given, the
	// empty string too, in the query parameter users, which the request repeats for each, in
	// order: users=alice&users=bob@example.com. Nil or empty keeps the audit logs of every user.
	// The API's spec takes at most 500 characters in each.
	Users []string

	// Decision keeps the audit logs whose decision is this one, true for allowed and false for
	// denied, sent in the query parameter decision as true or false: a pointer to false sends
	// decision=false. Nil keeps the audit logs of every decision.
	Decision *bool

	// Resources keeps the audit logs of the resources with these keys, such as document, each
	// sent as Users says, in the query parameter resources. Nil or empty keeps the audit logs of
	// every resource.
	Resources []string

	// Tenant keeps the audit logs of the tenant with this key, sent as it is given in the query
	// parameter tenant. Empty keeps those of every tenant.
	Tenant string

	// Action keeps the audit logs of this action, such as read, sent as it is given in the query
	// parameter action. Empty keeps those of every action.
	Action string

	// TimestampFrom and TimestampTo keep the audit logs of the decisions made in the window
	// between them, each a Unix time in seconds, such as 1767225600 for 2026-01-01T00:00:00Z,
	// sent as its decimal digits in the query parameters timestamp_from and timestamp_to: a
	// pointer to 0 sends 0. Nil leaves the bound out. The API filters by time only when both are
	// set and neither is 0.
	TimestampFrom *int64

	// TimestampTo is the end of the window, as TimestampFrom says.
	TimestampTo *int64

	// SortBy is the order of the audit logs, sent as it is given in the query parameter sort_by.
	// Empty leaves the order to the API, which sorts them by their time, newest first,
	// [models.AuditLogSortKeyTimestamp].
	SortBy models.AuditLogSortKey

	// Query keeps the audit logs of this kind of query of the PDP, sent as it is given in the
	// query parameter query. Empty leaves it to the API, which then keeps the audit logs of the
	// checks alone, [models.AuditLogQueryTypeCheck]; [models.AuditLogQueryTypeNone] keeps those
	// of every query.
	Query models.AuditLogQueryType
}

// auditLogsRequest returns the request of the list of the audit logs that opts filters, with a
// parameter in its query for each value of each filter of opts that is set, and the page of
// opts. A nil opts is the zero AuditLogsListOptions. The request holds copies of the values of
// opts, so a later change to opts does not change it.
func auditLogsRequest(opts *AuditLogsListOptions) (request, ListOptions) {
	var o AuditLogsListOptions
	if opts != nil {
		o = *opts
	}
	query := &wire.Query{}
	for _, filter := range []struct{ name, value string }{
		{"pdp_id", o.PDPID},
		{"tenant", o.Tenant},
		{"action", o.Action},
		{"sort_by", string(o.SortBy)},
		{"query", string(o.Query)},
	} {
		if filter.value != "" {
			query.AddString(filter.name, &filter.value)
		}
	}
	query.AddStrings("users", o.Users)
	query.AddStrings("resources", o.Resources)
	query.AddBool("decision", o.Decision)
	query.AddInt("timestamp_from", o.TimestampFrom)
	query.AddInt("timestamp_to", o.TimestampTo)
	return request{op: opListAuditLogs, target: apiTarget, query: query}, o.ListOptions
}

// List returns one page of the audit logs of the environment, those that match each filter of
// opts, in the order of the API's reply, with GET /v2/pdps/{proj_id}/{env_id}/audit_logs. A nil
// opts, like the zero AuditLogsListOptions, asks for the first page of the audit logs of the
// checks, with the API's default of 30 logs on it; the API gives at most 100 on a page. The page
// holds the counts that the reply reports. A page without audit logs, such as that of a filter
// that no log matches, has an empty Data, not nil. [AuditLogsService.All] reads every page.
//
// The API's documentation says that it reads at most the first 10000 audit logs of a list, and
// answers a page past them, such as page 101 of 100 logs, with 400: narrow a longer list with
// the filters, such as the time window of TimestampFrom and TimestampTo.
//
// List is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [AuditLogsService] says. The call changes nothing, so it sends its request
// again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// List returns every failure as an error, and never a page with it:
//
//   - a scope without an environment: an error that matches [ErrInvalidConfig], and no request
//     for the list; without WithScope, the lookup of the scope comes first;
//   - a Page or a PerPage less than zero: an error that matches ErrInvalidConfig, and no request
//     for the list;
//   - a reply with an error status, such as 400 for a page past the first 10000 audit logs, or
//     422 for a PerPage above 100 or a PDPID that is not a UUID: an [Error] with the status and
//     the body of the reply;
//   - a reply in 200-299 that is not the paginated JSON object of audit logs: an Error that says
//     so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *AuditLogsService) List(ctx context.Context, opts *AuditLogsListOptions,
) (*Page[models.AuditLogModel], error) {
	r, page := auditLogsRequest(opts)
	r, err := s.client.inScope(ctx, r)
	if err != nil {
		return nil, err
	}
	return listPage[models.AuditLogModel](ctx, s.client, r, pageShape, page)
}

// All returns an iterator over the audit logs of the environment that match each filter of opts,
// for a range loop. It reads the pages in turn, as [AuditLogsService.List] reads one, from the
// page of opts.Page on, or the first, each of opts.PerPage audit logs, or else of 100, the most
// that the API gives on a page. It stops after the last page that the replies count, or after the
// first page without audit logs, and never after a page with fewer audit logs than it asked for.
// It asks for the next page only when the loop wants an audit log past the last of a page, so a
// loop that stops early sends no other request. It reads opts when it is called: a later change
// to opts does not change the iterator. Each loop over the iterator reads the audit logs again,
// from the first page that opts asks for. The pages are requests of their own, so a decision
// stored during a loop can move the others from one page to the next. The API reads at most the
// first 10000 audit logs of a list, as List says.
//
// All is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as List is; each loop gets the scope as List does before its first page. Each
// page is a call of its own, with its own timeout and retries. When the scope or a page fails,
// with one of the errors that List returns, the iterator yields its error once, with the zero
// AuditLogModel, and stops.
func (s *AuditLogsService) All(ctx context.Context, opts *AuditLogsListOptions,
) iter.Seq2[models.AuditLogModel, error] {
	r, page := auditLogsRequest(opts)
	return allInScope[models.AuditLogModel](ctx, s.client, r, pageShape, page)
}

// Get returns the audit log whose ID is logID, a UUID, with the decision log of the engine of the
// PDP and the objects that the decision names, with
// GET /v2/pdps/{proj_id}/{env_id}/audit_logs/{log_id}. It sends logID as one segment of the path,
// escaped.
//
// Get is sent with an environment-level API key, or with a key of a wider level and
// [WithScope], as [AuditLogsService] says. The call changes nothing, so it sends its request
// again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no audit log:
//
//   - for a scope without an environment, or a logID that is empty, . or ..: an error that
//     matches [ErrInvalidConfig], and no request for the audit log; without WithScope, the
//     lookup of the scope comes first;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     such as one that matches [ErrNotFound] when the environment has no such audit log, or a
//     422 for a logID that is not a UUID;
//   - for a reply in 200-299 that is not a JSON object of an audit log: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *AuditLogsService) Get(ctx context.Context, logID string,
) (*models.DetailedAuditLogModel, error) {
	r, err := s.client.inScope(ctx, request{
		op: opGetDetailedAuditLog, target: apiTarget, params: []string{logID},
	})
	if err != nil {
		return nil, err
	}
	return do[models.DetailedAuditLogModel](ctx, s.client, r)
}
