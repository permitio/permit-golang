#!/usr/bin/env bash
# check-commit-msg.sh [--committed] <file> checks a commit message for text that must not reach
# the public history of this repository. It has two modes:
#   - by default, for the commit-msg hook, which passes the file git wrote the message to before
#     git cleans it up: it skips lines that start with # and stops at the scissors line that
#     git commit --verbose adds, as git's cleanup removes both;
#   - with --committed, for CI, which passes each commit's message from git log: it checks every
#     line and does not stop at a scissors line, because a message committed with git commit -m
#     or -F keeps its lines that start with #.
#
# It rejects, case-insensitively where the rule is about words:
#   - a Claude-Session trailer, or the two words with any separator or none, such as
#     Claude_Session;
#   - a claude.ai/ link;
#   - a path on a developer's machine: /Users/, /home/, /private/tmp/ or /tmp/claude-;
#   - an issue or pull request reference: # or GH- followed by digits;
#   - a closing keyword (close, closes, closed, closing, and the same forms of fix, resolve and
#     complete) followed by an issue reference, a GitHub issue link or a ticket ID such as
#     PER-123;
#   - an @handle, an e-mail address, or an @ that ends a word, such as "me@". A module version
#     such as golang.org/x/vuln@v1.8.0 passes.
# An e-mail address may appear only inside <...> on a Co-authored-by, Signed-off-by,
# Reviewed-by, Tested-by, Acked-by, Reported-by, Suggested-by or Helped-by line in the trailer
# block, such as "Co-authored-by: Name <id+user@users.noreply.github.com>". The trailer block is
# the last paragraph when it is not the first one and each of its lines starts with a token and
# a colon.
#
# Exit codes: 0 when the message passes; 1 when it contains rejected text, each piece named on
# stderr with how to fix it; 2 when the check did not run: the arguments are not an optional
# --committed and one file, the file cannot be read, or the message has no text to check.
set -euo pipefail

fail() {
  echo "check-commit-msg: $*" >&2
  exit 2
}

committed=false
if [[ $# -gt 0 && $1 == --committed ]]; then
  committed=true
  shift
fi
[[ $# -eq 1 ]] || fail "usage: check-commit-msg.sh [--committed] <commit message file>"
file=$1
[[ -f $file && -r $file ]] || fail "cannot read the commit message file $file"

scissors_re='^# -+ >8 -+$'
comment_re='^#'
trailer_re='^[A-Za-z][A-Za-z0-9-]*:[[:space:]]'
email_trailer_re='^(co-authored-by|signed-off-by|reviewed-by|tested-by|acked-by|reported-by'
email_trailer_re+='|suggested-by|helped-by):[[:space:]]'
links='move issue links to the pull request description'
session_re='claude[^[:alnum:]]*session'
claude_link_re='claude\.ai/'
local_path_re='/Users/|/home/|/private/tmp/|/tmp/claude-'
issue_re='#[0-9]+'
gh_issue_re='(^|[^[:alnum:]_])(GH-[0-9]+)'
closing_re='(^|[^[:alnum:]_])((close|closes|closed|closing|fix|fixes|fixed|fixing'
closing_re+='|resolve|resolves|resolved|resolving|complete|completes|completed|completing)'
closing_re+=':?[[:space:]]+(#[0-9]+|[[:alnum:]_.-]+/[[:alnum:]_.-]+#[0-9]+|PER-[0-9]+|GH-[0-9]+'
closing_re+='|https?://github\.com/[^[:space:]]+/issues/[0-9]+))'
email_group_re='<[^<>@[:space:]]+@[^<>@[:space:]]+>'
handle_re='(^|[^[:alnum:]_./+-])(@[[:alnum:]-]+)'
email_re='[[:alnum:]._%+-]+@[[:alnum:]-]+(\.[[:alnum:]-]+)*\.[[:alpha:]]{2,}'
bare_at_re='([^[:space:]]*@)([^[:alnum:]]|$)'
at_fix='remove @handles and e-mail addresses; an e-mail address may appear only inside <...> on'
at_fix+=' a trailer line at the end of the message, such as Co-authored-by: Name <email>'
path_fix='a path on your machine; use a path relative to the repository root'

lines=()
while IFS= read -r line || [[ -n $line ]]; do
  line=${line%$'\r'}
  [[ $committed == true || ! $line =~ $scissors_re ]] || break
  lines+=("$line")
done <"$file"
count=${#lines[@]}

# trailer_start is the index of the first line of the trailer block, or count when there is none.
paragraphs=0
trailer_start=$count
all_trailers=false
blank_before=true
for ((i = 0; i < count; i++)); do
  line=${lines[i]}
  [[ $committed == true || ! $line =~ $comment_re ]] || continue
  if [[ -z ${line//[[:space:]]/} ]]; then
    blank_before=true
    continue
  fi
  if [[ $blank_before == true ]]; then
    paragraphs=$((paragraphs + 1))
    trailer_start=$i
    all_trailers=true
    blank_before=false
  fi
  [[ $line =~ $trailer_re ]] || all_trailers=false
done
[[ $paragraphs -gt 1 && $all_trailers == true ]] || trailer_start=$count

problems=0
checked=0
line_no=0

# reject names the rejected text on the current line and says how to fix it.
reject() {
  echo "check-commit-msg: line $line_no: \"$1\": $2" >&2
  problems=$((problems + 1))
}

for ((i = 0; i < count; i++)); do
  line=${lines[i]}
  line_no=$((i + 1))
  [[ $committed == true || ! $line =~ $comment_re ]] || continue
  [[ -n ${line//[[:space:]]/} ]] || continue
  checked=$((checked + 1))

  email_trailer=false
  shopt -s nocasematch
  if [[ $line =~ $session_re ]]; then
    reject "${BASH_REMATCH[0]}" "remove it; session links stay out of the public history"
  fi
  if [[ $line =~ $claude_link_re ]]; then
    reject "${BASH_REMATCH[0]}" "remove the claude.ai link"
  fi
  if [[ $line =~ $closing_re ]]; then
    reject "${BASH_REMATCH[2]}" "remove the closing keyword and $links"
  fi
  if [[ $line =~ $gh_issue_re ]]; then
    reject "${BASH_REMATCH[2]}" "an issue reference; $links"
  fi
  if [[ $i -ge $trailer_start && $line =~ $email_trailer_re ]]; then
    email_trailer=true
  fi
  shopt -u nocasematch
  if [[ $line =~ $local_path_re ]]; then
    reject "${BASH_REMATCH[0]}" "$path_fix"
  fi
  if [[ $line =~ $issue_re ]]; then
    reject "${BASH_REMATCH[0]}" "an issue reference; $links"
  fi
  rest=$line
  if [[ $email_trailer == true ]]; then
    while [[ $rest =~ $email_group_re ]]; do
      rest=${rest/"${BASH_REMATCH[0]}"/ }
    done
  fi
  if [[ $rest =~ $handle_re ]]; then
    reject "${BASH_REMATCH[2]}" "$at_fix"
  fi
  if [[ $rest =~ $email_re ]]; then
    reject "${BASH_REMATCH[0]}" "$at_fix"
  fi
  if [[ $rest =~ $bare_at_re ]]; then
    reject "${BASH_REMATCH[1]}" "$at_fix"
  fi
done

[[ $checked -gt 0 ]] || fail "$file has no message text to check"
if [[ $problems -gt 0 ]]; then
  echo "check-commit-msg: $problems problems in the commit message; edit it and commit again" >&2
  exit 1
fi
