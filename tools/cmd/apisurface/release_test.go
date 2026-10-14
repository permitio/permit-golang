package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"

	"github.com/permitio/permit-golang/v2/tools/internal/execfile"
)

const (
	// releaseModule is the module path of the release fixtures.
	releaseModule = "example.com/sdk/v2"
	// releaseGoMod is their go.mod.
	releaseGoMod = "module example.com/sdk/v2\n\ngo 1.26.0\n"
	// apiV1 is the API the tagged versions have; apiCompatible adds to it, and apiIncompatible
	// changes it.
	apiV1 = "// Package sdk is a fixture.\npackage sdk\n\n// F does nothing.\n" +
		"func F(a int) {}\n"
	apiCompatible   = apiV1 + "\n// G does nothing.\nfunc G() {}\n"
	apiIncompatible = "// Package sdk is a fixture.\npackage sdk\n\n// F does nothing.\n" +
		"func F(a string) {}\n"
	// noPending is a pending.txt that lists nothing.
	noPending = "# Nothing is pending.\n"
)

// releaseFixture is a repository with tags and a file module proxy that serves the published
// ones.
type releaseFixture struct {
	dir, proxy string
}

// newReleaseFixture creates a repository whose tags each mark a commit of apiV1 and noPending;
// the proxy serves the published tags. The working tree then holds head as doc.go and pending as
// tools/migration/pending.txt, which an empty pending removes.
func newReleaseFixture(t *testing.T, tags, published []string, head, pending string,
) releaseFixture {
	t.Helper()
	f := releaseFixture{
		dir: newRepo(t, map[string]string{
			"go.mod": releaseGoMod, "doc.go": apiV1, pendingFile: noPending,
		}),
		proxy: t.TempDir(),
	}
	for _, tag := range tags {
		gitIn(t, f.dir, "tag", tag)
	}
	for _, version := range published {
		f.publish(t, version)
	}
	writeFiles(t, f.dir, map[string]string{"doc.go": head, pendingFile: pending})
	commitAll(t, f.dir, "head")
	return f
}

