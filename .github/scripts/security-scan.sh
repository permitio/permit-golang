#!/usr/bin/env bash
# security-scan.sh --trufflehog run|skip runs the scheduled security scan, writes its report to
# the job summary and posts it to Slack. The security-scan job in .github/workflows/ci.yml runs it
# twice a week and in the security mode; it runs the same way on any clone with the full history.
# It needs the network, jq, gh and Go, and reads these variables:
#
#   GITHUB_REPOSITORY               owner/name of the repository, which the report names and
#                                   whose Dependabot alerts it reads; required
#   DEPENDABOT_ALERTS_TOKEN         a token that can read the repository's Dependabot alerts,
#                                   which the GITHUB_TOKEN cannot
#   GITLEAKS_HISTORY_ACCEPT_SHA256  the sha256 digests of the accepted history findings' gitleaks
#                                   fingerprints, one per line
#   SECURITY_SLACK_WEBHOOK_URL      the webhook that the report is posted to
#   GITHUB_STEP_SUMMARY, GITHUB_SERVER_URL and GITHUB_RUN_ID, which CI sets: the job summary and
#   the run's URL
#
# It reads each secret, unsets it, and passes it to the one command that needs it: the token to
# gh, the digests to its own comparison and the webhook to notify. No scanner gets any of them.
#
# Each source below is one scan of the report, with a result of its own: exit 0 when it completed
# and found nothing, 1 when it completed with findings, and 2 when it did not complete.
#
#   master-floor        vulnscan -only floor and trivy.sh report on the root module as committed
#   master-ceiling      the same on the ceiling: every dependency at its newest version
#   tools               the same on tools/
#   consumer-v2-latest  the same on a throwaway module that requires the SDK's latest release, as
#                       an application resolves it, and imports its packages; before the first
#                       release it completes with the state "no release yet"
#   dependabot-alerts   the open Dependabot alerts, read with gh api and DEPENDABOT_ALERTS_TOKEN;
#                       the log prints their count only, and the Slack report lists them
#   history-secrets     gitleaks over the full history of a bare mirror of the repository
#   verified-secrets    with --trufflehog run only: TruffleHog --results=verified over the mirror
#
# vuln.sh --prepare builds govulncheck, vulnscan and secreport, installs Trivy and prepares the
# ceiling for the first four. The history scan writes its own gitleaks configuration, the rules of
# .gitleaks.toml, and runs gitleaks from an empty directory with an empty ignore file, without any
# GITLEAKS_ variable or git configuration of the user or the system, with --redact and --text.
# Its log and the report print counts only: "history: N findings, M accepted, K new". A finding
# is accepted when the sha256 of its fingerprint is in GITLEAKS_HISTORY_ACCEPT_SHA256; the new
# ones are listed in the Slack report only, as private findings. Every accepted digest must match
# a finding: one that matches none makes the source exit 2, since the scan may have missed part
# of the history. Without the variable, every finding is new, and the source's state says so.
# TruffleHog's output and log are never printed: a verified secret is counted in the log and
# listed, redacted, in the Slack report only. TruffleHog's log must count a scan that read
# something and verified as many secrets as it printed, or the source exits 2. A value that a
# scanner or GitHub reports is cut to fit the report, so that no finding is lost.
#
# secreport (tools/cmd/secreport) reports the sources with -findings report: findings are listed
# and pass, a source that did not complete fails, and so does a verified secret (-fail-on
# verified-secrets). It writes the standard report to the job summary and in Slack's format,
# which notify (tools/cmd/notify) posts to SECURITY_SLACK_WEBHOOK_URL under the line
# "<repo> security-scan: <verdict>. <run URL>"; without the webhook, notify warns and posts
# nothing. Everything until the report is printed with workflow commands stopped.
#
# security-scan.sh --history-digests <file> runs only the history scan, on a local clone, and
# writes the sha256 digest of each finding's fingerprint to the file, one per line, for the
# maintainer who sets GITLEAKS_HISTORY_ACCEPT_SHA256; it prints each digest with its finding. It
# refuses to run in GitHub Actions, whose log is public, and to write the file inside the
# repository, where it could be committed.
#
# Exit codes: 0 when every source completed and no secret was verified; 1 when TruffleHog
# verified a secret, or the report could not be posted; 2 when a source did not complete, the
# arguments are wrong, jq is missing, GITHUB_REPOSITORY is not owner/name, secreport or notify
# cannot be built, or the report cannot be written.
# shellcheck disable=SC2329 # run_source runs each source's function by name.
set -euo pipefail

