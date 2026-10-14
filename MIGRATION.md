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

## Kept on purpose

What v2 keeps from v1 although a new major version could change it:

- The name of the package: v2's root package is `permit`, as v1's `pkg/permit` was, so code that
  refers to `permit.` keeps its qualifier.

## Appendix: v1 models types

Each type of v1's `pkg/models` and the v2 type or entry that replaces it.

<!-- apisurface:models:begin -->
<!-- apisurface -mode migration -write writes this table; do not edit it. -->

| v1 | v2 |
| --- | --- |
| `models.APIKeyScopeRead` | `models.APIKeyScopeRead` |
| `models.AttributeType` | `models.AttributeType` |
| `models.Attributes` | `models.Attributes` |

Not mapped yet: 381 of the 384 types of v1's pkg/models.

<!-- apisurface:models:end -->
