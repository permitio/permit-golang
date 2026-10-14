#!/usr/bin/env bash
# digest.sh writes the weekly security digest and posts it to Slack. The digest job in
# .github/workflows/ci.yml runs it every week, and the digest is sent even when nothing needs
# attention. It runs the same way on any clone. It needs the network, gh, curl and jq, and reads:
#
#   GITHUB_REPOSITORY           owner/name of the repository; required
#   GH_TOKEN                    the token gh reads the pull requests, the runs and the releases
#                               with
#   SECURITY_SLACK_WEBHOOK_URL  the webhook that the digest is posted to
#   GITHUB_SERVER_URL and GITHUB_RUN_ID, which CI sets: the run's URL
#   DIGEST_NOW                  the time of the digest, in seconds since 1970-01-01 UTC, instead
#                               of the clock's, for tests
#
# It reads each secret, unsets it, and passes it to the one command that needs it: the token to
# gh and the webhook to notify (tools/cmd/notify). The digest has five sections, in Slack's text
# format, with every value from outside the repository escaped once:
#
#   Dependabot pull requests  every open pull request of dependabot[bot] with its age; one open
#                             14 days or more needs attention
#   Accepted vulnerabilities  the entries of .github/vulnscan-accept.txt whose last day is within
#                             30 days or past, each of which needs attention
#   Last security scan        the result of the newest security-scan job of CI on master, among
#                             the 30 newest scheduled runs and the 30 newest manual runs; one
#                             that did not succeed, or none, needs attention
#   Go                        for go.mod's toolchain line and each Go release that a test leg of
#                             ci.yml pins (GO_1NN), the newest stable patch of its Go release,
#                             from go.dev/dl, with its release date from go.dev/doc/devel/release;
#                             a newer patch that is at least 7 days old needs attention
#   Release binaries          for each entry of tools/binaries.lock, the newest stable release of
#                             its GitHub project; a newer one published at least 7 days (168
#                             hours) ago needs attention
#
# A section that cannot be read says so in the digest. notify posts the digest under the line
# "<repo> security digest: <result>. <run URL>", where the result counts what needs attention, or
# says that the digest did not complete; without the webhook, notify warns and posts nothing.
# Everything until notify runs is printed with workflow commands stopped.
#
# Exit codes: 0 when every section was read and the digest posted, or the webhook is not set; 1
# when the digest could not be posted; 2 when a section could not be read (the digest is posted
# all the same), or the digest did not run: the arguments are wrong, jq, gh or curl is missing,
# GITHUB_REPOSITORY is not owner/name, DIGEST_NOW is not a number, or notify cannot be built.
# shellcheck disable=SC2329 # section runs each section's function by name.
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
  echo "digest: $*"
  echo "digest: DID NOT RUN"
  exit 2
}

