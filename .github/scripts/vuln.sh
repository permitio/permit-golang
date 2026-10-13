#!/usr/bin/env bash
# vuln.sh scans the SDK and the tools module for known vulnerabilities, then reports the result
# and decides it with secreport (tools/cmd/secreport). The vuln job in .github/workflows/ci.yml
# runs it, and it runs the same way on any clone. It needs the network: it downloads Trivy, the
# vulnerability databases and, for the ceiling, the newest version of every dependency.
#
# It builds bin/govulncheck (the version tools/go.mod pins), bin/vulnscan and bin/secreport,
# installs bin/trivy with fetch-tool.sh, and copies the repository without .git and bin/ to a
# temporary directory, in which go get -u -t ./... and go mod tidy move every dependency of the
# root module to its newest version: the ceiling. Then it runs four scans:
#
#   govulncheck    vulnscan on the floor (the root module as committed), the ceiling and tools/
#   trivy-floor    trivy.sh block on the root module
#   trivy-ceiling  trivy.sh block on the ceiling
#   trivy-tools    trivy.sh report on tools/
#
# Everything it prints until the scans end is inside a stop-commands block, so that no scanner
# output can issue a workflow command; the annotations secreport prints come after it. The
# scanners and the commands that prepare them run without GH_TOKEN and GITHUB_TOKEN; only
# secreport gets GH_TOKEN, to comment on the pull request. vuln.sh passes secreport these
# variables, which the CI job sets, when they are set:
#
#   GITHUB_STEP_SUMMARY                                     the job summary file
#   GITHUB_SERVER_URL, GITHUB_REPOSITORY and GITHUB_RUN_ID  the run's URL
#   PR_NUMBER                                               the pull request's number
#   PR_HEAD_REPO                                            its head repository's owner/name
#
# Exit codes are secreport's: 0 when every scan passed, 1 when a scan failed and every scan ran,
# 2 when a scan did not run or the report could not be written. vuln.sh exits 2 itself when it was
# given arguments, or could not build the programs, install Trivy or prepare the ceiling.
set -euo pipefail

token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
echo "::stop-commands::$token"
trap 'echo "::$token::"' EXIT

fail() {
  echo "vuln: $*"
  echo "vuln: DID NOT RUN"
  exit 2
}

[[ $# -eq 0 ]] || fail "usage: vuln.sh (it takes no arguments)"
root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
cd -- "$root"
gh_token=${GH_TOKEN:-}
unset GH_TOKEN GITHUB_TOKEN
work=$(mktemp -d "${TMPDIR:-/tmp}/vuln.XXXXXX") || fail "creating a temporary directory failed"
trap 'rm -rf -- "$work"; echo "::$token::"' EXIT

echo "vuln: building govulncheck, vulnscan and secreport from tools/go.mod"
go -C tools build -o "$root/bin/govulncheck" golang.org/x/vuln/cmd/govulncheck ||
  fail "building govulncheck failed"
go -C tools build -o "$root/bin/vulnscan" ./cmd/vulnscan || fail "building vulnscan failed"
go -C tools build -o "$root/bin/secreport" ./cmd/secreport || fail "building secreport failed"
.github/scripts/fetch-tool.sh trivy >/dev/null ||
  fail "installing Trivy with .github/scripts/fetch-tool.sh failed"

echo "vuln: preparing the ceiling: the root module with every dependency at its newest version"
ceiling=$work/ceiling
mkdir "$ceiling"
tar -C "$root" --exclude ./.git --exclude ./bin -cf - . | tar -C "$ceiling" -xf - ||
  fail "copying the repository to the ceiling failed"
(cd -- "$ceiling" && go get -u -t ./... && go mod tidy) ||
  fail "moving the dependencies of the ceiling to their newest versions failed"

results=$work/results
mkdir "$results"
# scan <name> <directory> <command>... runs a scan in the directory, prints what it printed, and
# records that and its exit code for secreport.
scan() {
  local name=$1 dir=$2 code=0
  shift 2
  echo "vuln: $name"
  (cd -- "$dir" && "$@") >"$results/$name.out" 2>&1 || code=$?
  cat -- "$results/$name.out"
  echo "$code" >"$results/$name.exit"
}
scan govulncheck "$root" bin/vulnscan -ceiling "$ceiling"
scan trivy-floor "$root" .github/scripts/trivy.sh block .
# From the temporary directory, so the report names the ceiling's go.mod ceiling/go.mod.
scan trivy-ceiling "$work" "$root/.github/scripts/trivy.sh" block ceiling
scan trivy-tools "$root" .github/scripts/trivy.sh report tools

echo "::$token::"
trap 'rm -rf -- "$work"' EXIT
report=(bin/secreport -id vuln -title Vulnerabilities -dir "$results")
if [[ -n ${GITHUB_STEP_SUMMARY:-} ]]; then
  report+=(-summary "$GITHUB_STEP_SUMMARY")
fi
if [[ -n ${GITHUB_SERVER_URL:-} && -n ${GITHUB_REPOSITORY:-} && -n ${GITHUB_RUN_ID:-} ]]; then
  report+=(-run-url "$GITHUB_SERVER_URL/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID")
fi
if [[ -n ${PR_NUMBER:-} ]]; then
  report+=(-pr "$PR_NUMBER" -repo "${GITHUB_REPOSITORY:-}" -head-repo "${PR_HEAD_REPO:-}")
fi
code=0
GH_TOKEN=$gh_token "${report[@]}" govulncheck trivy-floor trivy-ceiling trivy-tools || code=$?
exit "$code"
