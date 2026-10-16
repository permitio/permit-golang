package scripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckCommitMsg(t *testing.T) {
	t.Parallel()
	script, err := filepath.Abs(repositoryFile(".github/scripts/check-commit-msg.sh"))
	if err != nil {
		t.Fatal(err)
	}
	const subject = "PER-1: Return an error for an empty user key\n\n"
	const trailer = "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>\n"
	const links = "move issue links to the pull request description"
	const usage = "usage: check-commit-msg.sh [--committed] <commit message file>"
	type testCase struct {
		name    string
		message string
		// committed runs the script with --committed, the mode CI uses for each commit's message.
		committed bool
		// args replaces the message file as the script's arguments when it is not nil.
		args     []string
		wantExit int
		// wantStderr lists text stderr must contain.
		wantStderr []string
	}
	tests := []testCase{
		{
			name: "the trailers this repository uses",
			message: subject + "The body says what changed and why.\n\n" + trailer +
				"Co-authored-by: X <id+user@users.noreply.github.com>\n",
			wantExit: 0,
		},
		{
			name: "the trailers this repository uses, in a committed message",
			message: subject + "The body says what changed and why.\n\n" + trailer +
				"Co-authored-by: X <id+user@users.noreply.github.com>\n",
			committed: true, wantExit: 0,
		},
		{
			name: "every trailer that may hold an e-mail address, in any case",
			message: subject + "A body.\n\nCo-authored-by: A <a@example.com>\n" +
				"SIGNED-OFF-BY: B <b@example.com>\nreviewed-by: C <c@example.com>\n" +
				"Tested-By: D <d@example.com>\nAcked-by: E <e@example.com>\n" +
				"Reported-by: F <f@example.com>\nSuggested-by: G <g@example.com>\n" +
				"Helped-by: H <h@example.com>\n",
			wantExit: 0,
		},
		{
			name:     "two e-mail addresses on a trailer line",
			message:  subject + "A body.\n\nCo-authored-by: A <a@example.com> <b@example.com>\n",
			wantExit: 0,
		},
		{
			name: "the comments that git commit adds, with an Author line",
			message: subject + "A body.\n\n" + trailer +
				"# Please enter the commit message for your changes. Lines starting\n" +
				"# with '#' will be ignored, and an empty message aborts the commit.\n#\n" +
				"# Author:    Name <a@example.com>\n#\n# On branch fix-73\n" +
				"#\tmodified:   /Users/someone/x.go\n",
			wantExit: 0,
		},
		{
			name: "the comments that a squash in git rebase adds",
			message: "# This is a combination of 2 commits.\n" +
				"# This is the 1st commit message:\n\n" +
				"PER-1: Return an error for an empty user key\n\n" +
				"# This is the commit message #2:\n\nPER-1: Test the empty key\n",
			wantExit: 0,
		},
		{
			name: "the diff that git commit --verbose adds",
			message: subject + "A body.\n\n" + trailer +
				"# ------------------------ >8 ------------------------\n" +
				"# Do not modify or remove the line above.\n" +
				"+// Fixes #73, thanks @someone, see /Users/someone/x.go\n",
			wantExit: 0,
		},
		{
			name: "the diff that git commit --verbose adds, with CRLF line endings",
			message: "Return an error for an empty user key\r\n\r\n" +
				"# ------------------------ >8 ------------------------\r\n+// Fixes #73\r\n",
			wantExit: 0,
		},
		{
			name:     "a line that starts with #, which git removes",
			message:  subject + "# Fixes #73, see https://claude.ai/code/session_1\n",
			wantExit: 0,
		},
		{
			name:     "CRLF line endings",
			message:  "Return an error for an empty user key\r\n\r\nA body.\r\n",
			wantExit: 0,
		},
		{
			name:      "a committed message of lines that start with #",
			message:   "# Return an error for an empty user key\n#\n",
			committed: true, wantExit: 0,
		},
		{
			name: "words that only look like the rules",
			message: subject + "Fix the prefix-30 case and close the PERIOD-1 gap in UTF-8 for " +
				"high-5 users.\n",
			wantExit: 0,
		},
		{
			name:     "a closing keyword at the end of a longer word",
			message:  subject + "Keep the suffix PER-12 in the name.\n",
			wantExit: 0,
		},
		{
			name: "module versions",
			message: subject + "Pin golang.org/x/vuln@v1.8.0, golang.org/x/tools@v0.38.0 and " +
				"actions/checkout@v5, run go install x@v1.2.3, and read the list from " +
				"proxy.golang.org/golang.org/x/vuln/@v/list.\n",
			wantExit: 0,
		},
		{
			name:     "an @handle",
			message:  subject + "Reported by @someone.\n",
			wantExit: 1, wantStderr: []string{`line 3: "@someone": remove @handles`},
		},
		{
			name:     "an @handle at the start of a line",
			message:  subject + "@someone reported it.\n",
			wantExit: 1, wantStderr: []string{`"@someone": remove @handles`},
		},
		{
			name:     "an @handle in angle brackets on a trailer line",
			message:  subject + "Thanks: <@octocat>\n",
			wantExit: 1, wantStderr: []string{`line 3: "@octocat": remove @handles`},
		},
		{
			name:     "an @handle in angle brackets on a Co-authored-by line",
			message:  subject + "A body.\n\nCo-authored-by: Octo <@octocat>\n",
			wantExit: 1, wantStderr: []string{`line 5: "@octocat": remove @handles`},
		},
		{
			name:     "an @handle next to an e-mail address on a trailer line",
			message:  subject + "Reviewed-by: @someone <someone@example.com>\n",
			wantExit: 1, wantStderr: []string{`"@someone": remove @handles`},
		},
		{
			name:     "an @ at the end of a word",
			message:  subject + "Ping me@\n",
			wantExit: 1, wantStderr: []string{`line 3: "me@": remove @handles`},
		},
		{
			name:     "an e-mail address outside a trailer",
			message:  subject + "Reported by someone@example.com.\n",
			wantExit: 1, wantStderr: []string{`"someone@example.com": remove @handles`},
		},
		{
			name:     "an e-mail address in angle brackets outside a trailer",
			message:  subject + "Reported by Someone <someone@example.com>.\n",
			wantExit: 1, wantStderr: []string{`"someone@example.com": remove @handles`},
		},
		{
			name:     "an e-mail address after a colon in the middle of a line",
			message:  subject + "See the note: Jane <jane@example.com>\n",
			wantExit: 1, wantStderr: []string{`"jane@example.com": remove @handles`},
		},
		{
			name:     "an e-mail address on a trailer line that is not for people",
			message:  subject + "See: Jane <jane.doe@example.com>\n",
			wantExit: 1, wantStderr: []string{`line 3: "jane.doe@example.com": remove @handles`},
		},
		{
			name:     "an e-mail address on a trailer line whose token ends like one for people",
			message:  subject + "A body.\n\nX-Co-authored-by: A <a@example.com>\n",
			wantExit: 1, wantStderr: []string{`line 5: "a@example.com": remove @handles`},
		},
		{
			name: "an e-mail address on a trailer line after one that is for people",
			message: subject + "A body.\n\nCo-authored-by: A <a@example.com>\n" +
				"See: B <b@example.com>\n",
			wantExit: 1, wantStderr: []string{`line 6: "b@example.com": remove @handles`},
		},
		{
			name: "a committed trailer paragraph with a line that starts with #",
			message: subject + "A body.\n\nCo-authored-by: A <a@example.com>\n" +
				"# A note.\n",
			committed: true,
			wantExit:  1, wantStderr: []string{`line 5: "a@example.com": remove @handles`},
		},
		{
			name:     "an e-mail address on a Co-authored-by line before the trailer block",
			message:  subject + "Co-authored-by: Jane <jane@example.com>\n\n" + trailer,
			wantExit: 1, wantStderr: []string{`line 3: "jane@example.com": remove @handles`},
		},
		{
			name: "an e-mail address on a Co-authored-by line next to a line with a later colon",
			message: subject + "A body.\n\nCo-authored-by: A <a@example.com>\n" +
				"Thanks to the note: it helped.\n",
			wantExit: 1, wantStderr: []string{`line 5: "a@example.com": remove @handles`},
		},
		{
			name:     "an e-mail address on a Co-authored-by line in a paragraph with other text",
			message:  subject + "A body.\nCo-authored-by: Jane <jane@example.com>\n",
			wantExit: 1, wantStderr: []string{`line 4: "jane@example.com": remove @handles`},
		},
		{
			name:     "an e-mail address on a Co-authored-by line in the subject",
			message:  "Co-authored-by: Jane <jane@example.com>\n",
			wantExit: 1, wantStderr: []string{`line 1: "jane@example.com": remove @handles`},
		},
		{
			name:      "an e-mail address on a committed line that starts with #",
			message:   subject + "A body.\n\n" + trailer + "# Author: Jane <jane@example.com>\n",
			committed: true,
			wantExit:  1, wantStderr: []string{`line 6: "jane@example.com": remove @handles`},
		},
		{
			name:     "a closing keyword",
			message:  subject + "Fixes #73\n",
			wantExit: 1, wantStderr: []string{
				`line 3: "Fixes #73": remove the closing keyword and ` + links,
				`line 3: "#73": an issue reference; ` + links,
				"2 problems in the commit message",
			},
		},
		{
			name:     "a closing keyword on a last line with no newline",
			message:  subject + "Fixes #73",
			wantExit: 1, wantStderr: []string{`line 3: "Fixes #73": remove the closing keyword`},
		},
		{
			name:      "a closing keyword on a committed line that starts with #",
			message:   subject + "# Fixes #73\n",
			committed: true,
			wantExit:  1, wantStderr: []string{`line 3: "Fixes #73": remove the closing keyword`},
		},
		{
			name: "a closing keyword after a scissors line in a committed message",
			message: subject + "# ------------------------ >8 ------------------------\n" +
				"Fixes #73\n",
			committed: true,
			wantExit:  1, wantStderr: []string{`line 4: "Fixes #73": remove the closing keyword`},
		},
		{
			name:     "an issue reference",
			message:  subject + "Follows up on #12.\n",
			wantExit: 1, wantStderr: []string{`"#12": an issue reference; ` + links},
		},
		{
			name:     "an issue reference with one digit",
			message:  subject + "See #7.\n",
			wantExit: 1, wantStderr: []string{`"#7": an issue reference; ` + links},
		},
		{
			name:      "an issue reference at the start of a committed line",
			message:   subject + "#12 reported it.\n",
			committed: true,
			wantExit:  1, wantStderr: []string{`"#12": an issue reference`},
		},
		{
			name:     "a GH- issue reference",
			message:  subject + "Follows up on GH-73.\n",
			wantExit: 1, wantStderr: []string{`"GH-73": an issue reference; ` + links},
		},
		{
			name:     "a closing keyword with a GH- issue reference",
			message:  subject + "Fixes GH-73\n",
			wantExit: 1, wantStderr: []string{
				`"Fixes GH-73": remove the closing keyword`, `"GH-73": an issue reference`,
			},
		},
		{
			name:     "a closing keyword with a ticket ID",
			message:  subject + "This closes PER-12.\n",
			wantExit: 1, wantStderr: []string{`"closes PER-12": remove the closing keyword`},
		},
		{
			name:     "the closing keyword complete with an issue reference",
			message:  subject + "Completes #7.\n",
			wantExit: 1, wantStderr: []string{
				`"Completes #7": remove the closing keyword`, `"#7": an issue reference`,
			},
		},
		{
			name:     "a closing keyword in capitals with a colon",
			message:  subject + "RESOLVED: per-12\n",
			wantExit: 1, wantStderr: []string{`"RESOLVED: per-12": remove the closing keyword`},
		},
		{
			name:     "a closing keyword with an issue in another repository",
			message:  subject + "Fixed example/repo#3\n",
			wantExit: 1, wantStderr: []string{`"Fixed example/repo#3": remove the closing keyword`},
		},
		{
			name:     "a closing keyword with an issue link",
			message:  subject + "Resolves https://github.com/example/repo/issues/3\n",
			wantExit: 1, wantStderr: []string{
				`"Resolves https://github.com/example/repo/issues/3": remove the closing keyword`,
			},
		},
		{
			name:     "a claude.ai link",
			message:  subject + "Planned in https://claude.ai/code/session_1.\n",
			wantExit: 1, wantStderr: []string{`"claude.ai/": remove the claude.ai link`},
		},
		{
			name:     "a Claude-Session trailer",
			message:  subject + "Claude-Session: https://example.com/session\n",
			wantExit: 1, wantStderr: []string{
				`"Claude-Session": remove it; session links stay out of the public history`,
			},
		},
		{
			name:     "a Claude_Session line",
			message:  subject + "Claude_Session: 1\n",
			wantExit: 1, wantStderr: []string{`"Claude_Session": remove it`},
		},
		{
			name:     "the words claude and session with more than one character between them",
			message:  subject + "Keep the Claude - session id.\n",
			wantExit: 1, wantStderr: []string{`"Claude - session": remove it`},
		},
		{
			name:      "a committed Claude-Session line that starts with #",
			message:   subject + "A body.\n\n# Claude-Session: https://claude.ai/code/session_1\n",
			committed: true,
			wantExit:  1, wantStderr: []string{
				`line 5: "Claude-Session": remove it; session links stay out of the public history`,
				`line 5: "claude.ai/": remove the claude.ai link`,
			},
		},
		{
			name:     "a temporary directory on a developer's machine",
			message:  subject + "The log is in /private/tmp/example/run.log.\n",
			wantExit: 1, wantStderr: []string{`"/private/tmp/": a path on your machine`},
		},
		{
			name:     "a temporary directory of a coding agent",
			message:  subject + "The log is in /tmp/claude-example/run.log.\n",
			wantExit: 1, wantStderr: []string{`"/tmp/claude-": a path on your machine`},
		},
		{
			name:     "a home directory on macOS",
			message:  subject + "Run /Users/someone/bin/tool.\n",
			wantExit: 1, wantStderr: []string{`"/Users/": a path on your machine`},
		},
		{
			name:     "a home directory on Linux",
			message:  subject + "Run /home/someone/bin/tool.\n",
			wantExit: 1, wantStderr: []string{`"/home/": a path on your machine`},
		},
		{
			name:     "only whitespace",
			message:  "\n  \n\t\n",
			wantExit: 2, wantStderr: []string{"has no message text to check"},
		},
		{
			name:     "only lines that start with #",
			message:  "# Please enter the commit message for your changes.\n#\n",
			wantExit: 2, wantStderr: []string{"has no message text to check"},
		},
		{
			name:     "an empty message",
			message:  "",
			wantExit: 2, wantStderr: []string{"has no message text to check"},
		},
		{
			name:      "an empty committed message",
			message:   "",
			committed: true,
			wantExit:  2, wantStderr: []string{"has no message text to check"},
		},
		{
			name:     "no file",
			args:     []string{},
			wantExit: 2, wantStderr: []string{usage},
		},
		{
			name:     "--committed and no file",
			args:     []string{"--committed"},
			wantExit: 2, wantStderr: []string{usage},
		},
		{
			name:     "another option",
			args:     []string{"--verbose", "COMMIT_EDITMSG"},
			wantExit: 2, wantStderr: []string{usage},
		},
		{
			name:     "two files",
			args:     []string{"a", "b"},
			wantExit: 2, wantStderr: []string{usage},
		},
		{
			name:     "a file that does not exist",
			args:     []string{"missing.txt"},
			wantExit: 2, wantStderr: []string{"cannot read the commit message file missing.txt"},
		},
	}
	// The forms of each closing keyword that the cases above do not cover.
	for _, keyword := range []string{
		"close", "closed", "closing", "fix", "fixing", "resolve", "resolving",
		"complete", "completes", "completed", "completing",
	} {
		tests = append(tests, testCase{
			name:     "the closing keyword " + keyword + " with a ticket ID",
			message:  subject + "This will " + keyword + " PER-12.\n",
			wantExit: 1, wantStderr: []string{
				`"` + keyword + ` PER-12": remove the closing keyword and ` + links,
			},
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			args := tt.args
			if args == nil {
				file := filepath.Join(dir, "COMMIT_EDITMSG")
				if err := os.WriteFile(file, []byte(tt.message), 0o644); err != nil {
					t.Fatal(err)
				}
				args = []string{file}
				if tt.committed {
					args = []string{"--committed", file}
				}
			}

			result := runScript(t, dir, nil, script, args...)

			if result.exit != tt.wantExit {
				t.Errorf("exit = %d, want %d; stderr:\n%s", result.exit, tt.wantExit, result.stderr)
			}
			for _, want := range tt.wantStderr {
				if !strings.Contains(result.stderr, want) {
					t.Errorf("stderr does not contain %q:\n%s", want, result.stderr)
				}
			}
			if tt.wantExit == 0 && result.stderr != "" {
				t.Errorf("stderr = %q, want nothing for a message that passes", result.stderr)
			}
			if result.stdout != "" {
				t.Errorf("stdout = %q, want nothing", result.stdout)
			}
		})
	}
}