token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
echo "::stop-commands::$token"
stopped=true
work=''
# resume prints the line that resumes workflow commands, once; it runs on exit too.
resume() {
  if [[ $stopped == true ]]; then
    echo "::$token::"
    stopped=false
  fi
}
trap 'resume; if [[ -n $work ]]; then rm -rf -- "$work"; fi' EXIT

fail() {
  echo "security-scan: $*"
  echo "security-scan: DID NOT RUN"
  exit 2
}

usage="usage: security-scan.sh --trufflehog run|skip | --history-digests <file>"
trufflehog=''
digests_file=''
if [[ $# -eq 2 && $1 == --trufflehog && ($2 == run || $2 == skip) ]]; then
  trufflehog=$2
elif [[ $# -eq 2 && $1 == --history-digests && -n $2 ]]; then
  [[ ${GITHUB_ACTIONS:-} != true ]] || fail "--history-digests prints where each finding is," \
    "and the log of GitHub Actions is public; run it on your own clone"
  digests_file=$2
  [[ $digests_file == /* ]] || digests_file=$PWD/$digests_file
else
  fail "$usage"
fi
command -v jq >/dev/null 2>&1 || fail "jq is not installed, so the scans cannot be read"
root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
if [[ -n $digests_file ]]; then
  # The digests belong in the secret only: a file in the clone could be committed.
  digests_dir=$(CDPATH='' cd -- "$(dirname -- "$digests_file")" && pwd -P) ||
    fail "the directory of $digests_file does not exist"
  physical_root=$(cd -- "$root" && pwd -P) || fail "reading the path of $root failed"
  [[ $digests_dir != "$physical_root" && $digests_dir != "$physical_root"/* ]] ||
    fail "$digests_file is inside the repository; write the digests to a file outside the clone"
fi
cd -- "$root"

alerts_token=${DEPENDABOT_ALERTS_TOKEN:-}
webhook=${SECURITY_SLACK_WEBHOOK_URL:-}
accept_list=${GITLEAKS_HISTORY_ACCEPT_SHA256:-}
unset DEPENDABOT_ALERTS_TOKEN SECURITY_SLACK_WEBHOOK_URL SLACK_WEBHOOK_URL GH_TOKEN GITHUB_TOKEN
for var in $(compgen -e); do
  [[ $var != GITLEAKS_* && $var != TRUFFLEHOG_* ]] || unset "$var"
done
repo=${GITHUB_REPOSITORY:-}
repo_re='^[A-Za-z0-9-]+/[A-Za-z0-9_.-]+$'
if [[ -z $digests_file ]]; then
  [[ $repo =~ $repo_re ]] || fail "GITHUB_REPOSITORY \"$repo\" is not owner/name; set it to" \
    "the repository whose alerts to read, as CI does"
fi

work=$(mktemp -d "${TMPDIR:-/tmp}/security-scan.XXXXXX") ||
  fail "creating a temporary directory failed"
results=$work/results
mkdir "$results" "$work/empty" "$work/parts"

# lock_version <name> prints the version tools/binaries.lock pins for name, read the way
# fetch-tool.sh reads it.
lock_version() {
  awk -v entry="[$1]" '/^\[/ { found = ($0 == entry); next }
    found && /^version[[:space:]]*=/ {
      sub(/^version[[:space:]]*=[[:space:]]*/, ""); sub(/[[:space:]]+$/, ""); print; exit
    }' tools/binaries.lock
}

# install_tool <name> <version flag> <prefix> installs bin/<name> with fetch-tool.sh and checks
# that <name> <version flag> prints <prefix> and the version the lock pins.
install_tool() {
  local name=$1 want have
  want=$(lock_version "$name") || return 1
  if [[ -z $want ]]; then
    echo "security-scan: tools/binaries.lock has no version for $name"
    return 1
  fi
  .github/scripts/fetch-tool.sh "$name" >/dev/null || {
    echo "security-scan: installing $name with .github/scripts/fetch-tool.sh failed"
    return 1
  }
  have=$("bin/$name" "$2" 2>&1) || have=''
  if [[ $have != "$3$want" ]]; then
    echo "security-scan: bin/$name reports \"$have\", want version $want from tools/binaries.lock"
    return 1
  fi
}

# run_source <name> <command>... runs a source's command, which prints its output and returns
# the source's exit code, and records both for secreport. A function that run_source runs checks
# each command it runs, since errexit does not stop it.
run_source() {
  local name=$1 code=0
  shift
  echo "security-scan: $name"
  "$@" >"$results/$name.out" 2>&1 || code=$?
  cat -- "$results/$name.out"
  echo "$code" >"$results/$name.exit"
}

# not_prepared prints that vuln.sh --prepare failed, and returns 2.
not_prepared() {
  echo "security-scan: vuln.sh --prepare failed, so this tree was not scanned"
  return 2
}

# scanners <name> <cwd> <module> <vulnscan arguments>... runs vulnscan and trivy.sh report, from
# cwd, on one module, writes the findings of both to the source's findings file, and returns 2
# when a scanner did not complete, otherwise 1 when one found something, and 0.
scanners() {
  local name=$1 cwd=$2 module=$3 vulnscan=0 trivy=0 worst=0 part code
  shift 3
  bin/vulnscan "$@" -findings "$work/parts/$name.vulnscan" || vulnscan=$?
  (cd -- "$cwd" && "$root/.github/scripts/trivy.sh" report "$module" \
    "$work/parts/$name.trivy") || trivy=$?
  : >"$results/$name.findings" || return 2
  # A scanner that completed wrote its findings.
  for part in vulnscan trivy; do
    if [[ -f $work/parts/$name.$part ]]; then
      cat -- "$work/parts/$name.$part" >>"$results/$name.findings" || return 2
    elif [[ ${!part} -le 1 ]]; then
      echo "security-scan: $part wrote no findings file"
      printf -v "$part" 2
    fi
  done
  for code in "$vulnscan" "$trivy"; do
    case $code in
    0) ;;
    1) [[ $worst -eq 2 ]] || worst=1 ;;
    *) worst=2 ;;
    esac
  done
  # A source with findings found something, even when its scanners exit 0, as trivy.sh report
  # always does.
  if [[ $worst -eq 0 && -s $results/$name.findings ]]; then
    worst=1
  fi
  return "$worst"
}

