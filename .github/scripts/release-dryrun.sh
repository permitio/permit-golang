#!/usr/bin/env bash
# release-dryrun.sh <sha> runs the checks of a release that need no release: those of the tree,
# as if the commit <sha>, which must be HEAD, were released. The release-dryrun job of
# .github/workflows/ci.yml runs it on every pull request, with the pull request's head checked
# out. It never tags, creates a release or reads a secret, and needs no network. From the
# repository root:
#
#   validate   bin/releasecheck validate, as the validate job of release.yml runs it, with the
#              tag v<Version>-rc.1, a release candidate of the Version of version.go, whose
#              changelog section may still say (unreleased); <sha> as the commit and as the head
#              of master; and two inputs that only a release has, made up to pass: a run of CI
#              for a push of <sha> that succeeded, and harness evidence of <sha> on the Go
#              releases that bin/releasecheck lanes prints, those of the GO_FLOOR and GO_<n>
#              variables of the env of .github/workflows/ci.yml, which validate requires, with
#              each phase of tools/e2e/local-only.txt passed. So the tree decides: the version,
#              the changelog, tools/e2e/pending.txt and the lists that validate reads.
#   migration  bin/apisurface -mode migration: MIGRATION.md covers the v1 API.
#   migration-pending
#              tools/migration/pending.txt has no line other than blank lines and comments,
#              lines whose first field starts with #, as apisurface -mode release requires.
#   zipcheck   bin/zipcheck -version v<Version>-dryrun -out <zip>: builds the module zip of HEAD
#              for a version that is never published, checks it, and prints its h1: hash.
#   verify     .github/scripts/release-verify.sh <module>@v<Version>-dryrun <h1>, as the verify
#              job of release.yml runs it, against a module proxy in a directory that serves the
#              zip, GOPROXY=file://<dir>, with GOSUMDB=off and one attempt; it runs only when
#              zipcheck passed and printed the hash.
#
# Each check runs whatever the others did, and the script prints each check's result. It needs
# git, jq and unzip, and the release-verify.sh beside it.
#
# Exit codes: 0 when every check passed; 1 when a check failed, such as when the proxy serves a
# zip whose hash is not the one zipcheck printed; 2 when no check failed but one did not run, or
# when the script did not run: the argument is not a commit hash or not HEAD, version.go has no
# Version, releasecheck lanes prints no Go release, or a tool is missing. What the checks print
# is printed with workflow commands stopped.
set -euo pipefail

token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
echo "::stop-commands::$token"
work=''
trap 'echo "::$token::"; if [[ -n $work ]]; then rm -rf -- "$work"; fi' EXIT

fail() {
  echo "release-dryrun: $*"
  echo "release-dryrun: DID NOT RUN"
  exit 2
}

[[ $# -eq 1 ]] || fail "usage: release-dryrun.sh <sha>"
sha=$1
[[ $sha =~ ^[0-9a-f]{40}$ ]] || fail "\"$sha\" is not a full commit hash"
root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
bin=$root/bin
for tool in git jq unzip; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is not installed"
done
for tool in releasecheck apisurface zipcheck; do
  [[ -x $bin/$tool ]] || fail "$tool is not at $bin/$tool; build it with" \
    "go -C tools build -o ../bin/$tool ./cmd/$tool"
done
head=$(git -C "$root" rev-parse --verify HEAD 2>&1) || fail "git cannot read HEAD: $head"
[[ $sha == "$head" ]] || fail "$sha is not HEAD, $head, whose tree zipcheck builds: check out" \
  "$sha first"
version=$(sed -n 's/^const Version = "\([^"]*\)"$/\1/p' "$root/version.go")
[[ -n $version && $version != *$'\n'* ]] ||
  fail "version.go has no line const Version = \"<version>\""
code=0
lanes=$("$bin/releasecheck" lanes -root "$root") || code=$?
if [[ $code -ne 0 || -z $lanes ]]; then
  printf '%s\n' "$lanes"
  fail "releasecheck lanes printed no Go release of a test leg of .github/workflows/ci.yml"
fi
work=$(mktemp -d "${TMPDIR:-/tmp}/release-dryrun.XXXXXX") ||
  fail "creating a temporary directory failed"

tag=v$version-rc.1
zipversion=v$version-dryrun
jq -n --arg sha "$sha" '{total_count: 1, workflow_runs: [{id: 1, run_attempt: 1,
  head_sha: $sha, event: "push", status: "completed", conclusion: "success"}]}' \
  >"$work/ci-runs.json"
