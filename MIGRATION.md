# Migrating from v1 to v2

v2 of the Go SDK for Permit.io is the module `github.com/permitio/permit-golang/v2`. It replaces
v1, the module `github.com/permitio/permit-golang`, with a new client: the packages, the
identifiers and some requests change. v2 is under development and is not released; this guide
grows with it, and lists every change from v1 when v2 is released.

## Staying on v1

Until you move to v2, pin v1.3.x, the latest v1 release:

```sh
go get github.com/permitio/permit-golang@v1.3
```

v1.4.0 will be the last v1 release, and v1 gets no fixes after it, security fixes included. See
[SECURITY.md](SECURITY.md).

## How to read this guide

Each change is an entry with an ID, such as M-module-1, that stays the same in every later
version of this guide, so a link to an entry keeps working. An entry names what changes in v1 on
its `v1:` lines and what to use in v2 on its `v2:` line. A `v1:` line in backquotes names a v1
identifier, such as `api.Users.Create`, or a group of them, such as `api.Users.*`. When v2 has no
counterpart, the `v2:` line says why: not exposed in v2, alone or with one of the reasons account
administration, dashboard or CLI helper, Elements back-end, operator route, deprecated upstream
or EAP.

The entries are in three buckets:

- Compatibility: what you build with, such as the module path and the Go version.
- API: what you call, such as packages, types, functions and methods.
- Wire behaviour: what the SDK sends and receives for the same call, such as routes, headers and
  retries.

An entry marked "Bug fix: could never work before" changes a v1 feature that could never work as
documented. Code that used it did not get what it asked for, so check what it relied on.

## Compatibility

<a id="M-module-1"></a>
### M-module-1: The module path ends in /v2

- v1: the module `github.com/permitio/permit-golang`
- v2: the module `github.com/permitio/permit-golang/v2`

Go treats a new major version of a module as a module of its own, whose path ends in the major
version. Change the import paths from `github.com/permitio/permit-golang/pkg/...` to v2's
packages: `github.com/permitio/permit-golang/v2` (package `permit`) and
`github.com/permitio/permit-golang/v2/models`. The API entries below say what each v1 identifier
becomes.

<a id="M-module-2"></a>
### M-module-2: v2 needs Go 1.26 or later

- v1: the go directive `go 1.19` of v1's go.mod
- v2: the go directive `go 1.26.0` of v2's go.mod

v2 builds with Go 1.26.0 or later. With an older Go, `go get` of v2 fails, or switches to a newer
toolchain when `GOTOOLCHAIN` allows it.

<a id="M-module-3"></a>
### M-module-3: v2 has no package main

- v1: the example server in `main.go`, package main at the root of the module
- v2: no package main; the module holds library packages only

v1's module root held an example server: tools that load every package of the module built it,
and `go install` could install it. v2 has no command to build or install.

<a id="M-module-4"></a>
### M-module-4: v1 stays on the Go module proxy

- v1: every v1 version on the Go module proxy, proxy.golang.org
- v2: a module of its own beside v1, on the same proxy

The Go module proxy keeps every published version, so a build that requires a v1 version keeps
working after v2 is released. v1 and v2 are separate modules: an application can require both and
move one package at a time.

<a id="M-client-1"></a>
### M-client-1: New reads the configuration from the environment

- v1: a configuration given in code only
- v2: the environment variables `PERMIT_API_KEY`, `PERMIT_API_URL` and `PERMIT_PDP_URL`

`permit.New` reads the API key from `PERMIT_API_KEY`, the API URL from `PERMIT_API_URL` and the
PDP URL from `PERMIT_PDP_URL` when no option gives them. An option wins over its variable, and a
variable that is set to the empty string counts as unset. A process that sets these variables for
another program gives them to the SDK too: to keep a setting from coming from the environment,
pass its option. The SDK for Node.js reads the same variables, but takes an empty one as its
value.

<a id="M-client-2"></a>
### M-client-2: New checks the configuration before it returns

- v1: no check of the configuration when the client is built
- v2: `permit.ErrInvalidConfig` from `permit.New`

`permit.New` returns an error that matches `permit.ErrInvalidConfig`, and no client, when there is
no API key; when an option gives an empty key or URL, a nil `*http.Client` or a timeout of zero or
less; and when a URL does not start with `http://` or `https://`, has no host, or has a user name
or a password, a query or a fragment. The error names the option or the environment variable, and
`permit.New` sends no request. A path after the host of a URL, such as `/permit` in
`https://proxy.example.com/permit`, starts every route, and one slash at the end of a URL is
removed.

## API

<a id="M-errors-1"></a>
### M-errors-1: PermitError becomes *permit.Error

- v1: `errors.PermitError`
- v1: `errors.PermitError.Unwrap`
- v2: `permit.Error`

A call whose request fails returns a `*permit.Error`, or an error that wraps one. It is a
pointer: read it with `errors.AsType[*permit.Error](err)`, or with `errors.As` and a
`*permit.Error` target, instead of a `PermitError` value.

- `StatusCode` keeps its name.
- `ResponseBody`, a string, becomes `Body`, a `[]byte` that holds the body of a reply whose
  status is not in 200-299, cut to 8 KiB, with credentials replaced.
- `ErrorCode` and `ErrorType` give way to `Code`, the error code of the API's error body, and to
  the sentinels of the entries below.
- New fields name the request: `Op`, the ID of the operation, `Method` and `Route`, the route
  template. Others say what the reply said: `Title`, `Detail` and `RequestID`, the id of the
  API's error body. `Err` is the cause, which `Unwrap` returns.

The text of the error has the operation, the method, the route, the status, the code and the
detail, and never the body.

<a id="M-errors-2"></a>
### M-errors-2: Check for a missing object with ErrNotFound

- v1: `errors.NotFound`
- v2: `permit.ErrNotFound`

`errors.Is(err, permit.ErrNotFound)` reports a reply with status 404, unless the error wraps
`permit.ErrContainerPDPOnly`: a route that only the PDP container serves is not a missing object.
Use it instead of comparing `ErrorCode` with `NotFound`.

<a id="M-errors-3"></a>
### M-errors-3: Check for a conflict with ErrConflict

- v1: `errors.Conflict`
- v1: `errors.DuplicateEntity`
- v2: `permit.ErrConflict`

`errors.Is(err, permit.ErrConflict)` reports a reply with status 409, such as the reply to a
create of an object that exists already.

<a id="M-errors-4"></a>
### M-errors-4: Check for a rejected API key with ErrUnauthorized

- v1: `errors.Unauthorized`
- v2: `permit.ErrUnauthorized`

`errors.Is(err, permit.ErrUnauthorized)` reports a reply with status 401.

<a id="M-errors-5"></a>
### M-errors-5: Check for a forbidden request with ErrForbidden

- v1: `errors.ForbiddenAccess`
- v2: `permit.ErrForbidden`

`errors.Is(err, permit.ErrForbidden)` reports a reply with status 403.

<a id="M-errors-6"></a>
### M-errors-6: A request without a reply keeps its cause

- v1: `errors.ConnectionError`
- v2: `permit.Error.Err`

A request that gets no reply, such as one whose connection fails or whose context ends, returns a
`*permit.Error` whose `StatusCode` is 0 and whose `Err` wraps the cause. `errors.Is(err,
context.DeadlineExceeded)` and `errors.Is(err, context.Canceled)` report the end of the context,
and `errors.As` finds a `*url.Error` or a `*net.OpError`.

<a id="M-errors-7"></a>
### M-errors-7: Error codes are the API's

- v1: `errors.ErrorCode`
- v1: `errors.UnexpectedError`
- v1: `errors.EmptyDecisionLogs`
- v1: `errors.MissingRequestAttribute`
- v1: `errors.InvalidPermissionFormat`
- v1: `errors.MissingPermissions`
- v1: `errors.UnsupportedAttributeType`
- v1: `errors.MissingResourceAttribute`
- v1: `errors.InvalidPolicyRepoStatus`
- v1: `errors.MismatchAttributesTypes`
- v2: `permit.Error.Code`

`Code` is a string: the `error_code` of the API's error body as the API spells it, such as
`MISSING_PERMISSIONS` for v1's `MissingPermissions`, or empty when the body has none. For a status
that has no sentinel, such as 400 or 500, which v1 reported as `UnexpectedError`, read
`StatusCode`. A reply with status 429 matches `permit.ErrRateLimited`.

<a id="M-errors-8"></a>
### M-errors-8: A validation error lists its fields in Detail

- v1: `errors.UnprocessableEntityError`
- v2: `permit.Error.Detail`

A request that the API rejects with status 422 returns a `*permit.Error` whose `StatusCode` is 422
and whose `Detail` lists the first 10 fields that failed, each with its message, such as
`body.email: value is not a valid email address`, and then how many more failed.

<a id="M-errors-9"></a>
### M-errors-9: Options and arguments that cannot work match ErrInvalidConfig

- v1: `errors.PaginationError`
- v1: `errors.ContextError`
- v2: `permit.ErrInvalidConfig`

An option of the client or an argument of a call that the SDK cannot use fails before any request
is sent, with an error that matches `permit.ErrInvalidConfig`.

<a id="M-errors-10"></a>
### M-errors-10: Errors have no type

- v1: `errors.ErrorType`
- v1: `errors.*_ERROR`
- v1: `errors.TeapotError`
- v2: not exposed in v2

v2 does not sort errors into types. A `*permit.Error` whose `StatusCode` is not 0 holds a reply of
the API or the PDP, and one whose `StatusCode` is 0 holds a request that got no reply. A call that
sends nothing, because an option or an argument is not valid or the client is closed, returns an
error that matches the sentinel `permit.ErrInvalidConfig` or `permit.ErrClosed`, without a status.
v1 never returned `TeapotError`.

<a id="M-errors-11"></a>
### M-errors-11: Error messages are not constants

- v1: `errors.*Message`
- v2: not exposed in v2

The text of a `*permit.Error` is written from its fields, with the API's detail. Do not match the
text: use `errors.Is` with the sentinels, or read `StatusCode` and `Code`.

<a id="M-errors-12"></a>
### M-errors-12: Only the SDK creates its errors

- v1: `errors.NewPermit*`
- v1: `errors.HttpErrorHandle`
- v2: not exposed in v2

A test that needs an error of the SDK builds a `*permit.Error` literal, such as
`&permit.Error{StatusCode: 404}`, which matches `permit.ErrNotFound`.

<a id="M-client-3"></a>
### M-client-3: New and options replace the configuration builder

- v1: `config.NewConfigBuilder`
- v1: `config.PermitBuilder`
- v1: `config.NewPermitConfig`
- v1: `config.PermitConfig`
- v1: `config.IPermitConfig`
- v1: `config.PermitConfig.Build`
- v1: `permit.NewPermit`
- v1: `permit.New`
- v2: `permit.New`

Pass options to `permit.New`, which returns the client and an error, instead of building a
`config.PermitConfig` and passing it to `permit.NewPermit`:

```go
client, err := permit.New(
	permit.WithAPIKey(apiKey),
	permit.WithPDPURL("http://localhost:7766"),
)
if err != nil {
	return err
}
defer client.Close()
```

The client keeps its configuration to itself, so v2 has no getters such as `GetApiUrl`: printing
the client shows its settings, with a marker in place of the API key. The entries below map the
methods of the builder to options.

<a id="M-client-4"></a>
### M-client-4: WithApiUrl becomes WithAPIURL

- v1: `config.PermitConfig.WithApiUrl`
- v1: `config.PermitConfig.GetApiUrl`
- v1: `config.DefaultApiUrl`
- v2: `permit.WithAPIURL`

