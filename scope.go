package permit

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/permitio/permit-golang/v2/internal/scope"
	"github.com/permitio/permit-golang/v2/internal/validate"
	"github.com/permitio/permit-golang/v2/internal/wire"
	"github.com/permitio/permit-golang/v2/models"
)

// opGetAPIKeyScope is the ID of the operation that returns the scope of the client's API key.
const opGetAPIKeyScope = "get_api_key_scope" //nolint:gosec // G101: an operation ID, no credential

// Scope is the project and the environment that the calls of a [Client] act on, each a key or an
// ID: those that [WithScope] gives, or else those of the client's API key, which [Client.Scope]
// looks up.
type Scope struct {
	// Project is the key or the ID of the project, or empty when the client acts on no project,
	// as with an organization-level API key and without WithScope.
	Project string

	// Environment is the key or the ID of the environment, or empty when the client acts on no
	// environment, as with an organization-level or a project-level API key and without
	// WithScope, or with WithScope and an empty environment.
	Environment string
}

// Scope returns the project and the environment that the client's calls act on. With
// [WithScope], it returns what the option gives and sends no request. Without it, the client
// looks up the scope of its API key with one request, GET /v2/api-key/scope, which Scope or the
// first call that needs the scope sends; the calls that need the scope meanwhile wait for that
// request, and the client keeps the scope for its life. A lookup that fails is not kept, so the
// next call that needs the scope looks it up again. Call Scope after [New] to check the API key
// and its scope at once, rather than with the first call that needs them.
//
// Any API key may call it, an environment-level API key too. The scope of an organization-level
// key has no project and no environment, and that of a project-level key has no environment.
//
// Scope returns the error of a lookup that failed as the API answered it: an [Error] with the
// reply's status, code and body, which matches a sentinel such as [ErrUnauthorized],
// [ErrForbidden] or, after the retries that [WithRetry] allows, [ErrRateLimited]. When ctx ends
// while Scope waits, it returns an Error that matches the error of ctx, such as
// [context.Canceled], at once. The lookup runs on for the other calls that wait for it, bounded by
// the client's timeout, [WithTimeout], and not by the deadline of ctx. After [Client.Close], Scope
// returns an error that matches [ErrClosed].
func (c *Client) Scope(ctx context.Context) (Scope, error) {
	if err := c.checkOpen(); err != nil {
		return Scope{}, err
	}
	s, err := c.scope.Get(ctx)
	if err == nil {
		return Scope(s), nil
	}
	if _, ok := errors.AsType[*Error](err); ok || errors.Is(err, ErrClosed) {
		return Scope{}, err
	}
	// Any other error is that of ctx, which ended while the call waited for the lookup.
	op, _ := c.registry.Op(opGetAPIKeyScope)
	return Scope{}, opCall(op).transportError(err, c.secrets)
}

// lookupScope looks up the scope of the client's API key, for the client's scope.Resolver. It
// returns the error of the request, or ErrClosed when the client is closed, as Close ends the
// lookup.
func (c *Client) lookupScope(ctx context.Context) (scope.Scope, error) {
	read, err := do[models.APIKeyScopeRead](ctx, c,
		request{op: opGetAPIKeyScope, target: apiTarget})
	if err != nil && c.checkOpen() != nil {
		return scope.Scope{}, ErrClosed
	}
	if err != nil {
		return scope.Scope{}, err
	}
	return scope.Scope{Project: read.ProjectID, Environment: read.EnvironmentID}, nil
}

// scopeFor returns the project and the environment that a call of the operation id acts on, for
// the {proj_id} and {env_id} of its route, as the route needs them: both for a route with
// {env_id}; the project, and no environment, for a route with {proj_id} and no {env_id}; and
// none for a route with neither, such as one of the organization, which looks nothing up. The
// KeyLevel of the operation, the narrowest key that may call it, plays no part. It gets the
// scope as Client.Scope does, and returns its errors. When the scope lacks what the route needs,
// it returns an error that matches ErrInvalidConfig and names the operation, the part that is
// missing and WithScope, so that the call sends no request for the operation.
func (c *Client) scopeFor(ctx context.Context, id string) (project, environment string,
	err error,
) {
	op, ok := c.registry.Op(id)
	if !ok {
		return "", "", fmt.Errorf("permit: the operation %s is not in the client's operation "+
			"registry", id)
	}
	placeholders, err := wire.Placeholders(op.Route)
	if err != nil {
		return "", "", fmt.Errorf("permit: %s: %w", op.ID, err)
	}
	needsEnvironment := slices.Contains(placeholders, "env_id")
	if !needsEnvironment && !slices.Contains(placeholders, "proj_id") {
		return "", "", nil
	}
	s, err := c.Scope(ctx)
	if err != nil {
		return "", "", err
	}
	if s.Project == "" {
		return "", "", &configError{setting: op.ID, rule: validate.ScopeProjectMissing}
	}
	if !needsEnvironment {
		return s.Project, "", nil
	}
	if s.Environment == "" {
		return "", "", &configError{setting: op.ID, rule: validate.ScopeEnvironmentMissing}
	}
	return s.Project, s.Environment, nil
}
