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
- that `go mod tidy -diff` prints nothing, in the root and in `tools/`;
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
own commit, and `testdata/api-surface.txt` *(added in a later commit on this branch)* stays
byte-identical in that commit.

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

## CI

`.github/workflows/ci.yml` runs every check. Its last job, `CI`, is the one check the ruleset
requires: it fails unless every job it needs succeeded, or was not selected for the run.

| Job | What it does |
| --- | --- |
| `prek` | Checks the workflows with cicheck, runs every git hook on every file through `.github/scripts/prek-count.sh`, and on a pull request checks the message of each of its commits through `.github/scripts/commits-check.sh`. |
| `test` | Runs the tests of both modules with the race detector, offline, and checks their output with checkgotest. Five legs: Go 1.26.0, the minimum, on the dependency versions that `go.mod` and `go.sum` pin; and the newest patch of Go 1.26 and of Go 1.27 that is at least 7 days old, on those versions and on the newest version of every dependency. Each leg checks that it runs the Go version it names. |
| `crossbuild` | Builds the SDK for linux, darwin and windows on amd64 and arm64. |
| `vuln` | Scans the SDK and `tools/` for known vulnerabilities through `.github/scripts/vuln.sh`; see [Vulnerability scan](#vulnerability-scan). |
| `secrets` | Scans the commits that are new in the run for secrets with gitleaks, through `.github/scripts/gitleaks-range.sh`; see [Secret scan and dependency review](#secret-scan-and-dependency-review). |
| `mutation` | Pull requests only: mutation tests of the Go lines the pull request changed, in both modules, as [Mutation tests](#mutation-tests) describes. Fails when the tests detect fewer than 90% of the mutants that ran. |
| `dependency-review` | Pull requests only: fails when the pull request adds a dependency with a known vulnerability of high or critical severity; see [Secret scan and dependency review](#secret-scan-and-dependency-review). |
| `prek-upstream` | Weekly: runs every hook with prek's built-in versions of the pre-commit-hooks hooks and with the upstream hooks (`PREK_NO_FAST_PATH=1`), and fails when they report a hook differently. |
| `mutation (weekly)` | Weekly: mutation tests of every Go file of both modules, with no threshold; it reports the efficacy in the job summary and fails only when the run does not complete. |
| `CI` | Decides the result with `.github/scripts/ci-aggregate.sh`. |

Which jobs a run selects depends on its mode:

- A pull request into any branch and a push to `master` run the pull request checks: `prek`,
  `test`, `crossbuild`, `vuln` and `secrets`. A pull request runs `mutation` and
  `dependency-review` too.
- A manual run (`workflow_dispatch`) takes a mode: `pr` runs the pull request checks, `weekly` the
  weekly jobs, and `security` and `e2e` the jobs of those modes *(added in later commits on this
  branch)*. The release workflow *(added in a later commit on this branch)* calls CI with the
  mode `release`, which runs the pull request checks.
- Two crons start the periodic modes: `SECURITY_CRON` twice a week and `WEEKLY_CRON` once a week.
  Each cron is listed in `on.schedule` and set once more in the workflow's `env`, as
  `<MODE>_CRON`, which cicheck and ci-aggregate.sh read.

The `CI` job fails when a job it needs failed or was cancelled, or was skipped in a run that
selects it, and passes a job that the run does not select. Three lists in its step change that:
`PULL_REQUEST_JOBS` run on pull requests only, each job in `SCHEDULED_JOBS` (`job:mode`) runs in
its mode only, and a job in `ADVISORY_JOBS` may fail without failing CI. ci-aggregate.sh exits 0
when CI passes, 1 when a job did not succeed, and 2 when it could not decide: the job results
cannot be read, there are not `EXPECTED_JOBS` of them, a list names a job CI does not need, or
the mode, event or cron is unknown.

When you add a job, add it to `CI`'s `needs` and change `EXPECTED_JOBS`; when it runs only on
pull requests, on a cron or as an advisory check, add it to the list for that. Give the job the
`if:` of its kind, exactly as `ci.yml` writes it:

- a pull request check, advisory or not:
  `(!inputs.mode && (github.event_name == 'pull_request' || github.event_name == 'push')) || inputs.mode == 'pr' || inputs.mode == 'release'`;
- a job in `PULL_REQUEST_JOBS`: `!inputs.mode && github.event_name == 'pull_request'`;
- a job in `SCHEDULED_JOBS`:
  `(!inputs.mode && github.event.schedule == '<cron>') || inputs.mode == '<mode>'`, with the cron
  its `<MODE>_CRON` declares. A run that another workflow calls sees its caller's schedule, so
  `!inputs.mode` keeps the cron from selecting the job in a run that has a mode.

When you add or remove a git hook, change `EXPECTED_HOOKS`.

cicheck checks these rules and the workflows' hardening: every action pinned to a commit SHA with
a version comment, `persist-credentials: false`, `cache: false` and an exact Go version on every
`setup-go`, a pinned runner and a timeout on every job, no `${{` in a `run:`, and the rest that
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
EXPECTED_HOOKS=25 .github/scripts/prek-count.sh  # the EXPECTED_HOOKS of ci.yml's env
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
- never mutates comments, blank lines, the package clause or imports, so a change of only those
  passes with nothing to mutate;
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
SDK, are tested like SDK code. Maintainers push a pull request's next commit only after CI
passed on the previous one, so that each commit's lines get a run. The weekly job tests every Go
file of both modules and only reports.

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
`.github/vulnscan-trees.txt` lists it with the reason it never ships; a module that is neither
makes the scan exit 2. Trivy gets every setting on its command line, and cicheck fails on a
committed `trivy.yaml`, `.trivyignore` or `.trivyignore.yaml` anywhere in the repository.

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
them in the script as well; a test checks that they match.

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
and docs use fake keys that start with `permit_key_FAKE`.

The `dependency-review` job runs on pull requests only. It compares the dependencies of the pull
request with those of its base in GitHub's dependency graph, and fails when the pull request adds
a dependency with a known vulnerability of high or critical severity; on a failure it also
comments a summary on the pull request. It has no local command; locally, the [vulnerability
scan](#vulnerability-scan) checks the dependencies of both modules.

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
  end-to-end tests against Permit *(added in a later commit on this branch)* do not run on forks.

## Public repository rules

This repository is public.

- Never commit secrets: API keys, tokens, `.env` files, private keys or certificates.
  `.gitignore` covers the usual file names (`.env*` except `.env.example`, `*.pem`, `*.key`,
  `*.p12`, `*.pfx`, `*.tfstate*`, `credentials*`). Tests, examples and docs use fake values: API
  keys that look like `permit_key_FAKE...`, and `example.com` for hosts and email addresses.
- Commit messages, code, comments, tests and pull request text refer to internal work only by
  ticket ID (`PER-<id>`), and name no internal hostnames, private repository names, customers or
  paths on your machine.
