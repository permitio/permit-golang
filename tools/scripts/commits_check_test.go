package scripts

import (
	"path/filepath"
	"strings"
	"testing"
)

// commitEnv returns the environment of a git command that commits in root, with a fixed author.
func commitEnv(root string) []string {
	return append(gitEnv(root),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
}

// commit commits an empty change with message in root and returns its SHA.
func commit(t *testing.T, root, message string) string {
	t.Helper()
	env := commitEnv(root)
	result := runScript(t, root, env, "git", "commit", "--quiet", "--allow-empty",
		"--allow-empty-message", "--cleanup=verbatim", "-m", message)
	if result.exit != 0 {
		t.Fatalf("git commit exited %d: %s", result.exit, result.stderr)
	}
	return revParse(t, root, "HEAD")
}

func revParse(t *testing.T, root, rev string) string {
	t.Helper()
	result := runScript(t, root, gitEnv(root), "git", "rev-parse", "--verify", rev)
	if result.exit != 0 {
		t.Fatalf("git rev-parse %s exited %d: %s", rev, result.exit, result.stderr)
	}
	return strings.TrimSpace(result.stdout)
}

func TestCommitsCheck(t *testing.T) {
	t.Parallel()
	const zeros = "0000000000000000000000000000000000000000"
	// newRepository returns a repository that holds the scripts in scripts and a history of
	// base, good (two commits that pass), bad (one that fails), a commit whose message tries to
	// issue a workflow command, a commit with an empty message, and hash, a commit with a
	// message line that starts with #, which git commit -m keeps.
	type history struct {
		root, base, good1, good2, bad, command, empty, hash string
	}
	newRepository := func(t *testing.T, scripts ...string) history {
		t.Helper()
		root := t.TempDir()
		linkScripts(t, root, scripts...)
		gitInit(t, root)
		var h history
		h.root = root
		h.base = commit(t, root, "PER-1: Add the scripts\n")
		h.good1 = commit(t, root, "PER-2: Return an error for an empty user key\n\n"+
			"The body says why.\n\nCo-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>\n")
		h.good2 = commit(t, root, "Test the empty key\n")
		h.bad = commit(t, root, "PER-3: Fix the key\n\nFixes #73\n")
		h.command = commit(t, root, "::warning::Log the key\n\n::error::from a commit message\n")
		h.empty = commit(t, root, "")
		h.hash = commit(t, root, "PER-4: Check the key\n\n#73 is fixed by this\n")
		return h
	}
	both := []string{"commits-check.sh", "check-commit-msg.sh"}
	// The subtests share h, which commits-check.sh only reads, so they run in parallel.
	h := newRepository(t, both...)
	tests := []struct {
		name string
		// args returns the script's arguments for the history h.
		args       func(h history) []string
		scripts    []string
		wantExit   int
		wantStdout []string
		wantStderr []string
	}{
		{
			name:     "commits whose messages pass",
			args:     func(h history) []string { return []string{h.base, h.good2} },
			wantExit: 0,
			wantStdout: []string{
				"commits-check: " + h.good1[:12] +
					" PER-2: Return an error for an empty user key\n" +
					"commits-check: " + h.good2[:12] + " Test the empty key\n",
			},
			wantStderr: []string{"commits-check: checked 2 commits; every message passed\n"},
		},
		{
			name:     "a commit with Fixes #73",
			args:     func(h history) []string { return []string{h.good2, h.bad} },
			wantExit: 1,
			wantStdout: []string{
				"check-commit-msg: line 3: \"Fixes #73\": remove the closing keyword",
				"::error title=Commit messages::1 of 1 commit messages have text that must not " +
					"reach the public history; see check-commit-msg above\n",
			},
		},
		{
			name: "a bad commit in a longer range",
			args: func(h history) []string { return []string{h.base, h.command} }, wantExit: 1,
			wantStdout: []string{"::error title=Commit messages::1 of 4 commit messages"},
		},
		{
			name:     "a commit with a message line that starts with #",
			args:     func(h history) []string { return []string{h.empty, h.hash} },
			wantExit: 1,
			wantStdout: []string{
				"check-commit-msg: line 3: \"#73\": an issue reference",
				"::error title=Commit messages::1 of 1 commit messages",
			},
		},
		{
			name:     "a base of 40 zeros, which is not a commit",
			args:     func(h history) []string { return []string{zeros, h.good2} },
			wantExit: 2,
			wantStderr: []string{
				"commits-check: commit " + zeros + " is not in the repository; check it out " +
					"with fetch-depth: 0\n",
			},
		},
		{
			name: "a range with no commits",
			args: func(h history) []string { return []string{h.good2, h.good2} }, wantExit: 2,
			wantStderr: []string{"commits-check: " + h.good2 + ".." + h.good2 +
				" holds no commit, so no message was checked"},
		},
		{
			name:     "a commit that is not in the repository",
			args:     func(h history) []string { return []string{h.base, strings.Repeat("a", 40)} },
			wantExit: 2,
			wantStderr: []string{
				"commits-check: commit " + strings.Repeat("a", 40) + " is not in the repository",
			},
		},
		{
			name:     "a base that is not a commit SHA",
			args:     func(h history) []string { return []string{"master", h.good2} },
			wantExit: 2,
			wantStderr: []string{
				"commits-check: want two 40-character commit SHAs, got \"master\" and",
			},
		},
		{
			name:       "one argument",
			args:       func(h history) []string { return []string{h.base} },
			wantExit:   2,
			wantStderr: []string{"commits-check: usage: commits-check.sh <base> <head>"},
		},
		{
			name:     "no check-commit-msg.sh",
			args:     func(h history) []string { return []string{h.base, h.good2} },
			scripts:  []string{"commits-check.sh"},
			wantExit: 2,
			wantStderr: []string{
				"commits-check: .github/scripts/check-commit-msg.sh is missing or not executable",
			},
		},
		{
			name: "a commit with an empty message",
			args: func(h history) []string { return []string{h.command, h.empty} }, wantExit: 2,
			wantStderr: []string{
				"commits-check: check-commit-msg.sh exited 2 on " + h.empty[:12] +
					", so its message was not checked",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run := h
			if tt.scripts != nil {
				run = newRepository(t, tt.scripts...)
			}

			result := runScript(t, run.root, gitEnv(run.root),
				filepath.Join(run.root, ".github", "scripts", "commits-check.sh"),
				tt.args(run)...)

			if result.exit != tt.wantExit {
				t.Errorf("exit = %d, want %d; stdout:\n%s\nstderr:\n%s", result.exit,
					tt.wantExit, result.stdout, result.stderr)
			}
			for _, want := range tt.wantStdout {
				if !strings.Contains(result.stdout, want) {
					t.Errorf("stdout does not contain %q:\n%s", want, result.stdout)
				}
			}
			for _, want := range tt.wantStderr {
				if !strings.Contains(result.stdout+result.stderr, want) {
					t.Errorf("output does not contain %q:\nstdout:\n%s\nstderr:\n%s", want,
						result.stdout, result.stderr)
				}
			}
		})
	}

	t.Run("a commit message cannot issue workflow commands", func(t *testing.T) {
		t.Parallel()
		env := gitEnv(h.root)
		result := runScript(t, h.root, env,
			filepath.Join(h.root, ".github", "scripts", "commits-check.sh"), h.bad, h.command)
		if result.exit != 0 {
			t.Fatalf("exit = %d, want 0; stdout:\n%s\nstderr:\n%s", result.exit, result.stdout,
				result.stderr)
		}
		assertWrapped(t, result.stdout, "::warning::Log the key")
	})
}
