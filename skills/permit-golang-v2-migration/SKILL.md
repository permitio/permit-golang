---
name: permit-golang-v2-migration
description: Migrates Go code from v1 of the Go SDK for Permit.io, the module github.com/permitio/permit-golang (v1.3.x), to v2, github.com/permitio/permit-golang/v2. Use it when a Go module imports github.com/permitio/permit-golang/pkg/... and is to move to v2. A scanner lists each use of v1 with the entries of v2's migration guide that replace it; then edit each use by its entry and verify with go build and go vet.
---

# Migrate from permit-golang v1 to v2

Move a Go module from `github.com/permitio/permit-golang` (v1.3.x) to
`github.com/permitio/permit-golang/v2`, each use of v1 by its entry in v2's migration guide,
`MIGRATION.md`. Run the commands from the root of the module. `<skill>` is the directory of this
file. Each entry has an ID, such as `M-users-5`, that never changes; this file links entries on
the repository's default branch, as it is copied out of the repository, where a relative link
would break.

## 1. Start from code that builds

- `go version` prints go1.26.0 or later: v2 needs Go 1.26
  ([M-module-2](https://github.com/permitio/permit-golang/blob/master/MIGRATION.md#M-module-2)).
- `go build ./...`, `go vet ./...` and `go test ./...` pass with v1. Commit first, so that the
  migration is one diff to review.

## 2. Add v2 and find its guide

```sh
go get github.com/permitio/permit-golang/v2@latest
guide="$(go list -m -f '{{.Dir}}' github.com/permitio/permit-golang/v2)/MIGRATION.md"
```

The guide in the v2 module matches the version you migrate to. v1 and v2 are separate modules,
so both can be required until the last use of v1 is gone
([M-module-4](https://github.com/permitio/permit-golang/blob/master/MIGRATION.md#M-module-4)).

## 3. Scan

```sh
scanner="$(mktemp -d)/scanner"
GOWORK=off go -C <skill> build -o "$scanner" ./scanner
"$scanner" -guide "$guide" . > "$(dirname "$scanner")/hits.json"
```

The scanner needs only the Go standard library; `GOWORK=off` builds it even when a directory
above `<skill>` has a `go.work` file. It exits 0 when each use of v1 has an entry, 1 when a use
has none (`"mapped": false`), and 2 when it did not run: its last line on standard error says
why; fix that and scan again. Each hit has `file`, `line` and `column`, `kind` (`import` or
`identifier`), `v1` (the import path or the v1 identifier, such as `api.Users.Create`, or
`models.UserCreate.SetEmail` for a method or a field of a v1 models value), `ids` (the IDs of its
entries) and, for a v1 models type that v2 keeps under its name, `v2`. `go -C <skill> doc
./scanner` says how it finds each use, and what it cannot follow.

## 4. Map each hit to its entries

- Read each entry in `ids`: the section of `$guide` that starts at `<a id="<ID>"></a>`. Its
  `v2:` line names the replacement and its text says what else changes. Many hits share an
  entry: read it once.
- An `import` hit maps to
  [M-module-1](https://github.com/permitio/permit-golang/blob/master/MIGRATION.md#M-module-1):
  `pkg/permit`, `pkg/api`, `pkg/config`, `pkg/enforcement`, `pkg/errors` and `pkg/openapi` become
  `github.com/permitio/permit-golang/v2` (package `permit`), and `pkg/models` becomes
  `github.com/permitio/permit-golang/v2/models`.
- A hit of pkg/models (a type, a constructor, a constant, or `models.<Type>.<Name>`, a method or
  a field of a models value) has the entries of its type. v2's models are structs without
  getters or setters: `SetName(v)` becomes an assignment to the field `Name`, and `GetName()` a
  read of it, in the field's v2 type. v2 names a constant after its type, such as
  `models.AttributeTypeString` for `models.STRING`.
- A hit with `v2` and no `ids` is of a models type that v2 keeps under its name. Build it as a
  struct literal, as the entry of the call that takes it says; `go doc
  github.com/permitio/permit-golang/v2/models <Type>` lists its fields and constants.
- For any hit with no `ids`, first search the guide for its name, as some entries name a models
  type or constant only in their text: `grep -n 'models.STRING' "$guide"`.
- A hit with `"mapped": false` that the search does not explain has no entry. Do not guess a
  replacement: read v2's API with `go doc github.com/permitio/permit-golang/v2`, and when no
  replacement is clear, stop and ask.
- Calls through `pkg/openapi`, v1's generated client, are not hits: `go build` lists them in
  step 7, and the entries whose `v1:` lines name `openapi.<Service>.<Operation>` say what
  replaces each.

## 5. Edit

Change the imports first, then each hit by its entry; lines move as you edit, so work from the
end of each file up, or scan again. Keep what the code did: where an entry says v2 behaves
differently, such as an error, a default or a request, adapt the caller and say so in your
summary.

## 6. STOP at each SyncUser: v2's Sync replaces the whole user

**For each `SyncUser` in the code, a hit on `permit.Client.SyncUser` or `api.Users.SyncUser`
([M-users-5](https://github.com/permitio/permit-golang/blob/master/MIGRATION.md#M-users-5)) or
not: v1's `SyncUser` kept each field that the call left out; v2's `client.Users.Sync` clears it.
An empty `Email`, `FirstName` or `LastName`, or nil `Attributes`, removes what the stored user
had. Never rename `SyncUser` to `Sync` and stop there.**

The scanner does not follow a client that another package declares, so find each call by text
too, `grep -rn 'SyncUser' --include='*.go' .`, and treat each `go build` error that names
`SyncUser` the same way.

- Give `Sync` every field of the user, from where the code keeps them, or read the user with
  `client.Users.Get` first; or use `client.Users.Update` to change some fields and keep the rest.
- Check what the call does with the user's roles, as the entry says.
- List each `SyncUser` call site you changed, and how, in your summary.

## 7. Remove v1 and verify

```sh
go mod tidy
go build ./...
go vet ./...
go test ./...
"$scanner" -guide "$guide" .
```

`go mod tidy` drops v1 once nothing imports it. `go build` also finds what the scanner cannot
follow, and names the type in its error: a method or a field of a value that a v1 API call
returns, such as `GetEmail()` on the user that `Users.Get` returned; a v1 value that another
package of the module hands over; a call through one of v1's interfaces; and a call through
`pkg/openapi`. Fix each error by the entries of the type it names. Repeat until each command
passes and the scan exits 0 with `"hits": []`. The scanner reads nested modules too: build and
vet each of them.
