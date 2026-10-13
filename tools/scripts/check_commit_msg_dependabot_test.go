package scripts

import (
	"path/filepath"
	"testing"
)

// TestCheckCommitMsgDependabot checks that the commit messages Dependabot writes for the entries
// of .github/dependabot.yml pass, so that the commit message check in CI passes its pull
// requests. Each file in testdata/dependabot holds one message in the format of Dependabot's
// message builder and of its commits on public repositories: the subject with the entry's
// prefix, shortened when it is longer than 72 characters; the Bumps line with links; the Release
// notes, Changelog and Commits links; the updated-dependencies block that GitHub adds; and the
// sign-off.
func TestCheckCommitMsgDependabot(t *testing.T) {
	script, err := filepath.Abs(repositoryFile(".github/scripts/check-commit-msg.sh"))
	if err != nil {
		t.Fatal(err)
	}
	messages := []string{
		"gomod-minor-and-patch.txt", // the minor-and-patch group of the root module
		"gomod-major.txt",           // a major update, which no group takes
		"gomod-security.txt",        // a security update
		"gomod-tools-tooling.txt",   // the tooling group of the tools module
		"actions-major.txt",         // an action pinned to a commit SHA
		"actions-tooling-table.txt", // five updates or more, which Dependabot lists in a table
		"pre-commit-one-hook.txt",   // a hook, named by its URL, and a shortened subject
		"pre-commit-two-hooks.txt",  // two hooks
	}
	// The commit-msg hook checks a message before git cleans it up, and CI checks it with
	// --committed as git log prints it.
	modes := []struct {
		name string
		args []string
	}{{name: "hook"}, {name: "committed", args: []string{"--committed"}}}
	for _, name := range messages {
		for _, mode := range modes {
			t.Run(name+", "+mode.name, func(t *testing.T) {
				file, err := filepath.Abs(filepath.Join("testdata", "dependabot", name))
				if err != nil {
					t.Fatal(err)
				}
				args := append(append([]string{}, mode.args...), file)

				result := runScript(t, t.TempDir(), nil, script, args...)

				if result.exit != 0 || result.stderr != "" || result.stdout != "" {
					t.Errorf("exit = %d, stdout = %q, stderr:\n%s\nwant exit 0 and no output",
						result.exit, result.stdout, result.stderr)
				}
			})
		}
	}
}
