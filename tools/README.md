# Tools

`tools/` is a separate Go module, `github.com/permitio/permit-golang/v2/tools`, that holds the
programs and version pins used to build, test and check the SDK. Its requirements stay out of the
SDK's `go.mod`, so they never reach an application that imports the SDK, and `go test ./...` at
the repository root does not run its tests.

| Path | What it is |
| --- | --- |
| `go.mod`, `go.sum` | `tool` directives that pin govulncheck, apidiff, gorelease and editorconfig-checker, the requirement on `go.yaml.in/yaml/v3`, the YAML parser that cicheck uses, the one on `golang.org/x/mod`, whose `zip` package builds the module zip for zipcheck, and the one on `golang.org/x/perf`, whose `benchfmt` package, benchstat's parser, reads benchmark output for benchreport. The `replace` directive points the SDK module at the repository root, so code here can import the SDK's packages, `internal/` ones included. |
| `license_test.go` | `TestRepositoryLicense`, which checks that `LICENSE` is the MIT License word for word with the SDK's copyright line. |
| `cmd/checkgotest` | Reads `go test -json` output and fails a run in which a test failed or skipped, no test passed, no test ran in one of the input files, or a required test did not run. |
| `cmd/cicheck` | Checks the workflows in `.github/workflows` and the scripts in `.github/scripts` against the rules CI depends on, checks that `.github/dependabot.yml` and the tidy hooks cover every Go module, and prints the pinned actions and release binaries. The CI section of [CONTRIBUTING.md](../CONTRIBUTING.md#ci) says how to run it. |
| `cmd/mutationgate` | Runs mutation tests with gremlins on the changed Go lines, or every Go file, of the packages in the directories it is given, with every test process bounded in time, memory and output, and fails when the tests detect fewer than a threshold of the mutants. [CONTRIBUTING.md](../CONTRIBUTING.md#mutation-tests) says how to run it; it is the only way to run mutation tests here. |
| `cmd/vulnscan` | Runs govulncheck on the SDK's floor and ceiling and on this module, or on one of them or a module outside the repository, and fails on a vulnerability outside the standard library that `.github/vulnscan-accept.txt` does not accept. It can write its findings for secreport. |
| `cmd/secreport` | Decides the result of a CI job's security scans, and writes the job summary, the annotations, the pull request comment and the report for Slack. The [vulnerability scan](../CONTRIBUTING.md#vulnerability-scan) section of CONTRIBUTING.md says how both run, and [scheduled scans and alerts](../CONTRIBUTING.md#scheduled-scans-and-alerts) how the scheduled report works. |
| `cmd/zipcheck` | Builds the module zip of the commit `HEAD` names, the way the Go module proxy builds it, and fails when a file is not on its allow list or is over its cap, when `LICENSE` is missing, or when the zip is over its compressed budget; it prints the 10 largest files. The limits are in `cmd/zipcheck/limits.go`. [CONTRIBUTING.md](../CONTRIBUTING.md#the-module-zip) says how to run it and how to raise a limit. |
| `cmd/benchreport` | Compares a result of the decision benchmarks with the previous one, and reports each benchmark whose median `ns/op` or `allocs/op` is more than 20% higher; a missing previous result is reported, not failed. [CONTRIBUTING.md](../CONTRIBUTING.md#benchmarks) says how to run it. |
| `cmd/notify` | Posts a one-line notice, and a report under it, to a Slack channel's incoming webhook, and fails when Slack does not accept it. [CONTRIBUTING.md](../CONTRIBUTING.md#posting-a-notice) says how to run it. |
| `cmd/specsnap` | Writes the snapshots in `specs`: fetches a spec, pre-fixes the constructs of the control plane's spec that are not valid OpenAPI 3.1, writes it in canonical form and records it in `specs/lock.json`. It also checks the snapshots against the lock, and the published specs and the container PDP's latest image against the snapshots. [CONTRIBUTING.md](../CONTRIBUTING.md#spec-snapshots) says how to run it. |
| `drift` | The drift tests of package `models`, and the walker of the part of JSON Schema that the specs use, with which the end-to-end fake checks request bodies too: `model-map.yaml` maps each model to its schema in `specs`, and the tests check each model and its strict round trip, the models of each operation of the registry, and each reply fixture in the root's `testdata/replies` against the specs, and the routes on which the PDP container waits for sync against the root's `doc.go`, the registry and `coverage.yaml`; `allow.yaml` accepts the findings that known defects of the specs cause. [CONTRIBUTING.md](../CONTRIBUTING.md#model-drift) says how to write a row of the map and an entry of the allowlist. |
| `cmd/apicoverage` | Reads the records of the requests that the tests send and checks them, with `specs/coverage.yaml`, against the specs and the operation map: it fails on an undecided GA operation, a stale or untested row, a stage change and a request that no row explains, among others, and reports each covered operation's methods and end-to-end state. [CONTRIBUTING.md](../CONTRIBUTING.md#api-coverage) says how to run it. |
| `specs` | The snapshots of the control plane's, the cloud PDP's and the container PDP's specs, with their lock, the routes the container PDP serves without listing them, the routes v1.2.8 sent with the ones v2 changes, and `coverage.yaml`, the SDK's decision for every operation. [CONTRIBUTING.md](../CONTRIBUTING.md#spec-snapshots) describes each file. |
| `cmd/apisurface` | Writes the v1 surface from the module zip of v1.3.0, which the checksum database verified; checks that `MIGRATION.md` or `migration/pending.txt` covers each part of it once, and that no entry ID is removed (migration mode); finds the breaking changes since a base commit with apidiff, the validation rules and the model map, and fails on one without the breaking label (diff mode); and checks a release's version with gorelease, and that nothing is pending (release mode). It reads v2's API from `testdata/api-surface.txt`, so it builds without the SDK's packages. [CONTRIBUTING.md](../CONTRIBUTING.md#public-surface-and-migration) says how to run it. |
| `e2e` | The end-to-end scenarios, which build with the tag `e2e`, and their completeness gate: `local-only.txt`, `advisory.txt` and `pending.txt` give each public method that no cloud scenario covers its end-to-end state, `keys.yaml` says which API keys the cloud lane has, and `required.txt` lists the tests that every run must pass. `framework` creates and checks what the scenarios need in plain `net/http`, with a ledger of what a run must delete, and `framework/surface` checks the states; `fake` is the stateful fake of the Permit API that the dry run runs against. [CONTRIBUTING.md](../CONTRIBUTING.md#end-to-end-tests) says how to run them. |
| `migration` | `v1-surface.txt`, the parts of the v1 API that `MIGRATION.md` must cover; `pending.txt`, the parts whose entry is not written yet, each with the area of v2 that will write it; and `ids.txt`, every entry ID of `MIGRATION.md`, which may only grow. |
| `internal/execfile` | Writes a file that the program, or a process it starts, runs next, while the program starts no other process, so that running it on Linux does not fail with "text file busy". Every file that the programs in this module and their tests write and then run is written with it. |
| `scripts` | Tests of the shell scripts in `.github/scripts`, and of `.editorconfig` with the pinned editorconfig-checker. |
| `binaries.lock` | Release binaries that are not Go modules, pinned by version and by sha256 for each platform. |

Check this module the way you check the root module, with `go -C tools`. The tests run offline,
so first download the modules and the gitleaks that the tests of `gitleaks-range.sh` run:

```sh
go -C tools vet ./...
go -C tools mod download
.github/scripts/fetch-tool.sh gitleaks
HTTP_PROXY=http://127.0.0.1:9 HTTPS_PROXY=http://127.0.0.1:9 \
  go -C tools test -race -count=1 -shuffle=on -p 2 -timeout 5m ./...
go -C tools mod tidy -diff
```

## Go tools

Run a pinned tool from the repository root with `-modfile`. The go command builds the tool from
this module, and the tool runs in the root, so it checks the SDK:

```sh
go tool -modfile=tools/go.mod govulncheck ./...
```

apidiff, gorelease and editorconfig-checker, which a prek hook runs, run the same way. To move a
tool to a new version, choose a release that is at least 7 days old:

```sh
go -C tools get -tool golang.org/x/vuln/cmd/govulncheck@v1.8.0
go -C tools mod tidy
```

## checkgotest

`go test` exits 0 when a test skips, when `-run` matches no test, and when a build constraint
leaves a package out. checkgotest reads the `go test -json` output, prints the test output and
what it found, and exits 0 when every test passed, 1 when a test failed or skipped, and 2 when no
test passed, no test ran in one of the input files, a required test did not run or the input is
not `go test -json` output. `go -C tools doc ./cmd/checkgotest` describes every case.

Build it before you use it, because `go run` reports every non-zero exit as 1. Check the output
of both modules together, since `.github/required-tests.txt` names tests in both:

```sh
go -C tools build -o ../bin/checkgotest ./cmd/checkgotest
go test -race -count=1 -shuffle=on -json ./... >bin/root.json
go -C tools test -race -count=1 -shuffle=on -json ./... >bin/tools.json
bin/checkgotest -require-file .github/required-tests.txt \
  -allow-skips .github/allowed-skips.txt bin/root.json bin/tools.json
```

- `.github/required-tests.txt` lists the tests that must run and pass. Add a test there when a
  check depends on it running. A package that `-run` filters out or a build constraint leaves
  out, in a file in which other tests ran, is caught only when one of its tests is listed here.
- `.github/allowed-skips.txt` lists the tests that may skip. Each entry names the ticket that
  tracks the skip, the last day the skip is allowed and what must happen before the test can run
  again. An entry that has expired, or whose test did not skip, fails the run. The headers of
  both files describe their format.

## Release binaries

`.github/scripts/fetch-tool.sh <name>` downloads the release that `binaries.lock` pins for the
machine it runs on, checks its sha256 before it unpacks anything, installs the binary as
`bin/<name>` and prints that path. It exits 2 when the lock is missing, has no complete entry
for the name or this platform, `bin/<name>` exists and is not a regular file, the download
fails, the sha256 does not match, or the binary cannot be unpacked or installed. It needs bash,
curl, tar and `sha256sum` or `shasum`.

To add a binary or move one to a new version:

1. Choose a release that is at least 7 days old.
2. Write its entry in the format the header of `binaries.lock` describes. List the binary's path
   inside the archive as `tar -tzf` prints it.
3. Copy the sha256 for darwin/arm64, darwin/amd64, linux/amd64 and linux/arm64 from the
   release's checksums file, and compare at least one with `shasum -a 256` of a file you
   downloaded.
4. Run `.github/scripts/fetch-tool.sh <name>`, then run the binary it installed.

Dependabot does not read `binaries.lock`, so its versions change only by hand.
