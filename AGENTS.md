# Agent instructions

Instructions for coding agents that work in this repository. `CLAUDE.md` is a link to this file.
[CONTRIBUTING.md](CONTRIBUTING.md) applies to agents too.

The repository holds v2 of the Go SDK for Permit.io, the module
`github.com/permitio/permit-golang/v2`. `go.mod` sets the minimum Go version (`go 1.26.0`) and a
newer `toolchain`.

## Build, test and lint

Run every check twice: with `GOTOOLCHAIN=go1.26.0`, the minimum version, and with `GOTOOLCHAIN`
set to the `toolchain` line of `go.mod`. Call gofmt through `go env GOROOT` so it comes from the
selected toolchain. Run tests offline, with a proxy address that refuses connections, so a
request that tries to leave the machine fails:

```sh
"$(go env GOROOT)/bin/gofmt" -l .  # must print nothing
go vet ./...
go build ./...
HTTP_PROXY=http://127.0.0.1:9 HTTPS_PROXY=http://127.0.0.1:9 \
  go test -race -count=1 -shuffle=on -p 2 -timeout 5m ./...
HTTP_PROXY=http://127.0.0.1:9 HTTPS_PROXY=http://127.0.0.1:9 \
  go -C tools test -race -count=1 -shuffle=on -p 2 -timeout 5m ./...
go mod tidy -diff                  # must print nothing
```

The tests that `go test ./...` runs never reach the Permit API, a PDP, Docker or the network:
every URL such a test gives a client points at a local `net/http/httptest` server.

Then run every git hook on every file, with the prek that `tools/binaries.lock` pins. The hooks
include golangci-lint over every module (`.github/scripts/golangci-all.sh`); the first run needs
the network to download and build them:

```sh
.github/scripts/fetch-tool.sh prek
bin/prek run --all-files           # must pass with no file changed
```

Install the hooks with `bin/prek install` only in the main clone. Do not run it from a git
worktree: a worktree shares its hooks directory with the main clone. In a worktree, check each
commit message with the commit-msg hook before you commit:

```sh
bin/prek run --stage commit-msg --commit-msg-filename <file>  # the file that holds the message
```

The hook skips lines that start with `#`, as git removes them from a message you edit. CI checks
each commit message of a pull request with `.github/scripts/check-commit-msg.sh --committed`,
which checks every line as `git log --format=%B` prints it. Before you push, run the same check
over the branch's new commits:

```sh
.github/scripts/commits-check.sh <base> <head>  # the commit the branch starts from, and its tip
```

## Working rules

- One commit per ticket. The subject is `PER-<id>: <imperative summary>`, at most 72 characters.
- A lint or format fix that changes no behaviour and is found after the code it fixes goes in its
  own commit, with `testdata/api-surface.txt` byte-identical.
- Run mutation tests only through `tools/cmd/mutationgate`, within its caps on workers, memory
  and time, as CONTRIBUTING.md shows; never run a mutation tool directly.
- Run at most one Docker stack at a time.
- Push the next ticket's commit only after CI is green on the previous one.

## Public text

This repository is public. These rules apply to commit messages, code, comments, tests, docs,
pull request text and test evidence:

- Refer to internal work only by ticket ID (`PER-<id>`).
- No `Claude-Session` trailer, no claude.ai links and no local paths (`/Users/`, `/private/tmp/`).
  Evidence from planted failures uses paths relative to the repository root.
- Commit messages contain no `#N` references, `@handles` or closing keywords.
- No backend file paths, private repository names, internal hostnames, traffic counts or ranks,
  customer or support references, incidents or session URLs.
- A commit body is 1-3 plain sentences a public reader can follow, written fresh for the commit,
  never copied from a ticket's problem statement.
