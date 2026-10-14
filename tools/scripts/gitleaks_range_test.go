package scripts

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/permitio/permit-golang/v2/tools/internal/execfile"
)

// gitleaksStub is a stand-in for gitleaks. For version it prints STUB_VERSION. For a scan it
// appends its working directory, the number of files in it, its arguments, the size of the file
// that --gitleaks-ignore-path names and the GITLEAKS_ variables of its environment to the file
// STUB_LOG names, and copies the file --config names to STUB_CONFIG. Then it copies the file
// STUB_REPORT names, when it is set, to the --report-path, prints STUB_STDERR to stderr and exits
// with STUB_EXIT.
const gitleaksStub = `#!/bin/sh
if [ "$1" = version ]; then
	echo "$STUB_VERSION"
	exit 0
fi
{
	echo "pwd $PWD"
	echo "files $(ls -A | wc -l | tr -d ' ')"
	for arg in "$@"; do echo "arg $arg"; done
	env | grep '^GITLEAKS_' | sed 's/^/env /'
} >>"$STUB_LOG"
prev=
for arg in "$@"; do
	case $prev in
	--config) cp "$arg" "$STUB_CONFIG" ;;
	--gitleaks-ignore-path) echo "ignore-size $(wc -c <"$arg" | tr -d ' ')" >>"$STUB_LOG" ;;
	--report-path) report=$arg ;;
	esac
	prev=$arg
done
if [ -n "$STUB_REPORT" ]; then cp "$STUB_REPORT" "$report"; fi
printf '%s' "$STUB_STDERR" >&2
exit "$STUB_EXIT"
`

const gitleaksLock = "[gitleaks]\nversion = 1.2.3\n" +
	"url = https://example.com/gitleaks-{version}.tar.gz\n"

// gitleaksOutput matches the output of gitleaks-range.sh: it stops workflow commands first and
// resumes them with the same token. It captures the token, what is printed while the commands are
// stopped, the token that resumes them and what follows.
var gitleaksOutput = regexp.MustCompile(
	`(?s)^::stop-commands::([0-9a-f]{32})\n(.*)::([0-9a-f]{32})::\n(.*)$`)

// splitStopped checks that stdout stops workflow commands first and resumes them once with the
// same token, and returns what it printed while they were stopped and what it printed after.
func splitStopped(t *testing.T, stdout string) (stopped, after string) {
	t.Helper()
	m := gitleaksOutput.FindStringSubmatch(stdout)
	if m == nil || m[1] != m[3] || strings.Count(stdout, m[1]) != 2 {
		t.Fatalf("stdout does not stop workflow commands first and resume them once with the "+
			"same token:\n%s", stdout)
	}
	return m[2], m[4]
}

// runGit runs git in root with a fixed author and no user or system configuration, and fails the
// test when git fails.
func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	result := runScript(t, root, commitEnv(root), "git", args...)
	if result.exit != 0 {
		t.Fatalf("git %s exited %d: %s", strings.Join(args, " "), result.exit, result.stderr)
	}
}

// commitFiles writes files in root, commits every change with message, and returns the commit's
// SHA.
func commitFiles(t *testing.T, root string, files map[string]string, message string) string {
	t.Helper()
	writeFiles(t, root, files)
	runGit(t, root, "add", "--all")
	runGit(t, root, "commit", "--quiet", "--allow-empty", "-m", message)
	return revParse(t, root, "HEAD")
}

// linkGitleaks links bin/gitleaks under root to gitleaks.
func linkGitleaks(t *testing.T, root, gitleaks string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitleaks, filepath.Join(root, "bin", "gitleaks")); err != nil {
		t.Fatal(err)
	}
}

// newGitleaksRepository makes a git repository laid out like this one, with gitleaks-range.sh,
// lock as tools/binaries.lock, and bin/gitleaks linked to gitleaks unless gitleaks is empty. Its
// first commit holds every file but bin/; it returns the repository's root and that commit.
func newGitleaksRepository(t *testing.T, lock, gitleaks string) (root, first string) {
	t.Helper()
	root = t.TempDir()
	copyScripts(t, root, "gitleaks-range.sh")
	writeFiles(t, root, map[string]string{
		"tools/binaries.lock": lock, ".gitignore": "/bin/\n", "README.md": "# Test\n",
	})
	if gitleaks != "" {
		linkGitleaks(t, root, gitleaks)
	}
	runGit(t, root, "init", "--quiet")
	return root, commitFiles(t, root, nil, "First")
}

// tomlLines returns the lines of a TOML file that are not blank or comments.
func tomlLines(data []byte) []string {
	var lines []string
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return lines
}

// gitleaksHistory is a repository from newGitleaksRepository with two more commits, second and
// third, which add a file each.
type gitleaksHistory struct {
	root, first, second, third string
}

