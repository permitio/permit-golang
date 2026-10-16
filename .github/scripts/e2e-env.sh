#!/usr/bin/env bash
# e2e-env.sh {create | delete} <leg> creates or deletes the scratch environment of one leg of the
# end-to-end jobs of .github/workflows/ci.yml: the environment gosdk-ci-<run>-<attempt>-<leg> of
# the Permit project PERMIT_E2E_PROJECT, which the leg's container PDP and its tests use, and
# which the leg deletes at its end. <run> and <attempt> are GITHUB_RUN_ID and GITHUB_RUN_ATTEMPT;
# <leg> is 1 to 16 lower-case letters and digits that start with a letter, such as pinned, as
# the run prefix of tools/e2e/framework takes it. It reads:
#
#   PERMIT_E2E_PROJECT_KEY  a project-level API key of the project: one that can create, read and
#                           delete its environments and read their API keys
#   PERMIT_E2E_PROJECT      the key or the ID of the project
#   PERMIT_E2E_API_URL      the URL of the Permit API; https://api.permit.io when it is not set
#   GITHUB_RUN_ID, GITHUB_RUN_ATTEMPT, and for create GITHUB_ENV, which the runner sets
#   GITHUB_EVENT_NAME, GITHUB_REF and MODE (the workflow's mode input), for the rule below
#
# create: without PERMIT_E2E_PROJECT_KEY, the leg cannot run. That passes with a notice on a
# pull request, which a fork's or Dependabot's may be, and in a manual run (workflow_dispatch)
# of the mode e2e from a ref other than master, and appends created=false to GITHUB_OUTPUT; a
# push to master, a release and a manual run from master fail, so that they never pass without
# the tests. With the key, it creates the environment with POST /v2/projects/<project>/envs and
# reads its API key with GET /v2/api-key/<project ID>/<environment ID>. It masks the API key with
# ::add-mask:: before anything else, and only then appends PERMIT_E2E_ENV_API_KEY,
# PERMIT_E2E_ENV_ID and PERMIT_E2E_PROJECT_ID to GITHUB_ENV, for the later steps of the job, and
# created=true to GITHUB_OUTPUT when that is set. A 401 or 403 to either request says which key
# level the lane needs.
#
# delete: deletes the environment with DELETE /v2/projects/<project>/envs/<key>. A 404 means
# that it is gone already or was never created; without PERMIT_E2E_PROJECT_KEY the leg created
# none. Any other reply, or no reply, leaves the environment behind: the script warns that it
# leaked, and passes, as the sweep at the start of a later e2e job (sweep-envs.sh) deletes it
# once it is 24 hours old.
#
# Replies are printed with workflow commands stopped; only the script's own annotations and the
# ::add-mask:: line are printed with them on. The API key goes to curl in a header file, not on
# its command line, and is never printed.
#
# Exit codes: 0 when the environment was created or deleted, the rule above passes the leg
# without the key, or a delete leaked the environment; 2 when the script did not run: the
# arguments or a setting are wrong, jq or curl is missing, the key is missing where the rule
# needs it, a request of create fails or its reply is not what the API documents, or the exports
# cannot be written.
set -euo pipefail

work=''
token=''
stopped=false
# stop stops workflow commands with a new random token; resume resumes them, once.
stop() {
  token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
  echo "::stop-commands::$token"
  stopped=true
}
resume() {
  if [[ $stopped == true ]]; then
    echo "::$token::"
    stopped=false
  fi
}
trap 'resume; if [[ -n $work ]]; then rm -rf -- "$work"; fi' EXIT
stop

# fail prints why the script did not run as an error annotation, and exits 2. Its text holds
# only fixed words and checked values.
fail() {
  resume
  echo "::error title=e2e environment::$*"
  exit 2
}