[[ $# -eq 0 ]] || fail "usage: digest.sh (it takes no arguments)"
for tool in jq gh curl; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is not installed"
done
root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
cd -- "$root"

gh_token=${GH_TOKEN:-}
webhook=${SECURITY_SLACK_WEBHOOK_URL:-}
unset GH_TOKEN GITHUB_TOKEN SECURITY_SLACK_WEBHOOK_URL SLACK_WEBHOOK_URL
repo=${GITHUB_REPOSITORY:-}
repo_re='^[A-Za-z0-9-]+/[A-Za-z0-9_.-]+$'
[[ $repo =~ $repo_re ]] || fail "GITHUB_REPOSITORY \"$repo\" is not owner/name; set it to the" \
  "repository to report on, as CI does"
now=${DIGEST_NOW:-$(date -u +%s)}
[[ $now =~ ^[0-9]+$ ]] || fail "DIGEST_NOW \"$now\" is not a number of seconds"

work=$(mktemp -d "${TMPDIR:-/tmp}/digest.XXXXXX") || fail "creating a temporary directory failed"
echo "digest: building notify"
go -C tools build -o "$root/bin/notify" ./cmd/notify || fail "building notify failed"

digest=$work/digest
problems=0
attention=0
# esc escapes a value for Slack's text format; days is the number of whole days from a time, in
# seconds, to now; ver is a version's numbers, for comparing versions.
# shellcheck disable=SC2016 # $now is a jq variable, which jq --argjson sets
defs='def esc: gsub("&"; "&amp;") | gsub("<"; "&lt;") | gsub(">"; "&gt;");
  def days: ($now - .) / 86400 | floor;
  def ver: ltrimstr("v") | ltrimstr("go") | split(".") | map(try tonumber catch -1);'

# api runs gh api with the token.
api() {
  GH_TOKEN=$gh_token gh api "$@"
}

# section <title> <function> writes a section of the digest. The function prints its lines and
# returns 0, or 2 when it could not read what it reports; it runs with errexit off, so it checks
# each command it runs.
section() {
  local code=0
  printf '*%s*\n' "$1" >>"$digest"
  "$2" >>"$digest" 2>"$work/error" || code=$?
  if [[ $code -ne 0 ]]; then
    problems=$((problems + 1))
    echo "• Could not be read; the run's log says why." >>"$digest"
    echo "digest: $1 could not be read:"
    cat -- "$work/error"
  fi
}

# need <count> adds the count of items that need attention.
need() {
  attention=$((attention + $1))
}

dependabot_pulls() {
  local flagged
  api --paginate "repos/$repo/pulls?state=open&per_page=100" >"$work/pulls.json" || return 2
  jq -se 'all(.[]; type == "array") and length > 0' "$work/pulls.json" >/dev/null || {
    echo "digest: gh returned something other than lists of pull requests" >&2
    return 2
  }
  jq -s --argjson now "$now" "$defs"'[add[] | select(.user.login == "dependabot[bot]")
    | {number, title, url: .html_url, days: (.created_at | fromdateiso8601 | days)}]
    | sort_by(-.days, .number)' "$work/pulls.json" >"$work/dependabot.json" || return 2
  flagged=$(jq '[.[] | select(.days >= 14)] | length' "$work/dependabot.json") || return 2
  jq -r "$defs"'if length == 0 then "• None open." else .[]
    | "• <\(.url)|#\(.number)> \(.title | esc): open \(.days) days"
      + (if .days >= 14 then ", *14 days or more*" else "" end) end' \
    "$work/dependabot.json" || return 2
  need "$flagged"
}

accepted_vulnerabilities() {
  local due
  # left is the number of days from today, in UTC, to the entry's last day.
  awk '!/^[[:space:]]*(#|$)/ { print $1, $2, $NF }' .github/vulnscan-accept.txt |
    jq -R --argjson now "$now" 'split(" ") | {id: .[0], module: .[1], last: .[2],
      left: (((.[2] | strptime("%Y-%m-%d") | mktime) - ($now / 86400 | floor) * 86400) / 86400)}
      | select(.left <= 30)' | jq -s 'sort_by(.left, .id)' >"$work/due.json" || return 2
  due=$(jq length "$work/due.json") || return 2
  jq -r "$defs"'if length == 0 then "• None due within 30 days." else .[]
    | "• \(.id) in \(.module | esc): " + (if .left < 0 then "expired on \(.last)"
      elif .left == 0 then "its last day is today, \(.last)"
      else "its last day is \(.last), in \(.left) days" end) end' "$work/due.json" || return 2
  need "$due"
}

last_scan() {
  local ids id line event
  # Each event is read on its own, so that pushes to master do not crowd the scans out.
  for event in schedule workflow_dispatch; do
    api "repos/$repo/actions/workflows/ci.yml/runs?branch=master&event=$event&per_page=30" \
      >"$work/runs-$event.json" || return 2
  done
  ids=$(jq -rs '[.[].workflow_runs[] | select(.event == "schedule"
      or .event == "workflow_dispatch")] | sort_by(.created_at) | reverse | .[]
    | .id | tostring | select(test("^[0-9]+$"))' "$work/runs-schedule.json" \
    "$work/runs-workflow_dispatch.json") || return 2
  for id in $ids; do
    api "repos/$repo/actions/runs/$id/jobs?per_page=100" >"$work/jobs.json" || return 2
    line=$(jq -r "$defs"'[.jobs[] | select(.name == "security-scan" and .conclusion != null
      and .conclusion != "skipped")][0] // empty
      | "\(.conclusion) • <\(.html_url)|\(.conclusion | esc)> on \(.completed_at[0:10])"' \
      "$work/jobs.json") || return 2
    if [[ -n $line ]]; then
      echo "${line#* }"
      [[ ${line%% *} == success ]] || need 1
      return 0
    fi
  done
  echo "• No security-scan job in the 30 newest scheduled runs or the 30 newest manual runs of" \
    "CI on master."
  need 1
}

