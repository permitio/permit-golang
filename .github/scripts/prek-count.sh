#!/usr/bin/env bash
# prek-count.sh [--upstream] runs every hook in .pre-commit-config.yaml on every file with prek,
# and checks that EXPECTED_HOOKS hooks reported Passed, or Skipped with a reason such as
# "(no files to check)". A hook that the SKIP variable skips reports Skipped without a reason,
# and a hook removed from the configuration reports nothing, so either leaves the count short.
# prek's output is printed between stop-commands lines, so a hook's output cannot issue workflow
# commands.
#
# With --upstream it runs the hooks twice: with prek's built-in versions of the pre-commit-hooks
# hooks, and then with PREK_NO_FAST_PATH=1, which runs the upstream hooks. It fails when the two
# runs report a hook differently.
#
# It reads EXPECTED_HOOKS, the number of hooks, and PREK, the prek to run: bin/prek at the
# repository root by default, which .github/scripts/fetch-tool.sh prek installs.
#
# Exit codes: 0 when every hook passed or had nothing to check, EXPECTED_HOOKS hooks reported,
# and with --upstream both runs reported every hook the same way; 1 when a hook failed or the two
# runs differ; 2 when the check did not run: the arguments are wrong, EXPECTED_HOOKS is not a
# positive number, prek is not installed or exited with a code other than 0 or 1, or another
# number of hooks than EXPECTED_HOOKS reported.
set -euo pipefail

fail() {
  echo "prek-count: $*" >&2
  exit 2
}

upstream=false
case $# in
0) ;;
1)
  [[ $1 == --upstream ]] || fail "usage: prek-count.sh [--upstream]"
  upstream=true
  ;;
*) fail "usage: prek-count.sh [--upstream]" ;;
esac
root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
expected=${EXPECTED_HOOKS:-}
[[ $expected =~ ^[1-9][0-9]*$ ]] || fail "EXPECTED_HOOKS \"$expected\" is not a positive number"
prek=${PREK:-$root/bin/prek}
[[ -f $prek && -x $prek ]] ||
  fail "${prek#"$root"/} is not an executable; install it with .github/scripts/fetch-tool.sh prek"

tmp=$(mktemp -d "${TMPDIR:-/tmp}/prek-count.XXXXXX") || fail "creating a temporary directory failed"
trap 'rm -rf -- "$tmp"' EXIT
token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')

# run_hooks <label> <log> [VAR=value...] runs every hook with the variables set, prints prek's
# output with workflow commands stopped and keeps it in log, and fails unless every hook passed or
# had nothing to check and EXPECTED_HOOKS hooks reported. It exits 1 when a hook failed.
run_hooks() {
  local label=$1 log=$2 code=0 count
  shift 2
  echo "prek-count: running every hook ($label)" >&2
  echo "::stop-commands::$token"
  (cd -- "$root" && env "$@" "$prek" run --all-files --show-diff-on-failure --color=never \
    --no-progress --no-hide-status) 2>&1 | tee "$log" || code=$?
  echo "::$token::"
  case $code in
  0) ;;
  1)
    echo "prek-count: a hook failed ($label); see its output above" >&2
    exit 1
    ;;
  *) fail "prek exited $code ($label), so the hooks did not run" ;;
  esac
  count=$(awk '/(Passed|\)Skipped)$/ { n++ } END { print n + 0 }' "$log")
  [[ $count -eq $expected ]] || fail "$count of $expected hooks passed or had nothing to check" \
    "($label); a hook was skipped, added or removed: run every hook, or change EXPECTED_HOOKS"
  echo "prek-count: $count of $expected hooks passed or had nothing to check ($label)" >&2
}

run_hooks "built-in hooks" "$tmp/fast"
[[ $upstream == true ]] || exit 0
run_hooks "upstream hooks" "$tmp/upstream" PREK_NO_FAST_PATH=1

grep -E '(Passed|Skipped|Failed)$' "$tmp/fast" >"$tmp/fast-results" ||
  fail "reading the hook results of the built-in hooks failed"
grep -E '(Passed|Skipped|Failed)$' "$tmp/upstream" >"$tmp/upstream-results" ||
  fail "reading the hook results of the upstream hooks failed"
echo "::stop-commands::$token"
code=0
diff -u -L built-in -L upstream "$tmp/fast-results" "$tmp/upstream-results" || code=$?
echo "::$token::"
case $code in
0) echo "prek-count: the built-in and upstream hooks reported every hook the same way" >&2 ;;
1)
  echo "prek-count: the built-in and upstream hooks reported hooks differently; see the diff" \
    "above" >&2
  exit 1
  ;;
*) fail "comparing the hook results failed" ;;
esac
