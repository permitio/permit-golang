#!/usr/bin/env bash
# mutation-range.sh <owner/repo> <pull request> <base> <head> <check name> prints the commit that
# the mutation job of a pull request starts its range at: mutationgate then mutates the Go lines
# changed from that commit to head. It is the newest commit of the pull request whose check run
# named <check name> concluded success and that head contains, so a run that a newer push
# cancelled leaves its lines in the range of the next run. When there is none, it is the merge
# base of base and head. A commit the repository does not have, or one that head no longer
# contains after a force push, is skipped. ci.yml passes the mutation job's id, ${{ github.job }},
# as the check name: the job has no name:, so GitHub names its check run by that id.
#
# It reads the commits of the pull request and their check runs with gh api, which needs GH_TOKEN
# with read access to checks, and needs git, jq and the history of head (fetch-depth: 0). Only
# check runs that GitHub Actions created count. It prints the commit on stdout and, when
# GITHUB_OUTPUT is set, appends start=<commit> to that file, the step's outputs, once the commit
# is checked to be a 40-character SHA. What it chose, and what gh and git report, go to stderr,
# printed with workflow commands stopped.
#
# Exit codes: 0 when it printed the commit; 2 when it could not decide: the arguments are wrong,
# jq is not installed, base or head is not in the repository, gh fails or prints something other
# than the JSON it expects, git fails or prints something other than a commit SHA, or the commit
# cannot be written to GITHUB_OUTPUT.
set -euo pipefail

token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
echo "::stop-commands::$token" >&2
trap 'echo "::$token::" >&2' EXIT

fail() {
  echo "mutation-range: $*" >&2
  exit 2
}

[[ $# -eq 5 ]] ||
  fail "usage: mutation-range.sh <owner/repo> <pull request> <base> <head> <check name>"
repo=$1
number=$2
base=$3
head=$4
check=$5
repo_re='^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$'
[[ $repo =~ $repo_re ]] || fail "\"$repo\" is not owner/repo"
[[ $number =~ ^[1-9][0-9]*$ ]] || fail "\"$number\" is not a pull request number"
sha_re='^[0-9a-f]{40}$'
[[ $base =~ $sha_re && $head =~ $sha_re ]] ||
  fail "want two 40-character commit SHAs, got \"$base\" and \"$head\""
[[ $check =~ ^[A-Za-z_][A-Za-z0-9_-]*$ ]] || fail "check name \"$check\" is not a job id"
command -v jq >/dev/null 2>&1 || fail "jq is not installed"
root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
for commit in "$head" "$base"; do
  git -C "$root" cat-file -e "$commit^{commit}" 2>/dev/null ||
    fail "commit $commit is not in the repository; check it out with fetch-depth: 0"
done

# finish <commit> <why...> appends the commit to GITHUB_OUTPUT as start when that is set, prints
# why the range starts at it on stderr and the commit on stdout. It fails unless the commit is a
# SHA.
finish() {
  local start=$1
  shift
  [[ $start =~ $sha_re ]] || fail "the start commit \"$start\" is not a 40-character commit SHA"
  if [[ -n ${GITHUB_OUTPUT:-} ]]; then
    printf 'start=%s\n' "$start" >>"$GITHUB_OUTPUT" ||
      fail "writing the start commit to GITHUB_OUTPUT ($GITHUB_OUTPUT) failed"
  fi
  echo "mutation-range: $*" >&2
  printf '%s\n' "$start"
  exit 0
}

listed=$(gh api --paginate "repos/$repo/pulls/$number/commits?per_page=100") ||
  fail "listing the commits of pull request $number with gh api failed"
shas=$(jq -r '.[].sha' <<<"$listed" 2>/dev/null) ||
  fail "gh api did not list the commits of pull request $number as JSON"
commits=()
while IFS= read -r sha; do
  [[ -n $sha ]] || continue
  [[ $sha =~ $sha_re ]] || fail "gh api listed \"$sha\", which is not a commit SHA"
  commits+=("$sha")
done <<<"$shas"

# The API lists the commits oldest first; the newest that qualifies wins.
query="check_name=$check&filter=latest&per_page=100"
# shellcheck disable=SC2016 # $name is a jq variable, which jq --arg sets
passed_filter='[.check_runs[] | select(.name == $name and .conclusion == "success"
  and .app.slug == "github-actions")] | length'
for ((i = ${#commits[@]} - 1; i >= 0; i--)); do
  commit=${commits[i]}
  runs=$(gh api "repos/$repo/commits/$commit/check-runs?$query") ||
    fail "reading the check runs of $commit with gh api failed"
  passed=$(jq --arg name "$check" "$passed_filter" <<<"$runs" 2>/dev/null) ||
    fail "gh api did not list the check runs of $commit as JSON"
  [[ $passed -gt 0 ]] || continue
  if ! git -C "$root" cat-file -e "$commit^{commit}" 2>/dev/null; then
    echo "mutation-range: skipping ${commit:0:12}: its $check check succeeded, but the" \
      "repository does not have it" >&2
    continue
  fi
  if git -C "$root" merge-base --is-ancestor "$commit" "$head"; then
    finish "$commit" "starting at ${commit:0:12}, the newest commit of pull request $number" \
      "whose $check check succeeded"
  else
    code=$?
    [[ $code -eq 1 ]] || fail "git merge-base --is-ancestor $commit $head failed"
  fi
  echo "mutation-range: skipping ${commit:0:12}: its $check check succeeded, but head does" \
    "not contain it" >&2
done

start=$(git -C "$root" merge-base "$base" "$head") || fail "git merge-base $base $head failed"
finish "$start" "starting at the merge base ${start:0:12}: no commit of pull request $number in" \
  "the history of head has a successful $check check"
