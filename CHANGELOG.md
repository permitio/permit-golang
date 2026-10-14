# Changelog

## v2.0.0 (unreleased)

v2 is the module `github.com/permitio/permit-golang/v2`. [MIGRATION.md](MIGRATION.md) lists each
change from v1 and what to use instead.

### Client

- `permit.New(opts ...permit.Option)` builds a `*permit.Client` from the options `WithAPIKey`,
  `WithAPIURL`, `WithPDPURL`, `WithHTTPClient`, `WithTimeout`, `WithRetry`, `WithLogger`,
  `WithScope` and `WithFactsViaPDP`, and returns an error when the configuration is not one the
  SDK can use.
- A setting that no option gives comes from the environment variables `PERMIT_API_KEY`,
  `PERMIT_API_URL` and `PERMIT_PDP_URL`, and else from its default: `https://api.permit.io` for
  the API, `http://localhost:7766` for the PDP and 30 seconds for the timeout. A variable that is
  set to the empty string counts as unset. There is no default API key.
- `New` sends no request. It returns an error that matches `ErrInvalidConfig`, and names the
  option or the variable, when there is no API key; when an option gives an empty key or URL, a
  nil `*http.Client`, a timeout of zero or less, or a `MaxRetries` or `MaxBackoff` of less than
  zero; when a URL does not use `http` or `https` (in any case), has no host, or has a user name
  or a password, a query or a fragment; and for the zero `Option`. A path after the host of a URL
  is kept, and one slash at its end is removed.
- `WithTimeout` sets the longest that a call may take, across all the requests it sends. When the
  context of a call has a deadline, the deadline bounds the call instead. A call that runs out of
  time returns an error that matches `context.DeadlineExceeded`.
- Each client has an `*http.Client` of its own, whose transport is a copy of
  `http.DefaultTransport`: it uses the proxy that the environment names and the default TLS
  settings, and keeps up to 64 idle connections to each host. `WithHTTPClient` sends the requests
  through the `*http.Client` given, and changes none of its fields.
- `Client.Close` closes the idle connections of the client's own `*http.Client`, and not those of
  one that `WithHTTPClient` gave. It returns nil, also when called again, and a call that starts
  after it fails with an error that matches `ErrClosed` and sends no request. A call in flight
  when `Close` is called runs on and returns its result or its error; it sends no other attempt,
  and a retry it was waiting for returns the error of its last attempt. A client that is garbage
  collected without `Close` has the idle connections of its own `*http.Client` closed too.
- A `Client` and an `Option` print with `[REDACTED]` in place of the API key: `fmt` with every
  verb, `String`, `GoString` and the `LogValue` that `log/slog` calls.
- `WithScope(project, environment)` sets the project and the environment that the client's calls
  act on, by key or by ID, and the client then sends no request for the scope of its API key.
  `New` returns an error that matches `ErrInvalidConfig` for an environment without a project, an
  empty project, and a project or an environment that is `.` or `..`.
- `Client.Scope(ctx)` returns the `permit.Scope`, the project and the environment, that the
  client's calls act on: those that `WithScope` gave, or else those of the API key, which the
  client looks up once with `GET /v2/api-key/scope`. The calls that need the scope while the
  lookup runs wait for it, and the client keeps the scope of the first lookup that succeeds; a
  lookup that fails is not kept, so the next call looks it up again. A failed lookup returns the
  `*permit.Error` of the reply, such as one that matches `ErrUnauthorized`, or after the retries
  `ErrRateLimited`. A call whose context ends while it waits for the lookup returns an error that
  matches the context's error at once, and the lookup goes on for the other calls; `Close` ends
  the lookup.
- A call that acts on an environment needs a project and an environment in the scope, and one
  that acts on a project needs a project. When the scope lacks one, the call returns an error
  that matches `ErrInvalidConfig`, names its operation and `WithScope`, and sends no request for
  the operation; without `WithScope`, the client may first look up the scope of its API key. A
  call that acts on the organization needs no scope, and sends no lookup.

### Facts

