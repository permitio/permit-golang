# Contributing

Thanks for helping with the Go SDK for Permit.io. Everyone who takes part follows the
[Code of Conduct](CODE_OF_CONDUCT.md).

## Reporting a vulnerability

Do not open a public issue or pull request for a vulnerability. Report it privately as
[SECURITY.md](SECURITY.md) describes.

## Versions

Changes go to v2 of the SDK, the module `github.com/permitio/permit-golang/v2`, on the `master`
branch.

v1, the module `github.com/permitio/permit-golang`, receives no fixes after v1.4.0, its last
release. v1 changes are not accepted on `master`.

## Setup

You need Go 1.26.0 or newer, and git. `go.mod` sets the minimum Go version (`go 1.26.0`) and a
`toolchain` line: with the default `GOTOOLCHAIN=auto`, the go command downloads and uses that
toolchain when the Go you have installed is older.

## Build and test

Run these from the repository root before you open a pull request:

```sh
gofmt -l .        # prints the Go files that are not formatted; prints nothing when all are
go vet ./...
go build ./...
go test -race -count=1 -shuffle=on ./...
go mod tidy -diff # prints the changes go.mod and go.sum need; prints nothing when they are tidy
```

Run the tests on the minimum Go version too:

```sh
GOTOOLCHAIN=go1.26.0 go test -race -count=1 -shuffle=on ./...
```

Follow `.editorconfig`: UTF-8, LF line endings, a final newline, no trailing whitespace, and lines
of at most 100 characters (Markdown has no limit). Go files use tabs and are formatted with
`bin/golangci-lint fmt` (gofumpt, gci and golines), shell scripts with `shfmt -i 2`. Every exported
identifier has a doc comment.

## Git hooks

