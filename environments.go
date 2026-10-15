package permit

import (
	"context"
	"iter"

	"github.com/permitio/permit-golang/v2/models"
)

// The IDs of the operations of the environments of a project.
const (
	opListEnvironments  = "list_environments"
	opCreateEnvironment = "create_environment"
	opGetEnvironment    = "get_environment"
	opUpdateEnvironment = "update_environment"
	opDeleteEnvironment = "delete_environment"
)

// EnvironmentsService lists, reads, creates, changes and deletes the environments of a project;
// [Client.Environments] holds it. Each method takes the key or the ID of the project, such as
// my-project, and a method that acts on one environment takes its key or its ID too, such as
// production. It sends each as one segment of the path, escaped: a key that holds a / or a %
// names that project or that environment, not another route.
//
// Its calls need no scope: they send no lookup of the scope of the API key, and fill in no
// project and no environment from [WithScope], so they act on the project that they name, also
// with an organization-level API key, whose scope has neither. Each method says the level of API
// key that it is sent with; the API answers a key that may not act on the project with an error
// status, which the call returns as an [Error], such as one that matches [ErrForbidden].
type EnvironmentsService struct {
	client *Client
}

// List returns one page of the environments of the project whose key or ID is projectKey, in the
// order of the API's reply, with GET /v2/projects/{proj_id}/envs. A nil opts, like the zero
// ListOptions, asks for the first page, with the API's default of 30 environments on it; the API
// gives at most 100 on a page. The reply reports no counts, so the TotalCount and the PageCount
// of the page are nil. Each environment is a [models.EnvironmentReadWithEmailConfig], with the ID
// of its email configuration. [EnvironmentsService.All] reads every page.
//
// List is sent with a project-level API key of the project, or an organization-level key, and
// needs no scope. The call changes nothing, so it sends its request again after a 500, 502, 503
// or 504 reply or a failed connection, as after a 429.
//
// List returns every failure as an error, and never a page with it:
//
//   - a projectKey that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request;
//   - a Page or a PerPage less than zero: an error that matches ErrInvalidConfig, and no
//     request;
//   - a reply with an error status, such as 422 for a PerPage above 100: an [Error] with the
//     status and the body of the reply;
//   - a reply in 200-299 that is not a JSON array of environments: an Error that says so;
//   - a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *EnvironmentsService) List(ctx context.Context, projectKey string, opts *ListOptions,
) (*Page[models.EnvironmentReadWithEmailConfig], error) {
	r := request{op: opListEnvironments, target: apiTarget, params: []string{projectKey}}
	return listPage[models.EnvironmentReadWithEmailConfig](ctx, s.client, r, arrayShape,
		listOptions(opts))
}

// All returns an iterator over the environments of the project whose key or ID is projectKey,
// for a range loop. It reads the pages in turn, as [EnvironmentsService.List] reads one, from the
// page of opts.Page on, or the first, each of opts.PerPage environments, or else of 100, the most
// that the API gives on a page. It stops after the first page without environments, and never
// after a page with fewer environments than it asked for. It asks for the next page only when the
// loop wants an environment past the last of a page, so a loop that stops early sends no other
// request. It reads opts when it is called: a later change to opts does not change the iterator.
// Each loop over the iterator reads the environments again, from the first page that opts asks
// for. The pages are requests of their own, so an environment created or deleted during a loop
// can move the others from one page to the next.
//
// All is sent with a project-level API key of the project, or an organization-level key, and
// needs no scope, as List is. Each page is a call of its own, with its own timeout and retries.
// When a page fails, with one of the errors that List returns, the iterator yields its error
// once, with the zero EnvironmentReadWithEmailConfig, and stops.
func (s *EnvironmentsService) All(ctx context.Context, projectKey string, opts *ListOptions,
) iter.Seq2[models.EnvironmentReadWithEmailConfig, error] {
	r := request{op: opListEnvironments, target: apiTarget, params: []string{projectKey}}
	return all[models.EnvironmentReadWithEmailConfig](ctx, s.client, r, arrayShape,
		listOptions(opts))
}

// Get returns the environment whose key or ID is environmentKey, of the project whose key or ID
// is projectKey, with GET /v2/projects/{proj_id}/envs/{env_id}, with the ID of its email
// configuration.
//
// Get is sent with a project-level API key of the project, or an organization-level key, and
// needs no scope. The call changes nothing, so it sends its request again after a 500, 502, 503
// or 504 reply or a failed connection, as after a 429.
//
// Get returns an error, and no environment:
//
//   - for a projectKey or an environmentKey that is empty, . or ..: an error that matches
//     [ErrInvalidConfig], and no request;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the project has no such environment;
//   - for a reply in 200-299 that is not a JSON object of an environment: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *EnvironmentsService) Get(ctx context.Context, projectKey, environmentKey string,
) (*models.EnvironmentReadWithEmailConfig, error) {
	r := request{
		op: opGetEnvironment, target: apiTarget, params: []string{projectKey, environmentKey},
	}
	return do[models.EnvironmentReadWithEmailConfig](ctx, s.client, r)
}

