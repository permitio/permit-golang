package scripts

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/mod/module"
)

// dryrunModule is the module version that the dry run of the fixture builds: the Version of its
// version.go, 2.1.0, with the suffix -dryrun.
var dryrunModule = module.Version{Path: "example.com/m/v2", Version: "v2.1.0-dryrun"}

// The stubs of the dry run's tools append their command lines to the file STUB_LOG names.
// releasecheck lanes prints STUB_LANES and exits with STUB_LANES_EXIT; any other releasecheck
// copies the files that -ci-runs and -evidence name into STUB_DIR, and exits with STUB_VALIDATE.
// apisurface exits with STUB_MIGRATION. zipcheck -repo <dir> -version <v> -out <file> copies
// STUB_DIR/module.zip to <file>, prints zipcheck's first line with the version STUB_VERSION, or
// <v>, and the hash STUB_HASH, and exits with STUB_ZIPCHECK; zipcheck -zip <file> exits 0.
const (
	releasecheckStub = `#!/bin/sh
printf 'releasecheck %s\n' "$*" >>"$STUB_LOG"
if [ "$1" = lanes ]; then
	printf '%s' "$STUB_LANES"
	exit "${STUB_LANES_EXIT:-0}"
fi
while [ $# -gt 0 ]; do
	case $1 in
	-ci-runs) cp "$2" "$STUB_DIR/ci-runs.json" ;;
	-evidence) cp "$2" "$STUB_DIR/evidence.json" ;;
	esac
	shift
done
exit "${STUB_VALIDATE:-0}"
`
	apisurfaceStub = `#!/bin/sh
printf 'apisurface %s\n' "$*" >>"$STUB_LOG"
exit "${STUB_MIGRATION:-0}"
`
	dryrunZipcheckStub = `#!/bin/sh
printf 'zipcheck %s\n' "$*" >>"$STUB_LOG"
if [ "$1" = -zip ]; then
	exit 0
fi
cp "$STUB_DIR/module.zip" "$6"
printf 'zipcheck: example.com/m/v2@%s %s\n' "${STUB_VERSION:-$4}" "$STUB_HASH"
exit "${STUB_ZIPCHECK:-0}"
`
)

// dryrunCase is a run of release-dryrun.sh: its arguments, the zip that zipcheck writes and the
// hash it prints, the files of the repository that change, and the environment that changes.
type dryrunCase struct {
	name string
	// args are the arguments, HEAD's commit when nil.
	args []string
	// zip is the zip that zipcheck writes, and hash the hash it prints, the genuine zip's by
	// default.
	zip  []byte
	hash string
	// files replace files of the repository, and an empty content removes the file; env adds to
	// the environment.
	files map[string]string
	env   []string
	// wantExit is the exit code, want parts of what the script prints, and ran the tools that
	// it ran, in order, each by a prefix of its command line; {head} in want and ran is HEAD's
	// commit, and <root> in ran the repository root.
	wantExit int
	want     []string
	ran      []string
}

// dryrunResult is a run of release-dryrun.sh: its result; the commit that HEAD names; the tools
// it ran, with the repository root and that commit written <root> and {head}; and the runs of CI
// and the harness evidence that it gave releasecheck.
type dryrunResult struct {
	scriptResult
	head             string
	ran              []string
	ciRuns, evidence string
}