The hooks in `.pre-commit-config.yaml` run with [prek](https://prek.j178.dev). Install the
version that `tools/binaries.lock` pins as `bin/prek`, then install the hooks from your main clone
(a git worktree shares its hooks directory with the main clone) and run them once on every file:

```sh
.github/scripts/fetch-tool.sh prek
bin/prek install            # runs the hooks on each commit and checks each commit message
bin/prek run --all-files    # every hook on every file
```

The prek job in [CI](#ci) runs the same hooks. The first run downloads the hooks and builds the
ones written in Go, so it needs the network; later runs work offline. The hooks check:

- formatting and lint with golangci-lint, through `.github/scripts/golangci-all.sh` (below);
- that `go mod tidy -diff` prints nothing, in the root, in `tools/` and in `consumer/`;
- `.editorconfig`, with editorconfig-checker;
- spelling with typos (`_typos.toml`), shell scripts with shellcheck and shfmt, GitHub Actions
  workflows with actionlint and zizmor (`.github/zizmor.yml`);
- staged changes for secrets with gitleaks (`.gitleaks.toml`, which adds a rule for Permit API
  keys: use fake keys that start with `permit_key_FAKE`); CI scans the commits of each pull
  request with the same rules (see
  [Secret scan and dependency review](#secret-scan-and-dependency-review));
- that `.gitignore` still ignores credential files, with `.github/scripts/check-ignore.sh`;
- each commit message, with `.github/scripts/check-commit-msg.sh` (see
  [Commits and pull requests](#commits-and-pull-requests));
- whitespace, line endings, merge conflict markers, large files, private keys, symlinks, and that
  scripts are executable.

A hook that rewrites a file, such as the whitespace and spelling fixers, fails the run; check its
change and run the hooks again. Each script exits 0 when the check passes, 1 when it found a
problem, and 2 when it could not run the check; its header says when.

`.github/scripts/golangci-all.sh` runs the golangci-lint release that `tools/binaries.lock` pins,
which it installs as `bin/golangci-lint` when that is missing or another version, with
`.golangci.yml`: it verifies the configuration, checks the formatting of every Go file and lints
every module that git tracks a `go.mod` for, including the `e2e`-tagged tests. Run it directly to
lint without prek, and run `bin/golangci-lint fmt` to fix the formatting it reports.

To move the hooks to their newest releases that are at least 7 days old, run `bin/prek update`;
`.pre-commit-config.yaml` sets the 7-day cooldown and frozen commit SHAs for it. golangci-lint and
prek are updated in `tools/binaries.lock`, and editorconfig-checker in `tools/go.mod`, as
[tools/README.md](tools/README.md) describes.

A lint or format fix that changes no behaviour and is found after the code it fixes goes in its
own commit, and `testdata/api-surface.txt` (see
[Public surface and wire cases](#public-surface-and-wire-cases)) stays byte-identical in that
commit.

## Tests run offline

The tests that `go test ./...` runs never reach the Permit API, a PDP, Docker or any other network
service, and need no API key. A test that makes HTTP requests serves the replies from
`net/http/httptest` and points every URL of the client it builds at that server.

To check that no request leaves your machine, run the tests with a proxy address that refuses
connections. Go sends requests for loopback addresses directly, so `httptest` servers still work:

```sh
HTTP_PROXY=http://127.0.0.1:9 HTTPS_PROXY=http://127.0.0.1:9 go test -count=1 ./...
```

## Wire tests

Wire tests check what each public method sends and how it handles the reply, against the fakes of
the Permit API and the PDP in `internal/testkit/mockpermit`. `mockpermit.NewAPI(t)` and
`mockpermit.NewPDP(t)` each start a local server for the test to point the client at, and
`mockpermit.NewPDP(t, mockpermit.Cloud)` fakes the cloud PDP, which answers 404 with an empty body
on the routes that only the PDP container serves. The servers are strict. Each of these fails the
test: an `Authorization` header that is not exactly `Bearer` and `mockpermit.APIKey`, a path with
an empty or dot segment, a route the test did not register or a method it lacks, more requests to
a route than it has replies, and, when the test ends, a reply that no request used.

A test registers each route with its replies, calls the SDK, and checks the requests the server
recorded:

```go
api := mockpermit.NewAPI(t)
api.Handle("POST /v2/facts/{proj_id}/{env_id}/users",
	mockpermit.Reply{Status: http.StatusOK, Body: `{"key": "alice"}`})
// Call the SDK with a client whose API URL is api.URL.
api.Requests()[0].ExpectJSON(t, `{"key": "alice"}`)
```

`ExpectJSON` compares the body as JSON with numbers as written, so a rounded 2^53+1, or a null for
a missing field, fails. `ExpectBytes` compares the body byte for byte with a golden file in
`testdata/`, which holds the body and then one newline. `FailRequests` answers a route with an
error status until its stop function is called, and fails the test when no request reached it.
`AssertUntouched` checks that nothing reached the other server, and `Connections` counts the
connections a client opened.

`internal/testkit` also has `fixtures`, values that JSON encoders and decoders easily change
(2^53+1, the largest int64, -0, 1e300, U+2028 and U+2029, `<>&`, an escaped lone surrogate, a key
shaped like a UUID and a `type:key` identifier); `canary`, fake API keys and a scanner that fails
a test when one shows up in output; `leak`, which fails a test that leaves goroutines running;
`concurrent`, which runs a function on many goroutines at once and reads every result; and
`faketb`, a fake `testing.TB` for the tests of these helpers. Only tests may import
`internal/testkit`, and only tests and `internal/testkit` may import go-cmp; depguard rules in
`.golangci.yml` enforce both.

An Example has no `testing.T`, and pkg.go.dev shows all of its code. A package with Examples starts
the servers in its `TestMain` with `mockpermit.StartExample()`, registers the routes its Examples
use, and puts the servers' URLs in package variables, such as `exampleAPIURL` and
`examplePDPURL`, that the Examples pass to the client. `os.Exit(servers.Run(m))` then runs the
tests and exits 1 when a server found a problem. `internal/testkit/mockpermit/exampletest` shows
how. A package whose `TestMain` also writes the request record, as the root package's does,
runs its tests through both:
`os.Exit(mockpermit.RunRecorded(func() int { return servers.Run(m) }))`; see
[API coverage](#api-coverage).

## API reference

The SDK's API reference is its documentation on
[pkg.go.dev](https://pkg.go.dev/github.com/permitio/permit-golang/v2), which go/doc reads from
the doc comments in the source. Every package has a package comment, and every exported
identifier a doc comment. The tests in `doc_test.go` are the fail-closed form of what pkg.go.dev
needs, run on the `permit` and `models` packages. The linters godoclint and testableexamples cover
part of it: the doc comments of packages and top-level declarations, and Examples without an
output comment. The tests add doc links that resolve, the doc comments of struct fields and of
interface and promoted methods, Examples for the service types and the decision methods, the
phrases of `docRules`, and a count sentinel:

- `TestDocLinks`: every doc link resolves. A doc link is an identifier in square brackets, such as
  `[Client]`, `[Client.Users]`, `[*Client]`, `[models.User]` or `[context.Context]`, or a package,
  such as `[models]`, and pkg.go.dev shows it as a link. Bracketed text that has the shape of a
  doc link but names nothing, such as `[UserService]` for `UsersService`, stays plain text there;
  a qualified link to an identifier that its package does not have, such as `[models.Usr]`, is a
  broken link. Both fail with their file and line. A link names the package itself, an identifier
  of the package, of a package that the package imports (under one name), or of a standard library
  package with a one-element path, such as `[strings.Cut]`; link a package outside the module and
  the standard library with a URL. The test reads every doc comment of the source files, also
  those of unexported declarations, which gopls shows, and the doc comments of the Examples, which
  pkg.go.dev shows above their code; a link there names what the package declares or imports, also
  in the external test package.
- `TestDocComments`: every package has a package comment, and every exported identifier a doc
  comment: constants and variables (the comment of their group, their own or a line comment),
  types, functions, methods, also those that a struct promotes from an unexported type, the
  exported fields and embedded types of structs, and the methods and embedded elements of
  interfaces. Its count sentinel fails when it checked fewer identifiers than
  `testdata/api-surface.txt` lists declarations.
- `TestDocExamples`: every Example of the package has an output comment, since go test compiles
  an Example without one but never runs it. Every service type and every decision method has an
  Example with an output comment in the external test package, `package permit_test`, where it
  uses the SDK the way an application does. A service type is the type of an exported field of
  `Client`, such as `UsersService` for the field `Users *UsersService`, or any exported type
  whose name ends in `Service`. Its Example is `Example<Type>`, or `Example<Type>_<suffix>` with a
  suffix that starts with a lower-case letter; `Example<Type>_<Method>` is the Example of the
  method and does not count for the type. When `Client` is not a struct type, such as an alias,
  the test fails, as it cannot read the fields of `Client`. The decision methods are listed in
  `decisionMethods`, and each has an Example `Example<Type>_<Method>`. An Example that calls the
  API runs against the `mockpermit` servers that the package's `TestMain` starts; see
  [Wire tests](#wire-tests).
- `TestDocRules`: `docRules` derives the rules of each public method that a wire case binds to
  an operation from the operations of the [operation registry](#operation-registry) that its
  wire cases send, and the doc comment of the method holds each rule's phrase, ignoring case and
  line breaks: `environment-level API key`, `project-level API key` or
  `organization-level API key`, the widest `KeyLevel` of its operations; `container PDP only`
  when one of them is `ContainerOnly`; and `waits for sync` when one of them is `WaitsForSync`.
  A method that does not wait must not say `waits for sync`.

The fixture modules in `testdata/doccheck` prove the rules: `pass` passes them all, and `links`,
`undocumented`, `examples` and `notstruct` break them. `TestDocRulesFromTheRegistry` and
`TestDocRulesFromTheRegistryThatFail` bind the methods of `pass` to test-only operations.

`readme_test.go` compiles the Go programs of `README.md`. `TestReadmeBlocksCompile` builds each
fenced go block whose opening fence is on the line right after a `<!-- compile -->` line as the
main package of a temporary module that requires the SDK and replaces it with the repository
root, offline. A marked block is a complete program, `package main` with its imports, and a
compile error names its line in the Markdown file. A marker that no go block follows fails, and
so does a run that built fewer or more blocks than there are markers; a built block is one that
go build ran on, whether it compiles or not. Mark every block that shows a whole program:

````markdown
<!-- compile -->
```go
package main

import (
	"fmt"

	"github.com/permitio/permit-golang/v2"
)

func main() {
	fmt.Println(permit.Version)
}
```
````

`compileFiles` lists the Markdown files that the test reads; the fixtures in `testdata/readme`
prove its rules.

The `docs` job in [CI](#ci) runs these tests, `go vet` on each package outside `internal/`,
whose `tests` analyzer reports an Example whose name names no identifier, and `go doc -all` on
each of them. Run the same checks from the repository root, on the packages that CI selects:

```sh
pkgs=$(go list ./... | grep -Ev '/internal(/|$)')
go vet $pkgs
for pkg in $pkgs; do go doc -all "$pkg" >/dev/null; done
HTTP_PROXY=http://127.0.0.1:9 HTTPS_PROXY=http://127.0.0.1:9 \
  go test -count=1 -run '^Test(Doc|Readme)' .
```

## Public surface and wire cases

`testdata/api-surface.txt` is a snapshot of the exported API of the `permit` and `models`
packages: one declaration per line, sorted, with parameter names, as `internal/surface` reads it
from the source. `TestSurfaceSnapshot` fails and shows the difference when the source and the
snapshot differ. When you change the API on purpose, rewrite the snapshot and review its diff
with the rest of your change:

```sh
go test -run TestSurfaceSnapshot -update .
```

`TestNoInternalTypesInSignatures` fails when an exported declaration mentions a type of a package
under `internal/`, which applications cannot import.

Every public method, a method of `Client` or of a type whose name ends in `Service`, needs a wire
case: a `wireCase` (`wirecases_test.go`) returned by the `<area>Cases()` function of its
`<area>_wire_test.go` file, which `wireCaseLists` lists. A case names the method, such as
`UsersService.Get`, the operationId of its operation in the API spec, the route as
`mockpermit.Server.Handle` takes it, the modes it runs in, and the call of the method with the
client it is given. It carries the request the method must send, the reply and the result the
method must return for it (`wireNoResult{}` for a method that returns only an error), and an error
reply of the API, with its status and error body, and the error the method must return for it.

`TestWireCases` runs each case in each of its modes against new `mockpermit` servers: `api` sends
the request to the API, `pdp` to the PDP, and `facts-via-pdp` to the PDP container with
`WithFactsViaPDP`, where the runner expects the route, the path and the route of the error under
`/facts`, without `/v2/facts/{proj_id}/{env_id}`. The method must send the case's request to the
server of the mode and nothing to the other server, return the result and the error of the case,
and send no `X-Wait-Timeout` or `X-Timeout-Policy`. When the route fills `{proj_id}` or `{env_id}`,
the client has `WithScope("p", "e")`, so a path such as `/v2/facts/p/e/users` needs no lookup of
the scope. The case of an operation with `Facts` lists `api` and `facts-via-pdp`, and no other
case lists `facts-via-pdp`. The tests below check the cases too:

- the registry rejects a case whose method is not `<Type>.<Method>` with exported names, or that
  has no operationId, a malformed route or one with an empty segment, no mode, an unknown or
  repeated mode, a request path that does not start with `/`, no result, an error reply without a
  status from 400 to 599 and a body, or no error;
- `TestEveryPublicMethodHasAWireCase` fails when a public method has no case, a case names no
  public method, `Client` or a type whose name ends in `Service` embeds a type of another package
  or `error`, whose methods `internal/surface` cannot list (declare the methods on the type
  instead), or the counts do not add up: public methods = methods with a case + exempt methods;
- `TestRouteParamOrder` fails unless the placeholders of each case's route name the method's
  parameters in the route's order. A placeholder names the parameter with the same stem:
  `{resource_id}` names `resourceKey` or `resourceID`. `{proj_id}` and `{env_id}` name
  `projectKey` and `environmentKey` when the method takes them, and the client fills them in from
  its scope when it does not.

A public method that sends no request, such as one that returns a copy of its service, needs no
wire case: list it in `testdata/no-wire.txt` as `<Type>.<Method>`, a space and the reason. A line
without a reason, or one that names no public method or a method that has a wire case, fails the
test.

`go test ./...` runs these tests with the others. To run the four gates alone:

```sh
go test -run '^Test(SurfaceSnapshot|NoInternalTypesInSignatures|EveryPublicMethodHasAWireCase|RouteParamOrder)$' .
```

## Public surface and migration

`tools/cmd/apisurface` checks that [MIGRATION.md](MIGRATION.md) covers every part of the v1 API,
and that a breaking change to the v2 API is labelled as one. The `api-surface` job runs it on
every pull request: in migration mode while the base of the pull request names another module
path than the pull request, as the pull request that moves the SDK to v2 does, and in diff mode
otherwise. It exits 0 when the check passes, 1 when it fails and 2 when it did not run, and
`go -C tools doc ./cmd/apisurface` lists every rule. Build it, and the apidiff that `tools/go.mod`
pins, from the repository root:

```sh
go -C tools build -o ../bin/apisurface ./cmd/apisurface
go -C tools build -o ../bin/apidiff golang.org/x/exp/cmd/apidiff
bin/apisurface -mode migration                                 # the migration check
bin/apisurface -mode migration -ids-base origin/master         # and no entry ID removed since
bin/apisurface -mode pr -base origin/master -apidiff bin/apidiff  # what the job runs
```

### The v1 surface

`tools/migration/v1-surface.txt` lists the parts of the v1 API that the migration check counts,
read from `github.com/permitio/permit-golang@v1.3.0`, the release that `MIGRATION.md` tells v1
users to pin:

- a: the 352 exported identifiers of `pkg/api`, `pkg/config`, `pkg/enforcement`, `pkg/errors` and
  `pkg/permit`, such as `api.Users.Create`;
- b: 86 operations of the generated client in `pkg/openapi`, such as
  `openapi.MembersApiService.ListOrganizationMembers`: 56 whose route the API spec has and that
  only the generated client sends, and 30 whose route the spec does not have;
- c: the 384 types of `pkg/models`, such as `models.UserRead`.

apisurface writes the file from the module zip that the Go module proxy serves, after the
checksum database verified it; do not edit it by hand. `TestRepositoryV1Surface` checks its
counts. To write it again, which needs the network:

```sh
bin/apisurface -module github.com/permitio/permit-golang@v1.3.0
```

### Migration entries

Each change from v1 is an entry of `MIGRATION.md`, in the section of its bucket:
`## Compatibility`, `## API` or `## Wire behaviour`. An entry starts with an anchor and a heading
with its ID, followed by its fields as list items:

```markdown
<a id="M-users-1"></a>
### M-users-1: Users.Create moves to UsersService.Create

- v1: `api.Users.Create`
- v2: `permit.UsersService.Create`
- Bug fix: could never work before

What changed, and what to do.
```

- The ID is `M-<area>-<n>`: the area in lower case, such as `users`, and the next number in that
  area. Add it at the end of `tools/migration/ids.txt`, which lists every entry ID. An ID is never
  removed, renumbered or reused, so that a link to an entry keeps working: the check fails when a
  commit removes an ID from `ids.txt`.
- A `v1:` line names what changes in v1. When it starts with a backquote, its first term in
  backquotes is an id of `v1-surface.txt` or a `path.Match` pattern of ids, such as
  `api.Users.*`, and it must match at least one. Otherwise it is prose, such as the module path.
- The one `v2:` line names the v2 declaration that takes its place, in backquotes, optionally
  after `replaced by `, such as `permit.UsersService.Create`; the check fails when v2 does not
  export it. When v2 has nothing in its place, the `v2:` line of an entry that names v1
  identifiers is one of these reasons, exactly:
  - `not exposed in v2`
  - `not exposed in v2: account administration`
  - `not exposed in v2: dashboard or CLI helper`
  - `not exposed in v2: Elements back-end`
  - `not exposed in v2: operator route`
  - `not exposed in v2: deprecated upstream`
  - `not exposed in v2: EAP`

  The vocabulary applies to these removals only. A `v2:` line that is prose, in an entry that is
  not a removal, such as a compatibility or wire behaviour entry, is free text, and so is the
  text after a declaration in backquotes: reviewers check it, and apisurface checks only that it
  holds none of the words of the next item.
- `MIGRATION.md` is public, so an entry must not describe a weakness of the API: the check fails
  on a line of an entry, from its heading to the next anchor or heading, that contains `secret`,
  `exposes` or `vulnerab`, in any case.
- `- Bug fix: could never work before` marks a change to a v1 feature that could never work as
  documented.

### Pending lines

Until the entry for a part of the v1 API exists, a line of `tools/migration/pending.txt` assigns
it to the area of v2 that will write the entry: an id or a pattern, then the area, such as
`api.Users.* users`. Each line of `v1-surface.txt` must be in exactly one entry or one pending
line. When you write an entry, remove what it covers from `pending.txt` in the same commit, and
split a pattern when the entry covers only part of it. A pending line that matches nothing fails.
A models type of v1 needs no entry once v2's `models` package exports a type of the same name:
remove a pending line that names it, while a pattern such as `models.*` may keep matching it
along with types that are still pending. A pending line that matches only such types fails.
Release mode fails while `pending.txt` has a line that is not blank or a comment, since the
release needs every v1 item to have its entry.

The appendix at the end of `MIGRATION.md` maps each models type of v1 to the v2 type or the entry
that replaces it. apisurface writes it, and the check fails when it is out of date: after you
change entries, pending lines or v2's models, run `bin/apisurface -mode migration -write`.

### Breaking changes

Diff mode compares a pull request with its base, once both are v2, and counts as breaking:

- each incompatible change that apidiff reports for the exported API of the module's packages, but
  the ones under `internal/`;
- each new rule ID in `testdata/validation-rules.txt`, since the SDK then rejects input that it
  accepted before. `internal/validate` holds the rules, and `TestValidationRules` fails when the
  file differs from them: after you add or change a rule, run
  `go test -run TestValidationRules -update .`;
- each field that `tools/drift/model-map.yaml` marks `required: true` in a models type that the
  base's map has, where the base's map had the field not required or did not have it; a models
  type that is new in the map breaks nothing; see [Model drift](#model-drift).
  apisurface reads only the map's `models` key: each models type by its v2 name, with `fields`,
  each field with a boolean `required`, such as `models: {User: {fields: {key: {required: true}}}}`.
  It ignores the other keys, and exits 2 when the map lists no models type.

A breaking change fails `api-surface` unless the pull request has the `breaking` label. When the
change is intended, add the label, describe the change in `CHANGELOG.md`, and run the job again:
it reads the labels when it runs. Diff mode also fails when a commit of the pull request removes
an entry ID from `ids.txt`.

Release mode checks the version of a release with gorelease, which `tools/go.mod` also pins,
against the newest earlier tag of the same major version: with `-base=none` for the first one,
allowing incompatible changes after a prerelease, and otherwise failing on an incompatible change
or a version that does not follow from the changes. It also fails while `pending.txt` is not
empty, as [Pending lines](#pending-lines) says. The release workflow *(added in a later commit on
this branch)* runs it:

```sh
go -C tools build -o ../bin/gorelease golang.org/x/exp/cmd/gorelease
bin/apisurface -mode release -version v2.1.0 -gorelease bin/gorelease
```

## Checks

`tools/` is a separate Go module with the programs that check the SDK;
[tools/README.md](tools/README.md) describes them. Run the go commands above in it too, with
`go -C tools`, for example `go -C tools test -race -count=1 -shuffle=on ./...`.

`go test` passes a run in which tests skip or no test runs at all. checkgotest does not: run the
tests of both modules with `-json` and check their output together.

```sh
go -C tools build -o ../bin/checkgotest ./cmd/checkgotest
go test -race -count=1 -shuffle=on -json ./... >bin/root.json
go -C tools test -race -count=1 -shuffle=on -json ./... >bin/tools.json
bin/checkgotest -require-file .github/required-tests.txt \
  -allow-skips .github/allowed-skips.txt bin/root.json bin/tools.json
```

It exits 0 when every test passed; 1 when a test failed or skipped, or an entry in
`.github/allowed-skips.txt` expired or its test did not skip; and 2 when no test passed, no test
ran in one of the files, a test that `.github/required-tests.txt` lists did not run, or the input
is not `go test -json` output. A test may skip only with an entry in `.github/allowed-skips.txt`
that names its ticket, the last day the skip is allowed and what must happen before the test can
run again.

## Consumer checks and the module zip

These checks look at the SDK as an application gets it.

### The consumer module

`consumer/` is a Go module of its own, `example.com/consumer`, that imports the SDK's packages the
way an application does: its `go.mod` requires the SDK and replaces it with the repository root,
and its path is outside the SDK's. Its tests, in `consumer/main_test.go`, check that:

- `go list -m all` lists exactly the consumer, the SDK and go-cmp. go-cmp is the SDK's one
  requirement; only the SDK's tests use it, and module graph pruning still lists it. Every
  requirement in the root `go.mod`, a tool or a test helper too, reaches the module graph of every
  application that imports the SDK, so the test fails when one is added, until `wantModules`
  lists it on purpose. Tools belong in `tools/go.mod`.
- `go doc` resolves the SDK's packages from the consumer, and their documentation holds
  `wantDeprecated` deprecation notices: paragraphs that start with `Deprecated: `, in the
  documentation of a function, type or method, or in the comment on a struct field or on a spec
  of a grouped const or var declaration. v2.0.0 has none. Change the count only together with a
  deprecation.
- each fixture in `consumer/mustfail/` fails to compile with the message it names. A fixture is
  code that an application must not be able to write, such as an import of a package under
  `internal/`. The test compiles each one as the main package of a temporary module with the
  consumer's `go.mod` and `go.sum`, offline. A fixture that compiles fails the test, and so does a
  file in `consumer/mustfail/` that is not a fixture: the test must compile as many fixtures as
  the directory holds files.

golangci-all.sh lints `consumer/` like every module, with `.golangci.yml`, whose staticcheck
reports the use of a deprecated identifier (SA1019). Run the checks from the repository root; the
tests run offline once every module of the module graph is downloaded:

```sh
go -C consumer mod download all
go -C consumer vet ./...
HTTP_PROXY=http://127.0.0.1:9 HTTPS_PROXY=http://127.0.0.1:9 go -C consumer test -count=1 ./...
go -C consumer mod tidy -diff
```

To add a must-fail fixture, write `consumer/mustfail/<name>.go.txt`: a first line
`// want: <part of the compiler's message>`, then the rest of a Go file of package `main` that
uses the SDK the way it must not be used. Run the tests, and check that the fixture fails for the
reason you meant. The `.go.txt` extension keeps the go command and the linters from reading the
file.

The consumer job in CI names the tests it requires with `-require` in `.github/workflows/ci.yml`,
not in `.github/required-tests.txt`, so a new test in `consumer/` needs a `-require` line there.

### The module zip

zipcheck (`tools/cmd/zipcheck`) builds the module zip of the commit that `HEAD` names, the way
the Go module proxy builds it for a version tagged on that commit: from the committed files, with
`git archive`, through `golang.org/x/mod/zip`, which leaves out the files of nested modules such as
`tools/` and `consumer/`, and symbolic links. It reads committed files only, so commit a change
before you check it. It checks the whole repository from any directory in it. It fails when:

- a file has a name that a module zip cannot hold, such as `testdata/aux.json` (Windows reserves
  `aux`) or a name with `:`; the module proxy cannot serve such a commit;
- a file is not on the allow list: Go source files, files in `testdata` directories, `go.mod`,
  `go.sum`, `LICENSE`, `README.md`, `MIGRATION.md`, `CHANGELOG.md` and `llms.txt` at the root,
  `AGENTS.md`, `CODE_OF_CONDUCT.md`, `CONTRIBUTING.md` and `SECURITY.md`, the configuration files
  at the root (dotfiles and `_typos.toml`), and the files under `.github`;
- a file is an image or an API specification, or is under `tools/`, `consumer/` or `skills/`,
  which must stay out of the zip: a `go.mod` of their own, or `export-ignore` in
  `.gitattributes`, keeps them out;
- a file outside `testdata` directories is not text: valid UTF-8 without a NUL byte;
- `LICENSE` is missing: pkg.go.dev shows the documentation of a module only when it detects a
  license it allows;
- a file is larger than 256 KiB uncompressed, unless it has an exception, or an exception names a
  file that is not in the zip;
- the zip, compressed, is larger than the budget.

It prints the module's path, its number of files, the zip's size and the budget, and the 10
largest files with their sizes uncompressed and compressed, on every run that builds the zip. It
exits 0 when every rule holds, 1 when one does not, and 2 when it could not check, such as when
`HEAD` has no `go.mod` or the limits are not valid. Run it from the repository root:

```sh
go -C tools build -o ../bin/zipcheck ./cmd/zipcheck
bin/zipcheck
```

The limits are in `tools/cmd/zipcheck/limits.go`. Raise one only when the growth is wanted, in the
change that needs it, and say why next to it:

- A file that must be larger than 256 KiB, such as a generated snapshot, gets an entry in
  `exceptions` with its path, its own cap in bytes and the reason. An exception needs a cap above
  256 KiB, and fails once its file is gone.
- The budget is the zip's compressed size measured on a given day, `baseline` and `measured`, plus
  `headroom`. To raise it, measure the zip of your change with zipcheck, write its size and the
  day, choose the headroom, and say in `reason` what the zip grew by and why.

`TestRepositoryLicense` in `tools/license_test.go` checks that `LICENSE` is the MIT License word
for word, with the copyright line `Copyright (c) 2023-2026 Permit.io, Ltd.`: pkg.go.dev detects a
license by its text and shows a module's documentation only when it detects a license it allows,
so a changed clause can hide the SDK's documentation.

## CI

`.github/workflows/ci.yml` runs every check. Its last job, `CI`, is the one check the ruleset
requires: it fails unless every job it needs succeeded, or was not selected for the run.

| Job | What it does |
| --- | --- |
| `prek` | Checks the workflows with cicheck, runs every git hook on every file through `.github/scripts/prek-count.sh`, and on a pull request checks the message of each of its commits through `.github/scripts/commits-check.sh`. |
| `test` | Runs the tests of both modules and the [end-to-end dry run](#end-to-end-tests) with the race detector, offline, and checks their output with checkgotest. Five legs: Go 1.26.0, the minimum, on the dependency versions that `go.mod` and `go.sum` pin; and the newest patch of Go 1.26 and of Go 1.27 that is at least 7 days old, on those versions and on the newest version of every dependency. Each leg checks that it runs the Go version it names. Then the benchmarks of the root package run 100 times each, so that each compiles and runs. |
| `crossbuild` | Builds the SDK for linux, darwin and windows on amd64 and arm64. |
| `consumer` | Vets and tests `consumer/`, offline, and checks the test output with checkgotest; see [Consumer checks and the module zip](#consumer-checks-and-the-module-zip). |
| `docs` | Checks the API reference, offline: `go vet` and `go doc -all` on each package outside `internal/`, and the tests of `doc_test.go` and `readme_test.go`, whose output checkgotest checks; see [API reference](#api-reference). Then builds and runs the zapslog example of `WithLogger`, which needs the module proxy; see [Logging](#logging). |
| `zipcheck` | Builds the module zip of the commit and checks what it holds with zipcheck, offline; see [Consumer checks and the module zip](#consumer-checks-and-the-module-zip). |
| `api-coverage` | Runs the root package's tests offline with the request record on, and checks the record and `tools/specs/coverage.yaml` with apicoverage, which writes its report to the job summary; see [API coverage](#api-coverage). |
| `vuln` | Scans the SDK and `tools/` for known vulnerabilities through `.github/scripts/vuln.sh`; see [Vulnerability scan](#vulnerability-scan). |
| `secrets` | Scans the commits that are new in the run for secrets with gitleaks, through `.github/scripts/gitleaks-range.sh`; see [Secret scan and dependency review](#secret-scan-and-dependency-review). |
| `mutation` | Pull requests only: mutation tests of the Go lines the pull request changed, in both modules, as [Mutation tests](#mutation-tests) describes. Fails when the tests detect fewer than 90% of the mutants that ran. |
| `dependency-review` | Pull requests only: fails when the pull request adds a dependency with a known vulnerability of high or critical severity; see [Secret scan and dependency review](#secret-scan-and-dependency-review). |
| `api-surface` | Pull requests only: checks that `MIGRATION.md` covers the v1 API while the base is v1, and otherwise fails on a breaking change to the API without the `breaking` label; see [Public surface and migration](#public-surface-and-migration). |
| `prek-upstream` | Weekly: runs every hook with prek's built-in versions of the pre-commit-hooks hooks and with the upstream hooks (`PREK_NO_FAST_PATH=1`), and fails when they report a hook differently. |
| `mutation (weekly)` | Weekly: mutation tests of every Go file of both modules, with no threshold; it reports the efficacy in the job summary, posts each module's efficacy and count of mutants that lived to Slack, and fails only when the run does not complete or its result cannot be posted. |
| `security-scan` | Twice a week: the scheduled security scan, posted to Slack; see [Scheduled scans and alerts](#scheduled-scans-and-alerts). |
| `digest` | Weekly: the security digest, posted to Slack; see [Scheduled scans and alerts](#scheduled-scans-and-alerts). |
| `spec-drift` | Weekly: compares the published specs and the container PDP's latest image with the snapshots in `tools/specs`, checks the models against the published specs, and posts the result to Slack; it also reports the API coverage against the published specs. See [Spec snapshots](#spec-snapshots) and [Model drift](#model-drift). |
| `bench` | On a push to `master` and weekly: runs the benchmarks of the decision calls, compares them with the previous result on `master`, writes the report to the job summary and posts a regression, or a run that did not complete, to Slack. Advisory: it never fails CI. See [Benchmarks](#benchmarks). |
| `e2e` | On a push to `master`, in the mode `e2e` and in a release: the end-to-end scenarios against Permit in a scratch environment, with the container PDP pinned by digest. See [Against Permit](#against-permit). |
| `CI` | Decides the result with `.github/scripts/ci-aggregate.sh`. |
| `notify-gate-failure` | After `CI`, when it failed on a push to `master` or in a release: posts one line to Slack. `CI` does not need it. |

Which jobs a run selects depends on its mode:

- A pull request into any branch and a push to `master` run the pull request checks: `prek`,
  `test`, `crossbuild`, `consumer`, `docs`, `zipcheck`, `api-coverage`, `vuln` and `secrets`. A
  pull request runs `mutation`, `dependency-review` and `api-surface` too, and a push to `master`
  runs `bench` and `e2e`.
- A manual run (`workflow_dispatch`) takes a mode: `pr` runs the pull request checks, `weekly` the
  weekly jobs, `security` the security scan, and `e2e` the end-to-end jobs. The release workflow
  *(added in a later commit on this branch)* calls CI with the mode `release`, which runs the
  pull request checks and the end-to-end jobs.
- `.github/workflows/e2e-pr.yml`, the pull request lane of the end-to-end tests, calls CI with
  the mode `e2e` on a pull request from a branch of this repository; see
  [Against Permit](#against-permit). That run's `CI` decides the lane, and is not the check the
  ruleset requires; a pull request into `master` needs the lane's deployment to `integration`
  instead.
- Two crons start the periodic modes: `SECURITY_CRON` twice a week and `WEEKLY_CRON` once a week.
  Each cron is listed in `on.schedule` and set once more in the workflow's `env`, as
  `<MODE>_CRON`, which cicheck and ci-aggregate.sh read.

The `CI` job fails when a job it needs failed or was cancelled, or was skipped in a run that
selects it, and passes a job that the run does not select. Four lists in its step change that:
`PULL_REQUEST_JOBS` run on pull requests only, each job in `SCHEDULED_JOBS` (`job:mode`) is
required in its mode only (an advisory one, such as `bench`, may also run on a push to `master`),
`E2E_JOBS` run on a push to `master` and in the modes `e2e` and `release` only, and a job in
`ADVISORY_JOBS` may fail without failing CI. ci-aggregate.sh exits 0
when CI passes, 1 when a job did not succeed, and 2 when it could not decide: the job results
cannot be read, there are not `EXPECTED_JOBS` of them, a list names a job CI does not need, or
the mode, event or cron is unknown.

When you add a job, add it to `CI`'s `needs` and change `EXPECTED_JOBS`; when it runs only on
pull requests, on a cron, as an end-to-end job or as an advisory check, add it to the list for
that. Give the job the
`if:` of its kind, exactly as `ci.yml` writes it:

- a pull request check, advisory or not:
  `(!inputs.mode && (github.event_name == 'pull_request' || github.event_name == 'push')) || inputs.mode == 'pr' || inputs.mode == 'release'`;
- a job in `PULL_REQUEST_JOBS`: `!inputs.mode && github.event_name == 'pull_request'`;
- a job in `SCHEDULED_JOBS`:
  `(!inputs.mode && github.event.schedule == '<cron>') || inputs.mode == '<mode>'`, with the cron
  its `<MODE>_CRON` declares. A run that another workflow calls sees its caller's schedule, so
  `!inputs.mode` keeps the cron from selecting the job in a run that has a mode;
- a job in `SCHEDULED_JOBS` that runs on a push to `master` too, as `bench` does:
  `(!inputs.mode && (github.event_name == 'push' || github.event.schedule == '<cron>')) || inputs.mode == '<mode>'`.
  Only a job in `ADVISORY_JOBS` may: no pull request runs it before the push, so its failure must
  not fail CI on `master`;
- a job in `E2E_JOBS`:
  `(!inputs.mode && github.event_name == 'push') || inputs.mode == 'e2e' || inputs.mode == 'release'`.
  A job of `ci.yml` in the `integration` environment must be in `E2E_JOBS`, so that no pull
  request of `ci.yml`, which may come from a fork, runs it.

A job that needs `CI` runs after it, so `CI` does not need it; only `notify-gate-failure` does,
and its `if:` is
`failure() && ((!inputs.mode && github.event_name == 'push') || inputs.mode == 'release')`. A
scheduled job posts its own result, and a failed scheduled run is red in Actions.

When you add or remove a git hook, change `EXPECTED_HOOKS`.

cicheck checks these rules and the workflows' hardening: every action pinned to a commit SHA with
a version comment, `persist-credentials: false`, `cache: false` and an exact Go version on every
`setup-go`, a pinned runner and a timeout on every job, no `${{` in a `run:`, the secrets of the
end-to-end jobs read only in the `integration` environment, an end-to-end job that checks
`tools/e2e/required.txt`, deletes its environment under `if: always()` and, when required,
starts a PDP pinned by digest, `e2e-pr.yml` as the one workflow outside CI that runs on pull
requests, only for those from a branch of this repository and in the mode `e2e`, a call of a
workflow that grants every permission the called jobs ask for, and the rest that
`go -C tools doc ./cmd/cicheck` lists. Tool output must not be able to issue workflow commands,
so a `run:` that does more than call scripts in `.github/scripts` starts with these three lines,
after any blank and comment lines, with one variable name in all three:

```sh
token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
echo "::stop-commands::$token"
trap 'echo "::$token::"' EXIT
```

A line that calls a script counts as a script call only with plain or quoted arguments: a `&`,
`|`, `;`, `<`, `>`, backtick or `$(` after the script's path makes it an ordinary command. A
script that CI calls prints a `::stop-commands::` line with its token in a variable.

cicheck exits 0 when every rule holds, 1 when one does not, and 2 when it could not check. Run it
from the repository root:

```sh
go -C tools build -o ../bin/cicheck ./cmd/cicheck
bin/cicheck              # checks .github/workflows and .github/scripts
bin/cicheck -print-pins  # every pinned action and release binary, with its version
```

The CI scripts run locally too. Each says in its header what it reads and what its exit codes
mean, and the tests in `tools/scripts` run them; those tests need bash, git, jq, diff and od, and
the gitleaks that `.github/scripts/fetch-tool.sh gitleaks` installs as `bin/gitleaks`.

```sh
EXPECTED_HOOKS=26 .github/scripts/prek-count.sh  # the EXPECTED_HOOKS of ci.yml's env
.github/scripts/commits-check.sh "$(git merge-base origin/master HEAD)" "$(git rev-parse HEAD)"
```

## Mutation tests

Mutation tests change the code, one small change (a mutant) at a time, and run its package's
tests on each: a mutant the tests do not detect shows code they do not check. Run them only
through `tools/cmd/mutationgate`, with the limits below, and never run gremlins or another
mutation tool directly: a mutant can allocate memory or run without end, and nothing else bounds
it. mutationgate runs [gremlins](https://gremlins.dev), the version that `tools/binaries.lock`
pins, and:

- bounds every test process to `-cap` (3 minutes), `-memory-mib` (4096 MiB; the kernel enforces
  it on Linux, and mutationgate polls on macOS) and 64 MiB of output, and runs `-workers` (2) at a
  time; before it starts, a probe that allocates without end must be stopped by the memory limit;
- runs gremlins once per package, so a mutant is tested with its own package's tests, and leaves
  mutants that do not compile out of the count;
- with `-base <ref>`, mutates the lines changed since the merge base with `<ref>`, and with
  `-all` every Go file, of the packages in the directories it is given;
- mutates only the code in function bodies, the only code Go's coverage counts, so a change of
  only comments, blank lines, the package clause, imports or declarations outside function
  bodies, such as a package-level variable's value, passes with nothing to mutate. In a function
  body, coverage does not count a case clause up to its colon, or the rest of a statement after
  the body of a function literal in it (up to the `{` of the statement's own body), either, so
  the gate does not mutate a line that holds only such code;
- with `-max-mutants`, tests at most that many mutants, or the mutants of the sample's first line
  when that line alone holds more: a sample of the changed lines, fixed by the commit `HEAD`
  names, spread over the files first. The report states the share of the mutants, lines and
  files that the sample covers;
- skips the tests that `-skip` matches; and
- exits 0 when the tests detect at least `-threshold` percent (90) of the mutants that ran, 1 when
  they detect fewer, and 2 when the run did not complete: the limits do not hold, gremlins failed,
  or Go lines changed and no mutant on them ran. With `-base`, nothing to mutate passes; with
  `-all`, a module with nothing to mutate is a notice, and a run in which no mutant ran is DID
  NOT RUN.

gremlins tests each mutant in a copy of the package's module, without the rest of the
repository. A test that reads a file outside its module, such as the workflows, fails there for
every mutant, which would then count as detected; mutationgate fails a run in which a package's
tests fail in such a copy. Name such a test `TestRepository...`, so that the `-skip` below leaves
it out. Live tests that need Permit are named `TestLive...` and are left out too.

Install gremlins and build mutationgate from the repository root:

```sh
.github/scripts/fetch-tool.sh gremlins
go -C tools build -o ../bin/mutationgate ./cmd/mutationgate
```

Then run it there, offline, on the changes of your branch, and on a whole package when you check
its tests:

```sh
export HTTP_PROXY=http://127.0.0.1:9 HTTPS_PROXY=http://127.0.0.1:9
bin/mutationgate -gremlins bin/gremlins -workers 2 -memory-mib 4096 -cap 3m \
  -timeout-coefficient 5 -skip '^(TestLive|TestRepository)' -base origin/master . tools
bin/mutationgate -gremlins bin/gremlins -workers 2 -memory-mib 4096 -cap 3m \
  -timeout-coefficient 5 -skip '^(TestLive|TestRepository)' -all tools/cmd/cicheck  # a package
```

Give a lower `-memory-mib` when a memory watchdog on your machine would stop a process first.
`bin/mutationgate -h` lists the flags, and `go -C tools doc ./cmd/mutationgate` describes the
gate and its exit codes.

In CI, the `mutation` job runs the same command on a pull request with `-max-mutants 400`. Its
range starts at the newest commit of the pull request whose `mutation` check succeeded and that
the head still contains, or at the merge base when there is none, as
`.github/scripts/mutation-range.sh` works it out; so the lines of a run that a newer push
cancelled are tested in the next one. Changed lines in `tools/`, the programs that check the
SDK, are tested like SDK code. The gate builds without the tag `e2e`, so it mutates the
packages under `tools/e2e/framework` and `tools/e2e/fake` but not the scenarios of `tools/e2e`,
which the end-to-end dry run and the `e2e` job exercise instead. Maintainers push a pull
request's next commit only after CI passed on the previous one, so that each commit's lines get
a run. The weekly job tests every Go file of both modules and only reports.

## Benchmarks

`decisions_bench_test.go` holds the benchmarks of the decision calls, the SDK's hot path:
`Client.Check`, `Client.BulkCheck` of 10 and of 100 checks, and `Client.FilterObjects` of 100
resources. Each client sends its requests through an in-memory `http.RoundTripper` that answers
as the PDP does, with no network, and logs at Debug, through the client's redacting handler, to a
handler that writes to `io.Discard`. So `ns/op` and `allocs/op` count what the client adds to a
call: encoding the checks, the retry wrapper, the Debug line of each attempt and decoding the
decisions. A new benchmark calls `b.ReportAllocs()`. Run them from the repository root, offline,
and compare two results with benchreport (`tools/cmd/benchreport`), such as those of `master` and
of your branch on one machine. Building benchreport first creates `bin/`, which holds the results:

```sh
go -C tools build -o ../bin/benchreport ./cmd/benchreport
git switch master
HTTP_PROXY=http://127.0.0.1:9 HTTPS_PROXY=http://127.0.0.1:9 \
  go test -run '^$' -bench . -count 6 . | tee bin/old.txt
git switch -
HTTP_PROXY=http://127.0.0.1:9 HTTPS_PROXY=http://127.0.0.1:9 \
  go test -run '^$' -bench . -count 6 . | tee bin/new.txt
bin/benchreport bin/old.txt bin/new.txt
```

The `test` job runs each benchmark 100 times (`-benchtime 100x`), so that each compiles and runs.
The `bench` job runs them with `-count 6` on a push to `master` and weekly, and compares the
output with the previous result through benchreport. Once benchreport accepted the output, the
job keeps it as the artifact `bench-<Go version>`, such as `bench-go1.27.1`; the previous result
is the artifact of the same name of the newest earlier run of this repository's `master` that
has one.

benchreport reads both files with benchstat's parser, `golang.org/x/perf/benchfmt`, and prints a
row for each benchmark and unit: the median of the previous runs, the median of the current runs
and the change. A change marked `over 20%` is a regression, a median more than 20% higher in
`ns/op` or `allocs/op`; a benchmark that the previous result does not have shows
`no previous result`. The last line is the verdict:

- `PASS`: no benchmark regressed;
- `REGRESSION`: each regression, as `<benchmark> <unit> <change> (<previous> -> <current>)`;
- `NO PREVIOUS RESULT`: the previous file does not exist, or has none of the benchmarks;
- `DID NOT RUN`: a file cannot be read, holds no result or a line that is not valid benchmark
  output, or a benchmark without `ns/op` or `allocs/op`.

It exits 0, 1 on a regression and 2 when it did not run. The `bench` job writes the report to its
job summary and posts a regression, or a report that did not complete, to Slack; a missing
previous result passes. The job is in `ADVISORY_JOBS`, so it never fails CI. `ns/op` varies with
the runner that a run gets, so a regression in `ns/op` alone can be noise: run the job again
before you look for a cause. `allocs/op` hardly varies within one Go version; as the artifact's
name holds the version, the first run on a new Go version has no previous result.

## Vulnerability scan

The `vuln` job runs `.github/scripts/vuln.sh`, which scans three trees:

- the floor: the root module with the dependency versions `go.mod` requires, the lowest an
  application can build the SDK with;
- the ceiling: a copy of the root module in which `go get -u -t ./...` and `go mod tidy` moved
  every dependency to its newest version;
- `tools/`: the programs that build and check the SDK, and the ones the `tool` directives of
  `tools/go.mod` pin, such as govulncheck.

Two scanners look at each tree:

- govulncheck, the version `tools/go.mod` pins, through vulnscan (`tools/cmd/vulnscan`). It fails
  on every vulnerability in a module the build uses, whether or not the code calls the vulnerable
  part, unless `.github/vulnscan-accept.txt` accepts it. A vulnerability in the standard library
  never fails: vulnscan reports it with the Go release that fixes it.
- Trivy, the release `tools/binaries.lock` pins, through `.github/scripts/trivy.sh`. It reads
  `go.mod`, so it also sees a module that `go.mod` requires but no package imports. A
  vulnerability of HIGH or CRITICAL severity that has a fixed version fails the floor and the
  ceiling; in `tools/` it is reported only.

secreport (`tools/cmd/secreport`) decides the job's result from the scans and writes the job
summary. On a pull request from a branch of this repository, it posts a comment when a scan fails
or does not run, and updates that comment on every later run. When every scan passes and there is
no comment to update, it posts none, so findings that are reported only appear in the job summary
only: vulnerabilities in the standard library, Trivy's findings in `tools/`, and Trivy's findings
below HIGH or without a fixed version.

Each scan exits 0 when it passes, 1 when it finds a vulnerability that fails it, and 2 when it did
not run: a scanner failed, its output was incomplete, govulncheck's module list lacks a module that
the scanned packages use (`go list -deps`), or Trivy listed a different number of packages than
`go.mod` has require directives, not counting the ones a replace directive points at a local
directory. The job fails on 1 and on 2. Run the whole scan from the repository root; it needs `jq`,
and the network to download Trivy, the vulnerability databases and, for the ceiling, the newest
version of each dependency:

```sh
.github/scripts/vuln.sh
```

To accept a vulnerability the SDK is not exposed to, add a line to `.github/vulnscan-accept.txt`
with its ID, its module, the reason and the last day the entry applies; the file's header
describes the format. Called code and the standard library cannot be accepted, an entry fails
after its last day, and an entry whose vulnerability is gone fails until you remove it. Trivy has
no accept list: fix a HIGH or CRITICAL finding by raising the version `go.mod` requires.

Every Go module in the repository outside testdata directories is scanned unless
`.github/vulnscan-trees.txt` lists it with the reason it never ships, as it lists `consumer/`; a
module that is neither makes the scan exit 2. Trivy gets every setting on its command line, and
cicheck fails on a committed `trivy.yaml`, `.trivyignore` or `.trivyignore.yaml` anywhere in the
repository.

## Secret scan and dependency review

The `secrets` job scans the commits that are new in the run with the gitleaks release that
`tools/binaries.lock` pins, through `.github/scripts/gitleaks-range.sh`. Which commits depends on
the run:

- A pull request: its commits, from its base to its head. A pull request always adds a commit, so
  a range without one fails.
- A push to `master`: the commits it adds. A push that adds none passes with a notice.
- The `pr` mode: the commits that are not on `master`. A run on `master` has none, which passes
  with a notice.
- The `release` mode: the commits since the last release. `--since-last-tag` starts the range at
  the newest tag that matches `v[0-9]*` and comes before the release commit, so a release that is
  tagged already still scans from the release before it. The range always holds the release
  commit, and the scan fails when there is no such tag. The job's checkout fetches the full
  history and the tags.

It fails on any finding, and reports each with its file, line, rule and commit, never the
secret. gitleaks gets the rules of `.gitleaks.toml` from the script, so no configuration file,
ignore file or comment in a commit changes how it is scanned: no `.gitleaks.toml`,
`.gitleaksignore`, `.gitattributes` or `GITLEAKS_` variable is read, a `gitleaks:allow` comment
hides nothing, a file that git takes for binary because it holds a NUL byte is scanned as text,
and what a merge commit adds is scanned too. A change to the scan itself shows in the diff of the
script, `ci.yml` or `tools/binaries.lock`. When you change the rules in `.gitleaks.toml`, change
them in both scripts that copy them, `gitleaks-range.sh` and `security-scan.sh`; tests check that
they match.

The rules extend gitleaks' default rules, and with them its default allowlist of paths, which
applies to every rule, the Permit API key rule too: a secret in a path such as `.gitleaks.toml`,
`go.mod`, `go.sum`, an image, font or executable file, or a module under `vendor/` is not
reported, by the script or by the gitleaks git hook.

The script exits 0 when no secret is found, 1 when one is, and 2 when the scan did not run, such
as in a shallow clone, when base or head is not a commit of the repository, or when the range
holds no commit. With `--if-empty notice`, which CI gives on a push and in the `pr` mode, a range
without a commit passes with a notice. Scan your branch's commits, or those since the last
release, from the repository root:

```sh
.github/scripts/fetch-tool.sh gitleaks
.github/scripts/gitleaks-range.sh origin/master HEAD
.github/scripts/gitleaks-range.sh --since-last-tag HEAD
```

When it reports a key, revoke the key and remove it from your branch's history; tests, examples
and docs use fake keys that start with `permit_key_FAKE`. The scheduled
[security scan](#scheduled-scans-and-alerts) scans the whole history with the same rules and
hardening, except what a merge commit changes itself, which this scan covers when the merge is
pushed.

The `dependency-review` job runs on pull requests only. It compares the dependencies of the pull
request with those of its base in GitHub's dependency graph, and fails when the pull request adds
a dependency with a known vulnerability of high or critical severity; on a failure it also
comments a summary on the pull request. It has no local command; locally, the [vulnerability
scan](#vulnerability-scan) checks the dependencies of both modules.

## Scheduled scans and alerts

These jobs of `ci.yml` post to Slack:

| Job | When | Channel |
| --- | --- | --- |
| `security-scan` | Mondays and Thursdays (`SECURITY_CRON`), and in a manual run with the mode `security` | `SECURITY_SLACK_WEBHOOK_URL` |
| `digest` | Mondays (`WEEKLY_CRON`), and in the mode `weekly` | `SECURITY_SLACK_WEBHOOK_URL` |
| `mutation (weekly)` | Mondays, and in the mode `weekly` | `SLACK_WEBHOOK_URL` |
| `spec-drift` | Mondays, and in the mode `weekly` | `SLACK_WEBHOOK_URL` |
| `bench` | A push to `master`, Mondays and the mode `weekly`, when the benchmarks regressed or their report did not complete | `SLACK_WEBHOOK_URL` |
| `notify-gate-failure` | After `CI`, when it failed on a push to `master` or in a release, such as when `e2e` failed | `SLACK_WEBHOOK_URL` |

Each post starts with one line, `<repo> <check>: <result>. <details>. <run URL>`, which
`tools/cmd/notify` writes and posts; the security scan and the digest put their report under it,
and the weekly mutation tests give each module's efficacy and count of mutants that lived as the
details. notify escapes the line for Slack, and does not follow a redirect or print the webhook.
Without the webhook secret it prints a warning and exits 0; when Slack does not accept the post,
it exits 1 and the job fails. Only the step that posts gets the webhook, and only the step that
reads the alerts gets their token; no pull request job reads a secret.

### Security scan

`.github/scripts/security-scan.sh` reports each source with a result and a state of its own:

| Source | What it scans |
| --- | --- |
| `master-floor`, `master-ceiling`, `tools` | The three trees of the `vuln` job, one each, with vulnscan (`-only`) and Trivy. |
| `consumer-v2-latest` | A throwaway module that requires the latest release of `github.com/permitio/permit-golang/v2`, as an application resolves it with the default module proxy, and imports its packages; vulnscan (`-consumer`) and Trivy scan it. Before the first v2 release its state is "no release yet", which is not a failure. |
| `dependabot-alerts` | The open Dependabot alerts, read with `DEPENDABOT_ALERTS_TOKEN`; the job's own token cannot read them. The log and the job summary give their count only, and the Slack report lists them. |
| `history-secrets` | gitleaks over the full history of a mirror of every ref, with the rules of `.gitleaks.toml` and the hardening of the [secret scan](#secret-scan-and-dependency-review), except that what a merge commit changes itself is not scanned; the secret scan of each pull request and push covers it. |
| `verified-secrets` | In a manual run only: TruffleHog with `--results=verified` over the same mirror. Verification tries each candidate against its service. |

Findings are reported and pass. The job fails when a source did not complete, which secreport
reports as DID NOT RUN, and when TruffleHog verified a secret. secreport (`-findings report`)
writes the standard report to the job summary and in Slack's format: a headline with the findings
by severity, one line per source, the high and critical findings with their package, advisory
link and fix, the findings without a score ("Unscored"), and every source that did not complete.
It says "Clean" only when every source completed and none found anything. A value that a scanner
or GitHub reports is cut to fit the report, so that no finding is lost. A vulnerability that
`.github/vulnscan-accept.txt` accepts is listed with its fix `accepted until <last day>: <reason>`.

The history scan prints counts only: `history: N findings, M accepted, K new`; without
`GITLEAKS_HISTORY_ACCEPT_SHA256` its state says that every finding is new. Where a new finding is
goes to the Slack report only. A finding is accepted when the sha256 of its gitleaks fingerprint
is in the secret `GITLEAKS_HISTORY_ACCEPT_SHA256`, one digest per line. The list is a secret, not
a file, so the repository names no commit or file of an accepted finding. Every digest must match
a finding, or the source did not complete: the scan may have missed part of the history. Accept a
finding only once the secret it holds is revoked. TruffleHog's log must count a scan that read
something and verified as many secrets as it printed, or `verified-secrets` did not complete.

To list the digests of the findings, with where each is, run this on your own clone with the full
history. It refuses to run in GitHub Actions, whose log is public, and to write the file inside
the clone, where it could be committed:

```sh
.github/scripts/fetch-tool.sh gitleaks
digests="$(mktemp -d)/digests.txt"
.github/scripts/security-scan.sh --history-digests "$digests"
```

Keep in the file only the lines of the findings to accept, set the secret from it, and trash the
file:

```sh
gh secret set GITLEAKS_HISTORY_ACCEPT_SHA256 -R permitio/permit-golang < "$digests"
trash "$digests"
```

Run the whole scan from the repository root. It needs jq, gh, Go and the network. Without
`SECURITY_SLACK_WEBHOOK_URL` it posts nothing, and the report, without the private findings, goes
to the file that `GITHUB_STEP_SUMMARY` names:

```sh
GITHUB_REPOSITORY=permitio/permit-golang DEPENDABOT_ALERTS_TOKEN="$(gh auth token)" \
  GITHUB_STEP_SUMMARY=report.md .github/scripts/security-scan.sh --trufflehog skip
```

The scan needs these settings, which a maintainer makes before its first run on `master`:

- the secrets `SECURITY_SLACK_WEBHOOK_URL` and `SLACK_WEBHOOK_URL`, the incoming webhooks of the
  security channel and of the channel for releases, failures, the weekly mutation report, spec
  drift and benchmark regressions;
- the environment `security-scan`, which deploys only from `master`, with the secret
  `DEPENDABOT_ALERTS_TOKEN`: a fine-grained token that can read this repository's Dependabot
  alerts and nothing else. Create the environment before the job first runs, since GitHub
  creates an environment that a job names without any rule. Until the token is set,
  `dependabot-alerts` does not complete and the job fails;
- the secret `GITLEAKS_HISTORY_ACCEPT_SHA256`, once the secrets of the findings it accepts are
  revoked. Without it, every finding is new.

### Digest

`.github/scripts/digest.sh` posts the digest every Monday, even when nothing needs attention. It
lists the open pull requests of Dependabot, and flags the ones open 14 days or more; the entries of
`.github/vulnscan-accept.txt` whose last day is within 30 days or past; the result of the last
security scan, the newest of the 30 newest scheduled runs and the 30 newest manual runs; a newer
patch of the Go release in `go.mod`'s `toolchain` line and of each Go release that a test leg of
`ci.yml` pins (`GO_126`, `GO_127`), from go.dev; and every entry of `tools/binaries.lock` whose
GitHub project has a newer release. A new Go patch or release can be adopted once it is at least
7 days old, and the digest says from which day. A section that cannot be read says so, and the
job fails after posting the digest. Run it from the repository root; without
`SECURITY_SLACK_WEBHOOK_URL` it only prints the digest:

```sh
GITHUB_REPOSITORY=permitio/permit-golang GH_TOKEN="$(gh auth token)" .github/scripts/digest.sh
```

### Posting a notice

```sh
go -C tools build -o ../bin/notify ./cmd/notify
SLACK_WEBHOOK_URL=... bin/notify -channel ops -repo permitio/permit-golang -check CI \
  -result FAIL -details "what happened" -run-url "https://github.com/..."
```

`-channel security` posts to `SECURITY_SLACK_WEBHOOK_URL` and `-channel ops` to
`SLACK_WEBHOOK_URL`; `-body <file>` adds a report that its writer escaped for Slack. notify exits
0 when Slack accepted the post or the webhook is not set, 1 when the post failed, and 2 when its
command line or body is wrong. `go -C tools doc ./cmd/notify` describes it.

## Spec snapshots

`tools/specs` holds the API specs the SDK is checked against. It is in the `tools/` module, so the
specs never reach the module that applications download.

| File | What it is |
| --- | --- |
| `control-plane.json` | The control plane's spec, from `https://api.permit.io/v2/openapi.json`. |
| `pdp-cloud.json` | The cloud PDP's spec, from `https://cloudpdp.api.permit.io/openapi.json`. |
| `pdp-container.json` | The container PDP's spec, from `GET /openapi.json` of a running `permitio/pdp-v2` container. |
| `lock.json` | Where each of the three came from (the URL, or the image and its digest), the day it was fetched, the sha256 of the spec as fetched and of the snapshot, and its number of operations. |
| `pdp-container.extra.json` | What the container PDP serves without listing it in its `/openapi.json`: the AuthZEN, health and Trino routes, the facts catch-all and the compatibility mount of the facts routes, each with the file and line of the [PDP repository](https://github.com/permitio/PDP) at the image's tag that declares it. |
| `v1-routes.json` | The 91 routes of the control plane that v1.2.8 sent and the spec has, each with its operation ID, the line of v1.2.8 that holds the route, and the methods that send it. |
| `v1-route-fixes.yaml` | The routes that v2 changes on purpose, each with the v1 method, the route v1 sent and v2's route, and the reason. |
| `coverage.yaml` | The SDK's decision for every operation of the specs; see [API coverage](#api-coverage). |

[tools/cmd/specsnap](tools/cmd/specsnap) writes and checks the three snapshots. A snapshot is the
spec in canonical form, without its descriptions, summaries, titles and examples, with sorted keys
and an indent of two spaces, so a change in prose alone does not change it. Before it
canonicalizes the control plane's spec, specsnap pre-fixes the three constructs that are not
valid OpenAPI 3.1, so that a tool that compiles the schemas accepts the snapshot: the secret types
of the proxy config schemas, the draft-04 tuple in `OPALUpdateCallback`, and the second
declaration of the tag `Bulk Operations`. `go -C tools doc ./cmd/specsnap` describes both.

Each patch checks that its defect is still there. When the API fixes one, `-refresh` and the
weekly check fail with "fix no longer needed": remove the patch from `controlPlanePatches` in
`tools/cmd/specsnap/prefix.go` and its fixture in `testdata/control-plane.json`, then refresh.
A patch that finds its defect in other places fails with "the defect changed": update it.

The tests check the snapshots: `TestCommittedSnapshots` runs `specsnap -verify`, which fails when
a snapshot's sha256 or number of operations is not the one in `lock.json`, or a snapshot is not
in canonical form, and `TestV1Routes`, `TestV1RouteFixes` and `TestContainerExtras` check the
other files against the snapshots. specsnap exits 0 when the check passes, 1 when it fails, and 2
when it did not run, such as on a spec with fewer operations than a complete one has. Build it and
run it from the repository root:

```sh
go -C tools build -o ../bin/specsnap ./cmd/specsnap
bin/specsnap -verify
```

To refresh a snapshot, write it with specsnap, never by hand, and commit it with `lock.json`. The
control plane's and the cloud PDP's specs are fetched from where they are published:

```sh
bin/specsnap -refresh control-plane
bin/specsnap -refresh pdp-cloud
```

The container PDP publishes its spec only from a running container. Save its `/openapi.json` and
give specsnap the file and the image, with its tag and digest:

```sh
bin/specsnap -refresh pdp-container -from openapi.json \
  -image permitio/pdp-v2:<tag>@sha256:<digest>
```

When the image changes, check `pdp-container.extra.json` against the PDP repository at the new
tag. When a refresh drops a route that `v1-routes.json` or `v1-route-fixes.yaml` names, their
tests fail.

The `spec-drift` job of `ci.yml` runs every Monday (`WEEKLY_CRON`) and in the mode `weekly`. It
runs `specsnap -check-live`, which fetches the control plane's and the cloud PDP's specs, makes
their snapshots as `-refresh` does, and compares them with the committed ones, and compares the
digest of Docker Hub's `latest` `permitio/pdp-v2` with the image of `pdp-container.json`. It
uploads the live snapshots as the `spec-snapshots` artifact, and posts the result to
`SLACK_WEBHOOK_URL`. When something drifted, the post names the owner, `@permitio/r-d`, and the
day, 7 days on, by which a pull request must refresh `tools/specs`; when specsnap exits 2, it
says the check did not complete. The job fails in both cases. A step after the check runs the
[API coverage](#api-coverage) report with `-live`, against the live snapshots, and adds it to
the job summary without changing the job's result, so that an operation the published specs add
shows there before the snapshots are refreshed. Another step runs the [model drift](#model-drift)
tests against the live snapshots: when the models do not match them, the post says so, with the
owner and the day by which a pull request must fix the models or the allowlist, and when a test
prints `DID NOT RUN:`, it says that the check did not run, and why. The job fails in both cases.
To run the check yourself, which needs the network:

```sh
bin/specsnap -check-live -out "$(mktemp -d)"
```

## Model drift

The models in `models/` are written by hand, and the drift tests in [tools/drift](tools/drift)
check them against the spec snapshots in `tools/specs`, so that a model cannot leave out a field
that the spec lists, or name one that it does not, without a test failing. They are tests of the
tools module; to run them alone:

```sh
HTTP_PROXY=http://127.0.0.1:9 HTTPS_PROXY=http://127.0.0.1:9 \
  go -C tools test -count=1 ./drift
```

They check five things:

- **Models.** `tools/drift/model-map.yaml` maps each type of `models` to its schema, and each
  decision type of the root package that a request to the PDP or its reply holds, such as
  `CheckRequest` or `ObjectPermissions`, to the schema of the PDP container and, under `also`, to
  that of the cloud PDP. A struct has a field for each property of each of its schemas, and no
  other, by their JSON names. The type of each field holds every alternative of its property, and
  is of no JSON kind that the property does not allow: an object schema of `components.schemas` is
  the type whose row maps it, an enum schema the enum type whose row maps it, a string of format
  `date-time` a `models.Time`, and an object without properties `models.Attributes`, or a map of
  the type of its values. An enum type has a constant for each value of its schema, and no other.
  A count keeps the map complete: each exported type of `models` is a row under `models`, or under
  `blocks` when no schema describes it, such as `Optional`; each row is an exported type of
  `models` or of the root package, and not a name that both export; and `modelTypes` in
  `tools/drift/types_test.go` holds a value of each type under `models`, for the tests to read and
  decode into.
- **Strict round trip.** The tests write the maximal values of each schema of a row in memory:
  every property but those that a row leaves out, and across the values every alternative and
  every value of an enum, at each place in the value and under each alternative above it. Each decodes into the type with a strict decoder,
  which fails on a field that the type does not have, and encodes back unchanged, except that a
  null property whose field is not an `Optional` may come back left out. With an unknown field,
  and with an unknown value in place of each value of an enum, each still decodes with the SDK's
  decoder.
- **Operations.** The `RequestModel` and `ResponseModel` of each operation of the
  [registry](#operation-registry) map, through their rows, to the schemas of the operation's
  request body and of each of its replies in 200-299 that has a JSON body, in the spec that lists
  the operation's ID and in each other spec that lists its method and route, as the cloud PDP
  lists the routes of the PDP container that it serves. A request holds the model when one of its
  alternatives does, as the SDK chooses what it sends; a reply only when each of its alternatives
  does, as the API chooses. An alternative holds the model when it is the model's schema; for a
  `RequestModel` of `[]` and a name, such as `[]CheckRequest`, which the SDK sends as a JSON
  array, as the request of `POST /allowed/bulk` is, when it is an array of the model instead; and
  in the reply of a list, an operation with a `MaxPerPage`, when it is an array or a paginated
  envelope of it instead. So a Get whose model is smaller than the schema that the API returns,
  such as an environment without its email configuration, fails, and so does an operation that
  sends one model where the spec takes an array, or an array where it takes one.
- **Reply fixtures.** Each file in `testdata/replies` is the body of a reply that the wire tests
  serve, named `<operationId>.<status>[.<variant>].json`, such as `get_user.200.json` or
  `list_users.200.page.json`, with a variant of lower-case letters, digits and hyphens. The tests
  validate it against the schema of its operation's reply with that status, and against that of
  the operation of each other spec that lists the same method and route. Keep fixtures small:
  they are in the module zip.
- **Waits for sync.** The PDP container waits for sync on the routes under `/facts` that
  `pdp-container.json` lists. `tools/drift/waits_test.go` fails unless the table of the section
  "Facts and wait-for-sync" of `doc.go` lists exactly these routes; an operation of the registry
  is `WaitsForSync` exactly when the container waits on its route; the routes on which no
  operation waits yet are exactly those whose row of the control plane in `coverage.yaml` is
  planned, and the `pdp` row of each route has the status of that row; and no `pdp` row under
  `/facts` names a route outside the list but the catch-all.

The checks share one walker of the part of JSON Schema that the snapshots use. It checks `$ref`,
`type` (one type or a list), `enum`, `properties`, `required`, `additionalProperties`,
`patternProperties`, `propertyNames`, `items`, `prefixItems`, `anyOf`, `oneOf` and `allOf`, and is
stricter than JSON Schema in one way: an object schema that lists properties and leaves out
`additionalProperties` accepts no other property, so that a fixture cannot hold a field that the
spec does not name. It does not check documentation, defaults, formats and bounds on values, such
as `pattern` and `minimum`, which say nothing of a value's shape. A keyword that it does not know
stops the check that meets it, which reports `DID NOT RUN:` with the keyword's name instead of
skipping it: add the keyword to `keywords` in `tools/drift/schema.go`, with its check when it
constrains the shape of a value.

### Rows of the map

```yaml
models:
  UserRead:
    schema: UserRead        # its schema under components.schemas
    fields:
      key: {required: true, null_policy: n/a}
      email: {required: false, null_policy: n/a}
  AttributeType:
    schema: AttributeType   # an enum has no fields
blocks:
  Time: a timestamp, the schema of a string of format date-time
```

A row whose schema is in another snapshot than `control-plane.json` names it with `spec`:
`pdp-container` or `pdp-cloud`. A type that the schemas of two snapshots describe, as the PDP
container and the cloud PDP both describe a check, lists the others under `also`, one for each
snapshot, and the tests compare it with each. A property of a schema that the type has no field
for on purpose goes under `left_out`, with the reason, so that the maximal values of the round
trip leave it out; the comparison still finds it, and `allow.yaml` accepts that finding with its
trigger. A property of `left_out` that a field holds, or that none of the row's schemas has,
fails:

```yaml
models:
  CheckRequest:
    spec: pdp-container
    schema: AuthorizationQuery
    also:
      - {spec: pdp-cloud, schema: AllowedQuery}
    left_out:
      sdk: the PDP reads the language of the SDK from the X-Permit-SDK-Language header
```

Each field of a struct has a row of its own, by its JSON name, with:

- `required`, as the schema has it. apisurface reads it to find a field that turned required,
  which breaks code that builds the model; see [Breaking changes](#breaking-changes). A required
  field is tagged without `omitzero`, so that a body always holds it, any other field with
  `omitzero`, and no field with `omitempty`.
- `null_policy`, what the API does with null for the field in a request: `accepts`, and the field
  is a `models.Optional` tagged `omitzero`; `rejects`; `ignores`, when the API reads null as the
  field left out, as both PDPs do for an optional string of a check, which the spec of the cloud
  PDP types as a string or null, so that the SDK leaves the field out instead and it is no
  `Optional`; or `n/a` for a type that no request holds, such as the model of a reply. A field is an `Optional` exactly when its policy is `accepts`, and no field
  of a type that a `RequestModel` holds, itself or through its fields, has `n/a`.

The specs do not say which fields take null, so decide a policy from the API: send a request that
sets the field to null, and write `accepts` when the API takes it, such as to clear the field, and
`rejects` when it answers with an error.

### The allowlist

When the spec is wrong and the SDK follows the API, `tools/drift/allow.yaml` accepts the finding,
as it does when the spec lists an optional property that the API reads elsewhere and the SDK
leaves out, such as the `sdk` of a check, as the PDP reads the SDK from a header. A failing test
prints each finding after its key, such as `model UserRead field email`,
`round trip UserRead#/email` (`#` alone when a value does not decode),
`operation get_user response` or `reply get_user.200.json#/email`. A finding against a schema of
`also`, or against the operation of another spec with the same route, names the spec after `@`,
such as `model User@pdp-cloud field email` or `reply is_allowed_allowed_post.200.json@pdp-cloud`.
An entry gives the key, the `reason`, what the API does that the spec does not say, and the
`trigger`, when the entry can go, such as when the published spec lists the field:

```yaml
allow:
  - key: model UserRead field email
    reason: the API returns email, which the published spec does not list
    trigger: when the published spec lists email
```

A finding without a key, such as a field without a row in the map, is one that the SDK's own files
must fix, and no entry accepts it. An entry that accepts no finding fails, so that it goes once
the spec is fixed.

### Against the published specs

The `spec-drift` job runs the tests against the live snapshots that `specsnap -check-live -out`
writes, which `-specs` names, and its post says whether the models match them; a snapshot that the
directory does not hold, such as the container PDP's, is read from `tools/specs`. To run them so
yourself, which needs the network:

```sh
dir=$(mktemp -d)
bin/specsnap -check-live -out "$dir"
go -C tools test -count=1 ./drift -args -specs "$dir"
```

## API coverage

Which operations of the Permit API and the PDP the SDK covers is measured from the requests its
tests send, and [tools/cmd/apicoverage](tools/cmd/apicoverage) checks them against the
decisions in `tools/specs/coverage.yaml`.

`coverage.yaml` has a row for each operation of the control plane's spec, `control-plane`, and
for each route of the PDP, `pdp`: those of the container's and the cloud PDP's specs and of
`pdp-container.extra.json`, with one row for a route that more than one of them lists. A row
names the operation (`op`, its operationId), its `route` and its `stage`, the spec's: GA, EAP or
deprecated. Its `status` is one of these:

| Status | Meaning |
| --- | --- |
| `covered` | A public method sends it, and an offline wire test records a request to it. |
| `planned` | The SDK is to send it, and no public method does yet. |
| `excluded` | The SDK does not send it. `reason` is a key of `reasons` at the top of the file, each with a sentence that says why, such as `account-admin` or `operator`. |
| `deferred` | The SDK does not send it yet. `reason` says why, and `trigger` when to decide again. |

A covered or planned row whose route no published spec lists, such as the login-as route of
Permit Elements or the container PDP's facts catch-all, has `sdk_only` with the reason the SDK
sends it. `stage_override` gives the stage the SDK treats an operation as when the spec marks it
in a way the snapshot does not keep, such as "(EAP)" in a summary. `sdk_only_params` lists the
query parameters that the SDK sends to an operation whose spec does not list them. The file's
header describes each key.

To classify an operation that a spec adds, which apicoverage reports as undecided, add its row:
`planned` when the SDK is to send it; `excluded` when an application has no use for it, with a
reason of the vocabulary (add a key to `reasons` only with a public sentence, as a key that no
row gives fails); or `deferred` with a reason and a trigger when the decision waits for
something. When a public method starts to send a planned operation, its wire case records the
request: change the row to `covered` in the same commit, and rewrite the operation map. A
planned row that a request or a wire case reaches fails as stale.

`main_test.go` runs the root package's tests through `mockpermit.RunRecorded`, with
`os.Exit(mockpermit.RunRecorded(m.Run))`. When `PERMIT_API_COVERAGE_RECORD` names a file that
does not exist yet, each request that a `mockpermit` server receives goes there as a line of
JSON (JSONL): the mode, `wire`; the target, `api` or `pdp`; the method, the template of the route
that answered it, the path as the client sent it, the status and the name of the test, but never
the query, the headers or the body. A session trailer with the exit code of the tests and the
number of tests that wrote to the record ends it, so a record without it comes from a run that
did not finish. That count is not the number of tests that ran: a test writes to the record when
it starts a server or calls `mockpermit.RecordTest`. `internal/testhook` writes the format; the
[end-to-end scenarios](#end-to-end-tests) record through it in the modes `dryrun`, run offline,
and `live`, against the Permit cloud: the transport writes each attempt
of a call whose context carries a `testhook.Recorder`, which only a scenario puts there, so a
wire test's request is recorded once, by the `mockpermit` server. Requests to the test-only
routes, under `/__test/`, are never recorded.

A record holds no method names, so `testdata/operation-map.json` maps each public method to the
operations its wire cases send. `TestOperationMap` fails when a wire case names an operation that
the [operation registry](#operation-registry) does not hold, and when the map is stale; rewrite
it and review its diff:

```sh
go test -run TestOperationMap -update .
```

Build apicoverage, record the requests of the root package's tests, and run the report from the
repository root. The record file must not exist, so delete it before you record again:

```sh
go -C tools build -o ../bin/apicoverage ./cmd/apicoverage
PERMIT_API_COVERAGE_RECORD="$PWD/bin/api-coverage.jsonl" go test -count=1 .
bin/apicoverage bin/api-coverage.jsonl
```

apicoverage prints the rows of each section by stage and status, each covered row with the
methods that send it, and every problem, then its verdict. Each covered row also gets an
end-to-end state: `cloud` when a live request reached it, `local` when the record that `-local`
names did, `advisory` when one of its methods is on the list that `-advisory` names, `dry run`
when only a scenario run offline did, and `none`. `-local` takes the record of a run against a
full local stack in the same JSONL format, with every request in the mode `local`, as that run
will write it *(added in a later commit on this branch)*. A dry-run request is no end-to-end
evidence, and a report of records without a live request says so. apicoverage exits 0 when
every rule holds; 1 on an undecided GA operation, a stale or untested row, a stage change, an
SDK-only request that no row explains, an unused `sdk_only` entry, a row whose route the specs
no longer list, a route of another shape than the one v1 sent (the names of parameters do not
count) without an entry in `v1-route-fixes.yaml`, a template that does not fit its path or is
the route of another row, a request to a test-only route, or a reason outside the vocabulary;
and 2 when it could not check, such as on a record without its trailer, from tests that failed,
with a path that holds a query, or with fewer requests than covered rows.
`go -C tools doc ./cmd/apicoverage` describes every rule.

The `api-coverage` job of `ci.yml` runs these steps offline on every pull request and push to
`master`, adds the record of `TestCloud`'s [dry run](#end-to-end-tests) and
`-advisory tools/e2e/advisory.txt`, and writes the report to the job summary, where the
end-to-end column is a dry run or advisory:

```sh
go -C tools test -count=1 -tags e2e -run '^TestCloud$' ./e2e -args -e2e.dryrun \
  -e2e.record "$PWD/bin/e2e-dryrun.jsonl"
bin/apicoverage -advisory tools/e2e/advisory.txt bin/api-coverage.jsonl bin/e2e-dryrun.jsonl
```

## End-to-end tests

The end-to-end scenarios in [tools/e2e](tools/e2e) build only with the tag `e2e`. `TestCloud`
runs the scenarios of the cloud lane, the list `cloudScenarios`, in a scratch environment with
the key `gosdk-ci-<run>-<attempt>-<leg>`, which is created for the run and deleted at its end,
then checks that the Debug logs of every client of the run hold none of its API keys, and that
the run's request record holds a request of each scenario that passed. The code
that creates and checks what a scenario needs, in `tools/e2e/framework`, uses plain `net/http`
and not the SDK, so it does not depend on the code under test. It registers the teardown of
each object in a ledger before the request that creates it, treats a 404 to a deletion as
success and checks that the object is gone, polls with a bound on the elapsed time rather than
sleeping, and retries a 429 with the reply's `Retry-After` or a backoff with jitter, apart from
the SDK's own retries. Each call of the SDK carries the request recorder of `internal/testhook`.
The seed builder in `tools/e2e/framework/seed` creates the test data that a scenario needs in
plain `net/http` too: resources, roles, resource roles, relations, tenants, users, resource
instances, role assignments and relationship tuples, each registered in a ledger before its
request. A ledger tears its objects down the last registered first, and checks that each is gone
before it removes the next, so the seed may share the ledger of the scratch environment, whose
deletion makes every object in it read as gone; a scenario may also give the builder a ledger of
its own. `TestSeedBuilder` runs every call of the builder offline against the fake in every run,
live runs included, and the fake checks each body against the spec.

The dry run runs every scenario offline against the fake of the Permit API in
`tools/e2e/fake`, which keeps the objects of each collection of the control plane's spec in
memory and checks each request body with the walker of `tools/drift`. The `test` job runs it on
every pull request; run it from the repository root:

```sh
HTTP_PROXY=http://127.0.0.1:9 HTTPS_PROXY=http://127.0.0.1:9 \
  go -C tools test -race -count=1 -p 2 -timeout 5m -tags e2e ./e2e -args -e2e.dryrun
```

The test binary prints its verdict last and exits 0 on PASS, 1 on FAIL and 2 on DID NOT RUN:
when the ledger of a run does not reconcile, which means that it may have left objects behind,
or when an input of the gate below cannot be read. `go test` reports any exit but 0 as 1, so to
see the code, build the binary and run it in `tools/e2e`:

```sh
go -C tools test -c -race -tags e2e -o ../bin/e2e.test ./e2e
(cd tools/e2e && ../../bin/e2e.test -e2e.dryrun)
```

Without `-e2e.dryrun`, `TestCloud` runs against the Permit API, or the one at
`PERMIT_E2E_API_URL`, such as a local stack, with the project-level key in
`PERMIT_E2E_PROJECT_KEY`, in the project that `PERMIT_E2E_PROJECT` names, with the run and the
attempt in `GITHUB_RUN_ID` and `GITHUB_RUN_ATTEMPT` and the leg that `-e2e.leg` gives, and
skips when the key is not set. It runs in the scratch environment that
`.github/scripts/e2e-env.sh create` made for the run, which exports its ID, its project's ID
and its API key, and deletes it afterwards (see [Against Permit](#against-permit)); the SDK's
clients reach the PDP at `PERMIT_PDP_URL`. `-e2e.record` keeps its request record for
apicoverage. `tools/e2e/required.txt` lists the tests that every run of the scenarios must run
and pass, and the `e2e` job checks them with checkgotest.

`TestE2ESurfaceComplete` runs the cloud scenarios in a dry run of its own and checks that each
method of `testdata/operation-map.json` is in exactly one end-to-end state, with a count
sentinel: the methods add up to the four states.

| State | Where |
| --- | --- |
| cloud | A cloud scenario lists the method in its `methods`, and its dry run sends each of the method's operations. Two methods may send the same operation, so a request alone makes no method cloud. |
| local-only | `tools/e2e/local-only.txt`: `<Method> <reason> <phase>`, with the reason `org-key`, `account-level-write`, `needs-pdp-id` or `unverified-response`, and the phase of the local harness that proves the method instead. |
| advisory | `tools/e2e/advisory.txt`: `<Method> <ticket> <reason>`, for a scenario that may skip while the ticket is open. The scenario skips in the dry run too, with an entry of `.github/allowed-skips.txt` for its test that names the same ticket; the gate fails an advisory line whose ticket no entry for a test of `tools/e2e` names. The `e2e` job checks its live run against `tools/e2e/allowed-skips.txt`, which needs the same entry: checkgotest fails on an entry whose test did not skip, and `.github/allowed-skips.txt` names tests that the live run does not run, so the live run has a file of its own. |
| pending | `tools/e2e/pending.txt`: `<Method> e2e-pending <area>`, for a method that no scenario calls yet. A release refuses a pending line *(added in a later commit on this branch)*. |

When a commit adds a public method, it adds a line to `pending.txt`, or a scenario that calls
it. To move a method from pending to cloud, add a scenario to `cloudScenarios`, or extend one,
that calls the method with the context of `suite.ctx` and lists it in its `methods`, and remove
the method's line from `pending.txt` in the same commit: a line of a method that a cloud
scenario covers fails the gate, as a stale local-only line does. A cloud scenario that sends an
operation whose `KeyLevel` is `OrganizationLevel` fails the gate too, while
`tools/e2e/keys.yaml` sets `organization_key: false`, as the cloud lane has no
organization-level key: remove the call from the cloud scenarios and list each method that
sends the operation in `local-only.txt` with the reason `org-key`, or set `organization_key:
true` once the lane has the key.

### Against Permit

The `e2e` job of `ci.yml` runs the scenarios against Permit's production API, with the container
PDP pinned by digest (`PDP_IMAGE_PINNED_TAG` and `PDP_IMAGE_PINNED_DIGEST` in the workflow's
`env`); a push to `master`, a release and a manual run with the mode `e2e` require it. Advisory
legs against the newest release of the container PDP and against the cloud PDP *(added in a
later commit on this branch)* do not fail CI. `.github/workflows/e2e-pr.yml` runs the same job
on a pull request from a branch of this repository, in a run that Dependabot did not trigger, by
calling `ci.yml` with the mode `e2e`; a newer push to the pull request cancels its older run.
That lane is outside the check the ruleset requires, but required through the deployment rule:
a pull request into `master` needs a successful deployment to the `integration` environment,
which the lane makes, so a failing lane blocks the merge. A pull request from a fork, or a run
that Dependabot triggered, gets no secrets, so the lane skips it with a notice; a maintainer
merges such a pull request once the lane has run on a branch of this repository that holds its
commits. An outside contributor runs the offline tests and the end-to-end dry run, which CI
runs on their pull request too.

The job runs these scripts, which `tools/scripts` tests offline against a fake of the Permit
API and a stub `docker`, for its leg, `pinned`:

- `.github/scripts/sweep-envs.sh`, first, as the lane has no scheduled run: deletes the
  environments of the key's project whose key is a run prefix, `gosdk-ci-<run>-<attempt>-<leg>`,
  and that are older than 24 hours, such as one that a leg failed to delete. It deletes no other
  environment, at any age, and fails on a key that is not project-level or that the API refuses.
  A delete that fails, or a scope or a list that it cannot read for another reason, is a
  warning, and the job goes on: the sweep of a later run tries again.
- `.github/scripts/e2e-env.sh create <leg>`: creates the leg's scratch environment, reads its API
  key, masks it with `::add-mask::` before anything else, and then exports it with the
  environment's ID and its project's ID. Without `PERMIT_E2E_PROJECT_KEY`, a pull request and a
  manual run from a ref other than `master` pass with a notice, and a push to `master`, a release
  and a manual run from `master` fail, so that they never pass without the tests.
- `.github/scripts/pdp-up.sh up`: starts the container PDP as a step, with the environment's API
  key, on `127.0.0.1:7766`, and waits up to 300 seconds of elapsed time for `/healthy`. A
  service container would start before any step, when no key of the run exists yet. `pdp-up.sh
  logs` prints its log without the health checks when the job fails, and `pdp-up.sh down`
  removes it.
- The tests run in the environment, and checkgotest checks their output with
  `-require-file tools/e2e/required.txt -allow-skips tools/e2e/allowed-skips.txt`. Their request
  record is uploaded as the artifact `e2e-record-pinned`.
- `.github/scripts/e2e-env.sh delete <leg>`: deletes the environment, and warns that it leaked
  when the delete fails; a later sweep deletes it.

Runs of the job do not wait for each other: each works in a scratch environment of its own, and
the sweep deletes only environments older than 24 hours. A failure of `e2e` on a push to
`master` or in a release fails `CI`, which `notify-gate-failure` posts to Slack.

The job runs in the GitHub environment `integration`, to which a maintainer adds the secret
before the lane's first run. It deploys from `master`, from pull requests (`refs/pull/*/merge`)
and from `release/*` branches: a manual run from another branch fails before its first step,
and one from a `release/*` branch passes with a notice while the secret is missing. It holds:

- the secret `PERMIT_E2E_PROJECT_KEY`: a project-level API key of the project of these tests, one
  that can create, read and delete the project's environments and read their API keys. An
  environment-level key cannot; `e2e-env.sh` names the key level that is missing when the API
  answers 401 or 403. The lane has no organization-level key (`tools/e2e/keys.yaml`);
- the variable `PERMIT_PROJECT`: the key of that project.

cicheck checks that only a job in that environment reads a secret named `PERMIT_E2E_*`, and that
no workflow reads `PERMIT_API_KEY` or `PERMIT_ENV`, the v1 lane's settings, which the environment
holds too. To run a leg from your machine with such a key, against Permit, or against a local
stack at `PERMIT_E2E_API_URL` that the PDP container reaches too, such as one at
`host.docker.internal`, as `pdp-up.sh` gives the container that URL:

```sh
export PERMIT_E2E_PROJECT_KEY=... PERMIT_E2E_PROJECT=<project key>
export GITHUB_RUN_ID=<a number of your own> GITHUB_RUN_ATTEMPT=1 GITHUB_ENV="$(mktemp)"
.github/scripts/e2e-env.sh create local
set -a && . "$GITHUB_ENV" && set +a
PDP_IMAGE=permitio/pdp-v2:<tag> .github/scripts/pdp-up.sh up
go -C tools test -count=1 -tags e2e ./e2e -args -e2e.leg local
.github/scripts/pdp-up.sh down
.github/scripts/e2e-env.sh delete local
```

### Test tiers

| Tier | When | What | Required | Alert |
| --- | --- | --- | --- | --- |
| Offline | Every pull request and push to `master` | The tests of both modules and the end-to-end dry run, with no network | Yes: `CI` | The pull request's `CI` check |
| Local full stack | Before a release | A maintainer-run local full-stack suite | For a release *(the release workflow is added in a later commit on this branch)* | The release's harness evidence *(added in a later commit on this branch)* |
| Production scratch | A push to `master`, a manual run with the mode `e2e`, a release, and a pull request from a branch of this repository | The `e2e` job above, in a scratch environment of Permit's production API | Yes on `master`, in a release and in a manual run; on a pull request into `master`, through the deployment rule | `notify-gate-failure` on `master` and in a release; on a pull request, the lane's `CI` check and its `integration` deployment |
| Post-release smoke | After a release *(added in a later commit on this branch)* | The scenarios against the released module | No | *(Added in a later commit on this branch.)* |

The tiers are owned by `@permitio/r-d`.

## Dependency updates

Dependabot (`.github/dependabot.yml`) looks for new versions every Monday and opens pull requests
into `master`:

| Entry | What it updates | Commit prefix | Groups |
| --- | --- | --- | --- |
| `gomod` in `/` | The SDK's requirements in `go.mod`. | `deps` | `minor-and-patch`, `security` |
| `gomod` in `/tools` | Every requirement in `tools/go.mod`, the indirect ones too, so the programs its `tool` directives pin move as well. | `deps-dev` | `tooling`, `security` |
| `gomod` in `/consumer` | The requirements in `consumer/go.mod`: only the SDK, which it replaces with the repository root, so nothing until it requires another module. | `deps-dev` | `minor-and-patch`, `security` |
| `github-actions` in `/` | The actions in `.github/workflows`, pinned to commit SHAs with version comments. | `deps-dev` | `tooling`, `security` |
| `pre-commit` in `/` | The hooks in `.pre-commit-config.yaml`, pinned to commit SHAs with `# frozen:` comments. | `deps-dev` | `tooling` |

- A new version gets a pull request only once it is at least 7 days old (the cooldown), and a new
  major version of a Go module once it is 14 days old. A group takes the minor and patch updates
  of its entry; a major update gets a pull request of its own. Each entry keeps at most 5 version
  update pull requests open.
- Security updates are a repository setting. Each entry's `security` group puts its security
  updates in one pull request, which waits for no cooldown. Dependabot has no security updates
  for pre-commit hooks.
- The `go` line of a `go.mod` is the minimum Go version and changes only on purpose. Dependabot
  does not move it or the `toolchain` line to new Go releases, and does not read
  `tools/binaries.lock`. Move the `toolchain` line and `tools/binaries.lock` by hand to the
  newest release that is at least 7 days old; the weekly [digest](#scheduled-scans-and-alerts)
  reports newer releases. A dependency update whose new version needs a newer Go raises
  the `go` line: the Go 1.26.0 test leg then fails, and the pull request is not merged as it is.
- `consumer/go.sum` records the sums of the SDK's requirements too, and Dependabot does not update
  it with the root module's. On a pull request that changes the root's requirements, run
  `go -C consumer mod tidy` and push its change; until then the consumer's tidy hook fails.
- v1 gets no dependency updates: it receives no fixes after v1.4.0.

Every Go module that git tracks, outside testdata directories, needs a `gomod` entry for its
directory and a hook in `.pre-commit-config.yaml` whose entry is `go mod tidy -diff` (the root)
or `go -C <dir> mod tidy -diff`. When you add a module, add both, or list it in
`moduleExceptions` in `tools/cmd/cicheck/modules.go` with the reason it gets neither. Only an
entry without a `target-branch` counts. cicheck fails when a module lacks one, when an exception
has no reason, names no module or still has an entry, when an entry names no module, when any
of an entry's cooldown days is shorter than the times above, and when a cooldown has an
`include` or `exclude` list. Run it as the [CI](#ci) section shows.

CI checks the message of every commit of a pull request, Dependabot's included. The test
`TestCheckCommitMsgDependabot` in `tools/scripts` checks that the messages Dependabot writes for
these entries pass.

## Errors and redaction

`errors.go` holds `Error`, which a failed call returns or wraps, and the sentinel errors. The SDK
creates an `Error` only through the unexported constructors there, which the transport
(`transport.go`) calls: for a reply with an error status, for a reply in 200-299 it cannot
decode, and for a request that got no complete reply.

- To add a sentinel, declare it in the `var` block of `errors.go`, with a doc comment and a text
  that starts with `permit: `, and add it to the `sentinels` table of `errors_test.go`. A sentinel
  that stands for a status also gets a case in `Error.Is`. Then rewrite the surface snapshot and
  add the sentinel to `CHANGELOG.md`.
- Credentials are replaced when an `Error` is created, never when it is printed. Each client owns
  a `redact.Set` (`internal/redact`) that holds its API key and the credentials its calls return.
  The constructors replace its members, and what the stateless patterns of `redact.Scrub` find, in
  the fields they read from the reply, in the body they keep and in the text of the cause. No
  package-level variable holds a credential.
- The transport adds the credentials that a reply in 200-299 returns to the client's
  `redact.Set` with `Set.AddFields`, so that the client's later errors and log lines replace them
  too: the strings under a key that is, in lower case and without `-` and `_`, one of the
  credential names that its doc lists, such as secret, token, apikey or accesstoken.
- When a route's body holds a credential in a field whose name is not one of those names, list
  the field in the `SensitiveBodyFields` of the route's `rest.Op` in the
  [operation registry](#operation-registry), also when the name contains one of them, such as
  `webhook_secret`: the transport passes them to `redact.Scrub`, which then replaces the field's
  values in the error bodies of that route, and to `Set.AddFields`, which adds their values in a
  reply in 200-299 to the client's `redact.Set`.
- `FuzzScrub` (`internal/redact`) and `FuzzError` (the root package) check that no key reaches an
  error. `go test` runs their seeds, in `testdata/fuzz`; to search for new inputs, run one, for
  example `go test -run '^$' -fuzz '^FuzzError$' -fuzztime 1m .`, and add each input that fails
  to `testdata/fuzz` with the fix.

## Logging

Only `internal/logging` logs. depguard fails an import of `log` outside `internal/logging` and
`options.go`, which declares `WithLogger`, and of `log/slog` outside those two, `internal/redact`,
for `Secret.LogValue`, and test files, which record what the SDK logs with slog's handlers. Each
client has a `logging.Logger`, which
wraps the handler of the caller's logger in one that replaces the members of the client's
`redact.Set`, and what the stateless patterns of `redact.Scrub` find, in every part of a line. To
log something new, add a method to `logging.Logger`, test it there, and list its attributes in
the documentation of `WithLogger`:

- Its message is a constant that starts with `permit: `: sloglint fails a message that is not
  static, and an attribute key on its forbidden list (api_key, token, authorization, password,
  secret).
- A Debug line says what a request did: its route template, status, attempt, duration and error,
  never a header or a body. The transport writes one for each attempt. The line reports the HTTP
  attempt: a reply in 200-299 that `do` or `doList` cannot decode is the error of the call, and
  not of the line, which has its status and no error.
- Warn is for warnings about the configuration only, each logged once, from `New`.

No go.mod of the repository requires zap, so the zapslog example of `WithLogger`,
`internal/logging/testdata/zapslog_example.go.txt`, builds in a module of its own. The `docs` job
runs these commands; locally, they need the network for the module proxy:

```sh
dir=$(mktemp -d)
cp internal/logging/testdata/zapslog_example.go.txt "$dir/main.go"
go -C "$dir" mod init example.com/zapslogexample
go -C "$dir" mod edit -require=github.com/permitio/permit-golang/v2@v2.0.0 \
  -replace=github.com/permitio/permit-golang/v2="$PWD"
go -C "$dir" get go.uber.org/zap@v1.28.0 go.uber.org/zap/exp@v0.3.0
go -C "$dir" mod tidy
go -C "$dir" run .
```

Dependabot does not see those two versions, which the `docs` job sets in `ZAP_VERSION` and
`ZAP_EXP_VERSION`: move them by hand to the newest release that is at least 7 days old.

## Adding a client option

`options.go` holds the options of `permit.New`, each a `With` function that returns an `Option`,
and it is the one file that reads the environment: forbidigo fails `os.Getenv` and
`os.LookupEnv` anywhere else. To add an option:

1. Write its `With` function in `options.go`, with a doc comment that gives its default and what
   it rejects. The option sets a field of `settings`. For an argument that the SDK cannot use, its
   `apply` returns a `*configError`, so that `New` returns an error that matches
   `ErrInvalidConfig` and sends no request. The text the option prints must not hold a
   credential.
2. Give each check a rule in `internal/validate`: an ID that never changes and a one-line message,
   in the sorted `rules` list. Rewrite `testdata/validation-rules.txt` with
   `go test -run TestValidationRules -update .`; diff mode counts a new ID as a breaking change.
3. Add the option to `optionEffects` in `options_test.go`: the option with a value other than the
   default, how to read its setting from a `Client`, the value the option gives it, and, in
   `unset`, values of the option that keep the default, such as `WithLogger(nil)`.
   `TestOptionsHaveEffect` lists the exported `With` functions through `internal/surface` and
   fails for one without an entry; its count sentinel fails when the two lists differ.
4. Add each input that the option rejects to `TestInvalidConfiguration`, rewrite the surface
   snapshot, and add the option to `CHANGELOG.md` and `MIGRATION.md`.

## Operation registry

`internal/rest` holds a `rest.Op` for each operation that the SDK sends, in `defaultOps`
(`internal/rest/ops.go`): its ID, the operationId of the spec and the `op` of its row in
`tools/specs/coverage.yaml`; its method; its route, the spec's path template; and the facts about
it that the transport and the checks before a request read, so that no fact is kept in a list of
its own:

| Field | Meaning |
| --- | --- |
| `Idempotent` | A retry may send it again after a 500, 502, 503 or 504 reply or a request without a complete reply: the GET, PUT and DELETE operations, and the PDP's decisions. |
| `Facts` | A route of the facts API that the PDP container also serves under `/facts`: its route starts with `/v2/facts/{proj_id}/{env_id}/`, and with `WithFactsViaPDP` the call goes to the container, along the route that `Op.ViaPDP` gives. Not set rules, user invites or proxy configs, which only the API serves. |
| `WaitsForSync` | The PDP container can hold its answer until the change reaches its policy. Only a `Facts` operation can wait. |
| `ContainerOnly` | Only the PDP container serves it; the cloud PDP answers 404 with an empty body. |
| `KeyLevel` | The narrowest API key that may call it: `EnvironmentLevel`, `ProjectLevel` or `OrganizationLevel`. |
| `DeleteSelf` | A DELETE whose path names the object it deletes, so that a 404 means the object is gone already. Not a removal whose body names what it removes. |
| `NoRetry` | Never sent twice, not even after a 429, such as rotating an API key. |
| `SensitiveBodyFields` | Fields of its bodies that hold a credential under a name other than the credential names of `redact.Set.AddFields`, such as `webhook_secret`. |
| `NoReplyCredentials` | Its replies in 200-299 hold no credential that the server issued, so their values do not join the client's `redact.Set`: the PDP's decisions and the PDP's lists of permissions, of authorized users, of tenants and of role assignments, whose replies hold the application's values and can echo the request. |
| `MaxPerPage` | The largest `per_page` of a list operation, or 0. |
| `RequestModel`, `ResponseModel` | The names of the types of `models` that its request and reply bodies hold, or empty; an operation of the PDP names the type of the root package that its request holds, such as `CheckRequest`, and no `ResponseModel`, as the SDK reads the reply itself: `allow` of a decision, or the entries of the permissions of a user, of the authorized users or of the tenants; a list of the PDP whose items are a type of the root package, such as `LocalRoleAssignment`, names none either, as its reply holds no type of `models`. A request that is a JSON array of the type names it after `[]`, such as `[]CheckRequest`. |

A `*permit.Client` holds the registry, `rest.Default()`. The registry fails on an operation that
is not valid, such as one without a key level, `DeleteSelf` on another method than DELETE,
`WaitsForSync` without `Facts`, `Facts` on a route outside `/v2/facts/{proj_id}/{env_id}/` or
`NoRetry` with `Idempotent`; on two operations with one ID; and on a route of `defaultOps` that
starts with `/__test/`. `TestRepositoryWaits`, in `tools/drift/waits_test.go`, fails unless an
operation is `WaitsForSync` exactly when the PDP container waits on its route, one of the routes
under `/facts` that `pdp-container.json` lists; see [Model drift](#model-drift).
`TestRepositoryRegistry`, in `tools/registry_test.go`, checks each operation of `rest.Default()` against `tools/specs`: its ID
is the op of a planned or covered row of `coverage.yaml`, its method and route are those that a
spec lists for the ID, or those of the row when the row is `sdk_only`, and its `RequestModel`,
after the `[]` of a JSON array, and `ResponseModel` name exported types of `models` or of the root
package. `TestOperationMap` fails when a wire case names an operation that the registry does not
hold.

A test that needs an operation the SDK does not have, such as to check the transport, builds its
client with `newTestClient` (`export_test.go`), which takes test-only operations, whose routes
start with `/__test/`. That client's registry holds `rest.Default()` and the test's operations,
and no other registry holds them, so a test-only operation never reaches an application. The
request record leaves out requests to these routes.

## Wire encoding

`internal/wire` writes and reads what goes over the wire, one way for each shape:

- request bodies with `wire.Encode`: `encoding/json`, with `<`, `>` and `&` as they are;
- reply bodies with `wire.Decode`, which keeps a number that it reads into an `any` as a
  `json.Number`, and fails on an empty body or on a second value after the first;
- list replies with `wire.DecodeList`, which reads a bare array or the paginated envelope by the
  first byte, never by trying one shape and then the other;
- queries with `wire.Query`, which sorts the parameters, repeats the key of a list, leaves out a
  parameter that is not set, never sends a parameter of `wire.DeprecatedParams`, and writes a
  `float64` with 64 bits, as `encoding/json` does;
- paths with `wire.Path`, which escapes each path parameter on its own with `url.PathEscape`, and
  returns a `*wire.PathError`, with the validation rule `path.segment.empty` or
  `path.segment.dot`, for an empty parameter or one that is `.` or `..`, before anything is sent.

`TestRepositoryDeprecatedParams`, in `tools/registry_test.go`, checks `wire.DeprecatedParams`
against the specs: it lists every query parameter that a spec marks deprecated, and no parameter
that an operation takes without marking it deprecated. `FuzzQuery` and `FuzzPath` check that a
query and a path read back as what was put in; `go test` runs their seeds, in
`internal/wire/testdata/fuzz`, and you search for new inputs as for `FuzzScrub`, as
[Errors and redaction](#errors-and-redaction) shows.

## Adding or changing an API method

There is no code generator to run: the models in `models/` are written by hand, and the
[drift tests](#model-drift) check them against the snapshots of the Permit API's specs. The steps
below are a skeleton that later commits on this branch fill in; whatever is marked *(added in a
later commit on this branch)* does not exist yet.

1. Find the operation in the spec snapshots, `tools/specs/*.json`, and its row in
   `tools/specs/coverage.yaml`.
2. Write or extend the model in `models/`, its row in `tools/drift/model-map.yaml`, and for a new
   type its value in `modelTypes`; see [Model drift](#model-drift). Give each field the type that
   says what the API does with it:
   - A field of an update model that the API accepts null for is a `models.Optional[T]`, tagged
     `omitzero`: its zero value leaves the field out, so the API keeps what it stores,
     `models.Null[T]()` sends null and `models.Some(v)` sends `v`. Use it only where the API
     accepts null, the fields whose row has `null_policy: accepts`. Encoding an unset `Optional`
     fails, so a field tagged `omitempty`, or not tagged, fails instead of sending null.
   - A nested map that the API rejects null for is a plain map, tagged `omitzero`: nil leaves the
     field out, and an empty map that is not nil sends `{}`.
   - An enum is an open enum, as `models.AttributeType` in `models/enum.go` is: a string type,
     a constant for each value whose name starts with the type's name, such as
     `AttributeTypeString`, and a `Known` method, without an `UnmarshalJSON` that rejects a
     value the SDK does not know. Check it in `TestOpenEnums` (`models/enum_test.go`), which
     fails for a type with a `Known` method that it does not check.
   - Attributes, and any other map of JSON values, are `models.Attributes`, which keeps each
     number as a `json.Number`; a timestamp is a `models.Time`, which reads RFC 3339 with or
     without a zone.

   `models/testdata/golden` holds the bodies that each kind of `Optional` field, and a map
   tagged `omitzero`, encode to, and both supported Go versions must write them byte for byte.
   `FuzzOptional` and `FuzzAttributes` check that a body decodes and encodes back by value, and
   `go test` runs their seeds, in `models/testdata/fuzz`.
3. Add the method, and its `rest.Op` to `defaultOps` in `internal/rest/ops.go`, with the fields
   that the [operation registry](#operation-registry) lists. The method makes its call with one
   of the functions of `transport.go`: `do` for a reply that holds an object and `doNoValue` for
   one whose body it does not read. A List method calls `listPage` of `page.go`, with the list's
   shape, and returns its `*Page[T]`; its All method returns the iterator of `all`.
   Two of the fields decide when a call sends the request again: `Idempotent` for an operation
   that does the same when it is sent twice, which a call sends again after a 429, a 500, 502,
   503 or 504, or a request without a complete reply; `NoRetry` for one that a call never sends
   twice, not even after a 429, such as rotating an API key; and neither for any other
   operation, such as a create, which a call sends again after a 429 only, as the server did not
   act on it. A method that fills `{proj_id}` or `{env_id}` from the client's scope, rather than
   from its parameters, gets them from `c.scopeFor(ctx, opID)` (`scope.go`), which reads the
   placeholders of the operation's route, not its `KeyLevel`: a route with `{env_id}` needs a
   project and an environment, one with `{proj_id}` alone a project, and one with neither no
   scope and no lookup. When the scope lacks what the route needs, `scopeFor` returns an error
   that matches `ErrInvalidConfig`, and the call sends no request for the operation; without
   `WithScope`, the lookup of the scope may come first. A method of the facts API instead gets
   its request from `facts.prepare` (`facts.go`), with the parameters that follow `{env_id}`:
   with `WithFactsViaPDP` the call goes to the PDP container under `/facts` and needs no scope,
   and without it to the API with the scope. A call through a copy that waits for sync carries
   the wait's headers, and fails with an error that matches `ErrInvalidConfig`, sending nothing,
   without `WithFactsViaPDP`, for an operation that is not `WaitsForSync` and for a timeout less
   than zero.
4. Document it, with the phrases that `docRules` asks of it. Give a new service type, and a new
   decision method, an Example with an output comment in `package permit_test`, and list a
   decision method in `decisionMethods`; see [API reference](#api-reference).
5. Add a wire test case for the method and a fixture with the API's reply, in `testdata/replies`
   as `<operationId>.<status>[.<variant>].json`, which the drift tests check against the spec,
   and rewrite the surface snapshot; see [Wire tests](#wire-tests),
   [Model drift](#model-drift) and [Public surface and wire cases](#public-surface-and-wire-cases).
6. Rewrite `testdata/operation-map.json`, mark the operation's row of
   `tools/specs/coverage.yaml` covered, and run the API coverage report; see
   [API coverage](#api-coverage).
7. Cover the method in a cloud scenario, or list it in `tools/e2e/pending.txt`, or in
   `tools/e2e/local-only.txt` with a reason; see [End-to-end tests](#end-to-end-tests).
8. Add its entries to `CHANGELOG.md` and `MIGRATION.md`, and remove what the `MIGRATION.md` entries
   cover from `tools/migration/pending.txt`; see
   [Public surface and migration](#public-surface-and-migration).

A call sends its request again as `internal/transport/retry.go` decides from the operation's
fields, and each attempt has its own Debug line and line of the request record. A wire test
against `mockpermit` registers a reply for each attempt, since the server fails a test that sends
more requests to a route than it has replies; a test about one attempt gives its client
`WithRetry(RetryPolicy{})`, which sends each request once. To test the retries of an operation,
and the waits between them, call it in a `testing/synctest` bubble with an in-memory
`http.RoundTripper` that `WithHTTPClient` passes, as `retry_test.go` does: the bubble's clock
moves only while every goroutine in it waits, which a request over a network connection never
lets it do, so a wait of seconds takes no real time and is exact.

## Commits and pull requests

- Keep each pull request to one change.
- Write the commit subject in the imperative mood, in at most 72 characters, for example
  `Return an error for an empty user key`. Maintainers start the subject with an internal ticket
  ID, `PER-<id>: `; outside contributors leave it out.
- Commit messages must not contain `#N` or `GH-N` references, closing keywords such as `Fixes` or
  `Closes`, `@handles` or e-mail addresses. Link the issue in the pull request description
  instead. An e-mail address may appear only inside `<...>` on a `Co-authored-by`,
  `Signed-off-by`, `Reviewed-by`, `Tested-by`, `Acked-by`, `Reported-by`, `Suggested-by` or
  `Helped-by` line at the end of the message, such as `Co-authored-by: Name <email>`; a module
  version such as `golang.org/x/vuln@v1.8.0` is fine.
- `.github/scripts/check-commit-msg.sh` checks these rules in two modes. The commit-msg hook runs
  it on the message before git cleans it up, so it skips the lines that start with `#` and the
  diff that `git commit --verbose` adds, as git removes both. CI runs it with `--committed` on
  the message of every commit of a pull request, as `git log --format=%B` prints it, and checks
  every line: `git commit -m` and `-F` keep lines that start with `#`.
- What lands on `master` is the merge of a pull request whose commits CI checked. A rebase merge
  keeps those commits. A maintainer who squashes writes the squash commit's message to the same
  rules, and deletes the `(#N)` that GitHub appends to its title.
- Maintainers merge pull requests, and close the issue when the release that contains the change
  ships.
- CI runs on a pull request from a fork only after a maintainer approves the run, and the
  end-to-end tests against Permit do not run on forks; see [Against Permit](#against-permit).

## Public repository rules

This repository is public.

- Never commit secrets: API keys, tokens, `.env` files, private keys or certificates.
  `.gitignore` covers the usual file names (`.env*` except `.env.example`, `*.pem`, `*.key`,
  `*.p12`, `*.pfx`, `*.tfstate*`, `credentials*`). Tests, examples and docs use fake values: API
  keys that look like `permit_key_FAKE...`, and `example.com` for hosts and email addresses.
- Commit messages, code, comments, tests and pull request text refer to internal work only by
  ticket ID (`PER-<id>`), and name no internal hostnames, private repository names, customers or
  paths on your machine.