go_release() {
  local toolchain count
  toolchain=$(awk '$1 == "toolchain" { print $2; exit }' go.mod) || return 2
  if [[ ! $toolchain =~ ^go1\.[0-9]+\.[0-9]+$ ]]; then
    echo "digest: go.mod has no toolchain line with a Go patch release" >&2
    return 2
  fi
  # The patch releases that ci.yml's test legs pin as GO_1NN; GO_FLOOR is the minimum and stays.
  sed -nE 's/^ +(GO_1[0-9]+): "(1\.[0-9]+\.[0-9]+)"$/\1 go\2/p' .github/workflows/ci.yml \
    >"$work/legs" || return 2
  if [[ ! -s $work/legs ]]; then
    echo "digest: .github/workflows/ci.yml pins no Go release for a test leg (GO_1NN)" >&2
    return 2
  fi
  curl --disable --fail --silent --show-error --location --proto '=https' --max-time 60 \
    --output "$work/dl.json" 'https://go.dev/dl/?mode=json&include=all' || return 2
  curl --disable --fail --silent --show-error --location --proto '=https' --max-time 60 \
    --output "$work/release.html" 'https://go.dev/doc/devel/release' || return 2
  tr -s ' \t\r\n' ' ' <"$work/release.html" |
    grep -oE 'go1\.[0-9]+\.[0-9]+ \(released [0-9]{4}-[0-9]{2}-[0-9]{2}\)' |
    jq -R 'capture("^(?<version>[^ ]+) \\(released (?<date>[^)]+)\\)$")' |
    jq -s 'map({(.version): .date}) | add // {}' >"$work/dates.json" || return 2
  { echo "toolchain $toolchain" && cat -- "$work/legs"; } >"$work/pins" || return 2
  # Each pin is "<name> <version>": the toolchain line, then each test leg's GO_1NN.
  jq -r --argjson now "$now" --arg toolchain "$toolchain" --rawfile pins "$work/pins" \
    --slurpfile dates "$work/dates.json" "$defs"'$dates[0] as $date | . as $dl
    | if type != "array" then error("go.dev/dl did not return a list of releases")
      elif $date[$toolchain] == null then
        error("go.dev/doc/devel/release has no release date for \($toolchain)")
      else . end
    | $pins | split("\n")[] | select(. != "") | split(" ")
    | {name: (if .[0] == "toolchain" then "the toolchain line of go.mod"
          else .[0] + " in .github/workflows/ci.yml" end),
        current: (if .[0] == "toolchain" then .[1] else .[1] | ltrimstr("go") end),
        version: .[1], series: (.[1] | sub("\\.[0-9]+$"; ""))} as $pin
    | [$dl[] | select(.stable == true) | .version | select(startswith($pin.series + "."))
      | select(ver > ($pin.version | ver))] | unique | sort_by(ver)
    | map({version: ., date: $date[.]}
      | .ready = (if .date == null then null
          else (.date | strptime("%Y-%m-%d") | mktime) + 7 * 86400 end))
    | (map(select(.ready != null and .ready <= $now)) | last) as $adopt
    | (map(select(.ready == null or .ready > $now)) | last) as $later
    | if length == 0 then "• \($pin.current), \($pin.name), is the newest \($pin.series)."
      else (if $adopt == null then empty
        else "need • \($adopt.version), released \($adopt.date), can be adopted: move "
          + "\($pin.name) from \($pin.current)." end),
        (if $later == null then empty
        elif $later.date == null then
          "• \($later.version), newer than \($pin.name), has no release date on go.dev yet."
        else "• \($later.version), released \($later.date), can be adopted for \($pin.name) "
          + "from " + ($later.ready | strftime("%Y-%m-%d")) + "." end) end' \
    "$work/dl.json" >"$work/go.txt" || return 2
  count=$(awk '/^need / { count++ } END { print count + 0 }' "$work/go.txt") || return 2
  need "$count"
  sed 's/^need //' "$work/go.txt"
}