// publish writes the module zip, go.mod, info and list files of a version of apiV1 into the
// proxy.
func (f releaseFixture) publish(t *testing.T, version string) {
	t.Helper()
	src := t.TempDir()
	writeFiles(t, src, map[string]string{"go.mod": releaseGoMod, "doc.go": apiV1})
	dir := filepath.Join(f.proxy, filepath.FromSlash(releaseModule), "@v")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	zipFile, err := os.Create(filepath.Join(dir, version+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer zipFile.Close()
	m := module.Version{Path: releaseModule, Version: version}
	if err = modzip.CreateFromDir(zipFile, m, src); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, dir, map[string]string{
		version + ".mod":  releaseGoMod,
		version + ".info": fmt.Sprintf(`{"Version":%q,"Time":"2026-01-02T03:04:05Z"}`, version),
	})
	list, err := os.ReadFile(filepath.Join(dir, "list"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	writeFiles(t, dir, map[string]string{"list": string(list) + version + "\n"})
}

// sys returns a system whose go command reads modules from the fixture's proxy only.
func (f releaseFixture) sys(t *testing.T) system {
	t.Helper()
	return system{extraEnv: []string{
		"GOPROXY=file://" + filepath.ToSlash(f.proxy), "GOSUMDB=off", "GOENV=off",
		"GOFLAGS=-modcacherw", "GOMODCACHE=" + t.TempDir(), "GOPATH=" + t.TempDir(),
	}}
}

func TestRelease(t *testing.T) {
	t.Parallel()
	gorelease := buildTool(t, "golang.org/x/exp/cmd/gorelease")
	for name, tc := range map[string]struct {
		tags, published []string
		head, version   string
		// pending is the working tree's pending.txt; empty means noPending.
		pending string
		code    int
		want    []string
	}{
		"the first release": {
			tags: []string{"v1.2.8", "v1.3.0"},
			head: apiIncompatible, version: "v2.0.0-rc.1",
			code: exitPassed,
			want: []string{
				"gorelease -base=none -version=v2.0.0-rc.1\n",
				"v2.0.0-rc.1 is a valid semantic version for this release.",
				"PASS: gorelease accepts v2.0.0-rc.1 against none\n",
			},
		},
		"incompatible changes after a prerelease": {
			tags: []string{"v1.3.0", "v2.0.0-rc.1"}, published: []string{"v2.0.0-rc.1"},
			head: apiIncompatible, version: "v2.0.0-rc.2",
			code: exitPassed,
			want: []string{
				"gorelease -base=v2.0.0-rc.1\n",
				"  incompatible: F: changed from func(int) to func(string)\n",
				"PASS: v2.0.0-rc.2: 1 incompatible change since the prerelease v2.0.0-rc.1, " +
					"allowed after a prerelease\n",
			},
		},
		"compatible changes after a prerelease": {
			tags: []string{"v2.0.0-rc.1"}, published: []string{"v2.0.0-rc.1"},
			head: apiCompatible, version: "v2.0.0",
			code: exitPassed,
			want: []string{
				"gorelease -base=v2.0.0-rc.1\n",
				"PASS: gorelease accepts v2.0.0 against v2.0.0-rc.1\n",
			},
		},
		"incompatible changes after a release": {
			tags:      []string{"v2.0.0-rc.1", "v2.0.0"},
			published: []string{"v2.0.0-rc.1", "v2.0.0"},
			head:      apiIncompatible, version: "v2.1.0",
			code: exitFailed,
			want: []string{
				"gorelease -base=v2.0.0 -version=v2.1.0\n",
				"  incompatible: F: changed from func(int) to func(string)\n",
				"v2.1.0 is not a valid semantic version for this release.",
				"- gorelease rejects v2.1.0 against v2.0.0\n", "FAIL: 1 problem\n",
			},
		},
		"compatible changes after a release": {
			tags: []string{"v2.0.0"}, published: []string{"v2.0.0"},
			head: apiCompatible, version: "v2.1.0",
			code: exitPassed,
			want: []string{
				"gorelease -base=v2.0.0 -version=v2.1.0\n",
				"PASS: gorelease accepts v2.1.0 against v2.0.0\n",
			},
		},
		"a patch release with new API": {
			tags: []string{"v2.0.0"}, published: []string{"v2.0.0"},
			head: apiCompatible, version: "v2.0.1",
			code: exitFailed,
			want: []string{
				"the minor version is not incremented",
				"- gorelease rejects v2.0.1 against v2.0.0\n",
			},
		},
		"other tags": {
			tags: []string{
				"v1.9.0", "v2.0.0", "v2.0.0-rc.1", "v2.1.0", "v2.2.0", "v3.0.0", "2.1.0",
				"v2.1", "latest",
			},
			published: []string{"v2.0.0"},
			head:      apiCompatible, version: "v2.1.0",
			code: exitPassed,
			want: []string{"gorelease -base=v2.0.0 -version=v2.1.0\n", "PASS: "},
		},
		"pending lines": {
			head: apiCompatible, version: "v2.0.0",
			pending: noPending + "\napi.Users users\napi.Roles.* roles\n",
			code:    exitFailed,
			want: []string{
				"gorelease -base=none -version=v2.0.0\n",
				"- tools/migration/pending.txt still has 2 lines: the release needs every v1 " +
					"item to have its entry in MIGRATION.md; write the entries and remove the " +
					"lines\n",
				"FAIL: 1 problem\n",
			},
		},
		"pending lines and a version gorelease rejects": {
			tags: []string{"v2.0.0"}, published: []string{"v2.0.0"},
			head: apiCompatible, version: "v2.0.1",
			pending: "api.Users users\n",
			code:    exitFailed,
			want: []string{
				"- gorelease rejects v2.0.1 against v2.0.0\n",
				"- tools/migration/pending.txt still has 1 line: ",
				"FAIL: 2 problems\n",
			},
		},
		"a base the proxy does not have": {
			tags: []string{"v2.0.0"},
			head: apiCompatible, version: "v2.1.0",
			code: exitDidNotRun,
			want: []string{"DID NOT RUN: gorelease -base=v2.0.0 -version=v2.1.0 exited 1 " +
				"without a verdict: gorelease: "},
		},
		"a version of another major version": {
			head: apiCompatible, version: "v3.0.0",
			code: exitDidNotRun,
			want: []string{"DID NOT RUN: -version \"v3.0.0\" is not a canonical semantic " +
				"version of the major version of example.com/sdk/v2, such as v2.0.0"},
		},
		"a version that is not canonical": {
			head: apiCompatible, version: "v2.1",
			code: exitDidNotRun,
			want: []string{"DID NOT RUN: -version \"v2.1\" is not a canonical semantic version"},
		},
		"a version without a v": {
			head: apiCompatible, version: "2.1.0",
			code: exitDidNotRun,
			want: []string{"DID NOT RUN: -version \"2.1.0\" is not a canonical semantic version"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pending := tc.pending
			if pending == "" {
				pending = noPending
			}
			f := newReleaseFixture(t, tc.tags, tc.published, tc.head, pending)
			code, out := runTool(t, f.sys(t), "-mode", "release", "-root", f.dir, "-version",
				tc.version, "-gorelease", gorelease)
			wantRun(t, code, out, tc.code, tc.want...)
		})
	}
}

// fakeGorelease writes a script that prints out and exits with code, and returns its path.
func fakeGorelease(t *testing.T, out string, code int) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "gorelease")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s' '%s'\nexit %d\n", out, code)
	if err := execfile.WriteFile(name, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestReleaseWithoutAVerdict(t *testing.T) {
	t.Parallel()
	const invalid = "# summary\nv2.1.0 is not a valid semantic version for this release.\n"
	for name, tc := range map[string]struct {
		tags []string
		// out and exit are what the fake gorelease prints and its exit code; missing runs a
		// gorelease that does not exist instead.
		out     string
		exit    int
		missing bool
		want    string
	}{
		"an exit code gorelease does not use": {
			out: invalid, exit: 3,
			want: "exited 3 without a verdict",
		},
		"a success without a summary": {
			want: "exited 0 without a verdict",
		},
		"a failure without a summary": {
			out: "gorelease: x\n", exit: 1,
			want: "exited 1 without a verdict: gorelease: x",
		},
		"a rejection after a prerelease": {
			tags: []string{"v2.0.0-rc.1"}, out: invalid, exit: 1,
			want: "gorelease -base=v2.0.0-rc.1 exited 1 without a verdict",
		},
		"a failure that is not a rejection": {
			out: "# summary\nSuggested version: v2.1.0\n", exit: 1,
			want: "exited 1 without a verdict",
		},
		"a missing gorelease": {
			missing: true,
			want:    "DID NOT RUN: running gorelease: ",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newReleaseFixture(t, tc.tags, nil, apiCompatible, noPending)
			gorelease := filepath.Join(t.TempDir(), "none")
			if !tc.missing {
				gorelease = fakeGorelease(t, tc.out, tc.exit)
			}
			code, out := runTool(t, system{}, "-mode", "release", "-root", f.dir, "-version",
				"v2.1.0", "-gorelease", gorelease)
			wantRun(t, code, out, exitDidNotRun, tc.want)
		})
	}
	t.Run("no pending.txt", func(t *testing.T) {
		t.Parallel()
		f := newReleaseFixture(t, nil, nil, apiCompatible, "")
		const accepted = "# summary\nv2.1.0 is a valid semantic version for this release.\n"
		valid := fakeGorelease(t, accepted, 0)
		code, out := runTool(t, system{}, "-mode", "release", "-root", f.dir, "-version", "v2.1.0",
			"-gorelease", valid)
		wantRun(t, code, out, exitDidNotRun, "DID NOT RUN: open ", "pending.txt: no such file")
	})
	t.Run("not a repository", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"go.mod": releaseGoMod})
		code, out := runTool(t, system{}, "-mode", "release", "-root", dir, "-version", "v2.1.0",
			"-gorelease", fakeGorelease(t, "", 0))
		wantRun(t, code, out, exitDidNotRun, "DID NOT RUN: git tag --list: ")
	})
	t.Run("no go.mod", func(t *testing.T) {
		t.Parallel()
		code, out := runTool(t, system{}, "-mode", "release", "-root", t.TempDir(), "-version",
			"v2.1.0", "-gorelease", fakeGorelease(t, "", 0))
		wantRun(t, code, out, exitDidNotRun, "DID NOT RUN: open ")
	})
}