[[ $# -eq 2 && ($1 == create || $1 == delete) ]] ||
  fail "usage: e2e-env.sh {create | delete} <leg>"
command=$1
leg=$2
[[ $leg =~ ^[a-z][a-z0-9]{0,15}$ ]] ||
  fail "the leg is not 1 to 16 lower-case letters and digits that start with a letter"
for tool in jq curl; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is not installed"
done

key=${PERMIT_E2E_PROJECT_KEY:-}
if [[ -z $key ]]; then
  if [[ $command == delete ]]; then
    echo "e2e-env: PERMIT_E2E_PROJECT_KEY is not set, so the leg created no environment"
    exit 0
  fi
  event=${GITHUB_EVENT_NAME:-}
  manual=false
  if [[ $event == workflow_dispatch && ${MODE:-} == e2e ]]; then
    manual=true
  fi
  if [[ $event == pull_request || ($manual == true && ${GITHUB_REF:-} != refs/heads/master) ]]; then
    if [[ -n ${GITHUB_OUTPUT:-} ]]; then
      echo "created=false" >>"$GITHUB_OUTPUT" || fail "writing to GITHUB_OUTPUT failed"
    fi
    resume
    echo "::notice title=e2e::PERMIT_E2E_PROJECT_KEY is not set, so the $leg leg does not run" \
      "the end-to-end tests; a push to master and a release fail without it, which the" \
      "integration environment holds"
    exit 0
  fi
  fail "PERMIT_E2E_PROJECT_KEY is not set: a push to master, a release and a manual run from" \
    "master must run the end-to-end tests; set the secret in the integration environment"
fi

project=${PERMIT_E2E_PROJECT:-}
id_re='^[A-Za-z0-9_-]{1,128}$'
[[ $project =~ $id_re ]] ||
  fail "PERMIT_E2E_PROJECT is not the key or the ID of a project (letters, digits, - and _)"
api=${PERMIT_E2E_API_URL:-https://api.permit.io}
api=${api%/}
[[ $api =~ ^https?://[A-Za-z0-9.-]+(:[0-9]{1,5})?$ ]] ||
  fail "PERMIT_E2E_API_URL is not an http or https URL of a host, without a path"
run=${GITHUB_RUN_ID:-}
attempt=${GITHUB_RUN_ATTEMPT:-}
[[ $run =~ ^[0-9]{1,20}$ && $attempt =~ ^[0-9]{1,20}$ ]] ||
  fail "GITHUB_RUN_ID and GITHUB_RUN_ATTEMPT must be decimal numbers"
env_key=gosdk-ci-$run-$attempt-$leg

work=$(mktemp -d "${TMPDIR:-/tmp}/e2e-env.XXXXXX") || fail "creating a temporary directory failed"
(umask 077 && printf 'Authorization: Bearer %s\n' "$key" >"$work/auth") ||
  fail "writing the request header failed"

# request <method> <path> [<body file>] sends a request to the API and sets status to the reply's
# status code, or to 000 when no reply came, and leaves the reply's body in $work/reply.
request() {
  local args=(--disable --silent --show-error --proto '=http,https' --max-time 60
    --header @"$work/auth" --header 'Accept: application/json'
    --output "$work/reply" --write-out '%{http_code}' --request "$1")
  if [[ $# -eq 3 ]]; then
    args+=(--header 'Content-Type: application/json' --data-binary @"$3")
  fi
  : >"$work/reply"
  status=$(curl "${args[@]}" "$api$2") || status=000
}

# show prints the reply's body, cut to 1000 bytes, with workflow commands stopped.
show() {
  echo "e2e-env: the reply of $1 was:"
  head -c 1000 "$work/reply"
  echo
}

# cannot fails with the key level the lane needs, after a 401 or 403 to the request $1.
cannot() {
  show "$1"
  fail "$1 answered $status: PERMIT_E2E_PROJECT_KEY cannot $2. The end-to-end jobs need a" \
    "project-level API key of the project $project, which can create, read and delete its" \
    "environments and read their API keys; an environment-level key cannot"
}

envs=/v2/projects/$project/envs
if [[ $command == delete ]]; then
  request DELETE "$envs/$env_key"
  case $status in
  200 | 204) echo "e2e-env: deleted the environment $env_key" ;;
  404) echo "e2e-env: the environment $env_key is gone already, or was never created" ;;
  *)
    show "DELETE $envs/$env_key"
    resume
    echo "::warning title=e2e environment leaked::The scratch environment $env_key of the" \
      "project $project leaked: DELETE answered $status. The sweep at the start of a later e2e" \
      "job deletes it once it is 24 hours old."
    ;;
  esac
  exit 0
fi

[[ -n ${GITHUB_ENV:-} ]] || fail "GITHUB_ENV is not set, so nothing can be exported"
jq -n --arg key "$env_key" --arg description "A scratch environment of the Go SDK's tests" \
  '{key: $key, name: $key, description: $description}' >"$work/create.json" ||
  fail "writing the request body failed"
request POST "$envs" "$work/create.json"
case $status in
200 | 201) ;;
401 | 403) cannot "POST $envs" "create environments in it" ;;
*)
  show "POST $envs"
  fail "POST $envs answered $status, so the environment $env_key was not created"
  ;;
esac
created=$(jq -r --arg key "$env_key" 'select(.key == $key and (.id | type) == "string"
  and (.project_id | type) == "string") | "\(.id) \(.project_id)"' "$work/reply" 2>/dev/null) ||
  created=''
read -r env_id project_id <<<"$created" || true
if [[ ! ${env_id:-} =~ $id_re || ! ${project_id:-} =~ $id_re ]]; then
  show "POST $envs"
  fail "POST $envs did not reply with the environment $env_key, its ID and its project's ID"
fi
echo "e2e-env: created the environment $env_key (ID $env_id, project $project_id)"

request GET "/v2/api-key/$project_id/$env_id"
case $status in
200) ;;
401 | 403) cannot "GET /v2/api-key/$project_id/$env_id" "read the API key of the environment" ;;
*)
  show "GET /v2/api-key/$project_id/$env_id"
  fail "GET /v2/api-key/$project_id/$env_id answered $status, so the environment's API key" \
    "could not be read"
  ;;
esac
secret=$(jq -r '.secret | strings' "$work/reply" 2>/dev/null) || secret=''
# The key is never shown, so an unexpected one is reported without it.
[[ $secret =~ ^[A-Za-z0-9_-]{8,255}$ ]] ||
  fail "GET /v2/api-key/$project_id/$env_id did not reply with a secret of letters, digits, -" \
    "and _"

resume
echo "::add-mask::$secret"
{
  echo "PERMIT_E2E_ENV_API_KEY=$secret"
  echo "PERMIT_E2E_ENV_ID=$env_id"
  echo "PERMIT_E2E_PROJECT_ID=$project_id"
} >>"$GITHUB_ENV" || fail "writing the environment's settings to GITHUB_ENV failed"
if [[ -n ${GITHUB_OUTPUT:-} ]]; then
  echo "created=true" >>"$GITHUB_OUTPUT" || fail "writing to GITHUB_OUTPUT failed"
fi
echo "e2e-env: exported the API key, the ID and the project ID of $env_key"