release_binaries() {
  local name version url project
  awk '/^\[/ { if (name != "") print name, version, url; name = substr($0, 2, length($0) - 2)
      version = ""; url = ""; next }
    /^version[[:space:]]*=/ { sub(/^version[[:space:]]*=[[:space:]]*/, ""); version = $0 }
    /^url[[:space:]]*=/ { sub(/^url[[:space:]]*=[[:space:]]*/, ""); url = $0 }
    END { if (name != "") print name, version, url }' tools/binaries.lock >"$work/lock" ||
    return 2
  [[ -s $work/lock ]] || {
    echo "digest: tools/binaries.lock has no entries" >&2
    return 2
  }
  while read -r name version url; do
    project_re='^https://github\.com/([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)/releases/download/'
    if [[ ! $url =~ $project_re ]]; then
      echo "• $name $version: its url is not a GitHub release, so check it by hand."
      need 1
      continue
    fi
    project=${BASH_REMATCH[1]}
    api "repos/$project/releases?per_page=30" >"$work/releases.json" || return 2
    jq -r --argjson now "$now" --arg name "$name" --arg version "$version" "$defs"'
      if type != "array" then error("gh returned something other than a list of releases")
      else . end
      | [.[] | select(.draft == false and .prerelease == false and .published_at != null)
        | {version: (.tag_name | ltrimstr("v")), date: .published_at,
          ready: ((.published_at | fromdateiso8601) + 7 * 86400)}
        | select(.version | ver > ($version | ver))] | sort_by(.version | ver)
      | (map(select(.ready <= $now)) | last) as $adopt
      | (map(select(.ready > $now)) | last) as $later
      | if length == 0 then "• \($name) \($version) is the newest release."
        else (if $adopt == null then empty
          else "need • \($name) \($version): \($adopt.version | esc), published "
            + "\($adopt.date[0:10]), can be adopted." end),
          (if $later == null then empty
          else "• \($name) \($version): \($later.version | esc), published "
            + "\($later.date[0:10]), can be adopted from \($later.ready | strftime("%Y-%m-%d"))."
          end) end' "$work/releases.json" >"$work/release.txt" || return 2
    if grep -q '^need ' "$work/release.txt"; then
      need 1
    fi
    sed 's/^need //' "$work/release.txt"
  done <"$work/lock"
}

section "Dependabot pull requests" dependabot_pulls
section "Accepted vulnerabilities" accepted_vulnerabilities
section "Last security scan" last_scan
section Go go_release
section "Release binaries" release_binaries
cat -- "$digest"

result="nothing needs attention"
if [[ $problems -gt 0 ]]; then
  result="did not complete: $problems sections could not be read"
elif [[ $attention -gt 0 ]]; then
  result="$attention items need attention"
fi
notify=(bin/notify -channel security -repo "$repo" -check "security digest" -result "$result"
  -body "$digest")
if [[ -n ${GITHUB_SERVER_URL:-} && -n ${GITHUB_RUN_ID:-} ]]; then
  notify+=(-run-url "$GITHUB_SERVER_URL/$repo/actions/runs/$GITHUB_RUN_ID")
fi
resume
posted=0
SECURITY_SLACK_WEBHOOK_URL=$webhook "${notify[@]}" || posted=$?
if [[ $problems -gt 0 ]]; then
  echo "digest: DID NOT COMPLETE: $problems sections could not be read"
  exit 2
fi
exit "$posted"