// Create creates an environment in the project whose key or ID is projectKey, with
// POST /v2/projects/{proj_id}/envs, and returns it as the API stores it, with its ID. The request
// holds the settings and the configuration of the JSON Web Key Sets of environment when they are
// set, as [models.EnvironmentCreate] says.
//
// Create is sent with a project-level API key of the project, or an organization-level key, and
// needs no scope. It is sent again only after a 429, which the API did not act on: after a 500,
// 502, 503 or 504 reply or a failed connection, the API may have created the environment, so
// Create returns that error, and [EnvironmentsService.Get] of the key tells whether the
// environment exists.
//
// Create returns an error, and no environment:
//
//   - for a projectKey that is empty, . or ..: an error that matches [ErrInvalidConfig], and no
//     request;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrConflict] when the project has an environment with the key already, and
//     whose Detail names each field that the API rejects for a 422, such as a key that is not
//     URL-friendly;
//   - for a reply in 200-299 that is not a JSON object of an environment: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *EnvironmentsService) Create(ctx context.Context, projectKey string,
	environment models.EnvironmentCreate,
) (*models.EnvironmentRead, error) {
	r := request{
		op: opCreateEnvironment, target: apiTarget, params: []string{projectKey},
		body: environment,
	}
	return do[models.EnvironmentRead](ctx, s.client, r)
}

// Update changes the environment whose key or ID is environmentKey, of the project whose key or
// ID is projectKey, with PATCH /v2/projects/{proj_id}/envs/{env_id}, and returns it as the API
// stores it. The request holds the fields of update that are set, as [models.EnvironmentUpdate]
// says, and the environment keeps the others: the zero EnvironmentUpdate changes nothing. Its
// settings and the configuration of its JSON Web Key Sets are each sent as a whole, or null.
//
// Update is sent with a project-level API key of the project, or an organization-level key, and
// needs no scope. Sending the same update twice sets the same fields, so the call sends its
// request again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Update returns an error, and no environment:
//
//   - for a projectKey or an environmentKey that is empty, . or ..: an error that matches
//     [ErrInvalidConfig], and no request;
//   - for a field of update that cannot be JSON, such as a [models.Some] of nil settings: an
//     error that matches ErrInvalidConfig, and no request;
//   - for a reply with an error status: an [Error] with the status and the body of the reply,
//     which matches [ErrNotFound] when the project has no such environment, and whose Detail
//     names each field that the API rejects for a 422;
//   - for a reply in 200-299 that is not a JSON object of an environment: an Error that says so;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *EnvironmentsService) Update(ctx context.Context, projectKey, environmentKey string,
	update models.EnvironmentUpdate,
) (*models.EnvironmentRead, error) {
	r := request{
		op: opUpdateEnvironment, target: apiTarget, params: []string{projectKey, environmentKey},
		body: update,
	}
	return do[models.EnvironmentRead](ctx, s.client, r)
}

// Delete deletes the environment whose key or ID is environmentKey, of the project whose key or
// ID is projectKey, with all its data, with DELETE /v2/projects/{proj_id}/envs/{env_id}. It
// returns nil when the API answers 404, as the environment is gone already, such as when an
// earlier attempt of the call deleted it; so a mistyped key returns nil too, and deletes nothing.
//
// Delete is sent with a project-level API key of the project, or an organization-level key, and
// needs no scope. Deleting an environment twice leaves it deleted, so the call sends its request
// again after a 500, 502, 503 or 504 reply or a failed connection, as after a 429.
//
// Delete returns an error:
//
//   - for a projectKey or an environmentKey that is empty, . or ..: an error that matches
//     [ErrInvalidConfig], and no request;
//   - for a reply with an error status other than 404: an [Error] with the status and the body
//     of the reply, such as one that matches [ErrForbidden] for a key that may not delete the
//     environment;
//   - for a request that got no reply: an Error whose Err wraps the cause.
//
// After [Client.Close], it returns an error that matches [ErrClosed] and sends no request.
func (s *EnvironmentsService) Delete(ctx context.Context, projectKey, environmentKey string,
) error {
	r := request{
		op: opDeleteEnvironment, target: apiTarget, params: []string{projectKey, environmentKey},
	}
	return s.client.doNoValue(ctx, r)
}
