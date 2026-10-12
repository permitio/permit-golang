#!/usr/bin/env bash
# Runs govulncheck in JSON mode, keeps its report, and gates on it.
#
# Usage: govulncheck.sh <govulncheck-binary> <report.json> <packages>...
#
# Exit 0: no finding outside the standard library.
# Exit 1: a module that this module requires has a known vulnerability. This
#         fails whether or not the vulnerable code is called, because every
#         application that uses this module inherits its minimum versions.
#         There is no allowlist on purpose: a finding is fixed by raising the
#         minimum version or by dropping the module, never by ignoring it.
# Exit 2: usage error, govulncheck exited non-zero, or the report lacks the
#         config, SBOM or progress messages of a scan.
#
# Standard library findings are printed as warnings only: they are fixed by
# the Go release that builds the application, not by this module's go.mod.
set -euo pipefail

if [[ $# -lt 3 ]]; then
  echo "usage: $0 <govulncheck-binary> <report.json> <packages>..." >&2
  exit 2
fi
govulncheck=$1
report=$2
shift 2

status=0
"$govulncheck" -format json "$@" >"$report" || status=$?
if ((status != 0)); then
  echo "error: govulncheck exited with status $status; its output is in $report." >&2
  exit 2
fi

ran='any(.[]; has("config")) and any(.[]; has("SBOM")) and any(.[]; has("progress"))'
if ! jq -se "$ran" "$report" >/dev/null; then
  echo "error: $report has no govulncheck config, SBOM and progress messages;" \
    "the scan did not complete." >&2
  exit 2
fi

# One entry per (advisory, module) at the most precise level govulncheck
# reached: called, imported or required.
findings='[.[] | select(has("finding")) | .finding
  | {id: .osv, module: .trace[0].module, version: .trace[0].version,
     fixed: (.fixed_version // "none"),
     level: (if .trace[0].function then 2 elif .trace[0].package then 1 else 0 end)}]
  | group_by([.id, .module]) | map(max_by(.level)) | sort_by(-.level, .id)
  | map(.level = ["required", "imported", "called"][.level])'
line='"\(.id) \(.module)@\(.version) \(.level), fixed in \(.fixed)"'

stdlib=$(jq -rs "$findings | map(select(.module == \"stdlib\"))[] | $line" "$report")
deps=$(jq -rs "$findings | map(select(.module != \"stdlib\"))[] | $line" "$report")

if [[ -n "$stdlib" ]]; then
  echo "Standard library findings (warnings only, fixed by a newer Go release):"
  while IFS= read -r finding; do
    echo "warning: $finding"
  done <<<"$stdlib"
fi

if [[ -n "$deps" ]]; then
  echo "Findings in required modules:" >&2
  while IFS= read -r finding; do
    echo "error: $finding" >&2
  done <<<"$deps"
  echo "Raise the module's minimum version in go.mod to the fixed version" \
    "(go get <module>@<fixed version>, then go mod tidy), or stop requiring the module." >&2
  exit 1
fi

echo "No findings in required modules."
