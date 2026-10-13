#!/usr/bin/env bash
# commits-check.sh <base> <head> checks the message of every commit in base..head, the commits a
# pull request adds, with check-commit-msg.sh --committed, which checks each message as
# git log --format=%B prints it. The subjects and what check-commit-msg.sh reports are printed
# with workflow commands stopped, so a commit message cannot issue one. CI checks out the full
# history (fetch-depth: 0) before it runs this.
#
# Exit codes: 0 when every message passes; 1 when a message has text that must not reach the
# public history; 2 when the check did not run: the arguments are not two commit SHAs,
# check-commit-msg.sh is missing, a commit is not in the repository, git fails, the range holds
# no commit, or check-commit-msg.sh exits with another code than 0 or 1.
set -euo pipefail

fail() {
  echo "commits-check: $*" >&2
  exit 2
}

[[ $# -eq 2 ]] || fail "usage: commits-check.sh <base> <head>"
base=$1
head=$2
sha_re='^[0-9a-f]{40}$'
[[ $base =~ $sha_re && $head =~ $sha_re ]] ||
  fail "want two 40-character commit SHAs, got \"$base\" and \"$head\""
root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
checker=$root/.github/scripts/check-commit-msg.sh
[[ -f $checker && -x $checker ]] ||
  fail ".github/scripts/check-commit-msg.sh is missing or not executable"

for commit in "$head" "$base"; do
  git -C "$root" cat-file -e "$commit^{commit}" 2>/dev/null ||
    fail "commit $commit is not in the repository; check it out with fetch-depth: 0"
done
range=$base..$head
commits=$(git -C "$root" rev-list --reverse "$range") || fail "git rev-list $range failed"
[[ -n $commits ]] || fail "$range holds no commit, so no message was checked"

tmp=$(mktemp -d "${TMPDIR:-/tmp}/commits-check.XXXXXX") ||
  fail "creating a temporary directory failed"
token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
stopped=false
# resume prints the line that resumes workflow commands, once; it runs on exit too.
resume() {
  if [[ $stopped == true ]]; then
    echo "::$token::"
    stopped=false
  fi
}
trap 'resume; rm -rf -- "$tmp"' EXIT
echo "::stop-commands::$token"
stopped=true

checked=0
rejected=0
while read -r commit <&3; do
  git -C "$root" log -1 --format=%B "$commit" >"$tmp/message" ||
    fail "reading the message of $commit failed"
  subject=$(head -n 1 "$tmp/message")
  echo "commits-check: ${commit:0:12} $subject"
  code=0
  "$checker" --committed "$tmp/message" 2>&1 || code=$?
  case $code in
  0) ;;
  1) rejected=$((rejected + 1)) ;;
  *) fail "check-commit-msg.sh exited $code on ${commit:0:12}, so its message was not checked" ;;
  esac
  checked=$((checked + 1))
done 3<<<"$commits"
resume

if [[ $rejected -gt 0 ]]; then
  echo "::error title=Commit messages::$rejected of $checked commit messages have text that must" \
    "not reach the public history; see check-commit-msg above"
  exit 1
fi
echo "commits-check: checked $checked commits; every message passed" >&2
