#!/usr/bin/env bash
# trivy.sh <block|report> <module directory> scans the go.mod in the directory with the Trivy
# release that tools/binaries.lock pins, which .github/scripts/fetch-tool.sh trivy installs as
# bin/trivy, and prints every vulnerability Trivy reports. With block, a vulnerability of HIGH or
# CRITICAL severity that has a fixed version fails the scan; with report, none does. Lower and
# UNKNOWN severities are printed and never fail: vulnscan (tools/cmd/vulnscan) fails on every
# vulnerability govulncheck finds in the modules a build uses, whatever its severity.
#
# Trivy reads go.mod as it is: it lists the module itself and one package for each module a
# require directive names, except a module that a replace directive points at a local directory,
# for every version or for the version go.mod requires. The scan counts only when Trivy lists
# exactly as many packages, besides the module itself, as go.mod has require directives (go mod
# edit -json), not counting the ones replaced by a local directory. A report without results,
# which Trivy writes as "Results": null or leaves out, counts only when that count is 0. Trivy runs
# in an empty directory with every setting on its command line, so no trivy.yaml, .trivyignore or
# TRIVY_ environment variable can change the scan.
#
# Exit codes: 0 when nothing fails the scan; 1 when block finds a HIGH or CRITICAL vulnerability
# with a fixed version; 2 when the scan did not run: the arguments are wrong, the directory has no
# go.mod, jq is missing, bin/trivy is missing or is not the version the lock pins, go mod edit or
# Trivy failed, Trivy's output is not a JSON report, or its package count is not the number of
# requirements.
set -euo pipefail

fail() {
  echo "trivy: $*"
  echo "trivy: DID NOT RUN"
  exit 2
}

[[ $# -eq 2 ]] || fail "usage: trivy.sh <block|report> <module directory>"
policy=$1
dir=$2
[[ $policy == block || $policy == report ]] || fail "the policy \"$policy\" is not block or report"
[[ -f $dir/go.mod ]] || fail "$dir has no go.mod"
command -v jq >/dev/null 2>&1 || fail "jq is not installed, so Trivy's report cannot be read"
gomod=$(CDPATH='' cd -- "$dir" && pwd)/go.mod
root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)

# The version line is read the way fetch-tool.sh reads it: spaces around = are optional.
want=$(awk '/^\[/ { entry = ($0 == "[trivy]"); next }
  entry && /^version[[:space:]]*=/ {
    sub(/^version[[:space:]]*=[[:space:]]*/, ""); sub(/[[:space:]]+$/, ""); print; exit
  }' "$root/tools/binaries.lock") || fail "reading tools/binaries.lock failed"
[[ -n $want ]] || fail "tools/binaries.lock has no version for trivy"
bin=$root/bin/trivy
[[ -x $bin ]] || fail "bin/trivy is missing; install it with .github/scripts/fetch-tool.sh trivy"
have=$("$bin" --version 2>/dev/null) || fail "bin/trivy --version failed"
have=${have%%$'\n'*}
[[ $have == "Version: $want" ]] || fail "bin/trivy reports \"$have\", want version $want from" \
  "tools/binaries.lock; install it with .github/scripts/fetch-tool.sh trivy"

modfile=$(go -C "$dir" mod edit -json) || fail "go mod edit -json could not read $dir/go.mod"
requires=$(jq '.Require // [] | length' <<<"$modfile") ||
  fail "jq could not read the go mod edit -json output for $dir/go.mod"
# A replace directive without a new version points at a local directory.
replaced=$(jq '[.Replace // [] | .[] | select((.New.Version // "") == "") | .Old] as $local
  | [.Require // [] | .[] | . as $required | select(any($local[]; .Path == $required.Path
      and ((.Version // "") == "" or .Version == $required.Version)))]
  | length' <<<"$modfile") ||
  fail "jq could not read the go mod edit -json output for $dir/go.mod"
expected=$((requires - replaced))
note=""
if [[ $replaced -gt 0 ]]; then
  note=", $replaced of them replaced by a local directory, which Trivy leaves out"
fi

work=$(mktemp -d "${TMPDIR:-/tmp}/trivy.XXXXXX") || fail "creating a temporary directory failed"
trap 'rm -rf -- "$work"' EXIT
mkdir "$work/empty"
for var in $(compgen -e); do
  [[ $var != TRIVY_* ]] || unset "$var"
done
code=0
(cd "$work/empty" && "$bin" fs --scanners vuln --list-all-pkgs --format json --exit-code 0 \
  --config '' --ignorefile '' --skip-version-check --disable-telemetry --no-progress \
  "$gomod") >"$work/report.json" 2>"$work/log" || code=$?
if [[ $code -ne 0 ]]; then
  cat -- "$work/log"
  fail "Trivy exited $code"
fi
jq -e '.SchemaVersion == 2 and has("ArtifactType")' "$work/report.json" >/dev/null 2>&1 ||
  fail "Trivy's output is not a JSON report"

results=$(jq '.Results // [] | length' "$work/report.json")
[[ $results -le 1 ]] || fail "Trivy reported $results targets for one go.mod"
packages=$(jq '[.Results // [] | .[].Packages // [] | .[] | select(.Relationship != "root")]
  | length' "$work/report.json")
if [[ $results -eq 0 && $expected -gt 0 ]]; then
  fail "Trivy reported no results for $dir/go.mod, which has $requires require directives$note"
fi
[[ $packages -eq $expected ]] || fail "Trivy listed $packages packages for $dir/go.mod, which" \
  "has $requires require directives$note"

# Each vulnerability, the ones that fail the scan with block first, then the counts.
vulns='def fixable: (.Severity == "HIGH" or .Severity == "CRITICAL") and .FixedVersion != "";
  [.Results // [] | .[].Vulnerabilities // [] | .[] | .FixedVersion //= ""]'
jq -r --arg policy "$policy" "$vulns"'
  | def ids: .VulnerabilityID + (.VendorIDs // [] | if . == [] then "" else
      " (" + join(", ") + ")" end);
    def verdict: if fixable and $policy == "block" then "FAIL" else "reported only" end;
    def fix: if .FixedVersion == "" then "no fixed version" else "fixed in " + .FixedVersion end;
    sort_by((if fixable then 0 else 1 end), .VulnerabilityID) | .[]
  | "\n" + ids + "  " + verdict + ", " + .Severity + ", " + fix
    + "\n    " + .PkgName + " " + .InstalledVersion
    + (.Title // "" | if . == "" then "" else "\n    " + gsub("[\r\n]+"; " ") end)
' "$work/report.json"
fixable=$(jq "$vulns"' | map(select(fixable)) | length' "$work/report.json")
total=$(jq "$vulns"' | length' "$work/report.json")

echo
echo "trivy: $dir/go.mod: Trivy $want; packages: $packages, require directives: $requires$note"
echo "trivy: HIGH or CRITICAL with a fixed version: $fixable, other vulnerabilities:" \
  "$((total - fixable)) ($policy)"
if [[ $policy == block && $fixable -gt 0 ]]; then
  echo "trivy: FAIL"
  exit 1
fi
echo "trivy: PASS"