- `WithFactsViaPDP()` sends the calls of the facts API, those of users, tenants and the users in
  them, role assignments, resource instances and relationship tuples, one at a time and in bulk,
  to the PDP container at the PDP URL under `/facts/...`, instead of to the Permit API under
  `/v2/facts/{proj_id}/{env_id}/...`. These calls fill in no project and no environment, as the
  container acts on the environment of its own API key, and send no request for the scope of the
  client's API key. The calls of set rules, user invites, proxy configs, the schema and projects
  go to the API with or without the option. v1's `WithProxyFactsViaPDP(true)` did not change
  where facts requests went: v1.2.8 and later sent every one to the API.
- A 404 with an empty body to a facts call through the PDP, which is how the cloud PDP answers
  every route under `/facts/`, is an error that matches `ErrContainerPDPOnly`, not `ErrNotFound`,
  and names the route under `/facts/`, also for a delete, which otherwise returns nil for a 404.
- The package documentation lists the 11 routes on which the PDP container can hold its answer
  until a write reaches its policy, and says that it answers every other facts call without
  waiting. A test checks the list against the container's spec.

### Errors

- A call whose request fails returns a `*permit.Error`, or an error that wraps one. It names the
  operation, the HTTP method and the route template of the request, and holds the status, the
  API's error code, title, detail and request ID, the body of a reply whose status is not in
  200-299, cut to 8 KiB, and the cause of a request that got no reply. Its text never includes the
  body.
- Check what failed with `errors.Is` and the sentinels `ErrNotFound` (404), `ErrConflict` (409),
  `ErrUnauthorized` (401), `ErrForbidden` (403) and `ErrRateLimited` (429), and
  `ErrContainerPDPOnly`, `ErrInvalidConfig`, `ErrMalformedDecision` and `ErrClosed`. Read the
  fields with `errors.AsType[*permit.Error]` or `errors.As`.
- The cause of a request that got no reply stays wrapped, so `errors.Is(err,
  context.DeadlineExceeded)` reports a deadline.
- An error has its credentials replaced with a marker when the SDK creates it: the client's API
  key and the credentials its calls return, which each client keeps to itself, Permit API keys,
  Authorization header values, and the values of JSON fields that name a credential, such as
  `token` or `password`. A reply in 200-299 that the SDK cannot decode gives an error that names
  the model and keeps nothing of the body.

### Logging

- `WithLogger(*slog.Logger)` sets the logger that the client writes to. By default the client
  logs nothing, and so it does with a nil `*slog.Logger` or one whose handler is nil, such as the
  zero `slog.Logger`. The module does not require zap: an application that logs with zap gives
  the client a logger whose handler is zapslog's, as the documentation of `WithLogger` shows.
- Every line has a fixed message that starts with `permit: `. At Debug, the client writes one
  line for each attempt of a request, with the attributes `route` (the route template),
  `status`, `attempt`, `duration` and `error`, and never a header or a body. At Warn, `New`
  writes each warning about the configuration once: when the API URL uses `http://` to a host
  that is not loopback (localhost, 127.0.0.0/8 or ::1), and when `WithFactsViaPDP` sends facts to
  the cloud PDP, `cloudpdp.api.permit.io`, which does not serve them. A PDP URL over `http://`
  gets no warning.
- Each line has the client's credentials replaced with a marker before the handler gets it: in
  the message, in the keys and string values of attributes, in groups at any depth, in what a
  `slog.LogValuer` returns, and in errors, which reach the handler as their text only.

### Requests and replies

- Every request, to the API and to the PDP, carries `Authorization: Bearer <API key>`,
  `User-Agent: permit-golang/<version> (<Go version>)`, such as
  `permit-golang/2.0.0 (go1.27.1)`, `X-Permit-SDK-Language: go`,
  `X-Permit-SDK-Version: <version>` and `Accept: application/json`, and a request with a body
  `Content-Type: application/json`.
- The client closes the body of every reply. Before it does, it reads and discards up to 1 MiB of
  what it did not read, so that the next request goes over the same connection instead of a new
  one.
- A reply in 200-299 whose body is empty, or is not what the call expects (a JSON object, or a
  list as a JSON array or as the paginated object), is an error that names the model, never an
  empty result without an error.
- Deleting an object by a path that names it, such as a user by its key, returns nil when the API
  answers 404, as the object is gone already. A removal whose body names what it removes, such as
  a role assignment, returns an error that matches `ErrNotFound` for a 404, which can mean a
  mistyped key.
