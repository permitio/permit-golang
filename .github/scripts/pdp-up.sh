#!/usr/bin/env bash
# pdp-up.sh {up | logs | down} runs the container PDP of an end-to-end job of
# .github/workflows/ci.yml as a step, with the API key of the leg's scratch environment, which
# e2e-env.sh create exported. A service container would start before any step, when only a
# fixed key exists, and a PDP with a key of another environment denies every decision. It reads:
#
#   PDP_IMAGE               up: the image, such as permitio/pdp-v2:<version>@sha256:<digest>
#   PERMIT_E2E_ENV_API_KEY  up: the API key of the scratch environment
#   PERMIT_E2E_API_URL      up: the Permit API the PDP syncs from; https://api.permit.io when it
#                           is not set
#   PDP_PORT                the port of 127.0.0.1 that the PDP listens on; 7766 when it is not
#                           set, the port that the SDK's default PDP URL names
#   PDP_HEALTH_TIMEOUT      up: the seconds that the PDP has to become healthy; 300 when it is
#                           not set
#
# up pulls the image, prints the digest it resolved to, which tells what a tag such as latest
# named in the run, and starts the container permit-e2e-pdp with the port published on
# 127.0.0.1 only. The API key goes to docker by name from the environment (--env PDP_API_KEY),
# never on its command line. Then it prints the bound it waits for, and asks
# http://127.0.0.1:<port>/healthy, every 2 seconds, each try up to 5 seconds, until it answers
# 200 or PDP_HEALTH_TIMEOUT seconds have passed since the container started, bounded by the
# elapsed time rather than by a number of tries: before the deadline it sleeps no longer than the
# time left, and it makes one last try at the deadline. Bash's SECONDS counts whole seconds, so
# the deadline is one second more on that clock: the last try starts more than
# PDP_HEALTH_TIMEOUT seconds after the container started, and less than two seconds after that.
# A container that stops running fails at once.
#
# logs prints the container's log without the lines of the health checks. down removes the
# container; one that does not exist is removed already. Everything docker and the PDP print is
# printed with workflow commands stopped.
#
# Exit codes: 0 when the PDP is healthy, its log was printed, or it was removed; 2 when the
# command did not run: the arguments or a setting are wrong, docker or curl is missing, docker
# fails, the container stopped, or the PDP was not healthy in time.
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

# fail prints why the command did not run as an error annotation, and exits 2. Its text holds
# only fixed words and checked values.
fail() {
  resume
  echo "::error title=PDP::$*"
  exit 2
}

[[ $# -eq 1 && ($1 == up || $1 == logs || $1 == down) ]] ||
  fail "usage: pdp-up.sh {up | logs | down}"
command -v docker >/dev/null 2>&1 || fail "docker is not installed"
name=permit-e2e-pdp
port=${PDP_PORT:-7766}
[[ $port =~ ^[1-9][0-9]{0,4}$ ]] || fail "PDP_PORT is not a port number"

case $1 in
logs)
  work=$(mktemp -d "${TMPDIR:-/tmp}/pdp-up.XXXXXX") ||
    fail "creating a temporary directory failed"
  docker logs "$name" >"$work/log" 2>&1 || {
    cat -- "$work/log"
    fail "docker logs $name failed"
  }
  echo "pdp-up: the log of $name, without the lines of the health checks:"
  grep -v -E '"GET /(healthy|ready)[ ?]' "$work/log" || true
  exit 0
  ;;
down)
  if output=$(docker rm --force "$name" 2>&1); then
    echo "pdp-up: removed $name"
  elif [[ $output == *"No such container"* ]]; then
    echo "pdp-up: $name does not exist, so there is nothing to remove"
  else
    echo "$output"
    fail "docker rm --force $name failed"
  fi
  exit 0
  ;;
esac

image=${PDP_IMAGE:-}
[[ $image =~ ^[a-z0-9][a-z0-9._/-]*(:[A-Za-z0-9._-]+)?(@sha256:[0-9a-f]{64})?$ ]] ||
  fail "PDP_IMAGE is not an image reference such as permitio/pdp-v2:<tag>@sha256:<digest>"
[[ -n ${PERMIT_E2E_ENV_API_KEY:-} ]] ||
  fail "PERMIT_E2E_ENV_API_KEY is not set; e2e-env.sh create exports it"
control_plane=${PERMIT_E2E_API_URL:-https://api.permit.io}
timeout=${PDP_HEALTH_TIMEOUT:-300}
[[ $timeout =~ ^[1-9][0-9]{0,3}$ ]] || fail "PDP_HEALTH_TIMEOUT is not a number of seconds"
command -v curl >/dev/null 2>&1 || fail "curl is not installed"

docker pull --quiet "$image" || fail "docker pull $image failed"
digest=$(docker image inspect --format '{{join .RepoDigests " "}}' "$image") ||
  fail "docker image inspect $image failed"
echo "pdp-up: $image resolved to $digest"
PDP_API_KEY=$PERMIT_E2E_ENV_API_KEY PDP_CONTROL_PLANE=$control_plane \
  docker run --detach --name "$name" --publish "127.0.0.1:$port:7000" --env PDP_API_KEY \
  --env PDP_CONTROL_PLANE "$image" || fail "docker run $image failed"

url=http://127.0.0.1:$port/healthy
echo "pdp-up: waiting up to $timeout seconds for $url to answer 200"
start=$SECONDS
deadline=$((start + timeout + 1))
while true; do
  state=$(docker container inspect --format '{{.State.Status}}' "$name") ||
    fail "docker container inspect $name failed"
  [[ $state == running ]] ||
    fail "the PDP container $name is $state, not running; pdp-up.sh logs prints its log"
  try=$((deadline - SECONDS))
  if ((try > 5)); then
    try=5
  elif ((try < 1)); then
    try=1
  fi
  status=$(curl --disable --silent --output /dev/null --max-time "$try" \
    --write-out '%{http_code}' "$url") || status=000
  if [[ $status == 200 ]]; then
    echo "pdp-up: $url answered 200 after $((SECONDS - start)) seconds"
    exit 0
  fi
  left=$((deadline - SECONDS))
  if ((left <= 0)); then
    break
  fi
  if ((left > 2)); then
    left=2
  fi
  sleep "$left"
done
fail "$url did not answer 200 within $timeout seconds; pdp-up.sh logs prints the PDP's log"
