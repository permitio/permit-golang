# Changelog

## v1.3.0 (unreleased)

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
- `Check`, `BulkCheck`, `FilterObjects`, `AllTenantsCheck`, `GetUserPermissions` and
  `GetUserPermissionsWithOptions` return a `PermitError` with `ErrorCode` `UnexpectedError` and the
  status when the PDP answers with a status the SDK did not handle, such as 400, 421 or 429.
  Before, `Check` returned `false` with a nil error, so a failure looked like a deny, and
  `AllTenantsCheck` returned no tenants. `errors.HttpErrorHandle(nil, response)` now returns this
  error, instead of nil, for a status outside 200-299 that it does not map to another code.
- Errors from `BulkCheck`, `FilterObjects`, `AllTenantsCheck`, `GetUserPermissions` and
  `GetUserPermissionsWithOptions` include the PDP response body in `ResponseBody`; it was empty.
- `AllTenantsCheck` returns only the tenants where the PDP allowed the action. Before, it also
  returned tenants whose decision was `"allow": false`.