- A 404 with an empty body from a route that only the PDP container serves, as the cloud PDP
  answers it, is an error that matches `ErrContainerPDPOnly`, not `ErrNotFound`, and names the
  route.
- Request bodies are JSON that `encoding/json` writes, with `<`, `>` and `&` sent as they are
  rather than escaped as `\u003c`, `\u003e` and `\u0026`.
- A number in a query keeps every digit: a `float64` is the shortest text that reads back as the
  same value, as `encoding/json` writes it, such as `0.30000000000000004`, and an integer is its
  decimal digits, such as `9007199254740993`.
- A list in a query repeats its key, such as `user=a&user=b`. A filter that is not set is left
  out, and the query parameters that the API marks deprecated, `detailed`, `include_total_count`
  and `return_deleted`, are never sent.
- Each path parameter is escaped on its own, so a key such as `a/b` stays one segment, `a%2Fb`.
  An empty path parameter, or one that is `.` or `..`, fails with an error that matches
  `ErrInvalidConfig` before any request is sent.
- A list reply is read by its first byte, as a JSON array or as the paginated object that holds
  the array in `data`, so both shapes give the same items, and a reply of another shape is an
  error rather than an empty list.
- A number in a reply that a model holds in an `any`, such as an attribute value, is read as a
  `json.Number` with every digit of its text, such as `9007199254740993`, rather than as a
  `float64`, which rounds integers beyond 2^53. Read it with its `Int64`, `Float64` or `String`
  method.

### Lists

- A `List` method returns one page as a `*permit.Page[T]`: its items in `Data`, the `Page` and
  `PerPage` that the call asked for, and `TotalCount` and `PageCount` when the reply reports
  them, else nil. `permit.ListOptions{Page, PerPage}` picks the page; 0 asks for the first page,
  and for the API's default number of items on it. A `Page` or `PerPage` less than zero is an
  error that matches `ErrInvalidConfig`, and no request is sent.
- An `All` method reads every page and returns an `iter.Seq2[T, error]` of the items, for a
  `range` loop. It asks for the largest page that the route accepts, unless `PerPage` is set,
  and starts at `Page`. It stops after a page without items, or after the last page when the
  reply reports `page_count`, and never after a page with fewer items than it asked for. A
  request that fails ends the loop with its error, yielded once. A loop that stops early sends no
  other request. Each page is a call of its own, with its own timeout and retries.
- The client sends `per_page` as given, with no limit of its own, also above the route's limit,
  which differs from route to route.

### Models

- `models.Optional[T]` is the type of a field of an update model that the API accepts null for.
  Its zero value leaves the field out of the body, `models.Null[T]()` sends null and
  `models.Some(v)` sends `v`, also when `v` is `""`, `0` or `false`. `Get`, `IsNull` and
  `IsZero` tell the three states apart. A field of this type is tagged `omitzero`; encoding an
  unset `Optional` without it fails rather than sending null, and so does `Some` of a value that
  encodes as null, such as a nil slice.
- Enums are open: `models.AttributeType` is a string type with a constant for each value the SDK
  knows, such as `models.AttributeTypeString`, and a `Known` method. A value that the API adds
  later decodes as it is, and `Known` reports false for it.
- `models.Attributes` is a `map[string]any` that keeps each number as a `json.Number`, also in
  nested objects and arrays and under a plain `json.Unmarshal`. In an update model, nil leaves
  the field out and an empty map sends `{}`.
- `models.Time` embeds `time.Time` and reads an RFC 3339 timestamp with or without a zone; one
  without a zone is in UTC.
- `models.APIKeyScopeRead` is the scope of an API key: the IDs of its organization, project and
  environment.

### Retries

- A call sends its request again after a reply with status 429, to any operation except one whose
  documentation says that it is sent once, after the wait that the reply's `Retry-After` header
  asks for, in seconds or as an HTTP-date.
- A call sends its request again after a reply with status 500, 502, 503 or 504, or a request
  that got no complete reply, only to an operation that does the same when it is sent twice, such
  as a GET, a PUT or a DELETE. A POST that creates an object is not sent again after these
  replies; only a 429 makes the client send it again.
