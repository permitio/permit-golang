package scripts

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/tools/internal/execfile"
)

// fakeGH is a gh that answers gh api from the files in $FAKE_GH: commits.json for the commits
// of pull request 7 of o/r, and checks-<sha>.json for the check runs of a commit named
// $FAKE_CHECK_NAME (none when the file is missing). It fails when $FAKE_GH/fail exists, and on
// a check runs call when $FAKE_GH/fail-checks exists. It logs each call to $FAKE_GH/calls.
const fakeGH = `#!/usr/bin/env bash
set -euo pipefail
echo "$*" >>"$FAKE_GH/calls"
if [[ -e $FAKE_GH/fail || (-e $FAKE_GH/fail-checks && $* == *check-runs*) ]]; then
	echo "gh: HTTP 401: Bad credentials" >&2
	exit 1
fi
case "$*" in
"api --paginate repos/o/r/pulls/7/commits?per_page=100")
	cat "$FAKE_GH/commits.json" ;;
"api repos/o/r/commits/"*"/check-runs?check_name=$FAKE_CHECK_NAME&filter=latest&per_page=100")
	sha=${2#repos/o/r/commits/}
	sha=${sha%%/*}
	if [[ -e $FAKE_GH/checks-$sha.json ]]; then
		cat "$FAKE_GH/checks-$sha.json"
	else
		echo '{"total_count": 0, "check_runs": []}'
	fi ;;
*)
	echo "fake gh: unexpected call: $*" >&2
	exit 1 ;;
esac
`

// fakeGit is a git that runs $REAL_GIT, except that with FAKE_GIT=is-ancestor-fails git
// merge-base --is-ancestor exits 128, and with FAKE_GIT=merge-base-junk git merge-base prints
// something that is not a commit SHA.
const fakeGit = `#!/usr/bin/env bash
case "${FAKE_GIT:-} $*" in
"is-ancestor-fails "*" merge-base --is-ancestor "*) exit 128 ;;
"merge-base-junk "*" merge-base "*) echo "::warning::not a commit"; exit 0 ;;
esac
exec "$REAL_GIT" "$@"
`

// checkRuns returns the check runs response of a commit with one check run named name that
// concluded conclusion, created by the GitHub App app.
func checkRuns(name, conclusion, app string) string {
	return fmt.Sprintf(`{"total_count": 1, "check_runs": [{"name": %q, "status": "completed", `+
		`"conclusion": %q, "app": {"slug": %q}}]}`, name, conclusion, app)
}

// commitsJSON returns the pull request commits response for shas, oldest first, split into
// pages of two as gh api --paginate prints them.
func commitsJSON(shas ...string) string {
	var pages []string
	for i := 0; i < len(shas); i += 2 {
		var page []string
		for _, sha := range shas[i:min(i+2, len(shas))] {
			page = append(page, fmt.Sprintf(`{"sha": %q, "commit": {"message": "x"}}`, sha))
		}
		pages = append(pages, "["+strings.Join(page, ",")+"]")
	}
	if len(pages) == 0 {
		return "[]"
	}
	return strings.Join(pages, "\n")
}

// rangeBin returns a directory for the script's PATH: the fake gh, git as fakeGit when fake is
// set, and the other tools the script needs, linked from the test's PATH, but not without.
func rangeBin(t *testing.T, fake bool, without string) string {
	t.Helper()
	bin := t.TempDir()
	scripts := map[string]string{"gh": fakeGH}
	if fake {
		scripts["git"] = fakeGit
	}
	for name, script := range scripts {
		if err := execfile.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, tool := range []string{"bash", "cat", "dirname", "git", "jq", "od", "tr"} {
		if _, ok := scripts[tool]; ok || tool == without {
			continue
		}
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, filepath.Join(bin, tool)); err != nil {
			t.Fatal(err)
		}
	}
	return bin
}

