#!/usr/bin/env bash
# release-github.sh {draft <tag> <sha> <notes> <dir> | publish <tag> <sha>} drafts and publishes
# the GitHub release of a v2 version, as the release workflow (.github/workflows/release.yml)
# runs it once its checks passed. A release candidate, a tag that ends in -rc.<n>, is a
# prerelease and never the latest release; any other release becomes the latest.
#
#   draft    creates a draft release of <tag> at the commit <sha>, titled <tag>, with the notes in
#            the file <notes>, the --prerelease flag for a release candidate, and every file of
#            the directory <dir> as an asset; <dir> holds SHA256SUMS. A draft creates no tag, so
#            nothing is published yet, and its assets can be checked before publishing locks
#            them, as an immutable release does. It fails when a release of <tag> exists, and
#            then checks the draft it created, as below, and that it has every file of <dir>.
#   publish  checks the draft of <tag>, then publishes it, which creates the tag at <sha>: with
#            --prerelease --latest=false for a release candidate and --latest otherwise. Then it
#            checks that the release is published, a prerelease exactly when <tag> is a release
#            candidate, and that the tag points at <sha>.
#
# The check of a draft: it is a draft, its target is <sha>, it is a prerelease exactly when <tag>
# is a release candidate, and it has the asset SHA256SUMS.
#
# It runs gh, which reads its token from GH_TOKEN: in the workflow, a GITHUB_TOKEN with contents:
# write for draft, and the token of the release GitHub App for publish. publish reads the
# repository, owner/name, from GITHUB_REPOSITORY. What gh prints is printed with workflow
# commands stopped.
#
# Exit codes: 0 when the release was drafted or published; 1 when a check fails, such as a draft
# of a release candidate that is not a prerelease, or a release of <tag> exists already; 2 when
# the script did not run: the arguments are wrong, gh or jq is missing, GITHUB_REPOSITORY is not
# set for publish, or a gh command fails.
set -euo pipefail

token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
echo "::stop-commands::$token"
trap 'echo "::$token::"' EXIT

fail() {
  echo "release-github: $*"
  echo "release-github: DID NOT RUN"
  exit 2
}

reject() {
  echo "release-github: FAIL: $*"
  echo "release-github: FAIL"
  exit 1
}

usage="usage: release-github.sh draft <tag> <sha> <notes> <dir> | publish <tag> <sha>"
[[ $# -ge 3 ]] || fail "$usage"
mode=$1
tag=$2
sha=$3
shift 3
tag_re='^v2\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-rc\.[1-9][0-9]*)?$'
[[ $tag =~ $tag_re ]] ||
  fail "the tag \"$tag\" is not a v2 release tag, v2.<minor>.<patch> or v2.<minor>.<patch>-rc.<n>"
candidate=false
if [[ -n ${BASH_REMATCH[3]} ]]; then
  candidate=true
fi
[[ $sha =~ ^[0-9a-f]{40}$ ]] || fail "\"$sha\" is not a full commit hash"
for tool in gh jq; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is not installed"
done

# view prints the release of the tag as JSON, and fails when gh finds none.
view() {
  gh release view "$tag" --json isDraft,isPrerelease,tagName,targetCommitish,assets
}

# field <json> <name> prints a field of the release's JSON.
field() {
  jq -r --arg name "$2" '.[$name] | tostring' <<<"$1"
}

# check_draft <json> checks the draft of the tag.
check_draft() {
  [[ $(field "$1" isDraft) == true ]] ||
    reject "the release of $tag is published already, or is not a draft"
  local target
  target=$(field "$1" targetCommitish)
  [[ $target == "$sha" ]] || reject "the draft of $tag targets $target, not $sha"
  [[ $(field "$1" isPrerelease) == "$candidate" ]] ||
    reject "the draft of $tag must be a prerelease exactly when the tag is a release candidate;" \
      "$tag is one: $candidate, and the draft is a prerelease: $(field "$1" isPrerelease)"
  has_asset "$1" SHA256SUMS
}

# has_asset <json> <name> checks that the release has the asset.
has_asset() {
  jq -e --arg name "$2" 'any(.assets[]; .name == $name)' <<<"$1" >/dev/null ||
    reject "the draft of $tag has no asset $2"
}

case $mode:$# in
draft:2)
  notes=$1
  dir=$2
  [[ -f $notes ]] || fail "the notes $notes are not a file"
  [[ -f $dir/SHA256SUMS ]] || fail "$dir holds no SHA256SUMS"
  if gh release view "$tag" --json tagName >/dev/null 2>&1; then
    reject "a release of $tag exists already: a version is published once; delete a draft that" \
      "a failed run left, and run the workflow again"
  fi
  # In byte order, whatever the locale.
  LC_ALL=C
  assets=("$dir"/*)
  args=(release create "$tag" --draft --target "$sha" --title "$tag" --notes-file "$notes")
  if [[ $candidate == true ]]; then
    args+=(--prerelease)
  fi
  gh "${args[@]}" "${assets[@]}" || fail "gh release create $tag failed"
  json=$(view) || fail "gh release view $tag failed after the draft was created"
  check_draft "$json"
  for asset in "${assets[@]}"; do
    has_asset "$json" "${asset##*/}"
  done
  echo "release-github: drafted $tag at $sha with ${#assets[@]} assets"
  ;;
publish:0)
  repo=${GITHUB_REPOSITORY:-}
  [[ $repo =~ ^[A-Za-z0-9-]+/[A-Za-z0-9_.-]+$ ]] || fail "GITHUB_REPOSITORY is not owner/name"
  json=$(view) || fail "gh release view $tag failed: there is no draft of $tag"
  check_draft "$json"
  args=(release edit "$tag" --draft=false --latest)
  if [[ $candidate == true ]]; then
    args=(release edit "$tag" --draft=false --prerelease --latest=false)
  fi
  gh "${args[@]}" || fail "gh release edit $tag failed"
  json=$(view) || fail "gh release view $tag failed after publishing"
  [[ $(field "$json" isDraft) == false ]] || reject "the release of $tag is still a draft"
  [[ $(field "$json" isPrerelease) == "$candidate" ]] ||
    reject "the published release of $tag is a prerelease: $(field "$json" isPrerelease), but" \
      "$tag is a release candidate: $candidate"
  [[ $(field "$json" tagName) == "$tag" ]] || reject "the published release is not of $tag"
  target=$(gh api "repos/$repo/git/ref/tags/$tag" --jq .object.sha) ||
    fail "reading the tag $tag failed"
  [[ $target == "$sha" ]] ||
    reject "the tag $tag points at $target, not $sha: retract the version in the next patch" \
      "release, as Releases in CONTRIBUTING.md says"
  echo "release-github: published $tag at $sha"
  ;;
*) fail "$usage" ;;
esac
