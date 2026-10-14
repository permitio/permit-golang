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

## Wire behaviour

No entries yet.

## Kept on purpose

What v2 keeps from v1 although a new major version could change it:

- The name of the package: v2's root package is `permit`, as v1's `pkg/permit` was, so code that
  refers to `permit.` keeps its qualifier.

## Appendix: v1 models types

Each type of v1's `pkg/models` and the v2 type or entry that replaces it.

<!-- apisurface:models:begin -->
<!-- apisurface -mode migration -write writes this table; do not edit it. -->

Not mapped yet: 384 of the 384 types of v1's pkg/models.

<!-- apisurface:models:end -->
