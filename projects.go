package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the projects of the organization.
const (
	opListProjects  = "list_projects"
	opCreateProject = "create_project"
	opGetProject    = "get_project"
	opUpdateProject = "update_project"
	opDeleteProject = "delete_project"
)

// ProjectsService lists, reads, creates, changes and deletes the projects of the organization of
// the client's API key; [Client.Projects] holds it. A method that acts on one project takes its
// key or its ID, such as my-project, and sends it as one segment of the path, escaped: a key that
// holds a / or a % names that project, not another route.
//
// Its calls need no scope: they send no lookup of the scope of the API key, and fill in no
// project and no environment from [WithScope], so they work with an organization-level API key,
// whose scope has neither. Each method says the level of API key that it needs; the API answers
// a key of a narrower level with an error status, which the call returns as an [Error], such as
// one that matches [ErrForbidden].
type ProjectsService struct {
	client *Client
}

// List returns one page of the projects of the organization, in the order of the API's reply,
// with GET /v2/projects. A nil opts, like the zero ListOptions, asks for the first page, with the
// API's default of 30 projects on it; the API gives at most 100 on a page. The reply reports no
// counts, so the TotalCount and the PageCount of the page are nil. [ProjectsService.All] reads
// every page.
//
// List needs an organization-level API key. It needs no scope, as [ProjectsService] says, so an
// organization-level key, whose scope has no project and no environment, lists the projects
// without WithScope. The call changes nothing, so it sends its request again after a 500, 502,
// 503 or 504 reply or a failed connection, as after a 429.
//
// List returns every failure as an error, and never a page with it:
//
//   - a Page or a PerPage less than zero: an error that matches [ErrInvalidConfig], and no
//     request;
//   - a reply with an error status, such as 403 for a key that may not list the projects, or 422
//     for a PerPage above 100: an [Error] with the status and the body of the reply;
//   - a reply in 200-299 that is not a JSON array of projects: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ProjectsService) List(ctx context.Context, opts *ListOptions,
) (*Page[models.ProjectRead], error) {
	r := request{op: opListProjects, target: apiTarget}
	return listPage[models.ProjectRead](ctx, s.client, r, arrayShape, listOptions(opts))
}

// All returns an iterator over the projects of the organization, for a range loop. It reads the
// pages in turn, as [ProjectsService.List] reads one, from the page of opts.Page on, or the
// first, each of opts.PerPage projects, or else of 100, the most that the API gives on a page.
// It stops after the first page without projects, and never after a page with fewer projects
// than it asked for. It asks for the next page only when the loop wants a project past the last
// of a page, so a loop that stops early sends no other request. It reads opts when it is called:
// a later change to opts does not change the iterator. Each loop over the iterator reads the
// projects again, from the first page that opts asks for. The pages are requests of their own,
// so a project created or deleted during a loop can move the others from one page to the next.
//
// All needs an organization-level API key, and no scope, as List does. Each page is a call of
// its own, with its own timeout and retries. When a page fails, with one of the errors that
// List returns, the iterator yields its error once, with the zero ProjectRead, and stops.
func (s *ProjectsService) All(ctx context.Context, opts *ListOptions,
) iter.Seq2[models.ProjectRead, error] {
	r := request{op: opListProjects, target: apiTarget}
	return all[models.ProjectRead](ctx, s.client, r, arrayShape, listOptions(opts))
}

// Get returns the project whose key or ID is projectKey, with GET /v2/projects/{proj_id}.
//
// Get needs a project-level API key of the project, or higher: an organization-level key may
// read any project of the organization. It needs no scope. The call changes nothing, so it sends
// its request again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no project:
//
//   - for a projectKey that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the organization has no such project;
//   - for a reply in 200-299 that is not a JSON object of a project: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ProjectsService) Get(ctx context.Context, projectKey string,
) (*models.ProjectRead, error) {
	r := request{op: opGetProject, target: apiTarget, params: []string{projectKey}}
	return do[models.ProjectRead](ctx, s.client, r)
}

// Create creates a project in the organization with POST /v2/projects, and returns it as the
// API stores it, with its ID. The API also creates the project's environments, as
// [models.ProjectCreate.InitialEnvironments] says: by default, Development and Production.
//
// Create needs an organization-level API key, and no scope. It is sent again only after a 429,
// which the API did not act on: after a 500, 502, 503 or 504 reply or a failed connection, the
// API may have created the project, so Create returns that error, and [ProjectsService.Get] of
// the key tells whether the project exists.
//
// Create returns an error, and no project:
//
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrConflict] when the organization has a project with the key already, and
//     whose Detail names each field that the API rejects for a 422, such as a key that is not
//     URL-friendly;
//   - for a reply in 200-299 that is not a JSON object of a project: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ProjectsService) Create(ctx context.Context, project models.ProjectCreate,
) (*models.ProjectRead, error) {
	r := request{op: opCreateProject, target: apiTarget, body: project}
	return do[models.ProjectRead](ctx, s.client, r)
}

// Update changes the project whose key or ID is projectKey with
// PATCH /v2/projects/{proj_id}, and returns it as the API stores it. The request holds the
// fields of update that are set, as [models.ProjectUpdate] says, and the project keeps the
// others: the zero ProjectUpdate changes nothing.
//
// Update needs a project-level API key of the project, or higher, and no scope. Sending the
// same update twice sets the same fields, so the call sends its request again after a 500, 502,
// 503 or 504 reply or a failed connection, as after a 429.
//
// Update returns an error, and no project:
//
//   - for a projectKey that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the organization has no such project, and whose Detail
//     names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of a project: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ProjectsService) Update(ctx context.Context, projectKey string,
	update models.ProjectUpdate,
) (*models.ProjectRead, error) {
	r := request{
		op: opUpdateProject, target: apiTarget, params: []string{projectKey}, body: update,
	}
	return do[models.ProjectRead](ctx, s.client, r)
}

// Delete deletes the project whose key or ID is projectKey, with its environments and all their
// data, with DELETE /v2/projects/{proj_id}. It returns nil when the API answers 404, as the
// project is gone already, such as when an earlier attempt of the call deleted it; so a mistyped
// key returns nil too, and deletes nothing.
//
// Delete needs a project-level API key of the project, or higher, and no scope. Deleting a
// project twice leaves it deleted, so the call sends its request again after a 500, 502, 503 or
// 504 reply or a failed connection, as after a 429.
//
// Delete returns an error:
//
//   - for a projectKey that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete the
//     project;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *ProjectsService) Delete(ctx context.Context, projectKey string) error {
	r := request{op: opDeleteProject, target: apiTarget, params: []string{projectKey}}
	return s.client.doNoValue(ctx, r)
}
