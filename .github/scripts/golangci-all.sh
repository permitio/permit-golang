#!/usr/bin/env bash
# golangci-all.sh runs golangci-lint, the release binary that tools/binaries.lock pins, with
# .golangci.yml over every Go module in the repository. It verifies the configuration, checks the
# formatting of every Go file under the root (golangci-lint fmt --diff, which reaches nested
# modules too), and lints each directory that has a go.mod git tracks, except under testdata/.
# Every machine runs the same official binary, so formatting does not depend on the Go installed.
# When bin/golangci-lint is missing or reports another version, fetch-tool.sh installs the pinned
# one first. Lint output goes to stdout, progress to stderr.
#
# Exit codes: 0 when the configuration is valid, every Go file is formatted and no module has an
# issue; 1 when a file is not formatted or a module has an issue; 2 when the check did not run:
# the script was given arguments, .golangci.yml or the lock's golangci-lint version is missing,
# golangci-lint cannot be installed, git lists no go.mod at the repository root, a go.mod that
# git tracks is missing from the working tree, the configuration is invalid, or golangci-lint
# stopped with an error (such as a module with no Go files).
set -euo pipefail

fail() {
  echo "golangci-all: $*" >&2
  exit 2
}

[[ $# -eq 0 ]] || fail "usage: golangci-all.sh (it takes no arguments)"
root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
config=$root/.golangci.yml
[[ -f $config ]] || fail ".golangci.yml is missing"

# The version line is read the way fetch-tool.sh reads it: spaces around = are optional.
want=$(awk '/^\[/ { entry = ($0 == "[golangci-lint]"); next }
  entry && /^version[[:space:]]*=/ {
    sub(/^version[[:space:]]*=[[:space:]]*/, ""); sub(/[[:space:]]+$/, ""); print; exit
  }' "$root/tools/binaries.lock") ||
  fail "reading tools/binaries.lock failed"
[[ -n $want ]] || fail "tools/binaries.lock has no version for golangci-lint"

bin=$root/bin/golangci-lint
have=''
if [[ -x $bin ]] && ! have=$("$bin" version --short 2>/dev/null); then
  have=''
fi
if [[ $have != "$want" ]]; then
  "$root/.github/scripts/fetch-tool.sh" golangci-lint >/dev/null ||
    fail "installing golangci-lint $want with .github/scripts/fetch-tool.sh failed"
  have=$("$bin" version --short) || fail "running bin/golangci-lint version failed"
  [[ $have == "$want" ]] ||
    fail "bin/golangci-lint is version $have, want $want from tools/binaries.lock"
fi

listed=$(git -C "$root" ls-files -- go.mod '*/go.mod') ||
  fail "listing the go.mod files git tracks failed"
modules=()
has_root=false
while IFS= read -r mod; do
  [[ -n $mod ]] || continue
  dir=$(dirname -- "$mod")
  case /$dir/ in
  */testdata/*) continue ;;
  esac
  [[ -f $root/$mod ]] || fail "git tracks $mod, but the working tree does not have it;" \
    "restore it, or remove it with git rm"
  [[ $dir != . ]] || has_root=true
  modules+=("$dir")
done <<<"$listed"
[[ $has_root == true ]] ||
  fail "git lists no go.mod at the repository root; add it with git add go.mod"

status=0
# step <dir> <golangci-lint arguments> runs golangci-lint in dir and records its exit code: 1
# means it found issues, any other non-zero code means it did not finish.
step() {
  local dir=$1 code=0
  shift
  (cd -- "$root/$dir" && "$bin" "$@" --config "$config") || code=$?
  case $code in
  0) ;;
  1) [[ $status -eq 2 ]] || status=1 ;;
  *)
    echo "golangci-all: golangci-lint $* in $dir exited $code, so it did not finish" >&2
    status=2
    ;;
  esac
}

echo "golangci-all: golangci-lint $want: verifying .golangci.yml" >&2
(cd -- "$root" && "$bin" config verify --config "$config") ||
  fail "golangci-lint config verify rejected .golangci.yml"
echo "golangci-all: checking the formatting of every Go file" >&2
step . fmt --diff
for dir in "${modules[@]}"; do
  echo "golangci-all: linting the module in $dir" >&2
  step "$dir" run ./...
done
echo "golangci-all: linted ${#modules[@]} modules; exit $status" >&2
exit "$status"