- Without a `Retry-After` header, the wait before a retry is random: up to 200 milliseconds
  before the first, and up to twice as long before each later one, up to 5 seconds.
- `WithRetry(permit.RetryPolicy{MaxRetries: n, MaxBackoff: d})` sets the most retries of a call,
  3 by default, and the longest wait without a `Retry-After` header, 5 seconds by default (and
  for a `MaxBackoff` of 0). A `MaxRetries` of 0 sends each request once.
- The timeout of a call covers every attempt and every wait between them. A call never waits past
  its deadline, nor for a `Retry-After` of more than 60 seconds: it returns the error of its last
  attempt at once, which matches `ErrRateLimited` for a 429. A call whose context ends while it
  waits returns an error that matches the context's error.
- Each attempt has its own Debug line, with its number, from 1, in `attempt`.

## v1.3.0 (2026-10-13)

### API keys and proxy config secrets in logs from v1.2.8 and earlier

If your application printed the SDK configuration or a client, or logged the SDK's errors or its
log output, with v1.2.8 or earlier, those logs may contain your Permit API key. Rotate the API key
and delete those logs. If you read proxy configs whose secret is a set of headers, and logged
`PermitError.ResponseBody` or debug output, those logs may also contain the header values: rotate
those secrets too.

### Behaviour changes

- Checks send a resource key for resources that have an ID and no key, so the PDP checks the
  resource instance instead of only its type:
  - `Check`, `BulkCheck` and `AllTenantsCheck`: a resource built with `WithID` and no `WithKey`
    sends its ID as `key`, the field the PDP reads.
  - `FilterObjects`: an `*enforcement.Resource` built with `WithKey` sends its key, which was
    dropped before, and any other object, including your own `ResourceI` types, sends `GetID()`
    as `key`.

  Before, the PDP received no key for these resources, so it checked the resource type, and every
  object of that type got the same decision. Decisions can now change for resources that have
  instance-level roles or relationships in Permit. Check the calls that pass resources built with
  `WithID`, and the objects you pass to `FilterObjects`: the IDs they send must be the resource
  instance keys you use in Permit. To keep a type-level check, leave both the ID and the key empty.
- `Check`, `BulkCheck`, `FilterObjects`, `AllTenantsCheck`, `GetUserPermissions` and
  `GetUserPermissionsWithOptions` return a `PermitError` with `ErrorCode` `UnexpectedError` and the
  status when the PDP answers with a status the SDK did not handle, such as 400, 421 or 429.
  Before, `Check` returned `false` with a nil error, so a failure looked like a deny, and
  `AllTenantsCheck` returned no tenants. `errors.HttpErrorHandle(nil, response)` now returns this
  error, instead of nil, for a status outside 200-299 that it does not map to another code.
  A rate-limited check (429) now returns an error instead of `false`: handle the error, for
  example by retrying with a backoff, instead of treating it as a deny.
- A JSON document in `PermitError.ResponseBody` that has a key naming a credential (see Fixes), or
  a credential written with JSON escapes, is now stored encoded again: compact, with the keys of
  each object sorted. Other bodies keep their bytes, apart from the values replaced in them.
- `Check`, `BulkCheck`, `FilterObjects`, `AllTenantsCheck`, `GetUserPermissions` and
  `GetUserPermissionsWithOptions` log the PDP response body of a failed request only at Debug
  level, with credentials replaced as in `PermitError.ResponseBody`. Before, they logged the whole
  body at Error level, and all but `Check` used it as the log message. The Error level line now
  has a fixed message, the error and the status code (`status_code`), without the body.

### Dependencies

- The module no longer requires `golang.org/x/net` or `golang.org/x/oauth2`, so its `go.mod` also
  drops `google.golang.org/appengine`, `github.com/golang/protobuf` and
  `google.golang.org/protobuf`. Applications that use the SDK no longer inherit these modules from
  it, including the old minimum versions of x/net and x/oauth2, which have published advisories.

### Releases

- Releases no longer attach prebuilt archives of the example server in `main.go`; a module
  version is published by its git tag. A manual workflow checks that the module proxy.golang.org
  serves for a version matches its tag.

### CI

