#!/usr/bin/env bash
# ci-aggregate.sh decides the result of the CI job, the one check the ruleset requires, from the
# results of the jobs it needs. The CI job in .github/workflows/ci.yml runs it with:
#   NEEDS              toJSON(needs): the id and result of each job CI needs
#   EXPECTED_JOBS      the number of jobs CI needs
#   EVENT              github.event_name; in a run that release.yml calls, the caller's event
#   INPUT_MODE         inputs.mode: empty, or pr, release, e2e, security or weekly
#   SCHEDULE           github.event.schedule: the cron that started a scheduled run
#   <MODE>_CRON        the crons of the workflow env, such as WEEKLY_CRON for mode weekly
#   PULL_REQUEST_JOBS  the jobs that run on pull requests only (space separated)
#   SCHEDULED_JOBS     the periodic jobs, as job:mode, each selected in its mode only
#   E2E_JOBS           the end-to-end jobs, selected on a push, in mode e2e and in a release
#   ADVISORY_JOBS      the jobs that may fail or be cancelled without failing CI
#
# The run's mode is INPUT_MODE when it is set. Otherwise it is the mode whose <MODE>_CRON is
# SCHEDULE on a scheduled run, and the event, pull_request or push, on any other run. The modes
# pull_request, push, pr and release select the pull request checks: every job in no list, or in
# ADVISORY_JOBS only. A job in PULL_REQUEST_JOBS is selected in mode pull_request only, a job in
# SCHEDULED_JOBS in its own mode only, and a job in E2E_JOBS in the modes push, e2e and release
# only. A job the mode does not select may be skipped. Every other result than success fails CI,
# except that a job in ADVISORY_JOBS may fail or be cancelled; that is printed as a warning.
#
# Exit codes: 0 when every needed job succeeded or was not selected; 1 when a job failed, was
# cancelled, or was skipped in a run that selects it; 2 when the check did not run: the script
# was given arguments, jq is not installed, NEEDS is not a JSON object of job results or does not
# hold EXPECTED_JOBS of them, a list names a job CI does not need, or the mode, event or cron is
# not one CI knows.
set -euo pipefail

fail() {
  echo "::error title=CI::$*"
  exit 2
}

# contains <list> <word> succeeds when the space-separated list holds the word.
contains() {
  [[ " $1 " == *" $2 "* ]]
}

[[ $# -eq 0 ]] || fail "usage: ci-aggregate.sh (it takes no arguments)"
command -v jq >/dev/null 2>&1 || fail "jq is not installed, so the job results cannot be read"

expected=${EXPECTED_JOBS:-}
[[ $expected =~ ^[1-9][0-9]*$ ]] || fail "EXPECTED_JOBS \"$expected\" is not a positive number"
event=${EVENT:-}
input_mode=${INPUT_MODE:-}
schedule=${SCHEDULE:-}
pull_request_jobs=${PULL_REQUEST_JOBS:-}
scheduled_jobs=${SCHEDULED_JOBS:-}
e2e_jobs=${E2E_JOBS:-}
advisory_jobs=${ADVISORY_JOBS:-}
gate_modes='pull_request push pr release'
e2e_modes='push e2e release'
periodic_modes='e2e security weekly'
mode_re='^[a-z0-9_-]+$'

if [[ -n $input_mode ]]; then
  # A mode that is not a plain word is not printed, so it cannot issue a workflow command.
  [[ $input_mode =~ $mode_re ]] || fail "the mode input is not one of pr, release, e2e," \
    "security and weekly"
  contains "pr release $periodic_modes" "$input_mode" ||
    fail "the mode \"$input_mode\" is not one of pr, release, e2e, security and weekly"
  mode=$input_mode
else
  case $event in
  pull_request | push) mode=$event ;;
  schedule)
    mode=''
    for var in $(compgen -v); do
      [[ $var =~ ^([A-Z][A-Z0-9]*)_CRON$ ]] || continue
      cron_mode=$(tr '[:upper:]' '[:lower:]' <<<"${BASH_REMATCH[1]}")
      [[ ${!var} == "$schedule" ]] || continue
      [[ -z $mode ]] || fail "two <MODE>_CRON variables are \"$schedule\""
      contains "$periodic_modes" "$cron_mode" ||
        fail "$var names the mode $cron_mode, which is not one of $periodic_modes"
      mode=$cron_mode
    done
    [[ -n $mode ]] || fail "the cron \"$schedule\" is not the value of any <MODE>_CRON variable"
    ;;
  *) fail "CI does not run on the event \"$event\" without a mode" ;;
  esac