# go_consumer runs go with the settings of an application that resolves a release: the default
# proxy and checksum database, and go.mod changes allowed.
go_consumer() {
  env GOPROXY=https://proxy.golang.org,direct GOSUMDB=sum.golang.org GOPRIVATE= GONOPROXY= \
    GONOSUMDB= GOFLAGS=-mod=mod GOWORK=off go "$@"
}

# scan_consumer scans the SDK's latest release as an application resolves it, and returns the
# source's exit code.
scan_consumer() {
  local dir=$work/consumer module found version packages code=0
  module=$(awk '$1 == "module" { print $2; exit }' go.mod) || return 2
  mkdir -- "$dir" || return 2
  (cd -- "$dir" && go_consumer mod init example.com/consumer) || return 2
  if ! found=$(cd -- "$dir" && go_consumer list -m -json "$module@latest" 2>"$work/latest"); then
    if grep -qF 'no matching versions for query "latest"' "$work/latest"; then
      echo "security-scan: $module has no release yet"
      echo "no release yet" >"$results/consumer-v2-latest.state"
      return 0
    fi
    cat -- "$work/latest"
    echo "security-scan: go list -m $module@latest failed"
    return 2
  fi
  version=$(jq -r '.Version // ""' <<<"$found") || return 2
  pseudo_re='[-.][0-9]{14}-[0-9a-f]{12}$'
  if [[ -z $version || $version =~ $pseudo_re ]]; then
    echo "security-scan: $module@latest is \"$version\", not a release; it has no release yet"
    echo "no release yet" >"$results/consumer-v2-latest.state"
    return 0
  fi
  echo "security-scan: $module@latest is $version"
  echo "latest release $version" >"$results/consumer-v2-latest.state"
  (cd -- "$dir" && go_consumer get "$module@$version") || return 2
  packages=$(cd -- "$dir" &&
    go_consumer list -f '{{if ne .Name "main"}}{{.ImportPath}}{{end}}' "$module/...") ||
    return 2
  packages=$(grep -v -e '/internal/' -e '/internal$' <<<"$packages" | grep -v '^$') || {
    echo "security-scan: $module@$version has no package that an application can import"
    return 2
  }
  {
    echo "// Command consumer imports every public package of the SDK, as an application does."
    echo "package main"
    echo
    echo "import ("
    while IFS= read -r package; do
      printf '\t_ "%s"\n' "$package"
    done <<<"$packages"
    echo ")"
    echo
    echo "func main() {}"
  } >"$dir/main.go" || return 2
  (cd -- "$dir" && go_consumer mod tidy) || return 2
  scanners consumer-v2-latest "$work" consumer -consumer "$dir" || code=$?
  return "$code"
}

