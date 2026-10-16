package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/internal/wire"
)

// opLocalRoleAssignments is the ID of the operation of the role assignments that the PDP
// container holds, which only the container serves.
const opLocalRoleAssignments = "list_role_assignments_local_role_assignments_get"

// LocalRoleAssignment is a role assignment that the PDP container holds, as
// [Client.LocalRoleAssignments] returns it: a role of a user in a tenant, or on a resource
// instance in a tenant.
type LocalRoleAssignment struct {
	// User is the key of the user that the role is assigned to, such as alice.
	User string `json:"user"`

	// Role is the key of the role, such as editor.
	Role string `json:"role"`

	// Tenant is the key of the tenant of the role assignment, such as tenant-a.
	Tenant string `json:"tenant"`

	// ResourceInstance is the resource instance that the role is assigned on, as the PDP names
	// it, such as document:2026-report, or nil for a role in the tenant: the reply holds null for
	// it, or leaves it out.
	ResourceInstance *string `json:"resource_instance,omitzero"`
}

// LocalRoleAssignmentsOptions are the page and the filters of [Client.LocalRoleAssignments] and
// [Client.LocalRoleAssignmentsAll]. Each filter that is not empty keeps only the role assignments
// that match it, and the call sends it as it is given, in the query parameter that its doc
// comment names. An empty filter, as each is in the zero LocalRoleAssignmentsOptions, is not sent
// and keeps every role assignment.
type LocalRoleAssignmentsOptions struct {
	// ListOptions picks the page and the number of role assignments on it, sent in page and
	// per_page. With a PerPage of 0, LocalRoleAssignments leaves the number to the PDP container,
	// which gives 30, and LocalRoleAssignmentsAll asks for 100, the most that the container gives.
	ListOptions

	// User keeps the role assignments of the user with this key, such as alice. Its query
	// parameter is user.
	User string

	// Role keeps the role assignments of the role with this key, such as editor. Its query
	// parameter is role.
	Role string

	// Tenant keeps the role assignments in the tenant with this key, such as tenant-a. Its query
	// parameter is tenant.
	Tenant string

	// Resource keeps the role assignments on the resource type with this key, such as document.
	// Its query parameter is resource.
	Resource string

	// ResourceInstance keeps the role assignments on this resource instance, named as the
	// ResourceInstance of a [LocalRoleAssignment] is, such as document:2026-report. Its query
	// parameter is resource_instance.
	ResourceInstance string
}

// localRoleAssignmentsRequest returns the request of the role assignments that opts filters, with
// a parameter in its query for each filter of opts that is not empty, and the page of opts. A nil
// opts is the zero LocalRoleAssignmentsOptions. The request holds copies of the values of opts, so
// a later change to opts does not change it.
func localRoleAssignmentsRequest(opts *LocalRoleAssignmentsOptions) (request, ListOptions) {
	var o LocalRoleAssignmentsOptions
	if opts != nil {
		o = *opts
	}
	query := &wire.Query{}
	for _, filter := range []struct{ name, value string }{
		{"user", o.User},
		{"role", o.Role},
		{"tenant", o.Tenant},
		{"resource", o.Resource},
		{"resource_instance", o.ResourceInstance},
	} {
		if filter.value != "" {
			query.AddString(filter.name, &filter.value)
		}
	}
	return request{op: opLocalRoleAssignments, target: pdpTarget, query: query}, o.ListOptions
}

// LocalRoleAssignments returns one page of the role assignments that the PDP container holds,
// those that match each filter of opts, in the order of the PDP's reply. They are the role
// assignments that the container's policy reads, so a role assignment made through the Permit
// API is listed once it reaches the container. A nil opts, like the zero
// LocalRoleAssignmentsOptions, asks for the first page of every role assignment, with the PDP's
// default of 30 role assignments on it. The reply reports no counts, so the TotalCount and the
// PageCount of the page are nil. [Client.LocalRoleAssignmentsAll] reads every page.
//
// LocalRoleAssignments is container PDP only: it sends one request, GET /local/role_assignments,
// to the PDP at the PDP URL, [WithPDPURL], which only the PDP container serves. The cloud PDP
// answers it with 404 and an empty body, which the call returns, after that one request, as an
// [Error] that matches [ErrContainerPDPOnly] and names the route. LocalRoleAssignments needs an
// environment-level API key. The call changes nothing, so it sends its request again as
// [Client.Check] does.
//
// LocalRoleAssignments returns every failure as an error, and never a page with it:
//
//   - a Page or a PerPage less than zero: an error that matches [ErrInvalidConfig], and no
//     request;
//   - a reply with an error status, such as 422 for a PerPage above 100, or 429 after the
//     retries: an [Error] with the status and the body of the reply;
//   - a reply in 200-299 that is not a JSON array, such as null or an object, or that holds an
//     item whose user, role, tenant or resource_instance is not a string: an Error that says
//     so. An item that is null, or leaves out a field, decodes with the field empty;
//   - a request that got no reply, such as one whose connection was refused: an Error whose Err
//     wraps the cause, such as a *url.Error.
func (c *Client) LocalRoleAssignments(ctx context.Context, opts *LocalRoleAssignmentsOptions,
) (*Page[LocalRoleAssignment], error) {
	r, page := localRoleAssignmentsRequest(opts)
	return listPage[LocalRoleAssignment](ctx, c, r, arrayShape, page)
}

// LocalRoleAssignmentsAll returns an iterator over the role assignments that the PDP container
// holds, those that match each filter of opts, for a range loop. It reads the pages in turn, as
// [Client.LocalRoleAssignments] reads one, from the page of opts.Page on, or the first, each of
// opts.PerPage role assignments, or else of 100, the most that the container gives on a page. It
// stops after the first page without role assignments, and never after a page with fewer role
// assignments than it asked for. It asks for the next page only when the loop wants a role
// assignment past the last of a page, so a loop that stops early sends no other request. It
// reads opts when it is called: a later change to opts does not change the iterator. Each loop
// over the iterator reads the role assignments again, from the first page that opts asks for.
// The pages are requests of their own, so a role assignment that reaches the container during a
// loop can move the others from one page to the next.
//
// LocalRoleAssignmentsAll is container PDP only: each page is one request,
// GET /local/role_assignments, to the PDP at the PDP URL, [WithPDPURL], which only the PDP
// container serves. The cloud PDP answers the first with 404 and an empty body, which the loop
// gets, after that one request, as an [Error] that matches [ErrContainerPDPOnly] and names the
// route. LocalRoleAssignmentsAll needs an environment-level API key. Each page is a call of its
// own, with its own timeout and retries.
//
// When a page fails, with one of the errors that LocalRoleAssignments returns, the iterator
// yields its error once, with the zero LocalRoleAssignment, and stops.
func (c *Client) LocalRoleAssignmentsAll(ctx context.Context, opts *LocalRoleAssignmentsOptions,
) iter.Seq2[LocalRoleAssignment, error] {
	r, page := localRoleAssignmentsRequest(opts)
	return all[LocalRoleAssignment](ctx, c, r, arrayShape, page)
}
