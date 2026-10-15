#!/usr/bin/env bash
# sweep-envs.sh deletes the scratch environments that end-to-end jobs left behind, such as one
# whose delete at the end of its leg failed: every environment of the project of
# PERMIT_E2E_PROJECT_KEY whose key is a run prefix, gosdk-ci-<run>-<attempt>-<leg> as
# e2e-env.sh names them, and that was created more than 24 hours ago. A leg runs for minutes, so
# no running leg's environment is that old. Nothing else is deleted: no environment with another
# key, such as production, at any age; no environment younger than 24 hours; no environment whose
# creation time it cannot read; and nothing outside that project. The e2e job of
# .github/workflows/ci.yml runs it at its start, as the lane has no scheduled run. It reads:
#
#   PERMIT_E2E_PROJECT_KEY  a project-level API key, which can list and delete the environments
#                           of its project; without it there is nothing to sweep
#   PERMIT_E2E_API_URL      the URL of the Permit API; https://api.permit.io when it is not set
#
# It reads the project from the key's scope (GET /v2/api-key/scope), and refuses a key whose
# scope is not a project, such as an environment-level or an organization-level key. It lists
# the environments with GET /v2/projects/<project ID>/envs, 100 a page, and deletes each match
# with DELETE /v2/projects/<project ID>/envs/<environment ID>; a 404 means it is gone already.
# Replies are printed with workflow commands stopped. The API key goes to curl in a header file,
# not on its command line, and is never printed.
#
# The sweep is housekeeping: what it leaves, the sweep of a later run deletes. So it warns and
# passes when a delete fails, when the scope or the list cannot be read for a reason other than
# the key, such as no reply or a 5xx, or when the project has more than 100 pages of
# environments, and fails only on the configuration.
#
# Exit codes: 0 when it deleted every match, found none, or warned that it could not; 2 when the
# sweep did not run: an argument is given, jq or curl is missing, a setting is wrong, the API
# refused the key (401 or 403), the key is not project-level, or a temporary file cannot be
# written.
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

# fail prints why the sweep did not run as an error annotation, and exits 2. Its text holds only
# fixed words and checked values.
fail() {
  resume
  echo "::error title=Sweep::$*"
  exit 2
}

# give_up prints why the sweep stopped as a warning, and exits 0, as the sweep is housekeeping.
# Its text holds only fixed words and checked values.
give_up() {
  resume
  echo "::warning title=Sweep::$*"
  exit 0
}
later='so the environments were not swept; a later run sweeps them'

