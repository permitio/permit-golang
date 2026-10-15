package permit_test

import (
	"context"
	"iter"
	"net/http"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/internal/testkit/mockpermit"
)

// pdpPerPageBody is the error body of the PDP container for a per_page above the most role
// assignments that it gives on a page.
const pdpPerPageBody = `{"detail":[{"loc":["query","per_page"],` +
	`"msg":"ensure this value is less than or equal to 100",` +
	`"type":"value_error.number.not_le","ctx":{"limit_value":100}}]}`

// localAssignments are the role assignments of the reply fixture of the role assignments that the
// PDP container holds: a role in a tenant, and a role on a resource instance.
func localAssignments() []permit.LocalRoleAssignment {
	return []permit.LocalRoleAssignment{
		{User: "alice", Role: "admin", Tenant: "tenant-a"},
		{
			User: "alice", Role: "editor", Tenant: "tenant-a",
			ResourceInstance: new("document:2026-report"),
		},
	}
}

// firstAssignments returns the first n role assignments that seq yields, and ends the loop
// there, so that it asks for no page past them; or the first error that seq yields.
func firstAssignments(seq iter.Seq2[permit.LocalRoleAssignment, error], n int,
) ([]permit.LocalRoleAssignment, error) {
	var got []permit.LocalRoleAssignment
	for assignment, err := range seq {
		if err != nil {
			return nil, err
		}
		got = append(got, assignment)
		if len(got) == n {
			break
		}
	}
	return got, nil
}

// localCases returns the wire cases of the role assignments that the PDP container holds: a page,
// and the first page of the iterator, which reads the role assignments of that page and asks for
// no other. They go to the PDP and never to the API.
func localCases() []wireCase {
	perPageErr := &permit.Error{
		Op: "list_role_assignments_local_role_assignments_get", Method: http.MethodGet,
		Route: "/local/role_assignments", StatusCode: http.StatusUnprocessableEntity,
		Detail: "query.per_page: ensure this value is less than or equal to 100",
		Body:   []byte(pdpPerPageBody),
	}
	return []wireCase{
		{
			Method: "Client.LocalRoleAssignments",
			Op:     "list_role_assignments_local_role_assignments_get",
			Route:  "GET /local/role_assignments",
			Modes:  []wireMode{pdpMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return c.LocalRoleAssignments(ctx, &permit.LocalRoleAssignmentsOptions{
					ListOptions: permit.ListOptions{Page: 2, PerPage: 10},
					User:        "alice",
					Tenant:      "tenant-a",
				})
			},
			Request: wireRequest{
				RawPath:  "/local/role_assignments",
				RawQuery: "page=2&per_page=10&tenant=tenant-a&user=alice",
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("list_role_assignments_local_role_assignments_get.200.json"),
			},
			WantResult: &permit.Page[permit.LocalRoleAssignment]{
				Data: localAssignments(), Page: 2, PerPage: 10,
			},
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: pdpPerPageBody,
			},
			WantErr: perPageErr,
		},
		{
			Method: "Client.LocalRoleAssignmentsAll",
			Op:     "list_role_assignments_local_role_assignments_get",
			Route:  "GET /local/role_assignments",
			Modes:  []wireMode{pdpMode},
			Call: func(ctx context.Context, c *permit.Client) (any, error) {
				return firstAssignments(c.LocalRoleAssignmentsAll(ctx,
					&permit.LocalRoleAssignmentsOptions{User: "alice"}), 2)
			},
			Request: wireRequest{
				RawPath:  "/local/role_assignments",
				RawQuery: "page=1&per_page=100&user=alice",
			},
			Reply: mockpermit.Reply{
				Body: replyFixture("list_role_assignments_local_role_assignments_get.200.json"),
			},
			WantResult: localAssignments(),
			ErrorReply: mockpermit.Reply{
				Status: http.StatusUnprocessableEntity, Body: pdpPerPageBody,
			},
			WantErr: perPageErr,
		},
	}
}