# fit is the jq that keeps a value from outside within what secreport accepts, so that a long or
# odd value never makes a source's findings invalid, and a finding is never lost: clip($n) turns
# each control character into a space and cuts the value to at most $n bytes, the last three of
# them "..."; link drops a URL that is not an https link secreport accepts or is longer than 300
# bytes.
# shellcheck disable=SC2016 # $n is a jq variable
fit='def clip($n): gsub("\\p{Cc}"; " ") | if utf8bytelength <= $n then .
    else .[0:$n] | until(utf8bytelength <= $n - 3; .[0:-1]) + "..." end;
  def link: if test("^https://[^\\s<>|()\\[\\]\"]+$") and utf8bytelength <= 300 then .
    else "" end;'

# scan_alerts reads the open Dependabot alerts, and returns the source's exit code. The alerts
# are private findings: the log prints their count only.
scan_alerts() {
  local name=dependabot-alerts count
  if [[ -z $alerts_token ]]; then
    echo "security-scan: DEPENDABOT_ALERTS_TOKEN is not set, so the open Dependabot alerts" \
      "were not read; the GITHUB_TOKEN cannot read them"
    return 2
  fi
  if ! GH_TOKEN=$alerts_token gh api --paginate \
    "repos/$repo/dependabot/alerts?state=open&per_page=100" >"$work/alerts.json"; then
    echo "security-scan: gh api could not read the open Dependabot alerts of $repo"
    return 2
  fi
  if ! jq -se 'all(.[]; type == "array" and all(.[]; type == "object")) and length > 0' \
    "$work/alerts.json" >/dev/null 2>&1; then
    echo "security-scan: gh api returned something other than lists of alerts"
    return 2
  fi
  jq -sc "$fit"'add | .[] | {id: .security_advisory.ghsa_id, package:
    (.security_vulnerability.package.name | clip(200)), version:
    (.security_vulnerability.vulnerable_version_range // "" | clip(200)),
    severity: (.security_advisory.severity // ""),
    fix: (.security_vulnerability.first_patched_version.identifier // "" | clip(200)),
    url: ("https://github.com/advisories/" + .security_advisory.ghsa_id | link),
    private: true}' "$work/alerts.json" >"$results/$name.findings" ||
    {
      echo "security-scan: the alerts do not have the fields of a Dependabot alert"
      return 2
    }
  count=$(wc -l <"$results/$name.findings" | tr -d ' ')
  echo "security-scan: $repo has $count open Dependabot alerts"
  echo "$count open alerts" >"$results/$name.state"
  [[ $count -eq 0 ]] || return 1
}

# sha256 prints the sha256 of its standard input in lowercase hex.
sha256() {
  local sum
  if command -v sha256sum >/dev/null 2>&1; then
    sum=$(sha256sum)
  else
    sum=$(shasum -a 256)
  fi
  echo "${sum%% *}"
}

# mirror makes a bare mirror of the repository's every ref in $work/mirror.git, once.
mirror() {
  local shallow
  [[ ! -d $work/mirror.git ]] || return 0
  shallow=$(git rev-parse --is-shallow-repository) || return 1
  if [[ $shallow != false ]]; then
    echo "security-scan: the repository is a shallow clone, which lacks part of the history;" \
      "clone the full history (fetch-depth: 0)"
    return 1
  fi
  GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 git clone --quiet --mirror --no-local \
    -- "$root" "$work/mirror.git" || return 1
}

# scan_history runs gitleaks over the mirror's full history. It leaves the findings in
# $work/history.json and the digest of each finding's fingerprint, in order, in
# $work/history.digests, prints counts only, and returns 0, or 2 when the scan did not complete.
scan_history() {
  local want code=0 commits findings esc plain
  install_tool gitleaks version '' || return 2
  mirror || return 2
  commits=$(git -C "$work/mirror.git" rev-list --all --count) || return 2
  if [[ $commits -eq 0 ]]; then
    echo "security-scan: the mirror holds no commit, so there is no history to scan"
    return 2
  fi
  : >"$work/ignore" || return 2
  # The rules of .gitleaks.toml, without its comments; a test in tools/scripts checks that they
  # stay the same.
  cat >"$work/config.toml" <<'TOML' || return 2
title = "permit-golang"

[extend]
useDefault = true

[[rules]]
id = "permit-api-key"
description = "Permit API key"
regex = '''permit_key_[A-Za-z0-9]{20,}'''
keywords = ["permit_key_"]

[[allowlists]]
description = "Fake Permit API keys"
regexes = ['''^permit_key_FAKE[A-Za-z0-9_]*$''']
TOML
  want=$(lock_version gitleaks)
  (cd -- "$work/empty" && GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 \
    "$root/bin/gitleaks" git --no-banner --redact --exit-code 0 \
    --config "$work/config.toml" --gitleaks-ignore-path "$work/ignore" --ignore-gitleaks-allow \
    --report-format json --report-path "$work/history.json" \
    "--log-opts=--full-history --all --text" "$work/mirror.git") >"$work/gitleaks.log" 2>&1 ||
    code=$?
  # The log without the colors gitleaks gives it, which the job summary would show as text.
  esc=$'\033'
  plain=$(sed "s/$esc\\[[0-9;]*m//g" "$work/gitleaks.log") || return 2
  if [[ -n $plain ]]; then
    printf '%s\n' "$plain"
  fi
  if [[ $code -ne 0 ]]; then
    echo "security-scan: gitleaks exited $code"
    return 2
  fi
  if grep -Eq '^[^ ]+ (ERR|FTL) ' <<<"$plain"; then
    echo "security-scan: gitleaks logged an error, so it may not have scanned every commit"
    return 2
  fi
  findings=$(jq -e 'if type == "array" and all(.[]; type == "object" and
    (.Fingerprint | type == "string" and . != "")) then length else error("not findings") end' \
    "$work/history.json" 2>/dev/null) || {
    echo "security-scan: gitleaks wrote no report, or one that is not a JSON list of findings"
    return 2
  }
  : >"$work/history.digests"
  jq -r '.[].Fingerprint' "$work/history.json" | while IFS= read -r fingerprint; do
    printf '%s' "$fingerprint" | sha256 >>"$work/history.digests"
  done
  if [[ $(wc -l <"$work/history.digests" | tr -d ' ') -ne $findings ]]; then
    echo "security-scan: the gitleaks report has a fingerprint that is not one line"
    return 2
  fi
  echo "security-scan: gitleaks $want scanned $commits commits of every ref"
}

# check_history compares the history's findings with the accepted digests, and returns the
# source's exit code.
check_history() {
  local name=history-secrets total accepted=0 unmatched new_list='' index=0 digest note='' state
  scan_history || return 2
  printf '%s\n' "$accept_list" | tr -d ' \t\r' | sed '/^$/d' >"$work/accepted" || return 2
  if grep -Evq '^[0-9a-f]{64}$' "$work/accepted"; then
    echo "security-scan: GITLEAKS_HISTORY_ACCEPT_SHA256 holds a line that is not a sha256" \
      "digest in lowercase hex"
    return 2
  fi
  if [[ -n $(sort "$work/accepted" | uniq -d) ]]; then
    echo "security-scan: GITLEAKS_HISTORY_ACCEPT_SHA256 lists a digest twice"
    return 2
  fi
  if [[ ! -s $work/accepted ]]; then
    note="; GITLEAKS_HISTORY_ACCEPT_SHA256 is not set, so every finding is new"
  fi
  : >"$work/matched"
  while IFS= read -r digest; do
    if grep -qxF "$digest" "$work/accepted"; then
      accepted=$((accepted + 1))
      echo "$digest" >>"$work/matched"
    else
      new_list+="${new_list:+,}$index"
    fi
    index=$((index + 1))
  done <"$work/history.digests"
  total=$index
  # grep exits 1 when every accepted digest matched a finding.
  unmatched=$(grep -cvxFf "$work/matched" "$work/accepted") || [[ $? -eq 1 ]] || {
    echo "security-scan: grep could not compare the accepted digests with the findings"
    return 2
  }
  if [[ ! -s $work/matched ]]; then
    unmatched=$(wc -l <"$work/accepted" | tr -d ' ')
  fi
  state="history: $total findings, $accepted accepted, $((total - accepted)) new$note"
  echo "$state"
  echo "$state" >"$results/$name.state"
  if [[ $unmatched -gt 0 ]]; then
    echo "security-scan: $unmatched accepted digests in GITLEAKS_HISTORY_ACCEPT_SHA256 match" \
      "no finding: the scan may have missed part of the history, or the list is out of date"
    return 2
  fi
  jq -c --argjson new "[$new_list]" "$fit"'. as $all | $new[] | $all[.] | {id: .RuleID,
    package: ("\(.File):\(.StartLine)" | clip(200)), version: (.Commit // "" | .[0:12]),
    fix: "revoke the secret, then add the sha256 of its fingerprint to the accepted digests",
    private: true}' "$work/history.json" >"$results/$name.findings" || return 2
  [[ $total -eq $accepted ]] || return 1
}

# check_verified runs TruffleHog over the mirror with --results=verified, and returns the
# source's exit code. Its output and its log may hold the secrets it found, so neither is
# printed. The log ends with the count of what it scanned, which must show that it read
# something and verified as many secrets as it printed.
check_verified() {
  local name=verified-secrets code=0 verified scanned
  install_tool trufflehog --version 'trufflehog ' || return 2
  mirror || return 2
  (cd -- "$work/empty" && GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 \
    "$root/bin/trufflehog" git "file://$work/mirror.git" --results=verified --json \
    --no-update --no-ignore-tag --fail-on-scan-errors) >"$work/trufflehog.json" \
    2>"$work/trufflehog.log" || code=$?
  if [[ $code -ne 0 ]]; then
    echo "security-scan: trufflehog exited $code; its log may hold secrets, so run it on your" \
      "own clone to see why"
    return 2
  fi
  verified=$(jq -se 'if all(.[]; type == "object") then map(select(.Verified == true)) | length
    else error("not results") end' "$work/trufflehog.json" 2>/dev/null) || {
    echo "security-scan: TruffleHog's output is not JSON results"
    return 2
  }
  scanned=$(jq -nR --argjson verified "$verified" '[inputs | fromjson? | objects
    | select(.msg == "finished scanning")] | last
    | (.chunks | type) == "number" and .chunks > 0 and .verified_secrets == $verified' \
    "$work/trufflehog.log" 2>/dev/null) || scanned=false
  if [[ $scanned != true ]]; then
    echo "security-scan: TruffleHog's log does not count a scan that read something and" \
      "verified the $verified secrets it printed, so it may not have scanned the history"
    return 2
  fi
  jq -sc "$fit"'.[] | select(.Verified == true) | .SourceMetadata.Data.Git as $git | {
    id: (.DetectorName // "unknown"),
    package: ("\($git.file // ""):\($git.line // 0)" | clip(200)),
    version: ($git.commit // "" | .[0:12]), severity: "critical",
    fix: ("revoke it now: TruffleHog verified that it works ("
      + (.Redacted // "" | clip(80)) + ")"),
    private: true}' "$work/trufflehog.json" >"$results/$name.findings" || return 2
  echo "security-scan: TruffleHog verified secrets: $verified"
  echo "TruffleHog verified secrets: $verified" >"$results/$name.state"
  [[ $verified -eq 0 ]] || return 1
}

if [[ -n $digests_file ]]; then
  code=0
  scan_history || code=$?
  [[ $code -eq 0 ]] || fail "the history scan did not complete"
  cp -- "$work/history.digests" "$digests_file" || fail "writing $digests_file failed"
  jq -r '.[] | "\(.RuleID) \(.File):\(.StartLine) commit \(.Commit // "" | .[0:12])"' \
    "$work/history.json" | paste -d ' ' "$work/history.digests" -
  echo "security-scan: wrote the digests of $(wc -l <"$digests_file" | tr -d ' ') findings to" \
    "$digests_file"
  exit 0
fi

echo "security-scan: building secreport and notify"
go -C tools build -o "$root/bin/secreport" ./cmd/secreport || fail "building secreport failed"
go -C tools build -o "$root/bin/notify" ./cmd/notify || fail "building notify failed"

echo "security-scan: preparing govulncheck, vulnscan, Trivy and the ceiling with vuln.sh"
if .github/scripts/vuln.sh --prepare "$work/ceiling"; then
  run_source master-floor scanners master-floor "$root" . -only floor
  run_source master-ceiling scanners master-ceiling "$work" ceiling -only ceiling \
    -ceiling "$work/ceiling"
  run_source tools scanners tools "$root" tools -only tools
  run_source consumer-v2-latest scan_consumer
else
  for name in master-floor master-ceiling tools consumer-v2-latest; do
    run_source "$name" not_prepared
  done
fi
run_source dependabot-alerts scan_alerts
run_source history-secrets check_history
names=(master-floor master-ceiling tools consumer-v2-latest dependabot-alerts history-secrets)
report=(bin/secreport -id security-scan -title "Security scan" -dir "$results" -findings report
  -slack "$work/report.slack")
if [[ $trufflehog == run ]]; then
  run_source verified-secrets check_verified
  names+=(verified-secrets)
  report+=(-fail-on verified-secrets)
fi

resume
run_url=''
if [[ -n ${GITHUB_SERVER_URL:-} && -n ${GITHUB_RUN_ID:-} ]]; then
  run_url=$GITHUB_SERVER_URL/$repo/actions/runs/$GITHUB_RUN_ID
  report+=(-run-url "$run_url")
fi
if [[ -n ${GITHUB_STEP_SUMMARY:-} ]]; then
  report+=(-summary "$GITHUB_STEP_SUMMARY")
fi
code=0
"${report[@]}" "${names[@]}" || code=$?
verdict=PASS
case $code in
0) ;;
1) verdict=FAIL ;;
*) verdict="DID NOT RUN" ;;
esac
notify=(bin/notify -channel security -repo "$repo" -check security-scan -result "$verdict")
if [[ -f $work/report.slack ]]; then
  notify+=(-body "$work/report.slack")
else
  notify+=(-details "the report could not be written; the run's log has the scans")
fi
if [[ -n $run_url ]]; then
  notify+=(-run-url "$run_url")
fi
posted=0
SECURITY_SLACK_WEBHOOK_URL=$webhook "${notify[@]}" || posted=$?
exit $((code > posted ? code : posted))
