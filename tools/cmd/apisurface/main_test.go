package main

import (
	"bytes"
	"maps"
	"strings"
	"testing"
)

func TestBadArgs(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"no mode": {
			want: "DID NOT RUN: -mode \"\" is not migration, diff, pr or release; or give -module",
		},
		"an unknown mode": {
			args: []string{"-mode", "check"},
			want: "-mode \"check\" is not migration, diff, pr or release",
		},
		"the module mode by name": {
			args: []string{"-mode", "module"},
			want: "-mode \"module\" is not migration, diff, pr or release",
		},
		"both -module and -mode": {
			args: []string{"-module", "example.com/sdk@v1.0.0", "-mode", "migration"},
			want: "DID NOT RUN: give -module or -mode, not both",
		},
		"an argument": {
			args: []string{"-mode", "migration", "extra"},
			want: "DID NOT RUN: unexpected arguments [\"extra\"]",
		},
		"an unknown flag": {
			args: []string{"-mode", "migration", "-label"},
			want: "DID NOT RUN: flag provided but not defined: -label",
		},
		"a flag of another mode": {
			args: []string{"-mode", "migration", "-base", "HEAD", "-apidiff", "x"},
			want: "DID NOT RUN: -mode migration does not take -apidiff, -base",
		},
		"a mode flag with -module": {
			args: []string{"-module", "example.com/sdk@v1.0.0", "-root", "."},
			want: "DID NOT RUN: -module does not take -root",
		},
		"-write in diff mode": {
			args: []string{"-mode", "diff", "-base", "HEAD", "-apidiff", "x", "-write"},
			want: "DID NOT RUN: -mode diff does not take -write",
		},
		"-ids-base in pull request mode": {
			args: []string{"-mode", "pr", "-base", "HEAD", "-ids-base", "HEAD"},
			want: "DID NOT RUN: -mode pr does not take -ids-base",
		},
		"-version in migration mode": {
			args: []string{"-mode", "migration", "-version", "v2.0.0"},
			want: "DID NOT RUN: -mode migration does not take -version",
		},
		"an empty -spec": {
			args: []string{"-module", "example.com/sdk@v1.0.0", "-spec", ""},
			want: "DID NOT RUN: -spec and -out must name files",
		},
		"an empty -out": {
			args: []string{"-module", "example.com/sdk@v1.0.0", "-out", ""},
			want: "DID NOT RUN: -spec and -out must name files",
		},
		"an empty -root": {
			args: []string{"-mode", "migration", "-root", ""},
			want: "DID NOT RUN: -root is empty",
		},
		"diff mode without a base": {
			args: []string{"-mode", "diff", "-apidiff", "x"},
			want: "DID NOT RUN: -mode diff needs -base, the pull request's base commit",
		},
		"diff mode without apidiff": {
			args: []string{"-mode", "diff", "-base", "HEAD"},
			want: "DID NOT RUN: -mode diff needs -apidiff, the apidiff binary",
		},
		"pull request mode without a base": {
			args: []string{"-mode", "pr"},
			want: "DID NOT RUN: -mode pr needs -base, the pull request's base commit",
		},
		"release mode without a version": {
			args: []string{"-mode", "release", "-gorelease", "x"},
			want: "DID NOT RUN: -mode release needs -version and -gorelease",
		},
		"release mode without gorelease": {
			args: []string{"-mode", "release", "-version", "v2.0.0"},
			want: "DID NOT RUN: -mode release needs -version and -gorelease",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(t.Context(), tc.args, &stdout, &stderr, system{})
			if code != exitDidNotRun {
				t.Errorf("exit code %d, want 2", code)
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tc.want)
			}
			if got := lastLine(stdout.String()); !strings.HasPrefix(got, "DID NOT RUN: ") {
				t.Errorf("the last line is %q, want the verdict", got)
			}
		})
	}
}

