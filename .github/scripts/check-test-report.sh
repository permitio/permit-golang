#!/usr/bin/env bash
# Checks a `go test -json` report.
#
# Exit 0: at least one test passed, every package with test files has a
#         passing test, no test or subtest was skipped, and nothing failed.
# Exit 1: one of those does not hold, or a line of the report is not a test
#         event. A package with test files that runs no test (for example a
#         TestMain that never calls m.Run) counts as a failure, because its
#         tests would never catch anything.
# Exit 2: the report is missing, empty or has no test events.
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <go-test-json-report>" >&2
  exit 2
fi
report=$1

if [[ ! -s "$report" ]]; then
  echo "error: $report is missing or empty." >&2
  exit 2
fi
# Go releases before 1.24 print a failed build as a plain "FAIL <package>
# [build failed]" line, so lines that are not JSON objects are kept and reported.
lines='[split("\n")[] | select(length > 0) | (fromjson? | objects) // {NotJSON: .}]'
all=$(jq -Rsc "$lines" "$report")
events=$(jq -c 'map(select(has("NotJSON") | not))' <<<"$all")
other=$(jq -r '.[] | .NotJSON // empty' <<<"$all")
if [[ "$events" == "[]" ]]; then
  echo "error: $report has no go test -json events." >&2
  exit 2
fi

passed=$(jq '[.[] | select(.Test != null and .Action == "pass")] | length' <<<"$events")
skipped=$(jq -r '.[] | select(.Test != null and .Action == "skip")
  | "\(.Package) \(.Test)"' <<<"$events")
failed=$(jq -r '.[] | select(.Action == "fail" or .Action == "build-fail")
  | [.Package // .ImportPath, .Test // empty] | join(" ")' <<<"$events")
norun=$(jq -r 'map(select(.Package != null)) | group_by(.Package)[]
  | select(any(.[]; .Test == null and .Action == "pass"))
  | select(any(.[]; .Test != null and .Action == "pass") | not)
  | .[0].Package' <<<"$events")

echo "Tests passed (including subtests): $passed"
status=0
if [[ -n "$other" ]]; then
  echo "These lines of the report are not test events:" >&2
  echo "$other" >&2
  status=1
fi
if [[ -n "$failed" ]]; then
  echo "These tests or packages failed:" >&2
  echo "$failed" >&2
  status=1
fi
if [[ -n "$skipped" ]]; then
  echo "These tests were skipped. Remove the skip or fix the test:" >&2
  echo "$skipped" >&2
  status=1
fi
if [[ -n "$norun" ]]; then
  echo "These packages have test files but no test passed:" >&2
  echo "$norun" >&2
  status=1
fi
if ((passed == 0)); then
  echo "No test passed, so the test step did not test anything." >&2
  status=1
fi
exit "$status"
