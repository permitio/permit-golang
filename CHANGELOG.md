# Changelog

## v1.3.0 (unreleased)

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