- Pull requests, and pushes to `master` and release branches, run gofmt, `go vet`, the unit tests
  with the race detector on Go 1.19 and Go 1.27, and govulncheck, without Permit credentials, and
  fail when `go.mod` or `go.sum` is not tidy. The tests that need live Permit credentials now build
  only with the `integration` build tag.
- A separate workflow runs the integration tests against Permit for pushes to `master` and release
  branches and for pull requests from branches of this repository. It runs them with a PDP
  container pinned by digest, and deletes what each run created.
- The GoReleaser and pre-commit workflows and `.pre-commit-config.yaml` are removed.

### Fixes

- `WaitForSync` no longer changes the client it is called on. It wrote `X-Wait-Timeout` and
  `X-Timeout-Policy` into headers shared by every API (`Users`, `Tenants`, ...) of the same
  client, so later requests that were not meant to wait carried them, and calling it while other
  requests were in flight could crash the process with a fatal concurrent map access error. The
  headers now go only on the requests of the client that `WaitForSync` returns, and `WaitForSync`
  no longer shares headers with other requests.
- `WaitForSync` and `NewFactsClientConfig` send the timeout in seconds without truncating it:
  1.5s is sent as `1.5` instead of `1`, and a timeout under one second is no longer sent as `0`.
- `ConditionSets.List` could never work before: the API answers with a JSON array, the SDK read
  only the paginated form, and the result was always empty. It now reads both forms.
- `RoleAssignments.List` returns an empty list instead of a nil pointer when there are no role
  assignments. `ResourceRoles.GetAncestors` and `ResourceRoles.GetDescendants` do the same when
  the response holds no roles.
- `ConditionSets.List`, `RoleAssignments.List`, `RoleAssignments.ListDetailed`,
  `Users.GetAssignedRoles`, `ResourceRoles.GetAncestors` and `ResourceRoles.GetDescendants` return
  an empty list instead of panicking when a successful response has an empty body.
- An API URL too short to hold a host, such as the empty string in `NewPermitConfig("", ...)`, no
  longer makes `NewPermit` panic; requests made with it return an error. `NewFactsClientConfig`
  handles a short PDP URL the same way.
- A nil logger no longer makes the SDK panic the first time it logs an error. `NewPermitConfig`
  with a nil logger, a builder chain used without `Build()`, a `PermitConfig` literal and
  `WithLogger(nil)` now all give a client that uses a no-op logger.
- `ResourceActionGroups.Delete` could never work before: it sent the request to the resource
  action route, so it deleted the action whose key matched the group's key, or failed when there
  was no such action. It now deletes the action group.
- Concurrent first calls on a client whose context is not set no longer each send a
  `GET /v2/api-key/scope` request and write the client's context at the same time. One call sends
  the request; calls that start while it is in flight wait for it, or until their own context
  ends, instead of sending their own, and share its result. The next call after a failed request
  sends a new one.
- When the API key scope request fails, the error is still a `PermitError` with `ErrorCode`
  `ContextError`, and it now keeps the cause: the text includes the status and the underlying
  error, `StatusCode` and `ResponseBody` come from the scope response, and `errors.Is`,
  `errors.As` and `errors.Unwrap` reach the underlying error, such as `context.Canceled` or a
  connection error. Before, the status, body and cause were dropped, and every failure other than
  401 and 403 had the same text. Because the text now includes the cause, match on `ErrorCode`
  (`ContextError`) or on the message constants, such as `errors.UnauthorizedMessage`, not on the
  full text. `PermitError` has a new `Unwrap` method, and `errors.NewPermitContextErrorWithCause`
  is new.
- Errors from `BulkCheck`, `FilterObjects`, `AllTenantsCheck`, `GetUserPermissions` and
  `GetUserPermissionsWithOptions` include the PDP response body in `ResponseBody`; it was empty.
- `AllTenantsCheck` returns only the tenants where the PDP allowed the action. Before, it also
  returned tenants whose decision was `"allow": false`.