fi

if ! results=$(jq -r 'if type == "object" then to_entries[] | "\(.key)\t\(.value.result)"
  else error("not an object") end' <<<"${NEEDS:-}" 2>/dev/null); then
  fail "NEEDS is not a JSON object of job results"
fi

jobs=''
count=0
while IFS=$'\t' read -r job result; do
  [[ -n $job ]] || continue
  [[ $job =~ ^[A-Za-z0-9_-]+$ ]] ||
    fail "NEEDS has a job id with characters other than letters, digits, - and _"
  jobs+=" $job"
  count=$((count + 1))
done <<<"$results"
[[ $count -eq $expected ]] ||
  fail "$count job results, but EXPECTED_JOBS is $expected; CI did not check every job"

# check_needed <list name> <jobs> fails when the list names a job CI does not need.
check_needed() {
  local job
  for job in $2; do
    contains "$jobs" "$job" || fail "$1 names $job, which CI does not need"
  done
}
check_needed PULL_REQUEST_JOBS "$pull_request_jobs"
check_needed E2E_JOBS "$e2e_jobs"
check_needed ADVISORY_JOBS "$advisory_jobs"
for entry in $scheduled_jobs; do
  if [[ $entry != *:* ]] || ! contains "$periodic_modes" "${entry#*:}"; then
    fail "SCHEDULED_JOBS entry \"$entry\" is not job:<mode>, with a mode of $periodic_modes"
  fi
  check_needed SCHEDULED_JOBS "${entry%%:*}"
done

# selected <job> succeeds when the run's mode selects the job.
selected() {
  local entry
  for entry in $scheduled_jobs; do
    if [[ ${entry%%:*} == "$1" ]]; then
      [[ ${entry#*:} == "$mode" ]]
      return
    fi
  done
  if contains "$pull_request_jobs" "$1"; then
    [[ $mode == pull_request ]]
    return
  fi
  if contains "$e2e_jobs" "$1"; then
    contains "$e2e_modes" "$mode"
    return
  fi
  contains "$gate_modes" "$mode"
}

problems=''
notices=''
warnings=''
while IFS=$'\t' read -r job result; do
  [[ -n $job ]] || continue
  case $result in
  success) ;;
  skipped)
    if selected "$job"; then
      problems+=", $job skipped, although mode $mode selects it"
    else
      notices+="$job was not selected in mode $mode"$'\n'
    fi
    ;;
  failure | cancelled)
    if contains "$advisory_jobs" "$job"; then
      warnings+="$job $result; it is advisory, so CI does not fail"$'\n'
    else
      problems+=", $job $result"
    fi
    ;;
  *) fail "job $job has the result \"$result\", which CI does not know" ;;
  esac
done <<<"$results"

# The results are printed with workflow commands stopped; the lines after them are commands.
token=$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')
echo "::stop-commands::$token"
printf '%s\n' "${results//$'\t'/ }"
echo "::$token::"
echo "ci-aggregate: mode $mode: $count job results"
while IFS= read -r line; do
  [[ -z $line ]] || echo "::notice title=CI::$line"
done <<<"$notices"
while IFS= read -r line; do
  [[ -z $line ]] || echo "::warning title=CI::$line"
done <<<"$warnings"
if [[ -n $problems ]]; then
  echo "::error title=CI::Jobs that did not succeed in mode $mode: ${problems#, }"
  exit 1
fi
echo "ci-aggregate: every job that mode $mode selects succeeded"