func TestMutationRange(t *testing.T) {
	// The history: base, then master moves on to master2; the pull request adds c1, c2 and c3
	// (head) on base; x1 is on another branch from base, as a force push leaves a commit.
	root := t.TempDir()
	copyScripts(t, root, "mutation-range.sh")
	gitInit(t, root)
	base := commit(t, root, "base\n")
	git := func(args ...string) {
		t.Helper()
		if result := runScript(t, root, commitEnv(root), "git", args...); result.exit != 0 {
			t.Fatalf("git %s: %s", strings.Join(args, " "), result.stderr)
		}
	}
	git("branch", "pr")
	master2 := commit(t, root, "master moves on\n")
	git("checkout", "--quiet", "-b", "old", base)
	x1 := commit(t, root, "a commit a force push dropped\n")
	git("checkout", "--quiet", "pr")
	c1 := commit(t, root, "c1\n")
	c2 := commit(t, root, "c2\n")
	c3 := commit(t, root, "c3\n")
	missing := strings.Repeat("a", 40)
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, ".github", "scripts", "mutation-range.sh")

	// The script runs with only bin on PATH: the fake gh and the tools it needs.
	bin := rangeBin(t, false, "")
	success := checkRuns("mutation", "success", "github-actions")
	tests := []struct {
		name string
		// args are the script's arguments; nil means o/r 7 master2 c3 and the check name.
		args []string
		// check is the check name, mutation when empty.
		check      string
		files      map[string]string
		path       string
		env        []string
		wantExit   int
		wantStdout string
		wantStderr string
	}{
		{
			name: "the newest commit whose mutation check succeeded",
			files: map[string]string{
				"commits.json": commitsJSON(c1, c2, c3), "checks-" + c1 + ".json": success,
				"checks-" + c2 + ".json": success,
			},
			wantStdout: c2 + "\n",
			wantStderr: "mutation-range: starting at " + c2[:12] + ", the newest commit of pull " +
				"request 7 whose mutation check succeeded\n",
		},
		{
			name: "no successful mutation check: the merge base",
			files: map[string]string{
				"commits.json":           commitsJSON(c1, c2, c3),
				"checks-" + c1 + ".json": checkRuns("mutation", "failure", "github-actions"),
				"checks-" + c2 + ".json": checkRuns("mutation", "cancelled", "github-actions"),
			},
			wantStdout: base + "\n",
			wantStderr: "mutation-range: starting at the merge base " + base[:12] + ": no commit " +
				"of pull request 7 in the history of head has a successful mutation check\n",
		},
		{
			name: "a successful commit that head no longer contains is skipped",
			files: map[string]string{
				"commits.json": commitsJSON(c1, x1), "checks-" + x1 + ".json": success,
				"checks-" + c1 + ".json": success,
			},
			wantStdout: c1 + "\n",
			wantStderr: "mutation-range: skipping " + x1[:12] + ": its mutation check " +
				"succeeded, but head does not contain it\n",
		},
		{
			name: "a successful commit the repository does not have is skipped",
			files: map[string]string{
				"commits.json":                commitsJSON(c1, missing),
				"checks-" + missing + ".json": success,
			},
			wantStdout: base + "\n",
			wantStderr: "mutation-range: skipping " + missing[:12] + ": its mutation check " +
				"succeeded, but the repository does not have it\n",
		},
		{
			name: "a check run of another app does not count",
			files: map[string]string{
				"commits.json":           commitsJSON(c1, c2),
				"checks-" + c2 + ".json": checkRuns("mutation", "success", "some-other-app"),
			},
			wantStdout: base + "\n",
		},
		{
			name:  "the check name is an argument",
			check: "mutants",
			files: map[string]string{
				"commits.json":           commitsJSON(c1, c2),
				"checks-" + c1 + ".json": checkRuns("mutants", "success", "github-actions"),
				"checks-" + c2 + ".json": success,
			},
			wantStdout: c1 + "\n",
			wantStderr: "mutation-range: starting at " + c1[:12] + ", the newest commit of pull " +
				"request 7 whose mutants check succeeded\n",
		},
		{
			name: "head's own check succeeded: an empty range",
			files: map[string]string{
				"commits.json": commitsJSON(c1, c2, c3), "checks-" + c3 + ".json": success,
			},
			wantStdout: c3 + "\n",
		},
		{
			name:       "a pull request with no commits listed",
			files:      map[string]string{"commits.json": "[]"},
			wantStdout: base + "\n",
		},
		{
			name:       "gh fails",
			files:      map[string]string{"fail": ""},
			wantExit:   2,
			wantStderr: "mutation-range: listing the commits of pull request 7 with gh api failed",
		},
		{
			name: "gh fails on the check runs",
			files: map[string]string{
				"commits.json": commitsJSON(c1, c2, c3), "checks-" + c1 + ".json": success,
				"fail-checks": "",
			},
			wantExit:   2,
			wantStderr: "mutation-range: reading the check runs of " + c3 + " with gh api failed",
		},
		{
			name:       "gh lists commits that are not JSON",
			files:      map[string]string{"commits.json": "<html>rate limited</html>"},
			wantExit:   2,
			wantStderr: "mutation-range: gh api did not list the commits of pull request 7 as JSON",
		},
		{
			name:     "gh lists something that is not a commit SHA",
			files:    map[string]string{"commits.json": `[{"sha": "::error::injected"}]`},
			wantExit: 2,
			wantStderr: "mutation-range: gh api listed \"::error::injected\", which is not a " +
				"commit SHA",
		},
		{
			name: "check runs that are not JSON",
			files: map[string]string{
				"commits.json": commitsJSON(c1), "checks-" + c1 + ".json": `{"message": "x"}`,
			},
			wantExit:   2,
			wantStderr: "mutation-range: gh api did not list the check runs of " + c1 + " as JSON",
		},
		{
			name: "git merge-base --is-ancestor fails",
			files: map[string]string{
				"commits.json": commitsJSON(c1), "checks-" + c1 + ".json": success,
			},
			path:     rangeBin(t, true, ""),
			env:      []string{"FAKE_GIT=is-ancestor-fails"},
			wantExit: 2,
			wantStderr: "mutation-range: git merge-base --is-ancestor " + c1 + " " + c3 +
				" failed",
		},
		{
			name:     "git merge-base prints something that is not a commit SHA",
			files:    map[string]string{"commits.json": commitsJSON(c1)},
			path:     rangeBin(t, true, ""),
			env:      []string{"FAKE_GIT=merge-base-junk"},
			wantExit: 2,
			wantStderr: "mutation-range: the start commit \"::warning::not a commit\" is not a " +
				"40-character commit SHA",
		},
		{
			name:       "jq is not installed",
			files:      map[string]string{"commits.json": commitsJSON(c1)},
			path:       rangeBin(t, false, "jq"),
			wantExit:   2,
			wantStderr: "mutation-range: jq is not installed",
		},
		{
			name:       "a GITHUB_OUTPUT that cannot be written",
			files:      map[string]string{"commits.json": commitsJSON(c1)},
			env:        []string{"GITHUB_OUTPUT=" + root},
			wantExit:   2,
			wantStderr: "mutation-range: writing the start commit to GITHUB_OUTPUT (" + root,
		},
		{
			name: "four arguments", args: []string{"o/r", "7", base, c3}, wantExit: 2,
			wantStderr: "mutation-range: usage: mutation-range.sh <owner/repo> <pull request> " +
				"<base> <head> <check name>",
		},
		{
			name: "a repository that is not owner/repo", args: []string{"o", "7", base, c3, "x"},
			wantExit: 2, wantStderr: "mutation-range: \"o\" is not owner/repo",
		},
		{
			name: "a pull request that is not a number", args: []string{"o/r", "7a", base, c3, "x"},
			wantExit: 2, wantStderr: "mutation-range: \"7a\" is not a pull request number",
		},
		{
			name: "a base that is not a SHA", args: []string{"o/r", "7", "master", c3, "x"},
			wantExit: 2, wantStderr: "mutation-range: want two 40-character commit SHAs",
		},
		{
			name: "a check name that is not a job id", args: []string{"o/r", "7", base, c3, "a/b"},
			wantExit: 2, wantStderr: "mutation-range: check name \"a/b\" is not a job id",
		},
		{
			name: "a head the repository does not have",
			args: []string{"o/r", "7", base, missing, "x"}, wantExit: 2,
			wantStderr: "mutation-range: commit " + missing + " is not in the repository; " +
				"check it out with fetch-depth: 0",
		},
		{
			name: "a base the repository does not have",
			args: []string{"o/r", "7", missing, c3, "x"}, wantExit: 2,
			wantStderr: "mutation-range: commit " + missing + " is not in the repository; " +
				"check it out with fetch-depth: 0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ghDir := t.TempDir()
			writeFiles(t, ghDir, tt.files)
			check := cmp.Or(tt.check, "mutation")
			args := tt.args
			if args == nil {
				args = []string{"o/r", "7", master2, c3, check}
			}
			output := filepath.Join(t.TempDir(), "output")
			env := append(gitEnv(root), "FAKE_GH="+ghDir, "FAKE_CHECK_NAME="+check,
				"REAL_GIT="+realGit, "PATH="+cmp.Or(tt.path, bin), "GITHUB_OUTPUT="+output)
			result := runScript(t, root, append(env, tt.env...), script, args...)
			if result.exit != tt.wantExit || result.stdout != tt.wantStdout ||
				!strings.Contains(result.stderr, tt.wantStderr) {
				t.Errorf("exit %d, stdout %q, stderr:\n%s\nwant exit %d, stdout %q, stderr with %q",
					result.exit, result.stdout, result.stderr, tt.wantExit, tt.wantStdout,
					tt.wantStderr)
			}
			// What the script reports, and what gh and git print, is printed with workflow
			// commands stopped.
			assertWrapped(t, result.stderr, tt.wantStderr)
			// The step's outputs get the commit the script prints, and nothing when it fails.
			written, err := os.ReadFile(output)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			wantOutput := ""
			if tt.wantExit == 0 {
				wantOutput = "start=" + tt.wantStdout
			}
			if string(written) != wantOutput {
				t.Errorf("GITHUB_OUTPUT holds %q, want %q", written, wantOutput)
			}
		})
	}

	// The commits are read newest first, and the search stops at the first that qualifies.
	// Without GITHUB_OUTPUT, as when run locally, the commit is only printed.
	t.Run("newest first", func(t *testing.T) {
		ghDir := t.TempDir()
		writeFiles(t, ghDir, map[string]string{
			"commits.json": commitsJSON(c1, c2, c3), "checks-" + c2 + ".json": success,
		})
		env := append(gitEnv(root), "FAKE_GH="+ghDir, "FAKE_CHECK_NAME=mutation", "PATH="+bin,
			"GITHUB_OUTPUT=")
		result := runScript(t, root, env, script, "o/r", "7", base, c3, "mutation")
		calls, err := os.ReadFile(filepath.Join(ghDir, "calls"))
		if err != nil {
			t.Fatal(err)
		}
		want := "api --paginate repos/o/r/pulls/7/commits?per_page=100\n" +
			"api repos/o/r/commits/" + c3 + "/check-runs?check_name=mutation&filter=latest&" +
			"per_page=100\n" +
			"api repos/o/r/commits/" + c2 + "/check-runs?check_name=mutation&filter=latest&" +
			"per_page=100\n"
		if result.exit != 0 || result.stdout != c2+"\n" || string(calls) != want {
			t.Errorf("exit %d, stdout %q, calls:\n%s\nwant:\n%s", result.exit, result.stdout,
				calls, want)
		}
	})

	// A range that starts at head holds no change, and mutationgate passes it with a notice
	// without starting gremlins.
	t.Run("an empty range passes through the gate", func(t *testing.T) {
		gate := filepath.Join(t.TempDir(), "mutationgate")
		build := exec.CommandContext(t.Context(), "go", "build", "-o", gate,
			"../cmd/mutationgate")
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("go build: %v\n%s", err, out)
		}
		repo := t.TempDir()
		copyScripts(t, repo, "mutation-range.sh")
		writeFiles(t, repo, map[string]string{
			"go.mod": "module example.com/m\n\ngo 1.26.0\n",
			"m.go":   "package m\n\n// F returns 1.\nfunc F() int { return 1 }\n",
		})
		gitInit(t, repo)
		repoBase := commit(t, repo, "base\n")
		repoHead := commit(t, repo, "head\n")
		ghDir := t.TempDir()
		writeFiles(t, ghDir, map[string]string{
			"commits.json": commitsJSON(repoHead), "checks-" + repoHead + ".json": success,
		})
		env := append(gitEnv(repo), "FAKE_GH="+ghDir, "FAKE_CHECK_NAME=mutation", "PATH="+bin,
			"GITHUB_OUTPUT=")
		result := runScript(t, repo, env,
			filepath.Join(repo, ".github", "scripts", "mutation-range.sh"), "o/r", "7", repoBase,
			repoHead, "mutation")
		if result.exit != 0 || result.stdout != repoHead+"\n" {
			t.Fatalf("mutation-range.sh: exit %d, stdout %q, stderr %s", result.exit,
				result.stdout, result.stderr)
		}
		// The gate runs go and git from the test's own PATH.
		gateEnv := append(gitEnv(repo), "GOWORK=off", "GOFLAGS=")
		result = runScript(t, repo, gateEnv, gate, "-gremlins",
			filepath.Join(repo, "no-gremlins"), "-base", strings.TrimSpace(result.stdout), ".")
		want := "**PASS**: No non-test Go line was added or changed since the merge base with " +
			repoHead + "; nothing to mutate."
		if result.exit != 0 || !strings.Contains(result.stdout, want) ||
			!strings.HasSuffix(result.stdout, "mutationgate: PASS\n") {
			t.Errorf("mutationgate: exit %d, stdout:\n%s\nstderr:\n%s", result.exit,
				result.stdout, result.stderr)
		}
	})
}