lanes_json=$(printf '%s\n' "$lanes" | jq -R . | jq -sc .)
phases_json=$(awk 'NF && $1 !~ /^#/ { print $3 }' "$root/tools/e2e/local-only.txt" | sort -u |
  jq -R . | jq -sc .)
# A dry run has no report of the local suite; its report_sha256 is zeros.
jq -n --arg sha "$sha" --arg sum "$(printf '0%.0s' {1..64})" --argjson lanes "$lanes_json" \
  --argjson phases "$phases_json" \
  '{sdk_commit: $sha, report_sha256: $sum, lanes: $lanes, passed_phases: $phases}' \
  >"$work/evidence.json"

failed=''
skipped=''
# record <check> <exit code> records a check's result by its exit code.
record() {
  case $2 in
  0) echo "release-dryrun: $1 passed" ;;
  1) failed+=" $1" ;;
  *) skipped+=" $1 (exit $2)" ;;
  esac
}

echo "release-dryrun: validate $tag at $sha"
code=0
"$bin/releasecheck" validate -root "$root" -tag "$tag" -sha "$sha" -master "$sha" \
  -ci-runs "$work/ci-runs.json" -evidence "$work/evidence.json" || code=$?
record validate "$code"

echo "release-dryrun: migration"
code=0
"$bin/apisurface" -mode migration -root "$root" || code=$?
record migration "$code"

echo "release-dryrun: migration-pending"
code=0
pending=$(awk 'NF && $1 !~ /^#/ { n++ } END { print n + 0 }' \
  "$root/tools/migration/pending.txt") || code=2
if [[ $code -eq 0 && $pending -gt 0 ]]; then
  echo "release-dryrun: tools/migration/pending.txt still lists v1 items (lines that are not" \
    "blank or a comment: $pending): a release needs every v1 item to have its entry in" \
    "MIGRATION.md, as apisurface -mode release checks; write the entries and remove the lines"
  code=1
fi
record migration-pending "$code"

echo "release-dryrun: zipcheck $zipversion"
code=0
"$bin/zipcheck" -repo "$root" -version "$zipversion" -out "$work/module.zip" |
  tee "$work/zipcheck.txt" || code=$?
record zipcheck "$code"

echo "release-dryrun: verify"
line=$(head -n 1 "$work/zipcheck.txt")
pattern='^zipcheck: ([^@[:space:]]+)@([^[:space:]]+) (h1:[A-Za-z0-9+/]{43}=)$'
if [[ $code -ne 0 ]]; then
  echo "release-dryrun: verify does not run, as zipcheck did not pass"
  skipped+=" verify"
elif [[ ! $line =~ $pattern || ${BASH_REMATCH[2]} != "$zipversion" ]]; then
  echo "release-dryrun: zipcheck printed no h1: hash of the module at $zipversion"
  record verify 2
else
  module=${BASH_REMATCH[1]}
  h1=${BASH_REMATCH[3]}
  at=$work/proxy/$module/@v
  mkdir -p "$at"
  printf '%s\n' "$zipversion" >"$at/list"
  printf '{"Version": "%s", "Time": "%s"}\n' "$zipversion" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    >"$at/$zipversion.info"
  unzip -p "$work/module.zip" "$module@$zipversion/go.mod" >"$at/$zipversion.mod"
  cp -- "$work/module.zip" "$at/$zipversion.zip"
  code=0
  GOPROXY=file://$work/proxy GOSUMDB=off RELEASE_VERIFY_ATTEMPTS=1 ZIPCHECK=$bin/zipcheck \
    "$root/.github/scripts/release-verify.sh" "$module@$zipversion" "$h1" || code=$?
  record verify "$code"
fi

if [[ -n $skipped ]]; then
  echo "release-dryrun: did not run:$skipped"
fi
if [[ -n $failed ]]; then
  echo "release-dryrun: FAIL:$failed"
  echo "release-dryrun: FAIL"
  exit 1
fi
if [[ -n $skipped ]]; then
  echo "release-dryrun: DID NOT RUN"
  exit 2
fi
echo "release-dryrun: PASS"