[[ $# -eq 0 ]] || fail "usage: sweep-envs.sh (it takes no arguments)"
for tool in jq curl; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is not installed"
done
key=${PERMIT_E2E_PROJECT_KEY:-}
if [[ -z $key ]]; then
  echo "sweep-envs: PERMIT_E2E_PROJECT_KEY is not set, so there is nothing to sweep"
  exit 0
fi
api=${PERMIT_E2E_API_URL:-https://api.permit.io}
api=${api%/}
[[ $api =~ ^https?://[A-Za-z0-9.-]+(:[0-9]{1,5})?$ ]] ||
  fail "PERMIT_E2E_API_URL is not an http or https URL of a host, without a path"

work=$(mktemp -d "${TMPDIR:-/tmp}/sweep-envs.XXXXXX") ||
  fail "creating a temporary directory failed"
(umask 077 && printf 'Authorization: Bearer %s\n' "$key" >"$work/auth") ||
  fail "writing the request header failed"

# request <method> <path> sends a request to the API and sets status to the reply's status code,
# or to 000 when no reply came, and leaves the reply's body in $work/reply.
request() {
  : >"$work/reply"
  status=$(curl --disable --silent --show-error --proto '=http,https' --max-time 60 \
    --header @"$work/auth" --header 'Accept: application/json' --output "$work/reply" \
    --write-out '%{http_code}' --request "$1" "$api$2") || status=000
}

# expect <request> returns when the last reply's status is 200. Otherwise it prints the reply,
# and fails when the API refused the key (401 or 403), or gives up.
expect() {
  [[ $status == 200 ]] && return
  echo "sweep-envs: the reply of $1 was:"
  head -c 1000 "$work/reply"
  echo
  if [[ $status == 401 || $status == 403 ]]; then
    fail "$1 answered $status: the API refused PERMIT_E2E_PROJECT_KEY. The sweep needs a" \
      "project-level API key, which can list and delete the environments of its project"
  fi
  give_up "$1 answered $status, $later"
}

id_re='^[A-Za-z0-9_-]{1,128}$'
request GET /v2/api-key/scope
expect "GET /v2/api-key/scope"
jq -e 'type == "object"' "$work/reply" >/dev/null 2>&1 ||
  give_up "GET /v2/api-key/scope did not reply with a scope, $later"
scope=$(jq -r 'if (.project_id | type) == "string" and .environment_id == null
  then .project_id else "" end' "$work/reply" 2>/dev/null) || scope=''
[[ $scope =~ $id_re ]] ||
  fail "PERMIT_E2E_PROJECT_KEY is not a project-level API key: its scope names no project, or" \
    "names an environment too, so the sweep does not know which project's environments it may" \
    "delete"

# The ID, key and creation time of each environment, the time in seconds since the epoch, or an
# empty field when created_at is not an ISO 8601 date and time. A time without an offset is UTC.
# An environment without a string ID and key is left out.
# shellcheck disable=SC2016 # $z is a jq variable
rows='def epoch:
    capture("^(?<t>[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2})(\\.[0-9]+)?"
      + "(?<z>Z|[+-][0-9]{2}:[0-9]{2})?$")
    | (.z // "Z") as $z
    | (.t + "Z" | fromdateiso8601) - (if $z == "Z" then 0
      else ($z[1:3] | tonumber) * 3600 + ($z[4:6] | tonumber) * 60
        | if $z[0:1] == "+" then . else 0 - . end
      end);
  .[] | [(.id | strings), (.key | strings), ((.created_at | strings | epoch) // "")] | @tsv'
listed=$work/listed.tsv
: >"$listed"
per_page=100
for ((page = 1; ; page++)); do
  ((page <= 100)) || give_up "the project has more than 100 pages of environments, which the" \
    "sweep does not read; delete its scratch environments older than 24 hours by hand"
  request GET "/v2/projects/$scope/envs?page=$page&per_page=$per_page"
  expect "GET /v2/projects/$scope/envs"
  count=$(jq 'if type == "array" then length else error("not a list") end' "$work/reply" \
    2>/dev/null) ||
    give_up "GET /v2/projects/$scope/envs did not reply with a list of environments, $later"
  jq -r "$rows" "$work/reply" >>"$listed" 2>/dev/null ||
    give_up "GET /v2/projects/$scope/envs did not reply with a list of environments, $later"
  ((count == per_page)) || break
done

now=$(date +%s)
key_re='^gosdk-ci-[0-9]{1,20}-[0-9]{1,20}-[a-z][a-z0-9]{0,15}$'
total=0 matched=0 deleted=0 failed=0
while IFS=$'\t' read -r id env_key created <&3; do
  total=$((total + 1))
  [[ $env_key =~ $key_re ]] || continue
  matched=$((matched + 1))
  if [[ ! $created =~ ^-?[0-9]+$ ]]; then
    echo "sweep-envs: keeping $env_key: its creation time cannot be read"
    continue
  fi
  age=$((now - created))
  if ((age <= 24 * 3600)); then
    echo "sweep-envs: keeping $env_key: it is $((age / 3600)) hours old"
    continue
  fi
  if [[ ! $id =~ $id_re ]]; then
    echo "sweep-envs: keeping $env_key: its ID is not letters, digits, - and _"
    continue
  fi
  request DELETE "/v2/projects/$scope/envs/$id"
  case $status in
  200 | 204 | 404)
    echo "sweep-envs: deleted $env_key, $((age / 3600)) hours old"
    deleted=$((deleted + 1))
    ;;
  *)
    echo "sweep-envs: deleting $env_key answered $status:"
    head -c 1000 "$work/reply"
    echo
    failed=$((failed + 1))
    ;;
  esac
done 3<"$listed"

echo "sweep-envs: the project holds $total environments, $matched of them scratch" \
  "environments; deleted $deleted older than 24 hours, and $failed could not be deleted"
if ((failed > 0)); then
  resume
  echo "::warning title=Sweep::$failed scratch environments older than 24 hours could not be" \
    "deleted; the log of this step lists them, and the sweep of a later run tries again"
fi
