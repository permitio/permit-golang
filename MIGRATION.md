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

## API

No entries yet.

## Wire behaviour

No entries yet.

## Kept on purpose

What v2 keeps from v1 although a new major version could change it:

- The name of the package: v2's root package is `permit`, as v1's `pkg/permit` was, so code that
  refers to `permit.` keeps its qualifier.

## Appendix: v1 models types

Each type of v1's `pkg/models` and the v2 type or entry that replaces it.

<!-- apisurface:models:begin -->
<!-- apisurface -mode migration -write writes this table; do not edit it. -->

Not mapped yet: 384 of the 384 types of v1's pkg/models.

<!-- apisurface:models:end -->