// runDryrun runs release-dryrun.sh in a git repository whose version.go has the Version 2.1.0,
// with the stubs of its tools and the real go, git and jq.
func runDryrun(t *testing.T, tc dryrunCase, genuine []byte, hash string) dryrunResult {
	t.Helper()
	root := t.TempDir()
	copyScripts(t, root, "release-dryrun.sh", "release-verify.sh")
	writeExecutable(t, root, "bin/releasecheck", releasecheckStub)
	writeExecutable(t, root, "bin/apisurface", apisurfaceStub)
	writeExecutable(t, root, "bin/zipcheck", dryrunZipcheckStub)
	files := map[string]string{
		"version.go": "package permit\n\n// Version is the version.\nconst Version = \"2.1.0\"\n",
		"tools/e2e/local-only.txt": "# Local-only methods.\n\n  # A.B org-key api.old\n" +
			"ProjectsService.List org-key api.projects\nAPIKeysService.Get org-key api.api_keys\n" +
			"ProjectsService.Create org-key api.projects\n",
		"tools/migration/pending.txt": "# Pending v1 items.\n\n  # api.Old.* old\n",
	}
	maps.Copy(files, tc.files)
	for name, content := range files {
		if content != "" {
			continue
		}
		delete(files, name)
		err := os.Remove(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	writeFiles(t, root, files)
	stubDir := filepath.Join(root, "stubdir")
	zip := genuine
	if tc.zip != nil {
		zip = tc.zip
	}
	if err := os.MkdirAll(stubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stubDir, "module.zip"), zip, 0o600); err != nil {
		t.Fatal(err)
	}
	gitInit(t, root)
	head := commit(t, root, "The release.")
	log := filepath.Join(root, "tools.log")
	env := append(gitEnv(root), offline...)
	env = append(env, "STUB_LOG="+log, "STUB_DIR="+stubDir, "STUB_HASH="+orHash(tc.hash, hash),
		"STUB_LANES=go1.26.0\ngo1.26.9\ngo1.27.2\n")
	env = append(env, tc.env...)
	args := tc.args
	if args == nil {
		args = []string{head}
	}

	result := dryrunResult{
		scriptResult: runScript(t, root, env,
			filepath.Join(root, ".github/scripts/release-dryrun.sh"), args...),
		head: head,
	}

	if data, err := os.ReadFile(log); err == nil {
		for line := range strings.Lines(string(data)) {
			line = strings.ReplaceAll(strings.ReplaceAll(line, root, "<root>"), head, "{head}")
			result.ran = append(result.ran, strings.TrimSpace(line))
		}
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	result.ciRuns = readStubFile(t, stubDir, "ci-runs.json")
	result.evidence = readStubFile(t, stubDir, "evidence.json")
	return result
}

// readStubFile returns the content of the file that a stub copied into dir, or "" when it copied
// none.
func readStubFile(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

// orHash returns hash, or def when hash is empty.
func orHash(hash, def string) string {
	if hash == "" {
		return def
	}
	return hash
}

func TestReleaseDryrun(t *testing.T) {
	genuine, hash := moduleZip(t, dryrunModule, moduleFiles)
	tampered, _ := moduleZip(t, dryrunModule, map[string]string{
		"go.mod": moduleFiles["go.mod"], "LICENSE": moduleFiles["LICENSE"],
		"m.go": moduleFiles["m.go"], "install.go": "package m\n\nfunc init() {}\n",
	})
	lanes := "releasecheck lanes -root <root>"
	validate := "releasecheck validate -root <root> -tag v2.1.0-rc.1 -sha {head} " +
		"-master {head} -ci-runs "
	migration := "apisurface -mode migration -root <root>"
	build := "zipcheck -repo <root> -version v2.1.0-dryrun -out "
	check := "zipcheck -zip "
	tests := []dryrunCase{
		{
			name: "a pull request whose release would pass", wantExit: 0,
			ran: []string{lanes, validate, migration, build, check},
			want: []string{
				"release-dryrun: validate v2.1.0-rc.1 at {head}\n",
				"release-dryrun: validate passed\n", "release-dryrun: migration passed\n",
				"release-dryrun: migration-pending passed\n", "release-dryrun: zipcheck passed\n",
				"release-verify: the module proxy serves example.com/m/v2@v2.1.0-dryrun with " +
					"the hash " + hash + " of the zip built from the tagged commit\n",
				"release-dryrun: verify passed\n", "release-dryrun: PASS\n",
			},
		},
		{
			name: "a tampered zip", zip: tampered, wantExit: 1,
			ran: []string{lanes, validate, migration, build},
			want: []string{
				"release-verify: FAIL: the module proxy serves example.com/m/v2@v2.1.0-dryrun " +
					"with the hash h1:",
				"release-dryrun: FAIL: verify\n", "release-dryrun: FAIL\n",
			},
		},
		{
			name: "a release that validate fails", env: []string{"STUB_VALIDATE=1"}, wantExit: 1,
			ran:  []string{lanes, validate, migration, build, check},
			want: []string{"release-dryrun: verify passed\n", "release-dryrun: FAIL: validate\n"},
		},
		{
			name: "a migration check that did not run", env: []string{"STUB_MIGRATION=2"},
			wantExit: 2, ran: []string{lanes, validate, migration, build, check},
			want: []string{
				"release-dryrun: did not run: migration (exit 2)\n",
				"release-dryrun: DID NOT RUN\n",
			},
		},
		{
			name: "a v1 item still pending", wantExit: 1,
			files: map[string]string{
				"tools/migration/pending.txt": "# Pending v1 items.\napi.Users.* users\n\n" +
					"  models.Old models\n",
			},
			ran: []string{lanes, validate, migration, build, check},
			want: []string{
				"release-dryrun: tools/migration/pending.txt still lists v1 items (lines that " +
					"are not blank or a comment: 2): a release needs every v1 item to have its " +
					"entry in MIGRATION.md, as apisurface -mode release checks; write the " +
					"entries and remove the lines\n",
				"release-dryrun: verify passed\n", "release-dryrun: FAIL: migration-pending\n",
				"release-dryrun: FAIL\n",
			},
		},
		{
			name: "no list of pending v1 items", wantExit: 2,
			files: map[string]string{"tools/migration/pending.txt": ""},
			ran:   []string{lanes, validate, migration, build, check},
			want: []string{
				"release-dryrun: did not run: migration-pending (exit 2)\n",
				"release-dryrun: DID NOT RUN\n",
			},
		},
		{
			name: "a zip that zipcheck fails", env: []string{"STUB_ZIPCHECK=1"}, wantExit: 1,
			ran: []string{lanes, validate, migration, build},
			want: []string{
				"release-dryrun: verify does not run, as zipcheck did not pass\n",
				"release-dryrun: did not run: verify\n", "release-dryrun: FAIL: zipcheck\n",
			},
		},
		{
			name: "a zipcheck that printed no hash", hash: "sha256:abc", wantExit: 2,
			ran: []string{lanes, validate, migration, build},
			want: []string{
				"release-dryrun: zipcheck printed no h1: hash of the module at v2.1.0-dryrun\n",
				"release-dryrun: did not run: verify (exit 2)\n",
			},
		},
		{
			name: "a zipcheck that printed the hash of another version", wantExit: 2,
			env: []string{"STUB_VERSION=v2.1.0"},
			ran: []string{lanes, validate, migration, build},
			want: []string{
				"release-dryrun: zipcheck printed no h1: hash of the module at v2.1.0-dryrun\n",
				"release-dryrun: did not run: verify (exit 2)\n",
			},
		},
		{
			name: "a commit that is not a hash", args: []string{"HEAD"}, wantExit: 2,
			want: []string{"release-dryrun: \"HEAD\" is not a full commit hash\n"},
		},
		{
			name: "a commit that is not HEAD", args: []string{otherCommit}, wantExit: 2,
			want: []string{"release-dryrun: " + otherCommit + " is not HEAD, {head}, whose tree " +
				"zipcheck builds: check out " + otherCommit + " first\n"},
		},
		{
			name: "no argument", args: []string{}, wantExit: 2,
			want: []string{"release-dryrun: usage: release-dryrun.sh <sha>\n"},
		},
		{
			name: "a version.go without Version", wantExit: 2,
			files: map[string]string{"version.go": "package permit\n"},
			want: []string{"release-dryrun: version.go has no line const Version = " +
				"\"<version>\"\n"},
		},
		{
			name: "test legs that releasecheck cannot read", wantExit: 2,
			env: []string{"STUB_LANES_EXIT=2", "STUB_LANES=releasecheck: DID NOT RUN: ci.yml: " +
				"no such file or directory\nreleasecheck: DID NOT RUN\n"},
			ran: []string{lanes},
			want: []string{
				"releasecheck: DID NOT RUN: ci.yml: no such file or directory\n",
				"release-dryrun: releasecheck lanes printed no Go release of a test leg of " +
					".github/workflows/ci.yml\nrelease-dryrun: DID NOT RUN\n",
			},
		},
		{
			name: "no test legs", wantExit: 2, env: []string{"STUB_LANES="}, ran: []string{lanes},
			want: []string{"release-dryrun: releasecheck lanes printed no Go release of a test " +
				"leg of .github/workflows/ci.yml\n"},
		},
		{
			name: "no zipcheck", wantExit: 2,
			files: map[string]string{"bin/zipcheck": ""},
			want:  []string{"release-dryrun: zipcheck is not at "},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runDryrun(t, tt, genuine, hash)

			if result.exit != tt.wantExit {
				t.Errorf("release-dryrun.sh exited %d, want %d:\n%s%s", result.exit,
					tt.wantExit, result.stdout, result.stderr)
			}
			for _, part := range tt.want {
				if part = strings.ReplaceAll(part, "{head}", result.head); !strings.Contains(
					result.stdout, part) {
					t.Errorf("release-dryrun.sh printed\n%s\nwant %q", result.stdout, part)
				}
			}
			if !slices.EqualFunc(result.ran, tt.ran, strings.HasPrefix) {
				t.Errorf("the tools ran\n%q\nwant\n%q", result.ran, tt.ran)
			}
		})
	}
}

// The run of CI that the dry run gives releasecheck is one push of the commit that succeeded,
// in its latest attempt; and the harness evidence is of the commit, on each Go release that
// releasecheck lanes printed, with each phase of tools/e2e/local-only.txt passed.
func TestReleaseDryrunInputs(t *testing.T) {
	genuine, hash := moduleZip(t, dryrunModule, moduleFiles)

	result := runDryrun(t, dryrunCase{}, genuine, hash)

	if result.exit != 0 {
		t.Fatalf("release-dryrun.sh exited %d:\n%s%s", result.exit, result.stdout,
			result.stderr)
	}
	var runs struct {
		TotalCount   int `json:"total_count"`
		WorkflowRuns []struct {
			ID         int64  `json:"id"`
			RunAttempt int    `json:"run_attempt"`
			HeadSHA    string `json:"head_sha"`
			Event      string `json:"event"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		} `json:"workflow_runs"`
	}
	if err := json.Unmarshal([]byte(result.ciRuns), &runs); err != nil {
		t.Fatalf("the runs of CI are not JSON: %v\n%s", err, result.ciRuns)
	}
	if len(runs.WorkflowRuns) != 1 || runs.TotalCount != 1 {
		t.Fatalf("the runs of CI are %+v; want one run", runs)
	}
	if run := runs.WorkflowRuns[0]; run.HeadSHA != result.head || run.Event != "push" ||
		run.Status != "completed" || run.Conclusion != "success" || run.ID <= 0 ||
		run.RunAttempt <= 0 {
		t.Errorf("the run of CI is %+v; want a push of %s that completed with success, with an "+
			"id and an attempt", run, result.head)
	}
	var got struct {
		SDKCommit    string   `json:"sdk_commit"`
		ReportSHA256 string   `json:"report_sha256"`
		Lanes        []string `json:"lanes"`
		PassedPhases []string `json:"passed_phases"`
	}
	if err := json.Unmarshal([]byte(result.evidence), &got); err != nil {
		t.Fatalf("the evidence is not JSON: %v\n%s", err, result.evidence)
	}
	slices.Sort(got.Lanes)
	if got.SDKCommit != result.head || got.ReportSHA256 != strings.Repeat("0", 64) ||
		!slices.Equal(got.Lanes, []string{"go1.26.0", "go1.26.9", "go1.27.2"}) ||
		!slices.Equal(got.PassedPhases, []string{"api.api_keys", "api.projects"}) {
		t.Errorf("the evidence is %+v; want the commit, zeros, the three legs and the two phases",
			got)
	}
}
