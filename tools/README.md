# Tools

`tools/` is a separate Go module, `github.com/permitio/permit-golang/v2/tools`, that holds the
programs and version pins used to build, test and check the SDK. Its requirements stay out of the
SDK's `go.mod`, so they never reach an application that imports the SDK, and `go test ./...` at
the repository root does not run its tests.

| Path | What it is |
| --- | --- |
| `go.mod`, `go.sum` | `tool` directives that pin govulncheck, apidiff, gorelease and editorconfig-checker, and the requirement on `go.yaml.in/yaml/v3`, the YAML parser that cicheck uses. The `replace` directive points the SDK module at the repository root, so code here can import the SDK's packages, `internal/` ones included. |
| `cmd/checkgotest` | Reads `go test -json` output and fails a run in which a test failed or skipped, no test passed, no test ran in one of the input files, or a required test did not run. |
| `cmd/cicheck` | Checks the workflows in `.github/workflows` and the scripts in `.github/scripts` against the rules CI depends on, and prints the pinned actions and release binaries. The CI section of [CONTRIBUTING.md](../CONTRIBUTING.md#ci) says how to run it. |
| `cmd/mutationgate` | Runs mutation tests with gremlins on the changed Go lines, or every Go file, of the packages in the directories it is given, with every test process bounded in time, memory and output, and fails when the tests detect fewer than a threshold of the mutants. [CONTRIBUTING.md](../CONTRIBUTING.md#mutation-tests) says how to run it; it is the only way to run mutation tests here. |
| `cmd/vulnscan` | Runs govulncheck on the SDK's floor and ceiling and on this module, and fails on a vulnerability outside the standard library that `.github/vulnscan-accept.txt` does not accept. |
| `cmd/secreport` | Decides the result of a CI job's security scans, and writes the job summary, the annotations and the pull request comment. The [vulnerability scan](../CONTRIBUTING.md#vulnerability-scan) section of CONTRIBUTING.md says how both run. |
| `scripts` | Tests of the shell scripts in `.github/scripts`, and of `.editorconfig` with the pinned editorconfig-checker. |
| `binaries.lock` | Release binaries that are not Go modules, pinned by version and by sha256 for each platform. |

Check this module the way you check the root module, with `go -C tools`:

```sh
go -C tools vet ./...
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
