# Go SDK for Permit.io

The Go client of [Permit.io](https://www.permit.io): it asks the policy decision point (PDP)
whether a user may do an action on a resource, and manages the policy and its data, such as
users, tenants, roles and resources, through the Permit API.

## Status

This branch holds v2 of the SDK, the module `github.com/permitio/permit-golang/v2`. v2 is under
development and is not released: the `go get` below works once it is.

If you use v1, the module `github.com/permitio/permit-golang`, stay on v1.3.x until v2 is
released. v1.4.0 will be the last v1 release, and v1 gets no fixes after it. See
[SECURITY.md](SECURITY.md), and [MIGRATION.md](MIGRATION.md) for the move to v2.

## Quick start

You need Go 1.26 or later, an API key of a Permit environment, and a PDP: by default the client
asks a PDP container at `http://localhost:7766`, as
[The PDP container and the cloud PDP](#the-pdp-container-and-the-cloud-pdp) says.

```sh
go get github.com/permitio/permit-golang/v2
```

This program creates the user alice in the environment of the API key, or replaces her when she
exists, asks the PDP whether she may read the document doc-1 in the tenant tenant-a, and closes
the client:

<!-- compile -->
```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	// Without WithAPIKey, New reads the key from the environment variable PERMIT_API_KEY.
	client, err := permit.New(permit.WithAPIKey("permit_key_..."))
	if err != nil {
		return err
	}
	defer client.Close()

	synced, err := client.Users.Sync(ctx, models.UserCreate{
		Key:   "alice",
		Email: "alice@example.com",
	})
	if err != nil {
		return err
	}
	fmt.Println("created alice:", synced.Created)

	allowed, err := client.Check(ctx,
		permit.User{Key: "alice"},
		"read",
		permit.Resource{Type: "document", Key: "doc-1", Tenant: "tenant-a"},
	)
	if err != nil {
		return err
	}
	fmt.Println("alice may read doc-1:", allowed)
	return nil
}
```

`Check` returns `false` only when the PDP denies; a failure, such as a PDP that does not answer,
is an error. A `Client` is safe for concurrent use: build one and share it.

## Configuration

`permit.New` takes options. A setting that no option gives comes from its environment variable,
when the variable is set and not empty, and else from its default. When options give the same
setting twice, the last one wins.

| Option | Environment variable | Default |
| --- | --- | --- |
| `WithAPIKey(key)` | `PERMIT_API_KEY` | None: `New` fails without a key. |
| `WithAPIURL(url)` | `PERMIT_API_URL` | `https://api.permit.io` |
| `WithPDPURL(url)` | `PERMIT_PDP_URL` | `http://localhost:7766` |
| `WithTimeout(d)` | | 30 seconds for a whole call, its retries included. |
| `WithRetry(policy)` | | Up to 3 retries, as [Retries](#retries) says. |
| `WithHTTPClient(c)` | | An `*http.Client` of the client's own. |
| `WithLogger(logger)` | | None: the client logs nothing. |
| `WithScope(project, environment)` | | The scope of the API key, as [API key levels](#api-key-levels) says. |
| `WithFactsViaPDP()` | | Facts go to the Permit API, as [Facts through the PDP container](#facts-through-the-pdp-container) says. |

A URL uses `http` or `https` and has a host, and has no user name or password, query or fragment.
A path after the host, such as `/permit` in `https://proxy.example.com/permit`, is kept and
starts every route.

`New` sends no request. It returns an error that matches `permit.ErrInvalidConfig`, and no client,
when the configuration breaks a rule, such as a URL without `http://` or `https://`; the error
names the option or the environment variable, and never holds the API key. A client prints its
settings with `[REDACTED]` in place of the key:

<!-- compile -->
```go
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/permitio/permit-golang/v2"
)

func main() {
	client, err := permit.New(
		permit.WithAPIKey("permit_key_..."),
		permit.WithPDPURL("http://localhost:7766"),
		permit.WithTimeout(10*time.Second),
		permit.WithRetry(permit.RetryPolicy{MaxRetries: 5, MaxBackoff: 2 * time.Second}),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println(client)
}
```

## Which URL each call uses

The client sends each call to one of two servers: the Permit API, at the API URL, by default
`https://api.permit.io`, or the PDP, at the PDP URL, by default `http://localhost:7766`. The
decisions, such as `client.Check`, go to the PDP; the services, such as `client.Users`, go to the
API, and the calls of the facts API go to the PDP container instead with `WithFactsViaPDP`. The
documentation of each method on
[pkg.go.dev](https://pkg.go.dev/github.com/permitio/permit-golang/v2) names its route.

<!-- table servers -->

| Call | URL |
| --- | --- |
| `client.AllTenantsCheck` | PDP URL |
| `client.AuthorizedUsers` | PDP URL |
| `client.BulkCheck` | PDP URL |
| `client.Check` | PDP URL |
| `client.CheckURL` | PDP URL |
| `client.FilterObjects` | PDP URL |
| `client.LocalRoleAssignments` | PDP URL |
| `client.LocalRoleAssignmentsAll` | PDP URL |
| `client.UserPermissions` | PDP URL |
| `client.UserTenants` | PDP URL |
| `client.APIKeys.*` | API URL |
| `client.AuditLogs.*` | API URL |
| `client.ConditionSetRules.*` | API URL |
| `client.ConditionSets.*` | API URL |
| `client.Elements.*` | API URL |
| `client.Environments.*` | API URL |
| `client.Groups.*` | API URL |
| `client.ImplicitGrants.*` | API URL |
| `client.PDPs.*` | API URL |
| `client.Projects.*` | API URL |
| `client.ProxyConfigs.*` | API URL |
| `client.ResourceActionGroups.*` | API URL |
| `client.ResourceActions.*` | API URL |
| `client.ResourceAttributes.*` | API URL |
| `client.ResourceRelations.*` | API URL |
| `client.ResourceRoles.*` | API URL |
| `client.Resources.*` | API URL |
| `client.Roles.*` | API URL |
| `client.Scope` | API URL |
| `client.UserAttributes.*` | API URL |
| `client.UserInvites.*` | API URL |
| `client.RelationshipTuples.*` | API URL, or PDP URL with `WithFactsViaPDP` |
| `client.ResourceInstances.*` | API URL, or PDP URL with `WithFactsViaPDP` |
| `client.RoleAssignments.*` | API URL, or PDP URL with `WithFactsViaPDP` |
| `client.Tenants.*` | API URL, or PDP URL with `WithFactsViaPDP` |
| `client.Users.*` | API URL, or PDP URL with `WithFactsViaPDP` |

<!-- end table -->

## The PDP container and the cloud PDP

The PDP decides with the policy and the facts of one environment, which it syncs from Permit. It
runs in one of two ways:

- The PDP container runs next to your application: the image `permitio/pdp-v2`, which serves on
  port 7000, and reads the API key of its environment from the environment variable
  `PDP_API_KEY` and the URL of the Permit API from `PDP_CONTROL_PLANE`. With the key in
  `PDP_API_KEY`, this runs it on the port of the default PDP URL, `http://localhost:7766`, as the
  end-to-end tests of this repository do:

  ```sh
  docker run --detach --publish 127.0.0.1:7766:7000 --env PDP_API_KEY \
    --env PDP_CONTROL_PLANE=https://api.permit.io permitio/pdp-v2:<version>
  ```

- The cloud PDP is run by Permit: give its URL, `https://cloudpdp.api.permit.io`, to
  `WithPDPURL` or `PERMIT_PDP_URL`.

Both serve the calls that the table above sends to the PDP URL, such as `client.Check` and
`client.BulkCheck`, but for those below, which only the PDP container serves. The cloud PDP
answers each of them with status 404 and an empty body, which the call returns as an error that
matches `permit.ErrContainerPDPOnly`, not `permit.ErrNotFound`:

<!-- table container-only -->

| Call | Route |
| --- | --- |
| `client.AllTenantsCheck` | `POST /allowed/all-tenants` |
| `client.CheckURL` | `POST /allowed_url` |
| `client.LocalRoleAssignments` | `GET /local/role_assignments` |
| `client.LocalRoleAssignmentsAll` | `GET /local/role_assignments` |
| `client.UserTenants` | `POST /user-tenants` |

<!-- end table -->

With `WithFactsViaPDP`, the calls of the facts API go to the PDP too, and only the PDP container
serves them.

## Facts through the PDP container

Facts are the data that policies decide on: users, tenants and the users in them, role
assignments, resource instances and relationship tuples. The client writes and reads them through
the Permit API, and a PDP gets each change from Permit after the write returns, so a check right
after a write can miss it.

With `WithFactsViaPDP`, the client sends these calls to the PDP container at the PDP URL, under
`/facts`, and the container sends each on to Permit. A call through the container acts on the
environment of the container's own API key, so it needs no scope. `New` logs a warning when
`WithFactsViaPDP` is given with the URL of the cloud PDP, which does not serve them.

The PDP container can hold its answer to a write until the write reaches its policy, so that a
check made after the write returns sees it. The `WaitForSync` method of a facts service, such as
`client.Users.WaitForSync`, returns a copy of the service whose calls ask for that wait, with a
timeout, and with `permit.WithSyncPolicy` what the container does when the timeout passes:
`permit.SyncPolicyFail` answers with an error, and `permit.SyncPolicyIgnore` with the result of
the write. The container waits on these calls only:

<!-- table waits-for-sync -->

| Call | Route |
| --- | --- |
| `client.RelationshipTuples.Create` | `POST /facts/relationship_tuples` |
| `client.ResourceInstances.Create` | `POST /facts/resource_instances` |
| `client.ResourceInstances.Update` | `PATCH /facts/resource_instances/{instance_id}` |
| `client.RoleAssignments.Assign` | `POST /facts/role_assignments` |
| `client.RoleAssignments.Unassign` | `DELETE /facts/role_assignments` |
| `client.Tenants.Create` | `POST /facts/tenants` |
| `client.Users.AssignRole` | `POST /facts/users/{user_id}/roles` |
| `client.Users.Create` | `POST /facts/users` |
| `client.Users.Sync` | `PUT /facts/users/{user_id}` |
| `client.Users.UnassignRole` | `DELETE /facts/users/{user_id}/roles` |
| `client.Users.Update` | `PATCH /facts/users/{user_id}` |

<!-- end table -->

Any other call through a copy that waits, and a wait from a client without `WithFactsViaPDP`,
returns an error that matches `permit.ErrInvalidConfig` and sends no request.

<!-- compile -->
```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/permitio/permit-golang/v2"
	"github.com/permitio/permit-golang/v2/models"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	client, err := permit.New(
		permit.WithAPIKey("permit_key_..."),
		permit.WithFactsViaPDP(),
	)
	if err != nil {
		return err
	}
	defer client.Close()

	// The container answers once the assignment reaches its policy, or with an error after 10
	// seconds.
	users := client.Users.WaitForSync(10*time.Second,
		permit.WithSyncPolicy(permit.SyncPolicyFail))
	assignment, err := users.AssignRole(ctx, "alice",
		models.UserRoleCreate{Role: "editor", Tenant: "tenant-a"})
	if err != nil {
		return err
	}
	fmt.Println("assigned", assignment.Role, "to", assignment.User)

	allowed, err := client.Check(ctx,
		permit.User{Key: "alice"},
		"edit",
		permit.Resource{Type: "document", Key: "doc-1", Tenant: "tenant-a"},
	)
	if err != nil {
		return err
	}
	fmt.Println("alice may edit doc-1:", allowed)
	return nil
}
```

## API key levels

A Permit API key belongs to an environment, to a project or to the organization. The
documentation of each method names the level of key that it needs: an environment-level,
project-level or organization-level API key. A key of a wider level may make the call too, so an
organization-level key may make every call; a key of a narrower level gets the API's error, such
as status 403, which matches `permit.ErrForbidden`.

A call that acts on a project or an environment, such as one of `client.Users`, fills in the
project and the environment from the client's scope: those of `WithScope(project, environment)`,
each by its key or its ID, or else those of the API key, which the client looks up with one
request, `GET /v2/api-key/scope`, and keeps. What to give `New` depends on the key:

| API key | The scope of the key | What to give `New` |
| --- | --- | --- |
| Environment-level | Its project and its environment | Nothing. |
| Project-level | Its project | `WithScope(project, environment)` for the calls that act on an environment. |
| Organization-level | None | `WithScope(project, environment)` for the calls that act on a project or an environment; `WithScope(project, "")` for a client that acts on a project only. |

A call whose scope lacks the project or the environment that it acts on sends no request for its
operation, and returns an error that matches `permit.ErrInvalidConfig` and names the operation,
such as this one of `client.Users.List` with a project-level key:

```text
permit: invalid configuration: list_users: the operation acts on an environment, and the client's scope has none; pass WithScope(project, environment) to New
```

Without `WithScope`, call `client.Scope` after `New` to look up the scope of the API key, and so
check the key, at once rather than with the first call that needs them. With `WithScope`,
`client.Scope` returns what the option gives and sends no request.

<!-- compile -->
```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/permitio/permit-golang/v2"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	client, err := permit.New(permit.WithAPIKey("permit_key_..."))
	if err != nil {
		return err
	}
	defer client.Close()

	scope, err := client.Scope(ctx)
	if errors.Is(err, permit.ErrUnauthorized) {
		return fmt.Errorf("the API does not accept the API key: %w", err)
	}
	if err != nil {
		return err
	}
	fmt.Println("project:", scope.Project, "environment:", scope.Environment)
	return nil
}
```

## Retries

A call sends its request again after an attempt that failed, up to 3 times by default:

- after status 429 Too Many Requests, to any operation, as the server did not act on the request,
  except an operation whose documentation says that it is sent once, such as rotating an API key;
- after status 500, 502, 503 or 504, or when the request got no complete reply, such as when the
  connection failed, only to an operation that does the same when it is sent twice, such as a
  read or a decision of the PDP. An operation that would repeat what it did, such as a create, is
  sent again only after a 429, and its documentation says so.

Any other status, such as 400, 401, 404 or 409, ends the call, as does a reply in 200-299. Before
a retry, the call waits as the reply's `Retry-After` header asks, in seconds or as a date.
Without the header, it waits a random time from 0 to 200 milliseconds before the first retry, and
from 0 to twice the limit of the previous wait before each later one, up to 5 seconds. The waits
count toward the timeout of the call, `WithTimeout`, or the deadline of its context: a call never
waits past it, nor for a `Retry-After` of more than 60 seconds, and returns the error of its last
attempt instead.

`WithRetry(permit.RetryPolicy{MaxRetries: 5, MaxBackoff: 2 * time.Second})` sets the number of
retries and the longest wait without a `Retry-After`, and `WithRetry(permit.RetryPolicy{})` sends
each request once.

## Errors

A call that fails returns an error that matches a sentinel error, which `errors.Is` finds through
any wrapping, or an error that is or wraps a `*permit.Error`:

| Sentinel | What failed |
| --- | --- |
| `permit.ErrNotFound` | The object does not exist: status 404. |
| `permit.ErrConflict` | The object exists already, or the request conflicts with another: status 409. |
| `permit.ErrUnauthorized` | The API or the PDP did not accept the API key: status 401. |
| `permit.ErrForbidden` | The API key may not do what the request asks: status 403. |
| `permit.ErrRateLimited` | Too many requests: status 429, after the retries. |
| `permit.ErrContainerPDPOnly` | Only the PDP container serves the route, and the PDP that the client calls, such as the cloud PDP, answered 404 with an empty body. |
| `permit.ErrInvalidConfig` | An option or an argument is not one the SDK can use. The call sends no request. |
| `permit.ErrMalformedDecision` | The PDP answered with a reply that is not a valid decision. |
| `permit.ErrClosed` | The call started after `Close`. |

Any other status, such as 400, 422 or 500, matches no sentinel. A `*permit.Error` names the
operation, the method and the route template of the request, and holds the status, the code,
title, detail and request ID of the API's error body, the body itself, and the cause of a request
that got no reply. Its text never holds the body.

<!-- compile -->
```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/permitio/permit-golang/v2"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	client, err := permit.New(permit.WithAPIKey("permit_key_..."))
	if err != nil {
		return err
	}
	defer client.Close()

	user, err := client.Users.Get(ctx, "alice")
	if errors.Is(err, permit.ErrNotFound) {
		fmt.Println("the environment has no user alice")
		return nil
	}
	if permitErr, ok := errors.AsType[*permit.Error](err); ok {
		return fmt.Errorf("status %d, request %s: %w", permitErr.StatusCode, permitErr.RequestID,
			err)
	}
	if err != nil {
		return err
	}
	fmt.Println("alice has the email", user.Email)
	return nil
}
```

## Lists

A `List` method returns one page, a `*permit.Page`: its items in `Data`, and the total count and
the page count when the reply reports them. Its options pick the page and the number of items on
it, `permit.ListOptions{Page: 2, PerPage: 50}`; without them, it returns the first page with the
API's default number of items.

An `All` method returns an iterator over the items of the list, for a range loop. It reads the
pages in turn from the page of its options, or the first, each with `PerPage` items, or by
default the most that the route gives on a page, and asks for the next page only when the loop
wants an item past the last one, so a loop that stops early sends no other request. It stops
after a page without items, or, when it started at the first page, once it read as many items as
the reply's total count. It never stops because a page holds fewer items than it asked for. When
a request fails, the iterator yields the error once and stops:

<!-- compile -->
```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/permitio/permit-golang/v2"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	client, err := permit.New(permit.WithAPIKey("permit_key_..."))
	if err != nil {
		return err
	}
	defer client.Close()

	for tenant, err := range client.Tenants.All(ctx, nil) {
		if err != nil {
			return err
		}
		fmt.Println(tenant.Key, tenant.Name)
	}
	return nil
}
```

## Logging and redaction

By default the client logs nothing. `WithLogger` gives it a `*slog.Logger`, and the logger's
handler keeps the lines at its level:

- at `slog.LevelDebug`, one line for each attempt of a request, with its route template, status,
  attempt number, duration and error, and never a header or a body;
- at `slog.LevelWarn`, a warning from `New` when the API URL uses `http://` to a host that is not
  loopback, and when `WithFactsViaPDP` is given with the URL of the cloud PDP.

<!-- compile -->
```go
package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/permitio/permit-golang/v2"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	client, err := permit.New(
		permit.WithAPIKey("permit_key_..."),
		permit.WithLogger(logger.With("component", "permit")),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
}
```

The client keeps credentials out of its log lines and its errors. It replaces each with
`[REDACTED]`: its API key, when the key has at least 16 bytes; the credentials of at least 8 bytes
that its calls return, such as the secret of an API key, or send in the secret of a proxy config;
every Permit API key; and the value of an `Authorization` header. In an error, it also replaces
the values of the JSON fields of a reply that name a credential, such as `secret`, `token`,
`password` or `api_key`, and keeps at most 8 KiB of the body and 1 KiB of each other text of the
reply. A client and an option print `[REDACTED]` in place of the API key, with every verb of
`fmt` and with `log/slog`. The attributes that your application gives the logger, such as with
`logger.With`, are its own, and the client leaves them as they are.

## Support policy

- v2 supports the two newest major releases of Go, now Go 1.26 and Go 1.27. The `go` line of
  `go.mod`, `go 1.26.0`, is the oldest Go that it builds with, and CI tests Go 1.26.0 and the
  newest patches of Go 1.26 and Go 1.27. The `go` line moves to a newer Go only in a minor
  release of v2, whose entry in [CHANGELOG.md](CHANGELOG.md) says so in a compatibility note.
- v2 follows semantic versioning. Fixes are released in a new 2.x version: update to the latest
  2.x release to get them.
- v1, the module `github.com/permitio/permit-golang`, gets no fixes after v1.4.0, its last
  release, security fixes included. [SECURITY.md](SECURITY.md) says how to report a
  vulnerability.

## Deprecations

None in v2.0.0. A deprecated identifier says so in its documentation, in a paragraph that starts
with `Deprecated:` and names what to use instead, and is listed here.

## More documentation

- The API reference on [pkg.go.dev](https://pkg.go.dev/github.com/permitio/permit-golang/v2):
  each method's documentation says what it sends, the API key it needs, how it retries and the
  errors it returns.
- [MIGRATION.md](MIGRATION.md): each change from v1, and what to use instead.
- [CHANGELOG.md](CHANGELOG.md): what each release changes.
- [SECURITY.md](SECURITY.md): the supported versions, and how to report a vulnerability.
- [CONTRIBUTING.md](CONTRIBUTING.md): how to build, test and change the SDK.
- [llms.txt](llms.txt): an index of these documents for language models.

## License

MIT. See [LICENSE](LICENSE).
