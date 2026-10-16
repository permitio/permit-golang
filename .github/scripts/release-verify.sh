#!/usr/bin/env bash
# release-verify.sh <module>@<version> <h1> checks a published version as an application gets it.
# It downloads the version with go mod download from the module proxy that GOPROXY names, which
# checks it against the checksum database that GOSUMDB names; checks that the h1: hash of the zip
# the proxy serves is <h1>, the hash of the zip that the release workflow built from the tagged
# commit before it published the version; and checks that zip with zipcheck -zip. The release
# workflow (.github/workflows/release.yml) runs it once the version is published, with
# GOPROXY=https://proxy.golang.org and GOSUMDB=sum.golang.org. With GOSUMDB=off, as against a
# local GOPROXY=file:// proxy, no checksum database is asked, which the script says.
#
# It reads:
#
#   GOPROXY                  the module proxy: one URL, without a fallback; required
#   GOSUMDB                  the checksum database, or off; the go command's default when unset
#   ZIPCHECK                 the zipcheck binary; bin/zipcheck under the repository root when
#                            unset
#   RELEASE_VERIFY_ATTEMPTS  how many times to ask for a version that the proxy does not serve
#                            yet, or that the checksum database does not answer for, 30 seconds
#                            apart; 20 when unset
#
# It downloads into a module cache of its own, with GOENV=off, GOWORK=off, GOFLAGS set to
# -modcacherw only, so that the cache can be deleted, and GONOSUMDB, GOPRIVATE, GONOPROXY and
# GOINSECURE cleared, so that no setting skips the proxy or the checksum database, and with
# GOTOOLCHAIN=local, the go command on PATH, which needs no other toolchain outside a module. What
# the go command and zipcheck print is printed with workflow commands stopped.
#
# Exit codes: 0 when the version verifies; 1 when it does not: the proxy does not serve it, the
# checksum database rejects it (the go command reports a checksum mismatch or a SECURITY ERROR),
# the hash of its zip is not <h1>, or zipcheck fails on its zip, and then the version, which
# cannot change, must be retracted in the next patch release, as Releases in CONTRIBUTING.md
# says; 2 when the script did not run: the arguments or a setting are wrong, go or jq is missing,
# the checksum database does not answer for the version, which says nothing of the version, so
# verify runs again later, the go command prints no zip, or zipcheck does not run.
set -euo pipefail

token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
echo "::stop-commands::$token"
work=''
trap 'echo "::$token::"; if [[ -n $work ]]; then rm -rf -- "$work"; fi' EXIT

fail() {
  echo "release-verify: $*"
  echo "release-verify: DID NOT RUN"
  exit 2
}

reject() {
  echo "release-verify: FAIL: $*"
  echo "release-verify: a published version cannot change: retract it in the next patch" \
    "release, as Releases in CONTRIBUTING.md says"
  echo "release-verify: FAIL"
  exit 1
}

[[ $# -eq 2 ]] || fail "usage: release-verify.sh <module>@<version> <h1>"
target=$1
want=$2
[[ $target =~ ^[^@[:space:]]+@v[0-9][^@[:space:]]*$ ]] ||
  fail "\"$target\" is not <module>@<version>"
[[ $want =~ ^h1:[A-Za-z0-9+/]{43}=$ ]] || fail "\"$want\" is not an h1: hash"
proxy=${GOPROXY:-}
[[ $proxy =~ ^(https|file):// && $proxy != *[,\|]* ]] ||
  fail "GOPROXY \"$proxy\" is not one https:// or file:// URL without a fallback"
attempts=${RELEASE_VERIFY_ATTEMPTS:-20}
[[ $attempts =~ ^[1-9][0-9]*$ ]] || fail "RELEASE_VERIFY_ATTEMPTS \"$attempts\" is not a count"
root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
zipcheck=${ZIPCHECK:-$root/bin/zipcheck}
[[ -x $zipcheck ]] || fail "zipcheck is not at $zipcheck; build it with" \
  "go -C tools build -o ../bin/zipcheck ./cmd/zipcheck"
for tool in go jq; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is not installed"
done
work=$(mktemp -d "${TMPDIR:-/tmp}/release-verify.XXXXXX") ||
  fail "creating a temporary directory failed"
export GOMODCACHE=$work/mod GOENV=off GOTOOLCHAIN=local GOFLAGS=-modcacherw GOWORK=off \
  GONOSUMDB='' GOPRIVATE='' GONOPROXY='' GOINSECURE=''
if [[ ${GOSUMDB:-} == off ]]; then
  echo "release-verify: GOSUMDB=off: no checksum database is asked, so only the hash of the zip" \
    "that the proxy serves is checked"
fi

for ((attempt = 1; ; attempt++)); do
  code=0
  (cd -- "$work" && go mod download -json "$target") >"$work/download.json" \
    2>"$work/download.err" || code=$?
  [[ $code -ne 0 ]] || break
  error=$(jq -r '.Error // empty' "$work/download.json" 2>/dev/null || true)
  error=${error:-$(cat -- "$work/download.err")}
  # The go command reports both a rejection by the checksum database and a lookup in it that
  # failed, such as one that the network refused, as "verifying module:" or "verifying go.mod:";
  # only a rejection reports a checksum mismatch or a SECURITY ERROR.
  case $error in
  *"SECURITY ERROR"* | *"checksum mismatch"*)
    reject "the checksum database rejects $target: $error"
    ;;
  *"verifying "*)
    [[ $attempt -lt $attempts ]] ||
      fail "the checksum database ${GOSUMDB:-sum.golang.org} does not answer for $target after" \
        "$attempts attempts: $error; this says nothing of the version: re-run verify"
    echo "release-verify: attempt $attempt: the checksum database does not answer for $target" \
      "yet: $error"
    ;;
  *)
    [[ $attempt -lt $attempts ]] ||
      reject "the module proxy $proxy does not serve $target after $attempts attempts: $error"
    echo "release-verify: attempt $attempt: the proxy does not serve $target yet: $error"
    ;;
  esac
  sleep 30
done

sum=$(jq -r '.Sum // empty' "$work/download.json")
zip=$(jq -r '.Zip // empty' "$work/download.json")
[[ -n $sum && -f $zip ]] || fail "go mod download -json printed no Sum or Zip for $target"
[[ $sum == "$want" ]] ||
  reject "the module proxy serves $target with the hash $sum, not $want, the hash of the zip" \
    "built from the tagged commit"
echo "release-verify: the module proxy serves $target with the hash $want of the zip built from" \
  "the tagged commit"
if [[ ${GOSUMDB:-} != off ]]; then
  echo "release-verify: the checksum database ${GOSUMDB:-sum.golang.org} has the same hash"
fi
code=0
"$zipcheck" -zip "$zip" || code=$?
case $code in
0) ;;
1) reject "zipcheck fails on the zip that the module proxy serves" ;;
*) fail "zipcheck did not run on the zip that the module proxy serves; it exited $code" ;;
esac
echo "release-verify: PASS"
