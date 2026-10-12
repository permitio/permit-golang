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