func TestGitleaksRange(t *testing.T) {
	const (
		zeros     = "0000000000000000000000000000000000000000"
		didNotRun = "gitleaks-range: DID NOT RUN\n"
		usage     = "gitleaks-range: usage: gitleaks-range.sh [--if-empty error|notice] " +
			"{<base> | --since-last-tag} <head>\n"
		lastTag = "gitleaks-range: v1.0.0 is the newest tag that matches v[0-9]* before HEAD\n"
		scanned = "gitleaks-range: FIRST..THIRD: gitleaks 1.2.3; commits scanned: 2, " +
			"findings: "
		noReport = "gitleaks-range: gitleaks wrote no report, or one that is not a JSON list " +
			"of findings\n"
		loggedError = "gitleaks-range: gitleaks logged an error, so it may not have scanned " +
			"every commit\n"
		twoFindings = `[{"File":"a.go","StartLine":3,"RuleID":"permit-api-key",` +
			`"Description":"Permit API key","Commit":"0123456789abcdef0123456789abcdef01234567"},` +
			`{"File":"b.txt","StartLine":1,"RuleID":"generic-api-key",` +
			`"Description":"Generic API Key"}]`
	)
	fromFirst := func(h gitleaksHistory) []string { return []string{h.first, h.third} }
	tests := []struct {
		name string
		// args returns the arguments for the history h; nil gives first and third.
		args func(h gitleaksHistory) []string
		// tags returns the tags to make in the history h, each name with its commit.
		tags func(h gitleaksHistory) map[string]string
		// code is the stub's exit code, 0 when empty; report is the report it writes, none when
		// noReport is set; stderr is what it prints on stderr.
		code, report, stderr string
		noReport             bool
		// version is what bin/gitleaks version prints, 1.2.3 when empty; noGitleaks leaves
		// bin/gitleaks out, and lock replaces tools/binaries.lock.
		version, lock string
		noGitleaks    bool
		// ran says that gitleaks ran, on first..third.
		ran      bool
		wantExit int
		// wantStopped is what the script prints while workflow commands are stopped, and
		// wantAfter what it prints after, once FIRST, SECOND and THIRD are replaced with the
		// first 12 characters of those commits.
		wantStopped, wantAfter string
	}{
		{
			name: "commits without a secret", report: "[]",
			stderr: "11:32PM INF 2 commits scanned.\n", ran: true, wantExit: 0,
			wantStopped: "11:32PM INF 2 commits scanned.\n" + scanned + "0\ngitleaks-range: PASS\n",
		},
		{
			name: "a warning that git skipped rename detection", report: "[]",
			stderr: "\x1b[90m11:32PM\x1b[0m \x1b[33mWRN\x1b[0m \x1b[1minexact rename detection " +
				"was skipped due to too many files.\x1b[0m\n",
			ran: true, wantExit: 0,
			wantStopped: "\x1b[90m11:32PM\x1b[0m \x1b[33mWRN\x1b[0m \x1b[1minexact rename " +
				"detection was skipped due to too many files.\x1b[0m\n" + scanned +
				"0\ngitleaks-range: PASS\n",
		},
		{
			name: "two findings", code: "10", report: twoFindings, ran: true, wantExit: 1,
			wantStopped: "a.go:3: permit-api-key (Permit API key), commit 0123456789ab\n" +
				"b.txt:1: generic-api-key (Generic API Key), commit \n" + scanned +
				"2\ngitleaks-range: FAIL\n",
			wantAfter: "::error title=Secret scan::gitleaks found secrets in the commits " +
				"FIRST..THIRD (findings: 2); see the job log. Revoke each one, remove it " +
				"from the branch's history, and use fake keys that start with " +
				"permit_key_FAKE in tests and docs.\n",
		},
		{
			name:   "revisions",
			args:   func(gitleaksHistory) []string { return []string{"HEAD~2", "HEAD"} },
			report: "[]", ran: true, wantExit: 0,
			wantStopped: scanned + "0\ngitleaks-range: PASS\n",
		},
		{
			name: "--if-empty notice on a range with commits",
			args: func(h gitleaksHistory) []string {
				return []string{"--if-empty", "notice", h.first, h.third}
			},
			report: "[]", ran: true, wantExit: 0,
			wantStopped: scanned + "0\ngitleaks-range: PASS\n",
		},
		{
			name: "a fatal error", code: "1", noReport: true,
			stderr: "11:32PM FTL unable to load gitleaks config\n", ran: true, wantExit: 2,
			wantStopped: "11:32PM FTL unable to load gitleaks config\n" +
				"gitleaks-range: gitleaks exited 1\n" + didNotRun,
		},
		{
			name: "gitleaks exits 126", code: "126", report: "[]", ran: true, wantExit: 2,
			wantStopped: "gitleaks-range: gitleaks exited 126\n" + didNotRun,
		},
		{
			name: "a git error in the log", report: "[]",
			stderr: "\x1b[90m11:32PM\x1b[0m \x1b[31mERR\x1b[0m \x1b[1m[git] fatal: bad object " +
				"HEAD\x1b[0m\n",
			ran: true, wantExit: 2,
			wantStopped: "\x1b[90m11:32PM\x1b[0m \x1b[31mERR\x1b[0m \x1b[1m[git] fatal: bad " +
				"object HEAD\x1b[0m\n" + loggedError + didNotRun,
		},
		{
			name: "a fatal line without colours and exit 0", report: "[]",
			stderr: "11:32PM FTL failed\n", ran: true, wantExit: 2,
			wantStopped: "11:32PM FTL failed\n" + loggedError + didNotRun,
		},
		{
			name: "no report", noReport: true, ran: true, wantExit: 2,
			wantStopped: noReport + didNotRun,
		},
		{
			name: "a report that is not JSON", report: "leaks found: 0\n", ran: true, wantExit: 2,
			wantStopped: noReport + didNotRun,
		},
		{
			name: "an empty report", report: "", ran: true, wantExit: 2,
			wantStopped: noReport + didNotRun,
		},
		{
			name: "a report that is an object", report: "{}", ran: true, wantExit: 2,
			wantStopped: noReport + didNotRun,
		},
		{
			name: "a report that lists numbers", report: "[1]", ran: true, wantExit: 2,
			wantStopped: noReport + didNotRun,
		},
		{
			name: "a report with two lists", report: "[] []", ran: true, wantExit: 2,
			wantStopped: noReport + didNotRun,
		},
		{
			name: "exit 0 with findings", report: twoFindings, ran: true, wantExit: 2,
			wantStopped: "gitleaks-range: gitleaks exited 0, but its report lists findings: 2\n" +
				didNotRun,
		},
		{
			name: "the finding exit code without findings", code: "10", report: "[]", ran: true,
			wantExit: 2,
			wantStopped: "gitleaks-range: gitleaks exited 10, its code for a finding, but its " +
				"report lists none\n" + didNotRun,
		},
		{
			name: "no bin/gitleaks", noGitleaks: true, wantExit: 2,
			wantStopped: "gitleaks-range: bin/gitleaks is missing; install it with " +
				".github/scripts/fetch-tool.sh gitleaks\n" + didNotRun,
		},
		{
			name: "another gitleaks version", version: "1.2.2", wantExit: 2,
			wantStopped: "gitleaks-range: bin/gitleaks reports version \"1.2.2\", want 1.2.3 " +
				"from tools/binaries.lock; install it with .github/scripts/fetch-tool.sh " +
				"gitleaks\n" + didNotRun,
		},
		{
			name: "a lock without gitleaks", lock: "[prek]\nversion = 0.5.5\n", wantExit: 2,
			wantStopped: "gitleaks-range: tools/binaries.lock has no version for gitleaks\n" +
				didNotRun,
		},
		{
			name:     "no arguments",
			args:     func(gitleaksHistory) []string { return []string{} },
			wantExit: 2, wantStopped: usage + didNotRun,
		},
		{
			name:     "one argument",
			args:     func(h gitleaksHistory) []string { return []string{h.first} },
			wantExit: 2, wantStopped: usage + didNotRun,
		},
		{
			name: "three arguments",
			args: func(h gitleaksHistory) []string {
				return []string{h.first, h.second, h.third}
			},
			wantExit: 2, wantStopped: usage + didNotRun,
		},
		{
			name:     "--if-empty without a value",
			args:     func(gitleaksHistory) []string { return []string{"--if-empty"} },
			wantExit: 2, wantStopped: usage + didNotRun,
		},
		{
			name: "--if-empty pass",
			args: func(h gitleaksHistory) []string {
				return []string{"--if-empty", "pass", h.first, h.third}
			},
			wantExit: 2,
			wantStopped: "gitleaks-range: --if-empty takes error or notice, not \"pass\"\n" +
				didNotRun,
		},
		{
			name: "a base that is not in the repository",
			args: func(h gitleaksHistory) []string {
				return []string{strings.Repeat("1", 40), h.third}
			},
			wantExit: 2,
			wantStopped: "gitleaks-range: the base \"" + strings.Repeat("1", 40) + "\" is not a " +
				"commit in the repository; clone the full history (fetch-depth: 0)\n" + didNotRun,
		},
		{
			name:     "an option as the base",
			args:     func(h gitleaksHistory) []string { return []string{"--all", h.third} },
			wantExit: 2,
			wantStopped: "gitleaks-range: the base \"--all\" is not a commit in the repository; " +
				"clone the full history (fetch-depth: 0)\n" + didNotRun,
		},
		{
			name:     "a tree as the head",
			args:     func(h gitleaksHistory) []string { return []string{h.first, "HEAD^{tree}"} },
			wantExit: 2,
			wantStopped: "gitleaks-range: the head \"HEAD^{tree}\" is not a commit in the " +
				"repository; clone the full history (fetch-depth: 0)\n" + didNotRun,
		},
		{
			name:     "the zero SHA as the base, as a push that creates a branch sends",
			args:     func(h gitleaksHistory) []string { return []string{zeros, h.third} },
			wantExit: 2,
			wantStopped: "gitleaks-range: \"" + zeros + "\" is the zero SHA, which a push sends " +
				"for a branch it creates or deletes, so there is no range of commits to scan\n" +
				didNotRun,
		},
		{
			name:     "the zero SHA as the head",
			args:     func(h gitleaksHistory) []string { return []string{h.first, zeros} },
			wantExit: 2,
			wantStopped: "gitleaks-range: \"" + zeros + "\" is the zero SHA, which a push sends " +
				"for a branch it creates or deletes, so there is no range of commits to scan\n" +
				didNotRun,
		},
		{
			name:     "a range without commits",
			args:     func(h gitleaksHistory) []string { return []string{h.third, h.third} },
			wantExit: 2,
			wantStopped: "gitleaks-range: THIRD..THIRD holds no commit, so there is nothing to " +
				"scan\n" + didNotRun,
		},
		{
			name: "a range without commits and --if-empty error",
			args: func(h gitleaksHistory) []string {
				return []string{"--if-empty", "error", h.third, h.third}
			},
			wantExit: 2,
			wantStopped: "gitleaks-range: THIRD..THIRD holds no commit, so there is nothing to " +
				"scan\n" + didNotRun,
		},
		{
			name:     "a head behind the base",
			args:     func(h gitleaksHistory) []string { return []string{h.third, h.first} },
			wantExit: 2,
			wantStopped: "gitleaks-range: THIRD..FIRST holds no commit, so there is nothing to " +
				"scan\n" + didNotRun,
		},
		{
			name: "a range without commits and --if-empty notice",
			args: func(h gitleaksHistory) []string {
				return []string{"--if-empty", "notice", h.third, h.third}
			},
			wantExit: 0,
			wantStopped: "gitleaks-range: THIRD..THIRD holds no commit; nothing to scan\n" +
				"gitleaks-range: PASS\n",
			wantAfter: "::notice title=Secret scan::THIRD..THIRD holds no commit, so no commit " +
				"was scanned for secrets\n",
		},
		{
			name: "--since-last-tag with a tag two commits back",
			args: func(gitleaksHistory) []string { return []string{"--since-last-tag", "HEAD"} },
			tags: func(h gitleaksHistory) map[string]string {
				return map[string]string{"v1.0.0": h.first}
			},
			report: "[]", ran: true, wantExit: 0,
			wantStopped: lastTag + scanned + "0\ngitleaks-range: PASS\n",
		},
		{
			// vnext matches v* but not v[0-9]*, and the head's own tag is not the one before it.
			name: "--since-last-tag with the head tagged",
			args: func(gitleaksHistory) []string { return []string{"--since-last-tag", "HEAD"} },
			tags: func(h gitleaksHistory) map[string]string {
				return map[string]string{"v1.0.0": h.first, "vnext": h.second, "v1.1.0": h.third}
			},
			report: "[]", ran: true, wantExit: 0,
			wantStopped: lastTag + scanned + "0\ngitleaks-range: PASS\n",
		},
		{
			name: "--since-last-tag after --if-empty",
			args: func(gitleaksHistory) []string {
				return []string{"--if-empty", "error", "--since-last-tag", "HEAD"}
			},
			tags: func(h gitleaksHistory) map[string]string {
				return map[string]string{"v1.0.0": h.first}
			},
			report: "[]", ran: true, wantExit: 0,
			wantStopped: lastTag + scanned + "0\ngitleaks-range: PASS\n",
		},
		{
			name: "--since-last-tag without a tag",
			args: func(gitleaksHistory) []string {
				return []string{"--since-last-tag", "HEAD"}
			},
			wantExit: 2,
			wantStopped: "gitleaks-range: no tag that matches v[0-9]* comes before the head " +
				"\"HEAD\", so there is no release to scan from; fetch the tags, as a clone of " +
				"the full history does (fetch-depth: 0)\n" + didNotRun,
		},
		{
			name:     "--since-last-tag without a head",
			args:     func(gitleaksHistory) []string { return []string{"--since-last-tag"} },
			wantExit: 2, wantStopped: usage + didNotRun,
		},
		{
			name: "--since-last-tag with a base",
			args: func(h gitleaksHistory) []string {
				return []string{"--since-last-tag", h.first, h.third}
			},
			wantExit: 2, wantStopped: usage + didNotRun,
		},
	}
	// Every repository links bin/gitleaks to one stub, since some systems check each new
	// executable the first time it runs, which is slow.
	stub := filepath.Join(t.TempDir(), "gitleaks")
	if err := execfile.WriteFile(stub, []byte(gitleaksStub), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lock, link := gitleaksLock, stub
			if tt.lock != "" {
				lock = tt.lock
			}
			if tt.noGitleaks {
				link = ""
			}
			var h gitleaksHistory
			h.root, h.first = newGitleaksRepository(t, lock, link)
			h.second = commitFiles(t, h.root, map[string]string{"a.txt": "a\n"}, "Second")
			h.third = commitFiles(t, h.root, map[string]string{"b.txt": "b\n"}, "Third")
			if tt.tags != nil {
				for name, commit := range tt.tags(h) {
					runGit(t, h.root, "tag", name, commit)
				}
			}
			// TMPDIR is a resolved path, so the paths the stub logs are the ones the script
			// gives it.
			work := t.TempDir()
			tmp, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			logFile, configCopy := filepath.Join(work, "log"), filepath.Join(work, "config")
			reportFile := ""
			if !tt.noReport {
				reportFile = filepath.Join(work, "report")
				if err := os.WriteFile(reportFile, []byte(tt.report), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			code, version := "0", "1.2.3"
			if tt.code != "" {
				code = tt.code
			}
			if tt.version != "" {
				version = tt.version
			}
			args := fromFirst(h)
			if tt.args != nil {
				args = tt.args(h)
			}
			env := append(gitEnv(h.root), "TMPDIR="+tmp,
				"STUB_LOG="+logFile, "STUB_CONFIG="+configCopy, "STUB_REPORT="+reportFile,
				"STUB_STDERR="+tt.stderr, "STUB_EXIT="+code, "STUB_VERSION="+version,
				"GITLEAKS_CONFIG="+filepath.Join(work, "missing.toml"),
				"GITLEAKS_CONFIG_TOML=title = \"from the environment\"")

			got := runScript(t, h.root, env,
				filepath.Join(h.root, ".github", "scripts", "gitleaks-range.sh"), args...)

			if got.exit != tt.wantExit {
				t.Errorf("exit = %d, want %d; stdout:\n%s", got.exit, tt.wantExit, got.stdout)
			}
			stopped, after := splitStopped(t, got.stdout)
			short := strings.NewReplacer("FIRST", h.first[:12], "SECOND", h.second[:12],
				"THIRD", h.third[:12])
			if want := short.Replace(tt.wantStopped); stopped != want {
				t.Errorf("printed with workflow commands stopped:\n%s\nwant:\n%s", stopped, want)
			}
			if want := short.Replace(tt.wantAfter); after != want {
				t.Errorf("printed after workflow commands resumed:\n%s\nwant:\n%s", after, want)
			}
			if got.stderr != "" {
				t.Errorf("stderr = %q, want nothing", got.stderr)
			}
			if tt.ran {
				checkGitleaksCall(t, logFile, configCopy, tmp, h)
			} else if _, err := os.Stat(logFile); err == nil {
				t.Errorf("gitleaks ran, want no run")
			}
			if left, err := os.ReadDir(tmp); err != nil || len(left) != 0 {
				t.Errorf("the script left %v in TMPDIR (%v), want nothing", left, err)
			}
		})
	}

	t.Run("a shallow clone", func(t *testing.T) {
		root, _ := newGitleaksRepository(t, gitleaksLock, stub)
		clone := filepath.Join(t.TempDir(), "clone")
		runGit(t, root, "clone", "--quiet", "--depth", "1", "file://"+root, clone)
		linkGitleaks(t, clone, stub)

		got := runScript(t, clone, append(gitEnv(clone), "STUB_VERSION=1.2.3"),
			filepath.Join(clone, ".github", "scripts", "gitleaks-range.sh"), "HEAD", "HEAD")

		stopped, _ := splitStopped(t, got.stdout)
		want := "gitleaks-range: the repository is a shallow clone, which may lack the commits " +
			"of the range; clone the full history (fetch-depth: 0)\n" + didNotRun
		if got.exit != 2 || stopped != want {
			t.Errorf("exit = %d, printed:\n%s\nwant exit 2 and:\n%s", got.exit, stopped, want)
		}
	})

	t.Run("without jq", func(t *testing.T) {
		root, first := newGitleaksRepository(t, gitleaksLock, stub)
		// PATH has every program the script runs before it looks for jq.
		bin := t.TempDir()
		for _, tool := range []string{"bash", "od", "tr"} {
			path, err := exec.LookPath(tool)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(path, filepath.Join(bin, tool)); err != nil {
				t.Fatal(err)
			}
		}

		got := runScript(t, root, []string{"PATH=" + bin},
			filepath.Join(root, ".github", "scripts", "gitleaks-range.sh"), first, first)

		stopped, after := splitStopped(t, got.stdout)
		want := "gitleaks-range: jq is not installed, so the gitleaks report cannot be read\n" +
			didNotRun
		if got.exit != 2 || stopped != want || after != "" {
			t.Errorf("exit = %d, stdout:\n%s\nwant exit 2 and:\n%s", got.exit, got.stdout, want)
		}
	})
}

