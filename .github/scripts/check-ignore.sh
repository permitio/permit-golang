#!/usr/bin/env bash
# check-ignore.sh checks that the ignore rules keep credential files and build output out of
# commits: git must ignore every path in must_ignore and must not ignore any path in must_track.
# The paths do not need to exist. It runs git check-ignore with no global excludes file, so a
# developer's own ignore rules cannot hide a pattern that .gitignore lost.
#
# Exit codes: 0 when git ignores exactly the paths in must_ignore; 1 when it does not ignore one
# of them or ignores a path in must_track, each named on stderr with the rule that decided it; 2
# when the check did not run: the script was given arguments, the repository root has no
# .gitignore, or git check-ignore failed or did not answer for every path.
set -euo pipefail

must_ignore=(
  .env .env.local sub/.env.local .secrets
  a.pem sub/b.pem a.key x.p12 y.pfx
  z.tfstate z.tfstate.backup credentials.json credentials.yaml
  coverage.out bin/tool .claude/settings.local.json
)
must_track=(.env.example sub/.env.example sub/bin/tool)

fail() {
  echo "check-ignore: $*" >&2
  exit 2
}

[[ $# -eq 0 ]] || fail "usage: check-ignore.sh (it takes no arguments)"
root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
[[ -f $root/.gitignore ]] || fail "the repository root has no .gitignore"

code=0
answers=$(printf '%s\n' "${must_ignore[@]}" "${must_track[@]}" |
  git -C "$root" -c core.excludesFile=/dev/null check-ignore --no-index --verbose \
    --non-matching --stdin) || code=$?
# git check-ignore exits 1 when it ignores none of the paths, which the checks below report.
[[ $code -le 1 ]] || fail "git check-ignore failed with exit code $code"

# ignored_by prints the rule that makes git ignore path, and fails when no rule ignores it.
ignored_by() {
  local line rule
  while IFS= read -r line; do
    [[ ${line#*$'\t'} == "$1" ]] || continue
    rule=${line%%$'\t'*}
    # A path that matches no rule is listed with the rule "::", and a path that matches a
    # negated rule with that rule, such as .gitignore:34:!.env.example.
    [[ $rule != "::" && ${rule#*:*:} != '!'* ]] || return 1
    printf '%s\n' "$rule"
    return 0
  done <<<"$answers"
  return 1
}

answered=$(grep -c $'\t' <<<"$answers") || answered=0
total=$((${#must_ignore[@]} + ${#must_track[@]}))
[[ $answered -eq $total ]] ||
  fail "git check-ignore answered for $answered of $total paths"

status=0
for path in "${must_ignore[@]}"; do
  if ! ignored_by "$path" >/dev/null; then
    echo "check-ignore: git does not ignore $path; add a pattern for it to .gitignore" >&2
    status=1
  fi
done
for path in "${must_track[@]}"; do
  if rule=$(ignored_by "$path"); then
    echo "check-ignore: git ignores $path because of $rule; it must stay committable" >&2
    status=1
  fi
done
echo "check-ignore: checked ${#must_ignore[@]} ignored and ${#must_track[@]} committable paths" >&2
exit "$status"
