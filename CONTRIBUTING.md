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
  keys: use fake keys that start with `permit_key_FAKE`);
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
| `prek-upstream` | Weekly: runs every hook with prek's built-in versions of the pre-commit-hooks hooks and with the upstream hooks (`PREK_NO_FAST_PATH=1`), and fails when they report a hook differently. |
| `CI` | Decides the result with `.github/scripts/ci-aggregate.sh`. |

Which jobs a run selects depends on its mode:

- A pull request into any branch and a push to `master` run the pull request checks: `prek`,
  `test` and `crossbuild`.
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
mean, and the tests in `tools/scripts` run them; those tests need bash, git, jq, diff and od.

```sh
EXPECTED_HOOKS=25 .github/scripts/prek-count.sh  # the EXPECTED_HOOKS of ci.yml's env
.github/scripts/commits-check.sh "$(git merge-base origin/master HEAD)" "$(git rev-parse HEAD)"
```

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