func TestCounted(t *testing.T) {
	for n, want := range map[int]string{0: "0 problems", 1: "1 problem", 2: "2 problems"} {
		if got := counted(n, "problem"); got != want {
			t.Errorf("counted(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestPrintVerdict(t *testing.T) {
	for msg, want := range map[string]string{
		"one line":                   "FAIL: one line\n",
		"one line\n":                 "FAIL: one line\n",
		"first\nsecond\n \n":         "first\nsecond\nFAIL: first\n",
		"cmd: exit 1: out\nSECURITY": "cmd: exit 1: out\nSECURITY\nFAIL: cmd: exit 1: out\n",
	} {
		var b bytes.Buffer
		printVerdict(&b, "FAIL", msg)
		if b.String() != want {
			t.Errorf("printVerdict(%q) printed %q, want %q", msg, b.String(), want)
		}
	}
}

func TestPullRequestMode(t *testing.T) {
	t.Parallel()
	t.Run("a base of another major version", func(t *testing.T) {
		t.Parallel()
		dir := newRepo(t, map[string]string{
			"go.mod": "module example.com/sdk\n\ngo 1.19\n", "main.go": "package main\n",
		})
		base := gitIn(t, dir, "rev-parse", "HEAD")
		gitIn(t, dir, "rm", "--quiet", "main.go")
		writeFiles(t, dir, fixtureMigration())
		commitAll(t, dir, "v2")
		code, out := runTool(t, system{}, "-mode", "pr", "-root", dir, "-base", base)
		wantRun(t, code, out, exitPassed, "the base is example.com/sdk and the working tree "+
			"example.com/sdk/v2: migration mode\n", "PASS: each of the 9 v1 items is covered once")

		// Migration mode checks that ids.txt only grew since the base.
		writeFiles(t, dir, map[string]string{idsFile: "M-module-1\nM-users-1\nM-models-1\n"})
		code, out = runTool(t, system{}, "-mode", "pr", "-root", dir, "-base", base)
		wantRun(t, code, out, exitFailed, "- the working tree removes M-members-1 from "+
			"tools/migration/ids.txt", "M-members-1 is not in tools/migration/ids.txt")
	})
	t.Run("a base of the same module", func(t *testing.T) {
		t.Parallel()
		apidiff := buildTool(t, "golang.org/x/exp/cmd/apidiff")
		dir := newRepo(t, fixtureSDK())
		base := gitIn(t, dir, "rev-parse", "HEAD")
		files := maps.Clone(fixtureSDK())
		files["doc.go"] = strings.Replace(files["doc.go"], "F(a int)", "F(a string)", 1)
		writeFiles(t, dir, files)
		code, out := runTool(t, system{}, "-mode", "pr", "-root", dir, "-base", base,
			"-apidiff", apidiff)
		wantRun(t, code, out, exitFailed, "the base and the working tree are "+
			"example.com/sdk/v2: diff mode\n", "  apidiff: F: changed from func(int) to "+
			"func(string)\n", "the pull request does not have the breaking label")
		code, out = runTool(t, system{}, "-mode", "pr", "-root", dir, "-base", base,
			"-apidiff", apidiff, "-breaking-label")
		wantRun(t, code, out, exitPassed, "PASS: 1 breaking change, and the pull request has "+
			"the breaking label\n")
		code, out = runTool(t, system{}, "-mode", "pr", "-root", dir, "-base", base)
		wantRun(t, code, out, exitDidNotRun, "DID NOT RUN: diff mode needs -apidiff, the "+
			"apidiff binary\n")
	})
	t.Run("a base without go.mod", func(t *testing.T) {
		t.Parallel()
		dir := newRepo(t, map[string]string{"README.md": "x\n"})
		base := gitIn(t, dir, "rev-parse", "HEAD")
		writeFiles(t, dir, fixtureSDK())
		commitAll(t, dir, "v2")
		code, out := runTool(t, system{}, "-mode", "pr", "-root", dir, "-base", base)
		wantRun(t, code, out, exitDidNotRun, "DID NOT RUN: "+base+" has no go.mod at the "+
			"repository root\n")
	})
	t.Run("a go.mod without a module path", func(t *testing.T) {
		t.Parallel()
		dir := newRepo(t, map[string]string{"go.mod": "go 1.26.0\n"})
		code, out := runTool(t, system{}, "-mode", "pr", "-root", dir, "-base", "HEAD")
		wantRun(t, code, out, exitDidNotRun, "DID NOT RUN: the go.mod of the working tree "+
			"names no module path\n")
		writeFiles(t, dir, map[string]string{"go.mod": "module example.com/sdk/v2\n"})
		code, out = runTool(t, system{}, "-mode", "pr", "-root", dir, "-base", "HEAD")
		wantRun(t, code, out, exitDidNotRun, "DID NOT RUN: the go.mod of HEAD names no module "+
			"path\n")
	})
	t.Run("an unknown base", func(t *testing.T) {
		t.Parallel()
		dir := newRepo(t, fixtureSDK())
		code, out := runTool(t, system{}, "-mode", "pr", "-root", dir, "-base", "nope")
		wantRun(t, code, out, exitDidNotRun, "DID NOT RUN: git ls-tree --name-only nope -- "+
			"go.mod: ")
	})
}

func TestModuleMode(t *testing.T) {
	// A wrong module argument stops before anything is downloaded.
	code, out := runTool(t, system{}, "-module", "example.com/sdk")
	wantRun(t, code, out, exitDidNotRun, "DID NOT RUN: -module \"example.com/sdk\" is not "+
		"<module path>@<version>\n")
}