The default API URL stays `https://api.permit.io`. Without the option, `permit.New` reads
`PERMIT_API_URL`, as [M-client-1](#M-client-1) says. An empty URL is an error: leave the option
out to use the variable or the default.

<a id="M-client-5"></a>
### M-client-5: WithPdpUrl becomes WithPDPURL

- v1: `config.PermitConfig.WithPdpUrl`
- v1: `config.PermitConfig.GetPdpUrl`
- v1: `config.DefaultPdpUrl`
- v2: `permit.WithPDPURL`

The default PDP URL stays `http://localhost:7766`, a PDP container next to the application.
Without the option, `permit.New` reads `PERMIT_PDP_URL`. An empty URL is an error, as it is for
`permit.WithAPIURL`.

<a id="M-client-6"></a>
### M-client-6: The API key is an option

- v1: `config.PermitConfig.GetToken`
- v2: `permit.WithAPIKey`

Give the API key with `permit.WithAPIKey`, or set `PERMIT_API_KEY`, instead of passing it to
`config.NewConfigBuilder` or `config.NewPermitConfig`. v2 has no getter for the key, and the
client prints a marker in its place.

<a id="M-client-7"></a>
### M-client-7: WithHTTPClient takes the http.Client as it is

- v1: `config.PermitConfig.WithHTTPClient`
- v1: `config.PermitConfig.GetHTTPClient`
- v2: `permit.WithHTTPClient`

The client sends its requests through the `*http.Client` you pass, and changes none of its fields,
such as `Timeout`; `Client.Close` leaves its connections open. Without the option, each client has
an `*http.Client` of its own, with a copy of `http.DefaultTransport` that keeps up to 64 idle
connections to each host, and `Client.Close` closes them.

<a id="M-client-8"></a>
### M-client-8: WithTimeout bounds a whole call

- v1: `config.PermitConfig.WithTimeout`
- v1: `config.DefaultTimeout`
- v2: `permit.WithTimeout`

`permit.WithTimeout` sets the longest that a call may take, across all the requests it sends. v1's
default was 5 seconds for each request; v2's is 30 seconds for the whole call. The client keeps the
timeout to itself, and does not set the `Timeout` of the `*http.Client` that
`permit.WithHTTPClient` gives. A timeout of zero or less is an error.

<a id="M-client-9"></a>
### M-client-9: permit.Client is the one client

- v1: `permit.Client`
- v1: `api.PermitApiClient`
- v1: `api.NewPermitApiClient`
- v2: `permit.Client`

v2 has one client type, `permit.Client`, which `permit.New` builds. v1's `api.PermitApiClient`, the
`Api` field of v1's client, has no type of its own in v2. Call `Close` when you are done with a
client: it closes the idle connections of the client's own `*http.Client`, and a call that starts
after it returns an error that matches `permit.ErrClosed`. A client that is garbage collected
without `Close` has those connections closed too.

<a id="M-client-10"></a>
### M-client-10: The client has no interface

- v1: `permit.PermitInterface`
- v1: `api.IPermitBaseApi`
- v2: not exposed in v2

v2 declares no interface for its client. To replace it in your tests, declare an interface in
your own code with the methods you call; the SDK's types that have those methods then implement
it.

<a id="M-client-11"></a>
### M-client-11: NewClientConfig has no counterpart

- v1: `api.NewClientConfig`
- v2: not exposed in v2

`api.NewClientConfig` built the configuration of v1's generated API client. v2 has no generated
client: `permit.New` builds what the client needs from its options.

<a id="M-logging-1"></a>
### M-logging-1: WithLogger takes a *slog.Logger

- v1: `config.PermitConfig.WithLogger`
- v1: `config.PermitConfig.GetLogger`
- v2: `permit.WithLogger`

`permit.WithLogger` takes a `*slog.Logger` of the standard library's `log/slog` instead of a
`*zap.Logger`, and v2's module does not require zap. Without the option, the client logs nothing,
and so it does with a nil `*slog.Logger` or the zero `slog.Logger`; v1's builder logged to a zap
production logger on standard error when it was given no logger. The client keeps its logger to
itself, so v2 has no getter.

An application that logs with zap gives the client a `*slog.Logger` whose handler is zapslog's,
the package `go.uber.org/zap/exp/zapslog` of the module `go.uber.org/zap/exp` (v0.3.0, which needs
zap v1.26.0 or later):

```go
client, err := permit.New(
	permit.WithAPIKey(apiKey),
	permit.WithLogger(slog.New(zapslog.NewHandler(zapLogger.Core()))),
)
```

zapslog adds a stack trace to each record at Error level and above by default; its option
`zapslog.AddStacktraceAt` changes the level.

<a id="M-logging-2"></a>
### M-logging-2: The handler's level replaces WithDebug

- v1: `config.PermitConfig.WithDebug`
- v1: `config.PermitConfig.GetDebug`
- v1: `config.DefaultDebugMode`
- v2: `permit.WithLogger`, with a handler at `slog.LevelDebug`

v2 has no debug switch. The client writes its Debug lines to the logger of `permit.WithLogger`,
and the logger's handler keeps them when its level is `slog.LevelDebug` or lower. There is no dump
of a request or a reply: a Debug line names the route template, the status, the attempt, the
duration and the error of a request, never its headers or its body.

<a id="M-logging-3"></a>
### M-logging-3: The client writes one Debug line for each attempt of a request

- v1: the lines that methods wrote, such as `user created` and `User does not exist, creating it...`
- v2: a Debug line for each attempt of a request, and Warn lines about the configuration

v1's methods wrote lines of their own, such as a Debug line after a create and an Info line when
`SyncUser` created a user. In v2, the client writes one Debug line for each attempt of a request,
whatever the method, with the attributes `route`, the route template, `status`, `attempt`,
`duration` and `error`. Warn lines are warnings about the configuration, which `permit.New` writes
once, such as for an API URL that uses `http://` to a host that is not loopback. Every line has a
fixed message that starts with `permit: `, and the client replaces its credentials in each line
before the handler gets it.

<a id="M-models-1"></a>
### M-models-1: Numbers in attributes are json.Number

- v1: attribute values and other values of type `interface{}` decoded with each number as a `float64`
- v2: `models.Attributes`, whose numbers decode as `json.Number`

v1 decoded each number in an attribute map, and in every other value of type `interface{}` in a
reply, as a `float64`, which cannot hold every integer beyond 2^53: 9007199254740993 became
9007199254740992. v2 keeps each such number as a `json.Number`, the number's text with every
digit, in `models.Attributes`, in their nested objects and arrays, and in every other value of
type `any` that a reply holds. Replace a type assertion to `float64`, such as `v.(float64)`, with
one to `json.Number`, and read the number with its `Int64`, `Float64` or `String` method. A
`json.Number` encodes as its text, so a number read from a reply is sent back unchanged.

v2's `models.Attributes` is a whole attribute map. v1's `models.Attributes` was one value of such
a map, a bool, an int32 or a string, in the `map[string]models.Attributes` of `models.UserData`.
In v2, put the value itself, such as `true`, `30` or `"gold"`, in a `models.Attributes`.

<a id="M-lists-1"></a>
### M-lists-1: List methods take ListOptions and return a page

- v1: `api.DefaultPerPageLimit`
- v2: `permit.ListOptions`

v1's list methods took the page and the number of items on it as two int arguments, failed when
that number was above `api.DefaultPerPageLimit` (100) on every route, and returned a slice of the
items. v2's List methods take a `permit.ListOptions{Page, PerPage}` and return a
`*permit.Page[T]`: the items in `Data`, and `TotalCount` and `PageCount` when the reply reports
them. The client sends `PerPage` as given, without a limit of its own, as the largest page
differs from route to route; above a route's limit, the API may reject the request or give fewer
items on a page. An `All` method reads every page and returns an `iter.Seq2[T, error]` of the
items, so a loop over the pages is no longer needed.

<a id="M-scope-1"></a>
### M-scope-1: SetContext and WithContext become WithScope

- v1: `api.PermitApiClient.SetContext`
- v1: `config.PermitConfig.WithContext`
- v1: `config.NewPermitContext`
- v1: `config.PermitContext`
- v1: `config.PermitContext.SetPermitContext`
- v1: `config.PermitContextInterface`
- v2: `permit.WithScope`

Give the project and the environment that the client's calls act on to `permit.New` with
`permit.WithScope`, each by its key or its ID, instead of calling `SetContext` on the client or
passing a `config.PermitContext` to the configuration:

```go
client, err := permit.New(
	permit.WithAPIKey(apiKey),
	permit.WithScope("my-project", "production"),
)
```

`permit.New` checks the scope and returns an error that matches `permit.ErrInvalidConfig` for an
environment without a project, an empty project, and a project or an environment that is `.` or
`..`; v1's `SetContext` returned no error. An empty environment gives a client that acts on a
project only. The scope is fixed for the life of the client: build a client for each scope.
Without `permit.WithScope`, the client acts on the scope of its API key, as
[M-scope-2](#M-scope-2) says.

<a id="M-scope-2"></a>
### M-scope-2: GetContext becomes Client.Scope

- v1: `config.PermitConfig.GetContext`
- v1: `config.PermitContext.GetContext`
- v1: `config.PermitContext.GetProject`
- v1: `config.PermitContext.GetEnvironment`
- v1: `config.PermitContextFactory`
- v2: `permit.Client.Scope`

`Client.Scope(ctx)` returns the project and the environment that the client's calls act on, as a
`permit.Scope`: those that `permit.WithScope` gave, or else those of the client's API key, which
the client looks up once, as [M-scope-4](#M-scope-4) says. Call it after `permit.New` to check
the API key at once. A lookup that fails returns the `*permit.Error` of the reply, with its
status, code and body, which matches a sentinel such as `permit.ErrUnauthorized`,
`permit.ErrForbidden` or `permit.ErrRateLimited`, rather than v1's error with the code
`ContextError`.

A call that acts on an environment needs a project and an environment in the scope, and a call
that acts on a project needs a project. When the scope lacks one, as the scope of an
organization-level or a project-level API key does without `permit.WithScope`, the call returns
an error that matches `permit.ErrInvalidConfig`, names the operation and `WithScope`, and sends
no request for the operation; only the lookup of the scope that [M-scope-4](#M-scope-4)
describes may come first. A call that acts on the organization, such as one that lists its
projects, needs no scope.

<a id="M-scope-3"></a>
### M-scope-3: The level of an API key has no type

- v1: `config.APIKeyLevel`
- v1: `config.NoneAPIKeyLevel`
- v1: `config.OrganizationAPIKeyLevel`
- v1: `config.ProjectAPIKeyLevel`
- v1: `config.EnvironmentAPIKeyLevel`
- v1: `config.GetApiKeyLevel`
- v2: not exposed in v2

Tell the level of the API key from the scope that `Client.Scope` returns for a client without
`permit.WithScope`: no project for an organization-level key, a project and no environment for a
project-level key, and both for an environment-level key.

<a id="M-facts-1"></a>
### M-facts-1: WithProxyFactsViaPDP becomes WithFactsViaPDP

- v1: `config.PermitConfig.WithProxyFactsViaPDP`
- v1: `config.PermitConfig.GetProxyFactsViaPDP`
- v2: `permit.WithFactsViaPDP`

Pass `permit.WithFactsViaPDP()`, which takes no argument, to `permit.New` instead of calling
`WithProxyFactsViaPDP(true)` on the configuration, and leave it out instead of
`WithProxyFactsViaPDP(false)`, the default. v2 has no getter: the client keeps its configuration
to itself, as [M-client-3](#M-client-3) says. With the option, the calls of the facts API go to
the PDP container, as [M-facts-2](#M-facts-2) says.

<a id="M-checks-1"></a>
### M-checks-1: Check takes a context.Context and returns every failure as an error

- v1: `permit.Client.Check`
- v1: `enforcement.PermitEnforcer.Check`
- v2: `permit.Client.Check`

`Client.Check` takes a `context.Context` first, then a `permit.User`, the action as a string, a
`permit.Resource` and options. `permit.WithDecisionContext` gives the check its context, a
`map[string]any` whose values of any JSON type, such as numbers, booleans and nested objects, reach
the PDP as they are; v1's `permit.Client.Check` took no context, and the enforcer's took a
`map[string]string`:

```go
allowed, err := client.Check(ctx,
	permit.User{Key: "alice"},
	"read",
	permit.Resource{Type: "document", Key: "2026-report", Tenant: "tenant-a"},
	permit.WithDecisionContext(map[string]any{"business_hours": true}),
)
```

`Check` returns `true` when the PDP allows, `false` when it denies, and an error for every
failure:

- a user without a key or a resource without a type: an error that matches
  `permit.ErrInvalidConfig`, and no request;
- a reply with an error status, such as 400, 421 or 422, or 429 after the retries: a
  `*permit.Error` with the status and the body;
- a reply in 200-299 that is not a JSON object with one key `allow` whose value is `true` or
  `false`, such as `{}` or `{"allow": null}`: a `*permit.Error` that matches
  `permit.ErrMalformedDecision`. v1 returned `false` and no error for `{}` and `{"allow": null}`.

A decision changes nothing, so a check is sent again after a 500, 502, 503 or 504 reply or a failed
connection too, as [M-wire-5](#M-wire-5) says.

<a id="M-checks-2"></a>
### M-checks-2: BulkCheck takes a slice of CheckRequest

- v1: `permit.Client.BulkCheck`
- v1: `enforcement.PermitEnforcer.BulkCheck`
- v1: `enforcement.NewBulkCheckRequest`
- v1: `enforcement.NewBulkCheckRequestParameterized`
- v2: `permit.Client.BulkCheck`

`Client.BulkCheck(ctx, checks)` takes a `[]permit.CheckRequest`, which you build as a slice
literal, and returns the decisions in the order of the checks, with the errors of
[M-checks-1](#M-checks-1). A reply that does not hold a decision for each check is an error that
matches `permit.ErrMalformedDecision`. A call without checks sends no request and returns an empty
slice. It sends one request for all the checks, as [M-checks-14](#M-checks-14) says.

<a id="M-checks-3"></a>
### M-checks-3: FilterObjects takes and returns Resource values

- v1: `permit.Client.FilterObjects`
- v1: `enforcement.PermitEnforcer.FilterObjects`
- v1: `enforcement.ResourceI`
- v2: `permit.Client.FilterObjects`

`Client.FilterObjects(ctx, user, action, resources, opts...)` takes a `[]permit.Resource` and
returns the resources that the PDP allows, each as it was given, in their order. The context of
the checks moves from a parameter to `permit.WithDecisionContext`. v2 has no `ResourceI`
interface: each resource is a `permit.Resource`, whose key, tenant, attributes and context reach
the PDP, so that each instance of a type gets its own decision.

<a id="M-checks-4"></a>
### M-checks-4: The decisions move from the enforcer to the client

- v1: `enforcement.NewPermitEnforcerClient`
- v1: `enforcement.PermitEnforcer`
- v1: `enforcement.PermitBaseEnforcer`
- v2: `permit.Client`

Call the decision methods on the `permit.Client` that `permit.New` builds. v2 has no enforcer of
its own: the client sends the decisions to the PDP at its PDP URL, `permit.WithPDPURL`, with its
API key, its timeout and its retries.

<a id="M-checks-5"></a>
### M-checks-5: User is a struct literal

- v1: `enforcement.User`
- v1: `enforcement.UserBuilder`
- v1: `enforcement.User.Build`
- v1: `enforcement.User.WithAttributes`
- v1: `enforcement.User.WithEmail`
- v1: `enforcement.User.WithFirstName`
- v1: `enforcement.User.WithLastName`
- v2: `permit.User`

Build a user as a struct literal, such as `permit.User{Key: "alice", Email: "alice@example.com"}`,
instead of with `UserBuilder` and its methods. The fields keep their names, and a check sends them
as v1 did. A user without a key is an error that matches `permit.ErrInvalidConfig`.

<a id="M-checks-6"></a>
### M-checks-6: A user of a check has no roles

- v1: `enforcement.User.WithRoles`
- v1: `enforcement.AssignedRole`
- v2: not exposed in v2

v1 sent the roles of a user in a check, in `roles`, as objects with the fields `Role` and
`Tenant`. The PDP's user has no roles: it decides with the roles that are assigned to the user in
Permit. Assign roles with role assignments.

<a id="M-checks-7"></a>
### M-checks-7: Resource is a struct literal

- v1: `enforcement.Resource`
- v1: `enforcement.ResourceBuilder`
- v1: `enforcement.Resource.Build`
- v1: `enforcement.Resource.WithAttributes`
- v1: `enforcement.Resource.WithContext`
- v1: `enforcement.Resource.WithKey`
- v1: `enforcement.Resource.WithTenant`
- v1: `enforcement.Resource.GetAttributes`
- v1: `enforcement.Resource.GetContext`
- v1: `enforcement.Resource.GetTenant`
- v1: `enforcement.Resource.GetType`
- v1: `enforcement.DefaultTenant`
- v2: `permit.Resource`

Build a resource as a struct literal, such as
`permit.Resource{Type: "document", Key: "2026-report", Tenant: "tenant-a"}`, instead of with
`ResourceBuilder` and its methods, and read its fields instead of the getters. `Context` holds
values of any JSON type, rather than strings. A resource without a type is an error that matches
`permit.ErrInvalidConfig`. A check of a resource whose `Tenant` is empty sends the tenant
`default`, the value of v1's `DefaultTenant`, as v1's `Build` set it; `Client.FilterObjects`
returns such a resource with its `Tenant` still empty.

<a id="M-checks-8"></a>
### M-checks-8: A resource has a key and no ID

- v1: `enforcement.Resource.WithID`
- v1: `enforcement.Resource.GetID`
- v2: `permit.Resource.Key`

Give the key of a resource instance in `Key`, which the PDP reads to check that instance. v1 sent
the value of `WithID` in `id`, and from v1.3.0 in `key` too when the resource had no key; v2 has no
ID and sends no `id`.

<a id="M-checks-9"></a>
### M-checks-9: CheckRequest is a struct literal

- v1: `enforcement.CheckRequest`
- v1: `enforcement.NewCheckRequest`
- v1: `enforcement.Action`
- v2: `permit.CheckRequest`

Build a check of `Client.BulkCheck` as a struct literal of `User`, `Action`, `Resource` and
`Context`. The action is a string, and `Context` holds values of any JSON type, rather than
strings.

<a id="M-checks-10"></a>
### M-checks-10: A decision is a bool

- v1: `enforcement.CheckResponse`
- v1: `enforcement.AllowKey`
- v2: not exposed in v2

`Client.Check` returns the decision as a `bool`, and `Client.BulkCheck` as a `[]bool`: the SDK
reads `allow` of the PDP's reply itself, and returns an error when it is not `true` or `false`.

<a id="M-checks-11"></a>
### M-checks-11: MapToJson has no counterpart

- v1: `enforcement.MapToJson`
- v2: not exposed in v2

Write a map as JSON with `encoding/json`. The SDK writes the maps of a check to the PDP itself.

<a id="M-checks-12"></a>
### M-checks-12: DefaultTimeout becomes the default of WithTimeout

- v1: `enforcement.DefaultTimeout`
- v2: `permit.WithTimeout`

v1's `DefaultTimeout`, 30, set no timeout of its own. A call of v2, a decision too, takes at most
the timeout of `permit.WithTimeout`, 30 seconds by default.

<a id="M-checks-13"></a>
### M-checks-13: Decisions go to the PDP only

- v1: `config.PermitConfig.WithOpaUrl`
- v1: `config.PermitConfig.GetOpaUrl`
- v1: `config.DefaultOpaUrl`
- v2: not exposed in v2

v2 sends every decision to the PDP at the PDP URL, `permit.WithPDPURL`. v1's `WithOpaUrl` sent the
checks to OPA at its `/v1/data/` routes instead, in the shape of OPA's input. Run a PDP container,
which serves the decisions of your policy, and give its URL to `permit.WithPDPURL`.

<a id="M-checks-15"></a>
### M-checks-15: GetUserPermissions becomes UserPermissions

- v1: `permit.Client.GetUserPermissions`
- v1: `permit.Client.GetUserPermissionsWithOptions`
- v1: `enforcement.PermitEnforcer.GetUserPermissions`
- v1: `enforcement.PermitEnforcer.GetUserPermissionsWithOptions`
- v2: `permit.Client.UserPermissions`

`Client.UserPermissions` takes a `context.Context` first, then a `permit.User` and options, and
takes the place of both of v1's methods. Give the tenants that v1's `GetUserPermissions` took after
the user to `permit.WithTenants`, as [M-checks-16](#M-checks-16) says:

```go
permissions, err := client.UserPermissions(ctx,
	permit.User{Key: "alice"},
	permit.WithTenants("tenant-a"),
	permit.WithDecisionContext(map[string]any{"business_hours": true}),
)
```

It returns a `map[string]permit.ObjectPermissions` under the keys that the PDP gives:
`__tenant:<tenant>` for the roles and the permissions of the user in a tenant, and `<type>:<key>`
for those on a resource instance. It returns an error for every failure:

- a user without a key, a nil option or a filter without keys: an error that matches
  `permit.ErrInvalidConfig`, and no request;
- a reply with an error status, such as 403 or 422, or 429 after the retries: a `*permit.Error`
  with the status and the body;
- a reply in 200-299 that is not a JSON object whose every entry is a JSON object with a list
  `permissions`, such as `null` or the entries inside another object, or that holds a key twice:
  a `*permit.Error` that matches `permit.ErrMalformedDecision`. v1 returned a nil map and no
  error for `null`, and an entry with no permissions under the key `permissions` for
  `{"permissions": {...}}`.

The call changes nothing, so it is sent again after a 500, 502, 503 or 504 reply or a failed
connection too, as [M-wire-5](#M-wire-5) says.

<a id="M-checks-16"></a>
### M-checks-16: The filters of UserPermissions take their keys as arguments

- v1: `enforcement.WithTenants`
- v1: `enforcement.WithResources`
- v1: `enforcement.WithResourceTypes`
- v1: `enforcement.UserPermissionsOption`
- v2: `permit.PermissionsOption`

`permit.WithTenants`, `permit.WithResources` and `permit.WithResourceTypes` take their keys as
arguments, such as `permit.WithTenants("tenant-a", "tenant-b")`, rather than as a slice; pass a
slice with `...`, such as `permit.WithTenants(tenants...)`. Each returns a
`permit.PermissionsOption`, which only `Client.UserPermissions` takes, so a filter given to another
method does not compile. A filter without keys, such as `permit.WithTenants()`, is an error that
matches `permit.ErrInvalidConfig`: v1 left an empty list out of the body, which asked for the
permissions in every tenant, resource or resource type. Leave the option out to ask about all of
them.

<a id="M-checks-17"></a>
### M-checks-17: WithContext becomes WithDecisionContext

- v1: `enforcement.WithContext`
- v2: `permit.WithDecisionContext`

Give `Client.UserPermissions` its context with `permit.WithDecisionContext`, the option that gives
the checks their context, as a `map[string]any`. A nil or empty map sends none, as v1's did.

<a id="M-checks-18"></a>
### M-checks-18: The permissions of a user are a map of ObjectPermissions

- v1: `enforcement.UserPermissions`
- v1: `enforcement.TenantUserPermissions`
- v2: `permit.ObjectPermissions`

v2 has no named map type: `Client.UserPermissions` returns a `map[string]permit.ObjectPermissions`.
`ObjectPermissions` has the fields of v1's `TenantUserPermissions`: `Tenant`, `Resource`,
`Permissions` and `Roles`. Its `Tenant` is a `*permit.TenantDetails`, nil when the reply holds no
tenant, rather than a struct, and its `Permissions` are never nil.

<a id="M-checks-19"></a>
### M-checks-19: TenantDetails moves to package permit

- v1: `enforcement.TenantDetails`
- v2: `permit.TenantDetails`

`permit.TenantDetails` has the fields of v1's, `Key` and `Attributes`. A number in `Attributes` is a
`json.Number`, as [M-models-1](#M-models-1) says.

<a id="M-checks-20"></a>
### M-checks-20: ResourceDetails moves to package permit

- v1: `enforcement.ResourceDetails`
- v2: `permit.ResourceDetails`

`permit.ResourceDetails` has the fields of v1's, `Type`, `Key` and `Attributes`. A number in
`Attributes` is a `json.Number`, as [M-models-1](#M-models-1) says.

<a id="M-checks-21"></a>
### M-checks-21: GetUserPermissionsRequest becomes UserPermissionsRequest

- v1: `enforcement.GetUserPermissionsRequest`
- v1: `enforcement.NewGetUserPermissionsRequest`
- v2: `permit.UserPermissionsRequest`

`permit.UserPermissionsRequest` is the body that `Client.UserPermissions` sends, which the method
builds from its user and its options; no method takes it. Call `Client.UserPermissions` with
options instead of building a request.

<a id="M-checks-23"></a>
### M-checks-23: AllTenantsCheck takes a context.Context and options

- v1: `permit.Client.AllTenantsCheck`
- v1: `enforcement.PermitEnforcer.AllTenantsCheck`
- v2: `permit.Client.AllTenantsCheck`

`Client.AllTenantsCheck` takes a `context.Context` first, then a `permit.User`, the action as a
string, a `permit.Resource` and options, as `Client.Check` does, and returns the tenants in which
the PDP allows the check as a `[]permit.TenantDetails`. `permit.WithDecisionContext` gives the
check its context, a `map[string]any`; v1's `permit.Client.AllTenantsCheck` took no context, and
the enforcer's took a `map[string]string`:

```go
tenants, err := client.AllTenantsCheck(ctx,
	permit.User{Key: "alice"},
	"read",
	permit.Resource{Type: "document"},
	permit.WithDecisionContext(map[string]any{"business_hours": true}),
)
```

It returns an error for every failure, and never tenants with it:

- a user without a key or a resource without a type: an error that matches
  `permit.ErrInvalidConfig`, and no request;
- a 404 with an empty body, which is how the cloud PDP answers the route, as only the PDP
  container serves it: an error that matches `permit.ErrContainerPDPOnly`, not
  `permit.ErrNotFound`, after one request. Give `permit.WithPDPURL` the URL of a PDP container;
- a reply with another error status, such as 422, or 429 after the retries: a `*permit.Error` with
  the status and the body;
- a reply in 200-299 that is not a JSON object with a list `allowed_tenants` of decisions, each
  with an `allow` that is `true` or `false` and a `tenant` with a `key`: a `*permit.Error` that
  matches `permit.ErrMalformedDecision`. v1 read a decision without `allow`, or with `allow`
  null, as a denial, and a reply without `allowed_tenants` as no tenants.

A later version of the PDP may drop the route. `Client.UserPermissions`, which every PDP serves,
returns the permissions of a user in each tenant, under the key `__tenant:<tenant>`.

<a id="M-checks-24"></a>
### M-checks-24: AllTenantsCheckResponse has no counterpart

- v1: `enforcement.AllTenantsCheckResponse`
- v2: not exposed in v2

`Client.AllTenantsCheck` returns the tenants that the PDP allows as a `[]permit.TenantDetails`: the
SDK reads the decision of each tenant of the PDP's reply itself, and returns an error when one is
not `true` or `false`.

<a id="M-projects-1"></a>
### M-projects-1: Projects becomes Client.Projects

- v1: `api.Projects`
- v1: `api.NewProjectsApi`
- v2: `permit.ProjectsService`

Call the methods of the projects on the `Projects` field of the client, `client.Projects`, in
place of `client.Api.Projects`; `permit.New` builds it, so nothing takes the place of
`api.NewProjectsApi`. Each method takes a `context.Context` first, and needs no scope: it sends
no request for the scope of the API key and reads none of `permit.WithScope`, so a client with an
organization-level API key, whose scope has no project and no environment, calls them without
`WithScope`.

The project models keep their names and change their fields: `models.ProjectRead` has `ID`,
`OrganizationID`, `URNNamespace` and `ActivePolicyRepoID` in place of `Id`, `OrganizationId`,
`UrnNamespace` and `ActivePolicyRepoId`; its optional fields are plain strings, empty when the
reply has none, in place of pointers; `CreatedAt` and `UpdatedAt` are `models.Time`, which embeds
`time.Time`; and `Settings` is `models.Attributes`, whose numbers are `json.Number`, as
[M-models-1](#M-models-1) says.

<a id="M-projects-2"></a>
### M-projects-2: GetById and GetByKey become Get

- v1: `api.Projects.Get`
- v1: `api.Projects.GetById`
- v1: `api.Projects.GetByKey`
- v2: `permit.ProjectsService.Get`

`ProjectsService.Get(ctx, projectKey)` takes the key or the ID of the project, as all three v1
methods sent the same request: pass `id.String()` for a `uuid.UUID`. A key that holds a `/` or a
`%` is sent escaped, as one segment of the path, and an empty key, `.` or `..` is an error that
matches `permit.ErrInvalidConfig`, with no request. A project that the organization does not have
is a `*permit.Error` that matches `permit.ErrNotFound`.

<a id="M-projects-3"></a>
### M-projects-3: Projects.List returns a page, and All reads every page

- v1: `api.Projects.List`
- v2: `permit.ProjectsService.List`

`ProjectsService.List(ctx, opts)` takes a `*permit.ListOptions`, nil for the first page with the
API's default of 30 projects, in place of the page and the number of projects as two ints, and
returns a `*permit.Page[models.ProjectRead]`, as [M-lists-1](#M-lists-1) says. v1 failed for
more than 100 projects on a page before any request; v2 sends the number as given, and the API
answers above 100 with an error. `ProjectsService.All(ctx, opts)` returns an
`iter.Seq2[models.ProjectRead, error]` that reads every page, 100 projects at a time:

```go
for project, err := range client.Projects.All(ctx, nil) {
	if err != nil {
		return err
	}
	fmt.Println(project.Key)
}
```

<a id="M-projects-4"></a>
### M-projects-4: Projects.Create takes models.ProjectCreate with its initial environments

- v1: `api.Projects.Create`
- v2: `permit.ProjectsService.Create`

Build the project as a struct literal, such as
`models.ProjectCreate{Key: "my-project", Name: "My project"}`, in place of
`models.NewProjectCreate`; an optional field that is empty is left out of the request, in place
of a nil pointer. `InitialEnvironments` is new: nil lets the API create its two default
environments, Development and Production, as v1 always did, and an empty slice that is not nil
sends an empty list, for a project whose environments you create yourself. Each initial
environment is a `models.EnvironmentCreate`, whose `Description` and `CustomBranchName` are plain
strings, left out when empty, in place of v1's pointers; `JWKS` and `Settings` are new. A key
that another project of the organization has is a `*permit.Error` that matches
`permit.ErrConflict`. Create is sent again only after a 429, as [M-wire-5](#M-wire-5) says, as
the API may have created the project when a request got no complete reply.

<a id="M-projects-5"></a>
### M-projects-5: Projects.Update takes models.ProjectUpdate, which can send a null description

- v1: `api.Projects.Update`
- v2: `permit.ProjectsService.Update`

Build the change as a struct literal of `models.ProjectUpdate`, in place of
`models.NewProjectUpdate` and its setters. The request holds only the fields that are set, and
the project keeps the others. `Description` is a `models.Optional[string]`: unset leaves it out,
and the project keeps its description; `models.Null[string]()` sends null, which v1 could not
send; and `models.Some("...")` sends the value. `Name` and `ActivePolicyRepoID` are plain
strings, left out when empty, and `Settings` is sent when it is not nil, an empty map as `{}`.

<a id="M-projects-6"></a>
### M-projects-6: Projects.Delete of a project that is gone returns nil

- v1: `api.Projects.Delete`
- v2: `permit.ProjectsService.Delete`

`ProjectsService.Delete(ctx, projectKey)` returns nil when the API answers 404, as the project
is gone already, such as when a retry follows a delete that succeeded; v1 returned an error with
the code `NotFound`. A mistyped key returns nil too, so check the key before a delete that must
remove something. Any other error status is a `*permit.Error` with the status and the body.

<a id="M-environments-1"></a>
### M-environments-1: Environments becomes Client.Environments, whose methods take the project

- v1: `api.Environments`
- v1: `api.NewEnvironmentsApi`
- v2: `permit.EnvironmentsService`

Call the methods of the environments on the `Environments` field of the client,
`client.Environments`, in place of `client.Api.Environments`; `permit.New` builds it, so nothing
takes the place of `api.NewEnvironmentsApi`. Each method takes a `context.Context` first, then
the key or the ID of the project, which v1 read from the project of the client's context: pass
the project that v1's context named. The methods need no scope: they send no request for the
scope of the API key and read none of `permit.WithScope`, so one client acts on the environments
of any project that its API key may reach.

`models.EnvironmentRead`, which `Create` and `Update` return, has `ID`, `OrganizationID` and
`ProjectID` in place of `Id`, `OrganizationId` and `ProjectId`; its optional fields are plain
strings, empty when the reply has none, in place of pointers; `CreatedAt` and `UpdatedAt` are
`models.Time`, which embeds `time.Time`; and it has the fields of the reply that v1 dropped:
`Settings`, a `models.Attributes` whose numbers are `json.Number`, as [M-models-1](#M-models-1)
says, `JWKS`, a `*models.JwksConfig`, `JWKSUnpinned` and `AVPPolicyStoreID`.

<a id="M-environments-2"></a>
### M-environments-2: GetById and GetByKey become Get, which returns the email configuration

- v1: `api.Environments.Get`
- v1: `api.Environments.GetById`
- v1: `api.Environments.GetByKey`
- v2: `permit.EnvironmentsService.Get`

`EnvironmentsService.Get(ctx, projectKey, environmentKey)` takes the key or the ID of the
environment, as all three v1 methods sent the same request: pass `id.String()` for a
`uuid.UUID`. It returns a `*models.EnvironmentReadWithEmailConfig`, the model that the API
returns, in place of a `*models.EnvironmentRead`, which dropped fields of the reply: it embeds
`models.EnvironmentRead`, whose fields it promotes, so `environment.Key` reads as before, and
`environment.EnvironmentRead` is the value that code which takes a `models.EnvironmentRead`
needs; `EmailConfiguration` is the ID of the environment's email configuration. A key that holds
a `/` or a `%` is sent escaped, as one segment of the path, and an empty key, `.` or `..` is an
error that matches `permit.ErrInvalidConfig`, with no request. An environment that the project
does not have is a `*permit.Error` that matches `permit.ErrNotFound`.

<a id="M-environments-3"></a>
### M-environments-3: Environments.List returns a page, and All reads every page

- v1: `api.Environments.List`
- v2: `permit.EnvironmentsService.List`

`EnvironmentsService.List(ctx, projectKey, opts)` takes a `*permit.ListOptions`, nil for the
first page with the API's default of 30 environments, in place of the page and the number of
environments as two ints, and returns a
`*permit.Page[models.EnvironmentReadWithEmailConfig]`, as [M-lists-1](#M-lists-1) says, whose
items are the model of [M-environments-2](#M-environments-2). v1 failed for more than 100
environments on a page before any request; v2 sends the number as given, and the API answers
above 100 with an error. `EnvironmentsService.All(ctx, projectKey, opts)` returns an
`iter.Seq2[models.EnvironmentReadWithEmailConfig, error]` that reads every page, 100
environments at a time:

```go
for environment, err := range client.Environments.All(ctx, "my-project", nil) {
	if err != nil {
		return err
	}
	fmt.Println(environment.Key)
}
```

<a id="M-environments-4"></a>
### M-environments-4: Environments.Create takes the project, and sends settings and JWKS

- v1: `api.Environments.Create`
- v2: `permit.EnvironmentsService.Create`

`EnvironmentsService.Create(ctx, projectKey, environment)` creates the environment in the
project that it names, and sends the settings and the configuration of the JSON Web Key Sets of
the `models.EnvironmentCreate` when they are set, which v1 could not send; see
[M-environments-7](#M-environments-7). A key that another environment of the project has is a
`*permit.Error` that matches `permit.ErrConflict`. Create is sent again only after a 429, as
[M-wire-5](#M-wire-5) says, as the API may have created the environment when a request got no
complete reply.

<a id="M-environments-5"></a>
### M-environments-5: Environments.Update takes the project, and can send null

- v1: `api.Environments.Update`
- v2: `permit.EnvironmentsService.Update`

`EnvironmentsService.Update(ctx, projectKey, environmentKey, update)` takes a
`models.EnvironmentUpdate`, whose request holds only the fields that are set, and the
environment keeps the others; see [M-environments-8](#M-environments-8). It returns the
environment as a `*models.EnvironmentRead`. An update is sent again after a 500, 502, 503 or 504
reply or a failed connection, as sending it twice sets the same fields.

<a id="M-environments-6"></a>
### M-environments-6: Environments.Delete of an environment that is gone returns nil

- v1: `api.Environments.Delete`
- v2: `permit.EnvironmentsService.Delete`

`EnvironmentsService.Delete(ctx, projectKey, environmentKey)` returns nil when the API answers
404, as the environment is gone already, such as when a retry follows a delete that succeeded;
v1 returned an error with the code `NotFound`. A mistyped key returns nil too, so check the key
before a delete that must remove something. Any other error status is a `*permit.Error` with the
status and the body.

<a id="M-environments-7"></a>
### M-environments-7: models.EnvironmentCreate has plain strings, settings and JWKS

- v1: models.EnvironmentCreate of pkg/models, with pointer fields and a constructor
- v2: `models.EnvironmentCreate`

Build the environment as a struct literal, such as
`models.EnvironmentCreate{Key: "staging", Name: "Staging"}`, in place of
`models.NewEnvironmentCreate`. `Description` and `CustomBranchName` are plain strings, left out of
the request when empty, in place of v1's `*string`. `Settings`, a `models.Attributes`, and
`JWKS`, a `*models.JwksConfig`, are new, each left out when nil; an empty `Settings` that is not
nil sends `{}`. The `Audience` and the `Issuer` of a `models.JwksConfig` are each a
`models.StringOrList`: `models.OneString("app")` sends a JSON string, and
`models.StringList("app", "admin")` a JSON array, and one read from a reply keeps the form that
the reply gave it.

<a id="M-environments-8"></a>
### M-environments-8: models.EnvironmentUpdate has Optional fields, settings and JWKS

- v1: models.EnvironmentUpdate of pkg/models, with pointer fields and setters
- v2: `models.EnvironmentUpdate`

Build the change as a struct literal of `models.EnvironmentUpdate`, in place of
`models.NewEnvironmentUpdate` and its setters. `Name` is a plain string, left out when empty, and
the environment keeps its name. `Description` and `CustomBranchName`, which were `*string`, are
`models.Optional[string]`, and `JWKS` and `Settings` are new, a
`models.Optional[models.JwksConfig]` and a `models.Optional[models.Attributes]`: unset leaves the
field out, and the environment keeps it; `models.Null[T]()` sends null, which v1 could not send;
and `models.Some(value)` sends the value, the settings and the configuration as a whole.

<a id="M-environments-9"></a>
### M-environments-9: StatsEnvironments of the generated client becomes Environments.Stats

- v1: `openapi.EnvironmentsApiService.StatsEnvironments`
- v2: `permit.EnvironmentsService.Stats`

v1 read the statistics of an environment only through the generated client of `pkg/openapi`.
Call `client.Environments.Stats(ctx, projectKey, environmentKey)` instead, with the key or the ID
of the project and of the environment. It returns a `*models.EnvironmentStats`, which embeds
`models.EnvironmentRead`, whose fields it promotes, as
[M-environments-2](#M-environments-2) says of `Get`, with `PDPConfigs` and `Stats`. `Stats` is a
`models.EnvironmentStatistics` in place of v1's `models.Statistics`: its counts are `int`, and it
adds `MAU`, the number of monthly active users. A `models.PDPConfigRead` has `ID`,
`OrganizationID`, `ProjectID` and `EnvironmentID` in place of `Id`, `OrganizationId`,
`ProjectId` and `EnvironmentId`, and the fields of the reply that v1 dropped. Its credentials
print as a marker, also with `%+v` and in a log, and their `Value` method returns them; the
client also replaces them in its later errors and log lines.

<a id="M-environments-10"></a>
### M-environments-10: CopyEnvironment of the generated client becomes Environments.Copy

- v1: `openapi.EnvironmentsApiService.CopyEnvironment`
- v2: `permit.EnvironmentsService.Copy`

v1 copied an environment only through the generated client of `pkg/openapi`. Call
`client.Environments.Copy(ctx, projectKey, environmentKey, environmentCopy)` instead: it copies
the environment that the keys name into the target of a `models.EnvironmentCopy`, and returns the
environment that it copied into as a `*models.EnvironmentRead`. Build the copy as a struct
literal:

- `TargetEnv` is a `models.EnvironmentCopyTarget` in place of v1's `models.TargetEnv`: `Existing`,
  a plain string left out when empty, names an existing environment, and `New`, a
  `*models.EnvironmentCreate` in place of v1's `*models.New`, creates one, with its settings and
  the configuration of its JSON Web Key Sets.
- `ConflictStrategy` is a `models.EnvironmentCopyConflictStrategy`, an open enum, in place of a
  `*string`: `models.EnvironmentCopyConflictStrategyFail` or
  `models.EnvironmentCopyConflictStrategyOverwrite`, left out when empty.
- `Scope` is a `models.EnvironmentCopyScope`, left out when it is the zero value, in place of
  v1's `*models.Scope`. Each of its filters, `Resources`, `Roles`, `UserSets`, `ResourceSets` and
  the new `CustomPolicies`, is a `models.EnvironmentCopyScopeFilters` in place of v1's
  `models.Resources`, `models.UserSets`, `models.ResourceSets`, `models.CopyEnvironmentScopeRoles`
  and the `*models.Roles` of v1's `Scope`, which held resource roles and not a filter. A nil
  `Include` or `Exclude` is left out, and an empty list that is not nil sends `[]`.

`Copy` is sent again only after a 429, as [M-wire-5](#M-wire-5) says, as the API may have copied
the environment when a request got no complete reply. It needs a project-level API key, or an
organization-level one. `EnvironmentsService.CopyAsync` and `EnvironmentsService.CopyResult`,
which v1 did not have, submit a copy that runs in the background and read its task.

<a id="M-apikeys-1"></a>
### M-apikeys-1: GetEnvironmentApiKey becomes Client.APIKeys.GetEnvironmentKey

- v1: `openapi.APIKeysApiService.GetEnvironmentApiKey`
- v2: `permit.APIKeysService.GetEnvironmentKey`

Call the methods of the API keys on the `APIKeys` field of the client, `client.APIKeys`, in place
of the generated `APIKeysApiService` of `pkg/openapi` and its request builders: each method takes
a `context.Context` first, then its parameters, and sends its request without `Execute`.
`APIKeysService.GetEnvironmentKey(ctx, projectKey, environmentKey)` takes the key or the ID of the
project and of the environment, each sent escaped as one segment of the path, and needs no scope:
it sends no request for the scope of the API key and reads none of `permit.WithScope`.

The calls return a `*models.APIKeyRead`, whose fields change: `ID`, `OrganizationID`, `ProjectID`
and `EnvironmentID` in place of `Id`, `OrganizationId`, `ProjectId` and `EnvironmentId`; its
optional fields are plain values, empty when the reply has none, in place of pointers;
`CreatedAt` and `LastUsedAt` are `models.Time`, which embeds `time.Time`; `Name`, `Env`, a
`*models.EnvironmentRead`, and `Project` are new; and `CreatedByMember` is not in the v2 model. The credential of the key, a `*string` in
v1, is a type that prints a marker in its place with `fmt`, `log/slog` and `encoding/json`: call
its `Value` method to read it. The client keeps each credential that its calls return out of its
errors and its log lines, as it does its own API key.

<a id="M-apikeys-2"></a>
### M-apikeys-2: ListApiKeys returns a page, and All reads every page

- v1: `openapi.APIKeysApiService.ListApiKeys`
- v2: `permit.APIKeysService.List`

`APIKeysService.List(ctx, opts)` takes a `*permit.APIKeysListOptions`, nil for the first page
with the API's default of 30 keys, in place of the `Page` and `PerPage` setters of the request,
and returns a `*permit.Page[models.APIKeyRead]` with the counts of the reply, in place of a
`*models.PaginatedResultAPIKeyRead`, as [M-lists-1](#M-lists-1) says. The options embed
`permit.ListOptions`, and filter by the level of the key, `ObjectType`, and by its project,
`Project`, neither of which v1 could send. `APIKeysService.All(ctx, opts)` returns an
`iter.Seq2[models.APIKeyRead, error]` that reads every page, 100 keys at a time.

<a id="M-apikeys-3"></a>
### M-apikeys-3: GetApiKey becomes Get

- v1: `openapi.APIKeysApiService.GetApiKey`
- v2: `permit.APIKeysService.Get`

`APIKeysService.Get(ctx, apiKeyID)` takes the ID of the key, sent escaped as one segment of the
path; an empty ID, `.` or `..` is an error that matches `permit.ErrInvalidConfig`, with no
request, and so is the ID `scope`, which names the route of the scope of the API key, not a key.
A key that the organization does not have is a `*permit.Error` that matches
`permit.ErrNotFound`.

<a id="M-apikeys-4"></a>
### M-apikeys-4: CreateApiKey takes models.APIKeyCreate as a value

- v1: `openapi.APIKeysApiService.CreateApiKey`
- v2: `permit.APIKeysService.Create`

Build the key as a struct literal, such as `models.APIKeyCreate{OrganizationID: orgID}`, in place
of `models.NewAPIKeyCreate` and its setters; an optional field that is empty is left out of the
request, in place of a nil pointer, and the API gives it its default. `ObjectType`, `AccessLevel`
and `OwnerType` are open enums with a constant for each value, such as `models.MemberAccessObjEnv`,
`models.MemberAccessLevelRead` and `models.APIKeyOwnerTypeMember`, and `models.APIKeyOwnerType`
has the value `nats_pdp_config`, which v1 did not know. Create is sent again only after a 429, as
[M-wire-5](#M-wire-5) says, as the API may have created the key when a request got no complete
reply.

<a id="M-apikeys-5"></a>
### M-apikeys-5: DeleteApiKey of a key that is gone returns nil

- v1: `openapi.APIKeysApiService.DeleteApiKey`
- v2: `permit.APIKeysService.Delete`

`APIKeysService.Delete(ctx, apiKeyID)` returns nil when the API answers 404, as the key is gone
already, such as when a retry follows a delete that succeeded; v1 returned an error. A mistyped
ID returns nil too, so check the ID before a delete that must remove something. Any other error
status is a `*permit.Error` with the status and the body.

<a id="M-resources-1"></a>
### M-resources-1: Resources becomes Client.Resources, which acts on the environment of the scope

- v1: `api.Resources`
- v1: `api.NewResourcesApi`
- v2: `permit.ResourcesService`

Call the methods of the resources on the `Resources` field of the client, `client.Resources`, in
place of `client.Api.Resources`; `permit.New` builds it, so nothing takes the place of
`api.NewResourcesApi`. Each method takes a `context.Context` first, and acts on the environment
of the client's scope, as v1's methods acted on the environment of the client's context:
`permit.WithScope`, or else the scope of the API key, which the client looks up once, as
[M-scope-2](#M-scope-2) says. A scope without an environment, such as that of a project-level API
key without `permit.WithScope`, is an error that matches `permit.ErrInvalidConfig`, with no
request for the call.

The resource models keep their names and change their fields: `models.ResourceRead` has `ID`,
`OrganizationID`, `ProjectID`, `EnvironmentID` and `URN` in place of `Id`, `OrganizationId`,
`ProjectId`, `EnvironmentId` and `Urn`; its optional fields are plain values, empty or nil when
the reply has none, in place of pointers; `CreatedAt` and `UpdatedAt` are `models.Time`, which
embeds `time.Time`; and it has the fields of the reply that v1 dropped: `TypeAttributes`, a
`models.Attributes` whose numbers are `json.Number`, as [M-models-1](#M-models-1) says, `Roles`,
`Relations` and `ActionGroups`. Build `models.ResourceCreate`, `models.ResourceReplace` and
`models.ResourceUpdate` as struct literals, in place of their constructors and setters: an
optional field that is its zero value is left out of the request, in place of a nil pointer, and
`TypeAttributes`, `Roles` and `Relations`, which v1 could not send, are new.

<a id="M-resources-2"></a>
### M-resources-2: GetById and GetByKey become Get

- v1: `api.Resources.Get`
- v1: `api.Resources.GetById`
- v1: `api.Resources.GetByKey`
- v2: `permit.ResourcesService.Get`

`ResourcesService.Get(ctx, resourceKey)` takes the key or the ID of the resource, as all three v1
methods sent the same request: pass `id.String()` for a `uuid.UUID`. A key that holds a `/` or a
`%` is sent escaped, as one segment of the path, and an empty key, `.` or `..` is an error that
matches `permit.ErrInvalidConfig`, with no request for the resource. A resource that the
environment does not have is a `*permit.Error` that matches `permit.ErrNotFound`.

<a id="M-resources-3"></a>
### M-resources-3: Resources.List returns a page, and All reads every page

- v1: `api.Resources.List`
- v2: `permit.ResourcesService.List`

`ResourcesService.List(ctx, opts)` takes a `*permit.ResourcesListOptions`, nil for the first page
with the API's default of 30 resources, in place of the page and the number of resources as two
ints, and returns a `*permit.Page[models.ResourceRead]`, as [M-lists-1](#M-lists-1) says. The
options embed `permit.ListOptions`. v1 failed for more than 100 resources on a page before any
request; v2 sends the number as given, and the API answers above 100 with an error.
`ResourcesService.All(ctx, opts)` returns an `iter.Seq2[models.ResourceRead, error]` that reads
every page, 100 resources at a time:

```go
for resource, err := range client.Resources.All(ctx, nil) {
	if err != nil {
		return err
	}
	fmt.Println(resource.Key)
}
```

<a id="M-resources-4"></a>
### M-resources-4: Resources.Search becomes the Search option of List

- v1: `api.Resources.Search`
- v2: `permit.ResourcesListOptions`

v2 has no `Search` method: give the text to `Search` of `permit.ResourcesListOptions`, which
`ResourcesService.List` and `ResourcesService.All` send in the query parameter `search`, as v1's
`Search` did, such as
`client.Resources.List(ctx, &permit.ResourcesListOptions{Search: "document"})` in place of
`client.Api.Resources.Search(ctx, 1, 30, "document")`. An empty `Search` lists every resource.

<a id="M-resources-5"></a>
### M-resources-5: Resources.Create takes models.ResourceCreate with roles and relations

- v1: `api.Resources.Create`
- v2: `permit.ResourcesService.Create`

`ResourcesService.Create(ctx, resource)` sends the `models.ResourceCreate` as
[M-resources-1](#M-resources-1) says: its key, its name and its actions always, an empty map of
actions as `{}`, and each other field that is set, the roles, the relations and the type
attributes of the resource among them. A key that another resource of the environment has is a
`*permit.Error` that matches `permit.ErrConflict`. Create is sent again only after a 429, as
[M-wire-5](#M-wire-5) says, as the API may have created the resource when a request got no
complete reply.

<a id="M-resources-6"></a>
### M-resources-6: ReplaceResource becomes Resources.Replace

- v1: `openapi.ResourcesApiService.ReplaceResource`
- v2: `permit.ResourcesService.Replace`

`ResourcesService.Replace(ctx, resourceKey, resource)` sends the whole definition of the
resource, a `models.ResourceReplace`, with PUT, in place of the generated `ReplaceResource` of
`pkg/openapi` and its request builder. The API's documentation says that the call replaces the
definition of the resource, or creates the resource when the environment has none with the key,
and that the actions that the definition leaves out or renames lose their role and permission
assignments. A replace is sent again after a 500, 502, 503 or 504 reply or a failed connection,
as sending the same definition twice leaves the resource with it.

<a id="M-resources-7"></a>
### M-resources-7: Resources.Update takes models.ResourceUpdate, which can send a null description

- v1: `api.Resources.Update`
- v2: `permit.ResourcesService.Update`

Build the change as a struct literal of `models.ResourceUpdate`, in place of
`models.NewResourceUpdate` and its setters. The request holds only the fields that are set.
`Description` is a `models.Optional[string]`: unset leaves it out, `models.Null[string]()` sends
null, which v1 could not send, and `models.Some("...")` sends the value. Each map, such as
`Actions`, is sent as it is given, as a whole: nil leaves it out, an empty map that is not nil
sends `{}`, and a map with some entries sends those entries only; the SDK never sends null for a
map. An update is sent again after a 500, 502, 503 or 504 reply or a failed connection, as
sending it twice sets the same fields.

<a id="M-resources-8"></a>
### M-resources-8: Resources.Delete of a resource that is gone returns nil

- v1: `api.Resources.Delete`
- v2: `permit.ResourcesService.Delete`

`ResourcesService.Delete(ctx, resourceKey)` returns nil when the API answers 404, as the resource
is gone already, such as when a retry follows a delete that succeeded; v1 returned an error with
the code `NotFound`. A mistyped key returns nil too, so check the key before a delete that must
remove something. Any other error status is a `*permit.Error` with the status and the body.

<a id="M-tenants-1"></a>
### M-tenants-1: Tenants becomes Client.Tenants

- v1: `api.Tenants`
- v1: `api.NewTenantsApi`
- v2: `permit.TenantsService`

Call the methods of the tenants on the `Tenants` field of the client, `client.Tenants`, in place
of `client.Api.Tenants`; `permit.New` builds it, so nothing takes the place of
`api.NewTenantsApi`. Each method takes a `context.Context` first. The calls act on the
environment of the client's scope, as v1's acted on the environment of its context: the project
and the environment of `permit.WithScope`, or else those of the API key, as
[M-scope-2](#M-scope-2) says. With `permit.WithFactsViaPDP()`, they go to the PDP container, as
[M-facts-2](#M-facts-2) says. An environment-level API key may call each of them.

`models.TenantRead` has `ID`, `OrganizationID`, `ProjectID` and `EnvironmentID` in place of `Id`,
`OrganizationId`, `ProjectId` and `EnvironmentId`; `Description` is a plain string, empty when
the reply has none, in place of a pointer; `CreatedAt`, `UpdatedAt` and `LastActionAt` are
`models.Time`, which embeds `time.Time`; and `Attributes` is a `models.Attributes`, whose numbers
are `json.Number`, as [M-models-1](#M-models-1) says. `Attributes` is nil when the reply holds
null, as it does after an update that set them to null, and an empty map that is not nil when it
holds `{}`.

<a id="M-tenants-2"></a>
### M-tenants-2: GetById and GetByKey become Get

- v1: `api.Tenants.Get`
- v1: `api.Tenants.GetById`
- v1: `api.Tenants.GetByKey`
- v2: `permit.TenantsService.Get`

`TenantsService.Get(ctx, tenantKey)` takes the key or the ID of the tenant, as all three v1
methods sent the same request: pass `id.String()` for a `uuid.UUID`. A key that holds a `/` or a
`%` is sent escaped, as one segment of the path, and an empty key, `.` or `..` is an error that
matches `permit.ErrInvalidConfig`, with no request for the tenant. A tenant that the environment
does not have is a `*permit.Error` that matches `permit.ErrNotFound`.

<a id="M-tenants-3"></a>
### M-tenants-3: List and ListByAttributes become List with TenantsListOptions

- v1: `api.Tenants.List`
- v1: `api.Tenants.ListByAttributes`
- v2: `permit.TenantsService.List`

`TenantsService.List(ctx, opts)` takes a `*permit.TenantsListOptions`, nil for the first page of
every tenant with the API's default of 30 tenants, in place of the page and the number of tenants
as two ints, and returns a `*permit.Page[models.TenantRead]`, as [M-lists-1](#M-lists-1) says. v1
failed for more than 100 tenants on a page before any request; v2 sends the number as given, and
the API answers above 100 with an error. A request that fails is an error, also a 500; before
v1.3.0, `List` and `ListByAttributes` returned no error for it, so it looked like an empty list.

The `Attributes` of the options take the place of `ListByAttributes`: each
`permit.AttributeFilter`, which `permit.AttributeEquals(name, value)` builds, keeps the tenants
whose attribute `name` equals the value, and is sent as the query parameter `attr_<name>`, as v1
sent each entry of its map. The value is a string, a bool, an int, an int64 or a float64, in
place of any value that v1 printed with `%v`; a float64 is sent as JSON writes it, such as
`1000000` where v1 sent `1e+06`, and NaN or an infinity is an error that matches
`permit.ErrInvalidConfig`, as are a filter without a name and a filter whose name an earlier
filter has, which v1's map could not hold.
`TenantsService.All(ctx, opts)` returns an `iter.Seq2[models.TenantRead, error]` that reads every
page, 100 tenants at a time:

```go
opts := &permit.TenantsListOptions{
	Attributes: []permit.AttributeFilter{permit.AttributeEquals("tier", "gold")},
}
for tenant, err := range client.Tenants.All(ctx, opts) {
	if err != nil {
		return err
	}
	fmt.Println(tenant.Key)
}
```

<a id="M-tenants-4"></a>
### M-tenants-4: Tenants.Create takes models.TenantCreate

- v1: `api.Tenants.Create`
- v2: `permit.TenantsService.Create`

Build the tenant as a struct literal, such as
`models.TenantCreate{Key: "tenant-a", Name: "Tenant A"}`, in place of `models.NewTenantCreate`.
`Description` is a plain string, left out of the request when empty, in place of a `*string`,
and `Attributes`, a `models.Attributes`, is left out when nil; an empty map that is not nil sends
`{}`. A key that another tenant of the environment has is a `*permit.Error` that matches
`permit.ErrConflict`. Create is sent again only after a 429, as [M-wire-5](#M-wire-5) says, as
the API may have created the tenant when a request got no complete reply.

<a id="M-tenants-5"></a>
### M-tenants-5: Tenants.Update takes models.TenantUpdate, which can send null

- v1: `api.Tenants.Update`
- v2: `permit.TenantsService.Update`

Build the change as a struct literal of `models.TenantUpdate`, in place of
`models.NewTenantUpdate` and its setters. The request holds only the fields that are set, and the
tenant keeps the others. `Name` is a plain string, left out when empty. `Description` and
`Attributes` are a `models.Optional[string]` and a `models.Optional[models.Attributes]`: unset
leaves the field out; `models.Null[T]()` sends null, which v1 could not send, and removes the
description, or stores the attributes as null; and `models.Some(value)` sends the value. The
attributes that an update sends replace the tenant's attributes as a whole: `models.Some` of an
empty map sends `{}`, which removes every attribute.

<a id="M-tenants-6"></a>
### M-tenants-6: Tenants.Delete of a tenant that is gone returns nil

- v1: `api.Tenants.Delete`
- v2: `permit.TenantsService.Delete`

`TenantsService.Delete(ctx, tenantKey)` returns nil when the API answers 404, as the tenant is
gone already, such as when a retry follows a delete that succeeded; v1 returned an error with the
code `NotFound`. A mistyped key returns nil too, so check the key before a delete that must remove
something. Any other error status is a `*permit.Error` with the status and the body.

<a id="M-tenants-7"></a>
### M-tenants-7: Tenants.WaitForSync takes a time.Duration and waits on creates only

- v1: `api.Tenants.WaitForSync`
- v2: `permit.TenantsService.WaitForSync`

`TenantsService.WaitForSync(timeout, opts...)` takes the timeout as a `time.Duration`, in place
of a `*time.Duration` whose nil meant the timeout of the configuration, and options, in place of
`api.WaitForSyncOptions`, as [M-facts-3](#M-facts-3) says. It returns a copy of the service whose
calls carry the wait; the service that it is called on keeps none.

The PDP container waits for sync on the create of a tenant only. A call through the copy of any
other method, such as `Update` or `Delete`, is an error that matches `permit.ErrInvalidConfig`,
and no request is sent; v1 sent the headers of the wait with every call through its copy. A
client without `permit.WithFactsViaPDP()` gets the same error for every call through the copy,
as only the PDP container waits; v1 logged a warning and made the calls without the wait.

<a id="M-facts-3"></a>
### M-facts-3: WaitForSyncOptions becomes SyncOption

- v1: `api.WaitForSyncOptions`
- v2: `permit.SyncOption`

Pass `permit.WithSyncPolicy(policy)` to the `WaitForSync` method of a service, such as
`client.Tenants.WaitForSync(10*time.Second, permit.WithSyncPolicy(permit.SyncPolicyFail))`, in
place of `api.WaitForSyncOptions{Policy: policy}`. Without the option, the call sends no
`X-Timeout-Policy`, and the PDP container decides; v1 sent the policy of the configuration then,
when it had one. A policy other than `permit.SyncPolicyFail` and `permit.SyncPolicyIgnore`, and the
zero `permit.SyncOption`, in any place among the options, are errors that match
`permit.ErrInvalidConfig` when a call is made through the copy, and no request is sent.

<a id="M-facts-4"></a>
### M-facts-4: FactsSyncTimeoutPolicy becomes SyncPolicy

- v1: `config.FactsSyncTimeoutPolicy`
- v1: `config.FactsSyncTimeoutPolicyFail`
- v1: `config.FactsSyncTimeoutPolicyIgnore`
- v2: `permit.SyncPolicy`

Use `permit.SyncPolicyFail` and `permit.SyncPolicyIgnore`, of the type `permit.SyncPolicy`, in
place of `config.FactsSyncTimeoutPolicyFail` and `config.FactsSyncTimeoutPolicyIgnore`; they send
the same values, `fail` and `ignore`.

<a id="M-facts-5"></a>
### M-facts-5: The configuration has no wait for sync

- v1: `config.DefaultFactsSyncTimeout`
- v1: `config.PermitConfig.WithFactsSyncTimeout`
- v1: `config.PermitConfig.GetFactsSyncTimeout`
- v1: `config.PermitConfig.GetFactsSyncTimeoutPolicy`
- v2: not exposed in v2

v1 kept a timeout and a policy of the wait for sync in its configuration, a timeout of 10 seconds
in one that `config.NewConfigBuilder` built, which `WaitForSync` used when it got no timeout or no
policy. v2 has no such setting: give the timeout, and the policy when you want one, to each
`WaitForSync` call, such as `client.Tenants.WaitForSync(10*time.Second)` for the timeout of v1's
builder.

<a id="M-facts-6"></a>
### M-facts-6: The base type of the facts APIs has no counterpart

- v1: `api.IPermitBaseFactsApi`
- v1: `api.PermitBaseFactsApi`
- v1: `api.PermitBaseFactsApi.WaitForSync`
- v1: `api.NewPermitBaseFactsApi`
- v1: `api.NewFactsClientConfig`
- v2: not exposed in v2

v1's facts APIs, such as `api.Tenants`, embedded `api.PermitBaseFactsApi`, whose `WaitForSync`
returned the base type. In v2, each service of the facts API has a `WaitForSync` method of its
own that returns a copy of that service, such as `permit.TenantsService.WaitForSync`.
`api.NewFactsClientConfig` built a configuration of the generated client for the PDP URL, which
v1 did not use; pass `permit.WithFactsViaPDP()` to `permit.New` to send the facts to the PDP
container.

<a id="M-resource-actions-1"></a>
### M-resource-actions-1: ResourceActions becomes Client.ResourceActions, which acts on the environment of the scope

- v1: `api.ResourceActions`
- v1: `api.NewResourceActionsApi`
- v2: `permit.ResourceActionsService`

Call the methods of the actions of a resource on the `ResourceActions` field of the client,
`client.ResourceActions`, in place of `client.Api.ResourceActions`; `permit.New` builds it, so
nothing takes the place of `api.NewResourceActionsApi`. Each method takes a `context.Context`
first and the key or the ID of the resource next, and acts on the environment of the client's
scope, as v1's methods acted on the environment of the client's context: `permit.WithScope`, or
else the scope of the API key, which the client looks up once, as [M-scope-2](#M-scope-2) says.
A scope without an environment, such as that of a project-level API key without
`permit.WithScope`, is an error that matches `permit.ErrInvalidConfig`, with no request for the
call.

v1's `List`, `Get`, `Create`, `Update` and `Delete` returned the error of the generated client of
`pkg/openapi` as it was, not an `errors.PermitError`. Every v2 method returns a
`*permit.Error`, or an error that wraps one, as [M-errors-1](#M-errors-1) says, so
`errors.Is(err, permit.ErrNotFound)` and the other sentinels work for each of them.

The action models keep their names and change their fields: `models.ResourceActionRead` has
`ID`, `OrganizationID`, `ProjectID`, `EnvironmentID` and `ResourceID` in place of `Id`,
`OrganizationId`, `ProjectId`, `EnvironmentId` and `ResourceId`; its description is a plain
string, empty when the reply has none, in place of a pointer; `CreatedAt` and `UpdatedAt` are
`models.Time`, which embeds `time.Time`; its `Attributes` are a `models.Attributes` whose
numbers are `json.Number`, as [M-models-1](#M-models-1) says; and it has the fields of the reply
that v1 dropped: `V1CompatPath`, `V1CompatIsBuiltIn` and `V1CompatName`. Build
`models.ResourceActionCreate` and `models.ResourceActionUpdate` as struct literals, in place of
their constructors and setters.

<a id="M-resource-actions-2"></a>
### M-resource-actions-2: GetById and GetByKey become Get

- v1: `api.ResourceActions.Get`
- v1: `api.ResourceActions.GetById`
- v1: `api.ResourceActions.GetByKey`
- v2: `permit.ResourceActionsService.Get`

`ResourceActionsService.Get(ctx, resourceKey, actionKey)` takes the key or the ID of the resource
and of the action, as all three v1 methods sent the same request: pass `id.String()` for a
`uuid.UUID`. A key that holds a `/` or a `%` is sent escaped, as one segment of the path, and an
empty key, `.` or `..` is an error that matches `permit.ErrInvalidConfig`, with no request for
the action. An action that the resource does not have is a `*permit.Error` that matches
`permit.ErrNotFound`.

<a id="M-resource-actions-3"></a>
### M-resource-actions-3: ResourceActions.List returns a page, and All reads every page

- v1: `api.ResourceActions.List`
- v2: `permit.ResourceActionsService.List`

`ResourceActionsService.List(ctx, resourceKey, opts)` takes a
`*permit.ResourceActionsListOptions`, nil for the first page with the API's default of 30
actions, in place of the page and the number of actions as two ints, and returns a
`*permit.Page[models.ResourceActionRead]`, as [M-lists-1](#M-lists-1) says. The options embed
`permit.ListOptions`. v1 failed for more than 100 actions on a page before any request; v2 sends
the number as given, and the API answers above 100 with an error.
`ResourceActionsService.All(ctx, resourceKey, opts)` returns an
`iter.Seq2[models.ResourceActionRead, error]` that reads every page, 100 actions at a time.

<a id="M-resource-actions-4"></a>
### M-resource-actions-4: ListByAttributes becomes the Attributes option of List

- v1: `api.ResourceActions.ListByAttributes`
- v2: `permit.ResourceActionsListOptions`

v2 has no `ListByAttributes` method: give the filters to `Attributes` of
`permit.ResourceActionsListOptions`, which `ResourceActionsService.List` and
`ResourceActionsService.All` send in the query parameters `attr_<name>`, as v1 did. In place of
a `map[string]interface{}`, `Attributes` is a `[]permit.AttributeFilter`, each of which
`permit.AttributeEquals(name, value)` builds, as [M-tenants-3](#M-tenants-3) says, such as
`client.ResourceActions.List(ctx, "document", &permit.ResourceActionsListOptions{Attributes: []permit.AttributeFilter{permit.AttributeEquals("risk", "low")}})`
in place of
`client.Api.ResourceActions.ListByAttributes(ctx, "document", 1, 30, map[string]interface{}{"risk": "low"})`.

The value is a string, a bool, an int, an int64 or a float64. A string is sent as it is, a bool
as `true` or `false`, and an integer as its decimal digits. A float64 is sent as `encoding/json`
writes it, such as `1000000` or `1e-7`, where v1 wrote fmt's `%v` form, such as `1e+06` or
`1e-07`; NaN and the infinities are errors that match `permit.ErrInvalidConfig`, with no
request. A value of any other type, such as a map or a slice, which v1 sent as text, does not
compile. Two filters of the same name, which a map could not hold, and a filter without a name
are errors that match `permit.ErrInvalidConfig`, with no request.

<a id="M-resource-actions-5"></a>
### M-resource-actions-5: ResourceActions.Create takes models.ResourceActionCreate as a struct literal

- v1: `api.ResourceActions.Create`
- v2: `permit.ResourceActionsService.Create`

`ResourceActionsService.Create(ctx, resourceKey, action)` sends the `models.ResourceActionCreate`
as [M-resource-actions-1](#M-resource-actions-1) says: its key and its name always, and its
description and its attributes when they are set, an empty map of attributes as `{}`. A key that
another action of the resource has is a `*permit.Error` that matches `permit.ErrConflict`. Create
is sent again only after a 429, as [M-wire-5](#M-wire-5) says, as the API may have created the
action when a request got no complete reply.

<a id="M-resource-actions-6"></a>
### M-resource-actions-6: ResourceActions.Update takes models.ResourceActionUpdate, which can send null

- v1: `api.ResourceActions.Update`
- v2: `permit.ResourceActionsService.Update`

Build the change as a struct literal of `models.ResourceActionUpdate`, in place of
`models.NewResourceActionUpdate` and its setters. The request holds only the fields that are set.
`Description` is a `models.Optional[string]` and `Attributes` a
`models.Optional[models.Attributes]`: unset leaves the field out, `models.Null` sends null, which
clears it and which v1 could not send, and `models.Some` sends the value, the attributes as a
whole. An update is sent again after a 500, 502, 503 or 504 reply or a failed connection, as
sending it twice sets the same fields.

<a id="M-resource-actions-7"></a>
### M-resource-actions-7: ResourceActions.Delete of an action that is gone returns nil

- v1: `api.ResourceActions.Delete`
- v2: `permit.ResourceActionsService.Delete`

`ResourceActionsService.Delete(ctx, resourceKey, actionKey)` returns nil when the API answers
404, as the action is gone already, such as when a retry follows a delete that succeeded; v1
returned the error of the generated client. A mistyped key of the action or of the resource
returns nil too, so check the keys before a delete that must remove something. Any other error
status is a `*permit.Error` with the status and the body.

<a id="M-action-groups-1"></a>
### M-action-groups-1: ResourceActionGroups becomes Client.ResourceActionGroups, with Update

- v1: `api.ResourceActionGroups`
- v1: `api.NewResourceActionGroupsApi`
- v2: `permit.ResourceActionGroupsService`

Call the methods of the action groups on the `ResourceActionGroups` field of the client,
`client.ResourceActionGroups`, in place of `client.Api.ResourceActionGroups`; `permit.New` builds
it, so nothing takes the place of `api.NewResourceActionGroupsApi`. Each method takes a
`context.Context` first, then the key or the ID of the resource, and acts on the environment of
the client's scope, as v1's methods acted on the environment of the client's context:
`permit.WithScope`, or else the scope of the API key, which the client looks up once, as
[M-scope-2](#M-scope-2) says. A scope without an environment, such as that of a project-level API
key without `permit.WithScope`, is an error that matches `permit.ErrInvalidConfig`, with no
request for the call.

`models.ResourceActionGroupRead` has `ID`, `OrganizationID`, `ProjectID`, `EnvironmentID` and
`ResourceID` in place of `Id`, `OrganizationId`, `ProjectId`, `EnvironmentId` and `ResourceId`;
`Description` is a plain string, empty when the reply has none, in place of a pointer;
`Attributes` is a `models.Attributes`, whose numbers are `json.Number`, as
[M-models-1](#M-models-1) says; and `CreatedAt` and `UpdatedAt` are `models.Time`, which embeds
`time.Time`.

`ResourceActionGroupsService.Update(ctx, resourceKey, actionGroupKey, update)` is new: v1 had no
update of an action group. It takes a `models.ResourceActionGroupUpdate`, whose request holds
only the fields that are set. Its `Actions` are a `models.Optional[[]string]`: unset leaves them
out, `models.Some` sends the keys, and `models.Null[[]string]()` sends null. Set the actions in
each update, to the keys of the actions that the group keeps: an update that leaves them out may
remove them all.

<a id="M-action-groups-2"></a>
### M-action-groups-2: GetById and GetByKey become Get

- v1: `api.ResourceActionGroups.Get`
- v1: `api.ResourceActionGroups.GetById`
- v1: `api.ResourceActionGroups.GetByKey`
- v2: `permit.ResourceActionGroupsService.Get`

`ResourceActionGroupsService.Get(ctx, resourceKey, actionGroupKey)` takes the key or the ID of the
resource and of the action group, as all three v1 methods sent the same request: pass
`id.String()` for a `uuid.UUID`. A key that holds a `/` or a `%` is sent escaped, as one segment
of the path, and an empty key, `.` or `..` is an error that matches `permit.ErrInvalidConfig`,
with no request. An action group that the resource does not have is a `*permit.Error` that
matches `permit.ErrNotFound`.

<a id="M-action-groups-3"></a>
### M-action-groups-3: ResourceActionGroups.List returns a page, and All reads every page

- v1: `api.ResourceActionGroups.List`
- v2: `permit.ResourceActionGroupsService.List`

`ResourceActionGroupsService.List(ctx, resourceKey, opts)` takes a `*permit.ListOptions`, nil for
the first page with the API's default of 30 action groups, in place of the page and the number of
action groups as two ints, and returns a `*permit.Page[models.ResourceActionGroupRead]`, as
[M-lists-1](#M-lists-1) says. v1 failed for more than 100 action groups on a page before any
request; v2 sends the number as given, and the API answers above 100 with an error.
`ResourceActionGroupsService.All(ctx, resourceKey, opts)` returns an
`iter.Seq2[models.ResourceActionGroupRead, error]` that reads every page, 100 action groups at a
time:

```go
for group, err := range client.ResourceActionGroups.All(ctx, "document", nil) {
	if err != nil {
		return err
	}
	fmt.Println(group.Key, group.Actions)
}
```

<a id="M-action-groups-4"></a>
### M-action-groups-4: ResourceActionGroups.ListByAttributes becomes List, without a filter

- v1: `api.ResourceActionGroups.ListByAttributes`
- v2: `permit.ResourceActionGroupsService.List`
- Bug fix: could never work before

v1's `ListByAttributes` never sent its filter: it listed every action group of the resource,
whatever the map it was given held. v2 has no attribute filter for the action groups: call
`ResourceActionGroupsService.List` or `ResourceActionGroupsService.All`, and keep the action
groups whose `Attributes` match. Code that called `ListByAttributes` got every action group, so
check what it did with those that did not match its filter.

<a id="M-action-groups-5"></a>
### M-action-groups-5: ResourceActionGroups.Create takes models.ResourceActionGroupCreate

- v1: `api.ResourceActionGroups.Create`
- v2: `permit.ResourceActionGroupsService.Create`

Build the action group as a struct literal, such as
`models.ResourceActionGroupCreate{Key: "editing", Name: "Editing", Actions: []string{"read", "write"}}`,
in place of `models.NewResourceActionGroupCreate`. `Description` is a plain string, left out of
the request when empty, in place of a `*string`; `Attributes`, a `models.Attributes`, and
`Actions` are left out when nil, and an empty map or slice that is not nil sends `{}` or `[]`. A
key that another action group of the resource has is a `*permit.Error` that matches
`permit.ErrConflict`. Create is sent again only after a 429, as [M-wire-5](#M-wire-5) says, as
the API may have created the action group when a request got no complete reply.

<a id="M-action-groups-6"></a>
### M-action-groups-6: ResourceActionGroups.Delete deletes the action group, and returns nil when it is gone

- v1: `api.ResourceActionGroups.Delete`
- v2: `permit.ResourceActionGroupsService.Delete`

Up to v1.2.8, v1's `Delete` sent
`DELETE /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/actions/{action_id}`, the route
that deletes an action of the resource: it deleted the action whose key was the group's key, or
failed with 404 and left the action group in place. v1.3.0 sends the route of the action group,
as v2 does: `DELETE /v2/schema/{proj_id}/{env_id}/resources/{resource_id}/action_groups/{action_group_id}`.
If code called `Delete` with v1.2.8 or earlier, check that its resources still have their
actions.

`ResourceActionGroupsService.Delete(ctx, resourceKey, actionGroupKey)` returns nil when the API
answers 404, as the action group is gone already, such as when a retry follows a delete that
succeeded; v1 returned an error. A mistyped key returns nil too, so check the key before a delete
that must remove something. Any other error status is a `*permit.Error` with the status and the
body.

<a id="M-resource-attributes-1"></a>
### M-resource-attributes-1: ResourceAttributes becomes Client.ResourceAttributes, which acts on the environment of the scope

- v1: `api.ResourceAttributes`
- v1: `api.NewResourceAttributesApi`
- v2: `permit.ResourceAttributesService`

Call the methods of the attributes of a resource on the `ResourceAttributes` field of the client,
`client.ResourceAttributes`, in place of `client.Api.ResourceAttributes`; `permit.New` builds it,
so nothing takes the place of `api.NewResourceAttributesApi`. Each method takes a
`context.Context` first and the key or the ID of the resource next, and acts on the environment
of the client's scope, as v1's methods acted on the environment of the client's context:
`permit.WithScope`, or else the scope of the API key, which the client looks up once, as
[M-scope-2](#M-scope-2) says. A scope without an environment, such as that of a project-level API
key without `permit.WithScope`, is an error that matches `permit.ErrInvalidConfig`, with no
request for the call.

v1's `List`, `Get`, `Create`, `Update` and `Delete` returned the error of the generated client of
`pkg/openapi` as it was, not an `errors.PermitError`. Every v2 method returns a
`*permit.Error`, or an error that wraps one, as [M-errors-1](#M-errors-1) says, so
`errors.Is(err, permit.ErrNotFound)` and the other sentinels work for each of them.

The attribute models keep their names and change their fields: `models.ResourceAttributeRead` has
`ID`, `ResourceID`, `OrganizationID`, `ProjectID` and `EnvironmentID` in place of `Id`,
`ResourceId`, `OrganizationId`, `ProjectId` and `EnvironmentId`; its description is a plain
string, empty when the reply has none, in place of a pointer; `CreatedAt` and `UpdatedAt` are
`models.Time`, which embeds `time.Time`; and it has `BuiltIn`, the `built_in` of the reply, which
v1 dropped. `Type` is a `models.AttributeType`, an open enum with a constant for each type, such
as `models.AttributeTypeString` in place of v1's `models.STRING`, and `models.AttributeTypeObject`
and `models.AttributeTypeObjectArray`, which v1 did not have. Build
`models.ResourceAttributeCreate` and `models.ResourceAttributeUpdate` as struct literals, in place
of their constructors and setters.

<a id="M-resource-attributes-2"></a>
### M-resource-attributes-2: GetById and GetByKey become Get, which reads every attribute type

- v1: `api.ResourceAttributes.Get`
- v1: `api.ResourceAttributes.GetById`
- v1: `api.ResourceAttributes.GetByKey`
- v2: `permit.ResourceAttributesService.Get`

`ResourceAttributesService.Get(ctx, resourceKey, attributeKey)` takes the key or the ID of the
resource and of the attribute, as all three v1 methods sent the same request: pass `id.String()`
for a `uuid.UUID`. A key that holds a `/` or a `%` is sent escaped, as one segment of the path,
and an empty key, `.` or `..` is an error that matches `permit.ErrInvalidConfig`, with no request
for the attribute. An attribute that the resource does not have is a `*permit.Error` that matches
`permit.ErrNotFound`.

v1's three methods failed for an attribute of the type `object` or `object_array`, with the error
`object is not a valid AttributeType`, as v1's `models.AttributeType` rejected every type that it
did not list. v2 reads an attribute of every type: `models.AttributeType` keeps a type that this
version of the SDK does not know, such as one that the API adds later, as it is, and its `Known`
method reports false for it.

<a id="M-resource-attributes-3"></a>
### M-resource-attributes-3: ResourceAttributes.List returns a page of every attribute type, and All reads every page

- v1: `api.ResourceAttributes.List`
- v2: `permit.ResourceAttributesService.List`

`ResourceAttributesService.List(ctx, resourceKey, opts)` takes a `*permit.ListOptions`, nil for
the first page with the API's default of 30 attributes, in place of the page and the number of
attributes as two ints, and returns a `*permit.Page[models.ResourceAttributeRead]`, as
[M-lists-1](#M-lists-1) says. v1 failed for more than 100 attributes on a page before any
request; v2 sends the number as given, and the API answers above 100 with an error.
`ResourceAttributesService.All(ctx, resourceKey, opts)` returns an
`iter.Seq2[models.ResourceAttributeRead, error]` that reads every page, 100 attributes at a time.

v1's `List` failed for the whole page when one attribute of the resource had the type `object` or
`object_array`, as [M-resource-attributes-2](#M-resource-attributes-2) says of `Get`. v2 reads
every attribute of the page, of any type, also one that the API adds later.

<a id="M-resource-attributes-4"></a>
### M-resource-attributes-4: ResourceAttributes.Create takes models.ResourceAttributeCreate as a struct literal

- v1: `api.ResourceAttributes.Create`
- v2: `permit.ResourceAttributesService.Create`

`ResourceAttributesService.Create(ctx, resourceKey, attribute)` sends the
`models.ResourceAttributeCreate` as [M-resource-attributes-1](#M-resource-attributes-1) says: its
key and its type always, and its description when it is not empty, in place of
`models.NewResourceAttributeCreate(key, type)` and `SetDescription`. A key that another attribute
of the resource has is a `*permit.Error` that matches `permit.ErrConflict`. Create is sent again
only after a 429, as [M-wire-5](#M-wire-5) says, as the API may have created the attribute when
a request got no complete reply.

v1's `Create` returned an error for an attribute of the type `object` or `object_array`, such as
`object is not a valid AttributeType`, although the API had created the attribute: v1 could not
read the reply. v2 reads the reply, and returns the attribute of any type.

<a id="M-resource-attributes-5"></a>
### M-resource-attributes-5: ResourceAttributes.Update takes models.ResourceAttributeUpdate, which can send null

- v1: `api.ResourceAttributes.Update`
- v2: `permit.ResourceAttributesService.Update`

Build the change as a struct literal of `models.ResourceAttributeUpdate`, in place of
`models.NewResourceAttributeUpdate` and its setters. The request holds only the fields that are
set: `Type` when it is not empty, and `Description`, a `models.Optional[string]`, when it is set:
`models.Null` sends null, which clears the description and which v1 could not send, and
`models.Some` sends the value. An update is sent again after a 500, 502, 503 or 504 reply or a
failed connection, as sending it twice sets the same fields.

v1's `Update` returned an error for an attribute of the type `object` or `object_array`, such as
`object is not a valid AttributeType`, although the API had changed the attribute: v1 could not
read the reply. v2 reads the reply, and returns the attribute of any type.

<a id="M-resource-attributes-6"></a>
### M-resource-attributes-6: ResourceAttributes.Delete of an attribute that is gone returns nil

- v1: `api.ResourceAttributes.Delete`
- v2: `permit.ResourceAttributesService.Delete`

`ResourceAttributesService.Delete(ctx, resourceKey, attributeKey)` returns nil when the API
answers 404, as the attribute is gone already, such as when a retry follows a delete that
succeeded; v1 returned the error of the generated client. A mistyped key of the attribute or of
the resource returns nil too, so check the keys before a delete that must remove something. Any
other error status is a `*permit.Error` with the status and the body.

<a id="M-resource-relations-1"></a>
### M-resource-relations-1: ResourceRelations becomes Client.ResourceRelations, which acts on the environment of the scope

- v1: `api.ResourceRelations`
- v1: `api.NewResourceRelationsApi`
- v2: `permit.ResourceRelationsService`

Call the methods of the relations of a resource on the `ResourceRelations` field of the client,
`client.ResourceRelations`, in place of `client.Api.ResourceRelations`; `permit.New` builds it,
so nothing takes the place of `api.NewResourceRelationsApi`. Each method takes a
`context.Context` first, then the key or the ID of the resource that has the relation, the
object, and acts on the environment of the client's scope, as v1's methods acted on the
environment of the client's context: `permit.WithScope`, or else the scope of the API key, which
the client looks up once, as [M-scope-2](#M-scope-2) says. A scope without an environment, such
as that of a project-level API key without `permit.WithScope`, is an error that matches
`permit.ErrInvalidConfig`, with no request for the call.

v1's methods returned an `errors.PermitError` for an error status of the API, and the error of
the generated client of `pkg/openapi` as it was for a request without a reply. Every v2 method
returns a `*permit.Error`, or an error that wraps one, as [M-errors-1](#M-errors-1) says, so
`errors.Is(err, permit.ErrNotFound)` and the other sentinels work for each of them.

The relation models keep their names and change their fields: `models.RelationRead` has `ID`,
`OrganizationID`, `ProjectID`, `EnvironmentID`, `ObjectResourceID` and `SubjectResourceID` in
place of `Id`, `OrganizationId`, `ProjectId`, `EnvironmentId`, `ObjectResourceId` and
`SubjectResourceId`; its description is a plain string, empty when the reply has none, in place
of a pointer; and `CreatedAt` and `UpdatedAt` are `models.Time`, which embeds `time.Time`. Build
`models.RelationCreate` as a struct literal, in place of `models.NewRelationCreate` and its
setters.

<a id="M-resource-relations-2"></a>
### M-resource-relations-2: ResourceRelations.List takes the resource first, and returns a page with its counts

- v1: `api.ResourceRelations.List`
- v2: `permit.ResourceRelationsService.List`

v1's `List(ctx, page, perPage, resourceId)` took the page and the number of relations before the
resource, and returned a `*[]models.RelationRead`, the items alone, without the `total_count` and
the `page_count` of the API's reply. `ResourceRelationsService.List(ctx, resourceKey, opts)`
takes the resource first and a `*permit.ListOptions` next, nil for the first page with the API's
default of 30 relations, and returns a `*permit.Page[models.RelationRead]` with the counts of the
reply in `TotalCount` and `PageCount`, as [M-lists-1](#M-lists-1) says. v2 reads 0 as the first
page or the API's default number, returns an error that matches `permit.ErrInvalidConfig` for a
number below 0, with no request for the list, and sends any number above 0 as given; the API
answers above 100 with an error. `ResourceRelationsService.All(ctx, resourceKey, opts)` returns
an `iter.Seq2[models.RelationRead, error]` that reads every page, 100 relations at a time, up to
the last page that the replies count:

```go
for relation, err := range client.ResourceRelations.All(ctx, "document", nil) {
	if err != nil {
		return err
	}
	fmt.Println(relation.Key, relation.SubjectResource)
}
```

<a id="M-resource-relations-3"></a>
### M-resource-relations-3: ResourceRelations.Get reads a relation by its key or its ID

- v1: `api.ResourceRelations.Get`
- v2: `permit.ResourceRelationsService.Get`

`ResourceRelationsService.Get(ctx, resourceKey, relationKey)` takes the key or the ID of the
resource and of the relation, as v1's `Get` did: pass `id.String()` for a `uuid.UUID`. A key that
holds a `/` or a `%` is sent escaped, as one segment of the path, and an empty key, `.` or `..`
is an error that matches `permit.ErrInvalidConfig`, with no request for the relation. A relation
that the resource does not have is a `*permit.Error` that matches `permit.ErrNotFound`.

<a id="M-resource-relations-4"></a>
### M-resource-relations-4: ResourceRelations.Create takes models.RelationCreate as a struct literal

- v1: `api.ResourceRelations.Create`
- v2: `permit.ResourceRelationsService.Create`

`ResourceRelationsService.Create(ctx, resourceKey, relation)` sends the `models.RelationCreate`
as [M-resource-relations-1](#M-resource-relations-1) says: its key, its name and its
`SubjectResource`, the key or the ID of the related resource, always, and its description, a
plain string in place of a `*string`, when it is not empty. A key that another relation of the
resource has is a `*permit.Error` that matches `permit.ErrConflict`. Create is sent again only
after a 429, as [M-wire-5](#M-wire-5) says, as the API may have created the relation when a
request got no complete reply.

<a id="M-resource-relations-5"></a>
### M-resource-relations-5: ResourceRelations.Delete of a relation that is gone returns nil

- v1: `api.ResourceRelations.Delete`
- v2: `permit.ResourceRelationsService.Delete`

`ResourceRelationsService.Delete(ctx, resourceKey, relationKey)` returns nil when the API answers
404, as the relation is gone already, such as when a retry follows a delete that succeeded; v1
returned an error with the code `NotFound`. A mistyped key of the relation or of the resource
returns nil too, so check the keys before a delete that must remove something. Any other error
status is a `*permit.Error` with the status and the body.

<a id="M-roles-1"></a>
### M-roles-1: Roles becomes Client.Roles, which acts on the environment of the scope

- v1: `api.Roles`
- v1: `api.NewRolesApi`
- v2: `permit.RolesService`

Call the methods of the roles on the `Roles` field of the client, `client.Roles`, in place of
`client.Api.Roles`; `permit.New` builds it, so nothing takes the place of `api.NewRolesApi`. Each
method takes a `context.Context` first, and acts on the environment of the client's scope, as
v1's methods acted on the environment of the client's context: `permit.WithScope`, or else the
scope of the API key, which the client looks up once, as [M-scope-2](#M-scope-2) says. A scope
without an environment, such as that of a project-level API key without `permit.WithScope`, is an
error that matches `permit.ErrInvalidConfig`, with no request for the call. Every method returns a
`*permit.Error`, or an error that wraps one, as [M-errors-1](#M-errors-1) says. v1's
`BulkAssignRole` and `BulkUnAssignRole` assign roles to users; they move with the role
assignments.

The role models keep their names and change their fields: `models.RoleRead` has `ID`,
`OrganizationID`, `ProjectID` and `EnvironmentID` in place of `Id`, `OrganizationId`, `ProjectId`
and `EnvironmentId`; its description is a plain string, empty when the reply has none, in place
of a pointer; `CreatedAt` and `UpdatedAt` are `models.Time`, which embeds `time.Time`; and it has
`GrantedTo` and the v1compat fields of the reply, which v1 dropped. `Attributes` is a
`models.Attributes`, a map of any JSON value whose numbers are `json.Number`, as
[M-models-1](#M-models-1) says, in place of a `map[string]string`: v1 failed to read a reply
with a role whose attribute was a number, a boolean or an object, also after the API had created
or changed the role. Build `models.RoleCreate` and `models.RoleUpdate` as struct literals, in
place of their constructors and setters.

<a id="M-roles-2"></a>
### M-roles-2: RoleRead of package api becomes models.RoleRead

- v1: `api.RoleRead`
- v2: `models.RoleRead`

v1's `api.RoleRead` embedded `models.RoleRead`, and no method returned it. Use `models.RoleRead`,
which every method of `permit.RolesService` that returns a role returns, as
[M-roles-1](#M-roles-1) says.

<a id="M-roles-3"></a>
### M-roles-3: GetById and GetByKey become Get

- v1: `api.Roles.Get`
- v1: `api.Roles.GetById`
- v1: `api.Roles.GetByKey`
- v2: `permit.RolesService.Get`

`RolesService.Get(ctx, roleKey)` takes the key or the ID of the role, as all three v1 methods
sent the same request: pass `id.String()` for a `uuid.UUID`. A key that holds a `/` or a `%` is
sent escaped, as one segment of the path, and an empty key, `.` or `..` is an error that matches
`permit.ErrInvalidConfig`, with no request for the role. A role that the environment does not
have is a `*permit.Error` that matches `permit.ErrNotFound`.

<a id="M-roles-4"></a>
### M-roles-4: List and ListByAttributes become List with RolesListOptions

- v1: `api.Roles.List`
- v1: `api.Roles.ListByAttributes`
- v2: `permit.RolesService.List`

`RolesService.List(ctx, opts)` takes a `*permit.RolesListOptions`, nil for the first page of
every role with the API's default of 30 roles, in place of the page and the number of roles as
two ints, and returns a `*permit.Page[models.RoleRead]`, as [M-lists-1](#M-lists-1) says. v1
failed for more than 100 roles on a page before any request; v2 sends the number as given, and
the API answers above 100 with an error. `RolesService.All(ctx, opts)` returns an
`iter.Seq2[models.RoleRead, error]` that reads every page, 100 roles at a time.

The `Attributes` of the options take the place of `ListByAttributes`: each
`permit.AttributeFilter`, which `permit.AttributeEquals(name, value)` builds, keeps the roles
whose attribute `name` equals the value, and is sent as the query parameter `attr_<name>`, as v1
sent each entry of its map, with the rules of [M-tenants-3](#M-tenants-3), such as
`client.Roles.List(ctx, &permit.RolesListOptions{Attributes: []permit.AttributeFilter{permit.AttributeEquals("tier", "gold")}})`
in place of
`client.Api.Roles.ListByAttributes(ctx, 1, 30, map[string]interface{}{"tier": "gold"})`.

<a id="M-roles-5"></a>
### M-roles-5: Roles.Create takes models.RoleCreate as a struct literal

- v1: `api.Roles.Create`
- v2: `permit.RolesService.Create`

`RolesService.Create(ctx, role)` sends the `models.RoleCreate`: its key and its name always, and
its other fields when they are set, an empty slice of permissions or of the roles that it
extends as `[]` and an empty map of attributes as `{}`. Its attributes take values of any JSON
type, as [M-roles-1](#M-roles-1) says, and `GrantedTo`, which v1 did not have, says how users get
the role through their roles on related resources. A key that another role of the environment has
is a `*permit.Error` that matches `permit.ErrConflict`. Create is sent again only after a 429, as
[M-wire-5](#M-wire-5) says, as the API may have created the role when a request got no complete
reply.

<a id="M-roles-6"></a>
### M-roles-6: Roles.Update takes models.RoleUpdate, which can send null

- v1: `api.Roles.Update`
- v2: `permit.RolesService.Update`

Build the change as a struct literal of `models.RoleUpdate`, in place of `models.NewRoleUpdate`
and its setters. The request holds only the fields that are set. `Permissions` and `Extends` are
plain slices, each sent as a whole in place of the role's list: nil leaves the list out, and the
role keeps it; an empty slice that is not nil sends `[]`, which clears it; and a list replaces
it. `Description` is a `models.Optional[string]`, `Attributes` a
`models.Optional[models.Attributes]` and `GrantedTo` a
`models.Optional[models.DerivedRoleBlockEdit]`: unset leaves the field out, `models.Null` sends
null, which clears it, and `models.Some` sends the value. v1 could not send null for the
description or the attributes, and had no `GrantedTo`. An update is sent again after a 500, 502,
503 or 504 reply or a failed connection, as sending it twice sets the same fields.

<a id="M-roles-7"></a>
### M-roles-7: Roles.Delete of a role that is gone returns nil

- v1: `api.Roles.Delete`
- v2: `permit.RolesService.Delete`

`RolesService.Delete(ctx, roleKey)` returns nil when the API answers 404, as the role is gone
already, such as when a retry follows a delete that succeeded; v1 returned an error. A mistyped
key returns nil too, so check the key before a delete that must remove something. Any other error
status is a `*permit.Error` with the status and the body.

<a id="M-roles-8"></a>
### M-roles-8: AssignPermissions returns the role

- v1: `api.Roles.AssignPermissions`
- v2: `permit.RolesService.AssignPermissions`

`RolesService.AssignPermissions(ctx, roleKey, permissions)` takes the same arguments as v1's, and
returns the role as the API stores it after the grant, a `*models.RoleRead`, with the error, in
place of the error alone. A nil slice sends `[]`, as an empty one does. A 404 is a
`*permit.Error` that matches `permit.ErrNotFound`, such as when the environment has no such role.
It is sent again after a 500, 502, 503 or 504 reply or a failed connection, as v1's documentation
says that the API skips a permission that the role has already.

<a id="M-roles-9"></a>
### M-roles-9: RemovePermissions returns the role

- v1: `api.Roles.RemovePermissions`
- v2: `permit.RolesService.RemovePermissions`

`RolesService.RemovePermissions(ctx, roleKey, permissions)` takes the same arguments as v1's, and
returns the role as the API stores it after the removal, a `*models.RoleRead`, with the error, in
place of the error alone. A nil slice sends `[]`, as an empty one does. A 404 is a
`*permit.Error` that matches `permit.ErrNotFound`, such as when the environment has no such role,
and not a removal that happened already, as the body names what to remove. It is sent again after
a 500, 502, 503 or 504 reply or a failed connection, as v1's documentation says that the API skips
a permission that the role does not have.

<a id="M-roles-10"></a>
### M-roles-10: AddParentRole and RemoveParentRole become Update with Extends

- v1: `api.Roles.AddParentRole`
- v1: `api.Roles.RemoveParentRole`
- v1: `openapi.RolesApiService.AddParentRole`
- v1: `openapi.RolesApiService.RemoveParentRole`
- v2: replaced by `permit.RolesService.Update`
- Bug fix: could never work before

v1's parent methods sent `PUT` and `DELETE` to `/v2/schema/{proj_id}/{env_id}/roles/{role_id}/parents/{parent_role_id}`,
a route that the API's documentation does not list. A role's parents are the roles that it
extends: read the role with `RolesService.Get`, and send its `Extends` with the parent added or
removed in `models.RoleUpdate.Extends` with `RolesService.Update`, as the update replaces the
whole list:

```go
role, err := client.Roles.Get(ctx, "editor")
if err != nil {
	return err
}
extends := append(slices.Clone(role.Extends), "viewer")
_, err = client.Roles.Update(ctx, "editor", models.RoleUpdate{Extends: extends})
```

<a id="M-resource-roles-1"></a>
### M-resource-roles-1: ResourceRoles becomes Client.ResourceRoles, which acts on the environment of the scope

- v1: `api.ResourceRoles`
- v1: `api.NewResourceRolesApi`
- v2: `permit.ResourceRolesService`

Call the methods of the roles of a resource on the `ResourceRoles` field of the client,
`client.ResourceRoles`, in place of `client.Api.ResourceRoles`; `permit.New` builds it, so
nothing takes the place of `api.NewResourceRolesApi`. Each method takes a `context.Context`
first, then the key or the ID of the resource, then that of the role when it acts on one, and
acts on the environment of the client's scope, as v1's methods acted on the environment of the
client's context: `permit.WithScope`, or else the scope of the API key, which the client looks up
once, as [M-scope-2](#M-scope-2) says. A scope without an environment, such as that of a
project-level API key without `permit.WithScope`, is an error that matches
`permit.ErrInvalidConfig`, with no request for the call. Every method returns a `*permit.Error`,
or an error that wraps one, as [M-errors-1](#M-errors-1) says.

The models keep their names and change their fields: `models.ResourceRoleRead` has `ID`,
`OrganizationID`, `ProjectID`, `EnvironmentID` and `ResourceID` in place of `Id`,
`OrganizationId`, `ProjectId`, `EnvironmentId` and `ResourceId`; its description is a plain
string, empty when the reply has none, in place of a pointer; `CreatedAt` and `UpdatedAt` are
`models.Time`, which embeds `time.Time`; and it has `Attributes`, a `models.Attributes` of any
JSON value whose numbers are `json.Number`, as [M-models-1](#M-models-1) says, and `Resource`,
the key of the resource, which v1's model dropped. Build `models.ResourceRoleCreate` and
`models.ResourceRoleUpdate` as struct literals, in place of their constructors and setters; each
has `Attributes` and `GrantedTo`, which v1's did not have.

<a id="M-resource-roles-2"></a>
### M-resource-roles-2: ResourceRoles.List takes the resource first, and returns a page

- v1: `api.ResourceRoles.List`
- v2: `permit.ResourceRolesService.List`

v1's `List(ctx, page, perPage, resourceId)` took the page and the number of roles before the
resource, and returned a `*[]models.ResourceRoleRead`. `ResourceRolesService.List(ctx,
resourceKey, opts)` takes the resource first and a `*permit.ListOptions` next, nil for the first
page with the API's default of 30 roles, and returns a `*permit.Page[models.ResourceRoleRead]`,
as [M-lists-1](#M-lists-1) says. v1 failed for more than 100 roles on a page before any request;
v2 sends the number as given, and the API answers above 100 with an error.
`ResourceRolesService.All(ctx, resourceKey, opts)` returns an
`iter.Seq2[models.ResourceRoleRead, error]` that reads every page, 100 roles at a time.

<a id="M-resource-roles-3"></a>
### M-resource-roles-3: ResourceRoles.Get reads a role by its key or its ID

- v1: `api.ResourceRoles.Get`
- v2: `permit.ResourceRolesService.Get`

`ResourceRolesService.Get(ctx, resourceKey, roleKey)` takes the key or the ID of the resource and
of the role, as v1's `Get` did: pass `id.String()` for a `uuid.UUID`. A key that holds a `/` or a
`%` is sent escaped, as one segment of the path, and an empty key, `.` or `..` is an error that
matches `permit.ErrInvalidConfig`, with no request for the role. A role that the resource does
not have is a `*permit.Error` that matches `permit.ErrNotFound`.

<a id="M-resource-roles-4"></a>
### M-resource-roles-4: ResourceRoles.Create takes models.ResourceRoleCreate as a struct literal

- v1: `api.ResourceRoles.Create`
- v2: `permit.ResourceRolesService.Create`

`ResourceRolesService.Create(ctx, resourceKey, role)` sends the `models.ResourceRoleCreate`: its
key and its name always, and its other fields when they are set, an empty slice of permissions or
of the roles that it extends as `[]` and an empty map of attributes as `{}`. A key that another
role of the resource has is a `*permit.Error` that matches `permit.ErrConflict`. Create is sent
again only after a 429, as [M-wire-5](#M-wire-5) says, as the API may have created the role when
a request got no complete reply.

<a id="M-resource-roles-5"></a>
### M-resource-roles-5: ResourceRoles.Update takes models.ResourceRoleUpdate, which can send null

- v1: `api.ResourceRoles.Update`
- v2: `permit.ResourceRolesService.Update`

The request holds only the fields of the `models.ResourceRoleUpdate` that are set. `Permissions`
and `Extends` are plain slices, each sent as a whole in place of the role's list: nil leaves the
list out of the request; an empty slice that is not nil sends `[]`; and a list replaces it.
`Description` is a `models.Optional[string]`, `Attributes` a `models.Optional[models.Attributes]`
and `GrantedTo` a `models.Optional[models.DerivedRoleBlockEdit]`: unset leaves the field out,
`models.Null` sends null, and `models.Some` sends the value. An update is sent again after a 500,
502, 503 or 504 reply or a failed connection, as sending it twice sets the same fields.

<a id="M-resource-roles-6"></a>
### M-resource-roles-6: ResourceRoles.Delete of a role that is gone returns nil

- v1: `api.ResourceRoles.Delete`
- v2: `permit.ResourceRolesService.Delete`

`ResourceRolesService.Delete(ctx, resourceKey, roleKey)` returns nil when the API answers 404, as
the role is gone already, such as when a retry follows a delete that succeeded; v1 returned an
error. A mistyped key of the role or of the resource returns nil too, so check the keys before a
delete that must remove something. Any other error status is a `*permit.Error` with the status
and the body.

<a id="M-resource-roles-7"></a>
### M-resource-roles-7: ResourceRoles.AssignPermissions takes the permissions as a slice

- v1: `api.ResourceRoles.AssignPermissions`
- v2: `permit.ResourceRolesService.AssignPermissions`

`ResourceRolesService.AssignPermissions(ctx, resourceKey, roleKey, permissions)` takes the
permissions as a `[]string`, in place of a `models.AddRolePermissions`, and returns the role as
the API stores it after the grant. A nil slice sends `[]`, as an empty one does. A 404 is a
`*permit.Error` that matches `permit.ErrNotFound`. It is sent again after a 500, 502, 503 or 504
reply or a failed connection, as v1's documentation says that the API skips a permission that
the role has already.

<a id="M-resource-roles-8"></a>
### M-resource-roles-8: ResourceRoles.RemovePermissions takes the permissions as a slice

- v1: `api.ResourceRoles.RemovePermissions`
- v2: `permit.ResourceRolesService.RemovePermissions`

`ResourceRolesService.RemovePermissions(ctx, resourceKey, roleKey, permissions)` takes the
permissions as a `[]string`, in place of a `models.RemoveRolePermissions`, and returns the role as
the API stores it after the removal. A nil slice sends `[]`, as an empty one does. A 404 is a
`*permit.Error` that matches `permit.ErrNotFound`, and not a removal that happened already, as
the body names what to remove. It is sent again after a 500, 502, 503 or 504 reply or a failed
connection, as v1's documentation says that the API skips a permission that the role does not
have.

<a id="M-resource-roles-9"></a>
### M-resource-roles-9: AddParent and RemoveParent become Update with Extends

- v1: `api.ResourceRoles.AddParent`
- v1: `api.ResourceRoles.RemoveParent`
- v1: `openapi.ResourceRolesApiService.AddParentResourceRole`
- v1: `openapi.ResourceRolesApiService.RemoveParentResourceRole`
- v2: replaced by `permit.ResourceRolesService.Update`
- Bug fix: could never work before

v1's parent methods sent `PUT` and `DELETE` to
`/v2/schema/{proj_id}/{env_id}/resources/{resource_id}/roles/{role_id}/parents/{parent_role_id}`,
a route that the API's documentation does not list. A role's parents are the roles of its
resource that it extends: read the role with `ResourceRolesService.Get`, and send its `Extends`
with the parent added or removed in `models.ResourceRoleUpdate.Extends` with
`ResourceRolesService.Update`, as the update replaces the whole list:

```go
role, err := client.ResourceRoles.Get(ctx, "document", "editor")
if err != nil {
	return err
}
extends := append(slices.Clone(role.Extends), "viewer")
_, err = client.ResourceRoles.Update(ctx, "document", "editor",
	models.ResourceRoleUpdate{Extends: extends})
```

<a id="M-user-attributes-1"></a>
### M-user-attributes-1: ListUserAttributes of the generated client becomes Client.UserAttributes.List

- v1: `openapi.UserAttributesApiService.ListUserAttributes`
- v2: `permit.UserAttributesService.List`

v1 had no method of its own for the attributes of the users, the schema of what a user may hold:
it sent their routes only through the generated `UserAttributesApiService` of `pkg/openapi`. Call
the methods of the attributes of the users on the `UserAttributes` field of the client,
`client.UserAttributes`, in place of that service and its request builders: each method takes a
`context.Context` first, then its parameters, and sends its request without `Execute`. Each acts
on the environment of the client's scope, in place of the project and the environment that each
generated call took: `permit.WithScope`, or else the scope of the API key, which the client looks
up once, as [M-scope-2](#M-scope-2) says. A scope without an environment, such as that of a
project-level API key without `permit.WithScope`, is an error that matches
`permit.ErrInvalidConfig`, with no request for the call.

The `ResourceId` setters of the request builders are not carried over: no v2 method sends
`resource_id`, so each acts on the attributes of the users alone. Call
`permit.ResourceAttributesService` for the attributes of another resource. The methods take and
return the models of the generated calls, `models.ResourceAttributeCreate`,
`models.ResourceAttributeUpdate` and `models.ResourceAttributeRead`, with the fields that
[M-resource-attributes-1](#M-resource-attributes-1) describes. Every method returns a
`*permit.Error`, or an error that wraps one, as [M-errors-1](#M-errors-1) says, in place of the
generated client's error and its `*http.Response`.

`UserAttributesService.List(ctx, opts)` takes a `*permit.ListOptions`, nil for the first page
with the API's default of 30 attributes, in place of the `Page` and `PerPage` setters, and
returns a `*permit.Page[models.ResourceAttributeRead]` in place of a
`[]models.ResourceAttributeRead`, as [M-lists-1](#M-lists-1) says.
`UserAttributesService.All(ctx, opts)` returns an `iter.Seq2[models.ResourceAttributeRead, error]`
that reads every page, 100 attributes at a time. v1's `ListUserAttributes` failed for the whole
page when one attribute had the type `object` or `object_array`, as
[M-resource-attributes-2](#M-resource-attributes-2) says of the attributes of a resource; v2
reads every attribute of the page, of any type, also one that the API adds later.

<a id="M-user-attributes-2"></a>
### M-user-attributes-2: GetUserAttribute becomes UserAttributes.Get, which reads every attribute type

- v1: `openapi.UserAttributesApiService.GetUserAttribute`
- v2: `permit.UserAttributesService.Get`

`UserAttributesService.Get(ctx, attributeKey)` takes the key or the ID of the attribute, in place
of the project, the environment and the attribute of `GetUserAttribute`: pass `id.String()` for a
`uuid.UUID`. A key that holds a `/` or a `%` is sent escaped, as one segment of the path, and an
empty key, `.` or `..` is an error that matches `permit.ErrInvalidConfig`, with no request for
the attribute. An attribute that the users do not have is a `*permit.Error` that matches
`permit.ErrNotFound`. v2 reads an attribute of every type, where v1 failed for the type `object`
or `object_array`.

<a id="M-user-attributes-3"></a>
### M-user-attributes-3: CreateUserAttribute becomes UserAttributes.Create, which takes the attribute as a value

- v1: `openapi.UserAttributesApiService.CreateUserAttribute`
- v2: `permit.UserAttributesService.Create`

`UserAttributesService.Create(ctx, attribute)` takes the `models.ResourceAttributeCreate` as a
struct literal, in place of the `ResourceAttributeCreate` setter of the request builder, and
sends its key and its type always, and its description when it is not empty. A key that another
attribute of the users has is a `*permit.Error` that matches `permit.ErrConflict`. Create is sent
again only after a 429, as [M-wire-5](#M-wire-5) says, as the API may have created the attribute
when a request got no complete reply. v1's `CreateUserAttribute` returned an error for an
attribute of the type `object` or `object_array` although the API had created it, as it could not
read the reply; v2 reads the reply, and returns the attribute of any type.

<a id="M-user-attributes-4"></a>
### M-user-attributes-4: UpdateUserAttribute becomes UserAttributes.Update, which can send null

- v1: `openapi.UserAttributesApiService.UpdateUserAttribute`
- v2: `permit.UserAttributesService.Update`

`UserAttributesService.Update(ctx, attributeKey, update)` takes the key or the ID of the attribute
and a `models.ResourceAttributeUpdate` as a struct literal, in place of the
`ResourceAttributeUpdate` setter of the request builder. The request holds only the fields that
are set, as [M-resource-attributes-5](#M-resource-attributes-5) says: `models.Null` sends null
for the description, which v1 could not send. An update is sent again after a 500, 502, 503 or
504 reply or a failed connection, as sending it twice sets the same fields. v2 reads the reply
for an attribute of any type, where v1 failed for the type `object` or `object_array`.

<a id="M-user-attributes-5"></a>
### M-user-attributes-5: DeleteUserAttribute of an attribute that is gone returns nil

- v1: `openapi.UserAttributesApiService.DeleteUserAttribute`
- v2: `permit.UserAttributesService.Delete`

`UserAttributesService.Delete(ctx, attributeKey)` sends no query: the `Page` and `PerPage`
setters of v1's request builder, which the API's documentation lists for the route, are not
carried over, as the request names one attribute in its path, and neither is `ResourceId`. It
returns nil when the API answers 404, as the attribute is gone already, such as when a retry
follows a delete that succeeded; v1 returned the error of the generated client. A mistyped key
returns nil too, so check the key before a delete that must remove something. Any other error
status is a `*permit.Error` with the status and the body.

## Wire behaviour

<a id="M-wire-1"></a>
### M-wire-1: Request bodies keep `<`, `>` and `&` as they are

- v1: request bodies with `<`, `>` and `&` escaped as `\u003c`, `\u003e` and `\u0026`
- v2: request bodies with `<`, `>` and `&` as they are

v1 wrote request bodies with the escapes that `encoding/json` adds for HTML. v2 sends these
characters as they are. A JSON reader, such as the API, reads the same string either way: only
the bytes of the body differ, as a proxy or a test that compares them sees.

<a id="M-wire-2"></a>
### M-wire-2: Numbers in a query are written as JSON writes them

- v1: attribute filter values written with fmt's `%v`, such as `1.23456789125e+08` for 123456789.125
- v2: numbers written as `encoding/json` writes them, such as `123456789.125`

v1 wrote the values of attribute filters with fmt's `%v`, which writes a large or small `float64`
in e notation, such as `9.007199254740992e+15` for 2^53. v2 writes the shortest text that reads
back as the same `float64` in the form `encoding/json` uses, such as `9007199254740992` or
`0.30000000000000004`, with e notation only below 1e-6 and from 1e21 on, and an integer as its
decimal digits, such as `9007199254740993`.

<a id="M-wire-3"></a>
### M-wire-3: An empty path parameter fails before the request

- v1: an empty key or ID sent as an empty segment of the path, such as `GET /v2/facts/p//users`
- v2: an error that matches `permit.ErrInvalidConfig`, and no request

The key or the ID of an object goes into the path of a request. v1 sent an empty one as an empty
segment, so the request reached another route or none, and the API answered for that route, such
as with 404. v2 fails the call with an error that matches `permit.ErrInvalidConfig`, and sends
nothing, when a path parameter is empty, and when it is `.` or `..`, which a server or a proxy on
the way may resolve as a step in the path.

<a id="M-wire-4"></a>
### M-wire-4: Requests name the SDK in their headers

- v1: the User-Agent `OpenAPI-Generator/1.0.0/go` on requests to the API and on facts requests sent through the PDP, and Go's default `Go-http-client/1.1` (`Go-http-client/2.0` over HTTP/2) on decision requests to the PDP
- v2: the User-Agent `permit-golang/<version> (<Go version>)`, with `X-Permit-SDK-Language: go` and `X-Permit-SDK-Version: <version>`, on every request

v2 sends the same headers to the API and to the PDP: a User-Agent that names the SDK, its version
and the version of Go that built the application, such as `permit-golang/2.0.0 (go1.27.1)`, and
the SDK's language and version in `X-Permit-SDK-Language` and `X-Permit-SDK-Version`, so the PDP
can tell the Go SDK's requests apart. A proxy, a firewall rule or a log filter that matches v1's
User-Agent must match the new one. Every request also carries `Accept: application/json`, and a
request with a body `Content-Type: application/json`.

<a id="M-wire-5"></a>
### M-wire-5: A request is sent again after a 429, and after a failure when it is safe to repeat

- v1: each request sent once, so a 429, a 5xx reply or a failed connection was the result of the call
- v2: a request sent again after a 429, and after a 500, 502, 503 or 504 reply or a failed connection when sending it twice does what sending it once does; `permit.WithRetry` sets how many times

v1 sent each request once. v2 sends a request again, up to 3 times by default:

- after a reply with status 429, to any operation except one whose documentation says that it is
  sent once, as the server did not act on the request. It waits for what the reply's
  `Retry-After` header asks, in seconds or as an HTTP-date; a `Retry-After` of more than 60
  seconds, or one that would pass the call's deadline, ends the call at once with the 429, which
  matches `permit.ErrRateLimited`;
- after a reply with status 500, 502, 503 or 504, or a request that got no complete reply, only
  to an operation that does the same when it is sent twice, such as a GET, a PUT or a DELETE. A
  POST that creates an object is not sent again after these replies; only a 429 makes the client
  send it again.

Without a `Retry-After` header, the wait before a retry is random: up to 200 milliseconds before
the first, and up to twice as long before each later one, up to 5 seconds. The timeout of a call,
`permit.WithTimeout`, covers every attempt and every wait, and a call never waits past it: it
returns the error of its last attempt at once instead. A server, a proxy or a log that counts
requests sees each attempt, and the client writes a Debug line for each. To send each request
once, as v1 did, pass `permit.WithRetry(permit.RetryPolicy{})`.

<a id="M-scope-4"></a>
### M-scope-4: The scope of the API key is looked up only for a call that needs it

- v1: one `GET /v2/api-key/scope` request before the first API call of a client, whatever it acts on, and one from each `SetContext`
- v2: one `GET /v2/api-key/scope` request for each client, before the first call that acts on a project or an environment, or from `Client.Scope`; none with `permit.WithScope`

v1 looked up the scope of the API key before the first call of a client, whatever the call acted
on, and again in each `SetContext`, which checked the scope it was given against it. v2 looks it
up only for a call that acts on a project or an environment, or for `Client.Scope`, and never
when `permit.WithScope` gives the scope. The calls that need the scope while the lookup runs wait
for it, and the client keeps the scope of the first lookup that succeeds for its life; a lookup
that fails is not kept, so the next call that needs the scope sends it again. The lookup follows
`permit.WithRetry` and `permit.WithTimeout`. A call whose context ends while it waits for the
lookup returns at once, and the lookup goes on for the other calls that wait for it.

<a id="M-facts-2"></a>
### M-facts-2: Facts go to the PDP container with WithFactsViaPDP

- v1: facts requests sent to the same place with or without `WithProxyFactsViaPDP(true)`: from v1.2.8 on, to the Permit API at `/v2/facts/{proj_id}/{env_id}/...`
- v2: facts requests sent to the PDP container at `/facts/...` with `permit.WithFactsViaPDP()`, and to the Permit API without it
- Bug fix: could never work before

In v1, `WithProxyFactsViaPDP(true)` did not change where the facts requests went: from v1.2.8 on,
each went to the Permit API, so the PDP container never saw them, and v1.2.7 and earlier sent
them to the PDP URL with or without the option. With `permit.WithFactsViaPDP()`, v2 sends the
calls of users, tenants and the users in them, role assignments, resource instances and
relationship tuples, one at a time and in bulk, to the PDP URL under `/facts/`, without the
project and the environment: the container acts on the environment of its own API key, and the
client sends no request for the scope of its API key for these calls. The calls of set rules,
user invites and proxy configs, which the facts API also serves, go to the Permit API as before.

Only the PDP container serves `/facts/`. The cloud PDP answers each of these calls with 404 and an
empty body, which the call returns as an error that matches `permit.ErrContainerPDPOnly`, and
`permit.New` logs a warning when the option is given with the URL of the cloud PDP.

<a id="M-checks-14"></a>
### M-checks-14: BulkCheck sends one request for all its checks

- v1: one `POST /allowed/bulk` request for each tenant of the checks, one after another
- v2: one `POST /allowed/bulk` request for all the checks, in their order

v1 sent the checks of `BulkCheck` in a request for each tenant of their resources. v2 sends all
the checks in one request, whatever their tenants, in the order of the slice, and the PDP answers
each in that order; `FilterObjects` does the same with its resources. A proxy or a log that counts
requests sees one request for a call.

<a id="M-checks-22"></a>
### M-checks-22: UserPermissions sends the context and each filter it is given

- v1: a `POST /user-permissions` request with the user and the tenants only from `GetUserPermissions`, and with an empty filter or an empty context left out of the body
- v2: a `POST /user-permissions` request with the user, each filter and the context that the call is given; no request for a filter without keys

v1's `GetUserPermissions` sent no context, resources or resource types, so a policy that reads the
context saw none; `GetUserPermissionsWithOptions` sent them, so for its callers only the rule for a
filter without keys changes. v2 sends the context of `permit.WithDecisionContext`, with values of any JSON
type, and the keys of `permit.WithTenants`, `permit.WithResources` and `permit.WithResourceTypes`,
in `context`, `tenants`, `resources` and `resource_types`. It sends no request for a filter
without keys, which v1 left out of the body, as [M-checks-16](#M-checks-16) says.

<a id="M-checks-25"></a>
### M-checks-25: AllTenantsCheck sends the context it is given

- v1: a `POST /allowed/all-tenants` request without a context from `permit.Client.AllTenantsCheck`, and with a context of strings from `enforcement.PermitEnforcer.AllTenantsCheck`
- v2: a `POST /allowed/all-tenants` request with the context of `permit.WithDecisionContext`, with values of any JSON type

v1's `permit.Client.AllTenantsCheck` sent no context, so a policy that reads the context saw none.
v2 sends the context of `permit.WithDecisionContext` in `context`, with values of any JSON type,
such as numbers, booleans and nested objects, and leaves out a nil or empty one, as a check does.
The rest of the body is that of `Client.Check`: a resource without a tenant is sent with the tenant
`default`.

## Kept on purpose

What v2 keeps from v1 although a new major version could change it:

- The name of the package: v2's root package is `permit`, as v1's `pkg/permit` was, so code that
  refers to `permit.` keeps its qualifier.
- The attribute filters of the tenants: `permit.TenantsListOptions` sends each as the query
  parameter `attr_<name>`, as v1's `ListByAttributes` did, although the API's spec does not list
  these parameters: the API applies them.

## Appendix: v1 models types

Each type of v1's `pkg/models` and the v2 type or entry that replaces it.

<!-- apisurface:models:begin -->
<!-- apisurface -mode migration -write writes this table; do not edit it. -->

| v1 | v2 |
| --- | --- |
| `models.APIKeyCreate` | `models.APIKeyCreate` |
| `models.APIKeyOwnerType` | `models.APIKeyOwnerType` |
| `models.APIKeyRead` | `models.APIKeyRead` |
| `models.APIKeyScopeRead` | `models.APIKeyScopeRead` |
| `models.ActionBlockEditable` | `models.ActionBlockEditable` |
| `models.ActionBlockRead` | `models.ActionBlockRead` |
| `models.AddRolePermissions` | `models.AddRolePermissions` |
| `models.AttributeBlockEditable` | `models.AttributeBlockEditable` |
| `models.AttributeBlockRead` | `models.AttributeBlockRead` |
| `models.AttributeType` | `models.AttributeType` |
| `models.Attributes` | `models.Attributes` |
| `models.DerivedRoleBlockEdit` | `models.DerivedRoleBlockEdit` |
| `models.DerivedRoleBlockRead` | `models.DerivedRoleBlockRead` |
| `models.DerivedRoleRuleCreate` | `models.DerivedRoleRuleCreate` |
| `models.DerivedRoleRuleRead` | `models.DerivedRoleRuleRead` |
| `models.EnvironmentCopy` | `models.EnvironmentCopy` |
| `models.EnvironmentCopyScope` | `models.EnvironmentCopyScope` |
| `models.EnvironmentCopyScopeFilters` | `models.EnvironmentCopyScopeFilters` |
| `models.EnvironmentCopyTarget` | `models.EnvironmentCopyTarget` |
| `models.EnvironmentCreate` | `models.EnvironmentCreate` |
| `models.EnvironmentRead` | `models.EnvironmentRead` |
| `models.EnvironmentStats` | `models.EnvironmentStats` |
| `models.EnvironmentUpdate` | `models.EnvironmentUpdate` |
| `models.MemberAccessLevel` | `models.MemberAccessLevel` |
| `models.MemberAccessObj` | `models.MemberAccessObj` |
| `models.PDPConfigRead` | `models.PDPConfigRead` |
| `models.ProjectCreate` | `models.ProjectCreate` |
| `models.ProjectRead` | `models.ProjectRead` |
| `models.ProjectUpdate` | `models.ProjectUpdate` |
| `models.RelationBlockRead` | `models.RelationBlockRead` |
| `models.RelationCreate` | `models.RelationCreate` |
| `models.RelationRead` | `models.RelationRead` |
| `models.RemoveRolePermissions` | `models.RemoveRolePermissions` |
| `models.ResourceActionCreate` | `models.ResourceActionCreate` |
| `models.ResourceActionGroupCreate` | `models.ResourceActionGroupCreate` |
| `models.ResourceActionGroupRead` | `models.ResourceActionGroupRead` |
| `models.ResourceActionRead` | `models.ResourceActionRead` |
| `models.ResourceActionUpdate` | `models.ResourceActionUpdate` |
| `models.ResourceAttributeCreate` | `models.ResourceAttributeCreate` |
| `models.ResourceAttributeRead` | `models.ResourceAttributeRead` |
| `models.ResourceAttributeUpdate` | `models.ResourceAttributeUpdate` |
| `models.ResourceCreate` | `models.ResourceCreate` |
| `models.ResourceRead` | `models.ResourceRead` |
| `models.ResourceReplace` | `models.ResourceReplace` |
| `models.ResourceRoleCreate` | `models.ResourceRoleCreate` |
| `models.ResourceRoleRead` | `models.ResourceRoleRead` |
| `models.ResourceRoleUpdate` | `models.ResourceRoleUpdate` |
| `models.ResourceUpdate` | `models.ResourceUpdate` |
| `models.RoleCreate` | `models.RoleCreate` |
| `models.RoleRead` | `models.RoleRead` |
| `models.RoleUpdate` | `models.RoleUpdate` |
| `models.TenantCreate` | `models.TenantCreate` |
| `models.TenantRead` | `models.TenantRead` |
| `models.TenantUpdate` | `models.TenantUpdate` |

Not mapped yet: 330 of the 384 types of v1's pkg/models.

<!-- apisurface:models:end -->
