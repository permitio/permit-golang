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
gofmt. Every exported identifier has a doc comment.

## Tests run offline

The tests that `go test ./...` runs never reach the Permit API, a PDP, Docker or any other network
service, and need no API key. A test that makes HTTP requests serves the replies from
`net/http/httptest` and points every URL of the client it builds at that server.

To check that no request leaves your machine, run the tests with a proxy address that refuses
connections. Go sends requests for loopback addresses directly, so `httptest` servers still work:

```sh
HTTP_PROXY=http://127.0.0.1:9 HTTPS_PROXY=http://127.0.0.1:9 go test -count=1 ./...
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

## Adding or changing an API method

There is no code generator to run: the models in `models/` are written by hand, and a drift test
*(added in a later commit on this branch)* checks them against a snapshot of the Permit API
specification. The steps below are a skeleton that later commits on this branch fill in; whatever
is marked *(added in a later commit on this branch)* does not exist yet.

1. Find the operation in the API spec snapshot *(added in a later commit on this branch)*.
2. Write or extend the model in `models/`, and its row in the drift map *(added in a later commit
   on this branch)*.
3. Add the method, and its entry in the operation registry *(added in a later commit on this
   branch)*.
4. Add a wire test case for the method and a fixture with the API's reply *(the wire tests are
   added in a later commit on this branch)*.
5. Run the update targets and the API coverage report *(added in a later commit on this branch)*.
6. Add an end-to-end scenario for the method, or mark it pending or local-only *(the end-to-end
   tests are added in a later commit on this branch)*.
7. Add its entries to `CHANGELOG.md` and `MIGRATION.md` *(`MIGRATION.md` is added in a later commit
   on this branch)*.

## Commits and pull requests

- Keep each pull request to one change.
- Write the commit subject in the imperative mood, in at most 72 characters, for example
  `Return an error for an empty user key`. Maintainers start the subject with an internal ticket
  ID, `PER-<id>: `; outside contributors leave it out.
- Commit messages must not contain `#N` references, closing keywords such as `Fixes` or `Closes`,
  or `@handles`. Link the issue in the pull request description instead.
- Maintainers merge pull requests, and close the issue when the release that contains the change
  ships.
- CI *(added in a later commit on this branch)* runs on a pull request from a fork only after a
  maintainer approves the run, and the end-to-end tests against Permit do not run on forks.

## Public repository rules

This repository is public.

- Never commit secrets: API keys, tokens, `.env` files, private keys or certificates.
  `.gitignore` covers the usual file names (`.env*` except `.env.example`, `*.pem`, `*.key`,
  `*.p12`, `*.pfx`, `*.tfstate*`, `credentials*`). Tests, examples and docs use fake values: API
  keys that look like `permit_key_FAKE...`, and `example.com` for hosts and email addresses.
- Commit messages, code, comments, tests and pull request text refer to internal work only by
  ticket ID (`PER-<id>`), and name no internal hostnames, private repository names, customers or
  paths on your machine.
