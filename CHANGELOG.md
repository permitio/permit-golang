# Changelog

## v1.3.0 (unreleased)

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