func TestParseGorelease(t *testing.T) {
	out := `# example.com/sdk/v2
## incompatible changes
F: changed from func(int) to func(string)
## compatible changes
G: added

# example.com/sdk/v2/models
## incompatible changes
User: removed

# summary
v2.1.0 is not a valid semantic version for this release.
There are incompatible changes.
`
	incompatible, summary := parseGorelease([]byte(out))
	wantIncompatible := []string{"F: changed from func(int) to func(string)", "User: removed"}
	if !slices.Equal(incompatible, wantIncompatible) {
		t.Errorf("incompatible = %q, want %q", incompatible, wantIncompatible)
	}
	wantSummary := []string{
		"v2.1.0 is not a valid semantic version for this release.",
		"There are incompatible changes.",
	}
	if !slices.Equal(summary, wantSummary) {
		t.Errorf("summary = %q, want %q", summary, wantSummary)
	}
	// Once the summary starts, every line is the summary's.
	incompatible, summary = parseGorelease([]byte("# summary\n## incompatible changes\nx\n" +
		"# other\ny\n"))
	wantSummary = []string{"## incompatible changes", "x", "# other", "y"}
	if len(incompatible) != 0 || !slices.Equal(summary, wantSummary) {
		t.Errorf("headings inside the summary: incompatible %q, summary %q, want none and %q",
			incompatible, summary, wantSummary)
	}
}