- Printing the SDK configuration or a client with `fmt` (the `%v`, `%+v`, `%#v`, `%s`, `%q`,
  `%x`, `%X` and `%d` verbs, with any flags and width) no longer shows the API key:
  - `PermitConfig` holds the API key behind a pointer, so a `PermitConfig`, the config builder, a
    `permit.Client`, its `Api` and the APIs in it such as `Users`, and an enforcer print an
    address in place of the key. Their other fields print as before. Two `PermitConfig` values
    from separate `NewPermitConfig` calls with the same arguments are no longer equal with `==`,
    because the key is compared by its pointer; copies of one config are still equal.
  - `openapi.Configuration`, such as the ones that `api.NewClientConfig` and
    `api.NewFactsClientConfig` return, and `openapi.APIClient` print `[REDACTED]` in place of the
    values of credential headers such as `Authorization`. They have new `Format` methods, and
    `openapi.Configuration` also has new `String`, `GoString` and `MarshalJSON` methods, so
    `encoding/json` and loggers that encode it as JSON show `[REDACTED]` for those headers too. A
    pointer to either type now prints without the leading `&`. A struct that embeds either type
    prints as that value alone, and one that embeds `openapi.Configuration` also encodes to JSON
    as it alone. If you set `Debug` on an `openapi.Configuration`, the generated client still
    logs each request with its headers; the SDK never sets it.
- The text and `ResponseBody` of a `PermitError` made from an API or PDP response have the API
  key the request was sent with, and any value that starts with `permit_key_`, replaced with
  `[REDACTED]`, also where a JSON body writes them with escapes. In a JSON `ResponseBody`, the
  strings and numbers under keys named `secret`, `token`, `api_key`, `apikey`, `password`,
  `passwd`, `pwd`, `authorization`, `secret_key`, `private_key`, `access_key`, `credential`,
  `credentials` or `cookie`, or ending in one of them such as `access_token`, `X-API-Key`,
  `aws_secret_access_key` or `Set-Cookie` (ignoring case, `-` and `_`), are replaced with
  `[REDACTED]` at any depth, and the rest of the body is kept. This includes the body kept when
  `ProxyConfigs` cannot read a proxy config whose secret is a set of headers. An API key shorter
  than 16 characters is not replaced, and a body nested more than 1000 levels deep is replaced
  with `[REDACTED]` as a whole. `ResourceActions`, `ResourceActionGroups` and
  `ResourceAttributes` still return the generated client's error type unchanged, as in v1.2.8;
  the SDK's own log line for those errors has the API key and `permit_key_` values replaced.
- `Tenants.List` and `Tenants.ListByAttributes` return the error when the request fails. Before,
  they logged it and returned a nil error, so a 401 or 500 response or a connection failure looked
  like an empty list, and a 200 response whose body they could not read gave no tenants or a
  partly read list. Now they return a nil list with the error: a `PermitError` with the status
  code when a response arrived, or else the error from sending the request, such as a connection
  failure or a cancelled context.
- The context error that a method returns when the client has no project or environment, such as
  with an organization API key and no `SetContext` call, names the call that sets them,
  `client.Api.SetContext(ctx, project, environment)`, and what to pass to it. Before, it named
  `PermitClient.SetPermitContext()`, which does not exist. The `ErrorCode` is still
  `ContextError`: match on `ErrorCode`, not on the text.
- When `SetContext` fails, its Error log line names the project and environment it could not set;
  the message was empty. With a project API key and no project, the logged error also asks for an
  environment.
- The list methods' log line for a page or page size out of range gives the page size limit as
  `100` and names what the method lists. Before, it gave `d`, and some methods named the wrong
  objects.

### Documentation

- The README examples compile. The `SyncUser` example checks the error before it reads the user,
  and the `Check` example prints the error and passes a resource type to `ResourceBuilder`. New
  README sections say which methods use the API URL and which use the PDP URL, with their
  defaults, and when to call `SetContext` for each API key level.
- The usage examples in the `pkg/api` doc comments call the method they document, with arguments
  of the right types, and the examples this release changes render as code in godoc. godoc now
  shows the `Environments.Update` doc comment. The doc comments of the `ResourceActionGroups`
  methods, of `ListByAttributes` on `Tenants`, `Roles` and `ResourceActions`, and of
  `ProxyConfigs.GetByKey` name their own method and object. `ProxyConfigs.List` says it lists the
  proxy configs of the context's environment, the `Projects` methods give the API key level each
  one needs, and `SetContext` has a doc comment.