// checkGitleaksCall checks that the stub ran once, with the arguments gitleaks-range.sh must give
// it: a configuration with the rules of .gitleaks.toml, an empty ignore file, a report in its
// temporary directory under tmp, and the git directory of h.root and the commits first..third to
// scan. It must run in an empty directory and with no GITLEAKS_ variable in its environment.
func checkGitleaksCall(t *testing.T, logFile, configCopy, tmp string, h gitleaksHistory) {
	t.Helper()
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	var pwds, files, ignoreSizes, args, env []string
	for line := range strings.Lines(string(data)) {
		key, value, _ := strings.Cut(strings.TrimSuffix(line, "\n"), " ")
		switch key {
		case "pwd":
			pwds = append(pwds, value)
		case "files":
			files = append(files, value)
		case "ignore-size":
			ignoreSizes = append(ignoreSizes, value)
		case "arg":
			args = append(args, value)
		case "env":
			env = append(env, value)
		default:
			t.Fatalf("the stub logged %q", line)
		}
	}
	if len(pwds) != 1 {
		t.Fatalf("gitleaks ran %d times, want once:\n%s", len(pwds), data)
	}
	work := filepath.Dir(pwds[0])
	if filepath.Dir(work) != tmp || filepath.Base(pwds[0]) != "empty" ||
		!slices.Equal(files, []string{"0"}) {
		t.Errorf("gitleaks ran in %s, which holds %v files; want an empty directory in a "+
			"temporary directory under %s", pwds[0], files, tmp)
	}
	if !slices.Equal(ignoreSizes, []string{"0"}) {
		t.Errorf("the ignore file gitleaks got has %v bytes, want 0", ignoreSizes)
	}
	if len(env) != 0 {
		t.Errorf("gitleaks ran with %q, want no GITLEAKS_ variable", env)
	}
	want := []string{
		"git", "--no-banner", "--redact", "--exit-code", "10",
		"--config", filepath.Join(work, "config.toml"),
		"--gitleaks-ignore-path", filepath.Join(work, "ignore"), "--ignore-gitleaks-allow",
		"--report-format", "json", "--report-path", filepath.Join(work, "report.json"),
		"--log-opts=--text --diff-merges=first-parent " + h.first + ".." + h.third,
	}
	if len(args) != len(want)+1 || !slices.Equal(args[:len(want)], want) {
		t.Fatalf("gitleaks ran with:\n%s\nwant the arguments:\n%s\nand the git directory",
			strings.Join(args, "\n"), strings.Join(want, "\n"))
	}
	gitDir := runScript(t, h.root, gitEnv(h.root), "git", "rev-parse", "--absolute-git-dir")
	if gitDir.exit != 0 {
		t.Fatalf("git rev-parse --absolute-git-dir exited %d: %s", gitDir.exit, gitDir.stderr)
	}
	scanned, err := filepath.EvalSymlinks(args[len(want)])
	if wantDir, _ := filepath.EvalSymlinks(strings.TrimSpace(gitDir.stdout)); err != nil ||
		scanned != wantDir {
		t.Errorf("gitleaks scanned %s, want the git directory %s", args[len(want)], wantDir)
	}
	config, err := os.ReadFile(configCopy)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(repositoryFile(".gitleaks.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := tomlLines(config), tomlLines(committed); !slices.Equal(got, want) ||
		!slices.Contains(want, `id = "permit-api-key"`) {
		t.Errorf("gitleaks got the configuration:\n%s\nwant the lines of .gitleaks.toml, "+
			"without comments:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// gitleaksBinary returns the absolute path of bin/gitleaks, which .github/scripts/fetch-tool.sh
// gitleaks installs, and fails the test when it is missing.
func gitleaksBinary(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(repositoryFile("bin/gitleaks"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("this test runs the gitleaks that tools/binaries.lock pins; install it with "+
			".github/scripts/fetch-tool.sh gitleaks: %v", err)
	}
	return path
}

// TestGitleaksRangeScans runs gitleaks-range.sh with the gitleaks that tools/binaries.lock pins
// on commits that hold a Permit API key, some of which try to keep gitleaks from reporting it.
func TestGitleaksRangeScans(t *testing.T) {
	gitleaks := gitleaksBinary(t)
	lock, err := os.ReadFile(repositoryFile("tools/binaries.lock"))
	if err != nil {
		t.Fatal(err)
	}
	// permitKey is built when the test runs, so that no key is in this file.
	permitKey := "permit_key_" + rand.Text() + rand.Text()
	keyFile := map[string]string{"key.go": "const k = \"" + permitKey + "\"\n"}
	allowAll := "[extend]\nuseDefault = true\n\n[[allowlists]]\ndescription = \"every path\"\n" +
		"paths = ['''.*''']\n"
	allowFile := filepath.Join(t.TempDir(), "allow.toml")
	if err = os.WriteFile(allowFile, []byte(allowAll), 0o600); err != nil {
		t.Fatal(err)
	}
	// prettyConfig is a user's git configuration that changes the format of git log, which
	// gitleaks reads.
	prettyConfig := filepath.Join(t.TempDir(), "gitconfig")
	err = os.WriteFile(prettyConfig, []byte("[format]\n\tpretty = oneline\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	finding := func(file string, line int, commit string) string {
		return fmt.Sprintf("%s:%d: permit-api-key (Permit API key), commit %s\n", file, line,
			commit[:12])
	}
	tests := []struct {
		name string
		// commit makes the commits to scan and returns the findings the script must print.
		commit func(t *testing.T, root string) []string
		env    []string
	}{
		{
			name: "a Permit API key",
			commit: func(t *testing.T, root string) []string {
				t.Helper()
				return []string{finding("key.go", 1, commitFiles(t, root, keyFile, "Add a key"))}
			},
		},
		{
			name: "a fake Permit API key",
			commit: func(t *testing.T, root string) []string {
				t.Helper()
				commitFiles(t, root, map[string]string{
					"fake.go": "const k = \"permit_key_FAKE" + rand.Text() + "\"\n",
				}, "Add a fake key")
				return nil
			},
		},
		{
			name: "commits without a secret",
			commit: func(t *testing.T, root string) []string {
				t.Helper()
				commitFiles(t, root, map[string]string{"a.go": "package a\n"}, "Add a")
				commitFiles(t, root, map[string]string{"b.go": "package b\n"}, "Add b")
				return nil
			},
		},
		{
			name: "a key that a later commit removes",
			commit: func(t *testing.T, root string) []string {
				t.Helper()
				added := commitFiles(t, root, keyFile, "Add a key")
				commitFiles(t, root, map[string]string{"key.go": "package a\n"}, "Remove it")
				return []string{finding("key.go", 1, added)}
			},
		},
		{
			name: "a .gitleaks.toml that allows every path",
			commit: func(t *testing.T, root string) []string {
				t.Helper()
				commitFiles(t, root, map[string]string{".gitleaks.toml": allowAll}, "Allow")
				return []string{finding("key.go", 1, commitFiles(t, root, keyFile, "Add a key"))}
			},
		},
		{
			name: "a .gitleaks.toml without the Permit API key rule",
			commit: func(t *testing.T, root string) []string {
				t.Helper()
				commitFiles(t, root, map[string]string{
					".gitleaks.toml": "[extend]\nuseDefault = true\n",
				}, "Use the default rules")
				return []string{finding("key.go", 1, commitFiles(t, root, keyFile, "Add a key"))}
			},
		},
		{
			name: "GITLEAKS_ variables that allow every path",
			commit: func(t *testing.T, root string) []string {
				t.Helper()
				return []string{finding("key.go", 1, commitFiles(t, root, keyFile, "Add a key"))}
			},
			env: []string{"GITLEAKS_CONFIG=" + allowFile, "GITLEAKS_CONFIG_TOML=" + allowAll},
		},
		{
			name: "a .gitleaksignore with the finding's fingerprint",
			commit: func(t *testing.T, root string) []string {
				t.Helper()
				added := commitFiles(t, root, keyFile, "Add a key")
				commitFiles(t, root, map[string]string{
					".gitleaksignore": added + ":key.go:permit-api-key:1\n",
				}, "Ignore it")
				return []string{finding("key.go", 1, added)}
			},
		},
		{
			name: "a gitleaks:allow comment",
			commit: func(t *testing.T, root string) []string {
				t.Helper()
				added := commitFiles(t, root, map[string]string{
					"key.go": "const k = \"" + permitKey + "\" // gitleaks:allow\n",
				}, "Add a key")
				return []string{finding("key.go", 1, added)}
			},
		},
		{
			name: "a .gitattributes that marks every file binary",
			commit: func(t *testing.T, root string) []string {
				t.Helper()
				commitFiles(t, root, map[string]string{".gitattributes": "* binary\n"}, "Binary")
				return []string{finding("key.go", 1, commitFiles(t, root, keyFile, "Add a key"))}
			},
		},
		{
			// git log -p prints only that the file differs, unless it is given --text.
			name: "a key in a file that git takes for binary, as it holds a NUL byte",
			commit: func(t *testing.T, root string) []string {
				t.Helper()
				added := commitFiles(t, root, map[string]string{
					"example": "\x00\nconst k = \"" + permitKey + "\"\n",
				}, "Add a build of an example")
				return []string{finding("example", 2, added)}
			},
		},
		{
			name: "a user git configuration that sets the format of git log",
			commit: func(t *testing.T, root string) []string {
				t.Helper()
				return []string{finding("key.go", 1, commitFiles(t, root, keyFile, "Add a key"))}
			},
			env: []string{"GIT_CONFIG_GLOBAL=" + prettyConfig},
		},
		{
			name: "a key added while resolving a merge",
			commit: func(t *testing.T, root string) []string {
				t.Helper()
				runGit(t, root, "checkout", "--quiet", "-b", "side")
				commitFiles(t, root, map[string]string{"one.go": "package one\n"}, "One")
				runGit(t, root, "checkout", "--quiet", "-")
				commitFiles(t, root, map[string]string{"two.go": "package two\n"}, "Two")
				runGit(t, root, "merge", "--quiet", "--no-commit", "side")
				merge := commitFiles(t, root, map[string]string{
					"merge.go": "const k = \"" + permitKey + "\"\n",
				}, "Merge side")
				return []string{finding("merge.go", 1, merge)}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, first := newGitleaksRepository(t, string(lock), gitleaks)
			findings := tt.commit(t, root)
			got := checkScan(t, root, first, tt.env, findings)
			if strings.Contains(got.stdout+got.stderr, permitKey) {
				t.Errorf("the output holds the key:\n%s%s", got.stdout, got.stderr)
			}
		})
	}

	t.Run("a linked worktree", func(t *testing.T) {
		root, first := newGitleaksRepository(t, string(lock), gitleaks)
		worktree := filepath.Join(t.TempDir(), "worktree")
		runGit(t, root, "worktree", "add", "--quiet", "-b", "side", worktree)
		linkGitleaks(t, worktree, gitleaks)
		added := commitFiles(t, worktree, keyFile, "Add a key")
		got := checkScan(t, worktree, first, nil, []string{finding("key.go", 1, added)})
		if strings.Contains(got.stdout+got.stderr, permitKey) {
			t.Errorf("the output holds the key:\n%s%s", got.stdout, got.stderr)
		}
	})
}

// checkScan runs gitleaks-range.sh in root on first..HEAD, with env added to the environment,
// and checks that it prints each of findings and exits 1 when there are findings and 0 when there
// are none. It returns the script's result, so the caller can check that no secret was printed.
func checkScan(t *testing.T, root, first string, env, findings []string) scriptResult {
	t.Helper()
	head := revParse(t, root, "HEAD")
	count := runScript(t, root, gitEnv(root), "git", "rev-list", "--count", first+"..HEAD")
	if count.exit != 0 {
		t.Fatalf("git rev-list --count %s..HEAD exited %d: %s", first, count.exit, count.stderr)
	}

	got := runScript(t, root, append(gitEnv(root), env...),
		filepath.Join(root, ".github", "scripts", "gitleaks-range.sh"), first, "HEAD")

	wantExit, verdict := 0, "PASS"
	if len(findings) > 0 {
		wantExit, verdict = 1, "FAIL"
	}
	if got.exit != wantExit {
		t.Errorf("exit = %d, want %d; stdout:\n%s", got.exit, wantExit, got.stdout)
	}
	stopped, _ := splitStopped(t, got.stdout)
	summary := fmt.Sprintf("gitleaks-range: %s..%s: gitleaks ", first[:12], head[:12])
	tail := fmt.Sprintf("; commits scanned: %s, findings: %d\ngitleaks-range: %s\n",
		strings.TrimSpace(count.stdout), len(findings), verdict)
	if !strings.Contains(stopped, summary) || !strings.HasSuffix(stopped, tail) {
		t.Errorf("stdout does not end with %q...%q:\n%s", summary, tail, got.stdout)
	}
	for _, want := range findings {
		if !strings.Contains(stopped, want) {
			t.Errorf("stdout does not report %q:\n%s", want, got.stdout)
		}
	}
	return got
}
